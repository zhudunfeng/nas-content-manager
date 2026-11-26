package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"nas-content-manager/internal/app"
)

// controlReply 是固定本机控制协议，不包含凭据、命令或任意跳转 URL。
type controlReply struct {
	State             string `json:"state"`                       // starting、ready、stopping、stopped 或 failed。
	Mode              string `json:"mode"`                        // native 或 docker。
	URL               string `json:"url,omitempty"`               // 本实例真实就绪后的回环地址。
	Error             string `json:"error,omitempty"`             // 脱敏中文错误说明。
	CollectionMode    string `json:"collectionMode,omitempty"`    // 当前本机获取方式。
	CollectionState   string `json:"collectionState,omitempty"`   // disabled、external、starting、ready、unavailable 或 failed。
	CollectionMessage string `json:"collectionMessage,omitempty"` // 不含凭据的漫画组件中文诊断。
}

// desktopCommand 串行传送本机托盘的受控配置变更。
type desktopCommand struct {
	Kind   string  // restart 或 config，不允许执行任意命令。
	Config *Config // 经过本机 UI 选择的新配置；nil 表示仅重启。
}

// controller 串行管理共享核心，保证旧数据库句柄退出后才启动新实例。
type controller struct {
	mu             sync.RWMutex        // 状态和配置快照锁。
	config         Config              // 已保存的当前启动配置。
	profile        string              // 当前模式的本机配置根。
	options        Options             // 本次交互与诊断参数。
	logger         *slog.Logger        // 有界中文日志。
	reply          controlReply        // 供托盘和本机 IPC 读取的状态。
	commands       chan desktopCommand // 有界串行生命周期指令。
	cancel         context.CancelFunc  // 取消整个启动器。
	serviceCancel  context.CancelFunc  // 取消当前原生或容器监视服务。
	serviceDone    chan error          // 当前共享核心退出结果。
	collection     *managedCollection  // 本实例托管的获取服务；外接服务永不归属此字段。
	collectionDone <-chan struct{}     // 托管子树真实退出信号；与媒体服务独立。
}

// Main 执行本机启动流程；返回码由薄平台入口传给操作系统。
func Main(args []string) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	o, err := ParseOptions(args, os.Stdout)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return reportFailure(o.Headless, err)
	}
	profile, err := profilePath(o)
	if err != nil {
		return reportFailure(o.Headless, err)
	}
	if o.Control != "" {
		reply, controlErr := sendControl(profile, o.Control, 5*time.Second)
		if controlErr != nil {
			return reportFailure(true, controlErr)
		}
		_ = json.NewEncoder(os.Stdout).Encode(reply)
		if reply.Error != "" {
			return 1
		}
		return 0
	}
	if o.PrintConfig {
		c, _, configErr := loadConfig(profile, o.Mode)
		if configErr == nil {
			c, configErr = applyOptions(c, o)
		}
		if configErr != nil {
			return reportFailure(true, configErr)
		}
		_ = json.NewEncoder(os.Stdout).Encode(c)
		return 0
	}
	guard, first, err := acquireInstance(profile)
	if err != nil {
		return reportFailure(o.Headless, err)
	}
	defer guard.Close()
	if !first {
		if o.Mode == "docker" && o.DockerAction == "stop" {
			return reportFailure(o.Headless, errors.New("此 Docker 配置的启动器正在运行；请从托盘选择完整停止，或先退出启动器后执行 --docker-action stop"))
		}
		action := "open"
		if o.Mode == "docker" && o.DockerAction == "status" {
			action = "status"
		}
		reply, controlErr := sendControl(profile, action, 15*time.Second)
		err = controlErr
		if err != nil {
			return reportFailure(o.Headless, err)
		}
		if action == "status" {
			_ = json.NewEncoder(os.Stdout).Encode(reply)
			if reply.Error != "" {
				return 1
			}
		}
		return 0
	}
	if err = privateDirectory(profile); err != nil {
		return reportFailure(o.Headless, err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	var bridgeMu sync.RWMutex
	var active *controller
	stopIPC, err := serveControl(ctx, profile, func(action string) controlReply {
		bridgeMu.RLock()
		current := active
		bridgeMu.RUnlock()
		if current == nil {
			return controlReply{State: "starting", Mode: o.Mode}
		}
		return current.handleControl(action)
	}, cancel)
	if err != nil {
		return reportFailure(o.Headless, err)
	}
	defer stopIPC()
	c, fresh, err := loadConfig(profile, o.Mode)
	if err == nil {
		c, err = applyOptions(c, o)
	}
	if err != nil {
		return reportFailure(o.Headless, err)
	}
	if fresh && o.Mode == "docker" && o.DockerAction != "start" {
		return reportFailure(o.Headless, errors.New("尚无此 Docker 模式的本机配置，请先完成显式启动与初始化"))
	}
	if fresh && len(c.MediaRoots) == 0 && !o.Headless {
		showMessage("首次启动", "请选择允许栖库读取的媒体文件夹。取消后将使用独立的空媒体目录，之后可从托盘添加授权目录。", false)
		root, pickerErr := chooseFolder(0, "选择授权媒体目录")
		if pickerErr != nil {
			return reportFailure(false, pickerErr)
		}
		if root != "" {
			c.MediaRoots = []string{root}
		}
	}
	if ctx.Err() != nil {
		return 0
	}
	if len(c.MediaRoots) == 0 {
		root := filepath.Join(profile, "media")
		if err = os.MkdirAll(root, 0700); err != nil {
			return reportFailure(o.Headless, errors.New("默认媒体目录无法创建"))
		}
		c.MediaRoots = []string{root}
	}
	c, err = validateConfig(c)
	if err == nil && c.Mode == "native" {
		err = validateLocalDataDir(c.DataDir)
	}
	if err != nil {
		return reportFailure(o.Headless, err)
	}
	if err = saveConfig(profile, c); err != nil {
		return reportFailure(o.Headless, err)
	}
	var mirror io.Writer
	if o.Headless {
		mirror = os.Stderr
	}
	logger, closer, err := newLogger(profile, mirror)
	if err != nil {
		return reportFailure(o.Headless, errors.New("无法创建本机诊断日志"))
	}
	defer closer.Close()
	ctrl := &controller{config: c, profile: profile, options: o, logger: logger, commands: make(chan desktopCommand, 4), cancel: cancel, reply: controlReply{State: "starting", Mode: c.Mode}}
	bridgeMu.Lock()
	active = ctrl
	bridgeMu.Unlock()
	result := make(chan error, 1)
	go func() { result <- ctrl.run(ctx) }()
	if !o.Headless {
		if err = runTray(ctx, ctrl); err != nil {
			logger.Error("托盘运行失败", "error", err)
			cancel()
		}
	} else {
		<-ctx.Done()
	}
	cancel()
	runErr := <-result
	if runErr != nil {
		logger.Error("启动器已停止", "error", runErr)
		return 1
	}
	if err != nil {
		return 1
	}
	return 0
}

// reportFailure 输出明确中文错误，交互模式同时保留可见提示。
func reportFailure(headless bool, err error) int {
	fmt.Fprintln(os.Stderr, "栖库启动失败："+err.Error())
	if !headless {
		showMessage("栖库启动失败", err.Error(), true)
	}
	return 1
}

// snapshot 复制状态，避免 GUI、服务和 IPC 的数据竞争。
func (c *controller) snapshot() (Config, controlReply) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	copyConfig := c.config
	copyConfig.MediaRoots = append([]string(nil), c.config.MediaRoots...)
	return copyConfig, c.reply
}

// setState 原子发布真实阶段，不在失败时保留可点击的旧地址。
func (c *controller) setState(state string, address string, err error) {
	c.mu.Lock()
	c.reply.State, c.reply.Mode, c.reply.URL, c.reply.Error = state, c.config.Mode, address, ""
	if err != nil {
		c.reply.Error = err.Error()
	}
	c.mu.Unlock()
}

// handleControl 仅接受固定动作，不允许远程扩展目录授权或命令执行。
func (c *controller) handleControl(action string) controlReply {
	_, reply := c.snapshot()
	switch action {
	case "status":
	case "open":
		if reply.State == "ready" && !c.options.Headless {
			go c.openPage()
		}
	case "quit":
		// 由管道服务收到客户端固定确认后取消，避免丢失退出响应。
	case "collection-enable", "collection-disable", "collection-check":
		cfg, _ := c.snapshot()
		if cfg.Mode != "native" {
			reply.Error = "本机漫画托管只适用于原生模式"
			break
		}
		if action == "collection-enable" {
			cfg.CollectionMode = "managed"
		}
		if action == "collection-disable" {
			cfg.CollectionMode = "disabled"
		}
		c.request(desktopCommand{Kind: "config", Config: &cfg})
	default:
		reply.Error = "不支持的本机控制动作"
	}
	return reply
}

// request 合并过密的托盘操作，服务关闭和重启始终串行。
func (c *controller) request(command desktopCommand) {
	select {
	case c.commands <- command:
	default:
		showMessage("操作进行中", "正在处理上一次启动或目录变更，请稍后重试。", false)
	}
}

// run 顺序完成启动、受控重启和有序退出，不进行无限自动重试。
func (c *controller) run(ctx context.Context) error {
	var lastErr error
	start := func() {
		lastErr = c.start(ctx)
		if cancellationOnly(lastErr) && ctx.Err() != nil {
			lastErr = nil
			return
		}
		if lastErr != nil {
			c.setState("failed", "", lastErr)
			c.logger.Error("本机实例启动失败", "error", lastErr)
			if c.options.Headless {
				c.cancel()
			} else {
				go showMessage("栖库启动失败", lastErr.Error()+"\n可从托盘查看日志后重试。", true)
			}
		}
	}
	start()
	for {
		select {
		case <-ctx.Done():
			return errors.Join(lastErr, c.stop())
		case command := <-c.commands:
			if err := c.stop(); err != nil {
				lastErr = err
				c.setState("failed", "", err)
				c.logger.Error("本机服务未能完整停止，已拒绝重启", "error", err)
				continue
			}
			if command.Kind == "docker-stop" {
				lastErr = c.stopDocker(ctx)
				if lastErr != nil {
					c.setState("failed", "", lastErr)
					c.logger.Error("停止完整模式失败", "error", lastErr)
					if !c.options.Headless {
						go showMessage("停止完整模式失败", lastErr.Error(), true)
					}
				}
				continue
			}
			if command.Config != nil {
				if err := saveConfig(c.profile, *command.Config); err != nil {
					c.setState("failed", "", err)
					continue
				}
				c.mu.Lock()
				c.config = *command.Config
				c.mu.Unlock()
			}
			start()
		case err := <-c.serviceDone:
			c.serviceDone = nil
			if ctx.Err() != nil {
				if cancellationOnly(err) {
					err = nil
				}
				return errors.Join(lastErr, err, c.stop())
			}
			if err == nil {
				err = errors.New("服务意外停止，请查看日志后重新启动")
			}
			lastErr = err
			c.setState("failed", "", err)
			c.logger.Error("服务已停止", "error", err)
			if c.options.Headless {
				c.cancel()
			} else {
				go showMessage("栖库服务已停止", err.Error(), true)
			}
		case <-c.collectionDone:
			c.collectionDone = nil
			c.setCollectionState("failed", "本机漫画获取服务已退出，已有媒体仍可阅读；请查看日志后重新检测")
			c.logger.Warn("本机获取服务意外退出，已降级漫画获取；未自动重启或重发任务")
		}
	}
}

// cancellationOnly 仅识别全部叶子均为正常取消的错误，不能掩盖同时发生的资源清理失败。
func cancellationOnly(err error) bool {
	if err == context.Canceled {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		found := false
		for _, child := range joined.Unwrap() {
			if child == nil {
				continue
			}
			if !cancellationOnly(child) {
				return false
			}
			found = true
		}
		return found
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return cancellationOnly(wrapped.Unwrap())
	}
	return false
}

// start 将桌面配置交给共享核心校验，真实健康与网页就绪后才发布地址。
func (c *controller) start(ctx context.Context) error {
	cfg, _ := c.snapshot()
	c.setState("starting", "", nil)
	if cfg.Mode == "docker" {
		return c.startDocker(ctx, cfg)
	}
	collectionValues, collectionReason := c.startCollection(ctx, cfg)
	listener, port, err := bindLoopback(cfg.Port)
	if err != nil {
		return errors.Join(err, c.stopCollection())
	}
	cfg.Port = port
	values := map[string]string{"NAS_ADDR": listener.Addr().String(), "NAS_DATA_DIR": cfg.DataDir, "NAS_MEDIA_ROOTS": mediaRootList(cfg.MediaRoots)}
	for key, value := range collectionValues {
		values[key] = value
	}
	core, err := app.LoadConfigWithLookup(func(key string) string {
		if value, ok := values[key]; ok {
			return value
		}
		return os.Getenv(key)
	})
	if err != nil {
		_ = listener.Close()
		return errors.Join(err, c.stopCollection())
	}
	if core.Collection.WorkerURL == "" {
		core.Collection.UnavailableReason = collectionReason
	}
	if err = saveConfig(c.profile, cfg); err != nil {
		_ = listener.Close()
		return errors.Join(err, c.stopCollection())
	}
	c.mu.Lock()
	c.config = cfg
	c.mu.Unlock()
	serviceCtx, cancel := context.WithCancel(ctx)
	c.serviceCancel = cancel
	c.serviceDone = make(chan error, 1)
	done := c.serviceDone
	ready := make(chan string, 1)
	go func() {
		done <- app.RunWithOptions(serviceCtx, core, c.logger, app.RunOptions{Listener: listener, Ready: func(address string) {
			select {
			case ready <- address:
			default:
			}
		}})
	}()
	select {
	case address := <-ready:
		url := "http://" + address
		if err = verifyReady(serviceCtx, url); err != nil {
			return errors.Join(err, c.stop())
		}
		c.setState("ready", url, nil)
		c.logger.Info("本机媒体库已就绪", "模式", "原生媒体", "地址", url)
		if cfg.AutoOpen && !c.options.NoBrowser && !c.options.Headless {
			go c.openPage()
		}
		return nil
	case err = <-done:
		c.serviceDone = nil
		cancel()
		_ = listener.Close()
		if err == nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			err = errors.New("服务在就绪前已停止")
		}
		return errors.Join(err, c.stopCollection())
	case <-ctx.Done():
		return errors.Join(ctx.Err(), c.stop())
	case <-time.After(45 * time.Second):
		return errors.Join(errors.New("服务启动超时，请检查日志、数据目录权限与工具文件"), c.stop())
	}
}

// stop 等待共享核心关闭数据库和工作协程，不强杀其他用户进程。
func (c *controller) stop() error {
	if c.serviceCancel == nil {
		return c.stopCollection()
	}
	c.setState("stopping", "", nil)
	c.serviceCancel()
	if c.serviceDone != nil {
		select {
		case err := <-c.serviceDone:
			c.serviceDone = nil
			c.serviceCancel = nil
			if err != nil && !cancellationOnly(err) {
				return errors.Join(err, c.stopCollection())
			}
		case <-time.After(30 * time.Second):
			// 主服务取消已发出；即使其排空超时，也先完成本实例可选进程的退出屏障。
			return errors.Join(errors.New("服务仍在有序关闭，已拒绝重启；请等待进程退出后重试"), c.stopCollection())
		}
	}
	c.serviceCancel = nil
	if err := c.stopCollection(); err != nil {
		return err
	}
	c.setState("stopped", "", nil)
	return nil
}

// verifyReady 有界验证健康和本地 HTML；调用前 listener 已由本实例独占。
func verifyReady(ctx context.Context, address string) error {
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	for _, path := range []string{"/healthz", "/"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, address+path, nil)
		if err != nil {
			return errors.New("本机就绪地址无效")
		}
		res, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("本机服务尚未通过真实就绪检查")
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, 512<<10))
		_ = res.Body.Close()
		if readErr != nil || res.StatusCode != http.StatusOK {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("本机健康或网页入口未就绪，请检查日志和资源包")
		}
		if path == "/healthz" {
			var health struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(data, &health) != nil || health.Status != "ok" {
				return errors.New("健康检查响应不匹配")
			}
		}
		if path == "/" && (!strings.Contains(res.Header.Get("Content-Type"), "text/html") || !strings.Contains(strings.ToLower(string(data)), "<html")) {
			return errors.New("前端资源缺失，请使用完整发布包")
		}
	}
	return nil
}

// openPage 仅打开当前已验证状态中的本实例回环网页。
func (c *controller) openPage() {
	_, reply := c.snapshot()
	if reply.State != "ready" {
		showMessage("媒体库尚未就绪", "请稍后重试；启动失败时可从托盘查看日志。", false)
		return
	}
	if err := openLocalURL(reply.URL); err != nil {
		c.logger.Warn("无法打开默认浏览器")
		showMessage("无法打开浏览器", "请手动访问："+reply.URL, false)
	}
}

// updateRoots 校验新增或删除的授权根后安排保存与重启。
func (c *controller) updateRoots(roots []string) {
	cfg, _ := c.snapshot()
	if len(roots) == 0 {
		showMessage("保留授权目录", "至少需要保留一个授权目录。可以改为一个空文件夹。", false)
		return
	}
	cfg.MediaRoots = roots
	validated, err := validateConfig(cfg)
	if err != nil {
		showMessage("授权目录无效", err.Error(), true)
		return
	}
	if cfg.Mode == "docker" && len(validated.MediaRoots) != 1 {
		showMessage("Docker 目录设置", "当前容器配置支持一个只读媒体根。请在该目录下整理子目录，或使用独立原生模式授权多个目录。", false)
		return
	}
	c.logger.Info("本机用户更新授权目录", "目录数量", len(roots))
	c.request(desktopCommand{Kind: "config", Config: &validated})
}

// loopbackPort 从自有监听器地址提取经过实际绑定的端口。
func loopbackPort(listener net.Listener) (int, error) {
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(port)
}
