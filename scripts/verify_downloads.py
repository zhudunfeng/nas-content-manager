"""启动隔离应用和可控假 qB，验证下载编排；不运行 Docker，不代表真实 BT 传输。"""

from __future__ import annotations

import argparse
from dataclasses import dataclass, field
from email import policy
from email.parser import BytesParser
import hashlib
import http.server
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import sqlite3
import subprocess
import threading
import time
from typing import Any, Callable
import urllib.error
import urllib.parse
import urllib.request
import wave

from verify import Client, make_png


def bencode(value: Any) -> bytes:
    """编码受控测试种子，字典采用 BEP 规定的字节排序。"""
    if isinstance(value, bytes):
        return str(len(value)).encode() + b':' + value
    if isinstance(value, int):
        return b'i' + str(value).encode() + b'e'
    if isinstance(value, list):
        return b'l' + b''.join(bencode(part) for part in value) + b'e'
    if isinstance(value, dict):
        return b'd' + b''.join(bencode(key) + bencode(value[key]) for key in sorted(value)) + b'e'
    raise TypeError('测试种子包含不支持的类型')


def bdecode(data: bytes) -> Any:
    """仅解码本脚本生成的小种子，避免依赖第三方 bencode 包。"""
    def read(offset: int) -> tuple[Any, int]:
        """按偏移读取一个有界 bencode 值。"""
        prefix = data[offset:offset + 1]
        if prefix == b'i':
            end = data.index(b'e', offset)
            return int(data[offset + 1:end]), end + 1
        if prefix in (b'd', b'l'):
            result: Any = {} if prefix == b'd' else []
            offset += 1
            while data[offset:offset + 1] != b'e':
                value, offset = read(offset)
                if prefix == b'd':
                    result[value], offset = read(offset)
                else:
                    result.append(value)
            return result, offset + 1
        colon = data.index(b':', offset)
        length = int(data[offset:colon])
        return data[colon + 1:colon + 1 + length], colon + 1 + length
    value, end = read(0)
    assert end == len(data), '测试种子存在尾随数据'
    return value


@dataclass
class Fixture:
    """持有本机生成的有限种子、身份和文件内容。"""

    # 种子顶层名称，不能由测试外部来源控制。
    name: str
    # 相对路径和文件内容，所有数据均为隔离样本。
    files: list[tuple[str, bytes]]
    # 种子编码模式，支持 v1、v2 与混合身份验证。
    mode: str = 'v1'
    # 按需生成的完整 .torrent 内容。
    torrent: bytes = field(init=False)
    # 完整 SHA-1 身份，纯 v2 时为空。
    v1: str = field(init=False)
    # 完整 SHA-256 身份，纯 v1 时为空。
    v2: str = field(init=False)
    # qB 使用的不透明 hash，纯 v2 模拟其截短内部标识。
    hash: str = field(init=False)

    def __post_init__(self) -> None:
        """生成具有正确原始 info 摘要的标准种子样本。"""
        info: dict[bytes, Any] = {b'name': self.name.encode(), b'piece length': 16384}
        payload = b''.join(content for _, content in self.files)
        if self.mode != 'v2':
            info[b'pieces'] = b''.join(hashlib.sha1(payload[index:index + 16384]).digest() for index in range(0, len(payload), 16384))
            info[b'files'] = [{b'length': len(content), b'path': [part.encode() for part in path.split('/')]} for path, content in self.files]
        if self.mode != 'v1':
            info[b'meta version'] = 2
            tree: dict[bytes, Any] = {}
            for path, content in self.files:
                node = tree
                for part in path.split('/'):
                    node = node.setdefault(part.encode(), {})
                node[b''] = {b'length': len(content), b'pieces root': hashlib.sha256(content).digest()}
            info[b'file tree'] = tree
        encoded = bencode(info)
        self.v1 = hashlib.sha1(encoded).hexdigest() if self.mode != 'v2' else ''
        self.v2 = hashlib.sha256(encoded).hexdigest() if self.mode != 'v1' else ''
        self.hash = self.v1 or self.v2[:40]
        self.torrent = bencode({b'info': info})

    def magnet(self) -> str:
        """提供只含测试身份的磁力，不添加外部 Tracker 或 Peer。"""
        identities = ([f'urn:btih:{self.v1}'] if self.v1 else []) + ([f'urn:btmh:1220{self.v2}'] if self.v2 else [])
        return 'magnet:?' + urllib.parse.urlencode([('xt', identity) for identity in identities])


class FakeQB:
    """模拟公开 WebAPI 与异步移动，文件传输由测试显式推进。"""

    def __init__(self, root: Path) -> None:
        """创建只监听宿主回环地址的假引擎。"""
        # 唯一可写的隔离下载根，与被测应用配置一一映射。
        self.root: Path = root
        # 只在内存保留的验证密码，不打印或写入普通日志。
        self.password: str = secrets.token_urlsafe(24)
        # 模拟 qB SID，测试会主动过期一次以验证刷新。
        self.sid: str = secrets.token_hex(24)
        # 允许映射的受控测试种子。
        self.fixtures: dict[str, Fixture] = {}
        # 引擎任务与文件清单，禁止外部网络写入。
        self.tasks: dict[str, dict[str, Any]] = {}
        # 跨 HTTP 协调与断言的短期内存锁。
        self.lock: threading.RLock = threading.RLock()
        # 记录公开接口调用，不包含凭据、原种子或私密磁力。
        self.commands: list[tuple[str, str]] = []
        # 模拟引擎失联开关。
        self.offline: bool = False
        # 下次 add 接受后中断响应，模拟提交结果不确定。
        self.drop_add_reply: bool = False
        # 登录次数用于确认 SID 过期只触发有限刷新。
        self.logins: int = 0
        # 完整引擎快照读取次数，用于等待外部任务被下一轮对账观察到。
        self.snapshot_count: int = 0
        # 移动延迟确保应用不能仅凭 HTTP 200 提前发布。
        self.move_delay: float = 3.0
        engine = self

        class Handler(http.server.BaseHTTPRequestHandler):
            """只接受本测试需要的 qB 公开接口。"""

            def log_message(self, format: str, *args: Any) -> None:
                """关闭默认访问日志，避免泄漏请求参数。"""
                return

            def do_GET(self) -> None:
                """分派受认证 GET，不模拟浏览器 WebUI。"""
                self.handle_api()

            def handle(self) -> None:
                """容忍应用正常关闭 keep-alive 连接时的 Windows 重置。"""
                try:
                    super().handle()
                except (ConnectionResetError, BrokenPipeError):
                    return

            def do_POST(self) -> None:
                """分派有界 POST，并验证 Origin/Referer。"""
                self.handle_api()

            def reply(self, status: int, value: Any, cookie: bool = False) -> None:
                """写入有限响应，允许模拟认证成功的空正文。"""
                data = value if isinstance(value, bytes) else json.dumps(value).encode()
                try:
                    self.send_response(status)
                    self.send_header('Content-Length', str(len(data)))
                    if cookie:
                        self.send_header('Set-Cookie', f'{engine.cookie_name}={engine.sid}; HttpOnly; SameSite=Strict; Path=/')
                    self.end_headers()
                    self.wfile.write(data)
                except (BrokenPipeError, ConnectionResetError):
                    return

            def handle_api(self) -> None:
                """处理身份、选集、停止、移动与只移除任务的契约。"""
                parsed = urllib.parse.urlsplit(self.path)
                endpoint = parsed.path.removeprefix('/api/v2/')
                query = urllib.parse.parse_qs(parsed.query)
                raw = self.rfile.read(min(int(self.headers.get('Content-Length', '0')), 9 * 1024 * 1024))
                values: dict[str, str] = {}
                torrent: bytes | None = None
                content_type = self.headers.get('Content-Type', '')
                if content_type.startswith('multipart/form-data'):
                    message = BytesParser(policy=policy.default).parsebytes(('Content-Type: ' + content_type + '\r\nMIME-Version: 1.0\r\n\r\n').encode() + raw)
                    for part in message.iter_parts():
                        name = part.get_param('name', header='content-disposition')
                        body = part.get_payload(decode=True) or b''
                        if name == 'torrents':
                            torrent = body
                        elif name:
                            values[name] = body.decode()
                elif raw:
                    values = {key: entries[0] for key, entries in urllib.parse.parse_qs(raw.decode(), keep_blank_values=True).items()}
                with engine.lock:
                    if engine.offline:
                        self.reply(503, b'unavailable')
                        return
                    if self.command == 'POST' and (self.headers.get('Origin') != engine.base or self.headers.get('Referer') != engine.base + '/'):
                        self.reply(403, b'Forbidden')
                        return
                    if endpoint == 'auth/login':
                        engine.logins += 1
                        if values.get('username') == 'verify' and values.get('password') == engine.password:
                            self.reply(204, b'', True)
                        else:
                            self.reply(200, b'Fails.')
                        return
                    if f'{engine.cookie_name}={engine.sid}' not in self.headers.get('Cookie', ''):
                        self.reply(403, b'Forbidden')
                        return
                    engine.advance()
                    if endpoint == 'app/version':
                        self.reply(200, b'v5.2.4')
                        return
                    if endpoint == 'app/webapiVersion':
                        self.reply(200, b'2.15.1')
                        return
                    if endpoint == 'torrents/info':
                        engine.snapshot_count += 1
                        tasks = list(engine.tasks.values())
                        if query.get('hashes'):
                            tasks = [task for task in tasks if task['hash'] in query['hashes'][0].split('|')]
                        self.reply(200, [{key: value for key, value in task.items() if not key.startswith('_')} for task in tasks])
                        return
                    if endpoint in ('torrents/files', 'torrents/properties'):
                        task = engine.tasks.get(query.get('hash', [''])[0])
                        if not task:
                            self.reply(404, b'')
                        elif endpoint.endswith('files'):
                            self.reply(200, task['_files'])
                        else:
                            self.reply(200, {key: task[key] for key in ('infohash_v1', 'infohash_v2')})
                        return
                    if endpoint == 'torrents/add':
                        if torrent:
                            info = bdecode(torrent)[b'info']
                            v1 = hashlib.sha1(bencode(info)).hexdigest()
                            fixture = next((candidate for candidate in engine.fixtures.values() if candidate.v1 == v1 or candidate.v2 == hashlib.sha256(bencode(info)).hexdigest()), None)
                        else:
                            magnet = urllib.parse.parse_qs(urllib.parse.urlsplit(values.get('urls', '')).query)
                            identities = magnet.get('xt', [])
                            fixture = next((candidate for candidate in engine.fixtures.values() if ('urn:btih:' + candidate.v1 in identities and candidate.v1) or ('urn:btmh:1220' + candidate.v2 in identities and candidate.v2)), None)
                        if not fixture:
                            self.reply(400, b'Invalid fixture')
                            return
                        engine.commands.append((endpoint, fixture.hash))
                        if fixture.hash not in engine.tasks:
                            engine.create_task(fixture, values['savepath'], values.get('tags', ''))
                        if engine.drop_add_reply:
                            engine.drop_add_reply = False
                            self.close_connection = True
                            self.connection.shutdown(socket.SHUT_RDWR)
                            self.connection.close()
                            return
                        self.reply(200, b'Ok.')
                        return
                    hash_value = values.get('hash', values.get('hashes', ''))
                    task = engine.tasks.get(hash_value)
                    if task is None:
                        self.reply(404, b'')
                        return
                    engine.commands.append((endpoint, hash_value))
                    if endpoint == 'torrents/filePrio':
                        indices = [int(index) for index in values['id'].split('|')]
                        for entry in task['_files']:
                            if entry['index'] in indices:
                                entry['priority'] = int(values['priority'])
                    elif endpoint == 'torrents/start':
                        task['state'] = 'downloading' if task['amount_left'] else 'uploading'
                    elif endpoint == 'torrents/stop':
                        task['state'] = 'stoppedDL' if task['amount_left'] else 'stoppedUP'
                    elif endpoint == 'torrents/setStopCondition':
                        pass
                    elif endpoint == 'torrents/setLocation':
                        task['_move_target'] = values['location']
                        task['_move_at'] = time.monotonic() + engine.move_delay
                        task['state'] = 'moving'
                    elif endpoint == 'torrents/delete':
                        assert values.get('deleteFiles') == 'false', '应用请求删除源文件'
                        del engine.tasks[hash_value]
                    elif endpoint in ('torrents/setDownloadLimit', 'torrents/setUploadLimit'):
                        task['_' + endpoint.split('/')[-1]] = int(values['limit'])
                    elif endpoint not in ('torrents/setDownloadLimit', 'torrents/setUploadLimit'):
                        self.reply(404, b'unsupported endpoint')
                        return
                    self.reply(200, b'')

        # 由操作系统分配空闲端口，避免污染开发服务。
        self.server: http.server.ThreadingHTTPServer = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        # 引擎固定基地址供 Origin/Referer 以及应用环境使用。
        self.base: str = f'http://127.0.0.1:{self.server.server_port}'
        # 固定版本真实引擎使用带WebUI端口的会话名，不能硬编码旧SID。
        self.cookie_name: str = f'QBT_SID_{self.server.server_port}'
        # HTTP 服务线程只存活于本次验证生命周期。
        self.thread: threading.Thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def register(self, fixture: Fixture) -> Fixture:
        """登记测试种子，不发起任何网络下载。"""
        self.fixtures[fixture.hash] = fixture
        return fixture

    def create_task(self, fixture: Fixture, save_path: str, tags: str) -> None:
        """按 qB 字段生成停止且已有元数据的任务快照。"""
        total = sum(len(content) for _, content in fixture.files)
        self.tasks[fixture.hash] = {'hash': fixture.hash, 'infohash_v1': fixture.v1, 'infohash_v2': fixture.v2, 'name': fixture.name, 'state': 'stoppedDL', 'save_path': save_path, 'content_path': save_path + '/' + fixture.name, 'tags': tags, 'size': total, 'total_size': total, 'completed': 0, 'downloaded': 0, 'dlspeed': 0, 'upspeed': 0, 'uploaded': 0, 'ratio': 0, 'num_leechs': 0, 'num_seeds': 0, 'eta': 8640000, 'progress': 0, 'amount_left': total, '_files': [{'index': index, 'name': fixture.name + '/' + path, 'size': len(content), 'progress': 0, 'priority': 1} for index, (path, content) in enumerate(fixture.files)]}

    def local_path(self, engine_path: str) -> Path:
        """将唯一受控的虚拟 /downloads 映射到隔离宿主目录。"""
        assert engine_path.startswith('/downloads/'), '引擎请求越过专用根'
        candidate = (self.root / engine_path.removeprefix('/downloads/')).resolve()
        assert candidate.is_relative_to(self.root), '引擎路径发生越界'
        return candidate

    def complete(self, hash_value: str) -> None:
        """显式写完隔离样本，同时保留未选文件以验证清单门禁。"""
        with self.lock:
            task = self.tasks[hash_value]
            fixture = self.fixtures[hash_value]
            for entry, (_, content) in zip(task['_files'], fixture.files, strict=True):
                target = self.local_path(task['save_path']) / entry['name']
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(content)
                entry['progress'] = 1 if entry['priority'] else 0
            selected = sum(entry['size'] for entry in task['_files'] if entry['priority'])
            task.update({'size': selected, 'completed': selected, 'downloaded': selected, 'amount_left': 0, 'progress': 1, 'state': 'uploading', 'upspeed': 128, 'ratio': .25})

    def advance(self) -> None:
        """延迟完成 setLocation，直到实际目录移动后才改变引擎保存路径。"""
        for task in self.tasks.values():
            if task.get('_move_at', float('inf')) > time.monotonic():
                continue
            target = self.local_path(task['_move_target'])
            source = self.local_path(task['save_path'])
            target.parent.mkdir(parents=True, exist_ok=True)
            if source.exists() and source != target:
                if target.exists() and not any(target.iterdir()):
                    target.rmdir()
                shutil.move(str(source), str(target))
            task['save_path'] = task.pop('_move_target')
            task['content_path'] = task['save_path'] + '/' + task['name']
            task['state'] = 'uploading'
            del task['_move_at']

    def close(self) -> None:
        """关闭假引擎及其服务器线程，不删除样本。"""
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)


class Harness:
    """管理隔离应用进程、会话和下载场景断言。"""

    def __init__(self, binary: Path, root: Path) -> None:
        """准备独立数据目录与动态端口，不使用现有用户服务。"""
        # 待测应用可执行文件，须由调用方事先构建。
        self.binary: Path = binary
        # 所有生成文件的唯一路径，强制含 verify 段。
        self.root: Path = root
        # 受控假下载器，与浏览器会话完全分离。
        self.engine: FakeQB = FakeQB(root / 'downloads')
        # 应用运行期间的进程句柄。
        self.process: subprocess.Popen[bytes] | None = None
        # 逐次启动的标准日志文件，不写入凭据。
        self.log: Any = None
        # 空闲回环端口供被测应用监听。
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            port = listener.getsockname()[1]
        # 独立应用地址，所有 API 保持同源。
        self.base: str = f'http://127.0.0.1:{port}'
        # 会话客户端复用原集成验证的请求与断言格式。
        self.client: Client = Client(self.base)
        # 启动参数集合，参数含义均与环境配置文档一致。
        self.env: dict[str, str] = os.environ.copy()
        for path in ('data', 'media', 'downloads/staging', 'downloads/complete'):
            (root / path).mkdir(parents=True, exist_ok=True)
        password_file = root / 'qb-password'
        password_file.write_text(self.engine.password, encoding='utf-8')
        if os.name != 'nt':
            password_file.chmod(0o600)
        self.env.update({
            # 隔离监听地址，避免开放开发实例。
            'NAS_ADDR': f'127.0.0.1:{port}',
            # SQLite、缓存与下载数据分开保存。
            'NAS_DATA_DIR': str(root / 'data'),
            # 普通媒体与 complete 是仅有的授权根。
            'NAS_MEDIA_ROOTS': os.pathsep.join((str(root / 'media'), str(root / 'downloads' / 'complete'))),
            # 复用仓库已构建的静态前端，不要求 Docker。
            'NAS_WEB_DIR': str(Path(__file__).resolve().parents[1] / 'web' / 'dist'),
            # 周期扫描设为一小时，显式场景控制扫描时点。
            'NAS_SCAN_INTERVAL_MINUTES': '60',
            # 关闭测试衍生缓存的额外保留空间，不改变源文件处理。
            'NAS_MIN_FREE_MB': '0',
            # 启用下载编排，假引擎只接收本机生成内容。
            'NAS_DOWNLOAD_ENABLED': 'true',
            # 应用可读的隔离宿主下载根。
            'NAS_DOWNLOAD_ROOT': str(root / 'downloads'),
            # 假引擎使用固定虚拟根并映射到同一实体。
            'NAS_DOWNLOAD_ENGINE_ROOT': '/downloads',
            # 固定回环 WebAPI 地址。
            'NAS_DOWNLOAD_QB_URL': self.engine.base,
            # 引擎验证用户名，与栖库管理员不同。
            'NAS_DOWNLOAD_QB_USERNAME': 'verify',
            # 仅读取本次生成的密码文件。
            'NAS_DOWNLOAD_QB_PASSWORD_FILE': str(password_file),
            # 测试小样本不要求额外磁盘 GiB 配额。
            'NAS_DOWNLOAD_MIN_FREE_GB': '0',
        })

    def start(self) -> None:
        """启动同一数据库的应用并有限等待健康检查。"""
        self.log = (self.root / 'application.log').open('ab')
        self.process = subprocess.Popen([str(self.binary)], env=self.env, stdout=self.log, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if self.process.poll() is not None:
                raise RuntimeError('应用提前退出：' + (self.root / 'application.log').read_text(encoding='utf-8')[-2000:])
            try:
                if self.client.request('/healthz')[0] == 200:
                    return
            except OSError:
                pass
            time.sleep(.2)
        raise RuntimeError('应用启动超时')

    def stop(self) -> None:
        """停止隔离应用，保留数据库以验证重启恢复。"""
        if self.process and self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=20)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait(timeout=5)
        if self.log:
            self.log.close()

    def wait_task(self, id: str, predicate: Callable[[dict[str, Any]], bool], timeout: float = 45) -> dict[str, Any]:
        """等待指定任务条件，失败返回最后一份不含隐私的 DTO。"""
        deadline = time.monotonic() + timeout
        last: dict[str, Any] = {}
        while time.monotonic() < deadline:
            last = self.client.json('/api/downloads/' + id)['download']
            if predicate(last):
                return last
            time.sleep(.25)
        raise AssertionError('下载状态未收敛：' + json.dumps(last, ensure_ascii=False))

    def upload(self, fixture: Fixture, kind: str, request_id: str | None = None) -> tuple[int, dict[str, Any]]:
        """通过真实 multipart 路由添加单个受控种子。"""
        boundary = 'verify-' + secrets.token_hex(12)
        fields = {'requestId': request_id or secrets.token_hex(16), 'contentType': kind, 'createLibrary': 'true'}
        body = b''.join(f'--{boundary}\r\nContent-Disposition: form-data; name="{key}"\r\n\r\n{value}\r\n'.encode() for key, value in fields.items())
        body += f'--{boundary}\r\nContent-Disposition: form-data; name="torrent"; filename="fixture.torrent"\r\nContent-Type: application/x-bittorrent\r\n\r\n'.encode() + fixture.torrent + f'\r\n--{boundary}--\r\n'.encode()
        request = urllib.request.Request(self.base + '/api/downloads', data=body, headers={'Origin': self.base, 'Content-Type': f'multipart/form-data; boundary={boundary}'}, method='POST')
        try:
            response = self.client.opener.open(request, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.code, json.loads(response.read(1024 * 1024))

    def add(self, fixture: Fixture, kind: str, magnet: bool = False, timeout: float = 45) -> dict[str, Any]:
        """添加种子或磁力并等待真实协调器取得文件清单。"""
        if magnet:
            result = self.client.json('/api/downloads', 'POST', {'requestId': secrets.token_hex(16), 'magnet': fixture.magnet(), 'contentType': kind, 'createLibrary': True}, status=202)
        else:
            code, result = self.upload(fixture, kind)
            assert code == 202, f'上传失败：{code} {result}'
        task = result['download']
        return self.wait_task(task['id'], lambda item: item['state'] == 'awaiting_selection', timeout=timeout)

    def select(self, task: dict[str, Any], indices: list[int]) -> dict[str, Any]:
        """用当前清单版本确认选集，禁止悄悄下载全包。"""
        result = self.client.json('/api/downloads/' + task['id'] + '/selection', 'PUT', {'requestId': secrets.token_hex(16), 'manifestVersion': task['manifestVersion'], 'indices': indices}, status=202)
        return result['download']

    def wait_engine(self, predicate: Callable[[], bool], timeout: float = 30) -> None:
        """等待异步持久命令生效，不假定 API 返回代表引擎已经执行。"""
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            with self.engine.lock:
                if predicate():
                    return
            time.sleep(.2)
        raise AssertionError('引擎命令没有在预算时间内生效')

    def run(self) -> None:
        """覆盖下载、身份、安全发布、门禁、断点恢复和会话边界。"""
        client = self.client
        self.start()
        anonymous = Client(self.base)
        client.check(anonymous.request('/api/downloads/status')[0] == 401, '下载状态沿用管理员认证')
        client.json('/api/setup', 'POST', {'username': 'downloads-verify', 'password': secrets.token_urlsafe(24)})
        client.check(client.request('/api/downloads', 'POST', {}, {'Origin': 'https://foreign.invalid'})[0] == 403, '跨站添加下载被拒绝')
        status = client.json('/api/downloads/connection-test', 'POST', {})
        client.check(status['state'] == 'ready' and status['version'] == 'v5.2.4', '探测固定引擎版本且接受空正文SID登录')
        client.check(client.request('/api/downloads', 'POST', {'requestId': secrets.token_hex(16), 'magnet': 'https://invalid.example/torrent', 'contentType': 'novel', 'createLibrary': True})[0] == 400, '拒绝任意远程种子URL')
        client.check(client.request('/api/downloads', 'POST', {'requestId': secrets.token_hex(16), 'magnet': 'x' * 70000, 'contentType': 'novel', 'createLibrary': True})[0] == 413, '普通JSON仍受64KiB限制')
        oversized = Fixture('超限上传', [('sample.txt', b'bounded')])
        oversized.torrent = b'x' * (8 * 1024 * 1024 + 1)
        client.check(self.upload(oversized, 'novel')[0] == 413, '种子本体超过8MiB返回413')
        unsafe = Fixture('越界种子', [('../escape.txt', b'outside')])
        client.check(self.upload(unsafe, 'novel')[0] == 400, '种子文件清单拒绝父目录穿越')
        text = ('本机 BT 下载验证。\n' * 5).encode()
        fixture = self.engine.register(Fixture('安全发布验证', [('selected.txt', text), ('unselected.txt', b'not selected')]))
        task = self.add(fixture, 'novel')
        id = task['id']
        files = client.json('/api/downloads/' + id + '/files?page=1&pageSize=1')
        client.check(files['total'] == 2 and len(files['files']) == 1, '文件清单独立分页且返回清单版本')
        duplicate = client.json('/api/downloads', 'POST', {'requestId': secrets.token_hex(16), 'magnet': fixture.magnet(), 'contentType': 'novel', 'createLibrary': True}, status=202)
        client.check(duplicate['download']['id'] == id, '同身份磁力与种子定位同一受管任务')
        code, _, _ = client.request('/api/downloads/' + id + '/selection', 'PUT', {'requestId': secrets.token_hex(16), 'manifestVersion': task['manifestVersion'] + 1, 'indices': [0]})
        client.check(code == 409, '过时文件清单版本不能确认选集')
        self.select(task, [0])
        task = self.wait_task(id, lambda item: item['selectionLocked'] and item['state'] == 'downloading')
        client.check(client.request('/api/downloads/' + id + '/selection', 'PUT', {'requestId': secrets.token_hex(16), 'manifestVersion': task['manifestVersion'], 'indices': [1]})[0] == 409, '下载开始后选集锁定')
        client.json('/api/downloads/' + id + '/limits', 'PUT', {'downloadKiB': 128, 'uploadKiB': 32})
        self.wait_engine(lambda: self.engine.tasks[fixture.hash].get('_setDownloadLimit') == 131072 and self.engine.tasks[fixture.hash].get('_setUploadLimit') == 32768)
        client.check(True, '任务级限速按KiB转字节且不修改全局偏好')
        client.check(client.request('/api/downloads/' + id + '/limits', 'PUT', {'downloadKiB': -1, 'uploadKiB': 0})[0] == 400, '负限速参数被拒绝')
        task = client.json('/api/downloads/' + id)['download']
        previous_stops = sum(endpoint == 'torrents/stop' and hash_value == fixture.hash for endpoint, hash_value in self.engine.commands)
        pause = {'requestId': secrets.token_hex(16), 'revision': task['revision'], 'action': 'pause'}
        client.json('/api/downloads/' + id + '/actions', 'POST', pause, status=202)
        client.json('/api/downloads/' + id + '/actions', 'POST', pause, status=202)
        paused = self.wait_task(id, lambda entry: entry['state'] == 'paused')
        self.wait_engine(lambda: sum(endpoint == 'torrents/stop' and hash_value == fixture.hash for endpoint, hash_value in self.engine.commands) >= previous_stops + 1)
        client.check(sum(endpoint == 'torrents/stop' and hash_value == fixture.hash for endpoint, hash_value in self.engine.commands) == previous_stops + 1, '相同暂停请求只执行一次引擎命令')
        client.check(client.request('/api/downloads/' + id + '/actions', 'POST', {'requestId': secrets.token_hex(16), 'revision': task['revision'], 'action': 'start'})[0] == 409, '过时修订不能覆盖最新暂停意图')
        client.json('/api/downloads/' + id + '/actions', 'POST', {'requestId': secrets.token_hex(16), 'revision': paused['revision'], 'action': 'start'}, status=202)
        self.wait_task(id, lambda entry: entry['state'] == 'downloading')
        self.engine.complete(fixture.hash)
        moving = self.wait_task(id, lambda item: item['state'] == 'moving' or item['importState'] == 'publishing')
        client.check(not moving['ready'], 'setLocation接受后尚未完成时保持发布门禁关闭')
        task = self.wait_task(id, lambda item: item['importState'] == 'imported', timeout=60)
        items = client.json('/api/downloads/' + id + '/items')['items']
        client.check(task['ready'] and len(items) == 1 and items[0]['relativePath'].endswith('selected.txt'), '所选清单经真实扫描入库，未选占位文件不泄漏')
        item_id = items[0]['id']
        client.check(client.request(f'/api/items/{item_id}/file')[2] == text, '入库读取返回原始字节')
        client.json(f'/api/items/{item_id}/favorite', 'POST', {'favorite': True})
        client.json(f'/api/items/{item_id}/progress', 'PUT', {'position': 2, 'total': 5, 'locator': ''})
        before = sum(endpoint == 'torrents/add' for endpoint, _ in self.engine.commands)
        self.stop()
        self.start()
        restored = self.wait_task(id, lambda item: item['importState'] == 'imported')
        state = client.json(f'/api/items/{item_id}')['item']
        client.check(restored['id'] == id and state['favorite'] and state['progress']['position'] == 2, '应用重启保持任务、收藏和进度')
        client.check(sum(endpoint == 'torrents/add' for endpoint, _ in self.engine.commands) == before, '重启对账不重复添加已有种子')
        self.engine.sid = secrets.token_hex(24)
        previous_logins = self.engine.logins
        client.json('/api/downloads/connection-test', 'POST', {})
        client.check(self.engine.logins == previous_logins + 1, '引擎SID过期只刷新一次，不影响应用会话')
        self.engine.offline = True
        offline = client.json('/api/downloads/connection-test', 'POST', {})
        client.check(offline['state'] in ('offline', 'auth_failed') and client.request('/healthz')[0] == 200, '引擎离线不影响基础健康检查')
        client.check(client.request(f'/api/items/{item_id}/file')[2] == text, '引擎离线不撤销已发布本地媒体')
        self.engine.offline = False
        client.json('/api/downloads/connection-test', 'POST', {})
        image_path = self.root / 'sample.png'
        make_png(image_path, 1, 32, 32)
        image = image_path.read_bytes()
        audio_path = self.root / 'sample.wav'
        with wave.open(str(audio_path), 'wb') as audio:
            audio.setparams((1, 2, 8000, 0, 'NONE', 'not compressed'))
            audio.writeframes(b'\x00\x00' * 8000)
        samples = {'image': [('picture.png', image)], 'comic': [('page1.png', image), ('page2.png', image)], 'music': [('track.wav', audio_path.read_bytes())], 'video': [('index-only.mp4', b'protocol fixture; not a decodable video')]}
        for kind, entries in samples.items():
            sample = self.engine.register(Fixture('五类入库-' + kind, entries))
            pending = self.add(sample, kind)
            self.select(pending, [0])
            self.wait_task(pending['id'], lambda entry: entry['selectionLocked'] and entry['state'] == 'downloading')
            self.engine.complete(sample.hash)
            self.wait_task(pending['id'], lambda entry: entry['importState'] == 'imported', timeout=60)
            imported = client.json('/api/downloads/' + pending['id'] + '/items')['items']
            client.check(len(imported) == 1 and imported[0]['type'] == kind, f'{kind}下载复用对应类型的真实扫描入库')
            if kind == 'comic':
                pages = client.json(f'/api/items/{imported[0]["id"]}/comic')['pages']
                client.check(len(pages) == 1 and pages[0]['name'].endswith('page1.png'), '目录漫画页数、枚举均排除未选占位图片')
        retry_fixture = self.engine.register(Fixture('重试入库验证', [('retry.txt', text)]))
        retry_task = self.add(retry_fixture, 'novel')
        self.select(retry_task, [0])
        self.wait_task(retry_task['id'], lambda entry: entry['state'] == 'downloading')
        # 仅向隔离数据库注入可撤销失败，验证真实扫描失败后的独立重试。
        with sqlite3.connect(self.root / 'data' / 'library.db', timeout=10) as connection:
            connection.execute("CREATE TRIGGER verify_download_import_failure BEFORE INSERT ON items WHEN NEW.relative_path LIKE '%retry.txt' BEGIN SELECT RAISE(FAIL, '验证入库失败'); END")
        self.engine.complete(retry_fixture.hash)
        failed = self.wait_task(retry_task['id'], lambda entry: entry['importState'] == 'failed', timeout=60)
        client.check(failed['ready'], '扫描入库失败不删除已下载和已验证的数据')
        before_retry = sum(endpoint == 'torrents/add' for endpoint, _ in self.engine.commands)
        with sqlite3.connect(self.root / 'data' / 'library.db', timeout=10) as connection:
            connection.execute('DROP TRIGGER verify_download_import_failure')
        client.json('/api/downloads/' + retry_task['id'] + '/retry-import', 'POST', {}, status=202)
        self.wait_task(retry_task['id'], lambda entry: entry['importState'] == 'imported', timeout=60)
        client.check(sum(endpoint == 'torrents/add' for endpoint, _ in self.engine.commands) == before_retry, '入库失败重试复用已有文件，不再次下载')
        for mode in ('v2', 'hybrid'):
            identity = self.engine.register(Fixture('身份验证-' + mode, [('sample.txt', mode.encode())], mode))
            found = self.add(identity, 'novel')
            again = client.json('/api/downloads', 'POST', {'requestId': secrets.token_hex(16), 'magnet': identity.magnet(), 'contentType': 'novel', 'createLibrary': True}, status=202)
            client.check(found['id'] == again['download']['id'], f'{mode}种子完整身份与磁力去重一致')
        conflict = self.engine.register(Fixture('外部任务', [('outside.txt', b'outside')]))
        with self.engine.lock:
            self.engine.create_task(conflict, '/downloads/external', '')
            snapshots = self.engine.snapshot_count
        client.json('/api/downloads/connection-test', 'POST', {})
        self.wait_engine(lambda: self.engine.snapshot_count > snapshots)
        time.sleep(.2)
        code, _ = self.upload(conflict, 'novel')
        client.check(code == 409, '外部同哈希任务拒绝接管')
        uncertain = self.engine.register(Fixture('不确定提交', [('uncertain.txt', b'uncertain')]))
        self.engine.drop_add_reply = True
        # 网络故障的生产退避为60秒，验收窗口须覆盖下一次自动对账。
        recovered = self.add(uncertain, 'novel', timeout=120)
        client.check(sum(endpoint == 'torrents/add' and hash_value == uncertain.hash for endpoint, hash_value in self.engine.commands) == 1 and bool(recovered['id']), '添加响应中断后按身份对账，不重新发送add')
        task = client.json('/api/downloads/' + id)['download']
        client.json('/api/downloads/' + id + '/actions', 'POST', {'requestId': secrets.token_hex(16), 'revision': task['revision'], 'action': 'remove'}, status=202)
        self.wait_task(id, lambda item: item['archived'])
        client.check(client.request(f'/api/items/{item_id}/file')[2] == text, '移除任务保留文件、发布记录和已入库内容')
        client.check(all(endpoint.startswith('torrents/') for endpoint, _ in self.engine.commands), '编排只使用受管任务API，没有全局偏好写入')
        print(f'下载编排集成验证通过：{client.passed} 项断言。引擎为可控假实现，不证明真实 BT 或 Docker 行为。', flush=True)

    def close(self) -> None:
        """保证异常后也释放应用和假引擎。"""
        self.stop()
        self.engine.close()

    def serve(self) -> None:
        """保留隔离浏览器联调实例，收到停止文件后释放进程。"""
        self.start()
        credentials = {'username': 'downloads-browser', 'password': secrets.token_urlsafe(24)}
        self.client.json('/api/setup', 'POST', credentials)
        credential_file = self.root / 'app-credentials.json'
        credential_file.write_text(json.dumps(credentials), encoding='utf-8')
        if os.name != 'nt':
            credential_file.chmod(0o600)
        # 只创建本机小样本；浏览器添加时由假引擎提供固定元数据。
        imported = self.engine.register(Fixture('浏览器已入库小说', [('示例章节.txt', ('栖库下载联调样本。\n' * 20).encode())]))
        pending = self.engine.register(Fixture('浏览器待选文件', [('第一章.txt', '第一章\n'.encode()), ('第二章.txt', '第二章\n'.encode())]))
        upload = self.engine.register(Fixture('浏览器上传种子', [('上传示例.txt', '本地生成的安全样本\n'.encode())]))
        (self.root / 'sample.torrent').write_bytes(upload.torrent)
        (self.root / 'sample-magnet.txt').write_text(upload.magnet(), encoding='utf-8')
        complete = self.add(imported, 'novel')
        self.select(complete, [0])
        self.wait_task(complete['id'], lambda entry: entry['state'] == 'downloading')
        self.engine.complete(imported.hash)
        self.wait_task(complete['id'], lambda entry: entry['importState'] == 'imported', timeout=60)
        self.add(pending, 'novel')
        print(json.dumps({'base': self.base, 'username': credentials['username'], 'credentialFile': str(credential_file), 'sampleTorrent': str(self.root / 'sample.torrent'), 'sampleMagnet': str(self.root / 'sample-magnet.txt'), 'stopFile': str(self.root / 'stop')}, ensure_ascii=False), flush=True)
        while not (self.root / 'stop').exists():
            # 外部验证可用标记文件切换离线状态，不开放额外HTTP管理接口。
            self.engine.offline = (self.root / 'engine-offline').exists()
            time.sleep(.5)


def main() -> None:
    """解析隔离验证参数，拒绝覆盖现有验证数据。"""
    parser = argparse.ArgumentParser(description=__doc__)
    # 应用二进制必须提前完成构建，脚本不隐式安装工具链。
    parser.add_argument('--binary', type=Path, required=True, help='本机构建的栖库可执行文件')
    # 根路径包含 verify 段且不存在，避免触碰真实媒体或老测试实例。
    parser.add_argument('--root', type=Path, default=Path('.runtime/verify/downloads-' + time.strftime('%Y%m%d-%H%M%S')), help='全新隔离目录，路径必须包含 verify 段')
    # 浏览器联调模式仅准备样本，不将其输出误计为自动断言通过。
    parser.add_argument('--serve', action='store_true', help='保留隔离浏览器样本，创建根目录下stop文件后退出')
    args = parser.parse_args()
    root = args.root.resolve()
    binary = args.binary.resolve()
    if not binary.is_file():
        parser.error('--binary 指向的文件不存在，请先构建应用')
    if 'verify' not in root.parts or root.exists():
        parser.error('--root 必须不存在且路径中包含 verify 段')
    root.mkdir(parents=True)
    harness = Harness(binary, root)
    completed = False
    try:
        if args.serve:
            harness.serve()
        else:
            harness.run()
            completed = True
    finally:
        harness.close()
        if not args.serve:
            # 只记录断言数量和执行边界，不保存测试凭据或原始来源。
            result = {'passed': completed, 'assertions': harness.client.passed, 'engine': 'controlled fake qB', 'realApplication': True, 'realBTTransfer': False, 'dockerExecuted': False}
            (root / 'result.json').write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding='utf-8')


if __name__ == '__main__':
    main()
