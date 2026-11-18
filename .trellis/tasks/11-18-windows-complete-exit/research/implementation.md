# Windows complete-exit implementation and focused validation

## Ownership

This implementation covers `internal/app/` and `internal/desktop/`. The coordinating agent owns the reusable desktop verification script, documentation/specification changes and final packaged-application checks. No Docker tests, user media, existing credentials, unrelated processes or legacy C-drive caches were touched.

## Root causes and changes

1. The old HTTP shutdown waited up to 10 seconds for a slow large-file transfer and then closed connections, but neither `Shutdown` nor `Close` waited for handler defers. Request contexts now inherit the application context. A request-admission mutex and wait group stop new business requests before cancellation, close this instance's active connections, and wait for admitted handlers and workers before database/lease closure. The response writer is not wrapped, preserving streaming interfaces and 64-bit range delivery.
2. `exec.CommandContext` killed only the direct media tool. A descendant could retain inherited output pipes, an executable mapping and open files after the parent exited. Windows media tools now start suspended, are assigned to a private Job before their first instruction, and resume only after successful assignment. The Job retains `KILL_ON_JOB_CLOSE`. Both cancellation and normal command completion terminate owned descendants and wait for captured process handles plus empty Job accounting. A two-second `WaitDelay` bounds orphan-held output pipes. Process-tree cleanup failures are retained by the application and returned during shutdown.
3. Merely seeing `ActiveProcesses == 0` was insufficient in an early fixture run: a captured descendant handle had not yet signalled. The implementation therefore keeps `SYNCHRONIZE` process handles before termination and waits for their signals. Before capturing handles it sets `JOB_OBJECT_LIMIT_ACTIVE_PROCESS` with limit zero, retaining `KILL_ON_JOB_CLOSE`, to close admission of newly spawned descendants during the snapshot/termination interval. Failure to establish that barrier is reported while termination is still attempted. There is no process-name termination, browser Job membership or cross-profile cleanup.
4. An independent, short-lived control EXE consistently timed out while the server was waiting for `ConnectNamedPipe` completion; the client had already successfully opened and written to the pipe. Temporary traces located the stall before request reading. Go escape analysis showed the connection/read/write `OVERLAPPED` values and message buffers did not escape, allowing kernel-held asynchronous addresses to refer to a movable goroutine stack. `runtime.Pinner` now pins each `OVERLAPPED` and each nonempty read/write buffer until completion or `CancelIoEx` followed by blocking `GetOverlappedResult` reclamation. Initial byte-count pointers are nil and final transfer counts always come from `GetOverlappedResult`. Post-fix escape analysis confirms heap placement. Temporary production traces were removed.
5. `errors.Is(err, context.Canceled)` also matched joined cancellation plus cleanup failures. The launcher now ignores cancellation only when every unwrapped leaf is `context.Canceled`. Startup, readiness, timeout, restart and service-result paths preserve cleanup errors, including a result racing launcher cancellation. A nil service result racing startup cancellation remains a normal cancellation.
6. The tray command dispatch was extracted without changing its menu actions. Tray exit and confirmed IPC quit still use the same launcher cancellation path. The instance guard, IPC owner and log owner remain tied to launcher return.

## Changed production files

- `internal/app/request_lifecycle.go`: request admission/drain barrier.
- `internal/app/server.go`: lifecycle-bound request contexts, active-connection closure, handler/worker drain and resource-close error propagation.
- `internal/app/media_tools.go`: private media-process wrapper, bounded inherited-pipe wait, cleanup-error retention/logging.
- `internal/app/media_process_windows.go`: suspended launch, Job ownership, sealed process admission, process-tree termination and handle waits.
- `internal/app/media_process_other.go`: standard non-Windows command execution adapter.
- `internal/desktop/ipc_windows.go`: pinned overlapped IO and accurate final counts/errors.
- `internal/desktop/launcher.go`: cancellation-leaf classification and complete cleanup-error propagation.
- `internal/desktop/shell_windows.go`: reusable product tray dispatch.

## Focused temporary behavioral tests

All commands used `python -B scripts/project_env.py -- ...`; generated samples, tool images, caches and build products stayed inside `.runtime/`. The following tests executed before their source files were deleted:

- `TestRequestShutdownBarrier`: a real listener/request with deliberately delayed file-closing defers proved the drain barrier did not report completion early; shutdown rejects new requests with 503, and the held file can be renamed afterward.
- `TestRequestShutdownAdmissionRace`: 100 rounds of 20 concurrent requests competing with shutdown in the final suite.
- `TestWindowsMediaTreeCancellation`: parent-running and parent-exited fixtures each create a descendant holding a zero-share source-file handle and inherited stdout. The original implementation was still blocked after four seconds in both baseline cases. After the change, captured parent/child process handles signal, the file becomes exclusively accessible and a dedicated copied tool EXE can be exclusively opened, renamed and overwritten immediately when `Run` returns. The final suite observed approximately 5 ms for a live parent and 2.002 seconds for the orphan-pipe `WaitDelay` case.
- `TestWindowsMediaStartupBoundary`: one ordinary command, one pre-cancelled command and 29 startup/cancellation races per invocation; no cleanup error or leftover owned process.
- `TestWindowsMediaKeepsIndependentProcess`: an independent sentinel retains its own exclusive file while both media-tree scenarios finish; the application does not kill that unrelated process.
- `TestWindowsMediaConcurrentFork`: six scenarios per invocation continuously spawn children while cancellation seals Job admission. Three focused invocations (18 scenarios) passed before the final suite; the final suite ran another six successfully. Each checks published PIDs and immediate exclusive open/rename/overwrite of the copied executable image. This also exercises lowering the Job active-process limit to zero while multiple processes already exist.
- `TestWindowsControlRepeated`: three groups of 100 status requests plus quit in each final-suite invocation; three earlier fixed-code invocations also passed.
- `TestWindowsControlShortLivedProcesses`: a separate server EXE and 20 separate status/open EXEs followed by a quit EXE. The unpinned version consistently failed at the first client. The pinned version passed three focused repetitions, then three repetitions with forced GC and idle reuse, and the final suite. The stress version forces GC every 5 ms on the server and before client requests/inside the handler.
- `TestWindowsControlCancellationBoundaries`: cancel while waiting for connection and while a connected client sends no message; each cancels within one second and immediately recreates the same pipe.
- `TestWindowsCleanupErrors`: eight cases covering nil, direct/wrapped/joined cancellation, direct/mixed/nested cleanup errors and deadline failure; mixed failures remain observable through `stop`.
- `TestWindowsLaunchAlreadyCanceled`: ten real pre-cancelled startup attempts per invocation create no SQLite database and return normally through the controller; cancelled readiness checks remain cancellation errors. Three focused repetitions and the final suite passed.
- `TestWindowsTrayExitMessagePump`: the real locked-OS-thread native tray message pump exits through the product exit dispatch, including repeated dispatch. This is native message-pump coverage, not a claim of a physical mouse click.

### Fixture correction during the full check

The first all-package test run overlapped first-time Linux/race toolchain compilation. Its temporary fixtures exposed two test-harness defects: a newly created PID file could be read before `WriteFile` populated it, and a fixture could exceed its eight-second startup budget. There were no failed post-exit process/file-release assertions in that run. The fixture publisher was changed to same-directory atomic rename, helper Go process parallelism was limited, and only fixture readiness budgets were increased to 20 seconds. The actual four/five-second exit assertions were unchanged. A complete serialized package run then passed. The failed run is preserved at `.runtime/windows-exit/go-test-first-fixture-failure.log`; it must not be reported as a passing run.

## Final commands and results

| Command after the project-env prefix | Result / evidence |
| --- | --- |
| `.tools/go/bin/gofmt.exe -w` on the eight changed production Go files | Passed; final `gofmt -l` checked separately. |
| `go vet ./...` | Passed; `.runtime/windows-exit/go-vet.log`. |
| `go test ./... -p 1 -count=1 -timeout 180s -v` | Passed with temporary behavioral tests present; app 13.670 s, desktop 2.934 s; `.runtime/windows-exit/go-test.log`. |
| `go build -o .runtime/windows-exit/checked-nas.exe ./cmd/nas` | Passed; `.runtime/windows-exit/go-build.log`. The coordinating agent reuses this exact stable CLI for service regressions. |
| `go build -o .runtime/windows-exit/linux-amd64-nas ./cmd/nas` with process-only `GOOS=linux GOARCH=amd64 CGO_ENABLED=0` | Passed; `.runtime/windows-exit/go-build-linux-amd64.log`. |
| `go build -o .runtime/windows-exit/linux-arm64-nas ./cmd/nas` with process-only `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` | Passed; `.runtime/windows-exit/go-build-linux-arm64.log`. |
| `go build -gcflags=nas-content-manager/internal/desktop=-m=2 ./internal/desktop` | Passed; relevant post-fix escape output saved in `.runtime/windows-exit/ipc-escape-fixed.log`. |
| `go test -race ./internal/app ./internal/desktop ...` | Not runnable: cgo disabled; `.runtime/windows-exit/go-race.log`. Explicitly enabling cgo confirmed missing `gcc`, `.runtime/windows-exit/go-race-cgo.log`. No compiler was installed and no race-detector success is claimed. |
| `go test ./... -p 1 -count=1 -timeout 120s` after temporary-source deletion | Passed; `.runtime/windows-exit/go-test-after-cleanup.log`. Packages have no remaining test files, so this is compilation verification only, not behavioral evidence. |

The coordinating agent separately performs the final GUI-subsystem application build and reusable desktop/five-media/engine protocol checks; see its verification record for those exact commands and outcomes. Linux binaries were cross-built, not executed on Linux in this subtask.

## Temporary source cleanup

These newly generated files were executed, then removed as required:

- `internal/app/request_lifecycle_temp_test.go`
- `internal/app/windows_exit_temp_test.go`
- `internal/desktop/windows_exit_temp_test.go`

No production TRACE output remains. Test cleanup targets exact captured processes/owned Job objects only; unrelated browsers, another application mode and Docker resources are preserved. Changes are uncommitted.
