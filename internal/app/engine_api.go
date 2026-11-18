package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"nas-content-manager/internal/enginecontract"
)

// engineRoutes 为部署控制面注册现有管理员会话和同源保护。
func (a *App) engineRoutes(mux *http.ServeMux) {
	for path, handler := range map[string]http.HandlerFunc{
		"GET /api/engines":                             a.engineStatus,
		"POST /api/engines/probe":                      a.engineProbe,
		"POST /api/engines/initializations":            a.engineInitialize,
		"GET /api/engines/initializations/{id}":        a.engineTask,
		"POST /api/engines/initializations/{id}/retry": a.engineRetry,
		"POST /api/engines/{component}/start":          a.engineStart,
		"POST /api/engines/search/sync-sources":        a.engineSyncSources,
	} {
		mux.HandleFunc(path, a.auth(handler))
	}
}

// engineMode 返回不可变启动配置中的模块模式，不接受浏览器切换部署边界。
func (a *App) engineMode(component string) string {
	if component == enginecontract.QBittorrent {
		return a.config.Downloads.Mode
	}
	if component == enginecontract.Jackett {
		return a.config.TorrentSearch.Mode
	}
	return ""
}

// helperCall 仅向固定 Unix socket 发送有界白名单 DTO，不转发浏览器地址或头部。
func (a *App) helperCall(ctx context.Context, method string, path string, input any, output any) (int, error) {
	if a.config.Engines.Socket == "" {
		return 503, fmt.Errorf("初始化助手尚未启用，请先应用 compose.engines.yaml 部署配置")
	}
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return 400, fmt.Errorf("初始化参数无效")
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://engine-helper"+path, body)
	if err != nil {
		return 400, fmt.Errorf("初始化请求无效")
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := a.engines.http.Do(request)
	if err != nil {
		return 503, fmt.Errorf("初始化助手暂不可用，请检查助手服务和私有通信目录")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if err != nil || len(data) > 256<<10 {
		return 502, fmt.Errorf("初始化助手返回内容无效")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// 不透传原始 Docker、网络或私密配置错误，助手事件保留受控中文诊断。
		status := response.StatusCode
		if status != 400 && status != 404 && status != 409 && status != 413 && status != 429 && status != 503 {
			status = 502
		}
		messages := map[int]string{400: "初始化参数无效，请刷新页面重试", 404: "初始化任务已不存在，请重新读取当前引擎状态", 409: "操作状态或版本已改变，请刷新引擎状态后重试", 413: "初始化请求超过大小限制", 429: "已有初始化任务正在执行，请稍后重试", 503: "初始化助手或 Docker 暂不可用，请检查部署配置", 502: "初始化操作未能完成，请查看任务诊断后重试"}
		return status, fmt.Errorf("%s", messages[status])
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(output) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return 502, fmt.Errorf("初始化助手响应格式无效")
	}
	return response.StatusCode, nil
}

// inspectEngines 合并助手资源快照与应用实际接入状态，助手故障不会禁用已就绪功能。
func (a *App) inspectEngines(ctx context.Context, probe bool) enginecontract.Status {
	status := enginecontract.Status{Helper: "not_enabled", Message: "请先按托管部署说明启用初始化助手", Components: []enginecontract.Component{}, NoticeVersion: enginecontract.NoticeVersion, NoticeText: enginecontract.NoticeText}
	managed := a.config.Downloads.Mode == "managed" || a.config.TorrentSearch.Mode == "managed"
	if a.config.Engines.Socket != "" && managed {
		path, method := "/api/engines", http.MethodGet
		var input any
		if probe {
			path, method, input = "/api/engines/probe", http.MethodPost, struct{}{}
		}
		if _, err := a.helperCall(ctx, method, path, input, &status); err != nil {
			status.Helper = "unavailable"
			status.Message = err.Error()
			status.Components = []enginecontract.Component{}
		}
	}
	byID := map[string]enginecontract.Component{}
	for _, c := range status.Components {
		if c.ID == enginecontract.QBittorrent || c.ID == enginecontract.Jackett {
			byID[c.ID] = c
		}
	}
	status.Components = []enginecontract.Component{}
	a.engines.mu.Lock()
	defer a.engines.mu.Unlock()
	for _, component := range []string{enginecontract.QBittorrent, enginecontract.Jackett} {
		c, exists := byID[component]
		if !exists {
			c = enginecontract.Component{ID: component, Presence: "unknown", Runtime: "unknown", Readiness: "not_configured"}
		}
		c.Mode = a.engineMode(component)
		if c.Mode != "managed" {
			c.Presence, c.Runtime, c.CanInitialize, c.CanStart = "unknown", "unknown", false, false
			if c.Mode == "disabled" {
				c.Readiness, c.Message = "not_configured", "模块未启用；自动初始化需要先以 managed 模式部署"
			} else {
				c.Readiness, c.Message = "connecting", "使用外接引擎，请检查原部署配置；不会自动接管或重新安装"
			}
		} else if status.Helper != "ready" {
			c.CanInitialize, c.CanStart = false, false
		}
		if c.Mode != "disabled" {
			if component == enginecontract.QBittorrent {
				download := a.downloads.connectionStatus()
				if c.Mode == "managed" && !download.Enabled && c.Readiness == "ready" {
					c.Readiness, c.Message = "connecting", "引擎部署完成，正在等待应用连接验证"
				}
				if download.Enabled {
					c.Readiness = download.State
					if c.Readiness == "offline" {
						c.Readiness = "error"
					}
					if download.State == "ready" {
						c.Runtime = "running"
						c.Message = "下载引擎已通过应用连接验证"
					} else if download.Error != "" {
						c.Message = download.Error
					}
				}
			} else {
				config, _ := a.torrentSearch.snapshotConfig()
				if config.Enabled {
					c.Readiness, c.Message = "ready", "已接入选定来源；实际站点可用性将在搜索时逐项报告"
				} else if c.Mode == "managed" && a.engines.jackettConnection != nil {
					c.Readiness, c.Message = "configuration_required", "搜索服务已安装，请通过回环管理入口配置站点后同步来源"
				} else if c.Mode == "managed" && c.Readiness == "ready" {
					c.Readiness, c.Message = "connecting", "引擎部署完成，正在读取私有连接清单"
				}
			}
		}
		if diagnostic := a.engines.diagnostics[component]; c.Mode == "managed" && diagnostic != "" {
			c.Message = diagnostic
			if state := a.engines.diagnosticStates[component]; state != "" {
				c.Readiness = state
			} else if c.Readiness != "ready" {
				c.Readiness = "error"
			}
		}
		if diagnostic := a.engines.diagnostics["manifest"]; c.Mode == "managed" && diagnostic != "" && c.Readiness != "ready" {
			c.Message = diagnostic
			c.Readiness = "error"
		}
		status.Components = append(status.Components, c)
	}
	return status
}

// engineStatus 仅检测资源与读取应用快照，GET 不拉取镜像或启动容器。
func (a *App) engineStatus(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, a.inspectEngines(r.Context(), false))
}

// engineProbe 显式重读完整连接清单和健康状态，不创建或重建引擎。
func (a *App) engineProbe(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	a.refreshManagedConnections(r.Context())
	if a.downloads.connectionStatus().Enabled {
		status, err := a.downloads.engine.probe(r.Context())
		a.downloads.setConnection(status, err)
	}
	respond(w, 200, a.inspectEngines(r.Context(), true))
}

// validateManagedComponents 防止对 disabled 或 external 组件调用部署动作。
func (a *App) validateManagedComponents(components []string) bool {
	if len(components) < 1 || len(components) > 2 {
		return false
	}
	seen := map[string]bool{}
	for _, component := range components {
		if a.engineMode(component) != "managed" || seen[component] {
			return false
		}
		seen[component] = true
	}
	return true
}

// engineInitialize 将用户明确确认的部署意图交给独立助手持久化，浏览器断开不取消任务。
func (a *App) engineInitialize(w http.ResponseWriter, r *http.Request) {
	var input enginecontract.InitializationRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validDownloadRequestID(input.RequestID) || !a.validateManagedComponents(input.Components) {
		fail(w, 400, "请提供有效请求编号及已启用托管模式的组件")
		return
	}
	for _, c := range input.Components {
		if c == enginecontract.QBittorrent && (!input.AcceptNotice || input.NoticeVersion != enginecontract.NoticeVersion) {
			fail(w, 409, "请阅读并主动确认当前版本的 qBittorrent 首次使用告知")
			return
		}
	}
	var response enginecontract.TaskResponse
	code, err := a.helperCall(r.Context(), http.MethodPost, "/api/engines/initializations", input, &response)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	a.logger.Info("已提交托管引擎初始化请求", "taskId", response.Task.ID)
	respond(w, 202, response)
}

// engineTaskPath 仅从安全任务标识和有界游标派生助手路径。
func helperEngineTaskPath(id string, query url.Values) (string, bool) {
	if !engineIdentityPattern.MatchString(id) {
		return "", false
	}
	path := "/api/engines/initializations/" + id
	if len(query) > 1 {
		return "", false
	}
	if value := query.Get("afterSequence"); value != "" {
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil || number < 0 {
			return "", false
		}
		path += "?afterSequence=" + strconv.FormatInt(number, 10)
	}
	for key := range query {
		if key != "afterSequence" || len(query[key]) != 1 {
			return "", false
		}
	}
	return path, true
}

// engineTask 有界读取持久任务，执行失败仍以成功读取的 HTTP200 返回。
func (a *App) engineTask(w http.ResponseWriter, r *http.Request) {
	path, ok := helperEngineTaskPath(r.PathValue("id"), r.URL.Query())
	if !ok {
		fail(w, 400, "初始化任务编号或事件游标无效")
		return
	}
	var response enginecontract.TaskResponse
	code, err := a.helperCall(r.Context(), http.MethodGet, path, nil, &response)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	respond(w, 200, response)
}

// engineRetry 仅重试仍属托管配置的原任务，不重新初始化已成功组件。
func (a *App) engineRetry(w http.ResponseWriter, r *http.Request) {
	var input enginecontract.ActionRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	path, ok := helperEngineTaskPath(r.PathValue("id"), nil)
	if !ok || !validDownloadRequestID(input.RequestID) || input.Revision < 1 {
		fail(w, 400, "重试参数无效")
		return
	}
	var previous enginecontract.TaskResponse
	code, err := a.helperCall(r.Context(), http.MethodGet, path, nil, &previous)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	if !a.validateManagedComponents(previous.Task.Components) {
		fail(w, 409, "原任务组件已不在托管模式，请恢复部署配置后重试")
		return
	}
	var response enginecontract.TaskResponse
	code, err = a.helperCall(r.Context(), http.MethodPost, path+"/retry", input, &response)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	respond(w, 202, response)
}

// engineStart 仅允许固定托管组件启动既有归属明确的容器，状态由助手再次核对。
func (a *App) engineStart(w http.ResponseWriter, r *http.Request) {
	component := r.PathValue("component")
	var input enginecontract.ActionRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !a.validateManagedComponents([]string{component}) || !validDownloadRequestID(input.RequestID) || input.Revision < 1 || strings.Contains(component, "/") {
		fail(w, 400, "启动参数无效或组件未使用托管模式")
		return
	}
	var response enginecontract.TaskResponse
	code, err := a.helperCall(r.Context(), http.MethodPost, "/api/engines/"+component+"/start", input, &response)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	respond(w, 202, response)
}
