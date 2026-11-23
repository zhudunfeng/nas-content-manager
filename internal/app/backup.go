package app

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// snapshotError 只携带固定安全分类，不能包装数据库诊断或私密路径。
type snapshotError struct {
	status int    // 旧下载接口保持的 HTTP 状态码。
	text   string // 可公开的固定中文提示。
	stage  string // 不含路径、SQL 或秘密的阶段标识。
}

// Error 返回已固定的中文提示。
func (e *snapshotError) Error() string { return e.text }

// errSnapshotBusy 表示同一服务已有生成、校验、下载或清理租约。
var errSnapshotBusy = &snapshotError{http.StatusConflict, "已有备份正在生成或下载，请稍后重试", "busy"}

// snapshotFileOperations 隔离少量文件收尾原语，便于包内故障验证且不引入运行时开关。
type snapshotFileOperations struct {
	restrict func(string, bool) error     // 对生成目录和文件设置真实私有权限。
	sync     func(*os.File) error         // 将候选文件内容同步到目标文件系统。
	close    func(*os.File) error         // 关闭候选文件并报告真实关闭错误。
	remove   func(*os.Root, string) error // 仅从受控目录句柄删除精确成员。
	free     func(string) (uint64, error) // 查询实际目标卷的可用字节。
}

// defaultSnapshotFileOperations 返回本次调用独立使用的系统原语，不修改全局行为。
func defaultSnapshotFileOperations() snapshotFileOperations {
	return snapshotFileOperations{
		restrict: restrictSnapshotPath,
		sync:     (*os.File).Sync,
		close:    (*os.File).Close,
		remove:   (*os.Root).Remove,
		free:     freeDiskBytes,
	}
}

// databaseSnapshot 独占本次候选、只读文件及共享互斥，不能脱离租约保留文件。
type databaseSnapshot struct {
	file       *os.File               // 同一只读句柄供摘要和 HTTP 流式读取。
	info       os.FileInfo            // 校验成功后固定的文件事实。
	metadata   snapshotMetadata       // 实际检查所得的非秘密元数据。
	parent     *os.Root               // canonical 数据根的目录句柄。
	root       *os.Root               // 本次私有临时目录的句柄。
	directory  string                 // 父目录内随机生成的单个目录名。
	dirInfo    os.FileInfo            // 创建时确认的临时目录身份。
	memberInfo os.FileInfo            // 生成结束后确认的候选文件身份。
	operations snapshotFileOperations // 本次固定使用的文件原语。
	once       sync.Once              // 并发和重复 Close 只能执行一次收尾。
	closeErr   error                  // 首次收尾结果，后续调用原样返回。
	app        *App                   // 唯一互斥所有者及安全收尾错误汇总。
}

// snapshotBudget 对负数、乘法和加法溢出失败封闭，不把预检当成空间预留。
func snapshotBudget(minimum uint64, sourceSize int64) (uint64, error) {
	if sourceSize < 0 || uint64(sourceSize) > (math.MaxUint64-minimum)/2 {
		return 0, errors.New("备份空间预算超出支持范围")
	}
	return minimum + uint64(sourceSize)*2, nil
}

// snapshotPageBytes 校验 SQLite 页参数并计算可表示的实际逻辑长度。
func snapshotPageBytes(count int64, size int64) (int64, error) {
	if count <= 0 || size < 512 || size > 65536 || size&(size-1) != 0 || count > math.MaxInt64/size {
		return 0, errors.New("备份数据库页尺寸无效")
	}
	return count * size, nil
}

// createDatabaseSnapshot 取得共用互斥，失败清理，成功将唯一所有权交给租约。
func (a *App) createDatabaseSnapshot(ctx context.Context) (*databaseSnapshot, error) {
	return a.createDatabaseSnapshotWithOperations(ctx, defaultSnapshotFileOperations())
}

// createDatabaseSnapshotWithOperations 用独立文件原语生成一次同步快照，不另起后台任务。
func (a *App) createDatabaseSnapshotWithOperations(ctx context.Context, operations snapshotFileOperations) (result *databaseSnapshot, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !a.backupMu.TryLock() {
		return nil, errSnapshotBusy
	}
	snapshot := &databaseSnapshot{app: a, operations: operations}
	defer func() {
		if result == nil {
			_ = snapshot.Close() // Close 自行记录并汇总失败，始终释放互斥。
		}
	}()
	failure := func(status int, text string, stage string) (*databaseSnapshot, error) {
		if a.logger != nil {
			a.logger.Warn("一致性备份未生成", "stage", stage)
		}
		return nil, &snapshotError{status, text, stage}
	}
	parent, err := os.OpenRoot(a.config.DataDir)
	if err != nil {
		return failure(500, "无法读取数据库状态", "source")
	}
	snapshot.parent = parent
	source, err := parent.Lstat("library.db")
	if err != nil || !source.Mode().IsRegular() || isSnapshotReparsePoint(source) || source.Size() < 0 {
		return failure(500, "无法读取数据库状态", "source")
	}
	var count, pageSize int64
	if err = a.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&count); err != nil {
		return failure(500, "无法读取数据库状态", "source-pages")
	}
	if err = a.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return failure(500, "无法读取数据库状态", "source-pages")
	}
	logicalSize, err := snapshotPageBytes(count, pageSize)
	if err != nil {
		return failure(507, "剩余空间不足，无法生成一致性备份", "budget")
	}
	budget, err := snapshotBudget(a.config.MinFreeBytes, max(source.Size(), logicalSize))
	if err != nil {
		return failure(507, "剩余空间不足，无法生成一致性备份", "budget")
	}
	free, err := operations.free(a.config.DataDir)
	if err != nil || free < budget {
		return failure(507, "剩余空间不足，无法生成一致性备份", "free-space")
	}
	path, err := os.MkdirTemp(a.config.DataDir, ".backup-pending-")
	if err != nil {
		return failure(500, "无法创建备份文件", "create-directory")
	}
	snapshot.directory = filepath.Base(path)
	snapshot.dirInfo, err = parent.Lstat(snapshot.directory)
	if err != nil || !snapshot.dirInfo.IsDir() || isSnapshotReparsePoint(snapshot.dirInfo) {
		return failure(500, "无法创建备份文件", "directory-identity")
	}
	snapshot.root, err = parent.OpenRoot(snapshot.directory)
	if err != nil {
		return failure(500, "无法创建备份文件", "open-directory")
	}
	if err = operations.restrict(path, true); err != nil {
		return failure(500, "无法创建备份文件", "directory-permissions")
	}
	if err = snapshot.checkDirectory(); err != nil {
		return failure(500, "无法创建备份文件", "directory-identity")
	}
	if _, err = snapshot.root.Lstat("snapshot.db"); !errors.Is(err, os.ErrNotExist) {
		return failure(500, "无法创建备份文件", "candidate-exists")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	// VACUUM 自己建立一致性时点；源库单连接可能使其它 SQL 等待，不外包业务事务。
	_, vacuumErr := a.db.ExecContext(ctx, "VACUUM INTO ?", filepath.Join(path, "snapshot.db"))
	member, memberErr := snapshot.root.Lstat("snapshot.db")
	if memberErr == nil && member.Mode().IsRegular() && !isSnapshotReparsePoint(member) {
		snapshot.memberInfo = member
	}
	if vacuumErr != nil {
		return failure(500, "数据库备份失败，请稍后重试", "vacuum")
	}
	if snapshot.memberInfo == nil || snapshot.checkDirectory() != nil {
		return failure(500, "备份状态不可用", "candidate-identity")
	}
	writable, err := snapshot.root.OpenFile("snapshot.db", os.O_RDWR, 0)
	if err != nil {
		return failure(500, "无法读取备份文件", "open-candidate")
	}
	info, statErr := writable.Stat()
	if statErr != nil || !os.SameFile(member, info) {
		if closeErr := operations.close(writable); closeErr != nil {
			snapshot.closeErr = errors.New("备份写入文件关闭失败")
		}
		return failure(500, "备份状态不可用", "candidate-identity")
	}
	permissionErr := operations.restrict(filepath.Join(path, "snapshot.db"), false)
	var syncErr error
	if permissionErr == nil {
		syncErr = operations.sync(writable)
	}
	closeErr := operations.close(writable)
	if closeErr != nil {
		snapshot.closeErr = errors.New("备份写入文件关闭失败")
	}
	if errors.Join(permissionErr, syncErr, closeErr) != nil {
		return failure(500, "数据库备份失败，请稍后重试", "candidate-sync-close")
	}
	snapshot.file, err = snapshot.root.Open("snapshot.db")
	if err != nil {
		return failure(500, "无法读取备份文件", "open-read-only")
	}
	if _, err = snapshot.checkMembers(); err != nil {
		return failure(500, "备份状态不可用", "candidate-members")
	}
	snapshot.metadata, err = verifyDatabaseSnapshot(ctx, filepath.Join(path, "snapshot.db"), snapshot.file, nil)
	if err != nil {
		return failure(500, "备份校验失败，请稍后重试", "verification")
	}
	snapshot.info, err = stableSnapshotFile(filepath.Join(path, "snapshot.db"), snapshot.file, member)
	if err != nil || snapshot.checkDirectory() != nil || !os.SameFile(member, snapshot.info) {
		return failure(500, "备份状态不可用", "final-identity")
	}
	if _, err = snapshot.checkMembers(); err != nil {
		return failure(500, "备份状态不可用", "final-members")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	snapshot.metadata.CreatedAt = time.Now().UTC()
	return snapshot, nil
}

// checkDirectory 比较父目录成员与打开的目录身份，不跟随替换链接。
func (snapshot *databaseSnapshot) checkDirectory() error {
	if snapshot.parent == nil || snapshot.root == nil || snapshot.dirInfo == nil {
		return errors.New("备份目录身份不可核实")
	}
	current, err := snapshot.parent.Lstat(snapshot.directory)
	opened, openedErr := snapshot.root.Stat(".")
	if err != nil || openedErr != nil || isSnapshotReparsePoint(current) || !current.IsDir() || !os.SameFile(snapshot.dirInfo, current) || !os.SameFile(current, opened) {
		return errors.New("备份目录身份已变化")
	}
	return nil
}

// checkMembers 核实精确成员集合，未知成员或身份改变不允许交付或清理。
func (snapshot *databaseSnapshot) checkMembers() (bool, error) {
	if err := snapshot.checkDirectory(); err != nil {
		return false, err
	}
	directory, err := snapshot.root.Open(".")
	if err != nil {
		return false, errors.New("备份目录不可检查")
	}
	// 合法目录最多一个候选；第二个成员已足以拒绝，避免未知目录触发无界枚举。
	entries, readErr := directory.ReadDir(2)
	closeErr := directory.Close()
	if errors.Is(readErr, io.EOF) {
		readErr = nil // 有界枚举到目录末尾是正常结束，不能掩盖真实读取或关闭错误。
	}
	if errors.Join(readErr, closeErr) != nil {
		return false, errors.New("备份目录检查或关闭失败")
	}
	if len(entries) > 1 || (len(entries) == 1 && (entries[0].Name() != "snapshot.db" || snapshot.memberInfo == nil)) {
		return false, errors.New("备份目录含未知成员，已保留")
	}
	if len(entries) == 1 {
		current, statErr := snapshot.root.Lstat("snapshot.db")
		if statErr != nil || !current.Mode().IsRegular() || isSnapshotReparsePoint(current) || !os.SameFile(snapshot.memberInfo, current) {
			return false, errors.New("备份文件身份已变化，已保留")
		}
	} else if snapshot.memberInfo != nil {
		return false, errors.New("备份文件意外消失，已保留目录")
	}
	return len(entries) == 1, nil
}

// cleanMembers 只删除身份未变的精确候选；未知成员导致保留整个目录。
func (snapshot *databaseSnapshot) cleanMembers() error {
	present, err := snapshot.checkMembers()
	if err != nil {
		return err
	}
	if present {
		if err = snapshot.operations.remove(snapshot.root, "snapshot.db"); err != nil {
			return errors.New("备份文件清理失败")
		}
	}
	return nil
}

// Close 同步幂等地关闭文件和目录，无论清理结果如何都最终释放唯一互斥。
func (snapshot *databaseSnapshot) Close() error {
	snapshot.once.Do(func() {
		defer snapshot.app.backupMu.Unlock()
		if snapshot.file != nil {
			if err := snapshot.operations.close(snapshot.file); err != nil {
				snapshot.closeErr = errors.Join(snapshot.closeErr, errors.New("备份读取文件关闭失败"))
			}
		}
		if snapshot.root != nil {
			memberErr := snapshot.cleanMembers()
			rootErr := snapshot.root.Close()
			if memberErr == nil && rootErr == nil {
				current, err := snapshot.parent.Lstat(snapshot.directory)
				if err != nil || isSnapshotReparsePoint(current) || !os.SameFile(current, snapshot.dirInfo) {
					memberErr = errors.New("备份目录身份已变化，已保留")
				} else if err = snapshot.operations.remove(snapshot.parent, snapshot.directory); err != nil {
					memberErr = errors.New("备份空目录清理失败")
				}
			}
			if rootErr != nil {
				rootErr = errors.New("备份目录句柄关闭失败")
			}
			snapshot.closeErr = errors.Join(snapshot.closeErr, memberErr, rootErr)
		} else if snapshot.directory != "" {
			snapshot.closeErr = errors.Join(snapshot.closeErr, errors.New("备份目录归属不可确认，已保留"))
		}
		if snapshot.parent != nil {
			if err := snapshot.parent.Close(); err != nil {
				snapshot.closeErr = errors.Join(snapshot.closeErr, errors.New("备份数据根句柄关闭失败"))
			}
		}
		if snapshot.closeErr != nil {
			if snapshot.app.logger != nil {
				snapshot.app.logger.Error("一致性备份资源收尾失败，候选材料可能已保留", "stage", "cleanup", "candidate", snapshot.directory)
			}
			snapshot.app.mu.Lock()
			snapshot.app.cleanupErr = errors.Join(snapshot.app.cleanupErr, snapshot.closeErr)
			snapshot.app.mu.Unlock()
		}
	})
	return snapshot.closeErr
}
