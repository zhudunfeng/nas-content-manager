// Package externalapi 提供固定外部自动化服务的有界、类型化适配器。
package externalapi

import (
	"encoding/json"
	"net/http"
)

// Config 保存仅在秘密文件持久化的连接参数。
type Config struct {
	Service string `json:"service"` // 固定服务标识。
	BaseURL string `json:"baseUrl"` // 私密 API 实例基址，保留反向代理前缀。
	APIKey  string `json:"apiKey"`  // 仅写入的 API Key 或 API token。
}

// Info 描述已检测的真实产品及能力。
type Info struct {
	Service      string   `json:"service"`      // 固定服务标识。
	Version      string   `json:"version"`      // 检测版本。
	Supported    bool     `json:"supported"`    // 是否为已核实可控制的版本。
	Capabilities []string `json:"capabilities"` // 固定能力列表。
	Message      string   `json:"message"`      // 已脱敏中文状态说明。
}

// Item 是不包含来源凭据和私密路径的统一展示条目。
type Item struct {
	ID         string   `json:"id"`         // 上游资源或事件编号。
	ExternalID string   `json:"externalId"` // TVDB/TMDB 元数据身份。
	Title      string   `json:"title"`      // 安全展示名称。
	MediaType  string   `json:"mediaType"`  // movie/series/rss/bangumi。
	Year       int      `json:"year"`       // 年份，零表示未知。
	Season     int      `json:"season"`     // 季号，零表示未指定。
	Enabled    *bool    `json:"enabled"`    // 监控或启用状态，未知为空。
	Status     string   `json:"status"`     // 脱敏状态说明。
	Progress   *float64 `json:"progress"`   // 百分比，未知为空。
	Date       string   `json:"date"`       // 历史时间。
}

// Page 保存单个来源分区的有限响应。
type Page struct {
	Info     Info   `json:"info"`     // 当前能力快照。
	Items    []Item `json:"items"`    // 条目，始终为数组。
	Total    int    `json:"total"`    // 已知总数。
	Page     int    `json:"page"`     // 当前页号。
	PageSize int    `json:"pageSize"` // 每页固定上限。
	Message  string `json:"message"`  // 部分视图限制说明。
}

// Option 是现有外部配置的只读选择。
type Option struct {
	ID   string `json:"id"`   // 服务端真实选项编号。
	Name string `json:"name"` // 选项名称。
}

// Options 只返回实例中已有的目录和画质选择。
type Options struct {
	Roots    []Option `json:"roots"`    // 现有外部完成目录选择，不构成本地授权。
	Profiles []Option `json:"profiles"` // 现有画质配置。
}

// Mutation 约束固定业务意图；含 RSS 地址时仅存入秘密文件。
type Mutation struct {
	Kind             string          `json:"kind"`             // add/monitor/search。
	TargetID         string          `json:"targetId"`         // 明确的单个外部资源编号。
	ExternalID       string          `json:"externalId"`       // 明确的 TVDB/TMDB 身份。
	Title            string          `json:"title"`            // 经用户确认的资源或 RSS 名称。
	MediaType        string          `json:"mediaType"`        // movie/series/rss。
	Season           int             `json:"season"`           // MP 电视剧季号。
	RootID           string          `json:"rootId"`           // 现有外部根目录编号。
	ProfileID        string          `json:"profileId"`        // 现有外部画质编号。
	Enabled          bool            `json:"enabled"`          // 明确的目标启用状态。
	RSSURL           string          `json:"rssUrl"`           // 仅写的 AB RSS 地址。
	SelectionID      string          `json:"selectionId"`      // 绑定来源修订的服务器不透明选择。
	ConfirmOverwrite bool            `json:"confirmOverwrite"` // 仅刮削需要明确确认覆盖元数据。
	Payload          json.RawMessage `json:"-"`                // 仅由服务器私密快照装配，禁止浏览器传入。
}

// Outcome 保存上游回执的有限、非敏感结果。
type Outcome struct {
	TargetID  string `json:"targetId"`  // 已确认的上游目标编号。
	ReceiptID string `json:"receiptId"` // 可选命令或批次编号。
	Message   string `json:"message"`   // 脱敏中文操作结果。
}

// Fault 区分明确拒绝和无法判断是否提交的失败。
type Fault struct {
	Message  string // 可公开的中文错误。
	Rejected bool   // 仅在可确定未接受写入时为真。
}

// Error 返回经过固定白名单生成的中文消息。
func (f *Fault) Error() string { return f.Message }

// Client 保留私密配置和调用方提供的安全网络边界。
type Client struct {
	config Config       // 私密连接配置。
	http   *http.Client // 已注入 DNS/IP 校验的专用客户端。
	info   Info         // 连接检测结果。
	prefix string       // 检测后的固定 API 路由前缀。
}
