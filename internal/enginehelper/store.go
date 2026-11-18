package enginehelper

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"nas-content-manager/internal/enginecontract"
)

// store 独立保存部署意图，绝不复用应用业务数据库或保存密码。
type store struct {
	db           *sql.DB // 只含部署安全元数据的单连接SQLite。
	installation string  // 早于任何Docker副作用持久化的随机安装身份。
}

// randomID 生成密码学随机标识，调用失败时不降级为时间戳。
func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// openStore 在独立SQLite建立单连接短事务状态与幂等绑定。
func openStore(ctx context.Context, path string) (*store, error) {
	return openBoundStore(ctx, path, "")
}

// openBoundStore 在命名卷绑定落盘后打开状态库，崩溃恢复仍复用同一安装身份。
func openBoundStore(ctx context.Context, path string, installation string) (*store, error) {
	info, statErr := os.Lstat(path)
	newDatabase := errors.Is(statErr, os.ErrNotExist) || statErr == nil && info.Mode().IsRegular() && info.Size() == 0
	if !newDatabase && (statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("助手状态库路径不安全")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, errors.New("无法打开助手状态库")
	}
	db.SetMaxOpenConns(1)
	s := &store{db: db}
	if !newDatabase {
		var applicationID, version int
		if db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&applicationID) != nil || db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version) != nil || applicationID != 1312904005 || version != 1 {
			db.Close()
			return nil, errors.New("状态库不属于此助手或版本不兼容，请恢复完整助手备份")
		}
	}
	_, err = db.ExecContext(ctx, `PRAGMA journal_mode=WAL;PRAGMA busy_timeout=5000;PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS tasks(id TEXT PRIMARY KEY,state TEXT NOT NULL,body TEXT NOT NULL,updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS requests(id TEXT PRIMARY KEY,signature TEXT NOT NULL,task_id TEXT NOT NULL REFERENCES tasks(id));
CREATE TABLE IF NOT EXISTS events(task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,sequence INTEGER NOT NULL,body TEXT NOT NULL,PRIMARY KEY(task_id,sequence));
PRAGMA application_id=1312904005;PRAGMA user_version=1;`)
	if err != nil {
		db.Close()
		return nil, errors.New("助手状态库初始化失败")
	}
	err = db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='installation'").Scan(&s.installation)
	if errors.Is(err, sql.ErrNoRows) {
		s.installation = installation
		if installation == "" {
			s.installation, err = randomID()
		} else {
			err = nil
		}
		if err == nil {
			_, err = db.ExecContext(ctx, "INSERT INTO meta(key,value) VALUES('installation',?)", s.installation)
		}
	}
	if err != nil || !validInstallation(s.installation) || installation != "" && s.installation != installation {
		db.Close()
		return nil, errors.New("助手安装身份无效，请恢复完整助手备份")
	}
	return s, nil
}

// getTask 从持久库读取完整安全任务快照。
func (s *store) getTask(ctx context.Context, id string) (enginecontract.Task, error) {
	var body string
	var t enginecontract.Task
	err := s.db.QueryRowContext(ctx, "SELECT body FROM tasks WHERE id=?", id).Scan(&body)
	if err != nil {
		return t, err
	}
	if json.Unmarshal([]byte(body), &t) != nil {
		return t, errors.New("任务快照损坏")
	}
	return t, nil
}

// activeTask 找出唯一未完成执行，网络调用期间不持有数据库事务。
func (s *store) activeTask(ctx context.Context) (*enginecontract.Task, error) {
	var id string
	err := s.db.QueryRowContext(ctx, "SELECT id FROM tasks WHERE state IN ('queued','running') ORDER BY updated LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t, err := s.getTask(ctx, id)
	return &t, err
}

// binding 读取已持久化请求，参数不同返回冲突。
func (s *store) binding(ctx context.Context, id string, signature string) (*enginecontract.Task, error) {
	var old, target string
	err := s.db.QueryRowContext(ctx, "SELECT signature,task_id FROM requests WHERE id=?", id).Scan(&old, &target)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if old != signature {
		return nil, errConflict
	}
	t, err := s.getTask(ctx, target)
	return &t, err
}

// alias 将重试幂等键绑定到同一个持久任务。
func (s *store) alias(ctx context.Context, id string, signature string, target string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO requests(id,signature,task_id) VALUES(?,?,?)", id, signature, target)
	return err
}

// insertTask 在任何Docker副作用前原子保存任务和请求身份。
func (s *store) insertTask(ctx context.Context, id string, signature string, components []string) (enginecontract.Task, error) {
	idTask, err := randomID()
	if err != nil {
		return enginecontract.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	t := enginecontract.Task{ID: idTask, Action: strings.Split(signature, "|")[0], State: "queued", Revision: 1, Components: components, Steps: []enginecontract.Step{}, CreatedAt: now, UpdatedAt: now, Message: "初始化已排队"}
	for _, c := range components {
		t.Steps = append(t.Steps, enginecontract.Step{Component: c, Phase: "preflight", State: "queued", Attempt: 1, Message: "等待环境检查", Layers: []enginecontract.Layer{}})
	}
	body, err := json.Marshal(t)
	if err != nil {
		return t, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return t, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO tasks(id,state,body,updated) VALUES(?,?,?,?)", t.ID, t.State, string(body), now); err != nil {
		return t, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO requests(id,signature,task_id) VALUES(?,?,?)", id, signature, t.ID); err != nil {
		return t, err
	}
	return t, tx.Commit()
}

// saveTask 持久化短状态更新并保留可恢复意图。
func (s *store) saveTask(ctx context.Context, t *enginecontract.Task) error {
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.Revision++
	body, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "UPDATE tasks SET state=?,body=?,updated=? WHERE id=?", t.State, string(body), t.UpdatedAt, t.ID)
	return err
}

// saveRetry 将重试状态和幂等别名放在同一个短事务，避免断线留下无绑定的排队任务。
func (s *store) saveRetry(ctx context.Context, t *enginecontract.Task, requestID string, signature string) error {
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.Revision++
	body, err := json.Marshal(t)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE tasks SET state=?,body=?,updated=? WHERE id=?", t.State, string(body), t.UpdatedAt, t.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO requests(id,signature,task_id) VALUES(?,?,?)", requestID, signature, t.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// event 写入有界受控中文事件，不接收任何上游原始日志。
func (s *store) event(ctx context.Context, id string, component string, level string, message string) error {
	if len([]rune(message)) > 256 {
		return errors.New("安全事件超过预算")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var seq int64
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(sequence),0)+1 FROM events WHERE task_id=?", id).Scan(&seq); err != nil {
		return err
	}
	e := enginecontract.Event{Sequence: seq, Component: component, Level: level, Message: message, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	body, _ := json.Marshal(e)
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(task_id,sequence,body) VALUES(?,?,?)", id, seq, string(body)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM events WHERE task_id=? AND sequence<=?", id, seq-500); err != nil {
		return err
	}
	return tx.Commit()
}

// response 返回完整快照与最多一页事件，明确说明历史游标缺口。
func (s *store) response(ctx context.Context, id string, after int64) (enginecontract.TaskResponse, error) {
	t, err := s.getTask(ctx, id)
	r := enginecontract.TaskResponse{Task: t, Events: []enginecontract.Event{}, NextSequence: after}
	if err != nil {
		return r, err
	}
	var minimum sql.NullInt64
	if err = s.db.QueryRowContext(ctx, "SELECT MIN(sequence) FROM events WHERE task_id=?", id).Scan(&minimum); err != nil {
		return r, err
	}
	r.ReloadRequired = after > 0 && minimum.Valid && after < minimum.Int64-1
	rows, err := s.db.QueryContext(ctx, "SELECT body FROM events WHERE task_id=? AND sequence>? ORDER BY sequence LIMIT 100", id, after)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var body string
		var e enginecontract.Event
		if err = rows.Scan(&body); err != nil {
			return r, err
		}
		if json.Unmarshal([]byte(body), &e) != nil {
			return r, errors.New("助手事件损坏")
		}
		r.Events = append(r.Events, e)
		r.NextSequence = e.Sequence
	}
	return r, rows.Err()
}

// pruneHistory 仅清理最旧的非活跃安全任务元数据，绝不删除引擎或凭据。
func (s *store) pruneHistory(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM requests WHERE task_id IN (SELECT id FROM tasks WHERE state NOT IN ('queued','running') ORDER BY updated DESC LIMIT -1 OFFSET 100); DELETE FROM tasks WHERE id IN (SELECT id FROM tasks WHERE state NOT IN ('queued','running') ORDER BY updated DESC LIMIT -1 OFFSET 100);`)
	return err
}

// taskSignature 绑定调用类型、组件和告知版本，避免同键改变授权范围。
func taskSignature(action string, components []string, version string) string {
	return fmt.Sprintf("%s|%s|%s", action, strings.Join(components, ","), version)
}
