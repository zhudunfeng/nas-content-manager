//go:build windows

package collectionworker

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// acquireWorkerFile 以零共享模式持有 Windows 文件句柄，覆盖数据库和工作线程生命周期。
func acquireWorkerFile(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("获取服务独占锁路径无效")
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fmt.Errorf("获取服务数据目录已有实例运行或独占锁不可写，请关闭旧实例并检查权限")
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &info); err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("获取服务独占锁必须为非重解析点的普通文件")
	}
	// 内核随句柄或进程退出释放独占权，不使用会遗留假阳性的 PID 文件。
	return os.NewFile(uintptr(handle), path), nil
}
