//go:build linux

package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// backupPathSyntax 仅接受本地绝对路径，不接受NUL及网络路径别名。
func backupPathSyntax(path string) bool {
	return filepath.IsAbs(path) && !strings.ContainsRune(path, 0) && !strings.HasPrefix(path, "//")
}

// backupPathKey 按Linux大小写敏感分段比较。
func backupPathKey(path string) string { return filepath.Clean(path) }

// backupResolveAnchorPath 先展开Linux链接，再由O_NOFOLLOW和实际设备身份核对稳定端点。
func backupResolveAnchorPath(path string) (string, error) { return filepath.EvalSymlinks(path) }

// backupStatLock Linux的受控Lstat不需要数据访问，可直接核固定锁目录项。
func backupStatLock(root *os.Root, path string) (os.FileInfo, error) {
	return root.Lstat(backupLockName)
}

// backupOpenAnchor 目录保留可同步句柄，其他端点仅用O_PATH而不读取或连接。
func backupOpenAnchor(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if info.IsDir() {
		if err = file.Close(); err != nil {
			return nil, err
		}
		fd, err = unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, err
		}
		file = os.NewFile(uintptr(fd), path)
	}
	return file, nil
}

// backupLinuxMount 保存有界mountinfo中的实际设备根与文件系统类型。
type backupLinuxMount struct {
	device string // 内核major:minor，用于与fstat对照。
	root   string // 设备内挂载起点，覆盖bind别名。
	point  string // 当前命名空间挂载位置。
	fs     string // 内核声明的文件系统类型，不能只凭ext家族magic认ext4。
}

// backupLinuxMountFor 有界读取当前挂载表，选择实际路径最长分段匹配。
func backupLinuxMountFor(path string) (backupLinuxMount, error) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return backupLinuxMount{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	closeErr := file.Close()
	if errors.Join(readErr, closeErr) != nil || len(data) > 2<<20 {
		return backupLinuxMount{}, errors.New("备份挂载信息不可读")
	}
	var found backupLinuxMount
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for line := range strings.Lines(string(data)) {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			return found, errors.New("备份挂载信息无效")
		}
		separator := -1
		for i := 6; i < len(fields); i++ {
			if fields[i] == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || separator+3 >= len(fields) {
			return found, errors.New("备份挂载信息无效")
		}
		point := unescape.Replace(fields[4])
		if backupContains(point, path) && (found.point == "" || len(point) >= len(found.point)) {
			found = backupLinuxMount{fields[2], unescape.Replace(fields[3]), point, fields[separator+1]}
		}
	}
	if found.point == "" {
		return found, errors.New("备份卷映射不可确认")
	}
	return found, nil
}

// backupPhysicalPath 对照fstat设备与mountinfo，将bind路径映射到同一设备内分段。
func backupPhysicalPath(file *os.File, path string) (string, string, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return "", "", err
	}
	mount, err := backupLinuxMountFor(path)
	if err != nil {
		return "", "", err
	}
	device := fmt.Sprintf("%d:%d", unix.Major(uint64(stat.Dev)), unix.Minor(uint64(stat.Dev)))
	if device != mount.device {
		return "", "", errors.New("备份设备映射不符")
	}
	relative, err := filepath.Rel(mount.point, path)
	if err != nil {
		return "", "", err
	}
	return device, filepath.Join(mount.root, relative), nil
}

// backupCheckFilesystem 首批只允许mountinfo和fstatfs一致的本地ext4或XFS。
func backupCheckFilesystem(file *os.File, path string) error {
	mount, err := backupLinuxMountFor(path)
	if err != nil {
		return err
	}
	var fs unix.Statfs_t
	var stat unix.Stat_t
	if unix.Fstatfs(int(file.Fd()), &fs) != nil || unix.Fstat(int(file.Fd()), &stat) != nil || unix.Major(uint64(stat.Dev)) == 0 || mount.device != fmt.Sprintf("%d:%d", unix.Major(uint64(stat.Dev)), unix.Minor(uint64(stat.Dev))) {
		return errors.New("备份文件系统不可确认")
	}
	if mount.fs == "ext4" && fs.Type == unix.EXT4_SUPER_MAGIC || mount.fs == "xfs" && fs.Type == unix.XFS_SUPER_MAGIC {
		return nil
	}
	return errors.New("备份文件系统未支持")
}

// backupCheckSingleLink 拒绝不是单硬链接普通文件的成员。
func backupCheckSingleLink(file *os.File) error {
	var stat unix.Stat_t
	if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Nlink != 1 || stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("备份成员不是单链接普通文件")
	}
	return nil
}

// backupCheckPermissions 核对实际POSIX权限和当前有效用户，而非仅相信chmod返回值。
func backupCheckPermissions(file *os.File, directory bool) error {
	var stat unix.Stat_t
	expected := uint32(0600)
	if directory {
		expected = 0700
	}
	if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Mode&07777 != expected || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("备份私密权限不符")
	}
	return nil
}

// backupCreateFile 在持有根内排他创建0600文件，不允许覆盖已有成员。
func backupCreateFile(root *os.Root, path string, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
}

// backupMoveNoReplace 使用持有File的dirfd和RENAME_NOREPLACE，任何错误均不回退。
func backupMoveNoReplace(directory *os.File, path string, old string, next string) error {
	return unix.Renameat2(int(directory.Fd()), old, int(directory.Fd()), next, unix.RENAME_NOREPLACE)
}

// backupSyncDirectory 明确检查每个变更父目录的fsync结果。
func backupSyncDirectory(directory *os.File) error { return directory.Sync() }
