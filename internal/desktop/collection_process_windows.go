//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// collectionProcess 独占托管 worker 的 Job，桌面异常退出时内核回收本实例子树。
type collectionProcess struct {
	command *exec.Cmd      // 已进入本实例 Job 的子进程。
	job     windows.Handle // 带关闭即终止标记的私有 Job。
	mu      sync.Mutex     // 强制终止与正常退出清理屏障。
	done    chan struct{}  // 全部进程与继承句柄释放后关闭。
	err     error          // Job 与句柄清理结果，在 done 之后读取。
	exitErr error          // worker 自身退出结果，不与清理失败混淆。
}

// startCollectionProcess 在主线程恢复之前分配 Job，失败绝不无所有权运行。
func startCollectionProcess(command *exec.Cmd) (*collectionProcess, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, errors.New("无法创建本机获取服务进程组")
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, errors.New("无法保护本机获取服务进程组")
	}
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED}
	if err = command.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return nil, errors.New("无法启动随包获取服务，请检查完整安装包")
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		_ = windows.CloseHandle(process)
	}
	if err == nil {
		err = resumeCollectionProcess(uint32(command.Process.Pid))
	}
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		_ = windows.CloseHandle(job)
		return nil, errors.New("无法安全托管获取服务，已取消启动")
	}
	p := &collectionProcess{command: command, job: job, done: make(chan struct{})}
	go func() {
		waitErr := command.Wait()
		p.mu.Lock()
		// worker 自行退出后也清理异常遗留的后代，等待实际句柄信号而不只检查 PID。
		cleanupErr := stopCollectionJob(job)
		closeErr := windows.CloseHandle(job)
		p.job = 0
		p.exitErr = waitErr
		p.err = errors.Join(cleanupErr, closeErr)
		p.mu.Unlock()
		close(p.done)
	}()
	return p, nil
}

// terminate 仅终止当前托管 Job，和正常退出共享同一个句柄释放屏障。
func (p *collectionProcess) terminate() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return nil
	}
	return stopCollectionJob(p.job)
}

// collectionJobAccounting 对应原生 Job 统计，确认所有后代均已释放。
type collectionJobAccounting struct {
	TotalUserTime             int64  // 累计用户态时间。
	TotalKernelTime           int64  // 累计内核态时间。
	ThisPeriodTotalUserTime   int64  // 本期用户态时间。
	ThisPeriodTotalKernelTime int64  // 本期内核态时间。
	TotalPageFaultCount       uint32 // 页错误计数。
	TotalProcesses            uint32 // 历史进程数。
	ActiveProcesses           uint32 // 尚未退出的进程数。
	TotalTerminatedProcesses  uint32 // 已终止进程数。
}

// stopCollectionJob 封闭新后代准入并等待本 Job 的真实进程句柄释放。
func stopCollectionJob(job windows.Handle) error {
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	limits.BasicLimitInformation.ActiveProcessLimit = 0
	_, sealErr := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	handles, captureErr := collectionJobHandles(job)
	defer func() {
		for _, handle := range handles {
			_ = windows.CloseHandle(handle)
		}
	}()
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return errors.Join(sealErr, captureErr, errors.New("无法终止本机获取服务进程组"))
	}
	deadline := time.Now().Add(5 * time.Second)
	for _, handle := range handles {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errors.New("本机获取服务进程句柄释放超时")
		}
		status, err := windows.WaitForSingleObject(handle, uint32(remaining.Milliseconds()+1))
		if err != nil || status != windows.WAIT_OBJECT_0 {
			return errors.Join(sealErr, captureErr, errors.New("本机获取服务进程句柄尚未释放"))
		}
	}
	for {
		accounting := collectionJobAccounting{}
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
		if err != nil {
			return errors.Join(sealErr, captureErr, errors.New("无法确认获取服务进程组退出"))
		}
		if accounting.ActiveProcesses == 0 {
			return errors.Join(sealErr, captureErr)
		}
		if time.Now().After(deadline) {
			return errors.New("本机获取服务子进程尚未完整释放")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// collectionJobHandles 只打开当前 Job 成员的等待句柄，防止影响其他 Python 实例。
func collectionJobHandles(job windows.Handle) ([]windows.Handle, error) {
	for capacity := uint32(16); capacity <= 65536; capacity *= 2 {
		storage := make([]uintptr, int(capacity)+2)
		pointer := unsafe.Pointer(&storage[0])
		size := uint32(8 + uintptr(capacity)*unsafe.Sizeof(uintptr(0)))
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicProcessIdList, uintptr(pointer), size, nil)
		if errors.Is(err, windows.ERROR_MORE_DATA) {
			continue
		}
		if err != nil {
			return nil, errors.New("无法读取本机获取服务进程组")
		}
		count := (*[2]uint32)(pointer)[1]
		if count > capacity {
			return nil, errors.New("本机获取服务进程清单超限")
		}
		ids := unsafe.Slice((*uintptr)(unsafe.Add(pointer, 8)), count)
		handles := make([]windows.Handle, 0, count)
		for _, pid := range ids {
			handle, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
			if openErr == nil {
				handles = append(handles, handle)
			} else if !errors.Is(openErr, windows.ERROR_INVALID_PARAMETER) {
				return handles, errors.New("无法等待本机获取服务子进程")
			}
		}
		return handles, nil
	}
	return nil, errors.New("本机获取服务子进程数量超限")
}

// resumeCollectionProcess 仅恢复本次挂起 worker 的主线程，后代自动继承 Job 所有权。
func resumeCollectionProcess(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return openErr
		}
		_, resumeErr := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		return resumeErr
	}
	return fmt.Errorf("未找到本次托管进程的挂起主线程：%w", err)
}
