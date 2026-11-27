# Windows 程序同级用户配置目录独立复核

## Findings (fixed)

未发现需要追加修改的产品代码缺陷。本次审查仅写入本报告，未修改主代理负责的实现、测试或文档。

核对结果：

- `internal/desktop/config.go`、`platform_windows.go`：默认 profile 基于实际 EXE 的规范绝对目录，取 `qiku-data/profiles/<mode>`；不读取 `LOCALAPPDATA`，失败明确提示 `--profile`，不存在 C 盘用户目录回退。显式 `--profile` 仍在默认路径解析前返回。
- `loadConfig`、`applyOptions`、`Main`：已保存 `desktop.json` 的绝对业务目录、身份和严格解析保留；CLI > 适用环境变量 > 保存配置 > 默认值的优先级不变。只打印配置不会创建用户目录。
- `collection_managed.go`：默认 profile 的托管状态可进入程序下的专用 `qiku-data` 子树；程序根、`tools`/`runtimes` 等其他子树和近似前缀仍拒绝。所有权、链接别名、本地磁盘及业务/媒体目录双向重叠检查保留。
- `ipc_windows.go`、`shell_windows.go`：普通启动、重复启动、控制命令和托盘另一模式入口都进入统一 `Main` 路径；固定 IPC 名称仍基于规范 profile，没有旧默认目录旁路。
- `platform_other.go`、共享 CLI 和发行脚本：Windows 默认路径变更保持平台隔离；Linux CLI 与容器内部数据布局未被本次实现改写。Docker 的桌面 profile 与业务命名卷需要在最终说明中明确区分。
- 已检索 Windows 文档、相关规格和脚本：当前旧 `%LOCALAPPDATA%` 引用用于说明历史目录，不再作为 Windows 新 profile 的默认值；临时测试中的污染值用于拒绝回退断言。

## Findings (not fixed)

无阻塞实现问题。

最终收尾仍由主代理负责：删除已执行的临时 `portable_profile_task_test.go`；完成发行包/跨平台验证记录；明确说明旧 C 盘 profile 不会自动迁移、移动程序目录不会自动重写已保存的绝对业务/媒体路径。本报告不将这些尚在主代理执行中的事项记为已完成。

## Verification

- Lint：pass。本审查独立运行 `git diff --check`、修改 Go 文件的 `gofmt -l`（空输出）和 `python -B scripts/project_env.py -- go vet ./internal/desktop`，均成功。
- TypeCheck：pass（Go 受影响包由上述 `go vet` 完成编译和类型检查）；`scripts/verify_desktop.py` 的 Python AST 解析通过。未改前端文件，本审查不重复主代理负责的仓库级检查，也不声称已独立执行 Vue 检查。
- Tests：pass（读取主代理实际运行记录）。`.runtime/verify/windows-portable-profile/temporary-go-behavior.stdout.log` 记录 3 个顶层行为测试及 5 个托管目录子场景通过；`desktop.stdout.log` 记录增强后的真实 Windows 桌面回归通过，包括不带 `--profile` 的首次启动、覆盖优先级、模式隔离、重复启动、SQLite 锁及退出释放。
- 只读边界：读取 `result.json` 与 `readonly-rejected.stderr.log`，确认实际 Windows DACL 拒绝写入、默认启动中文提示 `--profile`、不回退 `LOCALAPPDATA`，以及显式可写 profile 真正就绪并保存数据库/日志后正常退出。
- 证据边界：桌面回归为本机 Windows、显式 `--web-dir` 的真实启动证据；不等同于最终内嵌发行包、干净普通用户机器、真实 Docker 部署或插件实时站点验收。仓库级 Go、Vue 和 Linux 交叉编译结果由主代理单独归档。

以上 Go 和 Python 检查均通过仓库 `scripts/project_env.py` 入口运行；未修改用户级环境变量，未创建额外单元测试源。

## 收尾差异增量复核

本节审查 `verify_desktop_collection.py` 默认布局回归、`verify_desktop.py` 的 `LOCALAPPDATA` 哨兵、发行包开始使用说明以及升级/Docker 文档澄清；未修改这些文件，未重复主代理正在执行的发行和 Go 检查。

### Findings (fixed)

- `scripts/verify_desktop_collection.py`：新增 `use_default_profile=True` 分支使真实 `Popen` 命令完全不含 `--profile`；`cwd` 仍为程序之外的中文目录。此前场景使用的显式 profile 均不在新默认根，因此新场景会真正首次命中 `<program>/qiku-data/profiles/native`。随后通过该固定 profile 的 IPC 等待真实应用就绪，读取其配置并断言默认业务根，核对托管所有权身份、`data/secrets/tmp` 子目录、真实后代进程句柄和退出释放。回归覆盖了本次程序子树隔离规则变更，不只是打印配置。
- `scripts/release.py`、`docs/windows.md`、cross-platform 规格：发行说明仅在 Windows 分支新增；原位置升级明确保留整个 `qiku-data`，旧用户目录不自动迁移、移动程序不自动重写已保存绝对业务/媒体路径，均与 `loadConfig` 原样读取保存路径的代码一致。Docker 桌面配置与 Linux 业务命名卷已明确分开，消除了第一轮复核中待澄清的说明问题。

### Findings (not fixed)

- 文件：`scripts/verify_desktop.py` 的 `assert not forbidden_user_root.exists()`。
- 问题：新增断言将整个隔离 `LOCALAPPDATA` 根是否存在等同于“栖库用户配置回退”。主代理当前 `final-embedded-desktop-tray.stderr.log` / `final-result.json` 已记录此断言失败；现有失败记录没有保存该根的新增成员，仅凭根存在不足以判断是旧 `Qiku/profiles` 配置还是其他组件写入。此前的默认 `dataDir` 及实际 profile 文件断言已通过，不应据此宣称实现回退，也不能忽略失败宣布新增回归通过。
- 处理：已立即反馈主代理，建议先记录隔离根下的相对成员名称，再依据实际归属确认哨兵边界。由于本轮授权仅允许追加报告，未修改验证脚本。最终修复及重跑结果由主代理汇总到 `final-result.json`。

### Verification

- Lint：pass，增量涉及文件的 `git diff --check` 成功。
- TypeCheck：三个修改 Python 脚本的 AST 解析全部通过；本轮无 Go/前端实现变更，未重复相关检查。
- Tests：新增托管回归的调用与断言已静态复核；当前发行托盘回归在上述哨兵处失败，不能把本轮新增最终回归计为通过。最终运行状态以主代理完成后的 `final-result.json` 为准。

## LOCALAPPDATA 哨兵修正闭环

本节依据后续诊断及重跑证据，关闭前节唯一待核实问题；前节失败保留为历史记录。

### Findings (fixed)

- 文件：`scripts/verify_desktop.py`。
- 问题：将整个 `LOCALAPPDATA` 根存在误认为栖库配置回退。
- 证据：已读取 `.runtime/verify/windows-portable-profile/localappdata-members.json`，其 9 个成员均属于 `Google/Chrome` 目录树，内容为 `User Data/Crashpad` 的目录链、指标及元数据文件，没有 `Qiku`。同时核对 `Main` 重复启动发送 `open`，以及 GUI 主实例的 `handleControl("open")` 会打开系统默认浏览器；这与托盘场景浏览器缓存来源一致。
- 修正：主代理已将永久断言收窄为隔离 `LOCALAPPDATA/Qiku` 不存在，并增加中文说明。已核对原有 EXE 同级默认 `dataDir`、实际 `desktop.json` 路径、模式隔离和不传 `--profile` 首次启动断言仍保留，因此修正准确排除第三方浏览器缓存，同时继续检测原有配置根回退。

### Findings (not fixed)

本次独立审查无剩余阻塞问题。仓库级最终状态及 Linux 双架构编译由主代理继续汇总，不由本报告推断完成。

### Verification

- Lint / Python：修正后的 `verify_desktop.py` AST 解析及 `git diff --check` 均通过；未重跑长检查。
- 内嵌发行桌面：已读取 `final-embedded-desktop-tray.stdout.log`，`nativeTray=passed`、`resourceMode=embedded`，20 类桌面检查及十尺寸图标通过，记录退出耗时 0.090 / 0.048 秒。修正哨兵已随此次真实托盘回归通过。
- 默认托管布局：已读取 `final-collection-result.json`，11 类真实 worker/配置/IPC/所有权回归通过，明确包含“不传 profile 的程序同级配置、数据库及托管状态落盘，真实 worker 启动退出”。该记录的私有运行时和真实依赖探测均为 false，不代表 JMComic/UAV 私有依赖或站点验收。
- 最终 EXE 只读边界：已读取 `final-readonly/result.json`，实际 ACL 拒写、中文默认失败、无用户目录回退、显式可写 profile 的数据库/日志落盘及正常退出均有通过记录。
