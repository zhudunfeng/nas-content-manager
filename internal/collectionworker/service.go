package collectionworker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	contract "nas-content-manager/internal/collectioncontract"
)

// savedOperation 私密持久记录将请求和公开响应一起原子写入专用目录。
type savedOperation struct {
	Request   contract.OperationRequest `json:"request"`   // 不公开的获准请求。
	Operation contract.Operation        `json:"operation"` // 可回显的脱敏状态。
	Preview   contract.Preview          `json:"preview"`   // 提交时锁定的来源版本与内容身份。
	Attempts  int                       `json:"attempts"`  // 本意图已明确启动的执行次数，最多三次。
}

// savedPreview 保存有效预览及其绑定输入，服务退出后需要重新预览。
type savedPreview struct {
	Request  contract.PreviewRequest // 固定参数快照。
	Response contract.Preview        // 对外预览。
	Until    time.Time               // 自动过期时间。
}

// Service 管理本实例持久操作、有限执行并发和受控网络代理。
type Service struct {
	config        Config                        // 已验证的部署参数。
	logger        *slog.Logger                  // 脱敏中文业务日志。
	token         string                        // 专用认证令牌，不输出日志。
	instanceID    string                        // 持久实例身份。
	ctx           context.Context               // 服务生命周期。
	cancel        context.CancelFunc            // 关闭准入和执行。
	mu            sync.Mutex                    // 保护操作、预览和执行句柄。
	ops           map[string]savedOperation     // 有界持久操作索引。
	previews      map[string]savedPreview       // 有界临时预览。
	active        map[string]context.CancelFunc // 当前归属操作的取消句柄。
	wg            sync.WaitGroup                // 等待全部获取和后台请求退出。
	webSlots      chan struct{}                 // 有界预览并发预算。
	proxy         *publicProxy                  // 所有网页工具请求使用的有界公共网络代理。
	ytdlp         string                        // 已探测固定网页工具。
	rclone        string                        // 已探测固定复制工具。
	ffmpeg        string                        // 已探测固定后处理工具。
	ytdlpVersion  string                        // 非私密版本说明。
	rcloneVersion string                        // 非私密版本说明。
	jmcomicPython string                        // 已解析且通过固定版本探测的受信Python。
	jmcomicReason string                        // 本地运行时不可用的脱敏中文原因，不含真实路径。
	uavPython     string                        // 已通过固定版本探测的视频插件独立解释器。
	uavReason     string                        // 视频运行时或工具不可用的脱敏中文原因，不含解释器路径或工具原文。
	lease         *workerLease                  // 专用数据根独占锁。
	closing       bool                          // 已关闭HTTP处理器准入。
	requests      sync.WaitGroup                // 已获准处理器数量，保护退出后的文件和锁释放。
	closeOnce     sync.Once                     // 保证关闭过程只执行一次。
	closeErr      error                         // 完整退出的持久错误结果。
}

// Run 启动窄协议HTTP服务，并在退出时等待自有执行进程与请求释放。
func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", config.Addr)
	if err != nil {
		return fmt.Errorf("获取服务监听失败")
	}
	return runWithListener(ctx, config, logger, listener, nil)
}

// runWithListener 接管真实监听器，在完整初始化后就绪，并保留服务与进程回收错误。
func runWithListener(ctx context.Context, config Config, logger *slog.Logger, listener net.Listener, ready func(*Service, string) error) (result error) {
	defer listener.Close()
	service, err := New(ctx, config, logger)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, service.Close()) }()
	server := &http.Server{Handler: service.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return service.ctx }}
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	if ready != nil {
		if err = ready(service, listener.Addr().String()); err != nil {
			return err
		}
	}
	service.logger.Info("资源获取服务已启动", "addr", listener.Addr().String(), "instance", service.instanceID)
	select {
	case <-ctx.Done():
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
	service.cancel()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("获取服务请求未能完整退出")
	}
	return nil
}

// New 建立独占数据根和执行器，恢复操作只对账、不自动重发不确定请求。
func New(ctx context.Context, config Config, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(config.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("无法建立获取服务数据根")
	}
	canonical, err := filepath.EvalSymlinks(config.DataDir)
	if err != nil || !sameNativePath(config.DataDir, canonical) {
		return nil, fmt.Errorf("获取服务数据根不能包含链接别名")
	}
	for _, part := range []string{"", "private-state", "staging", "complete", "probes"} {
		p := filepath.Join(config.DataDir, part)
		if err := os.MkdirAll(p, 0700); err != nil {
			return nil, fmt.Errorf("无法建立获取服务数据目录")
		}
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("获取服务数据目录不能是链接")
		}
		if err = os.Chmod(p, 0700); err != nil {
			return nil, fmt.Errorf("获取服务数据目录无法收紧权限")
		}
	}
	lease, err := lockWorker(config.DataDir)
	if err != nil {
		return nil, err
	}
	token, err := ensureToken(config.TokenFile)
	if err != nil {
		lease.Close()
		return nil, err
	}
	workCtx, cancel := context.WithCancel(ctx)
	s := &Service{config: config, logger: logger, token: token, ctx: workCtx, cancel: cancel, ops: map[string]savedOperation{}, previews: map[string]savedPreview{}, active: map[string]context.CancelFunc{}, webSlots: make(chan struct{}, 2), lease: lease}
	s.instanceID, err = loadInstanceID(config.DataDir)
	if err != nil {
		s.Close()
		return nil, err
	}
	s.initWebTools()
	s.rclone = toolPath(config.Rclone)
	s.rcloneVersion = s.version(s.rclone, "version")
	if s.rcloneVersion == "" {
		s.rclone = ""
	}
	s.proxy, err = newPublicProxy(workCtx)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("无法建立网页请求策略代理")
	}
	s.initJMComic()
	s.initUAV()
	entries, err := os.ReadDir(filepath.Join(config.DataDir, "private-state"))
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("无法恢复获取服务状态")
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || entry.IsDir() {
			continue
		}
		if len(s.ops) >= 10000 {
			s.Close()
			return nil, fmt.Errorf("获取历史超过10000条，请先按保留策略归档")
		}
		p := filepath.Join(config.DataDir, "private-state", entry.Name())
		info, statErr := os.Lstat(p)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 2<<20 {
			s.Close()
			return nil, fmt.Errorf("获取历史文件不安全")
		}
		data, readErr := os.ReadFile(p)
		var saved savedOperation
		if readErr != nil || json.Unmarshal(data, &saved) != nil || !safeOperationID(saved.Operation.ID) || saved.Request.ID != saved.Operation.ID {
			s.Close()
			return nil, fmt.Errorf("获取历史文件损坏，需要恢复备份")
		}
		if saved.Operation.State == "running" || saved.Operation.State == "canceling" {
			saved.Operation.State = "recovery_required"
			saved.Operation.Message = "执行端重启，旧操作结果待核对；未自动重新获取"
		}
		if saved.Operation.State == "publishing" {
			s.recoverPublish(&saved)
		}
		if err = s.saveLocked(&saved); err != nil {
			s.Close()
			return nil, err
		}
		s.ops[saved.Operation.ID] = saved
	}
	s.wg.Add(1)
	go s.scheduler()
	return s, nil
}

// Close 停止本实例执行并等待写句柄释放，数据和源文件保留。
func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		s.cancel()
		s.mu.Unlock()
		if s.proxy != nil {
			s.proxy.Close()
		}
		s.requests.Wait()
		s.wg.Wait()
		if s.lease != nil {
			s.closeErr = s.lease.Close()
		}
	})
	return s.closeErr
}

// Handler 注册固定接口，并使用独立令牌和严格请求预算。
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /v1/capabilities", s.capabilitiesHTTP)
	mux.HandleFunc("POST /v1/previews", s.previewHTTP)
	mux.HandleFunc("POST /v1/jmcomic/search", s.jmcomicSearchHTTP)
	mux.HandleFunc("POST /v1/uav/search", s.uavSearchHTTP)
	mux.HandleFunc("POST /v1/operations", s.startHTTP)
	mux.HandleFunc("GET /v1/operations", s.listHTTP)
	mux.HandleFunc("GET /v1/operations/{id}", s.operationHTTP)
	mux.HandleFunc("POST /v1/operations/{id}/cancel", s.cancelHTTP)
	mux.HandleFunc("POST /v1/operations/{id}/publish", s.publishHTTP)
	mux.HandleFunc("POST /v1/operations/{id}/retry", s.retryHTTP)
	mux.HandleFunc("GET /v1/probe", s.probeHTTP)
	mux.HandleFunc("DELETE /v1/probe", s.probeHTTP)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path != "/healthz" {
			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")
			if !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
				failJSON(w, 401, "获取服务认证失败")
				return
			}
		}
		s.mu.Lock()
		if s.closing || s.ctx.Err() != nil {
			s.mu.Unlock()
			failJSON(w, 503, "获取服务正在退出")
			return
		}
		s.requests.Add(1)
		s.mu.Unlock()
		defer s.requests.Done()
		ctx, cancel := context.WithCancel(r.Context())
		stop := context.AfterFunc(s.ctx, cancel)
		defer func() { stop(); cancel() }()
		r = r.WithContext(ctx)
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		mux.ServeHTTP(w, r)
	})
}

// capabilitiesHTTP 返回当前真实工具和来源能力，不自动安装或启动工具。
func (s *Service) capabilitiesHTTP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Capabilities())
}

// Capabilities 生成不含真实来源路径和秘密的执行端能力。
func (s *Service) Capabilities() contract.Capabilities {
	webReady := s.ytdlp != "" && s.ffmpeg != ""
	remoteReady := s.rclone != "" && regularPrivateConfig(s.config.RcloneConfigFile)
	webReason := ""
	if !webReady {
		webReason = "未配置可运行的yt-dlp或ffmpeg"
	}
	remoteReason := ""
	if !remoteReady {
		remoteReason = "未配置可运行的rclone及其专用配置"
	}
	sources := []contract.Source{}
	for _, src := range s.config.Sources {
		sources = append(sources, contract.Source{ID: src.ID, Kind: src.Kind, Name: src.Name})
	}
	return contract.Capabilities{Version: contract.Version, InstanceID: s.instanceID, Capabilities: []contract.Capability{{Kind: "web", Available: webReady, Reason: webReason, Version: s.ytdlpVersion}, {Kind: "remote", Available: remoteReady, Reason: remoteReason, Version: s.rcloneVersion}, {Kind: "local", Available: true, Reason: "", Version: "native-copy-v1"}, s.jmcomicCapability(), s.uavCapability()}, Sources: sources, MaxBytes: s.config.MaxBytes, MaxFiles: s.config.MaxFiles}
}

// startHTTP 先检查幂等记录，再在有效预览和预算约束下保存待执行意图。
func (s *Service) startHTTP(w http.ResponseWriter, r *http.Request) {
	var input contract.OperationRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !safeOperationID(input.ID) {
		failJSON(w, 400, "操作标识必须为安全UUID")
		return
	}
	previewRequest := requestPreview(input)
	if err := s.validateRequest(&previewRequest); err != nil {
		failJSON(w, 400, err.Error())
		return
	}
	input.MaxBytes = previewRequest.MaxBytes
	input.MaxFiles = previewRequest.MaxFiles
	input.Profile = previewRequest.Profile
	input.Path = previewRequest.Path
	s.mu.Lock()
	defer s.mu.Unlock()
	if saved, ok := s.ops[input.ID]; ok {
		canonicalJMComicSelection(&input, saved.Request.ChapterIDs)
		hash := requestHash(input)
		if saved.Operation.RequestHash != hash {
			failJSON(w, 409, "同一操作标识已绑定其他获取意图")
			return
		}
		writeJSON(w, 200, saved.Operation)
		return
	}
	if len(s.ops) >= 10000 {
		failJSON(w, 409, "获取历史已达上限，请按保留策略归档")
		return
	}
	p, ok := s.previews[input.PreviewID]
	if ok {
		canonicalJMComicSelection(&input, p.Request.ChapterIDs)
		previewRequest.ChapterIDs = input.ChapterIDs
	}
	if !ok || time.Now().After(p.Until) || previewHash(p.Request) != previewHash(previewRequest) {
		failJSON(w, 409, "预览已过期或参数改变，请重新预览")
		return
	}
	if input.Kind == "jmcomic" && p.Response.JMComic != nil && (p.Response.JMComic.OverBudget || p.Response.JMComic.PageCount > input.MaxFiles) {
		failJSON(w, 422, "漫画选择超过页数预算，请减少章节后重新预览")
		return
	}
	hash := requestHash(input)
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	op := contract.Operation{ID: input.ID, Kind: input.Kind, State: "queued", Message: "已登记获取意图，等待执行", CreatedAt: stamp, UpdatedAt: stamp, RelativeDir: "staging/" + input.ID, Files: []contract.File{}, RequestHash: hash}
	saved := savedOperation{Request: input, Operation: op, Preview: p.Response, Attempts: 1}
	if err := s.saveLocked(&saved); err != nil {
		failJSON(w, 500, err.Error())
		return
	}
	s.ops[input.ID] = saved
	s.logger.Info("资源获取任务已登记", "operation", input.ID, "kind", input.Kind)
	writeJSON(w, 202, op)
}

// operationHTTP 返回操作事实，未找到的标识不会创建新任务。
func (s *Service) operationHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	saved, ok := s.ops[r.PathValue("id")]
	s.mu.Unlock()
	if !ok {
		failJSON(w, 404, "获取操作不存在")
		return
	}
	writeJSON(w, 200, saved.Operation)
}

// listHTTP 返回有界操作快照，客户端可通过稳定标识继续对账。
func (s *Service) listHTTP(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 100 {
			failJSON(w, 400, "列表数量需要在1至100之间")
			return
		}
		limit = n
	}
	s.mu.Lock()
	ops := make([]contract.Operation, 0, len(s.ops))
	for _, saved := range s.ops {
		ops = append(ops, saved.Operation)
	}
	s.mu.Unlock()
	sort.Slice(ops, func(i, j int) bool { return ops[i].CreatedAt > ops[j].CreatedAt })
	if len(ops) > limit {
		ops = ops[:limit]
	}
	writeJSON(w, 200, ops)
}

// cancelHTTP 持久记录取消意图，执行句柄退出前不报告取消完成。
func (s *Service) cancelHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, ok := s.ops[r.PathValue("id")]
	if !ok {
		failJSON(w, 404, "获取操作不存在")
		return
	}
	if saved.Operation.State == "published" || saved.Operation.State == "publishing" {
		failJSON(w, 409, "产物已发布或正在发布，不能撤销获取")
		return
	}
	if saved.Operation.State == "recovery_required" {
		failJSON(w, 409, "旧执行结果尚未确认，不能通过取消绕过对账")
		return
	}
	if cancel, active := s.active[saved.Operation.ID]; active {
		saved.Operation.State = "canceling"
		saved.Operation.Message = "正在停止本操作执行进程"
		if err := s.saveLocked(&saved); err != nil {
			failJSON(w, 500, err.Error())
			return
		}
		s.ops[saved.Operation.ID] = saved
		cancel()
	} else {
		saved.Operation.State = "canceled"
		saved.Operation.Message = "已取消，源文件和已取得数据保留"
		if err := s.saveLocked(&saved); err != nil {
			failJSON(w, 500, err.Error())
			return
		}
		s.ops[saved.Operation.ID] = saved
	}
	writeJSON(w, 200, saved.Operation)
}

// scheduler 仅启动已持久登记的排队操作，不恢复结果不确定的执行。
func (s *Service) scheduler() {
	defer s.wg.Done()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			for id, saved := range s.ops {
				if len(s.active) >= s.config.MaxConcurrent {
					break
				}
				if saved.Operation.State != "queued" {
					continue
				}
				ctx, cancel := context.WithCancel(s.ctx)
				saved.Operation.State = "running"
				saved.Operation.Message = "正在获取资源"
				if err := s.saveLocked(&saved); err != nil {
					cancel()
					continue
				}
				s.ops[id] = saved
				s.active[id] = cancel
				s.wg.Add(1)
				go s.execute(ctx, saved)
			}
			s.mu.Unlock()
		}
	}
}

// execute 在锁外执行I/O，返回后按当前取消意图提交事实。
func (s *Service) execute(ctx context.Context, saved savedOperation) {
	defer s.wg.Done()
	id := saved.Operation.ID
	files, err := s.acquire(ctx, saved)
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.ops[id]
	cancel := s.active[id]
	delete(s.active, id)
	if cancel != nil {
		cancel()
	}
	if err == nil && current.Operation.State != "canceling" {
		current.Operation.State = "acquired"
		current.Operation.Message = "获取完成，等待控制面验证发布"
		current.Operation.Files = files
		for _, f := range files {
			current.Operation.Bytes += f.Size
		}
	}
	wasCanceling := current.Operation.State == "canceling"
	if err != nil {
		current.Operation.State = "failed"
		current.Operation.Message = safeExecutionError(err)
	}
	if wasCanceling || errors.Is(err, context.Canceled) {
		current.Operation.State = "canceled"
		current.Operation.Message = "操作已取消，源文件和暂存数据保留"
	}
	if s.ctx.Err() != nil && err != nil {
		current.Operation.State = "recovery_required"
		current.Operation.Message = "获取服务退出，保留数据并等待显式对账"
	}
	if saveErr := s.saveLocked(&current); saveErr != nil {
		s.logger.Error("资源获取状态保存失败", "operation", id)
		return
	}
	s.ops[id] = current
	s.logger.Info("资源获取阶段已结束", "operation", id, "state", current.Operation.State)
}

// saveLocked 在专属目录原子落盘意图和状态，调用方必须持有操作锁。
func (s *Service) saveLocked(saved *savedOperation) error {
	saved.Operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("获取状态无法编码")
	}
	return atomicFile(filepath.Join(s.config.DataDir, "private-state", saved.Operation.ID+".json"), data)
}

// atomicFile 写入有界临时文件、同步并原子替换，仅用于本实例私密元数据。
func atomicFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".pending-")
	if err != nil {
		return fmt.Errorf("获取状态无法写入")
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return fmt.Errorf("获取状态无法持久化")
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("获取状态无法原子提交")
	}
	return nil
}

// loadInstanceID 返回持久实例标识，目录归属不会随服务重启改变。
func loadInstanceID(root string) (string, error) {
	p := filepath.Join(root, "private-state", "instance")
	data, err := os.ReadFile(p)
	if err == nil && safeID.MatchString(strings.TrimSpace(string(data))) {
		return strings.TrimSpace(string(data)), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("无法读取执行端身份")
	}
	id := rand.Text()
	if err = atomicFile(p, []byte(id)); err != nil {
		return "", err
	}
	return id, nil
}

// safeOperationID 只允许UUID形状的幂等标识，不允许路径字符。
func safeOperationID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= 'a' && c <= 'f') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// requestPreview 从不可变获取意图抽取预览绑定字段。
func requestPreview(r contract.OperationRequest) contract.PreviewRequest {
	return contract.PreviewRequest{Kind: r.Kind, URL: r.URL, Profile: r.Profile, SourceID: r.SourceID, Path: r.Path, MaxBytes: r.MaxBytes, MaxFiles: r.MaxFiles, AlbumID: r.AlbumID, UAVSite: r.UAVSite, VideoID: r.VideoID, ChapterIDs: r.ChapterIDs}
}

// canonicalJMComicSelection 在集合完全相同时复用已冻结的上游顺序，不改变全本的旧意图。
func canonicalJMComicSelection(input *contract.OperationRequest, frozen []string) {
	if input.Kind != "jmcomic" || len(input.ChapterIDs) == 0 || len(input.ChapterIDs) != len(frozen) {
		return
	}
	seen := make(map[string]bool, len(input.ChapterIDs))
	for _, id := range input.ChapterIDs {
		seen[id] = true
	}
	for _, id := range frozen {
		if !seen[id] {
			return
		}
	}
	input.ChapterIDs = append([]string{}, frozen...)
}

// requestHash 不把短期预览令牌作为业务意图身份。
func requestHash(r contract.OperationRequest) string {
	r.PreviewID = ""
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// previewHash 对规范化的输入预算和来源版本计算不可逆摘要。
func previewHash(r contract.PreviewRequest) string {
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// decodeJSON 严格限制字段及单一JSON对象，不接受尾随数据。
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		failJSON(w, 400, "请求内容或字段无效")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		failJSON(w, 400, "请求包含多余内容")
		return false
	}
	return true
}

// writeJSON 用统一JSON和显式状态响应窄协议。
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// failJSON 返回安全中文错误，不能直接回显工具stderr。
func failJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, contract.Error{Error: message})
}
