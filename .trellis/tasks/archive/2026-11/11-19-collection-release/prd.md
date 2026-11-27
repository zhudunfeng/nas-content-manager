# 资源收集：界面、平台部署与整体验收

## Goal

P5–P6，依赖全部来源链路；收集中心、Linux/Windows配置与精确部署校验、备份恢复和完整退出。2026-11-20 已授权实施。

## 依赖与范围

- 主任务：[资源收集 PRD](../11-19-resource-collection-plan/prd.md)。
- 实施细节：[完整修改计划](../11-19-resource-collection-plan/info.md)，以主任务的首期范围和契约为准。
- 依赖 collection-core、collection-rss-bt、collection-web 和 collection-file-import 的已验证链路。
- 2026-11-20 用户已指令按计划实现，当前状态为 in_progress，以实际验证作为完成依据。

## Acceptance Criteria（后续实施）

- [x] 资源收集概览/来源/订阅/候选/任务与现有下载和媒体详情连接，移动端从更多进入；真实Edge36项、四种宽度和两主题通过。
- [ ] Linux/NAS与Windows对接worker的目录映射和真实能力通过验证。
- [x] Windows v1/v2 Docker模板、固定清单和inspect归属数据校验通过；schema v3备份、Windows原生worker进程退出和文件释放验证通过。
- [ ] 目标NAS及Docker daemon上的镜像实际构建、三容器部署、目录权限和完整停止；不能以规范化Compose/交叉编译代替。
- [x] 首期隔离来源链路及原搜索43项/下载36项/托管35项/五媒体52项/发行76项回归通过；临时新增单元测试清理。

2026-11-20 实现及本机质量门通过，当前保留目标部署验收，尚未归档/提交。详细证据和环境边界见主任务 `research/runtime-validation.md`、`browser-acceptance.md`。

## Definition of Done

按主任务质量要求通过真实验证并记录证据；构建、测试和临时工具经 scripts/project_env.py 执行，遵守中文注释/日志和显式类型要求。实施前再配置本子任务上下文并读取相关技能，不以本文件存在认定已完成。

