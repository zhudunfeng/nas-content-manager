# Windows 原生视频搜索与下载

Windows 原生桌面入口通过同一个托管获取 worker 调用独立 UAV 私有 Python，提供 Jable、MissAV、Hanime1、黄果、SupJav 五站搜索与视频下载。首批目标为 Windows 11 x64 普通用户。站点业务与限制见 [UAV 使用说明](uav.md)，实际执行范围见 [验证记录](verification.md)。

## 安装与启用

选择含 UAV 的可信完整 Windows 包，核对归档摘要后解压整个目录。包内应有 `nas-collection-worker.exe`、`uav-runtime.json`、`runtimes/uav/`、`tools/yt-dlp.exe`、FFmpeg/ffprobe 及相应许可材料。仅携带 Python 或单独复制 EXE 不能组成完整功能包。

终端无需安装 Python、pip、Go、Node 或 Docker，不修改系统 PATH。不要直接从 ZIP 运行，也不要将开发机 venv 复制成便携运行时。程序目录允许只读，配置、秘密、日志和暂存位于本机 profile；媒体仍使用已授权的业务目录。

1. 启动 `栖库.exe`。新原生 profile 在有 worker 和至少一种随包插件清单时选择托管；已有停用或外接配置继续保留。
2. 需要时在托盘选择「启用本机资源获取」，查看漫画、视频各自的依赖状态；也可用固定 IPC 命令启用或重新检测。
3. 进入“资源收集”，添加 UAV 视频来源，选择站点、搜索、预览，再确认下载。下载、校验、发布与入库继续走原流程。
4. 一个可选插件不可用时，其他已就绪插件和已发布媒体仍可使用。依赖就绪仅表示本机材料可用，不代表站点或网络当前可达。

托盘「本机获取组件」显示总体状态，漫画和视频分别显示「已就绪」「依赖未就绪」或「未配置」；worker 整体失败时显示「未检测」。操作为「启用本机资源获取」「停用本机资源获取」「重新检测获取组件」。关闭浏览器不结束后台服务；升级前从托盘退出，等待本实例 worker、Python、下载器及 FFmpeg 完成回收，再成套替换程序和运行时。不会按名称结束其他 Python 或接管外接服务。

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
| `NAS_COLLECTION_WORKER_UAV_RUNTIME_MANIFEST` | 私有 UAV 清单绝对路径；桌面自动使用程序根清单，普通管理员模式可显式设置 |
| `NAS_COLLECTION_WORKER_UAV_PYTHON` | 高级可信解释器覆盖；托管时必须为绝对路径，显式错误不回退到包内或系统 Python |
| `NAS_COLLECTION_WORKER_YTDLP` | 固定下载器可执行路径；桌面指定包内绝对路径 |
| `NAS_COLLECTION_WORKER_FFMPEG` | 本地媒体校验工具路径；桌面指定包内绝对路径 |

UAV 与 JMComic 各自拥有独立运行时和清单，不能互相覆盖 site-packages。普通控制台部署仍可按 [手工部署说明](uav.md#原生执行端) 使用可信独立环境；Linux/Docker 与外接连接沿用既有配置。

## 数据与排错

默认原生 profile 为 `%LOCALAPPDATA%\Qiku\profiles\native`。托管 worker 使用其中 `collection-managed/`，日志在 `logs/collection-worker.log`。不要把 SQLite、令牌或所有权标记迁到 UNC/网络目录，也不要删除标记来接管未知目录。

| 状态 | 处理 |
| --- | --- |
| 视频未配置／停用 | 确认所用包含 UAV，或按管理员方式配置受信解释器后启用 |
| 清单、版本、架构或摘要错误 | 退出后从可信匹配包恢复整个 UAV 运行时；不要在启动时 pip 补包 |
| 缺下载器、下载器版本不匹配或缺 FFmpeg | 恢复固定 yt-dlp 2026.10.26 和材料齐全的包，并保留全部工具及其 DLL；开发缺工具包不等于完整发行包 |
| 依赖就绪但搜索／下载失败 | 按实际网络、站点变化和媒体限制排查；保留脱敏错误，不以空结果掩盖失败 |
| 取消、退出或替换文件失败 | 查看日志并等待自有进程退出；不要强制覆盖使用中的 EXE/DLL |

发布前需分别记录真实 embedded 依赖、本机桌面、干净 Windows、真实站点和 Linux/Docker 的验证证据。离线样例通过不能替代后几项。维护者装配与归档说明见 [发布流程](release.md)。
