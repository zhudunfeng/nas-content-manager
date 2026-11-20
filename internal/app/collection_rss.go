package app

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// collectionRSSItem 是有界解析后的内部条目，定位符只能存入私密文件。
type collectionRSSItem struct {
	Title     string // 已清理标题。
	Locator   string // 只能保存于私密文件的定位符。
	Identity  string // 来源条目的不可逆身份摘要。
	Published string // 来源声明的发布时间。
	Size      int64  // 已知字节数。
}

// collectionParseURL 拒绝内嵌凭据、控制字符和非 HTTP(S) 来源。
func collectionParseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || strings.ContainsAny(raw, "\r\n\x00") || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, collectionProblem(400, "来源必须为不含内嵌账号或片段的 HTTP(S) 地址")
	}
	return u, nil
}

// collectionNetworkIP 逐次拨号验证真实 DNS 地址，显式私网例外不包括元数据或链路本地网络。
func collectionNetworkIP(ip net.IP, allowPrivate bool) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip.IsLoopback() {
		return allowPrivate
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "::/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16"} {
		_, network, _ := net.ParseCIDR(block)
		if network.Contains(ip) {
			return false
		}
	}
	if ip.IsPrivate() || ip.IsLoopback() {
		return allowPrivate
	}
	if ip.To4() == nil {
		_, publicIPv6, _ := net.ParseCIDR("2000::/3")
		return ip.IsGlobalUnicast() && publicIPv6.Contains(ip)
	}
	return ip.IsGlobalUnicast()
}

// collectionRSSClient 固定来源域与实际目标地址，禁止代理和跨源重定向泄漏私密查询。
func collectionRSSClient(origin *url.URL, allowPrivate bool) *http.Client {
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 8 * time.Second, MaxResponseHeaderBytes: 32 << 10, DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("来源连接地址无效")
		}
		if !strings.EqualFold(host, origin.Hostname()) {
			return nil, fmt.Errorf("来源不允许跨主机连接")
		}
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(resolved) == 0 || len(resolved) > 16 {
			return nil, fmt.Errorf("来源域名无法安全解析")
		}
		for _, ip := range resolved {
			if !collectionNetworkIP(ip.IP, allowPrivate) {
				return nil, fmt.Errorf("来源目标地址不符合网络策略")
			}
		}
		dialer := net.Dialer{Timeout: 6 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(resolved[0].IP.String(), port))
	}}
	return &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) > 3 || request.URL.Scheme != origin.Scheme || !strings.EqualFold(request.URL.Host, origin.Host) || request.URL.User != nil {
			return fmt.Errorf("来源重定向越过授权边界")
		}
		return nil
	}}
}

// collectionFetchRSS 执行有条件有界拉取，测试模式不修改来源和候选。
func (a *App) collectionFetchRSS(ctx context.Context, s CollectionSource, test bool) ([]collectionRSSItem, collectionPrivate, error) {
	private, err := a.readCollectionPrivate(s.ID)
	if err != nil {
		return nil, private, collectionProblem(503, "RSS 私密地址不可用")
	}
	u, err := collectionParseURL(private.URL)
	if err != nil {
		return nil, private, err
	}
	client := collectionRSSClient(u, s.AllowPrivate)
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, private, err
	}
	request.Header.Set("Accept", "application/rss+xml,application/xml,text/xml")
	if !test {
		if private.ETag != "" {
			request.Header.Set("If-None-Match", private.ETag)
		}
		if private.LastModified != "" {
			request.Header.Set("If-Modified-Since", private.LastModified)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, private, collectionProblem(502, "RSS 连接失败、超时或目标不符合来源网络策略")
	}
	defer response.Body.Close()
	if response.StatusCode == 304 {
		return []collectionRSSItem{}, private, nil
	}
	if response.StatusCode != 200 {
		return nil, private, collectionProblem(502, "RSS 来源拒绝请求，请检查地址与凭据")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	if err != nil || len(data) > 2<<20 {
		return nil, private, collectionProblem(413, "RSS 响应读取失败或超过 2 MiB")
	}
	if err = validateTorznabXML(data); err != nil {
		return nil, private, collectionProblem(422, "RSS 文档结构无效或超过安全解析限制")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	items := []collectionRSSItem{}
	for {
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, private, collectionProblem(422, "RSS 文档解析失败")
		}
		element, ok := token.(xml.StartElement)
		if !ok || element.Name.Local != "item" {
			continue
		}
		var raw torznabXMLItem
		if decoder.DecodeElement(&raw, &element) != nil {
			return nil, private, collectionProblem(422, "RSS 条目解析失败")
		}
		if len(items) >= 100 {
			break
		}
		locator := raw.Enclosure.URL
		for _, attribute := range raw.Attributes {
			if attribute.Name == "magneturl" && attribute.XMLName.Space == torznabNamespace {
				locator = attribute.Value
			}
			if attribute.Name == "size" && raw.Size == "" {
				raw.Size = attribute.Value
			}
		}
		if locator == "" {
			locator = raw.Link
		}
		if len(locator) > 16384 {
			continue
		}
		if strings.HasPrefix(locator, "magnet:") {
			if _, e = parseMagnet(locator); e != nil {
				continue
			}
		} else {
			target, e := collectionParseURL(locator)
			if e != nil || target.Scheme != u.Scheme || !strings.EqualFold(target.Host, u.Host) {
				continue
			}
		}
		title := collectionSafeMessage(raw.Title)
		if title == "" {
			continue
		}
		size, _ := strconv.ParseInt(raw.Size, 10, 64)
		if size == 0 {
			size, _ = strconv.ParseInt(raw.Enclosure.Length, 10, 64)
		}
		if size < 0 || size > maxDownloadInteger {
			size = 0
		}
		identity := raw.GUID
		if identity == "" {
			identity = locator
		}
		published := ""
		if date, e := http.ParseTime(raw.PubDate); e == nil {
			published = date.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, collectionRSSItem{Title: title, Locator: locator, Identity: collectionDigest(identity), Published: published, Size: size})
	}
	if tag := response.Header.Get("ETag"); len(tag) <= 512 {
		private.ETag = tag
	}
	if modified := response.Header.Get("Last-Modified"); len(modified) <= 128 {
		private.LastModified = modified
	}
	return items, private, nil
}

// refreshCollectionRSS 串行收取来源，首次条目仅作为基线，显式回填才转待确认。
func (a *App) refreshCollectionRSS(ctx context.Context, s CollectionSource, backfill bool) (int, error) {
	if s.Kind != "rss" || !s.Enabled {
		return 0, collectionProblem(409, "仅已启用的 RSS 来源可以收取")
	}
	a.collections.refreshMu.Lock()
	defer a.collections.refreshMu.Unlock()
	current, err := a.collectionSourceByID(ctx, s.ID)
	if err != nil || current.Revision != s.Revision || !current.Enabled {
		return 0, collectionProblem(409, "来源配置已更新，请重新收取")
	}
	s = current
	// 回填需要完整响应，不能让 304 隐藏首次基线候选。
	items, private, fetchErr := a.collectionFetchRSS(ctx, s, backfill)
	a.collections.mu.Lock()
	defer a.collections.mu.Unlock()
	latest, err := a.collectionSourceByID(ctx, s.ID)
	if err != nil || latest.Revision != s.Revision {
		return 0, collectionProblem(409, "收取期间来源配置已更新，结果未应用")
	}
	if fetchErr != nil {
		latest.LastError = fetchErr.Error()
		_, _ = a.db.ExecContext(ctx, "UPDATE collection_sources SET body=? WHERE id=?", collectionJSON(latest), s.ID)
		return 0, fetchErr
	}
	added := 0
	for _, item := range items {
		var oldID string
		queryErr := a.db.QueryRowContext(ctx, "SELECT id FROM collection_candidates WHERE source_id=? AND identity=?", s.ID, item.Identity).Scan(&oldID)
		if queryErr == nil {
			old, e := a.collectionCandidateByID(ctx, oldID)
			if e != nil {
				return added, e
			}
			// 已确认任务共用原私密引用，必须保持其不可变；未确认条目才刷新授权快照。
			if old.TaskID == "" && (old.Decision == "baseline" || old.Decision == "review") {
				previous, privateErr := a.readCollectionPrivate(oldID)
				if privateErr != nil || previous.SourceRevision != s.Revision {
					if e = a.writeCollectionPrivate(oldID, collectionPrivate{URL: item.Locator, SourceRevision: s.Revision, Identity: item.Identity}); e != nil {
						return added, e
					}
					old.Title = item.Title
					old.SizeBytes = item.Size
					old.PublishedAt = item.Published
					if s.BaselineAt == "" && !backfill {
						old.Decision = "baseline"
						old.Reason = "来源更新后首次收取历史基线，不自动获取"
					}
					old.Revision++
					if _, e = a.db.ExecContext(ctx, "UPDATE collection_candidates SET body=? WHERE id=?", collectionJSON(old), old.ID); e != nil {
						return added, e
					}
				}
			}
			if backfill {
				if old.Decision == "baseline" {
					old.Decision = "review"
					old.Reason = "用户明确回填历史，等待确认"
					old.Revision++
					_, _ = a.db.ExecContext(ctx, "UPDATE collection_candidates SET body=? WHERE id=?", collectionJSON(old), old.ID)
				}
			}
			continue
		}
		id, e := newDownloadUUID()
		if e != nil {
			return added, e
		}
		decision, reason := "review", "新条目等待规则评估或用户确认"
		if s.BaselineAt == "" && !backfill {
			decision = "baseline"
			reason = "首次收取历史基线，不自动获取"
		}
		c := CollectionCandidate{ID: id, SourceID: s.ID, Title: item.Title, SizeBytes: item.Size, PublishedAt: item.Published, Decision: decision, Reason: reason, Revision: 1, CreatedAt: now()}
		if e = a.writeCollectionPrivate(id, collectionPrivate{URL: item.Locator, SourceRevision: s.Revision, Identity: item.Identity}); e != nil {
			return added, e
		}
		if _, e = a.db.ExecContext(ctx, "INSERT OR IGNORE INTO collection_candidates(id,source_id,identity,body) VALUES(?,?,?,?)", id, s.ID, item.Identity, collectionJSON(c)); e != nil {
			return added, e
		}
		added++
	}
	if latest.BaselineAt == "" {
		latest.BaselineAt = now()
	}
	latest.LastSuccessAt = now()
	latest.LastError = ""
	if err = a.writeCollectionPrivate(s.ID, private); err != nil {
		return added, err
	}
	_, err = a.db.ExecContext(ctx, "UPDATE collection_sources SET body=? WHERE id=?", collectionJSON(latest), s.ID)
	a.logger.Info("RSS 来源收取完成", "sourceId", s.ID, "added", added, "backfill", backfill)
	return added, err
}
