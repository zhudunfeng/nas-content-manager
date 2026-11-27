# UAV 自动初始化实现接口

## 模块与数据流

启动配置显式开启 UAV → 桌面选择有效随包文件或 profile 已发布依赖 → 原有 worker 本地能力探测 → UAV 不可用时后台 Initialize → 锁定物料下载/安全 ZIP 还原 → 严格 runtime Load → 改名至最终 gen-<32hex> 并重新校验 → 最终位置 worker --check-uav → 原子发布 current.json → controller.run 串行激活。

## runtime 模块契约

新增 internal/uavbootstrap，由原生 Go 实现，不执行网络脚本、不使用系统 Python。

```go
// Paths 保存受信、规范绝对的运行时与工具路径。
type Paths struct {
    Manifest string // 固定 basename 的 uav-runtime.json。
    YTDLP string // 固定下载器 EXE。
    FFmpeg string // 固定离线媒体工具。
    FFprobe string // 固定媒体探针。
}
// Progress 仅包含脱敏中文阶段和当前下载物料的真实字节进度。
type Progress struct {
    Stage string
    DownloadedBytes int64
    TotalBytes int64
}
// Resolve 只校验 profile 下已原子发布且匹配编译期 recipe 的依赖，不联网。
func Resolve(root string) (Paths, error)
// Initialize 在 owned collection root 内隔离下载/还原，调用 probe 成功后才发布。
func Initialize(ctx context.Context, root string, report func(Progress), probe func(context.Context, Paths) error) (Paths, error)
```

- root 是已通过 prepareCollectionDirectory 核验的 collection-managed，不是程序根。
- 子目录 uav-bootstrap：只认固定 owner 标记；拒绝未知既有目录/链接、UNC。
- 不可变 gen-<32hex> + 有界严格 current.json 原子选择；所有路径拒绝重解析点。最终 EXE/DLL/PYD 最长248个UTF-16单元，保留原生加载器额外预算，过长在下载前拒绝。
- 编译期 recipe 与运行时 inventory 固定所有下载来源、hash、大小和文件映射。
- download 超时、逐段 ctx 取消、有限重定向白名单、精确大小/SHA；ZIP 检查路径别名/重复/链接/预算/CRC；只创建本次 staging 拥有文件。
- 校验使用现有 uavruntime.Load + privateruntime，保留严格来源、RECORD、架构、隔离和闭包。
- profile 已发布文件不修改；取消/失败清理本次暂存或未选最终代次并返回清理失败。最终 probe 使用桌面自有 Job 管理的 worker --check-uav，必须在改名之后、发布指针之前执行。

## 编译资源格式（由主 agent 生成）

internal/uavbootstrap/assets/runtime.json：已验证 uav-runtime.json 的精确字节。

internal/uavbootstrap/assets/payload.zip：已装配的小型 UAV 文件、全部 UAV 组件许可，以及下载原物料不能直接还原的 _pth/direct_url/RECORD。身份为 payload，不能冒充原始构建 wheel；runtime manifest 的 builtWheelSha256 保留原构建回执。

internal/uavbootstrap/assets/recipe.json：

```json
{
  "schemaVersion": 1,
  "runtimeManifestSha256": "<sha>",
  "payloadSha256": "<sha>",
  "artifacts": [
    {
      "name": "python",
      "url": "https://...",
      "sha256": "<sha>",
      "size": 123,
      "format": "zip",
      "files": [{"source":"python.exe","destination":"runtimes/uav/python.exe"}]
    },
    {
      "name": "yt-dlp",
      "url": "https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.26/yt-dlp.exe",
      "sha256": "<sha>",
      "size": 123,
      "format": "raw",
      "files": [{"source":"","destination":"tools/yt-dlp.exe"}]
    }
  ],
  "tools": [
    {"path":"tools/yt-dlp.exe","sha256":"<sha>","size":123},
    {"path":"tools/ffmpeg.exe","sha256":"<sha>","size":123},
    {"path":"tools/ffprobe.exe","sha256":"<sha>","size":123},
    {"path":"licenses/ffmpeg/LICENSE","sha256":"<sha>","size":123}
  ]
}
```

tools 指所有不在 runtime manifest 的最终额外文件（工具、FFmpeg/yt-dlp 许可材料）。其中部分文件可在 payload，下载来源记录以只读编译 recipe 写入 generation 内 bootstrap-receipt.json（如实现采用此文件，需验证其精确字节）。目录闭包须拒绝未声明 DLL/文件。

## 桌面接线

- 有效随包 runtime + 工具优先；可选择已验证 profile 依赖路径作为 UAV manifest、yt-dlp、FFmpeg 和 PATH。只改变 child 环境，不放开配置路径 allowlist。
- worker 真实报告 UAV available 则零下载；unavailable 且 native/managed/UAV启用/无显式Python 才调 Initialize。显式 false、disabled、external 不触发。
- 准备在后台，不阻塞媒体就绪；report 只短锁更新安全 IPC/托盘阶段。
- 完成 channel 由 controller.run 消费，携带本次任务身份，禁止旧完成事件重新启用。
- stop/停用/重检/退出先 cancel + wait 安装结束；重检/新启动可再次尝试，失败不无限重试。
- 成功由 run 既有 stop/start 串行激活主服务和 worker，避免后台直接操作生命周期裸字段。
- 保留已有禁用/外接profile；可让全新 profile 的 UAV=true 推导 managed，显式模式仍优先。

## 分工

- 主 agent：生成构建资源的 scripts/uav_bootstrap.py + assets、文档/spec、总体集成验收。
- runtime implementer：internal/uavbootstrap Go 实现及针对性临时测试，勿修改 assets 内容/schema（提前沟通缺口）。
- desktop implementer：internal/desktop 自动初始化/状态接线及针对性临时测试，勿修改 runtime/resources/docs。
- 最后由 trellis-check 独立审查和修复。
