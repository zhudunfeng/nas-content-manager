package enginehelper

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"nas-content-manager/internal/enginecontract"
)

// probeCollection 仅验证固定匿名健康协议，不认证管理员或改变组件设置。
func probeCollection(ctx context.Context, client *http.Client, base string, component string) error {
	path := "/ping"
	switch component {
	case enginecontract.MoviePilot:
		path = "/api/v1/system/global?token=moviepilot"
	case enginecontract.AutoBangumi:
		path = "/health"
	case enginecontract.Sonarr, enginecontract.Radarr:
	default:
		return errors.New("不支持的组件健康检查")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return errors.New("组件健康请求无效")
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("组件HTTP接口尚未可访问")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(data) > 64<<10 || response.StatusCode != http.StatusOK {
		return errors.New("组件HTTP健康响应尚未就绪")
	}
	switch component {
	case enginecontract.MoviePilot:
		var envelope struct {
			Success *bool `json:"success"` // 上游v3公开响应业务成功事实。
			Data    struct {
				Version string `json:"BACKEND_VERSION"` // 当前实际后端版本。
			} `json:"data"` // 仅读取公开产品版本。
		}
		if json.Unmarshal(data, &envelope) != nil || envelope.Success == nil || !*envelope.Success || strings.TrimPrefix(envelope.Data.Version, "v") != "3.2.8" {
			return errors.New("MoviePilot匿名接口或固定3.2.8版本尚未就绪")
		}
	case enginecontract.AutoBangumi:
		var health struct {
			Status  string `json:"status"`  // 固定健康状态ok。
			Version string `json:"version"` // 固定应用版本4.0.0。
			DBOK    *bool  `json:"db_ok"`   // 必须明确成功的数据库检查。
		}
		if json.Unmarshal(data, &health) != nil || health.Status != "ok" || health.DBOK == nil || !*health.DBOK || strings.TrimPrefix(health.Version, "v") != "4.0.0" {
			return errors.New("AutoBangumi匿名接口、数据库或固定4.0.0版本尚未就绪")
		}
	default:
		var ping struct {
			Status string `json:"status"` // Sonarr/Radarr固定匿名状态OK。
		}
		if json.Unmarshal(data, &ping) != nil || ping.Status != "OK" {
			return errors.New("资源组件匿名数据库健康检查尚未通过")
		}
	}
	return nil
}

// waitCollectionHTTP 有限等待匿名HTTP就绪，成功仍只代表待首次配置。
func (s *Service) waitCollectionHTTP(parent context.Context, t template) error {
	ctx, cancel := context.WithTimeout(parent, 180*time.Second)
	defer cancel()
	client := s.collectionHTTP
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if probeCollection(ctx, client, s.engineURL(t), t.Component) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			if parent.Err() != nil {
				return parent.Err()
			}
			return collectionErrorForTask(t.Component)
		case <-ticker.C:
		}
	}
}
