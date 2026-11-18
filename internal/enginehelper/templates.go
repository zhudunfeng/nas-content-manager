package enginehelper

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"nas-content-manager/internal/enginecontract"
)

// template 是不接受浏览器覆盖的固定引擎镜像及端口定义。
type template struct {
	Component string // 固定组件标识。
	Image     string // 已钉住版本与多架构摘要的镜像。
	Port      int    // 应用仅在专用bridge访问的内部HTTP端口。
}

// engineTemplate 返回唯一受支持的固定模板。
func engineTemplate(component string) (template, error) {
	switch component {
	case enginecontract.QBittorrent:
		return template{component, "qbittorrentofficial/qbittorrent-nox:5.2.4-1@sha256:92bfd78d731e254ba64f62b77b0696add6447a33642ee5feda106a26b285aa7f", 8080}, nil
	case enginecontract.Jackett:
		return template{component, "ghcr.io/linuxserver/jackett:v0.24.3065-ls36@sha256:31b58c6acfae963dbb334ffd1b721fbefebed498a3df872f058d1e5544f8b02d", 9117}, nil
	default:
		return template{}, errors.New("未知引擎组件")
	}
}

// containerName 根据已持久化安装身份构造确定名称。
func containerName(installation string, component string) string {
	return "nas-engine-" + installation[:12] + "-" + component
}

// portBinding 是固定模板的一项宿主端口映射。
type portBinding struct {
	HostIP   string `json:"HostIp"`   // 固定回环或peer监听地址。
	HostPort string `json:"HostPort"` // 固定部署端口的十进制文本。
}

// containerSpec 是严格受控Docker创建请求，不允许用户注入JSON。
type containerSpec struct {
	Image        string              // 固定镜像版本及摘要。
	Env          []string            // 仅模板允许的非敏感环境参数。
	Labels       map[string]string   // 安装、组件、模板归属。
	ExposedPorts map[string]struct{} // 模板公开的容器端口。
	HostConfig   struct {
		Binds         []string                 // 固定宿主根派生的专用bind集合。
		Mounts        []volumeMount            `json:",omitempty"` // volume模式的完整固定挂载集合。
		NetworkMode   string                   // 预检通过的专用bridge名称。
		PortBindings  map[string][]portBinding // 固定peer或回环管理端口。
		RestartPolicy struct {
			Name string /* 保留手工停止意图的重启策略。 */
		} // 固定unless-stopped策略。
		SecurityOpt []string          // 固定禁止setuid获得新权限。
		Tmpfs       map[string]string // 有界且不可执行的临时存储。
		LogConfig   struct {
			Type   string            // 固定本地轮转日志驱动。
			Config map[string]string // 固定日志大小与保留数量。
		} // 防止日志占满NAS系统盘。
	} // 固定主机隔离与挂载。
	NetworkingConfig struct {
		EndpointsConfig map[string]struct {
			Aliases []string /* 当前安装的固定DNS别名。 */
		} // 只接入一个受信任网络。
	} // 唯一允许网络。
}

// makeSpec 仅从受信任部署参数生成完整的固定创建请求。
func makeSpec(c Config, installation string, t template) containerSpec {
	name := containerName(installation, t.Component)
	s := containerSpec{Image: t.Image, Labels: map[string]string{"nas.engines.installation": installation, "nas.engines.component": t.Component, "nas.engines.template": enginecontract.TemplateVersion}, ExposedPorts: map[string]struct{}{fmt.Sprintf("%d/tcp", t.Port): {}}}
	if c.StorageMode == "volume" {
		s.Labels["nas.engines.storage"] = "volume"
		s.Labels["nas.engines.deployment"] = c.VolumePrefix
		s.HostConfig.Mounts = []volumeMount{{Type: "volume", Source: volumeName(c, t.Component), Target: "/config"}}
		s.HostConfig.Mounts[0].VolumeOptions.NoCopy = true
	} else {
		s.HostConfig.Binds = []string{filepath.Join(c.HostRoot, t.Component) + ":/config:rw"}
	}
	s.HostConfig.NetworkMode = c.Network
	s.HostConfig.RestartPolicy.Name = "unless-stopped"
	s.HostConfig.SecurityOpt = []string{"no-new-privileges:true"}
	s.HostConfig.LogConfig.Type = "local"
	s.HostConfig.LogConfig.Config = map[string]string{"max-size": "10m", "max-file": "3"}
	s.HostConfig.PortBindings = map[string][]portBinding{}
	s.Env = []string{"PUID=" + strconv.Itoa(c.UID), "PGID=" + strconv.Itoa(c.GID), "TZ=Asia/Shanghai"}
	if t.Component == enginecontract.QBittorrent {
		s.Env = append(s.Env, "QBT_LEGAL_NOTICE=confirm", "QBT_WEBUI_PORT=8080", "QBT_TORRENTING_PORT="+strconv.Itoa(c.PeerPort), "UMASK=022")
		if c.StorageMode == "volume" {
			s.HostConfig.Mounts = append(s.HostConfig.Mounts, volumeMount{Type: "volume", Source: volumeName(c, "downloads"), Target: "/downloads"})
			s.HostConfig.Mounts[1].VolumeOptions.NoCopy = true
		} else {
			s.HostConfig.Binds = append(s.HostConfig.Binds, filepath.Join(c.HostRoot, "downloads")+":/downloads:rw")
		}
		s.HostConfig.Tmpfs = map[string]string{"/tmp": "rw,noexec,nosuid,size=64m"}
		for _, proto := range []string{"tcp", "udp"} {
			key := strconv.Itoa(c.PeerPort) + "/" + proto
			s.HostConfig.PortBindings[key] = []portBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(c.PeerPort)}}
			s.ExposedPorts[key] = struct{}{}
		}
	} else {
		s.Env = append(s.Env, "AUTO_UPDATE=false")
		s.HostConfig.PortBindings["9117/tcp"] = []portBinding{{HostIP: "127.0.0.1", HostPort: strconv.Itoa(c.SetupPort)}}
	}
	s.NetworkingConfig.EndpointsConfig = map[string]struct{ Aliases []string }{c.Network: {Aliases: []string{name}}}
	return s
}

// validateContainer 同时核对身份、完整挂载和权限，绝不仅凭同名接管。
func validateContainer(info *containerInfo, c Config, installation string, t template) error {
	if info.Config.Labels["nas.engines.installation"] != installation || info.Config.Labels["nas.engines.component"] != t.Component || info.Config.Labels["nas.engines.template"] != enginecontract.TemplateVersion || info.Config.Image != t.Image || info.HostConfig.Privileged || info.HostConfig.NetworkMode != c.Network {
		return errors.New("容器归属或固定模板冲突，请人工核对；不会接管或覆盖")
	}
	want := map[string]string{"/config": filepath.Join(c.HostRoot, t.Component)}
	if c.StorageMode == "volume" {
		if info.Config.Labels["nas.engines.storage"] != "volume" || info.Config.Labels["nas.engines.deployment"] != c.VolumePrefix {
			return errors.New("受管容器命名卷部署身份冲突")
		}
		want["/config"] = volumeName(c, t.Component)
	}
	if t.Component == enginecontract.QBittorrent {
		want["/downloads"] = filepath.Join(c.HostRoot, "downloads")
		if c.StorageMode == "volume" {
			want["/downloads"] = volumeName(c, "downloads")
		}
	}
	if len(info.Mounts) != len(want) {
		return errors.New("受管容器挂载集合冲突，不会修改已有资源")
	}
	for _, m := range info.Mounts {
		source := m.Source
		mountType := "bind"
		if c.StorageMode == "volume" {
			mountType = "volume"
			source = m.Name
			if m.Driver != "local" {
				return errors.New("受管容器命名卷驱动冲突")
			}
		}
		if m.Type != mountType || !m.RW || want[m.Destination] == "" || want[m.Destination] != source {
			return errors.New("受管容器挂载身份冲突，不会修改已有资源")
		}
		delete(want, m.Destination)
	}
	if len(info.NetworkSettings.Networks) != 1 || info.NetworkSettings.Networks[c.Network] == nil {
		return errors.New("受管容器网络冲突，不会修改已有资源")
	}
	if t.Component == enginecontract.QBittorrent {
		if len(info.HostConfig.Tmpfs) != 1 || !validQBTempfs(info.HostConfig.Tmpfs["/tmp"]) {
			return errors.New("受管下载器临时挂载配置冲突")
		}
	} else if len(info.HostConfig.Tmpfs) != 0 {
		return errors.New("受管搜索引擎存在额外临时挂载")
	}
	wantPorts := makeSpec(c, installation, t).HostConfig.PortBindings
	if len(info.HostConfig.PortBindings) != len(wantPorts) {
		return errors.New("受管容器端口配置冲突")
	}
	for p, bindings := range wantPorts {
		actual := info.HostConfig.PortBindings[p]
		if len(actual) != len(bindings) {
			return errors.New("受管容器端口配置冲突")
		}
		for i, b := range bindings {
			if actual[i] != b {
				return errors.New("受管容器端口配置冲突")
			}
		}
	}
	return nil
}

// validQBTempfs 核对Docker可能按字节规范化后的固定64MiB不可执行临时挂载。
func validQBTempfs(options string) bool {
	values := strings.Split(options, ",")
	if len(values) != 4 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if value == "size=64m" || value == "size=65536k" || value == "size=67108864" {
			value = "size"
		}
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	return seen["rw"] && seen["nosuid"] && seen["noexec"] && seen["size"]
}

// sortLayers 固定层展示顺序，避免轮询闪烁。
func sortLayers(layers []enginecontract.Layer) {
	sort.Slice(layers, func(i int, j int) bool { return layers[i].ID < layers[j].ID })
}

// canonicalComponents 拒绝未知或重复组件并按固定顺序生成幂等目标。
func canonicalComponents(values []string) ([]string, error) {
	if len(values) < 1 || len(values) > 2 {
		return nil, errors.New("请选择一至两个固定引擎")
	}
	seen := map[string]bool{}
	for _, v := range values {
		if _, err := engineTemplate(v); err != nil || seen[v] {
			return nil, errors.New("组件重复或不受支持")
		}
		seen[v] = true
	}
	result := []string{}
	for _, v := range []string{enginecontract.QBittorrent, enginecontract.Jackett} {
		if seen[v] {
			result = append(result, v)
		}
	}
	return result, nil
}

// safeID 校验请求与任务标识，不允许路径或无界输入。
func safeID(value string) bool {
	if len(value) < 8 || len(value) > 80 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-", r) {
			return false
		}
	}
	return true
}
