# First-release API contract

This contract is shared by the Go service and Vue application. JSON uses camelCase. All media endpoints require the same-origin session cookie. Errors use `{ "error": "Chinese user-facing explanation" }` and a meaningful HTTP status. Array fields are always arrays, never null.

## Shared types

- `ContentType`: `video | image | comic | novel | music`.
- `User`: `{ id: number, username: string }`.
- `Library`: `{ id: number, name: string, type: ContentType, path: string, status: string, itemCount: number, lastScannedAt: string }`.
- `Progress`: `{ position: number, total: number, locator: string, updatedAt: string }`. Position means seconds for video/music, page index for comic, paragraph index for TXT; locator stores EPUB location or reader-specific state.
- `Item`: `{ id: number, libraryId: number, title: string, type: ContentType, relativePath: string, size: number, mime: string, coverUrl: string, addedAt: string, updatedAt: string, missing: boolean, favorite: boolean, tags: string[], progress: Progress | null, duration: number, width: number, height: number }`.
  `duration` is audio/video duration in seconds (zero until probing succeeds). Music reuses this field. Optional downloads add separate tables through an additive v1→v2 migration, even when disabled.
- `Job`: `{ id: number, libraryId: number, kind: string, status: string, processed: number, message: string, createdAt: string, updatedAt: string }`. Status is `queued | running | completed | failed | cancelled`.

## Authentication

- `GET /api/status` → `{ setupRequired: boolean, user: User | null, version: string }`.
- `POST /api/setup` body `{ username: string, password: string }` →201 `{ user: User }`; first admin only, sets session cookie. New passwords must be 10–72 characters from U+0021–U+007E (the same count in UTF-8 bytes) and contain all four ASCII categories: uppercase `A–Z`, lowercase `a–z`, digits `0–9`, and printable punctuation (for example `!@#$%`). Reject Chinese, fullwidth characters, whitespace, control characters and emoji even when all four required classes are otherwise present. Passwords are never trimmed or truncated. Invalid passwords return400 `{ error: string }` with the Chinese rule hint and do not create an account.
- `POST /api/login` same body/result; matches the stored password without applying the new setup complexity rule, preserving existing account access. `POST /api/logout` → `{ ok: true }`.
- Mutations reject foreign origins, use HttpOnly SameSite cookies, and never embed credentials into media URLs.

## Account security

These routes use the actual administrator Cookie. No request may choose the acting user/session. They preserve the same-origin write protection and 64 KiB limit. Account JSON is stricter than legacy routes: a single object with every exact camelCase field is required; unknown, duplicate (including Unicode-escaped aliases), differently cased, missing, null/nonstring fields, nested values and trailing data are rejected. Password bytes are never trimmed or truncated. Current passwords use only the existing 72 UTF-8-byte upper bound, without new complexity/character restrictions, so legacy weak/Chinese values can be upgraded. New passwords use the setup ASCII/four-class policy.

Known account JSON lengths above 64 KiB return413 before parsing or bcrypt. Only this rejection branch may discard at most 64 KiB plus one overflow byte under a local250 ms read deadline so Go can perform its native limited-body connection close; unsupported deadlines skip this read, and failures keep an expired deadline. Expect-bearing requests are never read by this branch and receive no100 Continue; unknown-length JSON retains the normal decoder limit. Go's native close can add a500 ms connection cleanup delay. These local bounds leave ordinary request cleanup and media timeouts unchanged; clients must still handle transport failures without automatically replaying sensitive mutations.

- `AccountSession`: `{ id: string, current: boolean, clientBrowser: 'unknown'|'chrome'|'edge'|'firefox'|'safari'|'opera'|'other', clientOS: 'unknown'|'windows'|'macos'|'linux'|'android'|'ios'|'other', createdAt: string|null, expiresAt: string }`. ID is an independent random 32-character lowercase hexadecimal public identifier, never a bearer token or token-derived value. Times use RFC3339Nano; historical creation times remain null and historical client categories remain unknown. Client categories are display hints, not trusted device identities. No token, hash, IP or raw UA is returned.
- `GET /api/account/sessions?limit=20&cursor=v1.<lowerhex32>` → `{ currentSession: AccountSession, items: AccountSession[], nextCursor: string|null }`. `currentSession` is pinned separately with `current=true` from the real Cookie; `items` contains only other valid sessions with `current=false`. Default limit is20, accepted range1–100. Cursor is omitted initially or exactly `v1.` followed by32 lowercase hexadecimal characters. Unknown/repeated/empty query values are400. Ordering is `session_id ASC` (random ID order), not recent-login order. The server reads at most `limit+1` raw indexed rows, consumes the first `limit`, and uses one timestamp to filter expired/invalid/old-revision/current rows. If a lookahead exists, cursor is the last **consumed raw row**, even when no items were emitted; otherwise null. An empty items batch with non-null cursor can be continued manually. Deleted cursor rows remain valid keyset boundaries. No total/pages, automatic page filling, cross-request snapshot, or unlimited accumulated history is promised; refresh from the start to see concurrently inserted IDs before the current boundary.
- `POST /api/account/password` exact body `{ currentPassword: string, newPassword: string }` →200 `{ requiresLogin: true }`. Confirmation is local UI only. After bcrypt outside the transaction, a short transaction rechecks account/session, conditionally changes the hash, increments credential revision and deletes all user sessions atomically. Only successful commit clears the Cookie; log in with the new password. Do not automatically replay an uncertain mutation response.
- `DELETE /api/account/sessions/{sessionId}` →200 `{ requiresLogin: boolean }`. ID must match the public lowercase32-hex format. For an authenticated actor, unknown/already revoked targets are idempotent success and queries are scoped to the actor. `requiresLogin` is true only for the actual current session, then the Cookie is cleared. A repeated request carrying that now-dead Cookie is401.
- `POST /api/account/sessions/revoke-others` exact body `{ currentPassword: string }` →200 `{ requiresLogin: false, revokedCount: number }`. Preserve the actual current session and delete all other user session records, including expired/old-revision rows. Count is the transaction's actual deleted-row count, **not an active-device count**; no other records returns zero.

Wrong current passwords return400 with fixed Chinese `当前密码不正确`, leaving the Cookie intact. Missing/revoked/expired entry authentication is401; foreign writes403, invalid fields/ID/query400, media type415 and oversized input413. After admission, transactions compare user ID/hash/revision **before** checking current-session existence/expiry: changed credentials409 take priority; the same account snapshot with a revoked/expired session is401. Context cancellation and actual expiry are checked again immediately before commit; failures roll back. Commit invalidates new requests using revoked sessions; media streams already admitted may complete.

Password changes and revoke-others share separate in-memory reauthentication budgets: per account12 admitted bcrypt comparisons per5 minutes and per actual session6 per5 minutes. Both successful and failed comparisons consume budget; syntax/new-password-policy failures do not. Rejected requests increment neither budget nor extend their fixed window. Successful reauthentication/login, new session IDs and credential revision changes do not clear the account budget.429 includes integer `Retry-After` rounded up within1–300 seconds. Expired keys are purged; at most2048 total account/session keys are retained, with conservative rejection at capacity. These limits are process-local and do not survive restart.

### Offline account recovery

`nas account reset-password --data-dir <existing-directory> [--password-stdin]` is a local maintenance command, dispatched before service configuration or networking. Stop the instance first: maintenance holds the same canonical-directory OS lease, opens only an existing ordinary local `library.db`, and explicitly supports schemas 5 and 6 without migration, account creation, jobs/recovery or cache cleanup. It requires the unique valid ID=1 administrator, rechecks the exact preflight user version as well as schema/credential snapshots in a short transaction, replaces the hash, increments revision and deletes all sessions atomically. A change between supported versions still fails. Schema-6 backup rows and pending files are preserved; maintenance does not prepare a backup root. It never accepts a password in argv or environment.

Default input requires a real terminal, hidden entry and two confirmations; redirected input requires the explicit `--password-stdin` flag and EOF. Both use the same untrimmed 10–72-byte ASCII/four-class policy. The stdin reader accepts only one password with at most one final LF/CRLF, using a 75-byte bounded read including an overflow sentinel. Terminal input has a sticky 1024 raw non-line-ending-byte budget per prompt; invalid/control/non-ASCII or overflow input cannot become valid through editing. Mode restoration must finish before bcrypt or writes.

Only the exact `nas account reset-password --help` performs resource-free help. Options require separate space-delimited values; equals syntax, duplicates and extra arguments fail. Exit codes: 0 means commit and cleanup succeeded (or help succeeded), 2 usage error, 130 pure precommit cancellation, 1 other failure. A committed change followed by cleanup/output failure must not be interpreted as unchanged. An uncertain transaction result explicitly requires checking the account; never retry automatically. After successful reset, restart normally: all old Cookies/passwords fail and the new password can log in. Operational steps, backup/session implications and platform evidence limits: [accounts.md](accounts.md).

## Libraries and background jobs

- `GET /api/libraries` → `{ libraries: Library[] }`.
- `GET /api/browse?path=...` → `{ path: string, parent: string, entries: { name: string, path: string, isDir: boolean }[], roots: string[] }`; empty path lists configured roots. Only configured media roots may be browsed.
- `POST /api/libraries` body `{ name: string, type: ContentType, path: string }` → `{ library: Library }`; queues initial scan.
- `POST /api/libraries/{id}/scan` → `{ job: Job }`.
- `DELETE /api/libraries/{id}` → `{ ok: true }`; removes the library index, never source media.
- `GET /api/jobs` → `{ jobs: Job[] }`; `POST /api/jobs/{id}/cancel` and `/retry` → `{ ok: true }`.
- Retry returns 409 while a cancelled executor is still stopping, or when another active job owns the same library/content key. Wait for the current executor to finish stopping before retrying.

## Content and personal state

- `GET /api/items?type=&q=&favorite=&libraryId=&page=1&pageSize=48&sort=recent` → `{ items: Item[], total: number, page: number, pageSize: number }`. Sort supports recent, title, progress. Search title/path/tags. Favorite is `true` when enabled.
- `GET /api/items/{id}` → `{ item: Item }`.
- `POST /api/items/{id}/favorite` body `{ favorite: boolean }` → `{ ok: true }`.
- `PUT /api/items/{id}/tags` body `{ tags: string[] }` → `{ ok: true }`.
- `PUT /api/items/{id}/progress` body `{ position: number, total: number, locator: string }` → `{ ok: true }`.
- `GET /api/stats` → `{ total: number, video: number, image: number, comic: number, novel: number, music: number, totalSize: number }`. Every type count is present even when zero.
- `GET /api/items/{id}/file` supports GET/HEAD and byte Range, streams the registered source after permission/path checks.
- `GET /api/items/{id}/cover` → cached bounded image or a generated SVG cover with escaped title; a missing cover must not repeatedly trigger expensive work.

## Reading

Music libraries index MP3, M4A, AAC, FLAC, OGG, OPUS, WAV and AIF/AIFF files case-insensitively. They use the same list/filter/favorite/tag/progress APIs and authenticated `/file` streaming as video. Stable MIME values are `audio/mpeg`, `audio/mp4`, `audio/aac`, `audio/flac`, `audio/ogg` (OGG/OPUS), `audio/wav`, and `audio/aiff`. Browser codec/container support determines actual playback; no server audio transcoding is offered.

Audio duration and optional embedded artwork are extracted asynchronously with bounded external-tool budgets. Audio without artwork is a successful metadata-only result and receives the music SVG cover; unchanged rescans do not repeatedly enqueue artwork jobs. Failed probing remains visible in jobs and does not prevent file access. Progress uses seconds with an empty locator.

- `GET /api/items/{id}/comic` → `{ pages: { index: number, name: string, url: string }[] }`. Image-directory comic and CBZ supported; natural sort; page index is zero-based.
- `GET /api/items/{id}/pages/{index}` → a bounded image from the registered directory/archive. Never unpack arbitrary archive paths to disk.
- `GET /api/items/{id}/text?encoding=` → `{ title: string, paragraphs: string[], chapters: { title: string, index: number }[], truncated: boolean }`. Bounded TXT parsing, UTF-8/GB18030; unsupported huge input gets an explicit error rather than unbounded allocation.
- EPUB is served from `/file` and read by the bundled EPUB component with scripts disabled; same session used for its download. Save EPUB location in progress.locator.

## Maintenance

- `GET /api/settings` → `{ version: string, dataDir: string, mediaRoots: string[], scanIntervalMinutes: number, cacheMaxBytes: number, ffmpegAvailable: boolean, ffprobeAvailable: boolean }` (admin only in first release).
- `GET /api/backup` → consistent SQLite snapshot download, verified read-only before headers: current schema head 6/core structure, exact page length, SQLite integrity and foreign keys, followed by streaming SHA-256. The five backup tables must be STRICT with exact declared columns/types, no hidden columns and both singletons; this is a narrow structure probe, not proof of every DDL constraint. Administrator authentication, `application/vnd.sqlite3`, local-time `nas-library-YYYYMMDD-HHMMSS.db` naming and native `ServeContent` behavior remain unchanged. HEAD also generates a snapshot; concurrent generation/download returns 409 until cleanup completes. Free-space precheck failures return 507; generation or validation failures return fixed Chinese 500 responses. Each request generates its own temporary snapshot, so this is not a stable cross-request resumable object. Schema-6 storage alone adds no policy/history/scheduling API or persistent root.
- Snapshot scope is **database-only**. The DB contains sensitive indexes, password/session hashes and private references and must be kept private. It excludes external source-secret/helper/worker identity files, component volumes, download payloads, media and original user assets. This slice adds no persistent history, scheduler, manifest endpoint, UI or offline verification/recovery-preparation command; a same-disk export is not off-site disaster recovery. See [snapshot implementation contract](../.trellis/spec/backend/backup-contract.md).
- `NAS_BACKUP_DIR` and the package-internal root/publication protocol add no public route in S03.2.2; local independent acceptance is complete and real Linux/NAS validation remains pending. They do not initialize the persistent root during ordinary startup or change this synchronous download API. See [root/publication contract](../.trellis/spec/backend/backup-publish-contract.md).
- Restoration must preserve current data and use a separate stopped, isolated directory; source files and other recovery materials remain independent. A snapshot also preserves its then-valid sessions. The explicit session-revoking recovery-preparation workflow is pending S03.3: this slice's read-only snapshot checks do not revoke sessions or authorize exposing a restored copy.
- `/healthz` is unauthenticated and returns `{ status: "ok" }` only after storage initialization.

## Optional BT downloads

`ContentType` remains the five media types. Download tasks are independent of scan/enrichment jobs. Every endpoint below requires the existing administrator session and mutation origin checks. The browser never receives the engine URL, password, SID, raw torrent or magnet. Engine authentication failures are module errors, not application-session 401 responses.

- `DownloadStatus`: `{ enabled: boolean, state: string, version: string, webApiVersion: string, lastSyncedAt: string, error: string, canSelectFiles: boolean }`.
- `Download`: `{ id: string, name: string, contentType: ContentType, libraryId: number | null, state: string, engineState: string, importState: string, sizeBytes: number, downloadedBytes: number, downloadSpeed: number, uploadSpeed: number, uploadedBytes: number, ratio: number, peers: number, etaSeconds: number | null, manifestVersion: number, selectionLocked: boolean, revision: number, ready: boolean, archived: boolean, lastSyncedAt: string, lastError: string, createdAt: string, updatedAt: string }`. Speeds are bytes/second; byte counts use safe nonnegative 64-bit values. Unknown ETA is null.
- `DownloadFile`: `{ index: number, path: string, sizeBytes: number, selected: boolean, progress: number }`. Index is the engine file ID; progress is 0–1.

| Endpoint | Input / result |
| --- | --- |
| `GET /api/downloads/status` | `DownloadStatus`; disabled status is readable |
| `POST /api/downloads/connection-test` | `DownloadStatus`; uses fixed server configuration |
| `GET /api/downloads?page=1&pageSize=30&state=&q=` | `{ downloads: Download[], total: number, page: number, pageSize: number }` |
| `POST /api/downloads` | JSON `{ requestId, magnet?, searchResultId?, contentType, createLibrary, libraryId? }` with exactly one source, or multipart one `torrent` file plus the scalar fields without magnet/searchResultId; 202 `{ download: Download }` |
| `GET /api/downloads/{id}` | `{ download: Download }` |
| `GET /api/downloads/{id}/files?page=1&pageSize=100` | `{ files: DownloadFile[], total, page, pageSize, manifestVersion, selectionLocked }` |
| `PUT /api/downloads/{id}/selection` | `{ requestId: string, manifestVersion: number, indices: number[] }`; confirms selection, locks it and starts transfer; 202 `{ download }` |
| `POST /api/downloads/{id}/actions` | `{ requestId: string, revision: number, action: 'start' | 'pause' | 'reconcile' | 'remove' }`; 202 `{ download }` |
| `PUT /api/downloads/{id}/limits` | `{ downloadKiB: number, uploadKiB: number }`; `{ ok: true }`; zero adds no task limit, global limits still apply |
| `POST /api/downloads/{id}/retry-import` | 202 `{ download }`; reuses the downloaded files |
| `GET /api/downloads/{id}/items` | Existing `ItemPage` shape: `{ items, total, page, pageSize }` |

Ordinary JSON remains limited to 64 KiB. Only the exact selection PUT receives 256 KiB for up to 20,000 indices, and the exact torrent multipart POST receives 8 MiB plus 64 KiB envelope; torrent content itself is limited to 8 MiB. No arbitrary URL or client-selected save path is accepted. Duplicate request IDs are idempotent; stale revisions/manifests and externally owned hashes return conflicts.

Download and import state are separate. A successful engine call or 100% progress does not publish files. The coordinator checks stable selected metadata, engine state/location, file sizes and paths, persists ready/version/mtime, then verifies actual scan coverage. Staging, unknown final tasks, frozen tasks, unselected files and changed versions remain blocked in manual/periodic/restart scans, comic pages, covers and direct media reads. Archived publications retain their manifests; task removal always preserves data. Turning the module off does not remove the gate.

Database snapshots include the v2 download tables, but engine configuration/restore data, private submission sources, secrets and download files require separate backup. See `downloads.md` for deployment, recovery and supported engine verification details.

## Optional torrent search

Torznab search uses the existing authenticated session and same-origin mutation checks. It is independent of qB connectivity; only adding a result requires an available download engine. Disabled search performs no external requests and has no schema migration.

- `GET /api/torrent-search/status` → `{ enabled: boolean, sources: { id: string, name: string }[], maxResults: 500, resultTtlSeconds: 600 }`. Sources never include URL or API Key.
- `POST /api/torrent-search` body `{ query: string, sourceId?: string, category?: 'all' | 'video' | 'music' | 'book' }`. Trimmed query is 2–120 Unicode characters without controls. Empty source means all configured sources. The body retains the 64 KiB JSON limit.
- Response `{ results: TorrentSearchResult[], sources: TorrentSearchSourceStatus[], expiresAt: string, truncated: boolean }`. Partial source failure preserves successful results; complete failure returns 502 and a sanitized Chinese error. Disabled is 503, bad input 400, concurrent search saturation 429.
- `TorrentSearchResult`: `{ id: string, title: string, sourceId: string, sourceName: string, sizeBytes: number | null, seeders: number | null, leechers: number | null, publishedAt: string, downloadable: boolean }`. Unknown numeric data is null; invalid publication dates are empty. ID is a server-generated opaque reference, never a provider URL/GUID. No description HTML, API Key, private URL, torrent bytes or magnet is returned.
- `TorrentSearchSourceStatus`: `{ id: string, name: string, state: 'ready' | 'error', error: string }`.

Results are capped at 100 per source and 500 total; the UI sorts and paginates this finite snapshot without repeating the external search. The server retains at most eight batches for ten minutes. Restart, expiry or eviction invalidates unused result IDs; search again if a fresh add receives 404/410.

Adding uses the original `POST /api/downloads` with `searchResultId` instead of magnet. The backend resolves an already cached magnet or a same-origin indexer download proxy, then uses the same torrent validation, durable command registration, file-selection and publication workflow. Arbitrary client URLs, cross-origin HTTP redirects and same-origin non-proxy paths are rejected; a proxy redirect to a valid magnet is supported. Unavailable downloads return 422; failed upstream/invalid torrent is 502, oversized torrent is 413. The 8 MiB torrent bound remains in force.

The `add` operation payload stores only the opaque search result reference. Successful request IDs are checked against this persistent binding before consulting the short-lived search cache, allowing the original request to recover after an app restart without fetching or adding again. Reusing the key for another result or content type returns 409. No credentials or private source URLs are persisted in the operation payload.

Server-only configuration: `NAS_TORRENT_SEARCH_ENABLED=false` and `NAS_TORRENT_SEARCH_SOURCES_FILE`, a maximum 64 KiB strict JSON file `{ sources: [{ id, name, url, apiKey }] }` with 1–8 endpoints. Search has two concurrent global slots, three source slots per request, a ten-second source timeout and twelve-second overall timeout. XML is limited to 2 MiB/source and rejects DTD, malformed/deep structures. Seed resolution has two slots, fifteen seconds and 8 MiB. See `torrent-search.md` for field constraints, deployment and backup boundaries.

## Optional managed engine initialization

Every route uses the existing administrator session and same-origin mutation protection. Ordinary JSON is strictly decoded and limited to64KiB. The app talks only to its configured private Unix socket; it never proxies arbitrary Docker API paths. Safe shared DTO definitions live in `internal/enginecontract/models.go`. Deployment and backup details: `engines.md`.

| Endpoint | Input / result |
| --- | --- |
| `GET /api/engines` | `EngineStatus`; metadata only, no install/start/source query effects |
| `POST /api/engines/probe` | `{}` → refreshed `EngineStatus`; never deploys |
| `POST /api/engines/initializations` | `{ requestId: string, components: ('qbittorrent'|'jackett')[], noticeVersion: string, acceptNotice: boolean }` →202 `EngineTaskResponse` |
| `GET /api/engines/initializations/{id}?afterSequence=0` | `EngineTaskResponse`; nonnegative incremental event cursor |
| `POST /api/engines/initializations/{id}/retry` | `{ requestId: string, revision: number }` →202 `EngineTaskResponse`; requeues only failed steps |
| `POST /api/engines/{component}/start` | `{ requestId: string, revision: number }` →202 `EngineTaskResponse`; starts the original owned container, no replacement installation |
| `POST /api/engines/search/sync-sources` | `{ sourceIds?: string[] }` → `EngineSourceResponse`; managed search only, at most8 safe IDs |

- `EngineStatus`: `{ helper, message, components: EngineComponent[], activeTaskId, noticeVersion, noticeText, setupPort }`. Helper is `ready|unavailable|unsupported|not_enabled`. `setupPort` is the host-loopback Jackett management port, never a generic proxy address.
- `EngineComponent`: `{ id, mode, presence, runtime, readiness, message, revision, canInitialize, canStart }`. Mode is `disabled|external|managed`; presence `unknown|missing|present|conflict`; runtime `unknown|stopped|starting|running|offline`. Readiness additionally distinguishes `not_configured`, `initializing`, `connecting`, `configuration_required`, `ready`, `auth_failed`, `incompatible` and `error`. Treat these dimensions independently and use server-supplied action booleans.
- `EngineTaskResponse`: `{ task: EngineTask, events: EngineEvent[], nextSequence: number, reloadRequired: boolean }`. Events are at most100/request; a retained-history gap requires cursor reload.
- `EngineTask`: `{ id, action, state, revision, components: string[], steps: EngineStep[], createdAt, updatedAt, message }`. Action `initialize|start`; state `queued|running|completed|failed|interrupted`. Timestamps are RFC3339.
- `EngineStep`: `{ component, phase, state, attempt, message, layers: EngineLayer[] }`. State `queued|running|completed|failed`; phase reports the actual stage such as preflight/pulling/preparing/creating/starting/health/publishing/completed. Layer entries `{ id, status, current, total }` contain actual Docker byte counts; unknown total is0. There is no overall percentage.
- `EngineEvent`: `{ sequence, component, level, message, createdAt }`, level `info|error`; controlled Chinese text, never raw engine/Docker logs.
- `EngineSourceResponse`: `{ candidates: { id, name }[], selectedIds: string[], selectionRequired: boolean, state, message }`. State `ready|configuration_required`.0 candidates remains pending;1–8 may activate directly; more than8 returns a selection requirement and preserves the prior active set until an explicit valid selection. Unknown/duplicate selections fail400. Discovery never performs a keyword search.

Only successful Docker preflight plus exact resource absence means missing. Helper outage, connection/auth failure and conflict do not allow reinstall/takeover. External-mode components never expose managed actions. qB installation requires an actively accepted matching notice version; the client must not precheck it. A complete helper task means deployment/private publication finished; actual module readiness still requires the app adapter to connect.

Same requestId and same operation signature returns the original durable task even if a response was lost. A changed signature or stale revision returns409, different active work429, absent/pruned task404, unavailable helper503, malformed/oversized input400/413. Preserve requestId/revision on network retry; task-state failures are readable with HTTP200 and safe diagnostic events. Browser cancellation does not cancel the durable worker.404 uses a fixed safe explanation so the UI can discard an obsolete recovery ID.

Connection manifests, passwords, API Keys, upstream addresses, Cookie headers and raw XML never appear in these responses. The app atomically refreshes managed connections and preserves its last valid runtime if the helper or a new manifest fails. Source changes invalidate unbound old search-result tokens; existing persisted download request bindings remain recoverable.

## Multi-source collection

All `/api/collection` routes require the existing session and same-origin mutation protection. Ordinary JSON remains strict and limited to 64 KiB. IDs are opaque UUIDs; edits and task actions carry the current `revision`. Lists return empty arrays for successful empty results, while failures retain an HTTP error. Detailed DTO fields are maintained in [the collection API contract](../.trellis/tasks/11-19-resource-collection-plan/api-contract.md).

| Endpoint | Input / result |
| --- | --- |
| `GET /capabilities` | Actual tool capabilities, safe worker source IDs, authorized registration roots, worker readiness and reason |
| `GET /sources`, `POST /sources`, `PATCH /sources/{id}` | List, create and revise a fixed RSS/web/local/remote/external source; edits require revision |
| `POST /sources/{id}/test` | `{}` → connection result; no acquisition |
| `POST /sources/{id}/refresh` | `{backfill:boolean}` → bounded RSS refresh; first refresh establishes a historical baseline |
| `GET /subscriptions`, `POST /subscriptions`, `PATCH /subscriptions/{id}` | Rule, target, review/automatic mode and explicit selection policy |
| `POST /subscriptions/{id}/preview`, `POST /subscriptions/{id}/run` | `{}` → rule dry run or bounded refresh/evaluation |
| `GET /candidates?sourceId=&page=&pageSize=` | Candidate page with safe title, decision, reason and revision |
| `POST /candidates/{id}/ignore` | `{revision:number}` |
| `POST /candidates/{id}/acquire` | `{requestId,revision,contentType,createLibrary,libraryId}` → 202 durable task linked to the original BT workflow |
| `POST /previews` | `{sourceId,url,path,profile,mode}` → expiring server-bound preview; profile is video/audio, mode register/copy, path relative |
| `POST /tasks` | `{requestId,previewId,contentType,createLibrary,libraryId}` → 202 durable task |
| `GET /tasks?page=&pageSize=&state=`, `GET /tasks/{id}` | Safe task state, independent import state and actual item IDs |
| `GET /tasks/{id}/events` | Bounded persistent Chinese stage events |
| `POST /tasks/{id}/actions` | `{action:'cancel'|'retry'|'retry_import',revision}` |

All paths above are relative to `/api/collection`. Same accepted request ID and immutable plan returns the original task; conflicting reuse, duplicate resource under a different request ID, or stale revision returns 409. Local registration reads only authorized existing media. Copy/web tasks use the configured worker and shared-root probe, acquire into staging, validate, publish, then scan. `completed` acquisition alone does not mean imported: only actual matching scanner results populate `itemIds`. `retry_import` uses published data without acquiring again. Library deletion clears related non-BT task item IDs and marks import failed, retaining files without silently recreating the library. BT file selection and download actions remain in the original download APIs.

RSS URLs and worker request secrets reside in private files outside ordinary SQLite/DTOs. No endpoint accepts shell commands, executable paths, tool arguments, arbitrary remote definitions or web cookies. External applications expose fixed typed management APIs and independently selected completed-media directories. See [collection deployment and recovery](collection.md) for configuration, backups and supported website formats.

### External automation APIs

Source DTOs add `externalService: ''|'moviepilot'|'autobangumi'|'sonarr'|'radarr'` and `apiConfigured:boolean`. Source input accepts write-only `connection?:{baseUrl:string,apiKey:string}` and `clearConnection?:boolean`. Blank connection fields retain same-service credentials. Completion `rootId` is optional for API-enabled sources; actual import still requires an authorized mapped root.

All following routes are relative to `/api/collection/sources/{id}/external` and require the existing session. Public data never includes the API base URL/key, RSS URL, upstream raw errors, torrent context or absolute storage path.

| Endpoint | Input / result |
| --- | --- |
| `POST /test` | `{revision}` → `{info:{service,version,supported,capabilities,message}}` |
| `GET /items?section=resources\|queue\|history&page=1` | Bounded normalized page with detected capabilities; unavailable sections return errors |
| `GET /options` | Existing service root/profile choices; no new remote configuration |
| `GET /lookup?q=` | Servarr metadata choices, no automatic download search |
| `GET /actions` | Latest 30 safe persistent action records |
| `POST /actions` | `{requestId,revision,kind,targetId?,externalId?,title?,mediaType?,season?,rootId?,profileId?,enabled?,rssUrl?,selectionId?,confirmOverwrite?}` → `{action}` |
| `POST /actions/{actionId}/reconcile` | `{}` → read-only evidence, never replays a write |
| `POST /actions/{actionId}/acknowledge` | `{checkedExternally:true}` → records manual `unconfirmed` and unblocks the source |

Actions use `add|monitor|search`; MoviePilot 3.2.8 additionally uses `download|download_start|download_stop|organize|scrape|refresh_library`. An action records `{id,requestId,sourceId,sourceRevision,kind,targetId,state,message,createdAt,updatedAt}`; state is `dispatching|succeeded|failed|uncertain|unconfirmed`. `succeeded` confirms the specifically described receipt, not local import. Same accepted UUID and intent returns the original action even after source revision changes; different intent is 409. New writes require current revision. Unresolved actions block other writes for that source. Restart changes interrupted `dispatching` to `uncertain`, without reissuing upstream requests.

MoviePilot workflow routes below are relative to `/moviepilot`. Replies are `{choices:Choice[],message:string}`, where `Choice={id,title,kind,status,externalId,mediaType,year,season,organizeAllowed}`. `id` is a server-generated UUID bound to source revision and a 15-minute private snapshot. A snapshot accepted by one action cannot be consumed by a new action. Only already-configured local roots and their observed descendants are accepted; these are remote service choices, not local NAS filesystem grants.

| Endpoint | Input / result |
| --- | --- |
| `POST /search` | `{revision,query,mediaType:'movie'|'series'}` → media choices |
| `POST /releases` | `{revision,selectionId}` → resource choices for the selected metadata |
| `GET /downloads` | Single-task download control choices |
| `GET /roots` | Configured download/library root aliases; `organizeAllowed` reflects the no-overwrite policy |
| `POST /files` | `{revision,rootId,parentId?}` → one bounded directory level |
| `POST /organize/preview` | `{revision,fileId,targetId,externalId,mediaType}` → fixed-copy organization plan |
| `POST /scrape/preview` | `{revision,fileId,externalId,mediaType}` → read-only validation, no scraping yet |
| `GET /library` | Fixed `mediaserver_sync` task and observed state |

Workflow actions accept only the matching `selectionId` and, for scraping, mandatory `confirmOverwrite:true`; they reject unrelated raw arguments. Server-side immutable payloads are persisted outside SQLite. Organization rechecks the source file version and configured target policy; scraping covers only one selected supported video file and overwrites metadata/artwork according to MoviePilot. Library refresh synchronizes **MoviePilot's index from configured media servers**; it is not an Emby/Plex disk scan. Exact schemas and protocol boundaries: [external integration contract](../.trellis/tasks/11-20-collection-external-api/api-contract.md).

### Component Docker deployment

The app uses the existing administrator session, same-origin checks, strict 64 KiB JSON and its configured private helper socket. Opening the source dialog or reading status never installs anything. Deployment and first-configuration instructions: [components.md](components.md).

| Endpoint | Input / result |
| --- | --- |
| `GET /api/collection/components` | `CollectionStatus`; four fixed products and actual helper/runtime capability |
| `POST /api/collection/components/deploy` | `{requestId,component,templateVersion,noticeVersion,acceptNotice}` → 202 `EngineTaskResponse` |
| `POST /api/collection/components/{component}/start` | `{requestId,revision}` → 202 `EngineTaskResponse`; only starts the original owned container |
| `GET /api/collection/components/tasks/{id}?afterSequence=0` | `EngineTaskResponse`; bounded incremental events |
| `POST /api/collection/components/tasks/{id}/retry` | `{requestId,revision}` → 202 `EngineTaskResponse`; retries the original failed deployment/start task |

`component` is exactly `moviepilot|autobangumi|sonarr|radarr`. Deploy requires template `collection-2026-11-20.1`, notice `collection-storage-v1` and explicit `acceptNotice:true`; unchecked or outdated authorization returns 409. It accepts no image/command/port/mount overrides.

`CollectionStatus = {helper,message,templateVersion,noticeVersion,noticeText,activeTaskId,components:CollectionComponent[]}`. Each component is `{id,name,version,presence,runtime,readiness,message,revision,canDeploy,canStart,setupPort,serviceUrl,latestTaskId}`. Presence is `unknown|missing|present|conflict`, runtime `unknown|stopped|starting|running`, readiness `not_configured|starting|configuration_required|error`. Helper is `ready|unavailable|unsupported|not_enabled`. Actions follow the returned permission booleans. `serviceUrl` contains no credentials and is available only after the owned running component passes its bounded anonymous HTTP probe. It is an internal platform address, not a public browser proxy or proof of API authentication. `setupPort` is bound only on Docker-host loopback.

Task response fields match the managed-engine DTOs above; collection task actions are `deploy|start`. Persistent UUID binding survives lost responses and helper restart. Same accepted intent returns its original task; changed intent or stale revision is 409, different active work 429, absent/pruned/out-of-scope task 404 and unavailable execution 503. A failed task returns its snapshot with HTTP 200. These task endpoints cannot read/retry qB/Jackett or stop-all work, and old engine routes cannot deploy these new products.

`ENGINE_COLLECTION_ENABLED` defaults false. Linux managed Compose and desktop schema-v3 overlays enable it. Four products use separate owned private configuration volumes and explicitly authorized `/downloads` plus `/data` mounts; app can register only the read-only `/collection-media/library` subtree. GET never creates/adopts volumes. Conflict stops deployment without takeover. Components still require their own initial setup and the existing authenticated source test; container completion does not activate external business capabilities or import media. Exact executable contract and validation cases: [component deployment contract](../.trellis/spec/backend/collection-component-contract.md).

## Optional JMComic comic plugin

The fixed `jmcomic` source kind uses the existing authenticated collection API and does not add a schema migration. Source configuration uses `name` and `enabled`; it accepts no site URL, account, Python/YAML, executable or output-directory override. Capability `kind:'jmcomic'` reports actual optional worker readiness. Missing plugin dependencies do not disable other collection types.

- `POST /api/collection/sources/{id}/jmcomic/search` body `{query:string,page:number}` → `{query,page,total,hasNext,items:[{albumId:string,title:string,tags:string[]}]}`. Query is trimmed, requires 1–120 Unicode characters and rejects controls. Page 0 defaults to 1; valid pages are 1–1000. Only an enabled JMComic source can search. Results contain text and numeric IDs, without upstream links or image URLs. Empty success returns `items:[]`; unavailable or failed upstream requests return a Chinese error.
- Existing `POST /api/collection/previews` additionally accepts `{sourceId,albumId:string}` for this source; `albumId` must match `[1-9][0-9]{0,11}`. The server selects `kind:'jmcomic',profile:'comic'`. The usual `{preview:CollectionPreview}` binds canonical album identity, source revision, worker identity, budgets and expiry. Search/preview do not download page images.
- Existing `POST /api/collection/tasks` accepts the original `{requestId,previewId,contentType:'comic',createLibrary,libraryId}`. Other content types are rejected. A preview may have no files before acquisition; download completeness, actual raster files and budget facts are validated before normal publication/import.
- Existing task events, cancel/retry/import retry and item/comic/page APIs apply. Accepted UUID retries return the original task, including after preview expiry; conflicting intent returns 409.

The private worker adds authenticated `POST /v1/jmcomic/search` and the optional `albumId` field to preview/operation requests. Omit empty album IDs when serializing original operation types, preserving their existing hashes. Enable/configuration and dependency installation: [jmcomic.md](jmcomic.md).

## Optional UAV video plugin

The fixed optional `uav` source kind reuses collection source/task storage and keeps SQLite schema 6. Sources accept `name` and `enabled`; reject arbitrary URLs, paths, private-network permission, executable/header/proxy overrides and external connections. Capability `kind:'uav'` requires the fixed Python bridge, yt-dlp and FFmpeg. Capability probing does not contact a video site.

- `POST /api/collection/sources/{id}/uav/search` takes `{site:'jable'|'missav'|'hanime1'|'huangguo',query:string,page:number}` and returns `{site,query,page,hasNext,items:[{videoId:string,title:string,duration:string}]}`. Query is trimmed and requires 1–120 Unicode characters without controls; page 0 defaults to 1, valid range 1–1000. There is no authoritative `total`. Results expose text and IDs only, without thumbnails, media URLs or credentials. Empty success is `items:[]`; transport/parser errors are failures.
- Existing `POST /api/collection/previews` takes `{sourceId,uavSite,videoId}` and fixes `kind:'uav',profile:'video'`. Jable IDs are lowercase ASCII slugs up to 200 characters; MissAV IDs are ASCII slugs containing a digit except category `dm` followed only by digits; Hanime1 IDs match `[1-9][0-9]{0,11}`. Huangguo IDs are `huangguo-<series>-ep-<episode>` with positive decimal values and no leading zeros (series ≤9223372036854775807, episode ≤2147483647). Each ID selects one actual episode; a bare series ID is invalid. URLs and path components are not IDs. Preview is metadata-only (`files:[]`), with the original source revision, worker identity, budget, canonical content identity and expiry binding. UAV search/preview titles have a 1200-byte UTF-8 validation budget and existing worker safe-title normalization; other preview kinds retain their original limit.
- Existing `POST /api/collection/tasks` takes `{requestId,previewId,contentType:'video',createLibrary,libraryId}`. Other content types are rejected. Unknown submission results retry the identical UUID/body. Actual worker-owned native HTTP/HLS acquisition, offline MP4 remux, full decode and independent manifest/publication checks must succeed before import.
- Existing task events, cancellation, retry/import retry and authenticated item/file Range APIs apply. A disabled/missing plugin or offline worker preserves existing published video playback.

The private authenticated worker adds `POST /v1/uav/search`; preview/operation requests gain `uavSite,omitempty` and `videoId,omitempty`, preserving old serialized hashes. The fixed bridge exposes `probe`, `search` and `resolve` only and never invokes the upstream queue/downloader. Deployment and fixed source/dependency identity: [uav.md](uav.md).

Huangguo search expands actual episodes within a 22-second deadline and a 100-result bound; overflow is an explicit error, not a truncated successful page. Its private resolve result may include scoped CDN metadata. Only registered finite-playlist resources are relayed through the worker's authenticated proxy; each HTTPS redirect rechecks public-address policy and the original exact origin/resource header scope. CDN values and signed media URLs never enter public responses or durable task metadata. The relay does not change generic web acquisition rules.

## Release boundary

Deliver five content workflows, including music. Large-file direct playback/Range is required. The player does not provide HLS delivery, live remux or hardware acceleration; the optional UAV acquisition adapter separately supports finite native HTTP/HLS download with offline MP4 remux and validation. Audio transcoding, playlists/lyrics, CBR/PDF and AI remain optional later capabilities, and the UI must explain unsupported audio/video formats rather than silently pretend they work. No fabricated demo records appear in real libraries.
