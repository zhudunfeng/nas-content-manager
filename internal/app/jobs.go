package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// notifyWorkers 合并频繁入队通知，避免无界通知堆积。
func (a *App) notifyWorkers() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// startWorkers 启动固定数量执行器和定期对账，不创建每文件 goroutine。
func (a *App) startWorkers() {
	for i := 0; i < a.config.Workers; i++ {
		a.wg.Add(1)
		go a.worker()
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(time.Duration(a.config.ScanIntervalMinutes) * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				libraries, err := a.queryLibraries()
				if err != nil {
					a.logger.Error("定期扫描读取媒体库失败", "error", err)
					continue
				}
				for _, library := range libraries {
					if _, err = a.enqueueScan(library.ID); err != nil {
						a.logger.Warn("定期扫描入队失败", "libraryId", library.ID, "error", err)
					}
				}
				_, _ = a.db.ExecContext(a.ctx, "DELETE FROM sessions WHERE expires_at<?", now())
			}
		}
	}()
}

// claimJob 在短事务中原子领取任务，扫描优先于衍生处理。
func (a *App) claimJob() (Job, error) {
	tx, err := a.db.BeginTx(a.ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	var job Job
	err = tx.QueryRowContext(a.ctx, "SELECT id,library_id,kind,status,processed,message,created_at,updated_at,coalesce(item_id,0) FROM jobs WHERE status='queued' ORDER BY CASE kind WHEN 'scan' THEN 0 ELSE 1 END,id LIMIT 1").Scan(&job.ID, &job.LibraryID, &job.Kind, &job.Status, &job.Processed, &job.Message, &job.CreatedAt, &job.UpdatedAt, &job.ItemID)
	if err != nil {
		return job, err
	}
	_, err = tx.ExecContext(a.ctx, "UPDATE jobs SET status='running',message='正在处理',updated_at=? WHERE id=? AND status='queued'", now(), job.ID)
	if err != nil {
		return job, err
	}
	return job, tx.Commit()
}

// worker 串行消费任务并传播取消信号；进程重启后数据库恢复未完成任务。
func (a *App) worker() {
	defer a.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if a.ctx.Err() != nil {
			return
		}
		// 领取与安装取消句柄共用锁，重试不能穿过旧执行器尚未登记的窗口。
		a.mu.Lock()
		job, err := a.claimJob()
		var ctx context.Context
		var cancel context.CancelFunc
		if err == nil {
			ctx, cancel = context.WithCancel(a.ctx)
			a.cancels[job.ID] = cancel
		}
		a.mu.Unlock()
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) && a.ctx.Err() == nil {
				a.logger.Error("领取后台任务失败", "error", err)
			}
			select {
			case <-a.ctx.Done():
				return
			case <-a.wake:
			case <-ticker.C:
			}
			continue
		}
		// 领取与安装取消句柄之间的取消请求仍以数据库状态为准。
		var state string
		_ = a.db.QueryRowContext(a.ctx, "SELECT status FROM jobs WHERE id=?", job.ID).Scan(&state)
		if state != "running" {
			cancel()
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		} else if job.Kind == "scan" {
			err = a.runScan(ctx, job)
		} else {
			err = a.enrich(ctx, job)
		}
		cancel()
		// 收尾完成后才释放执行占位，防止旧任务覆盖立即重试后的新状态。
		a.mu.Lock()
		if a.ctx.Err() != nil {
			delete(a.cancels, job.ID)
			a.mu.Unlock()
			return
		}
		status, message := "completed", "处理完成"
		if err != nil {
			status = "failed"
			message = err.Error()
			if errors.Is(err, context.Canceled) {
				status = "cancelled"
				message = "任务已取消"
			}
			a.logger.Warn("后台任务未完成", "jobId", job.ID, "kind", job.Kind, "error", message)
		}
		_, _ = a.db.ExecContext(a.ctx, "UPDATE jobs SET status=?,message=?,updated_at=? WHERE id=? AND status='running'", status, message, now(), job.ID)
		if job.Kind == "enrich" {
			// 处理期间源文件若被替换，旧任务不覆盖新版本状态，重新安排仍待处理的版本。
			_, _ = a.db.ExecContext(a.ctx, "INSERT OR IGNORE INTO jobs(library_id,item_id,kind,created_at,updated_at) SELECT library_id,id,'enrich',?,? FROM items WHERE id=? AND enrich_state='pending' AND EXISTS(SELECT 1 FROM jobs WHERE id=? AND status<>'cancelled')", now(), now(), job.ItemID, job.ID)
		}
		delete(a.cancels, job.ID)
		a.mu.Unlock()
	}
}

// listJobs 返回有限历史记录，避免媒体量大时将任务全表装入内存。
func (a *App) listJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,library_id,kind,status,processed,message,created_at,updated_at FROM jobs ORDER BY CASE WHEN status IN ('running','queued') THEN 0 ELSE 1 END,id DESC LIMIT 200")
	if err != nil {
		fail(w, 500, "无法读取任务")
		return
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		var job Job
		if rows.Scan(&job.ID, &job.LibraryID, &job.Kind, &job.Status, &job.Processed, &job.Message, &job.CreatedAt, &job.UpdatedAt) != nil {
			fail(w, 500, "读取任务失败")
			return
		}
		jobs = append(jobs, job)
	}
	respond(w, 200, map[string]any{"jobs": jobs})
}

// cancelJob 更新任务状态并取消当前文件遍历或 FFmpeg 进程。
func (a *App) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, ok := requestID(w, r)
	if !ok {
		return
	}
	// 状态更新和取消句柄查找保持原子，迟到的取消请求不能误杀刚重试的执行器。
	a.mu.Lock()
	defer a.mu.Unlock()
	result, err := a.db.ExecContext(r.Context(), "UPDATE jobs SET status='cancelled',message='用户已取消',updated_at=? WHERE id=? AND status IN ('queued','running')", now(), id)
	if err != nil {
		fail(w, 500, "取消任务失败")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		fail(w, 409, "任务已结束或不存在")
		return
	}
	if cancel := a.cancels[id]; cancel != nil {
		cancel()
	}
	a.logger.Info("后台任务已取消", "jobId", id)
	respond(w, 200, map[string]bool{"ok": true})
}

// retryJob 仅重试失败或已取消任务，并保留幂等唯一索引约束。
func (a *App) retryJob(w http.ResponseWriter, r *http.Request) {
	id, ok := requestID(w, r)
	if !ok {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancels[id] != nil {
		fail(w, 409, "任务仍在停止，请稍后重试")
		return
	}
	result, err := a.db.ExecContext(r.Context(), "UPDATE jobs SET status='queued',processed=0,message='等待重试',updated_at=? WHERE id=? AND status IN ('failed','cancelled')", now(), id)
	if err != nil {
		fail(w, 409, "该媒体库或内容已有活跃任务")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		fail(w, 409, "只有失败或取消的任务可以重试")
		return
	}
	a.notifyWorkers()
	a.logger.Info("后台任务已安排重试", "jobId", id)
	respond(w, 200, map[string]bool{"ok": true})
}

// runScan 轻量登记文件，扫描完全成功后才更新缺失标记。
func (a *App) runScan(ctx context.Context, job Job) error {
	library, err := a.libraryByID(job.LibraryID)
	if err != nil {
		return fmt.Errorf("媒体库已移除")
	}
	if _, err = a.readableDirectory(library.Path); err != nil {
		_, _ = a.db.ExecContext(a.ctx, "UPDATE libraries SET status='offline' WHERE id=?", library.ID)
		return fmt.Errorf("媒体目录不可访问，请检查挂载与权限")
	}
	_, _ = a.db.ExecContext(ctx, "UPDATE libraries SET status='scanning' WHERE id=?", library.ID)
	a.logger.Info("媒体扫描开始", "libraryId", library.ID, "jobId", job.ID)
	token := now()
	var processed int64
	err = filepath.WalkDir(library.Path, func(path string, entry fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return fmt.Errorf("目录遍历失败，请检查挂载与权限")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if library.Type != "comic" {
				return nil
			}
			children, readErr := os.ReadDir(path)
			if readErr != nil {
				return fmt.Errorf("漫画目录读取失败")
			}
			var size int64
			var newest int64
			count := 0
			for _, child := range children {
				if child.Type()&os.ModeSymlink == 0 && !child.IsDir() && isImage(child.Name()) {
					info, infoErr := child.Info()
					if infoErr != nil {
						return infoErr
					}
					count++
					size += info.Size()
					if info.ModTime().UnixNano() > newest {
						newest = info.ModTime().UnixNano()
					}
				}
			}
			if count == 0 {
				return nil
			}
			if err := a.indexItem(ctx, library, path, size, newest, true, token); err != nil {
				return err
			}
			processed++
		} else {
			if !matchesType(library.Type, path) {
				return nil
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			if err := a.indexItem(ctx, library, path, info.Size(), info.ModTime().UnixNano(), false, token); err != nil {
				return err
			}
			processed++
		}
		if processed%25 == 0 {
			_, _ = a.db.ExecContext(ctx, "UPDATE jobs SET processed=?,message='正在发现内容',updated_at=? WHERE id=?", processed, now(), job.ID)
		}
		return nil
	})
	if err != nil {
		_, _ = a.db.ExecContext(a.ctx, "UPDATE libraries SET status='error' WHERE id=?", library.ID)
		return err
	}
	// 空挂载可能只是网络存储临时失联；保留旧索引供用户确认，绝不清库。
	if processed > 0 {
		_, err = a.db.ExecContext(ctx, "UPDATE items SET missing=1 WHERE library_id=? AND scan_token<>?", library.ID, token)
		if err != nil {
			return err
		}
	}
	_, err = a.db.ExecContext(ctx, "UPDATE libraries SET status=?,last_scanned_at=? WHERE id=?", map[bool]string{true: "ready", false: "empty"}[processed > 0], now(), library.ID)
	if err != nil {
		return err
	}
	_, _ = a.db.ExecContext(ctx, "UPDATE jobs SET processed=?,updated_at=? WHERE id=?", processed, now(), job.ID)
	a.logger.Info("媒体扫描完成", "libraryId", library.ID, "processed", processed)
	a.notifyWorkers()
	return nil
}

// indexItem 增量更新文件元数据，用户标签、收藏和进度不随扫描覆盖。
func (a *App) indexItem(ctx context.Context, library Library, path string, size int64, modTime int64, isDir bool, token string) error {
	relative, err := filepath.Rel(library.Path, path)
	if err != nil {
		return err
	}
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if isDir {
		title = filepath.Base(path)
	}
	contentMime := fileMIME(path)
	if isDir {
		contentMime = "application/x-comic-directory"
	}
	_, err = a.db.ExecContext(ctx, `INSERT INTO items(library_id,title,type,relative_path,size,mime,added_at,updated_at,mod_time,is_dir,scan_token) VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(library_id,relative_path) DO UPDATE SET size=excluded.size,mod_time=excluded.mod_time,updated_at=CASE WHEN items.size<>excluded.size OR items.mod_time<>excluded.mod_time THEN excluded.updated_at ELSE items.updated_at END,missing=0,scan_token=excluded.scan_token,enrich_state=CASE WHEN items.size<>excluded.size OR items.mod_time<>excluded.mod_time THEN 'pending' ELSE items.enrich_state END,duration=CASE WHEN items.size<>excluded.size OR items.mod_time<>excluded.mod_time THEN 0 ELSE items.duration END,width=CASE WHEN items.size<>excluded.size OR items.mod_time<>excluded.mod_time THEN 0 ELSE items.width END,height=CASE WHEN items.size<>excluded.size OR items.mod_time<>excluded.mod_time THEN 0 ELSE items.height END`, library.ID, title, library.Type, filepath.ToSlash(relative), size, contentMime, now(), now(), modTime, isDir, token)
	if err != nil {
		return fmt.Errorf("写入媒体索引失败: %w", err)
	}
	// 缓存可以安全丢弃；成功处理过的封面若被清理，下一次显式或定期扫描会重建。
	if library.Type != "novel" {
		var itemID int64
		var enrichState string
		if queryErr := a.db.QueryRowContext(ctx, "SELECT id,enrich_state FROM items WHERE library_id=? AND relative_path=?", library.ID, filepath.ToSlash(relative)).Scan(&itemID, &enrichState); queryErr != nil {
			return queryErr
		}
		if enrichState == "done" {
			if _, statErr := os.Stat(a.cachePath(Item{ID: itemID, ModTime: modTime, Size: size})); os.IsNotExist(statErr) {
				if _, updateErr := a.db.ExecContext(ctx, "UPDATE items SET enrich_state='pending' WHERE id=? AND enrich_state='done'", itemID); updateErr != nil {
					return updateErr
				}
			}
		}
	}
	_, err = a.db.ExecContext(ctx, "INSERT OR IGNORE INTO jobs(library_id,item_id,kind,created_at,updated_at) SELECT library_id,id,'enrich',?,? FROM items WHERE library_id=? AND relative_path=? AND enrich_state='pending'", now(), now(), library.ID, filepath.ToSlash(relative))
	return err
}

// isImage 判断安全支持的栅格图片类型，避免 SVG 活动内容。
func isImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	}
	return false
}

// matchesType 将文件扩展名映射到首版支持的内容入口。
func matchesType(kind string, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch kind {
	case "image":
		return isImage(path)
	case "comic":
		return ext == ".cbz"
	case "novel":
		return ext == ".txt" || ext == ".epub"
	case "video":
		switch ext {
		case ".mp4", ".m4v", ".mkv", ".webm", ".mov", ".avi", ".ts", ".m2ts", ".mpg", ".mpeg":
			return true
		}
	}
	return false
}

// fileMIME 提供稳定媒体类型；未知封装不会假装浏览器一定支持。
func fileMIME(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mkv":
		return "video/x-matroska"
	case ".cbz":
		return "application/vnd.comicbook+zip"
	case ".epub":
		return "application/epub+zip"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".ts", ".m2ts":
		return "video/mp2t"
	}
	value := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if value == "" {
		value = "application/octet-stream"
	}
	return value
}
