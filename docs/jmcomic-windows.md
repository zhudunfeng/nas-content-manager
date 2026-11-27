# Windows 原生漫画功能

Windows 原生入口可托管独立获取 worker，由 worker 调用随包或自动初始化的私有 Python。主应用、任务、发布门禁和漫画阅读器仍共用原实现。第一验收目标为 Windows 11 x64 普通用户；其他版本、ARM64、干净机器和真实站点的实际执行证据以 [验证记录](verification.md) 为准。

## 选择安装包

- **新版 Windows x64 基础媒体包**：提供媒体功能与匹配的 worker 入口；明确启用托管 JMComic 后，缺失的私有运行时会自动联网准备。真实探测通过前不会显示漫画已就绪；也可连接管理员配置的外接 worker。
- **含 JMComic 的完整漫画包**：程序旁有 `nas-collection-worker.exe`、`jmcomic-runtime.json`、`runtimes/jmcomic/` 和对应许可。Python 与依赖已在构建端装配，终端不执行 pip、不修改系统 PATH，也不使用全局 Python。
- **开发包**：文件名和发行元数据明确标明未带齐媒体工具；即使另有漫画运行时，也不能当作完整离线媒体发行包。

解压后保持整个程序目录结构。不要直接从 ZIP 运行，不要只复制 Python EXE 或把开发虚拟环境复制到另一台电脑充当运行包。默认 profile 位于程序目录下的 `qiku-data`，因此该目录需要可写；若程序目录只读，使用 `--profile` 指定可写位置。配置、下载、检查点、秘密与日志保存在 profile 中。

## 程序同级配置

在 `栖库.exe` 同级 UTF-8 `.env` 中配置：

```dotenv
# 选择本机托管获取，明确覆盖已保存的停用或外接模式。
NAS_DESKTOP_COLLECTION_MODE=managed
# 启用漫画插件；缺少或校验不通过的依赖会自动联网准备。
NAS_COLLECTION_WORKER_JMCOMIC_ENABLED=true
```

程序优先使用有效随包运行时，其次复用当前 profile 已验证的运行时；都不可用时，按锁定官方来源、版本、长度和 SHA256 准备 Python 与漫画依赖。仅启用漫画不会安装 UAV、yt-dlp 或 FFmpeg。需要首次就能离线使用时仍选择含漫画运行时的完整包。终端不执行 pip、不修改系统 Python 或 PATH。

高级可信 Python 覆盖使用 `NAS_COLLECTION_WORKER_JMCOMIC_PYTHON`，保持绝对路径；显式配置后由管理员维护依赖，无效时不自动安装或回退。每个参数也可继续用进程环境变量配置，环境变量优先于 `.env`；命令行仍最高。修改后从托盘退出再启动，重检不重新读文件。语法、示例与范围见 [Windows 配置说明](windows.md#程序同级-env-配置)。

## 启用与使用

1. 解压可信新版 Windows x64 包，核对发行摘要，按上节配置开启漫画，然后运行 `栖库.exe`。
2. 新原生 profile 在存在 worker 且有随包插件清单，或明确开启漫画／视频时选择本机托管；已有停用／外接选择及显式外接配置继续保留，除非明确覆盖模式。旧 v1 profile 升级后不会自动接管获取服务，可主动从托盘选择「启用本机资源获取」。漫画和视频的就绪状态分别显示。
3. 到“资源收集”检查 JMComic 能力，添加或启用漫画来源，再进行搜索、选章、预览与确认下载；具体业务流程见 [JMComic 使用说明](jmcomic.md)。依赖就绪不代表站点当前可达。
4. 遇到错误先看中文原因和托盘「打开日志目录」，修复运行时或配置后可使用「重新检测获取组件」。重新检测会有序重启当前原生服务，请先保存当前操作。
5. 「停用本机资源获取」阻止新的本机获取。已有任务、已发布漫画和阅读进度保留；停止或重检不会自动重发结果不确定的下载。

首次准备显示中文阶段和当前材料的真实字节进度，基础媒体服务仍可用。漫画和视频都需准备时按顺序处理，一项普通下载失败不阻断另一项；本轮完成后至多进行一次自动有序重启以载入已成功准备的依赖，播放或下载中的连接可能中断。初始化完成还须通过真实 worker 能力探测才算就绪。失败不会无限重试，修复网络／代理后选择「重新检测获取组件」或下次启动重试。停用、重检和退出会等待取消与自有资源清理。

关闭浏览器不会结束后台服务。退出使用托盘「退出栖库」，等待本实例主服务、worker 和 Python 子进程清理结束，再替换程序或运行时。程序不会按进程名关闭其他 Python。

## 模式与配置

| 参数／字段 | 中文解释 |
| --- | --- |
| `desktop.json.collectionMode` | `disabled` 停用本机获取；`managed` 托管本机 worker；`external` 使用管理员配置的外接连接 |
| `--collection-mode` | 启动时明确选择上述三种模式，优先于环境和保存配置；正常启动后保存，运行中通过固定 IPC 或托盘有序切换 |
| `NAS_DESKTOP_COLLECTION_MODE` | 桌面原生获取模式的环境配置；只作用于当前进程及子进程，不修改用户或系统环境 |
| `NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST` | 管理员或桌面传递的运行时清单绝对路径；桌面固定选择有效随包或 profile 已发布清单，失败不回退到任意 PATH Python |
| `NAS_COLLECTION_WORKER_JMCOMIC_PYTHON` | 高级管理员显式解释器覆盖；托管时必须为可信绝对路径，无效不回退；普通用户无需设置 |
| `NAS_COLLECTION_WORKER_JMCOMIC_ENABLED` | worker 的漫画插件开关；原生手工模式默认关闭，Windows x64 托管明确启用且无自定义 Python 时允许自动准备依赖；false 禁止自动准备 |

现有 v1 桌面配置显式迁移到 v2，并保留 `desktop.v1.json`。实例身份、业务目录和原媒体根保持原值；桌面 schema 版本不是数据库版本。运行中不要直接编辑配置。回退旧程序前，应恢复与旧程序兼容的配置和数据库备份。

PowerShell 示例，程序和 profile 路径按实际情况修改：

```powershell
# 尚未运行时，以本机托管模式启动；缺运行时须先明确配置漫画启用开关。
& '.\栖库.exe' --collection-mode managed
# 对正在运行的同一profile主动启用、停用或重新检测。
& '.\栖库.exe' --control collection-enable
& '.\栖库.exe' --control collection-disable
& '.\栖库.exe' --control collection-check
# 查看同一实例状态或正常退出；自定义profile时各命令都传相同--profile。
& '.\栖库.exe' --control status
& '.\栖库.exe' --control quit
```

GUI 子系统 EXE 不保证在当前终端显示输出，诊断同时看 profile 日志。外接模式沿用成组的 `NAS_COLLECTION_WORKER_URL`、`NAS_COLLECTION_WORKER_TOKEN_FILE`、`NAS_COLLECTION_READ_ROOT` 和必要的 HTTP 许可，不会被桌面自动覆盖。外接 worker 不归本机托管进程所有，退出桌面不会代停它。

管理员可对 worker 单独运行本地依赖诊断：

```powershell
# 已明确设置可信私有运行时清单或绝对Python路径时，仅检查本地依赖。
$env:NAS_COLLECTION_WORKER_JMCOMIC_ENABLED = 'true'
$env:NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST = (Resolve-Path '.\jmcomic-runtime.json').Path
& '.\nas-collection-worker.exe' --check-jmcomic
```

以上环境只在当前 PowerShell 进程生效。诊断不需要 worker 令牌或数据目录，不监听、不恢复任务、不访问站点；启用探测时仅创建并清理专用临时目录。退出码 `0` 表示本地依赖可用，`2` 表示插件关闭或未就绪，`1` 表示诊断本身失败。

## 数据位置与故障处理

默认原生 profile 为 `栖库.exe` 同级的 `qiku-data\profiles\native`；托管实例使用其中 `collection-managed/` 的自有数据和秘密目录，漫画自动依赖独立位于 `collection-managed/jm-bootstrap/`，worker 日志位于 `logs/collection-worker.log`。程序目录只读时请使用 `--profile` 指定可写目录。不要把 SQLite、检查点或令牌放在 UNC／映射网络盘。不要删除或复制实例所有权标记来强行接管未知目录。

| 现象 | 处理方式 |
| --- | --- |
| 未启用 | 在托盘主动启用，或检查当前是否应使用外接模式 |
| 运行时缺失、损坏或架构／版本不匹配 | 新版 x64 托管明确启用后自动准备；失败时检查中文阶段和网络，再重新检测；离线环境使用可信同版本完整漫画包 |
| 自动初始化失败或 profile 路径过长 | 查看日志并修复网络、空间或权限后重新检测；原生 DLL 路径预算不足时使用更短的 `--profile`，不要只复制 DLL 或终端 pip 补包 |
| worker 启动失败 | 查看日志，检查程序入口、profile 所有权、本地路径与权限；基础媒体仍应可用 |
| 外接共享根不一致 | 两端必须访问同一数据实体，不能仅填写相同字符串路径；保留原随机探针校验 |
| 依赖就绪但搜索失败 | 检查网络、站点可用性和区域限制；保留真实错误，不把失败当空结果 |
| 退出或清理报错 | 保留日志并确认本实例已停止后再更新，不强行覆盖仍在使用的 DLL |

运行时升级必须与 worker 和主应用一起核验。升级保留 profile、任务编号和检查点；取消／未完成内容不能发布，恢复仍须重新核对页面与预览身份。

## 构建端装配

以下只用于维护者，不是终端安装步骤。所有命令在仓库根运行：

```powershell
# 下载固定官方嵌入式Python及锁定wheel，装配至全新输出目录。
python -B scripts/project_env.py -- python -B scripts/jmcomic_runtime.py assemble --output .runtime/jmcomic-runtime
# 校验所有清单成员，并实际探测私有解释器及固定依赖，不请求漫画站点。
python -B scripts/project_env.py -- python -B scripts/jmcomic_runtime.py verify .runtime/jmcomic-runtime/jmcomic-runtime.json --probe
# 从已严格验证的漫画库存生成固定自动初始化资源；--check 可逐字节验证可重现性。
python -B scripts/project_env.py -- python -B scripts/jmcomic_bootstrap.py --runtime-manifest .runtime/jmcomic-runtime/jmcomic-runtime.json
# FFmpeg材料单独审核；显式加入漫画运行时后构建Windows包。
python -B scripts/project_env.py -- python -B scripts/release.py --target windows-amd64 --tools-manifest 'D:\release-inputs\windows-tools.json' --jmcomic-runtime-manifest .runtime/jmcomic-runtime/jmcomic-runtime.json
```

装配可显式提供 `--wheelhouse`（已下载的锁定 wheel 目录）、`--python-archive`（官方 Python ZIP）和 `--offline`（禁止联网）；精确参数以脚本 `--help` 为准。清单记录目标架构、Python／JMComic 版本、依赖锁摘要、来源、许可和每文件摘要。Python 补丁版本、wheel ABI、证书及 DLL 必须配套，架构头或摘要通过不等于干净系统运行通过。

本机继承管道仅传版本化的启动／就绪／停止消息，普通日志走 stderr；worker 真实绑定 `127.0.0.1:0`，桌面验证关联值、认证能力与共享根后才连接。浏览器继续不能提交 Python 参数、脚本、YAML、Cookie 或输出路径。
