//go:build !windows

package app

import "os"

// restrictSnapshotPath 为临时目录设置0700、候选文件设置0600权限。
func restrictSnapshotPath(path string, directory bool) error {
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	return os.Chmod(path, mode)
}

// isSnapshotReparsePoint 在 POSIX 上拒绝符号链接，普通文件类型由调用方另外确认。
func isSnapshotReparsePoint(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}
