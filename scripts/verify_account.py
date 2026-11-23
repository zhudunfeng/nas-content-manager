"""使用独立实际应用验证账号迁移、严格接口、改密、会话与限流；不访问真实账号。"""

from __future__ import annotations

import sys

if __name__ == '__main__':
    sys.dont_write_bytecode = True

import argparse
from contextlib import closing
from datetime import datetime, timedelta, timezone
import hashlib
import http.client
import json
from pathlib import Path
import secrets
import socket
import sqlite3
import string
import time
import urllib.error
import urllib.request
from urllib.parse import urlsplit

from verify import Client, generate_password
from verify_baseline import Blocked, OwnedProcess, clean_environment, file_identity, new_run_root, reserve_port, wait_ready, write_json
from verify_schema import create_legacy_database


# 以下为公开固定旧密码夹具的 bcrypt 摘要，不是任何真实账号凭据；新实例仅在回环隔离目录使用。
LEGACY_PASSWORDS: tuple[tuple[str, str, str], ...] = (
    ('weak', 'legacyweak', '$2a$10$heIZqR3QrPAnAj/lNt7J7uSPefeGBQ2lfu.7maap4FUuxBgmcTiD.'),
    ('chinese', '旧密码中文', '$2a$10$xSM5LoWqc7zyoJNVIuPa0em0z4iHbq9zRSTar2D8EH8zxafLsUE5e'),
)


class AccountHarness:
    """复用基线的安全进程所有权、随机就绪挑战与脱敏日志，管理一个隔离账号场景。"""

    def __init__(self, binary: Path, root: Path, assertions: list[str]) -> None:
        """准备专属目录与前端探针，拒绝继承真实应用配置。"""
        self.binary: Path = binary  # 已构建主服务，不自行构建或安装依赖。
        self.root: Path = root  # 当前唯一场景根，父运行根已经严格验证。
        self.assertions: list[str] = assertions  # 本次实际完成的安全中文断言。
        self.owned: OwnedProcess | None = None  # 自有进程树，所有失败路径都必须回收。
        self.starts: int = 0  # 启动编号，避免覆盖重启前日志。
        self.client: Client = Client('http://127.0.0.1')  # Cookie 只留内存并跨本场景重启保留。
        self.client.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(self.client.cookies))
        self.marker: str = 'account-' + secrets.token_hex(12) + '.txt'  # 本实例专属就绪路径。
        self.challenge: bytes = secrets.token_hex(24).encode('ascii')  # 就绪内容只用于证明实例归属。
        for name in ('data', 'media', 'web', 'logs'):
            (root / name).mkdir(parents=True)
        (root / 'web/index.html').write_text('<!doctype html><title>隔离账号验证</title>', encoding='utf-8')
        (root / 'web' / self.marker).write_bytes(self.challenge)

    def check(self, condition: bool, label: str) -> None:
        """仅在断言实际完成后递增计数，失败只报告固定中文说明。"""
        if not condition:
            raise AssertionError(label)
        self.assertions.append(label)
        print('  ✓ ' + label, flush=True)

    def start(self) -> None:
        """启动独立应用，必须通过自有进程、数据库和随机静态探针三重确认。"""
        if self.owned is not None:
            raise RuntimeError('隔离实例已经启动，须先停止才能重启')
        listener = reserve_port(0)
        port = int(listener.getsockname()[1])
        self.client.base = 'http://127.0.0.1:' + str(port)
        environment = clean_environment()
        environment.update({
            'NAS_ADDR': '127.0.0.1:' + str(port),  # 仅本次回环端口。
            'NAS_DATA_DIR': str(self.root / 'data'),  # 仅本次 SQLite 和派生缓存。
            'NAS_MEDIA_ROOTS': str(self.root / 'media'),  # 只授权隔离媒体根。
            'NAS_WEB_DIR': str(self.root / 'web'),  # 本次固定静态探针。
            'NAS_SCAN_INTERVAL_MINUTES': '60',  # 测试期间不以短周期自动扫描。
            'NAS_MIN_FREE_MB': '0',  # 小型隔离夹具不要求生产预留空间。
            'NAS_COOKIE_SECURE': 'false',  # 仅回环 HTTP 夹具允许普通 Cookie。
            'NAS_DOWNLOAD_MODE': 'disabled',  # 不连接真实下载器。
            'NAS_TORRENT_SEARCH_MODE': 'disabled',  # 不访问真实搜索站。
        })
        self.starts += 1
        try:
            listener.close()
            self.owned = OwnedProcess([str(self.binary)], environment, self.root / 'logs' / ('application-' + str(self.starts) + '.log'))
            wait_ready(self.owned, self.client.base, self.marker, self.challenge, 20)
        finally:
            listener.close()

    def stop(self) -> None:
        """停止并等待本场景的自有进程树；清理错误直接使整个验证失败。"""
        if self.owned is not None:
            owned, self.owned = self.owned, None
            owned.stop()

    def database(self) -> sqlite3.Connection:
        """只在隔离场景读取或明确改造夹具数据，不连接用户数据库。"""
        return sqlite3.connect(self.root / 'data/library.db')

    def raw_request(self, path: str, body: bytes, content_type: str = 'application/json') -> tuple[int, object, bytes]:
        """发送未被字典序列化修正的有界实际请求，保留重复键等非法输入。"""
        request = urllib.request.Request(self.client.base + path, data=body, headers={'Origin': self.client.base, 'Content-Type': content_type}, method='POST')
        try:
            response = self.client.opener.open(request, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            result = response.read(65537)
            if len(result) > 65536:
                raise AssertionError('账号响应超出有界验证预算')
            return response.code, response.headers, result

    def wire_account_request(self, framing: list[tuple[str, str]], fragments: list[bytes], pause: float = 0.0, end_write: bool = False) -> tuple[int, bytes, float]:
        """只向自有实例分片发送账号拒绝夹具，有界读取实际响应且不重试传输错误。"""
        address = urlsplit(self.client.base)
        if address.hostname != '127.0.0.1' or address.port is None:
            raise AssertionError('账号原始请求只允许当前隔离回环实例')
        headers = [('Host', address.netloc), ('Origin', self.client.base), ('Content-Type', 'application/json'), ('Connection', 'close'), ('Cookie', '; '.join(cookie.name + '=' + cookie.value for cookie in self.client.cookies)), *framing]
        with socket.create_connection((address.hostname, address.port), timeout=3) as connection:
            started = time.monotonic()
            connection.sendall(('POST /api/account/password HTTP/1.1\r\n' + '\r\n'.join(name + ': ' + value for name, value in headers) + '\r\n\r\n').encode('ascii'))
            for index, fragment in enumerate(fragments):
                if index and pause:
                    time.sleep(pause)
                connection.sendall(fragment)
            if end_write:
                connection.shutdown(socket.SHUT_WR)
            with connection.makefile('rb') as stream:
                status_line = stream.readline(4097)
                status = status_line.split(b' ', 2)
                if len(status_line) > 4096 or len(status) != 3 or status[0] not in (b'HTTP/1.0', b'HTTP/1.1'):
                    raise AssertionError('账号原始响应状态行无效或超出预算')
                code = int(status[1])
                if code == 100:
                    raise AssertionError('账号拒绝不应发送100继续')
                response_headers: dict[bytes, bytes] = {}  # 只保留当前有界响应头，用于精确读取固定正文。
                header_bytes = 0
                for _ in range(32):
                    line = stream.readline(8193)
                    header_bytes += len(line)
                    if not line or header_bytes > 8192:
                        raise AssertionError('账号原始响应头不完整或超出预算')
                    if line == b'\r\n':
                        break
                    name, separator, value = line.partition(b':')
                    if not separator:
                        raise AssertionError('账号原始响应头格式无效')
                    response_headers[name.lower()] = value.strip()
                else:
                    raise AssertionError('账号原始响应头数量超出预算')
                length = int(response_headers.get(b'content-length', b'-1'))
                if not 0 <= length <= 4096:
                    raise AssertionError('账号原始响应缺少有界正文长度')
                body = stream.read(length)
                if len(body) != length:
                    raise AssertionError('账号原始响应正文不完整')
                return code, body, time.monotonic() - started


def verify_legacy(binary: Path, root: Path, assertions: list[str], label: str, password: str, password_hash: str) -> None:
    """构造真正 schema4 并验证旧密码、未到期会话、精确修订与重启兼容。"""
    harness = AccountHarness(binary, root, assertions)
    old_token = secrets.token_hex(32)
    expired_token = secrets.token_hex(32)
    old_digest = hashlib.sha256(old_token.encode()).hexdigest()
    expires = (datetime.now(timezone.utc) + timedelta(days=2)).isoformat(timespec='seconds').replace('+00:00', 'Z')
    create_legacy_database(root / 'data/library.db', 4)
    with closing(harness.database()) as database:
        database.execute('INSERT INTO users(id,username,password_hash) VALUES(1,?,?)', ('account-verify', password_hash))
        database.executemany('INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,1,?)', ((old_digest, expires), (hashlib.sha256(expired_token.encode()).hexdigest(), '2001-01-01T00:00:00Z')))
        database.commit()
        harness.check(database.execute('PRAGMA user_version').fetchone()[0] == 4 and len(database.execute('PRAGMA table_info(sessions)').fetchall()) == 3, label + '：真实旧库结构未混入新账号字段')
    try:
        harness.start()
        harness.check(harness.client.request('/api/stats', headers={'Cookie': 'nas_session=' + old_token})[0] == 200, label + '：迁移后旧未到期 Cookie 仍可鉴权')
        harness.check(harness.client.request('/api/stats', headers={'Cookie': 'nas_session=' + expired_token})[0] == 401, label + '：旧过期 Cookie 不能鉴权')
        with closing(harness.database()) as database:
            version = database.execute('PRAGMA user_version').fetchone()[0]
            account = database.execute('SELECT password_hash,auth_revision FROM users WHERE id=1').fetchone()
            sessions = database.execute('SELECT session_id,created_at,client_browser,client_os,auth_revision FROM sessions').fetchall()
            original = database.execute('SELECT expires_at FROM sessions WHERE token_hash=?', (old_digest,)).fetchone()
            integrity = database.execute('PRAGMA integrity_check').fetchone()[0] == 'ok' and database.execute('PRAGMA foreign_key_check').fetchone() is None
        harness.check(version == 6 and account == (password_hash, 0) and original == (expires,), label + '：schema6 保留旧密码摘要、到期原文与零修订')
        harness.check(integrity, label + '：迁移后 SQLite 结构与外键完整性通过')
        harness.check(len(sessions) == 2 and len({row[0] for row in sessions}) == 2 and all(len(row[0]) == 32 and row[1:] == (None, 'unknown', 'unknown', 0) for row in sessions), label + '：旧行独立补号，未知创建时间和客户端未伪造')
        credentials = {'username': 'account-verify', 'password': password}
        harness.check(harness.client.request('/api/login', 'POST', dict(credentials, password=password + '-wrong'))[0] == 401, label + '：旧账号错误密码仍拒绝')
        code, headers, body = harness.client.request('/api/login', 'POST', credentials, {'User-Agent': 'Mozilla/5.0 Linux Firefox/141.0 account-private-ua'})
        harness.check(code == 200 and b'account-verify' in body and 'HttpOnly' in headers.get('Set-Cookie', '') and 'SameSite=Strict' in headers.get('Set-Cookie', ''), label + '：旧密码实际登录成功并设置安全 Cookie')
        token = next(cookie.value for cookie in harness.client.cookies if cookie.name == 'nas_session')
        digest = hashlib.sha256(token.encode()).hexdigest()
        with closing(harness.database()) as database:
            metadata = database.execute('SELECT session_id,created_at,client_browser,client_os,auth_revision FROM sessions WHERE token_hash=?', (digest,)).fetchone()
        harness.check(metadata is not None and len(metadata[0]) == 32 and metadata[0] not in (token[:32], digest[:32]) and metadata[1] is not None and metadata[2:] == ('firefox', 'linux', 0), label + '：新会话编号与令牌独立，客户端只存固定类别')
        harness.check(all(secret.encode() not in body for secret in (password, password_hash, token, digest, 'account-private-ua')), label + '：登录公开结果不含密码、令牌、摘要或原 UA')
        harness.stop()
        harness.start()
        harness.check(harness.client.request('/api/stats')[0] == 200 and harness.client.request('/api/stats', headers={'Cookie': 'nas_session=' + old_token})[0] == 200, label + '：应用重启后新旧持久会话均保留')
        with closing(harness.database()) as database:
            actual = database.execute('SELECT session_id,created_at,client_browser,client_os,auth_revision FROM sessions WHERE token_hash=?', (digest,)).fetchone()
            database.execute('UPDATE users SET auth_revision=auth_revision+1 WHERE id=1')
            database.commit()
        harness.check(actual == metadata, label + '：重启未重编公开编号或创建时间')
        harness.check(harness.client.request('/api/stats')[0] == 401 and harness.client.request('/api/stats', headers={'Cookie': 'nas_session=' + old_token})[0] == 401, label + '：夹具推进修订后全部旧修订会话立即失效')
        harness.check(harness.client.request('/api/login', 'POST', credentials)[0] == 200 and harness.client.request('/api/stats')[0] == 200, label + '：当前密码仍可建立绑定新修订的会话')
        latest_token = next(cookie.value for cookie in harness.client.cookies if cookie.name == 'nas_session')
        upgraded_password = generate_password()
        result = harness.client.json('/api/account/password', 'POST', {'currentPassword': password, 'newPassword': upgraded_password})
        harness.check(result == {'requiresLogin': True} and harness.client.request('/api/stats', headers={'Cookie': 'nas_session=' + latest_token})[0] == 401, label + '：旧弱或中文原密码可实际升级并撤销原会话')
        harness.check(harness.client.request('/api/login', 'POST', credentials)[0] == 401 and harness.client.request('/api/login', 'POST', dict(credentials, password=upgraded_password))[0] == 200, label + '：升级后旧密码拒绝且新密码可登录')
    finally:
        harness.stop()
    logs = '\n'.join(path.read_text(encoding='utf-8') for path in (root / 'logs').glob('*.log'))
    harness.check(all(secret not in logs for secret in (password, upgraded_password, password_hash, old_token, expired_token, token, latest_token, old_digest, digest, 'account-private-ua')), label + '：可分享脱敏日志不含凭据或原 UA')


def verify_mutations(binary: Path, root: Path, assertions: list[str]) -> None:
    """验证实际在线账号接口、严格输入、空批次分页及提交后的旧凭据失效。"""
    harness = AccountHarness(binary, root, assertions)
    credentials = {'username': 'account-mutations', 'password': generate_password()}
    updated_password = 'Aa1' + string.punctuation + secrets.token_hex(8)
    try:
        harness.start()
        harness.client.json('/api/setup', 'POST', credentials)
        first_token = next(cookie.value for cookie in harness.client.cookies if cookie.name == 'nas_session')
        first = harness.client.json('/api/account/sessions')
        harness.check(set(first) == {'currentSession', 'items', 'nextCursor'} and first['items'] == [] and first['nextCursor'] is None and first['currentSession']['current'] is True, '账号接口：唯一实际会话固定返回，空数组与末批游标准确')
        harness.client.json('/api/login', 'POST', credentials)
        second_token = next(cookie.value for cookie in harness.client.cookies if cookie.name == 'nas_session')
        page = harness.client.json('/api/account/sessions')
        public_fields = {'id', 'current', 'clientBrowser', 'clientOS', 'createdAt', 'expiresAt'}
        harness.check(len(page['items']) == 1 and page['items'][0]['id'] == first['currentSession']['id'] and page['currentSession']['id'] != first['currentSession']['id'] and all(set(item) == public_fields for item in [page['currentSession'], *page['items']]) and all(not item['current'] for item in page['items']), '账号接口：当前标记来自实际 Cookie，其他会话与精确公开字段独立')
        private_values = (credentials['password'], first_token, second_token, hashlib.sha256(first_token.encode()).hexdigest(), hashlib.sha256(second_token.encode()).hexdigest())
        harness.check(all(secret not in json.dumps(page) for secret in private_values), '账号接口：会话 DTO 不含密码、Cookie 或令牌摘要')
        with closing(harness.database()) as database:
            account_before = database.execute('SELECT password_hash,auth_revision FROM users WHERE id=1').fetchone()
        invalid_json = [b'null', b'[]', b'{}', b'{"CurrentPassword":"x","newPassword":"Aa1!abcdef"}', b'{"currentPassword":null,"newPassword":"Aa1!abcdef"}', b'{"currentPassword":false,"newPassword":"Aa1!abcdef"}', b'{"currentPassword":{},"newPassword":"Aa1!abcdef"}', b'{"currentPassword":"x","newPassword":"Aa1!abcdef","extra":"x"}', b'{"currentPassword":"x","currentPassword":"y","newPassword":"Aa1!abcdef"}', b'{"currentPassword":"x","current\\u0050assword":"y","newPassword":"Aa1!abcdef"}', b'{"currentPassword":"x","newPassword":"Aa1!abcdef"}{}', b'{"currentPassword":"x","newPassword":"Aa1!abcdef"}true', b'{"currentPassword":"x","newPassword":"Aa1!abcdef"']
        harness.check(all(harness.raw_request('/api/account/password', body)[0] == 400 for body in invalid_json), '账号接口：实际 HTTP 拒绝大小写别名、重复或转义重复键、未知缺失空值类型与尾随数据')
        harness.check(harness.raw_request('/api/account/sessions/revoke-others', b'{"currentPassword":"x","current\\u0050assword":"y"}')[0] == 400 and harness.raw_request('/api/account/sessions/revoke-others', b'{"currentPassword":"x","newPassword":"x"}')[0] == 400, '账号接口：撤销其他会话也拒绝重复键和跨表单字段')
        invalid_passwords = ['Aa1!abcde', 'Aa1!' + 'x' * 69, 'aa1!abcdef', 'AA1!ABCDEF', 'Aa!!abcdef', 'Aa12abcdef', 'Aa1!中文', 'Aa1!abcdef中文', 'Aa1!abcdef𠀀', 'Aa1!abcdefＡ', 'Aa1!abcdef😀', ' Aa1!abcdef', 'Aa1!abcdef ', 'Aa1!abcdef\n', 'Aa1!abcdef\r\n', 'Aa1!abcdef\t', 'Aa1!abcdef\0']
        harness.check(all(harness.client.request('/api/account/password', 'POST', {'currentPassword': credentials['password'], 'newPassword': value})[0] == 400 for value in invalid_passwords), '账号接口：9/73字节、缺类、中文全角空白控制表情的新密码全部拒绝')
        harness.check(all(harness.client.request('/api/account/password', 'POST', {'currentPassword': value, 'newPassword': updated_password})[0] == 400 for value in ('x' * 73, '中' * 25)), '账号接口：原密码按 UTF-8 字节执行72上限，无新字符限制或截断')
        invalid_queries = ['limit=', 'limit=0', 'limit=101', 'limit=1&limit=2', 'pageSize=1', 'cursor=', 'cursor=v2.' + 'a' * 32, 'cursor=v1.' + 'A' * 32, 'cursor=v1.' + 'a' * 31, 'cursor=v1.' + 'a' * 32 + '&cursor=v1.' + 'b' * 32]
        harness.check(all(harness.client.request('/api/account/sessions?' + query)[0] == 400 for query in invalid_queries), '账号接口：拒绝无界大小、重复未知参数及非法游标')
        harness.check(all(harness.client.request('/api/account/sessions/' + value, 'DELETE')[0] == 400 for value in ('x', 'A' * 32, 'a' * 64)) and harness.client.json('/api/account/sessions/' + '0' * 32, 'DELETE') == {'requiresLogin': False} and harness.client.json('/api/account/sessions/' + '0' * 32, 'DELETE') == {'requiresLogin': False}, '账号接口：公开编号格式严格且未知重复撤销幂等')
        harness.check(harness.client.request('/api/account/password', 'POST', {'currentPassword': 'wrong-password', 'newPassword': updated_password})[0] == 400 and harness.client.request('/api/stats')[0] == 200, '账号接口：错误原密码为400，当前 Cookie 保留且不误退出')
        harness.check(harness.raw_request('/api/account/password', b'{}', 'text/plain')[0] == 415 and harness.raw_request('/api/account/password', b'{"currentPassword":"' + b'x' * 65536 + b'"}')[0] == 413, '账号接口：媒体类型415与超大正文413独立拒绝')
        harness.check(harness.client.request('/api/account/password', 'POST', {}, {'Origin': 'https://foreign.invalid'})[0] == 403 and harness.client.request('/api/account/password', 'POST', {}, {'Cookie': 'nas_session=' + 'f' * 64})[0] == 401, '账号接口：跨站403和匿名401保持统一安全边界')
        with closing(harness.database()) as database:
            unchanged = database.execute('SELECT password_hash,auth_revision FROM users WHERE id=1').fetchone() == account_before
            future = (datetime.now(timezone.utc) + timedelta(days=1)).isoformat().replace('+00:00', 'Z')
            records = [(hashlib.sha256(('pagination-fixture-' + str(number)).encode()).hexdigest(), format(number, '032x'), expiry, revision) for number, expiry, revision in ((1, '2000-01-01T00:00:00Z', 0), (2, future, 1), (3, future, 0), (4, 'not-a-time', 0))]
            database.executemany("INSERT INTO sessions(token_hash,user_id,session_id,expires_at,created_at,client_browser,client_os,auth_revision) VALUES(?,1,?,?,NULL,'unknown','unknown',?)", records)
            database.commit()
        harness.check(unchanged, '账号接口：全部输入拒绝未修改账号hash或修订')
        first_batch = harness.client.json('/api/account/sessions?limit=1')
        harness.check(first_batch['items'] == [] and first_batch['nextCursor'] == 'v1.' + format(1, '032x'), '账号接口：首批只有过期原始行时保持空批次及可续游标')
        with closing(harness.database()) as database:
            database.execute('DELETE FROM sessions WHERE session_id=?', (format(1, '032x'),))
            database.commit()
        second_batch = harness.client.json('/api/account/sessions?limit=1&cursor=' + first_batch['nextCursor'])
        third_batch = harness.client.json('/api/account/sessions?limit=1&cursor=' + second_batch['nextCursor'])
        harness.check(second_batch['items'] == [] and second_batch['nextCursor'] == 'v1.' + format(2, '032x') and len(third_batch['items']) == 1 and third_batch['items'][0]['id'] == format(3, '032x') and third_batch['items'][0]['createdAt'] is None and third_batch['items'][0]['current'] is False, '账号接口：游标原行删除仍可续，旧修订跳过，历史有效会话不伪造时间')
        with closing(harness.database()) as database:
            other_count = database.execute('SELECT count(*) FROM sessions WHERE session_id<>?', (page['currentSession']['id'],)).fetchone()[0]
        result = harness.client.json('/api/account/sessions/revoke-others', 'POST', {'currentPassword': credentials['password']})
        harness.check(result == {'requiresLogin': False, 'revokedCount': other_count} and other_count == 4 and harness.client.request('/api/stats')[0] == 200 and harness.client.request('/api/stats', headers={'Cookie': 'nas_session=' + first_token})[0] == 401, '账号接口：退出其他按实际删除行计数（含旧修订与坏到期），保留当前会话')
        harness.check(harness.client.json('/api/account/sessions/revoke-others', 'POST', {'currentPassword': credentials['password']}) == {'requiresLogin': False, 'revokedCount': 0}, '账号接口：无其他记录时再认证撤销返回零且保持当前')
        result = harness.client.json('/api/account/sessions/' + page['currentSession']['id'], 'DELETE')
        harness.check(result == {'requiresLogin': True} and harness.client.request('/api/stats')[0] == 401 and harness.client.request('/api/account/sessions/' + page['currentSession']['id'], 'DELETE', headers={'Cookie': 'nas_session=' + second_token})[0] == 401, '账号接口：撤销真实当前清 Cookie，死 Cookie 重复请求须重新登录')
        harness.client.json('/api/login', 'POST', credentials)
        third_token = next(cookie.value for cookie in harness.client.cookies if cookie.name == 'nas_session')
        result = harness.client.json('/api/account/password', 'POST', {'currentPassword': credentials['password'], 'newPassword': updated_password})
        with closing(harness.database()) as database:
            actual = database.execute('SELECT password_hash,auth_revision FROM users WHERE id=1').fetchone()
            session_count = database.execute('SELECT count(*) FROM sessions').fetchone()[0]
        harness.check(result == {'requiresLogin': True} and actual[0] != account_before[0] and actual[1] == 1 and session_count == 0 and harness.client.request('/api/stats', headers={'Cookie': 'nas_session=' + third_token})[0] == 401, '账号接口：包含全部32标点的新密码原子生效、推进修订并撤销全部旧会话')
        harness.check(harness.client.request('/api/login', 'POST', credentials)[0] == 401 and harness.client.request('/api/login', 'POST', dict(credentials, password=updated_password))[0] == 200, '账号接口：改密后原密码拒绝，32标点原值可实际登录')
        harness.stop()
        harness.start()
        harness.check(harness.client.request('/api/stats')[0] == 200 and harness.client.json('/api/account/sessions')['currentSession']['current'] is True, '账号接口：改密后新会话与公开身份跨应用重启保持')
        for length, boundary_password in ((72, 'Aa1!' + 'x' * 68), (10, 'Aa1!abcdef')):
            result = harness.client.json('/api/account/password', 'POST', {'currentPassword': updated_password, 'newPassword': boundary_password})
            harness.check(result == {'requiresLogin': True} and harness.client.request('/api/login', 'POST', dict(credentials, password=boundary_password))[0] == 200, '账号接口：' + str(length) + '字节新密码合法边界实际生效并可登录')
            updated_password = boundary_password
    finally:
        harness.stop()


def verify_reauth_limits(binary: Path, root: Path, assertions: list[str]) -> None:
    """实际比较成功与失败均计费，两个端点共享预算，换会话和成功登录不能绕过账号上限。"""
    harness = AccountHarness(binary, root, assertions)
    credentials = {'username': 'account-limits', 'password': generate_password()}
    try:
        harness.start()
        harness.client.json('/api/setup', 'POST', credentials)
        harness.check(all(harness.client.request('/api/account/password', 'POST', {'currentPassword': 'wrong-password', 'newPassword': generate_password()})[0] == 400 for _ in range(5)), '再认证预算：当前会话前五次错误比较按400拒绝')
        harness.check(harness.client.request('/api/account/sessions/revoke-others', 'POST', {'currentPassword': credentials['password']})[0] == 200, '再认证预算：第六次正确比较实际成功但仍消耗会话预算')
        code, headers, _ = harness.client.request('/api/account/password', 'POST', {'currentPassword': credentials['password'], 'newPassword': generate_password()})
        harness.check(code == 429 and 1 <= int(headers.get('Retry-After', '0')) <= 300 and harness.client.request('/api/stats')[0] == 200, '再认证预算：跨端点共享每会话六次预算，429给有界等待且不退出')
        harness.client.json('/api/login', 'POST', credentials)
        harness.check(all(harness.client.request('/api/account/sessions/revoke-others', 'POST', {'currentPassword': 'wrong-password'})[0] == 400 for _ in range(6)), '再认证预算：新会话可用剩余六次账号预算，成功登录不清空旧预算')
        harness.client.json('/api/login', 'POST', credentials)
        results = [harness.client.request('/api/account/sessions/revoke-others', 'POST', {'currentPassword': credentials['password']}) for _ in range(3)]
        harness.check(all(code == 429 and 1 <= int(headers.get('Retry-After', '0')) <= 300 for code, headers, _ in results) and harness.client.request('/api/stats')[0] == 200, '再认证预算：第三会话与重复请求均不能绕过账号每五分钟十二次限制')
    finally:
        harness.stop()


def verify_setup(binary: Path, root: Path, assertions: list[str]) -> None:
    """验证新库首次设置、重复设置和退出行为未被存储迁移改变。"""
    harness = AccountHarness(binary, root, assertions)
    try:
        harness.start()
        harness.check(harness.client.json('/api/status')['setupRequired'] is True, '新库：schema6 保持首次设置入口')
        harness.check(harness.client.request('/api/setup', 'POST', {'username': 'account-verify', 'password': 'legacyweak'})[0] == 400, '新库：弱密码不能创建新管理员')
        with closing(harness.database()) as database:
            empty = database.execute('SELECT count(*) FROM users').fetchone()[0] == 0 and database.execute('SELECT count(*) FROM sessions').fetchone()[0] == 0
        harness.check(empty, '新库：非法设置没有账号或会话副作用')
        credentials = {'username': 'account-verify', 'password': generate_password()}
        code, _, _ = harness.client.request('/api/setup', 'POST', credentials)
        harness.check(code == 201 and harness.client.request('/api/stats')[0] == 200, '新库：合法首次设置建立当前快照会话')
        with closing(harness.database()) as database:
            account = database.execute('SELECT password_hash,auth_revision FROM users WHERE id=1').fetchone()
        harness.check(all(harness.client.request('/api/setup', 'POST', dict(credentials, password=generate_password()))[0] == 409 for _ in range(20)), '新库：连续二十次重复设置均明确冲突')
        with closing(harness.database()) as database:
            unchanged = database.execute('SELECT password_hash,auth_revision FROM users WHERE id=1').fetchone() == account
        harness.check(unchanged, '新库：重复设置未覆盖原账号快照')
        harness.check(all(harness.client.request('/api/logout', 'POST', {})[0] == 200 and harness.client.request('/api/stats')[0] == 401 and harness.client.request('/api/login', 'POST', credentials)[0] == 200 and harness.client.request('/api/stats')[0] == 200 for _ in range(12)), '新库：十二轮退出与重新登录小正文均实际成功且会话状态正确')
        harness.check(harness.client.request('/api/logout', 'POST', {})[0] == 200 and harness.client.request('/api/stats')[0] == 401, '新库：退出仍撤销当前会话并清除 Cookie')
    finally:
        harness.stop()


def verify_rejections(binary: Path, root: Path, assertions: list[str]) -> None:
    """重复验证匿名、跨站及限流的小正文拒绝，连接重置必须作为真实失败暴露。"""
    harness = AccountHarness(binary, root, assertions)
    try:
        harness.start()
        harness.check(all(harness.client.request('/api/libraries', 'POST', {})[0] == 401 for _ in range(20)), '拒绝响应：二十次匿名小正文写入均实际返回 401')
        harness.check(all(harness.client.request('/api/libraries', 'POST', {}, {'Origin': 'https://foreign.invalid'})[0] == 403 for _ in range(20)), '拒绝响应：二十次跨站小正文写入均实际返回 403')
        credentials = {'username': 'missing-account', 'password': generate_password()}
        harness.check(all(harness.client.request('/api/login', 'POST', credentials)[0] == 401 for _ in range(12)), '拒绝响应：登录前十二次错误尝试仍按原规则返回 401')
        results = [harness.client.request('/api/login', 'POST', credentials) for _ in range(20)]
        harness.check(all(code == 429 and headers.get('Retry-After') == '300' for code, headers, _ in results), '拒绝响应：二十次限流小正文均返回 429 和重试时间')
        with closing(harness.database()) as database:
            empty = all(database.execute('SELECT count(*) FROM ' + table).fetchone()[0] == 0 for table in ('users', 'sessions', 'libraries'))
        harness.check(empty and harness.owned is not None and harness.owned.process.poll() is None and harness.client.request('/healthz')[0] == 200, '拒绝响应：全部拒绝无账号或媒体写入，进程仍存活且健康')
    finally:
        harness.stop()


def verify_oversized_requests(binary: Path, root: Path, assertions: list[str]) -> None:
    """真实验证账号超限响应与有界等待；保留每次传输失败，不把重置当作413。"""
    harness = AccountHarness(binary, root, assertions)
    records: list[dict[str, object]] = []  # 仅保存状态与耗时，不持久化请求正文或Cookie。
    completed = False  # 只有全部真实分支完成后才声明本组通过。

    def check_response(label: str, result: tuple[int, bytes, float], expected: int = 413, maximum: float = 2.0) -> None:
        """记录安全响应证据，并断言精确状态、固定错误与本地等待上界。"""
        code, body, duration = result
        records.append({'name': label, 'status': code, 'elapsedMs': round(duration * 1000, 3)})
        error = json.loads(body).get('error')
        harness.check(code == expected and duration < maximum and (expected != 413 or error == '请求内容超过大小限制'), label)

    try:
        harness.start()
        harness.client.json('/api/setup', 'POST', {'username': 'account-body-boundary', 'password': generate_password()})
        with closing(harness.database()) as database:
            before = (database.execute('SELECT password_hash,auth_revision FROM users WHERE id=1').fetchone(), database.execute('SELECT count(*) FROM sessions').fetchone()[0])
        oversized = b'{"currentPassword":"' + b'x' * 65536 + b'"}'
        statuses: list[int] = []  # 每项都是独立请求；遇到传输错误立即失败，不补发。
        for number in range(100):
            try:
                statuses.append(harness.raw_request('/api/account/password', oversized)[0])
            except (OSError, http.client.HTTPException) as error:
                records.append({'name': 'close-connection-repeat', 'failedRequest': number + 1, 'completedRequests': len(statuses), 'errorType': type(error).__name__, 'winerror': getattr(error, 'winerror', None)})
                raise
            if statuses[-1] != 413 or harness.client.request('/healthz')[0] != 200:
                raise AssertionError('账号超限重复请求未返回精确413或服务不健康')
        records.append({'name': 'close-connection-repeat', 'requests': len(statuses), 'status413': statuses.count(413), 'transportErrors': 0})
        harness.check(len(statuses) == 100 and all(code == 413 for code in statuses), '大正文边界：一百次实际关闭连接请求均精确413且逐次健康')
        check_response('大正文边界：分片间隔50毫秒的完整超限正文返回413', harness.wire_account_request([('Content-Length', str(len(oversized)))], [oversized[:16384], oversized[16384:]], pause=0.05))
        check_response('大正文边界：声明1TiB但停滞的发送方在局部预算后返回413', harness.wire_account_request([('Content-Length', str(1 << 40))], [b'x']))
        check_response('大正文边界：Expect等待正文时不发送100继续且立即413', harness.wire_account_request([('Content-Length', str(1 << 40)), ('Expect', '100-continue')], []), maximum=1.0)
        chunked = format(len(oversized), 'x').encode('ascii') + b'\r\n' + oversized + b'\r\n0\r\n\r\n'
        check_response('大正文边界：未知长度分块请求继续受原JSON64KiB限额约束', harness.wire_account_request([('Transfer-Encoding', 'chunked')], [chunked]))
        check_response('大正文边界：超限声明但提前EOF仍固定413并有界退出', harness.wire_account_request([('Content-Length', str(len(oversized)))], [b'{"currentPassword":"x'], end_write=True))
        boundary = b'{"currentPassword":"' + b'x' * 73 + b'","newPassword":"Aa1!abcdef"}'
        boundary += b' ' * (65536 - len(boundary))
        check_response('大正文边界：恰好64KiB正常解码后按原密码字节规则返回400', harness.wire_account_request([('Content-Length', str(len(boundary)))], [boundary]), expected=400)
        with closing(harness.database()) as database:
            after = (database.execute('SELECT password_hash,auth_revision FROM users WHERE id=1').fetchone(), database.execute('SELECT count(*) FROM sessions').fetchone()[0])
            libraries = database.execute('SELECT count(*) FROM libraries').fetchone()[0]
        harness.check(before == after and libraries == 0 and harness.client.request('/api/stats')[0] == 200 and harness.client.request('/healthz')[0] == 200, '大正文边界：所有拒绝保持账号会话与媒体状态并继续鉴权')
        completed = True
    finally:
        harness.stop()
        write_json(root / 'oversized-result.json', {'passed': completed, 'artifact': file_identity(binary), 'cases': records, 'limits': ['已知超限非Expect账号JSON局部读取250毫秒、最多64KiB加一个哨兵字节', 'Go原生限额关闭可能另有500毫秒收尾；本机回归不是任意网络绝对送达保证', '真实传输重置直接失败，不重试或折算为HTTP413']})


def main() -> None:
    """默认运行在线账号；显式offline时追加同一产物的离线与终端验证。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True, help='已构建的栖库主服务产物；不接受桌面启动器')  # 仅执行显式给定产物。
    parser.add_argument('--root', help='仓库 .runtime/verify 下尚不存在的隔离运行根')  # 使用基线相同的新根安全边界。
    parser.add_argument('--offline', action='store_true', help='在线通过后追加离线CLI/受限存储和本机终端验证')  # 默认只跑既有在线回归，完整baseline必须显式启用。
    args = parser.parse_args()
    binary = args.binary.resolve()
    if not binary.is_file():
        parser.error('--binary 必须是已构建的普通应用文件')
    try:
        root = new_run_root(args.root, 'account')
    except Blocked as error:
        parser.error(str(error))
    assertions: list[str] = []
    legacy_completed: list[str] = []  # 只有真实旧库场景全部通过后才对外声明其完成。
    completed = False
    online_completed = False  # 在线能力和完整组合结果分别记账。
    online_assertions = 0  # 不将子验证重复计入在线断言。
    offline_result: dict[str, object] | None = None  # 只有真实执行后才存在离线结果。
    try:
        for name, password, password_hash in LEGACY_PASSWORDS:
            verify_legacy(binary, root / ('legacy-' + name), assertions, '旧' + ('弱密码' if name == 'weak' else '中文密码'), password, password_hash)
            legacy_completed.append(name)
        verify_setup(binary, root / 'fresh', assertions)
        verify_rejections(binary, root / 'rejections', assertions)
        verify_oversized_requests(binary, root / 'oversized', assertions)
        verify_mutations(binary, root / 'mutations', assertions)
        verify_reauth_limits(binary, root / 'reauth-limits', assertions)
        online_completed = True
        online_assertions = len(assertions)
        if args.offline:
            from verify_account_offline import run_offline_suite

            offline_root = root / 'offline'
            offline_root.mkdir()
            offline_result = run_offline_suite(binary, offline_root)
        completed = True
    finally:
        offline_path = root / 'offline/result.json'
        if args.offline and offline_result is None and offline_path.is_file():
            offline_result = json.loads(offline_path.read_text(encoding='utf-8'))
        offline_assertions = int(offline_result.get('assertions', 0)) if offline_result is not None else 0
        offline_passed = offline_result is not None and offline_result.get('passed') is True
        if not online_completed:
            online_assertions = len(assertions)
        write_json(root / 'assertions.json', assertions)
        write_json(root / 'result.json', {'passed': completed, 'assertions': online_assertions + offline_assertions, 'onlineAssertions': online_assertions, 'offlineAssertions': offline_assertions, 'realApplication': completed, 'artifact': file_identity(binary), 'trueSchema4Migration': len(legacy_completed) == len(LEGACY_PASSWORDS), 'legacyPasswordCases': legacy_completed, 'accountMutationAPIs': online_completed, 'offlineCLI': bool(args.offline and offline_passed), 'offlineVerification': {'requested': args.offline, 'passed': offline_passed, 'assertions': offline_assertions, 'resultPath': 'offline/result.json' if args.offline else None}, 'realExternalInstances': False, 'targetNAS': False, 'limits': ['默认仅验证在线；完整账号组合须显式--offline，并以offlineVerification和离线result为准', '在线日志已脱敏，原始秘密扫描另有临时Go证据；离线双流/Console原文扫描由离线验证器执行', 'ACL和Console覆盖以同次离线evidence为准；Windows本机运行不代表Linux或真实NAS验收']})
    print('账号集成验证通过：' + str(online_assertions + offline_assertions) + ' 项断言', flush=True)


if __name__ == '__main__':
    from project_env import run_cli

    raise SystemExit(run_cli(main))
