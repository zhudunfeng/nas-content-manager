# 主代理集成验证

## 已通过

- 真实 `go env -json GOCACHE GOPATH GOMODCACHE GOTMPDIR GOBIN GOENV`：可设置的缓存、模块、工作空间、安装和中间文件目录均在仓库 `.runtime/`；`GOENV=off` 的查询结果为空是工具的关闭状态。
- 真实 `npm --prefix web config get cache` / `config get logs-dir`：分别返回项目 `.runtime/npm/cache`、`.runtime/npm/logs`。
- 真实 Python 子进程 `tempfile.mkdtemp()`：目录位于 `.runtime/tmp/`，子进程工作目录为项目根，字节码已禁用；该专有空目录随后移除。
- `npm --prefix web run build`：严格 vue-tsc 与 Vite 生产构建通过，149 模块；npm/npx 原生 Node 解析修复后再次执行通过。
- `python -B scripts/verify_release.py --self-test`：68 项通过；Windows、Linux amd64/arm64 归档结构回归为受控样本，并非 ARM 实机执行。
- `go vet ./...`、`go test ./...`、Windows CLI `go build` 通过。按用户要求当前无保留的单测文件，因此 Go 空套件只计包编译，环境行为覆盖见实现 61 项回归与独立复核报告。
- `scripts/` 下 9 个 Python 文件 AST 解析通过；Git Bash `sh -n` 与 Linux 发布入口 `--help` 真实调用通过，不将 MSYS 证据称为 Linux 实机。
- 真实生成 Windows amd64、Linux amd64、Linux arm64 开发归档，并逐一通过 `verify_release.py` 的架构、结构及内外摘要核验，每包 8 份文件。使用已通过构建的 `web/dist` 与 `--skip-web-build --development-without-tools`，未携带 FFmpeg，不是完整离线发行版。Linux 目标是 Windows 宿主交叉构建与归档检查，未在 Linux/ARM 实机执行本轮产物。
- CodeGraph 索引同步与 Trellis implement/check 上下文校验通过。

## 下载环境与边界

- 首次隔离模块缓存为空。默认官方 Go 代理的 `.mod` 请求出现可复现连接超时；未宣称首次官方代理全量构建成功。
- 原缓存 `cache/download` 当前不存在，只读 file proxy 方案在目录存在性检查时中止，未复制、删除或修改 C 盘缓存。
- 随后仅在验证子终端临时设置 `GOPROXY=https://goproxy.cn`，收到有效模块响应后重新执行 Go 检查成功。未更改项目默认代理、用户配置、Go 校验设置或 `go.sum`。
- 真实构建使用上述项目内缓存与输出；未对系统所有写入做全盘追踪，不包含工具自身遥测的零写入保证。未运行 Docker 或使用远程服务器凭据。

## 独立复核与收尾

- 独立回归 49 项通过，修复 Windows npm/npx 批处理参数拆分、PowerShell JSON 大小写重复键、Linux 包装器相对路径语义和默认媒体样本根链接检查；发布边界 68 项已在修复后重跑通过。
- 两版 PowerShell 严格无错误点源、真实 npm/npx 元字符、中文空格项目路径、junction 拒绝、POSIX 分支和 Git Bash 包装器参数均有针对性证据，详见同目录 `quality-review.md`。
- 本轮新增临时测试源及专属 fixture 已清除；发布锁、临时发布暂存与网页嵌入暂存未残留。已保留项目内依赖缓存和三目标开发归档，不整目录删除共享 `.runtime/`。
- README、发布说明、缓存维护说明、专门环境文档、AGENTS 与跨平台规格均已同步；任务进入 review，未提交、未归档。

本报告只将实际命令成功输出记为通过，不将交叉编译、空 Go 测试套件或 Shell 语法检查扩大解释为实机运行、行为覆盖或 Docker 部署验收。
