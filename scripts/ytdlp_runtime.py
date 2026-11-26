"""离线装配固定Windows下载器及许可/对应源码材料，不联网安装或升级工具。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True

import argparse
import hashlib
import json
import os
import re
import shutil
import stat
import subprocess
import tempfile
from pathlib import Path
from typing import Any, BinaryIO

from project_env import runtime_directory
from release import SHA256, configure_console, copy_verified, digest, relative_path, unique_object, validate_binary_stream


VERSION = "2026.10.26"  # 与现有collection下载器锁一致的固定版本，不自动追踪最新版。
SOURCE_URL = f"https://github.com/yt-dlp/yt-dlp/releases/download/{VERSION}/yt-dlp.exe"  # 官方固定tag的Windows x64产物。
EXE_SHA256 = "f5389d3698ecde4ef06f4e6b1ce0294fe9ac1396228c8990b8d3416d6d177a7d"  # 固定tag官方SHA2-256SUMS；未宣称验证GPG签名。
SOURCE_SHA256 = "21f942d258d25cc4e60b4982a25c423460f0bebb835f38e6dfd1fcb140087f95"  # 同一tag官方yt-dlp.tar.gz摘要，不是完整第三方对应源码的替代。
THIRD_PARTY_SHA256 = "472aefe951c7db35e1657c1d13fd337140511ed6f2b329205105ad441c5a02b7"  # 同一tag官方完整第三方许可聚合文件摘要。
UNLICENSE_SHA256 = "7e12e5df4bae12cb21581ba157ced20e1986a0508dd10d0e8a4ab9a4cf94e85c"  # 同一tag源码LICENSE摘要，不能单独代表EXE的组合许可。
LICENSE_ROOT = "licenses/yt-dlp"  # 下载器独立许可、审核与对应源码材料根。
MANIFEST_NAME = f"{LICENSE_ROOT}/yt-dlp-manifest.json"  # 发布包中固定下载器清单路径。
EXECUTABLE = "tools/yt-dlp.exe"  # 唯一允许运行的固定包内下载器路径。
INVENTORY = f"{LICENSE_ROOT}/source-inventory.json"  # 维护者审核的组件与源码/许可引用闭包。
GROUPS = ("runtimeFiles", "licenseFiles", "sourceFiles")  # 运行材料、许可材料和对应源码材料分别核验。
MANIFEST_LIMIT = 1024 * 1024  # 元数据最多1MiB，避免无界读取。
MAX_FILE = 2 * 1024 * 1024 * 1024  # 单份源码材料最多2GiB，按流复制和校验。
MAX_TOTAL = 4 * 1024 * 1024 * 1024  # 全部下载器分发材料最多4GiB。


def ordinary(path: Path) -> None:
    """检查文件和全部祖先，拒绝链接、Windows重解析点及非普通文件。"""
    for candidate in (path, *path.parents):
        info = candidate.lstat()
        if stat.S_ISLNK(info.st_mode) or getattr(info, "st_file_attributes", 0) & 0x400:
            raise ValueError("下载器材料路径不能包含链接或重解析点")
    if not path.is_file():
        raise ValueError("下载器材料必须是普通文件")


def read_manifest(path: Path) -> dict[str, Any]:
    """读取有限且无重复字段的下载器元数据。"""
    ordinary(path)
    with path.open("rb") as stream:
        data = stream.read(MANIFEST_LIMIT + 1)
    if len(data) > MANIFEST_LIMIT:
        raise ValueError("下载器元数据超过1MiB预算")
    value = json.loads(data, object_pairs_hook=unique_object)
    if not isinstance(value, dict):
        raise ValueError("下载器元数据必须为JSON对象")
    return value


def validate_manifest(value: dict[str, Any], packaged: bool = True) -> dict[str, dict[str, Any]]:
    """固定EXE身份和材料位置；维护者审核声明不能替代人工许可完整性判断。"""
    fields = {"schemaVersion", "target", "version", "sourceUrl", "correspondingSourceReviewed", *GROUPS}
    if set(value) != fields or type(value["schemaVersion"]) is not int or value["schemaVersion"] != 1 or value["target"] != "windows-amd64" or value["version"] != VERSION or value["sourceUrl"] != SOURCE_URL or value["correspondingSourceReviewed"] is not True:
        raise ValueError("下载器字段、固定版本/来源、平台或维护者对应源码审核声明无效")
    records: dict[str, dict[str, Any]] = {}
    folded: set[str] = {MANIFEST_NAME.casefold()}
    ancestors: dict[str, str] = {}
    total = 0
    for group in GROUPS:
        items = value[group]
        if not isinstance(items, list) or not 1 <= len(items) <= 512:
            raise ValueError("下载器各组材料必须非空且不超过512份")
        for record in items:
            if not isinstance(record, dict) or set(record) != {"source", "destination", "sha256", "size"}:
                raise ValueError("下载器文件必须声明source、destination、sha256和size")
            name = relative_path(record["destination"])
            for index in range(1, len(name.split("/"))):
                parent = "/".join(name.split("/")[:index])
                if ancestors.setdefault(parent.casefold(), parent) != parent or parent.casefold() in folded:
                    raise ValueError("下载器材料包含大小写目录别名或文件/目录冲突")
            if name.casefold() in ancestors:
                raise ValueError("下载器材料文件覆盖了既有目录")
            if name.casefold() in folded or not isinstance(record["source"], str) or not record["source"] or not isinstance(record["sha256"], str) or not SHA256.fullmatch(record["sha256"]) or type(record["size"]) is not int or not 1 <= record["size"] <= MAX_FILE:
                raise ValueError("下载器文件路径重复、来源/摘要/大小无效")
            if packaged and record["source"] != name:
                raise ValueError("下载器成品清单含构建机路径或来源不匹配")
            if group == "runtimeFiles":
                if name != EXECUTABLE or record["sha256"] != EXE_SHA256 or record["size"] > 512 * 1024 * 1024:
                    raise ValueError("下载器只能使用固定官方Windows x64 EXE及摘要")
            elif not name.startswith(LICENSE_ROOT + "/") or name.casefold().endswith((".exe", ".dll", ".pyd")):
                raise ValueError("下载器许可/源码材料位置越界或包含可执行文件")
            total += record["size"]
            folded.add(name.casefold())
            records[name] = record
    licenses = {record["destination"]: record for record in value["licenseFiles"]}
    sources = {record["destination"]: record for record in value["sourceFiles"]}
    required = {f"{LICENSE_ROOT}/LICENSE": UNLICENSE_SHA256, f"{LICENSE_ROOT}/THIRD_PARTY_LICENSES.txt": THIRD_PARTY_SHA256}
    if total > MAX_TOTAL or set(record["destination"] for record in value["runtimeFiles"]) != {EXECUTABLE} or any(licenses.get(name, {}).get("sha256") != checksum for name, checksum in required.items()) or f"{LICENSE_ROOT}/COPYING-GPL-3.0.txt" not in licenses or INVENTORY not in sources or sources.get(f"{LICENSE_ROOT}/source/yt-dlp.tar.gz", {}).get("sha256") != SOURCE_SHA256 or len(sources) < 3:
        raise ValueError("下载器缺少固定官方许可、GPL文本、审核库存或额外对应源码材料，或超过预算")
    if records[INVENTORY]["size"] > MANIFEST_LIMIT:
        raise ValueError("下载器源码库存超过元数据预算")
    return records


def validate_inventory(value: dict[str, Any], manifest: dict[str, Any]) -> None:
    """核对维护者审核的组件闭包及包内引用，不能由工具自动声明法律合规。"""
    if set(value) != {"schemaVersion", "version", "executableSha256", "complete", "components"} or type(value["schemaVersion"]) is not int or value["schemaVersion"] != 1 or value["version"] != VERSION or value["executableSha256"] != EXE_SHA256 or value["complete"] is not True or not isinstance(value["components"], list) or not 3 <= len(value["components"]) <= 256:
        raise ValueError("下载器对应源码库存缺少明确的固定产物审核身份")
    licenses = {record["destination"] for record in manifest["licenseFiles"]}
    sources = {record["destination"] for record in manifest["sourceFiles"]} - {INVENTORY}
    names: set[str] = set()
    used_sources: set[str] = set()
    used_licenses: set[str] = set()
    for component in value["components"]:
        if not isinstance(component, dict) or set(component) != {"name", "version", "licenses", "sources"} or not isinstance(component["name"], str) or not re.fullmatch(r"[a-z0-9][a-z0-9.-]{0,79}", component["name"]) or component["name"] in names or not isinstance(component["version"], str) or not component["version"].strip() or len(component["version"]) > 100:
            raise ValueError("下载器源码库存组件身份重复或无效")
        for key, allowed, used in (("licenses", licenses, used_licenses), ("sources", sources, used_sources)):
            references = component[key]
            if not isinstance(references, list) or not references or not all(isinstance(item, str) for item in references) or len(references) != len(set(references)) or not set(references).issubset(allowed):
                raise ValueError("下载器组件许可或源码引用缺失、重复或越界")
            used.update(references)
        names.add(component["name"])
    if not {"yt-dlp", "python", "pyinstaller"}.issubset(names) or used_sources != sources or used_licenses != licenses:
        raise ValueError("下载器对应源码/许可库存闭包不完整；仍需维护者核对实际完整组件")


def verify_stream(stream: BinaryIO, record: dict[str, Any]) -> None:
    """流式校验文件长度与摘要，对固定下载器检查真实PE架构。"""
    checksum = hashlib.sha256()
    total = 0
    for block in iter(lambda: stream.read(1024 * 1024), b""):
        total += len(block)
        if total > record["size"]:
            raise ValueError("下载器材料实际长度超过声明")
        checksum.update(block)
    if total != record["size"] or checksum.hexdigest() != record["sha256"]:
        raise ValueError("下载器材料长度或摘要不匹配")
    if record["destination"] == EXECUTABLE:
        stream.seek(0)
        validate_binary_stream(stream, "windows-amd64")


def verify(manifest_path: Path, probe: bool = False) -> dict[str, Any]:
    """校验独立成品清单及自有材料；可选只执行固定离线版本探测。"""
    if not manifest_path.is_absolute() or not manifest_path.as_posix().endswith("/" + MANIFEST_NAME) or os.path.normcase(str(manifest_path)) != os.path.normcase(os.path.abspath(manifest_path)):
        raise ValueError("下载器成品清单须为固定位置的绝对路径")
    value = read_manifest(manifest_path)
    records = validate_manifest(value)
    root = manifest_path.parents[2]
    actual: set[str] = set()
    for base, directories, files in os.walk(root / LICENSE_ROOT, followlinks=False):
        for directory in directories:
            info = (Path(base) / directory).lstat()
            if stat.S_ISLNK(info.st_mode) or getattr(info, "st_file_attributes", 0) & 0x400:
                raise ValueError("下载器材料目录包含链接或重解析点")
        for name in files:
            path = Path(base) / name
            ordinary(path)
            local = path.relative_to(root).as_posix()
            if local != MANIFEST_NAME:
                actual.add(local)
    if actual != set(records) - {EXECUTABLE}:
        raise ValueError("下载器材料缺失或夹带未声明文件")
    for name, record in records.items():
        path = root / name
        ordinary(path)
        with path.open("rb") as stream:
            verify_stream(stream, record)
    validate_inventory(read_manifest(root / INVENTORY), value)
    if probe:
        # 不读取用户配置、不允许更新/插件/JS运行时；版本探测不会访问站点。
        with tempfile.TemporaryDirectory(prefix="qiku-ytdlp-probe-", dir=runtime_directory()) as temporary:
            stdout, stderr = Path(temporary) / "stdout", Path(temporary) / "stderr"
            with stdout.open("wb") as output, stderr.open("wb") as errors:
                result = subprocess.run([str(root / EXECUTABLE), "--ignore-config", "--no-plugin-dirs", "--no-js-runtimes", "--no-remote-components", "--version"], cwd=temporary, stdout=output, stderr=errors, check=False, timeout=15)
            if result.returncode != 0 or stdout.stat().st_size > 1024 or stderr.stat().st_size > 65536 or stdout.read_bytes().strip() != VERSION.encode("ascii"):
                raise ValueError("固定下载器本地版本探测未通过")
    return value


def install_runtime(manifest_path: Path, target: str, stage: Path) -> dict[str, str]:
    """从已验证成品复制独立下载器材料，不覆盖其他工具或运行时。"""
    if target != "windows-amd64":
        raise ValueError("固定下载器随包材料目前仅支持Windows x64")
    value = verify(manifest_path)
    root = manifest_path.parents[2]
    records = validate_manifest(value)
    for name, record in records.items():
        copy_verified(root / name, stage / name, record["sha256"], name == EXECUTABLE)
    copy_verified(manifest_path, stage / MANIFEST_NAME, digest(manifest_path))
    verify((stage / MANIFEST_NAME).absolute())
    return {"manifest": MANIFEST_NAME, "sha256": digest(stage / MANIFEST_NAME), "version": VERSION}


def assemble(materials_manifest: Path, output: Path) -> Path:
    """使用维护者准备的固定EXE、完整许可与对应源码库存离线装配。"""
    value = read_manifest(materials_manifest)
    records = validate_manifest(value, packaged=False)
    if output.exists():
        raise ValueError("下载器输出目录已存在，拒绝覆盖")
    for candidate in (output, *output.parents):
        try:
            info = candidate.lstat()
        except FileNotFoundError:
            continue
        if stat.S_ISLNK(info.st_mode) or getattr(info, "st_file_attributes", 0) & 0x400 or not stat.S_ISDIR(info.st_mode):
            raise ValueError("下载器输出目录祖先必须是普通目录，禁止链接或重解析点")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="qiku-ytdlp-", dir=runtime_directory()) as temporary:
        stage = Path(temporary) / "bundle"
        stage.mkdir()
        for name, record in records.items():
            source = Path(record["source"])
            if not source.is_absolute():
                source = materials_manifest.parent / source
            ordinary(source)
            with source.open("rb") as stream:
                verify_stream(stream, record)
            copy_verified(source, stage / name, record["sha256"], name == EXECUTABLE)
            record["source"] = name
        manifest = stage / MANIFEST_NAME
        manifest.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        verify(manifest.absolute())
        shutil.copytree(stage, output)
    verify((output / MANIFEST_NAME).absolute())
    print("固定下载器材料装配完成；来源库存声明由维护者审核，工具校验不替代人工许可判断", flush=True)
    return output / MANIFEST_NAME


def main() -> int:
    """配置中文控制台并执行固定离线装配或验证，不提供下载或自更新入口。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="action", required=True)
    build = commands.add_parser("assemble", help="从审核完成的本地材料装配下载器")
    build.add_argument("--materials-manifest", type=Path, required=True, help="构建材料JSON；含完整许可/源码清单与维护者审核声明")
    build.add_argument("--output", type=Path, required=True, help="不存在的新输出目录，成功后包含tools和licenses/yt-dlp")
    check = commands.add_parser("verify", help="静态校验固定成品清单与所有材料")
    check.add_argument("manifest", type=Path, help="licenses/yt-dlp/yt-dlp-manifest.json的绝对路径")
    check.add_argument("--probe", action="store_true", help="额外执行固定下载器的离线版本探测")
    args = parser.parse_args()
    try:
        if args.action == "assemble":
            print(assemble(args.materials_manifest.absolute(), args.output.absolute()))
        else:
            verify(args.manifest.absolute(), args.probe)
            print("固定下载器材料校验通过")
        return 0
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"下载器材料处理失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    from project_env import run_cli

    raise SystemExit(run_cli(main))
