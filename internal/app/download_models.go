package app

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

const (
	maxTorrentBytes              = 8 << 20                      // 种子本体最多 8 MiB，不接受媒体文件上传。
	maxDownloadUploadBytes       = maxTorrentBytes + (64 << 10) // multipart 总额包含最多 64 KiB 的表单开销。
	maxDownloadFiles             = 20000                        // 单个种子的文件清单条目上限。
	maxDownloadTasks             = 256                          // 单次协调与内存快照的受管未归档任务上限。
	maxEngineTasks               = 2048                         // 引擎完整身份对账的条目上限，超过后拒绝猜测所有权。
	maxDownloadInteger     int64 = 9007199254740991             // JSON 数值遵守前端安全整数范围。
	downloadActiveInterval       = 3 * time.Second              // 活跃下载统一同步间隔，浏览器不会额外连接引擎。
	downloadIdleInterval         = 15 * time.Second             // 空闲同步及指标持久化间隔。
	downloadErrorInterval        = 60 * time.Second             // 连接失败后的有界退避间隔。
)

// DownloadStatus 是不含引擎地址或凭据的连接诊断。
type DownloadStatus struct {
	Enabled        bool   `json:"enabled"`        // 是否配置启用下载。
	State          string `json:"state"`          // disabled/connecting/ready/offline/auth_failed/incompatible。
	Version        string `json:"version"`        // 实际 qB 应用版本。
	WebAPIVersion  string `json:"webApiVersion"`  // 实际 WebAPI 版本。
	LastSyncedAt   string `json:"lastSyncedAt"`   // 上次成功连接的 UTC 时间。
	Error          string `json:"error"`          // 已脱敏的中文诊断。
	CanSelectFiles bool   `json:"canSelectFiles"` // 当前版本是否满足文件选择契约。
}

// Download 是下载、做种与媒体入库的独立状态快照。
type Download struct {
	ID              string  `json:"id"`              // 本地随机任务编号。
	Name            string  `json:"name"`            // 已限制长度的展示名称。
	ContentType     string  `json:"contentType"`     // video/image/comic/novel/music。
	LibraryID       *int64  `json:"libraryId"`       // 已绑定的目标媒体库编号。
	State           string  `json:"state"`           // 下载状态。
	EngineState     string  `json:"engineState"`     // 引擎原始状态码。
	ImportState     string  `json:"importState"`     // 与传输分离的入库状态。
	SizeBytes       int64   `json:"sizeBytes"`       // 所选总字节，选择前为清单总字节。
	DownloadedBytes int64   `json:"downloadedBytes"` // 所选已完成字节。
	DownloadSpeed   int64   `json:"downloadSpeed"`   // 下载速度，单位 bytes/s。
	UploadSpeed     int64   `json:"uploadSpeed"`     // 上传速度，单位 bytes/s。
	UploadedBytes   int64   `json:"uploadedBytes"`   // 累计上传字节。
	Ratio           float64 `json:"ratio"`           // 无单位分享率。
	Peers           int64   `json:"peers"`           // 已连接 peer 数。
	ETASeconds      *int64  `json:"etaSeconds"`      // 未知时为 null 的预计秒数。
	ManifestVersion int     `json:"manifestVersion"` // 当前文件清单版本。
	SelectionLocked bool    `json:"selectionLocked"` // 确认正式下载后锁定选择。
	Revision        int64   `json:"revision"`        // 防止过时操作覆盖的新意图版本。
	Ready           bool    `json:"ready"`           // 通过本地文件核验的发布凭据。
	Archived        bool    `json:"archived"`        // 已从引擎移除但保留文件与清单。
	LastSyncedAt    string  `json:"lastSyncedAt"`    // 引擎快照时间。
	LastError       string  `json:"lastError"`       // 已脱敏的中文错误。
	CreatedAt       string  `json:"createdAt"`       // 创建 UTC 时间。
	UpdatedAt       string  `json:"updatedAt"`       // 最近持久状态更新时间。
}

// DownloadFile 是按引擎文件标识而非数组位置定位的有界清单条目。
type DownloadFile struct {
	Index     int     `json:"index"`     // 引擎返回的文件 ID。
	Path      string  `json:"path"`      // 相对当前任务保存目录的安全路径。
	SizeBytes int64   `json:"sizeBytes"` // 文件大小字节。
	Selected  bool    `json:"selected"`  // 用户已确认的选择。
	Progress  float64 `json:"progress"`  // 0–1 的完成比例。
	ModTime   int64   `json:"-"`         // 发布核验时的纳秒修改时间。
}

// downloadRecord 保存不对浏览器暴露的任务所有权与恢复字段。
type downloadRecord struct {
	Download             // 对外状态快照。
	RequestID     string // 首次添加的幂等键。
	IdentityV1    string // 原始 info 字节计算的 SHA-1 身份。
	IdentityV2    string // 原始 info 字节计算的 SHA-256 身份。
	EngineHash    string // 引擎不透明任务标识，不假定固定长度。
	LocalRoot     string // 创建时的应用下载根。
	StagingPath   string // 应用侧 staging 绝对目录。
	FinalPath     string // 应用侧 complete 类型及 UUID 目录。
	SourceFile    string // /data 下仅供恢复首次提交的私密源文件。
	SourceKind    string // torrent 或 magnet。
	CreateLibrary bool   // 是否明确授权首次创建受管媒体库。
	LibraryBound  bool   // 曾绑定目标库后禁止静默重新创建。
	ScanJobID     int64  // 本任务最近一次等待的媒体扫描任务。
	ScanAttempts  int    // 已安排扫描次数，避免无限失败重试。
}

// downloadOperation 是发出网络命令前持久化的幂等意图。
type downloadOperation struct {
	ID         int64  // 操作流水编号。
	RequestID  string // 客户端幂等键或内部固定标识。
	DownloadID string // 所属本地下载编号。
	Revision   int64  // 接受操作时的任务修订号。
	Kind       string // add/selection/start/pause/remove/move/limits/reconcile。
	Payload    string // 不含密码或磁力的有限命令参数。
	Status     string // queued/sent/uncertain/completed/failed/superseded。
}

// downloadManager 以独立协调器管理引擎，不持有现有扫描锁执行网络调用。
type downloadManager struct {
	lifecycleMu    sync.Mutex           // 保证托管连接最多激活一次并启动单个协调器。
	started        bool                 // 已发布固定适配器并启动工作循环后不再替换。
	app            *App                 // 应用数据库、日志及扫描接入点。
	engine         *qbClient            // 固定配置的 qB 适配器。
	mu             sync.RWMutex         // 仅保护短期内存快照。
	commandMu      sync.Mutex           // 串行持久化相互冲突的用户意图。
	status         DownloadStatus       // 最近连接快照。
	snapshots      map[string]Download  // 有界的最新传输指标。
	wake           chan struct{}        // 合并通知，避免每个请求创建后台线程。
	lastPersist    map[string]time.Time // 指标持久化节流时间。
	engineSnapshot []qbTorrent          // 最近完整引擎身份快照，仅用于添加时预先提示外部冲突。
}

// scanDownload 把数据库行转换为内部实体，空目标库输出 null。
func scanDownload(row rowScanner) (downloadRecord, error) {
	var d downloadRecord
	var library, eta sql.NullInt64
	err := row.Scan(&d.ID, &d.RequestID, &d.IdentityV1, &d.IdentityV2, &d.EngineHash, &d.Name, &d.ContentType, &library, &d.State, &d.EngineState, &d.ImportState, &d.SizeBytes, &d.DownloadedBytes, &d.DownloadSpeed, &d.UploadSpeed, &d.UploadedBytes, &d.Ratio, &d.Peers, &eta, &d.ManifestVersion, &d.SelectionLocked, &d.Revision, &d.Ready, &d.Archived, &d.LastSyncedAt, &d.LastError, &d.CreatedAt, &d.UpdatedAt, &d.LocalRoot, &d.StagingPath, &d.FinalPath, &d.SourceFile, &d.SourceKind, &d.CreateLibrary, &d.LibraryBound, &d.ScanJobID, &d.ScanAttempts)
	if library.Valid {
		d.LibraryID = &library.Int64
	}
	if eta.Valid {
		d.ETASeconds = &eta.Int64
	}
	return d, err
}

const downloadSelect = `SELECT id,request_id,identity_v1,identity_v2,engine_hash,name,content_type,target_library_id,state,engine_state,import_state,size_bytes,downloaded_bytes,download_speed,upload_speed,uploaded_bytes,ratio,peers,eta_seconds,manifest_version,selection_locked,revision,ready,archived,last_synced_at,last_error,created_at,updated_at,local_root,staging_path,final_path,source_file,source_kind,create_library,library_bound,scan_job_id,scan_attempts FROM downloads` // 实体查询使用稳定列顺序。

// downloadByID 仅读取本地任务，不把浏览器读取变成引擎请求。
func (a *App) downloadByID(ctx context.Context, id string) (downloadRecord, error) {
	return scanDownload(a.db.QueryRowContext(ctx, downloadSelect+" WHERE id=?", id))
}

// schemaV2 为现有数据库增量加入下载、清单与命令表，不重建媒体或用户状态。
const schemaV2 = `
CREATE TABLE downloads (
 id TEXT PRIMARY KEY, request_id TEXT NOT NULL UNIQUE,
 identity_v1 TEXT NOT NULL DEFAULT '', identity_v2 TEXT NOT NULL DEFAULT '', engine_hash TEXT NOT NULL DEFAULT '',
 name TEXT NOT NULL DEFAULT '正在获取元数据', content_type TEXT NOT NULL, target_library_id INTEGER,
 state TEXT NOT NULL DEFAULT 'submitting', engine_state TEXT NOT NULL DEFAULT '', import_state TEXT NOT NULL DEFAULT 'pending',
 size_bytes INTEGER NOT NULL DEFAULT 0, downloaded_bytes INTEGER NOT NULL DEFAULT 0, download_speed INTEGER NOT NULL DEFAULT 0,
 upload_speed INTEGER NOT NULL DEFAULT 0, uploaded_bytes INTEGER NOT NULL DEFAULT 0, ratio REAL NOT NULL DEFAULT 0, peers INTEGER NOT NULL DEFAULT 0, eta_seconds INTEGER,
 manifest_version INTEGER NOT NULL DEFAULT 0, selection_locked INTEGER NOT NULL DEFAULT 0, revision INTEGER NOT NULL DEFAULT 1,
 ready INTEGER NOT NULL DEFAULT 0, archived INTEGER NOT NULL DEFAULT 0, last_synced_at TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, local_root TEXT NOT NULL, staging_path TEXT NOT NULL, final_path TEXT NOT NULL,
 source_file TEXT NOT NULL DEFAULT '', source_kind TEXT NOT NULL DEFAULT '', create_library INTEGER NOT NULL DEFAULT 0,
 library_bound INTEGER NOT NULL DEFAULT 0, scan_job_id INTEGER NOT NULL DEFAULT 0, scan_attempts INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX idx_download_v1 ON downloads(identity_v1) WHERE identity_v1<>'';
CREATE UNIQUE INDEX idx_download_v2 ON downloads(identity_v2) WHERE identity_v2<>'';
CREATE INDEX idx_download_active ON downloads(archived,created_at DESC);
CREATE TABLE download_files (
 download_id TEXT NOT NULL REFERENCES downloads(id) ON DELETE CASCADE, version INTEGER NOT NULL,
 file_index INTEGER NOT NULL, relative_path TEXT NOT NULL, size_bytes INTEGER NOT NULL, selected INTEGER NOT NULL DEFAULT 0,
 progress REAL NOT NULL DEFAULT 0, mod_time INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(download_id,version,file_index), UNIQUE(download_id,version,relative_path)
);
CREATE TABLE download_operations (
 id INTEGER PRIMARY KEY AUTOINCREMENT, request_id TEXT NOT NULL UNIQUE,
 download_id TEXT NOT NULL REFERENCES downloads(id) ON DELETE CASCADE, revision INTEGER NOT NULL,
 kind TEXT NOT NULL, payload TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'queued',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE INDEX idx_download_operations_pending ON download_operations(status,id);
`
