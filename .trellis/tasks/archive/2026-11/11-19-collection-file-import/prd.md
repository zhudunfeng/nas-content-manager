# 资源收集：本地远程文件与音乐导入

## Goal

P4，依赖collection-core；只读登记、归档复制、rclone copy、自有音乐、目标冲突与完整性。2026-11-20 已授权实施。

## 依赖与范围

- 主任务：[资源收集 PRD](../11-19-resource-collection-plan/prd.md)。
- 实施细节：[完整修改计划](../11-19-resource-collection-plan/info.md)，以主任务的首期范围和契约为准。
- 依赖 collection-core 的统一协议、幂等和发布契约；不要求先完成通用插件平台。
- 2026-11-20 用户已指令按计划实现，当前状态为 in_progress，以实际验证作为完成依据。

## Acceptance Criteria（后续实施）

- [x] 本地只读登记与自有音乐扫描可用；归档复制按需选择，原文件与标签不变。
- [x] 真实rclone v1.76.0本地alias remote按固定copy语义复制，来源只读、任务隔离、发布冲突不覆盖。
- [x] 实际目标条目存在且size/mtime覆盖才算入库；原请求幂等，独立重试扫描不重复复制。
- [ ] 目标NAS实际云盘、磁盘不足、跨卷/网络卷与部分I/O故障验收；本机alias与路径行为测试不替代设备故障测试。

2026-11-20 代码实现和本机验证完成，保留上项部署验收。证据见主任务 `research/runtime-validation.md`、`backend-verification.md` 和 `worker-tool-versions.md`。

## Definition of Done

按主任务质量要求通过真实验证并记录证据；构建、测试和临时工具经 scripts/project_env.py 执行，遵守中文注释/日志和显式类型要求。实施前再配置本子任务上下文并读取相关技能，不以本文件存在认定已完成。

