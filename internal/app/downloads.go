package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var errDownloadSourceTooLarge = errors.New("种子上传超过大小限制，种子本体最多 8 MiB") // 超量与格式错误分别返回 413 和 400。

// downloadAddInput 是浏览器可提交的有限下载源与受管库授权。
type downloadAddInput struct {
	RequestID      string `json:"requestId"`                // 添加操作的客户端幂等键。
	Magnet         string `json:"magnet"`                   // 仅 JSON 接受的磁力源，不记录日志。
	SearchResultID string `json:"searchResultId,omitempty"` // 仅 JSON 接受的服务端搜种结果令牌，与磁力二选一。
	ContentType    string `json:"contentType"`              // 五类现有内容之一。
	CreateLibrary  bool   `json:"createLibrary"`            // 明确允许首次创建对应下载库。
	LibraryID      *int64 `json:"libraryId,omitempty"`      // 可选的同类型受管库。
}

// downloadRoutes 复用现有管理员会话注册下载 API。
func (a *App) downloadRoutes(mux *http.ServeMux) {
	for pattern, handler := range map[string]http.HandlerFunc{
		"GET /api/downloads/status": a.downloadStatus, "POST /api/downloads/connection-test": a.downloadConnectionTest,
		"GET /api/downloads": a.listDownloads, "POST /api/downloads": a.addDownload,
		"GET /api/downloads/{id}": a.getDownload, "GET /api/downloads/{id}/files": a.downloadFiles,
		"PUT /api/downloads/{id}/selection": a.selectDownloadFiles, "POST /api/downloads/{id}/actions": a.downloadAction,
		"PUT /api/downloads/{id}/limits": a.downloadLimits, "POST /api/downloads/{id}/retry-import": a.retryDownloadImport,
		"GET /api/downloads/{id}/items": a.downloadItems,
	} {
		mux.HandleFunc(pattern, a.auth(handler))
	}
}

// downloadStatus 返回后端统一同步快照，禁用时也可读取。
func (a *App) downloadStatus(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, a.downloads.connectionStatus())
}

// connectionStatus 在短读锁内复制连接状态。
func (m *downloadManager) connectionStatus() DownloadStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// requireDownloads 在写入前明确拒绝禁用或不兼容连接，不返回应用登录 401。
func (a *App) requireDownloads(w http.ResponseWriter) bool {
	status := a.downloads.connectionStatus()
	if !status.Enabled {
		fail(w, 503, "下载功能尚未启用，请先配置下载器与专用下载卷")
		return false
	}
	if status.State != "ready" {
		message := status.Error
		if message == "" {
			message = "下载器正在连接，请稍后重试"
		}
		fail(w, 503, message)
		return false
	}
	return true
}

// downloadConnectionTest 只探测后端已经配置的地址，不接受浏览器提供 URL。
func (a *App) downloadConnectionTest(w http.ResponseWriter, r *http.Request) {
	if !a.downloads.connectionStatus().Enabled {
		respond(w, 200, a.downloads.connectionStatus())
		return
	}
	status, err := a.downloads.engine.probe(r.Context())
	a.downloads.setConnection(status, err)
	respond(w, 200, a.downloads.connectionStatus())
	a.downloads.notify()
}

// downloadPage 解析统一分页，防止过大偏移和无界清单输出。
func downloadPage(r *http.Request, defaultSize int, maxSize int) (int, int, error) {
	page := 1
	size := defaultSize
	var err error
	if text := r.URL.Query().Get("page"); text != "" {
		page, err = strconv.Atoi(text)
		if err != nil || page < 1 || page > 1000000 {
			return 0, 0, fmt.Errorf("下载页码无效")
		}
	}
	if text := r.URL.Query().Get("pageSize"); text != "" {
		size, err = strconv.Atoi(text)
		if err != nil || size < 1 {
			return 0, 0, fmt.Errorf("分页大小无效")
		}
		if size > maxSize {
			size = maxSize
		}
	}
	return page, size, nil
}

// listDownloads 分页查询持久任务并覆盖最新有界内存指标。
func (a *App) listDownloads(w http.ResponseWriter, r *http.Request) {
	page, size, err := downloadPage(r, 30, 100)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	conditions := []string{"archived=0"}
	args := []any{}
	if state := r.URL.Query().Get("state"); state != "" && state != "all" {
		switch state {
		case "active", "downloading":
			conditions = append(conditions, "state IN ('submitting','metadata','awaiting_selection','queued','downloading','checking','moving')")
		case "completed":
			conditions = append(conditions, "state IN ('completed','seeding')")
		case "error":
			conditions = append(conditions, "(state IN ('error','missing') OR import_state IN ('failed','target_missing'))")
		case "paused":
			conditions = append(conditions, "state='paused'")
		default:
			fail(w, 400, "下载筛选状态无效")
			return
		}
	}
	if search := strings.TrimSpace(r.URL.Query().Get("q")); search != "" {
		if len(search) > 300 {
			fail(w, 400, "搜索内容过长")
			return
		}
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+search+"%")
	}
	where := " WHERE " + strings.Join(conditions, " AND ")
	var total int64
	if a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM downloads"+where, args...).Scan(&total) != nil {
		fail(w, 500, "下载列表读取失败")
		return
	}
	args = append(args, size, (page-1)*size)
	rows, err := a.db.QueryContext(r.Context(), downloadSelect+where+" ORDER BY created_at DESC,id LIMIT ? OFFSET ?", args...)
	if err != nil {
		fail(w, 500, "下载列表读取失败")
		return
	}
	defer rows.Close()
	values := []Download{}
	for rows.Next() {
		value, scanErr := scanDownload(rows)
		if scanErr != nil {
			fail(w, 500, "下载记录无效")
			return
		}
		values = append(values, a.downloads.view(value))
	}
	if rows.Err() != nil {
		fail(w, 500, "下载列表读取中断")
		return
	}
	respond(w, 200, map[string]any{"downloads": values, "total": total, "page": page, "pageSize": size})
}

// view 仅在修订号一致时使用实时指标，断线保留上次快照与时间。
func (m *downloadManager) view(record downloadRecord) Download {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if snapshot, exists := m.snapshots[record.ID]; exists && snapshot.Revision == record.Revision {
		record.SizeBytes = snapshot.SizeBytes
		record.DownloadedBytes = snapshot.DownloadedBytes
		record.DownloadSpeed = snapshot.DownloadSpeed
		record.UploadSpeed = snapshot.UploadSpeed
		record.UploadedBytes = snapshot.UploadedBytes
		record.Ratio = snapshot.Ratio
		record.Peers = snapshot.Peers
		record.ETASeconds = snapshot.ETASeconds
		record.LastSyncedAt = snapshot.LastSyncedAt
	}
	return record.Download
}

// requestDownload 统一取得 UUID 路径任务，不接受任意路径或引擎 hash。
func (a *App) requestDownload(w http.ResponseWriter, r *http.Request) (downloadRecord, bool) {
	id := r.PathValue("id")
	if !validDownloadUUID(id) {
		fail(w, 400, "下载任务编号无效")
		return downloadRecord{}, false
	}
	d, err := a.downloadByID(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "下载任务不存在")
		return d, false
	}
	if err != nil {
		fail(w, 500, "下载任务读取失败")
		return d, false
	}
	return d, true
}

// getDownload 返回单任务状态，源文件与引擎凭据永不进入 DTO。
func (a *App) getDownload(w http.ResponseWriter, r *http.Request) {
	d, ok := a.requestDownload(w, r)
	if ok {
		respond(w, 200, map[string]any{"download": a.downloads.view(d)})
	}
}

// readDownloadFiles 在关闭 rows 后返回版本清单，避免单 SQLite 连接嵌套查询。
func (a *App) readDownloadFiles(ctx context.Context, d downloadRecord) ([]DownloadFile, error) {
	rows, err := a.db.QueryContext(ctx, "SELECT file_index,relative_path,size_bytes,selected,progress,mod_time FROM download_files WHERE download_id=? AND version=? ORDER BY file_index", d.ID, d.ManifestVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []DownloadFile{}
	for rows.Next() {
		var file DownloadFile
		if err = rows.Scan(&file.Index, &file.Path, &file.SizeBytes, &file.Selected, &file.Progress, &file.ModTime); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

// downloadFiles 从本地清单分页，不为每个弹窗增加引擎轮询。
func (a *App) downloadFiles(w http.ResponseWriter, r *http.Request) {
	d, ok := a.requestDownload(w, r)
	if !ok {
		return
	}
	page, size, err := downloadPage(r, 100, 500)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	files, err := a.readDownloadFiles(r.Context(), d)
	if err != nil {
		fail(w, 500, "下载文件清单读取失败")
		return
	}
	total := len(files)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	respond(w, 200, map[string]any{"files": files[start:end], "total": total, "page": page, "pageSize": size, "manifestVersion": d.ManifestVersion, "selectionLocked": d.SelectionLocked})
}

// readDownloadSource 只接受有限 JSON 磁力或单个 multipart 种子，不把上传写入媒体卷。
func readDownloadSource(w http.ResponseWriter, r *http.Request) (downloadAddInput, []byte, string, error) {
	var input downloadAddInput
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "multipart/form-data" {
		if !decodeJSON(w, r, &input) {
			return input, nil, "", nil
		}
		return input, []byte(input.Magnet), "magnet", nil
	}
	reader, err := r.MultipartReader()
	if err != nil {
		return input, nil, "", fmt.Errorf("种子上传格式无效")
	}
	fields := map[string]string{}
	var source []byte
	seenFile := false
	for {
		part, readErr := reader.NextPart()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return input, nil, "", downloadUploadReadError(readErr)
		}
		name := part.FormName()
		if name == "torrent" && part.FileName() != "" {
			if seenFile {
				part.Close()
				return input, nil, "", fmt.Errorf("每次只能上传一个种子")
			}
			seenFile = true
			source, err = io.ReadAll(io.LimitReader(part, maxTorrentBytes+1))
			part.Close()
			if err != nil {
				return input, nil, "", downloadUploadReadError(err)
			}
			if len(source) > maxTorrentBytes {
				return input, nil, "", errDownloadSourceTooLarge
			}
			continue
		}
		if part.FileName() != "" || name == "torrent" {
			part.Close()
			return input, nil, "", fmt.Errorf("仅允许 torrent 字段上传一个种子文件")
		}
		if _, exists := fields[name]; exists {
			part.Close()
			return input, nil, "", fmt.Errorf("上传字段重复")
		}
		value, readErr := io.ReadAll(io.LimitReader(part, 4097))
		part.Close()
		if readErr != nil {
			return input, nil, "", downloadUploadReadError(readErr)
		}
		if len(value) > 4096 {
			return input, nil, "", fmt.Errorf("上传表单字段过长")
		}
		switch name {
		case "requestId", "contentType", "createLibrary", "libraryId":
			fields[name] = string(value)
		default:
			return input, nil, "", fmt.Errorf("上传包含不支持的字段")
		}
	}
	// 消耗最终边界后的尾随数据，防止利用 multipart epilogue 绕过总请求额度。
	if _, err = io.Copy(io.Discard, r.Body); err != nil {
		return input, nil, "", downloadUploadReadError(err)
	}
	if !seenFile || len(source) == 0 {
		return input, nil, "", fmt.Errorf("请选择非空种子文件")
	}
	input.RequestID = fields["requestId"]
	input.ContentType = fields["contentType"]
	input.CreateLibrary, err = strconv.ParseBool(fields["createLibrary"])
	if err != nil {
		return input, nil, "", fmt.Errorf("创建媒体库授权必须为 true 或 false")
	}
	if value := fields["libraryId"]; value != "" {
		id, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil || id < 1 {
			return input, nil, "", fmt.Errorf("目标媒体库编号无效")
		}
		input.LibraryID = &id
	}
	return input, source, "torrent", nil
}

// downloadUploadReadError 区分总量超限与损坏的 multipart 正文，且不暴露原始错误内容。
func downloadUploadReadError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return errDownloadSourceTooLarge
	}
	return fmt.Errorf("种子上传格式损坏或传输中断")
}

// addDownload 先保存有界私密来源和命令意图，再由协调器提交引擎。
func (a *App) addDownload(w http.ResponseWriter, r *http.Request) {
	if !a.requireDownloads(w) {
		return
	}
	input, source, kind, err := readDownloadSource(w, r)
	if kind == "" && err == nil {
		return
	}
	if err != nil {
		status := 400
		if errors.Is(err, errDownloadSourceTooLarge) {
			status = 413
		}
		fail(w, status, err.Error())
		return
	}
	if !validDownloadRequestID(input.RequestID) || !validType(input.ContentType) {
		fail(w, 400, "请提供有效请求编号与内容类型")
		return
	}
	if input.SearchResultID != "" {
		if input.Magnet != "" || kind != "magnet" {
			fail(w, 400, "搜索结果与磁力链接必须二选一")
			return
		}
		if _, valid := parseTorrentSearchResultID(input.SearchResultID); !valid {
			fail(w, 400, "搜索结果编号无效，请重新搜索")
			return
		}
		if a.searchDownloadDuplicate(w, r, input) {
			return
		}
		var status int
		var message string
		source, kind, status, message = a.resolveSearchDownload(r.Context(), input.SearchResultID)
		if status != 0 {
			fail(w, status, message)
			return
		}
	}
	var identity torrentIdentity
	if kind == "torrent" {
		identity, err = parseTorrent(source)
	} else {
		if strings.TrimPrefix(a.downloads.connectionStatus().Version, "v") != "5.2.4" {
			fail(w, 503, "此 qB 版本尚未验证磁力元数据停止行为，请使用 5.2.4 或上传本地种子")
			return
		}
		identity, err = parseMagnet(string(source))
	}
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	m := a.downloads
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	// 网络解析后再次核对幂等绑定，避免并发重试把迟到结果登记成另一任务。
	if input.SearchResultID != "" && a.searchDownloadDuplicate(w, r, input) {
		return
	}
	// 来源切换与持久添加共享命令锁，尚未绑定的旧令牌不得穿过新来源快照。
	if input.SearchResultID != "" {
		if _, status, message := a.torrentSearch.findResult(input.SearchResultID); status != 0 {
			fail(w, 409, message)
			return
		}
	}

	if existing, findErr := scanDownload(a.db.QueryRowContext(r.Context(), downloadSelect+" WHERE request_id=? OR id=(SELECT download_id FROM download_operations WHERE request_id=?)", input.RequestID, input.RequestID)); findErr == nil {
		if (identity.V1 != "" && identity.V1 == existing.IdentityV1) || (identity.V2 != "" && identity.V2 == existing.IdentityV2) {
			respond(w, 202, map[string]any{"download": m.view(existing)})
			return
		}
		fail(w, 409, "该请求编号已用于另一下载，请重新提交")
		return
	} else if !errors.Is(findErr, sql.ErrNoRows) {
		fail(w, 500, "查询幂等请求失败")
		return
	}
	if existing, findErr := scanDownload(a.db.QueryRowContext(r.Context(), downloadSelect+" WHERE (identity_v1<>'' AND identity_v1=?) OR (identity_v2<>'' AND identity_v2=?)", identity.V1, identity.V2)); findErr == nil {
		if existing.Archived {
			fail(w, 409, "该内容已有保留文件的归档记录，请使用原内容或在下载器中处理")
			return
		}
		if err = a.bindDuplicateSearchDownload(r.Context(), input, existing); err != nil {
			fail(w, 409, err.Error())
			return
		}
		respond(w, 202, map[string]any{"download": m.view(existing)})
		return
	} else if !errors.Is(findErr, sql.ErrNoRows) {
		fail(w, 500, "查询内容身份失败")
		return
	}
	library, bound, err := a.resolveDownloadLibrary(r.Context(), input)
	if err != nil {
		fail(w, 400, err.Error())
		return
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
		fail(w, 409, "下载器已有相同内容的外部任务，栖库不会接管或修改它")
		return
	}
	var count int
	if a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM downloads WHERE archived=0").Scan(&count) != nil || count >= maxDownloadTasks {
		fail(w, 409, "活跃下载已达到 256 项上限，请先归档旧任务")
		return
	}
	id, err := newDownloadUUID()
	if err != nil {
		fail(w, 500, "生成下载任务编号失败")
		return
	}
	spool := filepath.Join(a.config.DataDir, "download-sources")
	if os.MkdirAll(spool, 0700) != nil {
		fail(w, 500, "私密种子暂存目录不可写")
		return
	}
	filePath := filepath.Join(spool, id+".source")
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fail(w, 500, "保存下载源失败")
		return
	}
	_, err = file.Write(source)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(filePath)
		fail(w, 500, "保存下载源失败")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		_ = os.Remove(filePath)
		fail(w, 500, "保存下载任务失败")
		return
	}
	defer tx.Rollback()
	timestamp := now()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO downloads(id,request_id,identity_v1,identity_v2,name,content_type,target_library_id,created_at,updated_at,local_root,staging_path,final_path,source_file,source_kind,create_library,library_bound) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, input.RequestID, identity.V1, identity.V2, identity.Name, input.ContentType, library, timestamp, timestamp, a.config.Downloads.Root, filepath.Join(a.config.Downloads.Root, "staging", id), filepath.Join(a.config.Downloads.Root, "complete", input.ContentType, id), filePath, kind, input.CreateLibrary, bound)
	if err == nil {
		payload := ""
		if input.SearchResultID != "" {
			encoded, _ := json.Marshal(searchDownloadPayload{SearchResultID: input.SearchResultID})
			payload = string(encoded)
		}
		_, err = tx.ExecContext(r.Context(), "INSERT INTO download_operations(request_id,download_id,revision,kind,payload,created_at,updated_at) VALUES(?,?,1,'add',?,?,?)", input.RequestID, id, payload, timestamp, timestamp)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		_ = os.Remove(filePath)
		fail(w, 409, "下载身份或请求已存在，请刷新任务列表")
		return
	}
	d, err := a.downloadByID(r.Context(), id)
	if err != nil {
		fail(w, 500, "读取已保存下载失败，请刷新列表")
		return
	}
	a.logger.Info("下载任务已登记，等待提交引擎", "downloadId", id, "type", input.ContentType, "sourceKind", kind)
	m.notify()
	respond(w, 202, map[string]any{"download": d.Download})
}

// resolveDownloadLibrary 仅允许 complete/类型 这一受管媒体库，首次创建须有明确授权。
func (a *App) resolveDownloadLibrary(ctx context.Context, input downloadAddInput) (any, bool, error) {
	path := filepath.Join(a.config.Downloads.Root, "complete", input.ContentType)
	if input.LibraryID != nil {
		var kind, registered string
		if a.db.QueryRowContext(ctx, "SELECT type,path FROM libraries WHERE id=?", *input.LibraryID).Scan(&kind, &registered) != nil || kind != input.ContentType || filepath.Clean(registered) != path {
			return nil, false, fmt.Errorf("只能选择相同类型的受管下载媒体库")
		}
		return *input.LibraryID, true, nil
	}
	var id int64
	err := a.db.QueryRowContext(ctx, "SELECT id FROM libraries WHERE type=? AND path=?", input.ContentType, path).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("读取目标媒体库失败")
	}
	if !input.CreateLibrary {
		return nil, false, fmt.Errorf("尚无对应下载媒体库，请允许首次完成时创建")
	}
	return nil, false, nil
}

// selectDownloadFiles 固定清单版本和选集；持久化成功后才允许正式下载。
func (a *App) selectDownloadFiles(w http.ResponseWriter, r *http.Request) {
	if !a.requireDownloads(w) {
		return
	}
	var input struct {
		RequestID       string `json:"requestId"`       /* 客户端幂等键。 */
		ManifestVersion int    `json:"manifestVersion"` /* 用户查看的清单版本。 */
		Indices         []int  `json:"indices"`         /* 引擎文件 ID 列表。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validDownloadRequestID(input.RequestID) || len(input.Indices) == 0 || len(input.Indices) > maxDownloadFiles {
		fail(w, 400, "请选择 1–20000 个文件并提供有效请求编号")
		return
	}
	m := a.downloads
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	d, ok := a.requestDownload(w, r)
	if !ok {
		return
	}
	if a.returnDuplicateDownloadOperation(w, r, d, input.RequestID, "selection") {
		return
	}
	if d.Archived || d.SelectionLocked || d.ManifestVersion != input.ManifestVersion || d.ManifestVersion < 1 || d.State != "awaiting_selection" {
		fail(w, 409, "文件清单已变化或选择已锁定，请刷新详情")
		return
	}
	files, err := a.readDownloadFiles(r.Context(), d)
	if err != nil {
		fail(w, 500, "文件清单读取失败")
		return
	}
	available := map[int]DownloadFile{}
	for _, file := range files {
		available[file.Index] = file
	}
	selected := map[int]bool{}
	var total int64
	for _, index := range input.Indices {
		file, exists := available[index]
		if !exists || selected[index] {
			fail(w, 400, "选择包含无效或重复文件标识")
			return
		}
		selected[index] = true
		total += file.SizeBytes
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "保存文件选择失败")
		return
	}
	defer tx.Rollback()
	for _, file := range files {
		if selected[file.Index] {
			if _, err = tx.ExecContext(r.Context(), "UPDATE download_files SET selected=1 WHERE download_id=? AND version=? AND file_index=?", d.ID, d.ManifestVersion, file.Index); err != nil {
				break
			}
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE downloads SET selection_locked=1,size_bytes=?,state='queued',revision=revision+1,last_error='',updated_at=? WHERE id=? AND revision=?", total, now(), d.ID, d.Revision)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO download_operations(request_id,download_id,revision,kind,created_at,updated_at) VALUES(?,?,?,'selection',?,?)", input.RequestID, d.ID, d.Revision+1, now(), now())
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 409, "保存文件选择失败，请刷新后重试")
		return
	}
	a.logger.Info("下载文件选择已锁定", "downloadId", d.ID, "selectedCount", len(selected), "sizeBytes", total)
	a.respondDownloadAccepted(w, r, d.ID)
}

// returnDuplicateDownloadOperation 对相同操作幂等返回，跨任务或跨动作复用编号则冲突。
func (a *App) returnDuplicateDownloadOperation(w http.ResponseWriter, r *http.Request, d downloadRecord, request string, kind string) bool {
	var id, previousKind string
	err := a.db.QueryRowContext(r.Context(), "SELECT download_id,kind FROM download_operations WHERE request_id=?", request).Scan(&id, &previousKind)
	if err == nil {
		if id != d.ID || previousKind != kind {
			fail(w, 409, "该请求编号已用于其他操作")
		} else {
			respond(w, 202, map[string]any{"download": a.downloads.view(d)})
		}
		return true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		fail(w, 500, "查询操作记录失败")
		return true
	}
	return false
}

// downloadAction 保存显式修订号的任务意图；移除始终保留下载文件。
func (a *App) downloadAction(w http.ResponseWriter, r *http.Request) {
	if !a.requireDownloads(w) {
		return
	}
	var input struct {
		RequestID string `json:"requestId"` /* 幂等操作编号。 */
		Revision  int64  `json:"revision"`  /* 当前任务修订号。 */
		Action    string `json:"action"`    /* start/pause/reconcile/remove。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validDownloadRequestID(input.RequestID) || (input.Action != "start" && input.Action != "pause" && input.Action != "reconcile" && input.Action != "remove") {
		fail(w, 400, "下载操作无效")
		return
	}
	m := a.downloads
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	d, ok := a.requestDownload(w, r)
	if !ok {
		return
	}
	if a.returnDuplicateDownloadOperation(w, r, d, input.RequestID, input.Action) {
		return
	}
	if d.Archived || input.Revision != d.Revision {
		fail(w, 409, "任务状态已更新，请刷新后操作")
		return
	}
	if input.Action == "start" && !d.SelectionLocked {
		fail(w, 409, "请先确认文件选择后开始下载")
		return
	}
	if d.EngineHash == "" && input.Action != "reconcile" && input.Action != "remove" {
		fail(w, 409, "任务仍在确认提交结果，请稍后操作")
		return
	}
	state := d.State
	if input.Action == "pause" {
		state = "paused"
	}
	if input.Action == "start" {
		state = "queued"
	}
	if err := a.queueDownloadOperation(r.Context(), d, input.RequestID, input.Action, "", state); err != nil {
		fail(w, 409, "任务操作冲突，请刷新后重试")
		return
	}
	a.logger.Info("下载操作意图已保存", "downloadId", d.ID, "action", input.Action)
	a.respondDownloadAccepted(w, r, d.ID)
}

// queueDownloadOperation 在短事务中递增修订并保存操作，网络调用永远发生在提交之后。
func (a *App) queueDownloadOperation(ctx context.Context, d downloadRecord, request string, kind string, payload string, state string) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE downloads SET revision=revision+1,state=?,last_error='',updated_at=? WHERE id=? AND revision=?", state, now(), d.ID, d.Revision)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return fmt.Errorf("修订号冲突")
	}
	_, err = tx.ExecContext(ctx, "UPDATE download_operations SET status='superseded',updated_at=? WHERE download_id=? AND kind<>'add' AND status IN ('queued','sent','uncertain')", now(), d.ID)
	if err == nil {
		_, err = tx.ExecContext(ctx, "INSERT INTO download_operations(request_id,download_id,revision,kind,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", request, d.ID, d.Revision+1, kind, payload, now(), now())
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// downloadLimits 将 KiB/s 明确转换为字节上限，0 保留引擎全局限制。
func (a *App) downloadLimits(w http.ResponseWriter, r *http.Request) {
	if !a.requireDownloads(w) {
		return
	}
	var input struct {
		DownloadKiB int64 `json:"downloadKiB"` /* 任务下载 KiB/s，0 不附加限制。 */
		UploadKiB   int64 `json:"uploadKiB"`   /* 任务上传 KiB/s，0 不附加限制。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.DownloadKiB < 0 || input.UploadKiB < 0 || input.DownloadKiB > 1<<30 || input.UploadKiB > 1<<30 {
		fail(w, 400, "限速必须为 0–1073741824 KiB/s 的整数")
		return
	}
	m := a.downloads
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	d, ok := a.requestDownload(w, r)
	if !ok {
		return
	}
	if d.Archived || d.EngineHash == "" {
		fail(w, 409, "该任务尚不能设置限速")
		return
	}
	id, err := newDownloadUUID()
	if err != nil {
		fail(w, 500, "生成限速操作失败")
		return
	}
	payload, _ := json.Marshal(input)
	// 限速是独立意图，不能递增传输修订并意外取消尚未执行的文件选择或暂停。
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "保存限速失败")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), "UPDATE download_operations SET status='superseded',updated_at=? WHERE download_id=? AND kind='limits' AND status IN ('queued','sent','uncertain')", now(), d.ID)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO download_operations(request_id,download_id,revision,kind,payload,created_at,updated_at) VALUES(?,?,?,'limits',?,?,?)", "limits-"+id, d.ID, d.Revision, string(payload), now(), now())
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 409, "限速操作冲突，请刷新重试")
		return
	}
	m.notify()
	respond(w, 200, map[string]bool{"ok": true})
}

// retryDownloadImport 只重置发布入库失败，不重新添加或下载种子。
func (a *App) retryDownloadImport(w http.ResponseWriter, r *http.Request) {
	if !a.requireDownloads(w) {
		return
	}
	m := a.downloads
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	d, ok := a.requestDownload(w, r)
	if !ok {
		return
	}
	if d.Archived || !d.SelectionLocked || (d.ImportState != "failed" && d.ImportState != "target_missing" && d.ImportState != "no_supported_files") {
		fail(w, 409, "当前任务没有可重试的入库失败")
		return
	}
	_, err := a.db.ExecContext(r.Context(), "UPDATE downloads SET import_state='pending',scan_job_id=0,scan_attempts=0,last_error='',revision=revision+1,updated_at=? WHERE id=? AND revision=?", now(), d.ID, d.Revision)
	if err != nil {
		fail(w, 500, "安排入库重试失败")
		return
	}
	a.logger.Info("下载入库已安排重试", "downloadId", d.ID)
	a.respondDownloadAccepted(w, r, d.ID)
}

// respondDownloadAccepted 读取已提交结果后发送后台唤醒。
func (a *App) respondDownloadAccepted(w http.ResponseWriter, r *http.Request, id string) {
	d, err := a.downloadByID(r.Context(), id)
	if err != nil {
		fail(w, 500, "任务已保存，请刷新列表确认")
		return
	}
	a.downloads.notify()
	respond(w, 202, map[string]any{"download": a.downloads.view(d)})
}

// downloadItems 返回最终目录下已登记内容，保持现有个人状态 DTO。
func (a *App) downloadItems(w http.ResponseWriter, r *http.Request) {
	d, ok := a.requestDownload(w, r)
	if !ok {
		return
	}
	page, size, err := downloadPage(r, 48, 96)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	items := []Item{}
	var total int64
	if d.LibraryID == nil || !d.Ready {
		respond(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
		return
	}
	prefix := d.ID + "/"
	where := " WHERE i.library_id=? AND i.missing=0 AND (i.relative_path=? OR substr(i.relative_path,1,?)=?)"
	if a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM items i"+where, *d.LibraryID, d.ID, len(prefix), prefix).Scan(&total) != nil {
		fail(w, 500, "入库内容计数失败")
		return
	}
	user := currentUser(r).ID
	rows, err := a.db.QueryContext(r.Context(), itemSelect+where+" ORDER BY i.id LIMIT ? OFFSET ?", user, user, *d.LibraryID, d.ID, len(prefix), prefix, size, (page-1)*size)
	if err != nil {
		fail(w, 500, "入库内容读取失败")
		return
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			fail(w, 500, "入库记录无效")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		fail(w, 500, "入库内容读取中断")
		return
	}
	respond(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

// newDownloadUUID 产生不可预测的每任务隔离目录编号。
func newDownloadUUID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	data[6] = (data[6] & 15) | 64
	data[8] = (data[8] & 63) | 128
	value := hex.EncodeToString(data)
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:], nil
}

// validDownloadUUID 只允许内部生成格式，阻止路径内容进入文件操作。
func validDownloadUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}

// validDownloadRequestID 限制可持久化幂等键，不允许日志控制字符。
func validDownloadRequestID(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
