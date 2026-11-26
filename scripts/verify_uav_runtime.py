"""交叉验证视频私有运行时的Go/Python静态边界与真实离线依赖，夹具不冒充成品。"""

from __future__ import annotations

import sys

sys.dont_write_bytecode = True

import argparse
import base64
import copy
import csv
import hashlib
import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any, Callable
from uuid import uuid4

from project_env import resolve_command, verification_directory
from release import configure_console, digest
from uav_runtime import LICENSE_ROOT, MANIFEST_NAME, PYTHON_SHA256, PYTHON_VERSION, ROOT, RUNTIME_ROOT, contract, source_url, uav_source_url, verify


def write_manifest(root: Path, value: dict[str, Any]) -> Path:
    """保存夹具当前清单，使用与装配器相同的编码和换行。"""
    path = root / MANIFEST_NAME
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return path


def fixture(root: Path) -> dict[str, Any]:
    """构造仅供静态校验的最小PE与来源夹具，不执行其中的虚拟解释器。"""
    locked = contract()
    binary = b"MZ" + b"\0" * 58 + (64).to_bytes(4, "little") + b"PE\0\0\x64\x86"
    files: list[dict[str, Any]] = []

    def material(name: str, content: bytes, component: str = "python", role: str = "runtime") -> None:
        """生成一份自有夹具材料，记录完整文件身份。"""
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)
        files.append({"path": name, "sha256": hashlib.sha256(content).hexdigest(), "size": len(content), "component": component, "role": role})

    for name in ("python.exe", "python314.dll"):
        material(f"{RUNTIME_ROOT}/{name}", binary)
    material(f"{RUNTIME_ROOT}/python314.zip", b"static-standard-library-fixture")
    material(f"{RUNTIME_ROOT}/python314._pth", b"python314.zip\n.\nLib/site-packages\n")
    material(f"{RUNTIME_ROOT}/Lib/site-packages/certifi/cacert.pem", b"static CA fixture", "certifi")
    for component, name in (("curl-cffi", "curl_cffi/_wrapper.pyd"), ("cffi", "_cffi_backend.pyd"), ("pycryptodome", "Crypto/cipher.pyd")):
        material(f"{RUNTIME_ROOT}/Lib/site-packages/{name}", binary, component)
    sources = [{"name": "python", "version": PYTHON_VERSION, "url": source_url(PYTHON_VERSION), "sha256": PYTHON_SHA256}, {"name": "uav-downloader", "version": locked["uavVersion"], "url": uav_source_url(), "sha256": locked["uavSourceSha256"]}]
    for name, package in locked["packages"].items():
        material(f'{RUNTIME_ROOT}/Lib/site-packages/{name.replace("-", "_")}-{package["version"]}.dist-info/METADATA', b"static metadata fixture", name)
        sources.append({"name": name, "version": package["version"], "url": "https://files.pythonhosted.org/packages/static-fixture.whl", "sha256": package["hashes"][0]})
    metadata = f'{RUNTIME_ROOT}/Lib/site-packages/uav_downloader-{locked["uavVersion"]}.dist-info/'
    material(metadata + "METADATA", b"static upstream metadata", "uav-downloader")
    material(metadata + "direct_url.json", json.dumps({"url": uav_source_url(), "archive_info": {"hashes": {"sha256": locked["uavSourceSha256"]}}}).encode(), "uav-downloader")
    material(metadata + "RECORD", b"", "uav-downloader")
    material(f"{RUNTIME_ROOT}/Lib/site-packages/uav_downloader/__init__.py", b"# static source fixture\n", "uav-downloader")
    for source in sources:
        material(f'{LICENSE_ROOT}/{source["name"]}/LICENSE.txt', b"static license fixture", source["name"], "license")
    value = {"schemaVersion": 1, "target": "windows-amd64", "pythonVersion": PYTHON_VERSION, "uavVersion": locked["uavVersion"], "uavCommit": locked["uavCommit"], "python": f"{RUNTIME_ROOT}/python.exe", "runtimeRoot": RUNTIME_ROOT, "requirementsSha256": locked["requirementsSha256"], "buildRequirementsSha256": locked["buildRequirementsSha256"], "builtWheelSha256": "a" * 64, "sources": sources, "files": files}
    seal_record(root, value)
    write_manifest(root, value)
    return value


def seal_record(root: Path, value: dict[str, Any]) -> None:
    """重算安装记录及清单摘要，让恶例独立验证语义而非只触发外层摘要失败。"""
    prefix = RUNTIME_ROOT + "/Lib/site-packages/"
    name = prefix + f'uav_downloader-{value["uavVersion"]}.dist-info/RECORD'
    with (root / name).open("w", encoding="utf-8", newline="") as output:
        writer = csv.writer(output, lineterminator="\n")
        for record in value["files"]:
            if record["component"] != "uav-downloader" or record["role"] != "runtime":
                continue
            path = root / record["path"]
            local = record["path"][len(prefix):]
            checksum = "sha256=" + base64.urlsafe_b64encode(bytes.fromhex(digest(path))).decode("ascii").rstrip("=") if record["path"] != name else ""
            writer.writerow([local, checksum, str(path.stat().st_size) if record["path"] != name else ""])
    for record in value["files"]:
        path = root / record["path"]
        record.update(sha256=digest(path), size=path.stat().st_size)


def build_go_checker(directory: Path) -> Path:
    """生成临时Go入口核验实际加载器，源文件随自有临时目录清理。"""
    source = directory / "runtime_checker.go"
    source.write_text('''package main
import("encoding/json";"os";"nas-content-manager/internal/uavruntime")
// main 只检查本地运行时清单，不运行解释器或访问站点。
func main(){value,err:=uavruntime.Load(os.Args[1]);if err!=nil{json.NewEncoder(os.Stdout).Encode(map[string]string{"error":err.Error()});os.Exit(2)};json.NewEncoder(os.Stdout).Encode(map[string]string{"python":value.Python,"commit":value.UAVCommit})}
''', encoding="utf-8")
    executable = directory / "runtime-checker.exe"
    subprocess.run(resolve_command(["go", "build", "-buildvcs=false", "-o", str(executable), str(source)]), cwd=ROOT, check=True, timeout=120)
    return executable


def run(manifest: Path | None) -> int:
    """运行两种实现的一致性、重算摘要恶例、重解析及可选真实依赖验证。"""
    if os.name != "nt":
        raise ValueError("本验证需要真实Windows x64，不将跨平台编译冒充NTFS证据")
    count = 0
    with tempfile.TemporaryDirectory(prefix="uav-runtime-", dir=verification_directory()) as temporary:
        base = Path(temporary)
        checker = build_go_checker(base)
        root = base / "中文 空格运行时"
        root.mkdir()
        value = fixture(root)
        path = root / MANIFEST_NAME

        def go_check(target: Path, expected: int) -> None:
            """从不同CWD及污染Python环境调用真实Go加载器，检查规范绝对路径。"""
            result = subprocess.run([str(checker), str(target)], cwd=base, env=dict(os.environ, PATH="", PYTHONHOME="invalid", PYTHONPATH="invalid"), capture_output=True, timeout=30, check=False)
            if result.returncode != expected:
                raise AssertionError("Go运行时检查结果不符：" + result.stdout.decode("utf-8", "replace"))
            if expected == 0 and json.loads(result.stdout).get("python") != str(target.parent / RUNTIME_ROOT / "python.exe"):
                raise AssertionError("Go运行时未返回规范绝对解释器")

        def accepted() -> None:
            """要求静态夹具同时通过Python与Go，不运行虚拟解释器。"""
            nonlocal count
            verify(path)
            go_check(path, 0)
            count += 1

        def rejected(action: Callable[[], Any]) -> None:
            """要求非法材料被两种实现共同拒绝，完成后恢复原清单。"""
            nonlocal count
            try:
                action()
                try:
                    verify(path)
                except (ValueError, OSError, TypeError, KeyError):
                    pass
                else:
                    raise AssertionError("Python接受了非法视频运行时")
                go_check(path, 2)
                count += 1
            finally:
                write_manifest(root, value)

        accepted()
        for key in value:
            rejected(lambda key=key: write_manifest(root, {name: item for name, item in value.items() if name != key}))
            rejected(lambda key=key: write_manifest(root, {**value, key: None}))
        for key, content in (("schemaVersion", True), ("target", "windows-arm64"), ("pythonVersion", "3.14.4rc1"), ("uavVersion", "0.0.0"), ("uavCommit", "0" * 40), ("requirementsSha256", "a" * 64), ("buildRequirementsSha256", "b" * 64), ("builtWheelSha256", "invalid"), ("python", "python.exe"), ("extra", True)):
            rejected(lambda key=key, content=content: write_manifest(root, {**value, key: content}))
        for index, key, content in ((0, "url", "https://example.invalid/python.zip"), (1, "sha256", "b" * 64), (2, "version", "0.0.0"), (2, "sha256", "b" * 64), (2, "url", "https://files.pythonhosted.org/packages/a.whl?other=1"), (2, "extra", "invalid")):
            sources = copy.deepcopy(value["sources"])
            sources[index][key] = content
            rejected(lambda sources=sources: write_manifest(root, {**value, "sources": sources}))
        rejected(lambda: write_manifest(root, {**value, "sources": [*value["sources"][:-1], value["sources"][0]]}))
        rejected(lambda: path.write_text(path.read_text(encoding="utf-8").replace('"schemaVersion": 1', '"schemaVersion":1,"schemaVersion":1'), encoding="utf-8"))
        rejected(lambda: write_manifest(root, {**value, "files": value["files"] + [{**value["files"][0], "path": value["files"][0]["path"].upper()}]}))
        for name in ("../escape", "runtimes/uav/CON.txt", "runtimes/uav/COM¹.txt", "runtimes/uav/a.", "runtimes/uav/a ", "runtimes/uav/a:b", "/absolute"):
            records = copy.deepcopy(value["files"])
            records[0]["path"] = name
            rejected(lambda records=records: write_manifest(root, {**value, "files": records}))
        for missing in ("python314.dll", "cacert.pem", "METADATA", "LICENSE.txt", "_wrapper.pyd", "direct_url.json", "RECORD"):
            rejected(lambda missing=missing: write_manifest(root, {**value, "files": [record for record in value["files"] if not record["path"].endswith(missing)]}))
        for filename, content in (("python.exe", b"not PE"), ("python314._pth", b"python314.zip\n.\nimport site\n")):
            file = root / RUNTIME_ROOT / filename
            original = file.read_bytes()
            try:
                file.write_bytes(content)
                changed = copy.deepcopy(value)
                seal_record(root, changed)
                rejected(lambda changed=changed: write_manifest(root, changed))
            finally:
                file.write_bytes(original)
                seal_record(root, value)
                write_manifest(root, value)
        metadata = root / RUNTIME_ROOT / "Lib/site-packages" / f'uav_downloader-{value["uavVersion"]}.dist-info'
        provenance = metadata / "direct_url.json"
        original = provenance.read_bytes()
        for bad in ({"url": "file:///C:/private/build/source.tar.gz", "archive_info": {"hashes": {"sha256": contract()["uavSourceSha256"]}}}, {"url": uav_source_url(), "archive_info": {"hashes": {"sha256": "a" * 64}}}):
            try:
                provenance.write_text(json.dumps(bad), encoding="utf-8")
                changed = copy.deepcopy(value)
                seal_record(root, changed)
                rejected(lambda changed=changed: write_manifest(root, changed))
            finally:
                provenance.write_bytes(original)
                seal_record(root, value)
                write_manifest(root, value)
        record_path = metadata / "RECORD"
        original = record_path.read_bytes()
        for content in (b"../outside,sha256=bad,1\n", b"", original + original, original.replace(b"sha256=", b"sha512=", 1)):
            try:
                record_path.write_bytes(content)
                changed = copy.deepcopy(value)
                next(record for record in changed["files"] if record["path"].endswith("/RECORD")).update(sha256=digest(record_path), size=record_path.stat().st_size)
                rejected(lambda changed=changed: write_manifest(root, changed))
            finally:
                record_path.write_bytes(original)
        extra = root / RUNTIME_ROOT / "unlisted.dll"
        try:
            rejected(lambda: extra.write_bytes(b"unlisted"))
        finally:
            extra.unlink(missing_ok=True)
        junction = root / RUNTIME_ROOT / "junction"
        subprocess.run(["cmd.exe", "/d", "/c", "mklink", "/J", str(junction), str(base)], capture_output=True, check=True, timeout=10)
        try:
            rejected(lambda: None)
        finally:
            junction.rmdir()
        accepted()
        real_files: int | None = None
        if manifest is not None:
            moved = base / "真实 中文空格运行时"
            shutil.copytree(manifest.parent, moved)
            moved_manifest = moved / MANIFEST_NAME
            real = verify(moved_manifest, probe=True)
            go_check(moved_manifest, 0)
            verify(moved_manifest)
            real_files = len(real["files"])
            print(f'真实视频运行时：Python {real["pythonVersion"]}，UAV {real["uavVersion"]}，{len(real["files"])}份文件', flush=True)
            count += 1
    evidence = verification_directory() / ("uav-runtime-result-" + uuid4().hex)
    evidence.mkdir()
    report = {"checks": count, "realRuntime": manifest is not None, "runtimeManifestSha256": digest(manifest) if manifest else None, "realRuntimeFiles": real_files, "movedChineseSpacePath": manifest is not None, "pollutedPythonEnvironment": manifest is not None, "offlineOnly": True, "liveSiteTested": False, "cleanWindowsTested": False, "temporaryGoSourceRemoved": True}
    (evidence / "result.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"视频运行时交叉验证通过：{count}项；结果：{evidence / 'result.json'}", flush=True)
    return 0


def main() -> int:
    """读取中文说明参数并执行本地验证，不接收站点或运行脚本参数。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, help="可选的真实uav-runtime.json绝对路径，额外执行隔离依赖probe")
    args = parser.parse_args()
    return run(args.manifest)


if __name__ == "__main__":
    from project_env import run_cli

    raise SystemExit(run_cli(main))
