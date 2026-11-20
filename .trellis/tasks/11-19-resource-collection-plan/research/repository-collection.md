# Research: 资源收集功能的仓库现状与修改边界

- Query: 当前资源发现、下载、扫描、音乐和部署能力如何复用，影视/动漫 RSS+BT、网页音视频、自有音乐、本地/远程文件导入同时纳入首期需要修改哪些模块？
- Scope: internal；仅代码与契约研究，没有实施、构建、测试或外部服务连接。
- Date: 2026-11-19
- Task directory: `.trellis/tasks/11-19-resource-collection-plan/`，由父代理显式指定。读取时 `.trellis/.current-task` 仍指向旧插件计划，本研究未改动该指针或旧任务。

## Findings

### 1. 检索与总体结论

- 已优先使用现有 CodeGraph 索引：`python -B scripts/project_env.py -- D:\adunsoftware\nodejs\node.exe D:\adunsoftware\nodejs\node_modules\@colbymchenry\codegraph\npm-shim.js query <symbol> --json`，查询 `download`、`enqueueScan`、`torrentSearch`；没有重建索引。
- 用 `rg` 和带行号源码核验路由、配置、Vue 和部署校验。CodeGraph 索引记录时间可能早于本次读取，结论以核验源码为准。
- 当前系统已实现“手工 Torznab 搜索 → qB 受管添加 → 获取清单 → 用户选文件 → 下载 → 验证发布 → 扫描实际覆盖 → 原播放器”的 BT 闭环。
- 没有 RSS 订阅调度、网页音视频获取、远程文件复制、媒体上传、跨来源候选收件箱或统一收集任务。现有 multipart 仅上传 `.torrent`，不能表述为已有音乐/媒体上传。
- 用户最新选择是首期同时验收多种来源入口；后续可以拆 PR，但不能把网页、本地/远程文件或音乐入口移出首期。通用插件市场和飞牛执行兼容仍可独立，不是完成这些业务入口的前置依赖。

### 2. 当前能力与代码依据

| 能力 | 代码依据 | 可复用点与限制 |
| --- | --- | --- |
| 聚合搜种 | `internal/app/torrent_search.go:104`、`:124`；`torrent_search_torznab.go:42` | 已有已配置 Jackett/Prowlarr Torznab 通用关键词搜索；来源部分失败保留其他结果；没有订阅轮询。 |
| 有界搜索快照 | `torrent_search.go:17`、`:69`、`:80`、`:164`、`:172` | 最多 500 条结果、10 分钟 TTL、8 批快照、2 个搜索槽、3 个来源并发；可复用有界网络/脱敏思路，不能把内存令牌作为持久候选身份。 |
| 来源配置 | `torrent_search_config.go:36`、`:53`、`:63`；`config.go:87`；`engine_runtime.go:238` | 来源为服务端只读 JSON，1–8 个，托管引擎可更新运行时配置；没有通用网页可编辑来源 CRUD。保留外部文件和托管来源的原语义，新增可写来源不能静默覆盖。 |
| 私密下载解析 | `torrent_search_download.go:69`、`:94`、`:122` | 后端解析缓存 magnet 或已配置同源下载代理；15 秒预算、种子 8 MiB 限额。浏览器没有私密 URL/API Key。RSS 私密凭据也应留在后端。 |
| 添加与幂等 | `downloads.go:23`、`:264`、`:361`、`:717` | JSON source XOR，或单个 `.torrent`；先持久登记操作再调用引擎，有 requestId、revision 和重试归属；应提取受控服务方法供收集模块调用，避免通过本机 HTTP 绕圈。 |
| 下载数据模型 | `download_models.go:34`、`:62`、`:72`、`:136` | `Download`、文件清单和操作表已有独立传输/入库状态。BT 专用 identity/hash/selection 字段不能强塞网页或文件复制任务。 |
| BT 身份与归属 | `download_models.go:151`、`:152`；`download_sync.go:194`、`:200`、`:449` | v1/v2/hybrid 身份去重；受管 tag、受控目录和身份共同验证，不接管第三方外部任务。订阅去重需要在这层之外增加条目身份和作品/剧集判断。 |
| 选文件 | `downloads.go:563`；`download_sync.go:920` | 清单版本、实际 file index、锁定 selection 和乐观 revision。当前必须用户确认；自动模式必须作为明确新增策略设计。 |
| 完成发布 | `download_import.go:15`、`:45`、`:71`；`download_gate.go:67`、`:267`、`:399` | 选中文件完成、稳定状态、最终位置、常规文件/路径/大小和纳秒 mtime 核验后才 ready；所有扫描/封面/读取受 gate。不能以 qB 100% 或 callback 代替发布。 |
| 入库可靠性 | `download_import.go:237`、`:251`、`:264`、`:280`、`:285`、`:300` | 独立入库状态；检查实际 items 覆盖；复用活跃 scan 并安排有界补扫；入库失败可重试而不重下。新获取器应共享同等级发布/扫描覆盖要求。 |
| 本地媒体扫描 | `libraries.go:168`、`:263`、`:281`；`jobs.go:222`、`:401`、`:448` | 已有允许目录浏览、创建媒体库、手工/定期扫描；`enqueueScan` 复用唯一活跃扫描；适合首期本地文件“登记目录/扫描”入口，不必复制现有只读媒体。 |
| 音乐播放与探测 | `music.go:17`、`:63`、`:77`、`:115`；`jobs.go:457` | 已支持常见音频、异步时长/封面，原生 Range 播放；没有艺人/专辑元数据模型或 beets 标签写入。首期自有音乐可作为文件导入的 `music` 分类闭环。 |
| 后台 jobs | `jobs.go:26`、`:58`、`:109`；`store.go:91` | 当前 worker 分支只有 scan 与 enrich，重启把 running 改回 queued；网络轮询/大文件复制不能直接塞进 jobs，否则非 scan 会误走 enrich，也会阻塞媒体处理。 |
| 现有定时机制 | `jobs.go:34`；`download_sync.go:84`；`engine_runtime.go:55` | 媒体库定期扫描、BT 对账、助手连接刷新各自独立；没有订阅调度器。新协调器应受 App.ctx / App.wg 管理，并持久保存 next_run、尝试/退避和不确定提交状态。 |
| 数据库 | `store.go:28`、`:33`、`:41`、`:45`、`:70`；`:104` | SQLite WAL、单连接、外键和忙锁预算；当前 schema v2，较新库被拒绝。新收集表应增量迁移，不能重建 media/user/progress；网络/文件操作不得持有事务，必须及时关闭 rows。 |
| 认证与请求边界 | `server.go:237`、`:253`、`:264`、`:279`、`:290`、`:308`、`:326` | 管理 API 会话鉴权、同源变更、严格 JSON、一般请求 64 KiB。媒体上传若另做必须独立端点/预算；不能扩大所有 API 请求额度。 |
| 运行与退出 | `server.go:111`、`:116`、`:137`、`:147`、`:149` | 管理器在数据所有权和存储完成后启动；退出取消、关闭连接并等待 handler/worker 后关闭 DB。收集轮询、复制和子进程均需加入该生命周期。 |
| 设置与备份 | `server.go:461`；`media.go:76` | settings 仅回显启动配置；`VACUUM INTO` 备份主数据库。可写来源/订阅需要新 API；源密码、工具配置、文件和 worker 状态若不在主库须单独说明备份恢复。 |

### 3. 已有 UI 及建议增量

- `web/src/types.ts:2` 的 ContentType 固定 `video | image | comic | novel | music`；`:4` 的 ViewName 已含 downloads/jobs/settings。资源收集是管理视图，不增加第六种媒体类型。
- `web/src/App.vue:15` 懒加载下载中心，`:23` 的 secondaryNav 同时供桌面侧栏和移动 More；`:74`、`:187` 附近具有视图描述与 hash 恢复逻辑。建议增加 `collection` 管理视图，保持移动底部最多五项。
- `DownloadCenter.vue:13`、`:227`、`:323` 已有搜索到添加弹窗的 handoff；应复用 TorrentSearchDialog 和 AddDownloadDialog，或把搜索组件拆为可内嵌内容，避免嵌套原生 dialog。
- `DownloadCenter.vue:90`、`:102` 以同源 API 获取安全任务快照，单请求轮询和离线速度提示已有模式；新增“收集状态”和“传输/入库状态”应分开，不造统一伪百分比。
- `AddDownloadDialog.vue:102` 明确提示“这里只上传种子，不上传媒体”，不能把这个入口当成本地音乐导入。
- `App.vue:419` 设置页解释环境配置且无写入接口；新增来源页应有连接测试、能力/可用性、最近成功、脱敏错误和停用操作；凭据不回显。
- `web/src/api.ts:14`、`:31`、`:42` 提供 typed JSON/multipart、会话失效和 requestId；新 DTO 仍放 types.ts，并更新 docs/api.md。

### 4. 推荐的增量模块和实体（建议，未实施）

最小职责划分，不要求无关架构重构：

```text
internal/app/collection_models.go       来源、候选、订阅、规则、收集任务及迁移
internal/app/collection_api.go          管理路由、DTO、CRUD、预览、确认、重试
internal/app/collection_runtime.go      有界协调器、持久调度、退避、恢复/对账
internal/app/collection_rss.go          RSS/Torznab 收取、条目去重、游标/首次启用策略
internal/app/collection_rules.go        关键词/排除/范围/质量规则与命中解释
internal/app/collection_bt.go           调用现有 BT 受控提交/选集流程的桥接
internal/app/collection_import.go       本地目录与外部获取产物发布、导入覆盖
internal/collectioncontract/            worker 的固定任务/状态/产物清单协议
internal/collectionworker/              固定网页下载/远程复制执行器，单独权限与目录
cmd/nas-collection-worker/              worker 入口（若采用独立执行进程）
web/src/components/CollectionCenter.vue 统一入口、候选收件箱、订阅与任务
web/src/components/CollectionSourceDialog.vue
web/src/components/CollectionSubscriptionDialog.vue
web/src/collection.ts                   typed 帮助函数与中文状态
docs/collection.md                     用户部署、来源、目录、恢复与限制
scripts/verify_collection.py            可复用集成验证（具体实施时再添加）
```

建议新增 `collection_sources`、`collection_candidates`、`collection_subscriptions`、`collection_runs`、`collection_tasks`、`collection_task_events`，并按需拆独立 rules 表。字段至少包括类型/来源修订、稳定外部身份、secretRef、显示元数据、ruleRevision、调度/退避、决定/跳过原因、requestId、revision、下载或 worker 任务引用和最终 Item 关系。

- 候选身份先用 `sourceId + stable provider ID/GUID`；来源无可靠 ID 时使用规范化字段摘要并标记弱身份，BT 最终再按完整算法身份去重。作品名相同不代表同一发行，不能只按标题自动丢弃。
- 订阅不可长期保存现有临时 `searchResultId`。私密附件应以来源/远端稳定引用重新解析，或使用受限私有持久源存储，后者必须有过期、权限、清理和备份说明；公开 DTO 仅有随机本地 ID。
- 可展示“命中待确认”为默认策略，自动提交/自动选集属于新增显式策略；集合包、季包、集数不明、多版本冲突可转人工。首次启用默认只建基线，历史回填另作显式操作。
- 去重检查已有任务和已入库条目；跨来源无足够身份时提示冲突，不按推测静默跳过或替换。源停用不取消已接收的下载和完成导入任务。
- 新总 schema 版本必须与另一个尚未实施的插件计划统一分配：当前可称 v2→v3 建议，不能让两个独立任务都直接占用 v3 后互相覆盖。

### 5. 建议 API 边界

- `GET /api/collection/capabilities`：各来源类型与执行环境真实可用性。
- `/api/collection/sources`：创建/编辑/停用来源；独立测试动作只做连接与能力探测，不抓取历史、不开始下载。
- `POST /api/collection/sources/{id}/refresh`：明确触发有界收取，持久记录 run；刷新来源不代表批准全部候选。
- `/api/collection/candidates`：分页、命中/跳过理由、安全详情；动作如确认获取、忽略、重试，不允许客户端给任意文件落盘路径或 shell 参数。
- `/api/collection/subscriptions`、`/{id}/preview`、`/{id}/run`：规则预览与启停；变更带 revision，首次启用行为明确。
- `/api/collection/tasks`、`/{id}/events`：收集/worker 任务、BT task 引用、入库结果分层展示。
- 本地入口提交已授权目录 ID/相对路径和内容分类，可复用现有库/扫描；远程入口只能选预配置 remote 和路径，首期采用单向复制，目标由服务器生成。
- 若首期含浏览器上传自有音乐，应另设流式媒体上传端点、单文件/总量/磁盘预算、临时目录与重启恢复，不复用 torrent 上传。若采用现有本地目录选择满足本地音乐入口，应在 PRD 明确无浏览器文件上传承诺。

### 6. 网页 worker、rclone 与当前部署的实际影响

**不能直接塞进现有 engine helper。**

- `internal/enginehelper/templates.go:22` 仅接受 qB/Jackett；`:202` 限 1–2 个固定组件；`internal/enginecontract/models.go:21` 也明确两个组件。现有 helper 是安装固定引擎的控制面，不是下载执行器。
- `Dockerfile.engine-helper:12`–`:23` 是 scratch + 单 Go binary，无 shell、Python、yt-dlp 或 rclone。`service.go:146` 明确仅 Linux。把网页/remote 任务发给现有 helper 没有可执行基础。
- 推荐新增固定协议的 collection worker，可直接调用钉定工具的 argv（不经 shell），只写独立 collection staging/complete，且不持有 Docker socket；保持已有 helper 专管 qB/Jackett。网页/remote 工具升级与能力探测独立，主程序只管任务和产物校验。
- 若选择主进程调用本地工具，仍需独立工具解析器和能力模型；`media_tools.go:36` 只允许 ffmpeg/ffprobe，不能假设可运行任意工具。可复用 `mediaProcess.Run` 的 owned-process-tree 清理设计，但新长期下载进程需持久恢复、退出和不确定结果对账。

**只改 Compose 不会在 Windows Docker 模式生效。**

- `internal/desktop/docker_compose.go:94` 检验恰好两个 service；`:132`–`:140` 检验精确 environment 和七个角色卷；`:156` 检验每个 mount。新增 worker、数据卷、秘密或 NAS_COLLECTION_* 参数都会被当前验证器拒绝。
- `internal/desktop/docker.go:390`、`:412`、`:447`、`:501` 同时检查运行中容器/挂载/卷归属，`:690` 附近有完整 stop。新增 collection worker 必须同步修改“声明模板 + 规范化 Compose 验证 + 实际运行资源验证 + 镜像清单/发布 + 启停/健康 + 完整退出”。不能只放宽 service/volume 数量而不校验具体角色。
- `compose.desktop.yaml`、`compose.engines.yaml`、`compose.yaml`、`internal/desktop/config.go`、`internal/desktop/docker.go`、`internal/desktop/docker_compose.go` 和发布打包脚本应作为同批文件影响项。保持基础单服务部署有效，collection 增强以明确 profile/overlay 开启。
- collection staging/complete 不能借用 qB 的受管目录/manifest，也不能让 worker 改写发布过的 BT 文件。主服务仅需只读访问已发布产物；worker 才有专用写权限。所有媒体扫描/直接读/封面入口需识别 collection 暂存与未发布区域，不能假设当前 download gate 自动覆盖新目录。
- 建议 collection 独立卷：worker_state、collection_data、collection_secrets/IPC 等角色按实际架构收敛；它们必须有实例标签、明确归属和备份边界。主应用不获得 Docker socket。
- Windows 原生可以提供规则/来源/本地扫描和远程执行器连接，但网页/remote 工具必须按实际可用路径与版本探测；缺工具时显示未配置，并给出配置动作。若首期宣称原生模式支持这些获取，必须交付匹配架构工具及真实进程树退出验证，不能以 Linux 容器成功替代。
- 当前 Windows Docker 仅本机 Docker Desktop Linux amd64（`docker.go:196`–`:221`、`:345`）；fnOS/ARM NAS 要单独验证 worker 镜像架构、UID/ACL、秘密和目录映射。支持容器并不等同飞牛原生 FPK 兼容。

### 7. 必须显式修订的现有契约

| 现有契约 | 当前禁止/限定 | 新功能应如何修订 |
| --- | --- | --- |
| `backend/torrent-search-contract.md` Scope/3 | 不做 RSS、自动下载或新 schema；来源和搜索结果不持久化 | 保留手工搜索原边界，新增 collection contract；仅把共享 Torznab 解析/受控提交抽为服务，不使每次搜索自动写入/下载。 |
| `backend/download-contract.md` 3 | 用户锁定选集、原媒体不可改、无 RSS、不接管外部同 hash、仅三种来源入口 | 明确 collection 可创建受管 BT 的桥接和可选自动选集授权；保留所有归属、意图持久化、选文件版本、发布门禁和保文件移除。 |
| `backend/runtime-contract.md` | 源媒体不改写、媒体读取受界、原五类类型 | 本地登记继续只读；新产物以专用写目录和发布契约处理；beets 不直接改现有媒体。 |
| `backend/database-guidelines.md`/`runtime-contract.md` | 当前 v2 和单连接、短事务 | 增量迁移并更新最大版本；数据升级/一致性快照/恢复方法一起更新。 |
| `backend/engine-bootstrap-contract.md` | 固定两个引擎，私有 helper 权限 | 不把 collection 变成任意插件执行；如果改模板/协议需版本化兼容和完整原有回归，优先独立 worker。 |
| `backend/cross-platform-contract.md` | 两 service/七卷/精确配置；只有 owned 资源和完整停止 | 新 profile 的每个明确角色同步验证，并测真正退出、文件可替换和恢复，不删数据卷。 |
| `frontend/app-contract.md` | 五媒体类型/五底部入口、同源 API、原生 modal、严格轮询 | 新增管理视图与 More 项，外部来源不直接浏览器请求，不嵌套 modal，不展示私密链接。 |

### 8. 首期同时覆盖多入口时的建议验收

1. RSS/BT：首次基线、手动历史回填、重复/改 GUID/跨来源重合、无匹配、部分来源失败、外部 hash 冲突、暂停后恢复、磁链元数据安全选集、重启不重复添加；最终 Item 确认。
2. 网页：合法受支持链接产物、格式不可用/站点变化、断线/取消/进程崩溃、同一 URL 重复、工具缺失、磁盘不足；完成前任何 API 不可读，完成后真实可播放。
3. 本地/音乐：已授权目录登记与重扫、只读原文件、不复制无必要大文件、音乐格式与封面/续播、路径/符号链接/离线根不误清索引。
4. 远程文件：已配置 remote 选择、单向 copy、凭据失效/部分复制/重启/重名、目标已存在不覆盖、目录映射和检查；源文件不删除，失败不误入库。
5. 发布/导入：BT 与非 BT 各自完成证据、mtime/size 被改冻结、扫描已过新目录补扫、库被删不重建、失败只重试导入、备份恢复后未完任务先对账。
6. 部署：Linux 容器和目标 NAS、Windows Docker 的精确资源校验和完整停止；原生模式按宣称能力验收。原五媒体、认证、5+ GiB Range、搜索/BT/引擎初始化保留回归。

## Related Specs

- `.trellis/spec/backend/runtime-contract.md`
- `.trellis/spec/backend/download-contract.md`
- `.trellis/spec/backend/torrent-search-contract.md`
- `.trellis/spec/backend/database-guidelines.md`
- `.trellis/spec/backend/engine-bootstrap-contract.md`
- `.trellis/spec/backend/cross-platform-contract.md`
- `.trellis/spec/frontend/app-contract.md`
- `.trellis/spec/guides/cross-layer-thinking-guide.md`

## External References

本文件没有新外部兼容性结论。此前候选工具研究见 `../../11-19-plugin-market-fnos-plan/research/nas-video-acquisition.md`、`nas-download-import.md`、`nas-music-acquisition.md`；本次外部协议和工具选择依据由同任务其他研究文件记录。

## Caveats / Not Found

- 本研究只支持规划，不代表上述新增实体/API/worker 已实施或任一真实来源已验证。
- 没有执行 Git 操作、构建、测试、安装、Docker 命令或连接真实来源；没有生成测试类。
- 一次把 `torrent_search*.go`/`download*.go` 作为 Windows literal path 的 rg 调用失败，随后改为 `rg internal/app -g ...` 成功覆盖；没有影响最终结论。
- 不应把“支持所有类别的入口”误表述为“每个站点/云盘都可用”；需要按首期明确选定协议/工具及真实平台逐项验收。
