package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"nas-content-manager/internal/collectionworker"
)

// main 启动固定资源获取执行端，退出信号只回收本实例所属操作。
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := collectionworker.LoadConfig()
	if err != nil {
		logger.Error("资源获取服务配置无效", "error", err.Error())
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = collectionworker.Run(ctx, config, logger); err != nil {
		logger.Error("资源获取服务已停止", "error", err.Error())
		os.Exit(1)
	}
}
