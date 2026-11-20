# Research: 飞牛 fnOS 应用包、市场接入与运行兼容边界

- Query: 当前系统如何新增插件系统、配置插件市场，并使用飞牛 OS 应用；区分包格式、市场发现、原生运行与容器迁移。
- Scope: mixed，主要为官方公开资料研究，结合现有引擎助手代码。
- Date: 2026-11-19（Asia/Shanghai）。
- Method: 首先尝试 Web 搜索/打开工具，但工具未返回可用来源；随后经 `python -B scripts/project_env.py -- python -B -` 使用标准库读取官方公开 HTML，检查 HTTP 200 并提取正文。未下载/运行 FPK、安装脚本或 CLI，也未请求私有应用市场接口。
- Task note: 调研开始时 `.trellis/.current-task` 仍指向旧桌面图标任务；依据本次主任务明确指定路径，仅写入本任务 research，不改变旧任务。

## Findings

### 1. 已证实：FPK 是依赖 fnOS 应用框架的应用交付单元

依据 [S1][S2][S3][S4][S5][S6][S7]：

- 官方称 fnpack 用于将项目打包成可安装的 `.fpk`，可在 Windows/Linux/macOS 开发机打包；这只证明打包工具跨平台，不证明应用包可在这些系统运行。
- 包包含 `manifest`、`config/privilege`、`config/resource`、`cmd/` 生命周期脚本、`wizard/` 向导、UI 与应用文件。不能把任意 FPK 当作普通 Docker 镜像、Vue 组件或 Go 动态库。
- `manifest` 是无扩展名的元数据文件；字段包括 appname/version/display_name/source、platform、os_min_version/os_max_version、install_dep_apps、service_port、desktop_uidir 等。
- `platform` 的 `x86` / `arm` / `all` 是飞牛设备架构声明。`all` 不代表跨操作系统；官方 Docker 案例明确要求镜像自身支持目标架构。
- 系统安装后管理 `/var/apps/{appname}`，并将 target/etc/var/tmp/home 分别关联到 `/vol{n}/@appcenter`、`@appconf`、`@appdata`、`@apptemp`、`@apphome`。程序还依赖 `TRIM_*` 环境上下文。
- `cmd/main` 处理 start/stop/status；另有 install/upgrade/uninstall/config 的 init/callback 脚本。**存在这些钩子，不等于有文档化的远程管理 API 或对应 CLI 命令。**
- `config/privilege` 可以选择 package/root 运行身份与附加组；资源声明可以要求系统链接、共享目录、Docker 项目等。共享目录由 fnOS 配置 Windows ACL 访问模型，不是简单 chmod 即等价。
- `install_dep_apps` 有自己的声明语法与处理规则，官方说明非递归解析。自有插件依赖模型不能直接假定与之等价。
- FPK 容器封装的确切归档/压缩层次、校验/签名结构，在本轮查阅的官方页面没有明确格式契约；不能仅凭扩展名在计划中写死“就是 tar.gz”。若需要自行解析，必须先对许可允许的实际样包验证格式、大小上限与版本兼容。

### 2. 已证实：部分 FPK 使用 Docker Compose，但转换仍需限定子集

依据 [S4][S8]：

- `config/resource` 的 `docker-project.projects[]` 声明 Compose 项目，path 相对于应用 app 目录；官方示例位于 `app/docker/docker-compose.yaml`。
- 官方 Docker 示例将 `${TRIM_APPDEST}`、`${TRIM_PKGVAR}`、`${TRIM_SERVICE_PORT}` 等变量用于挂载与端口。它们需要被映射到本系统可控路径和部署能力。
- Docker 应用的主要启停可由 fnOS 处理资源声明，`cmd/main status` 仍可执行额外状态逻辑。应用还可能包含生命周期脚本、本地进程、网关 Unix Socket、用户授权与系统 API 依赖。
- 官方提供端口入口和统一网关入口两种示例；本系统不能假定读取 manifest 的 service_port 就一定能打开应用。
- **工程推断**：只有经过审核、无需执行原包脚本、不依赖 fnOS API/ACL/系统链接，且其 Compose/资源可以完整映射的应用，才适合转换成自有容器插件。优先让维护者提供经审核的自有声明式描述，避免运行时自动改写任意 Compose。
- 转换应采用允许列表：镜像摘要、受控端口、应用私有卷、显式授权的只读目录、限定环境变量、健康检查和配额。拒绝 privileged、宿主根目录/任意设备/Docker socket 挂载、host PID/IPC、不受控网络模式和未知 Compose 特性；不能简单将远程 YAML 交给高权限 Docker 执行。
- `platform=all` 不能省略镜像 CPU 架构检查；Windows 侧只有在可用的 Linux 容器运行环境下，才有条件运行这一容器子集，不是 Windows 原生 FPK 兼容。

### 3. 已证实：appcenter-cli 是 fnOS 设备工具；功能必须按文档和实机能力拆分

依据 [S9][S10]，官方页明确描述下列命令：

| 能力 | 已文档化形式 | 本系统计划中的处理 |
| --- | --- | --- |
| 安装本地 FPK | `appcenter-cli install-fpk myapp.fpk` | 仅在真实 fnOS 节点、经 capability 探测、明确安装授权后开放 |
| 传入向导配置 | `install-fpk ... --env config.env` | 服务端生成权限受限临时文件，退出后清理，敏感值不进 argv/日志/浏览器返回 |
| 安装开发目录 | `appcenter-cli install-local` | 开发测试功能，首版不暴露给插件市场 |
| 安装空间 | `appcenter-cli default-volume [1]` | 不应为某插件静默改变全局默认空间；具体单次位置能力另验 |
| 列举应用 | `appcenter-cli list` | 需实机确认输出格式/本地化/退出码/状态准确性，不能假定 JSON |
| 启动/停止 | `appcenter-cli start myapp` / `stop myapp` | 使用固定命令与独立 argv，严格应用标识，限制可管理应用归属 |
| 升级/卸载 | 本轮官方 CLI 页面未列 | **P0 验证门槛**：实机检查官方帮助及支持版本；没有稳定能力则跳转/提示用户在 fnOS 应用中心手动操作 |
| 任务/进度/日志 | 本轮官方 CLI 页面未列标准接口 | **P0 验证门槛**：仅报告可确认阶段与受控日志，不能编造整体百分比或假定异步作业 API |

注意：未列出的功能只能表述为“本次文档未证实”，不能断言系统一定没有。安装测试需要管理员能力，但具体每条命令的最小权限、sudo/服务运行身份、交互要求必须在目标版本实测。

### 4. 官方开放 API 与可配置市场协议是两件事

依据 [S11][S12]：

- 开放 API 概述当前说明文件/目录授权、后端权限查询、路径转换、页面路由、语言/主题/系统版本等能力。
- 这不能作为“官方已经公开应用市场搜索、下载、安装、升级、卸载 API”的证据。
- 官方发布流程说明通过平台流程/开发者交流渠道提交应用和材料；它不是面向第三方客户端的可配置仓库格式说明。
- **本次查阅的官方文档中未发现**可让任意第三方客户端仅配置 URL 即兼容官方应用市场的稳定索引/鉴权/包分发协议。该结论只描述已核实范围，**不证明不存在其他官方合作接口或尚未覆盖的文档**。
- 官方市场、社区 FPK 汇总仓库、第三方商店程序，应建模为不同 `sourceType`，分别适配。相同下载后缀不代表相同索引格式、授权范围或更新规则。
- 本轮未穷尽社区市场，不将任何未核实三方地址预置为官方市场；待用户指定实际市场 URL 后，再验证发布者文档、格式、鉴权、缓存策略、分发许可与接口稳定性。

### 5. 三种可行模式及兼容矩阵

| 模式 | 真实 fnOS 主机 | 普通 Linux | Windows | 范围与代价 |
| --- | --- | --- | --- | --- |
| A：自有声明式插件 + 可配置标准市场 | 可支持 | 有相应运行时可支持 | 外部连接可支持；容器插件需 Linux 容器环境 | 推荐产品基础，平台规则与生命周期可控；不声称兼容任意 FPK |
| B：fnOS 原生节点代理 + 原生应用管理 | 条件支持，需宿主代理及管理员/授权能力 | 不支持原生 FPK | 不支持原生 FPK；可作为远程管理前端 | 对系统型/Native FPK 最合理，fnOS 自己处理权限、依赖、向导与目录 |
| C：审核后转换 Docker 型 FPK 为自有容器描述 | 条件支持 | 条件支持 | 条件支持，需 Linux 容器运行时 | 只支持可完整映射子集，维护成本逐应用评估；禁止任意脚本模拟 |

模式 B 的部署建议：NAS Web 主服务继续低权限运行；fnOS 宿主侧使用独立受限代理，暴露明确动作与能力列表。即使 Web 应用本身跑在 fnOS 的容器里，也不能假定容器中存在宿主 appcenter-cli、用户数据库、系统资源服务或目录语义。不要为运行 CLI 而把整个宿主根目录挂进业务容器，也不要开放通用 shell/SSH 命令入口。若未来支持远程节点，需单独的身份绑定、传输鉴权与审计设计，不作为首版隐含能力。

### 6. 对现有仓库的映射

Files found / code patterns：

- `internal/enginehelper/templates.go:14-30`：现有模板只支持固定 qBittorrent/Jackett 镜像；它是受控模型，不是可直接接收任意市场配置的插件运行时。
- `internal/enginehelper/templates.go:72-110`：从受信任部署参数生成容器规格，固定网络/挂载/日志和隔离属性，可借鉴为插件策略校验机制。
- `internal/enginehelper/templates.go:113-145`：复用前核验实际身份、镜像和挂载，不凭同名容器接管；插件系统应保留此原则。
- `internal/enginehelper/service.go:32-44`：独立助手状态库、有界单执行器、生命周期和跨进程锁，可以作为持久插件操作设计参考。
- `internal/enginehelper/service.go:47-88`：状态恢复前获得安装锁；新插件管理仍需持久操作和崩溃恢复约束。
- `internal/enginecontract/models.go:17-39`：浏览器 DTO 返回脱敏状态与版本，适合作为公开状态和私有连接数据分离的参考。
- `internal/enginecontract/models.go:41-52`：requestId 与 revision 可用作幂等提交/乐观并发模式参考，不应粗暴放宽固定引擎协议为任意命令。

Related specs：

- `.trellis/spec/backend/engine-bootstrap-contract.md`：固定 helper 协议、低权限业务面与高权限控制面的边界、任务持久化和 Docker 归属。
- `.trellis/spec/backend/runtime-contract.md`：会话鉴权、Origin、SQLite 状态、路径与退出生命周期。
- `.trellis/spec/backend/cross-platform-contract.md`：Windows/Linux 能力差异、部署所有权和退出过程。

### 7. 建议分期与进入下一期的门槛

1. **P0：真实兼容性样板**。选择目标 fnOS 版本和 CPU、至少一个简单 Docker FPK 与一个 Native FPK、用户希望接入的实际市场地址。仅解析元数据与只读探测；确认 appcenter-cli 功能/权限/输出格式；明确升级、卸载、操作取消与失败恢复方案。没有实机证据前，把这些能力标为 experimental/unsupported，并保留系统应用中心手动操作路径。
2. **P1：插件核心 + 标准市场**。自有 schema、市场地址/凭据/信任根、多源命名空间、缓存、固定版本/摘要、权限审批、持久任务、归属和健康状态。先用已有外部连接/内置固定引擎作真实闭环，避免首次同时扩大执行面。
3. **P2：fnOS 原生适配**。仅在 fnOS 宿主代理能力就绪时开放 install/list/start/stop；升级卸载按 P0 实测结果开放，否则由应用中心完成。保留原生向导和依赖语义，不在 Windows 假装具备能力。
4. **P3：审核容器应用目录**。将已适配 Compose 子集维护为自有插件描述，必要时把来源 FPK 与适配器版本/摘要记录为追溯资料。不是“任意 FPK 自动转换器”。
5. **P4：逐应用扩展兼容**。仅在用户明确目标和样包后增加必要特性；每次新增系统依赖都补平台和升级/回滚验证，不以模拟完整 fnOS 为目标。

## External References

以下均为 2026-11-19 实际读取的官方公开页面。短摘用于识别关键证据，每个来源只保留必要短句。

| ID | URL | 关键证据/短摘 |
| --- | --- | --- |
| S1 | https://developer.fnnas.com/docs/cli/fnpack/ | “打包为可安装的 .fpk 文件”；列出项目结构与 Windows/Linux/macOS 打包工具 |
| S2 | https://developer.fnnas.com/docs/core-concepts/framework/ | “在 /var/apps/{appname} 下创建应用目录结构”；目录与生命周期定义 |
| S3 | https://developer.fnnas.com/docs/core-concepts/manifest/ | “检查兼容性，并准备运行环境”；架构/OS 版本/依赖/端口字段 |
| S4 | https://developer.fnnas.com/docs/core-concepts/resource/ | “需要的系统资源和集成能力”；data-share、usr-local-linker、docker-project |
| S5 | https://developer.fnnas.com/docs/core-concepts/privilege/ | “大多数应用应使用专用包用户运行”；package/root、附加组和目录权限 |
| S6 | https://developer.fnnas.com/docs/core-concepts/environment-variables/ | “提供环境变量”；TRIM_*、向导变量、认证令牌与安装目录 |
| S7 | https://developer.fnnas.com/docs/core-concepts/dependency/ | “依赖检查不递归”；安装依赖规则不可随意替换 |
| S8 | https://developer.fnnas.com/docs/examples/docker/ | “镜像本身仍需要支持目标设备架构”；Compose、生命周期与网关示例 |
| S9 | https://developer.fnnas.com/docs/cli/appcentercli/ | “飞牛 fnOS 设备上的应用中心命令行工具”；本地安装/列举/启停 |
| S10 | https://developer.fnnas.com/docs/quick-started/prerequisites/ | “一台用于安装和运行应用的 飞牛 fnOS 测试设备”；测试设备/管理员能力要求 |
| S11 | https://developer.fnnas.com/api/overview/ | “当前重点放在文件授权、页面路由、界面状态和后端查询等基础能力上” |
| S12 | https://developer.fnnas.com/docs/quick-started/publish-application/ | “开发者后台上线前”；提交应用、测试和发布材料的流程，并非第三方市场协议 |

## Caveats / Not Found

- 未接触真实 fnOS 设备、实际市场端点或 FPK 样包，因此这是一份设计研究，不是兼容性验收结果。
- 官方文档会更新；目标 fnOS 版本必须锁定后重新核验，不能把当前文档当所有历史版本的契约。
- 页面中的共享目录有 `share/` 与框架图 `shares/` 的用词差异；实现应使用系统返回变量/真实能力，不靠手工创建同名目录模拟。
- 未证实市场索引/签名/授权 API，不可把抓取到的私有接口或临时下载链接当稳定合同；也不应绕过签名或审核。
- 未证实 CLI 的升级/卸载/取消/结构化输出，均为 P0 门槛。即便相同 install-fpk 在某版本能覆盖安装，也必须明确验证其升级语义、向导、迁移与失败数据状态。
- 本系统提供 FPK 安装包属于“把本系统部署到 fnOS”的另一项交付需求，并不自动让本系统兼容运行其他飞牛应用。
- FPK 元数据描述即使有 HTML 也必须清洗后展示；远程市场地址、图标、描述、下载/重定向与 Compose 都是独立不可信输入。
