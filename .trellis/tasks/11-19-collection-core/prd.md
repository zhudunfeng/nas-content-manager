# 资源收集：契约、统一任务与执行端基础

## Goal

P0–P1；先修订收集契约，交付来源/候选/任务/产物、顺序迁移、幂等和worker窄协议。2026-11-20 已授权实施。

## 依赖与范围

- 主任务：[资源收集 PRD](../11-19-resource-collection-plan/prd.md)。
- 实施细节：[完整修改计划](../11-19-resource-collection-plan/info.md)，以主任务的首期范围和契约为准。
- 先完成契约/样例验证，再建立所有来源共用的控制面和执行协议。
- 2026-11-20 用户已指令按计划实现；本子任务已完成本地开发验收，目标部署以发布子任务记录为准。

## Acceptance Criteria（后续实施）

- [x] 迁移保留原有用户、媒体、下载和进度；schema v3真实SQLite迁移及备份回归通过。
- [x] 重复requestId、派发超时和重启能按原编号对账，不重复启动操作。
- [x] 暂存经扫描/封面/直读拒绝；主服务复核产物后才发布，版本变动冻结。

2026-11-20 本地开发验收完成。证据见主任务 `research/backend-verification.md`、`runtime-validation.md`；目标平台部署验收归入collection-release，不以交叉编译代替NAS实机。

## Definition of Done

按主任务质量要求通过真实验证并记录证据；构建、测试和临时工具经 scripts/project_env.py 执行，遵守中文注释/日志和显式类型要求。实施前再配置本子任务上下文并读取相关技能，不以本文件存在认定已完成。

