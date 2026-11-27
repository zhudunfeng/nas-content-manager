// Package jmcomicbootstrap 绑定固定漫画合同与共享安全初始化引擎。
package jmcomicbootstrap

import (
	"context"
	_ "embed"

	"nas-content-manager/internal/jmcomicruntime"
	"nas-content-manager/internal/runtimebootstrap"
)

// embeddedRecipe 固定漫画官方来源、精确摘要、长度和文件还原映射。
//
//go:embed assets/recipe.json
var embeddedRecipe []byte

// embeddedRuntime 是构建侧严格验证后的完整漫画清单。
//
//go:embed assets/runtime.json
var embeddedRuntime []byte

// embeddedPayload 仅包含已装配的小文件、许可和隔离元数据。
//
//go:embed assets/payload.zip
var embeddedPayload []byte

// Paths 保存独立漫画代次的受信清单，不引用视频工具或解释器。
type Paths struct {
	Manifest string // 通过完整校验的固定漫画清单绝对路径。
}

// Progress 仅包含当前材料的真实下载字节与固定中文阶段。
type Progress = runtimebootstrap.Progress

// ErrCleanup 保持统一的资源清理错误身份，不能被普通下载失败掩盖。
var ErrCleanup = runtimebootstrap.ErrCleanup

// engine 只拥有漫画专属短目录，不安装视频插件或媒体工具。
var engine = runtimebootstrap.New(runtimebootstrap.Config{
	Label:           "漫画",                        // 固定中文状态名称。
	Namespace:       "jm-bootstrap",              // 缩短最终原生扩展路径，并隔离视频归属。
	ManifestName:    jmcomicruntime.ManifestName, // 固定漫画合同清单。
	Recipe:          embeddedRecipe,              // 固定来源及精确还原映射。
	Runtime:         embeddedRuntime,             // 完整漫画库存原始字节。
	Payload:         embeddedPayload,             // 已验证小型安装材料和许可。
	RequiredFiles:   nil,                         // 漫画没有 yt-dlp 或 FFmpeg 工具依赖。
	ValidateRuntime: validateRuntime,             // 漫画原有严格加载器。
})

// validateRuntime 保留漫画版本、来源、许可、PE 与隔离边界检查。
func validateRuntime(manifest string) error {
	_, err := jmcomicruntime.Load(manifest)
	return err
}

// Resolve 只读解析本程序配方的已发布漫画依赖，不联网或重写文件。
func Resolve(root string) (Paths, error) {
	paths, err := engine.Resolve(root)
	return Paths{Manifest: paths.Manifest}, err
}

// Initialize 还原固定漫画材料，最终路径真实探针成功后才原子发布。
func Initialize(ctx context.Context, root string, report func(Progress), probe func(context.Context, Paths) error) (Paths, error) {
	var adapter func(context.Context, runtimebootstrap.Paths) error
	if probe != nil {
		adapter = func(ctx context.Context, paths runtimebootstrap.Paths) error {
			return probe(ctx, Paths{Manifest: paths.Manifest})
		}
	}
	paths, err := engine.Initialize(ctx, root, report, adapter)
	return Paths{Manifest: paths.Manifest}, err
}
