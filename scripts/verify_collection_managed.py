"""验证真实托管 worker 的私有管道、运行时降级、诊断与停止屏障，不访问漫画站点。"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import queue
import secrets
import subprocess
import sys
import threading
import time
from typing import Any
import urllib.error
import urllib.request
import uuid

from project_env import ROOT, configure_console, configure_environment, resolve_command
from verify_baseline import clean_environment, digest, new_run_root, write_json
from verify_jmcomic import FIXTURE_GO


MAX_LINE: int = 16 * 1024  # 与生产父子控制协议一致的最大行字节数。


class ManagedWorker:
    """只持有本次创建的真实 worker 及其私有匿名管道，不按名称终止进程。"""

    def __init__(self, binary: Path, root: Path, overrides: dict[str, str] | None = None) -> None:
        """准备中文空格路径、独立数据根和受污染但可控的子进程环境。"""
        self.root: Path = root  # 本次测试的独占根，不复用用户目录。
        self.data: Path = root / "执行端 数据"  # ready必须精确报告该规范根。
        self.token: Path = root / "私密目录" / "token"  # 测试专属令牌，不打印或写报告。
        self.nonce: str = secrets.token_hex(32)  # 本次管道关联秘密，仅保留内存。
        self.instance: str = uuid.uuid4().hex  # 模拟桌面稳定身份。
        self.process: subprocess.Popen[bytes] | None = None  # 唯一本测试进程。
        self.log: Any = None  # 只记录生产脱敏stderr，不保存控制消息。
        self.lines: queue.Queue[bytes | BaseException] = queue.Queue()  # 有界单条就绪读取结果。
        self.ready: dict[str, Any] = {}  # 验证后的本次就绪记录。
        root.mkdir(mode=0o700)
        self.environment: dict[str, str] = clean_environment()  # 隔离NAS配置和用户代理。
        self.environment.update({
            "PATH": "",  # 托管模式不得悄悄使用系统Python或其他工具。
            "PYTHONHOME": str(root / "错误 Python 根"),  # 隔离启动必须忽略环境污染。
            "PYTHONPATH": str(root / "不可信 模块"),  # 不允许工作目录模块注入。
            "NAS_COLLECTION_WORKER_ADDR": "127.0.0.1:0",  # 由真正监听器选择端口。
            "NAS_COLLECTION_WORKER_DATA_DIR": str(self.data),  # 独占持久根。
            "NAS_COLLECTION_WORKER_TOKEN_FILE": str(self.token),  # 专用认证文件。
            "NAS_COLLECTION_WORKER_JMCOMIC_ENABLED": "true",  # 验证真实依赖降级。
            "NAS_COLLECTION_WORKER_MIN_FREE_MB": "0",  # 本地小样例不预留额外磁盘空间。
            "NAS_COLLECTION_WORKER_MAX_BYTES": "20971520",  # 单操作20MiB预算。
            "NAS_COLLECTION_WORKER_MAX_FILES": "100",  # 单操作最多100页。
        })
        self.environment.update(overrides or {})
        self.binary: Path = binary  # 预先解析的真实worker绝对路径。

    def start(self, message: bytes | None = None, expect_ready: bool = True) -> None:
        """启动真实托管入口，严格等待唯一ready对象而不是普通日志。"""
        self.log = (self.root / "worker-stderr.log").open("wb")
        self.process = subprocess.Popen([str(self.binary), "--managed-stdio"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.log, env=self.environment, cwd=self.root)
        if message is None:
            message = self.message("start")
        assert self.process.stdin is not None and self.process.stdout is not None
        self.process.stdin.write(message)
        self.process.stdin.flush()
        if not expect_ready:
            return

        def read_line() -> None:
            """读取一条有界消息，进程关闭时由关闭管道释放。"""
            try:
                assert self.process is not None and self.process.stdout is not None
                self.lines.put(self.process.stdout.readline(MAX_LINE + 1))
            except BaseException as error:
                self.lines.put(error)

        threading.Thread(target=read_line, daemon=True).start()
        line = self.lines.get(timeout=40)
        assert isinstance(line, bytes) and 0 < len(line) <= MAX_LINE and line.endswith(b"\n"), "未收到有界ready"
        self.ready = json.loads(line)
        assert set(self.ready) == {"version", "action", "nonce", "instanceId", "address", "readRoot"}
        assert self.ready["version"] == 1 and self.ready["action"] == "ready"
        assert self.ready["nonce"] == self.nonce and self.ready["instanceId"] == self.instance
        assert self.ready["readRoot"] == str(self.data.resolve())
        assert self.ready["address"].startswith("http://127.0.0.1:") and not self.ready["address"].endswith(":0")

    def message(self, action: str) -> bytes:
        """生成当前会话固定消息，不把秘密写入磁盘或日志。"""
        return (json.dumps({"version": 1, "action": action, "nonce": self.nonce, "instanceId": self.instance}) + "\n").encode("ascii")

    def request(self, route: str, body: dict[str, Any] | None = None, authenticated: bool = True) -> tuple[int, dict[str, Any]]:
        """经实际HTTP执行有界认证请求，异常仅保留状态不回显令牌。"""
        headers = {"Content-Type": "application/json"}
        if authenticated:
            headers["Authorization"] = "Bearer " + self.token.read_text(encoding="utf-8").strip()
        request = urllib.request.Request(self.ready["address"] + route, data=json.dumps(body).encode() if body is not None else None, headers=headers)
        try:
            response = urllib.request.build_opener(urllib.request.ProxyHandler({})).open(request, timeout=5)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, json.loads(response.read(256 * 1024 + 1))

    def stop(self, action: str = "stop", expected: int = 0, eof: bool = False) -> None:
        """通过当前管道停止并等待根锁释放，失败兜底只针对本次PID。"""
        assert self.process is not None and self.process.stdin is not None
        if not eof:
            self.process.stdin.write(self.message(action))
            self.process.stdin.flush()
        self.process.stdin.close()
        assert self.process.wait(timeout=40) == expected, "托管进程退出码不符"
        assert self.process.stdout is not None and self.process.stdout.read() == b"", "控制stdout混入普通日志"
        self.log.close()

    def close(self) -> None:
        """失败恢复时只回收本次持有的worker，绝不按进程名操作。"""
        if self.process is not None:
            if self.process.poll() is None:
                if self.process.stdin is not None:
                    self.process.stdin.close()
                try:
                    self.process.wait(timeout=40)
                except subprocess.TimeoutExpired:
                    self.process.kill()
                    self.process.wait(timeout=5)
            if self.process.stdout is not None:
                self.process.stdout.close()
        if self.log is not None and not self.log.closed:
            self.log.close()


def check_diagnostics(binary: Path, root: Path, python: Path | None, manifest: Path | None) -> list[str]:
    """验证诊断不要求令牌、不创建持久根，明确区分未启用和配置错误。"""
    cases: list[tuple[str, dict[str, str], int, str]] = [
        ("disabled", {}, 2, "未启用"),
        ("missing-manifest", {"NAS_COLLECTION_WORKER_JMCOMIC_ENABLED": "true", "NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST": str(root / "missing.json")}, 2, "私有运行时"),
        ("invalid-explicit-no-fallback", {"NAS_COLLECTION_WORKER_JMCOMIC_ENABLED": "true", "NAS_COLLECTION_WORKER_JMCOMIC_PYTHON": str(root / "missing.exe"), "NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST": str(root / "missing.json")}, 2, "显式配置"),
    ]
    if python is not None:
        cases.append(("real-interpreter", {"NAS_COLLECTION_WORKER_JMCOMIC_ENABLED": "true", "NAS_COLLECTION_WORKER_JMCOMIC_PYTHON": str(python)}, 0, "本地依赖可用"))
    if manifest is not None:
        cases.append(("private-runtime", {"NAS_COLLECTION_WORKER_JMCOMIC_ENABLED": "true", "NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST": str(manifest)}, 0, "本地依赖可用"))
        cases.append(("explicit-invalid-before-valid-runtime", {"NAS_COLLECTION_WORKER_JMCOMIC_ENABLED": "true", "NAS_COLLECTION_WORKER_JMCOMIC_PYTHON": str(root / "missing.exe"), "NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST": str(manifest)}, 2, "显式配置"))
    passed: list[str] = []
    for name, overrides, status, text in cases:
        current = root / name
        current.mkdir()
        environment = clean_environment()
        environment.update({"PATH": "", "PYTHONHOME": "invalid", "PYTHONPATH": str(current)})
        environment.update(overrides)
        result = subprocess.run([str(binary), "--check-jmcomic"], env=environment, cwd=current, capture_output=True, timeout=20)
        assert result.returncode == status and text in result.stdout.decode("utf-8")
        assert not list(current.iterdir()), "诊断创建了worker持久文件"
        assert b"Traceback" not in result.stdout and b"Traceback" not in result.stderr
        passed.append("diagnostic-" + name)
    return passed


def run_suite(args: argparse.Namespace, root: Path) -> list[str]:
    """验证拒绝边界、真实认证、正常停止与持久操作退出屏障。"""
    binary = args.worker.resolve(strict=True)
    passed = check_diagnostics(binary, root, args.python.resolve(strict=True) if args.python else None, args.runtime_manifest.resolve(strict=True) if args.runtime_manifest else None)
    worker = ManagedWorker(binary, root / "缺少运行时 仍可服务")
    try:
        worker.start()
        assert worker.request("/healthz", authenticated=False) == (200, {"ok": True})
        assert worker.request("/v1/capabilities", authenticated=False)[0] == 401
        status, capabilities = worker.request("/v1/capabilities")
        comic = next(value for value in capabilities["capabilities"] if value["kind"] == "jmcomic")
        assert status == 200 and not comic["available"] and "私有运行时缺失" in comic["reason"]
        worker.stop()
        passed.extend(["real-bound-ready-and-auth", "managed-no-path-fallback", "normal-stop"])
        worker.start()
        worker.stop(eof=True)
        passed.append("parent-eof-and-immediate-profile-restart")
    finally:
        worker.close()
    if args.runtime_manifest is not None:
        private = ManagedWorker(binary, root / "真实私有运行时", {"NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST": str(args.runtime_manifest.resolve(strict=True))})
        try:
            private.start()
            status, capabilities = private.request("/v1/capabilities")
            comic = next(value for value in capabilities["capabilities"] if value["kind"] == "jmcomic")
            assert status == 200 and comic["available"], "真实私有运行时未通过托管依赖探测"
            private.stop()
            passed.append("managed-private-runtime-with-empty-path")
        finally:
            private.close()
    startup_eof = ManagedWorker(binary, root / "启动即父管道断开")
    try:
        startup_eof.start(message=b"", expect_ready=False)
        startup_eof.stop(eof=True, expected=1)
        assert not startup_eof.data.exists()
        passed.append("startup-parent-eof-before-data-init")
    finally:
        startup_eof.close()
    invalid = [
        b"{}\n", b"[]\n", b'{"version":1,"version":1}\n', b"x" * (MAX_LINE + 1) + b"\n",
        b'{"version":1,"action":"start","nonce":null,"instanceId":"test"}\n',
        b'{"version":1,"action":"start","nonce":"' + b"a" * 64 + b'","instanceId":"' + b"b" * 32 + b'","extra":true}\n',
    ]
    for index, message in enumerate(invalid):
        invalid_worker = ManagedWorker(binary, root / f"invalid-start-{index}")
        try:
            invalid_worker.start(message=message, expect_ready=False)
            assert invalid_worker.process is not None
            assert invalid_worker.process.wait(timeout=10) == 1
            assert not invalid_worker.data.exists()
            passed.append(f"invalid-start-{index}")
        finally:
            invalid_worker.close()
    fixture = root / "协议替身 工具"
    fixture.mkdir()
    source = fixture / "main.go"
    source.write_text(FIXTURE_GO, encoding="utf-8")
    tool = fixture / ("comic-fixture.exe" if os.name == "nt" else "comic-fixture")
    go = ROOT / ".tools/go/bin" / ("go.exe" if os.name == "nt" else "go")
    command = [str(go) if go.is_file() else "go", "build", "-o", str(tool), str(source)]
    try:
        subprocess.run(resolve_command(command), cwd=ROOT, check=True, timeout=180)
    finally:
        source.unlink(missing_ok=True)
    active = ManagedWorker(binary, root / "下载中停止", {"NAS_COLLECTION_WORKER_JMCOMIC_PYTHON": str(tool)})
    try:
        active.start()
        status, preview = active.request("/v1/previews", {"kind": "jmcomic", "albumId": "1009", "profile": "comic", "maxBytes": 20971520, "maxFiles": 100})
        assert status == 200
        operation_id = str(uuid.uuid4())
        status, _ = active.request("/v1/operations", {"id": operation_id, "previewId": preview["id"], "kind": "jmcomic", "albumId": "1009", "profile": "comic", "maxBytes": 20971520, "maxFiles": 100})
        assert status == 202
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            status, operation = active.request("/v1/operations/" + operation_id)
            if status == 200 and operation["state"] == "running":
                break
            time.sleep(.05)
        else:
            raise AssertionError("样例下载未进入运行态")
        active.stop()
        snapshot = json.loads((active.data / "private-state" / (operation_id + ".json")).read_text(encoding="utf-8"))
        assert snapshot["operation"]["state"] == "recovery_required"
        assert not (active.data / "complete" / operation_id).exists()
        active.start()
        status, operation = active.request("/v1/operations/" + operation_id)
        assert status == 200 and operation["state"] == "recovery_required"
        time.sleep(.5)
        active.stop()
        passed.extend(["stop-running-owned-tool-and-release-root", "preserve-uncertain-no-auto-retry"])
    finally:
        active.close()
    foreign = ManagedWorker(binary, root / "错误停止关联")
    try:
        foreign.start()
        foreign.nonce = secrets.token_hex(32)
        foreign.stop(expected=1)
        passed.append("foreign-stop-cancels-and-rejects")
    finally:
        foreign.close()
    return passed


def main() -> int:
    """初始化项目环境并将执行证据写入全新验证目录。"""
    configure_console()
    configure_environment()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--worker", required=True, type=Path, help="本次构建的真实worker二进制绝对路径")
    parser.add_argument("--python", type=Path, help="可选真实固定依赖Python；用于本地probe，不代表私有发行验收")
    parser.add_argument("--runtime-manifest", type=Path, help="可选真实私有运行时清单；执行实际清单校验和依赖probe")
    parser.add_argument("--root", help="仅允许.runtime/verify下的全新证据目录")
    args = parser.parse_args()
    root = new_run_root(args.root, "jmcomic-managed-worker")
    result: dict[str, Any] = {"status": "failed", "scope": "真实本地worker控制与受控离线协议，不含公网/干净系统验收", "workerSha256": digest(args.worker.resolve()), "cases": []}
    try:
        result["cases"] = run_suite(args, root)
        result["status"] = "passed"
        print(f"托管worker验证通过：{len(result['cases'])}项；证据目录：{root}")
        return 0
    finally:
        write_json(root / "result.json", result)


if __name__ == "__main__":
    raise SystemExit(main())
