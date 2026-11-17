"""使用隔离媒体样本验证首版接口；所有生成数据只写入明确指定的验证目录。"""

from __future__ import annotations

import argparse
import http.cookiejar
import json
import math
import os
import secrets
import shutil
import struct
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import wave
import zipfile
import zlib
from pathlib import Path
from typing import Any


def make_png(path: Path, seed: int, width: int = 600, height: int = 800) -> None:
    """生成受控大小的渐变 PNG，供图片与漫画读取验证使用。"""
    def chunk(kind: bytes, data: bytes) -> bytes:
        """封装带校验和的 PNG 数据块。"""
        return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data))

    raw = bytearray()
    for y in range(height):
        raw.append(0)
        for x in range(width):
            raw.extend(((x // 3 + seed * 31) % 256, (y // 4 + seed * 51) % 256, (x // 8 + y // 8 + seed * 20) % 256))
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 2, 0, 0, 0)) + chunk(b'IDAT', zlib.compress(raw)) + chunk(b'IEND', b''))


def make_epub(path: Path) -> None:
    """生成最小合法 EPUB，验证目录、阅读与 CFI 进度保存。"""
    with zipfile.ZipFile(path, 'w') as book:
        book.writestr('mimetype', 'application/epub+zip', compress_type=zipfile.ZIP_STORED)
        book.writestr('META-INF/container.xml', '<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>')
        book.writestr('OEBPS/content.opf', '<?xml version="1.0" encoding="utf-8"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">urn:nas:verification</dc:identifier><dc:title>海边的图书馆</dc:title><dc:language>zh-CN</dc:language><meta property="dcterms:modified">2026-11-17T00:00:00Z</meta></metadata><manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="chapter"/></spine></package>')
        book.writestr('OEBPS/nav.xhtml', '<?xml version="1.0" encoding="utf-8"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>目录</title></head><body><nav epub:type="toc"><ol><li><a href="chapter.xhtml">第一章 潮汐</a></li></ol></nav></body></html>')
        paragraphs = ''.join(f'<p>第{i}段：海风翻开书页，远处的灯塔逐渐亮起。这是本机生成的阅读验证文本。</p>' for i in range(1, 101))
        book.writestr('OEBPS/chapter.xhtml', '<?xml version="1.0" encoding="utf-8"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>第一章 潮汐</title></head><body><h1>第一章 潮汐</h1>' + paragraphs + '</body></html>')


def make_sparse_video(path: Path) -> int:
    """创建超过 4 GiB 的稀疏协议样本，绝不将其作为可解码影片。"""
    size = 5 * 1024 ** 3 + 123
    path.touch()
    if os.name == 'nt':
        # Windows 先显式标记稀疏，避免验证样本实际占用数 GiB。
        subprocess.run(['fsutil', 'sparse', 'setflag', str(path)], check=True, capture_output=True)
    with path.open('r+b') as stream:
        stream.truncate(size)
        for offset in (0, 4 * 1024 ** 3 + 17, size - 64):
            stream.seek(offset)
            stream.write(bytes(range(64)))
    return size


def make_music(root: Path, ffmpeg: str | None) -> None:
    """生成真实音频和内嵌封面样本，验证全部声明格式及无封面回退。"""
    music = root / 'music'
    music.mkdir(parents=True, exist_ok=True)
    source = music / '林间回响.wav'
    # 使用十二秒低音量正弦信号，供播放、拖动和断点恢复验证。
    sample_rate: int = 22050
    with wave.open(str(source), 'wb') as audio:
        audio.setparams((1, 2, sample_rate, 0, 'NONE', 'not compressed'))
        audio.writeframes(b''.join(struct.pack('<h', int(1800 * math.sin(2 * math.pi * 440 * index / sample_rate))) for index in range(sample_rate * 12)))
    (music / '不应入库.txt').write_text('音乐库应忽略非音频文件。', encoding='utf-8')
    if not ffmpeg:
        return
    cover = root.parent / 'music-cover.png'
    make_png(cover, 4, 400, 400)
    subprocess.run([ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', '-i', str(source), '-i', str(cover), '-map', '0:a', '-map', '1:v', '-c:a', 'libmp3lame', '-c:v', 'mjpeg', '-id3v2_version', '3', '-disposition:v', 'attached_pic', '-metadata', 'title=夜间电台', str(music / '夜间电台.MP3')], check=True)
    for name, codec in (('午后慢调.m4a', 'aac'), ('晨间序曲.aac', 'aac'), ('无损片刻.flac', 'flac'), ('远山来信.ogg', 'libvorbis'), ('小径随想.opus', 'libopus'), ('海边散步.aiff', 'pcm_s16be')):
        subprocess.run([ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', '-i', str(source), '-c:a', codec, str(music / name)], check=True)
    shutil.copyfile(music / '海边散步.aiff', music / '黄昏序曲.aif')


def prepare(root: Path, ffmpeg: str | None, large: bool) -> None:
    """构造五类隔离媒体，复用已有样本而不修改任何真实源目录。"""
    for kind in ('video', 'image', 'comic', 'novel', 'music'):
        (root / kind).mkdir(parents=True, exist_ok=True)
    make_png(root / 'image' / '晨光.png', 1)
    make_png(root / 'image' / '山间来信.png', 3, 900, 600)
    for number in (1, 2, 10):
        make_png(root / 'comic' / '星际漫游' / f'第{number}页.png', number)
    with zipfile.ZipFile(root / 'comic' / '海岛日记.cbz', 'w', zipfile.ZIP_DEFLATED) as comic:
        for number in (10, 2, 1):
            comic.write(root / 'comic' / '星际漫游' / f'第{number}页.png', f'pages/{number}.png')
    text = '\n'.join(f'第{chapter}章 夜航\n' + '\n'.join(f'第{paragraph}段。城市渐渐安静，窗外有一束温暖的光。这是用于验证的文本。' for paragraph in range(1, 31)) for chapter in range(1, 5))
    (root / 'novel' / '夜航手记.txt').write_text(text, encoding='utf-8')
    (root / 'novel' / '旧日时光.txt').write_bytes(text.encode('gb18030'))
    make_epub(root / 'novel' / '海边的图书馆.epub')
    make_music(root, ffmpeg)
    if ffmpeg:
        subprocess.run([ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', '-f', 'lavfi', '-i', 'testsrc2=size=640x360:rate=24', '-t', '4', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', str(root / 'video' / '色彩实验.mp4')], check=True)
    if large:
        make_sparse_video(root / 'video' / '仅协议验证-5GiB.mp4')


class Client:
    """维护独立验证会话，统一处理 JSON 与二进制响应。"""

    def __init__(self, base: str) -> None:
        """创建自动保存 Cookie 的客户端。"""
        # API 根地址，仅限用户传入的被测应用。
        self.base: str = base.rstrip('/')
        # 会话 Cookie 容器，不写入磁盘。
        self.cookies: http.cookiejar.CookieJar = http.cookiejar.CookieJar()
        # 使用标准库 HTTP 客户端，不引入额外验证依赖。
        self.opener: urllib.request.OpenerDirector = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.cookies))
        # 已通过的断言数。
        self.passed: int = 0

    def request(self, path: str, method: str = 'GET', data: dict[str, Any] | None = None, headers: dict[str, str] | None = None) -> tuple[int, Any, bytes]:
        """发送同源请求并返回状态码、响应头和有界响应正文。"""
        request_headers = {'Origin': self.base}
        request_headers.update(headers or {})
        body = None if data is None else json.dumps(data).encode()
        if body is not None:
            request_headers['Content-Type'] = 'application/json'
        request = urllib.request.Request(self.base + path, data=body, headers=request_headers, method=method)
        try:
            response = self.opener.open(request, timeout=60)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            result = response.read(64 * 1024 * 1024)
            return response.code, response.headers, result

    def json(self, path: str, method: str = 'GET', data: dict[str, Any] | None = None, status: int = 200) -> dict[str, Any]:
        """读取 JSON 并检查状态，允许 POST 创建或排队返回 201／202。"""
        code, _, body = self.request(path, method, data)
        accepted = (status, 201, 202) if method == 'POST' and status == 200 else (status,)
        assert code in accepted, f'{method} {path}: {code}: {body[:500]!r}'
        return json.loads(body)

    def check(self, condition: bool, label: str) -> None:
        """记录可复查的中文断言结果，失败时立即停止。"""
        assert condition, label
        self.passed += 1
        print(f'  ✓ {label}', flush=True)


def verify_music(client: Client, anonymous: Client, root: Path) -> None:
    """验证音乐类型、元数据、封面、流式字节和重复扫描后的个人状态。"""
    result = client.json('/api/items?type=music&pageSize=96')
    tracks = result['items']
    source_names = {path.name for path in (root / 'music').iterdir() if path.suffix.lower() != '.txt'}
    client.check({track['relativePath'] for track in tracks} == source_names and all(track['type'] == 'music' for track in tracks), '音乐过滤收录全部实际音频且忽略非音乐')
    client.check(client.json('/api/stats')['music'] == len(tracks), '音乐数量与统计一致')
    mime_types: dict[str, str] = {'.mp3': 'audio/mpeg', '.m4a': 'audio/mp4', '.aac': 'audio/aac', '.flac': 'audio/flac', '.ogg': 'audio/ogg', '.opus': 'audio/ogg', '.wav': 'audio/wav', '.aif': 'audio/aiff', '.aiff': 'audio/aiff'}
    client.check(all(track['mime'] == mime_types[Path(track['relativePath']).suffix.lower()] for track in tracks), '所有音乐格式 MIME 稳定且识别大写扩展名')
    settings = client.json('/api/settings')
    if settings['ffprobeAvailable']:
        client.check(all(10 < track['duration'] < 14 for track in tracks), '真实音频时长完成异步探测')
    track = next(track for track in tracks if track['relativePath'].endswith('.wav'))
    path = f'/api/items/{track["id"]}'
    file_path = path + '/file'
    source = (root / 'music' / track['relativePath']).read_bytes()
    code, headers, body = client.request(file_path, 'HEAD')
    client.check(code == 200 and not body and int(headers['Content-Length']) == len(source) and headers['Content-Type'] == 'audio/wav' and headers['Accept-Ranges'] == 'bytes', '音乐 HEAD 返回精确长度、MIME 与 Range 能力')
    client.check(anonymous.request(file_path)[0] == 401 and anonymous.request(file_path, 'HEAD')[0] == 401, '音乐 GET 与 HEAD 均要求鉴权')
    for offset in (0, 44, len(source) - 64):
        code, headers, body = client.request(file_path, headers={'Range': f'bytes={offset}-{offset + 63}'})
        client.check(code == 206 and headers['Content-Range'] == f'bytes {offset}-{offset + 63}/{len(source)}' and body == source[offset:offset + 64], f'音乐 Range 在 {offset} 精确返回字节')
    code, _, body = client.request(file_path, headers={'Range': 'bytes=-64'})
    client.check(code == 206 and body == source[-64:] and client.request(file_path, headers={'Range': f'bytes={len(source)}-'})[0] == 416, '音乐尾部 Range 与越界 416 正确')
    code, headers, body = client.request(path + '/cover')
    client.check(code == 200 and headers['Content-Type'].startswith('image/svg+xml') and b'MUSIC' in body, '无内嵌封面返回音乐专属占位图')
    embedded = next((value for value in tracks if value['relativePath'].endswith('.MP3')), None)
    if embedded and settings['ffmpegAvailable'] and settings['ffprobeAvailable']:
        code, headers, body = client.request(f'/api/items/{embedded["id"]}/cover')
        client.check(code == 200 and headers['Content-Type'] == 'image/jpeg' and body.startswith(b'\xff\xd8'), '音乐内嵌专辑封面实际生成为 JPEG')
    client.json(path + '/favorite', 'POST', {'favorite': True})
    client.json(path + '/tags', 'PUT', {'tags': ['轻音乐', '音乐验证']})
    client.json(path + '/progress', 'PUT', {'position': 3, 'total': 12, 'locator': ''})
    client.check(client.json('/api/items?type=music&favorite=true')['total'] >= 1 and client.json('/api/items?type=music&q=' + urllib.parse.quote('音乐验证'))['total'] >= 1, '音乐收藏过滤与中文标签搜索可用')
    previous_jobs = client.json('/api/jobs')['jobs']
    client.json(f'/api/libraries/{track["libraryId"]}/scan', 'POST', {})
    deadline = time.monotonic() + 120
    while True:
        jobs = client.json('/api/jobs')['jobs']
        pending = [job for job in jobs if job['libraryId'] == track['libraryId'] and job['status'] in ('queued', 'running')]
        if not pending or time.monotonic() >= deadline:
            break
        time.sleep(0.2)
    rescanned = client.json('/api/items?type=music&pageSize=96')['items']
    updated = client.json(path)['item']
    client.check(not pending and {value['id'] for value in tracks} == {value['id'] for value in rescanned} and updated['favorite'] and updated['tags'] == ['轻音乐', '音乐验证'] and updated['progress']['position'] == 3, '音乐重复扫描 ID 稳定且保留收藏标签和秒数进度')
    old_enrich = {job['id'] for job in previous_jobs if job['libraryId'] == track['libraryId'] and job['kind'] == 'enrich'}
    new_enrich = {job['id'] for job in jobs if job['libraryId'] == track['libraryId'] and job['kind'] == 'enrich'}
    if settings['ffmpegAvailable'] and settings['ffprobeAvailable']:
        client.check(old_enrich == new_enrich and all(job['status'] == 'completed' for job in jobs if job['libraryId'] == track['libraryId']), '无封面音频正常完成且重扫不重复探测')
    client.check((root / 'music' / track['relativePath']).read_bytes() == source, '音乐扫描与播放不改写源文件')


def verify(base: str, root: Path, username: str, password: str, large: bool) -> None:
    """执行账户、建库、扫描、阅读、备份及大偏移协议的集成验证。"""
    client = Client(base)
    client.check(client.json('/healthz')['status'] == 'ok', '健康检查就绪')
    status = client.json('/api/status')
    client.json('/api/setup' if status['setupRequired'] else '/api/login', 'POST', {'username': username, 'password': password})
    client.check(bool(client.json('/api/status')['user']), '设置或登录后会话有效')
    anonymous = Client(base)
    client.check(anonymous.request('/api/items')[0] == 401, '未登录内容请求被拒绝')
    client.check(client.request('/api/libraries', 'POST', {}, {'Origin': 'https://foreign.invalid'})[0] == 403, '拒绝跨站写请求')
    client.check(client.request('/api/browse?path=' + urllib.parse.quote(str(root.parent.parent)))[0] in (400, 403), '目录浏览不能越过媒体根')
    existing = client.json('/api/libraries')['libraries']
    initial_stats = client.json('/api/stats')
    client.check(isinstance(initial_stats.get('music'), int), '统计始终提供音乐数字字段，包括空库')
    for kind, name in (('video', '验证视频'), ('image', '验证图片'), ('comic', '验证漫画'), ('novel', '验证小说'), ('music', '验证音乐')):
        matching = [library for library in existing if library['name'] == name]
        library = matching[0] if matching else client.json('/api/libraries', 'POST', {'name': name, 'type': kind, 'path': str(root / kind)})['library']
        client.json(f'/api/libraries/{library["id"]}/scan', 'POST', {})
    deadline = time.monotonic() + 120
    while True:
        jobs = client.json('/api/jobs')['jobs']
        pending = [job for job in jobs if job['status'] in ('queued', 'running')]
        if not pending or time.monotonic() > deadline:
            break
        time.sleep(0.5)
    failed_scans = [job for job in jobs if job['kind'] == 'scan' and job['status'] == 'failed']
    client.check(not pending and not failed_scans, f'五类扫描及衍生任务结束（扫描失败 {len(failed_scans)}）')
    items = client.json('/api/items?pageSize=200')['items']
    for kind in ('video', 'image', 'comic', 'novel', 'music'):
        client.check(any(item['type'] == kind for item in items), f'{kind} 类型实际入库')
    item = next(item for item in items if item['type'] == 'novel' and item['title'].find('夜航') >= 0)
    path = f'/api/items/{item["id"]}'
    client.json(path + '/favorite', 'POST', {'favorite': True})
    client.json(path + '/tags', 'PUT', {'tags': ['集成验证', '中文标签']})
    client.json(path + '/progress', 'PUT', {'position': 3, 'total': 100, 'locator': 'paragraph:3'})
    state = client.json(path)['item']
    client.check(state['favorite'] and '中文标签' in state['tags'] and state['progress']['position'] == 3, '收藏、标签、阅读进度写入后可读取')
    client.check(client.json('/api/items?q=' + urllib.parse.quote('中文标签'))['total'] >= 1, '可搜索中文标签')
    parsed = client.json(path + '/text')
    client.check(len(parsed['paragraphs']) > 20 and '夜航' in ''.join(parsed['paragraphs']), 'UTF-8 小说实际解析')
    gb = next(item for item in items if '旧日' in item['title'])
    client.check('城市' in ''.join(client.json(f'/api/items/{gb["id"]}/text?encoding=gb18030')['paragraphs']), 'GB18030 手动编码读取')
    for comic in [item for item in items if item['type'] == 'comic']:
        pages = client.json(f'/api/items/{comic["id"]}/comic')['pages']
        client.check(len(pages) == 3, f'漫画页面数量正确：{comic["title"]}')
        client.check('1' in pages[0]['name'] and '10' in pages[2]['name'], f'漫画自然页序正确：{comic["title"]}')
        code, _, body = client.request(pages[0]['url'])
        client.check(code == 200 and body.startswith(b'\x89PNG'), '漫画页输出真实图片')
    image = next(item for item in items if item['type'] == 'image')
    client.check(client.request(f'/api/items/{image["id"]}/cover')[0] == 200, '封面可读取')
    epub = next(item for item in items if item['relativePath'].endswith('.epub'))
    client.check(client.request(f'/api/items/{epub["id"]}/file')[2].startswith(b'PK'), 'EPUB 原文可交给本地阅读器')
    verify_music(client, anonymous, root)
    code, _, backup = client.request('/api/backup')
    client.check(code == 200 and backup.startswith(b'SQLite format 3\x00'), '下载一致性 SQLite 备份')
    (root.parent / 'verified-backup.db').write_bytes(backup)
    # 使用独立连接验证备份完整性，不触碰正在运行的数据库。
    import sqlite3
    with sqlite3.connect(root.parent / 'verified-backup.db') as connection:
        client.check(connection.execute('PRAGMA integrity_check').fetchone()[0] == 'ok', '备份数据库完整性检查通过')
        client.check(connection.execute('PRAGMA user_version').fetchone()[0] == 1 and connection.execute("SELECT count(*) FROM items WHERE type='music'").fetchone()[0] > 0, '音乐与原四类共存于兼容的 v1 数据库备份')
    if large:
        video = next(item for item in items if '5GiB' in item['title'])
        source = root / 'video' / '仅协议验证-5GiB.mp4'
        size = source.stat().st_size
        file_path = f'/api/items/{video["id"]}/file'
        code, headers, body = client.request(file_path, 'HEAD')
        client.check(code == 200 and int(headers['Content-Length']) == size and not body, 'HEAD 返回超过 4 GiB 的完整长度且无正文')
        for offset in (0, 4 * 1024 ** 3 + 17, size - 64):
            code, headers, body = client.request(file_path, headers={'Range': f'bytes={offset}-{offset + 63}'})
            client.check(code == 206 and headers['Content-Range'] == f'bytes {offset}-{offset + 63}/{size}' and body == bytes(range(64)), f'Range 大偏移 {offset} 字节精确匹配')
        code, headers, body = client.request(file_path, headers={'Range': 'bytes=-64'})
        client.check(code == 206 and body == bytes(range(64)), 'Suffix Range 正确读取片尾')
        client.check(client.request(file_path, headers={'Range': f'bytes={size + 1}-'})[0] == 416, '越界 Range 返回 416')
        _, last_headers, _ = client.request(file_path, 'HEAD')
        if last_headers.get('Last-Modified'):
            client.check(client.request(file_path, 'HEAD', headers={'If-Modified-Since': last_headers['Last-Modified']})[0] == 304, '条件请求命中资源验证信息')
        client.check(anonymous.request(file_path, 'HEAD')[0] == 401, '超大媒体 HEAD 同样要求登录')
    client.json('/api/logout', 'POST', {})
    client.check(client.request('/api/items')[0] == 401, '退出后会话失效')
    print(f'集成验证通过：{client.passed} 项断言。协议样本不代表真实视频性能。', flush=True)


def main() -> None:
    """解析有中文说明的验证参数并执行指定阶段。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path('.runtime/verify/media'), help='隔离样本根目录；必须包含 verify 路径段')
    parser.add_argument('--base-url', default='http://127.0.0.1:8090', help='已启动的被测应用根地址')
    parser.add_argument('--prepare-only', action='store_true', help='只准备样本，不请求应用')
    parser.add_argument('--large-file', action='store_true', help='创建并验证 5 GiB 稀疏文件，仅验证 HTTP 偏移')
    parser.add_argument('--ffmpeg', default=shutil.which('ffmpeg'), help='FFmpeg 可执行文件路径，用于生成真实短视频')
    parser.add_argument('--username', default='nas-verify', help='隔离验证实例的管理员名称')
    parser.add_argument('--password', default=None, help='已有验证实例的密码；首次运行可自动生成')
    args = parser.parse_args()
    root = args.root.resolve()
    if 'verify' not in root.parts:
        parser.error('为避免污染真实媒体，--root 路径必须包含 verify 路径段')
    password_file = root.parent / 'credentials.json'
    password = args.password
    if not password and password_file.exists():
        password = json.loads(password_file.read_text(encoding='utf-8'))['password']
    if not password:
        password = secrets.token_urlsafe(24)
        password_file.parent.mkdir(parents=True, exist_ok=True)
        password_file.write_text(json.dumps({'password': password}), encoding='utf-8')
        if os.name != 'nt':
            password_file.chmod(0o600)
    prepare(root, args.ffmpeg, args.large_file)
    print(f'隔离样本已准备：{root}；本机凭据保存于忽略目录，禁止用于公开实例。', flush=True)
    if not args.prepare_only:
        verify(args.base_url, root, args.username, password, args.large_file)


if __name__ == '__main__':
    main()
