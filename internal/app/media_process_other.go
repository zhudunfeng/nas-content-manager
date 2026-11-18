//go:build !windows

package app

import "os/exec"

// configureMediaProcess 在 Linux 保持标准子进程与取消行为。
func configureMediaProcess(_ *exec.Cmd) {}

// runMediaProcess 在非 Windows 平台保留标准命令执行与上下文取消。
func runMediaProcess(command *exec.Cmd) (error, error) {
	return command.Run(), nil
}
