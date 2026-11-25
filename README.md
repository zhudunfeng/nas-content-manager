# 栖库 · NAS 内容管理

将视频、图片、漫画、小说和音乐放在同一个私人媒体库中。Vue 3 界面、Go API、SQLite 与后台任务通过一个应用容器部署，原媒体只读挂载，不需要额外安装数据库、Redis 或搜索服务。

当前为 `0.1.0` 首版源码。建议先在局域网和少量样本上试用，再接入完整媒体目录。当前没有已发布的公共镜像，Compose 会从本仓库构建。

已增加 Linux/Windows 共用内核的发布与启动入口：Windows 原生包使用 `栖库.exe` 双击/托盘管理，Linux 归档使用 `start.sh` 或原 CLI；可选 Docker Desktop 完整模式使用独立配置和 Linux 命名卷。使用与升级见 [Windows 与双平台说明](docs/windows.md)，维护者构建见 [发布流程](docs/release.md)。仓库不会把缺少 FFmpeg 的开发验证包当作完整离线发行，也没有虚构公共应用/助手镜像。

## 首版能力

- 网页创建管理员、登录、选择已挂载目录、创建五类媒体库。
- 持久化扫描、任务状态、取消与重试、重启恢复、定期目录对账。
- 中文搜索、标签、收藏、继续观看／阅读／收听、按账号保存进度。
- 视频原文件直放，鉴权 `GET/HEAD` 与 HTTP Range，64 位文件大小和偏移。
- JPEG、PNG、GIF、WebP 图片预览及受控封面缓存。
- 图片目录和 CBZ 漫画，自然页序、单页／双页／连续阅读及方向切换。
- UTF-8／GB18030 TXT 和无 DRM EPUB，目录、阅读设置与进度。
- 音乐文件扫描、原生音频直放、拖动与秒数进度恢复，异步时长探测和内嵌专辑封面。
- 明暗主题、响应式界面、数据库一致性备份。
- 可选 BT 下载中心：连接 qBittorrent，添加磁力／种子、确认文件选择、管理任务和限速，验证完成后入库五类媒体。
- 可选搜种：接入已有 Prowlarr／Jackett，聚合关键词搜索、来源／类别筛选、排序与分页，选择结果后进入原安全下载流程。
- 多来源资源收集：RSS候选与订阅、网页视频/提取音频、本地只读登记、归档复制和预配置远程目录复制；获取与实际入库分别展示。配置方式见[资源收集](docs/collection.md)。
- 可选 JMComic 漫画插件：关键词分页搜索、编号预览下载，复用资源收集任务与漫画库阅读；部署和使用见[漫画搜索下载](docs/jmcomic.md)。
- 可选 UAV 视频插件：搜索 Jable、MissAV、Hanime1、Huangguo（黄果），预览选定视频并下载到视频库；复用原任务、发布和播放器，部署见[视频搜索下载](docs/uav.md)。
- 外部自动化整合：三步连接 MoviePilot、AutoBangumi、Sonarr/Radarr，按服务能力管理订阅与任务；MoviePilot 3.2.8 提供搜索、订阅、下载控制、复制整理、刮削和媒体库索引刷新，完成目录另行预览导入。
- 平台部署资源组件：Linux NAS 托管模式与 Windows Docker 完整模式可从来源弹窗部署上述四个组件，展示镜像层进度并恢复任务；完成首次配置后接入原 API 管理。启用与目录说明见[组件部署](docs/components.md)。

视频实际可播放性取决于浏览器对容器、视频和音轨的支持。首版采用原片直放，播放服务不提供 HLS、实时转封装或硬件转码；UAV 获取阶段会将支持的有限 HTTP/HLS 视频离线封装并校验为 MP4，不改变播放器能力。不兼容时会提示使用兼容客户端／外部播放器。CBR、PDF、HEIC、RAW、DRM 图书、AI 整理和用户权限细分不在此版本范围。

为控制低配置 NAS 与浏览器的资源占用，TXT 原文限 16 MiB／10 万段；CBZ 限 2 GiB／10000 页／单页解压后 32 MiB；EPUB 下载限 64 MiB，最多 5000 个归档条目，解压总预算 256 MiB。超过限制会给出明确错误，不自动拆书或改写源文件。视频不受这些阅读器限额影响。

EPUB 在交给阅读组件前校验 ZIP 目录与本地条目，并流式检查实际解压量。压缩 EPUB 需要浏览器支持原生 `DecompressionStream('deflate-raw')`；不支持时会提示使用更新的浏览器或本地阅读器，不会降级为无界解压。

## Docker Compose 部署

前提：NAS 已安装可用的 Docker Engine 与 Compose 插件，且能拉取构建镜像及依赖。需在本仓库根目录操作。

1. 复制 `.env.example` 为 `.env`。
2. 编辑 `.env` 的 `MEDIA_PATH`，填入 NAS 上现有媒体目录的绝对路径；按需修改 `WEB_PORT`。
3. 执行：

```sh
docker compose up -d --build
```

4. 打开 `http://NAS地址:8090`，创建管理员，点击“添加媒体库”。选择容器中可见的 `/media` 或其子目录，选择内容类型，启动扫描。

例如宿主机 `/volume1/Media` 挂到 `/media` 后，`/volume1/Media/电影` 在网页中显示为 `/media/电影`。网页只能访问已经挂载的目录，无法直接选择未挂载的宿主路径。五类内容可以来自任意子目录，不要求重新组织原文件。

```dotenv
# 宿主机现有媒体根目录，容器内对应 /media。
MEDIA_PATH=/volume1/Media
# 宿主机网页端口，容器内部端口保持 8090。
WEB_PORT=8090
```

Windows 路径可写作 `MEDIA_PATH=E:/Media`。若尚无媒体，请先创建本地 `media` 文件夹，保留默认配置启动，再添加内容。

**权限与数据：**容器以 UID/GID `1000:1000` 运行。请给予该身份媒体目录的读取／目录遍历权限。默认 `nas_data` 命名卷保存 `/data/library.db` 和缓存，位于 Docker 本机存储。改用宿主数据目录时，先创建目录并授予 `1000:1000` 写权限。SQLite 数据目录不能放在 SMB/NFS 共享上。不要使用 `docker compose down -v`，它会删除应用数据卷。

有图形化容器管理器时，可以导入 `compose.yaml` 作为项目／堆栈，设置媒体挂载与端口后构建启动。各厂商界面不同；仓库构建需要将 Dockerfile 与源码一起上传。

```sh
# 查看应用状态和中文业务日志。
docker compose ps
docker compose logs --tail=100 nas
# 正常停止服务，保留数据卷。
docker compose down
```

## 使用方式

- 首次创建管理员时，密码需为 10–72 位，仅允许英文字母、数字和英文标点，并同时包含大写字母、小写字母、数字和特殊字符（如 `!@#$%`）。不允许中文、全角字符、空格或表情；已有账户仍可使用原密码登录。
- 视频库按文件登记；首次扫描不读取整片哈希，封面和元数据在后台增强，扫描期间已发现的内容可以浏览。
- 图片库按文件登记。列表只请求缩略图，打开详情后读取原图。
- 漫画库中含图片的目录可作为一本／一章阅读，CBZ 作为独立条目；页面按 `1, 2, 10` 自然排序。更复杂的系列合并与元数据编辑留待后续版本。
- 小说库登记 `.txt` 与 `.epub`。TXT 可手动指定 GB18030 纠正编码，EPUB 由本地打包的阅读组件打开。
- 音乐库登记 `.mp3`、`.m4a`、`.aac`、`.flac`、`.ogg`、`.opus`、`.wav`、`.aif`／`.aiff`，扩展名大小写均可。添加目录后逐文件显示，可搜索、收藏与打标签；暂停、拖动或关闭时保存进度。手机底栏的“音乐”可直接进入，漫画、小说和管理入口位于“更多”。
- 搜索标题、相对路径或标签；收藏和进度保存在数据库中，不写回原文件。
- 媒体路径失联时检查挂载和读取权限，再重新扫描。失败或取消的扫描不应把整库视为已删除；成功扫描可标记确实缺失的条目。
- 删除媒体库只移除索引与相关应用状态，不删除媒体原文件。

### 音乐播放说明

音乐沿用鉴权 `GET/HEAD/Range` 原文件流式接口，不会先下载整个音频到浏览器内存，也不改写原文件。有内嵌封面时后台生成缩略图，没有封面时显示音乐占位图；无封面不会反复产生失败任务。FFprobe 缺失或文件损坏会在任务页显示原因，基础扫描与原文件访问仍可用。

可播放性取决于浏览器支持的封装与音频编码，扫描支持不代表所有设备均可解码。播放失败可下载原文件交给兼容播放器，或自行转换副本；本版不提供音频转码、在线曲库、歌词、歌单或离开播放器后的后台迷你播放。

## 超大视频设计

视频接口在检查登录与媒体根目录边界后，使用文件句柄和 Go `http.ServeContent`。没有将视频读入内存、转换成 Base64 或先下载到浏览器 Blob 的中转步骤；没有全局短写入超时截断长时间播放。

- 支持起始、中段、尾部 Range、`206/416`、`HEAD` 和资源验证头。
- 文件大小、偏移、数据库存储使用 64 位类型。
- 浏览器使用原生播放器，从服务端按需请求字节；关闭播放器释放旧资源并保存时间进度。
- 探测与封面任务限并发、限时，衍生缓存具有容量与剩余空间保护，不改写原片。
- 不因文件体积大自动进行全片转码或 `faststart` 复制。

Range 只解决随机字节读取。尾部 MP4 索引、MKV 索引、关键帧间隔、HDR、音轨、NAS 磁盘、网络带宽与浏览器解码均影响起播和拖动。首版不承诺任意编码、设备或网络下的实时 4K/HDR 播放。

`scripts/verify.py --large-file` 使用 5 GiB 稀疏文件验证超过 4 GiB 的偏移与返回字节，**该样本不能解码，不是视频性能测试**。实际大影片的起播、片尾拖动、持续播放和峰值内存仍需在目标 NAS 上实测。详细验证结果见 `docs/verification.md`。

## 可选 BT 下载

基础部署默认关闭下载，继续只运行一个应用容器。已有 qBittorrent 时连接其 WebAPI；没有下载器时可叠加可选官方引擎配置。初始化、独立凭据文件、目录权限和手动部署步骤见 [下载部署与使用](docs/downloads.md)，参数模板见 `.env.downloads.example`。

希望从网页自动安装缺失的 qBittorrent / Jackett，可先应用可选的 `compose.engines.yaml`。随后在下载中心“引擎管理”中完成检测、拉取、安装与来源同步；支持真实阶段进度、持久任务与失败重试。首次部署步骤、Docker权限说明和旧宿主兼容限制见 [引擎一键初始化](docs/engines.md)，中文参数模板为 `.env.engines.example`。

网页侧栏或手机“更多”进入“下载中心”，添加一个磁力链接或 `.torrent`，选择媒体类型。元数据就绪后确认所需文件，选集随后锁定。任务详情支持暂停／开始、对账、任务级限速和查看入库内容；移除任务保留下载文件，入库失败可以单独重试。

下载器只写专用下载卷。栖库只读挂载该卷的 `staging` 与 `complete`，由引擎移动文件并经持久化选集及文件版本核验后发布；原 `/media` 保持只读。暂存、未知成品目录、未选文件及尚未发布的任务不能通过扫描、漫画页、封面或文件流接口读取。关闭下载开关或归档任务仍保留这道门禁。入库与做种分别记录，入库不会主动停止做种。

超大下载由引擎直接接收与校验，Go 不中转整片，不重复计算全片哈希。推荐 staging 与 complete 位于同一真实文件系统；跨文件系统移动可能复制，实际吞吐、缓存和同时播放表现需在目标 NAS 验证。RSS入口见[资源收集](docs/collection.md)；PT站点专属自动化、自动删文件、边下边播和转码仍不在此版本范围。

### 可选搜种

在下载中心点击“搜种”，输入关键词，按来源／影视／音乐／书籍筛选，比较体积、做种数和发布时间后添加。添加时仍需确认五类内容中的目标类型，再选择所需文件；搜索不会直接启动载荷下载。

默认关闭，已有 Prowlarr／Jackett 可直接连接 Torznab；没有时可额外部署固定版本 Jackett。配置、私密来源文件、可选 Compose 和手动验收见 [搜种部署与使用](docs/torrent-search.md)，环境模板为 `.env.search.example`。API Key、私密URL和原始磁力仅保留后端，不返回搜索界面。搜索失败不影响媒体浏览和已有下载。

## 备份、恢复与升级

在设置页下载数据库备份。接口创建一致性 SQLite 快照，避免直接复制正在使用 WAL 的数据库主文件。备份含账号、媒体索引、标签、收藏与进度，属于敏感资料，请妥善保存。原媒体须独立备份，可重建缓存不必纳入业务备份。

当前数据库头版本为 **6**，持久备份策略默认关闭。`NAS_BACKUP_DIR` 可预配置专属备份根，默认是规范化数据目录下的 `backups`；正常启动不会因此创建根或扫描离线备份盘。持久根与平台发布的内部能力已完成本机独立验收，真实 Linux/NAS 仍待验；尚未开放手动持久备份、定时、历史下载或轮转入口，现有设置页下载仍是每次生成的临时快照。同盘备份不能替代异地备份。

恢复必须停机进行：

1. `docker compose down`，保留原数据卷并另外备份其内容。
2. 找到挂载的 `/data`（可用 `docker volume inspect` 查询实际项目卷），将旧 `library.db`、`library.db-wal`、`library.db-shm` 一起移到单独保留目录；不要把旧 WAL 留给恢复的数据库。
3. 将下载的备份放为 `/data/library.db`，确保 UID/GID `1000:1000` 可读写。
4. `docker compose up -d`，登录并重新扫描。缓存可重建，路径映射需与原配置保持一致。

升级前保留当前源码版本、镜像、完整数据和一致性备份，再更新源码并运行 `docker compose up -d --build`。回滚时镜像与对应数据库快照一起恢复；不能假设旧镜像能读取未来数据库结构。

下载功能将数据库从 v1 增量迁移到 **v2**，保留原五类索引、账户、收藏、标签和进度。即使下载开关关闭也会执行迁移；旧程序不能直接读取 v2，回滚必须恢复配套的 v1 快照。音乐沿用原有字段，仍无需额外音乐服务。

数据库备份含下载任务、选集与操作意图，但**不包含**原媒体、下载数据、qB 配置／恢复数据、后端私密提交源文件和密码文件。需要完整恢复下载时，应在停止双方写入后独立备份这些数据，并保持挂载映射一致，详见下载文档。

## 本机开发

当前工作区的 Windows 可用开发包及启动说明见[当前可用构建（2026-11-23）](docs/usable-preview.md)。已经解压，可直接运行其中的 `栖库.exe`；开发包未捆绑 FFmpeg，持久备份执行器仍待后续实施。

需要 Node.js 22.14 或兼容版本、Go 1.26+、Python 3.10+、FFmpeg／ffprobe。工具缺失时仍可使用基础内容功能，音视频探测与封面能力受限。无需启动 Docker。以下跨平台命令在 Linux 使用 `python3`，Windows 替换为 `python`。

```sh
# 安装锁文件中的前端依赖并构建。
python3 -B scripts/project_env.py -- npm --prefix web ci
python3 -B scripts/project_env.py -- npm --prefix web run build
# 先创建 media 目录，或用 NAS_MEDIA_ROOTS 指定已有目录，再运行后端。
python3 -B scripts/project_env.py -- go run ./cmd/nas
```

默认网页地址为 `http://127.0.0.1:8090`。开发热更新另开终端运行 `python3 -B scripts/project_env.py -- npm --prefix web run dev`，Vite 的 `/api` 代理指向 `127.0.0.1:8090`。

PowerShell 快捷命令：

```powershell
# 启动后端；Windows 多个媒体根用分号分隔。
./scripts/dev.ps1 -Mode backend -MediaRoots 'E:/Media;F:/Books'
# 另开终端启动前端热更新。
./scripts/dev.ps1 -Mode frontend
# 生成 web/dist 与 bin/nas.exe。
./scripts/dev.ps1 -Mode build
```

普通 `go build` 开发产物从 `NAS_WEB_DIR` 读取网页，移动时应带上 `web/dist`。发布脚本和容器构建使用 `webembed` 内嵌网页；显式设置 `NAS_WEB_DIR` 时仍优先使用该目录。完整原生包还携带经核验的媒体工具，运行时不依赖开发环境。

### 构建缓存与临时目录

开发、发布和验证脚本自动将可控缓存与临时文件放入当前项目的 `.runtime/`。统一配置为 `scripts/project-env.json`，支持改为 `.runtime/` 下的专用子目录，不修改系统或用户全局环境。已有的 `bin/`、`web/dist/` 和网页嵌入暂存目录继续位于项目内。

```powershell
# 当前 PowerShell 终端加载配置，之后直接运行 Go/npm 等命令。
. .\scripts\project-env.ps1

# Windows/Linux 通用：仅为这一条命令及其子进程加载配置。
python -B scripts/project_env.py -- npm --prefix web run build
```

首次使用会重新建立项目独立依赖缓存。已启动的 IDE/MCP 服务需要在启动前加载配置，旧 C 盘缓存不会自动迁移或删除。目录列表、Linux 用法及控制范围见 [项目构建环境说明](docs/project-build-environment.md)。

### Windows 会话缓存清理

固定脚本 `scripts/clean-session-temp.ps1` 默认只预览并记录清单，使用 `-Execute` 才按已有清单清理。Go 缓存需要显式选择，浏览器资料需要精确指定测试 profile；清理依赖缓存后需联网重新下载。参数、目录范围与复用示例见 [会话缓存清理说明](docs/session-temp-cleanup.md)。

## 环境参数

普通使用只需修改媒体路径和端口；其余保留默认。启动时校验配置，非法范围会报中文错误并退出。

| 参数 | 默认值 | 中文说明 |
| --- | --- | --- |
| `NAS_ADDR` | `:8090` | HTTP 监听地址；容器固定监听 8090，由宿主映射端口 |
| `NAS_DATA_DIR` | `data` | 可写应用目录，SQLite、缓存与临时备份所在位置 |
| `NAS_BACKUP_DIR` | 规范化数据目录下的 `backups` | 预配置专属持久备份根，须为本地绝对路径；启动只校验静态配置，不创建或探测该根，持久执行入口尚未开放 |
| `NAS_MEDIA_ROOTS` | `media` | 可访问的媒体根；Linux 用冒号分隔，Windows 用分号分隔 |
| `NAS_WEB_DIR` | `web/dist` | 已构建网页目录 |
| `NAS_FFMPEG_PATH` | 自动定位 | 可选 FFmpeg 绝对路径；否则优先可执行文件旁 `tools`，其次安全 PATH |
| `NAS_FFPROBE_PATH` | 自动定位 | 可选 ffprobe 绝对路径；封面、探测和诊断共用解析结果 |
| `NAS_SCAN_INTERVAL_MINUTES` | `60` | 周期扫描间隔，单位分钟 |
| `NAS_CACHE_MAX_MB` | `1024` | 可重建封面缓存上限，单位 MiB |
| `NAS_MIN_FREE_MB` | `512` | 缓存与数据库快照工作的最低余量基线，单位 MiB；具体操作另加所需空间，不代表已预留容量 |
| `NAS_WORKERS` | `1` | 后台任务并发数，支持 1–4，低配置 NAS 使用 1 |
| `NAS_COOKIE_SECURE` | `false` | HTTPS 反向代理访问时设 `true`，直接 HTTP 使用 `false` |

下载参数以 `NAS_DOWNLOAD_` 开头，默认 `NAS_DOWNLOAD_ENABLED=false`；完整中文说明见 `.env.downloads.example` 和 `docs/downloads.md`。不要将 qB 密码写入前端、URL、Git 或日志。

下载和搜索也可显式设置 `NAS_DOWNLOAD_MODE` / `NAS_TORRENT_SEARCH_MODE` 为 `disabled`、`external` 或 `managed`，其优先级高于旧开关。托管参数与兼容规则见 `docs/engines.md`；默认不启用助手、不访问Docker。

Compose 对应的简化变量与中文说明见 `.env.example`。如使用反向代理，请转发原始 Host、支持 Range、避免缓存鉴权响应，并允许长时间流式响应。公网访问应先配置 HTTPS 和网络访问控制；不要将第一次管理员初始化页面直接暴露在公网。

## 验证

```sh
python3 -B scripts/project_env.py -- go vet ./...
python3 -B scripts/project_env.py -- go test ./...
python3 -B scripts/project_env.py -- go build ./cmd/nas
python3 -B scripts/project_env.py -- npm --prefix web run typecheck
python3 -B scripts/project_env.py -- npm --prefix web run build
docker compose config --quiet
```

可复用集成验证脚本仅操作隔离目录和指定应用。请使用独立数据目录，不要对真实业务实例执行：

```powershell
# 准备隔离媒体；--ffmpeg 可显式传入可执行文件路径。
python -X utf8 scripts/verify.py --prepare-only --large-file
# 在独立终端启动隔离实例，媒体根须与验证脚本的 --root 一致。
$env:NAS_DATA_DIR='.runtime/verify/data'
$env:NAS_MEDIA_ROOTS=(Resolve-Path '.runtime/verify/media').Path
go run ./cmd/nas
# 再执行接口验证；本机随机验证凭据保存在 .runtime/verify 下。
python -X utf8 scripts/verify.py --large-file
```

临时生成的单元测试文件在完成验证后按项目要求清理；保留集成验证脚本、结果和可执行契约。已执行与尚未执行的检查见 `docs/verification.md`。

## 代码组织

```text
cmd/nas/            服务启动入口
internal/app/       配置、存储、认证、索引、任务与媒体接口
web/src/            Vue 界面与播放器／阅读器
docs/api.md         前后端共享接口契约
scripts/verify.py   隔离样本与集成验收
.trellis/           PRD、设计、研究与项目开发规范
```

代码变更遵守中文方法／字段／配置注释、明确类型与中文业务日志。暂未自动提交 Git，请审阅验证后自行提交。
