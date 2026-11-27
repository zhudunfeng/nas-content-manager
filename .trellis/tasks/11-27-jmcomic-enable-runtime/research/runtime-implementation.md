# Shared private runtime bootstrap implementation

- Date: 2026-11-27 (Asia/Shanghai).
- Scope: shared Windows x64 installer and independent UAV/JMComic wrappers. All commands run through `python -B scripts/project_env.py -- ...`; temporary state and tool caches stay in repository `.runtime/`.

## Actual delivery

- `internal/runtimebootstrap/` now contains the original UAV safe download, ZIP indexing/extraction, inventory closure, native path budget, owned generation cleanup and atomic-selection state machine. Safety fixes are shared instead of copying the installer for a second plugin.
- `runtimebootstrap.New(Config)` takes only compile-time fixed plugin configuration, defensively copies resource bytes and required-file lists, rejects invalid namespace/manifest metadata collisions, and caches each plugin plan independently. No browser/config input selects a source or command.
- `internal/uavbootstrap/bootstrap.go` retains the original public `Paths`, `Progress`, `ErrCleanup`, `Resolve` and `Initialize` interfaces. Its namespace, owner marker, embedded recipe/runtime/payload bytes, inventory and generation identity remain compatible. Tool paths still derive from the verified generation.
- `internal/jmcomicbootstrap/bootstrap.go` exposes the matching API with `Paths{Manifest string}`, namespace `jm-bootstrap`, and the unchanged `jmcomicruntime.Load` strict validator. It installs no yt-dlp, FFmpeg or UAV dependencies. The main agent generated its independent three assets.
- Both wrappers share the same `ErrCleanup` identity. Chinese progress/error labels are adapted without discarding cancellation or cleanup error chains. Public initialization always uses the production fixed network materializer. An unexported `initialize` method accepts the internal materializer to exercise the identical publication path with locked local artifacts during fault injection.

## Preserved contracts

- Standard TLS, fixed official source allowlist, finite retries/time/size budgets and narrow HTTPS release-CDN redirects.
- Exact artifact lengths/SHA256; ZIP path/device/ADS/case/directory conflicts, encryption, links and expansion budgets; full member CRC and digest while extracting selected files.
- Complete declared inventory and plugin-specific source/version/license/PE/isolated-Python validation.
- Check the final native DLL/PYD/EXE UTF-16 budget (248 units) before acquisition and during read-only resolution.
- Random `stage-<32hex>` then immutable `gen-<32hex>`; validate after relocation, execute the real final-location probe, then atomically publish bounded strict `current.json`. Failure/cancellation leaves an old current intact and only removes the exact candidate. Cleanup failure is never treated as a retryable network failure.

## Validation evidence

1. `python -B scripts/project_env.py -- go test -v ./internal/runtimebootstrap -run Temporary -count=1`: passed, package 52.511s. Log: `.runtime/runtimebootstrap-temporary-tests.log`.
   - Production publication path: first final-probe failure publishes nothing; success selects a generation; subsequent valid reuse makes no material request; failed replacement and final-probe cancellation preserve byte-identical old current; only the new candidate is removed; old generation still resolves; `ErrCleanup` blocks acquisition; nil probe refuses initialization.
   - Namespace ownership isolation, defensive byte copies, unsafe configuration, strict metadata duplicates/case/unknown/null fields, and preserved `errors.Is` cancellation/cleanup identity.
   - ZIP traversal/ADS/device/case/file-directory aliases; untrusted URLs/redirects; damaged, short and oversized file rejection; cancelled downloads; wrong response lengths; native-path overflow.
   - Actual locked JMComic inventory: 9 artifacts, 620 files. Restore from the original hash-named cache through a fake transport (no network), strict `jmcomicruntime.Load`, real `collectionworker.CheckJMComic` at the final generation, read-only resolve and zero-request reuse all passed.
   - Actual locked UAV inventory: 20 artifacts, 791 files. Restore from the original hash-named cache (no network), strict `uavruntime.Load`, real `collectionworker.CheckUAV` with the actual yt-dlp/FFmpeg files at the final generation, read-only resolve and zero-request reuse all passed.
2. Added explicit CRC corruption and final case-insensitive reserved-manifest checks, then ran `go test -v ./internal/runtimebootstrap -run 'TemporaryArchiveAndDownload|TemporaryIsolation' -count=1`: passed, package 2.908s. Log: `.runtime/runtimebootstrap-boundary-final-tests.log`.
3. `go vet ./internal/runtimebootstrap ./internal/uavbootstrap ./internal/jmcomicbootstrap`: passed.
4. `go test ./internal/runtimebootstrap ./internal/uavbootstrap ./internal/jmcomicbootstrap` initially passed compilation. After verification the newly generated `runtimebootstrap_temporary_test.go` was removed as required; an empty retained test suite establishes compilation only.
5. `gofmt` is not discoverable on the runner PATH; used the configured Go runtime directly via `python -B scripts/project_env.py -- .tools/go/bin/gofmt.exe ...`. No global environment was changed.

## Boundaries and next steps

- These cached-artifact tests do not establish first-run internet acquisition; main-task integration must provide that separately.
- Only JMComic/UAV local dependency probes ran; no live sites or site credentials were used. Clean ordinary-user Windows, Linux runtime, Docker and ARM are not claimed.
- Parent task owns desktop batching/cancellation, release build, UI/status integration, documentation/spec updates and final artifact acceptance. No commit or task archive was performed.

## Sources

- Original baseline: `.trellis/tasks/11-27-windows-uav-auto-init/research/final-validation.md`, `internal/uavbootstrap/assets/`.
- Contracts: `.trellis/spec/backend/uav-windows-contract.md`, `.trellis/spec/backend/jmcomic-windows-contract.md`, `.trellis/spec/backend/cross-platform-contract.md`.
- Actual shared implementation: `internal/runtimebootstrap/{bootstrap,config,plan,download,materialize,atomic_windows,atomic_other}.go`.
