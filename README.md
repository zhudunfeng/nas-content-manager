# 栖库 · NAS 内容管理

将视频、图片、漫画、小说和音乐放在同一个私人媒体库中。Vue 3 界面、Go API、SQLite 与后台任务通过一个应用容器部署，原媒体只读挂载，不需要额外安装数据库、Redis 或搜索服务。

当前为 `0.1.0` 首版源码。建议先在局域网和少量样本上试用，再接入完整媒体目录。当前没有已发布的公共镜像，Compose 会从本仓库构建。

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

视频实际可播放性取决于浏览器对容器、视频和音轨的支持。首版采用原片直放，不提供 HLS、转封装或硬件转码；不兼容时会提示使用兼容客户端／外部播放器。CBR、PDF、HEIC、RAW、DRM 图书、AI 整理、下载器和用户权限细分不在此版本范围。

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

## 备份、恢复与升级

在设置页下载数据库备份。接口创建一致性 SQLite 快照，避免直接复制正在使用 WAL 的数据库主文件。备份含账号、媒体索引、标签、收藏与进度，属于敏感资料，请妥善保存。原媒体须独立备份，可重建缓存不必纳入业务备份。

恢复必须停机进行：

1. `docker compose down`，保留原数据卷并另外备份其内容。
2. 找到挂载的 `/data`（可用 `docker volume inspect` 查询实际项目卷），将旧 `library.db`、`library.db-wal`、`library.db-shm` 一起移到单独保留目录；不要把旧 WAL 留给恢复的数据库。
3. 将下载的备份放为 `/data/library.db`，确保 UID/GID `1000:1000` 可读写。
4. `docker compose up -d`，登录并重新扫描。缓存可重建，路径映射需与原配置保持一致。

升级前保留当前源码版本、镜像、完整数据和一致性备份，再更新源码并运行 `docker compose up -d --build`。回滚时镜像与对应数据库快照一起恢复；不能假设旧镜像能读取未来数据库结构。

本次新增音乐沿用 v1 数据结构，已有四类索引、账户、收藏、标签和进度可继续使用，无需删除或重建数据库。音乐目录仍须放在已有授权挂载下；无需新增容器、服务或环境参数。

## 本机开发

需要 Node.js 22.14 或兼容版本、Go 1.26+、FFmpeg／ffprobe。工具缺失时仍可使用基础内容功能，音视频探测与封面能力受限。无需启动 Docker。

```sh
# 安装锁文件中的前端依赖并构建。
npm --prefix web ci
npm --prefix web run build
# 先创建 media 目录，或用 NAS_MEDIA_ROOTS 指定已有目录，再运行后端。
go run ./cmd/nas
```

默认网页地址为 `http://127.0.0.1:8090`。开发热更新另开终端运行 `npm --prefix web run dev`，Vite 的 `/api` 代理指向 `127.0.0.1:8090`。

PowerShell 快捷命令：

```powershell
# 启动后端；Windows 多个媒体根用分号分隔。
./scripts/dev.ps1 -Mode backend -MediaRoots 'E:/Media;F:/Books'
# 另开终端启动前端热更新。
./scripts/dev.ps1 -Mode frontend
# 生成 web/dist 与 bin/nas.exe。
./scripts/dev.ps1 -Mode build
```

本机构建的 Go 程序从 `NAS_WEB_DIR` 读取网页；移动程序时应连同 `web/dist` 部署。容器镜像已包含所有前端构建产物。

## 环境参数

普通使用只需修改媒体路径和端口；其余保留默认。启动时校验配置，非法范围会报中文错误并退出。

| 参数 | 默认值 | 中文说明 |
| --- | --- | --- |
| `NAS_ADDR` | `:8090` | HTTP 监听地址；容器固定监听 8090，由宿主映射端口 |
| `NAS_DATA_DIR` | `data` | 可写应用目录，SQLite、缓存与临时备份所在位置 |
| `NAS_MEDIA_ROOTS` | `media` | 可访问的媒体根；Linux 用冒号分隔，Windows 用分号分隔 |
| `NAS_WEB_DIR` | `web/dist` | 已构建网页目录 |
| `NAS_SCAN_INTERVAL_MINUTES` | `60` | 周期扫描间隔，单位分钟 |
| `NAS_CACHE_MAX_MB` | `1024` | 可重建封面缓存上限，单位 MiB |
| `NAS_MIN_FREE_MB` | `512` | 新增缓存所需最低可用空间，单位 MiB |
| `NAS_WORKERS` | `1` | 后台任务并发数，支持 1–4，低配置 NAS 使用 1 |
| `NAS_COOKIE_SECURE` | `false` | HTTPS 反向代理访问时设 `true`，直接 HTTP 使用 `false` |

Compose 对应的简化变量与中文说明见 `.env.example`。如使用反向代理，请转发原始 Host、支持 Range、避免缓存鉴权响应，并允许长时间流式响应。公网访问应先配置 HTTPS 和网络访问控制；不要将第一次管理员初始化页面直接暴露在公网。

## 验证

```sh
go vet ./...
go test ./...
go build ./cmd/nas
npm --prefix web run typecheck
npm --prefix web run build
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
