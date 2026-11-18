# Torrent search runtime contract

## 1. Scope / Trigger

Applies to `torrent_search*.go`, Torznab source secrets, search API/UI, and search-origin additions through the existing download coordinator. Search is optional and independent of engine availability. Do not add public-site crawlers, executable search plugins, RSS subscriptions, automatic payload downloads, or a new database schema. See [Download contract](download-contract.md) for all existing ownership, selection and publication boundaries.

Design decision: use configured Prowlarr/Jackett Torznab sources. qB's native `search/downloadTorrent` creates an engine task and bypasses the application's persisted add intent and selection workflow; it is not a torrent-byte resolver.

## 2. Signatures

- `loadTorrentSearchConfig() (TorrentSearchConfig, error)` reads bounded, strict server-only configuration; disabled mode does not open the source file.
- `newTorrentSearchManager(config TorrentSearchConfig) *torrentSearchManager` creates separate, bounded HTTP resources and memory-only result storage.
- `(*App).torrentSearchRoutes(mux *http.ServeMux)` exposes authenticated `GET /api/torrent-search/status` and authenticated same-origin `POST /api/torrent-search`.
- `POST /api/downloads` JSON accepts `searchResultId` XOR `magnet`, with the original typed content/library/request fields. Multipart torrent upload remains unchanged.
- `download_operations(kind='add').payload` binds `{searchResultId}` without URL, key or magnet. SQLite remains schema v2. No search metadata or source configuration is persisted.

## 3. Contracts

### Configuration and upstream protocol

- `NAS_TORRENT_SEARCH_ENABLED=false`; `NAS_TORRENT_SEARCH_SOURCES_FILE` names a read-only JSON file, maximum 64 KiB. Every environment/sample configuration parameter has a Chinese explanation.
- Exact top-level structure: `{"sources":[{"id":"books","name":"图书来源","url":"http://jackett:9117/api/v2.0/indexers/books/results/torznab/api","apiKey":"server-only"}]}`. Allow 1–8 sources; reject unknown fields, trailing JSON and duplicate IDs. ID: 1–40 ASCII letters/digits/`_`/`-`; display name: 1–80 Unicode characters. Key: 1–4096 characters. Text must be valid UTF-8 without controls or surrounding whitespace.
- API URL: HTTP(S), no userinfo/query/fragment, valid port, maximum 4096 bytes. Example Prowlarr endpoint: `http://prowlarr:9696/1/api`. Secret files are outside Git, require readable owner/ACL protection, and are not covered by database backups.
- Request uses `t=search`, `q`, `apikey`, `limit=100`, `offset=0`. Category is `all`/empty (omit `cat`), `video` (`2000,5000`), `music` (`3000`), or `book` (`7000`). This version does not probe caps or negotiate per-source categories; actual support requires deployment verification.
- Global search concurrency 2; each request uses at most 3 source workers; per-source deadline 10 seconds and aggregate deadline 12 seconds. Source response maximum 2 MiB and 100 items; aggregate maximum 500. XML parsing rejects DOCTYPE, malformed/unexpected structure and bounded-node/depth violations. Resolve attributes by namespace URI, not prefix. Unknown metadata is nullable, not invented zero. Keep integer byte sizes through Go `int64`.

### Browser data and cache

- Status: `{enabled,sources:[{id,name}],maxResults:500,resultTtlSeconds:600}`; it makes no upstream probe.
- Search request: `{query,sourceId?,category?}`. Trim query, then require 2–120 Unicode characters and no controls. Empty source means all configured sources. The ordinary 64 KiB JSON budget still applies.
- Search response: `{results,sources,expiresAt,truncated}`. Each result is `{id,title,sourceId,sourceName,sizeBytes:number|null,seeders:number|null,leechers:number|null,publishedAt,downloadable}`; each source is `{id,name,state:'ready'|'error',error}`. Empty results serialize as `[]`. Dates normalize to UTC when recognized.
- No DTO may contain upstream URL, GUID, magnet, API key or raw XML. Redact the complete known source key from the original title **before** text cleaning/truncation, so a 512-rune boundary cannot leak a key prefix. Titles are rendered as text.
- Retain at most 8 batches for 10 minutes; use an expiry-prefixed 128-bit random result token. No read extends TTL. Eviction/restart discards unused tokens. Prefer known v1/v2 identities for deduplication, propagate hybrid aliases, and prefer a downloadable duplicate over an unavailable one. Do not merge groups with conflicting known hashes of the same algorithm. Without a hash or stable GUID/URL, retain each row independently; apply the 500-result limit after alias grouping. A same title alone is never stable identity.

### Search to managed download

- Check persisted request binding **before** cache lookup. Repeating an accepted request returns its original task after expiry or restart; a changed result or content type with the same request key conflicts. A new request that matches an existing managed identity needs its own completed add alias binding for later retries.
- Prefer a valid magnet. Otherwise accept only a configured source's same scheme/hostname/effective-port and exact supported proxy path: Jackett `/dl/<indexer>/` or Prowlarr `/<id>/download`, including the configured reverse-proxy prefix. Reject encoded/ambiguous proxy paths and all management/unrelated endpoints, even on the same host. Browser-supplied arbitrary URLs are never inputs.
- Use a separate HTTP client with no environment proxy, no cookie jar and no automatic redirects. A 30x Location that is a valid magnet can enter the original magnet flow. All HTTP redirects are refused. Torrent body maximum 8 MiB, deadline 15 seconds, global resolution concurrency 2. Validate torrent structure and declared identities through the original parser before submitting.
- Never hold `commandMu`, `App.mu`, a SQLite transaction or DB rows across source network work. Recheck binding after resolution to handle concurrent retries. Keep the original stopped metadata/explicit selection/publish/import flow and five content types.
- Logs use Chinese messages with source ID, bounded counts, duration or managed download ID; never log query URLs, raw network errors, keys, magnets, cookies or response bodies.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing session / foreign mutation origin | Existing 401 / 403 |
| Disabled search | 503; status remains readable and no source traffic |
| Invalid query/source/category or ambiguous add input | 400 |
| Search JSON >64 KiB / torrent >8 MiB | 413 |
| Search or source-resolution concurrency exhausted | 429 |
| At least one source succeeds | 200; keep successful rows and per-source errors |
| All sources fail, invalid XML/torrent or disallowed redirect | Sanitized 502 |
| Unavailable result has no supported download source | 422; do not fetch it |
| Unknown/evicted/restarted unused token | 404; expired token 410 |
| Reuse bound request key for different result/type | 409 |
| Retry already accepted request after cache loss | Original managed task; no second fetch/add |
| Engine offline | Search still works; original download write policy applies |

## 5. Good / Base / Bad Cases

- Good: search a configured source, choose a result, confirm `novel`, select one file, finish moving, publish and read exactly the selected original bytes.
- Base: one source is offline; available rows, their origin and a separate failure notice remain visible. Empty successful search has no failure claim.
- Bad: an item contains a management URL, cross-origin redirect or malformed torrent. Return an explicit bounded error without fetching another endpoint or creating an untracked engine task.

## 6. Tests Required

- Config validation, XML namespaces/dates/null values, limits/DTD, duplicate identity aliases, conflicting hybrid declarations, no-identity same-title rows, unavailable-first duplicates, and redaction across the title truncation boundary. Execute temporary Go behavior tests and remove only newly generated test files afterward per AGENTS.md.
- Keep `scripts/verify_torrent_search.py`: real Go/SQLite/coordinator with controlled Torznab and qB. Assert authentication, origin, partial/all/empty failures, budgets/concurrency, 64-bit size, credential isolation, proxy restrictions, stopped selection, selected-byte import, persisted retries after restart and independent disabling.
- Repeat original BT integration and five-media `verify.py --large-file` when add/runtime wiring changes. Sparse >4 GiB checks prove byte protocol, not NAS performance.
- Strict Vue typecheck/build; real app UI search → confirmation → pending selection. Mock delayed response cancellation, expiry, stable retry key and all error states; check both themes at 375/768/1024/1440 px. Preserve five mobile navigation entries and native dialog focus restoration.
- Docker execution, live indexer accounts and NAS filesystem/performance are user-manual acceptance. Fixture success is not evidence of those environments.

## 7. Wrong vs Correct

```go
// 错误：先截短会泄漏横跨边界的密钥前缀；原始网络错误还可能包含私密URL。
title = strings.ReplaceAll(truncate(original, 512), source.APIKey, "[已隐藏]")
logger.Error("搜索失败", "error", err)

// 正确：先对完整输入脱敏，再进行有界展示；日志仅保留安全上下文。
title = truncate(strings.ReplaceAll(original, source.APIKey, "[已隐藏]"), 512)
logger.Warn("搜种来源请求失败", "sourceId", source.ID)
```

The snippet illustrates ordering; use the actual bounded title helper and existing sanitized errors. Never replace a failed resolver with an unrestricted fetch or qB native download call.
