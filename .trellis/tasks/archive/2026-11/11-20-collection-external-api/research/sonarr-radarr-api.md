# Research: Sonarr/Radarr API

- Query: 核实 Sonarr/Radarr 官方 API、认证、版本、订阅/监控/搜索、队列/历史、幂等恢复以及现有 NAS 扩展点。
- Scope: mixed — official source/OpenAPI and repository extension points; no real user instance traffic.
- Date: 2026-11-20

## Findings

### 1. Verified versions and source policy

本次通过官方 GitHub `releases/latest` 核实的是以下非 prerelease 版本。实现和 fixture 应引用固定提交，不能把 `develop` 或网站显示的下一代文档当作已验证稳定版本。

| Service | Verified stable tag | Published UTC | Examined source commit | API prefix |
| --- | --- | --- | --- | --- |
| Sonarr | `v4.0.20.3014` | 2026-09-16 16:39:36 | `cab419ade8ac7fcab5bf80394ee492abd35d5f5a` | `/api/v3` |
| Radarr | `v7.0.3.10918` | 2026-11-16 01:40:22 | `af772b051ac27a0615c7f8b7f78622b6dc183be0` | `/api/v3` |

官方 Sonarr OpenAPI 的 `info.description` 说明 v3 API 文档适用于 Sonarr v3/v4，一部分能力只在 v4 提供；这不能证明旧版本具有本次全部接口。建议首期明确 Sonarr 4.x / Radarr 7.x 已核实，其他主版本显示“未验证”，至少禁止写操作，不从相同 `/api/v3` 前缀推断兼容。实际连接必须读取 `system/status` 的 `appName`、`version`，拒绝软件类型不符；部署实例版本才是最终事实。

本文件中的路由均来自各固定提交的 `src/{Sonarr|Radarr}.Api.V3/openapi.json`，关键写动作再核对 Controller/Resource/Command 源码。OpenAPI 的成功状态码有陈旧之处：Sonarr 新增/命令标注 200，但 Controller 实际调用 `Created(...)`；Sonarr editor 标注 200，Controller 调用 `Accepted(...)`。客户端宜接受预期成功的 2xx，按响应结构及后续读取判断，不依赖唯一状态码。

### 2. Authentication and connection

- 两者 OpenAPI 都声明 `X-Api-Key` 请求头，也支持 `apikey` 查询参数。固定适配器只使用 `X-Api-Key: <server-side-secret>`，不把密钥拼入 URL。
- 两者 API-key handler 还支持 `Authorization: Bearer ...` 回退，但本项目无需提供多种认证路径；采用官方 OpenAPI 的专用头即可。无凭据/无效凭据返回 401，禁止把 401 当作空资源列表。
- 参考 Sonarr `ApiKeyAuthenticationHandler.cs:38-79`；Radarr 对应文件 `:36-77`；两者 `HandleChallengeAsync` / `HandleForbiddenAsync` 设置 401/403。
- 基址允许用户反向代理前缀，例如 `/sonarr`；固定追加 `/api/v3/...` 时保留此前缀。拒绝 userinfo、query、fragment、控制字符、含混或编码后的路径分隔符。基址是私密连接配置；与现有公开 `externalUrl` 分离。
- 连接客户端禁用环境代理、Cookie jar 和所有自动重定向。重定向即明确错误；不跟随登录页或换主机响应，不把 API key 发往重定向目标。TLS 使用默认验证，不提供任意跳过证书检查。
- 私网实例仅在来源 `allowPrivate=true` 后允许；每次 DNS/拨号均复用现有网络 IP 分类。链路本地、元数据和其他保留网段不随私网开关放行。

### 3. Fixed read route matrix

以下路径均相对保留代理前缀的 `/api/v3`。只允许这些类型化请求；浏览器不得提供原始 method/path/query/body。

| Purpose | Sonarr | Radarr | Response / mapping |
| --- | --- | --- | --- |
| Identity/version | `GET /system/status` | same | object: `appName`, `instanceName`, `version`, `branch`, `urlBase`, `startTime`; 不透传 `appData`/`startupPath` 等私密路径 |
| Health | `GET /health` | same | array: `source`, `type`, `message`, `wikiUrl`; 空数组是无已报告异常，不代表所有功能可用 |
| Existing root choices | `GET /rootfolder` | same | array: `id`, `path`, `accessible`, nullable `freeSpace`; 仅列已有目录，不创建目录 |
| Quality profile choices | `GET /qualityprofile` | same | array: `id`, `name`, profile metadata; 首期 UI 只需有界 id/name |
| Managed resources | `GET /series` | `GET /movie` | 顶层数组，无标准 page/pageSize 参数；不能伪装为上游分页 |
| Identity reconciliation | `GET /series?tvdbId=N` | `GET /movie?tmdbId=N` | 零项/单项数组；按数值元数据身份查已有条目，不按标题认同一资源 |
| Resource detail | `GET /series/{id}` | `GET /movie/{id}` | object, unknown id -> 404 |
| Metadata lookup | `GET /series/lookup?term=...` | `GET /movie/lookup?term=...` | 元数据候选数组；与下载 release 搜索不同 |
| Queue | `GET /queue?page=1&pageSize=50` | same | paging envelope |
| History | `GET /history?page=1&pageSize=50&sortKey=date&sortDirection=descending` | same | paging envelope |
| Recent/in-memory commands | `GET /command` | same | command array，不是永久审计记录 |
| Known command | `GET /command/{id}` | same | command object, 404/清理后不可推断是否曾执行 |

补充：Radarr 存在 `GET /movie/lookup/tmdb?tmdbId=N`，可按 TMDB id 重新取得 metadata；Sonarr 本次核实的 `SeriesLookupController` 只有通用 `term` 路由，未宣称一个对称的 `/series/lookup/tvdb` 路由。不要未经验证地构造对称 URL。

资源公共字段建议：外部条目数值 `id` 转字符串 `externalId`；`title`, nullable `year`, `monitored`, `status`, 元数据身份（Sonarr `tvdbId`，Radarr `tmdbId`）、支持的动作。不把两种身份混用。Sonarr 为剧集管理，Radarr 为电影管理，不声称支持音乐等资源。

资源列表没有上游分页：必须给 HTTP body、解码条目数、文本和返回条目数设预算。超预算明确返回受限/错误，不能把截断数据伪装为完整成功。建议一次最多 2–4 MiB、1000 条作为初期可配置前的固定保护；大库需要后续专门方案。本建议数字是本项目预算，不是上游协议限制。

### 4. Queue/history pagination and fields

两者 paging envelope：`{page,pageSize,sortKey,sortDirection,totalRecords,records:[...]}`。上游的 `records` schema 可 nullable，本项目应输出 `[]`。本项目建议 `page >= 1`，`pageSize` 最大 100、默认 50，每次只取一页，禁止后台无限翻页；`totalRecords` 保持上游值。

- Sonarr queue 支持 `includeSeries`, `includeEpisode`, `includeUnknownSeriesItems`, `seriesIds`；Radarr 支持 `includeMovie`, `includeUnknownMovieItems`, `movieIds`。不要使用 Sonarr `seriesId` 去请求 Radarr 或反之。
- queue 共同字段：`id`, `title`, `status`, `trackedDownloadStatus`, `trackedDownloadState`, `size`, nullable `estimatedCompletionTime`, `downloadId`, `protocol`, `statusMessages`, `errorMessage`。Sonarr 额外 `seriesId`, `episodeId`, `seasonNumber`；Radarr 为 `movieId`。
- 两者 OpenAPI 把 `sizeleft`/`timeleft` 标注 deprecated，不能依赖这些字段做完成判定；`size` 在上游 schema 是 double，要做有限值和范围检查。大小在 NAS 对外模型仍用整数/nullable，未知值不能填 0 冒充已知。
- queue `status` 枚举包含 `unknown|queued|paused|downloading|completed|failed|warning|delay|downloadClientUnavailable|fallback`。这只是外部系统/下载器状态，绝不等于本系统媒体入库完成。
- history 共同字段：`id`, `sourceTitle`, `date`, `downloadId`, `eventType`, `quality`, `data`；Sonarr 关联 `seriesId`/`episodeId`，Radarr 关联 `movieId`。`eventType` 各服务语义不同，作为外部事件显示或明确映射，不当作 NAS 任务状态。
- history 建议只允许固定 `sortKey=date`、`sortDirection=descending` 和一个明确外部 target id 的过滤，不透传任意排序字段。Radarr `HistoryController.cs:64-95` 显式映射 sortKey 白名单与默认排序。
- 绝不调用队列 DELETE 充当暂停：Sonarr `QueueController.cs:73-94` 的删除默认 `removeFromClient=true`，有实际下载删除/停止跟踪副作用，超出本次非破坏性控制范围。

### 5. Add subscription / managed item

Sonarr 和 Radarr 的订阅实际上是受监控 series/movie，不是本项目 RSS 规则记录。新增前获取已有 root folder 和 quality profile；只允许选择已有选项。不要把浏览器传来的任意外部绝对路径直接透传，也不要新增/修改上游根目录或下载客户端。

Sonarr `POST /series` 的固定负载示例（示例数值/名称必须从当前实例 lookup 与选项中取得）：

```json
{
  "title": "Selected lookup title",
  "tvdbId": 123,
  "qualityProfileId": 1,
  "rootFolderPath": "/series",
  "monitored": true,
  "monitorNewItems": "all",
  "seasonFolder": true,
  "seriesType": "standard",
  "addOptions": {
    "monitor": "all",
    "searchForMissingEpisodes": false,
    "searchForCutoffUnmetEpisodes": false
  }
}
```

Sonarr `SeriesController.cs:100-105` 校验有效 quality profile、非空 title、正数 tvdbId 和重复身份；`:168-175` 交给 AddSeriesService。`AddSeriesService.cs:114-139` 根据 TVDB 身份重新拉取元数据，未提供 seasons 时使用 Skyhook seasons；`:142-166` 从 root 生成 path 并校验。首期只需固定 `seriesType=standard`，如 UI 开放 anime/daily 必须显式枚举；无须承诺支持所有 season/episode 粒度监控策略。

Radarr `POST /movie` 的固定负载示例：

```json
{
  "title": "Selected lookup title",
  "tmdbId": 123,
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

Radarr `MovieController.cs:93-114` 校验 path/root/quality/tmdbId；`:239-246` 新增；`AddMovieService.cs:108-150` 根据 TMDB 元数据生成并验证目录。`minimumAvailability` 有 `tba|announced|inCinemas|released|deleted` 枚举，首期可固定 `released`；不能把 `deleted` 等全部枚举自动当作可写选项。`monitor=movieAndCollection` 会扩大到 collection，首期固定 `movieOnly` 避免扩大授权。

新增成功返回包含 `id` 的资源。新增本身可触发上游元数据刷新；即使立即搜索参数为 false，受监控项目后续仍由外部系统自身的 RSS/自动流程编排。UI 应明确新增监控的含义，不宣称是纯本地收藏。

**新增与搜索分开持久化**：所有 addOptions 自动搜索开关固定 false；用户明确搜索时使用下一节独立操作。这样创建丢响应后只需核对条目，不会为重试再触发搜索。

重复 ID 并非远端幂等：Sonarr `SeriesExistsValidator.cs:17-28` 和 Radarr `MovieExistsValidator.cs:15-26` 会拒绝已存在身份，通常为 validation 4xx，不是返回原成功。创建前已有同身份时，返回“已存在/冲突”或明确“采用现有条目”的无写结果，不能记录为新创建，也不能悄悄覆盖其目录/质量/监控设置。

### 6. Narrow monitor/unmonitor mutation

优先使用 editor 路由的部分字段语义，不使用“读全对象后 PUT 全对象”来改一个布尔值。后者会覆盖其他用户刚刚修改的目录、质量、标签等字段。

```http
PUT /api/v3/series/editor
Content-Type: application/json

{"seriesIds":[42],"monitored":false}
```

```http
PUT /api/v3/movie/editor
Content-Type: application/json

{"movieIds":[42],"monitored":false}
```

两者可用 `true` 恢复顶层监控。固定仅单项 id，不暴露整库批量写。Sonarr `SeriesEditorResource.cs:8-18` / `SeriesEditorController.cs:27-58`，Radarr `MovieEditorResource.cs:8-16` / `MovieEditorController.cs:48-71` 都用 nullable 字段 + `HasValue` 选择更新，省略其他字段不会重置设置。Sonarr 最终返回 Accepted/202（Controller `:106`），Radarr Ok/200（Controller `:139`）。应在 PUT 后用 GET detail 确认该布尔值。

这不是下载暂停：取消监控不取消已入队下载；恢复 series 顶层监控也不等于改变所有 season/episode 的监控标记。动作名称使用“开启监控/取消监控”，提示保留现有子项选择，不能显示“暂停全部下载/恢复全部下载”。固定 `pause/resumeDownload` 能力为 false。

### 7. Explicit targeted search and command tracking

```json
{"name":"SeriesSearch","seriesId":42}
```

```json
{"name":"MoviesSearch","movieIds":[42]}
```

分别向 Sonarr/Radarr `POST /command`，只允许固定命令名和一个已存在正数 target id。名称由 Controller 去掉类型 `Command` 后匹配，原始 JSON 再反序列化为派生命令：Sonarr `CommandController.cs:51-77`；Radarr 对应 `:53-79`。派生参数见 Sonarr `SeriesSearchCommand.cs:5-17` 的 `SeriesId`，Radarr `MoviesSearchCommand.cs:6-10` 的 `MovieIds`。

这也是 OpenAPI 不足处：`CommandResource` schema 不能完整描述每种动态命令的顶层目标字段，不能只按通用 schema 生成客户端后丢弃 `seriesId/movieIds`。

成功响应保存数值 command `id`，后续 `GET /command/{id}`。公共字段可映射 `id`, `name`, `status`, `result`, `queued`, `started`, `ended`。`status=completed` 仅说明搜索命令执行结束，不说明找到资源或下载完成；`result=unsuccessful` 与 failed 需单独展示。命令状态枚举：`queued|started|completed|failed|aborted|cancelled|orphaned`。上游异常正文/exception 可能含路径和来源信息，不直接输出。

禁止无目标/全局命令和浏览器自定义 name，例如 `MissingEpisodeSearch`、全库搜索、刷新磁盘、移动、删除、安装等；缺少 target 时直接 400，不能让默认空数组变成更广搜索。

### 8. Durable idempotency and uncertain responses

**没有核实到远端 Idempotency-Key 或持久 client request ID 契约。NAS 必须自建操作记录。** 核实的 `CommandQueueManager.cs:102-139` 只在同名命令仍处于 queued/started 时按派生参数去重并返回现有 command；已完成后相同 POST 会新建执行。这不能替代 NAS 幂等。

两者 `CommandQueueManager.cs:156-171`：GET list 取内存命令集合，GET id 才有数据库回退；`:235-245` 从内存清除结束超过 5 分钟的命令；`CommandRepository.cs:25-29` 清理结束超过 1 天的持久命令。上游重启还可能将 started 标为 orphaned（repository `:32-44`）。因此“查不到原命令”永远不等于“可以安全重发”。

建议 NAS 操作记录独立于 collection acquisition task，包含：稳定 UUID、sourceId、sourceRevision、service、固定 action、单项 target/metadata identity、不可变意图摘要、连接身份摘要、发送前已知事实、远端 resource/command id、`prepared|sending|accepted|completed|failed|unknown`、安全中文消息和时间。先落盘 prepared/sending，再发送一次。相同 UUID + 相同意图返回原记录；相同 UUID + 改意图 409。进程重启后 sending 进入 unknown/reconcile，绝不自动回到待发送。

| Action | Response lost reconciliation | Unsafe behavior |
| --- | --- | --- |
| Add series/movie | GET exact tvdbId/tmdbId；核实发送前不存在、当前身份及期望 root/quality/monitored。匹配可记“已确认存在”，持久外部 id；不冒充拥有因果证明。不存在/字段冲突仍 unknown/冲突，不盲重 POST。 | 重 POST 后把重复 validation 错误当失败再换请求 id |
| Set monitored | GET detail 验证目标布尔值；一致记“已确认目标状态”，不一致标冲突/unknown。持久稳定 intent，避免旧 retry 覆盖用户后来的反向操作。 | 用过时快照完整 PUT 或响应丢失后不停 toggle |
| Targeted search, known id | GET original command id，持久实际状态；404/已清理保持 unknown/recovery_required。 | 404 后重新 POST |
| Targeted search, no id | 有界读取近期 command，核对固定 name、精确目标、发送窗口与发送前命令快照；只有可验证唯一对应才关联。多个候选/列表已清理/无强相关证据保持 unknown，明确需外部确认。 | 同标题/同目标恰巧有记录就宣称属于本操作，或自动重发 |

额外源码细节：两个 CommandController 都读取非标准 `UserAgent` 请求头，存入 command body 的 ClientUserAgent；公共 `CommandResource.ClientUserAgent` 经过 `UserAgentParser.SimplifyUserAgent`（两者 `CommandResource.cs:114`）。这不是受支持的幂等或可靠 correlation API；若日后利用标记做辅助核对，必须在固定版本验证 raw body 和重启持久化，不能依赖被简化的顶层字符串，也不能宣称跨版本保证。

连接变更必须纳入恢复：source revision 不仅防 UI 陈旧，还防未知操作随后到另一个 endpoint“对账成功”。有未知操作时，要么保留不可变私密连接快照并约束凭据版本，要么禁止会使原目标无法对账的连接变更；不能拿新来源配置证明旧目标结果。不要将原 endpoint/API key 放进普通操作 JSON/SQLite。

### 9. Suggested unified capabilities and DTO behavior

已验证 Sonarr/Radarr 均可声明：`status`, `health`, `resources`, `queue`, `history`, `lookup`, `options`, `add`, `setMonitored`, `targetedSearch`。缺认证、版本不符、某必需路由不可用时禁用对应写能力并返回原因。`pauseDownload`, `delete`, `move`, `manageDownloader`, `globalSearch` 固定 false。

读操作各自返回成功/失败；队列错误不能被资源列表成功吞没。外部服务健康警告可与有效列表同时存在。绝不伪造默认空列表成功；错误只输出有限中文说明和可公开状态码，标题/health message 要清理和密钥脱敏。

Root/profile 选项应有短期受控快照，浏览器用 opaque choice id 或来源绑定选项引用，服务端再解出真实外部 rootPath；保存操作时绑定快照或复核其仍属于当前实例。首次连接只读；任何有副作用动作需要明确选择目标和用户确认。请求/响应预算、并发上限均在服务端固定，不接受浏览器任意调整。

外部队列继续由 Sonarr/Radarr 独立编排；不将 `downloadId` 自动登记为现有 NAS qB 任务，不干预上游 category。完成目录仍用现有独立授权目录 -> register preview -> immutable task -> exact scan coverage 链路。API 状态不扩大 NAS 可读路径授权。

## Files and extension points

检索先使用现有 CodeGraph CLI `query collectionSaveSource`（命中 `internal/app/collection_api.go:91`），再读源码确认；以下为 2026-11-20 观察位置，父代理并发实现后行号可能变化。

| File and location | Existing behavior / safe extension point |
| --- | --- |
| `internal/app/collection_models.go:10-47` | Source/input 分离、revision、hasSecret、allowPrivate、externalUrl；增加固定 service 配置应保留老 external 数据默认可用 |
| `internal/app/collection_api.go:18-40` | 所有 collection routes 统一 `a.auth`；新增适配器路由必须复用会话/同源/strict JSON |
| `internal/app/collection_api.go:91-200` | 保存完整来源并校验 revision、kind、公开入口、64 来源上限；连接配置独立于公开入口，写密码空值保留语义需明确 |
| `internal/app/collection_api.go:203-248` | 当前 external test 只测独立完成目录；API test 应独立报告，不用一个成功替代另一条链路 |
| `internal/app/collection_store.go:59-119` | `collection-secrets/<uuid>.json`、原子写、限制权限、读大小/regular-file 检查；适合扩展服务秘密，不混普通 DB |
| `internal/app/collection_store.go:123-152` | 有界 source 读取；网络操作前关闭 rows |
| `internal/app/collection_store.go:211-229` | optimistic task revision 与安全中文事件模式可参考；不要把外部 API 操作伪装成已发布媒体任务 |
| `internal/app/collection_store.go:276-315` | request_id 不可变绑定与 fingerprint 冲突范式；外部操作需自己的持久记录和冲突语义 |
| `internal/app/collection_rss.go:27-86` | 现有 URL/IP/DNS/无代理有界 client；API 客户端应复用边界思想，但收紧为拒绝所有自动重定向 |
| `internal/app/collection_models.go:153-159` | manager 生命周期与锁；锁不跨上游网络请求，来源 dispatch 前后复核 revision |
| `internal/app/collection_preview.go:124-175` | external register 走现有授权目录/版本预览；API 新增不应跳过或替换该链路 |
| `internal/app/collection_import.go:143-244` | exact coverage 和实际 item IDs；外部 completed 不直接写 imported |

## Related specs

- `.trellis/spec/backend/collection-contract.md`: 当前 schema v3、source secrets、external 独立编排、revision/UUID、发布和 exact import coverage。
- `.trellis/spec/backend/runtime-contract.md`: strict 64 KiB JSON、鉴权/同源、SQLite 一连接、网络外不持有 DB transaction/rows、生命周期、中文 API 错误。
- `.trellis/spec/backend/torrent-search-contract.md`: 私密 endpoint/credential 分离、有界上游、脱敏顺序、无环境代理、未知响应不能重提、不可借 qB 绕过原所有权。
- `.trellis/tasks/11-20-collection-external-api/prd.md`: 四固定适配器、真实能力、稳定请求 ID、来源 revision、无破坏性控制、隔离 fixture 验证。
- `AGENTS.md`: 中文方法/字段/配置说明与业务日志、显式类型、项目环境包装器、新增临时单元测试执行后清理。

## Official references and versions

URL below are exact official sources examined, not third-party client library assumptions.

1. Releases: `https://api.github.com/repos/Sonarr/Sonarr/releases/latest` and `https://api.github.com/repos/Radarr/Radarr/releases/latest` (queried 2026-11-20); fixed releases `https://github.com/Sonarr/Sonarr/releases/tag/v4.0.20.3014` and `https://github.com/Radarr/Radarr/releases/tag/v7.0.3.10918`.
2. Sonarr pinned OpenAPI: `https://raw.githubusercontent.com/Sonarr/Sonarr/cab419ade8ac7fcab5bf80394ee492abd35d5f5a/src/Sonarr.Api.V3/openapi.json`.
3. Radarr pinned OpenAPI: `https://raw.githubusercontent.com/Radarr/Radarr/af772b051ac27a0615c7f8b7f78622b6dc183be0/src/Radarr.Api.V3/openapi.json`.
4. Human API docs: `https://sonarr.tv/docs/api/` and `https://radarr.video/docs/api/`; prefer fixed source when live docs target another generation.
5. Sonarr pinned source prefix: `https://github.com/Sonarr/Sonarr/blob/cab419ade8ac7fcab5bf80394ee492abd35d5f5a/`; all Sonarr file:line citations above are relative to this commit.
6. Radarr pinned source prefix: `https://github.com/Radarr/Radarr/blob/af772b051ac27a0615c7f8b7f78622b6dc183be0/`; all Radarr file:line citations above are relative to this commit.
7. Controller/resource paths relative to those prefixes: `src/Sonarr.Api.V3/Series/{SeriesController,SeriesLookupController,SeriesEditorController,SeriesEditorResource}.cs`; `src/Radarr.Api.V3/Movies/{MovieController,MovieLookupController,MovieEditorController,MovieEditorResource}.cs`; corresponding `Commands/{CommandController,CommandResource}.cs`, `Queue/QueueController.cs`, `History/HistoryController.cs`.
8. Both repositories: `src/NzbDrone.Core/Messaging/Commands/{CommandQueueManager,CommandRepository,CommandEqualityComparer}.cs`; `src/NzbDrone.Core/IndexerSearch/{SeriesSearchCommand|MoviesSearchCommand}.cs`; `src/NzbDrone.Core/Validation/Paths/{SeriesExistsValidator|MovieExistsValidator}.cs`; service-specific `src/{Sonarr|Radarr}.Http/Authentication/ApiKeyAuthenticationHandler.cs`.

## Caveats / Not Found

- 本研究未连接真实 Sonarr/Radarr、未运行第三方二进制/容器、未做真实元数据查找或添加/搜索/监控动作；源码/OpenAPI 协议核实不等于实机验收。
- 未确认远端持久 idempotency key、ETag/If-Match 或精确一次执行契约。NAS source revision 不能防止外部 UI 同时修改；窄 editor 写减少影响，但最终“目标状态已确认”不能证明没有并发写。
- Sonarr 3.x/5.x、Radarr 5.x/6.x 或未来版本未逐一验证；只因使用 API v3 不能自动启用所有写操作。未知版本与未知响应默认禁用/失败，不能假成功。
- 未确认完整资源列表的大库分页接口；上游 `/series`、`/movie` 目前是数组。需保留明确预算/超限说明，不能截断后给虚假总数。
- 没有把取消监控映射成停止已下载任务，没有将 DELETE queue/series/movie、全局命令或根目录/下载器配置纳入支持范围。
- API 返回的远端绝对路径不是 NAS 路径授权；即使上游称 imported/completed，仍须独立完成目录授权与本地版本/扫描覆盖验证。
