package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// App 持有服务资源和有限后台任务，不保存媒体文件内容。
type App struct {
	config        Config                       // 已验证配置。
	db            *sql.DB                      // 本地数据库连接。
	logger        *slog.Logger                 // 中文结构化业务日志。
	ctx           context.Context              // 应用生命周期。
	mu            sync.Mutex                   // 活跃任务与登录速率锁。
	cancels       map[int64]context.CancelFunc // 可取消的任务句柄。
	loginAttempts map[string]loginAttempt      // 有界登录限速记录。
	wake          chan struct{}                // 合并任务唤醒通知。
	wg            sync.WaitGroup               // 等待后台任务退出。
	cacheMu       sync.Mutex                   // 保证缓存预算检查与发布串行。
	backupMu      sync.Mutex                   // 一次只生成一个一致性备份。
	streams       chan struct{}                // 限制并发大文件流数量。
	readerSlots   chan struct{}                // 限制同时解压漫画或解析文本的请求数。
}

// loginAttempt 保存单个来源的短期失败次数。
type loginAttempt struct {
	Count int       // 当前窗口的失败次数。
	Until time.Time // 计数失效时间。
}

// userContextKey 隔离会话上下文键，避免字符串冲突。
type userContextKey struct{}

// Run 启动 HTTP 与后台执行器，并执行有序关闭。
func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	db, err := openDatabase(ctx, config.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	a := &App{config: config, db: db, logger: logger, ctx: workCtx, cancels: make(map[int64]context.CancelFunc), loginAttempts: make(map[string]loginAttempt), wake: make(chan struct{}, 1), streams: make(chan struct{}, 16), readerSlots: make(chan struct{}, 4)}
	a.startWorkers()
	server := &http.Server{Addr: config.Addr, Handler: a.routes(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
	// 不设置全局 WriteTimeout，避免长视频被短响应超时截断。
	errorsCh := make(chan error, 1)
	go func() { errorsCh <- server.ListenAndServe() }()
	logger.Info("媒体库服务已启动", "addr", config.Addr, "version", Version, "workers", config.Workers)
	select {
	case <-ctx.Done():
	case err = <-errorsCh:
		if !errors.Is(err, http.ErrServerClosed) {
			cancel()
			a.wg.Wait()
			return err
		}
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err = server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
	}
	a.wg.Wait()
	logger.Info("媒体库服务已安全停止")
	return nil
}

// routes 注册原生 HTTP 路由，所有媒体接口均经过统一鉴权。
func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/status", a.status)
	mux.HandleFunc("POST /api/setup", a.setup)
	mux.HandleFunc("POST /api/login", a.login)
	for pattern, handler := range map[string]http.HandlerFunc{
		"POST /api/logout": a.logout, "GET /api/libraries": a.listLibraries, "POST /api/libraries": a.createLibrary,
		"DELETE /api/libraries/{id}": a.deleteLibrary, "POST /api/libraries/{id}/scan": a.scanLibrary, "GET /api/browse": a.browse,
		"GET /api/jobs": a.listJobs, "POST /api/jobs/{id}/cancel": a.cancelJob, "POST /api/jobs/{id}/retry": a.retryJob,
		"GET /api/items": a.listItems, "GET /api/items/{id}": a.getItem, "POST /api/items/{id}/favorite": a.favorite,
		"PUT /api/items/{id}/tags": a.tags, "PUT /api/items/{id}/progress": a.progress, "GET /api/stats": a.stats,
		"GET /api/items/{id}/file": a.file, "GET /api/items/{id}/cover": a.cover,
		"GET /api/items/{id}/comic": a.comic, "GET /api/items/{id}/pages/{index}": a.comicPage,
		"GET /api/items/{id}/text": a.text, "GET /api/settings": a.settings, "GET /api/backup": a.backup,
	} {
		mux.HandleFunc(pattern, a.auth(handler))
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "接口不存在") })
	mux.HandleFunc("/", a.static)
	return a.security(mux)
}

// security 拒绝跨站写入、限制请求体，并提供默认安全响应头。
func (a *App) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				a.logger.Error("请求处理异常", "path", r.URL.Path, "error", fmt.Sprint(value))
				fail(w, 500, "服务暂时不可用")
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; media-src 'self' blob:; frame-src 'self' blob:; connect-src 'self' blob:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			origin := r.Header.Get("Origin")
			foreign := r.Header.Get("Sec-Fetch-Site") == "cross-site"
			if origin != "" {
				parsed, err := url.Parse(origin)
				foreign = foreign || err != nil || parsed.Host != r.Host || (parsed.Scheme != "http" && parsed.Scheme != "https")
			}
			if foreign {
				fail(w, 403, "不允许跨站修改请求")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		}
		next.ServeHTTP(w, r)
	})
}

// auth 验证持久会话并将安全用户信息放入请求上下文。
func (a *App) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := a.sessionUser(r)
		if user == nil {
			fail(w, 401, "请先登录")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	}
}

// currentUser 获取已由鉴权中间件验证的用户。
func currentUser(r *http.Request) *User {
	user, _ := r.Context().Value(userContextKey{}).(*User)
	return user
}

// decodeJSON 严格解析单个受大小限制的 JSON 对象。
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		fail(w, 415, "请使用 JSON 请求")
		return false
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(w, 400, "请求内容格式不正确或超过大小限制")
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		fail(w, 400, "请求必须为单个 JSON 对象")
		return false
	}
	return true
}

// respond 返回统一 UTF-8 JSON 响应。
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// fail 返回可读的中文错误，不向客户端暴露内部堆栈。
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}

// requestID 解析正整数路径标识。
func requestID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "内容编号无效")
		return 0, false
	}
	return id, true
}

// health 仅在数据库可用时报告服务就绪。
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if a.db.PingContext(r.Context()) != nil {
		fail(w, 503, "数据库暂时不可用")
		return
	}
	respond(w, 200, map[string]string{"status": "ok"})
}

// static 在已构建前端目录中提供资源及客户端路由入口。
func (a *App) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		fail(w, 405, "不支持的请求方法")
		return
	}
	clean := filepath.Clean(filepath.FromSlash(r.URL.Path))
	root, err := filepath.Abs(a.config.WebDir)
	if err != nil {
		fail(w, 503, "前端资源尚未构建")
		return
	}
	file := filepath.Join(root, strings.TrimLeft(clean, "/\\"))
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		fail(w, 404, "页面不存在")
		return
	}
	info, err := os.Stat(file)
	if err != nil || info.IsDir() {
		file = filepath.Join(root, "index.html")
	}
	if _, err = os.Stat(file); err != nil {
		fail(w, 503, "前端资源尚未构建，请先执行 npm run build")
		return
	}
	http.ServeFile(w, r, file)
}

// settings 返回非敏感运行信息，便于诊断目录和媒体处理能力。
func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	_, ffmpegErr := exec.LookPath("ffmpeg")
	_, ffprobeErr := exec.LookPath("ffprobe")
	respond(w, 200, map[string]any{"version": Version, "dataDir": a.config.DataDir, "mediaRoots": a.config.MediaRoots, "scanIntervalMinutes": a.config.ScanIntervalMinutes, "cacheMaxBytes": a.config.CacheMaxBytes, "ffmpegAvailable": ffmpegErr == nil, "ffprobeAvailable": ffprobeErr == nil})
}
