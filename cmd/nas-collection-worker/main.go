package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"nas-content-manager/internal/collectionworker"
)

// main 启动固定资源获取执行端，退出信号只回收本实例所属操作。
func main() {
	os.Exit(run(os.Args[1:]))
}

// run 严格分派普通服务、私有托管和本地依赖诊断，不接受任意工具参数。
func run(args []string) int {
	mode := "serve"
	if len(args) > 0 {
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "执行端参数无效；支持 --managed-stdio、--check-jmcomic 或 --help")
			return 2
		}
		switch args[0] {
		case "--managed-stdio":
			mode = "managed"
		case "--check-jmcomic":
			mode = "check"
		case "--help", "-h":
			fmt.Fprintln(os.Stdout, "资源获取执行端：无参数启动普通服务；--managed-stdio 仅供桌面继承管道托管；--check-jmcomic 只检查本地漫画依赖，不下载或访问站点")
			return 0
		default:
			fmt.Fprintln(os.Stderr, "执行端参数无效；请使用 --help 查看支持的固定入口")
			return 2
		}
	}
	var logOutput io.Writer = os.Stdout
	if mode != "serve" {
		logOutput = os.Stderr
	}
	logger := slog.New(slog.NewJSONHandler(logOutput, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if mode == "check" {
		return checkJMComic(ctx, logger)
	}
	config, err := collectionworker.LoadConfig()
	if err != nil {
		logger.Error("资源获取服务配置无效", "error", err.Error())
		return 1
	}
	if mode == "managed" {
		err = collectionworker.RunManaged(ctx, config, logger, os.Stdin, os.Stdout)
	} else {
		err = collectionworker.Run(ctx, config, logger)
	}
	if err != nil {
		logger.Error("资源获取服务已停止", "error", err.Error())
		return 1
	}
	return 0
}

// checkJMComic 输出脱敏中文本地诊断，返回0表示依赖可用、2表示未就绪、1表示诊断失败。
func checkJMComic(ctx context.Context, logger *slog.Logger) int {
	config, err := collectionworker.LoadJMComicConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "漫画插件检查失败："+err.Error())
		return 1
	}
	capability, err := collectionworker.CheckJMComic(ctx, config, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "漫画插件检查失败："+err.Error())
		return 1
	}
	if !capability.Available {
		fmt.Fprintln(os.Stdout, "漫画插件未就绪："+capability.Reason+"；本次未访问站点")
		return 2
	}
	fmt.Fprintln(os.Stdout, "漫画插件本地依赖可用，jmcomic "+capability.Version+"；站点连通性未检查")
	return 0
}
