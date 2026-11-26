# UAV／SupJav Windows 原生兼容修改方案

日期：2026-11-26。交付类型：技术方案，尚未实施。首批验收目标：Windows 11 x64 普通用户、解压后双击桌面程序。

实施更新：2026-11-26用户已确认“按计划进行实现”。本文以下保留原方案快照，实际实施范围和验收状态以最新 `prd.md` 及验证记录为准。

## 结论与范围

保留已有五站业务，通过现有桌面托管 worker 增加 UAV 私有 Python 与下载器分发，补齐自动配置、独立能力状态、Windows 路径和生命周期验证。用户机器无需安装 Python/pip、修改 PATH 或取得管理员权限。

Windows 手工 venv 和外接 worker 继续可用；Linux/Docker 不改默认部署方式。本轮不接入新站点、不安装依赖、不修改生产代码、不生成发布包。

## 方案比较

| 方案 | 适用对象 | 优点 | 代价与结论 |
| --- | --- | --- | --- |
| 手工安装独立 venv 和工具 | 管理员、开发调试 | 当前路径已有文档；改动少 | 安装与排错仍由用户负责；作为过渡方案保留 |
| 私有 embedded Python + 桌面托管 worker | 普通 Windows 桌面用户 | 隔离、可校验、可离线交付，复用 JMComic 基础 | 增加构建装配和包体；推荐主方案 |
| Docker Desktop／外接 worker | 已有容器或 NAS 执行端的用户 | 复用已有部署链路 | 需要额外基础设施；不作为 Windows 原生双击使用的前置条件 |

## 推荐包结构

```text
栖库.exe
nas.exe
nas-collection-worker.exe
uav-runtime.json
jmcomic-runtime.json                 # 含漫画插件时提供
runtimes/
  uav/                              # 独立 Python、标准库、无头依赖、CA
  jmcomic/                          # 可选；不与 UAV 共享 site-packages
tools/
  yt-dlp.exe                        # 固定版本、官方 x64 产物
  ffmpeg.exe                        # 复用现有受控工具分发
  ffprobe.exe
licenses/
  uav/                              # Python、UAV、依赖许可和来源
  yt-dlp/                           # 下载器清单、全部许可和源码材料
  ffmpeg/
docs/uav.md
docs/uav-windows.md
release.json
checksums.sha256
```

程序目录可只读。worker 状态、令牌、下载暂存、日志放在现有 profile 的自有目录；实际媒体沿用已授权业务目录。路径均由程序根/profile 计算，不依赖启动工作目录。

## 文件级改动

### 1. UAV 私有运行时

- 新增 `internal/uavruntime/`、`scripts/uav_runtime.py` 与配套契约数据，仿照 `internal/jmcomicruntime/` 和 `scripts/jmcomic_runtime.py` 的模式，不把 JMComic 名称替换成通用大框架。
- 清单固定目标架构、Python 补丁版本、UAV 版本/commit、依赖锁摘要、构建输入来源和 SHA-256，以及逐文件大小、摘要、角色、组件。
- 优先验证仓库现有 CPython 3.14.4 候选；只有选定架构的 wheel 闭包、DLL 导入、证书和真实 embedded probe 通过后才确认版本。保留 UAV 3.8.4 与现有上游 commit，不追踪浮动版本。
- 构建时验证源码归档，用锁定构建工具离线生成 UAV wheel，再与固定依赖装配；构建工具不带到终端运行时。支持准备 wheelhouse 后离线装配。
- 特别保留 `uav_bridge.py` 现有 `direct_url.json` 源码 provenance 检查：不能仅解压 UAV wheel。装配器须先证明固定源归档身份，再保留/规范生成对应的官方来源和源摘要，另记派生 wheel 摘要；去除构建机绝对路径，并同步 RECORD 和最终逐文件清单。缺少或错误 provenance 必须使真实 probe 失败，不能删除或放宽检查。
- 严格检查 `._pth`、Python ZIP、原生 PYD/DLL、CA 文件、路径穿越、大小写别名、重解析点、缺失/额外运行文件。运行时不执行 pip，不导入用户 site，不复制开发 venv。
- 新增持久化集成验证入口，例如 `scripts/verify_uav_runtime.py`；不为本轮生成临时单元测试类。

### 2. worker 配置、启动和诊断

修改 `internal/collectionworker/config.go`、`uav.go`、`service.go`、`cmd/nas-collection-worker/main.go`：

- 补齐下表的 manifest 配置和显式解释器标记；解析优先级为“显式解释器 > 包内清单 > 普通控制台兼容默认”。显式配置错误时停止该插件，不静默回退；托管模式禁止查找系统 Python。
- 为 UAV 保存具体不可用原因，区分缺解释器、清单/架构/摘要错误、依赖导入/版本错误、下载器缺失、FFmpeg 缺失；其他能力照常服务。
- 桥接固定增加 `-X utf8`，保持 `-I -B`、现有 JSON 协议、超时、输出上限、代理和脱敏边界。
- 新增 `--check-uav`：只在自有临时目录验证运行时、固定 bridge probe、下载器版本和 FFmpeg 本地执行；不监听端口、不恢复任务、不访问站点。建议退出码 0=依赖就绪、2=未就绪或未启用、1=诊断自身错误。
- 诊断与桌面使用同一依赖解析逻辑，避免独立实现两套判断；日志记录版本、阶段和脱敏原因，不记录搜索词、媒体 URL、令牌或完整工具 stderr。

| 配置 | 中文解释与行为 |
| --- | --- |
| `NAS_COLLECTION_WORKER_UAV_ENABLED` | 现有插件启用开关；普通控制台默认 false，桌面根据显式配置或随包插件材料受控启用 |
| `NAS_COLLECTION_WORKER_UAV_PYTHON` | 现有受信解释器覆盖；桌面托管仅接受绝对路径；显式错误不回退 |
| `NAS_COLLECTION_WORKER_UAV_RUNTIME_MANIFEST` | 新增，UAV 私有运行时清单绝对路径；桌面自动填入程序根的 `uav-runtime.json`，普通控制台默认空 |
| `UAVPythonExplicit` | 新增内部布尔标记，用于区别用户覆盖与平台默认值，不新增用户输入入口 |
| `NAS_COLLECTION_WORKER_YTDLP` | 现有下载器路径；桌面注入程序根下固定 `tools/yt-dlp.exe` 绝对路径 |
| `NAS_COLLECTION_WORKER_FFMPEG` | 现有校验工具路径；桌面注入固定 `tools/ffmpeg.exe` 绝对路径 |
| `collectionMode` | 复用现有 disabled/managed/external；不增加桌面配置版本或数据库迁移 |

### 3. 桌面托管和多插件状态

修改 `internal/desktop/collection_managed.go`、`collection_config.go`、`collection_controller.go`、`launcher.go`、`shell_windows.go`：

- 仍启动同一个 worker，沿用随机回环端口、继承管道、实例验证、Bearer 和共享根挑战。
- 构建受控子进程环境，分别注入随包 JMComic/UAV 清单、启用状态及绝对工具路径；对可信的显式 UAV 解释器覆盖采用与 JMComic 相同规则，不全量继承环境。
- 新 profile 在 worker 和至少一个受支持插件清单存在时选择 managed；存在清单只是默认模式选择，不能代替运行时校验。保留旧配置 disabled/external 和外接配置优先级，升级不静默启用旧用户的插件。
- 将单一 `capability` 改为按 kind 保存的能力集合，复用已有 `/v1/capabilities`；托盘显示“本机获取组件”，分别显示漫画和视频状态。
- 保留既有服务状态枚举；至少一个预期启用插件可用时 ready，同时显示另一插件的失败原因；全部预期插件不可用时 unavailable。worker 进程/握手失败才作为 failed。不要把未随包的可选插件显示为损坏。
- 不重复实现下载队列、播放器、入库、鉴权或新视频 worker；网页已有能力数据则直接复用，仅在实际信息不足时补显示。

### 4. 明确分发下载器

- 首选固定版本官方 `yt-dlp.exe`，与当前“可执行路径 + argv”调用方式吻合；以现有锁定版本 2026.10.26 为候选，不能把 wheel 摘要用作 EXE 摘要。
- 新增独立下载器装配/校验清单，记录官方版本、EXE SHA-256、目标架构和许可/源码材料；可用独立脚本 `scripts/ytdlp_runtime.py` 实现，不挤入仅管 FFmpeg 的旧清单。
- 官方 PyInstaller EXE 的组合许可与 wheel 不同；必须按该版本官方说明整理完整材料并完成分发审查，不能只复制一份 Unlicense。完整材料准备不齐时不得作为正式包发布。
- 若最终采用私有 Python 的 `-m yt_dlp` 备选路线，则需新增结构化“可执行路径 + 固定前缀参数”类型，并同步修改 web/UAV/版本探测及工具失败识别；禁止拼成 shell 字符串，禁止复制含开发机绝对路径的 console launcher。两种路线首版只实施一种。
- FFmpeg 继续只处理本地媒体；Windows 兼容不放开联网 FFmpeg、任意站点或浏览器插件执行。

### 5. 发布、升级与文档

- `scripts/release.py` 增加有中文说明的 `--uav-runtime-manifest`、`--ytdlp-manifest`，`scripts/release-windows.ps1` 透传；只对 windows-amd64 接受首批运行时。新增参数均为构建输入，不接受网页请求配置。
- `release.json` 演进为 schema 3，新增严格的 `uavRuntime` 和 `ytDlp` 可空摘要字段；保留原 `tools`、`jmcomicRuntime` 语义。普通包字段为 null，完整 UAV 包必须同时携带已验收 UAV、下载器和媒体工具。
- `scripts/verify_release.py` 同步支持旧 schema 1/2、新 schema 3，对实际归档成员、清单绑定、摘要、架构、路径别名及额外文件执行验证；不得把新字段直接加入严格 schema 2。
- 支持基础包、JMComic 包、UAV 包、两插件组合包；包名按所含插件确定稳定后缀，开发缺工具标记继续独立保留。
- 复制 `docs/uav.md`、新增 `docs/uav-windows.md`，更新 `docs/windows.md`、`docs/release.md`、第三方声明及相关 spec；记录支持矩阵和实际验证证据。
- 更新前退出本实例并等待 worker/Python/yt-dlp/FFmpeg 的 Job 和句柄释放。采用新目录解压、验证后启动，保留旧包回退；不在运行中覆盖运行时、不结束其他 Python、不重放结果未知的下载。

## 分阶段交付与验收门槛

| 阶段 | 交付 | 进入下一阶段的门槛 |
| --- | --- | --- |
| A：运行时和本地诊断 | UAV 装配器/加载器、固定下载器材料、配置和 `--check-uav` | 实际 embedded 导入及本地离线 probe 通过；损坏、错架构、显式无效覆盖均按预期失败 |
| B：桌面集成 | 单 worker、多能力、托盘、受控环境和新 profile 默认 | UAV-only、JMComic-only、组合包、旧 disabled、external 均独立正确；取消和退出释放进程文件 |
| C：发行与回归 | schema 3、完整归档校验、用户文档和许可 | 干净机无需 Python/admin；中文空格/只读路径通过；Linux/Docker、漫画及原资源收集回归 |
| D：站点验收 | 五站小样本搜索、预览、下载、播放和入库记录 | 对每站分别记录真实网络结果；失败要区分网络、站点变化和本地缺陷，不能用 fixture 代替真实验证 |

## 验收矩阵

| 维度 | 必测情形 | 通过标准 |
| --- | --- | --- |
| 机器 | Windows 11 x64 干净用户环境，无 Python/pip/yt-dlp，普通用户 | 解压双击；无需写系统环境或临时安装依赖；本地依赖诊断正确 |
| 路径 | 中文、空格、从不同 CWD 启动、只读程序目录 | JSON 不乱码；工具定位稳定；所有可写状态位于自有 profile/任务目录 |
| 运行时 | 缺 DLL/PYD/CA、错摘要/版本/架构、重解析/别名、额外文件 | UAV 独立不可用，给出中文脱敏原因；漫画和已发布媒体仍可使用 |
| 配置 | 有效/无效显式 Python、无清单、UAV-only、组合包、旧 profile、external | 优先级正确；无静默 PATH 回退或接管外部进程 |
| 流程 | 五站搜索→预览→提交→下载→校验→发布→播放/入库 | 沿用原身份、鉴权、限额、任务状态和入库重试语义 |
| 媒体 | 直链、允许的普通/AES HLS、SupJav 候选线路及伪装 TS；缺片/非法媒体 | 合法小样本完整通过，非法或不完整产物不发布；不扩大现有媒体支持范围 |
| 生命周期 | 下载取消、超时、普通退出、父进程异常、同 profile 重启、升级/回退 | 本实例子进程和句柄释放；不杀其他进程、不重放不确定操作 |
| 发布与回归 | 旧/新清单、归档缺件/改件、JMComic、Linux amd64/arm64 与 Docker | 旧验证规则保留；错误包拒绝；跨平台编译与目标平台实跑分别记录 |

开发/验证命令通过 `python -B scripts/project_env.py -- ...`；缓存和临时目录限定 `.runtime/`。PowerShell 脚本兼容 5.1/7，含中文时遵循仓库编码规范。后续生成方法、字段、配置均写中文注释和类型，关键业务写中文脱敏日志；临时单元测试完成后删除，集成验证 harness 保留。

## 本轮边界与研究依据

- 已完成代码和分发路线调查、方案与验收设计；未运行实际 embedded 装配、站点搜索下载、干净机、Docker 或 ARM64 验收。
- Windows 10、Windows ARM64、长路径/UNC 网络 profile 不纳入首批支持承诺；遇到现有目录约束应明确报错，后续可单独立项验证。
- 方案推荐不等于用户已授权实施；Trellis 保持 planning。
- 仓库证据：`research/windows-codebase.md`。
- 官方材料与方案取舍：`research/windows-runtime-options.md`。
