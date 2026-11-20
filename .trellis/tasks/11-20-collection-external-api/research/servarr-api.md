# Research: Sonarr / Radarr 深度 API 接入

- Query: 核实 Sonarr、Radarr 官方接口、版本边界、最小创建/监控/搜索动作，以及写入超时对账方式。
- Scope: mixed；官方源码/OpenAPI 为协议证据，当前 NAS 仓库为集成约束；未连接真实用户实例。
- Date: 2026-11-20

## Findings

**实施主权威文件统一为同目录 `sonarr-radarr-api.md`**：Sonarr 固定 `v4.0.20.3014` / commit `cab419ade8ac7fcab5bf80394ee492abd35d5f5a`，Radarr 固定 `v7.0.3.10918` / commit `af772b051ac27a0615c7f8b7f78622b6dc183be0`。本报告是补充比对和 Sonarr v5 风险说明；下文 Sonarr `v4.0.17.2952` 是已读取的较旧协议参考，不能称为最新稳定版，也不替代主权威文件的实施 pin。两个主权威 commit 的 OpenAPI 已直接读取确认。本 agent 仅做研究，不生产代码。

### 1. 实施结论与简单易用原则

建议首批验收固定支持 **Sonarr 产品 v4 + API v3**、**Radarr 产品 v7 + API v3**，fixture 使用上段主权威 commit。Sonarr v5 必须分开适配：产品 `5.0.0.2997` 是预发布，官方 API 为 `/api/v5`；它的新增剧集还强制要求有效 `language` 和非空 `seasonType`。不能仅替换路径后宣称创建兼容。证据：[S1–S8]。

用户只需“选择服务 → 地址/API Key → 测试连接”。日常界面提供作品列表、监控开关、单作品搜索、队列/历史和“打开原服务”。版本自动探测，根目录/质量配置从远端选择；单个选项可以预选，多个选项要求确认。根目录、质量、语言属于新增作品时的配置，不应全部阻塞首次连接。高级参数逐步展开。下列默认值属于本项目建议，并非上游服务保证。

两个状态必须分开显示：**外部服务下载/整理状态**和**本系统扫描入库状态**。远端 `outputPath` 不能直接成为 NAS 任意文件读取路径；继续通过原有授权根目录、相对路径、预览和独立完成目录导入流程。

### 2. 产品版本与 API 版本

| 服务证据标签 | 产品发布信息 | 使用的 API 前缀 | 推荐支持策略 |
| --- | --- | --- | --- |
| Sonarr `v4.0.17.2952` | 较旧标签，仅用于本报告协议比对；实施改用主权威文件 `v4.0.20.3014` | `/api/v3`；OpenAPI info.version 为 `3.0.0` | 不能据此保证全部旧 v3 产品功能 |
| Sonarr `v5.0.0.2997` | 2026-11-17，预发布，release HTML commit `51a2412af235197ce15d7d4a17c053a8e2c26609` | `/api/v5`；OpenAPI info.version 为 `5.0.0` | 独立能力矩阵/fixture；本轮建议全部写能力关闭，保留外部入口 |
| Radarr `v7.0.3.10918` | 2026-11-16，实施 commit `af772b051ac27a0615c7f8b7f78622b6dc183be0` | `/api/v3`；OpenAPI info.version 为 `3.0.0` | 首批完整支持本研究最小集合 |

Sonarr 当前公共文档已经切至 v5，因此直接从未固定版本的文档复制字段会误用于 v4。稳定证据使用标签或 commit URL。GitHub REST API 本次请求遇到匿名速率限制，未使用失败请求作提交 pin；主权威 commit 的 OpenAPI、标签源码和 Sonarr5 release HTML 已直接读取。[S1–S6]

**固定探测流程建议**：

1. 服务选择为 Sonarr 或 Radarr，验证 base URL；允许已配置反向代理的路径前缀，如 `/sonarr`，其后拼固定 API 路径。禁止调用者传入任意 API 路径。
2. 向 `{base}/api/v3/system/status` 发送认证 GET。读取 `appName`、`version`，核对所选服务与产品版本；不要把任意 HTTP 200 或 HTML 登录页当作成功。
3. Sonarr 若为 v5，改以固定 `/api/v5/system/status` 核验；若 v3 返回 404，可以且仅可以再尝试该 v5 固定探测。401/403、网络超时、TLS 错误不能当成版本不匹配继续尝试写接口。
4. 探测只读健康与选项，分别记录连接、健康、队列和目录配置错误。`health=[]` 表示无当前健康告警，不是服务离线。某列表失败不能伪造空列表并称为成功。
5. 不在 API 前缀之间自动重试已经发出的写请求。服务版本不在已验证范围时先禁用写入。

### 3. 认证、固定端点与响应

所有请求使用 `X-Api-Key: <secret>`；JSON 写请求使用 `Content-Type: application/json`，读取使用 `Accept: application/json`。官方 OpenAPI 也提供 query `apikey`，本项目应禁用该形式，避免密钥进入 URL、日志和历史。API Key 从服务原有设置中获取，不由 NAS 代用户修改。认证定义见各固定 OpenAPI 的 `components.securitySchemes`。[S4–S6]

以下 `P` 为已核实的 API 前缀。仅实现表内方法，不增加任意请求代理。`N` 为正整数远端实体 ID。

| 功能 | Sonarr | Radarr | 最小映射/限制 |
| --- | --- | --- | --- |
| 版本 | `GET P/system/status` | 同左 | 对象：`appName,version,instanceName,urlBase`；不公开 `appData/startupPath` 等宿主信息 |
| 健康 | `GET P/health` | 同左 | 数组：`source,type,message`；错误消息需脱敏 |
| 已管理作品 | `GET P/series` | `GET P/movie` | 普通数组；`id,title,tvdbId|tmdbId,monitored,qualityProfileId`；不是分页包装 |
| 单作品 | `GET P/series/N` | `GET P/movie/N` | 用于刷新和监控结果核验 |
| 稳定身份对账 | `GET P/series?tvdbId=ID` | `GET P/movie?tmdbId=ID` | 返回数组；匹配一个再核对稳定身份与远端 ID |
| 队列 | `GET P/queue?page=1&pageSize=50` | 同左 | 包装对象：`page,pageSize,totalRecords,records` |
| 历史 | `GET P/history?page=1&pageSize=50` | 同左 | 同上；只读，不能把历史条目当成当前任务控制入口 |
| 远端根目录 | `GET P/rootfolder` | 同左 | 数组：`id,path,accessible`；用于远端新增时选择，不能作为 NAS 本地根授权 |
| 质量配置 | `GET P/qualityprofile` | 同左 | 数组：`id,name`；只读选择，不写全局质量配置 |
| 元数据查找 | `GET P/series/lookup?term=文本` | `GET P/movie/lookup?term=文本` | 普通数组；固定 term 编码，用户选择确切作品后新增 |
| 新增 | `POST P/series` | `POST P/movie` | 严格服务专用 DTO，见下节 |
| 监控开关 | `PUT P/series/editor` | `PUT P/movie/editor` | 每次 ID 数组限定一个，body 只含 ID 和 monitored |
| 单作品搜索 | `POST P/command` | 同左 | 固定命令名和目标，见下节 |
| 搜索命令观察 | `GET P/command/N` | 同左 | `id,name,status,result`；有确定远端 command ID 才查询 |

端点、分页形状和字段证据：[S4–S6]；作品控制器的稳定 ID 查询与新增约束：[S7–S9]；lookup：[S14–S16]。不把未核实的查询字符串形式（例如 `tvdb:` 文本搜索语法）作为对账依赖。

队列最少映射 `id,title,seriesId|movieId,downloadId,status,trackedDownloadStatus,trackedDownloadState,size,estimatedCompletionTime,errorMessage`。可选字段缺失和 null 必须兼容；可公开信息应截断并脱敏，远端任意路径/错误正文不原样回传。历史最少映射 `id,seriesId|movieId,sourceTitle,date,eventType,downloadId`。两类列表的原始字段不能用于接管下载器。[S4–S6]

### 4. 新增作品：服务专用最小请求

#### Sonarr 产品 v4 / API v3

控制器明确校验非空 `title`、正数 `tvdbId`、现存 `qualityProfileId`，以及有效 `path` 或 `rootFolderPath`。本项目应只允许从只读根目录选项选择 `rootFolderPath`，不接受任意 path。新增服务会按 TVDB ID 重新获取元数据，未提供 seasons 时可使用 Skyhook 季度；为了意图清晰，建议基于已选择 lookup 结果生成严格 DTO。[S7,S17]

```json
{
  "title": "用户已选择的 lookup 标题",
  "tvdbId": 12345,
  "qualityProfileId": 1,
  "rootFolderPath": "/tv",
  "monitored": true,
  "seasonFolder": true,
  "seriesType": "standard",
  "addOptions": {
    "monitor": "all",
    "searchForMissingEpisodes": false,
    "searchForCutoffUnmetEpisodes": false
  }
}
```

以上 ID/路径仅示意，必须使用连接实例实际返回的选项；`seriesType` 可选 `standard|daily|anime`，优先保留 lookup 对应类型。该示例关闭附带搜索，用户再点显式“搜索此剧集”，避免创建超时造成隐含重复搜索。不要顺带建立第二条内部自动订阅。

**Sonarr v4 OpenAPI 的 POST response 标为 200，但固定版本控制器实际返回 Created**；实现应接受符合协议的成功 2xx 后严格验证 JSON `id>0` 与 `tvdbId`，不能仅接受一种成功状态码。[S4,S7]

#### Sonarr 产品 v5 / API v5

上述请求还不完整。`SeriesController.cs:106–115` 要求 `language` 非 null 且能映射 ISO 语言，`seasonType` 非空。v5 lookup 可省略 language query，服务使用自身 PreferredMetadataLanguage；新建必须从经过验证的 v5 lookup 元数据保留有效 language/seasonType，而非把 UI 默认值硬编码为某语言。v5 首次发布尚在预发布阶段，若没有独立的有效/无效语言、季度类型 fixture，禁用 create 并说明原因。[S8,S16]

#### Radarr 产品 v7 / API v3

控制器校验 `tmdbId`、存在的质量配置、有效路径/根目录；新增服务会重新获取 TMDB 元数据。推荐仍携带 lookup 标题以便审计，而非接收随意拼接的标题和任意 JSON。[S9,S18]

```json
{
  "title": "用户已选择的 lookup 标题",
  "tmdbId": 12345,
  "qualityProfileId": 1,
  "rootFolderPath": "/movies",
  "monitored": true,
  "minimumAvailability": "released",
  "addOptions": {
    "monitor": "movieOnly",
    "searchForMovie": false
  }
}
```

`minimumAvailability=released` 是本项目推荐的首期显式值，不是上游强制默认；高级场景可以支持经过枚举校验的其他值，但不能允许未知枚举。`monitor=movieOnly` 避免隐式监控整个系列。成功返回后持久化远端 `id` 与 `tmdbId`。[S6,S9,S18]

### 5. 监控和显式搜索

**监控只改一个字段**，使用上游 editor 控制器的可空字段语义，避免 GET 全量作品后 PUT 回写覆盖其他客户端的更改：[S10,S11]

```http
PUT /api/v3/series/editor
{"seriesIds":[17],"monitored":false}

PUT /api/v3/movie/editor
{"movieIds":[29],"monitored":true}
```

Sonarr v5 对应 `/api/v5/series/editor`，同样以 `Monitored.HasValue` 判断仅更改目标字段。[S19] 不传 `rootFolderPath/tags/moveFiles/deleteFiles`。开关表示“监控作品”，不能标成“暂停当前下载”；它不等于暂停或删除已经提交的下载。成功后 GET 单作品核验 desired state；不把空响应或目标已不存在当作成功。

**搜索固定命令**：[S12,S13,S20,S21]

```http
POST /api/v3/command
{"name":"SeriesSearch","seriesId":17}

POST /api/v3/command
{"name":"MoviesSearch","movieIds":[29]}
```

Sonarr v5 前缀改为 `/api/v5`，但必须另测该版本。严格禁止未指定目标的命令、任意 command name/body、全局 missing search，以及把 `seriesIds` 误用于 `SeriesSearch`。该动作可能产生下载，应由用户明确触发并显示目标。持久化响应 command `id` 后才轮询该命令；`queued/started` 表示外部正在执行，`completed` 是命令完成，不代表媒体已下载或本系统已入库。未知状态原样归一为“未知/待核实”，不能当成功。

### 6. 有界读取、错误和不确定结果

以下为建议的 NAS 客户端预算，并非服务端承诺：单次连接/普通请求超时 10 秒，lookup 可 20 秒；单次响应上限 4 MiB，队列/历史每页 50、最大 100；UI 每次只请求一页。无上游分页的作品/选项列表要限制响应字节及解码实体数，超过限制明确提示“列表过大”，不能伪装完整列表或无限自动拉取。所有请求有 context 取消；不在失败时无穷重试。网络预算和中文解释应集中在具类型常量/配置中。

安全 HTTP 沿用来源私网授权、DNS/地址验证、禁重定向或逐跳同等检查、TLS 校验及响应字节上限。API Key 不能发送到其他 origin。禁止因默认允许 NAS 私网而完全取消外部来源授权。

错误应按 HTTP 类别映射：401/403 为认证/授权错误；404 为路径、版本或目标不存在；400 类校验错误只返回经过提炼的字段与中文原因；429/5xx/非 JSON 为外部暂不可用。不要把上游正文、请求 URL、反向代理 HTML、exception 或头部整体写入公开错误和日志。不能假设所有错误都符合某一个 JSON schema。

上游未发现本研究最小写接口支持客户端 idempotency key 的正式契约。使用本地 `sourceId+revision+requestId+action+canonicalInput` 持久化唯一意图，在联网前记录 accepted/sending；不能仅用内存防重复。

| 动作 | 提交前 | 响应丢失后的安全处理 |
| --- | --- | --- |
| 新增 series/movie | 用 `tvdbId/tmdbId` 查询；已存在时不再 POST；保存选中身份、远端根目录和质量等意图 | 查询相同稳定身份；唯一匹配则核验远端实体和创建意图，保存 remoteId；无匹配仍可能是前次正在执行，应保持 uncertain，不能立即重发；多匹配为冲突 |
| 监控 | 保存 remoteId 与期望 bool，确认 ID 对应稳定身份 | GET 单作品；与期望一致可确认达成，不一致先报告仍待核实；再次 set 同值只能通过明确可重试流程，不偷偷切成 toggle |
| 搜索 | 确认一个 remoteId，存固定命令目标，发送一次 | 有 commandId 就 GET；无 commandId 不能依据短期 command 列表未找到就宣称没执行，更不能自动重发。保留“结果待核实”，提示打开服务查看 |

`GET P/command` 虽存在，但没有本地 requestId 的可靠跨重启关联保证；不是“搜索 exactly-once”解决方案。服务重启、命令清理和其他客户端同目标搜索都会让相似命令匹配有歧义。

### 7. HTTP fixture 与最小支持矩阵

建议按产品标签分别启动隔离 HTTP fixture，而不对真实外部服务写入。至少断言：

1. 每个请求固定 method/path；API Key 只在 header；支持 base path；401/403 不继续探测写接口；返回 HTML 的 200 不能通过连接测试。
2. Sonarr4 `/api/v3`、Sonarr5 `/api/v5`、Radarr7 `/api/v3` 分别核对 appName/version；未知主版本写入关闭。v5 不接收 v4 创建体；缺 language/seasonType 提示明确。
3. 作品列表为数组；queue/history 为分页对象；空数组稳定输出 `[]`。分页参数受限；超大响应、超多项目、截断 JSON、null 可选字段、未知状态不会 panic 或误报成功。
4. create 先检查稳定身份；根目录/质量必须来自该实例；发送固定 body 且附带搜索为 false。接受受支持成功码并核对返回实体 ID 与稳定身份。
5. monitor body 只有单目标 IDs+monitored，不发送其他字段；false 必须作为 JSON false 发送，不能被 omitempty 丢失。成功后读回核验。
6. 搜索分别为 SeriesSearch/seriesId 和 MoviesSearch/movieIds；目标为空、多个、负数或 command name 非白名单直接拒绝，不发送远端请求。
7. 写成功但响应断开、超时、进程重启后，重复相同 requestId 不出现第二个 POST。创建按身份对账；搜索无 receipt 维持 uncertain。不同 requestId 也不能绕过同一未决意图保护。
8. 上游路径、令牌、私有 URL、错误 exception、重定向 Location 不泄漏；完成队列不会直接触发未经授权的文件读取或标为已入库。

| 能力 | Sonarr4 | Radarr7 | Sonarr5 |
| --- | --- | --- | --- |
| 连接/版本、健康、作品、队列/历史、root/profile | 可按固定协议实现 | 可按固定协议实现 | 协议已核实，须单独 fixture 后开启 |
| lookup/create | 支持本研究约束 | 支持本研究约束 | create 需 language+seasonType 独立实现；未实现禁用 |
| 单作品监控 | editor 单目标 | editor 单目标 | v5 editor 单目标，须独立 fixture |
| 单作品显式搜索 | SeriesSearch | MoviesSearch | 路径/版本单独 fixture；无 commandId 超时不重发 |
| 文件删除、下载器接管、全局配置/搜索 | 不提供 | 不提供 | 不提供 |

### 8. 当前仓库扩展点与相关规范

- `internal/app/collection_models.go:13`：`CollectionSource` 公开 DTO；`:18` revision，`:27` 外部公开入口。不能把新连接密钥直接加入公开结构序列化。
- `internal/app/collection_models.go:34`：来源输入结构；`:45` 为编辑 revision。新增私密字段应写入既有分离秘密存储模式，空值保留/删除须显式区分。
- `internal/app/collection_api.go:18`：已认证 collection 路由入口；`:54–64` capabilities 当前 external 仅为入口/成品目录登记；需要按实际服务版本生成能力。
- `internal/app/collection_api.go:91–142`：来源验证和数量上限；`:127` 当前 external 要求 RootID，新深度连接若允许先连服务后配置导入目录，应有明确兼容分支，不能影响原独立目录来源。
- `internal/app/collection_api.go:438`：现有分页辅助函数可参考，但远端普通数组和分页包装必须分别处理。
- `.trellis/spec/backend/collection-contract.md:14,22–28`：revision、请求 UUID、64 KiB 严格输入、公共错误形状、数组、只读目录登记与任务归属边界。
- `.trellis/spec/backend/collection-contract.md:51`：已接受意图不可变；外部 API 操作亦应绑定来源 revision 和当时连接配置。
- `.trellis/tasks/11-20-collection-external-api/prd.md`：本次深度 API 授权、真实能力、持久幂等、不接管外部下载器。

### 9. Official external references

所有外部来源均为项目官方发布页、标签源码或官方 OpenAPI；每个链接在 2026-11-20 核实。行号是上游文件 1-based 行号，后续新标签可能变化。

- S1 Sonarr v4 release：https://github.com/Sonarr/Sonarr/releases/tag/v4.0.17.2952
- S2 Sonarr v5 prerelease：https://github.com/Sonarr/Sonarr/releases/tag/v5.0.0.2997
- S3 Radarr v7 release：https://github.com/Radarr/Radarr/releases/tag/v7.0.3.10918
- S4 Sonarr4 OpenAPI：https://raw.githubusercontent.com/Sonarr/Sonarr/v4.0.17.2952/src/Sonarr.Api.V3/openapi.json
- S5 Sonarr5 OpenAPI：https://raw.githubusercontent.com/Sonarr/Sonarr/v5.0.0.2997/src/Sonarr.Api.V5/openapi.json
- S6 Radarr7 OpenAPI：https://raw.githubusercontent.com/Radarr/Radarr/v7.0.3.10918/src/Radarr.Api.V3/openapi.json
- S7 Sonarr4 SeriesController，76–105 新增校验、110 稳定身份筛选、171–175 Created：https://github.com/Sonarr/Sonarr/blob/v4.0.17.2952/src/Sonarr.Api.V3/Series/SeriesController.cs
- S8 Sonarr5 SeriesController，106–115 language/seasonType 校验：https://github.com/Sonarr/Sonarr/blob/v5.0.0.2997/src/Sonarr.Api.V5/Series/SeriesController.cs
- S9 Radarr7 MovieController，83–113 新增校验、242–246 Created：https://github.com/Radarr/Radarr/blob/v7.0.3.10918/src/Radarr.Api.V3/Movies/MovieController.cs
- S10 Sonarr4 SeriesEditorController，35–38 仅赋值明确传入的 monitored：https://github.com/Sonarr/Sonarr/blob/v4.0.17.2952/src/Sonarr.Api.V3/Series/SeriesEditorController.cs
- S11 Radarr7 MovieEditorController，58–61 同样可空字段更新：https://github.com/Radarr/Radarr/blob/v7.0.3.10918/src/Radarr.Api.V3/Movies/MovieEditorController.cs
- S12 Sonarr4 SeriesSearchCommand，SeriesId 属性：https://github.com/Sonarr/Sonarr/blob/v4.0.17.2952/src/NzbDrone.Core/IndexerSearch/SeriesSearchCommand.cs
- S13 Radarr7 MoviesSearchCommand，MovieIds 属性：https://github.com/Radarr/Radarr/blob/v7.0.3.10918/src/NzbDrone.Core/IndexerSearch/MoviesSearchCommand.cs
- S14 Sonarr4 lookup controller：https://github.com/Sonarr/Sonarr/blob/v4.0.17.2952/src/Sonarr.Api.V3/Series/SeriesLookupController.cs
- S15 Radarr7 lookup controller：https://github.com/Radarr/Radarr/blob/v7.0.3.10918/src/Radarr.Api.V3/Movies/MovieLookupController.cs
- S16 Sonarr5 lookup controller，33–42 语言默认来自实例设置：https://github.com/Sonarr/Sonarr/blob/v5.0.0.2997/src/Sonarr.Api.V5/Series/SeriesLookupController.cs
- S17 Sonarr4 AddSeriesService，102–125 元数据及季度回填、127–146 路径/监控校验：https://github.com/Sonarr/Sonarr/blob/v4.0.17.2952/src/NzbDrone.Core/Tv/AddSeriesService.cs
- S18 Radarr7 AddMovieService，99–136 元数据和目标路径：https://github.com/Radarr/Radarr/blob/v7.0.3.10918/src/NzbDrone.Core/Movies/AddMovieService.cs
- S19 Sonarr5 editor controller：https://github.com/Sonarr/Sonarr/blob/v5.0.0.2997/src/Sonarr.Api.V5/Series/SeriesEditorController.cs
- S20 Sonarr4 command controller，按 name 解固定 Command 类型并返回入队 ID：https://github.com/Sonarr/Sonarr/blob/v4.0.17.2952/src/Sonarr.Api.V3/Commands/CommandController.cs
- S21 Radarr7 command controller：https://github.com/Radarr/Radarr/blob/v7.0.3.10918/src/Radarr.Api.V3/Commands/CommandController.cs

## Caveats / Not Found

- 本研究是固定标签静态协议核查，不是四种外部软件真实实例联调；fixture 通过后仍须记录“未验证真实实例”，不能宣称已经连接用户 NAS。
- 未发现可依赖的远端客户端幂等键契约；搜索请求发生响应丢失时无法承诺端到端 exactly-once。必须保留结果待核实并避免重发。
- 未为 Sonarr 产品 v3、Radarr 旧产品主版本及未来标签保证完整兼容。不能从 API 名称 `/v3` 推导产品主版本支持。
- Sonarr5 正处预发布且多语言/季度模型已变化；把 v4 DTO 通用于 v5 会漏字段。未完成独立 fixture 前，不开启 v5 新增能力。
- 未核实或不在本次范围的操作，包括文件删除、下载器全局暂停、服务安装、全局搜索、任意 API 透传，保持不可用。
- 公共文档和 release 的 Latest 会变化；本文件“当前”均指 2026-11-20，实施以引用标签和运行时版本检测为准。
