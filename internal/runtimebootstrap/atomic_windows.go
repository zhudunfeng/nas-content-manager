//go:build windows

package runtimebootstrap

import "golang.org/x/sys/windows"

// replaceSelection 在本机卷原子替换选择文件并要求写穿，不先删除旧文件。
func replaceSelection(source string, target string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
