# BT 引擎一键初始化

栖库可按需部署固定版本的 qBittorrent 下载器和 Jackett 搜索服务。在下载中心打开“引擎管理”，确认首次使用告知后点击初始化，即可查看检测、镜像层进度、启动及应用接入结果。Jackett 安装完成后还需要自行配置站点，再点击“同步来源”。

## 第一次启用

适用于同一 Linux Docker 主机上的 amd64/arm64 部署。基础 `compose.yaml` 仍只有应用服务。自动部署需要先加一次可选助手及挂载配置，网页无法给运行中的容器追加宿主卷，也不会替你安装 Docker。

```sh
# 复制参数模板；填写Docker宿主上的真实专用路径，勿填写原媒体根。
cp .env.engines.example .env.engines
# 编辑 .env.engines 中的 ENGINE_HOST_ROOT、MEDIA_PATH、端口和UID/GID。
mkdir -p /srv/nas/bt
# 先检查合并配置，再构建并启动应用和私有助手。
docker compose --env-file .env.engines -f compose.yaml -f compose.engines.yaml config --quiet
docker compose --env-file .env.engines -f compose.yaml -f compose.engines.yaml up -d --build
```

不要同时叠加旧的 `compose.downloads*.yaml`、`compose.search*.yaml`。已有引擎应继续按 [下载部署](downloads.md) 或 [搜索部署](torrent-search.md) 使用 external 模式，助手不会接管已有实例或把连接失败当成“未安装”。

助手持有真实 Docker 管理权限，必须视为受信任的部署组件。Docker socket 只挂给助手，主应用不挂载它。助手仅通过权限受限的 Unix socket 接受固定组件操作，没有 TCP 控制端口、任意镜像/命令入口或 Docker API 代理；这些接口限制并不构成 Docker daemon 的权限沙箱。不要把 daemon 的未认证 TCP 接口用于正式部署。

## 页面操作

1. 打开下载中心 → 引擎管理，先查看下载、搜索各自状态。
2. 对明确检测为缺失的组件选择初始化；新建 qB 必须主动勾选首次使用告知。只选 Jackett 不需要确认 qB 告知。
3. 等待任务结束。层进度只展示 Docker 实际报告的字节数，没有虚构整体百分比。刷新或关闭页面不取消安装。
4. qB 通过固定版本、永久凭据与应用生产适配器认证后才能下载；不会自动添加任何 BT 任务。
5. Jackett 若显示“待配置来源”，按下一节配置站点，然后回栖库同步。最多启用八个来源；发现更多时明确选择，不静默截取。

助手完成部署与应用连接成功是不同状态。镜像存在、容器 running 或文件发布成功，都不等于搜索/下载已经可用。助手暂时离线不会停掉已接入的下载、搜索或本地媒体读取。

## 配置 Jackett 来源

管理入口只绑定 **Docker宿主的127.0.0.1**，默认端口19117。不要用远程工作站的 localhost 直接代替宿主回环地址。

```sh
# 从工作站转发宿主回环管理端口；nas-user与nas-host替换为实际SSH账号和主机。
ssh -N -L 19117:127.0.0.1:19117 nas-user@nas-host
```

打开工作站的 `http://127.0.0.1:19117`，先在 Jackett 设置管理密码，再添加自己有权使用的站点。栖库不会提供账号、绕过验证码或代理管理页面。配置后点击“同步来源”；后台使用带 API Key 的 Torznab 索引器发现，浏览器只收到来源编号与名称，无需手工复制 Key。初始化/同步不会执行实际关键词搜索。

## 参数与模式

所有部署参数的逐项中文解释在 `.env.engines.example`、`compose.engines.yaml` 中。修改固定根、网络或UID/GID属于运维变更，不可从网页提交。

| 参数 | 默认值或约束 | 中文解释 |
| --- | --- | --- |
| `NAS_DOWNLOAD_MODE` | 未设置时沿用旧开关 | 下载模式：disabled关闭、external外接、managed托管；显式模式优先 |
| `NAS_TORRENT_SEARCH_MODE` | 未设置时沿用旧开关 | 搜索模式，取值及优先级同下载 |
| `NAS_ENGINE_HELPER_SOCKET` | 空值 | 应用的私有Unix socket路径；托管Compose设为`/run/nas-engine-helper/helper.sock` |
| `NAS_ENGINE_CONNECTION_DIR` | `/run/nas-engines` | 应用只读的版本化连接目录，原子替换整个目录中的文件可见 |
| `ENGINE_HOST_ROOT` | 必填 | Docker宿主专用绝对根，必须精确对应助手`/managed`的bind源，不得为根目录或媒体目录 |
| `ENGINE_STORAGE_MODE` | `bind` | 存储驱动；现有Linux部署保持bind，Desktop专用配置使用volume |
| `ENGINE_VOLUME_PREFIX` | bind模式为空 | volume模式的受信任实例前缀，派生六个固定角色命名卷；不可从网页提交 |
| `ENGINE_NETWORK` | Compose为`nas_engine_bridge` | 应用、助手和引擎共享的真实bridge名称，需允许必要出站 |
| `ENGINE_UID` / `ENGINE_GID` | `1000` / `1000` | 下载与搜索进程的目标身份；GID也控制应用读取连接文件和socket的权限 |
| `QBT_PEER_PORT` | `6881` | qB的TCP/UDP传输端口，须未被其他容器占用，不是WebUI端口 |
| `JACKETT_SETUP_PORT` | `19117` | Jackett宿主回环管理端口，远程使用SSH转发 |
| `DOWNLOAD_MIN_FREE_GB` | `2` | Compose传给下载协调器的磁盘预留GiB |
| `ENGINE_MANAGED_ROOT` | `/managed` | 助手内部专用根；标准Compose保持此默认值 |
| `ENGINE_HELPER_SOCKET` | `/ipc/helper.sock` | 助手监听位置，须通过同一IPC卷与应用路径对应 |
| `ENGINE_DOCKER_SOCKET` | `/var/run/docker.sock` | 助手连接的同宿主Unix socket，不支持远端daemon产品模式 |

未显式设置新模式时，旧 `NAS_DOWNLOAD_ENABLED=true` / `NAS_TORRENT_SEARCH_ENABLED=true` 对应 external，false对应disabled；旧外接凭据校验继续严格执行。managed允许首次没有连接文件而先启动媒体服务。应用非root身份须与连接读取组匹配；自定义GID时要一并调整应用运行组，不能只改助手变量。

托管模板固定 qB `5.2.4-1` 与 Jackett `v0.24.3065-ls36` 的多架构摘要，集中在 `internal/enginehelper/templates.go`。不自动升级、不回退latest。已有镜像加速由宿主Docker配置控制；阿里云等加速服务也必须保留目标镜像摘要，不应通过改版本或关闭摘要验证绕过拉取失败。助手不会修改daemon镜像源或代理。

## 文件与权限

`ENGINE_HOST_ROOT`下保存：

```text
state/          助手SQLite、安装身份、任务、安装级进程锁与持久凭据
connections/    版本清单current.json及只供主应用读取的连接文件
qbittorrent/    当前安装的qB配置和恢复数据
jackett/        当前安装的Jackett配置和来源
downloads/      staging与complete；只有complete可登记媒体库
```

具体凭据文件名由实现管理，不要手工改写。state为root私有目录，连接文件只给应用读取组；下载卷在应用中只读、qB中可写。Jackett没有下载卷；两引擎都没有原媒体和主应用数据库挂载。备份和日志工具同样不得收集或展示密码、API Key、Cookie及原始引擎日志。

qB在新空配置上离线生成并持久保存随机永久密码，使用固定版本的PBKDF2格式，随后重新认证验证；不读取临时密码日志、不关闭认证。引擎同名但标签/镜像/挂载/网络/端口不符，或目录含未知内容时，显示冲突并保留现场。不会自动chmod777、重置配置或停止外部容器。

## 失败与恢复

| 页面或任务结果 | 处理方法 |
| --- | --- |
| 助手未启用/不可用 | 核对Compose覆盖文件、socket卷与GID；不会因此显示引擎未安装 |
| 拉取失败/限流/鉴权错误 | 检查宿主出站、可信镜像加速及磁盘后重试同一任务；不删现有配置 |
| Jackett的`pwritev2`预检被安全策略拒绝 | 升级Docker及兼容安全策略后重试；不自动使用`seccomp=unconfined`或特权容器 |
| UID权限/端口/归属冲突 | 核对专用根和实际UID、空闲端口及旧安装身份；不会接管未知目录 |
| qB认证/固定版本失败 | 保留永久凭据和配置，核对完整恢复数据后重试，不能改为关闭认证 |
| 任务部分失败 | 成功组件保留，任务重试只处理失败步骤 |
| 未配置站点/超过八个来源 | 在Jackett配置后同步；超过上限由用户明确选择 |
| 清单损坏/新实例身份不符 | 已接入连接保留，核对并恢复完整安装；不静默换实例 |

本次在Docker20.10.3、3.10内核的授权服务器上，默认seccomp使`pwritev2`返回EPERM，阻断Jackett的.NET写入回退。预检会在拉镜像前明确失败。临时诊断放宽策略只用于定位，不是推荐部署方式；不能把诊断结果解释为该机默认部署成功。更多真实验收边界见 [验证记录](verification.md)。

## 备份、恢复与停用

普通应用数据库快照不包含引擎密钥、引擎状态和下载文件。完整备份应协调停止写入，分别保存主应用一致性快照、整个助手state与安装ID、connections、两引擎配置/恢复数据、下载根及部署参数。使用受控的加密备份存储；勿把这些文件加入Git。

恢复时先停止相关服务，保留当前现场，再还原相互匹配的整套目录、权限和安装身份，确认卷、网络、端口相同后启动并对账。不要删除安装ID或`helper.lock`来强行接管容器；锁文件持久存在是正常现象，实际锁由进程退出释放。已发布媒体仍受原选集与版本门禁约束。

停用控制面可移除托管覆盖文件并显式改回disabled或原external配置。Compose停止助手不会自动停止其动态创建的引擎容器；可先调用固定的私有停止命令，再停止Compose服务：

```sh
# 使用与启动时相同的参数文件和覆盖文件，等待本实例两引擎停止成功。
docker compose --env-file .env.engines -f compose.yaml -f compose.engines.yaml exec -T nas-engine-helper /nas-engine-helper stop-all
# 仅在前一命令成功后停止应用与助手；保留全部配置和下载数据。
docker compose --env-file .env.engines -f compose.yaml -f compose.engines.yaml stop
```

停止命令复用运行中助手的Unix socket，最长等待90秒，失败返回非零状态。初始化/启动正在执行时会拒绝停止；核实任务结果后重试。停止前检查实际容器、镜像和挂载身份，不停止同名外来容器，不拉取、创建或删除资源。接受停止后，本次助手进程拒绝新初始化/启动，避免关机竞态；重新启动助手后可恢复管理，再使用页面上的“启动”启动已停引擎。首版没有一键卸载、自动删数据、全局prune或自动升级。

## Windows Docker Desktop 命名卷

Windows完整模式使用独立的`compose.desktop.yaml`，由本机启动器核验固定镜像清单、实例和Compose配置。不要将它与Linux bind覆盖文件混用。使用方法见[Windows启动说明](windows.md)。

此模式将`state`、`qbittorrent`、`jackett`、`downloads`、`connections`、`ipc`放入六个具有固定角色和实例标签的Linux命名卷。卷驱动必须为不带设备/NFS/bind选项的`local`。助手还核验卷内持久安装标记与SQLite身份；未知非空卷或身份不一致时保留现场并拒绝接管。媒体根仍只读，应用数据库另用独立卷。

下载位于Docker Desktop Linux数据盘，不是Windows普通下载目录。完整备份应停写并同时保存上述相关卷、应用数据卷和本机`desktop.json`；不要只恢复助手数据库或复制其他实例的标记。启动器的“停止完整模式”会先执行上述固定停止协议，成功后再停止应用和助手。单纯退出托盘保留容器运行。
