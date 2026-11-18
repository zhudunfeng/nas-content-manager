package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// mediaTool 统一诊断与执行使用的工具路径；显式覆盖失败时不静默退回其他工具。
func (a *App) mediaTool(name string) (string, error) {
	configured := ""
	switch name {
	case "ffmpeg":
		configured = a.config.FFmpegPath
	case "ffprobe":
		configured = a.config.FFprobePath
	default:
		return "", fmt.Errorf("不支持的媒体处理工具")
	}
	if configured != "" {
		return validateMediaTool(configured)
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法定位应用程序目录")
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("无法规范化应用程序目录")
	}
	filename := name
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	packaged := filepath.Join(filepath.Dir(executable), "tools", filename)
	if _, err = os.Stat(packaged); err == nil {
		return validateMediaTool(packaged)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("包内媒体工具不可访问，请修复安装包")
	}
	// 兼容 Linux CLI/容器的系统工具，同时拒绝当前工作目录或相对 PATH 项加载。
	resolved, err := exec.LookPath(name)
	if err != nil || !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("未找到可用的 %s，请检查包内 tools 或配置绝对工具路径", name)
	}
	return validateMediaTool(resolved)
}

// validateMediaTool 拒绝目录、相对路径及不可执行文件，返回规范的绝对路径。
func validateMediaTool(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("媒体工具必须使用绝对路径")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("媒体工具不存在或不可访问")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("媒体工具必须为可执行的普通文件")
	}
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(filepath.Ext(resolved), ".exe") {
			return "", fmt.Errorf("Windows 媒体工具必须为 EXE 文件")
		}
	} else if info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("媒体工具没有执行权限")
	}
	return resolved, nil
}

// mediaCommand 仅以统一解析后的绝对程序路径启动可取消子进程，不经过 shell。
func (a *App) mediaCommand(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	path, err := a.mediaTool(name)
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, path, args...)
	configureMediaProcess(command)
	return command, nil
}
