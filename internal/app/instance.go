package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// canonicalDataDirectory 解析现有父目录的符号链接和 Windows 长短路径，兼容尚未创建的数据根。
func canonicalDataDirectory(dataDir string) (string, error) {
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return "", fmt.Errorf("应用数据目录无效")
	}
	parent := root
	var missing []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(parent)
		if resolveErr == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) || filepath.Dir(parent) == parent {
			return "", fmt.Errorf("应用数据目录的父目录不可访问")
		}
		missing = append(missing, filepath.Base(parent))
		parent = filepath.Dir(parent)
	}
}

// acquireInstance 创建并规范化数据根，在任何数据库恢复操作前持有内核级独占锁。
func acquireInstance(dataDir string) (*os.File, string, error) {
	root, err := canonicalDataDirectory(dataDir)
	if err != nil {
		return nil, "", fmt.Errorf("应用数据目录无效")
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, "", fmt.Errorf("应用数据目录不可创建，请检查写入权限")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, "", fmt.Errorf("应用数据目录不可规范化")
	}
	// 数据库与锁始终使用同一规范目录，避免符号链接或 junction 别名各自恢复任务。
	lease, err := acquireInstanceFile(filepath.Join(root, ".nas-instance.lock"))
	if err != nil {
		return nil, "", err
	}
	return lease, root, nil
}
