# Research: GitHub Actions 免费能力与边界

- Query: GitHub Actions 都能免费做什么；当前额度、运行限制，以及本仓库的实际用途。
- Scope: mixed（GitHub 官方文档及本地构建契约）
- Date: 2026-11-19（Asia/Shanghai）
- Method: 已调用 web.run 搜索及打开官方文档，但本次工具没有返回正文或可用引用编号；随后通过仓库运行时包装器中的 Python urllib 直接抓取 GitHub 官方页面，抽取正文核对。以下数值来自当前可访问正文，不使用记忆中的旧计费表。

## Findings

### 1. 免费边界（已确认）

- 公开仓库的标准 GitHub-hosted runners 免费；涵盖标准 Linux、Windows、macOS 运行器。公开仓库的 larger runners 仍收费，不能把全部云端机器称为免费。免费使用仍受作业时长、并发、可接受使用政策等限制。
- 私有仓库可使用账户套餐包含的每月计算额度；费用记在仓库所有者，多个仓库共享所有者额度，并非每个仓库另送一份。
- 当前官方计费页将额度写成金额而非固定分钟；GitHub Free 为每月 $12 的 included compute usage。按标准 Linux x64 2-core 的 $0.006/分钟折算，约为 2,000 分钟/月。这个 $12 是抵扣计算使用量的包含额度，不能提现。
- 标准 Windows 2-core 当前为 $0.010/分钟，macOS M1/Intel 3/4-core 为 $0.062/分钟。因此不要套用旧资料中的 Windows×2、macOS×10 作为当前精确规则。Free 额度在只运行这些机器且不被其他合格计算使用消耗时，分别约可覆盖 1,200 Windows 分钟或 193.5 macOS 分钟。
- Linux 1-core 当前 $0.002/分钟，Linux arm64 2-core 为 $0.005/分钟，进一步说明“Free 固定 2,000 分钟”不够精确；通常用标准 Linux 2-core 的约 2,000 分钟解释最易懂。
- Self-hosted runners 当前官方计费页明确列为独立免费的使用项目，不消耗上述包含计算额度。需要自己提供 NAS/电脑/服务器，并承担电费、网络、维护等成本；上传的 artifacts 等另受存储规则约束。
- 无有效支付方式时，包含计算额度耗尽后，使用会被阻止；有支付方式时应检查账户预算及停止使用设置，超额可能收费。

核对的关键原文（计费页）：

> The included compute usage is expressed as a monetary amount, not a fixed number of minutes

> Self-hosted runners.

### 2. 额度和存储（已确认）

| 套餐 | 每月包含计算额度 | 标准 Linux x64 2-core 对应分钟（折算） | Artifact 包含存储 | Cache 包含存储 |
| --- | --- | --- | --- | --- |
| GitHub Free（个人/组织） | $12 | 约 2,000 | 500 MB | 每仓库 10 GB |
| GitHub Pro | $18 | 约 3,000 | 1 GB | 每仓库 10 GB |
| GitHub Team | $18 | 约 3,000 | 2 GB | 每仓库 10 GB |
| GitHub Enterprise Cloud | $300 | 约 50,000 | 50 GB | 每仓库 10 GB |

- Pro/Team 本身是收费套餐，表内为套餐包含权益，不能把 Pro/Team 称为免费账户。
- 计算额度可能与 cloud sandbox compute/memory 等合格计算共享；Team/Enterprise 合格 larger runners 也可能从同一包含计算额度抵扣。公开仓库标准 runner 和 self-hosted runner 独立免费。
- Actions artifacts 与 GitHub Packages 共享套餐对应存储池；cache 为单独的每仓库 10 GB，不占同一池。
- 存储按小时累计，删除只停止后续累计，不能抹去之前已经发生的存储使用。不要解释成每月可以重新上传 500 MB 的传输额度。
- Cache 超过 10 GB 的计费只有在仓库缓存上限被主动调高到包含量以上时才会发生。

### 3. 免费可以做的实际任务

| 任务 | 具体例子 | 免费成立的条件/边界 |
| --- | --- | --- |
| 自动检查代码 | push/PR 后执行 Go vet/test、Vue TypeScript 检查、前端构建 | 使用免费标准 runner 或私有仓库计算额度内 |
| 跨平台打包 | 构建 Windows amd64、Linux amd64/arm64 安装包 | 构建和依赖下载受计算额度、第三方分发条件影响；不代表代码签名服务免费 |
| 自动发布版本 | 打 tag 后生成发布说明、校验和、上传 GitHub Release 附件 | Release 与 Actions 临时 artifacts 是不同产品机制 |
| 构建并发布容器 | 构建 NAS Docker 镜像、推送到 GHCR 等镜像仓库 | 镜像存储、流量遵循目标仓库独立政策，不能笼统保证所有私有镜像永久免费 |
| 发布文档和静态网站 | 构建项目说明、更新日志、Vue 静态展示页并发布 Pages | Free 套餐 Pages 面向公开仓库；Pages 只能承载静态内容，不能运行 Go 服务和 SQLite |
| 仓库和定时自动化 | 定时检查依赖/链接、生成插件索引/版本清单、管理 issue 标签 | 定时触发可能延迟或丢弃；外部收费 API/消息服务不因 Actions 免费而免费 |

### 4. 运行及定时限制（已确认）

- GitHub-hosted runner：每个 job 最长 6 小时，超过后终止并失败。不要把 workflow 的 35 天总上限误认为单个云端作业能持续跑 35 天。
- Self-hosted runner：每个 job 当前最长 5 天；整个 workflow 的总时长上限为 35 天，包含等待和审批时间。
- schedule 最短间隔 5 分钟，仅运行默认分支的最新提交；工作流文件必须位于默认分支。
- schedule 高负载时可能延迟，严重时排队作业可能被丢弃，整点较繁忙；不能当精确定时器或可靠监控平台。
- 公开仓库连续 60 天没有仓库活动，定时工作流会被自动禁用。
- 当前 schedule 默认 UTC，文档已经支持 IANA timezone 参数；如介绍具体中国时间配置，不能继续称“只能 UTC”。
- Actions 适合有开始和结束的自动任务；Go NAS 服务、媒体转码常驻队列、数据库、个人网盘应运行在 NAS/服务器，不能把 Actions runner 当免费 24 小时主机。

### 5. 本仓库对应关系

本地发现文件：

- `go.mod:1`：Go 模块为 nas-content-manager，`go.mod:3` 指定 Go 1.26.0；可由 Actions 安装对应 Go 并运行构建检查。
- `web/package.json:7`：已有 dev/typecheck/build/preview scripts；可在 CI 运行 npm ci 与 npm run build。
- `.trellis/spec/backend/index.md:20`：质量门要求 Go vet、Go test、Go build 及隔离 verify 脚本，适合放进 CI。
- `.trellis/spec/backend/cross-platform-contract.md:14`：已有 `scripts/release.py` 支持 windows-amd64、linux-amd64、linux-arm64，可形成发布矩阵；正式包需要工具 manifest，不能把不含工具的开发包当成正式完整包。
- 用户 AGENTS.md：本仓库本地构建测试及临时命令使用 `python -B scripts/project_env.py -- ...`，缓存和临时目录限定在 `.runtime/`。本次网络核对也使用该包装器，没有启动外部浏览器或服务。

建议先做三个工作流：push/PR 自动检查；tag 自动打包并发布 Release；构建 NAS Docker 镜像。文档站、插件索引定时更新可作为后续增强。这里只给用途说明，不修改 CI 文件。

## External References

均为 GitHub 官方资料；核对日期 2026-11-19。

1. **GitHub Actions billing — GitHub Docs**：https://docs.github.com/en/billing/concepts/product-billing/github-actions — 免费类型、美元计算额度、存储池与超额行为的主依据。
2. **Actions runner pricing — GitHub Docs**：https://docs.github.com/en/billing/reference/actions-runner-pricing — Linux/Windows/macOS 机器当前每分钟费率。
3. **Actions limits — GitHub Docs**：https://docs.github.com/en/actions/reference/limits — job 6 小时、self-hosted 5 天、workflow 35 天等上限。
4. **Events that trigger workflows — GitHub Docs**：https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows — schedule 5 分钟、默认分支、延迟与 60 天禁用、时区参数。
5. **What is GitHub Pages? — GitHub Docs**：https://docs.github.com/en/pages/getting-started-with-github-pages/what-is-github-pages — Pages 静态托管定位与 Free 仓库可见性条件。
6. **About releases — GitHub Docs**：https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases — 发布附件目前每 release 最多 1,000 个、单文件小于 2 GiB，发布总量和带宽无硬性总限；仍受平台政策约束。

## Related Specs

- `.trellis/workflow.md`：调研应落入当前 task/research。
- `.trellis/spec/backend/index.md`：已有质量门。
- `.trellis/spec/backend/cross-platform-contract.md`：平台构建与发布契约。

## Caveats / Not Found

- web.run 本次返回空工具结果，无法提供正常可渲染的搜索引用 ID；不能编造引用编号。以上官方 URL 已通过 urllib 成功抓取及核对。
- 当前文档明确 self-hosted 免费；本次未定位到可核对的 2026 自托管收费延期/取消公告原文，不推测公告历史，也不把旧的“将收费”计划说成当前事实。
- 当前计费页已明确按美元额度描述。未核对该变化何时上线，不声称变更发生于某个具体日期；也未核实所有账户旧账单展示是否同步切换。
- 发布和容器场景属于能力举例；第三方云主机、域名、Docker Hub/GHCR 私有存储、签名证书、短信/AI API 费用不包含在 Actions 免费权益里。
