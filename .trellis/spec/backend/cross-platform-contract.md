# Cross-platform launcher and release contract

## 1. Scope / Trigger

Applies to `cmd/nas-desktop`, `internal/desktop`, application lifecycle/static assets/tool discovery, `compose.desktop.yaml`, release/maintenance scripts and changes to Linux/Windows platform boundaries. One media/download/search implementation remains authoritative. Do not introduce Windows GUI dependencies into the Linux CLI or silently migrate between native and container data.

## 2. Signatures

- `app.Run(ctx context.Context, config Config, logger *slog.Logger) error` remains the compatible CLI wrapper.
- `app.RunWithOptions(ctx context.Context, config Config, logger *slog.Logger, options RunOptions) error`; `RunOptions.Listener net.Listener` transfers ownership, including startup failures; `Ready func(address string)` fires once after this instance is actually reachable.
- `app.LoadConfigWithLookup(lookup func(string) string) (Config, error)` validates the final merged configuration without changing global environment. `LoadConfig` delegates to `os.Getenv`.
- CLI-only offline reset: `nas account reset-password --data-dir <existing-local-directory> [--password-stdin]`. It runs before ordinary configuration/network initialization; use the service CLI, not desktop launcher flags. The [account lifecycle contract](./account-contract.md) defines exact parsing, schema, exits and acceptance status.
- `webassets.Files() fs.FS` returns embedded `dist` under `webembed`, nil in ordinary development builds.
- Windows entry: `栖库.exe --mode native|docker --profile <local-root>`; native diagnostic flags include `--headless`, `--no-browser`, `--media-root` (repeatable), `--data-dir`, `--port`, `--print-config`, and `--control open|status|quit`.
- `python scripts/release.py --target windows-amd64|linux-amd64|linux-arm64 --tools-manifest <file>` produces a package; `--development-without-tools` explicitly selects an incomplete development package.
- Windows cache maintenance: `scripts/clean-session-temp.ps1 [-ManifestPath <repo-file>] [-IncludeGoBuildCache] [-IncludeGoModuleCache] [-BrowserProfileName <exact-name>] [-Since <date>]` previews; `-Execute [-ManifestPath <same-file>]` consumes that saved plan without adding selectors.

## 3. Contracts

### Shared runtime

- Acquire canonical data-directory OS lease before database open/migration, job recovery or background workers. Linux uses flock; Windows uses a native file lock. Hold through database and worker shutdown; do not unlink a held lock file. Two different ports do not make shared SQLite writers safe.
- Offline maintenance uses this same canonical lease directly after existing-directory/database checks, holds it through database close, and refuses active owners or unsupported platforms. Do not create a data directory, force a lock or stop another instance to reset a password. Terminal ownership is separate: establish raw mode synchronously, restore exactly once before mutation and retain cleanup failures even when cancellation is also present. Windows real Console keys/system events, Linux terminal/flock execution and cross-compilation are separate evidence categories.
- Preserve the listener actually bound by the launcher; do not close/rebind between probing a port and service startup. Failure to bind must not migrate/recover the database. On all failures close the transferred listener.
- Ready requires a database-backed `/healthz` probe carrying this instance's random token plus available static HTML; never accept an unrelated service's generic HTTP 200. Caller callbacks cannot replace actual startup checks.
- Shared shutdown stops request admission, cancels the application/request contexts, closes active HTTP connections and waits for admitted handlers plus background workers before closing SQLite and releasing the data lease. `Server.Serve` returning or `Server.Close` completing does not prove handlers have released their files. Prevent handler admission from racing a zero-count wait. Preserve 64-bit streaming, no global short WriteTimeout, source read-only and download publication gates.
- Windows media tools must belong to an application-owned process tree before their first instruction executes. Cancellation and normal parent completion reclaim that tree and inherited pipes before reporting completion; a bounded `WaitDelay` alone does not release orphan-owned files. Before collecting process handles for shutdown, seal new process admission with an active-process limit of zero while retaining kill-on-close, then terminate the Job and wait for the collected process handles as well as an empty Job. A zero active-process count alone is not proof that executable images have been released. Never kill processes by executable name, attach a user's browser, or claim another profile's processes. Keep process/job handles owned and close them on start/assignment/resume failures as well as cancellation.
- `NAS_WEB_DIR` explicitly set takes precedence over embed. Ordinary builds retain `web/dist`. Embedded and disk static routes preserve MIME, HEAD, SPA fallback and asset-not-found semantics. Missing assets must not return SPA HTML for JS requests.
- `NAS_FFMPEG_PATH` / `NAS_FFPROBE_PATH` must be absolute when present. Otherwise use executable-adjacent `tools`, then safe PATH. Resolve once consistently for execution and settings. Reject accidental working-directory tool loading; keep Windows process-window details in platform files.

### Desktop profile and control

- Default profile is `%LOCALAPPDATA%/Qiku/profiles/native` or `docker`; the business data root is separate. Precedence: explicit CLI > applicable NAS environment > saved JSON > defaults. Original CLI does not read desktop JSON.
- Config schema v1 fields: `schemaVersion`, stable random 32-hex `instanceId`, `mode`, `dataDir`, array `mediaRoots`, `port`, `autoOpen`, `dockerContext`, `imageManifest`. Every field is documented in Chinese. Saved files require all nine exact canonical keys, none null; decode into a fresh value rather than overlaying defaults. Stored data/media paths must be absolute, `dataDir` nonempty and `mediaRoots` nonempty. Only an absent file receives defaults; explicitly supplied CLI/environment relative paths may be normalized before saving. Bounded strict JSON, atomic same-directory replacement and actionable failure; do not reset malformed config into a new library or resolve an empty saved data path to the caller's CWD.
- Default `127.0.0.1:8090`, at most 20 actual bind candidates; preserve the chosen port. LAN access uses explicit CLI configuration. Local IPC is current-user-only and only accepts fixed actions, never commands or arbitrary URLs. Browser opens after actual readiness.
- Native supports at most 32 explicit roots; Docker Desktop first release supports one Windows read-only media bind and rejects additional roots. Changing data directory means an explicit independent library selection, not automatic migration. Offline saved roots remain configured.
- Closing a browser does not stop native service. Docker launcher exit and whole-stack stop are separate actions. Stop first uses helper's owned-engine stop protocol, then Compose stop; never claim Compose down alone stops dynamic engines. When the same profile's launcher is running, `--docker-action status` uses its fixed status IPC; `--docker-action stop` must explicitly refuse with instructions to use the tray's complete-stop action or first exit the launcher. It must not reinterpret stop as opening the page.
- Native tray exit and confirmed `--control quit` share the launcher cancellation path. Keep the single-instance guard, IPC and log owner until the service has finished cleanup, then return from the launcher so the executable image can be replaced. Repeated exit is safe, startup cancellation cannot leave the service running, and a restart must wait for the old instance to release its data.
- Desktop branding uses `assets/windows/qiku.ico` (16/20/24/32/40/48/64/96/128/256 px) and checked-in `cmd/nas-desktop/resource_windows_amd64.syso`. Plain package builds and release builds must embed the resource without extra tool installation; Linux builds must ignore the Windows/architecture-scoped object. Regenerate explicitly with `scripts/generate_windows_icon.py` (fixed rsrc v0.10.2), and use `--check` for byte equality. The icon-only resource group is ID 1; adding a manifest to rsrc changes automatic ID allocation and requires coordinated loader/verifier changes. Run by package path, not an individual `main_windows.go` file.
- `loadApplicationIcon(instance uintptr, small bool) (uintptr, error)` loads owned large/small icons with `LoadImageW` and system metrics; do not use `LR_SHARED` for both sizes of the same resource because its cache ignores requested size. Bind large and small class icons separately, use the small icon for the tray, and destroy both only after tray removal, window destruction and class unregistration. Partial failures must release any successful icon handle. Missing resources return a Chinese error, never silently fall back to the generic system icon.
- Preserve resource-cleanup errors through startup, stop and service-completion races. Only normalize an error to ordinary cancellation when every wrapped/joined leaf is `context.Canceled`; `errors.Is(err, context.Canceled)` alone must not suppress a joined database, lease or process-tree cleanup failure. Report failure rather than announcing safe completion when cleanup failed.
- Windows overlapped IPC retains Go memory beyond the syscall. Pin both `OVERLAPPED` and each read/write backing buffer with `runtime.Pinner` until the IO completes; on cancellation, `CancelIoEx` must be followed by completion collection before unpinning or closing its event. `runtime.KeepAlive` alone is not an address-stability guarantee. Obtain the final byte count through `GetOverlappedResult` for immediate and pending completion, and cover separate short-lived client processes rather than only in-process loops.

### Desktop Docker and release

- Only selected local Desktop Linux context, no TCP/SSH daemon. First-release Desktop daemon and app/helper image platform must be Linux amd64; inspect actual image OS/architecture/content identity and reject implicit emulation. Resolve Docker CLI to an absolute path before changing process directory. Clear inherited DOCKER/COMPOSE redirection before explicit context/project/env-file arguments. Never start/install/reconfigure Docker or WSL as a side effect of checks.
- `release-images.json`: schemaVersion1 requires `appImage`/`helperImage`; schemaVersion2 additionally requires `collectionImage` and the packaged `compose.desktop.collection.yaml` overlay. SchemaVersion3 enables fixed resource-component deployment and permits an optional worker image. Every reference and resolved image ID must be distinct and immutable repository digests or exact local sha256 IDs. v1 rejects collectionImage. Optional `archive` under `images/` and `archiveSha256` appear together. Verify SHA-256 before load; absent manifest gives a real prerequisite error, never a fabricated public image. A worker adds exactly one private service and two instance-owned local volumes; no extra worker host mounts, privileges or exposed control ports. Full stop includes worker, keeping all data. New packages use v3; v1/v2 retain their original contracts.
- Compose env: `NAS_APP_IMAGE`, `NAS_HELPER_IMAGE`, identical `NAS_DESKTOP_PREFIX` and `ENGINE_VOLUME_PREFIX`, `ENGINE_NETWORK=<prefix>-engines`, `MEDIA_PATH`, `WEB_PORT`, `WEB_BIND_IP=127.0.0.1`, `QBT_PEER_PORT=6881`, `JACKETT_SETUP_PORT=19117`. All source config keys carry Chinese explanations.
- Before pull/load/create/start/stop, strictly validate the final normalized Compose JSON: v1 has exactly app/helper and seven role volumes; v2 additionally requires the fixed collection worker and its two volumes. v3 adds `collection-media`, app read-only/helper read-write mounts, the enabled helper environment and only the library subtree in media roots; its worker is optional. Append `compose.desktop.components.yaml` after the worker overlay and derive its media-root interpolation value from the trusted service environment. Require immutable distinct selected images, fixed environment/labels/security/logging/commands, exact read/write mount sets, one loopback app port and one instance bridge. Reject unknown fields, privilege/entry overrides, extra networks/mounts/ports, foreign names/labels, external or driver-option volumes and custom IPAM. Revalidate after selecting a fallback port; checking the source YAML or initial port alone is insufficient. See [component deployment](./collection-component-contract.md).
- Container ownership requires actual Compose project/service plus deployment/role labels, expected images and mounts. Also compare actual container environment against image defaults plus exact fixed overrides, image user, and no-new-privileges; reject extra/duplicate environment keys. Do not stop/reuse a foreign same-name resource. Default BT data is in Linux Docker volumes, not an NTFS download folder.
- Whole-stack stop allows 255 seconds for helper `stop-all`, exceeding its 240-second internal polling budget for up to six managed components. Continue to exact Compose app/helper stop only after successful owned-component completion; helper failure/unavailability/timeout must not claim full stop or discard data volumes.
- Full packages require fixed tool version/source, per-file hashes, matching PE/ELF architecture, actual applicable licenses and corresponding source/build materials. Shared tool builds need matching DLL/libraries. Metadata checks are not a substitute for license review or clean-machine execution.
- Source and packaged tool manifests share strict field/group validation. Required `tools/ffmpeg[.exe]` and `tools/ffprobe[.exe]` names are exact lowercase; case aliases cannot satisfy architecture checks. The generated `licenses/ffmpeg/tools-manifest.json` destination is reserved and cannot be supplied as input material. Package paths reject control characters and Windows device/trailing-dot/space aliases on every target.
- Archive verification rejects duplicate/non-object JSON and mutable image references, checks every runtime/license/source record against actual member hashes, and inspects the packaged PE/ELF architecture without executing or extracting tools. A recomputed outer checksum alone does not bypass package structure validation.
- Development package names and `release.json.developmentWithoutTools=true` clearly state missing tools. Build matrix covers Windows amd64 and Linux amd64/arm64; cross-build and actual execution are reported separately. No automatic publishing/signing/updating.
- Release-tool CLI entry points (`release.py`, `verify_release.py`, `package_desktop_images.py`) configure both standard streams as UTF-8 before argument parsing, including Chinese help and failure diagnostics. Share the initializer without reconfiguring the importing process at module import time; tolerate replacement streams without `reconfigure`. CI sets documented `PYTHONUTF8=1` and `PYTHONIOENCODING=utf-8`; scripts must also work when directly invoked with legacy redirected stream encodings. Changing the Windows code page or removing Chinese text is not the fix.
- The build workflow uses Node 24 action runtimes (`checkout`, `setup-go`, `setup-node`, `setup-python`, `upload-artifact` v6, verified from their official `action.yml`). The JavaScript runtime executing an action is separate from the pinned `node-version` used to build the application; do not change application toolchain versions just to suppress an action-runtime deprecation warning.

### Development cache maintenance

#### Project-local build environment

- `scripts/project-env.json` has one documented setting: `runtimeDirectory: {value: ".runtime", description: "<Chinese explanation>"}`. Accept `.runtime` or a descendant only; reject absolute paths, traversal, repository root, links/reparse points and unwritable destinations. Resolve from the script's repository root, not the caller's CWD. Never fall back to the system temp directory after configuration failure.
- `scripts/project_env.py:configure_environment() -> dict[str, str]` explicitly initializes only the current process and its descendants. It must have no environment-changing import side effect. The CLI `python -B scripts/project_env.py -- <argv...>` runs from the repository root, preserves argument boundaries and child status, and uses no command-string shell interpolation. `--print-env` emits JSON only to stdout; `--shell sh` quotes every exported value.
- `scripts/project-env.ps1` is dot-sourceable on Windows PowerShell 5.1/7, retains UTF-8 BOM, imports the same JSON environment without changing the current directory, and throws rather than exiting the interactive shell on failure. Resolve Windows npm/npx via Node and the corresponding CLI JavaScript, not the implicit batch shell; reject other batch wrappers explicitly. Release subprocesses reuse the same command resolver. The Linux release shell calls the automatically initialized Python release entry directly to retain caller-relative manifest/output semantics.
- All checked-in `scripts/*.ps1` containing Chinese comments or messages must retain UTF-8 BOM (`EF BB BF`); `.editorconfig` sets `charset = utf-8-bom` for `*.ps1` only. Windows PowerShell 5.1 interprets BOM-less scripts using the legacy ANSI code page, which can produce unexpected-token or unterminated-string errors before any command runs. PowerShell 7 parsing alone does not validate 5.1 compatibility. Preserve all bytes after the BOM for encoding-only fixes; changing console encoding after startup cannot repair source parsing.
- `dev.ps1` invokes npm through `python -B scripts/project_env.py -- npm --prefix web run dev|build`, reusing `resolve_command` to execute Node and the npm CLI directly. Keep the development host in `web/package.json` (`vite --host 127.0.0.1`) without appending a second host argument. Bare `npm` can resolve to `npm.ps1`; PowerShell can consume its `--` separator, causing npm/Vite to interpret the remaining host address as a positional project root and serve HTTP 404 despite reporting ready. Do not change global npm files or execution policy to repair project dispatch.
- Default relative paths: `tmp/` for TEMP/TMP/TMPDIR, `tmp/go/` for GOTMPDIR, `go/build-cache/` for GOCACHE, `go/` for GOPATH, `go/pkg/mod/` for GOMODCACHE, `go/bin/` for GOBIN, `npm/cache/`, `npm/logs/`, `node/compile-cache/`, `cache/` for XDG cache, and `playwright/browsers/`, `playwright/profile/`, `playwright/output/`. Windows emits only lowercase npm cache/log keys, because environment keys are case-insensitive and PowerShell 5.1/7 JSON parsing rejects case-only duplicates; POSIX overrides both variants. PowerShell JSON conversion must stop on errors. Use `GOENV=off` and disable Python bytecode in environment and the current interpreter; reset an already-cached `tempfile.tempdir` to the project temp directory.
- Python child text encoding uses PYTHONUTF8=1 and PYTHONIOENCODING=utf-8; PYTHONPYCACHEPREFIX points to `python/pycache/` as a fallback if a child explicitly enables bytecode, and PIP_CACHE_DIR to `python/pip-cache/`. Default verify sample roots and the release lock/staging root follow runtimeDirectory even when invoked from another working directory. Explicit output/root arguments preserve their prior semantics.
- All project dev/release/verification entry points initialize this environment. Direct Python release entry calls remain supported, local sibling imports must not generate early `__pycache__`, and library imports must not reconfigure the importing process. Existing `bin/`, `web/dist/` and embedded staging layouts remain unchanged. CI Go checks use the runner; setup-go's shared project cache restore is disabled.
- Preserve unrelated user settings such as process GOPROXY/GOPRIVATE. Explain that `GOENV=off` ignores persisted `go env -w` settings. Do not rewrite user/system environment, APPDATA/HOME, registry, Docker configuration or global telemetry; do not use Go's internal TEST_TELEMETRY_DIR hook. Go telemetry, OS/tool installation, explicit external output paths, existing IDE/MCP processes and released application business data are outside the containment promise.
- Browser MCP service environment must be supplied before startup. PLAYWRIGHT_MCP_USER_DATA_DIR/OUTPUT_DIR apply only to supporting services, explicit arguments may override them, and one persistent profile cannot be shared concurrently. PLAYWRIGHT_BROWSERS_PATH does not move installed Chrome/Edge. Never silently delete or migrate prior C-drive caches.

#### Legacy Windows cache cleanup

- Support Windows PowerShell 5.1 and PowerShell 7. Preview is the default and writes a versioned exact-file manifest under the repository; execution is explicit and never silently rescans to enlarge that plan. Keep native/CLI/media data outside the allowlist.
- Default candidates are Node compile-cache files containing the repository's absolute path with a path boundary after the repository name; a sibling such as `<repo>-old` must not match. Go build and module/checksum caches are shared, rebuildable resources and require separate explicit switches. A browser target requires an exact `mcp-chrome-<hex>` profile name; never choose the newest profile or the whole browser-cache parent.
- Accept only known current-user C-drive cache roots. Validate canonical paths, every ancestor, and the manifest's repository/target identities; reject relative paths, unknown kinds, root-directory targets and reparse points. `Since` filters creation time; it does not establish ownership by itself.
- Check relevant build/test-browser processes before execution; do not terminate them. Validate every file against its recorded length and creation/write metadata before deletion. Keep new or changed files, treat already absent files idempotently, and remove only recorded directories that are empty. Never use a recursive delete on a user/drive/Temp root.
- Persist the actual per-file outcome and confirmed deleted-byte count; a preview total is not recovered space. Locked/failed files require a partial-failure result, not a success claim. Do not raise privileges, change execution policy, or use another API to bypass a rejected cleanup action.
- Publish manifests atomically and exercise repeated writes on both PowerShell versions. For `File.Replace`, pass `[System.Management.Automation.Language.NullString]::Value` for an absent backup path; PowerShell can otherwise bind `$null` as an invalid empty string. Normalize JSON ISO timestamps because PowerShell 7 can deserialize them as `DateTime` while 5.1 retains strings.

## 4. Validation & Error Matrix

| Condition | Expected behavior |
| --- | --- |
| Canonical data directory already locked | Refuse before DB/recovery; do not alter data or terminate owner |
| Requested port occupied | Bind next bounded candidate; do not probe/open foreign app |
| All candidates unavailable | Chinese actionable error, no false readiness |
| Bad config, wrong mode/schema, missing/null saved field, empty/relative saved path, unsafe directory | Preserve config/data, fail explicitly |
| Missing embedded HTML or explicit web directory | Ready callback not called; no browser opened |
| Relative explicit FFmpeg/probe path | Configuration error |
| Native exit with a blocked large-file client | Interrupt the owned connection and drain its handler; do not wait for the entire file to transfer |
| Media command cancelled or parent exits while descendants retain pipes/files | Reclaim only its owned tree, wait for exit and close handles; preserve independent processes |
| Immediate restart or program replacement after native exit | HTTP/IPC gone; database, lock, logs, media and EXE permit exclusive access/rename |
| OS retains pointers for an asynchronous named-pipe operation | Buffer and OVERLAPPED addresses remain pinned through actual completion/cancellation collection |
| Desktop icon group is missing or cannot be loaded | Chinese startup error; release any partially created icon and stop through normal lifecycle cleanup |
| Offline saved media source | Keep authorization/index, failed scan cannot mark all missing |
| Remote Docker context, wrong daemon/image OS/architecture, missing manifest | Refuse before deployment effects |
| Mutated/extra final Compose field, resource, mount or final fallback port | Refuse before deployment effects |
| Mutable image tag / bad archive hash | Refuse load/start |
| Foreign container/volume with expected name | Conflict, no takeover/stop/delete |
| Helper initialization active on stop request | 409; keep containers until safe retry |
| CLI whole-stack stop while same-profile launcher runs | Explicitly require tray stop or launcher exit; never open instead |
| Formal release without tool manifest | Fail; only explicit development mode may omit tools |
| Wrong tool architecture / repeated JSON/path traversal / existing archive | Refuse packaging |
| Redirected stdout/stderr start as cp1252 or ASCII | Chinese help, progress, success and errors remain valid UTF-8; preserve success/failure exit codes |
| Cache cleanup invoked without `-Execute` | Preview and record plan only; no cache deletions |
| Cleanup manifest is foreign, malformed, out of bounds, uses links, or selects a running target | Fail explicitly before deleting files |
| A planned file is missing / changed or an unplanned file appears | Missing is idempotent; changed and unplanned files remain |
| Build environment contains external TEMP/Go/npm cache settings | Replace only the managed path keys with validated project-local paths |
| Build runtime path escapes `.runtime`, uses links, is malformed or unwritable | Explicit error; no child launch or fallback to the system temporary directory |
| Environment runner child returns a nonzero status | Preserve the actual failure status |

## 5. Good / Base / Bad Cases

- Good: move a built package into a Chinese/space path, launch from a different CWD, serve embedded reader chunks, use the same user data after replacing the program directory, then restart safely.
- Base: Linux CLI preserves all previous defaults, environment and no-desktop behavior; Windows repeated launch activates the existing instance; Docker and native profiles have separate identities/data.
- Bad: treat a port probe as a data lock, restart before previous DB closes, load FFmpeg from arbitrary CWD, reset broken JSON, map arbitrary NTFS paths into helper bind mode, or announce a development artifact as an offline full release.
- Exit good: stop during a deliberately unread large-file response and inherited-stdout media fixture, observe the exact owned PIDs finish, exclusively reopen/rename the sample files and immediately restart the same profile. Exit bad: hide the tray, wait only for `Serve`, or use `taskkill /IM ffmpeg.exe` and assume all resources were safely released.
- Cleanup base: review the exact-file plan, then execute it after closing the matching build/test-browser process. Bad: infer ownership from time alone, clear all of `%TEMP%`, or delete other Node projects/profile directories.

## 6. Tests Required

- Native Windows temporary behavioral tests: cross-process data lease, concurrent startup, lock/bind failures before DB mutations, canceled startup and cleanup, current-user IPC, bounded port fallback, configuration precedence, path aliases, real readiness, shutdown/restart and subprocess lifecycle. Execute then remove newly generated unit-test sources.
- Exit tests use real Windows process handles, inherited pipe/file descendants, an independent sentinel, active slow HTTP clients and the native tray message pump/menu dispatch. Check both a live tool parent and an already-exited parent, startup cancellation, repeated quit and restart. Report internal menu-dispatch testing separately from physical tray clicking. `scripts/verify_desktop.py` uses a copied EXE and isolated 5 GiB sparse file to assert slow/idle quit, unavailable HTTP/IPC, zero-share access/rename of EXE/DB/lease/log/media and immediate restart; sparse data proves streaming lifecycle, not media decode or throughput.
- Program-file release is the primary Windows exit acceptance: check the copied EXE first immediately after `process.wait`, overwrite that same path with the original build, verify its hash, rename/restore its containing directory and immediately run the overwritten executable with the same profile. Do not put a five-second negative IPC lookup or arbitrary sleeps before the file check. Independently verify owned tool image release; do not claim real shared DLL coverage from an unexecuted sidecar fixture.
- Saved-profile regression tests omit and null each of all nine keys and reject empty/relative saved data/media paths while preserving malformed source bytes. Desktop preflight tests accept actual offline normalized Compose JSON and reject mutations of image platform, fixed fields/resources/privileges and the final selected port.
- Real Linux `/tmp` tests via Linux host/WSL for flock, symlink aliases, POSIX UID/mode and cancellation; do not use NTFS mount mode results as Linux permission evidence.
- `scripts/verify_release.py --self-test` for traversal, duplicate JSON, mutable images, wrong architectures, deterministic archives and tampered/unlisted members. Verify actual target packages and run moved embedded binaries. `scripts/verify_desktop.py` records isolated launcher checks when available.
- `scripts/verify_desktop.py --with-tray` additionally checks all ten embedded icon frames against the source ICO, releases its resource mapping before EXE release checks, and starts the real native tray to inspect distinct class icon handles both before and after an in-place executable replacement. Preserve the existing streaming/exit/restart assertions. This is native API verification, not physical mouse-click or Explorer screenshot acceptance.
- Release-console regression must launch fresh Python subprocesses with `PYTHONUTF8=0` and `PYTHONIOENCODING=cp1252` / `ascii`, capture bytes, then strictly decode UTF-8 and assert Chinese output plus exit codes. Cover all three release CLI help paths and release progress/error paths; a UTF-8 parent process alone does not reproduce the Windows Actions failure.
- Go vet/test/build, strict Vue typecheck/production build, Linux amd64/arm64 cross-build, existing five-media + 5 GiB offset/auth/download/search/bootstrap regression harnesses. Empty retained Go test suite is compilation evidence only.
- Docker `compose config --quiet` is offline structure validation. Actual image pulls, Desktop volumes, real qB/Jackett initialization, readiness and exact stop are separate manual deployment acceptance; do not start the daemon to obtain evidence when user reserved it for manual validation.
- Maintenance checks cover both PowerShell parsers, read-only preview, selector opt-ins, manifest tampering, sibling/path escapes, links, active processes, changed/missing/new files, partial failure, idempotency and empty-directory handling. Use isolated or mocked cleanup targets for destructive behavior. If real deletion was rejected by tool policy, do not reroute it through the script; report preview and fixture evidence separately from actual cleanup.
- Project environment checks cover clean imports, config validation and links/junctions, polluted external environment, fresh and pre-cached Python tempfile, argument/exit-code preservation, process-only scope, both PowerShell versions, real Node/npm/Go cache queries and applicable existing release verification. Test from a different CWD and a Chinese/space path; distinguish shell parsing from actual Linux execution. Remove newly generated temporary test sources after collecting evidence.
- PowerShell source-encoding fixes require byte comparison against the original file (only the UTF-8 BOM may change), `Parser.ParseFile` for every `scripts/*.ps1` under Windows PowerShell 5.1 and PowerShell 7, and controlled `dev.ps1` backend/frontend/build dispatch with exact arguments and native exit-code propagation. Include a semicolon-separated `MediaRoots` value such as `E:/Media;F:/Books` without opening those real paths; keep fixtures under `.runtime/verify/` and remove generated test sources afterward.
- Development-command changes also require real `dev.ps1 -Mode frontend` startup under both PowerShell versions using the installed npm/Vite. Assert HTTP 200 for `/` (project title, Vite client and `/src/main.ts`) and `/src/main.ts`, inspect the actual command line for an unexpected positional root, and stop only processes created by the check. A ready log or a `.cmd` replacement alone cannot validate a real `npm.ps1` dispatch path. Backend API readiness remains a separate check; frontend mode starts only Vite.

## 7. Wrong vs Correct

```go
// 错误：关闭端口后再启动，健康检查可能打开第三方应用。
listener.Close()
go app.Run(ctx, config, logger)
openBrowser("http://127.0.0.1:8090")

// 正确：传递真实监听器，在运行时验证本实例就绪后才打开网页。
err := app.RunWithOptions(ctx, config, logger, app.RunOptions{
    Listener: listener,
    Ready: func(address string) { notifyReady(address) },
})
```

Do not call `os.Setenv` to merge profile configuration. Use a snapshot lookup so native and Docker profiles, inherited environment and validation all agree without global mutation.

Shutdown example: wrong — `server.Close(); <-serveDone; db.Close()` with untracked handlers, or direct-child `Process.Kill()` as proof that inherited resources are free; correct — close admission, cancel request/work contexts, terminate owned connections/process trees, wait for handlers/workers/processes, then close the database and release instance/log/IPC owners.

Cache cleanup example: wrong — recursively remove `%TEMP%` or all browser profiles; correct — generate the repository-scoped file manifest, review the exact targets, then revalidate and execute only its unchanged cache files.

Build environment example: wrong — run `go env -w GOCACHE=...` or set a fake `GOTELEMETRYDIR` environment variable and claim all system writes are isolated; correct — use `python -B scripts/project_env.py -- go build ...`, validate the actual cache paths, and document the scope of third-party tool settings.

Release CLI encoding must be configured before argparse can print Chinese help:

```python
# 错误：依赖宿主默认编码，Windows 重定向输出可能无法编码中文。
parser.parse_args()
print("执行发布构建")

# 正确：仅在 CLI 入口初始化两个标准流，保留中文与真实错误。
def main() -> int:
    """解析参数前统一发布日志编码。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    # 按现有发布契约继续解析参数与构建。
```
