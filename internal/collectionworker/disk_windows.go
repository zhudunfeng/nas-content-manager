//go:build windows

package collectionworker

import "golang.org/x/sys/windows"

// freeDiskBytes 查询 Windows 卷的可用空间，不改变文件系统。
func freeDiskBytes(path string) (uint64, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available, total, free uint64
	err = windows.GetDiskFreeSpaceEx(pointer, &available, &total, &free)
	return available, err
}
