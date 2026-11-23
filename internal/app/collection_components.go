package app

import (
	"net/http"
	"strings"

	"nas-content-manager/internal/enginecontract"
)

// collectionComponentRoutes 仅注册四类固定组件的受认证部署接口。
func (a *App) collectionComponentRoutes(register func(string, http.HandlerFunc)) {
	register("GET /api/collection/components", a.collectionComponentStatus)
	register("POST /api/collection/components/deploy", a.collectionComponentDeploy)
	register("POST /api/collection/components/{component}/start", a.collectionComponentStart)
	register("GET /api/collection/components/tasks/{id}", a.collectionComponentTask)
	register("POST /api/collection/components/tasks/{id}/retry", a.collectionComponentRetry)
}

// unavailableCollectionComponents 返回明确的能力缺失快照，读取不会安装任何资源。
func unavailableCollectionComponents(message string) enginecontract.CollectionStatus {
	status := enginecontract.CollectionStatus{Helper: "not_enabled", Message: message, TemplateVersion: enginecontract.CollectionTemplateVersion, NoticeVersion: enginecontract.CollectionNoticeVersion, NoticeText: enginecontract.CollectionNoticeText, Components: []enginecontract.CollectionComponent{}}
	names := map[string]string{enginecontract.MoviePilot: "MoviePilot", enginecontract.AutoBangumi: "AutoBangumi", enginecontract.Sonarr: "Sonarr", enginecontract.Radarr: "Radarr"}
	for _, id := range enginecontract.CollectionComponents() {
		status.Components = append(status.Components, enginecontract.CollectionComponent{ID: id, Name: names[id], Presence: "unknown", Runtime: "unknown", Readiness: "not_configured", Message: message})
	}
	return status
}

// collectionComponentStatus 通过私有助手读取实际能力，旧部署仍保留连接已有实例路径。
func (a *App) collectionComponentStatus(w http.ResponseWriter, r *http.Request) {
	status := unavailableCollectionComponents("请使用 Docker 完整模式，或在 Linux NAS 托管部署中启用资源组件安装；已有实例仍可直接连接")
	if a.config.Engines.Socket == "" {
		respond(w, http.StatusOK, status)
		return
	}
	code, err := a.helperCall(r.Context(), http.MethodGet, "/api/collection/components", nil, &status)
	if err != nil {
		status = unavailableCollectionComponents("部署助手暂不可用，请检查 Docker 完整模式或 Linux NAS 的助手服务后重新检测")
		status.Helper = "unavailable"
		if code == http.StatusNotFound {
			status.Helper = "unsupported"
			status.Message = "当前部署助手尚不支持资源组件安装，请更新完整部署；已有实例仍可直接连接"
		}
	}
	respond(w, http.StatusOK, status)
}

// validCollectionComponentTask 防止新控制面读取或重试下载引擎及完整停止任务。
func validCollectionComponentTask(task enginecontract.Task) bool {
	return engineIdentityPattern.MatchString(task.ID) && len(task.Components) == 1 && enginecontract.IsCollectionComponent(task.Components[0]) && (task.Action == "deploy" || task.Action == "start")
}

// collectionComponentDeploy 只提交已主动确认的固定组件模板，凭据仍由原连接流程保存。
func (a *App) collectionComponentDeploy(w http.ResponseWriter, r *http.Request) {
	var input enginecontract.CollectionDeployRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validDownloadRequestID(input.RequestID) || !enginecontract.IsCollectionComponent(input.Component) {
		fail(w, http.StatusBadRequest, "请选择支持的固定组件并提供有效请求编号")
		return
	}
	if !input.AcceptNotice || input.TemplateVersion != enginecontract.CollectionTemplateVersion || input.NoticeVersion != enginecontract.CollectionNoticeVersion {
		fail(w, http.StatusConflict, "部署模板或目录授权已变化，请重新检测并主动确认")
		return
	}
	var result enginecontract.TaskResponse
	code, err := a.helperCall(r.Context(), http.MethodPost, "/api/collection/components/deploy", input, &result)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	if !validCollectionComponentTask(result.Task) || result.Task.Components[0] != input.Component || result.Task.Action != "deploy" {
		fail(w, http.StatusBadGateway, "部署回执暂不可确认，请保留原请求编号重试核对")
		return
	}
	a.logger.Info("资源组件部署请求已受理，首次配置和API连接仍需完成", "component", input.Component, "taskId", result.Task.ID)
	respond(w, http.StatusAccepted, result)
}

// collectionComponentTaskPath 复用安全任务标识及有界事件游标校验，不开放任意助手路径。
func collectionComponentTaskPath(r *http.Request) (string, bool) {
	path, ok := helperEngineTaskPath(r.PathValue("id"), r.URL.Query())
	if !ok {
		return "", false
	}
	return "/api/collection/components/tasks/" + strings.TrimPrefix(path, "/api/engines/initializations/"), true
}

// collectionComponentTask 有界读取单个资源组件的部署任务，执行失败也保留可恢复快照。
func (a *App) collectionComponentTask(w http.ResponseWriter, r *http.Request) {
	path, ok := collectionComponentTaskPath(r)
	if !ok {
		fail(w, http.StatusBadRequest, "组件部署任务编号或事件游标无效")
		return
	}
	var result enginecontract.TaskResponse
	code, err := a.helperCall(r.Context(), http.MethodGet, path, nil, &result)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	if !validCollectionComponentTask(result.Task) || result.Task.ID != r.PathValue("id") {
		fail(w, http.StatusNotFound, "资源组件部署任务不存在")
		return
	}
	respond(w, http.StatusOK, result)
}

// collectionComponentStart 仅启动已归属本安装的原组件，不隐式重装或修改配置。
func (a *App) collectionComponentStart(w http.ResponseWriter, r *http.Request) {
	var input enginecontract.ActionRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	component := r.PathValue("component")
	if !enginecontract.IsCollectionComponent(component) || !validDownloadRequestID(input.RequestID) || input.Revision < 1 {
		fail(w, http.StatusBadRequest, "组件、请求编号或当前修订号无效")
		return
	}
	var result enginecontract.TaskResponse
	code, err := a.helperCall(r.Context(), http.MethodPost, "/api/collection/components/"+component+"/start", input, &result)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	if !validCollectionComponentTask(result.Task) || result.Task.Components[0] != component || result.Task.Action != "start" {
		fail(w, http.StatusBadGateway, "组件启动回执暂不可确认，请保留原请求编号重试核对")
		return
	}
	a.logger.Info("已受理原资源组件启动请求", "component", component, "taskId", result.Task.ID)
	respond(w, http.StatusAccepted, result)
}

// collectionComponentRetry 先核对原任务作用域，再按原修订重试失败的固定组件。
func (a *App) collectionComponentRetry(w http.ResponseWriter, r *http.Request) {
	var input enginecontract.ActionRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	path, ok := collectionComponentTaskPath(r)
	if !ok || len(r.URL.Query()) != 0 || !validDownloadRequestID(input.RequestID) || input.Revision < 1 {
		fail(w, http.StatusBadRequest, "重试请求编号、任务或修订号无效")
		return
	}
	var previous enginecontract.TaskResponse
	code, err := a.helperCall(r.Context(), http.MethodGet, path, nil, &previous)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	if !validCollectionComponentTask(previous.Task) || previous.Task.ID != r.PathValue("id") {
		fail(w, http.StatusNotFound, "资源组件部署任务不存在")
		return
	}
	var result enginecontract.TaskResponse
	code, err = a.helperCall(r.Context(), http.MethodPost, path+"/retry", input, &result)
	if err != nil {
		fail(w, code, err.Error())
		return
	}
	if !validCollectionComponentTask(result.Task) || result.Task.ID != previous.Task.ID || result.Task.Components[0] != previous.Task.Components[0] || result.Task.Action != previous.Task.Action {
		fail(w, http.StatusBadGateway, "组件重试回执暂不可确认，请保留原请求编号重试核对")
		return
	}
	a.logger.Info("资源组件失败步骤重试已受理", "component", result.Task.Components[0], "taskId", result.Task.ID)
	respond(w, http.StatusAccepted, result)
}
