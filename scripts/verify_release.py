"""流式核验发布归档、内外SHA-256、目标结构与工具材料；不运行Docker或解压不可信路径。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    # 直接执行入口时，在其他本地模块导入前关闭源码旁的字节码缓存。
    sys.dont_write_bytecode = True

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile
from pathlib import Path
from typing import Any, BinaryIO, Callable

from release import SHA256, TARGETS, TOOL_GROUPS, TOOL_MANIFEST, archive_tree, configure_console, digest, install_images, install_tools, read_json, relative_path, unique_object, validate_binary, validate_binary_stream, validate_images_manifest, validate_tools_manifest


def metadata_object(data: bytes, limit: int = 1024 * 1024) -> dict[str, Any]:
    """解析有限元数据并拒绝重复字段及非对象值，使校验和构建保持同一解释。"""
    if len(data) > limit:
        raise ValueError("发布元数据过大")
    value = json.loads(data, object_pairs_hook=unique_object)
    if not isinstance(value, dict):
        raise ValueError("发布元数据必须为JSON对象")
    return value


def verify_architectures(path: Path, root: str, target: str, entries: list[str]) -> None:
    """以归档流检查指定固定入口架构，不把归档解压到用户文件系统。"""
    names = [f"{root}/{name}" for name in entries]
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
                    raise ValueError("归档入口无法读取")
                with stream:
                    validate_binary_stream(stream, target)


def verify_jmcomic(path: Path, root: str, metadata: dict[str, Any], small: dict[str, bytes], hashes: dict[str, str], sizes: dict[str, int]) -> None:
    """复用运行时契约校验归档全部依赖、许可、资源和本机架构。"""
    from jmcomic_runtime import LICENSE_ROOT, MANIFEST_LIMIT, MANIFEST_NAME, RUNTIME_ROOT, validate_manifest, verify_stream

    materials = {name for name in hashes if name.casefold().startswith((RUNTIME_ROOT + "/", LICENSE_ROOT + "/")) or name.casefold() == MANIFEST_NAME}
    summary = metadata.get("jmcomicRuntime")
    if summary is None:
        if materials:
            raise ValueError("漫画基础包夹带未声明的运行时材料")
        return
    if metadata["target"] != "windows-amd64" or not isinstance(summary, dict) or set(summary) != {"manifest", "sha256", "pythonVersion", "jmcomicVersion"} or summary["manifest"] != MANIFEST_NAME or MANIFEST_NAME not in small or summary["sha256"] != hashes.get(MANIFEST_NAME):
        raise ValueError("漫画运行时发布摘要或目标无效")
    value = metadata_object(small[MANIFEST_NAME], MANIFEST_LIMIT)
    records = validate_manifest(value)
    if summary["pythonVersion"] != value["pythonVersion"] or summary["jmcomicVersion"] != value["jmcomicVersion"] or materials != {MANIFEST_NAME, *records}:
        raise ValueError("漫画运行时版本、摘要或声明文件集合不一致")
    if path.suffix != ".zip":
        raise ValueError("Windows漫画包必须使用ZIP归档")
    with zipfile.ZipFile(path) as archive:
        for name, record in records.items():
            if hashes.get(name) != record["sha256"] or sizes.get(name) != record["size"]:
                raise ValueError("漫画运行时文件摘要或长度与归档不一致")
            with archive.open(f"{root}/{name}") as stream:
                verify_stream(stream, record, value["pythonVersion"])


def stream_digest(stream: BinaryIO) -> str:
    """分块校验单个归档成员，空间不随归档或镜像大小增长。"""
    value = hashlib.sha256()
    for block in iter(lambda: stream.read(1024 * 1024), b""):
        value.update(block)
    return value.hexdigest()


def verify_uav(path: Path, root: str, metadata: dict[str, Any], small: dict[str, bytes], hashes: dict[str, str], sizes: dict[str, int]) -> None:
    """独立核验视频运行时摘要、固定来源、RECORD与归档成员闭包。"""
    from uav_runtime import LICENSE_ROOT, MANIFEST_LIMIT, MANIFEST_NAME, RUNTIME_ROOT, validate_manifest, validate_record, verify_stream

    materials = {name for name in hashes if name.casefold().startswith((RUNTIME_ROOT + "/", LICENSE_ROOT + "/")) or name.casefold() == MANIFEST_NAME}
    summary = metadata.get("uavRuntime")
    if summary is None:
        if materials:
            raise ValueError("视频基础包夹带未声明的运行时材料")
        return
    if metadata["target"] != "windows-amd64" or not isinstance(summary, dict) or set(summary) != {"manifest", "sha256", "pythonVersion", "uavVersion", "uavCommit"} or summary["manifest"] != MANIFEST_NAME or MANIFEST_NAME not in small or summary["sha256"] != hashes.get(MANIFEST_NAME):
        raise ValueError("视频运行时发布摘要或目标无效")
    value = metadata_object(small[MANIFEST_NAME], MANIFEST_LIMIT)
    records = validate_manifest(value)
    if any(summary[key] != value[key] for key in ("pythonVersion", "uavVersion", "uavCommit")) or materials != {MANIFEST_NAME, *records}:
        raise ValueError("视频运行时版本、来源或声明文件集合不一致")
    if path.suffix != ".zip":
        raise ValueError("Windows视频包必须使用ZIP归档")
    record_names = [name for name in records if name.endswith(f'/uav_downloader-{value["uavVersion"]}.dist-info/RECORD')]
    if len(record_names) != 1 or sizes.get(record_names[0], MANIFEST_LIMIT + 1) > MANIFEST_LIMIT:
        raise ValueError("视频运行时RECORD缺失或超过校验预算")
    with zipfile.ZipFile(path) as archive:
        for name, record in records.items():
            if hashes.get(name) != record["sha256"] or sizes.get(name) != record["size"]:
                raise ValueError("视频运行时文件摘要或长度与归档不一致")
            with archive.open(f"{root}/{name}") as stream:
                verify_stream(stream, record, value["pythonVersion"])
        with archive.open(f"{root}/{record_names[0]}") as stream:
            validate_record(stream.read(MANIFEST_LIMIT + 1), value)


def verify_ytdlp(path: Path, root: str, metadata: dict[str, Any], small: dict[str, bytes], hashes: dict[str, str], sizes: dict[str, int]) -> None:
    """独立核验固定下载器及人工审核材料库存，不与FFmpeg混用清单。"""
    from ytdlp_runtime import EXECUTABLE, INVENTORY, LICENSE_ROOT, MANIFEST_LIMIT, MANIFEST_NAME, VERSION, validate_inventory, validate_manifest, verify_stream

    materials = {name for name in hashes if name.casefold().startswith(LICENSE_ROOT + "/") or name.casefold() == EXECUTABLE}
    summary = metadata.get("ytDlp")
    if summary is None:
        # 旧完整包已由原tools清单逐项核验；其中允许声明额外工具，不能追溯套用schema3独立下载器契约。
        if metadata["schemaVersion"] in (1, 2) and not metadata["developmentWithoutTools"]:
            materials = {name for name in materials if name.casefold() != EXECUTABLE}
        if materials:
            raise ValueError("发布包夹带未声明的固定下载器材料")
        return
    if metadata["target"] != "windows-amd64" or not isinstance(summary, dict) or set(summary) != {"manifest", "sha256", "version"} or summary["manifest"] != MANIFEST_NAME or summary["version"] != VERSION or MANIFEST_NAME not in small or summary["sha256"] != hashes.get(MANIFEST_NAME):
        raise ValueError("下载器发布摘要或目标无效")
    value = metadata_object(small[MANIFEST_NAME], MANIFEST_LIMIT)
    records = validate_manifest(value)
    if materials != {MANIFEST_NAME, *records} or INVENTORY not in small:
        raise ValueError("下载器材料未声明、缺失或夹带额外文件")
    validate_inventory(metadata_object(small[INVENTORY], MANIFEST_LIMIT), value)
    if path.suffix != ".zip":
        raise ValueError("Windows下载器包必须使用ZIP归档")
    with zipfile.ZipFile(path) as archive:
        for name, record in records.items():
            if hashes.get(name) != record["sha256"] or sizes.get(name) != record["size"]:
                raise ValueError("下载器文件摘要或长度与归档不一致")
            with archive.open(f"{root}/{name}") as stream:
                verify_stream(stream, record)


def verify(path: Path) -> int:
    """核对完整包的每一份文件，不接受遗漏、重复路径、链接或未经校验的额外文件。"""
    expected = path.with_name(path.name + ".sha256").read_text(encoding="ascii").strip()
    if expected != f"{digest(path)}  {path.name}":
        raise ValueError("外部归档摘要不匹配")
    hashes: dict[str, str] = {}
    sizes: dict[str, int] = {}
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
        sizes[local] = size
        if local in ("release.json", "checksums.sha256", "release-images.json", TOOL_MANIFEST, "jmcomic-runtime.json", "uav-runtime.json", "licenses/yt-dlp/yt-dlp-manifest.json", "licenses/yt-dlp/source-inventory.json"):
            limit = 4 * 1024 * 1024 if local in ("jmcomic-runtime.json", "uav-runtime.json") else 1024 * 1024
            if size > limit:
                raise ValueError("归档元数据过大")
            data = stream.read(limit + 1)
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
    fields = {"schemaVersion", "version", "target", "developmentWithoutTools", "tools"}
    schema = metadata.get("schemaVersion")
    additions = {"jmcomicRuntime", "uavRuntime", "ytDlp"} if schema == 3 else {"jmcomicRuntime"} if schema == 2 else set()
    if type(schema) is not int or schema not in (1, 2, 3) or set(metadata) != fields | additions or not isinstance(metadata["target"], str) or metadata["target"] not in TARGETS or type(metadata["developmentWithoutTools"]) is not bool or not isinstance(metadata["version"], str) or not re.fullmatch(r"[0-9A-Za-z][0-9A-Za-z.+_-]{0,63}", metadata["version"]):
        raise ValueError("发布元数据无效")
    windows = metadata["target"].startswith("windows-")
    required = {"nas.exe", "栖库.exe"} if windows else {"nas", "start.sh"}
    required |= {"compose.desktop.yaml", "开始使用.txt"}
    if schema >= 2:
        required.add("nas-collection-worker.exe" if windows else "nas-collection-worker")
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
        materials = {name for name in hashes if name.startswith(("tools/", "licenses/ffmpeg/")) and name != TOOL_MANIFEST and not (schema == 3 and metadata["ytDlp"] is not None and name == "tools/yt-dlp.exe")}
        if listed != materials:
            raise ValueError("工具或再分发材料存在未声明文件")
    elif metadata["tools"] is not None:
        raise ValueError("开发验证包不能声明完整工具材料")
    if not required.issubset(hashes):
        raise ValueError("发布包缺少平台入口或必需材料")
    root = next(iter(roots))
    if schema >= 2:
        executables = ["nas.exe", "栖库.exe", "nas-collection-worker.exe"] if windows else ["nas", "nas-collection-worker"]
        verify_architectures(path, root, metadata["target"], executables)
    verify_jmcomic(path, root, metadata, small, hashes, sizes)
    verify_uav(path, root, metadata, small, hashes, sizes)
    verify_ytdlp(path, root, metadata, small, hashes, sizes)
    if metadata.get("uavRuntime") is not None and (metadata.get("ytDlp") is None or metadata["developmentWithoutTools"]):
        raise ValueError("完整视频包必须同时提供私有运行时、固定下载器和媒体工具")
    if not metadata["developmentWithoutTools"]:
        suffix = ".exe" if windows else ""
        verify_architectures(path, root, metadata["target"], [f"tools/{name}{suffix}" for name in ("ffmpeg", "ffprobe")])
    if "release-images.json" in small:
        manifest = metadata_object(small["release-images.json"])
        name = validate_images_manifest(manifest)
        if "collectionImage" in manifest and "compose.desktop.collection.yaml" not in hashes:
            raise ValueError("三镜像发布包缺少获取服务覆盖模板")
        if manifest["schemaVersion"] == 3 and "compose.desktop.components.yaml" not in hashes:
            raise ValueError("v3发布包缺少固定组件部署覆盖模板")
        if name is not None and hashes.get(name) != manifest["archiveSha256"]:
            raise ValueError("离线镜像归档摘要不匹配")
    print(f"发布包校验通过：{metadata['target']}，{len(hashes)}份文件，开发验证包={metadata['developmentWithoutTools']}")
    return len(hashes)


def verify_windows_wrapper(root: Path) -> int:
    """在实际可用的PowerShell5.1/7解析原脚本并验证含中文空格参数的原样透传。"""
    script = Path(__file__).with_name("release-windows.ps1")
    if not script.read_bytes().startswith(b"\xef\xbb\xbf"):
        raise AssertionError("含中文发布脚本必须保留UTF-8 BOM")
    driver = root / "wrapper-check.ps1"
    driver.write_text('''param([string]$SourcePath)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$tokens = $null
$parseErrors = $null
$null = [System.Management.Automation.Language.Parser]::ParseFile($SourcePath, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw ($parseErrors | Out-String) }
# 仅替换构建启动和环境初始化以观察argv；被测试的param声明与透传语句来自原脚本。
$source = [System.IO.File]::ReadAllText($SourcePath)
$source = $source.Replace(". (Join-Path `$PSScriptRoot 'project-env.ps1')", '')
$source = $source.Replace("(Join-Path `$PSScriptRoot 'release.py')", "'release.py'")
$source = $source.Replace('& python @releaseArguments', '[Console]::WriteLine((ConvertTo-Json -InputObject @($releaseArguments) -Compress))')
$source = $source.Replace('exit $LASTEXITCODE', '')
$block = [scriptblock]::Create($source)
& $block -UAVRuntimeManifest 'C:\\中文 空格\\uav-runtime.json' -YtDlpManifest 'C:\\中文 空格\\yt-dlp-manifest.json' -JMComicRuntimeManifest 'C:\\中文 空格\\jmcomic-runtime.json' -ToolsManifest 'C:\\中文 空格\\tools.json' -Output 'C:\\发布 空格' -SkipWebBuild
''', encoding="utf-8-sig")
    expected = ["release.py", "--target", "windows-amd64", "--tools-manifest", "C:\\中文 空格\\tools.json", "--jmcomic-runtime-manifest", "C:\\中文 空格\\jmcomic-runtime.json", "--uav-runtime-manifest", "C:\\中文 空格\\uav-runtime.json", "--ytdlp-manifest", "C:\\中文 空格\\yt-dlp-manifest.json", "--output", "C:\\发布 空格", "--skip-web-build"]
    count = 0
    engines = list({os.path.normcase(path): path for name in ("powershell.exe", "pwsh.exe", "pwsh") if (path := shutil.which(name))}.values())
    for engine in engines:
        result = subprocess.run([engine, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", str(driver), "-SourcePath", str(script)], capture_output=True, check=False, timeout=30)
        if result.returncode or json.loads(result.stdout.decode("utf-8-sig")) != expected:
            raise AssertionError("Windows发布包装脚本解析或参数透传失败：" + Path(engine).name + " " + result.stderr.decode("utf-8", errors="replace")[:1000])
        print("Windows发布脚本解析与中文空格参数透传通过：" + Path(engine).name)
        count += 1
    return count


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

    def console_case(arguments: list[str], encoding: str, expected_code: int, expected_stdout: str = "", expected_stderr: str = "") -> None:
        """在真实非 UTF-8 重定向子进程中检查中文输出与原始退出码。"""
        nonlocal count
        environment = dict(os.environ, PYTHONIOENCODING=encoding, PYTHONUTF8="0")
        result = subprocess.run([sys.executable, *arguments], cwd=Path(__file__).resolve().parent, env=environment, capture_output=True, check=False, timeout=30)
        stdout, stderr = result.stdout.decode("utf-8"), result.stderr.decode("utf-8")
        if result.returncode != expected_code or expected_stdout not in stdout or expected_stderr not in stderr:
            raise AssertionError(f"发布入口未保持 UTF-8 中文输出或退出码：{encoding}，{arguments[0]}")
        if "UnicodeEncodeError" in stderr or "charmap" in stderr:
            raise AssertionError("发布入口仍存在控制台编码异常")
        count += 1

    # 显式覆盖 CI 环境，防止父进程已启用 UTF-8 导致回归虚假通过。
    for encoding in ("cp1252", "ascii"):
        console_case(["-c", "import sys; before = (sys.stdout.encoding, sys.stderr.encoding); import release, verify_release, package_desktop_images, ytdlp_runtime, verify_ytdlp_runtime; assert before == (sys.stdout.encoding, sys.stderr.encoding)"], encoding, 0)
        memory_streams = "import contextlib, io, release\noutput, errors = io.StringIO(), io.StringIO()\nwith contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):\n    release.configure_console()\n    print('中文输出')\nassert output.getvalue() == '中文输出\\n' and errors.getvalue() == ''"
        console_case(["-c", memory_streams], encoding, 0)
        for script, description in (("release.py", "发布目标操作系统与CPU架构"), ("verify_release.py", "归档"), ("package_desktop_images.py", "本机Docker"), ("ytdlp_runtime.py", "下载器"), ("verify_ytdlp_runtime.py", "下载器")):
            console_case([script, "--help"], encoding, 0, expected_stdout=description)
        console_case(["-c", "import release, sys; release.configure_console(); release.run([sys.executable, '-c', 'pass'])"], encoding, 0, expected_stdout="执行发布构建：")
        # 仅替换耗时构建函数，真实执行 main 的成功输出，不下载依赖或制造发布包。
        success = "from pathlib import Path\nfrom unittest.mock import patch\nimport release\nwith patch.object(release, 'build', return_value=Path('发布 输出.zip')):\n    raise SystemExit(release.main())"
        console_case(["-c", success, "--target", "windows-amd64"], encoding, 0, expected_stdout="发布归档已生成：发布 输出.zip")
        console_case(["release.py", "--target", "windows-amd64"], encoding, 1, expected_stderr="发布构建失败：完整发布需要 --tools-manifest")

    for name in ("../escape", "/absolute", "C:/drive", "a\\b", "a//b", "a/./b", "a/../b", "a/line\nb", "a/line\rb", "a/tab\tb", "a/\x7fb", "a/name.", "a/name ", "a/CON", "a/nul.txt", "a/COM1", "a/LPT9.txt", "a/a?b", 'a/a"b'):
        rejected(lambda: relative_path(name), "接受了不安全的包内路径")
    for value in (b"[]", b"null", b'{"version":1,"version":2}'):
        rejected(lambda: metadata_object(value), "归档元数据接受了非对象或重复字段")
    with tempfile.TemporaryDirectory(prefix="qiku-release-check-") as temporary:
        root = Path(temporary)
        count += verify_windows_wrapper(root)
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
        collection_images = {**fixed_images, "schemaVersion": 2, "collectionImage": "sha256:" + "c" * 64}
        if validate_images_manifest(collection_images) is not None:
            raise AssertionError("无归档的三镜像清单解析错误")
        count += 1
        component_images = {**fixed_images, "schemaVersion": 3}
        for value in (component_images, {**collection_images, "schemaVersion": 3}):
            if validate_images_manifest(value) is not None:
                raise AssertionError("v3组件部署清单未兼容双镜像与三镜像")
            count += 1
        for value in ({**component_images, "collectionImage": None}, {**component_images, "collectionImage": ""}, {**component_images, "schemaVersion": 4}):
            rejected(lambda: validate_images_manifest(value), "错误组件部署版本或空镜像未被拒绝")
        for replacement in ({**fixed_images, "schemaVersion": 2}, {**collection_images, "schemaVersion": 1}, {**collection_images, "collectionImage": fixed_images["appImage"]}, {**collection_images, "collectionImage": "collection:latest"}, {**collection_images, "collectionImage": None}):
            rejected(lambda: validate_images_manifest(replacement), "获取执行端版本、镜像身份或不可变引用未被校验")
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
        (stage / "release-images.json").write_text(json.dumps(collection_images), encoding="utf-8")
        seal(stage, second)
        rejected(lambda: verify(second), "三镜像归档缺少覆盖模板仍被接受")
        (stage / "compose.desktop.collection.yaml").write_text("仅用于封装校验", encoding="utf-8")
        seal(stage, second)
        verify(second)
        count += 1
        (stage / "release-images.json").write_text(json.dumps(component_images), encoding="utf-8")
        seal(stage, second)
        rejected(lambda: verify(second), "v3归档缺少组件部署覆盖模板仍被接受")
        (stage / "compose.desktop.components.yaml").write_text("仅用于封装校验", encoding="utf-8")
        seal(stage, second)
        verify(second)
        count += 1
        (stage / "release-images.json").unlink()

        # PE头部只是包校验夹具，不能作为真实工具运行或许可审核证据。
        binary = root / "binary.exe"
        binary.write_bytes(b"MZ" + b"\x00" * 58 + (64).to_bytes(4, "little") + b"PE\x00\x00\x64\x86")
        # schema2必须有真实架构入口，不能把基础包误报为已包含漫画运行时。
        for name in ("nas.exe", "栖库.exe", "nas-collection-worker.exe"):
            (stage / name).write_bytes(binary.read_bytes())
        basic = {**metadata, "schemaVersion": 2, "jmcomicRuntime": None}
        (stage / "release.json").write_text(json.dumps(basic), encoding="utf-8")
        seal(stage, second)
        verify(second)
        count += 1
        (stage / "nas-collection-worker.exe").unlink()
        seal(stage, second)
        rejected(lambda: verify(second), "schema2缺少worker仍被接受")
        (stage / "nas-collection-worker.exe").write_bytes(b"wrong architecture")
        seal(stage, second)
        rejected(lambda: verify(second), "schema2未核验worker架构")
        (stage / "nas-collection-worker.exe").write_bytes(binary.read_bytes())
        schema3 = {**basic, "schemaVersion": 3, "uavRuntime": None, "ytDlp": None}
        (stage / "release.json").write_text(json.dumps(schema3), encoding="utf-8")
        seal(stage, second)
        verify(second)
        count += 1
        for field in ("jmcomicRuntime", "uavRuntime", "ytDlp"):
            incomplete = {key: value for key, value in schema3.items() if key != field}
            (stage / "release.json").write_text(json.dumps(incomplete), encoding="utf-8")
            seal(stage, second)
            rejected(lambda: verify(second), "schema3接受了缺失的可空摘要字段")
        for field in ("uavRuntime", "ytDlp"):
            (stage / "release.json").write_text(json.dumps({**schema3, field: {}}), encoding="utf-8")
            seal(stage, second)
            rejected(lambda: verify(second), "schema3接受了无效插件摘要")
        for base in (metadata, basic, schema3):
            (stage / "release.json").write_text(json.dumps(base), encoding="utf-8")
            for name in ("uav-runtime.json", "licenses/uav/undeclared", "runtimes/uav/undeclared", "tools/yt-dlp.exe", "licenses/yt-dlp/undeclared"):
                extra = stage / name
                extra.parent.mkdir(parents=True, exist_ok=True)
                extra.write_bytes(b"undeclared")
                seal(stage, second)
                rejected(lambda: verify(second), "基础包夹带未声明视频/下载器材料仍被接受")
                extra.unlink()
        (stage / "release.json").write_text(json.dumps(basic), encoding="utf-8")
        (stage / "jmcomic-runtime.json").write_text("{}", encoding="utf-8")
        seal(stage, second)
        rejected(lambda: verify(second), "基础包夹带运行时清单仍被接受")
        (stage / "release.json").write_text(json.dumps({**basic, "jmcomicRuntime": {}}), encoding="utf-8")
        seal(stage, second)
        rejected(lambda: verify(second), "无效运行时摘要仍被接受")
        (stage / "release.json").write_text(json.dumps(metadata), encoding="utf-8")
        seal(stage, second)
        rejected(lambda: verify(second), "旧schema1夹带未声明运行时仍被接受")
        (stage / "jmcomic-runtime.json").unlink()
        (stage / "nas-collection-worker.exe").unlink()
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
        # schema1/2原tools契约允许附带已声明的工具；schema3仍要求独立下载器材料。
        legacy_tools = {**tools, "runtimeFiles": [*tools["runtimeFiles"], {"source": str(binary), "destination": "tools/yt-dlp.exe", "sha256": digest(binary)}]}
        validate_tools_manifest(legacy_tools, "windows-amd64")
        (stage / "tools/yt-dlp.exe").write_bytes(binary.read_bytes())
        legacy_packaged = {**packaged, "runtimeFiles": [*packaged["runtimeFiles"], {"source": "tools/yt-dlp.exe", "destination": "tools/yt-dlp.exe", "sha256": digest(binary)}]}
        for base in (metadata, basic):
            if base["schemaVersion"] == 2:
                (stage / "nas-collection-worker.exe").write_bytes(binary.read_bytes())
            (stage / "release.json").write_text(json.dumps({**base, "developmentWithoutTools": False, "tools": summary}), encoding="utf-8")
            packaged_path.write_text(json.dumps(legacy_packaged), encoding="utf-8")
            seal(stage, second)
            verify(second)
            count += 1
            packaged_path.write_text(json.dumps(packaged), encoding="utf-8")
            seal(stage, second)
            rejected(lambda: verify(second), "旧完整包接受了未被原tools清单声明的下载器")
        packaged_path.write_text(json.dumps(legacy_packaged), encoding="utf-8")
        (stage / "release.json").write_text(json.dumps({**schema3, "developmentWithoutTools": False, "tools": summary}), encoding="utf-8")
        seal(stage, second)
        rejected(lambda: verify(second), "schema3绕过独立下载器契约而沿用旧tools声明")
        (stage / "tools/yt-dlp.exe").unlink()
        (stage / "nas-collection-worker.exe").unlink()
        packaged_path.write_text(json.dumps(packaged), encoding="utf-8")
        (stage / "release.json").write_text(json.dumps({**metadata, "developmentWithoutTools": False, "tools": summary}), encoding="utf-8")
        for replacement in ({**packaged, "sourceFiles": []}, {**packaged, "licenseFiles": []}, {**packaged, "version": "other"}):
            packaged_path.write_text(json.dumps(replacement), encoding="utf-8")
            seal(stage, second)
            rejected(lambda: verify(second), "完整包缺失材料或版本不一致未被拒绝")
        packaged_path.write_text(json.dumps(packaged), encoding="utf-8")
        # 下载器夹具只证明schema3的独立材料绑定，不能证明真实分发许可齐备。
        from verify_ytdlp_runtime import fixture_materials
        from ytdlp_runtime import EXECUTABLE, INVENTORY, LICENSE_ROOT, MANIFEST_NAME, install_runtime as install_ytdlp_runtime
        import ytdlp_runtime

        ytdlp_fixtures = root / "ytdlp-fixtures"
        ytdlp_fixtures.mkdir()
        with fixture_materials(ytdlp_fixtures) as material_path:
            source_manifest = ytdlp_runtime.assemble(material_path, root / "ytdlp-bundle")
            ytdlp_summary = install_ytdlp_runtime(source_manifest.absolute(), "windows-amd64", stage)
            complete3 = {**schema3, "developmentWithoutTools": False, "tools": summary, "ytDlp": ytdlp_summary}
            (stage / "nas-collection-worker.exe").write_bytes(binary.read_bytes())
            (stage / "release.json").write_text(json.dumps(complete3), encoding="utf-8")
            seal(stage, second)
            verify(second)
            count += 1
            for replacement in ({**ytdlp_summary, "version": "latest"}, {**ytdlp_summary, "sha256": "0" * 64}, {**ytdlp_summary, "unexpected": True}, None):
                (stage / "release.json").write_text(json.dumps({**complete3, "ytDlp": replacement}), encoding="utf-8")
                seal(stage, second)
                rejected(lambda: verify(second), "下载器归档摘要或独立声明未被绑定")
            (stage / "release.json").write_text(json.dumps(complete3), encoding="utf-8")
            inventory_path = stage / INVENTORY
            inventory = metadata_object(inventory_path.read_bytes())
            inventory_path.write_text(json.dumps({**inventory, "complete": False}), encoding="utf-8")
            downloader_manifest_path = stage / MANIFEST_NAME
            downloader_manifest = metadata_object(downloader_manifest_path.read_bytes())
            for record in downloader_manifest["sourceFiles"]:
                if record["destination"] == INVENTORY:
                    record["sha256"], record["size"] = digest(inventory_path), inventory_path.stat().st_size
            downloader_manifest_path.write_text(json.dumps(downloader_manifest), encoding="utf-8")
            complete3["ytDlp"]["sha256"] = digest(downloader_manifest_path)
            (stage / "release.json").write_text(json.dumps(complete3), encoding="utf-8")
            seal(stage, second)
            rejected(lambda: verify(second), "重算摘要后仍接受未审核的下载器库存")
        (stage / EXECUTABLE).unlink()
        shutil.rmtree(stage / LICENSE_ROOT)
        (stage / "release.json").write_text(json.dumps({**metadata, "developmentWithoutTools": False, "tools": summary}), encoding="utf-8")
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
            for name in ("nas", "nas-collection-worker"):
                (linux_stage / name).write_bytes(elf)
            (linux_stage / "release.json").write_text(json.dumps({**metadata, "schemaVersion": 2, "target": target, "developmentWithoutTools": False, "tools": summary, "jmcomicRuntime": None}), encoding="utf-8")
            seal(linux_stage, second_tar, False)
            verify(second_tar)
            count += 1
            (linux_stage / "release.json").write_text(json.dumps({**metadata, "schemaVersion": 3, "target": target, "developmentWithoutTools": False, "tools": summary, "jmcomicRuntime": None, "uavRuntime": None, "ytDlp": None}), encoding="utf-8")
            seal(linux_stage, second_tar, False)
            verify(second_tar)
            count += 1
        with zipfile.ZipFile(first, "a") as archive:
            archive.writestr("qiku-test/extra.txt", "unexpected")
        first.with_name(first.name + ".sha256").write_text(f"{digest(first)}  {first.name}\n", encoding="ascii")
        rejected(lambda: verify(first), "未经内部校验的额外文件未被拒绝")
    print(f"发布边界回归通过：{count}项")
    return count


def main() -> int:
    """解析归档校验或构建边界回归模式。"""
    configure_console()
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
    from project_env import run_cli

    raise SystemExit(run_cli(main))
