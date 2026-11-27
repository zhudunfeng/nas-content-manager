# Research: 资源组件部署的现有架构与最小复用方案

- Query: 当前平台如何触发 MoviePilot、AutoBangumi、Sonarr、Radarr 的部署，并复用已有托管引擎助手，兼容 Linux NAS、Windows Docker 完整模式及旧发布布局？
- Scope: internal；本报告只分析仓库代码和已有契约，不替代上游镜像、配置和认证协议的官方核实。
- Date: 2026-11-20

## Findings

### 推荐结论

复用现有 `nas-engine-helper` 的 Linux Docker 控制面、独立持久任务库、单执行器和私有 Unix HTTP 协议，为四个固定组件增加受信任模板和可选部署能力。组件候选列表、来源绑定和简单使用流程留在 app 层。不要另建通用 Docker 代理、第二套部署任务系统或 Windows Python 安装器。

首期可真正支持的部署边界是 Linux NAS 的 helper 模式，以及 Windows Docker 完整模式内的 Linux helper。Windows 原生模式在未配置助手时应明确显示“需使用 Docker 完整模式或连接已安装服务”，保留现有外部服务接入能力。不能把 Windows 原生安装声明为已支持。

旧版部署必须继续工作。新增组件部署能力应由受信任部署配置及桌面发布清单显式声明；未声明时保留原有六角色 helper 布局，不要求新增卷，也不把缺少新增挂载判定为旧 qB/Jackett 故障。与父任务进一步收敛后，推荐混合存储方案：helper 只新增一个显式的共享完成媒体角色，四产品配置/cache/data 使用不挂入 helper 的固定受管动态卷。下文区分该新增设计与目前已有能力。

### Files found

| 文件 | 当前职责及对本任务的意义 |
| --- | --- |
| `internal/enginehelper/service.go` | Linux 私有服务、租约、状态探测、固定目标请求校验和单执行器提交。 |
| `internal/enginehelper/tasks.go` | 持久阶段、实际拉取进度、丢失创建响应核对、恢复和启动流程。 |
| `internal/enginehelper/store.go` | 独立 SQLite、安装身份、任务、请求签名绑定和事件。 |
| `internal/enginehelper/docker.go` | 固定本机 Unix Docker API、Linux/架构/网络/安全模式预检。 |
| `internal/enginehelper/templates.go` | 两个固定镜像模板、确定性容器名、完整实际容器归属核验。 |
| `internal/enginehelper/config.go` | 仅受信任环境提供的根、存储模式、网络、UID/GID和端口配置。 |
| `internal/enginehelper/files.go` | 防软链接、目录认领、私密文件原子发布及 qB/Jackett 凭据准备。 |
| `internal/enginehelper/volumes.go` | 精确六角色命名卷、元数据与持久安装标记核验。 |
| `internal/enginehelper/health.go` | 当前仅 qB/Jackett 的认证健康检查。 |
| `internal/enginehelper/stop.go` | 私有完整停止任务、精确原容器停止和 CLI 等待。 |
| `internal/enginecontract/models.go` | 安全公开 DTO，以及不能公开的 qB/Jackett 专用连接清单。 |
| `internal/app/engine_api.go` | app 至固定 Unix helper 的有界代理、公开管理端点与 managed 模式校验。 |
| `internal/app/engine_runtime.go` | qB/Jackett 私密连接原子清单读取、旧良好快照保留和下载实例绑定。 |
| `internal/app/collection_external_store.go` | 外部 API 私密快照、来源事务及连接引用，可用于安装后显式接入。 |
| `internal/app/collection_external_api.go` | 现有四类外部服务的检测、列表与持久业务动作端点。 |
| `internal/app/collection_models.go` | 外部来源安全字段和仅写连接输入。 |
| `internal/externalapi/` | 实际生产协议适配器，应作为安装后 API 可用性的最终验证。 |
| `compose.engines.yaml` | Linux NAS bind 模式：helper 持有 daemon 权限，app 只读完成区/连接。 |
| `compose.desktop.yaml` | Windows Docker 完整模式：Linux helper 与精确角色卷。 |
| `compose.desktop.collection.yaml` | 现有可选收集 worker 扩展，可参考显式发布能力而非隐式增加角色的模式。 |
| `internal/desktop/docker.go` | 严格 v1/v2 镜像清单、Compose 组合、角色集、真实容器核验和完整停止。 |
| `internal/desktop/docker_compose.go` | 规范化后 Compose JSON 的精确服务/环境/挂载校验。 |
| `web/src/components/CollectionSourceDialog.vue` | 现有外部服务选择和仅写 API 凭据入口。 |
| `web/src/components/CollectionCenter.vue` | 收集中心及外部业务面板持有者。 |
| `web/src/components/EngineSetupDialog.vue` | 可参考的单原生弹窗、恢复任务和真实阶段进度模式。 |

### 可直接复用的代码模式

1. **部署权限边界已经成立。** `internal/enginehelper/service.go:146` 明确只支持 Linux；`internal/enginehelper/docker.go:60` 只拨固定 Unix socket，`:115` 检查 Linux Docker API、amd64/arm64、已有 bridge 并拒绝不支持的安全模式。app 不需要 Docker socket，也不能转发浏览器指定的镜像、宿主路径、命令或原始 Docker JSON。
2. **先租约后恢复。** `internal/enginehelper/service.go:48` 在打开状态库、读取安装身份和恢复执行器之前取得安装级租约。新增四组件继续共享该租约与任务库，不按产品创建并发执行器。
3. **先持久化意图再产生效果。** `internal/enginehelper/tasks.go:34` 保存阶段和事件；`internal/enginehelper/service.go:355` 根据 requestId 和规范签名恢复原任务，同目标活动任务可复用，不同目标保持忙碌约束。
4. **真实、可恢复的部署管线。** `internal/enginehelper/tasks.go:114` 已串联预检、目录归属、固定镜像验证、拉取、配置、创建、启动、健康与连接发布；`:269` 在创建响应不确定时检查确定性原容器，避免重复创建。新增模板应接入同一流程，保留已成功步骤和凭据。
5. **模板和实际资源双重核对。** `internal/enginehelper/templates.go:37` 从安装身份派生容器名；`:86` 定义最小固定 spec；`:134` 检查实际标签、精确镜像内容、挂载、网络、端口与权限。不是只校验请求参数或 Compose 源文件。
6. **只读探测不能部署。** `internal/enginehelper/service.go:288` 当前状态循环只有 qB/Jackett；新组件需要显式允许集和只读状态，Docker 不可达必须是 unknown/unavailable，不能冒充 missing 并启用安装按钮。
7. **目录和卷身份必须保留。** `internal/enginehelper/files.go:139` 仅认领空配置目录并写归属标记；`internal/enginehelper/volumes.go:126` 核对普通私密标记文件、角色、部署和随机安装身份，拒绝未知非空数据与软链接。

### 必须拆除的两个组件专用假设

- `internal/enginecontract/models.go:11` 只定义 qB/Jackett 标识；`:21` 的状态注释及 `:41` 的请求语义也限于两个组件。
- `internal/enginehelper/templates.go:24` 只有两套固定镜像。`:86` 的生成逻辑中 qB 分支后的 else 默认是 Jackett，不能直接让 MoviePilot 等落入该分支。
- `internal/enginehelper/health.go:23` 同样是 qB 与 Jackett 二分，新增产品必须有明确认证/就绪实现。
- `internal/enginehelper/service.go:394` 的初始化、`:504` 的启动和当前规范化组件集合需要分别纳入显式允许范围；qB 告知接受条件只在确实安装/启动 qB 时要求，不能误用于所有四产品。
- `internal/enginehelper/files.go:285` 和 `internal/enginecontract/models.go:103` 的私密清单把 qB/Jackett generation 写死。不能将四类服务 API key 填入 Jackett 字段，也不能修改 v1 清单后让旧读取器静默解释。
- `internal/app/engine_api.go:196` 目前只允许 1–2 个组件且依赖 download/search managed 模式。四个资源组件需要独立部署能力门控，不能因下载/搜索是 external 而完全隐藏已配置 helper 的四组件能力。
- `internal/app/engine_runtime.go:43` 的后台连接刷新只在 download/search managed 时启动；四类外部来源接入不能依赖这条 qB/Jackett 专用循环。

### bind 模式的最小兼容设计

`compose.engines.yaml:48` 已将整个受控宿主根映射到 helper 的 `/managed`。因此新组件的固定私有目录可位于同根下的 `moviepilot`、`autobangumi`、`sonarr`、`radarr` 子树，无需让浏览器提交路径，也不需要增加通用宿主挂载能力。

实现应增加受信任的“资源组件部署能力”配置，默认关闭或由新版固定部署 overlay 开启；关闭时保持旧行为。只在用户授权的部署任务内创建/认领相应空子目录。每个产品所需配置子目录和容器目标从固定模板得到；保留逐级目录、软链接、权限和安装标记检查。状态请求不能提前认领目录。

四个产品需要的可写下载/整理目录应单独列入固定模板。现有 `downloads` 角色只能按已有下载发布契约使用；原始 `/media` 仍是只读授权媒体。若计划给组件一个默认整理输出库，必须明确提供独立受管输出目录及 app 只读映射，不能为了安装方便将用户原始媒体根改成可写。仅连接 API 的来源本身可以不配置本地完成目录。

### volume 模式的向后兼容设计

`internal/enginehelper/volumes.go:35` 的角色集固定为 `state,qbittorrent,jackett,downloads,connections,ipc`，`:83` 检查 helper 的所有固定角色挂载与元数据。`internal/desktop/docker.go:178` 按发布清单返回精确角色集；`:23`、`:87` 严格区分 v1 和 v2，v2 是现有 collection worker 能力。`internal/desktop/docker_compose.go:152`、`:175`、`:198` 同时校验精确卷数、环境和实际挂载。

建议让 helper 角色集成为受信任能力配置的纯函数，而非无条件扩大。收敛后的最小方案不是把四产品的所有配置目录都挂给 helper，而是仅增加一个共享媒体角色：

| 发布/部署能力 | helper 必需角色 | 行为 |
| --- | --- | --- |
| 旧 v1、v2 / 未启用资源部署 | 原有六角色 | 旧镜像清单、Compose 和 qB/Jackett 安装继续成立。 |
| 新版显式资源部署能力 | 原六角色 + 一个 `collection-media` 共享输出角色 | 仅接受新版固定 Compose、匹配标签及准确挂载集合；四产品私密卷独立管理，不属于 helper 挂载角色。 |

具体步骤：

1. 新增版本化桌面发布能力，例如严格 v3 或同等明确的新版 schema；选择新版固定 overlay，精确声明共享 `collection-media` 卷和能力环境，例如默认 false 的 `ENGINE_COLLECTION_ENABLED`。现有 v2 字段仍维持其已定义的收集 worker 语义，不通过“helper 镜像较新”猜测能力。
2. helper、desktop `volumeRoles`、最终 Compose 校验、实际容器挂载核验和发布脚本使用同一明确能力判定。旧版本不得隐式认领新角色；新版本缺少必要卷应给出更新完整包/部署配置的可操作错误，不能回退成 bind 或 Docker 内部 Mountpoint。
3. 共享媒体新角色复用 local driver、无 driver options、部署/角色标签和 `.nas-engine-volume` 安装绑定。由 Compose 预建并挂给 helper，app 只读挂载 `/collection-media`，`NAS_MEDIA_ROOTS` 仅授权 `/collection-media/library`。Linux bind 对应受控 `HostRoot/collection-media`。四产品按固定模板挂到 `/data`，不能允许浏览器扩充卷或目标。
4. 不使用卷子路径规避独立角色。`internal/enginehelper/volumes.go:16` 已明确坚持 Docker API1.41 原生命名卷字段；复用整个 state 卷给产品容器会暴露私有 SQLite/身份，不能作为省卷方案。
5. qB/Jackett 模板、私密协议及原卷内容保持原有身份和语义。部署包升级可以添加新卷，但不是用户数据自动迁移；出现旧容器实际挂载与新模板不一致时，不能直接接管或删除重建。

### 收敛方案：固定动态私密卷与共享路径的明显问题

父任务提出四产品 config/cache/data 使用固定、带安装归属标签的动态 local 卷，helper 不读取产品凭据。该方案可以避免把所有私密卷预先挂给 helper，也不需要 Docker 卷子路径，适合作为首期的最小存储扩展，但以下是新增实现责任：

1. **当前没有动态卷创建能力。** `internal/enginehelper/volumes.go:65` 的 `volumeIdentity` 明确只验证 Compose 预建卷，不创建资源。新增接口只能由固定产品模板派生确定名称、配置/cache/data角色和目标；不提供通用创建卷 API。
2. **Docker 创建卷不证明刚刚新建。** 最终采用 state 中私密持久授权 nonce 文件：先 inspect 确认卷不存在，再原子持久化稳定随机 nonce，然后 create；卷标签绑定 installation/component/role/nonce。首次遇到未知同名卷要拒绝；请求响应丢失或重启后，只有已有 nonce 与精确标签、local driver、空 options 全部匹配，才允许恢复复用。若 inspect 与 create 之间发生同名竞争，创建后实际 metadata 的 nonce 不符必须拒绝。仅检查名字或收到 create HTTP 成功不够。
3. **私密卷不挂 helper，现有文件标记逻辑不能直接复用。** 不应读取 Docker 内部 Mountpoint，也不应挂整个 state 卷给产品。需要为产品卷新增“state 私密持久授权 nonce + 精确 daemon 元数据 + 每次实际容器挂载”的归属契约；状态仅检查，不认领、不创建。nonce 文件保持 root 私密、普通文件、严格格式、有界读取和原子替换，nonce 不进入公开任务或日志。该契约需由实现阶段更新 spec，不能声称等同现有 `.nas-engine-volume` 文件证明。
4. **helper 不读凭据可简化引导，但不等于自动接通 API。** 产品 entrypoint 自己生成配置和认证；安装显示运行/待配置，用户经产品管理入口完成初始设置，再在 app 仅写凭据字段中接入。app 使用生产 Probe 验证。若要求完全自动接入，则还需官方可验证的固定离线初始化/认证流程，不能绕到容器日志、任意 exec 或 archive 读写。
5. **现有 qB 与新 `/data/downloads` 并不共享数据。** qB 固定模板只挂旧 `downloads` 角色到 `/downloads`（`internal/enginehelper/templates.go:86`）。只给四产品挂 `collection-media:/data` 不会让 qB 写入 `/data/downloads`。若界面默认声称路径已经配通，将导致下载后找不到文件。
6. **最小兼容路径建议：** 四产品同时获得原有 downloads 角色到 `/downloads`，整理输出为 `/data/library`；初始下载器路径沿用当前 qB 的 `/downloads`，使用现有 MoviePilot 固定 copy 整理语义。两卷之间不能承诺 hardlink。共享 `/data/downloads` 可以保留为未来由同根下载器使用的目录，但不能默认说当前 qB 已使用它。
7. **如果必须统一 `/data/downloads`：** 需明确升级 qB 模板和挂载、旧 qB 实际容器的兼容处理，以及下载任务实例/路径绑定。这不是给新组件加一个卷就能完成的改动，不能静默重新创建现有 qB。保持简单体验应优先选择上一项的可用路径，或如实保留下载器待配置。
8. **输出目录所有权要一致。** 共享库可以让产品执行整理写入，但 `/data/downloads` 不能作为 app 媒体根。`/data/library` 只读呈现给 app 不会自动完成其媒体登记/扫描；依然使用既有独立完成目录导入契约。四产品 config/cache/data 具体卷数量和容器目标仍须按官方模板，而非一律假设三个卷。

父任务已确认采用两数据挂载：四组件旧 downloads 卷/目录到 `/downloads` RW，独立 collection-media 到 `/data` RW；app 只读挂载共享媒体并仅授权 `/collection-media/library`。明确跨卷复制，不宣传硬链接，不自动配置 qB 账号，不接管已有下载任务。产品凭据由用户完成上游初始化后通过 app 仅写字段提交，helper 不读取或代管四产品 API 密钥。

### 生命周期和完整停止不能遗漏

`internal/enginehelper/stop.go:18` 的 stop-all 当前只提交 qB/Jackett；`:49` 在效果前确认原容器 ID、固定镜像和完整模板，未拉取/创建/删除任何资源。`internal/desktop/docker.go:798` 先运行该私有命令，成功后才停止 Compose。

四个动态容器必须加入同一安装完整停止集合，且安装/启动仍受既有 quiescing gate 阻止。即使当前发布能力关闭，若持久记录表明本安装曾部署的固定组件仍存在，也不能把它遗漏并宣称全部停止；能力降级应明确拒绝不完整停止或继续核对持久受管集合。

现有 CLI 总预算是 90 秒（`internal/enginehelper/stop.go:105`），desktop 等待 105 秒（`internal/desktop/docker.go:815`）。单组件可用 20 秒 Docker 停止宽限、30 秒调用预算（`stop.go:83`）。扩展到最多六个顺序停止后预算必须统一调整并让外层大于内层；不能只扩大数组而留下原超时。继续保留“任何停止不确定/归属冲突即不进行后续 Compose stop”的行为。

### 凭据、健康和自动连接边界

安装成功、进程运行、API 已认证及业务配置就绪是不同事实。未完成上游账户、站点、下载器或媒体服务配置时应显示 `configuration_required`，保留“打开组件完成设置”和“检测并连接”操作，不能仅凭 HTTP200 宣称六阶段业务可用。

- 现有 `internal/enginecontract/models.go:112` 的私密连接不得直接公开。最终方案不让 helper 管理四产品凭据，因而无需扩充该旧连接清单；app 继续通过现有 collection 外部私密快照保存仅写凭据，保持 qB/Jackett v1 协议兼容。未来若自动引导凭据再单独设计明确版本化协议。
- `internal/app/engine_runtime.go:70` 有有界私密 JSON 读取；`:138` 保留旧良好运行快照并阻止身份/版本倒退，可作为模式参考。
- `internal/app/collection_external_store.go:55`、`:68`、`:131` 已有独立私密连接快照与生产适配器；`:261` 在来源事务内保存私密引用。安装后接入应复用它，而不是在 helper 向主应用 SQLite 写来源。
- app 根据已核验安装身份、固定产品与服务端派生地址创建或复用来源；绑定关系应持久化且幂等。重试连接不得反复生成相同来源，也不能静默覆盖用户已配置的外部来源。
- 使用实际 `internal/externalapi` 生产 `Probe` 确认支持版本和 API 身份后才完成 API 绑定。`.trellis/spec/backend/external-collection-contract.md:35` 的 MoviePilot 六阶段只验证精确 3.2.8，不能安装任意最新版后继续承诺相同协议。
- `.trellis/spec/backend/external-collection-contract.md:36` 规定 AutoBangumi 使用 API-scope Bearer token；初始网页登录密码不是 API token。无法安全取得 API token 时应停在待配置状态。
- 服务端内部地址与浏览器能打开的管理入口是两种用途。管理端口如需发布，应由部署模板固定限制与部署配置决定；不能把 Docker 内部 DNS 直接当成用户可访问入口。NAS 远程访问和桌面本机入口需要分别验证。
- API key、密码、私密 BaseURL、上游原始响应和日志不能出现在公开任务、中文事件、主库普通备份或常规日志中。需要展示凭据的上游初始设置不得通过读容器日志实现。

### 简单易用的 app/UI 分工

收集中心展示四个固定组件卡片或选项，每项仅呈现明确状态和一个主要操作：未部署→“安装”、停止→“启动”、待配置→“完成设置”、已就绪→“使用/连接”。检测能力来自服务端只读快照；未安装 Docker/helper 时给出实际环境说明。

沿用现有单原生弹窗与稳定请求键模式，展示真实阶段/层进度、可恢复任务和失败重试；不要要求用户填写镜像、命令、Docker 网络或宿主路径。完成部署后在同一流程中检测并连接已有 external 来源适配器。MoviePilot 现有搜索、订阅、下载、整理、刮削、媒体库刷新入口继续使用已有业务面板，安装流程无需复制六套业务控制。

`web/src/components/CollectionSourceDialog.vue:30` 是现有产品映射，`:104` 是服务选择，`:108` 是仅写凭据输入。`web/src/components/CollectionCenter.vue:309` 持有外部面板，可在这一 app/UI 边界加入“安装组件”入口并完成绑定。

### 方案比较

| 方案 | 优点 | 代价/约束 | 判断 |
| --- | --- | --- | --- |
| 扩展现有 Linux helper 的可选固定目录 | 复用全部权限、任务、镜像、租约和恢复机制；覆盖 Linux NAS 与 Windows Docker 完整模式 | 必须明确旧/新角色集与私密协议；扩大完整停止覆盖 | 推荐的最小完整路径。 |
| 独立组件 helper，提取共用 Docker/任务原语 | 可隔离发布节奏与组件状态 | 新镜像/socket/SQLite/Compose/发布清单和停止协调；直接复制会产生两套安全实现 | 当前需求没有足够收益，不建议首期采用。 |
| 原生 app/桌面本地 Docker 适配器或平台安装器 | 理论上可给 Windows 原生用户安装入口 | 新增 daemon 权限、Windows ACL/锁/凭据/上下文清理与生命周期；四产品 OS 安装/更新并无基础设施 | 超出最小方案；不能将 socket 交给公共 app 进程凑合实现。 |

### 最小实现影响范围

1. `internal/enginecontract/models.go`：固定组件标识、安全能力/状态 DTO；保持旧连接协议明确版本。
2. `internal/enginehelper/config.go`、`templates.go`、`service.go`、`tasks.go`、`health.go`、`files.go`、`volumes.go`：显式资源能力、固定模板、产品运行探测、可选共享媒体角色与动态私密卷 nonce；共用原任务库，四产品认证由 app 既有来源适配器完成。
3. `internal/enginehelper/stop.go`：完整受管组件集合、预算、归属核验与降级处理。
4. app engine 代理及新的固定组件 app 层处理：独立 capability 门控、只读目录、稳定任务代理、服务端派生的连接和来源幂等绑定；复用 `collection_external_store.go` 的秘密管理。
5. `compose.engines.yaml` 的明确可选资源部署能力、桌面固定 overlay、新版发布清单及 `internal/desktop/docker.go` / `docker_compose.go`：配置、卷和实际资源检查同步变化。
6. `web/src` 的现有收集来源/管理入口：单弹窗安装、真实任务恢复、配置提示和来源绑定；不新增重复的资源业务面板。
7. `scripts/release.py` 及其相关清单校验/打包流程、环境示例与 `docs/api.md`：仅按实际新增发布格式修改；不能只改源码而遗漏完整包所需 overlay。

### 必需验证

- 旧清单 v1/v2 + 原六角色 helper 的启动/探测/qB/Jackett 初始化回归；新版关闭扩展能力也仍接受旧布局。
- 新版能力与模板精确一致；缺少/多余/重复/嵌套挂载、外部卷、driver options、错误标签、未知非空内容及软链接全部拒绝。状态探测无目录认领或创建效果。
- 四固定产品的完整实际 spec 核验：不可变镜像、所需架构、网络、允许端口、独立配置和限定输出挂载、运行身份、环境及健康认证。
- 相同请求重复提交、不同 requestId 的同目标活动复用、不同目标忙碌、重启恢复、拉取错误/空流、丢失 create 响应后的原容器核对与失败后重试。
- 安装后来源绑定重复调用保持唯一；真实生产适配器版本/认证失败则待配置；凭据不进入公开 DTO/事件/日志/主库备份；现有外部来源不被替换。
- 六组件完整停止、已有停止幂等、外来同名容器、停止响应丢失、活动部署冲突、关闭能力但存在旧受管资源、内外超时预算和停止后启动门禁。
- 单原生弹窗、Esc/焦点、移动端、网络恢复、稳定 UUID 与 revision、任务切换旧响应隔离；四项部署状态与 MoviePilot 原六阶段入口可连贯使用。
- 项目要求的 Go vet/build、前端类型检查/生产构建、Linux amd64/arm64 编译及适当的 collection/download/search/media 回归。临时行为测试文件运行后只删除本次生成文件；空测试集不能当作行为覆盖。
- 实际 Docker 与上游产品部署验证使用独立命名资源并记录范围；仅协议 fixture 成功不能声称真实产品部署已成功。不要全局 prune 或改用户 Docker 配置。

### Related specs

- `.trellis/spec/backend/engine-bootstrap-contract.md`：Linux 私有 helper、固定模板、任务意图、精确角色、连接私密协议与完整停止。
- `.trellis/spec/backend/cross-platform-contract.md`：原生/Docker 平台边界、发布清单 v1/v2、规范化 Compose 和实际资源核验、桌面完整停止。
- `.trellis/spec/backend/external-collection-contract.md`：四产品现有 API、精确版本、秘密/来源事务、MoviePilot 六阶段和独立完成目录导入。
- `.trellis/spec/frontend/app-contract.md`：收集界面、单原生弹窗、稳定请求身份、任务轮询与跨主题响应式检查。

### External references and versions

本报告未进行上游 HTTP 调研。仓库已有证据限定为 `.trellis/spec/backend/external-collection-contract.md` 所列 MoviePilot 3.2.8 六阶段、AutoBangumi 4、Sonarr 4、Radarr 7；qB/Jackett 固定镜像见 `internal/enginehelper/templates.go:24`。这不是对其“最新版本”的断言。

四产品的官方镜像仓库、精确 tag/digest、多架构、运行 UID、挂载布局、初始认证、可离线初始化配置及所需外部依赖，应由同任务的上游部署研究记录确认后才能写入模板。不要使用 latest，也不要依据相似镜像猜测环境变量。

## Caveats / Not Found

- 此研究没有安装、启动或探测真实 Docker/上游组件；不能据此承诺真实 NAS/Windows 部署已验证。
- 当前 Windows 原生 helper、Windows 四产品安装器、动态受限 Docker broker 均不存在；只编译 Windows app 不会获得该能力。
- 默认整理输出库已收敛为显式 `collection-media` 角色，app 仅授权其 library 子树；实际产品的整理路径初始化仍以官方支持方式为准。当前 app 只读原始媒体原则不能改变。
- 不同官方镜像可能要求不同的运行身份或配置目标，不能机械套用 qB/Jackett 的 UID/GID 和 `/config`；需依据上游证据分别处理。
- 本次 CodeGraph 对 `helperRequest` 的定向查询没有结果，随后使用 `rg` 定位实现；未发现可直接复用的四组件部署实现。
- 桌面新 schema 的精确格式及是否组合现有 collection worker 要在实现设计中明确；不要使 v1/v2 读取器静默接受新字段或对旧清单要求新增卷。
