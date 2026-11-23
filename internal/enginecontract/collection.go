package enginecontract

const (
	// CollectionTemplateVersion 固定四类资源组件的镜像、挂载和权限模板版本。
	CollectionTemplateVersion = "collection-2026-11-20.1"
	// CollectionNoticeVersion 固定组件访问受管下载和独立资源目录的授权版本。
	CollectionNoticeVersion = "collection-storage-v1"
	// CollectionNoticeText 是部署前必须主动确认的目录访问告知。
	CollectionNoticeText = "部署将创建固定组件容器及独立配置卷，并允许组件读写受管下载目录 /downloads 和独立资源目录 /data。请在组件中自行完成账号、站点、下载器及整理目录配置；平台不会自动接管原下载任务。跨目录整理请选择复制。"
	// MoviePilot 是影视自动化组件固定标识。
	MoviePilot = "moviepilot"
	// AutoBangumi 是追番组件固定标识。
	AutoBangumi = "autobangumi"
	// Sonarr 是剧集自动化组件固定标识。
	Sonarr = "sonarr"
	// Radarr 是电影自动化组件固定标识。
	Radarr = "radarr"
)

// CollectionComponents 返回顺序稳定且不共享可变数组的固定组件列表。
func CollectionComponents() []string {
	return []string{MoviePilot, AutoBangumi, Sonarr, Radarr}
}

// IsCollectionComponent 判断标识是否属于唯一受支持的资源组件集合。
func IsCollectionComponent(component string) bool {
	switch component {
	case MoviePilot, AutoBangumi, Sonarr, Radarr:
		return true
	default:
		return false
	}
}

// CollectionStatus 是四类组件可安全返回管理员浏览器的部署快照。
type CollectionStatus struct {
	Helper          string                `json:"helper"`          // 助手状态：ready/unavailable/unsupported/not_enabled。
	Message         string                `json:"message"`         // 脱敏中文环境说明。
	TemplateVersion string                `json:"templateVersion"` // 固定部署模板版本。
	NoticeVersion   string                `json:"noticeVersion"`   // 目录访问授权版本。
	NoticeText      string                `json:"noticeText"`      // 部署前主动确认的授权告知。
	ActiveTaskID    string                `json:"activeTaskId"`    // 仅当前资源组件任务的恢复标识。
	Components      []CollectionComponent `json:"components"`      // 四个固定组件的真实检测结果。
}

// CollectionComponent 区分容器运行与应用实际认证接入，不包含任何凭据。
type CollectionComponent struct {
	ID           string `json:"id"`           // 固定组件标识。
	Name         string `json:"name"`         // 产品显示名称。
	Version      string `json:"version"`      // 本平台固定部署版本。
	Presence     string `json:"presence"`     // unknown/missing/present/conflict。
	Runtime      string `json:"runtime"`      // unknown/stopped/starting/running。
	Readiness    string `json:"readiness"`    // not_configured/starting/configuration_required/error。
	Message      string `json:"message"`      // 不含原始上游响应的中文说明。
	Revision     int64  `json:"revision"`     // 组件身份与最新任务版本派生的乐观版本。
	CanDeploy    bool   `json:"canDeploy"`    // 当前是否允许提交明确授权的固定部署。
	CanStart     bool   `json:"canStart"`     // 当前是否允许仅启动原受管容器。
	SetupPort    int    `json:"setupPort"`    // 只绑定宿主127.0.0.1的首次配置端口。
	ServiceURL   string `json:"serviceUrl"`   // 匿名探针通过后的平台内部服务地址，不含密钥。
	LatestTaskID string `json:"latestTaskId"` // 当前组件最近部署或启动任务标识。
}

// CollectionDeployRequest 仅接受固定组件、当前模板和主动目录访问授权。
type CollectionDeployRequest struct {
	RequestID       string `json:"requestId"`       // 浏览器复用的持久幂等意图标识。
	Component       string `json:"component"`       // 单个固定组件标识。
	TemplateVersion string `json:"templateVersion"` // 页面读取的固定部署模板版本。
	NoticeVersion   string `json:"noticeVersion"`   // 页面显示的目录授权告知版本。
	AcceptNotice    bool   `json:"acceptNotice"`    // 用户主动勾选，不能默认接受。
}
