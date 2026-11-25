"""通过固定 JSON 协议调用受控 JMComic，不加载外部脚本、YAML 或账号。"""

from __future__ import annotations

import contextlib
import hashlib
import importlib.metadata
import json
import logging
import os
from pathlib import Path
import re
import stat
import sys
import time
import unicodedata
from typing import Any
from urllib.parse import urlsplit
import warnings


VERSION: str = "2.7.12"  # 唯一通过固定适配协议核验的上游版本。
MAX_FILES: int = 10000  # 执行端允许的绝对图片数上限。
MAX_PIXELS: int = 40000000  # 单张图片的最大像素数，限制异常解码开销。
ATOMIC_RETRIES: int = 20  # Windows短暂只读句柄竞争时的原子替换重试次数。
ATOMIC_RETRY_SECONDS: float = 0.01  # 每次原子替换竞争等待10毫秒，最多等待200毫秒。
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


def album_data(album: Any, client: Any, selection: list[str]) -> dict[str, Any]:
    """读取真实章节页数并以全本顺序规范化显式选择。"""
    chapters: list[str] = [album_id(str(photo.photo_id)) for photo in album]
    if not chapters or len(chapters) > MAX_FILES or len(set(chapters)) != len(chapters):
        raise ValueError("invalid chapters")
    if len(set(selection)) != len(selection) or any(aid not in chapters for aid in selection):
        raise ValueError("invalid selection")
    details: list[dict[str, Any]] = []
    for photo in album:
        client.check_photo(photo)
        details.append({"id": album_id(str(photo.photo_id)), "title": text(photo.name, 160),
                        "pageCount": integer(len(photo), 1, MAX_FILES)})
    pages: int = integer(album.page_count, 1, MAX_FILES)
    if sum(chapter["pageCount"] for chapter in details) != pages:
        raise ValueError("inconsistent chapter pages")
    return {
        "albumId": album_id(str(album.album_id)),
        "title": text(album.name, 160),
        "pageCount": pages,
        "chapters": chapters,
        "files": 0,
        "chapterDetails": details,
        "selectedChapterIds": [aid for aid in chapters if not selection or aid in selection],
    }


def digest(value: Any) -> str:
    """把受控身份或图片语义转换为不泄露来源地址的固定摘要。"""
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def safe_directory(path: Path) -> None:
    """拒绝任何链接、目录别名或非普通目录。"""
    if not path.is_absolute() or path.is_symlink() or path.resolve(strict=True) != path or not path.is_dir():
        raise ValueError("invalid private directory")


def atomic_json(path: Path, value: Any) -> None:
    """仅在受控私密目录原子替换检查点，临时文件永不进入暂存产物。"""
    safe_directory(path.parent)
    temporary: Path = path.with_suffix(".tmp")
    if temporary.is_symlink() or (temporary.exists() and (not temporary.is_file() or temporary.stat().st_nlink != 1)):
        raise ValueError("invalid checkpoint temporary")
    descriptor: int = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as output:
        json.dump(value, output, separators=(",", ":"))
        output.flush()
        os.fsync(output.fileno())
    # Windows读取进度的短暂句柄可能阻止替换；只对权限竞争有限等待，始终保留旧完整快照。
    for attempt in range(ATOMIC_RETRIES):
        try:
            os.replace(temporary, path)
            return
        except PermissionError:
            if attempt + 1 == ATOMIC_RETRIES:
                raise
            time.sleep(ATOMIC_RETRY_SECONDS)


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
    selection: list[str] = []  # 空选择保留旧协议的全本下载语义。
    checkpoint: Path | None = None  # 检查点只能位于执行端私密目录。
    binding: str = ""  # 绑定本操作请求与预览的不可逆摘要。
    if action in ("album", "download"):
        requested_id = album_id(request.get("albumId"))
        raw_selection: object = request.get("chapterIds", [])
        if not isinstance(raw_selection, list) or len(raw_selection) > MAX_FILES:
            raise ValueError("invalid selection")
        selection = [album_id(value) for value in raw_selection]
        if len(set(selection)) != len(selection):
            raise ValueError("duplicate chapter")
    if action == "download":
        maximum = integer(request.get("maxFiles"), 1, MAX_FILES)
        max_bytes = integer(request.get("maxBytes"), 1, 1 << 40)
        directory: object = request.get("outputDir")
        if not isinstance(directory, str) or not Path(directory).is_absolute():
            raise ValueError("invalid output")
        base = Path(directory)
        safe_directory(base)
        if base != Path.cwd().resolve():
            raise ValueError("invalid staging")
        if "checkpointDir" in request:
            checkpoint = Path(str(request["checkpointDir"]))
            safe_directory(checkpoint)
            if checkpoint == base or checkpoint.is_relative_to(base) or base.is_relative_to(checkpoint):
                raise ValueError("checkpoint intersects staging")
            binding = str(request.get("binding", ""))
            if re.fullmatch(r"[a-f0-9]{64}", binding) is None:
                raise ValueError("invalid binding")
        elif any(base.iterdir()):
            raise ValueError("resume without checkpoint")

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
            "cache": False,  # 仅由下载钩子完成摘要和真实图片复核后逐页启用复用。
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
        if not {"action", "query", "page", "proxy"} <= set(request) or set(request) - {"action", "query", "page", "proxy", "orderBy", "time"}:
            raise ValueError("invalid search")
        query: object = request.get("query")
        page: int = integer(request.get("page"), 1, 1000)
        if (not isinstance(query, str) or not 1 <= len(query) <= 120
                or query != query.strip() or any(unicodedata.category(ch) == "Cc" for ch in query)):
            raise ValueError("invalid query")
        order: object = request.get("orderBy", "mr")
        period: object = request.get("time", "a")
        if order not in ("mr", "mv", "mp", "tf") or period not in ("a", "t", "w", "m"):
            raise ValueError("invalid search filter")
        result: Any = option.new_jm_client().search_site(query, page=page, order_by=order, time=period)
        items: list[dict[str, Any]] = []
        for aid, title, tags in result.iter_id_title_tag():
            if len(items) == 100:
                raise ValueError("too many results")
            items.append({"albumId": album_id(str(aid)), "title": text(title, 160),
                          "tags": [text(tag, 160) for tag in list(tags)[:20]]})
        return {"query": query, "page": page, "total": integer(result.total, 0, 100000000),
                "hasNext": page < integer(result.page_count, 0, 100000000), "items": items}
    if action == "album":
        if not {"action", "albumId", "proxy"} <= set(request) or set(request) - {"action", "albumId", "proxy", "chapterIds"}:
            raise ValueError("invalid album request")
        client: Any = option.new_jm_client()
        return album_data(client.get_album_detail(requested_id), client, selection)
    required: set[str] = {"action", "albumId", "outputDir", "maxBytes", "maxFiles", "proxy"}
    if action != "download" or not required <= set(request) or set(request) - required - {"chapterIds", "checkpointDir", "binding"}:
        raise ValueError("invalid action")
    if ("checkpointDir" in request) != ("binding" in request):
        raise ValueError("incomplete checkpoint request")

    class FixedDownloader(jmcomic.JmDownloader):
        """逐章逐页核验预算和解码结果，拒绝任何部分成功产物。"""

        def __init__(self, fixed_option: FixedOption) -> None:
            """为一次完整漫画下载初始化独立计数与客户端。"""
            self.files: int = 0  # 真实完成并验证的图片数。
            self.bytes: int = 0  # 已验证图片的累计字节数。
            self.photo_pages: int = 0  # 章节元数据实际声明的页数之和。
            self.paths: set[str] = set()  # 防止图片顺序或路径重复覆盖。
            self.metadata: dict[str, Any] = {}  # 下载前绑定的完整漫画身份。
            self.expected: dict[str, str] = {}  # 每个固定输出路径对应的来源语义摘要。
            self.receipts: dict[str, Any] = {}  # 仅保存真实验证通过的页面凭据。
            self.completed: set[str] = set()  # 本次已验证完成的相对页面集合。
            self.total: int = 0  # 本次冻结选择要求的完整页数。
            self.reused: int = 0  # 本次复用且重新验证的页面数。
            self.metadata_hash: str = ""  # 全本与选章元信息绑定摘要。
            super().__init__(fixed_option)

        def before_album(self, album: Any) -> None:
            """在传输图片之前验证完整漫画身份和页数预算。"""
            self.metadata = album_data(album, self.client, selection)
            if self.metadata["albumId"] != requested_id:
                raise ValueError("album alias forbidden")
            option.chapter_order = {aid: i + 1 for i, aid in enumerate(self.metadata["chapters"])}
            selected: list[str] = self.metadata["selectedChapterIds"]
            self.total = sum(chapter["pageCount"] for chapter in self.metadata["chapterDetails"] if chapter["id"] in selected)
            if self.total > maximum:
                raise ValueError("selected page budget exceeded")
            # 预先绑定每页来源语义；文件名存在从来不是可信缓存的依据。
            for photo in album:
                if str(photo.photo_id) not in selected:
                    continue
                self.client.check_photo(photo)
                for picture in photo:
                    relative: str = f"{option.chapter_order[str(photo.photo_id)]:05d}/{picture.index:05d}" + (".gif" if picture.is_gif else ".jpg")
                    if relative in self.expected:
                        raise ValueError("duplicate expected page")
                    self.expected[relative] = digest([str(photo.photo_id), picture.index, str(picture.scramble_id), picture.img_url, picture.query_params])
            if len(self.expected) != self.total:
                raise ValueError("invalid expected pages")
            identity: dict[str, Any] = {key: value for key, value in self.metadata.items() if key != "title"}
            identity["chapterDetails"] = [{"id": chapter["id"], "pageCount": chapter["pageCount"]} for chapter in self.metadata["chapterDetails"]]
            self.metadata_hash = digest([identity, self.expected])
            if checkpoint is not None:
                receipt_path: Path = checkpoint / "checkpoint.json"
                if receipt_path.exists() or receipt_path.is_symlink():
                    if receipt_path.is_symlink() or not receipt_path.is_file() or receipt_path.stat().st_nlink != 1 or receipt_path.stat().st_size > 4 * 1024 * 1024:
                        raise ValueError("invalid checkpoint")
                    saved: Any = json.loads(receipt_path.read_text(encoding="utf-8"))
                    if (not isinstance(saved, dict) or set(saved) != {"binding", "metadata", "pages"}
                            or saved["binding"] != binding or saved["metadata"] != self.metadata_hash
                            or not isinstance(saved["pages"], dict) or len(saved["pages"]) > self.total):
                        raise ValueError("checkpoint identity drift")
                    self.receipts = saved["pages"]
                    if any(key not in self.expected for key in self.receipts):
                        raise ValueError("unexpected checkpoint page")
            for path in base.rglob("*"):
                if path.is_symlink() or path.resolve() != path:
                    raise ValueError("linked staging entry")
                relative = path.relative_to(base).as_posix()
                if path.is_dir():
                    if not any(key.startswith(relative + "/") for key in self.expected):
                        raise ValueError("unexpected chapter directory")
                elif not path.is_file() or path.stat().st_nlink != 1 or relative not in self.expected:
                    raise ValueError("unexpected staging file")
            self.save_progress()
            super().before_album(album)

        def do_filter(self, detail: Any) -> Any:
            """仅过滤章节集合，保留选中章节的全部图片。"""
            if detail.is_album():
                return [photo for photo in detail if str(photo.photo_id) in self.metadata["selectedChapterIds"]]
            return detail

        def before_photo(self, photo: Any) -> None:
            """在每章开始前检查实际页数，防止隐含超量下载。"""
            self.photo_pages += integer(len(photo), 1, maximum)
            expected_count: int = next(chapter["pageCount"] for chapter in self.metadata["chapterDetails"] if chapter["id"] == str(photo.photo_id))
            if str(photo.photo_id) not in self.metadata["selectedChapterIds"] or len(photo) != expected_count or self.photo_pages > self.total:
                raise ValueError("page budget exceeded")
            super().before_photo(photo)

        def before_image(self, image: Any, img_save_path: str) -> None:
            """保证每页只写入一次固定根内路径。"""
            if img_save_path in self.paths or not Path(img_save_path).is_relative_to(base):
                raise ValueError("duplicate image")
            self.paths.add(img_save_path)
            target: Path = Path(img_save_path)
            relative: str = target.relative_to(base).as_posix()
            if relative not in self.expected:
                raise ValueError("unselected image")
            image.cache = False
            if target.exists() or target.is_symlink():
                # 先检查链接与真实文件，再允许上游跳过传输；损坏普通文件重新取得。
                receipt: Any = self.receipts.get(relative)
                info: os.stat_result = target.lstat()
                if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or target.resolve() != target:
                    raise ValueError("invalid cache file")
                try:
                    actual: dict[str, Any] = self.inspect_image(target)
                    trusted: bool = isinstance(receipt, dict) and receipt == {"source": self.expected[relative], **actual}
                except Exception:
                    trusted = False
                if trusted:
                    image.cache = True
                    image.exists = True
                    self.reused += 1
                else:
                    target.unlink()
                    self.receipts.pop(relative, None)
                    image.exists = False
            super().before_image(image, img_save_path)

        def after_image(self, image: Any, img_save_path: str) -> None:
            """逐页验证实际图片与磁盘预算，通过后才记录下载成功。"""
            target: Path = Path(img_save_path)
            actual: dict[str, Any] = self.inspect_image(target)
            relative: str = target.relative_to(base).as_posix()
            if relative in self.completed:
                raise ValueError("duplicate completion")
            self.files += 1
            self.bytes += actual["size"]
            if self.files > self.total or self.bytes > max_bytes:
                raise ValueError("download budget exceeded")
            self.completed.add(relative)
            self.receipts[relative] = {"source": self.expected[relative], **actual}
            self.save_progress()
            super().after_image(image, img_save_path)

        def inspect_image(self, target: Path) -> dict[str, Any]:
            """独立核验普通文件、真实栅格及内容摘要，供下载和复用共用。"""
            info: os.stat_result = target.lstat()
            if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or target.resolve() != target or not 1 <= info.st_size <= max_bytes:
                raise ValueError("invalid image file")
            with Image.open(target) as picture:
                if (picture.format not in ("JPEG", "GIF") or picture.width < 1 or picture.height < 1
                        or picture.width * picture.height > MAX_PIXELS):
                    raise ValueError("invalid raster image")
                picture.verify()
            content: Any = hashlib.sha256()
            with target.open("rb") as source:
                while chunk := source.read(1024 * 1024):
                    content.update(chunk)
            content_hash: str = content.hexdigest()
            return {"size": info.st_size, "sha256": content_hash}

        def save_progress(self) -> None:
            """在私密根保存可信页面凭据与无敏感信息的进度快照。"""
            if checkpoint is not None:
                atomic_json(checkpoint / "checkpoint.json", {"binding": binding, "metadata": self.metadata_hash, "pages": self.receipts})
                atomic_json(checkpoint / "progress.json", {"completedPages": self.files, "totalPages": self.total,
                            "bytes": self.bytes, "reusedPages": self.reused})

    result: Any = jmcomic.download_album(requested_id, option, FixedDownloader,
                                        check_exception=True, login=False)
    album, downloader = result
    metadata: dict[str, Any] = album_data(album, downloader.client, selection)
    successful: dict[Any, Any] = downloader.download_success_dict.get(album, {})
    if (downloader.download_failed_image or downloader.download_failed_photo
            or {str(photo.photo_id) for photo in successful} != set(metadata["selectedChapterIds"])
            or any(len(images) != len(photo) for photo, images in successful.items())
            or downloader.completed != set(downloader.expected)
            or downloader.files != downloader.total or downloader.photo_pages != downloader.total
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
