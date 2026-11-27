# S11 规则模板、命中解释与缺集视图

## Goal

复用已有RSS和自动规则执行器，降低配置门槛并解释每次决策，再基于可信作品身份展示缺集状态。状态：planning；相对工作量：大，可分段交付。

## 依赖与入口

- S11.1–2只需S01及现有RSS evaluator；S11.3–4依赖S05/S06的确认作品/季集和可信预期集目，不能用标题猜全集数。
- 现有入口：`internal/app/collection_rules.go`、`collection_models.go`、`collection_api.go`、`collection_store.go`、`collection_bt.go`。
- 前端：`web/src/components/CollectionSubscriptionDialog.vue`、`CollectionCenter.vue`、`CollectionAcquireDialog.vue`、`types.ts`；作品缺集接S05实际详情域。
- [总路线](../11-23-nas-evolution-plan/roadmap.md)、[拟议契约](../11-23-nas-evolution-plan/info.md)、[自动化研究](../11-23-nas-evolution-plan/research/automation-implementation.md)。

## 范围与拟议契约

- 固定模板生成原include/exclude/quality/episodePattern/selectionPattern；保存实际规则快照和模板ID/版本，旧规则/高级表达式不被后台改写。
- 预览与真实调度共用evaluator：输出稳定reasonCode与中文reasonText、命中/排除、季集解析、已有意图/冲突、人工原因；预览无下载副作用。
- 作品绑定明确确认，旧规则无绑定继续运行；跨来源身份不能仅按标题判等，失败历史也需分清“曾接受”与“当前可播放”。
- 缺集区分预期未发现、候选待选、已有意图、下载中、已发布待入库、真实入库、原文件缺失、失败待处理；预期未知显示未知，不显示误导完成率。
- 拟议templates查询、扩展订阅preview、作品episodes/status和必要获取意图账本；原requestId/fingerprint继续约束传输动作。

## 小 PR 顺序

1. **S11.1 模板编辑**：有限版本模板、中文字段说明/样例，预览生成真实规则；不自动启用新规则。
2. **S11.2 解释/冲突**：共用判定函数、结构化原因、规则revision、候选分页、季集歧义与原历史去重展示。
3. **S11.3 作品绑定/缺集**：可信集目来源或有限人工维护、明确绑定、来源无关身份、实际item/发布可用性账本。
4. **S11.4 偏好建议**：身份明确后支持画质/语言/字幕候选排序，默认只推荐；多版本替换/洗版另立任务，不在此自动执行。

## Acceptance Criteria

- [ ] 原规则未改变；模板快照可复现，高级表达式仍可编辑；预览零外部写。
- [ ] 同一规则/输入的预览与调度决策一致，旧revision和过期预览不作为执行授权。
- [ ] 单集/多集/季包、S00、多个发行、绝对集号未知、同名不同作品、跨来源重复/手动自动竞争均有解释。
- [ ] 失败/取消/冷历史不引发无意重复获取；归档后去重语义保持。
- [ ] 缺集基于可信预期身份和真实items/发布/文件可用性；外部100%或历史completed不直接算可播放。
- [ ] 未确认作品/不完整预期显示未知和待确认，不给虚假0缺集/100%完成率。
- [ ] 候选排序默认不替换旧文件；停用新功能仍可人工确认和使用原规则。
- [ ] 资源命名/季集样本、集目来源及限制写入说明；若要新外部元数据，另核官方协议/许可/预算再实施。

## 失败与回退

停用模板/绑定增强不删除旧规则或获取历史；歧义转人工，不猜测下载。保留作品映射与原幂等事实，修订冲突刷新再预览，不换requestId强行提交。

## Out of Scope / Definition of Done

不新建平行订阅系统、远程模板脚本、自动洗版删旧、自动绝对集号推断。按父路线统一PR门禁，扩展verify_collection和qB fixture，新增临时单测执行后删除。
