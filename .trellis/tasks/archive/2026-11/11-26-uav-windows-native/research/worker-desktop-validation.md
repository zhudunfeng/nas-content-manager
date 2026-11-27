# Worker／desktop Windows verification

Date: 2026-11-26. Scope: worker configuration/local diagnostics and the existing
single desktop-managed worker with independent JMComic/UAV capabilities.
Production Go is stable; the final verification changes are limited to the
reusable `scripts/verify_desktop_collection.py` harness.

## Results and evidence

| Check | Result | Evidence |
| --- | --- | --- |
| Targeted Go compilation, vet and worker/desktop builds | Passed | `go test` / `go vet` for `./internal/collectionworker ./internal/desktop ./cmd/nas-collection-worker`; no retained unit tests, so test invocation is compilation evidence |
| Base desktop collection matrix | Passed, 10 checks | Real Windows copied application/worker; no optional runtime |
| JMComic-only matrix | Passed, 14 checks | `.runtime/verify/uav-windows-worker/desktop-jmcomic.json` |
| Original private UAV `--check-uav` | Passed, exit 0 | Actual CPython 3.14.4/UAV 3.8.4, official yt-dlp 2026.10.26 and local FFmpeg; approximately 8.95 seconds |
| Precise read-only program ACL | Passed | An actual write is rejected while the copied worker remains executable |
| Final standalone UAV nested fixture preflight | Passed | `.runtime/verify/uav-windows-worker/nested-uav-preflight.json`; actual marker, loaded curl/AES/CPython modules and exclusive access after stopping only the observed child |
| Full UAV-only matrix | Passed, 20 checks; 25 actual UAV mapped images | `.runtime/verify/uav-windows-worker/desktop-uav.json` |
| Full JMComic + UAV matrix | Passed, 24 checks; 26 actual JMComic and 25 actual UAV mapped images | `.runtime/verify/uav-windows-worker/desktop-combined.json` |

The standalone fixture is explicitly marked `desktopJobAcceptance: false` and
does not replace the complete desktop Job/lifecycle checks. All dependency
probes and fixtures are local; none contacts a real video site.

Both final full matrices exited 0 without additional production changes. They
cover actual fixed dependency probes, authenticated app/worker capabilities,
independent optional failures, precise read-only ACL, real IPC mode changes,
normal quit and parent termination, exact descendant process handles, unrelated
sentinel survival, exclusive image access/replacement and immediate same-profile
restart. The combined matrix additionally proves a corrupt JMComic manifest
leaves video ready and unavailable UAV dependencies leave JMComic ready.

## Production changes covered

- `internal/collectionworker/config.go`, `service.go`, `uav.go` and
  `cmd/nas-collection-worker/main.go`: explicit interpreter/manifest/default
  precedence, managed isolation, UTF-8 fixed bridge invocation, precise local
  unavailable reasons, exact UAV downloader version and shared `--check-uav`
  dependency probes/exit codes.
- `internal/desktop/collection_config.go`, `collection_managed.go`,
  `collection_controller.go`, `launcher.go` and `shell_windows.go`: fixed
  per-plugin descriptors, adjacent runtime defaults, controlled child tools and
  enablement, expected per-kind capability snapshots, partial availability and
  manga/video tray state while preserving authenticated ownership/lifecycle.
- `scripts/verify_desktop_collection.py`: retained integration scenarios,
  diagnostics, independent failure cases, actual read-only ACL and real loaded
  runtime lifecycle coverage. Its newly generated Go fixture sources are removed
  after compiling.

## Binary and source identity

The matrix uses `.runtime/verify/uav-windows-worker/nas-collection-worker.exe`
and `栖库.exe`, built from the actual repository with Go 1.27.4, Windows amd64,
CGO disabled, no `webembed` and the default `go build` options. These include VCS
revision `286691b8d6d6d18b902ebda000a815e1f33667b8` with `vcs.modified=true` and
do not set `-trimpath`.

Root's `.runtime/bin/uav-windows-final-20261126/` artifacts instead use
`-buildvcs=false -trimpath`, with the same target/toolchain/dependency versions
and without `webembed`. Thus their EXE hashes differ. All 154 production inputs
recorded in root's `build-inputs.json` were rehashed with no drift. A second build
with this matrix's original default options reproduced both tested binaries
byte for byte, establishing source identity rather than assuming equal source
from an equal VCS commit. Details are in
`.runtime/verify/uav-windows-worker/source-input-identity.json` and
`build-provenance.json`.

| Binary | Matrix SHA-256 | Root final SHA-256 |
| --- | --- | --- |
| worker | `a36412fcb7dc523a7f17343b46e62ad59fbae6bd01f2b5c93173b36d2ef3cba6` | `38e1c86976eddb7e7cc162a6e07459b57f9ceb84a88ee29e5b82835db8dc3ec4` |
| desktop | `055ff9c516e6fdc92f9f87235cf58f1233c36de51d7035acecbf310218d9606a` | `f1a2ddd3b1c390efc6eba7befe37ec75432ef0d2a2f9d20d17a92f840b590358` |

## Reproducible commands

All commands use the repository environment runner. The actual FFmpeg input is a
trusted local installation; the harness copies its sibling DLLs into its owned
program directory before running it.

```powershell
python -B scripts/project_env.py -- go test ./internal/collectionworker ./internal/desktop ./cmd/nas-collection-worker
python -B scripts/project_env.py -- go vet ./internal/collectionworker ./internal/desktop ./cmd/nas-collection-worker
python -B scripts/project_env.py -- go build -o .runtime/verify/uav-windows-worker/nas-collection-worker.exe ./cmd/nas-collection-worker
python -B scripts/project_env.py -- go build -o .runtime/verify/uav-windows-worker/栖库.exe ./cmd/nas-desktop
python -B scripts/project_env.py -- python -B scripts/verify_desktop_collection.py --binary .runtime/verify/uav-windows-worker/栖库.exe --worker .runtime/verify/uav-windows-worker/nas-collection-worker.exe --runtime-manifest .runtime/tools/jmcomic-private/jmcomic-runtime.json --web-dir web/dist --report .runtime/verify/uav-windows-worker/desktop-jmcomic.json
python -B scripts/project_env.py -- python -B scripts/verify_desktop_collection.py --binary .runtime/verify/uav-windows-worker/栖库.exe --worker .runtime/verify/uav-windows-worker/nas-collection-worker.exe --uav-runtime-manifest .runtime/tools/uav-private/uav-runtime.json --ytdlp .runtime/tools/uav-windows/yt-dlp.exe --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.EXE --web-dir web/dist --report .runtime/verify/uav-windows-worker/desktop-uav.json
python -B scripts/project_env.py -- python -B scripts/verify_desktop_collection.py --binary .runtime/verify/uav-windows-worker/栖库.exe --worker .runtime/verify/uav-windows-worker/nas-collection-worker.exe --runtime-manifest .runtime/tools/jmcomic-private/jmcomic-runtime.json --uav-runtime-manifest .runtime/tools/uav-private/uav-runtime.json --ytdlp .runtime/tools/uav-windows/yt-dlp.exe --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.EXE --web-dir web/dist --report .runtime/verify/uav-windows-worker/desktop-combined.json
```

## Bug Analysis: verification preconditions

### 1. Root Cause Category

- **Category E — Implicit Assumption:** the older JMComic-only harness timing and
  dependency assumptions were carried into a package that first probes actual
  downloader/FFmpeg executables, then enters its private Python bridge.
- **Category D — Test Coverage Gap:** the newly expanded lifecycle fixture had
  not been executed independently before the long complete matrix.
- These failures were in the verification harness, not demonstrated production
  runtime failures. Actual fixed UAV bridge probes already passed before the
  final standalone fixture was corrected.

### 2. Why previous attempts failed

1. The inherited IPC helper had a 20-second subprocess budget. Local diagnostics
   independently permit two 10-second tool probes plus a 15-second bridge probe.
   The diagnostic helper now has its own bounded 60-second budget; product probe
   timeouts are unchanged.
2. `icacls` generic `(W)` includes `SYNCHRONIZE`, which prevented CreateProcess
   during the supposedly read-only-program test. The fixture now denies only
   `(OI)(CI)(WD,AD,WEA,WA)` and proves an actual write fails before proceeding.
   It restores the deny ACL in `finally`, only on the owned isolated copy.
3. The original nested marker waited eight seconds, inherited from the JM-only
   package without real bundled tools. The video package runs actual tool
   version probes first. The marker deadline is now 30 seconds, matching the
   desktop startup budget; no production startup/cleanup timeout was increased.
4. The UAV nested fixture imported `lxml.etree`, which is absent from the fixed
   headless UAV dependency closure. Independent execution produced
   `ModuleNotFoundError: lxml`. The import was removed; no dependency was
   installed or runtime/source check relaxed. The final fixture imports actual
   UAV/ssl plus curl and AES, creates its marker, and holds the loaded modules.

### 3. Prevention Mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Independent fixture preflight | Run the final actual private Python/Go fixture in the controlled environment and require the marker plus native mapped modules before a long matrix | Done; JSON evidence retained |
| P0 | Explicit timing contracts | Keep local diagnostic, IPC, product startup and active-child marker budgets distinct | Done in harness; production budgets preserved |
| P0 | Positive ACL proof | Verify write rejection and continued execution, then restore only the owned directory ACL | Done |
| P1 | Executable dependency identity | Import only fixed plugin runtime dependencies; never add a package solely to satisfy a fixture | Done |
| P1 | Persistent specification | Record the above harness prerequisites in the Windows UAV contract | Done by root to avoid concurrent writes |

### 4. Systematic Expansion

- Both runtime fixture branches continue to use their actual distinct dependency
  closures: Pillow for JMComic, no Pillow/lxml requirement for UAV, and real
  curl/AES/CPython extensions for both.
- Child-only environment pollution tests still clear Python home/path and tool
  configuration. Full matrices retain exact process handles, unrelated sentinel
  survival, copied image replacement and same-profile restart assertions.
- No global process-name termination, arbitrary cleanup sleeps, system dependency
  installation or production failure suppression was added.

### 5. Knowledge Capture

- This note and the reusable integration harness preserve the root causes and
  executable preconditions. Root has updated the domain spec prerequisites.
- No template mirror of this project's domain contract was found under `src/`
  or `packages/`; no Trellis scripts were changed by this work.

## Additional observation and acceptance limits

One initial base run observed Win32 sharing error 32 when opening the copied
worker image with zero sharing immediately after an exact forced-kill process
handle signaled. A read-only process snapshot showed no surviving worker. The
next complete base run and JMComic-only run passed without a production change.
This remains an observed transient file-release timing risk; no process-name
kill or delay was added to hide it.

Final harness AST parsing and focused `git diff --check` passed. No generated
unit-test sources remain under `internal/` or `cmd/`; successful harness-owned
temporary directories/Go fixture sources have been removed.

These are development-machine Windows checks, run under the existing local
user. They do not establish clean ordinary-user VM, Windows 10,
Windows ARM64, Linux/Docker runtime, real website search/download, UNC/long-path
profile or human distribution/license acceptance. Parent owns the broader
release, offline media/bridge regression and whole-repository gates. Temporary
Go fixture sources are removed by their builders after compilation; only the
reusable Python harness and sanitized reports are retained.
