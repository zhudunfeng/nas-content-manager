"""在隔离 Windows 副本验证程序同级 .env、配置优先级和真实托管 worker；不访问站点。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True
    from project_env import configure_environment
    configure_environment()

import argparse
import ctypes
from ctypes import wintypes
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
from typing import Any

from verify_desktop import assert_exclusive, control, execute, request, wait_ready
from verify_desktop_collection import authenticated, descendants, observed_image, wait_state


def main() -> int:
    """运行配置边界及实际启动回归，并可输出不含配置值的检查报告。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True, help="当前 Windows 桌面 EXE")
    parser.add_argument("--worker", type=Path, required=True, help="当前 Windows 获取 worker EXE")
    parser.add_argument("--web-dir", type=Path, help="非内嵌构建使用的网页目录")
    parser.add_argument("--report", type=Path, help="可选的脱敏 JSON 验证报告")
    args = parser.parse_args()
    assert os.name == "nt", "此验证必须运行于真实 Windows"
    # 保留 runner 的缓存/临时目录，只清理会影响业务配置的父环境。
    environment = {key: value for key, value in os.environ.items() if not key.upper().startswith(("NAS_", "ENGINE_", "QBT_", "JACKETT_"))}
    if args.web_dir:
        environment["NAS_WEB_DIR"] = str(args.web_dir.resolve(strict=True))
    checks: list[str] = []
    with tempfile.TemporaryDirectory(prefix="Windows 同级配置 ") as temporary:
        root = Path(temporary)
        program, foreign, media = (root / name for name in ("中文 程序", "其他 工作目录", "中文 媒体"))
        for directory in (program, foreign, media):
            directory.mkdir()
        binary = program / "栖库.exe"
        worker = program / "nas-collection-worker.exe"
        shutil.copyfile(args.binary.resolve(strict=True), binary)
        shutil.copyfile(args.worker.resolve(strict=True), worker)
        dotenv = program / ".env"
        profile = root / "自定义 配置"
        profile.mkdir()
        (foreign / ".env").write_text("NAS_DESKTOP_COLLECTION_MODE=external\nNAS_ADDR=错误的工作目录\n", encoding="utf-8")
        (profile / ".env").write_text("NAS_ADDR=错误的配置目录\n", encoding="utf-8")

        def print_config(extra: dict[str, str] | None = None, flags: list[str] | None = None) -> dict[str, Any]:
            """检查只读合并结果，避免打印环境或文件中的秘密值。"""
            result = execute(binary, ["--headless", "--profile", str(profile), "--print-config", *(flags or [])], {**environment, **(extra or {})}, foreign)
            assert result.returncode == 0, result.stderr[-1200:]
            return json.loads(result.stdout)

        baseline = print_config()
        assert baseline["port"] == 8090 and baseline["collectionMode"] == "disabled"
        assert not (profile / "desktop.json").exists() and not (program / "qiku-data").exists()
        checks.append("缺文件兼容、忽略CWD/profile文件、print-config只读")

        chosen = root / "文件指定 数据"
        dotenv.write_text(f"\ufeff# 中文注释\r\nexport nas_data_dir='{chosen}'\r\nNAS_ADDR=127.0.0.1:18171 # 行内注释\r\nNAS_DESKTOP_COLLECTION_MODE=disabled\r\nNAS_TEST_SECRET='不应输出的配置值'\r\n", encoding="utf-8")
        assert print_config()["dataDir"] == str(chosen)
        assert print_config()["port"] == 18171
        assert print_config({"nas_addr": "127.0.0.1:18172"})["port"] == 18172
        assert print_config({"NAS_ADDR": ""})["port"] == 8090
        assert print_config({"NAS_ADDR": "127.0.0.1:18172"}, ["--port", "18173"])["port"] == 18173
        assert print_config()["mediaRoots"] == [] and not chosen.exists()
        checks.append("BOM/CRLF/中文引号路径、CLI高于环境高于文件、大小写及空值覆盖")

        # 路径中的 shell 片段必须原样返回，不能执行命令或展开变量。
        literal = str(root / "${UNCHANGED}%TEMP%`echo nope`#原文")
        dotenv.write_text(f'NAS_DATA_DIR="{literal}"\nNAS_ADDR=127.0.0.1:18174\nNAS_ADDR=127.0.0.1:18175\n', encoding="utf-8")
        config = print_config()
        assert config["dataDir"] == literal and config["port"] == 18175 and not Path(literal).exists()
        checks.append("重复键最后生效、路径反斜杠及shell片段不展开不执行")

        for content in (b"NAS_BROKEN='private-error-sentinel", b"NAS_KEY=private-error-sentinel\x00", b"NAS_KEY=\xff", b"#" + b"x" * 65536):
            dotenv.write_bytes(content)
            result = execute(binary, ["--headless", "--profile", str(profile), "--print-config"], environment, foreign)
            assert result.returncode != 0 and ".env" in result.stderr
            assert "private-error-sentinel" not in result.stderr + result.stdout
        result = execute(binary, ["--help"], environment, foreign)
        assert result.returncode == 0
        dotenv.unlink()
        dotenv.mkdir()
        result = execute(binary, ["--headless", "--print-config"], environment, foreign)
        assert result.returncode != 0 and ".env" in result.stderr
        dotenv.rmdir()
        checks.append("格式/编码/NUL/超限/非普通文件拒绝、不泄露值、帮助可用")

        # Windows 零共享句柄模拟真实文件不可读，不修改用户 ACL。
        dotenv.write_text("NAS_WORKERS=1\n", encoding="utf-8")
        kernel = ctypes.WinDLL("kernel32", use_last_error=True)
        kernel.CreateFileW.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD, ctypes.c_void_p, wintypes.DWORD, wintypes.DWORD, wintypes.HANDLE]
        kernel.CreateFileW.restype = wintypes.HANDLE
        kernel.CloseHandle.argtypes = [wintypes.HANDLE]
        kernel.CloseHandle.restype = wintypes.BOOL
        handle = kernel.CreateFileW(str(dotenv), 0x80000000, 0, None, 3, 0x80, None)
        assert handle != wintypes.HANDLE(-1).value
        try:
            result = execute(binary, ["--headless", "--print-config"], environment, foreign)
            assert result.returncode != 0 and ".env" in result.stderr
        finally:
            assert kernel.CloseHandle(handle)
        checks.append("真实Windows文件不可读时明确失败")

        output_path = root / "实际启动.log"
        processes: list[subprocess.Popen[bytes]] = []
        with output_path.open("wb") as output:
            def launch(extra: dict[str, str] | None = None) -> subprocess.Popen[bytes]:
                """只启动本轮隔离副本，并登记清理范围。"""
                process = subprocess.Popen([str(binary), "--headless", "--no-browser", "--profile", str(profile), "--media-root", str(media)], cwd=foreign, env={**environment, **(extra or {})}, stdout=output, stderr=output)
                processes.append(process)
                return process

            def quit_instance(process: subprocess.Popen[bytes]) -> None:
                """通过真实IPC退出并确认主程序与worker文件释放。"""
                control(binary, profile, "quit", environment, foreign)
                assert process.wait(timeout=25) == 0
                assert_exclusive(binary)
                assert_exclusive(worker)

            def managed_text(enabled: str = "true", concurrent: int = 1) -> str:
                """生成带固定边界污染项的文件，验证托管白名单保持。"""
                return "\n".join((
                    "# 开启本轮隔离托管进程。", "NAS_DESKTOP_COLLECTION_MODE=managed",
                    f"NAS_COLLECTION_WORKER_UAV_ENABLED={enabled}", "NAS_COLLECTION_WORKER_JMCOMIC_ENABLED=false",
                    f"NAS_COLLECTION_WORKER_MAX_CONCURRENT={concurrent}", "NAS_SCAN_INTERVAL_MINUTES=17", "NAS_CACHE_MAX_MB=32",
                    f"NAS_COLLECTION_WORKER_DATA_DIR='{root / '不得创建的数据'}'", f"NAS_COLLECTION_WORKER_TOKEN_FILE='{root / '不得创建的令牌'}'",
                    "NAS_COLLECTION_WORKER_UAV_RUNTIME_MANIFEST=不得采用的清单", "NAS_COLLECTION_WORKER_ADDR=错误的地址",
                    "PATH=不得改变路径", "PYTHONPATH=不得透传", "NAS_TEST_SECRET=private-log-sentinel", "",
                ))

            try:
                dotenv.write_text("NAS_WORKERS=5\nNAS_DESKTOP_COLLECTION_MODE=disabled\n", encoding="utf-8")
                process = launch()
                assert process.wait(timeout=25) != 0
                assert "NAS_WORKERS" in output_path.read_text(encoding="utf-8")
                process = launch({"NAS_WORKERS": "1"})
                wait_ready(binary, profile, process, environment, foreign)
                quit_instance(process)
                checks.append("共享app实际读取文件配置、环境覆盖错误文件值后可启动")

                dotenv.write_text(managed_text(), encoding="utf-8")
                process = launch()
                reply = wait_ready(binary, profile, process, environment, foreign)
                assert reply["collectionMode"] == "managed" and reply["collectionState"] == "unavailable"
                assert [item["kind"] for item in reply["collectionCapabilities"]] == ["uav"]
                assert any(observed_image(pid) == worker for pid in descendants(process.pid))
                opener = authenticated(reply["url"])
                _, payload = request(opener, reply["url"], "/api/settings")
                settings = json.loads(payload)
                assert settings["scanIntervalMinutes"] == 17 and settings["cacheMaxBytes"] == 32 * 1024 * 1024
                _, payload = request(opener, reply["url"], "/api/collection/capabilities")
                capabilities = json.loads(payload)
                assert capabilities["workerReady"] is True
                by_kind = {item["kind"]: item for item in capabilities["capabilities"]}
                assert "视频插件未启用" not in by_kind["uav"].get("reason", "")
                assert (profile / "collection-managed/secrets/token").is_file()
                assert not (root / "不得创建的数据").exists() and not (root / "不得创建的令牌").exists()
                checks.append("文件启用真实视频worker、认证API和应用参数生效、固定路径不被覆盖")

                # 修改为不同配置并触发内部重启，本次进程必须继续使用启动快照。
                dotenv.write_text(managed_text("false").replace("MINUTES=17", "MINUTES=19"), encoding="utf-8")
                workers = {pid for pid in descendants(process.pid) if observed_image(pid) == worker}
                control(binary, profile, "collection-check", environment, foreign)
                reply = wait_state(binary, profile, process, environment, foreign, "unavailable", workers)
                assert [item["kind"] for item in reply["collectionCapabilities"]] == ["uav"]
                _, payload = request(opener, reply["url"], "/api/settings")
                assert json.loads(payload)["scanIntervalMinutes"] == 17
                dotenv.write_text("NAS_BROKEN='private-control-sentinel", encoding="utf-8")
                assert control(binary, profile, "status", environment, foreign)["state"] == "ready"
                quit_instance(process)
                assert execute(binary, ["--headless", "--print-config"], environment, foreign).returncode != 0
                checks.append("内部重检复用启动快照、坏文件仍可status/quit、下次启动重新校验")

                dotenv.write_text(managed_text(), encoding="utf-8")
                process = launch({"NAS_COLLECTION_WORKER_UAV_ENABLED": "false"})
                reply = wait_ready(binary, profile, process, environment, foreign)
                assert reply["collectionState"] == "unavailable" and not reply.get("collectionCapabilities")
                quit_instance(process)
                process = launch({"NAS_COLLECTION_WORKER_UAV_ENABLED": ""})
                reply = wait_ready(binary, profile, process, environment, foreign)
                assert reply["collectionState"] == "failed"
                quit_instance(process)
                checks.append("真实worker开关环境false和显式空值均优先于文件true")

                dotenv.write_text(managed_text(concurrent=0), encoding="utf-8")
                process = launch()
                reply = wait_ready(binary, profile, process, environment, foreign)
                assert reply["collectionState"] == "failed"
                quit_instance(process)
                process = launch({"NAS_COLLECTION_WORKER_MAX_CONCURRENT": "1"})
                reply = wait_ready(binary, profile, process, environment, foreign)
                assert reply["collectionState"] == "unavailable"
                quit_instance(process)
                checks.append("真实worker配额由文件传入、环境覆盖后恢复、保存配置被文件覆盖")

                logs = output_path.read_text(encoding="utf-8") + "".join(path.read_text(encoding="utf-8") for path in (profile / "logs").glob("*.log"))
                assert all(value not in logs for value in ("private-log-sentinel", "private-control-sentinel"))
                assert_exclusive(dotenv)
                checks.append("日志不输出文件秘密值且程序退出释放.env文件")
            finally:
                for process in processes:
                    if process.poll() is None:
                        process.kill()
                        process.wait(timeout=10)
    report = {"status": "passed", "platform": "windows", "checks": checks, "count": len(checks), "scope": "真实桌面和worker；无私有Python包、无站点网络、无Docker"}
    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
