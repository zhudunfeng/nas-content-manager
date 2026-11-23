# Research: S09–S12 自动导入、容量治理、规则与组件维护实施拆分

- Query: 如何在现有获取/发布/入库和固定组件控制面上，逐步实现 S09 外部成品自动导入、S10 历史容量治理、S11 规则模板与缺集解释、S12 组件维护？
- Scope: internal，实施规划；所有新接口、表名、状态和字段均为拟议。
- Date: 2026-11-23
- 基准: main / `6f380be`。不实施业务代码，不创建子任务，不修改既有评估资料。

## 研究依据与入口

复用 [既有获取能力研究](../../11-23-nas-feature-gap-assessment/research/acquisition-capabilities.md)，本轮通过 CodeGraph 定向查询 `collectionLocalPreview`、`collectionTakenEpisodes`、`makeCollectionSpec`、`StopAll`，再读真实调用入口。没有展开外部选型或核验上游当前版本。

| 入口 | 现有契约与实施含义 |
| --- | --- |
| `internal/app/collection_preview.go:19–121` | 授权目录和有界文件预览已复用下载/收集发布门禁；S09 不另开文件系统通道。 |
| `internal/app/collection_preview.go:124–350` | `register`/`copy`、预览修订、幂等和指纹；应抽出共享应用服务供 HTTP 与调度调用，不在后端向自身 HTTP 发请求。 |
| `internal/app/collection_import.go:163–240` | 版本复核、安排扫描和实际目标覆盖；所有自动导入仍走同一判定。 |
| `internal/app/collection_import.go:12–71` | 历史根与发布清单仍由任务记录承载，S10 不能删除任务行后假设媒体继续可读。 |
| `internal/app/collection_store.go:261–320` | 活动快照、原 requestId 与 fingerprint 去重、256 队列上限；S09/S10 必须保留。 |
| `internal/collectionworker/service.go:164–200`、`:294–341` | 启动恢复和新提交都限制 10000 条内存操作；已有 requestHash 冲突判定。S10 要解决冷历史检索和幂等，不只是移除上限。 |
| `internal/collectionworker/files.go:328–399` | 发布意图与原子移动、崩溃对账；归档不能改变 published 的事实。 |
| `internal/app/collection_rules.go:40–111`、`:114–192` | 季集显式映射、持久历史去重和歧义转人工；S11 在其上增强解释。 |
| `internal/app/collection_api.go:315–355` | 原规则只绑定 RSS，自动模式要求关键词、季集与选文件表达式。 |
| `internal/app/collection_components.go:11–16` | 当前仅状态、部署、启动、任务和重试；新维护动作必须单独能力协商。 |
| `internal/enginehelper/collection_api.go:58–122`、`:129–291` | 实际状态、来源/模板修订、请求和活动意图去重。 |
| `internal/enginehelper/collection_templates.go:24–101` | 固定产品、镜像、网络、挂载和完整容器验证。 |
| `internal/enginehelper/collection_volumes.go:21–115`、`:206` | 安装身份、角色、nonce 与精确标签；不可只凭容器或卷名认领。 |
| `internal/enginehelper/stop.go:18–105` | 已有 stop-all 下的单组件核验停止原语；S12 可复用，不重新实现任意 Docker 控制。 |
| `internal/enginehelper/tasks.go:14–108` | 单一执行器、先持久意图后副作用、失败保留资源。 |
| `internal/desktop/docker.go:69`、`:292`、`:521`、`:556`、`:814` | 清单、运行实例、挂载和完整停止验证，S12 需同步升级兼容策略。 |
| `internal/desktop/docker_compose.go:92`、`:196` | 归一化 Compose 和挂载白名单，不能因新动作放宽现有校验。 |

## 共同实施规则

- 按实施时实际数据库和协议头版本制定下一次增量迁移；不预定 schema 版本号，不修改历史迁移，不把此研究当现有 spec。
- 新查询沿用会话鉴权；变更沿用同源检查、严格 JSON、有界输入、中文错误。新副作用使用持久 requestId、意图摘要和 revision/plan 绑定。
- 私密路径、凭据、nonce、上游原始上下文只在既有私密存储；公共 DTO 只给来源 ID、相对路径或经过授权的脱敏摘要。方法、字段、参数和关键逻辑按 AGENTS 使用中文解释与显式类型。
- 默认停用新增自动化。先读/试运行，再明确启用；停止策略不改源文件，不自动取消或重发已接受的外部动作。
- 本机隔离 fixture、真实 app/worker、浏览器与目标 NAS 验收分开记录。新增临时单元测试完成后清除，复用并保留集成脚本。

## S09：外部完成目录自动导入

### 目标与依赖

将“外部已完成、仍需手动预览登记”补为可显式授权的自动登记。仅作用于已配置并只读映射的外部整理完成目录，不负责触发上游下载或整理。依赖已有 register/preview/task/import；事件中心仅用于通知，不应阻塞核心实现。目标库和目录映射必须先存在；首轮不自动创建任意媒体库。

### 可独立交付顺序

1. **S09.1 完成目录映射与只读检测。** 在原来源弹窗补“本地完成目录 → 目标库”摘要，复用 `collectionLocalDirectory` 和 `collectionLocalPreview` 做可读性/门禁/类型检查，分别显示上游 API 在线、本地挂载可读、目标库可用。先提供检测和手动试运行，不启用调度。
2. **S09.2 共享预览/建任务服务与发现台账。** 从 `collectionCreatePreview`、`collectionCreateTask` 抽出有显式类型的应用服务，HTTP 和自动导入共用。按来源修订、规范相对路径、size/mtime 与文件清单摘要持久记录 observation、关联 taskId、原因和最后检查时间，调度请求键在入库意图创建前持久化。
3. **S09.3 增量发现、稳定检查、明确自动授权。** 默认仅对启用后的新产物建立候选；历史基线导入需单独预览确认。采用有界轮询，不要求跨平台文件监视器。相同文件版本至少连续稳定观测、避开临时扩展名并复核目标后，可按已授权策略调用共享 register 流程。稳定 size/mtime 只是必要条件，不是下载完成证明；只信任用户明确指定的整理完成目录，禁止直接将 qB staging、下载中的目录或未知受管 complete 当完成根。
4. **S09.4 故障恢复和可见性。** 展示“发现/待稳定/待确认/登记中/已实际入库/失败”及具体原因；失败复用原任务重试入库。来源/目标修订变化使未提交观察失效，已提交任务保留不可变旧意图。移除目标库不从历史意图自动重建。

### 拟议接口与数据

- `GET /api/collection/sources/{id}/import-policy`：策略、修订、最近扫描摘要和启用状态。
- `PUT /api/collection/sources/{id}/import-policy`：`revision`（旧策略版本）、`enabled`（明确启用）、`libraryId`（已存在目标库）、`relativePath`（已授权根内子目录）、`mode`（首轮固定 register）、`intervalMinutes`（有界轮询间隔）、`stableMinutes`（稳定窗口）、`includeExisting`（只允许单次历史确认意图，不能默认 true）。字段名实施时与来源编辑协议统一。
- `POST .../import-policy/preview`：只读有界试运行；返回候选和不能导入原因，绑定来源/策略/目标修订并有有效期。
- `POST .../import-policy/run`：`requestId` + 预览/策略修订，手动运行与调度均经过相同服务；可返回有界批次任务，而不是一次创建无限子任务。
- `GET .../import-observations?cursor=...`：分页发现台账，`state`、`reasonCode`、`taskId` 与已确认 itemIds；不公开宿主秘密路径。
- 可用来源策略附表和 observation 表，或扩展既有来源 JSON + 独立发现表；唯一性与版本检查必须在数据库短事务落实。文件扫描/上游读取不得持有 DB 事务。
- 全目录新快照可能包含已导入文件，须依发现台账形成有界新文件/目录批次；不靠每次随机 UUID 绕过 fingerprint。相同意图返回原任务，变更目标/参数沿用原冲突规则。

### 入口文件

`internal/app/collection_preview.go`、`collection_import.go`、`collection_store.go`、`collection_models.go`、`collection_runtime.go`、`collection_api.go`；新增专用 `collection_auto_import.go` 容纳策略与调度，避免继续扩大路由文件。前端 `CollectionSourceDialog.vue`、`CollectionExternalPanel.vue`、`CollectionCenter.vue`、`CollectionTaskDialog.vue`、`web/src/types.ts`。验证扩展 `scripts/verify_collection.py`，外部动作回归用 `scripts/verify_external.py`。

### 验收、回退与不做

- 覆盖目录未挂载/空挂载、文件增长和替换、符号链接越界、源停用、目标删除、策略 revision 冲突、批次一部分失败、响应丢失原键恢复、重启去重、gate 拒绝、worker 离线仍可读已发布文件。
- 至少一个真实外部完成文件经自动发现 → 原 register 任务 → 真实扫描 → itemIds → 鉴权 Range；同时证明未发任何新的上游下载/整理请求。
- 关闭策略只停止新发现，保留任务/台账/媒体；已有手动导入始终可用。迁移回退使用匹配备份，不让旧程序忽略未知 schema。
- 不接管 qB、不将外部 API 的 accepted/100% 当本地入库、不重发 uncertain 动作、不自动重命名/移动/覆盖媒体、不自动连接任意目录。
- **当步实机门禁：** 用户需提供实际部署方式、主服务可见的完成根和目标库；NAS ACL、外部整理何时原子完成、网络卷时钟/缓存语义须在该宿主验证。没有可信完成目录时只能生成待确认候选，不能宣称全自动可靠。

## S10：历史与容量治理

### 目标与依赖

先看清占用，再释放热历史压力，最后清理明确无引用的暂存。依赖一致性备份/恢复基线和既有 app/worker 身份；真正删除路径依赖可验证的归属、活动句柄、引用和回收规则。S10.1 的只读容量不应依赖 S09 或作品模型。

### 可独立交付顺序

1. **S10.1 只读容量/历史视图。** 分开显示原媒体、应用缓存、worker staging/complete、qB 活动/做种和各组件卷。物理字节与逻辑文件大小分开；共享卷、硬链接或无法测量给未知/估算，不叠加成虚假准确总数。显示热历史条数、剩余额度、低水位及异常操作；可只交付检测不清理。
2. **S10.2 保数据归档协议与冷历史查询。** app 只对已终态且没有待执行/对账/导入重试的任务生成归档计划。首批可限制 `completed + imported` 和已确认无活动进程的失败/取消项；`acquired/publishing/recovery_required/uncertain` 不归档。worker 将完整私密记录原子转为冷记录，按 UUID 直接查询，热内存仍有上限；完整记录和 payload 不删除。归档不是 publication revoke，也不是媒体删除。
3. **S10.3 在真实大量历史上验证替换 10000 条启动限制。** `GET /operations/{id}`、同 UUID 提交、publish/对账都能读冷热记录；已归档 ID 永不被当全新操作，hash 不同仍 409。启动只加载活动记录与有界索引，不全量读取冷 JSON；分页历史查询不全排序全量对象。冷库不可读或有损坏时失败封闭，不能视为不存在并重新下载。
4. **S10.4 可预览的失败暂存与可重建缓存清理。** 先扩展现有缓存清理；worker 仅清理明确终态、进程退出、无发布事实且无引用的 staging。每次生成带文件清单版本和过期时间的计划，再显式确认执行；执行前重验，变化项跳过并解释。发布 complete、qB 做种数据和组件数据库不在首轮自动清理范围。

### 归档必须保留的事实

| 对象 | 必须保留的最小语义 |
| --- | --- |
| app 任务与幂等 | taskId、requestId、指纹/意图摘要、来源和目标、最终状态、revision、原操作引用；归档后仍能定位原结果。 |
| 发布凭证 | 执行端 instanceId、授权根身份、operationId、ready/published、finalPath 的受控相对关系、完整文件 size/mtime 清单及可选摘要；用于所有扫描/流/封面门禁。 |
| 媒体关联 | libraryId/itemIds 和文件版本；删除索引不是删除文件。目标已删/文件变更要正确显示失效，不伪造仍可播放。 |
| RSS 去重 | 历史作品/季集身份和已接受获取意图，保留防重复信息；不能因归档让同一集再次自动下载。 |
| worker 冷记录 | operation/requestHash、完整固定请求与旧私密上下文、最终响应、manifest 和恢复事实，私密权限保持；冷记录不得公开。 |
| BT 引用 | downloadId、实际 qB 任务状态/归属及选集发布事实；仍在下载、做种或状态未知默认保留，worker 归档不控制 qB。 |

现有 `addCollectionGate` 直接依赖 `collection_tasks`，因此首版保留任务主行和 files，只增加归档标记、把非必要事件/私密热状态转冷。后续如拆独立 publication receipt 表，必须先迁移并逐项验证，再统一改门禁查询；不得先删除原行。需评估门禁全量加载的内存，设计按根/具体操作分页或按需精确读取，不能为省内存截断凭证后改变安全判定。

### 拟议接口与数据

- app `GET /api/storage/usage`、`GET /api/collection/history?archived=...&cursor=...`。
- `POST /api/collection/archive-plans`：`taskIds`（有界任务集）与修订，返回 planId、预计热记录减少量、保留文件说明、阻断原因。
- `POST /api/collection/archive-plans/{id}/execute`：requestId + plan revision，保存归档意图再调用 worker；跨 app/worker 以可恢复状态机对账，不伪造分布式原子事务。
- worker 拟新增受 Bearer/能力版本保护的 `/v1/archive-plans`、`/v1/archive-plans/{id}/execute` 与只读恢复查询；公共 GET 不创建目录/索引或移动历史。旧 worker 不支持时界面禁用归档，不能改成直接删文件。
- 清理另用 `POST /api/storage/cleanup-plans` 与 `.../{id}/execute`，由各数据所有者执行自己的范围；不能让主应用对 worker/qB/组件卷执行通用 rm。
- 新策略参数需中文解释，保留期/批次数/空间阈值均有硬上界。具体默认值由容量样本决定，不能靠无限增大 10000 规避设计。

### 入口、验收与回退

入口：`internal/collectionworker/service.go`、`files.go`、`retry.go`、`internal/collectioncontract/models.go`；app `collection_store.go`、`collection_import.go`、`download_gate.go`、`collection_runtime.go`、`collection_rules.go`；前端收集任务页和设置存储页。脚本扩展 `verify_collection.py`、`verify_downloads.py`，新增可复用归档/清理集成脚本需归属清晰。

- 验证超过旧阈值的冷热历史、重启内存与时延、并发提交/归档、旧 ID 原结果与冲突、归档中断各阶段、冷目录损坏、app ACK 丢失、回归原 gate、已入库文件读取和 RSS 不重复。
- 清理验证活动进程、引用变化、路径重解析点、同名替换、权限不足、部分 I/O 失败和空间统计不误报；真实磁盘低空间与文件系统语义单独设备验收。
- 关闭归档/清理调度不影响冷热读取，冷记录可通过恢复工具重新载入；新程序认识冷格式后方可归档，旧程序不具备读取能力时必须以完整停机备份恢复，不能直接降级程序。
- 归档只减少热状态压力，首期不回收完成媒体空间；不可把“任务隐藏”宣传为释放 N GiB。真实删除需独立授权，不删除原媒体、发布凭证、做种数据、秘密或组件数据库。

## S11：规则模板、命中解释与缺集视图

### 目标与依赖

先降低正则门槛并说明决策，再基于确认的作品/季集身份显示缺集。S11.1–2 只依赖现有 RSS 与候选；S11.3 缺集依赖作品/季集模型及可信预期集目，不应阻塞前两小交付。外部元数据尚未可信时允许用户维护有限预期集目，不推测全集数。

### 可独立交付顺序

1. **S11.1 固定模板生成器。** 用已核验的有限模板生成原有 `include/exclude/quality/episodePattern/selectionPattern`，版本化模板 ID 仅辅助编辑；持久保存实际规则快照。保留高级表达式，不后台改写旧规则。试运行永不获取。
2. **S11.2 结构化解释与冲突预览。** 给每条候选返回规则快照修订、命中/排除条件、季集解析、是否已有意图、多个发行/规则冲突和转人工原因；reasonCode 是稳定枚举，中文 reasonText 是展示。预览与调度共用同一 evaluator，过期结果不能作为新动作授权。
3. **S11.3 作品/季集绑定与缺集账本。** 对已确认作品身份绑定 source-independent workId/season/episode；迁移旧规则先无绑定，不按标题自动认领。分别显示“预期但未发现、已发现待选、已有获取意图、下载中、已发布待入库、已实际入库、原文件缺失、失败待处理”。缺少预期集目时显示未知，而非 0 集缺失。
4. **S11.4 候选优先级，先建议再自动。** 在身份明确后再加画质/语言/字幕偏好与候选排序，默认只推荐；多版本替换、洗版、删除旧文件和绝对集数映射自动推断另立范围，不在首轮启用。

### 拟议接口与数据

- `GET /api/collection/rule-templates`：固定模板 ID/版本、中文字段说明、可用内容类型和示例；不执行脚本或下载远程正则。
- 扩展现有 `POST /api/collection/subscriptions/{id}/preview`，分页输出 evaluator 解释和绑定 revision；原响应保持兼容或显式版本协商。
- 规则新增可选 `templateId/templateVersion`、经用户确认的作品/季集绑定和偏好；具体字段以作品模型已实施契约为准。
- `GET /api/works/{id}/episodes/status` 或同等作品域入口：预期与实际状态分开，可信来源、最后更新时间和未知项明确；不要把缺集表做成第二个下载权威。
- 建议持久 episode acquisition ledger/映射关系，唯一身份覆盖跨来源去重；原 requestId/fingerprint 继续约束传输任务。已有失败/取消历史也要区分“曾接受意图”与“当前可播放”，不能简单删除历史让定时任务重复提交。

### 入口、验收与回退

入口：`internal/app/collection_rules.go`、`collection_models.go`、`collection_api.go`、`collection_store.go`、`collection_bt.go`；前端 `CollectionSubscriptionDialog.vue`、`CollectionCenter.vue`、`CollectionAcquireDialog.vue` 与 `web/src/types.ts`；缺集视图接入届时已实施的作品详情域。扩展 `scripts/verify_collection.py` 与既有 qB fixture。

- 验证单集/多集/全季包、特别篇、相同季集多个发行、绝对集数未知、标题近似不同作品、跨源重复、规则修订冲突、冷历史去重、手动与自动竞争；歧义必须转人工。
- 缺集以真实 items/发布状态和可用性为准；外部订阅/下载完成不直接变成已入库。预期集目不完整时不显示误导性完成率。
- 新模板不改变已启用规则；停用新自动判定后原人工确认和高级正则仍可用。迁移先 additive，旧规则无绑定照常运行。
- **当步输入门禁：** 需明确首批资源命名样本、季集/绝对集号映射规则与预期集目来源。若依赖第三方元数据 API，实施时再核实其官方协议、许可、当前版本与预算，不在规划中假定当前可用。

## S12：固定组件维护

### 目标与依赖

仍限定现有四固定产品，延续私有 helper、固定镜像与安装身份。按“状态和安全启停 → 维护计划与备份 → 单一受控升级 → 保数据卸载”推进；不能一开始做通用镜像管理器。状态/单组件启停依赖当前 helper，无需等待 S09/S11；升级与卸载需已验证的完整备份恢复和该组件实机数据迁移证据。

### 可独立交付顺序

1. **S12.1 真实 readiness/版本与单组件安全启停。** 分开显示当前容器 image digest、应用 API 实际版本、模板目标版本、HTTP readiness、API 已连接与配置待完成。复用 `stopComponent` 扩展仅白名单单组件的停止动作，启动仍复用原动作。单组件停止不得设置全局 quiescing 或阻止其他组件；读取不自动启动。未知实际版本只能显示不支持维护，不据版本字符串自动接管。
2. **S12.2 只读维护计划与配套备份检查。** 计划固定源/目标模板、组件/安装/容器/卷身份、revision、摘要、端口/挂载/权限差异、兼容 API 能力与数据迁移风险。先不执行升级。检查备份覆盖 app/helper state/授权 nonce/组件私密卷及相关媒体根；备份只靠用户勾选不足以成为“可验证回退”。
3. **S12.3 首个产品和单条升级路径。** 从一个有真实宿主证据的产品开始，目标 image 先核验拉取，短事务持久维护意图，单组件维护锁阻止新 API 写与导入计划变更；停止/快照/替换/启动/HTTP + API 兼容验证按阶段恢复。源和目标模板均须在固定受信清单，不能放宽成任何旧 digest 都通过。成功后原连接重新 Probe，不能沿用旧版本 capability。
4. **S12.4 保数据卸载与按原身份恢复。** 卸载计划明确只移除已停止且确认为本安装的容器；保留命名卷、媒体、helper 安装身份/nonce 和恢复记录。对未知结果先 inspect 精确容器 ID，不因超时重复删除。再安装必须通过已保留身份和受信模板验证，不能把保留卷当新卷自动 claim。清库/删卷不在本任务。

### 拟议接口与持久数据

- 在现有 `GET /api/collection/components` 扩展维护能力和只读事实；旧 helper 通过能力协商显示 unsupported，旧界面不误启新动作。
- `POST /api/collection/components/{id}/stop`：requestId + revision，限定一个固定组件，沿用 durable TaskResponse。
- `POST /api/collection/components/{id}/maintenance-plans`：action 为固定 `upgrade|uninstall_keep_data`、targetTemplateId（来自固定白名单）；返回 planId、sourceRevision、expiry、完整权限/数据影响摘要。
- `POST /api/collection/components/maintenance-plans/{id}/execute`：requestId、plan revision、明确确认；不接受 image URL、Docker 参数、任意端口或环境。
- helper 保存 `maintenance plan → durable task → receipts`，包含旧/新容器精确身份、模板/镜像 digest、卷/nonce 引用、备份标识和恢复阶段。API 密钥和备份秘密不进入公共 DTO/主库导出。
- 新维护任务不可被旧 deploy/start retry 接口错误重试；任务作用域验证和动作白名单随协议同步更新。

### 模板、身份和 Windows 约束

- 继续使用 `collectionDefinitionFor`、`makeCollectionSpec`、`validateCollectionContainer`、`validateCollectionRuntime`；保持原媒体只读，四组件只写被授权独立 collection-media 与配置卷。
- 当前私密卷标签包含 template 字段。升级设计须区分长期不变的安装/角色/nonce 归属和某次容器模板事实，显式制定旧标签到新模板的兼容映射；不能直接改常量导致旧卷全冲突，更不能删标签或重新生成 nonce 绕过。
- 更换容器前后均验证所有卷、端口、网络、权限、镜像和真实 container ID；异常停止在当前阶段，保留数据供恢复，不接管外来同名资源。
- Desktop 清单、Compose 白名单、镜像包和运行验证同时跟进，包含 `docker.go` 的 `readManifest/verifyExisting/validateContainerRuntime/validateMounts/stopAll` 与 `docker_compose.go` 的 `validateComposeData/validateComposeMounts`；保留旧清单能力边界，不全局放宽既有版本。
- 相同维护动作/意图不确定后读取原任务；整机停止与单组件维护共用互斥/停止预算，不能让一个正在升级的组件被全局停止路径误报已成功。

### 验收、回退与当步门禁

- 本机 fixture：只读不产生 Docker 副作用、外来容器/卷/nonce 冲突、请求冲突、并发维护、失联/重启各阶段、部分成功、备份缺失/失效、上下游版本不支持、旧 helper、旧 Desktop 清单和完整 stop-all。
- Linux NAS和Windows Docker完整模式各自验证拟开放的维护动作，含容器初始化、ACL、配置持久性、精确归属、API能力变化和配套恢复；已验证组合可先交付，未实测的平台/产品/动作先禁用执行，只允许诊断/预览，不把等待全部平台当首条路径的前置。
- **升级回退只在证据允许时开放。** 旧镜像 + 旧模板 + 升级前一致性数据快照 + 相同安装身份/nonce 一起恢复；新版本已经迁移数据库时，不直接启动旧镜像读取新数据。若组件迁移不可逆或无恢复证据，计划显示维护不可回退，阻止所谓自动回滚；失败保留维护状态并提供按已验证备份恢复流程。
- 保数据卸载的恢复依赖保留卷和 helper state；删卷、清库、重置密码、自动配置真实站点、任意镜像、通用插件/FPK 仍不做。
- **实施前需核实：** 实际宿主/架构、Docker/Compose 与卷布局、目标产品现有版本、官方支持升级路径/发布说明、官方镜像 digest/平台清单、数据库迁移和备份/恢复方式。此处仅规定核实门禁，不引用尚未查询的“最新版本”。

## 建议交付依赖摘要

| 交付 | 必须前置 | 可并行/后置 |
| --- | --- | --- |
| S09.1–2 | 已有 register、目标库/完成根映射 | 无需 S11 作品模型 |
| S09.3–4 | S09.2 的幂等发现台账与可信完成目录；目标环境目录验证 | 通知渠道可后接 |
| S10.1 | 只读能力与数据所有者边界 | 与 S09、S11 模板并行 |
| S10.2–3 | 完整备份恢复、冷热协议与门禁凭证兼容设计 | 在实测长时运行前完成 |
| S10.4 | S10.1、引用/归属/进程校验与删除计划 | complete/做种清理后续单立任务 |
| S11.1–2 | 原 RSS evaluator 和试运行 | 不必等待 S09/S10 |
| S11.3 | 已确认作品/季集模型与预期集目 | 不能仅靠标题或下载历史替代 |
| S12.1 | 原固定 helper 与身份/任务协议 | 不必等待 S09/S11 |
| S12.2–4 | 完整备份恢复、目标版本官方路径与真实宿主证据 | 每产品/每平台逐条放行 |

## Related specs

现有 `.trellis/spec/backend/collection-contract.md`、`external-collection-contract.md`、`collection-component-contract.md`、`download-contract.md`、`cross-platform-contract.md` 和 `.trellis/spec/frontend/app-contract.md` 仍为现状约束。仅在实现和验证完成后，将对应已实现协议更新到 spec，不将本研究中的拟议接口提前写成现有能力。

## External references

本轮没有新增外部事实；S11 外部元数据、S12 官方升级/镜像/迁移信息列为实施当步核验门禁。

## Caveats / Not Found

- 未运行应用、Docker、测试、构建或目标环境；本文件是静态实施拆分，不是功能已完成或兼容性证明。
- 新 API/模型名是供子任务 PRD 收敛的拟议边界，不是可以直接依赖的现有契约；实施时需按最新分支重新定位行号与迁移头。
- 10000 条限制同时影响 worker 新提交与启动恢复；归档如果丢失 requestHash、发布清单或 app gate 行，会导致重复获取或合法媒体不可读，应作为实现审查的阻断项。
