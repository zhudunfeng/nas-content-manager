//go:build !linux

package enginehelper

import (
	"errors"
	"os"
)

// acquireLease 拒绝在非Linux平台启动不具备安装级进程锁的助手。
func acquireLease(_ string) (*os.File, error) {
	return nil, errors.New("助手跨进程独占锁仅支持Linux")
}
