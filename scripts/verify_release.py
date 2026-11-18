"""流式核验发布归档、内外SHA-256、目标结构与工具材料；不运行Docker或解压不可信路径。"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import tarfile
import tempfile
import zipfile
from pathlib import Path
from typing import Any, BinaryIO, Callable

from release import SHA256, TARGETS, TOOL_GROUPS, TOOL_MANIFEST, archive_tree, digest, install_images, install_tools, read_json, relative_path, unique_object, validate_binary, validate_binary_stream, validate_images_manifest, validate_tools_manifest


def metadata_object(data: bytes) -> dict[str, Any]:
    """解析有限元数据并拒绝重复字段及非对象值，使校验和构建保持同一解释。"""
    if len(data) > 1024 * 1024:
        raise ValueError("发布元数据过大")
    value = json.loads(data, object_pairs_hook=unique_object)
    if not isinstance(value, dict):
        raise ValueError("发布元数据必须为JSON对象")
    return value


def verify_tool_architectures(path: Path, root: str, target: str) -> None:
    """重新以归档流检查两个固定工具的架构，不把归档解压到用户文件系统。"""
    suffix = ".exe" if target.startswith("windows-") else ""
    names = [f"{root}/tools/{name}{suffix}" for name in ("ffmpeg", "ffprobe")]
    if path.suffix == ".zip":
        with zipfile.ZipFile(path) as archive:
            for name in names:
                with archive.open(name) as stream:
                    validate_binary_stream(stream, target)
    else:
        with tarfile.open(path, "r:gz") as archive:
            for name in names:
                stream = archive.extractfile(name)
                if stream is None:
                    raise ValueError("归档工具无法读取")
                with stream:
                    validate_binary_stream(stream, target)


def stream_digest(stream: BinaryIO) -> str:
    """分块校验单个归档成员，空间不随归档或镜像大小增长。"""
    value = hashlib.sha256()
    for block in iter(lambda: stream.read(1024 * 1024), b""):
        value.update(block)
    return value.hexdigest()


def verify(path: Path) -> int:
    """核对完整包的每一份文件，不接受遗漏、重复路径、链接或未经校验的额外文件。"""
    expected = path.with_name(path.name + ".sha256").read_text(encoding="ascii").strip()
    if expected != f"{digest(path)}  {path.name}":
        raise ValueError("外部归档摘要不匹配")
    hashes: dict[str, str] = {}
    small: dict[str, bytes] = {}
    roots: set[str] = set()
    folded: set[str] = set()

    def member(name: str, stream: BinaryIO, size: int) -> None:
        """核对归档成员路径并只缓存有限的元数据，不缓存媒体工具和镜像。"""
        safe = relative_path(name)
        root, separator, local = safe.partition("/")
        if not separator or not local or safe.casefold() in folded:
            raise ValueError("归档路径重复或缺少单一根目录")
        roots.add(root)
        folded.add(safe.casefold())
        if local in ("release.json", "checksums.sha256", "release-images.json", TOOL_MANIFEST):
            if size > 1024 * 1024:
                raise ValueError("归档元数据过大")
            data = stream.read(1024 * 1024 + 1)
            if len(data) != size:
                raise ValueError("归档元数据截断")
            small[local] = data
            hashes[local] = hashlib.sha256(data).hexdigest()
        else:
            hashes[local] = stream_digest(stream)

    if path.suffix == ".zip":
        with zipfile.ZipFile(path) as archive:
            for info in archive.infolist():
                mode = info.external_attr >> 16
                if info.is_dir() or (mode & 0o170000) not in (0, 0o100000) or info.flag_bits & 1:
                    raise ValueError("归档包含目录、链接或加密成员")
                with archive.open(info) as stream:
                    member(info.filename, stream, info.file_size)
    else:
        with tarfile.open(path, "r|gz") as archive:
            for info in archive:
                if not info.isfile():
                    raise ValueError("归档包含非普通文件")
                stream = archive.extractfile(info)
                if stream is None:
                    raise ValueError("无法读取归档文件")
                with stream:
                    member(info.name, stream, info.size)
    if len(roots) != 1 or not {"release.json", "checksums.sha256"}.issubset(small):
        raise ValueError("归档缺少唯一根目录或校验元数据")
    checksums: dict[str, str] = {}
    for line in small["checksums.sha256"].decode("utf-8").splitlines():
        checksum, separator, name = line.partition("  ")
        if not separator or not SHA256.fullmatch(checksum) or relative_path(name) in checksums:
            raise ValueError("包内摘要清单无效或重复")
        checksums[name] = checksum
    actual = {name: value for name, value in hashes.items() if name != "checksums.sha256"}
    if checksums != actual:
        raise ValueError("包内文件缺失、被修改或存在未校验的额外文件")
    metadata = metadata_object(small["release.json"])
    if set(metadata) != {"schemaVersion", "version", "target", "developmentWithoutTools", "tools"} or type(metadata["schemaVersion"]) is not int or metadata["schemaVersion"] != 1 or not isinstance(metadata["target"], str) or metadata["target"] not in TARGETS or type(metadata["developmentWithoutTools"]) is not bool or not isinstance(metadata["version"], str) or not re.fullmatch(r"[0-9A-Za-z][0-9A-Za-z.+_-]{0,63}", metadata["version"]):
        raise ValueError("发布元数据无效")
    windows = metadata["target"].startswith("windows-")
    required = {"nas.exe", "栖库.exe"} if windows else {"nas", "start.sh"}
    required |= {"compose.desktop.yaml", "开始使用.txt"}
    if not metadata["developmentWithoutTools"]:
        suffix = ".exe" if windows else ""
        required |= {f"tools/ffmpeg{suffix}", f"tools/ffprobe{suffix}", TOOL_MANIFEST}
        if TOOL_MANIFEST not in small:
            raise ValueError("完整发布缺少工具清单")
        tools = metadata_object(small[TOOL_MANIFEST])
        validate_tools_manifest(tools, metadata["target"], packaged=True)
        if metadata["tools"] != {key: tools[key] for key in ("version", "sourceUrl")}:
            raise ValueError("发布元数据与工具版本、来源不一致")
        for group in TOOL_GROUPS:
            for record in tools[group]:
                if hashes.get(record["destination"]) != record["sha256"]:
                    raise ValueError("工具、许可或源码材料与清单摘要不一致")
        listed = {record["destination"] for group in TOOL_GROUPS for record in tools[group]}
        materials = {name for name in hashes if name.startswith(("tools/", "licenses/ffmpeg/")) and name != TOOL_MANIFEST}
        if listed != materials:
            raise ValueError("工具或再分发材料存在未声明文件")
    elif metadata["tools"] is not None:
        raise ValueError("开发验证包不能声明完整工具材料")
    if not required.issubset(hashes):
        raise ValueError("发布包缺少平台入口或必需材料")
    if not metadata["developmentWithoutTools"]:
        verify_tool_architectures(path, next(iter(roots)), metadata["target"])
    if "release-images.json" in small:
        manifest = metadata_object(small["release-images.json"])
        name = validate_images_manifest(manifest)
        if name is not None and hashes.get(name) != manifest["archiveSha256"]:
            raise ValueError("离线镜像归档摘要不匹配")
    print(f"发布包校验通过：{metadata['target']}，{len(hashes)}份文件，开发验证包={metadata['developmentWithoutTools']}")
    return len(hashes)


def self_test() -> int:
    """运行构建边界和确定性封装回归，测试材料仅写入自动清理的临时目录。"""
    count = 0

    def rejected(action: Callable[[], Any], description: str) -> None:
        """确认非法输入被明确拒绝，避免异常类型以外的静默通过。"""
        nonlocal count
        try:
            action()
        except ValueError:
            count += 1
        else:
            raise AssertionError(description)

    def seal(stage: Path, destination: Path, windows: bool = True) -> None:
        """为测试目录重新生成完整摘要与归档，以隔离结构校验和传输校验。"""
        (stage / "checksums.sha256").write_text("".join(f"{digest(p)}  {p.relative_to(stage).as_posix()}\n" for p in sorted(stage.rglob("*")) if p.is_file() and p.name != "checksums.sha256"), encoding="utf-8")
        archive_tree(stage, destination, windows)
        destination.with_name(destination.name + ".sha256").write_text(f"{digest(destination)}  {destination.name}\n", encoding="ascii")

    for name in ("../escape", "/absolute", "C:/drive", "a\\b", "a//b", "a/./b", "a/../b", "a/line\nb", "a/line\rb", "a/tab\tb", "a/\x7fb", "a/name.", "a/name ", "a/CON", "a/nul.txt", "a/COM1", "a/LPT9.txt", "a/a?b", 'a/a"b'):
        rejected(lambda: relative_path(name), "接受了不安全的包内路径")
    for value in (b"[]", b"null", b'{"version":1,"version":2}'):
        rejected(lambda: metadata_object(value), "归档元数据接受了非对象或重复字段")
    with tempfile.TemporaryDirectory(prefix="qiku-release-check-") as temporary:
        root = Path(temporary)
        duplicate = root / "duplicate.json"
        duplicate.write_text('{"schemaVersion":1,"schemaVersion":2}', encoding="utf-8")
        rejected(lambda: read_json(duplicate), "重复JSON字段未被拒绝")
        manifest = root / "release-images.json"
        manifest.write_text(json.dumps({"schemaVersion": 1, "appImage": "example/app:latest", "helperImage": "sha256:" + "a" * 64}), encoding="utf-8")
        stage = root / "qiku-test"
        stage.mkdir()
        rejected(lambda: install_images(manifest, stage), "可变镜像标签未被拒绝")
        fixed_images = {"schemaVersion": 1, "appImage": "sha256:" + "a" * 64, "helperImage": "sha256:" + "b" * 64}
        rejected(lambda: validate_images_manifest({**fixed_images, "helperImage": fixed_images["appImage"]}), "应用与助手相同入口未被发布校验拒绝")
        for extra in ({"archive": None, "archiveSha256": None}, {"archive": "images/archive.tar"}, {"archive": "other/archive.tar", "archiveSha256": "c" * 64}, {"archive": "images/archive.tar", "archiveSha256": "invalid"}):
            rejected(lambda: validate_images_manifest({**fixed_images, **extra}), "错误镜像归档字段未被拒绝")
        wrong = root / "wrong-tool"
        wrong.write_bytes(b"not an executable")
        for target in TARGETS:
            rejected(lambda: validate_binary(wrong, target), "错误架构工具未被拒绝")
        for name in ("nas.exe", "栖库.exe", "compose.desktop.yaml", "开始使用.txt"):
            (stage / name).write_text("仅用于封装验证", encoding="utf-8")
        metadata = {"schemaVersion": 1, "version": "0.1.0", "target": "windows-amd64", "developmentWithoutTools": True, "tools": None}
        (stage / "release.json").write_text(json.dumps(metadata), encoding="utf-8")
        first, second = root / "first.zip", root / "second.zip"
        seal(stage, first)
        seal(stage, second)
        if digest(first) != digest(second):
            raise AssertionError("相同输入未产生确定性归档")
        count += 1
        verify(first)
        count += 1
        for raw in ("[]", '{"schemaVersion":1,"schemaVersion":1}', json.dumps({**metadata, "schemaVersion": True})):
            (stage / "release.json").write_text(raw, encoding="utf-8")
            seal(stage, second)
            rejected(lambda: verify(second), "错误发布元数据未被归档校验拒绝")
        (stage / "release.json").write_text(json.dumps(metadata), encoding="utf-8")
        (stage / "release-images.json").write_text(json.dumps({**fixed_images, "appImage": "example/app:latest"}), encoding="utf-8")
        seal(stage, second)
        rejected(lambda: verify(second), "归档校验接受了可变镜像")
        (stage / "release-images.json").unlink()

        # PE头部只是包校验夹具，不能作为真实工具运行或许可审核证据。
        binary = root / "binary.exe"
        binary.write_bytes(b"MZ" + b"\x00" * 58 + (64).to_bytes(4, "little") + b"PE\x00\x00\x64\x86")
        license_file = root / "license.txt"
        source_file = root / "source.txt"
        license_file.write_text("仅用于校验的许可材料", encoding="utf-8")
        source_file.write_text("仅用于校验的对应源码材料", encoding="utf-8")
        tools = {"schemaVersion": 1, "target": "windows-amd64", "version": "fixture-1", "sourceUrl": "https://example.invalid/fixture", "runtimeFiles": [{"source": str(binary), "destination": f"tools/{name}.exe", "sha256": digest(binary)} for name in ("ffmpeg", "ffprobe")], "licenseFiles": [{"source": str(license_file), "destination": "licenses/ffmpeg/license.txt", "sha256": digest(license_file)}], "sourceFiles": [{"source": str(source_file), "destination": "licenses/ffmpeg/source.txt", "sha256": digest(source_file)}]}
        tools_path = root / "tools.json"
        for target in TARGETS:
            suffix = ".exe" if target.startswith("windows-") else ""
            malformed = {**tools, "target": target, "runtimeFiles": [{**record, "destination": f"tools/{name}{suffix}"} for record, name in zip(tools["runtimeFiles"], ("FFMPEG", "FFPROBE"))]}
            tools_path.write_text(json.dumps(malformed), encoding="utf-8")
            rejected(lambda: install_tools(tools_path, target, stage), "工具大小写绕过了固定名称与架构校验")
        reserved = {**tools, "sourceFiles": [{**tools["sourceFiles"][0], "destination": TOOL_MANIFEST}]}
        tools_path.write_text(json.dumps(reserved), encoding="utf-8")
        rejected(lambda: install_tools(tools_path, "windows-amd64", stage), "输入材料覆盖了自动生成的工具清单")
        tools_path.write_text(json.dumps(tools), encoding="utf-8")
        summary = install_tools(tools_path, "windows-amd64", stage)
        (stage / "release.json").write_text(json.dumps({**metadata, "developmentWithoutTools": False, "tools": summary}), encoding="utf-8")
        seal(stage, second)
        verify(second)
        count += 1
        packaged_path = stage / TOOL_MANIFEST
        packaged = metadata_object(packaged_path.read_bytes())
        for replacement in ({**packaged, "sourceFiles": []}, {**packaged, "licenseFiles": []}, {**packaged, "version": "other"}):
            packaged_path.write_text(json.dumps(replacement), encoding="utf-8")
            seal(stage, second)
            rejected(lambda: verify(second), "完整包缺失材料或版本不一致未被拒绝")
        packaged_path.write_text(json.dumps(packaged), encoding="utf-8")
        # 即使两层摘要都重算，也不能把错误架构工具伪装成完整发布。
        (stage / "tools/ffmpeg.exe").write_bytes(b"wrong architecture")
        packaged["runtimeFiles"][0]["sha256"] = digest(stage / "tools/ffmpeg.exe")
        packaged_path.write_text(json.dumps(packaged), encoding="utf-8")
        seal(stage, second)
        rejected(lambda: verify(second), "归档校验未检查工具架构")
        for target, machine in (("linux-amd64", 62), ("linux-arm64", 183)):
            linux_stage = root / f"qiku-{target}"
            linux_stage.mkdir()
            for name in ("nas", "start.sh", "compose.desktop.yaml", "开始使用.txt"):
                (linux_stage / name).write_text("仅用于Linux封装验证", encoding="utf-8")
            elf = bytearray(64)
            elf[:6] = b"\x7fELF\x02\x01"
            elf[18:20] = machine.to_bytes(2, "little")
            binary.write_bytes(elf)
            linux_tools = {**tools, "target": target, "runtimeFiles": [{"source": str(binary), "destination": f"tools/{name}", "sha256": digest(binary)} for name in ("ffmpeg", "ffprobe")]}
            tools_path.write_text(json.dumps(linux_tools), encoding="utf-8")
            summary = install_tools(tools_path, target, linux_stage)
            (linux_stage / "release.json").write_text(json.dumps({**metadata, "target": target, "developmentWithoutTools": False, "tools": summary}), encoding="utf-8")
            first_tar, second_tar = root / f"{target}-first.tar.gz", root / f"{target}-second.tar.gz"
            seal(linux_stage, first_tar, False)
            seal(linux_stage, second_tar, False)
            if digest(first_tar) != digest(second_tar):
                raise AssertionError("Linux封装结果不确定")
            count += 1
            verify(first_tar)
            count += 1
        with zipfile.ZipFile(first, "a") as archive:
            archive.writestr("qiku-test/extra.txt", "unexpected")
        first.with_name(first.name + ".sha256").write_text(f"{digest(first)}  {first.name}\n", encoding="ascii")
        rejected(lambda: verify(first), "未经内部校验的额外文件未被拒绝")
    print(f"发布边界回归通过：{count}项")
    return count


def main() -> int:
    """解析归档校验或构建边界回归模式。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", nargs="?", type=Path, help="release.py生成的ZIP或tar.gz及同目录.sha256")
    parser.add_argument("--self-test", action="store_true", help="验证路径、重复字段、固定镜像、架构及归档一致性")
    args = parser.parse_args()
    try:
        if args.self_test:
            self_test()
        if args.archive:
            verify(args.archive.resolve())
        if not args.self_test and not args.archive:
            parser.error("请提供归档路径或--self-test")
        return 0
    except (OSError, ValueError, AssertionError, zipfile.BadZipFile, tarfile.TarError) as error:
        print(f"发布校验失败：{error}")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
