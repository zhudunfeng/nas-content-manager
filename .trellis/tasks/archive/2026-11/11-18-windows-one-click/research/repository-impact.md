# Research: 当前仓库的 Windows 启动与部署边界

- Query: 哪些现有能力可直接复用，哪些阻碍普通用户双击使用？
- Scope: internal
- Date: 2026-11-18
- Baseline: main，5e36995；本轮开始时工作区干净。

## Findings

| 当前文件/入口 | 已确认行为 | 规划影响 |
| --- | --- | --- |
| `README.md` | 无已发布公共镜像；部署需配置 `.env` 并源码构建；本机构建需携带 `web/dist` | 发布真正构建好的 Windows 包和固定版本 Docker 镜像，不能仅包装开发脚本 |
| `scripts/dev.ps1` | 调用 Go/npm，仅有开发/编译入口 | 保留开发用途；增加面向用户的 EXE，不要求用户安装开发工具 |
| `cmd/nas/main.go` | 读取环境变量，输出控制台日志，依赖终止信号关闭 | 新增 Windows GUI 子系统入口，复用后端；保留原 CLI |
| `internal/app/config.go` | 配置来自环境变量，路径相对当前工作目录；默认 `:8090`、`data`、`media`、`web/dist` | 原生启动器使用稳定数据目录、回环监听、持久配置；保留原 CLI/容器默认 |
| `internal/app/server.go:Run` | 打开数据库、启动下载/引擎/扫描后才 `ListenAndServe`，启动日志早于绑定结果 | 在打开数据库和恢复任务前取得数据目录锁；预先绑定并持有实际 listener，真实就绪后打开浏览器 |
| `internal/app/server.go:static` | 通过 `os.Stat/http.ServeFile` 提供外置资源，缺失时报前端未构建 | 抽象受限静态 FS；发布包嵌入构建产物，开发模式继续外置 |
| `internal/app/server.go:settings`、`cache.go`、`music.go` | 使用 `LookPath` 或命令名查找 ffmpeg/ffprobe | 统一工具定位到已校验包内绝对路径；诊断与执行使用同一个解析结果 |
| `internal/app/libraries.go:allowedPath/browse` | 仅允许部署预授权的根及子目录，并进行 canonical/symlink 检查 | 本机用户通过原生选择器增加根，网页仍沿用原目录授权和下载发布门禁 |
| `internal/app/disk_windows.go` | 已有 Windows 空间查询 | 保留并纳入 Windows 行为验收，不据此宣称全部 Windows 行为已验证 |
| `internal/app/download_gate.go` | Windows 有大小写路径 key；非 Linux 不读取 mountinfo | 原生外接 BT 需另做 NTFS/目录别名/凭据验证，不视为完成 Windows 托管支持 |
| `internal/enginehelper/config.go` | 要求 Linux 环境规范绝对 HostRoot，与 `/managed` 的 Docker bind 源严格一致 | 不能直接使用 Windows `E:/...` 作为现存 helper HostRoot |
| `internal/enginehelper/lease_other.go/permissions_other.go` | 非 Linux 明确拒绝进程租约、UID 身份和写入探针 | 原生启动器不能直接运行现有 helper.exe |
| `internal/enginehelper/files.go/service.go/templates.go` | chmod/chown、flock/fsuid、私有 Unix socket、严格 bind/模板/实例身份检查 | Windows Docker 全栈保持 app/helper/engines 全在 Linux 容器内；增加 named-volume 存储驱动而非放宽旧检查 |
| `compose.engines.yaml` | app/helper 在 Linux bridge 上，IPC 命名卷，配置/下载/连接仍绑定宿主专用目录 | 新增 Windows 专用组合，保留现有 NAS/Linux Compose；不能误认为当前 overlay 已适配 Desktop |

## Code navigation

优先使用 CodeGraph CLI（`status/query/files`）。当前索引可定位 LoadConfig、Run 和 browse，但缺少近期 helper/engine 文件，故缺口使用 `rg` 与定点读取；本轮未重建索引。

## Related specs

- `.trellis/spec/backend/runtime-contract.md`
- `.trellis/spec/backend/download-contract.md`
- `.trellis/spec/backend/engine-bootstrap-contract.md`
- `.trellis/spec/frontend/app-contract.md`

## Caveats / Not Found

- 仓库当前未见面向用户的 Windows 启动器、发布打包流水线或安装器。
- 本轮仅形成计划，没有构建/运行原生发布包，也未部署 Docker、修改系统或访问远程服务器。
- 旧 BT 任务在旧服务器上受到 registry/旧 seccomp 环境限制，不能将旧诊断结果升级为完整 Docker Desktop 两引擎成功证据。
