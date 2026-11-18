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
- `POST /api/setup` body `{ username: string, password: string }` → `{ user: User }`; first admin only, sets session cookie.
- `POST /api/login` same body/result; `POST /api/logout` → `{ ok: true }`.
- Mutations reject foreign origins, use HttpOnly SameSite cookies, and never embed credentials into media URLs.

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
- `GET /api/backup` → consistent SQLite snapshot download (not a raw copy of an active WAL database).
- Restore is an offline operation documented in README: stop application, preserve current data, replace database from backup, restart and rescan; source files remain independent.
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

## Release boundary

Deliver five content workflows, including music. Large-file direct playback/Range is required. Remux/HLS, hardware acceleration, audio transcoding, playlists/lyrics, CBR/PDF and AI remain optional later capabilities, and the UI must explain unsupported audio/video formats rather than silently pretend they work. No fabricated demo records appear in real libraries.
