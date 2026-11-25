# 多来源资源收集

“资源收集”统一管理来源、RSS订阅、候选确认、获取任务和实际入库结果。首期入口包含RSS/BT、网页视频/提取音频、本地只读登记、归档复制和预配置remote复制。基础媒体服务不要求安装worker；缺失工具或执行端时会显示能力不可用。

## 使用流程

1. 从侧栏或手机“更多”进入资源收集，先查看概览中的真实能力。
2. 添加来源。RSS地址仅写入服务器的独立秘密文件，不会回显；本地来源选择已授权媒体根；复制来源选择worker公开的授权来源编号。
3. RSS首次刷新建立历史基线。需要历史内容时显式补查；新条目进入候选。订阅默认人工确认，可以先试运行规则，再启用明确的自动策略。作品、集数或选文件有歧义时仍须人工确认。
4. 网页/文件先预览，再选择媒体类型和目标媒体库提交。网页只支持单条目；批量链接逐条预览和提交，每条分别记录成功或失败。
5. 获取完成之后校验、发布、扫描。任务中的获取状态和入库状态分别显示；扫描失败可单独重试入库，无需再次获取。

已有音乐默认只读登记，读取现有标签和封面，不移动媒体、不修改标签。选择归档复制才会生成受管副本。rclone只执行复制，不提供同步、删除、移动或挂载。MoviePilot、AutoBangumi、Sonarr/Radarr 提供站内 API 管理和可选公开入口，成品通过独立完成目录导入；实际下载继续由外部工具编排。

BT仍使用下载中心的固定流程。RSS确认后产生关联下载任务，元数据就绪后在下载中心确认文件；已授权且无歧义的自动规则可以执行确定性选择。做种与入库分别记录，入库完成不主动停止做种。

## JMComic 漫画插件

“漫画搜索下载”提供可选 JMComic 来源、关键词分页搜索与漫画编号预览。确认后沿用原获取、发布、扫描与漫画阅读流程。插件依赖仅在独立执行端安装，原生部署默认关闭；专用新版 worker 镜像内置锁定依赖并启用。完整配置和使用步骤见[JMComic 漫画搜索与下载](jmcomic.md)。

## 接入外部自动化服务

1. 添加“外部工具”来源并选择服务，填写实例地址和已有 API 凭据。HTTP 仅用于显式授权的内网；私网或回环地址需勾选允许访问私网。
2. 保存后进入管理面板，检测连接。系统自动判断版本和可用操作，无需选择 API 版本。凭据只写不回显，普通编辑留空会保留原连接。
3. 在同一个面板查看订阅、下载队列、历史及操作记录。完成目录按需在高级设置中映射，随后使用独立预览和登记功能扫描入库。

| 服务 | 本轮可用操作 |
| --- | --- |
| MoviePilot 3 | 订阅列表、精确 TMDB/类型/季度订阅、暂停/启用订阅、单订阅显式搜索、队列和历史 |
| MoviePilot **3.2.8** | 额外提供下述六阶段工作台；其它版本不推断此扩展兼容性 |
| AutoBangumi 4 | 番剧/RSS 列表、RSS 登记、启停监控和有限下载队列；RSS 登记本身不等于完成番剧订阅配置 |
| Sonarr 4 / Radarr 7 | 资料查询、选择已有根目录和画质新增条目、监控切换、单目标显式搜索、队列和历史 |

未知版本不开放未经确认的写操作。实例权限或插件可能影响实际支持，连接失败与空列表分别提示。软件不会安装这些套件，也不修改它们的站点、下载器或全局规则。

### MoviePilot 工作台

- **集成搜索与订阅**：搜索影视资料，选择正确的 TMDB 身份，再寻找资源并确认下载；也可从资料创建精确订阅。资料查询本身不下载，资源搜索也必须经确认才创建所选下载。
- **下载编排**：查看当前任务，明确暂停或继续一个任务。MoviePilot 使用自身配置选择下载目录和编排后续工作，本系统不直接控制它的 qBittorrent。
- **整理**：读取已配置的本地下载根，选择一个视频文件和已设置为“不覆盖”的媒体库目标，核对 TMDB 资料、查看预览后提交固定复制整理。提交前重读实际目录策略，保留源文件，不允许任意路径或整棵目录提交。外部管理员并发改配置仍属于 MoviePilot 实例边界。
- **刮削**：在已配置本地媒体库根中选择单个视频文件，校验资料和文件版本，明确勾选允许覆盖图片及 NFO 后执行。刮削预览不写文件；真正刮削使用 MoviePilot 的覆盖语义。
- **媒体库刷新**：执行并查看固定的“刷新 MoviePilot 媒体库索引”任务。该任务从已配置的媒体服务器同步索引，**不等于要求 Emby/Plex 扫描磁盘**。

站点 Cookie、资源地址、下载器身份和远端绝对路径保存在独立秘密文件，浏览器只取得有时效的不透明选择。单次工作流最多 100 项，选择 15 分钟后过期，最多保留每来源 600 个活动选择；文件列表过大时明确提示分组处理。每个写动作有持久请求编号，同一已接受请求返回原结果，不会重复下发。断线或超时导致结果未知时先核对；无法自动证明时，在外部检查后可记录“人工核查并解除阻塞”，系统仍保留“未确认”事实且不会重发。

外部已受理、外部已下载、外部已整理、本系统已入库是不同状态。只有本系统独立扫描并确认具体媒体条目后才显示入库完成。

## Linux/NAS部署

需要从平台部署 MoviePilot、AutoBangumi、Sonarr/Radarr 时使用新版引擎助手覆盖文件；选择服务后可在来源窗口完成部署并接入。具体首次配置、目录映射与Windows v3完整模式见[资源组件部署](components.md)。

在仓库根构建可选服务：

```sh
docker compose -f compose.yaml -f compose.collection.yaml up -d --build
```

该组合创建应用数据、获取数据和秘密三个独立持久卷。主应用只读挂载获取数据和秘密，worker以UID/GID1000运行，首次生成私有令牌。worker不挂Docker socket，不发布宿主控制端口。默认只能读取原有`/media`并处理公共网页；远程来源需管理员配置。

同时启用托管BT时，在`.env`中显式设置媒体根并叠加两个覆盖文件；`ENGINE_HOST_ROOT`等参数仍按[引擎部署](engines.md)配置：

```dotenv
# 合并原媒体、BT完成区和获取完成区，不能将staging/私有状态加入媒体根。
COLLECTION_MEDIA_ROOTS=/media:/downloads/complete:/collection/complete:/collection-media/library
```

```sh
docker compose -f compose.yaml -f compose.engines.yaml -f compose.collection.yaml up -d --build
```

获取网络和引擎网络各自保留。不要改成host网络或将worker直接暴露到公网。需要跨主机连接时使用管理员配置的TLS反向代理，并保留Bearer认证；主服务还会验证共享根随机探针。

## 原生/Windows对接

原生主服务和worker必须能读到同一获取数据实体；两侧路径可以不同，配置`NAS_COLLECTION_READ_ROOT`指向主服务侧的只读映射。仅设置相同字符串路径不代表映射成立，连接检查会验证随机探针和执行端身份。

worker可用`go build ./cmd/nas-collection-worker`单独构建。使用固定路径配置工具，不把用户输入作为命令参数模板。Windows启动器完整模式支持v2固定三镜像清单，自动叠加`compose.desktop.collection.yaml`并核验额外服务、卷、挂载、端口和镜像归属；旧v1清单保持两服务模式。生成方式见[发布流程](release.md)。

## 主服务配置

配置由管理员环境提供；网页展示能力和安全原因，不回显令牌或私密路径配置内容。执行端位置、工具路径、凭据和挂载权限属于部署配置，首期不允许网页修改；来源、规则、目标和任务可在收集中心维护。

| 参数 | 默认值与中文解释 |
| --- | --- |
| `NAS_COLLECTION_WORKER_URL` | 空；获取执行端地址，配置时必须同时提供令牌文件和只读映射 |
| `NAS_COLLECTION_WORKER_TOKEN_FILE` | 空；读取执行端专用Bearer令牌的绝对文件路径 |
| `NAS_COLLECTION_READ_ROOT` | 空；执行端数据根在主服务侧的绝对只读映射 |
| `NAS_COLLECTION_WORKER_ALLOW_HTTP` | `false`；仅显式允许回环/受限内网HTTP，跨主机优先使用HTTPS |
| `NAS_COLLECTION_MAX_BYTES` | `21474836480`；单任务总字节上限，默认20GiB |
| `NAS_COLLECTION_MAX_FILES` | `1000`；单任务文件数上限 |
| `COLLECTION_MEDIA_ROOTS` | Compose覆盖参数，默认`/media:/collection/complete`；同时启用BT时加入`/downloads/complete`，启用组件部署时再加入`/collection-media/library` |

## Worker配置

| 参数 | 默认值与中文解释 |
| --- | --- |
| `NAS_COLLECTION_WORKER_ADDR` | `127.0.0.1:8092`；监听地址，容器显式设置`:8092` |
| `NAS_COLLECTION_WORKER_DATA_DIR` | `collection-data`；专用持久根，状态/staging/complete位于同一实体文件系统 |
| `NAS_COLLECTION_WORKER_TOKEN_FILE` | 必填；首次生成的专用令牌文件，既有内容不会被覆盖 |
| `NAS_COLLECTION_WORKER_SOURCES_FILE` | 空；管理员授权来源JSON，文件缺失时来源列表为空 |
| `NAS_COLLECTION_WORKER_RCLONE_CONFIG_FILE` | 空；rclone独立私密配置，缺失时远程复制不可用 |
| `NAS_COLLECTION_WORKER_YTDLP` | `yt-dlp`；固定网页工具路径，镜像中为`/opt/collection/bin/yt-dlp` |
| `NAS_COLLECTION_WORKER_RCLONE` | `rclone`；固定复制工具路径，镜像中为`/usr/bin/rclone` |
| `NAS_COLLECTION_WORKER_FFMPEG` | `ffmpeg`；固定后处理工具路径，镜像中为`/usr/bin/ffmpeg` |
| `NAS_COLLECTION_WORKER_MIN_FREE_MB` | `512`；每次任务应保留的磁盘空闲空间，单位MiB |
| `NAS_COLLECTION_WORKER_MAX_BYTES` | `21474836480`；执行端单任务字节预算，主服务不能突破此上限 |
| `NAS_COLLECTION_WORKER_MAX_FILES` | `1000`；执行端单任务文件上限 |
| `NAS_COLLECTION_WORKER_MAX_CONCURRENT` | `2`；执行并发数，允许1–4 |

授权来源文件示例；下面路径须根据真实部署修改，配置每一项含义如下：`id`为公开安全编号，`kind`为local/remote，`name`为显示名称，`path`为只在执行端保留的授权根。

```json
[
  {"id":"music","kind":"local","name":"已有音乐副本输入","path":"/imports/music"},
  {"id":"cloud","kind":"remote","name":"自有远程目录","path":"myremote:Media"}
]
```

本地复制需要另行将`/imports/music`只读挂载给worker；桌面固定模板不接受额外宿主挂载，桌面本地媒体使用主服务只读登记。remote必须已在专用`rclone.conf`中配置，网页不能提交任意连接字符串。配置文件和token不能提交到Git或普通数据库备份。

## 文件、恢复与升级

执行端目录包括`private-state/`、`staging/`、`complete/`和`probes/`。主服务即使读到整个映射，也会拒绝未发布、未知、越界或版本变化的文件。下载或复制完成先停在`acquired`；主服务持久化事实并显式请求发布后，worker才在同文件系统原子重命名到完成区。不能手动把暂存文件移入媒体根绕过门禁。

主库增量迁移到 v4，保留原账号、会话、五类媒体、个人状态、作业、v2 下载表和 v3 收集记录，新增外部连接引用、操作审计和短期选择。数据库快照不含 API 密钥、实例私密地址或资源原始上下文；完整恢复还需配套恢复独立来源秘密、worker 状态/令牌/来源配置、获取产物和原 BT 引擎数据。停服后备份这些材料并保留权限；先恢复文件及秘密，再恢复匹配数据库，最后启动并对账。恢复后未确认的外部写操作不会自动重发。不要手工修改 `user_version`。

worker重启不会盲目重发不确定获取操作；运行中断进入待核对，发布中断根据暂存/完成事实恢复。已有发布媒体在worker离线时仍可读取。取消保留输入和已有产物；清理暂存、删除媒体索引和删除原文件是不同操作，首期不提供自动删源文件。保留策略固定为保留任务与产物、不按时间自动删数据；worker历史上限为10000条，达到上限停止接收新获取。主服务每任务最多保留100条阶段事件；预览有时效，过期后必须重建。归档须在停服并完成备份后由管理员维护，不能只删除业务库中的记录。

网页入口不提供Cookie上传、DRM处理或播放列表/频道递归。当前固定格式只允许HTTP(S)直连与原生DASH分片；仅提供HLS的来源会明确拒绝，避免工具退回ffmpeg直接联网。ffmpeg只进行本地后处理。工具可运行不保证任意站点可下载；需要登录、JavaScript挑战、站点更新或附加运行依赖时返回安全中文失败。视频/音频输出能否在浏览器直接播放仍取决于编码支持。

## 验证

通过项目环境包装器构建并运行隔离脚本：

```sh
python -B scripts/project_env.py -- go build -o .runtime/verify/nas ./cmd/nas
python -B scripts/project_env.py -- go build -o .runtime/verify/nas-collection-worker ./cmd/nas-collection-worker
python -B scripts/project_env.py -- python -B scripts/verify_collection.py --binary .runtime/verify/nas --worker-binary .runtime/verify/nas-collection-worker
```

可附加`--rclone /绝对路径/rclone`验证真实CLI复制；附加`--ytdlp`、`--ffmpeg`和`--web-url`验证小型公共网页视频与提取音频。命名local/alias remote测试不等于真实云盘兼容测试；受控假qB测试不等于真实BT传输。具体已执行结果和平台限制见[验证记录](verification.md)。
