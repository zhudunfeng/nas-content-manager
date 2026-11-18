# 搜种功能独立质量复核

日期：2026-11-18。按当前任务 `prd.md`、`info.md`、`check.jsonl`、后端运行时/下载契约和前端应用契约复核。复核期间保持实现代理文件归属；发现后与后端实现代理协调修复，随后重新阅读最终代码和执行独立检查。未执行 Docker、真实索引站点请求或 Git 提交。

## Findings (fixed)

### 1. 标题截断可能保留密钥前缀

- 文件：`internal/app/torrent_search_torznab.go`，`convertTorznabItem`。
- 问题：原逻辑先裁剪到 512 个字符，再替换已知 API Key。如果密钥横跨裁剪边界，已被裁短的前缀无法被完整匹配，仍会进入浏览器展示标题。
- 修复：在完整且受 8 KiB 约束的原始标题中先隐藏已知密钥，再清理控制字符和执行最终长度裁剪。后端临时回归覆盖裁剪边界和含空格密钥。

### 2. 无稳定身份的条目退化为同名合并

- 文件：`internal/app/torrent_search_torznab.go`、`internal/app/torrent_search.go`。
- 问题：无 hash、GUID 和 URL 时，原摘要仍包含标题，使同源的不同同名条目合并；同时相同稳定来源的标题变化又无法去重。
- 修复：只对存在 GUID 或 URL 的稳定材料生成来源内摘要，不再包含标题。无稳定材料时不生成去重键，各条结果独立保留。

### 3. Hybrid 别名传播未完整处理已有分组

- 文件：`internal/app/torrent_search.go`，`dedupeTorrentSearchResults`。
- 问题：单次命中的索引映射不能完整折叠已存在的纯 v1、纯 v2 结果与后续 hybrid 桥接；不可信的矛盾 hash 声明还可能把独立资源错误关联。先占身份的坏链接也不应屏蔽后续可用来源。
- 修复：使用最多八百候选的有界身份分组，压缩全部已知别名，优先保留可安全下载的来源；任一同算法已知 hash 冲突时禁止合并。全部分组完成后才执行五百条聚合输出上限，避免先截断影响后续桥接。

上述修复由持有 Go 文件的后端实现代理完成，独立复核已逐段检查修复后的真实代码。相关可重复行为证据见 `search-backend-validation.md`。

## Findings (not fixed)

无已确认但未修复的实现缺陷。真实索引器类别/代理兼容性、Docker/NAS 部署和真实网络质量属于已约定的人工验收范围，不作为本地 fixture 的已验证结论。

## 已核对的实际调用链

- 搜索 API 注册在现有鉴权路由下，复用同源写保护和 64 KiB JSON 限额，来源配置及原始链接不进入公开 DTO。
- 固定配置来源经过 URL / 有效端口校验；HTTP 下载严格限定精确 Jackett / Prowlarr 代理路径及反代前缀，拒绝编码路径、用户凭据、片段及 HTTP 重定向；磁力重定向复用现有身份校验。
- 搜索使用独立 HTTP Transport，没有环境代理、浏览器 CookieJar 或 qB 会话；上游错误转换成固定中文消息，日志不输出查询、来源 URL、密钥或原始 XML。
- 搜索、单来源、XML 文档形状、数值精度、条目、聚合数量、缓存、种子解析分别有固定预算；来源 goroutine 仅写自己对应的结果槽，缓存锁不跨网络调用。
- 添加搜索结果在缓存查询前核对持久化请求绑定，并在网络解析后、登记锁内再次核对。内容身份已存在时写入 completed 别名命令；该别名不会再次发送引擎。缓存过期或重启后仍能返回同一任务。
- 新入口继续进入现有种子/磁力解析、任务登记、文件选择、发布门禁和媒体入库流程，未复制下载业务或引入 schema 迁移。
- 前端有明确状态、手动提交、来源/类别筛选、本地排序及分页。旧请求会被取消且陈旧响应被忽略；关闭搜索后经过 `nextTick` 再打开原添加表单，保留类型确认和稳定请求键。下载器不可添加时仍可搜索，选择按钮禁用且有原因说明。

## Verification

独立执行：

- Lint：通过，`.tools/go/bin/go.exe vet ./...`；前端未配置单独 ESLint 命令，未宣称执行了不存在的检查。
- TypeCheck：通过，`web/` 下 `npm run typecheck`（严格 `vue-tsc --noEmit`）。
- Build：通过，`.tools/go/bin/go.exe build -o .runtime/nas-search-quality.exe ./cmd/nas`。
- Tests：最终 `.tools/go/bin/go.exe test ./...` 通过，此时包内已无测试文件，因此仅代表编译检查。后端实现代理已执行并记录九个顶层临时行为测试、三十个子用例，随后按用户要求删除 `torrent_search_regression_test.go` 和 `torrent_search_boundary_test.go`；复核确认目录中没有新增遗留 `_test.go`。
- Whitespace：`git diff --check` 通过；已有 LF/CRLF 提示不构成检查失败。

主代理负责保留的 HTTP 集成脚本、真实 Go 应用浏览器流程和原五类媒体回归；前端代理负责响应式与双主题浏览器专项。此记录不重复或冒称这些尚由其他代理执行的检查。
