# Research: NAS 影视与动漫获取链路

- Query: 为 NAS 推荐可在 GitHub 获取的影视、剧集和动漫自动化工具。
- Scope: mixed；外部项目能力与维护证据，结合本仓库既有下载与搜索契约。
- Date: 2026-11-19（Asia/Shanghai）

## Findings

### 推荐与职责

| 工具 / 官方仓库 | 适合场景 | 前置条件与边界 |
| --- | --- | --- |
| MoviePilot — https://github.com/jxxghp/MoviePilot | 中文影视一体化管理：搜索、订阅、下载编排、整理、刮削与媒体库刷新；希望少维护多套服务时优先考虑。 | 官方优先推荐 Docker；仍须配置实际站点/订阅、下载器和存储目录，不是一个自带全部片源的服务。当前 V3 使用独立镜像，不能照搬 V2 教程。 |
| AutoBangumi — https://github.com/EstrellaXD/Auto_Bangumi | 专注 RSS 自动追番、标题解析、集号匹配和重命名；适合动漫占主要需求。 | 已有可用 RSS 后把任务交给 qBittorrent / aria2；README 提供 Docker Compose 配置示例，下载器需另行正确配置。 |
| Sonarr — https://github.com/Sonarr/Sonarr | 连续剧订阅、补集、质量升级与整理。 | 管理 Usenet / BT 流程，实际文件传输依赖下载客户端；支持 Windows、Linux 等平台。 |
| Radarr — https://github.com/Radarr/Radarr | 电影订阅、质量配置、失败重试与入库整理。 | 需要索引器与 qBittorrent 等客户端；官方列出 Windows、Linux 等平台。 |
| Prowlarr — https://github.com/Prowlarr/Prowlarr | 统一管理 BT / Usenet 索引器，把配置同步给 Sonarr / Radarr 等。 | 属于搜索与索引管理层，不能代替订阅管理、传输或媒体播放。 |
| Jackett — https://github.com/Jackett/Jackett | 把各 BT 站点的搜索转换成 Torznab 标准 API；适合保留本仓库已有接入。 | 支持公开、半私有与私有 tracker；私有源仍须有效账户权限。无需仅为补齐功能而同时部署 Jackett 与 Prowlarr。 |

建议组合（研究者判断）：**中文综合影视选 MoviePilot + qBittorrent；动漫选 AutoBangumi + qBittorrent**。更偏好分工清晰、细致质量规则时选 **Prowlarr + Sonarr + Radarr + qBittorrent**。媒体服务器可以另接 Jellyfin / Emby / Plex；它们承担媒体库和播放，不能把海报刮削理解成获得正片。以上组合是根据各工具职责形成的建议，不是本仓库已完成集成的声明。

### 已核验发布状态

以下来自 2026-11-19 直接打开官方 `releases/latest` 的跳转结果，说明有正式发布证据；不以 star 数推断稳定性，不表示已经在用户 NAS 测试。

- MoviePilot：v3.2.8，发布页时间 2026-11-18T17:25:57Z（北京时间 11 月 19 日 01:25:57）。
- AutoBangumi：4.0.0，发布页时间 2026-10-24T10:04:31Z。
- Sonarr：4.0.20.3014，发布页时间 2026-09-16T16:39:36Z。
- Radarr：7.0.3.10918，发布页时间 2026-11-16T01:40:22Z。
- Prowlarr：3.0.2.5780，发布页时间 2026-11-16T18:25:56Z。

### 本仓库关联

- `internal/enginehelper/service.go:288`：现有助手只遍历 qBittorrent / Jackett 两个受控引擎；外部项目推荐不意味着可以直接作为当前内置插件安装。
- `.trellis/spec/backend/torrent-search-contract.md:7`：既有搜索使用配置的 Prowlarr / Jackett Torznab；该契约明确本期不支持 RSS 自动订阅。
- `.trellis/spec/backend/download-contract.md:19`：qB 承担传输、校验与做种，Go 不代理影音载荷；后续应保留此职责边界。
- `.trellis/spec/backend/download-contract.md:28`：已就绪媒体受文件选择、路径与状态发布门控制。外部自动整理器不应与本系统同时争抢同一受控任务和目录（研究者建议）。

## 平台与来源边界

Docker / Compose 是 MoviePilot 和 AutoBangumi 的可证部署入口；普通 Linux NAS 或 fnOS 是否适用，要检查目标 CPU、容器运行时、卷挂载和权限。此为部署条件推论，不是“已有官方飞牛 FPK”或“全 NAS 可用”的结论。Windows 可先用 Sonarr / Radarr 已声明支持的原生平台能力，其他工具按官方容器/源码说明验证。

RSS 是更新清单，BT 是传输协议，PT 是带账户与站点规则的私有 tracker 场景，Usenet 需要相应索引与新闻服务器服务；工具安装本身不会授予内容访问权。最终使用建议应强调“已有来源 → 索引/订阅 → 下载器 → 整理 → 媒体库”，避免承诺免费全集或自动产生片源。索引与下载的分工可由官方 README 直接印证。

## External References

### 可供最终答复复用的 web 引用（补核）

- MoviePilot：订阅、搜索、下载、整理、刮削、媒体库刷新；官方推荐 Docker，V3 使用独立镜像。citeturn965472search1
- AutoBangumi：RSS 自动追番、规则生成与命名整理，支持 qBittorrent / aria2；补核时同时取得根页和更新后的 README，应优先使用后者描述当前能力。citeturn965472search0turn965472search2
- Sonarr：RSS 剧集监控、补集、质量升级与重命名，支持 Windows / Linux 等。citeturn770243search0
- Radarr 官方 Wiki：配置索引器与下载客户端后监控 RSS，支持已有电影库导入、电影及合集监控。citeturn770243search10
- Prowlarr 官方项目站源码：索引器管理工具，列出 Windows / Linux 部署入口。citeturn770243search6

补核说明：2026-11-19 两次精简 search_query 已恢复返回可读结果；以上均来自官方 GitHub 仓库或官方 Servarr Wiki。未引用搜索结果中的非官方 RadarrTV 仿冒/再分发仓库。

- MoviePilot 官方 README：https://github.com/jxxghp/MoviePilot
- MoviePilot 直接正式发布：https://github.com/jxxghp/MoviePilot/releases/tag/v3.2.8
- AutoBangumi README：https://github.com/EstrellaXD/Auto_Bangumi/blob/main/README.md
- AutoBangumi Compose：https://www.autobangumi.org/deploy/docker-compose.html
- AutoBangumi 发布：https://github.com/EstrellaXD/Auto_Bangumi/releases/tag/4.0.0
- Sonarr 正式发布：https://github.com/Sonarr/Sonarr/releases/tag/v4.0.20.3014
- Radarr 正式发布：https://github.com/Radarr/Radarr/releases/tag/v7.0.3.10918
- Prowlarr 正式发布：https://github.com/Prowlarr/Prowlarr/releases/tag/v3.0.2.5780

## Related Specs

- `.trellis/spec/backend/download-contract.md`
- `.trellis/spec/backend/torrent-search-contract.md`
- `.trellis/spec/backend/engine-bootstrap-contract.md`

## Caveats / Not Found

- 未部署、未接入真实站点账户，不能证明中文匹配率、实际下载速度、ARM 老机兼容或某个 fnOS 版本可用。
- Servarr 安装 Wiki 和 MoviePilot Wiki 本次抓取无可读正文；能力结论使用官方仓库 README 与发布页，不猜测具体认证门槛或收费条件。
- 不提供站点账号、绕过认证/DRM的方法或第三方盗版聚合源；来源应由用户按实际授权配置。
- web.run 多次检索/打开未返回可读结果或可复用引用标记；已通过项目环境包装器中的 urllib 直接读取官方 raw README 与 GitHub releases/latest 跳转及 relative-time datetime 核验。GitHub API 请求返回 403 rate limit exceeded，因此未使用 API 维护状态字段，不声称已经核实仓库归档状态。以上发布日期来自已读发布页，核实日期为 2026-11-19。
