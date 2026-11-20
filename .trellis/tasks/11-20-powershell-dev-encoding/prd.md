# 修复 Windows PowerShell 脚本编码解析错误

## Goal

修复用户在 Windows PowerShell 中运行 `./scripts/dev.ps1 -Mode backend -MediaRoots 'E:/Media;F:/Books'` 时出现的意外右括号、字符串未闭合等解析错误。

## Requirements

- 保持现有 backend/frontend/build 参数、命令、退出码及项目内缓存行为。
- 在 Windows PowerShell 5.1 与 PowerShell 7 均能正确解析脚本和中文。
- 将受同一编码问题影响的 `scripts/dev.ps1` 与 `scripts/release-windows.ps1` 保存为 UTF-8 BOM；保留脚本文字和原有换行。
- 增加仅针对 PowerShell 脚本的编辑器编码约束，防止保存时移除 BOM；配置参数必须附中文说明。
- 不访问用户提供的真实媒体目录、不启动持久服务、不修改全局环境或执行策略。

## Evidence

- 本机 Windows PowerShell 5.1.19041.6456 默认编码为 gb2312，ParseFile(dev.ps1) 精确复现用户报告的四项解析错误。
- PowerShell 7.6.4 可正常解析相同脚本文本。
- `dev.ps1` 与 `release-windows.ps1` 无 BOM；后者也有一项 5.1 解析错误。`project-env.ps1`、`clean-session-temp.ps1` 已有 BOM 且可解析，无需修改。
- CodeGraph 已存在但未收录 scripts/*.ps1，直接读取脚本作为补充。

## Acceptance Criteria

- [x] 两个脚本仅增加 EF BB BF 前缀，其他原始字节完全保留。
- [x] scripts/*.ps1 全部通过 Windows PowerShell 5.1 和 PowerShell 7 ParseFile。
- [x] 两个解释器都能以受控 Go/npm 替身执行 backend/frontend/build，确认原样传递分号媒体根、调用参数、工作目录及退出码。
- [x] `.editorconfig` 仅约束 `*.ps1` 为 utf-8-bom，并带中文说明。
- [x] 验证只使用仓库 `.runtime/` 内的隔离样本，临时生成的测试源码在完成后清除。
- [x] 记录质量检查结果并同步跨平台规范。

## Out of Scope

Go/前端业务代码、真实媒体扫描、应用数据、完整发布构建以及外部 API 集成均不属于此次修复。

## Technical Notes

适用规范：`.trellis/spec/backend/cross-platform-contract.md`。之前的 collection-external-api 任务保持原样，本修复使用独立任务。
