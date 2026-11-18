package app

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// browseEntry 是目录选择器可见的已授权目录。
type browseEntry struct {
	Name  string `json:"name"`  // 目录名称。
	Path  string `json:"path"`  // 服务内部挂载路径。
	IsDir bool   `json:"isDir"` // 是否为目录。
}

// validType 检查媒体库类型是否属于首版支持范围。
func validType(value string) bool {
	return value == "video" || value == "image" || value == "comic" || value == "novel" || value == "music"
}

// within 判断规范化路径是否位于给定根目录内。
func within(root string, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// allowedPath 解析软链接后验证媒体根目录边界。
func (a *App) allowedPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("目录不存在或暂不可访问")
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	for _, root := range a.config.MediaRoots {
		canonical, rootErr := filepath.EvalSymlinks(root)
		if rootErr == nil && within(canonical, resolved) {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("路径不在允许的媒体挂载目录内")
}

// readableDirectory 验证可读目录，拒绝将普通文件纳管为媒体库。
func (a *App) readableDirectory(path string) (string, error) {
	canonical, err := a.allowedPath(path)
	if err != nil {
		return "", err
	}
	file, err := os.Open(canonical)
	if err != nil {
		return "", fmt.Errorf("没有目录读取权限")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("请选择可访问的目录")
	}
	if err = a.checkDownloadPath(canonical, true); err != nil {
		return "", err
	}
	if _, err = file.ReadDir(1); err != nil && err != io.EOF {
		return "", fmt.Errorf("无法读取目录内容")
	}
	return canonical, nil
}

// browse 只列出允许挂载根目录内的子目录，不暴露任意宿主路径。
func (a *App) browse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	entries := []browseEntry{}
	parent := ""
	gate, gateErr := a.loadDownloadGate(r.Context())
	if gateErr != nil {
		fail(w, 503, "下载发布状态暂不可用")
		return
	}
	if path == "" {
		for _, root := range a.config.MediaRoots {
			if allowed, checkErr := gate.allow(root, true); checkErr == nil && allowed {
				entries = append(entries, browseEntry{Name: filepath.Base(root), Path: root, IsDir: true})
			}
		}
		respond(w, 200, map[string]any{"path": "", "parent": "", "entries": entries, "roots": a.config.MediaRoots})
		return
	}
	canonical, err := a.readableDirectory(path)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	children, err := os.ReadDir(canonical)
	if err != nil {
		fail(w, 403, "无法读取目录，请检查挂载与权限")
		return
	}
	for _, child := range children {
		if len(entries) >= 2000 {
			break
		}
		candidate := filepath.Join(canonical, child.Name())
		safe, safeErr := a.allowedPath(candidate)
		if safeErr != nil {
			continue
		}
		info, statErr := os.Stat(safe)
		if statErr == nil && info.IsDir() {
			if allowed, checkErr := gate.allow(safe, true); checkErr != nil || !allowed {
				continue
			}
			entries = append(entries, browseEntry{Name: child.Name(), Path: safe, IsDir: true})
		}
	}
	sort.Slice(entries, func(i int, j int) bool { return naturalLess(entries[i].Name, entries[j].Name) })
	if candidate := filepath.Dir(canonical); candidate != canonical {
		if safe, safeErr := a.allowedPath(candidate); safeErr == nil {
			if allowed, checkErr := gate.allow(safe, true); checkErr == nil && allowed {
				parent = safe
			}
		}
	}
	respond(w, 200, map[string]any{"path": canonical, "parent": parent, "entries": entries, "roots": a.config.MediaRoots})
}

// queryLibraries 读取媒体库与项目数，不执行文件系统扫描。
func (a *App) queryLibraries() ([]Library, error) {
	rows, err := a.db.QueryContext(a.ctx, "SELECT l.id,l.name,l.type,l.path,l.status,l.last_scanned_at,(SELECT count(*) FROM items i WHERE i.library_id=l.id) FROM libraries l ORDER BY l.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	libraries := []Library{}
	for rows.Next() {
		var library Library
		if err = rows.Scan(&library.ID, &library.Name, &library.Type, &library.Path, &library.Status, &library.LastScannedAt, &library.ItemCount); err != nil {
			return nil, err
		}
		libraries = append(libraries, library)
	}
	return libraries, rows.Err()
}

// listLibraries 返回所有已登记媒体库。
func (a *App) listLibraries(w http.ResponseWriter, r *http.Request) {
	libraries, err := a.queryLibraries()
	if err != nil {
		fail(w, 500, "无法读取媒体库")
		return
	}
	respond(w, 200, map[string]any{"libraries": libraries})
}

// libraryByID 查询单个媒体库，包括索引计数。
func (a *App) libraryByID(id int64) (Library, error) {
	var library Library
	err := a.db.QueryRowContext(a.ctx, "SELECT id,name,type,path,status,last_scanned_at,(SELECT count(*) FROM items WHERE library_id=libraries.id) FROM libraries WHERE id=?", id).Scan(&library.ID, &library.Name, &library.Type, &library.Path, &library.Status, &library.LastScannedAt, &library.ItemCount)
	return library, err
}

// createLibrary 验证可读路径后登记媒体库并持久化初始扫描任务。
func (a *App) createLibrary(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"` // 媒体库展示名。
		Type string `json:"type"` // 内容类型。
		Path string `json:"path"` // 已挂载目录。
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 200 || !validType(input.Type) {
		fail(w, 400, "请填写媒体库名称并选择有效类型")
		return
	}
	path, err := a.readableDirectory(input.Path)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "创建媒体库失败")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "INSERT INTO libraries(name,type,path) VALUES(?,?,?)", input.Name, input.Type, path)
	if err != nil {
		fail(w, 409, "该目录已登记为相同类型的媒体库")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		fail(w, 500, "创建媒体库失败")
		return
	}
	_, err = tx.ExecContext(r.Context(), "INSERT INTO jobs(library_id,kind,created_at,updated_at) VALUES(?,'scan',?,?)", id, now(), now())
	if err != nil {
		fail(w, 500, "创建扫描任务失败")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 500, "保存媒体库失败")
		return
	}
	a.notifyWorkers()
	library, err := a.libraryByID(id)
	if err != nil {
		fail(w, 500, "读取新媒体库失败")
		return
	}
	a.logger.Info("媒体库已创建并安排扫描", "libraryId", id, "type", input.Type)
	respond(w, 201, map[string]any{"library": library})
}

// deleteLibrary 删除数据库索引并取消相关任务，绝不修改原文件。
func (a *App) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := requestID(w, r)
	if !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), "SELECT id FROM jobs WHERE library_id=? AND status='running'", id)
	if err != nil {
		fail(w, 500, "删除媒体库失败")
		return
	}
	ids := []int64{}
	for rows.Next() {
		var jobID int64
		if rows.Scan(&jobID) == nil {
			ids = append(ids, jobID)
		}
	}
	rows.Close()
	a.mu.Lock()
	for _, jobID := range ids {
		if cancel := a.cancels[jobID]; cancel != nil {
			cancel()
		}
	}
	a.mu.Unlock()
	result, err := a.db.ExecContext(r.Context(), "DELETE FROM libraries WHERE id=?", id)
	if err != nil {
		fail(w, 500, "删除媒体库失败")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		fail(w, 404, "媒体库不存在")
		return
	}
	a.logger.Info("媒体库索引已删除，源文件保持不变", "libraryId", id)
	respond(w, 200, map[string]bool{"ok": true})
}

// scanLibrary 提交幂等扫描任务，重复点击复用已有等待或运行任务。
func (a *App) scanLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := requestID(w, r)
	if !ok {
		return
	}
	if _, err := a.libraryByID(id); err != nil {
		fail(w, 404, "媒体库不存在")
		return
	}
	job, err := a.enqueueScan(id)
	if err != nil {
		fail(w, 500, "无法创建扫描任务")
		return
	}
	respond(w, 202, map[string]any{"job": job})
}

// enqueueScan 通过唯一索引保证同一媒体库仅有一个活跃扫描。
func (a *App) enqueueScan(libraryID int64) (Job, error) {
	_, err := a.db.ExecContext(a.ctx, "INSERT OR IGNORE INTO jobs(library_id,kind,created_at,updated_at) VALUES(?,'scan',?,?)", libraryID, now(), now())
	if err != nil {
		return Job{}, err
	}
	var job Job
	err = a.db.QueryRowContext(a.ctx, "SELECT id,library_id,kind,status,processed,message,created_at,updated_at,coalesce(item_id,0) FROM jobs WHERE library_id=? AND kind='scan' AND status IN ('queued','running') ORDER BY id DESC LIMIT 1", libraryID).Scan(&job.ID, &job.LibraryID, &job.Kind, &job.Status, &job.Processed, &job.Message, &job.CreatedAt, &job.UpdatedAt, &job.ItemID)
	if err == sql.ErrNoRows {
		return job, fmt.Errorf("扫描任务已结束，请刷新列表")
	}
	a.notifyWorkers()
	return job, err
}
