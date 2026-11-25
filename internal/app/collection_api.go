package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"nas-content-manager/internal/collectioncontract"
)

// collectionRoutes 注册会话认证和同源检查之内的固定收集接口。
func (a *App) collectionRoutes(mux *http.ServeMux) {
	// 所有新增端点必须复用现有管理员会话鉴权。
	register := func(pattern string, handler http.HandlerFunc) { mux.HandleFunc(pattern, a.auth(handler)) }
	register("GET /api/collection/capabilities", a.collectionCapabilities)
	register("GET /api/collection/sources", a.collectionListSources)
	register("POST /api/collection/sources", a.collectionSaveSource)
	register("PATCH /api/collection/sources/{id}", a.collectionSaveSource)
	register("POST /api/collection/sources/{id}/test", a.collectionTestSource)
	register("POST /api/collection/sources/{id}/refresh", a.collectionRefreshSource)
	register("POST /api/collection/sources/{id}/jmcomic/search", a.collectionJMComicSearch)
	register("POST /api/collection/sources/{id}/uav/search", a.collectionUAVSearch)
	register("GET /api/collection/subscriptions", a.collectionListSubscriptions)
	register("POST /api/collection/subscriptions", a.collectionSaveSubscription)
	register("PATCH /api/collection/subscriptions/{id}", a.collectionSaveSubscription)
	register("POST /api/collection/subscriptions/{id}/preview", a.collectionPreviewSubscription)
	register("POST /api/collection/subscriptions/{id}/run", a.collectionRunSubscription)
	register("GET /api/collection/candidates", a.collectionListCandidates)
	register("POST /api/collection/candidates/{id}/ignore", a.collectionIgnoreCandidate)
	register("POST /api/collection/candidates/{id}/acquire", a.collectionAcquireCandidate)
	register("POST /api/collection/previews", a.collectionCreatePreview)
	register("POST /api/collection/tasks", a.collectionCreateTask)
	register("GET /api/collection/tasks", a.collectionListTasks)
	register("GET /api/collection/tasks/{id}", a.collectionGetTask)
	register("GET /api/collection/tasks/{id}/events", a.collectionTaskEvents)
	register("POST /api/collection/tasks/{id}/actions", a.collectionTaskAction)
	a.collectionExternalRoutes(register)
	a.collectionComponentRoutes(register)
}

// collectionFail 只公开经过审查的业务错误。
func collectionFail(w http.ResponseWriter, err error) {
	var e collectionFailure
	if errors.As(err, &e) {
		fail(w, e.Status, e.Message)
	} else {
		fail(w, 500, "资源收集操作失败，请稍后重试")
	}
}

// collectionCapabilities 返回实际平台能力和明确配置的授权目录。
func (a *App) collectionCapabilities(w http.ResponseWriter, r *http.Request) {
	type rootView struct {
		ID   string `json:"id"`   // 授权根的公开编号。
		Name string `json:"name"` // 目录展示名。
		Path string `json:"path"` // 管理员配置的根路径。
	}
	roots := []rootView{}
	for i, p := range a.config.MediaRoots {
		roots = append(roots, rootView{ID: fmt.Sprintf("media-%d", i), Name: filepath.Base(p), Path: p})
	}
	caps := []collectioncontract.Capability{{Kind: "rss", Available: true, Reason: "支持有界 RSS 和候选确认"}, {Kind: "bt", Available: a.downloads != nil && a.downloads.connectionStatus().Enabled && a.downloads.connectionStatus().State == "ready", Reason: "BT 获取与选文件由下载中心处理；引擎须处于已连接状态"}, {Kind: "register", Available: true, Reason: "授权本地目录只读登记并扫描"}, {Kind: "external", Available: true, Reason: "登记外部应用和人工确认的成品目录"}}
	worker, err := a.collectionWorkerReady(r.Context())
	reason := ""
	sources := []collectioncontract.Source{}
	if err != nil {
		reason = err.Error()
		for _, kind := range []string{"web", "local", "remote", "jmcomic", "uav"} {
			caps = append(caps, collectioncontract.Capability{Kind: kind, Reason: reason})
		}
	} else {
		caps = append(caps, worker.Capabilities...)
		sources = append(sources, worker.Sources...)
	}
	respond(w, 200, map[string]any{"capabilities": caps, "workerSources": sources, "roots": roots, "workerReady": err == nil, "workerReason": reason})
}

// collectionListSources 返回不含私密 URL 的来源快照。
func (a *App) collectionListSources(w http.ResponseWriter, r *http.Request) {
	out, err := a.collectionSources(r.Context())
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"sources": out})
}

// collectionSaveSource 验证完整配置并通过修订号保存来源，停用不删除历史。
func (a *App) collectionSaveSource(w http.ResponseWriter, r *http.Request) {
	var input collectionSourceInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) == 0 || len(input.Name) > 120 || strings.ContainsAny(input.Name, "\r\n\x00") || !(input.Kind == "rss" || input.Kind == "web" || input.Kind == "local" || input.Kind == "remote" || input.Kind == "external" || input.Kind == "jmcomic" || input.Kind == "uav") {
		fail(w, 400, "请填写有效来源名称和类型")
		return
	}
	if input.Kind == "jmcomic" || input.Kind == "uav" {
		if input.RootID != "" || input.Path != "" || input.URL != "" || input.AllowPrivate || input.ExternalURL != "" || input.ExternalService != "" || input.Connection != nil || input.ClearConnection {
			fail(w, 400, "固定插件来源不接受目录、链接或外部连接参数")
			return
		}
		input.OutputProfile = ""
	}
	path, err := collectionRelative(input.Path)
	if err != nil {
		collectionFail(w, err)
		return
	}
	input.Path = path
	if input.RefreshMinutes == 0 {
		input.RefreshMinutes = 30
	}
	if input.RefreshMinutes < 1 || input.RefreshMinutes > 10080 || (input.OutputProfile != "" && input.OutputProfile != "video" && input.OutputProfile != "audio") || len(input.RootID) > 120 {
		fail(w, 400, "来源刷新间隔、输出规格或根编号无效")
		return
	}
	if input.ExternalURL != "" {
		u, e := url.Parse(input.ExternalURL)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
			fail(w, 400, "外部应用入口必须为无凭据和查询参数的 HTTP(S) 地址")
			return
		}
	}
	if input.Kind == "rss" && input.URL != "" {
		if _, err = collectionParseURL(input.URL); err != nil {
			collectionFail(w, err)
			return
		}
	}
	if (input.Kind == "local" || input.Kind == "remote") && input.RootID == "" {
		fail(w, 400, "请选择已配置的输入根目录")
		return
	}
	a.collections.mu.Lock()
	defer a.collections.mu.Unlock()
	id := r.PathValue("id")
	status := 200
	var old CollectionSource
	if id == "" {
		all, e := a.collectionSources(r.Context())
		if e != nil {
			collectionFail(w, e)
			return
		}
		if len(all) >= 64 {
			fail(w, 409, "最多可配置 64 个收集来源")
			return
		}
		id, err = newDownloadUUID()
		status = 201
		old = CollectionSource{ID: id, Revision: 0, CreatedAt: now()}
	} else {
		if a.collections.externalBusy[id] {
			fail(w, 409, "此来源正在执行操作，请等待结果后再编辑")
			return
		}
		old, err = a.collectionSourceByID(r.Context(), id)
		if err != nil {
			fail(w, 404, "收集来源不存在")
			return
		}
		if input.Revision != old.Revision || input.Kind != old.Kind {
			fail(w, 409, "来源版本已变化或类型不可修改，请刷新")
			return
		}
	}
	if err != nil {
		collectionFail(w, err)
		return
	}
	// 普通编辑缺省服务字段时保留旧连接，移除必须有显式授权。
	if input.Kind == "external" && input.ExternalService == "" && old.ExternalService != "" && !input.ClearConnection {
		input.ExternalService = old.ExternalService
	}
	externalRef, err := a.prepareCollectionExternal(r.Context(), old, input)
	if err != nil {
		collectionFail(w, err)
		return
	}
	if input.Kind == "external" && input.RootID == "" && externalRef == "" {
		fail(w, 400, "请配置 API 连接或选择已授权的完成目录")
		return
	}
	s := CollectionSource{ID: id, Kind: input.Kind, Name: input.Name, Enabled: input.Enabled, Revision: old.Revision + 1, HasSecret: old.HasSecret, RootID: input.RootID, Path: input.Path, AllowPrivate: input.AllowPrivate, RefreshMinutes: input.RefreshMinutes, OutputProfile: input.OutputProfile, ExternalURL: input.ExternalURL, CreatedAt: old.CreatedAt, UpdatedAt: now(), BaselineAt: old.BaselineAt, LastSuccessAt: old.LastSuccessAt, LastError: old.LastError}
	s.ExternalService, s.APIConfigured = input.ExternalService, externalRef != ""
	if input.URL != "" {
		if err = a.writeCollectionPrivate(id, collectionPrivate{URL: input.URL}); err != nil {
			collectionFail(w, err)
			return
		}
		s.HasSecret = true
		s.BaselineAt = ""
	} else if s.Kind == "rss" && s.HasSecret {
		private, privateErr := a.readCollectionPrivate(id)
		if privateErr != nil {
			collectionFail(w, privateErr)
			return
		}
		// 配置版本变化后强制新快照，防止 304 留下不可使用的旧候选授权。
		private.ETag = ""
		private.LastModified = ""
		if err = a.writeCollectionPrivate(id, private); err != nil {
			collectionFail(w, err)
			return
		}
	}
	if s.Kind == "rss" && !s.HasSecret {
		fail(w, 400, "RSS 来源必须配置地址")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer tx.Rollback()
	if status == 201 {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO collection_sources(id,body) VALUES(?,?)", id, collectionJSON(s))
	} else {
		_, err = tx.ExecContext(r.Context(), "UPDATE collection_sources SET body=? WHERE id=?", collectionJSON(s), id)
	}
	if err == nil {
		err = collectionExternalSaveRef(r.Context(), tx, s, externalRef)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		collectionFail(w, err)
		return
	}
	a.logger.Info("收集来源配置已保存", "sourceId", id, "kind", s.Kind, "enabled", s.Enabled)
	respond(w, status, map[string]any{"source": s})
}

// collectionTestSource 仅验证连接或输入根，不抓取历史候选或创建任务。
func (a *App) collectionTestSource(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	s, err := a.collectionSourceByID(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "收集来源不存在")
		return
	}
	if s.Kind == "rss" {
		_, _, err = a.collectionFetchRSS(r.Context(), s, true)
	} else if s.Kind == "external" || s.Kind == "local" && strings.HasPrefix(s.RootID, "media-") {
		_, err = a.collectionLocalDirectory(s, "")
	} else {
		var caps collectioncontract.Capabilities
		caps, err = a.collectionWorkerReady(r.Context())
		if err == nil {
			available := false
			for _, capability := range caps.Capabilities {
				if capability.Kind == s.Kind && capability.Available {
					available = true
				}
			}
			if !available {
				err = collectionProblem(503, "执行端尚未具备该来源的工具或安全能力")
			}
			if s.Kind == "local" || s.Kind == "remote" {
				found := false
				for _, source := range caps.Sources {
					if source.ID == s.RootID && source.Kind == s.Kind {
						found = true
					}
				}
				if !found {
					err = collectionProblem(400, "执行端未配置所选来源根")
				}
			}
		}
	}
	if err != nil {
		respond(w, 200, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	message := "来源连接与授权检查通过，未收取历史或创建获取任务"
	if s.Kind == "jmcomic" {
		message = "漫画插件与执行端授权检查通过；站点连接将在搜索或预览时实际验证"
	} else if s.Kind == "uav" {
		message = "视频插件与执行端授权检查通过；站点连接将在搜索或预览时实际验证"
	}
	respond(w, 200, map[string]any{"ok": true, "message": message})
}

// collectionRefreshSource 通过显式 backfill 区分首次基线和历史候选。
func (a *App) collectionRefreshSource(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Backfill bool `json:"backfill"` /* 是否明确回填历史候选。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	s, err := a.collectionSourceByID(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "收集来源不存在")
		return
	}
	n, err := a.refreshCollectionRSS(r.Context(), s, input.Backfill)
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"added": n, "message": "RSS 收取完成，首次历史默认作为基线保留"})
}

// collectionListSubscriptions 返回持久化规则。
func (a *App) collectionListSubscriptions(w http.ResponseWriter, r *http.Request) {
	out, err := a.collectionSubscriptions(r.Context())
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"subscriptions": out})
}

// collectionSaveSubscription 保存显式自动授权并检查正则与目标类型。
func (a *App) collectionSaveSubscription(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SourceID         string         `json:"sourceId"`         /* RSS 来源。 */
		Name             string         `json:"name"`             /* 规则名称。 */
		Enabled          bool           `json:"enabled"`          /* 是否调度。 */
		ContentType      string         `json:"contentType"`      /* 媒体类别。 */
		Mode             string         `json:"mode"`             /* review/automatic。 */
		Rule             CollectionRule `json:"rule"`             /* 有界匹配条件。 */
		CreateLibrary    bool           `json:"createLibrary"`    /* 首次建库授权。 */
		LibraryID        int64          `json:"libraryId"`        /* 已有受管库。 */
		SelectionPattern string         `json:"selectionPattern"` /* 唯一文件选择正则。 */
		Revision         int64          `json:"revision"`         /* 编辑版本。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 120 || !validType(input.ContentType) || (input.Mode != "review" && input.Mode != "automatic") || input.LibraryID < 0 || len(input.Rule.Include) > 512 || len(input.Rule.Exclude) > 512 || len(input.Rule.Quality) > 64 || input.Rule.MinBytes < 0 || input.Rule.MaxBytes < 0 || (input.Rule.MaxBytes > 0 && input.Rule.MaxBytes < input.Rule.MinBytes) {
		fail(w, 400, "订阅名称、媒体类型或规则范围无效")
		return
	}
	for _, pattern := range []string{input.Rule.EpisodePattern, input.SelectionPattern} {
		if len(pattern) > 256 {
			fail(w, 400, "匹配表达式最多 256 字节")
			return
		}
		if pattern != "" {
			if _, err := regexp.Compile(pattern); err != nil {
				fail(w, 400, "匹配表达式无效")
				return
			}
		}
	}
	if input.Mode == "automatic" && (input.Rule.Include == "" || input.Rule.EpisodePattern == "" || input.SelectionPattern == "") {
		fail(w, 400, "自动规则必须明确关键词、季集匹配和唯一文件选择表达式")
		return
	}
	source, err := a.collectionSourceByID(r.Context(), input.SourceID)
	if err != nil || source.Kind != "rss" {
		fail(w, 400, "订阅必须关联 RSS 来源")
		return
	}
	a.collections.mu.Lock()
	defer a.collections.mu.Unlock()
	id := r.PathValue("id")
	status := 200
	created := now()
	revision := int64(1)
	if id == "" {
		all, e := a.collectionSubscriptions(r.Context())
		if e != nil {
			collectionFail(w, e)
			return
		}
		if len(all) >= 256 {
			fail(w, 409, "最多可配置 256 个订阅")
			return
		}
		id, err = newDownloadUUID()
		status = 201
	} else {
		var body string
		err = a.db.QueryRowContext(r.Context(), "SELECT body FROM collection_subscriptions WHERE id=?", id).Scan(&body)
		var old CollectionSubscription
		if err != nil || json.Unmarshal([]byte(body), &old) != nil {
			fail(w, 404, "订阅不存在")
			return
		}
		if input.Revision != old.Revision {
			fail(w, 409, "订阅版本已变化")
			return
		}
		created = old.CreatedAt
		revision = old.Revision + 1
	}
	if err != nil {
		collectionFail(w, err)
		return
	}
	s := CollectionSubscription{ID: id, SourceID: input.SourceID, Name: input.Name, Enabled: input.Enabled, ContentType: input.ContentType, Mode: input.Mode, Rule: input.Rule, CreateLibrary: input.CreateLibrary, LibraryID: input.LibraryID, SelectionPattern: input.SelectionPattern, Revision: revision, NextRunAt: collectionFuture(source.RefreshMinutes), CreatedAt: created, UpdatedAt: now()}
	if status == 201 {
		_, err = a.db.ExecContext(r.Context(), "INSERT INTO collection_subscriptions(id,source_id,body) VALUES(?,?,?)", id, s.SourceID, collectionJSON(s))
	} else {
		_, err = a.db.ExecContext(r.Context(), "UPDATE collection_subscriptions SET source_id=?,body=? WHERE id=?", s.SourceID, collectionJSON(s), id)
	}
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, status, map[string]any{"subscription": s})
}

// collectionRequestSubscription 为规则操作读取服务器记录。
func (a *App) collectionRequestSubscription(w http.ResponseWriter, r *http.Request) (CollectionSubscription, bool) {
	var body string
	var s CollectionSubscription
	if a.db.QueryRowContext(r.Context(), "SELECT body FROM collection_subscriptions WHERE id=?", r.PathValue("id")).Scan(&body) != nil || json.Unmarshal([]byte(body), &s) != nil {
		fail(w, 404, "订阅不存在")
		return s, false
	}
	return s, true
}

// collectionPreviewSubscription 仅展示已持久化候选的规则命中，无获取副作用。
func (a *App) collectionPreviewSubscription(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	s, ok := a.collectionRequestSubscription(w, r)
	if !ok {
		return
	}
	candidates, err := a.collectionCandidates(r.Context(), s.SourceID, 100)
	if err != nil {
		collectionFail(w, err)
		return
	}
	out := []CollectionCandidate{}
	for _, c := range candidates {
		if matched, reason := collectionRuleMatch(s.Rule, c); matched {
			c.Reason = reason
			out = append(out, c)
		}
	}
	respond(w, 200, map[string]any{"candidates": out, "matched": len(out)})
}

// collectionRunSubscription 明确执行一次有界收取和规则评估。
func (a *App) collectionRunSubscription(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	s, ok := a.collectionRequestSubscription(w, r)
	if !ok {
		return
	}
	source, err := a.collectionSourceByID(r.Context(), s.SourceID)
	if err != nil {
		fail(w, 404, "来源不存在")
		return
	}
	n, err := a.refreshCollectionRSS(r.Context(), source, false)
	if err == nil {
		err = a.evaluateCollectionSubscription(r.Context(), s)
	}
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"added": n, "message": "订阅已评估；无歧义且明确授权的规则才会自动获取"})
}

// collectionPage 读取有界分页参数。
func collectionPage(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	if size < 1 || size > 100 {
		size = 30
	}
	return page, size
}

// collectionListCandidates 返回无秘密的候选分页。
func (a *App) collectionListCandidates(w http.ResponseWriter, r *http.Request) {
	page, size := collectionPage(r)
	source := r.URL.Query().Get("sourceId")
	where := ""
	args := []any{}
	if source != "" {
		where = " WHERE source_id=?"
		args = append(args, source)
	}
	var total int
	err := a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM collection_candidates"+where, args...).Scan(&total)
	if err != nil {
		collectionFail(w, err)
		return
	}
	queryArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := a.db.QueryContext(r.Context(), "SELECT body FROM collection_candidates"+where+" ORDER BY rowid DESC LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer rows.Close()
	out := []CollectionCandidate{}
	for rows.Next() {
		var body string
		var c CollectionCandidate
		if rows.Scan(&body) != nil || json.Unmarshal([]byte(body), &c) != nil {
			fail(w, 500, "候选记录读取失败")
			return
		}
		out = append(out, c)
	}
	respond(w, 200, map[string]any{"candidates": out, "total": total, "page": page, "pageSize": size})
}

// collectionIgnoreCandidate 在版本一致且尚未提交时标记忽略。
func (a *App) collectionIgnoreCandidate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Revision int64 `json:"revision"` /* 候选版本。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	a.collections.mu.Lock()
	defer a.collections.mu.Unlock()
	c, err := a.collectionCandidateByID(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "候选不存在")
		return
	}
	if c.Revision != input.Revision || c.TaskID != "" {
		fail(w, 409, "候选已更新或已经获取")
		return
	}
	c.Decision = "ignored"
	c.Reason = "用户已忽略"
	c.Revision++
	_, err = a.db.ExecContext(r.Context(), "UPDATE collection_candidates SET body=? WHERE id=?", collectionJSON(c), c.ID)
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"ok": true})
}

// collectionListTasks 返回持久化任务分页，获取与入库状态分别呈现。
func (a *App) collectionListTasks(w http.ResponseWriter, r *http.Request) {
	page, size := collectionPage(r)
	state := r.URL.Query().Get("state")
	where := ""
	args := []any{}
	if state != "" {
		where = " WHERE state=?"
		args = append(args, state)
	}
	var total int
	if err := a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM collection_tasks"+where, args...).Scan(&total); err != nil {
		collectionFail(w, err)
		return
	}
	args = append(args, size, (page-1)*size)
	rows, err := a.db.QueryContext(r.Context(), collectionSelect+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer rows.Close()
	out := []CollectionTask{}
	for rows.Next() {
		t, e := scanCollectionTask(rows)
		if e != nil {
			collectionFail(w, e)
			return
		}
		out = append(out, t.CollectionTask)
	}
	respond(w, 200, map[string]any{"tasks": out, "total": total, "page": page, "pageSize": size})
}

// collectionGetTask 返回单项安全任务详情。
func (a *App) collectionGetTask(w http.ResponseWriter, r *http.Request) {
	t, err := a.collectionTaskByID(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "收集任务不存在")
		return
	}
	respond(w, 200, map[string]any{"task": t.CollectionTask})
}

// collectionTaskEvents 返回最多一百条脱敏中文事件。
func (a *App) collectionTaskEvents(w http.ResponseWriter, r *http.Request) {
	type event struct {
		ID        int64  `json:"id"`        // 事件编号。
		TaskID    string `json:"taskId"`    // 所属任务。
		Stage     string `json:"stage"`     // 阶段。
		Message   string `json:"message"`   // 中文脱敏说明。
		CreatedAt string `json:"createdAt"` // 创建时间。
	}
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,task_id,stage,message,created_at FROM collection_task_events WHERE task_id=? ORDER BY id DESC LIMIT 100", r.PathValue("id"))
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer rows.Close()
	out := []event{}
	for rows.Next() {
		var e event
		if err = rows.Scan(&e.ID, &e.TaskID, &e.Stage, &e.Message, &e.CreatedAt); err != nil {
			collectionFail(w, err)
			return
		}
		out = append(out, e)
	}
	respond(w, 200, map[string]any{"events": out})
}
