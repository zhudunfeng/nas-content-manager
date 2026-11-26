"""在构建机装配和校验Windows私有视频运行时；终端不联网安装、不修改全局Python。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True

import argparse
import base64
import csv
import hashlib
import io
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
from typing import Any, BinaryIO

from runtime_common import read_manifest, ordinary, verify_stream as verify_common_stream, download, extract
from project_env import resolve_command, runtime_directory
from release import ROOT, SHA256, configure_console, copy_verified, digest, relative_path, unique_object


MANIFEST_NAME = "uav-runtime.json"  # 程序根中的固定视频运行时清单名。
RUNTIME_ROOT = "runtimes/uav"  # 运行时只读文件的固定包内根。
LICENSE_ROOT = "licenses/uav"  # 独立保存全部组件许可的包内根。
MANIFEST_LIMIT = 4 * 1024 * 1024  # 清单最多4MiB，文件记录最多一万条。
PYTHON_VERSION = "3.14.4"  # 首个与真实锁定依赖一起验证的CPython补丁版本，不追踪最新版。
PYTHON_SHA256 = "cda80a9b1e75c0f1b4f9872ca1b417f0d19bce32facc811aea9180e70fad5fb9"  # Python官方对应zip.sigstore中公布的SHA2_256；不表示本脚本验证了Sigstore签名。
VERSION = re.compile(r"3\.(12|13|14)\.[0-9]{1,3}\Z")  # 只接纳现有wheel锁覆盖的正式CPython系列。
MAX_FILE = 512 * 1024 * 1024  # 单文件最多512MiB，防止异常压缩归档扩张。
MAX_TOTAL = 1024 * 1024 * 1024  # 一套视频运行材料最多1GiB。


def contract() -> dict[str, Any]:
    """读取Go和构建器共用契约，统一LF换行后确认requirements未漂移。"""
    value = read_manifest(ROOT / "internal/uavruntime/contract.json")
    requirements = (ROOT / "deploy/uav-requirements.txt").read_bytes().replace(b"\r\n", b"\n")
    build = (ROOT / "deploy/uav-build-requirements.txt").read_bytes().replace(b"\r\n", b"\n")
    if hashlib.sha256(requirements).hexdigest() != value["requirementsSha256"] or hashlib.sha256(build).hexdigest() != value["buildRequirementsSha256"]:
        raise ValueError("视频requirements已改变，须审查并同步编译期运行时契约后再构建")
    return value


def source_url(version: str) -> str:
    """生成唯一的Python官方Windows x64 embeddable来源，不接受镜像地址替换。"""
    if not VERSION.fullmatch(version):
        raise ValueError("Python版本必须为受支持系列的正式补丁版本")
    return f"https://www.python.org/ftp/python/{version}/python-{version}-embed-amd64.zip"


def uav_source_url() -> str:
    """固定上游源码来源，不允许用派生wheel或本机路径替换来源身份。"""
    return "https://codeload.github.com/Alos21750/UAV-Downloader/tar.gz/" + contract()["uavCommit"]


def validate_provenance(data: bytes) -> None:
    """核对UAV源码来源的唯一结构、官方地址及固定归档摘要。"""
    if len(data) > 4096:
        raise ValueError("视频源码来源记录过大")
    value = json.loads(data, object_pairs_hook=unique_object)
    expected = {"url": uav_source_url(), "archive_info": {"hashes": {"sha256": contract()["uavSourceSha256"]}}}
    if value != expected:
        raise ValueError("视频固定源码安装来源不符")


def validate_record(data: bytes, value: dict[str, Any]) -> None:
    """按最终清单核验UAV安装RECORD，要求来源规范化后的文件也被完整覆盖。"""
    if len(data) > MANIFEST_LIMIT:
        raise ValueError("视频安装记录超过预算")
    prefix = RUNTIME_ROOT + "/Lib/site-packages/"
    metadata = f'uav_downloader-{value["uavVersion"]}.dist-info/'
    expected = {record["path"][len(prefix):]: record for record in value["files"] if record["component"] == "uav-downloader" and record["role"] == "runtime" and record["path"].startswith(prefix)}
    seen: set[str] = set()
    for row in csv.reader(io.StringIO(data.decode("utf-8"), newline=""), strict=True):
        if len(row) != 3 or row[0] in seen or row[0] not in expected or relative_path(row[0]) != row[0]:
            raise ValueError("视频安装记录重复、越界或文件缺失")
        name, encoded, size = row
        seen.add(name)
        if name == metadata + "RECORD":
            if encoded or size:
                raise ValueError("视频安装记录不能绑定自身摘要")
            continue
        record = expected[name]
        checksum = "sha256=" + base64.urlsafe_b64encode(bytes.fromhex(record["sha256"])).decode("ascii").rstrip("=")
        if encoded != checksum or size != str(record["size"]):
            raise ValueError("视频安装记录摘要或长度不一致")
    if seen != set(expected):
        raise ValueError("视频安装记录未完整覆盖源码文件")


def verify_provenance(root: Path, value: dict[str, Any]) -> None:
    """读取已由文件清单校验的来源与安装记录，核对其跨文件语义。"""
    metadata = root / RUNTIME_ROOT / "Lib/site-packages" / f'uav_downloader-{value["uavVersion"]}.dist-info'
    validate_provenance((metadata / "direct_url.json").read_bytes())
    validate_record((metadata / "RECORD").read_bytes(), value)


def verify_stream(stream: BinaryIO, record: dict[str, Any], python_version: str) -> None:
    """复用共用文件完整性算法，并对视频源码来源记录执行固定语义检查。"""
    verify_common_stream(stream, record, python_version)
    if record["path"].endswith("/direct_url.json") and record["component"] == "uav-downloader":
        stream.seek(0)
        validate_provenance(stream.read(4097))


def write_install_record(site: Path, files: list[Path], record: Path) -> None:
    """对安装后材料重写RECORD，使规范元数据目录及来源记录与实际字节一致。"""
    material = sorted({*files, record})
    with record.open("w", newline="", encoding="utf-8") as output:
        writer = csv.writer(output, lineterminator="\n")
        for path in material:
            name = path.relative_to(site).as_posix()
            if path == record:
                writer.writerow([name, "", ""])
            else:
                encoded = base64.urlsafe_b64encode(bytes.fromhex(digest(path))).decode("ascii").rstrip("=")
                writer.writerow([name, "sha256=" + encoded, str(path.stat().st_size)])


def normalize_wheel_metadata(site: Path, files: list[Path], component: str, version: str) -> list[Path]:
    """仅规范依赖元数据目录的大小写，并同步安装记录，不改变Python模块路径。"""
    canonical = f'{component.replace("-", "_")}-{version}.dist-info'
    candidates = [file for file in files if file.name == "METADATA" and len(file.relative_to(site).parts) == 2 and file.parent.name.casefold().endswith(".dist-info")]
    if len(candidates) != 1 or candidates[0].parent.name.casefold() != canonical.casefold():
        raise ValueError("固定wheel缺少唯一规范版本元数据")
    metadata = candidates[0]
    parsed = BytesParser().parsebytes(metadata.read_bytes())
    normalized = re.sub(r"[-_.]+", "-", parsed.get("Name", "")).lower()
    if normalized != component or parsed.get("Version") != version:
        raise ValueError("wheel内部元数据与固定来源版本不一致")
    original = metadata.parent
    if original.name == canonical:
        return files
    # Windows对Path比较不区分大小写，显式两次改名才能保证最终清单字符串一致。
    temporary = site / (canonical + ".normalizing")
    if temporary.exists():
        raise ValueError("元数据规范化临时目录发生冲突")
    original.rename(temporary)
    destination = site / canonical
    temporary.rename(destination)
    installed = [destination / file.relative_to(original) if file.relative_to(site).parts[0] == original.name else file for file in files]
    record = destination / "RECORD"
    if record not in installed:
        raise ValueError("固定wheel缺少安装记录")
    write_install_record(site, installed, record)
    return installed


def normalize_provenance(site: Path, files: list[Path]) -> list[Path]:
    """仅在验证固定源后规范化direct_url与RECORD，不留下构建机绝对路径。"""
    metadata = site / f'uav_downloader-{contract()["uavVersion"]}.dist-info'
    if not (metadata / "METADATA").is_file():
        raise ValueError("固定UAV wheel缺少规范元数据")
    provenance = metadata / "direct_url.json"
    provenance.write_text(json.dumps({"url": uav_source_url(), "archive_info": {"hashes": {"sha256": contract()["uavSourceSha256"]}}}, separators=(",", ":")) + "\n", encoding="utf-8")
    record = metadata / "RECORD"
    write_install_record(site, [*files, provenance], record)
    return [provenance, record]


def build_uav_wheel(args: argparse.Namespace, cache: Path) -> tuple[Path, dict[str, str]]:
    """以固定构建工具从已核验源码装配wheel，运行时不携带pip或构建依赖。"""
    locked = contract()
    archive = Path(args.source_archive).resolve() if args.source_archive else cache / "uav-source.tar.gz"
    if not args.offline:
        download(uav_source_url(), archive, locked["uavSourceSha256"])
    ordinary(archive)
    if digest(archive) != locked["uavSourceSha256"]:
        raise ValueError("UAV源码归档与锁定来源不符")
    tools = Path(args.build_wheelhouse).resolve() if args.build_wheelhouse else cache / "build-wheels"
    tools.mkdir(parents=True, exist_ok=True)
    if not args.offline:
        subprocess.run(resolve_command([sys.executable, "-I", "-B", "-m", "pip", "download", "--only-binary=:all:", "--no-deps", "--require-hashes", "--dest", str(tools), "-r", str(ROOT / "deploy/uav-build-requirements.txt")]), cwd=ROOT, check=True, timeout=180)
    # 每次从固定源重新构建，不采信未经本次构建验证的派生缓存wheel。
    with tempfile.TemporaryDirectory(prefix="uav-build-", dir=runtime_directory()) as temporary:
        build = Path(temporary)
        environment_root = build / "env"
        subprocess.run([sys.executable, "-I", "-B", "-m", "venv", str(environment_root)], cwd=ROOT, check=True, timeout=120)
        python = environment_root / ("Scripts/python.exe" if os.name == "nt" else "bin/python")
        subprocess.run([str(python), "-I", "-B", "-m", "pip", "install", "--no-index", "--find-links", str(tools), "--only-binary=:all:", "--no-deps", "--require-hashes", "-r", str(ROOT / "deploy/uav-build-requirements.txt")], cwd=ROOT, check=True, timeout=120)
        wheels = build / "wheels"
        subprocess.run([str(python), "-I", "-B", "-m", "pip", "wheel", "--no-index", "--no-deps", "--no-build-isolation", "--wheel-dir", str(wheels), str(archive)], cwd=ROOT, check=True, timeout=180)
        candidates = list(wheels.glob("*.whl"))
        if len(candidates) != 1 or not candidates[0].name.startswith("uav_downloader-" + locked["uavVersion"] + "-"):
            raise ValueError("固定源码构建未生成唯一预期UAV wheel")
        checksum = digest(candidates[0])
        destination = cache / (checksum + ".whl")
        if destination.exists():
            ordinary(destination)
            if digest(destination) != checksum:
                raise ValueError("派生wheel缓存摘要不符")
        else:
            shutil.copyfile(candidates[0], destination)
    return destination, {"name": "uav-downloader", "version": locked["uavVersion"], "url": uav_source_url(), "sha256": locked["uavSourceSha256"]}


def validate_manifest(value: dict[str, Any]) -> dict[str, dict[str, Any]]:
    """统一静态清单契约，供本地校验、发行复制和归档校验复用。"""
    locked = contract()
    fields = {"schemaVersion", "target", "pythonVersion", "uavVersion", "python", "runtimeRoot", "requirementsSha256", "uavCommit", "buildRequirementsSha256", "builtWheelSha256", "sources", "files"}
    if set(value) != fields or type(value["schemaVersion"]) is not int or value["schemaVersion"] != 1 or value["target"] != "windows-amd64":
        raise ValueError("视频运行时清单字段、版本或目标无效")
    if not isinstance(value["pythonVersion"], str) or not VERSION.fullmatch(value["pythonVersion"]) or value["uavVersion"] != locked["uavVersion"] or value["requirementsSha256"] != locked["requirementsSha256"] or value["uavCommit"] != locked["uavCommit"] or value["buildRequirementsSha256"] != locked["buildRequirementsSha256"] or not isinstance(value["builtWheelSha256"], str) or not SHA256.fullmatch(value["builtWheelSha256"]):
        raise ValueError("视频运行时版本或依赖锁与当前程序不一致")
    if value["runtimeRoot"] != RUNTIME_ROOT or value["python"] != f"{RUNTIME_ROOT}/python.exe":
        raise ValueError("视频解释器不在固定包内位置")
    sources = value["sources"]
    if not isinstance(sources, list) or len(sources) != len(locked["packages"]) + 2:
        raise ValueError("视频运行时来源闭包不完整")
    components: set[str] = set()
    for source in sources:
        if not isinstance(source, dict) or set(source) != {"name", "version", "url", "sha256"} or not all(isinstance(item, str) for item in source.values()) or source["name"] in components or not SHA256.fullmatch(source["sha256"]):
            raise ValueError("视频运行时来源字段、重复组件或摘要无效")
        name = source["name"]
        if name == "python":
            if source["version"] != value["pythonVersion"] or source["url"] != source_url(value["pythonVersion"]):
                raise ValueError("私有Python来源与版本不匹配")
        elif name == "uav-downloader":
            if source["version"] != locked["uavVersion"] or source["url"] != uav_source_url() or source["sha256"] != locked["uavSourceSha256"]:
                raise ValueError("视频上游源码身份不匹配")
        else:
            entry = locked["packages"].get(name)
            url = urllib.parse.urlsplit(source["url"])
            if entry is None or source["version"] != entry["version"] or source["sha256"] not in entry["hashes"] or url.scheme != "https" or url.netloc != "files.pythonhosted.org" or not url.path.startswith("/packages/") or url.query or url.fragment:
                raise ValueError("视频wheel来源、版本或摘要不在固定白名单")
        components.add(name)
    if components != {"python", "uav-downloader", *locked["packages"]}:
        raise ValueError("视频来源组件缺失")
    files = value["files"]
    if not isinstance(files, list) or not 1 <= len(files) <= 10000:
        raise ValueError("视频运行文件数必须在1至10000之间")
    records: dict[str, dict[str, Any]] = {}
    folded: set[str] = set()
    licenses: set[str] = set()
    total = 0
    for record in files:
        if not isinstance(record, dict) or set(record) != {"path", "sha256", "size", "role", "component"}:
            raise ValueError("视频运行时文件字段无效")
        name = relative_path(record["path"])
        if not isinstance(record["sha256"], str) or not SHA256.fullmatch(record["sha256"]) or type(record["size"]) is not int or not 0 <= record["size"] <= MAX_FILE or not isinstance(record["component"], str) or record["component"] not in components or name.casefold() in folded:
            raise ValueError("视频运行文件摘要、长度、组件或重复路径无效")
        if record["role"] == "runtime":
            if not name.startswith(RUNTIME_ROOT + "/"):
                raise ValueError("视频运行文件位置越界")
        elif record["role"] == "license":
            if not name.startswith(f'{LICENSE_ROOT}/{record["component"]}/') or record["size"] == 0:
                raise ValueError("视频许可材料位置或长度无效")
            licenses.add(record["component"])
        else:
            raise ValueError("视频文件角色无效")
        if name.lower().endswith((".pth", ".pyc")):
            raise ValueError("私有运行时不得夹带额外site配置或生成缓存")
        total += record["size"]
        folded.add(name.casefold())
        records[name] = record
    if total > MAX_TOTAL or licenses != components:
        raise ValueError("视频材料超过整体预算或缺少组件许可")
    abi = "".join(value["pythonVersion"].split(".")[:2])
    required = {f"{RUNTIME_ROOT}/{name}" for name in ("python.exe", f"python{abi}.dll", f"python{abi}.zip", f"python{abi}._pth", "Lib/site-packages/certifi/cacert.pem")}
    required |= {f'{RUNTIME_ROOT}/Lib/site-packages/{name.replace("-", "_")}-{entry["version"]}.dist-info/METADATA' for name, entry in locked["packages"].items()}
    required |= {f'{RUNTIME_ROOT}/Lib/site-packages/uav_downloader-{locked["uavVersion"]}.dist-info/{name}' for name in ("METADATA", "direct_url.json", "RECORD")}
    if not required.issubset(records):
        raise ValueError("视频运行时缺少解释器、隔离路径、证书或依赖元数据")
    for prefix in ("curl_cffi/_wrapper", "_cffi_backend", "Crypto/"):
        if not any(name.startswith(f"{RUNTIME_ROOT}/Lib/site-packages/{prefix}") and name.endswith(".pyd") for name in records):
            raise ValueError("视频运行时缺少必要本机扩展")
    return records


def verify(manifest_path: Path, probe: bool = False) -> dict[str, Any]:
    """校验完整私有运行时；可选执行固定离线依赖探测，不访问视频站点。"""
    if manifest_path.name != MANIFEST_NAME or not manifest_path.is_absolute() or str(manifest_path) != os.path.abspath(manifest_path) or os.path.normcase(str(manifest_path.resolve())) != os.path.normcase(str(manifest_path)):
        raise ValueError("视频运行时清单必须是固定名称的绝对路径")
    value = read_manifest(manifest_path)
    records = validate_manifest(value)
    root = manifest_path.parent
    actual: set[str] = set()
    folded: set[str] = set()
    for directory in (root / RUNTIME_ROOT, root / LICENSE_ROOT):
        if not directory.is_dir():
            raise ValueError("视频运行时或许可目录缺失")
        for base, directories, files in os.walk(directory, followlinks=False):
            for name in directories:
                information = (Path(base) / name).lstat()
                if stat.S_ISLNK(information.st_mode) or getattr(information, "st_file_attributes", 0) & 0x400:
                    raise ValueError("视频运行目录包含链接或重解析点")
            for name in files:
                path = Path(base) / name
                ordinary(path)
                local = path.relative_to(root).as_posix()
                if local not in records or local.casefold() in folded:
                    raise ValueError("视频运行时含未声明文件或路径别名")
                with path.open("rb") as stream:
                    verify_stream(stream, records[local], value["pythonVersion"])
                actual.add(local)
                folded.add(local.casefold())
    if actual != set(records):
        raise ValueError("视频运行时缺少声明文件")
    verify_provenance(root, value)
    if probe:
        run_probe(root / value["python"], value)
    return value


def run_probe(python: Path, value: dict[str, Any]) -> None:
    """在私有解释器隔离执行真实五站点桥接及AES/证书探测，禁止站点网络。"""
    expected = {name: entry["version"] for name, entry in contract()["packages"].items()}
    code = """import json,socket,sys,runpy,io,ssl
from importlib.metadata import version
def deny_network(*args: object, **kwargs: object) -> None:
    '禁止离线探测产生任何站点网络连接。'
    raise RuntimeError('离线视频运行时探测禁止网络')
socket.create_connection=deny_network
socket.socket.connect=deny_network
import certifi,curl_cffi,cffi
from Crypto.Cipher import AES
expected=json.loads(sys.argv[1])
assert {name:version(name) for name in expected}==expected
assert sys.version.split()[0]==sys.argv[2] and sys.maxsize>2**32
assert sys.flags.isolated and sys.flags.utf8_mode and sys.dont_write_bytecode
ssl.create_default_context(cafile=certifi.where())
assert len(AES.new(b'0123456789abcdef',AES.MODE_ECB).encrypt(b'0123456789abcdef'))==16
runpy.run_path(sys.argv[3],run_name='__main__')
"""
    environment = dict(os.environ, PATH="", PYTHONHOME="invalid-runtime-pollution", PYTHONPATH="invalid-runtime-pollution")
    bridge = ROOT / "internal/collectionworker/uav_bridge.py"
    result = subprocess.run([str(python), "-I", "-B", "-X", "utf8", "-c", code, json.dumps(expected), value["pythonVersion"], str(bridge)], cwd=runtime_directory(), env=environment, input=b'{"action":"probe"}', stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30, check=False)
    if result.returncode != 0 or len(result.stdout) > 65536 or len(result.stderr) > 65536:
        raise ValueError("私有视频运行时真实依赖探测失败，请检查Python ABI、VC运行库及wheel闭包")
    response = json.loads(result.stdout)
    if response != {"version": value["uavVersion"], "commit": value["uavCommit"], "protocolVersion": 1}:
        raise ValueError("私有视频运行时桥接响应不符合固定版本")
    print("私有视频运行时离线探测通过：" + value["pythonVersion"], flush=True)


def wheel_sources(wheelhouse: Path, version: str, offline: bool) -> list[dict[str, str]]:
    """下载哈希锁定wheel并保存可供离线重建的官方来源记录。"""
    locked = contract()
    if not offline:
        wheelhouse.mkdir(parents=True, exist_ok=True)
        abi = "cp" + "".join(version.split(".")[:2])
        command = [sys.executable, "-I", "-B", "-m", "pip", "download", "--disable-pip-version-check", "--only-binary=:all:", "--no-deps", "--require-hashes", "--platform", "win_amd64", "--python-version", ".".join(version.split(".")[:2]), "--implementation", "cp", "--abi", abi, "--index-url", "https://pypi.org/simple", "--dest", str(wheelhouse), "-r", str(ROOT / "deploy/uav-requirements.txt")]
        print("下载固定哈希的视频Windows wheel闭包", flush=True)
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
        raise ValueError("随包视频运行时目前仅支持Windows x64")
    value = verify(manifest_path)
    for record in value["files"]:
        copy_verified(manifest_path.parent / record["path"], stage / record["path"], record["sha256"], record["path"].lower().endswith(".exe"))
    destination = stage / MANIFEST_NAME
    destination.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    verify(destination.resolve())
    return {"manifest": MANIFEST_NAME, "sha256": digest(destination), "pythonVersion": value["pythonVersion"], "uavVersion": value["uavVersion"], "uavCommit": value["uavCommit"]}


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
    cache = runtime_directory() / "tools/uav-inputs"
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
    uav_wheel, uav_source = build_uav_wheel(args, cache)
    locked = contract()
    with tempfile.TemporaryDirectory(prefix="uav-assemble-", dir=destination.parent) as temporary:
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
            files = normalize_wheel_metadata(site, files, source["name"], source["version"])
            for file in files:
                owned[file] = source["name"]
        uav_files = extract(uav_wheel, site, "uav-downloader", wheel=True)
        uav_files += normalize_provenance(site, uav_files)
        for file in uav_files:
            owned[file] = "uav-downloader"
        # 原样保留wheel内部材料，并另归集每组件许可，确保发行审查有固定入口。
        for component in ("python", "uav-downloader", *locked["packages"]):
            material = [file for file, owner in owned.items() if owner == component and any(word in file.name.lower() for word in ("license", "licence", "copying", "notice", "authors"))]
            if not material:
                raise ValueError(f"组件{component}缺少许可材料，拒绝分发")
            for index, file in enumerate(sorted(material)):
                target = stage / LICENSE_ROOT / component / f"{index:02d}-{file.name}"
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(file, target)
                owned[target] = component
        files = [{"path": file.relative_to(stage).as_posix(), "sha256": digest(file), "size": file.stat().st_size, "role": "runtime" if file.is_relative_to(runtime) else "license", "component": owner} for file, owner in sorted(owned.items())]
        manifest = {"schemaVersion": 1, "target": "windows-amd64", "pythonVersion": version, "uavVersion": locked["uavVersion"], "uavCommit": locked["uavCommit"], "buildRequirementsSha256": locked["buildRequirementsSha256"], "builtWheelSha256": digest(uav_wheel), "python": f"{RUNTIME_ROOT}/python.exe", "runtimeRoot": RUNTIME_ROOT, "requirementsSha256": locked["requirementsSha256"], "sources": [{"name": "python", "version": version, "url": url, "sha256": expected}, uav_source, *[{key: value for key, value in source.items() if key != "filename"} for source in sources]], "files": files}
        manifest_path = stage / MANIFEST_NAME
        manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        verify(manifest_path.resolve(), probe=True)
        stage.rename(destination)
    print("视频私有运行时已完成固定依赖与完整性校验", flush=True)
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
    build.add_argument("--source-archive", help="可选的固定UAV源码归档路径，仍核验源码摘要")
    build.add_argument("--build-wheelhouse", help="可选的固定构建工具wheel目录；离线构建必须已准备")
    build.add_argument("--offline", action="store_true", help="禁止下载，仅使用已准备归档、wheel及来源记录")
    check = commands.add_parser("verify", help="校验现有私有运行时的完整材料")
    check.add_argument("manifest", type=Path, help="程序根uav-runtime.json的绝对路径")
    check.add_argument("--probe", action="store_true", help="执行固定离线真实依赖、AES及证书探测，不访问视频站点")
    args = parser.parse_args()
    try:
        if args.command == "assemble":
            print("运行时清单：" + str(assemble(args)))
        else:
            value = verify(args.manifest, args.probe)
            print(f'视频运行时校验通过：{value["target"]}，{len(value["files"])}份文件')
        return 0
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError, zipfile.BadZipFile) as error:
        print(f"视频运行时处理失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    from project_env import run_cli

    raise SystemExit(run_cli(main))
