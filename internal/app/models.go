package app

// User 表示已认证管理员，不包含密码字段。
type User struct {
	ID       int64  `json:"id"`       // 用户唯一编号。
	Username string `json:"username"` // 管理员登录名。
}

// Library 表示一个只读媒体根目录的索引设置。
type Library struct {
	ID            int64  `json:"id"`            // 媒体库编号。
	Name          string `json:"name"`          // 用户设置的名称。
	Type          string `json:"type"`          // video/image/comic/novel 内容类型。
	Path          string `json:"path"`          // 已验证的规范化根目录。
	Status        string `json:"status"`        // 媒体库扫描状态。
	ItemCount     int64  `json:"itemCount"`     // 已登记项目数。
	LastScannedAt string `json:"lastScannedAt"` // 最近成功扫描时间。
}

// Progress 记录用户在不同阅读器中的位置。
type Progress struct {
	Position  float64 `json:"position"`  // 秒、页码或段落序号。
	Total     float64 `json:"total"`     // 总时长、页数或段落数。
	Locator   string  `json:"locator"`   // EPUB 定位及阅读器状态。
	UpdatedAt string  `json:"updatedAt"` // 最近保存时间。
}

// Item 是对前端输出的内容与文件元数据。
type Item struct {
	ID           int64     `json:"id"`           // 内容唯一编号。
	LibraryID    int64     `json:"libraryId"`    // 所属媒体库编号。
	Title        string    `json:"title"`        // 展示标题。
	Type         string    `json:"type"`         // 内容类型。
	RelativePath string    `json:"relativePath"` // 相对媒体库根目录路径。
	Size         int64     `json:"size"`         // 文件大小，使用 64 位避免超大视频溢出。
	Mime         string    `json:"mime"`         // 文件媒体类型。
	CoverURL     string    `json:"coverUrl"`     // 鉴权后的封面地址。
	AddedAt      string    `json:"addedAt"`      // 首次发现时间。
	UpdatedAt    string    `json:"updatedAt"`    // 文件最近更新时间。
	Missing      bool      `json:"missing"`      // 最近成功扫描是否发现文件缺失。
	Favorite     bool      `json:"favorite"`     // 当前用户收藏状态。
	Tags         []string  `json:"tags"`         // 当前用户设置的标签。
	Progress     *Progress `json:"progress"`     // 当前用户播放或阅读进度。
	Duration     float64   `json:"duration"`     // 视频秒数。
	Width        int       `json:"width"`        // 图像或视频宽度。
	Height       int       `json:"height"`       // 图像或视频高度。
	ModTime      int64     `json:"-"`            // 源文件纳秒修改时间，用于缓存失效。
	LibraryPath  string    `json:"-"`            // 服务器侧媒体库根目录。
	IsDir        bool      `json:"-"`            // 是否为漫画图片目录。
	EnrichState  string    `json:"-"`            // 衍生物状态，用于首次封面就绪后的缓存版本切换。
}

// Job 是可恢复、可取消的持久后台任务。
type Job struct {
	ID        int64  `json:"id"`        // 任务编号。
	LibraryID int64  `json:"libraryId"` // 所属媒体库编号。
	Kind      string `json:"kind"`      // scan 或 enrich。
	Status    string `json:"status"`    // queued/running/completed/failed/cancelled。
	Processed int64  `json:"processed"` // 本轮已处理内容数量。
	Message   string `json:"message"`   // 中文状态或错误说明。
	CreatedAt string `json:"createdAt"` // 任务创建时间。
	UpdatedAt string `json:"updatedAt"` // 最近更新时间。
	ItemID    int64  `json:"-"`         // 衍生任务关联内容编号。
}
