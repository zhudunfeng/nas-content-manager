package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

// backupRootOpenMode 防止零值或Web输入隐式接管未绑定的根。
type backupRootOpenMode uint8

const (
	backupRootExisting backupRootOpenMode = iota // 零值仅打开已经绑定的根。
	backupRootPrepare                            // 明确允许首次准备或匹配最终marker后补CAS。
)

// backupStorageError 仅携带固定安全阶段与分类，绝不包装系统路径或SQL异常。
type backupStorageError struct {
	stage string // 固定中文业务阶段。
	code  string // 固定非秘密分类。
}

// Error 返回可安全记录的固定中文错误。
func (e *backupStorageError) Error() string {
	return "备份存储操作未完成：" + e.stage + "（" + e.code + "）"
}

// backupFailure 集中生成受控错误；调用方不得传入原始错误或动态路径。
func backupFailure(stage string, code string) error { return &backupStorageError{stage, code} }

// backupIOOperations 为一次租约固定系统原语，供包内故障验证，不提供生产开关。
type backupIOOperations struct {
	free    func(string) (uint64, error)                 // 读取实际目标卷剩余容量。
	sync    func(*os.File) error                         // 对写入文件逐项同步。
	close   func(*os.File) error                         // 逐项关闭并保留关闭失败。
	move    func(*os.File, string, string, string) error // 同一目录句柄内无覆盖移动。
	syncDir func(*os.File) error                         // 平台规定的目录同步，Windows采用文档边界。
	bind    func(context.Context, *sql.DB, string) error // 短事务首次绑定及提交未知复核。
}

// defaultBackupIOOperations 返回每次调用私有的默认原语集合。
func defaultBackupIOOperations() backupIOOperations {
	return backupIOOperations{freeDiskBytes, (*os.File).Sync, (*os.File).Close, backupMoveNoReplace, backupSyncDirectory, bindBackupRoot}
}

// backupPathSpec 区分整树和精确文件点，父目录不会被误升级为整树。
type backupPathSpec struct {
	path string // 同次配置快照内的绝对路径。
	tree bool   // 真为双向目录树保护；假为精确文件点。
}

// backupPathAnchor 保留现存祖先句柄及缺失尾部的物理投影，关键I/O前重新核对。
type backupPathAnchor struct {
	path     string      // 需要重核的原始绝对路径。
	existing string      // 已实际解析的现存祖先。
	suffix   []string    // 尚未创建的路径分段，顺序从叶到根。
	file     *os.File    // 生命周期内持有的现存祖先句柄。
	info     os.FileInfo // 首次实际身份。
	device   string      // 平台内设备标识，仅用于比较、不输出日志。
	physical string      // 设备内规范投影路径，不输出日志。
}

// backupRootLease 唯一持有根和保护映射，发生替换后持续阻断，最后才释放OS锁。
type backupRootLease struct {
	app         *App                // 固定配置、数据库与安全中文日志所有者。
	ctx         context.Context     // 明确准备的上下文，不启动脱离生命周期的任务。
	path        string              // 受控根绝对路径，不进入业务日志。
	storeID     string              // 数据库命名空间。
	root        *os.Root            // 受控根句柄。
	directory   *os.File            // 平台移动及同步使用的真实目录句柄。
	anchor      *backupPathAnchor   // 根身份及原配置路径映射。
	parent      *backupPathAnchor   // 现存外层父目录，覆盖首次创建协议。
	guards      []*backupPathAnchor // DataDir和所有保护路径的必要句柄。
	specs       []backupPathSpec    // 首次保护集合，历史集合变化会阻断旧租约。
	lock        *os.File            // 固定空文件上的OS独占租约。
	lockInfo    os.FileInfo         // 固定锁目录项首次身份。
	markerInfo  os.FileInfo         // 最终marker首次身份、长度和mtime。
	markerBytes []byte              // 有界原始marker，不含秘密路径。
	operations  backupIOOperations  // 本次固定使用的文件及绑定原语。
	blocked     bool                // 任一身份/保护映射变化后不允许热切换。
	once        sync.Once           // Close幂等保护；不许可Close与发布并发。
	closeErr    error               // 首次资源关闭结果。
}

// normalizeBackupConfig 只校验静态形状并固定相对路径基准，不探测备用根。
func normalizeBackupConfig(config *Config) error {
	if config.BackupDir == "" {
		config.BackupDir = filepath.Join(config.DataDir, "backups")
	}
	if !backupPathSyntax(config.BackupDir) {
		return fmt.Errorf("NAS_BACKUP_DIR 必须是无路径别名的本地绝对目录")
	}
	config.BackupDir = filepath.Clean(config.BackupDir)
	// 原模块允许相对文件和Web目录；仍按加载时工作目录解释，并固定到本次快照。
	for _, target := range []*string{&config.WebDir, &config.Downloads.PasswordFile, &config.TorrentSearch.SourcesFile} {
		if *target != "" {
			absolute, err := filepath.Abs(*target)
			if err != nil || strings.ContainsRune(*target, 0) {
				return fmt.Errorf("备份保护路径配置无效")
			}
			*target = filepath.Clean(absolute)
		}
	}
	if backupContains(config.BackupDir, config.DataDir) || backupContains(config.DataDir, config.BackupDir) && !backupPathEqual(config.BackupDir, filepath.Join(config.DataDir, "backups")) {
		return fmt.Errorf("NAS_BACKUP_DIR 与数据目录隔离不正确")
	}
	return nil
}

// backupContains 按平台大小写和分段判断祖先关系，不使用原始字符串前缀。
func backupContains(parent string, child string) bool {
	parent, child = backupPathKey(parent), backupPathKey(child)
	relative, err := filepath.Rel(parent, child)
	return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}

// backupPathEqual 比较已清理的同平台路径分段。
func backupPathEqual(left string, right string) bool {
	return backupPathKey(left) == backupPathKey(right)
}

// resolveBackupAnchor 只沿现存祖先解析缺失尾部，不创建缺失保护目录或读取秘密正文。
func resolveBackupAnchor(path string) (result *backupPathAnchor, resultErr error) {
	if path == "" || !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return nil, backupFailure("路径隔离", "backup_path_unavailable")
	}
	anchor := &backupPathAnchor{path: filepath.Clean(path)}
	defer func() {
		if result == nil && anchor.file != nil && anchor.file.Close() != nil {
			resultErr = errors.Join(resultErr, backupFailure("路径隔离", "backup_close_failed"))
		}
	}()
	current := anchor.path
	for {
		info, err := os.Lstat(current)
		if err == nil {
			resolved, e := backupResolveAnchorPath(current)
			if e != nil {
				return nil, backupFailure("路径隔离", "backup_path_unavailable")
			}
			if len(anchor.suffix) > 0 && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 && !isSnapshotReparsePoint(info) {
				return nil, backupFailure("路径隔离", "backup_path_unavailable")
			}
			anchor.existing = resolved
			anchor.file, e = backupOpenAnchor(resolved)
			if e != nil {
				return nil, backupFailure("路径隔离", "backup_path_unavailable")
			}
			anchor.info, e = anchor.file.Stat()
			actual, statErr := os.Stat(resolved)
			if e != nil || statErr != nil || !os.SameFile(anchor.info, actual) || len(anchor.suffix) > 0 && !anchor.info.IsDir() {
				return nil, backupFailure("路径隔离", "backup_path_unavailable")
			}
			anchor.device, anchor.physical, e = backupPhysicalPath(anchor.file, resolved)
			if e != nil {
				return nil, backupFailure("路径隔离", "backup_path_unavailable")
			}
			for i := len(anchor.suffix) - 1; i >= 0; i-- {
				anchor.physical = filepath.Join(anchor.physical, anchor.suffix[i])
			}
			return anchor, nil
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(current) == current {
			return nil, backupFailure("路径隔离", "backup_path_unavailable")
		}
		anchor.suffix = append(anchor.suffix, filepath.Base(current))
		current = filepath.Dir(current)
	}
}

// check 重新解析原始父链，映射或祖先实体改变就拒绝使用陈旧结论。
func (anchor *backupPathAnchor) check() (resultErr error) {
	current, err := resolveBackupAnchor(anchor.path)
	if err != nil {
		return err
	}
	defer func() {
		if current.file.Close() != nil {
			resultErr = errors.Join(resultErr, backupFailure("路径隔离", "backup_close_failed"))
		}
	}()
	if !os.SameFile(anchor.info, current.info) || anchor.device != current.device || !backupPathEqual(anchor.physical, current.physical) || !reflect.DeepEqual(anchor.suffix, current.suffix) {
		return backupFailure("路径隔离", "backup_mapping_changed")
	}
	return nil
}

// backupProtectionPaths 有界读取仍授权的完整历史根，所有Rows及Close失败均拒绝准备。
func (a *App) backupProtectionPaths(ctx context.Context) ([]backupPathSpec, error) {
	c := a.config
	specs := []backupPathSpec{{c.DataDir, true}}
	for _, path := range append(append([]string{}, c.MediaRoots...), c.Downloads.Root, c.Collection.ReadRoot, c.Engines.ConnectionDir, filepath.Join(c.DataDir, "collection-secrets"), filepath.Join(c.DataDir, "download-sources"), c.WebDir) {
		if path != "" {
			specs = append(specs, backupPathSpec{path, true})
		}
	}
	for _, path := range []string{c.Engines.Socket, c.Collection.TokenFile, c.Downloads.PasswordFile, c.TorrentSearch.SourcesFile} {
		if path != "" {
			specs = append(specs, backupPathSpec{path, false})
		}
	}
	rows, err := a.db.QueryContext(ctx, "SELECT DISTINCT local_root FROM collection_tasks WHERE local_root<>'' AND kind<>'bt' ORDER BY local_root LIMIT 257")
	if err != nil {
		return nil, backupFailure("历史隔离", "backup_history_unavailable")
	}
	count := 0
	for rows.Next() {
		var path string
		if err = rows.Scan(&path); err != nil {
			break
		}
		count++
		if count > 256 {
			err = errors.New("历史根超出预算")
			break
		}
		specs = append(specs, backupPathSpec{path, true})
	}
	if errors.Join(err, rows.Err(), rows.Close()) != nil {
		return nil, backupFailure("历史隔离", "backup_history_unavailable")
	}
	return specs, nil
}

// checkBackupIsolation 核对设备内双向目录关系与精确文件点，并保留全部实际anchor。
func checkBackupIsolation(root *backupPathAnchor, specs []backupPathSpec, dataDir string) (guards []*backupPathAnchor, resultErr error) {
	defer func() {
		if resultErr != nil {
			for _, guard := range guards {
				if guard.file.Close() != nil {
					resultErr = errors.Join(resultErr, backupFailure("路径隔离", "backup_close_failed"))
				}
			}
			guards = nil
		}
	}()
	for i, spec := range specs {
		guard, err := resolveBackupAnchor(spec.path)
		if err != nil {
			return guards, err
		}
		guards = append(guards, guard)
		if !spec.tree && len(guard.suffix) == 0 && guard.info.Mode().IsRegular() && backupCheckSingleLink(guard.file) != nil {
			return guards, backupFailure("路径隔离", "backup_file_alias")
		}
		if root.device != guard.device {
			continue
		}
		contains := backupContains(root.physical, guard.physical)
		reverse := backupContains(guard.physical, root.physical)
		if i == 0 {
			// DataDir只对精确backups兄弟材料开放例外，其它保护项仍逐个检查。
			if contains || reverse && !backupPathEqual(root.physical, filepath.Join(guard.physical, "backups")) {
				return guards, backupFailure("路径隔离", "backup_path_overlap")
			}
		} else if contains || spec.tree && reverse {
			return guards, backupFailure("路径隔离", "backup_path_overlap")
		}
	}
	return guards, nil
}

// backupMembers 最多读取允许成员数加一；未知项永远不能触发无界枚举。
func backupMembers(root *os.Root, allowed map[string]os.FileInfo, exact bool) error {
	file, err := root.Open(".")
	if err != nil {
		return backupFailure("成员核对", "backup_members_unavailable")
	}
	entries, readErr := file.ReadDir(len(allowed) + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	closeErr := file.Close()
	if errors.Join(readErr, closeErr) != nil || len(entries) > len(allowed) || exact && len(entries) != len(allowed) {
		return backupFailure("成员核对", "backup_members_unknown")
	}
	for _, entry := range entries {
		expected, ok := allowed[entry.Name()]
		if !ok {
			return backupFailure("成员核对", "backup_members_unknown")
		}
		var info os.FileInfo
		var e error
		if entry.Name() == backupLockName {
			info, e = backupStatLock(root, filepath.Join(root.Name(), backupLockName))
		} else {
			info, e = root.Lstat(entry.Name())
		}
		if e != nil || !info.Mode().IsRegular() || isSnapshotReparsePoint(info) || expected != nil && (!os.SameFile(expected, info) || expected.Size() != info.Size() || !expected.ModTime().Equal(info.ModTime())) {
			return backupFailure("成员核对", "backup_member_changed")
		}
	}
	return nil
}

// backupReadMetadata 以limit+1读入小元数据，核对普通单链接、私密权限和前后身份。
func backupReadMetadata(root *os.Root, path string, name string, limit int) (data []byte, info os.FileInfo, resultErr error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, backupFailure("元数据核对", "backup_metadata_unavailable")
	}
	defer func() {
		if file.Close() != nil {
			resultErr = errors.Join(resultErr, backupFailure("元数据核对", "backup_close_failed"))
		}
	}()
	info, err = stableSnapshotFile(filepath.Join(path, name), file, nil)
	if err != nil || info.Size() <= 0 || info.Size() > int64(limit) || backupCheckSingleLink(file) != nil || backupCheckPermissions(file, false) != nil {
		return nil, nil, backupFailure("元数据核对", "backup_metadata_identity")
	}
	data, err = io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit || int64(len(data)) != info.Size() {
		return nil, nil, backupFailure("元数据核对", "backup_metadata_unavailable")
	}
	if _, err = stableSnapshotFile(filepath.Join(path, name), file, info); err != nil {
		return nil, nil, backupFailure("元数据核对", "backup_metadata_identity")
	}
	return data, info, nil
}

// bindBackupRoot 用短CAS事务绑定；提交异常后重读事实，不把未知解释为未写。
func bindBackupRoot(ctx context.Context, db *sql.DB, storeID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return backupFailure("根绑定", "backup_binding_unavailable")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE backup_store SET root_bound=1 WHERE id=1 AND store_id=? AND root_bound=0", storeID)
	if err != nil {
		return backupFailure("根绑定", "backup_binding_failed")
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return backupFailure("根绑定", "backup_binding_changed")
	}
	if err = tx.Commit(); err == nil {
		return nil
	}
	var actual string
	var bound int
	if db.QueryRowContext(ctx, "SELECT store_id,root_bound FROM backup_store WHERE id=1").Scan(&actual, &bound) != nil || actual != storeID {
		return backupFailure("根绑定", "backup_binding_needs_reconcile")
	}
	if bound == 1 {
		return nil
	}
	return backupFailure("根绑定", "backup_binding_failed")
}

// acquireBackupRoot 默认不被启动调用，只有明确内部请求才会准备专属根。
func (a *App) acquireBackupRoot(ctx context.Context, mode backupRootOpenMode) (*backupRootLease, error) {
	return a.acquireBackupRootWithOperations(ctx, mode, defaultBackupIOOperations())
}

// acquireBackupRootWithOperations 将根、权限、marker与短CAS串成可验证的单一协议。
func (a *App) acquireBackupRootWithOperations(ctx context.Context, mode backupRootOpenMode, operations backupIOOperations) (result *backupRootLease, resultErr error) {
	if mode != backupRootExisting && mode != backupRootPrepare {
		return nil, backupFailure("准备根", "backup_mode_invalid")
	}
	if ctx.Err() != nil {
		return nil, backupFailure("准备根", "backup_cancelled")
	}
	lease := &backupRootLease{app: a, ctx: ctx, operations: operations}
	defer func() {
		if result == nil {
			resultErr = errors.Join(resultErr, lease.Close())
		}
		if resultErr != nil {
			a.logBackupFailure(resultErr)
		}
	}()
	var bound int
	if a.db.QueryRowContext(ctx, "SELECT store_id,root_bound FROM backup_store WHERE id=1").Scan(&lease.storeID, &bound) != nil || !backupHex(lease.storeID, 32) || bound < 0 || bound > 1 {
		return nil, backupFailure("准备根", "backup_store_unavailable")
	}
	if bound == 0 && mode == backupRootExisting {
		return nil, backupFailure("准备根", "backup_root_not_bound")
	}
	path := a.config.BackupDir
	if path == "" {
		path = filepath.Join(a.config.DataDir, "backups")
	}
	if !backupPathSyntax(path) {
		return nil, backupFailure("准备根", "backup_path_invalid")
	}
	lease.path = filepath.Clean(path)
	var err error
	lease.anchor, err = resolveBackupAnchor(lease.path)
	if err != nil {
		return nil, err
	}
	lease.specs, err = a.backupProtectionPaths(ctx)
	if err != nil {
		return nil, err
	}
	lease.guards, err = checkBackupIsolation(lease.anchor, lease.specs, a.config.DataDir)
	if err != nil {
		return nil, err
	}
	lease.parent, err = resolveBackupAnchor(filepath.Dir(lease.path))
	if err != nil || len(lease.parent.suffix) != 0 || !lease.parent.info.IsDir() {
		return nil, backupFailure("准备根", "backup_parent_unavailable")
	}
	if len(lease.anchor.suffix) > 0 {
		if bound == 1 || mode != backupRootPrepare || len(lease.anchor.suffix) != 1 {
			return nil, backupFailure("准备根", "backup_root_missing")
		}
		if backupCheckFilesystem(lease.parent.file, lease.parent.existing) != nil {
			return nil, backupFailure("准备根", "backup_storage_unsupported")
		}
		if lease.parent.check() != nil {
			return nil, backupFailure("准备根", "backup_parent_changed")
		}
		if err = os.Mkdir(lease.path, 0700); err != nil {
			return nil, backupFailure("准备根", "backup_create_failed")
		}
		if err = restrictSnapshotPath(lease.path, true); err != nil {
			return nil, backupFailure("准备根", "backup_permissions_failed")
		}
		if err = operations.syncDir(lease.parent.file); err != nil {
			return nil, backupFailure("准备根", "backup_sync_failed")
		}
		if operations.close(lease.anchor.file) != nil {
			lease.anchor = nil
			return nil, backupFailure("准备根", "backup_close_failed")
		}
		lease.anchor, err = resolveBackupAnchor(lease.path)
		if err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(lease.path)
	if err != nil || !info.IsDir() || isSnapshotReparsePoint(info) || !os.SameFile(info, lease.anchor.info) {
		return nil, backupFailure("准备根", "backup_root_identity")
	}
	lease.root, err = os.OpenRoot(lease.path)
	if err != nil {
		return nil, backupFailure("准备根", "backup_root_unavailable")
	}
	lease.directory, err = lease.root.Open(".")
	if err != nil {
		return nil, backupFailure("准备根", "backup_root_unavailable")
	}
	if err = backupCheckFilesystem(lease.directory, lease.anchor.existing); err != nil {
		return nil, backupFailure("准备根", "backup_storage_unsupported")
	}
	lease.lock, err = acquireInstanceFile(filepath.Join(lease.path, backupLockName))
	if err != nil {
		return nil, backupFailure("根锁", "backup_lock_unavailable")
	}
	lease.lockInfo, err = lease.lock.Stat()
	if err != nil || !lease.lockInfo.Mode().IsRegular() || lease.lockInfo.Size() != 0 || backupCheckSingleLink(lease.lock) != nil || lease.checkRootAndLock() != nil {
		return nil, backupFailure("根锁", "backup_lock_identity")
	}
	data, markerInfo, markerErr := backupReadMetadata(lease.root, lease.path, backupMarkerName, backupJSONLimit)
	if markerErr == nil {
		marker, parseErr := parseBackupMarker(data)
		if parseErr != nil || marker.StoreID != lease.storeID {
			return nil, backupFailure("根标记", "backup_marker_mismatch")
		}
		lease.markerBytes = data
		lease.markerInfo = markerInfo
	} else {
		if _, e := lease.root.Lstat(backupMarkerName); !errors.Is(e, os.ErrNotExist) || bound == 1 {
			return nil, backupFailure("根标记", "backup_marker_unavailable")
		}
		if _, e := lease.root.Lstat(backupMarkerTemp); !errors.Is(e, os.ErrNotExist) {
			return nil, backupFailure("根标记", "backup_initialization_needs_reconcile")
		}
		if err = backupMembers(lease.root, map[string]os.FileInfo{backupLockName: lease.lockInfo}, true); err != nil {
			return nil, err
		}
	}
	// 只有已证空根可设置继承DACL；合法marker根可能包含未知子树，重设会触发Windows自动传播。
	if lease.markerInfo == nil && restrictSnapshotPath(lease.path, true) != nil {
		return nil, backupFailure("根权限", "backup_root_permissions_failed")
	}
	if backupCheckPermissions(lease.directory, true) != nil {
		return nil, backupFailure("根权限", "backup_root_permissions_mismatch")
	}
	if restrictSnapshotPath(filepath.Join(lease.path, backupLockName), false) != nil {
		return nil, backupFailure("根权限", "backup_lock_permissions_failed")
	}
	if backupCheckPermissions(lease.lock, false) != nil {
		return nil, backupFailure("根权限", "backup_lock_permissions_mismatch")
	}
	if lease.markerInfo == nil {
		data, err = json.Marshal(backupMarker{backupMarkerMagic, 1, lease.storeID})
		if err != nil {
			return nil, backupFailure("根标记", "backup_marker_invalid")
		}
		if _, err = lease.writeFile(lease.root, lease.path, backupMarkerTemp, data); err != nil {
			return nil, err
		}
		if lease.checkRootAndLock() != nil {
			return nil, backupFailure("根标记", "backup_root_identity")
		}
		if operations.move(lease.directory, lease.path, backupMarkerTemp, backupMarkerName) != nil || operations.syncDir(lease.directory) != nil {
			return nil, backupFailure("根标记", "backup_initialization_needs_reconcile")
		}
		actual, actualInfo, e := backupReadMetadata(lease.root, lease.path, backupMarkerName, backupJSONLimit)
		if e != nil || !bytes.Equal(data, actual) {
			return nil, backupFailure("根标记", "backup_initialization_needs_reconcile")
		}
		lease.markerBytes = actual
		lease.markerInfo = actualInfo
	}
	if err = lease.checkIdentity(); err != nil {
		return nil, err
	}
	if bound == 0 {
		if err = operations.bind(ctx, a.db, lease.storeID); err != nil {
			return nil, err
		}
		if err = lease.checkIdentity(); err != nil {
			return nil, err
		}
	}
	if a.logger != nil {
		a.logger.Info("专属备份根已核对并取得独占租约")
	}
	return lease, nil
}

// checkRootAndLock 核对持有目录、路径、固定锁及实际平台能力，锁文件永不删除。
func (lease *backupRootLease) checkRootAndLock() error {
	if lease.root == nil || lease.anchor == nil || lease.lock == nil || lease.directory == nil || lease.anchor.check() != nil || lease.parent.check() != nil {
		return backupFailure("根身份", "backup_root_changed")
	}
	pathInfo, err := os.Lstat(lease.path)
	opened, e := lease.directory.Stat()
	rootInfo, re := lease.root.Stat(".")
	if err != nil || e != nil || re != nil || isSnapshotReparsePoint(pathInfo) || !pathInfo.IsDir() || !os.SameFile(lease.anchor.info, pathInfo) || !os.SameFile(pathInfo, opened) || !os.SameFile(opened, rootInfo) || backupCheckFilesystem(lease.directory, lease.anchor.existing) != nil {
		return backupFailure("根身份", "backup_root_changed")
	}
	lockInfo, err := backupStatLock(lease.root, filepath.Join(lease.path, backupLockName))
	lockOpened, e := lease.lock.Stat()
	if err != nil || e != nil || isSnapshotReparsePoint(lockInfo) || !lockInfo.Mode().IsRegular() || lockInfo.Size() != 0 || !os.SameFile(lease.lockInfo, lockInfo) || !os.SameFile(lockInfo, lockOpened) || backupCheckSingleLink(lease.lock) != nil {
		return backupFailure("根身份", "backup_lock_identity")
	}
	return nil
}

// checkIdentity 任一替换或映射变化后持续阻断旧租约，不热切换同marker的新根。
func (lease *backupRootLease) checkIdentity() error {
	if lease.blocked {
		return backupFailure("根身份", "backup_root_blocked")
	}
	failure := func(err error) error { lease.blocked = true; return err }
	if lease.checkRootAndLock() != nil {
		return failure(backupFailure("根身份", "backup_root_changed"))
	}
	if backupCheckPermissions(lease.directory, true) != nil || backupCheckPermissions(lease.lock, false) != nil {
		return failure(backupFailure("根身份", "backup_permissions_changed"))
	}
	for _, guard := range lease.guards {
		if guard.check() != nil {
			return failure(backupFailure("路径隔离", "backup_mapping_changed"))
		}
	}
	specs, err := lease.app.backupProtectionPaths(lease.ctx)
	if err != nil || !reflect.DeepEqual(specs, lease.specs) {
		return failure(backupFailure("历史隔离", "backup_mapping_changed"))
	}
	data, info, err := backupReadMetadata(lease.root, lease.path, backupMarkerName, backupJSONLimit)
	if err != nil || lease.markerInfo == nil || !os.SameFile(info, lease.markerInfo) || info.Size() != lease.markerInfo.Size() || !info.ModTime().Equal(lease.markerInfo.ModTime()) || !bytes.Equal(data, lease.markerBytes) {
		return failure(backupFailure("根身份", "backup_marker_changed"))
	}
	return nil
}

// writeFile 排他私密创建有界元数据，逐项Sync/Close，不掩盖任何关闭失败。
func (lease *backupRootLease) writeFile(root *os.Root, path string, name string, data []byte) (os.FileInfo, error) {
	file, err := backupCreateFile(root, path, name)
	if err != nil {
		return nil, backupFailure("文件写入", "backup_create_failed")
	}
	info, statErr := file.Stat()
	permissionErr := backupCheckPermissions(file, false)
	var count int
	var writeErr error
	if errors.Join(statErr, permissionErr) == nil {
		count, writeErr = file.Write(data)
	}
	if count != len(data) && writeErr == nil {
		writeErr = io.ErrShortWrite
	}
	var syncErr error
	if errors.Join(statErr, permissionErr, writeErr) == nil {
		syncErr = lease.operations.sync(file)
	}
	if current, e := file.Stat(); e == nil {
		info = current
	} else {
		statErr = errors.Join(statErr, e)
	}
	closeErr := lease.operations.close(file)
	if errors.Join(statErr, permissionErr, writeErr, syncErr, closeErr) != nil {
		return info, backupFailure("文件写入", "backup_write_sync_close_failed")
	}
	actual, e := root.Lstat(name)
	if e != nil || info == nil || !os.SameFile(info, actual) || isSnapshotReparsePoint(actual) || !actual.Mode().IsRegular() {
		return info, backupFailure("文件写入", "backup_member_changed")
	}
	return actual, nil
}

// logBackupFailure 只输出固定中文阶段与受控分类，不输出底层异常和宿主路径。
func (a *App) logBackupFailure(err error) {
	if a.logger == nil {
		return
	}
	var safe *backupStorageError
	if errors.As(err, &safe) {
		a.logger.Warn("持久备份操作未完成，材料已按归属保留", "stage", safe.stage, "code", safe.code)
	} else {
		a.logger.Warn("持久备份资源收尾未完成", "stage", "资源收尾", "code", "backup_cleanup_failed")
	}
}

// Close 幂等关闭全部根及保护句柄，合并错误并最后释放固定OS锁。
func (lease *backupRootLease) Close() error {
	lease.once.Do(func() {
		lease.blocked = true
		closeFile := func(file *os.File) {
			if file != nil && lease.operations.close(file) != nil {
				lease.closeErr = errors.Join(lease.closeErr, backupFailure("资源收尾", "backup_close_failed"))
			}
		}
		if lease.root != nil && lease.root.Close() != nil {
			lease.closeErr = errors.Join(lease.closeErr, backupFailure("资源收尾", "backup_close_failed"))
		}
		closeFile(lease.directory)
		if lease.anchor != nil {
			closeFile(lease.anchor.file)
		}
		if lease.parent != nil {
			closeFile(lease.parent.file)
		}
		for _, guard := range lease.guards {
			closeFile(guard.file)
		}
		closeFile(lease.lock)
	})
	return lease.closeErr
}
