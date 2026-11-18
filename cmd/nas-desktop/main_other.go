//go:build !windows

// nas-desktop 在其他平台引导使用不依赖桌面的原生服务入口。
package main

import (
	"fmt"
	"os"
)

// main 明确说明平台边界，Linux 保持使用 nas 或 Docker。
func main() {
	fmt.Fprintln(os.Stderr, "此托盘启动器仅适用于 Windows；Linux 请运行 nas 或使用 Docker Compose。")
	os.Exit(1)
}
