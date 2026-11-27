# Helper named-volume storage and fixed-engine stop implementation

Date: 2026-11-18. Scope: `internal/enginehelper/*` and `cmd/nas-engine-helper/*` only, plus this implementation record. The existing main application database schema is unchanged.

## 1. Scope and delivered behavior

- Linux bind storage remains the default and preserves the existing host-root/container-root mapping, template labels, pinned images, authentication, permission probes, private manifests and API behavior.
- Docker Desktop full mode selects a separate, trusted named-volume storage driver. Browser requests cannot provide a volume name, host path, image, command or Docker JSON.
- The helper continues to run only inside a Linux container, with a private Unix HTTP control socket and no Docker TCP/proxy endpoint.
- Fixed-engine stopping is a durable helper task. It never pulls, creates or deletes a container, image, network or volume.

## 2. Configuration and Compose contract

| Parameter | Contract |
| --- | --- |
| `ENGINE_STORAGE_MODE` | `bind` by default; only `bind` or `volume` |
| `ENGINE_VOLUME_PREFIX` | Required only in volume mode; lower-case letters, digits and internal hyphens, 4–64 characters; no trailing hyphen |
| `ENGINE_HOST_ROOT` | Required clean absolute non-root bind source in bind mode; must be empty in volume mode |
| `ENGINE_MANAGED_ROOT` | Existing bind behavior retained; volume mode fixes it to `/managed` |
| `ENGINE_HELPER_SOCKET` | Existing bind behavior retained; volume mode fixes it to `/ipc/helper.sock` |

For a trusted prefix such as `qiku-0123456789ab`, Compose creates these `local` driver volumes:

| Role | Exact volume name | Exact helper target | Helper access |
| --- | --- | --- | --- |
| state | `${prefix}-state` | `/managed/state` | RW |
| qbittorrent | `${prefix}-qbittorrent` | `/managed/qbittorrent` | RW |
| jackett | `${prefix}-jackett` | `/managed/jackett` | RW |
| downloads | `${prefix}-downloads` | `/managed/downloads` | RW |
| connections | `${prefix}-connections` | `/managed/connections` | RW |
| ipc | `${prefix}-ipc` | `/ipc` | RW |

Every role volume requires labels `nas.engines.deployment=${prefix}` and `nas.engines.role=<role>`, exact name, `Driver=local`, `Scope=local` and no local-driver options. NFS/device/bind options are rejected even when the driver is named `local`. Compose-owned metadata labels may coexist.

The app mounts downloads, connections and IPC read-only. qB mounts its config volume at `/config` and downloads at `/downloads`, both RW; Jackett mounts only its config volume at `/config` RW. Engine creation uses API `HostConfig.Mounts` with `Type=volume` and `VolumeOptions.NoCopy=true`, never a daemon `Mountpoint` disguised as a bind or a volume subpath feature. The serialized fields are available in API 1.41.

`compose.desktop.yaml` was read and matched against this contract. No Compose file was changed by this child agent.

## 3. First binding, ownership and recovery

1. Before modifying role storage, read-only Docker preflight verifies Linux/API/network and the helper's exact role mounts plus daemon volume metadata.
2. Only an empty state role, or a state role with its existing binding, may proceed. A root-owned regular `helper.lock` with mode `0600` may remain after an interrupted fresh bootstrap. Unknown nonempty state is rejected.
3. State is private `0700` root:root. `flock` is acquired before reading or generating the installation identity, opening SQLite or restoring tasks.
4. Under the lease, `.nas-engine-volume` is atomically written with strict version/deployment/role/installation fields. The state marker fixes the random installation ID before the initial state database is opened. A crash after marker publication but before SQLite creation therefore reuses the same identity.
5. `openBoundStore` validates an existing helper database identity and checks its stored installation against the marker. It does not reset mismatches.
6. IPC is a helper startup resource, so its empty volume is bound after the installation is known. Configuration, connections and downloads are only bound by a previously persisted user-authorized initialization task. Status/probe and stop tasks do not claim them.
7. Every later preflight checks all existing role bindings. Files are bounded regular non-symlink private files with checked owners, strict JSON fields/EOF and matching deployment/role/installation. A missing marker is accepted only on an empty unclaimed role. Unknown nonempty data, foreign identity, driver, label, mount target, access mode, duplicate target or nested override fails closed.

Bindings are durable filesystem metadata, not imaginary Compose-created random-installation labels. A complete backup must preserve the helper state, role bindings, engine configuration, private connections and downloads together. Volume data is Docker Linux data, not a Windows `Downloads` directory.

## 4. Stop command and protocol

The desktop launcher command is:

```text
docker compose exec -T nas-engine-helper /nas-engine-helper stop-all
```

Only after this returns exit code 0 should the launcher stop its Compose services. The command reads `ENGINE_HELPER_SOCKET` (default `/ipc/helper.sock`), connects to the existing helper, and never calls `New`, opens SQLite or obtains a second installation lease.

Private protocol:

```http
POST /api/engines/stop-all
Content-Type: application/json

{"requestId":"<safe idempotency ID>"}
```

- The request is strictly decoded with the existing 64 KiB/unknown-field/EOF rules.
- It creates or aliases a persisted task with action `stop` and exactly `qbittorrent,jackett`. The response is the existing `TaskResponse` with HTTP 202.
- An active initialization/start action produces HTTP 409. No second executor is created.
- The CLI polls the existing task endpoint with a total 90-second budget and only succeeds for its matching completed stop task. Invalid/unavailable/failed/timeout results return a nonzero exit code with a controlled Chinese explanation.
- On accepting a new stop task, the current helper process closes a quiescing gate against new initialize/start and non-stop retries. This prevents a browser request restarting an engine between CLI completion and Compose shutdown. A recovered active stop task also closes the gate. A later normal helper process starts with the existing task-recovery rules.
- For each existing deterministic engine name, verify installation/component/template/storage/deployment labels, pinned image reference and actual image content, exact mounts, network, fixed ports and qB tmpfs. Then persist the stopping phase and address the already verified **container ID**, not just its name.
- Docker receives a fixed 20-second graceful stop allowance, inside a 30-second bounded call. Ordinary Docker calls retain their 10-second context budget. Stop results are re-inspected: an error response with confirmed stopped state succeeds; uncertain state, changed ID/image/template or a still-running container fails safely.
- A successful inspect 404 is idempotent absence. Already-stopped verified resources are preserved. All containers, images, volumes, configuration, credentials, connections and downloads remain intact.
- Successful stopping does not automatically restart completed engines after helper restart; the existing user-authorized original-container start action remains the route to start them again.

## 5. Validation matrix

| Case | Result |
| --- | --- |
| Missing bind host root, bad mode/prefix, volume plus host root, overridden volume internal paths | Startup config error |
| Wrong driver/scope/volume options/deployment/role/name | Fail preflight; do not claim |
| Duplicate/wrong/RO/missing helper role mount or nested role override | Fail preflight |
| Marker missing on empty config/downloads/connections | Detect only; no status-side mutation |
| Marker absent on unknown nonempty role, symlink/private-file mismatch, foreign installation | Conflict; preserve contents |
| Second process for the same state | Linux lease rejects it before SQLite/recovery |
| Active initialization/start when stop requested | HTTP 409; no stop task side effect |
| Foreign mount, label/template or actual image content during stop | Failed task; no Docker stop for that resource |
| Docker stop response uncertain but exact inspected original is stopped | Complete existing step; no retry/create/pull/delete |
| Whole stop task complete then executed again | No repeated engine side effect |
| New initialize/start while current process has accepted complete stop | Conflict through quiescing gate |

## 6. Checks actually executed

The temporary Linux test suite was cross-compiled with `.tools/go/bin/go.exe test -c` and executed in the already installed `Ubuntu-22.04` WSL distribution, using `TMPDIR=/tmp`. Permission evidence uses WSL's native `/tmp`, never `/mnt/e` file permissions. No Docker daemon or remote server was started or used.

Latest behavior result: **9 tests passed, 1 explicitly skipped**:

- PASS `TestVolumeConfig`: unchanged bind default and volume configuration rejection boundaries.
- PASS `TestVolumeTemplates`: both fixed engine templates under bind/volume, exact mounts, foreign/extra mount rejection, native `NoCopy` volume specification.
- PASS `TestVolumeIdentity`: role/deployment/name/driver/scope/options rejection using an in-process API 1.41 fake Docker transport.
- PASS `TestVolumeHelperMounts`: all six fixed helper targets, access, duplicate targets, missing role and nested override rejection.
- PASS `TestVolumeBindingFiles`: actual Linux-file `chmod/chown`, state lock exclusion, atomic markers, installation/database reload and mismatch rejection, read-only no-claim behavior, foreign nonempty and symlink rejection, and existing Jackett write-probe policy.
- PASS `TestStopAllUnix`: real Unix socket HTTP client/server plus real helper SQLite, fake Docker only; two engine stops, a simulated HTTP 500 with reconciled stopped state, repeated invocation without another stop, no create/pull/delete/data-directory creation, and post-stop initialization blocked.
- PASS `TestStopConflicts`: foreign mount never stopped and active initialization returns 409.
- PASS `TestStopImageConflict`: matching labels/reference cannot authorize stopping a different actual image.
- PASS `TestStopDurableReload`: stop task is reloaded from reopened real SQLite, executes the original intent, and a second execution of the completed task has no side effect. This is database/task recovery coverage, not a full operating-system process restart or real Docker restart claim.
- SKIP `TestTargetUIDAccessProbe`: this host is **WSL1**, kernel `4.4.0-19041-Microsoft`. It reports successful fsuid/fsgid switching but denies the target write with EACCES. Positive fsuid capability remains unverified here; the production helper keeps rejecting the failed write rather than relaxing permissions.

Additional successful checks:

- `gofmt` on changed Go files.
- Helper/package `go vet` for Windows amd64 and Linux amd64.
- Windows amd64 helper compilation, Linux amd64 helper cross-compilation, Linux arm64 helper cross-compilation. The Windows helper binary is compile coverage only; the server remains intentionally Linux-only.
- `git diff --check` for the owned code paths.

The task-required temporary `zz_volume_temporary_test.go` source and its temporary test binary are removed after execution. Sanitized command results are retained in this document and local ignored `.tools/helper-volume-check/results.txt`.

## 7. Remaining acceptance boundaries and implementation lessons

- Actual Docker Desktop/WSL2 volume metadata, Linux UID/fsuid behavior, fixed production image deployment, real qB/Jackett health/authentication, graceful stop timing and restart behavior require the user's manual deployment validation. Cross-compilation and fake Docker are not substitutes.
- No remote Docker endpoint was contacted. No test containers/images/networks/volumes were created, so there are no Docker test resources to delete. No Docker/WSL system setting was modified.
- In the host's WSL1, `setfsuid(-1)` / `setfsgid(-1)` queries also changed the emulated identity to `4294967295` according to strace. The helper now re-sets the desired UID/GID and verifies the returned previous value instead of using the sentinel. This preserves the actual access probe and restoration. It does not turn WSL1 into a verified deployment target.
- Root cause category: implicit environment assumption. An initial test-fixture parent-permission correction was insufficient; syscall tracing separated parent traversal from compatibility-layer fsuid semantics. Future validation must record the kernel/runtime and distinguish genuine Linux-container evidence from WSL1 compatibility behavior.

Wrong versus correct:

```text
Wrong: inspect a Docker volume's Mountpoint and pass it as an engine bind source.
Correct: validate the role volume's logical name/labels/driver and use Type=volume.

Wrong: make GET status create owner markers to make an empty volume look installed.
Correct: GET only validates; persisted authorized initialization binds data roles.

Wrong: compose stop alone implies helper-created engines have stopped.
Correct: await the fixed private stop-all task, verify exact originals are stopped,
         then stop the Compose services while the helper rejects new starts.
```

Changed code files: `cmd/nas-engine-helper/main.go`; `internal/enginehelper/config.go`, `docker.go`, `files.go`, `permissions_linux.go`, `service.go`, `store.go`, `tasks.go`, `templates.go`; new `stop.go` and `volumes.go`.
