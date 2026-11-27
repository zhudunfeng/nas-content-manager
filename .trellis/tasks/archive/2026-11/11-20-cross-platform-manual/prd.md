# 跨平台构建改为手动触发

## 目标与需求

按用户要求，将 `.github/workflows/cross-platform.yml` 改为仅通过 `workflow_dispatch` 手动触发，移除 `push`、`pull_request` 及其对应说明。

## 范围

保留现有构建矩阵、步骤、权限和手动触发说明。现有其他任务及未提交改动不属于本次范围；保持当前任务指针不变。

## 验收标准

- [x] `on` 中仅存在 `workflow_dispatch`。
- [x] 工作流相对于原文件仅删除两个自动触发事件及对应两行注释。
- [x] `git diff --check -- .github/workflows/cross-platform.yml` 通过。

## 核验与规格审视

本次仅更改 CI 触发条件，不涉及 API、存储、媒体和构建行为，因此执行目标文件精确差异检查，不运行应用全量构建，不新增临时单元测试类。已审视现有跨平台契约，无需新增通用约定或修改现有规格。
