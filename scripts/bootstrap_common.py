"""构建侧共用的固定初始化材料处理；终端不执行这些 Python 工具。"""

from __future__ import annotations

import hashlib
import io
from pathlib import Path
import stat
import tempfile
from typing import Any
import urllib.request
import zipfile

from release import digest, relative_path
from runtime_common import download, ordinary


ARTIFACT_LIMIT = 512 * 1024 * 1024  # 单个官方归档或工具的最大下载字节数。
ASSET_NAMES = ("runtime.json", "recipe.json", "payload.zip")  # Go 嵌入的确定性资源文件名。


def fetch(source: dict[str, Any], cache: Path) -> Path:
    """按固定来源和摘要下载构建材料，已验证缓存可复用。"""
    target = cache / source["sha256"]
    if target.exists():
        ordinary(target)
        if digest(target) == source["sha256"]:
            return target
        raise ValueError("构建缓存摘要不符，请核对并移除该缓存后重试")
    # 通用下载器失败时会保留.part；所有中间文件隔离到本次自有目录，避免重试/并发互删。
    with tempfile.TemporaryDirectory(prefix="uav-source-", dir=cache) as temporary:
        staging = Path(temporary) / "artifact"
        if source["name"] != "yt-dlp":
            download(source["url"], staging, source["sha256"], ARTIFACT_LIMIT)
        else:
            # GitHub 固定 release EXE 会跳转到其资产 CDN；所有最终字节仍受固定 SHA256 约束。
            request = urllib.request.Request(source["url"], headers={"User-Agent": "Qiku-runtime-bootstrap-build/1"})
            with urllib.request.urlopen(request, timeout=120) as incoming, staging.open("xb") as outgoing:
                if not incoming.url.startswith("https://"):
                    raise ValueError("下载器构建材料不允许降级到非 HTTPS")
                total = 0
                while block := incoming.read(1024 * 1024):
                    total += len(block)
                    if total > ARTIFACT_LIMIT:
                        raise ValueError("下载器构建材料超过预算")
                    outgoing.write(block)
            if digest(staging) != source["sha256"]:
                raise ValueError("下载器构建材料摘要不符")
        staging.rename(target)
    return target


def zip_members(path: Path) -> dict[str, zipfile.ZipInfo]:
    """校验归档全部成员的路径、类型、别名和大小后返回普通文件索引。"""
    result: dict[str, zipfile.ZipInfo] = {}
    folded: set[str] = set()
    total = 0
    with zipfile.ZipFile(path) as archive:
        for member in archive.infolist():
            name = relative_path(member.filename.rstrip("/"))
            mode = member.external_attr >> 16
            if name.casefold() in folded or member.flag_bits & 1 or stat.S_IFMT(mode) not in (0, stat.S_IFREG, stat.S_IFDIR):
                raise ValueError("固定归档存在重复路径、加密或非普通成员")
            folded.add(name.casefold())
            if member.file_size > ARTIFACT_LIMIT:
                raise ValueError("固定归档成员过大")
            total += member.file_size
            if total > 1024 * 1024 * 1024:
                raise ValueError("固定归档解压总量超过预算")
            if not member.is_dir():
                result[name] = member
    return result


def record(path: str, data: bytes) -> dict[str, Any]:
    """生成额外工具或许可文件的精确字节身份，不宣称正式分发审查完成。"""
    return {"path": path, "sha256": hashlib.sha256(data).hexdigest(), "size": len(data)}


def payload_bytes(files: dict[str, bytes]) -> bytes:
    """确定性压缩已验证安装文件载荷，身份独立于原始构建 wheel。"""
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for name, data in sorted(files.items()):
            member = zipfile.ZipInfo(name, date_time=(2020, 1, 1, 0, 0, 0))
            member.create_system = 3
            member.external_attr = (stat.S_IFREG | 0o644) << 16
            member.compress_type = zipfile.ZIP_DEFLATED
            archive.writestr(member, data)
    return output.getvalue()
