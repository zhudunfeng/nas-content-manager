"""隔离实际维护进程、SQLite 和 Windows Console，验证停机改密且不操作用户实例。"""

from __future__ import annotations

import sys

if __name__ == '__main__':
    sys.dont_write_bytecode = True

import argparse
from contextlib import closing
import ctypes
import errno
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import sqlite3
import string
import subprocess
import threading
import time
import traceback
from typing import Any, BinaryIO

from verify import generate_password
from verify_account import AccountHarness
from verify_baseline import Blocked, ROOT, WindowsJob, clean_environment, file_identity, new_run_root, sanitize, write_json
from verify_schema import create_legacy_database


OUTPUT_LIMIT: int = 262144  # 每条原始输出流仅保留256KiB，超量持续排空但必须失败。
PROCESS_TIMEOUT: float = 12.0  # 单次离线命令的墙钟预算，不影响真实实例。
FIRST_PROMPT: bytes = '新密码（输入不回显）：'.encode()  # 产品固定首提示，不用输入内容判断进度。
SECOND_PROMPT: bytes = '再次输入新密码（输入不回显）：'.encode()  # 产品固定确认提示。


class RawProcess:
    """在执行前取得自有进程树，原始双流有界留内存，检查后才允许脱敏持久化。"""

    def __init__(self, command: list[str], environment: dict[str, str], *, console: bool = False, input_file: BinaryIO | None = None, inherit_console: bool = False) -> None:
        """挂起创建并加入独占Job；专属隐藏Console只用于本脚本helper。"""
        self.job: WindowsJob | None = WindowsJob() if os.name == 'nt' else None  # 不触及其他进程树。
        self.raw: list[bytearray] = [bytearray(), bytearray()]  # stdout/stderr原文只在内存保留。
        self.overflow: bool = False  # 不能把截断日志声明为无秘密。
        self.reader_failed: bool = False  # 排空失败使本次验证失败。
        self.closed: bool = False  # 幂等清理标记。
        self.readers: list[threading.Thread] = []  # 持有并等待自己的两条排空线程。
        options: dict[str, Any] = {}  # 仅当前子进程的控制台和环境参数。
        if os.name == 'nt':
            options['creationflags'] = 0x00000004 | (0x00000010 if console else 0 if inherit_console else 0x08000000)
            startup = subprocess.STARTUPINFO()
            startup.dwFlags |= subprocess.STARTF_USESHOWWINDOW
            startup.wShowWindow = 0  # SW_HIDE，避免干扰用户桌面。
            options['startupinfo'] = startup
        else:
            options['start_new_session'] = True
        try:
            self.process: subprocess.Popen[bytes] = subprocess.Popen(command, cwd=ROOT, env=environment, stdin=input_file if input_file is not None else subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **options)
            if self.job:
                self.job.attach_and_resume(self.process)
            for index, stream in enumerate((self.process.stdout, self.process.stderr)):
                if stream is None:
                    raise RuntimeError('原始输出管道不可用')
                reader = threading.Thread(target=self.drain, args=(index, stream), daemon=True)
                self.readers.append(reader)
                reader.start()
        except BaseException:
            self.close()
            raise

    def drain(self, index: int, stream: BinaryIO) -> None:
        """原始流限长收集；超量后继续排空，避免子进程因写满管道假挂起。"""
        try:
            while block := stream.read1(4096):
                remaining = OUTPUT_LIMIT - len(self.raw[index])
                self.raw[index].extend(block[:max(0, remaining)])
                if len(block) > remaining:
                    self.overflow = True
        except (OSError, ValueError):
            self.reader_failed = True

    def send(self, data: bytes, *, eof: bool = True) -> None:
        """只向本进程私有stdin发送秘密，默认随后关闭管道证明完整单个输入。"""
        if self.process.stdin is None:
            raise RuntimeError('私有输入管道不可用')
        try:
            self.process.stdin.write(data)
            self.process.stdin.flush()
        except OSError as error:
            # Windows对已关闭匿名管道还可能返回EINVAL，和CPython communicate采用同一边界。
            if error.errno != errno.EPIPE and not (os.name == 'nt' and error.errno == errno.EINVAL):
                raise
        if eof:
            try:
                self.process.stdin.close()
            except OSError as error:
                if error.errno != errno.EPIPE and not (os.name == 'nt' and error.errno == errno.EINVAL):
                    raise

    def wait(self, timeout: float = PROCESS_TIMEOUT) -> int:
        """有界等待自然退出并确认双流排空，超时交给finally严格回收。"""
        code = self.process.wait(timeout=timeout)
        for reader in self.readers:
            reader.join(timeout=3)
        if any(reader.is_alive() for reader in self.readers) or self.reader_failed or self.overflow:
            raise AssertionError('原始输出未完整有界收集')
        return code

    def close(self) -> None:
        """等待所有自有后代和流句柄，任何清理错误均不能变成通过。"""
        if self.closed:
            return
        self.closed = True
        failures: list[str] = []
        try:
            if self.job:
                self.job.stop()
            elif hasattr(self, 'process'):
                try:
                    os.killpg(self.process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
        except BaseException:
            failures.append('自有进程树回收失败')
        if hasattr(self, 'process'):
            if self.process.poll() is None:
                self.process.kill()
            try:
                self.process.wait(timeout=8)
            except subprocess.TimeoutExpired:
                failures.append('自有进程退出等待失败')
            for reader in self.readers:
                reader.join(timeout=3)
                if reader.is_alive():
                    failures.append('自有原始输出线程未退出')
            for stream in (self.process.stdin, self.process.stdout, self.process.stderr):
                if stream is not None and not stream.closed:
                    stream.close()
        if failures:
            raise RuntimeError('；'.join(failures))


def raw_has_no_secrets(streams: list[bytes], secrets: list[bytes]) -> bool:
    """直接检查未脱敏内存流，不把已遮蔽文件当作秘密未输出的证据。"""
    return all(secret not in block for block in streams for secret in secrets if len(secret) >= 4) and all(re.search(rb'\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}', block) is None for block in streams)


def maintenance_environment() -> dict[str, str]:
    """故意设置无效服务配置，证明维护分派不加载在线媒体/网络配置。"""
    environment = clean_environment()
    environment.update({'NAS_ADDR': 'invalid-address', 'NAS_MEDIA_ROOTS': 'missing-media', 'NAS_WEB_DIR': 'missing-web', 'NAS_FFMPEG': 'missing-ffmpeg', 'NAS_MIN_FREE_MB': 'not-a-number'})
    return environment


def run_cli(binary: Path, arguments: list[str], data: bytes, secrets: list[bytes], log: Path) -> tuple[int, bytes, bytes]:
    """执行固定维护产物，先扫描原始输出，再保存安全日志和准确退出码。"""
    process = RawProcess([str(binary), *arguments], maintenance_environment())
    try:
        process.send(data)
        code = process.wait()
        stdout, stderr = (bytes(value) for value in process.raw)
    finally:
        process.close()
    if not raw_has_no_secrets([stdout, stderr], secrets):
        raise AssertionError('维护原始输出包含测试凭据；禁止持久化原文')
    log.parent.mkdir(parents=True, exist_ok=True)
    log.write_text(sanitize((stdout + stderr).decode('utf-8', 'replace')) + '\n', encoding='utf-8')
    return code, stdout, stderr


def snapshot(path: Path) -> tuple[int, list[tuple[Any, ...]], dict[str, list[tuple[Any, ...]]]]:
    """按逻辑行而非WAL物理布局比较所有表，账号敏感值不持久化。"""
    with closing(sqlite3.connect(path.as_uri() + '?mode=ro', uri=True)) as database:
        schema = database.execute('SELECT type,name,tbl_name,sql FROM sqlite_schema ORDER BY type,name').fetchall()
        tables: dict[str, list[tuple[Any, ...]]] = {}
        for name, in database.execute("SELECT name FROM sqlite_schema WHERE type='table' ORDER BY name"):
            quoted = '"' + name.replace('"', '""') + '"'
            tables[name] = sorted(database.execute('SELECT * FROM ' + quoted).fetchall(), key=repr)
        return database.execute('PRAGMA user_version').fetchone()[0], schema, tables


def fixture_files(root: Path) -> dict[str, str]:
    """仅对本次普通哨兵文件摘要，不遍历数据库、锁或用户媒体目录。"""
    result: dict[str, str] = {}
    for directory in (root / 'data/cache', root / 'media', root / 'data/.backup-pending-sentinel'):
        if directory.exists():
            for path in directory.rglob('*'):
                if path.is_file() and not path.is_symlink():
                    result[path.relative_to(root).as_posix()] = hashlib.sha256(path.read_bytes()).hexdigest()
    return result


def copy_database(source: Path, target: Path) -> None:
    """SQLite一致备份到全新隔离文件，不复制仍打开的WAL旁路文件。"""
    target.parent.mkdir(parents=True)
    with target.open('xb'):
        pass
    with closing(sqlite3.connect(source.as_uri() + '?mode=ro', uri=True)) as original, closing(sqlite3.connect(target)) as copied:
        original.backup(copied)


def directory_alias(source: Path, target: Path) -> str:
    """在Windows优先用无需符号链接特权的Junction，其他平台使用目录符号链接。"""
    if os.name == 'nt':
        import _winapi
        _winapi.CreateJunction(str(source), str(target))
        return 'junction'
    target.symlink_to(source, target_is_directory=True)
    return 'symlink'


def lock_available(root: Path) -> bool:
    """复核维护退出后同一OS锁可重新取得，不删除永久锁文件。"""
    path = root / '.nas-instance.lock'
    if os.name == 'nt':
        from ctypes import wintypes
        kernel = ctypes.WinDLL('kernel32', use_last_error=True)
        kernel.CreateFileW.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD, ctypes.c_void_p, wintypes.DWORD, wintypes.DWORD, wintypes.HANDLE]
        kernel.CreateFileW.restype = wintypes.HANDLE
        kernel.CloseHandle.argtypes = [wintypes.HANDLE]
        kernel.CloseHandle.restype = wintypes.BOOL
        handle = kernel.CreateFileW(str(path), 0xC0000000, 0, None, 4, 0x00200080, None)
        if handle == ctypes.c_void_p(-1).value:
            return False
        if not kernel.CloseHandle(handle):
            raise RuntimeError('复核锁句柄关闭失败')
        return True
    import fcntl
    with path.open('a+b') as stream:
        try:
            fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return False
        fcntl.flock(stream, fcntl.LOCK_UN)
    return True


def seed_unrelated(harness: AccountHarness) -> None:
    """停服后种入中断作业、外部动作和缓存哨兵，以检测错误调用正常初始化。"""
    (harness.root / 'data/cache/.pending-offline-probe').write_bytes(b'pending-must-stay')
    (harness.root / 'media/offline-preservation.bin').write_bytes(b'media-must-stay')
    pending = harness.root / 'data/.backup-pending-sentinel'
    pending.mkdir()
    (pending / 'unknown.bin').write_bytes(b'backup-pending-must-stay')
    with closing(harness.database()) as database:
        database.execute("INSERT INTO libraries(id,name,type,path) VALUES(991,'离线哨兵','video',?)", (str(harness.root / 'media'),))
        database.execute("INSERT INTO jobs(library_id,kind,status,message,created_at,updated_at) VALUES(991,'offline-probe','running','维护前原值','2000','2000')")
        database.execute('INSERT INTO collection_sources(id,body) VALUES(?,?)', ('offline-probe', json.dumps({'id': 'offline-probe', 'kind': 'local', 'name': '离线哨兵', 'enabled': False, 'revision': 1})))
        database.execute("INSERT INTO collection_external_actions(id,request_id,source_id,source_revision,kind,target_id,state,message,created_at,updated_at,intent_hash,private_ref) VALUES('offline-action','offline-request','offline-probe',1,'add','none','dispatching','维护前原值','2000','2000','fixture','none')")
        seed_backup_domain(database)
        database.commit()


def seed_backup_domain(database: sqlite3.Connection) -> None:
    """为真正schema6种入非空五表及未发布所有权，调用者负责提交和隔离目录。"""
    file_id, run_id = 'a' * 32, 'b' * 32
    request_id = '019af0aa-0000-7000-8000-000000000001'
    store_id = database.execute('SELECT store_id FROM backup_store WHERE id=1').fetchone()[0]
    database.execute("UPDATE backup_store SET last_good_file_id=?,last_success_json='{}',retention_authorizer_file_id=?,retention_authorized_keep_count=7,retention_authorized_max_bytes=10737418240 WHERE id=1", (file_id, file_id))
    database.execute("INSERT INTO backup_runs(id,request_id,trigger,policy_revision,keep_count,max_bytes,state,created_at_ms,updated_at_ms,started_at_ms,finished_at_ms,file_id,message,admission_deadline_ms) VALUES(?,?,'manual',1,7,10737418240,'completed',1,1,1,1,?,'隔离完成事实',1)", (run_id, request_id, file_id))
    seq = database.execute('SELECT seq FROM backup_runs WHERE id=?', (run_id,)).fetchone()[0]
    database.execute("INSERT INTO backup_files(id,store_id,run_id,run_seq,state,created_at_ms,published_at_ms,verified_at_ms,size_bytes,sha256,schema_version,app_version,manifest_size,manifest_sha256,owner_size,owner_sha256) VALUES(?,?,?,?,'retained',1,1,1,4096,?,6,'fixture',2,?,2,?)", (file_id, store_id, run_id, seq, '1' * 64, '2' * 64, '3' * 64))
    database.execute("INSERT INTO backup_request_keys(request_id,request_time_ms,accepted_at_ms,expires_at_ms,run_id,run_seq,terminal,summary_json) VALUES(?,0,0,86400000,?,?,1,'{}')", (request_id, run_id, seq))
    database.execute("INSERT INTO backup_runs(id,request_id,trigger,policy_revision,keep_count,max_bytes,state,created_at_ms,updated_at_ms,finished_at_ms,file_id,error_code,message,admission_deadline_ms) VALUES(?,?,'manual',1,7,10737418240,'failed',1,1,1,?,'fixture_failed','未发布残留',1)", ('c' * 32, '019af0aa-0000-7000-8000-000000000002', 'd' * 32))
    pending_seq = database.execute('SELECT seq FROM backup_runs WHERE id=?', ('c' * 32,)).fetchone()[0]
    database.execute("INSERT INTO backup_files(id,store_id,run_id,run_seq,state,created_at_ms,owner_size,owner_sha256) VALUES(?,?,?,?,'pending',1,2,?)", ('d' * 32, store_id, 'c' * 32, pending_seq, '4' * 64))


def verify_windows_permission(binary: Path, source: Path, root: Path, secrets: list[bytes], password: str) -> dict[str, Any]:
    """仅改隔离副本DACL并复原，先证明真实写入被拒绝，再核验维护失败且无写。"""
    from ctypes import wintypes

    target = root / 'acl-fixture/library.db'
    copy_database(source, target)
    original = snapshot(target)
    kernel = ctypes.WinDLL('kernel32', use_last_error=True)
    security = ctypes.WinDLL('advapi32', use_last_error=True)
    kernel.GetCurrentProcess.restype = wintypes.HANDLE
    kernel.CloseHandle.argtypes, kernel.CloseHandle.restype = [wintypes.HANDLE], wintypes.BOOL
    kernel.LocalFree.argtypes, kernel.LocalFree.restype = [ctypes.c_void_p], ctypes.c_void_p
    security.OpenProcessToken.argtypes, security.OpenProcessToken.restype = [wintypes.HANDLE, wintypes.DWORD, ctypes.POINTER(wintypes.HANDLE)], wintypes.BOOL
    security.GetTokenInformation.argtypes, security.GetTokenInformation.restype = [wintypes.HANDLE, ctypes.c_int, ctypes.c_void_p, wintypes.DWORD, ctypes.POINTER(wintypes.DWORD)], wintypes.BOOL
    security.ConvertSidToStringSidW.argtypes, security.ConvertSidToStringSidW.restype = [ctypes.c_void_p, ctypes.POINTER(wintypes.LPWSTR)], wintypes.BOOL
    security.GetFileSecurityW.argtypes, security.GetFileSecurityW.restype = [wintypes.LPCWSTR, wintypes.DWORD, ctypes.c_void_p, wintypes.DWORD, ctypes.POINTER(wintypes.DWORD)], wintypes.BOOL
    security.GetSecurityDescriptorControl.argtypes, security.GetSecurityDescriptorControl.restype = [ctypes.c_void_p, ctypes.POINTER(wintypes.WORD), ctypes.POINTER(wintypes.DWORD)], wintypes.BOOL
    security.ConvertStringSecurityDescriptorToSecurityDescriptorW.argtypes, security.ConvertStringSecurityDescriptorToSecurityDescriptorW.restype = [wintypes.LPCWSTR, wintypes.DWORD, ctypes.POINTER(ctypes.c_void_p), ctypes.c_void_p], wintypes.BOOL
    security.SetFileSecurityW.argtypes, security.SetFileSecurityW.restype = [wintypes.LPCWSTR, wintypes.DWORD, ctypes.c_void_p], wintypes.BOOL
    token, size = wintypes.HANDLE(), wintypes.DWORD()
    sid_text, denied_descriptor = wintypes.LPWSTR(), ctypes.c_void_p()
    changed = False
    saved: Any = None  # 原始安全描述符只在内存，不持久化SID或ACL原文。
    restore_flags = 4
    try:
        if not security.OpenProcessToken(kernel.GetCurrentProcess(), 8, ctypes.byref(token)):
            return {'attempted': True, 'established': False, 'reason': 'token-query-unavailable'}
        security.GetTokenInformation(token, 1, None, 0, ctypes.byref(size))
        information = ctypes.create_string_buffer(size.value)
        if not security.GetTokenInformation(token, 1, information, size, ctypes.byref(size)) or not security.ConvertSidToStringSidW(ctypes.c_void_p.from_buffer(information), ctypes.byref(sid_text)):
            return {'attempted': True, 'established': False, 'reason': 'sid-query-unavailable'}
        security.GetFileSecurityW(str(target), 4, None, 0, ctypes.byref(size))
        saved = ctypes.create_string_buffer(size.value)
        if not security.GetFileSecurityW(str(target), 4, saved, size, ctypes.byref(size)):
            return {'attempted': True, 'established': False, 'reason': 'dacl-query-unavailable'}
        control, revision = wintypes.WORD(), wintypes.DWORD()
        if not security.GetSecurityDescriptorControl(saved, ctypes.byref(control), ctypes.byref(revision)):
            return {'attempted': True, 'established': False, 'reason': 'dacl-control-unavailable'}
        restore_flags = 4 | (0x80000000 if control.value & 0x1000 else 0x20000000)  # 精确保留原DACL继承保护状态。
        sid = sid_text.value
        if not security.ConvertStringSecurityDescriptorToSecurityDescriptorW('D:P(D;;0x116;;;' + sid + ')(A;;FA;;;' + sid + ')', 1, ctypes.byref(denied_descriptor), None):
            return {'attempted': True, 'established': False, 'reason': 'fixture-descriptor-unavailable'}
        if not security.SetFileSecurityW(str(target), 4 | 0x80000000, denied_descriptor):
            return {'attempted': True, 'established': False, 'reason': 'dacl-update-unavailable'}
        changed = True
        denied = False
        try:
            with target.open('r+b'):
                pass
        except PermissionError:
            denied = True
        if not denied:
            return {'attempted': True, 'established': False, 'reason': 'current-token-bypassed-fixture'}
        code, _, _ = run_cli(binary, ['account', 'reset-password', '--data-dir', str(target.parent), '--password-stdin'], password.encode(), secrets, root / 'logs/acl.log')
        if code != 1:
            raise AssertionError('真实ACL拒绝时维护没有失败')
    finally:
        if changed and not security.SetFileSecurityW(str(target), restore_flags, saved):
            raise RuntimeError('隔离ACL夹具未恢复，验证失败')
        if changed:
            restored_size = wintypes.DWORD()
            restored = ctypes.create_string_buffer(len(saved))
            if not security.GetFileSecurityW(str(target), 4, restored, len(restored), ctypes.byref(restored_size)) or restored_size.value != len(saved) or restored.raw != saved.raw:
                raise RuntimeError('隔离ACL恢复后安全描述符不一致，验证失败')
        if token and not kernel.CloseHandle(token):
            raise RuntimeError('权限夹具令牌句柄关闭失败')
        if sid_text:
            kernel.LocalFree(sid_text)
        if denied_descriptor:
            kernel.LocalFree(denied_descriptor)
    if snapshot(target) != original or not lock_available(target.parent):
        raise AssertionError('ACL拒绝后账号变化或锁未释放')
    return {'attempted': True, 'established': True, 'realWriteDenied': True, 'maintenanceExitCode': 1, 'unchanged': True, 'aclRestored': True}


class WindowsConsole:
    """仅访问helper专属Console，不AttachConsole到用户控制台。"""

    def __init__(self) -> None:
        """声明控制台ABI并安装不可继承的具体保护处理器。"""
        from ctypes import wintypes
        import msvcrt

        class CharUnion(ctypes.Union):
            """Windows键盘字符联合字段，保留原生宽字符布局。"""
            _fields_ = [('UnicodeChar', wintypes.WCHAR), ('AsciiChar', ctypes.c_char)]  # Unicode/单字节字符。

        class Key(ctypes.Structure):
            """KEY_EVENT_RECORD原生键盘事件。"""
            _fields_ = [('down', wintypes.BOOL), ('repeat', wintypes.WORD), ('virtual', wintypes.WORD), ('scan', wintypes.WORD), ('char', CharUnion), ('control', wintypes.DWORD)]  # 按下、次数、虚键、扫描码、字符及修饰键。

        class EventUnion(ctypes.Union):
            """事件联合最大16字节，避免写错INPUT_RECORD步长。"""
            _fields_ = [('key', Key), ('padding', ctypes.c_byte * 16)]  # 本脚本仅发送键盘事件。

        class Record(ctypes.Structure):
            """INPUT_RECORD事件类型及对齐联合。"""
            _fields_ = [('kind', wintypes.WORD), ('event', EventUnion)]  # KEY_EVENT及其负载。

        class Coord(ctypes.Structure):
            """Console二维短整数坐标。"""
            _fields_ = [('x', ctypes.c_short), ('y', ctypes.c_short)]  # 列与行。

        class Rect(ctypes.Structure):
            """Console窗口矩形。"""
            _fields_ = [('left', ctypes.c_short), ('top', ctypes.c_short), ('right', ctypes.c_short), ('bottom', ctypes.c_short)]  # 四条边。

        class ScreenInfo(ctypes.Structure):
            """屏幕缓冲区结构，只用于有界原始采样。"""
            _fields_ = [('size', Coord), ('cursor', Coord), ('attributes', wintypes.WORD), ('window', Rect), ('maximum', Coord)]  # 缓冲区、光标、属性、窗口及最大尺寸。

        self.record: Any = Record  # 真实键盘事件结构。
        self.coord: Any = Coord  # 原始屏幕采样起点。
        self.screen_type: Any = ScreenInfo  # 原始屏幕尺寸。
        self.kernel: Any = ctypes.WinDLL('kernel32', use_last_error=True)  # 只调用自有控制台接口。
        signatures = {
            'GetConsoleMode': ([wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD)], wintypes.BOOL),
            'WriteConsoleInputW': ([wintypes.HANDLE, ctypes.POINTER(Record), wintypes.DWORD, ctypes.POINTER(wintypes.DWORD)], wintypes.BOOL),
            'GenerateConsoleCtrlEvent': ([wintypes.DWORD, wintypes.DWORD], wintypes.BOOL),
            'GetConsoleScreenBufferInfo': ([wintypes.HANDLE, ctypes.POINTER(ScreenInfo)], wintypes.BOOL),
            'ReadConsoleOutputCharacterW': ([wintypes.HANDLE, wintypes.LPWSTR, wintypes.DWORD, Coord, ctypes.POINTER(wintypes.DWORD)], wintypes.BOOL),
            'CreateFileW': ([wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD, ctypes.c_void_p, wintypes.DWORD, wintypes.DWORD, wintypes.HANDLE], wintypes.HANDLE),
            'CloseHandle': ([wintypes.HANDLE], wintypes.BOOL),
        }  # 每个参数有明确ABI类型，不截断64位句柄。
        for name, (arguments, result) in signatures.items():
            getattr(self.kernel, name).argtypes, getattr(self.kernel, name).restype = arguments, result
        self.user: Any = ctypes.WinDLL('user32', use_last_error=True)  # 虚键映射由User32提供，不属于Kernel32。
        self.user.VkKeyScanW.argtypes, self.user.VkKeyScanW.restype = [wintypes.WCHAR], ctypes.c_short
        handler_type = ctypes.WINFUNCTYPE(wintypes.BOOL, wintypes.DWORD)
        self.handler: Any = handler_type(lambda event: True)  # 保护本helper；不使用可继承的NULL忽略属性。
        self.kernel.SetConsoleCtrlHandler.argtypes = [handler_type, wintypes.BOOL]
        self.kernel.SetConsoleCtrlHandler.restype = wintypes.BOOL
        if not self.kernel.SetConsoleCtrlHandler(self.handler, True):
            raise AssertionError('专属控制台保护处理器安装失败')
        self.input: BinaryIO = open('CONIN$', 'r+b', buffering=0)  # 独立真实Console输入，不是payload管道。
        self.input_handle: int = msvcrt.get_osfhandle(self.input.fileno())
        # CPython的ConsoleIO不支持CONOUT$的r+b组合；直接取得读屏原生句柄。
        self.output_handle: int = self.kernel.CreateFileW('CONOUT$', 0xC0000000, 3, None, 3, 0, None)
        if self.output_handle == ctypes.c_void_p(-1).value:
            self.input.close()
            raise AssertionError('专属Console屏幕句柄不可打开')

    def mode(self) -> int:
        """读取准确输入模式，恢复时必须与原值完全相同。"""
        value = ctypes.c_ulong()
        if not self.kernel.GetConsoleMode(self.input_handle, ctypes.byref(value)):
            raise AssertionError('Console输入模式不可读取')
        return int(value.value)

    def keys(self, value: str) -> None:
        """用真实KEY_EVENT写入按下/释放，不用管道字符串冒充终端键盘。"""
        records = (self.record * (len(value) * 2))()
        for index, character in enumerate(value):
            for offset, down in enumerate((True, False)):
                item = records[index * 2 + offset]
                item.kind = 1  # KEY_EVENT。
                item.event.key.down, item.event.key.repeat = down, 1
                mapping = int(self.user.VkKeyScanW(character))
                item.event.key.virtual = mapping & 0xFF if mapping != -1 else 0
                modifiers = (mapping >> 8) & 0xFF if mapping != -1 else 0
                item.event.key.control = (0x10 if modifiers & 1 else 0) | (0x8 if modifiers & 2 else 0) | (0x2 if modifiers & 4 else 0)
                item.event.key.char.UnicodeChar = character
        written = ctypes.c_ulong()
        if not self.kernel.WriteConsoleInputW(self.input_handle, records, len(records), ctypes.byref(written)) or written.value != len(records):
            raise AssertionError('真实Console键盘写入未完成')

    def screen(self) -> bytes:
        """有界读取原始屏幕文本，扫描后只保存秘密未回显的布尔证据。"""
        info = self.screen_type()
        if not self.kernel.GetConsoleScreenBufferInfo(self.output_handle, ctypes.byref(info)):
            raise AssertionError('专属Console屏幕不可读取')
        count = int(info.size.x) * (int(info.window.bottom) - int(info.window.top) + 1)
        if count < 1 or count > 131072:
            raise AssertionError('Console屏幕超过采样预算')
        buffer = ctypes.create_unicode_buffer(count + 1)
        read = ctypes.c_ulong()
        if not self.kernel.ReadConsoleOutputCharacterW(self.output_handle, buffer, count, self.coord(0, info.window.top), ctypes.byref(read)) or read.value != count:
            raise AssertionError('Console屏幕原始采样不完整')
        return buffer[:read.value].encode('utf-8')

    def ctrl_event(self) -> None:
        """只向专属Console广播真正CTRL_C_EVENT，不使用CTRL_BREAK替代。"""
        if not self.kernel.GenerateConsoleCtrlEvent(0, 0):
            raise AssertionError('专属Console CTRL_C_EVENT发送失败')

    def close(self) -> None:
        """关闭本helper显式打开的两条Console句柄。"""
        self.input.close()
        if not self.kernel.CloseHandle(self.output_handle):
            raise RuntimeError('专属Console屏幕句柄关闭失败')


def wait_prompt(process: RawProcess, prompt: bytes) -> None:
    """仅按固定产品提示等待，不读取或打印敏感输入值。"""
    deadline = time.monotonic() + PROCESS_TIMEOUT
    while prompt not in process.raw[1]:
        if process.process.poll() is not None or time.monotonic() >= deadline:
            raise AssertionError('维护未到达预期Console提示；退出码=' + str(process.process.poll()))
        time.sleep(0.01)


def console_helper(payload: dict[str, Any]) -> dict[str, Any]:
    """在专属隐藏Console执行单个真实场景，秘密只由父stdin传入且不输出。"""
    console = WindowsConsole()
    initial = console.mode()
    scenario, stage = payload['scenario'], int(payload['stage'])
    password, partial = payload['password'], payload['partial']
    process: RawProcess | None = None
    screens: list[bytes] = []
    during: int | None = None
    schema_stable: bool | None = None
    try:
        command = [payload['binary'], 'account', 'reset-password', '--data-dir', payload['dataDir']]
        pipe_mode = scenario == 'pipe-event'
        if pipe_mode or scenario == 'stdin-terminal':
            command.append('--password-stdin')
        process = RawProcess(command, maintenance_environment(), input_file=None if pipe_mode else console.input, inherit_console=True)
        if pipe_mode:
            process.send(partial.encode(), eof=False)
            time.sleep(0.2)
            during = console.mode()
            console.ctrl_event()
        elif scenario != 'stdin-terminal':
            wait_prompt(process, FIRST_PROMPT)
            if stage == 2:
                console.keys(password + '\r')
                wait_prompt(process, SECOND_PROMPT)
            if scenario == 'version-drift':
                # 已到第二提示才改user_version，真实CLI预检已完成且不改SQLite结构修订。
                drift_to = payload.get('driftTo')
                if drift_to not in (5, 6) or stage != 2:
                    raise AssertionError('版本漂移场景参数无效')
                with closing(sqlite3.connect(Path(payload['dataDir']) / 'library.db')) as database:
                    schema_before = database.execute('PRAGMA schema_version').fetchone()
                    database.execute('PRAGMA user_version=' + str(drift_to))
                    database.commit()
                    schema_stable = schema_before == database.execute('PRAGMA schema_version').fetchone()
                if not schema_stable:
                    raise AssertionError('版本漂移夹具意外修改结构修订')
            during = console.mode()
            if during & 0x7:
                raise AssertionError('Console输入没有关闭行缓冲、回显或系统按键处理')
            console.keys(partial)
            time.sleep(0.06)
            screens.append(console.screen())
            if scenario == 'key-cancel':
                console.keys('\x03')
            elif scenario == 'event-cancel':
                console.ctrl_event()
            elif scenario == 'eof-cancel':
                console.keys('\x04')
            elif scenario in ('mismatch', 'success-separated', 'version-drift'):
                console.keys('\r')
            elif scenario == 'success':
                # 两行CRLF粘贴经过真实Console输入，首行有效时继续确认。
                console.keys('\r\n' + password + '\r\n')
            else:
                raise AssertionError('未知内部Console场景')
        code = process.wait()
        restored = console.mode()
        screens.append(console.screen())
        raw = [bytes(value) for value in process.raw]
        known = [bytes.fromhex(value) for value in payload['secretsHex']]
        if not raw_has_no_secrets([*raw, *screens], known):
            raise AssertionError('原始Console或进程双流出现测试秘密')
        if restored != initial:
            raise AssertionError('Console精确输入模式未恢复')
        expected = 0 if scenario in ('success', 'success-separated') else 1 if scenario in ('mismatch', 'stdin-terminal', 'version-drift') else 130
        if code != expected:
            raise AssertionError('Console场景退出码不符合契约')
        return {'scenario': scenario, 'stage': stage, 'exitCode': code, 'initialMode': initial, 'duringMode': during, 'restoredMode': restored, 'rawSecretScan': True, 'screenSecretScan': True, 'screenScope': 'visible-viewport', 'boundedExit': True, 'sqliteSchemaStable': schema_stable}
    finally:
        if process is not None:
            process.close()
        console.close()


def run_console(binary: Path, root: Path, scenario: str, stage: int, password: str, secrets: list[bytes], partial: str, drift_to: int | None = None) -> dict[str, Any]:
    """父进程创建专属隐藏Console helper，继承项目环境并在执行前归入Job。"""
    payload = {'binary': str(binary), 'dataDir': str(root), 'scenario': scenario, 'stage': stage, 'password': password, 'partial': partial, 'driftTo': drift_to, 'secretsHex': [value.hex() for value in [*secrets, partial.encode()]]}
    process = RawProcess([sys.executable, '-B', str(Path(__file__).resolve()), '--console-helper'], clean_environment(), console=True)
    try:
        process.send(json.dumps(payload).encode())
        code = process.wait(PROCESS_TIMEOUT + 8)
        stdout, stderr = (bytes(value) for value in process.raw)
    finally:
        process.close()
    if not raw_has_no_secrets([stdout, stderr], [*secrets, partial.encode()]):
        raise AssertionError('专属Console helper泄露秘密')
    if code != 0:
        raise AssertionError('专属Console helper失败：' + sanitize(stderr.decode('utf-8', 'replace'))[:500])
    value = json.loads(stdout)
    if not isinstance(value, dict) or value.get('scenario') != scenario or value.get('rawSecretScan') is not True:
        raise AssertionError('Console证据结构无效')
    return value


def verify_offline(binary: Path, root: Path, assertions: list[str]) -> dict[str, Any]:
    """执行实际CLI、失败路径、输入矩阵与Windows控制台，返回可分享分类证据。"""
    harness = AccountHarness(binary, root / 'fixture %#中文', assertions)
    credentials = {'username': 'offline-account', 'password': generate_password()}
    password = generate_password()
    secrets: list[bytes] = [credentials['password'].encode(), password.encode()]
    evidence: dict[str, Any] = {'realCLI': False, 'rawOutputSecretScan': False, 'windowsConsole': [], 'actualPermissionDenial': False, 'linuxActual': False, 'targetNAS': False}
    try:
        harness.start()
        harness.client.json('/api/setup', 'POST', credentials)
        token = next(cookie.value for cookie in harness.client.cookies if cookie.name == 'nas_session')
        secrets.extend((token.encode(), hashlib.sha256(token.encode()).hexdigest().encode()))
        with closing(harness.database()) as database:
            secrets.append(database.execute('SELECT password_hash FROM users').fetchone()[0].encode())
        command = ['account', 'reset-password', '--data-dir', str(harness.root / 'data'), '--password-stdin']
        before = snapshot(harness.root / 'data/library.db')
        code, _, _ = run_cli(binary, command, password.encode(), secrets, root / 'logs/active.log')
        harness.check(code == 1 and snapshot(harness.root / 'data/library.db') == before and harness.client.request('/healthz')[0] == 200 and harness.owned is not None and harness.owned.process.poll() is None, '离线锁：活实例拒绝且账号无写，原进程仍存活健康')
        alias = root / 'data-alias'
        evidence['directoryAliasType'] = directory_alias(harness.root / 'data', alias)
        alias_command = ['account', 'reset-password', '--data-dir', str(alias), '--password-stdin']
        code, _, _ = run_cli(binary, alias_command, password.encode(), secrets, root / 'logs/alias-active.log')
        harness.check(code == 1 and snapshot(harness.root / 'data/library.db') == before and harness.client.request('/healthz')[0] == 200, '离线锁：目录重解析别名竞争同一租约，不杀占用者')
        harness.stop()
        seed_unrelated(harness)
        source = harness.root / 'data/library.db'
        files = fixture_files(harness.root)
        before = snapshot(source)

        # 固定参数错误必须早于配置、路径、媒体和数据库初始化。
        bad_arguments = [[], ['other'], ['account'], ['account', 'reset-password'], ['account', 'reset-password', '--password', password], ['account', 'reset-password', '--data-dir'], ['account', 'reset-password', '--data-dir', '--password-stdin'], [*command, '--password-stdin'], [*command, '--data-dir', str(source.parent)], [*command, '--unknown=' + password], ['account', 'reset-password', '--data-dir=' + str(source.parent)]]
        for index, arguments in enumerate(bad_arguments[1:], 1):
            code, _, _ = run_cli(binary, arguments, b'', secrets, root / ('logs/usage-' + str(index) + '.log'))
            harness.check(code == 2 and snapshot(source) == before and fixture_files(harness.root) == files, '离线参数：严格拒绝组合' + str(index) + '且原始输出不回显秘密')
        code, stdout, _ = run_cli(binary, ['account', 'reset-password', '--help'], b'', secrets, root / 'logs/help.log')
        harness.check(code == 0 and b'--password-stdin' in stdout and snapshot(source) == before, '离线帮助：只输出固定用法且不要求服务配置')

        # 每个畸形数据库均由一致副本单独改造，失败不能污染原账号或旁支夹具。
        invalid_sql: list[tuple[str, str]] = [
            ('future', 'PRAGMA user_version=7'),
            ('missing-column', 'ALTER TABLE users DROP COLUMN auth_revision'),
            ('no-admin', 'DELETE FROM sessions; DELETE FROM users'),
            ('other-id', 'PRAGMA ignore_check_constraints=ON; DELETE FROM sessions; UPDATE users SET id=2'),
            ('multiple-admins', "PRAGMA ignore_check_constraints=ON; INSERT INTO users SELECT 2,'other-fixture',password_hash,auth_revision FROM users"),
            ('overflow', 'UPDATE users SET auth_revision=9223372036854775807'),
            ('invalid-hash', "UPDATE users SET password_hash='broken-fixture'"),
            ('hash-suffix', "UPDATE users SET password_hash=password_hash || 'extra'"),
            ('hash-salt-alphabet', "UPDATE users SET password_hash=substr(password_hash,1,7) || '!' || substr(password_hash,9)"),
            ('hash-digest-alphabet', "UPDATE users SET password_hash=substr(password_hash,1,30) || '!' || substr(password_hash,32)"),
            ('hash-version', "UPDATE users SET password_hash=substr(password_hash,1,2) || 'z' || substr(password_hash,4)"),
            ('hash-signed-cost', "UPDATE users SET password_hash=substr(password_hash,1,4) || '+4' || substr(password_hash,7)"),
            ('trigger', "CREATE TRIGGER offline_malicious AFTER UPDATE ON users BEGIN UPDATE jobs SET message='unexpected'; END"),
            ('cascade', 'CREATE TABLE offline_child(parent TEXT REFERENCES sessions(token_hash) ON DELETE CASCADE)'),
            ('account-cascade', 'CREATE UNIQUE INDEX offline_hash ON users(password_hash); CREATE TABLE offline_child(parent TEXT REFERENCES users(password_hash) ON UPDATE CASCADE)'),
        ]
        for name, statements in invalid_sql:
            target = root / ('invalid-' + name) / 'library.db'
            copy_database(source, target)
            with closing(sqlite3.connect(target)) as database:
                database.executescript(statements)
                database.commit()
            original = snapshot(target)
            code, _, _ = run_cli(binary, ['account', 'reset-password', '--data-dir', str(target.parent), '--password-stdin'], password.encode(), secrets, root / ('logs/' + name + '.log'))
            harness.check(code == 1 and snapshot(target) == original and lock_available(target.parent), '离线预检：' + name + '拒绝、逻辑库不变且锁已释放')
        old = root / 'true-v4/library.db'
        old.parent.mkdir()
        create_legacy_database(old, 4, source)
        original = snapshot(old)
        code, _, _ = run_cli(binary, ['account', 'reset-password', '--data-dir', str(old.parent), '--password-stdin'], password.encode(), secrets, root / 'logs/true-v4.log')
        harness.check(code == 1 and snapshot(old) == original, '离线版本：真正schema4拒绝且未隐式迁移')
        legacy = root / 'true-v5/library.db'
        legacy.parent.mkdir()
        create_legacy_database(legacy, 5, source)
        legacy_before = snapshot(legacy)
        harness.check(legacy_before[0] == 5 and not any(name.startswith('backup_') for name in legacy_before[2]), '离线版本：真实schema5由冻结迁移构造且没有备份新表')
        rejected = root / 'true-v5-rejected/library.db'
        copy_database(legacy, rejected)
        with closing(sqlite3.connect(rejected)) as database:
            database.execute("CREATE TRIGGER offline_v5_malicious AFTER UPDATE ON users BEGIN UPDATE jobs SET message='unexpected'; END")
            database.commit()
        rejected_before = snapshot(rejected)
        code, _, _ = run_cli(binary, ['account', 'reset-password', '--data-dir', str(rejected.parent), '--password-stdin'], password.encode(), secrets, root / 'logs/true-v5-rejected.log')
        harness.check(code == 1 and snapshot(rejected) == rejected_before, '离线版本：真实schema5异常触发器拒绝且原样保留')
        if os.name == 'nt':
            for version, target_version, original_source in ((5, 6, legacy), (6, 5, source)):
                drift = root / ('drift-' + str(version) + '-' + str(target_version)) / 'library.db'
                copy_database(original_source, drift)
                drift_before = snapshot(drift)
                record = run_console(binary, drift.parent, 'version-drift', 2, password, secrets, password, target_version)
                evidence['windowsConsole'].append(record)
                harness.check(snapshot(drift) == (target_version, *drift_before[1:]) and record['sqliteSchemaStable'] is True and lock_available(drift.parent), '离线版本barrier：真实CLI预检' + str(version) + '后改' + str(target_version) + '，结构修订未变仍拒绝账号写入')
        code, _, _ = run_cli(binary, ['account', 'reset-password', '--data-dir', str(legacy.parent), '--password-stdin'], password.encode(), secrets, root / 'logs/true-v5-success.log')
        legacy_after = snapshot(legacy)
        harness.check(code == 0 and legacy_after[:2] == legacy_before[:2] and legacy_after[2] == dict(legacy_before[2], users=legacy_after[2]['users'], sessions=[]) and legacy_after[2]['users'][0][:2] == legacy_before[2]['users'][0][:2] and legacy_after[2]['users'][0][2] != legacy_before[2]['users'][0][2] and legacy_after[2]['users'][0][3] == legacy_before[2]['users'][0][3] + 1, '离线成功：真实schema5仅改hash/修订及会话，没有隐式迁移')
        missing = root / 'absent-root'
        empty = root / 'empty-data'
        empty.mkdir()
        directory_db = root / 'directory-db'
        (directory_db / 'library.db').mkdir(parents=True)
        corrupt = root / 'corrupt-db'
        corrupt.mkdir()
        (corrupt / 'library.db').write_bytes(b'not-a-sqlite-fixture')
        linked = root / 'linked-db'
        linked.mkdir()
        try:
            (linked / 'library.db').symlink_to(source)
            evidence['databaseLinkFixture'] = 'file-symlink'
        except OSError as error:
            if os.name != 'nt' or error.winerror != 1314:
                raise
            # 只有实际遇到符号链接特权拒绝，才能如实记录 Junction 替代边界。
            directory_alias(source.parent, linked / 'library.db')
            evidence['databaseLinkFixture'] = 'directory-junction; file-symlink privilege unavailable'
        for name, target in (('missing-root', missing), ('missing-db', empty), ('directory-db', directory_db), ('corrupt-db', corrupt), ('linked-db', linked)):
            code, _, _ = run_cli(binary, ['account', 'reset-password', '--data-dir', str(target), '--password-stdin'], password.encode(), secrets, root / ('logs/' + name + '.log'))
            harness.check(code == 1 and snapshot(source) == before and not missing.exists() and not (empty / 'library.db').exists() and (corrupt / 'library.db').read_bytes() == b'not-a-sqlite-fixture', '离线路径：' + name + '明确拒绝且不误建空库或账号')

        invalid_input = [b'Aa1!abcde', b'Aa1!' + b'x' * 69, b'aa1!abcdef', b'AA1!ABCDEF', b'Aa!!abcdef', b'Aa12abcdef', 'Aa1!中文abcdef'.encode(), 'Aa1!Ａabcdef'.encode(), 'Aa1!😀abcdef'.encode(), b' Aa1!abcdef', b'Aa1!abcdef ', b'Aa1!abcdef\t', b'Aa1!abcdef\x00', b'Aa1!abcdef\xff', b'Aa1!abcdef\r', b'Aa1!abcdef\n\n', b'Aa1!abc\ndef', b'Aa1!abcdef\r\nextra', b'', b'x' * 10000]
        for index, value in enumerate(invalid_input, 1):
            local_secrets = [*secrets, value.strip(b'\r\n')]
            code, _, _ = run_cli(binary, command, value, local_secrets, root / ('logs/input-' + str(index) + '.log'))
            harness.check(code == 1 and snapshot(source) == before and fixture_files(harness.root) == files and lock_available(source.parent), '离线输入：非法边界' + str(index) + '拒绝且账号/作业/缓存/媒体无写')
        code, _, _ = run_cli(binary, command[:-1], password.encode(), secrets, root / 'logs/no-stdin-flag.log')
        harness.check(code == 1 and snapshot(source) == before, '离线输入：无显式旗标的管道拒绝，不降级成明文交互')

        # 真正Console的两种Ctrl+C分别覆盖两个提示，绝不以管道EOF代替。
        if os.name == 'nt':
            evidence['permissionFixture'] = verify_windows_permission(binary, source, root, secrets, password)
            evidence['actualPermissionDenial'] = evidence['permissionFixture'].get('established') is True
            if evidence['actualPermissionDenial']:
                harness.check(True, '真实ACL：普通写句柄确被拒绝、维护退出1且无写，原始ACL完整恢复')
            for scenario in ('key-cancel', 'event-cancel', 'eof-cancel'):
                for stage in (1, 2):
                    record = run_console(binary, source.parent, scenario, stage, password, secrets, 'Partial1!' + generate_password())
                    evidence['windowsConsole'].append(record)
                    harness.check(snapshot(source) == before and fixture_files(harness.root) == files and lock_available(source.parent), '真实Console：' + scenario + '提示' + str(stage) + '原模式恢复、秘密无回显、130退出且无写')
            for scenario, stage in (('mismatch', 2), ('pipe-event', 1), ('stdin-terminal', 1)):
                record = run_console(binary, source.parent, scenario, stage, password, secrets, 'Different1!' + generate_password())
                evidence['windowsConsole'].append(record)
                harness.check(snapshot(source) == before and fixture_files(harness.root) == files and lock_available(source.parent), '真实Console：' + scenario + '有界退出、恢复及无写通过')
            target = root / 'console-punctuation/library.db'
            copy_database(source, target)
            punctuation_password = 'Aa1' + string.punctuation
            record = run_console(binary, target.parent, 'success-separated', 2, punctuation_password, [*secrets, punctuation_password.encode()], punctuation_password + 'x\b')
            evidence['windowsConsole'].append(record)
            actual_console = snapshot(target)
            harness.check(actual_console[2]['sessions'] == [] and actual_console[2]['users'][0][3] == before[2]['users'][0][3] + 1 and lock_available(target.parent), '真实Console：全部32个标点逐键及退格、两提示分开确认成功且模式恢复')

        # 每种合法边界独立副本真实执行，确认恰好一次推进修订并清掉旧会话。
        valid_inputs: list[tuple[str, bytes]] = [('ten', b'Aa1!abcdef'), ('seventy-two', b'Aa1!' + b'x' * 68), ('lf', password.encode() + b'\n'), ('crlf', password.encode() + b'\r\n'), ('punctuation', ('Aa1' + string.punctuation).encode())]
        for name, value in valid_inputs:
            target = root / ('valid-' + name) / 'library.db'
            copy_database(source, target)
            old_snapshot = snapshot(target)
            code, _, _ = run_cli(binary, ['account', 'reset-password', '--data-dir', str(target.parent), '--password-stdin'], value, [*secrets, value.rstrip(b'\r\n')], root / ('logs/valid-' + name + '.log'))
            actual = snapshot(target)
            previous_user, actual_user = old_snapshot[2]['users'][0], actual[2]['users'][0]
            expected_tables = dict(old_snapshot[2], users=actual[2]['users'], sessions=[])
            harness.check(code == 0 and actual[:2] == old_snapshot[:2] and actual[2] == expected_tables and actual_user[:2] == previous_user[:2] and actual_user[2] != previous_user[2] and actual_user[3] == previous_user[3] + 1 and lock_available(target.parent), '离线成功：' + name + '真实输入仅改hash/修订并清全部会话')

        if os.name == 'nt':
            evidence['windowsConsole'].append(run_console(binary, source.parent, 'success', 1, password, secrets, password))
        else:
            code, _, _ = run_cli(binary, command, password.encode(), secrets, root / 'logs/final-success.log')
            harness.check(code == 0, '离线成功：实际停机密码重置返回成功')
        actual = snapshot(source)
        harness.check(actual[0:2] == before[0:2] and actual[2] == dict(before[2], users=actual[2]['users'], sessions=[]) and actual[2]['users'][0][:2] == before[2]['users'][0][:2] and actual[2]['users'][0][2] != before[2]['users'][0][2] and actual[2]['users'][0][3] == before[2]['users'][0][3] + 1 and fixture_files(harness.root) == files and lock_available(source.parent), '离线最终：所有非账号表、running作业、dispatching动作、pending缓存与媒体原样保留')
        with closing(harness.database()) as database:
            harness.check(database.execute('PRAGMA integrity_check').fetchone()[0] == 'ok' and database.execute('PRAGMA foreign_key_check').fetchone() is None, '离线最终：SQLite完整性与全部外键一致')
        harness.start()
        harness.check(harness.client.request('/api/stats', headers={'Cookie': 'nas_session=' + token})[0] == 401 and harness.client.request('/api/login', 'POST', credentials)[0] == 401 and harness.client.request('/api/login', 'POST', dict(credentials, password=password))[0] == 200, '离线恢复：实际重启后全部旧Cookie/旧密码失效，新密码成功登录')
        evidence.update({'realCLI': True, 'rawOutputSecretScan': True, 'canonicalAliasLock': True, 'noRecoverySideEffects': True, 'restartCredentials': True})
        return evidence
    finally:
        harness.stop()


def run_offline_suite(binary: Path, root: Path) -> dict[str, Any]:
    """保留独立结果契约，失败时仍记录真实已完成断言而不误称离线通过。"""
    assertions: list[str] = []
    evidence: dict[str, Any] = {}
    completed = False
    try:
        evidence = verify_offline(binary, root, assertions)
        completed = True
    finally:
        result = {'passed': completed, 'assertions': len(assertions), 'artifact': file_identity(binary), 'realApplication': completed, 'offlineCLI': completed, 'evidence': evidence, 'limits': ['Windows Console仅在Windows本机真实执行时提供各场景记录；非Windows不自动声明终端通过', 'ACL仅在evidence.actualPermissionDenial=true时有实际拒绝证据，其余环境仍待验', 'Linux交叉构建不是flock/终端/权限或NAS实测；目标NAS仍待验收']}
        write_json(root / 'assertions.json', assertions)
        write_json(root / 'result.json', result)
    return result


def main() -> None:
    """验证显式产物与全新隔离根，隐藏helper入口不接受秘密argv或环境。"""
    if sys.argv[1:] == ['--console-helper']:
        try:
            payload = json.loads(sys.stdin.buffer.read(65537))
            result = console_helper(payload)
            print(json.dumps(result, ensure_ascii=False), flush=True)
        except BaseException as error:
            # 不能打印原始异常、payload、命令或控制台内容。
            locations = ','.join(str(frame.lineno) for frame in traceback.extract_tb(error.__traceback__))
            print('专属Console场景失败：' + (str(error) if isinstance(error, AssertionError) else type(error).__name__) + '；行号=' + locations, file=sys.stderr, flush=True)
            raise SystemExit(1)
        return
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True, help='已构建的主服务产物，维护入口与在线服务必须为同一个文件')  # 不自行下载或构建产物。
    parser.add_argument('--root', help='仓库.runtime/verify下尚不存在的隔离运行根')  # 不复用或覆盖历史证据。
    args = parser.parse_args()
    binary = args.binary.resolve()
    if not binary.is_file() or binary.is_symlink():
        parser.error('--binary必须为已有普通主服务产物')
    try:
        root = new_run_root(args.root, 'account-offline')
    except Blocked as error:
        parser.error(str(error))
    result = run_offline_suite(binary, root)
    print('离线账号验证通过：' + str(result['assertions']) + ' 项断言', flush=True)


if __name__ == '__main__':
    from project_env import run_cli as project_run_cli

    raise SystemExit(project_run_cli(main))
