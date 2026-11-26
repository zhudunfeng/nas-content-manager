# Research: Windows executable-adjacent dotenv configuration

- Query: How can Windows desktop read `.env` beside the real executable with process environment > dotenv precedence, without changing global environment or managed-worker isolation?
- Scope: internal repository configuration, native worker handoff, release packaging and verification.
- Date: 2026-11-26.

## Findings

### Existing paths and extension points

- `internal/desktop/launcher.go:61` owns desktop startup. It parses options before profile/config loading; both print-config (`:86`) and ordinary startup (`:150`) load/apply configuration. One immutable snapshot should be created before these reads, after CLI help/control routing if desired. Avoid rereading dotenv on tray restart; document full process restart semantics.
- Keep explicit `--control status|quit|open` before dotenv parsing: these actions only need the profile/control channel. A user must still be able to stop a running process after accidentally damaging `.env`; neither control diagnostics nor IPC responses need the file's settings or secrets. CLI help should also remain independent of dotenv parsing. Add a malformed-dotenv + real `--control quit` regression.
- `internal/desktop/platform_windows.go:17` resolves the default profile through `os.Executable`, `filepath.Abs`, and `filepath.EvalSymlinks`. Resolve dotenv through the same real-executable-directory identity rather than the caller's CWD or `--profile`.
- `internal/desktop/config.go:116` loads strict schema-2 desktop JSON. `applyOptions` (`:182`) currently reads `NAS_DATA_DIR`, `NAS_MEDIA_ROOTS`, `NAS_ADDR`, and `NAS_DESKTOP_COLLECTION_MODE` through global getenv before applying CLI. Add lookup plumbing rather than dotenv fields to `desktop.json`.
- `internal/desktop/collection_config.go:37` and `:42` use environment presence/value for external collection detection and initial collection mode; v1 migration at `config.go:159` calls the same helper. All must use the same merged source, or dotenv external configuration can produce inconsistent modes.
- `internal/desktop/launcher.go:401` passes `os.Getenv` to native BT configuration; `:423-428` builds the service source with `app.LoadConfigWithLookup`. Replace both with the snapshot lookup. This naturally covers existing NAS core, external worker, BT/Torznab, cache and media-tool configuration without altering CLI/server packages.
- `internal/app/config.go:41` already exposes `LoadConfigWithLookup(func(string) string)`. Its nested download/search/engine/collection readers use that lookup. Ordinary CLI `LoadConfig` remains environment-only at `:36-37`.
- `internal/desktop/bt_config.go:91` has an existing lookup seam. BT JSON still uses nonempty overrides; an explicit empty environment variable must suppress dotenv, while the existing application fallback for empty values can stay compatible. Distinguish source precedence from downstream empty-value meaning.

### Minimal immutable-source design

1. Read process environment into a map once. On Windows, compare environment identifiers case-insensitively, matching OS lookup semantics.
2. Parse the real EXE directory's `.env` into a separate map. Merge by **presence**, not by nonempty value: process `KEY=` shadows dotenv `KEY=value`.
3. Expose both `lookup(key) (string, bool)` for explicit flags and `get(key) string` for existing configuration readers. Store this source on the controller so restarts and worker checks use the same snapshot.
4. Keep process-specific runtime values (bound listener address, selected profile paths, owned worker token/read root) above administrator configuration, preserving current ownership invariants. CLI still overrides desktop-configurable values.
5. Do not use `os.Setenv`, `godotenv.Load`, user/registry environment, or child-wide arbitrary dotenv inheritance.

Two alternatives are less suitable: global dotenv loading violates the existing no-global-mutation contract; adding dotenv to the generic service/worker loaders broadens Windows-desktop scope and can accidentally change Linux CLI or independently launched worker behavior. A desktop-only source is the smallest integration seam.

### Parser and error contract to define explicitly

- No dotenv parser or dotenv dependency was found in the repository (`go.mod`, desktop source, script references). A bounded small parser can use the standard library and avoid introducing a new dependency.
- Suggested documented subset: UTF-8 with optional initial BOM; CRLF/LF; blank lines and `#` comments; `KEY=value`; single/double quoted values; optional `export` if implemented; inline comments only outside quotes with a defined whitespace rule. Preserve Windows backslashes literally unless escape handling is intentionally documented.
- No variable expansion, command substitution, sourced scripts, or multiline shell evaluation. Values such as `$HOME`, `${NAME}`, `%TEMP%`, and backticks remain literal or are explicitly rejected as unsupported syntax; never execute them.
- Use bounded reads (64 KiB is consistent with other desktop config files), validate UTF-8 and identifiers, reject NUL/newline contamination and malformed quotes, and decide duplicate-key behavior. A missing `.env` is normal; an unreadable/non-regular/oversized/malformed file fails with Chinese file/line information and no value echo.
- Prefer limiting dotenv influence to recognized NAS configuration namespaces, or at least never let dotenv replace `SystemRoot`, `WINDIR`, PATH or Python loader controls when constructing a managed child. Existing system prerequisites should come from the captured process environment.
- Parse and validate the entire present file even if some keys are shadowed; otherwise broken configuration can become latent and activate unexpectedly after an environment change.
- No log/print-config output should enumerate secret values; source file and number of accepted entries are sufficient startup diagnostics.

### Managed-worker boundary

- `internal/desktop/collection_managed.go:111` explicitly constructs a child-only environment. It currently permits two plugins' `_ENABLED`, `_PYTHON` plus four resource limits (`MAX_BYTES`, `MAX_FILES`, `MIN_FREE_MB`, `MAX_CONCURRENT`). Feed these configurable fields from the merged snapshot.
- `_ENABLED` at `:132` uses `LookupEnv` rather than nonempty getenv. Preserve explicit false and explicit-empty validation: true/false are the only accepted literals.
- The worker address, data root, token path, tools directory/PATH, yt-dlp/FFmpeg and runtime manifests are fixed by desktop at `:112-120`, `:128`, `:141`; dotenv must not override these in managed mode.
- `startManagedCollection` (`:157`) starts the packaged worker with `command.Env = environment` and `--managed-stdio`; there is no need for that worker to parse the parent's `.env` again. Ordinary worker `LoadConfig` (`internal/collectionworker/config.go:57`) can remain unchanged.
- Optional worker config such as `SOURCES_FILE`, `RCLONE_CONFIG_FILE`, `RCLONE`, `_RUNTIME_MANIFEST`, `_YTDLP`, `_FFMPEG` exists in the generic worker but is not currently exposed by desktop's allowlist. Do not advertise it as supported by the desktop merely because its identifier parses in dotenv. Extending those permissions is a separate contract decision.
- Setting only `NAS_COLLECTION_WORKER_UAV_ENABLED=true` does not guarantee an existing disabled desktop profile switches to managed. The usage example should pair `NAS_DESKTOP_COLLECTION_MODE=managed` with the plugin flag; packaged manifest/Python and downloader dependencies remain required.

### Docker boundary and release

- `internal/desktop/docker.go:277-278` intentionally supplies a managed empty `docker.env` with explicit `--env-file` (`:161`) to block Compose's implicit `.env` ingestion. Preserve that isolation: desktop dotenv can influence existing desktop choices, but must not become arbitrary Compose interpolation/container environment.
- `internal/desktop/docker_compose.go:241` fixes worker container configuration. Do not replace it with arbitrary dotenv passthrough.
- `.gitignore:8` already ignores `.env`; no new ignore is needed unless adding another sensitive filename.
- `scripts/release.py:352-354` copies Windows example configuration files. Add a checked-in `.env.example` to that list, not a populated `.env`, and describe copy/edit/restart/precedence in the Windows quick start and plugin docs.
- `scripts/release.py:364-365` constructs the packaged Chinese start notice. Add the `.env.example` instruction there. `checksums.sha256` is generated from stage contents at `:372`, automatically covering the example.
- `scripts/verify_release.py:150` checks archive structure and hashes; source expected-file checks can assert the example's presence without requiring a sensitive real `.env`.

## Verification map

- Temporary Go behavioral tests: parser UTF-8/BOM/CRLF, literal Windows paths/hash/equals, comments/quotes, empty values, invalid keys/quotes/encoding/NUL/size, duplicate policy; environment true/false/empty overrides dotenv; no global mutation; identical desktop/BT/app/worker snapshot behavior; controlled child path invariants.
- Real Windows executable: copy under an isolated Chinese/space path; run from foreign CWD with a conflicting CWD `.env`; assert EXE-side file wins and `--profile` does not relocate it; run `--print-config` with no filesystem creation; verify CLI > process > dotenv > saved/default; test malformed redacted failures.
- Real worker: use `scripts/verify_desktop_collection.py` helpers/fixtures for owned-child startup; assert plugin capabilities reflect dotenv and process overrides, quotas flow, fixed token/data/runtime paths cannot be redirected, and quit releases executable/child handles.
- Existing reusable regressions: `scripts/verify_desktop.py --binary ... [--web-dir ...]`; `scripts/verify_desktop_collection.py --binary ... --worker ... --report ...`; `scripts/verify_release.py --self-test`; a development Windows release archive plus verification. Do not claim a development package contains complete UAV dependencies.
- Run commands through `python -B scripts/project_env.py -- ...`; remove only new temporary unit-test sources after evidence is captured, per AGENTS.

## Related specs and local references

- `.trellis/spec/backend/cross-platform-contract.md:11`, `:169`: lookup configuration and no global mutation.
- `.trellis/spec/backend/jmcomic-windows-contract.md:66-69`: child-only managed environment and external process ownership.
- `.trellis/spec/backend/uav-windows-contract.md`: independent plugin health, fixed runtime/tool publication.
- `.trellis/spec/backend/bt-windows-contract.md:39`: nonempty environment overrides and legacy flag handling.
- `docs/windows.md:64`: existing precedence description to update.
- `docs/uav-windows.md:47-52`, `docs/jmcomic-windows.md:27-32`: configuration tables needing precise dotenv scope.
- No external references were required; these recommendations use the current repository's implementation and executable contracts, not new third-party library behavior.

## Caveats / Not Found

- CodeGraph was tried through the project environment runner for `loadNativeBTValues` and `collectionEnvironment`; both returned empty result arrays. Current index does not cover those symbols. Exact `rg` and bounded source reads supplied file/line evidence instead.
- Source line numbers refer to the pre-implementation checkout and may move during the task.
- The process-empty-vs-dotenv precedence is distinct from legacy downstream `getenv != ""` fallback behavior. Preserve the former without silently changing every existing parameter's empty-value semantics.
- This research does not verify remote video sites, dependency installation, complete release licensing, or an independently started worker's reading of `.env`; those are outside this desktop configuration change.
