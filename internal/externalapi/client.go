package externalapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"
)

// ValidateConfig 校验固定服务、私密基址和已有令牌，不进行网络访问。
func ValidateConfig(config Config) error {
	switch config.Service {
	case "moviepilot", "autobangumi", "sonarr", "radarr":
	default:
		return &Fault{Message: "请选择已支持的外部服务", Rejected: true}
	}
	u, err := url.Parse(config.BaseURL)
	if err != nil || len(config.BaseURL) > 2048 || strings.ContainsAny(config.BaseURL, "\r\n\x00\\%") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && path.Clean(u.Path) != strings.TrimRight(u.Path, "/") && u.Path != "/") {
		return &Fault{Message: "服务地址必须是无账号、查询参数和片段的 HTTP(S) 基址，可保留反向代理前缀", Rejected: true}
	}
	if strings.TrimSpace(config.APIKey) == "" || len(config.APIKey) > 4096 {
		return &Fault{Message: "请填写服务中已生成的 API 凭据", Rejected: true}
	}
	for _, character := range config.APIKey {
		if character < 33 || character > 126 {
			return &Fault{Message: "API 凭据不能包含空白或控制字符", Rejected: true}
		}
	}
	return nil
}

// New 使用调用方已授权的网络客户端创建单次有界连接。
func New(config Config, httpClient *http.Client) (*Client, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, &Fault{Message: "外部连接缺少受控网络边界", Rejected: true}
	}
	return &Client{config: config, http: httpClient, info: Info{Service: config.Service, Capabilities: []string{}}}, nil
}

// call 只发送适配器选择的路径，限制响应并丢弃第三方错误原文。
func (c *Client) call(ctx context.Context, method string, route string, query url.Values, input any, output any) error {
	if !strings.HasPrefix(route, "/") || strings.ContainsAny(route, "?#\\\r\n") || strings.Contains(route, "..") {
		return &Fault{Message: "外部操作路径无效", Rejected: true}
	}
	u, _ := url.Parse(c.config.BaseURL + route)
	u.RawQuery = query.Encode()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil || len(data) > 64<<10 {
			return &Fault{Message: "外部操作参数无效或过大", Rejected: true}
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return &Fault{Message: "无法构造外部操作", Rejected: true}
	}
	req.Header.Set("Accept", "application/json")
	// AB4 无请求体的控制端点也要求 JSON Content-Type。
	req.Header.Set("Content-Type", "application/json")
	if c.config.Service == "autobangumi" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	} else {
		req.Header.Set("X-API-KEY", c.config.APIKey)
	}
	httpClient := c.http
	if c.config.Service == "moviepilot" && method == "GET" && route == "/api/v1/system/runscheduler" {
		// 此固定 GET 会启动任务，不能复用旧连接后由 Transport 自动重放。
		baseTransport := c.http.Transport
		if baseTransport == nil {
			baseTransport = http.DefaultTransport
		}
		if transport, ok := baseTransport.(*http.Transport); ok {
			isolated := transport.Clone()
			isolated.DisableKeepAlives = true
			defer isolated.CloseIdleConnections()
			clientCopy := *c.http
			clientCopy.Transport = isolated
			httpClient = &clientCopy
		}
		req.Close = true
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return &Fault{Message: "服务连接失败、超时或目标不符合网络策略，请检查地址和私网授权"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := "外部服务未接受请求，请在外部应用检查服务状态"
		if response.StatusCode == 401 || response.StatusCode == 403 {
			message = "API 凭据无效或权限不足，请在外部应用检查后重新填写"
		} else if response.StatusCode >= 300 && response.StatusCode < 400 {
			message = "服务地址发生重定向，请填写最终 API 实例地址"
		} else if response.StatusCode == 404 {
			message = "服务接口或目标不存在，请检查服务类型、版本与反向代理地址"
		}
		return &Fault{Message: message, Rejected: response.StatusCode < 500 && response.StatusCode != 408 && response.StatusCode != 429}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return &Fault{Message: "外部服务响应不完整或超过 4 MiB，请在外部应用查看"}
	}
	if len(data) == 0 && output == nil {
		return nil
	}
	if len(data) == 0 || !json.Valid(data) || (output != nil && json.Unmarshal(data, output) != nil) {
		return &Fault{Message: "外部服务响应格式不兼容，请检查实例版本"}
	}
	return nil
}

// Probe 检测真实版本和认证，不修改外部业务配置。
func (c *Client) Probe(ctx context.Context) (Info, error) {
	var info Info
	var err error
	switch c.config.Service {
	case "sonarr", "radarr":
		info, err = c.probeServarr(ctx)
	case "moviepilot":
		info, err = c.probeMoviePilot(ctx)
	case "autobangumi":
		info, err = c.probeAutoBangumi(ctx)
	}
	if info.Capabilities == nil {
		info.Capabilities = []string{}
	}
	if err == nil {
		// 六阶段契约仅核实到精确版本，不把同一主版本的其它实例推断为兼容。
		if c.config.Service == "moviepilot" && strings.TrimPrefix(info.Version, "v") == "3.2.8" && info.Supported {
			info.Capabilities = append(info.Capabilities, "integrated_search", "downloads", "organize", "scrape", "refresh_library", "download", "download_start", "download_stop")
		}
		c.info = info
	}
	return info, err
}

// requireCapability 禁止未检测或版本未知的实例获得写能力。
func (c *Client) requireCapability(name string) error {
	for _, capability := range c.info.Capabilities {
		if capability == name {
			return nil
		}
	}
	return &Fault{Message: "此服务版本尚未确认支持该操作，请使用外部应用入口", Rejected: true}
}

// List 按固定分区读取有限条目，不伪造离线空列表。
func (c *Client) List(ctx context.Context, section string, page int) (Page, error) {
	if section != "resources" && section != "queue" && section != "history" || page < 1 || page > 10000 {
		return Page{}, &Fault{Message: "列表分区或页码无效", Rejected: true}
	}
	if err := c.requireCapability(section); err != nil {
		return Page{}, err
	}
	var result Page
	var err error
	switch c.config.Service {
	case "sonarr", "radarr":
		result, err = c.listServarr(ctx, section, page)
	case "moviepilot":
		result, err = c.listMoviePilot(ctx, section, page)
	case "autobangumi":
		result, err = c.listAutoBangumi(ctx, section, page)
	}
	result.Info = c.info
	if result.Items == nil {
		result.Items = []Item{}
	}
	return result, err
}

// Options 返回外部实例中已有的目录与画质，不创建配置。
func (c *Client) Options(ctx context.Context) (Options, error) {
	if err := c.requireCapability("options"); err != nil {
		return Options{}, err
	}
	return c.optionsServarr(ctx)
}

// Lookup 查询元数据候选，不能触发下载搜索。
func (c *Client) Lookup(ctx context.Context, query string) ([]Item, error) {
	if err := c.requireCapability("lookup"); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if len(query) < 1 || len(query) > 160 || strings.ContainsAny(query, "\r\n\x00") {
		return nil, &Fault{Message: "请输入 1 至 160 字节的标题或元数据编号", Rejected: true}
	}
	return c.lookupServarr(ctx, query)
}

// ValidateMutation 限制操作种类和载荷，拒绝含混或无关参数。
func ValidateMutation(m Mutation, service string) error {
	if moviePilotWorkflowKind(m.Kind) {
		if service != "moviepilot" || m.SelectionID == "" || len(m.SelectionID) != 36 || m.TargetID != "" || m.ExternalID != "" || m.Title != "" || m.MediaType != "" || m.Season != 0 || m.RootID != "" || m.ProfileID != "" || m.Enabled || m.RSSURL != "" || (m.Kind == "scrape" && !m.ConfirmOverwrite) || (m.Kind != "scrape" && m.ConfirmOverwrite) {
			return &Fault{Message: "请从当前 MoviePilot 工作流选择目标并确认影响范围", Rejected: true}
		}
		return nil
	}
	if m.SelectionID != "" || m.ConfirmOverwrite {
		return &Fault{Message: "此操作不能附带其它工作流参数", Rejected: true}
	}
	bad := m.Kind != "add" && m.Kind != "monitor" && m.Kind != "search"
	bad = bad || len(m.Title) > 240 || strings.ContainsAny(m.Title, "\r\n\x00") || m.Season < 0 || m.Season > 999
	if m.Kind == "add" {
		bad = bad || m.TargetID != "" || !m.Enabled
		if service == "autobangumi" {
			u, err := url.Parse(m.RSSURL)
			bad = bad || err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || len(m.RSSURL) > 4096 || strings.ContainsAny(m.RSSURL, "\r\n\x00") || m.MediaType != "rss" || strings.TrimSpace(m.Title) == "" || m.ExternalID != "" || m.RootID != "" || m.ProfileID != "" || m.Season != 0
		} else {
			bad = bad || !positiveID(m.ExternalID) || m.RSSURL != ""
			if service == "moviepilot" {
				bad = bad || strings.TrimSpace(m.Title) == "" || m.RootID != "" || m.ProfileID != "" || (m.MediaType != "movie" && m.MediaType != "series") || (m.MediaType == "series" && m.Season < 1) || (m.MediaType == "movie" && m.Season != 0)
			} else {
				bad = bad || !positiveID(m.RootID) || !positiveID(m.ProfileID) || (service == "sonarr" && m.MediaType != "series") || (service == "radarr" && m.MediaType != "movie") || m.Season != 0
			}
		}
	} else {
		target := m.TargetID
		if service == "autobangumi" {
			parts := strings.Split(target, ":")
			bad = bad || len(parts) != 2
			if len(parts) == 2 {
				bad = bad || (parts[0] != "bangumi" && parts[0] != "rss")
				target = parts[1]
			}
		}
		bad = bad || !positiveID(target) || m.ExternalID != "" || m.RSSURL != "" || m.RootID != "" || m.ProfileID != "" || m.Title != "" || m.MediaType != "" || m.Season != 0 || (m.Kind == "search" && m.Enabled)
	}
	if bad {
		return &Fault{Message: "外部操作参数无效，请重新选择资源并确认", Rejected: true}
	}
	return nil
}

// Execute 仅分派经过能力和参数校验的固定单目标操作。
func (c *Client) Execute(ctx context.Context, m Mutation) (Outcome, error) {
	if err := ValidateMutation(m, c.config.Service); err != nil {
		return Outcome{}, err
	}
	if err := c.requireCapability(m.Kind); err != nil {
		return Outcome{}, err
	}
	if moviePilotWorkflowKind(m.Kind) {
		return c.executeMoviePilotWorkflow(ctx, m)
	}
	switch c.config.Service {
	case "sonarr", "radarr":
		return c.executeServarr(ctx, m)
	case "moviepilot":
		return c.executeMoviePilot(ctx, m)
	default:
		return c.executeAutoBangumi(ctx, m)
	}
}

// Reconcile 只读取既有目标确认结果，不能隐式重发写请求。
func (c *Client) Reconcile(ctx context.Context, m Mutation) (Outcome, bool, error) {
	if err := ValidateMutation(m, c.config.Service); err != nil {
		return Outcome{}, false, err
	}
	if err := c.requireCapability(m.Kind); err != nil {
		return Outcome{}, false, err
	}
	if moviePilotWorkflowKind(m.Kind) {
		return c.reconcileMoviePilotWorkflow(ctx, m)
	}
	switch c.config.Service {
	case "sonarr", "radarr":
		return c.reconcileServarr(ctx, m)
	case "moviepilot":
		return c.reconcileMoviePilot(ctx, m)
	default:
		return c.reconcileAutoBangumi(ctx, m)
	}
}

// positiveID 只接受规范化正整数编号，杜绝路径和查询透传。
func positiveID(value string) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == value
}

// CleanText 限制外部展示文本，移除控制字符和疑似地址。
func CleanText(value string, limit int) string {
	if strings.Contains(value, "://") || strings.ContainsAny(value, "\\/") {
		return "外部条目"
	}
	var result []rune
	for _, character := range value {
		if !unicode.IsControl(character) {
			result = append(result, character)
		}
		if len(result) >= limit {
			break
		}
	}
	return strings.TrimSpace(string(result))
}
