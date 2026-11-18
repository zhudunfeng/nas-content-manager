"""验证真实应用的托管引擎接入、凭据边界与来源热刷新；不连接 Docker 或外部站点。"""

from __future__ import annotations

import argparse
import http.server
import json
import os
from pathlib import Path
import secrets
import sqlite3
import threading
import time
from typing import Any
import urllib.parse

from verify import Client, generate_password
from verify_downloads import Harness


class DiscoveryFixture:
    """提供只监听回环的 Jackett 发现协议样本，不包含实际站点或 BT 载荷。"""

    def __init__(self) -> None:
        """建立带服务端随机密钥的隔离来源发现端点。"""
        # 私密测试密钥只驻留内存及忽略目录内连接文件。
        self.key: str = secrets.token_urlsafe(24)
        # 可切换的发现样本：空、少量、超过上限、恶意 XML。
        self.count: int = 0
        # 故意畸形的 XML 用于验证 DTD 拒绝与旧来源保留。
        self.dtd: bool = False
        # 只记录是否出现不应由初始化触发的关键词请求。
        self.searches: int = 0
        owner = self

        class Handler(http.server.BaseHTTPRequestHandler):
            """严格区分来源发现与关键词搜索，不输出 URL 或密钥。"""

            def log_message(self, format: str, *args: Any) -> None:
                """关闭可能记录查询密钥的默认访问日志。"""
                return

            def do_GET(self) -> None:
                """返回受控索引器清单并校验固定查询参数。"""
                query = urllib.parse.parse_qs(urllib.parse.urlsplit(self.path).query)
                if query.get('apikey') != [owner.key]:
                    self.send_error(403)
                    return
                if query.get('t') != ['indexers'] or query.get('configured') != ['true']:
                    owner.searches += 1
                    self.send_error(400)
                    return
                entries = ''.join(f'<indexer id="fixture_{i}" configured="true"><title>隔离来源 {i}</title></indexer>' for i in range(owner.count))
                prefix = '<!DOCTYPE indexers [<!ENTITY unsafe "refused">]>' if owner.dtd else ''
                body = (prefix + '<indexers>' + entries + '</indexers>').encode()
                self.send_response(200)
                self.send_header('Content-Type', 'application/xml')
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        # 动态回环端口避免占用用户应用端口。
        self.server: http.server.ThreadingHTTPServer = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        # 与主验证线程分离的受控 HTTP 服务线程。
        self.thread: threading.Thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        # 仅后端私密清单使用此地址。
        self.base: str = f'http://127.0.0.1:{self.server.server_port}'
        self.thread.start()

    def close(self) -> None:
        """停止受控端点并释放套接字。"""
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)


class BootstrapHarness(Harness):
    """使用真实 SQLite 与生产适配器验证托管模式，不模拟应用的运行状态。"""

    def __init__(self, binary: Path, root: Path) -> None:
        """复用原下载假引擎，在启动后才发布托管连接。"""
        super().__init__(binary, root)
        # 版本化连接目录模拟助手对应用的只读发布边界。
        self.connections: Path = root / 'connections'
        self.connections.mkdir()
        # 独立可切换的来源发现服务。
        self.discovery: DiscoveryFixture = DiscoveryFixture()
        self.env.update({
            # 显式 managed 优先于 Harness 原有 enabled 布尔开关。
            'NAS_DOWNLOAD_MODE': 'managed',
            # managed 初次没有来源配置，必须仍然能启动。
            'NAS_TORRENT_SEARCH_MODE': 'managed',
            # 本集成故意不接助手，验证控制面缺席不等于未安装。
            'NAS_ENGINE_HELPER_SOCKET': '',
            # 只读取本次隔离的固定文件名连接清单。
            'NAS_ENGINE_CONNECTION_DIR': str(self.connections),
        })

    def publish(self, generation: int = 1) -> None:
        """按实际私有协议原子发布两组件清单，密钥不进入浏览器。"""
        common: dict[str, Any] = {'version': 1, 'installationId': 'fixture-installation', 'generation': generation}
        files = {
            f'qbittorrent-{generation}.json': {**common, 'component': 'qbittorrent', 'containerId': 'fixture-qb-container', 'url': self.engine.base, 'username': 'verify', 'password': self.engine.password, 'apiKey': ''},
            f'jackett-{generation}.json': {**common, 'component': 'jackett', 'containerId': 'fixture-jackett-container', 'url': self.discovery.base, 'username': '', 'password': '', 'apiKey': self.discovery.key},
            'current.json': {'version': 1, 'installationId': common['installationId'], 'generation': generation, 'qbGeneration': generation, 'jackettGeneration': generation},
        }
        for name, value in files.items():
            temporary = self.connections / (name + '.tmp')
            temporary.write_text(json.dumps(value), encoding='utf-8')
            temporary.chmod(0o600)
            os.replace(temporary, self.connections / name)

    def run(self) -> None:
        """验证鉴权、只读状态、接入、重启、来源预算和损坏清单恢复。"""
        self.start()
        client = self.client
        anonymous = Client(self.base)
        routes = [('/api/engines', 'GET'), ('/api/engines/probe', 'POST'), ('/api/engines/initializations', 'POST'), ('/api/engines/initializations/fixture-task', 'GET'), ('/api/engines/initializations/fixture-task/retry', 'POST'), ('/api/engines/qbittorrent/start', 'POST'), ('/api/engines/search/sync-sources', 'POST')]
        for route, method in routes:
            client.check(anonymous.request(route, method, {} if method == 'POST' else None)[0] == 401, f'{method} {route} 要求管理员会话')
        client.json('/api/setup', 'POST', {'username': 'bootstrap-verify', 'password': generate_password()})
        for route, method in routes:
            if method == 'POST':
                client.check(client.request(route, method, {}, {'Origin': 'https://foreign.invalid'})[0] == 403, f'{route} 拒绝跨源修改')
        status = client.json('/api/engines')
        client.check(status['helper'] == 'not_enabled' and all(c['presence'] == 'unknown' and not c['canInitialize'] for c in status['components']), '未接助手显示未知而非未安装，禁止初始化')
        client.check(not client.json('/api/downloads/status')['enabled'], '没有托管凭据仍可启动，下载保持待接入')
        client.check(client.request('/api/engines/probe', 'POST', {'unexpected': True})[0] == 400, '拒绝未知 JSON 字段')
        client.check(client.request('/api/engines/probe', 'POST', {'unexpected': 'x' * 70000})[0] == 413, '部署接口保留64KiB请求预算')
        init = {'requestId': secrets.token_hex(16), 'components': ['qbittorrent'], 'noticeVersion': status['noticeVersion'], 'acceptNotice': False}
        client.check(client.request('/api/engines/initializations', 'POST', init)[0] == 409, '首次告知必须主动确认')
        init.update(acceptNotice=True)
        client.check(client.request('/api/engines/initializations', 'POST', init)[0] == 503, '助手未启用返回受控503，不假装启动部署')
        self.publish()
        status = client.json('/api/engines/probe', 'POST', {})
        components = {c['id']: c for c in status['components']}
        client.check(components['qbittorrent']['readiness'] == 'ready', '真实生产 qB 适配器验证后在线激活下载')
        client.check(components['jackett']['readiness'] == 'configuration_required', 'Jackett有密钥但无来源时显示待配置')
        for _ in range(3):
            client.json('/api/engines/probe', 'POST', {})
        client.check(self.engine.logins == 1, '重复清单与检测不重复创建下载客户端或登录')
        sync = '/api/engines/search/sync-sources'
        result = client.json(sync, 'POST', {})
        client.check(result['state'] == 'configuration_required' and result['candidates'] == [], '空来源清单不宣称可搜索')
        self.discovery.count = 9
        result = client.json(sync, 'POST', {})
        client.check(result['selectionRequired'] and len(result['candidates']) == 9 and result['selectedIds'] == [], '超过八个候选明确等待选择，不静默截断')
        client.check(client.request(sync, 'POST', {'sourceIds': ['fixture_0'] * 9})[0] == 400, '拒绝超过八个选择')
        client.check(client.request(sync, 'POST', {'sourceIds': ['not_discovered']})[0] == 400, '拒绝未知候选ID')
        result = client.json(sync, 'POST', {'sourceIds': ['fixture_0', 'fixture_1']})
        client.check(result['state'] == 'ready' and len(result['selectedIds']) == 2, '显式来源选择立即接入，不重启应用')
        client.check(client.json('/api/torrent-search/status')['enabled'], '原搜索接口读取同一已激活快照')
        self.discovery.dtd = True
        client.check(client.request(sync, 'POST', {})[0] == 502, '来源发现拒绝DTD并返回脱敏502')
        client.check(client.json('/api/torrent-search/status')['enabled'], '发现失败保留旧来源')
        self.discovery.dtd = False
        before = self.discovery.searches
        client.json('/api/engines')
        client.check(self.discovery.searches == before == 0, '初始化、轮询与来源同步均没有发起关键词站点搜索')
        public = json.dumps(client.json('/api/engines')) + json.dumps(client.json('/api/torrent-search/status'))
        client.check(self.engine.password not in public and self.discovery.key not in public and self.discovery.base not in public, '浏览器状态不包含密钥、密码或私有端点')
        (self.connections / 'current.json').write_text('{broken', encoding='utf-8')
        status = client.json('/api/engines/probe', 'POST', {})
        client.check(next(c for c in status['components'] if c['id'] == 'qbittorrent')['readiness'] == 'ready', '清单损坏保留已验证下载连接')
        self.publish(2)
        client.json('/api/engines/probe', 'POST', {})
        self.stop()
        self.start()
        status = client.json('/api/engines/probe', 'POST', {})
        client.check(next(c for c in status['components'] if c['id'] == 'qbittorrent')['readiness'] == 'ready' and client.json('/api/torrent-search/status')['enabled'], '应用重启恢复原实例与已选来源')
        with sqlite3.connect(self.root / 'data' / 'library.db') as connection:
            client.check(connection.execute('PRAGMA user_version').fetchone()[0] == 2, '主应用数据库仍为schema v2')
        client.check(not any(self.engine.password in line or self.discovery.key in line for line in (self.root / 'application.log').read_text(encoding='utf-8').splitlines()), '关键业务日志未输出凭据')
        print(f'托管接入验证通过：{client.passed} 项断言；助手Docker状态机及真实镜像需单独验收。', flush=True)

    def close(self) -> None:
        """释放本次应用、假引擎和发现服务，不清理用户资源。"""
        super().close()
        self.discovery.close()


def main() -> None:
    """解析带中文说明的隔离运行参数。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True, help='已经构建的本机栖库程序')
    parser.add_argument('--root', type=Path, default=Path('.runtime/verify/engines-' + time.strftime('%Y%m%d-%H%M%S')), help='不存在且包含verify路径段的隔离目录')
    args = parser.parse_args()
    root = args.root.resolve()
    if root.exists() or 'verify' not in root.parts or not args.binary.is_file():
        parser.error('二进制必须存在；隔离根必须不存在并包含verify路径段')
    root.mkdir(parents=True)
    harness = BootstrapHarness(args.binary.resolve(), root)
    completed = False
    try:
        harness.run()
        completed = True
    finally:
        harness.close()
        (root / 'result.json').write_text(json.dumps({'passed': completed, 'assertions': harness.client.passed, 'realApplication': True, 'engine': 'controlled protocol fixtures', 'dockerExecuted': False}, indent=2), encoding='utf-8')


if __name__ == '__main__':
    main()
