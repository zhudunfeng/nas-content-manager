# S03 实施契约

2026-11-23。整体范围沿用 `prd.md` 与父路线；本文件冻结各激活切片的实施边界。S03.1及S03.2.1已完成实施、独立可写复核与主线最终证据核对，按local_complete_target_pending记账；当前实施S03.2.2，真实Linux/NAS门禁仍待验。下文S03.1保留当时边界，当前头6以S03.2.1及正式spec为准。S03整体仍实施中，后续切片开始前另行冻结，不能把研究写成已交付功能。

## S03.1 共用快照服务与只读校验

### 兼容与范围

- 仅抽取现有同步快照路径并在下载前校验；不新增路由、迁移、配置项、前端、定时器、持久历史或 CLI。
- `GET /api/backup` 保留原鉴权、中间件、SQLite 单文件响应、文件名格式、Content-Type/Disposition 与 `http.ServeContent`。HEAD 保留现有生成行为，不在本片另定义跨请求稳定断点续传。
- 唯一 `App.backupMu` 覆盖生成、校验、下载、文件关闭和清理。竞争仍立即409；持有下载租约期间也算 busy。未来执行器复用同一服务和锁，不另写 VACUUM 路径。
- 保留原状态映射：主库/创建/生成/打开错误500；余量查询或预检不足507；新增校验失败固定中文500。客户端已经断开时不承诺响应码，头已发送后不追加 JSON。
- 快照仍为 `database-only`：DB本身含敏感索引、口令/会话哈希与私密引用，必须私密保管；不包含外部来源秘密文件、helper/worker身份文件、组件卷、下载载荷、媒体、用户资产原件，也不代表异地灾备。

### 内部边界与生命周期

- 共享生成函数返回私有快照租约，持有只读文件、实际 schema/长度/SHA-256/UTC生成完成时间、唯一临时文件所有权及幂等 Close。调用者不自行重新打开、rename或删除路径；暂不暴露 Keep/Detach。
- 在 canonical DataDir 内建立随机私有 `.backup-pending-*` 目录和固定候选文件。只删除本次确认归属的确切成员，最后删除空目录；禁止按前缀清整个数据目录或盲目 RemoveAll。替换/未知成员令清理失败并保留材料。
- 沿用 `MinFreeBytes + 2×源尺寸` 的准入预算，尺寸取主文件与连接实际页计数的保守值，算术先防负数与溢出。查询真实目标卷余量；预检不保证后续无 FULL/IOERR，不先删旧材料换空间。
- 源库只执行带 context 的一次 `VACUUM INTO ?`；不加业务长事务、不复制在线主库、不改线上 journal_mode。源库单连接下其它 SQL 可能等待，不能宣称真正并行写入。
- 候选完成后检查普通文件身份、设置受限权限、Sync/Close，再打开只读句柄。校验连接和行全部关闭后，同一只读文件句柄用于摘要与 HTTP。
- 失败自行清理并解锁；成功将唯一所有权交给租约。Close 无论成功失败都最终解锁，重复调用返回同一清理结果而不重复解锁。
- 请求上下文沿用原 workCtx；停服仍先取消/关闭连接、等待请求及worker，再关闭DB和数据锁。不新增脱离请求的后台协程或短全局媒体写超时。
- 头后关闭/删除失败须记录安全中文日志并进入资源收尾错误；不能把 handler 返回宣称为客户端完整下载。强杀/掉电可能遗留候选，本片不自动扫描删除或把它当有效持久备份。

### 独立只读校验

- 不调用 `openDatabase`、Run 或 LoadConfig，不迁移、恢复任务、撤销会话、清缓存或 checkpoint。
- 使用正确转义的 `file:` URI、`mode=ro&immutable=1`、单连接和固定连接完成检查。immutable 只用于已完成且隔离稳定的单文件候选；不能用来忽略在线库的 WAL。未来离线入口须先建立相同前提。
- 检查打开前后 Lstat/Stat/文件身份与普通文件类型；路径、句柄、长度、mtime在校验/摘要结束时再次一致。无 expected 时只生成摘要事实，不声称核对过可信manifest。
- 精确SQLite头、正长度、受检 `page_count×page_size` 等于文件长度；拒绝截断和尾随垃圾。显式 expected 时额外核对正长度及SHA-256。
- 实际读取 `user_version`，S03.1只接受当前头5；只读结构探针确认核心表及账号头版本字段，不读取账号秘密值。完整性必须为 `integrity_check(1)` 恰好一行ok，外键检查必须零行，Rows/Scan/Close错误均不得算成功。
- 校验连接关闭后以固定64KiB缓冲流式SHA-256，每轮检查取消并核对总长度；禁止整库读入内存。最后重置游标，再检查取消才交付下载。
- 正常启动与快照校验共享当前schema常量；历史迁移固定版本不变。S02.3离线维护的显式支持范围不能靠修改常量自动扩大。若S03.2引入新schema，须同片更新离线账号维护的显式支持集、结构探针及稳定验证，证明新增结构不改变账号写入前提，避免升级后恢复入口不可用。

### 可执行验收

- 临时Go行为测试覆盖租约barrier竞争、Close幂等/失败解锁、预算与溢出、生成/校验/摘要取消、Sync/Close/删除/身份替换故障、未知sentinel保留。测试执行后清源，保留安全结果日志。
- 只读检查的真实SQLite反例包括旧schema/未来schema/伪结构、坏页/截断/尾随、完整性可过但外键失败、错误expected大小/摘要。前后hash/size/mtime和目录成员不变，无sidecar；Windows ACL不能用chmod替代。
- 保留 `scripts/verify_backup.py --binary <产物> --root <全新根>`：复用既有OwnedProcess/新根/确切就绪，独立计算下载schema/长度/hash，真实慢HTTP下载时第二请求409，关闭首请求清理后可再生成；并发收藏/进度提交与快照保持事务一致。
- 自有实例正常停服时验证慢下载关闭、文件/DB/锁释放并立即重启；不得按名字杀进程或结束用户实例。原始内存秘密检查先于日志脱敏，失败证据不覆盖。
- Windows私有目录/文件须使用真实受限ACL，可新增`backup_permissions_windows.go`与`backup_permissions_unix.go`；不能以chmod代替DACL或从app反向import desktop形成循环。正常停服证明须来自自有隔离Console的CTRL_C_EVENT，或临时真实RunWithOptions上下文取消；OwnedProcess的Job强制回收仅作兜底，不可算正常停服。
- gofmt/vet、临时行为测试、清源包编译、fresh构建、core与受影响账号回归必须完成；记录本机产物摘要。Linux交叉编译、真实Linux/NAS、真实FULL/拔盘/ACL及大库性能分别记账。
- 实施者自查后停写，交独立checker可写复核；正式spec/API/verification与任务状态由主线同步。未提交、部署或真实环境操作。

## 后续切片边界

S03.2再增加受控专属备份根、持久策略/记录、原子发布、单执行器、对账与轮转；S03.3再加入设置UI、显式离线核验/恢复准备和inventory/资产支持；S03.4完成隔离恢复与证据。各片在开始前按最新代码与上片验证补充本文件，不把上述规划提前写成已实现。

## S03.1 本机验收交回

实施者报告为`implementation.md`，独立修复与最终报告为`research/quality-review-s03-1.md`。独立修复ReadDir有界枚举后，最终同一Windows产物SHA256 `49754b278cfb26c3725efbcf2f45d462fbda72617eb7442859f5a21e0b40e790`，21,551,616字节；备份38、账号157=83+74、核心52项分别通过，不累加实施者旧产物重复结果。临时Go最终8组/22子场景通过并清源；3次真实Console正常停服、0次应用强制回收、收尾错误为空。gofmt/vet/清源编译/typecheck及Linux两架构交叉构建通过，后者仅编译。详细路径和边界见独立报告及`docs/verification.md`；正式已实现契约见`.trellis/spec/backend/backup-contract.md`。

S03.2内部依次交付：**S03.2.1 v6存储与账号兼容 → S03.2.2受控根与平台发布 → S03.2.3执行器及手动去重/取消 → S03.2.4 UTC调度/下载与轮转**。完成相应门禁前不开放enabled=true；第一片只迁移和维护兼容，不新增持久目标生成或API。

## S03.2.1 已冻结：v6存储与账号兼容

本片权威为[`storage-schema6-contract.md`](storage-schema6-contract.md)。主线结合S03.1实际交付与独立接缝研究冻结五表字段/NULL/范围/单active/单deleting/授权三字段、真实v5→v6迁移、离线显式{5,6}与user_version CAS、当前头6快照探针和稳定隔离门禁。S03.1报告保留当时schema5历史事实；S03.2.1已独立验收并将正式snapshot/runtime/storage/account spec同步为当前头6，不改变历史证据。

独立接缝报告[`research/schema6-storage-contract-review.md`](research/schema6-storage-contract-review.md)已停写交回并由主线完整读回，SHA256为`516cb2797cfdc7f4faac8f9dc7d20156ca0f55f6b00a0452430c68a76427d178`。无阻断；其queued无file、completed有started、published非空则完整tuple三项建议均已纳入冻结稿。报告只记录设计核对，不计产品通过数。

范围只到存储和维护兼容，不增加备份根、配置/API、执行器、启用策略、裁剪或轮转；三类容量上限与跨表关系由后片服务负责，不加计数触发器假装已实现。pending/unverified允许未完成发布元数据，精确清理后可→deleted；deleting只用于已完成备份的轮转，必须完整发布tuple。历史v5离线账号支持与当前6快照checker是不同入口，后者仍仅当前头。

实施者已停写交回[`implementation-s03-2-1.md`](implementation-s03-2-1.md)，主线完整读回并核SHA256 `c2187314431e7bf4e2255b966774f7c11b8ab87c3de50592bc229cad22b610a2`。其自查storage17/backup38/account162/core52及6组202子场景仅作历史交接证据，不与独立最终轮次累加。

独立checker已修复隐藏生成列漏检并正式停写，报告[`research/quality-review-s03-2-1.md`](research/quality-review-s03-2-1.md)完整读回，SHA256 `0506957e6e2e1a461fc4e7362dbf40fda52c6fb9d27e80812f912b3e3211dded`。最终Windows产物21,581,824字节、SHA256 `18a0a77fe0c10256e7eb406d0b07f64210901b8320012a78f2b02fc810a188cd`，同产物storage17/backup38/account162=83+79/core52通过；Go8顶层/653通过叶子，658带斜线pass另含5容器。storage/backup共6次真实正常停服、0强制，core Job回收单列；精确进程0、临时源0、源码漂移0。主线独立重算事件和三平台/源码SHA，证据`.runtime/verify/s0321-mainline-closeout/evidence-readback.json`，actual UTC `2026-11-23T05:11:39.419590+00:00`。正式契约已同步`.trellis/spec/backend/backup-storage-contract.md`及相关runtime/snapshot/account/docs；本片核定local_complete_target_pending，真实Linux/NAS及故障介质门禁仍待验。失败历史、原始秘密扫描边界与默认随机源fatal限制见独立报告和docs/verification。

S03.2后续行为以新研究纠正旧草案：高水位的正常业务拒绝须先提交、未知提交结果独立处理；marker跨重启仅逻辑命名空间；轮转新成功授权/有限续跑/结束CAS清空，不因last-good重开。它们在本片只冻结必要存储字段，具体API/根/执行和平台门禁在对应后片开始前冻结，尚未实施。

## S03.2.2 已冻结并激活：受控根与平台发布

[`root-publisher-contract.md`](root-publisher-contract.md)已结合独立接缝报告冻结，现作为本片实施权威。报告[`research/root-publisher-contract-review.md`](research/root-publisher-contract-review.md)已停写并由主线分段完整读回；最终214行/38,725字节，SHA256 `2dc04d3b45100e0342838c07d71834d5a16d13d4948ae55076e94552b34670b6`。包含目录树/精确文件点保护、历史根257/256预算、缺省未创建路径判定、固定锁及marker/owner/manifest、平台无覆盖发布和三类确定性。研究未运行产品门禁，不计通过数；已实现存储继续遵循storage-schema6-contract和正式spec。实施者负责本片产品/验证/新交接报告，主线独占正式spec/docs/task；实施停写后再交独立可写检查，全部门禁前不开放后片能力。

## S03.2.3 冻结前研究（历史，当前见文末激活节）

独立建议[`research/executor-manual-contract-review.md`](research/executor-manual-contract-review.md)已停写交回，主线分段完整读回169行/33,779字节并核SHA256 `59445a34413f8633d674ece3a0b5cd74353899a0bf03800252697bc08b2506b3`。建议包括五个手动/只读路由、UUIDv7窗口与拒绝结果提交高水位、200/64/2048上限、唯一manager及mu外SQL/Commit、owner先登记、无source原ID对账、Windows未知final的持久事实与补确认、无删除型授权结束和请求排空后的停服收尾。它是下一片设计输入，未运行产品或fixture门禁，不计通过；须待S03.2.2独立验收后结合实际接口另行冻结。空闲周期检查应保持轻量，不能将有界唤醒解释为每30秒全量hash历史数据库。

主线曾将单一推荐整理为[`executor-manual-contract.md`](executor-manual-contract.md)草稿，并完整读回冻结前复核[`research/executor-manual-freeze-review.md`](research/executor-manual-freeze-review.md)，100行/12,087字节、SHA256 `74cf3f17a0c443e413b3398cb6f7539ddd8eeff69decb9f99925e14b817c31e2`。六组最小修订已并入：HTTP与后台认证边界、executor单写缓存、pending/retained缺失区分、时钟/排队精确边界、授权dirty轻量核验、final_seen和排空收尾。冻结前修订草稿121行/21,558字节，SHA256 `d247dc8b4ef8d55e21c052d5d2f5950185671b9e3a35f56a63c6c32778ccd2ff`；当时未冻结/激活、未加入实施上下文，active为S03.2.2/Phase2.2。该摘要仅保留草稿历史，当前冻结与激活以下节为准。

## S03.2.2 实施者停写，进入独立检查

主线已完整读回[`implementation-s03-2-2.md`](implementation-s03-2-2.md)，131行/17,561字节，SHA256 `2a1271b4e5d73af432449abb0f8b8c9484696132d084f542fd685c33f8f32870`；实施者明确停止产品/临时测试/报告写入。自查最终轮次为behavior-final3的18顶层/210叶子、另单列3个无子测试；fresh Windows SHA256 `aa285c1824d368b9410c5abc94e5c3514240b604d09a4765f2966d3b791e14d0`的storage17/backup38/account162/core52均通过。此处仅登记交接，不能替代独立检查或与其通过数累加；旧失败轮次和symlink夹具权限限制保留。当前切换Phase2.2，S03.2.3仍仅草稿/研究，不激活。

## S03.2.2 本机独立验收完成

checker已全面停写，最终报告[`research/quality-review-s03-2-2.md`](research/quality-review-s03-2-2.md)由主线分段完整读回，115行/15,581字节、SHA256 `4baa31ee219316e75e2290f12f37201d97ca0f81a66d090386d81b069835bbdb`。独立最小修复Windows设备名主干扩展前ASCII空格漏检与Linux Stat失败遗漏Close错误；Linux仅静态修复/交编。最终24顶层/257叶子/0嵌套容器、另5无子测试顶层单列；同一fresh Windows SHA256 `9839729e64f80e052f6e5acd3114e251f8fa5fbffad8276753d2a15148a43137`、21,593,600字节，storage17/backup38/account162=83+79/core52通过。storage/backup6次正常Console停服/0强制，core Job回收单列；原始red、夹具失败和所有旧轮保留，不重复计数。

主线独立证据`.runtime/verify/s0322-mainline-closeout/evidence-readback.json`实际UTC `2026-11-23T06:12:12.158591+00:00`：原始事件重新计数、193产品源零漂移、三产物摘要/长度一致、四回归同产物、4临时源已清且txt摘要一致、确切产物活进程0。193行产品源清单SHA256 `0dd6d62aa89118b7e40e6291b23f38d9ea95780eb220f01dac2471734c08d199`。CodeGraph已在checker停写后同步2修改文件/43节点。Phase3.1核验通过，Phase3.2调试复盘见[`research/root-publisher-debug-retrospective.md`](research/root-publisher-debug-retrospective.md)；Phase3.3已将根/发布及别名/ACL/句柄/确定性/预算边界同步正式spec、README、API和verification文档。本片记local_complete_target_pending；真实Linux/NAS、多卷/介质故障、进程/系统崩溃和物理断电仍待验，不关闭S03整体。下一片须按上文草稿明确冻结激活后才实施。

## S03.2.3 已冻结并激活：唯一执行器与手动意图

S03.2.2的Phase3.1原始证据读回和Phase3.3正式文档校验均通过；最终11份文档链接/摘要和193行源码清单一致性证据为`.runtime/verify/s0322-mainline-closeout/doc-readback-final.json`，actual UTC `2026-11-23T06:14:45.088313+00:00`。主线已对照CodeGraph的实际root/publisher/source/auth/lifecycle签名，将上述六组最小修订合入[`executor-manual-contract.md`](executor-manual-contract.md)并冻结；121行/21,876字节，SHA256 `38d99a58c43ae58821fc24fc0b349d85236a4731646bae5fdea38a504f3b28b6`。当前任务切换S03.2.3/Phase2.1，旧研究/草稿状态只作为交回时历史，不覆盖该冻结稿。

本片只交付五个手动/只读路由、UUIDv7有限窗口幂等/高水位、唯一manager/executor、source前峰值、取消/原ID无source对账、无删除型授权结束及请求排空后收尾；策略仍关闭，无新迁移/第二连接、策略启用/调度、持久下载/轮转或恢复CLI/UI。附录与冻结前报告、正式root规范及上片最终独立报告加入implement/check上下文。实施者独占产品/临时验证/稳定专项脚本和`implementation-s03-2-3.md`；主线独占正式spec/docs/task，实施者停写后再独立可写检查。冻结不记产品通过数，也不关闭S03。

## 本轮改为优先交付当前可用包（2026-11-23）

用户进一步要求“请快速在当前修改上形成一个可用版本”。主线收敛本轮交付为当前 S03.2.2 已验收基线的 Windows x64 开发包，未完成的 S03.2.3 不接入路由或生命周期。实施者仅有三份自有新源草稿已按原 SHA 保存 `.go.txt` 后精确移出编译路径；193 项既有产品源零漂移。交接报告为 [`research/s03-2-3-draft-checkpoint.md`](research/s03-2-3-draft-checkpoint.md)，26 行/3,419 字节，SHA256 `44d640541b56f6a2114b699778f6fa7a8e7439da7ef11a2eb16b147f8716ee06`，主线全文读回。后续实施先重新审阅草稿与当前冻结稿，不自动恢复未验证代码。

发布包为 `bin/releases/usable-20261123/qiku-0.1.0-windows-amd64-development.zip`，已解压到 `bin/usable-20261123/`。前端 typecheck/build 通过，现有构建器生成两份内嵌网页 EXE；开发包未捆绑 FFmpeg，不是完整离线发行包。同一 nas.exe 的 core52、backup38、account162（含离线）、storage17 通过；桌面 EXE 无托盘模式的嵌入网页/端口避让/真实 IPC/退出/原位覆盖后重启通过，exitSeconds 为 0.029/0.017。storage/backup 共6次正常 Console停服/0次应用强制回收，core Job回收单列；真实托盘/目标NAS/断电未计本轮通过。

主线读回 `.runtime/verify/usable-20261123-mainline/readback.json` 于 UTC `2026-11-23T06:31:29.992738+00:00` 为 passed，193源零漂移、269项同产物、包内与归档摘要一致、草稿编译源0、确切产物活进程0。轻量独立检查报告另列为 `research/quality-review-usable-20261123.md`；正式操作说明为 `docs/usable-preview.md`。本次产品净修改为零，trellis-update-spec 判断没有新的产品契约需写入正式 backend spec，保留已验收的四份备份规范；README/verification只记录可用构建及真实门禁边界。S03.2.3仍待继续，S03整体保持 in_progress。

轻量独立检查已通过并停写，报告全文读回，54行/5,857字节，SHA256 `1268d28b3f2045a11745755ddd8b68bf9fd368483cf446a66f4fc805ed069f19`。独立核验包与解压目录一致、193源零漂移、静态检查通过，本轮范围无阻断；本机可用包登记为 `completed_local`，目标设备验证仍待完成，不代表 S03.2.3 或 S03 整体完成。
