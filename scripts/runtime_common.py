"""供漫画和视频运行时共用的有界材料、路径和PE校验，不执行插件。"""
from __future__ import annotations

import hashlib
import json
import stat
import urllib.request
import zipfile
from pathlib import Path
from typing import Any, BinaryIO

from release import SHA256, digest, relative_path, unique_object, validate_binary_stream

MANIFEST_LIMIT = 4 * 1024 * 1024  # 普通清单读取最多4MiB。
MAX_FILE = 512 * 1024 * 1024  # 每份材料最多512MiB。
MAX_TOTAL = 1024 * 1024 * 1024  # 解压总材料最多1GiB。


def read_manifest(path: Path) -> dict[str, Any]:
    """有界读取无重复字段的普通清单，不接受目录链接与畸形JSON。"""
    ordinary(path)
    if path.stat().st_size > MANIFEST_LIMIT:
        raise ValueError("私有插件运行时清单超过4MiB预算")
    with path.open("rb") as stream:
        data = stream.read(MANIFEST_LIMIT + 1)
    if len(data) > MANIFEST_LIMIT:
        raise ValueError("私有插件运行时清单读取期间超过预算")
    value = json.loads(data, object_pairs_hook=unique_object)
    if not isinstance(value, dict):
        raise ValueError("私有插件运行时清单必须为JSON对象")
    return value

def ordinary(path: Path) -> None:
    """拒绝文件及其父路径的符号链接、Windows重解析点和非普通文件。"""
    for candidate in (path, *path.parents):
        information = candidate.lstat()
        if stat.S_ISLNK(information.st_mode) or getattr(information, "st_file_attributes", 0) & 0x400:
            raise ValueError("私有插件运行材料路径不能包含链接或重解析点")
    if not path.is_file():
        raise ValueError("私有插件运行材料必须是普通文件")

def verify_stream(stream: BinaryIO, record: dict[str, Any], python_version: str) -> None:
    """核对一个文件的摘要、字节数、PE架构及固定隔离搜索路径。"""
    checksum = hashlib.sha256()
    total = 0
    for block in iter(lambda: stream.read(1024 * 1024), b""):
        total += len(block)
        if total > record["size"]:
            raise ValueError("私有插件运行文件实际长度超过声明")
        checksum.update(block)
    if total != record["size"] or checksum.hexdigest() != record["sha256"]:
        raise ValueError("私有插件运行文件字节数或摘要不匹配")
    suffix = Path(record["path"]).suffix.lower()
    if suffix in (".exe", ".dll", ".pyd"):
        stream.seek(0)
        validate_binary_stream(stream, "windows-amd64")
    if record["path"].endswith("._pth"):
        stream.seek(0)
        abi = "".join(python_version.split(".")[:2])
        if stream.read(1025) != f"python{abi}.zip\n.\nLib/site-packages\n".encode("ascii"):
            raise ValueError("私有Python搜索路径与固定隔离规则不一致")

def download(url: str, path: Path, expected: str, limit: int = MAX_FILE) -> None:
    """下载固定HTTPS材料并校验预先确定的摘要，拒绝改用非官方来源。"""
    if not SHA256.fullmatch(expected):
        raise ValueError("下载材料必须预先给出有效SHA256")
    if path.exists():
        ordinary(path)
        if digest(path) != expected:
            raise ValueError("已缓存运行时材料摘要不匹配，请检查后使用新目录")
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".part")
    with urllib.request.urlopen(url, timeout=60) as incoming, temporary.open("xb") as outgoing:
        if incoming.geturl() != url:
            raise ValueError("固定运行时下载发生重定向，拒绝更换来源")
        count = 0
        while block := incoming.read(1024 * 1024):
            count += len(block)
            if count > limit:
                raise ValueError("运行时下载超过材料大小预算")
            outgoing.write(block)
    if digest(temporary) != expected:
        raise ValueError("官方运行时下载摘要不匹配")
    temporary.rename(path)

def extract(archive_path: Path, destination: Path, component: str, wheel: bool = False) -> list[Path]:
    """逐成员校验并解压可信摘要归档，拒绝穿越、链接、重复路径和异常扩张。"""
    ordinary(archive_path)
    extracted: list[Path] = []
    seen: set[str] = set()
    total = 0
    with zipfile.ZipFile(archive_path) as archive:
        for information in archive.infolist():
            raw = information.filename.rstrip("/")
            name = relative_path(raw)
            mode = information.external_attr >> 16
            if information.flag_bits & 1 or (mode & 0o170000) not in (0, stat.S_IFREG, stat.S_IFDIR) or name.casefold() in seen:
                raise ValueError("源运行材料包含重复、链接或加密成员")
            seen.add(name.casefold())
            if information.is_dir():
                continue
            if information.file_size > MAX_FILE:
                raise ValueError("源运行材料单文件过大")
            total += information.file_size
            if total > MAX_TOTAL:
                raise ValueError("源运行材料解压超过预算")
            if wheel and ".data/" in name:
                prefix, _, tail = name.partition(".data/")
                category, separator, local = tail.partition("/")
                if not prefix or not separator or category not in ("purelib", "platlib"):
                    raise ValueError("固定wheel包含未经支持的安装重定位材料")
                name = relative_path(local)
            if name.lower().endswith((".pth", ".pyc")):
                raise ValueError("固定wheel夹带额外site配置或缓存")
            path = destination / name
            path.parent.mkdir(parents=True, exist_ok=True)
            with archive.open(information) as incoming, path.open("xb") as outgoing:
                copied = 0
                while block := incoming.read(1024 * 1024):
                    copied += len(block)
                    if copied > information.file_size:
                        raise ValueError("源运行材料实际扩张超过声明")
                    outgoing.write(block)
                if copied != information.file_size:
                    raise ValueError("源运行材料截断")
            path.chmod(0o644)
            extracted.append(path)
    if not extracted:
        raise ValueError(f"运行组件{component}没有可分发材料")
    return extracted
