# S03.2.3 执行器与手动意图最小契约复核

2026-11-23。状态：Phase 1.2 独立研究建议，**未冻结、未激活、未实施**。S03.2.1 已独立验收，S03.2.2 正按 `root-publisher-contract.md` 实施；本文只研究下一片执行器、手动幂等、取消、无 source 对账及必要的授权结束检查。唯一新增文件为本文；不修改产品、测试、任务状态、正式 spec、既有研究或共享 CodeGraph 索引，不运行产品门禁，不访问用户实例。

## 1. 实际前提与建议结论

- 实际代码先经 CodeGraph CLI 定位再读正文：`internal/app/backup.go` 的 `createDatabaseSnapshot`/租约 Close；`account_store.go` 的 `validateAccountMutation`/`validateAccountCommit`；并读 `backup_store.go` 五表、`store.go` 单连接、`server.go` 启停、`account.go` 严格输入与中间件、`jobs.go` 领取路径。CodeGraph 仅作导航，不把索引旧行号当当前实现证明；不索引并发新代码、不依赖 S03.2.2 尚未交付的内部函数名。
- 已有 schema6 五表足够，不加迁移、FK、计数触发器或第六张表。`200 run / 64 非 deleted ownership / 2048 request key` 目前是后片服务责任；单 active、单 deleting、授权三字段全空/全有已由 DDL 约束。当前快照 checker 只接受头6，离线账号入口显式支持 `{5,6}`；本片不扩大任一版本矩阵。
- 共用 source 自行取得 `backupMu`，锁一直持有到 source.Close；旧 `GET /api/backup` 的生成、校验和下载全周期也占此锁。新执行器调用同一服务，不预先再锁同一互斥、不重新实现 VACUUM、不接管 source 路径。
- 主 DB 实际 `SetMaxOpenConns(1)`；VACUUM 期间鉴权、查询及持久取消都可能等待。不得声称后台备份与业务 SQL 真正并行，或承诺取消接口即时返回；不为规避此边界新开写连接或绕过真实会话验证。
- 冻结权威顺序：已验收 storage spec/`storage-schema6-contract.md` → 当前 `root-publisher-contract.md` → 本文拟议下一片行为。参考研究为 `persistent-backup-design.md`、`persistent-clock-root-seams.md`、`persistent-retention-authorization.md`、`persistent-freeze-review.md`；旧草案“mu 包事务”“对账直接调用已有 publisher”“只凭 last-good 轮转”等表述按本文收窄。

**推荐只交付一个 manager、一个 executor、五个手动/只读路由、原 key 有限窗口重放、无 source 的原意图对账，以及不删除文件的授权结束检查。** 每次真正成功仍原子发放授权；真实阈值已满足才 CAS 清空。超额、未知材料或需要删除时保留成功与授权，暂停新意图，删除留待 S03.2.4。策略仍关闭；不加入策略 PUT/启用、UTC 调度、持久下载/HEAD、下载租约、轮转删除、公开恢复 CLI、历史 checker 或长期测试后门。

## 2. 建议直接冻结的选择

| 接缝 | 最小可实施结论 |
| --- | --- |
| 根唯一所有者 | manager 在唯一 executor 内持有一份根 lease，HTTP 不自行开根/持根；默认 disabled 且 root_bound=0 启动零持久根 I/O。root_bound=1 启动异步核根/对账，不阻塞媒体就绪。 |
| 第一次手动操作 | 合法、重新鉴权的新 POST 遇未准备根，提交高水位后返回503/reconciling、Retry-After:1，不建 run/key；提交确认后合并唤醒 executor 做明确准备。客户端沿原 key 重试；GET 不初始化根。 |
| 提交未知 | 不发送尚未确认的202/410/409等候选结果，不换 key/run/fileId，不再次生成 source；500/result_unknown，保留原事实并沿原 key/id 读回。 |
| 无 source 对账 | 新增 S03.2.3 包内协调入口，复用冻结严格格式/平台原语；不要求 S03.2.2 publisher 接续既有 pending/final，也不调用它覆盖碰撞。 |
| 成功与授权 | 原 running/pending → completed/retained、key终态摘要、last-success/last-good及授权三字段同事务；重复 completed 只读，不重发或覆盖授权。 |
| 授权结束 | 只检查真实数量/字节及有效 guard 已满足 `max(授权阈值,当前阈值)`，按原三字段和当前事实 CAS 清 NULL；本片不选择牺牲文件、不认领 deleting、不 unlink 已完成备份。 |
| 生命周期 | 事务先获得单连接，再短 mu 处理内存事实；不在 mu 中等连接/SQL/Commit/文件 I/O。停服先 seal/signal，请求排空后唯一 executor 做一次有界 DB 收尾，最后释放根。 |

## 3. API、DTO 与安全边界

下列是本片拟议完整入口；`GET /api/backup` 保持既有同步语义、响应及 busy 映射，不迁移为持久下载。新入口沿用原鉴权、Origin/Sec-Fetch 写保护、no-store、64KiB 请求体上限；只用既有一次 unread-body 收尾，不叠加全局短 WriteTimeout。修改账户/session 的竞态仍按实际 actor、账号修订和会话过期验证。

| 路由 | 严格输入与确认响应 |
| --- | --- |
| GET /api/backup-policy | 无 query；200 `{policy,storage,execution,clock}`，只读缓存/DB事实，不触发根、hash、快照或清理。 |
| POST /api/backups | 仅 JSON `{requestId:string}`，规范小写 UUIDv7；新接纳202 `{run,replayed:false,historyRetained:true,expiresAt}` 及本地 Location。已知 active 202；窗口内 terminal 200；均 replayed=true，同一 runId，expiresAt 不续期。run 已压缩时返回 RunSummary、historyRetained=false。 |
| GET /api/backups | 可选单值 limit，默认20、1..100；可选 `cursor=v1.<正十进制seq>`，无前导零、最多19位且不超 MaxInt64。拒绝空值、重复和未知 query；seq DESC/keyset 取 limit+1。200 `{items:RunView[],nextCursor:string\|null,historyLimit:200,historyTruncated:boolean}`。 |
| GET /api/backups/{id} | id精确32位小写hex，无 query；200 RunView；已裁剪404。404不赋予重新使用旧 key 的权限，压缩摘要仍可经 POST 原 key 重放。 |
| POST /api/backups/{id}/cancel | id同上，无 query，仅严格空对象 `{}`；queued 成功终结200，running 持久记录取消请求后202，已终态200幂等，未知404。202不表示文件已删除或执行器已退出。 |

JSON 解码逐 token 拒绝大小写别名、重复/未知键、null、尾随对象和非 application/json；空对象也精确校验。账号 string-only decoder 可参考词法规则，不直接把通用 decodeJSON 当严格 decoder。未来策略 PUT 的布尔/整数词法不借本片提前开放。

| DTO / 字段类型 | 中文语义与边界 |
| --- | --- |
| policy | `revision:string,enabled:boolean,hourUTC:int,minuteUTC:int,keepCount:int,maxBytes:int`；沿当前已提交策略，enabled 保持 false；revision 为规范十进制字符串，预算范围仍64MiB..1TiB。本片不暴露可写参数。 |
| storage | `status:ready\|reconciling\|unavailable\|mismatch\|unsafe,scope:"database-only",publishProtocolReady:boolean,retainedCount:int\|null,ownedBytes:string\|null,usageComplete:boolean,retentionPending:boolean,excessCount:int\|null,excessBytes:string\|null,lastErrorCode:string\|null,lastError:string\|null`。聚合字节用十进制字符串；计量未完成/溢出时为 null、usageComplete=false，不能假0或截断 JS 数值。 |
| execution / clock | `activeRunId:string\|null,lastSuccess:RunSummary\|null,lastFailure:RunSummary\|null`；`serverTime:string,highWaterAt:string\|null,requestWindowSeconds:86400,maxFutureSkewSeconds:300,rollbackDetected:boolean`。GET 不推进 H；高水位0显示 null。 |
| RunSummary | `id:string,trigger:manual\|scheduled,state:queued\|running\|completed\|failed\|interrupted,createdAt:string,startedAt:string\|null,finishedAt:string\|null,errorCode:string\|null,message:string,cancelRequested:boolean`；安全 JSON UTF-8 编码后≤2048字节，message≤256字节。 |
| RunView | RunSummary 加 `policyRevision:string,scheduleRevision:string\|null,scheduledAt:string\|null,skippedSlots:int,file:FileView\|null`；seq只用于服务端排序/字符串游标，不作为 JS number 输出。 |
| FileView | `id:string,availability:retained\|deleting\|missing\|unverified\|deleted,size:int,sha256:string,schemaVersion:int,createdAt:string`；只在完整发布 tuple 可用时返回。pending、未发布 unverified/deleted 一律 file=null，不伪造部分字段；本片没有下载链接。 |

时间均 UTC RFC3339 毫秒，nullable 明确 null；items 始终数组。`publishProtocolReady` 表达受支持根及所需 OS 发布协议可用，替代旧研究易误解的 durabilityVerified 名称，**不表示真实断电已验**；新鲜完成仍要求每次实际协议成功。retainedCount/超额只有完整计量才报确定值。historyTruncated 只取存储计数>0，不用列表恰有200条猜测；最后失败不因历史压缩消失，用户取消不覆盖 lastFailure。

错误建议统一为新域 `{error:string,code:string}` 固定中文安全消息；未来时间拒绝额外给 serverTime/窗口信息。继承中间件的401/403可保持原 `{error}`，不虚假承诺所有失败都有 code。400 `backup_invalid_input`、413/415沿输入错误；409 `backup_busy`/`backup_request_time_ahead`；410 `backup_request_expired`；503 `backup_reconciling`、`backup_storage_unavailable`、`backup_storage_mismatch`、`backup_storage_unsafe`、`backup_clock_rollback`、`backup_clock_unavailable`、`backup_dedupe_full`、`backup_ownership_full`、`backup_history_full`、`backup_retention_pending`。账号变更409与会话无效401仍走原优先级；500用 `backup_store_failed`/`backup_result_unknown`，后者明确要求原 key 核对。运行失败码固定允许 `backup_no_space`、`backup_budget_exceeded`、`backup_io_failed`、`backup_validation_failed`、`backup_publish_failed`、`backup_queue_timeout`、`backup_execution_timeout`；中断码为 `backup_user_cancelled`、`backup_shutdown`、`backup_process_interrupted`。日志只输出安全随机ID、固定stage/code及计数；不输出路径、文件名异物、SQL、原始异常、Cookie、口令或会话hash。

## 4. UUIDv7 与高水位：拒绝也是必须确认的事务结果

固定 W=86,400,000ms、F=300,000ms；UUID规范位含 version7/variant，解析48位毫秒 T，要求 `0≤T≤253402214399999`，checked 得 `expires=T+W`。UUID仅幂等键，不是凭据或可信时钟；客户端时间从不推进 H。实际 N、H、E及所有持久毫秒取 int64，范围0..253402300799999；实际时钟越界不能夹到边界冒充正常，返回受控不可用。拒绝回拨/容量时不得回滚已正常观察到的 H。

1. 严格输入、同源/初次鉴权在事务外；不做根 I/O。BeginTx 取得连接时不持 manager.mu。事务内用实际当前认证时间重新 validateAccountMutation，取一次合法实际 N，读取唯一 store/policy，令 E=max(H,N)，仅 N>H 时推进 H；缺singleton/SQL失败是故障，不能用默认0继续。
2. **先找原 key/run 事实**：已知 queued/running 即使过期、回拨、根不ready、busy也返回原 active；已知 terminal 在 E<expires 重放，否则410。无 key 但 run.request_id 仍在，按同一 T/原 run 重放或410，不能再次插入；不续期、不创建第二 run。terminal=0 却无对应 active 或 id/seq/state 不一致须 fail-closed，不伪造结果。
3. 只有新 key 才检查 `T≤E-W`→410、`T>E+F`→409；用受检加减/差值避免边界溢出。再检查 `H-N>F`→503，恰好 F 允许；客户端未来 T 不能让 H 前进。旧 active、查询、取消及兼容同步导出不受新意图回拨门禁阻挡。
4. 对新 key 顺序核 closing、根缓存ready/对账完成、无 active、无未结束授权/deleting、容量及时间上界。未准备根的503可在提交确认后设置合并prepare唤醒；任何根故障/业务拒绝都只作为 Outcome 保存，**仍提交 H**，不提前 return error 导致回滚。N+30分钟截止不可表示时拒绝接纳，不插入不完整 run。
5. 仅准备接受时，事务内计算有界 expired-terminal-key/eligible-run 清理计划；容量确可满足才执行清理并插入 queued+terminal=0 key，冻结当前 policy_revision/keep/max，created/accepted取 N，expires 固定 T+W，admission_deadline=N+1,800,000。新 key busy 不建 run、不占 key 槽。所有状态CAS检查 RowsAffected=1。
6. 提交前重新检查请求 ctx、closing 及 validateAccountCommit 的**实际时间**过期条件，不拿 E 代替认证时钟。已确认 rollback 的认证/取消失败取代候选 Outcome；Commit/rollback 结果未知统一独立分类，不发送尚未确认的业务拒绝/接纳。提交确认后才响应/唤醒；同原 key 读回是唯一恢复入口。

已开始提交与 shutdown 存在边界时，视为在途操作，不承诺强制回滚；请求排空后再终结其可能已提交的 queued。拒绝事务不附带历史裁剪。GET不写 H；唯一 executor 可利用原 timer 至多每分钟进行一次 DB 维护，按同样实际观察推进 H、清过期终态 key/合规历史，不能另起清理循环。大幅误跳后不自动降低 H、清表或换 storeId；仍允许旧GET作临时导出，没有新增时钟修复CLI。

## 5. 200 / 64 / 2048 上限与裁剪

- 每次服务提交后 `backup_runs≤200`、非 deleted `backup_files≤64`、key总行数≤2048；采用201/65/2049 lookahead 检出已有漂移，不能 SELECT 后静默截断假装正常。所有陈旧/终态 key 也占2048，只有 terminal=1 且 E≥expires 才可清；active key 过窗仍不驱逐，不按访问时间LRU。满额且不可清则503。
- 新意图准入及 pending 登记前都重查 ownership；保守预留一份新目标能力，不通过抹 ownership 或先删旧文件换空间。target pending/missing/unverified/deleting 均占64，source私有 `.backup-pending-*` 强杀残留不是 target ownership，不能声称64也限制那些 S03.1 旧候选；本片不扫描接管/清扫它们。
- history 只按 seq 最老终态裁剪，active、任何非deleted文件所关联的 run、活跃授权来源、guard及deleting相关事实受保护。终态 key 自带≤2KiB压缩摘要，可独立于完整 run 保留，故不要求为有效终态 key 永久保留 run；terminal=0 不得失去 active 事实。若 key缺失而 run 是窗口内唯一去重事实，不能同时裁掉使原 key 复活，须修复同事实或阻新意图等待核对。
- 实际裁剪数量与 `history_truncated_count` 同事务增加，MaxInt64后饱和；不称永久审计总数。不级联文件/key；deleted墓碑仅在其 run 已裁剪且不再被任何活跃指针/授权引用时有界清掉。历史/墓碑DB清理不触碰文件系统。
- root 最多128直接成员、ReadDir(129) lookahead；先看≤64登记文件，再做有界根核对，严格子目录成员沿冻结 n+1规则。不凭名字 adopt，无意图/未知成员、超预算或完整性关系不成立即 usageComplete=false、阻新意图，保留材料。

## 6. manager、根准备与锁序

唯一 executor 在 RunWithOptions 开始等待前注册 a.wg，容量1合并 wake，一个可复用 timer；busy约1秒重试，根恢复/空闲≤30秒检查，DB维护另以同 timer 内的到期事实限频。不每run/文件/重试开goroutine、不新建第二队列。root_bound=1启动先核根/对账，API可查询缓存；root_bound=0须显式合法POST且提交确认后才请求初次根准备，合法匹配marker的绑定提交未知按原store读回，不换根绕过。

根 lease 由 executor 唯一获得/持有/Close；HTTP只看已发布的内存状态和DB，不打开成员。prepare请求合并、失败有界退避；缓存ready不是替代关键步骤 root.checkIdentity。无法证明原根身份则 blocked，不能热跟随同marker的替换根；只有相关I/O全部结束并关闭旧lease后才允许明确重新准备/完整对账。平台支持矩阵、精确路径隔离和marker逻辑命名空间边界沿S03.2.2，不重复创造另一套根协议。

统一顺序为：**取得事务连接 → 读DB事实 → 短mu核closing/安装或取出精确句柄 → 释放mu → SQL/CAS/Commit → 短mu发布已确认缓存**。不在mu中 BeginTx、等SQL、Commit、hash、Flush或Close；不从持mu路径反向拿DB连接。claim先为该run准备子context并在提交前安装其唯一句柄，提交失败只移除同对象，未知则保留对账占位；在途Commit允许cancel先触发子context，但直到提交确认才允许source/目标I/O。取消事务确认后再取匹配run句柄，释放mu后调用cancel；晚到信号只能持有旧对象，不能取消后来另一run。

closing 使用一次性seal状态，短mu不包数据库等待；进入提交的在途准入由请求排空与末尾收尾承担，不凭内存“已封口”推断其肯定未提交。没有永远持锁的数据库事务；single connection 已串行化持久状态，mu只补充句柄/生命周期关系。所有新方法入参/出参显式类型，方法/实体字段/固定参数/关键分支中文注释，业务接缝输出固定安全中文日志。

## 7. 一次运行、峰值预算与 pending 登记

1. queued先核 durable cancel、closing、截止。CAS queued→running（started取实际时间）并安装唯一句柄后，运行子context来自a.ctx、最长30分钟，与HTTP断开无关。排队同时受持久 admission_deadline 和同进程单调30分钟预算限制；跨任意回拨/重启不能声称精确真实经过30分钟，重启仍沿原截止，不续期。
2. source前在事务/mu外取得保守 S=`max(普通主文件尺寸,checked page_count×page_size)`，M固定1MiB，防负数及int64/uint64溢出；真实源卷需 MinFree+2S，能证明不同卷时目标需 MinFree+S+M，同卷或不能证明不同卷则合并 MinFree+3S+M。根/卷身份前后核对；这只是估计、不是空间预留，不能先删旧材料。
3. 调共用 createDatabaseSnapshot；不预锁backupMu。仅 busy且未有fileId/目标意图时，重新短事务核取消/closing/截止，退回queued、started=NULL、保留原截止，复用timer再试；不刷失败历史。其它生成/预算/校验失败进入真实失败或取消中断，不造第二source路径。
4. 取得唯一source后再分配fileId，固定 owner原始字节/长度/SHA；source尺寸明显超run预算可早拒，完整单份 `DB+owner+manifest≤run.max_bytes` 仍由publisher按实际内容确认。短事务复读same store、running、无cancel/closing、无旧授权/deleting、ownership有槽，登记pending并令run.file_id=原fileId；确认提交后才允许目标Mkdir/写文件。
5. 调冻结publisher借用root/source；publisher目标复核 `MinFree+实际sourceSize+1MiB` 和单份预算。全程无长事务/mu；source唯一Close归调用者，不重开、不移动、不删除其路径。不论publish error是否nil，都同时消费 Certainty：confirmed进入完成CAS；not_published仅在确无未知旧发布时安全清理原pending/终结；needs_reconcile保留原ID与材料，冻结执行，转无source对账。
6. owner登记、claim或完成的Commit未知均先读原ID，不盲目重调source/publisher。source.Close无论发布结果均执行；confirmed后的源Close错误只增加资源收尾告警，不撤销已发布/已完成事实、不重发授权。预检通过也仍处理实际FULL、卸载、ACL、Sync/Close错误。

## 8. 完成CAS与无删除型授权结束

完成事务核对原running/pending、store/run/file/seq及owner tuple；confirmed的实际schema/size/hash、manifest长度/hash、原manifest appVersion/source createdAt填完整tuple，published/verified取本次协议实际确认时间，不伪造崩溃前时刻。要求旧授权三字段全NULL、无deleting；同事务CAS retained/completed、写terminal key/安全摘要、lastSuccess/lastGood，读取此刻最新policy.keep/max发放三字段。run准入冻结参数不变；已completed重复读回不再次发放，不把别的授权覆盖/合并。根/SQL异常保留待对账事实，不能制造“文件成功但没权限”的旁路成功事务。

为避免第一份成功后永远挡住第二份，建议本片实现以下严格子集，**不是轮转**：

1. 三字段非NULL时在事务/mu外核根、≤64ownership、精确成员、实际逻辑字节、当前有效guard。guard取最高run_seq实际严格核验有效的retained，不把lastGood指针或缺失/不完整文件充数。必须无pending/deleting/unverified/未知材料等需要破坏性处理的对象；所有占用可完整计量，禁止未知按0。
2. 令原授权为 `(authorizer,Ka,Ba)`，当前为 `(revision,Kc,Bc)`，取 `Keff=max(Ka,Kc),Beff=max(Ba,Bc)`。只有真实保留数量≤Keff、全部已知自有逻辑字节（DB+owner+manifest及已登记残留）≤Beff，guard仍有效、根身份稳定，才允许结束。
3. 短事务复读**原三字段完整token**、当前policy修订/阈值、guard及相关文件状态/元数据；与I/O决策快照一致才CAS三字段一起NULL并检查RowsAffected。漂移丢弃快照、有界重算；提交未知读回原token。不得从lastGood/旧completed续发，不更新旧授权阈值，不选择受害者、不置deleting、不unlink。
4. 超阈值/无法证明/任一未知均保留授权和completed，展示retentionPending，下一新POST503/retention_pending。旧key重放/取消/查询/旧GET仍可用；S03.2.4再消化删除。默认keep7可持续积累至满足目标的7份；第8次成功可保留为成功和积压并阻下一份，字节也可能更早触发。不得宣传本片无限连续手动轮转。

`persistent-retention-authorization.md` 的“唯一guard超预算仍结束，允许更小替代”需要完整guard下限/可删对象判断，**本片保守不启用该例外**；源/manifest额外开销也可能使旧guard落入此限制。只要阈值不满足就保留原授权，明确阶段限制，不能省发授权换取继续生成。S03.2.4必须接回guard下限规则，否则将形成永久阻断。

## 9. 无source对账、确切pending/final与Windows未知结果

建议新增包内 `backupPublicationFacts`（字段均中文注释）：`Publication backupPublishResult` 为平台确认事实，`AppVersion string` 为严格原manifest版本。入口只接受原意图，不收路径、source或外部expected：

```go
// reconcileBackupPublication 只核对原运行登记的候选并补足平台协议，不生成源快照。
func (a *App) reconcileBackupPublication(ctx context.Context, root *backupRootLease, runID string) (backupPublicationFacts, error)
```

入口自行短事务取意图后关闭事务；重用S03.2.2严格格式/身份/平台封装，所需新协调代码归S03.2.3。不可调用原publisher去“试一下既有目录”，不可VACUUM/新fileId/任意adopt。strict manifest原始字节hash、owner原始hash、实际DB长度/hash/schema6/SQLite integrity/FK全部核实；metadata.CreatedAt取原manifest，appVersion不改成当前程序常量。每步受有限context，所有长核验在事务/mu外；移动/完成前CAS原意图未漂移。

| 持久事实 / 实际材料 | 本片精确处理 |
| --- | --- |
| queued且无file意图 | 检查取消/closing/原截止后继续同run；不换runId。 |
| running、file_id=NULL | 进程中断，终结原run，不重做source；已有source私有残留不扫不adopt。 |
| 原pending，根整体健康，两名均不存在 | 原run interrupted/process_interrupted（有durable取消则user_cancelled），file直接deleted并记实际时间；只在确认根不是卸载/身份失联后补缺失事实。 |
| 自有但不完整pending | owner原始长度/hash及store/run/file匹配，目录/成员普通、精确允许名和身份成立后才逐个清理，owner最后、空目录最后；无完整发布tuple也可pending→deleted。清理失败保留ownership/pending或unverified；deleting不承载未发布清理。 |
| owner最后删除后仅剩空pending目录 | 仅普通、精确登记且没有final_seen的未发布pending，在健康受控根下以n+1核验确为空后可单次Remove空目录；非空/不明/已标记则保留。owner缺失但还有任一成员不能凭名字清理。此窄窗仍受marker仅逻辑命名空间的边界约束。 |
| 完整pending，仅manifest.json或仅完整manifest.json.tmp之一 | 核实全部事实；普通未标记pending先尊重durable取消，不再执行首次最终发布。未取消则必要时将同原始tmp无覆盖提升manifest，再逐文件flush/close、正确目录同步、最终无覆盖移动/后验核验，confirmed后补原run完成；不重新编码manifest。 |
| 只有完整final，原run仍running/file仍pending | 视为未知发布，先严格核验，再按下述原ID协议补确认，完成后原CAS；不能只因可读/hash正确直接completed。 |
| pending与final同时存在、任一名为异物/链接/未知成员 | conflict/unverified，保持两者和原ownership，不猜选、不覆盖、不自动删一边，不换ID。 |
| owner/hash/store不符或terminal失败后外部修好 | 保留unverified/人工核对；不把外部修补自动提升为新的成功，不复活终态run。 |
| completed/retained重启 | 只读核验；保留历史completed。根整体正常而确实缺失才file missing，坏/身份不符则unverified，重选有效guard；不发新授权、不做Windows回环。 |
| 既有deleting/未完授权 | 本片不执行已完成文件删除。先尝试第8节无删除结束；需要删除或存在deleting则可见积压并阻新意图，交后片，不伪清token。 |
| 无DB意图的任何相似产物、根整体离线/锁占用 | 不adopt、不补completed、不扫删；根失联不批量missing/deleted。先报告根故障并有界重试。 |

**Windows final恢复最小方案（待主线冻结）：** 仅原running/pending、未有completed登记、没有持久下载/成员租约的未知发布可进入；S03.2.3本身不开放持久下载。正常retained不写盘。只重新flush普通文件无法证明/补足要求的目录发布移动，因此对未知final需要重新取得一次规定的最终写穿移动确认，而非为所有启动多做写盘。

先严格验证final及原意图，所有只读/SQL/摘要子句柄Close确认；短事务CAS该pending的现有 `backup_files.error_code="backup_reconcile_final_seen"`，**提交确认或精确读回确认后**才允许下步。当前DDL允许pending的error_code为≤64字节安全固定值，且pending不强制完整发布tuple；不加字段、不假填published_at。该值由协调器内部生成，不从API/manifest输入，不表示已完成/已授权；Commit未知先停下读原ID，不能先搬目录。

随后按原fileId逐步：核根/目录/精确成员 → 关闭Windows子句柄 → 同根无覆盖WRITE_THROUGH `b-id→.pending-id` → 重新核根/同目录身份和内容 → 以无截断、无新增成员、受控写句柄请求WRITE_THROUGH逐文件Sync/Close → 严格复验并关闭子句柄 → 无覆盖WRITE_THROUGH `.pending-id→b-id` → 根/最终目录/成员/真实内容后验复核及全部Close成功。任一步异常保留原ID、原token/标记和材料，结果needs_reconcile；两名同存/异物仍fail-closed。原语禁REPLACE_EXISTING/COPY_ALLOWED/失败回退，不要求Windows执行无等价语义的POSIX目录Sync，也不宣称物理断电保证。

`final_seen`填补回环中断接缝：final→pending后退出，重启完整pending仍知道“已观察最终发布事实”；不得因durable取消把它当从未发布而删。该分支用a.ctx派生的有限恢复context继续确认原发布，忽略用户取消信号对该确认的撤回；停服仍可中断并保留。普通无此标记pending先按取消清理；已见final分支确认后completed优先、保留cancelRequested。标记在成功retained时清NULL，任何unknown不以普通错误覆盖丢失它；不完整/异物的已标记pending一律保留待人工核对，不能套普通partial清理。标记不是通用修复状态，不允许据它跳过owner/schema/hash/平台校验。

Linux已有完整final同样不能凭读回成功补完成：对严格原意图以受控写句柄重新逐文件Sync/Close、正确备份目录与根dirfd Sync，再后验核验；无需无谓反向rename。完整pending按冻结Renameat2 NOREPLACE及正确pending/root目录Sync收尾。若平台不能补足协议，保持unknown，不降级completed；新协议只证明本次OS调用确认，不倒推出此前断电持久性。

## 10. 取消优先级与停服

- queued取消同短事务写cancel_requested首次实际时间、interrupted及terminal key/摘要；running用COALESCE保留首次时间，确认提交后才signal精确句柄；终态200不新增迟到cancel标记。不因重复取消创造新run或新的消息历史。
- 已confirmed目标/已提交completed优先于取消；needs_reconcile优先于“取消/超时所以failed”。未有未知发布时，durable用户取消优先shutdown、shutdown优先execution_timeout，最后才普通失败。普通未发布候选按确切所有权清理；已见final的标记分支按第9节，不抹去发布事实。状态不确定不提前释放为可接纳新运行。
- 句柄保留至source/目标资源收尾完成且状态明确，或已建立阻新意图的持久unknown并转受控对账；卸下句柄只比较同run/同对象。HTTP取消可能因单连接鉴权/写入等待，执行子context超时/停服信号不依赖该HTTP写入即时成功。
- RunWithOptions在既有 requests.stop 附近先seal新准入/prepare并signal当前句柄，再取消workCtx、关闭HTTP；不在此等待DB/mu内I/O。增加一个一次性requests-drained信号，现有 requests.wait 完成后发给**同一个**executor，再a.wg.Wait。executor停止新的source/hash/恢复写盘，收好现有资源，等待在途handler排空后，才用独立最多5秒DB清理context终结尚queued/可明确中断的running；避免worker先退出而在途POST刚提交queued。此信号不增加goroutine/队列，不在Wait后Add。
- 5秒只约束DB收尾预算，不承诺底层文件Close/设备I/O可即时取消；停服不新做大库核验、不补跑完整发布回环。已confirmed事实可在同有界context补完成；无法安全记终态/Commit未知，保留原running/pending及安全cleanupErr供重启。源租约唯一Close、executor全部I/O结束后Close根；DB/DataDir租约仍最后释放。收尾异常合并到App.cleanupErr并记中文安全日志，不覆盖主事实。

## 11. 冻结后的小步实施与待验输入

| 顺序 | 可独立审查产物 | 必须验证后才前进 |
| --- | --- | --- |
| 1 | 本片执行/DTO/错误/锁序冻结，新增局部严格decoder及UUID时间/summary纯函数 | 规范UUID位/时间上界；重复键/别名/尾随；毫秒及JSON字节上限；现有账号/旧GET不扩张。 |
| 2 | 存储服务：Outcome提交、高水位、原key重放、容量/历史/维护 | T=E-W过期、E+F边界、active过窗、rollback300秒边界；拒绝H提交；Commit/rollback未知；200/64/2048含保护行/压缩重放，饱和计数。 |
| 3 | manager唯一根/prepare/worker，领取、峰值预检及pending→publisher→完成 | 默认root_bound0零根I/O；首POST503后原key重试；单连接锁序；旧下载busy仅queued无file；同/异/不明卷峰值及溢出；owner先登记；成功原子授权。 |
| 4 | 无source原意图对账与无删除授权结束 | 每个pending/final/DB断点；final_seen CAS/未知/回环两次移动间重启及取消；完整manifest.tmp；两名并存/异物保留；当前schema6实际检查；没有VACUUM/新ID/adopt。 |
| 5 | 五路由、取消/关页/停服联动，局部文档与稳定验证脚本 | 真实鉴权撤销/过期、Origin/no-store/输入；cancel/claim/complete barriers；已接受HTTP断开仍执行；请求排空后DB收尾/lease释放及立即重启；同产物旧backup/account/core回归。 |

授权门禁专列：第一次成功三字段必发；满足阈值清空后第二次可接纳；第8份或字节超限成功仍为completed且下一意图503；当前/授权数量与字节一严一宽按各分量max；guard坏/未知残留/计量溢出不清；原token或policy revision漂移不误清；completed重放/重启不复活授权。唯一guard超预算例外明确待S03.2.4，不借本片伪称目标已满足。

临时Go测试将来执行后按项目要求清源；不留生产故障开关/等待端口/长期hook。真实Windows本地NTFS产品与进程中断证据、Linux交叉编译、真实Linux/NAS、系统崩溃、物理断电各自记账，不拿mock/fixture/正常重启互相替代。待主线冻结并激活后实施者才运行gofmt/vet、临时行为测试、清源编译、fresh产物、相关稳定回归与TypeCheck；本研究未运行，不能填pass。正式spec/docs及任务进度由主线统一同步，本文不先行改写。

## 12. 本轮交回边界

已落实的是本研究中的最小契约建议，无产品代码修复。待主线冻结的实质取舍为：有限UUIDv7窗口与事务拒绝提交；只读策略/首次POST准备及DTO；mu外提交与请求排空信号；final_seen私有持久事实与Windows未知final回环；本片仅无删除授权结束及guard下限延期。它们已给出单一可执行推荐，不是要求S03.2.2返工；后片依赖仍以S03.2.2最终交付和独立门禁为准。

Lint：未运行（研究范围）；TypeCheck：未运行（研究范围）；Tests/fixture：未运行（研究范围）。未新增临时测试源、未提交、未部署、未操作用户数据；不将本文计为S03.2.3已实施或任何产品验收通过。
