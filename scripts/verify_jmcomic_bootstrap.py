"""真实 Windows 漫画自动初始化、取消及离线复用验收；只操作隔离副本，不请求站点。"""

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

import jmcomic_runtime
import uav_runtime
from verify_desktop import assert_exclusive, control, file_digest, request, wait_ready
from verify_desktop_collection import authenticated, readonly_program


OFFLINE_PROXY = {"HTTPS_PROXY": "http://127.0.0.1:1", "HTTP_PROXY": "http://127.0.0.1:1", "NO_PROXY": ""}  # 仅对子进程生效的确定性断网代理。
PLUGIN_VERSIONS = {"jmcomic": "2.7.12", "uav": "3.8.4"}  # 本次编译期契约要求的实际 worker 插件版本。
PLUGIN_NAMESPACES = {"jmcomic": "jm-bootstrap", "uav": "uav-bootstrap"}  # 两插件独立的固定应用自有初始化命名空间。


def capability_ready(status: dict[str, Any], kind: str) -> bool:
    """只接受真实 worker 能力及固定版本，不以初始化 ready 代替插件就绪。"""
    return any(item.get("kind") == kind and item.get("available") is True and item.get("version") == PLUGIN_VERSIONS[kind] for item in status.get("collectionCapabilities", []))


def initialization_states(status: dict[str, Any]) -> dict[str, dict[str, Any]]:
    """合并兼容状态和按插件状态数组，检查阶段类型与真实字节进度边界。"""
    values = list(status.get("collectionInitializations", []))
    legacy = status.get("collectionInitialization")
    if legacy is not None:
        values.append(legacy)
    result: dict[str, dict[str, Any]] = {}
    for value in values:
        kind = value["kind"]
        assert kind in PLUGIN_VERSIONS, "初始化状态包含未知插件"
        assert value["state"] in ("initializing", "ready", "failed"), "初始化状态无效"
        assert isinstance(value["stage"], str) and value["stage"], "缺少中文初始化阶段"
        downloaded, total = value["downloadedBytes"], value["totalBytes"]
        assert isinstance(downloaded, int) and isinstance(total, int) and 0 <= downloaded <= total, "下载字节进度越界"
        result[kind] = value
    return result


def wait_initialization(binary: Path, profile: Path, process: subprocess.Popen[bytes], environment: dict[str, str], cwd: Path, wanted: str, kinds: tuple[str, ...] = ("jmcomic",), timeout: float = 120.0, observations: list[dict[str, Any]] | None = None) -> dict[str, Any]:
    """有界等待实际能力或初始化阶段，容忍受控激活时的短暂 starting。"""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        assert process.poll() is None, "初始化期间桌面进程提前退出"
        try:
            status = control(binary, profile, "status", environment, cwd)
        except (AssertionError, json.JSONDecodeError):
            # 首次启动的私有IPC可能尚未建立；只在进程存活及总期限内等待。
            time.sleep(0.2)
            continue
        assert status["state"] != "failed", "媒体服务在初始化期间启动失败"
        states = initialization_states(status)
        if observations is not None:
            observations.extend(value.copy() for value in states.values())
        if wanted == "available" and status["state"] == "ready" and all(capability_ready(status, kind) for kind in kinds):
            return status
        if wanted == "downloading" and any(value["state"] == "initializing" and value["downloadedBytes"] > 0 for kind, value in states.items() if kind in kinds):
            assert status["state"] == "ready", "后台下载阻塞了基础媒体就绪"
            return status
        if wanted in ("initializing", "failed") and any(value["state"] == wanted for kind, value in states.items() if kind in kinds):
            assert status["state"] == "ready", "依赖准备破坏了基础媒体就绪"
            return status
        if wanted in ("available", "downloading") and any(states.get(kind, {}).get("state") == "failed" for kind in kinds):
            reasons = [states[kind]["stage"] for kind in kinds if states.get(kind, {}).get("state") == "failed"]
            raise AssertionError("自动初始化失败：" + "；".join(reasons))
        time.sleep(0.2)
    raise AssertionError("等待指定插件初始化阶段超时")


def snapshot(root: Path) -> dict[str, tuple[str, int]]:
    """记录完整目录的文件摘要和修改时间，用于判定无重装的离线复用。"""
    return {path.relative_to(root).as_posix(): (file_digest(path), path.stat().st_mtime_ns) for path in root.rglob("*") if path.is_file()}


def assert_unpublished(base: Path) -> None:
    """确认失败或取消没有留下已发布选择、暂存或未发布代次。"""
    assert not (base / "current.json").exists(), "失败或取消发布了运行时选择"
    assert not list(base.glob("stage-*")), "失败或取消遗留暂存目录"
    assert not list(base.glob("gen-*")), "失败或取消遗留未发布代次"


def assert_images_released(profile: Path) -> None:
    """退出后立即独占访问两插件全部映像，验证 Job 及原生 DLL 已释放。"""
    for namespace in PLUGIN_NAMESPACES.values():
        for path in (profile / "collection-managed" / namespace).rglob("*"):
            if path.suffix.lower() in (".exe", ".dll", ".pyd"):
                assert_exclusive(path)


def verify_published(profile: Path, kind: str) -> Path:
    """对最终 current 选择执行独立构建侧严格校验和真实离线依赖探针。"""
    base = profile / "collection-managed" / PLUGIN_NAMESPACES[kind]
    selection = json.loads((base / "current.json").read_bytes())
    assert set(selection) == {"schemaVersion", "recipeSha256", "generation"} and selection["schemaVersion"] == 1
    name = selection["generation"]
    assert isinstance(name, str) and len(name) == 36 and name.startswith("gen-") and all(letter in "0123456789abcdef" for letter in name[4:])
    generation = base / name
    if kind == "jmcomic":
        value = jmcomic_runtime.verify(generation / "jmcomic-runtime.json", probe=True)
        assert value["jmcomicVersion"] == PLUGIN_VERSIONS[kind]
        assert not (generation / "tools").exists(), "独立漫画初始化夹带视频工具"
    else:
        value = uav_runtime.verify(generation / "uav-runtime.json", probe=True)
        assert value["uavVersion"] == PLUGIN_VERSIONS[kind]
    return base


def assert_authenticated_capabilities(status: dict[str, Any], kinds: tuple[str, ...]) -> None:
    """从实际登录后的应用 API 再核实 worker 就绪和每个请求插件的可用性。"""
    opener = authenticated(status["url"])
    capabilities = json.loads(request(opener, status["url"], "/api/collection/capabilities")[1])
    assert capabilities["workerReady"] is True
    for kind in kinds:
        assert any(item["kind"] == kind and item["available"] is True for item in capabilities["capabilities"]), "认证 API 未报告实际插件可用"


def media_ready_count(profile: Path) -> int:
    """读取本轮自有桌面日志中的实际媒体就绪次数，约束自动激活重启次数。"""
    return sum(path.read_text(encoding="utf-8", errors="replace").count('"msg":"本机媒体库已就绪"') for path in (profile / "logs").glob("*.log"))


def main() -> int:
    """在仅含两个 EXE 的隔离只读副本验证漫画及可选双插件准备全过程。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True, help="已编译并包含初始化资源的 Windows 桌面 EXE")
    parser.add_argument("--worker", type=Path, required=True, help="与桌面匹配的 Windows worker EXE")
    parser.add_argument("--web-dir", type=Path, help="开发构建使用的前端资源目录；正式嵌入资源包无需传入")
    parser.add_argument("--report", type=Path, required=True, help="不含凭据与私有目录的 JSON 验收记录路径")
    parser.add_argument("--timeout", type=float, default=1800.0, help="每轮真实联网初始化的最大等待秒数，默认1800")
    parser.add_argument("--with-uav", action="store_true", help="额外从空 profile 联网准备 JMComic 和 UAV，验证单次激活与双插件离线复用")
    args = parser.parse_args()
    assert os.name == "nt", "必须在真实 Windows 上执行验收"
    assert args.timeout > 0, "初始化等待秒数必须大于零"
    environment = {key: value for key, value in os.environ.items() if not key.upper().startswith(("NAS_", "ENGINE_", "QBT_", "JACKETT_", "PYTHONHOME", "PYTHONPATH"))}
    if args.web_dir:
        environment["NAS_WEB_DIR"] = str(args.web_dir.resolve(strict=True))
    environment["PATH"] = str(Path(os.environ["SystemRoot"]) / "System32")
    checks: list[str] = []
    observations: list[dict[str, Any]] = []
    processes: list[tuple[Path, subprocess.Popen[bytes]]] = []
    started = time.monotonic()
    online_seconds: dict[str, float] = {}
    with tempfile.TemporaryDirectory(prefix="JM 初始化 ") as temporary:
        root = Path(temporary)
        program, foreign, media = (root / name for name in ("只读 中文程序", "其他 工作目录", "中文 媒体"))
        for directory in (program, foreign, media):
            directory.mkdir()
        binary, worker = program / "栖库.exe", program / "nas-collection-worker.exe"
        shutil.copyfile(args.binary.resolve(strict=True), binary)
        shutil.copyfile(args.worker.resolve(strict=True), worker)
        (program / ".env").write_text("# 本轮隔离配置：明确开启漫画，由桌面自动准备。\nNAS_DESKTOP_COLLECTION_MODE=managed\nNAS_COLLECTION_WORKER_JMCOMIC_ENABLED=true\nNAS_COLLECTION_WORKER_UAV_ENABLED=false\n", encoding="utf-8")
        environment.update(PYTHONHOME=str(foreign), PYTHONPATH=str(foreign))
        profile = root / "隔离 配置"
        log = root / "本轮验收.log"
        with log.open("wb") as output, readonly_program(program):
            def launch(target: Path, extra: dict[str, str] | None = None) -> subprocess.Popen[bytes]:
                """仅启动本轮已登记的隔离 profile，不继承真实业务或站点配置。"""
                process = subprocess.Popen([str(binary), "--headless", "--no-browser", "--profile", str(target), "--media-root", str(media)], env={**environment, **(extra or {})}, cwd=foreign, stdout=output, stderr=output)
                processes.append((target, process))
                return process

            def stop(target: Path, process: subprocess.Popen[bytes]) -> None:
                """通过确切实例 IPC 正常退出，紧接着验证私有运行时映像释放。"""
                control(binary, target, "quit", environment, foreign)
                assert process.wait(timeout=30) == 0, "隔离桌面未正常退出"
                assert_images_released(target)

            try:
                for name, extra in (("关闭开关", {"NAS_COLLECTION_WORKER_JMCOMIC_ENABLED": "false"}), ("停用模式", {"NAS_DESKTOP_COLLECTION_MODE": "disabled"}), ("外接模式", {"NAS_DESKTOP_COLLECTION_MODE": "external"}), ("错误自定义Python", {"NAS_COLLECTION_WORKER_JMCOMIC_PYTHON": str(root / "不存在.exe")})):
                    target = root / name
                    process = launch(target, extra)
                    wait_ready(binary, target, process, environment, foreign)
                    time.sleep(0.5)
                    status = control(binary, target, "status", environment, foreign)
                    assert not initialization_states(status), "禁止自动安装的配置仍触发初始化"
                    assert not (target / "collection-managed/jm-bootstrap").exists()
                    assert not capability_ready(status, "jmcomic")
                    stop(target, process)
                checks.append("false/disabled/external/错误显式Python均零初始化")

                process = launch(profile, OFFLINE_PROXY)
                wait_ready(binary, profile, process, environment, foreign)
                status = wait_initialization(binary, profile, process, environment, foreign, "failed")
                stage = initialization_states(status)["jmcomic"]["stage"]
                assert "初始化失败" in stage and "重新检测" in stage
                base = profile / "collection-managed/jm-bootstrap"
                assert_unpublished(base)
                failed_snapshot = snapshot(base)
                time.sleep(1)
                assert snapshot(base) == failed_snapshot, "失败后仍在无限自动重试"
                before_recheck = media_ready_count(profile)
                control(binary, profile, "collection-check", environment, foreign)
                deadline = time.monotonic() + 30
                while time.monotonic() < deadline and media_ready_count(profile) == before_recheck:
                    time.sleep(0.1)
                assert media_ready_count(profile) == before_recheck + 1, "主动重检没有启动新一轮核验"
                wait_initialization(binary, profile, process, environment, foreign, "failed")
                assert_unpublished(base)
                stop(profile, process)
                checks.append("断网中文失败/基础媒体ready/无半成品/无无限重试/主动重检再次尝试")

                cancel_profile = root / "下载中停用"
                process = launch(cancel_profile)
                wait_initialization(binary, cancel_profile, process, environment, foreign, "downloading", timeout=args.timeout)
                control(binary, cancel_profile, "collection-disable", environment, foreign)
                deadline = time.monotonic() + 30
                while time.monotonic() < deadline:
                    status = control(binary, cancel_profile, "status", environment, foreign)
                    if status["state"] == "ready" and status.get("collectionState") == "disabled":
                        break
                    time.sleep(0.1)
                else:
                    raise AssertionError("下载中停用没有等待清理并恢复基础媒体就绪")
                time.sleep(1)
                assert control(binary, cancel_profile, "status", environment, foreign)["collectionState"] == "disabled"
                assert_unpublished(cancel_profile / "collection-managed/jm-bootstrap")
                stop(cancel_profile, process)
                checks.append("真实下载字节出现后停用，等待清理且无迟到激活")

                quit_profile = root / "下载中退出"
                process = launch(quit_profile)
                wait_initialization(binary, quit_profile, process, environment, foreign, "downloading", timeout=args.timeout)
                stop(quit_profile, process)
                assert_unpublished(quit_profile / "collection-managed/jm-bootstrap")
                checks.append("真实下载中正常退出，无current/stage/未发布gen且映像释放")

                online_started = time.monotonic()
                process = launch(profile)
                wait_initialization(binary, profile, process, environment, foreign, "initializing")
                status = wait_initialization(binary, profile, process, environment, foreign, "available", timeout=args.timeout, observations=observations)
                online_seconds["jmcomic"] = round(time.monotonic() - online_started, 3)
                assert_authenticated_capabilities(status, ("jmcomic",))
                base = verify_published(profile, "jmcomic")
                stop(profile, process)
                before = snapshot(base)
                checks.append("同失败profile真实联网准备，认证API/worker与最终路径严格离线探针通过")
                process = launch(profile, OFFLINE_PROXY)
                wait_initialization(binary, profile, process, environment, foreign, "available")
                stop(profile, process)
                assert snapshot(base) == before, "漫画离线重启改变已发布库存或选择"
                checks.append("漫画离线重启复用全部文件SHA/mtime/current原字节，无视频工具")

                if args.with_uav:
                    both_profile = root / "双插件"
                    both_environment = {"NAS_COLLECTION_WORKER_UAV_ENABLED": "true"}
                    both_started = time.monotonic()
                    process = launch(both_profile, both_environment)
                    status = wait_initialization(binary, both_profile, process, environment, foreign, "available", kinds=("jmcomic", "uav"), timeout=args.timeout, observations=observations)
                    online_seconds["jmcomicAndUAV"] = round(time.monotonic() - both_started, 3)
                    assert_authenticated_capabilities(status, ("jmcomic", "uav"))
                    bases = {kind: verify_published(both_profile, kind) for kind in ("jmcomic", "uav")}
                    assert media_ready_count(both_profile) == 2, "双插件未合并为一次受控自动激活"
                    states = initialization_states(status)
                    assert all(states.get(kind, {}).get("state") == "ready" for kind in ("jmcomic", "uav")), "双插件状态未分别保留最终事实"
                    stop(both_profile, process)
                    before_both = {kind: snapshot(path) for kind, path in bases.items()}
                    process = launch(both_profile, {**both_environment, **OFFLINE_PROXY})
                    status = wait_initialization(binary, both_profile, process, environment, foreign, "available", kinds=("jmcomic", "uav"))
                    stop(both_profile, process)
                    assert all(snapshot(path) == before_both[kind] for kind, path in bases.items()), "双插件离线复用改变已发布材料"
                    assert media_ready_count(both_profile) == 3, "双插件离线复用发生额外自动重启"
                    checks.append("空profile双插件真实联网准备/各自严格探针/一次受控激活/离线复用")
            except BaseException:
                args.report.parent.mkdir(parents=True, exist_ok=True)
                args.report.write_text(json.dumps({"result": "failed", "checksCompleted": checks, "elapsedSeconds": round(time.monotonic() - started, 3), "observedStages": sorted({item["stage"] for item in observations})}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
                raise
            finally:
                for target, process in processes:
                    if process.poll() is None:
                        try:
                            control(binary, target, "quit", environment, foreign)
                            process.wait(timeout=30)
                        except (AssertionError, OSError, subprocess.SubprocessError):
                            # 仅终止本轮确切PID；桌面异常退出后专属Job回收其worker树。
                            process.terminate()
                            process.wait(timeout=30)
        assert_exclusive(binary)
        assert_exclusive(worker)
        assert any(item["kind"] == "jmcomic" and item["downloadedBytes"] > 0 for item in observations), "未观察到漫画真实下载字节进度"
        report = {"result": "passed", "platform": "windows-amd64", "elapsedSeconds": round(time.monotonic() - started, 3), "onlineInitializationSeconds": online_seconds, "observedStages": sorted({item["stage"] for item in observations}), "checks": checks, "limits": ["只读程序/中文空格profile/异cwd/Python环境污染的本机隔离验证，不等于干净普通用户Windows验收", "未请求漫画或视频站点", "未完成正式发行的许可/对应源码人工审查"]}
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(report, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
