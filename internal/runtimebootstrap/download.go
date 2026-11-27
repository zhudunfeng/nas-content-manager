package runtimebootstrap

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// errDownloadRetryable 仅标识可有限重试的瞬时网络或上游服务故障。
var errDownloadRetryable = errors.New("组件依赖下载遇到临时网络或服务故障")

// errUntrustedRedirect 标识来源策略拒绝，不允许以网络重试掩盖该失败。
var errUntrustedRedirect = errors.New("组件依赖下载重定向不符合固定可信来源")

// validHTTPSURL 拒绝用户信息、端口、片段、非 HTTPS 和非规范主机。
func validHTTPSURL(value *url.URL) bool {
	return value != nil && value.Scheme == "https" && value.User == nil && value.Fragment == "" && value.Opaque == "" && value.Host == value.Hostname() && value.Host != "" && strings.ToLower(value.Host) == value.Host
}

// validArtifactURL 仅接受编译期官方来源且无查询串的原始材料 URL。
func validArtifactURL(raw string) bool {
	value, err := url.Parse(raw)
	if err != nil || !validHTTPSURL(value) || value.RawQuery != "" || value.ForceQuery {
		return false
	}
	switch value.Host {
	case "www.python.org", "files.pythonhosted.org", "github.com", "www.gyan.dev":
		return true
	default:
		return false
	}
}

// trustedRedirect 仅允许 GitHub 官方发布链转到已审查的 HTTPS CDN。
func trustedRedirect(request *http.Request, via []*http.Request) error {
	failure := errUntrustedRedirect
	if len(via) == 0 || len(via) > 5 || !validHTTPSURL(request.URL) || via[0].URL.Host != "github.com" || !validArtifactURL(via[0].URL.String()) {
		return failure
	}
	switch request.URL.Host {
	case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
		return nil
	default:
		return failure
	}
}

// newDownloadClient 使用标准 TLS 和既有受信代理，固定连接及总请求时间预算。
func newDownloadClient() (*http.Client, *http.Transport) {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,                                                         // 继承父进程已配置代理，不新增全局设置。
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext, // 连接最多等待30秒。
		ForceAttemptHTTP2:     true,                                                                              // 官方 HTTPS 连接允许协商 HTTP/2。
		TLSHandshakeTimeout:   15 * time.Second,                                                                  // TLS 握手最多等待15秒。
		ResponseHeaderTimeout: 30 * time.Second,                                                                  // 响应头最多等待30秒。
		ExpectContinueTimeout: time.Second,                                                                       // 保持标准有限等待行为。
		IdleConnTimeout:       30 * time.Second,                                                                  // 空闲连接及时释放。
		MaxIdleConns:          4,                                                                                 // 串行材料下载不积累连接。
		MaxConnsPerHost:       2,                                                                                 // 限制单主机并发连接。
		DisableCompression:    true,                                                                              // 验证原始材料字节，不接受透明解压改变长度。
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Minute, CheckRedirect: trustedRedirect}, transport
}

// downloadArtifact 对瞬态故障最多尝试3次，退避可取消，完整性或清理失败不重试。
func downloadArtifact(ctx context.Context, client *http.Client, target string, source artifact, report func(Progress)) error {
	for attempt := 0; ; attempt++ {
		err := downloadAttempt(ctx, client, target, source, report)
		if ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		}
		if err == nil || attempt >= 2 || !errors.Is(err, errDownloadRetryable) || errors.Is(err, ErrCleanup) {
			return err
		}
		progress(report, "组件依赖下载暂时失败，正在有限重试："+source.Name, 0, source.Size)
		delay := time.Second // 第一次重试仅等待1秒，避免短暂瞬断导致整次初始化失败。
		if attempt == 1 {
			delay = 3 * time.Second // 最后一次重试等待3秒，不循环或无限重试。
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// downloadAttempt 下载精确字节到本次专用文件，所有网络错误均转换为脱敏中文。
func downloadAttempt(ctx context.Context, client *http.Client, target string, source artifact, report func(Progress)) (resultErr error) {
	if !validArtifactURL(source.URL) || !artifactNamePattern.MatchString(source.Name) {
		return errors.New("组件依赖下载来源不受信，已拒绝连接")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return errors.New("无法创建固定组件依赖下载请求")
	}
	request.Header.Set("User-Agent", "NAS-Content-Manager-Runtime-Bootstrap/1")
	request.Header.Set("Accept-Encoding", "identity")
	stage := "正在下载组件依赖：" + source.Name
	progress(report, stage, 0, source.Size)
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, errUntrustedRedirect) {
			return errUntrustedRedirect
		}
		return errors.Join(errDownloadRetryable, errors.New("组件依赖 "+source.Name+" 下载失败或超时，请检查网络或代理后重新检测"))
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			resultErr = errors.Join(resultErr, ErrCleanup, errors.New("组件依赖下载连接未能完整关闭，请重新检测"))
		}
	}()
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 && response.StatusCode <= 599 {
		return errors.Join(errDownloadRetryable, errors.New("组件依赖 "+source.Name+" 下载服务暂时不可用，请稍后重新检测"))
	}
	if response.StatusCode != http.StatusOK || response.ContentLength >= 0 && response.ContentLength != source.Size || response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" {
		return errors.New("组件依赖 " + source.Name + " 下载响应状态、编码或文件长度不匹配")
	}
	last := time.Time{}
	update := func(downloaded int64) {
		if time.Since(last) >= 200*time.Millisecond || downloaded == source.Size {
			progress(report, stage, downloaded, source.Size)
			last = time.Now()
		}
	}
	err = writeExpected(ctx, target, response.Body, fileRecord{SHA256: source.SHA256, Size: source.Size}, update)
	if errors.Is(err, errSourceRead) && !errors.Is(err, ErrCleanup) {
		return errors.Join(errDownloadRetryable, err)
	}
	return err
}
