# Windows 原生 JMComic 实现与主线验收

日期：2026-11-26（Asia/Shanghai）。用户授权“按计划进行实现”，本轮沿用既有高级漫画插件，补齐 Windows 私有依赖、桌面托管与分发能力。所有构建、测试和临时工具均经 `python -B scripts/project_env.py -- ...`，缓存与临时目录限定在仓库 `.runtime/`。

## 交付内容

- 新增 `internal/jmcomicruntime/` 与 `scripts/jmcomic_runtime.py`：构建端装配官方嵌入式 CPython 和锁定 wheel，完整保留本机扩展、标准库、CA、元数据与许可；终端不执行 pip，不依赖全局 Python。
- 新增私有继承管道协议和 worker 运行入口：16 KiB 有界严格 JSON、关联身份、真实回环临时端口、stop／EOF 取消、本地 `--check-jmcomic` 诊断；固定桥接保持 `-I -B -X utf8` 和原业务语义。
- 桌面支持 `disabled|managed|external`，v1→v2 显式迁移与原字节备份，CLI／环境／保存值优先级、完整包新 profile 默认托管、旧外接配置优先保留。提供固定 IPC 和托盘启用／停用／重新检测入口。
- 专属 profile 状态、秘密目录实际 Windows DACL、挂起创建后归入私有 Job、真实后代句柄排空；不按进程名批量终止，不修改全局环境，不自动重发不确定任务。worker 故障只降级可选获取，媒体和已发布漫画保持可用。
- 所有发行目标纳入 worker；可选 Windows amd64 漫画运行时、严格清单与 schema 2 发行摘要，继续兼容旧 schema 1 归档。含漫画运行时与缺媒体工具的标识独立。
- 使用说明在 `docs/jmcomic-windows.md`；同步更新漫画、资源收集、Windows、发行文档与正式后端契约。数据库仍为 schema 6，桌面 JSON schema 为 2。

## 实际私有运行时

清单为 `.runtime/tools/jmcomic-private/jmcomic-runtime.json`，CPython **3.14.4 x64**、JMComic **2.7.12**，620 份成员合计 **50,123,069 字节**。manifest SHA256 为 `90bfc61fb39f652a5bcec534d6bd265c27e3159b2ef7f3ccb1c2aceb1399a66b`。依赖锁采用 LF 规范字节，避免工作区 CRLF 与 Git blob 不一致。

官方 Python ZIP 已对来源摘要核验，但没有执行 Sigstore 发布者身份签名验证。静态清单、架构和文件摘要校验保证受信运行包的一致性，不能替代发布者签名或干净机器兼容验收。

## 统一候选结果

最终候选位于 `.runtime/verify/jmwindows-final-20261126-2/`。**19 条构建／验证命令全部退出 0，统一候选和最终审计通过。**每条命令、参数、耗时与日志由 `commands.json` 保存；`source-before.json` 和 `source-after.json` 绑定 169 份实际发行构建输入，内容无漂移。输入范围为 `cmd/`、`internal/`、`deploy/`、`web/dist/`、Go 依赖声明及两个发行／装配脚本。

| 最终同批检查 | 结果及证据 |
| --- | --- |
| 私有运行时 | 真实清单 620 文件完整校验与隔离依赖 probe 通过；`runtime-verify.log`。 |
| 实际发行归档 | Windows amd64 三入口和私有运行时真实构建；归档严格验证通过，636 份文件，明确为开发包；`release.log`、`release-verify.log`。 |
| 原漫画／高级漫画／UAV | 分别 **66／144／102 项**通过；各自 `result.json`。使用真实最终应用和 worker，上游为离线协议 fixture，不执行真实站点传输。 |
| 基线 | 核心媒体 **44**、资源收集 **27**、发行自检 **91** 项通过；`baseline/run-manifest.json`。仅选中这三组，其余基线组未运行。核心媒体采用真实 FFmpeg 和生成样本；发行自检结构夹具与实际归档校验分开记账。 |
| 原桌面 | **17 类检查**通过，embedded 网页模式，含中文／空格／分号路径、实际避让端口、IPC、慢流退出、文件释放和同 profile 重启；`desktop-original.log`。 |
| 托管 worker | **20 项**通过，实际二进制 SHA256 与本轮最终 worker 一致；`managed-worker.log` 指向 `.runtime/verify/jmcomic-managed-worker-20261126T004752326847Z-34e700/result.json`。 |
| 托管桌面 | **14 组**通过，`privateRuntime=true`，真实加载映像 **26 个**；`desktop-managed.log`。覆盖私有依赖、嵌套 Job、启动取消、父强杀、独立哨兵、worker 异常退出后媒体健康／主动重检和原位替换。 |
| Go | Go **1.27.4 windows/amd64**；全仓 vet、清理临时测试后的全包编译通过，所有包 `[no test files]`，不计作行为测试；`go-version.log`、`go-vet.log`、`go-test.log`。 |
| 格式与脚本 | 20 份改动 Go 的 gofmt、7 份 Python 的 AST／参数返回类型／中文方法说明、Git 空白检查通过；发布 PowerShell UTF-8 BOM、Windows PowerShell **5.1.19041.6456** 与 PowerShell **7.6.4** 真实解析通过；`source-audit.log`。 |
| Linux 编译兼容 | amd64／arm64 主服务和 worker 共 **4 个**交叉构建通过，未执行；各 `linux-*.log`。桌面 Linux 交编由实施者另行记录。 |
| Trellis | implement **13**、check **12** 条上下文引用全部有效；`task-context.log`。 |
| 最终回读审计 | `final-audit.json` 为 `passed=true`，回读上述数量和实际产物身份，8 份归档／二进制与 169 份输入的字节数、摘要一致；`sourceDrift`、`temporaryGoTests`、`trailingWhitespace`、`liveOwnedProcesses` 均为空。 |

## 可审阅产物

验证用开发包已解压至 `.runtime/bin/jmwindows-final-20261126-2/`，入口为 `栖库.exe`；运行时和许可必须随整个目录保留。未自动启动用户 profile。归档为 `.runtime/releases/jmwindows-final-20261126-2/qiku-0.1.0-windows-amd64-jmcomic-development.zip`，**41,542,637 字节**，SHA256 为 `8f9d826a66d55324ad91c87985b544c1bae204e6694e120ce1ba48119761c184`。

| Windows 文件 | 字节数 | SHA256 |
| --- | ---: | --- |
| `nas.exe` | 15,579,136 | `e58e84b3eda0b5e681783b60538711189da0fed3015e590ff501b01a29e2efd1` |
| `nas-collection-worker.exe` | 8,658,944 | `75be26e53d45a31d7b378d1248a76793ca38e59535ec9c365ff525cacd54148c` |
| `栖库.exe` | 15,898,112 | `b1d5bde8253d3da82061dab75e9d94e9928187548f5ef52e758e93079425fc85` |

其余 Linux 产物的完整身份见同证据根 `artifacts.json`。构建输入、程序与集成结果均为本轮最终候选；不复用早期候选作为最终二进制。

## 专项与独立检查

这些结果与统一候选分开记账，重复执行的相同检查不累计为额外覆盖数量。

| 检查 | 实际结果与范围 |
| --- | --- |
| 私有运行时交叉验证 | 49 项通过；Python 与实际 Go 加载器对完整性、版本、来源、许可、PE、`_pth`、Windows 设备别名、实际 junction、污染环境及中文／空格路径交叉判定。证据 `.runtime/verify/jmcomic-runtime-result-8e11771ee46e490dabb4814d1320c2e4/result.json`。 |
| 离线重建 | 禁用 `urlopen`，使用缓存 ZIP／wheel 重建并真实 probe 通过；manifest 与首次装配完全一致。 |
| 私有 Python 真实桥接 | 22 个检查点通过；执行真实固定包、客户端、下载器、图像解码、选章、进度、可信页面复用及拒绝边界，仅替换站点传输。`realNetworkCalls=0`，证据 `.runtime/verify/jmcomic-real-bridge-5bcfd09caa494461aa124ff01bc0eb1f/report.json`。 |
| Worker 实现阶段 | 私有运行时 20 项；含实际继承管道、认证、stop／EOF、诊断、显式解释器不回退、污染环境和中断恢复。慢下载使用 Go 协议替身，不是实际 Python 公网下载。 |
| 桌面实现阶段 | 私有运行时 14 组；实际 PSAPI 观察 26 个已加载映像，含 Python、Pillow、curl、AES、SSL；验证占用正例、启动取消、父强杀、嵌套 Job、独立哨兵存活、映像独占／改名／原位覆盖、摘要与同 profile 重启。 |
| 独立 Trellis 复核 | 修正失败退出仍报成功日志、CLI 帮助漏项，并同步补齐运行时设备别名；实际初始化失败日志回归连续 30 次通过，全量 Go vet 与包编译通过。临时测试源码已删；报告 `quality-check.md`。 |
| 前端 | 本轮未改前端源码；主线已运行 Vue 严格类型检查和 Vite 生产构建，通过并生成发行使用的 `web/dist`。 |

专项细节见本目录 `runtime-implementation.md`、`worker-implementation.md`、`desktop-implementation.md` 和 `quality-check.md`。桌面退出测试使用真实私有依赖与可信进程 fixture，证明进程和映像生命周期；不能替代真实站点下载中断验收。

## 早期候选与失败保留

- 第一候选 `.runtime/verify/jmwindows-final-20261126/` 的各条命令退出 0，但与最终独立审查修改重叠，收尾源码快照检测到 `internal/collectionworker/managed.go`、`internal/desktop/collection_managed.go` 和 `internal/desktop/config.go` 漂移，整轮退出 1。已保存 `candidate-disposition.json`，不将该轮称为最终候选通过；稳定后另建全新第二候选，不覆盖早期证据。
- worker 的旧契约二进制与 LF 依赖摘要曾不一致；重新编译后私有清单验证通过，没有放宽完整性门禁或回退系统 Python。裸 gofmt 路径失败和该诊断失败分别保留在实施报告。
- `go test -race` 因 CGO 禁用而未执行，不能计为通过。临时测试清理后的 `go test ./...` 只计包编译和类型检查。

## 交付边界

- 仅本机现有 Windows x64 开发环境；未取得干净 Windows 11 普通用户、无预装全局工具虚拟机验收。其他 Windows 版本和 ARM64 执行也未验收。
- 未访问真实漫画站点，搜索／下载协议 fixture、真实依赖离线桥接与公网传输分别记账。本地能力 ready 仅表示依赖就绪。
- Linux amd64／arm64 仅交叉编译，未执行 Linux 产物；未启动 Docker 或连接物理 NAS。
- 当前成品含完整漫画运行时，但使用 `--development-without-tools`，未审核／纳入 FFmpeg 完整发行材料，不能称为完整离线媒体发行版。
- 没有物理点击托盘菜单；本轮验证固定 IPC 路由和菜单接线，原桌面退出／媒体流程另由统一回归覆盖。稀疏大文件测试只证明偏移和退出协议，不代表真实存储吞吐。
- 未提交、部署、归档任务、修改用户实例或全局环境；用户审阅后自行提交。最终门禁完成，任务进入 review；旧高级漫画任务仍为 review。
