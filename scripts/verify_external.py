"""以真实隔离应用和四类本地 HTTP 服务验证外部控制；不连接实际账号或媒体。"""

from __future__ import annotations

import sys

if __name__ == '__main__':
    # 直接入口先关闭源码旁缓存，所有临时文件由项目包装器管理。
    sys.dont_write_bytecode = True

import argparse
from contextlib import closing
import http.server
import json
import os
from pathlib import Path
import secrets
import socket
import sqlite3
import threading
import time
from typing import Any
import urllib.parse
import uuid

from project_env import verification_directory
from verify import Client, generate_password
from verify_downloads import Harness
from verify_schema import create_legacy_database


class ExternalFixture:
    """仅在回环地址模拟固定官方协议，原始认证和写入内容只留内存。"""

    def __init__(self) -> None:
        """初始化受控服务数据、断线开关和各协议的独立密钥。"""
        self.lock: threading.RLock = threading.RLock()  # 同步请求记录与受控状态。
        self.keys: dict[str, str] = {name: 'external-key-' + secrets.token_hex(20) for name in ('sonarr', 'radarr', 'moviepilot', 'autobangumi')}  # 只读本次测试认证。
        self.accepted_keys: dict[str, set[str]] = {name: {value} for name, value in self.keys.items()}  # 允许测试显式轮换密钥。
        self.events: list[tuple[str, str, str, dict[str, Any], dict[str, list[str]]]] = []  # 不落盘的真实请求记录。
        self.errors: list[str] = []  # 只保存固定中文协议失败标签。
        self.drop_next: tuple[str, str, str] | None = None  # 接受指定写入后断开响应，制造真实不确定提交。
        self.mp_partial_next: str = ''  # 指定MP写入执行后返回业务失败，模拟部分成功。
        self.redirect: set[str] = set()  # 指定服务所有请求返回跨源重定向。
        self.offline: set[str] = set()  # 指定服务返回503而不是空列表。
        self.trap_hits: int = 0  # 重定向目标实际命中次数。
        self.rss_secret: str = 'rss-private-' + secrets.token_hex(18)  # 验证仅写RSS地址不进入主库。
        self.remote_root: str = '/private/external-root-marker/library'  # 外部目录只在适配器私密配置中出现。
        self.resources: dict[str, list[dict[str, Any]]] = {}  # Servarr受控媒体实体。
        for service in ('sonarr', 'radarr'):
            identity = 'tvdbId' if service == 'sonarr' else 'tmdbId'
            self.resources[service] = [{'id': 1, identity: 100, 'title': service + ' 现有媒体', 'year': 2025, 'monitored': True, 'status': 'continuing', 'qualityProfileId': 2, 'rootFolderPath': self.remote_root, 'path': self.remote_root + '/existing'}]
        self.subscriptions: list[dict[str, Any]] = [{'id': 1, 'name': 'MoviePilot 现有订阅', 'type': '电视剧', 'year': '2025', 'season': 1, 'state': 'P', 'media_source': 'themoviedb', 'media_id': '100'}]  # P仍是运行状态。
        self.feeds: list[dict[str, Any]] = [{'id': 1, 'name': 'AB 现有RSS', 'url': 'https://rss.invalid/feed?passkey=' + self.rss_secret, 'enabled': True, 'aggregate': False, 'parser': 'mikan'}]  # RSS地址只在内存。
        self.bangumi: list[dict[str, Any]] = [{'id': 1, 'official_title': 'AB 归档规则', 'title_raw': 'AB 原始规则', 'season': 1, 'deleted': False, 'archived': True}]  # 归档与启用独立。
        self.mp_secret: str = 'mp-context-private-' + secrets.token_hex(18)  # 资源链接和Cookie共用的泄漏检测标记。
        self.mp_downloader: str = 'private-downloader-' + secrets.token_hex(8)  # 仅供服务端精确下载控制的名称。
        self.mp_download_root: str = '/private/mp-downloads-' + secrets.token_hex(8)  # 已配置外部下载根，不是本地应用授权。
        self.mp_library_root: str = '/private/mp-library-' + secrets.token_hex(8)  # 已配置外部媒体库根。
        self.mp_overwrite: str = 'never'  # 可切换目标目录策略验证提交前重读。
        self.mp_version: str = 'v3.2.8'  # 精确工作流版本，可切换以验证能力禁用。
        self.mp_file_size: int = 1048576  # 可变化文件版本，用于执行前重复校验。
        self.mp_large_releases: bool = False  # 控制超过1000项的上游资源响应。
        self.mp_outside_file: bool = False  # 控制越界目录条目响应。
        self.mp_media: dict[str, Any] = {'media_source': 'themoviedb', 'media_id': '42', 'tmdb_id': 42, 'type': '电影', 'title': '隔离电影', 'year': '2025', 'overview': '受控元数据'}  # 固定TMDB身份。
        self.mp_torrent: dict[str, Any] = {'title': '隔离电影.1080p', 'description': '受控候选', 'site': 1, 'site_name': '受控站点', 'site_cookie': self.mp_secret, 'site_ua': 'Fixture', 'enclosure': 'https://torrent.invalid/file?passkey=' + self.mp_secret, 'seeders': 12, 'size': 1048576}  # 完整上下文只在私密快照流转。
        self.mp_downloads: list[dict[str, Any]] = [{'hash': 'b' * 40, 'downloader': self.mp_downloader, 'name': 'MP受控下载', 'title': '隔离电影', 'state': 'downloading', 'progress': 25.0, 'path': self.mp_download_root + '/电影.mkv', 'save_path': self.mp_download_root}]  # 外部任务唯一身份为下载器和hash。
        self.mp_scheduler: dict[str, Any] = {'id': 'mediaserver_sync', 'name': '同步媒体服务器', 'provider': 'MoviePilot', 'status': '等待', 'progress': 0.0, 'progress_enable': False, 'progress_detail': {'id': 'mediaserver_sync', 'enable': False, 'value': 0.0, 'status': 'waiting', 'success': None}}  # 固定索引同步任务。
        fixture = self

        class Handler(http.server.BaseHTTPRequestHandler):
            """通过固定分派函数响应请求，禁用默认含URL访问日志。"""

            def log_message(self, format: str, *args: Any) -> None:
                """避免默认日志记录私密请求地址和参数。"""

            def do_GET(self) -> None:
                """处理固定协议读取和真实GET副作用端点。"""
                self.handle_request()

            def do_POST(self) -> None:
                """处理单次明确业务提交。"""
                self.handle_request()

            def do_PUT(self) -> None:
                """处理固定实体状态更新。"""
                self.handle_request()

            def do_PATCH(self) -> None:
                """处理AB单字段启用变更。"""
                self.handle_request()

            def send_json(self, status: int, value: Any) -> None:
                """返回有界JSON，不回显入参或认证。"""
                data = json.dumps(value, ensure_ascii=False).encode()
                self.send_response(status)
                self.send_header('Content-Type', 'application/json')
                self.send_header('Content-Length', str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def handle_request(self) -> None:
                """验证认证后记录真实协议，并按需模拟响应丢失。"""
                parsed = urllib.parse.urlsplit(self.path)
                if parsed.path == '/trap':
                    fixture.trap_hits += 1
                    self.send_json(200, {})
                    return
                parts = parsed.path.split('/', 2)
                service = parts[1] if len(parts) > 1 else ''
                route = '/' + parts[2] if len(parts) > 2 else '/'
                query = urllib.parse.parse_qs(parsed.query)
                with fixture.lock:
                    if service not in fixture.keys:
                        self.send_json(404, {'error': '未知测试服务'})
                        return
                    expected = fixture.accepted_keys[service]
                    auth = self.headers.get('Authorization', '') if service == 'autobangumi' else self.headers.get('X-API-KEY', '')
                    valid = auth.removeprefix('Bearer ') in expected if service == 'autobangumi' else auth in expected
                    if not valid or service == 'autobangumi' and not auth.startswith('Bearer '):
                        fixture.errors.append('外部认证头不符合固定协议')
                        self.send_json(401, {})
                        return
                    if self.headers.get('Content-Type') != 'application/json':
                        fixture.errors.append('外部请求未声明JSON类型')
                        self.send_json(400, {})
                        return
                    if service in fixture.redirect:
                        self.send_response(302)
                        self.send_header('Location', fixture.base.replace('127.0.0.1', 'localhost') + '/trap')
                        self.end_headers()
                        return
                    if service in fixture.offline:
                        self.send_json(503, {'error': fixture.keys[service]})
                        return
                    try:
                        size = int(self.headers.get('Content-Length', '0'))
                        assert 0 <= size <= 65536
                        body = json.loads(self.rfile.read(size)) if size else {}
                        assert isinstance(body, dict)
                        fixture.events.append((service, self.command, route, body, query))
                        status, result = fixture.dispatch(service, self.command, route, query, body)
                    except (AssertionError, ValueError, KeyError, TypeError):
                        fixture.errors.append('固定协议请求载荷不匹配')
                        self.send_json(400, {})
                        return
                    if fixture.drop_next == (service, self.command, route):
                        fixture.drop_next = None
                        self.close_connection = True
                        try:
                            self.connection.shutdown(socket.SHUT_RDWR)
                        except OSError:
                            pass
                        self.connection.close()
                        return
                    self.send_json(status, result)

        self.server: http.server.ThreadingHTTPServer = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)  # 只监听回环随机端口。
        self.base: str = f'http://127.0.0.1:{self.server.server_port}'  # 各服务带独立反代前缀。
        self.thread: threading.Thread = threading.Thread(target=self.server.serve_forever, daemon=True)  # close统一回收线程。
        self.thread.start()

    def close(self) -> None:
        """关闭HTTP监听器并有限等待线程退出。"""
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)

    def mutations(self, service: str = '') -> list[tuple[str, str, str, dict[str, Any], dict[str, list[str]]]]:
        """返回写请求内存快照，读取列表不计入副作用。"""
        with self.lock:
            return [event for event in self.events if self.is_mutation(event) and (not service or event[0] == service)]

    def is_mutation(self, event: tuple[str, str, str, dict[str, Any], dict[str, list[str]]]) -> bool:
        """按官方语义区分POST只读浏览与GET下载控制的真实副作用。"""
        service, method, route, body, _ = event
        if service == 'moviepilot':
            if route == '/api/v1/storage/list' or route == '/api/v1/transfer/manual' and body.get('preview') is True:
                return False
            if route.startswith(('/api/v1/download/start/', '/api/v1/download/stop/')) or route == '/api/v1/system/runscheduler':
                return True
        return method != 'GET'

    def dispatch(self, service: str, method: str, route: str, query: dict[str, list[str]], body: dict[str, Any]) -> tuple[int, Any]:
        """只分派四个服务已核实的官方路由。"""
        if service in ('sonarr', 'radarr'):
            return self.servarr(service, method, route, query, body)
        if service == 'moviepilot':
            status, result = self.moviepilot(method, route, query, body)
            if self.mp_partial_next == route and method != 'GET':
                self.mp_partial_next = ''
                return status, {'success': False, 'message': self.mp_secret, 'data': result}
            return status, {'success': status < 300, 'message': '', 'data': result}
        return self.autobangumi(method, route, query, body)

    def servarr(self, service: str, method: str, route: str, query: dict[str, list[str]], body: dict[str, Any]) -> tuple[int, Any]:
        """模拟Sonarr4/Radarr7公开API及单实体变更。"""
        prefix = '/api/v3'
        media = '/series' if service == 'sonarr' else '/movie'
        identity = 'tvdbId' if service == 'sonarr' else 'tmdbId'
        resources = self.resources[service]
        if route == prefix + '/system/status' and method == 'GET':
            return 200, {'appName': 'Sonarr' if service == 'sonarr' else 'Radarr', 'version': '4.0.0' if service == 'sonarr' else '7.0.0'}
        if route == prefix + '/health' and method == 'GET':
            return 200, []
        if route in (prefix + '/queue', prefix + '/history') and method == 'GET':
            assert query.get('pageSize') == ['50']
            return 200, {'page': int(query['page'][0]), 'pageSize': 50, 'totalRecords': 1, 'records': [{'id': 1, 'title': '受控下载', 'sourceTitle': '受控历史', 'status': 'downloading', 'eventType': 'grabbed', 'date': '2026-11-20T00:00:00Z'}]}
        if route == prefix + '/rootfolder' and method == 'GET':
            return 200, [{'id': 1, 'path': self.remote_root, 'accessible': True}]
        if route == prefix + '/qualityprofile' and method == 'GET':
            return 200, [{'id': 2, 'name': '标准1080p'}]
        if route == prefix + media + '/lookup' and method == 'GET':
            term = query.get('term', [''])[0]
            external_id = int(term.split(':', 1)[1]) if ':' in term else 1001
            return 200, [{identity: external_id, 'title': '受控元数据 ' + str(external_id), 'year': 2025}]
        if route == prefix + media and method == 'GET':
            selected = query.get(identity)
            return 200, [item for item in resources if not selected or item[identity] == int(selected[0])]
        if route == prefix + media and method == 'POST':
            assert body['qualityProfileId'] == 2 and body['rootFolderPath'] == self.remote_root and body['monitored'] is True
            if service == 'sonarr':
                assert body['seriesType'] == 'standard' and body['seasonFolder'] is True and body['monitorNewItems'] == 'all'
                assert body['addOptions'] == {'monitor': 'all', 'searchForMissingEpisodes': False, 'searchForCutoffUnmetEpisodes': False}
            else:
                assert body['minimumAvailability'] == 'released' and body['addOptions'] == {'monitor': 'movieOnly', 'searchForMovie': False}
            item = dict(body, id=len(resources) + 1, path=self.remote_root + '/added-' + str(body[identity]), status='released', year=2025)
            resources.append(item)
            return 201, item
        if route == prefix + media + '/editor' and method == 'PUT':
            ids_key = 'seriesIds' if service == 'sonarr' else 'movieIds'
            assert set(body) == {ids_key, 'monitored'} and len(body[ids_key]) == 1
            target = next(item for item in resources if item['id'] == body[ids_key][0])
            target['monitored'] = body['monitored']
            return 200, [target]
        if route.startswith(prefix + media + '/') and method == 'GET':
            target_id = int(route.rsplit('/', 1)[1])
            return 200, next(item for item in resources if item['id'] == target_id)
        if route == prefix + '/command' and method == 'POST':
            assert body == ({'name': 'SeriesSearch', 'seriesId': 1} if service == 'sonarr' else {'name': 'MoviesSearch', 'movieIds': [1]})
            return 201, {'id': 31, 'status': 'queued'}
        return 404, {}

    def moviepilot(self, method: str, route: str, query: dict[str, list[str]], body: dict[str, Any]) -> tuple[int, Any]:
        """模拟MP3业务信封、订阅状态与搜索批次回执。"""
        prefix = '/api/v1'
        if route == prefix + '/system/global' and method == 'GET':
            assert query == {'token': ['moviepilot']}
            return 200, {'BACKEND_VERSION': self.mp_version}
        if route == prefix + '/system/ping' and method == 'GET':
            return 200, None
        if route in (prefix + '/subscribe/', prefix + '/download/', prefix + '/history/download') and method == 'GET':
            assert query.get('count', ['100'])[0].isdigit() and 1 <= int(query.get('count', ['100'])[0]) <= 1000
            items = self.subscriptions if '/subscribe/' in route else self.mp_downloads if route.endswith('/download/') else [{'id': 1, 'name': 'MP受控历史', 'date': '2026-11-20'}]
            page = int(query.get('page', ['1'])[0])
            return 200, items[(page - 1) * 100:page * 100]
        if route == prefix + '/subscribe/' and method == 'POST':
            assert set(body) <= {'name', 'type', 'media_source', 'media_id', 'season'}
            assert body['media_source'] == 'themoviedb' and body['type'] in ('电影', '电视剧')
            target = dict(body, id=len(self.subscriptions) + 1, state='R', year='2025')
            self.subscriptions.append(target)
            return 200, {'id': target['id']}
        if route.startswith(prefix + '/subscribe/status/') and method == 'PUT':
            assert not body and query.get('state') in (['R'], ['S'])
            target = next(item for item in self.subscriptions if item['id'] == int(route.rsplit('/', 1)[1]))
            target['state'] = query['state'][0]
            return 200, None
        if route.startswith(prefix + '/subscribe/search/') and method == 'POST':
            assert not body
            return 200, {'batch_id': 'fixture-batch-31'}
        if route.startswith(prefix + '/subscribe/') and method == 'GET':
            return 200, next(item for item in self.subscriptions if item['id'] == int(route.rsplit('/', 1)[1]))
        if route == prefix + '/media/search' and method == 'GET':
            assert query.get('media_source') == ['themoviedb'] and query.get('type', ['media']) == ['media']
            return 200, [self.mp_media]
        if route == prefix + '/media/42' and method == 'GET':
            assert query.get('media_source') == ['themoviedb'] and query.get('type_name') == ['电影']
            return 200, self.mp_media
        if route == prefix + '/search/media/42' and method == 'GET':
            assert query.get('media_source') == ['themoviedb'] and query.get('mtype') == ['电影']
            context = {'media_info': self.mp_media, 'torrent_info': self.mp_torrent, 'meta_info': {'name': '隔离电影'}, 'resource_source': 'search', 'candidate_recognized': True}
            return 200, [context] * (1001 if self.mp_large_releases else 1)
        if route == prefix + '/download/' and method == 'POST':
            assert set(body) <= {'media_in', 'torrent_in', 'downloader', 'save_path'}
            assert body['media_in']['media_id'] == '42' and body['media_in']['media_source'] == 'themoviedb'
            assert body['torrent_in']['enclosure'] == self.mp_torrent['enclosure'] and body['torrent_in']['site_cookie'] == self.mp_secret
            assert not body.get('save_path') and not body.get('downloader')
            target_hash = 'c' * 40
            self.mp_downloads.append(dict(self.mp_downloads[0], hash=target_hash))
            return 200, {'download_id': target_hash, 'requires_confirmation': False}
        if route.startswith((prefix + '/download/start/', prefix + '/download/stop/')) and method == 'GET':
            assert query.get('name') == [self.mp_downloader]
            target = next(item for item in self.mp_downloads if item['hash'] == route.rsplit('/', 1)[1])
            target['state'] = 'paused' if '/stop/' in route else 'downloading'
            return 200, None
        if route.startswith(prefix + '/download/') and method == 'PATCH':
            assert set(body) == {'action', 'downloader'} and body['action'] in ('start', 'stop') and body['downloader'] == self.mp_downloader
            target = next(item for item in self.mp_downloads if item['hash'] == route.rsplit('/', 1)[1])
            target['state'] = 'paused' if body['action'] == 'stop' else 'downloading'
            return 200, {'hash': target['hash'], 'downloader': self.mp_downloader, 'results': [{'operation': body['action'], 'success': True, 'message': '已更新'}]}
        if route == prefix + '/storage/directories' and method == 'GET':
            assert query.get('storage_type', ['local']) == ['local']
            directory = {'name': '隔离本地目录', 'storage': 'local', 'download_path': self.mp_download_root, 'library_storage': 'local', 'library_path': self.mp_library_root, 'transfer_type': 'copy', 'overwrite_mode': self.mp_overwrite, 'renaming': True, 'scraping': False, 'notify': False}
            return 200, [directory]
        if route == prefix + '/storage/list' and method == 'POST':
            assert body.get('storage', 'local') == 'local' and body['path'].rstrip('/') in (self.mp_download_root, self.mp_library_root)
            root = body['path'].rstrip('/')
            path = '/private/outside-secret.mkv' if self.mp_outside_file else root + '/电影.mkv'
            return 200, [{'path': path, 'storage': 'local', 'type': 'file', 'name': '电影.mkv', 'extension': 'mkv', 'size': self.mp_file_size, 'modify_time': 1763596800.0, 'children': []}]
        if route == prefix + '/transfer/manual' and method == 'POST':
            selected = body.get('fileitems', [body.get('fileitem')])
            assert isinstance(selected, list) and len(selected) == 1 and selected[0]['path'] == self.mp_download_root + '/电影.mkv' and selected[0]['storage'] == 'local'
            assert selected[0].get('size') == 1048576 and selected[0].get('modify_time') == 1763596800.0
            assert body['target_storage'] == 'local' and body['target_path'] == self.mp_library_root
            assert body['transfer_type'] == 'copy' and body.get('reorganize', False) is False and body.get('from_history', False) is False
            assert body.get('scrape', False) is False and 'overwrite_mode' not in body
            assert body['media_source'] == 'themoviedb' and body['media_id'] == '42' and body['type_name'] == '电影'
            assert self.mp_overwrite == 'never'
            item = {'source': self.mp_download_root + '/电影.mkv', 'source_storage': 'local', 'target': self.mp_library_root + '/隔离电影/电影.mkv', 'target_dir': self.mp_library_root + '/隔离电影', 'success': True, 'overwrite_skipped': False}
            if body.get('preview') is True:
                return 200, {'summary': {'total': 1, 'success': 1, 'failed': 0}, 'items': [item], 'message': '预览完成'}
            assert query.get('background') == ['true']
            return 200, {'items': [dict(item, state='accepted')], 'message': '已接收'}
        if route == prefix + '/media/scrape/local' and method == 'POST':
            assert body['path'] == self.mp_library_root + '/电影.mkv' and body['storage'] == 'local'
            assert query.get('media_source') == ['themoviedb'] and query.get('media_id') == ['42'] and query.get('type_name') == ['电影']
            return 200, None
        if route == prefix + '/dashboard/schedule' and method == 'GET':
            return 200, [self.mp_scheduler]
        if route == prefix + '/dashboard/schedule/mediaserver_sync/progress' and method == 'GET':
            return 200, self.mp_scheduler['progress_detail']
        if route == prefix + '/system/runscheduler' and method == 'GET':
            assert query == {'jobid': ['mediaserver_sync']}
            self.mp_scheduler['progress_detail'] = {'id': 'mediaserver_sync', 'enable': True, 'value': 10.0, 'status': 'running', 'success': None, 'started_at': '2026-11-20T10:00:00Z', 'finished_at': None}
            return 200, None
        return 404, None

    def autobangumi(self, method: str, route: str, query: dict[str, list[str]], body: dict[str, Any]) -> tuple[int, Any]:
        """模拟AB4后台停机、独立RSS与番剧规则状态。"""
        prefix = '/api/v1'
        if route == '/health' and method == 'GET':
            return 200, {'status': 'ok', 'version': '4.0.0', 'db_ok': True}
        if route == prefix + '/status' and method == 'GET':
            return 200, {'status': False, 'version': '4.0.0'}
        if route == prefix + '/bangumi/get/all' and method == 'GET':
            return 200, self.bangumi
        if route.startswith(prefix + '/bangumi/get/') and method == 'GET':
            return 200, next(item for item in self.bangumi if item['id'] == int(route.rsplit('/', 1)[1]))
        if route == prefix + '/rss' and method == 'GET':
            return 200, self.feeds
        if route == prefix + '/downloader/torrents' and method == 'GET':
            return 200, [{'hash': 'a' * 40, 'name': 'AB分类下载', 'state': 'downloading', 'downloader_id': 'primary'}]
        if route == prefix + '/rss/add' and method == 'POST':
            assert set(body) == {'name', 'url', 'aggregate', 'parser'} and body['aggregate'] is False and body['parser'] == 'mikan'
            self.feeds.append(dict(body, id=len(self.feeds) + 1, enabled=True))
            return 200, {'msg_zh': '添加成功', 'msg_en': 'Added'}
        if route.startswith(prefix + '/rss/update/') and method == 'PATCH':
            assert set(body) == {'enabled'}
            target = next(item for item in self.feeds if item['id'] == int(route.rsplit('/', 1)[1]))
            target['enabled'] = body['enabled']
            return 200, {'msg_zh': '更新成功', 'msg_en': 'Updated'}
        if route.startswith(prefix + '/bangumi/enable/') or route.startswith(prefix + '/bangumi/disable/'):
            assert method == 'POST' and not body
            target = next(item for item in self.bangumi if item['id'] == int(route.rsplit('/', 1)[1]))
            target['deleted'] = '/disable/' in route
            if target['deleted']:
                assert query == {'file': ['false']}
            return 200, {'msg_zh': '已更新', 'msg_en': 'Updated'}
        return 404, {}


class ExternalHarness(Harness):
    """复用下载验证的真实进程生命周期和同源会话，不接触现有服务。"""

    def __init__(self, binary: Path, root: Path) -> None:
        """建立四协议fixture和公开响应收集器。"""
        super().__init__(binary, root)
        self.external: ExternalFixture = ExternalFixture()  # 外部服务的本地协议边界。
        self.sources: dict[str, dict[str, Any]] = {}  # 已保存来源的公开快照。
        self.public: list[bytes] = []  # 仅用于检查公开DTO泄漏。
        self.credentials: dict[str, str] = {'username': 'external-verify', 'password': generate_password()}  # 本次隔离管理员，不打印密码。

    def close(self) -> None:
        """无论断言是否通过都释放应用与本地HTTP监听器。"""
        super().close()
        self.external.close()

    def api(self, path: str, method: str = 'GET', body: dict[str, Any] | None = None, expected: int = 200) -> dict[str, Any]:
        """检查正式API状态并保留无请求凭据的公开回包。"""
        code, _, data = self.client.request(path, method, body)
        assert code == expected or method == 'POST' and expected == 200 and code in (201, 202), f'API状态不符合预期：{method} {path} -> {code}'
        self.public.append(data)
        return json.loads(data)

    def source_body(self, service: str, **values: Any) -> dict[str, Any]:
        """生成完整正式来源输入，连接凭据不进入公开来源字段。"""
        body: dict[str, Any] = {'kind': 'external', 'name': service + ' 本地验证', 'enabled': True, 'rootId': '', 'path': '', 'allowPrivate': True, 'refreshMinutes': 30, 'outputProfile': 'video', 'externalUrl': '', 'externalService': service, 'connection': {'baseUrl': self.external.base + '/' + service, 'apiKey': self.external.keys[service]}}
        body.update(values)
        return body

    def patch_source(self, source: dict[str, Any], **values: Any) -> dict[str, Any]:
        """普通编辑默认不提交服务类型与密钥，以验证服务端保留原连接。"""
        fields = ('kind', 'name', 'enabled', 'rootId', 'path', 'allowPrivate', 'refreshMinutes', 'outputProfile', 'externalUrl', 'revision')
        body = {key: source[key] for key in fields}
        body.update(values)
        return self.api('/api/collection/sources/' + source['id'], 'PATCH', body)['source']

    def endpoint(self, source: dict[str, Any], suffix: str) -> str:
        """组合固定来源控制地址，不接受任意外部URL。"""
        return '/api/collection/sources/' + source['id'] + '/external/' + suffix

    def action(self, source: dict[str, Any], kind: str, **values: Any) -> tuple[dict[str, Any], dict[str, Any]]:
        """使用新UUID提交一个固定意图，并保留同意图重取所需输入。"""
        body: dict[str, Any] = {'requestId': str(uuid.uuid4()), 'revision': source['revision'], 'kind': kind}
        body.update(values)
        return body, self.api(self.endpoint(source, 'actions'), 'POST', body)['action']

    def connect(self, source: dict[str, Any]) -> dict[str, Any]:
        """在不执行业务写的条件下检测保存后的连接。"""
        return self.api(self.endpoint(source, 'test'), 'POST', {'revision': source['revision']})['info']

    def prepare(self) -> None:
        """创建独立管理员、迁移旧数据并配置四个隔离外部来源。"""
        self.start()
        anonymous = Client(self.base)
        self.client.check(anonymous.request('/api/collection/sources')[0] == 401, '无会话不能读取来源')
        self.api('/api/setup', 'POST', self.credentials)
        self.client.check(self.client.request('/api/collection/sources', 'POST', {}, {'Origin': 'https://foreign.invalid'})[0] == 403, '跨源修改被同源边界拒绝')
        capabilities = self.api('/api/collection/capabilities')
        local_root = next(item['id'] for item in capabilities['roots'] if Path(item['path']).resolve() == (self.root / 'media').resolve())
        original = self.api('/api/collection/sources', 'POST', {'kind': 'local', 'name': '迁移前既有来源', 'enabled': True, 'rootId': local_root, 'path': '', 'allowPrivate': False, 'refreshMinutes': 30, 'outputProfile': 'video', 'externalUrl': ''})['source']
        self.stop()
        database_path = self.root / 'data/library.db'
        with closing(sqlite3.connect(database_path)) as database:
            checkpoint = database.execute('PRAGMA wal_checkpoint(TRUNCATE)').fetchone()
            assert checkpoint[0] == 0, '隔离旧库准备前必须完整回收应用写入'
        legacy_path = self.root / 'schema3-fixture.db'
        preserved_counts = create_legacy_database(legacy_path, 3, database_path)
        self.client.check(preserved_counts['users'] == 1 and preserved_counts['sessions'] >= 1 and preserved_counts['collection_sources'] == 1, '真实schema3夹具逐字段保留用户、旧会话和原来源，未混入新账号字段')
        database_path.replace(self.root / 'before-schema3-fixture.db')
        legacy_path.replace(database_path)
        self.start()
        self.api('/api/login', 'POST', self.credentials)
        preserved = self.api('/api/collection/sources')['sources']
        with sqlite3.connect(self.root / 'data/library.db') as database:
            version = database.execute('PRAGMA user_version').fetchone()[0]
            users = database.execute('SELECT count(*) FROM users').fetchone()[0]
        self.client.check(version == 6 and users == 1 and any(item['id'] == original['id'] for item in preserved), '真实schema3经外部服务及账号/备份增量迁移到6，保留用户和原来源')
        rejected = self.source_body('sonarr', allowPrivate=False)
        self.api('/api/collection/sources', 'POST', rejected, expected=400)
        self.client.check(not self.external.events, 'HTTP私网未授权在保存时拒绝且没有发送凭据')
        for service in self.external.keys:
            source = self.api('/api/collection/sources', 'POST', self.source_body(service))['source']
            self.sources[service] = source
            count = len(self.external.mutations())
            info = self.connect(source)
            self.client.check(source['apiConfigured'] and info['supported'] and info['service'] == service and len(self.external.mutations()) == count, service + '：三步配置后可连接且检测没有业务副作用')
            path = self.endpoint(source, 'items?section=resources&page=1')
            self.client.check(anonymous.request(path)[0] == 401, service + '：控制端点需要会话')

    def verify_protocols(self) -> None:
        """验证四类真实协议、映射、精确写入及持久请求防重。"""
        for service, source in self.sources.items():
            resources = self.api(self.endpoint(source, 'items?section=resources&page=1'))
            queue = self.api(self.endpoint(source, 'items?section=queue&page=1'))
            self.client.check(resources['total'] >= 1 and queue['total'] == 1, service + '：读取现有资源与有界队列')
            if service != 'autobangumi':
                self.client.check(self.api(self.endpoint(source, 'items?section=history&page=1'))['total'] == 1, service + '：读取真实历史分区')
            else:
                self.api(self.endpoint(source, 'items?section=history&page=1'), expected=502)
                self.client.check('部分' in queue['message'] and {item['id'] for item in resources['items']} == {'bangumi:1', 'rss:1'}, 'AB：未支持历史明确报错，RSS和番剧编号独立且队列声明部分视图')
            if service in ('sonarr', 'radarr'):
                options = self.api(self.endpoint(source, 'options'))
                lookup = self.api(self.endpoint(source, 'lookup?q=fixture'))
                self.client.check(options['roots'][0]['id'] == '1' and options['profiles'][0]['id'] == '2' and lookup['items'][0]['externalId'] == '1001', service + '：只读现有目录画质及元数据选择')
                _, added = self.action(source, 'add', externalId='1001', mediaType='series' if service == 'sonarr' else 'movie', rootId='1', profileId='2', enabled=True)
            elif service == 'moviepilot':
                self.client.check(resources['items'][0]['enabled'] is True, 'MoviePilot：P状态仍为运行，不误当暂停')
                _, added = self.action(source, 'add', externalId='1001', title='MP新增订阅', mediaType='series', season=1, enabled=True)
            else:
                _, added = self.action(source, 'add', title='AB新增RSS', mediaType='rss', rssUrl='https://rss.invalid/new?passkey=' + self.external.rss_secret, enabled=True)
            self.client.check(added['state'] == 'succeeded' and bool(added['targetId']), service + '：单次新增得到真实回执或唯一身份确认')
            target = 'rss:1' if service == 'autobangumi' else '1'
            body, monitored = self.action(source, 'monitor', targetId=target, enabled=False)
            self.client.check(monitored['state'] == 'succeeded', service + '：单目标状态变更并读回确认')
            count = len(self.external.mutations(service))
            repeated = self.api(self.endpoint(source, 'actions'), 'POST', body)['action']
            self.client.check(repeated['id'] == monitored['id'] and len(self.external.mutations(service)) == count, service + '：相同UUID重取持久结果，不重发外部写')
            self.api(self.endpoint(source, 'actions'), 'POST', dict(body, enabled=True), expected=409)
            self.api(self.endpoint(source, 'actions'), 'POST', dict(body, requestId=str(uuid.uuid4()), revision=source['revision'] + 1), expected=409)
            self.client.check(len(self.external.mutations(service)) == count, service + '：不同意图复用UUID及过期revision均拒绝')
            if service != 'autobangumi':
                _, searched = self.action(source, 'search', targetId='1')
                self.client.check(searched['state'] == 'succeeded', service + '：显式搜索仅作用单个媒体或订阅')
            else:
                _, searched = self.action(source, 'search', targetId='bangumi:1')
                self.client.check(searched['state'] == 'failed', 'AB：未核实的通用搜索不能伪造成功')
                _, changed = self.action(source, 'monitor', targetId='bangumi:1', enabled=False)
                self.client.check(changed['state'] == 'succeeded' and self.external.bangumi[0]['archived'] is True, 'AB：规则停用明确file=false且保留归档状态')
        self.client.check(not self.external.errors, '四类请求均符合真实认证头、反代前缀和固定请求body')

    def mp_choices(self, operation: str, body: dict[str, Any] | None = None) -> list[dict[str, Any]]:
        """通过真实应用固定端点读取短期选择，路径从不由浏览器指定。"""
        source = self.sources['moviepilot']
        request = dict(body, revision=source['revision']) if body is not None else None
        return self.api(self.endpoint(source, 'moviepilot/' + operation), 'POST' if body is not None else 'GET', request)['choices']

    def mp_file(self, kind: str) -> tuple[dict[str, Any], dict[str, Any], list[dict[str, Any]]]:
        """从已配置根中取得单一受控媒体文件，并返回本轮目录快照。"""
        roots = self.mp_choices('roots')
        root = next(item for item in roots if item['kind'] == kind)
        files = self.mp_choices('files', {'rootId': root['id']})
        return root, files[0], roots

    def verify_moviepilot_workflow(self) -> None:
        """验证六阶段真实应用链路和私密快照、覆盖、幂等及未知状态边界。"""
        source = self.sources['moviepilot']
        endpoint = self.endpoint(source, 'actions')
        capabilities = self.connect(source)['capabilities']
        required = {'integrated_search', 'downloads', 'organize', 'scrape', 'refresh_library', 'download'}
        self.client.check(required <= set(capabilities), 'MP：精确已核实版本开放六阶段能力')
        self.api(endpoint, 'POST', {'requestId': str(uuid.uuid4()), 'revision': source['revision'], 'kind': 'download', 'selectionId': str(uuid.uuid4()), 'payload': {}}, expected=400)
        self.client.check(True, 'MP：拒绝浏览器注入原始上游payload')
        count = len(self.external.mutations('moviepilot'))
        media = self.mp_choices('search', {'query': '隔离电影', 'mediaType': 'movie'})
        self.client.check(len(media) == 1 and media[0]['externalId'] == '42' and media[0]['kind'] == 'media', 'MP：集成资料搜索返回准确TMDB身份和不透明选择')
        releases = self.mp_choices('releases', {'selectionId': media[0]['id']})
        self.client.check(len(releases) == 1 and len(self.external.mutations('moviepilot')) == count, 'MP：查找资源只读取候选，确认前没有创建下载')
        body, downloaded = self.action(source, 'download', selectionId=releases[0]['id'])
        count = len(self.external.mutations('moviepilot'))
        repeated = self.api(endpoint, 'POST', body)['action']
        self.client.check(downloaded['state'] == 'succeeded' and downloaded['id'] == repeated['id'] and len(self.external.mutations('moviepilot')) == count, 'MP：所选私密资源一次下发，原UUID重取不重复下载')
        self.api(endpoint, 'POST', dict(body, requestId=str(uuid.uuid4())), expected=409)
        self.client.check(True, 'MP：同一下载选择不能被另一新请求再次消费')
        self.api(self.endpoint(source, 'moviepilot/releases'), 'POST', {'revision': source['revision'], 'selectionId': releases[0]['id']}, expected=409)
        queue = self.mp_choices('downloads')
        _, paused = self.action(source, 'download_stop', selectionId=queue[0]['id'])
        self.client.check(paused['state'] == 'succeeded' and self.external.mp_downloads[0]['state'] == 'paused', 'MP：暂停仅作用实际下载器与散列对应的一项任务')
        queue = self.mp_choices('downloads')
        self.external.drop_next = ('moviepilot', 'PATCH', '/api/v1/download/' + 'b' * 40)
        _, unknown = self.action(source, 'download_start', selectionId=queue[0]['id'])
        count = len(self.external.mutations('moviepilot'))
        reconciled = self.api(self.endpoint(source, 'actions/' + unknown['id'] + '/reconcile'), 'POST', {})['action']
        self.client.check(unknown['state'] == 'uncertain' and reconciled['state'] == 'succeeded' and len(self.external.mutations('moviepilot')) == count, 'MP：启停响应丢失后只读核对，不重发控制')
        _, file, roots = self.mp_file('download_root')
        target = next(item for item in roots if item['kind'] == 'library_root')
        self.client.check(target['organizeAllowed'] is True, 'MP：整理目标资格来自真实不覆盖目录配置')
        count = len(self.external.mutations('moviepilot'))
        plan = self.mp_choices('organize/preview', {'fileId': file['id'], 'targetId': target['id'], 'externalId': '42', 'mediaType': 'movie'})[0]
        self.client.check(plan['kind'] == 'organize_plan' and len(self.external.mutations('moviepilot')) == count, 'MP：整理预览验证单文件资料和目录，不产生文件写入')
        self.external.mp_overwrite = 'always'
        _, rejected = self.action(source, 'organize', selectionId=plan['id'])
        self.client.check(rejected['state'] == 'failed' and len(self.external.mutations('moviepilot')) == count, 'MP：预览后策略改为覆盖时执行前拒绝，不调用整理写接口')
        roots = self.mp_choices('roots')
        self.client.check(next(item for item in roots if item['kind'] == 'library_root')['organizeAllowed'] is False, 'MP：非不覆盖库根仍可供刮削，但不能作为整理目标')
        self.external.mp_overwrite = 'never'
        _, file, roots = self.mp_file('download_root')
        target = next(item for item in roots if item['kind'] == 'library_root')
        plan = self.mp_choices('organize/preview', {'fileId': file['id'], 'targetId': target['id'], 'externalId': '42', 'mediaType': 'movie'})[0]
        self.external.mp_file_size += 1
        count = len(self.external.mutations('moviepilot'))
        _, rejected = self.action(source, 'organize', selectionId=plan['id'])
        self.client.check(rejected['state'] == 'failed' and len(self.external.mutations('moviepilot')) == count, 'MP：文件版本变化时撤销旧预览，源文件无写入')
        self.external.mp_file_size -= 1
        _, file, roots = self.mp_file('download_root')
        target = next(item for item in roots if item['kind'] == 'library_root')
        plan = self.mp_choices('organize/preview', {'fileId': file['id'], 'targetId': target['id'], 'externalId': '42', 'mediaType': 'movie'})[0]
        body, organized = self.action(source, 'organize', selectionId=plan['id'])
        count = len(self.external.mutations('moviepilot'))
        self.api(endpoint, 'POST', body)
        self.client.check(organized['state'] == 'succeeded' and '受理' in organized['message'] and '尚未入库' in organized['message'] and len(self.external.mutations('moviepilot')) == count, 'MP：固定copy整理仅显示受理，UUID重取不重发且不伪造入库')
        _, file, _ = self.mp_file('library_root')
        count = len(self.external.mutations('moviepilot'))
        scrape = self.mp_choices('scrape/preview', {'fileId': file['id'], 'externalId': '42', 'mediaType': 'movie'})[0]
        self.client.check(len(self.external.mutations('moviepilot')) == count and scrape['kind'] == 'scrape_plan', 'MP：刮削预览只查文件和元数据，不覆盖资料')
        self.api(endpoint, 'POST', {'requestId': str(uuid.uuid4()), 'revision': source['revision'], 'kind': 'scrape', 'selectionId': scrape['id']}, expected=400)
        self.client.check(len(self.external.mutations('moviepilot')) == count, 'MP：刮削缺少独立覆盖确认时拒绝')
        _, scraped = self.action(source, 'scrape', selectionId=scrape['id'], confirmOverwrite=True)
        self.client.check(scraped['state'] == 'succeeded' and len(self.external.mutations('moviepilot')) == count + 1, 'MP：明确确认后按官方query和单FileItem同步刮削')
        _, file, _ = self.mp_file('library_root')
        scrape = self.mp_choices('scrape/preview', {'fileId': file['id'], 'externalId': '42', 'mediaType': 'movie'})[0]
        self.external.mp_partial_next = '/api/v1/media/scrape/local'
        _, partial = self.action(source, 'scrape', selectionId=scrape['id'], confirmOverwrite=True)
        self.client.check(partial['state'] == 'uncertain', 'MP：已发出刮削的业务失败保留不确定，不能视作未执行')
        self.api(self.endpoint(source, 'actions/' + partial['id'] + '/acknowledge'), 'POST', {'checkedExternally': True})
        library = self.mp_choices('library')
        body, refreshed = self.action(source, 'refresh_library', selectionId=library[0]['id'])
        count = len(self.external.mutations('moviepilot'))
        self.api(endpoint, 'POST', body)
        status = self.mp_choices('library')
        self.client.check(refreshed['state'] == 'succeeded' and '接受' in refreshed['message'] and status[0]['status'] == '正在同步' and len(self.external.mutations('moviepilot')) == count, 'MP：固定媒体索引任务只启动一次，受理与实际进度分别显示')
        self.external.mp_scheduler['progress_detail']['enable'] = False
        self.external.mp_scheduler['progress_detail']['status'] = 'success'
        library = self.mp_choices('library')
        self.external.drop_next = ('moviepilot', 'GET', '/api/v1/system/runscheduler')
        body, unknown = self.action(source, 'refresh_library', selectionId=library[0]['id'])
        count = len(self.external.mutations('moviepilot'))
        self.api(endpoint, 'POST', body)
        checked = self.api(self.endpoint(source, 'actions/' + unknown['id'] + '/reconcile'), 'POST', {})['action']
        self.client.check(unknown['state'] == checked['state'] == 'uncertain' and len(self.external.mutations('moviepilot')) == count, 'MP：副作用GET丢响应不自动重放，不用全局进度伪造本次完成')
        acknowledged = self.api(self.endpoint(source, 'actions/' + unknown['id'] + '/acknowledge'), 'POST', {'checkedExternally': True})['action']
        self.client.check(acknowledged['state'] == 'unconfirmed', 'MP：人工核查解除阻塞仍保留未证实事实')
        self.external.mp_outside_file = True
        roots = self.mp_choices('roots')
        root = next(item for item in roots if item['kind'] == 'download_root')
        self.api(self.endpoint(source, 'moviepilot/files'), 'POST', {'revision': source['revision'], 'rootId': root['id']}, expected=502)
        self.external.mp_outside_file = False
        self.client.check(True, 'MP：上游返回越界文件时整批拒绝，不产生可执行选择')
        media = self.mp_choices('search', {'query': '隔离电影', 'mediaType': 'movie'})
        self.external.mp_large_releases = True
        self.api(self.endpoint(source, 'moviepilot/releases'), 'POST', {'revision': source['revision'], 'selectionId': media[0]['id']}, expected=502)
        self.external.mp_large_releases = False
        self.client.check(True, 'MP：资源超出预算明确失败，不静默截断或保存无界上下文')
        releases = self.mp_choices('releases', {'selectionId': media[0]['id']})
        with sqlite3.connect(self.root / 'data/library.db') as database:
            database.execute("UPDATE collection_external_choices SET expires_at='2000-01-01T00:00:00Z' WHERE id=?", (releases[0]['id'],))
        self.api(endpoint, 'POST', {'requestId': str(uuid.uuid4()), 'revision': source['revision'], 'kind': 'download', 'selectionId': releases[0]['id']}, expected=409)
        self.client.check(True, 'MP：过期私密快照不能创建下载')
        self.external.mp_version = 'v3.1.0'
        info = self.connect(source)
        self.client.check('integrated_search' not in info['capabilities'] and 'resources' in info['capabilities'], 'MP：其它未核实工作流版本只保留已核实基础能力')
        self.external.mp_version = 'v3.2.8'
        self.client.check(not self.external.errors, 'MP：全部真实HTTP载荷符合固定官方接口，不含伪造overwrite参数')

    def verify_recovery(self) -> None:
        """验证接受后断线、只读对账、连接修订和重启恢复。"""
        source = self.sources['sonarr']
        self.external.drop_next = ('sonarr', 'POST', '/api/v3/series')
        body, uncertain = self.action(source, 'add', externalId='2002', mediaType='series', rootId='1', profileId='2', enabled=True)
        count = len(self.external.mutations('sonarr'))
        self.client.check(uncertain['state'] == 'uncertain', '上游已接受但响应断开时持久化不确定态')
        self.api(self.endpoint(source, 'actions'), 'POST', body)
        self.api(self.endpoint(source, 'actions'), 'POST', dict(body, requestId=str(uuid.uuid4()), externalId='3003'), expected=409)
        reconciled = self.api(self.endpoint(source, 'actions/' + uncertain['id'] + '/reconcile'), 'POST', {})['action']
        self.client.check(reconciled['state'] == 'succeeded' and len(self.external.mutations('sonarr')) == count, '相同UUID不重发且未知操作阻塞新写，只读精确对账后解除')
        self.external.drop_next = ('sonarr', 'POST', '/api/v3/command')
        search_body, search = self.action(source, 'search', targetId='1')
        count = len(self.external.mutations('sonarr'))
        checked = self.api(self.endpoint(source, 'actions/' + search['id'] + '/reconcile'), 'POST', {})['action']
        self.client.check(search['state'] == checked['state'] == 'uncertain' and len(self.external.mutations('sonarr')) == count, '未知搜索无批次回执时保持不确定，不因对账重发搜索')
        with sqlite3.connect(self.root / 'data/library.db') as database:
            ref = database.execute('SELECT private_ref FROM collection_external_connections WHERE source_id=?', (source['id'],)).fetchone()[0]
        private_path = self.root / 'data/collection-secrets' / (ref + '.json')
        immutable = private_path.read_bytes()
        source = self.patch_source(source, name='普通编辑后仍保留连接')
        self.sources['sonarr'] = source
        with sqlite3.connect(self.root / 'data/library.db') as database:
            new_ref = database.execute('SELECT private_ref FROM collection_external_connections WHERE source_id=?', (source['id'],)).fetchone()[0]
        self.client.check(source['apiConfigured'] and source['externalService'] == 'sonarr' and new_ref != ref and private_path.read_bytes() == immutable and self.connect(source)['supported'], '普通PATCH保留服务与密钥，同时创建新私密快照且旧快照不变')
        rotated = 'rotated-key-' + secrets.token_hex(20)
        self.external.accepted_keys['sonarr'].add(rotated)
        source = self.patch_source(source, connection={'apiKey': rotated})
        self.sources['sonarr'] = source
        self.api(self.endpoint(source, 'actions/' + search['id'] + '/reconcile'), 'POST', {}, expected=409)
        repeated = self.api(self.endpoint(source, 'actions'), 'POST', search_body)['action']
        self.client.check(repeated['id'] == search['id'] and len(self.external.mutations('sonarr')) == count, '连接改变后拒绝历史对账，但原UUID仍可取回且不会转发新地址')
        source = self.patch_source(source, connection={'apiKey': self.external.keys['sonarr']})
        self.sources['sonarr'] = source
        self.stop()
        with sqlite3.connect(self.root / 'data/library.db') as database:
            database.execute("UPDATE collection_external_actions SET state='dispatching' WHERE id=?", (search['id'],))
        self.start()
        self.api('/api/login', 'POST', self.credentials)
        actions = self.api(self.endpoint(source, 'actions'))['actions']
        recovered = next(item for item in actions if item['id'] == search['id'])
        self.client.check(recovered['state'] == 'uncertain' and len(self.external.mutations('sonarr')) == count, '重启把未完成dispatching恢复为uncertain，未重新发送业务写')
        self.api(self.endpoint(source, 'actions/' + search['id'] + '/acknowledge'), 'POST', {'checkedExternally': False}, expected=400)
        acknowledged = self.api(self.endpoint(source, 'actions/' + search['id'] + '/acknowledge'), 'POST', {'checkedExternally': True})['action']
        self.client.check(acknowledged['state'] == 'unconfirmed' and len(self.external.mutations('sonarr')) == count, '人工核查只解除阻塞，不虚构成功或重发')

    def verify_boundaries(self) -> None:
        """验证离线、重定向和普通备份/数据库/DTO的秘密分离。"""
        source = self.sources['radarr']
        self.external.offline.add('radarr')
        self.api(self.endpoint(source, 'items?section=queue&page=1'), expected=502)
        self.external.offline.remove('radarr')
        self.external.redirect.add('radarr')
        self.api(self.endpoint(source, 'test'), 'POST', {'revision': source['revision']}, expected=502)
        self.external.redirect.remove('radarr')
        self.client.check(self.external.trap_hits == 0, '离线明确报错且跨源重定向被拒绝，没有向新目标发送认证')
        # 将反射凭据混入标题，检查白名单字段清理仍会去掉当前连接秘密。
        self.external.resources['radarr'][0]['title'] += ' ' + self.external.keys['radarr'] + ' ' + self.external.base + '/radarr'
        self.api(self.endpoint(source, 'items?section=resources&page=1'))
        self.api('/api/collection/sources')
        for item in self.sources.values():
            self.api(self.endpoint(item, 'actions'))
        status, _, backup = self.client.request('/api/backup')
        assert status == 200 and backup.startswith(b'SQLite format 3\x00')
        (self.root / 'verified-backup.db').write_bytes(backup)
        with sqlite3.connect(self.root / 'verified-backup.db') as database:
            self.client.check(database.execute('PRAGMA integrity_check').fetchone()[0] == 'ok', '普通备份保持SQLite一致性')
        self.stop()
        main_files = [path.read_bytes() for path in (self.root / 'data').glob('library.db*') if path.is_file()]
        public = b'\n'.join(self.public)
        log = (self.root / 'application.log').read_bytes()
        secrets_to_check = [value.encode() for values in self.external.accepted_keys.values() for value in values]
        secrets_to_check += [self.external.base.encode(), self.external.rss_secret.encode(), self.external.remote_root.encode()]
        secrets_to_check += [value.encode() for value in (self.external.mp_secret, self.external.mp_downloader, self.external.mp_download_root, self.external.mp_library_root)]
        self.client.check(all(marker not in public + backup + log + b''.join(main_files) for marker in secrets_to_check), '密钥、私密实例地址、RSS passkey和外部目录不进入DTO/主库/普通备份/日志')
        private_files = list((self.root / 'data/collection-secrets').glob('*.json'))
        self.client.check(bool(private_files) and any(self.external.keys['autobangumi'].encode() in path.read_bytes() for path in private_files), '秘密只存入独立collection-secrets快照')
        self.client.check(not self.external.errors, '完整验证未出现协议载荷或认证错误')

    def run(self, workflow: bool = False) -> None:
        """执行基础四协议、持久恢复和秘密边界的完整回归。"""
        self.prepare()
        self.verify_protocols()
        if workflow:
            self.verify_moviepilot_workflow()
        self.verify_recovery()
        self.verify_boundaries()

    def serve_external(self) -> None:
        """保留四服务和真实应用供桌面/手机浏览器验收。"""
        self.prepare()
        credential_file = self.root / 'app-credentials.json'
        credential_file.write_text(json.dumps(self.credentials), encoding='utf-8')
        if os.name != 'nt':
            credential_file.chmod(0o600)
        info = {'base': self.base, 'root': str(self.root), 'credentialFile': str(credential_file), 'sources': {name: {'id': source['id'], 'revision': source['revision']} for name, source in self.sources.items()}, 'stopFile': str(self.root / 'stop')}
        (self.root / 'fixture-info.json').write_text(json.dumps(info, ensure_ascii=False, indent=2), encoding='utf-8')
        print(json.dumps({'fixtureInfo': str(self.root / 'fixture-info.json'), 'base': self.base, 'stopFile': info['stopFile']}, ensure_ascii=False), flush=True)
        while not (self.root / 'stop').exists():
            self.external.offline = {name for name in self.sources if (self.root / (name + '-offline')).exists()}
            time.sleep(.5)


def main() -> None:
    """解析独立验证配置并拒绝覆盖既有测试或用户目录。"""
    parser = argparse.ArgumentParser(description=__doc__)
    # 提前构建的应用可执行文件；不会安装第三方服务或触碰生产进程。
    parser.add_argument('--binary', type=Path, required=True, help='提前构建的栖库应用可执行文件')
    # 唯一生成根，路径必须含verify段且尚不存在，避免覆盖媒体。
    parser.add_argument('--root', type=Path, default=verification_directory() / ('external-' + time.strftime('%Y%m%d-%H%M%S')), help='不存在且包含verify路径段的隔离目录')
    # 保留本机HTTP fixture和应用，用stop文件结束，不把准备阶段算完整自动通过。
    parser.add_argument('--serve', action='store_true', help='保留本地浏览器验收环境，创建根目录stop文件后结束')
    parser.add_argument('--workflow', action='store_true', help='额外验证MoviePilot六阶段完整工作流及安全边界')
    args = parser.parse_args()
    root = args.root.resolve()
    if not args.binary.is_file():
        parser.error('--binary文件不存在，请先构建应用')
    if root.exists() or 'verify' not in root.parts:
        parser.error('--root必须不存在且路径包含verify段')
    root.mkdir(parents=True)
    harness = ExternalHarness(args.binary.resolve(), root)
    completed = False
    try:
        if args.serve:
            harness.serve_external()
        else:
            harness.run(args.workflow)
            completed = True
    finally:
        harness.close()
        if not args.serve:
            result = {'passed': completed, 'assertions': harness.client.passed, 'moviePilotWorkflow': args.workflow, 'realApplication': True, 'controlledExternalHTTP': ['Sonarr4', 'Radarr7', 'MoviePilot3.2.8', 'AutoBangumi4'], 'realExternalInstances': False, 'realMediaChanged': False, 'thirdPartyToolsInstalled': False}
            (root / 'result.json').write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding='utf-8')


if __name__ == '__main__':
    from project_env import run_cli

    raise SystemExit(run_cli(main))
