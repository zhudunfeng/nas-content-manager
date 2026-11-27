# Windows JMComic 独立质量复核

日期：2026-11-26。角色：本任务独立 Trellis reviewer。审查依据为当前任务 PRD、技术接口、check.jsonl 指定规范及新增 `jmcomic-windows-contract.md`。本报告记录源码审查和独立执行证据；最终统一发行归档及既有全量业务回归由主线在最终源码稳定后重跑，不以实现者的早期二进制替代最终候选。

## Findings (fixed)

- File: `internal/collectionworker/managed.go`
  - Issue: `RunManaged` 合并服务与控制通道错误之前，无条件输出「已结束并释放自有操作」。初始化、协议或资源释放错误发生时，成功日志会与真实错误并存，违反生命周期失败不得宣布成功的契约。
  - Fix: 先合并并保留原始错误；存在错误时输出中文 Warn「退出检查未完整通过」及原错误，仅 nil 结果保留成功日志。没有改变控制消息、返回值或停止流程。
  - Verification: 临时 `TestManagedReviewFailureLog` 将数据根设为普通文件，使实际 `RunManaged` 初始化失败，确认原错误保留、失败中文诊断存在且成功日志不存在；连续运行 30 次全部通过。测试源码按项目要求删除，仅在 `.runtime/verify/jmwindows-independent-20261126T003723Z/managed_review_test.go.txt` 留存不可编译的证据副本。

- File: `internal/desktop/config.go`
  - Issue: 新增漫画启用／停用／重检已经接入固定 IPC，但 `Options.Control` 注释与 `--control` 帮助仍只列 open/status/quit，导致 CLI 的能力说明漏项。
  - Fix: 两处中文说明补齐 `collection-enable`、`collection-disable`、`collection-check`，不改变解析与执行逻辑。
  - Verification: 回读源码确认帮助与实际 `validControlAction` 白名单一致；gofmt 与 `go test ./...` 通过。

- File: `internal/jmcomicruntime/runtime.go`、`scripts/release.py`、`scripts/verify_jmcomic_runtime.py`（与运行时实现者同步修复）
  - Issue: 首版相对路径规则遗漏 Windows 扩展名前空格、控制台设备名及端口上标数字别名，与既有 `backup_storage_windows.go` 的设备名防护不一致。
  - Fix: 运行时实现者与 reviewer 同步确认该问题；实现者已统一 Go／Python 的设备别名拒绝规则，覆盖 `CONIN$`、`CONOUT$`、`CLOCK$`、`COM¹`／`LPT²` 及 `NUL .txt` 等，并补充负向验证。reviewer 核对最终源码，未重复覆盖协作者修改。
  - Verification: 实现者的 Python／真实 Go 交叉验证 49 项通过，结果在 `.runtime/verify/jmcomic-runtime-result-8e11771ee46e490dabb4814d1320c2e4/result.json`。来源 URL 在最终源码中也已采用结构化 URL 校验，原 HasPrefix 形式不再存在。该行为证据由运行时实现者执行，不能重复计作 reviewer 独立执行的第二套覆盖。

## Findings (not fixed)

没有发现需要产品决策或阻塞本轮交付、但无法直接修复的问题。干净 Windows 普通用户机器、公网站点、其他 Windows／ARM64／Linux 实机验收仍是明确的目标环境证据缺口，并非本机离线验证的通过项。

## Source and cross-layer review

- 私有运行时：审查固定清单 schema、依赖锁与 LF 规范化、来源闭包、完整文件枚举、PE x64／`_pth`、证书、元数据、许可、祖先重解析点和路径拒绝规则；未发现放宽显式错误为 PATH 回退的路径。
- Worker：审查固定 `-I -B -X utf8` 桥接、独立本地诊断、真实监听器就绪、严格有界 start/ready/stop、关联身份、父 EOF 与停止取消、服务 Close 错误传播；未改变搜索／选章／发布／恢复状态机。
- 桌面：审查 schema 1→2 身份与业务根保持、字节精确保留备份、print-config 只读、CLI／环境／保存值优先级、三模式连接快照、专属目录归属和 Windows DACL、挂起归 Job 后恢复、能力认证及落盘挑战、正常／异常退出屏障与外接进程不接管。
- 桌面异常 worker 退出后的重检：最终实现将 worker 自身 exitErr 与 Job／句柄 cleanup err 分开，完整回收后允许用户主动重检；真正资源清理失败仍拒绝重启。没有无限重试或重放不确定下载。
- 发布：审查所有新目标均包含 worker、schema 1 兼容与 schema 2 漫画摘要、运行时可选且仅 Windows amd64 激活、重新校验归档成员以及 `-development` 独立标识。无 FFmpeg 工具材料的包仍不称完整离线媒体发行。
- 规范：新增 Windows 契约与 runtime／cross-platform／JMComic 接线一致；已有契约已明确规定错误日志不得宣称成功，本次修复是在履行该条款，无需扩张契约。

## Verification

所有本地构建、测试及临时工具均通过 `python -B scripts/project_env.py -- ...`，缓存与临时文件在仓库 `.runtime/`；没有修改用户级环境、调用 `go env -w`、Git 提交、启动 Docker 或访问漫画站点。

| 项目 | 结果及范围 |
| --- | --- |
| Lint | **pass**：独立执行 `.tools/go/bin/go.exe vet ./...`；证据 `jmwindows-independent-20261126T003723Z/2.log`。新改两份 Go 经仓库 gofmt，作用域 `git diff --check` 无空白错误，只有既有 Git 行尾提示。 |
| TypeCheck | **pass（Go）**：最终帮助说明修改后 `.tools/go/bin/go.exe test ./...` 退出 0；全部保留包显示 `[no test files]`，因此仅作为全项目编译／类型检查。Vue 严格类型与生产构建由主线最终统一回归记录，本 reviewer 未重复执行。 |
| Tests | **pass（独立定点行为）**：`.tools/go/bin/go.exe test ./internal/collectionworker -run '^TestManagedReviewFailureLog$' -count=30 -v`；30 次全部通过。命令与退出码见 `jmwindows-independent-20261126T003723Z/result.json`，原始输出见 `1.log`。 |
| 临时源码清理 | **pass**：回读确认 `internal/collectionworker/managed_review_test.go` 已不存在；未遗留本 reviewer 生成的 `_test.go`。 |
| 实现者专项证据 | 运行时 49 项与 release self-test 91 项、worker 私有运行时 20 项、真实固定桥接离线套件、桌面私有运行时 14 组（26 个真实加载映像／嵌套 Job／异常 worker 重检）分别由对应实施报告记录；已核对其证据范围，不能当作本 reviewer 独立复跑。 |
| 最终候选全量回归 | 由主线在本报告所述源码稳定点之后生成新候选、记录源码摘要并统一执行；早期候选及其测试不得标为最终候选。 |

补充证据参见同目录 `runtime-implementation.md`、`worker-implementation.md`、`desktop-implementation.md`。真实依赖离线 probe／桥接与真实公网站点访问已明确区分；Go 空测试集、Linux 交叉编译、管理员开发机、可信结构夹具均不扩大为未执行目标的验收。
