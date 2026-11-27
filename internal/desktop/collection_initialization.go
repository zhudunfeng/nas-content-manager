package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"nas-content-manager/internal/jmcomicbootstrap"
	"nas-content-manager/internal/jmcomicruntime"
	"nas-content-manager/internal/privateruntime"
	"nas-content-manager/internal/uavbootstrap"
	"nas-content-manager/internal/uavruntime"
)

// collectionInitialization 仅发布对应插件的脱敏准备阶段，不代替真实 worker 能力。
type collectionInitialization struct {
	Kind            string `json:"kind"`            // 固定插件种类 jmcomic 或 uav。
	State           string `json:"state"`           // initializing、ready 或 failed。
	Stage           string `json:"stage"`           // 不含来源地址、凭据或私有目录的中文阶段。
	DownloadedBytes int64  `json:"downloadedBytes"` // 当前材料已下载的真实字节数。
	TotalBytes      int64  `json:"totalBytes"`      // 当前材料的锁定字节数；非下载阶段为零。
}

// collectionInitializationResult 仅在批次清理屏障完成后交给串行控制循环读取。
type collectionInitializationResult struct {
	jmcomic jmcomicbootstrap.Paths // 本次通过最终路径探测的漫画依赖。
	uav     uavbootstrap.Paths     // 本次通过最终路径探测的视频依赖。
	err     error                  // 当前插件的普通失败、取消或资源清理错误。
}

// collectionInitializationTask 将固定顺序的插件准备绑定到同一个取消与清理屏障。
type collectionInitializationTask struct {
	cancel  context.CancelFunc                        // 仅取消当前 owned 批次，不操作其他 profile。
	done    chan struct{}                             // 全部结果写入且 owned 清理结束后关闭。
	kinds   []string                                  // 仅包含本轮待准备的固定插件种类。
	results map[string]collectionInitializationResult // 后台独占写，done 关闭后由控制循环读。
}

// automaticCollectionPluginEnabled 仅允许 Windows x64 中已启用且没有显式解释器的插件自动准备。
func automaticCollectionPluginEnabled(expected []string, kind string, python string) bool {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" || python != "" {
		return false
	}
	for _, candidate := range expected {
		if candidate == kind {
			return kind == "jmcomic" || kind == "uav"
		}
	}
	return false
}

// collectionPluginName 为固定插件返回中文名，未知值不得成为命令或路径。
func collectionPluginName(kind string) string {
	for _, plugin := range collectionPlugins {
		if plugin.kind == kind {
			return plugin.name
		}
	}
	return "获取"
}

// automaticPlugin 判断本次实际 worker 环境是否允许准备对应插件。
func (worker *managedCollection) automaticPlugin(kind string) bool {
	return kind == "jmcomic" && worker.autoJMComic || kind == "uav" && worker.autoUAV
}

// packagedJMComicPresent 优先使用严格通过静态校验的随包漫画运行时。
func packagedJMComicPresent(programDir string) bool {
	_, err := jmcomicruntime.Load(filepath.Join(programDir, "jmcomic-runtime.json"))
	return err == nil
}

// packagedUAVPresent 优先选择静态有效的随包运行时及普通工具，真实可用性仍由 worker 探测。
func packagedUAVPresent(programDir string) bool {
	if _, err := uavruntime.Load(filepath.Join(programDir, "uav-runtime.json")); err != nil {
		return false
	}
	for _, name := range []string{"yt-dlp.exe", "ffmpeg.exe", "ffprobe.exe"} {
		path := filepath.Join(programDir, "tools", name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || privateruntime.RejectAliases(path) != nil {
			return false
		}
	}
	return true
}

// collectionEnvironmentWithOverrides 只复制并覆盖固定的子进程路径，不改变启动快照或系统环境。
func collectionEnvironmentWithOverrides(environment []string, overrides map[string]string) []string {
	result := make([]string, 0, len(environment)+len(overrides))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if _, replace := overrides[strings.ToUpper(key)]; !replace {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

// collectionEnvironmentWithJMComicPaths 只覆盖已经完整校验的漫画清单，保留视频工具及其他隔离配置。
func collectionEnvironmentWithJMComicPaths(environment []string, paths jmcomicbootstrap.Paths) []string {
	return collectionEnvironmentWithOverrides(environment, map[string]string{
		"NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST": paths.Manifest, // 受信漫画清单的固定绝对路径。
	})
}

// collectionEnvironmentWithUAVPaths 只覆盖受信视频清单和工具，保持漫画解释器及清单独立。
func collectionEnvironmentWithUAVPaths(environment []string, paths uavbootstrap.Paths) []string {
	return collectionEnvironmentWithOverrides(environment, map[string]string{
		"PATH": filepath.Dir(paths.FFmpeg), // 子进程仅搜索已核验的私有工具目录。
		"NAS_COLLECTION_WORKER_UAV_RUNTIME_MANIFEST": paths.Manifest, // 固定视频运行时清单。
		"NAS_COLLECTION_WORKER_YTDLP":                paths.YTDLP,    // 固定版本独立下载器的受信绝对路径。
		"NAS_COLLECTION_WORKER_FFMPEG":               paths.FFmpeg,   // 同一代次离线媒体工具的受信绝对路径。
	})
}

// collectionInitializationForKind 按插件查找状态，保留原单对象 IPC 的读兼容性。
func collectionInitializationForKind(reply controlReply, kind string) *collectionInitialization {
	for index := range reply.CollectionInitializations {
		if reply.CollectionInitializations[index].Kind == kind {
			return &reply.CollectionInitializations[index]
		}
	}
	if reply.CollectionInitialization != nil && reply.CollectionInitialization.Kind == kind {
		return reply.CollectionInitialization
	}
	return nil
}

// collectionInitializationReply 仅增强对应尚不可用能力的原因，绝不把准备成功改写为实际可用。
func collectionInitializationReply(reply controlReply) controlReply {
	if reply.CollectionMode != "managed" || reply.CollectionState != "ready" && reply.CollectionState != "unavailable" {
		return reply
	}
	for index, capability := range reply.CollectionCapabilities {
		initialization := collectionInitializationForKind(reply, capability.Kind)
		if initialization != nil && !capability.Available && (initialization.State == "initializing" || initialization.State == "failed") {
			reply.CollectionCapabilities[index].Reason = initialization.Stage
		}
	}
	reply.CollectionState, reply.CollectionMessage = collectionReadiness(reply.CollectionCapabilities)
	return reply
}

// collectionInitializationLabel 为托盘展示对应插件与当前材料的实际字节数，不虚构总体百分比。
func collectionInitializationLabel(initialization *collectionInitialization) string {
	label := collectionPluginName(initialization.Kind) + "依赖 · " + initialization.Stage
	if initialization.TotalBytes > 0 {
		label += fmt.Sprintf("（%.1f / %.1f MiB）", float64(initialization.DownloadedBytes)/(1<<20), float64(initialization.TotalBytes)/(1<<20))
	}
	return label
}

// setCollectionInitializationLocked 在持锁期间更新单个插件，并选择兼容状态中最需关注的一项。
func (c *controller) setCollectionInitializationLocked(value collectionInitialization) {
	found := false
	for index := range c.reply.CollectionInitializations {
		if c.reply.CollectionInitializations[index].Kind == value.Kind {
			c.reply.CollectionInitializations[index] = value
			found = true
			break
		}
	}
	if !found {
		c.reply.CollectionInitializations = append(c.reply.CollectionInitializations, value)
	}
	selected := value
	if value.State != "initializing" {
		for _, state := range []string{"failed", "initializing"} {
			for _, candidate := range c.reply.CollectionInitializations {
				if candidate.State == state {
					selected = candidate
				}
			}
		}
	}
	c.reply.CollectionInitialization = &selected
}

// beginCollectionInitialization 在媒体真实就绪后启动一轮准备，插件失败不进行无限自动重试。
func (c *controller) beginCollectionInitialization(ctx context.Context, worker *managedCollection) {
	if ctx.Err() != nil || c.initialization != nil || c.initializationCleanupErr != nil {
		return
	}
	if c.initializationAttempted == nil {
		c.initializationAttempted = map[string]bool{}
	}
	pending := []string{}
	for _, plugin := range collectionPlugins {
		if !worker.automaticPlugin(plugin.kind) {
			continue
		}
		var capabilityAvailable bool
		var reason string
		for _, capability := range worker.capabilities {
			if capability.Kind == plugin.kind {
				capabilityAvailable, reason = capability.Available, capability.Reason
				break
			}
		}
		if c.initializationAttempted[plugin.kind] {
			activationFailed := false
			c.mu.Lock()
			previous := collectionInitializationForKind(c.reply, plugin.kind)
			if capabilityAvailable {
				c.setCollectionInitializationLocked(collectionInitialization{Kind: plugin.kind, State: "ready", Stage: plugin.name + "依赖已初始化并通过本地探测"})
			} else if previous == nil || previous.State != "failed" {
				activationFailed = true
				stage := plugin.name + "依赖载入后仍不可用"
				if reason != "" {
					stage += "：" + reason
				}
				c.setCollectionInitializationLocked(collectionInitialization{Kind: plugin.kind, State: "failed", Stage: stage + "；请查看日志并重新检测"})
			}
			c.mu.Unlock()
			if activationFailed {
				c.logger.Warn("获取插件依赖自动初始化后未通过实际载入探测", "插件", plugin.name, "原因", reason)
			}
			continue
		}
		if !capabilityAvailable {
			c.initializationAttempted[plugin.kind] = true
			pending = append(pending, plugin.kind)
		}
	}
	if len(pending) == 0 {
		return
	}
	initializationCtx, cancel := context.WithCancel(ctx)
	task := &collectionInitializationTask{cancel: cancel, done: make(chan struct{}), kinds: pending, results: map[string]collectionInitializationResult{}}
	c.initialization, c.initializationDone = task, task.done
	c.mu.Lock()
	c.initializationStatusTask = task
	for _, kind := range pending {
		c.setCollectionInitializationLocked(collectionInitialization{Kind: kind, State: "initializing", Stage: "正在等待准备" + collectionPluginName(kind) + "所需的私有依赖"})
	}
	c.mu.Unlock()
	c.logger.Info("获取插件依赖尚未就绪，已在后台按顺序自动初始化；已有媒体保持可用", "插件", strings.Join(pending, ","))
	go c.runCollectionInitialization(initializationCtx, task, func(runCtx context.Context, kind string) collectionInitializationResult {
		return c.initializeCollectionDependency(runCtx, task, worker.programDir, worker.root, kind)
	})
}

// runCollectionInitialization 串行准备独立插件，普通失败继续，取消或清理失败则阻止剩余工作。
func (c *controller) runCollectionInitialization(ctx context.Context, task *collectionInitializationTask, initialize func(context.Context, string) collectionInitializationResult) {
	defer close(task.done)
	defer task.cancel()
	var blockingErr error
	for _, kind := range task.kinds {
		result := collectionInitializationResult{}
		if ctx.Err() != nil {
			result.err = ctx.Err()
		} else if blockingErr != nil {
			result.err = errors.New("前一插件依赖尚未完整清理，已停止本轮准备")
		} else {
			result = initialize(ctx, kind)
		}
		task.results[kind] = result
		name := collectionPluginName(kind)
		status := collectionInitialization{Kind: kind, State: "ready", Stage: name + "依赖已准备完成，等待统一载入"}
		if result.err != nil {
			status.State, status.Stage = "failed", name+"依赖初始化失败："+result.err.Error()+"；请重新检测后重试"
			c.logger.Warn("获取插件依赖自动初始化失败，已有媒体保持可用，可重新检测后重试", "插件", name, "原因", result.err)
			if collectionInitializationCleanupFailed(result.err) {
				blockingErr = result.err
			}
		}
		c.reportCollectionInitialization(task, status)
	}
}

// initializeCollectionDependency 通过各自独立配方和最终路径探针准备固定插件，不共享解释器目录。
func (c *controller) initializeCollectionDependency(ctx context.Context, task *collectionInitializationTask, programDir string, root string, kind string) collectionInitializationResult {
	result := collectionInitializationResult{}
	report := func(progress uavbootstrap.Progress) {
		c.reportCollectionInitialization(task, collectionInitialization{Kind: kind, State: "initializing", Stage: progress.Stage, DownloadedBytes: progress.DownloadedBytes, TotalBytes: progress.TotalBytes})
	}
	switch kind {
	case "jmcomic":
		result.jmcomic, result.err = jmcomicbootstrap.Initialize(ctx, root, func(progress jmcomicbootstrap.Progress) {
			report(uavbootstrap.Progress{Stage: progress.Stage, DownloadedBytes: progress.DownloadedBytes, TotalBytes: progress.TotalBytes})
		}, func(probeCtx context.Context, paths jmcomicbootstrap.Paths) error {
			environment, _, err := collectionEnvironmentWithLookup(programDir, root, c.environment.lookup)
			if err != nil {
				return err
			}
			return c.probeCollectionInitialization(probeCtx, programDir, root, kind, collectionEnvironmentWithJMComicPaths(environment, paths))
		})
	case "uav":
		result.uav, result.err = uavbootstrap.Initialize(ctx, root, report, func(probeCtx context.Context, paths uavbootstrap.Paths) error {
			environment, _, err := collectionEnvironmentWithLookup(programDir, root, c.environment.lookup)
			if err != nil {
				return err
			}
			return c.probeCollectionInitialization(probeCtx, programDir, root, kind, collectionEnvironmentWithUAVPaths(environment, paths))
		})
	default:
		result.err = errors.New("未知获取插件，已拒绝自动准备")
	}
	return result
}

// reportCollectionInitialization 在短锁内发布当前批次的插件进度，旧批次不能污染新状态。
func (c *controller) reportCollectionInitialization(task *collectionInitializationTask, status collectionInitialization) {
	c.mu.Lock()
	if c.initializationStatusTask != task {
		c.mu.Unlock()
		return
	}
	previous := collectionInitializationForKind(c.reply, status.Kind)
	changed := previous == nil || previous.Stage != status.Stage
	c.setCollectionInitializationLocked(status)
	c.mu.Unlock()
	if changed {
		c.logger.Info("获取插件依赖自动初始化", "插件", collectionPluginName(status.Kind), "阶段", status.Stage)
	}
}

// collectionInitializationCleanupFailed 同时识别两个插件共享的不可忽略资源清理错误。
func collectionInitializationCleanupFailed(err error) bool {
	return errors.Is(err, uavbootstrap.ErrCleanup) || errors.Is(err, jmcomicbootstrap.ErrCleanup)
}

// completeCollectionInitialization 在串行循环消费整批结果，至多对仍有效的托管实例激活一次。
func (c *controller) completeCollectionInitialization(ctx context.Context) bool {
	task := c.initialization
	if task == nil {
		return false
	}
	<-task.done
	c.initialization, c.initializationDone = nil, nil
	task.cancel()
	c.mu.Lock()
	c.initializationStatusTask = nil
	c.mu.Unlock()
	prepared := false
	for _, kind := range task.kinds {
		result := task.results[kind]
		if result.err != nil {
			if collectionInitializationCleanupFailed(result.err) {
				c.initializationCleanupErr = errors.Join(c.initializationCleanupErr, result.err)
			}
			continue
		}
		switch kind {
		case "jmcomic":
			c.jmcomicPaths = result.jmcomic
			prepared = prepared || result.jmcomic.Manifest != ""
		case "uav":
			c.uavPaths = result.uav
			prepared = prepared || result.uav.Manifest != ""
		}
	}
	cfg, reply := c.snapshot()
	if !prepared || c.initializationCleanupErr != nil || ctx.Err() != nil || cfg.Mode != "native" || cfg.CollectionMode != "managed" || reply.State != "ready" || c.collection == nil || len(c.commands) > 0 || len(c.serviceDone) > 0 {
		return false
	}
	eligible := false
	for _, kind := range task.kinds {
		result := task.results[kind]
		if result.err == nil && c.collection.automaticPlugin(kind) && (result.jmcomic.Manifest != "" || result.uav.Manifest != "") {
			eligible = true
		}
	}
	if !eligible {
		return false
	}
	select {
	case <-c.collection.process.done:
		return false
	default:
	}
	c.logger.Info("获取插件私有依赖已完成本轮校验和离线探测，正在受控重启以统一载入")
	return true
}

// stopCollectionInitialization 先撤销状态所有权，再等待整批取消与清理，保留不可忽略的资源错误。
func (c *controller) stopCollectionInitialization() error {
	c.mu.Lock()
	c.initializationStatusTask = nil
	c.mu.Unlock()
	task := c.initialization
	if task != nil {
		task.cancel()
		<-task.done
		c.initialization, c.initializationDone = nil, nil
		c.mu.Lock()
		for _, kind := range task.kinds {
			previous := collectionInitializationForKind(c.reply, kind)
			if previous == nil || previous.State == "initializing" {
				c.setCollectionInitializationLocked(collectionInitialization{Kind: kind, State: "failed", Stage: collectionPluginName(kind) + "依赖初始化已取消，可重新检测后重试"})
			}
		}
		c.mu.Unlock()
		for _, result := range task.results {
			if collectionInitializationCleanupFailed(result.err) {
				c.initializationCleanupErr = errors.Join(c.initializationCleanupErr, result.err)
			}
		}
	}
	return c.initializationCleanupErr
}

// resetCollectionInitialization 只在用户操作完整停止服务后允许重试，不清除未解决的清理失败。
func (c *controller) resetCollectionInitialization() {
	c.jmcomicPaths = jmcomicbootstrap.Paths{}
	c.uavPaths = uavbootstrap.Paths{}
	c.initializationAttempted = nil
	c.mu.Lock()
	c.reply.CollectionInitialization = nil
	c.reply.CollectionInitializations = nil
	c.mu.Unlock()
}

// probeCollectionInitialization 使用本实例 Job 执行固定离线探针，不访问站点或继承系统 Python。
func (c *controller) probeCollectionInitialization(ctx context.Context, programDir string, root string, kind string, environment []string) error {
	if kind != "jmcomic" && kind != "uav" {
		return errors.New("未知获取插件，已拒绝离线探测")
	}
	name := collectionPluginName(kind)
	probeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := probeCtx.Err(); err != nil {
		return err
	}
	command := exec.Command(filepath.Join(programDir, "nas-collection-worker.exe"), "--check-"+kind)
	command.Dir, command.Env = root, environment
	command.Stdout, command.Stderr = io.Discard, io.Discard
	command.WaitDelay = 5 * time.Second
	process, err := startCollectionProcess(command)
	if err != nil {
		return errors.New("无法安全启动" + name + "依赖离线探测")
	}
	select {
	case <-process.done:
		if process.err != nil {
			return errors.Join(uavbootstrap.ErrCleanup, errors.New(name+"依赖离线探测进程未能完整清理"))
		}
		if process.exitErr != nil {
			return errors.New(name + "依赖离线探测未通过，请检查完整包版本与本机兼容性")
		}
		return probeCtx.Err()
	case <-probeCtx.Done():
		terminateErr := process.terminate()
		select {
		case <-process.done:
			if terminateErr != nil || process.err != nil {
				return errors.Join(probeCtx.Err(), uavbootstrap.ErrCleanup, errors.New(name+"依赖离线探测取消后未能完整清理"))
			}
			return probeCtx.Err()
		case <-time.After(7 * time.Second):
			return errors.Join(probeCtx.Err(), uavbootstrap.ErrCleanup, errors.New(name+"依赖离线探测取消后进程尚未完整释放"))
		}
	}
}
