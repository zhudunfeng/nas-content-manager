# 2026-11 归档总结：当前项目可复用的要点

更新日期：2026-11-27（Asia/Shanghai）。

## 范围与证据口径

本总结补齐 2026-11-27 批量归档的 **50 个任务**。本月此前已有的 [密码可见性任务](./11-20-password-visibility/prd.md) 作为补充经验单列，不计入 50 个；本次总结维护任务另有 [自身总结](./11-27-trellis-archive-lessons/summary.md)。

原生 archive 命令会把状态写为 `completed`、改写 `completedAt` 并处理活动父子关系。这些是生命周期元数据，**不能证明功能实现、测试、真实设备验收或正式分发已完成**。归档前状态和父子关系已从源提交 `68bbd0e` 提取为 [可机读快照](./11-27-trellis-archive-lessons/research/pre-archive-task-status.json)。

| 归档前状态 | 数量 | 使用方式 |
| --- | ---: | --- |
| planning | 10 | 阅读规划与未决输入，不当作当前接口 |
| in_progress | 6 | 检查已落地切片和未完成事项，不整体推断 |
| review | 23 | 查具体实现、复核和环境限制 |
| completed | 11 | 仍需区分本机、fixture 与目标设备证据 |

本次只整理已有任务资料、当前规范并定向核对源码，**没有重跑历史业务测试、访问真实上游或新增 NAS 验收**。以下“已实现/本机验证”来自对应记录；历史日期和环境保留在来源中。后续实现以当前 spec 和源码为准，旧 PRD 的 schema/接口/版本不得覆盖当前契约。

## 先记住的项目约束

### 1. 复用现有媒体事实来源，保护用户原文件和个人状态

- 当前主体仍是 Go HTTP + SQLite，五类媒体为 `video/image/comic/novel/music`；获取来源、下载器和插件是工作流，不另造内容类型或播放器。
- 数据库当前 head 为 schema 6；增量迁移保留 item 身份、收藏、标签和进度。音乐复用 Item/duration，不为其重建旧库。
- 源媒体只读，缓存是衍生数据。大媒体在授权和根边界检查后沿用文件句柄与 Range 流，不能整片读入内存或加破坏流式读取的全局压缩/短写超时。
- 来源与实施入口：[运行时契约](../../../spec/backend/runtime-contract.md)、[前端契约](../../../spec/frontend/app-contract.md)、[收集研究要点 1](./11-27-trellis-archive-lessons/research/collection-plugin-lessons.md)。

### 2. 先持久化意图，再执行外部副作用

- 获取、引擎部署和外部 API 写入复用稳定 UUID、不可变请求/预览及来源修订；超时或断线先按原身份对账，不能换 UUID 再提交。
- `acquired`（私有暂存）、`published`（验证后发布）、`imported`（扫描实际覆盖）是不同事实；进度 100%、move 受理或扫描完成均不能代替发布/入库证明。
- 扫描必须核对具体路径、版本和 itemIds；图片目录漫画核对聚合条目覆盖。`retry_import` 复用已发布文件，不重新获取。
- 网络调用期间不持 SQLite 事务、DB rows 或全局业务锁。
- 来源：[收集契约](../../../spec/backend/collection-contract.md)、[BT 发布门禁](../../../spec/backend/download-contract.md)、[收集研究要点 2–5](./11-27-trellis-archive-lessons/research/collection-plugin-lessons.md)。

### 3. 固定适配器、有限能力和外部编排权是现有边界

- Torznab 搜索返回短期私有结果 ID，继续走受管下载；不能暴露密钥/magnet 或借 qB 搜索接口绕过持久意图。
- 本地导入默认 register，copy 是显式选择；远程只 copy，不做删除式同步。工具、来源、目录、并发、字节和时间均有边界。
- MoviePilot 等外部套件继续拥有自己的编排权；只提供固定版本的业务 API，不开放任意 URL/method/body 或 executable/args/env 入口。
- 主服务不持 Docker socket。助手具有 daemon 实权，固定模板和 DTO 白名单不能被描述为完整沙箱。容器运行、探测就绪、API 鉴权连接分别记状态。
- 来源：[搜种契约](../../../spec/backend/torrent-search-contract.md)、[外部 API 契约](../../../spec/backend/external-collection-contract.md)、[组件契约](../../../spec/backend/collection-component-contract.md)、[收集研究要点 6–8](./11-27-trellis-archive-lessons/research/collection-plugin-lessons.md)。

### 4. Windows 配置以真实 EXE 和稳定 profile 身份为锚

- 默认数据位于真实 EXE 同级 `qiku-data/profiles/<mode>`；原位升级保留整个数据目录。已有保存的绝对路径和 instanceId 保持权威，不按 CWD 推断、不静默回退 C 盘、不自动迁移旧 profile。
- 桌面配置优先级是 **显式 CLI > 进程环境的存在性 > EXE 同级 .env > 保存 JSON > 默认值**；显式空值也会屏蔽文件值。
- .env 是启动时只读快照，不执行脚本、不展开命令、不全局 Setenv；普通 CLI/worker/Docker 不隐式读取桌面文件，修改后须完整重启。开关不会安装缺失依赖。
- 原生 BT 只连接既有外部 qB/Torznab，不安装、接管或停止它们；路径比较根据引擎路径格式，不能根据宿主 OS 想当然。
- 来源：[跨平台契约](../../../spec/backend/cross-platform-contract.md)、[Windows BT 契约](../../../spec/backend/bt-windows-contract.md)、[Windows 研究要点 4–6](./11-27-trellis-archive-lessons/research/windows-runtime-lessons.md)。

### 5. 进程退出、运行材料与发行身份分别校验

- 退出应关闭请求接纳，取消并排空 handler/worker/自有进程树，再释放 SQLite、租约与 IPC/日志；托盘消失、Serve 返回或 Job 统计归零不证明文件已释放。禁止按 exe 名通杀。
- 托管漫画/视频共用自有 worker，但能力分别探测、独立降级；disabled/external 配置不能因新包存在运行时就自动启用。
- 私有 Python、固定下载器、媒体工具、manifest、来源摘要与许可证材料构成闭包；必须真实 import/probe，不能复制开发 venv、靠运行时 pip 或单看 manifest 文件。
- 数据库 schema、desktop JSON schema、release schema 是独立版本域。开发包、本机补材料、交叉构建与正式完整离线发行/干净机验收不能互相代替。
- 来源：[跨平台契约](../../../spec/backend/cross-platform-contract.md)、[Windows 漫画契约](../../../spec/backend/jmcomic-windows-contract.md)、[Windows UAV 契约](../../../spec/backend/uav-windows-contract.md)、[Windows 研究要点 3、7–9](./11-27-trellis-archive-lessons/research/windows-runtime-lessons.md)。

### 6. 固定插件复用原有发布链，网络故障不能放宽门禁

- JMComic 选章冻结到预览/任务；恢复页须重新验证身份、大小/摘要与图片有效性；新增可选字段保持旧意图 hash 兼容，不能只看总页数相同。
- UAV 用规范单集身份、有限 HLS 与操作级 relay，下载后离线 remux/解码再走原清单。过期签名 URL 不当持久身份；搜索成功不等于下载/完整性验收。
- Fake-IP 问题按运行材料→DNS/代理→站点规则→下载分层定位；本机 DNS 定向修复不能把 `198.18/15` 加入公网许可，也不能降低 100 集展开等资源保护。
- 来源：[JMComic 契约](../../../spec/backend/jmcomic-contract.md)、[UAV 契约](../../../spec/backend/uav-contract.md)、[本机 Fake-IP 修复记录](./11-27-windows-uav-fake-ip-repair/prd.md)、[收集研究要点 9–10](./11-27-trellis-archive-lessons/research/collection-plugin-lessons.md)。

### 7. 开发工具与前端交互都要验证真实调用链

- 构建、测试、临时工具和新启动浏览器/MCP 先继承 `project_env.py` 或 `project-env.ps1`；受控缓存/临时目录限定在仓库 .runtime，不改用户级环境、不用 `go env -w`。
- PowerShell 中文脚本 BOM、5.1/7、真实 npm.ps1 参数与退出码分别验证；Vite ready/端口监听不是页面正常证据。`changeOrigin:false` 保留同源信息，不能为代理问题放宽后端校验。
- 新密码使用严格 ASCII/10–72/四类矩阵，登录和旧密码再认证保留既有 hash 兼容，不 trim；密码显隐不改值、不提交，切换凭据上下文须重新掩码。
- 清理按精确计划和所有权执行；保留运行材料闭包、验证产物以及 S03 唯一暂停草稿，不按年龄整目录删除。
- 来源：[环境入口](../../../../scripts/project_env.py)、[前端契约](../../../spec/frontend/app-contract.md)、[Windows 研究要点 1–2、10–11](./11-27-trellis-archive-lessons/research/windows-runtime-lessons.md)、[密码显隐记录（此前归档）](./11-20-password-visibility/prd.md)。

## 已落地基础、尚未完成的链路与下一步

| 主题 | 当前最小可信结论 | 后续行动与来源 |
| --- | --- | --- |
| S01 基线 | 已有隔离进程/数据、四态报告和真实断言门禁；本机记录不等于 NAS 验收 | 补真实 Linux/NAS UID/GID/ACL、挂载、Docker、大库与大媒体证据；[基线契约](../../../spec/backend/baseline-contract.md) |
| S02 账号 | 改密、会话与独占离线重置有本机实现和历史验证；保持单管理员 | 保留凭据修订/CAS、短事务与不确定结果语义；补 Linux/NAS 权限/终端/锁验收；[账号契约](../../../spec/backend/account-contract.md) |
| S03 备份 | 共用快照、schema 6 存储、受控根/无覆盖发布三个基础切片本机落地 | **执行器、调度、轮转、last-good/完成登记、恢复工具闭环尚未交付**；按 [冻结手动执行器接缝](./11-23-nas-backup-recovery/executor-manual-contract.md) 重核当前代码后续做，不能直接复制隔离草稿 |
| S04 事件 | 规划/只读研究 | 先事件存储/未读，通知默认关；不让通知反向阻塞 S03；[PRD](./11-23-nas-events-notifications/prd.md) |
| S05–S08 内容管理 | 作品、元数据、字幕诊断、批量管理仍属规划 | 保留 item 身份；人工资产独立恢复；字幕受发布边界约束；可独立的筛选/诊断切片先行；见下方逐任务索引 |
| S09–S12 自动化运维 | 自动入库、容量维护、规则解释、组件维护仍属规划 | 先试运行/只读口径/真实 evaluator/恢复点，再放开写操作；见下方逐任务索引 |
| 通用插件/飞牛 | 现有固定插件和组件不代表通用市场或任意 FPK 兼容 | 先明确宿主/版本/首批应用和真实兼容输入，再按阶段单独实施；[规划](./11-19-plugin-market-fnos-plan/prd.md) |
| 既有获取链路验收 | 已有本地/协议/依赖证据，但部分真实云盘、Docker、站点与设备门禁未完成 | 按受影响域补真实验证，不把 fixture、搜索成功、稀疏 Range 或交叉构建扩写成下载/性能/设备通过 |

数据库快照仍是 **database-only**：媒体、秘密、helper/worker 身份、组件卷和用户上传资产有各自恢复责任。发布器返回磁盘事实，不自动意味着 DB 完成登记或轮转授权。未知发布结果保留原 ID/材料对账，不能删除或换身份重试。详情见 [快照](../../../spec/backend/backup-contract.md)、[存储](../../../spec/backend/backup-storage-contract.md)、[根与发布](../../../spec/backend/backup-publish-contract.md)。

继续路线可按 S01→M1（账号/备份/事件）→M2（内容整理）→M3（自动化运维）组织，但**切片真实依赖优先于编号**，不要制造串行互等。历史 [缺口评估](./11-23-nas-feature-gap-assessment/assessment.md) 是当时的优先级依据，账号与备份的后续进展应以现有 spec 更新。完整证据导航：[NAS 路线研究](./11-27-trellis-archive-lessons/research/nas-roadmap-evidence.md)。

## 50 个任务逐项索引

以下状态保留归档前背景；每项链接为该任务 PRD 入口，结论应结合上文规范及三份研究阅读，不以状态替代验收。

| 任务（PRD） | 归档前状态 | 对后续开发有用的结论 / 未完成事项 |
| --- | --- | --- |
| [00-bootstrap-guidelines](./00-bootstrap-guidelines/prd.md) | in_progress | 已有可执行契约优先；通用指南模板仍有未填项，后续从真实代码补例，不把 bootstrap 归档当全部规范已完善。 |
| [11-17-bt-download](./11-17-bt-download/prd.md) | review | qB 传输与应用发布/入库分离；所选清单门禁在下载禁用后仍有效。 |
| [11-17-music-support](./11-17-music-support/prd.md) | review | 音乐复用既有 Item、duration、个人状态与鉴权 Range；不改写原音频。 |
| [11-17-nas-content-plan](./11-17-nas-content-plan/prd.md) | review | 复用五类媒体与单一 Go/SQLite 运行时；按真实设备里程碑验收。 |
| [11-18-bt-engine-bootstrap](./11-18-bt-engine-bootstrap/prd.md) | review | 固定助手与模板、先写意图再部署；识别实际资源归属，容器运行不等于 API 已连接。 |
| [11-18-password-policy](./11-18-password-policy/prd.md) | review | 新密码严格 ASCII/长度/四类策略；旧密码登录兼容，原字节不 trim。 |
| [11-18-project-local-runtime](./11-18-project-local-runtime/prd.md) | review | 所有本地工具先继承 project_env，将受控缓存/临时目录限定在 .runtime。 |
| [11-18-session-temp-cleanup](./11-18-session-temp-cleanup/prd.md) | review | 先精确预览，再校验所有权/活动进程/文件变化；不按目录年龄通删。 |
| [11-18-torrent-search](./11-18-torrent-search/prd.md) | review | 搜索返回短期私有结果 ID，经原持久下载意图链路提交，不泄露来源秘密。 |
| [11-18-windows-complete-exit](./11-18-windows-complete-exit/prd.md) | review | 退出以 handler、worker、自有进程树、文件句柄与数据租约真正释放为准。 |
| [11-18-windows-one-click](./11-18-windows-one-click/prd.md) | review | 共享生命周期和包路径构建；交叉编译、Windows 本机与 NAS 运行分别记证据。 |
| [11-19-collection-core](./11-19-collection-core/prd.md) | completed | 稳定 UUID/不可变输入/修订对账；acquired、published、imported 独立。 |
| [11-19-collection-file-import](./11-19-collection-file-import/prd.md) | in_progress | register 与 copy 分离，远程只 copy；真实云盘/NAS卷及故障验收仍待补。 |
| [11-19-collection-release](./11-19-collection-release/prd.md) | in_progress | 保留 UI/集成成果；真实 Docker、NAS ACL、整条部署链路仍待验收。 |
| [11-19-collection-rss-bt](./11-19-collection-rss-bt/prd.md) | completed | RSS 初次建立基线、补历史显式执行；无歧义规则与原 BT 所有权并存。 |
| [11-19-collection-web](./11-19-collection-web/prd.md) | completed | 固定工具、有限条目/预算、公网访问边界；缺能力只禁用对应适配器。 |
| [11-19-plugin-market-fnos-plan](./11-19-plugin-market-fnos-plan/prd.md) | planning | 仍属规划；固定插件/容器成功不等于通用市场或任意 FPK 兼容。 |
| [11-19-resource-collection-plan](./11-19-resource-collection-plan/prd.md) | in_progress | 复用统一获取/发布/入库架构；全平台首期验收尚未全部完成。 |
| [11-19-windows-desktop-icon](./11-19-windows-desktop-icon/prd.md) | review | 图标资源与生成器字节一致；区分 API 验证和物理托盘/Explorer 验收。 |
| [11-20-collection-component-install](./11-20-collection-component-install/prd.md) | review | 主服务不持 Docker socket；模板/挂载/卷归属受控，部署与鉴权连接独立。 |
| [11-20-collection-external-api](./11-20-collection-external-api/prd.md) | review | 固定版本/类型 API、外部套件保留编排权；fixture 不代表真实实例验收。 |
| [11-20-cross-platform-manual](./11-20-cross-platform-manual/prd.md) | completed | 跨平台工作流只手动触发，不能借构建调整恢复自动发布。 |
| [11-20-powershell-dev-encoding](./11-20-powershell-dev-encoding/prd.md) | completed | 中文脚本 BOM 与 PowerShell 5.1/7 分别验证，不只看一种解析器。 |
| [11-20-powershell-npm-arguments](./11-20-powershell-npm-arguments/prd.md) | completed | 核对真实 npm.ps1→Node/Vite argv、HTTP 资源与退出码，不只看 ready 日志。 |
| [11-20-vite-proxy-origin](./11-20-vite-proxy-origin/prd.md) | completed | changeOrigin:false 保留 Host/Origin；不为开发代理放宽后端同源门禁。 |
| [11-23-nas-account-lifecycle](./11-23-nas-account-lifecycle/prd.md) | completed | 本机改密/会话/离线重置已交付；凭据修订与 CAS 防竞态，Linux/NAS 待验。 |
| [11-23-nas-backup-recovery](./11-23-nas-backup-recovery/prd.md) | in_progress | 仅三个备份基础切片本机落地；执行器/调度/轮转/恢复闭环尚未完成。 |
| [11-23-nas-component-maintenance](./11-23-nas-component-maintenance/prd.md) | planning | S12 规划：先状态/单组件停启，再受控升级；维护动作匹配恢复点。 |
| [11-23-nas-events-notifications](./11-23-nas-events-notifications/prd.md) | planning | S04 规划/只读研究：先事件存储，通知默认关；不反向阻塞备份主体。 |
| [11-23-nas-evolution-plan](./11-23-nas-evolution-plan/prd.md) | in_progress | 保留 S01→M1/M2/M3 出口和切片依赖；可独立能力不必串行互等。 |
| [11-23-nas-external-auto-import](./11-23-nas-external-auto-import/prd.md) | planning | S09 规划：可信完成根、试运行、首期只 register，并验证真实 itemIds。 |
| [11-23-nas-feature-gap-assessment](./11-23-nas-feature-gap-assessment/prd.md) | completed | 历史优先级依据；S02/S03 后续已变化，不能把 11-23 评估当完整现状。 |
| [11-23-nas-library-management](./11-23-nas-library-management/prd.md) | planning | S08 规划：现有字段筛选可先行；重关联保留身份，批量操作保持原子边界。 |
| [11-23-nas-local-metadata](./11-23-nas-local-metadata/prd.md) | planning | S06 规划：预览/来源/人工值/锁定；用户上传资产需独立恢复责任。 |
| [11-23-nas-rule-guidance](./11-23-nas-rule-guidance/prd.md) | planning | S11 规划：解释复用真实 evaluator；未知预期集数不显示假 100%。 |
| [11-23-nas-storage-maintenance](./11-23-nas-storage-maintenance/prd.md) | planning | S10 规划：先只读容量口径，再精确归档清理；索引减少不等于媒体空间释放。 |
| [11-23-nas-subtitles-diagnostics](./11-23-nas-subtitles-diagnostics/prd.md) | planning | S07 规划：有界诊断、授权字幕、播放器资源回收；同目录不等于 sidecar 已发布。 |
| [11-23-nas-validation-baseline](./11-23-nas-validation-baseline/prd.md) | completed | 隔离实例/进程所有权/断言与四态报告已落地；真实 NAS 门禁仍待验。 |
| [11-23-nas-work-catalog](./11-23-nas-work-catalog/prd.md) | planning | S05 规划：保留 item_id 和文件视图；作品/季集/多版本显式关联，歧义不静默合并。 |
| [11-25-jmcomic-advanced](./11-25-jmcomic-advanced/prd.md) | review | 冻结选章；可信页恢复和分章分布核对；新增可选字段保持历史意图摘要兼容。 |
| [11-25-jmcomic-plugin](./11-25-jmcomic-plugin/prd.md) | review | 固定可选适配器复用发布和阅读器；真实依赖 fixture 不等于真实站点/NAS 验收。 |
| [11-25-uav-downloader-plugin](./11-25-uav-downloader-plugin/prd.md) | review | 规范单集身份、有限 HLS/relay、离线解码后发布；搜索成功不等于下载验收。 |
| [11-26-bt-windows-native](./11-26-bt-windows-native/prd.md) | review | 只连接外部引擎；按引擎路径格式比较，不按宿主 OS 推断；真实 peers 待验。 |
| [11-26-jmcomic-windows-native](./11-26-jmcomic-windows-native/prd.md) | review | 私有 Python 材料闭包、真实探测和自有 worker；原 disabled/external 不自动启用。 |
| [11-26-runtime-slim](./11-26-runtime-slim/prd.md) | completed | 保留运行材料/许可证/验证产物和 S03 唯一暂停草稿；摘要保留不等于重新回归。 |
| [11-26-uav-windows-native](./11-26-uav-windows-native/prd.md) | review | 多插件独立降级、UAV 材料来源身份及 release schema 3；完整发行/干净机另验。 |
| [11-26-windows-dotenv](./11-26-windows-dotenv/prd.md) | review | 真实 EXE 同级只读启动快照；CLI > 进程变量存在性 > .env > 保存值 > 默认。 |
| [11-26-windows-portable-profile](./11-26-windows-portable-profile/prd.md) | review | 默认 EXE 同级 qiku-data；保留 profile/instanceId，不静默回退或改写已有绝对路径。 |
| [11-27-windows-uav-fake-ip-repair](./11-27-windows-uav-fake-ip-repair/prd.md) | review | 修复限本机 DNS 定向配置；公网校验仍拒绝 Fake-IP，搜索/下载分别验收。 |
| [11-27-windows-uav-local-repair](./11-27-windows-uav-local-repair/prd.md) | review | 先核对运行材料再分层排查；本机补齐不等于正式离线发行或真实下载成功。 |


## 归档后如何使用

1. 先由本页定位主题与任务，再读当前 spec 和原始验证记录；遇到旧引用路径，用本页的归档入口定位，不修改历史证据来伪造当时结果。
2. 持续有效的实现契约留在相应 spec，月度总结只作带证据的导航；具体新实现按 [归档复盘清单](../../../spec/guides/archive-retrospective-guide.md) 记录增量经验。
3. 未来每次归档先写任务 `summary.md`；批量归档可写覆盖全部任务的汇总，并在归档后校验可发现性、链接和资料完整性。
