//go:build windows

package app

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureMediaProcess 防止桌面模式探测媒体时弹出额外命令行窗口。
func configureMediaProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
