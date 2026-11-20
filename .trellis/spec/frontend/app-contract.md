# Frontend application contract

## 1. Scope / Trigger

Applies to `web/**`, new API consumers, reader behavior, and responsive/theme changes. The app is a media library called 栖库, not a marketing site or an analytics dashboard.

## 2. Signatures

- `api<T>(path: string, method: string = 'GET', body?: unknown, signal?: AbortSignal): Promise<T>` in `src/api.ts` prefixes `/api`, sends same-origin credentials, and raises `ApiError`.
- `errorMessage(error: unknown): string` supplies readable errors; `isAbort(error: unknown): boolean` distinguishes intentional cancellation.
- `validNewPassword(password: string): boolean` and `newPasswordHint: string` in `src/password.ts` define setup-only validation and its Chinese accessible hint; keep them consistent with Go `auth.go`.
- DTOs are defined in `src/types.ts` and follow `docs/api.md`.
- Collection DTOs, bounded batch constants and safe state/action labels are in `src/types.ts` and `src/collection.ts`. `CollectionCenter.vue` owns collection views and delegates one native dialog at a time.
- Download views use `DownloadCenter.vue`, `AddDownloadDialog.vue`, `DownloadDetailDialog.vue` and the typed multipart helper in `api.ts`; see the backend download contract for upload/selection budgets and publication behavior.
- `ReaderDialog.vue` owns media/reading sessions. `LibraryDialog.vue` selects already mounted directories. `DialogShell.vue` owns the native modal lifecycle and focus restoration.
- `fetchEpub(url: string, signal: AbortSignal): Promise<ArrayBuffer>` and `sanitizeEpubSection(output: string, section: Section): void` isolate the one bounded full-book download workflow.

## 3. Contracts

- Use Vue composition state and computed values; no extra global state framework in v1. Keep server state authoritative, requery after mutation, and persist personal state through APIs.
- `ApiError.status === 401` dispatches `session-expired`. Auth failure returns to login; never silently keep a dead session displayed as active.
- Only when `status.setupRequired` is true, validate the new password as 10–72 characters exclusively from U+0021–U+007E (equivalent UTF-8 byte count via `TextEncoder`) with all four ASCII classes `A–Z`, `a–z`, `0–9`, and printable punctuation (U+0021–002F / U+003A–0040 / U+005B–0060 / U+007B–007E). Reject Chinese, fullwidth characters, whitespace, controls and emoji even alongside all required classes. Do not trim/truncate passwords or change confirmation matching. Link the visible `newPasswordHint` to the setup input with `aria-describedby`; keep this rule and hint off the login flow so existing accounts, including Chinese passwords, remain usable. The backend remains authoritative.
- Abort superseded list/reader requests, debounce search, preserve the list position when closing a reader, and clear timers/observers/media on unmount.
- Native video/audio use `/api/items/{id}/file`, `preload=metadata`, and direct browser byte requests. Never fetch full audio/video into Blob/ArrayBuffer. Music uses the shared Item `duration` and optional cover; format support is determined by the browser.
- Video/music use seconds, comics zero-based pages, TXT zero-based paragraphs, and EPUB CFI in `Progress.locator`. Preserve these units across UI/API. Media progress must not be written before metadata restoration is complete; early close/error cannot overwrite a saved position with zero. Closing must pause, clear src, and release the player.
- Normal progress saves allow one in-flight request and one latest pending snapshot; merge intermediate positions instead of accumulating a Promise per event. Each write has a four-second timeout. Close releases the media immediately, then awaits the in-flight and latest final save. `pagehide` starts a keepalive request immediately, without waiting for the queue; unloading remains best effort, without a server-side ordering or delivery guarantee.
- EPUB uses a dynamic local import, scripts disabled, sanitized sections, restrictive CSP, and archive preflight limits: 64 MiB download, 5000 entries, 32 MiB/entry, 256 MiB expanded total. No untrusted content may request application endpoints or remote resources.
- ZIP preflight selects the last EOCD signature and requires its comment to end at EOF, both disk numbers to be zero, both entry counts to agree without ZIP64 sentinels, and `centralOffset + centralSize === eocdOffset`. Walk the actual directory byte boundary, not the untrusted declared count; check each record boundary, actual count, encryption/size limits, and non-overlapping local header/data ranges before JSZip.
- `fetchEpub` awaits native streaming inflation of every entry before JSZip. Count actual bytes against the declared, per-entry and aggregate limits without retaining expanded output; feed at most 1 KiB compressed chunks, yield and propagate cancellation. A browser without safe raw-DEFLATE streaming gets an explicit unsupported message, with no unbounded fallback. JSZip only checks size mismatch after emitting output and cannot provide this resource boundary alone.
- EPUB binary input uses `book.open(buffer, "binary")`; `"epub"` selects a URL path and can incorrectly fetch SPA HTML. Keep the rendition host mounted while the async book load is busy, and bound parsing with an abort-aware timeout.
- `fetchEpub` must dispatch the same `session-expired` event as the API client on HTTP 401.
- Keep library/stat/job load failures separate from list results. A successful list request cannot erase a metadata error or turn unavailable metadata into an empty-library success state.
- Directory browsing aborts the previous request and only accepts the latest request result; disable submission while it loads.
- Use text interpolation for titles/TXT; never `v-html` raw content. Media URLs come from the same-origin backend.
- Theme values live in semantic CSS variables. Dark graphite/teal and light neutral/teal; system fonts and SVG icons are local. Respect reduced motion and visible focus.
- Desktop has sidebar/global search; mobile has at most five primary bottom entries (home/video/music/image/more). The native More dialog exposes all content types and management with focus restoration. Music cards/artwork are square, keeping the existing semantic theme and local icons. Readers are immersive, maintain usable touch controls, and do not leave background scroll unlocked.
- Empty/loading/errors are distinct, with explicit actions. Do not create fake records to make a new library look populated.
- `ViewName` may include downloads; `ContentType` stays the five existing media types. Download center is in the desktop sidebar and mobile More dialog. Keep the five mobile bottom entries.
- Download status/transfer/import are independent; offline views label old snapshots and suppress live speed/ETA claims. Selection uses actual engine file IDs and manifestVersion. Do not enable confirmation until the entire paginated selectable manifest has loaded; clear it on version changes. Confirmed selection is locked. Every add/action retries with its original requestId, and optimistic actions include revision.
- Poll with one in-flight request, schedule the next after completion, abort on unmount/filter changes and discard stale responses. Detail and list requests must not overwrite newer data. Return imported Item objects through the existing reader lifecycle rather than adding another media player.
- Restored management/download hashes must wait for the app session before privileged metadata/settings requests. Disabling downloads stops mutations and engine coordination but still loads local historical tasks and read-only details.
- `TorrentSearchDialog.vue` uses the typed Torznab DTOs and manual submission, aborts superseded requests and rejects late responses. Distinguish unconfigured, initial, empty, partial failure, all failed and expired states; sort/page the bounded snapshot locally (20 rows/page). Unknown numbers remain unknown. Use text interpolation, never links or HTML from a source. See the [torrent search contract](../backend/torrent-search-contract.md).
- Selecting a search result first unmounts the search modal, then opens `AddDownloadDialog` after `nextTick`. Its optional `searchResult` prop sends only `searchResultId` through the original confirmation flow. Preserve the requestId on retry and reset it on result/type/library changes. Engine offline allows searching but disables adding with a reason. Do not nest native dialogs or add mobile navigation entries.
- `EngineStatusCards` and `EngineSetupDialog` distinguish helper availability, resource presence, runtime and actual app readiness. See [engine bootstrap contract](../backend/engine-bootstrap-contract.md). No GET/open action installs engines. Never preselect qB notice; persist exact requestId/revision across uncertain POST retries. Clear task events/cursor atomically when task ID changes; a404 clears a stale recovery ID. Layer progress needs a real denominator, never an overall percentage. Search→engine management closes the first native modal before opening the next; retain focus restoration, one in-flight polling request, hidden-page pause and stale-response guards.
- External connection uses three main steps: select service → enter address/credential → test connection. No API-version selector; optional completion-directory mapping stays folded. `CollectionExternalPanel.vue` owns one native management dialog and shared confirmation/action recovery; `CollectionMoviePilotWorkflow.vue` is embedded in it. Preserve the external application entry and independent completed-directory preview/import.
- Render only capabilities returned by the server. MoviePilot basic subscriptions and exact3.2.8 six-stage workflow are distinct. Search metadata then releases, explicitly confirm a download/subscription; never perform write operations merely by opening the panel. Only opaque server choice IDs go into workflow mutations. Changing file, target, TMDB ID or media type clears the old plan and scrape confirmation. Accepted actions invalidate consumed choices.
- Organization exposes only `organizeAllowed===true` targets and previews fixed copy without automatic scrape. Scrape requires a separate unchecked overwrite acknowledgement. Label refresh as “刷新 MoviePilot 媒体库索引” and explain that it syncs configured media-server indexes, without implying Emby/Plex filesystem scan or local NAS import. Keep external receipt, external completion and local import separate.
- Keep one stable UUID for the same pending intent after a lost response. An uncertain action offers read-only reconciliation or explicit external-check acknowledgement recorded as `unconfirmed`; never relabel it succeeded. Actual HTTP503 retains old lists as snapshots and remains distinct from empty success. Stale revision409 requires reopening/reloading, without dispatching the obsolete confirmation. Download state labels accept controlled Chinese values; unknown states retain explicit pause/continue choices.

## 4. Validation & Error Matrix

Collection adds overview/sources/subscriptions/candidates/tasks without adding a sixth mobile navigation entry. Keep query failures independent from successful empty data. Show actual worker/tool readiness; registration roots and worker copy roots are distinct. RSS secrets are write-only. Automatic subscriptions require explicit authorization and complete deterministic rules. Web batches contain at most ten independent sequential previews/submissions; retain each accepted task and retry only failed entries with their original immutable plan and UUID. Never accept tool arguments or cookies in the UI.

Collection acquisition and import states remain separate; only server item IDs enable media navigation. `retry_import` must never be described as re-downloading. BT selection/actions jump into the existing download details, and imported media returns through App's shared detail/reader lifecycle. Unmount the current native dialog and await `nextTick` before opening another. Guard delayed dry-run responses from creating nested dialogs. Pause polling while hidden and retain higher revisions when list/detail responses overlap.

| Condition | Behavior |
| --- | --- |
| setupRequired | Create first administrator; then offer directory selection |
| New password missing any required class, outside length limits or containing anything outside U+0021–U+007E | Show `newPasswordHint`; do not submit setup |
| Existing-account login with a password that fails new complexity | Send original password to login; let stored-hash matching decide |
| Request failure | Chinese error and retry/action; do not silently show empty success |
| Obsolete aborted request | Ignore its error, do not overwrite current data |
| Native media error | Explain format/codec limits; no invented transcode promise |
| Missing/offline source | Display server error and recovery action |
| Invalid/large EPUB or unsupported TXT | Explicit limit message, clean up partial session |
| Reader closed / route changed | Save latest progress, release media, abort fetches |
| Session expires while open | Return to auth and close privileged content |

## 5. Good / Base / Bad Cases

- Good: create account, add real directory, open scanned comic, switch direction/mode, close, reopen at saved page.
- Base: browse/search/filter/favorite/tag in either theme at all target widths without horizontal overflow.
- Password cases: `Aa1!abcdef` meets the 10-byte minimum; `Aa1!中文` fails even though it is 10 UTF-8 bytes and contains all classes; an existing weak or Chinese password must reach `/api/login` without a setup-policy error.
- Bad: book contains scripts/forms/external URLs, malformed ZIP central directory, or a video lacks supported codecs. Reject or explain within the reader; never execute content or silently claim success.

## 6. Tests Required

- Strict vue-tsc and production Vite build.
- Collection browser coverage includes real source create/edit, local preview→task→actual imported item, RSS candidate→original BT detail, empty/error/partial states, fixed request retries, four widths and both themes, modal focus/Escape and five mobile bottom entries. Controlled route responses test presentation only and must not be reported as real webpage acquisition.
- Collection dialogs retain body padding matching the existing library form (26px desktop,18px mobile). An overflow assertion alone does not catch edge-flush fields or buttons; inspect actual screenshots after modal layout changes.
- External browser coverage includes all six MoviePilot workflows, three-step connection, four-service capability differences, stable UUID recovery, stale revision/plan invalidation, offline recovery and single-modal focus/Escape. Test 375/768/1024/1440 in both themes. Distinguish route-intercepted presentation fixtures from real application→adapter→local protocol fixture evidence; neither proves a real user instance.
- Password rules use the same boundary/category matrix as Go, including all ASCII punctuation and invalid Chinese/supplementary Han/fullwidth/whitespace/control/emoji additions, especially trailing LF and CRLF. Browser checks must show weak and Chinese setup blocked without a POST, valid ASCII setup returning201, logout/login preserving input, no setup hint on login, accessible hint association and no hint overflow at all target widths/themes.
- Browser workflow for initial setup/login, directory browser, all five content types, favorites/tags, jobs, settings/backup, logout.
- Save/reopen progress for video/music/comic/TXT/EPUB; check request errors and console output. Music additionally needs early-close/error without losing progress, direct byte requests, no audio src after close, and reachable mobile More entries. Delay progress responses to assert one normal in-flight write, coalescing to the latest seek position, immediate source release on close, and immediate keepalive dispatch during pagehide.
- EPUB regressions cover a valid book, 5001 actual entries with EOCD counts forged to 1, truncated directory records, ZIP64/split/encrypted archives, invalid local offsets, declared/actual expansion budgets, abort, and download HTTP 401. JSZip tolerates mismatched directory counts, so a successful library parse is not sufficient validation.
- At widths 375/768/1024/1440 inspect overflow, navigation, modal sizing, large/long titles, touch controls, light/dark themes, visible keyboard focus, and reduced motion.
- Check that opening video triggers Range streaming, not a whole-file fetch, and closing removes the source.
- Download flows require delayed/failed pagination, non-contiguous IDs, 20,000-file confirmation within 256 KiB, idempotent retry, locked read-only selection, offline snapshots, imported item navigation, mobile More and both themes at all target widths.
- Search flows require cancellation/late-response isolation, nullable metadata, partial/empty/all-failed states, local sorting/pagination, expiry disabling, stable add retries, focus restoration and a real application result-to-selection handoff. Source credentials and private links must be absent from browser responses and console output.

Record executed evidence and remaining device limitations in `docs/verification.md`. Browser checks are not a substitute for real NAS throughput measurements.

## 7. Wrong vs Correct

```ts
// 错误：大视频下载完成才可播放，浏览器内存随影片大小增长。
video.src = URL.createObjectURL(await (await fetch(url)).blob())

// 正确：浏览器按需请求原文件，服务端提供鉴权 Range。
video.src = `/api/items/${item.id}/file`
video.preload = 'metadata'
```

Use explicit parameter/return types and Chinese doc comments for named functions; comment entity fields and configuration entries. Avoid `any` and type-safety suppression. Do not erase errors with empty catches; cleanup-only catches must explain their narrow purpose.

Password rule example: wrong — `password.length` as a UTF-8 byte count, `\W` as the punctuation rule, or complexity validation on every authentication; correct — `TextEncoder().encode(password).byteLength`, explicit ASCII classes and setup-only validation, sending the original password unchanged.

For the character whitelist, reject any match of `/[^\x21-\x7e]/`. JavaScript `$` can match before a final line terminator, so a positive `^[\x21-\x7e]+$` test alone can accept an otherwise valid password followed by LF; keep explicit trailing-LF/CRLF regression cases.
