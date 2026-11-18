package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// EngineConfig 定义主应用能够访问的私有助手与连接目录，不包含 Docker 权限。
type EngineConfig struct {
	Socket        string // NAS_ENGINE_HELPER_SOCKET，私有 Unix socket；空值表示未启用部署控制面。
	ConnectionDir string // NAS_ENGINE_CONNECTION_DIR，助手原子发布的只读连接目录，默认 /run/nas-engines。
}

// loadEngineMode 明确指定的新模式优先，否则兼容既有布尔开关。
func loadEngineMode(key string, legacy string) (string, error) {
	return loadEngineModeWithLookup(key, legacy, os.Getenv)
}

// loadEngineModeWithLookup 保证新模式和旧开关从同一只读配置来源判定。
func loadEngineModeWithLookup(key string, legacy string, lookup func(string) string) (string, error) {
	source := newConfigSource(lookup)
	if value := source.lookup(key); value != "" {
		if value != "disabled" && value != "external" && value != "managed" {
			return "", fmt.Errorf("%s 必须为 disabled、external 或 managed", key)
		}
		return value, nil
	}
	enabled, err := strconv.ParseBool(source.value(legacy, "false"))
	if err != nil {
		return "", fmt.Errorf("%s 必须为 true 或 false", legacy)
	}
	if enabled {
		return "external", nil
	}
	return "disabled", nil
}

// loadEngineConfig 仅验证部署参数形状；首次没有清单不妨碍应用启动。
func loadEngineConfig() (EngineConfig, error) {
	return loadEngineConfigWithLookup(os.Getenv)
}

// loadEngineConfigWithLookup 读取调用方最终选定的私有助手参数，不改变进程环境。
func loadEngineConfigWithLookup(lookup func(string) string) (EngineConfig, error) {
	source := newConfigSource(lookup)
	c := EngineConfig{Socket: source.lookup("NAS_ENGINE_HELPER_SOCKET")}
	if c.Socket != "" && !filepath.IsAbs(c.Socket) {
		return c, fmt.Errorf("NAS_ENGINE_HELPER_SOCKET 必须为绝对路径")
	}
	var err error
	c.ConnectionDir, err = filepath.Abs(source.value("NAS_ENGINE_CONNECTION_DIR", "/run/nas-engines"))
	if err != nil {
		return c, fmt.Errorf("NAS_ENGINE_CONNECTION_DIR 无效")
	}
	return c, nil
}
