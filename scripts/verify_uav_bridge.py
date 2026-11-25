"""真实锁定UAV解析器与原生HLS离线验证；站点传输被固定样例替代。"""
from __future__ import annotations

import http.server
import importlib.metadata
import io
import json
import os
from pathlib import Path
import re
import runpy
import shutil
import socket
import subprocess
import sys
import threading
from typing import Any
from urllib.parse import parse_qs, urlsplit
from uuid import uuid4

from project_env import ROOT, run_cli, verification_directory

MODE: str = "normal"  # 固定传输样例的搜索或媒体失败分支。
CALLS: list[dict[str, Any]] = []  # 只在内存核验请求代理，不持久化秘密。
BASE: Path = ROOT  # main初始化的独占验证目录。


def deny_network(*args: Any, **kwargs: Any) -> Any:
    """真实上游解析期间禁止任何未替换套接字访问。"""
    raise AssertionError("离线解析验证禁止真实网络")


def fixture_dns(host: str, port: int, *args: Any, **kwargs: Any) -> list[tuple[Any, ...]]:
    """只给固定离线域名返回公网样例地址，不替代生产实际连接策略。"""
    assert host in {"huangguoai.com", "media.example.com", "variant.example.com"}
    return [(socket.AF_INET, socket.SOCK_STREAM, socket.IPPROTO_TCP, "", ("93.184.216.34", port))]


def transport(session: Any, method: str, url: str, **kwargs: Any) -> Any:
    """仅替代底层HTTP传输，保留上游真实搜索、详情与m3u8解析。"""
    import requests
    assert kwargs["proxies"]["https"].startswith("http://worker:fixture@127.0.0.1:")
    assert kwargs["allow_redirects"] is False
    CALLS.append({"url": url, "method": method.upper(), "headers": dict(kwargs.get("headers", {}))})
    parsed = urlsplit(url)
    site = "jable" if parsed.hostname in {"jable.tv", "fs1.app"} else "missav" if str(parsed.hostname).startswith("missav") else "huangguo" if parsed.hostname == "huangguoai.com" else "hanime1"
    query = parse_qs(parsed.query)
    search_page = "/search" in parsed.path
    status: int = 200
    headers: dict[str, str] = {"Content-Type": "text/html; charset=utf-8"}
    if parsed.path == "/redirect-fixture":
        body = "intermediate-body-must-not-mix"
        status, headers["Location"] = 302, "https://media.example.com/final-fixture"
    elif parsed.path == "/final-fixture":
        body = "final-body-only"
    elif parsed.path.startswith("/api/videos/"):
        assert method.upper() == "POST" and kwargs.get("json") == {} and not session.cookies
        assert not any(key.lower() in {"cookie", "authorization"} for key in kwargs["headers"])
        data = {"id": 42, "ep": 3, "manifest_method": "GET", "streams": {"main": "https://media.example.com/hg-master.m3u8?sig=PRIVATE-UAV-SIGNATURE"}, "cdn": {"m3u8Enabled": True, "otherEnabled": False, "authKey": "x-cdn-auth", "authValue": "PRIVATE-UAV-CDN", "app": "150"}}
        if MODE == "playback-identity":
            data["ep"] = 1
        if MODE == "playback-flags":
            data["cdn"]["otherEnabled"] = "true"
        body = json.dumps({"code": 200, "data": data})
        if MODE == "playback-redirect":
            status, headers["Location"] = 302, "https://media.example.com/forbidden-playback-redirect"
    elif parsed.hostname in {"media.example.com", "variant.example.com"}:
        if "master" in parsed.path:
            variant = 'https://variant.example.com/media.m3u8' if parsed.path.startswith('/hg-') else 'media.m3u8'
            body = '#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=100000\n' + variant + '\n'
        else:
            body = '#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\nsegment.ts\n' + ('' if MODE == "live" else '#EXT-X-ENDLIST\n')
            if MODE == "gap":
                body = body.replace('#EXTINF', '#EXT-X-GAP\n#EXTINF')
            elif MODE == "parts":
                body = body.replace('#EXTINF', '#EXT-X-PART:DURATION=0.5,URI="part.ts"\n#EXTINF')
            elif MODE == "drm":
                body = body.replace('#EXTINF', '#EXT-X-KEY:METHOD=SAMPLE-AES,URI="media.key"\n#EXTINF')
            elif MODE == "keyformat":
                body = body.replace('#EXTINF', '#EXT-X-KEY:METHOD=AES-128,URI="media.key",KEYFORMAT="unsupported"\n#EXTINF')
            elif MODE == "init":
                body = body.replace('#EXTINF', '#EXT-X-MAP:URI="init.mp4"\n#EXTINF')
            elif MODE in {"skip", "server-control", "part-inf", "preload-hint", "rendition-report", "session-key", "content-steering", "external-audio", "unknown-tag"}:
                tag = {"skip": '#EXT-X-SKIP:SKIPPED-SEGMENTS=5', "server-control": '#EXT-X-SERVER-CONTROL:CAN-SKIP-UNTIL=12', "part-inf": '#EXT-X-PART-INF:PART-TARGET=0.5', "preload-hint": '#EXT-X-PRELOAD-HINT:TYPE=PART,URI="next.ts"', "rendition-report": '#EXT-X-RENDITION-REPORT:URI="other.m3u8"', "session-key": '#EXT-X-SESSION-KEY:METHOD=AES-128,URI="key.bin"', "content-steering": '#EXT-X-CONTENT-STEERING:SERVER-URI="steering.json"', "external-audio": '#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",URI="audio.m3u8"', "unknown-tag": '#EXT-X-UNREVIEWED:VALUE=1'}[MODE]
                body = body.replace('#EXTINF', tag + '\n#EXTINF')
            elif MODE == "http-segment":
                body = body.replace('segment.ts', 'http://media.example.com/segment.ts')
    elif search_page and MODE == "broken":
        body = '<html><body>unexpected upstream layout</body></html>'
    elif search_page and MODE == "empty":
        body = '<main><section class="hg-list-page"></section></main>' if site == "huangguo" else '<main class="container grid"><div id="list_videos_common_videos_list">No videos found</div></main>'
    elif search_page and site == "huangguo":
        assert parsed.path == "/search/video/fixture/2/"
        body = '<main><section class="hg-list-page"><div class="hg-card-grid"><div class="hg-drama-card"><a class="hg-drama-card__cover-link" href="/video/42/"></a><h3 class="hg-drama-card__title"><a>测试剧集</a></h3></div></div><a href="/search/video/fixture/3/">next</a></section></main>'
    elif search_page and site == "jable":
        assert query.get("from") == ["2"]
        body = '<div id="list_videos_common_videos_list"><div class="video-img-box"><div class="detail"><h6><a href="https://jable.tv/videos/test-123/">测试视频</a></h6></div><span class="label">01:02</span></div></div><a href="?q=fixture&from=3">next</a>'
    elif search_page and site == "missav":
        assert query.get("page") == ["2"]
        body = '<div class="thumbnail"><a href="https://missav.ai/en/test-123"><img alt="测试视频"></a></div><a href="?page=3">next</a>'
    elif search_page:
        assert query.get("page") == ["2"]
        body = '<div class="video-item-container"><a class="video-link" href="/watch?v=123"></a><div class="title">测试视频</div><div class="duration">01:02</div></div><a href="?query=fixture&page=3">next</a>'
    elif site == "huangguo":
        episode = int(re.search(r"/ep-([0-9]+)/", parsed.path)[1]) if "/ep-" in parsed.path else 1
        data = {"id": "42", "ep": episode, "title": "测试剧集", "videoSrc": "", "videoApi": "", "videoKey": "", "videoIv": ""}
        episodes = range(1, 102) if MODE == "oversized" else (1, 3)
        links = ''.join(f'<a class="hg-web-play__ep" data-ep-id="{number}" href="/video/42/' + (f'ep-{number}/' if number != 1 else '') + f'">{number}</a>' for number in episodes)
        body = '<script id="videoInitialData" type="application/json">' + json.dumps(data) + '</script><div data-web-play>' + links + '<a class="hg-web-play__ep" data-ep-id="2" href="/video/999/ep-2/">other series</a><a class="hg-web-play__ep" data-ep-id="4" href="/video/42/ep-9/">mismatch</a></div>'
    elif site == "hanime1" and parsed.path == "/":
        body = '<html><body>home</body></html>'
    elif site == "hanime1":
        body = '<html><head><meta property="og:title" content="测试视频"></head><body><h3 class="video-details-wrapper">测试视频</h3><video><source src="https://media.example.com/video-1080p.mp4"></video></body></html>'
    else:
        body = '<meta property="og:title" content="测试视频"><meta property="og:image" content="https://media.example.com/image.jpg"><script>var source="https://media.example.com/master.m3u8";</script>'
        if site == "missav":
            body = body.replace('var source="https://media.example.com/master.m3u8";', "eval(function(p,a,c,k,e,d){}('source=https://media.example.com/master.m3u8',36,0,''.split('|')))")
    content = body.encode("utf-8")
    if kwargs.get("content_callback"):
        from curl_cffi.requests import Response
        response = Response()
    else:
        response = requests.Response()
    assert method.upper() == "GET" or parsed.path.startswith("/api/videos/")
    response.status_code = status
    response.url = url
    response.encoding = "utf-8"
    response.headers = headers
    response.raw = io.BytesIO(content)
    if kwargs.get("content_callback"):
        kwargs["content_callback"](content)
        response.content = b""
    return response


def bridge_checks() -> dict[str, Any]:
    """核验真实隔离探测、四站搜索/详情、明确空结果与坏页失败。"""
    global MODE
    import requests
    from curl_cffi import requests as curl_requests
    from uav_downloader.core import config
    from uav_downloader.sites import jabletv, missav, hanime1, huangguo

    source = ROOT / "internal/collectionworker/uav_bridge.py"
    probe = subprocess.run([sys.executable, "-I", "-B", "-c", source.read_text(encoding="utf-8")], input=b'{"action":"probe"}', capture_output=True, cwd=BASE, timeout=20, check=True)
    assert json.loads(probe.stdout)["version"] == "3.8.4" and not probe.stderr
    namespace = runpy.run_path(str(source), run_name="uav_offline_verification")
    original_request = requests.sessions.Session.request
    original_curl = curl_requests.Session.request
    original_socket = socket.socket.connect
    original_dns = socket.getaddrinfo
    requests.sessions.Session.request = transport
    curl_requests.Session.request = transport
    socket.socket.connect = deny_network
    socket.getaddrinfo = fixture_dns
    modules = {"jable": jabletv, "missav": missav, "hanime1": hanime1, "huangguo": huangguo}
    counts = {"search": 0, "resolve": 0, "empty": 0, "bad": 0, "transportRedirect": 0}
    try:
        responses = namespace["install_transport"]("http://worker:fixture@127.0.0.1:12345", config)
        for site in modules:
            MODE = "normal"
            huangguo.HuangguoBrowser.clear_metadata_cache()
            result = namespace["search"]({"site": site, "query": "fixture", "page": 2}, modules, responses)
            assert len(result["items"]) == (2 if site == "huangguo" else 1) and result["hasNext"] and "://" not in json.dumps(result, ensure_ascii=False)
            if site == "huangguo":
                assert [item["videoId"] for item in result["items"]] == ["huangguo-42-ep-1", "huangguo-42-ep-3"]
            counts["search"] += 1
            resolved = namespace["resolve"]({"site": site, "videoId": "huangguo-42-ep-3" if site == "huangguo" else "123" if site == "hanime1" else "test-123"}, modules)
            assert resolved["mediaUrl"].startswith("https://variant.example.com/" if site == "huangguo" else "https://media.example.com/")
            assert bool(resolved["manifest"]) == (site != "hanime1")
            if site == "huangguo":
                assert resolved["cdn"]["origin"] == "https://media.example.com" and resolved["cdn"]["other"] is False
                origin_call = next(call for call in CALLS if "/hg-master" in call["url"])
                variant_call = next(call for call in CALLS if "variant.example.com" in call["url"])
                assert origin_call["headers"].get("x-cdn-auth") == "PRIVATE-UAV-CDN"
                assert "x-cdn-auth" not in variant_call["headers"]
                assert "x-cdn-auth" not in resolved["headers"]
            counts["resolve"] += 1
            MODE = "empty"
            empty = namespace["search"]({"site": site, "query": "fixture", "page": 2}, modules, responses)
            assert empty["items"] == [] and not empty["hasNext"]
            counts["empty"] += 1
            MODE = "broken"
            try:
                namespace["search"]({"site": site, "query": "fixture", "page": 2}, modules, responses)
                raise AssertionError("坏页被错误接受")
            except ValueError:
                counts["bad"] += 1
        for MODE in ("live", "gap", "parts", "drm", "keyformat", "skip", "server-control", "part-inf", "preload-hint", "rendition-report", "session-key", "content-steering", "external-audio", "unknown-tag", "http-segment"):
            try:
                namespace["resolve_manifest"]("https://media.example.com/media.m3u8", {})
                raise AssertionError("不完整或非标准清单被错误接受：" + MODE)
            except ValueError:
                counts["bad"] += 1
        for MODE in ("playback-redirect", "playback-identity", "playback-flags", "init"):
            try:
                namespace["resolve"]({"site": "huangguo", "videoId": "huangguo-42-ep-3"}, modules)
                raise AssertionError("黄果错误播放信息被接受：" + MODE)
            except ValueError:
                counts["bad"] += 1
        assert not any("forbidden-playback-redirect" in call["url"] for call in CALLS)
        MODE = "oversized"
        huangguo.HuangguoBrowser.clear_metadata_cache()
        try:
            namespace["search"]({"site": "huangguo", "query": "fixture", "page": 2}, modules, responses)
            raise AssertionError("超限分集被静默截断")
        except ValueError:
            counts["bad"] += 1
        MODE = "normal"
        for session in (requests.Session(), curl_requests.Session()):
            assert session.get("https://media.example.com/redirect-fixture").content == b"final-body-only"
            assert session.get("https://media.example.com/redirect-fixture", allow_redirects=False).status_code == 302
            session.close()
            counts["transportRedirect"] += 1
        for site, invalid in (("jable", "../x"), ("missav", "dm123"), ("missav", "DM123"), ("missav", "new"), ("hanime1", "01"), ("huangguo", "42"), ("huangguo", "huangguo-42-ep-0"), ("huangguo", "huangguo-42-ep-2147483648")):
            assert not namespace["valid_id"](site, invalid)
        assert CALLS
    finally:
        requests.sessions.Session.request = original_request
        curl_requests.Session.request = original_curl
        socket.socket.connect = original_socket
        socket.getaddrinfo = original_dns
    return counts


class HLSHandler(http.server.BaseHTTPRequestHandler):
    """只为本次原生下载器验证提供有限片段与失败样例。"""

    def do_GET(self) -> None:
        """按固定路径发送样例媒体，记录请求用于识别非预期下载行为。"""
        path = urlsplit(self.path).path
        self.server.requests_seen.append(path)
        if path in {"/segment.ts", "/encrypted.ts", "/key.bin"}:
            body = (BASE / path[1:]).read_bytes()
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        else:
            self.send_error(404)

    def log_message(self, format: str, *args: Any) -> None:
        """禁止HTTP默认日志记录临时请求信息。"""
        return


def hls_checks() -> dict[str, Any]:
    """用真实锁定yt-dlp和FFmpeg核验内联清单、缺片、DRM及回退隔离。"""
    assert importlib.metadata.version("yt-dlp") == "2026.10.26"
    ffmpeg = shutil.which("ffmpeg")
    assert ffmpeg, "离线HLS验证需要FFmpeg"
    subprocess.run([ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=64x64:d=1", "-c:v", "mpeg2video", "-f", "mpegts", str(BASE / "segment.ts")], check=True, capture_output=True, timeout=20)
    from Crypto.Cipher import AES
    key = bytes(range(16))
    source = (BASE / "segment.ts").read_bytes()
    padding = 16 - len(source) % 16
    (BASE / "key.bin").write_bytes(key)
    (BASE / "encrypted.ts").write_bytes(AES.new(key, AES.MODE_CBC, iv=bytes(16)).encrypt(source + bytes([padding]) * padding))
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), HLSHandler)
    server.requests_seen = []
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base = f"http://127.0.0.1:{server.server_port}"
    manifest = '#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\nsegment.ts\n#EXT-X-ENDLIST\n'
    command = [sys.executable, "-I", "-B", "-m", "yt_dlp", "--ignore-config", "--no-plugin-dirs", "--no-js-runtimes", "--no-remote-components", "--no-playlist", "--no-cache-dir", "--no-warnings", "--no-progress", "--retries", "0", "--fragment-retries", "0", "--abort-on-unavailable-fragments", "--downloader", "native", "--load-info-json", "-", "--fixup", "never", "--ffmpeg-location", str(BASE / ".no-network-ffmpeg"), "--no-continue", "--no-overwrites", "--output", "media.download"]
    try:
        results: dict[str, Any] = {}
        for name in ("normal", "aes128", "missing", "unsupported"):
            directory = BASE / name
            directory.mkdir()
            playlist = manifest.replace("segment.ts", "missing.ts") if name == "missing" else manifest
            if name == "unsupported":
                playlist = playlist.replace("#EXTINF", '#EXT-X-KEY:METHOD=SAMPLE-AES,URI="key.bin"\n#EXTINF')
            if name == "aes128":
                playlist = playlist.replace("segment.ts", "encrypted.ts").replace("#EXTINF", '#EXT-X-KEY:METHOD=AES-128,URI="key.bin",IV=0x00000000000000000000000000000000\n#EXTINF')
            info = {"id": "123", "title": "fixture", "url": base + "/never-fetched.m3u8", "protocol": "m3u8_native", "ext": "mp4", "extractor_key": "UAV", "is_live": False, "hls_media_playlist_data": playlist}
            env = dict(os.environ)
            for key in list(env):
                if "proxy" in key.lower():
                    del env[key]
            completed = subprocess.run(command, input=json.dumps(info).encode(), capture_output=True, cwd=directory, env=env, timeout=30)
            assert (completed.returncode == 0) == (name in {"normal", "aes128"}), name
            results[name] = completed.returncode
            if name in {"normal", "aes128"}:
                decoded = subprocess.run([ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-nostats", "-progress", "pipe:1", "-stats_period", "86400", "-xerror", "-protocol_whitelist", "file,pipe,crypto,data", "-i", str(directory / "media.download"), "-map", "0:v:0", "-f", "null", "-"], check=True, capture_output=True, timeout=20)
                assert b"progress=end" in decoded.stdout and any(int(match) > 0 for match in re.findall(rb"frame=([0-9]+)", decoded.stdout))
        assert "/never-fetched.m3u8" not in server.requests_seen and server.requests_seen.count("/key.bin") == 1
        return {"exitCodes": results, "requests": server.requests_seen, "inlineManifest": True, "networkFFmpegFallback": False, "aes128": True, "decodedFrames": True}
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


def main() -> None:
    """运行真实解析与本机媒体回归并保存不含秘密的验证报告。"""
    global BASE
    BASE = verification_directory() / ("uav-real-bridge-" + uuid4().hex)
    BASE.mkdir(mode=0o700)
    result = {"bridge": bridge_checks(), "hls": hls_checks(), "scope": "真实固定包解析+替代站点传输+真实本机原生HLS；不证明公网可用或Docker运行"}
    (BASE / "result.json").write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    raise SystemExit(run_cli(main))
