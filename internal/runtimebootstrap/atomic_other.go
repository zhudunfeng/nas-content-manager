//go:build !windows

package runtimebootstrap

import "os"

// replaceSelection 保留跨平台构建能力；公开初始化入口仍只支持 Windows x64。
func replaceSelection(source string, target string) error {
	return os.Rename(source, target)
}
