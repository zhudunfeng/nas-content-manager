"""以真实应用/获取服务和隔离文件验证收集闭环；BT使用可控假qB，不连接真实账号。"""

from __future__ import annotations

import sys

if __name__ == '__main__':
    sys.dont_write_bytecode = True

import argparse
import hashlib
import http.server
import json
import os
from pathlib import Path
import socket
import sqlite3
import subprocess
import threading
import time
from typing import Any, Callable
import urllib.request
import uuid
import wave
from xml.sax.saxutils import escape

from project_env import verification_directory
from verify import Client, generate_password
from verify_downloads import Fixture, Harness


def unused_port() -> int:
    """申请回环临时端口供隔离服务使用。"""
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        return int(listener.getsockname()[1])


class Feed:
    """提供有界RSS、ETag与本机生成的磁力链接。"""

    def __init__(self) -> None:
        """启动独立来源并记录条件请求次数。"""
        self.entries: list[Fixture] = []  # 仅当前测试生成的候选内容。
        self.conditional: int = 0  # 收到有效ETag的次数。
        owner = self

        class Handler(http.server.BaseHTTPRequestHandler):
            """处理测试RSS，不记录带秘密查询参数的URL。"""

            def log_message(self, format: str, *args: Any) -> None:
                """关闭原始HTTP日志，防止测试秘密被输出。"""

            def do_GET(self) -> None:
                """返回有限条目并遵守来源条件请求。"""
                etag = f'"fixture-{len(owner.entries)}"'
                if self.headers.get('If-None-Match') == etag:
                    owner.conditional += 1
                    self.send_response(304)
                    self.end_headers()
                    return
                items = ''.join('<item><guid>' + item.hash + '</guid><title>' + escape(item.name) + '</title><enclosure url="' + escape(item.magnet(), {'"': '&quot;'}) + '" length="' + str(sum(len(body) for _, body in item.files)) + '" type="application/x-bittorrent"/></item>' for item in owner.entries)
                body = ('<?xml version="1.0"?><rss version="2.0"><channel><title>隔离来源</title>' + items + '</channel></rss>').encode()
                self.send_response(200)
                self.send_header('Content-Type', 'application/rss+xml')
                self.send_header('Content-Length', str(len(body)))
                self.send_header('ETag', etag)
                self.end_headers()
                self.wfile.write(body)

        self.server: http.server.ThreadingHTTPServer = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)  # 仅回环来源。
        self.base: str = f'http://127.0.0.1:{self.server.server_port}/feed?token=collection-test-secret'  # 验证不得进入公开DTO/主库的私密地址。
        self.thread: threading.Thread = threading.Thread(target=self.server.serve_forever, daemon=True)  # 生命周期由close负责。
        self.thread.start()

    def close(self) -> None:
        """关闭本次来源和线程。"""
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)


class CollectionHarness(Harness):
    """管理真实主服务、真实worker和所有隔离样例。"""

    def __init__(self, args: argparse.Namespace, root: Path) -> None:
        """准备授权目录、工具路径、命名remote和私有凭据文件。"""
        super().__init__(args.binary.resolve(), root)
        self.args: argparse.Namespace = args  # 中文CLI参数定义在main中。
        self.feed: Feed = Feed()  # 可控RSS，不使用真实索引账号。
        self.worker: subprocess.Popen[bytes] | None = None  # 本次worker进程。
        self.worker_log: Any = None  # 不包含原始来源和工具输出的日志。
        self.worker_base: str = f'http://127.0.0.1:{unused_port()}'  # 私有执行端回环地址。
        self.worker_root: Path = root / 'collection'  # 状态、staging和complete同盘。
        self.input_root: Path = root / 'media' / 'music'  # 原文件默认只读登记。
        self.input_root.mkdir()
        with wave.open(str(self.input_root / '收集样本.wav'), 'wb') as audio:
            audio.setparams((1, 2, 8000, 0, 'NONE', 'not compressed'))
            audio.writeframes(b'\x00\x00' * 8000)
        (self.input_root / '忽略的说明.md').write_text('原文件保持不变。', encoding='utf-8')
        self.source_digest: str = hashlib.sha256((self.input_root / '收集样本.wav').read_bytes()).hexdigest()  # 源保护断言。
        secret_root = root / 'worker-secrets'
        secret_root.mkdir(mode=0o700)
        self.token_file: Path = secret_root / 'token'  # 首次启动由worker创建。
        sources = [{'id': 'fixture-local', 'kind': 'local', 'name': '隔离复制输入', 'path': str(self.input_root)}]
        if args.rclone:
            sources.append({'id': 'fixture-remote', 'kind': 'remote', 'name': '隔离命名remote', 'path': 'fixture:'})
        (secret_root / 'sources.json').write_text(json.dumps(sources, ensure_ascii=False), encoding='utf-8')
        remote_config = secret_root / 'rclone.conf'
        remote_config.write_text('[fixture]\ntype = alias\nremote = ' + str(self.input_root) + '\n', encoding='utf-8')
        if os.name != 'nt':
            remote_config.chmod(0o600)
        self.worker_env: dict[str, str] = {key: value for key, value in os.environ.items() if not key.startswith('NAS_')}  # 不继承真实服务配置。
        self.worker_env.update({
            'NAS_COLLECTION_WORKER_ADDR': self.worker_base.removeprefix('http://'),  # 仅回环监听。
            'NAS_COLLECTION_WORKER_DATA_DIR': str(self.worker_root),  # 本次专属持久根。
            'NAS_COLLECTION_WORKER_TOKEN_FILE': str(self.token_file),  # 本次专属令牌。
            'NAS_COLLECTION_WORKER_SOURCES_FILE': str(secret_root / 'sources.json'),  # 仅授权测试输入。
            'NAS_COLLECTION_WORKER_RCLONE_CONFIG_FILE': str(remote_config),  # 无真实云盘凭据。
            'NAS_COLLECTION_WORKER_MIN_FREE_MB': '0',  # 小样例不额外预留空间。
            'NAS_COLLECTION_WORKER_MAX_BYTES': '20971520',  # 单样例20MiB预算。
            'NAS_COLLECTION_WORKER_MAX_FILES': '100',  # 有界文件枚举。
            'NAS_COLLECTION_WORKER_MAX_CONCURRENT': '2',  # 受控并发。
            'NAS_COLLECTION_WORKER_YTDLP': str(args.ytdlp.resolve()) if args.ytdlp else '',  # 可选真实工具。
            'NAS_COLLECTION_WORKER_FFMPEG': str(args.ffmpeg.resolve()) if args.ffmpeg else '',  # 可选真实后处理。
            'NAS_COLLECTION_WORKER_RCLONE': str(args.rclone.resolve()) if args.rclone else '',  # 可选真实复制工具。
        })
        self.env.update({
            'NAS_COLLECTION_WORKER_URL': self.worker_base,  # 与上方同一受控实例。
            'NAS_COLLECTION_WORKER_TOKEN_FILE': str(self.token_file),  # 不在日志回显值。
            'NAS_COLLECTION_READ_ROOT': str(self.worker_root),  # 本机共享映射。
            'NAS_COLLECTION_WORKER_ALLOW_HTTP': 'true',  # 显式允许本次回环HTTP。
            'NAS_COLLECTION_MAX_BYTES': '20971520',  # 主服务同样限制20MiB。
            'NAS_COLLECTION_MAX_FILES': '100',  # 主服务同样限制条目数。
            'NAS_MEDIA_ROOTS': os.pathsep.join((str(root / 'media'), str(root / 'downloads/complete'), str(self.worker_root / 'complete'))),  # 不授权暂存和私密目录。
        })

    def start_worker(self) -> None:
        """启动真实worker并有界等待健康接口。"""
        self.worker_log = (self.root / 'worker.log').open('ab')
        self.worker = subprocess.Popen([str(self.args.worker_binary.resolve())], env=self.worker_env, stdout=self.worker_log, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            if self.worker.poll() is not None:
                raise RuntimeError('获取服务提前退出：' + (self.root / 'worker.log').read_text(encoding='utf-8')[-2000:])
            try:
                with urllib.request.urlopen(self.worker_base + '/healthz', timeout=1) as response:
                    if response.status == 200:
                        return
            except OSError:
                pass
            time.sleep(.2)
        raise RuntimeError('获取服务启动超时')

    def stop_worker(self) -> None:
        """停止本次worker，保留数据以验证重启对账。"""
        if self.worker and self.worker.poll() is None:
            self.worker.terminate()
            try:
                self.worker.wait(timeout=20)
            except subprocess.TimeoutExpired:
                self.worker.kill()
                self.worker.wait(timeout=5)
        if self.worker_log:
            self.worker_log.close()

    def source(self, kind: str, name: str, **values: Any) -> dict[str, Any]:
        """按正式完整配置创建测试来源。"""
        body: dict[str, Any] = {'kind': kind, 'name': name, 'enabled': True, 'rootId': '', 'path': '', 'allowPrivate': False, 'refreshMinutes': 30, 'outputProfile': 'video', 'externalUrl': ''}
        body.update(values)
        return self.client.json('/api/collection/sources', 'POST', body)['source']

    def wait_collection(self, id: str, predicate: Callable[[dict[str, Any]], bool], timeout: float = 90) -> dict[str, Any]:
        """等待真实持久收集任务，失败输出安全DTO。"""
        deadline = time.monotonic() + timeout
        last: dict[str, Any] = {}
        while time.monotonic() < deadline:
            last = self.client.json('/api/collection/tasks/' + id)['task']
            if predicate(last):
                return last
            time.sleep(.25)
        raise AssertionError('收集状态未收敛：' + json.dumps(last, ensure_ascii=False))

    def collect(self, source: dict[str, Any], kind: str = 'music', mode: str = 'register', url: str = '', profile: str = 'video') -> dict[str, Any]:
        """先预览再幂等提交，并验证实际媒体关联与原文件读取。"""
        preview = self.client.json('/api/collection/previews', 'POST', {'sourceId': source['id'], 'url': url, 'path': '', 'profile': profile, 'mode': mode})['preview']
        body = {'requestId': str(uuid.uuid4()), 'previewId': preview['id'], 'contentType': kind, 'createLibrary': True, 'libraryId': 0}
        task = self.client.json('/api/collection/tasks', 'POST', body)['task']
        repeated = self.client.json('/api/collection/tasks', 'POST', body)['task']
        self.client.check(repeated['id'] == task['id'], source['name'] + '：相同请求返回同一任务')
        conflict = dict(body, requestId=str(uuid.uuid4()))
        self.client.check(self.client.request('/api/collection/tasks', 'POST', conflict)[0] == 409, source['name'] + '：重复资源的新请求明确冲突，不留下未绑定幂等键')
        task = self.wait_collection(task['id'], lambda item: item['importState'] in ('imported', 'failed'))
        self.client.check(task['importState'] == 'imported' and bool(task['itemIds']), source['name'] + '：实际扫描覆盖后才入库完成')
        item_id = task['itemIds'][0]
        status, headers, body = self.client.request(f'/api/items/{item_id}/file', headers={'Range': 'bytes=0-31'})
        self.client.check(status == 206 and len(body) == 32 and headers['Content-Range'].startswith('bytes 0-31/'), source['name'] + '：入库文件支持鉴权Range读取')
        self.client.check(bool(self.client.json('/api/collection/tasks/' + task['id'] + '/events')['events']), source['name'] + '：中文过程事件持久化')
        return task

    def run(self) -> None:
        """验证全部可用适配器和来源、任务、发布、恢复边界。"""
        self.start_worker()
        self.start()
        credentials = {'username': 'collection-verify', 'password': generate_password()}
        self.client.json('/api/setup', 'POST', credentials)
        (self.root / 'app-credentials.json').write_text(json.dumps(credentials), encoding='utf-8')
        client = self.client
        self.client.check(Client(self.base).request('/api/collection/tasks')[0] == 401, '收集接口需要已有会话鉴权')
        capability = client.json('/api/collection/capabilities')
        client.check(capability['workerReady'], '真实worker身份、令牌和共享根探针通过')
        local_root = next(root['id'] for root in capability['roots'] if Path(root['path']).resolve() == (self.root / 'media').resolve())
        local = self.source('local', '本地只读音乐', rootId=local_root, path='music')
        registered = self.collect(local)
        copy = self.source('local', '本地归档副本', rootId='fixture-local')
        copied = self.collect(copy, mode='copy')
        if self.args.rclone:
            remote = self.source('remote', '真实rclone命名remote', rootId='fixture-remote')
            self.collect(remote, mode='copy')
        if self.args.web_url:
            web = self.source('web', '真实网页视频')
            self.collect(web, kind='video', mode='copy', url=self.args.web_url, profile='video')
            self.collect(web, kind='music', mode='copy', url=self.args.web_url, profile='audio')
        client.check(hashlib.sha256((self.input_root / '收集样本.wav').read_bytes()).hexdigest() == self.source_digest, '只读登记和归档复制均未改写输入媒体')
        client.check(client.request('/api/collection/previews', 'POST', {'sourceId': local['id'], 'url': '', 'path': '../escape', 'profile': 'audio', 'mode': 'register'})[0] == 400, '来源相对路径穿越被拒绝')
        self.rss()
        # 确认数据库增量迁移与秘密隔离；公开普通库不能保存来源查询令牌。
        with sqlite3.connect(self.root / 'data/library.db') as database:
            client.check(database.execute('PRAGMA user_version').fetchone()[0] == 4 and database.execute('PRAGMA integrity_check').fetchone()[0] == 'ok', 'v4增量数据库完整且保留原下载模块')
            dump = '\n'.join(database.iterdump())
            client.check('collection-test-secret' not in dump, 'RSS私密地址未保存到普通数据库')
        public = json.dumps(client.json('/api/collection/sources'), ensure_ascii=False)
        client.check('collection-test-secret' not in public and self.feed.base not in public, '来源公开DTO不含私密地址和令牌')
        self.stop()
        self.stop_worker()
        self.start_worker()
        self.start()
        client.json('/api/login', 'POST', credentials)
        restored = client.json('/api/collection/tasks/' + copied['id'])['task']
        client.check(restored['itemIds'] == copied['itemIds'] and restored['importState'] == 'imported', '主服务和worker重启后发布关联保留，不重复复制')
        client.check(client.json('/api/collection/tasks/' + registered['id'])['task']['importState'] == 'imported', '只读登记任务重启后仍保持真实入库结果')
        self.stop_worker()
        capability = client.json('/api/collection/capabilities')
        client.check(not capability['workerReady'] and bool(capability['workerReason']), '执行端断线显示明确不可用原因')
        client.check(client.request(f'/api/items/{restored["itemIds"][0]}/file', 'HEAD')[0] == 200, '执行端离线后已有发布媒体仍可读取')
        print(f'收集集成验证通过：{client.passed}项；真实rclone={bool(self.args.rclone)}；真实网页={bool(self.args.web_url)}；BT为可控假引擎。', flush=True)

    def rss(self) -> None:
        """验证首次基线、条件拉取、增量去重、人工BT选择与真实入库。"""
        client = self.client
        old = self.engine.register(Fixture('历史条目 E01', [('历史.txt', '历史条目'.encode())]))
        new = self.engine.register(Fixture('收集新条目 E02', [('新章节.txt', ('第二章\n采集验证。\n' * 20).encode())]))
        self.feed.entries = [old]
        source = self.source('rss', '隔离RSS', url=self.feed.base, allowPrivate=True)
        client.json('/api/collection/sources/' + source['id'] + '/refresh', 'POST', {'backfill': False})
        initial = client.json('/api/collection/candidates?sourceId=' + source['id'])['candidates']
        client.check(len(initial) == 1 and initial[0]['decision'] == 'baseline', 'RSS首次拉取只建立历史基线，不自动获取旧条目')
        self.feed.entries.append(new)
        refresh = '/api/collection/sources/' + source['id'] + '/refresh'
        client.json(refresh, 'POST', {'backfill': False})
        client.json(refresh, 'POST', {'backfill': False})
        candidates = client.json('/api/collection/candidates?sourceId=' + source['id'])['candidates']
        client.check(len(candidates) == 2 and self.feed.conditional > 0, 'RSS增量去重并发送ETag条件请求')
        candidate = next(item for item in candidates if item['title'] == new.name)
        body = {'requestId': str(uuid.uuid4()), 'revision': candidate['revision'], 'contentType': 'novel', 'createLibrary': True, 'libraryId': 0}
        task = client.json('/api/collection/candidates/' + candidate['id'] + '/acquire', 'POST', body)['task']
        task = self.wait_collection(task['id'], lambda item: bool(item['downloadId']))
        download = self.wait_task(task['downloadId'], lambda item: item['state'] == 'awaiting_selection')
        self.select(download, [0])
        self.wait_task(download['id'], lambda item: item['state'] == 'downloading')
        self.engine.complete(new.hash)
        imported = self.wait_collection(task['id'], lambda item: item['importState'] == 'imported')
        client.check(bool(imported['itemIds']) and imported['downloadId'] == download['id'], 'RSS候选经原BT人工选文件与发布门禁后关联实际入库媒体')
        additions = sum(endpoint == 'torrents/add' for endpoint, _ in self.engine.commands)
        client.json(refresh, 'POST', {'backfill': False})
        client.check(sum(endpoint == 'torrents/add' for endpoint, _ in self.engine.commands) == additions, '重复RSS拉取不会再次向下载引擎添加资源')
        # 删除媒体索引必须同时撤销收集关联，BT成品和做种数据仍保留。
        published = next((self.root / 'downloads' / 'complete').rglob('新章节.txt'))
        published_bytes = published.read_bytes()
        client.json('/api/libraries/' + str(imported['libraryId']), 'DELETE')
        invalidated = self.wait_collection(task['id'], lambda item: not item['itemIds'] and item['importState'] in ('failed', 'target_missing'))
        client.check(not invalidated['itemIds'] and invalidated['importState'] != 'imported', '删除BT媒体库立即撤销收集已入库关联，不保留失效媒体入口')
        client.check(published.read_bytes() == published_bytes and client.request('/api/items/' + str(imported['itemIds'][0]))[0] == 404, '删除媒体索引不改写BT发布文件，旧媒体记录不可继续访问')

    def close(self) -> None:
        """无论断言成功与否均释放本次服务和来源线程。"""
        self.stop_worker()
        self.feed.close()
        super().close()

    def serve_collection(self) -> None:
        """保留仅含隔离媒体的真实服务供浏览器验收，停止文件结束生命周期。"""
        self.start_worker()
        self.start()
        credentials = {'username': 'collection-browser', 'password': generate_password()}
        self.client.json('/api/setup', 'POST', credentials)
        credential_file = self.root / 'app-credentials.json'
        credential_file.write_text(json.dumps(credentials), encoding='utf-8')
        if os.name != 'nt':
            credential_file.chmod(0o600)
        capability = self.client.json('/api/collection/capabilities')
        local_root = next(entry['id'] for entry in capability['roots'] if Path(entry['path']).resolve() == (self.root / 'media').resolve())
        local = self.source('local', '浏览器本地音乐', rootId=local_root, path='music')
        self.collect(local)
        self.source('web', '浏览器网页入口')
        old = self.engine.register(Fixture('浏览器历史条目 E01', [('历史.txt', '历史'.encode())]))
        pending = self.engine.register(Fixture('浏览器候选 E02', [('章节.txt', '示例章节'.encode())]))
        self.feed.entries = [old]
        source = self.source('rss', '浏览器隔离RSS', url=self.feed.base, allowPrivate=True)
        self.client.json('/api/collection/sources/' + source['id'] + '/refresh', 'POST', {'backfill': False})
        self.feed.entries.append(pending)
        self.client.json('/api/collection/sources/' + source['id'] + '/refresh', 'POST', {'backfill': False})
        info = {'base': self.base, 'root': str(self.root), 'credentialFile': str(credential_file), 'localRoot': local_root, 'stopFile': str(self.root / 'stop')}
        (self.root / 'browser.json').write_text(json.dumps(info, ensure_ascii=False), encoding='utf-8')
        print(json.dumps(info, ensure_ascii=False), flush=True)
        while not (self.root / 'stop').exists():
            time.sleep(.5)


def main() -> None:
    """解析中文参数，拒绝覆盖任何现有目录或使用真实媒体根。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True, help='提前构建的主应用可执行文件')
    parser.add_argument('--worker-binary', type=Path, required=True, help='提前构建的获取执行端文件')
    parser.add_argument('--root', type=Path, default=verification_directory() / ('collection-' + time.strftime('%Y%m%d-%H%M%S')), help='不存在且包含verify路径段的隔离目录')
    parser.add_argument('--rclone', type=Path, help='可选真实rclone；通过不含账号的命名本地remote验证复制')
    parser.add_argument('--ytdlp', type=Path, help='可选真实yt-dlp固定路径')
    parser.add_argument('--ffmpeg', type=Path, help='可选真实FFmpeg固定路径')
    parser.add_argument('--web-url', default='', help='可选公共小样例媒体URL；显式启用真实视频和提取音频测试')
    parser.add_argument('--serve', action='store_true', help='保留隔离浏览器服务；创建根目录stop文件后关闭')
    args = parser.parse_args()
    root = args.root.resolve()
    if root.exists() or 'verify' not in root.parts:
        parser.error('--root必须为不存在且包含verify路径段的隔离目录')
    for path in (args.binary, args.worker_binary, args.rclone, args.ytdlp, args.ffmpeg):
        if path is not None and not path.is_file():
            parser.error('工具或应用文件不存在，请提前构建/配置')
    if args.web_url and not (args.ytdlp and args.ffmpeg):
        parser.error('--web-url需要同时提供真实--ytdlp和--ffmpeg')
    root.mkdir(parents=True)
    harness = CollectionHarness(args, root)
    passed = False
    try:
        if args.serve:
            harness.serve_collection()
        else:
            harness.run()
            passed = True
    finally:
        harness.close()
        result = {'passed': passed, 'assertions': harness.client.passed, 'realApplication': True, 'realWorker': True, 'realRclone': bool(args.rclone), 'realWeb': bool(args.web_url), 'realBTTransfer': False, 'dockerExecuted': False}
        if not args.serve:
            (root / 'result.json').write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding='utf-8')


if __name__ == '__main__':
    from project_env import run_cli

    raise SystemExit(run_cli(main))
