package collectionworker

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// uavRelayResource 只将一次已解析的固定HLS资源映射到不可猜测路径。
type uavRelayResource struct {
	URL      string            // 原始资源地址，不暴露给下载进程。
	Headers  map[string]string // 固定Referer和Origin，不包含CDN访问值。
	CDN      *uavCDN           // 原始CDN作用域，仅在每跳匹配后注入。
	Context  context.Context   // 所属操作生命周期，取消立即撤销网络访问。
	MaxBytes int64             // 单响应上限，实际暂存另有总预算监测。
	Key      bool              // 普通AES-128密钥必须完整且恰为16字节。
}

// uavManifestURI 仅识别受上游m3u8解析验证过的双引号URI属性。
var uavManifestURI = regexp.MustCompile(`URI="([^"\r\n]*)"`)

// uavRequestRange 和 uavResponseRange 仅支持明确的单字节区间，不允许后缀或多区间语义。
var uavRequestRange = regexp.MustCompile(`^bytes=([0-9]+)-([0-9]+)$`)
var uavResponseRange = regexp.MustCompile(`^bytes ([0-9]+)-([0-9]+)/([0-9]+)$`)

// parseUAVRange 在访问上游前限制唯一明确的起止位置及长度预算。
func parseUAVRange(value string, maxBytes int64) (int64, int64, error) {
	parts := uavRequestRange.FindStringSubmatch(value)
	if len(parts) != 3 {
		return 0, 0, errors.New("视频资源字节范围无效")
	}
	start, startErr := strconv.ParseInt(parts[1], 10, 64)
	end, endErr := strconv.ParseInt(parts[2], 10, 64)
	if startErr != nil || endErr != nil || start > end || maxBytes < 1 || end-start >= maxBytes {
		return 0, 0, errors.New("视频资源字节范围超限")
	}
	return start, end, nil
}

// validUAVRangeResponse 拒绝CDN忽略范围或返回错段，不能由下载器自行猜测完整性。
func validUAVRangeResponse(response *http.Response, start int64, end int64) bool {
	parts := uavResponseRange.FindStringSubmatch(response.Header.Get("Content-Range"))
	if response.StatusCode != http.StatusPartialContent || len(parts) != 4 || response.ContentLength >= 0 && response.ContentLength != end-start+1 {
		return false
	}
	actualStart, startErr := strconv.ParseInt(parts[1], 10, 64)
	actualEnd, endErr := strconv.ParseInt(parts[2], 10, 64)
	total, totalErr := strconv.ParseInt(parts[3], 10, 64)
	return startErr == nil && endErr == nil && totalErr == nil && actualStart == start && actualEnd == end && total > end
}

// validUAVCDN 独立限制短期访问元数据，不允许注入额外头或跨源扩展。
func validUAVCDN(cdn uavCDN) bool {
	u, err := url.Parse(cdn.Origin)
	if err != nil || !validUAVURL(cdn.Origin) || u.Scheme != "https" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || len(cdn.Auth) < 1 || len(cdn.Auth) > 2048 || len(cdn.App) < 1 || len(cdn.App) > 64 {
		return false
	}
	for _, ch := range cdn.Auth + cdn.App {
		if ch <= 32 || ch >= 127 {
			return false
		}
	}
	return true
}

// uavSameOrigin 按HTTPS域名和有效端口比较，不按字符串前缀授予权限。
func uavSameOrigin(left *url.URL, right *url.URL) bool {
	leftPort, rightPort := left.Port(), right.Port()
	if leftPort == "" {
		leftPort = "443"
	}
	if rightPort == "" {
		rightPort = "443"
	}
	return left.Scheme == "https" && right.Scheme == "https" && strings.EqualFold(left.Hostname(), right.Hostname()) && leftPort == rightPort
}

// uavMediaHeaders 每一跳重新计算访问头，跨源和不允许扩展名绝不携带CDN秘密。
func uavMediaHeaders(resource uavRelayResource, target *url.URL) http.Header {
	headers := http.Header{}
	for key, value := range resource.Headers {
		headers.Set(key, value)
	}
	headers.Set("User-Agent", "NAS-Collection-UAV/1")
	if resource.CDN == nil {
		return headers
	}
	origin, err := url.Parse(resource.CDN.Origin)
	if err != nil || !uavSameOrigin(origin, target) {
		return headers
	}
	ext := strings.ToLower(path.Ext(target.Path))
	if ext == ".m3u8" && resource.CDN.Manifest || (ext == ".ts" || ext == ".m4s" || ext == ".key") && resource.CDN.Other {
		headers.Set("x-cdn-auth", resource.CDN.Auth)
		headers.Set("x-cdn-app", resource.CDN.App)
	}
	return headers
}

// registerUAVMedia 把有限清单内的资源改写为操作级虚拟地址，不向yt-dlp提供CDN秘密。
func (p *publicProxy) registerUAVMedia(ctx context.Context, media uavMedia, maxBytes int64) (uavMedia, func(), error) {
	noop := func() {}
	base, err := url.Parse(media.MediaURL)
	if err != nil || base.Scheme != "https" || media.Manifest == "" || maxBytes < 1 {
		return media, noop, errors.New("视频媒体中继输入无效")
	}
	scope := "/" + rand.Text() + "/"
	routes := map[string]uavRelayResource{}
	seen := map[string]string{}
	var rewriteErr error
	// 资源白名单只从固定桥接验证后的清单生成，调用方不能借中继任意拼接URL。
	rewrite := func(raw string, key bool) string {
		if rewriteErr != nil {
			return ""
		}
		target, parseErr := url.Parse(raw)
		if parseErr != nil {
			rewriteErr = errors.New("视频清单资源地址无效")
			return ""
		}
		target = base.ResolveReference(target)
		if target.Scheme != "https" || !validUAVURL(target.String()) {
			rewriteErr = errors.New("视频清单资源协议无效")
			return ""
		}
		if existing, ok := seen[target.String()]; ok {
			if key {
				resourcePath := strings.TrimPrefix(existing, "http://uav-media.invalid")
				resource := routes[resourcePath]
				resource.Key = true
				routes[resourcePath] = resource
			}
			return existing
		}
		if len(routes) >= 20002 {
			rewriteErr = errors.New("视频清单资源数量超限")
			return ""
		}
		resourcePath := scope + strconv.Itoa(len(routes))
		routes[resourcePath] = uavRelayResource{URL: target.String(), Headers: media.Headers, CDN: media.CDN, Context: ctx, MaxBytes: maxBytes, Key: key}
		relayURL := "http://uav-media.invalid" + resourcePath
		seen[target.String()] = relayURL
		return relayURL
	}
	lines := strings.Split(strings.ReplaceAll(media.Manifest, "\r\n", "\n"), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			key := strings.HasPrefix(trimmed, "#EXT-X-KEY:") && strings.Contains(trimmed, "METHOD=AES-128")
			lines[index] = uavManifestURI.ReplaceAllStringFunc(line, func(match string) string {
				parts := uavManifestURI.FindStringSubmatch(match)
				return `URI="` + rewrite(parts[1], key) + `"`
			})
		} else {
			lines[index] = rewrite(trimmed, false)
		}
	}
	if rewriteErr != nil {
		return media, noop, rewriteErr
	}
	if len(routes) == 0 {
		return media, noop, errors.New("视频清单缺少媒体资源")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || ctx.Err() != nil {
		return media, noop, errors.New("视频媒体操作已结束")
	}
	if p.uavRoutes == nil {
		p.uavRoutes = map[string]uavRelayResource{}
	}
	for key, value := range routes {
		p.uavRoutes[key] = value
	}
	cleanup := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		for key := range routes {
			delete(p.uavRoutes, key)
		}
	}
	media.Manifest = strings.Join(lines, "\n")
	media.MediaURL = "http://uav-media.invalid" + scope + "manifest.m3u8"
	media.Headers = map[string]string{}
	media.CDN = nil
	return media, cleanup, nil
}

// serveUAVMedia 用标准HTTPS传输逐跳获取已登记资源，CONNECT隧道不参与注入。
func (p *publicProxy) serveUAVMedia(w http.ResponseWriter, r *http.Request) {
	if (r.Method != "GET" && r.Method != "HEAD") || r.URL.RawQuery != "" || r.ContentLength > 0 {
		http.Error(w, "media rejected", http.StatusForbidden)
		return
	}
	p.mu.Lock()
	resource, ok := p.uavRoutes[r.URL.Path]
	p.mu.Unlock()
	if !ok || resource.Context.Err() != nil {
		http.Error(w, "media unavailable", http.StatusNotFound)
		return
	}
	requestRange := r.Header.Get("Range")
	var rangeStart, rangeEnd int64
	if requestRange != "" {
		var rangeErr error
		rangeStart, rangeEnd, rangeErr = parseUAVRange(requestRange, resource.MaxBytes)
		if rangeErr != nil {
			http.Error(w, "range rejected", http.StatusBadRequest)
			return
		}
	}
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(resource.Context, cancel)
	defer func() { stop(); cancel() }()
	target, err := url.Parse(resource.URL)
	if err != nil {
		http.Error(w, "media rejected", http.StatusForbidden)
		return
	}
	for redirect := 0; redirect < 6; redirect++ {
		if target.Scheme != "https" || !validUAVURL(target.String()) {
			http.Error(w, "media rejected", http.StatusForbidden)
			return
		}
		request, createErr := http.NewRequestWithContext(ctx, r.Method, target.String(), nil)
		if createErr != nil {
			http.Error(w, "media rejected", http.StatusForbidden)
			return
		}
		request.Header = uavMediaHeaders(resource, target)
		if requestRange != "" {
			request.Header.Set("Range", requestRange)
		}
		response, transportErr := p.transport.RoundTrip(request)
		if transportErr != nil {
			http.Error(w, "media unavailable", http.StatusBadGateway)
			return
		}
		if response.StatusCode == 301 || response.StatusCode == 302 || response.StatusCode == 303 || response.StatusCode == 307 || response.StatusCode == 308 {
			location := response.Header.Get("Location")
			response.Body.Close()
			if location == "" {
				http.Error(w, "media unavailable", http.StatusBadGateway)
				return
			}
			next, parseErr := target.Parse(location)
			if parseErr != nil {
				http.Error(w, "media rejected", http.StatusForbidden)
				return
			}
			target = next
			continue
		}
		defer response.Body.Close()
		if response.StatusCode != 200 && response.StatusCode != 206 {
			http.Error(w, "media unavailable", http.StatusBadGateway)
			return
		}
		if requestRange != "" && !validUAVRangeResponse(response, rangeStart, rangeEnd) || requestRange == "" && response.StatusCode != http.StatusOK {
			http.Error(w, "media range rejected", http.StatusBadGateway)
			return
		}
		if response.ContentLength > resource.MaxBytes {
			http.Error(w, "media too large", http.StatusRequestEntityTooLarge)
			return
		}
		if resource.Key {
			// AES-128仅允许完整16字节密钥，不能让下载器接受24/32字节AES密钥。
			key, keyErr := io.ReadAll(io.LimitReader(response.Body, 17))
			if r.Method != "GET" || r.Header.Get("Range") != "" || response.StatusCode != http.StatusOK || keyErr != nil || len(key) != 16 {
				http.Error(w, "media key rejected", http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", "16")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(key)
			return
		}
		for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
			if value := response.Header.Get(header); value != "" {
				w.Header().Set(header, value)
			}
		}
		if requestRange != "" {
			w.Header().Set("Content-Length", strconv.FormatInt(rangeEnd-rangeStart+1, 10))
		}
		w.WriteHeader(response.StatusCode)
		if r.Method != "HEAD" {
			limit := resource.MaxBytes
			if requestRange != "" {
				limit = rangeEnd - rangeStart + 1
				// 保留最后一字节至上游完整结束，防止下载器先满足Content-Length而忽略随后中止。
				if _, copyErr := io.CopyN(w, response.Body, limit-1); copyErr != nil {
					panic(http.ErrAbortHandler)
				}
				tail, tailErr := io.ReadAll(io.LimitReader(response.Body, 2))
				if tailErr != nil || len(tail) != 1 {
					panic(http.ErrAbortHandler)
				}
				if _, writeErr := w.Write(tail); writeErr != nil {
					panic(http.ErrAbortHandler)
				}
				return
			}
			written, copyErr := io.Copy(w, io.LimitReader(response.Body, limit+1))
			if copyErr != nil || written > limit {
				// 响应头已发送后必须中止连接，不能把缺片或超预算正文包装为正常结束。
				panic(http.ErrAbortHandler)
			}
		}
		return
	}
	http.Error(w, "media redirects rejected", http.StatusBadGateway)
}
