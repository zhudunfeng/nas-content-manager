package enginehelper

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"crypto/sha512"
	"golang.org/x/crypto/pbkdf2"
	"nas-content-manager/internal/enginecontract"
)

// ensureDirectory 拒绝符号链接并仅为新建专用目录设置身份，不改原媒体权限。
func ensureDirectory(path string, mode os.FileMode, uid int, gid int) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir(path, mode); err != nil {
			return errors.New("无法创建专用目录，请检查挂载和权限")
		}
		if err = os.Chown(path, uid, gid); err != nil {
			return errors.New("无法设置专用目录身份")
		}
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("专用目录无效或包含符号链接")
	}
	actualUID, actualGID, identityErr := fileIdentity(info)
	if identityErr != nil {
		return identityErr
	}
	if actualUID != uid || actualGID != gid {
		entries, readErr := os.ReadDir(path)
		if readErr != nil || len(entries) != 0 {
			return errors.New("非空专用目录属主不符，请人工修复；助手不会接管")
		}
		// Compose可能预先创建空bind目录；只有确认为空才允许认领其身份。
		if os.Chown(path, uid, gid) != nil || os.Chmod(path, mode) != nil {
			return errors.New("无法认领空专用目录权限")
		}
	}
	if mode == 0700 && info.Mode().Perm()&0077 != 0 {
		return errors.New("助手私有状态目录权限必须为0700")
	}
	if info.Mode().Perm()&0002 != 0 {
		return errors.New("专用目录不能允许所有用户写入，请人工修复权限")
	}
	return nil
}

// checkDirectoryTree 拒绝配置路径中任何符号链接或普通文件。
func checkDirectoryTree(path string) error {
	clean := filepath.Clean(path)
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("专用挂载路径无效或存在符号链接")
		}
	}
	return nil
}

// atomicFile 使用fsync和同目录rename发布私有文件，禁止跟随旧符号链接。
func atomicFile(path string, data []byte, mode os.FileMode, uid int, gid int) error {
	if len(data) > 64*1024 {
		return errors.New("私有连接文件超过预算")
	}
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("私有文件目标不安全")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".nas-tmp-")
	if err != nil {
		return errors.New("无法创建私有临时文件")
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err = f.Chmod(mode); err == nil {
		err = f.Chown(uid, gid)
	}
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("私有文件落盘失败")
	}
	if err = os.Rename(temp, path); err != nil {
		return errors.New("私有文件原子发布失败")
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return errors.New("私有目录同步失败")
	}
	defer dir.Close()
	if err = dir.Sync(); err != nil {
		return errors.New("私有目录同步失败")
	}
	return nil
}

// readPrivate 有界读取普通私密文件，拒绝符号链接及宽泛读取权限。
func readPrivate(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("私有文件尚未生成: %w", os.ErrNotExist)
		}
		return nil, errors.New("私有文件不可访问")
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 64*1024 || info.Mode().Perm()&0007 != 0 {
		return nil, errors.New("私密文件类型、权限或大小不安全")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("私有文件无法安全打开")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return nil, errors.New("私密文件读取超过预算")
	}
	return data, nil
}

// ownedConfig 只接受当前安装的配置目录；空目录先原子落盘归属标记。
func (s *Service) ownedConfig(component string, claim bool) error {
	if s.config.StorageMode == "volume" {
		_, err := claimVolume(s.config, component, s.store.installation, claim)
		return err
	}
	return s.ownedDirectory(component, claim)
}

// ownedDirectory 只为固定受管子目录原子绑定安装身份，不接管未知非空内容。
func (s *Service) ownedDirectory(component string, claim bool) error {
	dir := filepath.Join(s.config.Root, component)
	if !claim {
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errConflict
		}
	} else {
		if err := ensureDirectory(dir, 0750, s.config.UID, s.config.GID); err != nil {
			return err
		}
	}
	marker := filepath.Join(dir, ".nas-engine-owner")
	value, err := readPrivate(marker)
	if err == nil {
		if string(value) != s.store.installation+"\n" {
			return errConflict
		}
		return checkDirectoryTree(dir)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return errConflict
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		return errConflict
	}
	if !claim {
		return nil
	}
	return atomicFile(marker, []byte(s.store.installation+"\n"), 0640, s.config.UID, s.config.GID)
}

// prepareQB 仅对已认领全新目录离线生成永久密码，并为中断重试复用原值。
func (s *Service) prepareQB() (string, error) {
	passwordFile := filepath.Join(s.config.Root, "state", "qb-password")
	data, err := readPrivate(passwordFile)
	var password string
	if errors.Is(err, os.ErrNotExist) {
		if _, existingErr := os.Lstat(filepath.Join(s.config.Root, enginecontract.QBittorrent, "qBittorrent", "config", "qBittorrent.conf")); existingErr == nil {
			return "", errors.New("qB配置已存在但永久凭据缺失，请恢复完整助手备份")
		}
		random := make([]byte, 32)
		if _, err = rand.Read(random); err != nil {
			return "", errors.New("安全随机源不可用")
		}
		password = base64.RawURLEncoding.EncodeToString(random)
		if err = atomicFile(passwordFile, []byte(password), 0600, 0, 0); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else {
		password = string(data)
		if len(password) != 43 {
			return "", errors.New("已保存凭据格式无效，请恢复完整助手备份")
		}
	}
	base := filepath.Join(s.config.Root, enginecontract.QBittorrent)
	for _, dir := range []string{filepath.Join(base, "qBittorrent"), filepath.Join(base, "qBittorrent", "config")} {
		if err = ensureDirectory(dir, 0750, s.config.UID, s.config.GID); err != nil {
			return "", err
		}
	}
	configPath := filepath.Join(base, "qBittorrent", "config", "qBittorrent.conf")
	if info, statErr := os.Lstat(configPath); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", errConflict
		}
		return password, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", errors.New("qB配置不可访问")
	}
	// 永久凭据先落盘，再写固定版本Qt字节数组哈希；重试绝不重置已存在配置。
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return "", errors.New("安全随机源不可用")
	}
	hash := pbkdf2.Key([]byte(password), salt, 100000, 64, sha512.New)
	encoded := base64.StdEncoding.EncodeToString(salt) + ":" + base64.StdEncoding.EncodeToString(hash)
	text := "[Preferences]\nWebUI\\Enabled=true\nWebUI\\Port=8080\nWebUI\\Username=admin\nWebUI\\Password_PBKDF2=\"@ByteArray(" + encoded + ")\"\nWebUI\\LocalHostAuth=true\nWebUI\\AuthSubnetWhitelistEnabled=false\nWebUI\\CSRFProtection=true\nWebUI\\HostHeaderValidation=true\n"
	if err = atomicFile(configPath, []byte(text), 0640, s.config.UID, s.config.GID); err != nil {
		return "", err
	}
	return password, nil
}

// readJackettKey 仅读取固定受管ServerConfig字段，不向外返回整个配置。
func (s *Service) readJackettKey() (string, error) {
	path := filepath.Join(s.config.Root, enginecontract.Jackett, "Jackett", "ServerConfig.json")
	if err := checkDirectoryTree(filepath.Dir(path)); err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 64*1024 {
		return "", errors.New("Jackett配置无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var cfg struct{ APIKey string }
	dec := json.NewDecoder(io.LimitReader(f, 64*1024+1))
	if dec.Decode(&cfg) != nil || len(cfg.APIKey) != 32 {
		return "", errors.New("Jackett尚未生成有效密钥")
	}
	for _, r := range cfg.APIKey {
		if !strings.ContainsRune("abcdefABCDEF0123456789", r) {
			return "", errors.New("Jackett密钥格式无效")
		}
	}
	return cfg.APIKey, nil
}

// loadManifest 读取最后完整发布版本，没有清单时返回当前安装的空清单。
func (s *Service) loadManifest() (enginecontract.Manifest, error) {
	m := enginecontract.Manifest{Version: 1, InstallationID: s.store.installation}
	data, err := readPrivate(filepath.Join(s.config.Root, "connections", "current.json"))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	// 已存在清单必须完整验证，拒绝未知字段、尾随JSON及指向未来的组件版本。
	var published enginecontract.Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&published) != nil || decoder.Decode(&struct{}{}) != io.EOF || published.Version != 1 || published.InstallationID != s.store.installation || published.Generation < 1 || published.QBGeneration < 0 || published.JackettGeneration < 0 || published.QBGeneration > published.Generation || published.JackettGeneration > published.Generation {
		return m, errors.New("连接清单损坏或安装身份不符")
	}
	return published, nil
}

// publish 先发布固定版本连接，再原子更新current清单；从不发布半成品。
func (s *Service) publish(t template, containerID string, password string, key string) error {
	m, err := s.loadManifest()
	if err != nil {
		return err
	}
	if m.Generation == 1<<63-1 {
		return errors.New("连接版本达到安全上限，请人工核对助手状态")
	}
	m.Generation++
	generation := m.Generation
	connection := enginecontract.Connection{Version: 1, InstallationID: s.store.installation, Component: t.Component, Generation: generation, ContainerID: containerID, URL: "http://" + containerName(s.store.installation, t.Component) + ":" + strconv.Itoa(t.Port)}
	if t.Component == enginecontract.QBittorrent {
		connection.Username = "admin"
		connection.Password = password
		m.QBGeneration = generation
	} else {
		connection.APIKey = key
		m.JackettGeneration = generation
	}
	data, err := json.Marshal(connection)
	if err != nil {
		return errors.New("连接序列化失败")
	}
	path := filepath.Join(s.config.Root, "connections", fmt.Sprintf("%s-%d.json", t.Component, generation))
	if err = atomicFile(path, data, 0640, s.config.UID, s.config.GID); err != nil {
		return err
	}
	data, err = json.Marshal(m)
	if err != nil {
		return errors.New("连接清单序列化失败")
	}
	return atomicFile(filepath.Join(s.config.Root, "connections", "current.json"), data, 0640, s.config.UID, s.config.GID)
}
