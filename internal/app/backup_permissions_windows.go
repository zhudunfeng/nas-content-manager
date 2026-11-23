//go:build windows

package app

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// restrictSnapshotPath 设置真实受保护DACL，仅当前用户和SYSTEM拥有访问权限。
func restrictSnapshotPath(path string, directory bool) error {
	descriptor, err := backupPrivateDescriptor(directory)
	if err != nil {
		return errors.New("备份私有权限不可创建")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return errors.New("备份私有权限不可读取")
	}
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return errors.New("备份私有权限不可设置")
	}
	return nil
}

// isSnapshotReparsePoint 拒绝所有Windows重解析点，不能只检查符号链接模式位。
func isSnapshotReparsePoint(info os.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return !ok || data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
