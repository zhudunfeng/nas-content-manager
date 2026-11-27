# S02 实施契约

2026-11-23。以实施前 schema 4、单管理员模型为基准；本文件冻结本步骤实现边界。S02.1/2/3已在schema5上完成本机实施与独立复核，真实Linux/NAS仍待验。开始前读取 research/current-implementation.md 和父任务可靠性研究。实际结果由 implementation.md 与research内各切片quality-review记录，未实现不能标成已通过。

## S02.1：增量存储和并发凭据防线

- 使用下一增量版本 schema 5，不改 schemaV1–V4。用户增加非负 auth_revision；会话增加独立公开随机 session_id、nullable created_at、有限客户端类别及绑定的 auth_revision。老用户/hash/未到期会话原样保留，旧创建时间为未知，旧客户端为 unknown。
- 新 session_id 与 bearer token 独立随机生成；公开 ID 绝不能由 token/hash 推导。列表/日志不暴露 token、hash、IP 或原始 UA。
- setup/login 在 bcrypt 后使用原用户 ID/hash/revision 快照条件创建会话；凭据已变更则不设置有效 cookie，不允许旧验证线程补建新会话。sessionUser 校验会话存在、到期与当前修订。
- 旧expires_at为可变小数位RFC3339Nano，不可用字符串比较判到期；认证与敏感事务统一按解析后的time.Time精确比较。过期清理也不能误删同秒未到期行，须有同秒/精确到期断言。
- 将共享快照/CAS/撤销原语保持为账号域小函数，事务不跨 bcrypt、终端输入或媒体处理。所有新方法/字段按项目要求中文注释；关键动作中文脱敏日志。
- 同步既有 verifier 的精确 schema 断言。外部迁移夹具必须构造真正旧库；不能在最新库只改 user_version 假装旧版本，不能删除保留性断言。

## S02.2：接口与局部账号安全 UI

- `POST /api/account/password`：严格 JSON `{currentPassword,newPassword}`。原密码兼容旧弱/中文值，不 trim；新密码复用10–72 ASCII四类规则。成功事务更新hash/推进revision/删全部会话，清Cookie，返回需要重新登录的明确结果。
- `GET /api/account/sessions`：有界分页，当前标识来自实际会话；公开ID、安全浏览器/系统类别、创建时间（可null）和过期时间。具体分页字段/稳定排序在实现文档和API同时固定，不用列表第一项猜当前。
- `DELETE /api/account/sessions/{id}`：校验ID格式及归属，重复/未知撤销幂等；当前撤销清Cookie并退出。`POST /api/account/sessions/revoke-others` 以严格JSON `{currentPassword}` 再认证，只保留当前会话。
- 账号写事务提交前再次校验会话仍有效、账号hash/revision未变；登录CAS、双改密、撤销与敏感写用确定性临时Go测试制造竞态。已准入媒体流可完成，提交后的新请求不能继续使用旧会话。
- 复用同源写保护和64KiB上限，账号JSON额外拒绝重复字段、未知字段、尾随对象；密码错误不误触全局401退出，实际会话失效用401，冲突用409，猜测用429及Retry-After。再认证限流需同时约束账号，不能通过换会话绕过。
- JSON字段名精确匹配camelCase，不能让encoding/json默认的大小写别名绕过重复/未知字段拒绝；原密码保留登录的72字节上限，但不适用新字符/复杂度规则。
- 在现有设置页加入局部AccountSecurity组件，复用PasswordInput、password.ts、api与session-expired。表单忙碌/错误/成功/空态、分页和当前撤销明确；密码提交后/离开时清除，不进localStorage或日志。键盘与窄屏/明暗沿用现有规范，不重设计导航。

### S02.2 补充冻结（已完成本机实施与独立复核）

- 改密成功精确响应`{requiresLogin:true}`；单撤销`{requiresLogin:boolean}`；撤销其它`{requiresLogin:false,revokedCount:number}`。revokedCount取本事务实际删除的其它会话记录数（含已过期/旧修订），不能称活跃设备数。
- 会话列表使用`limit`默认20、1..100；cursor省略或精确`v1.<32位小写hex>`。拒绝未知、重复、空的query值与非法参数。DTO为`{currentSession:AccountSession,items:AccountSession[],nextCursor:string|null}`；AccountSession为`{id,current,clientBrowser,clientOS,createdAt,expiresAt}`。当前会话来自实际Cookie且单独固定展示，不猜列表首项或UA。
- 原始行按`session_id ASC` keyset排序，只查`limit+1`行并处理前limit行；最后一行只判lookahead。Go用同一asOf精确解析时间及检查修订，跳过当前/过期/无效行，不无限补页。存在lookahead时cursor是最后消费的原始行ID，否则null。空items仍可能有nextCursor，UI必须保留手动“继续查看”；不自动追页，不声称最近登录排序或全局快照。
- 敏感事务先比较expected用户ID/hash/revision，变更409；快照未变但当前会话被撤销/到期401。bcrypt和新hash计算均在事务外；提交前重新检查上下文取消和实际会话有效期，失败整笔回滚。自身撤销/改密删会话后不再要求该行存在，使用本事务写前已核验的会话到期信息。
- 改密与撤销其它共享有界进程内再认证预算：同账号每5分钟12次、实际会话每5分钟6次，成功/失败bcrypt尝试均预占；换会话或成功登录不清空账号预算。429带ceil剩余秒数Retry-After（1..300）；清过期项，总键最多2048，满时保守拒绝而不逐出有效预算。不声称跨重启持久限流。
- UI使用独立“修改密码”“退出其他会话”表单；current/new/confirm/revoke输入ID依次为`account-current-password`、`account-new-password`、`account-confirm-password`、`account-revoke-password`，新密码提示关联`account-new-password-hint`。当前会话撤销用单个DialogShell明确退出后果，Esc/焦点恢复；改密或当前撤销成功复用原会话清理，网络结果未知时不自动重发。

## S02.3：停机离线重置

```text
nas account reset-password --data-dir <已有数据目录> [--password-stdin]
```

- main在LoadConfig/网络初始化前分派维护命令；不要求媒体根、FFmpeg或网页，未知参数明确失败。不接受argv/环境密码，不打印密码。
- 仅已有目录/普通library.db；先校验再获取现有canonical数据目录OS独占锁。持锁直到DB关闭，不调用openDatabase，不创建空账号，不恢复jobs/外部动作，不扫媒体/清缓存。
- 离线只接受显式支持的schema（首版本为5）；旧库需用对应应用正常迁移后停机执行，新库拒绝。不在重置工具里隐式迁移。
- 交互无回显、两次确认；显式password-stdin有界读取单个新密码。输入校验与在线完全一致，取消/错误/不匹配无写；事务提交前检查取消。可复用官方终端库并固定兼容依赖，避免手写不完整终端模式切换。
- 成功只更新现有管理员hash、推进修订、删会话。活实例/目录别名/缺库/未来schema/不支持平台/权限或锁失败不修改账号；不强制破锁、不杀进程。日志只含动作结果与安全标识。

### S02.3 接口冻结（本机独立验收完成）

- 无参数才启动原服务；只要存在参数，统一进入维护解析，未知命令不回退启动。精确支持上述空格分隔的`--data-dir <值>`和可选`--password-stdin`，不支持`--data-dir=<值>`、重复选项、未知位置参数或密码参数。仅`nas account reset-password --help`输出固定帮助并零资源初始化；未知值不回显。
- 退出码0表示成功提交并完成清理（或只读帮助成功），2表示用法错误，1表示前置/输入/存储/恢复或清理失败，130仅表示提交前的纯取消。提交结果未知必须明确提示核对且不自动重试；已提交但清理/结果输出失败不能宣称账号未变。
- 默认stdin必须是真实终端；`--password-stdin`必须为重定向输入，真实终端加此选项拒绝。标准输入最多读取75字节含溢出哨兵，少于上限仍必须等EOF证明完整；只剥一个最终LF或CRLF，拒绝剩余换行，不trim或截断。
- 同一次终端所有权覆盖两次无回显提示；同步MakeRaw后才能启动读取协程，唯一Restore结束后才允许bcrypt与事务。每次提示最多1024个原始非行结束输入字节，非法/超限粘性失败；确认不符、Ctrl+C/Ctrl+D、系统取消、提示失败或恢复失败均不得写库。Windows按键事件和系统CTRL_C_EVENT须分别在两提示实测，不能以管道EOF代替。
- 离线schema5支持是显式维护许可，独立于正常启动的当前schema常量。读取实际结构版本及受限账号结构、唯一ID1账号快照；事务同时核对结构版本、hash和revision，拒绝修订溢出及外来触发器/级联副作用。后续schema升级须同片审查并验证维护支持，不能仅改共享常量自动放行。

## 验证与完成

### 独立复核暴露的账号大正文响应边界（已修复并完成本机验收）

- 已保留同产物真实复现：已知超64KiB的账号JSON请求在Windows、`Connection: close`下可能发生RST，不能把TCP错误当413通过，或只重复到一次成功。原共享小体defer及大体skip仍保持其适用范围。
- 批准最小账号局部修复：仅已知`ContentLength>64KiB`且无Expect的拒绝分支，在写413前以250ms局部读取deadline经`MaxBytesReader`最多消费64KiB+1实际字节，不留存、不解析，也不扩大JSON接纳上限。实际超限由标准库limit-hit路径收尾，不以无界drain或媒体全局超时换取响应。
- 成功或预期超限后恢复deadline；超时/异常保持已到期deadline并明确关闭连接，deadline能力不支持时不开始可能阻塞的消费。Expect仍零读取，未知长度仍走原受限解析路径；声明巨大或发送停滞不能突破同一字节/时间预算。标准库固定半关闭等待不是任意网络条件下绝对送达保证。
- 独立验证区分完整超限体、分片/停滞、巨大声明、Expect、未知长度和异常EOF；真实close-connection重复请求不得吞错或自动重试。最终fresh同产物账号157（83+74）、core52与主线baseline account157均通过，100次关闭连接请求逐次413且health200；证据见`research/quality-review-s02-3.md`与`docs/verification.md`。这不是任意网络送达保证。

### 综合门禁

- 稳定HTTP/CLI断言保留在可复用集成脚本；本次临时Go测试执行后删除。真实浏览器检查账号页状态、键盘、375/768/1024/1440与明暗；隔离实际后端，不操作用户实例。
- 验证v4→v5旧会话兼容、重启、改密/撤销、旧弱/中文升级、密码边界/标点全量、严格JSON/同源/限流、离线成功及拒绝路径、秘密不进输出。
- gofmt/vet/包编译/构建、web typecheck/build、核心媒体Range与迁移领域回归。Windows实际执行与Linux交叉构建/实测必须分开；未提供Linux/NAS时保留待验，不能虚报实机通过。
- S04将来接入事件生产者；本步骤只提供正确结果与日志，不提前实现事件中心。提交/部署仍由用户决定。
