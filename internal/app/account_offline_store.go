package app

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// errOfflineStateUncertain 区分提交或回滚未确认，禁止对外承诺账号未修改。
var errOfflineStateUncertain = errors.New("维护事务结果未确认，请核对账号后再执行，不能自动重试")

// offlineAccountSnapshot 同时固定凭据和账号对象的结构版本，防止预检后的外部改写。
type offlineAccountSnapshot struct {
	account       accountSnapshot // 已有唯一管理员的私有凭据快照。
	schemaVersion int64           // SQLite 结构修订，不是应用迁移版本。
	userVersion   int64           // 预检固定的应用迁移版本，提交时禁止支持集内漂移。
}

// offlineDatabaseFile 拒绝不存在、链接和非普通的数据库文件，不创建任何数据库。
func offlineDatabaseFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("library.db 必须是已有的普通文件，不能为链接")
	}
	return info, nil
}

// offlineDatabaseURI 安全转义固定本地文件名，mode=rw 只能作用于真正的 SQLite URI。
func offlineDatabaseURI(path string) (string, error) {
	name := filepath.ToSlash(path)
	if strings.HasPrefix(name, "//") {
		return "", errors.New("离线维护仅支持本地数据目录")
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	uri := url.URL{Scheme: "file", Path: name, RawQuery: "mode=rw"}
	return uri.String(), nil
}

// supportsOfflineAccountSchema 显式限定已核对账号写入前提的迁移版本，不随最新头自动扩大。
func supportsOfflineAccountSchema(version int64) bool {
	return version == 5 || version == 6
}

// openOfflineAccountDatabase 只打开现有 schema5或6，不执行迁移、任务恢复或缓存处理。
func openOfflineAccountDatabase(ctx context.Context, path string) (*sql.DB, offlineAccountSnapshot, error) {
	var empty offlineAccountSnapshot
	uri, err := offlineDatabaseURI(path)
	if err != nil {
		return nil, empty, err
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, empty, errors.New("已有维护数据库不可打开")
	}
	db.SetMaxOpenConns(1)
	if err = db.PingContext(ctx); err == nil {
		_, err = db.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=1000; PRAGMA trusted_schema=OFF")
	}
	if err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			return nil, empty, errors.New("维护数据库打开和关闭失败")
		}
		if ctx.Err() != nil {
			return nil, empty, ctx.Err()
		}
		return nil, empty, errors.New("已有维护数据库不可读取")
	}
	expected, checkErr := inspectOfflineAccount(ctx, db)
	if checkErr != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, empty, errors.New("维护数据库校验和关闭失败")
		}
		if errors.Is(checkErr, errOfflineStateUncertain) {
			return nil, empty, checkErr
		}
		if ctx.Err() != nil {
			return nil, empty, ctx.Err()
		}
		return nil, empty, checkErr
	}
	return db, expected, nil
}

// inspectOfflineAccount 在一致只读事务内验证支持版本、账号结构和唯一管理员。
func inspectOfflineAccount(ctx context.Context, db *sql.DB) (snapshot offlineAccountSnapshot, resultErr error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return snapshot, errors.New("维护数据库不可检查")
	}
	defer finishOfflineTransaction(tx, &resultErr)
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&snapshot.userVersion); err != nil {
		return snapshot, errors.New("维护数据库版本不可读取")
	}
	if snapshot.userVersion < 5 {
		return snapshot, errors.New("离线维护仅支持 schema5或6，请先用匹配应用正常迁移后停服")
	}
	if !supportsOfflineAccountSchema(snapshot.userVersion) {
		return snapshot, errors.New("数据库版本较新，请使用匹配版本的维护程序")
	}
	if err = validateOfflineAccountStructure(ctx, tx); err != nil {
		return snapshot, errors.New("账号存储结构不受支持，未执行重置")
	}
	if err = tx.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&snapshot.schemaVersion); err != nil {
		return snapshot, errors.New("账号存储结构不可读取")
	}
	snapshot.account, err = readOfflineAccount(ctx, tx)
	if err != nil {
		return snapshot, errors.New("必须存在唯一且有效的 ID=1 管理员，未执行重置")
	}
	if snapshot.account.authRevision == math.MaxInt64 {
		return snapshot, errors.New("账号凭据修订已达上限，未执行重置")
	}
	if err = tx.Commit(); err != nil {
		return snapshot, errors.New("账号预检未完成")
	}
	return snapshot, nil
}

// validateOfflineAccountStructure 只验证将被写入的账号对象，拒绝异常表和额外触发器。
func validateOfflineAccountStructure(ctx context.Context, tx *sql.Tx) error {
	columns := map[string][]string{
		"users":    {"id:INTEGER:0:1", "username:TEXT:1:0", "password_hash:TEXT:1:0", "auth_revision:INTEGER:1:0"},
		"sessions": {"token_hash:TEXT:0:1", "user_id:INTEGER:1:0", "expires_at:TEXT:1:0", "session_id:TEXT:1:0", "created_at:TEXT:0:0", "client_browser:TEXT:1:0", "client_os:TEXT:1:0", "auth_revision:INTEGER:1:0"},
	}
	for table, expected := range columns {
		var kind string
		if err := tx.QueryRowContext(ctx, "SELECT type FROM sqlite_schema WHERE name=?", table).Scan(&kind); err != nil || kind != "table" {
			return errors.New("账号表不存在")
		}
		rows, err := tx.QueryContext(ctx, "SELECT name,upper(type),\"notnull\",pk,hidden FROM pragma_table_xinfo(?) ORDER BY cid", table)
		if err != nil {
			return err
		}
		index := 0
		for rows.Next() {
			var name, kind string
			var nonnull, primary, hidden int
			if err = rows.Scan(&name, &kind, &nonnull, &primary, &hidden); err != nil {
				break
			}
			if index >= len(expected) || hidden != 0 || name+":"+kind+":"+strconv.Itoa(nonnull)+":"+strconv.Itoa(primary) != expected[index] {
				err = errors.New("账号字段不匹配")
				break
			}
			index++
		}
		readErr, closeErr := rows.Err(), rows.Close()
		if err != nil || readErr != nil || closeErr != nil || index != len(expected) {
			return errors.New("账号字段不匹配")
		}
	}
	var foreignKeys int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_list('sessions') WHERE "table"='users' AND "from"='user_id' AND "to"='id' AND on_delete='CASCADE'`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		return errors.New("会话外键不匹配")
	}
	var extraKeys int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM pragma_foreign_key_list('sessions')").Scan(&extraKeys); err != nil || extraKeys != 1 {
		return errors.New("会话外键不匹配")
	}
	rows, err := tx.QueryContext(ctx, "SELECT name,sql FROM sqlite_schema WHERE type='trigger' AND tbl_name IN ('users','sessions')")
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		var name, statement string
		if err = rows.Scan(&name, &statement); err != nil {
			break
		}
		// 只接受本应用 schema5 原有的两个非删除触发器，不执行外来维护副作用。
		if (name != "sessions_public_id_insert" && name != "sessions_public_id_update") || !strings.Contains(schemaV5SessionConstraints, statement+";") {
			err = errors.New("存在未知账号触发器")
			break
		}
		count++
	}
	readErr, closeErr := rows.Err(), rows.Close()
	if err != nil || readErr != nil || closeErr != nil || count != 2 {
		return errors.New("账号触发器不匹配")
	}
	// 禁止其他表依赖 sessions 的删除而级联修改无关业务记录。
	var references int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema AS s JOIN pragma_foreign_key_list(s.name) AS f WHERE s.type='table' AND f."table"='sessions'`).Scan(&references)
	if err != nil || references != 0 {
		return errors.New("存在未知会话引用")
	}
	// 其他外来外键也不能通过密码或修订更新级联写入无关业务表。
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema AS s JOIN pragma_foreign_key_list(s.name) AS f WHERE s.type='table' AND f."table"='users' AND f."to" NOT IN ('id','username','')`).Scan(&references)
	if err != nil || references != 0 {
		return errors.New("存在未知账号引用")
	}
	return nil
}

// readOfflineAccount 有界读取单管理员，拒绝弱类型伪装、空值和异常账号编号。
func readOfflineAccount(ctx context.Context, tx *sql.Tx) (accountSnapshot, error) {
	var account accountSnapshot
	rows, err := tx.QueryContext(ctx, "SELECT id,username,password_hash,auth_revision,typeof(id),typeof(username),typeof(password_hash),typeof(auth_revision) FROM users LIMIT 2")
	if err != nil {
		return account, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var idType, usernameType, hashType, revisionType string
		if err = rows.Scan(&account.user.ID, &account.user.Username, &account.passwordHash, &account.authRevision, &idType, &usernameType, &hashType, &revisionType); err != nil {
			return account, err
		}
		count++
		if count != 1 || account.user.ID != 1 || account.user.Username == "" || account.authRevision < 0 || idType != "integer" || usernameType != "text" || hashType != "text" || revisionType != "integer" {
			return account, errors.New("管理员记录不符合预期")
		}
		if !validOfflinePasswordHash(account.passwordHash) {
			return account, errors.New("管理员摘要不符合预期")
		}
	}
	if err = rows.Err(); err != nil || count != 1 {
		return account, errors.New("管理员记录不可读取")
	}
	if err = rows.Close(); err != nil {
		return account, err
	}
	return account, nil
}

// validOfflinePasswordHash 同时检查完整 bcrypt 形状与成本，不能仅凭 Cost 的宽松解析放行损坏摘要。
func validOfflinePasswordHash(hash string) bool {
	payload := 7
	switch {
	case len(hash) == 60 && (strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$")) && hash[6] == '$':
	case len(hash) == 59 && strings.HasPrefix(hash, "$2$") && hash[5] == '$':
		payload = 6
	default:
		return false
	}
	if hash[payload-3] < '0' || hash[payload-3] > '9' || hash[payload-2] < '0' || hash[payload-2] > '9' {
		return false
	}
	// salt 与 digest 共 53 个 bcrypt-base64 字符，不接受尾缀或其他字符。
	for _, value := range hash[payload:] {
		if !strings.ContainsRune("./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", value) {
			return false
		}
	}
	_, err := bcrypt.Cost([]byte(hash))
	return err == nil
}

// resetOfflineAccountPassword 独立 CAS 短事务更换现有凭据并撤销会话，不伪造在线身份。
func resetOfflineAccountPassword(ctx context.Context, db *sql.DB, expected offlineAccountSnapshot, newHash string) (resultErr error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer finishOfflineTransaction(tx, &resultErr)
	var version, schemaVersion int64
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&schemaVersion); err != nil {
		return err
	}
	actual, err := readOfflineAccount(ctx, tx)
	if err != nil {
		return err
	}
	// 即使新旧版本均受支持，也不能跨越预检后发生的应用迁移版本变更。
	if !supportsOfflineAccountSchema(version) || version != expected.userVersion || schemaVersion != expected.schemaVersion || actual.user.ID != expected.account.user.ID || actual.passwordHash != expected.account.passwordHash || actual.authRevision != expected.account.authRevision || actual.authRevision == math.MaxInt64 {
		return errAccountChanged
	}
	result, err := tx.ExecContext(ctx, "UPDATE users SET password_hash=?,auth_revision=auth_revision+1 WHERE id=? AND password_hash=? AND auth_revision=?", newHash, actual.user.ID, actual.passwordHash, actual.authRevision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return errAccountChanged
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", actual.user.ID); err != nil {
		return err
	}
	// 提交前最后检查取消；成功提交后不再把随后信号解释为回滚。
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return errOfflineStateUncertain
	}
	return nil
}

// finishOfflineTransaction 保留真正回滚失败；已提交或上下文已完成的事务不重复关闭。
func finishOfflineTransaction(tx *sql.Tx, resultErr *error) {
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		*resultErr = errors.Join(*resultErr, errOfflineStateUncertain)
	}
}
