package enginehelper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nas-content-manager/internal/enginecontract"
)

// isCollectionTask 限制资源组件路由只能读取或恢复单产品部署/启动意图。
func isCollectionTask(task enginecontract.Task) bool {
	return (task.Action == "deploy" || task.Action == "start") && len(task.Components) == 1 && enginecontract.IsCollectionComponent(task.Components[0])
}

// latestCollectionTasks 有界读取保留历史，不增加第二套任务库。
func (s *store) latestCollectionTasks(ctx context.Context) (map[string]enginecontract.Task, error) {
	result := map[string]enginecontract.Task{}
	rows, err := s.db.QueryContext(ctx, "SELECT body FROM tasks ORDER BY updated DESC LIMIT 101")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var body string
		var task enginecontract.Task
		if rows.Scan(&body) != nil || json.Unmarshal([]byte(body), &task) != nil {
			return nil, errors.New("组件部署历史快照不可读取")
		}
		if isCollectionTask(task) {
			component := task.Components[0]
			if _, exists := result[component]; !exists {
				result[component] = task
			}
		}
	}
	return result, rows.Err()
}

// collectionRevision 绑定容器身份、运行状态与最近任务版本，避免启动旧快照。
func collectionRevision(info *containerInfo, task enginecontract.Task) int64 {
	identity := "missing"
	if info != nil {
		identity = info.ID + "|" + info.Image + "|" + info.State.Status + "|" + strconv.FormatBool(info.State.Running)
	}
	digest := sha256.Sum256([]byte(identity + "|" + task.ID + "|" + strconv.FormatInt(task.Revision, 10)))
	return int64(binary.BigEndian.Uint64(digest[:8])&0x001ffffffffffffe) + 1
}

// collectionStatus 只读检测固定Docker资源与匿名HTTP，不认领卷或改变应用配置。
func (s *Service) collectionStatus(parent context.Context) enginecontract.CollectionStatus {
	ctx, cancel := context.WithTimeout(parent, 22*time.Second)
	defer cancel()
	result := enginecontract.CollectionStatus{Helper: "ready", Message: "部署助手可用；容器运行与API接入分别检测", TemplateVersion: enginecontract.CollectionTemplateVersion, NoticeVersion: enginecontract.CollectionNoticeVersion, NoticeText: enginecontract.CollectionNoticeText, Components: []enginecontract.CollectionComponent{}}
	var preErr error
	if !s.config.CollectionEnabled {
		result.Helper, result.Message = "not_enabled", "组件部署尚未启用，请使用当前Linux托管部署配置或Windows Docker完整模式；也可连接已有服务"
		preErr = errors.New(result.Message)
	} else if preErr = s.preflight(ctx); preErr != nil {
		result.Helper, result.Message = "unavailable", preErr.Error()
	}
	latest, historyErr := s.store.latestCollectionTasks(ctx)
	active, activeErr := s.store.activeTask(ctx)
	if historyErr != nil || activeErr != nil {
		result.Helper, result.Message = "unavailable", "部署任务历史暂不可读取，请稍后刷新"
		preErr = errors.New(result.Message)
	}
	s.opMu.Lock()
	quiescing := s.quiescing
	s.opMu.Unlock()
	if active != nil && isCollectionTask(*active) {
		result.ActiveTaskID = active.ID
	}
	client := s.collectionHTTP
	for _, id := range enginecontract.CollectionComponents() {
		definition, _ := collectionDefinitionFor(id)
		item := enginecontract.CollectionComponent{ID: id, Name: definition.Name, Version: definition.Version, SetupPort: definition.Setup, Presence: "unknown", Runtime: "unknown", Readiness: "not_configured", Message: result.Message, Revision: collectionRevision(nil, latest[id]), LatestTaskID: latest[id].ID}
		if preErr == nil {
			info, err := s.docker.inspect(ctx, containerName(s.store.installation, id))
			item.Revision = collectionRevision(info, latest[id])
			switch {
			case err != nil:
				item.Message = "Docker组件状态暂不可读取，请稍后刷新"
			case info == nil:
				if err = s.collectionPrivateVolumes(ctx, id, false, false); err != nil {
					item.Presence, item.Readiness, item.Message = "conflict", "error", err.Error()
				} else if s.config.StorageMode == "bind" && s.ownedDirectory("collection-media", false) != nil {
					item.Presence, item.Readiness, item.Message = "conflict", "error", "独立资源目录包含未知数据，不能部署组件"
				} else {
					item.Presence, item.Message, item.CanDeploy = "missing", "尚未部署，可创建固定组件及独立配置卷", active == nil && !quiescing
				}
			case s.validateCollectionRuntime(ctx, info, definition.Template) != nil:
				item.Presence, item.Readiness, item.Message = "conflict", "error", "组件实际镜像、配置卷或模板归属冲突，请人工核对；不会接管"
			default:
				item.Presence, item.Runtime, item.Message, item.CanStart = "present", "stopped", "组件已停止，可启动原容器；配置和数据保留", active == nil && !quiescing
				if info.State.Running {
					item.Runtime, item.Readiness, item.CanStart, item.Message = "running", "starting", false, "容器已运行，等待组件HTTP接口响应"
					probeContext, probeCancel := context.WithTimeout(ctx, 2*time.Second)
					probeErr := probeCollection(probeContext, client, s.engineURL(definition.Template), id)
					probeCancel()
					if probeErr == nil {
						item.Readiness, item.Message, item.ServiceURL = "configuration_required", "组件可访问，待首次配置和API凭据连接；已配置的服务可直接连接", s.engineURL(definition.Template)
					}
				}
			}
		}
		if active != nil && isCollectionTask(*active) && active.Components[0] == id {
			item.CanDeploy, item.CanStart = false, false
			item.Readiness, item.Message = "starting", active.Message
		}
		result.Components = append(result.Components, item)
	}
	return result
}

// handleCollectionStatus 返回不含凭据的四组件快照，读取不会产生部署副作用。
func (s *Service) handleCollectionStatus(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, s.collectionStatus(r.Context()))
}

// handleCollectionDeploy 仅接受当前固定模板和主动目录授权，持久化后独立执行。
func (s *Service) handleCollectionDeploy(w http.ResponseWriter, r *http.Request) {
	var request enginecontract.CollectionDeployRequest
	if !decode(w, r, &request) {
		return
	}
	if !safeID(request.RequestID) || !enginecontract.IsCollectionComponent(request.Component) {
		failure(w, 400, "请选择受支持的组件并提供有效请求标识")
		return
	}
	if request.TemplateVersion != enginecontract.CollectionTemplateVersion || request.NoticeVersion != enginecontract.CollectionNoticeVersion || !request.AcceptNotice {
		failure(w, 409, "请刷新固定部署方案并主动确认组件目录访问告知")
		return
	}
	signature := taskSignature("deploy", []string{request.Component}, request.TemplateVersion+"|"+request.NoticeVersion)
	if s.respondExistingCollectionRequest(w, r, request.RequestID, signature) {
		return
	}
	if !s.config.CollectionEnabled {
		failure(w, 503, "当前部署尚未启用资源组件安装，请应用受管Docker部署配置")
		return
	}
	if s.respondActiveCollectionIntent(w, r, request.RequestID, signature, request.Component, "deploy") {
		return
	}
	status := s.collectionStatus(r.Context())
	allowed := false
	for _, item := range status.Components {
		if item.ID == request.Component && item.CanDeploy {
			allowed = true
		}
	}
	if !allowed {
		writeError(w, errConflict)
		return
	}
	task, err := s.submit(r.Context(), request.RequestID, signature, []string{request.Component})
	if err != nil {
		writeError(w, err)
		return
	}
	s.logger.Info("已接收资源组件Docker部署授权，后续仍需首次配置", "task", task.ID, "component", request.Component)
	s.respondCollectionTask(w, r, task.ID, http.StatusAccepted)
}

// respondExistingCollectionRequest 在资源探测前恢复同一请求，响应丢失不会创建新意图。
func (s *Service) respondExistingCollectionRequest(w http.ResponseWriter, r *http.Request, requestID string, signature string) bool {
	old, err := s.store.binding(r.Context(), requestID, signature)
	if err != nil {
		writeError(w, err)
		return true
	}
	if old == nil {
		return false
	}
	if !isCollectionTask(*old) {
		writeError(w, errConflict)
		return true
	}
	s.respondCollectionTask(w, r, old.ID, http.StatusAccepted)
	return true
}

// respondActiveCollectionIntent 为相同活动意图绑定新请求键，其他组件保持明确忙碌响应。
func (s *Service) respondActiveCollectionIntent(w http.ResponseWriter, r *http.Request, requestID string, signature string, component string, action string) bool {
	task, err := s.aliasActiveCollectionIntent(r.Context(), requestID, signature, component, action)
	if err != nil {
		writeError(w, err)
		return true
	}
	if task == nil {
		return false
	}
	s.respondCollectionTask(w, r, task.ID, http.StatusAccepted)
	return true
}

// aliasActiveCollectionIntent 在同一短锁内核对完整意图并只绑定已存在的任务。
func (s *Service) aliasActiveCollectionIntent(ctx context.Context, requestID string, signature string, component string, action string) (*enginecontract.Task, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if old, err := s.store.binding(ctx, requestID, signature); err != nil || old != nil {
		return old, err
	}
	active, err := s.store.activeTask(ctx)
	if err != nil || active == nil {
		return nil, err
	}
	if !isCollectionTask(*active) || active.Action != action || active.Components[0] != component {
		return nil, errBusy
	}
	if err = s.validateCollectionIntentSignature(ctx, active.ID, signature); err != nil {
		return nil, err
	}
	// 此处只添加别名；原任务刚结束时也不新建绕过状态校验的任务。
	if err = s.store.alias(ctx, requestID, signature, active.ID); err != nil {
		return nil, err
	}
	return active, nil
}

// validateCollectionIntentSignature 要求活动组件任务已有完全相同的持久化意图。
func (s *Service) validateCollectionIntentSignature(ctx context.Context, id string, signature string) error {
	var bindings int
	if err := s.store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM requests WHERE task_id=? AND signature=?", id, signature).Scan(&bindings); err != nil {
		return err
	}
	if bindings == 0 {
		return errConflict
	}
	return nil
}

// respondCollectionTask 读取固定任务快照，不把任务执行失败误当作读取失败。
func (s *Service) respondCollectionTask(w http.ResponseWriter, r *http.Request, id string, status int) {
	response, err := s.store.response(r.Context(), id, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, status, response)
}

// handleCollectionStart 只启动当前快照中的原受管容器，不补建配置卷或拉取镜像。
func (s *Service) handleCollectionStart(w http.ResponseWriter, r *http.Request) {
	var request enginecontract.ActionRequest
	if !decode(w, r, &request) {
		return
	}
	component := r.PathValue("component")
	if !enginecontract.IsCollectionComponent(component) || !safeID(request.RequestID) || request.Revision < 1 {
		failure(w, 400, "组件启动参数无效")
		return
	}
	signature := fmt.Sprintf("start|%s|%d", component, request.Revision)
	if s.respondExistingCollectionRequest(w, r, request.RequestID, signature) {
		return
	}
	if !s.config.CollectionEnabled {
		failure(w, 503, "当前未启用资源组件部署，请恢复受管Docker配置")
		return
	}
	if s.respondActiveCollectionIntent(w, r, request.RequestID, signature, component, "start") {
		return
	}
	status := s.collectionStatus(r.Context())
	allowed := false
	for _, item := range status.Components {
		if item.ID == component && item.CanStart && item.Revision == request.Revision {
			allowed = true
		}
	}
	if !allowed {
		writeError(w, errConflict)
		return
	}
	task, err := s.submit(r.Context(), request.RequestID, signature, []string{component})
	if err != nil {
		writeError(w, err)
		return
	}
	s.respondCollectionTask(w, r, task.ID, http.StatusAccepted)
}

// handleCollectionTask 校验任务范围和单值有界事件游标，禁止读取其他引擎任务。
func (s *Service) handleCollectionTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := r.URL.Query()
	after := int64(0)
	var err error
	if len(query) > 1 || len(query["afterSequence"]) > 1 {
		failure(w, 400, "组件任务事件游标无效")
		return
	}
	for key := range query {
		if key != "afterSequence" {
			failure(w, 400, "组件任务事件游标无效")
			return
		}
	}
	if value := query.Get("afterSequence"); value != "" {
		after, err = strconv.ParseInt(value, 10, 64)
	}
	if !safeID(id) || after < 0 || err != nil {
		failure(w, 400, "组件任务标识或游标无效")
		return
	}
	task, err := s.store.getTask(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if !isCollectionTask(task) {
		failure(w, 404, "组件部署任务不存在")
		return
	}
	response, err := s.store.response(r.Context(), id, after)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, 200, response)
}

// handleCollectionRetry 复用同一持久重试机制，仅允许资源组件原失败任务。
func (s *Service) handleCollectionRetry(w http.ResponseWriter, r *http.Request) {
	if !s.config.CollectionEnabled {
		failure(w, 503, "当前未启用资源组件部署，请恢复受管Docker部署配置")
		return
	}
	s.retryTask(w, r, true)
}

// collectionErrorForTask 返回执行器可记录的固定中文组件错误，不传递上游响应。
func collectionErrorForTask(component string) error {
	return fmt.Errorf("%s HTTP接口就绪检查超时，请检查固定端口、资源和组件日志；配置已保留，可重试", strings.ToUpper(component[:1])+component[1:])
}
