"""隔离实际应用验证同步快照、慢下载租约、只读产物与正常停服，不访问用户实例。"""

from __future__ import annotations

import sys

if __name__ == '__main__':
    sys.dont_write_bytecode = True

import argparse
from contextlib import closing
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import secrets
import signal
import socket
import sqlite3
import threading
import time
import urllib.request
from urllib.parse import urlsplit

from verify import Client, generate_password
from verify_account_offline import RawProcess, WindowsConsole, raw_has_no_secrets
from verify_baseline import Blocked, OwnedProcess, clean_environment, file_identity, new_run_root, reserve_port, sanitize, wait_ready, write_json


PAYLOAD_ROWS: int = 256  # 每行64KiB，共16MiB真实SQLite负载，足以阻塞受限接收窗口。
DOWNLOAD_LIMIT: int = 64 * 1024 * 1024  # 单个测试下载最多64MiB，异常响应不能耗尽磁盘。
WAIT_SECONDS: float = 20.0  # 本机单阶段就绪和资源收尾的墙钟上限。


class BackupHarness:
    """管理独立数据根、原始内存日志和真实正常停止，不把Job强杀算作正常退出。"""

    def __init__(self, binary: Path, root: Path, console: WindowsConsole | None) -> None:
        """准备私有验证目录和不可与第三方混淆的静态探针。"""
        self.binary: Path = binary  # 用户明确指定且全程不再构建的单一产物。
        self.root: Path = root  # new_run_root验证过的本轮全新根。
        self.console: WindowsConsole | None = console  # 仅helper自己的隐藏Console。
        self.owned: RawProcess | None = None  # 自有Job或session，异常时才强制回收。
        self.client: Client = Client('http://127.0.0.1')  # Cookie只在内存保留。
        self.client.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(self.client.cookies))
        self.assertions: list[str] = []  # 只记录实际完成的固定中文断言。
        self.secret_values: list[bytes] = []  # 测试密码、摘要和Cookie从不写入证据。
        self.secret_values.extend((str(root).encode(), json.dumps(str(root), ensure_ascii=False)[1:-1].encode()))
        self.starts: int = 0  # 日志编号，重启不覆盖先前证据。
        self.raw_checked: int = 0  # 原始完整双流检查成功的实际启动次数。
        self.normal_stops: int = 0  # 真实中断后wait返回零的实际正常停服次数。
        self.forced_reclaims: int = 0  # 关闭Job前仍存活的应用次数，不能冒充正常停服。
        self.marker: str = 'backup-' + secrets.token_hex(12) + '.txt'  # 唯一就绪路径。
        self.challenge: bytes = secrets.token_hex(24).encode('ascii')  # 精确回显证明自有实例。
        for name in ('data', 'media', 'web', 'logs'):
            (root / name).mkdir()
        (root / 'web/index.html').write_text('<!doctype html><title>隔离备份验证</title>', encoding='utf-8')
        (root / 'web' / self.marker).write_bytes(self.challenge)

    def check(self, condition: bool, label: str) -> None:
        """失败只报告固定标签，避免HTTP正文、凭据和路径进入日志。"""
        if not condition:
            raise AssertionError(label)
        self.assertions.append(label)
        print('  ✓ ' + label, flush=True)

    def start(self) -> None:
        """继承项目缓存隔离，但清除真实NAS配置与代理；精确证明服务归属。"""
        if self.owned is not None:
            raise RuntimeError('隔离备份实例已经运行')
        listener = reserve_port(0)
        port = int(listener.getsockname()[1])
        self.client.base = 'http://127.0.0.1:' + str(port)
        environment = clean_environment()
        environment.update({
            'NAS_ADDR': '127.0.0.1:' + str(port),  # 仅本次预留的回环端口。
            'NAS_DATA_DIR': str(self.root / 'data'),  # 仅本次数据库和临时候选。
            'NAS_MEDIA_ROOTS': str(self.root / 'media'),  # 不授权用户媒体。
            'NAS_WEB_DIR': str(self.root / 'web'),  # 本次随机静态探针。
            'NAS_SCAN_INTERVAL_MINUTES': '60',  # 不依赖短周期后台扫描。
            'NAS_MIN_FREE_MB': '0',  # 仅测试小型夹具不设生产预留。
            'NAS_COOKIE_SECURE': 'false',  # 仅回环HTTP夹具。
            'NAS_DOWNLOAD_MODE': 'disabled',  # 不连接真实下载器。
            'NAS_TORRENT_SEARCH_MODE': 'disabled',  # 不访问站点。
        })
        self.starts += 1
        try:
            listener.close()
            self.owned = RawProcess([str(self.binary)], environment, inherit_console=os.name == 'nt')
            wait_ready(self.owned, self.client.base, self.marker, self.challenge, WAIT_SECONDS)
        finally:
            listener.close()

    def close_process(self, graceful: bool) -> None:
        """先用真实中断等待退出，再检查未脱敏内存，最后保存安全日志和回收Job。"""
        if self.owned is None:
            return
        owned, self.owned = self.owned, None
        code: int | None = None  # 强制回收的退出码不能冒充正常退出。
        try:
            if graceful:
                if self.console is not None:
                    self.console.ctrl_event()
                else:
                    os.killpg(owned.process.pid, signal.SIGINT)
                code = owned.wait(timeout=WAIT_SECONDS)
        finally:
            if owned.process.poll() is None:
                self.forced_reclaims += 1
            owned.close()
            raw = [bytes(block) for block in owned.raw]
            if owned.overflow or owned.reader_failed or not raw_has_no_secrets(raw, self.secret_values):
                raise AssertionError('应用原始日志未通过有界完整与秘密检查，未持久化原文')
            self.raw_checked += 1
            (self.root / 'logs' / ('application-' + str(self.starts) + '.log')).write_text(sanitize(b''.join(raw).decode('utf-8', 'replace')), encoding='utf-8')
        if graceful:
            self.check(code == 0, '真实中断使本实例正常返回零，强杀仅作未使用的兜底')
            self.normal_stops += 1

    def pending(self) -> list[Path]:
        """只枚举本轮数据根，区分预先放入的未知哨兵和活跃候选。"""
        return [path for path in (self.root / 'data').glob('.backup-pending-*') if path.name != '.backup-pending-sentinel']

    def wait_clean(self) -> None:
        """有界等待handler收尾，不以任意固定sleep假定文件释放。"""
        deadline = time.monotonic() + WAIT_SECONDS
        while self.pending() and time.monotonic() < deadline:
            time.sleep(0.02)
        self.check(not self.pending(), '本次精确候选文件及空目录已清理')

    def slow_download(self) -> tuple[http.client.HTTPConnection, http.client.HTTPResponse]:
        """限制真实TCP接收窗口，在收到成功头后保持合法大快照未读。"""
        address = urlsplit(self.client.base)
        connection = http.client.HTTPConnection(address.hostname, address.port, timeout=WAIT_SECONDS)
        try:
            connection.connect()
            if connection.sock is None:
                raise AssertionError('慢下载连接未建立')
            connection.sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4096)
            cookie = '; '.join(value.name + '=' + value.value for value in self.client.cookies)
            connection.request('GET', '/api/backup', headers={'Cookie': cookie, 'Connection': 'close'})
            response = connection.getresponse()
            self.check(response.status == 200 and int(response.getheader('Content-Length', '0')) > 8 * 1024 * 1024 and bool(response.read(1)), '真实慢HTTP下载已进入大快照正文')
            return connection, response
        except BaseException:
            connection.close()
            raise

    def download(self, name: str) -> dict[str, object]:
        """分块保存真实HTTP快照，再独立只读核对SQLite、schema、大小与摘要。"""
        target = self.root / name
        request = urllib.request.Request(self.client.base + '/api/backup')
        digest = hashlib.sha256()
        total = 0
        with self.client.opener.open(request, timeout=WAIT_SECONDS) as response, target.open('xb') as output:
            self.check(response.status == 200 and response.headers.get_content_type() == 'application/vnd.sqlite3' and re.fullmatch(r'attachment; filename=nas-library-\d{8}-\d{6}\.db', response.headers['Content-Disposition']) is not None, '下载保持SQLite类型与原有本地时间文件名格式')
            length = int(response.headers['Content-Length'])
            while block := response.read(65536):
                total += len(block)
                if total > DOWNLOAD_LIMIT:
                    raise AssertionError('测试下载超出固定预算')
                output.write(block)
                digest.update(block)
        self.check(total == length == target.stat().st_size, 'HTTP长度、实际写入和独立文件长度完全一致')
        before = (target.stat().st_size, target.stat().st_mtime_ns, file_identity(target)['sha256'])
        members = sorted(path.name for path in self.root.iterdir())
        with closing(sqlite3.connect(target.as_uri() + '?mode=ro&immutable=1', uri=True)) as database:
            self.check(database.execute('PRAGMA user_version').fetchone() == (6,), '下载快照实际schema为6，未隐式迁移')
            self.check(database.execute('PRAGMA integrity_check(1)').fetchall() == [('ok',)] and not database.execute('PRAGMA foreign_key_check').fetchall(), '独立SQLite完整性与外键检查全部通过')
            pages = database.execute('PRAGMA page_count').fetchone()[0]
            size = database.execute('PRAGMA page_size').fetchone()[0]
            self.check(pages * size == total and database.execute('SELECT count(*) FROM backup_payload').fetchone() == (PAYLOAD_ROWS,), '实际页长度与完整大负载均在快照内')
            row = database.execute('SELECT position,total,locator FROM progress WHERE user_id=1 ORDER BY item_id LIMIT 1').fetchone()
            self.check(row is not None and row[1] == row[0] + 100 and row[2] == 'step:' + str(int(row[0])), '并发进度提交在快照中保持同一业务事务的字段关系')
            self.check(database.execute('SELECT favorite FROM user_items WHERE user_id=1').fetchone()[0] in (0, 1), '并发收藏提交在快照中为完整合法状态')
        after = (target.stat().st_size, target.stat().st_mtime_ns, file_identity(target)['sha256'])
        self.check(before == after and members == sorted(path.name for path in self.root.iterdir()) and digest.hexdigest() == after[2], '独立只读检查未改字节时间或目录成员，流式摘要相同')
        return {'file': name, 'sizeBytes': total, 'sha256': digest.hexdigest(), 'schemaVersion': 6, 'scope': 'database-only'}


def run_suite(binary: Path, root: Path, console: WindowsConsole | None) -> None:
    """执行实际HTTP竞争、并发状态、未知哨兵、正常停服与立即重启。"""
    harness = BackupHarness(binary, root, console)
    sentinel: OwnedProcess | None = None
    completed = False
    snapshots: list[dict[str, object]] = []
    artifact_before = file_identity(binary)
    slow: tuple[http.client.HTTPConnection, http.client.HTTPResponse] | None = None
    writer: threading.Thread | None = None
    stop = threading.Event()
    writer_failures: list[str] = []
    writes: list[int] = []
    try:
        sentinel = OwnedProcess([sys.executable, '-B', '-c', 'import time; time.sleep(240)'], clean_environment(), root / 'logs/sentinel.log')
        harness.start()
        anonymous = Client(harness.client.base)
        anonymous.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        harness.check(anonymous.request('/api/backup')[0] == 401 and anonymous.request('/api/backup', 'HEAD')[0] == 401, '备份GET和HEAD继续要求鉴权')
        password = generate_password()
        harness.secret_values.append(password.encode())
        harness.client.json('/api/setup', 'POST', {'username': 'backup-verify', 'password': password})
        harness.secret_values.extend(cookie.value.encode() for cookie in harness.client.cookies)
        harness.secret_values.extend(hashlib.sha256(cookie.value.encode()).hexdigest().encode() for cookie in harness.client.cookies)
        (root / 'media/快照小说.txt').write_text('隔离备份验证。\n' * 40, encoding='utf-8')
        harness.client.json('/api/libraries', 'POST', {'name': '备份验证', 'type': 'novel', 'path': str(root / 'media')})
        deadline = time.monotonic() + WAIT_SECONDS
        items: list[dict[str, object]] = []
        while time.monotonic() < deadline:
            items = harness.client.json('/api/items')['items']
            if items:
                break
            time.sleep(0.03)
        harness.check(bool(items), '隔离媒体通过真实扫描入库')
        path = '/api/items/' + str(items[0]['id'])
        harness.client.json(path + '/favorite', 'POST', {'favorite': True})
        harness.client.json(path + '/progress', 'PUT', {'position': 0, 'total': 100, 'locator': 'step:0'})
        harness.close_process(True)
        with closing(sqlite3.connect(root / 'data/library.db')) as database:
            database.execute('CREATE TABLE backup_payload(id INTEGER PRIMARY KEY, payload BLOB NOT NULL)')
            database.executemany('INSERT INTO backup_payload(payload) VALUES(?)', ((secrets.token_bytes(65536),) for _ in range(PAYLOAD_ROWS)))
            database.commit()
            harness.secret_values.extend(str(row[0]).encode() for row in database.execute('SELECT password_hash FROM users UNION ALL SELECT token_hash FROM sessions'))
        unknown = root / 'data/.backup-pending-sentinel'
        unknown.mkdir()
        (unknown / 'sentinel').write_bytes(b'keep-unknown-backup-material')
        harness.start()
        writer_client = Client(harness.client.base)
        writer_client.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(harness.client.cookies))

        def update_personal_state() -> None:
            """真实提交收藏和进度；每次进度请求的三个字段必须原子可见。"""
            try:
                for number in range(1, 101):
                    if stop.is_set():
                        break
                    writer_client.json(path + '/favorite', 'POST', {'favorite': bool(number % 2)})
                    writer_client.json(path + '/progress', 'PUT', {'position': number, 'total': number + 100, 'locator': 'step:' + str(number)})
                    writes.append(number)
            except BaseException:
                writer_failures.append('真实并发个人状态提交失败')

        writer = threading.Thread(target=update_personal_state)
        writer.start()
        snapshots.append(harness.download('snapshot-1.db'))
        stop.set()
        writer.join(timeout=WAIT_SECONDS)
        harness.check(not writer.is_alive() and not writer_failures and bool(writes), '真实收藏与进度写请求完成，单连接等待未被冒充并行SQL')
        harness.wait_clean()
        code, headers, body = harness.client.request('/api/backup', 'HEAD')
        harness.check(code == 200 and not body and int(headers['Content-Length']) > 8 * 1024 * 1024 and headers.get_content_type() == 'application/vnd.sqlite3', 'HEAD继续生成经校验的快照并保持精确长度与空正文')
        harness.wait_clean()
        slow = harness.slow_download()
        started = time.monotonic()
        harness.check(harness.client.request('/api/backup')[0] == 409 and time.monotonic() - started < 2.0, '慢下载持有共用租约时第二个GET立即409')
        slow[1].close()
        slow[0].close()
        slow = None
        harness.wait_clean()
        snapshots.append(harness.download('snapshot-2.db'))
        harness.wait_clean()
        slow = harness.slow_download()
        harness.check(harness.client.request('/api/backup')[0] == 409, '正常停服前真实慢下载仍持有租约')
        old_base = harness.client.base
        harness.close_process(True)
        slow[1].close()
        slow[0].close()
        slow = None
        harness.wait_clean()
        if os.name == 'nt':
            from verify_desktop import assert_exclusive
            assert_exclusive(root / 'data/library.db')
            assert_exclusive(root / 'data/.nas-instance.lock')
            harness.check(True, '正常停服后数据库与数据锁均可立即零共享独占打开')
        address = urlsplit(old_base)
        try:
            with socket.create_connection((str(address.hostname), int(address.port or 0)), timeout=0.3):
                closed = False
        except OSError:
            closed = True
        harness.check(closed, '正常停服后旧HTTP监听已释放')
        harness.check(sentinel.process.poll() is None, '正常停服未结束独立哨兵进程')
        harness.check((unknown / 'sentinel').read_bytes() == b'keep-unknown-backup-material', '未知pending哨兵未被生成、清理或停服扫描删除')
        harness.start()
        harness.check(harness.client.json('/api/status')['user'] is not None, '慢下载正常停服后同数据根立即重启且原会话仍有效')
        harness.close_process(True)
        harness.check(file_identity(binary) == artifact_before, '全程使用同一产物，二进制身份和摘要未变化')
        completed = True
    finally:
        stop.set()
        cleanup_failures: list[str] = []
        try:
            if writer is not None:
                writer.join(timeout=WAIT_SECONDS)
        except BaseException:
            cleanup_failures.append('并发提交线程等待失败')
        if slow is not None:
            for resource in (slow[1], slow[0]):
                try:
                    resource.close()
                except BaseException:
                    cleanup_failures.append('慢下载连接关闭失败')
        try:
            harness.close_process(False)
        except BaseException:
            cleanup_failures.append('应用强制兜底或原始日志收尾失败')
        try:
            if sentinel is not None:
                sentinel.stop()
        except BaseException:
            cleanup_failures.append('自有哨兵进程回收失败')
        finally:
            try:
                if writer is not None:
                    writer.join(timeout=5)
                    if writer.is_alive():
                        cleanup_failures.append('并发提交线程未退出')
            except BaseException:
                cleanup_failures.append('并发提交线程最终等待失败')
            write_json(root / 'assertions.json', harness.assertions)
            write_json(root / 'result.json', {'passed': completed and not cleanup_failures, 'assertions': len(harness.assertions), 'artifact': file_identity(binary), 'snapshots': snapshots, 'normalStopMechanism': 'isolated-console-CTRL_C_EVENT' if os.name == 'nt' else 'owned-session-SIGINT', 'applicationStarts': harness.starts, 'normalStops': harness.normal_stops, 'forcedApplicationReclaims': harness.forced_reclaims, 'rawLogsCheckedBeforeSanitize': harness.raw_checked == harness.starts, 'cleanupFailures': cleanup_failures, 'targetNAS': False, 'limits': ['database-only，不包含外部来源秘密文件、helper/worker身份文件、组件卷、载荷、媒体和用户原件；数据库本身含敏感索引、账号口令/会话哈希与私密引用，必须私密保管', '单连接VACUUM时其它SQL可等待；未证明真正并行写入', 'Windows本机验证不代表真实Linux/NAS权限、大库性能、断电或FULL/拔盘']})
        if cleanup_failures:
            raise AssertionError('自有资源收尾失败，失败证据已保留')
    print('备份集成验证通过：' + str(len(harness.assertions)) + ' 项断言', flush=True)


def main() -> int:
    """验证全新根和固定产物；Windows仅在自有隐藏Console广播真实CTRL_C。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True, help='明确指定已构建的主服务产物，全程不自行构建')
    parser.add_argument('--root', help='尚不存在的.runtime/verify后代目录；已有证据绝不覆盖')
    parser.add_argument('--console-helper', action='store_true', help=argparse.SUPPRESS)
    args = parser.parse_args()
    if os.name == 'nt' and not args.console_helper:
        owned = RawProcess([sys.executable, '-B', str(Path(__file__).resolve()), *sys.argv[1:], '--console-helper'], clean_environment(), console=True)
        try:
            code = owned.wait(timeout=180)
            output = b''.join(bytes(block) for block in owned.raw)
            if not raw_has_no_secrets([output], []):
                raise AssertionError('专属Console helper输出未通过秘密检查')
            print(output.decode('utf-8', 'replace').replace('\r\n', '\n'), end='', flush=True)
            return code
        finally:
            owned.close()
    binary = args.binary.resolve()
    if not binary.is_file():
        raise Blocked('缺少明确指定的主服务产物')
    root = new_run_root(args.root, prefix='backup')
    console: WindowsConsole | None = WindowsConsole() if os.name == 'nt' else None
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
    except (OSError, ValueError, RuntimeError, AssertionError, http.client.HTTPException) as error:
        print('备份集成验证失败：' + type(error).__name__, file=sys.stderr, flush=True)
        raise SystemExit(2 if isinstance(error, Blocked) else 1)
