# Research: Huangguo 搜索、分集身份与安全 HLS 接入

- Query: 用户明确要求 Huangguo 站点接入；固定上游版本如何搜索和分页、标识实际分集、解析详情及匿名播放、限制 CDN 鉴权头，并安全复用本项目的公网代理、yt-dlp 原生 HLS 与离线 FFmpeg？
- Scope: mixed；固定上游源码及其测试的静态审阅，本项目网络和收集契约的定向核对。
- Date: 2026-11-25
- Status: 这是对 `uav-upstream.md` 原先“不纳入简单全局请求头方案”的补充。用户追加的 Huangguo 必须接入，原先的站点排除建议不再适用；安全网络边界仍需保留。

## Findings

### 1. 来源、版本及阅读边界

- 仓库：`https://github.com/Alos21750/UAV-Downloader`。
- 本研究只针对提交 `d40f09457fbd38dd7dc94580637242afafaf3939`，声明版本 `3.8.4`、Python `>=3.12`、Apache-2.0。提交时间 `2026-11-25T00:41:26Z`；不能由此推断任意后续版本行为相同。
- 固定归档：`https://codeload.github.com/Alos21750/UAV-Downloader/tar.gz/d40f09457fbd38dd7dc94580637242afafaf3939`，SHA256 `88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41`，大小 1,952,930 字节。
- 只从 GitHub/raw/codeload 读取源码及测试，不执行下载的代码，不访问视频站、不下载真实视频、不导入浏览器会话。归档测试只作契约证据，未运行。

### 2. Files found

| 文件 | 用途 |
| --- | --- |
| 上游 `src/uav_downloader/sites/huangguo.py` | 黄果 listing、分集展开、匿名播放、CDN 请求头、重定向及 TS 限制的主要实现。 |
| 上游 `src/uav_downloader/core/video_identity.py:103–125` | 规范站点/系列/集数身份；根页只代表第一集。 |
| 上游 `src/uav_downloader/base.py:850–898` | 普通 identity AES-128 key/IV 规则；key 下载通过站点 `_media_get`。 |
| 上游 `tests/test_huangguo.py` | CDN 作用域、重定向、布局错误/空页、展开预算、fMP4 拒绝的静态测试证据。 |
| 上游 `tests/test_huangguo_playback.py` | 匿名 POST、无 Cookie/Authorization、每作业刷新及代理/deadline 契约。 |
| 上游 `tests/test_huangguo_metadata.py` | 元数据缓存排除播放签名与鉴权信息，取消/代理快照和有界重试。 |
| 上游 `tests/test_huangguo_apps.py`、`test_huangguo_integration.py`、`test_huangguo_collection.py` | 分集身份与收集流程的额外静态证据。 |
| 本项目 `internal/collectionworker/proxy.go:40,111,194–221` | 现有公网 DNS/连接策略；HTTPS CONNECT 为透明隧道，不能注入路径级鉴权头。 |
| 本项目 `internal/collectionworker/uav_bridge.py:162,215,250–267` | 本次正在实现的搜索、有限 HLS 解析和私有解析响应；当前 generic `_extra_headers` 不足以支持黄果。 |
| 本项目 `internal/collectionworker/uav.go:29–37,162–193,210–270` | 私有媒体 DTO、解析核验、原生 HLS 下载及离线封装/完整解码；行号是研究时工作区快照。 |
| 本任务 `research/integration-architecture.md` | 既有来源、持久任务、取消、发布和入库的复用方案。 |

### 3. 搜索、listing、分页与实际分集

固定根域名为 `https://huangguoai.com`，允许的站点主机是根域与 `www` 形式。`HuangguoBrowser.search_url` 将非空词编码为 `/search/video/{quote(query,safe='')}/`；空词返回首个分类 `/ai-duanju/`（`huangguo.py:723–731`）。

`page_url` 第一页不带页码，第二页为 `/search/video/{query}/2/`；数值搜索词不是页码，不能直接删掉路径最后一个数字（`huangguo.py:711–720`）。应用当前若要求非空搜索词，应保留自身契约，不必扩展为全分类浏览。

`_parse_videos` 识别 `main .hg-list-page`，不存在识别出的 listing 容器是布局错误；存在受支持的空容器才是正常空结果（`huangguo.py:500–525`）。系列卡片来自 `main .hg-list-page .hg-card-grid .hg-drama-card`，只读取主列表/`latest`，忽略热门推荐和建议；封面链接为 `a.hg-drama-card__cover-link[href]`，标题为 `.hg-drama-card__title a`。

`fetch_page` 不是简单系列卡片列表。它对每张实际卡片调用 `fetch_episodes`，将所有实际公开集数扁平展开，按规范分集 URL 去重（`huangguo.py:677–700`）。`fetch_episodes` 读取系列根页，使用 `[data-web-play] a.hg-web-play__ep[data-ep-id][href]`，要求 URL 同系列、URL 集号与 `data-ep-id` 一致，按集号数值排序；只返回观察到的集链接，不推测缺号、不按总集数合成 URL（`huangguo.py:640–674`）。

每个结果形如：

```json
{"url":"https://huangguoai.com/video/42/ep-3/","title":"示例标题 - EP 3","thumbnail":"","duration":"","date":"","author":"","episode":3,"series_id":"42"}
```

上游边界为每页最多 100 个系列、每系列 1000 集、展开页 5000 集、展开总时限 120 秒。超过限制明确报错，不能返回截断的“成功”页；本项目搜索共享 30 秒预算、公开结果最多 100 条，与完整上游页展开存在实际冲突。

接入必须显式选择受支持的有界行为：

1. 保留扁平实际分集语义时，传入不晚于应用操作的 deadline，超过 100 结果/时间预算明确失败并给出缩小搜索的中文提示。优点是改动最少；缺点是大结果页可能不可用，不能称已支持任意大页。
2. 若需要可靠覆盖大页，扩展受控的系列选择/分集分页协议，或将已完整观察的系列/集快照做应用级有限分页；分页身份必须明确区分上游页与本地集页。先完整有界观察再切页，不能先截断后声称没有更多。
3. 仅返回系列根页是可定义的 EP1 搜索策略，但必须在 UI/契约明确“第一集”，不能冒充 `fetch_page` 的完整分集结果，也不能让用户以为选择系列会整季下载。

不要把 5000 条切成前 100 条并返回普通上游 `page`，不要把未知布局或超时吞成空页，也不要伪造总数/总页数。

### 4. 规范 ID 与详情身份

`core/video_identity.py:103–125` 仅接受 HTTPS、允许主机、无用户凭据、无 query/fragment、无反斜线/控制字符、默认端口或 443。路径必须匹配 `/video/([1-9][0-9]*)(?:/ep-([1-9][0-9]*))?/?`。

- 稳定上游 ID：`huangguo-{series}-ep-{episode}`。
- `/video/42/` 与 `/video/42/ep-1/` 都代表 EP1；规范 URL 为 `/video/42/`。
- EP>1 规范 URL 为 `/video/42/ep-3/`；每集独立，标题相同也不能合并。
- 播放端再限制系列 ID 长度 <=19、值 <=9,223,372,036,854,775,807，集数 <=2,147,483,647（`huangguo.py:387–389`）。

建议本项目沿用上述完整站点前缀 ID，或使用同等严格且一一映射的 `{series}-ep-{episode}`；必须把站点和集数带入预览、不可变操作、摘要和重试副本。系列根页不意味着批量取得其他分集。

### 5. 详情解析与匿名 playback

`SiteHuangguo` 仍继承 `M3U8Crawler`；构造期间会解析网络详情，并非完全无副作用的纯对象创建。与其他站点一样可固定 `savepath`、`silence=True,max_workers=1,resolution="highest"`，只用解析结果，不调用上游完整下载流程。应核验 `_last_error` 与 `is_url_vaildate()`（原上游命名）。

`get_url_infos` 的顺序（`huangguo.py:536–577`）：

1. 规范分集 URL，读取公开详情页。
2. 解析 `script#videoInitialData[type="application/json"]`；最大 1 MiB，页面 id/ep 必须匹配目标（`huangguo.py:360–373`）。
3. `videoApi`、`videoKey` 或 `videoIv` 非空时拒绝，不能进入私有播放器/边缘解密协议。
4. SSR 的 `videoSrc` 为空时调用公开匿名播放元数据，不能直接判定没有媒体。
5. 要求有效标题和公开 HTTPS `.m3u8` 源；源可含临时签名 query。
6. 禁止获取黄果缩略图：上游 `_imageUrl=None`，因为它采用独立加密图片协议；不要将密文字节当 JPG。

`_playback_data`（`huangguo.py:376–446`）使用：

```text
POST https://huangguoai.com/api/videos/42/episodes/3/playback
Content-Type: application/json
Referer: https://huangguoai.com/video/42/ep-3/
Origin: https://huangguoai.com
Body: {}
```

请求带固定 User-Agent，不带导入的 Authorization；会话 Cookie 在前后清空。`allow_redirects=False`，流式读最大 256 KiB，整体 25 秒、单次 timeout <=15 秒。401/403 或挑战是拒绝，不能触发登录/绕过流程；暂态错误与格式错误显式区分。要求外层 `status==1` 或 `code==200`；data 必须匹配 id/ep，`manifest_method=="GET"`，streams/cdn 均为 dict。

示意响应字段（值为研究用占位）：

```json
{"code":200,"data":{"id":42,"ep":3,"manifest_method":"GET","streams":{"main":"https://cdn.example.test/index.m3u8?sig=PRIVATE"},"cdn":{"m3u8Enabled":true,"otherEnabled":false,"authKey":"x-cdn-auth","authValue":"PRIVATE","app":"150"}}}
```

签名 URL、authValue 与 app 仅存在解析/下载操作私有内存或私有进程管道，不进入浏览器、公开预览、SQLite 公共字段、日志或持久下载描述。测试 `test_huangguo_playback.py:62–80,127–147` 验证无 Cookie/Authorization、每个作业重新获取、代理/deadline 保持。没有证据支持跨作业复用签名或把它绑定到浏览器会话。

### 6. CDN 头的精确作用域

黄果不将 CDN 头放在通用 `_extra_headers`。只读取该属性会漏掉鉴权；直接把 `_cdn` 变成 yt-dlp 全局 `http_headers` 又会泄露鉴权。

内部 `_cdn` 为 `{auth,app,manifest,other}`，`_media_origin` 为最初解析出的 HLS 源 `(hostname.lower(),port or 443)`。authKey 忽略大小写后只能等于 `x-cdn-auth`；auth 不能为空且 <=2048 字符，app 不能为空且 <=64；二者都不能含 `[\x00-\x20\x7f]`。enabled 字段必须是真实布尔值（`huangguo.py:547–576`）。

`_media_headers(url)`（`huangguo.py:579–591`）总是先校验 HTTPS，并给 `Referer=规范分集URL`、`Origin=https://huangguoai.com`。仅在目的 origin 与最初媒体 origin 完全相同，且资源类型启用时，同时加 `x-cdn-auth` 与 `x-cdn-app`：

| 目标 | 是否发送 CDN 头 |
| --- | --- |
| 同 origin、路径 `.m3u8`、manifest=true | 是 |
| 同 origin、路径 `.ts/.m4s/.key`、other=true | 是；但该版本 HLS 随后仍拒绝 fMP4 |
| 同 origin、对应开关=false | 否 |
| 跨 origin、同名扩展 | 否 |
| 同 origin、`.jpg` 或无扩展 | 否 |

判断使用 `urlsplit(url).path.lower()`，签名 query 不属于扩展名。端口属于 origin，不能仅比较域名后缀，也不能把初始 `.m3u8` 的鉴权资格传播到所有子请求。

`_get`（`huangguo.py:266–324`）手动处理最多 6 次重定向，逐跳重新验证公网目的地、调用 header factory；因此跨域/资源类型变化时自动剥离 CDN 头。公开站点页面重定向另外受站点/路径白名单限制。`_verify_public_destination` 会解析 DNS 拒绝非 global IP。本项目必须保留自身实际连接时 DNS/固定 IP 核验，不能只信任 Python 的预检查。

### 7. 片段格式与加密边界

- 黄果没有覆盖 `_transform_segment`，没有 SupJav 式伪 PNG 前缀剥离；不应额外猜测/清洗片段头。
- 该版本只接受普通 MPEG-TS HLS。任一 segment 有 `init_section`，或 URI path 以 `.m4s/.mp4` 结尾就拒绝（`huangguo.py:597–609`）。头策略允许识别 `.m4s` 不表示当前支持下载 fMP4。
- 继承的普通 HLS AES-128 只允许 keyformat 为空或 `identity`、METHOD 为 NONE/AES-128；key 必须 16 字节，显式 IV 验证，未给 IV 时采用 media-sequence+segment-index（`base.py:850–898`）。key 获取同样走 `_media_get`，所以必须执行同源/扩展名/otherEnabled/重定向头作用域。
- 本项目还需维持自身有限完整清单政策：ENDLIST、非空有界 segment、无缺片/不完整片段、拒绝直播、递归或无界 playlist、LL-HLS、未知加密及无法验证的特殊特性。不能通过允许联网 FFmpeg 绕开。
- 详情出现私有 `videoKey/videoIv/videoApi` 与普通 HLS AES-128 是不同情况；前者拒绝，后者可在严格标准 HLS 路径中支持。

### 8. 可落地的下载接入：操作级固定资源代理

现有 `publicProxy` 的 HTTPS CONNECT 是透明隧道：它看到的是主机/端口，看不到 TLS 内资源路径，不能安全实现“只对特定 `.m3u8/.ts/.key` 注入头”。因此不能只把 `x-cdn-*` 放到全局 yt-dlp 请求头，也不应尝试在现有 CONNECT handler 中按 URL 路径改头。

推荐一个只服务单次下载的固定资源代理，复用现有 `publicDial` 发起真实 HTTPS 请求：

1. Python 解析返回规范身份、标题、临时媒体 URL、`Referer/Origin`，以及严格结构化的 Huangguo CDN 策略；该 DTO 仅 worker 私用。Go 重新验证 tokens、flags、HTTPS 和初始 media origin，不接受任意 header 名/任意站点。
2. 取 master/选定 media 清单时由同一有界资源客户端处理，手动重定向、逐跳重新计算 CDN 头，并在实际 Dial 时执行 `publicDial`。优先只选择一个有限清晰度，禁止子清单递归爆炸。
3. 完整解析并验证选定有限清单。将它引用的每个 segment/key URI 解析为原始绝对 HTTPS 地址，建立有限、不可变的操作内资源表；资源身份是生成的随机/不可预测句柄，不是客户端传入的任意 URL。
4. 将清单所有引用重写为 `http://uav.invalid/<operationNonce>/<opaqueResourceId>`；可使用生成的 `.ts/.key/.m3u8` 后缀便于工具识别。虚拟域不做 DNS/真实网络连接，代理只接受该操作准确 origin、nonce、句柄和允许方法。
5. 仅本次 yt-dlp 通过显式 `--proxy <操作代理地址>` 使用该代理；代理认证、连接生命周期和 nonce 都是单次操作所有。拒绝 CONNECT、任意外部目标、未知资源、query 注入、未知 Range 语义、超量请求；不能把它做成通用 HTTP 转发服务。
6. 代理自身根据不可变资源表取得原始 HTTPS 资源，逐跳应用源域/类型的 CDN 策略，限制总实际字节、每资源大小、并发、重试和超时。key 大小必须精确 16 字节；下载不完整、超预算、取消或缺片均终止，不发布。
7. yt-dlp 接收只含虚拟资源引用的私有 info/清单，通过原生 `m3u8_native` 下载；禁用 FFmpeg downloader 回退。完成后沿现有流程离线 remux+全程 decode，只留下固定 `media.mp4`，再走原有检查/发布。
8. 操作取消或 worker 关闭同时关闭代理 listener、现有连接和上游 response；过期资源句柄失效。预览不创建持久代理，也不把 transient token 绑入预览 identity；真正下载时重新解析并核对规范身份。

这条路径不需要放宽现有公网代理的私网/loopback 禁止，也不需要 MITM 证书。`uav.invalid` 只是生成的内部资源标识，不能被用户配置或当作外部站点请求。显式 yt-dlp proxy 参数需要确认覆盖其进程通用代理环境；未证明这一点前，不能称已形成闭环。

备选是认证 loopback 资源 relay 配合专用受限 runner；只有当所有原始引用均已移除、操作只可访问生成资源、且不会全局放宽 publicProxy 私网策略时才成立。优先采用一种明确实现，避免同时维护两套网络路径。

### 9. 最低验证矩阵

| 输入/故障 | 预期 |
| --- | --- |
| 根系列 URL、显式 EP1、EP3 | 前两者同规范 EP1；EP3 独立 ID/预览/任务，绝不批量下载其他集 |
| 数值关键词与搜索页2 | 关键词保留，只有实际页后缀被替换 |
| 未知布局、已识别空容器 | 前者错误，后者空数组；失败不伪装空页 |
| 非连续 EP1/EP3、其他系列链接、`data-ep-id` 不一致 | 只保留真实有效同系列链接，不合成 EP2 |
| 扩展分集 >100 或超过应用 deadline | 明确预算错误/受支持的分页；无静默前100条 |
| SSR videoSrc 为空 | 调公开匿名 POST；无 Cookie/Authorization，API 重定向拒绝 |
| API id/ep 不符、manifest_method!=GET、无效布尔/token/私有播放器字段 | 解析失败，不下载、不泄露原始 payload |
| manifest=true/other=false | 同源清单带 CDN 头，片段/key 不带 |
| manifest=false/other=true | 清单不带，符合扩展名同源片段/key 带 |
| 同域不同端口、跨域重定向、同源不同扩展 | 逐跳剥离 CDN 头；始终执行真实公网连接校验 |
| 私网/metadata/IP literal、DNS rebinding、重定向环 | 连接前拒绝，或在有界跳数内失败 |
| AES-128 标准 key/IV、非16字节key、其他KEYFORMAT/METHOD | 标准可支持；无效及未知加密失败 |
| fMP4、MAP、LL-HLS、直播、无ENDLIST、缺片/截断 | 拒绝并阻止发布 |
| relay 未知句柄/目标、CONNECT、资源重放/超量Range/超预算 | 拒绝，不能成为通用代理 |
| 取消、进程退出、worker关闭 | 中止上游与 relay；无悬挂进程/凭据/可访问代理 |
| 凭据/签名含敏感标记 | 浏览器、日志、事件、普通DB/公开预览均没有标记 |
| 同一集再次下载或 preview后下载 | 每操作重新解析播放元数据，仍绑定原站点/集身份 |
| 下载/remux/decode异常 | 任务失败、无发布；原有入库覆盖验证不变 |

## External references

下列引用均固定到上述提交；行号只对该提交有效：

- `https://github.com/Alos21750/UAV-Downloader/blob/d40f09457fbd38dd7dc94580637242afafaf3939/src/uav_downloader/sites/huangguo.py`
- `https://github.com/Alos21750/UAV-Downloader/blob/d40f09457fbd38dd7dc94580637242afafaf3939/src/uav_downloader/core/video_identity.py#L103-L125`
- `https://github.com/Alos21750/UAV-Downloader/blob/d40f09457fbd38dd7dc94580637242afafaf3939/src/uav_downloader/base.py#L850-L898`
- `https://github.com/Alos21750/UAV-Downloader/blob/d40f09457fbd38dd7dc94580637242afafaf3939/tests/test_huangguo.py`
- `https://github.com/Alos21750/UAV-Downloader/blob/d40f09457fbd38dd7dc94580637242afafaf3939/tests/test_huangguo_playback.py`
- `https://github.com/Alos21750/UAV-Downloader/blob/d40f09457fbd38dd7dc94580637242afafaf3939/tests/test_huangguo_metadata.py`

## Related specs

- `.trellis/workflow.md`：研究持久化、实现/检查代理以及任务步骤。
- `.trellis/spec/backend/collection-contract.md`：来源、不可变持久任务、产物检查、发布与独立入库。
- `.trellis/spec/backend/jmcomic-contract.md`：可选固定上游 Python 桥接、受控执行、公网代理、敏感值隔离及真实验证边界。
- `.trellis/spec/backend/download-contract.md`：有限资源预算、实际产物验证、传输与入库相互独立。
- `.trellis/spec/backend/external-collection-contract.md`：受控 DTO、逐目的地网络策略、不得将原始凭据回传浏览器；其服务特定 API 流程不直接套用于黄果。
- `.trellis/spec/backend/runtime-contract.md`、`cross-platform-contract.md`：进程/环境归属及兼容性。
- `.trellis/tasks/11-25-uav-downloader-plugin/prd.md`：用户追加 Huangguo 的明确范围、固定提交与不放宽原有网页插件策略。

## Caveats / Not Found

- 本文是静态源码和测试契约审阅，没有证明黄果真实站点当前可访问、公开 API/签名可用、CDN 支持范围请求或特定地区可下载。真实公网访问、Docker/NAS 与授权样本下载需分别标注证据。
- 没有发现黄果需要伪图片片段转换、浏览器 Cookie/账号或私有播放器解密；不能将“没发现”转化为对未来站点版本的保证。
- 操作级固定资源代理是本项目安全接入建议，不是现有代码已实现的能力。实现后必须验证所有清单引用已重写、鉴权逐跳作用域及运行环境不能绕开该代理。
- 搜索大页/大量分集与当前 100条/30秒协议的冲突必须在实现和使用说明中明确处置；上游支持5000条不等于本项目可安全一次返回5000条。
- 正常 AES-128 支持应以实际锁定 yt-dlp 原生 HLS 的离线 fixture 验证，不能只靠上游 base.py 支持就声称本项目已经支持。
- GitHub 页面抓取工具无有效正文；使用 raw GitHub 与 codeload 归档只读内容复核。GitHub API 曾触发匿名限流，未将其失败解释为仓库缺失。
