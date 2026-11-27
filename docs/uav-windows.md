# Windows 原生视频搜索与下载

Windows 原生桌面入口通过同一个托管获取 worker 调用独立 UAV 私有 Python，提供 Jable、MissAV、Hanime1、黄果、SupJav 五站搜索与视频下载。首批目标为 Windows 11 x64 普通用户。站点业务与限制见 [UAV 使用说明](uav.md)，实际执行范围见 [验证记录](verification.md)。

## 安装与启用

选择可信的新版 Windows x64 桌面包，核对归档摘要后解压整个目录，必须包含匹配的 `栖库.exe` 与 `nas-collection-worker.exe`。启用 UAV 后，程序优先使用有效随包依赖或当前 profile 中已初始化的依赖；缺少或校验不通过时，在后台联网准备锁定版本和 SHA256 的 Python、UAV、yt-dlp 与 FFmpeg/ffprobe。无需手动复制 DLL 或预装 Python。

需要首次启动就能离线使用时，仍选择完整视频包，包内应有 `uav-runtime.json`、`runtimes/uav/`、`tools/yt-dlp.exe`、FFmpeg/ffprobe 及相应许可材料。自动初始化不改变开发包的发行身份，也不代表完成了正式分发的许可/对应源码审查。

终端无需安装 Python、pip、Go、Node 或 Docker，不修改系统 PATH。不要直接从 ZIP 运行，也不要将开发机 venv 复制成便携运行时。默认 profile 位于程序目录下的 `qiku-data`，因此该目录需要可写；若程序目录只读，使用 `--profile` 指定可写位置。配置、秘密、日志和暂存位于本机 profile；媒体仍使用已授权的业务目录。

1. 按下节配置开启 UAV，启动 `栖库.exe`。新原生 profile 在有 worker 且有随包插件清单或显式开启 UAV 时选择托管；已有停用或外接配置继续保留。
2. 需要时在托盘选择「启用本机资源获取」，查看漫画、视频各自的依赖状态；也可用固定 IPC 命令启用或重新检测。
3. 进入“资源收集”，添加 UAV 视频来源，选择站点、搜索、预览，再确认下载。下载、校验、发布与入库继续走原流程。
4. 一个可选插件不可用时，其他已就绪插件和已发布媒体仍可使用。依赖就绪仅表示本机材料可用，不代表站点或网络当前可达。

托盘「本机获取组件」显示总体状态，漫画和视频分别显示「已就绪」「依赖未就绪」或「未配置」；worker 整体失败时显示「未检测」。初始化时额外显示中文阶段和当前下载物料的字节进度，IPC `status` 中保留 `collectionInitialization`，并用 `collectionInitializations` 分别保存漫画和视频状态。两者都需准备时先漫画、后视频，一项普通失败不阻断另一项，整批结束后至多自动有序激活一次。操作为「启用本机资源获取」「停用本机资源获取」「重新检测获取组件」。普通“启用本机资源获取”不会擅自选择一个未配置的视频插件。

首次联网准备期间基础媒体服务可用；准备完成会短暂有序重启本机服务以载入，正在播放或下载的连接可能中断。失败后不会无限重试，可修复网络/代理后选择「重新检测获取组件」或退出再启动。停用、重检和退出会等待后台初始化取消与清理。关闭浏览器不结束后台服务；升级前从托盘退出，等待本实例 worker、Python、下载器及 FFmpeg 完成回收，再成套替换程序和运行时。

## 在 .env 中启用视频插件

在 `栖库.exe` 同级创建 UTF-8 `.env`，或复制发行包 `.env.example` 后改名，加入：

```dotenv
# 选择本机托管获取，已有停用或外接配置也会被明确切换。
NAS_DESKTOP_COLLECTION_MODE=managed
# 启用视频插件；缺依赖时自动联网准备。
NAS_COLLECTION_WORKER_UAV_ENABLED=true
```

保存后从托盘退出再启动。命令行参数优先于进程环境变量，进程环境变量优先于 `.env`；同名环境变量存在时文件值不会覆盖它。文件也可配置 `NAS_COLLECTION_WORKER_UAV_PYTHON` 和既有 worker 配额。显式自定义 Python 由管理员维护全部依赖，错误时不安装、不回退。托管清单、下载器、FFmpeg、地址和状态目录仍由桌面固定。完整语法、空值处理与配置范围见 [程序同级配置](windows.md#程序同级-env-配置)。独立 `nas-collection-worker.exe --check-uav` 只读取环境变量，不自动读取桌面 `.env`，也不会安装文件。

## 模式与本地诊断

```powershell
# 当前实例未运行时，显式以本机托管模式启动。
& '.\栖库.exe' --collection-mode managed
# 同一profile主动启用、停用、重检或查看状态。
& '.\栖库.exe' --control collection-enable
& '.\栖库.exe' --control collection-disable
& '.\栖库.exe' --control collection-check
& '.\栖库.exe' --control status
```

使用自定义 `--profile` 时，所有命令都指定同一 profile。GUI 子系统不保证向当前终端显示输出，状态与错误同时查看 profile 日志。重检会有序重启本机服务，请先保存当前操作；不会自动重发结果未知的下载。

管理员可单独检查本地视频依赖，以下变量仅作用于当前 PowerShell 及其子进程：

```powershell
# 启用本地诊断并选择包内固定清单与工具，不需要服务令牌。
$env:NAS_COLLECTION_WORKER_UAV_ENABLED = 'true'
$env:NAS_COLLECTION_WORKER_UAV_RUNTIME_MANIFEST = (Resolve-Path '.\uav-runtime.json').Path
$env:NAS_COLLECTION_WORKER_YTDLP = (Resolve-Path '.\tools\yt-dlp.exe').Path
$env:NAS_COLLECTION_WORKER_FFMPEG = (Resolve-Path '.\tools\ffmpeg.exe').Path
& '.\nas-collection-worker.exe' --check-uav
```

诊断只创建并清理专用临时目录，检查固定运行时、解析依赖和本地工具；不监听、不恢复任务、不请求站点。退出码 `0` 为依赖就绪，`2` 为未启用或未就绪，`1` 为诊断自身错误。

| 参数／字段 | 中文解释 |
| --- | --- |
| `desktop.json.collectionMode` | 复用 `disabled`、`managed`、`external`；漫画与视频共用一个 worker，不代表删除历史任务或媒体 |
| `NAS_COLLECTION_WORKER_UAV_ENABLED` | 普通手工 worker 的视频插件开关，默认 false；桌面按随包或显式运行时配置受控启用 |
| `NAS_COLLECTION_WORKER_UAV_RUNTIME_MANIFEST` | 私有 UAV 清单绝对路径；桌面选择有效随包或 profile 已发布清单，普通管理员模式可显式设置 |
| `NAS_COLLECTION_WORKER_UAV_PYTHON` | 高级可信解释器覆盖；托管时必须为绝对路径，显式错误不回退到包内或系统 Python |
| `NAS_COLLECTION_WORKER_YTDLP` | 固定下载器可执行路径；桌面指定受控随包或已初始化绝对路径 |
| `NAS_COLLECTION_WORKER_FFMPEG` | 本地媒体校验工具路径；桌面指定受控随包或已初始化绝对路径 |

UAV 与 JMComic 各自拥有独立运行时和清单，不能互相覆盖 site-packages。普通控制台部署仍可按 [手工部署说明](uav.md#原生执行端) 使用可信独立环境；Linux/Docker 与外接连接沿用既有配置。

## 数据与排错

默认原生 profile 为 `栖库.exe` 同级的 `qiku-data\profiles\native`。托管 worker 使用其中 `collection-managed/`，日志在 `logs/collection-worker.log`。程序目录只读时请使用 `--profile` 指定可写目录。不要把 SQLite、令牌或所有权标记迁到 UNC/网络目录，也不要删除标记来接管未知目录。

自动准备的依赖位于 `<profile>/collection-managed/uav-bootstrap/`，使用独立暂存和不可变 `gen-<32hex>` 目录。下载归档严格核对大小、SHA256 和安全路径，再核对运行时完整库存、架构、来源、RECORD 及工具；改名到最终目录后，真实 worker 离线探测通过才原子发布 `current.json`。失败不会用半成品替换现有依赖。已发布依赖重启时只读复用；更新程序后若固定配方发生变化，会重新验证并按需准备。应预留下载归档、解压文件和旧 generation 共存的空间。固定下载遵循当前进程既有 HTTP(S) 代理环境，不修改全局代理；网络瞬断或临时服务器错误最多尝试三次，完整性错误不重试。

| 状态 | 处理 |
| --- | --- |
| 视频未配置／停用 | 配置 UAV_ENABLED=true，并确认是 native + managed；已有 disabled/external 不会被隐式覆盖 |
| 正在初始化视频依赖 | 等待托盘阶段完成；基础媒体仍可用，首次完成时会短暂重启载入 |
| 下载失败、超时或摘要不匹配 | 检查网络、HTTPS代理和磁盘空间，使用「重新检测获取组件」重试；不能关闭完整性校验 |
| 提示配置路径过长 | 使用较短的本机 `--profile` 路径；部分原生 DLL 受 Windows 路径长度限制，程序会在下载前拒绝该路径 |
| 自定义 Python 不可用 | 修复该受信解释器的依赖，或移除覆盖后退出再启动以使用自动初始化 |
| 清单、版本、架构或工具错误 | 新版托管模式会尝试自动准备；需离线运行则退出后从可信匹配完整包恢复全部材料 |
| 依赖就绪但搜索／下载失败 | 按实际网络、站点变化和媒体限制排查；保留脱敏错误，不以空结果掩盖失败 |
| 取消、退出或替换文件失败 | 查看日志并等待自有进程退出；不要强制覆盖使用中的 EXE/DLL |

发布前需分别记录真实 embedded 依赖、本机桌面、干净 Windows、真实站点和 Linux/Docker 的验证证据。离线样例通过不能替代后几项。维护者装配与归档说明见 [发布流程](release.md)。
