package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nas-content-manager/internal/collectioncontract"
	"nas-content-manager/internal/externalapi"
)

// collectionPrivate 将敏感来源和不可变获取输入与普通数据库隔离。
type collectionPrivate struct {
	URL            string                              `json:"url"`                    // 私密 RSS 或资源地址。
	ETag           string                              `json:"etag"`                   // 有界条件请求标识。
	LastModified   string                              `json:"lastModified"`           // 有界服务器修改时间。
	Request        collectioncontract.OperationRequest `json:"request"`                // 执行端固定类型请求。
	Identity       string                              `json:"identity"`               // 来源内容和输出规格身份。
	LocalPath      string                              `json:"localPath"`              // 只读登记的已授权真实目录。
	SourceRevision int64                               `json:"sourceRevision"`         // 提交时来源版本。
	WorkerInstance string                              `json:"workerInstance"`         // 已验证的持久执行端实例身份。
	External       *externalapi.Config                 `json:"external,omitempty"`     // 不可变外部 API 连接快照。
	Mutation       *externalapi.Mutation               `json:"mutation,omitempty"`     // 外部固定动作私密参数。
	ExternalData   json.RawMessage                     `json:"externalData,omitempty"` // 仅由服务端生成的固定工作流秘密快照。
}

// collectionPlan 仅保存私密引用和经过授权的选文件策略。
type collectionPlan struct {
	PrivateID        string `json:"privateId"`        // 私密文件编号。
	SelectionPattern string `json:"selectionPattern"` // 明确授权的单文件自动选择正则。
	ScanAttempts     int    `json:"scanAttempts"`     // 有界补扫计数。
	TargetLibraryID  int64  `json:"targetLibraryId"`  // 原始目标选择，零表示授权首次建库。
}

// collectionFailure 携带可以向客户端公开的中文失败和 HTTP 状态。
type collectionFailure struct {
	Status  int    // HTTP 失败状态。
	Message string // 已脱敏的中文消息。
}

// Error 返回已经脱敏的错误文本。
func (e collectionFailure) Error() string { return e.Message }

// collectionProblem 创建有明确客户端含义的失败。
func collectionProblem(status int, message string) error {
	return collectionFailure{Status: status, Message: message}
}

// collectionDigest 为去重生成不泄漏地址的摘要。
func collectionDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// collectionSecretPath 仅接受服务器生成编号，防止普通数据库引用越界。
func (a *App) collectionSecretPath(id string) (string, error) {
	if !validDownloadUUID(id) {
		return "", fmt.Errorf("私密引用无效")
	}
	return filepath.Join(a.config.DataDir, "collection-secrets", id+".json"), nil
}

// writeCollectionPrivate 原子保存限制权限的私密输入，不写普通日志。
func (a *App) writeCollectionPrivate(id string, value collectionPrivate) error {
	p, err := a.collectionSecretPath(id)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return fmt.Errorf("私密收集目录不可写")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".pending-")
	if err != nil {
		return fmt.Errorf("无法保存私密输入")
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp, p)
	}
	if err != nil {
		return fmt.Errorf("无法保存私密输入")
	}
	return nil
}

// readCollectionPrivate 对私密文件执行大小限制并拒绝符号链接。
func (a *App) readCollectionPrivate(id string) (collectionPrivate, error) {
	var value collectionPrivate
	p, err := a.collectionSecretPath(id)
	if err != nil {
		return value, err
	}
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() || st.Size() > 2<<20 {
		return value, fmt.Errorf("私密收集输入不可用")
	}
	data, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(data, &value) != nil {
		return value, fmt.Errorf("私密收集输入不可用")
	}
	return value, nil
}

// collectionSourceByID 读取已持久化公开来源。
func (a *App) collectionSourceByID(ctx context.Context, id string) (CollectionSource, error) {
	var s CollectionSource
	var body string
	err := a.db.QueryRowContext(ctx, "SELECT body FROM collection_sources WHERE id=?", id).Scan(&body)
	if err == nil {
		err = json.Unmarshal([]byte(body), &s)
	}
	return s, err
}

// collectionSources 关闭查询后返回有界来源快照。
func (a *App) collectionSources(ctx context.Context) ([]CollectionSource, error) {
	rows, err := a.db.QueryContext(ctx, "SELECT body FROM collection_sources ORDER BY id LIMIT 64")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CollectionSource{}
	for rows.Next() {
		var body string
		var s CollectionSource
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(body), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// collectionSubscriptions 返回有界订阅快照，调用方可在查询关闭后执行网络。
func (a *App) collectionSubscriptions(ctx context.Context) ([]CollectionSubscription, error) {
	rows, err := a.db.QueryContext(ctx, "SELECT body FROM collection_subscriptions ORDER BY id LIMIT 256")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CollectionSubscription{}
	for rows.Next() {
		var body string
		var s CollectionSubscription
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(body), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// collectionCandidateByID 读取不含私密资源定位符的候选。
func (a *App) collectionCandidateByID(ctx context.Context, id string) (CollectionCandidate, error) {
	var c CollectionCandidate
	var body string
	err := a.db.QueryRowContext(ctx, "SELECT body FROM collection_candidates WHERE id=?", id).Scan(&body)
	if err == nil {
		err = json.Unmarshal([]byte(body), &c)
	}
	return c, err
}

// collectionCandidates 返回指定来源的有限候选。
func (a *App) collectionCandidates(ctx context.Context, sourceID string, limit int) ([]CollectionCandidate, error) {
	rows, err := a.db.QueryContext(ctx, "SELECT body FROM collection_candidates WHERE source_id=? ORDER BY rowid DESC LIMIT ?", sourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CollectionCandidate{}
	for rows.Next() {
		var body string
		var c CollectionCandidate
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(body), &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// persistCollectionTask 原子保存修订号，迟到执行结果不能覆盖用户取消。
func (a *App) persistCollectionTask(ctx context.Context, t *collectionTaskRecord) error {
	result, err := a.db.ExecContext(ctx, `UPDATE collection_tasks SET state=?,import_state=?,message=?,revision=revision+1,download_id=?,library_id=?,job_id=?,item_ids=?,bytes=?,updated_at=?,plan=?,local_root=?,final_path=?,ready=?,files=? WHERE id=? AND revision=?`, t.State, t.ImportState, t.Message, t.DownloadID, t.LibraryID, t.JobID, collectionJSON(t.ItemIDs), t.Bytes, now(), t.Plan, t.LocalRoot, t.FinalPath, t.Ready, collectionJSON(t.Files), t.ID, t.Revision)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return collectionProblem(409, "任务状态已更新，请刷新后操作")
	}
	t.Revision++
	t.UpdatedAt = now()
	return nil
}

// collectionEvent 保存有限中文事件，禁止将底层工具输出作为消息传入。
func (a *App) collectionEvent(ctx context.Context, id string, stage string, message string) {
	_, _ = a.db.ExecContext(ctx, "INSERT INTO collection_task_events(task_id,stage,message,created_at) VALUES(?,?,?,?)", id, stage, message, now())
	_, _ = a.db.ExecContext(ctx, "DELETE FROM collection_task_events WHERE task_id=? AND id NOT IN(SELECT id FROM collection_task_events WHERE task_id=? ORDER BY id DESC LIMIT 100)", id, id)
	a.logger.Info("资源收集阶段更新", "taskId", id, "stage", stage, "message", message)
}

// collectionRelative 限定跨平台安全相对路径，拒绝链接式路径歧义。
func collectionRelative(value string) (string, error) {
	if value == "" || value == "." {
		return "", nil
	}
	if len(value) > 2048 || strings.ContainsAny(value, "\\:\x00\r\n") || strings.HasPrefix(value, "/") {
		return "", collectionProblem(400, "请选择授权根内的相对目录")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", collectionProblem(400, "相对目录包含无效片段")
		}
	}
	if !filepath.IsLocal(filepath.FromSlash(value)) {
		return "", collectionProblem(400, "相对目录越界")
	}
	return value, nil
}

// collectionExpired 判断统一格式的预览是否已过期。
func collectionExpired(value string) bool {
	t, err := time.Parse(time.RFC3339Nano, value)
	return err != nil || !t.After(time.Now())
}

// collectionTaskList 读取有限执行快照，不在打开游标时嵌套查询。
func (a *App) collectionTaskList(ctx context.Context) ([]collectionTaskRecord, error) {
	rows, err := a.db.QueryContext(ctx, collectionSelect+" WHERE state NOT IN('completed','canceled','failed','reconciliation_required') AND NOT(state='importing' AND import_state='failed') ORDER BY created_at LIMIT 256")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []collectionTaskRecord{}
	for rows.Next() {
		t, e := scanCollectionTask(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// insertCollectionTask 在外部提交前持久化不可变意图并执行两层去重。
func (a *App) insertCollectionTask(ctx context.Context, t collectionTaskRecord) (collectionTaskRecord, error) {
	if old, err := scanCollectionTask(a.db.QueryRowContext(ctx, collectionSelect+" WHERE request_id=?", t.RequestID)); err == nil {
		var oldPlan, newPlan collectionPlan
		_ = json.Unmarshal([]byte(old.Plan), &oldPlan)
		_ = json.Unmarshal([]byte(t.Plan), &newPlan)
		if old.Fingerprint != t.Fingerprint || oldPlan.TargetLibraryID != newPlan.TargetLibraryID || old.CreateLibrary != t.CreateLibrary {
			return old, collectionProblem(409, "该请求编号已用于其他收集计划")
		}
		return old, nil
	} else if err != sql.ErrNoRows {
		return old, err
	}
	if old, err := scanCollectionTask(a.db.QueryRowContext(ctx, collectionSelect+" WHERE fingerprint=?", t.Fingerprint)); err == nil {
		return old, collectionProblem(409, "相同资源已有收集计划，请查看原任务并沿用原请求编号")
	} else if err != sql.ErrNoRows {
		return old, err
	}
	var active int
	if err := a.db.QueryRowContext(ctx, "SELECT count(*) FROM collection_tasks WHERE state NOT IN('completed','failed','canceled')").Scan(&active); err != nil {
		return t, err
	}
	if active >= 256 {
		return t, collectionProblem(409, "收集队列已达到 256 项上限")
	}
	t.ItemIDs = []int64{}
	if t.Files == nil {
		t.Files = []CollectionFile{}
	}
	t.State = "queued"
	t.ImportState = "pending"
	t.Message = "等待资源收集"
	t.Revision = 1
	t.CreatedAt = now()
	t.UpdatedAt = t.CreatedAt
	_, err := a.db.ExecContext(ctx, `INSERT INTO collection_tasks(id,request_id,source_id,candidate_id,kind,title,content_type,state,import_state,message,revision,library_id,item_ids,created_at,updated_at,plan,fingerprint,local_root,final_path,create_library,files) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, t.ID, t.RequestID, t.SourceID, t.CandidateID, t.Kind, t.Title, t.ContentType, t.State, t.ImportState, t.Message, t.Revision, t.LibraryID, "[]", t.CreatedAt, t.UpdatedAt, t.Plan, t.Fingerprint, t.LocalRoot, t.FinalPath, t.CreateLibrary, collectionJSON(t.Files))
	if err != nil {
		return t, collectionProblem(409, "收集计划已存在或发生冲突，请刷新列表")
	}
	a.collectionEvent(ctx, t.ID, "queued", t.Message)
	a.collections.collectionWake()
	return t, nil
}
