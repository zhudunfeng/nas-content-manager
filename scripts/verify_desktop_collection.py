"""在真实 Windows 隔离副本验证漫画托管、旧配置迁移和进程所有权；不访问真实站点。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True
    from project_env import configure_environment
    configure_environment()

import argparse
import ctypes
from ctypes import wintypes
import http.cookiejar
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
from typing import Any
import urllib.request

from verify import generate_password
from verify_desktop import assert_exclusive, control, execute, file_digest, request, wait_ready


def authenticated(base: str) -> urllib.request.OpenerDirector:
    """为本轮全新数据库创建隔离账号，不持久记录口令和会话。"""
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    request(opener, base, "/api/setup", {"username": "managed_check", "password": generate_password()})
    return opener


def descendants(parent: int) -> list[int]:
    """通过只读 Toolhelp 快照列出本轮父进程的后代，不按进程名终止。"""
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)

    class ProcessEntry(ctypes.Structure):
        """原生进程快照结构，仅保留关联和可执行名称。"""
        _fields_ = [
            ("size", wintypes.DWORD),  # 原生结构字节数。
            ("usage", wintypes.DWORD),  # 系统保留引用计数。
            ("pid", wintypes.DWORD),  # 本快照进程编号。
            ("heap", ctypes.c_size_t),  # 系统保留默认堆标识。
            ("module", wintypes.DWORD),  # 系统保留模块标识。
            ("threads", wintypes.DWORD),  # 当前线程数量。
            ("parent", wintypes.DWORD),  # 创建该进程的父编号。
            ("priority", wintypes.LONG),  # 基础优先级。
            ("flags", wintypes.DWORD),  # 系统保留标志。
            ("name", wintypes.WCHAR * 260),  # 可执行文件名称。
        ]

    kernel.CreateToolhelp32Snapshot.argtypes = [wintypes.DWORD, wintypes.DWORD]
    kernel.CreateToolhelp32Snapshot.restype = wintypes.HANDLE
    kernel.Process32FirstW.argtypes = [wintypes.HANDLE, ctypes.POINTER(ProcessEntry)]
    kernel.Process32NextW.argtypes = [wintypes.HANDLE, ctypes.POINTER(ProcessEntry)]
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    snapshot = kernel.CreateToolhelp32Snapshot(2, 0)
    assert snapshot != wintypes.HANDLE(-1).value
    parents: dict[int, int] = {}
    try:
        entry = ProcessEntry()
        entry.size = ctypes.sizeof(entry)
        found = kernel.Process32FirstW(snapshot, ctypes.byref(entry))
        while found:
            parents[int(entry.pid)] = int(entry.parent)
            found = kernel.Process32NextW(snapshot, ctypes.byref(entry))
    finally:
        kernel.CloseHandle(snapshot)
    owned = {parent}
    while True:
        previous = len(owned)
        owned.update(pid for pid, owner in parents.items() if owner in owned)
        if len(owned) == previous:
            return sorted(owned - {parent})


def wait_handles(pids: list[int]) -> list[int]:
    """在触发退出前保存确切进程句柄，避免 PID 重用产生错误结论。"""
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    kernel.OpenProcess.restype = wintypes.HANDLE
    result: list[int] = []
    for pid in pids:
        handle = kernel.OpenProcess(0x100000, False, pid)
        if handle:
            result.append(int(handle))
    return result


def terminate_observed(pid: int) -> int:
    """只通过本轮已观察 worker 的确切内核句柄制造异常退出，不按进程名结束程序。"""
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    kernel.OpenProcess.restype = wintypes.HANDLE
    kernel.TerminateProcess.argtypes = [wintypes.HANDLE, wintypes.UINT]
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    handle = kernel.OpenProcess(0x100001, False, pid)
    assert handle, "无法打开本轮 worker 的终止验证句柄"
    if not kernel.TerminateProcess(handle, 137):
        kernel.CloseHandle(handle)
        raise AssertionError("未能制造本轮 worker 的异常退出")
    return int(handle)


def observed_image(pid: int) -> Path | None:
    """读取已观察进程的实际映像路径，仅用于精确选择本轮 worker。"""
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    kernel.OpenProcess.restype = wintypes.HANDLE
    kernel.QueryFullProcessImageNameW.argtypes = [wintypes.HANDLE, wintypes.DWORD, wintypes.LPWSTR, ctypes.POINTER(wintypes.DWORD)]
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    handle = kernel.OpenProcess(0x1000, False, pid)
    if not handle:
        return None
    try:
        size = wintypes.DWORD(32768)
        name = ctypes.create_unicode_buffer(size.value)
        if not kernel.QueryFullProcessImageNameW(handle, 0, name, ctypes.byref(size)):
            return None
        return Path(name.value).resolve()
    finally:
        kernel.CloseHandle(handle)


def assert_processes_released(handles: list[int]) -> None:
    """验证所有保存的内核句柄已触发退出，然后释放验证者自己的引用。"""
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.WaitForSingleObject.argtypes = [wintypes.HANDLE, wintypes.DWORD]
    kernel.WaitForSingleObject.restype = wintypes.DWORD
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    try:
        for handle in handles:
            assert kernel.WaitForSingleObject(handle, 7000) == 0, "托管后代未完整退出"
    finally:
        for handle in handles:
            kernel.CloseHandle(handle)


def loaded_runtime_images(pid: int, runtime: Path) -> list[Path]:
    """读取真实 Python 已映射的私有 DLL/PYD，避免用未执行的旁置文件冒充释放证据。"""
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    psapi = ctypes.WinDLL("psapi", use_last_error=True)
    kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    kernel.OpenProcess.restype = wintypes.HANDLE
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    psapi.EnumProcessModulesEx.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.HMODULE), wintypes.DWORD, ctypes.POINTER(wintypes.DWORD), wintypes.DWORD]
    psapi.GetModuleFileNameExW.argtypes = [wintypes.HANDLE, wintypes.HMODULE, wintypes.LPWSTR, wintypes.DWORD]
    handle = kernel.OpenProcess(0x410, False, pid)
    assert handle, "无法读取本轮真实 Python 已加载扩展"
    try:
        modules = (wintypes.HMODULE * 1024)()
        needed = wintypes.DWORD()
        assert psapi.EnumProcessModulesEx(handle, modules, ctypes.sizeof(modules), ctypes.byref(needed), 3)
        assert 0 < needed.value <= ctypes.sizeof(modules), "真实 Python 加载模块数超出检查预算"
        result: list[Path] = []
        for module in modules[:needed.value // ctypes.sizeof(wintypes.HMODULE)]:
            name = ctypes.create_unicode_buffer(32768)
            assert psapi.GetModuleFileNameExW(handle, module, name, len(name))
            path = Path(name.value).resolve(strict=True)
            if path.is_relative_to(runtime):
                result.append(path)
        assert any(path.name.startswith("_imaging.") for path in result), "没有实际加载 Pillow 原生扩展"
        assert any(path.name == "_wrapper.pyd" for path in result), "没有实际加载 curl_cffi 原生扩展"
        assert any(path.name.startswith("_raw_aes.") for path in result), "没有实际加载 AES 原生扩展"
        assert any(path.name.startswith("python3") and path.suffix == ".dll" for path in result), "没有实际加载私有 CPython DLL"
        return result
    finally:
        kernel.CloseHandle(handle)


def stage_runtime(manifest: Path, program: Path) -> tuple[Path, list[Path]]:
    """仅复制清单声明的受信构建输出，所有修改和占用验证都局限于本轮隔离副本。"""
    data: dict[str, Any] = json.loads(manifest.read_bytes())
    source_root = manifest.parent.resolve(strict=True)
    images: list[Path] = []
    for item in data["files"]:
        relative = Path(item["path"])
        assert not relative.is_absolute() and ".." not in relative.parts and ":" not in str(relative)
        source = (source_root / relative).resolve(strict=True)
        assert source.is_relative_to(source_root) and source.is_file()
        target = program / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
        if target.suffix.lower() in (".exe", ".dll", ".pyd"):
            images.append(target)
    shutil.copyfile(manifest, program / "jmcomic-runtime.json")
    return program / data["python"], images


def assert_runtime_replaced(images: list[Path], source_root: Path, program: Path) -> None:
    """退出后立即独占打开、改名并原位替换真实运行时映像，再移动整个程序目录。"""
    for path in images:
        assert_exclusive(path)
        moved = path.with_name(path.name + ".exit-check")
        path.rename(moved)
        moved.rename(path)
        original = source_root / path.relative_to(program)
        shutil.copyfile(original, path)
        assert file_digest(path) == file_digest(original), "私有运行时原位替换摘要不一致"
    moved_program = program.with_name(program.name + ".exit-check")
    program.rename(moved_program)
    moved_program.rename(program)


def build_nested_fixture(root: Path, python: Path, marker: Path, environment: dict[str, str]) -> Path:
    """临时构建可信桥接替身以保持真实 Python 活动，仅验证嵌套 Job，不模拟站点下载。"""
    source = root / "nested_fixture.go"
    target = root / "nested-fixture.exe"
    script = "\n".join([
        "import json,os,time",
        "from pathlib import Path",
        "from PIL import Image",
        "from curl_cffi import Curl",
        "from Crypto.Cipher import AES",
        "image = Image.new('RGB', (2, 2)); image.load()",
        "curl = Curl(); curl.close()",
        "AES.new(b'0'*16, AES.MODE_ECB).encrypt(b'1'*16)",
        f"Path({str(marker)!r}).write_text(json.dumps({{'pid':os.getpid()}}), encoding='utf-8')",
        "time.sleep(120)",
    ])
    source.write_text("\n".join([
        "package main",
        'import ("os"; "os/exec")',
        "// main 仅为隔离生命周期回归保留真实 Python 和已加载的扩展。",
        "func main() {",
        f"command := exec.Command({json.dumps(str(python))}, \"-I\", \"-B\", \"-X\", \"utf8\", \"-c\", {json.dumps(script)})",
        "command.Stdout = os.Stdout; command.Stderr = os.Stderr",
        "if command.Run() != nil { os.Exit(1) }",
        "}",
    ]), encoding="utf-8")
    repository = Path(__file__).resolve().parents[1]
    try:
        result = subprocess.run([sys.executable, "-B", str(repository / "scripts/project_env.py"), "--", "go", "build", "-o", str(target), str(source)], cwd=repository, env=environment, capture_output=True, timeout=90)
        assert result.returncode == 0, result.stderr.decode("utf-8", errors="replace")[-2000:]
    finally:
        source.unlink(missing_ok=True)
    return target


def wait_state(binary: Path, profile: Path, process: subprocess.Popen[bytes], environment: dict[str, str], cwd: Path, state: str, previous_workers: set[int] | None = None) -> dict[str, Any]:
    """等待托管模式切换完成，避免把尚未执行的 IPC 排队响应当作结果。"""
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        assert process.poll() is None, "切换期间桌面进程退出"
        status = control(binary, profile, "status", environment, cwd)
        if status["state"] == "ready" and status.get("collectionState") == state:
            current = {pid for pid in descendants(process.pid) if observed_image(pid) == binary.parent / "nas-collection-worker.exe"}
            if previous_workers is None or current and not current.intersection(previous_workers):
                return status
        time.sleep(0.1)
    raise AssertionError(f"等待漫画状态 {state} 超时")


def main() -> int:
    """验证真实桌面与 worker、私有管道、配置迁移和退出；保留可复用集成脚本。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True, help="当前 Windows 桌面 EXE 构建产物")
    parser.add_argument("--worker", type=Path, required=True, help="支持 --managed-stdio 的当前 worker EXE")
    parser.add_argument("--python", type=Path, help="已装配或已安装锁定依赖的可信绝对 Python；仅离线探测")
    parser.add_argument("--runtime-manifest", type=Path, help="受信完整包运行时清单；复制到隔离程序目录后验证真实依赖、DLL替换和嵌套Job")
    parser.add_argument("--web-dir", type=Path, help="开发构建使用的实际网页资源目录")
    args = parser.parse_args()
    assert not (args.python and args.runtime_manifest), "--python 与 --runtime-manifest 不能混用"
    assert os.name == "nt", "此脚本必须运行于真实 Windows"
    source = args.binary.resolve(strict=True)
    worker_source = args.worker.resolve(strict=True)
    environment = {key: value for key, value in os.environ.items() if not key.upper().startswith(("NAS_", "ENGINE_", "QBT_", "JACKETT_", "PYTHONPATH", "PYTHONHOME"))}
    if args.web_dir:
        environment["NAS_WEB_DIR"] = str(args.web_dir.resolve(strict=True))
    if args.python:
        environment["NAS_COLLECTION_WORKER_JMCOMIC_PYTHON"] = str(args.python.resolve(strict=True))
    processes: list[subprocess.Popen[bytes]] = []
    checks: list[str] = []
    loaded_images: list[str] = []
    with tempfile.TemporaryDirectory(prefix="Windows 漫画托管 ") as temporary:
        root = Path(temporary)
        program = root / "中文 程序"
        program.mkdir()
        binary = program / "栖库.exe"
        shutil.copyfile(source, binary)
        media = root / "中文 媒体"
        media.mkdir()
        foreign = root / "其他 工作目录"
        foreign.mkdir()
        profile = root / "新 配置"
        output_path = root / "启动诊断.log"
        with output_path.open("wb") as output:
            def launch(target_profile: Path, mode: str, extra_environment: dict[str, str] | None = None) -> subprocess.Popen[bytes]:
                """只启动本轮隔离副本，并登记精确进程以便异常清理。"""
                env = {**environment, **(extra_environment or {})}
                process = subprocess.Popen([str(binary), "--headless", "--no-browser", "--profile", str(target_profile), "--media-root", str(media), "--collection-mode", mode], cwd=foreign, env=env, stdout=output, stderr=output)
                processes.append(process)
                return process

            def quit_instance(target_profile: Path, process: subprocess.Popen[bytes]) -> None:
                """正常退出后立即核对 EXE 可写，再执行其他可能耗时的检查。"""
                control(binary, target_profile, "quit", environment, foreign)
                assert process.wait(timeout=25) == 0, output_path.read_text(encoding="utf-8")[-1500:]
                assert_exclusive(binary)
                worker = program / "nas-collection-worker.exe"
                if worker.exists():
                    assert_exclusive(worker)

            try:
                # 缺少 worker 必须仅降级漫画功能，不阻止登录与媒体服务。
                process = launch(profile, "managed")
                reply = wait_ready(binary, profile, process, environment, foreign)
                assert reply["collectionState"] == "failed"
                opener = authenticated(reply["url"])
                caps = json.loads(request(opener, reply["url"], "/api/collection/capabilities")[1])
                assert caps["workerReady"] is False and "本机漫画获取服务启动失败" in caps["workerReason"]
                quit_instance(profile, process)
                checks.append("缺worker仅降级，真实登录后API返回具体原因")

                # 外接环境污染不得混入 disabled 的连接快照。
                polluted = {"NAS_COLLECTION_WORKER_URL": "http://foreign.invalid", "NAS_COLLECTION_WORKER_TOKEN_FILE": "bad", "NAS_COLLECTION_READ_ROOT": "bad", "NAS_COLLECTION_WORKER_ALLOW_HTTP": "false"}
                process = launch(profile, "disabled", polluted)
                reply = wait_ready(binary, profile, process, environment, foreign)
                assert reply["collectionState"] == "disabled"
                quit_instance(profile, process)
                checks.append("disabled清空完整外接连接，不受污染环境影响")

                # 新版配置严格必需 collectionMode，不自动修补损坏配置。
                saved = json.loads((profile / "desktop.json").read_bytes())
                invalid_profile = root / "坏 配置"
                invalid_profile.mkdir()
                for field in saved:
                    for null in (False, True):
                        invalid = dict(saved)
                        if null:
                            invalid[field] = None
                        else:
                            del invalid[field]
                        raw = json.dumps(invalid, ensure_ascii=False).encode("utf-8")
                        (invalid_profile / "desktop.json").write_bytes(raw)
                        result = execute(binary, ["--headless", "--profile", str(invalid_profile), "--print-config"], environment, foreign)
                        assert result.returncode != 0, f"缺失或null字段被接受：{field}"
                        assert (invalid_profile / "desktop.json").read_bytes() == raw
                checks.append("新版十字段缺失/null全部拒绝并保留原文")

                # 旧配置保留身份/账号路径，并在真实保存时生成原文备份。
                legacy = dict(saved)
                legacy["schemaVersion"] = 1
                del legacy["collectionMode"]
                raw = json.dumps(legacy, ensure_ascii=False, indent=2).encode("utf-8")
                (profile / "desktop.json").write_bytes(raw)
                result = execute(binary, ["--headless", "--profile", str(profile), "--print-config"], environment, foreign)
                migrated = json.loads(result.stdout)
                assert result.returncode == 0 and migrated["collectionMode"] == "disabled" and migrated["instanceId"] == saved["instanceId"] and migrated["dataDir"] == saved["dataDir"]
                assert (profile / "desktop.json").read_bytes() == raw
                process = subprocess.Popen([str(binary), "--headless", "--no-browser", "--profile", str(profile)], cwd=foreign, env=environment, stdout=output, stderr=output)
                processes.append(process)
                wait_ready(binary, profile, process, environment, foreign)
                assert (profile / "desktop.v1.json").read_bytes() == raw
                quit_instance(profile, process)
                checks.append("v1显式迁移、身份数据目录不变、原文备份与只读print-config")

                external_profile = root / "外接旧 配置"
                external_profile.mkdir()
                (external_profile / "desktop.json").write_bytes(raw)
                result = execute(binary, ["--headless", "--profile", str(external_profile), "--print-config"], {**environment, **polluted}, foreign)
                assert result.returncode == 0 and json.loads(result.stdout)["collectionMode"] == "external"
                assert (external_profile / "desktop.json").read_bytes() == raw
                result = execute(binary, ["--headless", "--profile", str(profile), "--print-config"], {**environment, "NAS_DESKTOP_COLLECTION_MODE": "external"}, foreign)
                assert result.returncode == 0 and json.loads(result.stdout)["collectionMode"] == "external"
                result = execute(binary, ["--headless", "--profile", str(profile), "--print-config", "--collection-mode", "managed"], {**environment, "NAS_DESKTOP_COLLECTION_MODE": "disabled"}, foreign)
                assert result.returncode == 0 and json.loads(result.stdout)["collectionMode"] == "managed"
                checks.append("旧外接配置不被接管，模式优先级CLI高于环境高于已保存配置")

                shutil.copyfile(worker_source, program / "nas-collection-worker.exe")
                runtime_python: Path | None = None
                runtime_images: list[Path] = []
                if args.runtime_manifest:
                    runtime_python, runtime_images = stage_runtime(args.runtime_manifest.resolve(strict=True), program)
                    default_profile = root / "完整包默认 配置"
                    result = execute(binary, ["--headless", "--profile", str(default_profile), "--print-config"], environment, foreign)
                    assert result.returncode == 0 and json.loads(result.stdout)["collectionMode"] == "managed"
                    assert not (default_profile / "desktop.json").exists()
                    checks.append("新完整包profile自动选择managed，print-config仍无写入")
                process = launch(profile, "managed", {**polluted, "PYTHONPATH": str(foreign), "PYTHONHOME": str(foreign)})
                reply = wait_ready(binary, profile, process, environment, foreign)
                expected = "ready" if args.python or runtime_python else "unavailable"
                assert reply["collectionState"] == expected, reply
                children = descendants(process.pid)
                assert children, "未观察到真实worker进程"
                handles = wait_handles(children)
                quit_instance(profile, process)
                assert_processes_released(handles)
                for path in (profile / "logs").glob("*.log"):
                    assert_exclusive(path)
                checks.append("真实worker、认证目录挑战、正常退出、自有句柄释放")
                if args.runtime_manifest:
                    assert_runtime_replaced(runtime_images, args.runtime_manifest.resolve().parent, program)
                    checks.append("清单选择真实私有Python执行固定桥接，退出后运行时映像原位替换/摘要/程序目录改名")

                # 通过真实当前用户IPC启停/重新检测，历史数据库仍沿用。
                process = launch(profile, "disabled")
                wait_ready(binary, profile, process, environment, foreign)
                control(binary, profile, "collection-enable", environment, foreign)
                wait_state(binary, profile, process, environment, foreign, expected)
                previous_workers = {pid for pid in descendants(process.pid) if observed_image(pid) == program / "nas-collection-worker.exe"}
                assert previous_workers
                control(binary, profile, "collection-check", environment, foreign)
                wait_state(binary, profile, process, environment, foreign, expected, previous_workers)
                control(binary, profile, "collection-disable", environment, foreign)
                wait_state(binary, profile, process, environment, foreign, "disabled")
                quit_instance(profile, process)
                checks.append("真实IPC启用/重检/停用，模式变更串行重启")

                # 异常退出与资源清理是不同事实；已核验释放的崩溃不能永久阻止主动重检。
                process = launch(profile, "managed")
                reply = wait_ready(binary, profile, process, environment, foreign)
                children = descendants(process.pid)
                workers = [pid for pid in children if observed_image(pid) == program / "nas-collection-worker.exe"]
                assert len(workers) == 1, f"未找到本轮唯一worker，实际后代：{[(pid, str(observed_image(pid))) for pid in children]}"
                assert_processes_released([terminate_observed(workers[0])])
                wait_state(binary, profile, process, environment, foreign, "failed")
                assert request(urllib.request.build_opener(urllib.request.ProxyHandler({})), reply["url"], "/healthz")[0] == 200
                control(binary, profile, "collection-check", environment, foreign)
                wait_state(binary, profile, process, environment, foreign, expected)
                quit_instance(profile, process)
                checks.append("真实worker异常退出仅降级，媒体健康且主动重检可恢复")

                # 桌面异常终止应由内核Job兜底清理worker；独立进程必须保留。
                sentinel = subprocess.Popen([sys.executable, "-B", "-c", "import time;time.sleep(120)"], env=environment, stdout=output, stderr=output)
                processes.append(sentinel)
                process = launch(profile, "managed")
                wait_ready(binary, profile, process, environment, foreign)
                handles = wait_handles(descendants(process.pid))
                assert handles
                process.kill()
                process.wait(timeout=10)
                assert_exclusive(binary)
                assert_processes_released(handles)
                assert sentinel.poll() is None
                assert_exclusive(program / "nas-collection-worker.exe")
                process = launch(profile, "managed")
                wait_ready(binary, profile, process, environment, foreign)
                quit_instance(profile, process)
                checks.append("父进程异常退出Job回收、独立哨兵保留、同profile立即重启")

                if args.runtime_manifest and runtime_python:
                    marker = root / "真实Python加载完成.json"
                    fixture = build_nested_fixture(root, runtime_python, marker, environment)
                    nested_sentinel = subprocess.Popen([sys.executable, "-B", "-c", "import time;time.sleep(180)"], env=environment, stdout=output, stderr=output)
                    processes.append(nested_sentinel)
                    for action in ("quit", "kill"):
                        marker.unlink(missing_ok=True)
                        process = launch(profile, "managed", {"NAS_COLLECTION_WORKER_JMCOMIC_PYTHON": str(fixture)})
                        deadline = time.monotonic() + 8
                        while not marker.exists():
                            assert process.poll() is None, "真实Python活动检查前桌面提前退出"
                            assert time.monotonic() < deadline, "真实Python扩展未在探测预算内加载"
                            time.sleep(0.02)
                        python_pid = int(json.loads(marker.read_bytes())["pid"])
                        children = descendants(process.pid)
                        assert len(children) >= 3 and python_pid in children, "未形成桌面→worker→工具Job→真实Python嵌套子树"
                        handles = wait_handles(children)
                        assert len(handles) == len(children)
                        images = loaded_runtime_images(python_pid, runtime_python.parent)
                        loaded_images = sorted(path.name for path in images)
                        for path in images:
                            assert_exclusive(path, available=False)
                        if action == "quit":
                            quit_instance(profile, process)
                        else:
                            process.kill()
                            process.wait(timeout=10)
                            assert_exclusive(binary)
                        assert_processes_released(handles)
                        assert_exclusive(program / "nas-collection-worker.exe")
                        assert_exclusive(fixture)
                        assert nested_sentinel.poll() is None
                        assert_runtime_replaced(images, args.runtime_manifest.resolve().parent, program)
                    checks.append("真实私有Python已映射Pillow/curl/AES/CPython扩展，启动取消与父强杀均回收嵌套Job且保留独立哨兵")
                    process = launch(profile, "managed")
                    wait_ready(binary, profile, process, environment, foreign)
                    quit_instance(profile, process)
                    checks.append("真实嵌套子树停止及运行时替换后，同profile立即恢复固定桥接")

                # 未标记专用目录不允许被接管，更不能设置其ACL。
                unknown_profile = root / "未知归属 配置"
                unknown_profile.mkdir()
                unknown = unknown_profile / "collection-managed"
                unknown.mkdir()
                sentinel_file = unknown / "用户原文.txt"
                sentinel_file.write_text("不得接管", encoding="utf-8")
                process = launch(unknown_profile, "managed")
                reply = wait_ready(binary, unknown_profile, process, environment, foreign)
                assert reply["collectionState"] == "failed"
                assert sentinel_file.read_text(encoding="utf-8") == "不得接管" and not (unknown / "owner.json").exists()
                quit_instance(unknown_profile, process)
                checks.append("拒绝未知目录，不写归属标记或改用户原文")
            finally:
                for process in reversed(processes):
                    if process.poll() is None:
                        process.kill()
                        process.wait(timeout=15)
        print(json.dumps({"passed": len(checks), "checks": checks, "realDependencyProbe": bool(args.python or args.runtime_manifest), "privateRuntime": bool(args.runtime_manifest), "realLoadedImages": loaded_images, "cleanWindowsVM": False, "publicSite": False}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
