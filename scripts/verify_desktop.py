"""在 Windows 隔离目录验证真实桌面二进制；不访问 Docker，不操作真实媒体或用户数据。"""

from __future__ import annotations

import argparse
import http.cookiejar
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
from typing import Any
import urllib.parse
import urllib.request

from verify import generate_password


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


def main() -> None:
    """验证跨工作目录、实际端口保留、配置恢复、单实例和数据锁，结束后精确回收样本。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True, help="已构建的 Windows 桌面 EXE，推荐带内嵌网页的发布产物")
    parser.add_argument("--web-dir", type=Path, help="仅开发构建使用的显式前端目录；省略时验证包内嵌资源")
    args = parser.parse_args()
    assert os.name == "nt", "此脚本需要真实 Windows；Linux 使用原有 verify.py"
    binary = args.binary.resolve(strict=True)
    environment = {key: value for key, value in os.environ.items() if not key.upper().startswith(("NAS_", "ENGINE_", "QBT_", "JACKETT_"))}
    if args.web_dir:
        environment["NAS_WEB_DIR"] = str(args.web_dir.resolve(strict=True))
    processes: list[subprocess.Popen[bytes]] = []
    with tempfile.TemporaryDirectory(prefix="栖库 启动验证 ") as temporary:
        root = Path(temporary)
        profile = root / "profiles" / "native"
        media = root / "中文 资料;音乐"
        foreign = root / "任意 工作目录"
        media.mkdir()
        foreign.mkdir()
        (media / "只读 原文.txt").write_text("启动器验证保留原文。", encoding="utf-8")
        source_before = (media / "只读 原文.txt").read_bytes()
        occupied = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        occupied.bind(("127.0.0.1", 0))
        preferred = occupied.getsockname()[1]
        assert preferred <= 65515
        occupied.listen()
        log_path = root / "process-output.txt"
        with log_path.open("wb") as output:
            try:
                command = [str(binary), "--headless", "--no-browser", "--profile", str(profile), "--media-root", str(media), "--port", str(preferred)]
                process = subprocess.Popen(command, cwd=foreign, env=environment, stdout=output, stderr=output)
                processes.append(process)
                reply = wait_ready(binary, profile, process, environment, foreign)
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
                duplicate = execute(binary, ["--headless", "--no-browser", "--profile", str(profile)], environment, foreign)
                assert duplicate.returncode == 0 and process.poll() is None, duplicate.stderr
                assert (profile / "desktop.json").read_bytes() == config_before, "重复启动改写了已有配置"
                collision = execute(binary, ["--headless", "--no-browser", "--profile", str(root / "second-profile"), "--data-dir", config["dataDir"], "--media-root", str(media)], environment, foreign)
                assert collision.returncode != 0, "不同 profile 绕过同一 SQLite 数据目录锁"
                assert json.loads(request(opener, address, "/healthz")[1])["status"] == "ok"
                control(binary, profile, "quit", environment, foreign)
                assert process.wait(timeout=30) == 0
                restored = subprocess.Popen([str(binary), "--headless", "--no-browser", "--profile", str(profile)], cwd=foreign, env=environment, stdout=output, stderr=output)
                processes.append(restored)
                restored_reply = wait_ready(binary, profile, restored, environment, foreign)
                assert restored_reply["url"] == address
                assert json.loads((profile / "desktop.json").read_bytes())["instanceId"] == config["instanceId"]
                _, payload = request(opener, address, "/api/status")
                assert json.loads(payload)["setupRequired"] is False, "本机重启丢失已有账号"
                control(binary, profile, "quit", environment, foreign)
                assert restored.wait(timeout=30) == 0
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
                assert (media / "只读 原文.txt").read_bytes() == source_before
                print(json.dumps({"result": "passed", "platform": "windows", "resourceMode": "explicit-web-dir" if args.web_dir else "embedded", "checks": ["中文/空格/分号路径", "任意工作目录", "端口冲突实际避让", "健康与HTML就绪", "真实命名管道", "重复启动", "同数据目录跨profile拒绝", "有序退出", "本机重启配置与账号保留", "重复JSON键拒绝且不覆盖", "保存配置完整字段/null/空相对路径拒绝", "源文件保持"]}, ensure_ascii=False))
            except BaseException:
                output.flush()
                print(log_path.read_text(encoding="utf-8", errors="replace")[-12000:])
                raise
            finally:
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
    main()
