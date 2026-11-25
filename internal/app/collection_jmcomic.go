package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"nas-content-manager/internal/collectioncontract"
)

// collectionJMComicSearch 通过获授权来源调用固定漫画插件，不保存查询或远程链接。
func (a *App) collectionJMComicSearch(w http.ResponseWriter, r *http.Request) {
	var input collectioncontract.JMComicSearchRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := collectioncontract.NormalizeJMComicSearch(&input); err != nil {
		fail(w, 400, err.Error())
		return
	}
	source, err := a.collectionSourceByID(r.Context(), r.PathValue("id"))
	if err != nil || source.Kind != "jmcomic" {
		fail(w, 404, "漫画插件来源不存在")
		return
	}
	if !source.Enabled {
		fail(w, 409, "漫画插件来源已停用")
		return
	}
	caps, err := a.collectionWorkerReady(r.Context())
	if err != nil {
		collectionFail(w, err)
		return
	}
	available := false
	for _, capability := range caps.Capabilities {
		if capability.Kind == "jmcomic" && capability.Available {
			available = true
		}
	}
	if !available {
		fail(w, 503, "执行端漫画插件未启用或固定依赖不可用，请检查插件配置")
		return
	}
	var output collectioncontract.JMComicSearchResponse
	if _, err = a.collectionWorkerCall(r.Context(), "POST", "/v1/jmcomic/search", input, &output); err != nil {
		collectionFail(w, err)
		return
	}
	if output.Page != input.Page || output.Query != input.Query || output.Total < 0 || output.Total > 100000000 || len(output.Items) > 100 {
		fail(w, 502, "漫画插件搜索响应格式无效")
		return
	}
	if output.Items == nil {
		output.Items = []collectioncontract.JMComicSearchItem{}
	}
	for i := range output.Items {
		item := &output.Items[i]
		if !collectioncontract.ValidJMComicAlbumID(item.AlbumID) || !utf8.ValidString(item.Title) || len(item.Title) > 1024 || strings.ContainsAny(item.Title, "\x00\r\n") || len(item.Tags) > 20 {
			fail(w, 502, "漫画插件返回的结果超出安全限制")
			return
		}
		if item.Tags == nil {
			item.Tags = []string{}
		}
		for _, tag := range item.Tags {
			if len(tag) > 1024 || !utf8.ValidString(tag) || strings.ContainsAny(tag, "\x00\r\n") {
				fail(w, 502, "漫画插件返回的标签无效")
				return
			}
		}
	}
	current, err := a.collectionSourceByID(r.Context(), source.ID)
	if err != nil || !current.Enabled || current.Revision != source.Revision {
		fail(w, 409, "漫画来源已变化或停用，请重新搜索")
		return
	}
	a.logger.Info("漫画来源搜索已完成", "sourceId", source.ID, "page", input.Page, "count", len(output.Items))
	respond(w, 200, output)
}

// collectionValidJMComicMetadata 在控制面再次验证目录顺序、选择绑定与预算标志。
func collectionValidJMComicMetadata(metadata *collectioncontract.JMComicMetadata, request collectioncontract.PreviewRequest) bool {
	if metadata == nil || metadata.AlbumID != request.AlbumID || len(metadata.Chapters) < 1 || len(metadata.Chapters) > 10000 || len(metadata.SelectedChapterIDs) < 1 || metadata.PageCount < 1 || metadata.PageCount > 10000 || metadata.OverBudget != (metadata.PageCount > request.MaxFiles) {
		return false
	}
	if collectioncontract.ValidateJMComicChapters(metadata.SelectedChapterIDs) != nil {
		return false
	}
	wanted := map[string]bool{}
	for _, id := range request.ChapterIDs {
		wanted[id] = true
	}
	selected := []string{}
	seen := map[string]bool{}
	pages, total := 0, 0
	for _, chapter := range metadata.Chapters {
		if !collectioncontract.ValidJMComicAlbumID(chapter.ID) || seen[chapter.ID] || chapter.PageCount < 1 || chapter.PageCount > 10000-total || !utf8.ValidString(chapter.Title) || len(chapter.Title) > 1024 || strings.ContainsAny(chapter.Title, "\x00\r\n") {
			return false
		}
		seen[chapter.ID] = true
		total += chapter.PageCount
		if len(wanted) == 0 || wanted[chapter.ID] {
			selected = append(selected, chapter.ID)
			pages += chapter.PageCount
		}
	}
	if pages != metadata.PageCount || len(selected) != len(metadata.SelectedChapterIDs) || len(wanted) > 0 && len(selected) != len(wanted) {
		return false
	}
	for i := range selected {
		if selected[i] != metadata.SelectedChapterIDs[i] {
			return false
		}
	}
	return true
}

// collectionAttachJMComicProgress 以短超时读取已绑定操作进度，执行端不可达时保留正常任务事实。
func (a *App) collectionAttachJMComicProgress(ctx context.Context, task *collectionTaskRecord) {
	if task.Kind != "jmcomic" {
		return
	}
	var plan collectionPlan
	if json.Unmarshal([]byte(task.Plan), &plan) != nil {
		return
	}
	private, err := a.readCollectionPrivate(plan.PrivateID)
	if err != nil {
		return
	}
	request := private.Request
	request.ID, request.PreviewID = task.ID, ""
	progressCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var operation collectioncontract.Operation
	if _, err = a.collectionWorkerCall(progressCtx, "GET", "/v1/operations/"+task.ID, nil, &operation); err != nil || operation.ID != task.ID || operation.Kind != task.Kind || operation.RequestHash != collectionDigest(collectionJSON(request)) {
		return
	}
	progress := operation.JMComicProgress
	if progress == nil || progress.TotalPages < 1 || progress.TotalPages > request.MaxFiles || progress.CompletedPages < 0 || progress.CompletedPages > progress.TotalPages || progress.Bytes < 0 || progress.Bytes > request.MaxBytes || progress.ReusedPages < 0 || progress.ReusedPages > progress.CompletedPages {
		return
	}
	task.JMComicProgress = progress
}
