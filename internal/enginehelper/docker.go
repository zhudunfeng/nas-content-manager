package enginehelper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"nas-content-manager/internal/enginecontract"
)

// dockerClient 只封装固定模板使用的本机Docker操作。
type dockerClient struct {
	http     *http.Client                   // 有连接与头部超时的本机HTTP客户端。
	metadata atomic.Pointer[dockerMetadata] // 并发检测可安全读取的不可变协商结果。
}

// dockerMetadata 保存一次协商后的不可变API与平台信息。
type dockerMetadata struct {
	version string // 已协商的API版本前缀。
	arch    string // 已核验的Linux架构。
}

// api 返回原子协商的API前缀；未协商时不发送受版本约束操作。
func (d *dockerClient) api() string {
	if m := d.metadata.Load(); m != nil {
		return m.version
	}
	return "/v1.41"
}

// architecture 返回最近成功检测的平台。
func (d *dockerClient) architecture() string {
	if m := d.metadata.Load(); m != nil {
		return m.arch
	}
	return ""
}

// dockerError 只携带HTTP状态，避免泄漏原始Docker返回信息。
type dockerError struct {
	status int /* 仅保留HTTP状态，不保存Docker响应体。 */
}

// Error 返回可安全记录的Docker状态说明。
func (e dockerError) Error() string { return fmt.Sprintf("Docker调用失败（HTTP %d）", e.status) }

// newDocker 创建只能连接固定Unix socket的客户端。
func newDocker(socket string) *dockerClient {
	t := &http.Transport{DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
	}, ResponseHeaderTimeout: 30 * time.Second, MaxIdleConns: 4, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &dockerClient{http: &http.Client{Transport: t, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

// call 有界执行普通Docker请求，绝不回传原始错误体。
func (d *dockerClient) call(ctx context.Context, method string, path string, input any, output any) error {
	return d.callWithin(ctx, method, path, input, output, 10*time.Second)
}

// callWithin 为固定停止动作提供额外有界宽限，其余请求继续使用十秒预算。
func (d *dockerClient) callWithin(ctx context.Context, method string, path string, input any, output any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, body)
	if err != nil {
		return errors.New("Docker请求无效")
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := d.http.Do(req)
	if err != nil {
		return errors.New("Docker连接不可用，请检查助手挂载")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return dockerError{res.StatusCode}
	}
	if output == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return err
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return errors.New("Docker响应超过预算")
	}
	if json.Unmarshal(data, output) != nil {
		return errors.New("Docker响应结构无效")
	}
	return nil
}

// preflight 协商API并验证受支持的Linux bridge运行边界，不创建任何资源。
func (d *dockerClient) preflight(ctx context.Context, c Config) error {
	if runtime.GOOS != "linux" {
		return errors.New("自动初始化仅支持Linux助手")
	}
	var v struct {
		APIVersion    string // daemon提供的API版本。
		MinAPIVersion string // daemon最低API版本。
		Os            string // daemon操作系统。
		Arch          string // daemon架构。
	}
	if err := d.call(ctx, "GET", "/version", nil, &v); err != nil {
		return err
	}
	parts := strings.Split(v.APIVersion, ".")
	if len(parts) != 2 || parts[0] != "1" {
		return errors.New("Docker API版本不兼容")
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 41 || minor > 100 {
		return errors.New("Docker API版本不兼容")
	}
	if v.Os != "linux" || (v.Arch != "amd64" && v.Arch != "arm64" && v.Arch != "aarch64") {
		return errors.New("Docker主机系统或架构不受支持")
	}
	arch := v.Arch
	if arch == "aarch64" {
		arch = "arm64"
	}
	d.metadata.Store(&dockerMetadata{version: "/v" + v.APIVersion, arch: arch})
	var info struct {
		SecurityOptions []string /* daemon启用的安全模式。 */
	}
	if err = d.call(ctx, "GET", d.api()+"/info", nil, &info); err != nil {
		return err
	}
	for _, s := range info.SecurityOptions {
		if strings.Contains(s, "rootless") || strings.Contains(s, "userns") {
			return errors.New("当前助手不支持rootless或userns-remap，请使用外接模式")
		}
	}
	var network struct {
		Driver   string // 必须为bridge。
		Internal bool   // 内部隔离网络无法访问索引器与peer。
	}
	if err = d.call(ctx, "GET", d.api()+"/networks/"+url.PathEscape(c.Network), nil, &network); err != nil {
		return errors.New("专用Docker网络不可访问，请先应用托管Compose配置")
	}
	if network.Driver != "bridge" || network.Internal {
		return errors.New("初始化需要可出站的专用bridge网络")
	}
	return nil
}

// containerInfo 是归属校验需要的最小Docker容器结构。
type containerInfo struct {
	ID     string `json:"Id"` // Docker容器身份。
	Image  string // 镜像内容身份。
	Config struct {
		Image      string            // 创建时的完整固定镜像引用。
		Labels     map[string]string // 安装与模板归属标签。
		Env        []string          // 实际运行环境，资源组件须与固定镜像默认值和受信任覆盖一致。
		Cmd        []string          // 实际启动参数，不允许浏览器或外部修改。
		Entrypoint []string          // 实际入口，须与固定镜像一致。
		User       string            // 实际初始用户，须保留固定镜像初始化身份。
	} // 固定模板和归属标签。
	State struct {
		Running bool   // daemon确认的运行状态。
		Status  string // daemon生命周期状态。
	} // 实际运行状态。
	Mounts []struct {
		Type        string // 只允许所选存储驱动的固定挂载。
		Name        string // volume模式的daemon逻辑卷名，不使用Mountpoint作bind来源。
		Driver      string // 命名卷必须使用本地驱动。
		Source      string // daemon侧宿主绝对来源。
		Destination string // 固定容器目标。
		RW          bool   // 引擎专用数据挂载必须可写。
	} // 完整挂载集合。
	HostConfig struct {
		CapAdd          []string          // 禁止添加额外Linux权限。
		CapDrop         []string          // 不允许未核验的权限集合变更。
		Devices         []json.RawMessage // 禁止任意宿主设备。
		DeviceRequests  []json.RawMessage // 禁止额外GPU或其他设备请求。
		VolumesFrom     []string          // 禁止继承其他容器挂载。
		PidMode         string            // 禁止共享宿主进程命名空间。
		IpcMode         string            // 只接受Docker默认private IPC。
		UTSMode         string            // 禁止共享宿主UTS。
		UsernsMode      string            // 禁止模板外用户命名空间。
		PublishAllPorts bool              // 禁止自动发布其他端口。
		ReadonlyRootfs  bool              // 保留上游所需根目录初始化能力。
		AutoRemove      bool              // 禁止退出时自动删除持久容器。
		SecurityOpt     []string          // 固定no-new-privileges，不自动关闭seccomp。
		RestartPolicy   struct {
			Name              string // 固定unless-stopped。
			MaximumRetryCount int    // 不设置模板外重试次数。
		} // 保留明确手动停止意图。
		LogConfig struct {
			Type   string            // 固定local日志驱动。
			Config map[string]string // 固定轮转大小与数量。
		} // 防止无界日志占用。
		Privileged   bool                     // 禁止特权容器。
		NetworkMode  string                   // 必须加入固定专用网络。
		PortBindings map[string][]portBinding // 固定peer或回环管理映射。
		Tmpfs        map[string]string        // 固定临时文件系统，不能带入额外挂载。
	} // 受控宿主权限与端口。
	NetworkSettings struct {
		Networks map[string]json.RawMessage /* 实际连接的网络名称。 */
	} // 所接网络集合。
}

// inspect 查询确定名称容器，只有404才代表不存在。
func (d *dockerClient) inspect(ctx context.Context, name string) (*containerInfo, error) {
	var info containerInfo
	err := d.call(ctx, "GET", d.api()+"/containers/"+url.PathEscape(name)+"/json", nil, &info)
	var de dockerError
	if errors.As(err, &de) && de.status == 404 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// imageReady 核验固定摘要、平台与镜像配置，返回实际内容ID。
func (d *dockerClient) imageReady(ctx context.Context, t template) (string, bool, error) {
	var info struct {
		ID           string   `json:"Id"` // 实际镜像内容ID。
		RepoDigests  []string // 拉取后daemon记录的仓库摘要。
		Os           string   // 镜像目标操作系统。
		Architecture string   // 镜像目标架构。
	}
	err := d.call(ctx, "GET", d.api()+"/images/"+url.PathEscape(t.Image)+"/json", nil, &info)
	var de dockerError
	if errors.As(err, &de) && de.status == 404 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	digest := strings.Split(t.Image, "@")[1]
	found := false
	for _, value := range info.RepoDigests {
		if strings.HasSuffix(value, "@"+digest) {
			found = true
		}
	}
	if !found || info.Os != "linux" || info.Architecture != d.architecture() {
		return "", false, errors.New("镜像摘要或平台不匹配，请检查固定模板")
	}
	return info.ID, true, nil
}

// pull 消费真实分层NDJSON并检查流内错误，完成后必须再次inspect。
func (d *dockerClient) pull(ctx context.Context, t template, progress func([]enginecontract.Layer)) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	parts := strings.Split(t.Image, "@")
	req, err := http.NewRequestWithContext(ctx, "POST", "http://docker"+d.api()+"/images/create?fromImage="+url.QueryEscape(strings.Split(parts[0], ":")[0])+"&tag="+url.QueryEscape(parts[1]), nil)
	if err != nil {
		return errors.New("镜像请求无效")
	}
	res, err := d.http.Do(req)
	if err != nil {
		return errors.New("镜像拉取连接失败，可稍后重试")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("镜像拉取失败，请检查网络或仓库限流")
	}
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	layers := map[string]enginecontract.Layer{}
	last := time.Time{}
	for scanner.Scan() {
		var row struct {
			ID             string          `json:"id"`
			Status         string          `json:"status"`
			Error          string          `json:"error"`
			ErrorDetail    json.RawMessage `json:"errorDetail"`
			ProgressDetail struct {
				Current int64 `json:"current"`
				Total   int64 `json:"total"`
			} `json:"progressDetail"`
		}
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			return errors.New("镜像进度流无效")
		}
		if row.Error != "" || len(row.ErrorDetail) > 0 && string(row.ErrorDetail) != "null" {
			if strings.Contains(strings.ToLower(row.Error), "no space") {
				return errors.New("Docker镜像存储空间不足，请手动释放空间后重试")
			}
			return errors.New("镜像拉取或解包失败，请检查网络、仓库和Docker存储空间")
		}
		if row.ID != "" {
			// 镜像层标识只接受摘要字符，绝不把仓库URL或任意文本当作层ID回传。
			if !safeLayerID(row.ID) {
				continue
			}
			if len(row.ID) > 128 || len(layers) >= 128 && layers[row.ID].ID == "" {
				return errors.New("镜像进度超过资源预算")
			}
			state := "处理中"
			switch row.Status {
			case "Downloading":
				state = "下载中"
			case "Extracting":
				state = "解包中"
			case "Pull complete", "Already exists":
				state = "已就绪"
			case "Verifying Checksum":
				state = "校验中"
			case "Download complete":
				state = "下载完成"
			}
			current, total := row.ProgressDetail.Current, row.ProgressDetail.Total
			if current < 0 || total < 0 || current > total {
				current, total = 0, 0
			}
			layers[row.ID] = enginecontract.Layer{ID: row.ID, Status: state, Current: current, Total: total}
			if time.Since(last) >= time.Second {
				progress(layerSlice(layers))
				last = time.Now()
			}
		}
	}
	if scanner.Err() != nil {
		return errors.New("镜像进度连接中断，可重试并复用已完成层")
	}
	progress(layerSlice(layers))
	_, ready, err := d.imageReady(ctx, t)
	if err != nil {
		return err
	}
	if !ready {
		return errors.New("镜像流结束但目标摘要未就绪，可重试")
	}
	return nil
}

// safeLayerID 仅接受Docker常见的十六进制层摘要，不回传自由文本。
func safeLayerID(value string) bool {
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// layerSlice 生成有界且顺序稳定的最新镜像层快照。
func layerSlice(values map[string]enginecontract.Layer) []enginecontract.Layer {
	result := make([]enginecontract.Layer, 0, len(values))
	for _, v := range values {
		result = append(result, v)
	}
	sortLayers(result)
	return result
}
