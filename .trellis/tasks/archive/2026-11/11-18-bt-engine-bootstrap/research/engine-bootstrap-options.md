# Research: BT 搜索与下载引擎初始化方案

- Query: 在现有 qBittorrent + Torznab 系统中，缺少引擎时如何由按钮触发受控拉取、部署、凭据初始化和就绪检测？
- Scope: mixed，现有代码/配置与 Docker、qBittorrent、Jackett、Prowlarr 发布方一手资料。
- Date: 2026-11-18；以下外部链接均于本日访问。
- Execution boundary: 仅研究和文档写入；未执行 Docker、未拉取镜像、未启动服务、未验证目标 NAS。

## Findings

### 1. 现有仓库事实与代码模式

| 文件/定位 | 已存在行为及其对方案的约束 |
| --- | --- |
| `compose.downloads.qbittorrent.yaml:6` | 当前推荐是 **qB 项目官方镜像** `qbittorrentofficial/qbittorrent-nox:5.2.4-1@sha256:92bfd78d731e254ba64f62b77b0696add6447a33642ee5feda106a26b285aa7f`，不是 LinuxServer 镜像。初始化应沿用已经做过协议探针的版本，不能改用浮动 latest。 |
| `internal/app/downloads.go:405` | 磁力元数据停止仅对 qB **5.2.4** 开放，其他版本给出明确错误；选择镜像时版本相容性是功能门槛。 |
| `internal/app/download_qbittorrent.go:220` | `probe` 会认证并读取 app/version 和 WebAPI version。初始化完成需走实际适配器认证，不能以容器 running 代替。 |
| `internal/app/download_qbittorrent.go:319` | `add` 对磁力使用 `stopCondition=MetadataReceived`，不允许初始化新版本后悄悄退回整包传输。 |
| `internal/app/download_config.go:54` | 启用下载时要求永久密码文件可读；先启用旧开关再等初始化会造成启动失败。托管模式必须允许未初始化并在就绪后激活。 |
| `internal/app/torrent_search_config.go:30` | 搜种配置启动时读取；启用时必须有 1–8 个有效来源。Jackett 空站点初始化状态不能伪装成有效 sources。 |
| `internal/app/config.go:72`、`internal/app/server.go:61` | 当前连接配置启动时载入，搜索 manager 与下载 coordinator 在 Run 中建立；点击初始化后无需重启需新增受控运行时配置切换。 |
| `compose.downloads.yaml` | 主应用只读 `/downloads`，qB 写同一实体根；complete 才能属于普通媒体根。保持 publication gate 与既有选集/入库路径。 |
| `compose.search.jackett.yaml:6` | 已有 LinuxServer Jackett 固定版本/摘要，可作为首版搜索模板；它不是 Jackett 项目官方镜像。 |
| `compose.yaml`、`Dockerfile` | 主应用默认非 root，不含 Docker CLI、不挂 Docker socket。浏览器无法为一个运行中的容器补加宿主目录或 socket 挂载。 |

检索先使用了本项目 CodeGraph（`status/query`），再针对已定位文件读取。旧 BT 与搜种改动尚未提交，本研究未改动它们。

### 2. 三种实现方式比较

| 方式 | 优点 | 具体代价 | 判断 |
| --- | --- | --- | --- |
| 主应用直连 Docker Engine socket | 少一个进程；可直接分阶段 pull/create/start/inspect。 | 主 Web 应用一旦可调用 Docker 管理接口，就具备改变宿主容器、挂载与网络的能力；只把 socket 挂成 `:ro` 不能限制 HTTP 写操作。和现有非 root/只读媒体应用边界冲突。 | 不推荐默认采用。 |
| **独立受限部署助手 + 固定 Engine API 模板** | Web 应用不持有 Docker socket；助手仅接受 `qbittorrent`/`jackett`、部署 ID、请求 ID 等固定枚举；可持久化拉取进度并对账恢复。 | 初次仍需手动叠加一次助手 Compose、授予助手 socket 权限并预挂目录；多一个小服务、状态库和升级责任。助手本身仍持有高权限，不能把它描述成完全沙箱。 | **本仓库推荐**。Linux 单机 NAS 首版。 |
| 预定义 Compose profile + 宿主手动命令 | 部署方式最简单，不需要应用或助手操作 Docker；复用现有 compose 覆盖文件。 | profile 决定服务是否参与 compose 启动，网页点按钮本身不能执行宿主命令；如果网页通过助手调用 compose，仍回到第二种，只是运行依赖更重。 | 保留为手动部署回退，不满足严格的一键自动安装主需求。 |

推荐助手固定镜像、固定端口、固定挂载与受管标签；不接受任意 Compose YAML、镜像、shell、host path、Docker API 路径或环境变量。Compose 初始包一次性声明主程序所需的只读下载根、只读连接目录和与助手通信的 Unix socket。引擎自己的 bridge 需允许访问注册表/站点/Peer 的出站路径，不能误用 `internal: true` 把 BT 和搜索隔离到无法联网。

### 3. Docker Engine API 能力与进度

官方 API 提供 image create/pull、container create/start/inspect、volume/network 创建和检查。`POST /images/create` 的 `fromImage` 接受 tag/digest，并可指定 platform；关闭该拉取 HTTP 连接会取消拉取。应固定允许的摘要、先探测 Docker API/主机架构，再发有界操作，不把 HTTP 200 等同于流内无错误。分层进度、未知总量、解包/校验阶段分别展示；总大小未确定时显示阶段/字节数，不虚构总百分比。

同名容器冲突时先 inspect 完整受管标签、镜像摘要、挂载、网络和身份。缺少本部署标签的现有容器禁止接管、覆盖或重建。API 离线、403、认证失败都不能证明没有容器。下载器 WebAPI 的离线/401 更不能证明引擎不存在。持久化命令意图后再发网络请求，超时后按资源 ID/标签对账，不能盲目重建。

参考：

- [Docker Engine API v1.47](https://docs.docker.com/reference/api/engine/version/v1.47/)，固定版本文档便于复现接口，并不要求宿主恰好使用该版本。
- [同版本 OpenAPI YAML](https://docs.docker.com/reference/api/engine/version/v1.47.yaml)，本次直接阅读确认 `/images/create`、`/containers/create`、`/containers/{id}/start`、`/volumes/create`、`/networks/create`。
- [Docker daemon attack surface](https://docs.docker.com/engine/security/#docker-daemon-attack-surface)，说明 Docker 管理权限的实际影响。
- [Docker bind mounts](https://docs.docker.com/engine/storage/bind-mounts/)，宿主路径按 **Docker daemon 所在机器**解释，不按调用容器的路径解释。
- [Compose profiles](https://docs.docker.com/compose/how-tos/profiles/)，profile 是 compose 执行时选择机制。

### 4. qB 首启凭据：首选离线预置，必须真实探针验证

#### 已证实

- 官方镜像文档明确：≥4.6.1 未设永久密码时生成临时密码并写 stdout；没有承诺通用 `QBT_PASSWORD` 初始化变量。
- 官方 entrypoint（研究时 main）把 profile 指向 `/config`，文件为 `/config/qBittorrent/config/qBittorrent.conf`；文件存在时不执行“缺文件时写默认配置”分支。该脚本与固定镜像构建源的对应关系仍需实施时确认。
- **qB release-5.2.4 源码**：`Preferences/WebUI/Password_PBKDF2` 是 `QByteArray`，PBKDF2 使用 HMAC-SHA512、100000 次迭代、16 字节随机 salt、64 字节派生输出；内容为 `base64(salt):base64(hash)`。
- 同版本源码保留 `Preferences/WebUI/Enabled`、`Username`、`LocalHostAuth`、`AuthSubnetWhitelistEnabled`、`CSRFProtection`、`HostHeaderValidation`。认证/CSRF/Host 校验不得作为安装方便的降级项。
- 官方镜像 `QBT_LEGAL_NOTICE=confirm` 的含义是用户已阅读上游法律告知，不能预填替用户确认。初始化弹窗应链接告知，只有用户明确确认后才能把该参数传给托管首次启动；若已有宿主显式确认配置，则保存来源与确认版本。

#### 建议方案（可验证设计，不宣称已经实测）

仅对本部署创建、确认全新且为空的 qB 配置目录：先生成随机永久密码并持久化到助手私有凭据文件，再生成 PBKDF2 和最小配置，最后启动 qB。密码不放环境变量/命令行/数据库/业务日志，主程序只获只读服务端连接文件。写文件须以独占创建、原子落盘、受控权限与进程 UID/GID 为准；非空配置绝不覆盖。

根据 release-5.2.4 的逻辑键，**待探针确认**的 QSettings INI 形式为：`[Preferences]` 组中的 `WebUI\Password_PBKDF2="@ByteArray(<salt-base64>:<hash-base64>)"`，以及 `WebUI\Username`、`WebUI\Enabled=true`、`WebUI\LocalHostAuth=true`、`WebUI\AuthSubnetWhitelistEnabled=false`、`WebUI\CSRFProtection=true`、`WebUI\HostHeaderValidation=true`。密码相关值是 Qt 字节数组，不是 bcrypt、MD5 或未类型化普通字符串；不要因为 main entrypoint 中出现新的分组形式就推断固定 5.2.4 镜像也使用该新形式。

实施前必须通过**同版本本机二进制的无 Docker 探针**验证实际 profile 路径、QSettings 转义、默认 WebUI 与端口、固定 Host/Origin、适配器认证、重启后同一永久密码仍有效。验证时可让 qB 原生保存一次并读取仅限键结构/脱敏的实际 INI 作为序列化证据。目标镜像和 NAS 仍由用户手动验证。登录失败即进入可恢复失败状态，保留配置和凭据，不自动解析日志、不关闭认证、不偷偷换版本。

#### 日志取密备选及代价

备选是仅助手有界读取受管新容器的当前启动日志，在内存提取临时密码，再调用 qB `app/setPreferences` 设置永久随机密码，并重新认证确认。官方 WebAPI 的 `web_ui_password` 对现代 API 是只写明文输入。此方案需要精确处理日志帧、TTY、日志驱动支持、重启密码变化、日志轮转和中途崩溃，且最初密码已经存在容器日志中；因此不作为首版默认或静默失败回退。不能将原始日志交给浏览器，也不能对已有外部实例改密码。

参考：

- [qB 官方镜像 README](https://github.com/qbittorrent/docker-qbittorrent-nox/blob/main/Readme.md)，注意文件名是 `Readme.md`。
- [qB 官方 entrypoint](https://github.com/qbittorrent/docker-qbittorrent-nox/blob/main/entrypoint.sh)。
- [qB 5.2.4 password.cpp](https://github.com/qbittorrent/qBittorrent/blob/release-5.2.4/src/base/utils/password.cpp)。
- [qB 5.2.4 preferences.cpp](https://github.com/qbittorrent/qBittorrent/blob/release-5.2.4/src/base/preferences.cpp)。
- [qB 5.2.4 settingsstorage.cpp](https://github.com/qbittorrent/qBittorrent/blob/release-5.2.4/src/base/settingsstorage.cpp)，QSettings 读写及原子替换路径。
- [Qt QSettings](https://doc.qt.io/qt-6/qsettings.html#Format-enum)，类型标记/转义/原子同步要求；它不是目标 qB 镜像实际 Qt 版本的测试证据。
- [qB 5.x WebAPI](https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-%28qBittorrent-5.0%29)，`setPreferences` / `web_ui_password`。
- [LinuxServer qB 发布方文档](https://docs.linuxserver.io/images/docker-qbittorrent/)，确认支持 amd64/arm64、PUID/PGID、临时密码；这是比较项，不建议本任务换镜像。

### 5. Jackett 首启、管理密码及来源自动同步

#### 配置位置与 API Key

Jackett `ConfigurationService.GetConfig<T>()` 使用 `DataFolder/<type-name>.json`；`BuildServerConfig` 调用 `GetConfig<ServerConfig>()`，因此是 **`ServerConfig.json`**。`GetConfigFile()` 返回的 `config.json` 属于另一个旧机制，不是应读取的当前服务器配置文件。缺失 APIKey 时生成 32 位随机值并保存。

LinuxServer 镜像设置 `XDG_CONFIG_HOME=/config`、`XDG_DATA_HOME=/config`，Jackett Unix `RuntimeSettings.DataFolder` 默认组合 ApplicationData 与 `Jackett`；因此固定模板下预期是 `/config/Jackett/ServerConfig.json`。如果以 `CustomDataFolder` 或 `RUN_OPTS` 改了位置，路径会变化；首版禁止任意 RUN_OPTS，实施时必须对所钉镜像实际确认路径。助手仅能读取其受管专用目录的这个已知有界普通文件，不做宿主递归搜密钥。

#### 管理密码

上游 `POST /api/v2.0/server/AdminPassword` 由服务器负责设置密码 hash；不能把明文直接写入 AdminPassword。当前源码 `SecurityService` 的 hash 是 UTF-16 编码的 `password + APIKey` 经 SHA512 的小写十六进制；这只是理解源代码，不建议在本功能重写该密码机制。首版沿用回环 WebUI + SSH 隧道，由用户在 Jackett 内设置管理密码和站点，不在栖库内代理其管理页面或暴露 API Key。

管理密码为空时上游会允许空密码登录，因此 UI 默认不发布至 LAN/公网，回环映射并不代表内网容器没有访问路径。助手网络只接本部署需要的组件；文档应提示首次设置管理密码，来源状态单列。修改管理密码不应该导致助手随后需要用户复制该密码才能同步来源。

#### 推荐的来源同步：Torznab `t=indexers`

`GET /api/v2.0/indexers?configured=true` 是管理控制器接口，受全局 Cookie 认证；**不能假定只加 `apikey` 就能通用认证**。

更合适的是上游 ResultsController 明确支持且带 `AllowAnonymous` + `RequiresApiKey` 的 Torznab 元索引器查询：

`GET /api/v2.0/indexers/all/results/torznab/api?t=indexers&configured=true&apikey=<server-only>`

该请求列出已配置索引器，XML `<indexer id="..." configured="true"><title>...</title>...`。仅后端/助手访问，不发起实际关键词搜索，不向浏览器返回 API Key、站点地址、description 或原始 XML。仅取通过现有 ID/标题校验的数据，构造各单索引器的既有 Torznab URL，按现有 8 来源上限选择/同步；超过上限明确提示，不静默扩大预算。这个发现接口与普通搜种 caps/category 协商不同，不改变现有 category 映射。

实施前必须用目标固定 Jackett 版本的无 Docker 二进制探针确认：空站点、配置站点、设置管理密码以后，上述 `t=indexers` 都仍可用 API Key 返回准确清单。若固定版本不支持，显式失败并保留旧来源；可另行设计助手对受管 `Indexers` 目录文件名的有界发现，但不将其作为任意文件扫描或静默回退。

安装 Jackett 只得到服务与 API Key，不会凭空有可搜站点。状态应拆分 `engineReady`、`needsSources`、`sourceReady`；用户完成站点配置后点击“同步来源”，通过当前来源文件严格校验和原子运行时切换生效。不自动添加公共站点、不代填账户、不绕过验证码。

参考（均为项目/发布方源码）：

- [Jackett ConfigurationService.cs](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Common/Services/ConfigurationService.cs)。
- [Jackett RuntimeSettings.cs](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Common/Models/Config/RuntimeSettings.cs)。
- [Jackett ServerConfigurationController.cs](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Server/Controllers/ServerConfigurationController.cs)。
- [Jackett IndexerApiController.cs](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Server/Controllers/IndexerApiController.cs)。
- [Jackett ResultsController.cs](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Server/Controllers/ResultsController.cs)，核实 `t=indexers`、`configured` 与 API Key 独立校验。
- [Jackett Startup.cs](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Server/Startup.cs) 与 [UIController.cs](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Server/Controllers/UIController.cs)，全局管理 Cookie 认证与 UI 登录。
- [Jackett SecurityService.cs](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Server/Services/SecurityService.cs)。
- [LinuxServer Jackett Dockerfile](https://github.com/linuxserver/docker-jackett/blob/master/Dockerfile) 与 [发布方 README](https://github.com/linuxserver/docker-jackett/blob/master/README.md)，XDG、/config、架构、AUTO_UPDATE 约定。

### 6. Prowlarr 与首版范围

Prowlarr 也需要配置索引器，具备 API Key 和 `/api/v1` 管理 API；首次认证/引导与数据库配置比目前 Jackett 配置文件和既有部署模板引入更多新边界。现有用户已连接 Prowlarr 继续支持“外部实例”，本轮推荐不同时自动托管两种搜索管理器。首版只托管 Jackett，避免为相同需求实现两套初始密钥/管理认证/索引器发现。

参考：[Prowlarr quick start](https://wiki.servarr.com/prowlarr/quick-start-guide)、[官方 API](https://prowlarr.com/docs/api/)。后续扩展可另增固定模板，不开放任意镜像安装。

## Related Specs

- `.trellis/spec/backend/runtime-contract.md`：单管理员、SQLite v2、无事务跨网络、错误与日志边界。
- `.trellis/spec/backend/download-contract.md`：任务所有权、固定版本元数据停传、目录门禁、同实体下载根与完成区。
- `.trellis/spec/backend/torrent-search-contract.md`：1–8 私密来源、64 KiB 配置、XML/请求预算、浏览器只获安全元数据。
- `.trellis/spec/frontend/app-contract.md`：单对话框、轮询与取消、五项移动导航和主题。

## Caveats / Not Found

- 这是静态代码与一手文档研究，不能声称已验证离线 PBKDF2 配置、实际镜像入口脚本、Jackett 目标版本 `t=indexers`、Docker UID/GID 或 NAS 挂载。
- qB **release-5.2.4** 算法事实已核实；官方镜像 main 脚本和 Jackett master 源码是研究日现状，实施前须对应到固定镜像发布源码与真实二进制探针。
- 计划中不执行 Docker；用户手动验收拉取、容器启动、回环端口、主机架构、ACL、rootless/NAS 兼容、磁盘和真实下载/播放争用。
- GitHub REST 未认证查询遇到限流，已改为官方页面和 raw 文件读取；未借助凭据或改变外部服务状态。
