package enginehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"nas-content-manager/internal/enginecontract"
)

// handleStopAll 只为本安装的两个固定引擎建立可恢复停止任务，不提供通用Docker操作。
func (s *Service) handleStopAll(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RequestID string `json:"requestId"` // 本次停止请求的幂等身份。
	}
	if !decode(w, r, &request) {
		return
	}
	if !safeID(request.RequestID) {
		failure(w, http.StatusBadRequest, "停止请求标识无效")
		return
	}
	components := []string{enginecontract.QBittorrent, enginecontract.Jackett}
	task, err := s.submit(r.Context(), request.RequestID, taskSignature("stop", components, "1"), components)
	if errors.Is(err, errBusy) {
		failure(w, http.StatusConflict, "初始化或启动仍在执行，请等待完成后停止完整实例")
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := s.store.response(r.Context(), task.ID, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	s.logger.Info("已接收本安装全部引擎停止请求，所有持久数据保留", "task", task.ID)
	respond(w, http.StatusAccepted, result)
}

// stopComponent 核对实际镜像、模板和完整挂载后仅停止原容器，绝不拉取、创建或删除。
func (s *Service) stopComponent(task *enginecontract.Task, index int) error {
	t, err := engineTemplate(task.Steps[index].Component)
	if err != nil {
		return err
	}
	if err = s.phase(task, index, "preflight", "正在核对本安装引擎身份与停止授权"); err != nil {
		return err
	}
	if err = s.preflight(s.ctx); err != nil {
		return err
	}
	name := containerName(s.store.installation, t.Component)
	existing, err := s.docker.inspect(s.ctx, name)
	if err != nil || existing == nil {
		return err
	}
	if err = validateContainer(existing, s.config, s.store.installation, t); err != nil {
		return err
	}
	imageID, ready, err := s.docker.imageReady(s.ctx, t)
	if err != nil {
		return err
	}
	if !ready || existing.Image != imageID || existing.ID == "" {
		return errors.New("受管引擎实际镜像无法验证，停止操作已拒绝")
	}
	if !existing.State.Running {
		return nil
	}
	if err = s.phase(task, index, "stopping", "正在停止已核对归属的原引擎，保留全部容器和数据卷"); err != nil {
		return err
	}
	// 使用已核实的容器ID，避免同名容器被并发替换后误停其他资源。
	stopErr := s.docker.callWithin(s.ctx, http.MethodPost, s.docker.api()+"/containers/"+url.PathEscape(existing.ID)+"/stop?t=20", nil, nil, 30*time.Second)
	actual, inspectErr := s.docker.inspect(s.ctx, name)
	if inspectErr != nil || actual == nil {
		return errors.New("引擎停止结果暂不可确认，请保留实例并重试核对")
	}
	if actual.ID != existing.ID || actual.Image != imageID {
		return errors.New("停止期间容器身份发生变化，已拒绝继续操作")
	}
	if err = validateContainer(actual, s.config, s.store.installation, t); err != nil {
		return err
	}
	if actual.State.Running {
		if stopErr != nil {
			return errors.New("引擎停止请求未完成，请检查Docker状态后重试")
		}
		return errors.New("Docker仍报告引擎运行中，不能宣称完整实例已停止")
	}
	return nil
}

// StopAll 通过既有私有Unix服务停止当前安装，不打开状态库、不取得第二把安装锁。
func StopAll(ctx context.Context, socket string) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if !filepath.IsAbs(socket) || filepath.Clean(socket) != socket || socket == string(filepath.Separator) {
		return errors.New("助手私有socket路径无效")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
	}, ResponseHeaderTimeout: 10 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	requestID, err := randomID()
	if err != nil {
		return errors.New("安全随机源不可用")
	}
	body, _ := json.Marshal(map[string]string{"requestId": requestID})
	response, err := stopRequest(ctx, client, http.MethodPost, "/api/engines/stop-all", body)
	if err != nil {
		return err
	}
	if !safeID(response.Task.ID) || response.Task.Action != "stop" {
		return errors.New("助手停止任务身份无效")
	}
	taskID := response.Task.ID
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if response.Task.State == "completed" {
			return nil
		}
		if response.Task.State == "failed" || response.Task.State == "interrupted" {
			return errors.New("部分受管引擎停止失败，请检查本实例助手日志；数据未删除")
		}
		select {
		case <-ctx.Done():
			return errors.New("等待引擎停止超时，任务仍可恢复；请勿宣称完整实例已停止")
		case <-ticker.C:
		}
		response, err = stopRequest(ctx, client, http.MethodGet, "/api/engines/initializations/"+url.PathEscape(taskID), nil)
		if err != nil {
			return err
		}
		if response.Task.ID != taskID || response.Task.Action != "stop" {
			return errors.New("助手返回了不一致的停止任务")
		}
	}
}

// stopRequest 使用有界私有协议读取停止状态，从不转发任意上游响应或错误体。
func stopRequest(ctx context.Context, client *http.Client, method string, path string, body []byte) (enginecontract.TaskResponse, error) {
	var result enginecontract.TaskResponse
	req, err := http.NewRequestWithContext(ctx, method, "http://helper"+path, bytes.NewReader(body))
	if err != nil {
		return result, errors.New("助手停止请求无效")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		return result, errors.New("现有助手私有服务不可用，未启动第二个助手或访问其他实例")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusConflict {
		return result, errors.New("初始化或启动仍在执行，请等待完成后停止完整实例")
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusAccepted {
		return result, errors.New("助手拒绝停止请求，请检查本实例状态")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 || json.Unmarshal(data, &result) != nil {
		return result, errors.New("助手停止状态响应无效或超过预算")
	}
	return result, nil
}
