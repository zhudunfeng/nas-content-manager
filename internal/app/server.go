package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nas-content-manager/internal/webassets"
)

// App 持有服务资源和有限后台任务，不保存媒体文件内容。
type App struct {
	engines       *engineRuntime               // 独立控制面与托管连接快照，助手离线不停止已就绪数据面。
	config        Config                       // 已验证配置。
	db            *sql.DB                      // 本地数据库连接。
	logger        *slog.Logger                 // 中文结构化业务日志。
	ctx           context.Context              // 应用生命周期。
	mu            sync.Mutex                   // 活跃任务与登录速率锁。
	cancels       map[int64]context.CancelFunc // 可取消的任务句柄。
	loginAttempts map[string]loginAttempt      // 有界登录限速记录。
	accountLimits accountReauthLimiter         // 独立账号再认证预算，不被登录或换会话清除。
	wake          chan struct{}                // 合并任务唤醒通知。
	wg            sync.WaitGroup               // 等待后台任务退出。
	cacheMu       sync.Mutex                   // 保证缓存预算检查与发布串行。
	cleanupErr    error                        // 由 mu 保护的资源收尾错误，退出时必须如实返回。
	backupMu      sync.Mutex                   // 共用快照生成、校验、下载、关闭和清理的唯一互斥。
	streams       chan struct{}                // 限制并发大文件流数量。
	readerSlots   chan struct{}                // 限制同时解压漫画或解析文本的请求数。
	downloads     *downloadManager             // 独立下载协调器，不占用媒体扫描执行器。
	torrentSearch *torrentSearchManager        // 只保存有限搜索快照和独立网络限额的搜种适配器。
	collections   *collectionManager           // 独立资源收集协调器，全部执行受应用生命周期控制。
	frontend      fs.FS                        // 本实例固定的前端资源根，发布嵌入与显式目录共用同一路由。
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
	return RunWithOptions(ctx, config, logger, RunOptions{})
}

// RunOptions 提供平台入口已保留的监听器和一次性真实就绪回调。
type RunOptions struct {
	Listener net.Listener         // 可选的已绑定监听器；交给运行时后所有成功或失败路径均由运行时关闭。
	Ready    func(address string) // 数据库、网页入口和本实例 HTTP 均可用后回调一次，参数为真实监听地址。
}

// RunWithOptions 在数据库恢复前取得数据锁，并统一管理 HTTP、工作线程与平台入口生命周期。
func RunWithOptions(ctx context.Context, config Config, logger *slog.Logger, options RunOptions) (runErr error) {
	listener := options.Listener
	defer func() {
		if listener != nil {
			_ = listener.Close()
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if logger == nil {
		logger = slog.Default()
	}
	lease, dataDir, err := acquireInstance(config.DataDir)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, lease.Close()) }()
	config.DataDir = dataDir
	// 先保留真实 listener，绑定失败不能触发数据库迁移或后台任务恢复。
	if listener == nil {
		listener, err = net.Listen("tcp", config.Addr)
		if err != nil {
			return fmt.Errorf("媒体库监听地址不可用：%w", err)
		}
	}
	frontend, err := frontendFiles(config)
	if err != nil {
		return err
	}
	if options.Ready != nil {
		if err = checkFrontend(frontend); err != nil {
			return err
		}
	}
	db, err := openDatabase(ctx, config.DataDir)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, db.Close()) }()
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	a := &App{config: config, db: db, logger: logger, ctx: workCtx, cancels: make(map[int64]context.CancelFunc), loginAttempts: make(map[string]loginAttempt), wake: make(chan struct{}, 1), streams: make(chan struct{}, 16), readerSlots: make(chan struct{}, 4), frontend: frontend}
	a.torrentSearch = newTorrentSearchManager(config.TorrentSearch)
	defer a.torrentSearch.http.CloseIdleConnections()
	a.startDownloads()
	a.startEngineRuntime()
	a.startWorkers()
	a.startCollections()
	defer a.collections.client.CloseIdleConnections()
	readyToken := rand.Text()
	routes := a.routes()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 仅回显本实例启动探针的随机标识，避免地址被意外复用后把第三方健康页判为就绪。
		if r.URL.Path == "/healthz" && r.Header.Get("X-Qiku-Ready") == readyToken {
			w.Header().Set("X-Qiku-Ready", readyToken)
		}
		routes.ServeHTTP(w, r)
	})
	requests := &requestLifecycle{}
	server := &http.Server{Addr: config.Addr, Handler: requests.wrap(handler), BaseContext: func(net.Listener) context.Context { return workCtx }, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
	// 不设置全局 WriteTimeout，避免长视频被短响应超时截断。
	serveDone := make(chan struct{})
	var serveErr error
	go func() {
		serveErr = server.Serve(listener)
		close(serveDone)
	}()
	defer func() {
		logger.Info("正在停止媒体库服务，取消后台任务并释放活动连接")
		requests.stop()
		cancel()
		// 退出不等待超大媒体传输自然结束；关闭本实例连接使阻塞写入立即返回。
		if closeErr := server.Close(); closeErr != nil {
			logger.Warn("媒体库连接关闭发生错误", "error", closeErr)
			runErr = errors.Join(runErr, closeErr)
		}
		<-serveDone
		// Serve 返回不代表 handler 已完成；先等待文件 defer，再关闭数据库和数据锁。
		requests.wait()
		a.wg.Wait()
		a.mu.Lock()
		runErr = errors.Join(runErr, a.cleanupErr)
		a.mu.Unlock()
		logger.Info("媒体库请求与后台任务已停止")
	}()
	address := listener.Addr().String()
	if options.Ready != nil {
		if err = waitRuntimeReady(ctx, address, readyToken, serveDone); err != nil {
			select {
			case <-serveDone:
				if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					return serveErr
				}
			default:
			}
			return err
		}
	}
	logger.Info("媒体库服务已启动", "addr", address, "version", Version, "workers", config.Workers)
	if options.Ready != nil {
		if err = ctx.Err(); err != nil {
			return err
		}
		options.Ready(address)
	}
	select {
	case <-ctx.Done():
	case <-serveDone:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
	}
	return nil
}

// waitRuntimeReady 使用无代理的本机探针验证真实服务与数据库，失败不会通知桌面打开网页。
func waitRuntimeReady(ctx context.Context, address string, token string, serveDone <-chan struct{}) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("媒体库就绪探针需要 TCP 监听地址")
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
		if ip != nil && ip.To4() == nil {
			host = "::1"
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-serveDone:
			return fmt.Errorf("媒体库 HTTP 服务未能启动")
		case <-probeCtx.Done():
			return fmt.Errorf("媒体库未在启动预算内就绪：%w", probeCtx.Err())
		default:
		}
		request, requestErr := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/healthz", nil)
		if requestErr != nil {
			return fmt.Errorf("媒体库就绪地址无效")
		}
		request.Header.Set("X-Qiku-Ready", token)
		response, probeErr := client.Do(request)
		if probeErr == nil {
			ready := response.StatusCode == http.StatusOK && response.Header.Get("X-Qiku-Ready") == token
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
			_ = response.Body.Close()
			if ready {
				return nil
			}
		}
		select {
		case <-serveDone:
			return fmt.Errorf("媒体库 HTTP 服务未能启动")
		case <-probeCtx.Done():
			return fmt.Errorf("媒体库未在启动预算内就绪：%w", probeCtx.Err())
		case <-ticker.C:
		}
	}
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
	a.downloadRoutes(mux)
	a.torrentSearchRoutes(mux)
	a.collectionRoutes(mux)
	a.engineRoutes(mux)
	a.accountRoutes(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "接口不存在") })
	mux.HandleFunc("/", a.static)
	return a.security(mux)
}

// security 拒绝跨站写入、限制请求体，并提供默认安全响应头。
func (a *App) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 统一收尾忽略或只读了一部分的小正文，成功与拒绝响应遵循相同的连接边界。
		defer discardUnreadSmallBody(w, r)
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
			bodyLimit := int64(64 << 10)
			// 仅种子上传开放 8 MiB 文件及有限表单开销，其他写接口保持原限额。
			mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if r.Method == http.MethodPost && r.URL.Path == "/api/downloads" && mediaType == "multipart/form-data" {
				bodyLimit += 8 << 20
			}
			selectionPath := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/downloads/"), "/")
			if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/downloads/") && len(selectionPath) == 2 && selectionPath[0] != "" && selectionPath[1] == "selection" {
				// 两万个文件索引的完整选集仍受独立额度约束。
				bodyLimit = 256 << 10
			}
			r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
		}
		next.ServeHTTP(w, r)
	})
}

// discardUnreadSmallBody 有界消费已知小请求体的剩余内容，使关闭连接时仍可交付响应。
func discardUnreadSmallBody(w http.ResponseWriter, r *http.Request) {
	// 只兼容已知不超过普通 JSON 限额的正文；不等待大文件、未知长度或 100-continue。
	if r.Body == nil || r.ContentLength <= 0 || r.ContentLength > 64<<10 || r.Header.Get("Expect") != "" {
		return
	}
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return
	}
	// 原始正文流保留已消费位置；EOF也表示handler已读完，不能据此误判连接损坏。
	// 不解析或保留剩余数据，也不允许未发送完正文的客户端占满默认30秒读取预算。
	if _, err := io.CopyN(io.Discard, r.Body, r.ContentLength); err != nil && !errors.Is(err, io.EOF) {
		w.Header().Set("Connection", "close")
		// 不清除失败预算，否则net/http后续关闭正文时可能再次等待未发完的数据。
		_ = controller.SetReadDeadline(time.Now())
		return
	}
	// 成功请求恢复预算；HTTP服务会在下一请求重新设置读取截止时间。
	_ = controller.SetReadDeadline(time.Time{})
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
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			fail(w, 413, "请求内容超过大小限制")
		} else {
			fail(w, 400, "请求内容格式不正确")
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			fail(w, 413, "请求内容超过大小限制")
		} else {
			fail(w, 400, "请求必须为单个 JSON 对象")
		}
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

// frontendFiles 使显式资源目录优先，发布包默认使用与工作目录无关的嵌入资源。
func frontendFiles(config Config) (fs.FS, error) {
	if !config.WebDirExplicit {
		if embedded := webassets.Files(); embedded != nil {
			return embedded, nil
		}
	}
	directory := config.WebDir
	if directory == "" {
		directory = "web/dist"
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("前端资源目录无效")
	}
	return os.DirFS(root), nil
}

// checkFrontend 在桌面报告就绪前确认本地网页入口是可读取的普通文件。
func checkFrontend(files fs.FS) error {
	entry, err := files.Open("index.html")
	if err != nil {
		return fmt.Errorf("前端资源尚未构建或安装包缺少网页入口")
	}
	defer entry.Close()
	info, err := entry.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("前端网页入口无效，请修复安装包")
	}
	return nil
}

// static 统一提供嵌入或目录资源、HEAD 与 SPA 入口，缺失脚本不会伪装为 HTML。
func (a *App) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		fail(w, 405, "不支持的请求方法")
		return
	}
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00") {
		fail(w, 404, "页面不存在")
		return
	}
	files := a.frontend
	if files == nil {
		var err error
		files, err = frontendFiles(a.config)
		if err != nil {
			fail(w, 503, "前端资源尚未构建")
			return
		}
	}
	info, err := fs.Stat(files, name)
	if err != nil || info.IsDir() {
		if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
			if name == "index.html" && r.URL.Path == "/" {
				fail(w, 503, "前端资源尚未构建，请先构建前端或修复安装包")
				return
			}
			fail(w, 404, "页面资源不存在")
			return
		}
		name = "index.html"
	}
	if info, err = fs.Stat(files, name); err != nil || !info.Mode().IsRegular() {
		fail(w, 503, "前端资源尚未构建，请先构建前端或修复安装包")
		return
	}
	http.ServeFileFS(w, r, files, name)
}

// settings 返回非敏感运行信息，便于诊断目录和媒体处理能力。
func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	_, ffmpegErr := a.mediaTool("ffmpeg")
	_, ffprobeErr := a.mediaTool("ffprobe")
	respond(w, 200, map[string]any{"version": Version, "dataDir": a.config.DataDir, "mediaRoots": a.config.MediaRoots, "scanIntervalMinutes": a.config.ScanIntervalMinutes, "cacheMaxBytes": a.config.CacheMaxBytes, "ffmpegAvailable": ffmpegErr == nil, "ffprobeAvailable": ffprobeErr == nil})
}
