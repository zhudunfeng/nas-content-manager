# Backup root and publication contract

## 1. Scope / Trigger

Applies to `backup_format.go`, `backup_root.go`, `backup_publish.go`, platform `backup_storage_*.go`, `Config.BackupDir` and protection-path configuration. S03.2.2 is **local_complete_target_pending** after implementation, independent writable review and mainline evidence readback. The frozen implementation authority is `.trellis/tasks/11-23-nas-backup-recovery/root-publisher-contract.md`; final findings and evidence are in `research/quality-review-s03-2-2.md` under that task.

This is a package-internal root/publication capability. It does not add public APIs, an executor, enabled policy, scheduling, persistent download leases, completion transactions or rotation. Normal disabled/unbound startup does not acquire this root or create its lock/marker. The existing synchronous `GET /api/backup` continues using [Database snapshot contract](./backup-contract.md). Schema 6 and ownership constraints follow [Backup storage contract](./backup-storage-contract.md).

## 2. Signatures

```go
// acquireBackupRoot 只在明确模式下准备并绑定专属根。
func (a *App) acquireBackupRoot(ctx context.Context, mode backupRootOpenMode) (*backupRootLease, error)
// checkIdentity 重新核对入口映射、根、锁、marker及保护配置，变化后持续阻断。
func (lease *backupRootLease) checkIdentity() error
// Close 幂等收好本租约资源并保留错误，最后释放固定锁。
func (lease *backupRootLease) Close() error
// backupOwnerBytes 在登记pending前固定规范owner原始字节。
func backupOwnerBytes(storeID string, runID string, fileID string) ([]byte, error)
// publishBackupSnapshot 借用同App根与源，返回目标事实及发布确定性。
func (a *App) publishBackupSnapshot(ctx context.Context, root *backupRootLease, source *databaseSnapshot, runID string) (backupPublishResult, error)
```

- `backupRootOpenMode` is package-private uint8. Zero means existing-bound-only; `backupRootPrepare` explicitly permits first preparation or finishing a matching final marker's bind CAS. Reject other values. An unbound zero-mode call performs no root I/O.
- `backupPublishCertainty` has zero `backupPublishNeedsReconcile`, then `backupPublishNotPublished` and `backupPublishConfirmed`. Always consume it together with error.
- `backupPublishResult` contains Certainty, Metadata (`snapshotMetadata`), actual OwnerSize/OwnerSHA256, ManifestSize/ManifestSHA256, and safe Stage/Code. It is not a run-completed or deletion-authorization result.
- Lease Close uses once-only ownership and reports all required close errors. It does not permit concurrent publication/Close, delete the fixed lock, or transfer source ownership.

## 3. Contracts

### Configuration and isolation

- The only new host key is `NAS_BACKUP_DIR` → `Config.BackupDir`; default is canonical `DataDir/backups`. It is a dedicated absolute local root, not an API-supplied path. Config loading validates static shape and fixes relative protection-path bases without probing/creating the backup root.
- Protect complete trees for MediaRoots, Downloads.Root even when disabled, Collection.ReadRoot, Engines.ConnectionDir, DataDir/collection-secrets, DataDir/download-sources, and disk WebDir candidate. Protect exact file points for helper Socket/TokenFile, download PasswordFile and Torznab SourcesFile. A file's parent is an anchor, not a blanket forbidden tree.
- Preserve SourcesFile from the same configuration lookup; disabled search does not open its secret file. Do not add a public secret-path DTO or a second environment lookup.
- Query historical collection roots with `DISTINCT local_root ... kind<>'bt' LIMIT 257`; at most 256 may be validated. Query/Rows/Close errors or excess roots refuse only backup preparation, not media/account service.
- Project missing paths through an existing ancestor and retained tail, without creating protected directories or reading secret contents. Compare path segments and physical device/root projections, including both ancestor directions; only exact default DataDir/backups is allowed inside DataDir.
- Retain the configured entry, parent and protected-path anchors; re-resolve them around critical I/O. Keep a sticky failure after identity/configuration/mapping changes, even if the path is later restored. A held old handle alone does not prove the current configured path.
- Windows rejects UNC/device/drive-relative, alternate-stream, trailing dot/space and reserved-name aliases, including COM/LPT¹²³ with extensions. Reserved-name matching also removes ASCII spaces from the stem before the first extension separator: CON .db and COM¹  .db are aliases, not ordinary names. Go can report a junction as ModeIrregular; Windows anchors let the kernel follow the original entry and derive physical volume paths from the resulting handle. Do not use EvalSymlinks as the sole Windows junction authority or discard the original mapping. Root terminal entries and members still reject reparse points. Linux uses EvalSymlinks plus O_NOFOLLOW and actual handle checks.

### Root, fixed lock and metadata

- Windows initially supports local fixed NTFS; Linux supports ext4/XFS only when bounded mountinfo, fstat device, fstatfs and required operations agree. Other filesystems/platforms return controlled unsupported. Linux cross-builds do not prove runtime support on a NAS.
- A missing root may be created only by explicit prepare, while unbound, with its immediate parent present. After root_bound=1, missing root/marker is a fault; do not create a replacement. Same marker bytes in a replacement root do not excuse runtime identity change.
- `.nas-backup.lock` is an empty regular single-link file held via `acquireInstanceFile`; never unlink it or write a PID. Cross-check its directory entry against the held file. Windows metadata verification uses a zero-data-access handle so checking the lock does not conflict with its zero-sharing exclusive lease.
- Marker `.nas-backup-store.json` has exact keys magic, formatVersion=1 and storeId; magic is `nas-content-manager-backup-store`. Owner has magic `nas-content-manager-backup-owner`, formatVersion=1 and exact storeId/runId/fileId. IDs are 32 lowercase hex characters. Each raw JSON payload is at most 1024 bytes, read using limit+1.
- Initial marker uses only `.nas-backup-store.json.tmp`, exclusive creation, required sync/close and no-overwrite promotion, followed by a short root_bound CAS. Any tmp residue is preserved; never auto-promote it. A matching final marker may permit the explicit bind CAS. Marker identity is a logical namespace, not physical-media attestation or a cross-clone global lock.
- Manifest is at most 16384 raw bytes, formatVersion=1, exact IDs, scope=`database-only`, canonical UTC source time, 1–64 byte valid version text, actual positive schema/size and lowercase SHA256. Its single file is role=`database`, name=`snapshot.db`. Fixed excluded roles, in order: source-secrets, helper-identity, worker-state, component-volumes, download-payloads, media, user-assets.
- Parse every JSON token and layer: exact case-sensitive fields, decoded duplicate-key detection, UTF-8/NUL checks, required types, decimal integer lexical rules, bounds, no trailing object and bounded shape. Never rely on Go struct decoding to reject aliases/duplicates, or re-encode content and call that the original digest.
- New roots proven empty may receive restricted permissions. An existing valid-marker root is verified only: reject inadequate permissions without resetting its inheritable DACL. On Windows, setting a directory DACL can propagate to unknown descendants; no recursive ACL repair or traversal is allowed. The fixed known lock is handled separately.
- Private permissions are protected self/SYSTEM DACL on Windows and exact effective-owner 0700 directories/0600 files on Linux. Windows may normalize a directory OICI/GA ACE into effective full-control plus OICI/INHERIT_ONLY generic-full-control ACEs; verify exact effective/inherited rights per unique SID, not an assumed two-ACE representation. Unknown subjects, extra rights/duplicate grants, unprotected DACL or wrong flags fail. Deduplicate when self is SYSTEM; branch coverage is not evidence of a real SYSTEM process.

### Publication and resource ownership

- Caller supplies unique execution ownership. A short read-only transaction verifies the same store/bound root, running run, pending file, both run/file references and sequence, no durable cancellation, and the registered raw owner size/hash. Close the transaction before filesystem I/O.
- Only `.pending-<fileId>` and `b-<fileId>` derived from the registered ID are accepted. Existing names are preserved and return reconcile-needed; this publisher neither resumes old candidates nor chooses a new ID.
- Borrow a verified source from the same App's unique S03.1 service. Never reopen, move, delete, detach or Close its source path/lease. Fixed 64 KiB copying checks context, exact length, exclusive destination, real permissions and every required Sync/Close.
- This slice checks target free space after source generation: `MinFreeBytes + source actual size + 1 MiB`, with checked arithmetic. Actual `DB+owner+manifest <= run.max_bytes`. This is not space reservation and not the source-before-generation same-volume `MinFree+3S+M` admission gate; that belongs to the executor slice.
- Independently verify target SQLite size/hash, actual schema, integrity and foreign keys, then manifest/member identity and original bytes. Explicit schema comparison is required because `snapshotExpected` contains only size/hash. Final directory has exactly owner.json, snapshot.db and manifest.json; use bounded n+1 directory reads.
- Manifest is first written exclusively as manifest.json.tmp, synced/closed, then promoted without overwrite. Close all target SQL/member/subdirectory handles before the final directory move on Windows; keep root anchors/lock held.
- Windows files request WRITE_THROUGH and explicit Sync/Close; marker/manifest/directory moves use only MoveFileExW with MOVEFILE_WRITE_THROUGH. No REPLACE_EXISTING, COPY_ALLOWED, ordinary rename or copy/remove fallback. Do not imitate POSIX directory fsync and ignore an unsupported error.
- Linux uses Renameat2 with RENAME_NOREPLACE and a live verified directory File/dirfd. Check file Sync, pending-directory Sync after manifest promotion, root Sync after final publication, and external-parent Sync for a new root. Unsupported primitives, collisions, cross-device or sync errors never downgrade to overwrite/fallback.
- Reopen final and verify root/directory/members and actual content. Confirmed means the required OS protocol and subsequent checks succeeded, not a DB completion transaction or hardware power-loss guarantee.
- Once final move is attempted, an unknown move/postcheck/required Sync/Close result is reconcile-needed: preserve original ID/material, do not remove final or regenerate source. Only proven no prior/possible final publication permits not-published. A later observed cancellation does not erase an already confirmed fact.
- Cleanup is exact, owned and identity-checked: remove known members individually, owner last, empty candidate last. Preserve unknown/replaced members and all unknown publication results. Never RemoveAll. Runtime state transitions/restart reconciliation are later work.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Disabled/unbound ordinary startup or existing-only request | No root creation/lock/marker; existing-only returns unbound failure |
| Static malformed NAS_BACKUP_DIR | Config rejection with safe Chinese explanation |
| Root offline, unsupported FS, protected path/257th history root, query error | Controlled backup-domain failure; preserve media/account/old export behavior |
| Bound root/marker missing, wrong store, changed anchor/config/marker identity | Refuse and retain evidence; runtime mapping failure is sticky |
| Marker tmp, name collision, unknown directory member | Preserve material; no auto-adoption, overwrite or alternate fileId |
| Inadequate existing-root DACL | Refuse without changing root/unknown-subtree ACLs or content |
| Bad intent/owner/source App, malformed JSON, budget/overflow | No confirmed result; only proved-owned prepublication cleanup |
| Required flush/close/no-overwrite move/postcheck failure | Never confirmed; after attempted final move preserve same-ID facts for reconciliation |
| Source Close fails after caller obtained confirmed | Resource warning remains separate from confirmed target; no blind retry |

Errors/logs use fixed Chinese stage and nonsecret code; do not expose raw paths, SQL, URLs or underlying exception text. Return value certainty and cleanup failures are separate facts.

## 5. Good / Base / Bad Cases

- Good: explicit first preparation of an isolated root, private marker/lock and confirmed bind; borrow one real source and produce a verified exact three-member final directory without changing run completion state.
- Base: normal startup with an offline configured backup root still serves existing media/account/export paths and creates no persistent backup root.
- Good: follow a parent junction into a local physical directory, then reject a runtime remap to another directory even with identical marker bytes; restoring the mapping does not reset the failure.
- Good: move an unprotected unknown subtree into an already marked root; acquisition only verifies that root, retaining actual subtree security-descriptor and content hashes. An invalid root is also rejected without ACL repair.
- Bad: accept a valid hash as proof that an attempted final move was durably confirmed, delete an uncertain final, or regenerate with another ID.
- Bad: treat a schema-compatible completed binary's public-API regression as proof that the unexposed publisher is wired into production. Internal behavior tests and API compatibility are separate evidence.

## 6. Tests Required

- Independent temporary Go behavior for root modes/CAS/marker residue/lock/Close, directory versus file-point isolation, missing projection/history 256–257, strict raw format/limits and n+1 enumeration. Execute then precisely remove only newly generated test sources; retained .runtime text evidence is not a compiled test source.
- Real Windows App/source/publisher with actual SQLite, schema/size/hash/manifest, budget below/exact/above and overflow, collision sentinels, handle-denied move, sync/close/move/postcheck fault boundaries, cancellation and source ownership. Enumerate certainty and preserved material for every failure stage.
- Actual junction/hardlink/guard replacement, restoration-sticky root remap and old physical lock exclusivity; actual DACL normalization/self=SYSTEM branch and unknown-subtree ACL/content preservation. A failed symlink fixture is not a passed symlink product case.
- After source cleanup, fresh same-artifact storage/old backup/account --offline/core --large-file regressions; gofmt/vet, package compilation, Python AST/annotations, Vue typecheck/build and Linux amd64/arm64 cross-builds. Empty cleaned go test is compilation only.
- Read back original events, safe-log scan-before-sanitization, exact artifact/source SHA, owned process/temporary source cleanup. Count leaf subcases, nested containers and standalone top-level cases separately; never add repeated/failed rounds together. Console graceful shutdown and Job cleanup are separate facts.
- Windows runtime, Linux cross-build, real Linux/NAS/mount/permission/FULL/IOERR/unplug, system crash, physical power loss and production-size performance remain separate acceptance tracks. A protocol fixture or injected error is not a physical device test.

Local final evidence: 24 top-level tests, 257 passed leaf subcases, zero nested containers; five standalone top-level tests are recorded separately. The same fresh Windows binary passed storage17/backup38/account162/core52. Four temporary test sources were removed after preserving exact-hash text; 193 source hashes and three artifacts were reread without drift, and exact-binary live processes were zero. Mainline readback is `.runtime/verify/s0322-mainline-closeout/evidence-readback.json`. Linux Stat/Close error aggregation was statically fixed and cross-built only; real Linux/NAS, process/system crash and physical power loss remain pending.

## 7. Wrong vs Correct

Wrong: reset an existing marked root's inheritable DACL to repair it. Correct: verify only; refuse mismatched permissions, and change root permissions only after proving it empty during first preparation.

Wrong: use EvalSymlinks alone for Windows junctions or hold only an old physical handle after dropping the original entry. Correct: derive physical identity from kernel-resolved handles and continuously recheck retained original/parent mappings.

Wrong: open the zero-sharing fixed lock with an ordinary Go read-attributes path and loosen lock sharing when verification fails. Correct: keep the exclusive original lock and use the dedicated zero-data-access identity query.

Wrong: interpret `error != nil` as proof of no published target, then delete or create a new fileId. Correct: consume Certainty together with error and preserve unknown same-ID facts for the next slice's source-free reconciliation.
