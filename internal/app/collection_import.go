package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// collectionHistoricalRoots 在查询关闭后返回历史发布根，保留停用后的只读媒体可用性。
func (a *App) collectionHistoricalRoots(ctx context.Context) ([]string, error) {
	rows, err := a.db.QueryContext(ctx, "SELECT DISTINCT local_root FROM collection_tasks WHERE local_root<>'' AND kind<>'bt' LIMIT 256")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var root string
		if err = rows.Scan(&root); err != nil {
			return nil, err
		}
		if filepath.IsAbs(root) {
			out = append(out, root)
		}
	}
	return out, rows.Err()
}

// addCollectionGate 将收集产物纳入全部扫描、流播放和封面读取共用的发布门禁。
func (a *App) addCollectionGate(ctx context.Context, gate *downloadGate, roots map[string]bool) error {
	if a.config.Collection.ReadRoot != "" {
		roots[a.config.Collection.ReadRoot] = true
	}
	rows, err := a.db.QueryContext(ctx, "SELECT id,local_root,final_path,ready,files FROM collection_tasks WHERE local_root<>'' AND kind<>'bt'")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, root, final, body string
		var ready bool
		if err = rows.Scan(&id, &root, &final, &ready, &body); err != nil {
			return err
		}
		if !filepath.IsAbs(root) || final != filepath.Join(root, "complete", id) || !validDownloadUUID(id) {
			return fmt.Errorf("收集发布路径记录异常")
		}
		var files []CollectionFile
		if json.Unmarshal([]byte(body), &files) != nil || len(files) > 10000 {
			return fmt.Errorf("收集发布清单异常")
		}
		p := &downloadPublication{root: root, final: final, staging: filepath.Join(root, "staging", id), ready: ready, files: map[string]downloadGateFile{}, dirs: map[string]bool{}, loaded: true, ctx: ctx, app: a, id: "collection-" + id}
		for _, file := range files {
			if _, e := collectionRelative(file.Path); e != nil || file.Path == "" {
				return fmt.Errorf("收集发布文件路径异常")
			}
			key := downloadPathKey(filepath.FromSlash(file.Path))
			p.files[key] = downloadGateFile{size: file.Size, modTime: file.ModTime}
			for dir := filepath.Dir(filepath.FromSlash(file.Path)); dir != "."; dir = filepath.Dir(dir) {
				p.dirs[downloadPathKey(dir)] = true
			}
		}
		p.dirs[downloadPathKey(".")] = true
		roots[root] = true
		gate.tasks[p.id] = p
		gate.finals[downloadPathKey(final)] = p
	}
	return rows.Err()
}

// ensureCollectionLibrary 仅使用已授权目标，已删除的绑定库不能静默重建。
func (a *App) ensureCollectionLibrary(ctx context.Context, t *collectionTaskRecord) (Library, error) {
	if t.LibraryID > 0 {
		library, err := a.libraryByID(t.LibraryID)
		if err != nil || library.Type != t.ContentType || !within(library.Path, t.FinalPath) {
			return Library{}, fmt.Errorf("目标媒体库已删除或目录、类型不匹配，请登记原路径后重试")
		}
		return library, nil
	}
	if !t.CreateLibrary {
		return Library{}, fmt.Errorf("未授权创建目标媒体库")
	}
	path := t.FinalPath
	if t.Kind != "register" {
		path = filepath.Join(t.LocalRoot, "complete")
	}
	if _, err := a.readableDirectory(path); err != nil {
		return Library{}, fmt.Errorf("目标目录未通过只读媒体边界检查")
	}
	// 建库与持久绑定在同一短事务中完成，后续扫描失败不会产生另一份库。
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return Library{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO libraries(name,type,path) VALUES(?,?,?)", "收集 · "+map[string]string{"video": "视频", "music": "音乐", "image": "图片", "comic": "漫画", "novel": "小说"}[t.ContentType], t.ContentType, path); err != nil {
		return Library{}, err
	}
	var id int64
	if err = tx.QueryRowContext(ctx, "SELECT id FROM libraries WHERE type=? AND path=?", t.ContentType, path).Scan(&id); err != nil {
		return Library{}, err
	}
	result, err := tx.ExecContext(ctx, "UPDATE collection_tasks SET library_id=?,revision=revision+1,updated_at=? WHERE id=? AND revision=? AND library_id=0", id, now(), t.ID, t.Revision)
	if err != nil {
		return Library{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return Library{}, fmt.Errorf("目标库绑定发生冲突")
	}
	if err = tx.Commit(); err != nil {
		return Library{}, err
	}
	t.LibraryID = id
	t.Revision++
	return a.libraryByID(id)
}

// collectionImportTargets 形成实际扫描覆盖清单，目录漫画按直接图片合并。
func collectionImportTargets(t collectionTaskRecord, library Library) map[string]int64 {
	targets := map[string]int64{}
	for _, file := range t.Files {
		absolute := filepath.Join(t.FinalPath, filepath.FromSlash(file.Path))
		relative, err := filepath.Rel(library.Path, absolute)
		if err != nil || !within(library.Path, absolute) {
			continue
		}
		relative = filepath.ToSlash(relative)
		if t.ContentType == "comic" && isImage(file.Path) {
			parent := filepath.ToSlash(filepath.Dir(relative))
			targets[parent] += file.Size
		} else if matchesType(t.ContentType, file.Path) {
			targets[relative] = file.Size
		}
	}
	return targets
}

// collectionCovered 核对每个预期条目及版本，不能把无关扫描完成当作入库成功。
func (a *App) collectionCovered(ctx context.Context, library int64, targets map[string]int64, versions ...map[string]int64) ([]int64, bool, error) {
	ids := []int64{}
	if len(targets) == 0 {
		return ids, false, nil
	}
	for path, size := range targets {
		var id, actual, modTime int64
		err := a.db.QueryRowContext(ctx, "SELECT id,size,mod_time FROM items WHERE library_id=? AND relative_path=? AND missing=0", library, path).Scan(&id, &actual, &modTime)
		if err == sql.ErrNoRows || err == nil && (actual != size || len(versions) > 0 && modTime != versions[0][path]) {
			return ids, false, nil
		}
		if err != nil {
			return ids, false, err
		}
		ids = append(ids, id)
	}
	return ids, true, nil
}

// importCollectionTask 复用唯一活动扫描并核对实际条目，最多安排一次补扫。
func (a *App) importCollectionTask(ctx context.Context, t collectionTaskRecord) {
	if !t.Ready {
		return
	}
	root, relative := t.LocalRoot, "complete/"+t.ID
	if t.Kind == "register" {
		root = t.FinalPath
		relative = ""
	}
	if _, err := a.collectionValidateFiles(root, relative, t.Files, t.ContentType); err != nil {
		t.Ready = false
		_ = a.collectionSetState(ctx, &t, "reconciliation_required", "failed", "入库前文件版本复核失败，请重新预览或恢复原产物")
		return
	}
	library, err := a.ensureCollectionLibrary(ctx, &t)
	if err != nil {
		_ = a.collectionSetState(ctx, &t, "importing", "failed", "目标媒体库不可用，请检查原目录和绑定后重试入库")
		return
	}
	targets := collectionImportTargets(t, library)
	versions := map[string]int64{}
	for _, file := range t.Files {
		absolute := filepath.Join(t.FinalPath, filepath.FromSlash(file.Path))
		rel, e := filepath.Rel(library.Path, absolute)
		if e != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if t.ContentType == "comic" && isImage(file.Path) {
			rel = filepath.ToSlash(filepath.Dir(rel))
		}
		if file.ModTime > versions[rel] {
			versions[rel] = file.ModTime
		}
	}
	ids, covered, err := a.collectionCovered(ctx, library.ID, targets, versions)
	if err != nil {
		return
	}
	if covered {
		t.ItemIDs = ids
		_ = a.collectionSetState(ctx, &t, "completed", "imported", "获取与媒体入库均已确认完成")
		return
	}
	if len(targets) == 0 {
		_ = a.collectionSetState(ctx, &t, "importing", "failed", "该任务没有可扫描的目标媒体条目，文件已保留")
		return
	}
	var plan collectionPlan
	_ = json.Unmarshal([]byte(t.Plan), &plan)
	if t.JobID > 0 {
		var status string
		err = a.db.QueryRowContext(ctx, "SELECT status FROM jobs WHERE id=? AND library_id=?", t.JobID, library.ID).Scan(&status)
		if err != nil && err != sql.ErrNoRows {
			return
		}
		if status == "queued" || status == "running" {
			_ = a.collectionSetState(ctx, &t, "importing", map[bool]string{true: "scanning", false: "queued"}[status == "running"], "等待媒体扫描实际覆盖已发布文件")
			return
		}
		if status == "failed" || status == "cancelled" || plan.ScanAttempts >= 2 {
			_ = a.collectionSetState(ctx, &t, "importing", "failed", "扫描未覆盖全部产物，请检查扫描任务后单独重试入库")
			return
		}
	}
	job, err := a.enqueueScan(library.ID)
	if err != nil {
		return
	}
	plan.ScanAttempts++
	t.Plan = collectionJSON(plan)
	t.JobID = job.ID
	t.State = "importing"
	t.ImportState = "queued"
	t.Message = "已安排媒体扫描，获取阶段已完成"
	if a.persistCollectionTask(ctx, &t) == nil {
		a.collectionEvent(ctx, t.ID, "importing", t.Message)
	}
}

// collectionSafeMessage 去除来源标题中的控制字符和过长文本。
func collectionSafeMessage(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
	if len(value) > 512 {
		value = string([]rune(value)[:min(128, len([]rune(value)))])
	}
	return strings.TrimSpace(value)
}
