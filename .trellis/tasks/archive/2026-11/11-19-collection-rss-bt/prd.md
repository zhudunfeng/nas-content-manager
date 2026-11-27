# 资源收集：RSS 订阅与 BT 桥接

## Goal

P2，依赖collection-core；有界RSS、规则试运行、人工确认/显式自动选择、复用原qB与入库门禁。2026-11-20 已授权实施。

## 依赖与范围

- 主任务：[资源收集 PRD](../11-19-resource-collection-plan/prd.md)。
- 实施细节：[完整修改计划](../11-19-resource-collection-plan/info.md)，以主任务的首期范围和契约为准。
- 依赖 collection-core 的统一协议、幂等和发布契约；不要求先完成通用插件平台。
- 2026-11-20 用户已指令按计划实现；本子任务已完成本地开发验收，目标部署以发布子任务记录为准。

## Acceptance Criteria（后续实施）

- [x] 首次基线不批量下载历史；回填有界；候选/规则/调度持久化；真实RSS HTTP与条件请求验证通过。
- [x] 自动规则显式启用且真实清单无歧义，按revision锁定选文件，模糊情况转人工；6类规则行为及来源并发修订验证通过。
- [x] 受控qB协议集成的所选真实文件经原BT门禁入库，外部同hash拒绝；删除媒体库撤销关联并保留文件。

2026-11-20 本地开发验收完成。证据见主任务 `research/backend-verification.md`、`runtime-validation.md`。公网RSS/BT载荷与目标NAS部署未验收，保留在发布验收中。

## Definition of Done

按主任务质量要求通过真实验证并记录证据；构建、测试和临时工具经 scripts/project_env.py 执行，遵守中文注释/日志和显式类型要求。实施前再配置本子任务上下文并读取相关技能，不以本文件存在认定已完成。

