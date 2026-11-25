"""通过固定 JSON 协议调用受控 JMComic，不加载外部脚本、YAML 或账号。"""

from __future__ import annotations

import contextlib
import importlib.metadata
import json
import logging
import os
from pathlib import Path
import re
import stat
import sys
import unicodedata
from typing import Any
from urllib.parse import urlsplit
import warnings


VERSION: str = "2.7.12"  # 唯一通过固定适配协议核验的上游版本。
MAX_FILES: int = 10000  # 执行端允许的绝对图片数上限。
MAX_PIXELS: int = 40000000  # 单张图片的最大像素数，限制异常解码开销。
ALBUM_ID: re.Pattern[str] = re.compile(r"[1-9][0-9]{0,11}\Z")  # 严格正整数编号。


def album_id(value: object) -> str:
    """只接受没有前导零、地址或命令语义的漫画编号。"""
    if not isinstance(value, str) or ALBUM_ID.fullmatch(value) is None:
        raise ValueError("invalid album")
    return value


def integer(value: object, minimum: int, maximum: int) -> int:
    """拒绝布尔值及超出明确业务预算的整数。"""
    if type(value) is not int or not minimum <= value <= maximum:
        raise ValueError("invalid number")
    return value


def text(value: object, maximum: int) -> str:
    """把上游展示元信息收敛为有界且没有控制字符的纯文本。"""
    return "".join(ch for ch in str(value) if ch.isprintable()).strip()[:maximum]


def album_data(album: Any, maximum: int) -> dict[str, Any]:
    """提取规范漫画身份和按上游语义排序的唯一章节清单。"""
    chapters: list[str] = [album_id(str(photo.photo_id)) for photo in album]
    if not chapters or len(chapters) > maximum or len(set(chapters)) != len(chapters):
        raise ValueError("invalid chapters")
    return {
        "albumId": album_id(str(album.album_id)),
        "title": text(album.name, 160),
        "pageCount": integer(album.page_count, 1, maximum),
        "chapters": chapters,
        "files": 0,
    }


def proxy_address(value: object) -> str:
    """只接受执行端生成的带认证本机 HTTP 代理。"""
    if not isinstance(value, str) or len(value) > 2048:
        raise ValueError("invalid proxy")
    parsed = urlsplit(value)
    if (parsed.scheme != "http" or parsed.hostname != "127.0.0.1"
            or not parsed.username or not parsed.password or not parsed.port
            or parsed.path or parsed.query or parsed.fragment):
        raise ValueError("invalid proxy")
    return value


def run(request: dict[str, Any]) -> dict[str, Any]:
    """用固定选项调度探测、搜索、预览或完整漫画下载。"""
    # 导入和上游日志均被外层重定向，只有最后的协议 JSON 可以写入标准输出。
    import jmcomic
    from PIL import Image

    if importlib.metadata.version("jmcomic") != VERSION:
        raise RuntimeError("unsupported dependency")
    jmcomic.disable_jm_log()
    Image.MAX_IMAGE_PIXELS = MAX_PIXELS
    warnings.simplefilter("error", Image.DecompressionBombWarning)
    action: object = request.get("action")
    if action == "probe":
        if set(request) != {"action"}:
            raise ValueError("invalid probe")
        return {"version": VERSION, "protocolVersion": 1}

    proxy: str = proxy_address(request.get("proxy"))
    # 初始化域名发现与实际 API、图床请求都显式绑定同一受控代理。
    proxies: dict[str, str] = {"http": proxy, "https": proxy}
    jmcomic.JmModuleConfig.DEFAULT_PROXIES = proxies
    base: Path = Path.cwd()
    maximum: int = MAX_FILES
    max_bytes: int = 0
    requested_id: str = ""
    if action in ("album", "download"):
        requested_id = album_id(request.get("albumId"))
    if action == "download":
        maximum = integer(request.get("maxFiles"), 1, MAX_FILES)
        max_bytes = integer(request.get("maxBytes"), 1, 1 << 40)
        directory: object = request.get("outputDir")
        if not isinstance(directory, str) or not Path(directory).is_absolute():
            raise ValueError("invalid output")
        base = Path(directory).resolve(strict=True)
        if not base.is_dir() or base != Path.cwd().resolve() or any(base.iterdir()):
            raise ValueError("invalid staging")

    class FixedOption(jmcomic.JmOption):
        """固定数字目录和图片名，任何上游标题均不参与文件系统路径。"""

        chapter_order: dict[str, int] = {}  # 当前完整漫画的章节编号与连续顺序。

        def decide_image_save_dir(self, photo: Any, ensure_exists: bool = True) -> str:
            """把章节编号映射为本操作根目录内的固定数字子目录。"""
            index: int = self.chapter_order[album_id(str(photo.photo_id))]
            target: Path = base / f"{index:05d}"
            if ensure_exists:
                target.mkdir(mode=0o700, exist_ok=True)
            if target.is_symlink() or target.resolve() != target:
                raise ValueError("invalid chapter path")
            return str(target)

        def decide_image_filename(self, image: Any) -> str:
            """使用章节内真实页序号生成稳定文件名。"""
            return f"{integer(image.index, 1, maximum):05d}"

        def decide_image_suffix(self, image: Any) -> str:
            """保留动图，其余图片解码后统一保存为可独立核验的 JPEG。"""
            return ".gif" if image.is_gif else ".jpg"

    option: FixedOption = FixedOption.construct({
        "log": False,  # 关闭可能包含来源地址、关键词和标题的上游日志。
        "auth": {},  # 不提供账号、Cookie 或环境变量认证配置。
        "dir_rule": {"rule": "Bd", "base_dir": str(base)},  # 固定独占暂存根。
        "download": {
            "cache": False,  # 每次操作必须重新取得全部图片。
            "threading": {"image": 1, "photo": 1},  # 单漫画单图片有界并发。
            "image": {"decode": True, "suffix": ".jpg"},  # 解码原始页并固定图片格式。
        },
        "client": {
            "impl": "api",  # 仅使用核验过的 API 客户端。
            "cache": False,  # 不把用户查询写入跨任务缓存。
            "retry_times": 1,  # 有限重试，生命周期另由执行端取消控制。
            "postman": {
                "type": "curl_cffi",  # 仅允许锁定依赖中的固定 HTTP 实现。
                "meta_data": {"proxies": proxies, "timeout": 20},  # 请求预算与受控代理。
            },
        },
        "plugins": {"dependencies_strategy": "failed-fast"},  # 禁止运行时自动安装依赖。
    })
    if action == "search":
        if set(request) != {"action", "query", "page", "proxy"}:
            raise ValueError("invalid search")
        query: object = request.get("query")
        page: int = integer(request.get("page"), 1, 1000)
        if (not isinstance(query, str) or not 1 <= len(query) <= 120
                or query != query.strip() or any(unicodedata.category(ch) == "Cc" for ch in query)):
            raise ValueError("invalid query")
        result: Any = option.new_jm_client().search_site(query, page=page)
        items: list[dict[str, Any]] = []
        for aid, title, tags in result.iter_id_title_tag():
            if len(items) == 100:
                raise ValueError("too many results")
            items.append({"albumId": album_id(str(aid)), "title": text(title, 160),
                          "tags": [text(tag, 160) for tag in list(tags)[:20]]})
        return {"query": query, "page": page, "total": integer(result.total, 0, 100000000),
                "hasNext": page < integer(result.page_count, 0, 100000000), "items": items}
    if action == "album":
        if set(request) != {"action", "albumId", "proxy"}:
            raise ValueError("invalid album request")
        return album_data(option.new_jm_client().get_album_detail(requested_id), maximum)
    if action != "download" or set(request) != {
            "action", "albumId", "outputDir", "maxBytes", "maxFiles", "proxy"}:
        raise ValueError("invalid action")

    class FixedDownloader(jmcomic.JmDownloader):
        """逐章逐页核验预算和解码结果，拒绝任何部分成功产物。"""

        def __init__(self, fixed_option: FixedOption) -> None:
            """为一次完整漫画下载初始化独立计数与客户端。"""
            self.files: int = 0  # 真实完成并验证的图片数。
            self.bytes: int = 0  # 已验证图片的累计字节数。
            self.photo_pages: int = 0  # 章节元数据实际声明的页数之和。
            self.paths: set[str] = set()  # 防止图片顺序或路径重复覆盖。
            self.metadata: dict[str, Any] = {}  # 下载前绑定的完整漫画身份。
            super().__init__(fixed_option)

        def before_album(self, album: Any) -> None:
            """在传输图片之前验证完整漫画身份和页数预算。"""
            self.metadata = album_data(album, maximum)
            if self.metadata["albumId"] != requested_id:
                raise ValueError("album alias forbidden")
            option.chapter_order = {aid: i + 1 for i, aid in enumerate(self.metadata["chapters"])}
            super().before_album(album)

        def before_photo(self, photo: Any) -> None:
            """在每章开始前检查实际页数，防止隐含超量下载。"""
            self.photo_pages += integer(len(photo), 1, maximum)
            if self.photo_pages > self.metadata["pageCount"]:
                raise ValueError("page budget exceeded")
            super().before_photo(photo)

        def before_image(self, image: Any, img_save_path: str) -> None:
            """保证每页只写入一次固定根内路径。"""
            if img_save_path in self.paths or not Path(img_save_path).is_relative_to(base):
                raise ValueError("duplicate image")
            self.paths.add(img_save_path)
            super().before_image(image, img_save_path)

        def after_image(self, image: Any, img_save_path: str) -> None:
            """逐页验证实际图片与磁盘预算，通过后才记录下载成功。"""
            target: Path = Path(img_save_path)
            info: os.stat_result = target.lstat()
            if not stat.S_ISREG(info.st_mode) or target.resolve() != target or info.st_size < 1:
                raise ValueError("invalid image file")
            with Image.open(target) as picture:
                if (picture.format not in ("JPEG", "GIF") or picture.width < 1 or picture.height < 1
                        or picture.width * picture.height > MAX_PIXELS):
                    raise ValueError("invalid raster image")
                picture.verify()
            self.files += 1
            self.bytes += info.st_size
            if self.files > maximum or self.bytes > max_bytes:
                raise ValueError("download budget exceeded")
            super().after_image(image, img_save_path)

    result: Any = jmcomic.download_album(requested_id, option, FixedDownloader,
                                        check_exception=True, login=False)
    album, downloader = result
    metadata: dict[str, Any] = album_data(album, maximum)
    if (not downloader.all_success or downloader.download_failed_image or downloader.download_failed_photo
            or downloader.files != metadata["pageCount"] or downloader.photo_pages != metadata["pageCount"]
            or downloader.metadata != metadata):
        raise ValueError("incomplete download")
    metadata["files"] = downloader.files
    return metadata


def main() -> int:
    """保留唯一有界 JSON 输出，并将所有错误压缩为不含敏感来源的类别。"""
    output: Any = sys.stdout
    error_output: Any = sys.stderr
    logging.disable(logging.CRITICAL)
    try:
        raw: bytes = sys.stdin.buffer.read(16385)
        if len(raw) > 16384:
            raise ValueError("request too large")
        request: object = json.loads(raw)
        if not isinstance(request, dict):
            raise ValueError("invalid request")
        # 使用丢弃流，避免上游调试输出在内存中无限积累。
        with open(os.devnull, "w", encoding="utf-8") as sink:
            with contextlib.redirect_stdout(sink), contextlib.redirect_stderr(sink):
                response: dict[str, Any] = run(request)
        encoded: str = json.dumps(response, ensure_ascii=True, separators=(",", ":"))
        if len(encoded) > 262144:
            raise ValueError("response too large")
        output.write(encoded)
        output.flush()
        return 0
    except Exception:
        error_output.write("漫画插件调用未完成，请检查固定依赖、预算与来源网络。\n")
        error_output.flush()
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
