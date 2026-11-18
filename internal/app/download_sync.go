package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var errDownloadIdentityPending = errors.New("下载器尚未提供完整内容身份，等待元数据确认") // 未确认身份继续对账，不按失败或成功处理。

// startDownloads 初始化独立协调器；禁用时只保留本地查询与发布门禁。
func (a *App) startDownloads() {
	m := &downloadManager{app: a, status: DownloadStatus{Enabled: a.config.Downloads.Enabled, State: "disabled"}, snapshots: map[string]Download{}, wake: make(chan struct{}, 1), lastPersist: map[string]time.Time{}}
	a.downloads = m
	if !a.config.Downloads.Enabled {
		return
	}
	m.activate(newQBClient(a.config.Downloads), DownloadStatus{Enabled: true, State: "connecting"})
}

// activate 仅在首次接入时发布不可变适配器，重复清单不创建第二个执行器。
func (m *downloadManager) activate(engine *qbClient, status DownloadStatus) bool {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.started {
		return false
	}
	m.mu.Lock()
	m.engine = engine
	m.status = status
	m.started = true
	m.mu.Unlock()
	m.app.wg.Add(1)
	go func() { defer m.app.wg.Done(); m.run() }()
	return true
}

// notify 合并后端命令唤醒，不为每次前端轮询创建执行器。
func (m *downloadManager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// setConnection 保存脱敏连接状态，掉线保留上次版本与同步时间。
func (m *downloadManager) setConnection(status DownloadStatus, err error) {
	m.mu.Lock()
	previous := m.status
	if err != nil {
		status = previous
		status.Enabled = true
		status.State = "offline"
		status.Error = "下载器暂不可用，正在展示上次同步状态"
		var engineErr *downloadEngineError
		if errors.As(err, &engineErr) {
			status.State = engineErr.State
			status.Error = engineErr.Message
		}
	} else {
		status.Error = ""
	}
	m.status = status
	m.mu.Unlock()
	if previous.State != status.State {
		if err != nil {
			m.app.logger.Warn("下载器连接状态变化", "state", status.State, "message", status.Error)
		} else {
			m.app.logger.Info("下载器连接已就绪", "version", status.Version, "webApiVersion", status.WebAPIVersion)
		}
	}
}

// run 以应用生命周期运行统一同步，失败退避不会阻塞扫描工作池。
func (m *downloadManager) run() {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-m.app.ctx.Done():
			m.engine.http.CloseIdleConnections()
			return
		case <-m.wake:
		case <-timer.C:
		}
		interval := m.cycle(m.app.ctx)
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(interval)
	}
}

// cycle 每轮先建立完整所有权快照，再处理有限命令与任务，不在事务中等待引擎。
func (m *downloadManager) cycle(ctx context.Context) time.Duration {
	status, err := m.engine.probe(ctx)
	if err != nil {
		m.setConnection(status, err)
		return downloadErrorInterval
	}
	torrents, err := m.engine.torrents(ctx)
	if err != nil {
		m.setConnection(status, err)
		return downloadErrorInterval
	}
	m.setConnection(status, nil)
	m.mu.Lock()
	m.engineSnapshot = append([]qbTorrent(nil), torrents...)
	m.mu.Unlock()
	changed, err := m.processOperations(ctx, torrents)
	if err != nil {
		m.setConnection(status, err)
		return downloadErrorInterval
	}
	if changed {
		torrents, err = m.engine.torrents(ctx)
		if err != nil {
			m.setConnection(status, err)
			return downloadErrorInterval
		}
		m.mu.Lock()
		m.engineSnapshot = append([]qbTorrent(nil), torrents...)
		m.mu.Unlock()
	}
	records, err := m.activeRecords(ctx)
	if err != nil {
		m.app.logger.Error("下载任务对账读取失败")
		return downloadErrorInterval
	}
	active := false
	live := map[string]bool{}
	for _, d := range records {
		if ctx.Err() != nil {
			return downloadErrorInterval
		}
		live[d.ID] = true
		if d.State != "paused" && d.State != "seeding" && d.State != "completed" && d.State != "missing" && d.State != "error" {
			active = true
		}
		if err = m.syncRecord(ctx, d, torrents); err != nil {
			var engineErr *downloadEngineError
			if errors.As(err, &engineErr) {
				m.setConnection(status, err)
				return downloadErrorInterval
			}
			m.recordError(ctx, d, "下载状态核对失败，请检查下载器与目录权限", false)
		}
	}
	m.mu.Lock()
	for id := range m.snapshots {
		if !live[id] {
			delete(m.snapshots, id)
			delete(m.lastPersist, id)
		}
	}
	m.mu.Unlock()
	if active {
		return downloadActiveInterval
	}
	return downloadIdleInterval
}

// activeRecords 有界读取当前任务后立即释放数据库 rows。
func (m *downloadManager) activeRecords(ctx context.Context) ([]downloadRecord, error) {
	rows, err := m.app.db.QueryContext(ctx, downloadSelect+" WHERE archived=0 ORDER BY created_at LIMIT ?", maxDownloadTasks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []downloadRecord{}
	for rows.Next() {
		d, scanErr := scanDownload(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, d)
	}
	return records, rows.Err()
}

// identityMatches 保守比较算法身份，纯 v2 截短引擎 ID 只用于寻找需补证的候选。
func identityMatches(d downloadRecord, t qbTorrent) bool {
	hash := strings.ToLower(t.Hash)
	return (d.IdentityV1 != "" && (strings.EqualFold(t.InfohashV1, d.IdentityV1) || hash == d.IdentityV1)) || (d.IdentityV2 != "" && (strings.EqualFold(t.InfohashV2, d.IdentityV2) || hash == d.IdentityV2 || (len(d.IdentityV2) == 64 && hash == d.IdentityV2[:40])))
}

// findOwnedTorrent 必须同时匹配 UUID 标签、受控目录与已知身份，不能接管外部同 hash 任务。
func (m *downloadManager) findOwnedTorrent(d downloadRecord, torrents []qbTorrent) (*qbTorrent, error) {
	var found *qbTorrent
	for i := range torrents {
		t := torrents[i]
		if t.Hash != d.EngineHash && !hasDownloadTag(t.Tags, d.ID) {
			continue
		}
		if !hasDownloadTag(t.Tags, d.ID) {
			return nil, fmt.Errorf("下载器任务所有权标签已改变，已停止自动操作")
		}
		if found != nil {
			return nil, fmt.Errorf("下载器存在重复任务标签，已停止自动操作")
		}
		if !identityMatches(d, t) {
			return nil, fmt.Errorf("下载器任务身份与本地记录不一致，已停止自动操作")
		}
		actual, err := cleanEnginePath(t.SavePath)
		stage := engineTaskPath(m.app.config.Downloads, "staging/"+d.ID)
		final := engineTaskPath(m.app.config.Downloads, "complete/"+d.ContentType+"/"+d.ID)
		if err != nil || (actual != stage && actual != final) || filepath.Clean(d.LocalRoot) != m.app.config.Downloads.Root {
			return nil, fmt.Errorf("下载器任务目录已偏离受管下载卷，已停止自动操作")
		}
		copy := t
		found = &copy
	}
	return found, nil
}

// pendingOperations 有界恢复未完成命令；已发送的 add 不会被自动再次发送。
func (m *downloadManager) pendingOperations(ctx context.Context) ([]downloadOperation, error) {
	rows, err := m.app.db.QueryContext(ctx, "SELECT id,request_id,download_id,revision,kind,payload,status FROM download_operations WHERE status IN ('queued','sent','uncertain') ORDER BY updated_at,id LIMIT 32")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := []downloadOperation{}
	for rows.Next() {
		var op downloadOperation
		if err = rows.Scan(&op.ID, &op.RequestID, &op.DownloadID, &op.Revision, &op.Kind, &op.Payload, &op.Status); err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}
	return operations, rows.Err()
}

// processOperations 只执行当前修订意图，迟到响应不得覆盖后续暂停或移除请求。
func (m *downloadManager) processOperations(ctx context.Context, torrents []qbTorrent) (bool, error) {
	operations, err := m.pendingOperations(ctx)
	if err != nil {
		return false, err
	}
	changed := false
	for _, op := range operations {
		if ctx.Err() != nil {
			return changed, ctx.Err()
		}
		d, readErr := m.app.downloadByID(ctx, op.DownloadID)
		if readErr != nil {
			return changed, readErr
		}
		if d.Archived || (op.Revision != d.Revision && op.Kind != "add") {
			m.finishOperation(ctx, op, "superseded")
			continue
		}
		t, ownershipErr := m.findOwnedTorrent(d, torrents)
		if ownershipErr != nil {
			m.recordError(ctx, d, ownershipErr.Error(), true)
			m.finishOperation(ctx, op, "failed")
			continue
		}
		if op.Kind == "add" {
			if t != nil {
				if err = m.bindTorrent(ctx, d, t); err != nil {
					if errors.Is(err, errDownloadIdentityPending) {
						m.finishOperation(ctx, op, "sent")
						continue
					}
					var engineErr *downloadEngineError
					if errors.As(err, &engineErr) {
						return changed, err
					}
					m.recordError(ctx, d, err.Error(), true)
					m.finishOperation(ctx, op, "failed")
					continue
				}
				m.finishOperation(ctx, op, "completed")
				continue
			}
			conflict := false
			for _, external := range torrents {
				if identityMatches(d, external) {
					conflict = true
					break
				}
			}
			if conflict {
				m.recordError(ctx, d, "下载器已有相同内容的外部任务，栖库不会接管或修改它", true)
				m.finishOperation(ctx, op, "failed")
				continue
			}
			if op.Status != "queued" {
				m.recordError(ctx, d, "添加结果尚未确认，系统只对账，不会重复提交；请检查下载器", false)
				m.finishOperation(ctx, op, "uncertain")
				continue
			}
			// 只有从未发出的添加可以发送；sent 必须在网络调用之前持久化。
			source, sourceErr := m.readSource(d)
			if sourceErr != nil {
				m.recordError(ctx, d, sourceErr.Error(), true)
				m.finishOperation(ctx, op, "failed")
				continue
			}
			if _, err = m.app.db.ExecContext(ctx, "UPDATE download_operations SET status='sent',updated_at=? WHERE id=? AND status='queued'", now(), op.ID); err != nil {
				return changed, err
			}
			err = m.engine.add(ctx, d, source)
			changed = true
			if err != nil {
				m.finishOperation(ctx, op, "uncertain")
				m.recordError(ctx, d, "下载器添加结果尚未确认，系统将按任务身份对账", false)
				return changed, err
			}
			continue
		}
		if op.Kind == "reconcile" {
			m.finishOperation(ctx, op, "completed")
			continue
		}
		if t == nil {
			if op.Kind == "remove" {
				var unsettled int
				if err = m.app.db.QueryRowContext(ctx, "SELECT count(*) FROM download_operations WHERE download_id=? AND kind='add' AND status IN ('sent','uncertain')", d.ID).Scan(&unsettled); err != nil {
					return changed, err
				}
				if unsettled > 0 {
					m.recordError(ctx, d, "添加结果尚未确认，确认引擎任务归属前暂不归档，以免遗留下载", false)
					m.finishOperation(ctx, op, "uncertain")
					continue
				}
				if err = m.archiveDownload(ctx, d); err != nil {
					return changed, err
				}
				m.finishOperation(ctx, op, "completed")
			} else {
				m.recordError(ctx, d, "下载器中未找到此任务，系统不会自动重新下载", true)
				_, _ = m.app.db.ExecContext(ctx, "UPDATE downloads SET state='missing',ready=0 WHERE id=? AND revision=?", d.ID, d.Revision)
				m.finishOperation(ctx, op, "failed")
			}
			continue
		}
		if d.EngineHash == "" {
			if err = m.bindTorrent(ctx, d, t); err != nil {
				if errors.Is(err, errDownloadIdentityPending) {
					m.finishOperation(ctx, op, "uncertain")
					continue
				}
				var engineErr *downloadEngineError
				if errors.As(err, &engineErr) {
					return changed, err
				}
				m.recordError(ctx, d, err.Error(), true)
				m.finishOperation(ctx, op, "failed")
				continue
			}
			d.EngineHash = t.Hash
		}
		if op.Kind == "move" {
			actual, _ := cleanEnginePath(t.SavePath)
			if actual == engineTaskPath(m.app.config.Downloads, "complete/"+d.ContentType+"/"+d.ID) {
				m.finishOperation(ctx, op, "completed")
				continue
			}
			if t.State == "moving" || strings.HasPrefix(t.State, "checking") {
				m.finishOperation(ctx, op, "sent")
				continue
			}
		}
		if op.Kind == "selection" || op.Kind == "start" {
			if err = m.checkSpace(ctx, d); err != nil {
				m.recordError(ctx, d, err.Error(), false)
				_, _ = m.app.db.ExecContext(ctx, "UPDATE downloads SET state='paused' WHERE id=? AND revision=?", d.ID, d.Revision)
				m.finishOperation(ctx, op, "failed")
				continue
			}
		}
		_, err = m.app.db.ExecContext(ctx, "UPDATE download_operations SET status='sent',updated_at=? WHERE id=?", now(), op.ID)
		if err != nil {
			return changed, err
		}
		err = m.executeOperation(ctx, d, op, *t)
		changed = true
		if err != nil {
			m.finishOperation(ctx, op, "uncertain")
			m.recordError(ctx, d, "下载命令结果待确认，系统将重新核对当前任务", false)
			return changed, err
		}
		if op.Kind == "remove" {
			if err = m.archiveDownload(ctx, d); err != nil {
				return changed, err
			}
		}
		if op.Kind != "move" {
			m.finishOperation(ctx, op, "completed")
		}
	}
	return changed, nil
}

// finishOperation 只更新命令执行状态，不覆盖任务的新修订号。
func (m *downloadManager) finishOperation(ctx context.Context, op downloadOperation, status string) {
	_, err := m.app.db.ExecContext(ctx, "UPDATE download_operations SET status=?,updated_at=? WHERE id=? AND status<>'superseded'", status, now(), op.ID)
	if err != nil && ctx.Err() == nil {
		m.app.logger.Error("下载命令执行状态保存失败", "downloadId", op.DownloadID)
	}
}

// readSource 只读取应用数据目录下本任务的有限私密源，不跟随替换后的链接。
func (m *downloadManager) readSource(d downloadRecord) ([]byte, error) {
	expected := filepath.Join(m.app.config.DataDir, "download-sources", d.ID+".source")
	if d.SourceFile != expected {
		return nil, fmt.Errorf("待提交下载源路径无效")
	}
	info, err := os.Lstat(expected)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxTorrentBytes {
		return nil, fmt.Errorf("待提交下载源不可读取，请重新添加")
	}
	file, err := os.Open(expected)
	if err != nil {
		return nil, fmt.Errorf("待提交下载源不可读取")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTorrentBytes+1))
	if err != nil || len(data) > maxTorrentBytes {
		return nil, fmt.Errorf("待提交下载源不可读取或超限")
	}
	var identity torrentIdentity
	if d.SourceKind == "magnet" {
		identity, err = parseMagnet(string(data))
	} else {
		identity, err = parseTorrent(data)
	}
	if err != nil || identity.V1 != d.IdentityV1 || identity.V2 != d.IdentityV2 {
		return nil, fmt.Errorf("待提交下载源与已登记身份不一致")
	}
	return data, nil
}

// bindTorrent 补齐完整算法身份后绑定引擎，并删除确认过的私密来源。
func (m *downloadManager) bindTorrent(ctx context.Context, d downloadRecord, t *qbTorrent) error {
	if t.State == "metaDL" || t.State == "forcedMetaDL" {
		_, err := m.app.db.ExecContext(ctx, "UPDATE downloads SET state='metadata',engine_state=?,last_synced_at=?,updated_at=? WHERE id=? AND revision=?", t.State, now(), now(), d.ID, d.Revision)
		if err != nil {
			return err
		}
		return errDownloadIdentityPending
	}
	// 首次绑定始终取完整属性，磁力可能只提供一种摘要，而混合种子应补齐两个去重别名。
	if err := m.engine.properties(ctx, t); err != nil {
		return err
	}
	v1 := strings.ToLower(t.InfohashV1)
	v2 := strings.ToLower(t.InfohashV2)
	if strings.Trim(v1, "0") == "" {
		v1 = ""
	}
	if strings.Trim(v2, "0") == "" {
		v2 = ""
	}
	if v1 != "" && !validDownloadHash(v1, 40) {
		return fmt.Errorf("下载器 v1 身份无效")
	}
	if v2 != "" && !validDownloadHash(v2, 64) {
		return fmt.Errorf("下载器 v2 身份无效")
	}
	if (d.IdentityV1 != "" && v1 == "") || (d.IdentityV2 != "" && v2 == "") {
		return errDownloadIdentityPending
	}
	if (d.IdentityV1 != "" && v1 != "" && d.IdentityV1 != v1) || (d.IdentityV2 != "" && v2 != "" && d.IdentityV2 != v2) {
		return fmt.Errorf("下载器完整内容身份与提交来源不符，已停止自动操作")
	}
	if v1 == "" {
		v1 = d.IdentityV1
	}
	if v2 == "" {
		v2 = d.IdentityV2
	}
	result, err := m.app.db.ExecContext(ctx, "UPDATE downloads SET engine_hash=?,identity_v1=?,identity_v2=?,source_file='',last_error='',updated_at=? WHERE id=? AND revision=?", t.Hash, v1, v2, now(), d.ID, d.Revision)
	if err != nil {
		return fmt.Errorf("该内容已绑定其他受管任务，请检查下载列表")
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	// 迟到响应不能清理仍由新修订使用的源文件；下轮对账会重新绑定当前意图。
	if count != 1 {
		return nil
	}
	if d.SourceFile == filepath.Join(m.app.config.DataDir, "download-sources", d.ID+".source") {
		if removeErr := os.Remove(d.SourceFile); removeErr != nil && !os.IsNotExist(removeErr) {
			m.app.logger.Warn("已确认下载源暂存清理失败", "downloadId", d.ID)
		}
	}
	return nil
}

// validDownloadHash 验证完整协议摘要，排除引擎用于空身份的全零字串。
func validDownloadHash(value string, size int) bool {
	if len(value) != size || strings.Trim(value, "0") == "" {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// executeOperation 仅发送已验证属于本地任务的 qB 5.x 任务级命令。
func (m *downloadManager) executeOperation(ctx context.Context, d downloadRecord, op downloadOperation, t qbTorrent) error {
	values := url.Values{"hashes": {t.Hash}}
	switch op.Kind {
	case "pause":
		return m.engine.form(ctx, "stop", values)
	case "remove":
		values.Set("deleteFiles", "false")
		return m.engine.form(ctx, "delete", values)
	case "move":
		values.Set("location", engineTaskPath(m.app.config.Downloads, "complete/"+d.ContentType+"/"+d.ID))
		return m.engine.form(ctx, "setLocation", values)
	case "selection", "start":
		files, err := m.app.readDownloadFiles(ctx, d)
		if err != nil {
			return err
		}
		if !d.SelectionLocked || len(files) == 0 {
			return fmt.Errorf("下载清单尚未锁定")
		}
		all := []string{}
		selected := []string{}
		for _, file := range files {
			all = append(all, strconv.Itoa(file.Index))
			if file.Selected {
				selected = append(selected, strconv.Itoa(file.Index))
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("选集为空")
		}
		if err = m.engine.form(ctx, "stop", values); err != nil {
			return err
		}
		if err = m.engine.form(ctx, "filePrio", url.Values{"hash": {t.Hash}, "id": {strings.Join(all, "|")}, "priority": {"0"}}); err != nil {
			return err
		}
		if err = m.engine.form(ctx, "filePrio", url.Values{"hash": {t.Hash}, "id": {strings.Join(selected, "|")}, "priority": {"1"}}); err != nil {
			return err
		}
		return m.engine.form(ctx, "start", values)
	case "limits":
		var limits struct {
			DownloadKiB int64 `json:"downloadKiB"` /* 下载 KiB/s。 */
			UploadKiB   int64 `json:"uploadKiB"`   /* 上传 KiB/s。 */
		}
		if json.Unmarshal([]byte(op.Payload), &limits) != nil || limits.DownloadKiB < 0 || limits.UploadKiB < 0 || limits.DownloadKiB > 1<<30 || limits.UploadKiB > 1<<30 {
			return fmt.Errorf("持久化限速参数无效")
		}
		values.Set("limit", strconv.FormatInt(limits.DownloadKiB*1024, 10))
		if err := m.engine.form(ctx, "setDownloadLimit", values); err != nil {
			return err
		}
		values.Set("limit", strconv.FormatInt(limits.UploadKiB*1024, 10))
		return m.engine.form(ctx, "setUploadLimit", values)
	}
	return fmt.Errorf("未知下载命令")
}

// archiveDownload 归档只移除引擎任务，保留发布清单和所有媒体实体。
func (m *downloadManager) archiveDownload(ctx context.Context, d downloadRecord) error {
	_, err := m.app.db.ExecContext(ctx, "UPDATE downloads SET archived=1,last_error='',download_speed=0,upload_speed=0,updated_at=? WHERE id=? AND revision=?", now(), d.ID, d.Revision)
	if err == nil {
		m.app.logger.Info("下载任务已归档，文件与发布清单保留", "downloadId", d.ID)
	}
	return err
}

// checkSpace 合并当前受管并发任务剩余量与保留空间，不把缓存预算当下载配额。
func (m *downloadManager) checkSpace(ctx context.Context, target downloadRecord) error {
	available, err := freeDiskBytes(target.LocalRoot)
	if err != nil {
		return fmt.Errorf("无法读取下载卷可用空间，请检查挂载")
	}
	records, err := m.activeRecords(ctx)
	if err != nil {
		return fmt.Errorf("读取下载空间预算失败")
	}
	reserve := m.app.config.Downloads.MinFreeBytes
	for _, record := range records {
		if !record.SelectionLocked || record.Ready || (record.ID != target.ID && (record.State == "paused" || record.State == "error" || record.State == "missing")) {
			continue
		}
		view := m.view(record)
		remaining := view.SizeBytes - view.DownloadedBytes
		if remaining > 0 {
			if uint64(remaining) > math.MaxUint64-reserve {
				return fmt.Errorf("下载剩余空间预算溢出")
			}
			reserve += uint64(remaining)
		}
	}
	if available < reserve {
		return fmt.Errorf("下载卷空间不足以容纳所选剩余文件及安全余量，请释放空间后开始")
	}
	return nil
}

// recordError 只保存中文受控诊断，必要时撤销发布以阻止半成品继续读取。
func (m *downloadManager) recordError(ctx context.Context, d downloadRecord, message string, invalidate bool) {
	if d.LastError == message && (!invalidate || !d.Ready) {
		return
	}
	query := "UPDATE downloads SET last_error=?,updated_at=? WHERE id=? AND revision=?"
	if invalidate {
		query = "UPDATE downloads SET last_error=?,updated_at=?,state='error',import_state='failed',ready=0 WHERE id=? AND revision=?"
	}
	if _, err := m.app.db.ExecContext(ctx, query, message, now(), d.ID, d.Revision); err == nil {
		m.app.logger.Warn("下载任务需要处理", "downloadId", d.ID, "message", message)
	}
}

// syncRecord 对账引擎身份、受控清单、完成发布与扫描覆盖。
func (m *downloadManager) syncRecord(ctx context.Context, d downloadRecord, torrents []qbTorrent) error {
	t, err := m.findOwnedTorrent(d, torrents)
	if err != nil {
		m.recordError(ctx, d, err.Error(), true)
		return nil
	}
	if t == nil {
		if d.EngineHash != "" {
			_, err = m.app.db.ExecContext(ctx, "UPDATE downloads SET state='missing',ready=0,last_error='下载器中未找到此任务，系统不会自动重新下载',updated_at=? WHERE id=? AND revision=?", now(), d.ID, d.Revision)
		}
		return err
	}
	// 先冻结重下、校验和元数据状态，不能被下方待处理命令或空文件清单的提前返回绕过。
	if d.Ready && !safeDownloadCompleteState(t.State) {
		m.recordError(ctx, d, "引擎不再满足完成条件，发布已撤销，请重新校验后重试入库", true)
		return nil
	}
	if d.EngineHash == "" {
		if err = m.bindTorrent(ctx, d, t); err != nil {
			if errors.Is(err, errDownloadIdentityPending) {
				m.recordError(ctx, d, err.Error(), false)
				return nil
			}
			var engineErr *downloadEngineError
			if errors.As(err, &engineErr) {
				return err
			}
			m.recordError(ctx, d, err.Error(), true)
			return nil
		}
		d.EngineHash = t.Hash
	}
	var pending int
	if m.app.db.QueryRowContext(ctx, "SELECT count(*) FROM download_operations WHERE download_id=? AND revision=? AND kind NOT IN ('add','move','reconcile') AND status IN ('queued','sent','uncertain')", d.ID, d.Revision).Scan(&pending) != nil {
		return fmt.Errorf("读取命令状态失败")
	}
	if pending > 0 {
		return nil
	}
	if t.State == "metaDL" || t.State == "forcedMetaDL" {
		_, err = m.app.db.ExecContext(ctx, "UPDATE downloads SET state='metadata',engine_state=?,last_synced_at=?,updated_at=? WHERE id=? AND revision=?", t.State, now(), now(), d.ID, d.Revision)
		return err
	}
	files, err := m.engine.files(ctx, t.Hash)
	if err != nil {
		var engineErr *downloadEngineError
		if errors.As(err, &engineErr) {
			return err
		}
		m.recordError(ctx, d, err.Error(), true)
		return nil
	}
	if len(files) == 0 {
		if d.Ready {
			m.recordError(ctx, d, "下载器暂未提供已发布文件清单，发布已撤销，请确认后重试入库", true)
		}
		return nil
	}
	if !d.SelectionLocked {
		if t.State != "stoppedDL" && t.State != "stoppedUP" {
			if err = m.engine.form(ctx, "stop", url.Values{"hashes": {t.Hash}}); err != nil {
				return err
			}
			return nil
		}
		if d.ManifestVersion == 0 {
			return m.saveManifest(ctx, d, *t, files)
		}
		oldFiles, readErr := m.app.readDownloadFiles(ctx, d)
		if readErr != nil {
			return readErr
		}
		if !sameDownloadManifest(oldFiles, files) {
			m.recordError(ctx, d, "待确认文件清单已被引擎修改，请移除后重新添加", true)
			return nil
		}
		_, err = m.app.db.ExecContext(ctx, "UPDATE downloads SET state='awaiting_selection',engine_state=?,last_synced_at=? WHERE id=? AND revision=?", t.State, now(), d.ID, d.Revision)
		return err
	}
	manifest, err := m.app.readDownloadFiles(ctx, d)
	if err != nil {
		return err
	}
	if !sameDownloadManifest(manifest, files) {
		m.recordError(ctx, d, "引擎文件清单已改变，发布已撤销；首期不支持改变已确认选集", true)
		return nil
	}
	if !sameDownloadSelection(manifest, files) {
		m.recordError(ctx, d, "下载器中的文件选择已改变，发布已撤销；请恢复原选集后重试入库", true)
		return nil
	}
	progress := map[int]float64{}
	for _, file := range files {
		progress[file.Index] = file.Progress
	}
	complete := true
	var size, downloaded int64
	for i := range manifest {
		manifest[i].Progress = progress[manifest[i].Index]
		if !manifest[i].Selected {
			continue
		}
		size += manifest[i].SizeBytes
		if manifest[i].Progress < 1 {
			complete = false
		}
		downloaded += int64(float64(manifest[i].SizeBytes) * manifest[i].Progress)
	}
	state := mapDownloadState(t.State, complete)
	if t.AmountLeft > 0 {
		complete = false
	}
	if t.DownloadSpeed < 0 || t.UploadSpeed < 0 || t.Uploaded < 0 || t.DownloadSpeed > maxDownloadInteger || t.UploadSpeed > maxDownloadInteger || t.Uploaded > maxDownloadInteger || t.Ratio < 0 || math.IsNaN(t.Ratio) || math.IsInf(t.Ratio, 0) || t.Leechs < 0 || t.Seeds < 0 {
		m.recordError(ctx, d, "下载器指标超出安全范围", true)
		return nil
	}
	view := d.Download
	view.Name = downloadDisplayName(t.Name)
	view.State = state
	view.EngineState = t.State
	view.SizeBytes = size
	view.DownloadedBytes = downloaded
	view.DownloadSpeed = t.DownloadSpeed
	view.UploadSpeed = t.UploadSpeed
	view.UploadedBytes = t.Uploaded
	view.Ratio = t.Ratio
	view.Peers = t.Leechs + t.Seeds
	view.LastSyncedAt = now()
	view.ETASeconds = nil
	if t.ETA > 0 && t.ETA < 86400*365 && t.DownloadSpeed > 0 && !complete {
		eta := t.ETA
		view.ETASeconds = &eta
	}
	m.mu.Lock()
	m.snapshots[d.ID] = view
	persist := time.Since(m.lastPersist[d.ID]) >= downloadIdleInterval || state != d.State || t.State != d.EngineState
	if persist {
		m.lastPersist[d.ID] = time.Now()
	}
	m.mu.Unlock()
	if persist {
		if err = m.persistSnapshot(ctx, d, view, manifest); err != nil {
			return err
		}
	}
	if !complete && state == "downloading" {
		available, spaceErr := freeDiskBytes(d.LocalRoot)
		if spaceErr != nil || available < m.app.config.Downloads.MinFreeBytes {
			if err = m.engine.form(ctx, "stop", url.Values{"hashes": {t.Hash}}); err != nil {
				return err
			}
			m.recordError(ctx, d, "下载卷低于安全余量，受管任务已暂停；释放空间后可继续", false)
			_, _ = m.app.db.ExecContext(ctx, "UPDATE downloads SET state='paused' WHERE id=? AND revision=?", d.ID, d.Revision)
			return nil
		}
	}
	if !complete || !safeDownloadCompleteState(t.State) {
		if d.Ready {
			m.recordError(ctx, d, "引擎不再满足完成条件，发布已撤销，请重新校验后重试入库", true)
		}
		return nil
	}
	if d.Ready {
		if err = m.verifyPublished(d, manifest); err != nil {
			m.recordError(ctx, d, "已发布文件发生变化或挂载不可用，请在下载器重新校验后重试入库", true)
			return nil
		}
	}
	if d.ImportState == "failed" || d.ImportState == "target_missing" || d.ImportState == "no_supported_files" {
		return nil
	}
	if !d.Ready {
		return m.publish(ctx, d, *t, manifest)
	}
	return m.syncImport(ctx, d, manifest)
}

// persistSnapshot 用一个短事务批量保存指标和变化的文件进度，避免大清单每条触发 WAL 提交。
func (m *downloadManager) persistSnapshot(ctx context.Context, d downloadRecord, view Download, files []DownloadFile) error {
	tx, err := m.app.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE downloads SET name=?,state=?,engine_state=?,size_bytes=?,downloaded_bytes=?,download_speed=?,upload_speed=?,uploaded_bytes=?,ratio=?,peers=?,eta_seconds=?,last_synced_at=?,updated_at=? WHERE id=? AND revision=?", view.Name, view.State, view.EngineState, view.SizeBytes, view.DownloadedBytes, view.DownloadSpeed, view.UploadSpeed, view.UploadedBytes, view.Ratio, view.Peers, view.ETASeconds, view.LastSyncedAt, now(), d.ID, d.Revision)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return nil
	}
	statement, err := tx.PrepareContext(ctx, "UPDATE download_files SET progress=? WHERE download_id=? AND version=? AND file_index=? AND progress<>?")
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, file := range files {
		if _, err = statement.ExecContext(ctx, file.Progress, d.ID, d.ManifestVersion, file.Index, file.Progress); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// mapDownloadState 保留做种、校验、移动等独立含义，不把 100% 一概标成完成。
func mapDownloadState(state string, complete bool) string {
	switch state {
	case "error", "missingFiles", "unknown":
		return "error"
	case "metaDL", "forcedMetaDL":
		return "metadata"
	case "checkingDL", "checkingUP", "checkingResumeData", "allocating":
		return "checking"
	case "moving":
		return "moving"
	case "queuedDL":
		return "queued"
	case "stoppedDL", "stoppedUP":
		if complete {
			return "completed"
		}
		return "paused"
	case "uploading", "stalledUP", "forcedUP", "queuedUP":
		return "seeding"
	case "downloading", "stalledDL", "forcedDL":
		return "downloading"
	}
	return "error"
}

// safeDownloadCompleteState 采用显式白名单，未知或移动、校验状态永不发布。
func safeDownloadCompleteState(state string) bool {
	return state == "stoppedUP" || state == "stoppedDL" || state == "uploading" || state == "stalledUP" || state == "forcedUP" || state == "queuedUP"
}

// downloadDisplayName 限制非可信元数据标题，避免超大或控制字符进入日志和界面。
func downloadDisplayName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if len(name) > 500 {
		name = string([]rune(name)[:min(160, len([]rune(name)))])
	}
	if name == "" {
		return "未命名下载"
	}
	return name
}

// sameDownloadManifest 使用路径、ID、尺寸比较整个锁定清单，不默默接纳外部重命名。
func sameDownloadManifest(left []DownloadFile, right []DownloadFile) bool {
	if len(left) != len(right) {
		return false
	}
	byID := map[int]DownloadFile{}
	for _, file := range right {
		byID[file.Index] = file
	}
	for _, file := range left {
		other, ok := byID[file.Index]
		if !ok || other.Path != file.Path || other.SizeBytes != file.SizeBytes {
			return false
		}
	}
	return true
}

// sameDownloadSelection 核对引擎实际 wanted 清单，防止外部调整优先级绕过已锁定选集。
func sameDownloadSelection(left []DownloadFile, right []DownloadFile) bool {
	selected := make(map[int]bool, len(right))
	for _, file := range right {
		selected[file.Index] = file.Selected
	}
	for _, file := range left {
		value, exists := selected[file.Index]
		if !exists || value != file.Selected {
			return false
		}
	}
	return len(left) == len(right)
}

// saveManifest 首次固定元数据版本，默认一个文件都不自动确认。
func (m *downloadManager) saveManifest(ctx context.Context, d downloadRecord, t qbTorrent, files []DownloadFile) error {
	tx, err := m.app.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var size int64
	for _, file := range files {
		size += file.SizeBytes
	}
	result, err := tx.ExecContext(ctx, "UPDATE downloads SET manifest_version=1,name=?,size_bytes=?,state='awaiting_selection',engine_state=?,last_synced_at=?,last_error='',updated_at=? WHERE id=? AND revision=? AND manifest_version=0", downloadDisplayName(t.Name), size, t.State, now(), now(), d.ID, d.Revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	// 先占有当前修订的清单版本，避免旧网络响应留下无法再次插入的孤立条目。
	if count != 1 {
		return nil
	}
	statement, err := tx.PrepareContext(ctx, "INSERT INTO download_files(download_id,version,file_index,relative_path,size_bytes,progress) VALUES(?,1,?,?,?,?)")
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, file := range files {
		if _, err = statement.ExecContext(ctx, d.ID, file.Index, file.Path, file.SizeBytes, file.Progress); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err == nil {
		m.app.logger.Info("下载元数据已就绪，等待用户确认文件", "downloadId", d.ID, "fileCount", len(files))
	}
	return err
}
