// nas-engine-helper 仅运行固定模板的同机Docker初始化控制面。
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nas-content-manager/internal/enginehelper"
)

// main 读取受信任部署参数并用信号控制持久任务的可恢复退出。
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if len(os.Args) > 1 {
		if len(os.Args) != 2 || os.Args[1] != "stop-all" {
			logger.Error("助手仅支持无参数服务启动或stop-all固定停止命令")
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		socket := os.Getenv("ENGINE_HELPER_SOCKET")
		if socket == "" {
			socket = "/ipc/helper.sock"
		}
		if err := enginehelper.StopAll(ctx, socket); err != nil {
			logger.Error("本安装引擎停止未完成", "reason", err.Error())
			os.Exit(1)
		}
		logger.Info("本安装全部受管引擎已停止，持久数据完整保留")
		return
	}
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
