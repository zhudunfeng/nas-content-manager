package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// publish 先持久化迁移意图，再等待引擎真实最终路径及逐文件核验。
func (m *downloadManager) publish(ctx context.Context, d downloadRecord, t qbTorrent, files []DownloadFile) error {
	actual, err := cleanEnginePath(t.SavePath)
	if err != nil {
		return err
	}
	final := engineTaskPath(m.app.config.Downloads, "complete/"+d.ContentType+"/"+d.ID)
	if actual != final {
		tx, beginErr := m.app.db.BeginTx(ctx, nil)
		if beginErr != nil {
			return beginErr
		}
		defer tx.Rollback()
		result, updateErr := tx.ExecContext(ctx, "UPDATE downloads SET import_state='publishing',ready=0,updated_at=? WHERE id=? AND revision=?", now(), d.ID, d.Revision)
		if updateErr != nil {
			return updateErr
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			return nil
		}
		_, updateErr = tx.ExecContext(ctx, "INSERT OR IGNORE INTO download_operations(request_id,download_id,revision,kind,created_at,updated_at) VALUES(?,?,?,'move',?,?)", "publish-"+d.ID+"-"+fmt.Sprint(d.ManifestVersion)+"-"+fmt.Sprint(d.Revision), d.ID, d.Revision, now(), now())
		if updateErr != nil {
			return updateErr
		}
		if updateErr = tx.Commit(); updateErr == nil {
			m.app.logger.Info("下载完成，已登记最终目录迁移意图", "downloadId", d.ID)
			m.notify()
		}
		return updateErr
	}
	// HTTP 200 只代表移动请求已接收；只有实际目录、稳定状态、常规文件及尺寸同时成立才发布。
	if !safeDownloadCompleteState(t.State) {
		return nil
	}
	verified, err := m.inspectPublication(d, files, false)
	if err != nil {
		m.recordError(ctx, d, "最终目录文件尚不可读或与选中文件尺寸不符，保留文件等待确认", false)
		return nil
	}
	targets := downloadImportTargets(d, verified)
	importState := "queued"
	if len(targets) == 0 {
		importState = "no_supported_files"
	}
	tx, err := m.app.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, file := range verified {
		if file.Selected {
			if _, err = tx.ExecContext(ctx, "UPDATE download_files SET mod_time=?,progress=1 WHERE download_id=? AND version=? AND file_index=? AND selected=1", file.ModTime, d.ID, d.ManifestVersion, file.Index); err != nil {
				return err
			}
		}
	}
	result, err := tx.ExecContext(ctx, "UPDATE downloads SET ready=1,import_state=?,last_error=?,updated_at=? WHERE id=? AND revision=? AND manifest_version=? AND selection_locked=1", importState, map[bool]string{true: "所选文件中没有当前内容类型支持的文件，下载文件已保留", false: ""}[len(targets) == 0], now(), d.ID, d.Revision, d.ManifestVersion)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return nil
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	m.app.logger.Info("下载文件已核验发布", "downloadId", d.ID, "selectedFileCount", len(verified), "supportedItems", len(targets))
	if len(targets) == 0 {
		return nil
	}
	d.Ready = true
	d.ImportState = "queued"
	return m.syncImport(ctx, d, verified)
}

// inspectPublication 不经过尚未开放的发布门禁，使用只读根句柄与每段 Lstat 校验选中文件。
func (m *downloadManager) inspectPublication(d downloadRecord, files []DownloadFile, baseline bool) ([]DownloadFile, error) {
	if !d.SelectionLocked || d.ManifestVersion < 1 || filepath.Clean(d.LocalRoot) != m.app.config.Downloads.Root || d.FinalPath != filepath.Join(d.LocalRoot, "complete", d.ContentType, d.ID) {
		return nil, fmt.Errorf("发布路径或清单状态不安全")
	}
	canonical, err := filepath.EvalSymlinks(d.FinalPath)
	if err != nil || downloadPathKey(canonical) != downloadPathKey(d.FinalPath) {
		return nil, fmt.Errorf("发布目录包含链接或暂不可读")
	}
	root, err := os.OpenRoot(d.FinalPath)
	if err != nil {
		return nil, fmt.Errorf("发布目录不可读取")
	}
	defer root.Close()
	result := append([]DownloadFile(nil), files...)
	selected := 0
	for i, file := range result {
		if !file.Selected {
			continue
		}
		selected++
		if err = validateDownloadRelative(file.Path); err != nil {
			return nil, err
		}
		relative := filepath.FromSlash(file.Path)
		current := relative
		for {
			info, statErr := root.Lstat(current)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("发布文件含链接或不可读取")
			}
			parent := filepath.Dir(current)
			if parent == "." {
				break
			}
			current = parent
		}
		f, openErr := root.Open(relative)
		if openErr != nil {
			return nil, fmt.Errorf("发布文件无法打开")
		}
		info, statErr := f.Stat()
		f.Close()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != file.SizeBytes {
			return nil, fmt.Errorf("发布文件不是预期尺寸的常规文件")
		}
		if baseline && (file.ModTime == 0 || file.ModTime != info.ModTime().UnixNano()) {
			return nil, fmt.Errorf("发布文件版本已改变")
		}
		result[i].ModTime = info.ModTime().UnixNano()
	}
	if selected == 0 {
		return nil, fmt.Errorf("发布选集为空")
	}
	return result, nil
}

// verifyPublished 核验发布基线，源文件同尺寸替换也会撤销门禁。
func (m *downloadManager) verifyPublished(d downloadRecord, files []DownloadFile) error {
	_, err := m.inspectPublication(d, files, true)
	return err
}

// downloadImportTargets 得到扫描必须实际覆盖的项目及大小，目录漫画只计算选中的直接图片。
func downloadImportTargets(d downloadRecord, files []DownloadFile) map[string]int64 {
	targets := map[string]int64{}
	for _, file := range files {
		if !file.Selected {
			continue
		}
		relative := path.Join(d.ID, file.Path)
		if d.ContentType == "comic" && isImage(file.Path) {
			targets[path.Dir(relative)] += file.SizeBytes
		} else if matchesType(d.ContentType, file.Path) {
			targets[relative] = file.SizeBytes
		}
	}
	return targets
}

// ensureDownloadLibrary 只进行首次显式授权创建，曾绑定后被删除的目标始终要求用户处理。
func (m *downloadManager) ensureDownloadLibrary(ctx context.Context, d downloadRecord) (int64, error) {
	expected := filepath.Join(d.LocalRoot, "complete", d.ContentType)
	if d.LibraryID != nil {
		var kind, registered string
		err := m.app.db.QueryRowContext(ctx, "SELECT type,path FROM libraries WHERE id=?", *d.LibraryID).Scan(&kind, &registered)
		if err != nil || kind != d.ContentType || downloadPathKey(registered) != downloadPathKey(expected) {
			return 0, fmt.Errorf("目标下载媒体库已删除或改变，请重新登记相同类型与目录后重试入库")
		}
		return *d.LibraryID, nil
	}
	if d.LibraryBound || !d.CreateLibrary {
		return 0, fmt.Errorf("目标下载媒体库缺失，未获授权自动创建")
	}
	canonical, err := m.app.readableDirectory(expected)
	if err != nil || downloadPathKey(canonical) != downloadPathKey(expected) {
		return 0, fmt.Errorf("下载库目录不可读或不在允许媒体根中，请授权 complete 子目录")
	}
	tx, err := m.app.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	names := map[string]string{"video": "下载 · 视频", "image": "下载 · 图片", "comic": "下载 · 漫画", "novel": "下载 · 小说", "music": "下载 · 音乐"}
	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO libraries(name,type,path) VALUES(?,?,?)", names[d.ContentType], d.ContentType, expected)
	if err != nil {
		return 0, err
	}
	var id int64
	if err = tx.QueryRowContext(ctx, "SELECT id FROM libraries WHERE type=? AND path=?", d.ContentType, expected).Scan(&id); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "UPDATE downloads SET target_library_id=?,library_bound=1,updated_at=? WHERE id=? AND revision=? AND library_bound=0", id, now(), d.ID, d.Revision)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return 0, fmt.Errorf("目标媒体库绑定已更新，请稍后重试")
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	m.app.logger.Info("受管下载媒体库已登记", "downloadId", d.ID, "libraryId", id, "type", d.ContentType)
	return id, nil
}

// recoverMissingLibrary 仅接受用户重新登记的相同受管路径，不自动重新创建被删除媒体库。
func (m *downloadManager) recoverMissingLibrary(ctx context.Context, d downloadRecord) (int64, error) {
	id, err := m.ensureDownloadLibrary(ctx, d)
	if err == nil || d.ImportState != "pending" || !d.LibraryBound {
		return id, err
	}
	var existing int64
	queryErr := m.app.db.QueryRowContext(ctx, "SELECT id FROM libraries WHERE type=? AND path=?", d.ContentType, filepath.Join(d.LocalRoot, "complete", d.ContentType)).Scan(&existing)
	if queryErr != nil {
		return 0, err
	}
	_, queryErr = m.app.db.ExecContext(ctx, "UPDATE downloads SET target_library_id=?,updated_at=? WHERE id=? AND revision=?", existing, now(), d.ID, d.Revision)
	if queryErr != nil {
		return 0, queryErr
	}
	return existing, nil
}

// syncImport 按实际项目覆盖确认入库，运行中扫描可能已越过新目录时最多安排一次补扫。
func (m *downloadManager) syncImport(ctx context.Context, d downloadRecord, files []DownloadFile) error {
	targets := downloadImportTargets(d, files)
	if len(targets) == 0 {
		_, err := m.app.db.ExecContext(ctx, "UPDATE downloads SET import_state='no_supported_files',last_error='所选文件中没有当前类型支持的内容，原文件已保留',updated_at=? WHERE id=? AND revision=?", now(), d.ID, d.Revision)
		return err
	}
	library, err := m.recoverMissingLibrary(ctx, d)
	if err != nil {
		_, dbErr := m.app.db.ExecContext(ctx, "UPDATE downloads SET import_state='target_missing',last_error=?,updated_at=? WHERE id=? AND revision=?", err.Error(), now(), d.ID, d.Revision)
		if dbErr == nil && d.ImportState != "target_missing" {
			m.app.logger.Warn("下载入库目标不可用", "downloadId", d.ID)
		}
		return dbErr
	}
	covered, err := m.importCovered(ctx, library, targets)
	if err != nil {
		return err
	}
	if covered {
		if d.ImportState != "imported" {
			_, err = m.app.db.ExecContext(ctx, "UPDATE downloads SET import_state='imported',last_error='',updated_at=? WHERE id=? AND revision=?", now(), d.ID, d.Revision)
			if err == nil {
				m.app.logger.Info("下载内容已确认入库", "downloadId", d.ID, "libraryId", library, "itemCount", len(targets))
			}
		}
		return err
	}
	if d.ScanJobID > 0 {
		var state string
		err = m.app.db.QueryRowContext(ctx, "SELECT status FROM jobs WHERE id=? AND library_id=? AND kind='scan'", d.ScanJobID, library).Scan(&state)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if state == "queued" || state == "running" {
			importState := "queued"
			if state == "running" {
				importState = "scanning"
			}
			if d.ImportState != importState {
				_, err = m.app.db.ExecContext(ctx, "UPDATE downloads SET import_state=?,updated_at=? WHERE id=? AND revision=?", importState, now(), d.ID, d.Revision)
			}
			return err
		}
		if state == "failed" || state == "cancelled" || d.ScanAttempts >= 2 {
			_, err = m.app.db.ExecContext(ctx, "UPDATE downloads SET import_state='failed',last_error='扫描未覆盖全部已选内容，请检查扫描任务后重试入库',updated_at=? WHERE id=? AND revision=?", now(), d.ID, d.Revision)
			return err
		}
	}
	job, err := m.app.enqueueScan(library)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	}
	_, err = m.app.db.ExecContext(ctx, "UPDATE downloads SET scan_job_id=?,scan_attempts=scan_attempts+1,import_state='queued',updated_at=? WHERE id=? AND revision=?", job.ID, now(), d.ID, d.Revision)
	if err == nil {
		m.app.logger.Info("下载内容已安排媒体扫描", "downloadId", d.ID, "libraryId", library, "jobId", job.ID)
	}
	return err
}

// importCovered 核对每个预期文件或漫画目录确实已登记，不能把无关扫描完成当作成功。
func (m *downloadManager) importCovered(ctx context.Context, library int64, targets map[string]int64) (bool, error) {
	if len(targets) == 0 {
		return true, nil
	}
	prefix := ""
	for relative := range targets {
		prefix = relative
		if slash := strings.IndexByte(relative, '/'); slash >= 0 {
			prefix = relative[:slash]
		}
		break
	}
	rows, err := m.app.db.QueryContext(ctx, "SELECT relative_path,size FROM items WHERE library_id=? AND missing=0 AND (relative_path=? OR substr(relative_path,1,?)=?)", library, prefix, len(prefix)+1, prefix+"/")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	covered := 0
	for rows.Next() {
		var relative string
		var size int64
		if err = rows.Scan(&relative, &size); err != nil {
			return false, err
		}
		if expected, ok := targets[relative]; ok && expected == size {
			covered++
		}
	}
	return covered == len(targets), rows.Err()
}
