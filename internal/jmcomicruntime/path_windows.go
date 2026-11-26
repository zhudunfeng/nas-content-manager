//go:build windows

package jmcomicruntime

import (
	"os"
	"syscall"
)

// isReparsePoint 识别所有Windows重解析点，而非仅识别符号链接。
func isReparsePoint(info os.FileInfo) bool {
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && attributes.FileAttributes&0x400 != 0
}
