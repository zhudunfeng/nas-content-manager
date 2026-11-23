# External collection API contract

## 1. Scope / Trigger

Applies to `internal/externalapi`, `internal/app/collection_external_*`, `collection_moviepilot.go`, schema v4 and the external automation UI. Extends the collection contract; original download ownership, publication and exact import coverage remain authoritative. Official protocol evidence is under `.trellis/tasks/11-20-collection-external-api/research/`.

## 2. Signatures

- `externalapi.New(Config, *http.Client) (*Client,error)`; `Probe(context.Context) (Info,error)` must run before any list or mutation.
- `(*Client).List(ctx,section,page) (Page,error)`, `Options(ctx) (Options,error)`, `Lookup(ctx,query) ([]Item,error)`.
- `ValidateMutation(Mutation,service) error`, `(*Client).Execute(ctx,Mutation) (Outcome,error)`, `Reconcile(ctx,Mutation) (Outcome,bool,error)`.
- MoviePilot: `MPMediaSearch`, `MPReleases`, `MPDownloads`, `MPRoots`, `MPFiles`, `MPPreview`, `MPLibrary` return safe `MPChoice` values with `Data` excluded from JSON.
- Schema v4 adds `collection_external_connections(source_id,revision,private_ref)`, `collection_external_actions(id,request_id UNIQUE,source_id,source_revision,kind,target_id,state,message,created_at,updated_at,intent_hash,private_ref,receipt_id)`, `collection_external_choices(id,source_id,source_revision,kind,private_ref,expires_at,used_by)`.

## 3. Contracts

### Browser and persistent intent

- Authenticated `/api/collection/sources/{id}/external`; strict 64 KiB JSON and existing same-origin checks. Source exposes `externalService` and `apiConfigured`; input `connection:{baseUrl,apiKey}` is write-only. Blank fields preserve same-service credentials; clear is explicit. API-only sources need no completion directory.
- Fixed routes and exact DTOs are in the task's `api-contract.md` and `docs/api.md`. Never accept arbitrary upstream methods, paths, JSON bodies, job IDs, storage paths or downloader configurations from the browser.
- Before a mutation, bind UUID, source revision, fixed intent and immutable connection/private payload to persistent action. The short transaction consumes the selected snapshot and inserts the action atomically. Do not hold collection locks or database transactions across network calls.
- Repeating an accepted UUID returns the original action, including after source edits or snapshot expiry. Different intent under that UUID is 409. `dispatching|uncertain` blocks new source writes. Restart converts interrupted dispatching to uncertain, never replays it.
- Read-only reconciliation uses original immutable intent and original connection only while the current connection still matches. `checkedExternally:true` records `unconfirmed`, never proven success, and never resends the write.
- Secrets, base URL, raw RSS/torrent context, cookies/passkeys and absolute paths live only in independent private files. Normal SQLite backup and public action/list DTOs contain no such values. Future-version schemas fail startup. Keep v1–v3 media, state and collection tables intact.

### Network and budget

- Inject the existing DNS/connect policy, no environment proxy/cookie jar, reject all redirects, normal TLS verification. Explicit private consent remains required. HTTP credentials may only connect to actual private or loopback addresses, never public HTTP through DNS rebinding.
- Reject base URL userinfo/query/fragment/control characters, escaped separators, backslashes and traversal. Upstream errors use fixed Chinese messages, never raw response text.
- Per-operation app-linked timeout 45 seconds, four network slots, 4 MiB response maximum. No source mutex held across I/O. Bound every array and pagination; do not silently fake empty results on failure.

### Version-specific business facts

- Sonarr 4 and Radarr 7 use `/api/v3`. Support existing root/profile choices, add with automatic search flags off, narrow editor monitor and explicit one-target command. Sonarr 5 writes are unverified.
- MoviePilot 3 basic subscriptions use TMDB/source/type/season identity. `R` and `P` are active, `S` paused. Exact six-stage extension is verified for **3.2.8**; other versions do not inherit it merely by major version.
- AutoBangumi 4 uses Bearer API-scope token, bare lists and message-only mutations. RSS registration is not a complete anime rule. Namespaces are `rss:N` and `bangumi:N`; no fabricated global history/search capability.

### MoviePilot six-stage workflow

- Search metadata → select exact media → search releases → explicit download. Private media/torrent snapshots are not browser-controlled. A direct download receipt does not mean local import.
- Choices expire after 15 minutes, at most 100 per result, 600 active per source and 60 KiB private payload per choice. Consume a selected write snapshot once; keep a separate immutable action payload for recovery after choice expiry.
- Download start/stop binds both actual downloader and hash. Re-read task identity. No downloader-wide operation and no qB takeover.
- Only configured local download/library roots and observed one-level descendants are browsable. Canonicalize remote POSIX/drive paths, reject traversal, UNC and out-of-root entries. Remote storage choices do not grant this application's own filesystem access. MoviePilot remains responsible for resolving remote local-storage links.
- Organization selects one supported video file, verifies size/mtime version, looks up exact TMDB identity and previews `/transfer/manual`. Fixed `copy`, `scrape:false`, `reorganize:false`; no user policy or command passthrough. **ManualTransferItem has no overwrite_mode field**. Require actual target directory policy `never`, reject ambiguous overlapping configuration and re-read just before dispatch. Remote concurrent administrator changes cannot be enclosed by a local transaction.
- Scrape preview is read-only. Actual `/media/scrape/local` overwrites artwork/NFO and requires independent explicit confirmation. Only one selected library video file, no directory tree. A lost synchronous receipt remains uncertain.
- Refresh uses only existing fixed `mediaserver_sync`, observes `/dashboard/schedule/{id}/progress`, executes the fixed scheduler route. It updates MoviePilot's index **from configured media servers**, not Emby/Plex filesystem scan. It has no durable per-run receipt; never infer completion or replay a lost request. `/system/progress/{type}` is SSE, not JSON.
- Transfer background `accepted` means accepted, `completed` means external completion. No invented job ID. Local import remains separate preview/register/scan with actual expected media coverage.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| No session / foreign origin | 401 / 403 before exposing private service data |
| Unknown field, raw payload, wrong choice kind | 400, no business dispatch |
| Stale revision, expired/used choice, UUID intent collision | 409, preserve existing action |
| Known preflight rejection | Safe failed state, no upstream mutation |
| Timeout, malformed write receipt, partial business failure | Uncertain; no blind retry |
| Connection changed before reconciliation | 409 until original connection is restored |
| Unknown service version | No inferred write capability |
| Redirect, metadata/reserved IP, unauthorized private network | Reject before forwarding credentials |
| Target overwrite policy is not never / ambiguous configured roots | Reject organization before dispatch |
| File changed or path escapes configured root | Reject and request fresh selection |
| External download or organization accepted | Describe receipt only, never mark local imported |

## 5. Good / Base / Bad Cases

- Good: search title → confirm TMDB → select private release snapshot → persist UUID → submit once → retain receipt → independent completed-directory registration.
- Base: service offline → useful Chinese error, original media service remains available; unknown MP version → safe basic capabilities without six-stage extension.
- Bad: send browser JSON as upstream payload, guess overwrite_mode body support, pass an arbitrary scheduler ID, call a lost write again, or treat external queue 100% as imported.

## 6. Tests Required

- `scripts/verify_external.py`: real application + four local protocol fixtures, auth/header/body, version gates, genuine v3→v4→v5→v6 preservation, idempotency/revision, uncertain writes and restart, read-only reconciliation/manual acknowledgement, secrets absent from DTO/DB/backup/log, redirects/private policy and complete MoviePilot workflow. Historical fixtures must have the actual old columns, not merely a downgraded `user_version` on the latest schema. Current storage compatibility is defined by the [runtime](runtime-contract.md) and [backup storage](backup-storage-contract.md) contracts; fixture support does not by itself claim this entire suite was rerun for every later slice.
- Temporary Go tests cover nontrivial path/version/envelope/partial-failure and snapshot boundaries, execute then remove per repository instructions. `go test` after removal is compilation evidence only.
- Real browser: one native management dialog, 375/768/1024/1440 and both themes, keyboard focus/Escape, three-step connection, six workflows, directory eligibility, separate empty/offline errors and stable UUID after lost response. Fixture-only UI evidence and real application-backed browser evidence must be distinguished.
- Run collection/download/search/media regressions after schema and app routing changes. Cross-build does not establish Linux runtime, Docker, real NAS storage or actual user service behavior.

## 7. Wrong vs Correct

Wrong: `POST /transfer/manual {overwrite_mode:"never",...}` then show imported; or retry the side-effecting scheduler GET on an uncertain response.

Correct: select configured no-overwrite target → verify one file and metadata → preview → persist immutable action → re-read actual policy → submit fixed copy once → describe accepted/completed fact → independently register and scan. Treat side-effecting GET as a write, including preventing HTTP transport automatic replay.
