# 搜种技术方案与实施计划

## Decision

采用 **Go Torznab 适配器 + 已有 Prowlarr / Jackett**，不引入站点爬虫、不执行第三方插件。推荐直接复用已有索引服务；新部署提供可选 Jackett 覆盖文件和手动配置说明。qB 原生搜索虽省一个服务，但插件耦合站点且原生 downloadTorrent 绕过受管选集，首期不使用。用户已授权规划并实施，沿用易用、易部署和安全下载默认方案，不增加批准步骤。

## Data flow

浏览器关键词 → 已鉴权同源 POST → 固定来源并发 Torznab 搜索 → 有界解析/去重 → 十分钟服务端私密结果缓存 → 浏览器仅获得随机 resultId 与安全元数据 → 原添加弹窗确认类型/建库 → POST /api/downloads searchResultId → 受控来源解析 → 原种子校验/命令幂等/选集/发布/入库。

## Configuration

- `NAS_TORRENT_SEARCH_ENABLED=false`：独立可选开关；关闭无任何外部请求。
- `NAS_TORRENT_SEARCH_SOURCES_FILE`：只读 JSON 文件路径，最大 64 KiB、1–8 个来源。JSON 顶层 `sources`，每项 `{id,name,url,apiKey}`；id 只用 ASCII 字母数字短横线下划线，1–40 字符；name 最长80字符。url 是不含凭据、查询和片段的 HTTP(S) Torznab API 完整端点，例如 `http://jackett:9117/api/v2.0/indexers/example/results/torznab/api` 或 `http://prowlarr:9696/1/api`。密钥只在文件中，配置文件不可入库。
- 密钥不得写到 URL 配置、前端、日志或 DB。启动严格校验 JSON 与重复 id。HTTP 明文仅适用于受信任本机/内网。

## HTTP contract

### GET /api/torrent-search/status

返回 `{enabled, sources:[{id,name}], maxResults:500, resultTtlSeconds:600}`。未启用也可读取，来源无 URL/Key。所有搜种接口均鉴权。

### POST /api/torrent-search

输入 `{query:string,sourceId:string,category:string}`。sourceId 空为全部；category 为 `all|video|music|book`，默认 all；query trim后 2–120 Unicode字符、禁止控制字符。复用64KiB JSON限制、同源保护。Torznab `t=search&q=...&limit=100&offset=0&apikey=...`，类别 video=`2000,5000`、music=`3000`、book=`7000`。

返回 `{results:TorrentSearchResult[], sources:TorrentSearchSourceStatus[], expiresAt:string, truncated:boolean}`。Result 为 `{id,title,sourceId,sourceName,sizeBytes:number|null,seeders:number|null,leechers:number|null,publishedAt:string,downloadable:boolean}`；来源状态 `{id,name,state:'ready'|'error',error:string}`。raw XML/URL/guid/magnet 不进入 DTO。不返回描述链接以防passkey泄漏。未知数值为null，不捏造0。仅部分成功返回200及sources错误；全部失败返回502。未配置503，无效400，繁忙429。

### POST /api/downloads

JSON 增加 `searchResultId?:string`，与 magnet 二选一，multipart不接受此字段。其余 `{requestId,contentType,createLibrary,libraryId?}` 保持原契约。所有令牌必须来自服务端缓存，过期410，未知404。结果downloadable=false时422。HTTP来源失败502/超量413，错误完全脱敏。验证requestId/contentType先于网络。

先用持久化 `download_operations` 中 kind=add、payload 中绑定的 `{searchResultId}` 核对原始请求键，已成功登记则返回同一任务，即便缓存过期/应用重启。更换结果或内容类型但重用旧请求键应409。首次提交最终仍复用既有身份幂等；INSERT add operation 时保存不含凭据的结果ID。不改变schema、不重复实现下载登记。

## Resource and security budgets

- 全局同时2次搜索、每次最多3个来源并发；单来源10秒、整个请求12秒；单源XML 2MiB、最多100项，聚合最多500条；配置最多8源。
- 最多缓存8个搜索快照，每个最多500条；固定十分钟TTL。过期/逐出结果可重新搜索。缓存不落盘，关闭与重启丢弃；随机ID至少128位，同一管理员会话保护。
- 按已验证的 BT hash 优先去重，无hash则按服务端来源+有限URL摘要去重，不以同名文件误合并。结果文本长度有界；URL/guid/属性长度有界，畸形XML/DOCTYPE/超大正文明确拒绝。Go XML不解析外部实体，额外拒绝DOCTYPE。
- 优先使用有效 magnet（原parseMagnet），否则只接受**配置来源同origin**的torrent代理URL；HTTP客户端不携带应用/qB会话，不使用环境代理、不跟随网络重定向；如上游以30x Location返回magnet，可本地校验该magnet，跨源HTTP重定向拒绝，提示通过索引服务代理。
- 代理URL限定HTTP(S)、无userinfo/fragment、长度≤16KiB、同源匹配scheme/hostname/effective port；不允许浏览器提交URL；不得借相同host错误端口外泄key。来源下载时保留服务端返回的私密query，不向陌生源附加key。
- 下载响应限8MiB、15秒；全局下载来源解析并发2。网络操作不得持有DB事务或全局业务锁；日志只记录来源ID、结果数量、耗时或受管downloadId。

## UI

- 下载中心按钮“搜种”，打开 `TorrentSearchDialog.vue`；保持五项移动主导航不变。
- 输入明确标签、手动提交，来源/类别筛选；结果按做种最多（默认）/最新/体积升降排序、每页20条，客户端对最多500条分页，不触发外部重复查询。
- loading/初始/空/部分失败/全部失败/未配置/取消状态区别展示；取消/关闭/重搜中止旧请求并防止旧响应覆盖。
- 选中结果不直接下载：关闭搜索弹窗再打开 `AddDownloadDialog`，由可选 `searchResult` prop 展示安全标题和来源，固定来源不可编辑，保留现有类型、建库和稳定requestId重试。新结果或更改类型重置requestId。
- 下载器离线允许搜索但禁添加，解释原因。避免嵌套原生dialog焦点和body overflow异常。

## Implementation plan

1. 研究/PRD/接口契约/上下文就绪。
2. 后端：配置、Torznab、缓存、鉴权路由与原添加流程接线；临时行为回归。
3. 前端：搜索弹窗、DTO、添加衔接、响应式双主题。
4. 复用假qB构造可重复Torznab集成脚本，验证成功/失败/限额/隐私/幂等及过期恢复。
5. 独立质量检查、Go/frontend检查和原媒体回归、真实浏览器交互；更新规范、部署和验收记录，清除临时单测。Docker不执行。

## Future / Out of scope

后续可扩展能力协商/每站类别细分、RSS和更多provider；本次不自动订阅、不托管站点账户、不跨公网任意抓取、不自动安装搜索插件。不以本机受控协议fixture冒称真实站点质量或NAS性能验收。
