package collectionworker

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

// publicProxy 对网页工具每一次实际连接实施DNS和IP策略，禁止绕过代理重定向到私网。
type publicProxy struct {
	server      *http.Server      // 仅绑定本机环回临时端口。
	transport   *http.Transport   // 禁用环境代理并使用经检查的连接函数。
	url         string            // 只在工具私密环境中使用的认证代理地址。
	auth        string            // 随机代理认证值。
	mu          sync.Mutex        // 保护关闭准入和隧道连接。
	closed      bool              // 禁止关闭后加入新隧道。
	connections map[net.Conn]bool // 已劫持的CONNECT连接归属。
	wg          sync.WaitGroup    // 等待隧道处理结束。
	slots       chan struct{}     // 限制网页并发连接数。
}

// newPublicProxy 建立默认拒绝私网的本机代理，工具必须显式使用该代理。
func newPublicProxy(ctx context.Context) (*publicProxy, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	secret := rand.Text()
	p := &publicProxy{connections: map[net.Conn]bool{}, slots: make(chan struct{}, 24), auth: "Basic " + base64.StdEncoding.EncodeToString([]byte("worker:"+secret))}
	p.url = "http://worker:" + secret + "@" + l.Addr().String()
	p.transport = &http.Transport{Proxy: nil, DialContext: publicDial, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second, MaxIdleConns: 8, MaxConnsPerHost: 8, IdleConnTimeout: 30 * time.Second, DisableCompression: true}
	p.server = &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() { _ = p.server.Serve(l) }()
	return p, nil
}

// ServeHTTP 只代理HTTP(S)固定端口，并对每个新连接重新解析和固定已批准的IP。
func (p *publicProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Proxy-Authorization") != p.auth {
		w.Header().Set("Proxy-Authenticate", "Basic realm=collection")
		http.Error(w, "proxy authentication required", 407)
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		http.Error(w, "proxy closing", 503)
		return
	}
	p.wg.Add(1)
	p.mu.Unlock()
	defer p.wg.Done()
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	case <-r.Context().Done():
		return
	default:
		http.Error(w, "proxy capacity", 503)
		return
	}
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	if r.URL.Scheme != "http" || r.URL.User != nil || r.URL.Host == "" || r.ContentLength > 1<<20 {
		http.Error(w, "destination rejected", 403)
		return
	}
	if _, _, err := publicAddress(r.URL.Host, "80"); err != nil {
		http.Error(w, "destination rejected", 403)
		return
	}
	req := r.Clone(r.Context())
	req.RequestURI = ""
	req.Close = true
	stripProxyHeaders(req.Header)
	response, err := p.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "destination unavailable", 502)
		return
	}
	defer response.Body.Close()
	stripProxyHeaders(response.Header)
	for k, values := range response.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

// connect 建立有截止时间的HTTPS隧道，关闭时回收两个方向的所有连接。
func (p *publicProxy) connect(w http.ResponseWriter, r *http.Request) {
	host, port, err := publicAddress(r.Host, "443")
	if err != nil || port != "443" {
		http.Error(w, "destination rejected", 403)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	upstream, err := publicDial(ctx, "tcp", net.JoinHostPort(host, port))
	cancel()
	if err != nil {
		http.Error(w, "destination unavailable", 502)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "tunnel unsupported", 500)
		return
	}
	client, buffer, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		client.Close()
		upstream.Close()
		return
	}
	p.connections[client] = true
	p.connections[upstream] = true
	p.mu.Unlock()
	defer func() {
		client.Close()
		upstream.Close()
		p.mu.Lock()
		delete(p.connections, client)
		delete(p.connections, upstream)
		p.mu.Unlock()
	}()
	_ = client.SetDeadline(time.Now().Add(45 * time.Minute))
	_ = upstream.SetDeadline(time.Now().Add(45 * time.Minute))
	if _, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if buffer.Flush() != nil {
		return
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(upstream, buffer); _ = upstream.Close(); close(done) }()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
	_ = upstream.Close()
	<-done
}

// Close 封闭代理准入，回收HTTP连接、隧道及等待中的处理器。
func (p *publicProxy) Close() {
	p.mu.Lock()
	p.closed = true
	for c := range p.connections {
		_ = c.Close()
	}
	p.mu.Unlock()
	_ = p.server.Close()
	p.transport.CloseIdleConnections()
	p.wg.Wait()
}

// publicAddress 只接受无用户信息、无控制字符的HTTP固定端口目标。
func publicAddress(address string, defaultPort string) (string, string, error) {
	if strings.ContainsAny(address, "@/\\\x00\r\n\t ") {
		return "", "", errors.New("destination rejected")
	}
	u, err := url.Parse("http://" + address)
	if err != nil || u.Hostname() == "" || strings.Contains(u.Hostname(), "%") {
		return "", "", errors.New("destination rejected")
	}
	port := u.Port()
	if port == "" {
		port = defaultPort
	}
	if port != "80" && port != "443" {
		return "", "", errors.New("destination rejected")
	}
	return u.Hostname(), port, nil
}

// publicDial 校验全部DNS答案并连接固定IP，防止解析重绑定和IPv4映射地址绕过。
func publicDial(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || (port != "80" && port != "443") {
		return nil, errors.New("destination rejected")
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("destination unavailable")
	}
	for _, ip := range addresses {
		if !publicIP(ip) {
			return nil, errors.New("destination rejected")
		}
	}
	var lastErr error
	for _, ip := range addresses {
		c, e := (&net.Dialer{Timeout: 12 * time.Second, KeepAlive: 20 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(ip.Unmap().String(), port))
		if e == nil {
			return c, nil
		}
		lastErr = e
	}
	return nil, lastErr
}

// publicIP 排除环回、私网、链路本地、文档、保留和云元数据网络。
func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, text := range []string{"0.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "::/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16", "fc00::/7", "fe80::/10"} {
		if netip.MustParsePrefix(text).Contains(ip) {
			return false
		}
	}
	return !ip.Is6() || netip.MustParsePrefix("2000::/3").Contains(ip)
}

// stripProxyHeaders 删除逐跳头和代理凭据，避免泄露到真实来源。
func stripProxyHeaders(h http.Header) {
	for _, key := range strings.Split(h.Get("Connection"), ",") {
		h.Del(strings.TrimSpace(key))
	}
	for _, key := range []string{"Connection", "Proxy-Connection", "Proxy-Authenticate", "Proxy-Authorization", "Keep-Alive", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		h.Del(key)
	}
}
