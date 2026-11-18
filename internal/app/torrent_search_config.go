package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TorrentSearchConfig 定义独立可选的服务端搜种配置，不进入设置响应或数据库。
type TorrentSearchConfig struct {
	Mode    string                // NAS_TORRENT_SEARCH_MODE，disabled/external/managed；未设置时兼容旧开关。
	Enabled bool                  // 启动时是否已有完整外接来源；旧开关只在未指定新模式时生效。
	Sources []TorrentSearchSource // NAS_TORRENT_SEARCH_SOURCES_FILE 中的来源，最多八个，只在后端使用。
}

// TorrentSearchSource 是只读 JSON 文件中的一个 Torznab 来源。
type TorrentSearchSource struct {
	ID     string `json:"id"`     // 稳定来源编号，1–40 个 ASCII 字母、数字、横线或下划线。
	Name   string `json:"name"`   // 界面显示名，1–80 个字符，不应包含账号或密钥。
	URL    string `json:"url"`    // 完整 HTTP(S) Torznab API 端点，不得包含查询参数或内嵌凭据。
	APIKey string `json:"apiKey"` // 索引器 API 密钥，保存在只读文件中，不返回浏览器或写入日志。
}

// loadTorrentSearchConfig 有界读取并严格验证来源文件；禁用时不读取文件或连接网络。
func loadTorrentSearchConfig() (TorrentSearchConfig, error) {
	return loadTorrentSearchConfigWithLookup(os.Getenv)
}

// loadTorrentSearchConfigWithLookup 以与主配置相同的来源读取模式和私密来源文件。
func loadTorrentSearchConfigWithLookup(lookup func(string) string) (TorrentSearchConfig, error) {
	source := newConfigSource(lookup)
	c := TorrentSearchConfig{Sources: []TorrentSearchSource{}}
	var err error
	c.Mode, err = loadEngineModeWithLookup("NAS_TORRENT_SEARCH_MODE", "NAS_TORRENT_SEARCH_ENABLED", source.lookup)
	c.Enabled = c.Mode == "external"
	if err != nil {
		return c, err
	}
	if !c.Enabled {
		return c, nil
	}
	file, err := os.Open(source.lookup("NAS_TORRENT_SEARCH_SOURCES_FILE"))
	if err != nil {
		return c, fmt.Errorf("搜种来源文件不可读取，请设置 NAS_TORRENT_SEARCH_SOURCES_FILE")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return c, fmt.Errorf("搜种来源文件读取失败或超过 64 KiB")
	}
	var document struct {
		Sources []TorrentSearchSource `json:"sources"` // 所有授权的索引服务端点。
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&document) != nil || decoder.Decode(&struct{}{}) != io.EOF || len(document.Sources) < 1 || len(document.Sources) > 8 {
		return c, fmt.Errorf("搜种来源文件必须为仅含 sources 数组的 JSON，包含 1–8 个来源")
	}
	seen := make(map[string]bool)
	for _, source := range document.Sources {
		u, parseErr := url.Parse(source.URL)
		if !validTorrentSourceID(source.ID) || seen[source.ID] || !validSearchText(source.Name, 1, 80) || !validSearchText(source.APIKey, 1, 4096) || parseErr != nil || !validSearchHTTPURL(u) || u.RawQuery != "" || u.ForceQuery || len(source.URL) > 4096 {
			return c, fmt.Errorf("搜种来源编号、名称、端点或密钥无效；端点不得包含凭据、查询或片段")
		}
		seen[source.ID] = true
		c.Sources = append(c.Sources, source)
	}
	return c, nil
}

// validTorrentSourceID 限定对外来源编号，避免把主机地址或私密参数作为编号传播。
func validTorrentSourceID(value string) bool {
	if len(value) < 1 || len(value) > 40 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

// validSearchText 按 Unicode 字符数验证有限文本，拒绝控制字符和无效 UTF-8。
func validSearchText(value string, minimum int, maximum int) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) < minimum || utf8.RuneCountInString(value) > maximum || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

// validSearchHTTPURL 拒绝非网络协议、内嵌凭据、片段和不规范端口，不将地址放入错误。
func validSearchHTTPURL(value *url.URL) bool {
	if value == nil || (value.Scheme != "http" && value.Scheme != "https") || value.Hostname() == "" || value.User != nil || value.Fragment != "" || value.Opaque != "" {
		return false
	}
	if port := value.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return false
		}
	}
	return true
}
