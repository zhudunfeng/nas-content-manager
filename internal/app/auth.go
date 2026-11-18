package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// newPasswordHint 说明首次创建管理员时的密码长度与字符类别要求。
const newPasswordHint = "密码需为 10–72 位，同时包含大写字母、小写字母、数字和英文特殊字符（如 !@#$%），不允许中文、空格或表情"

// credentials 定义登录和首次设置的输入。
type credentials struct {
	Username string `json:"username"` // 长度为 2–40 字符的登录名。
	Password string `json:"password"` // 新密码仅允许 10–72 位 ASCII 英文字母、数字和标点且包含四类；登录保留原密码匹配，不记录日志。
}

// validNewPassword 校验新密码长度及四类 ASCII 字符，拒绝中文、空白和其他非允许字符。
func validNewPassword(password string) bool {
	if len(password) < 10 || len(password) > 72 || !utf8.ValidString(password) {
		return false
	}
	var upper, lower, digit, special bool
	for _, character := range password {
		switch {
		case character >= 'A' && character <= 'Z':
			upper = true
		case character >= 'a' && character <= 'z':
			lower = true
		case character >= '0' && character <= '9':
			digit = true
		case character >= '!' && character <= '/' || character >= ':' && character <= '@' || character >= '[' && character <= '`' || character >= '{' && character <= '~':
			special = true
		default:
			// 白名单以外的字符直接拒绝，不删除或替换用户输入。
			return false
		}
	}
	return upper && lower && digit && special
}

// status 返回首次设置状态与当前会话。
func (a *App) status(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil {
		fail(w, 503, "无法读取账号状态")
		return
	}
	respond(w, 200, map[string]any{"setupRequired": count == 0, "user": a.sessionUser(r), "version": Version})
}

// setup 原子创建唯一管理员，避免并发首次设置覆盖已有账号。
func (a *App) setup(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	if utf8.RuneCountInString(input.Username) < 2 || utf8.RuneCountInString(input.Username) > 40 {
		fail(w, 400, "用户名需为 2–40 字")
		return
	}
	// 复杂度只约束新建密码，不改变已有账户的登录与原始密码内容。
	if !validNewPassword(input.Password) {
		a.logger.Warn("首次管理员设置未通过密码规则校验")
		fail(w, 400, newPasswordHint)
		return
	}
	var count int
	if a.db.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&count) != nil {
		fail(w, 503, "账号状态不可用")
		return
	}
	if count > 0 {
		fail(w, 409, "管理员已创建，请登录")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(w, 500, "密码保护失败")
		return
	}
	if _, err = a.db.ExecContext(r.Context(), "INSERT INTO users(id,username,password_hash) VALUES(1,?,?)", input.Username, string(hash)); err != nil {
		fail(w, 409, "管理员已创建，请登录")
		return
	}
	user := &User{ID: 1, Username: input.Username}
	if !a.createSession(w, r, user.ID) {
		return
	}
	a.logger.Info("首次管理员设置完成", "userId", user.ID)
	respond(w, 201, map[string]any{"user": user})
}

// login 校验管理员凭据并创建不可猜测的持久会话。
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !a.allowLogin(ip) {
		w.Header().Set("Retry-After", "300")
		fail(w, 429, "登录尝试过于频繁，请五分钟后再试")
		return
	}
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	var user User
	var hash string
	err = a.db.QueryRowContext(r.Context(), "SELECT id,username,password_hash FROM users WHERE username=?", strings.TrimSpace(input.Username)).Scan(&user.ID, &user.Username, &hash)
	if err != nil || len(input.Password) > 72 || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		a.logger.Warn("管理员登录验证失败")
		fail(w, 401, "用户名或密码不正确")
		return
	}
	if !a.createSession(w, r, user.ID) {
		return
	}
	a.mu.Lock()
	delete(a.loginAttempts, ip)
	a.mu.Unlock()
	a.logger.Info("管理员登录成功", "userId", user.ID)
	respond(w, 200, map[string]any{"user": user})
}

// allowLogin 在有限内存中限制来源的登录频率。
func (a *App) allowLogin(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	timestamp := time.Now()
	if len(a.loginAttempts) > 2048 {
		for key, value := range a.loginAttempts {
			if timestamp.After(value.Until) {
				delete(a.loginAttempts, key)
			}
		}
		if len(a.loginAttempts) > 2048 {
			return false
		}
	}
	entry := a.loginAttempts[ip]
	if timestamp.After(entry.Until) {
		entry = loginAttempt{Until: timestamp.Add(5 * time.Minute)}
	}
	entry.Count++
	a.loginAttempts[ip] = entry
	return entry.Count <= 12
}

// tokenHash 使用 SHA256 保存会话摘要，数据库不保存原始令牌。
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// createSession 创建有效期七天的随机会话，并清除过期会话。
func (a *App) createSession(w http.ResponseWriter, r *http.Request, userID int64) bool {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		fail(w, 500, "无法创建登录会话")
		return false
	}
	token := hex.EncodeToString(data)
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	_, _ = a.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE expires_at<?", now())
	if _, err := a.db.ExecContext(r.Context(), "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)", tokenHash(token), userID, expires.Format(time.RFC3339Nano)); err != nil {
		fail(w, 500, "无法保存登录会话")
		return false
	}
	http.SetCookie(w, &http.Cookie{Name: "nas_session", Value: token, Path: "/", HttpOnly: true, Secure: a.config.CookieSecure || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 7 * 24 * 3600, Expires: expires})
	return true
}

// sessionUser 根据 HttpOnly Cookie 查询仍有效的管理员会话。
func (a *App) sessionUser(r *http.Request) *User {
	cookie, err := r.Cookie("nas_session")
	if err != nil || len(cookie.Value) != 64 {
		return nil
	}
	var user User
	err = a.db.QueryRowContext(r.Context(), "SELECT u.id,u.username FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=? AND s.expires_at>?", tokenHash(cookie.Value), now()).Scan(&user.ID, &user.Username)
	if err != nil {
		return nil
	}
	return &user
}

// logout 立即撤销当前持久会话并清除浏览器 Cookie。
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("nas_session"); err == nil {
		if _, err = a.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=?", tokenHash(cookie.Value)); err != nil {
			fail(w, 500, "退出登录失败，请重试")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "nas_session", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.config.CookieSecure || r.TLS != nil, MaxAge: -1})
	respond(w, 200, map[string]bool{"ok": true})
}
