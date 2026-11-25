# Frontend application contract

## 1. Scope / Trigger

Applies to `web/**`, new API consumers, reader behavior, and responsive/theme changes. The app is a media library called 栖库, not a marketing site or an analytics dashboard.

## 2. Signatures

- `api<T>(path: string, method: string = 'GET', body?: unknown, signal?: AbortSignal): Promise<T>` in `src/api.ts` prefixes `/api`, sends same-origin credentials, and raises `ApiError`.
- `errorMessage(error: unknown): string` supplies readable errors; `isAbort(error: unknown): boolean` distinguishes intentional cancellation.
- `validNewPassword(password: string): boolean` and `newPasswordHint: string` in `src/password.ts` define validation for setup and a changed password, with a Chinese accessible hint; keep them consistent with Go `auth.go`. Login and current-password reauthentication retain legacy compatibility.
- DTOs are defined in `src/types.ts` and follow `docs/api.md`.
- `PasswordInput.vue` accepts a required string `v-model`, optional `disabled: boolean` (default false) and `secretLabel: string` (default `密码`); forwards remaining native input attributes/events to the actual input.
- Development API entry points are `/api/**` and `/healthz` on the Vite origin (normally `http://127.0.0.1:5173`); `web/vite.config.ts` forwards them to `http://127.0.0.1:8090` while preserving the original browser Host.
- Collection DTOs, bounded batch constants and safe state/action labels are in `src/types.ts` and `src/collection.ts`. `CollectionCenter.vue` owns collection views and delegates one native dialog at a time.
- Download views use `DownloadCenter.vue`, `AddDownloadDialog.vue`, `DownloadDetailDialog.vue` and the typed multipart helper in `api.ts`; see the backend download contract for upload/selection budgets and publication behavior.
- `ReaderDialog.vue` owns media/reading sessions. `LibraryDialog.vue` selects already mounted directories. `DialogShell.vue` owns the native modal lifecycle and focus restoration.
- `fetchEpub(url: string, signal: AbortSignal): Promise<ArrayBuffer>` and `sanitizeEpubSection(output: string, section: Section): void` isolate the one bounded full-book download workflow.

## 3. Contracts

- Use Vue composition state and computed values; no extra global state framework in v1. Keep server state authoritative, requery after mutation, and persist personal state through APIs.
- Development proxy entries must use explicit `{ target: 'http://127.0.0.1:8090', changeOrigin: false }` objects for both `/api` and `/healthz`, with Chinese comments for each option. Vite's string shorthand enables `changeOrigin: true`, rewriting Host to the backend port while leaving Origin at the frontend port; the backend then correctly rejects an apparent foreign-origin mutation. Preserve Host, Origin, Sec-Fetch-Site, cookies, method and body together. Never fix this mismatch by rewriting/removing Origin, allowing arbitrary CORS, disabling backend CSRF protection, or trusting forwarded headers in the backend.
- `ApiError.status === 401` dispatches `session-expired`. Auth failure returns to login; never silently keep a dead session displayed as active.
- Validate new credentials on setup (`status.setupRequired`) and the account password-change form as 10–72 characters exclusively from U+0021–U+007E (equivalent UTF-8 byte count via `TextEncoder`) with all four ASCII classes `A–Z`, `a–z`, `0–9`, and printable punctuation (U+0021–002F / U+003A–0040 / U+005B–0060 / U+007B–007E). Reject Chinese, fullwidth characters, whitespace, controls and emoji even alongside all required classes. Do not trim/truncate passwords or change confirmation matching. Link the visible `newPasswordHint` to each new-password input with `aria-describedby`; keep this complexity rule and hint off login/current-password reauthentication so existing accounts, including Chinese passwords, remain usable. The backend remains authoritative.
- Use `PasswordInput` for authentication/confirmation passwords and masked external API credentials. Each field starts hidden, toggles only its own `password`/`text` type, and resets to hidden when cleared or remounted. Key the external credential component by service so switching service restores masking even when the old value is already empty and assigning the same empty string cannot trigger a watcher. Preserve model, validation, autocomplete, hint association and submission values. Associate visible labels through input `for`/`id` without enclosing the toggle button in the label. The local SVG toggle uses `type=button`, a Chinese action label/title and `aria-pressed`, follows disabled state, retains visible keyboard focus and a 44px target, and reserves input padding. Do not store/log visibility or credential values, fetch saved credentials for reveal, or submit on toggle.
- `AccountSecurity.vue` mounts only within the existing settings layout. Keep separate accessible forms for password change and revoke-others; use `current-password` autocomplete for current/reverification inputs and `new-password` for new/confirmation. Current-password validation checks at most 72 UTF-8 bytes without adding complexity or nonempty requirements. Clear every temporary password and restore masking after a completed request, on leaving settings and on unmount, including errors; never persist these values. `clearSecrets()` advances a local `secretReset` key shared by the four input component keys: an already-empty field may have been revealed, so assigning `''` again does not trigger its model watcher and cannot alone restore masking. Keep IDs, labels and hint associations on the recreated native inputs.
- Account session reads use one `AbortController` and a generation guard. Refresh starts at the first raw batch; manual continuation replaces the current bounded batch, without infinite accumulation or automatic empty-page chasing. The actual current session is separately pinned by the DTO. Keep list errors independent from mutation errors; display random-ID order and client-category limitations rather than claiming recent-login ordering or trusted devices.
- Account sensitive writes share one busy state. Disable competing password/revocation operations, cancel stale reads and requery after successful non-current revocation. A successful password change/current revocation clears secrets before emitting App's existing session-cleanup event. Conflict409 performs only a read refresh; unknown network outcomes offer read-only confirmation, never automatic mutation replay. `ApiError.retryAfterSeconds` accepts only integer 1..300 from `Retry-After`; it is a display hint, not a retry timer.
- Reuse `DialogShell` for current-session revocation and explain the logout consequence. Native modal tests must prohibit focus on page background controls and confirm Escape restoration. Chromium may legitimately move Tab focus to browser chrome (`document.hasFocus() === false`, active element `BODY`); do not mistake that for interactive background focus or rewrite the shared dialog to satisfy an incorrect test.
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
- Fixed component deployment is embedded in the same source dialog: connect existing or deploy → unchecked storage notice → actual task progress → first configuration → prefilled API connection. Keep container readiness separate from API connectivity, retain UUIDs after lost responses, reject late service responses and never add a nested modal/form. Localhost configuration entries require a local host browser; remote users get forwarding instructions. See the [component deployment contract](../backend/collection-component-contract.md).
- Render only capabilities returned by the server. MoviePilot basic subscriptions and exact3.2.8 six-stage workflow are distinct. Search metadata then releases, explicitly confirm a download/subscription; never perform write operations merely by opening the panel. Only opaque server choice IDs go into workflow mutations. Changing file, target, TMDB ID or media type clears the old plan and scrape confirmation. Accepted actions invalidate consumed choices.
- Organization exposes only `organizeAllowed===true` targets and previews fixed copy without automatic scrape. Scrape requires a separate unchecked overwrite acknowledgement. Label refresh as “刷新 MoviePilot 媒体库索引” and explain that it syncs configured media-server indexes, without implying Emby/Plex filesystem scan or local NAS import. Keep external receipt, external completion and local import separate.
- Keep one stable UUID for the same pending intent after a lost response. An uncertain action offers read-only reconciliation or explicit external-check acknowledgement recorded as `unconfirmed`; never relabel it succeeded. Actual HTTP503 retains old lists as snapshots and remains distinct from empty success. Stale revision409 requires reopening/reloading, without dispatching the obsolete confirmation. Download state labels accept controlled Chinese values; unknown states retain explicit pause/continue choices.

### JMComic advanced flow

`CollectionJMComicDialog` keeps one native modal. Search exposes fixed `orderBy`
and `time` enums from the backend contract; filter changes abort old requests and
clear results. Retain legacy query/page defaults. Do not expose ignored categories.

Preview metadata includes chapter IDs/titles/counts and selected total. Any chapter
change invalidates the confirmation preview and requires another server preview.
Show an over-budget catalogue read-only and disable task submission; selecting a
smaller subset must still obtain a fresh preview. Never submit chapter choices
through `/tasks` or treat local totals as download authority.

Batch accepts at most 10 unique numeric full-album IDs, uses bounded sequential
preview/submission and shows each result. Freeze an independent complete task body
and UUID before each submit; unknown transport/5xx outcomes retain that body and
lock source/target/mode. Retry it even if preview expired before refreshing any
preview. Keep accepted tasks visible without immediately closing the whole batch,
and never resubmit them. Closing tells users to reconcile in the durable task list.

Task detail may show a native `<progress>` with verified pages, bytes including
reuse and reused-page count. No invented network speed or ETA. Preserve existing
download/publish/import states, abort/poll lifecycle and optional-worker fallback.
Test real application flows separately from intercepted UI-only failure/expiry
responses, at 375/768/1024/1440 widths in both themes.

## 4. Validation & Error Matrix

Collection adds overview/sources/subscriptions/candidates/tasks without adding a sixth mobile navigation entry. Keep query failures independent from successful empty data. Show actual worker/tool readiness; registration roots and worker copy roots are distinct. RSS secrets are write-only. Automatic subscriptions require explicit authorization and complete deterministic rules. Web batches contain at most ten independent sequential previews/submissions; retain each accepted task and retry only failed entries with their original immutable plan and UUID. Never accept tool arguments or cookies in the UI.

Collection acquisition and import states remain separate; only server item IDs enable media navigation. `retry_import` must never be described as re-downloading. BT selection/actions jump into the existing download details, and imported media returns through App's shared detail/reader lifecycle. Unmount the current native dialog and await `nextTick` before opening another. Guard delayed dry-run responses from creating nested dialogs. Pause polling while hidden and retain higher revisions when list/detail responses overlap.

| Condition | Behavior |
| --- | --- |
| setupRequired | Create first administrator; then offer directory selection |
| New password missing any required class, outside length limits or containing anything outside U+0021–U+007E | Show `newPasswordHint`; do not submit setup |
| Existing-account login with a password that fails new complexity | Send original password to login; let stored-hash matching decide |
| Password/credential reveal or hide; clear or reopen | Toggle only the selected field without submitting; clear/remount restores hidden state, disabled blocks editing and toggle |
| Request failure | Chinese error and retry/action; do not silently show empty success |
| Same-origin mutation through the Vite development proxy | Preserve browser Host and Origin; return the backend route's normal status, without a false cross-site 403 |
| Foreign Origin, another frontend port in Origin, or Sec-Fetch-Site=cross-site through the proxy | Preserve rejection: backend returns 403 with the Chinese cross-site error |
| Obsolete aborted request | Ignore its error, do not overwrite current data |
| Wrong current password on account mutation | Keep the actual login on HTTP400, show its field error and clear all temporary password inputs after the request |
| Account credential conflict409 | Clear secrets and refresh only read state; do not automatically repeat a sensitive write |
| Account reauthentication429 | Show bounded `Retry-After` seconds when present; no automatic retry or persistent secret timer |
| Password change/current-session revocation succeeds | Clear secrets before invoking App's existing session cleanup and show the explicit re-login message |
| Empty account batch with non-null cursor | Keep the explicit continuation; never auto-fetch until a valid item appears |
| Native media error | Explain format/codec limits; no invented transcode promise |
| Missing/offline source | Display server error and recovery action |
| Invalid/large EPUB or unsupported TXT | Explicit limit message, clean up partial session |
| Reader closed / route changed | Save latest progress, release media, abort fetches |
| Session expires while open | Return to auth and close privileged content |

## 5. Good / Base / Bad Cases

- Good: create account, add real directory, open scanned comic, switch direction/mode, close, reopen at saved page.
- Base: browse/search/filter/favorite/tag in either theme at all target widths without horizontal overflow.
- Development proxy base: same-origin POST/PUT/PATCH/DELETE to an intentionally absent `/api/...` route returns the backend's 404, proving the request passed source validation without changing real data. Bad: replacing Origin with a foreign origin or setting Sec-Fetch-Site=cross-site still returns 403.
- Password cases: `Aa1!abcdef` meets the 10-byte minimum; `Aa1!中文` fails even though it is 10 UTF-8 bytes and contains all classes; an existing weak or Chinese password must reach `/api/login` without a setup-policy error.
- Reveal base: password and confirmation remain independent, repeated mouse/Enter/Space toggles preserve the original string, and clearing/reopening returns to `type=password`. Disabled API credentials have a disabled toggle; no reveal action creates a network mutation.
- Bad: book contains scripts/forms/external URLs, malformed ZIP central directory, or a video lacks supported codecs. Reject or explain within the reader; never execute content or silently claim success.

## 6. Tests Required

- Strict vue-tsc and production Vite build.
- Password input changes require browser checks of login/setup/confirmation and the real external-source dialog at 375/768/1024/1440 in both themes. Assert initial mask, independent reversible toggles, unchanged value/attributes, clear/remount reset, service-switch reset for empty and nonempty revealed credentials, disabled, exact label/hint associations, keyboard focus and activation, 44px targets, reserved text padding, no overflow and no unintended submission. API fixtures may safely inspect original submission values and retained setup validation without modifying user data; report them as UI evidence rather than real authentication/backend success. Remove temporary test sources after recording results.
- Proxy configuration changes require real Vite-to-Go HTTP checks for same-origin mutations, foreign-Origin rejection, same-host/different-port rejection, cross-site fetch rejection, API reads and healthz. Use intentionally absent API routes on existing user instances to avoid data mutations; an isolated Vite/upstream fixture must additionally assert unchanged Host, Origin, Sec-Fetch-Site, cookie and body for both proxy prefixes. Keep generated fixtures under `.runtime/verify/`, stop only owned processes and remove temporary test sources after recording results.
- Collection browser coverage includes real source create/edit, local preview→task→actual imported item, RSS candidate→original BT detail, empty/error/partial states, fixed request retries, four widths and both themes, modal focus/Escape and five mobile bottom entries. Controlled route responses test presentation only and must not be reported as real webpage acquisition.
- Collection dialogs retain body padding matching the existing library form (26px desktop,18px mobile). An overflow assertion alone does not catch edge-flush fields or buttons; inspect actual screenshots after modal layout changes.
- External browser coverage includes all six MoviePilot workflows, three-step connection, four-service capability differences, stable UUID recovery, stale revision/plan invalidation, offline recovery and single-modal focus/Escape. Test 375/768/1024/1440 in both themes. Distinguish route-intercepted presentation fixtures from real application→adapter→local protocol fixture evidence; neither proves a real user instance.
- Password rules use the same boundary/category matrix as Go, including all ASCII punctuation and invalid Chinese/supplementary Han/fullwidth/whitespace/control/emoji additions, especially trailing LF and CRLF. Browser checks must show weak and Chinese setup blocked without a POST, valid ASCII setup returning201, logout/login preserving input, no setup hint on login, accessible hint association and no hint overflow at all target widths/themes.
- Account browser checks use two actual contexts, real login-generated pagination, wrong-current400 preserving login, specified/current/other revocation and password change invalidating old cookies. Reveal an empty field in the other form, submit a real failing password request and verify all four recreated inputs are empty and masked; a test that only clears nonempty values misses the unchanged-model boundary. Route fixtures for empty continuations,503,409,429, delayed reads and uncertain network outcomes are presentation evidence only. Count them separately, clear secrets before screenshots, inspect original in-memory console/DTO/storage without persisting raw secret-bearing diagnostics, and bind results to both binary and copied frontend asset identity.
- Browser workflow for initial setup/login, directory browser, all five content types, favorites/tags, jobs, settings/backup, logout.
- Save/reopen progress for video/music/comic/TXT/EPUB; check request errors and console output. Music additionally needs early-close/error without losing progress, direct byte requests, no audio src after close, and reachable mobile More entries. Delay progress responses to assert one normal in-flight write, coalescing to the latest seek position, immediate source release on close, and immediate keepalive dispatch during pagehide.
- EPUB regressions cover a valid book, 5001 actual entries with EOCD counts forged to 1, truncated directory records, ZIP64/split/encrypted archives, invalid local offsets, declared/actual expansion budgets, abort, and download HTTP 401. JSZip tolerates mismatched directory counts, so a successful library parse is not sufficient validation.
- At widths 375/768/1024/1440 inspect overflow, navigation, modal sizing, large/long titles, touch controls, light/dark themes, visible keyboard focus, and reduced motion.
- Check that opening video triggers Range streaming, not a whole-file fetch, and closing removes the source.
- Download flows require delayed/failed pagination, non-contiguous IDs, 20,000-file confirmation within 256 KiB, idempotent retry, locked read-only selection, offline snapshots, imported item navigation, mobile More and both themes at all target widths.
- Search flows require cancellation/late-response isolation, nullable metadata, partial/empty/all-failed states, local sorting/pagination, expiry disabling, stable add retries, focus restoration and a real application result-to-selection handoff. Source credentials and private links must be absent from browser responses and console output.

Record executed evidence and remaining device limitations in `docs/verification.md`. Browser checks are not a substitute for real NAS throughput measurements.

## 7. Wrong vs Correct

Development proxy: wrong — `'/api': 'http://127.0.0.1:8090'`, because Vite changes Host; correct — `'/api': { target: 'http://127.0.0.1:8090', changeOrigin: false }`, preserving the browser origin authority expected by `App.security`. Apply the same explicit options to `/healthz`.

```ts
// 错误：大视频下载完成才可播放，浏览器内存随影片大小增长。
video.src = URL.createObjectURL(await (await fetch(url)).blob())

// 正确：浏览器按需请求原文件，服务端提供鉴权 Range。
video.src = `/api/items/${item.id}/file`
video.preload = 'metadata'
```

Use explicit parameter/return types and Chinese doc comments for named functions; comment entity fields and configuration entries. Avoid `any` and type-safety suppression. Do not erase errors with empty catches; cleanup-only catches must explain their narrow purpose.

Password rule example: wrong — `password.length` as a UTF-8 byte count, `\W` as the punctuation rule, or complexity validation on every authentication; correct — `TextEncoder().encode(password).byteLength`, explicit ASCII classes for setup/new-password values, and sending the original current/login password unchanged.

Password reveal: wrong — a default submit button inside a label containing both input and toggle, shared reveal state across fields, or replacing/trimming the bound secret. Correct — a labeled native input in `PasswordInput` with an independent default-hidden state, `type=button` toggle, and unchanged string model.

For the character whitelist, reject any match of `/[^\x21-\x7e]/`. JavaScript `$` can match before a final line terminator, so a positive `^[\x21-\x7e]+$` test alone can accept an otherwise valid password followed by LF; keep explicit trailing-LF/CRLF regression cases.
