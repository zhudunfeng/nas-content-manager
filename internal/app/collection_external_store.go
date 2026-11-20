package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nas-content-manager/internal/externalapi"
)

// collectionExternalConnectionInput 是不会从 API 读取回来的连接参数。
type collectionExternalConnectionInput struct {
	BaseURL string `json:"baseUrl"` // 实例基址，留空保留同服务原值。
	APIKey  string `json:"apiKey"`  // 已生成的 API 凭据，留空保留原值。
}

// collectionExternalAction 是可安全显示的外部操作审计。
type collectionExternalAction struct {
	ID             string `json:"id"`             // 服务器生成的操作编号。
	RequestID      string `json:"requestId"`      // 用户确认意图的稳定 UUID。
	SourceID       string `json:"sourceId"`       // 外部来源编号。
	SourceRevision int64  `json:"sourceRevision"` // 接受时的来源修订。
	Kind           string `json:"kind"`           // 固定业务动作。
	TargetID       string `json:"targetId"`       // 已确认的外部编号。
	State          string `json:"state"`          // dispatching/succeeded/failed/uncertain/unconfirmed。
	Message        string `json:"message"`        // 脱敏中文结果。
	CreatedAt      string `json:"createdAt"`      // 接受时间。
	UpdatedAt      string `json:"updatedAt"`      // 最近状态时间。
}

// collectionExternalRecord 保存只在后端使用的持久意图引用。
type collectionExternalRecord struct {
	collectionExternalAction        // 公开结果。
	IntentHash               string // 原始不可变意图摘要。
	PrivateRef               string // 不可变秘密文件引用。
	ReceiptID                string // 有界外部回执编号。
}

// schemaV4 增量增加私密连接引用和不会自动重发的外部动作日志。
const schemaV4 = `
CREATE TABLE collection_external_connections(source_id TEXT PRIMARY KEY REFERENCES collection_sources(id),revision INTEGER NOT NULL,private_ref TEXT NOT NULL);
CREATE TABLE collection_external_actions(id TEXT PRIMARY KEY,request_id TEXT NOT NULL UNIQUE,source_id TEXT NOT NULL REFERENCES collection_sources(id),source_revision INTEGER NOT NULL,kind TEXT NOT NULL,target_id TEXT NOT NULL,state TEXT NOT NULL,message TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,intent_hash TEXT NOT NULL,private_ref TEXT NOT NULL,receipt_id TEXT NOT NULL DEFAULT '');
CREATE INDEX idx_collection_external_actions_source ON collection_external_actions(source_id,created_at);
CREATE TABLE collection_external_choices(id TEXT PRIMARY KEY,source_id TEXT NOT NULL REFERENCES collection_sources(id),source_revision INTEGER NOT NULL,kind TEXT NOT NULL,private_ref TEXT NOT NULL,expires_at TEXT NOT NULL,used_by TEXT NOT NULL DEFAULT '');
CREATE INDEX idx_collection_external_choices_source ON collection_external_choices(source_id,expires_at);
`

// collectionExternalConfig 读取对应来源的独立私密连接。
func (a *App) collectionExternalConfig(ctx context.Context, source CollectionSource) (externalapi.Config, error) {
	var ref string
	if err := a.db.QueryRowContext(ctx, "SELECT private_ref FROM collection_external_connections WHERE source_id=? AND revision=?", source.ID, source.Revision).Scan(&ref); err != nil {
		return externalapi.Config{}, collectionProblem(409, "API 连接未配置或来源已更新，请编辑后重新检测")
	}
	private, err := a.readCollectionPrivate(ref)
	if err != nil || private.External == nil || private.External.Service != source.ExternalService {
		return externalapi.Config{}, collectionProblem(503, "私密 API 连接不可用，请重新填写或恢复独立秘密备份")
	}
	return *private.External, nil
}

// prepareCollectionExternal 保存新的不可变秘密快照；调用方持有短配置锁。
func (a *App) prepareCollectionExternal(ctx context.Context, old CollectionSource, input collectionSourceInput) (string, error) {
	if input.Kind != "external" {
		if input.ExternalService != "" || input.Connection != nil || input.ClearConnection {
			return "", collectionProblem(400, "仅外部来源可配置 API 连接")
		}
		return "", nil
	}
	if input.ClearConnection {
		if input.Connection != nil {
			return "", collectionProblem(400, "清除连接时不能同时提供新凭据")
		}
		return "", nil
	}
	if input.ExternalService == "" {
		if input.Connection != nil {
			return "", collectionProblem(400, "请先选择外部服务类型")
		}
		return "", nil
	}
	config := externalapi.Config{Service: input.ExternalService}
	if old.APIConfigured && old.ExternalService == input.ExternalService {
		var err error
		config, err = a.collectionExternalConfig(ctx, old)
		if err != nil && (input.Connection == nil || input.Connection.BaseURL == "" || input.Connection.APIKey == "") {
			return "", err
		}
		config.Service = input.ExternalService
	}
	if input.Connection != nil {
		if value := strings.TrimSpace(input.Connection.BaseURL); value != "" {
			config.BaseURL = strings.TrimRight(value, "/")
		}
		if input.Connection.APIKey != "" {
			config.APIKey = input.Connection.APIKey
		}
	}
	if err := externalapi.ValidateConfig(config); err != nil {
		return "", collectionExternalError(err)
	}
	u, _ := url.Parse(config.BaseURL)
	if u.Scheme == "http" && !input.AllowPrivate {
		return "", collectionProblem(400, "HTTP API 仅用于明确授权的内网服务，请勾选允许访问私网或使用 HTTPS")
	}
	ref, err := newDownloadUUID()
	if err != nil {
		return "", err
	}
	if err = a.writeCollectionPrivate(ref, collectionPrivate{External: &config}); err != nil {
		return "", err
	}
	return ref, nil
}

// collectionExternalError 只返回固定中文适配器错误，不公开 URL 或响应体。
func collectionExternalError(err error) error {
	var fault *externalapi.Fault
	if errors.As(err, &fault) {
		return collectionProblem(502, fault.Message)
	}
	return collectionProblem(502, "外部服务暂不可用，请检查连接后重试")
}

// collectionExternalOpen 为一次操作构造受应用生命周期与并发限制的连接。
func (a *App) collectionExternalOpen(ctx context.Context, source CollectionSource, config externalapi.Config) (*externalapi.Client, context.Context, func(), error) {
	select {
	case a.collections.externalSlots <- struct{}{}:
	default:
		return nil, ctx, nil, collectionProblem(429, "外部服务请求较多，请稍后重试")
	}
	requestCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	stop := context.AfterFunc(a.ctx, cancel)
	origin, err := url.Parse(config.BaseURL)
	if err != nil {
		cancel()
		stop()
		<-a.collections.externalSlots
		return nil, ctx, nil, collectionProblem(400, "外部地址无效")
	}
	httpClient := collectionRSSClient(origin, source.AllowPrivate)
	if origin.Scheme == "http" {
		// 明文凭据只允许明确授权的真实内网，DNS 再绑定也不能转发到公网。
		transport := httpClient.Transport.(*http.Transport)
		transport.DialContext = func(dialCtx context.Context, network string, address string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(address)
			if splitErr != nil || !strings.EqualFold(host, origin.Hostname()) || !source.AllowPrivate {
				return nil, collectionProblem(400, "HTTP API 仅限已授权内网")
			}
			ips, resolveErr := net.DefaultResolver.LookupIPAddr(dialCtx, host)
			if resolveErr != nil || len(ips) == 0 || len(ips) > 16 {
				return nil, collectionProblem(400, "内网服务无法安全解析")
			}
			for _, ip := range ips {
				if !collectionNetworkIP(ip.IP, true) || !(ip.IP.IsPrivate() || ip.IP.IsLoopback()) {
					return nil, collectionProblem(400, "HTTP API 目标不属于真实内网")
				}
			}
			dialer := net.Dialer{Timeout: 6 * time.Second}
			return dialer.DialContext(dialCtx, network, net.JoinHostPort(ips[0].IP.String(), port))
		}
	}
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	client, err := externalapi.New(config, httpClient)
	cleanup := func() { cancel(); stop(); httpClient.CloseIdleConnections(); <-a.collections.externalSlots }
	if err != nil {
		cleanup()
		return nil, ctx, nil, collectionExternalError(err)
	}
	return client, requestCtx, cleanup, nil
}

// scanCollectionExternal 解码单条外部操作记录。
func scanCollectionExternal(row rowScanner) (collectionExternalRecord, error) {
	var record collectionExternalRecord
	err := row.Scan(&record.ID, &record.RequestID, &record.SourceID, &record.SourceRevision, &record.Kind, &record.TargetID, &record.State, &record.Message, &record.CreatedAt, &record.UpdatedAt, &record.IntentHash, &record.PrivateRef, &record.ReceiptID)
	return record, err
}

// collectionExternalSelect 固定审计读取字段，不包含秘密载荷。
const collectionExternalSelect = `SELECT id,request_id,source_id,source_revision,kind,target_id,state,message,created_at,updated_at,intent_hash,private_ref,receipt_id FROM collection_external_actions`

// collectionExternalFinish 在请求中断时仍用应用生命周期落盘不确定状态。
func (a *App) collectionExternalFinish(record *collectionExternalRecord, outcome externalapi.Outcome, state string, message string) error {
	ctx, cancel := context.WithTimeout(a.ctx, 4*time.Second)
	defer cancel()
	if outcome.TargetID != "" {
		record.TargetID = outcome.TargetID
	}
	record.ReceiptID = outcome.ReceiptID
	record.State, record.Message, record.UpdatedAt = state, message, now()
	_, err := a.db.ExecContext(ctx, "UPDATE collection_external_actions SET target_id=?,state=?,message=?,updated_at=?,receipt_id=? WHERE id=?", record.TargetID, state, message, record.UpdatedAt, record.ReceiptID, record.ID)
	if err == nil {
		a.logger.Info("外部服务操作状态更新", "sourceId", record.SourceID, "actionId", record.ID, "kind", record.Kind, "state", state)
	}
	return err
}

// collectionExternalIntentHash 将私密输入纳入不可逆幂等校验而不写入普通数据库。
func collectionExternalIntentHash(sourceID string, revision int64, mutation externalapi.Mutation) string {
	input := struct {
		SourceID string               // 原始来源编号。
		Revision int64                // 用户确认的来源修订。
		Mutation externalapi.Mutation // 固定业务意图，私密载荷不参与公开序列化。
	}{sourceID, revision, mutation}
	data, _ := json.Marshal(input)
	return collectionDigest(string(data))
}

// collectionExternalBegin 保证同一来源的写入与配置切换不发生竞态。
func (a *App) collectionExternalBegin(id string) bool {
	a.collections.mu.Lock()
	defer a.collections.mu.Unlock()
	if a.collections.externalBusy[id] {
		return false
	}
	a.collections.externalBusy[id] = true
	return true
}

// collectionExternalEnd 释放来源操作占用，不影响其它来源或收集任务。
func (a *App) collectionExternalEnd(id string) {
	a.collections.mu.Lock()
	delete(a.collections.externalBusy, id)
	a.collections.mu.Unlock()
}

// collectionExternalRemember 原子登记确认意图，之后才可发送上游写请求。
func (a *App) collectionExternalRemember(ctx context.Context, record collectionExternalRecord) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	private, err := a.readCollectionPrivate(record.PrivateRef)
	if err != nil || private.Mutation == nil {
		return collectionProblem(409, "操作私密快照不可用")
	}
	if private.Mutation.SelectionID != "" {
		result, updateErr := tx.ExecContext(ctx, "UPDATE collection_external_choices SET used_by=? WHERE id=? AND source_id=? AND source_revision=? AND expires_at>? AND used_by=''", record.RequestID, private.Mutation.SelectionID, record.SourceID, record.SourceRevision, now())
		if updateErr != nil {
			return updateErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return collectionProblem(409, "此选择已过期或已用于操作，请刷新后重新选择")
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO collection_external_actions(id,request_id,source_id,source_revision,kind,target_id,state,message,created_at,updated_at,intent_hash,private_ref,receipt_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", record.ID, record.RequestID, record.SourceID, record.SourceRevision, record.Kind, record.TargetID, record.State, record.Message, record.CreatedAt, record.UpdatedAt, record.IntentHash, record.PrivateRef, record.ReceiptID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// collectionExternalSaveRef 在来源事务中保存新的连接引用。
func collectionExternalSaveRef(ctx context.Context, tx *sql.Tx, source CollectionSource, ref string) error {
	if ref == "" {
		_, err := tx.ExecContext(ctx, "DELETE FROM collection_external_connections WHERE source_id=?", source.ID)
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO collection_external_connections(source_id,revision,private_ref) VALUES(?,?,?) ON CONFLICT(source_id) DO UPDATE SET revision=excluded.revision,private_ref=excluded.private_ref", source.ID, source.Revision, ref)
	return err
}
