// Package enginecontract 定义助手与应用间的受控协议；私有连接结构不得直接返回浏览器。
package enginecontract

const (
	// TemplateVersion 是固定部署模板版本，变更时必须重新验证镜像和配置。
	TemplateVersion = "2026-11-18.1"
	// NoticeVersion 是首次启动 qB 所需用户确认的告知版本。
	NoticeVersion = "qbittorrent-5.2.4-v1"
	// NoticeText 是初始化界面显示的上游首次使用告知。
	NoticeText = "qBittorrent 是文件共享程序。您分享的内容可能受版权保护；请确认您拥有下载与分享权限。阅读并接受 qBittorrent 首次使用告知后才能启动下载引擎。"
	// QBittorrent 是下载引擎固定标识。
	QBittorrent = "qbittorrent"
	// Jackett 是搜索引擎固定标识。
	Jackett = "jackett"
)

// Status 是可安全返回浏览器的助手检测快照。
type Status struct {
	Helper        string      `json:"helper"`        // 助手状态：ready/unavailable/unsupported/not_enabled。
	Message       string      `json:"message"`       // 已脱敏的中文说明。
	Components    []Component `json:"components"`    // 两个固定组件的检测结果。
	ActiveTaskID  string      `json:"activeTaskId"`  // 可恢复的当前执行任务。
	NoticeVersion string      `json:"noticeVersion"` // 上游告知版本。
	NoticeText    string      `json:"noticeText"`    // 上游告知文本。
	SetupPort     int         `json:"setupPort"`     // Jackett 仅宿主回环开放的配置端口。
}

// Component 是不包含地址或密钥的组件状态。
type Component struct {
	ID            string `json:"id"`            // 固定组件标识。
	Mode          string `json:"mode"`          // disabled/external/managed。
	Presence      string `json:"presence"`      // unknown/missing/present/conflict。
	Runtime       string `json:"runtime"`       // unknown/stopped/starting/running/offline。
	Readiness     string `json:"readiness"`     // 运行连接就绪状态。
	Message       string `json:"message"`       // 脱敏诊断说明。
	Revision      int64  `json:"revision"`      // 乐观操作版本。
	CanInitialize bool   `json:"canInitialize"` // 是否确认允许初始化。
	CanStart      bool   `json:"canStart"`      // 是否允许启动原受管容器。
}

// InitializationRequest 是唯一允许提交的固定组件安装参数。
type InitializationRequest struct {
	RequestID     string   `json:"requestId"`     // 浏览器持久复用的幂等请求键。
	Components    []string `json:"components"`    // 去重排序后的固定组件标识。
	NoticeVersion string   `json:"noticeVersion"` // 用户阅读的告知版本。
	AcceptNotice  bool     `json:"acceptNotice"`  // 用户主动勾选的确认。
}

// ActionRequest 是重试或启动原容器的乐观并发参数。
type ActionRequest struct {
	RequestID string `json:"requestId"` // 操作的幂等请求键。
	Revision  int64  `json:"revision"`  // 页面观察到的任务或组件版本。
}

// Layer 是 Docker 流实际报告的一个镜像层进度，不代表整体百分比。
type Layer struct {
	ID      string `json:"id"`      // 镜像层摘要的安全短标识。
	Status  string `json:"status"`  // 受控中文阶段。
	Current int64  `json:"current"` // 实际处理字节数。
	Total   int64  `json:"total"`   // 实际总字节数；未知为零。
}

// Step 是单个组件的持久执行状态。
type Step struct {
	Component string  `json:"component"` // 固定组件标识。
	Phase     string  `json:"phase"`     // 当前部署阶段。
	State     string  `json:"state"`     // queued/running/completed/failed。
	Attempt   int     `json:"attempt"`   // 重试次数。
	Message   string  `json:"message"`   // 脱敏阶段说明。
	Layers    []Layer `json:"layers"`    // 有界的最新真实分层进度。
}

// Task 是独立助手持久库内的初始化任务。
type Task struct {
	ID         string   `json:"id"`         // 服务端生成的任务标识。
	Action     string   `json:"action"`     // initialize/start，重启恢复时保留原操作授权。
	State      string   `json:"state"`      // queued/running/completed/failed/interrupted。
	Revision   int64    `json:"revision"`   // 乐观并发版本。
	Components []string `json:"components"` // 本任务固定目标。
	Steps      []Step   `json:"steps"`      // 每个组件独立结果。
	CreatedAt  string   `json:"createdAt"`  // RFC3339 创建时间。
	UpdatedAt  string   `json:"updatedAt"`  // RFC3339 最后进度时间。
	Message    string   `json:"message"`    // 脱敏总体结果。
}

// Event 是有界的中文安全事件，禁止写入原始引擎响应。
type Event struct {
	Sequence  int64  `json:"sequence"`  // 单任务递增游标。
	Component string `json:"component"` // 关联固定组件。
	Level     string `json:"level"`     // info/error。
	Message   string `json:"message"`   // 至多256字符的受控中文消息。
	CreatedAt string `json:"createdAt"` // RFC3339 记录时间。
}

// TaskResponse 支持增量轮询和事件缺口检测。
type TaskResponse struct {
	Task           Task    `json:"task"`           // 最新完整任务快照。
	Events         []Event `json:"events"`         // 最多100条新事件。
	NextSequence   int64   `json:"nextSequence"`   // 下次请求的 afterSequence。
	ReloadRequired bool    `json:"reloadRequired"` // 事件已过期时提示重载。
}

// Manifest 仅通过应用只读私密目录发布，不作为 API DTO。
type Manifest struct {
	Version           int    `json:"version"`           // 私有连接协议版本，当前为1。
	InstallationID    string `json:"installationId"`    // 防止旧安装被静默替换的身份。
	Generation        int64  `json:"generation"`        // 原子清单版本。
	QBGeneration      int64  `json:"qbGeneration"`      // 下载连接版本；零表示尚未发布。
	JackettGeneration int64  `json:"jackettGeneration"` // 搜索连接版本；零表示尚未发布。
}

// Connection 是服务端私有连接文件，严禁日志或浏览器序列化。
type Connection struct {
	Version        int    `json:"version"`        // 连接格式版本，当前为1。
	InstallationID string `json:"installationId"` // 所属安装身份。
	Component      string `json:"component"`      // 固定组件标识。
	Generation     int64  `json:"generation"`     // 对应清单版本。
	ContainerID    string `json:"containerId"`    // 下载实例持久身份。
	URL            string `json:"url"`            // 仅同机容器网络的固定上游地址。
	Username       string `json:"username"`       // 下载器账户，搜索引擎为空。
	Password       string `json:"password"`       // 私有下载器密码，搜索引擎为空。
	APIKey         string `json:"apiKey"`         // 私有搜索密钥，下载器为空。
}
