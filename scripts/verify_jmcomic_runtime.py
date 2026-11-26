"""交叉验证私有漫画运行时与归档边界；结构夹具不冒充真实Python执行证据。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True

import argparse
import copy
import hashlib
import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any, Callable
from uuid import uuid4

from jmcomic_runtime import LICENSE_ROOT, MANIFEST_NAME, PYTHON_SHA256, PYTHON_VERSION, ROOT, RUNTIME_ROOT, contract, source_url, verify
from project_env import resolve_command, verification_directory
from release import archive_tree, configure_console, digest
from verify_release import verify as verify_release


def write_manifest(root: Path, value: dict[str, Any]) -> Path:
    """保存当前测试清单，字段与换行和构建器保持一致。"""
    path = root / MANIFEST_NAME
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return path


def fixture(root: Path) -> dict[str, Any]:
    """构造最小PE头静态边界夹具，不运行夹具解释器或声称真实依赖验收。"""
    locked = contract()
    binary = b"MZ" + b"\0" * 58 + (64).to_bytes(4, "little") + b"PE\0\0\x64\x86"
    files: list[dict[str, Any]] = []

    def material(name: str, content: bytes, component: str = "python", role: str = "runtime") -> None:
        """生成一个独立测试材料并记录全部长度、角色和摘要。"""
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)
        files.append({"path": name, "sha256": hashlib.sha256(content).hexdigest(), "size": len(content), "role": role, "component": component})

    for name in ("python.exe", "python314.dll"):
        material(f"{RUNTIME_ROOT}/{name}", binary)
    material(f"{RUNTIME_ROOT}/python314.zip", b"static-fixture-not-an-interpreter")
    material(f"{RUNTIME_ROOT}/python314._pth", b"python314.zip\n.\nLib/site-packages\n")
    material(f"{RUNTIME_ROOT}/Lib/site-packages/certifi/cacert.pem", b"static CA fixture", "certifi")
    for component, name in (("curl-cffi", "curl_cffi/_wrapper.pyd"), ("pillow", "PIL/_imaging.pyd"), ("cffi", "_cffi_backend.pyd"), ("pycryptodome", "Crypto/cipher.pyd")):
        material(f"{RUNTIME_ROOT}/Lib/site-packages/{name}", binary, component)
    sources = [{"name": "python", "version": PYTHON_VERSION, "url": source_url(PYTHON_VERSION), "sha256": PYTHON_SHA256}]
    for name, package in locked["packages"].items():
        metadata = f'{name.replace("-", "_")}-{package["version"]}.dist-info/METADATA'
        material(f"{RUNTIME_ROOT}/Lib/site-packages/{metadata}", b"static metadata fixture", name)
        sources.append({"name": name, "version": package["version"], "url": "https://files.pythonhosted.org/packages/static-fixture.whl", "sha256": package["hashes"][0]})
    for source in sources:
        material(f'{LICENSE_ROOT}/{source["name"]}/LICENSE.txt', b"static license fixture", source["name"], "license")
    value = {"schemaVersion": 1, "target": "windows-amd64", "pythonVersion": PYTHON_VERSION, "jmcomicVersion": locked["packages"]["jmcomic"]["version"], "python": f"{RUNTIME_ROOT}/python.exe", "runtimeRoot": RUNTIME_ROOT, "requirementsSha256": locked["requirementsSha256"], "sources": sources, "files": files}
    write_manifest(root, value)
    return value


def build_go_checker(directory: Path) -> Path:
    """生成并构建临时Go交叉校验入口，源文件在检查完成后随临时目录清除。"""
    source = directory / "runtime_checker.go"
    source.write_text('''package main
import ("encoding/json"; "os"; "nas-content-manager/internal/jmcomicruntime")
// main 仅执行本地静态清单校验并以固定结构报告结果。
func main() {
    value, err := jmcomicruntime.Load(os.Args[1])
    if err != nil { json.NewEncoder(os.Stdout).Encode(map[string]string{"error":err.Error()}); os.Exit(2) }
    json.NewEncoder(os.Stdout).Encode(map[string]string{"python":value.Python})
}
''', encoding="utf-8")
    executable = directory / "runtime-checker.exe"
    subprocess.run(resolve_command(["go", "build", "-buildvcs=false", "-o", str(executable), str(source)]), cwd=ROOT, check=True, timeout=120)
    return executable


def seal(stage: Path, archive: Path) -> None:
    """重算双层摘要以独立验证语义边界，防止仅靠校验和失败掩盖漏洞。"""
    checksums = "".join(f"{digest(path)}  {path.relative_to(stage).as_posix()}\n" for path in sorted(stage.rglob("*")) if path.is_file() and path.name != "checksums.sha256")
    (stage / "checksums.sha256").write_text(checksums, encoding="utf-8")
    archive_tree(stage, archive, True)
    archive.with_name(archive.name + ".sha256").write_text(f"{digest(archive)}  {archive.name}\n", encoding="ascii")


def run(manifest: Path | None) -> int:
    """运行Python／Go一致性、路径污染、损坏材料和完整归档静态回归。"""
    if os.name != "nt":
        raise ValueError("本验证需要真实Windows x64；非Windows不得冒充NTFS路径证据")
    count = 0
    with tempfile.TemporaryDirectory(prefix="jmcomic-runtime-", dir=verification_directory()) as temporary:
        base = Path(temporary)
        checker = build_go_checker(base)
        root = base / "中文 带空格运行时"
        root.mkdir()
        value = fixture(root)
        path = root / MANIFEST_NAME

        def go_check(target: Path, accepted: bool) -> None:
            """从任意工作目录和污染环境验证Go加载结果与Python校验一致。"""
            environment = dict(os.environ, PATH="", PYTHONHOME="invalid-pollution", PYTHONPATH="invalid-pollution")
            result = subprocess.run([str(checker), str(target)], cwd=base, env=environment, capture_output=True, timeout=20, check=False)
            expected = 0 if accepted else 2
            if result.returncode != expected:
                raise AssertionError(f"Go静态校验结果不一致：{result.stdout.decode('utf-8', 'replace')}")
            if accepted and json.loads(result.stdout).get("python") != str(target.parent / RUNTIME_ROOT / "python.exe"):
                raise AssertionError("Go未返回规范绝对私有解释器")

        def accepted() -> None:
            """确认同一材料同时通过Python与实际Go静态加载。"""
            nonlocal count
            verify(path)
            go_check(path, True)
            count += 1

        def rejected(action: Callable[[], Any]) -> None:
            """要求Python与Go对相同非法材料均明确拒绝，最后恢复清单。"""
            nonlocal count
            try:
                action()
                try:
                    verify(path)
                except (ValueError, OSError, TypeError):
                    pass
                else:
                    raise AssertionError("Python接受非法运行时")
                go_check(path, False)
                count += 1
            finally:
                write_manifest(root, value)

        def changed(key: str, content: Any) -> None:
            """仅替换一项清单字段，保留其余固定输入。"""
            write_manifest(root, {**copy.deepcopy(value), key: content})

        accepted()
        for key, content in (("schemaVersion", True), ("target", "windows-arm64"), ("pythonVersion", "3.14.4rc1"), ("jmcomicVersion", "0.0.0"), ("requirementsSha256", "a" * 64), ("python", "python.exe"), ("extra", True)):
            rejected(lambda key=key, content=content: changed(key, content))
        rejected(lambda: path.write_text(path.read_text(encoding="utf-8").replace('"schemaVersion": 1', '"schemaVersion": 1, "schemaVersion": 1'), encoding="utf-8"))
        rejected(lambda: path.write_text(path.read_text(encoding="utf-8").replace('"schemaVersion":', '"SchemaVersion":'), encoding="utf-8"))
        rejected(lambda: path.write_text(path.read_text(encoding="utf-8").replace('"size": 70', '"Size": 70'), encoding="utf-8"))
        for name in ("../escape", "runtimes/jmcomic/CON.txt", "runtimes/jmcomic/COM¹.txt", "runtimes/jmcomic/LPT²", "runtimes/jmcomic/CONIN$", "runtimes/jmcomic/NUL .txt", "runtimes/jmcomic/a.", "runtimes/jmcomic/a ", "runtimes/jmcomic/a:b", "runtimes/jmcomic/a\\b", "/absolute", "runtimes/jmcomic/a/../b"):
            records = copy.deepcopy(value["files"])
            records[0]["path"] = name
            rejected(lambda records=records: changed("files", records))
        for key, content in (("size", None), ("size", True), ("component", "unknown"), ("sha256", "a" * 64), ("size", 512 * 1024 * 1024 + 1), ("extra", "invalid")):
            records = copy.deepcopy(value["files"])
            records[0][key] = content
            rejected(lambda records=records: changed("files", records))
        for source_key, content in (("url", "https://files.pythonhosted.org/packages/test.whl?query=1"), ("sha256", "a" * 64), ("version", "0.0.0")):
            sources = copy.deepcopy(value["sources"])
            sources[1][source_key] = content
            rejected(lambda sources=sources: changed("sources", sources))
        records = copy.deepcopy(value["files"])
        records.append({**records[0], "path": records[0]["path"].upper()})
        rejected(lambda: changed("files", records))
        for missing in ("python314.dll", "cacert.pem", "METADATA", "LICENSE.txt", "_wrapper.pyd"):
            records = [record for record in value["files"] if not record["path"].endswith(missing)]
            rejected(lambda records=records: changed("files", records))
        extra = root / RUNTIME_ROOT / "undeclared.dll"
        try:
            rejected(lambda: extra.write_bytes(b"unlisted"))
        finally:
            extra.unlink(missing_ok=True)
        for name, content in (("python.exe", b"wrong architecture"), ("python314._pth", b"python314.zip\n.\nimport site\n")):
            file = root / RUNTIME_ROOT / name
            original = file.read_bytes()
            try:
                records = copy.deepcopy(value["files"])
                record = next(record for record in records if record["path"] == f"{RUNTIME_ROOT}/{name}")
                record.update(sha256=hashlib.sha256(content).hexdigest(), size=len(content))
                file.write_bytes(content)
                rejected(lambda records=records: changed("files", records))
            finally:
                file.write_bytes(original)
        # 无关程序文件不属于运行时白名单，不应误拒绝正常发行包。
        (root / "unrelated-media-tool.exe").write_bytes(b"outside-owned-runtime")
        accepted()
        junction = root / RUNTIME_ROOT / "junction"
        subprocess.run(["cmd.exe", "/d", "/c", "mklink", "/J", str(junction), str(base)], capture_output=True, check=True, timeout=10)
        try:
            rejected(lambda: None)
        finally:
            junction.rmdir()
        accepted()

        # 完整归档使用结构夹具，重算所有摘要后仍须拒绝语义损坏。
        binary = (root / RUNTIME_ROOT / "python.exe").read_bytes()
        for name in ("nas.exe", "栖库.exe", "nas-collection-worker.exe"):
            (root / name).write_bytes(binary)
        for name in ("compose.desktop.yaml", "开始使用.txt"):
            (root / name).write_text("静态发行验证夹具", encoding="utf-8")
        metadata = {"schemaVersion": 2, "version": "fixture-1", "target": "windows-amd64", "developmentWithoutTools": True, "tools": None, "jmcomicRuntime": {"manifest": MANIFEST_NAME, "sha256": digest(path), "pythonVersion": value["pythonVersion"], "jmcomicVersion": value["jmcomicVersion"]}}
        (root / "release.json").write_text(json.dumps(metadata), encoding="utf-8")
        archive = base / "runtime-fixture.zip"
        seal(root, archive)
        verify_release(archive)
        count += 1
        file = root / RUNTIME_ROOT / "python.exe"
        file.write_bytes(b"tampered")
        seal(root, archive)
        try:
            verify_release(archive)
        except ValueError:
            count += 1
        else:
            raise AssertionError("归档接受与运行时清单不符的文件")
        file.write_bytes(binary)
        # 同时重算运行时记录、发布摘要和传输摘要，确认固定路径／架构规则本身仍生效。
        for filename, content in (("python.exe", b"wrong architecture"), ("python314._pth", b"python314.zip\n.\nimport site\n")):
            target = root / RUNTIME_ROOT / filename
            original = target.read_bytes()
            modified = copy.deepcopy(value)
            record = next(item for item in modified["files"] if item["path"] == f"{RUNTIME_ROOT}/{filename}")
            record.update(sha256=hashlib.sha256(content).hexdigest(), size=len(content))
            try:
                target.write_bytes(content)
                write_manifest(root, modified)
                metadata["jmcomicRuntime"]["sha256"] = digest(path)
                (root / "release.json").write_text(json.dumps(metadata), encoding="utf-8")
                seal(root, archive)
                try:
                    verify_release(archive)
                except ValueError:
                    count += 1
                else:
                    raise AssertionError("归档在重算全部摘要后接受错误架构或site配置")
            finally:
                target.write_bytes(original)
                write_manifest(root, value)
        if manifest is not None:
            # 仅此分支才是真实解释器／依赖证据，且复制到中文路径后再运行离线探测。
            real = base / "真实 迁移运行时"
            shutil.copytree(manifest.parent, real)
            real_manifest = real / MANIFEST_NAME
            verify(real_manifest, probe=True)
            go_check(real_manifest, True)
            count += 1
    evidence = verification_directory() / ("jmcomic-runtime-result-" + uuid4().hex)
    evidence.mkdir()
    report = {"checks": count, "realRuntime": manifest is not None, "runtimeManifestSha256": digest(manifest) if manifest else None, "liveSiteTested": False, "cleanWindowsTested": False, "temporaryGoSourceRemoved": True}
    (evidence / "result.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"漫画运行时跨校验回归通过：{count}项，真实依赖={manifest is not None}，结果：{evidence / 'result.json'}")
    return count


def main() -> int:
    """解析带中文解释的回归参数并保留实际失败退出码。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, help="可选的真实Windows x64私有运行时清单；省略仅运行静态夹具")
    args = parser.parse_args()
    try:
        run(args.manifest.resolve() if args.manifest else None)
        return 0
    except (OSError, ValueError, AssertionError, subprocess.SubprocessError) as error:
        print(f"漫画运行时回归失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    from project_env import run_cli

    raise SystemExit(run_cli(main))
