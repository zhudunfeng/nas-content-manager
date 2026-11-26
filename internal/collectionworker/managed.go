package collectionworker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"

	"nas-content-manager/internal/collectioncontrol"
)

// managedStart 保存首条控制消息的读取结果，避免取消时遗留管道读取协程。
type managedStart struct {
	message collectioncontrol.Message // 已完成有界语法检查的启动消息。
	err     error                     // 不含原始消息或关联秘密的读取错误。
}

// RunManaged 经父进程私有继承管道运行执行端，父管道断开和停止动作均有序取消本实例。
func RunManaged(ctx context.Context, config Config, logger *slog.Logger, input io.ReadCloser, output io.Writer) error {
	if config.Addr != "127.0.0.1:0" {
		return errors.New("托管执行端必须使用127.0.0.1:0实际绑定回环端口")
	}
	if logger == nil {
		logger = slog.Default()
	}
	config.Managed = true
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	decoder := collectioncontrol.NewDecoder(input)
	first := make(chan managedStart, 1)
	go func() {
		message, err := decoder.Read()
		first <- managedStart{message: message, err: err}
	}()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	var start collectioncontrol.Message
	select {
	case <-ctx.Done():
		_ = input.Close()
		<-first
		return ctx.Err()
	case <-timer.C:
		_ = input.Close()
		<-first
		return errors.New("等待桌面托管启动消息超时")
	case read := <-first:
		if read.err != nil || read.message.Action != "start" {
			_ = input.Close()
			return errors.New("桌面托管启动消息无效或父进程已断开")
		}
		start = read.message
	}
	controlDone := make(chan error, 1)
	go func() {
		message, err := decoder.Read()
		if runCtx.Err() != nil {
			// 服务自身退出时关闭输入仅用于释放阻塞读取，不反向制造协议错误。
			err = nil
		} else if errors.Is(err, io.EOF) {
			logger.Info("桌面托管控制通道已断开，正在停止获取执行端")
			err = nil
		} else if err == nil {
			err = collectioncontrol.Match(message, "stop", start.Nonce, start.InstanceID)
			if err == nil {
				logger.Info("已收到桌面托管停止请求，正在等待获取操作退出")
			}
		}
		if err != nil {
			logger.Warn("桌面托管控制消息无效，正在停止获取执行端")
		}
		controlDone <- err
		cancel()
	}()
	listener, listenErr := net.Listen("tcp", config.Addr)
	if listenErr != nil {
		cancel()
		_ = input.Close()
		<-controlDone
		return errors.New("托管获取服务监听失败")
	}
	runErr := runWithListener(runCtx, config, logger, listener, func(service *Service, address string) error {
		if runCtx.Err() != nil {
			return nil
		}
		// 地址来自仍由本进程持有的真实监听器，不进行释放后重新绑定。
		return collectioncontrol.Write(output, collectioncontrol.Message{Version: collectioncontrol.Version, Action: "ready", Nonce: start.Nonce, InstanceID: start.InstanceID, Address: "http://" + address, ReadRoot: service.config.DataDir})
	})
	cancel()
	_ = input.Close()
	controlErr := <-controlDone
	result := errors.Join(runErr, controlErr)
	if result != nil {
		logger.Warn("桌面托管获取执行端退出检查未完整通过", "原因", result)
	} else {
		logger.Info("桌面托管获取执行端已结束并释放自有操作")
	}
	return result
}
