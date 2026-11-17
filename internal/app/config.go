package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Version 是当前应用版本。
const Version = "0.1.0"

// Config 定义所有可由环境变量覆盖的应用参数。
type Config struct {
	Addr                string   // HTTP 监听地址，NAS_ADDR，默认 :8090。
	DataDir             string   // SQLite 与可重建缓存目录，NAS_DATA_DIR，默认 ./data。
	MediaRoots          []string // 允许纳管的媒体根目录，NAS_MEDIA_ROOTS，以系统路径分隔符分开。
	WebDir              string   // 编译后的前端资源目录，NAS_WEB_DIR，默认 web/dist。
	ScanIntervalMinutes int      // 全库定期对账间隔分钟数，NAS_SCAN_INTERVAL_MINUTES，默认 60。
	CacheMaxBytes       int64    // 封面缓存容量字节数，NAS_CACHE_MAX_MB，默认 1024 MiB。
	MinFreeBytes        uint64   // 生成衍生物前要求的剩余空间，NAS_MIN_FREE_MB，默认 512 MiB。
	CookieSecure        bool     // 是否仅通过 HTTPS 发送会话，NAS_COOKIE_SECURE，默认 false。
	Workers             int      // 受控后台任务并发数，NAS_WORKERS，默认 1，范围 1–4。
}

// LoadConfig 读取环境参数并验证范围，不修改全局运行环境。
func LoadConfig() (Config, error) {
	c := Config{Addr: env("NAS_ADDR", ":8090"), DataDir: env("NAS_DATA_DIR", "data"), WebDir: env("NAS_WEB_DIR", "web/dist")}
	var err error
	c.DataDir, err = filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	for _, root := range filepath.SplitList(env("NAS_MEDIA_ROOTS", "media")) {
		if strings.TrimSpace(root) == "" {
			continue
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return c, err
		}
		c.MediaRoots = append(c.MediaRoots, filepath.Clean(root))
	}
	if len(c.MediaRoots) == 0 {
		return c, fmt.Errorf("NAS_MEDIA_ROOTS 至少需要一个目录")
	}
	c.ScanIntervalMinutes, err = envInt("NAS_SCAN_INTERVAL_MINUTES", 60, 1, 10080)
	if err != nil {
		return c, err
	}
	cacheMB, err := envInt("NAS_CACHE_MAX_MB", 1024, 16, 1048576)
	if err != nil {
		return c, err
	}
	c.CacheMaxBytes = int64(cacheMB) * 1024 * 1024
	freeMB, err := envInt("NAS_MIN_FREE_MB", 512, 0, 1048576)
	if err != nil {
		return c, err
	}
	c.MinFreeBytes = uint64(freeMB) * 1024 * 1024
	c.Workers, err = envInt("NAS_WORKERS", 1, 1, 4)
	if err != nil {
		return c, err
	}
	c.CookieSecure, err = strconv.ParseBool(env("NAS_COOKIE_SECURE", "false"))
	if err != nil {
		return c, fmt.Errorf("NAS_COOKIE_SECURE 必须为 true 或 false")
	}
	return c, nil
}

// env 返回非空环境变量或其默认值。
func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// envInt 将配置解析为范围内的整数，拒绝拼写错误或溢出。
func envInt(key string, fallback int, minValue int, maxValue int) (int, error) {
	value, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
	if err != nil || value < minValue || value > maxValue {
		return 0, fmt.Errorf("%s 必须在 %d–%d 之间", key, minValue, maxValue)
	}
	return value, nil
}
