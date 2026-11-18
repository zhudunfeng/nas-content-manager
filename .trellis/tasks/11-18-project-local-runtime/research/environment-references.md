# Research: project-local tool cache and temporary-directory controls

- Query: 核对 Go、npm、Node、Python 与 Playwright/MCP 的官方缓存、临时目录环境变量，以及不能覆盖的边界。
- Scope: mixed — 仓库构建入口、已附带 Go 1.27.4 源码，以及官方文档。
- Date: 2026-11-18

## Findings

### Go: build, module and temporary files

- `GOCACHE` 是构建缓存目录，必须为绝对路径；可以覆盖系统默认的 `go-build` 缓存。
- `GOMODCACHE` 是下载模块目录，`GOPATH` 还关联工作空间及其他 Go 文件，`GOBIN` 决定安装命令的目标目录。单独改构建缓存不足以隔离依赖缓存。
- `GOTMPDIR` 覆盖 Go 命令和 testing 包写入临时源文件、包与可执行文件的位置。
- `GOENV=off` 禁用用户默认的 `go env -w` 配置文件。不要执行 `go env -w` 来改用户全局设置。副作用是用户先前保存在该文件中的 `GOPROXY`、`GOPRIVATE` 等设置不再生效；如果需要，应作为当前进程环境显式提供。
- 项目独立的空模块缓存可能重新下载依赖；不能把旧共享缓存仍存在理解为新配置没有生效。

官方来源：[Go command / Environment variables](https://go.dev/cmd/go/#hdr-Environment_variables)、[Build and test caching](https://go.dev/cmd/go/#hdr-Build_and_test_caching)。

### npm and Node

- npm 支持以 `npm_config_` 前缀设置配置；连字符配置键在环境变量中使用下划线。配置不区分大小写，但 npm 脚本内部优先采用其生成的小写形式，因此需要覆盖小写形式，不能仅设置大写而保留冲突的小写值。
- `cache` 控制 npm 缓存位置，`logs-dir` 控制日志位置；后者默认在缓存目录的 `_logs` 子目录。
- Node 22 官方文档支持 `NODE_COMPILE_CACHE=dir`（自 v22.1.0 起），用于当前 Node 实例的模块编译缓存；项目固定的 Node 22.14.0 在支持范围内。
- npm 命令行显式配置、任意脚本主动写入其他位置属于调用者另行指定的行为，不能由环境变量无条件禁止。

官方来源：[npm configuration](https://docs.npmjs.com/cli/v11/using-npm/config/)、[Node 22 CLI / NODE_COMPILE_CACHE](https://nodejs.org/docs/latest-v22.x/api/cli.html#node_compile_cachedir)。

### Python

- `tempfile.gettempdir()` 依次考虑 `TMPDIR`、`TEMP`、`TMP`，随后考虑平台默认目录和当前工作目录；选出的结果会缓存。
- 在同一 Python 进程内，仅在首次调用后修改环境变量，不会自动替换已缓存的 `tempfile.tempdir`。初始化器需要处理已经缓存旧目录的情况，并在采用新目录前验证其可写；否则可能回退系统目录。
- `-B` 或 `PYTHONDONTWRITEBYTECODE` 可禁止 import 写入 `.pyc`。对子进程设置环境的同时，需要处理当前进程的 `sys.dont_write_bytecode`；本地共享模块应在禁用字节码后导入。
- 官方也提供 `PYTHONPYCACHEPREFIX` 把字节码放入指定树，但本任务可直接禁用字节码，避免引入另一套缓存生成。

官方来源：[tempfile](https://docs.python.org/3/library/tempfile.html)、[Python command-line and environment](https://docs.python.org/3/using/cmdline.html)。

### Playwright and Playwright MCP

- Playwright 的 `PLAYWRIGHT_BROWSERS_PATH` 同时用于安装时下载位置和运行时查找位置。它**不改变 Google Chrome 或 Microsoft Edge 的安装目录**，因此不能宣称所有浏览器程序都会安装在项目内。
- `microsoft/playwright-mcp` 当前官方 README 明确列出 `PLAYWRIGHT_MCP_USER_DATA_DIR`（等价 `--user-data-dir`）和 `PLAYWRIGHT_MCP_OUTPUT_DIR`（等价 `--output-dir`）。这两个键是真实支持的配置，不是自定义无效变量。
- MCP 的 output-dir 控制自动命名输出（例如没有显式文件名的截图）。显式文件名按 workspace root 解析，不受该设置控制。
- 持久浏览器 profile 只能被一个浏览器实例占用。并发客户端要使用独立 profile 或 `--isolated`。
- 环境变量需要在启动对应进程前提供。已在 IDE/客户端中启动的 MCP 服务及浏览器不会继承后来由终端设置的环境；需修改其启动环境并重启后验证实际目录。
- 以上 MCP 环境变量核对的是 2026-11-18 官方 main README，未假定本机外部 MCP 服务的版本或配置；本任务不应擅自修改用户客户端全局配置或重启其既有浏览器。

官方来源：[Playwright MCP README](https://github.com/microsoft/playwright-mcp/blob/main/README.md)（直接读取官方 raw main 内容核对配置表）、[Playwright managing browser binaries](https://playwright.dev/docs/browsers#managing-browser-binaries)。

### Go telemetry: an explicit exception

- Go 官方 telemetry 文档将 `go env GOTELEMETRY` 和 `go env GOTELEMETRYDIR` 明确列为只读值，分别报告模式与实际配置/数据目录。设置同名进程环境变量不构成有效的目录重定向。
- 官方默认目录为 `os.UserConfigDir()/go/telemetry`；`local` 为默认模式，因此即使没有上传，也可能生成本机计数文件。
- `go telemetry off` 会持久修改用户遥测配置，不属于本任务获授权的仅项目配置；不执行。
- 本机附带 `.tools/go/VERSION` 为 `go1.27.4`。源码 `.tools/go/src/cmd/go/internal/envcmd/env.go:120–121` 读取真实 telemetry 状态，`:640–641` 把两键列为 non-settable。`.tools/go/src/cmd/vendor/golang.org/x/telemetry/internal/telemetry/dir.go:51–55` 通过 `os.UserConfigDir()` 初始化默认目录。
- 源码 `.tools/go/src/cmd/internal/telemetry/telemetry.go:34,46` 与 `counter/counter.go:24` 可见内部 `TEST_TELEMETRY_DIR`，这是 Go 自身测试覆盖接口，不是公开稳定配置；不要作为产品保证。除整体改写 APPDATA/HOME/XDG_CONFIG_HOME 等用户目录外，未发现受支持的仅进程路径重定向方式。
- 结论：不设置假 `GOTELEMETRY*` 映射，不依赖 `TEST_*`，不改用户根配置。文档必须把 Go 工具自身遥测和系统/第三方内部写入列为明确边界，不能承诺绝对零 C 盘写入。

官方来源：[Go telemetry](https://go.dev/doc/telemetry)、仓库已附带的 Go 1.27.4 源码（只读核对，没有执行 Go 命令）。

## Files found and code patterns

- `scripts/release.py:213`：参数数组运行子进程；应保留这种调用方式，让环境继承统一规则，避免 shell 拼接。
- `scripts/release.py:269–270`：发布入口依次调用 npm 安装和构建，因此环境初始化须先于这些命令。
- `scripts/release.py:287`：发布暂存已经显式放进仓库 `.runtime`，现有行为应保留。
- `scripts/verify_release.py:203`：没有显式 dir 的 `TemporaryDirectory` 依赖 Python 默认临时目录，需由共享初始化器覆盖。
- `scripts/verify_desktop.py:70`：含中文和空格前缀的默认临时目录，应确认仍在项目内并保持路径兼容。
- `scripts/dev.ps1:21`：直接运行 npm build，需在工具启动前引入项目环境。

行号是研究时读取的基线，实施插入 import 后可能移动；应以符号和相邻代码定位。

## Related specs

- `.trellis/spec/backend/cross-platform-contract.md`：发布入口编码、跨平台维护脚本与禁止修改用户环境的边界。
- `.trellis/tasks/11-18-project-local-runtime/prd.md`：统一目录配置、失败显式报告、不操作既有 C 盘缓存、不执行 Docker。

## Caveats / Not Found

- 官方页面读取经 PowerShell `Invoke-WebRequest` 直接取回内存；没有下载二进制或写入浏览器 profile，没有运行 Node/Go，没有改动 C 盘文件。
- web.run 官方页面调用在本会话没有返回内容，因此可核对原始资料为上列官方 HTTP 页面与附带工具链源码。
- 没有发现公开支持的 Go telemetry 仅进程目录设置；不把内部测试变量当作稳定功能。
- 这是配置能力与源码核对，不是文件系统写入追踪或本机 Playwright/MCP 行为验收；真实运行证据需由实现/检查阶段分别记录。
