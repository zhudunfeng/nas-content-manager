"""用同一实际主程序验证真实v5备份存储迁移、失败回滚与默认关闭，不访问用户实例。"""

from __future__ import annotations

import sys

if __name__ == '__main__':
    sys.dont_write_bytecode = True

import argparse
from contextlib import closing
import hashlib
import os
from pathlib import Path
import re
import sqlite3

from verify import generate_password
from verify_account_offline import RawProcess, WindowsConsole, raw_has_no_secrets, snapshot
from verify_backup import BackupHarness
from verify_baseline import Blocked, clean_environment, file_identity, new_run_root, write_json
from verify_schema import create_legacy_database


def run_suite(binary: Path, root: Path, console: WindowsConsole | None) -> None:
    """复用真实Console停服、内存秘密扫描和新根，保留失败证据而不覆盖旧运行。"""
    harnesses: list[BackupHarness] = []  # 每个场景独立持有进程、Cookie和原始双流。
    assertions: list[str] = []  # 仅计已经执行的固定中文断言。
    completed: bool = False  # 只有所有业务检查通过才置位。
    cleanup_failures: list[str] = []  # 收尾失败令本轮失败，不能被成功正文掩盖。
    artifact = file_identity(binary)

    def check(condition: bool, label: str) -> None:
        """只持久化固定标签，不输出凭据、宿主路径或数据库行。"""
        if not condition:
            raise AssertionError(label)
        assertions.append(label)
        print('  ✓ ' + label, flush=True)

    def make_harness(name: str) -> BackupHarness:
        """每个真实启动使用全新目录，不复用其它场景的数据库或日志。"""
        area = root / name
        area.mkdir()
        harness = BackupHarness(binary, area, console)
        harnesses.append(harness)
        return harness

    try:
        seed = make_harness('fresh')
        seed.start()
        password = generate_password()
        seed.secret_values.append(password.encode())
        seed.client.json('/api/setup', 'POST', {'username': 'backup-store-verify', 'password': password})
        for cookie in seed.client.cookies:
            seed.secret_values.extend((cookie.value.encode(), hashlib.sha256(cookie.value.encode()).hexdigest().encode()))
        source = seed.root / 'data/library.db'
        with closing(sqlite3.connect(source)) as database:
            seed.secret_values.append(database.execute('SELECT password_hash FROM users').fetchone()[0].encode())
            policy = database.execute('SELECT * FROM backup_policy').fetchall()
            store = database.execute('SELECT * FROM backup_store').fetchall()
            strict = dict(database.execute("SELECT name,strict FROM pragma_table_list WHERE name LIKE 'backup_%'"))
            check(database.execute('PRAGMA user_version').fetchone() == (6,) and len(strict) == 5 and all(value == 1 for value in strict.values()), '新库经过真实迁移成为schema6，五张备份表均为STRICT')
            check(len(policy) == 1 and policy[0][:8] == (1, 1, 0, 3, 0, 7, 10737418240, 1) and policy[0][8] == policy[0][11] and policy[0][9:11] == (None, None), '唯一默认策略关闭、每日03点UTC、保留7份10GiB且同一次初始化时间')
            check(len(store) == 1 and re.fullmatch('[0-9a-f]{32}', store[0][1]) is not None and store[0][2:] == (0, 0, 0, None, 'null', 'null', None, None, None), '唯一存储身份为32位随机小写hex，根未绑定且摘要与轮转授权为空')
            original_store = store[0][1]
            check(all(database.execute('SELECT count(*) FROM ' + table).fetchone() == (0,) for table in ('backup_runs', 'backup_files', 'backup_request_keys')), '迁移不伪造运行、文件或请求历史')
            check(not database.execute("SELECT name FROM sqlite_schema WHERE type='trigger' AND tbl_name LIKE 'backup_%'").fetchall() and all(not database.execute('PRAGMA foreign_key_list(' + table + ')').fetchall() for table in strict), '五表不添加跨域外键、级联或容量触发器')
        check(not (source.parent / 'backups').exists() and not seed.pending(), '默认关闭迁移不建立备份根或临时快照')
        seed.close_process(True)
        seed.start()
        with closing(sqlite3.connect(source)) as database:
            check(database.execute('SELECT * FROM backup_policy').fetchall() == policy and database.execute('SELECT store_id FROM backup_store').fetchone() == (original_store,), '重复真实启动不重建存储身份或重置策略时间')
        seed.close_process(True)

        legacy = make_harness('true-v5')
        legacy_source = legacy.root / 'data/library.db'
        create_legacy_database(legacy_source, 5, source)
        old = snapshot(legacy_source)
        check(old[0] == 5 and not any(name.startswith('backup_') for name in old[2]), '历史夹具从冻结v1至v5构造，没有备份新表且不是改头伪装')
        for cookie in seed.client.cookies:
            legacy.client.cookies.set_cookie(cookie)
        legacy.secret_values.extend(seed.secret_values)
        sentinel = legacy_source.parent / '.backup-pending-sentinel'
        sentinel.mkdir()
        (sentinel / 'unknown.bin').write_bytes(b'unknown-backup-must-stay')
        legacy.start()
        check(legacy.client.request('/api/stats')[0] == 200, '真实v5升级后原Cookie继续有效，迁移没有重置会话')
        with closing(sqlite3.connect(legacy_source)) as database:
            preserved = all(sorted(database.execute('SELECT * FROM "' + table.replace('"', '""') + '"').fetchall(), key=repr) == values for table, values in old[2].items())
            check(preserved and database.execute('PRAGMA user_version').fetchone() == (6,), '真实v5升6保留每张原表、账号hash修订和会话全部字段')
            check(database.execute('PRAGMA integrity_check(1)').fetchall() == [('ok',)] and not database.execute('PRAGMA foreign_key_check').fetchall(), '升级后的SQLite完整性和全部外键一致')
        legacy.close_process(True)
        check((sentinel / 'unknown.bin').read_bytes() == b'unknown-backup-must-stay' and not (legacy_source.parent / 'backups').exists(), '真实迁移不扫描删除未知pending哨兵，也不创建备份根')

        # 失败场景均从同一个真实v5来源重新构造，既有冲突对象必须完整保留。
        for name, statements in (('future', 'PRAGMA user_version=7'), ('conflict', "CREATE TABLE backup_runs(sentinel TEXT); INSERT INTO backup_runs VALUES('must-stay')")):
            rejected = make_harness(name)
            target = rejected.root / 'data/library.db'
            create_legacy_database(target, 5, source)
            with closing(sqlite3.connect(target)) as database:
                database.executescript(statements)
                database.commit()
            before = snapshot(target)
            rejected.secret_values.extend(seed.secret_values)
            refused = False
            try:
                rejected.start()
            except RuntimeError:
                refused = rejected.owned is not None and rejected.owned.process.poll() not in (None, 0)
            finally:
                rejected.close_process(False)
            check(refused and snapshot(target) == before, '启动拒绝' + name + '且原头、原对象、账号和记录全部保留')
            check(not (target.parent / 'backups').exists() and not rejected.pending(), '启动拒绝' + name + '不生成备份目录或候选')
        check(file_identity(binary) == artifact, '所有实际启动使用同一个未变化产物')
        completed = True
    finally:
        for harness in harnesses:
            try:
                harness.close_process(False)
            except BaseException:
                cleanup_failures.append('自有实例或原始日志收尾失败')
        write_json(root / 'assertions.json', assertions)
        write_json(root / 'result.json', {'passed': completed and not cleanup_failures, 'assertions': len(assertions), 'artifact': file_identity(binary), 'schemaVersion': 6, 'coverage': ['real-frozen-v5-migration', 'fresh-strict-defaults', 'old-cookie-and-all-original-rows', 'stable-store-id', 'future-7-refusal', 'atomic-conflict-rollback', 'no-backup-root-or-pending-side-effects'], 'applicationStarts': sum(value.starts for value in harnesses), 'normalStops': sum(value.normal_stops for value in harnesses), 'forcedApplicationReclaims': sum(value.forced_reclaims for value in harnesses), 'rawLogsCheckedBeforeSanitize': all(value.raw_checked == value.starts for value in harnesses), 'cleanupFailures': cleanup_failures, 'targetNAS': False, 'limits': ['只验证存储和维护兼容；无持久目标生成、API、调度或轮转', '真实Linux/NAS、断电、FULL和拔盘仍待对应后片实测']})
        if cleanup_failures:
            raise AssertionError('存储专项收尾失败，已保留失败证据')
    print('备份存储集成验证通过：' + str(len(assertions)) + ' 项断言', flush=True)


def main() -> int:
    """要求明确同一产物和全新仓库运行根，Windows在自有Console正常停服。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True, help='明确指定已经构建的主服务产物')  # 验证期间不自行构建。
    parser.add_argument('--root', help='尚不存在的.runtime/verify后代目录')  # 历史证据不得覆盖。
    parser.add_argument('--console-helper', action='store_true', help=argparse.SUPPRESS)  # 仅内部自有隐藏Console入口。
    args = parser.parse_args()
    if os.name == 'nt' and not args.console_helper:
        owned = RawProcess([sys.executable, '-B', str(Path(__file__).resolve()), *sys.argv[1:], '--console-helper'], clean_environment(), console=True)
        try:
            code = owned.wait(timeout=180)
            output = b''.join(bytes(block) for block in owned.raw)
            if not raw_has_no_secrets([output], []):
                raise AssertionError('存储Console helper输出未通过秘密检查')
            print(output.decode('utf-8', 'replace').replace('\r\n', '\n'), end='', flush=True)
            return code
        finally:
            owned.close()
    binary = args.binary.resolve()
    if not binary.is_file() or binary.is_symlink():
        raise Blocked('缺少明确指定的普通主服务产物')
    root = new_run_root(args.root, 'backup-store')
    console = WindowsConsole() if os.name == 'nt' else None
    try:
        run_suite(binary, root, console)
    finally:
        if console is not None:
            console.close()
    return 0


if __name__ == '__main__':
    from project_env import run_cli

    try:
        raise SystemExit(run_cli(main))
    except (OSError, ValueError, RuntimeError, AssertionError) as error:
        print('备份存储集成验证失败：' + type(error).__name__, file=sys.stderr, flush=True)
        raise SystemExit(2 if isinstance(error, Blocked) else 1)
