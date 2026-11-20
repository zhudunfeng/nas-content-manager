package app

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"nas-content-manager/internal/externalapi"
)

// collectionMoviePilotRoutes 注册固定工作流，不允许浏览器指定远端路径或方法。
func (a *App) collectionMoviePilotRoutes(register func(string, http.HandlerFunc)) {
	for _, route := range []string{"search", "releases", "files", "organize/preview", "scrape/preview"} {
		operation := route
		register("POST /api/collection/sources/{id}/external/moviepilot/"+route, func(w http.ResponseWriter, r *http.Request) { a.collectionMoviePilotRead(w, r, operation) })
	}
	for _, route := range []string{"downloads", "roots", "library"} {
		operation := route
		register("GET /api/collection/sources/{id}/external/moviepilot/"+route, func(w http.ResponseWriter, r *http.Request) { a.collectionMoviePilotRead(w, r, operation) })
	}
}

// collectionMoviePilotRead 将安全的用户选择转换为固定上游只读或预览动作。
func (a *App) collectionMoviePilotRead(w http.ResponseWriter, r *http.Request, operation string) {
	var input struct {
		Revision    int64  `json:"revision"`    // 确认时来源修订。
		Query       string `json:"query"`       // 搜索标题。
		MediaType   string `json:"mediaType"`   // movie/series。
		SelectionID string `json:"selectionId"` // 媒体选择。
		RootID      string `json:"rootId"`      // 配置目录选择。
		ParentID    string `json:"parentId"`    // 当前目录选择。
		FileID      string `json:"fileId"`      // 具体文件选择。
		TargetID    string `json:"targetId"`    // 已配置的整理目标目录。
		ExternalID  string `json:"externalId"`  // 用户确认的 TMDB 身份。
	}
	if r.Method == "POST" && !decodeJSON(w, r, &input) {
		return
	}
	client, source, cleanup, err := a.collectionExternalReadClient(r)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer cleanup()
	if source.ExternalService != "moviepilot" {
		fail(w, 400, "此工作流仅支持 MoviePilot")
		return
	}
	if r.Method == "POST" && (source.Revision != input.Revision || !source.Enabled) {
		fail(w, 409, "来源已更新或停用，请刷新后操作")
		return
	}
	choices := []externalapi.MPChoice{}
	message := "选择仅用于本次工作流，15 分钟后过期；未创建下载或入库任务"
	switch operation {
	case "search":
		choices, err = client.MPMediaSearch(r.Context(), input.Query, input.MediaType)
	case "releases":
		var selected externalapi.MPChoice
		selected, err = a.collectionMoviePilotChoice(r, source, input.SelectionID, "media")
		if err == nil {
			choices, err = client.MPReleases(r.Context(), selected.Data)
		}
	case "downloads":
		choices, err = client.MPDownloads(r.Context())
		message = "下载任务由 MoviePilot 编排；外部完成不表示已在本系统入库"
	case "roots":
		choices, err = client.MPRoots(r.Context())
		message = "仅可选择 MoviePilot 已配置的本地目录；外部目录不自动取得本系统挂载授权"
	case "files":
		var root, parent externalapi.MPChoice
		root, err = a.collectionMoviePilotChoice(r, source, input.RootID, "download_root", "library_root")
		if err == nil && input.ParentID != "" {
			parent, err = a.collectionMoviePilotChoice(r, source, input.ParentID, "dir")
		}
		if err == nil {
			choices, err = client.MPFiles(r.Context(), root.Data, parent.Data)
		}
	case "organize/preview", "scrape/preview":
		var file, target, plan externalapi.MPChoice
		file, err = a.collectionMoviePilotChoice(r, source, input.FileID, "file")
		if err == nil && operation == "organize/preview" {
			target, err = a.collectionMoviePilotChoice(r, source, input.TargetID, "library_root")
		}
		if err == nil {
			plan, err = client.MPPreview(r.Context(), operation == "scrape/preview", file.Data, target.Data, input.ExternalID, input.MediaType)
		}
		if err == nil {
			choices = append(choices, plan)
		}
		if operation == "scrape/preview" {
			message = "刮削将覆盖所选媒体的元数据及图片，需要下一步明确确认"
		} else {
			message = "整理预览已完成；确认后由 MoviePilot 复制文件，仅使用已配置为不覆盖的目标目录"
		}
	case "library":
		choices, err = client.MPLibrary(r.Context())
		message = "刷新将从已配置媒体服务器同步 MoviePilot 库索引，不等于要求 Emby/Plex 重新扫描磁盘"
	default:
		fail(w, 404, "工作流不存在")
		return
	}
	if err != nil {
		if _, ok := err.(collectionFailure); ok {
			collectionFail(w, err)
		} else {
			collectionFail(w, collectionExternalError(err))
		}
		return
	}
	views, err := a.collectionMoviePilotChoices(r, source, choices)
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"choices": views, "message": message})
}

// collectionMoviePilotChoices 保存有界不透明选择，地址、Cookie 和文件路径只进入秘密文件。
func (a *App) collectionMoviePilotChoices(r *http.Request, source CollectionSource, choices []externalapi.MPChoice) ([]externalapi.MPChoice, error) {
	if len(choices) > 100 {
		return nil, collectionProblem(413, "本次结果超过 100 项，请缩小范围后重试")
	}
	a.collections.mu.Lock()
	defer a.collections.mu.Unlock()
	current, err := a.collectionSourceByID(r.Context(), source.ID)
	if err != nil || current.Revision != source.Revision {
		return nil, collectionProblem(409, "来源已更新，请重新选择")
	}
	// 过期秘密不参与任何自动操作，逐次清理以限制长期运行占用。
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,private_ref FROM collection_external_choices WHERE source_id=? AND expires_at<=? LIMIT 600", source.ID, now())
	if err != nil {
		return nil, err
	}
	expired := map[string]string{}
	for rows.Next() {
		var id, ref string
		if err = rows.Scan(&id, &ref); err != nil {
			rows.Close()
			return nil, err
		}
		expired[id] = ref
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return nil, rowErr
	}
	for id, ref := range expired {
		if _, err = a.db.ExecContext(r.Context(), "DELETE FROM collection_external_choices WHERE id=?", id); err != nil {
			return nil, err
		}
		if path, pathErr := a.collectionSecretPath(ref); pathErr == nil {
			_ = os.Remove(path)
		}
	}
	var count int
	if err = a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM collection_external_choices WHERE source_id=?", source.ID).Scan(&count); err != nil {
		return nil, err
	}
	if count+len(choices) > 600 {
		return nil, collectionProblem(429, "最近选择较多，请等待过期或稍后重试")
	}
	out := []externalapi.MPChoice{}
	for _, choice := range choices {
		if len(choice.Data) == 0 || len(choice.Data) > 512<<10 || !json.Valid(choice.Data) {
			return nil, collectionProblem(413, "单项私密选择无效或过大")
		}
		id, uuidErr := newDownloadUUID()
		if uuidErr != nil {
			return nil, uuidErr
		}
		if err = a.writeCollectionPrivate(id, collectionPrivate{ExternalData: choice.Data, SourceRevision: source.Revision}); err != nil {
			return nil, err
		}
		if _, err = a.db.ExecContext(r.Context(), "INSERT INTO collection_external_choices(id,source_id,source_revision,kind,private_ref,expires_at) VALUES(?,?,?,?,?,?)", id, source.ID, source.Revision, choice.Kind, id, time.Now().UTC().Add(15*time.Minute).Format(time.RFC3339Nano)); err != nil {
			if privatePath, pathErr := a.collectionSecretPath(id); pathErr == nil {
				_ = os.Remove(privatePath)
			}
			return nil, err
		}
		choice.ID, choice.Data = id, nil
		out = append(out, choice)
	}
	return out, nil
}

// collectionMoviePilotChoice 在原来源、修订和有效期内解析服务器选择。
func (a *App) collectionMoviePilotChoice(r *http.Request, source CollectionSource, id string, allowed ...string) (externalapi.MPChoice, error) {
	var choice externalapi.MPChoice
	if !validDownloadUUID(id) {
		return choice, collectionProblem(400, "请选择当前列表中的目标")
	}
	var kind, ref, expiry, used string
	var revision int64
	err := a.db.QueryRowContext(r.Context(), "SELECT kind,private_ref,source_revision,expires_at,used_by FROM collection_external_choices WHERE id=? AND source_id=?", id, source.ID).Scan(&kind, &ref, &revision, &expiry, &used)
	if err != nil || revision != source.Revision || expiry <= now() || used != "" {
		return choice, collectionProblem(409, "选择已过期、已使用或来源已更新，请重新选择")
	}
	valid := false
	for _, value := range allowed {
		if value == kind {
			valid = true
		}
	}
	if !valid {
		return choice, collectionProblem(400, "所选目标与此操作不匹配")
	}
	private, err := a.readCollectionPrivate(ref)
	if err != nil || private.SourceRevision != source.Revision || len(private.ExternalData) == 0 {
		return choice, collectionProblem(409, "私密选择已不可用，请重新选择")
	}
	choice.ID, choice.Kind, choice.Data = id, kind, private.ExternalData
	return choice, nil
}

// collectionMoviePilotHydrate 只允许固定操作使用对应种类的私密选择。
func (a *App) collectionMoviePilotHydrate(r *http.Request, source CollectionSource, mutation *externalapi.Mutation) error {
	if mutation.SelectionID == "" {
		return nil
	}
	kinds := map[string]string{"download": "release", "download_start": "download", "download_stop": "download", "organize": "organize_plan", "scrape": "scrape_plan", "refresh_library": "library_job"}
	kind, exists := kinds[mutation.Kind]
	if !exists || source.ExternalService != "moviepilot" {
		return collectionProblem(400, "此操作不支持工作流选择")
	}
	choice, err := a.collectionMoviePilotChoice(r, source, mutation.SelectionID, kind)
	if err == nil {
		mutation.Payload = choice.Data
	}
	return err
}
