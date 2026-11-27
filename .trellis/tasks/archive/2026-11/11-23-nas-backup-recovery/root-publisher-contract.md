# S03.2.2 受控根与平台发布契约

2026-11-23。**已冻结并激活，作为S03.2.2当前实施权威。** S03.2.1已完成独立可写检查及主线证据读回，本文件结合`research/root-publisher-contract-review.md`冻结本片边界；既有存储和账号兼容继续遵循`storage-schema6-contract.md`及正式spec。冻结不代表本片产品已经实施或验收，不计任何产品通过数。

## 1. 交付边界

- 本片交付备份域专用的根租约、命名空间marker、owner/manifest严格格式、借用源快照的流式目标复制/复验及平台无覆盖发布能力。
- 不增加公开API、管理页面、调度器、手动请求准入、下载租约、历史裁剪、轮转或自动启用。S03.2.3再接执行器、意图登记/完成事务及取消；S03.2.4再接计划/下载与轮转。
- 默认disabled且root_bound=0时，正常启动不创建备份根/marker、不取得备份根锁。配置解析不因备用根暂时离线阻断媒体/账号服务；旧GET导出独立可用。
- 源租约仍由S03.1唯一快照路径产生；不得增加第二套VACUUM、修改源文件归属或增加Keep/Detach。

## 2. 配置与隔离

- 唯一新增宿主参数为`NAS_BACKUP_DIR`，对应`Config.BackupDir string`，从同一配置source读取；空值使用canonical DataDir的精确`backups`子目录。配置字段、环境示例和说明均用中文解释；不新增desktop.json字段或任意Docker卷挂载，不改全局环境。
- 显式值仅本地绝对目录、无NUL；拒绝UNC/远程路径。Windows拒绝设备命名空间、驱动器相对路径、保留名和尾点/空格分段别名，映射盘由实际卷再核。静态非法配置与暂时I/O不可用分开；实际根准备前再做canonical及平台能力核验。
- 根不得等于或包含DataDir；在DataDir内仅允许精确默认backups子目录。目录树保护集合精确包括所有MediaRoots、Downloads.Root（即使禁用）、非空Collection.ReadRoot、Engines.ConnectionDir、DataDir/collection-secrets、DataDir/download-sources及配置的磁盘WebDir候选根（嵌入资源本身无磁盘树）；逐树双向ancestor隔离。WebDir及其它已允许的相对路径按原读取基准固定绝对路径，不因备份引入新的相对路径解释。
- 文件点保护集合包括非空Engines.Socket、Collection.TokenFile、Downloads.PasswordFile及TorrentSearch.SourcesFile；备份根不得等于/包含这些文件，现存点核真实身份/别名。父目录只作为解析和句柄anchor，不因一个文件把DataDir/仓库根等通用父目录升级为整棵私密树，从而不误伤默认backups；实际文件落在backups中仍拒绝。不能凭TokenFile父目录就宣称枚举了外部worker秘密卷中的其它材料。
- TorrentSearchConfig最小保留既有`NAS_TORRENT_SEARCH_SOURCES_FILE`的内部SourcesFile string字段及中文说明，从同一lookup快照固定路径；不增加新宿主参数、公开DTO或重读秘密内容，disabled仍不打开文件。缺失默认Engines.ConnectionDir可沿可访问现存祖先加缺失尾部投影判定分离，不建目录/current.json；权限/卷/别名无法确认才令备份准备不可用。缺失路径不能被直接从保护集合省略。
- 还须核对数据库中继续授权读取的历史collection根，不能只检查当前ReadRoot。短查询`SELECT DISTINCT local_root FROM collection_tasks WHERE local_root<>'' AND kind<>'bt' LIMIT 257`，采用257项lookahead、固定256项核验预算；超量、查询/Rows/Close失败或任一根无法安全核验只阻断备份准备。按完整历史local_root双向隔离，从而覆盖仍授权的complete子树，不改变原读取授权。
- 比较必须按路径分段、平台大小写及现有父路径解析结果处理链接/junction/卷别名，不做简单字符串前缀判断。Windows核实际句柄最终卷路径/卷内分段和身份；Linux核st_dev、mountinfo的设备内根及bind别名。缺失媒体挂载不作为绕过隔离的理由；不能证明安全时只阻备份准备。不能仅靠os.Root宣称已拒绝根内全部链接或跨挂载。
- 首次仅可新建末级专属目录，要求现存且已核身份的父目录，不用MkdirAll穿过未知父链；写任何DB字节前先设置并核验私密权限。已知根可核验/收紧根权限，未知目录/成员不批量改ACL。
- Windows首批仅接受实际根句柄识别的本地fixed NTFS；Linux首批只接受mountinfo与Fstatfs一致的本地ext4/XFS并实际支持无覆盖移动/目录fsync。仅ext家族magic不足以认定ext4。NFS/CIFS/FUSE/overlay/tmpfs/Btrfs及其它未列类型保持unsupported/未验，不声称它们均不具备相关原语。未支持的平台/文件系统只令备份子系统不可用，不降级复制后删除。

## 3. 根租约与首次绑定

- 取得固定`.nas-backup.lock`的OS独占租约，和DataDir租约分别持有；复用底层acquireInstanceFile的Windows零共享/OPEN_REPARSE_POINT及Linux O_NOFOLLOW/非阻塞flock原语，映射为备份域错误。不按PID、端口、锁文件存在与否判断持有者。锁须普通、空、单硬链接且目录项与持有句柄一致；永久保留、不写PID、不unlink，避免锁inode分裂。
- 初次仅接管新建/空专属目录，或匹配本DB storeId的合法marker。空判断排除本次确认的固定锁文件，不能覆盖未知非空目录。
- marker固定为`.nas-backup-store.json`，唯一临时名`.nas-backup-store.json.tmp`，最多1024原始UTF-8字节；精确字段`magic,formatVersion,storeId`，magic=`nas-content-manager-backup-store`、格式整数1、小写32hex。单个JSON对象，拒绝重复解码键、未知/错大小写键、错误类型、NUL和尾随内容。
- marker临时文件排他创建、受限权限、Sync/Close、平台无覆盖移动及后验核验成功后，短事务CAS`id=1 AND store_id=expected AND root_bound=0`到1；要求RowsAffected确认。提交未知不能解释为未绑定，须重新核DB/store/marker分类。失败不抹去未知结果；留下的合法匹配最终marker可在下次明确准备时核对后补绑定。
- root_bound=0且仅有marker.tmp而无最终marker时，完整或部分tmp一律保守保留并报初始化待核对；不凭相似内容提升、覆盖或删除。root_bound=1不因tmp存在补建最终marker。此窗口不提供自动修复框架，明确记录为本片限制。
- root_bound=1遇根/marker缺失、错storeId、未知格式、锁占用或不能安全隔离时，不新建替代根或重写marker；保留事实并返回备份域的受控不可用/不匹配结果。
- 持有根及必要父目录句柄和首次OS身份，准备及关键I/O前后核对路径/句柄/锁/marker及相关保护路径的实际父链/映射。运行期换根或保护映射变化即持续阻断本次租约，不能因替换根带相同marker而热切换；不能复用模块启用/挂载替换前的陈旧隔离结论。不宣称抵御同权限管理员持续篡改文件系统。
- 跨重启marker只证明逻辑命名空间，不证明原物理介质；允许停服完整搬迁后重新取得根租约和核验。不能检测所有离线完整克隆，不提供两个独立克隆根之间的全局互斥。

## 4. 文件格式与所有权

- 名称只由经校验的小写32hex fileId推导：`.pending-<fileId>`和`b-<fileId>`，均为同根直接子目录；不保存/接收任意目录名。固定成员仅`owner.json`、`snapshot.db`、`manifest.json`，唯一写入中名字为`manifest.json.tmp`；最终精确三个成员。
- owner最多1024字节，精确字段`magic,formatVersion,storeId,runId,fileId`，magic=`nas-content-manager-backup-owner`、formatVersion整数1、三个ID为小写32hex。固定顺序struct json.Marshal输出且无换行；原始字节长度/摘要必须在pending意图登记前确定，不能用解析后重编码摘要代替原始字节。本片publisher核对已登记意图，不凭相似名字接管未知材料。
- manifest最多16384字节，格式1；精确字段`formatVersion,storeId,runId,fileId,scope,createdAt,appVersion,schemaVersion,files,excludedRoles`。scope固定`database-only`；files精确一个`{role:"database",name:"snapshot.db",size,sha256}`。
- excludedRoles按固定顺序精确七项且无重复：`source-secrets,helper-identity,worker-state,component-volumes,download-payloads,media,user-assets`。不是完整inventory验证结果，也不表示整栈/异地备份。
- createdAt沿用源生成完成时间，为UTC RFC3339Nano规范文本（Z结尾，解析回写一致且毫秒可表示在受检范围）；appVersion为本程序Version、1..64 UTF-8字节且无NUL。schema/长度/hash来自目标实测并与源实测匹配，不填编译常量代替实测。
- JSON用limit+1有界读入；先后核普通单链接文件、身份/长度/mtime，拒绝无效UTF-8、原始/解码NUL。逐对象token解析、精确键允许集和已解码seen键，嵌套files对象同样执行；拒绝转义重复、大小写别名、未知/缺字段及尾随非空白。整数使用UseNumber，仅无符号十进制整数词法并受检转int64，拒绝字符串/null/bool/小数/指数/负号/溢出。顶层恰一个对象后须EOF，不能只用DisallowUnknownFields或先解map丢失重复键。
- manifest禁止路径、原始私有URL、媒体清单、凭据或数据库秘密值。文件size正int64、sha256为64小写hex、schemaVersion正整数且与本次源/目标实测相等。本片不扩大历史快照导入支持。
- 每个新目录/文件排他创建；POSIX0700/0600，Windows真实protected DACL及继承核验。Lstat拒绝链接、重解析点、非普通成员、大小写别名和未知成员。成员检查以允许数量n+1 lookahead，最终最多ReadDir(4)；首次空根也按阶段固定预算，不用ReadDir(-1)后截断。

## 5. 内部发布接口与责任

```go
// acquireBackupRoot 只在明确模式下准备并绑定专属根。
func (a *App) acquireBackupRoot(ctx context.Context, mode backupRootOpenMode) (*backupRootLease, error)
// checkIdentity 核对路径、持有根、锁与marker，身份变化后持续阻断。
func (lease *backupRootLease) checkIdentity() error
// Close 幂等关闭本租约资源，保留错误并最后释放独占锁。
func (lease *backupRootLease) Close() error
// backupOwnerBytes 在登记意图前固定规范owner原始字节。
func backupOwnerBytes(storeID string, runID string, fileID string) ([]byte, error)
// publishBackupSnapshot 借用根与源，返回目标事实及发布确定性。
func (a *App) publishBackupSnapshot(ctx context.Context, root *backupRootLease, source *databaseSnapshot, runID string) (backupPublishResult, error)
```

- backupRootOpenMode为包内uint8枚举，零值仅打开已绑定根；另一值明确允许首次准备/匹配最终marker补CAS。非法枚举拒绝，零值遇未绑定不写盘。本片不从Web接收mode。
- backupPublishCertainty为包内uint8枚举，零值`needs_reconcile`、另有`not_published`/`confirmed`。backupPublishResult字段为Certainty、Metadata(snapshotMetadata)、OwnerSize(int64)/OwnerSHA256(string)、ManifestSize(int64)/ManifestSHA256(string)、Stage/Code(string)；所有字段中文注释。Stage/Code仅固定安全允许集，不承载原始异常。

- 根准备返回唯一可关闭租约；调用者在所有相关I/O结束后关闭，Close须sync.Once幂等并合并错误，先关根相关资源、最后放锁，不删除marker/锁/产物。幂等不表示Close与publish允许并发。本片不让默认启动持有一个永远未使用的根。
- publisher借用根租约、已登记pending意图和已验证源租约；同一短只读事务核同store、run running、file pending、run.file_id与file.run_id/run_seq及owner原始长度/hash一致，关闭事务后再排他创建目标。调用者提供同run唯一执行权；本片不造第二队列锁。复制/检查/移动期间不持DB事务/manager.mu。
- publisher只负责磁盘协议和目标事实，不在本片提交run completed、last-good、幂等摘要或授权轮转。已有同名pending/final不覆盖或接续；原意图遗留返回待核对，异物碰撞保持原样，不换fileId绕过。
- 源租约始终由调用者持有，借用期间不能并发读/Close；publisher不得重开、rename、删除或转移源路径。源最终Close失败和目标已发布事实分别报告；不把已完成发布说成可盲重试的新任务。
- 本片收到源时源候选已经存在；复制前按目标实际卷余量检查`free(target)>=MinFreeBytes+source实测SizeBytes+1MiB`，所有加法/类型转换先防负数及溢出，并复核根身份。单份DB+owner+manifest实际逻辑字节不得大于run.max_bytes；可先按源事实生成规范清单的预算长度，目标复验后仍须核最终实际总量。实际膨胀/超限/FULL/IOERR失败仅清本次确认候选，不删旧材料救空间；余量预检不是空间预留。
- S03.2.3在生成源前另做整体峰值准入：源卷`MinFree+2S`、不同目标卷`MinFree+S+M`，同卷或无法证明不同卷时合并`MinFree+3S+M`（M=1MiB）；不能把本片事后目标余量检查当整体峰值检查。全局usage、未知占用/轮转积压和64条ownership准入归对应后片，本片不声称已提供。
- 复制固定64KiB缓冲、检查ctx和精确长度；目标O_EXCL、受限权限、Sync/Close。目标独立只读复验须比较源实际size/hash，并额外显式比较实际schema；当前snapshotExpected没有Schema字段。
- 写manifest临时成员并Sync/Close，无覆盖改为manifest.json；复核精确成员与owner/manifest/DB身份、长度和摘要。移动前关闭所有目标SQL、摘要和成员句柄。
- 同根无覆盖发布pending目录为final后，执行平台必要同步并重新打开final核对根/目录/成员及实际内容。Windows移动前关闭所有目标子目录/成员/SQL/摘要句柄，保留根anchor/锁。成功只返回“规定OS发布协议已确认”，不是DB已登记completed或硬件断电保证。
- 结果区分明确未发布、已确认发布、移动后结果待核对；任一所需Sync/Close/移动/后验核对失败都不能返回已确认。调用过移动但结果/后续阶段未知时保留原ID与材料，不另建候选、不自动删除final、不盲重试源快照。已确认全部协议后才观察到的取消不抹去confirmed；取消阻断后验核验则待核对。
- 只有未调用最终移动且无原意图既有final/未知旧尝试，或有实际身份事实证明本次未移动的碰撞，才能not_published。error与Certainty必须同时消费；源最终Close失败与目标confirmed独立，不把成功目标改为可盲重试failed。未来完成事务失败/提交未知保留原ID对账。
- 清理只针对已登记且身份/允许成员均确认的本次对象，逐个删除，owner最后、空目录最后；未知/替换项保留。未发布残留的数据库状态转移及重启对账由下一片协调，不借deleting承载未发布清理。

## 6. 平台协议

- Windows逐文件写句柄请求WRITE_THROUGH并逐个Sync/Close；marker、manifest及目录移动仅`MoveFileExW(...,MOVEFILE_WRITE_THROUGH)`。禁止REPLACE_EXISTING、COPY_ALLOWED、延迟重启或失败后的os.Rename/copy-remove回退。
- Windows不强制一个没有相同文档语义的POSIX目录Sync，也不吞掉该错误假装完成；其完成边界仅为规定文件flush/关闭、写穿移动和后验核验成功。系统/过滤驱动/设备缓存及真实断电仍另轨待验。
- Linux使用`unix.Renameat2(dirfd,old,dirfd,new,unix.RENAME_NOREPLACE)`；dirfd来自持有的root.Open(".") File并核身份，manifest使用pending的受控dirfd。文件Sync、manifest移动后的pending目录Sync、最终移动后的备份根Sync逐个检查；新建根还同步其现存外层父目录。不得提前关闭File只保留裸fd。ENOSYS/EINVAL/EOPNOTSUPP、跨卷、碰撞或同步失败均不confirmed，不以预检存在性代替系统无覆盖保证，不回退Root.Rename/os.Rename。
- 不增加依赖、全卷管理员flush、全局环境变量或生产测试开关；平台错误只以固定中文阶段/受控代码记录，禁止秘密路径、原始SQL/URL/异常文本。

## 7. 本片可执行门禁

1. 内部临时Go行为：根隔离/双向ancestor/默认无根、首次绑定及匹配marker补绑定、错marker/空挂载/锁争用/运行期换根、Close幂等/失败释放；正反JSON与有界成员扫描。
2. 真实Windows产品publisher：合法SQLite源经固定缓冲复制、目标实际schema/size/hash/manifest匹配、文件及目录碰撞sentinel不变、真实句柄占用拒绝后同意图可核对、实际DACL和句柄释放。既有原语fixture不替代产品验证。
3. 故障注入：目标余量/单份字节边界与溢出、文件Sync/Close、manifest移动、目录移动前后、后验核对、root_bound CAS、取消；分别核对发布确定性、原意图/源归属/未知成员保留。整体同卷/不同卷源前峰值准入在S03.2.3另验。进程终止、系统崩溃、断电各自记账，不互相冒充。
4. 本片不公开生产CLI/API测试后门；稳定集成入口及重启对账由下一片接入后验收，本片内部行为覆盖不能标为异步API已经交付。
5. 新临时测试执行后精确清源；fresh同产物backup/storage/account/core回归、gofmt/vet/清源编译/Python/Vue检查及Linux双架构交叉构建。真实Linux/NAS、FULL/拔盘/断电与大库性能仍独立待验。
6. 实施者停写后独立可写检查，再由主线同步正式契约/文档。当前只有实施契约冻结，无本片上述门禁通过声明。
