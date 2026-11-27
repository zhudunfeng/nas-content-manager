"""构建侧生成 Windows JMComic 自动初始化资源；终端只还原固定材料，不执行 pip。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True

import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
from pathlib import Path
import subprocess
from typing import Any
import zipfile

from bootstrap_common import ASSET_NAMES, fetch, payload_bytes, zip_members
import jmcomic_runtime
from project_env import runtime_directory, run_cli
from release import configure_console, relative_path


def relocated(name: str) -> str:
    """严格复用漫画装配器的 wheel purelib/platlib 重定位规则。"""
    result = relative_path(name)
    if ".data/" in result:
        prefix, _, tail = result.partition(".data/")
        category, separator, local = tail.partition("/")
        if not prefix or not separator or category not in ("purelib", "platlib"):
            raise ValueError("固定漫画 wheel 包含不支持的安装重定位目录")
        result = relative_path(local)
    return result


def make_assets(manifest: Path, cache: Path) -> dict[str, bytes]:
    """从严格验证且真实可运行的漫画闭包生成独立的下载配方与差异载荷。"""
    value = jmcomic_runtime.verify(manifest, probe=True)
    if value["pythonVersion"] != jmcomic_runtime.PYTHON_VERSION:
        raise ValueError("漫画自动初始化仅固化已验证的 Python 3.14.4 x64 组合")
    root = manifest.parent
    sources = value["sources"]
    cache.mkdir(parents=True, exist_ok=True)
    print("正在核验并缓存固定官方漫画初始化材料", flush=True)
    with ThreadPoolExecutor(max_workers=4) as executor:
        pending = [executor.submit(fetch, source, cache) for source in sources]
        downloads = [future.result() for future in pending]
    expected = {entry["path"]: entry for entry in value["files"]}
    covered: set[str] = set()
    artifacts: list[dict[str, Any]] = []
    payload: dict[str, bytes] = {}
    for source, archive_path in zip(sources, downloads, strict=True):
        component = source["name"]
        artifact: dict[str, Any] = {
            "name": component,
            "url": source["url"],
            "sha256": source["sha256"],
            "size": archive_path.stat().st_size,
            "format": "zip",
            "files": [],
        }
        members = zip_members(archive_path)
        with zipfile.ZipFile(archive_path) as archive:
            for name, member in members.items():
                destination = jmcomic_runtime.RUNTIME_ROOT + "/" + (name if component == "python" else "Lib/site-packages/" + relocated(name))
                entry = expected.get(destination)
                if entry is None or entry["component"] != component:
                    raise ValueError("官方漫画材料成员不在已验证的运行时库存中")
                data = archive.read(member)
                # 装配器修订的隔离 _pth 等差异仅从已验证安装库存载入，不以原始 ZIP 冒充。
                if len(data) != entry["size"] or hashlib.sha256(data).hexdigest() != entry["sha256"]:
                    continue
                if destination in covered:
                    raise ValueError("多个漫画材料成员映射到同一最终文件")
                covered.add(destination)
                artifact["files"].append({"source": name, "destination": destination})
        if not artifact["files"]:
            raise ValueError("漫画初始化材料没有对应的最终文件")
        artifacts.append(artifact)
    for name, entry in expected.items():
        if name not in covered:
            data = (root / name).read_bytes()
            if len(data) != entry["size"] or hashlib.sha256(data).hexdigest() != entry["sha256"]:
                raise ValueError("漫画运行时输入在资源生成期间发生改变")
            payload[name] = data
    packed = payload_bytes(payload)
    runtime = manifest.read_bytes().replace(b"\r\n", b"\n")
    recipe = {
        "schemaVersion": 1,
        "runtimeManifestSha256": hashlib.sha256(runtime).hexdigest(),
        "payloadSha256": hashlib.sha256(packed).hexdigest(),
        "artifacts": artifacts,
        "tools": [],
    }
    print(f"已生成 {len(artifacts)} 份固定漫画下载材料映射；嵌入载荷 {len(packed)} 字节", flush=True)
    return {"runtime.json": runtime, "recipe.json": (json.dumps(recipe, ensure_ascii=False, indent=2) + "\n").encode("utf-8"), "payload.zip": packed}


def main() -> int:
    """生成或逐字节验证漫画 Go 内嵌资源，失败准确返回非零状态。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--runtime-manifest", type=Path, required=True, help="已装配且真实探测通过的 jmcomic-runtime.json 路径")
    parser.add_argument("--output", type=Path, default=Path("internal/jmcomicbootstrap/assets"), help="Go 嵌入资源目录，默认 internal/jmcomicbootstrap/assets")
    parser.add_argument("--cache", type=Path, default=runtime_directory() / "tools/jmcomic-bootstrap-inputs", help="固定材料缓存，默认位于仓库 .runtime 内")
    parser.add_argument("--check", action="store_true", help="只重新生成并逐字节核对资源，不改写输出文件")
    args = parser.parse_args()
    try:
        assets = make_assets(args.runtime_manifest.absolute(), args.cache.absolute())
        if args.check:
            if any(not (args.output / name).is_file() or (args.output / name).read_bytes() != data for name, data in assets.items()):
                raise ValueError("漫画自动初始化编译资源与已验证输入不一致，请重新生成")
            print("漫画自动初始化资源确定性比对通过", flush=True)
        else:
            args.output.mkdir(parents=True, exist_ok=True)
            for name in ASSET_NAMES:
                (args.output / name).write_bytes(assets[name])
            print("漫画自动初始化资源已写入指定构建目录", flush=True)
        return 0
    except (OSError, ValueError, KeyError, subprocess.SubprocessError, zipfile.BadZipFile) as error:
        print(f"漫画自动初始化资源处理失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(run_cli(main))
