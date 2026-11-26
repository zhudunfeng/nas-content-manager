# Windows 原生漫画功能

Windows 原生入口可托管独立获取 worker，由 worker 调用随包私有 Python。主应用、任务、发布门禁和漫画阅读器仍共用原实现。第一验收目标为 Windows 11 x64 普通用户；其他版本、ARM64、干净机器和真实站点的实际执行证据以 [验证记录](verification.md) 为准。

## 选择安装包

- **基础媒体包**：提供媒体功能与 worker 入口，不携带 JMComic 私有运行时。漫画获取不会被显示为已就绪，可连接管理员配置的外接 worker。
- **含 JMComic 的完整漫画包**：程序旁有 `nas-collection-worker.exe`、`jmcomic-runtime.json`、`runtimes/jmcomic/` 和对应许可。Python 与依赖已在构建端装配，终端不执行 pip、不修改系统 PATH，也不使用全局 Python。
- **开发包**：文件名和发行元数据明确标明未带齐媒体工具；即使另有漫画运行时，也不能当作完整离线媒体发行包。

解压后保持整个程序目录结构。不要直接从 ZIP 运行，不要只复制 Python EXE 或把开发虚拟环境复制到另一台电脑充当运行包。程序目录可以只读；配置、下载、检查点、秘密与日志保存在本机 profile 中。

## 启用与使用

1. 解压可信完整漫画包，核对发行摘要，然后运行 `栖库.exe`。
2. 新原生 profile 在存在随包 worker 和至少一种插件运行时清单时选择本机托管；已有外接配置优先保留外接模式。旧 v1 profile 升级后不会自动接管获取服务，可主动从托盘选择「启用本机资源获取」。漫画和视频的就绪状态分别显示。
3. 到“资源收集”检查 JMComic 能力，添加或启用漫画来源，再进行搜索、选章、预览与确认下载；具体业务流程见 [JMComic 使用说明](jmcomic.md)。依赖就绪不代表站点当前可达。
4. 遇到错误先看中文原因和托盘「打开日志目录」，修复运行时或配置后可使用「重新检测获取组件」。重新检测会有序重启当前原生服务，请先保存当前操作。
5. 「停用本机资源获取」阻止新的本机获取。已有任务、已发布漫画和阅读进度保留；停止或重检不会自动重发结果不确定的下载。

关闭浏览器不会结束后台服务。退出使用托盘「退出栖库」，等待本实例主服务、worker 和 Python 子进程清理结束，再替换程序或运行时。程序不会按进程名关闭其他 Python。

## 模式与配置

| 参数／字段 | 中文解释 |
| --- | --- |
| `desktop.json.collectionMode` | `disabled` 停用本机获取；`managed` 托管本机 worker；`external` 使用管理员配置的外接连接 |
| `--collection-mode` | 启动时明确选择上述三种模式，优先于环境和保存配置；正常启动后保存，运行中通过固定 IPC 或托盘有序切换 |
| `NAS_DESKTOP_COLLECTION_MODE` | 桌面原生获取模式的环境配置；只作用于当前进程及子进程，不修改用户或系统环境 |
| `NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST` | 管理员或桌面传递的运行时清单绝对路径；托管默认指向程序旁清单，失败不回退到任意 PATH Python |
| `NAS_COLLECTION_WORKER_JMCOMIC_PYTHON` | 高级管理员显式解释器覆盖；托管时必须为可信绝对路径，无效不回退；普通用户无需设置 |
| `NAS_COLLECTION_WORKER_JMCOMIC_ENABLED` | worker 的漫画插件开关；原生手工模式默认关闭，本机托管由桌面显式启用 |

现有 v1 桌面配置显式迁移到 v2，并保留 `desktop.v1.json`。实例身份、业务目录和原媒体根保持原值；桌面 schema 版本不是数据库版本。运行中不要直接编辑配置。回退旧程序前，应恢复与旧程序兼容的配置和数据库备份。

PowerShell 示例，程序和 profile 路径按实际情况修改：

```powershell
# 尚未运行时，以本机托管模式启动；程序目录须有worker及运行时。
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

默认原生 profile 为 `%LOCALAPPDATA%\Qiku\profiles\native`；托管实例使用其中 `collection-managed/` 的自有数据和秘密目录，worker 日志位于 `logs/collection-worker.log`。不要把 SQLite、检查点或令牌放在 UNC／映射网络盘。不要删除或复制实例所有权标记来强行接管未知目录。

| 现象 | 处理方式 |
| --- | --- |
| 未启用 | 在托盘主动启用，或检查当前是否应使用外接模式 |
| 运行时缺失、损坏或架构／版本不匹配 | 退出程序后，从可信同版本完整包恢复整个运行时与清单，再重新检测；不要在终端临时 pip 补包 |
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
# FFmpeg材料单独审核；显式加入漫画运行时后构建Windows包。
python -B scripts/project_env.py -- python -B scripts/release.py --target windows-amd64 --tools-manifest 'D:\release-inputs\windows-tools.json' --jmcomic-runtime-manifest .runtime/jmcomic-runtime/jmcomic-runtime.json
```

装配可显式提供 `--wheelhouse`（已下载的锁定 wheel 目录）、`--python-archive`（官方 Python ZIP）和 `--offline`（禁止联网）；精确参数以脚本 `--help` 为准。清单记录目标架构、Python／JMComic 版本、依赖锁摘要、来源、许可和每文件摘要。Python 补丁版本、wheel ABI、证书及 DLL 必须配套，架构头或摘要通过不等于干净系统运行通过。

本机继承管道仅传版本化的启动／就绪／停止消息，普通日志走 stderr；worker 真实绑定 `127.0.0.1:0`，桌面验证关联值、认证能力与共享根后才连接。浏览器继续不能提交 Python 参数、脚本、YAML、Cookie 或输出路径。
