# S10 容量概览、保数据归档与安全清理

## Goal

让长期运行的占用与历史增长有明确治理路径，同时不丢失幂等、发布授权或真实媒体。状态：planning；相对工作量：大，分四个切片。

## 依赖与入口

- S10.1只读概览只需S01；归档/清理执行需S03匹配恢复点，告警接S04；不依赖S09，不需等待作品模型。
- worker：`internal/collectionworker/service.go`、`files.go`、`retry.go`、`internal/collectioncontract/models.go`。
- app：`collection_store.go`、`collection_import.go`、`download_gate.go`、`collection_runtime.go`、`collection_rules.go`、`cache.go`（均在`internal/app/`）；前端收集历史/设置容量页。
- [总路线](../11-23-nas-evolution-plan/roadmap.md)、[拟议契约](../11-23-nas-evolution-plan/info.md)、[自动化研究](../11-23-nas-evolution-plan/research/automation-implementation.md)。

## 范围与拟议契约

- 分媒体、应用cache/持久资产/备份、worker staging/complete、qB活动/做种、组件卷；实际/估算/未知、逻辑/物理字节分开，共享卷不简单相加。
- 当前worker的10000条限制同时作用于新提交与启动恢复。用冷热历史协议/UUID查询解决，不能仅删JSON、删DB行或无限调大上限。
- 首轮保留app任务主行/files与gate所需凭证，归档非必要热状态。taskId/requestId/requestHash/原意图/最终状态/发布清单/实例根身份/itemIds/RSS去重事实必须可恢复查询。
- worker冷热查询、同UUID提交/冲突、publish/对账保持原语义；冷记录损坏失败封闭，不把旧ID当新任务下载。
- 拟议storage usage、archive-plan/execute、cleanup-plan/execute；各所有者执行自身路径，能力协商不支持时禁用，跨app/worker用可恢复状态机，不伪造分布式事务。
- 清理默认预览；只针对无活动进程/做种/索引/发布/资产引用的自有失败暂存或可重建缓存，执行消费精确有期限清单，再校验且不扩大范围。

## 小 PR 顺序

1. **S10.1 只读容量/压力**：口径、卷身份、历史数量/额度、低水位/恢复去抖；此片可提前交付，不删除任何文件。
2. **S10.2 保数据归档**：终态/引用过滤、完整冷热记录和有界索引、app/worker归档意图与对账、历史分页及按UUID精确查询。
3. **S10.3 旧阈值压力验证**：超过10000历史的启动/查询/内存、幂等/发布门禁/归档中断与冷损坏，确认兼容后才替换旧启动限制。
4. **S10.4 精确清理**：归属/进程/引用复核、计划预览/执行、变化项跳过与逐项结果；首轮不清理complete、做种和组件卷。

## Acceptance Criteria

- [ ] 未挂载/共享卷/硬链接/不可读远端不显示虚假精确总数；低水位告警和恢复有阈值/冷却。
- [ ] 仅可归档明确终态，无待执行/对账/入库重试；acquired/publishing/recovery_required/uncertain不归档。
- [ ] 超过旧上限后重启和分页有界；归档ID按UUID仍返回原结果，同ID不同hash仍409，冷库损坏不能重新提交成新任务。
- [ ] app ACK丢失、归档各阶段中断/并发提交可对账；查询GET无写目录/移动历史副作用。
- [ ] 原gate、已入库文件鉴权Range、worker离线读取和RSS跨源去重回归；不丢发布清单/原任务行。
- [ ] 清理覆盖引用变化、活动/做种/未知状态、重解析点/同名替换、权限/部分I/O失败；不触及原片/人工原件/秘密/组件DB。
- [ ] 清理/归档报告区分热状态减少与实际释放字节，不宣传任务隐藏等于释放媒体空间。
- [ ] 真实磁盘低空间/文件系统原子语义、配套恢复单独实测；冷格式恢复工具/说明与协议能力更新。

## 失败与回退

关闭归档/清理后冷热读取仍工作，保留所有事实；未知归属或引用一律跳过。旧程序不识别冷格式，不能直接降级，必须恢复升级前完整app/worker材料。不可证明可删时转人工，不靠放宽门禁或删除历史解困。

## Out of Scope / Definition of Done

不自动删除complete/做种/媒体/组件卷，不做通用rm或真实删除授权扩张。按父路线统一PR门禁；保留有界压力/归档集成脚本，临时单测执行后删除。
