# Research: Windows 一键启动路线与 BT 托管边界

- Query: 如何降低现有 Go/Vue/SQLite 栖库在 Windows 上的部署成本，并合理保留完整 BT 引擎托管能力？
- Scope: mixed（仓库只读核对与官方文档）
- Date: 2026-11-18
- 输出范围：仅调研与方案建议，未实现、未安装依赖、未连接远程服务器。
- 用户最新硬约束：系统必须同时兼容 Linux 与 Windows；Windows 便利化不得牺牲现有 Linux NAS 能力。

## Findings

### 1. 推荐路线

推荐采用同一 Go Windows 启动器、两个明确隔离的运行模式：

1. **原生媒体模式，默认交付**：预编译 Go 程序、构建时嵌入 Vue 产物、原生托盘与文件夹选择、系统浏览器。终端用户不需要 Go、Node、Docker 或 WSL。FFmpeg/ffprobe 可以通过经过许可审查的随包工具提供，或作为明确可选增强组件；缺失时保留基础媒体功能。
2. **Docker 完整模式，可选交付**：Windows 启动器仅作本机部署控制，主应用、helper、qB、Jackett 全部保持 Linux 容器、同一专用 bridge 和 Linux 存储语义。Docker Desktop/WSL2 等前置环境需要检测、引导，不能承诺新机器首次完全无人值守。

两个模式不共享同一个运行数据库和安装身份，不能通过“切换模式”悄悄切换路径语义或把原生数据库直接挂给 Linux 应用。迁移应另有显式导入、路径重映射与下载状态约束。

上述路线是结合现有代码做出的工程判断，不是 Go 或 Docker 官方对本项目的背书。

### 1.1 Linux 与 Windows 同时兼容的强制边界

- 共享 Go/Vue/SQLite 与媒体业务内核，不分叉一套 Windows 媒体服务；平台差异集中在入口、目录选择、配置目录、单实例、工具路径和桌面生命周期层。
- 原 `cmd/nas` CLI、环境变量配置、Linux headless 运行、生产 Dockerfile、Linux bind 部署与现有 API 必须兼容；Linux 服务器不需要桌面、浏览器、托盘或 WebView 才能启动。
- Windows-only 的 `golang.org/x/sys/windows`、Win32/COM 调用与平台结构体置于 `_windows.go` 或相应 build tags，其他平台提供匹配接口或明确能力降级。不能将 Windows GUI 初始化放到共享包的无条件 import/init 中。
- Windows 原生服务与 Linux 服务可以使用相同数据库 schema，但不能共用一个运行中的 DB，不能假设 `E:/Media` 与 `/media` 表示同一目录；跨模式/跨平台迁移需要停机快照、路径重映射和下载状态核对。
- 新 named volume 驱动是额外能力，不能把现有 Linux helper bind 驱动整体删除或放宽归属核验。Linux 与 Windows Docker Desktop 分别验收。
- 构建矩阵建议为 Linux amd64/arm64 的 app/helper、Windows amd64 的 app/launcher。Windows 原生不构造可运行的 helper；Windows ARM64 如未做原生运行验证须标明未纳入首期支持。
- 既有任务虽记录过 Linux amd64/arm64 交叉编译与部分真实 Linux Docker 验证，本轮未重复构建或部署；交叉编译成功不等于 ARM64 硬件、Windows 桌面、Docker Desktop 或新版存储契约已实测。

### 2. 当前仓库能力与约束

| 文件与位置 | 已核实行为 | 规划影响 |
| --- | --- | --- |
| `cmd/nas/main.go:15` | 配置、结构化日志、信号上下文、有序停止入口 | 可以复用应用生命周期，不必重写媒体业务 |
| `internal/app/config.go:31` | 从环境读取参数；默认数据、媒体和网页目录依赖当前工作目录 | Windows 启动器应指定稳定绝对目录和独立配置优先级 |
| `internal/app/server.go:53` | 应用持有 SQLite、HTTP 与任务生命周期 | 托盘退出应走取消与优雅关闭，不是常规强杀 |
| `internal/app/server.go:243` | 静态服务直接读取外部 `web/dist` | 发布包应嵌入静态资源或固定随包目录，不能只分发当前 exe |
| `internal/app/server.go:272` | FFmpeg/probe 通过 PATH 探测 | 随包工具需要受控绝对路径与统一探测/执行策略 |
| `scripts/dev.ps1` | 开发启动/构建依赖 Go 和 npm | 不能把开发脚本称为最终用户免环境一键包 |
| `internal/enginehelper/service.go:112` 附近 | helper 在非 Linux 上拒绝启动 | 不应将 helper 直接编译 Windows 版并声称可用 |
| `internal/enginehelper/config.go:26` | 路径要求 Linux 规范绝对路径，包括 HostRoot | `E:/...` 从 Linux helper 看不是规范绝对宿主路径；仅修改环境值不足以复用 |
| `internal/enginehelper/docker.go:60`、`:110` | Docker 连接固定 Unix socket；预检 Linux bridge、架构、安全模式 | 完整模式应保留 Linux helper，不新增未认证远端 Docker TCP |
| `internal/enginehelper/lease_linux.go:13` 与 `lease_other.go:11` | 状态打开之前通过 root 私有文件和 flock 取得安装级锁；非 Linux 拒绝 | 不能把 Windows 命名互斥量替换认定为完整 helper 移植 |
| `internal/enginehelper/permissions_linux.go:47` | 在锁定线程中以目标 fsuid/fsgid 做读写探针 | Windows bind 的可写成功不能替代 Linux UID 权限验证 |
| `internal/enginehelper/files.go:20`、`:78`、`:118` | 严格属主/0700/0600、Chown、同目录原子发布和目录 fsync | 私密 helper 状态、凭据与连接应放 Linux 文件系统 |
| `internal/enginehelper/templates.go:72`、`:101` | 当前模板只接受固定 bind mount，比较 daemon inspect 的精确 source、读写性与安装归属 | named volume 支持需要明确新存储契约与验证，不能把 volume 名直接填 HostRoot |
| `internal/app/engine_runtime.go:43` | 应用通过 Unix socket 访问 helper，接收 Linux 容器连接清单 | 原生 Windows 应用与 Linux helper 混跑需要专门 IPC/网络/凭据桥，MVP 无此必要 |
| `internal/app/download_config.go:115` | 引擎路径可规范化 `/...` 或 `C:/...` | 不能误称所有原生外接下载从配置层完全不支持；也不能把路径解析视为完整 Windows 验收 |
| `internal/app/download_gate.go:60`、`:115`、`:168` | Windows 路径键忽略大小写；Linux 额外读取 mountinfo 做物理挂载别名隔离 | 原生外接下载必须独立验证 junction/重解析点、别名、UNC、磁盘身份和发布门禁 |
| `internal/app/download_import.go:88` | 已选文件和版本经实际目录、根句柄、尺寸、mtime 核验才发布 | 完整模式必须保持同一下载存储视图，不能绕过门禁自动扫描 staging |

CodeGraph 已优先查询 `LoadConfig`；当前索引缺少部分 engine/helper 新符号，因此以定点读取和 `rg` 补充。未重建或修改索引。

### 3. 可比较的实现模式

| 模式 | 对终端用户的体验 | 与当前架构的适配 | 主要代价 |
| --- | --- | --- | --- |
| Go 原生启动器 + 浏览器 | 解压双击；托盘控制；使用系统浏览器 | Go/Vue 服务与接口基本复用，网页可编译嵌入 | 需要实现 Win32 生命周期、单实例、目录选择和诊断；BT 托管单独处理 |
| Docker Desktop + 启动包装 | 环境就绪后双击运行完整栈 | 最大限度保留 Linux app/helper/qB/Jackett 隔离 | 首次依赖虚拟化/WSL2；需要新持久卷契约与可靠安装向导 |
| Wails 桌面壳 | 独立桌面窗口，可集成托盘和原生能力 | Go + Vue 语言栈契合，但需要改造桌面绑定、资源和发布流程 | Windows WebView2 运行时需要检查；仍不解决 helper 的 Linux 限制 |
| Tauri 桌面壳 | 独立桌面窗口与发行框架 | Vue 可复用，Go 服务通常作为 sidecar | 增加 Rust/MSVC 构建链及 sidecar 生命周期，Windows 仍使用 WebView2 |

Go 官方 embed 支持编译时将文件纳入程序，模式相对于所属包，不能用 `../` 向上引用 `web/dist`。建议用专用资源包或构建时复制到包内；保留显式外部资源目录给开发/Docker 场景，区分无意漏包与有意开发覆盖。Go build 官方文档提供目标平台构建参数，Windows 可执行文件为 `.exe`；仓库已有 Windows 构建脚本，但发布包仍需独立验收。

Win32 原生文件夹对话框与通知区域图标是系统能力，可通过小范围平台适配实现；无需为了目录选择把整个前端迁入 WebView。实现仍需评估 COM 线程、消息循环、取消对话框、Explorer 重启后图标恢复和无管理员会话。

### 4. Docker Desktop 首次启动边界

官方文档截至调研日区分 Docker Desktop 安装模式与后端：不能笼统声称“安装 Docker 一律需要管理员”。当前 per-user 安装可以无需管理员，但首次启用 WSL2/虚拟化特性仍可能需要管理员和重启。Windows 系统版本、硬件虚拟化、WSL 版本和容器后端均需检测，主线验收先限定 Windows 11 x64 + Linux containers。

安装器/启动器应显示：未安装、已安装未运行、后端不可用、等待用户操作、需要重启、引擎就绪等真实状态。不要自动同意第三方许可、改 BIOS、重启电脑或关闭安全策略。环境就绪后才运行已固定版本的生产镜像与 Compose，避免最终用户源码构建。

现有阿里云镜像加速可以作为 Docker daemon 已配置能力使用，前端提示拉取故障和镜像摘要即可。不要写死某个私人 mirror 地址、修改 daemon 全局配置或通过取消摘要验证来宣称加速成功。

### 5. 完整 BT 模式的存储取舍

**默认推荐：安装级独立 named volumes，新增受控 volume 存储驱动；保留原 Linux bind 模式。**

- 应用 SQLite、helper state、连接清单、qB 配置、Jackett 配置和下载数据留在 Linux daemon 管理的卷中，原 Windows 媒体目录仅只读 bind 给应用。
- 为避免依赖新 Docker 的 volume-subpath 功能，可以为每种职责创建独立命名卷，helper 挂到固定内部目录；qB 与应用共享 downloads 卷，应用只读；连接清单卷与 Unix socket 卷同理。
- 不能只改 Compose：需要把配置、Docker inspect 模型、模板创建/核对、存储归属、恢复、诊断和备份一起升级。现模板严格拒绝 `Type != bind`，新驱动应检查 volume 名称、安装标签、精确挂载目标和读写性，禁止附加挂载与接管外来卷。
- 禁止由 Windows 直接访问 Docker VM 下的卷物理目录或 `/var/lib/docker/volumes/...`。备份/导出走受控接口或已核实归属的停机卷导出流程。
- 默认 Linux 卷中下载内容不会自动成为 `E:/Downloads`；界面必须说明存储位置与 Docker 虚拟磁盘空间，提供明确导出/容量管理规划。若硬性要求 Windows 可直接管理下载文件，需要另做 Windows 数据 bind 能力验证，不能把所有私密配置也搬过去。

**低改动备选：用户 WSL2 发行版内的专用 Linux 根。**

- 在已配置 Docker WSL integration 的发行版中创建真正 Linux 文件系统目录，保持现 Linux bind 模式；不是 `/mnt/c`、`/mnt/e` 或 Windows UNC 目录的别名。
- 必须在该实际配置验证 daemon 所见 bind source 与 helper 的精确路径检查一致，不能预设 Windows/WSL/Desktop 路径翻译与 inspect 永远保持相同。
- 代价是用户还要管理发行版、Linux 用户、Docker integration 与存储路径；与“一键降低门槛”目标相比适合作为高级文档路线，而非默认发行。

Microsoft 对 WSL 文件权限的官方说明表明：Windows 文件默认没有 WSL metadata，chmod 行为与 Linux 文件系统不同；不能据此保证所有 Docker Desktop sharing 实现都完全相同，但足以要求不要假设 NTFS bind 保留本 helper 所需的 Unix 私密权限。

### 6. 建议分期与验收

1. **跨平台可分发运行核心**：嵌入网页、稳定数据目录、配置文件、统一工具路径；干净 Windows 不安装开发工具即可访问五类媒体，同时原 Linux CLI/环境变量/无桌面服务继续可用。
2. **可见的一键入口**：托盘、单实例、就绪后打开浏览器、真实端口冲突、优雅退出、目录选择；关闭网页不等于停止服务。
3. **首次引导与维护**：管理员初始化、媒体根授权、普通用户权限、开机启动可选、日志/备份/故障导出、显式升级回滚。
4. **完整 BT 模式**：先完成 Linux volume 契约，再串联 Docker 预检/就绪/拉取/初始化/来源配置；首次未安装 WSL 的场景允许停在可恢复的用户操作阶段。

两种模式必须分别验证：路径含中文/空格，双击多次，启动失败可见错误，端口占用，普通用户/只读安装目录，断网，FFmpeg 缺失或故障，睡眠唤醒，退出与重启，配置损坏与升级失败。完整模式还要覆盖外来同名资源、卷身份、Unix socket 权限、helper 重启恢复和发布门禁。超大视频保持 HTTP Range 与流式读取，不引入 Blob 中转或全文件复制。

平台回归还应覆盖 Linux 无 `DISPLAY`/无桌面启动、现 Compose 与 bind 参数不变、Windows build tags 隔离，以及同一前端/API 的五类媒体回归。实施时分别记录编译、原生运行、真实引擎与硬件平台证据，不将交叉编译计为运行验收。

## External References

访问日期均为 2026-11-18；仅使用官方资料。版本号在实际实施时应重新固定，不据搜索摘要宣称某框架已被本项目集成。

1. Go embed：https://pkg.go.dev/embed — 编译时资源、包相对路径与 `..` 限制。引用标识 `turn716856search0`。
2. Docker Windows install：https://docs.docker.com/desktop/setup/install/windows-install/ — 安装模式、前置环境和首次启用 WSL2 权限。引用标识 `turn546955view0`。
3. WSL install：https://learn.microsoft.com/en-us/windows/wsl/install — 首次启用命令和重启。引用标识 `turn309317view0`。
4. Docker WSL best practices：https://docs.docker.com/desktop/features/wsl/best-practices/ — Linux 文件系统与 Windows bind 的差异。引用标识 `turn546955view1`。
5. Docker volumes：https://docs.docker.com/engine/storage/volumes/ — daemon 管理的持久卷、生命周期、共享和备份边界。引用标识 `turn546955view2`。
6. WSL file permissions：https://learn.microsoft.com/en-us/windows/wsl/file-permissions — metadata 默认关闭、chmod 与 NTFS 权限。引用标识 `turn546955view3`。
7. Windows folder dialog：https://learn.microsoft.com/en-us/windows/win32/shell/common-file-dialog — IFileDialog 现代对话框、COM 线程及取消处理。引用标识 `turn725469view0`。
8. Windows notification area：https://learn.microsoft.com/en-us/windows/win32/shell/notification-area — Shell_NotifyIcon 系统通知区域图标。引用标识 `turn725469view1`。
9. Wails Windows：https://v2.wails.io/docs/v2.15.0/guides/windows/ — WebView2 运行依赖与处理策略。引用标识 `turn309317view1`；v3 安装说明 https://v3.wails.io/quick-start/installation/，引用标识 `turn309317view2`。
10. Tauri prerequisites：https://v2.tauri.app/start/prerequisites/ — Rust/MSVC 构建环境与 WebView2。引用标识 `turn309317view3`。
11. Go build：https://pkg.go.dev/cmd/go — 构建与环境参数，Windows 可执行文件后缀。引用标识 `turn725469view2`。

## Related Specs

- `.trellis/spec/backend/runtime-contract.md`：本地 SQLite、原始媒体只读、Range、受限后台任务。
- `.trellis/spec/backend/download-contract.md`：发布选集与下载根隔离。
- `.trellis/spec/backend/engine-bootstrap-contract.md`：Linux helper、固定模板、私有 IPC 与归属核验。
- `.trellis/spec/frontend/app-contract.md`：现有主题、组件与可访问交互。
- `docs/engines.md`、`compose.engines.yaml`：现 Linux 托管部署边界。

## Caveats / Not Found

- 本轮未运行 Windows 发行包、未安装 Docker/WSL、未执行 BT 传输；所有新交付能力属于规划。
- 现有 Go Windows 编译能力不自动证明重解析点、UNC 与外接下载发布门禁满足新产品验收。
- 首次 WSL/虚拟化启用可能需要用户操作；不能将完整 BT 套件描述为 Windows 纯原生、无依赖、零确认一键安装。
- 官方资料未提供“Windows Docker bind 可原样满足本 helper 严格 Unix 权限契约”的承诺；必须通过适配和目标平台验收确认。
- 已存在 Docker daemon accelerator 只代表可尝试，并不保证固定 GHCR/Docker Hub 镜像和摘要都能从阿里云代理获得。
- Wails 比较依据官方 v2 Windows 指南与 v3 quick-start；不据此强制迁入某一框架版本。
