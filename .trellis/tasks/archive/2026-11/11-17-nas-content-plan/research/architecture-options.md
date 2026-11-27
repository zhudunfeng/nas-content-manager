# Research: NAS 内容管理架构与成熟方案对比

- Query: 为单台 NAS、个人或家庭少量用户的视频/图片/漫画/小说系统选择架构、处理链路和存储方案。
- Scope: mixed（仓库现状与官方公开资料）
- Date: 2026-11-17
- Status: 建议与暂定假设，未经用户确认，不代表已批准实施。
- 用户补充：易使用、易部署；据此将单一应用镜像、单服务 Compose 与内置 SQLite 调整为首选。
- 用户再次明确：内容共四类，漫画是独立类型，不能并入普通图片相册入口。

## Files found

- `AGENTS.md`：中文注释、显式类型、关键中文日志与 Trellis 任务规则。
- `.trellis/tasks/11-17-nas-content-plan/prd.md`：本轮目标与暂定场景。
- `.trellis/workflow.md`：方案和研究必须持久化。
- `.trellis/spec/backend/index.md`、`.trellis/spec/frontend/index.md`：均为初始化模板，尚无真实技术栈约束。
- 没有业务源码；主代理已检查 CodeGraph：文件/节点/边均为 0。因此没有可引用的业务代码模式或 file:line。

## Findings

### 1. 官方事实：成熟应用能解决什么

| 项目 | 经官方资料确认的事实 | 对当前项目的含义（设计判断） |
| --- | --- | --- |
| Jellyfin | 根据客户端能力区分 Direct Play、Remux、Direct Stream 与 Transcode；硬件加速需要正确驱动和设备配置。字幕烧录、HDR 色调映射和某些编码转换可能触发昂贵处理。 | 成熟影音体验优先复用 Jellyfin；自建系统首版只承诺一组已测格式，按条件启用转码。 |
| Immich | External Libraries 可通过只读挂载纳管已有目录；不能从只读库修改/删除源文件。当前要求至少 6GB RAM、推荐 8GB；禁用机器学习可在 4GB 系统运行。数据库不能存放网络共享；官方推荐/支持的部署方式为 Docker Compose。 | 适合照片体验优先且有足够资源的用户；不应将 Immich 作为低内存 NAS 的默认内嵌依赖。原目录纳管应走官方库机制。 |
| Kavita | 官方仓库列出漫画/条漫/美漫（CBR、CBZ、ZIP/RAR/7z、原始图片）与图书（EPUB、PDF），提供响应式阅读器、合集和阅读列表。小说不应默认承诺原生 TXT，需另行转换或自建解析。官方支持 OIDC，可作为纯登录、预配用户或完整账户接入。 | 漫画与 EPUB 可复用成熟阅读器思路；用户若重视海量 TXT 小说，要增加自己的 TXT 解析/阅读器或明确转换策略。 |
| Komga | 官方说明支持 CBZ、CBR、EPUB、PDF；图像阅读器提供从左到右、从右到左、纵向和 Webtoon 连续条带模式，以及按条件双页显示。 | 漫画需要独立书架、作品与章节模型、方向和阅读模式设置，不能直接拿照片浏览器代替。 |
| SQLite | WAL 支持读者与写者并行，但同一时刻只有一个写者；WAL 要求所有进程在同一宿主机，不能使用网络文件系统。FTS5 提供全文检索扩展。 | 单机小规模自建应用可从 SQLite 起步，但扫描任务写入必须限流、分批短事务；DB 必须落 NAS 本机文件系统的持久目录。 |

官方引用：

- Jellyfin 转码：https://jellyfin.org/docs/general/post-install/transcoding/
- Jellyfin 编码/容器矩阵：https://jellyfin.org/docs/general/clients/codec-support/
- Jellyfin 硬件加速：https://jellyfin.org/docs/general/post-install/transcoding/hardware-acceleration/
- Jellyfin 官方 API 入口：https://jellyfin.org/docs/general/clients/api/
- Immich 外部图库：https://docs.immich.app/guides/external-library/
- Immich 当前需求：https://docs.immich.app/install/requirements/
- Kavita 官方仓库/格式：https://github.com/Kareadita/Kavita
- Kavita OIDC：https://wiki.kavitareader.com/guides/settings/openid-connect/
- Komga 格式：https://komga.org/docs/introduction
- Komga 图像阅读器：https://komga.org/docs/guides/webreader-divina/
- SQLite WAL：https://www.sqlite.org/wal.html
- SQLite FTS5：https://www.sqlite.org/fts5.html

以上只说明核实当天的官方状态，不锁定依赖版本。实施时重新核实发行版、镜像和安全更新。

### 2. 三条可行路线

#### A. 直接组合成熟应用（最快形成可用系统）

- Jellyfin 管视频，Immich 管图片，Kavita 管漫画与 EPUB 等图书（漫画专用也可选择 Komga）；统一首页只提供入口。
- 优点：播放、照片时间线、阅读等深层能力已有实现，开发量低。
- 代价：多个服务、独立账户/数据与备份；硬件成本取决于 Immich、转码和衍生处理；无法天然跨应用统一收藏和进度。TXT 不可承诺由 Kavita 原生阅读。
- 不要直接读写这些项目的内部数据库。集成只能通过受支持 API/配置；统一账号必须逐产品验证 OIDC/代理/插件与客户端兼容性。不能因为 Kavita 支持 OIDC 就推断其他应用也支持同等 SSO。
- 更适合“主要想在 NAS 上使用”的场景。

#### B. 轻量统一内容管理系统（结合易使用、易部署要求，建议默认）

- 统一前端、账户、内容索引、标签、收藏和进度；视频、图片、漫画、小说四种处理器独立实现。
- 建议前端 Vue 3 + TypeScript + Vite；后端 Go（标准 net/http 或 Gin）+ SQLite；前端构建静态文件内嵌到 Go 程序，镜像同时提供 FFmpeg/ffprobe。API 和受管理的媒体任务共用一个应用容器，默认只需一个 Compose 服务。
- Go 方案便于形成统一启动入口和轻量应用分发；媒体重处理仍交给 FFmpeg 子进程，不能把 Go 编译产物小等同于完整镜像小，SQLite 驱动、图片解码器、FFmpeg 等原生依赖需验证 amd64/arm64 构建。FastAPI + Pydantic + SQLAlchemy/Alembic 同样可行，若团队更熟悉 Python 或重度图片/文本生态，可作为替代。该选择是部署目标下的设计建议，未做性能基准，也不是用户已选定技术栈；首版不引入两套后端。
- 应用启动后台任务管理器，任务状态持久化到 SQLite；任务表实现租约、心跳、超时重试和幂等；初期只允许一个扫描/写入 worker，媒体任务受 CPU/IO 并发限制。不要将进程内临时后台任务误当作可恢复队列。后续有独立扩容需求才把 worker 拆成同镜像的第二个服务。
- 初期不引入 Redis、Elasticsearch、MinIO、Kubernetes。若出现多机 worker、持续多写者竞争、复杂搜索规模或 HA 需求，再切 PostgreSQL、专用队列或搜索引擎。
- SQLite 必须留迁移入口：数据库访问封装、版本化 SQL 迁移、避免把 SQLite 特有 SQL 扩散到业务层。可先使用名称/作者/标签查找；FTS5 需要另行验证中文切词，不能声称默认 tokenizer 就能提供理想中文全文检索。
- 更适合“自己做一个统一产品”的场景，首版要约束格式和播放能力。

#### C. 统一门户 + 按需复用专业媒体服务（渐进路线）

- 以 B 的目录索引与统一界面为主体，视频复杂播放逐步接 Jellyfin API/跳转；图片、漫画和 TXT/EPUB 初期由自建系统管理，必要时给 Immich/Kavita/Komga 增加连接器。
- 优点：保留统一产品方向，降低重做转码和特殊客户端的负担。
- 代价：比单纯跳转复杂；外部 ID、权限、播放进度同步和失联状态都要设计。集成的第一步应为单向链接/只读，不承诺首版实现所有产品双向状态同步。
- 不宜在 MVP 同时实现多套连接器和自己的四类处理器。

### 3. 最小自建部署与边界

建议拓扑：浏览器/PWA → 单体应用（内嵌 Web 静态文件 + API + 持久任务管理器）→ SQLite/原文件/FFmpeg。默认不额外部署反向代理；外网与 TLS 场景按 NAS 现有能力可选接入。HTTP 大文件传输使用支持 Range 的受控发送文件能力，避免自行写循环搬运所有字节。

- `/media`：宿主 NAS 内容目录，默认只读挂载。
- `/app/data`：数据库、迁移状态、应用配置，落本机可靠持久存储，优先本机 SSD。
- `/app/cache`：封面、缩略图等可重建衍生物，独立容量限制。
- `/app/transcode`：临时切片，可清理、有并发/容量/超时限制。
- PostgreSQL 并非不适合 NAS；当用户明确资源充足、多人服务、并发作业与后续多机扩展时，可以直接选择 PostgreSQL 省去以后迁移。当前信息不足，不应伪造容量阈值。

### 4. 四类内容处理链路

视频：发现文件 → ffprobe 探测时长/编码/分辨率/音轨 → 提取海报与缩略图 → 首先尝试浏览器支持的直接播放；只需换容器则 remux；不兼容时才排队 FFmpeg 转码/HLS。支持 HTTP Range 与拖动，但 Range 或 HLS 本身不保证编码可播。首版明确已测试的 MP4/H.264/AAC；HEVC、HDR、特殊音轨和图形字幕作为单独兼容项。硬解是否可用取决于实际 CPU/GPU、驱动、设备映射和格式组合，不能承诺任意 NAS 实时 4K 转码。

图片：读取宽高、EXIF 时间与方向 → 按方向生成缩略图/预览 → 原图按权限读取 → 时间线与相册。图像处理器设置最大像素/内存限制，不在请求中同步解码超大图。JPEG/PNG/WebP 优先；HEIC/RAW/Live Photo 是否纳入由额外解码链路与真实样本决定。人脸识别/语义搜索延后。

漫画：建立“系列 → 卷/章节 → 页面”独立模型与漫画书架入口；首版接受图片目录和 CBZ（ZIP），按文件名做自然排序（2 在 10 前），允许修正章节/页序。阅读器提供单页、双页、连续条漫及 LTR/RTL 方向；双页避开封面与横向跨页图，按用户保存偏好。进度保存章节 ID、页面稳定标识/序号、条漫页内偏移和内容版本，不能使用照片相册浏览位置代替。首版页图片优先 JPEG/PNG/WebP；CBR/RAR、PDF 漫画后续再扩展解码链路。CBZ 需限制总解压字节、文件数、单页像素、压缩比与处理时限，禁止路径穿越、符号链接越界和嵌套压缩包递归，不把用户提供的文件名直接当输出路径。以上首版格式范围是易部署优先的设计取舍，并非 Kavita/Komga 的能力限制。

小说：TXT 检测/允许手工选择 UTF-8、GB18030 等编码 → 生成稳定章节索引；EPUB 解析 OPF/目录与 spine → 阅读器渲染。进度采用 TXT 章节 ID+字符偏移、EPUB CFI/定位信息，同时记录书籍版本，避免仅用页面百分比。EPUB/TXT 都要支持字体、主题、目录和书签，DRM 不纳入。HTML/XHTML、外链和脚本要隔离与清理，压缩包解压限制大小与路径。

### 5. 文件发现、一致性与任务可靠性

- Linux inotify 的官方说明明确：网络文件系统上的远端变化不会自动可靠呈现，事件队列也会溢出；因此监听只能加速，必须保留启动扫描、定期增量对账和手动全量校验。
- 官方来源：https://man7.org/linux/man-pages/man7/inotify.7.html
- 内容记录使用稳定 ID。文件位置单独建实体，保存库 ID、规范化相对路径、大小、mtime、设备/inode（仅作为提示，不作为跨 SMB/NFS/重挂载的稳定身份）。
- 首轮仅采集目录和轻量 stat，后续按需计算分段指纹；真正去重或确认文件一致时才计算完整强哈希。首轮对全部视频计算全文件哈希会产生不必要磁盘读量。分段指纹不可作为无碰撞唯一身份。
- 挂载目录不可读/突然清空时，先判定库离线；只有一次扫描成功完成、确认库可读且对账完整，才把缺失项标记为 unavailable。不自动删除库记录，更不删除源文件。
- 每次处理记录输入版本、处理器版本和任务唯一键。失败按次数/退避重试，允许取消与恢复；进程退出后靠租约过期恢复，重复执行不能产生重复衍生物。
- 移动/重命名先通过候选指纹确认身份；不确定时创建待合并候选，不盲目覆盖收藏和进度。

### 6. 最小数据模型（设计建议）

- Library：媒体类型、根目录、状态、扫描策略。
- ContentItem：用户感知的作品、标题、类型、简介、稳定 ID。
- FileAsset：位置与版本、指纹、媒体探测信息、是否在线；一部作品允许多个文件。
- VideoMetadata / ImageMetadata / BookMetadata：类型特有字段，避免全塞一个大 JSON。
- ComicSeries / ComicChapter / ComicPage：漫画系列、卷/章节、页面及自然排序、阅读方向和页序；独立于普通图片相册。
- Collection / Tag / ItemTag：相册/合集、分类。
- User / LibraryPermission / Favorite / ReadingOrPlaybackProgress：个人状态与库级权限。
- Job / ScanRun / Derivative：后台任务、完整扫描记录、衍生物可重建状态。

### 7. 首版收敛与验收建议

MVP 按“一个 NAS、只读纳管原目录、少量用户、四类基础消费”设计：账号、库管理、扫描/离线状态、分类/标签/收藏、通用搜索、视频直放与进度、图片缩略/预览、漫画目录/CBZ 阅读与按页进度、TXT/EPUB 阅读、任务状态、数据库备份恢复。

易使用/易部署验收：默认仅一个应用镜像、一个 Compose 服务与挂载配置，不需要用户额外装数据库/Redis/FFmpeg；首次启动通过网页向导设置管理员、导入内容库；前端中文导航按视频/图片/漫画/小说四区展示，图片为相册/时间线、漫画为系列书架/章节列表，任务与失败原因可见；升级前可备份数据并说明镜像/数据库迁移回退边界。

后续阶段：硬件转码、HLS、漫画 CBR/PDF、外部元数据匹配、精细权限、移动上传与去重、完整中文全文检索、Jellyfin 等连接器。

延期：下载/采集爬虫、AI 人脸和语义搜索、原生客户端、自动改名搬移、分布式部署、三方系统全量双向同步。

关键验收：同一目录重复扫描不重复建项；挂载断开不会清空库；扫描/处理被中断可恢复；源文件默认不会被修改；播放/阅读进度按用户隔离；漫画页码自然排序正确，LTR/RTL、单/双页及条漫进度恢复正确；数据库恢复后能重建缓存；实际样本集验证四类格式及特殊文件名。性能目标等用户提供规模/硬件后再确定。

### 8. 备份与访问建议

- 原文件备份和应用数据库备份是两件事；RAID 不替代备份。
- SQLite 需一致性备份（例如在线备份 API），不能在 WAL 活跃时只复制主 DB 文件。密钥和配置需要一同保护，缓存/转码切片可排除。
- 原文件挂载只读，API 仅允许访问已登记根目录下文件，校验路径与符号链接越界，登录和媒体文件读取都检查权限。
- 默认局域网使用；外网需求优先 VPN 或明确的 HTTPS 认证反代方案，不应直接暴露管理接口。

## Recommended five references for the final proposal

1. https://jellyfin.org/docs/general/post-install/transcoding/ — 为什么直放优先、何时需要转码。
2. https://docs.immich.app/guides/external-library/ — 现有照片目录可只读纳管。
3. https://docs.immich.app/install/requirements/ — Immich 资源与数据库存储约束。
4. https://github.com/Kareadita/Kavita — 漫画、条漫与图书格式支持。
5. https://www.sqlite.org/wal.html — 单机、单写者与网络共享限制。

## Caveats / Not Found

- 用户尚未提供 NAS 型号、系统、CPU 架构、内存、核显、内容体量、并发、TXT 比例与外网需求；上述默认场景必须明确。
- 没有做硬件性能或浏览器格式兼容实测，不能承诺具体吞吐量、并发数和完成工期。
- 没有对所有第三方项目当前完整 API/OIDC 能力做逐端点验证，不承诺一键 SSO 和完整双向状态同步。
- Go/Vue 与 FastAPI 替代栈是可行设计建议，非仓库已有决策；没有引用版本号或声称它们优于所有替代栈。
- 没有生成业务代码、单元测试、安装依赖或修改项目规范。
