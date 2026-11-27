# UAV-Downloader 视频搜索下载插件

## Goal

将用户指定的 https://github.com/Alos21750/UAV-Downloader 集成为 NAS 的可选视频搜索下载插件，沿用资源收集来源、执行端、任务与媒体入库流程。

## Requirements

- 2026-11-25 用户明确追加 SupJav：提供站点选择、关键词分页搜索、单视频预览、下载入库，复用现有稳定请求编号及受控网络下载流程。
- 研究上游真实搜索与下载接口、许可证及运行依赖，固定可审核的适配方式。
- 复用已有来源管理、鉴权、预览、持久任务、取消、发布与视频库入库机制。
- 提供视频关键词搜索、结果选择与下载入口，前后端契约一致。
- 可选启用；依赖缺失、站点不可达、空搜索与下载失败提供明确状态。
- 保持路径边界、网络访问限制、并发和暂存预算，不引入任意代码执行入口。
- 方法、字段及配置保持中文注释和明确类型，业务关键处输出脱敏中文日志。
- 本地构建、测试和工具通过 scripts/project_env.py 运行，临时产物放在 .runtime。

## Assumptions

- 用户已授权实施集成，技术细节优先采用现有插件约定。
- 新任务独立于已提交的 JMComic 插件任务，不修改原任务状态。
- 范围为固定插件适配，不扩展通用插件市场。

## Acceptance Criteria

- [x] 上游调研与适配契约落盘，依赖/版本/许可证可追溯。
- [x] 可配置、探测、搜索、选择视频并创建持久下载任务。
- [x] 下载产物经过现有校验与发布，可导入视频库；取消、失败正确反馈。
- [x] Go 与 Vue 质量检查、相关离线与集成回归通过；如实说明真实网络验证边界。
- [x] 使用文档和 Trellis 规范同步，清除本任务新增临时单元测试文件。

## Research References

- research/supjav-upstream.md（追加 SupJav 的规范 ID、多线路解析与固定依赖研究）
- research/supjav-implementation.md（第五站真实解析器、私有线路偏移与 TS 清理验证）
- research/supjav-quality.md（独立真实下载器矩阵、结构反例修复与错误分类）
- research/supjav-mainline-verification.md（本次 SupJav 同批构建、应用/浏览器及原功能回归）
- research/uav-upstream.md
- research/integration-architecture.md
- research/huangguo-upstream.md（用户追加站点的专项适配）
- research/quality-check.md（独立质量门及真实下载反例）
- research/uav-worker-verification.md（固定依赖与执行端验证）
- research/completeness-retrospective.md（范围响应与时长校验复盘）
- research/mainline-verification.md（最终产物、回归及验收边界）

## Out of Scope

- 任意插件代码上传执行、完整插件市场。
- 绕过付费或账号权限限制。
- 五站以外站点、账号/cookie输入、播放列表递归和自动批量下载。
- 黄果超过100单集的搜索页及超出当前搜索时限的整页展开，明确失败，不静默截断。

## Technical Approach

- 固定 `uav` 来源与能力，适配 `jable`、`missav`、`hanime1`、`huangguo`、`supjav`。搜索请求选择站点，不伪造未知总数。
- UAV-Downloader 固定上游提交 `d40f09457fbd38dd7dc94580637242afafaf3939`，只在独立 worker 运行审核后的无头搜索/详情桥接。
- 保留原 schema 6。共享持久请求新增 `uavSite,omitempty` 和 `videoId,omitempty`，旧类型序列化与摘要不变。
- 主服务接口 `POST /api/collection/sources/{id}/uav/search`；worker 对应 `/v1/uav/search`。预览输入 `{sourceId,uavSite,videoId}`，输出沿原 `preview`，固定 `kind=uav,profile=video`。
- 任务仍用原 `{requestId,previewId,contentType:"video",createLibrary,libraryId}`。仅在实际下载、产物校验、发布和扫描覆盖后入库。
- 桥接只解析媒体地址，下载由固定受控 yt-dlp 原生 HTTP/HLS 执行。所有请求经过执行端公网策略代理，FFmpeg 仅做离线处理，不放宽原网页插件格式规则。
- 原生默认关闭，Docker 在构建时安装固定依赖，提供明确能力探测。上游要求 Python >=3.12，不能直接复用 Bookworm 系统 Python 3.11。

## Decision

- 2026-11-25 用户追加 Huangguo 必须接入。新增专项上游研究、规范剧集ID和匿名播放元数据解析；动态CDN认证头只允许逐资源、逐重定向作用域使用，不能作为yt-dlp全局请求头。扩大前端站点选项及离线、集成和浏览器验收覆盖。

- 用户指示直接集成，接入既有来源与任务架构；完整上游GUI/Web队列不嵌入主服务。
- 比较完整上游下载器与固定搜索/解析桥接：选择后者，以复用本项目持久任务、取消、网络策略与产物发布。
- 覆盖空结果、站点失败、依赖缺失、无效ID、旧预览、重复提交、取消和重启；真实公网、Docker及NAS验收分别记录。

## Implementation Plan

1. 调研上游和现有收集链路，确定固定适配协议与依赖。
2. 实现执行端、主服务、前端及部署文档。
3. 进行 Trellis 独立复核、构建与集成验证，记录实际验证范围。

## SupJav Increment — 2026-11-25

用户已授权直接接入 SupJav。原四站验收记录保留；新增第五站，不升级固定上游提交，不改变原数据库及任务 API。

### Requirements and Decisions

- 搜索、预览、下载与原站点使用同一交互和错误反馈；搜索结果不暴露站外媒体地址、封面或凭据。
- 调研固定上游真实支持的播放线路，优先选择可由现有受控下载链路验证的线路；按确定顺序回退，所有线路均失败时明确失败。
- 不增加任意线路 URL、脚本、Cookie 或下载参数输入；不引入完整上游下载队列。
- 考虑站点空结果、非规范视频编号、无可用线路、错误页面、分页与重定向，保留旧四站行为。
- 本次扩展仅接入第五站；自动监控、批量抓取和自动追更不在范围内。

### Acceptance Criteria

- [x] 固定上游 SupJav 接口及支持线路研究落盘，代码规范和配置无漂移。
- [x] Go、Python 和 Vue 均识别 SupJav 规范编号，完成搜索与单视频预览/持久任务传递。
- [x] SupJav 真实解析器离线覆盖搜索、空页、分页、多线路解析及失败；媒体仍通过原受控下载和校验。
- [x] 真实应用/执行端离线集成与浏览器验证 SupJav 下载、入库及原播放器；四站回归通过。
- [x] Go vet/编译、前端类型检查和构建通过，清除新增临时单元测试，同步文档及真实验收边界。
