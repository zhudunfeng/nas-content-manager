# Research: 基础架构、资源收集与固定插件的归档要点

- Query: 本轮归档任务中，哪些决策、边界和踩坑能直接指导当前项目后续开发？
- Scope: internal，仅阅读现有归档 PRD/研究、可执行契约与当前源码。
- Date: 2026-11-27。
- 证据分级：**源码核对**表示本轮阅读了实现；**当前规范**表示当前可执行契约的要求；**历史验证**表示任务记录中的已执行结果；**未验收/规划**不得写成已交付。

## Findings

### 1. 保持单一媒体运行时与五种内容类型

- 触发：新增资源来源、音乐/视频读取、扫描、缓存或数据库字段。
- 要点：Go HTTP + SQLite 仍是媒体事实来源；内容类型保持 `video/image/comic/novel/music`。下载、来源和插件是工作流，不能另造内容类型或播放器。
- 边界：当前数据库 head 为 **6**；归档早期 PRD 提到的 v1/v2/v3 是当时基线，不能拿来覆盖当前版本。增量迁移必须保留媒体 ID、用户收藏/标签/进度。
- 原文件只读，缓存是衍生数据；大媒体沿用鉴权后的 `os.OpenRoot` 与 `http.ServeContent`，不能整片读入内存或加破坏 Range 的全局压缩/短写超时。
- 证据：**源码核对** `internal/app/media.go:14`、`:35`、`:73`；**当前规范** `.trellis/spec/backend/runtime-contract.md`、`.trellis/spec/frontend/app-contract.md`。

### 2. BT 下载器只负责传输，发布门禁决定能否读取

- 触发：下载完成回调、扫描、封面、直接文件读取或下载模块禁用。
- 要点：qB 负责 BT 传输/校验/做种；应用持久化自己的意图、选择清单和版本。必须先确认最终位置、所选文件、普通文件/链接边界及 size/mtime，才可标记 ready。
- 踩坑：进度 100%、目录出现或 move API 受理均不是发布证据。冻结/改写/未选文件不能通过另一条扫描或封面路径绕过；即使引擎离线或模块关闭，历史根与清单门禁仍需保留。
- 删除任务使用 `deleteFiles=false`，保留已发布清单与原文件，不接管外部相同 hash 任务。
- 证据：**源码核对** `internal/app/download_gate.go:402`、`internal/app/media.go:19`；**当前规范** `.trellis/spec/backend/download-contract.md`。

### 3. 搜种返回私有令牌，始终进入原有受管下载链路

- 触发：扩展索引器、搜索结果或从搜索创建下载。
- 要点：固定 Torznab 适配 Prowlarr/Jackett，浏览器只拿短期随机结果 ID 与有界展示字段，不拿密钥、magnet、GUID 或来源 URL。先全量脱敏再截短标题，防止截断边界泄漏密钥前缀。
- 踩坑：qB 原生 `search/downloadTorrent` 会直接建任务，绕过应用持久意图与选文件，不可当成种子解析器。解析下载仅允许固定来源同源代理路径，拒绝管理端点和 HTTP 重定向。
- 搜索与引擎可用性独立；部分来源失败保留成功结果，全部失败给错误，不能伪装成空列表。
- 证据：**当前规范** `.trellis/spec/backend/torrent-search-contract.md`；历史任务 `archive/2026-11/11-18-torrent-search/prd.md`（以下任务路径均在 `.trellis/tasks/` 下）。

### 4. 跨网络副作用先持久化，未知结果沿原身份对账

- 触发：创建获取任务、qB add、worker 发布、外部 API 写入、助手部署或浏览器重试。
- 要点：同一意图冻结 UUID、请求体、来源修订和不可变预览；先写意图再发网络请求。超时/断线不生成新 UUID；先查询原编号，恢复不确定状态。
- 边界：重复原 UUID 返回原任务，不同意图使用原 UUID 返回 409；过期预览不能让已受理请求失去可恢复性。不要跨网络持有 SQLite 事务、DB rows 或全局业务锁。
- 批量至多十个独立任务，逐项保留 accepted/failed/unknown，已成功项不重复提交。
- 证据：**当前规范** `.trellis/spec/backend/collection-contract.md`、`external-collection-contract.md`、`engine-bootstrap-contract.md`、`jmcomic-contract.md`。

### 5. 获取、发布与入库是三个独立事实

- 触发：显示任务完成、重试导入、删除目标媒体库或重启恢复。
- 要点：worker 的 `acquired` 仍是私有暂存；应用授权发布后，需独立复核清单，再让扫描实际覆盖所有目标路径、版本与条目 ID 才能显示 imported。
- 踩坑：一个扫描完成可能已越过新目录，不能当作入库成功；图片目录漫画需验证聚合条目覆盖。`retry_import` 只复用已发布文件，不重新下载/复制。媒体库删除不自动按历史意图重建。
- 当前实现先 `collectionValidateFiles`，再计算 `collectionImportTargets` 与 `collectionCovered`，未覆盖则等待/安排有界后续扫描。
- 证据：**源码核对** `internal/app/collection_import.go:163`；**当前规范** `.trellis/spec/backend/collection-contract.md`。

### 6. 适配器能力必须真实，执行端不能变成任意命令入口

- 触发：新增 web/remote/local/RSS 来源或固定插件。
- 要点：只开放固定类型协议、受控工具、管理员授权根与 named rclone remote；本地默认只读登记，复制是明确选择，远程只 copy，不做删除式同步。缺工具只禁用对应能力，基础浏览和已发布内容继续可用。
- web 是单一有限条目；限制并发/超时/字节/文件数，DNS、连接、重定向和子资源都执行公网边界，不接受任意 executable/args/env、Cookie 上传或 DRM。
- RSS 初次建立基线，补历史显式操作；只有用户明确开启且匹配无歧义才自动获取；来源修改会失效旧候选修订，已受理输入保持不可变。
- **未验收**：真实 NAS 云盘、跨卷/网络卷、磁盘不足及部分 I/O 故障；本机 rclone alias 测试不能代替这些。
- 证据：**当前规范** `.trellis/spec/backend/collection-contract.md`；**历史验证/未验收** `archive/2026-11/11-19-collection-file-import/prd.md`、`11-19-resource-collection-plan/prd.md`。

### 7. 外部自动化套件保持唯一编排权，能力按精确版本开放

- 触发：MoviePilot/AutoBangumi/Sonarr/Radarr 的连接、列表、写操作、整理或刷新。
- 要点：使用固定业务 API 与类型 DTO，不做任意 URL/method/body 代理，不接管 qB。秘密和私有连接放独立私密文件，普通 DTO、日志和 SQLite 备份不能携带它们。
- 版本边界：MoviePilot 六阶段限定 **3.2.8**；Sonarr 4、Radarr 7 的 `/api/v3` 与 AutoBangumi 4 各有不同语义，不能按“主版本看起来相同”推断写能力。
- 踩坑：外部下载回执不等于本地入库；`checkedExternally` 只记录 `unconfirmed`，不能写成成功；MoviePilot 索引刷新不代表 Emby/Plex 扫盘。
- 证据：**当前规范** `.trellis/spec/backend/external-collection-contract.md`；**历史验证** `archive/2026-11/11-20-collection-external-api/prd.md`（四 HTTP fixture 83 断言，不是真实用户实例验收）。

### 8. 组件助手遵循固定模板与实际资源归属

- 触发：部署/启动 MoviePilot 等组件、扩展助手能力或显示连接状态。
- 要点：主应用不持有 Docker socket；助手具有实际 daemon 权限，DTO 白名单并不构成 daemon 沙箱。固定镜像/端口/挂载/权限/卷标签和归属都需核对，不凭同名容器或标签接管。
- GET/打开页面不得初始化存储或安装。部署前 notice 必须主动确认；容器运行、匿名探测就绪、API 已鉴权连接是独立状态。
- 踩坑：远程用户浏览器的 localhost 不是 NAS；pull HTTP200 不等于镜像成功。超时预算要覆盖真实助手时限，层进度无真实分母时不造总百分比。
- 证据：**当前规范** `.trellis/spec/backend/engine-bootstrap-contract.md`、`collection-component-contract.md`；**未验收** `archive/2026-11/11-19-collection-release/prd.md`（真实 Docker daemon/NAS ACL/完整部署待验证）。

### 9. JMComic 扩展复用原链路并保留历史意图摘要

- 触发：增加漫画搜索过滤、选章、进度、页恢复或新持久字段。
- 要点：这是固定可选适配器，不是通用插件市场。编号/枚举有界；选章绑定不可变预览、执行与完整性，不能在任务创建时再改变。新增字段 `omitempty`，避免改变旧请求序列化与 hash。
- 进度只记已验证页/字节，复用页面也计入 bytes，不冒充网络速度；显式失败重试只能复用属于原操作且经身份/size/SHA256/图片校验的页面。
- checkpoint 放 `private-state/jmcomic/<opid>/`，不混入 staging 发布清单或根级操作 JSON；总页数相同也要检查分章分布漂移。
- 证据：**当前规范** `.trellis/spec/backend/jmcomic-contract.md`；**历史验证** `archive/2026-11/11-25-jmcomic-plugin/prd.md`、`11-25-jmcomic-advanced/prd.md`。真实站点/容器/NAS仍待验收。

### 10. UAV 用有限媒体身份与真实解码把住发布边界

- 触发：五站搜索、剧集识别、HLS、CDN relay、来源候补或下载结果校验。
- 要点：固定五站与版本来源；huangguo 身份是单集，不是整个 series。预览/任务绑定站点与规范 ID，不持久化过期签名 URL；继承原 UUID/版本/发布/入库协议。
- 有限 HLS、资源白名单和操作级 relay 限定网络访问；Referer/CDN 访问值不全局传播。下载后本地离线 remux、完整解码并检查有限 HLS 时长下界，再走原清单发布。
- 踩坑：能解码不代表原始内容语义完整，direct MP4 无可信原始时长；不宣称严格 AES padding 或内容真实性。候补仅在允许的传输/内容失败后推进，不重置超时/字节预算、不拼接不同候补残片。
- 证据：**当前规范** `.trellis/spec/backend/uav-contract.md`；任务 `archive/2026-11/11-25-uav-downloader-plugin/prd.md`。本轮未访问真实站点。

### 11. 前端安全体验也有可复用的状态边界

- 触发：密码输入、账号改密、外部凭据、收集弹窗、进度和错误显示。
- 要点：新密码的严格 ASCII 10–72 位/四类要求只用于新密码；登录/旧密码验证保持既有 hash 兼容，不 trim。后端为权威，前后端用相同矩阵。
- `PasswordInput` 独立显隐，不改值、不发请求，按钮 `type=button`；原值已空时切换服务需组件 key 重建来保证重新掩码，不能只依赖赋值 `''` 的 watcher。
- 保持一个原生 modal，先卸载旧框并 `nextTick` 再打开新框；错误、空结果、不可用、未知提交分别表达，不能用成功列表清掉另一独立请求的错误。
- 证据：**源码核对** `internal/app/auth.go:27`、`web/src/components/PasswordInput.vue:20`、`:25`、`:30`；**当前规范** `.trellis/spec/frontend/app-contract.md`；**历史验证** `archive/2026-11/11-20-password-visibility/prd.md`（先前已归档，不属于本次 50 个）。

## Files Found / 任务覆盖清单

所有任务目录前缀为 `.trellis/tasks/archive/2026-11/`，以 `prd.md` 为定位入口；这里只列本研究负责的主题，Windows/运行环境及 NAS 专题由其他研究补齐。

| 任务目录 | 可复用结论 / 本研究覆盖 |
| --- | --- |
| `00-bootstrap-guidelines` | 初始化 PRD 仍有未勾项；真实契约优先，勿把尚未填充的规范模板当完成成果 |
| `11-17-nas-content-plan` | 单一媒体运行时、只读源、持久个人状态、分阶段真实验收（要点1） |
| `11-17-music-support` | 音乐复用 Item/duration、原文件/标签只读、浏览器格式能力（要点1） |
| `11-17-bt-download` | 所选清单与发布门禁、传输/入库分离（要点2/5） |
| `11-18-password-policy` | 新密码严格策略不破坏旧账号登录（要点11） |
| `11-18-torrent-search` | 有界 Torznab、私有结果 ID、原受管下载链路（要点3） |
| `11-18-bt-engine-bootstrap` | 固定助手、持久意图、真实归属/就绪（要点4/8） |
| `11-19-plugin-market-fnos-plan` | 仅规划；固定适配器/外部应用不等于通用市场或任意 FPK 兼容 |
| `11-19-resource-collection-plan` | 统一多来源首期边界；真实多平台首期验收尚未全部勾选（要点4–8） |
| `11-19-collection-core` | 稳定协议、幂等、共享根探测、发布门禁（要点4–6） |
| `11-19-collection-rss-bt` | RSS 基线/显式补查、无歧义规则、原 BT 所有权（要点2/6） |
| `11-19-collection-web` | 固定工具、公网策略、单项/资源限额（要点6） |
| `11-19-collection-file-import` | register/copy 分离，实际覆盖，NAS 故障/云盘待验收（要点5/6） |
| `11-19-collection-release` | UI 与集成已记录，真实 NAS/Docker 部署仍有缺口（要点8） |
| `11-20-collection-external-api` | 固定版本业务适配、写意图/对账、唯一编排方（要点7） |
| `11-20-collection-component-install` | 固定模板、存储授权、部署与 API 连接状态分离（要点8） |
| `11-25-jmcomic-plugin` | 固定受控桥接、完整漫画发布、既有阅读器（要点9） |
| `11-25-jmcomic-advanced` | 选章冻结、可信页恢复、历史摘要兼容（要点9） |
| `11-25-uav-downloader-plugin` | 规范单集 ID、有限 HLS/relay、离线解码边界（要点10） |
| `11-20-password-visibility` | 先前已归档的补充资料；共享显隐组件与重掩码（要点11） |

## Code Patterns / Related Specs

- 本轮 CodeGraph 查询定位 `checkDownloadPath`、`importCollectionTask`、`validNewPassword`，随后直接阅读上述目标源码；查询前通过 `scripts/project-env.ps1` 继承项目环境。
- 后续实现以 `.trellis/spec/backend/index.md`、`.trellis/spec/frontend/index.md` 及各要点指向的当前契约为准；本研究是导航和复盘，不建立第二份 API/字段事实来源。

## External References

- 本轮没有联网或重新核验上游。历史契约记录 JMComic 2.7.12、UAV-Downloader 3.8.4、MoviePilot 3.2.8、AutoBangumi 4、Sonarr 4/Radarr 7 与 rclone 1.76.0；版本变化须另做源码/协议和真实环境核验。

## Caveats / Not Found

- 归档 `completed` 是生命周期元数据，不代表所有 checkbox、真实来源、Docker/NAS 或性能验收通过；本轮只整理已有证据，没有重跑历史测试。
- 历史 PRD 的旧数据库版本和“后续范围”可能已被当前规范/后续任务扩展，引用时需看时间和当前 head；通用插件市场与任意 FPK 运行兼容仍是规划。
- 初始化任务的规范填充 checkbox 仍未勾选，backend/frontend 索引也保留 `To fill` 主题模板；应依据已有具体 `*-contract.md`，不能把全部模板宣称已完善。
- 现有部分契约正文还引用归档前 `.trellis/tasks/<name>/` 的历史路径；资料已移动，查找时使用本文件列出的 archive 目录，后续文档维护应同步修复引用。
