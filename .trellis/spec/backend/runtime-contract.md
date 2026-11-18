# Backend runtime contract

## 1. Scope / Trigger

Applies to all changes in `cmd/nas/`, `internal/app/`, Docker environment wiring, or the shared API. This is the actual first-release contract, not a future architecture proposal.

## 2. Signatures

- `app.LoadConfig() (Config, error)` validates all `NAS_*` parameters.
- `app.Run(ctx context.Context, config Config, logger *slog.Logger) error` owns database, HTTP listener, workers, and graceful shutdown.
- `(*App).openSource(item Item) (*os.File, error)` validates the registered canonical library path and opens a relative path through `os.OpenRoot`.
- `(*App).file(w http.ResponseWriter, r *http.Request)` delegates seekable streaming to `http.ServeContent`.
- HTTP payloads and paths are defined in `docs/api.md`; Go models are in `models.go`.
- SQLite schema v1 is `schemaV1` in `store.go`; additive `schemaV2` adds downloads, download_files, download_operations. Current `PRAGMA user_version` is 2, even when downloads are disabled. Preserve users, sessions, libraries, items, user_items, progress and jobs. See [Download contract](./download-contract.md) for all download changes.
- Content types are `video/image/comic/novel/music`. Schema v1 type columns have no four-type CHECK; music reuses `duration`, so preserve existing databases without a migration or rebuild.

## 3. Contracts

- Single administrator in v1. Password hashes use bcrypt; session secrets are random and only their hashes are stored. HttpOnly SameSite cookies authenticate every media endpoint.
- JSON uses camelCase; collection fields are `[]`, not `null`; API failures are `{ "error": "Chinese explanation" }`.
- Database lives at `NAS_DATA_DIR/library.db` on local storage, with WAL, foreign keys, busy timeout, and one connection. Never hold a transaction while probing or scanning files.
- `UNIQUE(library_id, relative_path)` makes scans idempotent. User tags/favorites/progress are stored separately and survive rescans. Removal deletes index state only.
- Jobs persist `queued/running/completed/failed/cancelled`. Startup returns interrupted running jobs to queued. Active scan/enrichment jobs have partial unique indexes; cancellation is persisted and also signals the worker context.
- Job claim/handle registration, cancellation/status update, completion/handle removal, and retry eligibility share `App.mu`. `POST /api/jobs/{id}/retry` returns 409 while the old executor is still stopping, even if its persisted status is already cancelled. A late cancellation must never signal a new retry's handle. Never requeue a cancelled enrichment automatically.
- A failed or inaccessible scan cannot mark unseen entries missing. An entirely empty scan conservatively retains old index records because an empty mount can mean disconnected storage.
- Runtime defaults: `NAS_ADDR=:8090`, `NAS_DATA_DIR=data`, `NAS_MEDIA_ROOTS=media`, `NAS_WEB_DIR=web/dist`, `NAS_SCAN_INTERVAL_MINUTES=60`, `NAS_CACHE_MAX_MB=1024`, `NAS_MIN_FREE_MB=512`, `NAS_WORKERS=1`, `NAS_COOKIE_SECURE=false`. Media roots use the platform path-list separator.
- No full-media hash or transcode blocks initial registration. FFmpeg/probe work is asynchronous and bounded. Cache is derived data; never rewrite source media.
- Music extensions/MIME are defined centrally by `audioMIME`; OGG/OPUS use `audio/ogg`, AIF/AIFF use `audio/aiff`. Probe music duration and extract only attached artwork with the existing time/output/pixel budgets. No artwork is a successful `no_cover` enrichment result, not a failed job; unchanged rescans must not repeatedly probe those files. File changes return it to pending.
- TXT: 16 MiB source, 100,000 paragraphs; CBZ: 2 GiB, 10,000 pages, 32 MiB/page, bounded decompression. Path traversal, symlinks outside roots, encrypted/unsupported archives are rejected. Raster formats only; no active SVG media.
- TXT iterates `strings.Lines` before collecting non-empty paragraphs; do not `strings.Split` the complete input, since a bounded all-newline file otherwise allocates millions of string descriptors. Cover URLs include source mtime, size, and enrichment state, matching cache invalidation when sync tools preserve mtime.
- File length/offset fields use `int64`. Do not install compression/buffering middleware or a short global `WriteTimeout` over audio/video responses. A size/mtime ETag is weak, not a byte identity hash.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing/expired session on any media API | 401, Chinese JSON error |
| Foreign Origin or cross-site mutation | 403 |
| Invalid/extra JSON fields, bad types/ranges | 400; oversized bodies bounded |
| Browse outside configured media roots | 400/403; no directory content disclosure |
| Missing source, changed canonical mount | 404; no arbitrary fallback path |
| Invalid or out-of-file byte range | 416 from ServeContent |
| Valid partial bytes | 206 + exact Content-Range/Length |
| Oversized TXT / unsafe CBZ | Explicit 413/422; bounded work |
| Insufficient cache/backup space | Fail derived work; preserve source |
| Retry before cancelled executor has exited | 409; keep cancelled state until the existing handle is removed |
| Newer database schema | Refuse startup rather than guessing a downgrade |

## 5. Good / Base / Bad Cases

- Good: a 5+ GiB sparse protocol fixture returns exact bytes beyond offset 4 GiB; the application does not allocate a file-sized buffer.
- Base: repeat a scan, keep the same item IDs/count and user state; read all five media types through an authenticated session.
- Bad: replace a source by a symlink to an outside secret, request `../`, import ZIP traversal entries, or retry a job while another active job owns its unique key. Reject without exposing source contents.
- Hardware acceptance: a sparse fixture proves offsets only. Decode/seek/throughput/RSS for real large video must be measured separately on the target NAS.

## 6. Tests Required

`scripts/verify.py` uses isolated generated samples and asserts auth, cross-origin rejection, root boundaries, scans, all five types, Chinese tags/search, favorites/progress, CBZ natural order, UTF-8/GB18030, EPUB delivery, SQLite snapshot integrity, large-file HEAD/Range/suffix/416/304. Music assertions cover actual audio formats/duration, embedded/no-cover behavior, source bytes, Range/auth, personal state, and idempotent rescans. Lifecycle changes additionally need restart/cancel/retry, mount-unavailable retention, source preservation, and offline restore checks. Record actual execution in `docs/verification.md`.

Regression assertions: cancelling an executing job then retrying returns 409 until its executor is gone, after which it queues once; a source-size change with unchanged mtime changes `coverUrl`; a 4 MiB mostly blank TXT parses without allocating a string descriptor for every newline (review sample total allocation was approximately 13.6 MiB).

Run gofmt on modified Go files, then vet/build/tests. Temporary generated unit tests are executed and removed per user instruction. Do not report an empty `go test` suite as evidence of behavioral coverage.

## 7. Wrong vs Correct

```go
// 错误：视频整体读入内存，空间随原片大小增长。
data, _ := os.ReadFile(path)
w.Write(data)

// 正确：完成鉴权与目录边界检查后使用文件句柄。
file, err := a.openSource(item)
if err != nil { fail(w, 404, err.Error()); return }
defer file.Close()
info, err := file.Stat()
if err != nil { fail(w, 404, "原文件不可访问"); return }
http.ServeContent(w, r, info.Name(), info.ModTime(), file)
```

Do not use `io.ReadAll` for video. Bounded reads for TXT or one comic page are separate, explicitly limited workflows. A running WAL database must be backed up with consistent SQLite snapshot semantics, not by copying its main file.
