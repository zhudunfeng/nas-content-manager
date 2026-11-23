package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
)

// backupPublishCertainty 独立表达磁盘事实，错误本身不能说明是否已发布。
type backupPublishCertainty uint8

const (
	backupPublishNeedsReconcile backupPublishCertainty = iota // 零值必须保留原意图等待核对。
	backupPublishNotPublished                                 // 已证明本次最终移动尚未发生。
	backupPublishConfirmed                                    // 全部规定OS协议及最终复验已经确认。
)

// backupPublishResult 只返回目标事实，不表示DB已完成登记或允许轮转。
type backupPublishResult struct {
	Certainty      backupPublishCertainty // 需要核对、明确未发布或协议确认。
	Metadata       snapshotMetadata       // 目标实际schema、长度和摘要，时间沿用源生成事实。
	OwnerSize      int64                  // 实际owner原始字节数。
	OwnerSHA256    string                 // 实际owner原始字节摘要。
	ManifestSize   int64                  // 实际manifest原始字节数。
	ManifestSHA256 string                 // 实际manifest原始字节摘要。
	Stage          string                 // 固定中文安全阶段。
	Code           string                 // 固定非秘密分类。
}

// backupPublishIntent 只在短只读事务内取得登记的唯一归属，随后关闭事务再做磁盘I/O。
type backupPublishIntent struct {
	fileID   string // 登记的文件编号，唯一目录名来源。
	maxBytes int64  // 本run冻结的逻辑总字节上限。
	owner    []byte // 登记前已固定的规范owner原始字节。
}

// readBackupPublishIntent 在同一短事务核对store、running、pending及双向run/file关系。
func (a *App) readBackupPublishIntent(ctx context.Context, root *backupRootLease, runID string) (intent backupPublishIntent, resultErr error) {
	if !backupHex(runID, 32) || root == nil || root.app != a {
		return intent, backupFailure("登记核对", "backup_intent_invalid")
	}
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return intent, backupFailure("登记核对", "backup_intent_unavailable")
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			resultErr = errors.Join(resultErr, backupFailure("登记核对", "backup_intent_close_failed"))
		}
	}()
	var storeID, fileStore, runState, fileState, fileRun, runFile, ownerHash string
	var bound int
	var seq, fileSeq, ownerSize int64
	var cancelled sql.NullInt64
	if err = tx.QueryRowContext(ctx, "SELECT store_id,root_bound FROM backup_store WHERE id=1").Scan(&storeID, &bound); err != nil || storeID != root.storeID || bound != 1 {
		return intent, backupFailure("登记核对", "backup_intent_mismatch")
	}
	err = tx.QueryRowContext(ctx, `SELECT r.seq,r.state,r.file_id,r.max_bytes,r.cancel_requested_at_ms,f.id,f.store_id,f.run_id,f.run_seq,f.state,f.owner_size,f.owner_sha256
FROM backup_runs r JOIN backup_files f ON f.id=r.file_id WHERE r.id=?`, runID).Scan(&seq, &runState, &runFile, &intent.maxBytes, &cancelled, &intent.fileID, &fileStore, &fileRun, &fileSeq, &fileState, &ownerSize, &ownerHash)
	if err != nil || seq < 1 || seq != fileSeq || runState != "running" || fileState != "pending" || runFile != intent.fileID || fileRun != runID || fileStore != storeID || cancelled.Valid {
		return intent, backupFailure("登记核对", "backup_intent_mismatch")
	}
	intent.owner, err = backupOwnerBytes(storeID, runID, intent.fileID)
	if err != nil || int64(len(intent.owner)) != ownerSize || backupDigest(intent.owner) != ownerHash {
		return intent, backupFailure("登记核对", "backup_owner_mismatch")
	}
	return intent, nil
}

// backupTargetBudget 检查事后目标余量的加法，不冒充源生成前的整体峰值准入。
func backupTargetBudget(minimum uint64, source int64) (uint64, error) {
	if source <= 0 || uint64(source) > math.MaxUint64-(1<<20) || minimum > math.MaxUint64-uint64(source)-(1<<20) {
		return 0, backupFailure("目标预算", "backup_budget_overflow")
	}
	return minimum + uint64(source) + (1 << 20), nil
}

// backupLogicalBudget 检查DB、owner和manifest实际逻辑总量，不允许int64加法溢出。
func backupLogicalBudget(database int64, owner int64, manifest int64, maximum int64) error {
	if database <= 0 || owner <= 0 || manifest <= 0 || maximum <= 0 || owner > math.MaxInt64-database || manifest > math.MaxInt64-database-owner || database+owner+manifest > maximum {
		return backupFailure("单份预算", "backup_max_bytes")
	}
	return nil
}

// backupCandidate 只跟踪本次排他创建的目录和成员，不凭相似名字接管未知材料。
type backupCandidate struct {
	lease     *backupRootLease       // 借用的根，候选不得关闭根租约。
	name      string                 // 由已登记fileID推导的精确直接子名。
	info      os.FileInfo            // 排他创建后的目录身份，移动后仍须相同。
	root      *os.Root               // 本次持有的候选子根，最终移动前关闭。
	directory *os.File               // Linux清单无覆盖移动与目录同步句柄。
	members   map[string]os.FileInfo // 只有本次实际创建的成员才可清理。
}

// check 核对候选目录项及持有句柄，不接受替换后的相同owner文本。
func (candidate *backupCandidate) check() error {
	if candidate.lease.checkIdentity() != nil || candidate.root == nil || candidate.directory == nil {
		return backupFailure("候选身份", "backup_candidate_changed")
	}
	current, err := candidate.lease.root.Lstat(candidate.name)
	opened, e := candidate.directory.Stat()
	rootInfo, re := candidate.root.Stat(".")
	if err != nil || e != nil || re != nil || !current.IsDir() || isSnapshotReparsePoint(current) || !os.SameFile(candidate.info, current) || !os.SameFile(current, opened) || !os.SameFile(opened, rootInfo) || backupCheckPermissions(candidate.directory, true) != nil {
		return backupFailure("候选身份", "backup_candidate_changed")
	}
	return nil
}

// open 重新打开同一目录实体，Windows移动前必须先关闭所有子目录句柄。
func (candidate *backupCandidate) open() error {
	current, err := candidate.lease.root.Lstat(candidate.name)
	if err != nil || !current.IsDir() || isSnapshotReparsePoint(current) || !os.SameFile(candidate.info, current) {
		return backupFailure("候选身份", "backup_candidate_changed")
	}
	candidate.root, err = candidate.lease.root.OpenRoot(candidate.name)
	if err != nil {
		return backupFailure("候选身份", "backup_candidate_unavailable")
	}
	candidate.directory, err = candidate.root.Open(".")
	if err != nil {
		return backupFailure("候选身份", "backup_candidate_unavailable")
	}
	return candidate.check()
}

// closeHandles 关闭所有候选目录资源并保留每项失败，不把Close推迟至已确认之后。
func (candidate *backupCandidate) closeHandles() error {
	var result error
	if candidate.directory != nil {
		if candidate.lease.operations.close(candidate.directory) != nil {
			result = backupFailure("资源收尾", "backup_close_failed")
		}
		candidate.directory = nil
	}
	if candidate.root != nil {
		if candidate.root.Close() != nil {
			result = errors.Join(result, backupFailure("资源收尾", "backup_close_failed"))
		}
		candidate.root = nil
	}
	return result
}

// cleanup 只删本次身份确认的成员，owner最后、空目录最后；未知成员导致完整保留。
func (candidate *backupCandidate) cleanup() (resultErr error) {
	if candidate.info == nil {
		return nil
	}
	defer func() { resultErr = errors.Join(resultErr, candidate.closeHandles()) }()
	if candidate.root == nil {
		if err := candidate.open(); err != nil {
			return err
		}
	}
	if candidate.check() != nil || backupMembers(candidate.root, candidate.members, true) != nil {
		return backupFailure("候选清理", "backup_cleanup_preserved")
	}
	for _, name := range []string{"manifest.json.tmp", "manifest.json", "snapshot.db", "owner.json"} {
		if _, ok := candidate.members[name]; !ok {
			continue
		}
		if candidate.check() != nil || backupMembers(candidate.root, candidate.members, true) != nil || candidate.root.Remove(name) != nil {
			return backupFailure("候选清理", "backup_cleanup_preserved")
		}
		delete(candidate.members, name)
	}
	if err := candidate.closeHandles(); err != nil {
		return err
	}
	info, err := candidate.lease.root.Lstat(candidate.name)
	if err != nil || !os.SameFile(candidate.info, info) || isSnapshotReparsePoint(info) || candidate.lease.checkIdentity() != nil || candidate.lease.root.Remove(candidate.name) != nil {
		return backupFailure("候选清理", "backup_cleanup_preserved")
	}
	return nil
}

// copySnapshot 固定64KiB复制借用句柄，逐轮取消、精确长度、私密权限及Sync/Close均受检。
func (candidate *backupCandidate) copySnapshot(ctx context.Context, source *databaseSnapshot) (resultErr error) {
	if candidate.check() != nil {
		return backupFailure("复制数据库", "backup_candidate_changed")
	}
	path := filepath.Join(candidate.lease.path, candidate.name)
	file, err := backupCreateFile(candidate.root, path, "snapshot.db")
	if err != nil {
		return backupFailure("复制数据库", "backup_create_failed")
	}
	info, statErr := file.Stat()
	if statErr == nil {
		candidate.members["snapshot.db"] = info
	}
	defer func() {
		if current, e := file.Stat(); e == nil && info != nil && os.SameFile(info, current) {
			candidate.members["snapshot.db"] = current
		}
		if candidate.lease.operations.close(file) != nil {
			resultErr = errors.Join(resultErr, backupFailure("复制数据库", "backup_close_failed"))
		}
	}()
	if statErr != nil || backupCheckSingleLink(file) != nil || backupCheckPermissions(file, false) != nil {
		return backupFailure("复制数据库", "backup_permissions_failed")
	}
	if _, err = source.file.Seek(0, io.SeekStart); err != nil {
		return backupFailure("复制数据库", "backup_source_unavailable")
	}
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		if ctx.Err() != nil {
			return backupFailure("复制数据库", "backup_cancelled")
		}
		count, readErr := source.file.Read(buffer)
		if count > 0 {
			if int64(count) > source.metadata.SizeBytes-total {
				return backupFailure("复制数据库", "backup_source_changed")
			}
			written, writeErr := file.Write(buffer[:count])
			if writeErr != nil || written != count {
				return backupFailure("复制数据库", "backup_copy_failed")
			}
			total += int64(count)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil || count == 0 {
			return backupFailure("复制数据库", "backup_copy_failed")
		}
	}
	if total != source.metadata.SizeBytes {
		return backupFailure("复制数据库", "backup_source_changed")
	}
	if candidate.lease.operations.sync(file) != nil {
		return backupFailure("复制数据库", "backup_sync_failed")
	}
	return nil
}

// verify 完整复验三成员及实际SQLite事实，所有目标句柄关闭后才可返回成功。
func (candidate *backupCandidate) verify(ctx context.Context, intent backupPublishIntent, runID string, source snapshotMetadata, manifestBytes []byte) (result backupPublishResult, resultErr error) {
	if ctx.Err() != nil {
		return result, backupFailure("目标复验", "backup_cancelled")
	}
	if candidate.check() != nil || backupMembers(candidate.root, candidate.members, true) != nil || len(candidate.members) != 3 {
		return result, backupFailure("目标复验", "backup_members_unknown")
	}
	path := filepath.Join(candidate.lease.path, candidate.name)
	owner, _, err := backupReadMetadata(candidate.root, path, "owner.json", backupJSONLimit)
	if err != nil || !bytes.Equal(owner, intent.owner) {
		return result, backupFailure("目标复验", "backup_owner_mismatch")
	}
	if _, err = parseBackupOwner(owner); err != nil {
		return result, backupFailure("目标复验", "backup_owner_mismatch")
	}
	manifest, _, err := backupReadMetadata(candidate.root, path, "manifest.json", backupManifestLimit)
	if err != nil || !bytes.Equal(manifest, manifestBytes) {
		return result, backupFailure("目标复验", "backup_manifest_mismatch")
	}
	parsed, err := parseBackupManifest(manifest)
	if err != nil || parsed.StoreID != candidate.lease.storeID || parsed.RunID != runID || parsed.FileID != intent.fileID {
		return result, backupFailure("目标复验", "backup_manifest_mismatch")
	}
	file, err := candidate.root.Open("snapshot.db")
	if err != nil {
		return result, backupFailure("目标复验", "backup_database_unavailable")
	}
	defer func() {
		if candidate.lease.operations.close(file) != nil {
			resultErr = errors.Join(resultErr, backupFailure("目标复验", "backup_close_failed"))
		}
	}()
	if backupCheckSingleLink(file) != nil || backupCheckPermissions(file, false) != nil {
		return result, backupFailure("目标复验", "backup_database_identity")
	}
	var digest [sha256.Size]byte
	decoded, e := hex.DecodeString(source.SHA256)
	if e != nil || len(decoded) != len(digest) {
		return result, backupFailure("目标复验", "backup_source_invalid")
	}
	copy(digest[:], decoded)
	metadata, err := verifyDatabaseSnapshot(ctx, filepath.Join(path, "snapshot.db"), file, &snapshotExpected{source.SizeBytes, digest})
	if err != nil || metadata.SchemaVersion != source.SchemaVersion {
		return result, backupFailure("目标复验", "backup_database_mismatch")
	}
	if _, err = stableSnapshotFile(filepath.Join(path, "snapshot.db"), file, candidate.members["snapshot.db"]); err != nil || candidate.check() != nil {
		return result, backupFailure("目标复验", "backup_member_changed")
	}
	metadata.CreatedAt = source.CreatedAt
	if backupLogicalBudget(metadata.SizeBytes, int64(len(owner)), int64(len(manifest)), intent.maxBytes) != nil {
		return result, backupFailure("目标复验", "backup_max_bytes")
	}
	result.Metadata = metadata
	result.OwnerSize = int64(len(owner))
	result.OwnerSHA256 = backupDigest(owner)
	result.ManifestSize = int64(len(manifest))
	result.ManifestSHA256 = backupDigest(manifest)
	return result, nil
}

// checkBorrowedSnapshot 只核对借用的源句柄和租约，不重开、不转移源文件所有权。
func checkBorrowedSnapshot(source *databaseSnapshot) error {
	if source == nil || source.file == nil || source.info == nil || source.metadata.SizeBytes <= 0 || source.metadata.SchemaVersion <= 0 || source.metadata.Scope != "database-only" || !backupHex(source.metadata.SHA256, 64) || source.checkDirectory() != nil {
		return backupFailure("源核对", "backup_source_invalid")
	}
	if _, err := source.checkMembers(); err != nil {
		return backupFailure("源核对", "backup_source_changed")
	}
	if _, err := stableSnapshotFile(filepath.Join(source.app.config.DataDir, source.directory, "snapshot.db"), source.file, source.info); err != nil {
		return backupFailure("源核对", "backup_source_changed")
	}
	return nil
}

// publishBackupSnapshot 借用源和根，返回磁盘事实，不完成DB登记、last-good或轮转授权。
func (a *App) publishBackupSnapshot(ctx context.Context, root *backupRootLease, source *databaseSnapshot, runID string) (result backupPublishResult, resultErr error) {
	result.Stage = "登记核对"
	result.Code = "backup_needs_reconcile"
	var candidate *backupCandidate
	moveAttempted := false
	defer func() {
		if candidate != nil {
			if resultErr != nil && !moveAttempted {
				resultErr = errors.Join(resultErr, candidate.cleanup())
			} else {
				resultErr = errors.Join(resultErr, candidate.closeHandles())
			}
		}
		if resultErr != nil {
			var safe *backupStorageError
			if errors.As(resultErr, &safe) {
				result.Stage = safe.stage
				result.Code = safe.code
			}
			a.logBackupFailure(resultErr)
		}
	}()
	intent, err := a.readBackupPublishIntent(ctx, root, runID)
	if err != nil {
		return result, err
	}
	if root.checkIdentity() != nil {
		return result, backupFailure("根身份", "backup_root_changed")
	}
	if source == nil || source.app != a || checkBorrowedSnapshot(source) != nil {
		return result, backupFailure("源核对", "backup_source_invalid")
	}
	pending, final := ".pending-"+intent.fileID, "b-"+intent.fileID
	for _, name := range []string{pending, final} {
		if _, err = root.root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			return result, backupFailure("目录碰撞", "backup_collision_needs_reconcile")
		}
	}
	// 只有原意图没有既有目标且尚未调用最终移动，才明确标记本次未发布。
	result.Certainty = backupPublishNotPublished
	manifest, err := backupManifestBytes(root.storeID, runID, intent.fileID, source.metadata)
	if err != nil {
		return result, backupFailure("单份预算", "backup_source_invalid")
	}
	if backupLogicalBudget(source.metadata.SizeBytes, int64(len(intent.owner)), int64(len(manifest)), intent.maxBytes) != nil {
		return result, backupFailure("单份预算", "backup_max_bytes")
	}
	budget, err := backupTargetBudget(a.config.MinFreeBytes, source.metadata.SizeBytes)
	if err != nil {
		return result, err
	}
	free, err := root.operations.free(root.path)
	if err != nil || free < budget {
		return result, backupFailure("目标预算", "backup_space_unavailable")
	}
	if ctx.Err() != nil {
		return result, backupFailure("创建候选", "backup_cancelled")
	}
	if root.checkIdentity() != nil {
		return result, backupFailure("根身份", "backup_root_changed")
	}
	if err = root.root.Mkdir(pending, 0700); err != nil {
		result.Certainty = backupPublishNeedsReconcile
		return result, backupFailure("创建候选", "backup_create_collision")
	}
	candidate = &backupCandidate{lease: root, name: pending, members: make(map[string]os.FileInfo)}
	candidate.info, err = root.root.Lstat(pending)
	if err != nil || !candidate.info.IsDir() || isSnapshotReparsePoint(candidate.info) {
		return result, backupFailure("创建候选", "backup_candidate_changed")
	}
	if restrictSnapshotPath(filepath.Join(root.path, pending), true) != nil {
		return result, backupFailure("创建候选", "backup_permissions_failed")
	}
	if err = candidate.open(); err != nil {
		return result, err
	}
	info, err := root.writeFile(candidate.root, filepath.Join(root.path, pending), "owner.json", intent.owner)
	if info != nil {
		candidate.members["owner.json"] = info
	}
	if err != nil {
		return result, err
	}
	if err = candidate.copySnapshot(ctx, source); err != nil {
		return result, err
	}
	if checkBorrowedSnapshot(source) != nil {
		return result, backupFailure("源核对", "backup_source_changed")
	}
	// 先独立检查目标数据库，再以目标实测schema/size/hash生成清单。
	file, err := candidate.root.Open("snapshot.db")
	if err != nil {
		return result, backupFailure("数据库复验", "backup_database_unavailable")
	}
	var digest [sha256.Size]byte
	decoded, _ := hex.DecodeString(source.metadata.SHA256)
	copy(digest[:], decoded)
	metadata, verifyErr := verifyDatabaseSnapshot(ctx, filepath.Join(root.path, pending, "snapshot.db"), file, &snapshotExpected{source.metadata.SizeBytes, digest})
	closeErr := root.operations.close(file)
	if verifyErr != nil || closeErr != nil || metadata.SchemaVersion != source.metadata.SchemaVersion {
		return result, backupFailure("数据库复验", "backup_database_mismatch")
	}
	metadata.CreatedAt = source.metadata.CreatedAt
	manifest, err = backupManifestBytes(root.storeID, runID, intent.fileID, metadata)
	if err != nil || backupLogicalBudget(metadata.SizeBytes, int64(len(intent.owner)), int64(len(manifest)), intent.maxBytes) != nil {
		return result, backupFailure("单份预算", "backup_max_bytes")
	}
	info, err = root.writeFile(candidate.root, filepath.Join(root.path, pending), "manifest.json.tmp", manifest)
	if info != nil {
		candidate.members["manifest.json.tmp"] = info
	}
	if err != nil {
		return result, err
	}
	if candidate.check() != nil || ctx.Err() != nil {
		return result, backupFailure("清单发布", "backup_cancelled_or_changed")
	}
	if err = root.operations.move(candidate.directory, filepath.Join(root.path, pending), "manifest.json.tmp", "manifest.json"); err != nil {
		return result, backupFailure("清单发布", "backup_manifest_move_failed")
	}
	delete(candidate.members, "manifest.json.tmp")
	candidate.members["manifest.json"] = info
	if root.operations.syncDir(candidate.directory) != nil {
		return result, backupFailure("清单发布", "backup_sync_failed")
	}
	verified, err := candidate.verify(ctx, intent, runID, metadata, manifest)
	if err != nil {
		return result, err
	}
	if err = candidate.closeHandles(); err != nil {
		return result, err
	}
	if ctx.Err() != nil {
		return result, backupFailure("目录发布", "backup_cancelled")
	}
	if root.checkIdentity() != nil {
		return result, backupFailure("根身份", "backup_root_changed")
	}
	// 最终移动一旦调用，后续任何失败都保留原ID和材料，不能清理final或盲重试。
	moveAttempted = true
	result.Certainty = backupPublishNeedsReconcile
	if root.operations.move(root.directory, root.path, pending, final) != nil {
		return result, backupFailure("目录发布", "backup_publish_needs_reconcile")
	}
	candidate.name = final
	if root.operations.syncDir(root.directory) != nil {
		return result, backupFailure("目录发布", "backup_publish_needs_reconcile")
	}
	if err = candidate.open(); err != nil {
		return result, err
	}
	verified, err = candidate.verify(ctx, intent, runID, metadata, manifest)
	if err != nil {
		return result, err
	}
	if err = candidate.closeHandles(); err != nil {
		return result, err
	}
	if root.checkIdentity() != nil {
		return result, backupFailure("最终核对", "backup_publish_needs_reconcile")
	}
	result = verified
	result.Certainty = backupPublishConfirmed
	result.Stage = "发布确认"
	result.Code = "backup_published"
	if a.logger != nil {
		a.logger.Info("数据库备份目标已完成规定发布协议，等待执行器登记", "runId", runID, "fileId", intent.fileID, "bytes", metadata.SizeBytes)
	}
	return result, nil
}
