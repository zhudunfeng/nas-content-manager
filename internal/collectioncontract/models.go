// Package collectioncontract 定义资源获取服务的固定版本协议，不允许任意命令执行。
package collectioncontract

// Version 是必须显式匹配的获取协议版本。
const Version = 1

// Capability 描述执行端实际可用的固定能力。
type Capability struct {
	Kind      string `json:"kind"`      // 固定类型：web、remote、local。
	Available bool   `json:"available"` // 工具和执行条件是否满足。
	Reason    string `json:"reason"`    // 不可用的中文原因。
	Version   string `json:"version"`   // 已探测工具版本，不包含私密参数。
}

// Source 是执行端配置的授权输入目录或远程存储。
type Source struct {
	ID   string `json:"id"`   // 固定来源标识。
	Kind string `json:"kind"` // local 或 remote。
	Name string `json:"name"` // 中文显示名称。
}

// Capabilities 返回协议和安全执行能力快照。
type Capabilities struct {
	Version      int          `json:"version"`      // 获取协议版本。
	InstanceID   string       `json:"instanceId"`   // 持久执行端身份。
	Capabilities []Capability `json:"capabilities"` // 有界能力清单。
	Sources      []Source     `json:"sources"`      // 不含真实路径和凭据的来源清单。
	MaxBytes     int64        `json:"maxBytes"`     // 单任务字节上限。
	MaxFiles     int          `json:"maxFiles"`     // 单任务文件上限。
}

// File 描述相对操作目录的普通文件及观测版本。
type File struct {
	Path    string `json:"path"`             // 相对操作目录的安全路径，不能是绝对路径。
	Size    int64  `json:"size"`             // 文件字节数。
	ModTime int64  `json:"modTime"`          // 纳秒修改时间，用于发布后版本检查。
	SHA256  string `json:"sha256,omitempty"` // 复制时流式计算的内容摘要。
}

// Format 描述允许的网页输出配置。
type Format struct {
	ID        string `json:"id"`        // 固定选择值 video 或 audio。
	Label     string `json:"label"`     // 中文输出说明。
	Extension string `json:"extension"` // 已知扩展名；不确定时为空。
}

// PreviewRequest 是无任意命令参数的预览输入。
type PreviewRequest struct {
	Kind       string   `json:"kind"`                 // web、remote、local 或固定漫画插件 jmcomic。
	URL        string   `json:"url,omitempty"`        // 仅网页类型的公共 HTTP(S) 链接。
	Profile    string   `json:"profile,omitempty"`    // 网页输出 video/audio，漫画插件固定 comic。
	SourceID   string   `json:"sourceId,omitempty"`   // 执行端预配置的来源标识。
	Path       string   `json:"path,omitempty"`       // 来源下允许的相对路径。
	MaxBytes   int64    `json:"maxBytes,omitempty"`   // 请求字节预算，不能超过执行端上限。
	MaxFiles   int      `json:"maxFiles,omitempty"`   // 请求文件预算，不能超过执行端上限。
	AlbumID    string   `json:"albumId,omitempty"`    // JMComic 专用漫画正整数编号。
	UAVSite    string   `json:"uavSite,omitempty"`    // UAV 专用固定站点标识。
	VideoID    string   `json:"videoId,omitempty"`    // UAV 专用站点内视频编号。
	ChapterIDs []string `json:"chapterIds,omitempty"` // JMComic 明确选中的章节编号，空表示全本。
}

// Preview 是有时效且绑定输入版本的预览。
type Preview struct {
	ID         string           `json:"id"`                // 随机预览标识。
	Title      string           `json:"title"`             // 安全展示标题。
	Identity   string           `json:"identity"`          // 不含私密链接的来源身份摘要。
	Files      []File           `json:"files"`             // 已验证文件清单，网页或漫画下载前可为空。
	Formats    []Format         `json:"formats"`           // 固定输出配置。
	TotalBytes int64            `json:"totalBytes"`        // 已知总字节数；未知时为零。
	ExpiresAt  string           `json:"expiresAt"`         // UTC 预览失效时间。
	JMComic    *JMComicMetadata `json:"jmcomic,omitempty"` // 漫画章节目录和冻结的选择。
}

// OperationRequest 锁定一次不可变获取意图。
type OperationRequest struct {
	ID         string   `json:"id"`                   // 调用方持久保存的 UUID 幂等标识。
	Kind       string   `json:"kind"`                 // 固定获取类型。
	URL        string   `json:"url,omitempty"`        // 仅后台私密请求中的链接，不在响应回显。
	Profile    string   `json:"profile,omitempty"`    // 固定视频、音频或漫画输出配置。
	SourceID   string   `json:"sourceId,omitempty"`   // 已授权来源标识。
	Path       string   `json:"path,omitempty"`       // 来源相对路径。
	MaxBytes   int64    `json:"maxBytes,omitempty"`   // 本次字节预算。
	MaxFiles   int      `json:"maxFiles,omitempty"`   // 本次文件预算。
	PreviewID  string   `json:"previewId"`            // 绑定的有效预览。
	AlbumID    string   `json:"albumId,omitempty"`    // JMComic 专用漫画正整数编号。
	UAVSite    string   `json:"uavSite,omitempty"`    // UAV 专用固定站点标识。
	VideoID    string   `json:"videoId,omitempty"`    // UAV 专用站点内视频编号。
	ChapterIDs []string `json:"chapterIds,omitempty"` // JMComic 明确选章，空表示全本。
}

// JMComicSearchRequest 只允许有界关键词与页码，不接受站点或命令参数。
type JMComicSearchRequest struct {
	Query   string `json:"query"`             // 1–120 个字符的关键词或漫画编号。
	Page    int    `json:"page"`              // 从 1 开始的结果页，最大 1000。
	OrderBy string `json:"orderBy,omitempty"` // 排序白名单 mr/mv/mp/tf，省略沿用上游最新排序。
	Time    string `json:"time,omitempty"`    // 时间白名单 a/t/w/m，省略表示全部时间。
}

// JMComicChapter 是按上游顺序排列且不含来源地址的章节目录。
type JMComicChapter struct {
	ID        string `json:"id"`        // 章节正整数编号。
	Title     string `json:"title"`     // 有界纯文本章节名称。
	PageCount int    `json:"pageCount"` // 该章节声明的完整页数。
}

// JMComicMetadata 绑定可展示目录、实际选择和当前预算状态。
type JMComicMetadata struct {
	AlbumID            string           `json:"albumId"`              // 漫画编号。
	PageCount          int              `json:"pageCount"`            // 本次选择的总页数，全本时为全本页数。
	Chapters           []JMComicChapter `json:"chapters"`             // 全部章节目录，保留上游顺序。
	SelectedChapterIDs []string         `json:"selectedChapterIds"`   // 本次实际章节，按上游顺序规范化。
	OverBudget         bool             `json:"overBudget,omitempty"` // 超过本次页数预算，只能展示目录，不能创建获取操作。
}

// JMComicProgress 是独立于最终产物字节数的已核验页面进度。
type JMComicProgress struct {
	CompletedPages int   `json:"completedPages"` // 本次已完成真实校验的页面数。
	TotalPages     int   `json:"totalPages"`     // 冻结选择要求的完整页面总数。
	Bytes          int64 `json:"bytes"`          // 已验证页面字节数，包含复用，不表示网络流量。
	ReusedPages    int   `json:"reusedPages"`    // 本次从安全检查点复用并重新校验的页面数。
}

// JMComicSearchItem 是不含远程链接和凭据的漫画搜索结果。
type JMComicSearchItem struct {
	AlbumID string   `json:"albumId"` // 上游漫画正整数编号。
	Title   string   `json:"title"`   // 作为文本展示的漫画标题。
	Tags    []string `json:"tags"`    // 有界标签，未知时为空数组。
}

// JMComicSearchResponse 保存一次有界的搜索结果页。
type JMComicSearchResponse struct {
	Query   string              `json:"query"`   // 已规范化的请求关键词。
	Page    int                 `json:"page"`    // 当前结果页。
	Total   int                 `json:"total"`   // 上游报告的有限结果数量。
	HasNext bool                `json:"hasNext"` // 是否存在下一页。
	Items   []JMComicSearchItem `json:"items"`   // 最多 100 条结果，空结果为数组。
}

// Operation 是不含秘密的持久操作事实。
type Operation struct {
	ID              string           `json:"id"`                        // 全局幂等操作标识。
	Kind            string           `json:"kind"`                      // web、remote 或 local。
	State           string           `json:"state"`                     // queued/running/acquired/publishing/published/failed/canceled/recovery_required。
	Message         string           `json:"message"`                   // 脱敏中文状态说明。
	CreatedAt       string           `json:"createdAt"`                 // UTC 创建时间。
	UpdatedAt       string           `json:"updatedAt"`                 // UTC 最后状态时间。
	RelativeDir     string           `json:"relativeDir"`               // 相对执行端数据根的 staging/complete 目录。
	Files           []File           `json:"files"`                     // 相对 RelativeDir 的产物清单。
	Bytes           int64            `json:"bytes"`                     // 已验证产物字节数。
	RequestHash     string           `json:"requestHash"`               // 不可变意图摘要，用于冲突识别。
	JMComicProgress *JMComicProgress `json:"jmcomicProgress,omitempty"` // 漫画获取中的有界页面进度。
}

// Probe 返回目录映射挑战，不接触用户媒体。
type Probe struct {
	RelativePath string `json:"relativePath"` // 专属探针的受限相对路径。
	Nonce        string `json:"nonce"`        // 调用方给出的随机挑战。
}

// Error 是固定的脱敏失败格式。
type Error struct {
	Error string `json:"error"` // 中文错误，不包含源链接或命令输出。
}
