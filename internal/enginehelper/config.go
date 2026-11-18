// Package enginehelper 提供仅经私有 Unix socket 访问的固定模板部署服务。
package enginehelper

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config 是只能由受信任部署环境指定的助手参数。
type Config struct {
	HostRoot     string // ENGINE_HOST_ROOT：Docker宿主专用绝对根，必须对应 /managed bind source。
	Root         string // ENGINE_MANAGED_ROOT：助手内部专用挂载点，默认 /managed。
	Socket       string // ENGINE_HELPER_SOCKET：私有HTTP Unix socket，默认 /ipc/helper.sock。
	DockerSocket string // ENGINE_DOCKER_SOCKET：本机Docker Unix socket，默认 /var/run/docker.sock。
	Network      string // ENGINE_NETWORK：已存在的应用专用可出站bridge网络。
	UID          int    // ENGINE_UID：专用文件及下载进程用户ID，默认1000。
	GID          int    // ENGINE_GID：应用可读取连接及socket的组ID，默认1000。
	PeerPort     int    // QBT_PEER_PORT：固定宿主及容器BT TCP/UDP端口，默认6881。
	SetupPort    int    // JACKETT_SETUP_PORT：Jackett宿主回环配置端口，默认19117。
}

// LoadConfig 读取并拒绝不受支持或相互覆盖的部署路径。
func LoadConfig() (Config, error) {
	c := Config{HostRoot: os.Getenv("ENGINE_HOST_ROOT"), Root: setting("ENGINE_MANAGED_ROOT", "/managed"), Socket: setting("ENGINE_HELPER_SOCKET", "/ipc/helper.sock"), DockerSocket: setting("ENGINE_DOCKER_SOCKET", "/var/run/docker.sock"), Network: os.Getenv("ENGINE_NETWORK")}
	for _, p := range []string{c.HostRoot, c.Root, c.Socket, c.DockerSocket} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p || p == string(filepath.Separator) {
			return c, fmt.Errorf("助手配置需要规范的专用绝对路径")
		}
	}
	if c.Network == "" || len(c.Network) > 128 || strings.ContainsAny(c.Network, "/\\ \t\r\n") {
		return c, fmt.Errorf("ENGINE_NETWORK 必须指定专用bridge网络")
	}
	var err error
	for _, item := range []struct {
		name     string
		fallback int
		min      int
		max      int
		target   *int
	}{{"ENGINE_UID", 1000, 1, 65535, &c.UID}, {"ENGINE_GID", 1000, 1, 65535, &c.GID}, {"QBT_PEER_PORT", 6881, 1024, 65535, &c.PeerPort}, {"JACKETT_SETUP_PORT", 19117, 1024, 65535, &c.SetupPort}} {
		*item.target, err = strconv.Atoi(setting(item.name, strconv.Itoa(item.fallback)))
		if err != nil || *item.target < item.min || *item.target > item.max {
			return c, fmt.Errorf("%s 超出允许范围", item.name)
		}
	}
	return c, nil
}

// setting 返回环境参数或已注明的默认值。
func setting(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
