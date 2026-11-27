# S03.2.1 v6存储与账号维护兼容契约

2026-11-23。S03.2.1实施契约已由主线结合独立接缝核对冻结，并由info引用为本片权威。冻结不代表迁移已实施或验收。上片S03.1已完成本机独立验收，整个S03仍实施中。

## 1. 本片范围与明确不提前实现的部分

- 只交付五张STRICT表、v5→v6原子增量迁移、当前快照结构探针、显式v5/v6离线账号维护兼容及稳定隔离验证。
- 不增加持久备份API、配置项、备份根/marker/锁、文件生成、executor、调度、历史裁剪或轮转。正常启动只初始化存储，默认关闭；不因迁移新建backups目录。
- 原GET导出继续共用S03.1租约。本片checker仍**只接受当前头6**并增加五表核心结构探针；不增加公开历史快照核验接口或snapshotExpected.Schema字段。旧v5库的离线账号维护明确支持；历史快照verify/恢复准备的版本支持由S03.3另行冻结，不能混称当前checker已支持5。
- 后片目标副本复验仍须另外显式比较源快照实际schema；不能因expected当前仅大小/摘要就声称已经核过manifest schema。

## 2. 迁移与通用类型

新增`internal/app/backup_store.go`，以冻结raw常量`schemaV6`保存DDL，方法`migrateBackupSchema(ctx context.Context, tx *sql.Tx) error`只操作调用者提供的迁移事务；`openDatabase`在v<6时开启短事务，调用它、设置user_version=6后同笔提交。成功后输出安全中文升级日志；失败全回滚，不跳过冲突对象、不修补不明表或改写v1–v5常量。

迁移内一次采实际UTC毫秒N（0..253402300799999），一次crypto/rand生成16字节storeId并小写hex编码。随机源/时间失败回滚；policy的effective/updated同为N；store时钟高水位初始0。重启头已6时不得重新随机化store或重置策略/授权。fresh库正常依次经过全部固定旧迁移。

随机错误的可测边界：窄clock/io.Reader参数允许实际注入可返回的EOF/short错误并验证事务回滚；生产使用标准crypto/rand.Reader。固定Go1.27.4默认Reader的底层熵灾难可能由运行时不可恢复终止，不能把注入reader回滚写成该类生产fatal已受控返回或已动态验过崩溃恢复；不为绕过标准库行为新造系统随机源或修改全局Reader。

- 下述时间戳均为INTEGER毫秒，必填范围0..253402300799999；可空时间非NULL时同范围。修订/seq为正INTEGER（Go int64），计数非负。不得要求finished>=started或updated>=created，墙钟可能回拨。
- 本地ID固定32位小写ASCII hex；摘要固定64位小写ASCII hex。必填/可空ID都拒绝错误长度、大小写、非hex和NUL（不能依赖会在NUL截断的单一SQLite length检查）。
- JSON字节上限按`length(CAST(value AS BLOB))`计算；中文不能按字符数代替字节数。所有DDL字段均有中文注释。所有方法显式类型及中文方法注释，关键分支中文行注释，业务日志不得带凭据/路径/原始SQL。
- 本片不新增表间FK、级联或触发器；账号表原结构不变。跨表归属/CAS/有界裁剪由未来同事务服务维护，不能把无FK误称为跨表关系已由数据库保证。

## 3. 五表字段与数据库约束

### backup_policy：精确一条默认策略

`id INTEGER PRIMARY KEY CHECK(id=1)`；`revision INTEGER NOT NULL CHECK>=1`；`enabled INTEGER NOT NULL CHECK IN(0,1)`；`hour_utc INTEGER NOT NULL 0..23`；`minute_utc INTEGER NOT NULL 0..59`；`keep_count INTEGER NOT NULL 1..30`；`max_bytes INTEGER NOT NULL 67108864..1099511627776`；`schedule_revision INTEGER NOT NULL CHECK>=1`；`effective_from_ms INTEGER NOT NULL`；`schedule_cursor_ms INTEGER NULL`；`next_run_ms INTEGER NULL`；`updated_at_ms INTEGER NOT NULL`。

初始化精确一行：id/revision/schedule_revision=1，enabled=0，03:00UTC、keep7、max10737418240，effective/updated=N，cursor/next=NULL。数据库约束disabled时next必须NULL，enabled时next非NULL；不增加跨时钟时间顺序CHECK。未实现的API不能开放enabled=true；迁移默认不启用。

### backup_store：每库身份与可持久授权

`id INTEGER PRIMARY KEY CHECK(id=1)`；`store_id TEXT NOT NULL UNIQUE`（32hex）；`root_bound INTEGER NOT NULL DEFAULT 0 CHECK IN(0,1)`；`request_clock_high_water_ms INTEGER NOT NULL DEFAULT 0`；`history_truncated_count INTEGER NOT NULL DEFAULT 0 CHECK>=0`；`last_good_file_id TEXT NULL`（32hex）；`last_success_json TEXT NOT NULL DEFAULT 'null'`；`last_failure_json TEXT NOT NULL DEFAULT 'null'`；`retention_authorizer_file_id TEXT NULL`（32hex）；`retention_authorized_keep_count INTEGER NULL 1..30`；`retention_authorized_max_bytes INTEGER NULL 67108864..1099511627776`。

摘要必须json_valid、JSON类型object或null且UTF-8字节<=2048；本片只写初始'null'。授权三字段CHECK全NULL或全非NULL，迁移全NULL。唯一单例初始root_bound=0/H=0/truncated=0、lastgood=NULL、两摘要'null'，不绑定或探测根。last-good不是授权；授权NULL不能由历史completed/excess/wake重开。

### backup_runs：执行事实

`seq INTEGER PRIMARY KEY AUTOINCREMENT CHECK(seq>=1)`；`id TEXT NOT NULL UNIQUE`（32hex）；`request_id TEXT NULL UNIQUE`；`trigger TEXT NOT NULL IN('manual','scheduled')`；`schedule_key TEXT NULL UNIQUE`；`slot_ms INTEGER NULL`；`schedule_revision INTEGER NULL >=1`；`policy_revision INTEGER NOT NULL >=1`；`keep_count INTEGER NOT NULL 1..30`；`max_bytes INTEGER NOT NULL`（上述预算范围）；`state TEXT NOT NULL IN('queued','running','completed','failed','interrupted')`；`cancel_requested_at_ms INTEGER NULL`；`created_at_ms/updated_at_ms INTEGER NOT NULL`；`started_at_ms/finished_at_ms INTEGER NULL`；`file_id TEXT NULL`（32hex）；`error_code TEXT NULL`；`message TEXT NOT NULL`（UTF-8<=256字节）；`skipped_slots INTEGER NOT NULL DEFAULT 0 CHECK>=0`；`admission_deadline_ms INTEGER NOT NULL`。

- manual要求request_id非NULL、schedule_key/slot/schedule_revision全NULL；scheduled要求request_id=NULL且计划三项全非NULL。request_id存储约束为精确36字节、规范小写带连字符UUIDv7（位置/版本/variant/hex均检查）；schedule_key为1..96字节、允许集仅`[0-9s:]`、无NUL，精确s1/revision/slot语义留后片解析。
- queued：started/finished/file_id均NULL；running：started非NULL、finished=NULL；终态：finished非NULL，completed要求started非NULL，failed/interrupted允许started=NULL。
- queued/running无error_code；completed必须file_id非NULL、error_code=NULL；failed/interrupted必须非空受限error_code（1..64字节，允许集仅`[a-z0-9_]`且无NUL）；本片不造运行记录。
- 增加常量表达式部分唯一索引：`state IN('queued','running')`全表最多一行。正常列表未来按seq DESC，不按墙钟排序。

### backup_files：独立文件所有权

`id TEXT PRIMARY KEY`（32hex）；`store_id TEXT NOT NULL`（32hex）；`run_id TEXT NOT NULL UNIQUE`（32hex）；`run_seq INTEGER NOT NULL UNIQUE CHECK>=1`；`state TEXT NOT NULL IN('pending','retained','deleting','missing','unverified','deleted')`；`created_at_ms INTEGER NOT NULL`；`published_at_ms/verified_at_ms/deleted_at_ms INTEGER NULL`；`size_bytes INTEGER NULL CHECK>0`；`sha256 TEXT NULL`（64hex）；`schema_version INTEGER NULL CHECK>0`；`app_version TEXT NULL`（1..64UTF-8字节）；`manifest_size INTEGER NULL 1..16384`；`manifest_sha256 TEXT NULL`（64hex）；`owner_size INTEGER NOT NULL 1..1024`；`owner_sha256 TEXT NOT NULL`（64hex）；`error_code TEXT NULL`（同受限格式）。

retained/deleting要求完整发布元数据：published/verified/size/sha/schema/app_version/manifest_size/manifest_sha均非NULL；任一状态只要published非NULL，发布tuple其余字段也必须全非NULL。pending→unverified可尚无完整发布元数据，不用“非pending都必须完整”的CHECK阻断故障对账；已发布后进入missing/unverified保留原元数据是未来状态服务责任。本片不给未来每个状态强行新增时间顺序关系。deleted必须deleted_at非NULL，其它状态deleted_at=NULL；未发布残留在原意图下精确清理后可直接→deleted，不要求曾有发布tuple。

增加`state='deleting'`的常量表达式部分唯一索引，最多一个确切删除意图。deleting只承载completed/retained备份的轮转认领，不承载未发布残留清理。没有从runs级联删除files；不保存目录/宿主路径，后片只能由fileId推导固定名字。文件处于deleting不能恢复retained冒充完整。

### backup_request_keys：幂等账本

`request_id TEXT PRIMARY KEY`（同UUIDv7格式）；`request_time_ms INTEGER NOT NULL`；`accepted_at_ms INTEGER NOT NULL`；`expires_at_ms INTEGER NOT NULL`；`run_id TEXT NOT NULL UNIQUE`（32hex）；`run_seq INTEGER NOT NULL CHECK>=1`；`terminal INTEGER NOT NULL DEFAULT 0 CHECK IN(0,1)`；`summary_json TEXT NOT NULL`（json_valid、JSON object、UTF-8<=2048字节）。CHECK request_time<=时间上限-86400000且expires_at=request_time+86400000，不以accepted_at计算续期；UUID内48位时间与request_time相等的业务语义留后片严格解析。

### 尚未可声称数据库强制保证的关系

200运行/64非deleted所有权/2048有效键是后片准入和裁剪的应用上限，本片不加跨表计数触发器或虚假声称已提供运行时容量管理；只测DDL可表达所有有效状态。来源授权对应同store且实际completed/retained、run/file/key一致、引用中禁止裁剪等，未来须短事务CAS维护。`RowsAffected=1`、高水位只升不降与修订溢出拒绝均由对应后片服务实现，本片不新增无调用者的通用CRUD框架。

## 4. 账号兼容与快照接缝

- 离线入口用同一显式支持函数接受集合{5,6}，小于5/大于6分别固定中文拒绝。`offlineAccountSnapshot`新增user_version（Go int64，中文字段注释）；预检读取固定，提交要求版本仍受支持且**等于预检值**，同时保留已有schema_version/hash/revision/ID CAS。5预检后改6与6改5均拒绝，不能把“均受支持”当允许漂移。
- 离线只核账号对象及既有反向FK/触发器安全前提，不读策略、不迁移、不对账、不新建备份根。reset成功仅password_hash/auth_revision/sessions变化；五备份表结构/记录、pending文件sentinel和其它业务必须原样。
- `verify_schema.py`扩为真实v1–v5构造：v5使用冻结schemaV5和schemaV5SessionConstraints，在正确顺序复制来源已有合法账号/会话列；没有source时空账号可正常构造。旧v1–v4仍只有旧账号列。禁止把当前v6库改user_version=5冒充历史结构。
- `verify_account_offline.py`future反例改7，明确真实v5与真实v6成功、拒绝、预检版本barrier、秘密输出和零备份副作用；原Console/ACL覆盖继续保留。最新头断言同步verify.py/verify_backup.py/verify_account.py及CodeGraph查出的其它当前头断言；历史固定v4/v5反例不机械替换。

## 5. 本片可执行门禁

1. 临时Go：真实v5→v6，保留旧表/账号hash/revision/旧Cookie；fresh/重复启动storeId稳定；冲突表导致五表/单例/头全回滚；未来7拒绝。五表STRICT、字段边界/字节长度/NUL/UUID/时间/授权全有全无/单active/单deleting及合法故障状态均有反例。
2. 临时Go：离线5/6均支持，双向user_version漂移（schema_version不变）拒绝；真实6含非空备份域和pending sentinel，成功重置仅账号变化。保留已有认证/CAS/取消/提交未知前提，不把无测试编译记为行为通过。
3. 新稳定`scripts/verify_backup_store.py --binary <明确产物> --root <全新.runtime根>`：以真实旧结构与实际app启动迁移验证上述保留性、默认关闭/无备份根、稳定storeId、future/失败迁移保留性和旧Cookie；记录产物SHA及明确coverage。可复用既有隔离OwnedProcess与验证器，不接触用户数据。
4. fresh构建后同一Windows产物执行backup、core --large-file、account --offline以及新storage专项；断言数按实际新增记录，不能硬套S03.1的38/52/157。原始秘密扫描先于脱敏；正常停服证据与强制兜底分开。
5. gofmt/vet、临时行为验证后精确删除本片源、清源全包编译、Python AST/类型标注、Vue typecheck；Linux amd64/arm64交叉构建与真实NAS待验分别记账。实施者先停写，独立checker复核修复后再由主线更新正式spec/docs/任务。未提交、部署或归档。
