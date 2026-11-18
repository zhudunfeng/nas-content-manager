//go:build !windows

package app

import "os/exec"

// configureMediaProcess 在 Linux 保持标准子进程与取消行为。
func configureMediaProcess(_ *exec.Cmd) {}
