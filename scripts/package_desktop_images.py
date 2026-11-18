"""将维护者已构建的Linux应用/助手镜像封装成桌面离线资源，不连接远程Docker。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    # 直接执行入口时，在其他本地模块导入前关闭源码旁的字节码缓存。
    sys.dont_write_bytecode = True

import argparse
import json
import os
import re
import subprocess
import tempfile
from pathlib import Path
from typing import Any

from release import configure_console, digest


IMAGE_ID = re.compile(r"sha256:[0-9a-f]{64}\Z")  # 精确本地镜像身份，禁止用可变标签作为发布结果。


def invoke(arguments: list[str], environment: dict[str, str]) -> str:
    """以参数数组运行Docker，失败时仅给出命令阶段而不复制可能含凭据的输出。"""
    result = subprocess.run(["docker", *arguments], env=environment, capture_output=True, text=True, encoding="utf-8", errors="replace", check=False)
    if result.returncode:
        raise ValueError("Docker操作失败，请在本机检查Docker Desktop、镜像及磁盘空间")
    if len(result.stdout) > 2 * 1024 * 1024:
        raise ValueError("Docker元数据超过安全大小")
    return result.stdout


def local_context(name: str | None) -> tuple[str, dict[str, str]]:
    """只允许显式选择的本机context，清理可能将命令重定向到远端的环境。"""
    environment = {key: value for key, value in os.environ.items() if not key.upper().startswith("DOCKER_")}
    selected = name or invoke(["context", "show"], environment).strip()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,127}", selected):
        raise ValueError("Docker context名称无效")
    metadata = json.loads(invoke(["context", "inspect", selected], environment))
    if not isinstance(metadata, list) or len(metadata) != 1:
        raise ValueError("Docker context元数据无效")
    endpoint = metadata[0].get("Endpoints", {}).get("docker", {}).get("Host", "")
    if endpoint not in ("npipe:////./pipe/dockerDesktopLinuxEngine", "npipe:////./pipe/docker_engine", "unix:///var/run/docker.sock"):
        raise ValueError("仅允许本机Docker socket，不接受TCP、SSH或远端context")
    return selected, environment


def inspect_image(reference: str, selected: str, environment: dict[str, str]) -> dict[str, Any]:
    """将维护者输入解析为Linux镜像的不可变本地ID。"""
    if not reference or reference.startswith("-") or len(reference) > 512:
        raise ValueError("镜像引用无效")
    data = json.loads(invoke(["--context", selected, "image", "inspect", reference], environment))
    if not isinstance(data, list) or len(data) != 1 or not IMAGE_ID.fullmatch(str(data[0].get("Id", ""))):
        raise ValueError("无法取得精确镜像ID")
    if data[0].get("Os") != "linux" or data[0].get("Architecture") != "amd64":
        raise ValueError("首批Windows完整模式只接受Linux amd64镜像")
    return data[0]


def package(args: argparse.Namespace) -> Path:
    """保存两份固定镜像及归档摘要，不创建或删除容器、网络及数据卷。"""
    selected, environment = local_context(args.context)
    server = json.loads(invoke(["--context", selected, "version", "--format", "{{json .Server}}"], environment))
    if not isinstance(server, dict) or server.get("Os") != "linux":
        raise ValueError("需要运行中的本机Linux容器引擎")
    app = inspect_image(args.app_image, selected, environment)
    helper = inspect_image(args.helper_image, selected, environment)
    if app["Id"] == helper["Id"]:
        raise ValueError("应用和助手必须为不同镜像")
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    manifest = output / "release-images.json"
    archive = output / "images" / "desktop-linux-amd64.tar"
    if manifest.exists() or archive.exists():
        raise ValueError("目标已有镜像包，拒绝覆盖")
    archive.parent.mkdir(parents=True, exist_ok=True)
    # 临时归档与最终归档在同一文件系统，完成后原子替换，避免误发半成品。
    with tempfile.TemporaryDirectory(prefix=".qiku-images-", dir=output) as temporary:
        pending = Path(temporary) / "images.tar"
        print("正在保存本机固定镜像；不会启动容器", flush=True)
        invoke(["--context", selected, "image", "save", "--output", str(pending), app["Id"], helper["Id"]], environment)
        checksum = digest(pending)
        pending.replace(archive)
    value = {"schemaVersion": 1, "appImage": app["Id"], "helperImage": helper["Id"], "archive": "images/desktop-linux-amd64.tar", "archiveSha256": checksum}
    try:
        with manifest.open("x", encoding="utf-8", newline="\n") as stream:
            stream.write(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    except BaseException:
        archive.unlink(missing_ok=True)
        raise
    print("镜像资源已封装，归档使用分块SHA-256校验", flush=True)
    return manifest


def main() -> int:
    """解析有中文说明的镜像打包参数。"""
    configure_console()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", help="本机Docker context；默认读取当前选择并校验socket")
    parser.add_argument("--app-image", required=True, help="维护者已构建的Linux amd64应用镜像引用")
    parser.add_argument("--helper-image", required=True, help="维护者已构建的Linux amd64助手镜像引用")
    parser.add_argument("--output", required=True, help="资源输出目录，不覆盖已有归档或清单")
    try:
        print(f"镜像清单：{package(parser.parse_args())}")
        return 0
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"镜像打包失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    from project_env import run_cli

    raise SystemExit(run_cli(main))
