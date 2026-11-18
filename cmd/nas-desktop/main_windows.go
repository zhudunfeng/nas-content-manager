//go:build windows

// nas-desktop 提供 Windows 原生托盘与一键启动入口。
package main

import (
	"os"

	"nas-content-manager/internal/desktop"
)

// main 交由原生启动器处理配置、单实例和服务生命周期。
func main() { os.Exit(desktop.Main(os.Args[1:])) }
