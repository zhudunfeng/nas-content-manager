//go:build windows

package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// backupPathSyntax 拒绝UNC、设备路径、驱动器相对路径及保留/尾点空格别名。
func backupPathSyntax(path string) bool {
	if strings.ContainsRune(path, 0) || !filepath.IsAbs(path) {
		return false
	}
	normal := strings.ReplaceAll(path, "/", "\\")
	if len(normal) < 3 || normal[1] != ':' || normal[2] != '\\' || !(normal[0] >= 'A' && normal[0] <= 'Z' || normal[0] >= 'a' && normal[0] <= 'z') {
		return false
	}
	for _, part := range strings.Split(normal[3:], "\\") {
		if part == "" {
			continue
		}
		if part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, ":<>\"|?*") {
			return false
		}
		for _, c := range part {
			if c < 32 {
				return false
			}
		}
		// Win32设备名主干忽略扩展名前的ASCII空格，不能让CON .db等别名绕过拒绝。
		base := strings.ToUpper(strings.TrimRight(strings.SplitN(part, ".", 2)[0], " "))
		deviceSuffix := ""
		if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
			deviceSuffix = base[3:]
		}
		deviceReserved := false
		for _, suffix := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"} {
			if deviceSuffix == suffix {
				deviceReserved = true
			}
		}
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" || deviceReserved {
			return false
		}
	}
	return true
}

// backupPathKey 保守按Windows不区分大小写的分段比较，最终路径已由真实句柄规范化。
func backupPathKey(path string) string { return strings.ToLower(filepath.Clean(path)) }

// backupResolveAnchorPath 保留原解析入口；实际规范卷路径由跟随后的句柄取得，避免Go误拒junction祖先。
func backupResolveAnchorPath(path string) (string, error) { return filepath.Clean(path), nil }

// backupOpenAnchor 跟随端点取得不读取内容的实际anchor，junction不能只依赖EvalSymlinks解析。
func backupOpenAnchor(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Go可把junction标记为ModeIrregular；必须让内核跟随，随后以实际句柄卷路径和Stat身份交叉核对。
	h, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

// backupStatLock 用零数据访问读取锁目录项身份，避免Go Root.Lstat的读访问与零共享锁冲突。
func backupStatLock(root *os.Root, path string) (info os.FileInfo, resultErr error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(h), path)
	defer func() {
		if file.Close() != nil {
			resultErr = errors.Join(resultErr, errors.New("备份锁核对句柄关闭失败"))
		}
	}()
	return file.Stat()
}

// backupPhysicalPath 由实际句柄取得规范卷GUID路径，避免盘符、junction和长短名别名绕过。
func backupPhysicalPath(file *os.File, path string) (string, string, error) {
	buffer := make([]uint16, 32768)
	count, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &buffer[0], uint32(len(buffer)), 1)
	if err != nil || count == 0 || int(count) >= len(buffer) {
		return "", "", errors.New("卷路径不可确认")
	}
	final := windows.UTF16ToString(buffer[:count])
	prefix := `\\?\Volume{`
	if !strings.HasPrefix(final, prefix) {
		return "", "", errors.New("本地卷不可确认")
	}
	end := strings.Index(final, "}\\")
	if end < 0 {
		return "", "", errors.New("卷路径不可确认")
	}
	volume := final[:end+2]
	return strings.ToLower(volume), filepath.Clean(final), nil
}

// backupCheckFilesystem 首批仅允许真实根句柄确认的fixed NTFS，不凭盘符推断。
func backupCheckFilesystem(file *os.File, path string) error {
	device, _, err := backupPhysicalPath(file, path)
	if err != nil {
		return err
	}
	volume, err := windows.UTF16PtrFromString(device)
	if err != nil || windows.GetDriveType(volume) != windows.DRIVE_FIXED {
		return errors.New("备份卷类型未支持")
	}
	var fs [64]uint16
	var flags uint32
	if windows.GetVolumeInformationByHandle(windows.Handle(file.Fd()), nil, 0, nil, nil, &flags, &fs[0], uint32(len(fs))) != nil || windows.UTF16ToString(fs[:]) != "NTFS" || flags&windows.FILE_PERSISTENT_ACLS == 0 || flags&windows.FILE_READ_ONLY_VOLUME != 0 {
		return errors.New("备份文件系统未支持")
	}
	return nil
}

// backupCheckSingleLink 拒绝多硬链接或重解析成员，并核真实句柄类型。
func backupCheckSingleLink(file *os.File) error {
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info) != nil || info.NumberOfLinks != 1 || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return errors.New("备份成员不是单链接普通文件")
	}
	return nil
}

// backupPrivateDescriptor 生成仅当前用户和SYSTEM的protected DACL，不修改全局令牌。
func backupPrivateDescriptor(directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	flags := ""
	if directory {
		flags = "OICI"
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, err
	}
	entries := "(A;" + flags + ";GA;;;" + user.User.Sid.String() + ")"
	if !user.User.Sid.Equals(system) {
		entries += "(A;" + flags + ";GA;;;SY)"
	}
	return windows.SecurityDescriptorFromString("D:P" + entries)
}

// backupCheckPermissions 读回真实DACL，逐主体核对完整本对象权限和目录继承权限。
func backupCheckPermissions(file *os.File, directory bool) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return errors.New("备份权限不可核对")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("备份权限未受保护")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return errors.New("备份权限主体异常")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return errors.New("备份权限身份不可读")
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	return backupCheckDACL(dacl, user.User.Sid, system, directory)
}

// backupCheckDACL 按唯一SID验证受限授权，当前进程为SYSTEM时不要求重复主体。
func backupCheckDACL(dacl *windows.ACL, user *windows.SID, system *windows.SID, directory bool) error {
	identical := user.Equals(system)
	subjects := uint16(2)
	if identical {
		subjects = 1
	}
	if dacl == nil || dacl.AceCount < subjects || dacl.AceCount > subjects*2 || !directory && dacl.AceCount != subjects {
		return errors.New("备份权限主体异常")
	}
	// Windows可把目录OICI/GA规范化成有效权限与仅继承权限两项，不能假定原始ACE数量不变。
	const effective = byte(1)
	const inheritable = byte(2)
	userRights, systemRights := byte(0), byte(0)
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask != 0x001f01ff && ace.Mask != windows.GENERIC_ALL {
			return errors.New("备份权限授权异常")
		}
		rights := effective
		if directory {
			switch ace.Header.AceFlags {
			case 0:
			case windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE:
				rights |= inheritable
			case windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE | windows.INHERIT_ONLY_ACE:
				rights = inheritable
			default:
				return errors.New("备份权限继承异常")
			}
		} else if ace.Header.AceFlags != 0 {
			return errors.New("备份权限继承异常")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.Equals(user) {
			if userRights&rights != 0 {
				return errors.New("备份权限包含重复授权")
			}
			userRights |= rights
		} else if sid.Equals(system) {
			if systemRights&rights != 0 {
				return errors.New("备份权限包含重复授权")
			}
			systemRights |= rights
		} else {
			return errors.New("备份权限包含未知主体")
		}
	}
	expected := effective
	if directory {
		expected |= inheritable
	}
	if userRights != expected || !identical && systemRights != expected {
		return errors.New("备份权限缺少固定主体")
	}
	return nil
}

// backupCreateFile 使用CREATE_NEW、初始protected DACL及WRITE_THROUGH写句柄。
func backupCreateFile(root *os.Root, path string, name string) (*os.File, error) {
	if !filepath.IsLocal(name) || filepath.Base(name) != name {
		return nil, errors.New("备份成员名无效")
	}
	if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrExist
	}
	sd, err := backupPrivateDescriptor(false)
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	full := filepath.Join(path, name)
	pointer, err := windows.UTF16PtrFromString(full)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(pointer, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_WRITE_THROUGH, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), full), nil
}

// backupMoveNoReplace 仅使用文档规定的写穿移动，不覆盖、不跨卷复制且不回退。
func backupMoveNoReplace(directory *os.File, path string, old string, next string) error {
	from, err := windows.UTF16PtrFromString(filepath.Join(path, old))
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(filepath.Join(path, next))
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}

// backupSyncDirectory Windows协议不声称POSIX目录fsync语义，实际边界由文件flush和写穿移动提供。
func backupSyncDirectory(directory *os.File) error { _, err := directory.Stat(); return err }
