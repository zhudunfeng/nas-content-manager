package app

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"nas-content-manager/internal/externalapi"
)

// collectionExternalRoutes 将外部控制接入原有会话与同源边界。
func (a *App) collectionExternalRoutes(register func(string, http.HandlerFunc)) {
	a.collectionMoviePilotRoutes(register)
	register("POST /api/collection/sources/{id}/external/test", a.collectionExternalTest)
	register("GET /api/collection/sources/{id}/external/items", a.collectionExternalItems)
	register("GET /api/collection/sources/{id}/external/options", a.collectionExternalOptions)
	register("GET /api/collection/sources/{id}/external/lookup", a.collectionExternalLookup)
	register("GET /api/collection/sources/{id}/external/actions", a.collectionExternalActions)
	register("POST /api/collection/sources/{id}/external/actions", a.collectionExternalActionCreate)
	register("POST /api/collection/sources/{id}/external/actions/{actionId}/reconcile", a.collectionExternalReconcile)
	register("POST /api/collection/sources/{id}/external/actions/{actionId}/acknowledge", a.collectionExternalAcknowledge)
}

// collectionExternalAcknowledge 记录用户已在外部核查，解除阻塞但不伪造上游成功。
func (a *App) collectionExternalAcknowledge(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CheckedExternally bool `json:"checkedExternally"` /* 用户明确确认已核查外部状态。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !input.CheckedExternally {
		fail(w, 400, "请先在外部应用核对实际结果")
		return
	}
	id := r.PathValue("id")
	if !a.collectionExternalBegin(id) {
		fail(w, 409, "此来源正在处理操作，请稍后核对")
		return
	}
	defer a.collectionExternalEnd(id)
	record, err := scanCollectionExternal(a.db.QueryRowContext(r.Context(), collectionExternalSelect+" WHERE id=? AND source_id=?", r.PathValue("actionId"), id))
	if err != nil {
		fail(w, 404, "操作记录不存在")
		return
	}
	if record.State != "uncertain" {
		fail(w, 409, "此操作无需人工解除，请刷新记录")
		return
	}
	if err = a.collectionExternalFinish(&record, externalapi.Outcome{TargetID: record.TargetID, ReceiptID: record.ReceiptID}, "unconfirmed", "用户已在外部应用核查并解除阻塞，系统未确认执行结果，也未重新提交"); err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"action": record.collectionExternalAction})
}

// collectionExternalSource 只接受已经配置 API 的固定外部来源。
func (a *App) collectionExternalSource(r *http.Request) (CollectionSource, error) {
	source, err := a.collectionSourceByID(r.Context(), r.PathValue("id"))
	if err != nil {
		return source, collectionProblem(404, "外部来源不存在")
	}
	if source.Kind != "external" || !source.APIConfigured {
		return source, collectionProblem(409, "请先为此外部来源配置 API 连接")
	}
	return source, nil
}

// collectionExternalReadClient 为只读页面加载建立独立请求预算。
func (a *App) collectionExternalReadClient(r *http.Request) (*externalapi.Client, CollectionSource, func(), error) {
	source, err := a.collectionExternalSource(r)
	if err != nil {
		return nil, source, nil, err
	}
	config, err := a.collectionExternalConfig(r.Context(), source)
	if err != nil {
		return nil, source, nil, err
	}
	client, ctx, cleanup, err := a.collectionExternalOpen(r.Context(), source, config)
	if err != nil {
		return nil, source, nil, err
	}
	// 后续读取共用包含应用退出取消信号的请求上下文。
	*r = *r.WithContext(ctx)
	if _, err = client.Probe(ctx); err != nil {
		cleanup()
		return nil, source, nil, collectionExternalError(err)
	}
	return client, source, cleanup, nil
}

// collectionExternalTest 仅检测实例与凭据，不能触发搜索或扫描。
func (a *App) collectionExternalTest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Revision int64 `json:"revision"` /* 保存后的来源版本。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	source, err := a.collectionExternalSource(r)
	if err != nil {
		collectionFail(w, err)
		return
	}
	if source.Revision != input.Revision {
		fail(w, 409, "来源已更新，请刷新后重新检测")
		return
	}
	config, err := a.collectionExternalConfig(r.Context(), source)
	if err != nil {
		collectionFail(w, err)
		return
	}
	client, ctx, cleanup, err := a.collectionExternalOpen(r.Context(), source, config)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer cleanup()
	info, err := client.Probe(ctx)
	if err != nil {
		collectionFail(w, collectionExternalError(err))
		return
	}
	a.logger.Info("外部服务连接检测完成", "sourceId", source.ID, "service", source.ExternalService, "supported", info.Supported)
	respond(w, 200, map[string]any{"info": info})
}

// collectionExternalItems 分页读取单个服务分区，错误不转换为空列表。
func (a *App) collectionExternalItems(w http.ResponseWriter, r *http.Request) {
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		var err error
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 || page > 10000 {
			fail(w, 400, "列表页码无效")
			return
		}
	}
	section := r.URL.Query().Get("section")
	if section == "" {
		section = "resources"
	}
	client, _, cleanup, err := a.collectionExternalReadClient(r)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer cleanup()
	result, err := client.List(r.Context(), section, page)
	if err != nil {
		collectionFail(w, collectionExternalError(err))
		return
	}
	respond(w, 200, result)
}

// collectionExternalOptions 只提供已有实例选项，不把外部路径当本地授权。
func (a *App) collectionExternalOptions(w http.ResponseWriter, r *http.Request) {
	client, _, cleanup, err := a.collectionExternalReadClient(r)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer cleanup()
	result, err := client.Options(r.Context())
	if err != nil {
		collectionFail(w, collectionExternalError(err))
		return
	}
	respond(w, 200, result)
}

// collectionExternalLookup 只查资料候选，不执行会触发下载的搜索。
func (a *App) collectionExternalLookup(w http.ResponseWriter, r *http.Request) {
	client, _, cleanup, err := a.collectionExternalReadClient(r)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer cleanup()
	items, err := client.Lookup(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		collectionFail(w, collectionExternalError(err))
		return
	}
	if items == nil {
		items = []externalapi.Item{}
	}
	respond(w, 200, map[string]any{"items": items})
}

// collectionExternalActions 返回最近三十条公开操作，不读取秘密文件。
func (a *App) collectionExternalActions(w http.ResponseWriter, r *http.Request) {
	if _, err := a.collectionSourceByID(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 404, "来源不存在")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), collectionExternalSelect+" WHERE source_id=? ORDER BY rowid DESC LIMIT 30", r.PathValue("id"))
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer rows.Close()
	items := []collectionExternalAction{}
	for rows.Next() {
		record, readErr := scanCollectionExternal(rows)
		if readErr != nil {
			collectionFail(w, readErr)
			return
		}
		items = append(items, record.collectionExternalAction)
	}
	if err = rows.Err(); err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"actions": items})
}

// collectionExternalActionCreate 在持久化意图后只执行一次外部写操作。
func (a *App) collectionExternalActionCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RequestID            string `json:"requestId"` // 用户确认后保持不变的 UUID。
		Revision             int64  `json:"revision"`  // 确认时来源修订。
		externalapi.Mutation        // 固定业务参数。
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validDownloadUUID(input.RequestID) {
		fail(w, 400, "操作请求编号无效")
		return
	}
	id := r.PathValue("id")
	intentHash := collectionExternalIntentHash(id, input.Revision, input.Mutation)
	// 已接受请求优先返回原结果，即便随后来源修订或网络发生变化。
	old, err := scanCollectionExternal(a.db.QueryRowContext(r.Context(), collectionExternalSelect+" WHERE request_id=?", input.RequestID))
	if err == nil {
		if old.IntentHash != intentHash || old.SourceID != id {
			fail(w, 409, "此请求编号已用于不同操作，请先核对原操作")
			return
		}
		respond(w, 200, map[string]any{"action": old.collectionExternalAction})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		collectionFail(w, err)
		return
	}
	if !a.collectionExternalBegin(id) {
		fail(w, 409, "此来源正在处理操作，请先查看操作记录")
		return
	}
	defer a.collectionExternalEnd(id)
	source, err := a.collectionExternalSource(r)
	if err != nil {
		collectionFail(w, err)
		return
	}
	if !source.Enabled || source.Revision != input.Revision {
		fail(w, 409, "来源已停用或更新，请刷新后确认")
		return
	}
	if err = externalapi.ValidateMutation(input.Mutation, source.ExternalService); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err = a.collectionMoviePilotHydrate(r, source, &input.Mutation); err != nil {
		collectionFail(w, err)
		return
	}
	var unresolved int
	if err = a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM collection_external_actions WHERE source_id=? AND state IN('dispatching','uncertain')", id).Scan(&unresolved); err != nil {
		collectionFail(w, err)
		return
	}
	if unresolved > 0 {
		fail(w, 409, "此来源有尚未确认的操作，请先在操作记录核对结果")
		return
	}
	config, err := a.collectionExternalConfig(r.Context(), source)
	if err != nil {
		collectionFail(w, err)
		return
	}
	client, ctx, cleanup, err := a.collectionExternalOpen(r.Context(), source, config)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer cleanup()
	// 连接预检失败时未发送业务写，用户可以修正配置后重新确认。
	if _, err = client.Probe(ctx); err != nil {
		collectionFail(w, collectionExternalError(err))
		return
	}
	ref, err := newDownloadUUID()
	if err != nil {
		collectionFail(w, err)
		return
	}
	if err = a.writeCollectionPrivate(ref, collectionPrivate{External: &config, Mutation: &input.Mutation, ExternalData: input.Mutation.Payload, SourceRevision: source.Revision}); err != nil {
		collectionFail(w, err)
		return
	}
	stamp := now()
	record := collectionExternalRecord{collectionExternalAction: collectionExternalAction{ID: ref, RequestID: input.RequestID, SourceID: id, SourceRevision: source.Revision, Kind: input.Kind, TargetID: input.TargetID, State: "dispatching", Message: "操作意图已保存，正在提交外部服务", CreatedAt: stamp, UpdatedAt: stamp}, IntentHash: intentHash, PrivateRef: ref}
	if err = a.collectionExternalRemember(r.Context(), record); err != nil {
		// 意图未获原子登记时没有上游副作用，清理这次未引用的私密文件。
		if privatePath, pathErr := a.collectionSecretPath(ref); pathErr == nil {
			_ = os.Remove(privatePath)
		}
		collectionFail(w, err)
		return
	}
	a.logger.Info("外部服务操作已接受", "sourceId", id, "actionId", ref, "kind", input.Kind)
	outcome, executeErr := client.Execute(ctx, input.Mutation)
	state, message := "succeeded", outcome.Message
	if message == "" {
		message = "外部服务已接受操作，文件仍需独立确认并扫描入库"
	}
	if executeErr != nil {
		state, message = "uncertain", "提交结果尚未确认，请先核对状态，系统不会自动重复提交"
		var fault *externalapi.Fault
		if errors.As(executeErr, &fault) && fault.Rejected {
			state, message = "failed", fault.Message
		}
	}
	if err = a.collectionExternalFinish(&record, outcome, state, message); err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"action": record.collectionExternalAction})
}

// collectionExternalReconcile 通过原连接和原意图只读核对，绝不重发未知写操作。
func (a *App) collectionExternalReconcile(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	id := r.PathValue("id")
	if !a.collectionExternalBegin(id) {
		fail(w, 409, "此来源正在处理操作，请稍后核对")
		return
	}
	defer a.collectionExternalEnd(id)
	record, err := scanCollectionExternal(a.db.QueryRowContext(r.Context(), collectionExternalSelect+" WHERE id=? AND source_id=?", r.PathValue("actionId"), id))
	if err != nil {
		fail(w, 404, "操作记录不存在")
		return
	}
	if record.State != "uncertain" && record.State != "dispatching" {
		respond(w, 200, map[string]any{"action": record.collectionExternalAction})
		return
	}
	source, err := a.collectionExternalSource(r)
	if err != nil {
		collectionFail(w, err)
		return
	}
	private, err := a.readCollectionPrivate(record.PrivateRef)
	if err != nil || private.External == nil || private.Mutation == nil {
		fail(w, 503, "原操作的独立秘密快照不可用，请恢复秘密备份")
		return
	}
	current, err := a.collectionExternalConfig(r.Context(), source)
	if err != nil {
		collectionFail(w, err)
		return
	}
	if current != *private.External {
		fail(w, 409, "连接已改变，请先恢复原连接后核对历史操作")
		return
	}
	client, ctx, cleanup, err := a.collectionExternalOpen(r.Context(), source, *private.External)
	if err != nil {
		collectionFail(w, err)
		return
	}
	defer cleanup()
	if _, err = client.Probe(ctx); err != nil {
		collectionFail(w, collectionExternalError(err))
		return
	}
	private.Mutation.Payload = private.ExternalData
	outcome, confirmed, err := client.Reconcile(ctx, *private.Mutation)
	if err != nil {
		collectionFail(w, collectionExternalError(err))
		return
	}
	state, message := "uncertain", "暂时无法确认提交结果，请在外部应用核实；系统未重复提交"
	if confirmed {
		state, message = "succeeded", outcome.Message
		if strings.TrimSpace(message) == "" {
			message = "已核对外部目标状态，文件仍需独立扫描入库"
		}
	}
	if err = a.collectionExternalFinish(&record, outcome, state, message); err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"action": record.collectionExternalAction})
}
