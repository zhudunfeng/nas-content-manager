# Windows 双击启动与 Linux 兼容

系统共用 Go/Vue/SQLite 业务内核。Windows 提供原生托盘入口，Linux 保留无桌面 CLI、环境参数和原有 Compose/bind 部署；当前数据库为顺序增量迁移后的 schema v3，保留原媒体和v2下载数据。

## 选择运行方式

| 方式 | 适用场景 | 前提与数据位置 |
| --- | --- | --- |
| Windows 原生媒体 | 视频、图片、漫画、小说、音乐 | 完整发行 ZIP 内的 `栖库.exe`、内嵌网页与配套 `tools`，用户数据位于本机独立目录 |
| Windows Docker 完整模式 | 媒体与网页初始化托管 BT 引擎 | 已运行的 Docker Desktop Linux/amd64 容器环境、固定镜像清单；数据和下载位于 Linux 命名卷 |
| Linux 原生 | 无桌面 NAS 或服务器 | 对应 amd64/arm64 归档，`./start.sh` 或 `./nas` |
| Linux Compose | 延续现有部署 | 原 `compose.yaml` 与可选 `compose.engines.yaml`，现有路径和 UID/GID 约定保留 |

首批新增桌面目标为 Windows 11 x64。Windows ARM64、其他 Windows 版本及特定 NAS 设备需要分别验证。跨架构构建通过不等于在每台机器实际运行通过。

## 原生模式

1. 从可信发布渠道取得完整包，核对归档旁的 `.sha256`，解压到稳定的本地目录。不要直接在 ZIP 中运行。
2. 双击 `栖库.exe`，首次在 Windows 系统目录对话框中授权媒体目录；取消可使用程序创建的空媒体目录，稍后从托盘添加。
3. 实际健康检查和网页就绪后自动打开浏览器，创建管理员、按内容类型添加媒体库。媒体文件只读访问。
4. 后续双击复用已保存的配置；重复启动通过当前用户私有 IPC 激活已有实例。关闭浏览器不退出服务。
5. 右下角托盘提供打开网页、添加/移除授权根、查看数据和日志、重启、退出，以及启动另一运行模式的入口。

在原生模式选择托盘「退出栖库」后，启动器会停止本实例的 HTTP 连接、后台任务及 FFmpeg/ffprobe 进程树，等待清理完成后结束进程并解除程序文件占用。退出完成后即可在原路径覆盖 `栖库.exe`，或重命名、替换整个程序目录，再使用原配置启动；更新包内工具时应一起替换配套 `tools` 目录。数据库、媒体和日志文件也会关闭，浏览器正在播放或下载媒体的 HTTP 连接会中断。关闭浏览器和网页「退出登录」仍分别表示关闭页面和退出账户。

媒体工具的回收仅限栖库为当前命令创建的进程树，不会按进程名称关闭其他工具或用户浏览器。Docker 模式的「退出启动器」继续保留容器；停止容器仍使用「停止完整模式」。

默认配置和日志位于 `%LOCALAPPDATA%\Qiku\profiles\native`，默认业务目录在其 `data` 子目录。Docker 模式使用独立的 `docker` 配置目录。稳定实例身份保存在 `desktop.json`，升级时保留该文件。

配置变更通过本机校验、原子保存和有序重启生效。移除授权根保留已有索引与原文件，但不再允许访问该目录。更换“独立数据目录”会打开该目录自己的数据库或创建新库，**不会迁移账号、收藏或进度**。媒体网络盘暂不可用时保留原授权，扫描失败不清空已有索引。

默认使用 `127.0.0.1:8090`。端口冲突时真实绑定后续最多 20 个候选端口，并保存成功端口；不会误打开占用端口的其他网站。桌面入口限制回环访问。确需局域网访问时，使用 `nas.exe`，显式配置 `NAS_ADDR`、同一数据目录和媒体根；先退出托盘实例，避免争用同一数据库，并配置相应网络访问控制。

本机诊断示例（PowerShell，路径按解压位置修改）：

```powershell
# 控制台构建入口；输出配置但不启动服务。
& '.\栖库.exe' --print-config
# 隔离配置并明确授权一个目录，不打开托盘/浏览器。
& '.\栖库.exe' --headless --no-browser --profile 'E:\Qiku Profiles\native' --media-root 'F:\媒体'
# 对同一配置的已有实例查询、打开网页或有序退出。
& '.\栖库.exe' --profile 'E:\Qiku Profiles\native' --control status
& '.\栖库.exe' --profile 'E:\Qiku Profiles\native' --control open
& '.\栖库.exe' --profile 'E:\Qiku Profiles\native' --control quit
```

GUI 子系统的 EXE 不保证 PowerShell 显示标准输出；诊断结果同时查阅配置目录下的轮转日志。源码调试可使用 `go run ./cmd/nas-desktop --headless ...` 或编译不带 `-H=windowsgui` 的控制台版本。

## 桌面图标

`栖库.exe`、原生窗口和托盘使用与网页一致的深色薄荷绿品牌图标。EXE 内嵌十种尺寸（16–256 像素），无需在程序旁放置 ICO 文件。托盘从内嵌资源加载小图标，窗口使用独立的大小图标；退出时回收对应句柄。

普通源码构建和 GitHub Actions 发布构建自动带上已提交的图标资源，不需要额外安装资源编译器。请按包路径运行 `go run ./cmd/nas-desktop`，不要只运行单个 `main_windows.go`，否则可能遗漏包目录的 `.syso` 资源。维护图标的再生步骤见 [发布流程](release.md)。

更新后若资源管理器仍显示旧图标，可先用新文件名或新解压目录查看；Windows 可能保留旧路径的图标缓存，无需修改应用数据或全局清理系统缓存。

## 本机配置

仅在退出实例后手动修改 `desktop.json`，损坏、未知字段和不匹配模式会拒绝启动并保留原文件。参数优先级为明确 CLI 参数 > 对应 `NAS_*` 环境 > 保存配置 > 默认值；不会全局修改 PATH 或环境。完整 CLI 说明使用 `--help`。

| 字段 | 中文说明 |
| --- | --- |
| `schemaVersion` | 固定为 1，未知版本拒绝读取 |
| `instanceId` | 自动生成的 32 位十六进制安装身份，不要手动更改或跨配置复制 |
| `mode` | `native` 或 `docker`，两种模式分开保存 |
| `dataDir` | 原生模式的本地绝对业务目录，SQLite不使用UNC网络共享；Docker业务数据另存于实例命名卷 |
| `mediaRoots` | 本机用户授权的目录数组，原生最多 32 根；Desktop Compose 首版限制一个只读根 |
| `port` | 首选端口，1024–65515，冲突时最多向后寻找 20 个 |
| `autoOpen` | 真实就绪后是否自动打开浏览器 |
| `dockerContext` | 明确选择的本机 Docker Desktop Linux context，默认 `desktop-linux` |
| `imageManifest` | 固定镜像清单位置，相对路径相对程序目录解析 |

`--profile` 指定配置/日志根，`--data-dir` 仅指定业务数据根，含义不同。原有 `nas` CLI 不读取桌面配置，仍按原环境参数运行。

媒体工具按“显式绝对 `NAS_FFMPEG_PATH` / `NAS_FFPROBE_PATH` > 可执行文件旁 `tools` > 安全 PATH”顺序查找。所有封面、探测和设置诊断共用解析结果。保留整个 `tools` 目录及共享构建所需 DLL/库；不要只移动 EXE。开发包不携带工具时不能当作完整离线包分发。

## Docker 完整模式

首次安装或启用 Docker Desktop/WSL 可能需要管理员操作与重启；启动器不会自动安装或修改这些系统设置。用户先手动启动 Docker Desktop，选择 Linux 容器环境。然后从托盘选择 Docker 完整模式，或执行：

```powershell
# 启动本机Docker独立配置，等待应用真实就绪。
& '.\栖库.exe' --mode docker
# 诊断方式查看状态；无可用Docker时显示真实前置条件错误。
& '.\栖库.exe' --mode docker --headless --docker-action status
# 先退出同profile托盘启动器，再核验并停止本实例动态引擎及应用/助手；保留卷。
& '.\栖库.exe' --mode docker --headless --docker-action stop
```

包中需要 `compose.desktop.yaml` 和维护者提供的 `release-images.json`。当前仓库没有已发布的公共应用/助手镜像；缺少清单会明确报错，不会拉取虚构镜像。维护者生成方式见 [发布流程](release.md)。镜像只接受精确 SHA-256 ID 或仓库 digest；带离线归档时先流式校验再 `docker load`。在线 registry 认证由本机 Docker 管理，凭据不进入网页和发布清单。

新版 **v3** 镜像清单配合`compose.desktop.components.yaml`，可从“资源收集 → 添加来源”部署MoviePilot、AutoBangumi、Sonarr、Radarr。主动确认后创建固定组件及私密配置卷，完成首次配置再填写API凭据接入；组件数据放在独立`collection-media`卷，原Windows媒体盘仍只读。流程与入口见[资源组件](components.md)。

v3可选附带获取镜像，旧v2必须带获取镜像，均配合`compose.desktop.collection.yaml`。获取服务不发布控制端口，仅写本实例的`collection-data`/`collection-secrets`卷；主应用只读访问并执行发布门禁。完整停止包括新资源组件和可选获取服务，保留所有数据卷。旧v1/v2保留原角色和挂载，不隐式启用新组件部署。原生模式可连接独立worker或已有资源组件，参数和恢复步骤见[资源收集](collection.md)。

启动器显式传入本机 context，拒绝 TCP/SSH/远端 daemon，清除可能干扰的 Docker/Compose 环境。Compose 项目、网络和卷前缀由当前配置的稳定实例身份派生。复用与停止前核对实际镜像、标签、挂载和项目身份，拒绝接管同名外来资源。

**下载、数据库和引擎配置位于 Docker Desktop Linux 数据盘**，不是 `E:\Downloads`。首版不提供任意 NTFS 下载根或自动跨模式数据迁移。容量和数据盘位置请在 Docker Desktop 设置中查看。视频等既有媒体根仍可只读挂载 Windows 目录；首版 Docker 配置限制一个媒体根，多根请求会给出错误提示。

应用就绪后，进入“下载中心 → 引擎管理”，主动确认后初始化 qBittorrent/Jackett。引擎安装成功不表示 Jackett 已有可用站点；仍需配置来源并同步。原生模式不自动安装 Windows 版 BT 引擎。

退出 Docker 模式启动器会保留容器继续运行；托盘中的“停止完整模式”才会先请求助手停止本实例引擎，再停止应用和助手。命令行`--docker-action stop`要求同一profile的启动器已退出，运行中请使用托盘停止；重复命令不会静默变成打开网页。正在初始化时停止请求可能返回冲突，待任务完成后重试。重新打开Compose不会自动启动已停的动态引擎，需在页面使用现有“启动”动作。不要用`compose down`代替完整停止，更不要使用`down -v`删除数据卷。

## 升级、备份和恢复

原生升级：网页下载一致性备份 → 托盘退出并确认进程停止 → 另外复制整个配置/业务目录 → 解压新包到新目录 → 启动并复用原配置。不要用普通文件复制来备份运行中的 WAL 数据库。应用目录和数据目录分离，不需要覆盖配置即可换包。

原生恢复：退出实例，保存现有 `library.db`、`library.db-wal`、`library.db-shm` 到独立备份位置；将网页下载的快照放为 `dataDir/library.db`，不要遗留旧 WAL/SHM，再启动。保持原媒体根一致。回滚程序时恢复配套数据库备份，不能假定旧版能读取新版 schema。

Docker 恢复除网页数据库快照外，还须停机备份应用数据卷、助手 state、qB/Jackett 配置、下载及 connections 卷和本机 `desktop.json`。启用组件部署后还须备份 `collection-media` 和四产品各自的私密配置/缓存/数据库卷；附带获取服务时也备份其数据与秘密卷。凭据与安装标记属于敏感资料，保持卷角色和实例身份一致。逐卷恢复到原 deployment，不要把不同安装的标记混用。数据库备份不包含媒体、载荷或引擎凭据。跨 Linux/Windows 路径迁移不在本期自动化范围。

## 人工验收清单

- 干净 Windows 11 普通用户：无 Go/Node/Docker/全局 FFmpeg，完整包双击、目录选择、五类媒体和内嵌封面。
- 中文/空格目录、多盘符、从其他工作目录启动、重复双击、端口被第三方占用、托盘退出/重启、断网复启。
- 托盘退出后立即覆盖原 EXE、重命名/恢复整个程序目录，使用覆盖后的程序复用同一配置启动；实际使用共享构建媒体工具时，还需在目标完整包验证其 DLL 和工具文件可替换。
- 损坏配置保留原文件，离线盘不会清空索引，升级后账号/收藏/进度保留。
- Docker Desktop 完整模式：固定镜像安装、初始化真实引擎、来源同步、重启恢复、精确停止和无关项目保留。
- 超大影片实际解码/起播/拖动/吞吐在目标硬件实测；5 GiB 稀疏协议样本仅证明64位Range。

自动检查与实际执行边界见 [验证记录](verification.md)。本轮不自动启动 Docker、不进行部署验证。
