package app

import (
	"net/http"

	"nas-content-manager/internal/collectioncontract"
)

// collectionUAVSearch 通过已授权的视频来源搜索固定站点，不保存关键词或媒体链接。
func (a *App) collectionUAVSearch(w http.ResponseWriter, r *http.Request) {
	var input collectioncontract.UAVSearchRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input, err := collectioncontract.NormalizeUAVSearch(input)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	source, err := a.collectionSourceByID(r.Context(), r.PathValue("id"))
	if err != nil || source.Kind != "uav" {
		fail(w, 404, "视频插件来源不存在")
		return
	}
	if !source.Enabled {
		fail(w, 409, "视频插件来源已停用")
		return
	}
	caps, err := a.collectionWorkerReady(r.Context())
	if err != nil {
		collectionFail(w, err)
		return
	}
	available := false
	for _, capability := range caps.Capabilities {
		if capability.Kind == "uav" && capability.Available {
			available = true
		}
	}
	if !available {
		fail(w, 503, "执行端视频插件未启用或固定依赖不可用，请检查插件配置")
		return
	}
	var output collectioncontract.UAVSearchResponse
	if _, err = a.collectionWorkerCall(r.Context(), "POST", "/v1/uav/search", input, &output); err != nil {
		collectionFail(w, err)
		return
	}
	if err = collectioncontract.ValidateUAVSearchResponse(input, output); err != nil {
		fail(w, 502, "视频插件搜索响应格式无效或超出安全限制")
		return
	}
	if output.Items == nil {
		output.Items = []collectioncontract.UAVSearchItem{}
	}
	// 网络请求结束后重新核对授权，编辑或停用来源会使本次结果失效。
	current, err := a.collectionSourceByID(r.Context(), source.ID)
	if err != nil || !current.Enabled || current.Revision != source.Revision {
		fail(w, 409, "视频来源已变化或停用，请重新搜索")
		return
	}
	a.logger.Info("视频来源搜索已完成", "sourceId", source.ID, "site", input.Site, "page", input.Page, "count", len(output.Items))
	respond(w, 200, output)
}
