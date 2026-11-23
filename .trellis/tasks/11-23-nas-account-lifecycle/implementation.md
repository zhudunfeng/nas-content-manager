# S02 实施记录

日期：2026-11-23。当前仅交付 **S02.1 存储与认证竞态防线**；S02.2 在线改密/会话管理和账号安全 UI、S02.3 离线重置尚未交付。本文件区分实际执行、源码检查和待验范围，不代表整项 S02 完成。

## S02.1 实际改动

- `internal/app/account_store.go`：新增 schema 5 增量字段、历史会话独立随机补号、唯一与格式/枚举约束、账号快照条件建会话、普通连接/事务共用的精确会话读取与保守清理。
- `internal/app/store.go`：最大支持版本 5，在独立事务中执行增量迁移及版本推进；迁移失败回滚，未来版本拒绝。未修改 schemaV1–V4。
- `internal/app/auth.go`：setup/login 传递实际 bcrypt hash/revision 快照；比较成功后用单条条件插入核对 ID/hash/revision，零行时返回 409 且不设 Cookie。新会话独立生成公开 ID 和 bearer，记录真实创建时间及有限客户端类别；鉴权核对绑定修订和精确到期时间。
- `scripts/verify_schema.py`：从冻结历史迁移真正构造 schema 1–4，可将隔离来源库的历史字段逐表复制并校验行数、字段内容、完整性与外键；不采用仅降低 user_version 的伪旧库。
- `scripts/verify_external.py`：停服并 checkpoint 后构造真正 schema 3 副本，保留用户、旧会话、原来源，再实际升级到 5；原保留断言仍在，增加真实旧结构断言。
- `scripts/verify.py`、`verify_collection.py`、`verify_engine_bootstrap.py`：仅同步精确 schema 5 断言及说明，不放松数据保留断言。
- `scripts/verify_account.py`：可复用真实 HTTP 集成入口；验证旧弱/中文密码和 Cookie 的迁移、重启、新建会话、修订失效及首次设置/退出。另有十二轮退出/重新登录、二十次重复设置、匿名/跨站/限流拒绝的实际状态断言，连接重置直接作为失败。使用基线的独占进程树、随机就绪挑战、严格新运行根与有界脱敏日志，不连接真实账号或外部服务。

schema 5 保留所有原用户/hash/会话数据。旧会话 `created_at=NULL`、客户端 `unknown`、修订 0；公开 ID 为独立 128 位随机十六进制，原 bearer 摘要和 expires_at 原文不改写。账号字段是私有结构，不能直接作为公开 DTO。

## 后续切片可复用的原语

```go
newSessionID() (string, error)
migrateAccountSchema(ctx context.Context, tx *sql.Tx) error
saveAccountSession(ctx context.Context, db *sql.DB, account accountSnapshot, session storedAccountSession) error
readAccountSession(ctx context.Context, reader accountRowReader, tokenDigest string, timestamp time.Time) (authenticatedAccountSession, error)
deleteExpiredAccountSessions(ctx context.Context, db *sql.DB, timestamp time.Time) error
```

- `accountSnapshot` 保存私有 `user/passwordHash/authRevision`；`authenticatedAccountSession` 保存实际公开 `sessionID` 和解析后的 `expiresAt`。
- `accountRowReader` 的 `QueryRowContext` 同时由 `*sql.DB` 和 `*sql.Tx` 实现。敏感操作进入短事务后必须传事务本身，单连接下不能转用 db 等待另一连接。
- `readAccountSession` 要求会话绑定修订等于当前用户修订，并用 `time.Parse(RFC3339Nano)` 加 `After` 精确判断；等于到期即无效，非法时间无效，不能用可变小数字符串排序。
- `saveAccountSession` 只在用户 ID/hash/revision 均匹配时插入；`RowsAffected()!=1` 返回 `errAccountChanged`。bcrypt 不在数据库事务内运行。
- 清理只删除前一 UTC 日期以前的记录，保守保留近时过期行，避免把同秒未到期会话误删。此清理不是鉴权依据。

浏览器测试可导入 `AccountHarness(binary, area, assertions)`，构造后把实际 `web/dist` 用 `copytree(..., dirs_exist_ok=True)` 复制到 `area/web`；保留其随机探针，`start()` 后读取 `client.base`，在 `finally` 中 `stop()`。同对象 stop/start 保留内存 Cookie；重复 start 明确失败。`sqlite3` 已在模块级导入，导入及返回类型解析已实测。

## 已执行的确定性 Go 测试

临时源 `internal/app/account_s02_temporary_test.go` 已在测试完成后删除，只清理本次生成源文件。实际输出保留于 `.runtime/verify/s02-1-go-checks.log`。

7 个顶层测试全部通过，其中迁移测试含真正 v1、v3、v4 三个子例：

1. 增量迁移保留旧中文 bcrypt、未来及过期会话摘要/原到期值、用户、库、媒体、任务、收藏/标签/进度和来源，旧未到期会话仍可鉴权，重启不重复迁移。
2. bcrypt 预校验后分别改变 hash、revision、两者，旧快照均返回 409、无 Set-Cookie、无新增会话。
3. 同秒可变小数、精确到期、纳秒边界、非法时间，保守清理与修订失效。
4. 新 Cookie/公开编号/客户端枚举及数据库约束，空值/越界修订/非法 ID/原始客户端值被拒绝。
5. 索引冲突使迁移在 ALTER 后失败，确认版本、字段、hash、旧会话全部回滚。
6. schema 6 启动拒绝，原版本/账号/会话不变。
7. 单连接真实事务内读取有效会话，并立即观察同事务推进修订后的失效。

命令均通过项目环境包装器运行：

```powershell
python -B scripts/project_env.py -- .tools/go/bin/gofmt.exe -w internal/app/account_store.go internal/app/auth.go internal/app/store.go internal/app/account_s02_temporary_test.go
python -B scripts/project_env.py -- go test ./internal/app -run TestS02 -v -count=1
python -B scripts/project_env.py -- go test ./... -count=1 -v
python -B scripts/project_env.py -- go vet ./...
python -B scripts/project_env.py -- go build -o .runtime/bin/s02-1/nas.exe ./cmd/nas
```

gofmt/vet/build 均通过。删除临时源后的 `go test ./...` 也通过，但其全部包为 no test files，仅属于编译证据，不能替代上面的确定性测试结果。Python AST 语法检查和 `AccountHarness.database` 导入类型解析通过。局部 `git diff --check` 通过，仅有仓库既有 LF/CRLF 提示。

## 真实本机集成结果

所有运行根都位于忽略目录 `.runtime/verify`，成功与失败均保留，未复用或覆盖历史根。数据库/夹具凭据仅用于回环实例，不能整体公开上传。

- `s02-1-account-1`：35 项通过；随后将日志说明纠正为“可分享脱敏日志”，并在最终脚本增加两项迁移后 SQLite/FK 完整性断言。
- `s02-1-account-3`：37 项通过，含真正 schema 4 旧弱/中文密码、旧 Cookie、新会话安全元数据、重启、修订撤销与首次设置。
- `s02-1-account-final`：以最终重建产物再次通过 37 项；结果含产物 SHA256、truthfulness flags，明确在线变更 API、离线 CLI 和目标 NAS 尚未验收。
- `s02-1-account-final2`：增加小正文拒绝稳定性覆盖后通过 42 项；连续二十次重复 setup 409，另验证匿名 401、跨站 403、错误登录 401/限流 429 和无写入副作用。
- `s02-1-external-1`：84 项通过，包含真实 schema 3 迁移与完整受控 MoviePilot 工作流。
- `s02-1-external-2`：最终产物再次通过 84 项；真实外部实例/真实媒体写入/第三方工具安装均为 false。
- `s02-1-external-3`：包含匿名/限流早拒绝修复的后续产物再次通过 84 项。
- `s02-1-core-1`：失败，记录保留；定位与修复复验见下节。
- `s02-1-core-2`：52 项通过，五媒体、状态保存、真实 FFmpeg、SQLite 一致备份与 5GiB 稀疏 HTTP 偏移均完成。
- `s02-1-core-3`：失败，新的失败记录保留；在音乐收藏/搜索后触发重扫的成功响应也出现同类未读小正文 reset，由主线继续处理。
- **最终统一产物** `.runtime/bin/s02-1-stable/nas.exe`：21,260,800 字节，SHA256 `be86e7447505884e18ebacbc66d6f6fb3a85759e4f1a3970b5d3424e77a742f6`。下列三次最终回归均在公共层最终策略落地并重新构建后，以全新运行根执行；不能用上面的中间结果代替。
- `s02-1-account-final3`：**43 项通过**；含十二轮真实 logout→401→login→200、小正文重复拒绝、真正 schema 4 旧弱/中文密码迁移。证据 `result.json` 和 `assertions.json`；产物标识与上述 SHA256 一致。
- `s02-1-external-4`：**84 项通过**；真正 schema 3 迁移及受控四类外部 HTTP/完整 MoviePilot 工作流全部完成。证据 `result.json`；真实外部实例、真实媒体写入、第三方工具安装仍为 false。
- `s02-1-core-4`：**52 项通过**；`run-manifest.json` 明确 `status=passed`、结束时间、相同产物 SHA256 与子验证正常退出，证据 `evidence/core-assertions.json`、`logs/core.log`。五媒体、用户状态、实际 FFmpeg、SQLite 一致备份和 5GiB 稀疏偏移均通过；本次未选其它基线分组，不代表其重新执行。

实际集成命令：

```powershell
python -B scripts/project_env.py -- go build -o .runtime/bin/s02-1-stable/nas.exe ./cmd/nas
python -B scripts/project_env.py -- python -B scripts/verify_account.py --binary .runtime/bin/s02-1-stable/nas.exe --root .runtime/verify/s02-1-account-final3
python -B scripts/project_env.py -- python -B scripts/verify_external.py --binary .runtime/bin/s02-1-stable/nas.exe --root .runtime/verify/s02-1-external-4 --workflow
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases core --artifacts .runtime/bin/s02-1-stable --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe --ffprobe D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffprobe.exe --large-file --root .runtime/verify/s02-1-core-4
```

最终产物构建前再次执行 `go vet ./...`、`go test ./... -count=1` 与本片三个 Go 文件的 `gofmt -l`，均成功且无格式待改。此次包测试全部为 `no test files`，仅说明清除临时测试源后仍可编译，不增加行为测试数；行为证据仍为上述七个确定性测试及三组最终真实 HTTP 回归。六个变更 Python verifier 的 AST 和 `AccountHarness.database` 导入类型解析再次通过。

HTTP verifier 只检查可分享脱敏日志的持久内容；不能从已脱敏日志反推原始 stdout 没有泄漏。当前新增账号日志逐处源码复核仅含固定中文和 userId，未传入密码、hash、token、原 UA 或 IP；未来若需动态原始输出秘密扫描，应增加排空前的观察机制。

## 核心回归暴露的存量 HTTP 问题

`core-1` 在前三项通过后，于原有跨站 POST `{}` 断言收到 Windows 10054。应用没有 panic；服务 exit 1 是 finally 回收自有 Job 的结果，不能据此认定服务崩溃。额外隔离诊断证明每次异常后进程仍存活、health 都为 200：

- `s02-1-origin-diagnostic-1`：修改后的账号产物 20 次中 12 次 reset、8 次 403。
- `s02-1-origin-old-artifact-1`：S01 旧产物、schema 4，20 次中 1 次 reset、19 次 403，证明分支为存量行为。
- 源码对应 `security` 在读取小正文前直接 403；urllib 分开发送 header/body 并请求关闭连接，产生未读体关闭竞态。没有把 reset 当成 403 通过，也没有修改 verifier 放松断言。
- 第一版由主线在 `server.go` 的小正文拒绝前有界丢弃，限制读取 deadline；这个按拒绝分支调用的中间方案后来被下述统一 defer 替换。
- `s02-1-origin-fixed-1`：对主线修复产物原样重复 20 次跨站 POST，全部真实 403，进程始终存活且每次 health 200。随后才重建最终片产物并重跑 account/external/core。
- 中间还曾在匿名 401 与 login 早限流 429 分支增加相同处理；这些逐分支调用现已移除。setup 409 均在完整 decodeJSON 之后，连续二十次真实 409 已验证。
- `core-3` 暴露扫描成功 202 也可能未消费传入的 `{}`，说明要覆盖成功且不使用正文的路径，不能仅处理错误分支。最终在 `security` 仅注册一次 `defer discardUnreadSmallBody(w, r)`，成功和错误响应共用，handler 不再分别调用。
- 最终 helper 只处理 `0 < ContentLength <= 64KiB` 且无 Expect 的已知长度正文；保留原始流读取位置，仅在一秒局部读 deadline 下丢弃剩余字节，不解析、不保存。跳过未知长度、大正文、Expect 与不支持 deadline 的环境；不包装 ResponseWriter/ReadCloser，不改变原生 Close 或媒体全局写超时。
- handler 已经完整或部分读取后的普通 EOF 属于正常完成，成功才恢复读 deadline；截断/超时/其它读取错误则关闭连接并保留已到期 deadline，防止后续原生 `Body.Close` 再次阻塞。主线额外十个顶层临时测试实际通过并清源，证据 `.runtime/verify/s02-1-http-finish-go.log`、`s02-1-http-keepalive-go.log`；覆盖真实分开发送 TCP 的成功/拒绝、正常连接复用及 keep-alive 慢体。此测试数与本片七个账号测试独立。
- 上述最终统一产物已用新根重跑 account/core/external，结果分别为 43/52/84 项通过；所有早期失败根继续保留，不将 TCP reset 换算为期望 HTTP 成功。

## 未实现或待验

- 在线改密、会话分页/撤销、再认证限流、严格字段别名/重复检查、账号 UI、离线 CLI 均属于后续切片，不能视作已交付。
- 本次实际运行平台为 Windows AMD64；Linux 交叉构建不等于 Linux 内核锁/终端/NAS 实测，本片未声明这些结果。
- 真实 NAS、Docker/BT、外部账号、完整浏览器账号页、真实性能/长媒体传输尚未验收。5GiB 稀疏夹具仅证明 HTTP 偏移。
- 正式 spec、验证总表、任务状态和后续切片由主线维护；本片不自行推进整项 S02 完成状态，不提交、不部署。
- 本片实现与最终回归完成后停止业务写入，所有权交给 checker 独立复核；后续 S02.2/S02.3 仅在主线另行分派后推进。

## S02.2 在线账号 API 与局部 UI（2026-11-23）

本节在 S02.1 通过独立复核、主线明确授权后追加。上面的“未实现”是 S02.1 当时边界，保留为历史记录；本片已实施在线 API 与 UI，**未实施 S02.3 离线 CLI**。正式 spec、任务状态、验证总表及真实浏览器脚本由主线维护，独立 checker 尚需复核本片。

### 本片文件与实现

- `internal/app/account.go`：注册四个账号端点；实际 Cookie 读取完整私有快照；账号专用严格 token JSON 解析，拒绝大小写别名、转义后重复键、未知/缺失/空值/类型和尾随数据。保持原全局 decodeJSON 行为与公共 HTTP 正文收尾不变。
- `internal/app/account_store.go`：增加有限公开 DTO、事务内账号优先重核、改密 CAS 与全会话撤销、本人范围幂等单撤销、再认证撤销其他，以及有界 keyset 查询。沿用 schema5，无新迁移。事务不跨 bcrypt；先核对账号 ID/hash/revision，变更409；快照未变但会话撤销/到期401；写后提交前再拒绝上下文取消与精确到期。修订最大 int64 拒绝写入，不溢出。
- `internal/app/server.go`：仅为本片增加独立 accountLimits 字段和 accountRoutes 注册，不改媒体 auth 上下文或共享生命周期。`auth.go` 仅修正新密码提示注释适用范围。
- 再认证使用独立短锁同时预占账号12次/5分钟和实际会话6次/5分钟；两个端点共享，成功/失败均计数，换会话/成功登录不清账号预算。拒绝不增加计数、不延长窗口；两个 map 总键数上限2048，清到期键，满时保守429；不持锁执行 bcrypt。Retry-After 为剩余时间向上取整1–300秒；不声明跨重启持久化。
- 列表 DTO 精确为 `{currentSession,items,nextCursor}`；单会话 `{id,current,clientBrowser,clientOS,createdAt,expiresAt}`。当前固定身份来自实际 Cookie；其他会话按公开随机编号升序，不声称最近登录排序。仅查 limit+1 原始索引行，消费前 limit（默认20，1–100），Go 用同一 asOf 过滤，空批次不追页；有 lookahead 时游标为最后消费原始ID，已删除边界仍可续。旧创建时间保持 null；时间输出 RFC3339Nano。无 token/hash/IP/raw-UA。
- `web/src/components/AccountSecurity.vue`：设置页同级局部 panel，两个独立表单、四个独立 label/PasswordInput、提示关联、当前固定卡和当前批次其他卡、手动继续/从头刷新、单个原生撤销当前确认框。读写忙碌/错误分别保存，敏感写一次一个；AbortController+generation 防迟到读回写，卸载清密码并取消请求。实际敏感请求 finally 清四字段并恢复掩码；409只刷新只读状态，未知网络结果不重发；400原密码错误不触发全局退出；成功退出复用原 App 清理。
- `web/src/App.vue`：设置页按需加载组件，新增成功重新登录通知入口。`types.ts` 增加公开 DTO，`password.ts` 仅注释复用范围；`api.ts` 非破坏性增加可选 `ApiError.retryAfterSeconds`，401处理保持不变；局部样式位于组件，不重设计全站。
- `docs/api.md` 固定上述精确契约；`scripts/verify_account.py` 由存储基础回归扩展为稳定的真实 HTTP 在线账号验证，保留隔离进程/新根/真实旧库及失败即失败的安全边界。

### 临时确定性 Go 与实际 TypeScript 规则验证

已生成并执行 `internal/app/account_s022_temporary_test.go` 的 **9 个顶层测试**，最终输出保存在 `.runtime/verify/s02-2-go-checks-final.log`；执行成功后已删除该临时 Go 源。覆盖：

1. 两个已准入改密快照通过通道确定顺序：首提交成功、后提交409，旧登录快照无法补建会话；只保留首提交 hash/一次修订推进/零旧会话。
2. 已准入会话被另一个会话撤销后，敏感写401并不修改；重复目标幂等；若账号也发生变化，409先于会话失效。
3. 提交检查精确到期±1ns/等时、已取消上下文导致回滚保留会话、快照后实际会话过期拒绝、int64最大修订拒绝。
4. 105条其他原始行的 limit+1 上界、连续空批次、已删除游标、历史 null、真实当前固定与不重复。
5. 并发预算原子预占、每会话6/账号12、换会话不可绕过、拒绝不计数或续窗、固定窗口精确到期、2048容量不逐出活键。
6. 严格 JSON / Unicode转义别名 / 类型 / 尾随与分页参数矩阵，已知和未知长度的超大正文413。
7. 旧空密码、弱密码、前后空格、中文以及72 UTF-8字节原密码按原值比较；75字节中文在 bcrypt 前拒绝且不占预算。没有给原密码新增非空或复杂度限制。
8. 实际 Go `validNewPassword` 的全部32个ASCII标点、9/10/72/73边界、缺类、中文/补充汉字/全角/空白/控制/emoji矩阵。
9. 在**未经过脱敏的内存 bytes.Buffer 原始业务日志**扫描错误再认证、单撤销、退出其他、成功改密；密码/新密码/错误密码/hash/token/token digest/IP/raw UA均未出现，固定中文业务日志实际出现。HTTP verifier 的已脱敏日志不是此证据的替代。

实际读取并转换 `web/src/password.ts` 执行同样规则矩阵，**51 项断言通过**（含全部32标点）。证据 `.runtime/verify/s02-2-typescript-policy.log`；生成的 `.runtime/verify/s02-2-policy-temp.mjs` 同样已删除。没有用重新实现的独立正则代替生产函数。

### 最终本机产物与真实 HTTP / 核心回归

- 最终候选 `.runtime/bin/s02-2-stable/nas.exe`：21,334,016字节；SHA256 `2d8c2ed03b85e9b4c1ddbcca87cda119df44f2f17cab726d509abcf056d3873a`。与早期 first 产物哈希相同，后端在首次构建后未变化；前端独立 label 修复已包含在当前 `web/dist`。
- `.runtime/verify/s02-2-account-1` 和最终新根 **`s02-2-account-final` 均75项通过**。最终 `result.json` 标明相同 stable 产物、trueSchema4Migration=true、accountMutationAPIs=true、offlineCLI=false、targetNAS=false。覆盖真正v4旧弱/中文密码及旧Cookie迁移、在线原值升级、严格实际HTTP矩阵、输入拒绝库不变、当前与其他公开DTO、空批/删除边界、实际删除数含过期/旧修订/坏到期记录、单撤销与死Cookie重复401、含全部32标点的新密码实际登录、10/72合法边界、改密后重启，以及两端点/换三会话/成功登录不能绕过账号预算。
- 最终新根 **`.runtime/verify/s02-2-core-final`：52项通过**，`run-manifest.json status=passed` 且相同产物SHA，子验证exit0。五媒体实际索引与读取、收藏标签进度、真实FFmpeg、SQLite一致快照、5GiB稀疏Range/HEAD/416/304偏移均通过。没有把自有服务Job回收的exit1当成产品崩溃或验证失败，也没有放松断言。未重跑的其它分组保持 not_run。
- 主线首轮真实浏览器已报告103断言（89真实/14受控呈现），最终 stable 的重跑与截图目视结果以主线浏览器证据/独立复核为准；本片不代主线补称最终通过。

实际主要命令：

```powershell
python -B scripts/project_env.py -- .tools/go/bin/gofmt.exe -w internal/app/account.go internal/app/account_store.go internal/app/auth.go internal/app/server.go
python -B scripts/project_env.py -- go vet ./...
python -B scripts/project_env.py -- go test ./internal/app -run TestS022 -count=1 -v
python -B scripts/project_env.py -- go test ./... -count=1
python -B scripts/project_env.py -- D:/adunsoftware/nodejs/node.exe web/node_modules/vue-tsc/bin/vue-tsc.js --noEmit -p web/tsconfig.json
python -B scripts/project_env.py -- D:/adunsoftware/nodejs/node.exe web/node_modules/vite/bin/vite.js build web
python -B scripts/project_env.py -- go build -o .runtime/bin/s02-2-stable/nas.exe ./cmd/nas
python -B scripts/project_env.py -- python -B scripts/verify_account.py --binary .runtime/bin/s02-2-stable/nas.exe --root .runtime/verify/s02-2-account-final
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases core --artifacts .runtime/bin/s02-2-stable --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe --ffprobe D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffprobe.exe --large-file --root .runtime/verify/s02-2-core-final
```

gofmt/vet/删除临时测试后的全包编译、严格 vue-tsc、生产 Vite build、Python AST、局部 diff --check 均成功。清源后 `go test ./...` 的 no test files 仅编译证据，不计作行为覆盖。首次 gofmt 未提供本地exe路径、两次Vite命令相对路径不正确而失败，随后改用上面准确路径通过；没有因此安装全局工具或更改用户环境。

本片完成自查后停止业务与测试源码写入，交主线和独立 checker。S02.3、Linux/NAS实机、真实外部账号/容器/长媒体性能、S04事件接入仍不在本片完成声明内；没有提交或部署。

## S02.3 离线恢复实现与最终交付（2026-11-23，待独立复核）

### 实现文件与边界

- `cmd/nas/main.go` 将退出码与清理放入 `run()`，先建立系统取消上下文；有参数时在 `LoadConfig`、HTTP 与媒体初始化前进入严格维护分派，无参数保持原服务启动。
- 新增 `internal/app/account_maintenance.go`：精确空格旗标与资源为零的帮助、已有规范数据目录预检、复用原平台 `.nas-instance.lock`、库身份复查、先关库后放锁、固定安全中文输出。0/2/1/130 分别表示成功或帮助、用法、失败、纯取消；提交未确认和已提交后清理失败单独分类，不自动重试，不用取消覆盖清理错误。
- 新增 `internal/app/account_offline_store.go`：正确转义的 `file:` URI + `mode=rw`、单连接、连接级 PRAGMA，完全绕开 `openDatabase`。显式只支持 schema5，检查真实账号表、字段、原触发器和危险外来级联、唯一 ID1 管理员与有效 bcrypt/修订；短事务重新核对结构版本/hash/revision，条件更新并删全部会话。回滚错误保留未知结果，提交返回错误即使实际已经提交也不能承诺“账号未改”。
- 新增 `internal/app/account_terminal.go`：官方 `golang.org/x/term v0.48.0`，同步 MakeRaw、唯一 Restore，恢复结束后才 bcrypt/事务。两提示各1024原始非行结束字节粘性预算、CRLF跨提示、退格与控制/坏UTF-8拒绝。重定向输入仅显式 `--password-stdin`，最多75字节含哨兵，少于上限必须EOF，不trim/截断/只取首行。读协程不改变OS模式，仅用于维护进程退出模型。
- `go.mod/go.sum` 只增加官方 x/term v0.48.0 direct require 与其两个校验和，无其它依赖升级。
- 新增稳定 `scripts/verify_account_offline.py`，仅使用自有隔离进程/库/目录/Console与ACL副本；原始双流有界留内存，先扫描秘密再脱敏持久化；Job在Windows挂起子进程恢复前取得所有权，清理失败不能通过。`scripts/verify_account.py` 以显式 `--offline` 集成，保留默认在线行为、独立离线子结果与准确分项计数。
- 新增 `docs/accounts.md` 操作手册；`docs/api.md` 局部补离线入口/退出码/输入/事务和手册链接。正式spec、baseline入口和任务状态由主线管理，本片不改完成状态。

### 临时确定性 Go 测试与清源

`internal/app/account_s023_temporary_test.go` 最终6个顶层测试全部通过，原输出保留于 `.runtime/verify/s02-3-go-tests-final.log`：

1. 严格解析与stdin边界：空格旗标、未知参数不回显、75字节/EOF、LF/CRLF、内部换行、10/72/73及字符规则。
2. 官方终端适配：CRLF、逐字/退格、确认不符、非法控制/坏UTF-8/空白、超限后退格仍失败、两个Ctrl+C、EOF、1023/1024/1025预算、提示输出和恢复失败；恢复失败与取消并存不能冒称纯取消。这是确定性适配测试，不代替真实Console。
3. 真实SQLite CAS：仅hash、仅revision、同时变更、schema_version、user_version、溢出、成功及旧快照第二次失败。
4. 真实提交后驱动返回错误、真实回滚后返回错误：前者实际已改但必须报告未确认，后者清理错误不能被吞掉；生产代码没有测试开关。
5. 真实UPDATE后故障、DELETE后故障、DELETE后取消：全部hash/revision/session整笔回滚。
6. 实际modernc existing-only URI与含空格/百分号/中文路径，缺库不能误建。

运行完成后已删除仅本次新建临时Go测试源，证据 `.runtime/verify/s02-3-clean-source.log`。之后 `go vet ./...`、`go test ./... -count=1` 和Windows主程序构建通过；清源后各包 `no test files` 仅证明编译，不额外计行为断言。四个生产Go文件gofmt通过。

### 最终 fresh 产物与回归

最终Windows产物 `.runtime/bin/s02-3-stable/nas.exe`：**21,444,096 字节**，SHA256 **`ccec3f0aa05b0f551ff537ea614fa01906a30ccaba3f280e64cf9fefc551e5d8`**。它在最后的事务未确认/纯取消分类修正后重新构建；下列最终结果全部读取到同一产物标识，不以早期 first 产物替代。

- **`.runtime/verify/s02-3-account-final`：144项通过 = 在线75 + 离线69**。顶层 `offlineCLI=true`、`offlineVerification.requested/passed=true`、子计数69；真正schema4弱/中文旧账号迁移和全部在线接口仍通过。离线证据独立保存在 `offline/result.json` 与 `offline/assertions.json`，不重复拼接在线断言清单。
- 同次离线真实进程验证：活实例与Junction目录别名锁拒绝且原实例健康；无效服务配置下仍独立执行维护；真正v4、未来/损坏/缺列/账号异常/溢出/坏hash/恶意trigger/危险cascade、缺库/目录库/链接夹具、20组非法stdin和无旗标管道拒绝均无账号或相关业务写入。合法10/72/LF/CRLF/32标点各自副本成功；最终成功后所有非账号表、running作业、dispatching动作、pending缓存与媒体哨兵保留，SQLite integrity/FK正常，重启旧Cookie及旧密码失效，新密码登录成功。
- 同次**真实Windows Console共11条场景记录**：KEY_EVENT Ctrl+C、系统 `CTRL_C_EVENT`、Ctrl+D分别覆盖第一与第二提示；另有确认不符、部分管道未EOF时系统取消、真实终端加stdin旗标拒绝、全部32标点逐键+真实退格且两提示分别确认、CRLF两行粘贴成功。交互模式准确503→1008→503；管道/拒绝按各自实际模式记录。退出有界、锁释放、原始stdout/stderr与可见viewport屏幕采样无已知秘密；屏幕范围明确只是 `visible-viewport`。
- 同次**真实ACL拒绝已建立**：只修改自有SQLite副本DACL，先用普通 `r+b` 真实打开获得PermissionError，再断言维护exit1/逻辑账号不变/锁释放。finally恢复原DACL与保护状态，再读安全描述符逐字节比对一致；`actualPermissionDenial=true`、`permissionFixture.realWriteDenied/aclRestored=true`。没有修改用户、全局权限或实际实例。
- **`.runtime/verify/s02-3-core-final`：52项通过**。`run-manifest.json status=passed`，相同SHA/大小，子验证exit0；五媒体/个人状态、真实FFmpeg、SQLite一致备份、5GiB稀疏Range/HEAD/416/304等通过。稀疏文件只证明HTTP偏移，不能当真实媒体吞吐/解码/大规模NAS性能证据。未选其它baseline分组保持not_run，最终baseline account编排由主线继续验证。

主要复现命令：

```powershell
python -B scripts/project_env.py -- go test ./internal/app -run TestS023 -count=1 -v
python -B scripts/project_env.py -- go vet ./...
python -B scripts/project_env.py -- go test ./... -count=1
python -B scripts/project_env.py -- go build -o .runtime/bin/s02-3-stable/nas.exe ./cmd/nas
python -B scripts/project_env.py -- python -B scripts/verify_account.py --binary .runtime/bin/s02-3-stable/nas.exe --offline --root .runtime/verify/s02-3-account-final
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases core --artifacts .runtime/bin/s02-3-stable --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe --ffprobe D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffprobe.exe --large-file --root .runtime/verify/s02-3-core-final
```

第一个命令是清源前历史复现说明，当前测试源已按项目规则删除。新验证必须换尚不存在的运行根，不覆盖保留证据。

### Linux交叉编译与保留边界

在项目环境包装器启动的当前子进程内设置 GOOS/GOARCH/CGO_ENABLED，没有改用户级环境或 `go env -w`。编译记录 `.runtime/verify/s02-3-linux-cross-build.log` 与 `.json` 均显示exit0、compiled=true、executed=false：

| 产物 | 字节 | SHA256 |
| --- | ---: | --- |
| `.runtime/bin/s02-3-linux-stable/nas-linux-amd64` | 21,251,913 | `be8206ca4420028c1e76702227eb412222249332a64f75d8d4b22d7b5c08ce18` |
| `.runtime/bin/s02-3-linux-stable/nas-linux-arm64` | 19,908,628 | `a2c407a34f22f8ddccb2d8dd0a2b841329b842be6f36149117c6a7e2cabc9090` |

真实Linux/NAS的flock、终端模式、POSIX权限/挂载/文件系统和目标设备仍待验。Windows当前令牌无文件符号链接特权，早期WinError1314失败证据保留；最终 `databaseLinkFixture` 明确为 `directory-junction; file-symlink privilege unavailable`，不可声称真实文件symlink已验。Junction目录别名锁为真实执行。

早期 `.runtime/verify/s02-3-offline-first` 至 fourth失败根未覆盖：分别暴露文件symlink特权、Windows已关闭管道EINVAL、Console ABI/屏幕句柄/标点虚键映射的夹具问题；按实际原因修正稳定验证器，未放松产品断言。fifth67、sixth69为中间通过证据，最终以同次fresh144/69为准。

完成文档/记录读回、Python AST、产物/结果一致性与限定文件diff检查后，本片停写交主线及独立checker。没有提交、部署、操作真实账号/媒体、安装工具或启动Docker；不自行启动S03或把整个S02标为完成。
