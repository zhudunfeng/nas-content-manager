"""在构建机装配和校验Windows私有漫画运行时；终端不联网安装、不修改全局Python。"""

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
import urllib.parse
import urllib.request
import zipfile
from email.parser import BytesParser
from pathlib import Path
from typing import Any

from runtime_common import read_manifest, ordinary, verify_stream, download, extract
from project_env import resolve_command, runtime_directory
from release import ROOT, SHA256, configure_console, copy_verified, digest, relative_path, unique_object


MANIFEST_NAME = "jmcomic-runtime.json"  # 程序根中的固定漫画运行时清单名。
RUNTIME_ROOT = "runtimes/jmcomic"  # 运行时只读文件的固定包内根。
LICENSE_ROOT = "licenses/jmcomic"  # 独立保存全部组件许可的包内根。
MANIFEST_LIMIT = 4 * 1024 * 1024  # 清单最多4MiB，文件记录最多一万条。
PYTHON_VERSION = "3.14.4"  # 首个与真实锁定依赖一起验证的CPython补丁版本，不追踪最新版。
PYTHON_SHA256 = "cda80a9b1e75c0f1b4f9872ca1b417f0d19bce32facc811aea9180e70fad5fb9"  # Python官方对应zip.sigstore中公布的SHA2_256；不表示本脚本验证了Sigstore签名。
VERSION = re.compile(r"3\.(11|12|13|14)\.[0-9]{1,3}\Z")  # 只接纳现有wheel锁覆盖的正式CPython系列。
MAX_FILE = 512 * 1024 * 1024  # 单文件最多512MiB，防止异常压缩归档扩张。
MAX_TOTAL = 1024 * 1024 * 1024  # 一套漫画运行材料最多1GiB。


def contract() -> dict[str, Any]:
    """读取Go和构建器共用契约，统一LF换行后确认requirements未漂移。"""
    value = read_manifest(ROOT / "internal/jmcomicruntime/contract.json")
    requirements = (ROOT / "deploy/jmcomic-requirements.txt").read_bytes().replace(b"\r\n", b"\n")
    if hashlib.sha256(requirements).hexdigest() != value["requirementsSha256"]:
        raise ValueError("漫画requirements已改变，须审查并同步编译期运行时契约后再构建")
    return value


def source_url(version: str) -> str:
    """生成唯一的Python官方Windows x64 embeddable来源，不接受镜像地址替换。"""
    if not VERSION.fullmatch(version):
        raise ValueError("Python版本必须为受支持系列的正式补丁版本")
    return f"https://www.python.org/ftp/python/{version}/python-{version}-embed-amd64.zip"


def validate_manifest(value: dict[str, Any]) -> dict[str, dict[str, Any]]:
    """统一静态清单契约，供本地校验、发行复制和归档校验复用。"""
    locked = contract()
    fields = {"schemaVersion", "target", "pythonVersion", "jmcomicVersion", "python", "runtimeRoot", "requirementsSha256", "sources", "files"}
    if set(value) != fields or type(value["schemaVersion"]) is not int or value["schemaVersion"] != 1 or value["target"] != "windows-amd64":
        raise ValueError("漫画运行时清单字段、版本或目标无效")
    if not isinstance(value["pythonVersion"], str) or not VERSION.fullmatch(value["pythonVersion"]) or value["jmcomicVersion"] != locked["packages"]["jmcomic"]["version"] or value["requirementsSha256"] != locked["requirementsSha256"]:
        raise ValueError("漫画运行时版本或依赖锁与当前程序不一致")
    if value["runtimeRoot"] != RUNTIME_ROOT or value["python"] != f"{RUNTIME_ROOT}/python.exe":
        raise ValueError("漫画解释器不在固定包内位置")
    sources = value["sources"]
    if not isinstance(sources, list) or len(sources) != len(locked["packages"]) + 1:
        raise ValueError("漫画运行时来源闭包不完整")
    components: set[str] = set()
    for source in sources:
        if not isinstance(source, dict) or set(source) != {"name", "version", "url", "sha256"} or not all(isinstance(item, str) for item in source.values()) or source["name"] in components or not SHA256.fullmatch(source["sha256"]):
            raise ValueError("漫画运行时来源字段、重复组件或摘要无效")
        name = source["name"]
        if name == "python":
            if source["version"] != value["pythonVersion"] or source["url"] != source_url(value["pythonVersion"]):
                raise ValueError("私有Python来源与版本不匹配")
        else:
            entry = locked["packages"].get(name)
            url = urllib.parse.urlsplit(source["url"])
            if entry is None or source["version"] != entry["version"] or source["sha256"] not in entry["hashes"] or url.scheme != "https" or url.netloc != "files.pythonhosted.org" or not url.path.startswith("/packages/") or url.query or url.fragment:
                raise ValueError("漫画wheel来源、版本或摘要不在固定白名单")
        components.add(name)
    if components != {"python", *locked["packages"]}:
        raise ValueError("漫画来源组件缺失")
    files = value["files"]
    if not isinstance(files, list) or not 1 <= len(files) <= 10000:
        raise ValueError("漫画运行文件数必须在1至10000之间")
    records: dict[str, dict[str, Any]] = {}
    folded: set[str] = set()
    licenses: set[str] = set()
    total = 0
    for record in files:
        if not isinstance(record, dict) or set(record) != {"path", "sha256", "size", "role", "component"}:
            raise ValueError("漫画运行时文件字段无效")
        name = relative_path(record["path"])
        if not isinstance(record["sha256"], str) or not SHA256.fullmatch(record["sha256"]) or type(record["size"]) is not int or not 0 <= record["size"] <= MAX_FILE or not isinstance(record["component"], str) or record["component"] not in components or name.casefold() in folded:
            raise ValueError("漫画运行文件摘要、长度、组件或重复路径无效")
        if record["role"] == "runtime":
            if not name.startswith(RUNTIME_ROOT + "/"):
                raise ValueError("漫画运行文件位置越界")
        elif record["role"] == "license":
            if not name.startswith(f'{LICENSE_ROOT}/{record["component"]}/') or record["size"] == 0:
                raise ValueError("漫画许可材料位置或长度无效")
            licenses.add(record["component"])
        else:
            raise ValueError("漫画文件角色无效")
        if name.lower().endswith((".pth", ".pyc")):
            raise ValueError("私有运行时不得夹带额外site配置或生成缓存")
        total += record["size"]
        folded.add(name.casefold())
        records[name] = record
    if total > MAX_TOTAL or licenses != components:
        raise ValueError("漫画材料超过整体预算或缺少组件许可")
    abi = "".join(value["pythonVersion"].split(".")[:2])
    required = {f"{RUNTIME_ROOT}/{name}" for name in ("python.exe", f"python{abi}.dll", f"python{abi}.zip", f"python{abi}._pth", "Lib/site-packages/certifi/cacert.pem")}
    required |= {f'{RUNTIME_ROOT}/Lib/site-packages/{name.replace("-", "_")}-{entry["version"]}.dist-info/METADATA' for name, entry in locked["packages"].items()}
    if not required.issubset(records):
        raise ValueError("漫画运行时缺少解释器、隔离路径、证书或依赖元数据")
    for prefix in ("curl_cffi/_wrapper", "PIL/_imaging", "_cffi_backend", "Crypto/"):
        if not any(name.startswith(f"{RUNTIME_ROOT}/Lib/site-packages/{prefix}") and name.endswith(".pyd") for name in records):
            raise ValueError("漫画运行时缺少必要本机扩展")
    return records


def verify(manifest_path: Path, probe: bool = False) -> dict[str, Any]:
    """校验完整私有运行时；可选执行固定离线依赖探测，不访问漫画站点。"""
    if manifest_path.name != MANIFEST_NAME or not manifest_path.is_absolute() or str(manifest_path) != os.path.abspath(manifest_path) or os.path.normcase(str(manifest_path.resolve())) != os.path.normcase(str(manifest_path)):
        raise ValueError("漫画运行时清单必须是固定名称的绝对路径")
    value = read_manifest(manifest_path)
    records = validate_manifest(value)
    root = manifest_path.parent
    actual: set[str] = set()
    folded: set[str] = set()
    for directory in (root / RUNTIME_ROOT, root / LICENSE_ROOT):
        if not directory.is_dir():
            raise ValueError("漫画运行时或许可目录缺失")
        for base, directories, files in os.walk(directory, followlinks=False):
            for name in directories:
                information = (Path(base) / name).lstat()
                if stat.S_ISLNK(information.st_mode) or getattr(information, "st_file_attributes", 0) & 0x400:
                    raise ValueError("漫画运行目录包含链接或重解析点")
            for name in files:
                path = Path(base) / name
                ordinary(path)
                local = path.relative_to(root).as_posix()
                if local not in records or local.casefold() in folded:
                    raise ValueError("漫画运行时含未声明文件或路径别名")
                with path.open("rb") as stream:
                    verify_stream(stream, records[local], value["pythonVersion"])
                actual.add(local)
                folded.add(local.casefold())
    if actual != set(records):
        raise ValueError("漫画运行时缺少声明文件")
    if probe:
        run_probe(root / value["python"], value)
    return value


def run_probe(python: Path, value: dict[str, Any]) -> None:
    """在隔离模式验证真实扩展、版本、证书和图片编码解码，禁止网络探测。"""
    expected = {name: entry["version"] for name, entry in contract()["packages"].items()}
    code = """import io,json,socket,sys
from importlib.metadata import version
def deny_network(*args, **kwargs):
    raise RuntimeError('离线漫画运行时探测禁止网络')
socket.create_connection = deny_network
socket.socket.connect = deny_network
import jmcomic,certifi,curl_cffi,cffi,yaml,pycparser
from Crypto.Cipher import AES
from PIL import Image
expected=json.loads(sys.argv[1])
assert {name:version(name) for name in expected}==expected
assert sys.version.split()[0]==sys.argv[2] and sys.maxsize>2**32
assert sys.flags.isolated and sys.flags.utf8_mode and sys.dont_write_bytecode
with open(certifi.where(),'rb') as certificate:
    assert b'BEGIN CERTIFICATE' in certificate.read()
output=io.BytesIO()
Image.new('RGB',(3,2),(32,64,128)).save(output,format='PNG')
assert Image.open(io.BytesIO(output.getvalue())).size==(3,2)
assert len(AES.new(b'0123456789abcdef', AES.MODE_ECB).encrypt(b'0123456789abcdef'))==16
assert yaml.safe_load('ready: true')['ready'] is True
print(json.dumps({'ready':True,'pythonVersion':sys.version.split()[0],'packages':expected},ensure_ascii=True))
"""
    environment = dict(os.environ, PYTHONHOME="invalid-runtime-pollution", PYTHONPATH="invalid-runtime-pollution")
    result = subprocess.run([str(python), "-I", "-B", "-X", "utf8", "-c", code, json.dumps(expected), value["pythonVersion"]], cwd=runtime_directory(), env=environment, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30, check=False)
    if result.returncode != 0 or len(result.stdout) > 65536 or len(result.stderr) > 65536:
        # 维护者探测错误不输出原始依赖stderr，避免把本机或网络私密信息写入交付日志。
        raise ValueError("私有漫画运行时真实依赖探测失败，请检查Python ABI、VC运行库及wheel闭包")
    response = json.loads(result.stdout)
    if response.get("ready") is not True or response.get("packages") != expected:
        raise ValueError("私有漫画运行时探测响应不符合固定版本")
    print("私有漫画运行时离线探测通过：" + value["pythonVersion"], flush=True)


def wheel_sources(wheelhouse: Path, version: str, offline: bool) -> list[dict[str, str]]:
    """下载哈希锁定wheel并保存可供离线重建的官方来源记录。"""
    locked = contract()
    if not offline:
        wheelhouse.mkdir(parents=True, exist_ok=True)
        abi = "cp" + "".join(version.split(".")[:2])
        command = [sys.executable, "-I", "-B", "-m", "pip", "download", "--disable-pip-version-check", "--only-binary=:all:", "--no-deps", "--require-hashes", "--platform", "win_amd64", "--python-version", ".".join(version.split(".")[:2]), "--implementation", "cp", "--abi", abi, "--index-url", "https://pypi.org/simple", "--dest", str(wheelhouse), "-r", str(ROOT / "deploy/jmcomic-requirements.txt")]
        print("下载固定哈希的漫画Windows wheel闭包", flush=True)
        subprocess.run(resolve_command(command), cwd=ROOT, check=True)
    artifacts: dict[str, dict[str, str]] = {}
    stored_path = wheelhouse / "wheel-sources.json"
    if offline:
        stored = read_manifest(stored_path)
        if set(stored) != {"sources"} or not isinstance(stored["sources"], list):
            raise ValueError("离线wheel来源清单格式无效")
        for source in stored["sources"]:
            if not isinstance(source, dict) or set(source) != {"name", "version", "url", "sha256", "filename"}:
                raise ValueError("离线wheel来源字段无效")
            artifacts[source["sha256"]] = source
    result: list[dict[str, str]] = []
    for name, entry in locked["packages"].items():
        candidates = [path for path in wheelhouse.glob("*.whl") if digest(path) in entry["hashes"]]
        if len(candidates) != 1:
            raise ValueError(f"wheel目录中{name}必须恰有一份目标平台锁定材料")
        wheel = candidates[0]
        checksum = digest(wheel)
        if offline:
            source = artifacts.get(checksum)
            if source is None or source["name"] != name or source["version"] != entry["version"] or source["filename"] != wheel.name:
                raise ValueError("离线wheel缺少一致的固定来源记录")
        else:
            url = f'https://pypi.org/pypi/{name}/{entry["version"]}/json'
            with urllib.request.urlopen(url, timeout=30) as stream:
                data = json.loads(stream.read(4 * 1024 * 1024 + 1))
            matches = [item for item in data["urls"] if item["filename"] == wheel.name and item["digests"]["sha256"] == checksum]
            if len(matches) != 1:
                raise ValueError("wheel与官方PyPI版本记录不一致")
            source = {"name": name, "version": entry["version"], "url": matches[0]["url"], "sha256": checksum, "filename": wheel.name}
        result.append(source)
    if not offline:
        stored_path.write_text(json.dumps({"sources": result}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return result


def install_runtime(manifest_path: Path, target: str, stage: Path) -> dict[str, str]:
    """把已校验运行时复制进发行暂存根，并返回脱敏的固定版本摘要。"""
    if target != "windows-amd64":
        raise ValueError("随包漫画运行时目前仅支持Windows x64")
    value = verify(manifest_path)
    for record in value["files"]:
        copy_verified(manifest_path.parent / record["path"], stage / record["path"], record["sha256"], record["path"].lower().endswith(".exe"))
    destination = stage / MANIFEST_NAME
    destination.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    verify(destination.resolve())
    return {"manifest": MANIFEST_NAME, "sha256": digest(destination), "pythonVersion": value["pythonVersion"], "jmcomicVersion": value["jmcomicVersion"]}


def assemble(args: argparse.Namespace) -> Path:
    """组装全新的私有解释器闭包，验证后原子发布，失败不覆盖既有运行时。"""
    version: str = args.python_version
    expected: str = args.python_sha256
    url = source_url(version)
    if not SHA256.fullmatch(expected):
        raise ValueError("Python归档SHA256无效")
    if version != PYTHON_VERSION and expected == PYTHON_SHA256:
        raise ValueError("更换Python版本时必须显式提供对应官方归档SHA256")
    destination = Path(args.output).absolute()
    if destination.exists():
        raise ValueError("运行时输出目录已存在，拒绝覆盖；请使用全新目录")
    destination.parent.mkdir(parents=True, exist_ok=True)
    cache = runtime_directory() / "tools/jmcomic-inputs"
    cache.mkdir(parents=True, exist_ok=True)
    archive = Path(args.python_archive).resolve() if args.python_archive else cache / f"python-{version}-embed-amd64.zip"
    if args.offline and not archive.is_file():
        raise ValueError("离线组装必须已准备官方Python归档")
    if not args.offline:
        print("核验固定Python官方归档", flush=True)
        download(url, archive, expected)
    ordinary(archive)
    if digest(archive) != expected:
        raise ValueError("Python归档与预先锁定的SHA256不匹配")
    wheelhouse = Path(args.wheelhouse).resolve() if args.wheelhouse else cache / ("cp" + "".join(version.split(".")[:2]))
    sources = wheel_sources(wheelhouse, version, args.offline)
    locked = contract()
    with tempfile.TemporaryDirectory(prefix="jmcomic-assemble-", dir=destination.parent) as temporary:
        stage = Path(temporary) / "stage"
        stage.mkdir()
        runtime = stage / RUNTIME_ROOT
        runtime.mkdir(parents=True)
        owned: dict[Path, str] = {}
        for file in extract(archive, runtime, "python"):
            owned[file] = "python"
        abi = "".join(version.split(".")[:2])
        pth = runtime / f"python{abi}._pth"
        if not pth.is_file():
            raise ValueError("官方归档缺少预期版本的隔离配置")
        pth.write_text(f"python{abi}.zip\n.\nLib/site-packages\n", encoding="ascii", newline="\n")
        site = runtime / "Lib/site-packages"
        site.mkdir(parents=True)
        for source in sources:
            files = extract(wheelhouse / source["filename"], site, source["name"], wheel=True)
            metadata = site / f'{source["name"].replace("-", "_")}-{source["version"]}.dist-info/METADATA'
            if metadata not in files:
                raise ValueError("固定wheel缺少规范版本元数据")
            parsed = BytesParser().parsebytes(metadata.read_bytes())
            normalized = re.sub(r"[-_.]+", "-", parsed.get("Name", "")).lower()
            if normalized != source["name"] or parsed.get("Version") != source["version"]:
                raise ValueError("wheel内部元数据与固定来源版本不一致")
            for file in files:
                owned[file] = source["name"]
        # 原样保留wheel内部材料，并另归集每组件许可，确保发行审查有固定入口。
        for component in ("python", *locked["packages"]):
            material = [file for file, owner in owned.items() if owner == component and any(word in file.name.lower() for word in ("license", "licence", "copying", "notice", "authors"))]
            if not material:
                raise ValueError(f"组件{component}缺少许可材料，拒绝分发")
            for index, file in enumerate(sorted(material)):
                target = stage / LICENSE_ROOT / component / f"{index:02d}-{file.name}"
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(file, target)
                owned[target] = component
        files = [{"path": file.relative_to(stage).as_posix(), "sha256": digest(file), "size": file.stat().st_size, "role": "runtime" if file.is_relative_to(runtime) else "license", "component": owner} for file, owner in sorted(owned.items())]
        manifest = {"schemaVersion": 1, "target": "windows-amd64", "pythonVersion": version, "jmcomicVersion": locked["packages"]["jmcomic"]["version"], "python": f"{RUNTIME_ROOT}/python.exe", "runtimeRoot": RUNTIME_ROOT, "requirementsSha256": locked["requirementsSha256"], "sources": [{"name": "python", "version": version, "url": url, "sha256": expected}, *[{key: value for key, value in source.items() if key != "filename"} for source in sources]], "files": files}
        manifest_path = stage / MANIFEST_NAME
        manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        verify(manifest_path.resolve(), probe=True)
        stage.rename(destination)
    print("漫画私有运行时已完成固定依赖与完整性校验", flush=True)
    return destination / MANIFEST_NAME


def main() -> int:
    """解析有中文解释的构建参数，准确返回运行时装配或校验结果。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    build = commands.add_parser("assemble", help="在构建端组装新的Windows x64私有运行时")
    build.add_argument("--output", required=True, help="全新的运行时材料根目录，不覆盖既有目录")
    build.add_argument("--python-version", default=PYTHON_VERSION, help="固定CPython补丁版本，默认已核验候选3.14.4")
    build.add_argument("--python-sha256", default=PYTHON_SHA256, help="官方embeddable归档SHA256；更换版本须显式提供")
    build.add_argument("--python-archive", help="可选的已下载官方embeddable ZIP路径，仍强制核验SHA256")
    build.add_argument("--wheelhouse", help="可选的固定wheel目录；离线模式须同时含wheel-sources.json")
    build.add_argument("--offline", action="store_true", help="禁止下载，仅使用已准备归档、wheel及来源记录")
    check = commands.add_parser("verify", help="校验现有私有运行时的完整材料")
    check.add_argument("manifest", type=Path, help="程序根jmcomic-runtime.json的绝对路径")
    check.add_argument("--probe", action="store_true", help="执行固定离线真实依赖与图片解码探测，不访问漫画站点")
    args = parser.parse_args()
    try:
        if args.command == "assemble":
            print("运行时清单：" + str(assemble(args)))
        else:
            value = verify(args.manifest, args.probe)
            print(f'漫画运行时校验通过：{value["target"]}，{len(value["files"])}份文件')
        return 0
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError, zipfile.BadZipFile) as error:
        print(f"漫画运行时处理失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    from project_env import run_cli

    raise SystemExit(run_cli(main))
