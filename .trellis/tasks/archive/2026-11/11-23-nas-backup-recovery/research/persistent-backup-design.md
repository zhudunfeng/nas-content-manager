# S03.2 持久策略、执行与轮转设计准备

日期：2026-11-23。**全文均为研究推荐，尚未实现、冻结或验证通过。** 本轮按 Trellis Phase 1.2 只读核对源码，仅写本文件；不切换当前账号任务、不修改 S03.1 设计、正式 spec、PRD、上下文或业务代码。冻结时需将本文件选定的差异提炼到 `../info.md`，不能直接把研究写成已交付契约。

## 1. 已核对的现状与实施接缝

检索先用 CodeGraph CLI，再读实际文件；新增账号 CLI 尚未进入现有索引，相关空结果已用定向文件读取补核，不更新其他代理正在使用的索引。

| 当前源码 | 核对事实 | S03.2 接缝，均待实现 |
| --- | --- | --- |
| `internal/app/config.go:14–101` | Config 无备份配置；LoadConfigWithLookup 不改全局环境，最后加载下载/收集/引擎配置 | 在相同 lookup 快照解析 BackupDir；完整配置齐备后再交叉核验根隔离，不新增进程全局可变配置。 |
| `internal/app/store.go:17–165` | 单连接、WAL、短事务；当前头5；启动会恢复 jobs/外部动作并清缓存 | 在真正 v5 上追加 v6 迁移，不重写 v1–v5；备份恢复另设入口，不能把 running 备份套用 jobs 的自动重新排队。 |
| `internal/app/server.go:73–158` | 先数据目录锁/监听器，再开库、启动各管理器；退出取消 workCtx、关连接、等 handler/wg 后关库和数据锁 | 新管理器固定一个 executor，先完成本模块对账才开放新备份准入；加入 a.wg；备份根租约在 handler/executor 全部退出后才释放。 |
| `internal/app/media.go:75–122`、`../info.md` | 源码还是旧同步路径；S03.1 已冻结为私有快照租约，覆盖生成、校验、HTTP、Close 清理 | 将 S03.1 当待实现依赖，禁止另写 VACUUM 路径，也不修改其 Keep/Detach 限制。持久化从租约句柄流式复制到目标卷，不能重开、rename 或删除租约自有源文件。 |
| `internal/app/jobs.go:57–125` | 短事务领取，领取/取消句柄安装受锁保护，收尾后才释放执行占位 | 复用这种所有权思想，但使用独立 backupManager 的锁、取消句柄、wake，不能占用扫描 worker 或复用 job ID。 |
| `internal/app/instance.go`、平台锁文件 | canonical 根的 OS 独占，持锁文件不删除 | 数据锁不足以阻止不同 DataDir 误用同一个备份根；增加该根自己的独占租约，复用平台锁原语，不按 PID/端口/锁文件存在判断。 |
| `internal/app/account_offline_store.go:45–100,120–193,228–263` | 受限入口只支持 v5；预检和写事务各有版本判断；检查账号结构、触发器、反向外键 | v6 交付必须同时扩展两个显式支持判断及对应验证；不能只改应用当前 schema 常量。 |
| `internal/desktop/launcher.go:376–381` | native 覆盖地址/数据/媒体，其他键来自只读 os.Getenv | 原生 NAS_BACKUP_DIR 可沿用 lookup；不增加 desktop.json 字段，也不 Setenv。 |
| `internal/desktop/docker_compose.go:169–192`、Compose 模板 | 桌面 Docker 用固定环境及卷身份检测，不自动转发任意宿主环境 | 默认 `/data/backups` 可直接沿用现有 data 卷。本片不增加任意宿主挂载。若显式向固定模板增加环境键，模板、serviceEnvironment、已有容器检测/升级兼容须同片核对；不能只改一份 YAML。 |

现有固定依赖为 Go 1.26.0、modernc.org/sqlite v1.62.0；账号实施工作区已固定 x/term。本研究不改依赖。原 `GET /api/backup` 的鉴权、文件响应、锁和错误映射继续由 S03.1 冻结契约负责。

## 2. 推荐的最小范围与固定限额

- 只做 database-only 持久备份；一条每日 UTC 固定时分策略，升级后默认关闭；允许策略关闭时手动创建持久备份。禁用只停止未来计划准入，不删文件；已接受任务继续完成，需中止时调用其明确取消接口。
- 建议初始值：`hourUTC=3`、`minuteUTC=0`、`keepCount=7`、`maxBytes=10737418240`（10 GiB）。时/分范围 0..23、0..59；keepCount 1..30；maxBytes 为精确 JSON 整数，67108864..1099511627776（64 MiB..1 TiB）。字节上限小于 2^53，不能收浮点、指数、小数字面量或数字字符串。
- 模块固定上限：运行行总数200（包含活跃行和文件保留保护行）、非 deleted 文件目录记录64、有效手动去重键2048、同时 queued/running 合计1、持久文件并发下载2。不是新增可调环境参数；未来扩大须改契约与验证。
- 单 executor，容量1的合并 wake；一个可复用 timer：争用旧快照锁时约1秒再试，空闲调度最迟30秒检查一次，租约释放/新请求可提前唤醒；不得每个任务/文件/计划点起 goroutine。新异步备份等待锁最多30分钟，实际生成/复制/校验另有30分钟的应用子 context；不改变旧同步下载和媒体的超时。
- API 默认 limit20，1..100；只接受定义的 query，拒绝重复/空值/未知值。时间输出固定 UTC RFC3339 毫秒格式；本模块排序和计划使用整数 Unix 毫秒/稳定序号，不比较现有可变精度 RFC3339 文本，也不改账号域时间语义。

## 3. schema6：精确的数据责任与约束

下表是建议迁移定义，**不是现有表**。五张新表采用 STRICT；INTEGER 在 Go 为 int64，布尔值用0/1 CHECK；毫秒时间为 0..253402300799999。自增序号、策略修订等不能以 JS number 穿过接口：revision/序号游标用十进制字符串，达到 MaxInt64 明确拒绝，不能归零。随机本地ID为 crypto/rand 生成的32位小写hex，DB同时约束长度/字符。所有字段须有中文字段注释；所有方法、配置和关键业务分支遵守中文注释/安全中文日志要求。

### 3.1 backup_policy：唯一用户策略

| 字段 | 类型/空值/初始值 | 约束与含义 |
| --- | --- | --- |
| id | INTEGER PK，1 | CHECK(id=1)，迁移精确插入一行。 |
| revision | INTEGER NOT NULL，1 | >=1；任何实际配置变更CAS推进；相同 payload 不推进。 |
| enabled | INTEGER NOT NULL，0 | IN(0,1)。 |
| hour_utc / minute_utc | INTEGER NOT NULL，3/0 | 上述时分范围；不是宿主时区或 cron。 |
| keep_count / max_bytes | INTEGER NOT NULL，7/10GiB | 上述固定范围；max_bytes 是已知自有逻辑文件字节保留目标，非卷已分配块数/硬配额。 |
| schedule_revision | INTEGER NOT NULL，1 | >=1；仅 enabled/时/分实际变化推进。只改保留参数不产生新的计划身份。 |
| effective_from_ms | INTEGER NOT NULL | 当前 schedule_revision 生效时刻；新启用或改时分从此后第一个计划点开始，不补停用期间或新策略生效前的点。 |
| schedule_cursor_ms | INTEGER NULL | 本 schedule_revision 已持久接受的最新 slot；仅同一修订内单调增加。新 schedule_revision 显式重建游标。 |
| next_run_ms | INTEGER NULL | 仅启用时有值；是下一未处理计划点的缓存，和 cursor/策略在同一短事务更新，启动必须重新推导，不能把缓存当去重凭据。 |
| updated_at_ms | INTEGER NOT NULL | 原始观察到的UTC时间；不以它排序历史，不强加跨时钟回拨的先后CHECK。 |

### 3.2 backup_store：每库持久身份和少量运行摘要

唯一 id=1；`store_id TEXT NOT NULL UNIQUE`（32hex，迁移用随机数生成）、`root_bound INTEGER NOT NULL DEFAULT 0 CHECK IN(0,1)`、`request_clock_high_water_ms INTEGER NOT NULL DEFAULT 0`、`history_truncated_count INTEGER NOT NULL DEFAULT 0 CHECK>=0`、`last_good_file_id TEXT NULL`、`last_success_json TEXT NOT NULL DEFAULT 'null'`、`last_failure_json TEXT NOT NULL DEFAULT 'null'`。两个摘要是固定安全DTO的JSON，json_valid且UTF-8字节长度<=2048；不存原始异常/路径/凭据；无业务外键级联。history_truncated_count在实际裁剪run的同一短事务按删除数递增，达到MaxInt64后饱和不溢出；只用于说明历史曾被截断，不称永久审计总数。

`request_clock_high_water_ms` 只属于本数据目录的模块状态，不是进程全局变量/环境变量/签发密钥；仅由实际观察时钟推进，绝不能被客户端UUID的时间推进。last_good只是候选指针，仍须核验文件；摘要保留最近成功及最近非用户取消的 failed/interrupted 事实，不能因200行历史压缩丢失最后失败信息。GET不写高水位；在准入、裁剪或每分钟一次调度检查的短事务中先持久推进，再据此做过期决定。

### 3.3 backup_runs：有界执行事实

| 字段组 | 精确建议 |
| --- | --- |
| 身份 | `seq INTEGER PRIMARY KEY AUTOINCREMENT`；`id TEXT NOT NULL UNIQUE`（32hex）；`request_id TEXT NULL UNIQUE`；`trigger TEXT NOT NULL CHECK IN('manual','scheduled')`。 |
| 计划 | `schedule_key TEXT NULL UNIQUE`；`slot_ms INTEGER NULL`；`schedule_revision INTEGER NULL`；`policy_revision INTEGER NOT NULL`。manual要求request_id非空且计划三项全NULL；scheduled相反。 |
| 冻结输入 | `keep_count INTEGER NOT NULL`、`max_bytes INTEGER NOT NULL`：记录准入时策略；执行不会因随后编辑被偷偷改成另一计划。轮转用何版策略见第8节。 |
| 状态 | `state TEXT NOT NULL CHECK IN('queued','running','completed','failed','interrupted')`；`cancel_requested_at_ms INTEGER NULL`；`created_at_ms INTEGER NOT NULL`、`updated_at_ms INTEGER NOT NULL`；`started_at_ms/finished_at_ms INTEGER NULL`。queued无started/finished；running有started无finished；终态有finished，允许未实际开始就failed/interrupted。 |
| 结果 | `file_id TEXT NULL`；`error_code TEXT NULL`；`message TEXT NOT NULL`（<=256 UTF-8字节、固定安全中文）；`skipped_slots INTEGER NOT NULL DEFAULT 0 CHECK>=0`；`admission_deadline_ms INTEGER NOT NULL`。completed必须有file_id且无error_code；失败/中断必须有受控error_code；时间回拨不以finished>=started作为真实性判断。 |

索引：`UNIQUE INDEX ... ON backup_runs((1)) WHERE state IN('queued','running')`，数据库层保证单个已接纳执行占位；列表按seq DESC；request/schedule唯一值的规范格式同时由严格应用解析核验。所有状态CAS必须检查RowsAffected=1。没有重试同一个run的入口；失败后明确新意图使用新requestId，避免旧取消句柄命中新一轮。

### 3.4 backup_files：文件所有权目录，不由历史外键拥有

`id TEXT PRIMARY KEY`（fileId32hex）、`store_id TEXT NOT NULL`、`run_id TEXT NOT NULL UNIQUE`、`run_seq INTEGER NOT NULL UNIQUE`、`state TEXT NOT NULL CHECK IN('pending','retained','deleting','missing','unverified','deleted')`、`created_at_ms INTEGER NOT NULL`、`published_at_ms/verified_at_ms/deleted_at_ms INTEGER NULL`、`size_bytes INTEGER NULL CHECK>0`、`sha256 TEXT NULL`（64小写hex）、`schema_version INTEGER NULL CHECK>0`、`app_version TEXT NULL`（<=64字节）、`manifest_size INTEGER NULL`（1..16384）、`manifest_sha256 TEXT NULL`、`owner_size INTEGER NOT NULL`（1..1024）、`owner_sha256 TEXT NOT NULL`、`error_code TEXT NULL`。

- 目录名由fileId推导，固定 `.pending-<fileId>` / `b-<fileId>`，不能存/接收任意路径。成员只有owner.json、snapshot.db、manifest.json及明确的manifest写入中临时名。发布后长度/摘要/schema等必填；deleting/missing/unverified仍保留原发布元数据。
- 不对 backup_runs 做 ON DELETE CASCADE；文件身份及精确成员所有权独立保留。通常历史裁剪也保护仍有非deleted文件的run，最大64个保护行，故总200行仍可有界；因此“200行”不保证全是最近200条普通终态。
- 删除成功且目录已消失才置deleted。deleted行无活跃引用且其run已裁剪时才能移除；终态压缩后无需保留永久deleted墓碑。非deleted达到64则拒绝新的持久备份，保守保留待核对材料，不通过逐出ownership解限。

### 3.5 backup_request_keys：有界幂等账本

`request_id TEXT PRIMARY KEY`（规范UUIDv7）、`request_time_ms INTEGER NOT NULL`、`accepted_at_ms INTEGER NOT NULL`、`expires_at_ms INTEGER NOT NULL`、`run_id TEXT NOT NULL UNIQUE`、`run_seq INTEGER NOT NULL`、`terminal INTEGER NOT NULL DEFAULT 0 CHECK IN(0,1)`、`summary_json TEXT NOT NULL`（固定RunSummary，json_valid且<=2048字节）。expires_at_ms固定等于request_time_ms+86400000，checked算术且满足本模块时间范围；accepted_at_ms仅记录准入事务观察到的实际UTC时间，不决定或延长有效期。无指向run的级联外键；接受run和key同事务，终态和摘要更新同事务。

保留完整run可直接重放；run裁剪后返回安全终态摘要，仍是相同runId而不是创建新run。账本最多2048；只清理已过窗口且terminal=1的键，活跃键不按年龄/最近访问逐出。满额503而不丢未过期键。账本不承担文件所有权。相较永久保存完整run，这保留了有界的重试事实；相较只留“用过”位，能解释原任务结果，但过窗后不保证完整历史可查。

### 3.6 v5→v6与离线账号维护必须一起交付

1. 正常在线迁移在一笔短事务新建以上对象并初始化单例，最后 `PRAGMA user_version=6`；失败全回滚。保留所有旧表/ID/账号hash/revision/会话原文，不先启动任何备份任务；不回写旧迁移的固定版本。
2. 正常头版本与S03.1只读checker同步到6，checker补必要结构探针；历史v5快照兼容必须另行明确，不能因为把current常量改6就假定旧恢复工具已经支持。新的完整快照会包含自身run的running时点事实，这是正常一致性时点，恢复时不能因此重跑它。
3. 当前 `inspectOfflineAccount` 的 `<5 / !=5` 与 `resetOfflineAccountPassword` 的 `version!=5` **两处都必须改为显式支持集合{5,6}**，复用同一只读版本判定函数；expected快照还需携带user_version，提交要求仍等于预检版本，不能允许预检5/提交6。拒绝<5及>6，绝不迁移。
4. v6不改变users/sessions，维持现有列、触发器、反向FK检查；新备份表不引用session或密码/revision字段，不让重置触发备份写入。离线维护不打开/建备份根、不读策略、不做对账/轮转/任务恢复，成功仅hash/revision/sessions变化。
5. 稳定 `verify_account_offline.py` 当前以6作future反例，需改成7，新增真实v5和真正迁移后的v6成功/拒绝/CAS/秘密输出场景；`verify_account.py`、`verify.py`、`verify_schema.py` 的最新头断言同步，但旧v4/v5夹具仍保持真实旧结构，不能只改user_version伪造。任何一项缺失都不验收v6，避免升级备份后丢失忘记密码入口。

## 4. 根路径、私密材料和文件身份

- 仅宿主 `NAS_BACKUP_DIR`，空值默认 canonical DataDir/backups；显式值必须本地绝对目录，无NUL，不支持UNC/SMB/NFS或未验证的远程原子语义；浏览器完全不接收/返回根路径。固定Docker桌面仅支持原data卷内默认根，不因宿主环境自动增加挂载。
- 根不能等于/包含DataDir；位于DataDir内只允许精确默认backups子目录，禁止cache/private等其它子树。与所有媒体根、Download.Root（即使当前禁用）、已配置Collection.ReadRoot、引擎connections/私密材料根做双向ancestor检查。先解析现有父路径、符号链接/junction/大小写及卷别名，再按路径分段比较，不能字符串前缀判断；缺失媒体挂载也不能被当成不存在而绕过隔离。
- 初次只接管新建/空专属目录，或和本DB store_id匹配的已知标记。根下固定 `.nas-backup-store.json`（<=1KiB，magic/formatVersion/storeId，非秘密）和独占锁；root_bound=0时可在标记成功落盘后CAS绑定，崩溃留下匹配标记可完成绑定。root_bound=1后缺标记/根换实体/空挂载一律blocked，不能悄悄初始化到拔盘后的空目录。
- 保留根目录句柄、原始身份，关键I/O前后确认当前路径/句柄/标记仍同一实体；子目录/成员Lstat拒绝链接、非普通文件和未知成员。优先用 os.Root 的相对访问与路径局限能力，再做身份复核；os.Root会允许某些根内链接，不能只因使用它就宣称已拒绝所有链接。不能宣称能抵御有同权限管理员持续篡改文件系统。
- 目录0700、文件0600；Windows必须实际核验继承ACL，chmod不代表ACL隔离。源DB和目标目录都可能含敏感索引/用户名/密码hash/会话hash/私密引用；database-only绝不代表公开数据。manifest只放安全ID和摘要，不放密码/hash值、令牌、原始私有URL、绝对路径、媒体清单、主机/卷身份秘密。
- 本片不读取/复制来源秘密文件、helper/nonce身份、worker状态、组件卷、下载载荷、媒体或用户原件。manifest使用固定 `scope:"database-only"` 及 `excludedRoles`：`source-secrets,helper-identity,worker-state,component-volumes,download-payloads,media,user-assets`；这些只是未包含声明，不冒充S03.3的完整inventory核验结果。默认同盘不是异地副本，SHA-256也不是来源认证/加密。
- 变更NAS_BACKUP_DIR不自动搬迁或清除原root：要求停服后完整移动原专属根及标记，再核验启动；新空根不能替代已绑定根。不同DataDir共用根、错误storeId、未知目录需只读报告并人工核对，不自动“导入”或改owner。

## 5. UTC调度、错过合并与时钟

### 5.1 固定算法

对一次tick只读取一次原始 `nowUTC`，截取UTC日界D，计算 `today=D+hourUTC小时+minuteUTC分钟`；`latestDue=today`（now>=today）否则前一天同一UTC时刻。不遍历历史日期，不用本地时区/24小时sleep累加，不受DST影响。

- 新启用/改时分：schedule_revision++，effective_from=本次实际now；cursor=NULL，next为严格晚于effective_from的首个slot。因此正好计划点修改也从下一点开始，避免一次PUT立即补旧任务。只改keep/max不改这些字段。
- 对同一schedule_revision，候选必须 `latestDue>effective_from` 且 `cursor IS NULL 或 latestDue>cursor`。无active占位时短事务复读策略/修订，用 `s1:<十进制schedule_revision>:<十进制slot_ms>` 唯一键插入queued，并同事务把cursor置latestDue、next置latestDue后一天。cursor持久化后，即使run/schedule_key历史已裁剪也不会重跑。
- 错过多个slot只接受latestDue。`skippedSlots`为本修订此前未处理首点到latestDue之间的点数减1，checked算术计算，不制造每周期一条失败/跳过行。cursor不是“成功点”；失败/取消也消耗该计划点，不同tick不自动重试失败计划。下一日是新意图。
- 有active占位不推进cursor；下次空闲重新算最新Due，跨多日仍只补最近一个。旧GET的快照锁被占用时可已存在queued，但不增加第二个run；executor按第6节退回queued等待。
- 修改/禁用不偷偷取消已接纳旧修订run，它继续使用冻结输入并展示原修订；新计划只按当前修订准入。scheduled领取时必须匹配其已持久run，不能拿当前策略重写schedule_key。

例：日计划03:00，cursor为11月12日03:00，11月23日12:00恢复，只创建11月23日03:00一条，skippedSlots=10；重复tick/重启不再创建。随后回拨到11月20日不会重新接受cursor之前的点。修改策略会创建明确新schedule_revision，不宣称不同策略修订之间永远只有一个相同墙钟日期的备份。

### 5.2 UUIDv7与有限重试：必须修正原PRD的无限承诺

原草案“重复requestId返回原run”没有期限；它与永久有界历史/去重存储无法同时成立。**推荐新API只接受规范小写、带连字符、variant正确且版本位为7的UUIDv7**；从前48位取Unix毫秒，语法/位检查不依赖新增库。它是幂等键，不是凭据、可信时钟或防恶意限流令牌。当前尚无此API，不存在UUIDv4兼容迁移负担；冻结时须同步修正PRD/API说明。

推荐窗口W=86400000毫秒（24小时），未来容忍F=300000毫秒（300秒）。令T为UUIDv7内的requestTime，**expiresAt精确为T+W，不是acceptedAt+W**；返回值一经接受不续期。有效时钟E=max(已提交request_clock_high_water_ms,本次实际now)，客户端时间从不推进E；同一准入短事务固定这一E，先持久推进真实观察到的高水位、处理账本，再裁剪过期终态。新键时间允许区间精确为`E-W<T<=E+F`：T=E-W已经过期，T=E+F允许，T>E+F拒绝。因此接受时相对有效时钟E的剩余窗口满足`0<expiresAt-E<=W+F`；相对原始acceptedAt还受允许的小回拨影响，不能对用户另称“接受后完整24小时”。若实际时钟回退超过高水位300秒，暂停**新的**手动/计划准入，返回503 clock_rollback；恰好回退300秒仍按上述E判断，已接受执行/已知键查询/取消/下载/关闭策略仍可用。

精确重放顺序：语法合法后先查已知执行事实，已知非终态即使`E>=expiresAt`仍返回原run（不能另建，也不修改原expiresAt）；已知终态在`E<expiresAt`时返回原run或压缩RunSummary、同一runId，`historyRetained`明确，在`E>=expiresAt`时返回410，可通过先前runId查询仍保留的文件。未记录的键若`T<=E-W`返回410、绝不创建新run；若`T>E+F`返回409 request_time_ahead并附serverTime/允许窗口，余下才走新准入与容量检查。终态键清理条件同样是`E>=expires_at_ms`，不能按accepted_at_ms另算。若无对应账本但runs仍有该requestId，同样先按该run重建相同有效期判断并重放或410，不能再插入；唯一约束是最后防线。

比较：只保存200条run会让旧键失忆重跑；永久保存每个UUID/哈希不满足有界；有限时间键+2048条账本能在容量满时保守503，不驱逐有效键。只存used位不能恢复原结果，故推荐<=2KiB终态摘要；压缩不保留密码、路径或异常原文，不称永久审计。未知结果不得自动换新requestId重发；先用原键/原runId核对，确认是新业务意图才生成新键。

运维成本必须明说：UUIDv7生成器和时间错误比v4复杂；GET返回实际serverTime、高水位及可接受窗口，但不修系统时钟。短回拨等待追平；NTP误跳到远未来又回正会使高水位保守阻断新持久任务，不能自动降低、清空去重表或改user_version，否则旧键可能复活。S03.2的可操作退路是关闭计划、修正UTC/NTP、继续用兼容GET导出临时快照并单独保管，等真实时间达到安全窗口再启用。若不能等，需后续独立、显式的停机“重建备份命名空间”维护设计，保全旧根/历史并换storeId后再接纳；本片不偷偷实现清零高水位或签发密钥。冻结时需接受这一保守运维限制，不能宣称已提供自动时钟修复。高水位保证限于正常重启的本DB谱系，不跨管理员主动回滚整库/时钟的任意历史回退作永久幂等承诺。

## 6. 单执行器的领取、取消与关闭

1. API先完成严格输入/鉴权和必要的非事务预检；backupManager.mu只保护短准入事务、活动取消句柄与文件租约状态，不包文件I/O。短事务再次核验当前账号会话仍有效及账号修订，提交前检查ctx，防止已撤销会话晚提交新的持久操作；不在bcrypt或快照阶段持事务。
2. POST新意图在同事务插入queued+去重key后返回202，执行context来自a.ctx而不是HTTP请求，关页/丢响应不取消已接受任务。相同键先返回原事实，不因busy/账本满再建任务；新键遇active则409，不无限排队。
3. executor在mu内短事务CAS queued→running并安装该run的唯一cancel，然后释放mu，先调用S03.1获取有效源租约。若仅返回busy，此时没有本run的持久目标意图/目录：在同一执行所有权下检查取消/关闭，再将run退回queued、清startedAt，保留原准入截止；不作为失败刷历史。超过30分钟记failed/backup_queue_timeout。只有取得源租约后才分配fileId并在短事务保存pending/owner意图，仍保证先登记意图再写任何目标文件；不得把数据库事务跨过VACUUM/复制/hash/下载。
4. 其它真实生成失败进入failed，应用退出/用户取消进入interrupted；只有资源已收尾、状态已明确或留有可对账running事实后才移除活动句柄。取消queued可立即interrupted；取消running只持久写cancel_requested_at并signal，返回202表示取消请求已接纳，不能提前宣称文件已删除/执行器已退出。
5. 发布前取消：清理确切自有候选，标interrupted。有效最终目录已完成发布后，完成事实优先：可补记completed并保留cancelRequested，不能删除已发布好备份后谎称从未完成。无法证明是否发布完整则保留running/待对账，下一启动核验，不能以ctx取消直接覆盖为failed后重跑。
6. 退出仍按现有顺序封请求准入→取消workCtx→关本实例连接→等handler和a.wg→关DB/数据锁。备份manager显式封准入，不在wg.Wait后Add。收尾需要DB写时可用独立最多5秒的受限清理context（不做新的文件生成/大库校验），失败保留running供下次对账并记录cleanupErr；不能为持久化终态无限拖停服。
7. 根lease、目录句柄、快照租约、下载句柄都有唯一所有者和幂等Close；关闭/Sync/删除失败合并为安全中文资源收尾错误，不被主错误覆盖。无法取消的真实底层I/O仍需目标环境实测，不能承诺杀进程即可完成持久化。

## 7. 新文件、manifest与DB的跨资源协议

### 7.1 精确manifest与发布

建议manifest固定格式1、最多16KiB、严格单对象/精确字段/无重复键：`{formatVersion:1,storeId,runId,fileId,scope:"database-only",createdAt,appVersion,schemaVersion,files:[{role:"database",name:"snapshot.db",size,sha256}],excludedRoles:[上述固定七项]}`。数组精确一个DB成员；名字不接受分隔符/`.`/`..`/大小写别名/重复成员。schema/size/hash取S03.1实际校验及目标副本复核结果，不填编译常量冒充实测。owner.json<=1KiB只含固定magic/formatVersion/storeId/runId/fileId；其原始字节摘要在pending意图创建前算好并入库。

推荐顺序（每一步都有崩溃断言）：

1. 依第6节领取running后先调用S03.1取得DataDir内有效源租约；busy时不创建持久目标，依原截止退回queued。取得租约后才用短事务登记唯一fileId、pending所有权及预先算好的owner摘要；再在已持有根lease下排他Mkdir `.pending-id`，排他写owner.json、Sync/Close。已有同名对象不覆盖、不接管；登记失败仍须按S03.1关闭源租约，不自行处理其路径。
2. **不修改S03.1冻结服务以转移路径。** 持有源租约，通过其只读句柄以固定64KiB缓冲复制到备份根pending/snapshot.db，目标O_EXCL、受限权限、Sync/Close；同时检查长度/取消。
3. 独立只读checker再次以源租约的实际长度/hash/schema为expected验证目标；原源租约不能因为复制返回nil就提前丢掉失败信息。关闭所有目标校验/文件句柄，Windows尤其不能rename仍打开的目录成员。
4. 排他写有界manifest临时名、Sync/Close，在pending内改成manifest.json，确认成员精确且owner/DB/manifest元数据匹配；按平台publisher要求同步pending目录。随后同一根内将pending目录rename为 `b-id`，并完成根父目录的必要同步/持久化；禁止覆盖现有目标，禁止跨卷或失败后降级copy+delete。目录rename只是文件名公布边界，读取者仍只认DB retained且可校验产物，不凭名字或一次rename返回成功就认完成。
5. 确认最终目录身份、成员和必要持久化成功后，短事务把文件置retained、run置completed、去重摘要/最后成功/last_good_file_id一起提交。只有这之后才启动本次授权轮转。源租约最后Close；其清理失败保留已完成的备份事实，同时记录资源清理告警，不能把它改成可盲重试的新任务。

POSIX同目录rename、文件Sync、目录Sync与Windows行为不能混称；Go os.Rename在非Unix不保证原子，同卷也不自动证明断电持久性。**本研究明确推荐能力不足时整个持久发布失败，不采用“completed但durabilityVerified=false”的降级。** 平台publisher必须具备并成功执行“目标不存在、句柄已关、文件/目录及rename父目录的必要持久化、失败可对账”协议；不能把目录Sync不支持或返回错误静默当成功。启动预检即发现能力不足时storage为unsafe、durabilityVerified=false，拒绝新的持久备份/启用，旧兼容GET仍可用。执行中出现明确持久化失败则run为failed/backup_publish_failed，保留旧备份且不授权轮转；若rename后的完成结果无法判定，保留原running/pending意图及确切材料待对账，同样不登记completed、不更新last-good、不删旧。后续对账也必须由同一publisher补足并证明必要持久化成功后，才可补记原run completed；仅进程重启后文件可读不能替代此条件。durabilityVerified只表达当前根的平台发布协议能力与必要操作是否已核验，不宣称经过真实断电认证；Windows/Linux/NAS断电/挂载语义是独立发布门禁，不由本研究或普通文件系统fixture证明。若目标平台暂不能提供所需协议，该平台S03.2持久发布保持不可用，不能用暂停轮转掩盖已完成语义。

### 7.2 重启只对账，不重新执行未知run

单executor先对账，backup API读状态可用而新准入503 reconciling；不阻塞媒体服务就绪。优先查<=64条已登记文件意图，再用有界ReadDir识别未知/超量项（上限128个根成员，超出只报blocked）；不递归遍历全盘或按前缀删除。所有Hash/SQLite检查不在DB事务或manager锁内；有界状态提交前CAS原意图/根身份。

| 持久事实/文件事实 | 推荐结果 |
| --- | --- |
| queued、未开始且没有候选 | 可继续同一run，先检查取消、关闭及原准入截止，绝不新建runId。 |
| running/pending、完全没有成员 | interrupted/process_interrupted；不自动重做VACUUM。 |
| pending目录完整且owner、manifest、DB全部可验证 | 仅在平台publisher能力满足且完成剩余同根rename和必要持久化后补记原run completed；恢复本地产物事实不是重新执行源快照。 |
| 最终目录已公布、DB仍running/pending | 对照原意图及实际schema/完整性/外键/大小/hash重新验证，并补足同一publisher的必要持久化后原ID补completed；能力不足不能只因可读而补完成。 |
| DB completed/retained、成员缺失 | run仍保留“当时完成”事实；文件置missing、不可下载、重新寻找真正last-good，不能继续当保留数量/有效恢复点。 |
| 目录完整但digest/owner/storeId不符或出现未知成员 | unverified并保留，停止该路径删除/发布；根状态提示人工核对。摘要不是可信来源认证。 |
| 只有DB或半份manifest等明确自有不完整候选 | interrupted；只清理符合意图、身份和允许成员的确切临时对象，清理失败保留file ownership占位。 |
| deleting、删除部分完成 | 仅对已登记确切成员重放删除，缺失视为该步完成；仍须核验剩余成员，未知或替换对象不碰。 |
| 符合名字/manifest但无DB意图 | unknown，不自动adopt、补建completed或删除；可能是旧库恢复后遗留的新产物，留给显式离线核验。 |
| DB/根写失败、关闭时无法记终态 | 原running及意图留待下次，安全中文日志说明结果待对账；无自动新run/外部重发。 |

对账可幂等多次启动；若根整体缺失/锁占用/只读，不把所有产物逐个改missing或删除记录，先报告根unavailable，避免把暂时卸载误当永久丢失。备份副本的正常启动/恢复准备归S03.3；本片不隐式撤销会话、迁移快照或对外启动副本。

## 8. 预算、最后一份与下载租约轮转

- 令S为S03.1保守源尺寸，M=1MiB为有界元数据/临时余量，算术先防负数和int64/uint64溢出。源卷至少满足原MinFree+2S；不同目标卷至少MinFree+S+M。同卷必须合并为MinFree+3S+M，不能分别检查后误以为两份都能装下；无法证明不同卷时按同卷保守预算。卷身份/余量取真实源和目标，再复核根身份。预算是准入估计，不是对其它缓存/外部写者的预留锁，仍须处理FULL/IOERR/卸载。
- `maxBytes`统计已登记自有DB+manifest+owner的实际逻辑长度，包含pending/deleting残留；根常量标记/锁不算媒体容量，未知项令usageComplete=false，不能当0。单份S+元数据预计已大于maxBytes时不执行；实际膨胀越界记failed并清理本次明确候选，旧有效备份保留。
- 聚合预算是稳态保留目标，允许一次新候选的暂时双份空间；不能先删旧救空间。已有未消化轮转积压/未知占用时停止新的持久文件生成，防止下载租约或清理错误导致逐日无界增长。可继续原下载/只读检查；释放租约唤醒同一executor继续已授权的轮转，不需要先再成功生成一份。
- 轮转只能发生在新备份已有效发布且登记完成后，或继续这一已持久完成授权的轮转。用**当前最新已提交保留策略**决定删旧；running准入快照只用于解释当时输入，防止执行期间改策略导致旧run重写当前配置。只编辑keep/max不立即删除文件；下一成功点才授权应用新保留规则，禁用不删。
- 按run_seq新到旧选择保留，最高序号且实际核验有效的一份始终为guard，至少保留一份，即使keepCount=1或maxBytes不满足也不删guard。新guard无效/缺失时先在<=64目录中验证其它候选，找不到有效guard则停止删除。受限空间不能把“至少一份”降成0。
- 删除候选需completed、明确owner/storeId、精确成员/长度/摘要及身份复核，通过后在mu内确认租约计数0并CAS retained→deleting，阻止新下载；释放锁后才做大I/O/删除。只删owner许可的确切文件，owner最后删，目录最后用空目录Remove；禁止RemoveAll/按后缀扫删。任一步权限/占用/替换/未知成员失败保留可恢复记录，不能丢ownership解额度。
- 下载在同一mu内完成可用性检查/租约计数增加，再打开确切只读句柄；打开/验证失败回滚计数。整个ServeContent期间持租约，defer先关文件再减计数并wake；最多2个持久下载，包括HEAD/校验阶段，满额429+Retry-After:1。轮转跳过租约>0的文件；Windows外部占用即使内部计数0仍可能删失败，按事实积压。
- GET/HEAD文件下载校验manifest、实际size/hash/schema及文件身份，头发出前失败返回中文JSON，头后失败只记录资源收尾/连接事实，不追加JSON；支持现有64位Range/条件请求、强ETag取真实SHA、固定服务器文件名、no-store，无全局短WriteTimeout。持久下载不持backupMu，允许新快照生成；原兼容GET仍按S03.1持全周期快照锁。
- 轮转成功与备份成功是两件事。返回实际retainedCount/ownedBytes、usageComplete、retentionPending、超额count/bytes及受控原因；不能因文件下载中而将completed改failed，也不能谎报保留目标已满足。真实ACL/拔盘/空间/断电能力分别记录待验。

## 9. API候选：精确payload、响应和错误

全部新API经原鉴权/同源写保护/no-store；写入64KiB上限，要求application/json，精确camelCase键，拒绝大小写别名、重复/未知键、null、尾随对象、浮点/指数型整数字面量。不要误以为现有decodeJSON能拒绝重复或大小写别名；账号decoder只处理字符串，不能直接拿来吞布尔/整数字段。应做局部严格backup解码并复用可安全共享的词法原语，不重写已复核账号流程。

### 9.1 路由与DTO

| 路由 | 请求与精确语义 |
| --- | --- |
| GET /api/backup-policy | 无query。返回`{policy,storage,execution,clock}`，见下；只读，不创建根/触发快照/轮转。 |
| PUT /api/backup-policy | 必填且仅`{revision:"1",enabled:false,hourUTC:3,minuteUTC:0,keepCount:7,maxBytes:10737418240}`。CAS冲突409；成功200返回同GET结构。启用前核验根就绪与单份预算，失败不改策略；关闭始终允许，不因根离线阻止停用。相同配置200且revision不变。 |
| POST /api/backups | 仅`{requestId:"规范小写UUIDv7"}`。新接受202+`{run:RunView,replayed:false,historyRetained:true,expiresAt}`，Location为本地run地址；已知非终态202、窗口内已知终态200，replayed=true；压缩后run为RunSummary且historyRetained=false。新key busy409，不入队/不消耗去重槽；网络结果未知先重放原键。 |
| GET /api/backups?limit=20&cursor=... | cursor省略或`v1.<正十进制seq>`（最多19位且<=MaxInt64，无前导零）；按seq DESC/keyset取limit+1，返回`{items:RunView[],nextCursor:string|null,historyLimit:200,historyTruncated:boolean}`。无offset、无按墙钟排序、不自动补无限页。 |
| GET /api/backups/{id} | id精确32小写hex；存在200 RunView，已裁剪且无独立可用文件404；账本重放仍可返回原RunSummary，404不等于可以再次执行旧key。 |
| POST /api/backups/{id}/cancel | 严格空对象`{}`。queued立即200原ID终态；running202并标cancelRequested；已终态200幂等；未知404。取消无法承诺已完成发布会被撤回。 |
| GET/HEAD /api/backups/{id}/file | 不接收路径或fileId query。仅completed+retained且实际校验通过；已轮转410，missing404，未完成409，unverified409。直接查独立文件归属，不能因历史裁剪去猜路径；不为HEAD创建新快照。 |

`policy={revision:string,enabled:boolean,hourUTC:number,minuteUTC:number,keepCount:number,maxBytes:number,scheduleRevision:string,nextRunAt:string|null,updatedAt:string}`。

`storage={status:"ready"|"reconciling"|"unavailable"|"mismatch"|"unsafe",scope:"database-only",retainedCount:number,ownedBytes:number,usageComplete:boolean,retentionPending:boolean,excessCount:number,excessBytes:number,lastErrorCode:string|null,lastError:string|null,durabilityVerified:boolean}`；durabilityVerified=false时不允许新持久发布降级为completed，见7.1；不返回真实路径/锁文件/未知文件名。

`execution={activeRunId:string|null,lastSuccess:RunSummary|null,lastFailure:RunSummary|null}`；`clock={serverTime:string,highWaterAt:string|null,requestWindowSeconds:86400,maxFutureSkewSeconds:300,rollbackDetected:boolean}`。historyTruncated精确取backup_store.history_truncated_count>0，字段及同事务饱和递增见3.2；不能靠“恰好200条”猜。

`RunSummary={id:string,trigger:"manual"|"scheduled",state:五态,createdAt:string,startedAt:string|null,finishedAt:string|null,errorCode:string|null,message:string,cancelRequested:boolean}`；`RunView=RunSummary+{policyRevision:string,scheduleRevision:string|null,scheduledAt:string|null,skippedSlots:number,file:{id:string,availability:"retained"|"deleting"|"missing"|"unverified"|"deleted",size:number,sha256:string,schemaVersion:number,createdAt:string}|null}`。pending尚无完整发布元数据时file=null；摘要不假造原完整字段；items始终[]，nullable字段明确null。`file.sha256`只是备份字节摘要，绝不是密码或会话hash。

### 9.2 错误与异步结果

新备份域错误推荐`{error:"固定中文说明",code:"稳定机器码"}`，可选字段只能来自固定allowlist（serverTime/window/activeRunId/currentRevision），不能附原始异常。保持旧API原错误形状不变。

| HTTP/终态 | 机器码及作用 |
| --- | --- |
| 400 / 413 / 415 | backup_invalid_input / 请求过大 / 类型不符；不得回显坏参数或秘密。 |
| 401 / 403 | 原鉴权/同源拒绝；敏感提交前会话过期仍401，凭据修订变化409 account_changed，复用S02准则。 |
| 409 | backup_policy_changed、backup_busy、backup_request_time_ahead、backup_not_completed、backup_file_unverified；409重试不能换key伪装新意图。 |
| 410 | backup_request_expired（不能新建）或backup_file_rotated（原备份完成事实保留）；二者语义不能混。 |
| 422 | backup_budget_too_small：启用策略时单份需求已无法装入maxBytes；整数语法/静态范围错误用400。 |
| 429 | backup_download_limit，Retry-After:1；不挤占媒体slots。 |
| 503 | backup_reconciling、backup_storage_unavailable、backup_storage_mismatch、backup_clock_rollback、backup_dedupe_full、backup_ownership_full；保守拒绝、不驱逐活键/自有文件。容量类Retry-After:60只是建议核对时机，不保证届时有空间。 |
| 500 | backup_store_failed等受控存储故障；事务提交结果未知不能自动返回“肯定未接纳”，客户端沿原requestId核对。 |
| 已返回202后的run failed | backup_no_space、backup_budget_exceeded、backup_io_failed、backup_validation_failed、backup_publish_failed、backup_queue_timeout；保存受控中文原因。HTTP不能事后改507，原兼容GET的507映射保持。 |
| run interrupted | backup_user_cancelled、backup_shutdown、backup_process_interrupted；取消不当成通知失败，未明确的发布结果先对账。 |

## 10. 可直接执行的实施切片与验证门禁

建议同一个S03.2任务内按以下顺序，每片先读最新S03.1源码/结果及账号CLI实际交付，不凭本文行号继续写旧版本：

1. **v6存储与兼容门禁**：五表/约束/CAS/限额与真实v5→v6；同片更新离线维护{5,6}和expected版本快照、未来7反例及所有最新头断言。先证明升级不破坏账号恢复，再接调度。
2. **受控根和跨资源发布**：根身份/第二租约/owner+manifest/流式目标复制/目标复验/故障对账，先由内部调用和隔离HTTP验收；不写第二套VACUUM，不扩S03.1租约所有权。
3. **单执行器/手动API**：持久去重账本、严格DTO、短事务领取、取消与关闭、busy退回、已接纳任务不随HTTP断开；稳定保留原runId。
4. **UTC计划/有界保留**：确定性时间注入、错过合并/回拨游标、200/2048/64上限、独立下载租约/轮转与积压；最后才开放enabled=true。策略默认始终关闭，未做UI/通知。

临时Go行为测试（本轮不生成；正式执行后按用户要求清源）：真实SQLite约束/事务回滚、修订溢出；v5/v6离线维护/CAS；时间边界、跨闰日/月年、missed10/大量周期O(1)、重复tick/重启、仅保留参数更新；UUIDv7语法/48位时间、expiresAt=T+86400000而非acceptedAt+窗口、E=expiresAt精确过期、T=E+300000允许/再加1毫秒拒绝、非终态过窗重放不续期、回拨/高水位、有效键2048满不逐出、裁剪run后摘要重放；领取/取消/完成/barrier竞态；Busy时无目标意图/目录、30分钟排队与运行取消；目标Sync/Close/rename/DB提交前后故障、平台目录持久化不支持时无completed/无轮转；未知成员/根替换/清理幂等；下载barrier阻止删除。

稳定集成建议扩展 `scripts/verify_backup.py --binary <fresh产物> --root <全新隔离根>`，复用baseline OwnedProcess/受控环境/随机就绪/精确进程回收，不启动用户实例；新增或分组保持可重放：

- 真v5保留数据→v6→在线功能/快照头/旧会话；离线账号v5/v6均成功，只改三项账号事实；future7/结构伪装/触发器/缺库/活锁拒绝。原始内存输出扫描秘密后才写安全证据。
- 旧GET与新executor争用、重复key/掉响应/重启同ID、账本/历史上限，当前/过窗/压缩响应都实际断言；不能以模拟handler替代真实HTTP。
- 精确自有进程在每个发布断点终止再启动：只有源候选、目标半复制、manifest前后、目录rename后/DB前、completed后、deleting部分完成；每次证明没有重复VACUUM、最后好副本未误删、未知sentinel保留。测试控制注入不能变生产环境后门。
- 真慢Range下载与HEAD占用租约、keepCount=1、体积压力、删除失败、根临时不可用与重新挂回、last-good损坏回退、根与媒体/下载/collection别名隔离。同卷/不同卷预算必须单独断言；fixture/注入与真实FULL/拔盘/ACL分开记账。
- 同时修改收藏/标签/进度，下载快照完整性、外键、数据一致性；大库流式复制/hash的峰值内存不随库体积线性增长。源单连接VACUUM会阻塞其它SQL，性能须测而非承诺无影响。
- 停服取消慢生成/慢下载，等待句柄/worker关闭，DB和两个根锁可立即重新获取；不按进程名清理。运行gofmt/vet、临时行为测试后清源、clean-source包编译、fresh Windows构建、受影响core/account/offline集成；Linux交叉构建不冒称Linux/NAS实际锁/权限/目录同步通过。

本轮只完成设计审阅和文件读回，**未执行上述lint/typecheck/业务测试，未启动真实服务、Docker或浏览器，未提交**。S03.3再设计UI、只读离线verify/inventory与独立恢复准备，S04再接事件/通知；本研究不扩大为在线整栈抓卷或一键恢复。

## 11. 依据与冻结前必核项

- 项目：本任务PRD/冻结S03.1 info、`research/snapshot-service-design.md`；父路线info及reliability研究；backend runtime/cross-platform/account/baseline契约；cross-layer/code-reuse指南；`docs/api.md`、`docs/collection.md`、`docs/components.md`、`docs/windows.md`。当前源码才是“已实现”的依据，旧文档中的v4说明不覆盖实际v5。
- 官方参考：Go os（尤其Rename/Root/Sync）https://pkg.go.dev/os ；SQLite VACUUM INTO https://www.sqlite.org/lang_vacuum.html ；SQLite事务原子性边界 https://www.sqlite.org/atomiccommit.html 。当前网页不替代固定Go1.26/modernc嵌入版本与目标文件系统实测；跨资源发布并非SQLite事务能自动包住。
- 冻结必须明确接受：24小时UUIDv7/压缩摘要与原无限“返回原run”的差异，expiresAt固定由requestTime+24小时推导及精确边界；严重新时钟错误的保守手动退路；非Unix发布/目录持久化能力不足时整个持久发布失败、不得降级completed（7.1），该平台可能暂不可用；root默认同盘与database-only范围；真实设备未验单列。不得省掉schema6与离线账号维护器的同步门禁。本研究不新增时钟修复CLI或其它维护命令。
