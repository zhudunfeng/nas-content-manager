//go:build !webembed

// Package webassets 提供发布资源；开发构建继续使用外部前端目录。
package webassets

import "io/fs"

// Files 在未启用发布嵌入时返回 nil，由应用读取 NAS_WEB_DIR。
func Files() fs.FS {
	return nil
}
