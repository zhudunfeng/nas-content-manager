# S03.2.3 执行器草稿冻结前接缝复核

2026-11-23。状态：**独立研究，未冻结、未激活、未实施**。本轮只新增本文；原研究、草稿、产品、测试、正式spec/docs、任务/context及共享CodeGraph索引均不写入。S03.2.2实施者仍拥有当前产品/回归，不能据本文要求其提前实施S03.2.3。

## 1. 复核基线与结论

已逐段完整读取以下三份文件；不依赖截断输出推断遗漏内容：

| 基线 | 行 / 字节 | SHA256 |
| --- | --- | --- |
| `executor-manual-contract.md` 草稿 | 111 / 17,468 | `45da2e34f4e5f0246574dc4825cefb6683cdc2547498375bda41dbfc20aa48da` |
| `research/executor-manual-contract-review.md` 停写研究 | 169 / 33,779 | `59445a34413f8633d674ece3a0b5cd74353899a0bf03800252697bc08b2506b3` |
| `root-publisher-contract.md` 当前冻结契约 | 91 / 17,773 | `b4c3f3aab47babd9851721ba2eab79376a92b7cc1e2c8248105e6345bebd93e4` |

同时重新载入`.current-task`、PRD/info/check清单，并针对状态语义和时钟拒绝核对`storage-schema6-contract.md`与`persistent-clock-root-seams.md`。本轮是契约对照，不导航并发产品代码、不重建CodeGraph，不跑构建、lint、类型检查、测试、fixture或用户实例。

草稿已完整承接五路由、UUIDv7窗口、高水位拒绝提交、三类容量、source前峰值、唯一根lease、发布确定性、成功原子授权、final_seen及停服排空。**推荐保留现有七节结构，只合入下述一组最小修订后再冻结。**其中§2.1–2.3消除可导致错误实现的歧义；§2.4–2.6固定剩余边界。不需要新表/迁移、第二连接、调度/下载/轮转、通用修复框架或长期hook；其余已有细节按附录执行，不另提替代架构。

## 2. 单一最小修订集

### 2.1 将HTTP重新鉴权与后台事务分开，补齐输入/时间格式边界

位置：草稿§2第24、34行及§3第45行。第24行“所有写事务内重新校验账号”若按字面套到executor，会把已接纳运行的领取、完成、重启对账或维护绑定到原HTTP会话；账号改密/会话过期后，正确发布也可能无法落completed，与“已接纳执行不依赖HTTP”冲突。

最小替换建议：

> HTTP手动准入与取消事务必须在同一事务内按真实actor重新校验账号修订/会话，并在提交前按实际认证时间复核有效期。后台领取、完成、对账、授权结束及维护没有HTTP actor，只受应用生命周期、原意图/CAS与各自持久约束；不要求原会话继续存续，不重新执行已接纳意图的用户授权。新POST `/api/backups`同样无query；全体输入校验按解码后的精确字段和值执行，拒绝非法UTF-8/NUL和多余内容。

另在API时间句后加一句：

> API时间显示为UTC RFC3339毫秒；manifest原始createdAt继续按冻结根契约使用规范RFC3339Nano，恢复不重编码manifest，不以API格式重写其原始bytes/hash或降低内部时间精度。

这是界面表示与持久字节的区分，不修改S03.2.2格式。未知/错类型/缺requestId、空对象取消、转义重复键、尾随与64KiB等已有规则继续有效，不新增通用decoder重构。

### 2.2 mu外Commit后的缓存发布采用单写者规则

位置：草稿§4第54行。现有锁序避免单连接死锁，但“Commit→短mu发布确认缓存”未限定缓存所有者：POST提交queued后释放连接，executor可能已提交running并安装句柄；较早POST随后发布旧queued快照，若整体替换manager状态，就可能覆盖新句柄/根阻断甚至closing。数据库串行不等于mu外缓存发布也按提交顺序。

最小追加建议：

> 根/usage/对账缓存仅由唯一executor写入；HTTP事务确认后只合并wake并返回本次已确认响应，不回写该缓存、不安装/清空运行句柄。运行、摘要、策略、H等持久展示事实由只读DB查询取得，不维护另一份可倒退的HTTP写缓存。closing/seal为独立单向状态，不可被任何较早快照重置。活动句柄仅在claim提交前安装，确认失败/终结后按原run及同对象比较移除；SQL/Commit始终在mu外。

此选择不增加epoch字段/全局锁/第二连接；无需为每次HTTP再查根。202返回当次已确认queued而随后GET已running是正常进展，不靠更新共享缓存强求两者瞬时一致。Commit未知仍阻新意图、读原ID；不得把“移除同对象”理解为结果未知时释放执行占位。

### 2.3 拆开“原pending两名均缺”与“已完成文件缺失”

位置：草稿§5第76行。“健康根两名均缺才补missing事实”较附录第9节过度压缩，可能使原pending一直占ownership或把未发布文件套入已完成恢复点语义。

最小替换建议：

> 只有根整体健康、身份一致且确切pending/final两名均不存在时，原running/pending意图才在同事务以interrupted终结（durable用户取消优先，否则process_interrupted）、key终态摘要并将file置deleted、填deleted_at；不重新生成source。已completed/retained的最终成员确实缺失，则只把file置missing，保留原完整发布tuple及历史completed，重选实际有效guard。根整体失联不执行任一种缺失补记。

该规则与已冻结DDL及原附录一致，不新增删除动作。对于仍存在的目录，继续先认owner/原始摘要/精确成员；owner缺失但非空保留，空未发布pending只按既有窄Remove，final_seen的不完整候选保留。两名并存/异物/错owner不猜选，不因有final_seen跳过校验。

### 2.4 固定Outcome异常与精确过期/排队边界

位置：草稿§3第41–47行，§4第55行。核心时钟算法正确，无需另起时钟事务；只须防止“closing也作为业务Outcome”与“提交前closing失败回滚”被同时实现，以及将“过expires”误写为严格大于。

建议在§3末补一段：

> 高水位提交规则仅适用于已正常鉴权、ctx与最终closing检查仍通过的手动准入/维护事务。最终认证/ctx/closing检查失败且rollback确认时，H和run/key修改一并回滚，只返回原认证/取消/停服事实，不发送先算出的410/409/503；不能为发该候选拒绝再开第二时钟事务。Commit已确认成功后再取消，不撤销H/run；真正未知才result_unknown。业务拒绝不裁剪；接受和维护的终态key清理边界精确为terminal=1且E≥expires_at_ms。固定一次E用于本次窗口与裁剪，真实认证时间的复采不改变E。

同时明确queued截止：原`admission_deadline_ms=N+1,800,000`不续期，领取/每次busy退回前按本次`max(已提交H,合法实际N)≥deadline`或同进程单调30分钟预算先到者判queue_timeout；子执行context另限30分钟。重启只恢复原持久截止，不能重新发完整排队窗口；大幅回拨不作为已接纳运行的新准入拒绝。遇key terminal标志/run id/seq/state不一致时阻新意图并读回核对，不能伪重放或创建另一run。以上仅明确现有有限时钟语义，不扩张修复/维护入口。

### 2.5 阻塞授权也不得借重试恢复成每30秒全量hash

位置：草稿§4第61–63行。新增的空闲轻量原则正确，但若每tick见授权非NULL就执行“授权结束的必要严格校验”，永久超额/unknown仍会每30秒hash全部历史库，实际绕过该限制。

最小追加建议：

> 授权结束检查由启动对账、新成功、原token/策略/已登记文件事实变化、或轻量核验发现根/成员身份及计量变化触发；使用同一executor的内存dirty事实合并，不新增timer/goroutine。先做有界状态/成员/实际计量，若已有证据显示超额、deleting/pending/unknown或计量不完整，保留token并直接blocked，不为证明“仍不能结束”反复hash。未变化的阻塞token在后续普通tick只做轻量失效检测；条件可能满足才做本次结束所需的严格guard/相关文件校验并CAS。I/O失败有界退避；不能内循环紧密重算、不能因缓存曾有效而跳过实际使用/结束CAS前的根与guard必要复核。

数量/字节都按各分量max；未知不当0，原三字段完整token/policy修订/guard仍参与CAS，三字段不能省发/覆盖。第8份成功后阻下一份及唯一guard超预算例外延期保持。200/64/2048不扩容、不逐出有效key/ownership；lightweight不等于永久信任缓存，也不自动认领deleting。

### 2.6 final_seen和停服只补失败边界，不扩大修复能力

位置：草稿§5第78、82行。现有方向与原附录/根协议一致；建议把以下两句写入正文，避免实现时只看压缩段落：

> final_seen是现有backup_files.error_code允许的内部安全值，不是run.error_code、HTTP输入、已确认完成或已授予轮转权限。CAS必须匹配原store/run/file/seq、running/pending及owner事实，提交未知先读原ID；在确认该标记前不做final→pending。只有完整、原意图的已标记候选用a.ctx派生的有限恢复context补OS确认，用户取消不能将它当普通未发布pending清理；停服/超时可中断并保留。普通未标记pending仍先尊重durable取消，retained启动只读。

> requests-drained通过一次性关闭信号（不是可能阻塞的无缓冲send）交给同一executor；注册executor后每条退出路径都须在停止请求并排空handler之后保证发出该信号，不能让executor等待永不发生的事件。seal先行、不等DB；单个独立≤5秒DB收尾预算在排空后用于所有可明确终结/已confirmed的事务，不能每个run重新续5秒。收尾失败/未知保留原running/pending及cleanupErr，最终仍关闭根并等待executor，DB/DataDir lease最后释放。

不要求shutdown开始后补新的大库hash、完整发布回环或文件删除；底层Close的实际耗时不纳入“5秒全停服”承诺。单连接下HTTP取消可能等鉴权/SQL，不能以绕过鉴权、先释放根或强杀用户进程弥补。

## 3. 冻结前最小核对输入

这不是新增测试实现或本轮通过声明；后续激活后并入草稿既有门禁即可：

- 接纳后会话撤销/过期，后台仍按原意图正确完成或取消；新的POST/cancel真实重新鉴权，原HTTP断开不撤销接纳。manifest保留纳秒规范原始bytes，API仅显示毫秒，不改变原hash。
- 提交queued后停在HTTP缓存发布之前，executor先running/装句柄或shutdown先seal；恢复HTTP后不得覆盖较新状态、句柄或seal。mu不跨单连接等待/Commit。
- 健康根的原pending两名缺失→deleted/interrupted，与retained缺失→missing/completed分开；missing不用于清掉未知根，已标记不完整pending仍保留。
- 410/409/503候选遇最终会话/ctx/closing失败，确认rollback只返回原失败；E=expires精确可清终态key，active过窗仍同run；排队原截止及单调预算不被busy/重启续期。
- 已超额授权重复多个idle tick，根/成员轻量核验有界且不重复hash历史DB；事实变化后触发必要严格核验，原token/guard/policy漂移仍不能误清。
- final_seen CAS未知、final→pending之间退出/取消、普通pending取消、两名同存及root异常均保留正确事实；requests-drained在正常/服务提前返回路径只关闭一次，无新goroutine，排空后共用DB收尾预算。

## 4. 本轮交回

本轮未修产品代码，也未直接改主线草稿；上述为一组明确的最小修订建议。未发现需要改变冻结S03.2.2根/发布协议、重做schema6存储或提前引入S03.2.4能力的阻断。草稿仍须等待S03.2.2独立交付关闭，再由主线合并建议、核实际接口并明确冻结/激活；本文本身不触发阶段切换。

Lint：未运行；TypeCheck：未运行；Tests/fixture：未运行（均为研究范围限制，不填pass）。没有产品、临时测试、原研究、spec/docs/task/context或共享索引写入；只交回本文供主线审阅。
