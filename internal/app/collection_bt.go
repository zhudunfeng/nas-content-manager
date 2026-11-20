package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// queueDownloadSource 让手工入口和收集模块共用同一 BT 身份、归属、幂等与持久添加服务。
func (a *App) queueDownloadSource(ctx context.Context, input downloadAddInput, source []byte, kind string) (downloadRecord, error) {
	if a.downloads == nil || !a.downloads.connectionStatus().Enabled || a.downloads.connectionStatus().State != "ready" {
		return downloadRecord{}, collectionProblem(503, "下载引擎尚未连接，请先配置下载中心")
	}
	var identity torrentIdentity
	var err error
	if kind == "torrent" {
		identity, err = parseTorrent(source)
	} else {
		if strings.TrimPrefix(a.downloads.connectionStatus().Version, "v") != "5.2.4" {
			return downloadRecord{}, collectionProblem(503, "此 qB 版本尚未验证磁力元数据停止行为，请使用 5.2.4 或本地种子")
		}
		identity, err = parseMagnet(string(source))
	}
	if err != nil {
		return downloadRecord{}, collectionProblem(400, err.Error())
	}
	m := a.downloads
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	if input.SearchResultID != "" {
		if _, status, message := a.torrentSearch.findResult(input.SearchResultID); status != 0 {
			return downloadRecord{}, collectionProblem(409, message)
		}
	}
	if existing, e := scanDownload(a.db.QueryRowContext(ctx, downloadSelect+" WHERE request_id=? OR id=(SELECT download_id FROM download_operations WHERE request_id=?)", input.RequestID, input.RequestID)); e == nil {
		if identity.V1 != "" && identity.V1 == existing.IdentityV1 || identity.V2 != "" && identity.V2 == existing.IdentityV2 {
			return existing, nil
		}
		return existing, collectionProblem(409, "该请求编号已用于另一下载，请重新提交")
	} else if !errors.Is(e, sql.ErrNoRows) {
		return downloadRecord{}, e
	}
	if existing, e := scanDownload(a.db.QueryRowContext(ctx, downloadSelect+" WHERE (identity_v1<>'' AND identity_v1=?) OR (identity_v2<>'' AND identity_v2=?)", identity.V1, identity.V2)); e == nil {
		if existing.Archived {
			return existing, collectionProblem(409, "该内容已有保留文件的归档记录，请使用原内容")
		}
		if err = a.bindDuplicateSearchDownload(ctx, input, existing); err != nil {
			return existing, collectionProblem(409, err.Error())
		}
		return existing, nil
	} else if !errors.Is(e, sql.ErrNoRows) {
		return downloadRecord{}, e
	}
	library, bound, err := a.resolveDownloadLibrary(ctx, input)
	if err != nil {
		return downloadRecord{}, collectionProblem(400, err.Error())
	}
	probe := downloadRecord{IdentityV1: identity.V1, IdentityV2: identity.V2}
	m.mu.RLock()
	conflict := false
	for _, torrent := range m.engineSnapshot {
		if identityMatches(probe, torrent) {
			conflict = true
			break
		}
	}
	m.mu.RUnlock()
	if conflict {
		return downloadRecord{}, collectionProblem(409, "下载器已有相同内容的外部任务，栖库不会接管它")
	}
	var count int
	if a.db.QueryRowContext(ctx, "SELECT count(*) FROM downloads WHERE archived=0").Scan(&count) != nil || count >= maxDownloadTasks {
		return downloadRecord{}, collectionProblem(409, "活跃下载已达到 256 项上限")
	}
	id, err := newDownloadUUID()
	if err != nil {
		return downloadRecord{}, err
	}
	spool := filepath.Join(a.config.DataDir, "download-sources")
	if os.MkdirAll(spool, 0700) != nil {
		return downloadRecord{}, collectionProblem(500, "私密种子目录不可写")
	}
	sourceFile := filepath.Join(spool, id+".source")
	f, err := os.OpenFile(sourceFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return downloadRecord{}, collectionProblem(500, "保存下载源失败")
	}
	_, err = f.Write(source)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(sourceFile)
		return downloadRecord{}, collectionProblem(500, "保存下载源失败")
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		_ = os.Remove(sourceFile)
		return downloadRecord{}, err
	}
	defer tx.Rollback()
	timestamp := now()
	_, err = tx.ExecContext(ctx, `INSERT INTO downloads(id,request_id,identity_v1,identity_v2,name,content_type,target_library_id,created_at,updated_at,local_root,staging_path,final_path,source_file,source_kind,create_library,library_bound) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, input.RequestID, identity.V1, identity.V2, identity.Name, input.ContentType, library, timestamp, timestamp, a.config.Downloads.Root, filepath.Join(a.config.Downloads.Root, "staging", id), filepath.Join(a.config.Downloads.Root, "complete", input.ContentType, id), sourceFile, kind, input.CreateLibrary, bound)
	if err == nil {
		payload := ""
		if input.SearchResultID != "" {
			payload = collectionJSON(searchDownloadPayload{SearchResultID: input.SearchResultID})
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO download_operations(request_id,download_id,revision,kind,payload,created_at,updated_at) VALUES(?,?,1,'add',?,?,?)", input.RequestID, id, payload, timestamp, timestamp)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		_ = os.Remove(sourceFile)
		return downloadRecord{}, collectionProblem(409, "下载身份或请求已存在，请刷新任务列表")
	}
	d, err := a.downloadByID(ctx, id)
	if err == nil {
		a.logger.Info("下载任务已登记，等待提交引擎", "downloadId", id, "type", input.ContentType, "sourceKind", kind)
		m.notify()
	}
	return d, err
}

// collectionAcquireCandidate 持久化 BT 候选确认，长时间网络获取交给协调器。
func (a *App) collectionAcquireCandidate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RequestID     string `json:"requestId"`     /* 幂等请求编号。 */
		Revision      int64  `json:"revision"`      /* 候选版本。 */
		ContentType   string `json:"contentType"`   /* 目标媒体类型。 */
		CreateLibrary bool   `json:"createLibrary"` /* 首次建库授权。 */
		LibraryID     int64  `json:"libraryId"`     /* 目标受管库。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validDownloadRequestID(input.RequestID) || !validType(input.ContentType) || input.LibraryID < 0 {
		fail(w, 400, "候选获取参数无效")
		return
	}
	c, err := a.collectionCandidateByID(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "候选不存在")
		return
	}
	if c.TaskID != "" {
		t, e := a.collectionTaskByID(r.Context(), c.TaskID)
		if e != nil {
			collectionFail(w, e)
			return
		}
		if !a.collectionCandidateIntent(r.Context(), t, input.RequestID, input.ContentType, input.LibraryID, input.CreateLibrary) {
			fail(w, 409, "该候选已用于不同收集意图，请查看原任务")
			return
		}
		respond(w, 202, map[string]any{"task": t.CollectionTask})
		return
	}
	if c.Revision != input.Revision || c.Decision == "ignored" {
		fail(w, 409, "候选版本已更新或已经忽略")
		return
	}
	t, err := a.queueCollectionBT(r.Context(), c, input.RequestID, input.ContentType, input.CreateLibrary, input.LibraryID, "")
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 202, map[string]any{"task": t.CollectionTask})
}

// queueCollectionBT 将候选绑定唯一任务，BT 网络提交仍使用原持久添加服务。
func (a *App) queueCollectionBT(ctx context.Context, c CollectionCandidate, request string, contentType string, createLibrary bool, libraryID int64, selection string, subscriptions ...CollectionSubscription) (collectionTaskRecord, error) {
	s, err := a.collectionSourceByID(ctx, c.SourceID)
	if err != nil || !s.Enabled {
		return collectionTaskRecord{}, collectionProblem(409, "候选来源已停用")
	}
	private, err := a.readCollectionPrivate(c.ID)
	if err != nil {
		return collectionTaskRecord{}, err
	}
	if private.SourceRevision != s.Revision {
		return collectionTaskRecord{}, collectionProblem(409, "候选来源配置已更新，请重新收取该来源后确认")
	}
	if !createLibrary && libraryID == 0 {
		return collectionTaskRecord{}, collectionProblem(400, "请选择已有下载媒体库或允许首次创建")
	}
	id, err := newDownloadUUID()
	if err != nil {
		return collectionTaskRecord{}, err
	}
	a.collections.mu.Lock()
	defer a.collections.mu.Unlock()
	currentSource, err := a.collectionSourceByID(ctx, s.ID)
	if err != nil || !currentSource.Enabled || currentSource.Revision != s.Revision {
		return collectionTaskRecord{}, collectionProblem(409, "来源已停用或更新，请重新收取后确认")
	}
	if len(subscriptions) > 0 {
		var body string
		var currentSubscription CollectionSubscription
		expected := subscriptions[0]
		if a.db.QueryRowContext(ctx, "SELECT body FROM collection_subscriptions WHERE id=?", expected.ID).Scan(&body) != nil || json.Unmarshal([]byte(body), &currentSubscription) != nil || !currentSubscription.Enabled || currentSubscription.Mode != "automatic" || currentSubscription.Revision != expected.Revision {
			return collectionTaskRecord{}, collectionProblem(409, "订阅规则已更新或停用，本轮旧规则不再派发")
		}
	}
	current, err := a.collectionCandidateByID(ctx, c.ID)
	if err != nil {
		return collectionTaskRecord{}, err
	}
	if current.TaskID != "" {
		existing, e := a.collectionTaskByID(ctx, current.TaskID)
		if e != nil {
			return existing, e
		}
		if !a.collectionCandidateIntent(ctx, existing, request, contentType, libraryID, createLibrary) {
			return existing, collectionProblem(409, "该候选已经用于不同收集意图")
		}
		return existing, nil
	}
	if current.Revision != c.Revision || current.Decision == "ignored" {
		return collectionTaskRecord{}, collectionProblem(409, "候选状态已更新，请刷新后确认")
	}
	t, err := a.insertCollectionTask(ctx, collectionTaskRecord{CollectionTask: CollectionTask{ID: id, RequestID: request, SourceID: s.ID, CandidateID: c.ID, Kind: "bt", Title: c.Title, ContentType: contentType, LibraryID: libraryID}, Plan: collectionJSON(collectionPlan{PrivateID: c.ID, SelectionPattern: selection, TargetLibraryID: libraryID}), Fingerprint: collectionDigest("bt-candidate\x00" + c.SourceID + "\x00" + private.Identity + "\x00" + contentType), CreateLibrary: createLibrary})
	if err != nil {
		return t, err
	}
	latest, e := a.collectionCandidateByID(ctx, c.ID)
	if e != nil {
		return t, e
	}
	if latest.TaskID == "" {
		latest.TaskID = t.ID
		latest.Decision = "approved"
		latest.Reason = "已确认获取，BT 文件选择由下载中心完成"
		latest.Revision++
		_, err = a.db.ExecContext(ctx, "UPDATE collection_candidates SET body=? WHERE id=?", collectionJSON(latest), latest.ID)
	}
	return t, err
}

// collectionCandidateIntent 校验候选重复确认的不可变目标与请求归属。
func (a *App) collectionCandidateIntent(ctx context.Context, t collectionTaskRecord, request string, contentType string, library int64, create bool) bool {
	var plan collectionPlan
	if json.Unmarshal([]byte(t.Plan), &plan) != nil || t.RequestID != request || t.ContentType != contentType || plan.TargetLibraryID != library || t.CreateLibrary != create {
		return false
	}
	return true
}

// collectionCandidateTorrent 仅从同来源受控端点获取有界种子，磁力不经过网络转发。
func (a *App) collectionCandidateTorrent(ctx context.Context, t collectionTaskRecord, private collectionPrivate) ([]byte, string, error) {
	if strings.HasPrefix(private.URL, "magnet:") {
		return []byte(private.URL), "magnet", nil
	}
	s, err := a.collectionSourceByID(ctx, t.SourceID)
	if err != nil {
		return nil, "", err
	}
	sourceSecret, err := a.readCollectionPrivate(s.ID)
	if err != nil {
		return nil, "", err
	}
	origin, err := collectionParseURL(sourceSecret.URL)
	if err != nil {
		return nil, "", err
	}
	target, err := collectionParseURL(private.URL)
	if err != nil || target.Scheme != origin.Scheme || !strings.EqualFold(target.Host, origin.Host) {
		return nil, "", collectionProblem(400, "种子定位不属于来源授权域")
	}
	client := collectionRSSClient(origin, s.AllowPrivate)
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
	if err != nil {
		return nil, "", err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, "", collectionProblem(502, "获取候选种子失败，请检查 RSS 来源")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, "", collectionProblem(502, "RSS 来源拒绝种子获取")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxTorrentBytes+1))
	if err != nil || len(data) > maxTorrentBytes {
		return nil, "", collectionProblem(413, "候选种子超过 8 MiB 或不可读取")
	}
	return data, "torrent", nil
}

// runCollectionBT 始终读取原下载状态，不用第二套传输状态机覆盖 qB 事实。
func (a *App) runCollectionBT(ctx context.Context, t collectionTaskRecord) {
	previous := collectionJSON(t.CollectionTask)
	t.ItemIDs = []int64{}
	if t.DownloadID == "" {
		var plan collectionPlan
		if json.Unmarshal([]byte(t.Plan), &plan) != nil {
			return
		}
		private, err := a.readCollectionPrivate(plan.PrivateID)
		if err != nil {
			_ = a.collectionSetState(ctx, &t, "failed", "pending", "RSS 候选私密引用不可用")
			return
		}
		source, kind, err := a.collectionCandidateTorrent(ctx, t, private)
		if err != nil {
			_ = a.collectionSetState(ctx, &t, "failed", "pending", err.Error())
			return
		}
		input := downloadAddInput{RequestID: t.ID, ContentType: t.ContentType, CreateLibrary: t.CreateLibrary}
		if t.LibraryID > 0 {
			input.LibraryID = &t.LibraryID
		}
		d, err := a.queueDownloadSource(ctx, input, source, kind)
		if err != nil {
			message := "下载引擎未能接受候选，请检查下载中心后重试"
			var e collectionFailure
			if errors.As(err, &e) {
				message = e.Message
			}
			_ = a.collectionSetState(ctx, &t, "failed", "pending", message)
			return
		}
		t.DownloadID = d.ID
		if a.persistCollectionTask(ctx, &t) != nil {
			return
		}
		a.collectionEvent(ctx, t.ID, "dispatching", "候选已绑定受管 BT 下载")
	}
	d, err := a.downloadByID(ctx, t.DownloadID)
	if err != nil {
		_ = a.collectionSetState(ctx, &t, "reconciliation_required", "failed", "关联下载记录不可用，请检查下载中心")
		return
	}
	var plan collectionPlan
	_ = json.Unmarshal([]byte(t.Plan), &plan)
	if d.State == "awaiting_selection" && plan.SelectionPattern != "" {
		if err = a.selectCollectionBT(ctx, d, plan.SelectionPattern); err != nil {
			plan.SelectionPattern = ""
			t.Plan = collectionJSON(plan)
			t.Message = "自动选择存在歧义，请前往下载中心确认真实文件清单"
			_ = a.persistCollectionTask(ctx, &t)
		}
		d, _ = a.downloadByID(ctx, t.DownloadID)
	}
	state := d.State
	importState := d.ImportState
	message := "BT 任务由下载中心管理"
	if state == "awaiting_selection" {
		message = "等待确认真实文件清单，请打开下载中心"
	}
	if d.Ready {
		state = "importing"
		message = "BT 文件已发布，等待原下载入库确认"
	}
	if d.ImportState == "failed" || d.ImportState == "target_missing" {
		message = "BT 已获取但入库未完成，请在下载中心单独重试入库"
	}
	if d.LibraryID != nil {
		t.LibraryID = *d.LibraryID
	}
	t.Bytes = d.DownloadedBytes
	t.JobID = d.ScanJobID
	if d.ImportState == "imported" && d.LibraryID != nil {
		files, e := a.readDownloadFiles(ctx, d)
		if e == nil && d.Ready && a.downloads.verifyPublished(d, files) == nil {
			targets := downloadImportTargets(d, files)
			ids, covered, _ := a.collectionCovered(ctx, *d.LibraryID, targets)
			if covered {
				t.ItemIDs = ids
				state = "completed"
				message = "BT 获取与媒体入库均已确认"
				if d.Archived {
					message = "BT 引擎任务已移除，已发布媒体与入库结果保留"
				}
			}
		}
	}
	if d.ImportState == "imported" && len(t.ItemIDs) == 0 {
		state = "importing"
		importState = "failed"
		message = "BT 原入库覆盖或发布版本已失效，请在下载中心检查并重试入库"
	}
	if t.State != state || t.ImportState != importState || t.Message != message {
		_ = a.collectionSetState(ctx, &t, state, importState, message)
	} else if previous != collectionJSON(t.CollectionTask) {
		_ = a.persistCollectionTask(ctx, &t)
	}
}

// selectCollectionBT 仅在真实清单版本内选择唯一正则匹配媒体，歧义保留人工确认。
func (a *App) selectCollectionBT(ctx context.Context, d downloadRecord, pattern string) error {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	m := a.downloads
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	latest, err := a.downloadByID(ctx, d.ID)
	if err != nil {
		return err
	}
	if latest.Revision != d.Revision || latest.ManifestVersion != d.ManifestVersion || d.State != "awaiting_selection" || d.SelectionLocked || d.ManifestVersion < 1 {
		return fmt.Errorf("文件清单或修订已更新")
	}
	files, err := a.readDownloadFiles(ctx, d)
	if err != nil {
		return err
	}
	matches := []DownloadFile{}
	for _, file := range files {
		if matchesType(d.ContentType, file.Path) && expression.MatchString(file.Path) {
			matches = append(matches, file)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf("文件匹配并不唯一")
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE downloads SET selection_locked=1,size_bytes=?,state='queued',revision=revision+1,last_error='',updated_at=? WHERE id=? AND revision=? AND manifest_version=? AND selection_locked=0 AND state='awaiting_selection'", matches[0].SizeBytes, now(), d.ID, d.Revision, d.ManifestVersion)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("文件选择发生冲突")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE download_files SET selected=CASE WHEN file_index=? THEN 1 ELSE 0 END WHERE download_id=? AND version=?", matches[0].Index, d.ID, d.ManifestVersion); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO download_operations(request_id,download_id,revision,kind,created_at,updated_at) VALUES(?,?,?,'selection',?,?)", "collection-selection-"+d.ID, d.ID, d.Revision+1, now(), now())
	if err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		m.notify()
		a.logger.Info("订阅已按真实文件清单锁定唯一选集", "downloadId", d.ID, "manifestVersion", d.ManifestVersion, "fileIndex", matches[0].Index)
	}
	return err
}
