//go:build linux

package app

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// acquireInstanceFile 使用 Linux flock 排斥同一数据目录的并发恢复，锁文件永久保留。
func acquireInstanceFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("应用独占锁不可打开，请检查数据目录权限")
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("应用独占锁必须为普通文件")
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("当前数据目录已有媒体库实例运行，禁止重复打开数据库")
	}
	// 不删除锁文件；删除会使仍持有旧 inode 的实例与新实例同时取得锁。
	return file, nil
}
