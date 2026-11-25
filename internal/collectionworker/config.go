// Package collectionworker 提供固定获取适配器和隔离的持久操作执行端。
package collectionworker

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Config 是只能由受信任部署环境提供的执行端配置。
type Config struct {
	Addr             string         // NAS_COLLECTION_WORKER_ADDR：监听地址，默认127.0.0.1:8092，容器需显式配置:8092。
	DataDir          string         // NAS_COLLECTION_WORKER_DATA_DIR：本实例专用持久根，默认collection-data。
	TokenFile        string         // NAS_COLLECTION_WORKER_TOKEN_FILE：专用认证文件，不存在时生成随机令牌，必须显式设置。
	SourcesFile      string         // NAS_COLLECTION_WORKER_SOURCES_FILE：授权来源JSON文件，缺失时使用空来源。
	RcloneConfigFile string         // NAS_COLLECTION_WORKER_RCLONE_CONFIG_FILE：rclone私密配置，缺失时禁用远程复制。
	YTDLP            string         // NAS_COLLECTION_WORKER_YTDLP：受信yt-dlp绝对路径或固定工具名，默认yt-dlp。
	Rclone           string         // NAS_COLLECTION_WORKER_RCLONE：受信rclone绝对路径或固定工具名，默认rclone。
	FFmpeg           string         // NAS_COLLECTION_WORKER_FFMPEG：受信ffmpeg绝对路径或固定工具名，默认ffmpeg。
	JMComicEnabled   bool           // NAS_COLLECTION_WORKER_JMCOMIC_ENABLED：显式启用固定漫画插件，原生默认false。
	JMComicPython    string         // NAS_COLLECTION_WORKER_JMCOMIC_PYTHON：安装固定依赖的受信Python，默认python3，Windows默认python。
	MaxBytes         int64          // NAS_COLLECTION_WORKER_MAX_BYTES：单任务字节上限，默认20GiB。
	MaxFiles         int            // NAS_COLLECTION_WORKER_MAX_FILES：单任务文件数上限，默认1000。
	MinFreeMB        int64          // NAS_COLLECTION_WORKER_MIN_FREE_MB：磁盘保留MiB，默认512。
	MaxConcurrent    int            // NAS_COLLECTION_WORKER_MAX_CONCURRENT：同时执行数，默认2，范围1–4。
	Sources          []SourceConfig // 仅从受信配置文件读取的授权来源。
}

// SourceConfig 把公开来源标识绑定到管理员授权的真实根。
type SourceConfig struct {
	ID   string `json:"id"`   // 对外来源标识，不能包含路径。
	Kind string `json:"kind"` // local或remote。
	Name string `json:"name"` // 公开显示名称。
	Path string `json:"path"` // local的规范绝对根，或remote的具名远程前缀。
}

// safeID 仅允许不携带路径或命令语义的固定标识。
var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)

// LoadConfig 解析参数并拒绝越界、目录重叠和任意远程连接字符串。
func LoadConfig() (Config, error) {
	c := Config{Addr: env("NAS_COLLECTION_WORKER_ADDR", "127.0.0.1:8092"), DataDir: env("NAS_COLLECTION_WORKER_DATA_DIR", "collection-data"), TokenFile: os.Getenv("NAS_COLLECTION_WORKER_TOKEN_FILE"), SourcesFile: os.Getenv("NAS_COLLECTION_WORKER_SOURCES_FILE"), RcloneConfigFile: os.Getenv("NAS_COLLECTION_WORKER_RCLONE_CONFIG_FILE"), YTDLP: env("NAS_COLLECTION_WORKER_YTDLP", "yt-dlp"), Rclone: env("NAS_COLLECTION_WORKER_RCLONE", "rclone"), FFmpeg: env("NAS_COLLECTION_WORKER_FFMPEG", "ffmpeg")}
	if _, _, err := net.SplitHostPort(c.Addr); err != nil {
		return c, fmt.Errorf("获取服务监听地址需要主机和端口")
	}
	if strings.TrimSpace(c.TokenFile) == "" {
		return c, fmt.Errorf("必须配置获取服务专用令牌文件")
	}
	var err error
	switch env("NAS_COLLECTION_WORKER_JMCOMIC_ENABLED", "false") {
	case "true":
		c.JMComicEnabled = true
	case "false":
		c.JMComicEnabled = false
	default:
		return c, fmt.Errorf("NAS_COLLECTION_WORKER_JMCOMIC_ENABLED 必须为true或false")
	}
	python := "python3"
	if runtime.GOOS == "windows" {
		python = "python"
	}
	c.JMComicPython = env("NAS_COLLECTION_WORKER_JMCOMIC_PYTHON", python)
	for _, field := range []*string{&c.DataDir, &c.TokenFile} {
		*field, err = filepath.Abs(*field)
		if err != nil {
			return c, fmt.Errorf("获取服务路径无效")
		}
	}
	if c.DataDir == filepath.VolumeName(c.DataDir)+string(filepath.Separator) {
		return c, fmt.Errorf("获取服务不能使用文件系统根目录")
	}
	for _, field := range []*string{&c.SourcesFile, &c.RcloneConfigFile} {
		if *field != "" {
			*field, err = filepath.Abs(*field)
			if err != nil {
				return c, fmt.Errorf("私密配置路径无效")
			}
		}
	}
	for _, secretPath := range []string{c.TokenFile, c.SourcesFile, c.RcloneConfigFile} {
		if secretPath != "" && (within(filepath.Join(c.DataDir, "staging"), secretPath) || within(filepath.Join(c.DataDir, "complete"), secretPath) || within(filepath.Join(c.DataDir, "probes"), secretPath)) {
			return c, fmt.Errorf("私密配置不能存入可发布产物或映射探针目录")
		}
	}
	for _, item := range []struct {
		key                string
		fallback, min, max int64
		dst                *int64
	}{{"NAS_COLLECTION_WORKER_MAX_BYTES", 20 << 30, 1 << 20, 1 << 40, &c.MaxBytes}, {"NAS_COLLECTION_WORKER_MIN_FREE_MB", 512, 0, 1 << 20, &c.MinFreeMB}} {
		*item.dst, err = strconv.ParseInt(env(item.key, strconv.FormatInt(item.fallback, 10)), 10, 64)
		if err != nil || *item.dst < item.min || *item.dst > item.max {
			return c, fmt.Errorf("%s超出允许范围", item.key)
		}
	}
	for _, item := range []struct {
		key                string
		fallback, min, max int
		dst                *int
	}{{"NAS_COLLECTION_WORKER_MAX_FILES", 1000, 1, 10000, &c.MaxFiles}, {"NAS_COLLECTION_WORKER_MAX_CONCURRENT", 2, 1, 4, &c.MaxConcurrent}} {
		*item.dst, err = strconv.Atoi(env(item.key, strconv.Itoa(item.fallback)))
		if err != nil || *item.dst < item.min || *item.dst > item.max {
			return c, fmt.Errorf("%s超出允许范围", item.key)
		}
	}
	c.Sources = []SourceConfig{}
	if c.SourcesFile != "" {
		f, openErr := os.Open(c.SourcesFile)
		if openErr != nil && !os.IsNotExist(openErr) {
			return c, fmt.Errorf("无法读取授权来源配置")
		}
		if openErr == nil {
			defer f.Close()
			decoder := json.NewDecoder(io.LimitReader(f, 65537))
			decoder.DisallowUnknownFields()
			if err = decoder.Decode(&c.Sources); err != nil {
				return c, fmt.Errorf("授权来源配置不是有效JSON数组")
			}
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				return c, fmt.Errorf("授权来源配置包含多余内容")
			}
		}
	}
	if len(c.Sources) > 64 {
		return c, fmt.Errorf("授权来源最多64个")
	}
	seen := map[string]bool{}
	for i := range c.Sources {
		s := &c.Sources[i]
		if !safeID.MatchString(s.ID) || seen[s.ID] || len(s.Name) > 240 || strings.TrimSpace(s.Name) == "" {
			return c, fmt.Errorf("授权来源标识或名称无效")
		}
		seen[s.ID] = true
		switch s.Kind {
		case "local":
			if !filepath.IsAbs(s.Path) || filepath.Clean(s.Path) != s.Path {
				return c, fmt.Errorf("本地授权来源必须是规范绝对目录")
			}
			if within(c.DataDir, s.Path) || within(s.Path, c.DataDir) {
				return c, fmt.Errorf("来源目录不能与获取服务数据目录重叠")
			}
			for _, secretPath := range []string{c.TokenFile, c.SourcesFile, c.RcloneConfigFile} {
				if secretPath != "" && within(s.Path, secretPath) {
					return c, fmt.Errorf("授权来源不能包含获取服务的私密配置")
				}
			}
		case "remote":
			colon := strings.IndexByte(s.Path, ':')
			if colon < 1 || !safeID.MatchString(s.Path[:colon]) || strings.ContainsAny(s.Path, "\x00\r\n\\") || strings.Contains(s.Path[colon+1:], "..") || strings.HasPrefix(s.Path[colon+1:], "/") {
				return c, fmt.Errorf("远程来源必须为预配置名称和安全相对前缀")
			}
		default:
			return c, fmt.Errorf("授权来源类型仅支持local或remote")
		}
	}
	return c, nil
}

// env 返回已注明默认值的环境参数。
func env(name string, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// within 以路径分段判断根内关系，不用字符串前缀模拟边界。
func within(root string, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)))
}

// toolPath 只解析部署配置中的固定工具，返回规范绝对路径。
func toolPath(value string) string {
	if value == "" || !filepath.IsAbs(value) && strings.ContainsAny(value, "/\\") {
		return ""
	}
	p, err := exec.LookPath(value)
	if err != nil || !filepath.IsAbs(p) {
		return ""
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return ""
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(filepath.Ext(p), ".exe") {
			return ""
		}
	} else if info.Mode().Perm()&0111 == 0 {
		return ""
	}
	return p
}

// ensureToken 仅创建缺失的专用令牌文件，绝不替换既有秘密。
func ensureToken(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("无法建立令牌目录")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil || !sameNativePath(filepath.Dir(path), parent) {
		return "", fmt.Errorf("令牌目录不能包含链接别名")
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		f, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if createErr == nil {
			_, err = f.WriteString(rand.Text() + rand.Text() + "\n")
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				return "", fmt.Errorf("无法生成获取服务令牌")
			}
		}
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256 {
		return "", fmt.Errorf("获取服务令牌文件无效")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("获取服务令牌文件权限必须为0600")
	}
	data, err := os.ReadFile(path)
	token := strings.TrimSpace(string(data))
	if err != nil || len(token) < 32 || len(token) > 200 || strings.ContainsAny(token, " \t\r\n") {
		return "", fmt.Errorf("获取服务令牌长度或格式无效")
	}
	return token, nil
}
