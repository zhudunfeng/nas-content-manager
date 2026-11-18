//go:build linux

package enginehelper

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// fileIdentity 返回Linux文件实际属主，权限检查不以root写成功代替。
func fileIdentity(info os.FileInfo) (int, int, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, errors.New("文件属主不可验证")
	}
	return int(stat.Uid), int(stat.Gid), nil
}

// verifyJackettWrite 在当前默认安全策略下探测.NET需要的pwritev2；ENOSYS可回退，EPERM不可回退。
func verifyJackettWrite(path string) error {
	f, err := os.CreateTemp(path, ".nas-jackett-capability-")
	if err != nil {
		return errors.New("Jackett文件写入能力探针无法创建临时文件")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	data := []byte("probe")
	written, err := unix.Pwritev2(int(f.Fd()), [][]byte{data}, 0, 0)
	if errors.Is(err, unix.ENOSYS) {
		return nil
	}
	if errors.Is(err, unix.EPERM) {
		return errors.New("当前Docker安全策略阻止Jackett所需文件写入，请升级Docker或安全策略后重试；未降低安全配置")
	}
	if err != nil || written != len(data) {
		return errors.New("Jackett文件写入能力检查失败，请检查文件系统与权限")
	}
	return nil
}

// verifyTargetAccess 在锁定线程上以目标fsuid/fsgid进行实际读写探针，避免root成功掩盖ACL问题。
func verifyTargetAccess(path string, uid int, gid int) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.New("专用目录无法访问")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != uid || int(stat.Gid) != gid {
		return errors.New("专用目录UID/GID与引擎配置不符，请人工修复权限")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oldGID, err := unix.SetfsgidRetGid(gid)
	if err != nil {
		return errors.New("无法切换目录探针组身份")
	}
	oldUID, err := unix.SetfsuidRetUid(uid)
	if err != nil {
		_ = unix.Setfsgid(oldGID)
		return errors.New("无法切换目录探针用户身份")
	}
	// fsuid只影响当前线程的文件检查，探针结束必须恢复，失败时终止防止错误身份继续服务。
	defer func() {
		if unix.Setfsuid(oldUID) != nil || unix.Setfsgid(oldGID) != nil {
			panic("助手文件身份恢复失败")
		}
	}()
	actualUID, _ := unix.SetfsuidRetUid(-1)
	actualGID, _ := unix.SetfsgidRetGid(-1)
	if actualUID != uid || actualGID != gid {
		return errors.New("目标目录身份切换未生效")
	}
	f, err := os.CreateTemp(path, ".nas-permission-")
	if err != nil {
		return errors.New("引擎目标身份无法写入专用目录，请检查ACL")
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write([]byte("probe")); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("引擎目标身份写入探针失败")
	}
	if _, err = os.ReadFile(filepath.Clean(temp)); err != nil {
		return errors.New("引擎目标身份读取探针失败")
	}
	return nil
}
