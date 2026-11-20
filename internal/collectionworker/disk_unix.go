//go:build linux || darwin || freebsd || openbsd || netbsd

package collectionworker

import "golang.org/x/sys/unix"

// freeDiskBytes 查询 Unix 文件系统的当前可用空间。
func freeDiskBytes(path string) (uint64, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(path, &stat)
	return uint64(stat.Bavail) * uint64(stat.Bsize), err
}
