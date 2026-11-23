package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"time"
)

// migrateBackupSchema 在调用方短事务内创建备份存储和默认关闭策略，不探测文件根。
func migrateBackupSchema(ctx context.Context, tx *sql.Tx) error {
	return migrateBackupSchemaWithSources(ctx, tx, time.Now, rand.Reader)
}

// migrateBackupSchemaWithSources 注入单次采样源以验证失败回滚，不提供生产配置或全局可变时钟。
func migrateBackupSchemaWithSources(ctx context.Context, tx *sql.Tx, clock func() time.Time, entropy io.Reader) error {
	// 同一迁移只采一次墙钟，初始化时间不受相邻读数差异影响。
	timestamp := clock().UTC().UnixMilli()
	if timestamp < 0 || timestamp > 253402300799999 {
		return errors.New("备份存储初始化时间超出支持范围")
	}
	var identifier [16]byte
	if _, err := io.ReadFull(entropy, identifier[:]); err != nil {
		return errors.New("备份存储身份生成失败")
	}
	if _, err := tx.ExecContext(ctx, schemaV6); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO backup_policy
		(id,revision,enabled,hour_utc,minute_utc,keep_count,max_bytes,schedule_revision,effective_from_ms,schedule_cursor_ms,next_run_ms,updated_at_ms)
		VALUES(1,1,0,3,0,7,10737418240,1,?,NULL,NULL,?)`, timestamp, timestamp); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO backup_store(id,store_id) VALUES(1,?)", hex.EncodeToString(identifier[:]))
	return err
}

// schemaV6 冻结备份存储边界；跨表归属、容量、状态转换和裁剪由后续短事务服务维护。
const schemaV6 = `
CREATE TABLE backup_policy (
    -- 唯一策略槽位。
    id INTEGER PRIMARY KEY CHECK(id=1),
    -- 策略乐观锁修订。
    revision INTEGER NOT NULL CHECK(revision>=1),
    -- 是否启用每日计划；当前迁移始终关闭。
    enabled INTEGER NOT NULL CHECK(enabled IN(0,1)),
    -- 每日UTC小时。
    hour_utc INTEGER NOT NULL CHECK(hour_utc BETWEEN 0 AND 23),
    -- 每日UTC分钟。
    minute_utc INTEGER NOT NULL CHECK(minute_utc BETWEEN 0 AND 59),
    -- 保留数量上限。
    keep_count INTEGER NOT NULL CHECK(keep_count BETWEEN 1 AND 30),
    -- 保留字节预算。
    max_bytes INTEGER NOT NULL CHECK(max_bytes BETWEEN 67108864 AND 1099511627776),
    -- 计划语义修订。
    schedule_revision INTEGER NOT NULL CHECK(schedule_revision>=1),
    -- 当前计划生效的UTC毫秒。
    effective_from_ms INTEGER NOT NULL CHECK(effective_from_ms BETWEEN 0 AND 253402300799999),
    -- 已处理计划槽位的UTC毫秒。
    schedule_cursor_ms INTEGER CHECK(schedule_cursor_ms IS NULL OR schedule_cursor_ms BETWEEN 0 AND 253402300799999),
    -- 下一次执行的UTC毫秒，关闭时必须为空。
    next_run_ms INTEGER CHECK(next_run_ms IS NULL OR next_run_ms BETWEEN 0 AND 253402300799999),
    -- 最后更新的UTC毫秒。
    updated_at_ms INTEGER NOT NULL CHECK(updated_at_ms BETWEEN 0 AND 253402300799999),
    CHECK((enabled=0 AND next_run_ms IS NULL) OR (enabled=1 AND next_run_ms IS NOT NULL))
) STRICT;
CREATE TABLE backup_store (
    -- 唯一存储槽位。
    id INTEGER PRIMARY KEY CHECK(id=1),
    -- 数据库独立备份命名空间，不代表宿主物理身份。
    store_id TEXT NOT NULL UNIQUE CHECK(length(CAST(store_id AS BLOB))=32 AND store_id NOT GLOB '*[^0-9a-f]*' AND instr(store_id,char(0))=0),
    -- 是否已经显式绑定备份根。
    root_bound INTEGER NOT NULL DEFAULT 0 CHECK(root_bound IN(0,1)),
    -- 请求接纳时钟高水位。
    request_clock_high_water_ms INTEGER NOT NULL DEFAULT 0 CHECK(request_clock_high_water_ms BETWEEN 0 AND 253402300799999),
    -- 历史被有界裁剪的累计数量。
    history_truncated_count INTEGER NOT NULL DEFAULT 0 CHECK(history_truncated_count>=0),
    -- 最近已验证好文件的编号，不构成轮转授权。
    last_good_file_id TEXT CHECK(last_good_file_id IS NULL OR (length(CAST(last_good_file_id AS BLOB))=32 AND last_good_file_id NOT GLOB '*[^0-9a-f]*' AND instr(last_good_file_id,char(0))=0)),
    -- 有界成功摘要，仅允许对象或JSON空值。
    last_success_json TEXT NOT NULL DEFAULT 'null' CHECK(length(CAST(last_success_json AS BLOB))<=2048 AND CASE WHEN json_valid(last_success_json) THEN json_type(last_success_json) IN('object','null') ELSE 0 END),
    -- 有界失败摘要，仅允许对象或JSON空值。
    last_failure_json TEXT NOT NULL DEFAULT 'null' CHECK(length(CAST(last_failure_json AS BLOB))<=2048 AND CASE WHEN json_valid(last_failure_json) THEN json_type(last_failure_json) IN('object','null') ELSE 0 END),
    -- 触发本次有限轮转的成功文件编号。
    retention_authorizer_file_id TEXT CHECK(retention_authorizer_file_id IS NULL OR (length(CAST(retention_authorizer_file_id AS BLOB))=32 AND retention_authorizer_file_id NOT GLOB '*[^0-9a-f]*' AND instr(retention_authorizer_file_id,char(0))=0)),
    -- 授权时冻结的保留数量。
    retention_authorized_keep_count INTEGER CHECK(retention_authorized_keep_count IS NULL OR retention_authorized_keep_count BETWEEN 1 AND 30),
    -- 授权时冻结的字节预算。
    retention_authorized_max_bytes INTEGER CHECK(retention_authorized_max_bytes IS NULL OR retention_authorized_max_bytes BETWEEN 67108864 AND 1099511627776),
    CHECK((retention_authorizer_file_id IS NULL AND retention_authorized_keep_count IS NULL AND retention_authorized_max_bytes IS NULL)
       OR (retention_authorizer_file_id IS NOT NULL AND retention_authorized_keep_count IS NOT NULL AND retention_authorized_max_bytes IS NOT NULL))
) STRICT;
CREATE TABLE backup_runs (
    -- 与墙钟无关的递增列表顺序。
    seq INTEGER PRIMARY KEY AUTOINCREMENT CHECK(seq>=1),
    -- 本地执行编号。
    id TEXT NOT NULL UNIQUE CHECK(length(CAST(id AS BLOB))=32 AND id NOT GLOB '*[^0-9a-f]*' AND instr(id,char(0))=0),
    -- 手动请求的规范小写UUIDv7。
    request_id TEXT UNIQUE CHECK(request_id IS NULL OR (length(CAST(request_id AS BLOB))=36 AND length(request_id)=36 AND instr(request_id,char(0))=0 AND substr(request_id,9,1)='-' AND substr(request_id,14,1)='-' AND substr(request_id,19,1)='-' AND substr(request_id,24,1)='-' AND substr(request_id,15,1)='7' AND substr(request_id,20,1) IN('8','9','a','b') AND length(replace(request_id,'-',''))=32 AND replace(request_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    -- 手动或每日计划来源。
    trigger TEXT NOT NULL CHECK(trigger IN('manual','scheduled')),
    -- 有界计划去重键，精确语义由计划服务解析。
    schedule_key TEXT UNIQUE CHECK(schedule_key IS NULL OR (length(CAST(schedule_key AS BLOB)) BETWEEN 1 AND 96 AND schedule_key NOT GLOB '*[^0-9s:]*' AND instr(schedule_key,char(0))=0)),
    -- 原计划槽位UTC毫秒。
    slot_ms INTEGER CHECK(slot_ms IS NULL OR slot_ms BETWEEN 0 AND 253402300799999),
    -- 接纳时的计划修订。
    schedule_revision INTEGER CHECK(schedule_revision IS NULL OR schedule_revision>=1),
    -- 接纳时的策略修订。
    policy_revision INTEGER NOT NULL CHECK(policy_revision>=1),
    -- 接纳时冻结的保留数量。
    keep_count INTEGER NOT NULL CHECK(keep_count BETWEEN 1 AND 30),
    -- 接纳时冻结的保留字节预算。
    max_bytes INTEGER NOT NULL CHECK(max_bytes BETWEEN 67108864 AND 1099511627776),
    -- 执行生命周期事实。
    state TEXT NOT NULL CHECK(state IN('queued','running','completed','failed','interrupted')),
    -- 取消意图接纳的UTC毫秒。
    cancel_requested_at_ms INTEGER CHECK(cancel_requested_at_ms IS NULL OR cancel_requested_at_ms BETWEEN 0 AND 253402300799999),
    -- 创建的UTC毫秒。
    created_at_ms INTEGER NOT NULL CHECK(created_at_ms BETWEEN 0 AND 253402300799999),
    -- 最后更新的UTC毫秒，不要求墙钟单调。
    updated_at_ms INTEGER NOT NULL CHECK(updated_at_ms BETWEEN 0 AND 253402300799999),
    -- 实际开始的UTC毫秒。
    started_at_ms INTEGER CHECK(started_at_ms IS NULL OR started_at_ms BETWEEN 0 AND 253402300799999),
    -- 终态完成的UTC毫秒。
    finished_at_ms INTEGER CHECK(finished_at_ms IS NULL OR finished_at_ms BETWEEN 0 AND 253402300799999),
    -- 独立文件所有权编号。
    file_id TEXT CHECK(file_id IS NULL OR (length(CAST(file_id AS BLOB))=32 AND file_id NOT GLOB '*[^0-9a-f]*' AND instr(file_id,char(0))=0)),
    -- 有界安全错误码，不保存原始错误和路径。
    error_code TEXT CHECK(error_code IS NULL OR (length(CAST(error_code AS BLOB)) BETWEEN 1 AND 64 AND error_code NOT GLOB '*[^a-z0-9_]*' AND instr(error_code,char(0))=0)),
    -- 有界中文安全消息。
    message TEXT NOT NULL CHECK(length(CAST(message AS BLOB))<=256),
    -- 合并跳过的计划槽位数量。
    skipped_slots INTEGER NOT NULL DEFAULT 0 CHECK(skipped_slots>=0),
    -- 接纳等待截止的UTC毫秒。
    admission_deadline_ms INTEGER NOT NULL CHECK(admission_deadline_ms BETWEEN 0 AND 253402300799999),
    CHECK((trigger='manual' AND request_id IS NOT NULL AND schedule_key IS NULL AND slot_ms IS NULL AND schedule_revision IS NULL)
       OR (trigger='scheduled' AND request_id IS NULL AND schedule_key IS NOT NULL AND slot_ms IS NOT NULL AND schedule_revision IS NOT NULL)),
    CHECK((state='queued' AND started_at_ms IS NULL AND finished_at_ms IS NULL AND file_id IS NULL)
       OR (state='running' AND started_at_ms IS NOT NULL AND finished_at_ms IS NULL)
       OR (state='completed' AND started_at_ms IS NOT NULL AND finished_at_ms IS NOT NULL)
       OR (state IN('failed','interrupted') AND finished_at_ms IS NOT NULL)),
    CHECK((state IN('queued','running') AND error_code IS NULL)
       OR (state='completed' AND file_id IS NOT NULL AND error_code IS NULL)
       OR (state IN('failed','interrupted') AND error_code IS NOT NULL))
) STRICT;
CREATE UNIQUE INDEX idx_backup_runs_single_active ON backup_runs((1)) WHERE state IN('queued','running');
CREATE TABLE backup_files (
    -- 精确自有文件编号，文件名由后续服务固定推导。
    id TEXT PRIMARY KEY NOT NULL CHECK(length(CAST(id AS BLOB))=32 AND id NOT GLOB '*[^0-9a-f]*' AND instr(id,char(0))=0),
    -- 文件所属的备份命名空间。
    store_id TEXT NOT NULL CHECK(length(CAST(store_id AS BLOB))=32 AND store_id NOT GLOB '*[^0-9a-f]*' AND instr(store_id,char(0))=0),
    -- 产生文件的执行编号。
    run_id TEXT NOT NULL UNIQUE CHECK(length(CAST(run_id AS BLOB))=32 AND run_id NOT GLOB '*[^0-9a-f]*' AND instr(run_id,char(0))=0),
    -- 产生文件的执行顺序。
    run_seq INTEGER NOT NULL UNIQUE CHECK(run_seq>=1),
    -- 独立于执行历史的文件所有权状态。
    state TEXT NOT NULL CHECK(state IN('pending','retained','deleting','missing','unverified','deleted')),
    -- 所有权创建的UTC毫秒。
    created_at_ms INTEGER NOT NULL CHECK(created_at_ms BETWEEN 0 AND 253402300799999),
    -- 完成发布的UTC毫秒。
    published_at_ms INTEGER CHECK(published_at_ms IS NULL OR published_at_ms BETWEEN 0 AND 253402300799999),
    -- 完成只读复验的UTC毫秒。
    verified_at_ms INTEGER CHECK(verified_at_ms IS NULL OR verified_at_ms BETWEEN 0 AND 253402300799999),
    -- 确认删除的UTC毫秒。
    deleted_at_ms INTEGER CHECK(deleted_at_ms IS NULL OR deleted_at_ms BETWEEN 0 AND 253402300799999),
    -- 快照实际字节数。
    size_bytes INTEGER CHECK(size_bytes IS NULL OR size_bytes>0),
    -- 快照固定SHA256摘要。
    sha256 TEXT CHECK(sha256 IS NULL OR (length(CAST(sha256 AS BLOB))=64 AND sha256 NOT GLOB '*[^0-9a-f]*' AND instr(sha256,char(0))=0)),
    -- 快照实际数据库头版本。
    schema_version INTEGER CHECK(schema_version IS NULL OR schema_version>0),
    -- 生成程序的有界版本信息。
    app_version TEXT CHECK(app_version IS NULL OR length(CAST(app_version AS BLOB)) BETWEEN 1 AND 64),
    -- 有界manifest的实际字节数。
    manifest_size INTEGER CHECK(manifest_size IS NULL OR manifest_size BETWEEN 1 AND 16384),
    -- manifest固定SHA256摘要。
    manifest_sha256 TEXT CHECK(manifest_sha256 IS NULL OR (length(CAST(manifest_sha256 AS BLOB))=64 AND manifest_sha256 NOT GLOB '*[^0-9a-f]*' AND instr(manifest_sha256,char(0))=0)),
    -- 有界所有权材料的字节数。
    owner_size INTEGER NOT NULL CHECK(owner_size BETWEEN 1 AND 1024),
    -- 所有权材料固定SHA256摘要。
    owner_sha256 TEXT NOT NULL CHECK(length(CAST(owner_sha256 AS BLOB))=64 AND owner_sha256 NOT GLOB '*[^0-9a-f]*' AND instr(owner_sha256,char(0))=0),
    -- 最近核对的有界安全错误码。
    error_code TEXT CHECK(error_code IS NULL OR (length(CAST(error_code AS BLOB)) BETWEEN 1 AND 64 AND error_code NOT GLOB '*[^a-z0-9_]*' AND instr(error_code,char(0))=0)),
    CHECK(state NOT IN('retained','deleting') OR published_at_ms IS NOT NULL),
    CHECK(published_at_ms IS NULL OR (verified_at_ms IS NOT NULL AND size_bytes IS NOT NULL AND sha256 IS NOT NULL AND schema_version IS NOT NULL AND app_version IS NOT NULL AND manifest_size IS NOT NULL AND manifest_sha256 IS NOT NULL)),
    CHECK((state='deleted' AND deleted_at_ms IS NOT NULL) OR (state<>'deleted' AND deleted_at_ms IS NULL))
) STRICT;
CREATE UNIQUE INDEX idx_backup_files_single_deleting ON backup_files((1)) WHERE state='deleting';
CREATE TABLE backup_request_keys (
    -- 手动请求规范小写UUIDv7，不因运行历史裁剪而复用。
    request_id TEXT PRIMARY KEY NOT NULL CHECK(length(CAST(request_id AS BLOB))=36 AND length(request_id)=36 AND instr(request_id,char(0))=0 AND substr(request_id,9,1)='-' AND substr(request_id,14,1)='-' AND substr(request_id,19,1)='-' AND substr(request_id,24,1)='-' AND substr(request_id,15,1)='7' AND substr(request_id,20,1) IN('8','9','a','b') AND length(replace(request_id,'-',''))=32 AND replace(request_id,'-','') NOT GLOB '*[^0-9a-f]*'),
    -- 请求编号内时间的毫秒事实，精确一致性由后续解析器核对。
    request_time_ms INTEGER NOT NULL CHECK(request_time_ms BETWEEN 0 AND 253402214399999),
    -- 首次接纳的UTC毫秒。
    accepted_at_ms INTEGER NOT NULL CHECK(accepted_at_ms BETWEEN 0 AND 253402300799999),
    -- 固定请求时间加24小时，不按重试续期。
    expires_at_ms INTEGER NOT NULL CHECK(expires_at_ms BETWEEN 0 AND 253402300799999 AND expires_at_ms=request_time_ms+86400000),
    -- 原始接纳执行编号。
    run_id TEXT NOT NULL UNIQUE CHECK(length(CAST(run_id AS BLOB))=32 AND run_id NOT GLOB '*[^0-9a-f]*' AND instr(run_id,char(0))=0),
    -- 原始接纳执行顺序。
    run_seq INTEGER NOT NULL CHECK(run_seq>=1),
    -- 原始执行是否已有终态摘要。
    terminal INTEGER NOT NULL DEFAULT 0 CHECK(terminal IN(0,1)),
    -- 有界幂等响应对象，不存储秘密和宿主路径。
    summary_json TEXT NOT NULL CHECK(length(CAST(summary_json AS BLOB))<=2048 AND CASE WHEN json_valid(summary_json) THEN json_type(summary_json)='object' ELSE 0 END)
) STRICT;
`
