package desktop

import "context"

// setCollectionState 单独发布可选组件状态，不覆盖媒体服务的真实就绪地址。
func (c *controller) setCollectionState(state string, message string) {
	c.mu.Lock()
	c.reply.CollectionMode = c.config.CollectionMode
	c.reply.CollectionState = state
	c.reply.CollectionMessage = message
	c.mu.Unlock()
}

// startCollection 为当前启动创建成组连接快照，缺失可选组件只降级获取功能。
func (c *controller) startCollection(ctx context.Context, cfg Config) (map[string]string, string) {
	if cfg.CollectionMode == "external" {
		c.setCollectionState("external", "使用管理员配置的外接获取服务，不由本机启动器启停")
		return map[string]string{}, ""
	}
	values := map[string]string{
		"NAS_COLLECTION_WORKER_URL":        "",
		"NAS_COLLECTION_WORKER_TOKEN_FILE": "",
		"NAS_COLLECTION_READ_ROOT":         "",
		"NAS_COLLECTION_WORKER_ALLOW_HTTP": "false",
	}
	if cfg.CollectionMode == "disabled" {
		message := "本机漫画获取已停用，可在托盘启用；已发布漫画仍可阅读"
		c.setCollectionState("disabled", message)
		return values, message
	}
	c.setCollectionState("starting", "正在启动并核验本机漫画组件")
	worker, err := c.startManagedCollection(ctx, cfg)
	if err != nil {
		c.logger.Warn("本机获取服务不可用，原有媒体保持可用", "原因", err)
		message := "本机漫画获取服务启动失败，请在托盘查看日志或重新检测"
		c.setCollectionState("failed", message)
		return values, message
	}
	c.collection = worker
	c.collectionDone = worker.process.done
	values["NAS_COLLECTION_WORKER_URL"] = worker.address
	values["NAS_COLLECTION_WORKER_TOKEN_FILE"] = worker.tokenFile
	values["NAS_COLLECTION_READ_ROOT"] = worker.readRoot
	values["NAS_COLLECTION_WORKER_ALLOW_HTTP"] = "true"
	if worker.capability.Available {
		c.setCollectionState("ready", "本机漫画依赖已就绪；站点连接在实际请求时验证")
	} else {
		message := worker.capability.Reason
		if message == "" {
			message = "本机获取服务已启动，但漫画依赖尚未就绪，请查看日志"
		}
		c.setCollectionState("unavailable", message)
	}
	return values, ""
}

// stopCollection 等待私有 worker 与 Python 全部退出；永不操作外接服务。
func (c *controller) stopCollection() error {
	if c.collection == nil {
		return nil
	}
	c.collectionDone = nil
	err := c.collection.stop()
	if err != nil {
		c.logger.Error("本机获取服务未能正常完整停止", "原因", err)
		select {
		case <-c.collection.process.done:
			if c.collection.process.err == nil {
				// 超时事实仍返回调用方；已确认资源释放后允许用户下一次主动重检。
				c.collection = nil
			}
		default:
		}
		return err
	}
	if c.collection.process.exitErr != nil {
		c.logger.Warn("本机获取服务此前异常退出，已确认自有进程与句柄释放，可主动重新检测", "原因", c.collection.process.exitErr)
	}
	c.collection = nil
	c.logger.Info("本机获取服务与自有子进程已完整停止")
	return nil
}

// changeCollectionMode 将本机用户的固定模式选择交给串行重启流程。
func (c *controller) changeCollectionMode(mode string) {
	cfg, _ := c.snapshot()
	if cfg.Mode != "native" || !validCollectionMode(mode) {
		return
	}
	cfg.CollectionMode = mode
	c.request(desktopCommand{Kind: "config", Config: &cfg})
}
