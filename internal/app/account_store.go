package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"time"
)

// errAccountChanged 表示密码校验后账号快照已经变化，不能继续创建会话。
var errAccountChanged = errors.New("账号凭据已更新，请重新登录")

// errAccountSessionInvalid 区分仍为同一账号快照时的会话撤销或到期。
var errAccountSessionInvalid = errors.New("登录会话已失效")

// accountMutationActor 固定实际 Cookie 与进入请求时的账号快照，不接受客户端指定身份。
type accountMutationActor struct {
	tokenDigest string                      // 私有 Cookie 摘要，禁止输出。
	expected    authenticatedAccountSession // bcrypt 及敏感写入所依据的快照。
}

// AccountSession 只公开独立编号及有限客户端提示，不公开令牌、摘要、IP 或原始 UA。
type AccountSession struct {
	ID            string  `json:"id"`            // 独立随机公开会话编号，不是 bearer 凭据。
	Current       bool    `json:"current"`       // 是否为实际 Cookie 所对应的当前会话。
	ClientBrowser string  `json:"clientBrowser"` // 有限浏览器类别，仅作提示。
	ClientOS      string  `json:"clientOS"`      // 有限系统类别，仅作提示。
	CreatedAt     *string `json:"createdAt"`     // RFC3339Nano 创建时间，历史未知值为 null。
	ExpiresAt     string  `json:"expiresAt"`     // 精确到期时间。
}

// AccountSessionPage 是一批原始行过滤后的有界结果，空 items 仍可能允许继续。
type AccountSessionPage struct {
	CurrentSession AccountSession   `json:"currentSession"` // 来自真实 Cookie 的固定当前会话。
	Items          []AccountSession `json:"items"`          // 本批其他有效会话，不无限补齐。
	NextCursor     *string          `json:"nextCursor"`     // 最后消费的原始编号，末批为 null。
}

// validateAccountMutation 先核对账号快照，再核对实际会话，固定竞态下的错误优先级。
func validateAccountMutation(ctx context.Context, tx *sql.Tx, actor accountMutationActor, timestamp time.Time) (authenticatedAccountSession, error) {
	var actual accountSnapshot
	err := tx.QueryRowContext(ctx, "SELECT id,username,password_hash,auth_revision FROM users WHERE id=?", actor.expected.account.user.ID).
		Scan(&actual.user.ID, &actual.user.Username, &actual.passwordHash, &actual.authRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return authenticatedAccountSession{}, errAccountChanged
	}
	if err != nil {
		return authenticatedAccountSession{}, err
	}
	if actual.user.ID != actor.expected.account.user.ID || actual.passwordHash != actor.expected.account.passwordHash || actual.authRevision != actor.expected.account.authRevision {
		return authenticatedAccountSession{}, errAccountChanged
	}
	session, err := readAccountSession(ctx, tx, actor.tokenDigest, timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return authenticatedAccountSession{}, errAccountSessionInvalid
	}
	if err != nil {
		return authenticatedAccountSession{}, err
	}
	if session.sessionID != actor.expected.sessionID {
		return authenticatedAccountSession{}, errAccountSessionInvalid
	}
	return session, nil
}

// validateAccountCommit 在提交前再次拒绝取消与到期；自身删除当前行后仍核验已锁定的到期值。
func validateAccountCommit(ctx context.Context, session authenticatedAccountSession, timestamp time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !session.expiresAt.After(timestamp) {
		return errAccountSessionInvalid
	}
	return nil
}

// changeAccountPassword 用短事务条件更新凭据并撤销全部会话，bcrypt 必须在调用前完成。
func changeAccountPassword(ctx context.Context, db *sql.DB, actor accountMutationActor, newPasswordHash string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	session, err := validateAccountMutation(ctx, tx, actor, time.Now())
	if err != nil {
		return err
	}
	if session.account.authRevision == math.MaxInt64 {
		return errAccountChanged
	}
	result, err := tx.ExecContext(ctx, "UPDATE users SET password_hash=?,auth_revision=auth_revision+1 WHERE id=? AND password_hash=? AND auth_revision=?", newPasswordHash, session.account.user.ID, session.account.passwordHash, session.account.authRevision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errAccountChanged
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", session.account.user.ID); err != nil {
		return err
	}
	if err = validateAccountCommit(ctx, session, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

// revokeAccountSession 在本人范围内幂等删除指定编号，并报告是否退出真实当前会话。
func revokeAccountSession(ctx context.Context, db *sql.DB, actor accountMutationActor, targetSessionID string) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	session, err := validateAccountMutation(ctx, tx, actor, time.Now())
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=? AND session_id=?", session.account.user.ID, targetSessionID); err != nil {
		return false, err
	}
	if err = validateAccountCommit(ctx, session, time.Now()); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return targetSessionID == session.sessionID, nil
}

// revokeOtherAccountSessions 删除当前管理员的其他全部记录，包含过期及旧修订记录。
func revokeOtherAccountSessions(ctx context.Context, db *sql.DB, actor accountMutationActor) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	session, err := validateAccountMutation(ctx, tx, actor, time.Now())
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=? AND session_id<>?", session.account.user.ID, session.sessionID)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err = validateAccountCommit(ctx, session, time.Now()); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

// scanPublicAccountSession 转换一条有限元数据，错误或未知时间不会伪造为有效会话。
func scanPublicAccountSession(id string, created sql.NullString, browser, operatingSystem, expiry string) (AccountSession, time.Time, error) {
	expiresAt, err := time.Parse(time.RFC3339Nano, expiry)
	if err != nil {
		return AccountSession{}, time.Time{}, err
	}
	session := AccountSession{ID: id, ClientBrowser: browser, ClientOS: operatingSystem, ExpiresAt: expiresAt.UTC().Format(time.RFC3339Nano)}
	if created.Valid {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, created.String); parseErr == nil {
			formatted := parsed.UTC().Format(time.RFC3339Nano)
			session.CreatedAt = &formatted
		}
	}
	return session, expiresAt, nil
}

// listAccountSessions 按公开随机编号 keyset 查询至多 limit+1 行，不为补满结果而扫描其他行。
func listAccountSessions(ctx context.Context, db *sql.DB, actor accountMutationActor, afterID string, limit int) (AccountSessionPage, error) {
	if limit < 1 || limit > 100 {
		return AccountSessionPage{}, errors.New("会话分页大小无效")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return AccountSessionPage{}, err
	}
	defer tx.Rollback()
	asOf := time.Now()
	current, err := validateAccountMutation(ctx, tx, actor, asOf)
	if err != nil {
		return AccountSessionPage{}, err
	}
	var created sql.NullString
	var browser, operatingSystem, expiry string
	err = tx.QueryRowContext(ctx, "SELECT created_at,client_browser,client_os,expires_at FROM sessions WHERE user_id=? AND session_id=?", current.account.user.ID, current.sessionID).Scan(&created, &browser, &operatingSystem, &expiry)
	if err != nil {
		return AccountSessionPage{}, err
	}
	pinned, _, err := scanPublicAccountSession(current.sessionID, created, browser, operatingSystem, expiry)
	if err != nil {
		return AccountSessionPage{}, errAccountSessionInvalid
	}
	pinned.Current = true
	page := AccountSessionPage{CurrentSession: pinned, Items: make([]AccountSession, 0, limit)}
	rows, err := tx.QueryContext(ctx, `SELECT session_id,created_at,client_browser,client_os,expires_at,auth_revision
		FROM sessions WHERE user_id=? AND session_id>? ORDER BY session_id ASC LIMIT ?`, current.account.user.ID, afterID, limit+1)
	if err != nil {
		return AccountSessionPage{}, err
	}
	consumed, lastID := 0, ""
	for rows.Next() {
		var id string
		var revision int64
		if err = rows.Scan(&id, &created, &browser, &operatingSystem, &expiry, &revision); err != nil {
			rows.Close()
			return AccountSessionPage{}, err
		}
		if consumed == limit {
			cursor := "v1." + lastID
			page.NextCursor = &cursor
			break
		}
		consumed++
		lastID = id
		public, expiration, parseErr := scanPublicAccountSession(id, created, browser, operatingSystem, expiry)
		if parseErr == nil && expiration.After(asOf) && revision == current.account.authRevision && id != current.sessionID {
			page.Items = append(page.Items, public)
		}
	}
	readErr := rows.Err()
	closeErr := rows.Close()
	if readErr != nil {
		return AccountSessionPage{}, readErr
	}
	if closeErr != nil {
		return AccountSessionPage{}, closeErr
	}
	if err = validateAccountCommit(ctx, current, time.Now()); err != nil {
		return AccountSessionPage{}, err
	}
	if err = tx.Commit(); err != nil {
		return AccountSessionPage{}, err
	}
	return page, nil
}

// accountSnapshot 只在后端保存 bcrypt 校验所依据的完整账号快照，禁止作为公开 DTO。
type accountSnapshot struct {
	user         User   // 可公开的管理员基本信息。
	passwordHash string // 当前密码摘要，仅用于校验和条件写入，不记录日志。
	authRevision int64  // 非负凭据修订，用于拒绝过时的认证结果。
}

// storedAccountSession 保存新建会话所需的私有字段，不保存原始 Cookie 或 UA。
type storedAccountSession struct {
	tokenHash     string    // 独立 bearer 令牌的摘要，不能返回给客户端。
	sessionID     string    // 独立随机公开编号，本身不具备鉴权能力。
	createdAt     time.Time // 本次会话实际创建时间。
	expiresAt     time.Time // 七天会话有效期的精确截止时间。
	clientBrowser string    // 有限浏览器类别，不包含版本或原始 UA。
	clientOS      string    // 有限系统类别，不包含 IP 或原始 UA。
}

// authenticatedAccountSession 是精确核验有效期与修订后的私有认证结果。
type authenticatedAccountSession struct {
	account   accountSnapshot // 当前账号快照，可供敏感操作在事务内再次核对。
	sessionID string          // 此次认证对应的实际公开会话编号。
	expiresAt time.Time       // 已解析的精确到期时间。
}

// accountRowReader 允许普通读取和敏感短事务共用同一认证查询。
type accountRowReader interface {
	// QueryRowContext 在给定上下文内读取一条账号或会话记录。
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// newSessionID 为公开会话编号独立生成 128 位随机值，不从令牌或摘要推导。
func newSessionID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

// migrateAccountSchema 增量添加账号修订和会话元数据，保留原密码与全部既有会话。
func migrateAccountSchema(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, schemaV5); err != nil {
		return err
	}
	for {
		var tokenDigest string
		err := tx.QueryRowContext(ctx, "SELECT token_hash FROM sessions WHERE session_id='' LIMIT 1").Scan(&tokenDigest)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		publicID, err := newSessionID()
		if err != nil {
			return err
		}
		// 老会话的原令牌摘要、有效期不变；未知创建时间和客户端不作推断。
		if _, err = tx.ExecContext(ctx, "UPDATE sessions SET session_id=?,auth_revision=(SELECT auth_revision FROM users WHERE id=sessions.user_id) WHERE token_hash=?", publicID, tokenDigest); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, schemaV5SessionConstraints)
	return err
}

// saveAccountSession 在单条条件写入中核对密码和修订，拒绝 bcrypt 校验后的旧快照。
func saveAccountSession(ctx context.Context, db *sql.DB, account accountSnapshot, session storedAccountSession) error {
	result, err := db.ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at,session_id,created_at,client_browser,client_os,auth_revision)
		SELECT ?,id,?,?,?,?,?,auth_revision FROM users WHERE id=? AND password_hash=? AND auth_revision=?`,
		session.tokenHash, session.expiresAt.UTC().Format(time.RFC3339Nano), session.sessionID, session.createdAt.UTC().Format(time.RFC3339Nano), session.clientBrowser, session.clientOS,
		account.user.ID, account.passwordHash, account.authRevision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errAccountChanged
	}
	return nil
}

// readAccountSession 按真实时间精确核验会话，避免 RFC3339Nano 可变小数的字符串误序。
func readAccountSession(ctx context.Context, reader accountRowReader, tokenDigest string, timestamp time.Time) (authenticatedAccountSession, error) {
	var session authenticatedAccountSession
	var expiresAt string
	err := reader.QueryRowContext(ctx, `SELECT u.id,u.username,u.password_hash,u.auth_revision,s.session_id,s.expires_at
		FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=? AND s.auth_revision=u.auth_revision`, tokenDigest).
		Scan(&session.account.user.ID, &session.account.user.Username, &session.account.passwordHash, &session.account.authRevision, &session.sessionID, &expiresAt)
	if err != nil {
		return authenticatedAccountSession{}, err
	}
	session.expiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	// 到期瞬间即失效；坏时间不能退回字符串比较，也不能取得有效身份。
	if err != nil || !session.expiresAt.After(timestamp) {
		return authenticatedAccountSession{}, sql.ErrNoRows
	}
	return session, nil
}

// deleteExpiredAccountSessions 只清理前一日以前的会话，不按可变小数时间字符串判定精确到期。
func deleteExpiredAccountSessions(ctx context.Context, db *sql.DB, timestamp time.Time) error {
	_, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<?", timestamp.UTC().Add(-24*time.Hour).Format(time.DateOnly))
	return err
}

// schemaV5 只增量增加账号字段；临时普通索引让旧会话逐条补号保持有界内存。
const schemaV5 = `
-- 管理员凭据修订，保留旧密码并以零作为历史起点。
ALTER TABLE users ADD COLUMN auth_revision INTEGER NOT NULL DEFAULT 0 CHECK(auth_revision>=0);
-- 独立公开会话编号，空值仅供迁移补号阶段使用。
ALTER TABLE sessions ADD COLUMN session_id TEXT NOT NULL DEFAULT '' CHECK(session_id='' OR (length(session_id)=32 AND session_id NOT GLOB '*[^0-9a-f]*'));
-- 旧会话真实创建时间未知，因此保留 NULL。
ALTER TABLE sessions ADD COLUMN created_at TEXT;
-- 浏览器和系统只保存有限类别，历史会话保持 unknown。
ALTER TABLE sessions ADD COLUMN client_browser TEXT NOT NULL DEFAULT 'unknown' CHECK(client_browser IN ('unknown','chrome','edge','firefox','safari','opera','other'));
ALTER TABLE sessions ADD COLUMN client_os TEXT NOT NULL DEFAULT 'unknown' CHECK(client_os IN ('unknown','windows','macos','linux','android','ios','other'));
-- 会话绑定创建时的账号凭据修订。
ALTER TABLE sessions ADD COLUMN auth_revision INTEGER NOT NULL DEFAULT 0 CHECK(auth_revision>=0);
CREATE INDEX idx_sessions_public_id ON sessions(session_id);
`

// schemaV5SessionConstraints 在旧行补号完成后强制公开编号必填、唯一且不能退回迁移空值。
const schemaV5SessionConstraints = `
DROP INDEX idx_sessions_public_id;
CREATE UNIQUE INDEX idx_sessions_public_id ON sessions(session_id);
CREATE INDEX idx_sessions_user_public ON sessions(user_id,session_id);
CREATE TRIGGER sessions_public_id_insert BEFORE INSERT ON sessions WHEN NEW.session_id='' BEGIN SELECT RAISE(ABORT,'会话公开编号不能为空'); END;
CREATE TRIGGER sessions_public_id_update BEFORE UPDATE OF session_id ON sessions WHEN NEW.session_id='' BEGIN SELECT RAISE(ABORT,'会话公开编号不能为空'); END;
`
