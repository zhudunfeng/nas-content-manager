# S03 计划备份与离线恢复检查

## Goal

把已有手动一致性数据库快照与恢复材料说明变成可调度、可校验、可演练的闭环，准确报告未包含的外部材料。状态：in_progress；S03.1、S03.2.1及S03.2.2本机独立验收完成，当前激活S03.2.3执行器与手动意图；相对工作量：大。

## 依赖与入口

- 硬前置：S01本机基线；不强依赖S02。S04事件联调后补，快照/调度主体不等待通知。
- 现有入口：`internal/app/media.go`、`store.go`、`config.go`、`server.go`、`instance*.go`、`cmd/nas/main.go`；设置页、`web/src/api.ts/types.ts`。
- 复用`docs/collection.md`、`docs/components.md`、`docs/windows.md`恢复材料；拟新增备份服务/离线检查模块与集成脚本。
- [总路线](../11-23-nas-evolution-plan/roadmap.md)、[拟议契约](../11-23-nas-evolution-plan/info.md)、[可靠性研究](../11-23-nas-evolution-plan/research/reliability-implementation.md)。

## 范围与拟议契约

- 在线仅主DB快照：沿用VACUUM INTO与现有backup互斥；包含校验/摘要和scope=database-only清单。兼容旧GET /api/backup，不并行第二套快照。
- 第一版一条每日UTC固定时间策略，默认关闭，UI显示本地换算；keepCount建议7、1–30，体积预算int64有界。NAS_BACKUP_DIR由宿主配置专属根，不接受Web任意路径，默认同盘不等于异地灾备。
- 拟议backup_policy/backup_runs、策略revision、手动requestId和schedule_key；queued/running/completed/failed/interrupted，短事务领取，网络/校验不占长事务。
- 文件/manifest与DB跨资源对账，确认完整性/外键/版本/大小/hash后发布；重启识别可验证自有产物，否则interrupted。新成功后才轮转，至少留最近一份有效快照，不先删旧救空间。
- 完整恢复inventory列明DB、私密来源、helper/nonce身份、worker状态、组件卷/配置、下载/媒体映射；不可访问材料标unverified/externalRequired，不声称Web已在线备份全部卷。
- 为持久用户资产提供通用登记/清单、受控文件采集与恢复支持，S03以合成原件和引用完成独立验证，不等待S06上传功能。S06.3负责真实人工海报原件端到端备份恢复后才开放上传；派生缓存可重建。
- 拟议backup verify/inspect-recovery默认只读，不迁移/启动/覆盖生产。完整恢复在新隔离目录/端口操作；恢复后通知默认关闭，未知外部写仍待核对。
- 恢复副本对外启动前，必须在同一数据目录独占锁内经独立、显式的离线恢复准备步骤撤销所有快照会话；若受支持schema含auth_revision则同步递增。该步骤不修改密码、不启动网络/任务或隐式迁移，不能藏进只读verify/inspect。工具不支持该快照版本时阻止启用并给匹配版本指引，不能恢复已撤销设备的登录能力。

## 小 PR 顺序

1. **S03.1 快照服务与校验**：抽取共用互斥/预算/临时发布、流式摘要、快照只读检查；旧下载接口保持。
2. **S03.2 持久策略与轮转**：迁移、单执行器、手动/计划去重、错过周期合并、重启对账、下载租约与受控轮转。
3. **S03.3 设置UI/离线检查/inventory**：真实运行状态、保留文件下载、材料角色/权限/身份检查、只读检查与独立离线恢复准备、恢复演练脚本；实现通用用户资产清单/受控采集/恢复，以合成原件验证。
4. **S03.4 隔离恢复与证据**：本机端到端恢复；目标宿主材料/锁/权限与组件恢复另轨补证。S03提供可消费的运行结果/日志并在此完成；备份失败生产者接入及联调归S04.4负责，不作为S03完成前置。

## Acceptance Criteria

- [ ] 并发更新收藏/进度时快照可打开且是某一一致性时点；手动/计划争用不并行，重复requestId返回原run。
- [ ] 错过十周期只补最近一次、时钟回拨/重复tick不重跑；停服等待执行器后再释放DB/锁。
- [ ] 空间不足、卸载/只读目录、链接替换、取消/退出、文件/清单已发布但DB未登记均有明确可恢复结果。
- [ ] 轮转只删精确登记的自有文件；最近好备份始终保留；活跃下载跳过，实际超限积压可见。
- [ ] 验证错误摘要、坏SQLite/外键、新schema、缺材料、错实例和无权限，不隐式恢复/迁移。
- [ ] 隔离恢复核对账号、库索引、收藏/标签/进度、私密引用及worker/helper身份；对外启动前全部快照旧Cookie失效且管理员可重新登录，未知外部动作不重发，通知不批量重投。
- [ ] 通用资产清单/受控文件采集/恢复以合成原件独立验证，丢失/摘要错/引用不匹配能检出；S03不等待S06完成。真实上传原件恢复断言与开放门禁由S06.3验收。
- [ ] 真实NAS/多卷/组件未测试项保持待验，不把fixture或DB快照宣传成整栈备份；文档/spec同步。

## 当前准备进展（2026-11-23）

- S03.1已完成实施、独立可写复核及主线证据读回；设计依据为`research/snapshot-service-design.md`，最终报告为`research/quality-review-s03-1.md`，当前已实现契约为`.trellis/spec/backend/backup-contract.md`。修复目录无界枚举后，同一最终Windows SHA49754b…e790的备份38/账号157/核心52项通过；临时Go8组/22子场景清源，3次正常停服、0强制回收。完整证据见info及docs/verification，不与历史自查重复累加。
- S03.1记为local_complete_target_pending；S03整体尚未完成。S03.2按存储兼容、根/发布、执行器/手动意图、调度/下载轮转四个内部切片推进，冻结契约以info新增节为准，不提前开放未完成的enabled=true。
- S03.2持久策略、跨资源对账和Windows平台发布研究已落盘于`research/persistent-backup-design.md`、`research/platform-backup-publish.md`，主线冻结接缝核对见`research/persistent-freeze-review.md`。它们仍是设计输入，API/迁移/平台支持矩阵尚未冻结，不能算实现或测试通过；不扩张S03.1范围。
- 自有Windows/NTFS平台原语fixture已完成4场景19项断言，并由主线读回JSON、产物摘要及临时源码/fixture清理事实；报告为`research/platform-publish-fixture.md`。它不是产品publisher、崩溃或断电验证，不扩张S03.1范围。
- S03.2时钟拒绝提交和根身份接缝已补充研究于`research/persistent-clock-root-seams.md`，主线已完整读回；正式冻结时须明确业务拒绝提交高水位、事务未知分类，以及marker跨重启仅为逻辑命名空间的边界。当前仍是设计输入，没有新增迁移或产品通过声明。
- S03.2轮转授权研究`research/persistent-retention-authorization.md`已完整读回并核SHA，采用五表内三字段授权和单deleting约束；新删除按授权与当前阈值各分量max，变严编辑不扩大旧授权，结束CAS清空，不从last-good重开。此取舍替代旧研究的无限“直接最新策略”；唯一guard超预算下限与更小替代规则须进入后片门禁。本片研究不计产品通过数。
- S03.2.1已按`info.md`/`storage-schema6-contract.md`完成本机独立验收：交付v6五表及账号/当前快照兼容、真实旧结构夹具和隔离门禁；策略仍关闭，正常启动不建持久根，执行器等后片未开放。
- 独立研究`research/schema6-storage-contract-review.md`已停写交回、完整读回并核SHA，无阻断；NULL/状态组合、真实历史夹具与版本CAS建议已纳入冻结稿。它是设计依据，不是迁移或产品验收。
- S03.2.1实施者自查及独立可写检查均已停写交回，主线完整读回`research/quality-review-s03-2-1.md`并核SHA与原始事件、同产物四门禁、三平台产物/源码清单。独立修复隐藏生成列漏检；最终8顶层/653叶子子场景、storage17/backup38/account162/core52通过，临时源0/源码漂移0/确切活进程0。正式spec及docs已同步，S03.2.1记local_complete_target_pending，不重复累加旧轮次。
- S03.2.2根/发布接缝研究已停写交回，`research/root-publisher-contract-review.md`由主线完整读回并核最终SHA。`root-publisher-contract.md`现已冻结并激活为本片实施权威；只交付受控根、严格格式及平台发布，不开放API/启用/执行器/调度/轮转，研究和冻结不计产品通过数。
- S03.2.3独立建议`research/executor-manual-contract-review.md`已停写并由主线完整读回，169行/33,779字节，SHA256 `59445a34413f8633d674ece3a0b5cd74353899a0bf03800252697bc08b2506b3`。结合冻结前接缝复核六组修订和已经本机独立验收的S03.2.2实际接口，`executor-manual-contract.md`现已冻结激活；手动幂等/高水位、唯一执行器、无source对账、无删除授权结束及停服锁序按该单一权威实施。研究与冻结均不是产品验证通过声明。

S03.2.2已独立修复设备名主干空格别名漏检与Linux Stat失败遗漏Close错误。最终24顶层/257叶子、同一Windows产物storage17/backup38/account162/core52通过；主线完整读回115行/15,581字节的`research/quality-review-s03-2-2.md`并核SHA256 `4baa31ee219316e75e2290f12f37201d97ca0f81a66d090386d81b069835bbdb`及原始事件/193源/三产物，确认临时源0、确切活进程0。正式规范见`.trellis/spec/backend/backup-publish-contract.md`，主线证据见`.runtime/verify/s0322-mainline-closeout/evidence-readback.json`，真实Linux/NAS及崩溃/断电仍待验。S03.2.3现按`executor-manual-contract.md`冻结激活；策略仍关闭，调度/下载轮转及恢复UI/CLI留后片。

## 本轮交付优先级：快速可用版本（2026-11-23）

用户要求快速在当前修改上形成可用版本，本轮因此优先交付已验收 S03.2.2 产品基线的 Windows 内嵌网页开发包。S03.2.3 三份未接线草稿已按 SHA 隔离保存在 `.runtime/verify/s0323-draft-pause/`，不参与编译、不开放半成品 API，详见 `research/s03-2-3-draft-checkpoint.md`。冻结契约继续有效且后续待继续，整个 S03 仍进行中，不将本次构建计为 S03.2.3 已完成。

可用构建与启动说明为 `docs/usable-preview.md`；前端类型/构建和两 EXE 发布构建通过。同一 nas.exe 的 core52/backup38/account162/storage17 共269项通过，桌面入口另以 embedded/headless 模式完成启动/退出/重启检查。原产品193项源码零漂移；本轮不引入新产品契约、迁移或调度。独立轻量检查与主线读回属于本次可用构建验收，不替代后续 S03.2.3 行为门禁。

## 失败与回退

关闭策略不删除已有备份，已接纳执行明确完成/中断；保留最后成功点。恢复检查器不原地写生产；独立恢复准备失败时不对外启动副本，仍保留备份和原生产实例。程序降级恢复匹配版本的完整材料，不让旧程序打开新schema。清单/hash只证明一致性，不代替可信来源；秘密材料说明受限权限/离线保管。

## Out of Scope / Definition of Done

不做任意cron、在线整栈一致性抓卷、一键生产恢复、跨OS路径迁移或自动停容器。按父路线统一门禁；新增临时单测执行后删除，保留恢复集成脚本与脱敏证据。
