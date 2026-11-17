# First-release API contract

This contract is shared by the Go service and Vue application. JSON uses camelCase. All media endpoints require the same-origin session cookie. Errors use `{ "error": "Chinese user-facing explanation" }` and a meaningful HTTP status. Array fields are always arrays, never null.

## Shared types

- `ContentType`: `video | image | comic | novel | music`.
- `User`: `{ id: number, username: string }`.
- `Library`: `{ id: number, name: string, type: ContentType, path: string, status: string, itemCount: number, lastScannedAt: string }`.
- `Progress`: `{ position: number, total: number, locator: string, updatedAt: string }`. Position means seconds for video/music, page index for comic, paragraph index for TXT; locator stores EPUB location or reader-specific state.
- `Item`: `{ id: number, libraryId: number, title: string, type: ContentType, relativePath: string, size: number, mime: string, coverUrl: string, addedAt: string, updatedAt: string, missing: boolean, favorite: boolean, tags: string[], progress: Progress | null, duration: number, width: number, height: number }`.
  `duration` is audio/video duration in seconds (zero until probing succeeds). Music reuses this existing field; no database schema upgrade is needed from v1.
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

## Release boundary

Deliver five content workflows, including music. Large-file direct playback/Range is required. Remux/HLS, hardware acceleration, audio transcoding, playlists/lyrics, CBR/PDF and AI remain optional later capabilities, and the UI must explain unsupported audio/video formats rather than silently pretend they work. No fabricated demo records appear in real libraries.
