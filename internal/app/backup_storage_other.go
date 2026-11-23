//go:build !windows && !linux

package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// backupPathSyntax 仅完成静态本地绝对路径验证，不因此宣称平台受支持。
func backupPathSyntax(path string) bool {
	return filepath.IsAbs(path) && !strings.ContainsRune(path, 0) && !strings.HasPrefix(path, "//")
}

// backupPathKey 保留本平台路径大小写，实际准备始终拒绝未支持平台。
func backupPathKey(path string) string { return filepath.Clean(path) }

// backupResolveAnchorPath 未支持平台不能建立备份路径的实际映射。
func backupResolveAnchorPath(path string) (string, error) {
	return "", errors.New("备份平台未支持")
}

// backupStatLock 未支持平台不假定锁目录项可安全核对。
func backupStatLock(root *os.Root, path string) (os.FileInfo, error) {
	return nil, errors.New("备份平台未支持")
}

// backupOpenAnchor 未支持平台不打开持久备份资源。
func backupOpenAnchor(path string) (*os.File, error) { return nil, errors.New("备份平台未支持") }

// backupPhysicalPath 未支持平台不能证明物理隔离。
func backupPhysicalPath(file *os.File, path string) (string, string, error) {
	return "", "", errors.New("备份平台未支持")
}

// backupCheckFilesystem 未支持平台不得降级为普通复制。
func backupCheckFilesystem(file *os.File, path string) error {
	return errors.New("备份平台未支持")
}

// backupCheckSingleLink 未支持平台不假定链接身份安全。
func backupCheckSingleLink(file *os.File) error { return errors.New("备份平台未支持") }

// backupCheckPermissions 未支持平台不假定权限安全。
func backupCheckPermissions(file *os.File, directory bool) error {
	return errors.New("备份平台未支持")
}

// backupCreateFile 未支持平台禁止创建备份成员。
func backupCreateFile(root *os.Root, path string, name string) (*os.File, error) {
	return nil, errors.New("备份平台未支持")
}

// backupMoveNoReplace 未支持平台禁止降级移动。
func backupMoveNoReplace(directory *os.File, path string, old string, next string) error {
	return errors.New("备份平台未支持")
}

// backupSyncDirectory 未支持平台没有已核定同步协议。
func backupSyncDirectory(directory *os.File) error { return errors.New("备份平台未支持") }
