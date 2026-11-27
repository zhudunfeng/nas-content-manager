// Package uavbootstrap 绑定固定视频合同与共享安全初始化引擎。
package uavbootstrap

import (
	"context"
	_ "embed"
	"path/filepath"

	"nas-content-manager/internal/runtimebootstrap"
	"nas-content-manager/internal/uavruntime"
)

// embeddedRecipe 固定下载来源、精确摘要、长度及还原映射。
//
//go:embed assets/recipe.json
var embeddedRecipe []byte

// embeddedRuntime 是构建侧严格验证后生成的完整运行时清单。
//
//go:embed assets/runtime.json
var embeddedRuntime []byte

// embeddedPayload 仅包含已装配小文件和许可，不冒充原始 wheel。
//
//go:embed assets/payload.zip
var embeddedPayload []byte

// Paths 保存已校验视频依赖的固定绝对路径，不能由浏览器选择。
type Paths struct {
	Manifest string // 固定名称的视频私有运行时清单。
	YTDLP    string // 固定版本的独立下载器。
	FFmpeg   string // 离线媒体转换程序。
	FFprobe  string // 离线媒体探针。
}

// Progress 共享真实字节进度合同，各插件的状态仍独立发布。
type Progress = runtimebootstrap.Progress

// ErrCleanup 保持共享安全引擎的清理错误身份，调用方必须等待完整释放。
var ErrCleanup = runtimebootstrap.ErrCleanup

// engine 固化原视频材料和命名空间，不改变已发布 current 的配方身份。
var engine = runtimebootstrap.New(runtimebootstrap.Config{
	Label:           "视频",                                                                  // 固定中文状态名称。
	Namespace:       "uav-bootstrap",                                                       // 保留原 profile 运行时及归属字节兼容性。
	ManifestName:    uavruntime.ManifestName,                                               // 固定视频合同清单。
	Recipe:          embeddedRecipe,                                                        // 固定来源与完整还原映射。
	Runtime:         embeddedRuntime,                                                       // 完整视频库存原始字节。
	Payload:         embeddedPayload,                                                       // 已验证的小型固定材料。
	RequiredFiles:   []string{"tools/yt-dlp.exe", "tools/ffmpeg.exe", "tools/ffprobe.exe"}, // 视频额外必需的固定工具。
	ValidateRuntime: validateRuntime,                                                       // 原有视频严格加载器。
})

// validateRuntime 保留视频版本、来源、许可、PE 与完整目录校验。
func validateRuntime(manifest string) error {
	_, err := uavruntime.Load(manifest)
	return err
}

// videoPaths 仅从已核验的代次根计算固定工具路径。
func videoPaths(paths runtimebootstrap.Paths) Paths {
	if paths.Manifest == "" {
		return Paths{}
	}
	return Paths{Manifest: paths.Manifest, YTDLP: filepath.Join(paths.Root, "tools", "yt-dlp.exe"), FFmpeg: filepath.Join(paths.Root, "tools", "ffmpeg.exe"), FFprobe: filepath.Join(paths.Root, "tools", "ffprobe.exe")}
}

// Resolve 只读校验当前已发布视频依赖；调用方必须持有所属 profile 单实例租约。
func Resolve(root string) (Paths, error) {
	paths, err := engine.Resolve(root)
	return videoPaths(paths), err
}

// Initialize 还原固定视频材料，在最终路径通过真实探针后才原子发布。
func Initialize(ctx context.Context, root string, report func(Progress), probe func(context.Context, Paths) error) (Paths, error) {
	var adapter func(context.Context, runtimebootstrap.Paths) error
	if probe != nil {
		adapter = func(ctx context.Context, paths runtimebootstrap.Paths) error { return probe(ctx, videoPaths(paths)) }
	}
	paths, err := engine.Initialize(ctx, root, report, adapter)
	return videoPaths(paths), err
}
