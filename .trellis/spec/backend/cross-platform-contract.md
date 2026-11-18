# Cross-platform launcher and release contract

## 1. Scope / Trigger

Applies to `cmd/nas-desktop`, `internal/desktop`, application lifecycle/static assets/tool discovery, `compose.desktop.yaml`, release scripts and changes to Linux/Windows platform boundaries. One media/download/search implementation remains authoritative. Do not introduce Windows GUI dependencies into the Linux CLI or silently migrate between native and container data.

## 2. Signatures

- `app.Run(ctx context.Context, config Config, logger *slog.Logger) error` remains the compatible CLI wrapper.
- `app.RunWithOptions(ctx context.Context, config Config, logger *slog.Logger, options RunOptions) error`; `RunOptions.Listener net.Listener` transfers ownership, including startup failures; `Ready func(address string)` fires once after this instance is actually reachable.
- `app.LoadConfigWithLookup(lookup func(string) string) (Config, error)` validates the final merged configuration without changing global environment. `LoadConfig` delegates to `os.Getenv`.
- `webassets.Files() fs.FS` returns embedded `dist` under `webembed`, nil in ordinary development builds.
- Windows entry: `栖库.exe --mode native|docker --profile <local-root>`; native diagnostic flags include `--headless`, `--no-browser`, `--media-root` (repeatable), `--data-dir`, `--port`, `--print-config`, and `--control open|status|quit`.
- `python scripts/release.py --target windows-amd64|linux-amd64|linux-arm64 --tools-manifest <file>` produces a package; `--development-without-tools` explicitly selects an incomplete development package.

## 3. Contracts

### Shared runtime

- Acquire canonical data-directory OS lease before database open/migration, job recovery or background workers. Linux uses flock; Windows uses a native file lock. Hold through database and worker shutdown; do not unlink a held lock file. Two different ports do not make shared SQLite writers safe.
- Preserve the listener actually bound by the launcher; do not close/rebind between probing a port and service startup. Failure to bind must not migrate/recover the database. On all failures close the transferred listener.
- Ready requires a database-backed `/healthz` probe carrying this instance's random token plus available static HTML; never accept an unrelated service's generic HTTP 200. Caller callbacks cannot replace actual startup checks.
- Shared shutdown cancels workers/media subprocesses, closes HTTP and database, and only then releases the lease. Preserve 64-bit streaming, no global short WriteTimeout, source read-only and download publication gates.
- `NAS_WEB_DIR` explicitly set takes precedence over embed. Ordinary builds retain `web/dist`. Embedded and disk static routes preserve MIME, HEAD, SPA fallback and asset-not-found semantics. Missing assets must not return SPA HTML for JS requests.
- `NAS_FFMPEG_PATH` / `NAS_FFPROBE_PATH` must be absolute when present. Otherwise use executable-adjacent `tools`, then safe PATH. Resolve once consistently for execution and settings. Reject accidental working-directory tool loading; keep Windows process-window details in platform files.

### Desktop profile and control

- Default profile is `%LOCALAPPDATA%/Qiku/profiles/native` or `docker`; the business data root is separate. Precedence: explicit CLI > applicable NAS environment > saved JSON > defaults. Original CLI does not read desktop JSON.
- Config schema v1 fields: `schemaVersion`, stable random 32-hex `instanceId`, `mode`, `dataDir`, array `mediaRoots`, `port`, `autoOpen`, `dockerContext`, `imageManifest`. Every field is documented in Chinese. Saved files require all nine exact canonical keys, none null; decode into a fresh value rather than overlaying defaults. Stored data/media paths must be absolute, `dataDir` nonempty and `mediaRoots` nonempty. Only an absent file receives defaults; explicitly supplied CLI/environment relative paths may be normalized before saving. Bounded strict JSON, atomic same-directory replacement and actionable failure; do not reset malformed config into a new library or resolve an empty saved data path to the caller's CWD.
- Default `127.0.0.1:8090`, at most 20 actual bind candidates; preserve the chosen port. LAN access uses explicit CLI configuration. Local IPC is current-user-only and only accepts fixed actions, never commands or arbitrary URLs. Browser opens after actual readiness.
- Native supports at most 32 explicit roots; Docker Desktop first release supports one Windows read-only media bind and rejects additional roots. Changing data directory means an explicit independent library selection, not automatic migration. Offline saved roots remain configured.
- Closing a browser does not stop native service. Docker launcher exit and whole-stack stop are separate actions. Stop first uses helper's owned-engine stop protocol, then Compose stop; never claim Compose down alone stops dynamic engines. When the same profile's launcher is running, `--docker-action status` uses its fixed status IPC; `--docker-action stop` must explicitly refuse with instructions to use the tray's complete-stop action or first exit the launcher. It must not reinterpret stop as opening the page.

### Desktop Docker and release

- Only selected local Desktop Linux context, no TCP/SSH daemon. First-release Desktop daemon and app/helper image platform must be Linux amd64; inspect actual image OS/architecture/content identity and reject implicit emulation. Resolve Docker CLI to an absolute path before changing process directory. Clear inherited DOCKER/COMPOSE redirection before explicit context/project/env-file arguments. Never start/install/reconfigure Docker or WSL as a side effect of checks.
- `release-images.json`: `schemaVersion=1`, `appImage`, `helperImage` are immutable repository digests or exact local sha256 IDs. Optional `archive` under `images/` and `archiveSha256` appear together. Verify SHA-256 before load; absent manifest gives a real prerequisite error, never a fabricated public image.
- Compose env: `NAS_APP_IMAGE`, `NAS_HELPER_IMAGE`, identical `NAS_DESKTOP_PREFIX` and `ENGINE_VOLUME_PREFIX`, `ENGINE_NETWORK=<prefix>-engines`, `MEDIA_PATH`, `WEB_PORT`, `WEB_BIND_IP=127.0.0.1`, `QBT_PEER_PORT=6881`, `JACKETT_SETUP_PORT=19117`. All source config keys carry Chinese explanations.
- Before pull/load/create/start/stop, strictly validate the final normalized Compose JSON: exactly app/helper services, immutable distinct selected images, fixed environment/labels/security/logging/commands, exact read/write mount sets, one loopback app port, seven role volumes and one instance bridge. Reject unknown fields, privilege/entry overrides, extra networks/mounts/ports, foreign names/labels, external or driver-option volumes and custom IPAM. Revalidate after selecting a fallback port; checking the source YAML or initial port alone is insufficient.
- Container ownership requires actual Compose project/service plus deployment/role labels, expected images and mounts. Do not stop/reuse a foreign same-name resource. Default BT data is in Linux Docker volumes, not an NTFS download folder.
- Whole-stack stop allows 105 seconds for helper `stop-all`, exceeding its 90-second internal polling budget. Continue to exact Compose app/helper stop only after successful owned-engine completion; helper failure/unavailability/timeout must not claim full stop or discard data volumes.
- Full packages require fixed tool version/source, per-file hashes, matching PE/ELF architecture, actual applicable licenses and corresponding source/build materials. Shared tool builds need matching DLL/libraries. Metadata checks are not a substitute for license review or clean-machine execution.
- Source and packaged tool manifests share strict field/group validation. Required `tools/ffmpeg[.exe]` and `tools/ffprobe[.exe]` names are exact lowercase; case aliases cannot satisfy architecture checks. The generated `licenses/ffmpeg/tools-manifest.json` destination is reserved and cannot be supplied as input material. Package paths reject control characters and Windows device/trailing-dot/space aliases on every target.
- Archive verification rejects duplicate/non-object JSON and mutable image references, checks every runtime/license/source record against actual member hashes, and inspects the packaged PE/ELF architecture without executing or extracting tools. A recomputed outer checksum alone does not bypass package structure validation.
- Development package names and `release.json.developmentWithoutTools=true` clearly state missing tools. Build matrix covers Windows amd64 and Linux amd64/arm64; cross-build and actual execution are reported separately. No automatic publishing/signing/updating.
- Release-tool CLI entry points (`release.py`, `verify_release.py`, `package_desktop_images.py`) configure both standard streams as UTF-8 before argument parsing, including Chinese help and failure diagnostics. Share the initializer without reconfiguring the importing process at module import time; tolerate replacement streams without `reconfigure`. CI sets documented `PYTHONUTF8=1` and `PYTHONIOENCODING=utf-8`; scripts must also work when directly invoked with legacy redirected stream encodings. Changing the Windows code page or removing Chinese text is not the fix.
- The build workflow uses Node 24 action runtimes (`checkout`, `setup-go`, `setup-node`, `setup-python`, `upload-artifact` v6, verified from their official `action.yml`). The JavaScript runtime executing an action is separate from the pinned `node-version` used to build the application; do not change application toolchain versions just to suppress an action-runtime deprecation warning.

## 4. Validation & Error Matrix

| Condition | Expected behavior |
| --- | --- |
| Canonical data directory already locked | Refuse before DB/recovery; do not alter data or terminate owner |
| Requested port occupied | Bind next bounded candidate; do not probe/open foreign app |
| All candidates unavailable | Chinese actionable error, no false readiness |
| Bad config, wrong mode/schema, missing/null saved field, empty/relative saved path, unsafe directory | Preserve config/data, fail explicitly |
| Missing embedded HTML or explicit web directory | Ready callback not called; no browser opened |
| Relative explicit FFmpeg/probe path | Configuration error |
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

## 5. Good / Base / Bad Cases

- Good: move a built package into a Chinese/space path, launch from a different CWD, serve embedded reader chunks, use the same user data after replacing the program directory, then restart safely.
- Base: Linux CLI preserves all previous defaults, environment and no-desktop behavior; Windows repeated launch activates the existing instance; Docker and native profiles have separate identities/data.
- Bad: treat a port probe as a data lock, restart before previous DB closes, load FFmpeg from arbitrary CWD, reset broken JSON, map arbitrary NTFS paths into helper bind mode, or announce a development artifact as an offline full release.

## 6. Tests Required

- Native Windows temporary behavioral tests: cross-process data lease, concurrent startup, lock/bind failures before DB mutations, canceled startup and cleanup, current-user IPC, bounded port fallback, configuration precedence, path aliases, real readiness, shutdown/restart and subprocess lifecycle. Execute then remove newly generated unit-test sources.
- Saved-profile regression tests omit and null each of all nine keys and reject empty/relative saved data/media paths while preserving malformed source bytes. Desktop preflight tests accept actual offline normalized Compose JSON and reject mutations of image platform, fixed fields/resources/privileges and the final selected port.
- Real Linux `/tmp` tests via Linux host/WSL for flock, symlink aliases, POSIX UID/mode and cancellation; do not use NTFS mount mode results as Linux permission evidence.
- `scripts/verify_release.py --self-test` for traversal, duplicate JSON, mutable images, wrong architectures, deterministic archives and tampered/unlisted members. Verify actual target packages and run moved embedded binaries. `scripts/verify_desktop.py` records isolated launcher checks when available.
- Release-console regression must launch fresh Python subprocesses with `PYTHONUTF8=0` and `PYTHONIOENCODING=cp1252` / `ascii`, capture bytes, then strictly decode UTF-8 and assert Chinese output plus exit codes. Cover all three release CLI help paths and release progress/error paths; a UTF-8 parent process alone does not reproduce the Windows Actions failure.
- Go vet/test/build, strict Vue typecheck/production build, Linux amd64/arm64 cross-build, existing five-media + 5 GiB offset/auth/download/search/bootstrap regression harnesses. Empty retained Go test suite is compilation evidence only.
- Docker `compose config --quiet` is offline structure validation. Actual image pulls, Desktop volumes, real qB/Jackett initialization, readiness and exact stop are separate manual deployment acceptance; do not start the daemon to obtain evidence when user reserved it for manual validation.

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
