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
	Engines             EngineConfig        // 私有引擎助手与运行时连接清单参数。
	Addr                string              // HTTP 监听地址，NAS_ADDR，默认 :8090。
	DataDir             string              // SQLite 与可重建缓存目录，NAS_DATA_DIR，默认 ./data。
	MediaRoots          []string            // 允许纳管的媒体根目录，NAS_MEDIA_ROOTS，以系统路径分隔符分开。
	WebDir              string              // 编译后的前端资源目录，NAS_WEB_DIR，默认 web/dist。
	WebDirExplicit      bool                // 是否显式配置 NAS_WEB_DIR；显式目录优先于发布包内嵌资源。
	FFmpegPath          string              // NAS_FFMPEG_PATH，可选 FFmpeg 绝对路径；未设置时优先包内 tools，再查系统 PATH。
	FFprobePath         string              // NAS_FFPROBE_PATH，可选 FFprobe 绝对路径；未设置时优先包内 tools，再查系统 PATH。
	ScanIntervalMinutes int                 // 全库定期对账间隔分钟数，NAS_SCAN_INTERVAL_MINUTES，默认 60。
	CacheMaxBytes       int64               // 封面缓存容量字节数，NAS_CACHE_MAX_MB，默认 1024 MiB。
	MinFreeBytes        uint64              // 生成衍生物前要求的剩余空间，NAS_MIN_FREE_MB，默认 512 MiB。
	CookieSecure        bool                // 是否仅通过 HTTPS 发送会话，NAS_COOKIE_SECURE，默认 false。
	Workers             int                 // 受控后台任务并发数，NAS_WORKERS，默认 1，范围 1–4。
	Downloads           DownloadConfig      // 独立下载模块配置；默认关闭，不影响原媒体挂载。
	TorrentSearch       TorrentSearchConfig // 独立 Torznab 搜种配置；默认关闭，不影响原 BT 和媒体接口。
}

// LoadConfig 读取环境参数并验证范围，不修改全局运行环境。
func LoadConfig() (Config, error) {
	return LoadConfigWithLookup(os.Getenv)
}

// LoadConfigWithLookup 从调用方合并后的参数快照读取并完整验证配置，不修改进程环境。
func LoadConfigWithLookup(lookup func(string) string) (Config, error) {
	source := newConfigSource(lookup)
	c := Config{Addr: source.value("NAS_ADDR", ":8090"), DataDir: source.value("NAS_DATA_DIR", "data"), WebDir: source.value("NAS_WEB_DIR", "web/dist"), WebDirExplicit: source.lookup("NAS_WEB_DIR") != "", FFmpegPath: source.lookup("NAS_FFMPEG_PATH"), FFprobePath: source.lookup("NAS_FFPROBE_PATH")}
	var err error
	for key, value := range map[string]string{"NAS_FFMPEG_PATH": c.FFmpegPath, "NAS_FFPROBE_PATH": c.FFprobePath} {
		if value != "" && !filepath.IsAbs(value) {
			return c, fmt.Errorf("%s 必须为绝对路径", key)
		}
	}
	c.DataDir, err = canonicalDataDirectory(c.DataDir)
	if err != nil {
		return c, err
	}
	for _, root := range filepath.SplitList(source.value("NAS_MEDIA_ROOTS", "media")) {
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
	c.ScanIntervalMinutes, err = source.integer("NAS_SCAN_INTERVAL_MINUTES", 60, 1, 10080)
	if err != nil {
		return c, err
	}
	cacheMB, err := source.integer("NAS_CACHE_MAX_MB", 1024, 16, 1048576)
	if err != nil {
		return c, err
	}
	c.CacheMaxBytes = int64(cacheMB) * 1024 * 1024
	freeMB, err := source.integer("NAS_MIN_FREE_MB", 512, 0, 1048576)
	if err != nil {
		return c, err
	}
	c.MinFreeBytes = uint64(freeMB) * 1024 * 1024
	c.Workers, err = source.integer("NAS_WORKERS", 1, 1, 4)
	if err != nil {
		return c, err
	}
	c.CookieSecure, err = strconv.ParseBool(source.value("NAS_COOKIE_SECURE", "false"))
	if err != nil {
		return c, fmt.Errorf("NAS_COOKIE_SECURE 必须为 true 或 false")
	}
	c.Downloads, err = loadDownloadConfigWithLookup(c, source.lookup)
	if err != nil {
		return c, err
	}
	c.TorrentSearch, err = loadTorrentSearchConfigWithLookup(source.lookup)
	if err != nil {
		return c, err
	}
	c.Engines, err = loadEngineConfigWithLookup(source.lookup)
	return c, err
}

// configSource 将配置来源固定到本次加载，避免桌面入口通过全局环境传递参数。
type configSource struct {
	lookup func(string) string // 配置键到原始文本的只读查询函数，空值表示使用默认。
}

// newConfigSource 为未传查询函数的调用保留读取系统环境的兼容行为。
func newConfigSource(lookup func(string) string) configSource {
	if lookup == nil {
		lookup = os.Getenv
	}
	return configSource{lookup: lookup}
}

// value 返回当前来源的非空参数或兼容默认值。
func (source configSource) value(key string, fallback string) string {
	if value := source.lookup(key); value != "" {
		return value
	}
	return fallback
}

// integer 从同一配置来源解析范围内整数，拒绝溢出和无效参数。
func (source configSource) integer(key string, fallback int, minValue int, maxValue int) (int, error) {
	value, err := strconv.Atoi(source.value(key, strconv.Itoa(fallback)))
	if err != nil || value < minValue || value > maxValue {
		return 0, fmt.Errorf("%s 必须在 %d–%d 之间", key, minValue, maxValue)
	}
	return value, nil
}

// env 返回非空环境变量或其默认值。
func env(key string, fallback string) string {
	return newConfigSource(os.Getenv).value(key, fallback)
}

// envInt 将配置解析为范围内的整数，拒绝拼写错误或溢出。
func envInt(key string, fallback int, minValue int, maxValue int) (int, error) {
	return newConfigSource(os.Getenv).integer(key, fallback, minValue, maxValue)
}
