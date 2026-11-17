package app

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// openSource 以目录句柄打开相对路径，阻止路径穿越和软链接越界。
func (a *App) openSource(item Item) (*os.File, error) {
	canonical, err := a.allowedPath(item.LibraryPath)
	if err != nil || canonical != item.LibraryPath {
		return nil, fmt.Errorf("媒体挂载已变化或不可访问")
	}
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return nil, fmt.Errorf("媒体目录离线或无读取权限")
	}
	defer root.Close()
	file, err := root.Open(filepath.FromSlash(item.RelativePath))
	if err != nil {
		return nil, fmt.Errorf("原文件不可访问，请检查挂载并重新扫描")
	}
	return file, nil
}

// file 通过文件句柄提供 64 位 Range/HEAD，不全量读取或压缩媒体响应。
func (a *App) file(w http.ResponseWriter, r *http.Request) {
	item, ok := a.requestItem(w, r)
	if !ok {
		return
	}
	if item.IsDir {
		fail(w, 400, "图片目录请使用漫画阅读器")
		return
	}
	select {
	case a.streams <- struct{}{}:
		defer func() { <-a.streams }()
	default:
		fail(w, 503, "播放连接较多，请稍后重试")
		return
	}
	file, err := a.openSource(item)
	if err != nil {
		fail(w, 404, err.Error())
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		fail(w, 404, "原文件不存在或不是普通文件")
		return
	}
	w.Header().Set("Content-Type", item.Mime)
	// 元数据摘要只能作为弱验证器，不冒充经过整文件哈希的字节身份。
	w.Header().Set("ETag", fmt.Sprintf("W/\"%x-%x\"", info.ModTime().UnixNano(), info.Size()))
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	disposition := "inline"
	if item.Type == "novel" || item.Type == "comic" {
		disposition = "attachment"
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filepath.Base(item.RelativePath)}))
	// ServeContent 使用 Seek 和流式拷贝；连接取消会中断传输并关闭文件。
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

// backup 生成运行中数据库的一致性快照，下载结束后移除临时文件。
func (a *App) backup(w http.ResponseWriter, r *http.Request) {
	if !a.backupMu.TryLock() {
		fail(w, 409, "已有备份正在生成或下载，请稍后重试")
		return
	}
	defer a.backupMu.Unlock()
	info, err := os.Stat(filepath.Join(a.config.DataDir, "library.db"))
	if err != nil {
		fail(w, 500, "无法读取数据库状态")
		return
	}
	free, err := freeDiskBytes(a.config.DataDir)
	if err != nil || free < a.config.MinFreeBytes+uint64(info.Size())*2 {
		fail(w, 507, "剩余空间不足，无法生成一致性备份")
		return
	}
	temporary, err := os.CreateTemp(a.config.DataDir, "backup-*.db")
	if err != nil {
		fail(w, 500, "无法创建备份文件")
		return
	}
	path := temporary.Name()
	temporary.Close()
	os.Remove(path)
	defer os.Remove(path)
	// VACUUM INTO 包含 WAL 中已提交的数据，不是复制正在写入的主文件。
	if _, err = a.db.ExecContext(r.Context(), "VACUUM INTO ?", path); err != nil {
		a.logger.Error("生成一致性备份失败", "error", err)
		fail(w, 500, "数据库备份失败，请稍后重试")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		fail(w, 500, "无法读取备份文件")
		return
	}
	defer file.Close()
	snapshot, err := file.Stat()
	if err != nil {
		fail(w, 500, "备份状态不可用")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=nas-library-%s.db", time.Now().Format("20060102-150405")))
	a.logger.Info("一致性数据库备份已生成", "size", snapshot.Size())
	http.ServeContent(w, r, snapshot.Name(), snapshot.ModTime(), file)
}
