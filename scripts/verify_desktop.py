"""在 Windows 隔离目录验证真实桌面二进制；不访问 Docker，不操作真实媒体或用户数据。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    # 直接执行入口时，在其他本地模块导入前关闭源码旁的字节码缓存。
    sys.dont_write_bytecode = True

import argparse
import ctypes
from ctypes import wintypes
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import socket
import shutil
import struct
import subprocess
import tempfile
import time
from typing import Any
import urllib.parse
import urllib.request

from verify import generate_password, make_sparse_video


def assert_icon_resources(binary: Path) -> list[int]:
    """只映射 EXE 资源，核对图标组及每个尺寸与仓库 ICO 的真实字节一致。"""
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.LoadLibraryExW.argtypes = [wintypes.LPCWSTR, wintypes.HANDLE, wintypes.DWORD]
    kernel.LoadLibraryExW.restype = wintypes.HMODULE
    kernel.FindResourceW.argtypes = [wintypes.HMODULE, ctypes.c_void_p, ctypes.c_void_p]
    kernel.FindResourceW.restype = wintypes.HANDLE
    kernel.SizeofResource.argtypes = [wintypes.HMODULE, wintypes.HANDLE]
    kernel.SizeofResource.restype = wintypes.DWORD
    kernel.LoadResource.argtypes = [wintypes.HMODULE, wintypes.HANDLE]
    kernel.LoadResource.restype = wintypes.HANDLE
    kernel.LockResource.argtypes = [wintypes.HANDLE]
    kernel.LockResource.restype = ctypes.c_void_p
    kernel.FreeLibrary.argtypes = [wintypes.HMODULE]
    kernel.FreeLibrary.restype = wintypes.BOOL
    # 只按数据/资源加载，不执行入口；所有映射在程序启动和退出占用检查前释放。
    module = kernel.LoadLibraryExW(str(binary), None, 0x22)
    assert module, f"无法映射桌面 EXE 资源：Windows 错误 {ctypes.get_last_error()}"

    def resource(kind: int, identity: int) -> bytes:
        """从本次映射读取一个有界的整数命名资源。"""
        found = kernel.FindResourceW(module, identity, kind)
        assert found, f"桌面图标资源缺失：类型 {kind}，ID {identity}"
        length = kernel.SizeofResource(module, found)
        assert 0 < length <= 1024 * 1024, "图标资源大小无效"
        loaded = kernel.LoadResource(module, found)
        assert loaded, "无法加载桌面图标资源"
        pointer = kernel.LockResource(loaded)
        assert pointer, "无法读取桌面图标资源"
        return ctypes.string_at(pointer, length)

    try:
        source = (Path(__file__).resolve().parents[1] / "assets/windows/qiku.ico").read_bytes()
        reserved, kind, count = struct.unpack_from("<HHH", source)
        assert (reserved, kind, count) == (0, 1, 10), "图标源必须包含十种尺寸"
        group = resource(14, 1)
        assert group[:6] == source[:6] and len(group) == 6 + count * 14, "内嵌图标组与 ICO 不匹配"
        sizes: list[int] = []
        for index in range(count):
            entry = group[6 + index * 14:20 + index * 14]
            original = source[6 + index * 16:22 + index * 16]
            assert entry[:12] == original[:12], "内嵌图标帧描述与 ICO 不一致"
            size = entry[0] or 256
            assert size == (entry[1] or 256), "图标帧必须为正方形"
            length, offset = struct.unpack_from("<II", original, 8)
            identity = struct.unpack_from("<H", entry, 12)[0]
            assert resource(3, identity) == source[offset:offset + length], f"{size}px 内嵌图标内容不一致"
            sizes.append(size)
        assert sorted(sizes) == [16, 20, 24, 32, 40, 48, 64, 96, 128, 256], "内嵌图标尺寸不完整"
        return sorted(sizes)
    finally:
        assert kernel.FreeLibrary(module), "桌面资源映射未能释放"


def assert_tray_icons(process_id: int) -> None:
    """定位本轮进程的原生托盘窗口，确认独立大小图标已绑定到窗口类。"""
    user = ctypes.WinDLL("user32", use_last_error=True)
    callback_type = ctypes.WINFUNCTYPE(wintypes.BOOL, wintypes.HWND, wintypes.LPARAM)
    user.EnumWindows.argtypes = [callback_type, wintypes.LPARAM]
    user.EnumWindows.restype = wintypes.BOOL
    user.GetWindowThreadProcessId.argtypes = [wintypes.HWND, ctypes.POINTER(wintypes.DWORD)]
    user.GetWindowThreadProcessId.restype = wintypes.DWORD
    user.GetClassNameW.argtypes = [wintypes.HWND, wintypes.LPWSTR, ctypes.c_int]
    user.GetClassNameW.restype = ctypes.c_int
    user.GetClassLongPtrW.argtypes = [wintypes.HWND, ctypes.c_int]
    user.GetClassLongPtrW.restype = ctypes.c_void_p
    windows: list[int] = []

    @callback_type
    def collect(window: int, _extra: int) -> bool:
        """只记录目标 PID 的栖库窗口，不操作其他桌面应用。"""
        owner = wintypes.DWORD()
        user.GetWindowThreadProcessId(window, ctypes.byref(owner))
        if owner.value == process_id:
            name = ctypes.create_unicode_buffer(256)
            user.GetClassNameW(window, name, len(name))
            if name.value == "QikuDesktopLauncher":
                windows.append(window)
        return True

    assert user.EnumWindows(collect, 0), "无法枚举本轮桌面窗口"
    assert len(windows) == 1, "未找到本轮唯一的原生托盘窗口"
    large = user.GetClassLongPtrW(windows[0], -14)  # GCLP_HICON：窗口类大图标。
    small = user.GetClassLongPtrW(windows[0], -34)  # GCLP_HICONSM：窗口类小图标。
    assert large and small and large != small, "原生窗口没有使用独立的品牌大小图标"


def execute(binary: Path, arguments: list[str], environment: dict[str, str], cwd: Path) -> subprocess.CompletedProcess[str]:
    """以参数数组运行固定测试程序，避免 shell 解释中文、空格和分号路径。"""
    return subprocess.run([str(binary), *arguments], env=environment, cwd=cwd, capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=20)


def control(binary: Path, profile: Path, action: str, environment: dict[str, str], cwd: Path) -> dict[str, Any]:
    """通过真实当前用户命名管道执行固定 status/quit 动作。"""
    result = execute(binary, ["--headless", "--profile", str(profile), "--control", action], environment, cwd)
    assert result.returncode == 0, f"本机控制失败：{result.stderr[-1500:]}"
    return json.loads(result.stdout)


def wait_ready(binary: Path, profile: Path, process: subprocess.Popen[bytes], environment: dict[str, str], cwd: Path) -> dict[str, Any]:
    """等待本实例真实就绪，进程提前退出或失败时立即报告。"""
    deadline = time.monotonic() + 45
    last_error: str = ""
    while time.monotonic() < deadline:
        assert process.poll() is None, f"桌面服务提前退出，返回码 {process.returncode}"
        try:
            reply = control(binary, profile, "status", environment, cwd)
            if reply["state"] == "ready":
                return reply
            assert reply["state"] != "failed", reply.get("error", "启动失败")
        except (AssertionError, json.JSONDecodeError) as error:
            last_error = str(error)
        time.sleep(0.15)
    raise AssertionError(f"等待就绪超时：{last_error}")


def request(opener: urllib.request.OpenerDirector, base: str, path: str, body: dict[str, Any] | None = None) -> tuple[int, bytes]:
    """使用同源 HTTP 访问有限 JSON 接口，账号口令只存在隔离测试进程中。"""
    data = json.dumps(body, ensure_ascii=False).encode("utf-8") if body is not None else None
    headers = {"Origin": base, "Content-Type": "application/json"} if body is not None else {}
    with opener.open(urllib.request.Request(base + path, data=data, headers=headers), timeout=5) as response:
        return response.status, response.read(1 << 20)


def assert_exclusive(path: Path, available: bool = True) -> None:
    """用 Windows 零共享读写句柄验证本轮样本文件占用，绝不修改文件内容。"""
    assert path.is_file(), f"文件释放检查目标不存在：{path.name}"
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    create_file = kernel.CreateFileW
    create_file.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD, ctypes.c_void_p, wintypes.DWORD, wintypes.DWORD, wintypes.HANDLE]
    create_file.restype = wintypes.HANDLE
    close_handle = kernel.CloseHandle
    close_handle.argtypes = [wintypes.HANDLE]
    close_handle.restype = wintypes.BOOL
    # GENERIC_READ|GENERIC_WRITE、零共享、OPEN_EXISTING：现有读句柄或映像占用均不能假报释放。
    handle = create_file(str(path), 0xC0000000, 0, None, 3, 0x80, None)
    if handle == wintypes.HANDLE(-1).value:
        error = ctypes.get_last_error()
        assert not available and error in (32, 33), f"文件未完全释放：{path.name}，Windows 错误 {error}"
        return
    assert close_handle(handle), f"验证句柄关闭失败：{path.name}"
    assert available, f"慢流验证未建立持续文件占用：{path.name}"


def wait_item(opener: urllib.request.OpenerDirector, base: str, library_id: int, name: str) -> int:
    """等待本轮小说库扫描结果，失败时给出任务状态而不读取用户数据。"""
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        _, payload = request(opener, base, f"/api/items?libraryId={library_id}")
        for item in json.loads(payload)["items"]:
            if item["title"] == name:
                return int(item["id"])
        time.sleep(0.1)
    _, payload = request(opener, base, "/api/jobs")
    raise AssertionError(f"等待隔离文件索引超时：{payload.decode('utf-8')[:1500]}")


def start_slow_stream(base: str, item_id: int, jar: http.cookiejar.CookieJar, length: int) -> socket.socket:
    """建立认证的大文件慢读连接，保留小接收窗口使服务退出时仍有活动请求。"""
    address = urllib.parse.urlparse(base)
    cookie_request = urllib.request.Request(f"{base}/api/items/{item_id}/file")
    jar.add_cookie_header(cookie_request)
    stream = socket.create_connection((str(address.hostname), int(address.port or 80)), timeout=5)
    try:
        stream.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 1024)
        cookie = cookie_request.get_header("Cookie", "")
        message = f"GET /api/items/{item_id}/file HTTP/1.1\r\nHost: {address.netloc}\r\nCookie: {cookie}\r\nConnection: keep-alive\r\n\r\n"
        stream.sendall(message.encode("ascii"))
        headers = b""
        while b"\r\n\r\n" not in headers:
            part = stream.recv(1024)
            assert part, "大文件响应在返回头部前已关闭"
            headers += part
            assert len(headers) <= 32 << 10, "大文件响应头超出预算"
        header = headers.split(b"\r\n\r\n", 1)[0].lower()
        assert header.startswith(b"http/1.1 200 ") and f"content-length: {length}".encode() in header
        return stream
    except BaseException:
        stream.close()
        raise


def assert_sparse_source(path: Path, length: int) -> None:
    """核对稀疏源长度与三个边界标记，避免只验证原文而漏掉慢流文件被改写。"""
    assert path.stat().st_size == length, "退出流程改写了稀疏源文件长度"
    with path.open("rb") as source:
        for offset in (0, 4 * 1024 ** 3 + 17, length - 64):
            source.seek(offset)
            assert source.read(64) == bytes(range(64)), f"退出流程改写了稀疏源文件标记：{offset}"


def file_digest(path: Path) -> str:
    """以流式 SHA-256 固定启动前构建产物摘要，验证原位覆盖保留完整可执行内容。"""
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def assert_released(binary: Path, source_binary: Path, expected_digest: str, profile: Path, data_dir: Path, media: Path, base: str, environment: dict[str, str], cwd: Path, check_control: bool = True) -> None:
    """先验证 EXE 原位覆盖与程序目录替换，再检查附加资源；首次退出紧接真实重启。"""
    # 必须先检查文件；不存在的 IPC 查询会等待五秒，不能让该延时掩盖退出后的句柄残留。
    files = [binary, media, data_dir / "library.db", data_dir / ".nas-instance.lock", profile / "logs" / "desktop.log"]
    files.extend(sorted(path for path in data_dir.glob("library.db-*") if path.is_file()))
    for path in files:
        assert_exclusive(path)
        moved = path.with_name(path.name + ".exit-check")
        assert not moved.exists()
        path.rename(moved)
        moved.rename(path)
        if path == binary:
            # 在原有 EXE 路径实际写入构建产物，不能只以读写句柄或改名推断更新可用。
            shutil.copyfile(source_binary, binary)
            assert file_digest(binary) == expected_digest, "退出后原位覆盖的 EXE 与启动前构建产物不一致"
            program_directory = binary.parent
            moved_directory = program_directory.with_name(program_directory.name + ".exit-check")
            assert not moved_directory.exists()
            program_directory.rename(moved_directory)
            moved_directory.rename(program_directory)
    address = urllib.parse.urlparse(base)
    try:
        connection = socket.create_connection((str(address.hostname), int(address.port or 80)), timeout=1)
    except OSError:
        pass
    else:
        connection.close()
        raise AssertionError("退出后 HTTP 仍接受连接")
    if check_control:
        status = execute(binary, ["--headless", "--profile", str(profile), "--control", "status"], environment, cwd)
        assert status.returncode != 0 and not status.stdout and "未找到可响应的已有实例" in status.stderr, "退出后的 IPC 未明确报告实例已不存在"


def main() -> None:
    """验证跨工作目录、实际端口保留、配置恢复、单实例和数据锁，结束后精确回收样本。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True, help="已构建的 Windows 桌面 EXE，推荐带内嵌网页的发布产物")
    parser.add_argument("--web-dir", type=Path, help="仅开发构建使用的显式前端目录；省略时验证包内嵌资源")
    parser.add_argument("--with-tray", action="store_true", help="启用真实原生托盘，并检查 EXE 十尺寸图标资源和窗口图标句柄")
    args = parser.parse_args()
    assert os.name == "nt", "此脚本需要真实 Windows；Linux 使用原有 verify.py"
    source_binary = args.binary.resolve(strict=True)
    icon_sizes: list[int] = assert_icon_resources(source_binary) if args.with_tray else []
    desktop_flags: list[str] = [] if args.with_tray else ["--headless"]
    environment = {key: value for key, value in os.environ.items() if not key.upper().startswith(("NAS_", "ENGINE_", "QBT_", "JACKETT_"))}
    if args.web_dir:
        environment["NAS_WEB_DIR"] = str(args.web_dir.resolve(strict=True))
    processes: list[subprocess.Popen[bytes]] = []
    exit_durations: list[float] = []
    slow_stream: socket.socket | None = None
    with tempfile.TemporaryDirectory(prefix="栖库 启动验证 ") as temporary:
        root = Path(temporary)
        binary = root / "程序 更新验证" / "栖库.exe"
        binary.parent.mkdir()
        shutil.copy2(source_binary, binary)
        expected_digest = file_digest(source_binary)
        assert file_digest(binary) == expected_digest, "隔离程序副本与原始构建产物不一致"
        profile = binary.parent / "qiku-data" / "profiles" / "native"
        media = root / "中文 资料;音乐"
        foreign = root / "任意 工作目录"
        media.mkdir()
        foreign.mkdir()
        forbidden_user_root = root / "不得回退的用户目录"
        environment["LOCALAPPDATA"] = str(forbidden_user_root)
        default_config = execute(binary, ["--headless", "--print-config"], environment, foreign)
        assert default_config.returncode == 0, f"默认桌面配置不可读取：{default_config.stderr[-1500:]}"
        default_data = Path(json.loads(default_config.stdout)["dataDir"])
        expected_default_data = binary.parent / "qiku-data" / "profiles" / "native" / "data"
        assert default_data == expected_default_data, "默认数据目录未落在 EXE 同级 qiku-data 根或受当前工作目录影响"
        docker_config = execute(binary, ["--headless", "--mode", "docker", "--print-config"], environment, foreign)
        assert docker_config.returncode == 0, f"Docker profile 配置不可读取：{docker_config.stderr[-1500:]}"
        docker_data = Path(json.loads(docker_config.stdout)["dataDir"])
        assert docker_data == binary.parent / "qiku-data" / "profiles" / "docker" / "data" and docker_data != default_data, "native 与 Docker 默认 profile 未隔离"
        environment_data = root / "环境业务目录"
        environment_override = dict(environment, NAS_DATA_DIR=str(environment_data))
        env_config = execute(binary, ["--headless", "--print-config"], environment_override, foreign)
        assert env_config.returncode == 0 and Path(json.loads(env_config.stdout)["dataDir"]) == environment_data, "NAS_DATA_DIR 未覆盖默认目录"
        override = root / "自定义业务目录"
        custom_profile = root / "自定义配置目录"
        profile_config = execute(binary, ["--headless", "--profile", str(custom_profile), "--print-config"], environment, foreign)
        assert profile_config.returncode == 0 and Path(json.loads(profile_config.stdout)["dataDir"]) == custom_profile / "data", "显式 --profile 未决定默认业务目录"
        custom_override = dict(environment, NAS_DATA_DIR=str(environment_data))
        custom_config = execute(binary, ["--headless", "--profile", str(custom_profile), "--data-dir", str(override), "--print-config"], custom_override, foreign)
        assert custom_config.returncode == 0 and Path(json.loads(custom_config.stdout)["dataDir"]) == override, "显式 --data-dir 未优先于 NAS_DATA_DIR"
        assert not profile.exists() and not custom_profile.exists(), "仅打印配置不应创建用户数据目录"
        (media / "只读 原文.txt").write_text("启动器验证保留原文。", encoding="utf-8")
        source_before = (media / "只读 原文.txt").read_bytes()
        # EPUB 后缀仅用于文件流协议样本，不交给阅读器解码，也不启动媒体探测工具。
        stream_file = media / "超大 慢流.epub"
        stream_length = make_sparse_video(stream_file)
        occupied = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        occupied.bind(("127.0.0.1", 0))
        preferred = occupied.getsockname()[1]
        assert preferred <= 65515
        occupied.listen()
        log_path = root / "process-output.txt"
        with log_path.open("wb") as output:
            try:
                command = [str(binary), *desktop_flags, "--no-browser", "--media-root", str(media), "--port", str(preferred)]
                process = subprocess.Popen(command, cwd=foreign, env=environment, stdout=output, stderr=output)
                processes.append(process)
                reply = wait_ready(binary, profile, process, environment, foreign)
                if args.with_tray:
                    assert_tray_icons(process.pid)
                address = reply["url"]
                assert reply["mode"] == "native"
                assert urllib.parse.urlparse(address).hostname == "127.0.0.1"
                actual_port = urllib.parse.urlparse(address).port
                assert preferred < actual_port < preferred + 20, "未避让真实被占用端口"
                jar = http.cookiejar.CookieJar()
                opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar))
                status, html = request(opener, address, "/")
                assert status == 200 and b"<html" in html.lower() and b"/assets/" in html
                _, payload = request(opener, address, "/api/status")
                assert json.loads(payload)["setupRequired"] is True
                _, payload = request(opener, address, "/api/setup", {"username": "desktop_check", "password": generate_password()})
                assert json.loads(payload)["user"]["username"] == "desktop_check"
                _, payload = request(opener, address, "/api/browse")
                assert [Path(item) for item in json.loads(payload)["roots"]] == [media], "分号目录授权被路径列表拆分"
                config_before = (profile / "desktop.json").read_bytes()
                config = json.loads(config_before)
                assert config["port"] == actual_port
                duplicate = execute(binary, ["--headless", "--no-browser"], environment, foreign)
                assert duplicate.returncode == 0 and process.poll() is None, duplicate.stderr
                assert (profile / "desktop.json").read_bytes() == config_before, "重复启动改写了已有配置"
                collision = execute(binary, ["--headless", "--no-browser", "--profile", str(root / "second-profile"), "--data-dir", config["dataDir"], "--media-root", str(media)], environment, foreign)
                assert collision.returncode != 0, "不同 profile 绕过同一 SQLite 数据目录锁"
                assert json.loads(request(opener, address, "/healthz")[1])["status"] == "ok"
                _, payload = request(opener, address, "/api/libraries", {"name": "退出慢流验证", "type": "novel", "path": str(media)})
                library_id = int(json.loads(payload)["library"]["id"])
                item_id = wait_item(opener, address, library_id, stream_file.stem)
                slow_stream = start_slow_stream(address, item_id, jar, stream_length)
                assert_exclusive(stream_file, available=False)
                started = time.monotonic()
                control(binary, profile, "quit", environment, foreign)
                assert process.wait(timeout=20) == 0
                exited = time.monotonic()
                assert_released(binary, source_binary, expected_digest, profile, Path(config["dataDir"]), stream_file, address, environment, foreign, check_control=False)
                restored = subprocess.Popen([str(binary), *desktop_flags, "--no-browser", "--profile", str(profile)], cwd=foreign, env=environment, stdout=output, stderr=output)
                processes.append(restored)
                exit_durations.append(round(exited - started, 3))
                assert exit_durations[-1] < 8, f"慢读请求导致退出耗尽等待预算：{exit_durations[-1]} 秒"
                slow_stream.close()
                slow_stream = None
                assert_sparse_source(stream_file, stream_length)
                restored_reply = wait_ready(binary, profile, restored, environment, foreign)
                if args.with_tray:
                    assert_tray_icons(restored.pid)
                assert restored_reply["url"] == address
                assert json.loads((profile / "desktop.json").read_bytes())["instanceId"] == config["instanceId"]
                _, payload = request(opener, address, "/api/status")
                assert json.loads(payload)["setupRequired"] is False, "本机重启丢失已有账号"
                started = time.monotonic()
                control(binary, profile, "quit", environment, foreign)
                assert restored.wait(timeout=20) == 0
                exited = time.monotonic()
                assert_released(binary, source_binary, expected_digest, profile, Path(config["dataDir"]), stream_file, address, environment, foreign)
                exit_durations.append(round(exited - started, 3))
                assert exit_durations[-1] < 8, f"空闲退出超过等待预算：{exit_durations[-1]} 秒"
                assert_sparse_source(stream_file, stream_length)
                bad_profile = root / "损坏 配置"
                bad_profile.mkdir()
                malformed = '{"schemaVersion":1,"instanceId":"' + "a" * 32 + '","mode":"native","Mode":"docker"}'
                (bad_profile / "desktop.json").write_text(malformed, encoding="utf-8")
                invalid = execute(binary, ["--headless", "--profile", str(bad_profile)], environment, foreign)
                assert invalid.returncode != 0 and (bad_profile / "desktop.json").read_text(encoding="utf-8") == malformed
                assert not (bad_profile / "data" / "library.db").exists(), "坏配置触发了数据库创建"
                invalid_cases: list[tuple[str, dict[str, Any]]] = []
                for field in ("schemaVersion", "instanceId", "mode", "dataDir", "mediaRoots", "port", "autoOpen", "dockerContext", "imageManifest"):
                    missing = dict(config)
                    del missing[field]
                    invalid_cases.append((f"missing-{field}", missing))
                    null_value = dict(config)
                    null_value[field] = None
                    invalid_cases.append((f"null-{field}", null_value))
                for label, value in (("empty-data", ""), ("relative-data", ".")):
                    invalid_data = dict(config)
                    invalid_data["dataDir"] = value
                    invalid_cases.append((label, invalid_data))
                for label, invalid_config in invalid_cases:
                    invalid_profile = root / "配置 完整性" / label
                    invalid_profile.mkdir(parents=True)
                    malformed_data = json.dumps(invalid_config, ensure_ascii=False)
                    config_path = invalid_profile / "desktop.json"
                    config_path.write_text(malformed_data, encoding="utf-8")
                    invalid = execute(binary, ["--headless", "--profile", str(invalid_profile)], environment, foreign)
                    assert invalid.returncode != 0, f"已保存配置缺失/null/空路径未拒绝：{label}"
                    assert config_path.read_text(encoding="utf-8") == malformed_data, f"坏配置被覆盖：{label}"
                    assert not (invalid_profile / "data" / "library.db").exists(), f"坏配置建立新库：{label}"
                assert not (foreign / "library.db").exists(), "空数据路径在任意工作目录建立了数据库"
                # 托盘重复启动会打开默认浏览器，其私有缓存不属于栖库用户配置。
                assert not (forbidden_user_root / "Qiku").exists(), "默认用户配置静默回退到 LOCALAPPDATA/Qiku"
                assert (media / "只读 原文.txt").read_bytes() == source_before
                if args.with_tray:
                    print(json.dumps({"nativeTray": "passed", "iconSizes": icon_sizes, "checks": ["内嵌图标逐帧匹配ICO", "首次启动与原位覆盖后重启的独立大小图标", "真实托盘退出后EXE立即释放"]}, ensure_ascii=False))
                print(json.dumps({"result": "passed", "platform": "windows", "resourceMode": "explicit-web-dir" if args.web_dir else "embedded", "exitSeconds": exit_durations, "checks": ["EXE同级默认profile与数据目录", "native/Docker默认profile隔离", "显式profile/data-dir覆盖", "中文/空格/分号路径", "任意工作目录", "端口冲突实际避让", "健康与HTML就绪", "真实命名管道", "重复启动", "同数据目录跨profile拒绝", "5GiB稀疏慢流中断与快速退出", "HTTP和IPC关闭", "EXE优先独占打开/原位覆盖/SHA-256校验", "完整程序目录重命名与恢复", "数据库/锁/日志/媒体独占打开与重命名", "空闲退出", "覆盖后立即重启配置与账号保留", "重复JSON键拒绝且不覆盖", "保存配置完整字段/null/空相对路径拒绝", "源文件保持"]}, ensure_ascii=False))
            except BaseException:
                output.flush()
                print(log_path.read_text(encoding="utf-8", errors="replace")[-12000:])
                raise
            finally:
                if slow_stream is not None:
                    slow_stream.close()
                for process in processes:
                    if process.poll() is None:
                        try:
                            control(binary, profile, "quit", environment, foreign)
                            process.wait(timeout=30)
                        except (AssertionError, subprocess.SubprocessError, OSError):
                            # 仅终止此脚本自己创建且尚未退出的隔离进程，不扫描其他进程。
                            process.kill()
                            process.wait(timeout=10)
                occupied.close()


if __name__ == "__main__":
    from project_env import run_cli

    raise SystemExit(run_cli(main))
