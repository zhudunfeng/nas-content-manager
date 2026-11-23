package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// snapshotMetadata 保存检查实际所得的非秘密事实，不代表整栈恢复或可信来源。
type snapshotMetadata struct {
	SchemaVersion int       // 从候选数据库实际读取的头版本。
	SizeBytes     int64     // 页检查与流式读取共同确认的字节数。
	SHA256        string    // 同一只读文件句柄计算的小写十六进制摘要。
	CreatedAt     time.Time // UTC生成完成时间，不冒充精确事务时点。
	Scope         string    // 固定database-only，排除外部秘密、卷和媒体原件。
}

// snapshotExpected 是调用方已有的字节身份；nil仅计算事实，不声称可信清单已核对。
type snapshotExpected struct {
	SizeBytes int64             // 必须为正的预期文件长度。
	SHA256    [sha256.Size]byte // 已解码且固定算法的预期摘要。
}

// snapshotReadOnlyURI 正确转义本地绝对路径，禁止参数注入和隐式创建数据库。
func snapshotReadOnlyURI(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("备份校验仅接受本地绝对路径")
	}
	name := filepath.ToSlash(path)
	if strings.HasPrefix(name, "//") {
		return "", errors.New("备份校验仅支持本地文件")
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	uri := url.URL{Scheme: "file", Path: name, RawQuery: "mode=ro&immutable=1"}
	return uri.String(), nil
}

// stableSnapshotFile 确认路径和句柄始终对应同一未变化普通文件，不跟随链接。
func stableSnapshotFile(path string, file *os.File, expected os.FileInfo) (os.FileInfo, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.Mode().IsRegular() || isSnapshotReparsePoint(pathInfo) {
		return nil, errors.New("备份文件必须为非链接普通文件")
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(pathInfo, opened) || pathInfo.Size() != opened.Size() || !pathInfo.ModTime().Equal(opened.ModTime()) {
		return nil, errors.New("备份路径与句柄身份不一致")
	}
	if expected != nil && (!os.SameFile(expected, opened) || expected.Size() != opened.Size() || !expected.ModTime().Equal(opened.ModTime())) {
		return nil, errors.New("备份文件校验期间发生变化")
	}
	return opened, nil
}

// verifyDatabaseSnapshot 只读校验已完成且隔离稳定的单文件候选，绝不调用正常启动迁移。
func verifyDatabaseSnapshot(ctx context.Context, path string, file *os.File, expected *snapshotExpected) (metadata snapshotMetadata, resultErr error) {
	if err := ctx.Err(); err != nil {
		return metadata, err
	}
	info, err := stableSnapshotFile(path, file, nil)
	if err != nil {
		return metadata, err
	}
	if info.Size() < 100 || (expected != nil && (expected.SizeBytes <= 0 || expected.SizeBytes != info.Size())) {
		return metadata, errors.New("备份文件长度不符合预期")
	}
	var header [16]byte
	if _, err = file.ReadAt(header[:], 0); err != nil || !bytes.Equal(header[:], []byte("SQLite format 3\x00")) {
		return metadata, errors.New("备份文件不是SQLite3数据库")
	}
	uri, err := snapshotReadOnlyURI(path)
	if err != nil {
		return metadata, err
	}
	metadata.SchemaVersion, err = inspectSnapshotDatabase(ctx, uri, path, file, info)
	if err != nil {
		return metadata, err
	}
	if _, err = stableSnapshotFile(path, file, info); err != nil {
		return metadata, err
	}
	// SQL资源全部关闭后，才用将要交付HTTP的同一只读句柄计算摘要。
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return metadata, errors.New("备份文件不可定位")
	}
	digest, err := hashSnapshotFile(ctx, file, info.Size())
	if err != nil {
		return metadata, err
	}
	if expected != nil && digest != expected.SHA256 {
		return metadata, errors.New("备份摘要不符合预期")
	}
	if _, err = stableSnapshotFile(path, file, info); err != nil {
		return metadata, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return metadata, errors.New("备份文件不可重置")
	}
	if err = ctx.Err(); err != nil {
		return metadata, err
	}
	metadata.SizeBytes = info.Size()
	metadata.SHA256 = hex.EncodeToString(digest[:])
	metadata.Scope = "database-only"
	return metadata, nil
}

// inspectSnapshotDatabase 固定单条只读连接检查所有SQL，关闭错误也必须失败。
func inspectSnapshotDatabase(ctx context.Context, uri string, path string, file *os.File, info os.FileInfo) (version int, resultErr error) {
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return 0, errors.New("备份校验数据库不可打开")
	}
	db.SetMaxOpenConns(1)
	defer func() {
		if err := db.Close(); err != nil {
			resultErr = errors.Join(resultErr, errors.New("备份校验数据库关闭失败"))
		}
	}()
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, errors.New("备份校验连接不可打开")
	}
	defer func() {
		if err := conn.Close(); err != nil {
			resultErr = errors.Join(resultErr, errors.New("备份校验连接关闭失败"))
		}
	}()
	if _, err = stableSnapshotFile(path, file, info); err != nil {
		return 0, err
	}
	if _, err = conn.ExecContext(ctx, "PRAGMA query_only=ON; PRAGMA trusted_schema=OFF"); err != nil {
		return 0, errors.New("备份只读检查不可初始化")
	}
	if err = conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, errors.New("备份版本不可读取")
	}
	if version != currentDatabaseSchemaVersion {
		return 0, errors.New("备份数据库版本不受支持")
	}
	var count, pageSize int64
	if err = conn.QueryRowContext(ctx, "PRAGMA page_count").Scan(&count); err != nil {
		return 0, errors.New("备份页数不可读取")
	}
	if err = conn.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, errors.New("备份页尺寸不可读取")
	}
	actualSize, err := snapshotPageBytes(count, pageSize)
	if err != nil || actualSize != info.Size() {
		return 0, errors.New("备份页长度与文件不一致")
	}
	// LIMIT 0只探测当前核心、账号和备份列，不读取凭据或会话摘要值。
	for _, statement := range []string{
		"SELECT id,username,password_hash,auth_revision FROM users LIMIT 0",
		"SELECT token_hash,user_id,expires_at,session_id,created_at,client_browser,client_os,auth_revision FROM sessions LIMIT 0",
		"SELECT id,name,type,path FROM libraries LIMIT 0",
		"SELECT id,library_id,relative_path,size,missing FROM items LIMIT 0",
		"SELECT user_id,item_id,favorite,tags FROM user_items LIMIT 0",
		"SELECT user_id,item_id,position,total,locator FROM progress LIMIT 0",
		"SELECT id,library_id,item_id,kind,status FROM jobs LIMIT 0",
		"SELECT id FROM downloads LIMIT 0",
		"SELECT id,body FROM collection_sources LIMIT 0",
		"SELECT id,source_id,state FROM collection_tasks LIMIT 0",
		"SELECT id,source_id,state FROM collection_external_actions LIMIT 0",
		"SELECT id,revision,enabled,hour_utc,minute_utc,keep_count,max_bytes,schedule_revision,effective_from_ms,schedule_cursor_ms,next_run_ms,updated_at_ms FROM backup_policy LIMIT 0",
		"SELECT id,store_id,root_bound,request_clock_high_water_ms,history_truncated_count,last_good_file_id,last_success_json,last_failure_json,retention_authorizer_file_id,retention_authorized_keep_count,retention_authorized_max_bytes FROM backup_store LIMIT 0",
		"SELECT seq,id,request_id,trigger,schedule_key,slot_ms,schedule_revision,policy_revision,keep_count,max_bytes,state,cancel_requested_at_ms,created_at_ms,updated_at_ms,started_at_ms,finished_at_ms,file_id,error_code,message,skipped_slots,admission_deadline_ms FROM backup_runs LIMIT 0",
		"SELECT id,store_id,run_id,run_seq,state,created_at_ms,published_at_ms,verified_at_ms,deleted_at_ms,size_bytes,sha256,schema_version,app_version,manifest_size,manifest_sha256,owner_size,owner_sha256,error_code FROM backup_files LIMIT 0",
		"SELECT request_id,request_time_ms,accepted_at_ms,expires_at_ms,run_id,run_seq,terminal,summary_json FROM backup_request_keys LIMIT 0",
	} {
		rows, queryErr := conn.QueryContext(ctx, statement)
		if queryErr != nil {
			return 0, errors.New("备份核心结构不受支持")
		}
		unexpected := rows.Next()
		iterationErr := rows.Err()
		closeErr := rows.Close()
		if unexpected || errors.Join(iterationErr, closeErr) != nil {
			return 0, errors.New("备份结构探针未完成")
		}
	}
	var tables int
	if err = conn.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type='table' AND name IN ('users','sessions','libraries','items','user_items','progress','jobs','downloads','collection_sources','collection_tasks','collection_external_actions','backup_policy','backup_store','backup_runs','backup_files','backup_request_keys')").Scan(&tables); err != nil || tables != 16 {
		return 0, errors.New("备份核心表结构不受支持")
	}
	if err = checkSnapshotBackupStructure(ctx, conn); err != nil {
		return 0, err
	}
	if err = checkSnapshotIntegrity(ctx, conn); err != nil {
		return 0, err
	}
	return version, nil
}

// checkSnapshotBackupStructure 核对当前五表严格类型、字段类型及初始化单例，不读取备份摘要内容。
func checkSnapshotBackupStructure(ctx context.Context, conn *sql.Conn) error {
	var strictTables int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_list WHERE schema='main' AND type='table' AND strict=1 AND name IN('backup_policy','backup_store','backup_runs','backup_files','backup_request_keys')").Scan(&strictTables); err != nil || strictTables != 5 {
		return errors.New("备份存储严格表结构不受支持")
	}
	columns := map[string]string{
		"backup_policy":       "id:INTEGER revision:INTEGER enabled:INTEGER hour_utc:INTEGER minute_utc:INTEGER keep_count:INTEGER max_bytes:INTEGER schedule_revision:INTEGER effective_from_ms:INTEGER schedule_cursor_ms:INTEGER next_run_ms:INTEGER updated_at_ms:INTEGER",
		"backup_store":        "id:INTEGER store_id:TEXT root_bound:INTEGER request_clock_high_water_ms:INTEGER history_truncated_count:INTEGER last_good_file_id:TEXT last_success_json:TEXT last_failure_json:TEXT retention_authorizer_file_id:TEXT retention_authorized_keep_count:INTEGER retention_authorized_max_bytes:INTEGER",
		"backup_runs":         "seq:INTEGER id:TEXT request_id:TEXT trigger:TEXT schedule_key:TEXT slot_ms:INTEGER schedule_revision:INTEGER policy_revision:INTEGER keep_count:INTEGER max_bytes:INTEGER state:TEXT cancel_requested_at_ms:INTEGER created_at_ms:INTEGER updated_at_ms:INTEGER started_at_ms:INTEGER finished_at_ms:INTEGER file_id:TEXT error_code:TEXT message:TEXT skipped_slots:INTEGER admission_deadline_ms:INTEGER",
		"backup_files":        "id:TEXT store_id:TEXT run_id:TEXT run_seq:INTEGER state:TEXT created_at_ms:INTEGER published_at_ms:INTEGER verified_at_ms:INTEGER deleted_at_ms:INTEGER size_bytes:INTEGER sha256:TEXT schema_version:INTEGER app_version:TEXT manifest_size:INTEGER manifest_sha256:TEXT owner_size:INTEGER owner_sha256:TEXT error_code:TEXT",
		"backup_request_keys": "request_id:TEXT request_time_ms:INTEGER accepted_at_ms:INTEGER expires_at_ms:INTEGER run_id:TEXT run_seq:INTEGER terminal:INTEGER summary_json:TEXT",
	}
	for table, declaration := range columns {
		expected := make(map[string]string)
		for _, field := range strings.Fields(declaration) {
			parts := strings.SplitN(field, ":", 2)
			expected[parts[0]] = parts[1]
		}
		// table_info会遗漏生成隐藏列；当前头探针必须核对实际全部字段，不能放行伪结构。
		rows, err := conn.QueryContext(ctx, "SELECT name,type,hidden FROM pragma_table_xinfo(?)", table)
		if err != nil {
			return errors.New("备份存储字段不可读取")
		}
		matched := 0
		valid := true
		for rows.Next() {
			var name, kind string
			var hidden int
			if scanErr := rows.Scan(&name, &kind, &hidden); scanErr != nil {
				valid = false
				break
			}
			if hidden != 0 || expected[name] != kind {
				valid = false
			}
			matched++
		}
		iterationErr := rows.Err()
		closeErr := rows.Close()
		if !valid || matched != len(expected) || errors.Join(iterationErr, closeErr) != nil {
			return errors.New("备份存储字段类型不受支持")
		}
	}
	for _, table := range []string{"backup_policy", "backup_store"} {
		var count, id int64
		// 表名来自上方固定内部集合，不接受外部SQL标识符。
		if err := conn.QueryRowContext(ctx, "SELECT count(*),coalesce(min(id),0) FROM "+table).Scan(&count, &id); err != nil || count != 1 || id != 1 {
			return errors.New("备份存储初始化单例不受支持")
		}
	}
	return nil
}

// checkSnapshotIntegrity 要求完整性检查恰一行ok，外键检查零行，不收集无界诊断。
func checkSnapshotIntegrity(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, "PRAGMA integrity_check(1)")
	if err != nil {
		return errors.New("备份完整性检查不可执行")
	}
	var text string
	valid := rows.Next() && rows.Scan(&text) == nil && text == "ok" && !rows.Next()
	iterationErr := rows.Err()
	closeErr := rows.Close()
	if !valid || errors.Join(iterationErr, closeErr) != nil {
		return errors.New("备份完整性检查未通过")
	}
	rows, err = conn.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return errors.New("备份外键检查不可执行")
	}
	violated := rows.Next()
	iterationErr = rows.Err()
	closeErr = rows.Close()
	if violated || errors.Join(iterationErr, closeErr) != nil {
		return errors.New("备份外键检查未通过")
	}
	return nil
}

// hashSnapshotFile 用固定64KiB缓冲检查取消与精确总长，不按数据库体积申请内存。
func hashSnapshotFile(ctx context.Context, reader io.Reader, size int64) ([sha256.Size]byte, error) {
	var result [sha256.Size]byte
	hash := sha256.New()
	var buffer [64 * 1024]byte
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		count, err := reader.Read(buffer[:])
		if count < 0 || count > len(buffer) || int64(count) > math.MaxInt64-total || total+int64(count) > size {
			return result, errors.New("备份摘要读取长度无效")
		}
		if count > 0 {
			_, _ = hash.Write(buffer[:count])
			total += int64(count)
		}
		if cancelErr := ctx.Err(); cancelErr != nil {
			return result, cancelErr
		}
		if err == io.EOF {
			break
		}
		if err != nil || count == 0 {
			return result, errors.New("备份摘要读取失败")
		}
	}
	if total != size {
		return result, errors.New("备份摘要读取长度不一致")
	}
	copy(result[:], hash.Sum(nil))
	return result, nil
}
