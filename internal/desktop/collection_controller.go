package desktop

import (
	"context"
	"strings"

	"nas-content-manager/internal/collectioncontract"
)

// setCollectionState 单独发布可选组件状态，不覆盖媒体服务的真实就绪地址。
func (c *controller) setCollectionState(state string, message string) {
	c.setCollectionCapabilities(state, message, nil)
}

// setCollectionCapabilities 同时发布状态与各插件快照，避免重检时保留上一实例能力。
func (c *controller) setCollectionCapabilities(state string, message string, capabilities []collectioncontract.Capability) {
	c.mu.Lock()
	c.reply.CollectionMode = c.config.CollectionMode
	c.reply.CollectionState = state
	c.reply.CollectionMessage = message
	c.reply.CollectionCapabilities = append([]collectioncontract.Capability(nil), capabilities...)
	c.mu.Unlock()
}

// collectionReadiness 根据预期插件的本地探测结果生成总体状态和脱敏说明。
func collectionReadiness(capabilities []collectioncontract.Capability) (string, string) {
	ready := []string{}
	unavailable := []string{}
	for _, capability := range capabilities {
		name := "获取"
		for _, plugin := range collectionPlugins {
			if plugin.kind == capability.Kind {
				name = plugin.name
				break
			}
		}
		if capability.Available {
			ready = append(ready, name)
			continue
		}
		reason := capability.Reason
		if reason == "" {
			reason = "本地依赖尚未就绪，请查看日志"
		}
		unavailable = append(unavailable, name+"："+reason)
	}
	if len(ready) > 0 {
		message := "本机" + strings.Join(ready, "、") + "依赖已就绪；站点连接在实际请求时验证"
		if len(unavailable) > 0 {
			message += "；" + strings.Join(unavailable, "；")
		}
		return "ready", message
	}
	if len(unavailable) > 0 {
		return "unavailable", strings.Join(unavailable, "；")
	}
	return "unavailable", "本机获取服务已启动，尚未配置漫画或视频私有运行时"
}

// collectionPluginState 为托盘固定插件生成短状态，不把未配置的可选插件标为损坏。
func collectionPluginState(reply controlReply, kind string) string {
	switch reply.CollectionState {
	case "disabled":
		return "已停用"
	case "external":
		return "外接服务"
	case "starting":
		return "正在核验"
	case "failed":
		return "未检测"
	}
	for _, capability := range reply.CollectionCapabilities {
		if capability.Kind == kind {
			if capability.Available {
				return "已就绪"
			}
			return "依赖未就绪"
		}
	}
	return "未配置"
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
		message := "本机资源获取已停用，可在托盘启用；已发布媒体仍可使用"
		c.setCollectionState("disabled", message)
		return values, message
	}
	c.setCollectionState("starting", "正在启动并核验本机获取组件")
	worker, err := c.startManagedCollection(ctx, cfg)
	if err != nil {
		c.logger.Warn("本机获取服务不可用，原有媒体保持可用", "原因", err)
		message := "本机获取服务启动失败，请在托盘查看日志或重新检测"
		c.setCollectionState("failed", message)
		return values, message
	}
	c.collection = worker
	c.collectionDone = worker.process.done
	values["NAS_COLLECTION_WORKER_URL"] = worker.address
	values["NAS_COLLECTION_WORKER_TOKEN_FILE"] = worker.tokenFile
	values["NAS_COLLECTION_READ_ROOT"] = worker.readRoot
	values["NAS_COLLECTION_WORKER_ALLOW_HTTP"] = "true"
	state, message := collectionReadiness(worker.capabilities)
	c.setCollectionCapabilities(state, message, worker.capabilities)
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
