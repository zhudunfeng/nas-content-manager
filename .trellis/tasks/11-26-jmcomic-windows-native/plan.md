# Windows 原生 JMComic 兼容修改方案

日期：2026-11-26。性质：建议方案；本轮没有修改业务代码、安装运行时或启动真实服务。

## 1. 目标与当前判断

以 Windows 11 x64 普通用户为第一验收目标，使完整桌面包无需 Go、Node、Docker 或全局 Python，即可双击使用 JMComic 搜索、选章、批量获取、逐页进度、失败页复用及原漫画阅读。其他 Windows 版本／ARM64 在取得独立构建和运行证据前不承诺。

当前已有 Windows 主服务／worker、固定 JMComic 真实依赖离线验证、Job 进程回收、绝对工具路径和隔离 JSON 桥接。主要缺口是**完整分发、桌面托管及干净机器验收**，不需要重写搜索下载业务。依据见 `research/codebase-windows.md`。

## 2. 方案比较与推荐

| 方式 | 修改与优点 | 代价／适用范围 |
| --- | --- | --- |
| 手工 Python／venv 加配置脚本 | 复用现有协议，先提供本机诊断、启动／停止脚本，实施量最小 | 依赖用户准备解释器、网络和工具，不是免安装完整包；适合开发者／管理员 |
| 私有 CPython＋固定依赖＋桌面托管独立 worker（推荐） | 保留固定 Python 桥接和 worker 隔离，运行时可离线打包，普通用户无需全局 Python | 需新增运行时清单、生命周期、打包及干净系统验收；推荐产品交付路线 |
| 冻结 Python 桥接 EXE | 终端不见解释器，桥接仍按 JSON 调用 | 必须适配固定脚本的执行方式、动态导入／包元数据／数据／DLL收集，额外引入构建链；单文件临时解包和文件占用也需单独评估，暂不作为首选 |

推荐保持 `栖库.exe → nas-collection-worker.exe → 私有 python.exe`，Python 请求仍由现有 worker 生成；主应用和浏览器不直接执行 Python。官方运行时依据和冻结方案取舍见 `research/windows-python-runtime.md`。

## 3. 具体改动

### 3.1 私有运行时与可信工具解析

- 在构建机选择并精确锁定一个通过依赖 wheel、TLS 和图片验证的 Windows x64 CPython 补丁版本；不把“机器当前可运行的 venv”直接复制当发行运行时，也不凭空升级 jmcomic。
- 构建机装配私有运行时与当前哈希锁定依赖，终端不执行 pip、不修改 PATH、不触碰用户 Python。若使用 embeddable 包，由构建流程设置受控 `_pth`／包搜索路径，不能将它当成自带 pip 的安装器。
- 分发完整 `.pyd`／DLL、CA 数据、包数据及 `.dist-info`；包括 Pillow、curl_cffi／cffi、加密等实际闭包，不能只打包 `.py`。桥接用 `importlib.metadata.version` 校验依赖，必须保留发行元数据；对 VC 运行库等前提做干净目标验证，不能被开发机预装组件掩盖。
- `_pth` 仅声明包内标准库、扩展与 `Lib/site-packages`，优先不启用 site；如锁定依赖确需 `.pth`，单独核验并白名单，不加入用户目录或工作目录，不通过 `PYTHONPATH` 修补导入。
- 增加插件运行时清单：格式版本、目标架构、CPython 和 JMComic 精确版本、受控相对路径、每文件摘要、来源和许可证。只读程序目录与可写状态目录分离。
- 解释器优先级：管理员显式绝对配置 → 托管模式中清单声明的随包解释器 → 原控制台模式现有固定工具名兼容。显式配置失败不得静默改用另一解释器；托管模式不回退到 CWD 或系统 Python／商店别名。
- 保留 `-I -B`，建议显式加入 `-X utf8` 并验证错误流；`-I` 会忽略 `PYTHONIOENCODING`，不能只依赖现有环境设置。保留当前 ASCII JSON 成功协议、无 shell 和受控代理。通过真实编码反例决定是否还需调整固定桥接，避免重写已经可靠的中文路径处理。
- 当前固定桥接通过 `-c` 传入；将完整命令行长度纳入长路径验收，不无界追加配置。若将来改为短可信引导或包内脚本，继续验证固定来源、摘要和目录所有权，并保持业务参数仅从 JSON stdin 输入。

建议目录仅描述布局，不固定未经验证的版本号：

```text
程序目录/
  栖库.exe
  nas.exe
  nas-collection-worker.exe
  runtimes/jmcomic/<runtime-version>/...
  licenses/jmcomic/...
  jmcomic-runtime.json
```

### 3.2 桌面托管 worker 与配置兼容

- 新增独立的原生 worker 管理模块，由桌面启动器创建隐藏子进程，沿用 worker HTTP／认证／发布协议。
- 明确 `disabled / managed / external` 三种模式；旧 profile 与管理员已有 `NAS_COLLECTION_*` 保持外接语义，不被自动接管或覆盖。完整包新 profile 可在首次配置选择托管模式；基础包默认不提供虚假的可用状态。
- 现有 `desktop.json` 为严格 schema v1；新增模式按 v1→v2 显式迁移、校验和原子保存，保留原实例身份与目录。不将旧 schema 记录缺少新字段视为损坏。数据库 schema 6 无需为本次托管变化升级。
- 托管 worker 绑定 `127.0.0.1:0`，建立当前 SID 私有控制通道，真实启动完成后返回地址、实例身份和一次性关联值。父进程再用专用 token 验证 capabilities／共享根；不得先探测空闲端口再释放，不解析普通日志判断就绪。
- 主应用只在本次配置快照中整组注入 WorkerURL、TokenFile、ReadRoot 和本机 HTTP 许可；不调用全局 `os.Setenv`，不把随包 worker 与外接 token／数据根混用。
- worker／运行时初始化失败只使托管漫画功能不可用，桌面媒体服务和已发布漫画继续使用。运行期间子进程异常退出显示降级状态，保留持久任务，不无限重启或自动重发不确定下载。

### 3.3 生命周期与 Windows 权限

- 复用现有 Python 命令 Job 所有权机制；新增桌面父级对托管 worker 的所有权，实测嵌套 Job，归组失败不得降级为无所有权运行。
- 私有控制通道支持正常 stop 和父通道断开。不能仅依赖隐藏 GUI 子进程收到 Ctrl+C；正常退出先关闭新任务准入、协同取消并排空 app／worker，再等待 Python 后代和文件句柄释放；超时由本实例 Job 兜底并保留“不确定”事实。
- 托盘退出／重启／换配置统一等待上述屏障，不能只隐藏窗口。验证后才允许原位置更新 EXE、DLL 和运行时目录；禁止按进程名批量 taskkill。
- 默认 worker 状态、秘密和日志位于 profile 的自有本地目录，例如 `%LOCALAPPDATA%/Qiku/profiles/native/collection/`。若用户配置其他根，必须明确授权、可写且不与程序／源媒体／不兼容数据根重叠。
- 对新建自有秘密／控制通道使用当前用户及必要 SYSTEM 的真实 DACL，复用现有桌面权限模式；POSIX 0600／0700 不作为 Windows ACL 证据，不重置未知既有目录 ACL。
- SQLite、检查点和令牌默认不放 UNC／映射网络盘。已授权网络媒体读取保持原行为；将来支持网络下载根须单独验证，不因主目录可读就视为可发布。

### 3.4 诊断与用户入口

- 在现有资源收集／平台能力中区分：未启用、运行时缺失／版本不符、worker 启动失败、共享根不匹配、依赖就绪、站点请求失败。探测成功仍不代表公网连通。
- 桌面配置／托盘提供启用或停用托管漫画下载、查看日志和重新检测；不增加浏览器任意路径、Python 参数、脚本或 YAML 输入。
- 增加不启动下载的 `--check-jmcomic` 类诊断入口，输出脱敏中文结果。保留命令行／外接 worker 的高级配置路径。

### 3.5 分发与更新

- 扩展 `scripts/release.py` 构建并纳入 worker，增加可选 JMComic runtime manifest。同步 `verify_release.py` 的严格元数据版本／成员校验，兼容无插件基础包与旧发行校验，不复用 FFmpeg 专属清单冒充 Python 包清单。
- 发行检查覆盖 x64 PE／`.pyd` 架构、精确版本、数据与 DLL 完整性、大小写重复／路径别名／未知成员、哈希、来源及第三方许可。运行时启动探测与发行清单完整性都要通过。
- 提供“基础包”和“含 JMComic 的完整包”明确区分；开发包不得声称可在无 Python 的系统离线直接使用。
- 更新时退出所有托管进程后替换程序，持久目录与 operation ID 不变。首次不做在线自动安装／自升级。旧程序回退必须与可读的配置备份对应，不能假设能读新 schema。

## 4. 分阶段实施顺序

1. **运行时验证与原生 CLI 最小闭环**：确定 CPython／依赖闭包和清单，完善绝对路径解析／诊断，使用真实私有运行时执行现有桥接与 JMComic 回归。可附管理员辅助脚本；脚本只操作当前进程环境，不要求全局执行策略变更。
2. **桌面托管**：配置迁移、独立 worker 所有权、私有 ready／stop、真实端口及整组连接注入、降级与退出屏障。保留 external 模式及 Linux／Docker 行为。
3. **正式打包与 UI 诊断**：完整包／基础包、运行时／许可清单校验、界面状态及文档；同步发布主应用、网页、worker 和运行时。
4. **目标系统验收**：干净 Windows 11 x64 普通用户真实安装／移动／退出／升级及显式站点测试，补齐回归证据后再宣布支持。

## 5. 验收矩阵

| 范围 | 必须取得的证据 |
| --- | --- |
| 干净机器 | 无 Go／Node／Docker／全局 Python，普通用户解压运行；全局代理／Python环境污染不改变托管解释器 |
| 路径 | 中文／空格安装目录与用户名、非当前工作目录、其他盘符、只读程序目录；支持范围内长路径成功，越界提前中文报错 |
| 运行时 | 缺文件、错误架构／版本、被篡改／未声明文件、丢失 CA／DLL／dist-info 时不能误判可用；基础媒体继续可用 |
| 编码 | 正常及失败流在非 UTF-8 控制台／重定向下行为确定，中文搜索／章节／标题不丢失 |
| 生命周期 | 重复双击、端口冲突、启动取消、下载中取消／退出／重启、父进程异常终止；无孤儿 worker／Python，独立哨兵进程保留 |
| Windows 文件 | 实际 Python／DLL 执行后退出，立即独占打开／覆盖 EXE／DLL、重命名程序目录、同 profile 重启；不用只有假工具的结果冒充真实 DLL 释放 |
| 持久一致性 | 成功下载可读；取消／不完整不发布；已验证页复用；崩溃后的 uncertain 状态保持原对账规则；worker 离线时历史漫画可读 |
| 兼容回归 | 旧 profile、外接 worker、原漫画／高级漫画／批量／UAV／媒体／资源收集／发行校验；Linux amd64／arm64 交编与目标运行分开记账 |
| 站点与部署 | 协议 fixture、真实依赖离线和真实公网小样本分别记录；Docker／NAS 不是本地 Windows 回归的自动通过项 |

本轮不执行上述实现和目标测试。实施阶段遵守中文注释／日志、明确类型、中文配置说明及项目环境约束；生成的临时单元测试执行后清除，保留复用集成脚本。

## 6. 研究依据与本轮交付

- [项目接线与现有边界](research/codebase-windows.md)：以当前源码和既有验证报告为依据，区分已存在的 Windows 能力与本次缺口。
- [Windows Python 运行时研究](research/windows-python-runtime.md)：包含 Python 官方私有发行、venv、隔离搜索路径、编码与 PyInstaller 官方资料，并区分事实、推断和未验证项。
- Python 精确补丁版本及 ABI 在目标 wheel、私有运行时和干净系统验证后锁定；当前开发环境版本只作候选依据，不作支持承诺。
- 本轮仅新增规划／研究材料；旧 JMComic 高级集成任务仍为 review，本规划任务仍为 planning。

## 7. 主要改动模块（建议）

| 模块 | 预计改动 |
| --- | --- |
| `internal/desktop/launcher.go`、`config.go` 及新增原生 worker 管理文件 | 配置迁移、托管、就绪／停止、降级和现有退出屏障整合 |
| `cmd/nas-collection-worker/main.go`、`internal/collectionworker/service.go` | 托管专用控制／就绪支持；普通 CLI 和原 Run 入口兼容 |
| `internal/collectionworker/config.go`、`jmcomic.go`、`tools.go` | 私有运行时可信解析、诊断、UTF-8及继承环境检查；复用原固定桥接 |
| `internal/app/collection_config.go`、能力展示相关组件 | 原成组连接规则不放宽；必要的本地诊断状态与中文提示 |
| 新运行时装配脚本、`scripts/release.py`、`verify_release.py` | 固定闭包、许可、清单、架构及包验证 |
| `scripts/verify_desktop.py` 及 JMComic 验证脚本 | 真实运行时生命周期、路径、旧配置和断点页面恢复回归 |
| `docs/windows.md`、`docs/jmcomic.md`、`docs/collection.md`、`docs/release.md` 及 Trellis spec | 安装／模式选择／升级／故障排查和真实支持范围 |
