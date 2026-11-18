package app

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const torznabNamespace = "http://torznab.com/schemas/2015/feed" // Torznab 属性命名空间，前缀可由来源自由命名。

// torznabXMLAttribute 保留有限来源属性及其真实命名空间。
type torznabXMLAttribute struct {
	XMLName xml.Name // 属性元素的命名空间和本地名。
	Name    string   `xml:"name,attr"`  // size、seeders、peers、magneturl 或 infohash 等属性名。
	Value   string   `xml:"value,attr"` // 有界原始属性值，仅服务端解析。
}

// torznabXMLItem 是一个来源条目，不会直接序列化给客户端。
type torznabXMLItem struct {
	Title      string                `xml:"title"`   // 原始纯文本标题，输出前清理。
	GUID       string                `xml:"guid"`    // 只用于有限摘要去重的私密来源编号。
	Link       string                `xml:"link"`    // 候选磁力或代理链接，不进入 DTO。
	PubDate    string                `xml:"pubDate"` // RSS 发布时间。
	Size       string                `xml:"size"`    // 可选资源体积字节。
	Attributes []torznabXMLAttribute `xml:"attr"`    // 具有正确命名空间的扩展属性。
	Enclosure  struct {
		URL    string `xml:"url,attr"`    // 种子代理或磁力地址。
		Length string `xml:"length,attr"` // 资源体积字节，未知时为空。
	} `xml:"enclosure"` // 标准 RSS 种子附件。
}

// querySource 对固定来源执行一次有截止时间的 Torznab 通用搜索。
func (m *torrentSearchManager) querySource(ctx context.Context, source TorrentSearchSource, query string, category string) ([]torrentSearchPrivateResult, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	u, err := url.Parse(source.URL)
	if err != nil {
		return nil, false, fmt.Errorf("来源端点配置无效")
	}
	parameters := url.Values{"t": {"search"}, "q": {query}, "apikey": {source.APIKey}, "limit": {"100"}, "offset": {"0"}}
	if category != "" {
		parameters.Set("cat", category)
	}
	u.RawQuery = parameters.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, false, fmt.Errorf("来源请求配置无效")
	}
	request.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")
	response, err := m.http.Do(request)
	if err != nil {
		return nil, false, fmt.Errorf("来源连接失败或搜索超时，请检查索引服务")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, false, fmt.Errorf("来源拒绝认证，请检查服务端 API 密钥")
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("来源拒绝搜索，请检查索引器配置与可用性")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, false, fmt.Errorf("来源响应读取失败或超过 2 MiB 限额")
	}
	return parseTorznabResults(data, source)
}

// validateTorznabXML 先约束文档形状、深度与令牌数，拒绝 DTD、外部实体和非 RSS 文档。
func validateTorznabXML(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	tokens := 0
	roots := 0
	channel := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("来源 XML 格式无效")
		}
		tokens++
		if tokens > 80000 {
			return fmt.Errorf("来源 XML 结构超过解析预算")
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			if depth > 32 || len(value.Attr) > 32 {
				return fmt.Errorf("来源 XML 嵌套或属性超过解析预算")
			}
			if depth == 1 {
				roots++
				if roots != 1 || value.Name.Local != "rss" {
					return fmt.Errorf("来源未返回有效 Torznab RSS，请检查索引器与密钥")
				}
			}
			if depth == 2 && value.Name.Local == "channel" {
				channel = true
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			return fmt.Errorf("来源 XML 不允许 DTD 或指令")
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(value)) != "" {
				return fmt.Errorf("来源 XML 包含额外正文")
			}
		}
	}
	if roots != 1 || depth != 0 || !channel {
		return fmt.Errorf("来源 XML 缺少有效 RSS channel")
	}
	return nil
}

// parseTorznabResults 流式保留最多一百条，完整验证有限 XML 后再生成安全模型。
func parseTorznabResults(data []byte, source TorrentSearchSource) ([]torrentSearchPrivateResult, bool, error) {
	if len(data) > 2<<20 {
		return nil, false, fmt.Errorf("来源响应超过 2 MiB 限额")
	}
	if err := validateTorznabXML(data); err != nil {
		return nil, false, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	results := []torrentSearchPrivateResult{}
	path := []string{}
	items := 0
	truncated := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, fmt.Errorf("来源 XML 解析中断")
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "item" && len(path) == 2 && path[0] == "rss" && path[1] == "channel" {
				items++
				if items > 100 {
					truncated = true
					if decoder.Skip() != nil {
						return nil, false, fmt.Errorf("来源 XML 条目读取失败")
					}
					continue
				}
				var item torznabXMLItem
				if decoder.DecodeElement(&item, &value) != nil {
					return nil, false, fmt.Errorf("来源 XML 条目无效")
				}
				result, valid := convertTorznabItem(item, source)
				if valid {
					results = append(results, result)
				}
				continue
			}
			path = append(path, value.Name.Local)
		case xml.EndElement:
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		}
	}
	// 等于服务端 limit 时也保守提示可能还有结果，不宣称已遍历站点全部内容。
	return results, truncated || items >= 100, nil
}

// convertTorznabItem 提取允许的字段，优先磁力，不把来源下载链接透传到客户端。
func convertTorznabItem(item torznabXMLItem, source TorrentSearchSource) (torrentSearchPrivateResult, bool) {
	result := torrentSearchPrivateResult{Source: source}
	if len(item.Title) > 8192 || len(item.GUID) > 16384 || len(item.Link) > 16384 || len(item.Enclosure.URL) > 16384 || len(item.Attributes) > 64 {
		return result, false
	}
	attributes := make(map[string]string)
	for _, attribute := range item.Attributes {
		if (attribute.XMLName.Space == torznabNamespace || attribute.XMLName.Space == "") && len(attribute.Name) <= 80 && len(attribute.Value) <= 16384 {
			attributes[strings.ToLower(attribute.Name)] = attribute.Value
		}
	}
	// 先在完整原始标题中隐藏已知密钥，避免截断或控制字符清理后留下凭据片段。
	rawTitle := item.Title
	if source.APIKey != "" {
		rawTitle = strings.ReplaceAll(rawTitle, source.APIKey, "[已隐藏]")
	}
	title := strings.TrimSpace(strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return ' '
		}
		return char
	}, rawTitle))
	if title == "" {
		return result, false
	}
	if chars := []rune(title); len(chars) > 512 {
		title = string(chars[:512]) + "…"
	}
	result.View = TorrentSearchResult{Title: title, SourceID: source.ID, SourceName: source.Name, SizeBytes: firstSearchNumber(item.Size, attributes["size"], item.Enclosure.Length), Seeders: firstSearchNumber(attributes["seeders"]), Leechers: firstSearchNumber(attributes["leechers"])}
	if result.View.Leechers == nil {
		if peers := firstSearchNumber(attributes["peers"]); peers != nil && result.View.Seeders != nil && *peers >= *result.View.Seeders {
			value := *peers - *result.View.Seeders
			result.View.Leechers = &value
		}
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339} {
		if published, err := time.Parse(layout, strings.TrimSpace(item.PubDate)); err == nil {
			result.View.PublishedAt = published.UTC().Format(time.RFC3339)
			break
		}
	}
	for _, location := range []string{attributes["magneturl"], item.Enclosure.URL, item.Link} {
		if identity, err := parseMagnet(location); err == nil {
			result.Location = location
			result.V1, result.V2 = identity.V1, identity.V2
			break
		}
	}
	if result.Location == "" {
		for _, location := range []string{item.Enclosure.URL, item.Link} {
			if validTorznabProxyURL(source.URL, location) {
				result.Location = location
				break
			}
		}
	}
	for _, value := range []string{attributes["infohash"], attributes["infohashv1"], attributes["infohashv2"]} {
		value = strings.ToLower(value)
		if _, err := hex.DecodeString(value); err == nil {
			if len(value) == 40 && result.V1 == "" {
				result.V1 = value
			}
			if len(value) == 64 && result.V2 == "" {
				result.V2 = value
			}
		}
	}
	result.View.Downloadable = result.Location != ""
	// 没有稳定来源材料时保留每条结果，不能仅凭同名标题误合并不同资源。
	if strings.TrimSpace(item.GUID) != "" || strings.TrimSpace(item.Enclosure.URL) != "" || strings.TrimSpace(item.Link) != "" {
		result.Key = torrentSearchDigest(item.GUID + "\x00" + item.Enclosure.URL + "\x00" + item.Link)
	}
	return result, true
}

// firstSearchNumber 只接受可精确显示的非负十进制整数，缺失或异常值为未知。
func firstSearchNumber(values ...string) *int64 {
	for _, value := range values {
		number, err := strconv.ParseInt(value, 10, 64)
		if err == nil && number >= 0 && number <= maxDownloadInteger {
			return &number
		}
	}
	return nil
}

// searchOrigin 比较规范协议、主机和有效端口，避免凭据泄漏到同名主机的其它服务。
func searchOrigin(value *url.URL) string {
	port := value.Port()
	if port == "" {
		if value.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return value.Scheme + "://" + strings.ToLower(value.Hostname()) + ":" + port
}

// validTorznabProxyURL 仅授权已配置来源同源的 Jackett 或 Prowlarr 下载代理路径。
func validTorznabProxyURL(endpoint string, location string) bool {
	if location == "" || len(location) > 16384 {
		return false
	}
	base, baseErr := url.Parse(endpoint)
	target, targetErr := url.Parse(location)
	if baseErr != nil || targetErr != nil || !validSearchHTTPURL(base) || !validSearchHTTPURL(target) || searchOrigin(base) != searchOrigin(target) || target.RawPath != "" || strings.ContainsAny(target.Path, "\\\x00") {
		return false
	}
	marker := "/api/v2.0/indexers/"
	if index := strings.Index(base.Path, marker); index >= 0 {
		remainder := strings.TrimPrefix(base.Path[index+len(marker):], "/")
		parts := strings.Split(remainder, "/")
		if len(parts) == 4 && validTorrentSourceID(parts[0]) && parts[1] == "results" && parts[2] == "torznab" && parts[3] == "api" {
			prefix := base.Path[:index] + "/dl/" + parts[0] + "/"
			return target.Path == prefix
		}
	}
	if strings.HasSuffix(base.Path, "/api") {
		prefix := strings.TrimSuffix(base.Path, "/api")
		parts := strings.Split(prefix, "/")
		last := parts[len(parts)-1]
		id, err := strconv.ParseInt(last, 10, 64)
		if err == nil && id > 0 && strconv.FormatInt(id, 10) == last {
			return target.Path == prefix+"/download"
		}
	}
	return false
}
