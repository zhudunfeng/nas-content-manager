//go:build windows

// nas-desktop 提供 Windows 原生托盘与一键启动入口。
package main

import (
	"os"

	"nas-content-manager/internal/desktop"
)

// 桌面图标资源已提交，普通构建无需额外工具；仅更新 ICO 后执行再生。
//go:generate python -B ../../scripts/generate_windows_icon.py

// main 交由原生启动器处理配置、单实例和服务生命周期。
func main() { os.Exit(desktop.Main(os.Args[1:])) }
