# 项目构建缓存与临时目录

本配置将本项目能够控制的构建与验证文件留在当前仓库，默认目录为 `.runtime/`。「当前目录」指脚本所在的项目根，不取决于调用命令时的工作目录。

## 使用方法

现有 `scripts/dev.ps1`、Windows/Linux 发布入口，以及 `scripts/release.py`、`scripts/verify*.py`、`scripts/package_desktop_images.py` 直接运行时会自动加载，无需额外步骤。开发环境需要 Python 3.10+；发布成品的终端用户不需要 Python。

PowerShell 当前终端启用一次，后续启动的子进程继承：

```powershell
# 开头的点和空格表示在当前 PowerShell 中加载，兼容 Windows PowerShell 5.1 和 PowerShell 7。
. .\scripts\project-env.ps1

# Go 没在 PATH 时使用已有便携工具链。
.\.tools\go\bin\go.exe env GOCACHE GOMODCACHE GOTMPDIR
npm --prefix web run build
```

Windows/Linux 都可只为一条命令启用，不改变调用者环境：

```sh
# Linux 使用 python3，Windows 可替换为 python。
python3 -B scripts/project_env.py -- go vet ./...
python3 -B scripts/project_env.py -- npm --prefix web run build
python3 -B scripts/project_env.py -- python3 scripts/verify_release.py --self-test
```

Linux 当前终端启用：

```sh
# 先成功生成完整导出语句再应用；保留空格、中文等路径参数。
project_environment=$(python3 -B scripts/project_env.py --shell sh) && eval "$project_environment"
```

通用入口的子命令从项目根运行，参数逐项传递，返回子进程的退出码。Windows 下的 `npm`/`npx` 通过 Node 直接执行对应 CLI，避免 `.cmd` 隐式 shell 拆分参数；其他 `.cmd`/`.bat` 包装器会明确拒绝，可先在 PowerShell 当前终端加载环境后自行调用。`--print-env` 输出 JSON，可用于查看实际路径；中文状态日志不会夹入 JSON 标准输出。Linux 发布 Shell 入口直接调用会自动初始化的 Python 发布脚本，保留调用者相对路径参数的原有含义。

## 配置与目录

编辑 `scripts/project-env.json` 的 `runtimeDirectory.value`，默认 `.runtime`，也可使用 `.runtime/build` 等专用子目录；`description` 是中文用途说明。该路径必须为 `.runtime` 或其子目录，不能使用绝对路径、`..`、仓库根、软链接或 Windows junction。目录不可写或配置无效时立即失败，不会回退系统临时目录。

以下相对路径均相对于配置的目录：

| 环境参数 | 默认相对位置/值 | 中文说明 |
| --- | --- | --- |
| `TEMP`、`TMP`、`TMPDIR` | `tmp/` | 通用临时文件与子进程临时目录 |
| `GOTMPDIR` | `tmp/go/` | Go 编译中间工作目录 |
| `GOCACHE` | `go/build-cache/` | Go 构建与测试缓存 |
| `GOPATH` | `go/` | Go 项目独立工作空间，含依赖校验数据库缓存 |
| `GOMODCACHE` | `go/pkg/mod/` | Go 模块与自动工具链下载缓存 |
| `GOBIN` | `go/bin/` | 通过 Go 安装的工具输出 |
| `GOENV` | `off` | 禁用用户持久 Go 配置，避免沿用共享缓存路径 |
| `npm_config_cache` | `npm/cache/` | npm 下载缓存 |
| `npm_config_logs_dir` | `npm/logs/` | npm 调试日志 |
| `NODE_COMPILE_CACHE` | `node/compile-cache/` | Node 编译缓存 |
| `PYTHONDONTWRITEBYTECODE` | `1` | 不生成 Python 字节码缓存；当前解释器也同步禁用 |
| `PYTHONPYCACHEPREFIX` | `python/pycache/` | 子命令主动开启字节码时使用的项目缓存位置 |
| `PIP_CACHE_DIR` | `python/pip-cache/` | 显式执行 pip 时的下载缓存；初始化器不会安装依赖 |
| `PYTHONUTF8`、`PYTHONIOENCODING` | `1`、`utf-8` | 子进程 Python 使用 UTF-8 文本与日志编码 |
| `XDG_CACHE_HOME` | `cache/` | 遵循 XDG 规则的工具缓存 |
| `PLAYWRIGHT_BROWSERS_PATH` | `playwright/browsers/` | Playwright 下载的浏览器 |
| `PLAYWRIGHT_MCP_USER_DATA_DIR` | `playwright/profile/` | Playwright MCP 专用持久浏览器资料 |
| `PLAYWRIGHT_MCP_OUTPUT_DIR` | `playwright/output/` | Playwright MCP 自动命名的截图、轨迹等输出 |

`npm` 的配置环境键在 Windows 只输出一套小写名称，利用系统环境变量不区分大小写的规则覆盖旧值，避免 PowerShell 5.1/7 解析 JSON 时遇到重复键；POSIX 同时覆盖大小写形式。Python 已经缓存的 `tempfile` 目录会重置为本项目目录。现有 `bin/`、`web/dist/`、`internal/webassets/dist/` 等输出保持原有布局，仍位于仓库内；`.runtime/` 已被 Git 忽略。

回归脚本的默认 `verify/` 样本根与发布脚本的临时目录/构建锁也跟随配置，直接从其他工作目录调用脚本时不会把默认样本写到调用目录。显式 `--root`、`--output` 等参数继续保留原有含义。

`GOENV=off` 也意味着不读取先前由 `go env -w` 保存的 `GOPROXY`、`GOPRIVATE` 等设置。需要私有模块或下载代理时，应在当前终端显式设置对应环境变量，初始化器会保留这些与路径隔离无关的配置；不要把凭据写进已跟踪的 JSON 配置。

## 生效范围

- 配置作用于加载它的进程及其后续子进程，不写入系统/用户环境、注册表或 `go env -w`。新建终端要重新加载，或始终使用通用入口。
- IDE、Codex 和 MCP 浏览器服务若已运行，不会继承随后在另一个终端设置的环境。需先加载配置，再从这个终端启动对应服务；MCP 客户端也可在服务环境中显式填写 `--print-env` 列出的绝对路径。
- 同一持久浏览器资料目录不能由多个浏览器实例同时使用。并行独立服务应在各自启动环境中指定不同的项目内 profile；显式浏览器参数优先于环境默认值。
- Playwright MCP 的输出目录控制自动命名输出，显式文件名仍按其工作区解析，使用者需将截图等文件名指定在本仓库内。其他浏览器连接器是否使用这些配置取决于其自身实现。
- `PLAYWRIGHT_BROWSERS_PATH` 只控制 Playwright 管理的浏览器，不更改系统 Chrome/Edge 的安装路径。
- 显式传给工具的外部输出路径、第三方程序忽略环境配置后的写入、工具/操作系统安装器、Windows 日志与用户配置、Docker/WSL 镜像存储，以及发布成品的业务数据不在本配置保证范围内。
- Go 遥测使用工具自身的用户配置目录，`GOTELEMETRY`、`GOTELEMETRYDIR` 是查询结果而非可设置的环境参数；本功能不使用内部测试钩子、不更改全局遥测状态，也不改写 `APPDATA`、`HOME` 等用户目录来间接重定向。
- 项目中已有的媒体根、数据库位置、发布输出参数和桌面配置不作迁移；此功能只处理开发构建环境。

## 首次使用与旧缓存

项目独立缓存最初为空，首次 Go/npm 构建可能需要网络下载，现有共享缓存不会自动复制。环境设置不会迁移或删除旧 C 盘文件；维护旧缓存仍使用[会话缓存清理脚本](session-temp-cleanup.md)，先预览后由使用者执行。

不要在构建或测试浏览器运行期间手动删除 `.runtime/`。其中还可能保存既有验证样本、凭据和清理清单，不能仅凭目录名认定可以整目录删除。
