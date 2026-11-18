//go:build !linux && !windows

package app

import (
	"fmt"
	"os"
)

// acquireInstanceFile 拒绝未经验证的平台，避免缺少数据目录锁时误启动执行器。
func acquireInstanceFile(_ string) (*os.File, error) {
	return nil, fmt.Errorf("当前平台尚未提供应用数据目录独占锁，请使用 Linux 或 Windows")
}
