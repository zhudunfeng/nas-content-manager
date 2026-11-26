package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nas-content-manager/internal/collectioncontract"
	"nas-content-manager/internal/collectioncontrol"
)

// managedCollection 保存本实例私有控制句柄，凭据不写入桌面状态响应。
type managedCollection struct {
	process    *collectionProcess            // 已归属独占 Job 的 worker 进程树。
	input      *os.File                      // 仅父进程持有的命令写端。
	output     *os.File                      // 仅父进程持有的就绪读端。
	log        *rotatingWriter               // 单独限额的 worker 中文日志。
	nonce      string                        // 当前启动的一次性关联值，不回显。
	instanceID string                        // 当前桌面 profile 身份。
	address    string                        // 已通过认证和目录挑战的回环地址。
	readRoot   string                        // 私有 worker 数据根。
	tokenFile  string                        // 与本实例数据根绑定的秘密文件。
	capability collectioncontract.Capability // 固定漫画依赖的真实能力，不代表公网站点可达。
	once       sync.Once                     // 防止退出和错误清理重复发送停止。
	stopErr    error                         // 有序停止的完整结果。
}

// prepareCollectionDirectory 只创建本实例专用目录，拒绝接管未知目录或链接别名。
func prepareCollectionDirectory(profile string, cfg Config, programDir string) (string, error) {
	root := filepath.Join(profile, "collection-managed")
	canonical := canonicalConfigPath(root)
	if !strings.EqualFold(filepath.Clean(canonical), filepath.Clean(root)) || strings.HasPrefix(canonical, `\\`) {
		return "", errors.New("本机获取状态目录不能使用链接别名或网络共享")
	}
	for _, protected := range append([]string{cfg.DataDir, programDir}, cfg.MediaRoots...) {
		protected = canonicalConfigPath(protected)
		if pathContains(root, protected) || pathContains(protected, root) {
			return "", errors.New("本机获取状态目录不能与程序、业务数据或媒体授权目录重叠")
		}
	}
	created := false
	if err := os.Mkdir(root, 0700); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return "", errors.New("无法创建本机获取状态目录")
	}
	marker := filepath.Join(root, "owner.json")
	if !created {
		info, err := os.Lstat(root)
		ownerInfo, ownerErr := os.Lstat(marker)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || ownerErr != nil || !ownerInfo.Mode().IsRegular() || ownerInfo.Size() > 1024 {
			return "", errors.New("既有获取状态目录归属不明，拒绝接管或修改权限")
		}
		data, err := os.ReadFile(marker)
		var owner struct {
			Version    int    `json:"version"`    // 专属目录标记格式版本。
			InstanceID string `json:"instanceId"` // 拥有此目录的桌面 profile 身份。
		}
		if err != nil || decodeStrictJSON(data, &owner) != nil || owner.Version != 1 || owner.InstanceID != cfg.InstanceID {
			return "", errors.New("既有获取状态目录属于其他实例，拒绝修改")
		}
	}
	// 只有新建目录或已验证归属的专用目录才应用真实 Windows DACL。
	if err := validateLocalDataDir(root); err != nil {
		return "", err
	}
	if created {
		data := []byte(`{"version":1,"instanceId":"` + cfg.InstanceID + `"}` + "\n")
		file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return "", errors.New("无法记录本机获取状态目录归属")
		}
		_, err = file.Write(data)
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return "", errors.New("本机获取目录归属未完整写入")
		}
	}
	for _, name := range []string{"data", "secrets", "tmp"} {
		path := filepath.Join(root, name)
		if info, err := os.Lstat(path); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return "", errors.New("本机获取专属子目录包含链接或非目录")
		}
		if err := os.MkdirAll(path, 0700); err != nil {
			return "", errors.New("无法创建本机获取专属子目录")
		}
	}
	return root, nil
}

// collectionEnvironment 仅传递受控配置快照，托管模式绝不从用户 PATH 搜索 Python。
func collectionEnvironment(programDir string, root string) ([]string, error) {
	values := map[string]string{
		"PATH": filepath.Join(programDir, "tools"),
		"TEMP": filepath.Join(root, "tmp"), "TMP": filepath.Join(root, "tmp"),
		"NAS_COLLECTION_WORKER_ADDR":                     "127.0.0.1:0",
		"NAS_COLLECTION_WORKER_DATA_DIR":                 filepath.Join(root, "data"),
		"NAS_COLLECTION_WORKER_TOKEN_FILE":               filepath.Join(root, "secrets", "token"),
		"NAS_COLLECTION_WORKER_JMCOMIC_ENABLED":          "true",
		"NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST": filepath.Join(programDir, "jmcomic-runtime.json"),
	}
	for _, key := range []string{"SystemRoot", "WINDIR", "SYSTEMDRIVE", "PROCESSOR_ARCHITECTURE", "NUMBER_OF_PROCESSORS", "NAS_COLLECTION_WORKER_MAX_BYTES", "NAS_COLLECTION_WORKER_MAX_FILES", "NAS_COLLECTION_WORKER_MIN_FREE_MB", "NAS_COLLECTION_WORKER_MAX_CONCURRENT"} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
		}
	}
	if python := os.Getenv("NAS_COLLECTION_WORKER_JMCOMIC_PYTHON"); python != "" {
		if !filepath.IsAbs(python) {
			return nil, errors.New("本机托管漫画解释器覆盖必须为绝对路径")
		}
		values["NAS_COLLECTION_WORKER_JMCOMIC_PYTHON"] = python
	}
	environment := make([]string, 0, len(values))
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	return environment, nil
}

// startManagedCollection 建立私有继承管道，核验真实绑定和本实例目录后才返回连接。
func (c *controller) startManagedCollection(ctx context.Context, cfg Config) (*managedCollection, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, errors.New("无法定位本机程序目录")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, errors.New("无法核验本机程序目录")
	}
	programDir := filepath.Dir(path)
	worker := filepath.Join(programDir, "nas-collection-worker.exe")
	info, err := os.Lstat(worker)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("随包获取服务缺失，请使用含 JMComic 的完整 Windows 包")
	}
	root, err := prepareCollectionDirectory(c.profile, cfg, programDir)
	if err != nil {
		return nil, err
	}
	environment, err := collectionEnvironment(programDir, root)
	if err != nil {
		return nil, err
	}
	childInput, input, err := os.Pipe()
	if err != nil {
		return nil, errors.New("无法创建获取服务私有输入通道")
	}
	output, childOutput, err := os.Pipe()
	if err != nil {
		_ = childInput.Close()
		_ = input.Close()
		return nil, errors.New("无法创建获取服务私有输出通道")
	}
	log := &rotatingWriter{path: filepath.Join(c.profile, "logs", "collection-worker.log")}
	if err = log.open(); err != nil {
		_ = childInput.Close()
		_ = input.Close()
		_ = output.Close()
		_ = childOutput.Close()
		return nil, errors.New("无法创建获取服务诊断日志")
	}
	command := exec.Command(worker, "--managed-stdio")
	command.Dir = root
	command.Env = environment
	command.Stdin = childInput
	command.Stdout = childOutput
	command.Stderr = log
	command.WaitDelay = 5 * time.Second
	process, err := startCollectionProcess(command)
	_ = childInput.Close()
	_ = childOutput.Close()
	if err != nil {
		_ = input.Close()
		_ = output.Close()
		_ = log.Close()
		return nil, err
	}
	nonceBytes := make([]byte, 32)
	_, err = rand.Read(nonceBytes)
	m := &managedCollection{process: process, input: input, output: output, log: log, nonce: hex.EncodeToString(nonceBytes), instanceID: cfg.InstanceID, readRoot: filepath.Join(root, "data"), tokenFile: filepath.Join(root, "secrets", "token")}
	if err != nil {
		return nil, errors.Join(errors.New("无法生成获取服务关联值"), m.stop())
	}
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	type readyResult struct {
		message collectioncontrol.Message // 经有界严格解码的本次就绪消息。
		err     error                     // 私有管道写入或解码结果。
	}
	ready := make(chan readyResult, 1)
	go func() {
		if writeErr := collectioncontrol.Write(input, collectioncontrol.Message{Version: collectioncontrol.Version, Action: "start", Nonce: m.nonce, InstanceID: m.instanceID}); writeErr != nil {
			ready <- readyResult{err: writeErr}
			return
		}
		message, readErr := collectioncontrol.NewDecoder(output).Read()
		ready <- readyResult{message: message, err: readErr}
	}()
	select {
	case result := <-ready:
		if result.err != nil || collectioncontrol.Match(result.message, "ready", m.nonce, m.instanceID) != nil {
			return nil, errors.Join(errors.New("本机获取服务就绪协议无效，请查看日志"), m.stop())
		}
		u, parseErr := url.Parse(result.message.Address)
		if parseErr != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !strings.EqualFold(result.message.ReadRoot, canonicalConfigPath(m.readRoot)) {
			return nil, errors.Join(errors.New("本机获取服务地址或专属目录不匹配"), m.stop())
		}
		m.address = result.message.Address
		if err = m.verify(readyCtx); err != nil {
			return nil, errors.Join(err, m.stop())
		}
		c.logger.Info("本机获取服务已完成身份和目录核验")
		return m, nil
	case <-process.done:
		return nil, errors.Join(errors.New("本机获取服务在就绪前退出，请查看日志"), m.stop())
	case <-readyCtx.Done():
		return nil, errors.Join(errors.New("本机获取服务启动取消或超时"), m.stop())
	}
}

// verify 通过令牌认证能力与随机落盘挑战，防止连接错实例或混用共享根。
func (m *managedCollection) verify(ctx context.Context) error {
	info, err := os.Lstat(m.tokenFile)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256 {
		return errors.New("本机获取服务凭据不可用")
	}
	data, err := os.ReadFile(m.tokenFile)
	token := strings.TrimSpace(string(data))
	if err != nil || len(token) < 32 || strings.ContainsAny(token, " \r\n\t") {
		return errors.New("本机获取服务凭据格式无效")
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	call := func(callCtx context.Context, method string, endpoint string, output any) error {
		request, requestErr := http.NewRequestWithContext(callCtx, method, m.address+endpoint, nil)
		if requestErr != nil {
			return errors.New("本机获取服务检查地址无效")
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, requestErr := client.Do(request)
		if requestErr != nil {
			return errors.New("本机获取服务认证检查失败")
		}
		defer response.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 65537))
		if readErr != nil || len(body) > 65536 || response.StatusCode < 200 || response.StatusCode >= 300 {
			return errors.New("本机获取服务未通过认证检查")
		}
		if output != nil && decodeStrictJSON(body, output) != nil {
			return errors.New("本机获取服务检查响应无效")
		}
		return nil
	}
	var caps collectioncontract.Capabilities
	if err = call(ctx, http.MethodGet, "/v1/capabilities", &caps); err != nil {
		return err
	}
	if caps.Version != collectioncontract.Version || caps.InstanceID == "" || len(caps.Capabilities) > 8 || len(caps.Sources) > 64 {
		return errors.New("本机获取服务协议不兼容")
	}
	for _, capability := range caps.Capabilities {
		if capability.Kind == "jmcomic" {
			m.capability = capability
		}
	}
	// 探针使用 UUID 格式，和现有服务协议保持一致。
	probeBytes := make([]byte, 16)
	if _, err = rand.Read(probeBytes); err != nil {
		return errors.New("无法创建本机目录核验挑战")
	}
	probeBytes[6] = probeBytes[6]&15 | 64
	probeBytes[8] = probeBytes[8]&63 | 128
	hexValue := hex.EncodeToString(probeBytes)
	nonce := hexValue[:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:]
	endpoint := "/v1/probe?nonce=" + nonce
	var probe collectioncontract.Probe
	if err = call(ctx, http.MethodGet, endpoint, &probe); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = call(cleanup, http.MethodDelete, endpoint, nil)
	}()
	if probe.Nonce != nonce || probe.RelativePath != "probes/"+nonce {
		return errors.New("本机获取服务目录探针不匹配")
	}
	root, err := os.OpenRoot(m.readRoot)
	if err != nil {
		return errors.New("本机获取服务只读目录不可访问")
	}
	defer root.Close()
	file, err := root.Open(probe.RelativePath)
	if err != nil {
		return errors.New("本机获取服务与主程序目录映射不一致")
	}
	defer file.Close()
	data, err = io.ReadAll(io.LimitReader(file, 128))
	if err != nil || string(data) != nonce {
		return errors.New("本机获取服务目录挑战失败")
	}
	return nil
}

// stop 先发送正常停止再等待 Job 排空，超时仅终止本实例子树并报告未正常结束。
func (m *managedCollection) stop() error {
	m.once.Do(func() {
		select {
		case <-m.process.done:
		default:
			// 关闭输入同时提供 EOF 兜底；写入放入协程，避免异常子进程不读管道卡住退出。
			written := make(chan struct{})
			go func() {
				_ = collectioncontrol.Write(m.input, collectioncontrol.Message{Version: collectioncontrol.Version, Action: "stop", Nonce: m.nonce, InstanceID: m.instanceID})
				_ = m.input.Close()
				close(written)
			}()
			select {
			case <-written:
			case <-time.After(time.Second):
				_ = m.input.Close()
			}
		}
		select {
		case <-m.process.done:
		case <-time.After(15 * time.Second):
			m.stopErr = errors.Join(errors.New("本机获取服务未在正常停止预算内退出，已回收本实例子树"), m.process.terminate())
			select {
			case <-m.process.done:
			case <-time.After(7 * time.Second):
				m.stopErr = errors.Join(m.stopErr, errors.New("本机获取服务进程尚未完整释放"))
			}
		}
		_ = m.input.Close()
		_ = m.output.Close()
		select {
		case <-m.process.done:
			m.stopErr = errors.Join(m.stopErr, m.process.err)
		default:
		}
		m.stopErr = errors.Join(m.stopErr, m.log.Close())
	})
	return m.stopErr
}
