# Research: 搜种引擎与 Torznab 安全接入

- Query: 为现有 Go / Vue NAS 下载中心选择可完整衔接磁力与种子下载的搜种方案。
- Scope: mixed（CodeGraph 定位现有边界；查阅官方协议、项目源代码与镜像注册表）
- Date: 2026-11-18

## Findings

### 1. 结论与方案对比

首期采用 **Torznab 多来源只读配置文件 + 服务端聚合 + 结果令牌 + 原 BT 登记流程**。优先复用已有 Prowlarr / Jackett；无现成服务时提供可选 Jackett Compose。每个索引器单独配置端点，不使用 Jackett 的 `all` 端点作为推荐部署。

| 方案 | 部署与优势 | 代价与约束 | 本次决策 |
| --- | --- | --- | --- |
| qBittorrent 原生搜索插件 | 复用现有引擎，无额外服务；官方容器构建包含 Python | Python 插件必须信任与人工管理；原生搜索下载接口直接创建引擎任务，无法复用现有受管登记、停止元数据和路径控制；仅取磁力则部分结果不可下载 | 不选首期，可未来作为独立适配器 |
| Prowlarr / Torznab | 已部署用户可直接添加来源；标准 XML 与本地下载代理；独立维护索引器 | 新部署会多一个服务；每个来源有 API key 与类别差异 | 支持，复用标准 Torznab |
| Jackett / Torznab | 部署简单；每个索引器导出 Torznab URL；项目自带聚合但建议避免全局 all | 新部署会多一个服务；每个站点仍需用户在 Jackett 中配置 | 支持，提供可选部署示例 |

用户已明确“规划并实现”，这些实现选择在当前授权范围内，不需要把技术决策再次转为批准关卡。仅给可运行扩展，不内置站点账号、公共索引器名单或自动安装插件。

### 2. 现有代码接入点

经 `codegraph query` 后定点阅读：

- `internal/app/downloads.go:360`：`addDownload` 解析源、幂等去重、检查外部任务冲突、登记私密来源和命令意图，再通知协调器；搜种下载应复用同一登记逻辑，避免复制业务流程。
- `internal/app/download_torrent.go:44`：`parseTorrent` 有界解析与路径/清单约束。
- `internal/app/download_torrent.go:356`：`parseMagnet` 已有磁力身份校验。
- `internal/app/download_qbittorrent.go:27`：`qbClient` 保存后端 SID；搜索来源请求绝不能复用此认证客户端或 Cookie。
- `web/src/components/DownloadCenter.vue:1`：已有连接状态、添加/详情对话框、轮询与幂等动作；增加同级“搜种”标签，保持原任务列表查询语义。
- `internal/app/download_config.go:16`：环境配置与只读凭据模式；新增独立只读 JSON 文件能承载多个端点且不要求 SQLite 迁移。

### 3. Torznab 查询、XML 与类别

官方协议：[Torznab v1.3 draft](https://torznab.github.io/spec-1.3-draft/torznab/Specification-v1.3.html)。本次只需 `caps` 与通用 `search`，不假定每个索引器支持专门 `music` / `book` 模式。查询参数采用 URL 编码的 `t=search&q=<关键词>&apikey=<密钥>&limit=<有界值>&offset=<偏移>`；`t=caps` 可读取通用搜索能力和类别树，且通常也携带密钥。

响应是 RSS `channel/item`，属性以命名空间 `http://torznab.com/schemas/2015/feed` 中的 `attr name/value` 表达；Go 应按 URI + local name 解析，不能锁死 `torznab:` 前缀。示意（为本项目合成的协议样例）：

```xml
<rss xmlns:tz="http://torznab.com/schemas/2015/feed" version="2.0">
  <channel>
    <item>
      <title>示例资源</title>
      <guid isPermaLink="false">stable-source-id</guid>
      <pubDate>Wed, 18 Nov 2026 08:00:00 +0800</pubDate>
      <size>5368709243</size>
      <link>http://jackett:9117/dl/source/?jackett_apikey=REDACTED&amp;path=opaque&amp;file=example.torrent</link>
      <enclosure url="http://jackett:9117/dl/source/?jackett_apikey=REDACTED&amp;path=opaque&amp;file=example.torrent" length="5368709243" type="application/x-bittorrent" />
      <tz:attr name="seeders" value="12" />
      <tz:attr name="peers" value="18" />
      <tz:attr name="infohash" value="0123456789abcdef0123456789abcdef01234567" />
      <tz:attr name="category" value="2000" />
    </item>
  </channel>
</rss>
```

优先使用验证过的 `magneturl`，其次同来源代理 enclosure/link。不要只凭 `infohash` 构造磁力后丢弃私有 tracker 或改变原资源授权语义。`size` 可能位于 item、attr 或 enclosure；解析为 int64，非法或缺失值为未知。seeders、peers 缺失应为 null/unknown，不能当成真实零。RSS `pubDate` 使用 RFC1123/RFC1123Z 等常见时间，无法解析时为空而非失败整个搜索。无关 XML HTML 只能按文本展示。

常见根类别（以 caps 为准）：2000 影片、3000 音乐、5000 电视、7000 图书；漫画可能是 7030 子类，图片不存在通用可靠映射。应用五种 contentType 只在“添加下载”确认，不从站点类别强行推断入库类型。首期关键词 + 来源筛选 + 排序足够，不支持的细类不能静默让用户误以为已过滤。

### 4. Prowlarr 与 Jackett 实际路由

- Prowlarr 单索引器：`http://prowlarr:9696/<indexerId>/api`，通过查询 `apikey` 鉴权。下载代理 `/<indexerId>/download?apikey=...&link=...&file=...`，控制器允许代理真实种子数据，也可返回磁力重定向。来源与下载路径细节见 [Prowlarr NewznabController](https://github.com/Prowlarr/Prowlarr/blob/develop/src/Prowlarr.Api.V1/Indexers/NewznabController.cs)。
- Jackett 单索引器：`http://jackett:9117/api/v2.0/indexers/<id>/results/torznab/api`；复制的完整 URL 可含 `apikey`，本应用配置宜拆分为无查询端点和独立密钥字段。官方说明建议按单独索引器接入，`all` 有类别与结果截断限制。见 [Jackett README](https://github.com/Jackett/Jackett#torznab-feed)。
- Jackett 下载代理：`/dl/<indexerId>/?jackett_apikey=...&path=...&file=...`；无文件名路由可能 `RedirectPermanent` 到磁力，有文件名路由返回 `application/x-bittorrent`。见 [Jackett DownloadController](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Server/Controllers/DownloadController.cs)。
- Jackett 搜索结果生成时通过服务端代理包装下载 URI 和 API key；不能把 link/enclosure 原样返回浏览器。见 [Jackett ResultsController](https://github.com/Jackett/Jackett/blob/master/src/Jackett.Server/Controllers/ResultsController.cs)。

上面的路径不是让浏览器传任意 URL 的 API。服务端读取配置、组合搜索，保存受控私密结果。支持同一反向代理基路径时必须按 URL path components 推导 origin 和代理前缀，不能字符串 startsWith 误认 `http://jackett:9117.evil` 或 `/dl/source-evil`。

### 5. 安全下载闭环与限制

推荐契约：

1. `GET /api/torrent-search/status`：只返回启用状态、脱敏来源 id/name 与配置帮助；不返回地址、密钥或 Cookie。
2. `POST /api/torrent-search`：请求 `{query, sourceIds}`；后端固定来源并发搜索；返回 queryId、结果 DTO、来源成功/失败标记、截断提示。结果仅含服务端生成的随机不透明 resultId。
3. `POST /api/torrent-search/downloads`：请求 `{resultId,requestId,contentType,createLibrary,libraryId?}`；命中短期缓存后解析 magnet 或受限拉取 torrent，调用现有登记流程；不得提交任意 URL，也不接受客户端覆盖标题/哈希/来源。

实现可使用其他路由名称，只要数据边界保持一致。结果缓存需有 TTL、条目/批次总量上限；过期返回 410 并提示重新搜索。下载若已登记但响应丢失，应先依 requestId 返回既有任务，即使缓存此后过期也保持幂等。

网络边界：

- 独立 `http.Client`，禁止系统代理、无 Cookie jar、无 qB 认证；只访问管理员配置端点。
- 搜索和代理种子 `CheckRedirect` 返回 `http.ErrUseLastResponse`。收到 Location 时：magnet 经原 parseMagnet 校验；HTTP(S) 只能保持同 origin、相同配置导出的下载代理路径且有最大跳数，禁止跳往第三方 torrent URL。
- 管理员可显式配置回环/私网 Jackett 或 Prowlarr，因为这是 NAS 的预期部署。以固定来源 allowlist 解决 SSRF，不在此构造任意公网爬取器。非配置 origin、userinfo、片段、非常规协议、路径穿越/编码分隔符均拒绝。
- 固定 origin 必须比较规范化 scheme、hostname、有效 port，禁 URL.User；每次追加 API key 只对已授权来源执行，绝不携密钥跨 origin。
- 响应有界读取后解析：搜索 XML 建议 4 MiB、每来源 100 项、最多 8 来源；种子沿用现有 8 MiB。超量应明确报错或显示截断状态，不能无界分页到站点末尾。
- 10–20 秒超时、全局/每会话并发限制；相同短期查询可缓存，部分来源离线仍返回其余来源，不以单一 503 丢弃已成功结果。
- 日志只记录来源 id、耗时、结果数和脱敏错误；不能记录原查询 URL、响应体、磁力、torrent URL、API key 或原生 http error（后者常包含 URL）。

### 6. qBittorrent 原生搜索 API 的后续扩展契约

qB 5.x 的搜索 API 使用 `/api/v2/search/`：`plugins`；`start`(pattern/plugins/category) 返回作业 id；`status`；`results`(id/limit/offset)；`stop`；`delete`。结果可能包含 fileName、fileSize、nbSeeders、nbLeechers、siteUrl、descrLink、fileUrl。后端必须拥有作业 id、清理停止/删除生命周期，不能让客户端请求任意原生 id。

核对 [qB WebAPI wiki](https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-%28qBittorrent-5.0%29) 及固定 [5.2.4 SearchController](https://github.com/qbittorrent/qBittorrent/blob/release-5.2.4/src/webui/api/searchcontroller.cpp)：`search/downloadTorrent` 是直接调用 addTorrentManager 创建引擎任务，**不是返回本地 torrent 字节**，无法作为现有安全添加流程的替代。插件可执行 Python，不能自动从结果安装任意脚本。

官方 [docker-qbittorrent-nox Dockerfile](https://github.com/qbittorrent/docker-qbittorrent-nox/blob/main/Dockerfile) 当前明确含 `python3`；因此不能以“官方容器一定没有 Python”作为排除理由。排除首期的理由是完整下载衔接与协议维护边界。

### 7. 可选 Jackett 镜像证据

LinuxServer 的 [Jackett 镜像说明](https://docs.linuxserver.io/images/docker-jackett/) 与 [镜像仓库](https://github.com/linuxserver/docker-jackett) 属其发布方的一手资料；镜像是 LinuxServer 维护，不应称为 Jackett 项目官方镜像。

本次只通过 GHCR 标准 HTTP token + manifest GET 核验，**未执行任何 Docker 命令**。固定版本示例：

```text
ghcr.io/linuxserver/jackett:v0.24.3065-ls36@sha256:31b58c6acfae963dbb334ffd1b721fbefebed498a3df872f058d1e5544f8b02d
```

manifest 响应头 `Docker-Content-Digest` 与响应正文 SHA256 一致，包含 Linux amd64 和 arm64。机器可读证据见 [jackett-image.json](./jackett-image.json)。这只证明注册表元数据，不证明容器启动、镜像内 API 或 NAS ACL 已验收。示例应固定 PUID/PGID、只映射其 `/config`、WebUI 默认回环发布；不需要把原媒体或下载卷挂给索引器。

## Related Specs

- `.trellis/spec/backend/runtime-contract.md`：统一鉴权/同源、64 KiB JSON、中文脱敏错误、五类内容。
- `.trellis/spec/backend/download-contract.md`：先登记意图、8 MiB torrent、选集/发布门禁、qB 5.2.4 磁力停止能力。
- `.trellis/spec/frontend/app-contract.md`：既有组件、主题与会话处理。

## Caveats / Not Found

- 未对公共或私有索引站执行真实搜索，未持有用户站点账号/API key；应通过本地 Torznab fixture 覆盖正常、部分失败、恶意 link、重定向、超量和幂等，再由用户在真实配置下验收。
- Prowlarr / Jackett 当前源代码路径属于一手实现证据，未来升级可能变更；来源配置应验证 caps 和实际代理路由，遇到不支持来源失败关闭，不退化为任意 URL fetch。
- 官方 Torznab 文件仍标 draft，不能声称所有站点都完整支持 v1.3 功能；只依赖通用 RSS、caps、search 和本次实际解析边界。
- qB 结果 HTTP-only 的下载可能还需插件自定义处理，通用 fetch 不一定等价插件行为；本次 Torznab 代理可避免自行实现站点脚本。
