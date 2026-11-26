//go:build !windows

package privateruntime

import "os"

// IsReparsePoint 在非Windows平台只识别符号链接，不模拟NTFS重解析语义。
func IsReparsePoint(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}
