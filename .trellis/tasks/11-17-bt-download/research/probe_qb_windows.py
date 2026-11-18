"""以本地双 qB 进程验证公开 API，不连接外部 Tracker 或真实媒体。"""
from __future__ import annotations
import argparse
import base64
import hashlib
import http.cookiejar
import http.cookies
import json
import os
from pathlib import Path
import secrets
import subprocess
import socket
import time
from typing import Any, Callable
import urllib.error
import urllib.parse
import urllib.request

# 唯一允许写入的新建隔离目录，由命令行在 main 中验证。
ROOT: Path
# 调用方提供的官方 Windows 可执行文件，不自动安装到系统。
EXE: Path
# 随机验证凭据只保留在本进程内存与独立 profile 的哈希中。
PASS: str

def bencode(value: Any) -> bytes:
    """编码仅包含本探针需要的数据结构。"""
    if isinstance(value, bytes): return str(len(value)).encode() + b':' + value
    if isinstance(value, int): return b'i' + str(value).encode() + b'e'
    if isinstance(value, list): return b'l' + b''.join(bencode(x) for x in value) + b'e'
    return b'd' + b''.join(bencode(k) + bencode(value[k]) for k in sorted(value)) + b'e'

class QB:
    """持有一个隔离 profile 与受认证的本机 WebAPI 客户端。"""
    def __init__(self, name: str, port: int, peer: int) -> None:
        """生成独立配置并隐藏启动本机实例。"""
        # 本实例的回环 WebUI 端口。
        self.port: int = port
        # 只接收本机请求的固定 WebAPI 基地址。
        self.base: str = f'http://127.0.0.1:{port}'
        # 本次新建的独立配置根，不复用用户现有配置。
        self.profile: Path = ROOT / name
        self.profile.mkdir(exist_ok=True)
        config = self.profile / 'qBittorrent' / 'config'
        config.mkdir(parents=True, exist_ok=True)
        salt = secrets.token_bytes(16)
        digest = hashlib.pbkdf2_hmac('sha512', PASS.encode(), salt, 100000)
        encoded = base64.b64encode(salt).decode() + ':' + base64.b64encode(digest).decode()
        settings = [
            '[Preferences]',
            # 启用 WebUI 但只监听回环地址。
            'WebUI\\Enabled=true',
            # 不向局域网开放探针管理端口。
            'WebUI\\Address=127.0.0.1',
            # 由操作系统选出的本次空闲端口。
            f'WebUI\\Port={port}',
            # 仅供探针使用的固定用户名。
            'WebUI\\Username=probe',
            # 随机密码仅以PBKDF2摘要写入独立配置。
            f'WebUI\\Password_PBKDF2="@ByteArray({encoded})"',
            # 回环请求也必须认证。
            'WebUI\\LocalHostAuth=true',
            # 保留跨站请求防护。
            'WebUI\\CSRFProtection=true',
            # 保留Host头检查。
            'WebUI\\HostHeaderValidation=true',
            # 隐藏启动探针窗口，不影响WebAPI。
            'General\\StartMinimized=true',
            # 不申请路由器端口映射。
            'Connection\\UPnP=false',
            '[BitTorrent]',
            # 不加入公网DHT发现网络。
            'Session\\DHTEnabled=false',
            # 不通过已有Peer交换外部地址。
            'Session\\PeXEnabled=false',
            # 不广播局域网发现包。
            'Session\\LSDEnabled=false',
            # 仅绑定本机Peer接口。
            'Session\\InterfaceAddress=127.0.0.1',
            # 本机种源和下载端使用不同端口。
            f'Session\\Port={peer}',
            # 样本做种不被分享率上限提前停止。
            'Session\\GlobalMaxRatio=-1',
        ]
        (config / 'qBittorrent.ini').write_text('\n'.join(settings) + '\n', encoding='utf-8')
        startup = subprocess.STARTUPINFO()
        startup.dwFlags |= subprocess.STARTF_USESHOWWINDOW
        startup.wShowWindow = subprocess.SW_HIDE
        # 仅持有本探针启动的进程，清理时不触碰其他qB实例。
        self.process: subprocess.Popen[bytes] = subprocess.Popen([str(EXE), '--no-splash', '--confirm-legal-notice', f'--profile={self.profile}', f'--webui-port={port}', f'--torrenting-port={peer}'], startupinfo=startup)
        jar = http.cookiejar.CookieJar()
        # CookieJar仅管理本实例的SID，不打印认证值。
        self.client: urllib.request.OpenerDirector = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
        # 记录真实登录HTTP码，避免由空响应正文推测为200。
        self.auth_status: int = 0
        # 只记录会话Cookie的公开名称，不记录值。
        self.auth_cookie_name: str = ''
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            try:
                result = self.call('auth/login', {'username': 'probe', 'password': PASS})
                if result.strip() in (b'', b'Ok.'):
                    assert self.call('app/version').startswith(b'v5.2.4')
                    return
            except (OSError, urllib.error.HTTPError): pass
            time.sleep(.3)
        self.close()
        raise RuntimeError('本机隔离 qB 登录失败')

    def call(self, path: str, values: dict[str, Any] | None = None, torrent: bytes | None = None) -> bytes:
        """发送具有匹配 Origin/Referer 的有界本机请求。"""
        headers = {'Origin': self.base, 'Referer': self.base + '/'}
        body = None
        if torrent is not None:
            boundary = 'probe-' + secrets.token_hex(8)
            parts = []
            for key, value in (values or {}).items():
                parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{key}"\r\n\r\n{value}\r\n'.encode())
            parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="torrents"; filename="fixture.torrent"\r\nContent-Type: application/x-bittorrent\r\n\r\n'.encode() + torrent + b'\r\n')
            parts.append(f'--{boundary}--\r\n'.encode())
            body = b''.join(parts)
            headers['Content-Type'] = f'multipart/form-data; boundary={boundary}'
        elif values is not None:
            body = urllib.parse.urlencode(values).encode()
            headers['Content-Type'] = 'application/x-www-form-urlencoded'
        request = urllib.request.Request(self.base + '/api/v2/' + path, data=body, headers=headers)
        with self.client.open(request, timeout=15) as response:
            if path == 'auth/login':
                self.auth_status = response.status
                cookies = http.cookies.SimpleCookie()
                for value in response.headers.get_all('Set-Cookie', []):
                    cookies.load(value)
                self.auth_cookie_name = next(iter(cookies), '')
            return response.read(8 * 1024 * 1024)

    def data(self, path: str) -> Any:
        """读取本机 qB 的 JSON。"""
        return json.loads(self.call(path))

    def close(self) -> None:
        """让隔离 qB 正常保存状态并退出。"""
        try: self.call('app/shutdown', {})
        except OSError: pass
        try: self.process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            self.process.terminate()
            self.process.wait(timeout=5)

def wait_for(qb: QB, hash_value: str, predicate: Callable[[dict[str, Any]], bool], seconds: int = 30, peer: str | None = None) -> dict[str, Any]:
    """有限等待引擎状态，失败记录最后快照。"""
    deadline = time.monotonic() + seconds
    last = {}
    next_peer = 0.0
    while time.monotonic() < deadline:
        if peer and time.monotonic() >= next_peer:
            # 禁用所有发现机制后，只能重连显式允许的本机种源。
            qb.call('torrents/addPeers', {'hashes': hash_value, 'peers': peer})
            next_peer = time.monotonic() + 5
        entries = qb.data('torrents/info?hashes=' + hash_value)
        if entries:
            last = entries[0]
            if predicate(last): return last
        time.sleep(.3)
    raise RuntimeError('本机状态未收敛：' + json.dumps(last))

def main() -> None:
    """运行停止添加、元数据停止、选集、移动与保留文件删除探针。"""
    global ROOT, EXE, PASS
    parser = argparse.ArgumentParser(description=__doc__)
    # 已解包的官方 Windows 程序，不执行系统安装器。
    parser.add_argument('--binary', type=Path, required=True, help='官方 qbittorrent.exe 的绝对或相对路径')
    # 必须是全新目录且路径包含 verify 段，不操作现有 profile。
    parser.add_argument('--root', type=Path, required=True, help='不存在且含 verify 路径段的隔离工作目录')
    args = parser.parse_args()
    ROOT = args.root.resolve()
    EXE = args.binary.resolve()
    PASS = secrets.token_urlsafe(24)
    if os.name != 'nt' or not EXE.is_file() or ROOT.exists() or 'verify' not in ROOT.parts:
        parser.error('仅支持 Windows、已有程序文件和全新 verify 隔离目录')
    ROOT.mkdir(parents=True)
    listeners: list[socket.socket] = []
    for _ in range(4):
        listener = socket.socket()
        listener.bind(('127.0.0.1', 0))
        listeners.append(listener)
    ports = [listener.getsockname()[1] for listener in listeners]
    for listener in listeners:
        listener.close()
    results: dict[str, Any] = {'engine': 'qBittorrent Windows 5.2.4', 'dockerExecuted': False}
    seed: QB | None = None
    client: QB | None = None
    try:
        seed = QB('seed-profile', ports[0], ports[2])
        client = QB('client-profile', ports[1], ports[3])
        results['version'] = client.call('app/version').decode()
        results['webApiVersion'] = client.call('app/webapiVersion').decode()
        results['authLoginStatus'] = client.auth_status
        results['authCookieName'] = client.auth_cookie_name
        preferences = client.data('app/preferences')
        results['network'] = {key: preferences.get(key) for key in ('dht', 'pex', 'lsd', 'upnp', 'current_interface_address', 'listen_port')}
        payload = secrets.token_bytes(2 * 1024 * 1024)
        seed_dir = ROOT / 'seed-data'
        (seed_dir / 'probe-files').mkdir(parents=True, exist_ok=True)
        (seed_dir / 'probe-files' / 'first.bin').write_bytes(payload[:1048576])
        (seed_dir / 'probe-files' / 'second.bin').write_bytes(payload[1048576:])
        info = {b'name': b'probe-files', b'piece length': 16384, b'pieces': b''.join(hashlib.sha1(payload[i:i + 16384]).digest() for i in range(0, len(payload), 16384)), b'files': [{b'length': 1048576, b'path': [b'first.bin']}, {b'length': 1048576, b'path': [b'second.bin']}]}
        torrent = bencode({b'info': info})
        hash_value = hashlib.sha1(bencode(info)).hexdigest()
        results['v1Hash'] = hash_value
        seed.call('torrents/add', {'savepath': seed_dir.as_posix(), 'stopped': 'false', 'autoTMM': 'false', 'contentLayout': 'Original'}, torrent)
        wait_for(seed, hash_value, lambda task: task['progress'] == 1)
        stage = ROOT / 'staging'
        stage.mkdir(exist_ok=True)
        stopped_stage = ROOT / 'stopped-add'
        stopped_stage.mkdir()
        client.call('torrents/add', {'savepath': stopped_stage.as_posix(), 'stopped': 'true', 'autoTMM': 'false', 'contentLayout': 'Original'}, torrent)
        stopped = wait_for(client, hash_value, lambda task: task['state'].startswith('stopped'))
        results['torrentStoppedAdd'] = {'state': stopped['state'], 'downloaded': stopped['downloaded']}
        results['v1Properties'] = {key: value for key, value in client.data('torrents/properties?hash=' + hash_value).items() if key.startswith('infohash')}
        client.call('torrents/delete', {'hashes': hash_value, 'deleteFiles': 'false'})
        time.sleep(1)
        magnet = 'magnet:?xt=urn:btih:' + hash_value + '&dn=local-only&x.pe=127.0.0.1:' + str(ports[2])
        client.call('torrents/add', {'urls': magnet, 'savepath': stage.as_posix(), 'stopped': 'false', 'stopCondition': 'MetadataReceived', 'autoTMM': 'false', 'contentLayout': 'Original'})
        wait_for(client, hash_value, lambda task: task['state'] in ('metaDL', 'forcedMetaDL', 'stoppedDL'))
        client.call('torrents/addPeers', {'hashes': hash_value, 'peers': '127.0.0.1:' + str(ports[2])})
        metadata = wait_for(client, hash_value, lambda task: task['state'] == 'stoppedDL' and task['total_size'] == len(payload), 45)
        time.sleep(3)
        metadata = client.data('torrents/info?hashes=' + hash_value)[0]
        files = client.data('torrents/files?hash=' + hash_value)
        results['magnetMetadataReceived'] = {'state': metadata['state'], 'downloaded': metadata['downloaded'], 'completed': metadata['completed'], 'files': files}
        assert metadata['downloaded'] == 0 and all(file['progress'] == 0 for file in files), '确认选集前出现载荷下载'
        client.call('torrents/filePrio', {'hash': hash_value, 'id': '1', 'priority': '0'})
        client.call('torrents/filePrio', {'hash': hash_value, 'id': '0', 'priority': '1'})
        client.call('torrents/start', {'hashes': hash_value})
        client.call('torrents/addPeers', {'hashes': hash_value, 'peers': '127.0.0.1:' + str(ports[2])})
        completed = wait_for(client, hash_value, lambda task: task['progress'] == 1 and task['amount_left'] == 0, 90, peer='127.0.0.1:' + str(ports[2]))
        results['selectedFileProgress'] = client.data('torrents/files?hash=' + hash_value)
        final = ROOT / 'complete'
        client.call('torrents/setLocation', {'hashes': hash_value, 'location': final.as_posix()})
        moved = wait_for(client, hash_value, lambda task: Path(task['save_path']).resolve() == final.resolve() and task['state'] != 'moving')
        assert (final / 'probe-files' / 'first.bin').read_bytes() == payload[:1048576]
        results['selectedTransferAndMove'] = {'state': moved['state'], 'selectedSize': completed['size'], 'savePathMatches': True, 'selectedBytesMatch': True, 'unselectedExists': (final / 'probe-files' / 'second.bin').exists()}
        client.call('torrents/delete', {'hashes': hash_value, 'deleteFiles': 'false'})
        time.sleep(1)
        results['removeKeepsFiles'] = (final / 'probe-files' / 'first.bin').exists()
        results['passed'] = True
    except Exception as error:
        results['passed'] = False
        results['error'] = str(error)
        raise
    finally:
        (ROOT / 'result.json').write_text(json.dumps(results, ensure_ascii=False, indent=2), encoding='utf-8')
        if client: client.close()
        if seed: seed.close()
        print(json.dumps(results, ensure_ascii=False, indent=2))

if __name__ == '__main__': main()
