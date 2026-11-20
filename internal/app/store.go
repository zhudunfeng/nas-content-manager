package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// openDatabase 打开本地 SQLite，依次应用版本化迁移并恢复中断任务。
func openDatabase(ctx context.Context, dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "cache"), 0700); err != nil {
		return nil, err
	}
	// 上次进程被强制终止可能留下有限临时封面，启动时只清理本应用临时命名。
	if entries, readErr := os.ReadDir(filepath.Join(dataDir, "cache")); readErr == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".pending-") {
				_ = os.Remove(filepath.Join(dataDir, "cache", entry.Name()))
			}
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "library.db"))
	if err != nil {
		return nil, err
	}
	// 单写连接避免 SQLite 忙锁，短事务不覆盖文件扫描或媒体处理。
	db.SetMaxOpenConns(1)
	for _, statement := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	var version int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version > 4 {
		db.Close()
		return nil, fmt.Errorf("数据库版本较新，请使用匹配的应用版本")
	}
	if version == 0 {
		tx, txErr := db.BeginTx(ctx, nil)
		if txErr != nil {
			db.Close()
			return nil, txErr
		}
		_, err = tx.ExecContext(ctx, schemaV1)
		if err == nil {
			_, err = tx.ExecContext(ctx, "PRAGMA user_version=1")
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("初始化数据库失败: %w", err)
		}
		version = 1
	}
	if version == 1 {
		// 下载表采用独立增量迁移，原媒体、进度及扫描唯一索引不重建。
		tx, txErr := db.BeginTx(ctx, nil)
		if txErr != nil {
			db.Close()
			return nil, txErr
		}
		_, err = tx.ExecContext(ctx, schemaV2)
		if err == nil {
			_, err = tx.ExecContext(ctx, "PRAGMA user_version=2")
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("升级下载数据库失败: %w", err)
		}
	}
	if version < 3 {
		tx, txErr := db.BeginTx(ctx, nil)
		if txErr != nil {
			db.Close()
			return nil, txErr
		}
		_, err = tx.ExecContext(ctx, schemaV3)
		if err == nil {
			_, err = tx.ExecContext(ctx, "PRAGMA user_version=3")
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("升级资源收集数据库失败: %w", err)
		}
	}
	if version < 4 {
		tx, txErr := db.BeginTx(ctx, nil)
		if txErr != nil {
			db.Close()
			return nil, txErr
		}
		_, err = tx.ExecContext(ctx, schemaV4)
		if err == nil {
			_, err = tx.ExecContext(ctx, "PRAGMA user_version=4")
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("升级外部服务数据库失败: %w", err)
		}
	}
	// 写请求在进程中断后只能读取对账，绝不再次自动提交。
	_, err = db.ExecContext(ctx, "UPDATE collection_external_actions SET state='uncertain',message='应用已重启，外部提交结果待核对',updated_at=? WHERE state='dispatching'", now())
	if err != nil {
		db.Close()
		return nil, err
	}
	_, err = db.ExecContext(ctx, "UPDATE jobs SET status='queued',message='应用重启，等待恢复',updated_at=? WHERE status='running'", now())
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// now 返回统一的 UTC ISO8601 时间。
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// schemaV1 是首版数据库迁移；外键级联仅删除索引，不影响原媒体。
const schemaV1 = `
CREATE TABLE users (id INTEGER PRIMARY KEY CHECK(id=1), username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL);
CREATE TABLE sessions (token_hash TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, expires_at TEXT NOT NULL);
CREATE TABLE libraries (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, path TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'ready', last_scanned_at TEXT NOT NULL DEFAULT '', UNIQUE(type,path));
CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE, title TEXT NOT NULL, type TEXT NOT NULL, relative_path TEXT NOT NULL, size INTEGER NOT NULL, mime TEXT NOT NULL, added_at TEXT NOT NULL, updated_at TEXT NOT NULL, mod_time INTEGER NOT NULL, missing INTEGER NOT NULL DEFAULT 0, duration REAL NOT NULL DEFAULT 0, width INTEGER NOT NULL DEFAULT 0, height INTEGER NOT NULL DEFAULT 0, is_dir INTEGER NOT NULL DEFAULT 0, scan_token TEXT NOT NULL DEFAULT '', enrich_state TEXT NOT NULL DEFAULT 'pending', UNIQUE(library_id,relative_path));
CREATE INDEX idx_items_type ON items(type,added_at DESC);
CREATE TABLE user_items (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE, favorite INTEGER NOT NULL DEFAULT 0, tags TEXT NOT NULL DEFAULT '[]', PRIMARY KEY(user_id,item_id));
CREATE TABLE progress (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE, position REAL NOT NULL, total REAL NOT NULL, locator TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL, PRIMARY KEY(user_id,item_id));
CREATE TABLE jobs (id INTEGER PRIMARY KEY AUTOINCREMENT, library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE, item_id INTEGER REFERENCES items(id) ON DELETE CASCADE, kind TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'queued', processed INTEGER NOT NULL DEFAULT 0, message TEXT NOT NULL DEFAULT '等待处理', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE INDEX idx_jobs_status ON jobs(status,id);
CREATE UNIQUE INDEX idx_scan_active ON jobs(library_id) WHERE kind='scan' AND status IN ('queued','running');
CREATE UNIQUE INDEX idx_enrich_active ON jobs(item_id) WHERE kind='enrich' AND status IN ('queued','running');
`
