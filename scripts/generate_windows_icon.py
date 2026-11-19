"""从已维护的多尺寸 ICO 再生 Windows 桌面资源；普通构建直接使用已提交的 .syso。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    # 在导入项目模块前禁止源码旁的字节码缓存。
    sys.dont_write_bytecode = True

import argparse
import os
from pathlib import Path
import subprocess
import tempfile

from project_env import configure_console, resolve_command, run_cli, runtime_directory


ROOT = Path(__file__).resolve().parents[1]  # 从脚本定位仓库根，不依赖调用目录。
ICON = ROOT / "assets/windows/qiku.ico"  # 与网页 favicon 风格一致的多尺寸 Windows 图标源。
RESOURCE = ROOT / "cmd/nas-desktop/resource_windows_amd64.syso"  # 仅链接到 Windows amd64 桌面入口。
RSRC = "github.com/akavel/rsrc@v0.10.2"  # 固定维护工具版本；不加入应用 go.mod，也不要求全局安装。


def generate(check: bool) -> None:
    """在项目临时目录再生资源，并按需核对或更新唯一的桌面资源文件。"""
    if not ICON.is_file():
        raise ValueError("缺少桌面图标源：assets/windows/qiku.ico")
    environment: dict[str, str] = dict(os.environ)
    # 资源编译器必须在宿主运行，不继承调用者的交叉编译目标。
    environment.pop("GOOS", None)
    environment.pop("GOARCH", None)
    environment["CGO_ENABLED"] = "0"
    with tempfile.TemporaryDirectory(prefix="qiku-icon-", dir=runtime_directory()) as temporary:
        generated: Path = Path(temporary) / RESOURCE.name
        command: list[str] = ["go", "run", RSRC, "-arch", "amd64", "-ico", str(ICON), "-o", str(generated)]
        print("正在生成 Windows 桌面图标资源，首次运行会下载固定版本维护工具。", flush=True)
        # 此资源对象只包含一个图标组，固定 ID 1；不得附加会改变自动分配 ID 的 manifest。
        subprocess.run(resolve_command(command), cwd=ROOT, env=environment, check=True)
        identical: bool = RESOURCE.is_file() and RESOURCE.read_bytes() == generated.read_bytes()
        if check:
            if not identical:
                raise ValueError("已提交桌面资源与 ICO 不一致，请重新运行 scripts/generate_windows_icon.py")
            print("Windows 桌面图标资源一致性检查通过。")
        elif identical:
            print("Windows 桌面图标资源已是最新，无需修改。")
        else:
            generated.replace(RESOURCE)
            print("Windows 桌面图标资源已生成：cmd/nas-desktop/resource_windows_amd64.syso")


def main() -> int:
    """解析中文维护参数，并保留资源生成或一致性检查的真实退出状态。"""
    configure_console()
    parser: argparse.ArgumentParser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="仅再生并核对已提交资源，不修改源码文件")
    args: argparse.Namespace = parser.parse_args()
    try:
        generate(args.check)
        return 0
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"Windows 桌面图标资源生成失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(run_cli(main))
