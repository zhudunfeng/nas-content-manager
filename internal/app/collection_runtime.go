package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"nas-content-manager/internal/collectioncontract"
)

// startCollections 为所有收集任务安装单个有限协调器，退出时遵循应用生命周期。
func (a *App) startCollections() {
	a.collections = &collectionManager{wake: make(chan struct{}, 1), client: &http.Client{Timeout: 25 * time.Second, Transport: &http.Transport{Proxy: nil, MaxIdleConns: 4, MaxConnsPerHost: 4}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	a.wg.Add(1)
	a.collections.externalBusy = map[string]bool{}
	a.collections.externalSlots = make(chan struct{}, 4)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		nextRSS := time.Now()
		for {
			if a.ctx.Err() != nil {
				return
			}
			tasks, err := a.collectionTaskList(a.ctx)
			if err == nil {
				for _, task := range tasks {
					if a.ctx.Err() != nil {
						return
					}
					if task.State == "failed" || task.State == "reconciliation_required" {
						continue
					}
					a.runCollectionTask(a.ctx, task)
				}
			}
			if !time.Now().Before(nextRSS) {
				a.scheduleCollectionRSS(a.ctx)
				a.verifyCollectionPublished(a.ctx)
				nextRSS = time.Now().Add(time.Minute)
			}
			select {
			case <-a.ctx.Done():
				return
			case <-a.collections.wake:
			case <-ticker.C:
			}
		}
	}()
}

// collectionSetState 保存状态变化并记录一次脱敏中文阶段事件。
func (a *App) collectionSetState(ctx context.Context, t *collectionTaskRecord, state string, importState string, message string) error {
	if t.State == state && t.ImportState == importState && t.Message == message {
		return nil
	}
	previous := t.State
	t.State = state
	t.ImportState = importState
	t.Message = message
	err := a.persistCollectionTask(ctx, t)
	if err == nil && (previous != state || state == "failed") {
		a.collectionEvent(ctx, t.ID, state, message)
	}
	return err
}

// runCollectionTask 先对账稳定操作编号，再提交或发布，不能因超时盲目创建新操作。
func (a *App) runCollectionTask(ctx context.Context, t collectionTaskRecord) {
	if t.Kind == "bt" {
		a.runCollectionBT(ctx, t)
		return
	}
	if t.State == "canceling" {
		if t.Kind == "register" {
			_ = a.collectionSetState(ctx, &t, "canceled", t.ImportState, "收集已取消，原文件与已有索引保留")
			return
		}
		var op collectioncontract.Operation
		status, err := a.collectionWorkerCall(ctx, "POST", "/v1/operations/"+t.ID+"/cancel", struct{}{}, &op)
		if err != nil && status != 404 {
			return
		}
		if err == nil && op.State != "canceled" && op.State != "failed" && op.State != "published" {
			return
		}
		t.Ready = false
		_ = a.collectionSetState(ctx, &t, "canceled", t.ImportState, "获取已取消，源文件和已完成产物保留")
		return
	}
	if t.Ready || t.State == "importing" {
		if t.ImportState != "failed" {
			a.importCollectionTask(ctx, t)
		}
		return
	}
	var plan collectionPlan
	if json.Unmarshal([]byte(t.Plan), &plan) != nil {
		_ = a.collectionSetState(ctx, &t, "failed", "pending", "收集计划损坏，请重新预览")
		return
	}
	private, err := a.readCollectionPrivate(plan.PrivateID)
	if err != nil {
		_ = a.collectionSetState(ctx, &t, "failed", "pending", "收集私密计划缺失，请恢复私密备份或重新预览")
		return
	}
	if t.Kind == "register" {
		if _, err = a.allowedPath(t.FinalPath); err != nil {
			_ = a.collectionSetState(ctx, &t, "failed", "pending", "原目录不再属于授权媒体根")
			return
		}
		total, e := a.collectionValidateFiles(t.FinalPath, "", t.Files, t.ContentType)
		if e != nil {
			_ = a.collectionSetState(ctx, &t, "failed", "pending", "原目录文件已变化或不可读，请重新预览")
			return
		}
		t.Bytes = total
		t.Ready = true
		t.State = "importing"
		t.Message = "只读文件已核验，等待媒体扫描"
		if a.persistCollectionTask(ctx, &t) == nil {
			a.collectionEvent(ctx, t.ID, "importing", t.Message)
			a.importCollectionTask(ctx, t)
		}
		return
	}
	if filepath.Clean(t.LocalRoot) != filepath.Clean(a.config.Collection.ReadRoot) || t.LocalRoot == "" {
		_ = a.collectionSetState(ctx, &t, "reconciliation_required", t.ImportState, "执行端只读根已变化，请恢复原映射后对账")
		return
	}
	var op collectioncontract.Operation
	status, err := a.collectionWorkerCall(ctx, "GET", "/v1/operations/"+t.ID, nil, &op)
	if err != nil {
		if status != 404 {
			_ = a.collectionSetState(ctx, &t, "reconciliation_required", t.ImportState, "执行端暂不可达，尚未确认提交结果；恢复连接后可重试对账")
			return
		}
		caps, readyErr := a.collectionWorkerReady(ctx)
		if readyErr != nil || caps.InstanceID != private.WorkerInstance {
			_ = a.collectionSetState(ctx, &t, "reconciliation_required", t.ImportState, "执行端目录映射尚未通过检查，请恢复连接后重试")
			return
		}
		if err = a.collectionSetState(ctx, &t, "dispatching", "pending", "获取意图已保存，正在提交执行端"); err != nil {
			return
		}
		request := private.Request
		request.ID = t.ID
		_, err = a.collectionWorkerCall(ctx, "POST", "/v1/operations", request, &op)
		if err != nil {
			_ = a.collectionSetState(ctx, &t, "reconciliation_required", t.ImportState, "执行端提交结果待确认；重试将使用原操作编号先对账")
			return
		}
	}
	expectedRequest := private.Request
	expectedRequest.ID = t.ID
	expectedRequest.PreviewID = ""
	if op.ID != t.ID || op.Kind != t.Kind || op.RequestHash != collectionDigest(collectionJSON(expectedRequest)) {
		_ = a.collectionSetState(ctx, &t, "reconciliation_required", t.ImportState, "执行端操作归属与计划不一致，已停止自动操作")
		return
	}
	if t.State == "retrying" && op.State == "failed" {
		_, err = a.collectionWorkerCall(ctx, "POST", "/v1/operations/"+t.ID+"/retry", struct{}{}, &op)
		if err != nil {
			_ = a.collectionSetState(ctx, &t, "failed", t.ImportState, "执行端未能重试该操作，请重新预览或检查来源")
			return
		}
	}
	switch op.State {
	case "queued", "running":
		_ = a.collectionSetState(ctx, &t, "acquiring", "pending", "执行端正在获取资源；源文件保持不变")
	case "failed":
		_ = a.collectionSetState(ctx, &t, "failed", "pending", "获取失败，请检查来源、工具、空间和网络后重试")
	case "canceled":
		_ = a.collectionSetState(ctx, &t, "canceled", "pending", "执行端已取消，源文件保留")
	case "recovery_required":
		_ = a.collectionSetState(ctx, &t, "reconciliation_required", "pending", "执行端恢复结果待处理，请检查其状态后重试对账")
	case "publishing":
		_ = a.collectionSetState(ctx, &t, "publishing", "pending", "执行端正在原子发布已验证产物")
	case "acquired", "published":
		expected := "staging/" + t.ID
		if op.State == "published" {
			expected = "complete/" + t.ID
		}
		if op.RelativeDir != expected {
			_ = a.collectionSetState(ctx, &t, "reconciliation_required", "pending", "执行端产物目录不符合固定归属")
			return
		}
		all := collectionWorkerFiles(op.Files)
		total, e := a.collectionValidateFiles(t.LocalRoot, expected, all, "")
		if e != nil {
			_ = a.collectionSetState(ctx, &t, "failed", "pending", "产物未通过路径、类型、大小或版本检查，已隔离保留")
			return
		}
		if op.State == "acquired" {
			if err = a.collectionSetState(ctx, &t, "publishing", "pending", "产物已核验，正在授权执行端原子发布"); err != nil {
				return
			}
			_, err = a.collectionWorkerCall(ctx, "POST", "/v1/operations/"+t.ID+"/publish", struct{}{}, &op)
			if err != nil {
				return
			}
			if op.State != "published" {
				return
			}
			if op.RelativeDir != "complete/"+t.ID {
				return
			}
			all = collectionWorkerFiles(op.Files)
			total, e = a.collectionValidateFiles(t.LocalRoot, op.RelativeDir, all, "")
			if e != nil {
				_ = a.collectionSetState(ctx, &t, "failed", "pending", "发布后产物版本复核失败，未开放读取")
				return
			}
		}
		selected := []CollectionFile{}
		for _, file := range all {
			if matchesType(t.ContentType, file.Path) {
				selected = append(selected, file)
			}
		}
		if len(selected) == 0 {
			_ = a.collectionSetState(ctx, &t, "failed", "pending", "获取结果没有所选媒体类型的文件，成品保留")
			return
		}
		t.Files = selected
		t.Bytes = total
		t.Ready = true
		t.FinalPath = filepath.Join(t.LocalRoot, "complete", t.ID)
		if err = a.collectionSetState(ctx, &t, "importing", "pending", "产物已核验发布，正在确认媒体入库"); err == nil {
			a.importCollectionTask(ctx, t)
		}
	default:
		_ = a.collectionSetState(ctx, &t, "reconciliation_required", "pending", "执行端返回未知操作状态，请检查兼容版本")
	}
}

// collectionTaskAction 保存取消、对账重试或独立入库重试意图，不删除源数据。
func (a *App) collectionTaskAction(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action   string `json:"action"`   /* cancel/retry/retry_import。 */
		Revision int64  `json:"revision"` /* 当前任务版本。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	a.collections.mu.Lock()
	defer a.collections.mu.Unlock()
	t, err := a.collectionTaskByID(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "收集任务不存在")
		return
	}
	if t.Revision != input.Revision {
		fail(w, 409, "任务版本已更新，请刷新后操作")
		return
	}
	if t.Kind == "bt" && !(input.Action == "retry" && t.DownloadID == "") {
		fail(w, 409, "BT 选文件、控制和入库重试请前往下载中心")
		return
	}
	switch input.Action {
	case "cancel":
		if t.State == "completed" || t.State == "canceled" || t.Ready {
			fail(w, 409, "已发布内容保留，请在媒体库管理索引；不能取消已完成获取")
			return
		}
		t.State = "canceling"
		t.Message = "已记录取消意图，正在等待执行端确认"
	case "retry":
		if t.State != "failed" && t.State != "reconciliation_required" {
			fail(w, 409, "当前获取状态无需重试")
			return
		}
		if t.Ready {
			fail(w, 409, "已发布任务请重试入库")
			return
		}
		t.State = "retrying"
		t.Message = "已记录重试意图，将先对账原操作"
	case "retry_import":
		if !t.Ready || t.ImportState != "failed" {
			fail(w, 409, "当前任务无需重试入库")
			return
		}
		var plan collectionPlan
		_ = json.Unmarshal([]byte(t.Plan), &plan)
		plan.ScanAttempts = 0
		t.Plan = collectionJSON(plan)
		t.ImportState = "pending"
		t.JobID = 0
		t.State = "importing"
		t.Message = "等待重新扫描已发布文件"
	default:
		fail(w, 400, "收集动作无效")
		return
	}
	if err = a.persistCollectionTask(r.Context(), &t); err != nil {
		collectionFail(w, err)
		return
	}
	a.collectionEvent(r.Context(), t.ID, t.State, t.Message)
	a.collections.collectionWake()
	respond(w, 202, map[string]any{"task": t.CollectionTask})
}

// verifyCollectionPublished 有界轮转检查全部成品，不因旧记录占满窗口而遗漏后续产物。
func (a *App) verifyCollectionPublished(ctx context.Context) {
	rows, err := a.db.QueryContext(ctx, collectionSelect+" WHERE (ready=1 AND kind<>'bt' OR kind='bt' AND download_id<>'') AND rowid>? ORDER BY rowid LIMIT 256", a.collections.verifyAfter)
	if err != nil {
		return
	}
	tasks := []collectionTaskRecord{}
	for rows.Next() {
		t, e := scanCollectionTask(rows)
		if e != nil {
			break
		}
		tasks = append(tasks, t)
	}
	_ = rows.Close()
	if len(tasks) < 256 {
		a.collections.verifyAfter = 0
	} else if err = a.db.QueryRowContext(ctx, "SELECT rowid FROM collection_tasks WHERE id=?", tasks[len(tasks)-1].ID).Scan(&a.collections.verifyAfter); err != nil {
		a.collections.verifyAfter = 0
	}
	for _, t := range tasks {
		if ctx.Err() != nil {
			return
		}
		if t.Kind == "register" {
			continue
		}
		if t.Kind == "bt" {
			a.runCollectionBT(ctx, t)
			continue
		}
		relative := "complete/" + t.ID
		if _, e := a.collectionValidateFiles(t.LocalRoot, relative, t.Files, t.ContentType); e != nil {
			t.Ready = false
			_ = a.collectionSetState(ctx, &t, "reconciliation_required", "failed", "已发布文件发生变化，读取权限已冻结，请检查执行端产物")
		}
	}
}

// collectionRuntimeError 统一将内部运行失败转换为可公开中文信息。
func collectionRuntimeError(message string) error { return fmt.Errorf("%s", message) }
