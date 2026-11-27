# Windows native BT final validation

Date: 2026-11-26, Windows amd64 development host, Asia/Shanghai.

## Delivered scope

- Existing qBittorrent and Jackett/Prowlarr can be configured persistently through optional native-profile `bt.json` schema 1. Search and downloads are independent external/disabled modules, with explicit environment and legacy-flag precedence.
- Relative private files resolve under the profile, including launches from another CWD. Configuration is bounded, strict and valid UTF-8, accepts one leading UTF-8 BOM, rejects explicit null, and is preserved on failure.
- Engine path equality uses the path's own Windows/POSIX format. Windows drive roots, slash direction, case and trailing separators work through ownership, movement and publication. Windows-invalid torrent names are rejected rather than renamed.
- Only the download `complete` subtree enters the effective native media authorization. Existing selected-file publication gates remain; Docker/CLI configuration and external-process ownership remain unchanged.
- Added Windows BT documentation, sample, packaged source template and responsive engine-dialog guidance. No schema migration or new third-party dependency.

## Candidate identities

| Candidate | Bytes | SHA-256 |
| --- | ---: | --- |
| `.runtime/verify/bt-final-artifacts/nas.exe` | 21,651,456 | `a6c99e13e636202b0c0e8623573e6c08e3ae7b8dd1e15b7aba587870c298408b` |
| `.runtime/verify/bt-final-artifacts/desktop.exe`, rebuilt after final UTF-8 fix | 22,097,408 | `63c9560521797516e0cfb7ef2486b8ae232572745fb3561a83a2973e7524ee35` |
| Windows development archive | — | `c7941dfa1211e173f429b6bf2035367fbe89b5c55bce72ac2c37da179e42f862` |
| Desktop extracted directly from that archive | — | `fe5383f51edb64a29d74c717c796e70bf18101d18b0eb5b98ddd1aee8507c9c8` |

The release desktop has embedded web assets and release GUI/trimpath/linker flags; it is a distinct binary from the console validation build. Both final desktop binaries completed the native BT suite. CLI behavior is unaffected by the desktop-only final null/UTF-8 decoder changes.

## Executed verification

Every local command used `python -B scripts/project_env.py -- ...`. New browser processes inherited that environment before launch. Caches, tool temporary files, fixture media and outputs stayed under repository `.runtime/`; no user-level environment or `go env -w` changes were made.

| Check | Observed result / evidence |
| --- | --- |
| Temporary desktop Go matrix | 64 behavior condition groups; 6 test functions and 29 invalid JSON subtests, 35 PASS entries. Includes final null/UTF-8 fixes. See `quality-review.md`; generated source removed. |
| Temporary backend Go groups | 3 groups passed: path semantics, ownership reconciliation, password/source BOM and bounds. See `backend-validation.md`; generated source removed. |
| Windows-path full downloads | 36 assertions; `.runtime/verify/bt-windows-backend-20261126/result.json`. This earlier candidate predates the six additional filename checks. |
| Final POSIX-mapped full downloads | 42 assertions, including six illegal-name upload negatives; `.runtime/verify/bt-posix-backend-20261126/result.json`. Five media, identities, uncertain add, external refusal, restart and publication gates covered in both download modes. |
| Existing torrent search | 43 assertions, actual application with controlled Torznab/qB (`verify_torrent_search.py`). |
| Final native desktop BT | 12 assertions after final decoder fix; `.runtime/verify/bt-native-utf8-final/result.json`. Search → torrent → selection → move/publication → original file; restart/idempotence, independent disable, history and external-service survival. |
| Final packaged desktop BT | Same 12 assertions on archive-extracted GUI executable; `.runtime/verify/bt-packaged-desktop-final/result.json`. |
| Existing desktop lifecycle | 17 categories (`verify_desktop.py`), actual Windows private control IPC, Unicode/spaced/semicolon roots, orderly exit with sparse slow stream, original executable/data handle release and account restart. Observed exit times 0.036/0.020 seconds. This ran before desktop-only decoder hardening; final BT suite subsequently tested its startup/restart/shutdown. |
| Existing engine bootstrap integration | 35 assertions; `.runtime/verify/bt-engines-final/result.json`. Actual application, controlled helpers and engine protocol; no Docker execution. |
| Core media regression | 52 assertions; `.runtime/verify/bt-windows-core-final/run-manifest.json`. Actual local FFmpeg/ffprobe, all five media, authentication, state, SQLite snapshot and 5 GiB sparse HTTP offsets. |
| Browser guide | 33 assertions, Chromium 141.0.7390.108; `.runtime/verify/bt-guide-browser-final/result.json`. Real isolated app at 375/768/1024/1440, light/dark, document/dialog/guide overflow, Tab, Escape and focus restoration. Eight screenshots; manually inspected phone-light and desktop-dark. No route fixtures used. |
| Go quality | `go vet ./...` passed after final production fix; final `go test ./...` passed after temporary tests were deleted, counted only as compilation. Scoped gofmt and `git diff --check` passed. |
| Frontend | Vue strict typecheck and Vite production build passed; browser and release reused this build without modifying assets. |
| Linux compilation | Final production source `GOOS=linux CGO_ENABLED=0 go build ./...` passed for amd64 and arm64; no Linux execution claimed. |
| Release boundary self-test | 131 assertions, including PowerShell argument/encoding checks; actual archive separately checked below. |
| Task context | `task.py validate .trellis/tasks/11-26-bt-windows-native` passed for implement/check JSONLs. |

## Actual Windows development archive

Executed:

```text
python -B scripts/project_env.py -- python -B scripts/release.py --target windows-amd64 --development-without-tools --output .runtime/verify/bt-native-release-final --skip-web-build
python -B scripts/project_env.py -- python -B scripts/verify_release.py .runtime/verify/bt-native-release-final/qiku-0.1.0-windows-amd64-development.zip
python -B scripts/project_env.py -- python -B scripts/verify_windows_bt.py --binary .runtime/verify/bt-native-release-extracted/desktop.exe --root .runtime/verify/bt-packaged-desktop-final
```

The archive validator accepted **23 files**, checksums and Windows architecture. A separate ZIP read confirmed `bt.windows.example.json`, `torrent-search.sources.example.json`, `docs/bt-windows.md`, `docs/downloads.md`, `docs/torrent-search.md`, `docs/engines.md`; parsed the sample schema and both external modes; extracted only the known desktop member for the integration run. Evidence: `.runtime/verify/bt-native-release-extracted/package-evidence.json`.

This is explicitly a development package without bundled FFmpeg or third-party BT engines. It is not a complete offline media release. No release publishing, installation or deployment was performed.

## Boundaries and final state

- Controlled Torznab/qB fixtures validate real application HTTP, task, SQLite and publication behavior; they do not transfer BT pieces or prove seeding, peer discovery, trackers or public indexer availability.
- No clean Windows ordinary-user acceptance, Windows ARM64 runtime, Docker/WSL run, Linux container runtime or physical NAS acceptance was performed. These remain separate target checks.
- External engines must already be installed and configured; this task neither installs nor owns their processes. Stop/remove never deletes their payload automatically.
- All spawned checks completed and their owned application/browser/service processes were reclaimed by their harnesses. No newly generated `_test.go` remains.
- Trellis PRD, context and backend code-spec updated; task left for review, no Git commit/archive. The pre-existing runtime-slim task was left untouched.
