"""下载器材料的持久化离线边界回归；夹具不代表官方二进制或分发审查证据。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True

import argparse
import copy
import hashlib
import io
import json
import tempfile
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Callable, Iterator
from unittest.mock import patch

import ytdlp_runtime as downloader
from project_env import runtime_directory
from release import configure_console, digest


@contextmanager
def fixture_materials(root: Path) -> Iterator[Path]:
    """仅在当前测试上下文替换固定摘要，构造明确不可运行的最小材料夹具。"""
    binary = b"MZ" + b"\x00" * 58 + (64).to_bytes(4, "little") + b"PE\x00\x00\x64\x86"
    materials = {
        downloader.EXECUTABLE: binary,
        f"{downloader.LICENSE_ROOT}/LICENSE": b"fixture-unlicense",
        f"{downloader.LICENSE_ROOT}/THIRD_PARTY_LICENSES.txt": b"fixture-third-party",
        f"{downloader.LICENSE_ROOT}/COPYING-GPL-3.0.txt": b"fixture-gpl",
        f"{downloader.LICENSE_ROOT}/source/yt-dlp.tar.gz": b"fixture-ytdlp-source",
        f"{downloader.LICENSE_ROOT}/source/components.tar.gz": b"fixture-python-pyinstaller-sources-and-builds",
    }
    def checksum(data: bytes) -> str:
        """计算不可执行测试材料身份，不复用真实官方常量。"""
        return hashlib.sha256(data).hexdigest()
    inventory = {"schemaVersion": 1, "version": downloader.VERSION, "executableSha256": checksum(binary), "complete": True, "components": [
        {"name": "yt-dlp", "version": downloader.VERSION, "licenses": [f"{downloader.LICENSE_ROOT}/LICENSE", f"{downloader.LICENSE_ROOT}/THIRD_PARTY_LICENSES.txt"], "sources": [f"{downloader.LICENSE_ROOT}/source/yt-dlp.tar.gz"]},
        *({"name": name, "version": "fixture", "licenses": [f"{downloader.LICENSE_ROOT}/COPYING-GPL-3.0.txt"], "sources": [f"{downloader.LICENSE_ROOT}/source/components.tar.gz"]} for name in ("python", "pyinstaller")),
    ]}
    materials[downloader.INVENTORY] = (json.dumps(inventory) + "\n").encode("utf-8")
    manifest: dict[str, Any] = {"schemaVersion": 1, "target": "windows-amd64", "version": downloader.VERSION, "sourceUrl": downloader.SOURCE_URL, "correspondingSourceReviewed": True, **{group: [] for group in downloader.GROUPS}}
    for index, (name, data) in enumerate(materials.items()):
        source = root / f"material-{index}"
        source.write_bytes(data)
        group = "runtimeFiles" if name == downloader.EXECUTABLE else "sourceFiles" if name == downloader.INVENTORY or "/source/" in name else "licenseFiles"
        manifest[group].append({"source": str(source), "destination": name, "sha256": checksum(data), "size": len(data)})
    path = root / "materials.json"
    path.write_text(json.dumps(manifest), encoding="utf-8")
    # 所有替换仅用于边界测试；生产CLI没有跳过官方身份的参数。
    with patch.multiple(downloader, EXE_SHA256=checksum(binary), SOURCE_SHA256=checksum(materials[f"{downloader.LICENSE_ROOT}/source/yt-dlp.tar.gz"]), THIRD_PARTY_SHA256=checksum(materials[f"{downloader.LICENSE_ROOT}/THIRD_PARTY_LICENSES.txt"]), UNLICENSE_SHA256=checksum(materials[f"{downloader.LICENSE_ROOT}/LICENSE"])):
        yield path


def self_test() -> int:
    """验证装配、成品复制、身份/材料/库存/路径/文件预算失败边界。"""
    count = 0

    def rejected(action: Callable[[], Any]) -> None:
        """确认受测非法输入明确失败，禁止静默接受。"""
        nonlocal count
        try:
            action()
        except (ValueError, OSError):
            count += 1
        else:
            raise AssertionError("下载器非法夹具未被拒绝")

    with tempfile.TemporaryDirectory(prefix="qiku-ytdlp-check-", dir=runtime_directory()) as temporary:
        root = Path(temporary)
        with fixture_materials(root) as material_path:
            output = root / "中文 空格 bundle"
            manifest = downloader.assemble(material_path, output)
            value = downloader.verify(manifest.absolute())
            count += 1
            copied = root / "installed"
            copied.mkdir()
            summary = downloader.install_runtime(manifest.absolute(), "windows-amd64", copied)
            assert summary == {"manifest": downloader.MANIFEST_NAME, "sha256": digest(manifest), "version": downloader.VERSION}
            count += 1
            rejected(lambda: downloader.install_runtime(manifest.absolute(), "linux-amd64", copied))
            rejected(lambda: downloader.assemble(material_path, output))
            for extra in ({"version": "latest"}, {"target": "windows-arm64"}, {"correspondingSourceReviewed": False}, {"unexpected": True}):
                rejected(lambda: downloader.validate_manifest({**value, **extra}))
            for group in downloader.GROUPS:
                rejected(lambda: downloader.validate_manifest({**value, group: []}))
            for key, replacement in (("destination", "tools/../yt-dlp.exe"), ("destination", "tools/YT-DLP.exe"), ("sha256", "0" * 64), ("size", True), ("source", "C:/build/private.exe")):
                malformed = copy.deepcopy(value)
                malformed["runtimeFiles"][0][key] = replacement
                rejected(lambda: downloader.validate_manifest(malformed))
            malformed = copy.deepcopy(value)
            malformed["sourceFiles"].append({**malformed["sourceFiles"][0], "destination": malformed["sourceFiles"][0]["destination"].upper()})
            rejected(lambda: downloader.validate_manifest(malformed))
            for first, second in (("nested/first", "Nested/second"), ("nested", "nested/second"), ("nested/first", "nested")):
                malformed = copy.deepcopy(value)
                malformed["sourceFiles"].extend({**malformed["sourceFiles"][0], "destination": f"{downloader.LICENSE_ROOT}/{name}", "source": f"{downloader.LICENSE_ROOT}/{name}"} for name in (first, second))
                rejected(lambda: downloader.validate_manifest(malformed))
            for field, replacement in (("size", downloader.MAX_FILE + 1), ("size", 512 * 1024 * 1024 + 1)):
                malformed = copy.deepcopy(value)
                malformed["runtimeFiles"][0][field] = replacement
                rejected(lambda: downloader.validate_manifest(malformed))
            for group in ("licenseFiles", "sourceFiles"):
                malformed = copy.deepcopy(value)
                malformed[group][0]["sha256"] = "0" * 64
                rejected(lambda: downloader.validate_manifest(malformed))
            inventory = downloader.read_manifest(output / downloader.INVENTORY)
            for extra in ({"complete": False}, {"executableSha256": "0" * 64}, {"components": inventory["components"][:2]}):
                rejected(lambda: downloader.validate_inventory({**inventory, **extra}, value))
            invalid_inventory = copy.deepcopy(inventory)
            invalid_inventory["components"][0]["sources"] = ["licenses/yt-dlp/missing"]
            rejected(lambda: downloader.validate_inventory(invalid_inventory, value))
            invalid_inventory = copy.deepcopy(inventory)
            invalid_inventory["components"][0]["licenses"] = [invalid_inventory["components"][0]["licenses"][0]]
            rejected(lambda: downloader.validate_inventory(invalid_inventory, value))
            extra_file = output / downloader.LICENSE_ROOT / "undeclared"
            extra_file.write_bytes(b"extra")
            rejected(lambda: downloader.verify(manifest.absolute()))
            extra_file.unlink()
            executable = output / downloader.EXECUTABLE
            original = executable.read_bytes()
            executable.write_bytes(original + b"tail")
            rejected(lambda: downloader.verify(manifest.absolute()))
            executable.write_bytes(original)
            record = value["runtimeFiles"][0]
            rejected(lambda: downloader.verify_stream(io.BytesIO(b"bad"), record))
            rejected(lambda: downloader.verify_stream(io.BytesIO(b"bad"), {**record, "sha256": hashlib.sha256(b"bad").hexdigest(), "size": 3}))
            duplicate = root / "duplicate.json"
            duplicate.write_text('{"version":1,"version":2}', encoding="utf-8")
            rejected(lambda: downloader.read_manifest(duplicate))
            duplicate.write_bytes(b"x" * (downloader.MANIFEST_LIMIT + 1))
            rejected(lambda: downloader.read_manifest(duplicate))
        # 测试上下文结束后，真实固定官方身份必须恢复并拒绝夹具。
        rejected(lambda: downloader.verify(manifest.absolute()))
    print(f"下载器离线材料边界回归通过：{count}项（仅夹具，不代表官方许可材料齐备）")
    return count


def main() -> int:
    """执行持久化边界回归或校验维护者提供的真实完整材料。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--self-test", action="store_true", help="运行不联网的材料夹具边界回归，不能替代真实分发审查")
    parser.add_argument("--manifest", type=Path, help="维护者已装配的真实下载器清单绝对路径")
    parser.add_argument("--probe", action="store_true", help="对真实材料额外执行固定EXE的离线版本探测")
    args = parser.parse_args()
    try:
        if args.self_test:
            self_test()
        if args.manifest:
            downloader.verify(args.manifest.absolute(), args.probe)
            print("真实下载器材料校验通过")
        if not args.self_test and not args.manifest:
            parser.error("请提供--self-test或--manifest")
        return 0
    except (OSError, ValueError, AssertionError) as error:
        print(f"下载器材料回归失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    from project_env import run_cli

    raise SystemExit(run_cli(main))
