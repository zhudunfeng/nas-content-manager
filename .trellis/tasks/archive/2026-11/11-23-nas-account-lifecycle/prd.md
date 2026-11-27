# S02 管理员改密、会话与离线恢复

## Goal

在单管理员模型内完成安全可恢复的凭据生命周期，用户不需手工改数据库。状态：本机完成，S02.1/2/3均已实施且独立复核完成，真实Linux/NAS验收单独待验；相对工作量：中。

## 依赖与入口

- 硬前置：S01本机基线；与S03主体独立。事件生产者待S04基础就绪后接入，不阻塞账号核心能力。
- 现有入口：`internal/app/auth.go`、`store.go`、`instance*.go`、`server.go`、`cmd/nas/main.go`；`web/src/App.vue`、`api.ts`、`password.ts`、`components/PasswordInput.vue`。
- 拟新增账号管理局部UI与离线维护入口，复用原数据目录OS锁；不调用会启动任务恢复/缓存清理的服务初始化来执行离线改密。
- [总路线](../11-23-nas-evolution-plan/roadmap.md)、[拟议契约](../11-23-nas-evolution-plan/info.md)、[可靠性研究](../11-23-nas-evolution-plan/research/reliability-implementation.md)。

## 范围与拟议契约

- 拟议`POST /api/account/password`：currentPassword按旧bcrypt兼容校验，newPassword复用现有10–72 ASCII四类规则，不trim；成功原子改hash并撤销全部旧会话，清cookie，提示重新登录。
- 拟议会话分页查询、撤销指定会话、再认证后revoke-others；不返回token/hash/IP原文或未处理UA。公开随机session_id不是bearer凭据，旧记录创建时间允许null。
- 增量auth_revision/会话元数据；登录在bcrypt比较后建会话时再次核对账号修订/密码快照，防止改密竞态下旧密码补建新会话。
- 拟议`nas account reset-password --data-dir <目录>`：先获取相同canonical目录独占锁；已有DB和受支持schema/管理员才执行。交互无回显确认两次；管道需显式password-stdin，不接受argv/环境密码。
- 撤销保证提交后的新请求不再鉴权旧会话；已准入媒体传输可能完成。敏感操作提交时重新核验会话/修订，不破坏原流媒体生命周期。

## 小 PR 顺序

1. **S02.1 存储与竞态防线**：增量迁移、旧会话编号、auth_revision和登录CAS，保持老密码/会话兼容。
2. **S02.2 API与账号安全UI**：改密、会话列表/撤销、再认证限流、错误/空态、清除敏感表单；复用PasswordInput和401处理。
3. **S02.3 离线CLI和手册**：明确子命令分派、受限离线存储、OS独占、事务改密与脱敏中文日志；Windows/Linux操作与失败路径验证。

## Acceptance Criteria

- [x] 正确改密后旧密码/全部旧Cookie失效、新密码可登录；旧弱/中文密码可作为原密码升级。
- [x] 9/10/72/73边界、缺类别、32个ASCII标点、中文/全角/空白/控制/emoji输入前后端一致；无trim/截断，拒绝时库不变。
- [x] 并发登录与改密、双改密、撤销与敏感写操作的竞态有断言；旧验证线程不能补建有效会话。
- [x] 当前/其他/过期/未知会话行为明确；重复撤销幂等，不暴露token/hash，分页有界。
- [x] 匿名/跨站/超大/未知字段/猜测分别拒绝或限流；密码/令牌不进日志、DTO、localStorage和命令历史（原始输出检查边界见复核报告）。
- [x] 离线命令在Windows隔离实例验证活实例/目录别名/缺库/新schema/权限不足/确认失败/Ctrl+C零破坏；不杀占用者、不建空账号、不扫描媒体。
- [ ] Windows与Linux数据锁语义留有独立结果；成功离线重置后所有旧会话失效。
- [x] 账号/媒体Range/重启集成回归和API/spec/手册完成，临时单测执行后删除。

## 当前切片进展（2026-11-23）

- S02.1已验收，历史证据保留于`research/quality-review.md`。
- S02.2已验收，独立账号75项、核心52项、浏览器138项（122真实/14受控/2资产）通过；临时Go六组与生产TS54项通过并清源。复核修复局部空密码框掩码问题，报告为`research/quality-review-s02-2.md`与`research/browser-review.md`。
- 统一baseline新增account组合，实际75项通过；编排自检最终41项通过，原失败与心跳诊断保留，不重复汇总账号计数。
- S02.3完成本机实施及独立复核，最终同产物账号157项（在线83+离线74）、核心52项、主线baseline account157项通过；重复执行不叠加计数。独立复核修复损坏bcrypt摘要预检、已知超大正文真实413、symlink尝试证据与ACL完整恢复核对，报告为`research/quality-review-s02-3.md`。
- 真实Windows Console 11场景、实际ACL拒绝与恢复通过；文件symlink实际遇WinError1314，明确记录Junction替代边界。三批九组独立临时Go行为测试通过并清源。最终编排自检44项及72负例/变异检查见`research/baseline-account-integration.md`。
- 真实Linux/NAS锁、终端、权限及目标环境仍须单独验收；Linux双架构仅交叉构建。状态`local_complete_target_pending`不代表目标发布门禁通过。事件生产者由后续S04联调。

## 失败与回退

事务前失败保持原状态，提交后密码不自动回滚为旧密码。代码降级用匹配schema备份；恢复整库后需明确再次撤销会话，不能静默复活旧设备。锁冲突要求正常停服，不提供强制破锁。

## Out of Scope / Definition of Done

不做多用户、自助注册、匿名Web找回、SSO、强杀已准入媒体请求。按父路线统一门禁记录实际结果；事件接入作为S04联调切片。
