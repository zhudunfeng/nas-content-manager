# S03.1 共用快照服务与只读校验实施设计

日期：2026-11-23。范围：只读研究与下一切片实施输入。本文不表示任何新增能力已经实现或测试通过；本次仅新增本文件，不修改业务、正式 spec、任务状态或其他报告。

## 1. 结论与源码基准

S03.1 可以作为不迁移数据库、不新增配置/API/UI 的完整小 PR：把现有 `App.backup` 的生成与清理提取为持有共享互斥的快照租约，在响应头写出前完成只读数据库检查与流式 SHA-256，再交回原 `http.ServeContent`。租约一直持有到下载返回、文件关闭和临时清理结束。下一切片从这一候选产物边界增加持久发布，不复制第二条 `VACUUM INTO` 路径。

| 已核对入口 | 当前实际行为及设计影响 |
| --- | --- |
| `internal/app/media.go:75–122` | `backupMu.TryLock`，冲突409；主库大小和余量预检；`VACUUM INTO ?`；下载返回后删除临时数据库。互斥当前覆盖下载，不能在抽取时缩短为只覆盖SQL。 |
| `internal/app/server.go:28–50,108–158,240–266` | `App` 已有 `backupMu`；旧接口经既有鉴权和安全中间件。统一退出取消请求上下文、关闭活动连接、等待请求与worker后才关闭DB/释放数据锁。无需为S03.1添加独立后台goroutine。 |
| `internal/app/store.go:17–165` | 单连接、WAL、外键、busy timeout；当前实际头版本为5。`openDatabase` 会建缓存、清临时封面、迁移并恢复任务/外部状态，绝不能用于快照检查。 |
| `internal/app/config.go:14–31,70–79`、`disk_unix.go`、`disk_windows.go` | `MinFreeBytes` 是uint64，默认512MiB；已有按目标路径查询当前可用空间的实现。文件字节数与累计运算必须检查负数和溢出。 |
| `internal/app/instance.go`、`instance_linux.go`、`instance_windows.go` | 在线实例已持有canonical数据目录OS独占租约。在线快照不再次获取同目录租约，不以端口/PID代替它。此锁也不代替临时目录权限。 |
| `scripts/verify.py:272–279` | 现有断言覆盖下载SQLite头、完整性、schema5及媒体/收集表；未覆盖共同互斥、失败清理和检查器只读性，不代表本次已运行。 |
| `scripts/verify_baseline.py`、`scripts/verify_account.py:23` | 已有 `OwnedProcess`、`new_run_root`、`clean_environment`、`wait_ready` 和产物摘要，可复用隔离验证生命周期。 |
| `go.mod`、本地固定模块文档 | Go1.26.0；`modernc.org/sqlite v1.62.0`；其 `doc.go:143–179` 明确支持 `file:` URI及SQLite URI参数，连接状态会随池中物理连接保留。 |

检索先用现有CodeGraph CLI定位 `backup/openDatabase/RunWithOptions/freeDiskBytes/acquireInstance`，再定向读源码；没有重建索引。当前 `.current-task` 仍为S02；本研究不切换它。读取时 `cmd/nas/main.go` 尚未出现S02.3离线分派，S03实施前需读取届时最终代码，避免重复维护入口。

## 2. 旧 GET /api/backup 的冻结兼容面

- 路由、管理员鉴权及安全中间件原样保留；成功仍是原始SQLite单文件，不能改成ZIP、JSON、manifest或202任务响应。
- 保留 `Content-Type: application/vnd.sqlite3`、`Content-Disposition: attachment; filename=nas-library-<本地时间20060102-150405>.db`，继续使用 `http.ServeContent`；保留中间件的 `Cache-Control: no-store` 和原GET/HEAD行为。HEAD当前也会生成快照，不在本片改变。
- busy仍为409及“已有备份正在生成或下载，请稍后重试”；旧入口在另一次旧下载尚未结束时也必须返回409。未来执行器必须调用同一入口/同一互斥；不能由handler与service各持一把独立锁或重复加锁。
- 主库状态不可读500、余量查询失败/预检不足507、创建目标失败500、VACUUM失败500、打开/状态读取失败500的现有分支保留。新增校验失败使用固定中文500；原先VACUUM期间实际磁盘写满仍可走原500，不为兼容抽取顺手重定义所有错误码。
- 取消发生在响应头前时不发送任何有效产物；已断开的请求没有可承诺的HTTP响应，不制造499等新公开契约。头已写出后的传输/清理失败不能再追加JSON或宣称下载成功。
- `ServeContent` 返回不提供成功传完的证明。日志只能说明“快照生成并校验通过”，不能把handler返回当成客户端下载完成。旧接口每次请求均生成新快照，不新增跨请求断点续传的稳定文件承诺。

## 3. 最小文件变更与内部责任

以下为拟议名字，实施时按邻近代码命名落地；不要求创建通用备份框架或新依赖。

| 文件 | 最小变更 |
| --- | --- |
| 新增 `internal/app/backup.go` | 快照生成、共享 `backupMu`、有界预算、私有临时目录、候选租约、统一关闭/失败清理及安全错误分类。 |
| 新增 `internal/app/backup_verify.go` | `file:` URI构造、独立只读连接、schema/完整性/外键/大小检查、流式摘要；不读取线上账户秘密内容，不迁移。 |
| `internal/app/media.go` | 原handler改为取得租约→defer关闭→设置原头→`ServeContent`；删除原内联VACUUM/删文件逻辑，附近媒体读取不动。 |
| `internal/app/store.go` | 提取唯一的 `currentDatabaseSchemaVersion = 5`，启动的“较新版本拒绝”与校验器共用。历史迁移的 `PRAGMA user_version=1/2/3/4/5` 必须保持各自固定版本，不能全改成头版本常量。无新迁移。 |
| `internal/app/server.go` | 最多修正 `backupMu` 注释为生成/校验/下载/清理共同互斥；若采用下述退出错误汇总，将 `cleanupErr` 注释从进程树扩为资源收尾。无需启动新执行器。 |
| 新增 `scripts/verify_backup.py` | 保留可重放的隔离HTTP/重启与产物验证；复用现有基线进程/目录边界。临时Go故障注入测试执行后删除，不删除既有测试。 |
| `docs/api.md`、`docs/verification.md`、runtime spec | 业务实施并实际验证后再同步“下载前校验、database-only、实际结果”；本研究不修改。S03.1不新增设置项或前端类型。 |

建议的内部形状（签名示意，不是本次生成的业务代码）：

```go
// snapshotMetadata 保存本次只读检查实际得到的非秘密事实。
type snapshotMetadata struct {
    SchemaVersion int       // 从快照PRAGMA实际读取的数据库版本。
    SizeBytes     int64     // 本次完整读取且与检查结果一致的文件字节数。
    SHA256        string    // 同一文件句柄流式计算的小写十六进制摘要。
    CreatedAt     time.Time // 生成完成时间，使用UTC，不冒充精确事务快照时刻。
    Scope         string    // 固定database-only，不包含外部材料。
}

// snapshotExpected 描述调用方已有的字节身份；nil表示仅生成事实而无外部预期。
type snapshotExpected struct {
    SizeBytes int64            // 必须为正的预期文件长度。
    SHA256    [sha256.Size]byte // 已解码的预期摘要，不接受任意算法或未校验文本。
}

// createDatabaseSnapshot 取得共享互斥，失败自行清理，成功将唯一所有权交给租约。
func (a *App) createDatabaseSnapshot(ctx context.Context) (*databaseSnapshot, error)

// verifyDatabaseSnapshot 只读检查自有稳定候选，并可核对调用方已有的大小和摘要。
func verifyDatabaseSnapshot(ctx context.Context, path string, file *os.File, expected *snapshotExpected) (snapshotMetadata, error)

// Close 幂等地关闭、清理本次自有候选并最终释放共享互斥，保留首个收尾结果。
func (snapshot *databaseSnapshot) Close() error
```

`databaseSnapshot` 私有持有只读 `*os.File`、已核对 `FileInfo`、metadata、自己创建的目录/文件身份、`sync.Once`、首个close结果及释放互斥的函数。handler仅使用这些内部事实，不自行重新打开路径/重算摘要/删除文件。S03.1调用checker时 `expected=nil`；这表示摘要已计算，不能宣传为与可信manifest核对过。可选expected的内部正反断言为后续离线验证保留最小复用点，不提前增加manifest解析/API。

所有实际新增方法/字段/配置说明必须有中文注释、显式类型，关键分支有中文行注释和安全中文阶段日志。只把固定阶段/错误类别和大小/schema写日志，不输出SQL原文、完整路径、原始SQLite诊断、Cookie、密码hash或会话材料。

## 4. 一次生成的精确顺序

1. 检查ctx，`backupMu.TryLock()`；busy返回内部sentinel，旧handler映射原409。成功后立即安装失败清理/解锁defer；任何后续步骤错误均不能漏锁。
2. 核对canonical数据根下主库为非链接普通文件、size非负。沿用原最低 `MinFreeBytes + 2×size` 预检；推荐将size取主文件与源连接只读 `page_count×page_size` 逻辑尺寸的较大值，避免WAL中已提交增长导致只看主文件低估。页计数查询很短，不能在查询尚未关闭时开始VACUUM。
3. 乘法/加法均先检查uint64/int64溢出；不可表示就是明确失败，不能绕回小数。查询目标所在文件系统余量。预检是保守准入而非空间预留；并发增长、其它进程占盘、quota/卸载仍可能令后续I/O失败，必须保留失败清理。
4. 在canonical DataDir创建随机私有 `.backup-pending-*` 目录，POSIX权限0700；目录内固定 `snapshot.db` 尚不存在。替代现有“CreateTemp→忽略Close/Remove错误→复用裸路径”的窗口。只接受服务生成的目录/文件名，不从HTTP接受路径。Windows继承ACL需单独验收，0700不能当作Windows权限证明。
5. 源DB执行一次参数绑定的 `VACUUM INTO ?`，使用传入ctx，不包外层 `BeginTx`，不复制线上 `library.db`，不手动checkpoint或改线上journal_mode。此时源连接仍是既有单连接：其它SQL请求可能等待，不能宣传VACUUM与线上SQL写入真正并行。
6. VACUUM返回后确认候选非链接普通文件。在生成阶段用服务拥有的可写文件句柄设置0600并 `Sync`、检查Close，再打开只读句柄。检查器自身从不以读写模式开库。显式Sync避免对固定嵌入SQLite版本/源同步配置作未验证的持久化假设；S03.1仍不承诺崩溃后可发现的持久恢复点。
7. 完成下一节的只读校验。校验SQL在独立连接，不占源库连接或业务事务；每个Rows逐一关闭后再执行下一条SQL。所有错误均清理本次候选，不返回半成品。
8. 把只读文件游标置于0，再检查ctx、路径与句柄身份/大小未变，构造租约；只在此时把所有权从生成失败defer移交给调用方，不能局部完成就标记成功。
9. handler在首个响应头前获得完整租约与metadata，设置旧响应头并交给 `ServeContent`。defer关闭顺序为：校验连接/Rows已收完→下载文件关闭→仅本次已识别候选文件删除→空临时目录删除→互斥解锁。建议Close自身完成整个顺序，调用方不用依赖多个defer的排列。

新临时目录中只出现服务认识的固定候选/必要SQLite临时文件；清理使用受控目录句柄或逐个确切路径/身份检查，只删除本次确有归属的成员，最后 `os.Remove` 空目录。禁止对可替换路径盲目 `RemoveAll`、按前缀扫整个DataDir、跟随链接清理或删除任意未知成员。身份改变/未知成员导致清理失败并保留证据。OS数据锁不能阻止拥有相同OS权限的外部进程篡改文件，因此不作绝对防篡改承诺。

## 5. 只读checker的最小安全与判定

### 5.1 打开与只读证明

- 必须是服务完成VACUUM后的独立、稳定候选；拒绝symlink/Windows重解析点、非普通文件和不能证明归属的路径；打开前后 `Lstat/Stat/os.SameFile` 校验。复用同一只读文件句柄做摘要和HTTP读取。
- 独立 `sql.Open("sqlite", <正确转义file-URI>)`，显式 `mode=ro&immutable=1`，最多一个连接并固定一条 `*sql.Conn` 完成所有检查；及时关闭Rows、Conn和DB，检查关闭错误。可在该连接启用 `query_only` 作额外防线，但它不能替代 `mode=ro`。
- URI按库/标准URL编码构造，Windows绝对盘符转换为 `/E:/...`；中文、空格、`#`、`%`及平台允许的其它字符必须保持原文件身份，不能手工拼接查询串。不允许调用方注入mode、vfs、ATTACH或任意pragma。
- `immutable=1`只能用于已隔离且本片不会再改的单文件候选；不能套在在线主库上，也不能用它忽略仍有效的WAL。未来离线入口必须先证明完整单文件/拒绝遗留sidecar或复制到受控稳定检查区，再复用checker。不能将任意正在变化的用户DB标成immutable。
- 不调用 `openDatabase/Run/LoadConfig`，不运行迁移、恢复jobs、撤销会话、建缓存、checkpoint或设置持久journal_mode。数据目录与快照内容的前后摘要、目录成员和修改时间是验证只读性的实际证据。

### 5.2 检查顺序与成功条件

| 检查 | 具体成功条件与失败边界 |
| --- | --- |
| 文件/大小 | 普通文件、至少完整SQLite头、size为正；读取头严格匹配SQLite3签名。通过只读 `page_size/page_count` 做受检乘法，要求独立VACUUM候选的实际size等于页尺寸乘页数，拒绝截断或尾随垃圾；expected存在时先核对正长度。 |
| schema | 从快照读取实际 `PRAGMA user_version`，S03.1只接受当前真实头5；0、旧版和未来版明确不兼容，不改版本。并用只读 `LIMIT 0`/列信息探针确认核心表及本次头版本必需的account字段存在，不读取字段秘密值。探针是最低结构检查，不声称证明任意伪造库与全部历史迁移语义等价。 |
| 完整性 | 推荐直接 `PRAGMA integrity_check(1)`，必须恰好一行 `ok`且Rows.Err为nil；其它行/无行/扫描错误均失败。只保留安全错误类别，不拼接原始诊断。此参数限制诊断条数，不是只检查一个数据页。 |
| 外键 | 独立 `PRAGMA foreign_key_check` 必须零结果且迭代无错误；发现首个违反即终止/关闭，不收集无界列表。完整性检查不能替代这一项。 |
| 摘要 | 校验连接完全关闭后，从只读句柄用固定64KiB缓冲流式SHA-256；每次Read前后检查ctx，累计长度使用int64并要求恰好size，禁止ReadAll/整库内存缓冲。有expected时核对其摘要；无expected只生成事实。 |
| 结束稳定性 | 摘要结束后重新核对同一文件句柄/路径身份、size和mtime；再次检查ctx，重置游标。被替换或变化的候选不得下载。摘要证明字节一致性，不证明来源可信或整栈可恢复。 |

完整 `integrity_check` 比 `quick_check` 覆盖更多索引/唯一性检查，故本片优先前者，不把两者都跑一遍。若真实库证明时延不可接受，另基于测量决定检查等级，不能静默把“完整性”降成只读文件头。检查仍有I/O/CPU成本；局部测试需记录耗时，目标NAS大库另轨测量。

## 6. 故障、取消、关闭与清理矩阵

| 故障点 | 必须结果与验证方式 |
| --- | --- |
| TryLock竞争/慢下载 | 第二个旧GET立即409，不等待无界队列。首个handler退出并清理后可再成功；未来执行器使用同一锁。 |
| 主库stat失败/不安全类型；预算溢出 | 无VACUUM、无有效产物；固定安全错误，源DB不写。可用临时fixture或受控注入，不改用户库。 |
| freeDisk查询失败、预检不足 | 原507，不创建候选；不得先删除旧备份或用系统临时目录兜底。小磁盘/大阈值测试与真实quota测试分开。 |
| Mkdir/权限/卸载/只读目录失败 | 返回原创建失败500；保留源和其它文件，不把管理员权限下不能复现ACL拒绝当通过。 |
| VACUUM失败或取消，实际FULL/IOERR | 不服务半成品；关闭SQLite正在使用的资源后删本次文件/目录、解锁。验证原库完整性与后续新快照可用；不盲重试同一路径。 |
| Chmod/Sync/Close失败 | 候选不算ready，错误不能忽略；只清理已确认自有成员。Sync不是可抢占调用，慢/卡住存储仍是系统I/O风险。 |
| 只读打开、schema/完整性/外键/大小/摘要失败 | 不写成功头；固定中文500，候选被清理，源DB/旧文件不改。测试单独覆盖每类，无行和Rows.Err不得当成功。 |
| 校验/摘要中ctx取消 | 每个SQL用Context，摘要循环检查取消；资源收完后解锁。不能仅在开始检查ctx或开goroutine后遗忘等待。 |
| 流式下载断连/应用退出 | 关闭自有连接使ServeContent返回；Close完成后请求计数才归零。应用仍先等待请求、worker，后关DB/数据锁，不新增短全局WriteTimeout。 |
| 头已写出后的file.Close/删除失败 | 不能把200改成另一段JSON；输出固定安全中文收尾日志并保留可定位的自有产物标识。可复用App.mu保护的cleanupErr汇总，使退出返回收尾失败，不宣称安全清理完成。解锁必须执行，Close幂等且重复调用不二次解锁。 |
| 路径被替换/有未知成员 | 校验或清理失败封闭；不跟随链接/递归删除未知内容。保留遗留记录；测试sentinel文件必须存活。 |
| 进程被强杀/掉电 | defer可能不执行，DataDir可能留下不完整 `.backup-pending-*`；S03.1不把它当有效持久备份，也不扫描前缀自动删除。记录明确限制，由S03.2的专属根/归属/manifest对账解决。 |

`r.Context()`已经继承workCtx，统一退出会取消VACUUM和checker。S03.1不需额外加入a.wg；若后续S03.2新增执行器，它必须由RunWithOptions注册并在关闭DB/lease前等待，不能把当前同步handler改成脱离请求的后台任务。

## 7. 给S03.2留下的边界，暂不实施的内容

- 本片返回的是“已校验且持有所有权的临时候选”，不是备份历史行、可永久下载文件或整栈恢复点。metadata保留scope/schema/size/hash/UTC生成事实，便于下一片生成manifest。
- 暂时只在DataDir生成；未来配置 `NAS_BACKUP_DIR` 后，生成目录参数才扩到已验证专属根，并在实际目标卷做预算/同目录候选发布。不能先在DataDir生成，再假设跨卷rename原子。
- 持久发布作为下一片新增的一个显式所有权转移操作：校验完成→关闭阻碍Windows重命名的句柄→文件与manifest落盘/同步→专属目录原子发布→登记run；成功后临时候选Close不再删除已转交产物。禁止现在暴露任意 `Keep/Detach` 开关让handler遗留文件，也不能让调用方拿Path后随意rename。
- 下一片给文件/manifest/DB跨资源窗口定义reconciliation；仅有某个已命名文件不等于可恢复产物。旧GET继续持锁到下载收尾；未来持久文件下载使用独立引用租约，轮转跳过活跃下载，不能用缩短旧锁来回避Windows冲突。
- 不加策略表、迁移、每日定时器、requestId/schedule_key、保留轮转、重启回收、UI、通知事件或CLI命令。S03不等S04事件能力，S04.4再消费备份结果。
- 迁移交接：S02.3受限离线账号维护器首版严格只支持schema5；S03.1无迁移可保持该边界。S03.2若分配新schema，必须同片复核并扩展该维护器的显式支持集/结构探针及稳定验证，证明新增表不改变其账号写入前提，不能让正常升级的新库失去已交付的忘记密码恢复入口；不得用调用openDatabase隐式迁移来规避。
- 恢复inventory、通用持久用户资产采集/恢复、合成原件验收、离线verify/inspect、独立显式的恢复会话撤销都属于S03.3及后续。checker保持只读，不能顺手清会话/推进auth_revision；恢复副本正式启动前的撤销门禁仍必须实现，不能因为快照可校验就省略。
- DB内含账户hash与会话摘要等敏感资料，快照按敏感文件保管；不包含来源秘密、helper身份、worker私密状态、组件卷、下载载荷、媒体和未来用户资产原件。本片不能宣传整栈备份或异地灾备。

## 8. 可复现验证设计与实施顺序

### 8.1 实施顺序

1. 读取当时最新S02.3与schema头，冻结本片内部签名/旧GET错误映射；仅提取共享租约和失败清理，先证实旧成功下载仍兼容。
2. 加只读checker与schema常量，串入生成阶段；用临时Go测试覆盖内部故障矩阵、取消和Close幂等。
3. 加保留的隔离 `scripts/verify_backup.py`，复用基线新根/端口/真实就绪/OwnedProcess；运行真实HTTP竞争、文件清理和重启验证；不支持 `--binary` 的旧verify.py仍由基线启动实例后调用。
4. 跑Go质量门禁、core和受影响账号验证，读回产物和日志；删除仅本轮新建临时单测，保留集成脚本与脱敏证据；更新实际契约后交独立复核。

### 8.2 必须保留的断言

| 层级 | 正反例与确定性手段 |
| --- | --- |
| 生成/元数据 | 真正schema5隔离DB包含账号、媒体、收藏/标签/进度及原下载/收集结构；快照的actual schema、size、64位计数、sha256与Python独立只读计算一致。 |
| 版本/结构 | 真正旧schema fixture、空库、未来user_version、标5但缺必需结构分别拒绝且字节不变。历史迁移验收继续用verify_schema.py冻结fixture，不用“最新版改低版本号”冒充迁移。 |
| 完整性/外键/大小 | 非SQLite、截断/损坏页、尾随垃圾、关闭外键后写出的真实孤儿行；完整性可过但外键失败要独立命中。故意错expected长度/摘要均失败。 |
| 只读 | checker前后文件hash/size/mtime、目录清单一致，无新增-wal/-shm/-journal；对只有读取权限的隔离候选检查成功。权限测试依实际OS/账号报告，不能用chmod结果代替WindowsACL。 |
| 竞争 | 临时Go测试持有已生成租约，以barrier确定第二次调用busy；Close后第三次成功。真实HTTP用足够大合法DB、限制接收窗口并先观察首个响应进入传输，再发第二GET断言409，不靠固定sleep和小文件碰运气。 |
| 一致性 | 并发提交收藏/进度请求时生成快照，快照完整/外键成立且各业务事务内部关系一致；允许请求在单连接VACUUM前后等待，不要求包含开始后全部写入。源与快照分别核验。 |
| 取消/关闭 | 在VACUUM、检查、摘要受控边界取消；慢HTTP下载时通过自有进程的正常停止路径退出并验证文件/DB/锁可重新独占打开，立即重启同一隔离DataDir成功；原外部sentinel进程和文件存活。 |
| 失败清理 | 注入freeDisk错误/不足/算术溢出、mkdir/open/Sync/Close/Remove失败与路径替换；确认无200有效快照、无死锁、不删除未知文件/上次样本、后续请求可运行。系统真实FULL/拔盘/ACL与注入证据分开。 |
| 大文件资源 | 摘要走固定缓冲，不依产物大小分配；用有界真实大库或专门hash文件测峰值。稀疏文件不能作为SQLite有效性/真实NAS性能证据。 |
| 日志 | 原始内存日志先检查fixture密码、Cookie、hash及原始私密路径/诊断未泄漏，再持久化安全结果；不能用已脱敏日志证明原始日志无秘密。 |

内部故障注入可通过包内小依赖函数/测试专用构造实现，不新增生产环境故障开关、不修改全局库行为。新测试源执行后按用户要求移除；真实HTTP回归断言留在集成脚本，不能只剩空 `go test` 而宣称行为已覆盖。

### 8.3 命令模板与证据边界

以下是实施后的计划命令，**本研究未执行**。新脚本的参数也仍为拟议，先按实际 `--help` 冻结，不能把模板当现成功能：

```powershell
# 格式、静态分析、包测试和主服务产物；gofmt只传实际修改的Go文件。
python -B scripts/project_env.py -- gofmt -w internal/app/backup.go internal/app/backup_verify.go internal/app/media.go internal/app/store.go
python -B scripts/project_env.py -- go vet ./...
python -B scripts/project_env.py -- go test ./...
python -B scripts/project_env.py -- go build -o .runtime/bin/nas.exe ./cmd/nas
# 拟新增脚本；root必须尚不存在且位于.runtime/verify下，产物必须显式给定。
python -B scripts/project_env.py -- python -B scripts/verify_backup.py --binary .runtime/bin/nas.exe --root .runtime/verify/s03-1-<唯一编号>
# 现有core编排；FFmpeg/probe必须替换为实际已核验的绝对路径。
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --artifacts .runtime/bin --cases core --ffmpeg <绝对路径> --ffprobe <绝对路径>
```

有server.go改动时将其加入gofmt；前端代码不改时不把重复前端构建设为本研究门禁，若后续实际改到前端则补typecheck/build。记录源码/dirty状态、二进制hash、OS、工具版本、真实断言数、状态/超时/清理结果及安全证据路径；不把旧core历史结果当本轮重跑。Windows本机、Linux交叉编译、实际Linux/NAS权限/文件系统和真实断电演练分别标记。

## 9. 尚需实施验证的风险与官方依据

最高风险是单连接VACUUM对线上SQL时延的影响、临时所有权在错误路径泄漏、Windows重命名/删除句柄释放，以及把immutable误用于仍有有效WAL或会变化的数据库。实现者须先做上述barrier/注入断言，再做隔离HTTP，最后补目标NAS证据；本研究没有启动任何用户实例、Docker或业务验证。

依据以仓库固定依赖与当前源码为首：`.trellis/tasks/11-23-nas-backup-recovery/prd.md`、父路线info和reliability research、runtime/cross-platform/account/baseline契约、现有API与三份恢复手册。本次为核对SQLite语义另查阅官方文档（2026-11-23）：

- SQLite `VACUUM` 的INTO章节：一致快照、候选未完成的故障边界、目标要求与同步条件；当前文档不能替代对本仓库固定嵌入版本的实际验证。<https://www.sqlite.org/lang_vacuum.html>
- SQLite URI文档：`mode=ro`与`immutable=1`的不同保证、Windows盘符及转义；immutable依赖文件不再变化这一前提。<https://www.sqlite.org/uri.html>
- SQLite PRAGMA文档：integrity_check与foreign_key_check分工、quick_check覆盖差异。<https://www.sqlite.org/pragma.html>

本次验证状态：源码/任务/契约已只读核对，研究文件写后读回；Lint、TypeCheck与业务测试未运行（仅研究文档，无业务改动），不能记为pass。S03.1正式实施仍需完整执行本片质量门禁。
