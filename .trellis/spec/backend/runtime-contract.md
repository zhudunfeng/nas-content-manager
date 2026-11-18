# Backend runtime contract

## 1. Scope / Trigger

Applies to all changes in `cmd/nas/`, `internal/app/`, Docker environment wiring, or the shared API. This is the actual first-release contract, not a future architecture proposal.

## 2. Signatures

- `app.LoadConfig() (Config, error)` validates all `NAS_*` parameters.
- `app.Run(ctx context.Context, config Config, logger *slog.Logger) error` owns database, HTTP listener, workers, and graceful shutdown.
- `(*App).openSource(item Item) (*os.File, error)` validates the registered canonical library path and opens a relative path through `os.OpenRoot`.
- `(*App).file(w http.ResponseWriter, r *http.Request)` delegates seekable streaming to `http.ServeContent`.
- `validNewPassword(password string) bool` in `auth.go` is the new-administrator password policy; `newPasswordHint` supplies the Chinese rejection message.
- HTTP payloads and paths are defined in `docs/api.md`; Go models are in `models.go`.
- SQLite schema v1 is `schemaV1` in `store.go`; additive `schemaV2` adds downloads, download_files, download_operations. Current `PRAGMA user_version` is 2, even when downloads are disabled. Preserve users, sessions, libraries, items, user_items, progress and jobs. See [Download contract](./download-contract.md) for all download changes.
- Content types are `video/image/comic/novel/music`. Schema v1 type columns have no four-type CHECK; music reuses `duration`, so preserve existing databases without a migration or rebuild.

## 3. Contracts

- Single administrator in v1. Password hashes use bcrypt; session secrets are random and only their hashes are stored. HttpOnly SameSite cookies authenticate every media endpoint.
- `POST /api/setup` accepts only 10–72 characters from U+0021–U+007E (the same count in UTF-8 bytes), containing all four ASCII classes: `A–Z`, `a–z`, `0–9`, and punctuation in U+0021–002F / U+003A–0040 / U+005B–0060 / U+007B–007E. Reject Chinese, whitespace, controls, fullwidth characters and emoji, including when the required four classes are already present. Preserve password bytes without trimming/truncation. Apply this rule only on account creation: `/api/login` must continue matching existing bcrypt hashes, including legacy Chinese passwords, without new complexity/character checks. No schema change or password rewrite is needed; never log passwords or fragments.
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
- Request contexts follow the application lifecycle. Shutdown closes request admission and active connections, then waits for admitted handlers and workers to finish before releasing SQLite and the data lease; a returned `Serve`/`Close` call is insufficient. Windows FFmpeg/ffprobe cancellation must reclaim the command's owned descendants and inherited pipes before workers complete. See [Cross-platform contract](./cross-platform-contract.md) for process ownership and exit regression assertions.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing/expired session on any media API | 401, Chinese JSON error |
| Foreign Origin or cross-site mutation | 403 |
| Invalid/extra JSON fields, bad types/ranges | 400; oversized bodies bounded |
| Setup password outside 10–72 ASCII characters, containing anything outside U+0021–U+007E, or missing any required class | 400 + `newPasswordHint`; no user row or session is created |
| Existing password does not meet new complexity policy | Login still matches the original hash; wrong password remains 401 |
| Setup repeats after the administrator exists, with otherwise valid input | 409; preserve the original account/hash |
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
- Password cases: `Aa1!abcdef` is a valid 10-byte fixture. `Aa1!中文` and `Aa1!abcdef中文` must fail regardless of their byte length or four classes. New passwords with leading/trailing spaces must fail rather than be trimmed. Existing lowercase-only or Chinese passwords remain usable for login.
- Bad: replace a source by a symlink to an outside secret, request `../`, import ZIP traversal entries, or retry a job while another active job owns its unique key. Reject without exposing source contents.
- Hardware acceptance: a sparse fixture proves offsets only. Decode/seek/throughput/RSS for real large video must be measured separately on the target NAS.

## 6. Tests Required

`scripts/verify.py` uses isolated generated samples and asserts auth, cross-origin rejection, root boundaries, scans, all five types, Chinese tags/search, favorites/progress, CBZ natural order, UTF-8/GB18030, EPUB delivery, SQLite snapshot integrity, large-file HEAD/Range/suffix/416/304. Music assertions cover actual audio formats/duration, embedded/no-cover behavior, source bytes, Range/auth, personal state, and idempotent rescans. Lifecycle changes additionally need restart/cancel/retry, mount-unavailable retention, source preservation, and offline restore checks. Record actual execution in `docs/verification.md`.

Regression assertions: cancelling an executing job then retrying returns 409 until its executor is gone, after which it queues once; a source-size change with unchanged mtime changes `coverUrl`; a 4 MiB mostly blank TXT parses without allocating a string descriptor for every newline (review sample total allocation was approximately 13.6 MiB).

Run gofmt on modified Go files, then vet/build/tests. Temporary generated unit tests are executed and removed per user instruction. Do not report an empty `go test` suite as evidence of behavioral coverage.

Password changes require the same input matrix against Go `validNewPassword` and TypeScript `validNewPassword`: 9/10/72/73-byte edges, each missing class, all 32 ASCII punctuation characters, and Chinese (including supplementary Han), fullwidth characters, whitespace, controls and emoji rejected even with all four classes. Use isolated real HTTP setup/login tests to assert invalid input persists no account, valid input returns201 and an HttpOnly session, bcrypt matches the unmodified input, stored legacy weak/Chinese passwords log in, wrong credentials fail, and a repeated setup cannot change the hash. `scripts/verify.py:generate_password() -> str` prefixes random verification credentials with four known classes; reuse it for application setup fixtures, leaving engine/source credentials independent.

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

Password rule example: wrong — use `unicode.IsPunct` (or any non-alphanumeric rune), trim password input, or invoke the new policy in `login`; correct — check explicit ASCII ranges and UTF-8 byte length in `setup`, then hash the original input and preserve legacy login behavior.
