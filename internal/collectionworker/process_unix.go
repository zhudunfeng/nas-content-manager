//go:build !windows

package collectionworker

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// configureWorkerProcess 以本次命令独占的进程组持有Linux工具及其后代。
func configureWorkerProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// runWorkerProcess 在取消和工具结束时回收本次命令归属进程组。
func runWorkerProcess(command *exec.Cmd) (error, error) {
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	if err := command.Start(); err != nil {
		return err, nil
	}
	err := command.Wait()
	cleanupErr := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(cleanupErr, syscall.ESRCH) {
		cleanupErr = nil
	} else if cleanupErr == nil {
		// 进程组中的后代仍可能持有输出管道；确认全部退出后才返回完成。
		deadline := time.Now().Add(5 * time.Second)
		for {
			stateErr := syscall.Kill(-command.Process.Pid, 0)
			if errors.Is(stateErr, syscall.ESRCH) {
				break
			}
			if time.Now().After(deadline) {
				cleanupErr = errors.New("获取工具进程组退出未能确认")
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return err, cleanupErr
}
