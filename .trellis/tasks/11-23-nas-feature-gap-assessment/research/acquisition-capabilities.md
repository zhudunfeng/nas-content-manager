# Research: 资源获取、下载、导入与组件扩展能力评估

- Query: 当前 NAS 内容管理系统在资源获取、下载、导入、外部组件和插件扩展方面，哪些能力已经闭环，哪些仍然缺失？
- Scope: internal，仅检查仓库源码、现有契约、任务和验证记录。
- Date: 2026-11-23
- 基准: `main`，提交 `6f380be`。不安装组件、不连接真实用户来源、不运行应用。

## 检索方法

使用现有 CodeGraph CLI 查询 `collectionRoutes`、`runCollectionTask`、`CollectionSubscription`、`collectionComponents`、`downloadRoutes`、`torrentSearchRoutes`、`plugin`，再对定位源码定向读取。索引由主代理确认 up-to-date，211 files / 4399 nodes；未刷新索引。CLI 通过 `python -B scripts/project_env.py -- node D:/adunsoftware/nodejs/node_modules/@colbymchenry/codegraph/npm-shim.js ...` 运行。对字符串、界面文案和配置缺口补充使用 `rg`，没有将未命中单独视为不存在的证明。

## Files found

| 文件 | 用途 |
| --- | --- |
| `internal/app/collection_api.go` | 来源、RSS 订阅、候选与统一获取任务路由和校验 |
| `internal/app/collection_rules.go` | 定时 RSS、关键词/体积/季集匹配、唯一规则自动获取 |
| `internal/app/collection_preview.go` | 本地只读登记和 worker 获取预览，绑定来源修订 |
| `internal/app/collection_runtime.go` | 持久任务调度、发布、恢复、取消和独立入库重试 |
| `internal/app/collection_import.go` | 发布文件版本与实际媒体条目覆盖核验 |
| `internal/collectionworker/tools.go` | 真实 yt-dlp、rclone copy 与本地复制的固定执行策略 |
| `internal/app/downloads.go` | qB 添加、选文件、启停、限速、对账及入库接口 |
| `internal/app/torrent_search.go` | Torznab 搜种接口 |
| `internal/app/collection_external_api.go` | 四类外部服务固定业务动作及不确定结果对账 |
| `internal/app/collection_moviepilot.go` | MoviePilot 搜索、下载、整理、刮削与索引工作流 |
| `internal/app/collection_components.go` | 四固定组件状态、部署、启动、失败重试 |
| `internal/enginecontract/collection.go` | 固定产品白名单、配置授权和状态协议 |
| `web/src/components/CollectionCenter.vue` | 来源/订阅/候选/任务界面与现有下载详情衔接 |
| `web/src/components/CollectionExternalPanel.vue` | 外部 API 连接、动作及成品另行入库提示 |
| `web/src/components/CollectionMoviePilotWorkflow.vue` | 六阶段工作流的明确授权和结果文案 |
| `docs/collection.md`、`docs/components.md` | 功能、配置、恢复和保留策略边界 |
| `docs/verification.md` | 已执行 fixture/CLI/浏览器与仍缺实机验证的界线 |

## Findings

### 1. 已有能力，不应再列为全无

| 能力 | 结论与代码依据 |
| --- | --- |
| BT 下载与管理 | 已有 qB 状态/添加/文件清单/选集/限速/启停对账/入库重试及实际条目跳转。`internal/app/downloads.go:32–43`。移除保留文件，非自动清库。`internal/app/downloads.go:532–545`。 |
| 搜种 | 已有独立状态和搜索 API，非只有手填磁链。`internal/app/torrent_search.go:104–107`。 |
| RSS 与自动订阅 | 已有来源刷新、规则预览、人工补查、候选忽略/获取和任务事件。`internal/app/collection_api.go:18–43`。持久调度在 `internal/app/collection_rules.go:195`，规则唯一、季集明确、未重复时自动排入原 BT 流程。`internal/app/collection_rules.go:157–180`。 |
| 本地/远程文件导入 | 本地 `register` 与 `copy` 是明确区分的路径，`internal/app/collection_preview.go:159–179`；真实 rclone `copy --immutable` 只复制预览清单，不删除来源，`internal/collectionworker/tools.go:463–490`。 |
| 网页视频和提取音频 | 有真实工具与能力检测，不是链接收藏。`internal/collectionworker/service.go:275–290`；输出按视频/音频策略选择，`internal/collectionworker/tools.go:251–275`。 |
| 完成后自动扫描并确认入库 | 主流程核验产物版本，安排扫描，并在全部目标条目实际覆盖后保存 itemIds 和 `imported`，不是单凭下载 100%。`internal/app/collection_import.go:163–240`。有独立 `retry_import`，不需要重复复制或下载。`internal/app/collection_runtime.go:284–296`。 |
| 外部自动化管理与 MoviePilot 整理/刮削 | 四服务有 API 检测、项目、选项、查找、持久动作和对账路由。`internal/app/collection_external_api.go:17–24`。MoviePilot 有 search/releases/files/organize-preview/scrape-preview/downloads/roots/library，`internal/app/collection_moviepilot.go:12–22`。 |
| 四固定组件部署 | 已有部署/启动/任务恢复/重试接口，`internal/app/collection_components.go:11–16`。支持 MoviePilot、AutoBangumi、Sonarr、Radarr 白名单，`internal/enginecontract/collection.go:21–31`。不能把它描述成“尚无安装入口”。 |

### 2. P1：外部自动化到本地媒体库的流程仍有人工断点

**状态：部分具备。** 本系统自身的下载/worker 获取会自动发布并扫描；外部服务的订阅、下载和复制整理完成后，仍需另行对完成目录预览和登记。界面直接说明这一点：`web/src/components/CollectionExternalPanel.vue:322`，MoviePilot 下载/整理确认也强调单独导入，`web/src/components/CollectionMoviePilotWorkflow.vue:136`、`:174`。首次部署后还需要用户在组件中配置账号、站点、下载器及整理目录，`internal/enginecontract/collection.go:9`、`docs/components.md:19–35`。

**影响：** 已能把资源取得，但“追更完成 → 栖库中可播放”尚不是所有来源统一的一次配置流程；新用户容易停在“外部已完成、此处找不到”。

**建议验收：** 在首次设置中明确执行端、下载目录、完成目录和目标库映射，展示连通性/读写/空间检查；对用户明确授权的外部完成目录建立自动发现与受控导入策略，保留实际文件版本与目标覆盖核验。不要自动接管 qB 任务或将未知写操作盲目重发。

### 3. P1：任务异常与存储风险缺少可配置的后台通知渠道

**状态：未发现产品级通知中心、邮件/Webhook 配置或发送器。** 当前收集中心依赖页面查询，`web/src/components/CollectionCenter.vue:108–130`；任务失败/需要对账会保留状态并跳过自动执行，`internal/app/collection_runtime.go:35–38`。源码中的 `notify()` 是本地 UI 提示或内部唤醒，不是远程通知：例如 `internal/app/download_sync.go:47–48`。

**影响：** 无人值守 NAS 场景下，用户关掉网页后难以及时得知失败、待选文件、入库异常或空间问题。

**建议验收：** 先做站内未读事件 + 一种显式配置的 Webhook；覆盖获取失败、入库失败、待人工确认、组件离线和磁盘低水位；有脱敏、合并、重试上限、去重与测试通知。通知不自动扩大业务权限。

### 4. P1/P2：任务与产物的长期容量治理尚未产品化

**状态：部分具备。** 已有单任务文件/字节预算、磁盘空间保护和版本核验；但是保留策略固定保留任务与产物，worker 达 10000 条停止接收，归档要求停服后管理员维护，`docs/collection.md:123`。任务动作只有 `cancel/retry/retry_import`，`internal/app/collection_runtime.go:242–298`，没有在线保数据归档或受控暂存清理。BT 移除保留下载文件，`internal/app/downloads.go:532`。

**影响：** 长期运行后，历史和副本不断积累；用户很难知道哪些空间可安全回收。此问题区别于“缺少单任务磁盘保护”。

**建议验收：** 容量概览、按来源/任务区分暂存/完成/已入库/做种数据，保数据归档与过期预览/失败暂存清理；每次真实删除前有精确预览和归属/引用检查。默认不删除原媒体、做种数据、秘密和状态目录。P1 做可见性与告警，P2 做清理执行。

### 5. P2：组件生命周期只到部署、启动和失败重试

**状态：部分具备。** 公开组件路由只有 status/deploy/start/task/retry，`internal/app/collection_components.go:11–16`；版本由固定模板控制，`internal/enginecontract/collection.go:5`。停止完整模式/stop-all 另有路径，不能说完全不能停止。但没有组件级受控升级、升级前检查、配套数据备份、保数据卸载与兼容回滚流程。文档明确本轮不提供卸载、清库或任意镜像安装，`docs/components.md:50–56`。

**建议验收：** 先展示实际/目标版本与兼容状态，再做单组件停止/重启、升级计划、备份提示、失败保留诊断和保数据卸载。升级需根据真实 schema 迁移能力决定能否回滚，不能许诺任意版本无损降级。

### 6. P2：自动化规则覆盖和易用性仍有限

**状态：部分具备。** 原生规则目前绑定 RSS 来源，`internal/app/collection_api.go:352–355`；自动模式要求关键词、季集正则和唯一文件选择正则，`internal/app/collection_api.go:348–350`；画质匹配是文本字段，`internal/app/collection_models.go:53–60`。当作品或季集不明确、多个发行或绝对集数未映射时转人工，`internal/app/collection_rules.go:167–180`。

**建议验收：** 先给可解释的规则模板与命中预览、完整作品/季集身份映射、缺集视图和跨来源候选去重；画质/语言/字幕优先级与替换升级策略另做明确授权。将安全转人工作为保留能力，不把它当实现缺陷。网页/远程定时获取和跨组件编排应是新范围，不宜隐式扩大现有 RSS 订阅含义。

### 7. P3：通用插件、多市场和飞牛应用兼容仍是独立规划

**状态：未实现，但已有专门计划。** 当前白名单仅四固定组件，`internal/enginecontract/collection.go:21–31`。CodeGraph `plugin` 仅发现 Vue 编译插件导入；对实际控制面/路由/前端的补充检索未发现插件市场、动态能力注册和插件升级卸载协议。已有规划明确本轮不实施，`.trellis/tasks/11-19-plugin-market-fnos-plan/prd.md:5`；后续市场与生命周期验收仍未勾选，`:90–92`。

**建议：** 优先稳定媒体消费与恢复再推进此独立计划；从受控容器应用和版本化市场协议开始。固定组件已部署成功不能代替任意 FPK 或跨平台运行兼容。

### 8. P3：网页获取的来源覆盖明确受限

当前是逐条、无 Cookie 上传、无频道/播放列表递归；固定 `--no-playlist`，`internal/collectionworker/tools.go:251–252`。仅 HLS 的来源目前明确拒绝；需要登录、JavaScript 挑战或额外运行依赖的来源也不保证可获取，`docs/collection.md:125`。

如用户有明确需求，再按来源增加安全 HLS 获取、可解释站点诊断或批量队列；不能将“接入 yt-dlp”描述为任意站点均支持。DRM 不是建议补齐的常规功能。

## 当前任务状态不能直接代表功能缺失

- `.trellis/tasks/11-19-collection-file-import/prd.md:15–21`：前三条功能和本机真实 rclone alias 验收已完成，未完成的是目标 NAS 云盘/磁盘不足/跨卷和故障验收。
- `.trellis/tasks/11-19-collection-release/prd.md:15–24`：UI、归属校验、退出和本机回归已实现，未完成 Linux/NAS/真实 Docker 部署验证。
- `docs/verification.md:367–375`：真实 app+worker、rclone alias、隔离 yt-dlp/FFmpeg 和浏览器有证据；真实云盘、公网站点/BT 载荷、NAS ACL 和 Linux 内核运行不能据此视为已验证。
- `docs/verification.md:381–393`：四类外部 API 有 83 项本地协议 fixture 与实际应用浏览器结果；不是真实用户套件兼容验收。MoviePilot 六阶段限定已核验精确版本。
- `docs/verification.md:407–417`：四组件部署入口有 app/helper/浏览器/Compose 规范化证据；四个真实镜像首次初始化、NAS UID/ACL、Docker Desktop 卷和掉电恢复仍需实机验证。这是发布验收缺口，不等于安装功能不存在。

## Related specs

- `.trellis/spec/backend/collection-contract.md`
- `.trellis/spec/backend/external-collection-contract.md`
- `.trellis/spec/backend/collection-component-contract.md`
- `.trellis/spec/backend/download-contract.md`
- `.trellis/spec/backend/torrent-search-contract.md`
- `.trellis/spec/frontend/app-contract.md`

## External references

无。本研究不对第三方项目当前版本、功能或维护状态作新增判断；组件版本和协议范围仅复述当前仓库已锁定的能力。

## Caveats / Not Found

- 这是静态仓库评估，没有运行构建、测试、浏览器或真实 NAS 验收；验证数字均引自仓库历史记录。
- 否定判断限定当前业务路由、模型、UI 和配置，没有将名字搜索未命中当绝对证明。
- 插件规划文档中的早期“数据库 v2、两个引擎”是 2026-11-19 的历史上下文，当前已是 schema v4 和四组件扩展；只有其未实施的通用市场/兼容计划仍可引用，不能沿用旧现状。
- 新通知、自动成品导入、清理和组件升级属于后续建议，本次未实施，也不视为已获得业务变更授权。
