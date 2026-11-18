//go:build windows

package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// configureMediaProcess 隐藏工具窗口并挂起主线程，归入本命令的 Job 后才能执行。
func configureMediaProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED}
}

// mediaJobAccounting 对应 Windows Job 基本统计，仅用于确认所有后代均已退出。
type mediaJobAccounting struct {
	TotalUserTime             int64  // 累计用户态时间。
	TotalKernelTime           int64  // 累计内核态时间。
	ThisPeriodTotalUserTime   int64  // 当前统计周期用户态时间。
	ThisPeriodTotalKernelTime int64  // 当前统计周期内核态时间。
	TotalPageFaultCount       uint32 // 累计缺页次数。
	TotalProcesses            uint32 // 累计归属进程数。
	ActiveProcesses           uint32 // 尚未退出的归属进程数。
	TotalTerminatedProcesses  uint32 // 累计被终止的进程数。
}

// runMediaProcess 在工具执行首条指令前建立独占进程树所有权，并等待全部句柄释放。
func runMediaProcess(command *exec.Cmd) (error, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("无法创建媒体工具进程组：%w", err), nil
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return fmt.Errorf("无法配置媒体工具进程组：%w", err), nil
	}
	var startMu sync.Mutex
	owned, finished := false, false
	var cancelCleanupErr error
	command.Cancel = func() error {
		// Start 的上下文观察器可能先于归组完成；同一屏障避免取消漏掉新生进程。
		startMu.Lock()
		defer startMu.Unlock()
		if finished {
			return os.ErrProcessDone
		}
		if !owned {
			return command.Process.Kill()
		}
		if stopErr := stopMediaJob(job); stopErr != nil {
			cancelCleanupErr = stopErr
			return errors.Join(fmt.Errorf("无法终止媒体工具进程组：%w", stopErr), command.Process.Kill())
		}
		return nil
	}
	startMu.Lock()
	if err = command.Start(); err != nil {
		startMu.Unlock()
		return err, nil
	}
	process, setupErr := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if setupErr == nil {
		setupErr = windows.AssignProcessToJobObject(job, process)
		_ = windows.CloseHandle(process)
		if setupErr == nil {
			owned = true
			setupErr = resumeMediaProcess(uint32(command.Process.Pid))
		}
	}
	if setupErr != nil {
		// 归组或恢复失败时工具仍处于挂起状态，绝不降级为无所有权运行。
		_ = command.Process.Kill()
	}
	startMu.Unlock()
	waitErr := command.Wait()
	startMu.Lock()
	finished = true
	var cleanupErr error
	if owned {
		// 工具正常结束也必须回收异常遗留的后代，Job 不覆盖浏览器或其他实例。
		cleanupErr = stopMediaJob(job)
	}
	startMu.Unlock()
	if setupErr != nil {
		setupErr = fmt.Errorf("无法安全启动媒体工具：%w", setupErr)
	}
	return errors.Join(setupErr, waitErr), errors.Join(cancelCleanupErr, cleanupErr)
}

// stopMediaJob 保留终止前的进程句柄并等待其信号，避免统计归零先于进程实际释放。
func stopMediaJob(job windows.Handle) error {
	// 先关闭新进程准入，再取得等待句柄，防止快照后新生后代漏过释放屏障。
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	limits.BasicLimitInformation.ActiveProcessLimit = 0
	_, sealErr := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if sealErr != nil {
		sealErr = fmt.Errorf("无法关闭媒体工具进程组的新进程准入：%w", sealErr)
	}
	processes, captureErr := mediaJobProcessHandles(job)
	defer func() {
		for _, process := range processes {
			_ = windows.CloseHandle(process)
		}
	}()
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return errors.Join(sealErr, captureErr, fmt.Errorf("无法回收媒体工具进程组：%w", err))
	}
	deadline := time.Now().Add(5 * time.Second)
	for _, process := range processes {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errors.Join(sealErr, captureErr, fmt.Errorf("媒体工具进程句柄未在等待预算内释放"))
		}
		status, err := windows.WaitForSingleObject(process, uint32(remaining.Milliseconds()+1))
		if err != nil || status != windows.WAIT_OBJECT_0 {
			return errors.Join(sealErr, captureErr, fmt.Errorf("媒体工具进程句柄未释放：状态 %d，错误 %v", status, err))
		}
	}
	return errors.Join(sealErr, captureErr, waitMediaJobEmpty(job))
}

// mediaJobProcessHandles 仅取得当前 Job 中进程的等待句柄，不授予终止其他进程的权限。
func mediaJobProcessHandles(job windows.Handle) ([]windows.Handle, error) {
	for capacity := uint32(16); capacity <= 65536; capacity *= 2 {
		// Job PID 列表由两个 DWORD 计数和原生宽度 PID 数组组成，底层存储保持指针对齐。
		storage := make([]uintptr, int(capacity)+2)
		pointer := unsafe.Pointer(&storage[0])
		size := uint32(8 + uintptr(capacity)*unsafe.Sizeof(uintptr(0)))
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicProcessIdList, uintptr(pointer), size, nil)
		if errors.Is(err, windows.ERROR_MORE_DATA) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("无法取得媒体工具进程等待句柄：%w", err)
		}
		count := (*[2]uint32)(pointer)[1]
		if count > capacity {
			return nil, fmt.Errorf("媒体工具进程列表超过返回缓冲区")
		}
		ids := unsafe.Slice((*uintptr)(unsafe.Add(pointer, 8)), count)
		processes := make([]windows.Handle, 0, count)
		for _, pid := range ids {
			process, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
			if openErr == nil {
				processes = append(processes, process)
			} else if !errors.Is(openErr, windows.ERROR_INVALID_PARAMETER) {
				return processes, fmt.Errorf("无法等待媒体工具进程退出：%w", openErr)
			}
		}
		return processes, nil
	}
	return nil, fmt.Errorf("媒体工具进程数量超过安全等待上限")
}

// resumeMediaProcess 仅恢复本次新建挂起进程的主线程，确保后代从出生起归属同一 Job。
func resumeMediaProcess(pid uint32) error {
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
	return fmt.Errorf("未找到挂起媒体工具的主线程：%w", err)
}

// waitMediaJobEmpty 在终止后等待内核确认所有归属进程已退出，不把发出终止当作释放完成。
func waitMediaJobEmpty(job windows.Handle) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		accounting := mediaJobAccounting{}
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return fmt.Errorf("无法确认媒体工具进程组已退出：%w", err)
		}
		if accounting.ActiveProcesses == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("媒体工具进程组仍有 %d 个进程未释放", accounting.ActiveProcesses)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
