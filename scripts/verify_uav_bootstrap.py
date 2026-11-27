"""真实 Windows 在线初始化与离线复用验收；仅使用隔离程序/profile，不访问视频站点。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True
    from project_env import configure_environment
    configure_environment()

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
from typing import Any

import uav_runtime
from verify_desktop import assert_exclusive, control, file_digest, request, wait_ready
from verify_desktop_collection import authenticated, readonly_program


OFFLINE_PROXY = {"HTTPS_PROXY": "http://127.0.0.1:1", "HTTP_PROXY": "http://127.0.0.1:1", "NO_PROXY": ""}  # 仅用于子进程的确定性断网条件。


def uav_ready(status: dict[str, Any]) -> bool:
    """只接受真实 worker 上报的 UAV 能力，不以初始化状态代替探测。"""
    return any(item["kind"] == "uav" and item["available"] and item["version"] == "3.8.4" for item in status.get("collectionCapabilities", []))


def wait_initialization(binary: Path, profile: Path, process: subprocess.Popen[bytes], environment: dict[str, str], cwd: Path, wanted: str, timeout: float = 120.0, observations: list[dict[str, Any]] | None = None) -> dict[str, Any]:
    """有界等待指定初始化阶段或能力，容忍受控激活期间短暂 starting。"""
    deadline = time.monotonic() + timeout
    last: dict[str, Any] = {}
    while time.monotonic() < deadline:
        assert process.poll() is None, f"初始化期间进程退出：{process.returncode}"
        last = control(binary, profile, "status", environment, cwd)
        initialization = last.get("collectionInitialization", {})
        if initialization:
            assert initialization["kind"] == "uav" and initialization["state"] in ("initializing", "ready", "failed")
            assert 0 <= initialization["downloadedBytes"] <= initialization["totalBytes"]
            if observations is not None:
                observations.append(initialization)
        if wanted == "available" and last["state"] == "ready" and uav_ready(last):
            return last
        if wanted != "available" and initialization.get("state") == wanted:
            assert last["state"] == "ready", "依赖下载阻塞或破坏了基础媒体就绪"
            return last
        if wanted == "available" and initialization.get("state") == "failed":
            raise AssertionError(f"在线初始化失败：{initialization['stage']}")
        time.sleep(0.2)
    raise AssertionError(f"等待初始化 {wanted} 超时：{last}")


def snapshot(root: Path) -> dict[str, tuple[str, int]]:
    """记录发布目录全部文件摘要和修改时间，证明离线重启没有重新装配。"""
    return {path.relative_to(root).as_posix(): (file_digest(path), path.stat().st_mtime_ns) for path in root.rglob("*") if path.is_file()}


def media_roundtrip(generation: Path, root: Path, environment: dict[str, str]) -> dict[str, Any]:
    """执行真实静态 FFmpeg 编码、ffprobe 时长探测和解码，不接触站点。"""
    tools = generation / "tools"
    video = root / "自动初始化 媒体回环.mp4"
    encode = subprocess.run([str(tools / "ffmpeg.exe"), "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x90:r=10", "-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", str(video)], env=environment, capture_output=True, timeout=30)
    assert encode.returncode == 0, encode.stderr.decode("utf-8", errors="replace")[-1000:]
    probe = subprocess.run([str(tools / "ffprobe.exe"), "-v", "error", "-show_entries", "format=duration", "-of", "json", str(video)], env=environment, capture_output=True, timeout=20)
    assert probe.returncode == 0
    duration = float(json.loads(probe.stdout)["format"]["duration"])
    assert 0.9 <= duration <= 1.1
    decode = subprocess.run([str(tools / "ffmpeg.exe"), "-nostdin", "-v", "error", "-i", str(video), "-f", "null", "-"], env=environment, capture_output=True, timeout=20)
    assert decode.returncode == 0
    return {"codec": "H.264", "durationSeconds": duration, "encodedBytes": video.stat().st_size, "decode": "passed"}


def main() -> int:
    """在只含两个 EXE 的只读程序副本验收自动初始化及所有关键生命周期。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True, help="已编译的 Windows 桌面 EXE")
    parser.add_argument("--worker", type=Path, required=True, help="已编译的 Windows worker EXE")
    parser.add_argument("--web-dir", type=Path, help="开发构建使用的实际前端资源目录")
    parser.add_argument("--report", type=Path, required=True, help="不含凭据与私有路径的 JSON 验收记录")
    parser.add_argument("--timeout", type=float, default=1800.0, help="首次真实联网初始化总等待秒数，默认1800")
    args = parser.parse_args()
    assert os.name == "nt", "必须使用真实 Windows 验收"
    environment = {key: value for key, value in os.environ.items() if not key.upper().startswith(("NAS_", "ENGINE_", "QBT_", "JACKETT_", "PYTHONHOME", "PYTHONPATH"))}
    if args.web_dir:
        environment["NAS_WEB_DIR"] = str(args.web_dir.resolve(strict=True))
    environment["PATH"] = str(Path(os.environ["SystemRoot"]) / "System32")
    checks: list[str] = []
    observations: list[dict[str, Any]] = []
    processes: list[subprocess.Popen[bytes]] = []
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="UAV 自动初始化 ") as temporary:
        root = Path(temporary)
        program, foreign, media = (root / name for name in ("只读 中文程序", "其他 工作目录", "中文 媒体"))
        for directory in (program, foreign, media):
            directory.mkdir()
        binary = program / "栖库.exe"
        worker = program / "nas-collection-worker.exe"
        shutil.copyfile(args.binary.resolve(strict=True), binary)
        shutil.copyfile(args.worker.resolve(strict=True), worker)
        (program / ".env").write_text("# 本轮隔离配置：明确开启视频，由桌面自动准备。\nNAS_DESKTOP_COLLECTION_MODE=managed\nNAS_COLLECTION_WORKER_UAV_ENABLED=true\nNAS_COLLECTION_WORKER_JMCOMIC_ENABLED=false\n", encoding="utf-8")
        environment.update(PYTHONHOME=str(foreign), PYTHONPATH=str(foreign))
        profile = root / "隔离 配置"
        log = root / "本轮验收.log"
        with log.open("wb") as output, readonly_program(program):
            def launch(target: Path, extra: dict[str, str] | None = None) -> subprocess.Popen[bytes]:
                """只启动登记在本轮隔离集合内的桌面程序，不继承站点凭据。"""
                process = subprocess.Popen([str(binary), "--headless", "--no-browser", "--profile", str(target), "--media-root", str(media)], env={**environment, **(extra or {})}, cwd=foreign, stdout=output, stderr=output)
                processes.append(process)
                return process

            def stop(target: Path, process: subprocess.Popen[bytes]) -> None:
                """通过本实例 IPC 正常退出并立即确认程序和下载映像可独占访问。"""
                control(binary, target, "quit", environment, foreign)
                assert process.wait(timeout=30) == 0, log.read_text(encoding="utf-8", errors="replace")[-2000:]
                # 程序目录仍受真实写入拒绝ACL保护，EXE读写独占检查留到ACL恢复之后。
                for path in (target / "collection-managed/uav-bootstrap").rglob("*"):
                    if path.suffix.lower() in (".exe", ".dll", ".pyd"):
                        assert_exclusive(path)

            try:
                # 停用、外接和自定义 Python 不应触发任何下载或 owned 初始化目录。
                for name, extra in (("关闭开关", {"NAS_COLLECTION_WORKER_UAV_ENABLED": "false"}), ("停用模式", {"NAS_DESKTOP_COLLECTION_MODE": "disabled"}), ("外接模式", {"NAS_DESKTOP_COLLECTION_MODE": "external"}), ("自定义错误解释器", {"NAS_COLLECTION_WORKER_UAV_PYTHON": str(root / "不存在.exe")})):
                    target = root / name
                    process = launch(target, extra)
                    reply = wait_ready(binary, target, process, environment, foreign)
                    time.sleep(0.5)
                    reply = control(binary, target, "status", environment, foreign)
                    assert "collectionInitialization" not in reply, reply
                    assert not (target / "collection-managed/uav-bootstrap").exists()
                    if name == "自定义错误解释器":
                        assert not uav_ready(reply)
                    stop(target, process)
                checks.append("显式false/disabled/external/错误自定义Python零初始化")

                # 确定性下载失败不影响媒体服务，也不发布半成品或无限重试。
                process = launch(profile, OFFLINE_PROXY)
                wait_ready(binary, profile, process, environment, foreign)
                reply = wait_initialization(binary, profile, process, environment, foreign, "failed")
                stage = reply["collectionInitialization"]["stage"]
                assert "初始化失败" in stage and "重新检测" in stage
                base = profile / "collection-managed/uav-bootstrap"
                assert not (base / "current.json").exists()
                failed_snapshot = snapshot(base)
                time.sleep(1)
                assert snapshot(base) == failed_snapshot, "失败后仍在无界自动重试"
                control(binary, profile, "collection-check", environment, foreign)
                time.sleep(0.8)
                wait_initialization(binary, profile, process, environment, foreign, "failed")
                stop(profile, process)
                checks.append("断网失败中文可重试、媒体ready、零半成品发布、无自动重试循环")

                # 下载阶段主动停用必须等待清理，旧完成事件不能重新启用。
                cancel_profile = root / "下载中取消"
                process = launch(cancel_profile)
                wait_initialization(binary, cancel_profile, process, environment, foreign, "initializing")
                control(binary, cancel_profile, "collection-disable", environment, foreign)
                deadline = time.monotonic() + 30
                while time.monotonic() < deadline:
                    reply = control(binary, cancel_profile, "status", environment, foreign)
                    if reply["state"] == "ready" and reply.get("collectionState") == "disabled":
                        break
                    time.sleep(0.1)
                else:
                    raise AssertionError("取消初始化后未完成停用")
                time.sleep(1)
                assert control(binary, cancel_profile, "status", environment, foreign)["collectionState"] == "disabled"
                cancel_base = cancel_profile / "collection-managed/uav-bootstrap"
                assert not (cancel_base / "current.json").exists()
                assert not list(cancel_base.glob("gen-*")), "取消遗留未发布generation"
                stop(cancel_profile, process)
                checks.append("下载中停用等待清理且无迟到激活")

                quit_profile = root / "下载中退出"
                process = launch(quit_profile)
                wait_initialization(binary, quit_profile, process, environment, foreign, "initializing")
                stop(quit_profile, process)
                quit_base = quit_profile / "collection-managed/uav-bootstrap"
                assert not (quit_base / "current.json").exists()
                assert not list(quit_base.glob("stage-*")) and not list(quit_base.glob("gen-*"))
                checks.append("下载中正常退出等待清理，未发布generation且无暂存残留")

                # 同一失败 profile 下次联网启动自动恢复，程序目录始终保持只读。
                online_started = time.monotonic()
                process = launch(profile)
                initializing = wait_initialization(binary, profile, process, environment, foreign, "initializing")
                assert not uav_ready(initializing)
                reply = wait_initialization(binary, profile, process, environment, foreign, "available", args.timeout, observations)
                initialization_seconds = round(time.monotonic() - online_started, 3)
                opener = authenticated(reply["url"])
                capabilities = json.loads(request(opener, reply["url"], "/api/collection/capabilities")[1])
                assert capabilities["workerReady"] and any(item["kind"] == "uav" and item["available"] for item in capabilities["capabilities"])
                selection = json.loads((base / "current.json").read_bytes())
                generation = base / selection["generation"]
                runtime = uav_runtime.verify(generation / "uav-runtime.json", probe=True)
                assert runtime["uavVersion"] == "3.8.4"
                media_evidence = media_roundtrip(generation, root, environment)
                stop(profile, process)
                before = snapshot(base)
                checks.append("只读程序/中文空格路径/异cwd/无PythonPATH依赖，真实20物料联网还原及worker探测")
                checks.append("真实H.264编码、ffprobe时长与解码通过，退出全部映像释放")

                # 离线重启必须直接复用现有generation，字节与mtime均不变。
                process = launch(profile, OFFLINE_PROXY)
                wait_initialization(binary, profile, process, environment, foreign, "available")
                stop(profile, process)
                assert snapshot(base) == before, "离线重启修改或重新下载了已发布依赖"
                checks.append("无网络重启直接复用原generation，全部文件摘要/mtime/current完全不变")
            except BaseException:
                # 失败保留本轮隔离诊断，便于分析真实上游网络或加载错误；不记录账号口令。
                output.flush()
                args.report.parent.mkdir(parents=True, exist_ok=True)
                diagnostics = [log.read_text(encoding="utf-8", errors="replace")]
                for target in root.iterdir():
                    if target.is_dir():
                        for worker_log in (target / "logs").glob("*.log"):
                            diagnostics.append("\n" + worker_log.name + "\n" + worker_log.read_text(encoding="utf-8", errors="replace"))
                args.report.with_suffix(".failure.log").write_text("\n".join(diagnostics), encoding="utf-8")
                raise
            finally:
                for process in processes:
                    if process.poll() is None:
                        # 仅清理本轮确切实例；优先通过已知profile的IPC退出。
                        process.terminate()
                        process.wait(timeout=30)
        assert_exclusive(binary)
        assert_exclusive(worker)
        assert any(item["downloadedBytes"] > 0 for item in observations), "未观察到真实下载字节进度"
        report = {"result": "passed", "platform": "windows-amd64", "elapsedSeconds": round(time.monotonic() - started, 3), "onlineInitializationSeconds": initialization_seconds, "observedStages": sorted({item["stage"] for item in observations}), "checks": checks, "media": media_evidence, "limits": ["隔离本机开发环境，不等于干净Windows验收", "未请求视频站点", "未完成正式发行的许可/对应源码审查"]}
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(report, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
