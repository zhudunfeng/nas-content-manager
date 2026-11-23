# Database snapshot contract

## 1. Scope / Trigger

Applies to `internal/app/backup*.go`, `App.backup`, and any future consumer of the common snapshot service. S03.1 implements a verified temporary **database-only** snapshot and preserves the existing download endpoint. Persistent backup roots, history, scheduling, retention, manifest APIs, recovery preparation and UI are not implemented by this slice. Local acceptance and target Linux/NAS acceptance are separate.

S03.2.2's separate package-internal root/publication capability is documented in the locally accepted [Backup root and publication contract](./backup-publish-contract.md); target Linux/NAS evidence remains pending. It borrows this same source lease without reopening/transferring/closing it; adding the helper alone does not expose a persistent execution API or replace the existing download lifecycle.

## 2. Signatures

```go
func (a *App) createDatabaseSnapshot(ctx context.Context) (*databaseSnapshot, error)
func (snapshot *databaseSnapshot) Close() error
func verifyDatabaseSnapshot(ctx context.Context, path string, file *os.File, expected *snapshotExpected) (snapshotMetadata, error)
func snapshotReadOnlyURI(path string) (string, error)
func stableSnapshotFile(path string, file *os.File, expected os.FileInfo) (os.FileInfo, error)
func snapshotBudget(minimum uint64, sourceSize int64) (uint64, error)
func snapshotPageBytes(count int64, size int64) (int64, error)
```

- Current database head is `currentDatabaseSchemaVersion = 6`; the checker accepts exactly this head. S03.2.1 added the five-table storage and explicit offline-account compatibility described in [Backup storage contract](./backup-storage-contract.md). Fixed historical migration versions remain unchanged. A later head change must explicitly review offline account support; changing this constant does not authorize additional offline versions.
- `snapshotMetadata`: actual `SchemaVersion int`, `SizeBytes int64`, lowercase `SHA256 string`, `CreatedAt time.Time` in UTC, and `Scope string` fixed to `database-only`. CreatedAt means generation/verification completed, not the exact SQLite transaction instant.
- `snapshotExpected`: positive `SizeBytes int64` and decoded `[sha256.Size]byte SHA256`. It has **no schema field**. Nil computes facts without claiming comparison with a trusted manifest.
- `GET /api/backup` retains the existing administrator authentication and `http.ServeContent`; native HEAD still generates a new snapshot. There is no new route, request JSON or environment key.

## 3. Contracts

- `App.backupMu.TryLock` owns generation, validation, HTTP delivery, file close and cleanup as one lease. Contention returns immediately; a slow download is still busy. The caller owns only the lease, not the candidate path, and must close it once logically; `sync.Once` makes concurrent/repeated Close return the same cleanup result. Cleanup failure still releases the mutex.
- Create a random private `.backup-pending-*` directory inside canonical DataDir with the sole allowed member `snapshot.db`. Use controlled `os.Root` handles, Lstat/Stat/SameFile checks and exact ownership. Reject symlinks, all Windows reparse points, replacement identities, unexpected disappearance and unknown members. Read at most **two** directory entries: a second member already proves rejection. Never enumerate an attacker-sized directory just to establish this bound.
- Before generation, require `free(DataDir) >= MinFreeBytes + 2*S`, where `S=max(main-file size, checked page_count*page_size)`. Reject negative sizes, invalid pages and multiplication/addition overflow. This is a precheck, not reserved capacity; later FULL/IOERR must still fail safely. Never delete old material to make room.
- Execute exactly one context-aware `VACUUM INTO ?`. Do not copy the active WAL main file, change live journal_mode or wrap generation/file I/O in a long business transaction. The single SQLite connection can make concurrent application SQL wait; this is not a parallel-write guarantee.
- Restrict candidate permissions, Sync and Close the write handle, then open read-only. POSIX directories/files use 0700/0600. Windows uses a protected DACL for the current token user and SYSTEM, with directory inheritance; chmod is not equivalent. Permission, Sync and Close errors are failures.
- Validate only an already completed, isolated stable single-file candidate, using a correctly escaped absolute local `file:` URI with `mode=ro&immutable=1`, one database connection and one fixed connection. Never call normal database initialization, migration, application recovery, checkpoint or cache cleanup. Immutable is not a way to ignore a live WAL.
- Require the exact SQLite header, current actual user_version, valid page geometry with page_count*page_size equal to file length, 16 real core tables and LIMIT 0 probes for core/account columns. The five backup tables must be STRICT ordinary main-schema tables; `pragma_table_xinfo` must show exactly the expected name/type sets and hidden=0 for every column. Require exactly one id=1 policy row and one id=1 store row. This narrow probe does not establish equivalence of every DDL constraint/index/trigger. `integrity_check(1)` must yield exactly one `ok`; `foreign_key_check` must yield zero rows. Query, Scan, iteration and Close errors fail validation. Do not read secret values for structure probes.
- Close SQL rows/connection/database before hashing the same read-only file that will be served. SHA-256 uses a fixed 64 KiB buffer, checks cancellation each iteration and requires exact length; expected length/digest must match when supplied. Recheck path/handle identity, size and mtime before/after, reset the cursor, then recheck cancellation. Never allocate a database-sized buffer.
- Close removes only the exact unchanged owned candidate, then the empty unchanged directory. Unknown/replaced objects are preserved. Close failures are aggregated into `App.cleanupErr` and logged using fixed Chinese text, safe stage and random candidate basename; no SQL diagnostics, secret absolute paths, credentials or raw engine/source URLs.
- Request context follows existing application shutdown. Cancel/close connections, wait for handlers/workers, then close DB/data lease. No detached backup goroutine or new global media write timeout. A crash can leave a temporary candidate; this slice does not sweep or adopt it.
- Successful response uses `application/vnd.sqlite3` and `attachment; filename=nas-library-YYYYMMDD-HHMMSS.db` using existing local-time naming. After headers, never append JSON or claim that handler completion proves the client received every byte.
- The DB itself contains sensitive indexes, password/session hashes and private references. Database-only excludes **external** source-secret files, helper/worker identity files, component volumes, download payloads, media and user originals; it does not mean the DB is public, encrypted or a complete disaster-recovery copy. Keep it private. No offline verification or session-revoking recovery CLI is delivered here.

## 4. Validation & Error Matrix

| Condition before response headers | Result |
| --- | --- |
| Missing/expired session | Existing 401 authentication response |
| Snapshot lease already owned | 409, `已有备份正在生成或下载，请稍后重试` |
| Source/creation/generation/open failure | Fixed Chinese 500; no internal diagnostic exposure |
| Free-space query failure, insufficient precheck or unrepresentable budget | 507, `剩余空间不足，无法生成一致性备份` |
| Candidate schema/structure/integrity/FK/length/hash validation fails | 500, `备份校验失败，请稍后重试` |
| Candidate identity/member set unsafe | Fixed Chinese 500; preserve uncertain objects |
| Request already cancelled | Release resources; no promise of an observable HTTP status |
| Close/cleanup fails after headers | Safe Chinese log and cleanupErr; do not append JSON or change completed bytes |

## 5. Good / Base / Bad Cases

- Good: real concurrent favorites/progress commits and a snapshot produce a consistent SQLite point, complete integrity/FK checks and an independently matching digest; SQL may wait during VACUUM.
- Base: GET or HEAD owns the lease until delivery/Close ends. A second request returns 409; cancelling the first releases the lease and a later request can succeed.
- Bad: old/future schema, fake current structure, bad page, truncation, trailing bytes, FK violation or wrong expected digest/size is rejected read-only, without sidecars or source changes.
- Bad: replace the owned member/directory or add 2,048 unknown members. Reject and preserve them, with bounded member enumeration rather than unbounded allocation.

## 6. Tests Required

- Temporary Go tests: barrier contention, idempotent Close, failed cleanup unlocking, budget/page arithmetic edges, actual in-progress generation/check cancellation and fixed-buffer hashing cancellation; permission/Sync/Close/remove/free-space injections; identity replacement and bounded unknown-member enumeration. Execute and remove only newly generated temporary test sources.
- Real SQLite read-only matrix: current/old/future/fake structure, missing core columns, corruption/truncation/tail, valid integrity with invalid FK, wrong expected size/digest. Verify unchanged bytes/size/mtime/member set and absence of sidecars. Do not treat an empty post-cleanup `go test` suite as behavioral evidence.
- Keep `scripts/verify_backup.py --binary <explicit-artifact> --root <fresh-.runtime-root>` as reusable integration coverage: exact isolated readiness, real slow HTTP and 409, stream cancellation/retry, independent schema/size/hash, transactional personal state, original-log secret checks before sanitization, genuine graceful shutdown and immediate restart.
- On Windows, normal shutdown evidence must use an owned Console CTRL_C_EVENT or actual RunWithOptions cancellation. Job/process-tree reclamation is only fallback cleanup. Record fallback use and cleanup failures accurately.
- gofmt/vet, temporary behavior tests, clean-source package compilation, fresh Windows binary, backup/core/account regressions and affected type checks; record one artifact hash for integration results. Linux cross-builds, real Linux/NAS runs, actual FULL/unmount/ACL and large-database performance remain distinct evidence classes. See `docs/verification.md` and S03's independent review.

## 7. Wrong vs Correct

Wrong: `os.ReadFile(library.db)`, openDatabase(candidate), `ReadDir(-1)` followed by a member-count check, RemoveAll on a prefix, or serving a candidate before its SQL resources close and digest completes.

Correct: obtain the shared lease through `createDatabaseSnapshot`, verify the completed isolated candidate read-only, stream the same handle through ServeContent, and defer its idempotent Close. A directory with more than one member needs only `ReadDir(2)` to fail closed; preserve unknown objects instead of recursively deleting them.
