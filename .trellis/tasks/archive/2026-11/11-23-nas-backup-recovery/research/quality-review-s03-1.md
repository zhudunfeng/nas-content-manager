# S03.1 独立可写复核报告

2026-11-23，Windows 本机，Trellis Phase 2.2。实施者停写后开展独立代码路径复核、故障行为测试和新产物集成验收；本报告记录 checker 自行检查与修复后的结果，不沿用实施者旧产物的通过计数。

## 范围与所有权

- 对照本任务 `prd.md`、`info.md`、`check.jsonl` 引用规范、`research/snapshot-service-design.md` 和 `implementation.md` 复核 S03.1。
- 复核范围为共用快照租约、只读检查、Windows/POSIX 权限分支、原备份 HTTP 接口、当前 schema 常量和必要的收尾接缝，以及 `scripts/verify_backup.py`。
- checker 唯一产品修改位于 `internal/app/backup.go` 的 `databaseSnapshot.checkMembers`；另新增本报告。没有修改正式 spec、API/verification 文档、任务状态或父路线。以上正式同步由主线负责。
- 保留 S01/S02 和用户既有修改；未提交、部署、归档、操作用户运行实例或 Docker。当前 schema 仍为 **5**；未实施 S03.2—S03.4。
- 构建、测试和临时命令均经 `python -B scripts/project_env.py -- ...`，缓存及临时目录位于仓库 `.runtime/`。未设置用户级环境变量或使用 `go env -w`。
- 优先使用 CodeGraph，并同步索引；确认 `createDatabaseSnapshot` 的产品调用者为 `media.go` 的备份路径，没有新增第二套生成器或互斥。

## Findings (fixed)

### 未知目录成员造成无界枚举分配

- 文件：`internal/app/backup.go`，`databaseSnapshot.checkMembers`。
- 问题：原 `directory.ReadDir(-1)` 会先把目录全部成员装入内存，再判断是否有未知成员。合法目录最多只有一个候选文件；出现大量未知成员时，无须完整枚举即可拒绝，该实现却可能产生与成员数线性增长的临时分配。
- 修复：改为 `ReadDir(2)`，第二项足以判定拒绝；仅将 `io.EOF` 视为正常枚举结束，仍检查真实读取错误与目录关闭错误。未知成员或身份替换继续保留现场，Close 无论成功失败都最终释放互斥，重复 Close 返回相同结果。
- 独立反例：目录中放入 **2048 个未知成员**，单次成员检查实测 **39 次分配**；清理拒绝后 **2049 个成员全部保留**，重复 Close 幂等且可再次取得备份锁。39 是本机该用例的实测值，不作为跨平台固定分配次数承诺。
- 证据：`.runtime/verify/s03-1-review-behavior-final/go-test.jsonl` 中 `TestS031ReviewBoundedMembers`；同根 `result.json` 记录测试通过。

## 其它已复核路径

- 唯一 `backupMu` 覆盖生成、校验、HTTP 下载、文件关闭和清理；失败路径及并发幂等 Close 最终解锁。清理仅针对本次确认身份的精确成员及空目录，不使用 `RemoveAll`，不扫描删除未知前缀。
- Windows 使用真实 protected DACL，主体限制为当前用户与 SYSTEM，并拒绝重解析点；POSIX 权限分支分别为目录 0700、文件 0600。Linux 实际权限行为仍待目标实测。
- 候选生成使用带 context 的 `VACUUM INTO`；只读检查使用正确转义的 `file:` URI、`mode=ro&immutable=1`、单连接及固定连接，不启动应用、迁移或恢复任务。
- 检查实际当前 schema、核心结构、精确页长度、恰好一行 `integrity_check(1)=ok`、零行外键检查，并处理 Rows/Conn/DB 的关闭错误。SQL 检查连接关闭后，固定 64KiB 缓冲流式摘要；同一只读文件句柄归零后供 HTTP 使用。
- 候选 Sync/Close、打开前后与摘要后的文件身份、长度、mtime、目录成员及取消检查均有失败封闭路径。原 GET/HEAD 鉴权、响应头、文件名、ServeContent 和状态映射保持；未新增媒体 WriteTimeout。
- 日志只输出固定安全分类，不透传原始 SQLite 异常。快照为 **database-only**；数据库本身包含敏感索引、口令/会话哈希和私密引用，须私密保管。摘要不证明可信来源，单文件快照不等于整栈或异地灾备。

## 独立临时行为测试

最终通过 **8 个顶层测试组、22 个子场景**；30 个 pass 事件不能表述为 30 个独立断言。最终结果为 `.runtime/verify/s03-1-review-behavior-final/result.json`，详细事件为同根 `go-test.jsonl`。

1. 真快照元数据、竞争 busy、8 线程并发 Close 幂等和再次生成。
2. 2048 个未知成员的有界枚举、全体成员保留、清理失败幂等及锁释放。
3. 移走原候选后放入真实不同文件身份的替换文件，确认拒绝清理并保留替换现场。
4. 目录/文件权限、Sync、Close、删除文件、删除目录、余量查询错误及空间不足共 8 类故障；确认安全错误、注入诊断不泄漏、锁释放，空间不足映射 507。
5. 真实 SQLite 的合法及 expected 匹配、真实 schema 1、未来 schema 6、伪 schema 5、缺核心表、尾随垃圾、截断、坏页、真实外键孤儿、错误 expected 长度及摘要；使用中文、空格、`#`、`%` 文件名。前后字节/mtime/成员不变且无 sidecar，成功游标归零。
6. 预算/页尺寸溢出封闭、固定 64KiB 摘要、首次读取后的取消、短/超长及预取消。
7. 真实 64MiB 库：观察到 VACUUM 候选已有内容后取消，确认无候选遗留、锁释放及再次成功；实际完整性 SQL 调用中定时取消，随后同连接完整检查成功。没有精确 SQLite 进度回调探针，不声称在特定页位置触发取消。
8. Windows 读取真实 protected DACL 主体全集；实际撤销写权限后 O_RDWR 被拒，而只读 checker 成功；恢复权限、清理和解锁成功。

测试原始日志先在内存检查注入秘密和 fixture 临时根路径，再脱敏落盘；结果 `rawSecretScanPassed=true`。临时测试源码执行后已精确删除：

| 临时源码 | 删除前 SHA-256 |
| --- | --- |
| `backup_s031_review_temporary_test.go` | `c9310c0154be0d043ea72c53fa61699c6f807a9838e020c82bc2bca89438c9fa` |
| `backup_s031_review_windows_temporary_test.go` | `62257591339f0760a57aaca85caa15e083dd9779a8d0f4316c2724c460d386f5` |

较早行为测试结果保留在各自根，不覆盖、不与本轮最终 8/22 累加。清源后 `go test ./...` 退出 0，各包 `[no test files]`，仅作为包编译证据；保留可复用集成脚本。

## 修复后的 fresh 产物

实际 Go 工具链为 **Go 1.27.4 windows/amd64**，`go.mod` 语言版本为 1.26.0。以下三份在临时测试清源后重新构建，目录为 `.runtime/bin/s03-1-review-final/`：

| 文件 | bytes | SHA-256 | 实际运行 |
| --- | ---: | --- | --- |
| `nas.exe` | 21,551,616 | `49754b278cfb26c3725efbcf2f45d462fbda72617eb7442859f5a21e0b40e790` | Windows 本机 |
| `nas-linux-amd64` | 21,350,440 | `d79616cb6048eee9c08233f053e24b30a3e712c6a92ccd27017d828a04e23ea8` | 否，仅交叉构建 |
| `nas-linux-arm64` | 20,060,219 | `899f2bffac5a5a2bbbacdee15cdfd89b0cdd81d8d88f8b8e7d3450a80622d834` | 否，仅交叉构建 |

### 交接时产品源码摘要

以下为报告收尾时从磁盘实际读取的 SHA-256，便于主线核对交接后源码；不是 Git 提交摘要。

| 文件 | SHA-256 |
| --- | --- |
| `internal/app/backup.go` | `2e16f5a4394ed338d5d85b1eae4c61b52f3c32802ee44733bccdbdbf50484b6a` |
| `internal/app/backup_verify.go` | `5ce603e41958a5f253e64f180a95e5fa0ddcf6c9ceed071432cfd1fea5e68d5f` |
| `internal/app/backup_permissions_windows.go` | `f95f426288d2c918019763a74d0cc4130520af361ad06a477091c384ac222c08` |
| `internal/app/backup_permissions_unix.go` | `4b09fbad15d3f5563abcdd3d2f33b3490e8824573b2695b120c8b66786b92877` |
| `internal/app/media.go` | `9caf185da8da2243a4bc2a5a78a6235680835728d5196ce2357ea3df79d77bc3` |
| `internal/app/store.go` | `6a5b70cd25505ebaec51d230a15706ae5048e2ad54ec76530b2a8e38f7ac25c5` |
| `internal/app/server.go` | `f0933664f63e1acc072e56d2a2c121eeb5ccc13946083f85b3d653b29b6138e5` |
| `scripts/verify_backup.py` | `b88f4c4d34d0ac969f4f026488b3daaaf82fa709f3b516ecb658912f61b6d023` |

## 同一 Windows 新产物的真实集成验收

三组结果均已读回，产物大小和 SHA-256 与上表 `nas.exe` 相同。使用全新验证根，没有覆盖旧证据或累加实施者旧 SHA 的重复断言。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| backup | 通过 **38 项**，验证器退出 0 | `.runtime/verify/s03-1-review-backup-final/result.json`、`assertions.json` |
| core `--large-file` | 通过 **52 项**，验证器退出 0；其余 8 组 `not_run` | `.runtime/verify/s03-1-review-core-final/run-manifest.json` |
| account `--offline` | 通过 **157 = 在线 83 + 离线 74**，验证器退出 0 | `.runtime/verify/s03-1-review-account-final/result.json`、`offline/result.json` |

### backup 与正常停服证据

- 两份真实 16MiB 负载 SQLite 快照各为 17,162,240 bytes；schema 5、完整性、外键、精确页长、摘要和业务字段原子关系通过。生成时有真实收藏/进度提交，但源库为单连接，其他 SQL 可能等待，不声称 SQL 真正并行执行。
- GET/HEAD 鉴权及兼容响应头通过。真实慢 HTTP 下载保持租约，第二 GET 立即 409；断连后清理完成，随后再次生成成功。
- `applicationStarts=3`、`normalStops=3`、`normalStopMechanism=isolated-console-CTRL_C_EVENT`、`forcedApplicationReclaims=0`、`cleanupFailures=[]`。
- 三次均为自有隔离 Console 的真实 CTRL_C_EVENT 正常停服。其中慢下载期间停服后，DB 和锁可零共享独占打开、旧监听释放、独立哨兵存活，同根立即重启且原 Cookie 仍有效。S03.1 不撤销会话；恢复前撤销属于后续显式离线准备。
- 未知 `.backup-pending-sentinel` 保留，本轮候选清理。精确 review 产物路径的活应用进程盘点为 0。
- 原始应用双流先检查密码、Cookie、hash、fixture 根及 JSON 转义根，再脱敏落盘，`rawLogsCheckedBeforeSanitize=true`。数据库、下载快照等秘密材料只留本地受限验证根，不应整体上传。

### core 与账号证据边界

- core 使用真实 FFmpeg 及五类媒体，`largeSparseFixture=true`。5GiB 稀疏 Range 只证明 HTTP 偏移，不能代表实际解码或 NAS 吞吐。
- core 的应用角色进程由 OwnedProcess 回收，其 manifest 记录该进程 exit 1；本组验证器 exit 0、52 项证据完整。**不能将 core 的 Job 回收称为正常停服证明**；正常停服证据来自上述 backup 的三次真实 Console 中断。
- account 的 `realApplication`、`trueSchema4Migration`、`accountMutationAPIs`、`offlineCLI` 及 `offlineVerification.requested/passed` 均为 true。嵌套离线计数 74 与总数一致。
- 同次离线验收含 Windows Console 11 场景，按场景核对终端模式恢复及原始双流/可见 viewport 扫描，真实 ACL 写拒绝后完整恢复。目录 junction 同锁通过；文件 symlink 权限不可用，不能将 junction 替代证明当作真实文件 symlink 验收。

## Verification

质量检查结果：`.runtime/verify/s03-1-review-checks-final/result.json`，对应日志位于同根。

- Lint：通过；gofmt 无待格式化文件，`go vet ./...` 退出 0，`git diff --check` 通过。既有 LF→CRLF 提示不作为失败。
- TypeCheck：通过；本复核会话实际运行 `python -B scripts/project_env.py -- npm --prefix web run typecheck`，退出 0，结果文件记录同次会话证据说明。
- Tests：上述独立临时行为测试及同新产物 backup/core/account 均通过；清源全包编译退出 0。
- Build：Windows、Linux amd64、Linux arm64 均构建成功；后两者 `runtimeTested=false`。
- 脚本：`scripts/verify_backup.py` 的 Python AST 检查通过。
- Trellis：`task.py validate .trellis/tasks/11-23-nas-backup-recovery` 的 implement/check 各 17 个上下文引用均有效。
- 收尾：本轮两份 `backup_s031_review_*test.go` 已清除；没有 review 产物的活应用进程。没有停止或回收用户实例。

## Findings (not fixed)

本次已执行范围内没有剩余确定产品缺陷。以下保留为未验或后续范围，不记为通过：

- 真实 Linux/NAS 上权限、进程/信号、挂载和多卷行为；真实 FULL/IOERR、拔盘、断电和生产级大库性能。交叉构建、故障注入及本机 64MiB 用例不能替代这些证据。
- ACL/0600 不防御同用户或系统权限攻击者；成员/身份异常按约定保留现场。强杀或断电可能遗留候选，S03.1 不提供按前缀自动清扫或持久备份恢复承诺。
- S03.2 持久策略/发布/对账/轮转、S03.3 离线 CLI/独立恢复准备/inventory/通用用户资产、S03.4 隔离恢复仍未实施。正式 spec/API/verification 与任务状态由主线同步，本报告不提前宣称 S03 整体完成。

checker 完成上述报告与证据读回后停写，交回主线。

## UTC 时间记录

- core 结果原有 `finishedAt` 为 `2026-11-23T04:23:33.947+00:00`，直接来自 `.runtime/verify/s03-1-review-core-final/run-manifest.json`。
- 本轮 behavior/checks/backup/account 及嵌套 offline 的结果 JSON 格式未保存 `finishedAt` 字段；不从文件 mtime 伪推运行结束时间，也不回写旧证据补造时间。
- 本报告最后的 `finishedAt` 是交接定稿时读取现场 UTC 时钟生成的独立复核收尾记录，不冒充上述各验证器的结束时间。

复核交接 finishedAt（UTC）：2026-11-23T04:31:14Z。此刻定稿并停写交回主线。
