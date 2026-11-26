//go:build !windows

package jmcomicruntime

import "os"

// isReparsePoint 在非Windows平台只识别符号链接，不模拟NTFS重解析语义。
func isReparsePoint(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}
