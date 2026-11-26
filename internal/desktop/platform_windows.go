//go:build windows

package desktop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// defaultProfileRoot 返回栖库.exe 同级的用户数据 profile 根目录。
func defaultProfileRoot() (string, error) {
	executable, err := os.Executable()
	if err != nil || strings.TrimSpace(executable) == "" {
		return "", errors.New("无法定位栖库.exe所在目录，请使用 --profile 指定本机配置目录")
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return "", errors.New("无法规范化栖库.exe路径，请使用 --profile 指定本机配置目录")
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", errors.New("无法核实栖库.exe所在目录，请使用 --profile 指定本机配置目录")
	}
	return filepath.Join(filepath.Dir(absolute), portableDataDirectoryName, "profiles"), nil
}

// currentUserSID 返回当前用户身份，仅用于本机命名对象和私密 ACL。
func currentUserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", errors.New("无法读取当前 Windows 用户身份")
	}
	return user.User.Sid.String(), nil
}

// privateSecurity 仅允许当前用户和 SYSTEM 访问启动配置、互斥量及控制通道。
func privateSecurity(inherit bool) (*windows.SECURITY_DESCRIPTOR, error) {
	sid, err := currentUserSID()
	if err != nil {
		return nil, err
	}
	flags := ""
	if inherit {
		flags = "OICI"
	}
	return windows.SecurityDescriptorFromString("D:P(A;" + flags + ";GA;;;" + sid + ")(A;" + flags + ";GA;;;SY)")
}

// privateDirectory 为应用私密目录设置实际用户 ACL，不以 chmod 冒充 Windows 权限。
func privateDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return errors.New("无法创建本机私密配置目录")
	}
	sd, err := privateSecurity(true)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return errors.New("无法限制本机配置目录的 Windows 访问权限")
	}
	return nil
}

// validateLocalDataDir 拒绝网络映射与重解析后的网络盘，SQLite 只落本地磁盘。
func validateLocalDataDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return errors.New("本地数据目录不可创建或不可写")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return errors.New("无法核实数据目录的真实本地位置")
	}
	volume := filepath.VolumeName(resolved)
	if len(volume) != 2 || volume[1] != ':' || strings.HasPrefix(resolved, `\\`) {
		return errors.New("SQLite 数据目录必须位于本地磁盘，不能使用网络共享或映射网络盘")
	}
	root, err := windows.UTF16PtrFromString(volume + `\`)
	if err != nil {
		return err
	}
	typ := windows.GetDriveType(root)
	if typ != windows.DRIVE_FIXED && typ != windows.DRIVE_REMOVABLE && typ != windows.DRIVE_RAMDISK {
		return errors.New("SQLite 数据目录不在可用的本地磁盘上")
	}
	return privateDirectory(path)
}

// replaceConfigFile 使用 Windows 原子替换并要求写穿，升级不截断已有配置。
func replaceConfigFile(source string, target string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// hideCommand 避免 GUI 子系统调用 Docker 时反复弹出控制台。
func hideCommand(command *exec.Cmd) { command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

// securityAttributes 保持安全描述符有效直至同步 Windows 调用结束。
func securityAttributes(sd *windows.SECURITY_DESCRIPTOR) *windows.SecurityAttributes {
	return &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
}
