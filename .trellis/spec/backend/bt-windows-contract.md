# Windows native BT contract

## 1. Scope / Trigger

The native desktop launcher can persist connections to an existing qBittorrent and Jackett/Prowlarr without Docker/WSL. This extends desktop configuration and cross-platform download path handling. It does not install, launch, stop or adopt those external processes. Docker managed bootstrap keeps its existing contract.

## 2. Signatures

```go
func loadNativeBTValues(profile string, lookup func(string) string) (map[string]string, error)
func decodeNativeBTJSON(data []byte, saved *nativeBTConfig) error
func nativeBTFilePath(profile string, value string) (string, error)
func cleanEnginePath(value string) (string, error)
func isWindowsEnginePath(value string) bool
func sameEnginePath(left string, right string) bool
```

`controller.start` reads optional `<profile>/bt.json` only after the Docker-mode branch. CLI configuration continues to use environment variables. No database or `desktop.json` schema changes are required.

## 3. Contracts

```json
{
  "schemaVersion": 1,
  "downloads": {
    "mode": "external",
    "root": "E:/QikuDownloads",
    "engineRoot": "E:/QikuDownloads",
    "url": "http://127.0.0.1:8080",
    "username": "admin",
    "passwordFile": "bt/qb-password"
  },
  "search": {"mode": "external", "sourcesFile": "bt/sources.json"}
}
```

- `downloads` and `search` are independently optional; at least one must exist. Their modes are `external|disabled`. Every present field must have its declared type, never `null`.
- Download fields map to `NAS_DOWNLOAD_MODE`, `NAS_DOWNLOAD_ROOT`, `NAS_DOWNLOAD_ENGINE_ROOT`, `NAS_DOWNLOAD_QB_URL`, `NAS_DOWNLOAD_QB_USERNAME`, `NAS_DOWNLOAD_QB_PASSWORD_FILE`. Search fields map to `NAS_TORRENT_SEARCH_MODE`, `NAS_TORRENT_SEARCH_SOURCES_FILE`.
- Nonempty explicit environment variables override file values. Legacy `NAS_DOWNLOAD_ENABLED` / `NAS_TORRENT_SEARCH_ENABLED` suppress the file mode when an explicit new-mode environment variable is absent. Omitted `engineRoot` defaults to the final effective `root`, including an environment root override.
- `passwordFile` and `sourcesFile` may be absolute or local relative paths below the profile. Relative references never depend on CWD; reject drive-relative paths and profile escapes.
- A declared download root adds only `<root>/complete` to the effective native media roots, without changing persisted desktop media roots. Original staging/publication gates, root isolation and task identity checks remain authoritative, including disabled-mode historical access.
- Download root validation and qB/source connection validation reuse the existing application configuration validators. Logs report effective modes without passwords, keys or connection URLs.
- `sameEnginePath` detects the path format, not `runtime.GOOS`: Windows drive-absolute paths ignore case, slash direction and trailing separators; POSIX paths retain case sensitivity. Preserve `E:/` as a drive root. Host filesystem comparisons reuse `downloadPathKey`.
- Bounded password/source reads strip one leading UTF-8 BOM only. Preserve password spaces and internal bytes; do not trim credentials. Torrent filenames reject Windows-invalid characters as well as existing reserved/path traversal cases; never rename content silently.

## 4. Validation & Error Matrix

| Input / condition | Required behavior |
| --- | --- |
| No `bt.json` | Preserve existing environment-only behavior |
| Empty/nonregular/over-64-KiB config; malformed JSON, unknown/duplicate keys, trailing JSON, `null`, unsupported schema | Refuse native startup with sanitized Chinese error; preserve original file |
| One leading UTF-8 BOM | Accept otherwise valid config/source/password |
| Invalid UTF-8 / UTF-16 / two leading BOMs / invalid JSON | Reject; never guess encoding or replace malformed bytes |
| `managed` file mode | Explain native supported modes and Docker initialization option |
| `external` downloads without effective root | Reject with Chinese configuration error |
| Relative root, drive-relative secret reference, `../` profile escape | Reject before worker startup |
| Windows UNC/device path, ADS, upward traversal or unsafe engine component | Reject; retain existing path and length budgets |
| Windows same path with case/backslash variation | Ownership, move confirmation and publication still match |
| POSIX same spelling except case | Do not equate paths |
| External engine unavailable | Existing readiness/error state; never auto-install or stop it |

## 5. Good / Base / Bad Cases

- Good: native Windows profile with Chinese/spaced paths, BOM configuration, relative private files and same-host qB root; search result enters existing file-selection and publication flow.
- Base: missing BT file, search-only file, download-only file, independent disabled mode, explicit environment override, Docker mode ignoring the file.
- Bad: configured download root overlaps an ordinary media root; do not authorize staging or replace an unrelated media root to make startup pass.

## 6. Tests Required

- Temporary Go configuration matrix: strict fields/null, byte/encoding bounds, missing file, both sections, modes, path references, environment/legacy precedence and effective root default. Remove newly generated unit-test source after running per AGENTS.md.
- `verify_windows_bt.py --binary <desktop.exe>`: actual Windows launcher with separate CWD and Unicode profile; controlled Torznab/qB; search → torrent → file selection → publication → original bytes; restart/idempotence, independent disable, historical access, private IPC shutdown and external-engine survival; no credential leakage.
- `verify_downloads.py` with default POSIX roots and `--windows-engine-paths`: all five media, v1/v2/hybrid, filename rejection, uncertain-add recovery, ownership refusal and publication gates.
- Existing search, desktop, core-media, Go static/build and frontend checks. Build/validate a Windows package containing BT and source examples plus usage docs.
- Record real engine/peer/public-indexer, clean Windows and target NAS acceptance separately; controlled protocol fixtures cannot prove them.

## 7. Wrong vs Correct

Wrong: select path comparison using the application's OS, compare `E:/BT` and `e:\\bt\\` literally, or normalize POSIX case.

Correct: validate and identify the engine path's own format, then use `sameEnginePath` consistently at ownership, move and publication boundaries. The engine may use another OS from the application host.

Wrong: resolve `bt/qb-password` against the launch directory or add the whole download root to ordinary media roots.

Correct: resolve private references against the stable profile; authorize only `complete` and retain the existing selected-file publication gate.
