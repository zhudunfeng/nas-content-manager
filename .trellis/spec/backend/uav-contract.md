# Fixed UAV Video Plugin Contract

## 1. Scope / Trigger

The optional `uav` collection source adds a fixed Python integration across the Vue UI, Go application, worker, process boundary and deployment image. Read this contract with [Collection contract](./collection-contract.md). It does not introduce a plugin upload/command interface or a database migration: SQLite remains schema 6.

Supported sites are **jable, missav, hanime1, huangguo and supjav**. The source identity is UAV-Downloader 3.8.4 at commit `d40f09457fbd38dd7dc94580637242afafaf3939` (Apache-2.0). The source archive SHA-256 is `88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41`. Never fetch mutable upstream code from a request or worker startup.

Huangguo results represent individual episodes. A series is expanded within the search budget; exceeding the supported result/time bounds must fail explicitly rather than silently omit episodes. Account, Cookie and paid-access workflows, other upstream sites, the upstream GUI/queue and recursive batch downloads are outside this adapter.

## 2. Signatures

```go
// NormalizeUAVSearch 统一应用与执行端的关键词、站点和分页校验。
func NormalizeUAVSearch(input UAVSearchRequest) (UAVSearchRequest, error)
// ValidateUAVSearchResponse 校验严格有界的桥接结果。
func ValidateUAVSearchResponse(input UAVSearchRequest, output UAVSearchResponse) error
// ValidUAVSite 只接受五个固定站点。
func ValidUAVSite(site string) bool
// ValidUAVVideoID 校验站点对应的规范视频或单集编号。
func ValidUAVVideoID(site string, id string) bool
// normalizeSupJavTS 在原生解密后原地清理已知PNG包装，返回实际删除字节数。
func normalizeSupJavTS(ctx context.Context, name string) (int64, error)
```

- Application: `POST /api/collection/sources/{id}/uav/search` (existing session and same-origin protection).
- Worker: `POST /v1/uav/search` (existing worker Bearer token).
- Existing preview endpoint: `POST /api/collection/previews`.
- Existing durable submission endpoint: `POST /api/collection/tasks`.
- Existing task detail/events/actions, library scan and authenticated file Range endpoints remain authoritative.
- The embedded Python bridge accepts one bounded JSON input on stdin with action `probe`, `search` or `resolve`; only the trusted interpreter is executable. It runs with `-I -B -X utf8 -c <embedded source>`.

## 3. Contracts

### Source, search and canonical identity

`kind:"uav"` sources accept their name/enabled state and fixed video profile. They reject URL, root/path, `allowPrivate`, external connection, header, proxy, command and executable fields. Changing kind must clear hidden fields in the source form.

```json
{"site":"huangguo","query":"fixture","page":1}
```

```json
{"site":"huangguo","query":"fixture","page":1,"hasNext":false,"items":[{"videoId":"huangguo-42-ep-3","title":"样例 第3集","duration":""}]}
```

- Query: trimmed, 1–120 Unicode characters, valid UTF-8, no controls.
- Page: 0 defaults to 1; otherwise 1–1000.
- Response: exact site/query/page echo, at most 100 unique valid IDs, UTF-8 text only; no media URL, thumbnail, credential or invented `total`.
- `duration` is a string, at most 40 bytes. Search/preview title validation allows 1200 UTF-8 bytes; the worker applies existing `safeTitle` normalization (at most 240 characters). Do not apply the older generic preview limit of 512 bytes to UAV Chinese titles. Other source kinds retain their existing limit.
- `hasNext` comes from a real next-page anchor; result count is not evidence of a next page. Empty success is `items:[]`. Unknown markup/transport failure is an error.
- Jable: lowercase ASCII slug `[a-z0-9][a-z0-9_-]{0,199}`.
- MissAV: ASCII slug of the same maximum length, containing a digit; `dm` followed only by digits is excluded as a category.
- Hanime1: `[1-9][0-9]{0,11}`.
- SupJav: `[1-9][0-9]{0,11}`. Canonical URL is `https://supjav.com/<id>.html`; accepted search URLs may have a single `zh` or `ja` path prefix. Credentials, ports, queries, fragments, categories and noncanonical numbers must not become video IDs.
- Huangguo: `huangguo-<series>-ep-<episode>`; positive decimal values without leading zeros, series ≤9223372036854775807, episode ≤2147483647. EP1 uses the upstream series root URL; later episodes use `/video/<series>/ep-<episode>/`. Bare series IDs are invalid. Search expands actual episode links, never synthesizes missing episode numbers.

### Preview, submission and persistence

```json
{"sourceId":"<source>","uavSite":"huangguo","videoId":"huangguo-42-ep-3"}
```

The existing preview response fixes `kind:"uav"`, `profile:"video"`, `files:[]`. It retains source revision, worker identity, resource budgets, expiry and canonical content identity. Preview resolves metadata, not the video bytes. Canonical identity binds site, video ID and safe title, not expiring signed URLs.

```json
{"requestId":"<stable-uuid>","previewId":"<preview>","contentType":"video","createLibrary":true,"libraryId":0}
```

Only video targets are accepted. An accepted UUID must resolve before checking preview expiry, including after application/worker restart. A submission whose outcome is unknown retries the identical UUID and body; it must not generate a fresh UUID. Known failed execution uses the existing task retry action. Import retry must reuse the published files.

Private request/snapshot structs carry `uavSite,omitempty` and `videoId,omitempty` through preview to operation. The `omitempty` and field placement preserve old source kinds' serialized request bytes and hashes. Cross-kind selectors (e.g. `albumId` on UAV, UAV fields on web) are rejected. No media link or CDN access value enters durable task metadata.

### Private bridge and acquisition

- Probe returns `{version:"3.8.4",commit:"d40f09457fbd38dd7dc94580637242afafaf3939",protocolVersion:1}` after checking the trusted environment; it does not contact a site.
- Search/resolve receive the worker-owned authenticated public proxy. Explicit upstream direct-proxy defaults must be overridden for requests, cloudscraper and curl-cffi. Clear inherited proxies, credentials and user configuration using the existing process environment boundary.
- Resolve returns private `{site,videoId,title,mediaUrl,headers,manifest?}`. `headers` only permits URL-valued Referer/Origin. Site and video ID must match the input; the worker revalidates output independently. Raw stdout/stderr, HTML, query strings and credentials are not logged or returned to the browser.
- HTTP(S) direct media is supported where appropriate. HLS requires HTTPS resources and finite ENDLIST only, at most 512 KiB of playlist text and 20000 segments; master selection uses the highest supported bandwidth. Reject live, separate audio, missing fragments, GAP, delta/SKIP and LL-HLS control tags, unknown extension tags and unsupported encryption. Normalize the validated playlist with the pinned m3u8 serializer before resource rewriting. AES-128 requires an exact 16-byte key and valid URI/IV; DRM/SAMPLE-AES is rejected. Huangguo supports ordinary MPEG-TS HLS and rejects fMP4/init-map media.
- The fixed yt-dlp process receives `--load-info-json -`, a fixed `media.download` path, native HTTP/HLS only and inline `hls_media_playlist_data` when present. It must not refetch a different/live playlist or fall back to network FFmpeg. `--fixup never` and a nonexistent operation-owned `--ffmpeg-location` enforce the latter.
- After acquisition, FFmpeg remuxes the local file to `media.mp4`, verifies a video stream and fully decodes it. HLS must reach the sum of finite positive EXTINF durations minus `max(0.5 seconds, 2% of expected duration)`, using decoded `out_time_us`; clean packet-boundary truncation can otherwise still decode successfully. This lower bound detects obvious missing tails, but does not prove semantic completeness or losses within tolerance. Direct MP4 has no trusted expected duration here: successful HTTP transfer and decode alone cannot prove the upstream original's full length. Its protocol whitelist is `file,pipe,crypto,data`; it does not receive remote input. Reserve half the task byte budget for the temporary original plus remux copy. Delete the original after successful processing; exactly one regular MP4 must pass the existing independent manifest and publication gate.
- All processes retain operation cancellation/timeouts, process-tree ownership, concurrency slots, disk reserve, staging/file/byte budgets and no-overwrite publication. Generic web acquisition format/protocol restrictions must remain unchanged.

### SupJav candidate fallback and local TS normalization

The fixed provider order is EVS/FST/VAS/LUC/VOE/TV, followed by ST progressive MP4. Select each candidate with its own permitted Referer/Origin; ST uses `_direct_referer`, never the first HLS candidate's headers. FST URLs are HLS even when they end in `.txt` or have no extension. Each candidate must pass the existing finite-manifest validator. SupJav additionally rejects init maps/fMP4 and BYTERANGE.

Only the private bridge uses `sourceOffset` (integer 0–6 on SupJav resolve input) and `sourceIndex` (selected provider slot, 0–6 on resolve output). The worker independently checks monotonic progress and format/slot compatibility; these selectors are not public inputs, identity components or durable fields. After a candidate transport/content failure, release its relay scope, verify staging budgets, delete only the expected owned regular downloader/remux files, check disk reserve and resolve the next slot. Never splice partial candidates or restart timeout/budget accounting. Cancellation, storage/budget failures, unexpected staging entries, process cleanup failures and changed identity terminate without fallback. A task retry retains the existing durable operation.

Keep AES keys and IV handling in the locked native downloader. It decrypts and removes padding before concatenating each fragment. Before offline FFmpeg, normalize **only SupJav HLS** locally: process complete 188-byte TS packets; unknown corruption and partial trailing packets fail. Only an exact PNG signature at an expected packet boundary may trigger resynchronization, at most 8000 bytes forward, followed by five valid TS packets. Require valid sync, transport error and adaptation/payload control fields: adaptation-only length must equal 183, while adaptation with payload must have length ≤182. Do not treat arbitrary corruption as removable metadata. Use a fixed buffer and `WriteAt` to compact the operation-owned file without a second full-size TS copy, then truncate only after successful EOF. Check cancellation throughout; local write, truncate, sync and close failures terminate without fallback. Continue through the existing full decode, positive-frame and duration gate before publication.

This validates TS structure and received decodable content, not content authenticity. In particular, the pinned downloader's padding removal does not validate every PKCS#7 padding byte; a changed nonfinal padding byte which leaves identical TS plaintext cannot be detected by the local normalizer. Do not claim strict AES padding validation or semantic completeness.

### Finite HLS relay and Huangguo CDN scope

Huangguo requires finite HLS and may return private `cdn:{origin,auth,app,manifest,other}`. `origin` is an exact HTTPS origin derived from the original media source; `auth`/`app` are bounded printable access values. `manifest` permits `.m3u8`; `other` permits `.ts`, `.m4s`, `.key`. Header keys are fixed `x-cdn-auth` and `x-cdn-app`.

For every HLS download, the worker registers only resource URIs present in the resolved finite playlist (segments and URI attributes), replaces them with unguessable operation-scoped `http://uav-media.invalid/<scope>/<resource>` addresses, and removes original media/CDN values from the downloader intent. The existing authenticated proxy intercepts this virtual host. It is not a DNS target or a public API, and unknown paths cannot request arbitrary destinations. AES-128 key resources are fully read within a 17-byte probe bound and returned only when exactly 16 bytes; do not rely on yt-dlp's more permissive AES key acceptance. Other resource transfer errors or unknown-length budget overflow must abort the response connection, never finish a truncated response normally.

Each registered resource is fetched with normal TLS validation and existing `publicDial` public-address/port rules. Every redirect is bounded and independently validated; CDN headers are recomputed from the original exact origin and current resource extension at every hop. Cross-origin/subdomain/HTTP/unapproved-extension requests never inherit access values. Never attempt header injection through opaque HTTPS CONNECT tunnels. Cancel/finish unregisters the operation scope; canceled requests cannot outlive it. For HLS BYTERANGE, require a single explicit closed byte range and a matching 206 / Content-Range / response byte count. A CDN returning 200 while ignoring Range is failure, not a complete fragment. Reject unexpected partial 206 replies to non-Range requests.

When serving a bounded range, withhold its final byte until reading the upstream EOF proves the exact response length. Otherwise a chunked overlong source can satisfy the downstream Content-Length before the relay detects excess bytes, and the native downloader may report success despite a later aborted connection. Both short and overlong bodies must leave downstream framing incomplete and fail the actual downloader.

### Environment and packaging

| Key | Default / meaning |
| --- | --- |
| `NAS_COLLECTION_WORKER_UAV_ENABLED` | Native `false`; explicit opt-in. The dedicated image contains fixed dependencies and enables it. |
| `NAS_COLLECTION_WORKER_UAV_PYTHON` | Native `python3` (`python` on Windows); trusted Python ≥3.12 environment. Container `/opt/uav/bin/python`. |
| `NAS_COLLECTION_WORKER_UAV_RUNTIME_MANIFEST` | Optional Windows private runtime manifest absolute path; managed mode uses the trusted package rather than system Python. |

Reuse existing worker yt-dlp, FFmpeg, token, roots and resource-limit configuration. UAV Python is separate from the JMComic environment. `deploy/uav-requirements.txt` and `uav-build-requirements.txt` pin the headless runtime/build closure and hashes; install only at deployment/build time. The upstream pyproject also lists GUI/AI extras deliberately absent here: a successful headless import/probe is not a claim that a full upstream `pip check` passes. Include upstream LICENSE/attribution in distributed packages.

Windows private distribution, explicit interpreter precedence, local diagnostics,
multi-plugin desktop status and release schema 3 additionally follow the
[Windows UAV contract](uav-windows-contract.md).

## 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| No application session / bad worker token | 401; no site request |
| Foreign Origin | 403 |
| Unknown site/field, invalid query/page/ID, mixed selectors | 400 |
| Missing or wrong-kind source | 404 |
| Disabled/changed source | 409; check again after network completion |
| Expired/stale preview or duplicate content with a new UUID | Existing 409 behavior; accepted original UUID remains resolvable |
| Disabled/missing dependency or offline worker | Unavailable capability / 503; other kinds and published playback survive |
| Shared search slot busy | 429 |
| Upstream transport/parser/bounds error | Bounded sanitized failure (normally 502), never fabricated empty success |
| Huangguo expansion >100 items or deadline exceeded | Explicit search failure, no silent truncation |
| Unregistered relay path, extra query/method, ended scope | Denied without arbitrary outbound request |
| Private/rebinding/invalid port, unsupported manifest or redirect | Fail acquisition; never relax proxy rules |
| Missing/damaged/oversized/canceled media or remux/decode failure | No complete publication or media import |

## 5. Good / Base / Bad Cases

- **Good:** Search Huangguo, select `huangguo-42-ep-3`, preview, submit one stable UUID, download only that episode through scoped resources, offline remux/decode, publish and scan exactly one video, then play it through the original authenticated Range API.
- **Base:** Plugin is disabled in a native deployment. Worker and original collection features start normally; UAV explains its missing configuration. Already imported videos remain playable.
- **Bad:** Treat a series as an episode, silently truncate expanded results, pass dynamic headers globally to yt-dlp, expose signed URLs to the browser, rename raw TS to MP4 after FFmpeg failure, or resubmit unknown outcomes with a new UUID. All violate this contract.

## 6. Tests Required

1. Shared/app/worker boundary checks: five site IDs including integer extremes, Unicode/control/query/page limits, duplicate/oversized responses, selectors and schema-6 compatibility. Compare old web/local/remote/JMComic operation JSON and digest bytes, not just struct equality. Long Chinese preview regression must preserve generic web's 512-byte rule.
2. `scripts/verify_uav_bridge.py`: actual pinned Python imports and upstream parsing, five-site search/resolve/empty/bad cases, Huangguo actual episode expansion and numeric query/pagination, SupJav seven-provider static parsers, matching headers, manifest fallback, FST non-m3u8 suffixes and private offset bounds. Check explicit proxy use and no real site sockets. Use actual fixed yt-dlp to verify inline finite HLS and fatal incomplete/DRM behavior.
3. Relay boundary checks: exact origin/default port/case/subdomain, extension gates, same-/cross-origin redirects, private address rejection, finite registration/unknown paths, cleanup/cancel, no CDN values in yt-dlp intent, and standard TLS verification. Use the real locked downloader for exact Range responses, ignored/wrong/short/overlong ranges including unknown-length chunked replies, and an otherwise decodable cleanly truncated TS tail. Check the actual decoded-duration lower bound, not just process exit status.
4. `scripts/verify_uav.py`: real app/worker/FFmpeg with explicitly labeled offline bridge/downloader protocol fixtures; five-site search, Huangguo single-episode and SupJav preview/download/import, candidate transport fallback/all-failed, actual MP4 Range, auth/origin, failure/cancel/retry/restart, unavailable states, legacy collection.
5. `scripts/verify_uav_browser.py`: real isolated app endpoints, five-site selection, Huangguo and SupJav task/player, 375/768/1024/1440 widths, both themes, no overflow, keyboard/focus, stale response isolation and explicitly labeled controlled expiry/unknown-submission cases. Real locked downloader/production relay must separately cover plain/wrapped TS and AES, explicit/default IV and key changes; normalizer must reject unknown/internal corruption, >8000-byte wrappers, insufficient sync and partial tails. Distinguish downloader, protocol-fixture and UI evidence.
6. Go vet/build, frontend strict typecheck/build, collection/JMComic and core/release regression. Linux cross-compilation or wheel hash selection must not be described as Docker/NAS runtime validation. Live-site access, Docker and target NAS require separate evidence.

Run tools through `python -B scripts/project_env.py -- ...`; browser processes inherit the project environment before startup. Delete newly generated temporary unit tests after executing them per AGENTS.md, retain reusable integration harnesses and truthful evidence under `.runtime/verify`.

## 7. Wrong vs Correct

**Wrong:** `yt-dlp --add-headers "x-cdn-auth:<value>" <remote-playlist>` gives redirects and unrelated resources the same access scope and may select a network FFmpeg fallback.

**Correct:** Keep CDN metadata private, register a finite set of resources, recompute headers at every HTTPS hop, give yt-dlp only virtual scoped URLs plus the validated inline playlist, then run FFmpeg against the downloaded local file.

**Wrong:** Assume that a successful remux/decode proves full media length, or that aborting after writing a complete downstream Content-Length makes the downloader fail.

**Correct:** Compare HLS decoded duration with the finite playlist lower bound; verify an upstream ranged body's exact EOF before releasing its final byte. Test failures with the actual native downloader as well as handler response inspection.

**Wrong:** Add always-present `uavSite:""`/`videoId:""` to every persisted operation, changing historical hashes; check expiry before looking up an already accepted request ID.

**Correct:** Use `omitempty`, propagate selected site/ID through private snapshots, preserve old JSON bytes, and resolve original accepted UUIDs before requiring a fresh preview.
