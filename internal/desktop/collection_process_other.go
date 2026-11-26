//go:build !windows

package desktop

import "os/exec"

// collectionProcess 仅使共享配置在非 Windows 可编译，不启用桌面进程管理。
type collectionProcess struct {
	done    chan struct{} // 非 Windows 不会创建此进程完成信号。
	err     error         // 非 Windows 不会启动托管 worker。
	exitErr error         // 非 Windows 不会生成 worker 自身退出结果。
}

// startCollectionProcess 明确要求 Windows 桌面入口，不改变 Linux worker CLI 行为。
func startCollectionProcess(_ *exec.Cmd) (*collectionProcess, error) {
	return nil, unsupportedDesktop()
}

// terminate 在非 Windows 不终止任何进程。
func (p *collectionProcess) terminate() error { return unsupportedDesktop() }
