# JMComic 自动初始化独立质量审查

日期：2026-11-27（Asia/Shanghai）。范围为当前任务 PRD、`check.jsonl` 对应契约和本轮共享引擎、插件包装器、桌面编排与构建/验收脚本。保留了工作区既有改动；本审查没有提交、归档、调整全局环境或修改用户安装目录。

## Findings (fixed)

- File: `internal/desktop/collection_initialization.go:365`
  - Issue: 批次完成与媒体服务退出信号同时就绪时，控制循环可能先消费初始化完成；原激活条件只检测媒体状态快照和 worker 退出，未检测尚未消费的媒体退出结果，存在过期完成触发自动激活的窗口。
  - Fix: 审查时与桌面实现方协调确认，当前实现增加 `len(c.serviceDone) > 0` 激活门禁，保留通道内原始错误交给生命周期分支处理；同时复核当前 worker 仍允许自动准备成功的插件。此改动由桌面实现方写入，审查者已重新读取最终代码。
  - Evidence: `research/desktop-implementation.md` 记录 `TestInitializationTaskActivationRequiresCurrentOwnership` 已通过，覆盖待处理主服务错误及当前插件门禁失效。本审查后续独立 `go vet`、编译和格式检查也通过。

## Findings (not fixed)

无尚未修复的具体代码问题。实际联网与最终发布包验收由主任务执行，未在本审查重复下载材料或请求漫画/视频站点。

## 核对结果

- Windows amd64、native/managed、预期启用和非显式 Python 的门禁分别适用于 JMComic 与 UAV；false/disabled/external 和错误显式 Python 保留零安装及不回退语义。来源：`internal/desktop/collection_config.go`、`collection_managed.go`、`collection_initialization.go`。
- 有效随包运行时优先，其次严格验证的 profile 代次；后台准备在媒体真实就绪、实际 worker 能力不可用后才启动。初始化状态不修改 `Capability.Available`，新旧 IPC 状态均按 kind 隔离，快照复制数组及兼容对象。
- 固定 JMComic→UAV 队列、普通失败隔离、每轮一次尝试、清理失败阻断、整批至多一次受控激活、排队用户命令和退出信号优先等路径与任务契约一致。stop 撤销进度归属，取消后等待整批结果及清理；重检不抹除 `ErrCleanup`。
- `runtimebootstrap.New` 防御性复制资源并独立缓存计划；JM `jm-bootstrap` 与 UAV `uav-bootstrap` 使用独立材料/owner/current/generation。UAV 包装器保持原公开 API 及受信根推导工具路径；JM 不安装视频工具。
- 共享引擎保留固定 HTTPS/来源与跳转、有限超时/重试、精确字节/SHA、ZIP 路径/类型/别名/预算、全文件及目录闭包、插件严格加载器和最终原生路径长度检查。真实最终路径探针先于原子 current 发布；取消/失败只移除当前尝试，保留旧选择。
- Python 资源生成复用固定材料处理，运行时不执行 pip；生成的运行时清单、配方与载荷保持独立。验收脚本以实际 worker/认证 API 判定可用，保留离线文件摘要与 mtime 比较、取消和双插件一次激活检查。

## Verification

以下命令均由本审查者实际执行，全部通过 `python -B scripts/project_env.py -- ...`：

- Lint: pass。`go vet ./internal/runtimebootstrap ./internal/uavbootstrap ./internal/jmcomicbootstrap ./internal/desktop` 返回 0。
- Formatting: pass。`.tools/go/bin/gofmt.exe -l` 检查上述三个 bootstrap 包与六个受影响 desktop 文件，无待格式化文件。相关 `git diff --check` 返回 0，仅提示原有 LF/CRLF 转换。
- TypeCheck: pass。`npm --prefix web run typecheck` 中 `vue-tsc --noEmit` 返回 0。
- Go compilation: pass。`go test ./internal/runtimebootstrap ./internal/uavbootstrap ./internal/jmcomicbootstrap ./internal/desktop` 返回 0，四包均为 `[no test files]`，这里只计编译证据。
- Python syntax: pass。`ast.parse` 检查 `scripts/bootstrap_common.py`、`scripts/jmcomic_bootstrap.py`、`scripts/uav_bootstrap.py`、`scripts/verify_jmcomic_bootstrap.py`，四文件通过；未产生 pyc。
- Behavioral tests: 引用实现方已运行的证据，不冒称本审查重跑。`research/runtime-implementation.md` 记录共享生产发布路径、取消/完整性/ZIP/下载边界及两套实际锁定材料的最终路径真实探针；`research/desktop-implementation.md` 记录 11 组桌面行为测试。临时单元测试源码已由各拥有者按用户要求清除，本审查没有新增测试源码。

## 边界与来源

- 最终包构建、首次联网、完整重启离线复用和真实 Windows 映像释放证据由主任务汇总；这些不由本次静态/编译检查代替。
- 不将本机隔离验证扩大解释为干净普通用户 Windows、真实站点、Linux/Docker/ARM 或正式发行许可审核通过。
- 长期规则由主任务统一维护 spec。本审查未直接修改 spec；本次已确认生命周期门禁、独立命名空间、实际能力权威性和最终路径探针要求均已进入当前 JMComic/UAV Windows 契约。
- Sources: `../prd.md`、`../check.jsonl`；`.trellis/spec/backend/{jmcomic-windows-contract,uav-windows-contract,cross-platform-contract}.md`；`internal/runtimebootstrap/`、`internal/{jmcomicbootstrap,uavbootstrap}/bootstrap.go`；`internal/desktop/{collection_initialization,collection_managed,collection_controller,collection_config,launcher,shell_windows}.go`；`scripts/{bootstrap_common,jmcomic_bootstrap,uav_bootstrap,verify_jmcomic_bootstrap}.py`；`research/{runtime-implementation,desktop-implementation}.md`。
