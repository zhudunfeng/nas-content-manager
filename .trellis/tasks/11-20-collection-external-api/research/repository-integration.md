# Research: Repository integration points and a simple external-service UX

- Query: Extend the existing MoviePilot, AutoBangumi, Sonarr and Radarr external entries with verified API controls, preserving independent finished-directory import and keeping the integrated NAS simple to use.
- Scope: internal. Official third-party protocols are assigned to separate research topics and are not asserted here.
- Date: 2026-11-20 (Asia/Shanghai).

## Findings

The smallest safe extension is an optional API connection attached to an existing `external` collection source, with fixed typed adapters and a separate durable action journal. Keep all four services as owners of their orchestration. Keep the current read-only preview, registration, validation and scan pipeline for local finished files. Do not represent an upstream subscription or queue record as a managed qB download or as an imported local media item.

The user's latest requirement, “当前系统整合后必须遵循简单易用的原则”, should be an acceptance requirement. Initial connection must take three clear steps: choose the service, enter its address and credentials, and test the connection. Local completion-directory binding must be optional during connection. Daily actions should be presented through service subscriptions and tasks, with versions and directory mapping in details/settings.

Two existing assumptions need deliberate changes:

1. `external` source creation currently requires `rootId`; both frontend and backend reject a missing root before the API can be configured. An API-connected service must be allowed to exist without an import directory. Until a valid root is bound, disable only finished-file import and offer “设置完成目录”.
2. Existing “测试连接” for `external` checks the local directory, not the external application. API connection health and import-directory readiness must become distinct results, each with an accurate action label.

## Files found and code patterns

| File and exact location | Existing responsibility and reuse guidance |
| --- | --- |
| `internal/app/collection_models.go:12-46` | Public `CollectionSource` and source input. Source type, revision, public external entry and directory binding already exist. Add small optional service metadata; never add a readable secret endpoint/token/password to the public DTO. |
| `internal/app/collection_models.go:153-160` | Existing collection manager owns short business locks and wakeups. Its `mu` is explicitly not for network calls. |
| `internal/app/collection_models.go:201-210` | Schema v3 contains six collection tables; no durable external API action table exists. |
| `internal/app/store.go:32-45,70-109` | SQLite has one open connection, WAL/foreign keys, ordered additive migrations, and rejects schema versions above 3. Add a separate v4 transaction and raise the supported version; preserve v1/v2/v3 migration steps and all prior rows. |
| `internal/app/collection_store.go:18-36` | Private collection inputs and immutable task plans are separate from public task records. An external connection needs the same separation, with immutable credential references for accepted actions. |
| `internal/app/collection_store.go:58-119` | UUID-only paths under `collection-secrets`, atomic temporary-file writes with permissions, `Sync`, rename, regular-file and size checks. Reuse this storage boundary rather than SQLite credential columns or browser local storage. |
| `internal/app/collection_store.go:122-152` | Public source bodies are read straight from SQLite and returned as source DTOs. Anything added to that body is potentially public and present in ordinary backups. |
| `internal/app/collection_store.go:210-229` | Task optimistic revisions and bounded Chinese stage events. Use the conditional-update pattern for external action state, but keep external records distinct from import tasks. |
| `internal/app/collection_api.go:17-41` | Existing `/api/collection` routes register through the session wrapper. Extend here with a fixed route set. |
| `internal/app/collection_api.go:90-200` | Source validation, source-kind immutability, 64-source limit, short write lock, revision check, write-only RSS input and public response. Source updates construct the complete replacement object. |
| `internal/app/collection_api.go:114-120` | Public application entry is HTTP(S), with no embedded user, query or fragment. Preserve this entry separately from the private API base address. |
| `internal/app/collection_api.go:127-129` | Current local/remote/external sources require `rootId`. Relax only for the new optional external API connection flow; preserve legacy link-plus-directory behavior. |
| `internal/app/collection_api.go:164-184` | Source-ID secret file is mutable in the RSS flow. Do not copy that approach for accepted external actions: source edits must not redirect an uncertain prior action to a new endpoint/account. |
| `internal/app/collection_api.go:203-248` | Test endpoint currently checks RSS network, external/local directory, or worker ability. External API testing needs an explicit branch/result instead of calling a directory check “API connected”. |
| `internal/app/collection_preview.go:18-39` | Maps `media-N` to an authorized local root, resolves a safe relative child and checks canonical containment. API-supplied remote absolute paths must never bypass this helper. |
| `internal/app/collection_preview.go:42-99` | Read-only preview bounds traversal, rejects links and unsupported files, and applies the publication gate. |
| `internal/app/collection_preview.go:170-180,253-257` | External final directories already become `register` previews, with size/mtime file facts, source revision and private input. No new importer is required. |
| `internal/app/collection_preview.go:265-356` | Task acceptance binds the original request UUID, fingerprint, source revision, target choice and current file versions before durable insertion. Follow its immutable-intent principle for external actions. |
| `internal/app/collection_runtime.go:14-51` | Existing local/worker coordinator is lifecycle-owned, with one loop. Avoid introducing slow remote API polling into the path that advances local imports. |
| `internal/app/collection_runtime.go:70-161` | Durable worker intent, original operation ID lookup before submission, and ownership/hash checks. Its worker supports its own stable IDs; external services cannot be assumed to provide that feature. |
| `internal/app/collection_runtime.go:109-126` | Read-only registration independently revalidates files, marks them ready, and starts import. |
| `internal/app/collection_import.go:142-159,163-240` | Imports require actual indexed item coverage and size/mtime validation; scan completion alone is insufficient. Keep this authoritative. |
| `internal/app/collection_rss.go:26-87` | URL validation, real-DNS/IP checks per dial, explicit private access, no environment proxy and same-origin redirect boundary. Reuse this hardened network policy for fixed service clients. |
| `internal/app/collection_rss.go:90-127` | Context-bound request, separate timeout, sanitized errors and a 2 MiB read cap. Extend with protocol-specific strict decoding and bounded pages. |
| `internal/app/collection_worker.go:19-68` | Existing worker RPC is bounded and typed but authenticates with the worker token. Do not route arbitrary external APIs through this worker or reuse its token. |
| `internal/app/collection_config.go:12-54` | Worker HTTP allowance is a narrow worker setting. It must not silently authorize every new external service endpoint. |
| `internal/app/download_models.go:90-98,160-166` | Existing durable `queued/sent/uncertain/completed/failed/superseded` action pattern and unique request ID are useful precedents. Add a separate external journal, not rows in `download_operations`. |
| `internal/app/downloads.go:513-529,586-608` | Duplicate request binding and short transaction committing intent before network. New external duplicates must compare the full typed action intent, including parameters, not merely target and action name. |
| `internal/app/download_sync.go:228-243,301-320` | Recovery reads bounded pending actions, persists `sent` before mutation, and never automatically resends an uncertain add. This is the right conservative precedent. |
| `internal/app/server.go:110-121,140-152` | Existing app context and wait group own background work and shutdown. Reuse them for a bounded external-action coordinator. |
| `internal/app/server.go:267-305,329-352` | Same-origin mutations, 64 KiB JSON budget, content-type checks, unknown-field rejection and single JSON object. No need for a new authentication layer or larger generic request limit. |
| `internal/app/media.go:75-121` | `/api/backup` is a consistent SQLite snapshot. Secrets must remain outside every normal table/body/payload so this backup stays credential-free. |
| `web/src/types.ts:319-359,427-447` | Collection capability/source/input/task contracts. Keep external service records separate from local import-task DTOs. |
| `web/src/components/CollectionSourceDialog.vue:17-65,84-95` | Existing write-only form, cancellation, typed input and external root/link controls. Extend this form progressively rather than adding another top-level settings application. |
| `web/src/components/CollectionSourceDialog.vue:48,89-91` | Current external creation is blocked on directory input. The new three-step connection must not inherit that blocker. |
| `web/src/components/CollectionCenter.vue:16-38,79` | Five collection sections and one-modal guard. Do not add a sixth global mobile navigation item or nested native dialogs. |
| `web/src/components/CollectionCenter.vue:94-136` | One in-flight poll, separate query errors, stale cancellation, higher task revisions and hidden-page pause. Extend these ownership rules to the external service view. |
| `web/src/components/CollectionCenter.vue:171-183` | Current test invocation and full-object source enable/disable PATCH. Every newly added public field must survive source toggles; an omitted field can otherwise clear the connection metadata. |
| `web/src/components/CollectionCenter.vue:213-227` | Unmount current modal and await `nextTick` before task/media/download navigation. Use the same transition for service settings and finished-directory import. |
| `web/src/components/CollectionCenter.vue:256-274` | Current source cards, RSS-only subscriptions and local collection tasks are natural extension points. Avoid appending many service controls to every source card. |
| `web/src/components/CollectionPreviewDialog.vue:39-67,96-128` | Existing external finished-file confirmation and immutable original submission body. Exclude API-only unbound sources from valid directory choices and preserve accepted request UUIDs. |
| `web/src/components/DialogShell.vue:22-38` | Native dialog lifecycle, body scroll lock and focus restoration. Reuse unchanged. |
| `web/src/api.ts:13-27,41-48` | Same-origin API, session-expiry handling and UUID generation that works on HTTP LANs. Reuse for every external action. |
| `web/src/collection.ts:11-19,31-48` | Local acquisition/import labels and validated public external link. Do not reuse local “completed/imported” labels for upstream queue completion. |

CodeGraph CLI queries for `collectionSourceInput`, `collectionPrivate`, `collectionRSSClient` and `downloadAction` identified the existing definitions; references above were verified against the actual current source, because its uncommitted implementation is authoritative.

## Recommended minimal architecture

### Source, connection and adapter boundaries

- Preserve `kind: external`. Add a fixed optional `externalService` discriminator with `moviepilot|autobangumi|sonarr|radarr`; absent service means the existing entry-and-directory-only mode. Treat service identity as immutable after connection, or require explicit disconnect/recreate; do not reinterpret queued actions after a switch.
- Keep `externalUrl` as the existing separately consented browser entry. A private API endpoint is write-only and must not be copied into public DTOs, generated links, log fields or browser diagnostics.
- Keep current source revision as configuration concurrency control. Store an internal current connection reference outside the public source DTO. A small `collection_external_connections` table can hold `source_id`, `source_revision`, `private_ref` and non-secret verified capability/version metadata; the secret file contains the typed base URL, authentication material and any derived session.
- On secret edits, write a new server-UUID secret file, then transactionally switch the current connection reference and source revision. Do not overwrite the old file while durable actions reference it. A failed DB commit may leave an orphan private file; it must not change the effective connection. Never delete action-referenced secrets during configuration updates.
- Empty password/token means retain the existing secret, while explicit typed disconnect is separate. Never use a string of asterisks as a stored replacement. Credential clearing is a deliberate configuration operation, not an accidental effect of editing a name or toggling source enablement.
- Add one small adapter interface for health/capability negotiation, bounded subscription/resource lists, queue/history and fixed typed actions. Each service owns URL construction, authentication, response mapping and reconciliation evidence. Do not expose HTTP methods, arbitrary endpoints, generic bodies or upstream cookies through public APIs.
- Capabilities must be per verified service/version and distinguish read access from write access. Authentication success alone does not prove subscription mutation support. Unknown/unsupported versions remain readable only where verified, with unsupported operations unavailable and explained.
- Never add foreign queue items to `downloads`, invoke qB mutations for them, or run internal automatic RSS rules for an external subscription. Existing qB ownership remains unchanged.

### Fixed route and DTO proposal

The exact names may follow neighboring handlers; the important contract is the fixed surface:

- A source-scoped connection save/test endpoint accepts only the four service discriminators, write-only typed configuration, explicit private-network consent and source revision. A draft test may authenticate/read health but must not save subscriptions or start downloads.
- A source-scoped overview endpoint returns service identity, safe version/status, `checkedAt`, stale state and capability booleans, with independent errors for connection, subscriptions, queue/history and local completion-directory readiness.
- Bounded lists return normalized service subscription/resource, queue and history DTOs. Keep opaque upstream IDs, title, upstream status, observed time and supported actions. Return empty arrays, not null; preserve unknown progress/size as unknown. Do not return raw upstream objects, download URLs, tracker data or response bodies.
- Typed action input binds `requestId`, `sourceRevision`, action kind, opaque remote target identity, desired final value where appropriate and a server-generated resource snapshot/version reference. New subscriptions use server-validated lookup results and bounded service-provided option IDs, never arbitrary JSON.
- A local action detail/list endpoint allows recovery after closing the modal, network interruption or process restart. The UI must distinguish action acceptance from confirmed remote effect.
- Remote root/quality-profile choices use bounded server-issued identifiers and safe labels. A remote root path is not a local authorized path. Never trust an upstream path as an input to local scanning.

### Native HTTP policy

- Reuse actual-dial IP validation from `collectionRSSClient` and `collectionNetworkIP`, with one normalized authorized origin/base-path and fixed appended endpoint paths. Reject base URL userinfo, query/fragment, control characters, traversal, unsupported scheme and invalid port before saving.
- For API credentials, prefer rejecting redirects entirely, especially for mutations and login, rather than relying on redirect method rewriting. If a verified service needs a redirect, explicitly preserve the exact configured origin and prefix; never forward credentials across origins or downgrade HTTPS.
- `allowPrivate` is per-source explicit consent. Loopback/private DNS targets still need consent; metadata/link-local/reserved destinations remain forbidden. If HTTP is allowed for NAS services, require an explicitly allowed private destination at actual connect time. Public endpoints carrying credentials require HTTPS. Do not inherit the worker's broad setting implicitly or disable TLS verification.
- Apply bounded total time, header size, response bytes, page count, row count, retries and simultaneous requests. The existing 12-second/2-MiB RSS budgets provide a conservative starting point, not proof that each third-party API fits them.
- Decode only known public fields. Map upstream errors to fixed Chinese messages such as “凭据无效” or “服务暂时不可达”; do not include raw `url.Error`, upstream body, cookie, API key or private address. Logging records source/action IDs and Chinese stage descriptions only.
- Respect `App.ctx`, `wg`, shutdown cancellation and a small semaphore. A bounded external-action loop should not block the existing local collection/import loop; no new worker RPC protocol, plugin runtime or deployment helper is needed.

## Schema v4 action journal and uncertain writes

Add a v3-to-v4 migration instead of modifying/recreating the existing v3 tables. A proposed `collection_external_actions` journal should contain:

| Field | Purpose |
| --- | --- |
| `id`, unique `request_id` | Server action UUID and original client UUID; permanent retry identity. |
| `source_id`, `source_revision`, `service`, `connection_ref` | Immutable accepted source/service/credential-snapshot binding. A reference is allowed in SQLite; private endpoint and credential content are not. |
| `kind`, opaque `target_id`, `intent_hash`, bounded typed `payload` | Binds exact intended effect, such as desired monitored state; prevents a UUID from being reused for different parameters or targets. Payload must be secret-free. |
| `status`, `revision`, `remote_operation_id` | Durable lifecycle and optional verified upstream command identifier. |
| `reconciliation_identity`, sanitized `result`, `message`, timestamps | Evidence needed to distinguish accepted, confirmed, failed or uncertain work without raw upstream responses. |

Use a short transaction to validate configuration/resource snapshot, insert the immutable action and commit before any mutation. Idempotent retries must look up the original request first and compare the complete immutable intent. A matching retry returns the same action even after the current source revision has advanced; a different intent returns 409. New UUIDs are not a retry mechanism.

Recommended states are `queued -> sent -> succeeded|failed|uncertain`, plus `superseded` only for an action that provably has not been sent. Persist `sent` before the network call. A crash after that transition must be treated as uncertain even if no response was recorded. Do not mark `sent` back to `queued` on restart.

Reconciliation depends on the action and the actual adapter capability:

1. **Create subscription/resource:** query the original immutable external identity and exact creation intent. Bind an unambiguous match; conflict on ambiguity. If absence cannot be proved and upstream offers no reliable idempotency key, keep “结果待确认” and expose refresh/open external application. Do not automatically POST again after timeout.
2. **Set desired monitoring/enabled state:** read the exact target after timeout; success only when the desired value is observed. Do not implement a relative “toggle” that flips again on retry. Before a first write, reject a stale remote snapshot rather than overwrite an external user's concurrent edit.
3. **Explicit search command:** bind the returned remote command ID when the protocol supplies one. If the POST response is lost and no reliable command identity is queryable, retain uncertainty; do not repeat the search or broaden it to a whole library.
4. **Configuration changes/disable:** new requests use the new revision. An unsent action from an obsolete configuration can stop for reconfirmation; already sent/uncertain actions reconcile against their original snapshot, never a new endpoint/account. Their late results must not overwrite current source health/configuration. Configuration disable must not delete files or silently cancel unrelated upstream orchestration.

Never infer that “HTTP 2xx” equals completed external download or local import. Do not place destructive media deletion, library removal, downloader takeover or bulk global search in the action enum.

## Three-step UX and progressive disclosure

### First connection

1. **选择服务:** four named services and a preserved “仅入口与目录” option. Default the display name from the selection. Briefly explain that the selected application continues to handle acquisition.
2. **填写连接:** only the service-required base address and credential fields. Show “允许访问我的 NAS 私网服务” only when relevant and require active consent. State that credentials will not be displayed again. Put a separate browser entry and advanced base-path details in a collapsed “高级设置”.
3. **检测并连接:** show the real result, safe version and available capabilities. Button states distinguish connecting, authentication failure, unsupported version and network failure. Successful connection offers “查看订阅” and a secondary “设置完成目录”; absence of a local mount does not invalidate a working API connection.

Saving a source is not connection success. Editing without a new credential retains it. Changing a connection field clears an old test result so a late response cannot validate a different input. Testing must not create subscriptions, enable monitoring or launch a search.

### Daily usage

- Preserve the current five collection sections. Source cards need only the service name/status and one primary “管理” action, plus existing entry/import access as secondary controls. Avoid four new sets of controls on the overview.
- Within service management, prioritize two plain views: “订阅” and “任务”. History and connection details are secondary. Label records “由 Sonarr 管理” (or actual service) so ownership is always understandable.
- Keep internally managed RSS rules visibly separate from external subscriptions. A service selector/section label is enough; do not combine different mutation semantics into the RSS rule form.
- A subscription row shows the title, actual monitored/enabled state and only supported actions. Choose explicit verbs such as “开启监控” and “搜索此订阅”; explain the effect in a compact inline confirmation. Do not offer unknown actions and then fail with fake success.
- An external task shows upstream progress/status and the last checked time. Local import status is a separate line only after a real local registration task is created. A stale snapshot is marked “上次状态”; a failed query is not a successful empty list.
- “导入成品” opens the existing read-only preview with the configured source. If no local completion directory is bound, show one action to bind it. Advanced mapping displays the external path and chosen local root distinctly; no automatic import of arbitrary upstream paths.
- Preserve one native modal, focus restoration, Escape behavior and mobile five-entry bottom navigation. Close/unmount and await `nextTick` before opening the existing import/reader flow.
- Poll only the active service view with one in-flight request, bounded backend caching and hidden-page pause. Do not multiply the center's existing six-second poll by all four services and all their endpoints.
- For an uncertain action, the primary recovery action is “刷新确认”, using the original action/request ID. “进入外部应用” is a useful secondary action. Do not label a potentially duplicating POST as a harmless “重试”.

## Minimal file change list

Existing files that need narrow extensions:

- `internal/app/store.go`: additive schema v4 and future-version boundary.
- `internal/app/collection_models.go`: optional public external service metadata, source input and manager lifecycle fields if needed.
- `internal/app/collection_store.go`: reuse private-file path/I/O; introduce typed external immutable storage helpers without mixing API secrets into public source records.
- `internal/app/collection_api.go`: fixed route registration, optional external directory validation, complete source-field preservation and correctly named test behavior.
- `internal/app/collection_rss.go`: only if extracting the existing hardened origin/IP transport into a shared internal helper is actually smaller than keeping an adapter-owned wrapper. Preserve RSS semantics and regression coverage.
- `internal/app/server.go`: only for lifecycle start/close of a distinct bounded external-action loop if not encapsulated by the existing collection manager.
- `web/src/types.ts`, `web/src/collection.ts`: strict service/capability/action DTOs and human-facing labels.
- `web/src/components/CollectionSourceDialog.vue`, `CollectionCenter.vue`, `CollectionPreviewDialog.vue`: progressive connection input, management entry, complete source toggle preservation, API-only-source import gating and existing modal transitions.
- `web/src/components/collection.css`: small responsive styles for the new form/view, using existing spacing/theme conventions.

Suggested new focused files, whose exact split can follow complexity:

- `internal/app/collection_external_models.go`: fixed models and schema v4 definitions.
- `internal/app/collection_external_store.go`: immutable connection references and action journal operations.
- `internal/app/collection_external_api.go`: authenticated fixed app endpoints and validation.
- `internal/app/collection_external_client.go`: bounded authenticated client/adapter interface.
- `internal/app/collection_external_arr.go`, `collection_external_moviepilot.go`, `collection_external_autobangumi.go`: service-specific protocol mapping after official research; Sonarr/Radarr may share transport/model helpers only where their verified schemas agree.
- `internal/app/collection_external_runtime.go`: durable action dispatch and conservative reconciliation.
- `web/src/components/CollectionExternalDialog.vue`: active-service subscriptions/tasks UI using the existing `DialogShell`; configuration remains in the source dialog rather than a parallel settings system.
- `scripts/verify_collection_external.py`: reusable isolated HTTP protocol fixtures plus app round-trip/restart checks; extend the existing browser harness for actual UI handoff.

Existing regression files with hardcoded current-schema assertions must also be updated as part of schema v4, without weakening their data-preservation checks: `scripts/verify.py:279`, `scripts/verify_collection.py:229`, `scripts/verify_engine_bootstrap.py:179`.

Do not edit `internal/collectionworker`, `internal/collectioncontract`, engine-helper provisioning, Docker templates, original qB transport or readers merely to implement API controls. The new services need configuration, not another deployment/execution protocol.

## Cross-layer boundaries and regression assertions

| Boundary | Required assertion |
| --- | --- |
| Public source/input -> private connection | Credential and private endpoint absent from source/list/action responses, SQLite rows/backups, logs, URLs and browser console. Empty edit retains the credential; enable/disable does not clear service metadata or connection. |
| File -> DB activation | Write new private UUID snapshot before DB switch; simulate failed activation and show previous connection still effective. An accepted action retains its original connection reference after edit/restart. |
| v1/v2/v3 -> v4 | Fresh install and each upgrade preserve accounts, sessions, libraries, five media types, personal state, downloads, collection tasks and file associations; future schema refuses startup. `integrity_check` remains clean. |
| Browser -> action acceptance | Strict typed input, session401, cross-site403, unknown fields400, oversized413, stale source/resource409 and exact original UUID retry. Same UUID/different payload409. |
| Action journal -> upstream | No POST before durable commit/`sent`; response loss and process restart do not duplicate add/search. A late response cannot overwrite a later source/target version. Read-only absence is not assumed proof that an uncertain create never happened. |
| HTTP -> adapters | Real HTTP fixtures match official auth headers/login bodies/cookies and response envelopes, expired credentials, safe version detection, capability gating, malformed/large/partial responses and page budgets. A fixture must not merely validate an invented protocol. |
| DNS -> socket | Private service without consent fails; permitted private host succeeds; metadata/link-local/reserved, rebinding to a forbidden IP, cross-origin redirect and public credential-bearing HTTP fail. No proxy-env bypass or TLS-skip fallback. |
| Adapter -> UI | Empty/error/stale/partial states remain distinct. One list succeeding cannot erase a different list failure. Unsupported action stays unavailable with a reason; connection success does not mean directory ready. |
| Upstream task -> local import | Foreign queue records never become managed qB jobs. API completion alone creates no local item and exposes no media URL. Existing directory preview requires explicit final-file confirmation and exact size/mtime/indexed coverage. |
| Browser lifecycle | One active modal, focus restored, no hidden-page polling, one in-flight request, stale input/test/list results discarded, original action ID survives uncertain retry/reopen. |
| Ease of use | New API source connects without a completion root; user can reach subscriptions after three clear steps. Versions/mapping are secondary, service ownership is visible, and setup success offers one clear next action. |
| Existing behavior | RSS baseline/dedup and original BT file selection, webpage batch immutable retries, local/remote copy, original external entry and external read-only import still pass. |

Validation should use the project wrapper for Go vet/test/build, Vue typecheck/build and isolated integration/browser commands. Execute and remove only newly generated temporary unit-test files per the user's project instructions; retain reusable integration scripts. Inspect 375/768/1024/1440 widths, both themes, actual dialog screenshots/padding, keyboard focus and five mobile navigation entries.

Existing reusable evidence anchors: `scripts/verify_collection.py:184-198` checks request reuse, new-UUID duplicate conflict, real indexed import and Range read; `scripts/verify_collection.py:224-245` checks original-file preservation, secret absence and restart/offline behavior; `scripts/verify_collection_browser.py:108-134` checks lost accepted response and the one-modal import/media handoff; `scripts/verify_collection_browser.py:160-175` checks widths/themes/navigation. These are reusable patterns, not proof that this new API feature is already tested.

## Related specs

- `.trellis/workflow.md`: current task, persisted research, implementation then quality review and spec update.
- `.trellis/spec/backend/collection-contract.md`: source types, revision-bound previews, private inputs, publication/import separation, independent external ownership and fixed worker protocol.
- `.trellis/spec/backend/runtime-contract.md`: additive schema, bounded JSON/errors, lifecycle, consistent backup and preservation of existing user/media data.
- `.trellis/spec/backend/download-contract.md`: durable side effects, original task ownership, exact selected publication and no external adoption.
- `.trellis/spec/backend/torrent-search-contract.md`: source secret/error hygiene and bounded service responses; new API work must not broaden Torznab semantics.
- `.trellis/spec/frontend/app-contract.md`: native modal transitions, independent errors, single polling owner, real server item IDs, Chinese comments/types, responsive checks and five mobile primary entries.
- `.trellis/spec/guides/cross-layer-thinking-guide.md`: observed service round trips, sanitized protocol evidence and revision/transaction verification.
- `.trellis/spec/guides/code-reuse-thinking-guide.md`: search existing helpers before introducing another URL validator, transport, request ID or importer.
- `docs/api.md`: fixed collection routes and public DTO contract; update when actual implementation is confirmed.
- `docs/collection.md`, `docs/windows.md` and schema-related backend specs currently describe v3; update the current-version wording after implementation while preserving historical verification records as dated evidence.

## External references

No new external API facts were researched in this repository-only topic. Use the separate MoviePilot/AutoBangumi and Sonarr/Radarr official-source reports for concrete versions, endpoint names, authentication, action support and reconciliation limits. This file's route/model names and schema v4 design are recommendations, not an already implemented or third-party-guaranteed protocol.

## Caveats / Not Found

- Current production source contains only external public links and directory registration; no existing four-service API adapter or external action journal was found in the inspected code.
- The workspace includes the parent collection implementation as uncommitted changes. All line references describe that current filesystem snapshot and may shift during implementation. No git command, production write, live third-party connection or application deployment was performed for this research.
- Schema4 here is proposed. Do not claim migration, security, protocol support or browser acceptance has passed until the implementation and relevant checks run.
- Old source-secret overwrites are sufficient for the current RSS use but not a safe accepted-action credential binding; external journal recovery needs immutable private snapshots.
- A reliable exactly-once external mutation cannot be invented when an upstream API lacks idempotency or queryable command identity. The product must honestly preserve uncertainty and provide verification/open-service recovery.
- Real NAS mount topology, real installed versions and real external account permissions remain unverified. Isolated protocol tests can prove local handling and match official contracts, but cannot prove deployment-specific readiness.
