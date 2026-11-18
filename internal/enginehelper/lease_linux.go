//go:build linux

package enginehelper

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// acquireLease 在恢复状态前取得安装级进程锁，进程退出时由内核自动释放。
func acquireLease(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, errors.New("无法打开助手独占锁，请检查私有状态目录")
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		_ = file.Close()
		return nil, errors.New("助手独占锁类型或权限无效")
	}
	uid, gid, err := fileIdentity(info)
	if err != nil || uid != 0 || gid != 0 {
		_ = file.Close()
		return nil, errors.New("助手独占锁属主无效")
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errors.New("当前安装已有助手执行器运行，禁止重复恢复任务")
	}
	// 锁文件始终保留，避免删除后不同进程对不同inode取得同名锁。
	return file, nil
}
