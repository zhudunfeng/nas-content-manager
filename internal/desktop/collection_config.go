package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// validCollectionMode 仅允许三种固定获取方式，不接受解释器或任意命令参数。
func validCollectionMode(mode string) bool {
	return mode == "disabled" || mode == "managed" || mode == "external"
}

// validControlAction 限定当前用户私有 IPC 可以触发的固定操作。
func validControlAction(action string) bool {
	return action == "open" || action == "status" || action == "quit" || action == "collection-enable" || action == "collection-disable" || action == "collection-check"
}

// hasExternalCollectionEnvironment 保留既有外接配置，包括需要明确报错的不完整配置。
func hasExternalCollectionEnvironment() bool {
	return os.Getenv("NAS_COLLECTION_WORKER_URL") != "" || os.Getenv("NAS_COLLECTION_WORKER_TOKEN_FILE") != "" || os.Getenv("NAS_COLLECTION_READ_ROOT") != ""
}

// defaultCollectionMode 仅为新配置选择默认方式，完整随包运行时可直接托管。
func defaultCollectionMode(mode string) string {
	if hasExternalCollectionEnvironment() {
		return "external"
	}
	if mode == "native" {
		path, err := os.Executable()
		if err == nil {
			dir := filepath.Dir(path)
			worker, workerErr := os.Stat(filepath.Join(dir, "nas-collection-worker.exe"))
			manifest, manifestErr := os.Stat(filepath.Join(dir, "jmcomic-runtime.json"))
			if workerErr == nil && worker.Mode().IsRegular() && manifestErr == nil && manifest.Mode().IsRegular() {
				return "managed"
			}
		}
	}
	return "disabled"
}

// backupV1Config 在首次迁移保存前保留旧版本原文，拒绝覆盖不同的既有备份。
func backupV1Config(profile string) error {
	file, err := os.Open(filepath.Join(profile, "desktop.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("迁移前无法读取原本机配置")
	}
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	closeErr := file.Close()
	if err != nil || closeErr != nil || len(data) > 65536 {
		return errors.New("迁移前读取本机配置失败")
	}
	var previous map[string]json.RawMessage
	if decodeStrictJSON(data, &previous) != nil {
		return errors.New("迁移前本机配置格式无效，已保留原文件")
	}
	var version int
	if json.Unmarshal(previous["schemaVersion"], &version) != nil || version != 1 {
		return nil
	}
	backup := filepath.Join(profile, "desktop.v1.json")
	file, err = os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		info, statErr := os.Lstat(backup)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
			return errors.New("旧版本配置备份不是有界普通文件，拒绝覆盖；请核对 desktop.v1.json")
		}
		oldFile, openErr := os.Open(backup)
		if openErr != nil {
			return errors.New("旧版本配置备份不可读取，拒绝覆盖；请核对 desktop.v1.json")
		}
		old, readErr := io.ReadAll(io.LimitReader(oldFile, 65537))
		oldCloseErr := oldFile.Close()
		if readErr != nil || oldCloseErr != nil || !bytes.Equal(data, old) {
			return errors.New("旧版本配置备份已存在且内容不同，拒绝覆盖；请核对 desktop.v1.json")
		}
		return nil
	}
	if err != nil {
		return errors.New("无法保存旧版本配置备份，已停止迁移")
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr = file.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(backup)
		return errors.New("旧版本配置备份未完整写入，已停止迁移")
	}
	return nil
}
