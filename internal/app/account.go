package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// accountReauthWindow 是单进程账号再认证的固定计数窗口。
const accountReauthWindow = 5 * time.Minute

// accountReauthLimitKeys 限制同时保留的账号与会话计数键总数。
const accountReauthLimitKeys = 2048

// accountReauthLimiter 独立限制账号及实际会话的 bcrypt 比较次数。
type accountReauthLimiter struct {
	mu       sync.Mutex              // 仅保护预算预占，不跨 bcrypt 或数据库操作。
	accounts map[int64]loginAttempt  // 账号预算以用户编号为键，不含凭据修订。
	sessions map[string]loginAttempt // 会话预算以用户编号和公开会话编号为键。
}

// reserve 在短锁内同时预占两个预算；被拒请求不增加计数或延长窗口。
func (l *accountReauthLimiter) reserve(userID int64, sessionID string, timestamp time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.accounts == nil {
		l.accounts = make(map[int64]loginAttempt)
		l.sessions = make(map[string]loginAttempt)
	}
	for key, value := range l.accounts {
		if !value.Until.After(timestamp) {
			delete(l.accounts, key)
		}
	}
	for key, value := range l.sessions {
		if !value.Until.After(timestamp) {
			delete(l.sessions, key)
		}
	}
	key := strconv.FormatInt(userID, 10) + ":" + sessionID
	account, accountExists := l.accounts[userID]
	session, sessionExists := l.sessions[key]
	var retry time.Duration
	if account.Count >= 12 {
		retry = account.Until.Sub(timestamp)
	}
	if session.Count >= 6 && session.Until.Sub(timestamp) > retry {
		retry = session.Until.Sub(timestamp)
	}
	if retry > 0 {
		return retry
	}
	needed := 0
	if !accountExists {
		needed++
	}
	if !sessionExists {
		needed++
	}
	if len(l.accounts)+len(l.sessions)+needed > accountReauthLimitKeys {
		return accountReauthWindow
	}
	if !accountExists {
		account.Until = timestamp.Add(accountReauthWindow)
	}
	if !sessionExists {
		session.Until = timestamp.Add(accountReauthWindow)
	}
	account.Count++
	session.Count++
	l.accounts[userID], l.sessions[key] = account, session
	return 0
}

// accountRoutes 注册账号域端点，保留全局同源保护与请求体上限。
func (a *App) accountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/account/sessions", a.accountSessions)
	mux.HandleFunc("POST /api/account/password", a.accountPassword)
	mux.HandleFunc("DELETE /api/account/sessions/{sessionId}", a.accountRevokeSession)
	mux.HandleFunc("POST /api/account/sessions/revoke-others", a.accountRevokeOthers)
}

// accountActor 从实际 Cookie 读取完整私有快照，供敏感事务再次核对。
func (a *App) accountActor(w http.ResponseWriter, r *http.Request) (accountMutationActor, bool) {
	cookie, err := r.Cookie("nas_session")
	if err != nil || len(cookie.Value) != 64 {
		fail(w, 401, "请先登录")
		return accountMutationActor{}, false
	}
	actor := accountMutationActor{tokenDigest: tokenHash(cookie.Value)}
	actor.expected, err = readAccountSession(r.Context(), a.db, actor.tokenDigest, time.Now())
	if err != nil {
		accountOperationError(w, err)
		return accountMutationActor{}, false
	}
	return actor, true
}

// decodeAccountJSON 精确拒绝大小写别名、转义后重复键、空值及任何额外 JSON 内容。
func decodeAccountJSON(w http.ResponseWriter, r *http.Request, keys ...string) (map[string]string, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		fail(w, 415, "请使用 JSON 请求")
		return nil, false
	}
	if r.ContentLength > 64<<10 {
		discardOversizedAccountBody(w, r)
		fail(w, 413, "请求内容超过大小限制")
		return nil, false
	}
	decoder := json.NewDecoder(r.Body)
	value, err := decoder.Token()
	if err != nil || value != json.Delim('{') {
		return nil, accountJSONError(w, err)
	}
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	values := make(map[string]string, len(keys))
	for decoder.More() {
		value, err = decoder.Token()
		if err != nil {
			return nil, accountJSONError(w, err)
		}
		key, stringKey := value.(string)
		_, exists := values[key]
		if !stringKey || !allowed[key] || exists {
			return nil, accountJSONError(w, nil)
		}
		value, err = decoder.Token()
		if err != nil {
			return nil, accountJSONError(w, err)
		}
		text, valid := value.(string)
		if !valid {
			return nil, accountJSONError(w, nil)
		}
		values[key] = text
	}
	if value, err = decoder.Token(); err != nil || value != json.Delim('}') {
		return nil, accountJSONError(w, err)
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, accountJSONError(w, err)
	}
	if len(values) != len(keys) {
		return nil, accountJSONError(w, nil)
	}
	return values, true
}

// discardOversizedAccountBody 在账号已知超限时有界触发原生限额收尾，避免直接关闭丢失拒绝响应。
func discardOversizedAccountBody(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Connection", "close")
	// 不因拒绝正文而发送 100 Continue；共享中间件的大体跳过规则保持独立。
	if r.Body == nil || r.Header.Get("Expect") != "" {
		return
	}
	controller := http.NewResponseController(w)
	// 仅此账号拒绝分支最多等待 250 毫秒，不影响媒体传输或普通请求预算。
	if err := controller.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
		return
	}
	// 最多实际读取 64 KiB 加一个溢出字节，不解析、不保留，也不等待完整大文件。
	_, err := io.CopyN(io.Discard, http.MaxBytesReader(w, r.Body, 64<<10), (64<<10)+1)
	var limit *http.MaxBytesError
	if err == nil || errors.As(err, &limit) {
		_ = controller.SetReadDeadline(time.Time{})
		return
	}
	// 超时、提前断流等异常不恢复预算，避免 net/http 关闭原正文时再次等待。
	_ = controller.SetReadDeadline(time.Now())
}

// accountJSONError 仅输出固定错误，不把包含秘密的解码细节返回客户端。
func accountJSONError(w http.ResponseWriter, err error) bool {
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) {
		fail(w, 413, "请求内容超过大小限制")
	} else {
		fail(w, 400, "请求必须包含规定字段的单个 JSON 对象")
	}
	return false
}

// accountOperationError 把私有存储错误映射为安全且稳定的账号域状态。
func accountOperationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAccountChanged):
		fail(w, 409, "账号凭据已变化，请刷新后重新操作")
	case errors.Is(err, errAccountSessionInvalid), errors.Is(err, sql.ErrNoRows):
		fail(w, 401, "登录会话已失效，请重新登录")
	default:
		fail(w, 503, "账号服务暂时不可用，请稍后重试")
	}
}

// verifyAccountPassword 在语法检查后预占共享预算，bcrypt 不持有预算锁或数据库事务。
func (a *App) verifyAccountPassword(w http.ResponseWriter, actor accountMutationActor, password string) bool {
	if len(password) > 72 {
		fail(w, 400, "当前密码不能超过 72 个 UTF-8 字节")
		return false
	}
	if retry := a.accountLimits.reserve(actor.expected.account.user.ID, actor.expected.sessionID, time.Now()); retry > 0 {
		seconds := int(math.Ceil(retry.Seconds()))
		seconds = max(1, min(300, seconds))
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		a.logger.Warn("账号再认证尝试过于频繁", "userId", actor.expected.account.user.ID)
		fail(w, 429, "账号验证过于频繁，请稍后再试")
		return false
	}
	if bcrypt.CompareHashAndPassword([]byte(actor.expected.account.passwordHash), []byte(password)) != nil {
		a.logger.Warn("账号再认证失败", "userId", actor.expected.account.user.ID)
		fail(w, 400, "当前密码不正确")
		return false
	}
	return true
}

// accountPassword 改密成功后撤销全部旧会话，要求使用新密码重新登录。
func (a *App) accountPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	input, ok := decodeAccountJSON(w, r, "currentPassword", "newPassword")
	if !ok {
		return
	}
	if !validNewPassword(input["newPassword"]) {
		fail(w, 400, newPasswordHint)
		return
	}
	if !a.verifyAccountPassword(w, actor, input["currentPassword"]) {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input["newPassword"]), bcrypt.DefaultCost)
	if err != nil {
		fail(w, 500, "密码保护失败")
		return
	}
	if err = changeAccountPassword(r.Context(), a.db, actor, string(hash)); err != nil {
		accountOperationError(w, err)
		return
	}
	a.clearAccountCookie(w, r)
	a.logger.Info("管理员密码已更新并撤销全部会话", "userId", actor.expected.account.user.ID)
	respond(w, 200, map[string]bool{"requiresLogin": true})
}

// validAccountSessionID 校验独立公开编号，禁止接受令牌或大小写别名。
func validAccountSessionID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}

// accountSessionQuery 严格解析有界 keyset 分页，拒绝未知、重复或空参数。
func accountSessionQuery(rawQuery string) (int, string, bool) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return 0, "", false
	}
	limit, after := 20, ""
	for key, entries := range values {
		if len(entries) != 1 || entries[0] == "" {
			return 0, "", false
		}
		switch key {
		case "limit":
			if strings.Trim(entries[0], "0123456789") != "" {
				return 0, "", false
			}
			limit, err = strconv.Atoi(entries[0])
			if err != nil || limit < 1 || limit > 100 {
				return 0, "", false
			}
		case "cursor":
			if !strings.HasPrefix(entries[0], "v1.") || !validAccountSessionID(strings.TrimPrefix(entries[0], "v1.")) {
				return 0, "", false
			}
			after = entries[0][3:]
		default:
			return 0, "", false
		}
	}
	return limit, after, true
}

// accountSessions 只返回一批固定上限原始行及单独固定的真实当前会话。
func (a *App) accountSessions(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	limit, after, ok := accountSessionQuery(r.URL.RawQuery)
	if !ok {
		fail(w, 400, "会话分页参数无效")
		return
	}
	page, err := listAccountSessions(r.Context(), a.db, actor, after, limit)
	if err != nil {
		accountOperationError(w, err)
		return
	}
	respond(w, 200, page)
}

// accountRevokeSession 幂等撤销本人指定会话；退出真实当前会话时清 Cookie。
func (a *App) accountRevokeSession(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	target := r.PathValue("sessionId")
	if !validAccountSessionID(target) {
		fail(w, 400, "会话编号无效")
		return
	}
	requiresLogin, err := revokeAccountSession(r.Context(), a.db, actor, target)
	if err != nil {
		accountOperationError(w, err)
		return
	}
	if requiresLogin {
		a.clearAccountCookie(w, r)
	}
	a.logger.Info("管理员已撤销指定会话", "userId", actor.expected.account.user.ID, "current", requiresLogin)
	respond(w, 200, map[string]bool{"requiresLogin": requiresLogin})
}

// accountRevokeOthers 再认证后只保留真实当前会话，删除数量不代表活跃设备数。
func (a *App) accountRevokeOthers(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	input, ok := decodeAccountJSON(w, r, "currentPassword")
	if !ok || !a.verifyAccountPassword(w, actor, input["currentPassword"]) {
		return
	}
	count, err := revokeOtherAccountSessions(r.Context(), a.db, actor)
	if err != nil {
		accountOperationError(w, err)
		return
	}
	a.logger.Info("管理员已撤销其他会话记录", "userId", actor.expected.account.user.ID, "revokedCount", count)
	respond(w, 200, map[string]any{"requiresLogin": false, "revokedCount": count})
}

// clearAccountCookie 与现有退出保持相同安全属性，只在提交成功后执行。
func (a *App) clearAccountCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "nas_session", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.config.CookieSecure || r.TLS != nil, MaxAge: -1})
}
