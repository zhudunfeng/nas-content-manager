# S02.2 独立质量复核

日期：2026-11-23。本轮按当前 PRD、冻结 info、check.jsonl、账号/runtime/frontend/baseline 契约及跨层指南执行。继承并保留 S02.1 的 `quality-review.md`，不把 S02.2 结论写回旧片证据。S02.3 离线重置尚待主线另行推进。

## Findings (fixed)

### 空密码框的显示状态未在另一表单完成后恢复

- File：`web/src/components/AccountSecurity.vue`。
- Issue：空字段已切 `type=text` 时，再将 model 赋同值空字符串不能触发共享输入组件 watcher；真实错误原密码请求结束后，另一表单的空框仍为明文状态。
- Fix：新增局部 `secretReset`，每次 `clearSecrets()` 推进；四个输入各自使用唯一字段前缀 key，使清理恢复全部默认掩码。保留原 label、ID、autocomplete、提示与 aria 关联，不改共享输入和对话框实现。
- 失败与回归：`s02-2-review-browser-empty-mask` 保留真实 400 后失败的证据；修复后的 `s02-2-review-browser-final` 138 项通过，其中 122 真实浏览器/14 受控呈现/2 资产检查。四字段重置后标签关联、异步卸载、敏感写不自动重发、键盘/modal 与四宽度双主题均通过。

### 浏览器证据身份与失败清理补强

- File：`scripts/verify_account_browser.py`。
- Issue：原浏览器结果未绑定全部实际前端构建文件；分页仅比界面自身不足以独立证明当前身份及完整有效集合；清理异常不应中止其余 context 与 app 的回收。秘密扫描应保留撤销前后全部已知值。
- Fix：源/复制后/验收后资产摘要核对和独立资产计数；实际 Cookie 摘要到隔离 DB 公开号的独立比较；每次真实登录立即记录 token/digest、改密前后 hash；全部 context 尽力关闭、browser 清理错误使整体失败且继续 app 回收。细节与动态故障注入见 `browser-review.md`。

## 实际代码路径复核

独立通读 `account.go`、`account_store.go`、账号关联 auth/server、`AccountSecurity.vue`、api/types/password、共享 PasswordInput/DialogShell 与稳定 HTTP verifier。CodeGraph 对新增浏览器/账号符号查询无命中，当前新文件尚未进索引，随后按实际源码与 rg 追踪；未凭图谱缺失认定功能未实现。

- 严格 JSON 确认使用实际 token 解码路径：精确 camelCase、转义后重复键、未知/缺失/类型、尾随、已知/未知 ContentLength 的 64KiB 上限；公共 JSON decoder 未被全局替换。
- 原密码兼容旧空/弱/中文/空格，仅保留 72 UTF-8 字节上限；新密码调用共享 ASCII 四类规则，不 trim/截断。错误原密码 400 保留会话，真实会话失效 401、账号快照冲突 409 分开。
- 两敏感端点共享账号12/实际会话6次固定5分钟预算；先原子预占后离锁 bcrypt，换会话不清账号预算，总键2048保守拒绝，不逐出活预算。
- 短事务先比较账号 ID/hash/revision，变更409优先；再用同一事务读取实际 session，失效401。bcrypt/hash 在事务外；写后提交前检查取消/精确到期；当前删除后的检查用写前已验证 expiry。修订 int64 溢出明确拒绝。
- 会话列表按随机公开 ID keyset，最多查询 limit+1，前 limit 原始行按同一 asOf 过滤，不补页；lookahead 的 nextCursor 是最后消费原始 ID；当前来自实际 Cookie 单独固定。DTO 不包含 token/hash/IP/raw-UA。
- 单撤销本人范围幂等；撤销其他返回本事务实际删除数（含过期/旧修订）；只在提交成功后清 Cookie/输出安全中文成功日志。
- UI 读 generation/AbortController 防迟到结果、敏感写互斥、未知结果不重发、401 统一退出。局部 key 修复后没有改动全站导航、共享对话框或现有设计。
- `docs/api.md`、account spec 当前在线 DTO/错误/预算/分页语义与代码一致。主线新增 baseline `account` 注册的参数为现有 `--binary --root`；证据分类为 `local-real-application` + `historical-schema-fixtures`，没有误称外部协议服务。独立浏览器不被 baseline 自动启动，与文档一致。
- 主线 `verify_baseline_selftest.py:wait_heartbeat` 小改已静态核对：只在有界预算内等 ASCII 数字内容变化，跳过文件 truncate/write 的空窗；仍检查独立 observer 进程 poll，未改 OwnedProcess 回收。新增空心跳拒绝/延迟有效心跳测试合理。本轮未代主线重跑；主线 `s02-2-account-orchestrator-selftest-3` 41 项结果由其记录。

## 独立确定性测试

本轮临时 `internal/app/s022_review_temporary_test.go` 的六个顶层测试全部通过，日志 `.runtime/verify/s02-2-review-go.log`。运行成功后已删除该临时源码，最终又确认文件不存在。

| 测试 | 核心断言 |
| --- | --- |
| `TestReviewS022PasswordRace` | 两个 bcrypt 后快照用闸门确定顺序；首成功、后409；只保留首hash/revision1/零旧会话；旧登录快照不能补建 |
| `TestReviewS022RevocationRollback` | 他会话撤销后敏感写401、账号变更409优先；SQLite DELETE触发器失败整笔回滚；取消、提交时到期±1ns/等时、int64最大修订 |
| `TestReviewS022Pagination` | 123条混合原始会话、limit3、连续空批、真实当前固定、无重复/完整有效集合、删除游标仍继续、不会无限补页 |
| `TestReviewS022Limiter` | 64并发只准6次、跨会话总12；拒绝不计数/续期；精确5分钟重置；2048满不逐出 |
| `TestReviewS022StrictJSON` | 实际 security+handler 的严格 JSON/大小写/转义重复/null/尾随/超限矩阵；非法 query；hash/revision/预算无副作用 |
| `TestReviewS022RawSecretLogs` | 实际错误再认证、revoke-others、password-change 的未脱敏内存 slog/DTO/Set-Cookie 原值扫描；旧/新/错误密码、hash、实际 token/digest、IP/raw-UA 标记不在业务日志；固定中文动作日志实际出现 |

最后一项**没有**把单会话撤销计入本轮动态日志覆盖；实施者此前九项测试另有此分支，不能混为本轮独立执行。浏览器三组故障注入另证明原始含密码 Playwright 异常不进入捕获输出/持久文本，清理失败不能通过、后续 context/app 仍回收；不是全部服务原始 stdout 的无限扫描。

独立读取生产 `web/src/password.ts`，用已安装 TypeScript `transpileModule` 转译后调用真实 `validNewPassword`，54 项通过：空值、9/10/72/73、缺四类、全部32标点逐项与合并、中文/全角/空白/控制/emoji/首尾空格。证据 `.runtime/verify/s02-2-review-password-ts.json`；未另写一份正则代替生产实现，未保留临时测试脚本。

## 最终实际产物与回归

- 独立产物：`.runtime/bin/s02-2-review/nas.exe`，21,334,016字节，SHA256 `2d8c2ed03b85e9b4c1ddbcca87cda119df44f2f17cab726d509abcf056d3873a`。与实施者 stable 后端一致，UI 最小修复通过独立前端 build 和资产摘要绑定。
- `s02-2-review-account`：75项，passed=true、trueSchema4Migration=true、accountMutationAPIs=true、offlineCLI=false、targetNAS=false，进程exit0。
- `s02-2-review-core`：run-manifest passed，52项，真实子验证exit0、未超时/无cleanupError；五媒体、用户状态、FFmpeg、SQLite一致快照及5GiB稀疏HTTP偏移。自有服务最终Job回收exit1不代表崩溃，不替换子验证exit0结果。
- 主线另执行 `s02-2-account-baseline`，本轮静态核对注册/文档并读回其 manifest：account 75项passed，其余8组not_run，产物摘要一致；不将同一账号验证重复汇总成150项。
- `s02-2-review-browser-final`：138项，122真实/14受控/2资产；cleanupFailed=false、frontendUnchanged=true，前端清单SHA256 `3194b81659ed5e2a4bb045cea3460c243a6c362688c55387a14e2eee9759111a`。目视范围与失败根详见独立浏览器报告。
- 浏览器故障注入根：`s02-2-review-browser-failure-secret`、`s02-2-review-browser-cleanup-failure`、`s02-2-review-browser-context-cleanup-failure`。三类都正确以exit1/failed结束并回收自有app，不加入正常通过断言数。

主要命令均经 `python -B scripts/project_env.py --`：Go临时测试、vet、build、删临时源后全包test、web typecheck/build、实际 account/core/browser verifier；没有使用用户级环境变量或 `go env -w`。

## Verification

- Lint：pass。四个账号相关 Go 文件 gofmt 无待改；`go vet ./...` pass；局部 `git diff --check` pass（只显示既有 LF/CRLF 提示）。证据 `s02-2-review-vet.log`。
- TypeCheck：pass。`npm --prefix web run typecheck` 及生产 build（含 vue-tsc）成功；Python 最终 AST、导入及本轮 browser 脚本全部函数入参/返回标注检查通过。证据 `s02-2-review-web-typecheck.log`、`s02-2-review-web-build.log`。
- Tests：pass。上述六个临时行为测试、54个生产 TS 边界、75真实HTTP、52核心、138浏览器与三类故障注入各自通过。删源后的 `go test ./... -count=1` 全部 no test files，仅计作编译证据；日志 `s02-2-review-compile.log`。

## Findings (not fixed) / 待验边界

没有剩余的 S02.2 确定业务缺陷。主线已在 frontend 契约同步“空 model 同值清理也须重置显示状态”及对应浏览器回归，已静态读回确认；正式 spec、`docs/verification.md` 与任务状态由主线统一编辑。

S02.3尚未实施；真实Linux/NAS锁/终端/文件系统、真实外部账号与容器、长媒体性能本轮未验。5GiB为稀疏HTTP偏移夹具，不证明真实大视频性能。未重跑的其它baseline组仍是not_run。本轮只改局部账号组件、浏览器verifier及本报告/浏览器报告，不修改正式spec、验证总表、任务元数据，不覆盖其他代理修改，未提交或部署。
