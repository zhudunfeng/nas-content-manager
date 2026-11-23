// nas 是统一媒体库服务的启动入口。
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"nas-content-manager/internal/app"
)

// main 只在命令执行和全部清理结束后设置进程退出码。
func main() {
	os.Exit(run())
}

// run 先建立取消边界并分派维护命令，再加载正常服务配置。
func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if len(os.Args) > 1 {
		return app.RunAccountMaintenance(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := app.LoadConfig()
	if err != nil {
		logger.Error("应用配置无效", "error", err)
		return 1
	}
	if err = app.Run(ctx, config, logger); err != nil {
		logger.Error("应用运行失败", "error", err)
		return 1
	}
	return 0
}
