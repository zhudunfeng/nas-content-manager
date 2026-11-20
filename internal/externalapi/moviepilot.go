package externalapi

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// moviePilotSubscription 保存订阅身份与状态，禁止透传站点和目录配置。
type moviePilotSubscription struct {
	ID          int64  `json:"id"`           // 订阅编号。
	Name        string `json:"name"`         // 订阅名称。
	Type        string `json:"type"`         // 电影或电视剧。
	Year        string `json:"year"`         // 发行年份。
	Season      int    `json:"season"`       // 电视剧季号。
	State       string `json:"state"`        // R/P 运行、S 暂停。
	MediaSource string `json:"media_source"` // 元数据源枚举。
	MediaID     string `json:"media_id"`     // 元数据身份。
}

// moviePilotRecord 只接收队列和历史展示所需字段。
type moviePilotRecord struct {
	ID       json.RawMessage `json:"id"`       // 历史记录编号。
	Hash     string          `json:"hash"`     // 队列散列编号，不含地址。
	Name     string          `json:"name"`     // 传输名称。
	Title    string          `json:"title"`    // 媒体名称。
	State    string          `json:"state"`    // 下载器状态。
	Date     string          `json:"date"`     // 历史日期。
	Progress *float64        `json:"progress"` // 外部百分比进度。
}

// callMP 校验 v3 成功信封，不把 HTTP200 业务拒绝当作成功。
func (c *Client) callMP(ctx context.Context, method string, route string, query url.Values, input any, output any) error {
	var envelope struct {
		Success *bool           `json:"success"` // 业务成功标记。
		Data    json.RawMessage `json:"data"`    // 仅在服务端按窄模型解码的数据。
	}
	if err := c.call(ctx, method, route, query, input, &envelope); err != nil {
		return err
	}
	if envelope.Success == nil {
		return &Fault{Message: "MoviePilot 响应缺少 v3 业务信封，请核对版本"}
	}
	if !*envelope.Success {
		return &Fault{Message: "MoviePilot 未接受此业务请求，请在外部应用核实权限和目标", Rejected: true}
	}
	if output != nil && (len(envelope.Data) == 0 || string(envelope.Data) == "null" || json.Unmarshal(envelope.Data, output) != nil) {
		return &Fault{Message: "MoviePilot 响应数据不完整，请核对操作结果"}
	}
	return nil
}

// probeMoviePilot 读取公开版本后独立验证 API 身份，不加载环境秘密。
func (c *Client) probeMoviePilot(ctx context.Context) (Info, error) {
	info := Info{Service: "moviepilot", Capabilities: []string{}}
	c.prefix = "/api/v1"
	var global struct {
		Version string `json:"BACKEND_VERSION"` /* 当前后端版本。 */
	}
	if err := c.callMP(ctx, "GET", c.prefix+"/system/global", url.Values{"token": {"moviepilot"}}, nil, &global); err != nil {
		return info, err
	}
	if global.Version == "" || len(global.Version) > 80 {
		return info, &Fault{Message: "MoviePilot 未返回有效运行版本", Rejected: true}
	}
	info.Version = c.adapterText(global.Version, 80)
	if err := c.callMP(ctx, "GET", c.prefix+"/system/ping", nil, nil, nil); err != nil {
		return info, err
	}
	if adapterVersionMajor(global.Version) != 3 {
		info.Message = "连接通过，但此 MoviePilot 主版本尚未核实，请使用外部应用入口"
		return info, nil
	}
	info.Supported = true
	info.Capabilities = []string{"resources", "queue", "history", "add", "monitor", "search"}
	info.Message = "MoviePilot 连接与版本检测通过，下载、整理仍由该服务负责"
	return info, nil
}

// moviePilotType 将中文上游媒体类型映射为固定业务类别。
func moviePilotType(value string) string {
	if value == "电影" {
		return "movie"
	}
	if value == "电视剧" {
		return "series"
	}
	return ""
}

// moviePilotMediaName 返回上游精确媒体枚举。
func moviePilotMediaName(value string) string {
	if value == "movie" {
		return "电影"
	}
	return "电视剧"
}

// moviePilotSubscriptionItem 映射白名单订阅字段并保留 P 与 S 的差异。
func (c *Client) moviePilotSubscriptionItem(resource moviePilotSubscription) Item {
	enabled := resource.State == "R" || resource.State == "P"
	var active *bool
	status := "外部状态未知"
	if resource.State == "R" || resource.State == "P" || resource.State == "S" {
		active = &enabled
		status = "订阅运行中"
		if !enabled {
			status = "订阅已暂停"
		}
	}
	if resource.State == "P" {
		status = "订阅处理中"
	}
	year, _ := strconv.Atoi(resource.Year)
	externalID := ""
	if resource.MediaSource == "themoviedb" && positiveID(resource.MediaID) {
		externalID = resource.MediaID
	}
	return Item{ID: strconv.FormatInt(resource.ID, 10), ExternalID: externalID, Title: c.adapterText(resource.Name, 180), MediaType: moviePilotType(resource.Type), Season: resource.Season, Year: year, Enabled: active, Status: status}
}

// moviePilotSubscriptions 有界读取全部订阅，跨页重复视为不完整响应。
func (c *Client) moviePilotSubscriptions(ctx context.Context) ([]moviePilotSubscription, error) {
	result := []moviePilotSubscription{}
	seen := map[int64]bool{}
	for page := 1; page <= 11; page++ {
		var batch []moviePilotSubscription
		if err := c.callMP(ctx, "GET", c.prefix+"/subscribe/", url.Values{"page": {strconv.Itoa(page)}, "count": {"100"}}, nil, &batch); err != nil {
			return nil, err
		}
		if batch == nil || len(batch) > 100 || len(result)+len(batch) > 1000 {
			return nil, &Fault{Message: "MoviePilot 订阅列表无效或超过 1000 项，请在外部应用查看"}
		}
		for _, resource := range batch {
			if resource.ID <= 0 || seen[resource.ID] {
				return nil, &Fault{Message: "MoviePilot 订阅分页不完整或编号重复，请在外部应用查看"}
			}
			seen[resource.ID] = true
			result = append(result, resource)
		}
		if len(batch) < 100 {
			return result, nil
		}
	}
	return nil, &Fault{Message: "MoviePilot 订阅超出本次读取预算"}
}

// moviePilotRecordID 只接受有限数值或下载散列作为记录编号。
func moviePilotRecordID(record moviePilotRecord) string {
	if record.Hash != "" && len(record.Hash) <= 80 {
		for _, character := range record.Hash {
			if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f' || character >= 'A' && character <= 'F') {
				return ""
			}
		}
		return strings.ToLower(record.Hash)
	}
	var number int64
	if json.Unmarshal(record.ID, &number) == nil && number > 0 {
		return strconv.FormatInt(number, 10)
	}
	var value string
	if json.Unmarshal(record.ID, &value) == nil && positiveID(value) {
		return value
	}
	return ""
}

// listMoviePilot 返回订阅、当前下载或下载历史，不把外部完成解释为本地入库。
func (c *Client) listMoviePilot(ctx context.Context, section string, page int) (Page, error) {
	if section == "resources" {
		resources, err := c.moviePilotSubscriptions(ctx)
		if err != nil {
			return Page{}, err
		}
		items := make([]Item, 0, len(resources))
		for _, resource := range resources {
			items = append(items, c.moviePilotSubscriptionItem(resource))
		}
		return adapterPage(items, page, "MoviePilot 订阅独立运行；暂停订阅不会取消现有下载或人工搜索"), nil
	}
	route := c.prefix + "/download/"
	if section == "history" {
		route = c.prefix + "/history/download"
	}
	items := []Item{}
	seen := map[string]bool{}
	for upstreamPage := 1; upstreamPage <= 11; upstreamPage++ {
		var batch []moviePilotRecord
		if err := c.callMP(ctx, "GET", route, url.Values{"page": {strconv.Itoa(upstreamPage)}, "count": {"100"}}, nil, &batch); err != nil {
			return Page{}, err
		}
		if batch == nil || len(batch) > 100 || len(items)+len(batch) > 1000 {
			return Page{}, &Fault{Message: "MoviePilot 下载列表无效或超过 1000 项，请在外部应用查看"}
		}
		for _, record := range batch {
			id := moviePilotRecordID(record)
			if id == "" || seen[id] {
				return Page{}, &Fault{Message: "MoviePilot 下载记录分页不完整或身份重复"}
			}
			seen[id] = true
			title := record.Title
			if title == "" {
				title = record.Name
			}
			status := adapterStatus(record.State)
			if section == "history" {
				status = "外部下载历史"
			}
			items = append(items, Item{ID: id, Title: c.adapterText(title, 180), Status: status, Date: c.adapterText(record.Date, 40), Progress: adapterProgress(record.Progress)})
		}
		if len(batch) < 100 {
			return adapterPage(items, page, "外部下载和整理状态不代表本系统已扫描入库"), nil
		}
	}
	return Page{}, &Fault{Message: "MoviePilot 下载列表超出本次读取预算"}
}

// moviePilotDetail 严格核对单个订阅身份，空模型不是存在的目标。
func (c *Client) moviePilotDetail(ctx context.Context, id string) (moviePilotSubscription, error) {
	var resource moviePilotSubscription
	if !positiveID(id) {
		return resource, &Fault{Message: "订阅目标编号无效", Rejected: true}
	}
	if err := c.callMP(ctx, "GET", c.prefix+"/subscribe/"+id, nil, nil, &resource); err != nil {
		return resource, err
	}
	if strconv.FormatInt(resource.ID, 10) != id {
		return resource, &Fault{Message: "MoviePilot 订阅目标不存在"}
	}
	return resource, nil
}

// moviePilotIdentityMatch 使用完整媒体来源、身份、类型和季度核对订阅。
func moviePilotIdentityMatch(resource moviePilotSubscription, m Mutation) bool {
	season := resource.Season
	if m.MediaType == "movie" && season < 1 {
		season = 0
	}
	return resource.MediaSource == "themoviedb" && resource.MediaID == m.ExternalID && resource.Type == moviePilotMediaName(m.MediaType) && season == m.Season
}

// executeMoviePilot 新增精确身份订阅、固定 R/S 状态或单订阅显式搜索。
func (c *Client) executeMoviePilot(ctx context.Context, m Mutation) (Outcome, error) {
	if m.Kind == "add" {
		resources, err := c.moviePilotSubscriptions(ctx)
		if err != nil {
			return Outcome{}, adapterPreflightError(err)
		}
		for _, resource := range resources {
			if moviePilotIdentityMatch(resource, m) {
				return Outcome{}, &Fault{Message: "此媒体和季度已有订阅，请管理原有订阅", Rejected: true}
			}
		}
		body := map[string]any{"name": m.Title, "type": moviePilotMediaName(m.MediaType), "media_source": "themoviedb", "media_id": m.ExternalID}
		if m.MediaType == "series" {
			body["season"] = m.Season
		}
		var created struct {
			ID int64 `json:"id"` /* 新增订阅编号。 */
		}
		if err = c.callMP(ctx, "POST", c.prefix+"/subscribe/", nil, body, &created); err != nil {
			return Outcome{}, err
		}
		if created.ID <= 0 {
			return Outcome{}, &Fault{Message: "MoviePilot 新增回执缺少订阅编号，请核对结果"}
		}
		return Outcome{TargetID: strconv.FormatInt(created.ID, 10), Message: "MoviePilot 订阅已登记，后续获取由外部服务编排"}, nil
	}
	resource, err := c.moviePilotDetail(ctx, m.TargetID)
	if err != nil {
		return Outcome{}, adapterPreflightError(err)
	}
	if m.Kind == "monitor" {
		state := "S"
		if m.Enabled {
			state = "R"
		}
		if resource.State == state {
			return Outcome{TargetID: m.TargetID, Message: "已确认订阅处于所选运行状态"}, nil
		}
		if err = c.callMP(ctx, "PUT", c.prefix+"/subscribe/status/"+m.TargetID, url.Values{"state": {state}}, nil, nil); err != nil {
			return Outcome{}, err
		}
		updated, err := c.moviePilotDetail(ctx, m.TargetID)
		if err != nil {
			return Outcome{}, err
		}
		if updated.State != state {
			return Outcome{}, &Fault{Message: "MoviePilot 订阅状态尚未达到所选状态，请核对结果"}
		}
		return Outcome{TargetID: m.TargetID, Message: "订阅运行状态已确认；已下发下载和人工搜索保留"}, nil
	}
	var receipt struct {
		BatchID  string   `json:"batch_id"`  // 可关联的搜索批次编号。
		BatchIDs []string `json:"batch_ids"` // 上游返回的批次编号。
	}
	if err = c.callMP(ctx, "POST", c.prefix+"/subscribe/search/"+m.TargetID, nil, nil, &receipt); err != nil {
		return Outcome{}, err
	}
	if receipt.BatchID == "" && len(receipt.BatchIDs) == 1 {
		receipt.BatchID = receipt.BatchIDs[0]
	}
	if receipt.BatchID == "" || len(receipt.BatchID) > 100 || c.adapterText(receipt.BatchID, 100) != receipt.BatchID {
		return Outcome{}, &Fault{Message: "MoviePilot 搜索批次回执不完整，请核对结果，不能重复提交"}
	}
	return Outcome{TargetID: m.TargetID, ReceiptID: receipt.BatchID, Message: "已提交该订阅的显式搜索，可能由 MoviePilot 继续下载"}, nil
}

// reconcileMoviePilot 只读对账完整订阅身份或运行状态，搜索无回执时保持不确定。
func (c *Client) reconcileMoviePilot(ctx context.Context, m Mutation) (Outcome, bool, error) {
	if m.Kind == "search" {
		return Outcome{TargetID: m.TargetID, Message: "原搜索未取得可关联批次，请在 MoviePilot 核实；本系统不会重发"}, false, nil
	}
	if m.Kind == "monitor" {
		resource, err := c.moviePilotDetail(ctx, m.TargetID)
		if err != nil {
			return Outcome{}, false, err
		}
		state := "S"
		if m.Enabled {
			state = "R"
		}
		if resource.State == state {
			return Outcome{TargetID: m.TargetID, Message: "已只读确认订阅达到所选运行状态"}, true, nil
		}
		return Outcome{TargetID: m.TargetID, Message: "订阅运行状态不一致，保留待核对"}, false, nil
	}
	resources, err := c.moviePilotSubscriptions(ctx)
	if err != nil {
		return Outcome{}, false, err
	}
	matched := []moviePilotSubscription{}
	for _, resource := range resources {
		if moviePilotIdentityMatch(resource, m) {
			matched = append(matched, resource)
		}
	}
	if len(matched) == 1 {
		return Outcome{TargetID: strconv.FormatInt(matched[0].ID, 10), Message: "已按 TMDB 身份、媒体类型和季度唯一确认订阅存在"}, true, nil
	}
	return Outcome{Message: "尚未唯一确认原媒体身份的订阅，请在 MoviePilot 核实"}, false, nil
}
