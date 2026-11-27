# BT 搜种与下载衔接

## Goal

在现有下载中心增加易用、美观、易部署的种子搜索，支持按关键词查找资源、比较结果，并复用现有安全下载及选集入库流程。

## What I already know

- 用户已授权“规划并实现搜种功能”；由开发方选择合理专业默认方案并完成实现。
- 已有 Go HTTP + SQLite、Vue 3 + TypeScript、独立 qBittorrent 下载编排和五类媒体库。
- 现有 BT 任务处于 review，保持其未提交改动，不提交、不归档。
- Docker 部署由用户手动验证，本任务不执行 Docker 命令。
- 遵守 AGENTS.md 中文注释、日志、显式类型与配置说明要求；优先 CodeGraph。

## Assumptions

- 搜种为可选能力；未配置或服务离线不影响媒体浏览和原下载功能。
- 优先选择标准协议、服务端保存凭据和可控依赖，不自动安装未知搜索脚本。
- 首期仅手动搜索、筛选和选择下载；不包含 RSS 自动订阅、自动追剧和公网资源爬虫。

## Open Questions

- 无阻塞项；采用 Torznab 标准协议，详细决策与分步计划见 `info.md`。

## Requirements

- 下载中心提供搜索入口、关键词、来源筛选和清晰的加载、空结果、离线提示。
- 展示名称、来源、体积、做种数、发布时间；支持排序和受限分页。
- 选择结果后显式确认内容类型，再创建下载；复用现有选集、幂等、发布门禁。
- API 密钥及来源私密链接不返回浏览器、不写日志；所有接口鉴权与同源保护。
- 约束网络超时、响应大小、查询长度、并发、结果数量及缓存生命周期。

## Acceptance Criteria

- [x] 形成技术方案、接口契约及可执行分步计划。
- [x] 未配置、成功、空结果、部分失败、超时和畸形响应均有确定行为。
- [x] 能从搜索结果创建现有 BT 下载任务，不绕过选集与入库流程。
- [x] 浏览器与日志不泄漏凭据，不接受客户端任意下载 URL。
- [x] 桌面和移动端沿用现有主题与可访问交互。
- [x] Go 检查、前端类型检查构建、隔离集成验证通过；清理新增临时单元测试。
- [x] 更新部署与 API 文档，明确用户手动 Docker / NAS 验收范围。

## Definition of Done

- Trellis PRD、研究和实施验证记录落盘。
- 相关规范与部署示例同步，保留原五类媒体与 BT 功能。
- 无自动 Git 提交或任务归档，无 Docker 执行。

## Out of Scope

- 内置公共站点爬虫、验证码绕过、账号托管、RSS / 自动订阅。
- 搜索即自动下载，浏览器直接访问第三方凭据接口。
- Docker 与真实 NAS 部署验收。

## Research References

- [research/search-engine-options.md](research/search-engine-options.md) — 协议、方案取舍、下载代理与凭据边界。
- [research/jackett-image.json](research/jackett-image.json) — 固定版本及多架构摘要HTTP核验，无Docker执行。

## Technical Approach / Decision

采用只读来源配置文件连接已有 Prowlarr / Jackett，后端聚合 Torznab 并保存十分钟私密结果缓存。只返回随机结果编号和安全元数据，下载沿用原入口、幂等和选集。提供可选部署示例，不内置站点爬虫。关键词、来源、影视/音乐/书籍类别、做种/时间/体积排序与分页纳入MVP；RSS和自动订阅排除。用户已授权“规划并实现”，按现有易用易部署偏好采用该默认方案继续。详见 [info.md](info.md)。
