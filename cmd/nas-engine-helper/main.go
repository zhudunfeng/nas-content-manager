// nas-engine-helper 仅运行固定模板的同机Docker初始化控制面。
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"nas-content-manager/internal/enginehelper"
)

// main 读取受信任部署参数并用信号控制持久任务的可恢复退出。
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config, err := enginehelper.LoadConfig()
	if err != nil {
		logger.Error("助手配置无效", "reason", err.Error())
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = enginehelper.Run(ctx, config, logger); err != nil {
		logger.Error("助手退出，请检查受信任部署配置", "reason", err.Error())
		os.Exit(1)
	}
}
