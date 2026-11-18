package desktop

import (
	"errors"
	"maps"
	"slices"
	"strconv"
)

// composeDesktopConfig 是固定桌面模板的规范化视图，未知配置一律拒绝。
type composeDesktopConfig struct {
	Name     string                           `json:"name"`     // 当前实例 Compose project。
	Services map[string]composeDesktopService `json:"services"` // 仅允许固定应用和助手服务。
	Volumes  map[string]composeDesktopVolume  `json:"volumes"`  // 独立实例的七个角色卷。
	Networks map[string]composeDesktopNetwork `json:"networks"` // 唯一受控 bridge 网络。
}

// composeDesktopService 仅包含正式模板需要的配置，不接受附加权限或入口覆盖。
type composeDesktopService struct {
	Image           string                              `json:"image"`             // 不可变发布镜像引用。
	Environment     map[string]string                   `json:"environment"`       // 固定的容器内部配置。
	Labels          map[string]string                   `json:"labels"`            // 实例和服务角色标签。
	Restart         string                              `json:"restart"`           // 明确停止后保持停止。
	Ports           []composeDesktopPort                `json:"ports"`             // 仅应用的回环端口。
	Volumes         []composeDesktopMount               `json:"volumes"`           // 逐一验证的精确挂载。
	Networks        map[string]*struct{}                `json:"networks"`          // 唯一 engines 网络且无自定义别名。
	SecurityOpt     []string                            `json:"security_opt"`      // 固定禁止提权设置。
	StopGracePeriod string                              `json:"stop_grace_period"` // 数据库与任务的正常退出预算。
	DependsOn       map[string]composeDesktopDependency `json:"depends_on"`        // 应用仅依赖本实例助手。
	Logging         composeDesktopLogging               `json:"logging"`           // 有界本地日志。
}

// composeDesktopDependency 允许 Compose 展开短格式依赖后的受控默认字段。
type composeDesktopDependency struct {
	Condition string `json:"condition"` // 固定等待助手服务启动。
	Required  *bool  `json:"required"`  // 新版 Compose 输出 true，旧版允许省略。
}

// composeDesktopLogging 记录固定日志驱动及轮转预算。
type composeDesktopLogging struct {
	Driver  string            `json:"driver"`  // 必须使用 local 驱动。
	Options map[string]string `json:"options"` // 单份 10 MiB、最多三份。
}

// composeDesktopPort 限定唯一回环映射，禁止其他宿主接口或端口。
type composeDesktopPort struct {
	Target    int    `json:"target"`    // 应用容器固定 8090。
	Published string `json:"published"` // 最终已选宿主端口。
	HostIP    string `json:"host_ip"`   // 必须为 127.0.0.1。
	Protocol  string `json:"protocol"`  // 必须为 TCP。
	Mode      string `json:"mode"`      // 允许 Compose 默认 ingress 表示。
}

// composeDesktopMount 保留规范化挂载的最小必要字段。
type composeDesktopMount struct {
	Type     string       `json:"type"`      // 仅固定 bind 或命名卷。
	Source   string       `json:"source"`    // 受管逻辑卷名或唯一授权宿主目录。
	Target   string       `json:"target"`    // 固定容器路径。
	ReadOnly bool         `json:"read_only"` // 媒体与应用下载根必须只读。
	Bind     *composeBind `json:"bind"`      // 媒体禁止自动创建宿主目录。
	Volume   *struct{}    `json:"volume"`    // 不接受 subpath、nocopy 等模板外设置。
}

// composeBind 只允许 Compose 默认创建行为，不允许传播或 SELinux 覆盖。
type composeBind struct {
	CreateHostPath bool `json:"create_host_path"` // 媒体必须 false，固定 Docker socket 可使用短格式默认值。
}

// composeDesktopVolume 限定本机默认驱动与安装归属。
type composeDesktopVolume struct {
	Name       string            `json:"name"`        // prefix-role 的精确卷名。
	Driver     string            `json:"driver"`      // 只能使用本机 local。
	Labels     map[string]string `json:"labels"`      // 精确部署和角色标签。
	External   bool              `json:"external"`    // 不接管外部卷。
	DriverOpts map[string]string `json:"driver_opts"` // 禁止 NFS、宿主 bind 等额外驱动参数。
}

// composeDesktopNetwork 限定单一可出站 bridge，不接受外来网络。
type composeDesktopNetwork struct {
	Name       string            `json:"name"`        // prefix-engines 的精确名称。
	Driver     string            `json:"driver"`      // 必须为 bridge。
	External   bool              `json:"external"`    // 不接管外部网络。
	Internal   bool              `json:"internal"`    // 助手需要受控出站。
	Attachable bool              `json:"attachable"`  // 不开放额外手动附加能力。
	EnableIPv6 bool              `json:"enable_ipv6"` // 首版不扩大网络地址族。
	Labels     map[string]string `json:"labels"`      // 不接受模板外的归属标签。
	DriverOpts map[string]string `json:"driver_opts"` // 不覆盖默认网络设备或路由。
	IPAM       *struct{}         `json:"ipam"`        // 允许 Compose 默认空对象，不接受自定义子网或地址池。
}

// validateComposeData 在任何部署副作用前验证最终 JSON，也供无 daemon 回归复用。
func (d *dockerRuntime) validateComposeData(data []byte) error {
	var config composeDesktopConfig
	if decodeStrictJSON(data, &config) != nil || config.Name != d.project || len(config.Services) != 2 || len(d.config.MediaRoots) != 1 {
		return errors.New("Compose project、服务集合或配置字段与固定桌面模板不匹配")
	}
	for service, image := range map[string]string{"nas": d.manifest.AppImage, "nas-engine-helper": d.manifest.HelperImage} {
		entry, ok := config.Services[service]
		role := "app"
		if service == "nas-engine-helper" {
			role = "helper"
		}
		labels := map[string]string{"nas.engines.deployment": d.project, "nas.desktop.role": role}
		if !ok || entry.Image != image || !maps.Equal(entry.Labels, labels) || entry.Restart != "unless-stopped" || !slices.Equal(entry.SecurityOpt, []string{"no-new-privileges:true"}) || entry.StopGracePeriod != "30s" {
			return errors.New("Compose 服务镜像、标签或运行权限与固定桌面模板不匹配")
		}
		if len(entry.Networks) != 1 {
			return errors.New("Compose 服务只能加入当前实例的 engines 网络")
		}
		if _, present := entry.Networks["engines"]; !present {
			return errors.New("Compose 服务网络不属于当前实例")
		}
		if entry.Logging.Driver != "local" || !maps.Equal(entry.Logging.Options, map[string]string{"max-size": "10m", "max-file": "3"}) {
			return errors.New("Compose 日志轮转配置与固定模板不匹配")
		}
		if err := d.validateComposeMounts(entry.Volumes, service); err != nil {
			return err
		}
	}
	app := config.Services["nas"]
	if len(app.Ports) != 1 {
		return errors.New("Compose 应用只能发布一个本机回环端口")
	}
	port := app.Ports[0]
	if port.Target != 8090 || port.Published != strconv.Itoa(d.config.Port) || port.HostIP != "127.0.0.1" || port.Protocol != "tcp" || (port.Mode != "" && port.Mode != "ingress") {
		return errors.New("Compose 应用端口不是本实例固定回环绑定")
	}
	dependency, exists := app.DependsOn["nas-engine-helper"]
	if len(app.DependsOn) != 1 || !exists || dependency.Condition != "service_started" || (dependency.Required != nil && !*dependency.Required) {
		return errors.New("Compose 应用依赖与本实例助手不一致")
	}
	appEnvironment := map[string]string{"NAS_ADDR": "0.0.0.0:8090", "NAS_DATA_DIR": "/data", "NAS_MEDIA_ROOTS": "/media:/downloads/complete", "NAS_DOWNLOAD_MODE": "managed", "NAS_TORRENT_SEARCH_MODE": "managed", "NAS_DOWNLOAD_ROOT": "/downloads", "NAS_DOWNLOAD_ENGINE_ROOT": "/downloads", "NAS_ENGINE_CONNECTION_DIR": "/run/nas-engines", "NAS_ENGINE_HELPER_SOCKET": "/run/nas-engine-helper/helper.sock", "NAS_DOWNLOAD_MIN_FREE_GB": "2"}
	helperEnvironment := map[string]string{"ENGINE_STORAGE_MODE": "volume", "ENGINE_VOLUME_PREFIX": d.project, "ENGINE_MANAGED_ROOT": "/managed", "ENGINE_HELPER_SOCKET": "/ipc/helper.sock", "ENGINE_DOCKER_SOCKET": "/var/run/docker.sock", "ENGINE_NETWORK": d.project + "-engines", "ENGINE_UID": "1000", "ENGINE_GID": "1000", "QBT_PEER_PORT": "6881", "JACKETT_SETUP_PORT": "19117"}
	helper := config.Services["nas-engine-helper"]
	if !maps.Equal(app.Environment, appEnvironment) || !maps.Equal(helper.Environment, helperEnvironment) || len(helper.Ports) != 0 || len(helper.DependsOn) != 0 {
		return errors.New("Compose 内部路径、引擎参数或助手端口与固定模板不匹配")
	}
	roles := map[string]string{"app_data": "app-data", "state": "state", "qbittorrent": "qbittorrent", "jackett": "jackett", "downloads": "downloads", "connections": "connections", "ipc": "ipc"}
	if len(config.Volumes) != len(roles) {
		return errors.New("Compose 必须仅声明当前实例的七个角色卷")
	}
	for logical, role := range roles {
		volume, present := config.Volumes[logical]
		if !present || volume.Name != d.project+"-"+role || volume.Driver != "local" || volume.External || len(volume.DriverOpts) != 0 || !maps.Equal(volume.Labels, map[string]string{"nas.engines.deployment": d.project, "nas.engines.role": role}) {
			return errors.New("Compose 命名卷的驱动、实际名称或角色归属不匹配")
		}
	}
	network, present := config.Networks["engines"]
	if len(config.Networks) != 1 || !present || network.Name != d.project+"-engines" || network.Driver != "bridge" || network.External || network.Internal || network.Attachable || network.EnableIPv6 || len(network.DriverOpts) != 0 || len(network.Labels) != 0 {
		return errors.New("Compose 网络名称、归属或隔离配置不匹配")
	}
	return nil
}

// validateComposeMounts 比对每个服务的完整挂载集，保持原始媒体只读和角色卷隔离。
func (d *dockerRuntime) validateComposeMounts(mounts []composeDesktopMount, service string) error {
	expected := map[string]composeDesktopMount{
		"/data": {Type: "volume", Source: "app_data"}, "/downloads": {Type: "volume", Source: "downloads", ReadOnly: true}, "/run/nas-engines": {Type: "volume", Source: "connections", ReadOnly: true}, "/run/nas-engine-helper": {Type: "volume", Source: "ipc", ReadOnly: true}, "/media": {Type: "bind", Source: d.config.MediaRoots[0], ReadOnly: true},
	}
	if service == "nas-engine-helper" {
		expected = map[string]composeDesktopMount{"/var/run/docker.sock": {Type: "bind", Source: "/var/run/docker.sock"}, "/managed/state": {Type: "volume", Source: "state"}, "/managed/qbittorrent": {Type: "volume", Source: "qbittorrent"}, "/managed/jackett": {Type: "volume", Source: "jackett"}, "/managed/downloads": {Type: "volume", Source: "downloads"}, "/managed/connections": {Type: "volume", Source: "connections"}, "/ipc": {Type: "volume", Source: "ipc"}}
	}
	if len(mounts) != len(expected) {
		return errors.New("Compose 挂载数量与固定服务角色不匹配")
	}
	for _, mount := range mounts {
		want, present := expected[mount.Target]
		if !present || mount.Type != want.Type || mount.ReadOnly != want.ReadOnly {
			return errors.New("Compose 挂载路径、类型或写权限与固定模板不匹配")
		}
		delete(expected, mount.Target)
		if mount.Type == "volume" {
			if mount.Source != want.Source || mount.Bind != nil {
				return errors.New("Compose 挂载使用了未知角色卷或附加宿主参数")
			}
		} else if mount.Target == "/media" {
			if !desktopBindMatches(mount.Source, want.Source) || mount.Bind == nil || mount.Bind.CreateHostPath || mount.Volume != nil {
				return errors.New("Compose 媒体挂载必须为已授权的现存只读目录")
			}
		} else if mount.Source != want.Source || mount.Volume != nil {
			return errors.New("Compose Docker socket 挂载与固定模板不匹配")
		}
	}
	return nil
}
