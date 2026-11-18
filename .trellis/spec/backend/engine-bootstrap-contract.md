# Engine bootstrap contract

## 1. Scope / Trigger

Applies to `cmd/nas-engine-helper`, `internal/enginehelper`, `internal/enginecontract`, app `engine_*.go`, managed runtime wiring and engine UI. This optional control plane must preserve media streaming, download publication and external-engine contracts.

## 2. Signatures

- `enginehelper.LoadConfig() (Config, error)`, `New(context.Context, Config, *slog.Logger) (*Service, error)`, `Run(context.Context, Config, *slog.Logger) error`.
- Public API: `GET /api/engines`, `POST /api/engines/probe`, `POST /api/engines/initializations`, `GET /api/engines/initializations/{id}?afterSequence=`, `POST /api/engines/initializations/{id}/retry`, `POST /api/engines/{component}/start`, `POST /api/engines/search/sync-sources`.
- Helper-only `POST /api/engines/stop-all` takes strict `{"requestId":"<id>"}`, returns the existing `TaskResponse` with HTTP202, and persists action `stop` for exactly `qbittorrent,jackett`. `/nas-engine-helper stop-all` connects to the existing Unix socket and polls up to90s; it never opens a second store or installation lease. This is not a new public app endpoint.
- Shared safe DTOs are `Status`, `Component`, `TaskResponse`, `Task`, `Step`, `Layer`, `Event`, `InitializationRequest`, `ActionRequest` in `internal/enginecontract/models.go`. Exact public shapes are documented in `docs/api.md`.
- Helper uses its own SQLite application ID/schema in `state/state.db`; never add helper task/secret tables to the main app database. Main database remains schema v2.

## 3. Contracts

- Explicit `NAS_DOWNLOAD_MODE` / `NAS_TORRENT_SEARCH_MODE` accepts only `disabled|external|managed` and overrides legacy enabled flags. Unset uses legacy false→disabled / true→external. External still requires validated credentials; managed may start without a manifest.
- Helper only uses a local Docker Unix socket and serves a private Unix socket. Browser input is fixed components (`qbittorrent|jackett`), requestId, notice acceptance, revision and safe source IDs; never an image, host path, command or raw container JSON. Root helper has real daemon authority; a DTO allowlist is not a daemon sandbox.
- GET status does not claim config directories, pull images, create/start containers or search sites. Presence is missing only after successful Docker metadata/preflight plus target404. Unreachable, failed auth, conflict and stopped are distinct.
- Persist request binding and step intent before effects. Keep one executor and one active task; same requestId with same signature recovers the original task, changed signature conflicts. Different request IDs for an active identical target alias it. Retry only failed steps; retain successful data/credentials.
- Acquire installation-level Linux `flock` **before** opening the store, reading installation ID or restoring tasks. Hold it through worker and DB shutdown. Never unlink the lock file or use a socket-only race check.
- Validate installation/component/template labels, fixed image content, all binds, network, fixed ports and qB tmpfs before reuse. Docker20.10 reports tmpfs in `HostConfig.Tmpfs`, not `Mounts`: accept only the fixed `/tmp`64MiB rw/noexec/nosuid settings, including normalized size units.
- Pull HTTP200 is insufficient: reject error/errorDetail, empty EOF, truncation and unverified final image. Keep real bounded layer progress, not an invented overall percentage. Inspect after uncertain create instead of creating blindly.
- Fixed qB5.2.4 offline password uses Qt PBKDF2-HMAC-SHA512/100000/salt16/key64. Persist once, atomic fsync/rename; do not parse temporary-password logs, rewrite auth or guess a password environment variable. Probe a valid SID/QBT_SID_port cookie and version. App activation also probes its production adapter.
- Jackett preflight tests `pwritev2`: ENOSYS permits runtime fallback; EPERM fails with compatibility guidance before pulling. Do not automatically disable seccomp. Its discovery is authenticated Torznab `t=indexers&configured=true`, not a cookie-protected management API.
- Private directory protocol v1: atomic `current.json` selects per-component generation JSON. Strict fields/EOF, positive and bounded generations, matching installation/component/container, ordinary non-symlink files and permission/size checks. Never expose `Connection` through browser DTO/logs/backups. Persist app identity/source selections without secrets; reject silent instance replacement.
- App uses immutable connection/source snapshots; 5s refresh, preserves last good runtime on malformed update/helper outage. Source replacement bumps generation and invalidates unbound cached search tokens; already persisted download bindings remain retryable.
- Discovery XML:2MiB, no DTD/deep or excessive structures, safe unique IDs/names. Redact decoded known API Key before title validation; if an upstream ID embeds it, expose only a stable derived ID and retain actual indexer only server-side. 0→configuration_required,1–8→auto activation,>8→explicit ≤8 selection. Never perform a keyword query during setup.
- `ENGINE_STORAGE_MODE=bind` remains the Linux default. `volume` requires `ENGINE_VOLUME_PREFIX` (4–64 lower-case alphanumeric/internal-hyphen characters), empty `ENGINE_HOST_ROOT`, `/managed` and `/ipc/helper.sock`. Exact roles `state,qbittorrent,jackett,downloads,connections,ipc` map to `${prefix}-<role>` and fixed helper targets. Require local driver/scope, no driver options, and matching deployment/role labels; extra/nested/duplicate mounts fail. Never convert Docker's internal Mountpoint into a bind.
- Volume engine templates use `HostConfig.Mounts` with `Type=volume` and `VolumeOptions.NoCopy=true`: qB receives only config/downloads, Jackett only config. App downloads/connections/IPC are read-only. Existing bind templates and host mapping retain their previous checks.
- State role's private atomic `.nas-engine-volume` marker fixes schema/deployment/role/random installation identity under flock before opening SQLite; DB identity must match it. A crash after marker publication reuses the marker identity. Startup may claim only empty state/IPC; persisted user-authorized initialization claims config/downloads/connections. Status never claims these data roles. Reject foreign/nonempty/unmarked data, symlinks, unsafe owners/modes and inconsistent identities instead of resetting.
- Stop persists intent, verifies actual owned container ID/image/template/mounts before each effect, and reconciles uncertain Docker responses against that exact original. Missing/already stopped owned engines are no-ops; foreign resources fail. Stop never pulls/creates/deletes. An accepted stop closes a process-level gate to new initialize/start until helper restart. Await completed stop before stopping Compose; launcher timeout must exceed the helper's90s budget. Restarting Compose does not itself start stopped dynamic engines; use their existing explicit Start action.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing app session / foreign mutation origin | 401 / 403 |
| Unknown JSON field, malformed ID/component or negative cursor | 400;64KiB budget413 |
| Notice not actively accepted/current | 409; no engine side effect |
| Same requestId with changed signature / stale revision | 409; preserve original task |
| Different active targets | 429; no second executor |
| Task absent/pruned | 404 with fixed safe explanation; UI clears stale recovery ID |
| Helper/socket/daemon unavailable | 503 or safe status unknown; never missing |
| Unknown nonempty config or foreign same-name container | conflict; no takeover/reset |
| Pull stream error / auth/version/preflight failure | failed task with safe Chinese events; preserve files |
| Jackett EPERM write compatibility failure | fail before pull; default security unchanged |
| Bad manifest/source discovery | keep prior active configuration, safe error |
| Source set above8 / invalid selected IDs | selection_required /400 |
| Volume driver/options/role mount/marker mismatch or unknown nonempty role | conflict; no claim, takeover or reset |
| Active initialize/start when stop-all requested | 409; no stop side effect |
| New initialize/start after this helper accepted stop | 409 through quiescing gate; restart helper for a new lifecycle |
| Stop sees foreign actual image/mount/identity | fail without stopping that container |
| Stop CLI unavailable/failed/timeout | nonzero exit; launcher does not continue to Compose stop |

## 5. Good / Base / Bad Cases

- Good: deploy optional overlay once; explicit click initializes missing fixed engines; browser reconnect recovers original task; application verifies permanent qB credentials; Jackett without sites remains configuration_required.
- Base: two concurrent helper processes for one root cannot open independent recovery executors; stopped owned engine starts in place without a new install; helper outage leaves media/runtime intact.
- Volume base: restart after state marker publication but before DB creation preserves installation identity; repeated completed stop has no additional engine effect and retains every volume.
- Bad: daemon timeout is treated as missing, pull200 error is treated as success, config files come from another installation, upstream titles contain API Key, or an old response appends events to a new task. Reject/preserve/isolate rather than reset.

## 6. Tests Required

- Go vet/build; strict frontend typecheck/production build; Linux amd64/arm64 compile. Execute temporary behavioral tests then delete only generated tests per AGENTS.md; empty final go test is a compilation check, not behavioral coverage.
- Fake Docker tests: streaming errors/progress, lost create response, ownership, durable idempotency/restart, strict private HTTP, process lease, tmpfs. Linux actual files: UID/fsuid access, atomic secrets, manifest fields/EOF/bounds and pwritev2 policy branch.
- Volume/stop coverage: strict six-role metadata and mounts, no-copy native volume JSON, no-claim status, crash-safe marker/DB binding, foreign/symlink content refusal, actual Unix stop client/server, active-task conflict, foreign actual image, uncertain stop reconciliation, durable task reload/idempotency and post-stop gate. WSL1 does not establish Linux-container fsuid support; keep real access probes strict and record a capability skip instead of lowering permissions.
- `scripts/verify_engine_bootstrap.py`: real app/SQLite/production adapter with local controlled qB/Torznab, auth/origin/body/notice, absence semantics, manifest activation/restart,0/9/selection sources, malformed XML, old config retention, no secrets or keyword query. Keep download/search/52-point five-media plus5GiB Range regressions.
- UI: eight theme/width combinations, focus/Esc/native single modal, checkbox not preselected, task/event identity reset, stable POST retry IDs and revision, refresh recovery, no hidden polling overlap or stale responses; mobile remains five entries.
- Remote Docker tests must snapshot baseline, track every task container/image/network/root with a unique run marker, inspect actual scope before deletion and verify cleanup. Never prune globally. Distinguish scratch test images, production Dockerfile build, direct pinned pull, loaded verified archive and diagnostic security relaxation in evidence.

## 7. Wrong vs Correct

```go
// 错误：先恢复任务再检查socket，另一进程可能同时开始部署。
db := openStoreAndRecover()
checkSocket()

// 正确：以安装持久目录锁覆盖状态打开、恢复和执行器整个生命周期。
lease, err := acquireLease(lockPath)
if err != nil { return err }
defer lease.Close() // 锁文件不删除；先等待执行器和状态库退出。
```

Do not concatenate remote Docker errors, raw XML, private URL or engine logs into public errors. Use controlled Chinese messages and safe task sequence events. Deployment and real engine readiness are separate facts; fixture or diagnostic success must not be reported as default production deployment success.

Wrong: `compose stop` alone means helper-created engine containers have stopped. Correct: wait for the fixed private stop-all task to complete, then stop Compose while the helper rejects new starts. Wrong: infer ownership from a volume name or Mountpoint. Correct: validate logical volume metadata, exact mounts and persistent role/installation markers together.
