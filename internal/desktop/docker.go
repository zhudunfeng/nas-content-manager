package desktop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// imageManifest 只接受发布方提供的不可变镜像摘要与可选离线归档。
type imageManifest struct {
	SchemaVersion   int    `json:"schemaVersion"`             // 清单格式版本：1为应用/助手，2增加获取服务，3启用固定资源组件部署。
	AppImage        string `json:"appImage"`                  // 应用仓库摘要或精确 sha256 镜像 ID。
	HelperImage     string `json:"helperImage"`               // Linux helper 仓库摘要或精确镜像 ID。
	CollectionImage string `json:"collectionImage,omitempty"` // v2必需、v3可选的受控获取镜像；v1保持原双服务部署。
	Archive         string `json:"archive,omitempty"`         // 可选包内相对归档路径，不得越过发布目录。
	ArchiveSHA256   string `json:"archiveSha256,omitempty"`   // 归档内容校验，存在归档时必须填写。
}

// dockerRuntime 固定本实例 Docker 命令参数，不接受任意命令模板。
type dockerRuntime struct {
	command      string        // 通过 PATH 安全解析的 Docker CLI 绝对路径。
	context      string        // 已核实指向本机 Desktop Linux daemon 的 context。
	project      string        // 从随机安装 ID 派生的安全 Compose project。
	compose      string        // 包内固定 Compose 文件的绝对路径。
	envFile      string        // 私密目录中的受管插值参数。
	environment  []string      // 清除远端/Compose 污染后的 CLI 环境。
	manifest     imageManifest // 严格校验后的不可变镜像清单。
	packageDir   string        // 可选镜像归档的受控根。
	architecture string        // 已确认的 Linux daemon 架构，首版固定 amd64。
	config       Config        // 本次固定启动配置。
}

// limitedBuffer 有界收集 CLI 诊断，原始文本不暴露给页面或常规日志。
type limitedBuffer struct {
	buffer   bytes.Buffer // 保留允许范围内的字节。
	overflow bool         // 标识 CLI 输出超过预算。
}

// Write 保持子进程可退出，并在输出超过一 MiB 时标记拒绝解析。
func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := (1 << 20) - b.buffer.Len()
	if len(data) > remaining {
		b.overflow = true
	}
	if remaining > 0 {
		_, _ = b.buffer.Write(data[:min(len(data), remaining)])
	}
	return len(data), nil
}

// dockerPrefix 以完整随机安装身份派生安全名字，不接受外部资源前缀。
func dockerPrefix(config Config) string { return "qiku-" + config.InstanceID }

// readManifest 在任何拉取/导入前验证固定镜像来源；缺少发布清单不能猜测仓库。
func readManifest(path string) (imageManifest, error) {
	var manifest imageManifest
	file, err := os.Open(path)
	if err != nil {
		return manifest, errors.New("缺少固定发布镜像清单，请使用带 release-images.json 的完整发布包；不会自动猜测仓库或使用 latest")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil || len(data) > 65536 {
		return manifest, errors.New("镜像清单不可读取或超过 64 KiB")
	}
	if decodeStrictJSON(data, &manifest) != nil {
		return manifest, errors.New("镜像清单格式无效")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return manifest, errors.New("镜像清单字段无效")
	}
	_, hasCollection := fields["collectionImage"]
	if manifest.SchemaVersion == 2 && !hasCollection || manifest.SchemaVersion == 1 && hasCollection || hasCollection && manifest.CollectionImage == "" {
		return manifest, errors.New("v2清单必须包含获取镜像，v1不接受该字段，v3可选但不能为空")
	}
	digest := regexp.MustCompile(`^(?:[a-zA-Z0-9][a-zA-Z0-9._:/-]*@)?sha256:[a-f0-9]{64}$`)
	if (manifest.SchemaVersion != 1 && manifest.SchemaVersion != 2 && manifest.SchemaVersion != 3) || !digest.MatchString(manifest.AppImage) || !digest.MatchString(manifest.HelperImage) || (hasCollection && !digest.MatchString(manifest.CollectionImage)) {
		return manifest, errors.New("应用与助手镜像必须使用固定仓库摘要或精确 sha256 镜像 ID，拒绝可变标签")
	}
	if manifest.AppImage == manifest.HelperImage {
		return manifest, errors.New("应用和助手必须使用不同的固定镜像，不能混用相同入口")
	}
	if manifest.CollectionImage != "" && (manifest.CollectionImage == manifest.AppImage || manifest.CollectionImage == manifest.HelperImage) {
		return manifest, errors.New("获取服务必须使用独立的固定镜像入口")
	}
	if (manifest.Archive == "") != (manifest.ArchiveSHA256 == "") {
		return manifest, errors.New("离线镜像归档和 SHA-256 必须同时提供")
	}
	if manifest.Archive != "" {
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(manifest.ArchiveSHA256) || !strings.HasPrefix(manifest.Archive, "images/") || strings.ContainsAny(manifest.Archive, "\\:\x00\r\n") {
			return manifest, errors.New("离线归档路径或 SHA-256 无效")
		}
		for _, segment := range strings.Split(manifest.Archive, "/") {
			if segment == "" || segment == "." || segment == ".." {
				return manifest, errors.New("离线归档必须位于 images/ 内且不能包含空路径或相对跳转")
			}
		}
	}
	return manifest, nil
}

// dockerEnvironment 清除可能把操作导向远端或覆盖受管插值参数的继承变量。
func dockerEnvironment(values map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if upper == "DOCKER_HOST" || upper == "DOCKER_CONTEXT" || upper == "DOCKER_TLS_VERIFY" || upper == "DOCKER_CERT_PATH" || upper == "DOCKER_API_VERSION" || strings.HasPrefix(upper, "COMPOSE_") || strings.HasPrefix(upper, "NAS_") || strings.HasPrefix(upper, "ENGINE_") || strings.HasPrefix(upper, "QBT_") || strings.HasPrefix(upper, "JACKETT_") || upper == "MEDIA_PATH" || upper == "WEB_PORT" || upper == "WEB_BIND_IP" {
			continue
		}
		environment = append(environment, entry)
	}
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	return environment
}

// execute 运行固定参数数组并限制时间/输出；上游错误只转换为脱敏中文阶段。
func (d *dockerRuntime) execute(ctx context.Context, stage string, timeout time.Duration, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	all := append([]string{"--context", d.context}, args...)
	command := exec.CommandContext(commandCtx, d.command, all...)
	command.Env = d.environment
	command.Dir = d.packageDir
	hideCommand(command)
	var stdout, stderr limitedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if commandCtx.Err() != nil {
		return nil, fmt.Errorf("%s超时或已取消，请确认 Docker Desktop 状态后重试", stage)
	}
	if err != nil {
		return nil, fmt.Errorf("%s失败；请检查 Docker Desktop、镜像访问权限和当前模式日志，启动器不会修改 daemon 配置", stage)
	}
	if stdout.overflow || stderr.overflow {
		return nil, fmt.Errorf("%s输出超过安全预算，已停止解析", stage)
	}
	return stdout.buffer.Bytes(), nil
}

// composeArgs 为所有 Compose 操作绑定固定文件、环境和 project，禁用默认文件发现。
func (d *dockerRuntime) composeArgs(args ...string) []string {
	base := []string{"compose", "--project-name", d.project, "--project-directory", d.packageDir, "--env-file", d.envFile, "--file", d.compose}
	if d.manifest.CollectionImage != "" {
		base = append(base, "--file", filepath.Join(d.packageDir, "compose.desktop.collection.yaml"))
	}
	if d.manifest.SchemaVersion == 3 {
		base = append(base, "--file", filepath.Join(d.packageDir, "compose.desktop.components.yaml"))
	}
	return append(base, args...)
}

// serviceImages 返回本清单允许管理的精确服务和镜像，不接管模板之外的容器。
func (d *dockerRuntime) serviceImages() map[string]string {
	images := map[string]string{"nas": d.manifest.AppImage, "nas-engine-helper": d.manifest.HelperImage}
	if d.manifest.CollectionImage != "" {
		images["nas-collection-worker"] = d.manifest.CollectionImage
	}
	return images
}

// volumeRoles 返回当前发布模板声明的全部角色，旧清单不隐式认领新增卷。
func (d *dockerRuntime) volumeRoles() map[string]string {
	roles := map[string]string{"app_data": "app-data", "state": "state", "qbittorrent": "qbittorrent", "jackett": "jackett", "downloads": "downloads", "connections": "connections", "ipc": "ipc"}
	if d.manifest.SchemaVersion == 3 {
		roles["collection_media"] = "collection-media"
	}
	if d.manifest.CollectionImage != "" {
		roles["collection_data"] = "collection-data"
		roles["collection_secrets"] = "collection-secrets"
	}
	return roles
}

// prepareDocker 仅检查本机环境和静态发布材料，不启动/安装 Docker 或修改 WSL。
func (c *controller) prepareDocker(ctx context.Context, cfg Config) (*dockerRuntime, error) {
	if len(cfg.MediaRoots) != 1 {
		return nil, errors.New("Docker 完整模式当前支持一个只读媒体根，不能静默忽略其他授权目录；请使用共同父目录或独立原生模式")
	}
	if _, err := readableRoot(cfg.MediaRoots[0]); err != nil {
		return nil, err
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`).MatchString(cfg.DockerContext) {
		return nil, errors.New("Docker context 名称无效")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.New("无法定位程序包")
	}
	packageDir := filepath.Dir(executable)
	manifestPath := cfg.ImageManifest
	if !filepath.IsAbs(manifestPath) {
		manifestPath = filepath.Join(packageDir, manifestPath)
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	compose := filepath.Join(packageDir, "compose.desktop.yaml")
	info, err := os.Stat(compose)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return nil, errors.New("缺少有效的 compose.desktop.yaml，请使用完整发布包")
	}
	command, err := exec.LookPath("docker")
	if err != nil || !filepath.IsAbs(command) {
		return nil, errors.New("未找到 Docker CLI；请先手动安装并启动 Docker Desktop，完成 Linux/WSL 前置设置；本程序不会自动安装或改系统设置")
	}
	prefix := dockerPrefix(cfg)
	d := &dockerRuntime{command: command, context: cfg.DockerContext, project: prefix, compose: compose, envFile: filepath.Join(c.profile, "docker.env"), manifest: manifest, packageDir: packageDir, config: cfg, environment: dockerEnvironment(nil)}
	data, err := d.execute(ctx, "检查 Docker context", 15*time.Second, "context", "inspect", d.context)
	if err != nil {
		return nil, err
	}
	var contexts []struct {
		Endpoints map[string]struct {
			Host string /* context 的实际 daemon 端点。 */
		} /* 命名端点集合。 */
	}
	if json.Unmarshal(data, &contexts) != nil || len(contexts) != 1 {
		return nil, errors.New("Docker context 信息无效")
	}
	host := strings.ToLower(contexts[0].Endpoints["docker"].Host)
	if host != "npipe:////./pipe/dockerdesktoplinuxengine" && host != "npipe:////./pipe/docker_engine" {
		return nil, errors.New("仅允许显式本机 Docker Desktop named-pipe context，已拒绝远端、TCP 或其他宿主的 Docker 端点")
	}
	data, err = d.execute(ctx, "连接本机 Docker Desktop", 15*time.Second, "info", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	var daemon struct {
		OSType          string // daemon 容器操作系统。
		OperatingSystem string // Docker Desktop 身份说明。
		Architecture    string // daemon 架构。
		NCPU            int    // 可用虚拟 CPU 数量。
		MemTotal        int64  // 虚拟机可用内存字节数。
	}
	if json.Unmarshal(data, &daemon) != nil || daemon.OSType != "linux" || !strings.Contains(strings.ToLower(daemon.OperatingSystem), "docker desktop") {
		return nil, errors.New("当前 context 不是本机 Docker Desktop Linux 后端，请手动切换到 Linux containers 后重试")
	}
	if daemon.NCPU < 2 || daemon.MemTotal < 2<<30 {
		return nil, errors.New("Docker Desktop 至少需要 2 个可用 CPU 和 2 GiB 内存，请手动调整资源后重试")
	}
	if daemon.Architecture != "x86_64" && daemon.Architecture != "amd64" {
		return nil, errors.New("首版 Docker Desktop 完整模式只支持 Linux amd64 后端，其他架构须单独验收")
	}
	d.architecture = "amd64"
	if _, err = d.execute(ctx, "检查 Compose 插件", 15*time.Second, "compose", "version", "--short"); err != nil {
		return nil, err
	}
	values := map[string]string{"NAS_APP_IMAGE": manifest.AppImage, "NAS_HELPER_IMAGE": manifest.HelperImage, "NAS_DESKTOP_PREFIX": prefix, "ENGINE_VOLUME_PREFIX": prefix, "ENGINE_NETWORK": prefix + "-engines", "MEDIA_PATH": cfg.MediaRoots[0], "WEB_PORT": strconv.Itoa(cfg.Port), "WEB_BIND_IP": "127.0.0.1", "QBT_PEER_PORT": "6881", "JACKETT_SETUP_PORT": "19117"}
	if manifest.CollectionImage != "" {
		values["NAS_COLLECTION_IMAGE"] = manifest.CollectionImage
	}
	if manifest.SchemaVersion == 3 {
		// 媒体根由清单能力派生，不能从继承环境额外授权目录。
		values["NAS_DESKTOP_MEDIA_ROOTS"] = d.serviceEnvironment("nas")["NAS_MEDIA_ROOTS"]
	}
	d.environment = dockerEnvironment(values)
	// 空 env 文件阻止 Compose 隐式读取发布目录中的 .env；值只由经过校验的参数快照提供。
	if err = os.WriteFile(d.envFile, []byte("# 由栖库启动器管理；插值参数由私密进程环境提供，不保存凭据。\n"), 0600); err != nil {
		return nil, errors.New("无法创建本模式的 Compose 参数文件")
	}
	if err = d.validateCompose(ctx); err != nil {
		return nil, err
	}
	if err = d.verifyExisting(ctx, false); err != nil {
		return nil, err
	}
	c.logger.Info("本机 Docker Desktop 前置检查通过", "模式", "Linux 命名卷", "实例", prefix)
	return d, nil
}

// validateCompose 校验固定模板的镜像、实例前缀、服务集合和发布端口，防止插值错配。
func (d *dockerRuntime) validateCompose(ctx context.Context) error {
	data, err := d.execute(ctx, "检查完整模式部署模板", 15*time.Second, d.composeArgs("config", "--format", "json")...)
	if err != nil {
		return err
	}
	return d.validateComposeData(data)
}

// importImages 仅导入包内且 SHA-256 匹配的归档，或拉取已固定的仓库摘要。
func (d *dockerRuntime) importImages(ctx context.Context) error {
	if d.manifest.Archive != "" {
		path := filepath.Join(d.packageDir, d.manifest.Archive)
		resolved, err := filepath.EvalSymlinks(path)
		root, rootErr := filepath.EvalSymlinks(d.packageDir)
		if err != nil || rootErr != nil {
			return errors.New("离线镜像归档不存在或不可访问")
		}
		relative, err := filepath.Rel(root, resolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return errors.New("离线镜像归档的真实路径超出发布目录")
		}
		file, err := os.Open(resolved)
		if err != nil {
			return errors.New("离线镜像归档不可读取")
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() {
			_ = file.Close()
			return errors.New("离线镜像归档必须为普通文件")
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		_ = file.Close()
		if err != nil || hex.EncodeToString(hash.Sum(nil)) != d.manifest.ArchiveSHA256 {
			return errors.New("离线镜像归档 SHA-256 不匹配，未执行导入")
		}
		if _, err = d.execute(ctx, "导入已校验的离线镜像", 15*time.Minute, "load", "--input", resolved); err != nil {
			return err
		}
	}
	for _, image := range d.serviceImages() {
		if _, err := d.imageIdentity(ctx, image); err == nil {
			continue
		}
		if strings.HasPrefix(image, "sha256:") {
			return errors.New("本机缺少清单指定的精确镜像 ID，请提供已校验离线归档或固定仓库摘要")
		}
		if _, err := d.execute(ctx, "拉取固定摘要镜像", 20*time.Minute, "pull", image); err != nil {
			return err
		}
		if _, err := d.imageIdentity(ctx, image); err != nil {
			return err
		}
	}
	app, err := d.imageIdentity(ctx, d.manifest.AppImage)
	if err != nil {
		return err
	}
	helper, err := d.imageIdentity(ctx, d.manifest.HelperImage)
	if err != nil {
		return err
	}
	if app.ID == helper.ID {
		return errors.New("应用与助手镜像解析为相同内容身份，拒绝混用固定入口")
	}
	if d.manifest.CollectionImage != "" {
		worker, workerErr := d.imageIdentity(ctx, d.manifest.CollectionImage)
		if workerErr != nil {
			return workerErr
		}
		if worker.ID == app.ID || worker.ID == helper.ID {
			return errors.New("获取服务与其他角色解析为同一镜像，拒绝混用入口")
		}
	}
	return nil
}

// dockerImage 是本机精确镜像元数据，避免容器只匹配可变名称。
type dockerImage struct {
	ID           string `json:"Id"` // 内容寻址的实际镜像 ID。
	OS           string `json:"Os"` // 固定镜像的容器操作系统，必须为 Linux。
	Architecture string // 固定镜像的 CPU 架构，必须与已验收后端一致。
	Config       struct {
		Entrypoint []string // 发行镜像的固定入口。
		Cmd        []string // 发行镜像的固定默认参数。
		Env        []string // 仅在内存核对镜像默认环境，禁止输出值。
		User       string   // 发行镜像默认执行用户。
	} // 固定镜像运行配置，环境不写入日志或持久记录。
}

// imageIdentity 解析精确镜像 ID，确保后续核验不是仅检查标签。
func (d *dockerRuntime) imageIdentity(ctx context.Context, reference string) (dockerImage, error) {
	var result dockerImage
	data, err := d.execute(ctx, "核验固定镜像", 15*time.Second, "image", "inspect", reference)
	if err != nil {
		return result, err
	}
	var images []dockerImage
	if json.Unmarshal(data, &images) != nil || len(images) != 1 {
		return result, errors.New("固定镜像身份无效")
	}
	if err = validateImagePlatform(images[0], d.architecture); err != nil {
		return result, err
	}
	return images[0], nil
}

// validateImagePlatform 同时核验内容身份和已验收的 Linux amd64 平台，拒绝隐式模拟其他架构。
func validateImagePlatform(image dockerImage, architecture string) error {
	if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(image.ID) {
		return errors.New("固定镜像身份无效")
	}
	if architecture != "amd64" || image.OS != "linux" || image.Architecture != architecture {
		return errors.New("固定镜像不是当前已验收后端所需的 Linux amd64 版本，未启动容器")
	}
	return nil
}

// dockerContainer 是核验本实例固定应用、助手和可选获取服务需要的受限 inspect 视图。
type dockerContainer struct {
	ID     string `json:"Id"` // 实际容器 ID。
	Image  string // 不可变镜像 ID。
	Config struct {
		Labels     map[string]string // Compose 与实例归属标签。
		Entrypoint []string          // 实际入口，不允许被未知模板覆盖。
		Cmd        []string          // 实际默认参数。
		Env        []string          // 实际环境，仅用于精确核验，禁止输出值。
		User       string            // 必须保留发行镜像默认执行用户。
	} // 容器固定配置。
	HostConfig struct {
		Privileged  bool     // 必须关闭特权模式。
		NetworkMode string   // 不能使用 host 或其他实例网络。
		PidMode     string   // 不允许共享宿主 PID。
		CapAdd      []string // 不允许附加内核能力。
		SecurityOpt []string // 必须保留禁止提权设置。
	} // 受限运行权限。
	Mounts []struct {
		Type        string // 仅允许固定 bind/volume。
		Name        string // 命名卷的精确名称。
		Source      string // 只用于核对固定 socket 或媒体 bind。
		Destination string // 固定容器路径。
		RW          bool   // 实际写权限。
	} // 实际挂载集合。
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string
		} // 实际宿主端口绑定。
		Networks map[string]json.RawMessage // 必须只加入当前实例网络。
	} // 已连接网络与端口。
	State struct{ Running bool } // 容器当前运行状态。
}

// ownedContainers 检查所有带本 project 标签的静态容器，未知资源绝不接管或停止。
func (d *dockerRuntime) ownedContainers(ctx context.Context) ([]dockerContainer, error) {
	data, err := d.execute(ctx, "查找当前实例容器", 15*time.Second, "container", "ls", "--all", "--quiet", "--filter", "label=com.docker.compose.project="+d.project)
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(data))
	if len(ids) > len(d.serviceImages()) {
		return nil, errors.New("当前 Compose project 中存在未知容器，已拒绝接管")
	}
	if len(ids) == 0 {
		return []dockerContainer{}, nil
	}
	for _, id := range ids {
		if !regexp.MustCompile(`^[a-f0-9]{12,64}$`).MatchString(id) {
			return nil, errors.New("Docker 容器标识无效")
		}
	}
	data, err = d.execute(ctx, "核验当前实例容器", 15*time.Second, append([]string{"container", "inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}
	var containers []dockerContainer
	if json.Unmarshal(data, &containers) != nil || len(containers) != len(ids) {
		return nil, errors.New("当前实例容器信息不完整")
	}
	seen := make(map[string]bool)
	for _, container := range containers {
		labels := container.Config.Labels
		service := labels["com.docker.compose.service"]
		reference, role := d.manifest.AppImage, "app"
		if service == "nas-engine-helper" {
			reference, role = d.manifest.HelperImage, "helper"
		} else if service == "nas-collection-worker" && d.manifest.CollectionImage != "" {
			reference, role = d.manifest.CollectionImage, "collection"
		} else if service != "nas" {
			return nil, errors.New("当前实例标签下存在未知服务，拒绝操作")
		}
		if seen[service] || labels["com.docker.compose.project"] != d.project || labels["nas.engines.deployment"] != d.project || labels["nas.desktop.role"] != role || strings.EqualFold(labels["com.docker.compose.oneoff"], "True") {
			return nil, errors.New("同名容器的归属或角色不匹配，拒绝操作")
		}
		seen[service] = true
		image, imageErr := d.imageIdentity(ctx, reference)
		if imageErr != nil {
			return nil, imageErr
		}
		if container.Image != image.ID || !slices.Equal(container.Config.Entrypoint, image.Config.Entrypoint) || !slices.Equal(container.Config.Cmd, image.Config.Cmd) || container.HostConfig.Privileged || container.HostConfig.PidMode != "" || len(container.HostConfig.CapAdd) > 0 {
			return nil, errors.New("同名容器的实际镜像、入口或权限不匹配，拒绝操作")
		}
		if err = d.validateContainerRuntime(container, image, service); err != nil {
			return nil, err
		}
		if len(container.NetworkSettings.Networks) != 1 || container.NetworkSettings.Networks[d.project+"-engines"] == nil {
			return nil, errors.New("容器网络不属于当前实例，拒绝操作")
		}
		if err = d.validateMounts(container, service); err != nil {
			return nil, err
		}
		if service == "nas" && container.State.Running {
			binding := container.NetworkSettings.Ports["8090/tcp"]
			if len(binding) != 1 || binding[0].HostIP != "127.0.0.1" || binding[0].HostPort != strconv.Itoa(d.config.Port) {
				return nil, errors.New("当前应用的宿主端口不是本实例回环绑定")
			}
		}
		if service != "nas" {
			for _, bindings := range container.NetworkSettings.Ports {
				if len(bindings) > 0 {
					return nil, errors.New("私有助手和获取服务不得发布宿主控制端口")
				}
			}
		}
	}
	return containers, nil
}

// validateContainerRuntime 核对实际执行用户、环境和禁止提权，防止同镜像容器覆盖固定工具或来源边界。
func (d *dockerRuntime) validateContainerRuntime(container dockerContainer, image dockerImage, service string) error {
	if container.Config.User != image.Config.User || len(container.HostConfig.SecurityOpt) != 1 || (container.HostConfig.SecurityOpt[0] != "no-new-privileges:true" && container.HostConfig.SecurityOpt[0] != "no-new-privileges") {
		return errors.New("容器实际用户或禁止提权设置与固定模板不匹配")
	}
	expected, ok := desktopEnvironmentMap(image.Config.Env)
	if !ok {
		return errors.New("固定镜像环境存在重复或无效参数")
	}
	for key, value := range d.serviceEnvironment(service) {
		expected[key] = value
	}
	actual, valid := desktopEnvironmentMap(container.Config.Env)
	if !valid || !maps.Equal(actual, expected) {
		return errors.New("容器实际环境与固定镜像及模板不匹配，拒绝接管")
	}
	return nil
}

// desktopEnvironmentMap 无损解析环境且拒绝重复参数，不在错误中回显任何值。
func desktopEnvironmentMap(entries []string) (map[string]string, bool) {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, present := strings.Cut(entry, "=")
		if !present || key == "" {
			return nil, false
		}
		if _, duplicate := values[key]; duplicate {
			return nil, false
		}
		values[key] = value
	}
	return values, true
}

// validateMounts 逐个比对角色卷与只读授权根，拒绝附加挂载和跨实例卷。
func (d *dockerRuntime) validateMounts(container dockerContainer, service string) error {
	type expectedMount struct {
		Role  string
		Write bool
	} // 固定卷角色及写权限。
	expected := map[string]expectedMount{"/data": {"app-data", true}, "/downloads": {"downloads", false}, "/run/nas-engines": {"connections", false}, "/run/nas-engine-helper": {"ipc", false}}
	if d.manifest.SchemaVersion == 3 && service == "nas" {
		expected["/collection-media"] = expectedMount{"collection-media", false}
	}
	if d.manifest.CollectionImage != "" && service == "nas" {
		expected["/collection"] = expectedMount{"collection-data", false}
		expected["/run/nas-collection"] = expectedMount{"collection-secrets", false}
	}
	if service == "nas-engine-helper" {
		expected = map[string]expectedMount{"/managed/state": {"state", true}, "/managed/qbittorrent": {"qbittorrent", true}, "/managed/jackett": {"jackett", true}, "/managed/downloads": {"downloads", true}, "/managed/connections": {"connections", true}, "/ipc": {"ipc", true}}
		if d.manifest.SchemaVersion == 3 {
			expected["/managed/collection-media"] = expectedMount{"collection-media", true}
		}
	}
	extraMounts := 1
	if service == "nas-collection-worker" && d.manifest.CollectionImage != "" {
		expected = map[string]expectedMount{"/collection": {"collection-data", true}, "/run/nas-collection": {"collection-secrets", true}}
		extraMounts = 0
	}
	if len(container.Mounts) != len(expected)+extraMounts {
		return errors.New("容器实际挂载集合与固定模板不匹配")
	}
	seen := make(map[string]bool)
	for _, mount := range container.Mounts {
		if seen[mount.Destination] {
			return errors.New("容器存在重复挂载目标")
		}
		seen[mount.Destination] = true
		if role, ok := expected[mount.Destination]; ok {
			if mount.Type != "volume" || mount.Name != d.project+"-"+role.Role || mount.RW != role.Write {
				return errors.New("容器命名卷或写权限与本实例不匹配")
			}
			continue
		}
		if service == "nas-engine-helper" && mount.Destination == "/var/run/docker.sock" && mount.Type == "bind" && mount.Source == "/var/run/docker.sock" && mount.RW {
			continue
		}
		if service == "nas" && mount.Destination == "/media" && mount.Type == "bind" && !mount.RW && desktopBindMatches(mount.Source, d.config.MediaRoots[0]) {
			continue
		}
		return errors.New("容器存在未知宿主绑定或未授权媒体目录，拒绝操作")
	}
	return nil
}

// desktopBindMatches 只允许 Windows 盘符与 Desktop 两种已知精确路径表示，不做宽泛路径替换。
func desktopBindMatches(actual string, expected string) bool {
	if strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected)) {
		return true
	}
	volume := filepath.VolumeName(expected)
	if len(volume) != 2 || volume[1] != ':' {
		return false
	}
	rest := strings.ReplaceAll(expected[len(volume):], `\`, "/")
	for _, root := range []string{"/run/desktop/mnt/host/", "/host_mnt/"} {
		if strings.EqualFold(actual, root+strings.ToLower(volume[:1])+rest) {
			return true
		}
	}
	return false
}

// verifyExisting 在创建或停止前检查现存卷和网络，保留未知资源且不使用全局 prune。
func (d *dockerRuntime) verifyExisting(ctx context.Context, requireRunning bool) error {
	data, err := d.execute(ctx, "检查当前实例卷", 15*time.Second, "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	names := strings.Fields(string(data))
	for _, role := range d.volumeRoles() {
		name := d.project + "-" + role
		if !slices.Contains(names, name) {
			continue
		}
		data, err = d.execute(ctx, "核验当前实例卷", 15*time.Second, "volume", "inspect", name)
		if err != nil {
			return err
		}
		var volumes []struct {
			Name    string
			Driver  string
			Scope   string
			Labels  map[string]string
			Options map[string]string
		} // 仅接受固定本地角色卷。
		if json.Unmarshal(data, &volumes) != nil || len(volumes) != 1 || volumes[0].Name != name || volumes[0].Driver != "local" || volumes[0].Scope != "local" || len(volumes[0].Options) != 0 || volumes[0].Labels["nas.engines.deployment"] != d.project || volumes[0].Labels["nas.engines.role"] != role {
			return errors.New("同名 Docker 卷的归属、驱动或角色不匹配，拒绝接管")
		}
	}
	data, err = d.execute(ctx, "检查当前实例网络", 15*time.Second, "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	if slices.Contains(strings.Fields(string(data)), d.project+"-engines") {
		data, err = d.execute(ctx, "核验当前实例网络", 15*time.Second, "network", "inspect", d.project+"-engines")
		if err != nil {
			return err
		}
		var networks []struct {
			Name     string
			Driver   string
			Internal bool
			Labels   map[string]string
		} // 网络身份及隔离方式。
		if json.Unmarshal(data, &networks) != nil || len(networks) != 1 || networks[0].Name != d.project+"-engines" || networks[0].Driver != "bridge" || networks[0].Internal || networks[0].Labels["com.docker.compose.project"] != d.project || networks[0].Labels["com.docker.compose.network"] != "engines" {
			return errors.New("同名 Docker 网络不属于本实例，拒绝接管")
		}
	}
	containers, err := d.ownedContainers(ctx)
	if err != nil {
		return err
	}
	if requireRunning {
		if len(containers) != len(d.serviceImages()) {
			return errors.New("应用或助手容器尚未就绪")
		}
		for _, container := range containers {
			if !container.State.Running {
				return errors.New("应用或助手容器已停止")
			}
		}
	}
	return nil
}

// startDocker 运行已具备前置条件的固定完整栈，退出启动器不会擅自停止下载。
func (c *controller) startDocker(ctx context.Context, cfg Config) error {
	d, err := c.prepareDocker(ctx, cfg)
	if err != nil {
		return err
	}
	if c.options.DockerAction == "stop" {
		err = d.stopAll(ctx)
		if err == nil {
			c.setState("stopped", "", nil)
		}
		_, reply := c.snapshot()
		_ = json.NewEncoder(os.Stdout).Encode(reply)
		c.cancel()
		return err
	}
	if c.options.DockerAction == "status" {
		err = d.verifyExisting(ctx, true)
		if err == nil {
			err = verifyReady(ctx, "http://127.0.0.1:"+strconv.Itoa(cfg.Port))
		}
		if err == nil {
			c.setState("ready", "http://127.0.0.1:"+strconv.Itoa(cfg.Port), nil)
		} else {
			c.setState("stopped", "", err)
		}
		_, reply := c.snapshot()
		_ = json.NewEncoder(os.Stdout).Encode(reply)
		c.cancel()
		return err
	}
	// 已存在的正确实例保留原端口，首次启动才有限选择空闲端口。
	containers, err := d.ownedContainers(ctx)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		listener, port, bindErr := bindLoopback(cfg.Port)
		if bindErr != nil {
			return bindErr
		}
		_ = listener.Close()
		cfg.Port = port
		d.config = cfg
		for index, entry := range d.environment {
			if strings.HasPrefix(entry, "WEB_PORT=") {
				d.environment[index] = "WEB_PORT=" + strconv.Itoa(port)
			}
		}
	}
	// 端口候选可能已改变；部署前按最终参数重新校验完整模板。
	if err = d.validateCompose(ctx); err != nil {
		return err
	}
	if err = d.importImages(ctx); err != nil {
		return err
	}
	if err = d.verifyExisting(ctx, false); err != nil {
		return err
	}
	if err = saveConfig(c.profile, cfg); err != nil {
		return err
	}
	c.mu.Lock()
	c.config = cfg
	c.mu.Unlock()
	c.logger.Info("正在启动完整模式", "实例", d.project, "下载位置", "Docker Linux 命名卷")
	if _, err = d.execute(ctx, "启动完整模式", 90*time.Second, d.composeArgs("up", "--detach", "--no-build", "--pull", "never")...); err != nil {
		return err
	}
	address := "http://127.0.0.1:" + strconv.Itoa(cfg.Port)
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err = d.verifyExisting(ctx, true); err == nil {
			err = verifyReady(ctx, address)
		}
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("完整模式未在预算内就绪；已保留本实例资源和日志，请检查 Desktop 容量、媒体共享权限与镜像日志后重试")
		case <-ticker.C:
		}
	}
	c.setState("ready", address, nil)
	c.logger.Info("完整模式媒体服务已就绪", "地址", address, "提示", "引擎初始化与搜索来源配置仍需在网页中主动完成")
	if cfg.AutoOpen && !c.options.NoBrowser && !c.options.Headless {
		go c.openPage()
	}
	serviceCtx, cancel := context.WithCancel(ctx)
	c.serviceCancel = cancel
	c.serviceDone = make(chan error, 1)
	done := c.serviceDone
	go func() {
		monitor := time.NewTicker(10 * time.Second)
		defer monitor.Stop()
		failures := 0
		for {
			select {
			case <-serviceCtx.Done():
				done <- nil
				return
			case <-monitor.C:
				if checkErr := d.verifyExisting(serviceCtx, true); checkErr != nil {
					failures++
				} else if checkErr = verifyReady(serviceCtx, address); checkErr != nil {
					failures++
				} else {
					failures = 0
				}
				if failures >= 3 {
					done <- errors.New("完整模式服务连续三次无法确认就绪，请检查 Docker Desktop 后从托盘重启")
					return
				}
			}
		}
	}()
	return nil
}

// stopAll 先通过 helper 核验并停止动态引擎，再停止精确的本实例 Compose 服务，不删除卷。
func (d *dockerRuntime) stopAll(ctx context.Context) error {
	if err := d.verifyExisting(ctx, false); err != nil {
		return err
	}
	containers, err := d.ownedContainers(ctx)
	if err != nil {
		return err
	}
	helperRunning := false
	for _, container := range containers {
		if container.Config.Labels["com.docker.compose.service"] == "nas-engine-helper" && container.State.Running {
			helperRunning = true
		}
	}
	if !helperRunning {
		return errors.New("助手容器未运行，无法核验动态引擎已停止；请先恢复本实例助手后重试")
	}
	if _, err := d.execute(ctx, "安全停止本实例托管引擎及资源组件", 255*time.Second, d.composeArgs("exec", "-T", "nas-engine-helper", "/nas-engine-helper", "stop-all")...); err != nil {
		return errors.New("助手未确认全部引擎安全停止；可能仍有初始化任务或归属冲突，已保留 app/helper，请在网页等待任务完成后重试")
	}
	services := []string{"stop", "--timeout", "30", "nas", "nas-engine-helper"}
	if d.manifest.CollectionImage != "" {
		services = append(services, "nas-collection-worker")
	}
	_, err = d.execute(ctx, "停止本实例应用、助手及受控获取服务", 120*time.Second, d.composeArgs(services...)...)
	return err
}

// stopDocker 由托盘明确触发，停止监视不代表停止容器，成功后保留所有数据卷。
func (c *controller) stopDocker(ctx context.Context) error {
	cfg, _ := c.snapshot()
	d, err := c.prepareDocker(ctx, cfg)
	if err != nil {
		return err
	}
	c.setState("stopping", "", nil)
	if err = d.stopAll(ctx); err != nil {
		return err
	}
	c.setState("stopped", "", nil)
	c.logger.Info("本实例完整模式已停止", "数据卷", "保留")
	return nil
}
