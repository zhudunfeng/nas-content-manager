//go:build webembed

package webassets

import (
	"embed"
	"io/fs"
)

// releaseFiles 保存发布脚本预先构建的完整前端，缺少 dist 时编译必须失败。
//
//go:embed all:dist
var releaseFiles embed.FS

// Files 返回以 dist 为根的只读资源，资源错误在编译阶段排除。
func Files() fs.FS {
	files, _ := fs.Sub(releaseFiles, "dist")
	return files
}
