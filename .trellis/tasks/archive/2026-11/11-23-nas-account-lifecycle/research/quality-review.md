# S02.1 独立质量复核

日期：2026-11-23。平台：Windows AMD64。范围仅为 **S02.1 账号存储与认证竞态防线**，以及本轮真实集成发现并由主线修复的 HTTP 已知小正文统一收尾。S02.2 在线账号 API/UI、S02.3 离线 CLI 仍未交付；本报告不把整项 S02 标为完成。

## Findings (fixed)

本轮独立复核未发现新的确定缺陷，因此没有修改业务代码或稳定集成脚本。已独立验证主线与实施者交接的最终修复；没有把测试通过建立在放松状态码、伪旧库或重试掩盖失败之上。

复核期间仅新增不同名临时测试 `internal/app/s02_review_temporary_test.go`，实际执行完成后已删除。保留原有脚本及全部历史成功/失败证据，未提交或部署。

## Findings (not fixed)

S02.1 范围内无剩余确定问题。下述后续/设备边界不属于本片缺陷，不能标为已验收：

- 在线改密、会话分页/撤销、再认证限流、账号严格字段解析和账号安全 UI 属于 S02.2。
- 停机离线重置、终端输入与 Windows/Linux 数据锁维护路径属于 S02.3。
- 未实测 Linux/NAS、真实外部服务、Docker/BT、真实浏览器账号页或真实大视频性能。

## 审查范围与结论

- 重新读取当前 task/PRD/info、implement/check 上下文、实施记录、HTTP 复盘及正式 account/runtime 规范；依 Trellis check/before-dev 和相关后端、跨平台、跨层规范执行。优先使用已同步 CodeGraph 定位 `readAccountSession` 与 `discardUnreadSmallBody`，再读取实际源码与 diff。
- schema 5 是追加迁移；schemaV1–V4 未被改写。历史账号、摘要、会话到期原文保留；历史创建时间保持 NULL，客户端保持 unknown。公开 ID 单独调用随机源生成，不由 bearer 或摘要派生，迁移完成后用唯一索引与格式/非空限制保护。
- setup/login 传递实际 ID/hash/revision 快照，bcrypt 在事务外，条件插入重新匹配三者并检查影响行数；失败不设置 Cookie。读会话核验当前修订，解析 RFC3339Nano 后精确比较，到期相等即失效。
- 精确读取原语可接受真实 `*sql.Tx`；单连接事务内调用不会反取第二连接。清理只按至少前 24 小时的 UTC 日期保守删除；它不代替认证。清理失败保留有效登录并输出固定脱敏中文告警。
- `verify_schema.py` 从原历史迁移构造旧表，只复制其原有列。外部回归先停服/checkpoint，构造真正 schema 3 再升级，没有仅修改新版库 user_version 的伪迁移；原来源和用户保留断言仍在。新账号脚本验证真正 schema 4 的弱/中文密码与旧 Cookie。
- security 仅统一 defer 一次小正文收尾。原始流记录已读位置；0/未知长度/>64KiB/Expect/deadline 不支持均跳过。普通 EOF 是成功，成功才清零读取 deadline；实际读取错误保留已过期 deadline 并关闭连接。没有包装原生 Body.Close 或新增全局 WriteTimeout；媒体原生流与 Range 路径保留。
- 没有脚手架模板同步入口；仓库没有该行为的模板镜像。正式 account/runtime 与 collection 版本规范已由主线同步，本轮不重复写其所有权文件。

## Verification

以下均为本 checker 独立执行，而非仅引用实施者的通过值。全部构建、测试和临时工具命令通过 `python -B scripts/project_env.py -- ...`，缓存/临时目录限定在仓库 `.runtime/`。

| 检查 | 实际结果与证据 |
| --- | --- |
| 新临时 Go 行为测试 | **6 个顶层测试全部通过**；日志 `.runtime/verify/s02-1-review-go.log`，源文件已删除 |
| gofmt | `account_store.go/auth.go/store.go/server.go` 的 `gofmt -l` 无输出 |
| Lint | `go vet ./...` 通过；`.runtime/verify/s02-1-review-vet.log` |
| 全包编译 | 删除临时源后 `go test ./... -count=1` 通过；所有包 no test files，仅算编译证据；`.runtime/verify/s02-1-review-compile.log` |
| 主程序构建 | 独立 `go build` 成功，产物标识如下 |
| 前端 TypeCheck | `npm --prefix web run typecheck` 通过；`.runtime/verify/s02-1-review-web-typecheck.log` |
| 前端生产构建 | `npm --prefix web run build` 通过；`.runtime/verify/s02-1-review-web-build.log` |
| Python | 6 个受影响 verifier 的 AST、`verify_account` 导入及 `AccountHarness.database` 返回类型解析通过 |
| 局部 diff | 已跟踪受影响代码/脚本 `git diff --check` 通过；仅有 LF/CRLF 提示 |
| 真实账号 HTTP | **43 项通过**；`.runtime/verify/s02-1-review-account/result.json`、`assertions.json` |
| 外部领域 | **84 项通过**；`.runtime/verify/s02-1-review-external/result.json`，真实本应用到受控四类 HTTP 协议服务，不代表真实第三方实例 |
| 核心媒体 | **52 项通过**；`.runtime/verify/s02-1-review-core/run-manifest.json`、`evidence/core-assertions.json`，manifest 为 passed、子验证 exit 0 |

### 独立临时行为测试实际覆盖

1. `TestReviewS02HistoricalMigration`：从真正 v1/v3/v4 结构启动，检查旧 hash/到期原文、零修订、NULL/unknown、旧会话鉴权与重启稳定；索引冲突触发 ALTER 后回滚；schema 6 拒绝，版本/字段/会话不变。
2. `TestReviewS02LoginCAS`：在独立 goroutine 完成 bcrypt 校验后用闸门暂停，主线程分别改变 hash、revision、两者，再放行建会话；全部为 409、无 Set-Cookie、无新增会话。
3. `TestReviewS02ExactExpiryAndTransaction`：同秒无小数/可变小数、精确相等、纳秒超出、非法时间；保守清理不误删未到期行；真实单连接短事务读取并立即观察本事务的修订更新。
4. `TestReviewS02RawSecretAndCleanupFailure`：直接调用真实 login/createSession，以注入的原始内存 slog 缓冲扫描登录结果和日志；注入 DELETE trigger 故障验证清理失败仍创建有效 Cookie，原始数据库错误不出日志。
5. `TestReviewS02BodyRemainderAndSkip`：未读/部分/全部消费的普通 EOF，恰好 64KiB、0/大体/未知/Expect/deadline 不支持跳过，实际读取失败后 deadline 非零且已过期。
6. `TestReviewS02TCPReuseAndSlow`：真实 TCP 分开发头体，成功 202 和跨站 403 各 **20 次**均准确；同一连接三次完整 JSON 200、原值不变、无误关；keep-alive 慢体在局部一秒预算加原生关闭保护内交付 202，随后连接确实关闭。

**原始秘密扫描的准确范围**：上述第 4 项的后端内存日志及 JSON 响应，扫描本次固定旧中文密码、实际 bcrypt hash、实际 bearer token、其 SHA-256 摘要和原始 UA 标记；另外扫描受控清理错误标记。日志缓冲在脱敏前读取，未使用持久日志脱敏器。此结果只证明这次调用经过的账号路径，不泛称所有进程 stdout、全部失败路径、IP、浏览器存储或未来 API 均已动态扫描。其他新增账号日志已源码检查只含固定中文与安全 userId；真实 HTTP verifier 的持久日志检查仍按其“可分享脱敏日志”原范围解释。

### 独立真实集成命令

```powershell
python -B scripts/project_env.py -- go build -o .runtime/bin/s02-1-review/nas.exe ./cmd/nas
python -B scripts/project_env.py -- python -B scripts/verify_account.py --binary .runtime/bin/s02-1-review/nas.exe --root .runtime/verify/s02-1-review-account
python -B scripts/project_env.py -- python -B scripts/verify_external.py --binary .runtime/bin/s02-1-review/nas.exe --root .runtime/verify/s02-1-review-external --workflow
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases core --artifacts .runtime/bin/s02-1-review --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe --ffprobe D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffprobe.exe --large-file --root .runtime/verify/s02-1-review-core
```

账号结果明确 `accountMutationAPIs=false/offlineCLI=false/targetNAS=false`。外部结果明确 `realExternalInstances=false/realMediaChanged=false/thirdPartyToolsInstalled=false`。core 包括五媒体、用户状态、实际 FFmpeg、SQLite 一致备份和 5GiB 稀疏偏移；稀疏文件不等于真实解码/吞吐，未选基线组仍为 not_run。core 自有服务最终 exit 1 是回收自有进程树的记录，manifest 同时明确子验证 exit 0、owned-tree-reclaimed，不误写成服务崩溃或自然退出。

### 产物与收尾

- 独立产物：`.runtime/bin/s02-1-review/nas.exe`，21,260,800 字节。
- SHA256：`be86e7447505884e18ebacbc66d6f6fb3a85759e4f1a3970b5d3424e77a742f6`，与交接的最终 stable 一致；本轮没有业务修复，因此没有产生不同业务版本。
- core 的 web index SHA256：`aee6e8138db92cb2ba27199e026e4abbfcb60b765fb7cdddad4c7ee3cc03b9cf`。
- 三个独立运行根全部保留；已读回 result/manifest 确认状态、计数与产物标识。本轮临时 Go 源不存在，未发现匹配本次 review 产物/运行根的 nas/python 残留进程。
- 当前业务文件与稳定脚本停止写入，交回主线；本轮仅新增本复核报告，不擅自启动 S02.2 或变更整项任务状态。
