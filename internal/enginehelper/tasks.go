package enginehelper

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"nas-content-manager/internal/enginecontract"
)

// worker 是独立于媒体扫描和HTTP请求的唯一部署执行器。
func (s *Service) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case id := <-s.queue:
			s.opMu.Lock()
			s.running = id
			s.opMu.Unlock()
			s.execute(id)
			s.opMu.Lock()
			s.running = ""
			s.opMu.Unlock()
		}
	}
}

// phase 先持久化部署意图，再允许下一项网络或文件系统副作用。
func (s *Service) phase(t *enginecontract.Task, index int, phase string, message string) error {
	t.State = "running"
	t.Message = message
	t.Steps[index].State = "running"
	t.Steps[index].Phase = phase
	t.Steps[index].Message = message
	if err := s.store.saveTask(s.ctx, t); err != nil {
		return errors.New("无法持久化部署步骤，已停止后续操作")
	}
	if err := s.store.event(s.ctx, t.ID, t.Steps[index].Component, "info", message); err != nil {
		return errors.New("无法持久化部署事件，已停止后续操作")
	}
	s.logger.Info(message, "task", t.ID, "component", t.Steps[index].Component)
	return nil
}

// execute 顺序执行两个独立组件，失败保留成功组件和所有持久目录。
func (s *Service) execute(id string) {
	t, err := s.store.getTask(s.ctx, id)
	if err != nil || t.State != "queued" && t.State != "running" {
		return
	}
	for index := range t.Steps {
		if s.ctx.Err() != nil {
			return
		}
		if t.Steps[index].State == "completed" || t.Steps[index].State == "failed" {
			continue
		}
		err = s.executeComponent(&t, index)
		if s.ctx.Err() != nil {
			return
		}
		if err != nil {
			t.Steps[index].State = "failed"
			t.Steps[index].Message = err.Error()
			t.Message = err.Error()
			if saveErr := s.store.saveTask(s.ctx, &t); saveErr != nil {
				s.logger.Error("初始化失败状态无法持久化", "task", id)
				return
			}
			_ = s.store.event(s.ctx, id, t.Steps[index].Component, "error", err.Error())
			s.logger.Warn("引擎初始化阶段失败", "task", id, "component", t.Steps[index].Component, "reason", err.Error())
		} else {
			t.Steps[index].State = "completed"
			t.Steps[index].Phase = "completed"
			t.Steps[index].Message = "部署与私有连接发布完成，等待应用接入"
			if err = s.store.saveTask(s.ctx, &t); err != nil {
				return
			}
			_ = s.store.event(s.ctx, id, t.Steps[index].Component, "info", t.Steps[index].Message)
		}
	}
	t.State = "completed"
	t.Message = "引擎部署完成，应用将自动验证接入；搜索仍需配置站点来源"
	for _, step := range t.Steps {
		if step.State != "completed" {
			t.State = "failed"
			t.Message = "部分引擎初始化失败，成功组件已保留，可重试失败部分"
		}
	}
	if err = s.store.saveTask(s.ctx, &t); err != nil {
		s.logger.Error("初始化最终状态无法持久化", "task", id)
		return
	}
	_ = s.store.event(s.ctx, id, "", "info", t.Message)
	_ = s.store.pruneHistory(s.ctx)
	s.logger.Info(t.Message, "task", id)
}

// executeComponent 核对固定资源并恢复不确定create/start结果，不盲目重复创建。
func (s *Service) executeComponent(task *enginecontract.Task, index int) error {
	t, err := engineTemplate(task.Steps[index].Component)
	if err != nil {
		return err
	}
	if err = s.phase(task, index, "preflight", "正在检查本机Docker、网络与专用目录"); err != nil {
		return err
	}
	if err = s.preflight(s.ctx); err != nil {
		return err
	}
	if t.Component == enginecontract.Jackett {
		if err = verifyJackettWrite(filepath.Join(s.config.Root, "connections")); err != nil {
			return err
		}
	}
	if err = s.ownedConfig(t.Component, false); err != nil {
		return errConflict
	}
	name := containerName(s.store.installation, t.Component)
	existing, err := s.docker.inspect(s.ctx, name)
	if err != nil {
		return err
	}
	if existing != nil {
		if err = validateContainer(existing, s.config, s.store.installation, t); err != nil {
			return err
		}
	} else if task.Action == "start" {
		return errors.New("原受管容器已不存在，请刷新检测；启动操作不会创建替代容器")
	}
	imageID, ready, err := s.docker.imageReady(s.ctx, t)
	if err != nil {
		return err
	}
	if !ready {
		if task.Action == "start" {
			return errors.New("原受管镜像不可验证，启动操作不会重新安装")
		}
		if err = s.phase(task, index, "pulling", "正在拉取固定摘要镜像；下方仅显示真实镜像层进度"); err != nil {
			return err
		}
		progressErr := error(nil)
		err = s.docker.pull(s.ctx, t, func(layers []enginecontract.Layer) {
			if progressErr != nil {
				return
			}
			task.Steps[index].Layers = layers
			progressErr = s.store.saveTask(s.ctx, task)
		})
		if err != nil {
			return err
		}
		if progressErr != nil {
			return errors.New("镜像进度无法持久化，请重试")
		}
		imageID, ready, err = s.docker.imageReady(s.ctx, t)
		if err != nil {
			return err
		}
		if !ready {
			return errors.New("目标镜像尚未完成验证")
		}
	}
	if existing != nil && existing.Image != imageID {
		return errors.New("受管容器实际镜像内容与固定摘要不符")
	}
	if err = s.phase(task, index, "preparing", "正在验证配置归属与目标UID读写权限"); err != nil {
		return err
	}
	if err = s.ownedConfig(t.Component, true); err != nil {
		return errConflict
	}
	if err = verifyTargetAccess(filepath.Join(s.config.Root, t.Component), s.config.UID, s.config.GID); err != nil {
		return err
	}
	var password string
	if t.Component == enginecontract.QBittorrent {
		for _, path := range []string{"downloads", "downloads/staging", "downloads/complete"} {
			dir := filepath.Join(s.config.Root, filepath.FromSlash(path))
			if err = ensureDirectory(dir, 0755, s.config.UID, s.config.GID); err != nil {
				return err
			}
			if err = verifyTargetAccess(dir, s.config.UID, s.config.GID); err != nil {
				return err
			}
		}
		password, err = s.prepareQB()
		if err != nil {
			return err
		}
	}
	if existing == nil {
		if err = s.phase(task, index, "creating", "正在创建固定模板容器，已保存可恢复意图"); err != nil {
			return err
		}
		spec := makeSpec(s.config, s.store.installation, t)
		existing, err = s.createAndReconcile(s.ctx, t, spec, imageID)
		if err != nil {
			return err
		}
	}
	if !existing.State.Running {
		if err = s.phase(task, index, "starting", "正在启动已核对归属的原受管容器"); err != nil {
			return err
		}
		startErr := s.docker.call(s.ctx, http.MethodPost, s.docker.api()+"/containers/"+s.escapedName(t.Component)+"/start", nil, nil)
		existing, err = s.docker.inspect(s.ctx, name)
		if err != nil || existing == nil {
			return errors.New("启动结果暂不可确认，请重试对账")
		}
		if err = validateContainer(existing, s.config, s.store.installation, t); err != nil {
			return err
		}
		if !existing.State.Running {
			if startErr != nil {
				return errors.New("容器启动失败，请检查固定端口是否占用及Docker资源")
			}
			return errors.New("容器未进入运行状态，请保留配置后检查引擎")
		}
	}
	if err = s.phase(task, index, "credentials", "正在读取本安装私有凭据，不读取或传递容器日志"); err != nil {
		return err
	}
	if err = s.phase(task, index, "healthcheck", "正在验证引擎认证、版本与固定接口就绪状态"); err != nil {
		return err
	}
	key, err := s.waitHealthy(s.ctx, t, password)
	if err != nil {
		return err
	}
	if err = s.phase(task, index, "publishing", "正在原子发布私有连接，应用将独立验证并接入"); err != nil {
		return err
	}
	if err = s.publish(t, existing.ID, password, key); err != nil {
		return err
	}
	return nil
}

// createAndReconcile 在一次create后始终inspect；响应丢失不触发第二次create。
func (s *Service) createAndReconcile(ctx context.Context, t template, spec containerSpec, imageID string) (*containerInfo, error) {
	var created struct {
		ID string `json:"Id"`
	}
	createErr := s.docker.call(ctx, http.MethodPost, s.docker.api()+"/containers/create?name="+url.QueryEscape(containerName(s.store.installation, t.Component)), spec, &created)
	actual, err := s.docker.inspect(ctx, containerName(s.store.installation, t.Component))
	if err != nil {
		return nil, errors.New("创建结果暂不可确认，请重试对账；不会重复创建")
	}
	if actual == nil {
		if createErr != nil {
			return nil, errors.New("容器创建未完成，请检查端口、权限或Docker资源后重试")
		}
		return nil, errors.New("容器创建响应与实际资源不一致，请重试对账")
	}
	if err = validateContainer(actual, s.config, s.store.installation, t); err != nil {
		return nil, err
	}
	if actual.Image != imageID {
		return nil, errors.New("新建容器实际镜像内容不符，已保留以便人工检查")
	}
	return actual, nil
}

// reconcileAfterRestart 保留未完成任务授权；完成实例仅检测，绝不因服务重启自动拉起。
func (s *Service) reconcileAfterRestart(ctx context.Context) error {
	active, err := s.store.activeTask(ctx)
	if err != nil {
		return err
	}
	if active != nil {
		select {
		case s.queue <- active.ID:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// fileExists 仅用于判定受管普通文件是否已存在，不跟随符号链接。
func fileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}
