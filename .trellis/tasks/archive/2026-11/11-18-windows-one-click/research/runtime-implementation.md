# P0 共享运行时实现与验证

日期：2026-11-18。范围仅为共享应用配置、生命周期、静态资源与媒体工具定位；未实现或验证 Docker 部署、桌面交互及发行许可材料。

## 修改文件

- `internal/app/config.go`：新增 `LoadConfigWithLookup`；原 `LoadConfig` 保留为系统环境包装；新增 `WebDirExplicit`、`FFmpegPath`、`FFprobePath` 中文配置说明。
- `internal/app/download_config.go`、`torrent_search_config.go`、`engine_config.go`：新增内部 lookup 变体，保留既有函数；所有参数来自同一调用方来源，最终桌面目录先参与既有下载隔离校验。
- `internal/app/instance.go`、`instance_linux.go`、`instance_windows.go`、`instance_other.go`：数据根规范化和内核级独占锁；Linux 非阻塞 flock、Windows 零共享句柄；锁文件不删除。
- `internal/app/server.go`：`RunWithOptions`、保留真实 listener、数据库恢复前独占、所有退出路径归还 listener、有序 HTTP/worker/DB 关闭、一次性真实就绪通知、统一前端 FS。
- `internal/webassets/embed_dev.go`、`embed_release.go`：开发构建返回 nil；`webembed` 使用 `go:embed all:dist` 并返回以 dist 为根的 FS；缺少发布资源时嵌入编译失败。
- `internal/app/media_tools.go`、`media_process_windows.go`、`media_process_other.go`：统一绝对工具路径和 Windows 子进程隐藏控制台。
- `internal/app/cache.go`、`music.go`：所有 ffmpeg/ffprobe 调用使用同一 resolver，保留原探测时间、输出大小、协议白名单、线程与封面发布预算。

## 交付接口与行为

```go
func LoadConfigWithLookup(lookup func(string) string) (Config, error)

type RunOptions struct {
    Listener net.Listener         // 可选已绑定 listener，交给运行时后由运行时关闭。
    Ready    func(address string) // 数据库、前端入口与本实例 HTTP 就绪后回调一次。
}

func RunWithOptions(ctx context.Context, config Config, logger *slog.Logger, options RunOptions) error
func Run(ctx context.Context, config Config, logger *slog.Logger) error

// internal/webassets
func Files() fs.FS
```

- 独占锁先于 `openDatabase`，因此锁冲突不会触发 schema 迁移、缓存临时文件清理或中断任务恢复；锁持有到工作协程、HTTP 和数据库退出。
- 内部 listener 在打开数据库前实际 bind 并保留；显式传入的 listener 在配置、锁、网页、数据库、HTTP 的失败路径均关闭。
- `Ready` 非空时要求 index.html 为可读非空普通文件，并以无环境代理 HTTP 探针验证健康接口。每次运行随机探针标识由本实例回显，避免同地址被第三方复用时误判。桌面仍可执行自己的健康/入口复核。
- 旧 CLI 无 `Ready` 时保留前端尚未构建也可提供 API 的开发行为；默认地址、目录、扫描周期等不变。不添加全局 `WriteTimeout`。
- 显式 `NAS_WEB_DIR` 优先于发布嵌入；开发使用原 `web/dist`。GET/HEAD、正确 MIME、动态 JS/chunk 与 SPA 路由共享 FS 路由；缺失资产或带扩展名资源返回 404，不能回退 HTML 冒充脚本。
- `NAS_FFMPEG_PATH` / `NAS_FFPROBE_PATH` 非空时必须绝对路径且执行时权威生效，缺失不会静默回退。默认优先可执行文件同级 tools，再兼容绝对 PATH 定位；拒绝当前工作目录、相对 PATH 结果、目录及不可执行文件。工具缺失仍只使衍生处理降级，不阻断整个服务。
- Windows 真实临时测试发现长/短路径别名会使原下载隔离漏判相同 DataDir/DownloadRoot；`canonicalDataDirectory` 解析已有父目录后拼接尚未创建的尾路径，使数据根与下载根规范化一致，测试确认已拒绝重叠。

## 实际执行

使用仓库 `.tools/go/bin/go.exe` / `gofmt.exe`，没有更改全局 PATH 或系统 Docker/WSL 配置。

### Windows 当前主机

临时 `internal/app/runtime_p0_temporary_test.go` 实际执行：

```text
go test ./internal/app -run TestP0 -count=1 -timeout 75s -v
go test -tags webembed ./internal/app -run TestP0 -count=1 -timeout 75s -v
```

通过：

- 同进程及真实子进程独占排斥、关闭后重新获取、锁文件始终存在。
- 锁冲突前后数据库哨兵字节、待恢复缓存哨兵均不变；无 Ready、无已启动日志；外部 listener 关闭。
- 实际端口占用、预先取消、网页缺失、损坏 SQLite、已关闭 listener 的失败分支；网页/绑定失败在数据库创建前退出。
- 真实数据库、HTTP health、首页、SPA 均可用时 Ready 恰好一次，回调地址是实际分配端口；运行中锁被持续占有，关闭后端口和锁释放。
- 11 个静态路由子场景，包括 GET/HEAD、脚本 MIME、SPA/尾斜杠、缺失脚本/字体 404、路径穿越/反斜杠拒绝、方法限制。
- 真实构建前端的全部 JS 主包/动态块经内嵌 FS HEAD/MIME 检查；显式外部目录覆盖内嵌。
- 不修改环境的配置 lookup、原默认值、相对工具路径拒绝和最终目录下载隔离。
- 工具显式绝对路径、命令与诊断同路径、损坏配置不回退、当前目录/相对 PATH 拒绝、绝对系统 PATH 兼容。

Windows 符号链接别名用例因当前进程缺少创建 symlink 权限跳过；未修改系统策略。该用例在下面 Linux 分支实际通过。主进程直接执行 child-only 测试会跳过，但同一测试已经由 LeaseExclusion 用真实子进程单独执行成功。

### Linux 分支实际运行：WSL1 Ubuntu-22.04 amd64

先交叉编译测试到任务专属 `.runtime/windows-one-click/runtime-p0-*.test`，然后执行：

```text
wsl -d Ubuntu-22.04 --exec /mnt/e/vscodeProjects/nas-content-manager/.runtime/windows-one-click/runtime-p0-webembed-linux-amd64.test -test.run=TestP0 -test.skip=TestP0StartupFailures/port-before-database -test.v -test.timeout=45s
```

其余全部通过，包含 Linux flock 同进程/子进程排斥、符号链接目录别名排斥、SQLite/缓存前置保护、真实 HTTP/Ready/关闭、内嵌动态块、配置隔离和工具定位。

已如实排除的环境限制：首次完整测试在 port-before-database 子场景 75 秒超时。栈证明第二次 `net.Listen` 已成功，服务正在正常 Serve 等待取消，并非 flock 或数据库死锁。`wsl --list --verbose` 确认此发行版为 WSL **1**；`uname` 为 `4.4.0-19041-Microsoft`。独立 Python socket 同样用 SO_REUSEADDR 在同一进程二次 bind/listen 同一个 `127.0.0.1` 地址端口成功。未为迎合该环境更改产品 SO_REUSEADDR；Windows 的真实端口冲突用例已经通过，常规 Linux 内核的端口冲突仍需目标环境验收。本记录不将 WSL1 等同于完整 Linux NAS 实机验收。

### 构建与静态检查

通过：

```text
gofmt（本次修改和新增的 Go 文件，PowerShell 使用显式文件列表）
go vet ./internal/app ./internal/webassets
go test ./internal/app ./internal/webassets（临时测试清理后，仅为编译检查）
git diff --check -- internal/app/config.go internal/app/download_config.go internal/app/torrent_search_config.go internal/app/engine_config.go internal/app/server.go internal/app/cache.go internal/app/music.go
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/nas
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/nas
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/nas
```

普通构建二进制位于 `.runtime/windows-one-click/runtime-nas-*`。ARM64 仅编译，未真实运行；发布脚本、桌面包和完整回归由父任务另行记录。

## 清理与验收边界

- 已删除本次唯一临时单元测试源 `internal/app/runtime_p0_temporary_test.go`，并用 `Test-Path` 及清理后编译复核；未移除他人测试。
- 自建 `internal/webassets/dist` 暂存的递归删除被自动审批拒绝，未提供细分理由。经检查源/目标均位于本工作区且目标不存在后，改为安全整体移动到 `.runtime/windows-one-click/webembed-runtime-tested`，保留构建资源且释放源目录；父任务已获通知可继续发布构建。
- 未创建或拉取 Docker 容器、镜像、网络、卷；没有旧远端资源访问或全局 prune。
- 本轮工具测试证明定位/参数传递，不替代真实 FFmpeg 视频封面、音乐探测或许可/分发验收。
- 当前 Windows 主机通过不替代干净 Windows 11 普通用户离线包、Windows junction/UNC、Linux NAS 大视频性能和常规内核实际部署验收。
- 未改变 schema、原始媒体、下载发布门禁、共享业务模型或 Linux headless 入口。
