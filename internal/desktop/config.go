// Package desktop 封装 Windows 启动体验，媒体业务仍由共享 app 包提供。
package desktop

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config 是每个运行模式独立保存的本机启动配置，不存引擎凭据。
type Config struct {
	SchemaVersion  int      `json:"schemaVersion"`  // 配置格式版本，当前为 2；兼容显式迁移旧版 1。
	InstanceID     string   `json:"instanceId"`     // 随机安装身份，用于隔离本实例资源。
	Mode           string   `json:"mode"`           // native 原生媒体模式或 docker 完整模式。
	DataDir        string   `json:"dataDir"`        // 本地 SQLite/缓存目录，不能使用 UNC 网络共享。
	MediaRoots     []string `json:"mediaRoots"`     // 本机用户明确授权的媒体根目录数组。
	Port           int      `json:"port"`           // 首选本机回环端口，冲突后有限重试。
	AutoOpen       bool     `json:"autoOpen"`       // 服务真实就绪后是否自动打开系统浏览器。
	DockerContext  string   `json:"dockerContext"`  // 仅允许明确选中的本机 Desktop Linux context。
	ImageManifest  string   `json:"imageManifest"`  // 固定镜像清单文件；相对路径按程序目录解析。
	CollectionMode string   `json:"collectionMode"` // 本机获取方式：disabled 关闭、managed 托管、external 外接执行端。
}

// Options 保存显式命令行参数，与持久配置分开以保证优先级。
type Options struct {
	Profile        string   // 配置/日志目录；空值使用当前用户固定位置。
	Mode           string   // 显式选择 native 或 docker。
	DataDir        string   // 显式指定本地业务数据目录。
	MediaRoots     []string // 重复 --media-root 明确授权多个目录。
	Port           int      // 显式首选端口；零表示未指定。
	NoBrowser      bool     // 本次运行不自动打开网页。
	Headless       bool     // 无托盘诊断运行，不弹出目录选择或错误对话框。
	Control        string   // 对已运行实例执行 open/status/quit 或固定 collection-enable/disable/check 动作。
	PrintConfig    bool     // 仅输出脱敏启动配置，不启动服务。
	DockerContext  string   // 显式本机 Desktop context。
	ImageManifest  string   // 显式固定镜像清单位置。
	DockerAction   string   // Docker 模式的 start/stop/status 操作，默认 start。
	CollectionMode string   // --collection-mode：本次明确选择 disabled、managed 或 external，并保存到本机配置。
}

// stringsFlag 实现可重复目录参数，避免 Windows 路径分隔符歧义。
type stringsFlag []string

// String 返回参数列表的可读表示，不包含私密值。
func (v *stringsFlag) String() string { return strings.Join(*v, ";") }

// Set 记录一项明确的媒体目录授权。
func (v *stringsFlag) Set(value string) error { *v = append(*v, value); return nil }

// ParseOptions 严格解析启动参数，未知参数立即失败。
func ParseOptions(args []string, output io.Writer) (Options, error) {
	var o Options
	var roots stringsFlag
	f := flag.NewFlagSet("栖库", flag.ContinueOnError)
	f.SetOutput(output)
	f.StringVar(&o.Profile, "profile", "", "本机配置与日志目录，默认当前用户 LocalAppData/Qiku/profiles/模式")
	f.StringVar(&o.Mode, "mode", "native", "运行模式：native 原生媒体，docker 完整容器模式")
	f.StringVar(&o.DataDir, "data-dir", "", "本地业务数据目录，保留已有账号、索引与缓存")
	f.Var(&roots, "media-root", "授权媒体根目录，可以重复指定")
	f.IntVar(&o.Port, "port", 0, "首选回环端口，默认 8090，范围 1024–65515")
	f.BoolVar(&o.NoBrowser, "no-browser", false, "本次启动不自动打开浏览器")
	f.BoolVar(&o.Headless, "headless", false, "诊断运行：不使用托盘、不弹窗、不自动打开浏览器")
	f.StringVar(&o.Control, "control", "", "向已有实例发送 open、status、quit、collection-enable、collection-disable 或 collection-check")
	f.BoolVar(&o.PrintConfig, "print-config", false, "仅输出本机启动配置，不启动应用")
	f.StringVar(&o.DockerContext, "docker-context", "", "明确使用的本机 Docker Desktop Linux context")
	f.StringVar(&o.ImageManifest, "image-manifest", "", "固定版本/摘要的 Docker 镜像清单路径")
	f.StringVar(&o.DockerAction, "docker-action", "start", "Docker 固定操作：start、stop 或 status")
	f.StringVar(&o.CollectionMode, "collection-mode", "", "原生获取方式：disabled 关闭、managed 托管漫画运行时、external 使用外接执行端")
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if f.NArg() != 0 {
		return o, errors.New("不支持位置参数，请使用明确的启动选项")
	}
	o.MediaRoots = []string(roots)
	if o.Mode != "native" && o.Mode != "docker" {
		return o, errors.New("运行模式必须为 native 或 docker")
	}
	if o.Control != "" && !validControlAction(o.Control) {
		return o, errors.New("控制操作仅支持 open、status、quit、collection-enable、collection-disable 或 collection-check")
	}
	if o.CollectionMode != "" && !validCollectionMode(o.CollectionMode) {
		return o, errors.New("本机获取方式必须为 disabled、managed 或 external")
	}
	if o.DockerAction != "start" && o.DockerAction != "stop" && o.DockerAction != "status" {
		return o, errors.New("Docker 操作仅支持 start、stop 或 status")
	}
	return o, nil
}

// profilePath 选择固定配置位置，避免当前工作目录变化产生空库。
func profilePath(o Options) (string, error) {
	if o.Profile != "" {
		return filepath.Abs(o.Profile)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", errors.New("无法定位当前用户的本机应用目录")
	}
	return filepath.Join(base, "Qiku", "profiles", o.Mode), nil
}

// loadConfig 有界读取严格格式，损坏配置不覆盖、不偷偷生成新数据库。
func loadConfig(profile string, mode string) (Config, bool, error) {
	c := Config{SchemaVersion: 2, Mode: mode, DataDir: filepath.Join(profile, "data"), MediaRoots: []string{}, Port: 8090, AutoOpen: true, DockerContext: "desktop-linux", ImageManifest: "release-images.json", CollectionMode: defaultCollectionMode(mode)}
	file, err := os.Open(filepath.Join(profile, "desktop.json"))
	if errors.Is(err, os.ErrNotExist) {
		id := make([]byte, 16)
		if _, err = rand.Read(id); err != nil {
			return c, false, err
		}
		c.InstanceID = hex.EncodeToString(id)
		return c, true, nil
	}
	if err != nil {
		return c, false, errors.New("本机配置不可读取，请查看配置目录权限")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil || len(data) > 65536 {
		return c, false, errors.New("本机配置读取失败或超过 64 KiB")
	}
	var saved Config
	if err = decodeStrictJSON(data, &saved); err != nil {
		return c, false, errors.New("本机配置格式无效，已保留原文件；请恢复备份或修正 desktop.json")
	}
	// 已保存配置不能套用默认值，否则缺字段会悄悄建立另一份空库。
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return c, false, errors.New("本机配置字段不可读取，已保留原文件")
	}
	for _, field := range []string{"schemaVersion", "instanceId", "mode", "dataDir", "mediaRoots", "port", "autoOpen", "dockerContext", "imageManifest"} {
		value, present := fields[field]
		if !present || strings.TrimSpace(string(value)) == "null" {
			return c, false, errors.New("本机配置缺少必需字段或包含 null，已保留原文件；请恢复完整 desktop.json")
		}
	}
	if (saved.SchemaVersion != 1 && saved.SchemaVersion != 2) || saved.Mode != mode || len(saved.InstanceID) != 32 || saved.InstanceID != strings.ToLower(saved.InstanceID) {
		return c, false, errors.New("本机配置版本、运行模式或安装身份不匹配")
	}
	if saved.SchemaVersion == 1 {
		if _, present := fields["collectionMode"]; present {
			return c, false, errors.New("旧版本配置不能混入新版获取字段，已保留原文件")
		}
		// 旧配置只保留显式外接语义，不能因程序升级自动接管获取服务。
		saved.CollectionMode = "disabled"
		if hasExternalCollectionEnvironment() {
			saved.CollectionMode = "external"
		}
		saved.SchemaVersion = 2
	} else if raw, present := fields["collectionMode"]; !present || strings.TrimSpace(string(raw)) == "null" || !validCollectionMode(saved.CollectionMode) {
		return c, false, errors.New("本机配置缺少有效获取方式，已保留原文件")
	}
	if _, err = hex.DecodeString(saved.InstanceID); err != nil {
		return c, false, errors.New("本机安装身份无效")
	}
	if !filepath.IsAbs(saved.DataDir) || len(saved.MediaRoots) == 0 || strings.TrimSpace(saved.DockerContext) == "" || strings.TrimSpace(saved.ImageManifest) == "" {
		return c, false, errors.New("已保存的数据、媒体或模式配置不完整，拒绝按当前工作目录创建新库")
	}
	for _, root := range saved.MediaRoots {
		if !filepath.IsAbs(root) {
			return c, false, errors.New("已保存的媒体根必须为绝对路径，拒绝按当前工作目录重定向")
		}
	}
	saved, err = validateConfig(saved)
	return saved, false, err
}

// applyOptions 按显式参数、环境变量、本机配置的顺序合并，不修改全局环境。
func applyOptions(c Config, o Options) (Config, error) {
	if value := os.Getenv("NAS_DATA_DIR"); value != "" {
		c.DataDir = value
	}
	if value := os.Getenv("NAS_MEDIA_ROOTS"); value != "" {
		c.MediaRoots = filepath.SplitList(value)
	}
	if value := os.Getenv("NAS_ADDR"); value != "" {
		host, port, err := net.SplitHostPort(value)
		if err != nil || host != "127.0.0.1" {
			return c, errors.New("桌面模式 NAS_ADDR 必须为 127.0.0.1:端口；局域网服务请使用原生 nas CLI")
		}
		c.Port, err = strconv.Atoi(port)
		if err != nil {
			return c, errors.New("桌面监听端口无效")
		}
	}
	if o.DataDir != "" {
		c.DataDir = o.DataDir
	}
	if len(o.MediaRoots) > 0 {
		c.MediaRoots = append([]string(nil), o.MediaRoots...)
	}
	if o.Port != 0 {
		c.Port = o.Port
	}
	if o.DockerContext != "" {
		c.DockerContext = o.DockerContext
	}
	if o.ImageManifest != "" {
		c.ImageManifest = o.ImageManifest
	}
	if value := os.Getenv("NAS_DESKTOP_COLLECTION_MODE"); value != "" {
		c.CollectionMode = value
	}
	if o.CollectionMode != "" {
		c.CollectionMode = o.CollectionMode
	}
	return validateConfig(c)
}

// validateConfig 规范化本地存储和目录授权，不要求已保存的网络媒体此刻在线。
func validateConfig(c Config) (Config, error) {
	if !validCollectionMode(c.CollectionMode) || c.Mode == "docker" && c.CollectionMode == "managed" {
		return c, errors.New("本机获取方式无效；托管获取仅适用于 native 模式")
	}
	if strings.TrimSpace(c.DataDir) == "" || strings.ContainsRune(c.DataDir, 0) {
		return c, errors.New("SQLite 数据目录不能为空或包含无效字符")
	}
	if c.Port < 1024 || c.Port > 65515 {
		return c, errors.New("首选端口必须在 1024–65515 之间")
	}
	var err error
	c.DataDir, err = filepath.Abs(c.DataDir)
	if err != nil || strings.HasPrefix(c.DataDir, `\\`) {
		return c, errors.New("SQLite 数据目录必须为本地绝对路径，不能使用 UNC 网络共享")
	}
	if len(c.MediaRoots) > 32 {
		return c, errors.New("最多授权 32 个媒体根目录")
	}
	roots := make([]string, 0, len(c.MediaRoots))
	seen := make(map[string]bool)
	for _, root := range c.MediaRoots {
		if strings.TrimSpace(root) == "" || strings.ContainsRune(root, 0) {
			return c, errors.New("媒体根目录不能为空或包含无效字符")
		}
		absolute, e := filepath.Abs(root)
		if e != nil {
			return c, errors.New("媒体根目录无法规范化")
		}
		key := strings.ToLower(filepath.Clean(absolute))
		if !seen[key] {
			roots = append(roots, absolute)
			seen[key] = true
		}
	}
	c.MediaRoots = roots
	// 数据与普通媒体授权必须分离，避免扫描 SQLite、缓存或私密连接配置。
	dataPath := canonicalConfigPath(c.DataDir)
	for _, root := range roots {
		mediaPath := canonicalConfigPath(root)
		if pathContains(dataPath, mediaPath) || pathContains(mediaPath, dataPath) {
			return c, errors.New("本地数据目录与媒体授权根不能互相包含，请选择独立目录")
		}
	}
	return c, nil
}

// canonicalConfigPath 解析已有父目录的 junction/符号链接，保留尚未创建的末级目录。
func canonicalConfigPath(path string) string {
	current := filepath.Clean(path)
	missing := []string{}
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return resolved
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

// pathContains 按 Windows 大小写与完整路径段判断包含，拒绝前缀近似误判。
func pathContains(parent string, child string) bool {
	parent = strings.ToLower(filepath.Clean(parent))
	child = strings.ToLower(filepath.Clean(child))
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// mediaRootList 对 Windows 分号目录使用标准引号，保持重复目录参数的原样授权。
func mediaRootList(roots []string) string {
	encoded := make([]string, len(roots))
	for index, root := range roots {
		if os.PathListSeparator == ';' {
			encoded[index] = `"` + root + `"`
		} else {
			encoded[index] = root
		}
	}
	return strings.Join(encoded, string(os.PathListSeparator))
}

// saveConfig 在同目录原子替换配置，失败时保留旧文件。
func saveConfig(profile string, c Config) error {
	if err := backupV1Config(profile); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(profile, ".desktop-*.json")
	if err != nil {
		return errors.New("本机配置目录不可写")
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(append(data, '\n')); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceConfigFile(name, filepath.Join(profile, "desktop.json"))
}

// readableRoot 仅在本机用户新增授权时验证目录可读，离线旧根继续保留。
func readableRoot(root string) (string, error) {
	path, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("选择的媒体目录不可读取")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.IsDir() {
		return "", errors.New("请选择一个可读取的文件夹")
	}
	if _, err = file.ReadDir(1); err != nil && err != io.EOF {
		return "", errors.New("没有读取所选目录的权限")
	}
	return path, nil
}

// bindLoopback 保留真实绑定的 listener，避免检测端口后被第三方抢占。
func bindLoopback(port int) (net.Listener, int, error) {
	for candidate := port; candidate < port+20; candidate++ {
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(candidate)))
		if err == nil {
			return listener, candidate, nil
		}
	}
	return nil, 0, fmt.Errorf("端口 %d–%d 均不可监听，请通过 --port 选择其他范围", port, port+19)
}
