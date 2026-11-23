# Resource collection contract

## 1. Scope / Trigger

Applies to `internal/app/collection_*`, `internal/collectioncontract`, `internal/collectionworker`, `cmd/nas-collection-worker`, collection UI, Compose overlays, desktop immutable images and the schema-v3 collection foundation. This explicitly extends the earlier RSS exclusion. Keep the original qB ownership, selection, revision and publication contracts authoritative for BT. External automation tools remain independent owners; the current schema-v4 API integration extends this foundation through the [external collection contract](external-collection-contract.md).

## 2. Signatures

- `collectionworker.LoadConfig() (Config, error)`; `collectionworker.Run(ctx context.Context, config Config, logger *slog.Logger) error`.
- `collectionworker.New(ctx context.Context, config Config, logger *slog.Logger) (*Service, error)`; `(*Service).Handler() http.Handler`; `(*Service).Close() error` owns shutdown.
- `(*App).startCollections()`, `(*App).runCollectionTask(ctx context.Context, task collectionTaskRecord)`, `(*App).importCollectionTask(ctx context.Context, task collectionTaskRecord)`.
- `(*App).addCollectionGate(ctx context.Context, gate *downloadGate, roots map[string]bool) error` integrates into every existing file/scan/cover access gate.
- Schema v3 adds `collection_sources`, `collection_subscriptions`, `collection_candidates`, `collection_previews`, `collection_tasks`, `collection_task_events`. Schema v4 adds external connection/action/choice references; schema 5 adds compatible account/session metadata through the [account contract](account-contract.md); schema 6 adds disabled backup storage through the [backup storage contract](backup-storage-contract.md). Upgrade each incremental version transactionally and reject versions above the [current runtime head](runtime-contract.md). Preserve all prior data.
- `collection_tasks.request_id` and `fingerprint` are unique. Candidate uniqueness is `(source_id,identity)`. Changes use optimistic `revision`.

## 3. Contracts

### Browser ↔ application

Authoritative wire fields: `.trellis/tasks/11-19-resource-collection-plan/api-contract.md`; user-facing deployment: `docs/collection.md`.

- Authenticated base `/api/collection`; same-origin protection, strict 64KiB JSON. Arrays must be `[]`, never null. Existing error shape `{error:string}` remains.
- `GET /capabilities`; source list/create/full PATCH, source test and RSS refresh; subscription list/create/full PATCH, preview and run; candidate pagination/ignore/acquire; preview/create task; task pagination/detail/events/actions.
- Source types `rss|web|local|remote|external`. RSS URL is write-only and stored in private files. Empty URL on edit retains the existing value. Public external entry accepts HTTP(S) without credentials/query/fragment.
- `POST /previews`: `{sourceId,url,path,profile:'video'|'audio',mode:'register'|'copy'}`. Returned preview binds source revision and input snapshot with expiry. Paths are root-relative, never arbitrary command arguments.
- `POST /tasks`: `{requestId,previewId,contentType,createLibrary,libraryId}`. Persist the same UUID across uncertain retries. Identical accepted input under the original UUID returns the original task; conflicting intent or a duplicate fingerprint under a new UUID returns409. Never accept an unpersisted request alias.
- Candidate acquire carries current candidate revision and target authorization; original qB flow still owns actual transmission and selection.
- Task actions `cancel|retry|retry_import` carry `revision`. `retry_import` reuses published data; BT control redirects to existing download workflow.

### Application ↔ worker

- Protocol version1 lives in shared Go structs. Only `/healthz` is unauthenticated; `/v1/*` requires exact `Bearer <token>`. No arbitrary executable, arguments, environment, output path or remote connection string is accepted.
- `GET /v1/capabilities` returns actual tool availability, persistent instance ID and public authorized source IDs. Missing optional tools/remote config disables that capability; it must not prevent basic media use.
- `POST /v1/previews` returns a bounded expiring snapshot. `POST /v1/operations` binds UUID and preview to immutable input hash. Reusing an ID for different intent is409.
- Operation routes: GET/list, POST cancel/publish/retry. States: `queued|running|acquired|publishing|published|failed|canceled|recovery_required`.
- `acquired` is still private staging. Only an explicit app publish authorization moves to `complete/<UUID>`. Store the publishing intent before atomic same-filesystem rename; recover an interrupted rename by verifying manifest and actual path.
- `Operation.Files[].path` is relative to `relativeDir`, never an absolute path; `size/modTime` are version facts, SHA256 is optional. Application independently validates mapped files and only opens selected supported media.
- GET/DELETE `/v1/probe` challenges the configured shared root with a random nonce; do not assume identical path strings mean identical storage.

### Configuration and boundaries

- App keys: `NAS_COLLECTION_WORKER_URL`, `NAS_COLLECTION_WORKER_TOKEN_FILE`, `NAS_COLLECTION_READ_ROOT` are an all-or-none group; paths absolute. HTTPS by default; `NAS_COLLECTION_WORKER_ALLOW_HTTP=true` explicitly permits only restricted private/loopback/Compose endpoints. `NAS_COLLECTION_MAX_BYTES=21474836480`, `NAS_COLLECTION_MAX_FILES=1000` bound each plan.
- Worker keys and exact defaults are documented in `docs/collection.md` and `collectionworker.Config` Chinese comments. Data root is exclusive, locked, separate from input/secret roots. Token is generated only if missing, never overwritten. POSIX uses private modes; Windows must apply its actual supported permission model.
- Local register uses already authorized media roots, defaults to read-only. Local copy uses worker-configured root IDs. Remote uses configured named rclone remotes and fixed copy only. Inputs retain source names/tags/content.
- Web output is a single finite item. Fixed native yt-dlp downloader uses a controlled proxy enforcing DNS/connect destinations for requests, redirects and subresources. Block private/reserved/metadata destinations; keep postprocessing offline. No recursive channel/playlist, cookies upload or DRM.
- Worker owns fixed tool process trees, cancellation and final handle release. Do not let a normal parent exit leave descendants holding files. Every background loop and admitted request belongs to service shutdown.
- `private-state`, `staging`, `probes`, unknown completion paths, unpublished and version-changed files remain denied through all read paths. Gate remains when worker is down or removed from current config; persisted roots retain protection.
- Worker and app state are separate durable facts. On uncertain submit, query the original operation ID before any retry. Restarted running worker operations require reconciliation; never invent successful results.
- Import requires actual expected item coverage, valid target type/path and published size/mtime versions, including the scanner's directory-comic aggregate. A completed unrelated scan is insufficient. Source, acquisition, publication and import are independent states. Library deletion clears associated collection item IDs within the same transaction; published or bound-BT tasks become import-failed, unpublished/unbound tasks remain retryable acquisition failures. Retain files and never recreate the deleted target from historical create-library intent.
- Runnable acquisition excludes historical failures. Published-file and bound-BT verification uses a rowid cursor with at most256 records per round and wraps after exhaustion. Completed BT tasks still mirror the original download's authoritative state; clear old item IDs before validating publication and exact coverage. An archived engine task with valid retained media stays imported, while a missing target, changed file or missing download record invalidates it.
- RSS initial fetch creates a baseline; historical backfill is explicit. Persist entry identity, use ETag/Last-Modified and bounded XML/network budgets. An RSS edit clears conditional-request cursors and the next fetch refreshes unbound candidates' private snapshots/revisions; accepted tasks keep immutable prior input. Changing the URL establishes a new baseline. Default rules review; automatic rules require explicit intent and unambiguous episode and file matches, otherwise review. Recheck source/subscription enabled and revision under the acquisition lock before dispatch.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| No session / wrong worker token | 401; no source or operation exposure |
| Unknown field, traversal, malformed profile/URL | 400; no task execution |
| Stale revision, expired/changed preview, conflicting ID | 409; preserve original operation |
| Missing worker/tool/shared-root mismatch | Explicit unavailable capability or safe error; do not fake empty success |
| Source disabled | No new acquisition/scheduled fetch |
| Private webpage/redirect/subrequest | Denied at connection policy, no bypass through downloader choice |
| Source file/link/version changed | Reject preview execution or publication; preserve source |
| Low disk / oversized / excessive files | Stop safely before publication; source unchanged |
| Worker result unknown after restart | Reconcile original ID; retain pending/recovery status |
| Scan failed/no actual coverage | Import failure, published data retained; independent retry |

## 5. Good / Base / Bad Cases

- Good: preview an authorized music directory → register once → scan exact expected files → expose stable item IDs; retry preserves those IDs.
- Base: worker absent → RSS and authorized local registration still function, web/remote explain unavailable capability.
- Bad: torrent100% or worker`acquired` → label imported; scanning a sibling library → claim success; changing root on retry → duplicate or expose files.

## 6. Tests Required

- Meaningful temporary unit tests: migration preservation, revision/idempotency conflicts, path/link boundaries, private network policy, publication/recovery, cancel/process ownership; remove only new temporary tests after execution.
- `scripts/verify_collection.py`: real app+worker+files, RSS baseline/conditional/dedup, controlled qB bridge and manual selection, local register/copy, exact imported file Range, secret separation, current schema integrity with preserved v3 collection data, both services restart, worker outage preserving already published media. `scripts/verify_external.py --workflow` separately verifies a genuine v3 database through the external/account/backup increments to the current runtime head and external business semantics. A later schema slice records its actual executed suite selection separately.
- Optional real CLI modes: named local/alias remote with real rclone; public small video/audio with real yt-dlp+FFmpeg. Clearly separate fixture/CLI evidence from real cloud/BT/NAS acceptance.
- Desktop normalized v1/v2 Compose with no daemon: reject extra ports/mounts/privileges/services/env changes/wrong volumes/read-write escalation/missing dependency. Release manifest rejects missing/duplicate/mutable collection image; v2 package requires overlay.
- Frontend: typecheck/build; desktop and mobile, both themes, keyboard dialogs, independent query errors, batch partial failure and uncertain-submit key preservation. Run existing search/download/media regression harnesses.

## 7. Wrong vs Correct

Wrong: `state='downloaded'; importState='imported'` immediately after successful copy.

Correct: persist intent → execute under original UUID → `acquired` → independently validate path/versions → explicit publish → gate-approved manifest → enqueue/reuse scan → verify every expected item → `imported` with actual item IDs.
