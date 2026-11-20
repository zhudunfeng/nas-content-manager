package collectionworker

import (
	"os"
	"path/filepath"
)

// workerLease 持有数据根的内核独占句柄，进程崩溃后自动释放。
type workerLease struct {
	file *os.File /* 本实例专属的锁文件句柄。 */
}

// lockWorker 排斥同一数据根同时启动多个获取服务。
func lockWorker(root string) (*workerLease, error) {
	f, err := acquireWorkerFile(filepath.Join(root, ".worker.lock"))
	if err != nil {
		return nil, err
	}
	return &workerLease{file: f}, nil
}

// Close 释放独占句柄但保留锁文件，避免inode替换导致双重所有权。
func (l *workerLease) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
