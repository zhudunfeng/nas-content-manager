# BT 引擎初始化技术设计与实施计划

状态：代码与质量检查完成，进入review。日期：2026-11-18。实际验收及环境限制以 `prd.md`、`research/implementation-verification.md` 为准；下文保留设计背景，实际API与参数以 `docs/api.md`、`docs/engines.md` 为准。

## 1. 架构与适用范围

```text
浏览器（现有管理员会话）
  → 栖库同源 /api/engines/*
    → 私有 Unix socket
      → nas-engine-helper（固定模板 + 独立部署状态库）
        → 本机 Docker Engine HTTP API
          → qBittorrent：专用配置 + 专用下载根
          → Jackett：专用配置，无媒体卷

助手 → 原子发布只读连接清单 → 应用运行时激活/来源同步
引擎 → 原下载协调器/搜种适配器 → 原选集/发布/五类入库流程
```

推荐独立小型 Go 助手，Docker 调用使用有界 HTTP 客户端和本机 Unix socket，首版不依赖 Docker CLI/Compose CLI 子进程，也不把任意 shell 命令放进请求。

首版仅同机 Linux Docker Engine、amd64/arm64。rootless、userns-remap、远端 daemon、Windows Docker Engine 等不默认宣称兼容；预检不满足固定挂载/身份/网络模型时明确禁用自动初始化，继续保留外接部署方式。

助手持有 daemon 管理权限，属于受信任部署组件。主程序没有 Docker socket；助手的模板白名单限制正常调用范围，不能声称它获得了 daemon 层最小权限。只读挂载 Docker socket 也不能把其 API 变为只读。

## 2. 首次部署与目录/网络契约

新增可选 `compose.engines.yaml`：启动助手并预挂载应用需要的目录，但此时不创建 qB/Jackett。基础 `compose.yaml` 保持不变。旧下载/搜索叠加与托管叠加默认二选一；逐模块混合 external/managed 需清楚指定模式并校验，不能隐式覆盖连接。

| 资源 | 主程序 | 助手 | 引擎 |
| --- | --- | --- | --- |
| 原媒体 /media、应用 /data | 保持原配置 | 不挂载 | 不挂载 |
| 专用 download 子目录 | 固定 /downloads，只读 | 仅预检及目录准备，不写载荷 | qB /downloads 可写，Jackett无挂载 |
| qB/Jackett config 子目录 | 不挂载 | 专用根内管理 | 对应 /config 可写 |
| connections 子目录 | /run/nas-engines，只读 | 写入版本文件与清单 | 不挂载 |
| helper state 子目录 | 不挂载 | 保存安装ID、独立state.db及待发布凭据 | 不挂载 |
| 私有 IPC 目录 | 仅供访问助手socket | 创建Unix socket | 不挂载 |
| Docker socket | 不挂载 | 可调用 | 不挂载 |

宿主示例专用根 `/srv/nas/bt`，派生 downloads/qbittorrent/jackett/connections/state 子目录。宿主根与助手内 `/managed` 是不同路径：传给 Docker 的 bind source 必须由受信任的宿主根配置派生，不能把助手内 `/managed/...` 当作宿主路径。只允许固定子目录，拒绝符号链接逃逸、目录重叠和未知非空配置。

应用在托管部署启动时就固定授权 `/downloads/complete`，不得授权整个 `/downloads`；根路径不得在线改变。Compose 即使预先创建了空目录，助手仍需检查目标UID/GID/ACL、挂载身份及可读写性，不能用root写成功代替目标进程的权限检查。

qB/Jackett 与应用加入受信任的固定 bridge，允许搜索/BT所需出站，不能误设 `internal: true` 后声称可访问互联网。WebUI不发布公网；qB只发布预设 peer TCP/UDP；Jackett首次配置入口可按现有模式绑定NAS回环端口并给出SSH指引。网络/端口名称由部署配置提供，浏览器不能修改。助手不通过 HTTP 发布控制接口。

**必须告知的限制**：未接入助手的既有单应用部署需要用户先应用这份覆盖配置；网页不能安装宿主Docker，也不能给正在运行的容器追加bind mount。后续代码应给出直接的“如何启用初始化”指引，而非一个注定失败的按钮。

## 3. 模式与有效运行配置

拟新增 `NAS_DOWNLOAD_MODE`、`NAS_TORRENT_SEARCH_MODE`，可选 `disabled|external|managed`。显式新mode优先；未设置mode时沿用原 `*_ENABLED`（false→disabled，true→external）。托管示例不再同时设置旧开关；文档解释新mode是明确选择，不悄悄把external转换为managed。

- disabled：不启动对应模块、不访问引擎；如需启用托管，先通过部署配置选managed。
- external：保留现有URL、secret和来源文件的严格启动校验；不读取托管凭据，不自动管理实例。
- managed：固定下载根等静态边界必须有效，但允许初次没有连接清单；应用仍启动，显示待初始化。只有本安装助手生成的有效清单能激活模块。

不能在HTTP请求中修改 `os.Setenv`、直接改 `App.config` 或无锁改 `manager.config`。新增统一有效运行快照及generation，替换有关处理器对静态Enabled的直接依赖。原配置保持不可变；门禁使用固定根和原持久任务，禁用/助手离线也不能失效。

下载协调器使用专门生命周期锁执行激活，最多一个worker；完成新client验证后发布有效状态。重复读取同generation不产生第二个worker。存在历史托管下载时，拒绝将连接切换到另一实例或根路径，防止旧任务被发给新实例。

搜索来源刷新构建新immutable快照，以generation隔离在途请求；废弃未绑定的旧结果令牌并显示需重新搜索。已被原下载请求持久绑定的重试仍先查原binding，不因配置更新而重复添加。

## 4. 状态与界面动作

组件状态采用独立维度，不用一个online字段概括一切：

- mode：disabled/external/managed。
- presence：unknown/missing/present/conflict；external通常unknown，不做Docker安装扫描。
- runtime：unknown/stopped/starting/running/offline。
- readiness：not_configured/initializing/configuration_required/connecting/ready/auth_failed/incompatible/error。
- helper状态：not_enabled/ready/unavailable/unsupported，失联只影响部署控制面。

| 检测结果 | 界面主动作 | 允许的副作用 |
| --- | --- | --- |
| 托管模式，助手确认不存在 | 初始化搜索/下载，或初始化缺失引擎 | 固定模板拉取/创建/启动 |
| 同安装已停止容器 | 启动引擎 | 启动原容器；不重建、不改密 |
| 已运行且就绪 | 已就绪/测试连接 | 只探针 |
| Jackett运行但没有选定来源 | 配置来源、同步来源 | 仅来源发现和选择，不运行关键词搜索 |
| external离线/401 | 测试连接、修复配置指引 | 不安装、不接管 |
| 助手/Docker超时 | 重试检测 | 不把unknown显示成missing |
| 标签、镜像、挂载或未知配置冲突 | 查看处理指引 | 不覆盖、不删除 |
| 已安装但应用尚未加载清单 | 正在接入/重试连接 | 重新激活，不重复拉取 |

下载中心增加两张紧凑状态卡和一个原生初始化面板，沿用主题变量和DialogShell；不增加手机底栏入口。展示各组件阶段、层进度、日志、失败及重试入口。关闭面板只停止前端轮询，不取消安装任务，Esc/关闭恢复原按钮焦点；不堆叠多个原生模态框。

## 5. 初始化状态机与Docker调用

持久任务状态：queued/running/completed/failed/interrupted；每组件独立状态及attempt/revision。两个组件串行执行，首版全局最多一个部署执行器，避免NAS同时解包两个镜像争抢资源。

步骤：preflight → pulling → preparing → creating → starting → credentials → healthcheck → publishing → completed。应用再独立显示binding连接状态；助手completed仅代表部署与连接清单发布成功，不代表来源已配置或应用已经可下载。

- preflight：协商Engine API版本并检查可用能力、OS/架构、固定网络、资源归属、目录/权限、预置端口、首次告知。按实际可观测能力显示磁盘信息；助手不能访问Docker存储卷空闲量时显示未知，不能把下载卷statfs当作镜像存储空间。Docker报告ENOSPC按拉取/解包错误处理。
- pulling：先按固定digest检查镜像；缺失时调用images/create并消费NDJSON。HTTP200仍须检查流中error/errorDetail，成功结束后再次inspect确认目标digest和平台。仅显示有实际current/total的层进度，解包和全流程不得假造百分比；重试复用Docker已有层，不承诺所有中断字节都可续传。
- creating：先提交持久intent，再请求确定名称+安装ID/component/template标签的容器。创建返回丢失/冲突时inspect并核对标签、image和完整预期挂载，再决定复用；不能盲目重发或删除。
- starting：用户显式初始化/启动才调用；页面读取或周期状态刷新不拉起被用户停掉的容器。
- credentials/healthcheck：仅对本次新建且配置归属明确的实例初始化。失败保存阶段与安全错误，默认不清理卷或重置配置。
- publishing：先持久保存已验证凭据文件，再原子发布清单；应用使用实际生产适配器确认连接。

Docker客户端只实现固定的ping/version/info、镜像inspect/pull、网络inspect、目标容器inspect/create/start及失败新实例的stop。MVP不提供exec、任意日志代理、build、plugin、swarm、任意卷/网络删除或prune；正常外部请求不能指定Docker路径。既有实例恢复不会改变其全局偏好。

## 6. 凭据与搜索来源

### qBittorrent

优先针对固定5.2.4模板，在首次全新空配置目录离线预置随机永久密码的官方PBKDF2格式及必要设置；明文仅在助手私密文件及应用只读凭据中，qB配置写哈希。准确配置路径、QSettings序列化和算法参数以研究引用与实施探针为准，不泛化到其他镜像/版本。

密码及“已准备配置”的intent必须在创建/启动前持久化；中断恢复复用同一密码，不能每次尝试重新生成。启动后由原生产qB客户端实际登录，检查版本/WebAPI和元数据能力门槛。不能仅因容器running或测试脚本登录成功就显示可用。

该方案在实施首阶段先做无Docker的固定版本真实二进制探针；目标Linux固定镜像使用用户授权主机实际隔离验收。若固定配置不被识别，停在credential_failed并修正模板，不自动读取stdout临时密码、不关闭认证、不开启子网免密，也不修改任何已有外部/未知配置。上游首次告知由界面展示，必须取得用户对固定noticeVersion的显式确认才设置既有模板要求的确认参数。

### Jackett

等待固定模板生成专用ServerConfig后，有界读取API Key，精确路径按镜像模板验证。只读取本安装的固定文件，不允许浏览器传路径，不遍历任意Docker卷。API Key不会自动产生可用站点。

首版用户在回环WebUI/SSH入口设置管理员密码和站点。应用/助手通过固定Torznab发现接口同步已配置索引器，不依赖“管理API也支持apikey”的假设：候选路线是 `.../indexers/all/results/torznab/api?t=indexers&configured=true` 加服务端Key；这里all仅用于索引器发现，实际关键词请求仍使用单索引器端点。确切返回结构、设置管理员密码后的访问和configured过滤需对固定版本探针确认。

发现列表使用独立有界解析，拒绝DTD，最多2MiB响应、128个候选及10秒超时；只向前端返回安全ID/名称。0个显示待配置，1–8个可自动接入；超过8个要求用户选择最多8个，不静默截断。ID映射稳定且符合现有1–40字符约束，实际上游ID用于构造固定已验证路径；拒绝未知候选ID及可疑路径。

来源刷新是显式操作，不能每个前端轮询都请求Jackett或触发实际站点搜索。既有source XML解析、下载代理边界、短期结果缓存和下载binding契约全部保留。

## 7. 持久状态、发布与恢复

主业务SQLite维持v2。助手使用单独state.db，避免复用scan/enrich的jobs或业务DB连接；独立库在本机持久盘、单写连接、短事务，网络/拉取永远在事务之外。

拟有：installations（安装ID/模板版本）、components（已知容器ID/name/digest/配置归属/generation）、initializations（幂等请求/规范化目标/状态/revision/时间）、steps（组件/阶段/attempt/安全错误）、events（递增序号/组件/级别/有界中文消息）。凭据不进这些表；不得记录原Docker日志、完整环境或上游响应。

相同requestId绑定规范化的组件集合与noticeVersion，原请求参数不同返回409。未完成组件有唯一活跃执行占位；不同requestId重复请求相同目标复用已有执行并保留别名binding，不堆积第二套部署。重试需要原revision，且旧worker退出后才可重新领取。失败任务历史保留、非活跃详情最多保留最近100次；执行中的任务不被清理。

助手创建安装ID必须早于任何Docker资源；同组件名称由安装ID派生。每次网络操作后重新读取持久intent与目标元数据，解决“请求已成功、响应丢失”。已有用户停止意图不会因重启对账而被清除；只有未完成且已授权的初始化任务可继续原步骤。

连接目录采用固定component/generation文件名，不接受清单内任意路径。每份版本文件最大64KiB、严格字段/类型校验；先写临时文件+fsync+rename，再原子更新current清单并同步父目录。同一generation重复应用无副作用。应用挂载整个目录只读，避免单文件bind/secret对原子替换的可见性问题。目录与文件按实际1000:1000身份授权，并检查错误权限；不使用chmod777。

应用每5秒有界检查本地清单generation，或在安装成功后立即尝试加载；助手失联仍可用已提交清单恢复现有连接。解析失败保留最后已验证的运行快照并显示诊断，不切换到半成品。新qB实例不能无声接替有历史记录的旧实例。

## 8. 对外接口草案

所有接口均为同源管理员接口，JSON普通请求仍64KiB，错误遵循 `{error:中文说明}`。内部助手协议不透传Docker数据。

| 接口 | 入参/关键返回 | 行为 |
| --- | --- | --- |
| GET /api/engines | helper状态、组件模式/多维状态/能力/安全诊断、activeTaskId、noticeVersion/告知文本 | 无部署副作用；输出不含内部URL或secret |
| POST /api/engines/initializations | requestId、components枚举集合、noticeVersion与acceptNotice | 短请求，持久化后202返回task；只处理已选择managed组件 |
| GET /api/engines/initializations/{id} | afterSequence可选；task、步骤、最多100条增量事件、nextSequence | 可恢复轮询；事件缺口明确要求重载快照 |
| POST /api/engines/initializations/{id}/retry | requestId、revision | 仅重试失败组件；不重置成功组件 |
| POST /api/engines/{component}/start | requestId、revision | 仅已确认同安装stopped容器；走同一持久任务框架 |
| POST /api/engines/probe | 无地址入参；返回最新检测 | 有界检测，不安装/重建/重置 |
| POST /api/engines/search/sync-sources | sourceIds可选；返回候选/已应用来源、安全状态 | 仅managed Jackett；候选超过8个时等待选择，不能接受URL/Key |

400无效/未知字段，401缺会话，403跨源，409冲突/旧revision/告知未确认，413超预算，429忙，502受控部署失败，503助手未启用或不可用；上游qB认证失败是组件auth_failed，不能转成应用401踢出用户。任务执行失败通过任务状态返回，读取任务成功仍HTTP200。

前端轮询建议每2秒，完成再安排下一次；隐藏页面降频或暂停，返回时重载。每次只有一个在途请求，使用AbortController与request generation丢弃旧响应；浏览器取消不传播为服务端任务取消。requestId在POST超时后保持原值。

## 9. 默认参数草案（每项必须中文解释）

| 拟新增/复用参数 | 默认/示例 | 中文解释 |
| --- | --- | --- |
| NAS_DOWNLOAD_MODE | 未设置沿用旧开关 | 下载模块选择disabled/external/managed，明确指定时优先 |
| NAS_TORRENT_SEARCH_MODE | 未设置沿用旧开关 | 搜种模块选择disabled/external/managed，明确指定时优先 |
| NAS_ENGINE_HELPER_SOCKET | 未设置禁用控制面 | 应用访问助手的私有Unix socket固定路径 |
| NAS_ENGINE_CONNECTION_DIR | /run/nas-engines | 仅managed读取的只读版本化连接目录 |
| ENGINE_HOST_ROOT | /srv/nas/bt示例，部署必填 | Docker宿主专用管理根，必须与助手bind source一致，不接受网页修改 |
| ENGINE_NETWORK | 部署指定的固定bridge | 应用与托管引擎实际加入的网络，允许所需出站 |
| ENGINE_UID / ENGINE_GID | 1000 / 1000 | qB/Jackett专用目录的目标进程身份，按NAS实际ACL调整 |
| QBT_PEER_PORT | 6881 | 固定qB peer TCP/UDP端口，不是WebUI端口 |
| JACKETT_SETUP_PORT | 19117 | 首次设置来源的宿主回环WebUI端口，远程经SSH隧道 |
| 部署执行并发 | 固定1 | 单NAS逐个拉取/解包，避免资源突发占用 |
| 拉取阶段期限 | 固定30分钟/组件 | 慢网络有明确失败与重试，不无限挂起；真实需求再调整 |
| 就绪探针期限 | 固定120秒/组件 | 进程running不代表接口健康，超时保留配置并报错 |
| Docker普通调用期限 | 固定10秒 | 独立于长拉取流，控制接口等待有界 |
| 拉取流资源预算 | 最多128层、64KiB/行 | 仅保留最新层状态，不保存无限NDJSON；异常超限终止并诊断 |
| 事件预算 | 500条/任务、256字符/条 | 中文脱敏事件环形保留；每秒至多一次进度入库 |
| 事件页面 | 最多100条 | 用sequence有界增量读取，不返回整段Docker日志 |

固定常量也要中文注释。发布镜像、摘要、端口/环境支持清单集中在模板中；不开放网页任意修改镜像仓库或添加特权参数。代码交付时补齐实际参数名与单位，而非直接把本表宣称为已有配置。

## 10. 失败和恢复矩阵

| 情况 | 处理与保留 |
| --- | --- |
| daemon不可访问、API版本/架构不支持 | presence unknown；不给初始化，保留媒体服务 |
| 镜像鉴权/限流/网络错误、摘要不匹配 | 当前组件失败；固定镜像重试，不降级latest或随机镜像源 |
| 拉取流200中途error、EOF前断开 | 不进入创建；inspect核验后决定重试，不能以HTTP200为成功 |
| 下载目录无权限/卷缺失 | 停在预检；不chmod777，不修改原媒体权限 |
| Docker镜像存储空间不足 | 明确ENOSPC诊断，不自动prune或删下载文件 |
| 端口被占、同名外部容器、标签不符 | 冲突；只给修复配置指引，不停止外部容器 |
| create/start响应丢失 | inspect确定资源状态后恢复，不重复创建或改密 |
| qB配置预置未生效、版本不匹配 | 不发布ready；不抓日志自动降级，不改外部配置 |
| Jackett无来源 | 部署成功但configuration_required；其他下载功能可用 |
| 一组件成功、另一组件失败 | 分开展示，重试只做失败部分；不回滚成功组件 |
| 原子发布中断、清单损坏 | 读取最后完整generation；原有连接保持，重新发布/接入 |
| 用户刷新、浏览器断线、助手重启 | 原任务ID/intent/归属恢复；无重复资源 |
| 助手离线但引擎在线 | 控制面不可用，原下载/搜索继续；不联动停引擎 |

## 11. 分阶段实施文件清单

1. **P1 助手基础**：新增 `cmd/nas-engine-helper/main.go`、`internal/enginehelper/`（配置、模板、Docker客户端、store、tasks、IPC）；必要的无副作用DTO放 `internal/enginecontract/`。新增 `Dockerfile.engine-helper`、`compose.engines.yaml`、`.env.engines.example`。助手镜像本地构建，不虚构已发布公共镜像。
2. **P2 引擎安装**：在助手内补qB/Jackett固定模板、离线凭据初始化、来源发现、拉取进度、创建不确定性及重启对账；先做固定版本非Docker探针，不能把fixture通过当作镜像首启验证。
3. **P3 应用和界面**：新增 `internal/app/engine_*.go`，调整config/server/download配置与生命周期/原搜种manager；新增 `EngineSetupDialog.vue`，接下载中心和搜种未配置入口；更新types、api helpers和局部CSS。
4. **P4 验收与文档**：新增可复用 `scripts/verify_engine_bootstrap.py`，复用已有验证工具；更新README、API、verification和部署/备份恢复文档；按实际结果回填PRD，不预打勾。

## 12. 后续验收、备份与回退

自动验收先使用无Docker模拟：fake Docker HTTP用于pull流中error、分层进度、创建响应丢失、冲突/幂等/重启；真实helper状态库和真实应用用于鉴权/同源、运行时激活、secret发布、外接兼容及生产adapter流程。临时单元测试执行后只清理本任务生成文件，保留集成脚本。

原BT、搜种、五媒体及`--large-file`回归已执行；gofmt/vet/test/build、strict vue-tsc和Vite、Linux amd64/arm64交叉构建及双主题四宽度/焦点/请求竞态验收已完成。计数和实际覆盖以实施证据为准。

授权远程验收（仅清理本任务资源）：助手镜像构建/启动、socket权限、真实固定镜像的首启配置、架构与网络、回环管理入口、站点来源同步、UID/GID/ACL、重启恢复、真实下载与超大视频并发。不得把假Engine API或Windows二进制探针等同于Linux容器运行结果。

备份需分别保留主应用一致性快照、助手state/安装ID、连接凭据、两引擎config/恢复数据和下载根；凭据不纳入普通应用数据库备份。恢复这些相关实体后先对账再开放修改，不能遗失安装ID后接管旧容器。回退到external时显式写回原文件配置并停用助手控制面，保留卷/历史任务和发布门禁；本功能不提供一键卸载或删数据。

## 13. 实施授权更新（2026-11-18）

用户已要求按计划实现，允许通过指定Linux主机Docker TCP服务部署测试。该TCP地址仅属本轮测试入口，产品助手仍只连接同宿主Unix socket；不新增远端daemon产品支持。测试前保存基线，为新资源设置随机本任务标签，测试后仅删除本任务容器、无其他引用且不在基线的测试镜像、网络和测试专用目录/卷，禁止全局prune。认证信息不进入仓库、命令行或证据。
