# Research: BT 下载引擎与安全入库边界

- Query: 为栖库增加 BT 种子／磁力下载，选择可部署的成熟引擎并核验 WebAPI、完成发布、超大文件与故障恢复契约。
- Scope: mixed；官方资料、固定版本源码与仓库接入点。
- Date: 2026-11-17。
- 交付边界：仅研究，不实现代码，不下载资源，不启动下载器，不运行 Docker。

## Findings

### 1. 可直接用于方案收敛的主要结论

1. **推荐 qBittorrent 独立引擎 + Go 服务端适配器。** WebAPI 覆盖添加、文件优先级、速度、做种、位置迁移和状态对账；首期只实现 qB，保留内部接口供后续扩展。已有 qB 与可选 Compose sidecar 使用同一适配器。该选择是针对本仓库的工程建议，不是对其他引擎质量的否定。
2. **qBittorrent 有官方容器镜像。** 镜像为 `qbittorrentofficial/qbittorrent-nox`，维护仓库为 `qbittorrent/docker-qbittorrent-nox`；LinuxServer 镜像是独立的第三方发行。官方 README 没有承诺可用某个 PASSWORD 环境变量直接初始化 WebUI 密码。首次临时密码需要从日志取得，然后设置固定密码；不能将“只填环境变量、从不开放设置入口、自动获得固定密码”当作现成功能。
3. **100% 和迁移接口 HTTP 200 都不是入库凭据。** 必须同时核验元数据、被选文件完成、非校验／迁移／报错状态、预期路径和文件可读性；再更新栖库持久化 ready 门禁，扫描器只开放已完成的受管子树。
4. **超大文件路径应由引擎管理，栖库不复制媒体。** 同一文件系统内由 qB `setLocation` 从 staging 发布到 complete，有机会使用重命名；跨文件系统由 libtorrent 执行复制／搬移，耗时和空间不可忽略。不能笼统宣称任何挂载布局都零复制或 O(1)。
5. **下载生命周期与现有扫描工作池分离。** qB 持有传输与 resume 数据；栖库持有任务意图、目标库、选中文件、导入状态和引擎身份，重启后对账。网络失联显示离线／数据过期，不能把任务直接判为下载失败或重复添加。入库只触发现有扫描／增强链路，不占用扫描执行器等待 BT。

### 2. 仓库接入依据

通过 CodeGraph CLI 查询接入点；以下行号以研究时索引为准：

| 文件与位置 | 当前契约与规划影响 |
| --- | --- |
| `internal/app/jobs.go:222` `App.runScan` | 目录遍历仅按类型筛选，现无下载完成门禁；接入前必须对 managed complete 子树增加 ready／manifest 过滤，不能仅靠不触发自动扫描来隔离半成品。 |
| `internal/app/jobs.go:318` `App.indexItem` | `UNIQUE(library_id,relative_path)` 增量登记，用户状态分离；沿用稳定路径避免重扫生成重复条目。 |
| `internal/app/libraries.go:266` `App.enqueueScan` | 同一媒体库限制一个活跃扫描；导入需合并通知，并处理“现有扫描已越过新目录”的后续补扫，不能把已有扫描完成一概视作本次导入成功。 |
| `internal/app/media.go:13` `App.openSource` | 使用受控根目录与相对路径打开文件；受管下载还需 ready、路径与 selected manifest 检查，不能仅在列表层隐藏。 |
| `internal/app/store.go:81` `schemaV1` | 当前只有账号、媒体、用户状态、进度和 scan/enrich jobs；下载持久状态需新增增量迁移，不能复用 scan/enrich 状态的含义。 |
| `internal/app/config.go:28` `LoadConfig` | 所有参数启动校验；新增后端下载器凭据与目录参数要有中文解释、显式边界，敏感值不得回传前端。 |
| `compose.yaml` | 现有 `/media:ro` 与本机 SQLite 卷；建议保持原挂载，不把所有媒体改为可写。 |
| `.trellis/spec/frontend/app-contract.md` | 原石墨／青色明暗主题、手机底栏最多五项、401 退回登录、请求可取消；下载入口应放侧栏及手机“更多”，不挤入第六个底栏项目。 |

相关规范：`.trellis/spec/backend/runtime-contract.md`、`.trellis/spec/frontend/app-contract.md`、`.trellis/spec/guides/cross-layer-thinking-guide.md`、`docs/api.md`。

### 3. 选型对照与适配成本

| 方案 | 已有官方能力 | 本项目成本／结论 |
| --- | --- | --- |
| qBittorrent WebAPI + 可选 sidecar | HTTP API、文件优先级、种子做种、增量同步、速度与队列、官方容器 | **首选。** 新增一个可选服务，传输进程独立；外接现有实例时不接管其他种子或全局偏好。 |
| Transmission RPC | `torrent-add`、文件选择、session/速度和位置控制；RPC 会话 token；固定 4.0.6 文档可作稳定协议参考 | 可作后续适配器。需维护另一组认证、状态、选文件和位置映射，不在首期同时实现。不能混用旧版与主分支的新协议字段。 |
| aria2 JSON-RPC | BT／磁力与 HTTP 等多协议、`addTorrent`／`addUri`、选文件、元数据保存、RPC token | 若未来重点是通用下载器可考虑。磁力元数据与实际负载可能涉及后继任务，持久会话与文件发布需自行编排；本次没有 HTTP 下载需求，不作为首选。 |
| Go 内嵌 `anacrolix/torrent` | v1/v2 BitTorrent Go 库、可定制存储与 API | 可保持单业务进程，但应用必须承担网络、存储、恢复、配额、磁力阶段和做种生命周期；不符合本次优先复用成熟下载器的取舍。 |

### 4. qB API 核验：版本不能照抄旧 Wiki

官方 5.0 Wiki 页面仍存在历史 `pause/resume` 与 `paused` 描述。结合官方 `WebAPI_Changelog.md` 和固定 tag `release-5.0.5` 源码可确认：

- API URL 以 `/api/v2` 开头；WebAPI 主路径中的 `v2` **不等于** qB 应用主版本为 2。
- 使用 `POST /auth/login` 的 `username/password`，服务端保存 SID Cookie。`Origin`／`Referer` 必须与 qB 接受的源匹配；Go 客户端应显式设置正确值、保留校验，不关闭 qB 的 CSRF／Host／认证保护。
- 通过 `/app/version` 和 `/app/webapiVersion` 探测协议能力；只承诺经过验证的 5.x 范围。本文用 `release-5.0.5` 证明协议形态，**不是建议新部署安装这个历史补丁版，也未声称它是最新版本**。正式实施时再固定带维护补丁的镜像 tag/digest 并记录测试矩阵；未识别主版本只允许诊断，禁止猜测后继续写操作。
- qB 5.0 使用 `/torrents/start`、`/torrents/stop`，添加参数为 `stopped`，而非简单沿用 4.x 的 pause/resume/paused。适配器必须版本化，不能每个请求先试一个方法失败后再盲试另一个。
- `POST /torrents/add`：本地种子使用 multipart `torrents` 字段，磁力使用 `urls`。受控字段包括 `savepath`、`autoTMM=false`、独占分类、带任务 UUID 的 tag 和停止／元数据策略；不要允许浏览器透传任意 qB 参数。
- `POST /torrents/filePrio` 的 `hash`、`id`、`priority` 设置文件；`priority=0` 表示不下载。文件 ID 来自 `/torrents/files`，不能把 UI 数组序号当 ID。
- 源码中的 `/torrents/setLocation` 使用 `hashes`、`location`；`/torrents/delete` 使用 `hashes`、`deleteFiles`。首期移除下载管理记录应传 `deleteFiles=false`，不提供自动删除已入库文件。

认证细节、API 事实分别见参考 [1]–[4]；本文所有接口均为 qB 内部调用，栖库不原样对前端开放。

### 5. 添加、去重、磁力元数据和选文件

**建议流程（需要实施阶段技术探针验证）：**

1. 栖库先提交有幂等键的本地意图，再调用引擎。应用 UUID tag 作为提交超时后的关联线索；标签不代替数据库和路径核验。
2. `.torrent` 可停止状态添加；种子体积、条目数和路径须有限制。取得文件列表后持久化选择，只在有效选择落库后设置优先级并开始。
3. 磁力不能简单 `stopped=true` 后等待元数据：完全停止可能不进行元数据交换。固定源码支持 `stopCondition=MetadataReceived`，结合可交换元数据的初始状态取得列表；`NativeTorrentExtension` 在 downloading metadata 转换时暂停，`TorrentImpl` 根据 MetadataReceived 条件保持停止。**必须验证目标发行版在用户确认前不会开始有效负载下载；未通过时关闭磁力选文件流程，而不是默认下载全包。**
4. 5.2 changelog 另外加入 `/torrents/fetchMetadata` 等能力，是否采用应随实际版本能力检测；不能把它当作所有 5.x 都有。
5. 首期建议标准 v1／hybrid 种子为基础验收；若声称纯 v2 支持，必须增加对应样本。引擎 `hash` 作为不透明 ID，另存 infohash v1/v2 别名，不假设所有 hash 恒为 40 字符。不要靠磁力显示名或文件名去重。
6. 添加超时先按 UUID／infohash 对账；重复已有栖库任务返回已有任务。若引擎已有同资源但不属于栖库，不更改其目录、分类、速度或停止状态，提示用户在引擎端处理。

### 6. 发布目录、完成检测和选中清单

推荐两进程看到相同逻辑路径；允许外接实例作一个预先配置的前缀映射，但不接受任意客户端绝对路径。

```text
# 两者位于同一真实文件系统；不要仅靠相似路径名称判断同盘。
/downloads/staging/<task-uuid>/
/downloads/complete/<video|image|comic|novel|music>/<task-uuid>/
```

- qB 只对受控下载根有写权限；NAS 应用对整个下载根只读，对数据库与衍生缓存可写。已有 `/media` 继续只读；目标下载媒体库可指向 `complete/<type>`。
- 下载全程落在 staging；不要依赖 `. !qB` 文件扩展名作为唯一隔离机制。qB 未选择文件也可能留边界 piece／`.unwanted` 等磁盘痕迹，媒体扫描必须按选中文件清单开放，尤其漫画目录的页枚举和封面生成也必须过滤。
- 完成条件需同时满足：元数据就绪；至少一个有效选中文件；wanted 总大小与完成量／剩余量一致；每个选中文件完成；状态非 metadata、checking、allocating、moving、error 或 missingFiles；路径仍为本任务所拥有的目录。
- qB 报告下载完成后，由应用持久化 `publishing` 意图，再调用 `setLocation`。源码异步安排 `setSavePath`／`moveStorage`，API 返回成功只表示请求已接受。**同一时间不能修改选择、重命名或重复发起迁移。**
- 等待引擎真实保存位置指向本任务目标目录、状态不再 moving/checking、选中文件大小与可读性满足条件后，应用以短事务写入 ready。任何阶段退出都保持门禁关闭；重启按意图和实际引擎／文件状态恢复。
- 扫描器即使用户手动全盘扫，也必须排除 managed complete 中没有 ready 的子树；API、漫画页、缩略图读取应用相同授权。不能仅隐藏列表，因为旧缓存 ID 或手工拼接 API 仍可能绕过列表。
- 只有 ready 后安排目标库扫描并确认本次清单确实入库，才显示“已入库”。扫描失败单独显示“下载已完成，入库失败”，可重试入库而不重下。
- 同盘迁移有望近似重命名，但受 NAS 文件系统与引擎实现影响。libtorrent 明确支持跨盘搬移时复制文件；空间不足／文件冲突会导致 move 失败，需要显示原因并保留 staging。不能由 NAS 在只读挂载下自行复制，也不能入库时再做一次全文件哈希来拖慢超大文件发布。

### 7. 对账、做种与资源约束

- 用 `/sync/maindata?rid=...` 可以增量读取，RID 需按引擎会话管理；重启或 full update 后重新建立快照。首期任务量不大时也可分页有限查询。对外只管理栖库持有的 UUID/hash，不能把整个用户已有 qB 当作自己所有。
- 正常 UI 以秒级轮询或应用后端统一同步；隐藏页面降频，连续故障指数退避。不要每个浏览器都独立高频拉一次引擎，也不要把每秒速度变更永久写 SQLite。
- 引擎离线时保留最后状态并显示同步时间，不伪装为 0%／任务已消失。未找到任务、身份不一致、外部迁移路径都应进入待处理状态，不自动删除数据或重复提交。
- qB 配置/resume 数据与下载目录需要持久化；应用 SQLite 备份不包含引擎配置、做种状态或下载实体。恢复必须两者对账，不能宣传只恢复 SQLite 即恢复所有 BT 数据。
- `setDownloadLimit`、`setUploadLimit`、`setShareLimits` 可作受管任务策略。已有 qB 实例默认不改全局偏好；独立 sidecar 可使用保守队列并发。完成下载与是否做种是两个维度，继续做种不妨碍只读播放／入库。
- 文件字节、偏移、完成量使用 64 位整数；分片验证由引擎完成，栖库不把超大媒体传过 API/浏览器，不为进度读取整片。
- libtorrent 的磁盘缓存、mmap／操作系统页缓存会影响实际内存；不能用 qB 进程 RSS 小或一个参数来承诺整个系统固定内存。预分配、连接数、队列并发需按 NAS 实机测量；同盘改名与跨盘搬移分别测试。

### 8. 官方容器与首次配置

- 官方仓库提供 `qbittorrentofficial/qbittorrent-nox`，包含 amd64/arm64 构建说明；部署阶段按所选 tag 核验架构清单并固定 digest，本文没有拉取镜像验证。
- README 的配置入口包括 `QBT_VERSION`、`QBT_WEBUI_PORT`、`QBT_TORRENTING_PORT`、`PUID`、`PGID` 与法律告知变量等，挂载 `/config` 和下载目录。权限需与 NAS 读身份匹配，不能默认 chmod 777。
- 首次临时密码出现在容器日志；应经受限临时 WebUI 设置固定密码，再向栖库后端提供凭据。若未来要自动初始化密码，必须单独验证受支持的配置生成／写入流程；不存在一个经本文确认的通用 `QBT_PASSWORD` 环境变量。
- WebUI/RPC 不应默认直接暴露公网；P2P TCP/UDP 端口和 WebUI 是不同边界，入站端口可用性影响连接条件但不能承诺固定下载速度。
- 外接现有 qB 时保留其会话、种子、设置和做种策略。栖库只对新建受管任务写入分类／tag／目录／每任务策略，不接管历史资源。

## External References

全部访问于 2026-11-17。以下均为上游官方或项目主仓库；固定版本源码优先用于解决 Wiki 残留旧描述的歧义。

1. qBittorrent 5.0 WebUI API Wiki：<https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-%28qBittorrent-5.0%29>。认证、同步、任务字段与接口总览；部分旧字段需与源码交叉核验。
2. qBittorrent WebAPI changelog：<https://github.com/qbittorrent/qBittorrent/blob/master/WebAPI_Changelog.md>。5.0 start/stop 等变更、后续 fetchMetadata 能力；主分支可能继续演进，实施需固定版本。
3. qBittorrent `release-5.0.5` API controller：<https://github.com/qbittorrent/qBittorrent/blob/release-5.0.5/src/webui/api/torrentscontroller.cpp>。`addAction`、`filePrioAction`、`setLocationAction`、start/stop 与 delete 参数。
4. qBittorrent `release-5.0.5` TorrentImpl：<https://github.com/qbittorrent/qBittorrent/blob/release-5.0.5/src/base/bittorrent/torrentimpl.cpp>。`setSavePath`、moveStorage 与 MetadataReceived 停止条件。
5. qBittorrent `release-5.0.5` native extension：<https://github.com/qbittorrent/qBittorrent/blob/release-5.0.5/src/base/bittorrent/nativetorrentextension.cpp>。元数据状态切换时暂停的实现。
6. qBittorrent `release-5.0.5` JSON serializer：<https://github.com/qbittorrent/qBittorrent/blob/release-5.0.5/src/webui/api/serialize/serialize_torrent.cpp>。hash、infohash、进度、完成量与保存位置字段。
7. 官方容器仓库：<https://github.com/qbittorrent/docker-qbittorrent-nox>。官方镜像、卷、UID/GID、端口、临时密码和多架构部署说明。
8. Transmission 4.0.6 RPC spec：<https://github.com/transmission/transmission/blob/4.0.6/docs/rpc-spec.md>。固定协议参考；非对当前最新版的声称。
9. aria2 官方手册：<https://aria2.github.io/manual/en/html/aria2c.html>。页面标识 1.37.0，支持 BT／JSON-RPC／metadata／选文件等；本次未安装或实测。
10. anacrolix/torrent 主仓库：<https://github.com/anacrolix/torrent>。Go BT 库能力与嵌入方式。
11. libtorrent torrent handle：<https://libtorrent.org/reference-Torrent_Handle.html>。move_storage 跨文件系统时复制及失败行为。
12. libtorrent settings：<https://libtorrent.org/reference-Settings.html>。缓存、磁盘读写模式及与系统页缓存的关系。

## Caveats / Not Found

- 未运行 qB／Transmission／aria2，未下载测试种子，未运行任何 Docker 命令。本文为有源码依据的方案研究，不是互操作测试报告。
- 尚未验证实际 NAS 的文件系统、同盘 rename、真实大影片起播/持续吞吐、引擎版本与资源限制；不得将理论方案当成性能承诺。
- 5.x Wiki 存在字段陈旧，磁力“选文件前无有效负载”的行为、纯 v2 identity、sidecar 第一次密码初始化和迁移失败恢复必须列为实现前技术探针。
- 引擎能从下载根内写普通文件，不等于不可信种子可自由选择系统路径；上传限制、路径清单、服务端目录模板、禁用任意 shell 完成钩子与最小挂载都仍属应用职责。
- 本文未建议资源搜索、RSS、PT 自动化或边下边播；是否继续做种与每任务限速可在 PRD 中作为可见策略收敛，不能暗改用户现有实例的全局偏好。
