# Research: MoviePilot 与 AutoBangumi 深度 API 契约

- Query: 核实两个外部套件的版本、认证、探测、订阅/队列/历史、固定写操作和只读对账，供 `internal/externalapi` 实现。
- Scope: mixed；官方发布页和 tag 源码为外部依据，本仓库契约为边界。
- Date: 2026-11-20（Asia/Shanghai）
- 结论适用版本：MoviePilot **v3.2.8**；AutoBangumi **4.0.0**。按固定 tag 复核，不把滚动分支当版本契约。两者均未连接真实用户实例。

## Findings

### 1. 版本依据与核实方式

官方发布页分别显示 MoviePilot v3.2.8（2026-11-20，短提交 `5eeb454`）与 AutoBangumi 4.0.0（2026-11-20，短提交 `b54f7d4`）。标签对应源码通过官方 `raw.githubusercontent.com` 与官方 codeload ZIP 直接读取、按行审查；GitHub REST 当前出口遇到未认证限流，未使用 REST 回包猜测完整 SHA。

- MoviePilot 发布：<https://github.com/jxxghp/MoviePilot/releases/tag/v3.2.8>
- AutoBangumi 发布：<https://github.com/EstrellaXD/Auto_Bangumi/releases/tag/4.0.0>

AutoBangumi 4.0 发布说明明确包含破坏性改变：旧 GET 控制/刷新接口、旧鉴权兼容以及缺少 JSON Content-Type 的请求不能照搬。当前实施应以 **AB4 API Token** 为认证输入；旧 AB3、MoviePilot v2 和未知主版本不能仅因服务名称一致就启用写操作。MP3/AB4 的其它小版本尚未逐一验证；首版建议以已核实响应形状与已知主版本双重探测，遇到形状差异拒绝操作。

### 2. MoviePilot v3.2.8

#### 2.1 认证与响应封装

推荐服务连接凭据为配置中的 `API_TOKEN`，通过 **`X-API-KEY` 请求头**传入；不要把秘密加入 URL。源码 `app/adapters/web/security/access.py:43-62,307-345,382-396` 定义 header/query 凭据提取及校验，固定 API 凭据绑定当前有效超级管理员身份。普通 Bearer JWT 也被支持，但本项目不必存储用户名/密码或自动登录。

可选网页登录协议（不作为当前首选连接方式）：`POST /api/v1/login/access-token`，`application/x-www-form-urlencoded`，字段 `username`、`password`、可选 `otp_password`；成功为原生 `{access_token,token_type:"bearer",...}`。该登录接口被显式标记为 raw，认证失败为 401，要求 MFA 时 401 + `X-MFA-Required: true`，限流为 429 + `Retry-After`。不要把登录成功体规则套用到业务列表。

v3 业务路由使用 `ResponseAPIRouter`，普通结果统一包成 **`{success,message,data}`**，包括声明 `List[...]` 的订阅/队列/下载历史；不能直接按裸数组解码。`app/api/response.py:450-483` 的 `_wrap_result` 是证据。HTTP 200 且 `success:false` 为业务失败，不得假成功；错误文本只映射为本地固定中文，禁止透传上游任意消息。

官方源码：

- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/adapters/web/security/access.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/login.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/response.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/routers.py>

#### 2.2 固定读取端点

所有路径相对于服务 origin 或已配置反代子路径，API 默认根为 `/api/v1`。不要让用户传入任意 endpoint。

| 用途 | 方法与路径 | 最小查询与响应字段 | 依据 |
|---|---|---|---|
| 版本 | `GET /api/v1/system/global` | 固定公开查询 `token=moviepilot`；`data.BACKEND_VERSION`、可选 `FRONTEND_VERSION`。`moviepilot` 是源码公开常量，不是用户 API 秘密 | `system.py:449-478` |
| 鉴权存活 | `GET /api/v1/system/ping` | API header；必须 `success:true`，`data` 可为空 | `system.py:669-673` |
| 当前订阅 | `GET /api/v1/subscribe/` | 可带 `page=1&count=100`；`data:[Subscribe]`，可读 `X-Total-Count`。只读白名单字段 `id,name,type,year,season,state,media_source,media_id,lack_episode,total_episode` | `subscribe.py:209-229`、`response.py` |
| 一条订阅 | `GET /api/v1/subscribe/{id}` | `data:Subscribe`；仅以正整型 remote ID 构造路径 | `subscribe.py:948-969` |
| 精确身份查询 | `GET /api/v1/subscribe/media/{media_id}` | `media_source=themoviedb`，可带 `season`、`mtype=电影/电视剧`；返回 `data:Subscribe`，无结果为空模型；服务端有投影/标题回退，必须自行再次校验完整身份 | `subscribe.py:464-515` |
| 下载队列 | `GET /api/v1/download/` | `data:[DownloaderTorrent]`；可带固定分页参数；列表可能需要管理或下载查看权限 | `download.py:227-240` |
| 下载历史 | `GET /api/v1/history/download` | `page=1&count=100`；`data:[DownloadHistory]`，可读总数 header；只选 ID、标题、时间、状态等明确字段 | `history.py:264-302` |
| 搜索批次 | `GET /api/v1/subscribe/execution/batches/{batch_id}` | `data.batch_id,state,phase,...`；仅在已经持有可靠 batch ID 时查询 | `subexecution.py:141-163` |

禁止用 `/system/env` 当版本检测：其响应是配置集合，敏感范围不必要。`/system/versions` 是 GitHub Release 列表，不是当前运行版本，不能用于实例版本识别。版本响应本身不证明鉴权成功，必须补查 `ping`。

官方源码：

- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/system.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/subscribe.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/download.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/history.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/subexecution.py>

#### 2.3 已核实写操作及最小输入

**新增精确 TMDB 订阅**：`POST /api/v1/subscribe/`，`Content-Type: application/json`。

```json
{"name":"示例标题","type":"电影","media_source":"themoviedb","media_id":"12345"}
```

电视剧增加 `"type":"电视剧","season":1`。外部 ID 必须为用户选择/明确提供的正整数字符串，不允许仅凭模糊标题创建。**TMDB 的标准枚举字符串是 `themoviedb`，不是 `tmdb`**：`app/schemas/types.py:114-124`。成功体 `{"success":true,"data":{"id":123}}`；`id` 非正整型或 envelope 缺失视为响应不确定，不宣称完成。`SubscribeChain.async_add(...,exist_ok=True)` 有服务端去重逻辑，但没有客户端幂等键，不可当作本地持久幂等的替代。

**启用/暂停**：`PUT /api/v1/subscribe/status/{id}?state=R`（启用）或 `state=S`（暂停），无 body。`subscribe.py:441-461` 接受 `R|P|S`；**`P` 仍参与搜索，绝不能映射为暂停**。`app/chain/subscribe/policy.py:399-400`、`searchtask.py:81-82`、`app/application/subscription/query.py:367` 证明暂停为 `S`；`tests/test_subscription_schedule.py:37-44` 验证 R/P 会参与定时搜索、S 不会。成功只证明状态修改完成，已接受的人工搜索仍可能执行，不能宣称全部下载立即停止。

**显式单条搜索**：`POST /api/v1/subscribe/search/{id}`，无 body；可省略 `type` 查询（避免枚举/编码歧义）。成功 `data` 包括 `batch_id`、`batch_ids`、`target_count`、`queued_count`、`ongoing_count`、`single`。`ongoing_count>0` 可能是已在搜索、未新建任务，也属于有效回执。不要调用无 ID 的 `/subscribe/search`，不要使用旧 GET 兼容入口。对应源码 `submaintenance.py:200-235`，路由由 subscribe 组合挂载。

官方源码：

- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/schemas/types.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/schemas/subscribe.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/submaintenance.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/chain/subscribe/policy.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/chain/subscribe/searchtask.py>
- <https://github.com/jxxghp/MoviePilot/blob/v3.2.8/tests/test_subscription_schedule.py>

#### 2.4 不确定写入的只读对账

- 创建前和超时后，读取订阅列表/身份接口，仅当 **`media_source=themoviedb + media_id + type + season`** 精确相等且唯一，才可以关联 remote ID。电影季号空/0 的规范化必须固定，电视剧必须有确定季号。不要接受标题匹配或服务端 fallback 的近似结果。
- 出现多条同身份、分页未完整读取、超过本地预算、数据形状不符时维持 uncertain；不可取第一个，也不可重发 POST。
- Monitor 读取精确 ID，只有期望 `state=R/S` 与当前一致才对账成功。实体不存在不等于暂停成功。
- Search 若网络失败前没有持久化可靠 batch ID，现有接口没有本项目 requestId 的关联字段；看到“该订阅最近有搜索”不足以确认是本次请求。维持 uncertain，不重发。已有可靠 batch ID 可读批次状态，但本次对账本身只 GET。

### 3. AutoBangumi 4.0.0

#### 3.1 认证与版本检测

推荐用户先在 AB 自身 UI 建立 `scope=api` 的 API Token，然后本项目仅保存该写入凭据；业务请求携带 **`Authorization: Bearer <API Token>`**。`backend/src/module/security/api.py:66-112` 明确按 API Token 验证 scope，session token、旧 JWT/明文兼容不再接受于 header 路径。

网页登录 `POST /api/v1/auth/login` 接受 OAuth 表单 `username,password`，返回 `AuthenticationSuccess` 并通过 **`Set-Cookie: token=...; HttpOnly; SameSite=strict`** 设置持久 session（默认一天）；不是 `{access_token:...}`。`POST /api/v1/tokens` 创建 API Token 需要真实浏览器 session，`scope` 可为 api/mcp；本项目不代办此账户管理操作。用 mcp scope token 访问普通 API 应失败。

**`GET /health`** 位于应用根，不在 `/api/v1`；无认证，返回 `{status:"ok",version,db_ok}`，即使 `db_ok:false` 仍可能 HTTP 200，必须检查字段。随后 **`GET /api/v1/status`** 验证凭据并获得 `{status:boolean,version,first_run}`；这里 `status:false` 表示 AB 后台任务未运行，不等于鉴权失败。不得为“检测连接”自动调用 `/start`、`/restart`。

官方源码：

- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/security/api.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/api/auth.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/api/tokens.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/api/health.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/api/program.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/main.py>

#### 3.2 两类订阅实体不能混淆

AB 同时存在 Bangumi 规则与 RSS 来源，它们拥有独立的整数 ID；本项目应使用 `bangumi:N`、`rss:N` 作为目标，不能把两个 ID 当同一资源。

| 用途 | 端点 | 实际语义与响应 |
|---|---|---|
| 规则列表 | `GET /api/v1/bangumi/get/all` | 裸数组 Bangumi；可返回 `id,official_title,title_raw,season,deleted,archived`；`deleted` 在启停动作中作为软禁用字段 |
| 单规则 | `GET /api/v1/bangumi/get/{id}` | Bangumi 对象，用于目标确认/监控对账 |
| RSS 列表 | `GET /api/v1/rss`（无结尾斜杠） | 裸数组 RSSItem；`id,name,enabled,aggregate,parser,connection_status,last_checked_at`。`url` 只能内部对账，禁止进入公开 DTO |
| 队列 | `GET /api/v1/downloader/torrents` | 裸数组，附 `downloader_id`；仅 Bangumi 分类任务，无法等同于全部下载。部分下载器不可达会被跳过；全部不可达是 503 |
| RSS 种子记录 | `GET /api/v1/rss/torrent/{rss_id}` | 裸数组 Torrent；`downloaded` 代表曾成功提交下载，非文件完成/整理成功；不是全局历史 |
| 规则种子记录 | `GET /api/v1/bangumi/{id}/torrents` | 规则关联 Torrent 记录；首版无需为全局历史作 N+1 扫描 |

源码：`api/bangumi.py:65-96,478-549`，`api/rss.py:25-27,167-181`，`api/downloader.py:69-75`，`downloader/download_client.py:578-609`。AB4 未找到与 Servarr/MP 下载历史相等价的全局有界历史端点：**history 能力关闭**。`/search/bangumi` 是按关键词的 SSE 资源搜索，不是对某订阅补搜/下载命令，**generic search 能力关闭**。

#### 3.3 最窄写动作

**RSS 登记（本期 add）**：`POST /api/v1/rss/add`，明确 JSON Content-Type。

```json
{"name":"示例 RSS","url":"https://example.org/feed.xml","aggregate":false,"parser":"mikan"}
```

只允许类型化必要字段，不透传 id、连接健康、下载器任意配置。`parser` 的已知解析器为 `mikan|tmdb|parser`，首版固定 mikan 是保守默认；用户名称非空可避免 AB 为补名称额外请求 RSS。**这仅登记 RSS，不等于建立 Bangumi 规则、不保证立即获取任何资源**；UI 与成功消息必须称“RSS 已登记”。`aggregate:false` 尤其不会自行生成番剧规则。

`rss/engine.py:100-125` 返回服务层状态；`api/response.py:7-14` 的 `u_response` 实际仅发 `{msg_en,msg_zh}`，**没有 status 字段**，由 HTTP 状态携带成功/失败。RSS 重复 URL 在 DB 层返回 false（最终 406）；若既有 RSS 被禁用，同 URL add 会重新启用，不能靠“重复添加”当无害幂等。

**RSS 启停（首选最窄统一路径）**：`PATCH /api/v1/rss/update/{id}`，body 仅 `{"enabled":true}` 或 `{"enabled":false}`。数据库 `database/rss.py:52-65` 采用 `exclude_unset=True`，故不会因省略字段覆盖 URL/name/parser。成功 200、缺失 404，响应只有消息。已有专用 disable 是 **PATCH** `/rss/disable/{id}`，enable/many 是 POST + 数组；首版无需使用多条路径。

**Bangumi 规则启用**：`POST /api/v1/bangumi/enable/{id}`，无 body；**禁用**：`POST /api/v1/bangumi/disable/{id}?file=false`，无 body，固定显式 `file=false`。`manager/torrent.py:161-204` 表明修改的是 `deleted=false/true`；只有 file=true 才进一步操作种子/文件，所以绝不允许该参数由客户端随意传入。缺失 ID 为 406，非一概 404。归档字段 `archived` 另有业务意义，不要把恢复 enabled 视为取消 archived。

**明确但首版不开放的动作**：`POST /rss/refresh/{id}` 会拉取 RSS 并可能向下载器提交任务；没有 durable 操作回执，且不存在 ID 也可能 200。`POST /rss/analysis` → `POST /rss/subscribe` 才是完整的按 RSS 分析并订阅番剧流程；后者 body 为顶层 `{data:Bangumi,rss:RSSItem}`，会登记 RSS、创建/恢复规则并下载，不能包装为普通 RSS 登记。它对库、下载器、元数据要求更多，首版保留外部 UI 办理，不启用 generic search。

官方源码：

- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/api/rss.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/models/rss.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/database/rss.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/rss/engine.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/api/response.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/api/bangumi.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/manager/torrent.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/manager/collector.py>
- <https://github.com/EstrellaXD/Auto_Bangumi/blob/4.0.0/backend/src/module/api/search.py>

#### 3.4 不确定写入对账

- RSS add 使用原始 feed URL **精确相等**作为业务身份，读取 `GET /api/v1/rss` 后只在唯一匹配时关联 `rss:N`。不能随意删 query、排序 query、消除 passkey、改 path 大小写；这些改变可能指向不同凭据/资源。原 URL 只存私密意图文件，不进 DTO/日志/普通备份。
- 创建前已存在同 URL 时应在本地明确处理“已有登记”，不能再次 POST 尝试；尤其已有禁用条目，重新添加会改变其启用状态。
- 超时后只有发现唯一同 URL 的实体且 immutable 创建意图要求的字段相容，才可关联；若并发来源无法排除、字段冲突/重复/读取不完整，维持 uncertain，不重发。最低标准应核对 name、aggregate、parser 与 enabled，不能仅标题或 ID 猜测。
- RSS monitor 对比确切 `rss:N` 的 enabled；Bangumi monitor 对比确切 `bangumi:N` 的 deleted 与期望的逆值。对账只 GET，不能通过 enable/disable 或 add 来“探测”。
- 不把队列空数组、downloaded=true、RSS HTTP 刷新成功当作本地文件入库完成；本项目目录导入仍走既有 preview/register/scan。

### 4. 建议能力矩阵

| 能力 | MP v3.2.8 | AB 4.0.0 | 说明 |
|---|---|---|---|
| 连接、版本、基本健康 | 是 | 是 | MP global + ping；AB /health + authenticated /status |
| resources | 是 | 是 | AB 合并但区分 bangumi/rss |
| queue | 是 | 是，有限可见性 | AB 仅 Bangumi 分类、部分实例故障可静默跳过 |
| history | 是 | 否 | AB 没有已核实全局有界等价接口 |
| add | 精确 TMDB/type/season | 仅 RSS 登记 | 不开放完整 AB Bangumi subscribe |
| monitor | R/S | RSS enabled、Bangumi deleted | 禁止 MP P=暂停误映射，禁止 AB file=true |
| search | 仅单订阅 POST | 否 | AB SSE 搜索/刷新不伪装成通用补搜 |
| metadata lookup/options | 本期否 | 本期否 | 无可靠统一契约则由用户给精确 ID/名称或外部 UI |
| 未知/旧主版本写操作 | 否 | 否 | 失败关闭，禁止“相似 API”推断 |

### 5. HTTP fixture 与断言建议

1. MP Probe 精确收到 `X-API-KEY`；global 固定 token 公开常量，ping 必须鉴权；不能访问 env/versions。HTML 登录页、缺失 BACKEND_VERSION、未知主版本、`success:false` 都不能启用写。
2. MP 所有列表用 envelope 包裹；用同形状 HTTP 200 + success=false 验证不会当空列表；裸数组按已核实 MP3 形状不符处理，不能无声兼容未知协议。
3. MP create body 固定 `media_source:"themoviedb"`，media_id 是字符串、type 使用已知中文枚举，电视剧带 season。无状态、用户名、path 等任意字段；收到成功 id 后只映射安全字段。
4. MP monitor 禁用固定 state=S、启用 R；测试 P 不被识别为暂停。search 必须 POST `/search/{positive ID}`，永不命中无 ID 的全局路由；queued=0/ongoing=1 合法。search 断线对账不能靠最近批次猜成功。
5. AB API bearer，而非从登录响应提取 access_token；API token scope 不符/401、db_ok=false、status=false 各自区分。health 单独成功不能证明凭据可用。
6. AB RSS/Bangumi 列表 ID 相同也分别保留 rss:7/bangumi:7。成功体只有消息仍被接收；401/404/406 等确定拒绝不当 uncertain，500/网络断线等保守 uncertain，禁止自动重发。
7. AB add 仅 RSS 登记、精确 body；响应无 remote ID 时 GET rss 对账；模拟“同 URL 已禁用，add 会启用”确保实现先读，不把重复请求当纯幂等。超时后唯一 URL + 相容字段可收敛，否则 uncertain。
8. AB RSS 启停只 PATCH enabled；Bangumi POST disable 显式 file=false；拒绝传入任意字段/方法/path/文件删除。不存在实体不记为 monitor 成功。
9. 数据/大小预算：本地最大 4 MiB、上游最多 1000 条；MP 分页须完整或显式提示失败，AB 未分页数组超限则明确失败。不可先截断再把“未找到身份”当作安全创建条件。
10. 返回白名单：响应插入 RSS passkey、URL、私有目录、上游异常文本、恶意 HTML，确认均不回显；API credential 不进入日志/DB。禁止 redirect，显式私网授权，重启后 dispatching→uncertain，Reconcile 全程只 GET。

### 6. 本地相关文件与模式

| 文件 | 一行说明 / 已有模式 |
|---|---|
| `.trellis/spec/backend/collection-contract.md:14,24-28` | 来源 revision、秘密分离、任务 requestId 唯一、既有独立导入所有权 |
| `.trellis/spec/backend/runtime-contract.md:20,34` | 管理员 session、生命周期取消与退出边界 |
| `internal/app/collection_models.go:15-27,41-45` | external 类型、公开入口、allowPrivate、revision 与 hasSecret；不能把秘密放普通 DTO |
| `internal/app/collection_api.go:91-186` | 来源保存校验及秘密保留行为；external URL 仍是公开 UI 入口 |
| `internal/app/collection_store.go:58-107` | 私密文件命名与读写边界；连接及 immutable 操作意图应复用同级模式 |
| `internal/app/collection_rss.go:90-99` | 已有来源网络私网授权及客户端边界可供适配器 HTTP transport 参考 |
| `internal/app/collection_preview.go:19` | 目录授权独立于外部 API 根路径；远程返回目录不是本地授权 |
| `.trellis/tasks/11-20-collection-external-api/prd.md` | 用户本轮深度 API 目标与验收范围 |
| `.trellis/tasks/11-20-collection-external-api/api-contract.md` | 实施调度/私密连接/持久动作契约，R/S 与 AB RSS登记边界已与本报告对齐 |

## Caveats / Not Found

- 仅研究，无真实 MP/AB 实例调用，无真实订阅/下载/部署，fixture 不能替代用户环境联调。反代子路径和不同镜像构建版本需要实例探测验证。
- 仅固定 tags 是已审查依据；未取得完整提交 SHA（GitHub REST 限流），未宣称所有 MP3/AB4 小版本都完全一致。
- 上游返回的错误与健康文案可能带秘密；本项目应固定错误分类，不透传。
- MP 查询接口可能使用身份投影/元数据回退；本地对账必须重新检查完整媒体身份。MP3 正规 TMDB 标识为 themoviedb，暂停 S。
- AB4 成功体缺少 status 是源码事实；部分失败也只含消息。判断须结合 HTTP 状态，不要匹配自然语言消息。
- AB 全局 history、具有稳定请求关联 ID 的通用 search/refresh 回执未核实，默认禁用；AB Bangumi 完整新增虽有端点但首版不开放。
- 当前角色为 researcher，生产代码写入禁止；此文件供 implement agent 使用。本报告不授权移动现有未提交实现、接管外部下载器或更改其它任务文件。
