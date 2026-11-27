# S03.2.1 schema6 存储与账号兼容接缝核对

2026-11-23。状态：独立研究建议，供主线冻结；不是迁移实现或产品通过声明。本轮只新增本文件，不修改其他研究、PRD、info、主线草案、正式 spec、脚本或业务代码；未执行产品、构建、lint、类型检查或测试，没有生成临时测试。

## 1. 结论与本片边界

主线 `../storage-schema6-contract.md` 的五表、当前头6、离线账号显式支持 `{5,6}`、默认关闭和不引入执行器的方向可以实施。未发现需要另起 API、服务或物理根身份设计才能解决的阻断。冻结时应把下述 SQL NULL、合法故障状态和版本快照要求落实为具体 DDL 与门禁，不以旧研究的概略字段表替代。

- 本片只增加存储、原子迁移、当前快照结构探针、离线账号版本兼容及隔离验证。不实现根/marker/配置、持久目标、调度、轮转、管理器、API 或无调用者的 CRUD。
- `retained/deleting` 必须完整；`pending/missing/unverified/deleted` 可能来自未发布候选，不能以“非 pending 均完整”的 CHECK 拒绝合法对账或清理。
- 本片的 `deleting` 仅表示对**已发布文件**的轮转删除认领。未发布候选清理继续持有原 ownership，确认确切自有成员和目录全部消失后直接置 `deleted`；失败保持原待核对状态，不借 `deleting` 绕过成功授权。
- 200 条运行、64 条非 deleted 所有权、2048 条有效键由后片短事务准入/裁剪承担。本片不添加计数触发器，也不宣称容量管理已交付；单 active、单 deleting 的声明式部分唯一索引可以本片建立。
- 当前 shared snapshot checker 只支持头6；历史5离线账号维护仍支持。两种入口用途不同，不能把前者拒绝5解释为旧快照已损坏，也不能声称本片已有通用历史离线 verify/恢复入口。

## 2. 检索与实际代码证据

已先使用仓库现有 CodeGraph CLI 查询 `openDatabase`、`migrate`、`inspectOfflineAccount`、`resetOfflineAccountPassword`、`backup` 及夹具符号，再定向核对实际文件。索引存在待更新文件，空结果以源码补核；没有更新共享索引。下列行号是本轮读取时的源码位置，不是未来实现的固定锚点。

| 实际位置 | 已核实事实 | 本片必须处理的接缝 |
| --- | --- | --- |
| `internal/app/store.go:16–17,20–168` | 当前头5；v0 后局部 version 只设到1；后续迁移不逐版回写该局部变量；原启动还会恢复 jobs/外部动作 | v6 分支用 `version < 6`，不能 `version == 5`。放在 v5 后、原恢复步骤前；历史迁移的 literal 版本保持不变。 |
| `internal/app/account_offline_store.go:20–24,90–123,256–294` | 预检和提交各自硬编码5；snapshot 的 `schemaVersion` 存的是 `PRAGMA schema_version` cookie，不是应用迁移头 | 新增独立 `userVersion int64`，预检保存；提交支持集判定后仍必须等于预检值，原 cookie 和凭据 CAS 全保留。 |
| `internal/app/account_offline_store.go:125–198` | 精确检查 users/sessions 列、两条历史会话 trigger 及反向外键 | 五备份表不引用账号表；不得放宽原账号校验或调用正常启动迁移。 |
| `internal/app/backup_verify.go:119–195` | 只读固定连接核当前头；原 LIMIT 0 和核心表数量只覆盖11张旧表 | 同步头6并加入五表必要列/类型、STRICT、关键单例的只读探针，拒绝仅改头的假6；保留原长度/完整性/外键/流式摘要路径。 |
| `scripts/verify_schema.py:11–60` | 只从冻结 v1–v4 常量建真正旧库，最后断言旧三列账号结构 | 需要专门真实 v5 分支；不能只放宽版本上限或把 v6 降头为5。 |
| `internal/app/account_store.go:280–314,359–382` | v5 在 schemaV5 后为旧会话生成随机 session_id，再应用 schemaV5SessionConstraints | 真 v5 夹具必须保留这一结构与数据顺序，包括唯一索引和两条拒空 trigger。 |
| `scripts/verify_account_offline.py:576,601–606,661–683` | future 当前为6；保留真 v4 拒绝与账号以外零变化断言 | future 改7，另加真5/真6；原 v4、畸形/反向FK、秘密输出、取消/未知结果门禁不减少。 |
| `scripts/verify.py:279`、`verify_backup.py:170,180`、`verify_account.py:184,316` | 存在最新头5断言/标签 | 区分“当前头”和“历史夹具”逐项改，不能全局替换数字5。 |

设计依据为 `persistent-backup-design.md` 第3节；授权字段和删除边界沿用 `persistent-retention-authorization.md`；高水位和根身份语义沿用 `persistent-clock-root-seams.md`。本文件不重新开放这些取舍。

## 3. 最小精确 DDL 展开规则

以下是供 `schemaV6` 机械展开的定义清单，不是可直接执行的带宏 SQL。实现应把每个谓词在对应字段或表 CHECK 中展开，不创建 SQLite 自定义函数，不引入新依赖或触发器。五表一律 `STRICT`；所有列必须带中文字段说明。字段表中的非空性不能只靠值域 CHECK 隐含表达。

### 3.1 共用谓词

- `M=253402300799999`，`W=86400000`，`Bmin=67108864`，`Bmax=1099511627776`。INTEGER 在 Go 使用 int64；正整数为 `CHECK(x>=1)`，非负计数为 `CHECK(x>=0)`；毫秒为 `CHECK(x BETWEEN 0 AND 253402300799999)`。可空值域显式写 `x IS NULL OR <值域>`。
- `HEX(x,n)` 精确展开为 `length(x)=n AND length(CAST(x AS BLOB))=n AND x NOT GLOB '*[^0-9a-f]*'`。长度与字节双检拒绝 NUL/多字节/后缀；不要使用 `GLOB '[0-9a-f]*'`，该写法只限制首字符，星号并不表示重复前一字符集。必填字段另有 `NOT NULL`，可空字段写 `x IS NULL OR (HEX...)`。
- `ASCII(x,lo,hi,set)` 为 `length(CAST(x AS BLOB)) BETWEEN lo AND hi AND length(x)=length(CAST(x AS BLOB)) AND x NOT GLOB '*[^<set>]*'`。`error_code` 可空，非空时采用1..64字节及字符集 `a-z0-9_`；`schedule_key` 可空，非空时采用1..96字节及字符集 `0-9s:`。这仅是安全存储格式，不能声称已验证 `s1:<revision>:<slot>` 的业务对应关系。
- `UUID7(x)` 为36字符且36字节，`substr(x,9,1)='-'`、第14/19/24位均为 `'-'`、第15位为 `'7'`、第20位 `IN('8','9','a','b')`，并对 `replace(x,'-','')` 应用 `HEX(...,32)`。这些共同排除其他位置的连字符、大小写、错误 variant/版本和 NUL。可空 UUID 另包 `IS NULL OR (...)`。
- `SAFEJSON(x,types)`：`length(CAST(x AS BLOB))<=2048 AND CASE WHEN json_valid(x) THEN json_type(x) IN(<types>) ELSE 0 END`。列另有 `TEXT NOT NULL`。store 两摘要 types 为 `'object','null'`，key 摘要只有 `'object'`。CASE 避免对畸形 JSON 直接调用 json_type；固定 DTO 字段、秘密清理和原始异常禁止写入仍是应用职责，JSON 有效不代表内容安全。
- 不添加 `finished>=started`、`updated>=created`、`accepted>=request_time` 等墙钟先后 CHECK；小回拨和 UUID 未来容忍会形成合法反例。不能拿 CHECK 的 NULL 结果当“必须有值”：SQLite CHECK 结果为 NULL 会通过。

### 3.2 backup_policy

| 字段 | 精确类型/非空 | 值域或迁移初值 |
| --- | --- | --- |
| id | INTEGER PRIMARY KEY | CHECK(id=1)，迁移插1 |
| revision | INTEGER NOT NULL | >=1，初始1 |
| enabled | INTEGER NOT NULL | IN(0,1)，初始0 |
| hour_utc / minute_utc | INTEGER NOT NULL | 0..23 / 0..59，初始3/0 |
| keep_count / max_bytes | INTEGER NOT NULL | 1..30 / Bmin..Bmax，初始7/10737418240 |
| schedule_revision | INTEGER NOT NULL | >=1，初始1 |
| effective_from_ms / updated_at_ms | INTEGER NOT NULL | 毫秒范围，迁移均为同一个N |
| schedule_cursor_ms / next_run_ms | INTEGER NULL | NULL或毫秒范围，初始均NULL |

组合 CHECK 精确为 `(enabled=0 AND next_run_ms IS NULL) OR (enabled=1 AND next_run_ms IS NOT NULL)`。这不证明 next 尚未到期、slot 算法正确或 cursor 单调；这些留后片。`CHECK(id=1)` 只保证最多一行，迁移插入与只读探针共同保证恰一行，不能声称 CHECK 本身禁止删除单例。

### 3.3 backup_store

| 字段 | 精确类型/非空 | 值域或初值 |
| --- | --- | --- |
| id | INTEGER PRIMARY KEY | CHECK(id=1)，迁移插1 |
| store_id | TEXT NOT NULL UNIQUE | HEX32，随机生成且只初始化一次 |
| root_bound | INTEGER NOT NULL DEFAULT 0 | IN(0,1) |
| request_clock_high_water_ms | INTEGER NOT NULL DEFAULT 0 | 毫秒范围，初始0 |
| history_truncated_count | INTEGER NOT NULL DEFAULT 0 | >=0，初始0 |
| last_good_file_id | TEXT NULL | NULL或HEX32，初始NULL |
| last_success_json / last_failure_json | TEXT NOT NULL DEFAULT 'null' | SAFEJSON object/null，初始均JSON null |
| retention_authorizer_file_id | TEXT NULL | NULL或HEX32，初始NULL |
| retention_authorized_keep_count | INTEGER NULL | NULL或1..30，初始NULL |
| retention_authorized_max_bytes | INTEGER NULL | NULL或Bmin..Bmax，初始NULL |

授权组合 CHECK 用纯 IS 谓词：`(a IS NULL AND k IS NULL AND b IS NULL) OR (a IS NOT NULL AND k IS NOT NULL AND b IS NOT NULL)`；再独立检查各自值域。不能写仅 `k>=1 AND b>=Bmin` 的组合而放过半空三元组。授权与 last-good 分开，NULL 表示授权结束；不得凭 last-good、excess、重启或 wake 重新签发。

本片不加上次物理根ID/root_nonce；marker 跨重启只证明逻辑命名空间，不能证明物理盘没变。H 的 SQL 值域不自动保证单调；后片只能由实际时钟推进，业务拒绝可提交 H、认证/取消需确认回滚的区别保持既有研究定义。

### 3.4 backup_runs

| 字段 | 精确类型/非空 | 约束 |
| --- | --- | --- |
| seq | INTEGER PRIMARY KEY AUTOINCREMENT | **CHECK(seq>=1)**，自增本身不拒绝显式0/负数 |
| id | TEXT NOT NULL UNIQUE | HEX32 |
| request_id | TEXT NULL UNIQUE | NULL或UUID7 |
| trigger | TEXT NOT NULL | IN('manual','scheduled') |
| schedule_key | TEXT NULL UNIQUE | NULL或上述安全ASCII |
| slot_ms | INTEGER NULL | NULL或毫秒范围 |
| schedule_revision | INTEGER NULL | NULL或>=1 |
| policy_revision | INTEGER NOT NULL | >=1 |
| keep_count / max_bytes | INTEGER NOT NULL | 1..30 / Bmin..Bmax |
| state | TEXT NOT NULL | IN('queued','running','completed','failed','interrupted') |
| cancel_requested_at_ms | INTEGER NULL | NULL或毫秒范围 |
| created_at_ms / updated_at_ms / admission_deadline_ms | INTEGER NOT NULL | 毫秒范围 |
| started_at_ms / finished_at_ms | INTEGER NULL | NULL或毫秒范围 |
| file_id | TEXT NULL | NULL或HEX32 |
| error_code | TEXT NULL | NULL或上述1..64字节安全代码 |
| message | TEXT NOT NULL | UTF-8字节<=256，安全中文DTO；允许空说明 |
| skipped_slots | INTEGER NOT NULL DEFAULT 0 | >=0 |

表级 CHECK 分三组，显式非空以 IS 谓词表达：

1. manual 要求 request_id 非NULL，schedule_key/slot_ms/schedule_revision 全NULL；scheduled 要求 request_id 为NULL，计划三项全非NULL。精确 schedule_key 与修订/slot 的对应、UUID 内时刻一致性由后片严格解析，不在此伪称已保证。
2. queued：started/finished 全NULL；running：started 非NULL且 finished NULL；completed：started/finished 均非NULL；failed/interrupted：finished 非NULL，started 可NULL。completed 补充 started 非空与“失败/中断可未开始”的原意一致，不新增时间先后约束。
3. queued 的 file_id 必须 NULL，因为 busy 退回 queued 只允许发生在未分配持久意图时。running 可无或已有候选；completed 必有 file_id；failed/interrupted 可仍有 file_id，清理失败不能丢 ownership。queued/running/completed 的 error_code 必NULL；failed/interrupted 的 error_code 必非NULL且通过安全格式，具体受控枚举由后片生产者映射。

静态索引：`CREATE UNIQUE INDEX backup_runs_one_active ON backup_runs((1)) WHERE state IN('queued','running');`。不额外制造重试同run/重排队入口；修订/seq 到 MaxInt64 必须拒绝，不能归零或复用旧身份。

### 3.5 backup_files

| 字段 | 精确类型/非空 | 约束 |
| --- | --- | --- |
| id | TEXT NOT NULL PRIMARY KEY | HEX32 |
| store_id | TEXT NOT NULL | HEX32 |
| run_id | TEXT NOT NULL UNIQUE | HEX32 |
| run_seq | INTEGER NOT NULL UNIQUE | >=1 |
| state | TEXT NOT NULL | IN('pending','retained','deleting','missing','unverified','deleted') |
| created_at_ms | INTEGER NOT NULL | 毫秒范围 |
| published_at_ms / verified_at_ms / deleted_at_ms | INTEGER NULL | NULL或毫秒范围 |
| size_bytes | INTEGER NULL | NULL或>0 |
| sha256 | TEXT NULL | NULL或HEX64 |
| schema_version | INTEGER NULL | NULL或>0，不能固定等于6假装实际测量 |
| app_version | TEXT NULL | NULL或1..64 UTF-8字节，沿用主线草案的非空版本文本 |
| manifest_size | INTEGER NULL | NULL或1..16384 |
| manifest_sha256 | TEXT NULL | NULL或HEX64 |
| owner_size | INTEGER NOT NULL | 1..1024 |
| owner_sha256 | TEXT NOT NULL | HEX64 |
| error_code | TEXT NULL | NULL或上述安全代码 |

定义完整发布元数据 P 为 published_at/verified_at/size/sha/schema/app_version/manifest_size/manifest_sha **八项全非NULL**。三个独立组合 CHECK：

1. `state NOT IN('retained','deleting') OR P`，其中 P 必须逐项 `IS NOT NULL`，不能只拼值域比较。
2. `published_at_ms IS NULL OR P`。这允许未发布记录元数据尚不齐，也保证已明确写下发布时刻的 missing/unverified/deleted 不变成半份“发布事实”；原值的跨 UPDATE 保留仍由应用 CAS 负责，CHECK 不能证明没有被整体清空。
3. `(state='deleted' AND deleted_at_ms IS NOT NULL) OR (state<>'deleted' AND deleted_at_ms IS NULL)`。**deleted 不单独要求 P**：完全未发布且确切清理完成后可以只有 owner 意图和删除时刻。删除不能把 size/hash 改成0/空串来反映残留，保留原身份元数据；未确认或未知残留不许冒称 deleted。

pending 可以在目标验证后先填入部分或全部元数据，不能要求 pending 全部发布字段必NULL；missing/unverified 也可以来自未发布意图。retained/deleting 必完整不是“所有非 pending 必完整”。静态索引：`CREATE UNIQUE INDEX backup_files_one_deleting ON backup_files((1)) WHERE state='deleting';`。

本片无需为未发布 cleanup 扩大 deleting 含义。若未来另选“deleting 同时承担未发布 cleanup”，必须增加可区分的授权/来源语义并重新定义状态约束与轮转认领，不能只放松本条 P 后让半成品获得删除授权；不建议在本片引入该替代方案。

### 3.6 backup_request_keys

| 字段 | 精确类型/非空 | 约束 |
| --- | --- | --- |
| request_id | TEXT NOT NULL PRIMARY KEY | UUID7 |
| request_time_ms | INTEGER NOT NULL | 0..M-W |
| accepted_at_ms / expires_at_ms | INTEGER NOT NULL | 0..M |
| run_id | TEXT NOT NULL UNIQUE | HEX32 |
| run_seq | INTEGER NOT NULL | >=1 |
| terminal | INTEGER NOT NULL DEFAULT 0 | IN(0,1) |
| summary_json | TEXT NOT NULL | SAFEJSON，仅object |

额外 CHECK 为 `expires_at_ms=request_time_ms+86400000`。先以 request_time 上界排除溢出，不用 accepted_at 续期，不要求 accepted_at>=request_time。UUID 前48位解码与 request_time 相等由后片解析；SQLite 的十六进制文本 CAST 不是正确的 UUID 时间解码器。

## 4. 跨表一致性和防裁剪责任

五表之间不新增 FK/级联，尤其不指向 users/sessions。run→file 与 file→run 存在生命周期循环；key 需在 run 压缩后保留安全摘要；policy 的当前 revision 不是独立历史实体，不能用一个错误 FK 代替真实业务关系。`foreign_key_check` 为零不证明下列关系已正确维护。

| 后片原子操作 | 同事务必须维护的事实 |
| --- | --- |
| 接受手动意图 | run.id/seq、key.run_id/run_seq、request_id 同一事实；key 初始摘要匹配run；先完成有界容量判断再原子回收允许的终态键并插入。正常拒绝不得裁剪。 |
| 分配持久候选 | run.file_id 与 file.id/run_id/run_seq、当前 store_id 对应；owner 原始字节摘要先固定；登记成功之后才能写目标。 |
| 发布完成 | 原run CAS completed、文件 retained、key终态摘要、store最后成功/lastgood及授权三元组同事务提交；已completed重放不重新签发授权。 |
| 对账发现缺失/损坏 | 保留当时completed事实和已有发布元数据，更新文件可用性；根整体离线不批量标 missing。授权来源创建时须为同store的completed/retained，后续损坏不意味着删掉其历史身份。 |
| 轮转认领/结束 | 每次新认领先验证授权、当前策略与实际guard；Keff/Beff各取授权和当前值的max；保留单deleting并受控收尾；清授权采用原token CAS，禁止从lastgood重建。 |
| 运行历史裁剪 | 保护active、所有非deleted文件对应run、活跃授权来源及未解除的指针引用；实际删run与 history_truncated_count 饱和递增同事务。总数200包含保护行，不保证是最近200条普通终态。 |
| 删除文件目录记录 | deleted仅在实际目录消失后成立；只有其run已裁剪且没有活跃引用才可删ownership行，不为腾64槽位逐出未知材料。 |

last-good 是候选指针，运行时必须核验；单份guard超预算且已无安全可删文件时，按已接受研究结束授权并如实报告excess，不能永久挂住 token 阻止未来更小候选。本片只提供承载字段，不实现这些操作、H 单调触发器、裁剪器或下载租约。

## 5. 迁移随机ID、时间和原子性

沿用主线草案的 `migrateBackupSchema(ctx context.Context, tx *sql.Tx) error` 边界可行：只在真正需要 v6 时，在调用者已有短迁移事务中一次读取 crypto/rand 的16字节并编码 storeId，一次采受检实际UTC毫秒 N；任何失败返回错误，由调用者确认回滚。16字节随机不是账户身份、路径或机器ID，不用 SQL randomblob 或现有 session_id 复用。此选择无需为缩短采样再引入额外服务。

同笔事务建立五表和静态索引，精确 INSERT 两个单例，最后设置头6再 Commit；不使用 `IF NOT EXISTS`、`INSERT OR IGNORE` 或 upsert 掩盖未知冲突结构。事务失败不得留下部分新表/单例或把头写6；对旧于5的库，已有历史迁移各自已提交，v6失败应回到该次v6开始前的真实v5，不能声称所有历史迁移都一起回滚。

policy 初始 enabled=0、03:00UTC、keep7、max10GiB、revision/schedule_revision=1、effective/updated同N、cursor/next=NULL；store 初始root_bound=0、H=0、truncated=0、两JSON null、lastgood/授权三元组NULL。N 仅用于这次初始化，不要求它大于旧业务文本时间；非法/溢出N失败，不夹断到0或伪造墙钟。重启头6不再随机storeId或重置策略。

`openDatabase` 的新条件必须 `<6`，这样fresh0和真实1–5连续迁移均会进入；历史V1–V5常量和 PRAGMA literal 不改成current常量。迁移本身不触碰根、marker、媒体文件或触发备份。正常启动原有cache/jobs恢复仍存在，不能把它当离线维护/只读checker的快捷入口。

## 6. 真实 v5 夹具与版本矩阵

### 6.1 推荐夹具接缝

在现有 `create_legacy_database` 内增加明确的 v5 分支或专用真实v5 helper，保留旧1–4路径和最后的旧三列账号断言。v5先应用冻结v1–v4，再执行真实 schemaV5，迁移旧会话的 session_id/账号修订，最后执行 schemaV5SessionConstraints。检查头5、v5账号列/唯一索引/恰两条会话trigger、无五备份表及完整性/外键。

有最新隔离来源库时，按真实v5列复制并保留已有合法 users.auth_revision、sessions.session_id/created_at/client_browser/client_os/auth_revision，避免重新生成已有合法sessionId；无来源或仅旧三列来源时才按历史迁移规则生成独立随机会话ID。复制顺序和约束应用顺序要允许旧数据迁移而不绕过最终约束。无source的空账号v5夹具可以成立，但不能拿它替代含真实登录会话的数据保留验证。

禁止先启动新应用“得到v5”（会直接升6），禁止把v6的 user_version 调5，禁止复制完整v6结构/backup表后降头。真实v6应由新应用正常迁移上述真5得到；既有真v4→最新兼容路径继续保留。

### 6.2 显式支持范围

| 入口 | 本片推荐支持 | 拒绝和副作用 |
| --- | --- | --- |
| 正常 openDatabase | 真实0/1–5迁移到6；已有6直接使用 | >6拒绝；不是恢复准备或离线只读入口。 |
| shared snapshot checker / 原GET导出 | 当前真实6 | 真实5、未来7及伪6明确拒绝且只读；旧5指引使用匹配版本工具，不自动迁移或称损坏。 |
| 离线账号预检与提交 | 显式集合{5,6}，相同支持判定函数 | <5、>6拒绝；支持的两个版本之间也不允许预检后漂移。 |
| 历史快照verify/独立恢复准备 | 本片不新增公开入口或支持声明 | S03.3另行冻结，不能靠扩大账号支持集推导。 |

本片不扩 `snapshotExpected.Schema`。后片将源租约复制到目标后，除复用大小/hash expected 外，必须显式比较目标实际schema与源实际schema/manifest；当前checker只限6不等于通用expected schema能力已经实现。

### 6.3 离线账号 CAS 必须保留的两个版本

`offlineAccountSnapshot.userVersion` 与现有 `schemaVersion` 分别保存应用头与 SQLite schema cookie；两者不能合并。预检在同一只读事务保存，提交同一短事务要求 `supported(actualUserVersion) && actualUserVersion==expected.userVersion && actualSchemaCookie==expected.schemaVersion`，再保留原唯一管理员、hash/authRevision、MaxInt64、RowsAffected及提交前ctx检查。仅把两处`!=5`改为允许5/6会漏掉“预检5、提交6而cookie未变”的反例。

账号 v6 兼容的依据是账号结构未变且无新增引用；不要求离线CLI读取策略/完整backup域来做根或任务恢复。成功仍只改变 password_hash、auth_revision 和 sessions；所有非账号行、backup单例/授权/ownership、磁盘pending sentinel均原样。Commit/rollback未知不得宣传零修改。

## 7. S03.2.1 必须门禁（本轮均未执行）

1. **迁移链和原子性**：fresh0、真1–4、含账号/会话/收藏/进度/作业的真5分别迁6；原字段/ID/hash/authRevision/sessionId/cookie和原记录保留。重复启动storeId/默认策略稳定。随机失败、非法时钟、对象冲突或中途DDL失败时，五表/索引/单例/头整体回滚到v6事务前，未知旧对象保留；future7拒绝。
2. **真实SQLite约束**：五表STRICT、正seq、数值边界/可空分支、UTF-8字节而非字符、NUL/大小写/非hex、UUID版本/variant/连字符、半空授权、JSON坏值/错误shape、expires=T+W及溢出、单active、单deleting均有有效和无效反例。单例缺失和伪6被只读探针拒绝，不能只做创建SQL字符串断言。
3. **合法故障状态可承载**：未分配file的queued/running；running带pending；未启动failed/interrupted；failed/interrupted保留file；未发布pending→unverified/missing；未发布确切清理→deleted无P；已发布retained→missing/unverified/deleting→deleted保留P。无P的retained/deleting、queued带file、completed缺started/finished/file、半份published tuple必须拒绝。无需执行真实轮转来验证DDL。
4. **checker只读和兼容**：当前真6可生成/下载并通过独立SQLite核验；真5、未来7、只降/升头和缺五表/错类型/非STRICT/缺单例伪6拒绝。前后逻辑内容、hash/size/mtime和目录成员按稳定副本前提验证，无隐式迁移或sidecar；保留坏页/尾随/外键/expected大小摘要/取消和流式内存门禁。
5. **离线账号5/6**：真正5和实际迁移6均执行真实CLI成功/拒绝/CAS/秘密检查；v4仍拒绝、future反例改7。预检后分别5→6、6→5且schema cookie不变均拒绝；cookie或凭据漂移同样拒绝。真6可种合法非空备份域及pending sentinel，但不启executor/策略，证明只改三类账号事实、释放锁且不触碰备份域。
6. **稳定脚本及同产物证据**：扩真实v5 helper和offline场景；同步 verify/core、account、backup等“最新头”断言但保留历史常量。新storage专项复用OwnedProcess/隔离新根，证明默认关闭且不建备份根、重复启动稳定与旧cookie仍可用；不把无测试编译算行为通过。
7. **工程与清源**：按项目wrapper运行gofmt/vet、临时行为验证，结束后精确删除本片新增临时测试源；再做clean-source包编译、fresh Windows构建、Python AST/类型标注、Vue typecheck和受影响backup/core/account/offline/storage专项。读取同一产物SHA和实际断言数，不复用上片通过数字；原始秘密扫描先于脱敏。
8. **独立记账**：实施者停写后独立checker可写复核，主线再同步正式spec/docs/任务。正常停服与Job强制兜底分列；Linux两架构交叉构建只记编译，真实Linux/NAS、ACL/FULL/拔盘/断电/大库性能尚未验收。S03.2.1不能声称后片根/发布/调度/容量/轮转已完成。

## 8. 主线草案收敛意见

已读取主线草案第1–75行。其“retained/deleting完整、pending/unverified可不完整、deleted只需删除时刻”的方向正确，与账号validator无冲突；按本文件把未发布cleanup直达deleted及published tuple条件明确即可，无需扩展deleting角色。主线已接受的seq>=1、schedule_key `[0-9s:]` 和error_code `[a-z0-9_]`安全格式在本推荐中保持一致。

建议具体DDL落实时补充 queued.file_id=NULL、completed.started_at非NULL以及 `published_at IS NOT NULL => P`；三者都是既有状态契约的行内约束，不要求本片提前实现执行器。除上述精确化外不建议继续扩张本片。研究字段、格式与支持集只有经主线冻结并实际实施/验证后才成为交付事实；本文写入与读回不计产品通过数。
