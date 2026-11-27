# 搜种后端实施与临时行为验证

日期：2026-11-18。仅本机 Go / HTTP fixture，无 Docker，无真实站点凭据。

## 实现文件

- `internal/app/torrent_search_config.go`：独立启用开关、64 KiB 严格 JSON 来源文件、1–8 来源、URL / 凭据 / 显示字段约束。
- `torrent_search.go`：鉴权状态与搜索 API、2 搜索并发 / 每搜索 3 来源并发、12 秒聚合截止、随机结果令牌、最多八个快照和每快照五百项、逻辑 TTL 十分钟、身份优先去重。
- `torrent_search_torznab.go`：10 秒单源、2 MiB XML、深度32 / 令牌80000 / 条目100预算、DTD拒绝、命名空间属性、未知指标 null、隐私字段脱敏。
- `torrent_search_download.go`：2 个种子解析槽、15 秒 / 8 MiB、禁止 HTTP 自动重定向、仅允许已校验磁力 Location、精确同源 Jackett / Prowlarr 代理路径、内容身份核对、持久请求绑定和别名幂等。
- `config.go`、`server.go`、`downloads.go`：局部接线，原选集 / 下载登记 / 发布门禁保持。

## 已执行检查

命令使用仓库 `.tools/go/bin/` 工具链。

1. `gofmt`：本轮修改的后端文件格式化。
2. `go test ./internal/app -run 'TestTorrentSearch|TestTorznab|TestSearchDownload' -count=1 -v`：6 个顶层行为测试 / 19 个子用例通过。
3. `go vet ./...`：通过。
4. `go build -o .runtime/verify/search-backend.exe ./cmd/nas`：通过。

## 临时测试覆盖

- Config：禁用不读取不存在来源文件，合法来源，未知 JSON 字段，URL 查询密钥、内嵌凭据、64 KiB 超限拒绝。
- URL：反代前缀、Jackett 精确 indexer、Prowlarr 数字 indexer、默认443端口；跨端口 / 协议 / 非代理路径 / 另一索引器 / userinfo / fragment / 编码路径 / 穿越拒绝。
- XML：任意合法 Torznab 前缀、5,368,709,243 字节保留、seeders与peers转换、未知值与超安全整数为null、私密下载地址不进入DTO、已知密钥误入标题的脱敏、DTD / 畸形 / 多根 / error根 / 超深 / 超大拒绝、一百条限制。
- Cache：128位随机令牌的有效读取、过期410、未知404、非法400及过期清理。
- HTTP：原种子字节不变、无 Cookie/Authorization 转发、合法 magnet Location、外源HTTP跳转拒绝、无效torrent拒绝、8MiB超限413、来源声明身份不符拒绝。
- Add：搜种添加写入现有持久任务，丢弃内存缓存后相同requestId+resultId返回原任务；修改结果或类型409；同身份新requestId记录completed别名操作、再次丢弃缓存仍能幂等返回，最终仅一个下载任务。

临时文件 `torrent_search_regression_test.go` 在执行后按 AGENTS.md 删除；最终空 `go test` 仅代表编译检查，不作为行为测试覆盖。

## 独立复核后的边界修正

- 标题在完整原始文本中先替换已知 API Key，再清理控制字符、裁剪到最多 512 字符加省略号；避免密钥跨裁剪边界时泄漏前缀，并覆盖含普通空格的密钥。
- 使用有界身份分组把纯 v1、纯 v2 与 hybrid 的全部别名折叠，优先保留可安全下载的来源；聚合五百条上限在全部桥接后执行，避免提前截断造成误判。
- 同算法已知 hash 冲突时不合并，不允许未经内容验证的来源声明错误地隐藏不同资源。
- 无 hash 且没有 GUID / URL 时不生成去重键，不再按标题合并；稳定来源材料的摘要不包含可变化标题。
- `go test ./internal/app -run 'TestTorrentSearchTitleBoundary|TestTorrentSearchHybridBridge|TestTorrentSearchAnonymousResults' -count=1 -v`：3 个顶层测试、11 个子用例通过，包含六种 hybrid 到达顺序、后续别名、可下载偏好、冲突身份、上限后桥接、无身份同名保留和稳定编号去重。
- 本轮临时 `torrent_search_boundary_test.go` 已执行并删除，未改变可复用集成脚本归属。

## 边界与待主代理验证

- 不执行 caps 探测，仅通用 search 与可选标准父类别；真实索引器类别兼容性由用户手动验收。
- HTTP下载代理限定已配置来源同origin下的 Jackett `/dl/<indexer>/` 或 Prowlarr `/<数字id>/download`，包含固定反代前缀；泛用Torznab只能直接使用有效magnet。HTTP重定向一律拒绝，可接受已经校验的magnet Location。
- 缓存TTL为授权有效期，过期材料在下一次搜索或令牌读取时清理；最多八个快照，内存不会无界增长。
- 独立集成脚本、浏览器与五类媒体回归由主代理执行；真实Prowlarr/Jackett、站点和NAS/Docker部署不在本次临时测试结论内。
