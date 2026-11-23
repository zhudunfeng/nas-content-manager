# Backup storage contract

## 1. Scope / Trigger

Applies to `internal/app/backup_store.go`, normal database migrations, the current snapshot structure probe, and offline-account compatibility when the database head changes. S03.2.1 adds storage only: five STRICT tables, defaults, current-head verification and explicit offline support. It does not create a persistent backup root or implement policy APIs, an executor, scheduling, history pruning or retention. The policy remains disabled. Local Windows execution, Linux cross-builds and actual Linux/NAS acceptance are separate evidence classes.

## 2. Signatures

```go
func migrateBackupSchema(ctx context.Context, tx *sql.Tx) error
func migrateBackupSchemaWithSources(ctx context.Context, tx *sql.Tx, clock func() time.Time, entropy io.Reader) error
func checkSnapshotBackupStructure(ctx context.Context, conn *sql.Conn) error
func supportsOfflineAccountSchema(version int64) bool
```

- `currentDatabaseSchemaVersion = 6`. Fixed historical V1–V5 definitions, including V5 session constraints, remain unchanged. New raw `schemaV6` is the authoritative executable DDL.
- Normal startup at version `<6` creates the five tables/default rows and sets `user_version=6` in the same short migration transaction. Any conflict or error rolls back that increment; do not adopt an unknown preexisting object with `IF NOT EXISTS`.
- Offline reset independently supports the explicit set `{5,6}`. The current snapshot checker accepts exactly head 6; this is not historical snapshot verification or recovery preparation support.

## 3. Contracts

| Table | Required storage contract |
| --- | --- |
| `backup_policy` | Singleton `id=1`; positive revisions; enabled 0/1; UTC hour 0..23/minute 0..59; keep 1..30; max bytes 67,108,864..1,099,511,627,776; disabled requires NULL next-run and enabled requires a next-run. Defaults: revision/schedule_revision=1, disabled, 03:00 UTC, keep 7, max 10,737,418,240, cursor/next NULL. |
| `backup_store` | Singleton `id=1`; unique random 32-lowerhex store ID; root_bound 0/1; nonnegative high-water/truncation counters; nullable last-good ID; last-success/failure are valid object-or-null JSON within 2,048 UTF-8 bytes. The retention authorizer ID/keep/max tuple is all NULL or all non-NULL. Defaults are unbound, zero counters, no last-good/authorization and both summaries `null`. |
| `backup_runs` | Positive AUTOINCREMENT seq and unique ID; manual UUIDv7 request versus scheduled key/slot/revision are mutually exclusive. States queued/running/completed/failed/interrupted. Queued has no started/finished/file; running has started and no finished; terminal has finished; completed also has started/file and no error. Failed/interrupted require a bounded safe code. One partial unique index permits at most one queued/running row. |
| `backup_files` | Unique run ID and positive run seq; states pending/retained/deleting/missing/unverified/deleted. Owner size 1..1,024 and SHA are always required. Retained/deleting require a publication tuple; any non-NULL published time requires verified time, positive DB size/schema, DB SHA, bounded app version, manifest size 1..16,384 and manifest SHA. Deleted alone requires deleted time. One partial unique index permits at most one deleting row. |
| `backup_request_keys` | Canonical lowercase UUIDv7 primary key; unique run ID; positive run seq; terminal 0/1; object JSON <=2,048 UTF-8 bytes. Expiry is exactly request-time +86,400,000 ms, not retry/acceptance time +24h. |

- All five tables use STRICT. IDs/digests use exact byte lengths, lowercase ASCII hex and explicit NUL rejection. Request UUIDs validate hyphens, version 7, variant and the exact alphabet. Schedule keys are 1..96 bytes from `[0-9s:]`; error codes 1..64 bytes from `[a-z0-9_]`; messages are at most 256 UTF-8 bytes. Exact schedule-key semantics and UUID embedded-time equality remain future service responsibilities.
- UTC millisecond timestamps are INTEGER in 0..253402300799999; nullable timestamps have the same bound when present. Request time reserves the 24h addition range. Do not add ordering CHECKs such as finished>=started: wall clocks may move backwards.
- Sample the migration clock once as N and use it for both effective/updated policy timestamps; initialize high-water to 0. Read exactly 16 cryptographic random bytes for store ID. Head-6 restart must preserve identity, policy and authorization instead of reseeding them.
- The injected clock/reader seam tests returnable invalid-time, EOF and short-read rollback. Production uses standard `crypto/rand.Reader`; the pinned Go 1.27.4 default can terminate the runtime on a low-level entropy failure. Returnable-reader tests do not prove that fatal is recoverable or that process-crash recovery was dynamically exercised. Do not replace the standard RNG or mutate its global reader for this purpose.
- There are no new cross-table foreign keys, cascades, capacity triggers or generic CRUD framework. The later service must enforce run/file/key ownership, CAS, monotonic high-water, overflow rejection and the 200-run/64-owned-file/2,048-valid-key limits. These capacities are not delivered by the migration.
- Pending/unverified may lack a publication tuple. Exact cleanup of an unpublished owned remainder may go directly to deleted. Deleting is reserved for future rotation of a completed published backup; it is not the state for arbitrary pending cleanup. Last-good is not retention authorization, and NULL authorization must never be reopened merely from old history.
- Snapshot verification requires 16 real core tables, five backup tables in `main` with `type=table` and `strict=1`, their exact field-name/declaration-type sets from `pragma_table_xinfo`, every column `hidden=0`, and exactly one `id=1` row in each singleton. Query/Scan/iteration/Close errors fail closed. This narrow probe does not claim equivalence of every CHECK/index/trigger definition and does not read secret values or migrate the snapshot. See [Database snapshot contract](./backup-contract.md).
- Offline preflight retains a separate `userVersion int64` as well as SQLite schema cookie and administrator ID/hash/revision. The write transaction requires the actual supported user version to equal the preflight version; 5→6 or 6→5 drift is rejected even with an unchanged schema cookie. Successful reset changes only the account hash/revision/sessions, preserving nonempty backup rows and pending filesystem sentinels. It does not read policy, prepare a root or reconcile backup state. See [Account lifecycle](./account-contract.md).

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Future head >6 | Refuse normal startup; no downgrade |
| V6 object collision, invalid clock or returnable entropy failure | Roll back all V6 objects/defaults/head change |
| Bad types/ranges/UTF-8 byte budgets/NUL/ID/UUID/authorization tuple | SQLite rejects the statement |
| Second active run or second deleting file | Partial unique index rejects the statement |
| Queued file, completed without start/file, or incomplete published tuple | Reject; preserve prior rows |
| Pending→unverified without publication facts, or cleaned unpublished→deleted | Valid storage representation; no claim that a service already performs it |
| Current snapshot with extra/missing/wrong-type/hidden column, non-STRICT table or missing/extra singleton | Read-only validation failure; no source mutation or sidecars |
| Offline unsupported head or supported-version drift after preflight | Refuse reset/roll back; no unrelated mutation |

## 5. Good / Base / Bad Cases

- Good: construct a genuine frozen V5 database, start the application once, preserve the old account/hash/revision/Cookie and business rows, and obtain all five V6 tables/defaults atomically.
- Base: restart a V6 database without changing store ID, disabled policy, authorization or creating `backups`.
- Good: reset an existing V5 or V6 administrator offline; a nonempty backup domain and pending sentinel remain unchanged.
- Bad: lower the version pragma of a V6 database and call it a historical V5 migration fixture. Build V1–V5 from their frozen definitions instead.
- Bad: add a VIRTUAL generated column to an otherwise valid current backup table. A table-info-only check misses it; table_xinfo and hidden=0 must reject it.

## 6. Tests Required

- Temporary Go behavior coverage: genuine upgrade/restart, exact defaults, transaction rollback, time/entropy boundary, STRICT/type/range/byte/NUL/UUID/state/tuple constraints, both partial unique indexes, all five hidden-column counterexamples, offline supported-version drift with unchanged schema cookie, and backup-domain zero side effects. Execute and precisely remove only newly generated test sources.
- Retain `scripts/verify_backup_store.py --binary <explicit-artifact> --root <fresh-.runtime-root>`: actual old-structure startup migration, original Cookie/business preservation, defaults/no root/stable ID, future-head and conflicting-object failure preservation. `scripts/verify_schema.py` creates genuine V1–V5 structures; never fabricate history by lowering a latest head.
- Fresh same-artifact storage/backup/account `--offline`/core `--large-file` regressions, original-secret checks before sanitization, truthful Console graceful-stop versus Job cleanup accounting, zero owned live processes and no source drift. Run through `scripts/project_env.py`.
- gofmt/vet, clean-source package compilation, Python AST/annotations, Vue typecheck and Linux amd64/arm64 cross-builds. Empty post-cleanup `go test` is compilation evidence only. Report passed leaf subcases separately from nested test-container events; do not sum repeat rounds or label them assertions.

## 7. Wrong vs Correct

Wrong: inspect `pragma_table_info`, count expected visible columns and declare a current snapshot structurally complete. Correct: enumerate all `pragma_table_xinfo` rows, reject any hidden flag and require exact names/types, plus STRICT and singleton checks.

Wrong: let offline support automatically follow `currentDatabaseSchemaVersion`, or accept any member of `{5,6}` after preflight. Correct: use an explicit support set and require the transactional user version to equal the retained preflight value as part of the existing schema/account CAS.

Wrong: add cross-table count triggers or fabricate executed backup rows to claim runtime capacity/rotation support. Correct: store only the disabled defaults in this slice and leave cross-resource service behavior to its separately gated implementation.
