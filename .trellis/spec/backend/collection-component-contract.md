# Collection component deployment contract

## 1. Scope / Trigger

Applies to the four fixed MoviePilot/AutoBangumi/Sonarr/Radarr Docker templates, `internal/enginehelper/collection_*.go`, `internal/enginecontract/collection.go`, app `collection_components.go`, embedded deployment UI, Compose overlays and desktop schema-v3 image manifests. Reuse the private helper, durable task store, executor, image verification and existing authenticated source APIs. Do not deploy on GET, create another download coordinator, or automatically configure real external accounts.

## 2. Signatures

App and helper expose only:

```text
GET  /api/collection/components
POST /api/collection/components/deploy
POST /api/collection/components/{component}/start
GET  /api/collection/components/tasks/{id}?afterSequence=N
POST /api/collection/components/tasks/{id}/retry
```

`enginecontract.CollectionComponents() []string` returns the stable four-product whitelist; `IsCollectionComponent(string) bool` validates it. Old `/api/engines` installation/start/task routes remain restricted to qB/Jackett. Collection task reads/retries accept exactly one collection component with action `deploy` or `start`; they reject engine and stop-all tasks.

The helper-only stop-all action covers all six products when component deployment is enabled. Its client waits up to 240 seconds; desktop gives that client 255 seconds. Stop failure must preserve the app/helper for diagnosis rather than claim a complete shutdown.

## 3. Contracts

### DTOs and requests

- `CollectionStatus = {helper,message,templateVersion,noticeVersion,noticeText,activeTaskId,components:CollectionComponent[]}`.
- `CollectionComponent = {id,name,version,presence,runtime,readiness,message,revision,canDeploy,canStart,setupPort,serviceUrl,latestTaskId}`.
- `CollectionDeployRequest = {requestId,component,templateVersion,noticeVersion,acceptNotice}`. Require `collection-2026-11-20.1`, `collection-storage-v1`, and explicit `acceptNotice:true`.
- Start/retry reuse `{requestId,revision}`. Replies reuse `TaskResponse`, actual pull-layer counts, bounded events and persistent UUID bindings. A failed task remains readable with HTTP 200. An accepted browser request outlives its HTTP connection.
- Active-task request aliases must compare the full persisted intent signature under `opMu`, including action, component, deployment template/notice versions or the start revision. Alias only a task that is still active while holding that lock. If it has completed, return to normal status/revision validation; never create new work from the alias-only recovery path. This also applies to the second active-task check inside `submit` after a concurrent enqueue.
- `presence` is `unknown|missing|present|conflict`; `runtime` is `unknown|stopped|starting|running`; `readiness` is `not_configured|starting|configuration_required|error`. Container readiness never changes source `apiState` to connected. A `serviceUrl` requires owned running resources and the fixed anonymous probe; API authentication remains the original source test.
- The helper status operation has a 22-second total budget. App private transport allows 25 seconds for headers and 30 seconds total, with redirects disabled. The browser must cover sequential status and task reads and still keep one in-flight operation; its timeout cannot be shorter than the helper's valid response budget.

### Images, storage and runtime

Fixed digest-pinned templates and their verified upstream evidence live in task `research/official-deployment.md`. MoviePilot 3.2.8 uses port 3000, AutoBangumi 4.0.0 uses 7892, Sonarr 4.0.20.3014 uses 8989, Radarr 7.0.3.10918 uses 7878. Host configuration ports are respectively 19130/19132/19133/19134 on `127.0.0.1` only. Never accept browser-supplied images, Docker commands, ports, environments or mounts.

`ENGINE_COLLECTION_ENABLED` is strict `true|false`, default false. Preserve the original six storage roles; add `collection-media` only when enabled. Bind mode derives it from `ENGINE_HOST_ROOT`; desktop volume mode derives it from the validated deployment prefix. The helper mounts the role read-write at `/managed/collection-media`; app mounts it read-only at `/collection-media` and authorizes only `/collection-media/library` through `NAS_MEDIA_ROOTS`. The library need not exist at app startup; prepare `library/<component>` only during explicit authorized deployment.

Each product receives distinct fixed local configuration/cache/database volumes. A durable private state nonce plus exact installation/component/role/template labels proves volume ownership. GET cannot create or adopt a volume. Persist authorization before Docker creation; fail before further effects if persistence fails. Reconcile a lost create response by inspecting the exact expected volume. Reject unknown same-name volumes and containers without changing them.

All products mount existing managed downloads at `/downloads` and the separate collection role at `/data`. Recommend copy organization into `/data/library/<component>`; separate mounts cannot support cross-volume hardlinks or atomic moves. Do not expose the original read-only media library for component writes. Finished-media import remains an independent authorized preview/register operation.

Bind-mode downloads retain the original qB ownership/access protocol, which does not use a collection marker. Do not retroactively require the new collection-media marker on a valid nonempty original downloads directory. The collection-media role is independent: safely claim an empty Compose-created directory before publishing its own marker; unknown nonempty unmarked collection media remains a conflict.

Runtime validation checks immutable image ID/reference, all mounts, private-volume ownership, fixed network, loopback publication, privilege/security settings and logging. No Docker socket, host network, extra capabilities, arbitrary devices or public Docker proxy. Necessary image initialization retains its prescribed user behavior; do not guess credentials or extract them from logs/config.

### Platform and user flow

Desktop manifest schema v3 requires app/helper and permits an optional valid worker image. Only v3 adds `compose.desktop.components.yaml`, after the worker overlay if present. The launcher derives the media-root interpolation value from its trusted service contract, never the caller's arbitrary environment. v1/v2 keep their original roles, mounts and environments. New image packages produce v3 and include the component overlay.

`CollectionComponentDeploy.vue` is embedded in the existing source dialog, with no additional native modal or nested form. Flow: select service → connect existing or deploy → explicit unchecked directory notice → actual task/layer progress → first configuration → prefill connection → enter credentials and test. MoviePilot's existing six-stage API workflow remains unchanged.

Retain the UUID and immutable intent after lost responses; restore known tasks on reopen. A definitive 404 may clear obsolete recovery data. Service changes abort superseded requests and reject late responses. Do not persist credentials or sensitive URLs in deployment recovery storage. Use `type=button` for deployment controls inside the source form. Publish a localhost browser entry only for a browser on the Docker host; remote NAS users receive the host port and SSH-forwarding explanation. Internal service URLs are platform addresses, not browser reachability claims.

When a deployment prefills a browser entry or completion root/subdirectory, track the actual suggested values. On service changes clear only fields still equal to those suggestions; preserve user edits. Reconnecting another component must suggest its own port and `library/<component>` path rather than retain MoviePilot's generated values for Sonarr.

## 4. Validation & Error Matrix

| Input/state | Required behavior |
| --- | --- |
| Missing session / foreign-origin mutation | Existing 401 / 403; no helper mutation |
| Unknown JSON fields, unsafe requestId/component/cursor/revision | 400; no enqueue or Docker effects; request IDs reuse the existing bounded safe identifier contract and the browser generates UUIDs |
| Oversized ordinary JSON | 413 |
| Notice unchecked or stale template/notice | 409; no deployment |
| Same accepted UUID and intent | Original task, including after a lost response/restart |
| Concurrent active-task alias with a changed signature | 409; no new binding or enqueue, even when action/component match |
| UUID reused for another intent or stale revision | 409 |
| Different active work | 429; preserve original work |
| Missing/pruned/non-collection task | 404; do not leak/retry another control plane |
| Disabled, old or unavailable helper | Explicit `not_enabled|unsupported|unavailable`; existing-source connection remains available |
| Owned stopped component | Explicit start allowed; no replacement install |
| Running container without anonymous HTTP readiness | Starting/diagnostic state; no usable service URL or API-connected claim |
| Foreign container/volume or altered template | Conflict; no takeover, deletion or permission relaxation |
| First configuration incomplete | Show component instructions, then require original authenticated API probe |

## 5. Good / Base / Bad Cases

- Good: explicitly deploy MoviePilot, reopen the page to recover the same task, complete its initialization UI, then enter the actual API Key and use the existing six-stage workflow.
- Base: native mode or a desktop v1/v2 bundle shows an actionable deployment-unavailable reason and still supports connecting an already installed service.
- Good: failed image pull retains actual layer evidence; retry continues the same durable task without overwriting unrelated resources.
- Bad: GET creates empty configuration volumes, a container label alone proves ownership, localhost points a remote browser at its own workstation, or anonymous HTTP 200 becomes authenticated source readiness.

## 6. Tests Required

- Helper fixture: fixed spec-to-inspect validation; foreign/changed image, ports, mounts, privilege and volume labels; lost volume-create response; durable nonce persistence failure; four probes; task idempotency/recovery/scope; full-signature concurrent aliases and completed-task alias recovery without enqueue; stop-all includes enabled collection products.
- Real app through production Unix helper transport: session/origin/strict-body boundaries, original UUID recovery, stale revisions, failed task/retry/start, disabled/down helper and task scope. Protocol fixtures must be identified as fixtures, never real Docker deployment evidence.
- Real Compose normalization: desktop v1, v2+worker, v3, v3+worker and Linux engine overlay. Assert exact role identity, app read-only/helper read-write mounts and media-root authorization; reject mutated normalized configurations and runtime mounts.
- Browser: actual source dialog deploy/confirmation/progress/recovery, service switching, first configuration and connect prefill; no nested form/modal; keyboard/focus; 375/768/1024/1440 in both themes with no horizontal overflow.
- Go vet/build, strict Vue typecheck/build, release self-tests and relevant original collection/download/search/engine regressions. Linux cross-compilation proves compilation only. Real image initialization, NAS ACLs and Docker Desktop runtime need deployment evidence separately.

Remove newly generated temporary unit-test source files after execution per AGENTS.md; retain reusable verification harnesses and sanitized reports.

## 7. Wrong vs Correct

Wrong: `GET /components` creates a volume, returns `readiness:"ready"` after inspecting a running container, and prechecks the deployment notice on reopen.

Correct: GET only inspects owned resources, bounded anonymous probes return `configuration_required`, the notice begins unchecked, and the user completes setup plus authenticated source testing before business controls become available.

Wrong: add the collection role to every desktop manifest and trust `NAS_DESKTOP_MEDIA_ROOTS` inherited from the shell.

Correct: add it only for schema v3, append the component overlay last and derive the interpolation environment from the validated launcher service contract.
