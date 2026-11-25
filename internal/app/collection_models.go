package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"nas-content-manager/internal/collectioncontract"
)

// CollectionSource 是来源的公开配置，私密地址只保存于独立秘密文件。
type CollectionSource struct {
	ID              string `json:"id"`              // 随机来源编号。
	Kind            string `json:"kind"`            // rss/web/local/remote/external/jmcomic/uav 固定来源类型。
	Name            string `json:"name"`            // 来源展示名称。
	Enabled         bool   `json:"enabled"`         // 是否允许收取和创建新任务。
	Revision        int64  `json:"revision"`        // 编辑乐观版本。
	HasSecret       bool   `json:"hasSecret"`       // 是否已配置私密地址。
	RootID          string `json:"rootId"`          // 已授权的本地或执行端来源根。
	Path            string `json:"path"`            // 根内相对目录。
	AllowPrivate    bool   `json:"allowPrivate"`    // 是否明确允许来源访问私网。
	RefreshMinutes  int    `json:"refreshMinutes"`  // RSS 收取间隔分钟数。
	OutputProfile   string `json:"outputProfile"`   // video/audio 输出配置。
	LastSuccessAt   string `json:"lastSuccessAt"`   // 最近成功时间。
	LastError       string `json:"lastError"`       // 脱敏中文错误。
	ExternalURL     string `json:"externalUrl"`     // 无查询参数的外部应用入口。
	ExternalService string `json:"externalService"` // 固定外部服务类型，空为仅目录入口。
	APIConfigured   bool   `json:"apiConfigured"`   // 是否存在独立私密 API 连接。
	BaselineAt      string `json:"baselineAt"`      // 已建立首次收取基线的时间。
	CreatedAt       string `json:"createdAt"`       // 创建时间。
	UpdatedAt       string `json:"updatedAt"`       // 最近编辑时间。
}

// collectionSourceInput 限定来源编辑的完整公开参数。
type collectionSourceInput struct {
	Kind            string                             `json:"kind"`            // 来源类型。
	Name            string                             `json:"name"`            // 来源名称。
	Enabled         bool                               `json:"enabled"`         // 是否启用。
	URL             string                             `json:"url"`             // 仅写的 RSS 私密地址。
	RootID          string                             `json:"rootId"`          // 授权根编号。
	Path            string                             `json:"path"`            // 根内相对路径。
	AllowPrivate    bool                               `json:"allowPrivate"`    // 明确允许内网服务。
	RefreshMinutes  int                                `json:"refreshMinutes"`  // 刷新分钟数。
	OutputProfile   string                             `json:"outputProfile"`   // 输出类型。
	ExternalURL     string                             `json:"externalUrl"`     // 公开外部应用入口。
	Revision        int64                              `json:"revision"`        // 编辑依据的版本。
	ExternalService string                             `json:"externalService"` // 固定外部服务类型。
	Connection      *collectionExternalConnectionInput `json:"connection"`      // 可选仅写连接，空字段保留原值。
	ClearConnection bool                               `json:"clearConnection"` // 明确移除 API 连接，保留历史。
}

// CollectionRule 保存有界文本和体积匹配条件。
type CollectionRule struct {
	Include        string `json:"include"`        // 必须包含的关键词，逗号分隔且全部匹配。
	Exclude        string `json:"exclude"`        // 任一命中即排除的关键词。
	Quality        string `json:"quality"`        // 必须包含的画质文本。
	MinBytes       int64  `json:"minBytes"`       // 最小字节数，零为不限。
	MaxBytes       int64  `json:"maxBytes"`       // 最大字节数，零为不限。
	EpisodePattern string `json:"episodePattern"` // 显式集数正则，空表示只做人工确认。
}

// CollectionSubscription 保存可审计订阅策略。
type CollectionSubscription struct {
	ID               string         `json:"id"`               // 订阅编号。
	SourceID         string         `json:"sourceId"`         // RSS 来源编号。
	Name             string         `json:"name"`             // 订阅名称。
	Enabled          bool           `json:"enabled"`          // 是否调度。
	Revision         int64          `json:"revision"`         // 策略版本。
	ContentType      string         `json:"contentType"`      // 目标媒体类别。
	Mode             string         `json:"mode"`             // review 或明确授权的 automatic。
	Rule             CollectionRule `json:"rule"`             // 条件。
	CreateLibrary    bool           `json:"createLibrary"`    // 授权首次创建目标库。
	LibraryID        int64          `json:"libraryId"`        // 已有受管库，零表示按规则创建。
	SelectionPattern string         `json:"selectionPattern"` // 自动选择唯一文件的正则。
	NextRunAt        string         `json:"nextRunAt"`        // 下次调度时间。
	CreatedAt        string         `json:"createdAt"`        // 创建时间。
	UpdatedAt        string         `json:"updatedAt"`        // 更新时间。
}

// CollectionCandidate 是持久化且无私密载荷的来源条目。
type CollectionCandidate struct {
	ID          string `json:"id"`          // 候选编号。
	SourceID    string `json:"sourceId"`    // 来源编号。
	Title       string `json:"title"`       // 安全标题。
	SizeBytes   int64  `json:"sizeBytes"`   // 已知资源字节数。
	PublishedAt string `json:"publishedAt"` // 来源发布时间。
	Decision    string `json:"decision"`    // baseline/review/ignored/approved。
	Reason      string `json:"reason"`      // 中文判定原因。
	Revision    int64  `json:"revision"`    // 动作版本。
	CreatedAt   string `json:"createdAt"`   // 首次发现时间。
	TaskID      string `json:"taskId"`      // 已接受的任务编号。
}

// CollectionFile 保存输出版本，不保留完整媒体内容。
type CollectionFile struct {
	Path    string `json:"path"`    // 相对任务目录路径。
	Size    int64  `json:"size"`    // 64 位长度。
	ModTime int64  `json:"modTime"` // 纳秒修改时间。
	SHA256  string `json:"sha256"`  // 执行端提供的可选摘要。
}

// CollectionFormat 是网页预览的安全输出格式。
type CollectionFormat struct {
	ID        string `json:"id"`        // 格式编号。
	Label     string `json:"label"`     // 格式描述。
	Extension string `json:"extension"` // 输出扩展名。
}

// CollectionPreview 是有时效并绑定来源修订的获取计划。
type CollectionPreview struct {
	ID         string                              `json:"id"`                // 预览编号。
	SourceID   string                              `json:"sourceId"`          // 来源编号。
	Title      string                              `json:"title"`             // 标题。
	Kind       string                              `json:"kind"`              // 执行类型或 register。
	Profile    string                              `json:"profile"`           // 输出规格。
	Files      []CollectionFile                    `json:"files"`             // 有界文件列表。
	Formats    []CollectionFormat                  `json:"formats"`           // 安全格式说明。
	TotalBytes int64                               `json:"totalBytes"`        // 已知总量。
	ExpiresAt  string                              `json:"expiresAt"`         // 预览失效时间。
	Warnings   []string                            `json:"warnings"`          // 预览提示。
	JMComic    *collectioncontract.JMComicMetadata `json:"jmcomic,omitempty"` // 漫画有序章节、选中页数及预算状态。
}

// CollectionTask 保存业务获取与入库状态，BT 事实仍由原下载模块维护。
type CollectionTask struct {
	ID              string                              `json:"id"`                        // 随机任务编号。
	RequestID       string                              `json:"requestId"`                 // 幂等请求编号。
	SourceID        string                              `json:"sourceId"`                  // 来源编号。
	CandidateID     string                              `json:"candidateId"`               // 可选候选编号。
	Kind            string                              `json:"kind"`                      // bt/web/local/remote/register。
	Title           string                              `json:"title"`                     // 名称。
	ContentType     string                              `json:"contentType"`               // 目标类别。
	State           string                              `json:"state"`                     // 获取阶段。
	ImportState     string                              `json:"importState"`               // 入库阶段。
	Message         string                              `json:"message"`                   // 中文状态。
	Revision        int64                               `json:"revision"`                  // 操作版本。
	DownloadID      string                              `json:"downloadId"`                // 原 BT 任务编号。
	LibraryID       int64                               `json:"libraryId"`                 // 目标库。
	JobID           int64                               `json:"jobId"`                     // 当前扫描任务。
	ItemIDs         []int64                             `json:"itemIds"`                   // 实际入库条目。
	Bytes           int64                               `json:"bytes"`                     // 已确认字节数。
	CreatedAt       string                              `json:"createdAt"`                 // 创建时间。
	UpdatedAt       string                              `json:"updatedAt"`                 // 更新时间。
	JMComicProgress *collectioncontract.JMComicProgress `json:"jmcomicProgress,omitempty"` // 执行端动态验证进度，不新增数据库列。
}

// collectionTaskRecord 附加后台恢复和不可变计划字段。
type collectionTaskRecord struct {
	CollectionTask                  // 对外任务。
	Plan           string           // 不含秘密的不可变计划。
	Fingerprint    string           // 来源与预览/候选的幂等摘要。
	LocalRoot      string           // 发布根映射。
	FinalPath      string           // 通过验证的最终目录。
	Ready          bool             // 已发布授权。
	CreateLibrary  bool             // 首次建库授权。
	Files          []CollectionFile // 产物列表。
}

// collectionManager 串行协调有限获取作业，不占用扫描任务队列。
type collectionManager struct {
	mu            sync.Mutex      // 仅串行化业务写入，不覆盖网络操作。
	refreshMu     sync.Mutex      // 同一时刻只刷新一个有界 RSS 来源。
	wake          chan struct{}   // 合并唤醒。
	client        *http.Client    // 独立超时和重定向策略。
	verifyAfter   int64           // 周期完整性检查的行号游标，避免旧成品占满检查窗口。
	externalBusy  map[string]bool // 来源写操作占用，锁内变更且不跨网络持锁。
	externalSlots chan struct{}   // 外部有界网络请求并发上限。
}

// collectionSelect 为内部任务提供稳定的读取顺序。
const collectionSelect = `SELECT id,request_id,source_id,candidate_id,kind,title,content_type,state,import_state,message,revision,download_id,library_id,job_id,item_ids,bytes,created_at,updated_at,plan,fingerprint,local_root,final_path,ready,create_library,files FROM collection_tasks`

// scanCollectionTask 解码任务并保证数组有值。
func scanCollectionTask(row rowScanner) (collectionTaskRecord, error) {
	var t collectionTaskRecord
	var ids, files string
	err := row.Scan(&t.ID, &t.RequestID, &t.SourceID, &t.CandidateID, &t.Kind, &t.Title, &t.ContentType, &t.State, &t.ImportState, &t.Message, &t.Revision, &t.DownloadID, &t.LibraryID, &t.JobID, &ids, &t.Bytes, &t.CreatedAt, &t.UpdatedAt, &t.Plan, &t.Fingerprint, &t.LocalRoot, &t.FinalPath, &t.Ready, &t.CreateLibrary, &files)
	t.ItemIDs = []int64{}
	t.Files = []CollectionFile{}
	if err == nil {
		if json.Unmarshal([]byte(ids), &t.ItemIDs) != nil || json.Unmarshal([]byte(files), &t.Files) != nil {
			err = sql.ErrNoRows
		}
	}
	return t, err
}

// collectionTaskByID 仅读取持久化记录。
func (a *App) collectionTaskByID(ctx context.Context, id string) (collectionTaskRecord, error) {
	return scanCollectionTask(a.db.QueryRowContext(ctx, collectionSelect+" WHERE id=?", id))
}

// collectionJSON 编码已经明确类型的有限模型。
func collectionJSON(value any) string { data, _ := json.Marshal(value); return string(data) }

// collectionWake 合并调度通知。
func (m *collectionManager) collectionWake() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// collectionFuture 返回统一的未来调度时间。
func collectionFuture(minutes int) string {
	return time.Now().UTC().Add(time.Duration(minutes) * time.Minute).Format(time.RFC3339Nano)
}

// schemaV3 以增量方式增加收集来源、订阅、候选和可恢复任务。
const schemaV3 = `
CREATE TABLE collection_sources(id TEXT PRIMARY KEY,body TEXT NOT NULL);
CREATE TABLE collection_subscriptions(id TEXT PRIMARY KEY,source_id TEXT NOT NULL REFERENCES collection_sources(id),body TEXT NOT NULL);
CREATE TABLE collection_candidates(id TEXT PRIMARY KEY,source_id TEXT NOT NULL REFERENCES collection_sources(id),identity TEXT NOT NULL,body TEXT NOT NULL,UNIQUE(source_id,identity));
CREATE TABLE collection_previews(id TEXT PRIMARY KEY,source_id TEXT NOT NULL REFERENCES collection_sources(id),source_revision INTEGER NOT NULL,body TEXT NOT NULL,private_ref TEXT NOT NULL,expires_at TEXT NOT NULL);
CREATE TABLE collection_tasks(id TEXT PRIMARY KEY,request_id TEXT NOT NULL UNIQUE,source_id TEXT NOT NULL,candidate_id TEXT NOT NULL DEFAULT '',kind TEXT NOT NULL,title TEXT NOT NULL,content_type TEXT NOT NULL,state TEXT NOT NULL DEFAULT 'queued',import_state TEXT NOT NULL DEFAULT 'pending',message TEXT NOT NULL DEFAULT '等待处理',revision INTEGER NOT NULL DEFAULT 1,download_id TEXT NOT NULL DEFAULT '',library_id INTEGER NOT NULL DEFAULT 0,job_id INTEGER NOT NULL DEFAULT 0,item_ids TEXT NOT NULL DEFAULT '[]',bytes INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,plan TEXT NOT NULL,fingerprint TEXT NOT NULL UNIQUE,local_root TEXT NOT NULL DEFAULT '',final_path TEXT NOT NULL DEFAULT '',ready INTEGER NOT NULL DEFAULT 0,create_library INTEGER NOT NULL DEFAULT 0,files TEXT NOT NULL DEFAULT '[]');
CREATE INDEX idx_collection_active ON collection_tasks(state,updated_at);
CREATE TABLE collection_task_events(id INTEGER PRIMARY KEY AUTOINCREMENT,task_id TEXT NOT NULL REFERENCES collection_tasks(id),stage TEXT NOT NULL,message TEXT NOT NULL,created_at TEXT NOT NULL);
`
