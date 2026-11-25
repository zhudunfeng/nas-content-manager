"""固定UAV无头协议：仅搜索和解析，不调用上游下载、GUI或任意插件。"""
from __future__ import annotations

import contextlib
import importlib.metadata
import json
import math
import os
import re
import sys
import time
import unicodedata
from typing import Any
from urllib.parse import parse_qs, quote, urljoin, urlsplit

VERSION = "3.8.4"  # 唯一允许的上游包版本。
COMMIT = "d40f09457fbd38dd7dc94580637242afafaf3939"  # 构建时核验的源码提交。
SOURCE_SHA256 = "88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41"  # 原始源码包摘要，探测时验证安装来源。
MAX_RESPONSE = 4 << 20  # 单次站点响应硬上限，避免错误页撑满进程内存。
MAX_MANIFEST = 512 << 10  # 允许返回私有协议的有限HLS清单上限。
SLUG = re.compile(r"[A-Za-z0-9][A-Za-z0-9_-]{0,199}\Z")  # 不含路径的编号白名单。
EMPTY_MARKERS = ("no results", "no videos found", "nothing found", "沒有找到", "没有找到", "未找到", "無搜尋結果", "没有搜索结果")  # 明确的站点空搜索文案。
HLS_TAGS = frozenset({"#EXTM3U", "#EXTINF", "#EXT-X-VERSION", "#EXT-X-TARGETDURATION", "#EXT-X-MEDIA-SEQUENCE", "#EXT-X-DISCONTINUITY-SEQUENCE", "#EXT-X-PLAYLIST-TYPE", "#EXT-X-ENDLIST", "#EXT-X-KEY", "#EXT-X-MAP", "#EXT-X-BYTERANGE", "#EXT-X-DISCONTINUITY", "#EXT-X-PROGRAM-DATE-TIME", "#EXT-X-DATERANGE", "#EXT-X-INDEPENDENT-SEGMENTS", "#EXT-X-ALLOW-CACHE", "#EXT-X-STREAM-INF"})  # 仅允许已审查的有限完整HLS标签；增量、低延迟及未知扩展均拒绝。


def clean_text(value: Any, limit: int = 300) -> str:
    """清理上游文本并限制字符数，禁止URL或控制字符进入公开信息。"""
    text = str(value or "")
    text = " ".join("".join(ch for ch in text if not unicodedata.category(ch).startswith("C")).split())[:limit]
    return "视频媒体" if "://" in text else text


def valid_id(site: str, value: str) -> bool:
    """与Go共享严格编号约束，拒绝路径、语言前缀和栏目编号。"""
    if site == "hanime1":
        return re.fullmatch(r"[1-9][0-9]{0,11}", value) is not None
    if site == "huangguo":
        match = re.fullmatch(r"huangguo-([1-9][0-9]{0,18})-ep-([1-9][0-9]{0,9})", value)
        return bool(match and int(match[1]) <= 9223372036854775807 and int(match[2]) <= 2147483647)
    if not SLUG.fullmatch(value):
        return False
    if site == "jable":
        return value == value.lower()
    return site == "missav" and re.search(r"[0-9]", value) is not None and re.fullmatch(r"dm[0-9]+", value, re.I) is None


def http_url(value: str) -> str:
    """只允许无凭据的HTTP(S)地址，实际公网IP由父进程代理逐跳核验。"""
    parsed = urlsplit(value)
    if len(value) > 8192 or any(ch in value for ch in "\r\n\x00") or parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password or parsed.fragment or parsed.port not in {None, 80, 443}:
        raise ValueError("媒体地址不允许")
    return value


def canonical_url(site: str, video_id: str) -> str:
    """只从固定站点和规范编号生成上游入口，不接受任意地址。"""
    if not valid_id(site, video_id):
        raise ValueError("视频编号无效")
    if site == "huangguo":
        match = re.fullmatch(r"huangguo-([0-9]+)-ep-([0-9]+)", video_id)
        return f"https://huangguoai.com/video/{match[1]}/" + (f"ep-{match[2]}/" if match[2] != "1" else "")
    return {"jable": f"https://jable.tv/videos/{video_id}/", "missav": f"https://missav.ai/{video_id}", "hanime1": f"https://hanime1.me/watch?v={video_id}"}[site]


def item_id(site: str, value: str) -> str:
    """校验上游搜索链接域名并只提取规范编号，不向浏览器返回链接。"""
    parsed = urlsplit(value)
    hosts = {"jable": {"jable.tv", "fs1.app"}, "missav": {"missav.ai", "missav.ws", "missav.live", "missav123.com"}, "hanime1": {"hanime1.me", "www.hanime1.me"}, "huangguo": {"huangguoai.com", "www.huangguoai.com"}}
    if parsed.scheme != "https" or parsed.hostname not in hosts[site] or parsed.username or parsed.password or parsed.port not in {None, 443} or parsed.fragment:
        raise ValueError("搜索链接来源异常")
    if site == "huangguo":
        from uav_downloader.core.video_identity import huangguo_video_code
        video_id = huangguo_video_code(value)
    elif site == "hanime1":
        query = parse_qs(parsed.query)
        video_id = query.get("v", [""])[0] if parsed.path == "/watch" and len(query.get("v", [])) == 1 else ""
    else:
        video_id = parsed.path.rstrip("/").rsplit("/", 1)[-1]
        if site == "jable" and parsed.path != f"/videos/{video_id}/":
            raise ValueError("搜索视频路径异常")
    if not valid_id(site, video_id):
        raise ValueError("搜索视频编号异常")
    return video_id


def install_transport(proxy: str, config: Any) -> list[Any]:
    """强制两个上游HTTP栈走父进程策略代理，并在读取时限制响应大小。"""
    import requests
    from curl_cffi import requests as curl_requests

    parsed = urlsplit(proxy)
    if parsed.scheme != "http" or parsed.hostname != "127.0.0.1" or not parsed.port or parsed.username != "worker" or not parsed.password or parsed.path not in {"", "/"} or parsed.query or parsed.fragment:
        raise ValueError("缺少受控公网代理")
    proxies = {"http": proxy, "https": proxy, "all": proxy}
    responses: list[Any] = []

    def proxy_kwargs() -> dict[str, dict[str, str]]:
        """覆盖上游默认直连配置，不能被HTTP_PROXY空值或偏好文件绕过。"""
        return {"proxies": dict(proxies)}

    config.proxy_request_kwargs = proxy_kwargs
    config.RATE_LIMIT_WAITS = {}
    original_request = requests.sessions.Session.request

    def next_redirect(response: Any) -> str:
        """逐跳跟随有限重定向，所有中间正文仍受预算约束和代理核验。"""
        if int(response.status_code) in {301, 302, 303, 307, 308} and response.headers.get("Location"):
            return http_url(urljoin(str(response.url), str(response.headers["Location"])))
        return ""

    def request(session: Any, method: str, url: str, **kwargs: Any) -> Any:
        """有界读取requests响应并保留上游解析所需普通Response。"""
        session.trust_env = False
        follow_redirects = kwargs.pop("allow_redirects", True)
        kwargs.update(proxies=dict(proxies), timeout=12, stream=True, allow_redirects=False)
        for _ in range(6):
            response = original_request(session, method, http_url(str(url)), **kwargs)
            try:
                content = bytearray()
                for chunk in response.iter_content(65536):
                    content.extend(chunk)
                    if len(content) > MAX_RESPONSE:
                        raise ValueError("站点响应超过限制")
                response._content = bytes(content)
                response._content_consumed = True
            finally:
                response.close()
            url = next_redirect(response) if follow_redirects else ""
            if not url:
                responses.append(response)
                del responses[:-12]
                return response
        raise ValueError("站点重定向次数超限")

    requests.sessions.Session.request = request
    original_curl = curl_requests.Session.request

    def curl_request(session: Any, method: str, url: str, **kwargs: Any) -> Any:
        """通过curl回调限制解压后的正文，强制代理且不保留原始错误日志。"""
        kwargs.pop("proxy", None)
        follow_redirects = kwargs.pop("allow_redirects", True)
        kwargs.update(proxies=dict(proxies), timeout=12, stream=False, allow_redirects=False)
        for _ in range(6):
            content = bytearray()

            def collect(chunk: bytes) -> int:
                """达到响应预算立即终止本次curl接收，每个重定向重新计数。"""
                if len(content) + len(chunk) > MAX_RESPONSE:
                    raise ValueError("站点响应超过限制")
                content.extend(chunk)
                return len(chunk)

            response = original_curl(session, method, http_url(str(url)), content_callback=collect, **kwargs)
            response.content = bytes(content)
            url = next_redirect(response) if follow_redirects else ""
            if not url:
                responses.append(response)
                del responses[:-12]
                return response
        raise ValueError("站点重定向次数超限")

    curl_requests.Session.request = curl_request
    return responses


def empty_page(response: Any, site: str) -> bool:
    """仅确认站点真实空容器或明确空文案，不把未知布局当空结果。"""
    from bs4 import BeautifulSoup
    if int(getattr(response, "status_code", 0)) != 200:
        return False
    soup = BeautifulSoup(response.content, "html.parser")
    if site == "huangguo":
        # 黄果上游以空hg-list-page表达真实空页，不要求额外空文案。
        container = soup.select_one("main .hg-list-page")
        return container is not None and not container.select(".hg-drama-card")
    selectors = {"jable": "[id^=list_videos]", "missav": "main, .grid, [class*=grid]", "hanime1": "#content, #search-result, .search-result, .container", "huangguo": "main, .container, [data-search], .search-results"}
    return bool(soup.select_one(selectors[site])) and any(marker in soup.get_text(" ", strip=True).casefold() for marker in EMPTY_MARKERS)


def search(input_data: dict[str, Any], modules: dict[str, Any], responses: list[Any]) -> dict[str, Any]:
    """调用真实上游搜索解析，区分坏页、异常与明确空结果。"""
    from bs4 import BeautifulSoup
    site = str(input_data.get("site", ""))
    query = str(input_data.get("query", "")).strip()
    page = input_data.get("page", 1)
    if site not in modules or not 1 <= len(query) <= 120 or any(unicodedata.category(ch) == "Cc" for ch in query) or not isinstance(page, int) or isinstance(page, bool) or not 1 <= page <= 1000:
        raise ValueError("搜索参数无效")
    module = modules[site]
    if site in {"jable", "missav"}:
        original_fetch = module.fetch_with_mirrors

        def fetch(scraper: Any, url: str, site_key: str, validate: Any, **kwargs: Any) -> Any:
            """为原搜索解析补充明确空页验证，不把解析失败当空结果。"""
            return original_fetch(scraper, url, site_key, lambda response: validate(response) or empty_page(response, site), **kwargs)

        module.fetch_with_mirrors = fetch
    if site == "jable":
        url = f"https://jable.tv/search/?q={quote(query, safe='')}&from={page}"
        browser = module.JableTVBrowser
    elif site == "missav":
        browser = module.MissAVBrowser
        url = browser.page_url(f"https://missav.ai/search/{quote(query, safe='')}", page)
    elif site == "hanime1":
        browser = module.Hanime1Browser
        url = browser.page_url(browser.search_url(query), page)
    else:
        browser = module.HuangguoBrowser
        url = browser.page_url(browser.search_url(query), page)
    listing_response: list[Any] = []
    if site == "huangguo":
        # 按单集展示且严格执行协议100条上限，超限明确失败，不静默截断或批量下载。
        module._MAX_EPISODES_PER_SERIES = 100
        module._MAX_EPISODES_PER_PAGE = 100
        original_page = module._page_soup

        def page_soup(value: str) -> Any:
            """保留搜索页的已验证响应，避免作品展开覆盖分页依据。"""
            soup = original_page(value)
            if urlsplit(value).path == urlsplit(url).path:
                listing_response.extend(responses[-1:])
            return soup

        module._page_soup = page_soup
        try:
            items = browser.fetch_page(url, deadline=time.monotonic() + 22)
        finally:
            module._page_soup = original_page
    else:
        items = browser.fetch_page(url)
    candidates = [response for response in responses if urlsplit(str(response.url)).path == urlsplit(url).path and int(response.status_code) == 200]
    candidates = listing_response or candidates
    if not candidates or not isinstance(items, list) or len(items) > 100:
        raise ValueError("站点搜索响应无效")
    response = candidates[-1]
    soup = BeautifulSoup(response.content, "html.parser")
    if not items and not empty_page(response, site):
        raise ValueError("搜索页面缺少可识别结果或明确空结果")
    result: list[dict[str, str]] = []
    seen: set[str] = set()
    for item in items:
        video_id = item_id(site, str(item.get("url", "")))
        if video_id in seen:
            continue
        seen.add(video_id)
        result.append({"videoId": video_id, "title": clean_text(item.get("title")), "duration": clean_text(item.get("duration"), 20)})
    has_next = False
    for anchor in soup.select("a[href]"):
        candidate = urlsplit(urljoin(str(response.url), str(anchor.get("href", ""))))
        params = parse_qs(candidate.query)
        next_value = params.get("from" if site == "jable" else "page", [""])[0]
        # 只承认同一搜索路径中下一页的真实链接，不能由条数猜测。
        if candidate.hostname == urlsplit(str(response.url)).hostname and candidate.path == urlsplit(url).path and next_value == str(page + 1):
            has_next = True
        if site == "huangguo" and candidate.hostname == "huangguoai.com" and candidate.path == urlsplit(browser.page_url(browser.search_url(query), page + 1)).path:
            has_next = True
    return {"site": site, "query": query, "page": page, "hasNext": has_next and page < 1000, "items": result}


def resolve_manifest(url: str, headers: dict[str, str], media_get: Any = None) -> tuple[str, str]:
    """选择最多三层HLS最高质量变体，拒绝直播、外部音轨和不完整清单。"""
    import requests
    import m3u8
    for _ in range(4):
        if urlsplit(http_url(url)).scheme != "https":
            raise ValueError("HLS仅支持HTTPS清单与资源")
        response = media_get(http_url(url)) if media_get is not None else requests.get(http_url(url), headers=headers)
        if response.status_code != 200 or len(response.content) > MAX_MANIFEST or urlsplit(http_url(str(response.url))).scheme != "https":
            raise ValueError("媒体清单不可用")
        text = response.content.decode("utf-8-sig", errors="strict")
        if not text.lstrip().startswith("#EXTM3U"):
            raise ValueError("媒体清单格式无效")
        for line in text.splitlines():
            tag = line.strip().split(":", 1)[0]
            if tag.startswith("#EXT") and tag not in HLS_TAGS:
                raise ValueError("HLS包含未经支持的扩展或不完整清单特性")
        playlist = m3u8.loads(text, uri=str(response.url))
        if playlist.is_variant:
            choices = [entry for entry in playlist.playlists if not entry.stream_info.audio]
            if not choices:
                raise ValueError("暂不支持分离音轨HLS")
            selected = max(choices, key=lambda entry: (entry.stream_info.bandwidth or 0))
            url = http_url(selected.absolute_uri)
            continue
        if not playlist.is_endlist or not playlist.segments or len(playlist.segments) > 20000:
            raise ValueError("仅支持有限完整HLS")
        for segment in playlist.segments:
            if segment.gap_tag or segment.parts or segment.duration is None or not math.isfinite(segment.duration) or segment.duration <= 0:
                raise ValueError("HLS包含缺片或非完整片段")
            if urlsplit(http_url(segment.absolute_uri)).scheme != "https":
                raise ValueError("HLS仅支持HTTPS清单与资源")
            if segment.key and segment.key.method not in {"NONE", "AES-128"}:
                raise ValueError("HLS加密方式不支持")
            if segment.key and segment.key.keyformat not in {None, "", "identity"}:
                raise ValueError("HLS非普通AES密钥不支持")
            if segment.key and segment.key.method == "AES-128" and (not segment.key.uri or segment.key.iv and not re.fullmatch(r"0[xX][0-9a-fA-F]{1,32}", segment.key.iv)):
                raise ValueError("HLS普通AES密钥或IV无效")
            if segment.key and segment.key.uri:
                if urlsplit(http_url(segment.key.absolute_uri)).scheme != "https":
                    raise ValueError("HLS仅支持HTTPS清单与资源")
            if segment.init_section:
                if urlsplit(http_url(segment.init_section.absolute_uri)).scheme != "https":
                    raise ValueError("HLS仅支持HTTPS清单与资源")
            if media_get is not None and (segment.init_section or urlsplit(segment.absolute_uri).path.lower().endswith((".m4s", ".mp4"))):
                raise ValueError("黄果仅支持普通MPEG-TS有限HLS")
        # 只返回解析器规范化清单，使每个标准URI都可被父进程登记和重写。
        normalized = playlist.dumps()
        if len(normalized.encode("utf-8")) > MAX_MANIFEST:
            raise ValueError("规范媒体清单超过限制")
        return http_url(str(response.url)), normalized
    raise ValueError("HLS嵌套层级超限")


def resolve(input_data: dict[str, Any], modules: dict[str, Any]) -> dict[str, Any]:
    """只实例化固定详情解析器，拒绝上游吞掉的解析失败或权限不足。"""
    site = str(input_data.get("site", ""))
    video_id = str(input_data.get("videoId", ""))
    url = canonical_url(site, video_id)
    classes = {"jable": modules["jable"].SiteJableTV, "missav": modules["missav"].SiteMissAV, "hanime1": modules["hanime1"].SiteHanime1, "huangguo": modules["huangguo"].SiteHuangguo}
    crawler = classes[site](url, savepath=os.getcwd(), silence=True, max_workers=1, resolution="highest")
    if getattr(crawler, "_last_error", None) or not crawler.is_url_vaildate():
        raise ValueError("视频不可用或解析失败")
    direct_url = getattr(crawler, "_direct_url", None)
    media_url = http_url(str(direct_url or getattr(crawler, "_m3u8url", "")))
    headers = {key: http_url(str(value)) for key, value in dict(getattr(crawler, "_extra_headers", {}) or {}).items() if key in {"Referer", "Origin"}}
    headers.setdefault("Referer", url)
    if site == "huangguo":
        headers["Origin"] = "https://huangguoai.com"
    manifest = ""
    if not direct_url or ".m3u8" in urlsplit(media_url).path.lower():
        media_url, manifest = resolve_manifest(media_url, headers, crawler._media_get if site == "huangguo" else None)
    result = {"site": site, "videoId": video_id, "title": clean_text(crawler.target_name()), "mediaUrl": media_url, "headers": headers, "manifest": manifest}
    if site == "huangguo" and crawler._cdn:
        result["cdn"] = {"origin": "https://" + crawler._media_origin[0] + (":" + str(crawler._media_origin[1]) if crawler._media_origin[1] != 443 else ""), **crawler._cdn}
    return result


def main() -> None:
    """有界读取一次请求并隔离上游打印，标准输出只能包含固定JSON。"""
    raw = sys.stdin.buffer.read(8193)
    if len(raw) > 8192:
        raise ValueError("请求过大")
    input_data = json.loads(raw)
    if not isinstance(input_data, dict) or set(input_data) - {"action", "site", "query", "page", "videoId", "proxy"}:
        raise ValueError("请求格式无效")
    if sys.version_info < (3, 12) or importlib.metadata.version("uav-downloader") != VERSION:
        raise ValueError("固定依赖版本不符")
    provenance = json.loads(importlib.metadata.distribution("uav-downloader").read_text("direct_url.json") or "{}")
    if provenance.get("archive_info", {}).get("hashes", {}).get("sha256") != SOURCE_SHA256:
        raise ValueError("固定源码安装来源不符")
    # 将上游的打印直接丢弃，防止带签名链接、异常正文或凭据进入父进程输出。
    with open(os.devnull, "w", encoding="utf-8") as sink, contextlib.redirect_stdout(sink), contextlib.redirect_stderr(sink):
        from uav_downloader.core import config
        from uav_downloader.sites import jabletv, missav, hanime1, huangguo
        modules = {"jable": jabletv, "missav": missav, "hanime1": hanime1, "huangguo": huangguo}
        if input_data.get("action") == "probe":
            import requests, m3u8, cloudscraper, curl_cffi
            from Crypto.Cipher import AES
            output: dict[str, Any] = {"version": VERSION, "commit": COMMIT, "protocolVersion": 1}
        else:
            responses = install_transport(str(input_data.get("proxy", "")), config)
            if input_data.get("action") == "search":
                output = search(input_data, modules, responses)
            elif input_data.get("action") == "resolve":
                output = resolve(input_data, modules)
            else:
                raise ValueError("动作不允许")
    encoded = json.dumps(output, ensure_ascii=True, separators=(",", ":"))
    if len(encoded) > 1 << 20:
        raise ValueError("响应过大")
    sys.stdout.write(encoded)


if __name__ == "__main__":
    try:
        main()
    except BaseException:
        # 不向父进程暴露上游异常；父进程按固定中文错误映射处理。
        sys.exit(1)
