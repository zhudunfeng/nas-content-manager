<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

Use the `/trellis:start` command when starting a new session to:
- Initialize your developer identity
- Understand current project context
- Read relevant guidelines

Use `@/.trellis/` to learn:
- Development workflow (`workflow.md`)
- Project structure guidelines (`spec/`)
- Developer workspace (`workspace/`)

If you're using Codex, project-scoped helpers may also live in:
- `.agents/skills/` for reusable Trellis skills
- `.codex/agents/` for optional custom subagents

Keep this managed block so 'trellis update' can refresh the instructions.

<!-- TRELLIS:END -->


<!-- USER:START -->
- 所有方法都需要有中文方法注释
- 实体字段需要有中文字段注释
- 业务逻辑关键方法需要有中文行注释
- 业务关键之处自动输出中文日志记录
- 方法入参出参需要有类型
- 每一个配置参数上必须存在中文解释
- 如果系统存在codegraph,代码检索尽量使用codegraph cli或codegraph mcp
- 当存在trellis时，使用trellis推进，计划自动创建任务并生成prd
- Trellis 任务归档前必须总结对当前项目有用的要点：实际交付、可复用决策、踩坑与约束、验证证据、未完成事项和下一步，并提供来源路径；长期规则回写相关 spec，批量归档须有覆盖全部任务的汇总，不能用归档后的 completed 状态代替实现或验收证据。执行清单见 `.trellis/spec/guides/archive-retrospective-guide.md`。
- 每次代码生成的单元测试类在完成任务后自动清除
- 本项目的本地构建、测试和临时工具命令优先通过 `python -B scripts/project_env.py -- <命令与参数>` 运行；PowerShell 当前终端也可先执行 `. ./scripts/project-env.ps1`，将可控缓存和临时目录限定在仓库 `.runtime/` 中。不得用用户级环境变量或 `go env -w` 代替；外部浏览器/MCP服务必须在启动前继承同样环境。
<!-- USER:END -->
