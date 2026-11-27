# Desktop implementation evidence

- Date: 2026-11-27
- Scope: `internal/desktop/*.go`; runtime acquisition and public documentation remain owned by the parent task.

## Delivered

- Added `uav_initialization.go`: one owned background initializer after actual media readiness, short-lock progress, stale-task rejection, cancellation and completion barriers, fixed offline diagnostic and serial activation.
- Eligibility remains native managed mode, enabled UAV, Windows amd64, and no explicit custom Python. Disabled, external, false and custom Python do not initiate installation. Invalid custom Python never falls back.
- Static-valid packaged dependencies retain preference; otherwise a validated published profile generation is selected. Manifest, tool and PATH overrides affect only the copied child environment.
- Optional IPC `collectionInitialization` uses `kind`, `state`, `stage`, `downloadedBytes`, `totalBytes`. Tray presents the Chinese stage and current material's actual MiB counts; no invented aggregate percentage. Initialization never fabricates an available worker capability and does not degrade a ready JMComic capability.
- Completion enters `controller.run`, checks current native/managed/readiness/worker ownership and pending commands, then uses existing full stop/start. Automatic activation does not reopen the browser and cannot create an initialization loop.
- Stop, disable, recheck, quit and unexpected media/worker exit cancel and await initialization. A genuine `uavbootstrap.ErrCleanup` remains a restart-blocking error for the process; ordinary network/probe failure does not block explicit retry. New user-triggered lifecycle clears the attempted flag only after successful stop.
- A new profile can infer managed mode from explicit UAV=true plus adjacent worker. Existing saved disabled/external configuration remains authoritative.
- The final probe executes fixed adjacent `nas-collection-worker.exe --check-uav` inside the existing owned Windows Job with controlled environment, a 60-second deadline, bounded process cleanup, and discarded raw tool output.

## Files changed

- `internal/desktop/uav_initialization.go`
- `internal/desktop/launcher.go`
- `internal/desktop/collection_managed.go`
- `internal/desktop/collection_controller.go`
- `internal/desktop/collection_config.go`
- `internal/desktop/shell_windows.go`

## Executed verification

All Go and formatting commands ran through `python -B scripts/project_env.py -- ...`; controlled cache and TEMP were inside repository `.runtime/`.

1. gofmt of all six changed/new Go files: passed.
2. `.tools/go/bin/go.exe test ./internal/desktop -run TestUAV -v -count=1`: passed, final behavior run reported `ok ... 2.745s`.
3. `.tools/go/bin/go.exe vet ./internal/desktop`: passed.
4. `.tools/go/bin/go.exe build -o .runtime/verify/uav-desktop-implementation.exe ./cmd/nas-desktop`: passed.
5. `git diff --check -- internal/desktop`: passed (only configured Git LF/CRLF conversion warnings).

The temporary test sources were `uav_initialization_temp_test.go` and `uav_initialization_process_temp_test.go`; both were removed after execution per repository instruction. Nine test groups covered:

- `TestUAVAutomaticEligibility`: unset/manga/enable/custom-invalid/custom-space/platform cases and missing-package rejection.
- `TestUAVChildEnvironment`: case-insensitive replacement, no duplicate controlled keys, immutable input and preserved explicit interpreter.
- `TestUAVSnapshotKeepsActualCapabilities`: JMComic stays ready, UAV remains unavailable, status does not mutate shared snapshots, actual byte formatting.
- `TestUAVProgressOwner`: old and revoked progress cannot overwrite the active state.
- `TestUAVStopWaitsAndKeepsCleanupFailure`: stop actually waits for the cancellation/cleanup barrier and joined cancellation does not hide ErrCleanup.
- `TestUAVCompletionLifecycle`: normal activation, pending command, disabled, external, failed media, exited worker, canceled context and failed initialization; one attempt and explicit reset.
- `TestUAVBeginSkipsAvailableAndExplicit`: available or administrator-managed dependencies perform no initialization; failed activation cannot loop.
- `TestUAVOwnedProbeCancellation`: real copied Windows process fixture launches a descendant; cancellation waits for both actual process handles, preserves an independent sentinel, and permits zero-share opening and rename of the worker image after all fixture owners exit.
- `TestUAVOwnedProbeExitCodes`: real owned probe exit 0/2 handling, distinct from cleanup errors.

## Boundaries / remaining verification

- The process fixture verifies Job ownership and lifecycle; it is not evidence of real Python/UAV/FFmpeg readiness.
- Actual online install, real offline probe, profile reuse/offline restart, readonly program directory and integrated desktop environment regression belong to the parent task's `verify_uav_bootstrap.py` and existing desktop harnesses.
- First successful installation briefly stops and restarts the media service to activate the new worker. Downloads themselves start only after media is ready.
- Physical tray interaction, clean Windows machine compatibility, live site downloads, Linux and ARM64 are not claimed by these checks.
- Runtime implementer confirmed all installer error boundaries are sanitized Chinese messages; only standard context cancellation/deadline and the desktop's fixed diagnostic error are propagated. A reuse probe returning ErrCleanup must stop before any subsequent materialization (reported and fixed by runtime implementer).

## Follow-up: activation failure diagnosis

- Parent integration observed staging probe success and successful publication but an unavailable managed worker after activation. Static tracing excludes a stale stage path: `Initialize` validates and returns final-generation Paths after rename; `completeUAVInitialization` stores them; the automatic stop/start branch does not call the user retry reset; `startManagedCollection` applies the stored Paths with the same child-environment helper as the diagnostic.
- Changed the second-attempt status and Chinese warning log to preserve the worker's already-sanitized actual capability reason. The previous generic activation failure text obscured which local probe failed. It still never marks unavailable UAV as ready and does not trigger another automatic download.
- Temporary `TestUAVActivationKeepsActualReason` passed (`ok ... 0.647s`), asserting status/log preservation and no new initializer; the temporary source was removed.
- The follow-up reproduction below proves the final-location native DLL path-length failure. This diagnostic improvement preserves that actual worker reason but does not itself fix the installer layout; real integrated acceptance remains owned by the parent task.

### Root cause reproduced without another download

- A temporary desktop diagnostic test copied the already valid `.runtime/verify/windows-uav-local-repair/stage` bundle into a new owned `.runtime/tmp/` fixture matching the integrated profile depth. The same actual private Python, child environment, working directory and fixed imports were used before and after rename. The fixture was removed by its owning test.
- Stage: longest curl native DLL path **249 UTF-16 units**, `uavruntime.Load` passed, actual imports of UAV sites, requests/m3u8/cloudscraper/curl_cffi/AES returned `IMPORT_OK`.
- After changing only `stage-<32hex>` to `generation-<32hex>`: longest DLL path **254 UTF-16 units**, static `uavruntime.Load` still passed, but actual Python exited 1 with `ImportError: DLL load failed while importing _wrapper: 文件名或扩展名太长。` The stack was `curl_cffi.__init__` → `curl_cffi.__version__._resolve_curl_version` → `from ._wrapper import ffi, lib`.
- This demonstrates that the existing nominal `<260` final-file check alone is insufficient for this native loader and that a successful staging probe cannot prove final-location readiness. It excludes desktop path-reset/environment handoff as the cause in this reproduction.
- Command: `python -B scripts/project_env.py -- .tools/go/bin/go.exe test ./internal/desktop -run TestUAVRelocationImportDiagnosis -v -count=1`; diagnostic experiment completed in 15.076s. It deliberately logged the failing final import, so its test process PASS is not a claim of correct production behavior.
- Temporary `uav_relocation_temp_test.go` was removed after capturing evidence. Parent/runtime implementer were sent the trace and requested to shorten the immutable generation path and probe the **final location before publishing current.json**. Production fix and real end-to-end rerun remain their responsibility.

## Reusable decisions and sources

- Keep lifecycle mutation on `controller.run`; background callbacks update only an identity-checked status under `c.mu`. Sources: `internal/desktop/launcher.go`, `uav_initialization.go`.
- Preserve actual worker capability authority and one-ready-plugin semantics. Sources: `collection_controller.go`, `.trellis/spec/backend/uav-windows-contract.md`, `.trellis/spec/backend/jmcomic-windows-contract.md`.
- Keep managed process ownership through real process/handle release and treat cleanup as distinct from cancellation. Sources: `collection_process_windows.go`, `uav_initialization.go`, `.trellis/spec/backend/cross-platform-contract.md`.
