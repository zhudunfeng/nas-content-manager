//go:build !linux && !windows

package collectionworker

import (
	"fmt"
	"os"
)

// acquireWorkerFile 拒绝未经验证的平台，避免缺少数据目录锁时误启动执行器。
func acquireWorkerFile(_ string) (*os.File, error) {
	return nil, fmt.Errorf("当前平台尚未提供获取服务数据目录独占锁，请使用 Linux 或 Windows")
}
