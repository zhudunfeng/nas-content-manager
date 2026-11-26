package app

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DownloadConfig 定义只在服务端使用的下载连接、凭据与专用卷配置。
type DownloadConfig struct {
	Mode         string // NAS_DOWNLOAD_MODE，disabled/external/managed；显式模式优先于旧开关。
	Enabled      bool   // 启动时是否已有完整外接连接；NAS_DOWNLOAD_ENABLED 仅在未指定新模式时生效。
	Root         string // NAS_DOWNLOAD_ROOT，应用只读的专用下载根，默认 /downloads。
	EngineRoot   string // NAS_DOWNLOAD_ENGINE_ROOT，引擎看到的同一下载根，默认 /downloads。
	URL          string // NAS_DOWNLOAD_QB_URL，固定的 HTTP(S) WebAPI 基地址，禁止内嵌凭据。
	Username     string // NAS_DOWNLOAD_QB_USERNAME，只供后端登录的 WebUI 用户名。
	PasswordFile string // NAS_DOWNLOAD_QB_PASSWORD_FILE，只读密码文件，不进入数据库或响应。
	MinFreeBytes uint64 // NAS_DOWNLOAD_MIN_FREE_GB，下载卷保留字节数，默认 2 GiB。
}

// loadDownloadConfig 读取下载参数；禁用时仍保留根路径供历史发布门禁使用。
func loadDownloadConfig(config Config) (DownloadConfig, error) {
	return loadDownloadConfigWithLookup(config, os.Getenv)
}

// loadDownloadConfigWithLookup 用最终参数验证下载隔离，避免桌面目录覆盖绕过已有校验。
func loadDownloadConfigWithLookup(config Config, lookup func(string) string) (DownloadConfig, error) {
	source := newConfigSource(lookup)
	c := DownloadConfig{Root: source.value("NAS_DOWNLOAD_ROOT", "/downloads"), EngineRoot: source.value("NAS_DOWNLOAD_ENGINE_ROOT", "/downloads"), URL: strings.TrimRight(source.lookup("NAS_DOWNLOAD_QB_URL"), "/"), Username: source.lookup("NAS_DOWNLOAD_QB_USERNAME"), PasswordFile: source.lookup("NAS_DOWNLOAD_QB_PASSWORD_FILE")}
	var err error
	c.Mode, err = loadEngineModeWithLookup("NAS_DOWNLOAD_MODE", "NAS_DOWNLOAD_ENABLED", source.lookup)
	c.Enabled = c.Mode == "external"
	if err != nil {
		return c, err
	}
	c.Root, err = filepath.Abs(c.Root)
	if err != nil {
		return c, fmt.Errorf("NAS_DOWNLOAD_ROOT 路径无效")
	}
	c.Root = filepath.Clean(c.Root)
	c.EngineRoot, err = cleanEnginePath(c.EngineRoot)
	if err != nil {
		return c, fmt.Errorf("NAS_DOWNLOAD_ENGINE_ROOT 必须为绝对目录")
	}
	freeGB, err := source.integer("NAS_DOWNLOAD_MIN_FREE_GB", 2, 0, 1048576)
	if err != nil {
		return c, err
	}
	c.MinFreeBytes = uint64(freeGB) << 30
	if c.Mode == "disabled" {
		return c, nil
	}
	if c.Mode == "external" {
		parsed, err := url.Parse(c.URL)
		if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return c, fmt.Errorf("NAS_DOWNLOAD_QB_URL 必须为不含凭据、查询或片段的 HTTP(S) 地址")
		}
		if c.Username == "" || len(c.Username) > 200 || c.PasswordFile == "" {
			return c, fmt.Errorf("启用下载必须设置 WebUI 用户名和只读密码文件")
		}
		if _, err = readDownloadPassword(c.PasswordFile); err != nil {
			return c, err
		}
	}
	canonical, err := filepath.EvalSymlinks(c.Root)
	if err != nil {
		return c, fmt.Errorf("下载根目录不可访问，请先创建并挂载专用下载卷")
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return c, fmt.Errorf("下载根目录无法规范化")
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return c, fmt.Errorf("下载根必须是可读目录")
	}
	c.Root = filepath.Clean(canonical)
	if within(c.Root, config.DataDir) || within(config.DataDir, c.Root) {
		return c, fmt.Errorf("下载卷与应用数据目录必须分离")
	}
	// 普通媒体挂载只允许授权 complete 子树，防止另一根目录把 staging 暴露给扫描器。
	complete := filepath.Join(c.Root, "complete")
	for _, root := range config.MediaRoots {
		resolved, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr != nil {
			resolved = filepath.Clean(root)
		}
		if within(resolved, c.Root) || (within(c.Root, resolved) && !within(complete, resolved)) {
			return c, fmt.Errorf("媒体根不得包含下载根或 staging；请仅授权下载根的 complete 子目录")
		}
	}
	if err = checkDownloadMountIsolation(c, config.MediaRoots); err != nil {
		return c, err
	}
	return c, nil
}

// readDownloadPassword 有界读取独立凭据文件，不把内部路径或内容放入错误。
func readDownloadPassword(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", fmt.Errorf("下载器密码文件不可读取")
	}
	defer f.Close()
	buffer, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(buffer) == 0 || len(buffer) > 4096 {
		return "", fmt.Errorf("下载器密码文件必须包含 1–4096 字节密码")
	}
	// Windows PowerShell 5.1 的 UTF8 输出默认带 BOM，只移除文件开头标记。
	buffer = bytes.TrimPrefix(buffer, []byte{0xef, 0xbb, 0xbf})
	password := strings.TrimRight(string(buffer), "\r\n")
	if password == "" || strings.ContainsAny(password, "\r\n\x00") {
		return "", fmt.Errorf("下载器密码文件格式无效")
	}
	return password, nil
}

// cleanEnginePath 规范化引擎绝对路径，按组件映射而非字符串前缀替换。
func cleanEnginePath(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	windowsPath := isWindowsEnginePath(value)
	if len(value) > 4096 || !utf8.ValidString(value) || strings.HasPrefix(value, "//") || !(strings.HasPrefix(value, "/") || windowsPath) {
		return "", fmt.Errorf("引擎绝对路径无效")
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", fmt.Errorf("引擎绝对路径含控制字符")
		}
	}
	if windowsPath {
		// 保留盘符根的斜杠；path.Clean("E:/") 会变成含歧义的盘符相对路径 E:。
		tail := strings.Trim(path.Clean(value[2:]), "/")
		for _, part := range strings.Split(value[3:], "/") {
			if part == ".." {
				return "", fmt.Errorf("引擎绝对路径不得包含上级目录")
			}
		}
		if tail != "" && (strings.ContainsAny(tail, `<>:"|?*`) || validateDownloadRelative(tail) != nil) {
			return "", fmt.Errorf("Windows 引擎绝对路径无效")
		}
		if tail == "" {
			return value[:2] + "/", nil
		}
		return value[:2] + "/" + tail, nil
	}
	return path.Clean(value), nil
}

// isWindowsEnginePath 只识别盘符绝对路径，不把 UNC、设备路径或盘符相对路径当作授权目录。
func isWindowsEnginePath(value string) bool {
	return len(value) >= 3 && value[1] == ':' && value[2] == '/' && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z'))
}

// sameEnginePath 按引擎路径格式比较目录，Windows 忽略大小写而 POSIX 保持区分。
func sameEnginePath(left string, right string) bool {
	left, leftErr := cleanEnginePath(left)
	right, rightErr := cleanEnginePath(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	if isWindowsEnginePath(left) && isWindowsEnginePath(right) {
		return strings.EqualFold(left, right)
	}
	return left == right
}

// engineTaskPath 返回仅由服务端生成的引擎保存目录。
func engineTaskPath(config DownloadConfig, relative string) string {
	return path.Join(config.EngineRoot, filepath.ToSlash(relative))
}
