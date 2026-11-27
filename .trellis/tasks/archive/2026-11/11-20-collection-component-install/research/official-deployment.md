# Research: 四类资源组件的固定部署与首次接入

- Query: 当前平台如何受控触发 MoviePilot、AutoBangumi、Sonarr、Radarr 的部署，并正确区分运行、首次配置和 API 可连接状态。
- Scope: mixed（现有 helper/外部 API 契约 + 官方项目固定版本源码、镜像发布方源码及 Docker Hub 元数据）
- Date: 2026-11-20

## Findings

### 1. 结论与本轮固定部署选择

- 复用私有 Docker helper 的固定模板、持久任务、归属校验和重启恢复。浏览器仅选择四个产品，不接收镜像、命令、宿主路径或容器 JSON。
- 四类镜像均已从 Docker Hub tag API 核验 manifest 摘要与 `linux/amd64`、`linux/arm64`。下列是与现有 API 适配器匹配的固定版本，不声称所有产品的最新主版本。
- 按主任务决定，仅回环发布 `19130→3000`（MoviePilot）、`19132→7892`（AutoBangumi）、`19133→8989`（Sonarr）、`19134→7878`（Radarr）。MoviePilot 的 3000 nginx 已代理 API，无需额外发布 3001。
- 四组件使用各自独立配置/状态目录，固定挂载既有下载根 `/downloads` 和独立资源根 `/data`。最终实现采用复制整理到 `/data/library/<组件>`；两者是不同挂载，不承诺跨卷硬链接或原子移动，也不把现有整个媒体库授予写权限。下文发布方关于统一数据挂载的说明属于上游建议，不代表本平台的最终存储模型。
- 不预生成/猜测应用管理凭据，不修改已有实例账号。容器启动后引导打开组件完成首次配置，再通过现有“地址/令牌→测试连接”流程启用 API 管理。
- “部署完成，待首次配置”是有效的完成状态；不能将其显示为搜索站点、下载器、整理目录、媒体服务器全部可用。

### 2. 已核验的固定镜像目录

| 产品 | 固定镜像引用 | 已核验平台 |
| --- | --- | --- |
| MoviePilot | `jxxghp/moviepilot-v3:3.2.8@sha256:51cf8fe90a474bde2d79bbbccb5ce83f926b4da887b6fcc7c9457730095d6ebb` | linux/amd64、linux/arm64 |
| AutoBangumi | `estrellaxd/auto_bangumi:4.0.0@sha256:d2e2955b5e4caf345b4325db7bee150e861afaae5911181e5bd5646ddbcb66be` | linux/amd64、linux/arm64 |
| Sonarr | `linuxserver/sonarr:4.0.20.3014-ls5@sha256:c7d1df25e428da6c9ace05daea0d34e59522978a3843ea1944812f855af4086c` | linux/amd64、linux/arm64 |
| Radarr | `linuxserver/radarr:7.0.3.10918-ls328@sha256:044788f5508b7f67a3e0368cdd142c77c4d4cd5aec2efaba53e1a68b6b6231e7` | linux/amd64、linux/arm64 |

2026-11-20 直接获取并核验的发布方元数据：

- https://hub.docker.com/v2/repositories/jxxghp/moviepilot-v3/tags/3.2.8
- https://hub.docker.com/v2/repositories/estrellaxd/auto_bangumi/tags/4.0.0
- https://hub.docker.com/v2/repositories/linuxserver/sonarr/tags/4.0.20.3014-ls5
- https://hub.docker.com/v2/repositories/linuxserver/radarr/tags/7.0.3.10918-ls328

Sonarr/Radarr 使用 LinuxServer.io 维护的容器包装，应称“发布方维护的镜像”，不能声称其为 Sonarr/Radarr 项目自有官方容器。AutoBangumi 官方文档使用 GHCR；此处 Docker Hub 发布方仓库摘要已单独核验，不能把同一摘要直接替换到 GHCR/LSCR 而不核验该注册表。

### 3. 四服务探针与首次配置边界

| 产品 | 匿名/固定公开探针 | API 接入验证 | 首次配置限制 |
| --- | --- | --- | --- |
| MoviePilot 3.2.8 | `GET /api/v1/system/global?token=moviepilot` 可识别 `BACKEND_VERSION`；固定 query 是公开产品标识，不是管理密钥。镜像内部另有 `GET http://127.0.0.1:3001/health/ready` | `X-API-KEY` 调用 `/api/v1/system/ping`，并保持现有版本门控 | V3 首次启动无部署密码；必须在初始化页面创建管理员、密码、API Key。匿名版本或容器 health 不证明管理员存在或业务已配置 |
| AutoBangumi 4 | `GET /health`，要求 JSON `status="ok"`、`db_ok=true`、预期版本；HTTP200 单独不足 | `Authorization: Bearer <scope=api token>` 调用 `/api/v1/status`，版本与 health 一致 | 默认 `admin/adminadmin`，首次登录应修改密码并设置下载器；由 UI 创建 API scope 令牌。程序暂停不等于认证失败，不自动启动任务 |
| Sonarr 4 | `GET /ping`，要求 HTTP200 与 JSON `status="OK"` | `X-Api-Key` 调用 `/api/v3/system/status`，验证 `appName` 与主版本4 | 用户在组件 UI 完成认证、下载器、索引器、目录/配置等设置，再复制 API Key。匿名 ping 不证明这些配置完成 |
| Radarr 7 | `GET /ping`，要求 HTTP200 与 JSON `status="OK"` | `X-Api-Key` 调用 `/api/v3/system/status`，验证 `appName` 与主版本7 | 同上；当前项目不据 Sonarr5 或 Radarr 其他主版本推断写操作兼容 |

所有探针只读、有限超时/响应大小，禁止重定向；无需查询真实搜索站点、添加订阅、启动下载或运行整理来证明“连接成功”。对未配置实例，UI 应显示“打开组件完成首次配置”，随后让用户录入实际 API 令牌；不解析容器日志提取密码。

### 4. MoviePilot v3.2.8 固定源码依据

源码包： https://codeload.github.com/jxxghp/MoviePilot/zip/refs/tags/v3.2.8

- `docker/Dockerfile:44-46`：时区、HOME、`CONFIG_DIR=/config`；`:395-398`：只声明3000，持久化 `/config`，内置 `/health/ready` 检查（间隔30s、超时5s、启动期60s、重试3），tini 启动。基础 SQLite 模式无强制外部数据库/Redis。
- `app/runtime/config.py:199-200`：API3001、nginx3000；`:215-216`：`SUPERUSER` 默认空且由初始化页面设置；`SUPERUSER_PASSWORD` 已不再从部署配置初始化，密码哈希存数据库。
- `app/runtime/config.py:218-221`：API_TOKEN 为敏感字段；`:128` 起的规范化器拒绝过短 token，不应猜固定默认令牌。
- `app/runtime/config.py:414-424`：`MOVIEPILOT_AUTO_UPDATE=false` 默认只控制检查稳定版本提示；`MOVIEPILOT_UPDATE_DEV=false` 控制启动跟踪开发分支。不要套用旧版“自动更新”语义。`:425` 资源包更新是另一个配置。
- `docker/nginx.template.conf:35-48`：监听 nginx 端口，内部上游 `127.0.0.1:${PORT}`；`docker/nginx.common.conf:55-60`：`/api` 代理到内部上游。因此用户入口和 API base 可共用3000。
- `app/api/endpoints/login.py:43-65`：初始化状态只读接口；`:69-85`：首次初始化原子创建管理员并保存 API Key，已初始化返回409。此处不是常规安装探针必须执行的写操作。
- `app/schemas/initialization.py:10-23`：初始化状态及用户名/密码/API Key 提交模型。不能将 MP2 临时日志密码或环境变量密码流程用于 MP3。

镜像无需 privileged、Docker socket 或 host network 来完成普通基础安装。下载、整理、刮削仍需要用户配置站点、下载器、目录和元数据访问；媒体库刷新还需要配置媒体服务器。

### 5. AutoBangumi 4.0.0 固定源码依据

源码包： https://codeload.github.com/EstrellaXD/Auto_Bangumi/zip/refs/tags/4.0.0

- `Dockerfile`：应用端口7892，持久化 `/app/config`、`/app/data`，PUID/PGID/UMASK/TZ 可配置，tini/su-exec 启动；镜像 health 使用 `/app/healthcheck.py`，不能因此省略应用 JSON 的 db_ok 校验。
- `docs/resource/docker-compose/AutoBangumi/docker-compose.yml:3-12`：GHCR镜像、两个状态挂载、bridge、7892及 unless-stopped。在线更新通过退出容器让 Docker 重启生效；平台固定镜像引用不能证明用户后续没有执行组件内更新。
- `docs/deploy/quick-start.md:114-117`：初始后台暂停、默认管理员及立即改密要求。`backend/src/module/database/user.py:134-140` 也证实空用户表创建默认管理员。
- `backend/src/module/api/setup.py:98-101`：初始设置仍受默认管理员状态约束。未发现受支持的 `AB_USERNAME`/`AB_PASSWORD` 环境变量可安全替换 Web 管理员默认凭据；不要发明这些变量。外部入口应继续限制回环，在用户完成配置前不自动扩大访问面。
- `backend/src/module/application/auth.py:51-55`、`backend/src/module/security/api.py:73-105`：Bearer API token 必须 `scope=api`，登录会话 cookie 与 API 令牌不同。`AB_DEV_NO_AUTH` 是开发绕过开关，生产模板禁止设置。
- `docs/config/downloader.md:75` 附近：下载器初始环境变量仅无配置文件时读取，保存为插件实例；它们是下载器凭据，不是 Web 管理员引导。
- `docs/config/manager.md:73` 附近：本地硬链接插件默认关闭。RSS/qB 编排不必持有现有整库；需要本地整理时再使用受管 `/data` 映射。

不要因官方示例附带 qB 而自动创建第二个下载器或接管当前下载任务。平台安装 AB 后仍由用户配置实际下载器与规则。

### 6. Sonarr/Radarr 固定源码与容器包装依据

源码包：

- https://codeload.github.com/Sonarr/Sonarr/zip/refs/tags/v4.0.20.3014
- https://codeload.github.com/Radarr/Radarr/zip/refs/tags/v7.0.3.10918
- https://codeload.github.com/linuxserver/docker-sonarr/zip/refs/tags/4.0.20.3014-ls5
- https://codeload.github.com/linuxserver/docker-radarr/zip/refs/tags/7.0.3.10918-ls328
- 发布方部署文档：https://docs.linuxserver.io/images/docker-sonarr/ 与 https://docs.linuxserver.io/images/docker-radarr/

Sonarr：

- 包装仓库 `Dockerfile:49-52`：8989、`/config`；`README.md:104-115`：PUID、PGID、TZ、unless-stopped及可选媒体挂载。
- `README.md:70-74`：分开 `/tv`、`/downloads` 虽容易入门，会失去硬链接与原子移动。固定共享 `/data` 比把它们作为两个不同挂载更合适。
- `root/etc/s6-overlay/s6-rc.d/svc-sonarr/run:7-15`：以 abc 运行 `-nobrowser -data=/config`；`data/check:3-7` 使用 `/ping` 和 `status=OK`。这是 s6 readiness，不应假定 Docker inspect 必然存在 HEALTHCHECK 状态。
- 项目 `src/Sonarr.Http/Ping/PingController.cs:23-43`：匿名 `/ping`，会查询配置仓库以检测数据库；异常500。
- `src/Sonarr.Api.V3/System/SystemController.cs:56-82`：系统状态/版本/认证/数据库信息。
- `src/NzbDrone.Core/Configuration/ConfigFileProvider.cs:186-198`：API Key 优先使用 AuthOptions，否则持久配置自动生成；`:202-223`：认证方法与要求单独配置。无需读日志猜 key。

Radarr：

- 包装仓库 `Dockerfile:48-51`：7878、`/config`；`README.md:105-116`：PUID、PGID、TZ、unless-stopped及可选媒体挂载。
- `README.md:71-75` 同样指出分开 `/movies`、`/downloads` 的硬链接/原子移动限制。
- `root/etc/s6-overlay/s6-rc.d/svc-radarr/run:5-13`：abc、`-nobrowser -data=/config`；`data/check:3-7` 读取 `/ping` 的 `status=OK`。
- 项目 `src/Radarr.Http/Ping/PingController.cs`：匿名 `/ping`；`src/Radarr.Api.V3/System/SystemController.cs:59-86`：认证的产品/版本系统状态。
- `src/NzbDrone.Core/Configuration/ConfigFileProvider.cs:195-207`：API Key 来自 AuthOptions 或持久配置自动生成；`:211-243`：认证方法与要求单独配置。

两个固定版本都支持配置对象/环境变量绑定（`src/NzbDrone.Host/Bootstrap.cs`，Sonarr:254-258、Radarr:270-274；`src/NzbDrone.Common/Options/AuthOptions.cs:5` 有 ApiKey）。本轮不需要利用此机制注入凭据，也不应把 API Key 当作 Web 管理员密码或认证配置。用户 UI 首次设置与 API 连接作为两个明确步骤更稳妥。

### 7. 仓库文件与现有代码模式

| 文件 | 用途与应复用的模式 |
| --- | --- |
| `.trellis/spec/backend/engine-bootstrap-contract.md` | 固定 helper 模板、私有 socket、归属验证、持久幂等、拉取校验、状态不产生副作用；已有安装不接管 |
| `.trellis/spec/backend/external-collection-contract.md` | 四服务版本门控、API 凭据私有存储、现有外部管理和独立完成目录导入 |
| `.trellis/spec/frontend/app-contract.md:49-51` | 三步接入、能力由服务端返回、MoviePilot 六阶段及独立整理/刮削/刷新语义 |
| `internal/enginehelper/templates.go:23` | 受信任固定镜像目录；浏览器不能覆盖 |
| `internal/enginehelper/templates.go:79` | 固定 makeSpec、专用目录、bridge、回环管理端口、UID/GID、轮转日志 |
| `internal/enginehelper/templates.go:132` | validateContainer 核对标签、镜像、全部挂载、网络和端口，不能只凭容器名称 |
| `internal/enginehelper/config.go:15-30` | bind/volume、专用根、私有 socket、固定网络配置 |
| `internal/externalapi/moviepilot.go:56` | 公开版本加认证 ping，主版本及精确3.2.8能力区分 |
| `internal/externalapi/autobangumi.go:39` | health 的 db_ok +认证状态+版本一致性 |
| `internal/externalapi/servarr.go:111` | `/api/v3/system/status` 的 appName、主版本与认证校验 |
| `internal/externalapi/client.go:123` | 现有生产 Probe；部署后不能绕过它直接声称 API 接入完成 |

## Related Specs

- `.trellis/spec/backend/engine-bootstrap-contract.md`
- `.trellis/spec/backend/external-collection-contract.md`
- `.trellis/spec/frontend/app-contract.md`
- `.trellis/tasks/11-20-collection-component-install/prd.md`
- `.trellis/tasks/11-20-collection-external-api/research/moviepilot-autobangumi-api.md`
- `.trellis/tasks/11-20-collection-external-api/research/moviepilot-workflow-api.md`

## Caveats / Not Found

- 本次只读检查固定官方源码 ZIP、发布方元数据和仓库代码，ZIP 仅在内存中解读；没有执行外部源码，没有拉取/启动实际镜像、安装真实组件、访问用户服务或变更媒体。镜像可运行性、内存/CPU需求、初始化耗时和默认安全设置兼容性仍需隔离环境验证。
- 镜像固定摘要确定拉取内容，但应用内更新、持久配置、站点资源包或插件仍可能改变实际运行行为。后续 Probe 必须继续核验真实版本；不要因容器标签相同而省略版本门控。
- 未核验四镜像在当前 helper 的 no-new-privileges、UID/fsuid和实际 NAS 挂载组合下全部可运行；不得通过自动 privileged、公开 Docker socket、关闭 seccomp 等方式绕过失败。
- 匿名运行探针证明可响应及部分数据库状态，不证明认证配置、站点索引、下载器、TMDB网络、可写媒体目录或媒体服务器已就绪。首次部署可完成于“待配置”，接入以生产 API Probe 为准，业务能力以实际配置和现有接口结果为准。
- `/data` 是本项目受控映射选择，不是四产品约定的强制默认媒体目录；用户仍需在组件中选取正确子目录。完成目录导入保留既有独立预览与注册流程。
- GitHub releases API 曾返回403；未据此推断 latest。固定 tag 源码和对应 Docker Hub tag 元数据已直接取回。官方网页抓取未取得完整正文的部分，使用固定版本仓库中的官方部署文档/源码补证。
