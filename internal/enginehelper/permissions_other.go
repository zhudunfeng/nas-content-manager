//go:build !linux

package enginehelper

import (
	"errors"
	"os"
)

// fileIdentity 拒绝在非Linux上假装验证Unix属主。
func fileIdentity(_ os.FileInfo) (int, int, error) {
	return 0, 0, errors.New("文件身份验证仅支持Linux")
}

// verifyJackettWrite 拒绝在非Linux平台假装验证.NET系统调用。
func verifyJackettWrite(_ string) error {
	return errors.New("Jackett系统调用能力检查仅支持Linux")
}

// verifyTargetAccess 拒绝在非Linux平台假装完成目标UID权限验证。
func verifyTargetAccess(_ string, _ int, _ int) error {
	return errors.New("目标UID权限探针仅支持Linux助手")
}
