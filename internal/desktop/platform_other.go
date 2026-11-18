//go:build !windows

package desktop

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

// unsupportedDesktop 明确 Linux 使用原 CLI，避免引入桌面环境依赖。
func unsupportedDesktop() error {
	return errors.New("本机托盘入口仅支持 Windows；Linux 请使用 nas 或 Docker Compose")
}

// replaceConfigFile 保留跨平台配置单元验证所需的原子替换语义。
func replaceConfigFile(source string, target string) error { return os.Rename(source, target) }

// privateDirectory 仅供跨平台编译，原 Linux CLI 不调用桌面适配。
func privateDirectory(path string) error { return os.MkdirAll(path, 0700) }

// validateLocalDataDir 在非 Windows 桌面入口明确拒绝运行。
func validateLocalDataDir(_ string) error { return unsupportedDesktop() }

// acquireInstance 不在 Linux 桌面入口模拟 Windows 单实例。
func acquireInstance(_ string) (io.Closer, bool, error) { return nil, false, unsupportedDesktop() }

// serveControl 不在 Linux 注册 Windows 私有控制协议。
func serveControl(_ context.Context, _ string, _ func(string) controlReply, _ func()) (func(), error) {
	return nil, unsupportedDesktop()
}

// sendControl 不在 Linux 连接 Windows 私有控制协议。
func sendControl(_ string, _ string, _ time.Duration) (controlReply, error) {
	return controlReply{}, unsupportedDesktop()
}

// showMessage 在非 Windows 平台不创建 GUI。
func showMessage(_ string, _ string, _ bool) {}

// chooseFolder 不在 Linux 弹出目录选择器。
func chooseFolder(_ uintptr, _ string) (string, error) { return "", unsupportedDesktop() }

// runTray 不在 Linux 启动桌面消息循环。
func runTray(_ context.Context, _ *controller) error { return unsupportedDesktop() }

// openLocalURL 不在 Linux 桌面入口启动浏览器。
func openLocalURL(_ string) error { return unsupportedDesktop() }

// hideCommand 保留共享 Docker 参数构造在 Linux 上可编译。
func hideCommand(_ *exec.Cmd) {}
