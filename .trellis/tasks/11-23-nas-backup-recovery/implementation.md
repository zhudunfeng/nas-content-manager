# S03.1 实施与自查记录

2026-11-23，Windows 本机。实施者已完成本片代码、行为验证与收尾自查，停写后交独立 checker；本记录不代表 S03.2—S03.4 已实施，也不代表目标 NAS 验收、提交或部署。

## 交付范围

- `internal/app/backup.go`：共用快照服务、唯一互斥租约、溢出封闭的空间预算、随机私有候选、Sync/Close、明确所有权与同步幂等收尾。仅删除本次确认身份的精确文件和空目录；未知成员、目录或文件替换保留现场并汇总安全收尾错误。
- `internal/app/backup_verify.go`：独立只读 SQLite 校验、当前 schema 5 与核心结构探针、精确页长度、完整性/外键检查、可选 expected 长度和摘要、固定 64KiB 流式 SHA-256、取消与文件身份稳定检查。全部 SQL 资源关闭后，同一只读文件句柄供摘要及 HTTP 使用。
- `internal/app/backup_permissions_windows.go` / `backup_permissions_unix.go`：Windows 真实 protected DACL 仅当前用户及 SYSTEM，并拒绝所有重解析点；POSIX 目录 0700、文件 0600。没有从 app 反向依赖 desktop。
- `internal/app/media.go`：仅替换备份局部为租约调用，保留原鉴权、状态映射、SQLite Content-Type、文件名、HEAD 和 ServeContent；不把 handler 返回写成客户端下载完整。
- `internal/app/store.go`：仅提取当前 schema 头常量 5 并供启动未来版本检查共用；历史迁移版本不变，未增加迁移。
- `internal/app/server.go`：仅更新 backupMu/cleanupErr 的说明；此前 S02 账号逻辑原样保留。
- `scripts/verify_backup.py`：保留真实隔离应用集成脚本，复用项目新根、进程树与精确就绪机制，增加真实慢 HTTP 下载、并发提交、只读产物、正常停服、立即重启和原始日志先验秘密扫描。最终自查补齐慢连接关闭或线程等待异常时继续执行其余回收，并记录正常退出次数与活应用强制回收次数。

本片无新路由、配置、UI、CLI、定时策略或持久历史。只有一次带 context 的 `VACUUM INTO`，没有复制在线主库、额外业务长事务或另建第二套快照锁。

快照范围为 **database-only**，不包含外部来源秘密文件、helper/worker 身份文件、组件卷、下载载荷、媒体和用户资产原件。**数据库本身含敏感索引、账号口令/会话哈希与私密引用，必须私密保管**；生成摘要不能证明可信来源，更不等于异地灾备。

## 最终产物

Windows 本机实际构建产物 `.runtime/bin/s03-1-stable/nas.exe`：

- 大小：21,552,640 bytes。
- SHA-256：`6915b06f8ab0f46bd4be43218fe1208f2e81e148471b2f4479ac4e4d35c815d7`。
- 下列 backup 38、account 157 和 core 52 均由各自结果 JSON 核对为该产物；没有以旧 S02 产物代替。测试源码删除及最后脚本文字/收尾增强未改变业务 Go 代码或此二进制。

仅交叉编译、未在 Linux 运行：

| 目标 | 产物 | bytes | SHA-256 |
| --- | --- | ---: | --- |
| linux/amd64 | `.runtime/bin/s03-1-linux-amd64/nas` | 21,350,192 | `938d35d4f749fd5084d181c95267f768ec8c424064f82117ad395417e7ca65fb` |
| linux/arm64 | `.runtime/bin/s03-1-linux-arm64/nas` | 20,061,275 | `b9e5a94aa3933c7801e9274d330bae72297990e3fe5e93e91eb448c93c5991de` |

## 实际验证与证据

所有构建、测试及临时命令均通过 `python -B scripts/project_env.py -- ...` 运行；缓存和临时目录限定于仓库 `.runtime/`，未修改用户环境或 `go env -w`。

| 检查 | 结果与计数 | 主要证据 |
| --- | --- | --- |
| 真实备份集成最终重放 | 通过 **38 项断言** | `.runtime/verify/s03-1-backup-release-check/result.json`、`assertions.json` |
| 真实账号完整回归 | 通过 **157 = 在线 83 + 离线 74** | `.runtime/verify/s03-1-account-stable/result.json`、`offline/result.json` |
| core + 大文件 | 通过 **52 项断言**，largeSparseFixture=true | `.runtime/verify/s03-1-core-large-stable/run-manifest.json` |
| 临时 Go 行为验证 | 通过 **8 个顶层测试组、21 个子场景**；29 个 pass 事件不能当作 29 个独立断言 | `.runtime/verify/s03-1-go-behavior-final/result.json`、`go-test.jsonl`、`stderr.log` |
| gofmt / go vet | 通过，gofmt 无待格式化文件 | `.runtime/verify/s03-1-final-checks/result.json` 及对应日志 |
| 删除临时源码后 go test ./... | 退出 0；各包 `[no test files]`，仅作为包编译证据 | `.runtime/verify/s03-1-final-checks/go-test-clean-source.log` |
| TypeScript typecheck | 本轮已执行 `npm --prefix web run typecheck` 并退出 0 | 实施工具会话实际结果；未改前端 |
| Linux amd64 / arm64 | build 退出 0；runtimeTested=false | `.runtime/verify/s03-1-final-checks/result.json` 及对应日志 |
| 脚本语法 / 局部 diff 空白 | Python AST 与 `git diff --check` 通过 | 实施最终工具会话结果 |

core 普通模式也曾通过 44 项，保留于 `.runtime/verify/s03-1-core-stable/`；它与后续 52 项重叠，**不累加计数**。备份旧 35/38 项及最终重放同样各自保留，不累计宣传。

### 真实 HTTP 与正常停服

最终备份结果明确：`applicationStarts=3`、`normalStops=3`、`forcedApplicationReclaims=0`、`cleanupFailures=[]`、`rawLogsCheckedBeforeSanitize=true`。

- 三次正常退出均来自自有隐藏 Console 的真实 `CTRL_C_EVENT`，等待服务实际返回 0；Job 隔离在启动前建立，所有权句柄最后正常回收，**未用 Job 结束仍存活的应用来冒充正常停服**。独立哨兵进程在正常停服时仍存活，最后仅按自有句柄回收。
- 真实 16MiB SQLite BLOB 负载生成两份 17,162,240 bytes 的完整快照。下载分块计算 SHA-256，独立 SQLite 只读检查 schema、完整性、外键、页尺寸、负载数量与业务字段关系；检查前后字节摘要、长度、mtime 和目录成员不变，无校验 sidecar。
- 收藏和进度真实请求在生成期间提交；快照内 position/total/locator 保持同一业务事务关系。源库只有一个连接，其他 SQL 可能等待，未宣称真正同时执行 SQL。
- 限制 TCP 接收窗口后保持真实慢下载，第二 GET 在 2 秒内返回 409；断连后精确候选清理，再次备份成功。
- 慢下载期间正常停服后，数据库和数据锁立即可零共享独占打开、旧监听关闭，同 DataDir 立即重启且原 Cookie 仍有效。S03.1 校验不撤销会话，未来显式恢复准备另做。
- 最终数据根只保留预先放入的未知 `.backup-pending-sentinel`，其内容未被生成/停服删除；本次候选均已清理。精确 Windows 产物路径的进程盘点为 0，无本轮临时 Go 测试源遗留。
- 密码、Cookie、token hash、bcrypt 和整个夹具根路径（含 JSON 转义形式）在原始内存双流检查之后才允许脱敏落盘。完整数据库、下载快照和私密 fixture 仍仅在本地受限验证根，不应整体上传。

### 临时行为测试边界

- 租约竞争 barrier、8 线程并发 Close 幂等及再次生成；负数、页参数和 uint64/int64 乘加溢出封闭。
- 目录/文件权限、Sync、Close、精确删除文件/目录、余量查询与不足故障；每次失败后互斥可重新取得。
- 未知成员保留；Windows 上先将旧候选移出目录，再放入真实不同身份文件，确实命中文件身份替换检查，未只靠未知成员个数替代此断言。
- 合法及 expected 匹配、真正旧 schema 1、未来头、伪 schema 5、缺核心表、空库、截断、尾垃圾、坏页、实际外键孤儿、错误 expected 长度/摘要。各例前后摘要、长度、mtime、目录成员不变。
- 64MiB 真实库在观察到 VACUUM 创建候选后取消，确认清理和再次生成；实际完整性 SQL 运行中取消；摘要首轮取消、预取消。
- Windows 读取真实 protected DACL，确认主体全集仅当前用户和 SYSTEM。目录继承 ACE 的有效/仅继承规范化属于系统正常表示，不错误要求固定两条 ACE。实际撤销写权限后 O_RDWR 打开被拒，而只读 checker 成功。
- 已精确删除本轮 `backup_s03_temporary_test.go` 与 `backup_s03_windows_temporary_test.go`。它们执行时源码 SHA 保存在行为验证结果中；未删除既有可复用集成脚本。

## 失败记录及剩余边界

- 最早一次临时测试 helper 未解析到嵌套 Go 命令，**未执行任何测试**，失败证据仍为 `.runtime/verify/s03-1-go-behavior/result.json`（passed=false、testsExecuted=false）。随后使用仓库 Go 绝对路径实际执行的成功证据另存 final 根，没有覆盖失败记录。
- 最终备份脚本对数据库敏感性的范围文字已修正并在全新 `.runtime/verify/s03-1-backup-release-check/` 真实重放 38 项；较早结果保留原始运行事实，不回写旧证据。
- Linux 权限、进程/信号、实际 NAS、FULL/IOERR/拔盘、断电、真实大库性能及多卷行为仍待目标环境实测；交叉构建和 5GiB 稀疏 Range 不提供这些证明。
- ACL/0600 是本片文件访问边界，同用户或系统权限攻击者仍可修改文件；身份与成员检查失败后保留现场，未提供整文件系统对抗或自动修复承诺。强杀/掉电可能遗留候选，本片没有按前缀启动清理。
- S03.2 持久策略、发布/对账与轮转，S03.3 离线 CLI/显式恢复准备和 inventory，S03.4 完整隔离恢复均未实施。独立 checker、正式 spec/API/verification 和任务状态由主线继续；本代理未提交、部署、归档或接触用户运行实例。
