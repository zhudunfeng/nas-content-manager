# S03.2.2 受控根与平台发布接缝核对

日期：2026-11-23。范围：Phase 1.2 / S03.2.2 窄研究。本文件是实施输入，**不是冻结、实现或产品门禁通过报告**。仅新增本文件，不修改业务代码、依赖、正式 spec、任务指针或主线契约；不访问用户实例，不运行动态 fixture。本轮按父任务授权独占写本文件，S03.2.1 并行实施成果不回退、不覆盖。

已完整读取 persistent-backup-design.md、platform-backup-publish.md、persistent-clock-root-seams.md、platform-publish-fixture.md、冻结的 storage-schema6-contract.md；对照尚未冻结的 root-publisher-contract.md。既有跨重启逻辑命名空间、停服完整搬迁及 Windows OS 确认协议边界沿用已接受取舍，不重新提出物理身份字段或更强掉电承诺。

## 1. 结论与最小切片

本片可交付备份域专用 root lease、严格 marker/owner/manifest、借用已验证源租约的目标复制和平台无覆盖发布。**publisher 只返回目标实际事实及发布确定性**；读取既有 running/pending 意图，但不写 completed、last-good、去重摘要或轮转授权。S03.2.3 再接唯一执行器与同事务完成登记，本片不为测试增加公开 CLI/API、自动任务或 enabled=true 入口。

主线草案的基本边界可实施。冻结前建议补齐：固定锁名与 magic；marker 临时文件的保守崩溃处理；结果零值及移动后未知分类；Linux 明确使用 renameat2(RENAME_NOREPLACE)；完整历史收集只读根隔离；严格 JSON 的逐对象键检查；源 Close 与目标发布事实分离。它们无需增加表、物理身份、第三方依赖或通用存储框架。

## 2. 实际代码接缝

检索优先使用 CodeGraph CLI，再定向读取实际源码。索引缺失的 acquireInstanceFile、os.Root、snapshot lease 以源码补核，未更新共享索引。行号为本轮读取时定位，S03.2.1 的并行修改可能令后续行号变化。

| 实际位置 | 已核事实及实施含义 |
| --- | --- |
| internal/app/config.go:40–101 | LoadConfigWithLookup 从本次 source 读配置，最后载入 Downloads/Collection/Engines。备份静态配置应在其它隔离对象收齐后核对；备用根 I/O 错误不能变成全站配置 fatal。 |
| internal/app/instance.go:10–31,34–51 | canonicalDataDirectory 可解析现有父目录并拼回缺失尾部；acquireInstance 会 MkdirAll 并使用数据目录锁，不能直接用它准备备份根。 |
| internal/app/instance_windows.go:13–28 / instance_linux.go:13–29 | 已有零共享、OPEN_REPARSE_POINT 的 Windows 锁，以及 O_NOFOLLOW + 非阻塞 flock 的 Linux 锁；不截断、不写 PID、不 unlink。可复用底层锁原语，不复用“已有媒体库实例”的错误文案。 |
| internal/app/backup.go:49–62,81–216,219–253,278–319 | 唯一源快照入口已持 backupMu；租约持只读文件和目录身份，Close 幂等并精确清源。checkMembers 使用 ReadDir(2) 而非全目录枚举。源不能 Keep/Detach 或改属。 |
| internal/app/backup_verify.go:19–32,50–116 | metadata 有实际 schema/size/hash；snapshotExpected 只有 size/hash。目标复验之后仍须显式比较 source.metadata.SchemaVersion，不能声称 expected 已核 schema。 |
| internal/app/backup_permissions_windows.go:13–34,37–40 | 已能设置当前用户/SYSTEM protected DACL、拒绝重解析点；setter 不是实际继承/权限读回证明。新增根/文件应复核真实 ACL，不以 chmod 代替。 |
| internal/app/libraries.go:27–29,48–65 / collection_import.go:13–29 | within 只做分段路径比较；历史 local_root 的 complete 子树仍授权媒体读取，现 helper LIMIT 256 无截断信号。备份隔离不能只核当前 Collection.ReadRoot。 |
| internal/app/download_gate.go:123–196 | 已有 2 MiB 有界 mountinfo 读取及设备内路径映射，可参考识别 bind alias；下载特有放行规则不能原样套给备份。 |
| internal/app/account.go:110–161 / internal/desktop/json.go:12–76 | 账号 parser 已示范 token 后精确键比较；desktop parser 有重复键预扫，但处于另一包且不严格拒绝单个错大小写键。不能 app 反向 import desktop，也不能只用 DisallowUnknownFields。 |
| internal/app/server.go:86–153 | DataDir 租约、DB 和现有 manager 已有顺序；本片无调用者时不在启动创建闲置 backup manager/loop/root lease。未来关闭须等相关 I/O 后再放根、DB、数据锁。 |
| go.mod:3–11 / 固定 Go 与 x/sys 源 | 已有 x/sys v0.50.0，Linux Renameat2、RENAME_NOREPLACE、Fstatfs 和 Windows 所需 API 均存在，不必升级依赖。 |

## 3. 建议冻结的内部接口与所有权

以下为接口草图，不是已写入产品的代码。名称仅服务备份域；初始化模式为包内枚举，不能从 Web 传入任意路径/开关。

~~~go
// backupRootOpenMode 区分只打开既有根与明确允许首次绑定。
type backupRootOpenMode uint8

// backupPublishCertainty 表示本次目标发布事实，零值保守地要求核对。
type backupPublishCertainty uint8

// backupPublishResult 保存磁盘事实，不表示数据库已完成登记。
type backupPublishResult struct {
    Certainty      backupPublishCertainty // 明确未发布、协议确认或待核对。
    Metadata       snapshotMetadata       // 目标实测 schema/size/hash，生成时间沿用源事实。
    OwnerSize      int64                  // 实际 owner 原始字节数。
    OwnerSHA256    string                 // owner 原始字节的小写 SHA-256。
    ManifestSize   int64                  // 实际 manifest 原始字节数。
    ManifestSHA256 string                 // manifest 原始字节的小写 SHA-256。
    Stage          string                 // 来自固定允许集的安全中文阶段。
    Code           string                 // 来自固定允许集的非秘密错误分类。
}

// acquireBackupRoot 核对配置与存储身份，只在明确模式且未绑定时初始化专属根。
func (a *App) acquireBackupRoot(ctx context.Context, mode backupRootOpenMode) (*backupRootLease, error)
// checkIdentity 核对本租约的根、锁和 marker，变化后阻断继续写入。
func (lease *backupRootLease) checkIdentity() error
// Close 在相关 I/O 全部结束后幂等关闭句柄和锁，保留第一次收尾错误。
func (lease *backupRootLease) Close() error
// backupOwnerBytes 生成登记 pending 前必须固定的唯一规范 owner 字节。
func backupOwnerBytes(storeID string, runID string, fileID string) ([]byte, error)
// publishBackupSnapshot 借用根与源，读取已登记意图并返回目标发布事实。
func (a *App) publishBackupSnapshot(ctx context.Context, root *backupRootLease, source *databaseSnapshot, runID string) (backupPublishResult, error)
~~~

- mode 的零值只打开已绑定根，遇 root_bound=0 先返回需要明确准备，不写根或 CAS；另一值才允许首次初始化和匹配最终 marker 的补绑定，不使用含混 bool。非法枚举立即拒绝。certainty 的零值建议为 needs_reconcile，另外两值为 not_published / confirmed；不要让默认空结构冒称“肯定未写盘”。
- root lease 由调用者唯一持有：保存根与必要父目录句柄、首次 os.FileInfo/平台身份、独占锁、storeId、marker 原始身份和关闭结果。Close 不删除 marker/锁/产物；先关闭其它根资源、最后释放锁，并合并错误。sync.Once 只保证幂等，不代表允许 Close 与 publish 并发。
- source 始终属于调用者。publisher 只借 source.file/metadata；借用期间调用者不得并发读、Seek 或 Close。发布器不能重开源路径、rename、删除、转移目录归属或增加 Keep/Detach；退出所有分支后调用者仍按 S03.1 Close。
- publisher 拥有它创建/打开的目标文件、子目录和目标检查连接，负责同步、关闭及受控清理；不能关闭传入的根/源。Windows 移动前关闭全部目标成员、SQL、摘要及 pending 子目录句柄；根 anchor/锁继续持有。
- publisher 根据 runID 短查询当前 store、run 和 file，验证 running、pending、run.file_id、file.run_id/run_seq/store_id、owner_size/hash 全相配；owner 字节由上述 helper 再生成并比对已登记摘要。不得接受任意 owner/manifest 路径，不接管 unverified 或仅同名对象。
- caller 须提供同一 run 的唯一执行所有权；本片不新造第二套队列锁。DB 查询/首次绑定 CAS 是短事务，复制、SQLite 检查、摘要、移动及目录同步期间不持 DB 事务或未来 manager.mu。
- 已存在同名 pending/final 时，本函数不覆盖或接续写入。若可能是原意图已执行遗留，返回 needs_reconcile，留给下一片同 ID 对账；与本次意图不符的碰撞保持原样并报告 collision。不能新建 fileId 绕过它。

## 4. 配置、路径隔离与生命周期

唯一新增宿主配置为 NAS_BACKUP_DIR；Config.BackupDir 及环境示例逐项写中文解释。空值固定为 canonical DataDir/backups，不新增 desktop.json 参数或任意 Docker 挂载。经 LoadConfigWithLookup 的同一 source 读取；不改进程环境。默认值在现有 DataDir 卷内，不宣传为异地备份。

1. 静态校验只处理本地绝对路径、NUL、平台语法、明显双向 ancestor 冲突。Windows 拒绝用户提供的 UNC/设备命名空间、驱动器相对路径及会发生保留名/尾点空格别名的分段；映射盘是否远程再由实际卷能力判定。暂时权限/挂载/解析失败保留候选并记备份 unavailable，不让正常媒体/账号启动失败。
2. 根不得等于或包含 DataDir；位于 DataDir 内时，仅允许与默认目录完全相同的 backups。与每个 MediaRoot、Downloads.Root（即使 disabled）、已配置 Collection.ReadRoot、引擎连接目录和已配置私密材料所在保护根双向隔离；不能只比较当前启用模块。
3. **历史授权补点**：根准备时查询 SELECT DISTINCT local_root FROM collection_tasks WHERE local_root<>'' AND kind<>'bt' LIMIT 257；保留去重后的原始绝对根供规范化，不按 LIMIT 256 无信号截断。超过 256、查询/Rows/Close 失败、记录路径无法安全确认时，仅拒绝本次备份准备。建议按与当前 ReadRoot 一致的历史 local_root 整根隔离，至少必须覆盖仍被 allowedPath 授权的 complete 子树。不要据此更改历史授权或删除任务。
4. 对现存路径解析 symlink/junction 与真实父路径后，再分段双向比较。Windows 以实际句柄最终卷路径、卷内分段及 File ID/SameFile 复核，保守处理大小写/长短名/卷挂载别名；Linux 用实际 st_dev、mountinfo 中的设备内根及路径发现 bind alias。简单 strings.HasPrefix、小写路径或只比盘符均不够。离线媒体挂载不是忽略隔离的理由；无法证明时只阻断备份准备。
5. os.OpenRoot 会跟随打开根路径的链接，Root 也允许根内相对链接与跨挂载；因此仍须 Lstat 拒根/子目录/成员链接和重解析点，核对根路径、保留根句柄及成员句柄身份。只从受检 ID 推导固定直接子名，禁止任意内部相对路径。
6. 明确首次准备才允许创建末级专属目录；建议要求现存且可确认的父目录，不用 MkdirAll 穿过未知或离线父链。新根在写任何 DB 字节之前先设置并读回私密权限。合法已知根可核对/收紧根权限，不批量修改未知目录或成员 ACL；已有材料逐项核验。
7. 正常启动时，默认 disabled/root_bound=0 不 mkdir、不 marker、不锁、不探测外部根；明确准备模式仍可供下一片手动备份使用，不要求先启用计划。本片无公开调用者时正常启动不调用 acquireBackupRoot。root_bound=1 根离线或锁不可取不新建替代、不批量置文件 missing、不阻主媒体服务；旧 GET /api/backup 不依赖它。
8. 运行期路径/句柄/marker 换实体后，本 lease 持续 blocked，不在同一 lease 热切新根；关闭相关 I/O 后才可走下一片明确的重新准备/对账。跨重启逻辑身份和停服完整搬迁边界沿用既定契约，不新增物理身份字段。

桌面入口仍通过既有配置合并快照读取环境默认；本片不扩桌面可写 JSON。Docker 默认 /data/backups 沿现有 data 卷，无需新增卷模板。若实施者实际改动 compose/serviceEnvironment 或固定参数检测，应同步其模板、更新与检测位置；不能先改模板却漏检。此研究不预先扩张这些文件。

## 5. 锁、marker 与 root_bound CAS

建议固定锁名为 .nas-backup.lock，marker 为 .nas-backup-store.json，marker 临时名为 .nas-backup-store.json.tmp。锁是永久保留的空普通文件，不写 PID、时间或秘密。Windows 复用零共享、OPEN_ALWAYS、OPEN_REPARSE_POINT 原语；Linux 复用 O_NOFOLLOW/O_CLOEXEC 和 LOCK_EX|LOCK_NB。根/锁打开后核对当前目录项与保留句柄相同，拒绝链接、非普通文件、非零锁内容和文件多硬链接；不删除锁，否则 Linux 可分裂为旧 inode 与新锁两个持有者。

复用 acquireInstanceFile 只减少系统调用重复；外层应转换为 backup_lock_unavailable 等受控分类，不能把其所有错误都说成有另一个媒体实例。固定锁文件仅在普通、单链接、空、身份已确认且本进程已持有时，才可从“首次空根”判断中排除。只看文件名不能证明归属。

marker 建议 magic 固定为 nas-content-manager-backup-store，formatVersion 精确整数 1，storeId 精确 32 位小写 ASCII hex。它不含主机/卷/物理身份；最多 1024 原始 UTF-8 字节。写法：确定空/新根 → 私密权限 → 排他临时文件 → 有界规范字节 → Sync → Close → 平台无覆盖移动为最终 marker → 相应目录同步/后验身份内容核对 → 短事务 CAS。

CAS 精确条件为 backup_store.id=1、store_id=租约预期、root_bound=0，RowsAffected 必须确认。Commit 返回未知不能解释为“肯定没绑定”；重新读 DB/store/marker 后分类。没有锁外自动清 marker 或以文件状态回滚 DB 的补偿协议。

| 持久状态/失败窗口 | 本片最小处理 |
| --- | --- |
| 正常启动，root_bound=0，disabled，根不存在 | 完全无写盘动作；不因配置值存在而预建。 |
| 明确初始化，新/空根（仅确认的锁可排除） | 排他完成上述 marker 协议；任一失败不 CAS。已有未知成员不覆盖。 |
| root_bound=0，最终 marker 已完整匹配 storeId，尚未 CAS/提交结果未知 | 在独占锁下重新核根/marker/能力，再补 CAS；不重写合法 marker、不更换 storeId。 |
| root_bound=0，只有 marker.tmp，无最终 marker | **保守保留并报初始化待核对**，不覆盖、删除或仅因内容相似就提升 tmp。完整和部分 tmp 都不自动接管；若未来要自动恢复此窗口，应另明确所有权/恢复契约。本片不造修复框架。 |
| root_bound=1，根/最终 marker 缺失，或 marker 错 store/格式/权限/身份 | unavailable/mismatch；即使有合法 tmp 也不补建最终 marker，不把空挂载当新根。 |
| marker 已发布，CAS 失败或上下文在 CAS 附近取消 | 保留 marker 与 DB 事实，返回受控未绑定/待核对；下次只核同一 store，不假定事务已回滚。 |

Close 不承担“完成尚未同步的发布”；所需 Sync/移动须在返回 confirmed 或绑定成功之前明确完成，Close 错误不能静默消失。marker/锁不是用户备份容量，不能据此声称所有目录内容已验证。根初始化遗留临时项需安全人工核对是本片明确保守限制，不把它伪报为自动恢复成功。

## 6. owner/manifest 严格格式与有界解析

- owner magic 建议固定 nas-content-manager-backup-owner；精确字段为 magic、formatVersion、storeId、runId、fileId。三个 ID 均 32 小写 ASCII hex，formatVersion=1；最多 1024 字节。使用固定顺序 struct 的规范 json.Marshal 输出、无额外换行，登记 pending 前固定原始字节长度/hash；parser 不把重新序列化后的摘要当原始字节摘要。
- manifest 精确字段为 formatVersion、storeId、runId、fileId、scope、createdAt、appVersion、schemaVersion、files、excludedRoles；最多 16384 字节。scope=database-only；files 精确一个对象，精确字段 role/name/size/sha256，role=database、name=snapshot.db、size>0、sha256 为 64 小写 hex。
- excludedRoles 精确七项、无重复，建议按固定顺序输出并检查：source-secrets、helper-identity、worker-state、component-volumes、download-payloads、media、user-assets。它仅表达未包含角色，不冒充完整 inventory/恢复可用性/异地证明。
- createdAt 建议冻结为源实际生成完成时间的 UTC RFC3339Nano 规范文本（Z 结尾、解析后规范格式回写相同），且实际时间可转受检毫秒范围；不是事务精确时刻。appVersion 为当前受检应用版本，1–64 UTF-8 字节、无 NUL；不能从用户输入填宿主路径。schema/size/hash 取目标实际复验并与源实测匹配；本片只处理本程序已登记意图，不扩大历史版本导入支持。
- 读取前检查普通文件/单链接/身份及声明上限，再用 LimitReader(limit+1) 有界读入；Stat 长度不足以替代读取限额。拒绝无效 UTF-8、原始或解码后 NUL；读取前后核文件身份、长度、mtime。元数据可有界驻内存，数据库不可整库读入。
- 必须 token 级解析每一个固定对象：解码后的键名精确匹配允许集，seen 集记录已解码键，拒绝重复、转义重复、错大小写/大小写别名、未知或缺字段；嵌套 files 成员同样做。不能先 unmarshal 到 map 再查重复，也不能只依赖 DisallowUnknownFields 的大小写匹配行为。
- 采用 UseNumber，整数词法仅接受 0 或非零数字起首的十进制整数，再受检转 int64/目标范围；格式/大小/schema 拒绝字符串、null、bool、小数、指数、负号及溢出。时间、数组长度、固定角色与名字另外逐项验证。顶层必须恰一个对象，结束后只允许空白并得到 io.EOF。
- parser 可借鉴 decodeAccountJSON 的精确 token 模式，放 backup 域内的少量固定形状函数；不抽全项目 JSON 框架、不反向依赖 desktop。当前协议的身份与固定文本最终还须与登记意图/规范输出比较，不能接受解码替代字符造成的身份歧义。
- 对 root 首次空检查、pending/final 成员检查使用 ReadDir(n+1) lookahead：最终只许三个固定成员，读取最多四项即可拒绝未知；准备阶段按本阶段明确允许集设相同常数预算。拒绝大小写别名、目录/设备/链接、重复实际成员和未知项，不 ReadDir(-1) 后截断。

## 7. 平台协议与依赖

| 平台 | 最小实现与支持门槛 | 不可回退行为 |
| --- | --- | --- |
| Windows | 实际根句柄查询文件系统、最终卷路径及驱动类型，只接受 fixed NTFS；固定 x/sys 提供 GetVolumeInformationByHandle、GetFinalPathNameByHandle、GetDriveType、CreateFile、MoveFileEx 和安全描述符查询。文件 CREATE_NEW + WRITE_THROUGH，逐个 Sync/Close；marker/manifest/目录仅 MoveFileExW flags=8。 | 不设 REPLACE_EXISTING/COPY_ALLOWED/DELAY_UNTIL_REBOOT；不因移动失败改 os.Rename/copy-remove；不要求或吞掉没有等价文档语义的 POSIX 目录 Sync。不读取/记录卷序列号、SID 或真实路径到业务日志。 |
| Linux | 同根直接子名用固定 x/sys/unix.Renameat2(dirfd, old, dirfd, new, RENAME_NOREPLACE)。dirfd 来自持有 root.Open(".") 的 *os.File，生命周期覆盖调用，Stat/SameFile 与根 anchor 复核；manifest 临时移动以受控 pending dirfd 同样处理。文件 fsync、pending 目录 fsync、修改名称的父目录 fsync 分别检查。 | 不用允许覆盖的 os.Rename/Root.Rename，不用存在预检冒充原子无覆盖；ENOSYS/EINVAL/EOPNOTSUPP、EXDEV、碰撞和 Sync 错误均不能 confirmed。禁止 fallback。 |
| 其它 GOOS/未确认文件系统 | 返回 backup_storage_unsupported，仅影响持久备份。 | 不静默启用一般 Unix 路径、远程协议或弱保证 completed 模式。 |

Linux 首批建议**只列本地 ext4 / XFS 为实现候选矩阵**：实际 mountinfo fstype 与根句柄 Fstatfs 类型相符，允许 bind mount 但必须通过物理路径隔离；须实际支持 RENAME_NOREPLACE 和目录 fsync。Fstatfs 的 EXT4_SUPER_MAGIC 与旧 ext 家族共值，不能仅凭该 magic 就说是 ext4。NFS、CIFS/SMB、FUSE、overlay、tmpfs、Btrfs 及未列类型本片保持 unsupported/未验；这不是声称它们一概没有相应原语。man-pages 记录 ext4 自 Linux 3.15、XFS 自 4.0 支持 NOREPLACE，实际系统/挂载仍必须检查，版本字符串不能代替调用结果。

Linux 新建根还须同步其现存父目录，使根本身的目录项进入对应协议；marker 移动影响根，manifest 移动影响 pending，最终目录改名的父目录就是备份根。不要把“根父目录”误实现为只同步备份根外层、漏掉实际变更子项的根目录。目录 *os.File 与 os.Root 均要有唯一关闭者，传递裸 fd 时仍持有对应 File，不能提前 Close 后复用数字 fd。

Windows 根的 protected DACL 须读回，且新子目录/文件的实际继承/保护符合只向当前用户和 SYSTEM 授权的约定。既有 restrictSnapshotPath 只证明设置调用存在；可用固定 x/sys GetSecurityInfo/GetNamedSecurityInfo/SE_DACL_PROTECTED 做本域复核。建立私密父目录后才写秘密；不得 chmod 0600 后即宣称 Windows 隔离完成。

Windows 协议边界按已接受的平台研究执行，不重复推出物理掉电保证。已做的 4 场景/19 断言原语 fixture 是合成字节、真实 API/碰撞/占用证据；不是本片产品 publisher、正式 JSON、SQLite、DACL 或独立标准账号验收。该次占用实报 ERROR_ACCESS_DENIED=5，不能只认 32，也不能把所有 5 判为可自动重试。

## 8. 发布顺序、取消与未知结果

1. caller 已持根 lease、唯一 run 执行权和 S03.1 source lease，短事务已登记 pending/owner 意图。publisher 再核来源、根/锁/marker及原意图；源 Seek(0)，固定 64 KiB 缓冲复制，逐轮检查 ctx，检查短写/总长/超长，不能重开源路径或读入整库。
2. 排他建立 .pending-<fileId>，逐项私密创建 owner.json、snapshot.db；每项写入后 Sync/Close 并保留每个错误。只允许本次确认对象进入后续阶段，已有同名不覆盖。源稳定身份和 metadata 在复制前后核对，源租约仍持有。
3. 目标关闭写句柄后，以独立只读 checker 比较源实测 size/hash；再显式比较目标实际 SchemaVersion==source.metadata.SchemaVersion，检查 integrity/外键/页长及前后身份。checker 返回的 CreatedAt 不能替换源生成完成时间。全部目标检查连接/句柄关闭。
4. 根据实际结果生成有界 manifest，排他写 manifest.json.tmp、Sync/Close，平台无覆盖改为 manifest.json；复核恰三个成员、owner/manifest 原始长度/hash、目标 DB 事实及身份。Linux 完成 pending 同步；Windows 关闭所有目标子目录/成员句柄后才移动。
5. 关键 I/O 前后核根身份；同根无覆盖 .pending-id→b-id，执行平台必要同步，再打开最终目录重新核根/目录/成员身份与真实内容。不能只检查最终名字存在或只相信移动返回 nil。
6. 只有全部规定操作和后验核验成功才返回 confirmed 及完整发布 tuple；本函数不落 run completed。其后才观察到的 ctx 取消不能抹掉已确认发布事实。取消阻断了后验核验，则 needs_reconcile。

| 结果情形 | 返回语义与保留动作 |
| --- | --- |
| 本次没有调用最终目录移动，且不存在原意图既有 final/未知旧尝试 | not_published；保留确切阶段/错误。仅可清理由本次身份/owner/允许成员证明归属的候选，不触碰碰撞 sentinel。 |
| 最终移动明确碰撞，已证明目标不属于本次，源仍原样 | collision / not_published；不覆盖目标，不把任意碰撞都当可接续发布。 |
| 发现相同意图已有 final，或移动已尝试且返回/源目标状态不确定 | needs_reconcile；保留原 run/file/目录/意图，不新快照、不换 ID、不删除 final；明确错误可确定未移动时也须基于实际身份事实，而非一律归类成功或无写。 |
| 移动成功，但根同步、Close、身份/摘要或最终核验失败/取消 | needs_reconcile；不 completed、不授权、不删旧；不能因目录可读就跳过补足平台协议。 |
| 协议全部成功，源最终 Close/精确清理失败 | 目标仍 confirmed；caller 单独记录资源收尾错误。若下一片已 completed，不改为可盲重试的 failed。 |
| 目标 confirmed，下一片 DB 完成事务失败/Commit 未知 | 保留原 pending/running 或实际已提交事实，原 ID 对账；不能将发布确定性简化为 err!=nil 即未发布，也不自动开始新 run。 |

error 与 Certainty 必须同时消费：成功事实不能被清理 error 擦除，未知也不能靠 error==nil 误升成功。文件权限/磁盘 FULL/设备错误、取消、共享拒绝和身份不符分别映射受控分类；中文日志仅固定阶段/分类/非秘密 ID 或计数，不输出原始异常、宿主路径、SQL、私有 URL、用户名/SID/卷身份。

未发布清理逐个删除确认的自有成员，owner 最后、空目录最后；未知项或替换对象保留，禁止 RemoveAll/前缀扫描。publisher 只返回磁盘/收尾事实，pending→deleted/interrupted 等状态转换与重启有界对账由下一片协调；不把未发布残留塞进仅供 completed/retained 轮转的 deleting。

## 9. 最小生产接入与待实施门禁

建议组织为 backup_root.go、backup_publish.go、backup_format.go 与 Windows/Linux/其它平台小 helper；若职责较小可合并，不为凑文件扩框架。保留 S03.1 backup.go/backup_verify.go 的唯一快照和校验路径；本片通过包内临时行为用例直接构造真实 DB 意图、调用私有能力即可验证，运行时无需新空 manager/循环或测试后门。S03.2.3 唯一 executor 之后直接调用这些函数并提交完成事务，S03.2.4 再接调度/下载/轮转。

实施应覆盖：默认启动无根/锁/marker；根离线不阻媒体及旧 GET；完整历史根/离线根/链接和 bind alias 隔离；锁竞争与不 unlink；marker 写各窗口及 CAS 未知；严格键/整数/EOF/字节上限与 n+1 扫描；根/锁/成员替换；真实 SQLite 复制与目标实测 schema/size/hash；碰撞 sentinel、真实句柄拒绝和 DACL；Sync/Close/移动前后/后验核验/取消的确定性；所有 Close 幂等与唯一所有权。新临时单测执行后精确清源，不能删他人并行临时测试。

现有 Windows 原语 fixture 不替代上述产品验证；本片没有异步 API，因此稳定 API/重启对账端到端门禁由下一片接入后再补，不能宣称已交付异步服务。fresh 同产物 backup/storage/account/core、gofmt/vet/清源编译、Python/Vue 检查及 Linux 双架构交叉构建属于实施门禁；Linux 真实 ext4/XFS、真实 NAS/FULL/拔盘/断电和大库性能分开记账。

## 10. 来源与本轮验证记录

- 一手本地源码：.tools/go/src/os/root.go:34–67,79–127,198–224 明确 Root 跟随内部链接、允许跨挂载及重命名语义；固定 x/sys v0.50.0/unix/syscall_linux.go:2071、zsyscall_linux.go:1468 与 zerrors_linux.go:3163 提供 Renameat2/RENAME_NOREPLACE，Windows 源提供上述 Win32 与 ACL 查询 API。未改工具链、缓存或依赖。
- Linux man-pages 维护者文档：https://man7.org/linux/man-pages/man2/rename.2.html 。2026-11-23 经项目包装器只读 HTTP 取得 200 与相关正文，核对 NOREPLACE 的目标已存在拒绝、底层文件系统支持要求及 ext4/XFS 版本，不由此声称本机运行过 Linux。
- Microsoft 原始文档：https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw 。同日经包装器只读 HTTP 取得 200 与 flags/返回值相关正文，核对 WRITE_THROUGH=8、REPLACE_EXISTING/COPY_ALLOWED 及目录同驱动器限制；不将 copy/delete 的具体 flush 语句外推成同卷整目录树掉电保证。其余 Windows flush/权限依据沿已完整读取的 platform-backup-publish.md 官方来源，非本轮新增实验。
- 本轮 web 搜索/打开工具未返回可用正文或引用 ID；上述主源改用标准库有界只读 HTTP 取得，方法与边界如实记录，不伪造网络引用或实验结果。
- 本轮完成：任务/冻结契约/草案/研究/实际代码与固定依赖的只读核对；完成后全文读回本文件并计算 SHA-256，摘要交父任务记录。
- **Lint：未运行；TypeCheck：未运行；构建/业务测试/动态 fixture/产品门禁：均未运行。** 本片只交研究，既有他片或历史结果不计入本轮通过数；无新测试源、提交、部署、用户实例或产品设置变更。

## 11. 私密保护路径清单与缺失路径判定补核

本节为同日只读补核，细化第 4 节的“已配置私密材料所在保护根”，不扩张产品实现或测试结论。配置必须沿 LoadConfigWithLookup 的同一 source 快照取得，不在备份准备时重新读取 os.Getenv；目录树与单个文件须分开建模，不能把任意文件的 dirname 自动升级为私密专属树。

| 实际字段或落盘点 | 已核来源与实际用途 | 建议保护粒度 |
| --- | --- | --- |
| Config.Engines.ConnectionDir | engine_config.go:46–57 从 NAS_ENGINE_CONNECTION_DIR 读取并 Abs，默认 /run/nas-engines；engine_runtime.go:71–123,139–148 读取 current.json 与组件代次清单，后者含下载密码或 Jackett APIKey。loader 不创建目录，managed 未启用时不轮询。 | 明确目录配置，整树双向隔离；默认目录和 disabled 模式也保留在集合中，但允许按下述缺失尾部规则证明分离，不要求清单已经存在。 |
| Config.Engines.Socket | engine_config.go:48–50 从 NAS_ENGINE_HELPER_SOCKET 读取，非空要求绝对路径；它是控制面 Unix socket 端点，不是普通秘密文件。 | 非空端点作为精确保护点；父目录作为解析与句柄 anchor，不因一个 socket 就将整个 DataDir、仓库或卷根升级为私密树。不能连接 socket 来做隔离证明。 |
| Config.Collection.TokenFile | collection_config.go:23–55 从 NAS_COLLECTION_WORKER_TOKEN_FILE 读取，成组配置时要求绝对路径；collection_worker.go:19–26 在调用 worker 前读实际 token。 | 非空文件精确保护，父目录用于解析、别名与身份核对；不是无条件 dirname 整树禁用。 |
| Config.Downloads.PasswordFile | download_config.go:31–65,103–117 从 NAS_DOWNLOAD_QB_PASSWORD_FILE 保留原路径；external 模式通过 os.Open 读取密码，当前允许相对路径，disabled 时可未读过。 | 非空文件即使 disabled 仍精确保护；备份静态解析按加载配置时同一进程目录固定绝对路径快照，不以 DataDir 为基准，不顺手改变原模块的启用或凭据读取规则。 |
| NAS_TORRENT_SEARCH_SOURCES_FILE | torrent_search_config.go:36–75 直接从 source.lookup 交给 os.Open，JSON 内含 APIKey，允许相对路径；现有 TorrentSearchConfig 只有 Mode/Enabled/Sources，没有 SourcesFile，且 disabled 会先返回。 | 最小接缝是在 TorrentSearchConfig 增加有中文说明的内部 SourcesFile string，从同一 source 保留一次非空路径，原读取也用此值；备份保留同基准绝对快照，disabled 不丢来源。不新增宿主参数、不外发路径或秘密正文。 |
| canonical DataDir/collection-secrets | collection_store.go:19–31,63–123 写入每项私有 URL、固定 worker 请求、外部 API 连接/动作快照；目录 0700、文件 0600，含 .pending-* 临时文件。 | 代码固定的私密专用目录，整树双向隔离；不能因候选是默认 backups 就跳过此项。 |
| canonical DataDir/download-sources | collection_bt.go:87–109 写入 UUID.source，内容是实际 magnet/torrent 来源字节，供下载/收集流程引用。 | 代码固定的私密专用目录，整树双向隔离；不是下载载荷根的替代。 |
| Config.WebDir / WebDirExplicit 对应的磁盘资源候选根 | server.go:416–431 在显式目录或未提供嵌入资源时使用 os.DirFS，static:448–487 会提供根下已有普通文件；默认磁盘值为 web/dist。 | 配置磁盘候选根保守整树双向隔离，防止备份被当前或后续磁盘静态服务下载，即使本次选择嵌入资源也保留此项；嵌入资源本身不是磁盘树，不将保守隔离误述为已从磁盘提供服务。 |
| MediaRoots、Downloads.Root、Collection.ReadRoot 与历史 local_root | 第 4 节及 libraries/collection_import 的真实授权读取根；Downloads.Root 即使 disabled 也保留。 | 继续按目录树双向隔离和完整历史查询；Downloads.EngineRoot 属于引擎/容器命名空间，不能当作主进程本地路径 Abs/Stat。 |

DataDir 中的 engine-download-identity.json 是非秘密安装/容器/下载绑定，engine-search-sources.json 是非敏感来源选择；两者由 writeEnginePrivateJSON 及 .engine-*.tmp 发布，不是 Torznab APIKey 来源文件。它们受 DataDir 规则覆盖，不因此把整个 DataDir 归类成“私密专用树”，也不为备份隔离逐项读取秘密内容。FFmpegPath/FFprobePath 是可执行文件定位，不能将其父目录一概当私密材料树；URL/APIKey、容器内 EngineRoot 也不是本地文件路径。

### 11.1 默认 DataDir/backups 例外与文件父目录

1. 先对 DataDir 单独应用例外：候选不得等于或包含 DataDir；位于 DataDir 内时仅精确 canonical DataDir/backups 可继续检查。这个例外只豁免 DataDir 自身的祖先关系，不豁免任何其它目录树或文件保护项。
2. 私密专用树只由明确目录字段或上述代码固定私密目录构成；不能依据 basename 为 secrets、文件权限、单个 token 的父目录名称或一个文件路径猜出专属边界。DataDir/collection-secrets 与默认 backups 是可分离的兄弟树；任何已配置专用树实际落在 backups 内、包含 backups 或经别名重叠，都拒绝本次准备。
3. Socket、TokenFile、PasswordFile 与 SourcesFile 的父目录须保留为实际解析/句柄 anchor；它不是自动追加的整树禁用项。私密文件直接放 DataDir/token 或仓库根 sources.json 不会使精确默认 backups 永久不可用；文件位于 backups 下、与候选根冲突或无法排除实际别名时仍拒绝。空值直接跳过，禁止 filepath.Dir("") 意外保护当前目录或卷根。
4. 精确文件保护不能只比字符串：对现存端点核对类型、实际父链/最终路径与平台身份，保留文件 dev/inode 或卷/File ID；受检候选成员与保护文件不得为同一实体。现存硬链接多链路且无法排除目标别名时仅拒绝准备，不声称扫描过全盘所有硬链接；未知目标成员仍按根/owner 协议拒绝接管。不遍历父目录秘密正文，也不把普通父目录任意扩大为全盘禁用树。
5. 当前 Config 只能证明主进程可见的保护路径。桌面 compose 把 TokenFile 放在 /run/nas-collection/token，同一默认 secrets 映射还向 worker 提供 sources.json/rclone.conf（docker_compose.go:181–185,241–249）；这些 worker 路径不是主 Config 字段。helper 自己的 Root/state、qb-password、nonce/状态库和组件卷也不全在 ConnectionDir 内（enginehelper/config.go:13–55、service.go:70–105、files.go:185–198,290–321）。它们应在 inventory 中保持 externalRequired/unverified；不能只凭 TokenFile.dirname 宣称已枚举整个外部私密卷，不能为检查连接 Docker、遍历 helper 根或读取外部秘密。

### 11.2 缺失默认目录不是永久 unavailable

Engines.ConnectionDir 的默认值始终经 filepath.Abs 处理，loader 不要求存在；因此不能把任意 ENOENT 一律当隔离不可证明。尤其 Windows 默认未部署 managed 引擎时，缺省 /run/nas-engines 通常尚未创建，不能要求用户先建立它或 current.json 才允许数据库备份；不依赖新增 explicit/default 配置标志，也不能因 mode disabled 就彻底忽略该目录或非空秘密文件。

建议在明确准备时，以固定绝对配置快照逐项执行以下有界路径判断：

1. 清理平台语法并保留原始分段；从目标向上找到可访问的现存祖先，区分普通 ENOENT 缺失尾部与权限、I/O、设备、重解析点/悬空链接错误。不得把 EACCES 或解析失败伪装成“不存在”。
2. 对现存祖先解析链接并取得实际平台身份，拼回尚不存在的分段；连同候选根的已确认父链做双向分段关系、Windows 卷/路径别名或 Linux mountinfo 设备内路径映射检查。仅在父链与物理映射足以证明分离时通过，不创建缺失保护目录、不枚举清单、不读取凭据。
3. 未启用的默认连接目录或下载目录仅缺失尾部、祖先和候选实际身份可证明分离时，可以准备备份；若相同/相交则拒绝，若卷/挂载离线、身份或别名不能确认则仅将本次备份准备置 unavailable，不让主媒体/账号启动 fatal。未挂载路径不能因表面 ENOENT 越过已有 mount/bind 约束。
4. 准备及关键 I/O 前重核相关实际父链/根身份，发现变化阻断本 lease；未来启用模块、挂载替换或重新准备不能复用陈旧结论。不为此增加默认 disabled/root_bound=0 正常启动时的外部路径 I/O。

此补核对主线新增的目标实测空间、受检算术及 DB+owner+manifest 单份逻辑字节上限无异议；生成源前的整体同卷/异卷预算仍应由 S03.2.3 准入负责，publisher 的事后目标余量不能替代它。补核仅定向读配置/落盘/静态服务源码并追加本节；未改产品、其它文档、共享索引或配置，未运行 lint、TypeCheck、构建、测试或动态 fixture。全文读回后的新摘要单独交父任务记录。
