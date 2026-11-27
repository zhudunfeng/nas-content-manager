# UAV 自动初始化独立质量审查

## 范围与来源

- 日期：2026-11-27；真实 Windows amd64 开发机。
- 审查目标：启用 UAV 后，在 owned profile 后台初始化固定依赖、严格验证、最终位置离线探测、原子发布和桌面串行激活。
- 依据：本任务 `prd.md`、`info.md`、`check.jsonl`；`.agents/skills/trellis-check/SKILL.md`；`.trellis/spec/backend/{uav-windows,jmcomic-windows,cross-platform,collection,runtime}-contract.md`。
- 检查了真实入口与调用链：`internal/desktop/{launcher,collection_config,collection_controller,collection_managed,uav_initialization}.go`、`internal/uavbootstrap/*.go`、现有 `internal/collectionworker/{uav,tools,managed,service}.go`、资源生成器及桌面集成脚本，并复核 `scripts/release.py` 的基础包说明分支。
- 未修改无关 Trellis 历史任务、已有工作区改动、发布身份或现有严格运行时加载器。

## Findings (fixed)

### 1. 最终目录原生加载与暂存探针不等价

- 文件：`internal/uavbootstrap/bootstrap.go`。
- 问题：原逻辑在较短的 `stage-<32hex>` 目录执行真实探针，再改名至更长的 `generation-<32hex>`；即使静态校验和暂存探针均通过，正式 worker 仍可能因 curl_cffi 依赖 DLL 路径过长而失败。仅检查规范路径小于260个 UTF-16 单元不足以保证当前固定依赖可加载。
- 独立证据：复制同一真实私有 runtime，在等于集成样本长度的中文路径中，stage 根147字符、最长 DLL250字符时导入通过；改名后的根152字符、最长 DLL255字符时，两种不同 DataDir/TEMP 环境均报 `curl_cffi._wrapper` 的“文件名或扩展名太长”。继续只改名同一夹具，最长 DLL248/249/250成功，251至260全部失败。证据：`.runtime/verify/uav-review/native-loader-paths.json`。这将故障收敛到最终路径加载，而非 managed 状态目录或返回了 stage 路径。
- 修复：由 runtime 实现者将最终目录收敛为 `gen-<32hex>`，统一选择、临时指针与清理规则；下载前及 Resolve 检查最终原生文件最长248个 UTF-16 单元；先改名及严格静态复验，再在最终位置真实探测，成功后才发布 `current.json`。静态暂存校验不执行原生文件，也不错误套用最终加载预算。
- 独立复核：新前缀在选择正则、创建、临时选择文件及失败清理边界中一致。完整 Initialize 故障/取消注入均通过，见下文。真实最终位置 CheckUAV 证据另见 `research/runtime-implementation.md` 与 `.runtime/uav-bootstrap-final-relocation-tests.log`。

### 2. 构建物料下载失败后可能阻塞重试或清理其他尝试

- 文件：`scripts/uav_bootstrap.py`。
- 问题：通用下载器失败后留下按摘要命名的 `.part` 文件，后续独占写入会被同一残留永久阻塞；GitHub 分支使用共享 `.downloading` 路径且无条件清理，不同尝试间存在临时文件所有权冲突。
- 修复：审查者直接将两种下载分支统一放入每次独享的 `tempfile.TemporaryDirectory(prefix="uav-source-", dir=cache)`，只清理本次中间文件，精确校验后才改名至最终 SHA 缓存文件。
- 验证：内联 Python 分别注入通用下载器、GitHub 响应流部分写入后异常；两者失败均无残留，同目录重试成功；最终生成资源 `--check` 通过，20份固定来源及嵌入资源字节不变。

### 3. 桌面默认 managed 合同需要同步显式 UAV 开启例外

- 文件：`.trellis/spec/backend/jmcomic-windows-contract.md`、`.trellis/spec/backend/cross-platform-contract.md`。
- 问题：原合同只描述相邻 manifest 存在时新 profile 选择 managed，与本次明确 UAV=true 可自动准备的实际选择入口不一致；共享生命周期合同也缺少后台初始化的取消和串行激活约束。
- 修复：审查者直接同步新 profile 的显式 UAV 开启例外，并链接 UAV 专属合同；共享合同记录媒体就绪后准备、Job 探针屏障、串行 stop/start 和 disabled/external/自定义 Python 零安装规则。保留旧 profile 选择及 JMComic 不自动安装的边界。

## Findings (not fixed)

目前没有未修复的代码审查阻断项。下列总体集成结果仍由主任务收集，不能用本节代替最终验收。

## Verification

所有构建/测试均通过 `python -B scripts/project_env.py -- ...` 执行，缓存与临时数据位于仓库 `.runtime/`。

### 审查者独立执行

- Lint：`.tools/go/bin/go.exe vet ./...`，最终 runtime 修复后再次通过。
- TypeCheck/编译：`.tools/go/bin/go.exe build -o .runtime/verify/uav-review/nas.exe ./cmd/nas`，最终 runtime 修复后再次通过。此次未修改 TypeScript，不将未运行的前端检查记为已通过。
- Go 临时行为测试：`.tools/go/bin/go.exe test -count=1 -v ./internal/uavbootstrap -run TestReviewerFinalProbeFailure`，PASS，测试本体50.83秒，包52.862秒。
  - 先通过真实20份已校验缓存和 `materializeWithClient` 还原并严格验证791个配方库存文件；加上 manifest/receipt 共793个实际文件。
  - 仅在测试进程中浅拷贝已解析计划，将同一真实库存 ZIP 作为 payload 提供完整 Initialize 的失败注入输入；没有修改生产接口、嵌入材料、清单、库存或网络策略。这不是第二次联网下载验收。
  - 首次 final 探针失败：回调确认路径为最终 `gen-<32hex>`，`current.json` 尚不存在；返回预期错误后，候选 final 被清理。
  - 已有合法选择时：复用探针故意拒绝旧依赖后，新 final 探针再次失败；旧 current 精确字节不变，Resolve 仍指向旧代次，新候选被清理。
  - 新 final 探针返回成功但同时取消上下文：不发布新选择，旧选择/Resolve 保留，候选被清理。
  - 全部结束后 base 仅有 owner、原 current、原 gen，无 stage/新 gen/临时选择残留。
  - 临时 `reviewer_failure_temporary_test.go` 已删除，自动测试夹具已完整清理。
- 资源一致性：`python -B scripts/uav_bootstrap.py --runtime-manifest .runtime/tools/uav-private/uav-runtime.json --check` 最终通过；实际私有 Python3.14.4离线探测通过；20份固定映射、payload506897字节确定性比对通过。
- Python 生成/验证脚本 AST、相关 `git diff --check` 通过；Git 提示 LF/CRLF 工作区转换不是 diff 检查失败。
- 原生加载路径诊断使用已验证真实 runtime，无视频站点请求；只改名同一夹具得出上述边界。其 owned 目录 `.runtime/tmp/ur-f900a5tx` 的递归删除被自动策略拒绝，未更换工具绕过；保留该诊断数据，不影响产品代码或测试结果。

### 主任务最终包验证（已核对报告）

- `release/qiku-0.1.0-windows-amd64-development.zip`：从最终开发包取内嵌网页 EXE 和 worker，运行 `scripts/verify_uav_bootstrap.py`，没有指定 `--web-dir`。报告 `.runtime/verify/windows-uav-auto-init/bootstrap-report.json` 为 passed，总102.676秒、实际在线初始化65.732秒。
- 已核对的7组断言包含：false/disabled/external/错误自定义解释器零初始化；错误代理失败时媒体ready、中文可重试、无半成品/无限重试；下载中停用、正常退出等待清理且无迟到激活；只读程序目录、中文空格profile、异cwd及污染Python环境；20真实固定材料在线还原后最终位置worker探测及真实能力ready；真实H.264一秒编码/ffprobe/解码；退出全部映像可独占访问；错误代理离线重启时所有文件hash/mtime/current不变。
- 媒体报告：H.264、1.0秒、1880字节，解码passed；这只是隔离小样本工具可用性，不代表真实视频站点或大视频性能。
- 主任务报告最终 release 边界自测131项及最终ZIP24文件完整校验通过，最终 `go vet ./...` 和 `go test ./...` 通过。删除临时行为测试后的空测试套件只作为全仓编译证据。
- 主任务随后完成最终包桌面环境11项、普通桌面内嵌网页及真实托盘/10尺寸图标、双插件组合25项回归；报告为`env-final-report.json`和`collection-final-report.json`，完整命令及观察边界见`research/final-validation.md`。Linux amd64/arm64 core+worker共4次交叉构建亦通过。它们是主任务执行证据，不冒充审查者另行重复执行。

当前独立证据不声称干净 Windows、真实视频站点、Linux/Docker/ARM 运行时、物理托盘操作或对应源码/再分发人工审查已通过。
