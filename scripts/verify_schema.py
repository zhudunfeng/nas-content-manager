"""从未修改的历史迁移构造真正旧库，仅供隔离集成验证使用。"""

from __future__ import annotations

from contextlib import closing
from pathlib import Path
import re
import sqlite3


def create_legacy_database(target: Path, version: int, source: Path | None = None) -> dict[str, int]:
    """用真实 v1–v5 表结构创建新夹具，可按原列完整保留本机隔离来源库的数据。"""
    schema_files: tuple[str, ...] = (
        'internal/app/store.go',
        'internal/app/download_models.go',
        'internal/app/collection_models.go',
        'internal/app/collection_external_store.go',
        'internal/app/account_store.go',
    )  # 只允许仓库中冻结的历史迁移，绝不复制最新数据库的表定义。
    if version < 1 or version > len(schema_files):
        raise ValueError('历史夹具仅支持真实 schema1–5')
    repository = Path(__file__).resolve().parent.parent
    statements: list[str] = []
    for number, relative in enumerate(schema_files[:version], 1):
        content = (repository / relative).read_text(encoding='utf-8')
        matches = re.findall(r'^const schemaV' + str(number) + r' = `([^`]*)`', content, re.MULTILINE)
        if len(matches) != 1:
            raise AssertionError('历史迁移常量缺失或不唯一')
        statements.append(matches[0])
    if source is not None and (not source.is_file() or source.resolve() == target.resolve()):
        raise ValueError('历史夹具来源必须是另一个既有隔离数据库')
    # 排他创建，失败夹具也保留，不能覆盖旧证据或来源库。
    with target.open('xb'):
        pass
    counts: dict[str, int] = {}
    with closing(sqlite3.connect(target, uri=True)) as database:
        database.execute('PRAGMA foreign_keys=ON')
        database.executescript('\n'.join(statements))
        if source is not None:
            database.execute('ATTACH DATABASE ? AS existing', (source.resolve().as_uri() + '?mode=ro',))
            tables = [str(row[0]) for row in database.execute("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY rowid")]
            for table in tables:
                # 名称均来自冻结迁移，仍使用标识符转义而不是拼接外部输入。
                quoted = '"' + table.replace('"', '""') + '"'
                columns = ','.join('"' + str(row[1]).replace('"', '""') + '"' for row in database.execute('PRAGMA table_info(' + quoted + ')'))
                database.execute('INSERT INTO main.' + quoted + '(' + columns + ') SELECT ' + columns + ' FROM existing.' + quoted)
                counts[table] = int(database.execute('SELECT count(*) FROM main.' + quoted).fetchone()[0])
                original = int(database.execute('SELECT count(*) FROM existing.' + quoted).fetchone()[0])
                difference = database.execute('SELECT ' + columns + ' FROM existing.' + quoted + ' EXCEPT SELECT ' + columns + ' FROM main.' + quoted).fetchone()
                if counts[table] != original or difference is not None:
                    raise AssertionError('历史夹具未完整保留原表字段与记录')
        if version == 5:
            # 先拷贝已有合法公开编号，再安装唯一索引和非空触发器，不能以空编号冒充历史账号。
            account_source = (repository / schema_files[4]).read_text(encoding='utf-8')
            constraints = re.findall(r'^const schemaV5SessionConstraints = `([^`]*)`', account_source, re.MULTILINE)
            if len(constraints) != 1:
                raise AssertionError('历史账号约束常量缺失或不唯一')
            database.executescript(constraints[0])
        database.execute('PRAGMA user_version=' + str(version))
        database.commit()
        if source is not None:
            database.execute('DETACH DATABASE existing')
        if database.execute('PRAGMA integrity_check').fetchone()[0] != 'ok' or database.execute('PRAGMA foreign_key_check').fetchone() is not None:
            raise AssertionError('真实历史夹具完整性校验失败')
        users = {str(row[1]) for row in database.execute('PRAGMA table_info(users)')}
        sessions = {str(row[1]) for row in database.execute('PRAGMA table_info(sessions)')}
        expected_users = {'id', 'username', 'password_hash'}
        expected_sessions = {'token_hash', 'user_id', 'expires_at'}
        if version == 5:
            expected_users.add('auth_revision')
            expected_sessions.update({'session_id', 'created_at', 'client_browser', 'client_os', 'auth_revision'})
        if users != expected_users or sessions != expected_sessions:
            raise AssertionError('历史夹具账号字段与对应真实迁移不一致')
    return counts
