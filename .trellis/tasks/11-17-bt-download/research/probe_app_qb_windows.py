"""在隔离本机双qB与真实栖库间验证磁力选集、实际传输和小说入库。"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import sys
import time
from typing import Any, Callable

import probe_qb_windows as probe

# 从当前仓库复用已有会话客户端，不复制HTTP和认证实现。
REPOSITORY: Path = Path(__file__).resolve().parents[4]
sys.path.insert(0, str(REPOSITORY / 'scripts'))
from verify import Client  # noqa: E402


def wait_task(client: Client, identifier: str, predicate: Callable[[dict[str, Any]], bool], engine: probe.QB, hash_value: str, peer: str, timeout: float = 90) -> dict[str, Any]:
    """有限等待真实应用对账，并仅重连明确允许的回环Peer。"""
    deadline = time.monotonic() + timeout
    next_peer = 0.0
    last: dict[str, Any] = {}
    while time.monotonic() < deadline:
        last = client.json('/api/downloads/' + identifier)['download']
        if predicate(last):
            return last
        if time.monotonic() >= next_peer and engine.data('torrents/info?hashes=' + hash_value):
            engine.call('torrents/addPeers', {'hashes': hash_value, 'peers': peer})
            next_peer = time.monotonic() + 5
        time.sleep(.3)
    raise AssertionError('真实应用任务未收敛：' + json.dumps(last, ensure_ascii=False))


def main() -> None:
    """创建全新隔离实例，退出时关闭本次启动的所有进程。"""
    parser = argparse.ArgumentParser(description=__doc__)
    # 已解包官方qB程序，不运行安装器或Docker。
    parser.add_argument('--qb-binary', type=Path, required=True, help='官方qbittorrent.exe路径')
    # 当前仓库构建的栖库程序。
    parser.add_argument('--nas-binary', type=Path, required=True, help='栖库可执行文件路径')
    # 只允许新建且含verify段的路径，避免碰触用户数据。
    parser.add_argument('--root', type=Path, required=True, help='全新verify隔离目录')
    args = parser.parse_args()
    root = args.root.resolve()
    if os.name != 'nt' or root.exists() or 'verify' not in root.parts or not args.qb_binary.is_file() or not args.nas_binary.is_file():
        parser.error('需要Windows、已有程序和全新verify目录')
    root.mkdir(parents=True)
    probe.ROOT = root
    probe.EXE = args.qb_binary.resolve()
    probe.PASS = secrets.token_urlsafe(24)
    listeners: list[socket.socket] = []
    for _ in range(5):
        listener = socket.socket()
        listener.bind(('127.0.0.1', 0))
        listeners.append(listener)
    ports = [listener.getsockname()[1] for listener in listeners]
    for listener in listeners:
        listener.close()
    results: dict[str, Any] = {'dockerExecuted': False, 'realApplication': True, 'realBTTransfer': False}
    seed: probe.QB | None = None
    engine: probe.QB | None = None
    process: subprocess.Popen[bytes] | None = None
    log: Any = None
    try:
        seed = probe.QB('seed-profile', ports[0], ports[2])
        engine = probe.QB('engine-profile', ports[1], ports[3])
        results['version'] = engine.call('app/version').decode()
        results['webApiVersion'] = engine.call('app/webapiVersion').decode()
        results['authLoginStatus'] = engine.auth_status
        results['authCookieName'] = engine.auth_cookie_name
        # 两篇各64KiB，文件边界与BT分片对齐，排除边界分片歧义。
        selected = ('本机真实BT小说验证。\n'.encode() * 1024).ljust(65536, b' ')
        ignored = ('未选择的第二篇小说。\n'.encode() * 1024).ljust(65536, b' ')
        payload = selected + ignored
        seed_root = root / 'seed-data'
        (seed_root / 'local-novel').mkdir(parents=True)
        (seed_root / 'local-novel' / 'selected.txt').write_bytes(selected)
        (seed_root / 'local-novel' / 'ignored.txt').write_bytes(ignored)
        info = {b'name': b'local-novel', b'piece length': 16384, b'pieces': b''.join(hashlib.sha1(payload[offset:offset + 16384]).digest() for offset in range(0, len(payload), 16384)), b'files': [{b'length': len(selected), b'path': [b'selected.txt']}, {b'length': len(ignored), b'path': [b'ignored.txt']}]}
        torrent = probe.bencode({b'info': info})
        hash_value = hashlib.sha1(probe.bencode(info)).hexdigest()
        peer = '127.0.0.1:' + str(ports[2])
        seed.call('torrents/add', {'savepath': seed_root.as_posix(), 'stopped': 'false', 'autoTMM': 'false', 'contentLayout': 'Original'}, torrent)
        probe.wait_for(seed, hash_value, lambda task: task['progress'] == 1)
        downloads = root / 'downloads'
        for path in ('data', 'media', 'downloads/staging', 'downloads/complete', 'downloads/complete/novel'):
            (root / path).mkdir(parents=True, exist_ok=True)
        password_file = root / 'qb-password'
        password_file.write_text(probe.PASS, encoding='utf-8')
        env = os.environ.copy()
        env.update({
            # 仅回环监听，不暴露验证实例。
            'NAS_ADDR': f'127.0.0.1:{ports[4]}',
            # 独立SQLite和衍生数据目录。
            'NAS_DATA_DIR': str(root / 'data'),
            # 普通根只授权media与complete，seed-data不属于媒体根。
            'NAS_MEDIA_ROOTS': os.pathsep.join((str(root / 'media'), str(downloads / 'complete'))),
            # 使用当前已构建静态文件；本探针只调用HTTP API。
            'NAS_WEB_DIR': str(REPOSITORY / 'web/dist'),
            # 避免周期扫描干扰显式发布断言。
            'NAS_SCAN_INTERVAL_MINUTES': '60',
            # 小样本不要求额外封面空间余量。
            'NAS_MIN_FREE_MB': '0',
            # 明确启用本次可选下载编排。
            'NAS_DOWNLOAD_ENABLED': 'true',
            # 应用看到的专用根。
            'NAS_DOWNLOAD_ROOT': str(downloads),
            # 引擎与应用本机路径指向同一实体。
            'NAS_DOWNLOAD_ENGINE_ROOT': downloads.as_posix(),
            # 后端固定连接隔离qB。
            'NAS_DOWNLOAD_QB_URL': engine.base,
            # 与探针配置对应的WebUI用户名。
            'NAS_DOWNLOAD_QB_USERNAME': 'probe',
            # 独立随机凭据文件，不写入结果与日志。
            'NAS_DOWNLOAD_QB_PASSWORD_FILE': str(password_file),
            # 小样本不要求GiB下载预留。
            'NAS_DOWNLOAD_MIN_FREE_GB': '0',
        })
        log = (root / 'application.log').open('ab')
        process = subprocess.Popen([str(args.nas_binary.resolve())], env=env, stdout=log, stderr=subprocess.STDOUT)
        client = Client(f'http://127.0.0.1:{ports[4]}')
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if process.poll() is not None:
                raise RuntimeError('隔离应用提前退出，请检查application.log')
            try:
                if client.request('/healthz')[0] == 200:
                    break
            except OSError:
                pass
            time.sleep(.2)
        client.json('/api/setup', 'POST', {'username': 'real-bt-probe', 'password': secrets.token_urlsafe(24)})
        status = client.json('/api/downloads/connection-test', 'POST', {})
        results['connectionStatus'] = status
        assert status['state'] == 'ready' and status['canSelectFiles'], json.dumps(status, ensure_ascii=False)
        # 无外部Tracker，唯一x.pe指向本机种源。
        magnet = 'magnet:?xt=urn:btih:' + hash_value + '&x.pe=' + peer
        task = client.json('/api/downloads', 'POST', {'requestId': secrets.token_hex(16), 'magnet': magnet, 'contentType': 'novel', 'createLibrary': True}, status=202)['download']
        task = wait_task(client, task['id'], lambda entry: entry['state'] == 'awaiting_selection', engine, hash_value, peer)
        snapshot = engine.data('torrents/info?hashes=' + hash_value)[0]
        assert snapshot['downloaded'] == 0 and not task['ready']
        results['metadataStoppedBeforePayload'] = {'engineState': snapshot['state'], 'downloadedBytes': snapshot['downloaded'], 'ready': task['ready']}
        files = client.json('/api/downloads/' + task['id'] + '/files')['files']
        index = next(entry['index'] for entry in files if entry['path'].endswith('selected.txt'))
        client.json('/api/downloads/' + task['id'] + '/selection', 'PUT', {'requestId': secrets.token_hex(16), 'manifestVersion': task['manifestVersion'], 'indices': [index]}, status=202)
        task = wait_task(client, task['id'], lambda entry: entry['importState'] == 'imported', engine, hash_value, peer, timeout=120)
        items = client.json('/api/downloads/' + task['id'] + '/items')['items']
        assert task['ready'] and task['selectionLocked'] and len(items) == 1
        assert items[0]['relativePath'].endswith('selected.txt')
        assert client.request(f'/api/items/{items[0]["id"]}/file')[2] == selected
        results['realBTTransfer'] = True
        final = engine.data('torrents/info?hashes=' + hash_value)[0]
        assert Path(final['save_path']).resolve().is_relative_to(downloads / 'complete' / 'novel')
        results['publishedNovel'] = {'ready': task['ready'], 'state': task['state'], 'importState': task['importState'], 'selectedBytes': len(selected), 'itemCount': len(items), 'sourceBytesMatch': True, 'finalPathUnderComplete': True, 'files': engine.data('torrents/files?hash=' + hash_value)}
        results['passed'] = True
    except Exception as error:
        results['passed'] = False
        results['error'] = str(error)
        raise
    finally:
        if process and process.poll() is None:
            process.terminate()
            process.wait(timeout=20)
        if log:
            log.close()
        if engine:
            engine.close()
        if seed:
            seed.close()
        (root / 'result.json').write_text(json.dumps(results, ensure_ascii=False, indent=2), encoding='utf-8')
        print(json.dumps(results, ensure_ascii=False, indent=2), flush=True)


if __name__ == '__main__':
    main()
