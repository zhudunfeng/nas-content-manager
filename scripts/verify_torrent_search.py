"""隔离验证真实应用的 Torznab 搜种与原 BT 衔接；仅使用本机协议样本，不运行 Docker。"""

from __future__ import annotations

import argparse
from concurrent.futures import ThreadPoolExecutor
import http.server
import json
import os
from pathlib import Path
import secrets
import threading
import time
from typing import Any
import urllib.parse
from xml.sax.saxutils import escape, quoteattr

from verify import Client, generate_password
from verify_downloads import Fixture, Harness


class FakeTorznab:
    """提供无外部网络的 Torznab、同源下载代理和恶意响应边界。"""

    def __init__(self, fixtures: dict[str, Fixture]) -> None:
        """初始化随机密钥和回环服务，仅供隔离验证使用。"""
        # 可下载内容均为本机生成的小样本，不连接真实 Tracker。
        self.fixtures: dict[str, Fixture] = fixtures
        # 密钥只放在被忽略的隔离配置文件中，不打印或写入验收记录。
        self.key: str = secrets.token_urlsafe(24)
        # 请求记录排除查询中的密钥，仅用于验证分类与凭据边界。
        self.requests: list[tuple[str, str, str, str]] = []
        # 下载代理访问计数，用于证明重启重试不会重复拉取。
        self.downloads: int = 0
        # 未授权目标被访问次数，应始终为零。
        self.trap_hits: int = 0
        # 可用标记文件在浏览器验证时模拟来源全部失联。
        self.offline_file: Path | None = None
        # 并发处理请求的短期记录锁。
        self.lock: threading.Lock = threading.Lock()
        owner = self

        class Handler(http.server.BaseHTTPRequestHandler):
            """只处理本脚本定义的受控协议，不输出私密访问日志。"""

            def log_message(self, format: str, *args: Any) -> None:
                """禁止默认日志记录包含 API Key 的 URL。"""
                return

            def reply(self, status: int, body: bytes, location: str = '') -> None:
                """发送有界测试响应，容忍应用按预算主动取消。"""
                try:
                    self.send_response(status)
                    self.send_header('Content-Type', 'application/xml; charset=utf-8')
                    self.send_header('Content-Length', str(len(body)))
                    if location:
                        self.send_header('Location', location)
                    self.end_headers()
                    self.wfile.write(body)
                except (ConnectionResetError, ConnectionAbortedError, BrokenPipeError):
                    # 超时或取消场景下，客户端关闭连接是预期行为。
                    return

            def do_GET(self) -> None:
                """提供标准 XML 和有限下载代理，不向第三方转发流量。"""
                parsed = urllib.parse.urlsplit(self.path)
                query = urllib.parse.parse_qs(parsed.query)
                keyword = query.get('q', [''])[0]
                with owner.lock:
                    owner.requests.append((parsed.path, keyword, query.get('cat', [''])[0], self.headers.get('Cookie', '')))
                if parsed.path.startswith('/trap'):
                    owner.trap_hits += 1
                    self.reply(200, b'should never be fetched')
                    return
                if parsed.path.startswith('/dl/'):
                    if query.get('jackett_apikey', [''])[0] != owner.key:
                        self.reply(403, b'private key rejected')
                        return
                    owner.downloads += 1
                    mode = query.get('path', ['torrent'])[0]
                    if mode == 'redirectmagnet':
                        self.reply(302, b'', owner.fixtures['redirectmagnet'].magnet())
                    elif mode == 'crossredirect':
                        self.reply(302, b'', 'http://localhost:' + str(owner.server.server_port) + '/trap?private=' + owner.key)
                    elif mode == 'invalidtorrent':
                        self.reply(200, b'<html>private upstream error</html>')
                    elif mode == 'oversizetorrent':
                        self.reply(200, b'x' * (8 * 1024 * 1024 + 1))
                    else:
                        self.reply(200, owner.fixtures['torrent'].torrent)
                    return
                if not parsed.path.startswith('/api/v2.0/indexers/'):
                    self.reply(404, b'')
                    return
                if query.get('apikey', [''])[0] != owner.key:
                    self.reply(403, b'private key rejected')
                    return
                if owner.offline_file and owner.offline_file.exists():
                    self.reply(503, ('private upstream ' + owner.key).encode())
                    return
                if query.get('t') == ['caps']:
                    self.reply(200, b'<caps><searching><search available="yes" supportedParams="q"/></searching></caps>')
                    return
                secondary = '/secondary/' in parsed.path
                if keyword == 'allfailed' or (secondary and keyword not in ('duplicates', 'many')):
                    self.reply(503, ('private upstream ' + owner.key).encode())
                    return
                if keyword == 'delay':
                    time.sleep(12)
                elif keyword.startswith('parallel'):
                    time.sleep(1)
                if keyword == 'broken':
                    self.reply(200, b'<rss><channel><item>')
                    return
                if keyword == 'huge':
                    self.reply(200, b'x' * (2 * 1024 * 1024 + 1))
                    return
                if keyword == 'doctype':
                    self.reply(200, b'<!DOCTYPE rss [<!ENTITY secret "blocked">]><rss><channel><title>&secret;</title></channel></rss>')
                    return
                if keyword == 'rss-error':
                    self.reply(200, ('<error code="100" description="' + owner.key + '"/>').encode())
                    return
                if keyword == 'empty':
                    items = ''
                elif keyword == 'duplicates':
                    items = owner.item('同一内容的不同来源', owner.fixtures['magnet'].magnet(), 'magnet') * 2
                elif keyword == 'many':
                    items = ''.join(owner.item(f'分页样本 {number:03d}', f'magnet:?xt=urn:btih:{number + (1000 if secondary else 0):040x}', '', number + 1) for number in range(130))
                elif keyword in ('torrent', 'redirectmagnet', 'crossredirect', 'invalidtorrent', 'oversizetorrent'):
                    items = owner.item(keyword, owner.download_url(keyword), 'torrent' if keyword == 'torrent' else '')
                elif keyword == 'crossurl':
                    items = owner.item('跨源地址不应请求', 'http://localhost:' + str(owner.server.server_port) + '/trap?private=' + owner.key)
                elif keyword == 'adminpath':
                    items = owner.item('同源管理路径不应请求', owner.base + '/trap?private=' + owner.key)
                elif keyword == 'unknown-values':
                    items = '<item><title>未知元数据</title><link>' + escape(owner.fixtures['magnet'].magnet()) + '</link><size>-1</size><tz:attr name="seeders" value="not-a-number"/><pubDate>invalid</pubDate></item>'
                elif keyword == 'key-boundary':
                    # 密钥横跨标题截断位置，必须先整体脱敏再截短。
                    items = owner.item('甲' * 500 + owner.key + '尾' * 40, owner.fixtures['magnet'].magnet())
                else:
                    items = owner.item('开源示例小说 · 磁力', owner.fixtures['magnet'].magnet(), 'magnet', 5368709243)
                    items += owner.item('本地示例合集 · 种子', owner.download_url('torrent'), 'torrent', 65536)
                    items += owner.item('<img src=x onerror=alert(1)> 仅文本标题 ' + '很长的名称' * 25, owner.download_url('redirectmagnet'), '', 1024)
                self.reply(200, ('<?xml version="1.0"?><rss version="2.0" xmlns:tz="http://torznab.com/schemas/2015/feed"><channel>' + items + '</channel></rss>').encode())

        # 随机本机端口避免触碰现有开发服务。
        self.server: http.server.ThreadingHTTPServer = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        self.server.daemon_threads = True
        # 搜索与下载代理使用同一固定 origin。
        self.base: str = f'http://127.0.0.1:{self.server.server_port}'
        # 生命周期线程由 close 完整回收。
        self.thread: threading.Thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def download_url(self, mode: str) -> str:
        """构造标准 Jackett 私密代理 URL，只在服务端 fixture 间流转。"""
        return self.base + '/dl/main/?' + urllib.parse.urlencode({'jackett_apikey': self.key, 'path': mode, 'file': 'fixture.torrent'})

    def item(self, title: str, link: str, fixture: str = '', size: int = 1234) -> str:
        """生成带可变命名空间前缀和真实 64 位体积的 RSS 条目。"""
        identity = '<tz:attr name="infohash" value="' + self.fixtures[fixture].v1 + '"/>' if fixture else ''
        return '<item><title>' + escape(title) + '</title><link>' + escape(link) + '</link><enclosure url=' + quoteattr(link) + ' type="application/x-bittorrent" length="' + str(size) + '"/><tz:attr name="seeders" value="12"/><tz:attr name="peers" value="18"/>' + identity + '<pubDate>Wed, 18 Nov 2026 08:00:00 +0800</pubDate></item>'

    def close(self) -> None:
        """关闭本机索引器并等待监听线程退出。"""
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=3)


class SearchHarness(Harness):
    """复用下载隔离环境，覆盖搜种到原选集、发布与阅读闭环。"""

    def __init__(self, binary: Path, root: Path) -> None:
        """增加只读来源配置与本机受控索引器。"""
        super().__init__(binary, root)
        # 分别覆盖磁力、HTTP种子及磁力重定向，确保三条路径身份不同。
        self.fixtures: dict[str, Fixture] = {name: self.engine.register(Fixture('搜种验证-' + name, [('第一章.txt', ('本机搜种示例 ' + name).encode()), ('第二章.txt', b'unselected')])) for name in ('magnet', 'torrent', 'redirectmagnet')}
        # 独立客户端不应该携带应用或 qB 会话。
        self.indexer: FakeTorznab = FakeTorznab(self.fixtures)
        self.indexer.offline_file = root / 'indexer-offline'
        # 来源配置只在忽略的 .runtime/verify 目录保存，不含真实站点信息。
        source_file = root / 'sources.json'
        source_file.write_text(json.dumps({'sources': [{'id': name, 'name': label, 'url': self.indexer.base + '/api/v2.0/indexers/' + name + '/results/torznab/api', 'apiKey': self.indexer.key} for name, label in (('main', '本机示例来源'), ('secondary', '故障演示来源'))]}, ensure_ascii=False), encoding='utf-8')
        if os.name != 'nt':
            source_file.chmod(0o600)
        self.env.update({
            # 仅启用当前隔离样本来源，不覆盖原下载连接。
            'NAS_TORRENT_SEARCH_ENABLED': 'true',
            # 配置由服务端读取，浏览器不能查看密钥或更换URL。
            'NAS_TORRENT_SEARCH_SOURCES_FILE': str(source_file),
        })

    def search(self, query: str, source: str = 'main', category: str = 'all', status: int = 200) -> dict[str, Any]:
        """通过真实鉴权入口提交受控关键词，不直接调用适配器。"""
        return self.client.json('/api/torrent-search', 'POST', {'query': query, 'sourceId': source, 'category': category}, status=status)

    def download(self, result: dict[str, Any], request_id: str | None = None, kind: str = 'novel', status: int = 202) -> dict[str, Any]:
        """把服务器随机结果令牌交给既有下载添加路由。"""
        return self.client.json('/api/downloads', 'POST', {'requestId': request_id or secrets.token_hex(16), 'searchResultId': result['id'], 'contentType': kind, 'createLibrary': True}, status=status)

    def run(self) -> None:
        """验证搜索、资源预算、凭据隔离、持久幂等与实际入库。"""
        self.start()
        client = self.client
        anonymous = Client(self.base)
        client.check(anonymous.request('/api/torrent-search/status')[0] == 401 and anonymous.request('/api/torrent-search', 'POST', {'query': '示例'})[0] == 401, '状态与搜索均要求已有管理员会话')
        client.json('/api/setup', 'POST', {'username': 'search-verify', 'password': generate_password()})
        client.json('/api/downloads/connection-test', 'POST', {})
        status = client.json('/api/torrent-search/status')
        public = json.dumps(status)
        client.check(status['enabled'] and len(status['sources']) == 2 and self.indexer.key not in public and self.indexer.base not in public, '状态只返回安全来源名称与额度')
        client.check(client.request('/api/torrent-search', 'POST', {'query': '示例'}, {'Origin': 'https://foreign.invalid'})[0] == 403, '搜索沿用同源写请求保护')
        for query in ('a', '字' * 121, 'bad\x00query'):
            client.check(client.request('/api/torrent-search', 'POST', {'query': query})[0] == 400, '拒绝过短、过长或控制字符关键词')
        client.check(client.request('/api/torrent-search', 'POST', {'query': '示例', 'sourceId': 'unknown'})[0] == 400, '拒绝未配置来源标识')
        client.check(client.request('/api/torrent-search', 'POST', {'query': '示例', 'category': 'arbitrary'})[0] == 400, '拒绝未定义分类')
        client.check(client.request('/api/torrent-search', 'POST', {'query': 'x' * 70000})[0] == 413, '搜索请求仍受64KiB JSON上限')
        result = self.search('示例关键词', '')
        serialized = json.dumps(result, ensure_ascii=False)
        client.check(len(result['results']) == 3 and any(source['state'] == 'error' for source in result['sources']), '来源部分离线仍返回成功来源与独立失败提示')
        client.check(self.indexer.key not in serialized and self.indexer.base not in serialized and 'magnet:?' not in serialized, '浏览器响应不包含API Key、私密URL或磁力')
        client.check(result['results'][0]['sizeBytes'] == 5368709243, '搜索结果保留超过4GiB的64位体积')
        client.check(result['results'][0]['seeders'] == 12 and result['results'][0]['publishedAt'] != '', '兼容不同XML前缀的做种属性与RSS发布时间')
        empty = self.search('empty')
        client.check(empty['results'] == [] and empty['sources'][0]['state'] == 'ready', '空结果区别于来源请求失败')
        unknown = self.search('unknown-values')['results'][0]
        client.check(unknown['sizeBytes'] is None and unknown['seeders'] is None and unknown['publishedAt'] == '', '非法或缺失元数据展示未知而非捏造零')
        boundary = self.search('key-boundary')['results'][0]['title']
        client.check(self.indexer.key[:12] not in boundary and len(boundary) <= 513, '标题中的密钥先脱敏再截断，不泄漏跨边界密钥前缀')
        self.search('分类测试', category='video')
        self.search('分类测试', category='music')
        self.search('分类测试', category='book')
        categories = [record[2] for record in self.indexer.requests if record[1] == '分类测试']
        client.check('2000,5000' in categories and '3000' in categories and '7000' in categories, '影视、音乐和书籍分类传递标准Torznab类别')
        duplicate = self.search('duplicates', '')
        client.check(len(duplicate['results']) == 1, '同身份的站内和跨来源结果去重')
        many = self.search('many', '')
        client.check(0 < len(many['results']) <= 200 and many['truncated'], '不听从上游无界结果，每来源与总体都有上限及截断提示')
        for query in ('allfailed', 'broken', 'huge', 'doctype', 'rss-error'):
            response = self.search(query, status=502)
            client.check('error' in response and self.indexer.key not in json.dumps(response), '上游失败、损坏、超量、DOCTYPE及协议错误均有脱敏失败响应')
        started = time.monotonic()
        self.search('delay', status=502)
        client.check(time.monotonic() - started < 13, '单源超时在预算内退出')
        with ThreadPoolExecutor(max_workers=3) as pool:
            statuses = list(pool.map(lambda number: client.request('/api/torrent-search', 'POST', {'query': 'parallel-' + str(number), 'sourceId': 'main'})[0], range(3)))
        client.check(429 in statuses and statuses.count(200) <= 2, '搜索并发超限返回429且不排队耗尽NAS资源')
        for query in ('crossurl', 'adminpath'):
            blocked = self.search(query)['results'][0]
            code, _, _ = client.request('/api/downloads', 'POST', {'requestId': secrets.token_hex(16), 'searchResultId': blocked['id'], 'contentType': 'novel', 'createLibrary': True})
            client.check(not blocked['downloadable'] and code == 422 and self.indexer.trap_hits == 0, '跨源和同源非代理路径不可下载且无外部请求')
        for query, codes in (('crossredirect', (502,)), ('invalidtorrent', (502,)), ('oversizetorrent', (413,))):
            candidate = self.search(query)['results'][0]
            code, _, body = client.request('/api/downloads', 'POST', {'requestId': secrets.token_hex(16), 'searchResultId': candidate['id'], 'contentType': 'novel', 'createLibrary': True})
            client.check(code in codes and self.indexer.key.encode() not in body and self.indexer.trap_hits == 0, '拒绝跨源重定向、伪种子与超过8MiB种子')
        # 所有新任务仍须进入原受管选择阶段，搜种不能直接启动载荷。
        candidate = self.search('torrent')['results'][0]
        request_id = secrets.token_hex(16)
        first = self.download(candidate, request_id)['download']
        second = self.download(candidate, request_id)['download']
        client.check(first['id'] == second['id'], '相同搜索添加请求键始终返回原受管任务')
        client.check(self.download(candidate, request_id, kind='music', status=409).get('error') is not None, '重用请求键但改变内容类型被拒绝')
        task = self.wait_task(first['id'], lambda item: item['state'] == 'awaiting_selection')
        client.check(not task['ready'] and not task['selectionLocked'], '搜种下载先等待选集且尚未发布')
        self.select(task, [0])
        self.wait_task(task['id'], lambda item: item['state'] == 'downloading')
        self.engine.complete(self.fixtures['torrent'].hash)
        imported = self.wait_task(task['id'], lambda item: item['importState'] == 'imported', timeout=60)
        items = client.json('/api/downloads/' + task['id'] + '/items')['items']
        content = client.request('/api/items/' + str(items[0]['id']) + '/file')[2]
        client.check(imported['ready'] and len(items) == 1 and content == self.fixtures['torrent'].files[0][1], '搜索种子经选集、异步移动与发布后仅导入所选小说并读取原字节')
        redirected = self.search('redirectmagnet')['results'][0]
        redirected_task = self.download(redirected)['download']
        self.wait_task(redirected_task['id'], lambda item: item['state'] == 'awaiting_selection')
        client.check(True, '同源种子代理重定向到磁力后沿用元数据停止与选集')
        magnet = self.search('直接磁力')['results'][0]
        magnetic_task = self.download(magnet)['download']
        self.wait_task(magnetic_task['id'], lambda item: item['state'] == 'awaiting_selection')
        client.check(True, 'Torznab直接磁力沿用原下载入口')
        client.check(all(not cookie for _, _, _, cookie in self.indexer.requests), '来源请求不会携带应用或qB的Cookie')
        fetched = self.indexer.downloads
        self.stop()
        self.start()
        client.json('/api/downloads/connection-test', 'POST', {})
        replay = self.download(candidate, request_id)['download']
        client.check(replay['id'] == first['id'] and self.indexer.downloads == fetched, '应用重启丢失搜索缓存后仍按持久请求键恢复而不重复取种')
        code, _, _ = client.request('/api/downloads', 'POST', {'requestId': secrets.token_hex(16), 'searchResultId': candidate['id'], 'contentType': 'novel', 'createLibrary': True})
        client.check(code in (404, 410), '新请求不能在重启后使用失效结果令牌')
        self.stop()
        self.env['NAS_TORRENT_SEARCH_ENABLED'] = 'false'
        self.start()
        request_count = len(self.indexer.requests)
        client.check(not client.json('/api/torrent-search/status')['enabled'] and client.request('/api/torrent-search', 'POST', {'query': '示例'})[0] == 503 and len(self.indexer.requests) == request_count, '关闭搜种后不访问任何来源且原任务仍可读取')
        client.check(len(client.json('/api/downloads')['downloads']) >= 3, '关闭搜种不删除已有下载和已发布媒体')
        log = (self.root / 'application.log').read_text(encoding='utf-8')
        client.check(self.indexer.key not in log and self.engine.password not in log and self.indexer.download_url('torrent') not in log, '关键业务日志不包含来源密钥、下载器密码与私密URL')
        print(f'搜种隔离集成通过：{client.passed} 项断言；真实Go应用 + 受控Torznab/qB，未执行Docker或真实站点搜索。', flush=True)

    def serve(self) -> None:
        """为浏览器检查保留真实应用和受控来源，停止文件触发清理。"""
        self.start()
        credentials = {'username': 'search-browser', 'password': generate_password()}
        self.client.json('/api/setup', 'POST', credentials)
        credential_file = self.root / 'app-credentials.json'
        credential_file.write_text(json.dumps(credentials), encoding='utf-8')
        if os.name != 'nt':
            credential_file.chmod(0o600)
        print(json.dumps({'base': self.base, 'credentialFile': str(credential_file), 'stopFile': str(self.root / 'stop')}, ensure_ascii=False), flush=True)
        while not (self.root / 'stop').exists():
            self.engine.offline = (self.root / 'engine-offline').exists()
            time.sleep(.5)

    def close(self) -> None:
        """异常或正常结束均回收应用与两类协议服务。"""
        super().close()
        self.indexer.close()


def main() -> None:
    """解析明确的隔离路径，拒绝复用现有数据或执行部署命令。"""
    parser = argparse.ArgumentParser(description=__doc__)
    # 被测二进制由独立构建产生，不在脚本中安装工具链。
    parser.add_argument('--binary', required=True, type=Path, help='已构建的栖库可执行文件')
    # 全新根目录必须含 verify 段，以免修改真实媒体和已有应用数据。
    parser.add_argument('--root', type=Path, default=Path('.runtime/verify/search-' + time.strftime('%Y%m%d-%H%M%S') + '-' + secrets.token_hex(3)), help='全新隔离验证目录')
    # 浏览器模式与自动断言分离，不能把准备fixture声称为验收通过。
    parser.add_argument('--serve', action='store_true', help='保持样本直到根目录出现stop文件')
    args = parser.parse_args()
    binary = args.binary.resolve()
    root = args.root.resolve()
    if not binary.is_file() or root.exists() or 'verify' not in root.parts:
        parser.error('需要已构建二进制和不存在且包含verify段的隔离目录')
    root.mkdir(parents=True)
    harness = SearchHarness(binary, root)
    passed = False
    try:
        if args.serve:
            harness.serve()
        else:
            harness.run()
            passed = True
    finally:
        harness.close()
        if not args.serve:
            (root / 'result.json').write_text(json.dumps({'passed': passed, 'assertions': harness.client.passed, 'realApplication': True, 'providers': 'controlled Torznab and qB', 'dockerExecuted': False, 'realSiteSearch': False}, ensure_ascii=False, indent=2), encoding='utf-8')


if __name__ == '__main__':
    main()
