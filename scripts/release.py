"""构建带嵌入网页的跨平台发布包；终端用户运行发布包无需 Python 或开发工具。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    # 直接执行入口时，在其他本地模块导入前关闭源码旁的字节码缓存。
    sys.dont_write_bytecode = True

import argparse
import gzip
import hashlib
import json
import os
import re
import shutil
import stat
import struct
import subprocess
import tarfile
import tempfile
import zipfile
from pathlib import Path, PurePosixPath
from typing import Any, BinaryIO

from project_env import resolve_command, runtime_directory


ROOT = Path(__file__).resolve().parents[1]  # 由当前脚本定位的源码根，不依赖调用工作目录。
TARGETS = {"windows-amd64": ("windows", "amd64"), "linux-amd64": ("linux", "amd64"), "linux-arm64": ("linux", "arm64")}  # 已纳入构建矩阵的目标。
SHA256 = re.compile(r"[0-9a-f]{64}\Z")  # 发布材料使用小写 SHA-256，禁止省略或伪造校验。
IMAGE_REFERENCE = re.compile(r"(?:sha256:|[a-zA-Z0-9][a-zA-Z0-9._:/-]*@sha256:)[0-9a-f]{64}\Z")  # 镜像只允许不可变身份。
TOOL_GROUPS = ("runtimeFiles", "licenseFiles", "sourceFiles")  # 运行文件、许可与对应源码必须分别提供。
TOOL_MANIFEST = "licenses/ffmpeg/tools-manifest.json"  # 由构建器独占生成的脱敏工具清单。


def configure_console() -> None:
    """统一发布入口的中文输出编码，并兼容没有重配置接口的内存替代流。"""
    # Windows CI 重定向输出可能采用西文代码页，必须在中文帮助或日志写入前切换。
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if callable(reconfigure):
            reconfigure(encoding="utf-8", errors="backslashreplace")


def digest(path: Path) -> str:
    """分块计算文件摘要，不将工具或镜像归档整体读入内存。"""
    result = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            result.update(block)
    return result.hexdigest()


def unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    """拒绝重复 JSON 字段，避免校验者与执行者解释不同。"""
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"清单包含重复字段：{key}")
        result[key] = value
    return result


def read_json(path: Path) -> dict[str, Any]:
    """有界读取普通 JSON 对象，拒绝软链接及超大配置。"""
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 1024 * 1024:
        raise ValueError("发布清单必须为不超过 1 MiB 的普通文件")
    with path.open("rb") as stream:
        data = stream.read(1024 * 1024 + 1)
    if len(data) > 1024 * 1024:
        raise ValueError("发布清单读取期间超过1 MiB预算")
    value = json.loads(data.decode("utf-8-sig"), object_pairs_hook=unique_object)
    if not isinstance(value, dict):
        raise ValueError("发布清单必须为 JSON 对象")
    return value


def relative_path(value: Any) -> str:
    """校验两端均可解压的包内路径，拒绝控制字符、设备名、别名及目录穿越。"""
    if not isinstance(value, str) or not value or any(ord(char) < 32 or ord(char) == 127 or char in '\\:<>"|?*' for char in value):
        raise ValueError("包内路径必须为安全的正斜杠相对路径")
    path = PurePosixPath(value)
    if path.is_absolute() or any(part in ("", ".", "..") for part in value.split("/")):
        raise ValueError("包内路径不能包含绝对路径或上级目录")
    reserved = {"CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "CLOCK$", *(f"COM{number}" for number in "123456789¹²³"), *(f"LPT{number}" for number in "123456789¹²³")}
    if any(part[-1] in " ." or part.split(".", 1)[0].rstrip(" .").upper() in reserved for part in path.parts):
        raise ValueError("包内路径不能包含Windows设备名或尾部点、空格")
    return path.as_posix()


def validate_tools_manifest(manifest: dict[str, Any], target: str, packaged: bool = False) -> None:
    """统一验证源清单与成品清单，固定工具大小写并禁止覆盖生成的元数据。"""
    keys = {"schemaVersion", "target", "version", "sourceUrl", *TOOL_GROUPS}
    if set(manifest) != keys or type(manifest["schemaVersion"]) is not int or manifest["schemaVersion"] != 1 or manifest["target"] != target or target not in TARGETS:
        raise ValueError("工具清单字段、版本或目标不匹配")
    if not isinstance(manifest["version"], str) or not manifest["version"].strip() or not isinstance(manifest["sourceUrl"], str) or not manifest["sourceUrl"].startswith("https://"):
        raise ValueError("工具清单必须说明固定版本和HTTPS来源")
    seen: set[str] = set()
    runtime: set[str] = set()
    suffix = ".exe" if target.startswith("windows-") else ""
    required = {f"tools/{name}{suffix}" for name in ("ffmpeg", "ffprobe")}
    for group in TOOL_GROUPS:
        records = manifest[group]
        if not isinstance(records, list) or not records or len(records) > 512:
            raise ValueError(f"工具清单缺少或包含过多{group}材料")
        for record in records:
            if not isinstance(record, dict) or set(record) != {"source", "destination", "sha256"}:
                raise ValueError("每份发布材料必须包含source、destination、sha256")
            destination = relative_path(record["destination"])
            prefix = "tools/" if group == "runtimeFiles" else "licenses/ffmpeg/"
            if not destination.startswith(prefix) or destination.casefold() in seen or destination.casefold() == TOOL_MANIFEST.casefold():
                raise ValueError("发布材料位置越界、目标重复或覆盖保留清单")
            seen.add(destination.casefold())
            if not isinstance(record["source"], str) or not record["source"] or not isinstance(record["sha256"], str) or not SHA256.fullmatch(record["sha256"]):
                raise ValueError("发布材料来源路径或SHA-256无效")
            if packaged and record["source"] != destination:
                raise ValueError("成品工具清单未脱敏或来源与包内文件不匹配")
            if group == "runtimeFiles":
                runtime.add(destination)
    if not required.issubset(runtime):
        raise ValueError("工具清单必须按固定小写名称包含对应平台的ffmpeg和ffprobe")


def validate_images_manifest(manifest: dict[str, Any]) -> str | None:
    """统一验证镜像元数据与可选归档，拒绝空值、可变标签及非固定摘要。"""
    required = {"schemaVersion", "appImage", "helperImage"}
    optional = {"archive", "archiveSha256", "collectionImage"}
    if not required.issubset(manifest) or set(manifest) - required - optional or type(manifest["schemaVersion"]) is not int or manifest["schemaVersion"] not in (1, 2, 3):
        raise ValueError("镜像清单字段或版本无效")
    if (manifest["schemaVersion"] == 2 and "collectionImage" not in manifest) or (manifest["schemaVersion"] == 1 and "collectionImage" in manifest):
        raise ValueError("v2清单必须包含获取执行端，v1不接受该字段，v3可选")
    image_keys = ("appImage", "helperImage", "collectionImage") if "collectionImage" in manifest else ("appImage", "helperImage")
    for key in image_keys:
        value = manifest[key]
        if not isinstance(value, str) or len(value) > 512 or not IMAGE_REFERENCE.fullmatch(value):
            raise ValueError("镜像必须使用固定仓库摘要或精确本地sha256 ID")
    if len({manifest[key] for key in image_keys}) != len(image_keys):
        raise ValueError("应用、助手与获取执行端必须使用不同的固定镜像")
    if ("archive" in manifest) != ("archiveSha256" in manifest):
        raise ValueError("离线镜像路径和摘要必须同时提供")
    if "archive" not in manifest:
        return None
    name = relative_path(manifest["archive"])
    checksum = manifest["archiveSha256"]
    if not name.startswith("images/") or not isinstance(checksum, str) or not SHA256.fullmatch(checksum):
        raise ValueError("离线镜像必须位于images子目录并提供有效摘要")
    return name


def copy_verified(source: Path, destination: Path, expected: Any, executable: bool = False) -> None:
    """校验摘要后复制普通文件，并再次核对成品以发现复制中源文件变化。"""
    if source.is_symlink() or not source.is_file() or not isinstance(expected, str) or not SHA256.fullmatch(expected):
        raise ValueError("发布文件或 SHA-256 无效")
    if digest(source) != expected:
        raise ValueError(f"发布材料摘要不匹配：{source.name}")
    destination.parent.mkdir(parents=True, exist_ok=True)
    with source.open("rb") as incoming, destination.open("xb") as outgoing:
        shutil.copyfileobj(incoming, outgoing, 1024 * 1024)
    if digest(destination) != expected:
        raise ValueError("复制后发布材料摘要发生变化")
    destination.chmod(0o755 if executable else 0o644)


def validate_binary(path: Path, target: str) -> None:
    """检查主要工具的 PE/ELF 目标架构，避免将宿主工具误装入其他平台包。"""
    with path.open("rb") as stream:
        validate_binary_stream(stream, target)


def validate_binary_stream(stream: BinaryIO, target: str) -> None:
    """以有界头部读取检查文件或归档内工具，不解压执行未知二进制。"""
    platform, architecture = TARGETS[target]
    header = stream.read(64)
    if platform == "windows":
        if len(header) != 64 or header[:2] != b"MZ":
            raise ValueError("Windows工具不是PE可执行文件")
        offset = struct.unpack_from("<I", header, 60)[0]
        if offset < 64 or offset > 16 * 1024 * 1024:
            raise ValueError("Windows工具PE偏移无效")
        stream.seek(offset)
        if stream.read(6) != b"PE\x00\x00\x64\x86":
            raise ValueError("Windows工具必须为amd64架构")
    else:
        machine = 62 if architecture == "amd64" else 183
        if len(header) < 20 or header[:6] != b"\x7fELF\x02\x01" or struct.unpack_from("<H", header, 18)[0] != machine:
            raise ValueError("Linux工具ELF架构与目标不匹配")


def install_tools(manifest_path: Path, target: str, stage: Path) -> dict[str, str]:
    """按照可信发布清单安装固定工具及其许可和对应源码材料。"""
    manifest = read_json(manifest_path)
    validate_tools_manifest(manifest, target)
    for group in TOOL_GROUPS:
        for record in manifest[group]:
            destination = record["destination"]
            source = Path(record["source"])
            if not source.is_absolute():
                source = manifest_path.parent / source
            suffix = ".exe" if target.startswith("windows-") else ""
            executable = destination in (f"tools/ffmpeg{suffix}", f"tools/ffprobe{suffix}")
            if executable:
                validate_binary(source, target)
            copy_verified(source, stage / destination, record["sha256"], executable)
    # 成品清单保留来源与摘要，但不泄露维护者构建机的绝对目录。
    for group in TOOL_GROUPS:
        for record in manifest[group]:
            record["source"] = record["destination"]
    (stage / TOOL_MANIFEST).write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return {"version": manifest["version"], "sourceUrl": manifest["sourceUrl"]}


def install_images(manifest_path: Path, stage: Path) -> None:
    """校验并安装固定镜像清单，可选复制经过摘要核验的离线镜像归档。"""
    manifest = read_json(manifest_path)
    name = validate_images_manifest(manifest)
    if name is not None:
        copy_verified(manifest_path.parent / name, stage / name, manifest["archiveSha256"])
    (stage / "release-images.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def run(command: list[str], environment: dict[str, str] | None = None) -> None:
    """以参数数组运行构建命令，不经 shell 拼接配置或文件名。"""
    print("执行发布构建：" + command[0], flush=True)
    subprocess.run(resolve_command(command), cwd=ROOT, env=environment, check=True)


def archive_tree(stage: Path, destination: Path, windows: bool) -> None:
    """按固定顺序与时间戳封装发布文件，避免宿主修改时间污染可重现归档。"""
    files = sorted(path for path in stage.rglob("*") if path.is_file())
    if windows:
        with zipfile.ZipFile(destination, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
            for path in files:
                info = zipfile.ZipInfo(f"{stage.name}/{path.relative_to(stage).as_posix()}", date_time=(2020, 1, 1, 0, 0, 0))
                info.compress_type = zipfile.ZIP_DEFLATED
                info.external_attr = (stat.S_IFREG | (0o755 if path.suffix == ".exe" else 0o644)) << 16
                with path.open("rb") as source, archive.open(info, "w", force_zip64=True) as target:
                    shutil.copyfileobj(source, target, 1024 * 1024)
    else:
        with destination.open("wb") as raw, gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=0) as compressed, tarfile.open(fileobj=compressed, mode="w|") as archive:
            for path in files:
                info = archive.gettarinfo(str(path), arcname=f"{stage.name}/{path.relative_to(stage).as_posix()}")
                info.uid = info.gid = 0
                info.uname = info.gname = ""
                info.mtime = 0
                info.mode = 0o755 if path.name in ("nas", "nas-collection-worker", "start.sh", "ffmpeg", "ffprobe") else 0o644
                with path.open("rb") as source:
                    archive.addfile(info, source)


def build(args: argparse.Namespace) -> Path:
    """构建一个目标，完整发布默认要求可核验工具材料，开发包必须显式标识。"""
    platform, architecture = TARGETS[args.target]
    version_match = re.search(r'const Version = "([^"]+)"', (ROOT / "internal/app/config.go").read_text(encoding="utf-8"))
    if version_match is None or not re.fullmatch(r"[0-9A-Za-z][0-9A-Za-z.+_-]{0,63}", version_match[1]):
        raise ValueError("应用版本无效")
    version = version_match[1]
    if not args.tools_manifest and not args.development_without_tools:
        raise ValueError("完整发布需要 --tools-manifest；仅开发验证可明确使用 --development-without-tools")
    if args.tools_manifest and args.development_without_tools:
        raise ValueError("完整工具清单与缺少工具的开发包标识不能同时使用")
    runtime_manifest = getattr(args, "jmcomic_runtime_manifest", None)
    if runtime_manifest:
        from jmcomic_runtime import verify as verify_runtime

        if args.target != "windows-amd64":
            raise ValueError("随包漫画运行时目前仅支持Windows x64")
        verify_runtime(Path(runtime_manifest).resolve())
    uav_manifest = getattr(args, "uav_runtime_manifest", None)
    ytdlp_manifest = getattr(args, "ytdlp_manifest", None)
    if uav_manifest:
        from uav_runtime import verify as verify_uav_runtime

        if args.target != "windows-amd64" or not ytdlp_manifest or not args.tools_manifest or args.development_without_tools:
            raise ValueError("完整视频包仅支持Windows x64，必须同时提供UAV、下载器和FFmpeg材料")
        verify_uav_runtime(Path(uav_manifest).absolute())
    if ytdlp_manifest:
        from ytdlp_runtime import verify as verify_ytdlp_runtime

        if args.target != "windows-amd64":
            raise ValueError("随包固定下载器目前仅支持Windows x64")
        verify_ytdlp_runtime(Path(ytdlp_manifest).absolute())
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    runtime = runtime_directory()
    runtime.mkdir(exist_ok=True)
    lock = runtime / "release-build.lock"
    # 嵌入资源是共享构建输入，串行化发布，禁止清掉其他发布进程的资源。
    lock.mkdir()
    embedded = ROOT / "internal/webassets/dist"
    owned_embedded = False
    try:
        if embedded.exists():
            raise ValueError("发现既有嵌入暂存目录，请确认其他构建已结束后再处理")
        if not args.skip_web_build:
            npm = shutil.which("npm.cmd" if os.name == "nt" else "npm")
            if npm is None:
                raise ValueError("发布构建机缺少 npm，终端用户无需安装")
            run([npm, "--prefix", "web", "ci"])
            run([npm, "--prefix", "web", "run", "build"])
        if not (ROOT / "web/dist/index.html").is_file():
            raise ValueError("构建网页入口缺失，拒绝生成残缺发布包")
        shutil.copytree(ROOT / "web/dist", embedded)
        owned_embedded = True
        environment = dict(os.environ, GOOS=platform, GOARCH=architecture, CGO_ENABLED="0")
        go = args.go or shutil.which("go")
        local_go = ROOT / ".tools/go/bin" / ("go.exe" if os.name == "nt" else "go")
        if not go and local_go.is_file():
            go = str(local_go)
        if not go:
            raise ValueError("发布构建机缺少 Go，终端用户无需安装")
        suffix = ("-jmcomic" if runtime_manifest else "") + ("-uav" if uav_manifest else "-ytdlp" if ytdlp_manifest else "") + ("-development" if args.development_without_tools else "")
        package_name = f"qiku-{version}-{args.target}{suffix}"
        destination = output / (package_name + (".zip" if platform == "windows" else ".tar.gz"))
        if destination.exists():
            raise ValueError("发布归档已存在，拒绝覆盖；请指定新的输出目录")
        with tempfile.TemporaryDirectory(prefix="qiku-release-", dir=runtime) as temporary:
            stage = Path(temporary) / package_name
            stage.mkdir()
            executable = "nas.exe" if platform == "windows" else "nas"
            command = [str(go), "build", "-buildvcs=false", "-trimpath", "-tags", "webembed", "-ldflags=-s -w", "-o", str(stage / executable), "./cmd/nas"]
            run(command, environment)
            worker = "nas-collection-worker.exe" if platform == "windows" else "nas-collection-worker"
            run([str(go), "build", "-buildvcs=false", "-trimpath", "-ldflags=-s -w", "-o", str(stage / worker), "./cmd/nas-collection-worker"], environment)
            if platform == "windows":
                run([str(go), "build", "-buildvcs=false", "-trimpath", "-tags", "webembed", "-ldflags=-s -w -H=windowsgui", "-o", str(stage / "栖库.exe"), "./cmd/nas-desktop"], environment)
            else:
                (stage / "start.sh").write_text('#!/bin/sh\n# 定位程序根并保留现有 NAS_* 环境配置。\nset -eu\napp_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\ncd "$app_dir"\nif [ -z "${NAS_MEDIA_ROOTS:-}" ]; then mkdir -p "$app_dir/media"; fi\nexec "$app_dir/nas" "$@"\n', encoding="utf-8", newline="\n")
            tools = install_tools(Path(args.tools_manifest).resolve(), args.target, stage) if args.tools_manifest else None
            jmcomic_runtime = None
            if runtime_manifest:
                from jmcomic_runtime import install_runtime

                jmcomic_runtime = install_runtime(Path(runtime_manifest).resolve(), args.target, stage)
            uav_runtime = None
            if uav_manifest:
                from uav_runtime import install_runtime as install_uav_runtime

                uav_runtime = install_uav_runtime(Path(uav_manifest).absolute(), args.target, stage)
            yt_dlp = None
            if ytdlp_manifest:
                from ytdlp_runtime import install_runtime as install_ytdlp_runtime

                yt_dlp = install_ytdlp_runtime(Path(ytdlp_manifest).absolute(), args.target, stage)
            if args.images_manifest:
                install_images(Path(args.images_manifest).resolve(), stage)
            for name in ("compose.desktop.yaml", "compose.desktop.collection.yaml", "compose.desktop.components.yaml"):
                if not (ROOT / name).is_file():
                    raise ValueError(f"发布文件缺失：{name}")
                shutil.copyfile(ROOT / name, stage / name)
            for name in ("windows.md", "release.md", "collection.md", "components.md", "jmcomic.md", "jmcomic-windows.md", "uav.md", "uav-windows.md"):
                source = ROOT / "docs" / name
                if source.is_file():
                    (stage / "docs").mkdir(exist_ok=True)
                    shutil.copyfile(source, stage / "docs" / name)
            metadata = {"schemaVersion": 3, "version": version, "target": args.target, "developmentWithoutTools": bool(args.development_without_tools), "tools": tools, "jmcomicRuntime": jmcomic_runtime, "uavRuntime": uav_runtime, "ytDlp": yt_dlp}
            (stage / "release.json").write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
            notice = "Windows：双击栖库.exe。Linux：执行 ./start.sh。\n原生与容器配置数据独立；升级前退出旧版并备份。\nDocker完整模式需要Docker环境和release-images.json，不自动安装Docker/WSL。\n"
            if args.development_without_tools:
                notice += "注意：这是未携带FFmpeg的开发验证包，不是完整离线发布版。\n"
            notice += "已携带固定私有漫画运行时，可在桌面设置中启用托管获取。\n" if jmcomic_runtime else "这是漫画基础包；已发布漫画可读，在线获取需另配受信运行时或外接执行端。\n"
            notice += "已携带固定私有视频运行时、下载器及媒体工具；各插件本地就绪状态独立显示，站点可用性需单独验证。\n" if uav_runtime else "视频在线获取需另配受信运行时或外接执行端。\n"
            if yt_dlp:
                notice += "固定下载器的许可、对应源码库存和构建材料位于licenses/yt-dlp；机器校验不替代人工分发审查。\n"
            (stage / "开始使用.txt").write_text(notice, encoding="utf-8")
            checksums = "".join(f"{digest(path)}  {path.relative_to(stage).as_posix()}\n" for path in sorted(stage.rglob("*")) if path.is_file())
            (stage / "checksums.sha256").write_text(checksums, encoding="utf-8", newline="\n")
            archive_tree(stage, destination, platform == "windows")
        destination.with_name(destination.name + ".sha256").write_text(f"{digest(destination)}  {destination.name}\n", encoding="ascii")
        return destination
    finally:
        if owned_embedded:
            shutil.rmtree(embedded)
        lock.rmdir()


def main() -> int:
    """解析有中文解释的维护者构建参数并输出真实归档位置。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", choices=TARGETS, required=True, help="发布目标操作系统与CPU架构")
    parser.add_argument("--tools-manifest", help="含固定工具版本、每文件摘要、许可证和对应源码材料的JSON清单")
    parser.add_argument("--images-manifest", help="可选的固定应用/助手镜像清单，可含离线镜像归档")
    parser.add_argument("--jmcomic-runtime-manifest", help="可选的已装配Windows x64私有漫画运行时jmcomic-runtime.json绝对路径")
    parser.add_argument("--uav-runtime-manifest", help="可选的已装配Windows x64私有视频运行时清单；必须同时提供下载器和FFmpeg材料")
    parser.add_argument("--ytdlp-manifest", help="可选的固定官方Windows x64下载器及完整许可/对应源码材料成品清单")
    parser.add_argument("--output", default=str(ROOT / "bin/releases"), help="发布归档输出目录，默认bin/releases，不覆盖既有归档")
    parser.add_argument("--go", help="构建机Go可执行文件路径，默认优先PATH其次项目.tools工具链")
    parser.add_argument("--skip-web-build", action="store_true", help="复用已构建的web/dist；正式发布建议不使用")
    parser.add_argument("--development-without-tools", action="store_true", help="仅生成明确标识的开发验证包，缺少FFmpeg，不得当作完整离线版分发")
    args = parser.parse_args()
    try:
        print(f"发布归档已生成：{build(args)}")
        return 0
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"发布构建失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    from project_env import run_cli

    raise SystemExit(run_cli(main))
