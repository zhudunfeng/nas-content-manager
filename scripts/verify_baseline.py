"""有界复用现有回归脚本，分开记录本机证据和真实 NAS 待验档案。"""

from __future__ import annotations

import sys

sys.dont_write_bytecode = True
if __name__ == "__main__":
    # 自检会反向导入本模块；共享同一异常/所有权类型，避免__main__被重复载入。
    sys.modules.setdefault("verify_baseline", sys.modules[__name__])

import argparse
import codecs
import ctypes
import hashlib
import json
import os
import platform
import re
import secrets
import shutil
import signal
import socket
import stat
import subprocess
import threading
import time
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from collections.abc import Iterator
from typing import Any

from project_env import ROOT, check_directory, configure_console, configure_environment, unique_object, verification_directory


CASES: dict[str, tuple[str, str]] = {
    "core": ("五媒体、鉴权、扫描、用户状态及数据库快照", "verify.py"),
    "account": ("账号迁移、凭据生命周期与受控维护验证", "verify_account.py"),
    "downloads": ("下载发布门禁与状态恢复", "verify_downloads.py"),
    "search": ("Torznab 搜索与原下载确认", "verify_torrent_search.py"),
    "engines": ("引擎接入协议与不确定动作", "verify_engine_bootstrap.py"),
    "external": ("固定外部协议及六阶段工作流", "verify_external.py"),
    "components": ("私密 helper 协议与组件状态", "verify_components.py"),
    "collection": ("真实 worker 与受控来源收集", "verify_collection.py"),
    "release": ("发布边界自检，不代表实际发布归档", "verify_release.py"),
}  # 可选择的现有脚本及其中文证据范围；不会传入真实 Docker/账号选项。
LOG_LIMIT: int = 4 * 1024 * 1024  # 每个持久日志最多四 MiB，超出后仍排空管道。
LINE_LIMIT: int = 64 * 1024  # 单行输出超过六十四 KiB 时整行省略，避免秘密跨截断边界泄露。
STATUSES: tuple[str, ...] = ("passed", "failed", "blocked", "not_run")  # 清单唯一允许的结果状态。


class Blocked(RuntimeError):
    """表示前置或安全所有权不足，不能计作已执行成功。"""


def now() -> str:
    """返回带时区的 UTC 时间，便于不同设备比较结果。"""
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds")


def digest(path: Path) -> str:
    """分块计算文件摘要，避免将可执行文件整体载入内存。"""
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(block)
    return value.hexdigest()


def display_path(path: Path) -> str:
    """仅暴露仓库相对路径或外部文件名，不导出用户目录。"""
    absolute = path.absolute()
    return absolute.relative_to(ROOT).as_posix() if absolute.is_relative_to(ROOT) else "<external>/" + absolute.name


def sanitize(value: str, known_secrets: tuple[str, ...] = ()) -> str:
    """遮蔽凭据、私密 URL 与绝对路径，保留可诊断的中文上下文。"""
    for secret in sorted(set(known_secrets), key=len, reverse=True):
        if len(secret) >= 4:
            value = value.replace(secret, "<redacted>")
    value = re.sub(r"(?im)^\s*(authorization|proxy-authorization|cookie|set-cookie)\s*:.*$", r"\1: <redacted>", value)
    value = re.sub(r"(?i)\bBearer\s+[^\s,;]+", "Bearer <redacted>", value)
    value = re.sub(r"(?i)([\"']?(?:password|passwd|token|secret|api[-_]?key|authorization|cookie)[\"']?\s*[:=]\s*)(?:\"[^\"]*\"|'[^']*'|[^\s,;}]+)", r"\1<redacted>", value)
    value = re.sub(r"(?i)https?://[^\s<>\"']+", "<redacted-url>", value)
    value = re.sub(re.escape(str(ROOT)), "<repo>", value, flags=re.IGNORECASE)
    value = value.replace(ROOT.as_posix(), "<repo>")
    # 未加引号的空格路径无法可靠判断终点，保守遮蔽到引号/行末，避免泄露后半段。
    value = re.sub(r"(?:\\\\|//)[^\\/\r\n]+[\\/][^\r\n\"'<>]+", "<absolute-path>", value)
    value = re.sub(r"(?i)\b[A-Z]:[\\/][^\r\n\"'<>]+", "<absolute-path>", value)
    value = re.sub(r"(?<![\w<>:])/(?!/)[^\r\n\"'<>]+", "<absolute-path>", value)
    return value


def sanitize_json(value: Any) -> Any:
    """递归遮蔽结构化值，保持JSON有效，不对序列化文本做替换。"""
    if isinstance(value, dict):
        return {key: "<redacted>" if re.search(r"(?i)password|passwd|token|secret|api[-_]?key|authorization|cookie", str(key)) else sanitize_json(item) for key, item in value.items()}
    if isinstance(value, list):
        return [sanitize_json(item) for item in value]
    return sanitize(value) if isinstance(value, str) else value


def safe_files(root: Path) -> Iterator[Path]:
    """仅遍历普通目录与文件，逐级剪枝符号链接和Windows重解析点。"""
    for current, directories, files in os.walk(root, followlinks=False):
        retained: list[str] = []
        for name in directories:
            metadata = (Path(current) / name).lstat()
            if stat.S_ISDIR(metadata.st_mode) and not stat.S_ISLNK(metadata.st_mode) and not getattr(metadata, "st_file_attributes", 0) & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 1024):
                retained.append(name)
        directories[:] = retained
        for name in files:
            path = Path(current) / name
            metadata = path.lstat()
            if stat.S_ISREG(metadata.st_mode) and not getattr(metadata, "st_file_attributes", 0) & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 1024):
                yield path


def write_json(path: Path, value: Any) -> None:
    """在同目录原子发布清单，读取者不会看到半份 JSON。"""
    temporary = path.with_name(path.name + ".writing")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    temporary.replace(path)


def new_run_root(requested: str | None, prefix: str = "baseline") -> Path:
    """创建严格位于 verify 下的新根，拒绝复用、上溯和链接祖先。"""
    parent = verification_directory()
    name = prefix + "-" + datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ") + "-" + secrets.token_hex(3)
    raw = Path(requested) if requested is not None else parent / name
    if ".." in raw.parts:
        raise Blocked("运行根不能包含上级目录")
    path = raw if raw.is_absolute() else ROOT / raw
    if path == parent or not path.is_relative_to(parent):
        raise Blocked("运行根必须是仓库 .runtime/verify 下的全新后代目录")
    reserved = {"CON", "PRN", "AUX", "NUL", *(f"COM{number}" for number in range(1, 10)), *(f"LPT{number}" for number in range(1, 10))}
    for part in path.relative_to(parent).parts:
        if part in ("", ".", "..") or part.endswith((".", " ")) or part.split(".", 1)[0].upper() in reserved or any(ord(char) < 32 or char in ':<>"|?*\\' for char in part):
            raise Blocked("运行根不能包含设备名、流名称或尾随点/空格等路径别名")
    check_directory(path.parent)
    if not path.parent.resolve().is_relative_to(parent.resolve()):
        raise Blocked("运行根规范父目录越出verify范围")
    if os.path.lexists(path):
        raise Blocked("运行根已存在，拒绝覆盖历史结果")
    path.mkdir(mode=0o700)
    check_directory(path)
    return path


def clean_environment() -> dict[str, str]:
    """隔离所有继承的 NAS 参数和代理，防止夹具误连用户实例。"""
    environment = {key: value for key, value in os.environ.items() if not key.upper().startswith("NAS_")}
    for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
        environment.pop(key, None)
    environment.update({"NO_PROXY": "*", "no_proxy": "*"})  # 所有本机协议夹具直接回环访问。
    return environment


class WindowsJob:
    """为单个挂起命令建立独占 Job，后代从首条指令前归属本次运行。"""

    def __init__(self) -> None:
        """声明原生接口并配置仅终止自有 Job 的关闭语义。"""
        from ctypes import wintypes

        class BasicLimits(ctypes.Structure):
            """对应 Windows 基本限制，字段顺序与系统 ABI 一致。"""
            _fields_ = [("processTime", ctypes.c_int64), ("jobTime", ctypes.c_int64), ("flags", wintypes.DWORD), ("minimumWorkingSet", ctypes.c_size_t), ("maximumWorkingSet", ctypes.c_size_t), ("activeLimit", wintypes.DWORD), ("affinity", ctypes.c_size_t), ("priority", wintypes.DWORD), ("scheduling", wintypes.DWORD)]  # 时间、限制标志、工作集、进程数和调度参数。

        class Limits(ctypes.Structure):
            """对应扩展限制，IO计数与内存字段仅用于正确布局。"""
            _fields_ = [("basic", BasicLimits), ("io", ctypes.c_uint64 * 6), ("processMemory", ctypes.c_size_t), ("jobMemory", ctypes.c_size_t), ("peakProcess", ctypes.c_size_t), ("peakJob", ctypes.c_size_t)]  # 基本限制、IO计数及四个内存预算/峰值。

        self.limits_type: Any = Limits  # 动态平台结构，仅在 Windows 初始化。
        self.kernel: Any = ctypes.WinDLL("kernel32", use_last_error=True)  # Job 与等待句柄接口。
        self.native: Any = ctypes.WinDLL("ntdll")  # 恢复当前自有挂起进程，不枚举其他进程线程。
        signatures = {
            "CreateJobObjectW": ([ctypes.c_void_p, wintypes.LPCWSTR], wintypes.HANDLE),
            "SetInformationJobObject": ([wintypes.HANDLE, ctypes.c_int, ctypes.c_void_p, wintypes.DWORD], wintypes.BOOL),
            "AssignProcessToJobObject": ([wintypes.HANDLE, wintypes.HANDLE], wintypes.BOOL),
            "QueryInformationJobObject": ([wintypes.HANDLE, ctypes.c_int, ctypes.c_void_p, wintypes.DWORD, ctypes.c_void_p], wintypes.BOOL),
            "TerminateJobObject": ([wintypes.HANDLE, wintypes.UINT], wintypes.BOOL),
            "OpenProcess": ([wintypes.DWORD, wintypes.BOOL, wintypes.DWORD], wintypes.HANDLE),
            "WaitForSingleObject": ([wintypes.HANDLE, wintypes.DWORD], wintypes.DWORD),
            "CloseHandle": ([wintypes.HANDLE], wintypes.BOOL),
        }  # 每个原生函数的参数/返回类型，避免64位句柄截断。
        for name, (arguments, result) in signatures.items():
            function = getattr(self.kernel, name)
            function.argtypes, function.restype = arguments, result
        self.native.NtResumeProcess.argtypes = [wintypes.HANDLE]
        self.native.NtResumeProcess.restype = ctypes.c_long
        self.handle: int | None = self.kernel.CreateJobObjectW(None, None)  # 当前命令独占且不可继承的 Job 句柄。
        if not self.handle:
            raise Blocked("无法创建安全进程树")
        limits = Limits()
        limits.basic.flags = 0x2000  # JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE，不允许后代逃逸。
        if not self.kernel.SetInformationJobObject(self.handle, 9, ctypes.byref(limits), ctypes.sizeof(limits)):
            self.kernel.CloseHandle(self.handle)
            self.handle = None
            raise Blocked("无法配置安全进程树")

    def attach_and_resume(self, process: subprocess.Popen[bytes]) -> None:
        """先归组再恢复挂起进程，失败时不允许无约束继续运行。"""
        handle = int(process._handle)  # Python 保存的自有进程句柄，不通过PID查找用户进程。
        if not self.kernel.AssignProcessToJobObject(self.handle, handle):
            raise Blocked("无法取得自有进程树所有权，拒绝降级运行")
        if self.native.NtResumeProcess(handle) != 0:
            raise Blocked("无法恢复自有挂起进程")

    def stop(self) -> None:
        """封闭后代准入、保存等待句柄并回收整棵自有树。"""
        if self.handle is None:
            return
        handles: list[int] = []
        errors: list[str] = []
        try:
            limits = self.limits_type()
            limits.basic.flags = 0x2000 | 0x0008  # 关闭时终止且不再允许新进程加入。
            limits.basic.activeLimit = 0
            if not self.kernel.SetInformationJobObject(self.handle, 9, ctypes.byref(limits), ctypes.sizeof(limits)):
                errors.append("无法封闭自有进程树准入")
            for capacity in (16, 64, 256, 1024, 4096, 16384, 65536):
                storage = (ctypes.c_size_t * (capacity + 2))()
                if self.kernel.QueryInformationJobObject(self.handle, 3, storage, 8 + capacity * ctypes.sizeof(ctypes.c_size_t), None):
                    count = ctypes.c_uint32.from_buffer(storage, 4).value
                    if count > capacity:
                        raise RuntimeError("自有进程列表越过等待预算")
                    ids = (ctypes.c_size_t * count).from_buffer(storage, 8)
                    for pid in ids:
                        wait_handle = self.kernel.OpenProcess(0x00100000, False, pid)  # 只申请 SYNCHRONIZE 权限。
                        if wait_handle:
                            handles.append(wait_handle)
                        elif ctypes.get_last_error() != 87:  # 已退出的PID允许不存在。
                            errors.append("无法取得自有进程等待句柄")
                    break
                if ctypes.get_last_error() != 234:
                    raise RuntimeError("无法读取自有进程列表")
            else:
                raise RuntimeError("自有进程数量超过安全上限")
            if not self.kernel.TerminateJobObject(self.handle, 1):
                errors.append("无法终止自有进程树")
            deadline = time.monotonic() + 8
            for handle in handles:
                remaining = max(0, int((deadline - time.monotonic()) * 1000))
                if self.kernel.WaitForSingleObject(handle, remaining) != 0:
                    errors.append("自有后代未在预算内释放")
            # 对应 JOBOBJECT_BASIC_ACCOUNTING_INFORMATION，ActiveProcesses位于第40字节。
            accounting = (ctypes.c_byte * 48)()
            while True:
                if not self.kernel.QueryInformationJobObject(self.handle, 1, accounting, ctypes.sizeof(accounting), None):
                    errors.append("无法确认自有进程树清空")
                    break
                if ctypes.c_uint32.from_buffer(accounting, 40).value == 0:
                    break
                if time.monotonic() >= deadline:
                    errors.append("自有进程树清空超时")
                    break
                time.sleep(0.02)
        finally:
            # Kill-on-close是异常路径的最后一道保险，只影响当前Job。
            self.kernel.CloseHandle(self.handle)
            self.handle = None
            for handle in handles:
                self.kernel.CloseHandle(handle)
        if errors:
            raise RuntimeError("；".join(errors))


class OwnedProcess:
    """保存命令、限长脱敏日志与独占子进程树的统一生命周期。"""

    def __init__(self, command: list[str], environment: dict[str, str], log: Path) -> None:
        """创建独占进程树并在开始执行前安排输出排空。"""
        self.command: list[str] = command  # 原始命令只在内存执行，清单保存脱敏副本。
        self.log: Path = log  # 此命令唯一的可分享脱敏日志。
        self.started: float = time.monotonic()  # 墙钟超时与持续时间起点。
        self.tail: list[str] = []  # 仅保存尾部成功标记，避免子进程输出耗尽内存。
        self.lines: list[str] = []  # 至多两千条已脱敏断言，独立作为证据。
        self.reader_error: str = ""  # 日志线程失败必须反映在命令结果。
        self.job: WindowsJob | None = WindowsJob() if os.name == "nt" else None  # Windows独占Job，POSIX使用新session。
        self.closed: bool = False  # 保证清理幂等，不回收后来复用的进程组。
        log.parent.mkdir(parents=True, exist_ok=True)
        try:
            self.process: subprocess.Popen[bytes] = subprocess.Popen(command, cwd=ROOT, env=environment, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL, creationflags=(0x00000004 | 0x08000000) if os.name == "nt" else 0, start_new_session=os.name != "nt")
            if self.job:
                self.job.attach_and_resume(self.process)
        except BaseException:
            if hasattr(self, "process"):
                self.process.kill()
                self.process.wait(timeout=8)
                if self.process.stdout:
                    self.process.stdout.close()
            if self.job:
                self.job.stop()
            raise
        self.reader: threading.Thread = threading.Thread(target=self.drain, daemon=True)  # 输出排空不能阻塞命令超时控制。
        self.reader.start()

    def drain(self) -> None:
        """持续读取有界行并持久脱敏日志，超量也继续排空管道。"""
        try:
            if self.process.stdout is None:
                raise RuntimeError("子进程输出管道不可用")
            decoder = codecs.getincrementaldecoder("utf-8")("replace")
            pending, dropping, written = "", False, 0
            with self.log.open("w", encoding="utf-8") as output:
                while True:
                    block = self.process.stdout.read1(8192)
                    pending += decoder.decode(block, final=not block)
                    while "\n" in pending or (not block and pending):
                        if "\n" in pending:
                            line, pending = pending.split("\n", 1)
                        else:
                            line, pending = pending, ""
                        safe = "[超长输出已省略]" if dropping or len(line) > LINE_LIMIT else sanitize(line)
                        dropping = False
                        self.tail = (self.tail + [safe])[-300:]
                        if ("✓" in safe or "通过：" in safe) and len(self.lines) < 2000:
                            self.lines.append(safe)
                        size = len((safe + "\n").encode("utf-8"))
                        if written + size <= LOG_LIMIT:
                            output.write(safe + "\n")
                            written += size
                        elif written <= LOG_LIMIT:
                            notice = "[日志超出上限，后续输出仅排空]\n"
                            if written + len(notice.encode("utf-8")) <= LOG_LIMIT:
                                output.write(notice)
                            written = LOG_LIMIT + 1
                    if len(pending) > LINE_LIMIT:
                        pending, dropping = "", True
                    if not block:
                        break
        except (OSError, RuntimeError) as error:
            self.reader_error = sanitize(str(error))

    def stop(self) -> None:
        """在正常、失败和超时路径回收自有后代并等待日志关闭。"""
        if self.closed:
            return
        self.closed = True
        error: Exception | None = None
        try:
            if self.job:
                self.job.stop()
            else:
                try:
                    os.killpg(self.process.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass  # 自有session已经全部结束，不查找或接管其他进程。
                try:
                    self.process.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    pass  # 稍后对同一自有进程组发强制结束。
                try:
                    os.killpg(self.process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            self.process.wait(timeout=8)
        except Exception as failure:
            error = failure
        finally:
            self.reader.join(timeout=8)
            if self.reader.is_alive():
                error = RuntimeError("自有后代输出管道未释放")
            elif self.process.stdout:
                self.process.stdout.close()
        if self.reader_error:
            error = RuntimeError("无法保存子命令日志：" + self.reader_error)
        if error:
            raise error


def run_command(command: list[str], root: Path, label: str, timeout: float, environment: dict[str, str] | None = None) -> tuple[dict[str, Any], list[str], list[str]]:
    """有界执行一条命令，失败也保留退出码、超时和清理证据。"""
    log = root / "logs" / (label + ".log")
    record: dict[str, Any] = {"argv": [sanitize(argument) for argument in command], "exitCode": None, "timedOut": False, "log": log.relative_to(root).as_posix(), "durationSeconds": 0.0}  # 对外命令记录，不保存原始环境或凭据。
    started = time.monotonic()
    owned: OwnedProcess | None = None
    try:
        owned = OwnedProcess(command, environment or clean_environment(), log)
        try:
            record["exitCode"] = owned.process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            record["timedOut"] = True
    except Blocked as error:
        record.update({"startBlocked": True, "error": sanitize(str(error))})
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        record["error"] = sanitize(str(error))
    finally:
        if owned:
            try:
                owned.stop()
            except (OSError, RuntimeError, subprocess.SubprocessError) as error:
                record["cleanupError"] = sanitize(str(error))
            record["exitCode"] = owned.process.returncode
        record["durationSeconds"] = round(time.monotonic() - started, 3)
    if not log.exists():
        log.parent.mkdir(parents=True, exist_ok=True)
        log.write_text(record.get("error", "命令未能启动") + "\n", encoding="utf-8")
    return record, owned.tail if owned else [], owned.lines if owned else []


def file_identity(path: Path) -> dict[str, Any]:
    """为实际使用的普通产物保存大小与摘要，不以文件名代替身份。"""
    if path.is_symlink() or not path.is_file():
        raise Blocked("缺少普通文件产物：" + display_path(path))
    return {"path": display_path(path), "bytes": path.stat().st_size, "sha256": digest(path)}


def source_identity() -> dict[str, Any]:
    """限时采集 Git 基准与修改计数，不保存原始私密路径。"""
    try:
        commit = subprocess.run(["git", "rev-parse", "HEAD"], cwd=ROOT, capture_output=True, text=True, timeout=5, check=True).stdout.strip()
        changes = subprocess.run(["git", "status", "--porcelain"], cwd=ROOT, capture_output=True, text=True, timeout=5, check=True).stdout.splitlines()
        return {"commit": commit, "dirty": bool(changes), "changedEntries": len(changes)}
    except (OSError, subprocess.SubprocessError):
        return {"commit": None, "dirty": None, "changedEntries": None, "reason": "Git元数据不可用，不能声称干净版本"}


def case_record(name: str, selected: bool) -> dict[str, Any]:
    """为选中和未选中的用例都建立显式状态，初始绝不填通过。"""
    return {"id": name, "description": CASES.get(name, (name, ""))[0], "selected": selected, "status": "not_run", "startedAt": None, "finishedAt": None, "reason": "尚未执行" if selected else "本次未选择", "assertions": 0, "evidenceKinds": [], "commands": [], "evidence": []}


def save_manifest(root: Path, manifest: dict[str, Any], finished: bool = False) -> int:
    """按失败优先聚合四态，独立未选项不被算成成功。"""
    statuses = [case["status"] for case in manifest["cases"] if case["selected"]]
    status = "failed" if "failed" in statuses else "blocked" if "blocked" in statuses or "not_run" in statuses else "passed"
    manifest["status"] = status
    if finished:
        manifest["finishedAt"] = now()
    write_json(root / "run-manifest.json", manifest)
    return {"passed": 0, "failed": 1, "blocked": 2}[status]


def prerequisites(args: argparse.Namespace, case: dict[str, Any], manifest: dict[str, Any], root: Path) -> Path:
    """逐项检查实际产物和工具；独立release不依赖主服务构建。"""
    suffix = ".exe" if os.name == "nt" else ""
    binary = args.artifacts / ("nas" + suffix)
    if case["id"] != "release":
        manifest["artifacts"]["nas"] = file_identity(binary)
        if not (ROOT / "web/dist/index.html").is_file():
            raise Blocked("缺少 web/dist/index.html，请先构建网页")
        manifest["artifacts"]["webIndex"] = file_identity(ROOT / "web/dist/index.html")
    if case["id"] == "collection":
        manifest["artifacts"]["worker"] = file_identity(args.artifacts / ("nas-collection-worker" + suffix))
    if case["id"] == "components" and not shutil.which("go") and not (ROOT / ".tools/go/bin" / ("go" + suffix)).is_file():
        raise Blocked("components需要Go编译受控私密协议桥")
    if case["id"] == "components" and len(str(root / "components/h.sock").encode("utf-8")) > 100:
        raise Blocked("components私密socket全路径超过100字节，请显式使用更短的新运行根")
    if case["id"] == "core":
        for name in ("ffmpeg", "ffprobe"):
            tool: Path | None = getattr(args, name)
            if tool is None or not tool.is_absolute():
                raise Blocked("core要求显式绝对路径 --" + name + "，否则五媒体断言不足")
            identity = file_identity(tool)
            record, tail, _ = run_command([str(tool), "-version"], root, name + "-version", 10)
            case["commands"].append(record)
            if record.get("cleanupError"):
                require_command(record)
            if record["exitCode"] != 0 or record["timedOut"] or record.get("error") or record.get("cleanupError"):
                raise Blocked(name + "不能运行")
            identity.update({"version": next((line for line in tail if "version" in line.lower()), "未知"), "versionCommand": record})
            manifest["tools"][name] = identity
    return binary


def reserve_port(port: int) -> socket.socket:
    """只占用尚未使用的回环端口，不接管或停止既有监听器。"""
    listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        if os.name == "nt":
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
        listener.bind(("127.0.0.1", port))
        return listener
    except OSError as error:
        listener.close()
        raise Blocked("指定回环端口不可用，现有监听器未被操作") from error


def wait_ready(owned: OwnedProcess, base: str, marker: str, token: bytes, timeout: float) -> None:
    """同时验证自有进程、数据库健康和专属静态探针才允许业务断言。"""
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if owned.process.poll() is not None:
            raise RuntimeError("隔离主服务在就绪前退出，请查看脱敏应用日志")
        try:
            with opener.open(base + "/healthz", timeout=min(1, max(0.1, deadline - time.monotonic()))) as response:
                health = json.loads(response.read(4096))
            with opener.open(base + "/" + marker, timeout=min(1, max(0.1, deadline - time.monotonic()))) as response:
                probe = response.read(256)
            if health == {"status": "ok"} and probe == token and owned.process.poll() is None:
                return
        except (OSError, ValueError):
            pass  # 启动中暂未监听；只在有限预算内重试，不把HTTP200直接当作本实例。
        time.sleep(0.1)
    raise RuntimeError("隔离主服务就绪超时或专属实例标识不匹配")


def core_case(args: argparse.Namespace, root: Path, binary: Path, case: dict[str, Any]) -> None:
    """启动独立主服务并复用原五媒体断言，退出时收回全部自有后代。"""
    area = root / "core"
    area.mkdir()
    for name in ("data", "media"):
        (area / name).mkdir()
    shutil.copytree(ROOT / "web/dist", area / "web")
    token = secrets.token_hex(24).encode("ascii")
    marker = "baseline-" + secrets.token_hex(12) + ".txt"
    (area / "web" / marker).write_bytes(token)
    environment = clean_environment()
    listener = reserve_port(args.port)
    port = listener.getsockname()[1]
    base = "http://127.0.0.1:" + str(port)
    case["port"] = port
    environment.update({
        "NAS_ADDR": "127.0.0.1:" + str(port),  # 此次独占回环监听地址。
        "NAS_DATA_DIR": str(area / "data"),  # 本次SQLite/缓存，不复用用户数据。
        "NAS_MEDIA_ROOTS": str(area / "media"),  # 仅授权本次五媒体夹具。
        "NAS_WEB_DIR": str(area / "web"),  # 本次构建网页副本与独有就绪探针。
        "NAS_FFMPEG_PATH": str(args.ffmpeg),  # 用户显式选择的真实本机媒体生成工具。
        "NAS_FFPROBE_PATH": str(args.ffprobe),  # 同次媒体元数据探测工具。
        "NAS_SCAN_INTERVAL_MINUTES": "60",  # 断言显式触发扫描，避免短周期噪音。
        "NAS_MIN_FREE_MB": "0",  # 受控小夹具不要求生产预留空间。
        "NAS_COOKIE_SECURE": "false",  # 仅此回环HTTP实例允许普通Cookie。
        "NAS_DOWNLOAD_MODE": "disabled",  # 不连接现有下载器或启动Docker。
        "NAS_TORRENT_SEARCH_MODE": "disabled",  # 不访问真实搜索站点。
    })
    owned: OwnedProcess | None = None
    application: dict[str, Any] = {"argv": [display_path(binary)], "exitCode": None, "timedOut": False, "log": "logs/core-application.log", "durationSeconds": 0.0, "role": "isolated-owned-service"}  # 服务清理退出不冒充验证脚本断言结果。
    case["commands"].append(application)
    try:
        # 原CLI不能接收预绑定socket；随机文件挑战阻止竞争端口上的第三方被误判。
        listener.close()
        owned = OwnedProcess([str(binary)], environment, root / application["log"])
        wait_ready(owned, base, marker, token, args.startup_timeout)
        case["readiness"] = {"ownedProcessAlive": True, "databaseHealth": True, "uniqueStaticProbe": True}
        command = [sys.executable, "-B", str(ROOT / "scripts/verify.py"), "--root", str(area / "media"), "--base-url", base, "--ffmpeg", str(args.ffmpeg)]
        if args.large_file:
            command.append("--large-file")
        record, tail, lines = run_command(command, root, "core", args.timeout, environment)
        case["commands"].append(record)
        require_command(record)
        match = re.search(r"集成验证通过：(\d+)\s*项断言", "\n".join(tail))
        if match is None or int(match[1]) <= 0 or not (area / "verified-backup.db").is_file():
            raise RuntimeError("core缺少断言计数或实际SQLite备份，不能仅凭exit0通过")
        case["assertions"] = int(match[1])
        evidence = root / "evidence/core-assertions.json"
        write_json(evidence, {"count": int(match[1]), "assertions": lines, "backup": file_identity(area / "verified-backup.db")})
        case["evidence"] = [evidence.relative_to(root).as_posix(), "logs/core.log", "logs/core-application.log"]
        case["evidenceKinds"] = ["local-real-application", "generated-media", "local-real-ffmpeg", "sqlite-snapshot"] + (["sparse-http-offset-only"] if args.large_file else [])
    finally:
        listener.close()
        if owned:
            try:
                owned.stop()
                application["cleanup"] = "owned-tree-reclaimed"
            except (OSError, RuntimeError, subprocess.SubprocessError) as error:
                application["cleanupError"] = sanitize(str(error))
                raise
            finally:
                application.update({"exitCode": owned.process.returncode, "durationSeconds": round(time.monotonic() - owned.started, 3)})


def require_command(record: dict[str, Any]) -> None:
    """强制区分子进程失败、超时和成功，不能吞掉非零退出。"""
    if record.get("cleanupError"):
        raise RuntimeError("子验证自有进程回收失败：" + record["cleanupError"])
    if record.get("startBlocked"):
        raise Blocked(record["error"])
    if record.get("error"):
        raise RuntimeError("子验证无法执行：" + record["error"])
    if record["timedOut"]:
        raise RuntimeError("子验证达到墙钟超时，已回收自有进程树")
    if record["exitCode"] != 0:
        raise RuntimeError("子验证退出码非零：" + str(record["exitCode"]))


def domain_case(args: argparse.Namespace, root: Path, binary: Path, case: dict[str, Any]) -> None:
    """按各现有脚本真实参数执行，保持原脚本独立入口与断言。"""
    name = case["id"]
    command = [sys.executable, "-B", str(ROOT / "scripts" / CASES[name][1])]
    if name == "release":
        command.append("--self-test")
    else:
        command += ["--binary", str(binary), "--root", str(root / name)]
    if name == "collection":
        command += ["--worker-binary", str(args.artifacts / ("nas-collection-worker.exe" if os.name == "nt" else "nas-collection-worker"))]
    if name == "external":
        command.append("--workflow")
    if name == "account":
        command.append("--offline")
    record, tail, lines = run_command(command, root, name, args.timeout)
    case["commands"].append(record)
    require_command(record)
    if name == "release":
        match = re.search(r"发布边界回归通过：(\d+)项", "\n".join(tail))
        if match is None:
            raise RuntimeError("发布自检缺少实际断言计数")
        count = int(match[1])
        evidence = root / "evidence/release-assertions.json"
        write_json(evidence, {"count": count, "assertions": lines, "scope": "发布边界自检，不是实际发布包通过"})
    else:
        result_file = root / name / "result.json"
        if not result_file.is_file() or result_file.is_symlink():
            raise RuntimeError("子验证未生成预期result.json")
        result = json.loads(result_file.read_text(encoding="utf-8"), object_pairs_hook=unique_object)
        count = result.get("assertions")
        if result.get("passed") is not True or type(count) is not int or count <= 0:
            raise RuntimeError("子验证结果未通过或缺少实际断言")
        if name == "account":
            require_account_coverage(result)
        evidence = root / "evidence" / (name + "-assertions.json")
        write_json(evidence, {"result": result, "assertions": lines})
    if count <= 0:
        raise RuntimeError("断言数量必须大于零")
    case["assertions"] = count
    case["evidence"] = [evidence.relative_to(root).as_posix(), record["log"]]
    case["evidenceKinds"] = ["release-fixtures"] if name == "release" else ["local-real-application", "controlled-protocol-fixtures"]
    if name == "account":
        case["evidenceKinds"] = ["local-real-application", "historical-schema-fixtures", "local-offline-maintenance"]
    if name == "collection":
        case["evidenceKinds"].append("local-real-worker")


def require_account_coverage(result: dict[str, Any]) -> None:
    """要求账号组合同时完成在线和离线验证，不能只凭聚合成功掩盖漏跑。"""
    offline = result.get("offlineVerification")
    online_count, offline_count = result.get("onlineAssertions"), result.get("offlineAssertions")
    if (result.get("accountMutationAPIs") is not True or result.get("trueSchema4Migration") is not True
            or result.get("offlineCLI") is not True or not isinstance(offline, dict)
            or offline.get("requested") is not True or offline.get("passed") is not True
            or type(online_count) is not int or online_count <= 0
            or type(offline_count) is not int or offline_count <= 0
            or type(offline.get("assertions")) is not int or offline["assertions"] != offline_count
            or type(result.get("assertions")) is not int or result["assertions"] != online_count + offline_count):
        raise RuntimeError("账号组合缺少完整在线、历史迁移或离线维护验证证据")


def sanitize_fixture_logs(root: Path) -> None:
    """结束后按夹具秘密二次脱敏日志，不发布本地凭据和数据库。"""
    secrets_found: list[str] = []
    for path in safe_files(root):
        if any(word in path.name.lower() for word in ("password", "token", "credential", "secret")) and path.stat().st_size < 64 * 1024:
            text = path.read_text(encoding="utf-8", errors="replace").strip()
            secrets_found.append(text)
            try:
                value = json.loads(text)
                if isinstance(value, dict):
                    secrets_found += [item for key, item in value.items() if isinstance(item, str) and any(word in key.lower() for word in ("password", "token", "secret", "key"))]
            except ValueError:
                pass  # 纯文本密码文件同样加入精确遮蔽集合。
    for path in safe_files(root):
        if path.suffix == ".log":
            with path.open("rb") as stream:
                data = stream.read(LOG_LIMIT)
            safe = sanitize(data.decode("utf-8", errors="replace"), tuple(secrets_found)).encode("utf-8")[:LOG_LIMIT].decode("utf-8", errors="ignore")
            path.write_text(safe, encoding="utf-8")


def validate_target(path: Path, root: Path, manifest: dict[str, Any]) -> None:
    """只检查真实设备档案结构和待验门禁，绝不将模板校验当作设备通过。"""
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 256 * 1024:
        raise Blocked("target-file必须是至多256KiB的普通JSON档案")
    value = json.loads(path.read_text(encoding="utf-8-sig"), object_pairs_hook=unique_object)
    template = json.loads((ROOT / "docs/nas-acceptance.template.json").read_text(encoding="utf-8"))
    if not isinstance(value, dict) or set(value) != set(template) or value.get("formatVersion") != 1:
        raise ValueError("目标档案顶层字段或formatVersion不正确")
    if type(value["formatVersion"]) is not int or not isinstance(value["description"], str) or not value["description"].strip() or (value["targetId"] is not None and (not isinstance(value["targetId"], str) or not value["targetId"].strip())):
        raise ValueError("目标档案版本必须是整数，description需有说明，targetId为非空文字或null")
    for group in ("environment", "measurements"):
        entries = value[group]
        if not isinstance(entries, dict) or set(entries) != set(template[group]):
            raise ValueError("目标档案缺少字段组：" + group)
        for entry in entries.values():
            if not isinstance(entry, dict) or set(entry) != {"description", "value"} or not isinstance(entry["description"], str) or not entry["description"].strip():
                raise ValueError("目标档案字段必须含中文用途说明和可为null的value")
    if not isinstance(value["gates"], list) or len(value["gates"]) != 3:
        raise ValueError("目标档案必须保留M1/M2/M3三个门禁")
    for gate, expected in zip(value["gates"], template["gates"], strict=True):
        if not isinstance(gate, dict) or set(gate) != set(expected) or gate["id"] != expected["id"]:
            raise ValueError("目标门禁字段或顺序不正确")
        if gate["status"] not in ("blocked", "not_run"):
            raise ValueError("target v1只允许待验状态，不支持导入通过声明")
        if gate["owner"] is not None and (not isinstance(gate["owner"], str) or not gate["owner"].strip()):
            raise ValueError("目标责任人必须为非空文字或null")
        if not isinstance(gate["requiredInputs"], list) or not gate["requiredInputs"] or not all(isinstance(item, str) and item.strip() for item in gate["requiredInputs"]):
            raise ValueError("目标门禁必须明确所需输入")
        case = case_record(gate["id"], True)
        case.update({"status": gate["status"], "reason": "真实设备实测待完成；所需输入：" + "、".join(gate["requiredInputs"]), "evidenceKinds": ["target-pending"]})
        manifest["cases"].append(case)
    snapshot = root / "evidence/target-profile.json"
    write_json(snapshot, sanitize_json(value))
    manifest["cases"][0].update({"status": "passed", "assertions": 1, "reason": "仅档案结构校验通过，不代表NAS功能/性能通过", "evidence": ["evidence/target-profile.json"], "evidenceKinds": ["target-template-validation-only"]})


def parse_args() -> argparse.Namespace:
    """声明每个参数的中文解释并限制有界值，避免隐式扩大执行范围。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--suite", choices=("local", "target"), default="local", help="local复用本机夹具；target只校验设备档案")
    parser.add_argument("--cases", default="core", help="本机组合，逗号分隔；支持" + "/".join(CASES))
    parser.add_argument("--artifacts", type=Path, default=ROOT / ".runtime/bin", help="已构建nas与worker目录，不自动下载或构建")
    parser.add_argument("--root", help=".runtime/verify下尚不存在的独立运行目录；默认生成唯一编号")
    parser.add_argument("--port", type=int, default=0, help="core回环端口；0自动选择，指定端口被占用即阻塞")
    parser.add_argument("--timeout", type=float, default=300, help="每个子验证的墙钟上限秒数，1至3600")
    parser.add_argument("--startup-timeout", type=float, default=30, help="core数据库和专属静态探针就绪上限秒数，1至120")
    parser.add_argument("--ffmpeg", type=Path, help="core显式使用的真实FFmpeg绝对路径")
    parser.add_argument("--ffprobe", type=Path, help="core显式使用的真实ffprobe绝对路径")
    parser.add_argument("--large-file", action="store_true", help="core增加5GiB稀疏HTTP偏移夹具，不计真实解码性能")
    parser.add_argument("--target-file", type=Path, help="真实NAS待验档案；v1只校验，不连接设备")
    parser.add_argument("--self-test", action="store_true", help="运行可复用编排集成自检，独立于产品验收")
    args = parser.parse_args()
    if not 1 <= args.timeout <= 3600 or not 1 <= args.startup_timeout <= 120 or (args.port != 0 and not 1024 <= args.port <= 65535):
        parser.error("超时或端口越过允许范围")
    selected = args.cases.split(",")
    if len(set(selected)) != len(selected) or not set(selected).issubset(CASES):
        parser.error("cases存在重复项或未知组合")
    if args.suite == "target" and (args.target_file is None or args.cases != "core" or args.large_file or args.port or args.ffmpeg or args.ffprobe):
        parser.error("target需要target-file，不能混用本机执行参数")
    if args.suite == "local" and args.target_file is not None:
        parser.error("本机组合不接受target-file")
    args.artifacts = args.artifacts.absolute()
    args.selected = selected
    return args


def main() -> int:
    """运行选中组合并持续保存四态清单，失败后继续独立用例。"""
    configure_console()
    configure_environment()
    args = parse_args()
    if args.self_test:
        from verify_baseline_selftest import run_self_test

        try:
            return run_self_test(args.root)
        except Blocked as error:
            print("基线自检前置拒绝：" + sanitize(str(error)), file=sys.stderr)
            return 2
        except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
            print("基线自检执行失败：" + sanitize(str(error)), file=sys.stderr)
            return 1
    try:
        root = new_run_root(args.root)
    except (OSError, ValueError, Blocked) as error:
        print("基线前置拒绝：" + sanitize(str(error)), file=sys.stderr)
        return 2
    (root / "logs").mkdir()
    (root / "evidence").mkdir()
    manifest: dict[str, Any] = {
        "formatVersion": 1,  # 编排清单版本，不等于目标模板或数据库版本。
        "runId": root.name,  # 独立目录编号，不能复用历史运行。
        "suite": args.suite,  # 本机轨道或仅档案校验轨道。
        "startedAt": now(), "finishedAt": None, "status": "not_run",  # UTC起止及四态结果。
        "source": source_identity(),  # Git版本与修改计数，不导出变更路径。
        "platform": {"system": platform.system(), "release": platform.release(), "machine": platform.machine(), "python": platform.python_version(), "filesystem": None, "filesystemNote": "文件系统/ACL需目标设备实测，本机未自动识别"},  # 不推测NAS平台。
        "tools": {"python": {"version": platform.python_version(), "path": display_path(Path(sys.executable))}},  # 实际解释器与显式媒体工具身份。
        "artifacts": {},  # 运行前核对的产物大小与SHA256。
        "configuration": {"cases": args.selected if args.suite == "local" else [], "timeoutSeconds": args.timeout, "startupTimeoutSeconds": args.startup_timeout, "requestedPort": args.port, "largeSparseFixture": args.large_file},  # 所有生效的有界参数。
        "cases": [case_record(name, name in args.selected) for name in CASES] if args.suite == "local" else [case_record("target-profile", True)],  # 未选用例明确not_run。
        "limitations": ["本机协议夹具不代表真实Docker/BT/外部账号/NAS或浏览器性能", "5GiB稀疏文件只证明HTTP偏移", "原脚本凭据/数据库/夹具保留在本地私密根，不应整体上传", "target v1仅档案校验，不执行生产动作或导入验收通过声明"],  # 所有结果共同适用的证据边界。
    }
    save_manifest(root, manifest)
    print("基线开始：" + display_path(root), flush=True)
    try:
        for case in manifest["cases"][:]:
            if not case["selected"]:
                continue
            case["startedAt"], case["reason"] = now(), "执行中，尚未得出结论"
            save_manifest(root, manifest)
            try:
                if args.suite == "target":
                    validate_target(args.target_file, root, manifest)
                else:
                    binary = prerequisites(args, case, manifest, root)
                    if case["id"] == "core":
                        core_case(args, root, binary, case)
                    else:
                        domain_case(args, root, binary, case)
                    case.update({"status": "passed", "reason": "子验证正常退出，断言计数和证据完整"})
            except Blocked as error:
                case.update({"status": "blocked", "reason": sanitize(str(error))})
            except (OSError, ValueError, RuntimeError, subprocess.SubprocessError, AssertionError) as error:
                case.update({"status": "failed", "reason": sanitize(str(error))})
            case["finishedAt"] = now()
            print(case["id"] + "：" + case["status"] + "；" + case["reason"], flush=True)
            save_manifest(root, manifest)
    except KeyboardInterrupt:
        for case in manifest["cases"]:
            if case["selected"] and case["status"] == "not_run" and case["startedAt"]:
                case.update({"status": "failed", "reason": "运行被中断，不能计作通过", "finishedAt": now()})
    finally:
        try:
            sanitize_fixture_logs(root)
        except OSError as error:
            cleanup = case_record("evidence-sanitization", True)
            cleanup.update({"status": "failed", "reason": sanitize("日志脱敏失败：" + str(error))})
            manifest["cases"].append(cleanup)
        code = save_manifest(root, manifest, True)
    print("基线结束：" + manifest["status"] + "；清单：" + display_path(root / "run-manifest.json"), flush=True)
    return code


if __name__ == "__main__":
    raise SystemExit(main())
