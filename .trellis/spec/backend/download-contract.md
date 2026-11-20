# Download runtime contract

## 1. Scope / Trigger

Applies to optional BT downloads, `download*.go`, download mounts/secrets, schema v2, all scan/read paths, and `scripts/verify_downloads.py`. Preserve the base runtime contract and the five content types.

## 2. Signatures

- `loadDownloadConfig(config Config) (DownloadConfig, error)` validates server-only configuration.
- `(*App).startDownloads()` creates the independent coordinator under `App.ctx`/`App.wg`; disabled mode performs no engine calls.
- `(*App).downloadRoutes(mux *http.ServeMux)` registers authenticated same-origin routes documented in `docs/api.md`.
- `schemaV2` adds `downloads`, `download_files`, `download_operations` transactionally after v1. Current applications then apply collection schema v3; preserve original tables and reject `user_version > 3`. See [Collection contract](./collection-contract.md).
- `(*App).loadDownloadGate(ctx context.Context) (*downloadGate,error)`, `(*downloadGate).allow(path string,isDir bool) (bool,error)`, `(*App).checkDownloadPath(path string,isDir bool) error` enforce publication.
- `checkDownloadMountIsolation(config DownloadConfig,mediaRoots []string) error` rejects Linux mount aliases exposing staging or the entire root through ordinary media mounts.

## 3. Contracts

- `NAS_DOWNLOAD_ENABLED=false` by default. `NAS_DOWNLOAD_ROOT=/downloads` is application-side, `NAS_DOWNLOAD_ENGINE_ROOT=/downloads` is engine-side. `NAS_DOWNLOAD_QB_URL`, `NAS_DOWNLOAD_QB_USERNAME`, `NAS_DOWNLOAD_QB_PASSWORD_FILE` are server-only. `NAS_DOWNLOAD_MIN_FREE_GB=2` is an independent download reserve. Every configuration field must have a Chinese explanation.
- qB owns BT traffic, piece verification, recovery and seeding. Go never proxies the payload or reads/hashes an entire video. The browser only calls the existing app origin. Keep engine errors sanitized and separate from app-session 401.
- Login accepts observed HTTP 200 or 204 only with a nonempty accepted session cookie and an empty or `Ok.` body; reject `Fails.`, other status codes and missing sessions. Accept legacy `SID` or `QBT_SID_<valid numeric port>`, preserve the actual name and value together, and return that name on subsequent calls. Reverse proxy and engine ports can differ. Reject arbitrary cookie names and clear/back off invalid sessions; never log values. A real probe must exercise the production Go adapter, not only a separate cookie-jar client.
- qB receives server-generated `staging/<UUID>`, then asynchronous `setLocation` to `complete/<type>/<UUID>`. Do not assume different mount names imply the same filesystem, instant rename, or zero-copy migration.
- Ordinary JSON is 64 KiB. Only the exact torrent multipart POST allows 8 MiB + 64 KiB envelope, and the exact selection PUT allows 256 KiB. Torrent content is 8 MiB maximum, file manifests are 20,000 maximum. Keep all other body/response/allocation budgets bounded.
- Persist add/action command intents before network calls. Never hold a SQLite transaction or `App.mu` across network operations. A sent/uncertain add must be reconciled by ownership tag, controlled path and algorithm identity; never blindly repeat it. Existing external hashes must not be adopted.
- A manifest transaction must first win its conditional task revision update before inserting file rows; zero affected rows rolls back. A late binding result must not delete the private source unless its conditional binding update actually succeeds.
- Treat engine IDs as opaque; v1 SHA-1 and v2 SHA-256 are distinct, hybrid aliases point to one local task. File priorities use actual engine indices. Persist immutable selected manifest/version and optimistic command revisions.
- At first binding, fetch properties to establish complete algorithm identities, including hybrid aliases. Metadata-in-progress or missing full v2 identity stays pending; an engine's short opaque hash plus the submitted digest is not independent identity evidence.
- Magnet metadata safety is version-specific. Enable only versions verified by the recorded real-engine probe; unsupported versions receive a clear error, never an automatic full-payload fallback. Do not infer real behavior from a fake engine.
- Publish only after selected files complete, stable engine state, correct final location, symlink/path/regular-file/size validation. Persist selected file nanosecond `mod_time` and ready atomically. Revalidate published versions; freeze changed or rewriting tasks. Engine offline alone does not revoke confirmed media.
- Freeze a ready task before metadata/checking/moving/pending-command early returns and on an empty manifest or priorities differing from the locked selection; later branches must not bypass publication revocation.
- Every manual/periodic/restart scan, directory comic count/page/cover, direct source and cached cover must respect the gate. Directories expose only selected descendants; files match exact selected paths and published size/mtime. Archived tasks retain manifests; disabling the coordinator retains historical roots and the gate.
- Resolve lexical paths, directory identity aliases, and Linux mountinfo device/root mappings. A bind of a deep staging directory is still staging. Missing mount information fails closed. Keep ordinary media source `os.OpenRoot` boundaries intact.
- Load manifest entries lazily per accessed task and cache directory alias checks per scan, so a Range request does not materialize every historical file. Close DB rows before filesystem work or nested queries; SQLite has one connection.
- During missing cleanup, protect blocked/frozen download items while allowing ordinary missing media to retain existing semantics. A failed/empty mount must not erase old indexes or personal state.
- Import state is independent of transfer/seeding. Verify actual target item coverage; an already running scan may have passed the new folder. Reuse unique active scans, arrange bounded follow-up scanning, and offer independent import retry. Deleted target libraries are not silently recreated.
- Removal always uses `deleteFiles=false`, archives records and preserves published manifests. No source deletion, arbitrary paths, engine preferences takeover, external task adoption or RSS. Optional manual Torznab search follows [Torrent search contract](torrent-search-contract.md); it must enter this same managed add/selection workflow.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing app session / foreign mutation origin | Existing 401 / 403 |
| Disabled module / unavailable engine for write | Explicit Chinese 503; status/local snapshots remain readable |
| Engine login fails | Sanitized module diagnostic; do not log out the app |
| Malformed bencode/magnet, unsafe path, oversize manifest | Bounded 400/413; no engine submission |
| Duplicate request identity | Reuse existing local task |
| External same hash / stale revision or selection version | Conflict; do not modify external task |
| Unknown staging/final task, unselected file, changed size/mtime | No scan/read/cover access |
| Move accepted but still moving/checking | Keep ready false and wait for reconciliation |
| Low free space | Pause only managed payload, preserve files/progress |
| No importable type / removed target / failed scan | Distinct import status and recovery action |
| Downgrade to a v1 application | Restore a matching v1 snapshot; do not edit user_version manually |

## 5. Good / Base / Bad Cases

- Good: metadata stops before normal transfer, confirm sparse actual file IDs, selected files finish, qB moves asynchronously, app verifies and imports while seeding continues.
- Base: default disabled app passes all five media workflows and exact 5+ GiB Range; v1 upgrade preserves IDs, favorites, tags and progress.
- Bad: qB add times out, complete directory appears before move ends, a previously selected file changes, another mount exposes staging, or an old scan finishes without new items. Reconcile, freeze or retry; never guess publication.

## 6. Tests Required

- Real isolated engine probe: version/API/auth, stopped add, magnet metadata stop with observed payload bytes, actual file index priority, selected transfer, asynchronous location, removal preserving files. Record official binary/image provenance separately; no Docker commands when user reserves Docker validation.
- Reusable fake-engine/app integration: auth/origin, size/path/parser limits, v1/v2/hybrid identity, deduplication/external conflict, command uncertainty, selection/limits/actions, transfer vs import, publication gate, five-type imports, restart/retry.
- Temporary Go regressions: gate staging/unknown/unselected/archived/disabled/changed versions, deep mount mapping, additive v1→v2 preservation, command races, scanning coverage/missing retention. Execute then remove only generated test files per AGENTS.md; retain integration harnesses.
- `go vet ./...`, `go test ./...`, production backend/frontend build, `scripts/verify.py --large-file`, responsive actual UI verification. Empty final test packages prove compilation only, not behavior.

## 7. Wrong vs Correct

```go
// 错误：进度与 API 成功不是发布证据，普通扫描仍可能读到暂存文件。
if torrent.Progress == 1 { enqueueScan(libraryID) }

// 正确：协调器确认最终目录与文件基线，再持久化 ready；每个读取入口仍检查门禁。
if err := a.checkDownloadPath(sourcePath, item.IsDir); err != nil {
    return nil, err
}
```

The gate is a security boundary, not an optimization in the completion callback. Preserve native Range streaming after checks and document actual executed evidence separately from target NAS performance acceptance.
