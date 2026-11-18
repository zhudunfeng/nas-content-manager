"""为本项目统一配置仓库内构建缓存与临时目录，不修改用户或系统环境。"""

from __future__ import annotations

import sys

if __name__ == "__main__":
    # 在导入其他模块前关闭字节码，直接执行脚本也不在解释器目录生成缓存。
    sys.dont_write_bytecode = True

import argparse
import json
import os
import secrets
import shlex
import shutil
import stat
import subprocess
import tempfile
from collections.abc import Callable
from pathlib import Path, PurePosixPath
from typing import Any


ROOT: Path = Path(__file__).resolve().parents[1]  # 以源码位置确定当前仓库，不依赖调用者工作目录。
CONFIG: Path = ROOT / "scripts/project-env.json"  # 唯一可编辑配置，参数含义写在 JSON 的中文说明中。
DIRECTORIES: dict[str, str] = {
    "TEMP": "tmp",  # Windows 和原生工具的通用临时目录。
    "TMP": "tmp",  # 兼容使用 TMP 的构建与脚本工具。
    "TMPDIR": "tmp",  # Linux/POSIX 工具及 Python 的通用临时目录。
    "GOTMPDIR": "tmp/go",  # Go 编译、链接和测试的临时工作目录。
    "GOCACHE": "go/build-cache",  # Go 构建与测试缓存，避免使用当前用户共享缓存。
    "GOPATH": "go",  # Go 工作空间，同时约束模块校验数据库与工具安装默认位置。
    "GOMODCACHE": "go/pkg/mod",  # Go 模块及按需工具链下载缓存。
    "GOBIN": "go/bin",  # 显式 go install 的项目工具输出目录。
    "npm_config_cache": "npm/cache",  # npm 下载缓存，保留用户已有 registry/认证配置。
    "npm_config_logs_dir": "npm/logs",  # npm 调试日志，不写用户目录。
    "NODE_COMPILE_CACHE": "node/compile-cache",  # 支持该选项的 Node 版本的模块编译缓存。
    "XDG_CACHE_HOME": "cache",  # 遵循 XDG 约定的第三方工具缓存。
    "PYTHONPYCACHEPREFIX": "python/pycache",  # 即使子命令显式开启字节码，也优先放入项目专用目录。
    "PIP_CACHE_DIR": "python/pip-cache",  # 显式 pip 命令的下载缓存；本脚本不安装依赖。
    "PLAYWRIGHT_BROWSERS_PATH": "playwright/browsers",  # Playwright 浏览器可执行文件缓存。
    "PLAYWRIGHT_MCP_USER_DATA_DIR": "playwright/profile",  # 新启动 MCP 浏览器的专用资料目录，仅供一个实例使用。
    "PLAYWRIGHT_MCP_OUTPUT_DIR": "playwright/output",  # 新启动 MCP 浏览器的自动命名截图等输出目录。
}


def configure_console() -> None:
    """在 CLI 日志输出前统一 UTF-8，兼容没有 reconfigure 的替代流。"""
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if callable(reconfigure):
            reconfigure(encoding="utf-8", errors="backslashreplace")


def unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    """拒绝重复配置字段，避免误编辑时悄悄覆盖路径。"""
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"构建环境配置包含重复字段：{key}")
        result[key] = value
    return result


def runtime_directory() -> Path:
    """读取有界配置，要求缓存根始终位于仓库的 .runtime 专用目录。"""
    if CONFIG.is_symlink() or not CONFIG.is_file() or CONFIG.stat().st_size > 16 * 1024:
        raise ValueError("构建环境配置必须是 scripts/project-env.json 普通小文件")
    with CONFIG.open("rb") as stream:
        raw: bytes = stream.read(16 * 1024 + 1)
    if len(raw) > 16 * 1024:
        raise ValueError("构建环境配置超过 16 KiB")
    config: Any = json.loads(raw.decode("utf-8-sig"), object_pairs_hook=unique_object)
    if not isinstance(config, dict) or set(config) != {"runtimeDirectory"}:
        raise ValueError("构建环境配置必须且只能包含 runtimeDirectory")
    entry: Any = config["runtimeDirectory"]
    if not isinstance(entry, dict) or set(entry) != {"value", "description"} or not isinstance(entry["description"], str) or not entry["description"].strip():
        raise ValueError("runtimeDirectory 必须包含 value 和中文用途说明 description")
    value: Any = entry["value"]
    if not isinstance(value, str) or not value or value != value.strip() or "\\" in value or any(ord(char) < 32 or char in ':<>"|?*' for char in value):
        raise ValueError("runtimeDirectory 必须使用 .runtime 开始的安全正斜杠相对路径")
    parts: list[str] = value.split("/")
    reserved: set[str] = {"CON", "PRN", "AUX", "NUL", *(f"COM{number}" for number in range(1, 10)), *(f"LPT{number}" for number in range(1, 10))}
    if parts[0] != ".runtime" or any(part in ("", ".", "..") or part[-1:] in (".", " ") or part.split(".", 1)[0].upper() in reserved for part in parts):
        raise ValueError("runtimeDirectory 只允许 .runtime 或其专用子目录，不能使用根目录、上级目录或设备别名")
    return ROOT.joinpath(*PurePosixPath(value).parts)


def check_directory(path: Path) -> None:
    """逐级核对目录和 Windows 重解析点，禁止缓存路径通过链接离开仓库。"""
    relative: Path = path.relative_to(ROOT)
    current: Path = ROOT
    # 在每一级创建前后核对链接，既拒绝已有 junction，也不穿过失效软链接。
    for part in ("", *relative.parts):
        if part:
            current = current / part
        try:
            metadata: os.stat_result = current.lstat()
        except FileNotFoundError:
            current.mkdir()
            metadata = current.lstat()
        attributes: int = getattr(metadata, "st_file_attributes", 0)
        if stat.S_ISLNK(metadata.st_mode) or attributes & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 1024) or not stat.S_ISDIR(metadata.st_mode):
            raise ValueError(f"构建目录不能是文件、符号链接或重解析点：{current}")
    if not path.resolve().is_relative_to(ROOT):
        raise ValueError("构建目录规范路径越出当前仓库")


def verify_writable(path: Path) -> None:
    """以专属小文件验证目录可写，失败时立即报错而不回退系统临时目录。"""
    check_directory(path)
    probe: Path = path / (".write-check-" + secrets.token_hex(12))
    descriptor: int = os.open(probe, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    try:
        os.write(descriptor, b"project-cache-check\n")
    finally:
        os.close(descriptor)
        probe.unlink()


def verification_directory() -> Path:
    """返回配置根下可写且不经过链接的回归样本目录，避免受调用工作目录影响。"""
    path: Path = runtime_directory() / "verify"
    verify_writable(path)
    return path


def configure_environment() -> dict[str, str]:
    """校验并应用当前进程的本项目环境，返回可供子进程继承的变量集合。"""
    runtime: Path = runtime_directory()
    settings: dict[str, str] = {key: str(runtime / value) for key, value in DIRECTORIES.items()}
    # 先完成所有目录验证再修改进程环境；外部继承缓存变量不能覆盖本项目规则。
    for directory in sorted(set(settings.values())):
        verify_writable(Path(directory))
    settings.update({
        "GOENV": "off",  # 不读取或写入当前用户的 go env -w 配置。
        "PYTHONDONTWRITEBYTECODE": "1",  # 当前命令及新启动 Python 默认不生成源码旁的字节码。
        "PYTHONUTF8": "1",  # 新启动 Python 的默认文本处理统一为 UTF-8。
        "PYTHONIOENCODING": "utf-8",  # 子进程重定向日志保留中文。
    })
    if os.name != "nt":
        # POSIX 大小写键可能并存；Windows 环境不区分大小写且 PowerShell JSON 拒绝此类重复键。
        settings.update({
            "NPM_CONFIG_CACHE": settings["npm_config_cache"],  # 覆盖上游遗留的大写 npm 缓存配置。
            "NPM_CONFIG_LOGS_DIR": settings["npm_config_logs_dir"],  # 覆盖上游遗留的大写 npm 日志配置。
        })
    os.environ.update(settings)
    sys.dont_write_bytecode = True
    # tempfile 可能已被其他模块调用并缓存旧目录，必须同步重置当前解释器的选择。
    tempfile.tempdir = settings["TMPDIR"]
    return settings


def run_cli(action: Callable[[], int | None]) -> int:
    """为现有维护脚本入口先初始化环境，再执行原命令并保留退出码。"""
    configure_console()
    try:
        settings: dict[str, str] = configure_environment()
        print(f"构建缓存与临时目录已限定在项目内：{Path(settings['TMPDIR']).parent}", file=sys.stderr, flush=True)
        result: int | None = action()
        return 0 if result is None else result
    except (OSError, ValueError) as error:
        print(f"项目构建环境初始化或执行失败：{error}", file=sys.stderr)
        return 1


def resolve_command(command: list[str]) -> list[str]:
    """解析项目命令并将 Windows npm 批处理转为原生 Node 调用，保留元字符参数。"""
    requested: str = command[0]
    # 显式相对可执行路径与子命令工作目录保持一致，不能相对调用者目录查找。
    if "/" in requested or "\\" in requested:
        candidate: Path = Path(requested)
        if not candidate.is_absolute():
            candidate = ROOT / candidate
        requested = str(candidate)
    executable: str | None = shutil.which(requested)
    if executable is None and command[0] == "go":
        local_go: Path = ROOT / ".tools/go/bin" / ("go.exe" if os.name == "nt" else "go")
        if local_go.is_file():
            executable = str(local_go)
    if executable is None:
        raise ValueError(f"未找到外部命令：{command[0]}")
    path: Path = Path(executable)
    if os.name == "nt" and path.suffix.casefold() in (".cmd", ".bat"):
        name: str = path.stem.casefold()
        if name not in ("npm", "npx"):
            raise ValueError("为保留参数并避免隐式命令解释，不直接执行 Windows .cmd/.bat；请使用对应原生 EXE 或明确选择自己的命令解释器")
        cli: Path = path.parent / "node_modules/npm/bin" / f"{name}-cli.js"
        adjacent_node: Path = path.parent / "node.exe"
        node: str | None = str(adjacent_node) if adjacent_node.is_file() else shutil.which("node.exe")
        if node is None or not cli.is_file():
            raise ValueError("未找到 npm/npx 对应的 node.exe 与 CLI 脚本，请检查 Node.js 安装")
        # Windows 即使 shell=False 也可能隐式解释批处理，直接执行 Node 才能原样传参。
        return [node, str(cli), *command[1:]]
    return [executable, *command[1:]]


def main() -> int:
    """显示环境配置或用原始参数数组执行项目命令，不经 shell 拼接。"""
    configure_console()
    parser: argparse.ArgumentParser = argparse.ArgumentParser(description=__doc__)
    output: argparse._MutuallyExclusiveGroup = parser.add_mutually_exclusive_group()
    output.add_argument("--print-env", action="store_true", help="仅输出已初始化的环境 JSON，供 PowerShell 当前终端导入")
    output.add_argument("--shell", choices=("sh",), help="输出经过安全引号编码的 POSIX shell export 命令，供当前终端导入")
    parser.add_argument("command", nargs=argparse.REMAINDER, help="-- 后的外部命令与原始参数，工作目录固定为当前仓库根")
    args: argparse.Namespace = parser.parse_args()
    command: list[str] = args.command[1:] if args.command[:1] == ["--"] else args.command
    if (args.print_env or args.shell) and command:
        parser.error("显示环境时不能同时执行子命令")
    try:
        settings: dict[str, str] = configure_environment()
        if args.print_env:
            print(json.dumps(settings, ensure_ascii=True, sort_keys=True))
            return 0
        if args.shell:
            print("\n".join(f"export {key}={shlex.quote(value)}" for key, value in sorted(settings.items())))
            return 0
        print(f"构建缓存与临时目录已限定在项目内：{Path(settings['TMPDIR']).parent}", file=sys.stderr, flush=True)
        if not command:
            parser.error("请提供 -- 后的外部命令，或使用 --print-env / --shell sh")
        return subprocess.run(resolve_command(command), cwd=ROOT, check=False).returncode
    except (OSError, ValueError) as error:
        print(f"项目构建环境初始化或执行失败：{error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
