package enginehelper

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"

	"nas-content-manager/internal/enginecontract"
)

// collectionDefinition 固定一个资源产品的显示、端口和私密卷契约。
type collectionDefinition struct {
	Template template          // 受核验的不可变镜像及内部端口。
	Name     string            // 产品显示名称。
	Version  string            // 固定产品版本。
	Setup    int               // 仅宿主回环的配置端口。
	Volumes  map[string]string // 固定私密卷角色到容器路径的映射。
}

// collectionDefinitionFor 仅返回与现有生产API适配器匹配的固定产品版本。
func collectionDefinitionFor(component string) (collectionDefinition, error) {
	switch component {
	case enginecontract.MoviePilot:
		return collectionDefinition{template{component, "jxxghp/moviepilot-v3:3.2.8@sha256:51cf8fe90a474bde2d79bbbccb5ce83f926b4da887b6fcc7c9457730095d6ebb", 3000}, "MoviePilot", "3.2.8", 19130, map[string]string{"config": "/config", "cache": "/moviepilot/.cache"}}, nil
	case enginecontract.AutoBangumi:
		return collectionDefinition{template{component, "estrellaxd/auto_bangumi:4.0.0@sha256:d2e2955b5e4caf345b4325db7bee150e861afaae5911181e5bd5646ddbcb66be", 7892}, "AutoBangumi", "4.0.0", 19132, map[string]string{"config": "/app/config", "database": "/app/data"}}, nil
	case enginecontract.Sonarr:
		return collectionDefinition{template{component, "linuxserver/sonarr:4.0.20.3014-ls5@sha256:c7d1df25e428da6c9ace05daea0d34e59522978a3843ea1944812f855af4086c", 8989}, "Sonarr", "4.0.20.3014", 19133, map[string]string{"config": "/config"}}, nil
	case enginecontract.Radarr:
		return collectionDefinition{template{component, "linuxserver/radarr:7.0.3.10918-ls328@sha256:044788f5508b7f67a3e0368cdd142c77c4d4cd5aec2efaba53e1a68b6b6231e7", 7878}, "Radarr", "7.0.3.10918", 19134, map[string]string{"config": "/config"}}, nil
	default:
		return collectionDefinition{}, errors.New("不支持的资源组件")
	}
}

// deploymentTemplate 执行器支持两组模板，公开旧引擎路由仍只接受原两组件。
func deploymentTemplate(component string) (template, error) {
	if enginecontract.IsCollectionComponent(component) {
		definition, err := collectionDefinitionFor(component)
		return definition.Template, err
	}
	return engineTemplate(component)
}

// collectionVolumeName 从私有安装身份派生固定卷名，不接受客户端字符串拼接。
func collectionVolumeName(installation string, component string, role string) string {
	return "nas-collection-" + installation + "-" + component + "-" + role
}

// collectionVolumeRoles 稳定排序私密卷角色，保证执行及恢复顺序一致。
func collectionVolumeRoles(definition collectionDefinition) []string {
	return slices.Sorted(maps.Keys(definition.Volumes))
}

// makeCollectionSpec 只生成固定镜像、专用网络、数据根和回环入口。
func makeCollectionSpec(c Config, installation string, t template) containerSpec {
	definition, _ := collectionDefinitionFor(t.Component)
	name := containerName(installation, t.Component)
	port := strconv.Itoa(t.Port) + "/tcp"
	s := containerSpec{Image: t.Image, Labels: map[string]string{"nas.engines.installation": installation, "nas.engines.component": t.Component, "nas.engines.template": enginecontract.CollectionTemplateVersion}, ExposedPorts: map[string]struct{}{port: {}}}
	for _, role := range collectionVolumeRoles(definition) {
		mount := volumeMount{Type: "volume", Source: collectionVolumeName(installation, t.Component, role), Target: definition.Volumes[role]}
		mount.VolumeOptions.NoCopy = true
		s.HostConfig.Mounts = append(s.HostConfig.Mounts, mount)
	}
	for _, role := range []string{"downloads", "collection-media"} {
		target := "/downloads"
		if role == "collection-media" {
			target = "/data"
		}
		if c.StorageMode == "volume" {
			mount := volumeMount{Type: "volume", Source: volumeName(c, role), Target: target}
			mount.VolumeOptions.NoCopy = true
			s.HostConfig.Mounts = append(s.HostConfig.Mounts, mount)
		} else {
			s.HostConfig.Binds = append(s.HostConfig.Binds, filepath.Join(c.HostRoot, role)+":"+target+":rw")
		}
	}
	if c.StorageMode == "volume" {
		s.Labels["nas.engines.storage"] = "volume"
		s.Labels["nas.engines.deployment"] = c.VolumePrefix
	}
	s.Env = []string{"PUID=" + strconv.Itoa(c.UID), "PGID=" + strconv.Itoa(c.GID), "TZ=Asia/Shanghai", "UMASK=022"}
	if t.Component == enginecontract.MoviePilot {
		s.Env = append(s.Env, "MOVIEPILOT_AUTO_UPDATE=false", "MOVIEPILOT_UPDATE_DEV=false")
	}
	s.HostConfig.NetworkMode = c.Network
	s.HostConfig.RestartPolicy.Name = "unless-stopped"
	s.HostConfig.SecurityOpt = []string{"no-new-privileges:true"}
	s.HostConfig.PortBindings = map[string][]portBinding{port: {{HostIP: "127.0.0.1", HostPort: strconv.Itoa(definition.Setup)}}}
	s.HostConfig.LogConfig.Type = "local"
	s.HostConfig.LogConfig.Config = map[string]string{"max-size": "10m", "max-file": "3"}
	s.NetworkingConfig.EndpointsConfig = map[string]struct{ Aliases []string }{c.Network: {Aliases: []string{name}}}
	return s
}

// validateCollectionContainer 核对固定产品容器的全部挂载、端口与宿主隔离。
func validateCollectionContainer(info *containerInfo, c Config, installation string, t template) error {
	if !c.CollectionEnabled {
		return errors.New("资源组件部署尚未启用，请先应用当前托管部署配置")
	}
	want := makeCollectionSpec(c, installation, t)
	for key, value := range want.Labels {
		if info.Config.Labels[key] != value {
			return errors.New("资源组件容器归属或模板冲突，不会接管已有容器")
		}
	}
	if info.Config.Image != t.Image || info.HostConfig.Privileged || info.HostConfig.NetworkMode != c.Network || len(info.NetworkSettings.Networks) != 1 || info.NetworkSettings.Networks[c.Network] == nil || len(info.HostConfig.Tmpfs) != 0 {
		return errors.New("资源组件镜像、网络或权限配置冲突")
	}
	if len(info.HostConfig.CapAdd) != 0 || len(info.HostConfig.CapDrop) != 0 || len(info.HostConfig.Devices) != 0 || len(info.HostConfig.DeviceRequests) != 0 || len(info.HostConfig.VolumesFrom) != 0 || info.HostConfig.PidMode != "" || info.HostConfig.IpcMode != "private" || info.HostConfig.UTSMode != "" || info.HostConfig.UsernsMode != "" || info.HostConfig.PublishAllPorts || info.HostConfig.ReadonlyRootfs || info.HostConfig.AutoRemove {
		return errors.New("资源组件包含模板之外的宿主权限或共享配置")
	}
	if !slices.Equal(info.HostConfig.SecurityOpt, want.HostConfig.SecurityOpt) || info.HostConfig.RestartPolicy.Name != want.HostConfig.RestartPolicy.Name || info.HostConfig.RestartPolicy.MaximumRetryCount != 0 || info.HostConfig.LogConfig.Type != "local" || !maps.Equal(info.HostConfig.LogConfig.Config, want.HostConfig.LogConfig.Config) {
		return errors.New("资源组件安全选项、重启策略或日志配置冲突")
	}
	if len(info.HostConfig.PortBindings) != 1 || !slices.Equal(info.HostConfig.PortBindings[fmt.Sprintf("%d/tcp", t.Port)], want.HostConfig.PortBindings[fmt.Sprintf("%d/tcp", t.Port)]) {
		return errors.New("资源组件配置入口不符合固定回环端口约束")
	}
	definition, _ := collectionDefinitionFor(t.Component)
	targets := map[string]volumeMount{}
	for _, role := range collectionVolumeRoles(definition) {
		targets[definition.Volumes[role]] = volumeMount{Type: "volume", Source: collectionVolumeName(installation, t.Component, role)}
	}
	for role, target := range map[string]string{"downloads": "/downloads", "collection-media": "/data"} {
		mount := volumeMount{Type: "bind", Source: filepath.Join(c.HostRoot, role)}
		if c.StorageMode == "volume" {
			mount.Type, mount.Source = "volume", volumeName(c, role)
		}
		targets[target] = mount
	}
	if len(info.Mounts) != len(targets) {
		return errors.New("资源组件挂载集合冲突，不会访问额外目录")
	}
	for _, actual := range info.Mounts {
		mount, exists := targets[actual.Destination]
		source := actual.Source
		if mount.Type == "volume" {
			source = actual.Name
			if actual.Driver != "local" {
				return errors.New("资源组件私密卷驱动冲突")
			}
		}
		if !exists || actual.Type != mount.Type || source != mount.Source || !actual.RW {
			return errors.New("资源组件目录身份或读写权限冲突")
		}
		delete(targets, actual.Destination)
	}
	return nil
}
