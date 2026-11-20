package externalapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// servarrResource 只接收媒体身份及窄操作核验所需的字段。
type servarrResource struct {
	ID               int64  `json:"id"`               // 外部媒体编号。
	TVDBID           int64  `json:"tvdbId"`           // Sonarr 的 TVDB 身份。
	TMDBID           int64  `json:"tmdbId"`           // Radarr 的 TMDB 身份。
	Title            string `json:"title"`            // 媒体标题。
	Year             int    `json:"year"`             // 发行年份。
	Monitored        *bool  `json:"monitored"`        // 顶层监控状态。
	Status           string `json:"status"`           // 外部媒体状态。
	RootFolderPath   string `json:"rootFolderPath"`   // 仅在服务端核验的外部根目录。
	Path             string `json:"path"`             // 仅在服务端核验的媒体目录。
	QualityProfileID int64  `json:"qualityProfileId"` // 已选画质编号。
}

// servarrRoot 保存既有外部根目录，不形成 NAS 本地路径授权。
type servarrRoot struct {
	ID         int64  `json:"id"`         // 现有根编号。
	Path       string `json:"path"`       // 外部实例的绝对根路径。
	Accessible *bool  `json:"accessible"` // 可选可访问事实。
}

// servarrProfile 只接收已有画质选项。
type servarrProfile struct {
	ID   int64  `json:"id"`   // 画质编号。
	Name string `json:"name"` // 画质名称。
}

// servarrRecord 接收队列和历史展示所需的白名单字段。
type servarrRecord struct {
	ID          int64  `json:"id"`          // 记录编号。
	Title       string `json:"title"`       // 队列标题。
	SourceTitle string `json:"sourceTitle"` // 历史标题。
	Status      string `json:"status"`      // 队列状态。
	EventType   string `json:"eventType"`   // 历史事件。
	Date        string `json:"date"`        // 历史时间。
}

// adapterPreflightError 标记尚未发送任何业务写入的预检失败。
func adapterPreflightError(err error) error {
	var fault *Fault
	if errors.As(err, &fault) {
		return &Fault{Message: fault.Message, Rejected: true}
	}
	return &Fault{Message: "外部操作预检失败，尚未提交，请刷新后重试", Rejected: true}
}

// adapterText 在文本清理前移除当前连接秘密，防止恶意上游借标题回显凭据。
func (c *Client) adapterText(value string, limit int) string {
	for _, secret := range []string{c.config.APIKey, c.config.BaseURL} {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[已隐藏]")
		}
	}
	return CleanText(value, limit)
}

// adapterVersionMajor 解析产品主版本，不能用于推断 API 路径。
func adapterVersionMajor(version string) int {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	if len(parts) < 2 {
		return 0
	}
	major, _ := strconv.Atoi(parts[0])
	return major
}

// adapterPage 对已完整读取的有限数组做本地分页，保持实际总数。
func adapterPage(items []Item, page int, message string) Page {
	const size int = 50
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return Page{Items: append([]Item{}, items[start:end]...), Total: len(items), Page: page, PageSize: size, Message: message}
}

// adapterStatus 只显示已知业务状态，阻止原始错误或路径进入页面。
func adapterStatus(value string) string {
	labels := map[string]string{"continuing": "连载中", "ended": "已完结", "upcoming": "即将上线", "released": "已上映", "announced": "已公布", "inCinemas": "院线上映", "deleted": "外部已移除", "unknown": "状态未知", "queued": "排队中", "paused": "已暂停", "downloading": "下载中", "completed": "外部已完成", "failed": "外部失败", "warning": "需在外部检查", "delay": "延迟中", "downloadClientUnavailable": "下载器不可用", "fallback": "后备处理", "grabbed": "已提交下载", "downloadFolderImported": "外部已导入", "downloadFailed": "下载失败", "downloadImported": "外部已导入", "movieFileImported": "外部已导入", "seriesFolderImported": "外部已导入", "episodeFileImported": "外部已导入"}
	if label, ok := labels[value]; ok {
		return label
	}
	return "外部记录"
}

// adapterProgress 拒绝非法进度值，不把未知值伪装成零。
func adapterProgress(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 100 {
		return nil
	}
	return value
}

// probeServarr 通过产品身份和已核实主版本开放真实能力。
func (c *Client) probeServarr(ctx context.Context) (Info, error) {
	info := Info{Service: c.config.Service, Capabilities: []string{}}
	c.prefix = "/api/v3"
	var status struct {
		AppName string `json:"appName"` // 产品身份。
		Version string `json:"version"` // 实际运行版本。
	}
	if err := c.call(ctx, "GET", c.prefix+"/system/status", nil, nil, &status); err != nil {
		return info, err
	}
	if !strings.EqualFold(status.AppName, c.config.Service) || status.Version == "" || len(status.Version) > 80 {
		return info, &Fault{Message: "服务类型或版本响应不匹配，请检查实例地址", Rejected: true}
	}
	info.Version = c.adapterText(status.Version, 80)
	want := 4
	if c.config.Service == "radarr" {
		want = 7
	}
	if adapterVersionMajor(status.Version) != want {
		info.Message = "已连接，但此产品版本尚未核实，请使用外部应用入口"
		return info, nil
	}
	var health []struct {
		Type string `json:"type"` /* 健康项类型，原始消息不公开。 */
	}
	if err := c.call(ctx, "GET", c.prefix+"/health", nil, nil, &health); err != nil {
		return info, err
	}
	if health == nil || len(health) > 1000 {
		return info, &Fault{Message: "外部健康响应无效或超出限制", Rejected: true}
	}
	info.Supported = true
	info.Capabilities = []string{"resources", "queue", "history", "options", "lookup", "add", "monitor", "search"}
	info.Message = "连接和版本检测通过，外部服务继续负责下载与整理"
	if len(health) > 0 {
		info.Message = fmt.Sprintf("连接通过，外部服务报告 %d 项健康提示，请在外部应用检查", len(health))
	}
	return info, nil
}

// servarrRoute 返回服务端固定媒体路由和身份参数名。
func (c *Client) servarrRoute() (string, string) {
	if c.config.Service == "sonarr" {
		return "/series", "tvdbId"
	}
	return "/movie", "tmdbId"
}

// servarrMediaType 返回外部服务负责的固定媒体类别。
func (c *Client) servarrMediaType() string {
	if c.config.Service == "sonarr" {
		return "series"
	}
	return "movie"
}

// servarrIdentity 读取当前服务对应的元数据身份。
func (c *Client) servarrIdentity(resource servarrResource) int64 {
	if c.config.Service == "sonarr" {
		return resource.TVDBID
	}
	return resource.TMDBID
}

// servarrItem 只映射公开业务字段，不返回外部文件路径。
func (c *Client) servarrItem(resource servarrResource) Item {
	identity := c.servarrIdentity(resource)
	externalID := ""
	if identity > 0 {
		externalID = strconv.FormatInt(identity, 10)
	}
	id := ""
	if resource.ID > 0 {
		id = strconv.FormatInt(resource.ID, 10)
	}
	return Item{ID: id, ExternalID: externalID, Title: c.adapterText(resource.Title, 180), MediaType: c.servarrMediaType(), Year: resource.Year, Enabled: resource.Monitored, Status: adapterStatus(resource.Status)}
}

// servarrResources 完整读取有界资源数组，支持精确元数据身份过滤。
func (c *Client) servarrResources(ctx context.Context, externalID string) ([]servarrResource, error) {
	route, identity := c.servarrRoute()
	query := url.Values{}
	if externalID != "" {
		query.Set(identity, externalID)
	}
	var resources []servarrResource
	if err := c.call(ctx, "GET", c.prefix+route, query, nil, &resources); err != nil {
		return nil, err
	}
	if resources == nil || len(resources) > 1000 {
		return nil, &Fault{Message: "外部媒体列表无效或超过 1000 项，请在外部应用查看"}
	}
	for _, resource := range resources {
		if resource.ID <= 0 || c.servarrIdentity(resource) <= 0 || resource.Monitored == nil {
			return nil, &Fault{Message: "外部媒体字段不完整，请检查产品版本"}
		}
	}
	return resources, nil
}

// listServarr 区分数组媒体列表与真正分页的队列、历史。
func (c *Client) listServarr(ctx context.Context, section string, page int) (Page, error) {
	if section == "resources" {
		resources, err := c.servarrResources(ctx, "")
		if err != nil {
			return Page{}, err
		}
		items := make([]Item, 0, len(resources))
		for _, resource := range resources {
			items = append(items, c.servarrItem(resource))
		}
		return adapterPage(items, page, "外部监控列表；取消监控不会停止已开始的下载"), nil
	}
	var response struct {
		Page     int             `json:"page"`         // 上游页号。
		PageSize int             `json:"pageSize"`     // 上游页大小。
		Total    int             `json:"totalRecords"` // 上游总记录数。
		Records  []servarrRecord `json:"records"`      // 有限记录。
	}
	query := url.Values{"page": {strconv.Itoa(page)}, "pageSize": {"50"}}
	if section == "history" {
		query.Set("sortKey", "date")
		query.Set("sortDirection", "descending")
	}
	if err := c.call(ctx, "GET", c.prefix+"/"+section, query, nil, &response); err != nil {
		return Page{}, err
	}
	if response.Page != page || response.PageSize != 50 || response.Total < 0 || len(response.Records) > 50 || response.Total < len(response.Records) {
		return Page{}, &Fault{Message: "外部分页响应无效，请在外部应用查看"}
	}
	items := []Item{}
	for _, record := range response.Records {
		if record.ID <= 0 {
			return Page{}, &Fault{Message: "外部记录编号无效"}
		}
		title, status := record.Title, record.Status
		if section == "history" {
			title, status = record.SourceTitle, record.EventType
		}
		items = append(items, Item{ID: strconv.FormatInt(record.ID, 10), Title: c.adapterText(title, 180), MediaType: c.servarrMediaType(), Status: adapterStatus(status), Date: c.adapterText(record.Date, 40)})
	}
	return Page{Items: items, Page: page, PageSize: 50, Total: response.Total, Message: "外部状态不代表文件已在本系统扫描入库"}, nil
}

// servarrChoices 读取既有根目录和画质选项，后续写操作会重新核实。
func (c *Client) servarrChoices(ctx context.Context) ([]servarrRoot, []servarrProfile, error) {
	var roots []servarrRoot
	if err := c.call(ctx, "GET", c.prefix+"/rootfolder", nil, nil, &roots); err != nil {
		return nil, nil, err
	}
	var profiles []servarrProfile
	if err := c.call(ctx, "GET", c.prefix+"/qualityprofile", nil, nil, &profiles); err != nil {
		return nil, nil, err
	}
	if roots == nil || profiles == nil || len(roots) > 1000 || len(profiles) > 1000 {
		return nil, nil, &Fault{Message: "外部目录或画质选项无效或过多"}
	}
	return roots, profiles, nil
}

// optionsServarr 公开既有选项编号，外部路径仅在选择界面显示。
func (c *Client) optionsServarr(ctx context.Context) (Options, error) {
	roots, profiles, err := c.servarrChoices(ctx)
	if err != nil {
		return Options{}, err
	}
	options := Options{Roots: []Option{}, Profiles: []Option{}}
	for _, root := range roots {
		if root.ID > 0 && root.Path != "" && (root.Accessible == nil || *root.Accessible) {
			options.Roots = append(options.Roots, Option{ID: strconv.FormatInt(root.ID, 10), Name: fmt.Sprintf("目录 %d · %s", root.ID, c.adapterText(servarrRootLabel(root.Path), 80))})
		}
	}
	for _, profile := range profiles {
		if profile.ID > 0 {
			options.Profiles = append(options.Profiles, Option{ID: strconv.FormatInt(profile.ID, 10), Name: c.adapterText(profile.Name, 100)})
		}
	}
	return options, nil
}

// servarrRootLabel 仅显示外部目录最后一级，避免泄漏完整挂载路径。
func servarrRootLabel(value string) string {
	parts := strings.FieldsFunc(strings.TrimRight(value, "/\\"), func(character rune) bool { return character == '/' || character == '\\' })
	if len(parts) == 0 {
		return "外部根目录"
	}
	return CleanText(parts[len(parts)-1], 80)
}

// lookupServarr 查询元数据候选，不能执行 release 或下载搜索。
func (c *Client) lookupServarr(ctx context.Context, query string) ([]Item, error) {
	route, _ := c.servarrRoute()
	var resources []servarrResource
	if err := c.call(ctx, "GET", c.prefix+route+"/lookup", url.Values{"term": {query}}, nil, &resources); err != nil {
		return nil, err
	}
	if resources == nil || len(resources) > 1000 {
		return nil, &Fault{Message: "元数据候选无效或过多，请缩小标题范围"}
	}
	items := []Item{}
	for _, resource := range resources {
		if c.servarrIdentity(resource) > 0 && resource.Title != "" {
			item := c.servarrItem(resource)
			item.Enabled = nil
			item.Status = "元数据候选"
			items = append(items, item)
		}
	}
	return items, nil
}

// servarrDetail 读取单目标，防止列表标识指向不存在的外部资源。
func (c *Client) servarrDetail(ctx context.Context, id string) (servarrResource, error) {
	route, _ := c.servarrRoute()
	var resource servarrResource
	if !positiveID(id) {
		return resource, &Fault{Message: "外部目标编号无效", Rejected: true}
	}
	if err := c.call(ctx, "GET", c.prefix+route+"/"+id, nil, nil, &resource); err != nil {
		return resource, err
	}
	if strconv.FormatInt(resource.ID, 10) != id || resource.Monitored == nil {
		return resource, &Fault{Message: "外部目标不存在或状态不完整"}
	}
	return resource, nil
}

// servarrSelectedRoot 核实目标仍为当前实例已有且可访问的选项。
func (c *Client) servarrSelectedRoot(ctx context.Context, m Mutation) (string, error) {
	roots, profiles, err := c.servarrChoices(ctx)
	if err != nil {
		return "", err
	}
	rootPath := ""
	profileOK := false
	for _, root := range roots {
		if strconv.FormatInt(root.ID, 10) == m.RootID && (root.Accessible == nil || *root.Accessible) {
			rootPath = root.Path
		}
	}
	for _, profile := range profiles {
		if strconv.FormatInt(profile.ID, 10) == m.ProfileID {
			profileOK = true
		}
	}
	if rootPath == "" || !profileOK {
		return "", &Fault{Message: "所选外部目录或画质已变化，请重新选择", Rejected: true}
	}
	return rootPath, nil
}

// executeServarr 执行固定单媒体新增、窄监控更新或显式补搜。
func (c *Client) executeServarr(ctx context.Context, m Mutation) (Outcome, error) {
	route, identity := c.servarrRoute()
	if m.Kind == "add" {
		existing, err := c.servarrResources(ctx, m.ExternalID)
		if err != nil {
			return Outcome{}, adapterPreflightError(err)
		}
		for _, resource := range existing {
			if strconv.FormatInt(c.servarrIdentity(resource), 10) == m.ExternalID {
				return Outcome{}, &Fault{Message: "该媒体已在外部服务中，请从订阅列表管理现有条目", Rejected: true}
			}
		}
		root, err := c.servarrSelectedRoot(ctx, m)
		if err != nil {
			return Outcome{}, adapterPreflightError(err)
		}
		term := "tmdb:" + m.ExternalID
		if c.config.Service == "sonarr" {
			term = "tvdb:" + m.ExternalID
		}
		candidates, err := c.lookupServarr(ctx, term)
		if err != nil {
			return Outcome{}, adapterPreflightError(err)
		}
		title := ""
		for _, item := range candidates {
			if item.ExternalID == m.ExternalID {
				if title != "" {
					return Outcome{}, &Fault{Message: "元数据身份不唯一，请在外部应用核实", Rejected: true}
				}
				title = item.Title
			}
		}
		if title == "" {
			return Outcome{}, &Fault{Message: "未找到所选元数据身份，请重新查找", Rejected: true}
		}
		profileID, _ := strconv.ParseInt(m.ProfileID, 10, 64)
		externalID, _ := strconv.ParseInt(m.ExternalID, 10, 64)
		body := map[string]any{"title": title, identity: externalID, "qualityProfileId": profileID, "rootFolderPath": root, "monitored": true}
		if c.config.Service == "sonarr" {
			body["monitorNewItems"] = "all"
			body["seasonFolder"] = true
			body["seriesType"] = "standard"
			body["addOptions"] = map[string]any{"monitor": "all", "searchForMissingEpisodes": false, "searchForCutoffUnmetEpisodes": false}
		} else {
			body["minimumAvailability"] = "released"
			body["addOptions"] = map[string]any{"monitor": "movieOnly", "searchForMovie": false}
		}
		var resource servarrResource
		if err = c.call(ctx, "POST", c.prefix+route, nil, body, &resource); err != nil {
			return Outcome{}, err
		}
		if resource.ID <= 0 || strconv.FormatInt(c.servarrIdentity(resource), 10) != m.ExternalID {
			return Outcome{}, &Fault{Message: "新增响应缺少可核验的媒体身份，请核对操作结果"}
		}
		return Outcome{TargetID: strconv.FormatInt(resource.ID, 10), Message: "外部监控已创建；立即搜索未自动触发"}, nil
	}
	resource, err := c.servarrDetail(ctx, m.TargetID)
	if err != nil {
		return Outcome{}, adapterPreflightError(err)
	}
	if m.Kind == "monitor" {
		if *resource.Monitored == m.Enabled {
			return Outcome{TargetID: m.TargetID, Message: "已确认外部监控处于所选状态"}, nil
		}
		idsKey := "movieIds"
		if c.config.Service == "sonarr" {
			idsKey = "seriesIds"
		}
		if err = c.call(ctx, "PUT", c.prefix+route+"/editor", nil, map[string]any{idsKey: []int64{resource.ID}, "monitored": m.Enabled}, nil); err != nil {
			return Outcome{}, err
		}
		confirmed, err := c.servarrDetail(ctx, m.TargetID)
		if err != nil {
			return Outcome{}, err
		}
		if *confirmed.Monitored != m.Enabled {
			return Outcome{}, &Fault{Message: "外部监控尚未达到所选状态，请核对结果"}
		}
		return Outcome{TargetID: m.TargetID, Message: "外部监控状态已确认；现有下载和子项选择保留"}, nil
	}
	body := map[string]any{"name": "MoviesSearch", "movieIds": []int64{resource.ID}}
	if c.config.Service == "sonarr" {
		body = map[string]any{"name": "SeriesSearch", "seriesId": resource.ID}
	}
	var command struct {
		ID int64 `json:"id"` /* 外部命令回执编号。 */
	}
	if err = c.call(ctx, "POST", c.prefix+"/command", nil, body, &command); err != nil {
		return Outcome{}, err
	}
	if command.ID <= 0 {
		return Outcome{}, &Fault{Message: "搜索回执缺失，请核对结果，不能重复搜索"}
	}
	return Outcome{TargetID: m.TargetID, ReceiptID: strconv.FormatInt(command.ID, 10), Message: "已提交单个媒体的搜索，下载由外部服务编排"}, nil
}

// servarrWithinRoot 仅核实外部路径归属，不授予本地文件访问权。
func servarrWithinRoot(resource servarrResource, root string) bool {
	root = strings.TrimRight(strings.ReplaceAll(root, "\\", "/"), "/")
	if strings.TrimRight(strings.ReplaceAll(resource.RootFolderPath, "\\", "/"), "/") == root {
		return true
	}
	return strings.HasPrefix(strings.ReplaceAll(resource.Path, "\\", "/"), root+"/")
}

// reconcileServarr 只读精确身份和目标状态，不重复任何业务写入。
func (c *Client) reconcileServarr(ctx context.Context, m Mutation) (Outcome, bool, error) {
	if m.Kind == "search" {
		return Outcome{TargetID: m.TargetID, Message: "搜索缺少可关联回执，需在外部服务核实；本系统不会重发"}, false, nil
	}
	if m.Kind == "monitor" {
		resource, err := c.servarrDetail(ctx, m.TargetID)
		if err != nil {
			return Outcome{}, false, err
		}
		if *resource.Monitored == m.Enabled {
			return Outcome{TargetID: m.TargetID, Message: "已只读确认外部监控达到所选状态"}, true, nil
		}
		return Outcome{TargetID: m.TargetID, Message: "目标监控状态尚不一致，保留待核对"}, false, nil
	}
	resources, err := c.servarrResources(ctx, m.ExternalID)
	if err != nil {
		return Outcome{}, false, err
	}
	root, err := c.servarrSelectedRoot(ctx, m)
	if err != nil {
		return Outcome{}, false, err
	}
	var matched []servarrResource
	profile, _ := strconv.ParseInt(m.ProfileID, 10, 64)
	for _, resource := range resources {
		if strconv.FormatInt(c.servarrIdentity(resource), 10) == m.ExternalID {
			matched = append(matched, resource)
		}
	}
	if len(matched) == 1 && matched[0].Monitored != nil && *matched[0].Monitored == m.Enabled && matched[0].QualityProfileID == profile && servarrWithinRoot(matched[0], root) {
		return Outcome{TargetID: strconv.FormatInt(matched[0].ID, 10), Message: "已按元数据身份、目录、画质和监控状态确认媒体存在"}, true, nil
	}
	return Outcome{Message: "尚未唯一确认符合原始设置的媒体，请在外部服务核实"}, false, nil
}
