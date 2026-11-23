# S03.2.2 受控根与平台发布实施交接

2026-11-23。本报告为实施者自查与交接，尚未代替独立 checker 验收。当前激活范围是 S03.2.2 / Phase 2.1；实施权威为 `root-publisher-contract.md`，SHA256 `b4c3f3aab47babd9851721ba2eab79376a92b7cc1e2c8248105e6345bebd93e4`。独立接缝研究 `research/root-publisher-contract-review.md` 的 SHA256 仍为 `2dc04d3b45100e0342838c07d71834d5a16d13d4948ae55076e94552b34670b6`。未实施下一片执行器、手动/API、启用、调度、下载或轮转。

## 文件与交付

本片产品共涉及以下 9 个文件；工作区原有其它改动不归本片，不恢复或重写。

| 文件 | 本片内容 |
| --- | --- |
| `internal/app/config.go` | 新增有中文说明的 `BackupDir`，仅同次 lookup 读取 `NAS_BACKUP_DIR`，默认 DataDir/backups；加载只静态校验，不探测/创建备用根。 |
| `internal/app/torrent_search_config.go` | 保留已有参数的内部 `SourcesFile`，同 lookup 只取一次并固定绝对路径；disabled 不打开秘密文件，不新增公开 DTO。 |
| `internal/app/backup_permissions_windows.go` | 复用私密描述符生成逻辑，避免当前用户为 SYSTEM 时重复主体；继续使用真实 protected DACL。 |
| `internal/app/backup_format.go` | 固定 marker/owner/manifest、原始字节摘要、字段/时间/版本/角色/长度边界及逐 token 严格 JSON。 |
| `internal/app/backup_root.go` | 显式根准备/已有根模式、保护路径与历史根预算、持有 anchor/父/guard、固定锁、marker 无覆盖发布、短 CAS 补绑定、持续阻断及幂等 Close。 |
| `internal/app/backup_publish.go` | 短只读事务核意图；借用同 App 源租约、64 KiB 复制、余量与实际逻辑预算、目标独立 schema/size/hash 检查、精确清单/成员、无覆盖发布及确定性结果。 |
| `internal/app/backup_storage_windows.go` | 实际卷 GUID 路径、fixed NTFS、内核跟随 anchor、零数据访问锁身份检查、实际 DACL 核验、排他 WRITE_THROUGH 文件与仅 MOVEFILE_WRITE_THROUGH 移动。 |
| `internal/app/backup_storage_linux.go` | O_PATH 端点与持有目录 File、mountinfo/fstat/fstatfs 映射核对、仅 ext4/XFS、0600/0700/euid、Renameat2 NOREPLACE 与目录 fsync。本轮只交叉编译。 |
| `internal/app/backup_storage_other.go` | 未列平台明确 unsupported，不降级复制/删除或普通 rename。 |

所有新增方法、结构字段、配置有中文说明，关键流程用固定中文阶段/受控 code 记录。CodeGraph CLI 已用于入口/依赖检索，新增符号尚未在既有索引时返回空结果后才定点读源；没有无故更新索引。

README 中 NAS_BACKUP_DIR 的中文参数说明以及正式 spec/docs/任务状态由主线统一补充，本片没有写 README、Compose、desktop.json、公开路由或正式规范。

## 重要实现边界

- 默认正常启动不调用新根/publisher 能力、不建备份根/marker、不取得根锁；旧 GET/HEAD 快照继续独立工作。显式根准备失败只返回备份域安全错误。
- 目录树保护和文件点保护分开，文件父目录只作为解析 anchor；保留缺失尾部投影。历史 collection 根查询使用 LIMIT 257 lookahead、256 核验预算，超量或不可证明隔离即拒绝。
- 原始入口、父目录及保护路径实际映射均持续重核。Windows 物理比较唯一取真实句柄的最终卷路径；不能把原始路径丢掉后只核固定旧句柄。终端根和成员仍拒绝 reparse，anchor 本身可以由内核解析合法的父 junction。
- 固定锁永不 unlink，不写 PID；锁文件空/普通/单链接/目录项与句柄一致。marker.tmp 完整或部分残留均保守保留，不自动提升。绑定的根/marker 缺失不建替代物。
- 已有合法 marker 根只核 DACL，不重新设置根的继承 DACL；不合格已有根受控拒绝。仅已证空根设置根权限，避免 Windows 自动传播到未知子树。固定已知锁文件单独设权限。
- Windows 可把每主体目录 OICI/GA ACE 规范化为本对象 FILE_ALL_ACCESS 与 OICI/INHERIT_ONLY GENERIC_ALL 两项；按唯一 SID 的有效/继承授权精确核验，拒绝未知主体、额外位/重复授权、不受保护及错误 flags。不能硬编码必须恰好两条 ACE。
- publisher 不重开、不转移、不关闭源租约，源由调用者最终 Close。只读事务关闭后才做目标 I/O；目标余量为 MinFree+实际源大小+1 MiB，DB/owner/manifest 实际字节总量受 run.max_bytes 限制。源生成前的整体峰值准入仍归下一片。
- 目标实测 schema 除 size/hash 外显式比较；owner/manifest 使用原始字节事实。移动前关闭目标 SQL/成员/子目录句柄。最终移动调用后任何失败均 needs_reconcile，保留同 ID 材料；不自动重试/覆盖/final 删除。
- 成功仅说明规定 OS 发布协议及后验核验确认，不写 run completed、last-good、轮转授权，不等同于硬件断电保证。此时 CLI 链接器可能不保留未接入口的内部能力，公开 API 回归不能冒称 publisher 异步集成已经交付。

## 最终临时 Go 行为证据

唯一最终轮次：`.runtime/verify/s0322-behavior-final3/`，UTC `2026-11-23T05:47:49.400616+00:00` 至 `2026-11-23T05:48:02.223371+00:00`。实际 `go test ./internal/app -run ^TestS0322 -json -count=1 -timeout=240s`，exit 0，原始输出秘密哨兵扫描通过。

最终 **18 顶层测试、210 带斜线叶子子场景、0 嵌套容器、0 失败**。18 顶层中有 3 个无子场景的独立测试，不与 210 叶子混算，也不把 pass 事件叫断言。

| 顶层组 | 通过叶子 | 主要覆盖 |
| --- | ---: | --- |
| Budgets | 11 | 非法/溢出与逻辑/目标余量算术。 |
| StrictFormats | 67 | marker/owner/manifest 重复、转义重复、大小写、类型、未知/缺失、整数词法、尾随及嵌套反例。 |
| RootModes | 14 | 零值/非法模式不写、首次/重开、锁争用、CAS 前后故障、完整/部分 tmp、缺根/marker、错 store、未知根、缺父链、Close 失败仍放锁。 |
| Isolation | 18 | 目录树/文件点、默认 DataDir 例外、文件点父目录不误扩大、prefix sibling、历史根、256/257、BT 排除、缺默认引擎目录不创建。 |
| StickyIdentity | 4 | marker 原字节变化/同字节换实体、保护配置变化、缺失路径变现存，恢复后仍阻断。 |
| PublisherFaults | 25 | 目标 free 错误/不足/恰限、实际 64 MiB 源的 max below/exact、owner/DB/manifest Sync/Close、目录 Close、移动前后/同步/后验、未知材料、分阶段取消、源 schema/hash 不符。 |
| Collisions | 4 | pending/final 文件或目录同名哨兵保持，不换 fileId 绕过。 |
| IntentChecks | 7 | owner 大小/hash、取消、run/file 状态、seq、错误 App 源归属。 |
| Configuration | 5 | 默认/离线备用根不创建、相对/非法 DataDir 包含拒绝、SourcesFile 同 lookup 一次且 disabled 不打开。 |
| MemberBudget | 4 | 精确/缺失/未知/2048 未知成员，n+1 有界拒绝且保留。 |
| FormatEdges | 15 | 1024/16384 原始恰限/+1、epoch/最大时间/非规范 UTC、Version UTF-8 字节边界。 |
| WindowsPathSyntax | 23 | 保留名含 COM/LPT 上标¹²³、扩展名、设备/相对/尾点空格等拒绝及合法相邻名。 |
| WindowsDACL | 8 | 真实目录规范化/新文件/继承后需显式 protect、未知主体/额外授权/unprotected/错误继承、唯一 SYSTEM 逻辑。 |
| WindowsAliases | 3 | 真实目录 junction、实际硬链接文件点、实际 guard 目录替换。 |
| WindowsUnknownACLPreserved | 2 | 同卷移入未知未保护子树；合格根接管与不合格根拒绝时，根/未知 dir/file 实际安全描述符摘要和文件内容前后不变。 |

三个无子场景独立测试分别为：

1. `TestS0322ProductPublisher`：真实 App/SQLite 唯一快照服务和 running/pending 意图，产品发布确认，目标 schema/长度/hash/清单/精确成员、源归属及 Close 幂等，不写运行完成状态。
2. `TestS0322WindowsHandleCollision`：真实非删除共享 snapshot 句柄使目录移动失败，needs_reconcile 保留 pending；释放句柄后同意图仍拒接续/覆盖。
3. `TestS0322WindowsRootMappingSwap`：根通过父 junction 初次准备；另一物理根放同样 marker，实际替换父 junction 后旧租约拒绝，恢复映射仍 sticky，原物理固定锁仍独占。

SYSTEM 场景仅执行“当前主体与 SYSTEM SID 相同”的核验分支，不代表实际以 SYSTEM 进程运行。Windows symlink-tree 夹具在本机创建失败，未进入产品断言，不计通过；本轮明确使用真实 junction，没有提升权限/更改系统设置。失败轮次证据全部保留。

两份临时源在行为结束后按 SHA 精确移出编译目录，保存为 `.runtime/verify/s0322-final-checks/removed-test-sources/*.txt` 供复核；没有保留新生成 `_test.go`：

- `internal/app/backup_s0322_temp_test.go`：`674c61ac19bfb7448696a1970188af75507e783ed2e805b6d6086dad063c06b1`。
- `internal/app/backup_s0322_temp_windows_test.go`：`0d87226da7bc383c3a0acef1f4d76940c6d8e70c360a32748fa54a3badf60f24`。

清源清单：`.runtime/verify/s0322-final-checks/removed-test-sources.json`；清源后 `go test ./...` 均显示 no test files，只计包编译证据。

## fresh 工程门禁与同产物回归

命令均从 `python -B scripts/project_env.py --` 启动，缓存/临时目录限定仓库 `.runtime/`，没有用户级环境变量或 go env -w。Go/gofmt 固定 `.tools/go/bin/`；GOOS/GOARCH/CGO_ENABLED 仅子进程环境。工程门禁的外部命令保留实际 stdout/stderr、退出码、原始秘密哨兵检查和 UTC 时间；进程内 AST 核验单独记录文件数与缺失清单。

工程根 `.runtime/verify/s0322-final-checks/`，UTC `05:49:18.189336` 至 `05:49:53.924793`：gofmt、go vet ./...、清源 go test ./...、git diff --check、22 份 scripts/*.py AST/方法入参返回注解、Vue typecheck+build、Windows/amd64 以及 Linux/amd64、Linux/arm64 构建全部通过。Python 没有本片产品变更；该检查不替代 Python 集成行为。

| 产物 | 字节 | SHA256 |
| --- | ---: | --- |
| `.runtime/bin/s0322-implement-final/nas.exe` | 21,593,600 | `aa285c1824d368b9410c5abc94e5c3514240b604d09a4765f2966d3b791e14d0` |
| `.runtime/bin/s0322-implement-final/nas-linux-amd64` | 21,379,424 | `81fe48d63e7c75d21c27e3e273448850e9c14e3e9965db597e135bb1727b6bf2` |
| `.runtime/bin/s0322-implement-final/nas-linux-arm64` | 20,067,563 | `cf746f7c2a80612deac67e1c7f72ee6581a61bb303f0903ed64698d1eeec15a8` |

四项回归都使用上述同一个 Windows SHA；未复用 S03.2.1 产物或通过数。

| 回归 | 实际命令参数（均带上述环境前缀） | 结果与证据 |
| --- | --- | --- |
| storage | `python -B scripts/verify_backup_store.py --binary .runtime/bin/s0322-implement-final/nas.exe --root .runtime/verify/s0322-storage-implement-final` | 17 断言，exit0，result.json。 |
| backup | `python -B scripts/verify_backup.py --binary .runtime/bin/s0322-implement-final/nas.exe --root .runtime/verify/s0322-backup-implement-final` | 38 断言，exit0，result.json。 |
| account | `python -B scripts/verify_account.py --binary .runtime/bin/s0322-implement-final/nas.exe --root .runtime/verify/s0322-account-implement-final --offline` | 162=83 在线+79 离线，exit0，result.json 及 offline/result.json。 |
| core | `python -B scripts/verify_baseline.py --cases core --artifacts .runtime/bin/s0322-implement-final --root .runtime/verify/s0322-core-implement-final --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe --ffprobe D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffprobe.exe --large-file` | 52 断言，exit0，run-manifest.json；UTC05:50:11.068–05:50:38.105。 |

storage/backup 各 3 次真实隔离 Console CTRL_C_EVENT 正常停服，共 6 次正常、0 应用强制回收，cleanupFailures 均空。core 的应用退出1、`owned-tree-reclaimed` 为预期 Job 收尾，明确单列，不算正常停服。账号真实 ACL、Console、离线输入/版本 CAS 覆盖以本轮 offline/result 为准；没有把账号在线脱敏日志的回扫说成完整原始扫描。

实施者另行读回重算脚本 `.runtime/verify/s0322-tools/run_final_audit.py` 于 UTC `2026-11-23T05:51:37.435802+00:00` 写入 `.runtime/verify/s0322-final-checks/final-audit.json`：从原始事件重新数 18/210/0，三个产物重算长度/SHA；四项回归同 Windows 身份；193 项产品源摘要前后零漂移；临时 Go 源 0；Get-CimInstance 按本轮 nas.exe **完整绝对路径**核活进程为 0，没有按名称杀进程。storage/backup/account 结果没有 finishedAt，不用 mtime 冒充结束时间。本脚本不是独立 checker 代理验收。

关键证据摘要：

- behavior-final3/result.json：`4c5401a04208a7e78a907dba51ca7aaf46a06c14d3e938032844ef768d0fe8a9`。
- s0322-final-checks/result.json：`2b8fef768ba5a63587a87631a727f6ef58f5b13c783938d7434ca4da29759bb9`。
- s0322-final-checks/final-audit.json：`927b614b5b429523146544d6253aec2561994459d25dd00ba8eab87edc780c6a`。

## 调试根因与失败留痕

下列历史全部保留于 `.runtime/verify/s0322-behavior-*`，不与最终轮次累加：

- 初始编译使用不存在的 windows.FILE_ALL_ACCESS 标识，改为固定、受检 mask；behavior-5 的 SID 变量误用与 behavior-7 的 SECURITY_INFORMATION 显式类型问题均为编译失败，没有行为通过信用。
- behavior-1 固定锁身份误拒：Go 1.27 Root.Lstat 请求读访问，与 Windows 零共享锁冲突。专用零数据访问 Stat 核目录项身份，不改变独占原语或共享锁设置。这属于隐含 OS/API 假设与真实集成覆盖不足。
- behavior-2/3 根 DACL 误拒：假定原始两个 OICI/GA ACE 保持原样，而 Windows 将有效/仅继承权限规范化。改按唯一 SID 的精确有效/继承权限位核验，没有为通过测试放宽未知主体或额外权限。
- behavior-6 的 max below/exact 夹具错误使用低于 schema 最小 64 MiB 的 max_bytes；改为真实 64 MiB zeroblob SQLite 源后测边界。run/file terminal 夹具补齐 finished/error/deleted 必需字段后才测业务拒绝。这些是测试前提错误，不是产品业务通过。
- behavior-8 symlink-tree 仅在夹具创建阶段失败；最终明确换为本机可实际创建的 junction-tree，symlink 未计产品通过。
- 后续审查发现，对已有根重新设置继承 DACL 会自动传播至未知子树。改为合法已有根只核验，不合格受控拒绝；新增真实同卷移入带额外继承权限的未知子树，以实际安全描述符和内容摘要证明均未修改。
- behavior-final、mapping-diagnostic、final2 的新 RootMappingSwap 曾失败。Go 当前实现将 junction 标记 ModeIrregular：末端 EvalSymlinks 不展开，配合 OPEN_REPARSE_POINT anchor 与跟随后的 Stat 不同；仅令 anchor 跟随后，`alias/backup` 的中间 junction 又在 walkSymlinks 被判 ENOTDIR。最终采用平台解析接缝：Windows 由内核跟随，真实物理投影取最终句柄，保留原入口/父映射持续比对；Linux 仍显式 EvalSymlinks+O_NOFOLLOW。final3 实际执行初次/替换/恢复/锁反例通过。
- 补齐 anchor 临时/失败分支和 candidate 部分打开失败的 Close 收尾，合并受控 Close 错误；没有在 required Sync/Close 失败时返回 confirmed。

上述根因按 trellis-break-loop 的 E（隐含假设）与 D（真实集成覆盖缺口）归类。预防措施已体现在实际句柄/目录项交叉核对、按语义授权而非 ACE 条数核验、独立真实 junction/ACL 反例以及最终原始事件读回。本片只写实施报告，正式跨平台/备份规范与指南由主线同步，不自行提交规范或 Git。

## 剩余风险、后续与停写

- 真实 Linux/ext4/XFS、bind/mountinfo/目录 fsync/flock、真实 NAS/多卷、FULL/拔盘/只读介质、进程崩溃/系统崩溃/断电、过滤驱动/设备缓存与大库性能均未实测。本机 Windows 行为与故障注入不能代替这些目标门禁，Linux 两架构仅编译。
- 本次进程未能创建 symlink 夹具；真实 junction 与硬链接覆盖不代表所有 symlink 类型通过。唯一 SYSTEM SID 核验不等同 SYSTEM 宿主运行。
- marker.tmp 保守残留和跨资源移动未知不自动修复；需下一片按原 ID、无 source 事实完成对账，不重建新候选掩盖未知结果。marker 跨重启只证明逻辑命名空间，不能检测全部离线完整克隆。
- 当前能力是包内根/发布协议，尚无生产公开入口和唯一执行器。源前整体峰值、容量/ownership 准入、取消调度、完成事务、下载/轮转授权均待后片；不因本片通过而启用策略或提前激活 S03.2.3。
- 本地 fixtures 含私密 SQLite 与账号材料，证据根保留在仓库 `.runtime/`，不宜整体上传。秘密哨兵扫描只证明所列标记未出现在原始输出，不宣称能识别任意未知秘密。
- 未启动 Docker、未访问/停止用户实例、未部署、未提交、未归档任务或删除失败证据。

**实施者现停止写产品、临时测试与本报告，交主线安排独立可写 checker。** 主线可据本报告补 NAS_BACKUP_DIR README 说明、正式契约和阶段状态；独立复核通过前本报告仍只是实施自查证据。
