package enginehelper

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nas-content-manager/internal/enginecontract"
)

// probeHTTP 创建禁止重定向且不包含浏览器Cookie的有界引擎客户端。
func probeHTTP() *http.Client {
	return &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil, ResponseHeaderTimeout: 5 * time.Second, MaxIdleConns: 2, IdleConnTimeout: 15 * time.Second}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

// waitHealthy 在有限期限内验证配置与引擎，失败保留所有文件以供重试。
func (s *Service) waitHealthy(parent context.Context, t template, password string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	client := probeHTTP()
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		var err error
		key := ""
		if t.Component == enginecontract.QBittorrent {
			err = probeQB(ctx, client, s.engineURL(t), password)
		} else {
			key, err = s.readJackettKey()
			if err == nil {
				err = probeJackett(ctx, client, s.engineURL(t), key)
			}
		}
		if err == nil {
			return key, nil
		}
		select {
		case <-ctx.Done():
			if parent.Err() != nil {
				return "", parent.Err()
			}
			if t.Component == enginecontract.QBittorrent {
				return "", errors.New("qB认证或固定5.2.4版本检查超时，请保留配置后重试；不会关闭认证或读取临时密码")
			}
			return "", errors.New("Jackett固定配置或接口就绪检查超时，请保留配置后重试")
		case <-ticker.C:
		}
	}
}

// probeQB 验证固定版本永久密码、真实会话Cookie和后续认证请求。
func probeQB(ctx context.Context, client *http.Client, base string, password string) error {
	form := url.Values{"username": {"admin"}, "password": {password}}
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("qB探针请求无效")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", base)
	req.Header.Set("Referer", base+"/")
	res, err := client.Do(req)
	if err != nil {
		return errors.New("qB连接尚未就绪")
	}
	data, readErr := io.ReadAll(io.LimitReader(res.Body, 1025))
	res.Body.Close()
	if readErr != nil || len(data) > 1024 || (res.StatusCode != 200 && res.StatusCode != 204) || (strings.TrimSpace(string(data)) != "" && strings.TrimSpace(string(data)) != "Ok.") {
		return errors.New("qB认证尚未成功")
	}
	var session *http.Cookie
	for _, c := range res.Cookies() {
		valid := c.Name == "SID"
		if strings.HasPrefix(c.Name, "QBT_SID_") {
			port, portErr := strconv.Atoi(strings.TrimPrefix(c.Name, "QBT_SID_"))
			valid = portErr == nil && port > 0 && port < 65536
		}
		if valid && c.Value != "" {
			session = c
			break
		}
	}
	if session == nil {
		return errors.New("qB认证缺少有效会话")
	}
	req, err = http.NewRequestWithContext(ctx, "GET", base+"/api/v2/app/version", nil)
	if err != nil {
		return errors.New("qB版本探针无效")
	}
	req.AddCookie(session)
	req.Header.Set("Origin", base)
	req.Header.Set("Referer", base+"/")
	res, err = client.Do(req)
	if err != nil {
		return errors.New("qB版本探针连接失败")
	}
	defer res.Body.Close()
	data, err = io.ReadAll(io.LimitReader(res.Body, 129))
	if err != nil || res.StatusCode != 200 || strings.TrimSpace(string(data)) != "v5.2.4" && strings.TrimSpace(string(data)) != "5.2.4" {
		return errors.New("qB版本不兼容，固定元数据保护只验证5.2.4")
	}
	return nil
}

// probeJackett 仅用Key检查固定Torznab发现协议，不执行关键词搜索或管理API。
func probeJackett(ctx context.Context, client *http.Client, base string, key string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/api/v2.0/indexers/all/results/torznab/api?t=indexers&configured=true&apikey="+url.QueryEscape(key), nil)
	if err != nil {
		return errors.New("Jackett探针无效")
	}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("Jackett尚未就绪")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("Jackett发现接口尚未就绪")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return errors.New("Jackett发现响应超过预算")
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	root := false
	for {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			break
		}
		if tokenErr != nil {
			return errors.New("Jackett发现XML无效")
		}
		switch value := token.(type) {
		case xml.Directive:
			return errors.New("Jackett发现XML不允许DTD")
		case xml.StartElement:
			if !root {
				if value.Name.Local != "indexers" {
					return errors.New("Jackett发现结构不兼容")
				}
				root = true
			}
		}
	}
	if !root {
		return errors.New("Jackett发现结果为空")
	}
	return nil
}
