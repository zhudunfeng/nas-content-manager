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

// main 加载配置并在终止信号到来时有序停止服务。
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := app.LoadConfig()
	if err != nil {
		logger.Error("应用配置无效", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = app.Run(ctx, config, logger); err != nil {
		logger.Error("应用运行失败", "error", err)
		os.Exit(1)
	}
}
