package collectionworker

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// retryHTTP 仅重试已停止的失败或取消操作，清理范围固定为本意图的隔离目录。
func (s *Service) retryHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, ok := s.ops[r.PathValue("id")]
	if !ok {
		failJSON(w, 404, "获取操作不存在")
		return
	}
	if _, active := s.active[saved.Operation.ID]; active {
		failJSON(w, 409, "原执行进程尚未退出，请稍后核对")
		return
	}
	if saved.Operation.State == "queued" || saved.Operation.State == "running" {
		writeJSON(w, 200, saved.Operation)
		return
	}
	if saved.Operation.State != "failed" && saved.Operation.State != "canceled" {
		failJSON(w, 409, "只有已确认停止的失败或取消操作能够重试")
		return
	}
	if saved.Attempts >= 3 {
		failJSON(w, 409, "此操作已达到三次执行上限，需要重新预览并明确新意图")
		return
	}
	base := filepath.Join(s.config.DataDir, "staging")
	target := filepath.Join(base, saved.Operation.ID)
	absolute, err := filepath.Abs(target)
	if err != nil || !within(base, absolute) || !safeOperationID(saved.Operation.ID) {
		failJSON(w, 409, "暂存目录归属无法确认")
		return
	}
	if _, err = os.Lstat(filepath.Join(s.config.DataDir, "complete", saved.Operation.ID)); !os.IsNotExist(err) {
		failJSON(w, 409, "操作存在发布目录，必须先核对结果")
		return
	}
	if info, e := os.Lstat(target); e == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			failJSON(w, 409, "暂存目录不是本操作普通目录")
			return
		}
		canonical, e := filepath.EvalSymlinks(target)
		if e != nil || !sameNativePath(absolute, canonical) {
			failJSON(w, 409, "暂存目录存在链接别名")
			return
		}
		if os.RemoveAll(absolute) != nil {
			failJSON(w, 500, "无法清理本操作失败暂存，请检查占用")
			return
		}
	} else if !os.IsNotExist(e) {
		failJSON(w, 409, "暂存目录状态无法确认")
		return
	}
	saved.Attempts++
	saved.Operation.State = "queued"
	saved.Operation.Message = "已明确重试原获取意图，等待执行"
	saved.Operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	saved.Operation.Bytes = 0
	saved.Operation.Files = saved.Operation.Files[:0]
	if err = s.saveLocked(&saved); err != nil {
		failJSON(w, 500, err.Error())
		return
	}
	s.ops[saved.Operation.ID] = saved
	s.logger.Info("资源获取任务已明确重试", "operation", saved.Operation.ID, "attempt", saved.Attempts)
	writeJSON(w, 202, saved.Operation)
}
