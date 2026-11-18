package app

import (
	"net/http"
	"sync"
)

// requestLifecycle 在关闭期间停止接纳业务请求，并等待在途请求释放资源。
type requestLifecycle struct {
	mu       sync.Mutex     // 串行接纳请求和关闭屏障，避免 Add 与 Wait 竞争。
	stopping bool           // 已进入关闭阶段，后续请求不能再访问应用资源。
	active   sync.WaitGroup // 已接纳且尚未完成全部 defer 的请求。
}

// wrap 将请求收尾计入应用生命周期，不改变原始 ResponseWriter 的流式能力。
func (lifecycle *requestLifecycle) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lifecycle.mu.Lock()
		if lifecycle.stopping {
			lifecycle.mu.Unlock()
			fail(w, http.StatusServiceUnavailable, "媒体库正在退出")
			return
		}
		lifecycle.active.Add(1)
		lifecycle.mu.Unlock()
		defer lifecycle.active.Done()
		next.ServeHTTP(w, r)
	})
}

// stop 阻止新的业务请求进入，之后调用 wait 不会与新增任务竞争。
func (lifecycle *requestLifecycle) stop() {
	lifecycle.mu.Lock()
	lifecycle.stopping = true
	lifecycle.mu.Unlock()
}

// wait 等待已接纳请求完成文件、临时备份和数据库资源收尾。
func (lifecycle *requestLifecycle) wait() {
	lifecycle.active.Wait()
}
