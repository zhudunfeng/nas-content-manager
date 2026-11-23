package enginehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// volumeMount 使用Docker API1.41已有的原生命名卷字段，不依赖卷子路径。
type volumeMount struct {
	Type          string // 固定volume类型。
	Source        string // 受信任部署前缀派生的逻辑卷名。
	Target        string // 固定Linux容器目标目录。
	ReadOnly      bool   // 引擎仅使用可写角色卷。
	VolumeOptions struct {
		NoCopy bool // 禁止把镜像初始目录复制进受控卷。
	} // 仅使用固定的卷选项。
}

// volumeBinding 将部署角色与持久安装身份绑定，不依赖Compose预先知道随机身份。
type volumeBinding struct {
	Version      int    `json:"version"`      // 当前绑定协议版本，固定1。
	Deployment   string `json:"deployment"`   // 受信任部署前缀。
	Role         string `json:"role"`         // 固定存储职责。
	Installation string `json:"installation"` // 私有状态库的随机安装身份。
}

// volumeRoles 返回助手唯一允许的固定存储角色，顺序也用于稳定验证。
func volumeRoles() []string {
	return []string{"state", "qbittorrent", "jackett", "downloads", "connections", "ipc"}
}

// configuredVolumeRoles 保留旧六角色兼容，只在显式启用资源组件时要求独立资源卷。
func configuredVolumeRoles(c Config) []string {
	roles := volumeRoles()
	if c.CollectionEnabled {
		roles = append(roles, "collection-media")
	}
	return roles
}

// volumeName 从部署配置派生固定角色卷名，不接受HTTP输入。
func volumeName(c Config, role string) string { return c.VolumePrefix + "-" + role }

// volumePath 返回一个固定角色在助手中的唯一路径。
func volumePath(c Config, role string) string {
	if role == "ipc" {
		return "/ipc"
	}
	return filepath.Join(c.Root, role)
}

// volumePermissions 统一定义角色目录和绑定文件的Linux访问身份。
func volumePermissions(c Config, role string) (os.FileMode, int, int) {
	switch role {
	case "state":
		return 0700, 0, 0
	case "ipc":
		return 0750, 0, c.GID
	case "downloads", "collection-media":
		return 0755, c.UID, c.GID
	default:
		return 0750, c.UID, c.GID
	}
}

// volumeIdentity 验证Compose创建的本地角色卷，绝不创建或更新daemon资源。
func (d *dockerClient) volumeIdentity(ctx context.Context, c Config, role string) error {
	var info struct {
		Name    string            // daemon返回的逻辑卷名。
		Driver  string            // 只接受默认local驱动。
		Scope   string            // 只接受本地作用域。
		Labels  map[string]string // Compose声明的可信部署及角色标签。
		Options map[string]string // 禁止通过local驱动选项映射外部设备或网络存储。
	}
	if err := d.call(ctx, http.MethodGet, d.api()+"/volumes/"+url.PathEscape(volumeName(c, role)), nil, &info); err != nil {
		return errors.New("命名卷不可验证，请先应用当前实例的Compose配置")
	}
	if info.Name != volumeName(c, role) || info.Driver != "local" || info.Scope != "local" || len(info.Options) != 0 || info.Labels["nas.engines.deployment"] != c.VolumePrefix || info.Labels["nas.engines.role"] != role {
		return errors.New("命名卷驱动、标签或部署归属冲突，不会接管已有数据")
	}
	return nil
}

// verifyVolumeMounts 核对助手所有角色挂载及元数据，拒绝覆盖挂载和重复目标。
func (s *Service) verifyVolumeMounts(ctx context.Context, self *containerInfo) error {
	want := map[string]string{}
	for _, role := range configuredVolumeRoles(s.config) {
		want[volumePath(s.config, role)] = role
	}
	seen := map[string]bool{}
	for _, mount := range self.Mounts {
		role, required := want[mount.Destination]
		if !required {
			// /managed及其子目录不能另有挂载；Docker socket不属于角色数据卷。
			if mount.Destination == s.config.Root || strings.HasPrefix(mount.Destination, s.config.Root+"/") || strings.HasPrefix(mount.Destination, "/ipc/") {
				return errors.New("助手角色目录存在额外覆盖挂载")
			}
			continue
		}
		if seen[role] || mount.Type != "volume" || mount.Name != volumeName(s.config, role) || mount.Driver != "local" || !mount.RW {
			return errors.New("助手命名卷挂载身份或读写权限不符")
		}
		seen[role] = true
		if err := s.docker.volumeIdentity(ctx, s.config, role); err != nil {
			return err
		}
	}
	if len(seen) != len(want) {
		return errors.New("助手缺少固定角色命名卷，请应用当前实例Compose配置")
	}
	return nil
}

// validInstallation 拒绝不属于助手随机身份格式的绑定内容。
func validInstallation(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// readVolumeBinding 有界读取普通绑定文件，并核对部署、角色及可选安装身份。
func readVolumeBinding(c Config, role string, installation string) (volumeBinding, error) {
	var binding volumeBinding
	path := filepath.Join(volumePath(c, role), ".nas-engine-volume")
	data, err := readPrivate(path)
	if err != nil {
		return binding, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return binding, errConflict
	}
	uid, gid, err := fileIdentity(info)
	_, wantUID, wantGID := volumePermissions(c, role)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err != nil || uid != wantUID || gid != wantGID || decoder.Decode(&binding) != nil || decoder.Decode(&struct{}{}) != io.EOF || binding.Version != 1 || binding.Deployment != c.VolumePrefix || binding.Role != role || !validInstallation(binding.Installation) || installation != "" && binding.Installation != installation {
		return binding, errConflict
	}
	return binding, nil
}

// emptyVolume 只允许空卷；state可保留已核对的固定锁文件用于崩溃恢复。
func emptyVolume(c Config, role string) error {
	path := volumePath(c, role)
	if err := checkDirectoryTree(path); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return errConflict
	}
	for _, entry := range entries {
		if role != "state" || entry.Name() != "helper.lock" {
			return errConflict
		}
		info, statErr := os.Lstat(filepath.Join(path, entry.Name()))
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return errConflict
		}
		uid, gid, identityErr := fileIdentity(info)
		if identityErr != nil || uid != 0 || gid != 0 {
			return errConflict
		}
	}
	return nil
}

// prepareStateVolume 仅为已验证且为空或已有绑定的状态卷准备私密权限。
func prepareStateVolume(c Config) error {
	if _, err := os.Lstat(filepath.Join(volumePath(c, "state"), ".nas-engine-volume")); err == nil {
		return ensureDirectory(volumePath(c, "state"), 0700, 0, 0)
	} else if !errors.Is(err, os.ErrNotExist) {
		return errConflict
	}
	if err := emptyVolume(c, "state"); err != nil {
		return err
	}
	if os.Chown(volumePath(c, "state"), 0, 0) != nil || os.Chmod(volumePath(c, "state"), 0700) != nil {
		return errors.New("无法设置空状态卷的私密权限")
	}
	return nil
}

// claimVolume 在持有安装锁时绑定角色；只读检查绝不修改目录、标记或权限。
func claimVolume(c Config, role string, installation string, claim bool) (string, error) {
	if err := checkDirectoryTree(volumePath(c, role)); err != nil {
		return "", err
	}
	binding, err := readVolumeBinding(c, role, installation)
	if err == nil {
		if err = verifyVolumeDirectory(c, role); err != nil {
			return "", err
		}
		return binding.Installation, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", errConflict
	}
	if err = emptyVolume(c, role); err != nil {
		return "", err
	}
	if !claim {
		return "", nil
	}
	if installation == "" {
		if role != "state" {
			return "", errConflict
		}
		installation, err = randomID()
		if err != nil {
			return "", errors.New("安全随机源不可用")
		}
	}
	mode, uid, gid := volumePermissions(c, role)
	// 空卷身份已验证，允许Docker默认root:root目录被明确绑定到目标UID。
	if os.Chown(volumePath(c, role), uid, gid) != nil || os.Chmod(volumePath(c, role), mode) != nil {
		return "", errors.New("命名卷角色权限设置失败")
	}
	binding = volumeBinding{Version: 1, Deployment: c.VolumePrefix, Role: role, Installation: installation}
	data, err := json.Marshal(binding)
	if err != nil {
		return "", errors.New("命名卷绑定序列化失败")
	}
	fileMode := os.FileMode(0640)
	if role == "state" {
		fileMode = 0600
	}
	if err = atomicFile(filepath.Join(volumePath(c, role), ".nas-engine-volume"), data, fileMode, uid, gid); err != nil {
		return "", err
	}
	return installation, nil
}

// verifyVolumeDirectory 只读核对已绑定根目录身份及权限，不修复未知变更。
func verifyVolumeDirectory(c Config, role string) error {
	info, err := os.Lstat(volumePath(c, role))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errConflict
	}
	uid, gid, err := fileIdentity(info)
	mode, wantUID, wantGID := volumePermissions(c, role)
	if err != nil || uid != wantUID || gid != wantGID || info.Mode().Perm()&0002 != 0 || role == "state" && info.Mode().Perm() != mode {
		return errConflict
	}
	return nil
}

// verifyVolumeBindings 只读核验安装绑定，未知非空卷不会成为可初始化目标。
func (s *Service) verifyVolumeBindings() error {
	for _, role := range configuredVolumeRoles(s.config) {
		if _, err := claimVolume(s.config, role, s.store.installation, false); err != nil {
			return errors.New("命名卷安装绑定冲突或包含未知数据，不会接管")
		}
	}
	return nil
}
