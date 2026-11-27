"""构建侧生成 Windows UAV 自动初始化资源；终端仅还原这些固定材料，不运行 pip。"""

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
import zipfile
from typing import Any

from project_env import runtime_directory, run_cli
from release import configure_console, relative_path
from bootstrap_common import ARTIFACT_LIMIT, ASSET_NAMES, fetch, payload_bytes, record, zip_members
import uav_runtime
import ytdlp_runtime


FFMPEG_VERSION = "9.0.2"  # 固定的上游静态 Windows x64 构建，不追踪 latest。
FFMPEG_URL = "https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-9.0.2-essentials_build.zip"  # FFmpeg 官网链接的 Windows 构建提供方固定归档。
FFMPEG_SHA256 = "60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba"  # 同名官方 .sha256 固定摘要，不代表验证了发布者签名。
FFMPEG_PREFIX = "ffmpeg-9.0.2-essentials_build/"  # 只从该归档根选择固定工具与许可。


def relocated(name: str, component: str) -> str:
    """复用构建器的 wheel 重定位与 metadata 目录规范化约定。"""
    result = relative_path(name)
    if ".data/" in result:
        prefix, _, tail = result.partition(".data/")
        category, separator, local = tail.partition("/")
        if not prefix or not separator or category not in ("purelib", "platlib"):
            raise ValueError("固定 wheel 包含未支持的重定位目录")
        result = relative_path(local)
    parts = result.split("/")
    if parts[0].lower().endswith(".dist-info"):
        version = uav_runtime.contract()["packages"][component]["version"]
        canonical = f'{component.replace("-", "_")}-{version}.dist-info'
        if parts[0].casefold() != canonical.casefold():
            raise ValueError("固定 wheel 元数据目录与依赖版本不匹配")
        result = "/".join([canonical, *parts[1:]])
    return result


def make_assets(manifest: Path, cache: Path) -> dict[str, bytes]:
    """从已严格验证的运行时与固定上游物料生成完整还原 recipe。"""
    value = uav_runtime.verify(manifest, probe=True)
    if value["pythonVersion"] != uav_runtime.PYTHON_VERSION:
        raise ValueError("自动初始化仅固化已经验收的 Python 3.14.4 x64 组合")
    root = manifest.parent
    sources = [source.copy() for source in value["sources"] if source["name"] != "uav-downloader"]
    sources.extend([
        {"name": "yt-dlp", "url": ytdlp_runtime.SOURCE_URL, "sha256": ytdlp_runtime.EXE_SHA256},
        {"name": "ffmpeg", "url": FFMPEG_URL, "sha256": FFMPEG_SHA256},
    ])
    cache.mkdir(parents=True, exist_ok=True)
    print("正在核验并缓存固定官方初始化材料", flush=True)
    with ThreadPoolExecutor(max_workers=4) as executor:
        pending = [executor.submit(fetch, source, cache) for source in sources]
        downloads = [future.result() for future in pending]
    expected = {entry["path"]: entry for entry in value["files"]}
    covered: set[str] = set()
    artifacts: list[dict[str, Any]] = []
    payload: dict[str, bytes] = {}
    tools: list[dict[str, Any]] = []
    for source, path in zip(sources, downloads, strict=True):
        component = source["name"]
        artifact: dict[str, Any] = {"name": component, "url": source["url"], "sha256": source["sha256"], "size": path.stat().st_size, "format": "raw" if component == "yt-dlp" else "zip", "files": []}
        if component == "yt-dlp":
            artifact["files"].append({"source": "", "destination": "tools/yt-dlp.exe"})
            tools.append(record("tools/yt-dlp.exe", path.read_bytes()))
        else:
            members = zip_members(path)
            with zipfile.ZipFile(path) as archive:
                for name, member in members.items():
                    if component == "ffmpeg":
                        selected = {FFMPEG_PREFIX + "bin/ffmpeg.exe": "tools/ffmpeg.exe", FFMPEG_PREFIX + "bin/ffprobe.exe": "tools/ffprobe.exe", FFMPEG_PREFIX + "LICENSE": "licenses/ffmpeg/LICENSE", FFMPEG_PREFIX + "README.txt": "licenses/ffmpeg/README.txt"}
                        destination = selected.get(name)
                        if destination is None:
                            continue
                        data = archive.read(member)
                        tools.append(record(destination, data))
                    else:
                        destination = "runtimes/uav/" + (name if component == "python" else "Lib/site-packages/" + relocated(name, component))
                        entry = expected.get(destination)
                        if entry is None or entry["component"] != component:
                            raise ValueError("上游物料成员不在已经验证的运行时库存中")
                        data = archive.read(member)
                        # 仅 _pth 或规范化 RECORD 等真实差异进入随程序的安装文件载荷。
                        if len(data) != entry["size"] or hashlib.sha256(data).hexdigest() != entry["sha256"]:
                            continue
                        if destination in covered:
                            raise ValueError("多个上游成员映射到同一个最终文件")
                        covered.add(destination)
                    artifact["files"].append({"source": name, "destination": destination})
        if not artifact["files"]:
            raise ValueError("初始化材料没有对应的最终文件")
        artifacts.append(artifact)
    for name, entry in expected.items():
        if name not in covered:
            data = (root / name).read_bytes()
            if len(data) != entry["size"] or hashlib.sha256(data).hexdigest() != entry["sha256"]:
                raise ValueError("运行时输入在资源生成期间发生改变")
            payload[name] = data
    # 保留官方下载器许可聚合文本；不生成 correspondingSourceReviewed 等人工审核声明。
    notices = {
        "licenses/yt-dlp/LICENSE": ("https://raw.githubusercontent.com/yt-dlp/yt-dlp/2026.10.26/LICENSE", ytdlp_runtime.UNLICENSE_SHA256),
        "licenses/yt-dlp/THIRD_PARTY_LICENSES.txt": ("https://raw.githubusercontent.com/yt-dlp/yt-dlp/2026.10.26/THIRD_PARTY_LICENSES.txt", ytdlp_runtime.THIRD_PARTY_SHA256),
    }
    for name, (url, checksum) in notices.items():
        data = fetch({"name": "notice", "url": url, "sha256": checksum}, cache).read_bytes()
        payload[name] = data
        tools.append(record(name, data))
    packed = payload_bytes(payload)
    # 清单仅规范 JSON 外部换行，文件库存内的全部字节身份保持不变。
    runtime = manifest.read_bytes().replace(b"\r\n", b"\n")
    recipe = {"schemaVersion": 1, "runtimeManifestSha256": hashlib.sha256(runtime).hexdigest(), "payloadSha256": hashlib.sha256(packed).hexdigest(), "artifacts": artifacts, "tools": sorted(tools, key=lambda item: item["path"])}
    print(f"已生成 {len(artifacts)} 份固定下载物料映射；嵌入载荷 {len(packed)} 字节", flush=True)
    return {"runtime.json": runtime, "recipe.json": (json.dumps(recipe, ensure_ascii=False, indent=2) + "\n").encode(), "payload.zip": packed}


def main() -> int:
    """生成或比对 Go 编译资源，所有命令参数均提供中文用途说明。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--runtime-manifest", type=Path, required=True, help="已装配并可真实探测的 uav-runtime.json 绝对路径")
    parser.add_argument("--output", type=Path, default=Path("internal/uavbootstrap/assets"), help="Go 嵌入资源目录，默认 internal/uavbootstrap/assets")
    parser.add_argument("--cache", type=Path, default=runtime_directory() / "tools/uav-bootstrap-inputs", help="构建材料缓存，默认位于仓库 .runtime 内")
    parser.add_argument("--check", action="store_true", help="只重新生成并逐字节比对资源，不写入输出")
    args = parser.parse_args()
    try:
        assets = make_assets(args.runtime_manifest.absolute(), args.cache.absolute())
        if args.check:
            if any(not (args.output / name).is_file() or (args.output / name).read_bytes() != data for name, data in assets.items()):
                raise ValueError("自动初始化编译资源与已验证输入不一致，请重新生成")
            print("自动初始化资源确定性比对通过", flush=True)
        else:
            args.output.mkdir(parents=True, exist_ok=True)
            for name in ASSET_NAMES:
                (args.output / name).write_bytes(assets[name])
            print("自动初始化资源已写入指定构建目录", flush=True)
        return 0
    except (OSError, ValueError, KeyError, subprocess.SubprocessError, zipfile.BadZipFile) as error:
        print(f"自动初始化资源处理失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(run_cli(main))
