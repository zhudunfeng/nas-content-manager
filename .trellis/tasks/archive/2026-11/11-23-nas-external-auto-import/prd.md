# S09 外部完成目录自动入库

## Goal

将外部整理完成文件从“另行手动预览登记”推进到显式授权后的持续发现与真实入库，复用原有链路。状态：planning；相对工作量：大。

## 依赖与入口

- 核心只依赖S01及既有preview/register/实际扫描核验；异常事件在S04就绪后接入，不阻塞发现/登记核心开发。
- 现有入口：`internal/app/collection_preview.go`、`collection_import.go`、`collection_store.go`、`collection_models.go`、`collection_runtime.go`、`collection_api.go`。
- 前端：`web/src/components/CollectionSourceDialog.vue`、`CollectionExternalPanel.vue`、`CollectionCenter.vue`、`CollectionTaskDialog.vue`与`web/src/types.ts`；拟新增`internal/app/collection_auto_import.go`。
- [总路线](../11-23-nas-evolution-plan/roadmap.md)、[拟议契约](../11-23-nas-evolution-plan/info.md)、[自动化研究](../11-23-nas-evolution-plan/research/automation-implementation.md)。

## 范围与拟议契约

- 每个策略绑定已授权完成根/受控子目录、已存在目标库、来源/策略revision；第一版仅register，不自动copy/移动/建库。
- 默认关闭；先只读试运行，启用后默认只处理新产物；历史基线需单独预览确认。
- 有界轮询/稳定窗口/临时扩展排除、持久发现台账和原任务追踪；size/mtime稳定仅必要条件，只有可信完成目录才能全自动，未知根转人工。
- 从现有HTTP handler抽出带类型共享应用服务供HTTP/调度调用，不后端请求自身HTTP，不新增绕过发布gate的插表路径。
- 拟议source import-policy读写/preview/run及observations分页；requestId在提交前持久化，按规范源身份/版本/意图去重，参数变更冲突不换随机UUID绕过。

## 小 PR 顺序

1. **S09.1 映射与诊断**：本地完成根→目标库、API在线/挂载可读/目标可用分开，复用有界目录预览，不调度。
2. **S09.2 共享服务与台账**：抽取preview/register服务、观察版本/稳定性、批次幂等键及taskId/itemIds关联，保留手动流程。
3. **S09.3 授权自动发现**：默认停用、有界轮询/批次、明确新旧产物边界、来源/目标修订核对、调用原register/真实扫描核验。
4. **S09.4 恢复和可见性**：发现/待稳定/待确认/登记中/已入库/失败的追踪，失败复用原任务重试，接S04事件；目标宿主目录/ACL实测。

## Acceptance Criteria

- [ ] 未挂载/空挂载、文件增长/同名替换、临时文件/越界软链、来源停用/目标删除、revision冲突有准确结果。
- [ ] 新产物默认处理，历史不批量追溯；受管staging/未知complete不能当可信外部完成根。
- [ ] 重启/重复扫描/响应丢失使用原键原任务；部分失败可追踪，uncertain外部动作不重发。
- [ ] 来源变更使未提交候选失效，已提交意图不可变；删除目标不从历史偷偷重建。
- [ ] 至少一个真实外部完成文件→发现→原register任务→实际扫描→itemIds→鉴权Range全链路通过。
- [ ] worker离线仍可读原已发布文件；未发布gate拒绝，原手动导入与原生获取链路回归通过。
- [ ] 没有新增上游下载/整理请求，没有源文件移动/删除；事件只引用本地任务事实。
- [ ] NAS挂载可见性、UID/ACL、外部完成约定和网络卷稳定性有真实证据；缺少可信完成约定时只开放待确认模式。

## 失败与回退

停用策略只停止新发现，保留已提交任务、台账和媒体；不自动取消/重发外部操作。手动预览导入继续可用。数据库回退使用匹配备份，不删除台账当作“重新开始”。

## Out of Scope / Definition of Done

不接管qB、监听任意目录、自动重命名/整理、网页新来源、将外部accepted/100%视为入库。执行父路线统一PR门禁，扩展verify_collection/verify_external稳定脚本，新增临时单测执行后删除。
