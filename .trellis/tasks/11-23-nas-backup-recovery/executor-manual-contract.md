# S03.2.3 执行器与手动备份实施契约

2026-11-23。状态：**已冻结并激活，进入 S03.2.3 / Phase 2.1**。S03.2.2 已经实施、独立可写检查及主线原始证据关闭，记 local_complete_target_pending；其最终报告 SHA256 为 `4baa31ee219316e75e2290f12f37201d97ca0f81a66d090386d81b069835bbdb`。主线已用 CodeGraph 核对 acquireBackupRoot、publishBackupSnapshot、createDatabaseSnapshot、validateAccountCommit 和 RunWithOptions 的实际接缝。本契约是当前片实施权威，根/发布及 schema6 已验收边界继续有效；冻结不代表 S03.2.3 已实现或门禁通过。

## 1. 范围与权威

- 本片只提供一个 manager、一个 executor、五个手动/只读路由、有限窗口幂等、取消、原意图无 source 对账及无删除型授权结束；策略保持 `enabled=false`。
- 不提供策略 PUT/启用、UTC 调度、持久备份下载/HEAD/下载租约、已完成备份删除或轮转、恢复 CLI/UI、历史 schema checker、新迁移/第六表、第二数据库连接、生产测试后门。
- 原 `GET /api/backup` 的同步响应、HEAD、busy 语义和唯一快照租约保持兼容。旧快照与新执行器争用同一 `backupMu`；调用共用生成服务前不重复加锁，不复制正在运行的 WAL 主文件。
- 前提为已独立验收的 schema6 五表与 S03.2.2 根/格式/平台协议。发生冲突时，已验收 storage/root 契约优先；不能为本片方便削弱路径隔离、平台无覆盖发布、关闭检查或三类确定性。
- 技术附录为 `research/executor-manual-contract-review.md`，169 行、33,779 字节，SHA256 `59445a34413f8633d674ece3a0b5cd74353899a0bf03800252697bc08b2506b3`。该文第 2–10 节的单一推荐与第 11 节门禁按本契约补充成为实施要求；冲突以本契约为准，研究文件原有“未冻结/待主线冻结”描述仅是研究交回时状态。
- 冻结前接缝复核为 `research/executor-manual-freeze-review.md`，100 行、12,087 字节，SHA256 `74cf3f17a0c443e413b3398cb6f7539ddd8eeff69decb9f99925e14b817c31e2`；其六组最小修订已并入冻结稿，不另改 schema6 或根/发布协议，不构成产品验收。
- 所有方法/字段/固定参数与关键逻辑遵循 AGENTS 中文注释、显式类型和安全中文日志。日志只输出随机 ID、固定 stage/code、计数，不输出路径、原始异常、SQL、Cookie、口令或会话哈希。

## 2. 接口与数据契约

| 路由 | 输入及已确认响应 |
| --- | --- |
| `GET /api/backup-policy` | 无 query；200 `{policy,storage,execution,clock}`。只读已提交 DB/缓存事实，不准备根、不推进时钟、不 hash 或清理。 |
| `POST /api/backups` | 无 query；仅 application/json `{requestId:string}`，规范小写 UUIDv7。新接纳 202 `{run,replayed:false,historyRetained:true,expiresAt}` 及本地 Location。已知 active 202；窗口内 terminal 200；重放 `replayed=true`，同一 runId，不续期。完整历史已裁剪时返回压缩摘要及 `historyRetained=false`。 |
| `GET /api/backups` | 可选单值 `limit` 默认20、1..100；可选 `cursor=v1.<正十进制seq>`，无前导零、最多19位且≤MaxInt64。拒绝空值、重复及未知 query。seq DESC，limit+1；200 `{items,nextCursor,historyLimit:200,historyTruncated}`。 |
| `GET /api/backups/{id}` | id 精确32位小写hex，无 query；200 RunView，已裁剪404。404不授权重新使用旧 key。 |
| `POST /api/backups/{id}/cancel` | id 同上，无 query，仅严格 `{}`。queued 终结200；running 持久取消确认后202；已终态200且不新增迟到取消标记；未知404。202不表示文件已删除或执行器已退出。 |

严格逐 token 解码，拒绝大小写别名、解码后重复键、未知键、null、尾随对象及不合法 UTF-8/NUL；64KiB 上限与已有一次 unread-body 收尾保持，不叠加全局短 WriteTimeout。沿用真实鉴权、Origin/Sec-Fetch 和 no-store。HTTP 手动准入与取消在同一事务内按真实 actor 重验账号修订/会话，提交前按实际认证时间复核有效期；后台领取、完成、对账、授权结束及维护没有 HTTP actor，只受生命周期、原意图/CAS 和持久约束，不要求原会话继续存续。

DTO 字段、nullable 和字节上限以技术附录第 3 节为精确表：

- policy：`revision` 十进制字符串，`enabled` 布尔 false，`hourUTC/minuteUTC/keepCount/maxBytes` 为有界数值；不开放写参数。
- storage：`status=ready|reconciling|unavailable|mismatch|unsafe`、固定 `scope=database-only`、`publishProtocolReady`、完整计量及积压事实。聚合字节 `ownedBytes/excessBytes` 为十进制字符串或 null；计量不完整不得报0。`publishProtocolReady` 仅表达所需 OS 协议可用，不表示物理断电已验。
- execution/clock：activeRunId、lastSuccess、lastFailure；serverTime、highWaterAt、固定窗口86400秒/未来容差300秒、rollbackDetected。高水位0显示 null，GET 不推进。
- RunSummary：随机 id、trigger、state、createdAt/startedAt/finishedAt、安全 errorCode/message、cancelRequested。UTF-8 JSON 编码≤2048字节，message≤256字节。
- RunView 增加 policyRevision、scheduleRevision/scheduledAt、skippedSlots 及 file。seq 只用于服务端 keyset，不输出 JS number。
- FileView 仅在完整发布 tuple 可用时返回 id、availability、size、sha256、schemaVersion、createdAt；无完整 tuple 的 pending/unverified/deleted 为 null，无下载链接。
- API 时间 UTC RFC3339 毫秒；所有空值显式 null，items 为数组；historyTruncated 来源于持久裁剪计数而非列表长度。manifest 原始 createdAt 继续使用冻结根契约的规范 RFC3339Nano；对账不得按 API 格式重编码或改变原 bytes/hash 与内部精度。

新域失败为固定安全中文 `{error,code}`，具体 code 和额外时钟字段采用附录第 3 节允许集。原中间件401/403可保持 `{error}`。提交未知统一500/`backup_result_unknown`，不能发送未经提交确认的202/409/410/503。

## 3. 时钟、事务及容量

1. UUIDv7 必须验证规范字形、version/variant 与48位毫秒 T；W=86,400,000ms，F=300,000ms。持久毫秒合法范围0..253402300799999；T≤253402214399999，使 T+W 可表示。拒绝非法实际时钟，不夹值；客户端时钟不推进高水位 H。
2. 不持 manager.mu 取得事务连接；真实当前认证后读实际 N、singleton/policy，E=max(H,N)，N>H 时推进 H。先查原 key/run：已知 active 无论过期、回拨、根故障或 busy 都返回同一事实；terminal 在 E<expires 重放，否则410；key 缺失但 run.request_id 尚在也不得重新接纳。
3. 仅新 key 判断 T≤E-W 过期、T>E+F 超前、H-N>F 回拨；恰好 F 允许，checked 算术防溢出。检查 closing、根缓存/对账、active、授权/deleting、容量及 N+30分钟截止。业务拒绝作为 Outcome，仍提交正常观察的 H，不以 return error 回滚高水位。
4. 未准备根的合法新 POST 返回503/reconciling、Retry-After:1；只有高水位提交确认后才合并唤醒 executor 准备，不创建 run/key，沿原 key 重试。GET 不初始化根；disabled/root_bound0 启动零持久根 I/O；bound1 启动异步核根，不阻塞媒体就绪。
5. 只在确认可接纳时，同事务完成有界过期终态 key/合规历史维护及 queued+terminal0 key 插入。冻结 policy_revision/keep/max、N 与 N+30分钟原截止；所有 CAS 要求 RowsAffected=1。拒绝事务不附带历史裁剪。
6. 提交前重新检查 ctx/closing 和 `validateAccountCommit` 实际认证时间；确认回滚的认证/取消失败取代 Outcome。Commit/rollback 未知保留原 ID、返回 result_unknown，只读回原事实，不重跑 source、不换 key。
7. 上限为200 run、64非deleted ownership、2048 key；分别201/65/2049 lookahead 检出漂移。active key 即使过窗也不能清；仅 terminal=1 且 E≥expires_at_ms 可清。pending/missing/unverified/deleting 占64，source私有遗留不纳入该上限或自动接管。
8. 历史按 seq 最老终态裁剪，保护 active、非deleted ownership、授权、guard/deleting及窗口内唯一去重事实。terminal key 允许独立保存压缩摘要。裁剪计数同事务增加、MaxInt64饱和；只清无引用 deleted 墓碑，不触碰文件系统，不级联文件/key。
9. 根最多128直接成员，ReadDir(129)；先看≤64登记ownership，再核根。未知/超限/关系不一致使 usageComplete=false 并阻新意图，不 adopt、不按未知0计量。

高水位提交规则仅用于正常鉴权、ctx 与最终 closing 检查仍通过的手动准入/维护事务；维护没有 HTTP actor。最终认证/ctx/closing 失败且 rollback 确认时，H 和 run/key 一并回滚，只返回该失败，不发送候选410/409/503，也不开第二时钟事务。Commit 确认后再取消不撤销 H/run；真正未知才 result_unknown。一次事务固定 E 用于窗口与裁剪，认证时钟复采不改变 E。key 的 terminal/runId/seq/state 不一致时阻新意图并读回核对，不伪重放或另建 run。

## 4. 唯一执行器、发布与授权

- 在 RunWithOptions 开始等待前注册 a.wg；一个 executor、容量1合并 wake、一个复用 timer。busy 约1秒重试，空闲/根恢复≤30秒核验，DB维护至多每分钟一次，均共用 timer；不为每 run/文件/重试新增 goroutine 或连接。
- 根 lease 仅 executor 获得、持有和关闭，HTTP只读缓存/DB。ready 不能替代关键 I/O 的 root.checkIdentity；映射变化持续阻断，全部旧 I/O结束、旧lease关闭后才允许明确重新准备和完整对账。
- 锁序：取得事务连接→读DB→短mu核closing/精确句柄→释放mu→SQL/CAS/Commit→短mu发布确认事实。mu不包BeginTx、SQL等待、Commit、hash、Flush或Close。claim提交前安装唯一run句柄，失败只按原run/同对象移除，未知保留占位，确认提交前不做source/目标I/O。
- 根/usage/对账缓存仅由唯一 executor 写入；HTTP 确认提交后只合并 wake 并返回当次确认响应，不覆写缓存、不安装/清空句柄。run、摘要、policy、H 等持久展示事实由只读 DB 查询取得；closing/seal 独立且单向，不被较早快照重置。HTTP 返回 queued 后 GET 已 running 是正常进展。
- queued先核取消/closing/原截止，CAS running并安装子context，最长30分钟且不依赖HTTP连接。原 admission_deadline_ms=N+1,800,000 不续期；领取/每次busy退回前，按 max(已提交H,合法实际N)≥deadline 或同进程单调30分钟预算先到者判 queue_timeout。重启只恢复原持久截止；大幅回拨不重新套用新意图准入拒绝。跨任意回拨/重启不承诺精确真实经过30分钟。
- source前在事务/mu外估计 S=max(普通主文件size,checked page_count×page_size)，M=1MiB。源卷需 MinFree+2S；可证明异卷目标需 MinFree+S+M；同卷/不能证明异卷合并需 MinFree+3S+M。核卷/根身份及算术，预检不是空间预留，不删旧材料救空间。
- 调共用 createDatabaseSnapshot，不预锁 backupMu。仅 source busy 且无目标fileId时，复核取消/closing/截止后退回queued、started=NULL、保留原截止，复用 timer。其它失败不重复生成第二source。
- 得到唯一 source 后分配fileId，固定owner原始bytes/size/hash；短事务重核same store/running/取消/closing/旧授权/deleting/ownership，登记pending和run.file_id。只有提交确认后调用冻结publisher；源唯一Close归调用者，不重开/移动/删除源路径。
- 同时消费 error 与 Certainty：confirmed完成CAS；明确not_published才允许确切清理/终结；needs_reconcile保留原ID和材料，阻新意图并转无source对账。不把已确认发布后的source.Close错误改为可盲重试失败，只记录资源收尾告警。
- 完成同事务核原running/pending及owner双向归属，填目标实际完整tuple，retained/completed、terminal摘要、lastSuccess/lastGood及三字段授权原子提交。此刻读取最新policy keep/max发授权，旧授权必须全NULL且无deleting；重复completed只读，不重发/覆盖授权。
- 无删除型授权结束按附录第8节：事务/mu外核全部≤64 ownership、精确成员/实际字节和最高run_seq有效guard；仅无未知/待处理对象、数量及字节≤各分量max(原授权,当前策略)、根稳定才短事务核原三字段token/policy revision/guard快照并CAS一起NULL。漂移重算，提交未知读回。
- 超额/未知保留completed及原授权，展示retentionPending，阻下一新意图；默认keep7可积至7份，第8份可能成功后积压。唯一guard超预算下限例外及删除仍待S03.2.4，不省发授权绕过，不宣传无限连续手动轮转。
- **空闲周期核验必须轻量**：检查根/锁/marker/已知缓存失效等元数据；不得每30秒重新hash全部历史SQLite。启动完整对账、实际使用/授权结束的必要严格校验与周期轻量检查分开，变更后使缓存失效，不以永久缓存替代关键核验。
- 授权结束由启动对账、新成功、token/策略/登记事实变化或轻量检查发现根/成员身份及计量变化触发，以同 executor 的 dirty 事实合并。先有界核状态/成员/实际计量；已明显超额、deleting/pending/unknown 或计量不全时保留 token 并 blocked，不反复 hash 以证明仍不能结束。未变化的阻塞 token 后续 tick 仅轻量检测；条件可能满足才严格核 guard/相关文件并 CAS。I/O 失败有界退避，无紧密重算或新 timer/goroutine；实际使用/结束 CAS 前仍核必要根/guard。

## 5. 原意图对账与停服

新增包内协调入口可采用下列签名，不接收外部路径/source/expected：

```go
// reconcileBackupPublication 只核对原运行登记的候选并补足平台协议，不生成源快照。
func (a *App) reconcileBackupPublication(ctx context.Context, root *backupRootLease, runID string) (backupPublicationFacts, error)
```

`backupPublicationFacts` 含平台确认 Publication 和原严格manifest的 AppVersion，字段中文注释。短事务读原意图后关闭再I/O；复用S03.2.2严格格式/身份/平台原语，不重调publisher碰撞路径、不VACUUM/换ID/adopt。实际schema6、完整性/FK、size/hash、owner及manifest原始摘要均核验，时间/appVersion沿原manifest。

对账分支精确采用附录第9节矩阵：queued无file续原run；running无file按进程中断终结。仅根整体健康且身份一致、确切pending/final两名均缺时，原running/pending同事务以interrupted终结（durable用户取消优先，否则process_interrupted）、固定key终态摘要并置file为deleted/填deleted_at；不重生source。已completed/retained的final确实缺失则仅file置missing，保留原完整tuple/历史completed，重选实际有效guard；根整体失联不落任一种缺失事实。不完整自有pending仅在owner/身份/精确成员确认后逐项清、owner最后；owner最后删除后的确切空未发布pending可窄Remove；普通完整pending先尊重durable取消；manifest.tmp只提升原始bytes；两名并存/异物/未知/错owner保留。终态不因外部修复复活。

Windows未知完整final仅限原running/pending：严格验证并关闭子句柄后，短CAS `backup_files.error_code=backup_reconcile_final_seen`，提交或精确读回确认才允许原ID写穿无覆盖 `final→pending→final`，中间逐文件受控WRITE_THROUGH/Sync/Close、核身份及原内容，每步全部必要关闭成功。禁REPLACE_EXISTING/COPY_ALLOWED/回退。已标记pending不得按普通取消/不完整清理；已观察final的完成事实优先取消，unknown保留标记，成功retained才清NULL。正常completed/retained启动只读，不回环。Linux未知final重新逐文件Sync/Close及正确目录/root同步，无需反向rename。协议失败保持unknown，不凭读回hash正确直接completed。

final_seen 是 backup_files.error_code 内部安全事实，不是 run.error_code、HTTP 输入、已完成或轮转授权。CAS 匹配原 store/run/file/seq、running/pending 和 owner；提交未知先读原 ID，未确认标记前不反向移动。仅完整原意图的已标记候选用 a.ctx 派生的有限恢复 context 补 OS 确认，用户取消不将其当普通未发布 pending 清理；停服/超时可中断并保留。普通未标记 pending 仍先尊重 durable 取消。

取消确认提交后才signal同run/同对象的句柄，在mu外cancel；旧信号不命中新run。已confirmed/已completed优先，needs_reconcile优先于取消/超时失败；无未知时用户取消优先shutdown、shutdown优先execution_timeout。持久取消因单连接真实鉴权/SQL可能等待，不承诺即时返回。

停服先seal准入/prepare并signal当前句柄，再按既有顺序取消工作context/关闭HTTP，不等待DB。requests.wait后发一次requests-drained给同一executor，再a.wg.Wait；无新goroutine/等待后Add。executor先停止新source/hash/恢复写盘，收好资源并等handler排空，再用独立≤5秒context处理可明确终结的queued/running或已confirmed完成事实；未知保留待重启，不新做大库核验或完整回环。5秒仅DB收尾，不承诺设备Close可即时取消。所有I/O结束后关闭根、错误合并App.cleanupErr，DB及DataDir lease最后释放。

requests-drained 使用一次性 close 信号，不能用可能阻塞的无缓冲 send；executor 注册后每条退出路径均须在停止请求并排空 handler 后保证关闭该信号。排空后所有明确终结/confirmed 事务共享一次≤5秒 DB 收尾预算，不逐 run 续期。收尾失败/未知保留原 running/pending 并记录 cleanupErr，最终仍关闭根和等待 executor；不将其描述为5秒内完成所有停服。

## 6. 实施顺序与验收

按附录第11节顺序推进：纯输入/UUID/summary→Outcome/高水位/容量维护→manager/claim/峰值/pending/publisher/完成→无source对账/授权结束→五路由/取消/停服与稳定集成脚本。每个接缝先有可审查行为证据，不用编译冒充行为通过。

必须覆盖：

- UUID规范位/上下界、重复键/别名/尾随/编码与字节上限；窗口恰到期、未来恰F及F+1、回拨边界、已知active过窗；业务拒绝提交H；Commit/rollback未知；同key压缩重放与不续期。
- 200/64/2048含保护事实、lookahead、饱和计数；默认启动无根、首POST503后原key重试；唯一执行器/根及单连接锁序；旧同步下载busy只queued无file；同/异/不明卷峰值、溢出及真实剩余不足。
- owner登记前后及发布/完成各断点；每个pending/final/DB组合；Windows final_seen CAS未知与两次移动之间的自有进程中断、durable取消；完整manifest.tmp、未知材料保留；无source对账不生成新快照或ID。
- 首次成功必发授权、阈值内结束后第二次可接纳、第8份/字节超限成功及后续503、各分量max、坏guard/未知/溢出不清、token/policy漂移不误清、completed重放/重启不复活授权。
- 真实鉴权撤销/过期、Origin/no-store、HTTP断开后已接纳仍执行、cancel/claim/complete竞争、请求排空后收尾、根/DB/端口释放及立即同根重启。临时行为测试执行后精确清源，保留可复用稳定集成脚本及原失败证据。
- 已接纳后撤销原会话仍正确执行；HTTP较早queued响应不覆盖新running/句柄/seal；API毫秒与manifest纳秒原始摘要分离；E=expires精确清key、最终失败确认回滚H、busy/重启不续排队原截止；pending双名缺与retained缺失状态分开；未变化的超额授权多个idle tick不重复hash；注册后所有退出路径的drained信号与共享收尾预算均覆盖。
- 清源后fresh同一Windows产物跑新手动执行专项及storage/旧backup/account离线/core大文件回归；gofmt/vet、包编译、Python AST/标注、Vue类型检查及Linux amd64/arm64交编。源码与产物SHA、原始秘密扫描、精确自有活进程/临时源0记账；各失败/重复轮次不累加。
- Windows实际运行、Linux交编、真实Linux/NAS、系统崩溃及物理断电分列。自有进程中断不是系统崩溃或断电；5GiB稀疏Range不是大库/媒体性能证据。
- 实施者明确停写后独立可写检查，主线核原始证据后更新正式spec/docs/README及任务状态；未完成门禁不能提前标本片完成。

## 7. 禁止与正确做法

| 禁止 | 正确 |
| --- | --- |
| 根不ready时创建run/key后再准备；GET建根 | 合法POST只在高水位提交确认后唤醒准备，503沿原key重试，GET只读 |
| 业务拒绝直接return error使高水位回滚 | 保存Outcome，提交确认后发送固定拒绝 |
| manager.mu包BeginTx/SQL/Commit/文件I/O | 单连接先取得事务，mu只保护短暂生命周期/精确句柄 |
| publisher error当作肯定未发布；重试换ID/VACUUM | 同时消费Certainty，unknown保留原ID无source对账 |
| 读到完整final就补completed；所有retained启动回环 | 仅原unknown意图补规定OS协议，Windows先final_seen提交确认；retained启动只读 |
| 不发授权或无条件清授权以继续生成 | 成功原子发授权，真实阈值/guard/token满足才无删除结束，否则可见阻塞 |
| 每30秒hash所有历史库；worker先退出后handler仍提交queued | 轻量周期检查，必要时完整核验；请求排空信号后同executor做有界DB收尾 |

本契约冻结及激活只授权上述范围的实施，不产生产品门禁通过数；自查停写后仍须独立可写检查、主线证据读回和正式规范同步。
