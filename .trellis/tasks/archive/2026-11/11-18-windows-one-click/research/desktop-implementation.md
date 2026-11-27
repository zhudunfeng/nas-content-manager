# Desktop launcher implementation evidence

- Date: 2026-11-18.
- Scope: `cmd/nas-desktop/*`, `internal/desktop/*`, and `scripts/verify_desktop.py`.
- Environment: actual Windows development host; Linux targets were cross-compiled only in this subtask. No Docker daemon deployment, image pull/load, container start/stop or WSL setting changes were executed.

## Implemented structure

- `cmd/nas-desktop/main_windows.go`: thin native entry. `main_other.go` explicitly directs Linux users to the existing headless `nas`/Compose entry.
- `config.go`, `json.go`: independent native/docker profiles, stable random installation ID, explicit flags > environment > saved JSON > defaults, Chinese parameter comments, bounded strict JSON with duplicate/case-alias/unknown/trailing/depth checks, atomic replacement. Defaults apply only when the configuration file is absent. Saved schema-v1 JSON requires all nine canonical non-null keys, absolute stored data/media paths, valid identity and explicit directory/port settings; a damaged file cannot silently select a default or CWD database.
- `launcher.go`: shared `app.LoadConfigWithLookup` and `app.RunWithOptions`; held native listener with at most 20 bounded candidates, actual shared-runtime readiness plus health/HTML checks, serial configuration/restart, graceful exit, persistent selected port, unexpected-service-stop invalidation and bounded user-visible failures. No global environment mutation.
- `platform_windows.go`: current-user/SYSTEM protected ACL, local data-directory checks including resolved drive type, Windows atomic replacement, hidden CLI child windows.
- `ipc_windows.go`: per-user/per-canonical-profile named mutex and protected local named pipe; only `open`, `status`, `quit`; bounded messages, overlapped cancellation and an acknowledged quit response before shutdown. The server is available while the first-start wizard is active.
- `shell_windows.go`: native notification-area loop, Explorer restart recovery, native IFileOpenDialog directory picker, open/add/remove roots, independent data-library selection, logs, restart/quit and explicit Docker complete-stop menu. Browser close does not terminate the service.
- `log.go`: synchronized structured Chinese log, 5 MiB current file and three rotated backups.
- `docker.go`, `docker_compose.go`: explicitly selected local Docker Desktop named-pipe context, Linux **amd64** baseline, minimum 2 CPU / 2 GiB, absolute Docker CLI path, inherited routing/interpolation cleanup, fixed immutable image manifest, strict `images/` archive paths and SHA-256-before-load, actual image OS/architecture/content-ID checks. App/helper may not resolve to the same image ID.
- Docker preflight validates the final normalized Compose JSON before side effects: exact two services, fixed images/environment/labels/security/logging, exactly one loopback application port, exact read/write mount sets, seven role volumes and one instance bridge. Unknown fields, privilege knobs, entry overrides, extra networks/mounts/ports, foreign names/labels, external/driver-option volumes and custom IPAM are rejected. The final chosen port is revalidated.
- Existing resource reuse/stop checks actual volume/network/container ownership, fixed images/default commands, mounts, labels and local port bindings. Explicit whole-stack stop first asks helper to stop its owned dynamic engines, allowing 105 seconds for the helper's 90-second protocol, then Compose-stops only this app/helper. No resource deletion or global prune.

## Checks executed

Commands used the repository Go toolchain (`.tools/go/bin`) and current source:

```powershell
gofmt -w internal/desktop cmd/nas-desktop
go test ./internal/desktop -count=1 -v
go vet ./internal/desktop ./cmd/nas-desktop
go build -o .runtime/desktop-check/栖库.exe ./cmd/nas-desktop
python -m py_compile scripts/verify_desktop.py
python scripts/verify_desktop.py --binary .runtime/desktop-check/栖库.exe --web-dir web/dist
```

All passed. The temporary Go suite had eight top-level tests:

1. Strict config, identity persistence, semicolon root-list handling and overlap rejection.
2. Immutable manifest, strict `images/` archive path segments, duplicate keys and inherited Docker/Compose environment cleanup.
3. Every saved config key missing/null, empty/relative data paths, preservation of malformed source bytes.
4. Linux/amd64 image platform and content identity validation.
5. Actual offline `docker compose config --format json` output accepted; 33 mutated port/mount/volume/network/environment/privilege/entry cases rejected, including checking the changed final selected port.
6. Actual Windows protected mutex/named pipe, repeated requests, unknown action rejection and acknowledged quit cancellation.
7. Actual shared core: root save, controlled stop/restart, readiness, database lease release and persistence.
8. Actual shared core: unexpected cancellation clears ready URL and returns failure.

The final temporary run passed in approximately 1.86 seconds. `internal/desktop/desktop_temp_test.go` was deleted after execution per AGENTS.md. Subsequent `go test ./internal/desktop ./cmd/nas-desktop` passed with `[no test files]`; that subsequent command is compilation evidence, not replacement behavioral evidence.

`scripts/verify_desktop.py` is retained as a reusable real-binary harness. Its final source passed against the current ordinary Windows binary with explicit `web/dist`, covering Chinese/space/semicolon paths, arbitrary CWD, a genuinely occupied port, health and HTML, actual named-pipe control, duplicate launch without rewriting config, different-profile/same-data lock rejection without disrupting the owner, graceful quit, local restart retaining instance/port/admin, duplicate JSON keys, all missing/null saved fields, empty/relative data paths and unchanged original media bytes. It owns and cleans its temporary processes and data. Restart used local existing resources without a download, but the host network adapter was not disabled; it is not a physical network-isolation test.

An earlier embedded GUI development package at `.runtime/windows-one-click/移动 程序/qiku-0.1.0-windows-amd64-development/栖库.exe` also passed the harness before the final saved-config completeness additions. The final rebuilt embedded package is rerun by the parent task; do not treat the earlier artifact as the final binary.

Additional checks passed after temporary-source removal:

```powershell
go test ./internal/desktop ./cmd/nas-desktop
# GOOS=linux GOARCH=amd64
go build -o .runtime/desktop-check/nas-desktop-linux-amd64 ./cmd/nas-desktop
# GOOS=linux GOARCH=arm64
go build -o .runtime/desktop-check/nas-desktop-linux-arm64 ./cmd/nas-desktop
git diff --check -- internal/desktop cmd/nas-desktop scripts/verify_desktop.py
```

The Linux desktop builds prove platform import isolation only. They intentionally report unsupported tray use; the real Linux application and helper are verified independently by the parent task.

## Exact user-facing Docker behavior

- First release accepts a Docker Desktop Linux amd64 daemon and Linux amd64 app/helper images. Windows ARM64/other backend architectures are not implicitly emulated.
- Docker mode supports one explicitly authorized read-only Windows media root; additional roots fail visibly. Native mode supports up to 32 roots.
- Native and Docker have separate identity, configuration and data. Docker BT content is stored in Linux named volumes, not a normal NTFS download folder.
- Exiting the Docker launcher retains running containers/downloads. The tray's complete-stop action stops helper-owned engines first and preserves data volumes.
- `--mode docker --docker-action status` reads the active launcher's fixed status IPC when that profile is already running. If no launcher exists, it independently checks actual owned containers and readiness.
- `--mode docker --docker-action stop` refuses with an actionable Chinese message when that profile's launcher is already running; use the tray complete-stop action, or quit the launcher first and then run the stop command. It must never silently turn this request into `open`.
- Missing manifest/daemon/permissions/source setup remains an explicit prerequisite failure; no repository/latest guess, Docker/WSL installer, daemon reconfiguration or invented search readiness.

## Remaining verification and limitations

- No actual Docker Desktop deployment, pinned-image pull/import, named-volume runtime, qB/Jackett initialization or dynamic-engine stop was executed; deployment acceptance remains manual as requested.
- No clean Windows 11 ordinary-user test or complete FFmpeg redistribution package was verified by this subtask. The development package is explicitly incomplete; production packaging/license validation belongs to the parent release work.
- The parent observed a readable native first-start prompt and successfully stopped its isolated profile. Folder-picker interaction, tray menus, Explorer recovery, sleep/resume and browser shell integration have not completed automated GUI acceptance; no claim of that coverage is made.
- Same-name existing containers must exactly match the selected immutable manifest. Automatic Docker image-version replacement/rollback is deliberately not attempted. A changed-image release needs an explicit version-aware stop/recreate procedure preserving verified role volumes; unknown older containers are not silently adopted.
- Linux cross-builds are not Linux desktop/runtime execution, ARM64 hardware verification or large-video throughput/RSS evidence.

## Spec notes for the parent

Keep the cross-platform contract aligned with the saved-config completeness requirement, strict Compose final-JSON preflight, Linux/amd64 first Docker baseline and the helper-stop CLI timeout being longer than its 90-second protocol. Retain the separate native/Docker data and explicit upgrade boundaries.
