# Research: Repository impact of a plugin system and configurable markets

- Query: Which existing modules can support a plugin system, which contracts must remain intact, and what file/API/data boundaries should the modification plan use?
- Scope: Internal repository research; no implementation, build, installation or runtime changes.
- Date: 2026-11-19
- Task directory: `.trellis/tasks/11-19-plugin-market-fnos-plan/` (explicit assignment). At inspection time `.trellis/.current-task` still pointed to the unrelated desktop-icon review task; no existing task was changed by this researcher.

## Findings

### 1. Search method and current architecture

- Used the existing CodeGraph index through `python -B scripts/project_env.py -- D:\adunsoftware\nodejs\node.exe D:\adunsoftware\nodejs\node_modules\@colbymchenry\codegraph\npm-shim.js query <symbol> --json`. Queries found `RunWithOptions`, `LoadConfig`, `App::routes` and `App::backup`; verified their current source with bounded reads. No index rebuild or sync was performed.
- CodeGraph is useful for Go symbols; exact routes, configuration keys, Vue templates and Compose structures were inspected with `rg` and line-numbered reads.
- No `plugin`, `market`, `fpk`, `fnos` or `飞牛` implementation matches were found under `internal`, `web/src` and `docs`. The current engine manager is a fixed two-component deployment facility, not an existing general plugin loader.

### 2. App lifecycle, HTTP and persisted data

| File and current lines | Current behavior | Planning consequence |
| --- | --- | --- |
| `internal/app/server.go:29-47` | `App` owns separate download, torrent-search and engine runtime managers alongside SQLite, logger, context and wait group. | Add a narrow plugin-manager field and lifecycle integration, rather than placing market/network/install logic directly in the app type. |
| `internal/app/server.go:71-118` | Acquires instance/data ownership, retains a real listener, opens SQLite, creates a cancellable runtime, then starts managers/workers. | Initialize the plugin control plane after ownership/storage checks; startup must not automatically install market content. |
| `internal/app/server.go:137-153` | Closes admission, cancels runtime, closes connections, waits for handlers and workers before database/lease release. | Any plugin synchronizer/poller must join this lifecycle; plugin subprocesses or helpers need explicit ownership and shutdown policy. |
| `internal/app/server.go:237-260` | Standard-library `ServeMux`; feature-specific route registrars for downloads/search/engines; privileged endpoints use `a.auth`. | Add a scoped `pluginRoutes` registrar, retain session authentication and the `/api` namespace. |
| `internal/app/server.go:263-304` | Same-origin mutation checks, strict security headers and a default 64 KiB mutation body budget; only specific existing upload/selection routes receive larger budgets. | Market settings fit ordinary JSON. Local package upload, if introduced, needs its own explicit streaming limit and route exception; never enlarge the global limit. |
| `internal/app/server.go:308-352` | Session auth; JSON content type/unknown fields/trailing content rejected; Chinese error payloads. | New APIs should follow the same transport conventions and define typed DTOs. |
| `internal/app/server.go:461-464` | `GET /api/settings` reports read-only startup configuration. No writable generic settings API exists. | A UI to add market URLs requires new persistent settings and CRUD APIs; adding an environment variable alone cannot satisfy editable market addresses. |
| `internal/app/config.go:15-30,39-96` | Explicitly commented typed config, immutable source lookup, environment validation. | Add static plugin feature flags, data locations and trusted runner connection settings here; keep changing market-source records in persistent settings rather than mutating process environment. |
| `internal/app/store.go:16-96` | `library.db`, WAL, foreign keys, busy timeout, one connection; current schema version is 2 and newer versions are refused. | If public plugin metadata/market settings enter this DB, introduce an additive version-3 migration and revise the maximum supported version; do not rebuild existing media tables. |
| `internal/app/download_models.go:136-167` | Existing version-2 migration holds download tables and operation intent records. | Keep plugin migration separate; do not overload download rows or media jobs for privileged installation work. |
| `internal/app/media.go:76-121` | Backup uses `VACUUM INTO` for a consistent main-database snapshot. | Main DB records will be included automatically, but plugin payloads, app data, helper state and secrets will not. Backup UI/docs must state this precisely. |

### 3. Existing engine helper is a reusable pattern with a fixed trust boundary

| File and current lines | Current behavior | Planning consequence |
| --- | --- | --- |
| `internal/app/engine_api.go:18-28,44-85` | Authenticated app facade sends bounded, strict DTOs to one fixed private helper endpoint; raw Docker/network errors are not returned. | Reuse the facade/private-control-plane architecture, not a browser-to-Docker proxy. |
| `internal/app/engine_config.go:10-13,45-57` | Main app knows only a private Unix socket and read-only published connection directory; it has no Docker permission. | Plugin execution authority should remain outside the main media service. |
| `internal/app/engine_runtime.go:43-66` | Fixed Unix-socket client, redirects rejected, bounded timeouts; refresh loop joins app context/wait group. | Model catalog refresh and runner-status polling on bounded clients and cancellable lifecycle loops, with distinct trust policies for external markets. |
| `internal/enginehelper/service.go:48-131` | Root-owned installation state, lifetime lease before opening state DB/recovering work, one persistent executor, restart recovery. | Reuse durable intent, idempotency and reconciliation concepts for install/update/uninstall; do not make remote downloads or Docker calls inside main-DB transactions. |
| `internal/enginehelper/service.go:146-199` | Helper runs only on Linux, serves a Unix socket protected with ownership and mode 0660; no TCP mapping. | Native Windows cannot directly execute this helper. Docker Desktop Linux mode can host a Linux helper, but ordinary Windows mode should advertise unavailable capabilities. |
| `internal/enginehelper/templates.go:22-29` | Exactly qBittorrent and Jackett, with fixed versions and immutable image digests. | General market manifests are not acceptable replacements for the fixed templates without a separately designed policy boundary. |
| `internal/enginehelper/templates.go:73-110,114-174` | Fixed dedicated binds/volumes/network/ports/security options; reuse requires exact installation/template/image/mount/port checks. | Preserve current ownership validation. New plugin resources need their own namespace, labels, mounts, requested-permission review and validated deployment plan. |
| `internal/enginehelper/store.go:40-85` | Separate `state/state.db` with helper-specific `application_id`, schema 1, tasks, idempotency bindings and events. | The existing executable contract explicitly excludes helper task/secret tables from `library.db`; any new plugin runner should own its execution journal similarly. |
| `internal/enginehelper/service.go:355-410,445-538` | Request signatures, active-task conflict detection, explicit notice acceptance and revision checks. | Installation operations should use request IDs, immutable release/permission plans, optimistic revision checks and safe retries. |
| `internal/enginehelper/stop.go:19-31,50-104` | Stop-all targets exactly the two owned fixed components. | A new plugin subsystem needs an explicit stop policy; changing desktop shutdown to include plugins requires a deliberate contract update. |
| `Dockerfile.engine-helper:12-23` | Scratch Linux image with only the helper executable: no shell or Docker CLI. | Executing a generic fpk shell lifecycle in the existing helper cannot work merely by adding a parser. Use a dedicated fnOS host adapter or a separate carefully scoped runtime. |

Do not turn `engineTemplate(component)` into an arbitrary URL/image/shell loader, accept raw Compose/Docker JSON from the browser, mount the Docker socket into the main app, or automatically claim existing containers/data. Such changes would remove the primary controls of the current deployment contract.

### 4. Cross-platform and desktop packaging constraints

- `internal/desktop/config.go:19-45` defines persisted `native` and `docker` profiles with explicit media/data paths, selected local context and immutable image-manifest path. New plugin settings need to respect this separation.
- Native mode (`internal/desktop/launcher.go:365-405`) loads the shared Go service in-process using an immutable configuration lookup. Catalog browsing/source management can work here, while Linux container/fnOS actions need capability gating.
- Docker Desktop preflight (`internal/desktop/docker.go:153-239`) currently accepts only one read-only media root, a local named-pipe context, Linux Docker Desktop, amd64 and explicit resource minimums. It rejects remote/TCP/other-host Docker endpoints (`:199-221`). A remote fnOS host connector must be a new authenticated feature, not a relaxation of this local Docker selector.
- `internal/desktop/docker_compose.go:92-150` validates exactly two services, exact environment maps, exactly seven named-volume roles and a fixed bridge. Simply adding another helper service or mount to `compose.desktop.yaml` will be rejected until the corresponding versioned validation/release material is updated.
- `internal/desktop/docker.go:447-478,501-524` separately validates live container mounts and named-volume ownership; source YAML and runtime validation must be changed together.
- `internal/desktop/docker.go:690-707` performs the owned engine stop-all operation before Compose shutdown. New plugin containers launched outside the fixed Compose services would otherwise survive that stop path.

### 5. Vue integration and UI conventions

| File and current lines | Relevant pattern | Proposed integration |
| --- | --- | --- |
| `web/src/types.ts:1-4` | Five `ContentType` media types; `ViewName` adds management views. | Add `plugins` to `ViewName`, never to `ContentType`. |
| `web/src/types.ts:82-90` | `Settings` is explicitly read-only. | Add separate `PluginMarketSource`, `PluginRelease`, `PluginInstallation`, `PluginCapabilities`, `PluginOperation` DTOs. |
| `web/src/App.vue:12-24` | Heavy management views lazy-load; desktop management list and mobile More share navigation definitions. | Lazy-load `PluginCenter.vue`, add management navigation and expose it from More. Keep the five mobile bottom entries. |
| `web/src/App.vue:68-74,187-200` | Content/management distinction and hash restoration are explicit. | Register plugin view/hash and a separate catalog search; global media search should keep its original meaning. |
| `web/src/App.vue:310-315,419` | Settings use an API fetch and deployment/read-only explanation. | Add a market-settings section/dialog with validated URL, source kind, enable/disable, refresh, trust information and safe credential references. |
| `web/src/api.ts:14-27,31-38,42-48` | Same-origin typed JSON/multipart requests, abort signal, session-expired event, stable random request IDs. | Reuse API helpers; never fetch market data directly from the browser or expose registry tokens. |
| `web/src/engines.ts:11-37` | Presence/runtime/application readiness are distinct; real layer progress only. | Distinguish incompatible, unavailable runtime, installed, stopped, healthy, updating and failed; show stage/progress evidence without invented percentages. |
| `web/src/components/DialogShell.vue` and `EngineSetupDialog.vue` | Native modal lifecycle, focus restoration, operation progress and retries. | Reuse interaction conventions, keep market settings/install confirmation single-modal, abort and stop polling on close. |

Avoid arbitrary remote JavaScript/Vue loading into the host page. An installed web application's launch entry can initially open a separately validated origin; same-origin reverse proxy or iframe embedding would need separate authentication, cookie, CSP, WebSocket and origin-isolation design. The current `server.go:274-275` security policy is not a ready-made remote-plugin sandbox.

### 6. Deployment and backup impact

- `compose.yaml:14-17` mounts only main app data and read-only media. Keep plugin execution optional so the existing single-container/media-only deployment remains valid.
- `compose.engines.yaml:3-25,34-50` demonstrates a main app without Docker access, read-only published connections and a privileged helper on a private IPC volume. A new `compose.plugins.yaml` could use a similar explicit opt-in boundary, separate owned data/IPC and a dedicated runner.
- `compose.desktop.yaml:18-39,83-110,159-244` distinguishes app data, engine state/config, downloads, secrets publication and IPC role volumes. General plugin data must not borrow qB/Jackett volumes or the unpublished download root.
- `Dockerfile:37-43` keeps the main service non-root; `Dockerfile.engine-helper:12-23` keeps Docker authority in a trusted separate executable. Preserve this split.
- Main backup is a database snapshot only. A future plugin backup/export needs a manifest recording installation identity, exact release/digest, selected permissions, data locations and recovery instructions. Do not copy live helper WAL databases or disclose secret files through the existing `/api/backup` endpoint.

### 7. Suggested module and data/API boundaries (proposal, not implemented)

Minimal scope additions:

```text
internal/plugins/                  Catalog normalization, source adapters, manifests, compatibility and permission plans
internal/plugincontract/           Typed safe facade/runner DTOs, versioned independently from fixed engine DTOs
internal/pluginhelper/             Optional trusted execution journal and Docker runtime
internal/plugins/fnos/             Read-only fpk metadata/dependency classification, then explicit fnOS host adapter
cmd/nas-plugin-helper/             Optional Linux runner entry point
internal/app/plugin_*.go           Authenticated API facade, lifecycle and public state integration
web/src/plugins.ts                 Typed helpers and labels
web/src/components/PluginCenter.vue
web/src/components/PluginMarketSettings.vue
web/src/components/PluginInstallDialog.vue
compose.plugins.yaml               Explicit opt-in deployment overlay
docs/plugins.md                    Market protocol, operator setup, compatibility and backup boundaries
```

The exact package count can be reduced during implementation; these are responsibilities, not a request for an unrelated architecture rewrite.

Proposed public API groups:

- `GET /api/plugins/capabilities`: server OS/architecture, runner kind/availability, fnOS capability and supported source formats; no side effects.
- `/api/plugin-markets`: authenticated source list/create/update/delete; kind must select a known parser, URL is configuration and never an executable instruction.
- `POST /api/plugin-markets/{id}/refresh`: bounded server-side fetch with persisted last-good cache/error; no installs.
- `GET /api/plugins/catalog`, `GET /api/plugins/catalog/{qualifiedId}`: normalized list/details, compatibility reason and provenance.
- `POST /api/plugins/install-plans`: resolve source+plugin+immutable version/digest into a permission/runtime plan before effects.
- `POST /api/plugins/installations`, `/api/plugins/installations/{id}/actions`: accept plan identity, request ID and revision; execute through the selected runtime adapter.
- `GET /api/plugins/operations/{id}?afterSequence=...`: safe persistent task stages/events; bounded cursor response.

Data ownership decision to settle in the plan:

1. Recommended integration: public market records/catalog cache and desired installation references in an additive main schema-v3 migration; privileged execution journal, resource ownership and secrets remain in a separate plugin-runner state store/private directory. The runner is authoritative for actual resource identity and operation effects; app rows are not proof of ownership.
2. Smaller initial migration option: separate plugin-control-plane database for all new records, leaving `library.db` at v2. This makes backup/restore and service lifetime more complex and must be explicit rather than silently omitting plugin configuration from the existing backup.

In both options, qualify catalog identity with source identity plus plugin identity, bind installs to immutable release digests and verified source metadata, retain installed state when a market is disabled/deleted, preserve data on uninstall by default and never merge identically named packages across markets automatically.

### 8. Recommended sequence and validation inputs

1. Define source protocol/normalized manifest, capability model, ownership model and backup boundary; choose a small native catalog and two existing fixed engines as reference cases.
2. Deliver source CRUD and read-only catalog browsing first; prove URL validation, bounded fetch/cache, partial source failure, source provenance and compatibility display without Docker side effects.
3. Add an optional generic runner using reviewed declarative container plans; prove ownership checks, idempotent intent, crash recovery, data-preserving uninstall, health checks and upgrade recovery on isolated data.
4. Add the actual fnOS runtime adapter only after the external-research capability/format findings and a real fnOS test host establish a supported management contract. A Docker runtime is not itself proof of fnOS lifecycle compatibility.
5. Migrate or expose existing qB/Jackett engines only through an explicit compatibility adapter and ownership-aware transition; preserve existing APIs/config/selected-download publication and do not auto-adopt installed resources.
6. Update the exact desktop Compose validator, packaging and shutdown paths together when Windows Docker mode gains plugins; native Windows continues to show honest unsupported reasons where required.

Implementation validation should preserve the existing media/auth/5+ GiB Range/download-publication/search regressions; add market URL/redirect/digest/provenance, operation concurrency/restart/ownership, upgrade/uninstall data handling and UI theme/viewport/error/cancellation cases. Run local commands through `scripts/project_env.py`. Per AGENTS.md, remove newly generated temporary unit-test classes after execution but retain reusable integration harnesses and evidence. No tests were generated or run by this research task.

## Related Specs

- `.trellis/spec/backend/runtime-contract.md`: schema, authentication, request limits, media/lifecycle invariants.
- `.trellis/spec/backend/engine-bootstrap-contract.md`: fixed trusted helper, separate DB, idempotency, private IPC and resource ownership.
- `.trellis/spec/backend/download-contract.md`: selected-file publication and immutable download bindings to preserve.
- `.trellis/spec/backend/torrent-search-contract.md`: private-source credential isolation and bounded network behavior.
- `.trellis/spec/backend/cross-platform-contract.md`: desktop modes, immutable deployment assets, lifecycle and project-local runtime commands.
- `.trellis/spec/frontend/app-contract.md`: navigation, lazy loading, same-origin typed APIs, modal/polling/error contracts.
- `.trellis/spec/guides/cross-layer-thinking-guide.md`: explicit data flow and boundary contracts.

## External References

No external compatibility claims were researched in this file. Refer to sibling `fnos-compatibility.md` for official fnOS package/market/runtime evidence. Code paths and line numbers above refer to the repository observed on 2026-11-19.

## Caveats / Not Found

- This is a modification plan input, not evidence that a plugin system is implemented or that any fnOS package can run on this project.
- The existing helper's Linux requirement, scratch image and exact two-component protocol are deliberate constraints; an arbitrary fpk shell loader would require a different trust/runtime design.
- The current desktop Docker mode supports local Linux amd64 Docker Desktop only. Remote fnOS management and ARM desktop support are not existing capabilities.
- No live Docker daemon, fnOS host, third-party market, credentials, package installation, build or behavioral tests were used.
- `rg` over `compose*.yaml` as a literal Windows argument returned a glob error once; all relevant Compose files were subsequently inspected by exact paths. This did not affect source findings.
