# Research: 多来源资源收集与外部服务集成边界

- Query: 基于前次音视频获取搜索，为当前 NAS 规划影视/动漫 RSS+BT、网页音视频、自有音乐、本地与远程文件导入的首批执行适配，以及成熟自动化套件的后续集成方式。
- Scope: mixed；仅研究与修改计划，不安装、不部署、不访问真实帐号。
- Date: 2026-11-19（Asia/Shanghai）。
- Task directory: 本次显式指定 `11-19-resource-collection-plan`。调研开始时 `.current-task` 仍指向旧插件任务，因此不据此覆盖旧资料。

## Findings

### 已确认的范围调整

用户已选择首期“多种来源一起做”，接受更多工作量。首个产品版本必须完整覆盖影视/动漫 RSS+BT、网页音视频、自有音乐、本地文件与远程文件导入。以下分阶段是开发顺序，不得把网页、音乐或远程导入降为首期之外的功能。成熟自动化套件的深度 API 编排可以后续交付。

### 先前搜索结论如何进入产品

前次官方项目选型保存在：

- `../../11-19-plugin-market-fnos-plan/research/nas-video-acquisition.md`：MoviePilot / AutoBangumi / Servarr / Jackett 的职责。
- `../../11-19-plugin-market-fnos-plan/research/nas-download-import.md`：yt-dlp / MeTube / qBittorrent / rclone 的职责。
- `../../11-19-plugin-market-fnos-plan/research/nas-music-acquisition.md`：自有音乐、beets、Lidarr 和播放服务边界。

本次复核表明：来源、订阅规则、执行器和媒体服务必须区分。RSS 提供更新清单；Jackett/Prowlarr 提供索引搜索；qBittorrent 负责 BT；yt-dlp 保存可访问网页音视频；rclone 复制已有远程文件；beets 整理已有音源；MoviePilot、AutoBangumi 和 Servarr 组合负责更完整的自动化编排。部署这些工具不会自动赋予来源访问权。

### 可行路线比较

| 路线 | 本系统负责 | 外部组件负责 | 取舍与适用性 |
| --- | --- | --- | --- |
| A：内建统一收集中心 + 固定执行适配器（建议） | 来源管理、RSS、候选筛选、任务身份、默认人工确认与显式启用的受控自动选择、作业状态、入库验证、历史 | qB 传输；受控 yt-dlp 网页下载；受控 rclone copy 远程复制 | 可沿用现有 Go/SQLite/下载门禁并覆盖全部首期入口；需要维护作业恢复、工具部署和资源上限。无需等待通用插件市场。 |
| B：外部 MoviePilot / AutoBangumi + 本系统文件导入 | 保存服务连接与打开入口；仅导入外部独立最终目录；补充网页/本地/远程适配 | 外部系统负责影视/动漫搜索、订阅、BT 与整理 | 快速获得成熟订阅体验，但 NAS 无法在未验证 API 时承诺统一任务操作；部署与配置增加，不能把同一 qB 任务同时交两套协调器。适合作为高级用户可选集成。 |
| C：Sonarr + Radarr + Prowlarr + qB + 本系统导入 | 同 B，另负责统一展现与媒体阅读 | 分别负责剧集、电影、索引和传输 | 职责明确、质量规则丰富，但服务数量和双向状态同步成本较高；当前已有 Jackett 接入，首期无须强制替换。 |

建议采纳 A 作为首个产品版本，B/C 先提供外部连接、打开和独立目录导入，后续才按实际服务版本增加只读状态或受控动作。连接模式与托管安装模式分离：首期可连接已部署 qB，yt-dlp/rclone 使用管理员提供并通过版本/路径/能力探测的固定工具，或使用明确支持的固定执行服务；缺少工具时给出可操作诊断，不自动执行网上安装脚本。通用插件市场仅承担后续分发安装，并非统一收集作业的前置条件。

### 覆盖所有首期入口的最小执行适配

| 入口 | 首批实现建议 | 成功条件与失败边界 |
| --- | --- | --- |
| 影视/动漫 RSS、手动磁力、种子、既有 Torznab 搜索 | 新增有界 RSS 拉取和候选池，解析 RSS enclosure/Torznab；默认候选由用户确认，再转交现有受管 qB 添加、元数据与选文件流程。首期方案也允许用户明确启用受控自动提交与自动选文件，前提是规则和真实文件清单无歧义 | RSS 初次添加先建候选清单，默认不批量下载历史全集。自动模式实施前必须修订 download contract，先完成真实元数据/文件清单核验，再锁定 selection、清单版本和操作 revision，并记录命中规则；含糊标题、季包、多版本或无法唯一选择的文件转人工。仅人工确认是复杂度较低的备选，并非首期硬限制；上述自动能力仍属规划。 |
| 单个网页视频或音频 | 受控 yt-dlp 子进程，服务端固定参数模板，输出到每作业独立 staging，单 URL/单项为初版，明确不支持首期任意播放列表/频道递归 | 确认实际输出清单、工具退出与后处理完成，再发布。受支持站点只是候选能力，真实链接仍需测试；需要 FFmpeg、JS 运行时或登录而未配置时显示原因。重试不能仅按原始 URL 去重。 |
| 本地视频、已有音源/专辑 | 从已授权目录创建导入作业；可复用已有只读扫描注册，不强制复制；需要归档副本时走独立 staging 复制并验证 | 不改写源文件；不把元数据识别当作已经取得音源。音乐沿用本系统扫描/时长/内嵌封面/播放能力。 |
| 自有远程文件与音乐 | 管理员预设 rclone remote 和允许的源路径；使用受控 `rclone copy` 复制选定文件/目录到本地任务 staging | 默认 copy，不开放 sync/move/delete。copy 不删除目标已有文件，但并非“绝不覆盖”；任务隔离目录、重名冲突策略和发布检查仍不可省。远程 mount 视图不是本地取得文件的证据。 |

命令都由固定可执行文件与类型化参数数组启动，不拼接 shell；不向普通页面暴露任意 yt-dlp 参数、`--exec`、rclone 配置、远程 URL、任意工具路径或目录模板。来源 URL 与凭据需分开存储，日志和页面脱敏。网络媒体工具可跟随重定向并访问子资源，不能只校验初始 URL 就宣称具备完整网络隔离；首期工具身份/网络范围应明确，公开网页模式阻止访问本地/元数据等保留网络，内网远程存储使用显式允许的独立来源配置。

### 官方接口与部署事实复核

1. **yt-dlp**：官方 README 记录 `--ignore-config`、`--no-playlist`、`--paths`、`--continue`、`--progress-template`、`--download-archive` 等能力；FFmpeg/ffprobe 用于合并与多种后处理，当前还推荐 yt-dlp-ejs 与受支持的 JS 运行时。固定参数应关闭外部配置读取；继续传输与 archive 是执行优化，业务身份、选择、持久状态和失败分类仍需由 NAS 保存。工具更新会改变 extractor 行为，支持矩阵必须针对锁定版本实际验收，不能保证任意平台全部可下载。
2. **rclone**：官方 `copy` 文档明确不删除目标文件，跳过相同文件的判断由大小/时间/摘要等条件决定；不应把这些条件当作强内容唯一标识。RC 有 `_async`、`job/status`，但完成异步作业默认仅保留 60 秒、按默认 10 秒间隔检查过期。因此首期 CLI 作业或未来 RC 作业都必须在 NAS 中持久化状态、结果清单与执行身份；RC jobId 不能单独作为可永久恢复的业务作业。RC 如采用，应配置认证、TLS 与网络限制。
3. **MoviePilot**：官方 README 明确订阅、搜索、下载、整理、刮削、媒体库刷新等功能，后端基于 FastAPI，提供公开 REST API 文档入口；Docker 是推荐部署方式。官方 API 文档可访问到 Swagger UI。此证据证明存在可研究的 API，不证明某 NAS 实例的路由、认证、错误语义或能力版本相同；接入时以实例 OpenAPI/固定版本适配测试为准。先做外链和独立完成目录导入比直接接管现有下载可靠。
4. **AutoBangumi**：官方 README 明确 RSS、qBittorrent/aria2、重命名/硬链接等扩展能力，当前源码 `backend/src/main.py:130` 注册 `/api`，`module/api/__init__.py:26` 声明 `/v1`，README 记录 `/api/v1/events/stream` SSE。只能据此声明 REST/事件能力存在，不能依赖默认分支长期稳定；首次仅导入其独立完成目录，后续再锁定版本接状态和动作。
5. **Sonarr / Radarr / Prowlarr**：官方提供 API 文档/源码，可做后续固定服务适配。当前 Sonarr 官方 API 文档页面同时指向 develop 的 V5 OpenAPI 和 main 的 V3 OpenAPI，说明不能简单把所有部署写死为 `/api/v3`。针对用户实例识别版本、能力和认证，先只读后动作。Prowlarr 提供 Torznab 时可复用现有搜索入口，不需要立刻增加它的管理 API 或替换 Jackett。
6. **MeTube**：官方源码 `app/main.py:1081` 有 `add` POST，`app/main.py:543` 使用 Socket.IO，存在可以研究的集成表面；但当前未找到稳定版本化的外部 API 契约保证。首期建议单一受控 yt-dlp 适配器满足网页入口；MeTube 作为可选外链与目录导入，防止维护两套网页下载任务状态。若用户已经部署 MeTube，仍需单独验明其版本、鉴权和历史恢复方式。
7. **beets / Lidarr**：beets 是自有音乐导入与标签/目录整理，可 copy/move/write，因此只能对专门整理副本启用，不能修改本系统已有只读源。首期自有音乐扫描与远程 copy 已能覆盖音乐获取入口。Lidarr 自动补专辑及其外部元数据连通性可作为后续选项，不能作为首期自有音乐导入的依赖。前次记录的 Lidarr 元数据故障状态本轮不重新宣称当前仍成立，具体接入时重新核验。

### 多工具共存的身份、目录和恢复边界

- 本系统已有 qB 任务通过持久添加意图、受控目录和算法身份绑定，不得因为 hash 相同就认领外部任务；该事实来自下载契约。继续以现有协议为权威。
- 多个订阅工具可以连接同一 qB，但必须配置彼此隔离的分类、标签和路径，并确认外部整理器不会扫描/移动 NAS 自己的 staging/complete。单靠标签不足以证明所有权。
- 外部完整媒体导入是新输入来源，不是把其 qB 作业改成 NAS 受管任务；没有外部完成清单/标记/受信事件或等价可验证完成机制时，文件大小短时稳定不能证明完成，自动导入应等待明确完成条件，或仅允许管理员选择最终成品目录手动导入。
- BT 传输、网页后处理、远程复制和入库状态必须分离。入库扫描失败可独立重试，避免重新下载；已有相同内容仅凭标题不能跨来源自动合并。
- 统一任务中的外部 operationId 与本地 taskId 分开保存。网络调用/子进程启动之前落盘意图，重启先核对旧进程/目标目录/清单，再决定恢复或标为待处理，不以“超时”等同“未提交”。
- 所有受控工具必须纳入应用启动/取消/退出的进程树归属；Windows 使用既有受控 Job/句柄回收模式，不按进程名终止。下载停止后保留进度和文件，移除历史不等于删除来源。

## Files Found / Code Patterns

- `.trellis/spec/backend/download-contract.md`：qB 权威、添加意图、算法身份、明确选文件、staging/complete 发布门禁、不可接管外部任务、入库独立恢复。
- `.trellis/spec/backend/torrent-search-contract.md`：当前是手动 Torznab，明确排除 RSS 与自动载荷下载；实施 RSS 前要显式扩展契约，而不是偷偷改变原搜索语义。
- `.trellis/spec/backend/runtime-contract.md`：当前 SQLite v2、单连接、不持有事务跨网络/文件操作、异步有界增强、原媒体只读、音乐格式和发布规则。
- `.trellis/spec/backend/cross-platform-contract.md`：Linux/Windows 生命周期、绝对工具路径、安全工具发现、Windows 进程树与退出文件释放、受控发行与 Desktop Compose 严格校验。

仓库具体改动文件由本任务 `repository-collection.md` 负责核实，此研究不重复做大范围代码检索。

## External References

本次于 2026-11-19 尝试 web.run 官方站点查询/打开；工具未返回可读正文或可复用 citation 标识，故不伪造 web 引用。随后通过 `python -B scripts/project_env.py -- python -B -` 中的 urllib 只读官方 README、API 入口与文档，提取上述接口和参数证据；未执行上游程序、未把工具安装到本机。引用默认分支的源码只能说明此次观测，实施应锁定版本或 commit。

- yt-dlp README：https://github.com/yt-dlp/yt-dlp/blob/master/README.md
- rclone copy：https://rclone.org/commands/rclone_copy/
- rclone RC：https://rclone.org/rc/ （`_async`、`job/status`、`--rc-job-expire-duration`、认证）
- MoviePilot README：https://github.com/jxxghp/MoviePilot/blob/master/README.md
- MoviePilot REST 文档：https://api.movie-pilot.org
- AutoBangumi README：https://github.com/EstrellaXD/Auto_Bangumi/blob/main/README.md
- AutoBangumi FastAPI 入口：https://github.com/EstrellaXD/Auto_Bangumi/blob/main/backend/src/main.py
- AutoBangumi 版本路由：https://github.com/EstrellaXD/Auto_Bangumi/blob/main/backend/src/module/api/__init__.py
- Sonarr API：https://sonarr.tv/docs/api/
- Radarr API：https://radarr.video/docs/api/
- Prowlarr API：https://prowlarr.com/docs/api/
- MeTube 源码：https://github.com/alexta69/metube/blob/master/app/main.py
- beets 配置：https://beets.readthedocs.io/en/stable/reference/config.html

## Caveats / Not Found

- 未部署上述工具、未调用真实 NAS、索引器或个人凭据；不能保证用户实例的接口、目录、媒体格式、站点访问或 CPU 架构兼容。
- 本次不确认最新版本号、维护活跃度、下载来源覆盖、收费条件或飞牛原生 FPK；已有工具推荐不等于完成安装/集成。
- 路线 A 的 RSS、网页下载、远程导入、配置编辑、统一任务需要真实开发；当前 repo 的“RSS 不支持”等范围限制必须与新 PRD 一并修订，不能把规划能力当作现有能力。
- 来源 ID、remote 名称和外部应用 API 都需要版本化配置与认证处理；不能把元数据、API 文档可访问或短时文件稳定当成取得完整资源的证明。
