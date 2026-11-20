package externalapi

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// autoBangumiRule 是番剧规则的最小读取模型。
type autoBangumiRule struct {
	ID            int64  `json:"id"`             // 规则编号，与 RSS 编号独立。
	OfficialTitle string `json:"official_title"` // 规范标题。
	TitleRaw      string `json:"title_raw"`      // 原始标题，仅作清理后的展示。
	Season        int    `json:"season"`         // 规则季号。
	Deleted       *bool  `json:"deleted"`        // 软停用事实，反向映射启用状态。
	Archived      bool   `json:"archived"`       // 归档状态，启用操作不会改变此字段。
}

// autoBangumiRSS 保存只在服务端核验的 RSS 业务身份。
type autoBangumiRSS struct {
	ID        int64  `json:"id"`        // RSS 记录编号。
	Name      string `json:"name"`      // 用户定义名称。
	URL       string `json:"url"`       // 仅在服务端对账的私密地址。
	Enabled   *bool  `json:"enabled"`   // 来源启用状态。
	Aggregate *bool  `json:"aggregate"` // 是否聚合，首版新增固定为 false。
	Parser    string `json:"parser"`    // 首版新增固定 mikan。
}

// autoBangumiTorrent 保存下载器有限视图的公开字段。
type autoBangumiTorrent struct {
	Hash         string `json:"hash"`          // 下载散列编号。
	Name         string `json:"name"`          // 下载名称。
	State        string `json:"state"`         // 下载器状态。
	DownloaderID string `json:"downloader_id"` // 服务端下载器公开编号。
}

// probeAutoBangumi 独立核对健康和认证，后台未运行不等于鉴权失败。
func (c *Client) probeAutoBangumi(ctx context.Context) (Info, error) {
	info := Info{Service: "autobangumi", Capabilities: []string{}}
	c.prefix = "/api/v1"
	var health struct {
		Status  string `json:"status"`  // 健康端点状态。
		Version string `json:"version"` // 实际运行版本。
		DBOK    *bool  `json:"db_ok"`   // 数据库健康事实。
	}
	if err := c.call(ctx, "GET", "/health", nil, nil, &health); err != nil {
		return info, err
	}
	if health.Version == "" || len(health.Version) > 80 || health.DBOK == nil || health.Status != "ok" {
		return info, &Fault{Message: "AutoBangumi 健康响应不完整，请检查实例地址", Rejected: true}
	}
	info.Version = c.adapterText(health.Version, 80)
	var status struct {
		Running *bool  `json:"status"`  // 后台任务运行事实，不是 HTTP 成功标记。
		Version string `json:"version"` // 认证接口返回的运行版本。
	}
	if err := c.call(ctx, "GET", c.prefix+"/status", nil, nil, &status); err != nil {
		return info, err
	}
	if status.Running == nil || status.Version != health.Version {
		return info, &Fault{Message: "AutoBangumi 认证响应与健康版本不一致", Rejected: true}
	}
	if !*health.DBOK {
		return info, &Fault{Message: "AutoBangumi 数据库尚未就绪，请先在外部应用恢复", Rejected: true}
	}
	if adapterVersionMajor(health.Version) != 4 {
		info.Message = "已连接，但此 AutoBangumi 主版本尚未核实，请使用外部应用入口"
		return info, nil
	}
	info.Supported = true
	info.Capabilities = []string{"resources", "queue", "add", "monitor"}
	info.Message = "连接与版本检测通过；新增仅登记 RSS，下载视图可能不包含全部下载器"
	if !*status.Running {
		info.Message = "连接通过，AutoBangumi 后台当前未运行；请在外部应用启动，不会自动代启动"
	}
	return info, nil
}

// autoBangumiRules 有界读取番剧规则，保留停用与归档的真实差异。
func (c *Client) autoBangumiRules(ctx context.Context) ([]autoBangumiRule, error) {
	var rules []autoBangumiRule
	if err := c.call(ctx, "GET", c.prefix+"/bangumi/get/all", nil, nil, &rules); err != nil {
		return nil, err
	}
	if rules == nil || len(rules) > 1000 {
		return nil, &Fault{Message: "AutoBangumi 番剧规则无效或超过 1000 项"}
	}
	seen := map[int64]bool{}
	for _, rule := range rules {
		if rule.ID <= 0 || rule.Deleted == nil || seen[rule.ID] {
			return nil, &Fault{Message: "AutoBangumi 番剧规则身份或状态不完整"}
		}
		seen[rule.ID] = true
	}
	return rules, nil
}

// autoBangumiRSSList 只在服务端保留 RSS URL 以便精确对账。
func (c *Client) autoBangumiRSSList(ctx context.Context) ([]autoBangumiRSS, error) {
	var feeds []autoBangumiRSS
	if err := c.call(ctx, "GET", c.prefix+"/rss", nil, nil, &feeds); err != nil {
		return nil, err
	}
	if feeds == nil || len(feeds) > 1000 {
		return nil, &Fault{Message: "AutoBangumi RSS 列表无效或超过 1000 项"}
	}
	seen := map[int64]bool{}
	for _, feed := range feeds {
		if feed.ID <= 0 || feed.Enabled == nil || seen[feed.ID] {
			return nil, &Fault{Message: "AutoBangumi RSS 身份或状态不完整"}
		}
		seen[feed.ID] = true
	}
	return feeds, nil
}

// autoBangumiState 将有限下载器状态映射为中文，不回显上游错误原文。
func autoBangumiState(value string) string {
	labels := map[string]string{"downloading": "下载中", "stalledDL": "等待下载连接", "pausedDL": "下载已暂停", "stoppedDL": "下载已停止", "queuedDL": "下载排队中", "metaDL": "读取种子元数据", "checkingDL": "校验下载中", "uploading": "做种中", "stalledUP": "等待上传连接", "pausedUP": "做种已暂停", "stoppedUP": "做种已停止", "queuedUP": "做种排队中", "checkingUP": "做种校验中", "error": "下载器错误", "missingFiles": "外部文件缺失", "moving": "外部正在移动", "checkingResumeData": "正在恢复校验"}
	if label, ok := labels[value]; ok {
		return label
	}
	return adapterStatus(value)
}

// listAutoBangumi 合并规则和 RSS，但不混淆它们的实体编号。
func (c *Client) listAutoBangumi(ctx context.Context, section string, page int) (Page, error) {
	if section == "resources" {
		rules, err := c.autoBangumiRules(ctx)
		if err != nil {
			return Page{}, err
		}
		feeds, err := c.autoBangumiRSSList(ctx)
		if err != nil {
			return Page{}, err
		}
		if len(rules)+len(feeds) > 1000 {
			return Page{}, &Fault{Message: "AutoBangumi 合并订阅超过 1000 项，请在外部应用查看"}
		}
		items := []Item{}
		for _, rule := range rules {
			enabled := !*rule.Deleted
			title := rule.OfficialTitle
			if title == "" {
				title = rule.TitleRaw
			}
			status := "规则已启用"
			if !enabled {
				status = "规则已停用"
			}
			if rule.Archived {
				status = "规则已归档（启停不会取消归档）"
			}
			items = append(items, Item{ID: "bangumi:" + strconv.FormatInt(rule.ID, 10), Title: c.adapterText(title, 180), MediaType: "bangumi", Season: rule.Season, Enabled: &enabled, Status: status})
		}
		for _, feed := range feeds {
			status := "RSS 已启用"
			if !*feed.Enabled {
				status = "RSS 已停用"
			}
			items = append(items, Item{ID: "rss:" + strconv.FormatInt(feed.ID, 10), Title: c.adapterText(feed.Name, 180), MediaType: "rss", Enabled: feed.Enabled, Status: status})
		}
		return adapterPage(items, page, "RSS 来源登记与番剧规则分别管理；登记 RSS 不代表番剧订阅或下载已完成"), nil
	}
	var torrents []autoBangumiTorrent
	if err := c.call(ctx, "GET", c.prefix+"/downloader/torrents", nil, nil, &torrents); err != nil {
		return Page{}, err
	}
	if torrents == nil || len(torrents) > 1000 {
		return Page{}, &Fault{Message: "AutoBangumi 下载器视图无效或超过 1000 项"}
	}
	items := []Item{}
	for _, torrent := range torrents {
		record := moviePilotRecord{Hash: torrent.Hash}
		id := moviePilotRecordID(record)
		if id == "" {
			return Page{}, &Fault{Message: "AutoBangumi 下载器条目缺少可核验编号"}
		}
		items = append(items, Item{ID: c.adapterText(torrent.DownloaderID, 64) + ":" + id, Title: c.adapterText(torrent.Name, 180), Status: autoBangumiState(torrent.State)})
	}
	return adapterPage(items, page, "仅显示 AutoBangumi 分类中的下载；部分离线下载器可能被外部服务忽略，此视图不代表完整下载队列或本地入库"), nil
}

// autoBangumiTarget 解析严格分类型的单目标编号。
func autoBangumiTarget(target string) (string, string, error) {
	parts := strings.Split(target, ":")
	if len(parts) != 2 || (parts[0] != "bangumi" && parts[0] != "rss") || !positiveID(parts[1]) {
		return "", "", &Fault{Message: "请选择一个有效的番剧规则或 RSS 来源", Rejected: true}
	}
	return parts[0], parts[1], nil
}

// autoBangumiEnabled 只读取明确目标的停用状态，不修改任何配置。
func (c *Client) autoBangumiEnabled(ctx context.Context, target string) (bool, error) {
	kind, id, err := autoBangumiTarget(target)
	if err != nil {
		return false, err
	}
	if kind == "rss" {
		feeds, err := c.autoBangumiRSSList(ctx)
		if err != nil {
			return false, err
		}
		for _, feed := range feeds {
			if strconv.FormatInt(feed.ID, 10) == id {
				return *feed.Enabled, nil
			}
		}
		return false, &Fault{Message: "所选 AutoBangumi RSS 已不存在", Rejected: true}
	}
	var rule autoBangumiRule
	if err = c.call(ctx, "GET", c.prefix+"/bangumi/get/"+id, nil, nil, &rule); err != nil {
		return false, err
	}
	if strconv.FormatInt(rule.ID, 10) != id || rule.Deleted == nil {
		return false, &Fault{Message: "所选 AutoBangumi 规则不存在或状态无效", Rejected: true}
	}
	return !*rule.Deleted, nil
}

// autoBangumiRSSMatches 核对 RSS 的精确 URL 和不可变最小登记参数。
func autoBangumiRSSMatches(feed autoBangumiRSS, m Mutation) bool {
	return feed.URL == m.RSSURL && feed.Name == m.Title && feed.Aggregate != nil && !*feed.Aggregate && feed.Parser == "mikan" && feed.Enabled != nil && *feed.Enabled
}

// executeAutoBangumi 固定登记 RSS 或切换单规则状态，不移动删除文件。
func (c *Client) executeAutoBangumi(ctx context.Context, m Mutation) (Outcome, error) {
	if m.Kind == "add" {
		feeds, err := c.autoBangumiRSSList(ctx)
		if err != nil {
			return Outcome{}, adapterPreflightError(err)
		}
		for _, feed := range feeds {
			if feed.URL == m.RSSURL {
				return Outcome{}, &Fault{Message: "此 RSS 已登记，请从列表管理已有来源，不会重复添加或重新启用", Rejected: true}
			}
		}
		body := map[string]any{"name": m.Title, "url": m.RSSURL, "aggregate": false, "parser": "mikan"}
		if err = c.call(ctx, "POST", c.prefix+"/rss/add", nil, body, nil); err != nil {
			return Outcome{}, err
		}
		outcome, confirmed, err := c.reconcileAutoBangumi(ctx, m)
		if err != nil {
			return Outcome{}, err
		}
		if !confirmed {
			return Outcome{}, &Fault{Message: "RSS 登记请求已发送，但尚未唯一确认结果，请核对操作记录"}
		}
		return outcome, nil
	}
	kind, id, err := autoBangumiTarget(m.TargetID)
	if err != nil {
		return Outcome{}, err
	}
	enabled, err := c.autoBangumiEnabled(ctx, m.TargetID)
	if err != nil {
		return Outcome{}, adapterPreflightError(err)
	}
	if enabled == m.Enabled {
		return Outcome{TargetID: m.TargetID, Message: "已确认外部来源或规则处于所选状态"}, nil
	}
	if kind == "rss" {
		err = c.call(ctx, "PATCH", c.prefix+"/rss/update/"+id, nil, map[string]any{"enabled": m.Enabled}, nil)
	} else {
		route := c.prefix + "/bangumi/enable/" + id
		query := url.Values{}
		if !m.Enabled {
			route = c.prefix + "/bangumi/disable/" + id
			query.Set("file", "false")
		}
		err = c.call(ctx, "POST", route, query, nil, nil)
	}
	if err != nil {
		return Outcome{}, err
	}
	confirmed, err := c.autoBangumiEnabled(ctx, m.TargetID)
	if err != nil {
		return Outcome{}, err
	}
	if confirmed != m.Enabled {
		return Outcome{}, &Fault{Message: "AutoBangumi 状态尚未达到所选状态，请核对操作结果"}
	}
	return Outcome{TargetID: m.TargetID, Message: "外部启用状态已确认；已有下载、文件和归档状态保留"}, nil
}

// reconcileAutoBangumi 只读核实 RSS 精确身份或规则状态，不能重发写入。
func (c *Client) reconcileAutoBangumi(ctx context.Context, m Mutation) (Outcome, bool, error) {
	if m.Kind == "monitor" {
		enabled, err := c.autoBangumiEnabled(ctx, m.TargetID)
		if err != nil {
			return Outcome{}, false, err
		}
		if enabled == m.Enabled {
			return Outcome{TargetID: m.TargetID, Message: "已只读确认外部来源或规则达到所选状态"}, true, nil
		}
		return Outcome{TargetID: m.TargetID, Message: "外部状态与原始意图不一致，请在 AutoBangumi 核实"}, false, nil
	}
	if m.Kind != "add" {
		return Outcome{}, false, &Fault{Message: "此 AutoBangumi 版本没有已核实的通用搜索控制", Rejected: true}
	}
	feeds, err := c.autoBangumiRSSList(ctx)
	if err != nil {
		return Outcome{}, false, err
	}
	matches := []autoBangumiRSS{}
	for _, feed := range feeds {
		if feed.URL == m.RSSURL {
			matches = append(matches, feed)
		}
	}
	if len(matches) == 1 && autoBangumiRSSMatches(matches[0], m) {
		return Outcome{TargetID: "rss:" + strconv.FormatInt(matches[0].ID, 10), Message: "RSS 已登记并核实；尚未建立番剧规则或确认下载"}, true, nil
	}
	return Outcome{Message: "尚未唯一确认原始 RSS 地址和登记参数，请在 AutoBangumi 核实"}, false, nil
}
