package enginehelper

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"nas-content-manager/internal/enginecontract"
)

var (
	// errConflict 表示归属、版本或幂等参数冲突。
	errConflict = errors.New("状态或归属冲突，请刷新后核对配置；不会覆盖已有资源")
	// errBusy 表示已有不同组件集合正在部署。
	errBusy = errors.New("已有初始化任务执行中，请等待完成")
)

// Service 持有独立助手状态、单执行器和受控Docker客户端。
type Service struct {
	config  Config          // 不可变受信任部署配置。
	store   *store          // 独立SQLite状态库。
	docker  *dockerClient   // 本机Docker专用客户端。
	logger  *slog.Logger    // 仅记录受控中文业务日志。
	ctx     context.Context // 独立于浏览器请求的生命周期。
	queue   chan string     // 全局唯一执行器的有界任务通道。
	opMu    sync.Mutex      // 只保护短幂等提交，不跨网络调用。
	wg      sync.WaitGroup  // 等待任务在退出时停止。
	running string          // opMu保护的执行器当前任务，防止旧worker未退出时retry。
	lease   *os.File        // 在打开状态库和恢复任务前取得的跨进程独占锁。
}

// New 创建助手私有状态，恢复已授权的未完成任务；不自动启动已完成引擎。
func New(ctx context.Context, c Config, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := checkDirectoryTree(c.Root); err != nil {
		return nil, err
	}
	for _, dir := range []struct {
		name string
		mode os.FileMode
		uid  int
		gid  int
	}{{"state", 0700, 0, 0}, {"connections", 0750, c.UID, c.GID}} {
		if err := ensureDirectory(filepath.Join(c.Root, dir.name), dir.mode, dir.uid, dir.gid); err != nil {
			return nil, err
		}
	}
	// Unix监听检查晚于状态恢复，必须先以持久目录锁阻止第二个进程重复执行。
	lease, err := acquireLease(filepath.Join(c.Root, "state", "helper.lock"))
	if err != nil {
		return nil, err
	}
	keepLease := false
	defer func() {
		if !keepLease {
			_ = lease.Close()
		}
	}()
	db, err := openStore(ctx, filepath.Join(c.Root, "state", "state.db"))
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(filepath.Join(c.Root, "state", "state.db"), 0600); err != nil {
		db.db.Close()
		return nil, errors.New("助手状态库权限设置失败")
	}
	s := &Service{config: c, store: db, docker: newDocker(c.DockerSocket), logger: logger, ctx: ctx, queue: make(chan string, 1), lease: lease}
	active, err := db.activeTask(ctx)
	if err != nil {
		db.db.Close()
		return nil, err
	}
	s.wg.Add(1)
	go s.worker()
	if active != nil {
		s.queue <- active.ID
		logger.Info("恢复未完成的引擎初始化任务", "task", active.ID)
	}
	keepLease = true
	return s, nil
}

// Close 等待执行器释放后关闭独立状态库；调用方须先取消生命周期。
func (s *Service) Close() error {
	s.wg.Wait()
	s.docker.http.CloseIdleConnections()
	err := s.store.db.Close()
	if s.lease != nil {
		err = errors.Join(err, s.lease.Close())
	}
	return err
}

// Run 在私有Unix socket启动助手，socket没有任何宿主TCP映射。
func Run(ctx context.Context, c Config, logger *slog.Logger) error {
	if runtime.GOOS != "linux" {
		return errors.New("引擎部署助手仅支持Linux")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s, err := New(ctx, c, logger)
	if err != nil {
		return err
	}
	defer func() { cancel(); _ = s.Close() }()
	parent := filepath.Dir(c.Socket)
	if err = checkDirectoryTree(parent); err != nil {
		return err
	}
	if info, statErr := os.Lstat(c.Socket); statErr == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("助手socket路径被其他文件占用")
		}
		conn, dialErr := net.DialTimeout("unix", c.Socket, time.Second)
		if dialErr == nil {
			conn.Close()
			return errors.New("助手socket已有服务运行")
		}
		if err = os.Remove(c.Socket); err != nil {
			return errors.New("无法清理已失效的助手socket")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return errors.New("助手socket路径不可访问")
	}
	listener, err := net.Listen("unix", c.Socket)
	if err != nil {
		return errors.New("无法监听助手私有socket")
	}
	defer listener.Close()
	defer os.Remove(c.Socket)
	if err = os.Chown(c.Socket, 0, c.GID); err != nil {
		return errors.New("助手socket组权限设置失败")
	}
	if err = os.Chmod(c.Socket, 0660); err != nil {
		return errors.New("助手socket权限设置失败")
	}
	server := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		case <-done:
		}
	}()
	s.logger.Info("引擎助手已启动，控制接口仅使用私有socket")
	err = server.Serve(listener)
	close(done)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Handler 暴露固定协议路由，绝无Docker透传、exec或日志代理。
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/engines", s.handleStatus)
	mux.HandleFunc("POST /api/engines/probe", s.handleProbe)
	mux.HandleFunc("POST /api/engines/initializations", s.handleInitialize)
	mux.HandleFunc("GET /api/engines/initializations/{id}", s.handleTask)
	mux.HandleFunc("POST /api/engines/initializations/{id}/retry", s.handleRetry)
	mux.HandleFunc("POST /api/engines/{component}/start", s.handleStart)
	return mux
}

// verifyMount 检查助手内部根与Docker宿主根映射，拒绝把内部路径误传给daemon。
func (s *Service) verifyMount(ctx context.Context) error {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return errors.New("助手容器身份不可验证")
	}
	self, err := s.docker.inspect(ctx, hostname)
	if err != nil || self == nil {
		return errors.New("助手必须在本机Docker容器内运行，且使用默认容器hostname")
	}
	found := false
	for _, m := range self.Mounts {
		if m.Destination == s.config.Root {
			if m.Type != "bind" || !m.RW || m.Source != s.config.HostRoot {
				return errors.New("ENGINE_HOST_ROOT与助手专用bind source不一致")
			}
			found = true
		}
	}
	if !found {
		return errors.New("助手缺少专用根挂载，请应用托管Compose配置")
	}
	return nil
}

// preflight 验证Docker、网络及宿主映射，不持有业务锁或数据库事务。
func (s *Service) preflight(ctx context.Context) error {
	if err := s.docker.preflight(ctx, s.config); err != nil {
		return err
	}
	if err := s.verifyMount(ctx); err != nil {
		return err
	}
	return checkDirectoryTree(s.config.Root)
}

// status 只读取Docker元数据；只有明确404才显示未安装。
func (s *Service) status(ctx context.Context) enginecontract.Status {
	result := enginecontract.Status{Helper: "ready", Message: "部署助手可用", Components: []enginecontract.Component{}, NoticeVersion: enginecontract.NoticeVersion, NoticeText: enginecontract.NoticeText, SetupPort: s.config.SetupPort}
	preErr := s.preflight(ctx)
	if preErr != nil {
		result.Helper = "unavailable"
		if strings.Contains(preErr.Error(), "不支持") || strings.Contains(preErr.Error(), "不兼容") {
			result.Helper = "unsupported"
		}
		result.Message = preErr.Error()
	}
	active, activeErr := s.store.activeTask(ctx)
	if activeErr == nil && active != nil {
		result.ActiveTaskID = active.ID
	}
	manifest, manifestErr := s.loadManifest()
	revision := int64(1)
	if manifestErr == nil {
		revision = manifest.Generation + 1
	}
	for _, id := range []string{enginecontract.QBittorrent, enginecontract.Jackett} {
		item := enginecontract.Component{ID: id, Mode: "managed", Presence: "unknown", Runtime: "unknown", Readiness: "not_configured", Message: "等待环境检测", Revision: revision}
		if preErr == nil {
			t, _ := engineTemplate(id)
			info, err := s.docker.inspect(ctx, containerName(s.store.installation, id))
			if err != nil {
				item.Message = err.Error()
			} else if info == nil {
				if err = s.ownedConfig(id, false); err != nil {
					item.Presence = "conflict"
					item.Readiness = "error"
					item.Message = "专用配置目录包含未知内容，请人工核对"
				} else {
					item.Presence = "missing"
					item.Message = "引擎未安装，等待用户初始化"
					item.CanInitialize = active == nil
				}
			} else if err = validateContainer(info, s.config, s.store.installation, t); err != nil {
				item.Presence = "conflict"
				item.Readiness = "error"
				item.Message = err.Error()
			} else {
				item.Presence = "present"
				item.Runtime = "stopped"
				item.Readiness = "connecting"
				item.Message = "引擎已停止，可启动原容器"
				item.CanStart = active == nil
				if info.State.Running {
					item.Runtime = "running"
					item.CanStart = false
					item.Message = "引擎已运行，等待应用验证连接"
					if manifestErr == nil && ((id == enginecontract.QBittorrent && manifest.QBGeneration > 0) || (id == enginecontract.Jackett && manifest.JackettGeneration > 0)) {
						item.Readiness = "connecting"
					}
				}
			}
		} else {
			item.Message = result.Message
		}
		if active != nil {
			for _, step := range active.Steps {
				if step.Component == id && step.State != "completed" {
					item.Readiness = "initializing"
					item.Message = step.Message
				}
			}
		}
		result.Components = append(result.Components, item)
	}
	return result
}

// handleStatus 返回无敏感连接数据的当前检测结果。
func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, s.status(r.Context()))
}

// handleProbe 允许显式只读探针，但不接受任何上游地址。
func (s *Service) handleProbe(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if !decode(w, r, &request) {
		return
	}
	s.handleStatus(w, r)
}

// submit 在短锁内保存请求与任务身份，不把浏览器取消传播至执行器。
func (s *Service) submit(ctx context.Context, requestID string, signature string, components []string) (enginecontract.Task, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if old, err := s.store.binding(ctx, requestID, signature); err != nil {
		return enginecontract.Task{}, err
	} else if old != nil {
		return *old, nil
	}
	active, err := s.store.activeTask(ctx)
	if err != nil {
		return enginecontract.Task{}, err
	}
	if active != nil {
		if strings.Join(active.Components, ",") != strings.Join(components, ",") || active.Action != strings.Split(signature, "|")[0] {
			return enginecontract.Task{}, errBusy
		}
		if err = s.store.alias(ctx, requestID, signature, active.ID); err != nil {
			return enginecontract.Task{}, err
		}
		return *active, nil
	}
	task, err := s.store.insertTask(ctx, requestID, signature, components)
	if err != nil {
		return task, err
	}
	select {
	case s.queue <- task.ID:
	default:
	}
	return task, nil
}

// handleInitialize 仅接受固定组件及用户显式告知确认。
func (s *Service) handleInitialize(w http.ResponseWriter, r *http.Request) {
	var request enginecontract.InitializationRequest
	if !decode(w, r, &request) {
		return
	}
	components, err := canonicalComponents(request.Components)
	if err != nil || !safeID(request.RequestID) {
		failure(w, 400, "初始化参数无效")
		return
	}
	for _, c := range components {
		if c == enginecontract.QBittorrent && (!request.AcceptNotice || request.NoticeVersion != enginecontract.NoticeVersion) {
			failure(w, 409, "请先阅读并主动确认当前qBittorrent首次使用告知")
			return
		}
	}
	task, err := s.submit(r.Context(), request.RequestID, taskSignature("initialize", components, request.NoticeVersion+"|"+strconv.FormatBool(request.AcceptNotice)), components)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := s.store.response(r.Context(), task.ID, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	s.logger.Info("已接收引擎初始化请求", "task", task.ID)
	respond(w, 202, result)
}

// handleTask 支持重开页面恢复进度，不接受路径片段或负游标。
func (s *Service) handleTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	after := int64(0)
	var err error
	if value := r.URL.Query().Get("afterSequence"); value != "" {
		after, err = strconv.ParseInt(value, 10, 64)
	}
	if !safeID(id) || after < 0 || err != nil {
		failure(w, 400, "任务标识或游标无效")
		return
	}
	result, err := s.store.response(r.Context(), id, after)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, 200, result)
}

// handleRetry 只重排失败步骤，已成功组件和其凭据完全保留。
func (s *Service) handleRetry(w http.ResponseWriter, r *http.Request) {
	var request enginecontract.ActionRequest
	if !decode(w, r, &request) {
		return
	}
	id := r.PathValue("id")
	if !safeID(id) || !safeID(request.RequestID) || request.Revision < 1 {
		failure(w, 400, "重试参数无效")
		return
	}
	signature := fmt.Sprintf("retry|%s|%d", id, request.Revision)
	s.opMu.Lock()
	old, err := s.store.binding(r.Context(), request.RequestID, signature)
	if err == nil && old == nil {
		var task enginecontract.Task
		task, err = s.store.getTask(r.Context(), id)
		if err == nil {
			if task.Revision != request.Revision || task.State != "failed" && task.State != "interrupted" || s.running == id {
				err = errConflict
			} else if active, activeErr := s.store.activeTask(r.Context()); activeErr != nil {
				err = activeErr
			} else if active != nil {
				err = errBusy
			} else {
				for i := range task.Steps {
					if task.Steps[i].State != "completed" {
						task.Steps[i].State = "queued"
						task.Steps[i].Phase = "preflight"
						task.Steps[i].Message = "等待重试环境检查"
						task.Steps[i].Attempt++
						task.Steps[i].Layers = []enginecontract.Layer{}
					}
				}
				task.State = "queued"
				task.Message = "失败组件已安排重试"
				err = s.store.saveRetry(r.Context(), &task, request.RequestID, signature)
				if err == nil {
					select {
					case s.queue <- id:
					default:
					}
				}
			}
		}
	}
	s.opMu.Unlock()
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := s.store.response(r.Context(), id, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, 202, result)
}

// handleStart 只允许启动已核对归属的原停止容器，不创建替代实例。
func (s *Service) handleStart(w http.ResponseWriter, r *http.Request) {
	var request enginecontract.ActionRequest
	if !decode(w, r, &request) {
		return
	}
	component := r.PathValue("component")
	if _, err := engineTemplate(component); err != nil || !safeID(request.RequestID) {
		failure(w, 400, "启动参数无效")
		return
	}
	signature := fmt.Sprintf("start|%s|%d", component, request.Revision)
	if old, err := s.store.binding(r.Context(), request.RequestID, signature); err != nil {
		writeError(w, err)
		return
	} else if old != nil {
		result, resultErr := s.store.response(r.Context(), old.ID, 0)
		if resultErr != nil {
			writeError(w, resultErr)
			return
		}
		respond(w, 202, result)
		return
	}
	snapshot := s.status(r.Context())
	allowed := false
	for _, c := range snapshot.Components {
		if c.ID == component && c.CanStart && c.Revision == request.Revision {
			allowed = true
		}
	}
	if !allowed {
		writeError(w, errConflict)
		return
	}
	task, err := s.submit(r.Context(), request.RequestID, signature, []string{component})
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := s.store.response(r.Context(), task.ID, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, 202, result)
}

// decode 严格限制私有请求体与未知字段，拒绝尾随JSON。
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			failure(w, 413, "请求超过64KiB预算")
		} else {
			failure(w, 400, "请求JSON无效或包含未知字段")
		}
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		failure(w, 400, "请求包含多余内容")
		return false
	}
	return true
}

// respond 输出明确JSON类型与安全DTO。
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// failure 输出既有中文错误协议。
func failure(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}

// writeError 映射受控错误，数据库内部错误永不直接交给客户端。
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errConflict):
		failure(w, 409, errConflict.Error())
	case errors.Is(err, errBusy):
		failure(w, 429, errBusy.Error())
	case errors.Is(err, sql.ErrNoRows):
		failure(w, 404, "初始化任务不存在")
	default:
		failure(w, 503, "助手状态暂不可用，请稍后重试")
	}
}

// engineURL 只构造当前安装固定网络的固定端口地址。
func (s *Service) engineURL(t template) string {
	return "http://" + containerName(s.store.installation, t.Component) + ":" + strconv.Itoa(t.Port)
}

// escapedName 返回确定容器名称的安全Docker路径片段。
func (s *Service) escapedName(component string) string {
	return url.PathEscape(containerName(s.store.installation, component))
}
