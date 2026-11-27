# 文档质量复核：归档总结与流程规则

日期：2026-11-27（Asia/Shanghai）。范围仅为本任务新增/修改的文档和元数据；不修改业务代码或前批归档资料，不执行归档或 Git 提交。

## Findings（已修复）

- 文件：本任务 `prd.md`。三份研究文件已经生成，研究引用仍带“待生成”；移除过期标记，保留路径。
- 未发现月度任务覆盖、原状态、来源链接、入口一致性、工作流区块或原有资料完整性的实质缺陷。

## 核验方法与实际结果

1. `python -B scripts/project_env.py -- python -B .runtime/tmp/verify_archive_lessons_docs.py`
   - 归档前对本任务最终归档路径作单一虚拟映射，其他目标按真实文件系统解析。
   - 本报告生成前检查 13 份 Markdown、111 个本地链接；全部目标存在。
   - 月度逐任务表恰好覆盖原快照 50 个任务，无遗漏、重复或状态差异。
   - 原状态计数：planning 10、in_progress 6、review 23、completed 11，与快照一致。
   - 427 个前批归档文件的 SHA-256 与主会话保存的归档前摘要一致。
   - 三个 finish-work 入口正文在规范化指南相对路径后完全一致；archive 与 add_session 命令均带 `--no-commit`。
   - AGENTS 的 Trellis managed 区保持原样；workflow 四组状态区块标签完整；未发现本轮范围外的已跟踪修改。
2. `git diff --check -- AGENTS.md .trellis/workflow.md .agents/skills/trellis-finish-work/SKILL.md .claude/commands/trellis/finish-work.md .cursor/commands/trellis-finish-work.md .trellis/spec/guides/index.md`
   - 退出码 0，无空白错误；仅提示 Git 将来会按仓库策略规范化换行。
3. `python -B scripts/project_env.py -- python -B .trellis/scripts/task.py validate 11-27-trellis-archive-lessons`
   - 退出码 0；implement.jsonl 1 项、check.jsonl 3 项全部有效。

本报告落盘后再次执行文档核验脚本：14 份 Markdown、111 个本地链接、50 项覆盖、427 份原归档摘要和 3 个入口均通过，errors 为空。

## 内容与事实边界

- 月度总结按主题提供当前规范/研究来源，详细可执行字段仍由已有 spec 维护，没有建立第二套 API 事实来源。
- 归档 `completed`、归档前生命周期状态、历史本机验证、真实 NAS/站点/发行验收分开陈述。
- S03 基础切片与尚未交付的执行器/调度/轮转/恢复链明确区分；S04–S12 及通用插件/飞牛规划没有被写成已实现。
- 本次未重跑历史业务回归，未执行 Go/npm 构建、类型检查、真实站点、目标 NAS 或正式发行验收；这些不属于文档任务的适用检查，不能将文档检查结果扩大为产品测试通过。
- 未生成永久单元测试类；可复用检查脚本只保留在仓库忽略的 `.runtime/tmp/verify_archive_lessons_docs.py`。

## Findings（未修复）与收尾

- 无阻止归档的缺陷。当前自身总结和 PRD 的最终验收标记由主会话在收尾时据实际结果更新。
- 主会话归档本任务后，应执行 `python -B scripts/project_env.py -- python -B .runtime/tmp/verify_archive_lessons_docs.py --after-archive`，取消虚拟映射，检查真实归档链接和当前任务指针；此项在本复核阶段尚未执行。
- 允许主会话完成总结验证栏并归档；不授权自动 Git 提交。

## 主会话归档后真实路径复核

2026-11-27 主会话先完成任务总结，再执行原生 archive 命令并显式传入 --no-commit，随后执行：

```text
python -B scripts/project_env.py -- python -B .runtime/tmp/verify_archive_lessons_docs.py --after-archive
python -B scripts/project_env.py -- python -B .trellis/scripts/task.py list
```

- 两条命令均退出 0；取消虚拟路径映射后，14 份 Markdown、112 个本地链接、50 项任务及原状态、427 份原归档文件摘要和三个入口全部通过，errors 为空。
- 活动任务为 0；本任务活动目录不存在，当前任务指针已清除。
- 本任务 PRD 最后一项在实际归档核对成功后标记完成。本轮没有业务代码或永久单元测试类变更，没有自动提交 Git。
