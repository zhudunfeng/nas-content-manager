# 项目内构建环境实现记录

## 实现范围

- `scripts/project-env.json` 是唯一可编辑配置。`runtimeDirectory.value` 默认 `.runtime`，带中文 `description`；只接受 `.runtime` 或其子目录。
- `scripts/project_env.py` 提供无导入环境副作用的 `configure_environment() -> dict[str, str]`、已验证的默认回归目录、统一 `run_cli` 与通用外部命令入口。
- 配置重定向 Windows/POSIX 通用临时目录、Go 构建/模块/工作空间/工具安装/临时目录、npm 下载缓存/日志、Node 编译缓存、Python 字节码前缀/pip 缓存、Playwright 浏览器/MCP profile/MCP 输出目录及 XDG 缓存。
- `GOENV=off` 阻止沿用 `go env -w` 用户配置；不改 `APPDATA`、`HOME`、`XDG_CONFIG_HOME`，不伪设只读的 `GOTELEMETRYDIR`，不依赖 Go 内部测试钩子。
- 逐级核验文件类型/软链接/Windows 重解析点，所有目录写入探测通过后才修改当前进程环境；重置已缓存的 `tempfile.tempdir`，失败不回退系统临时目录。
- `project-env.ps1` 为 UTF-8 BOM，可在 PowerShell 5.1/7 点源导入当前进程环境，不改变调用 CWD、不修改用户/系统环境、不 `exit` 结束点源终端。
- `dev.ps1`、Windows/Linux release 包装及全部 Python 构建/验证 CLI 自动接入。Python 在本地模块导入前关闭字节码；普通库导入不做环境初始化。
- 默认 verify 样本目录及 release 构建锁/暂存目录按配置解析，不受从其他 CWD 调用的影响。用户显式输出路径保持原有含义。
- 未改动 `clean-session-temp.ps1` 的旧 C 盘清理白名单，未运行 Docker、未删除 C 盘任何文件。

## 已执行检查

- Python AST：`scripts/` 下 9 个 Python 脚本全部解析通过。
- `python -B scripts/verify_release.py --self-test`：真实执行通过 68 项既有边界回归。
- E 盘自有隔离 fixture 回归：61 项通过。覆盖配置类型/字段/重复 JSON/绝对与父目录路径/设备别名/危险目录，所有映射目录真实创建，污染环境覆盖，真实 Python 临时文件及旧 `tempfile` 缓存重置，junction 拒绝且外部哨兵保持，模拟不可写失败，shell 引号编码，原生子命令中文/空格/引号/元字符/空参数保真，退出码 37 保留，默认仓库 CWD，JSON 纯数据输出，未知命令中文错误。
- 8 个直接 Python CLI 在 `PYTHONUTF8=0`、`PYTHONIOENCODING=ascii` 初始环境下的 `--help` 均成功，stdout/stderr 严格 UTF-8 解码通过。
- Windows PowerShell 5.1 与 PowerShell 7 的实际 parser 和点源导入通过，导入后缓存路径属于项目且调用 CWD 不变。
- Git for Windows `sh -n scripts/release-linux.sh` 及从其他目录实际调用 `--help` 通过。此证据不等同 Linux 实机执行。

## 修复的实际问题

首轮 PowerShell 5.1 导入发现 `Get-Command python -CommandType Application` 可以同时返回真实 Python 与 WindowsApps alias，直接读取 `.Source` 形成多个路径拼接后的无效可执行文件名。现显式选取首条 Application 后，第二轮完整回归通过。

## 复核与验证边界

- 通用 CLI 的原生 exe 参数透传已验证；独立复核发现并修复 Windows `.cmd`/`.bat` 隐式 shell 参数拆分，现通过 npm/npx 原生 Node CLI 入口并拒绝其他批处理。Windows JSON 大小写重复键亦已修复。最终 49 项独立回归与 68 项发布边界回归均通过，详见 `quality-review.md`。
- 主代理统一执行 Go vet/test/build、npm typecheck/build、真实 Go/npm 配置查询，避免重复冷缓存构建。
- 测试仅删除本轮 E 盘自有 fixture，不触碰旧缓存；首轮失败留下的自有 fixture、第二轮成功 fixture 和本轮临时测试源均已删除，实际不存在已复查。
- Go 自身遥测/系统崩溃日志、工具安装器、已经启动的外部 IDE/MCP/Docker daemon 不属于环境初始化器可绝对控制的写入范围。
