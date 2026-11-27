# Windows 完整退出：独立质量复核

日期：2026-11-18。任务：`11-18-windows-complete-exit`。

## 当前范围与状态

已完成验证脚本、Windows 用法、生命周期规格和最终生产 Go 的独立源码复核。应用最终重建集成和质量门禁结果由主代理/实施代理执行，本报告核对共享日志后记录，避免重复构建。以下区分独立 fixture、真实应用执行和尚待收集的证据。

## Findings (fixed)

1. `scripts/verify_desktop.py:assert_released` 原先先执行负向 `--control status`。实例不存在时该命令等待约五秒，文件检查和“立即重启”因此延后，会掩盖退出后的短暂句柄残留。现按顺序先做真实 Windows 零共享打开和原位重命名、检查 HTTP；首次退出跳过耗时负向 IPC，紧接真实同 profile 重启验证命名管道与单实例资源可重新取得；最终退出才执行负向 IPC。
2. 原先负向 IPC 只要求非零退出码，可能把「管道仍存活但响应失败」误报为管道不存在。现同时要求 stdout 为空且错误明确为「未找到可响应的已有实例」。进程超时、协议损坏等其他错误不算释放成功。
3. 原先仅核对原文 TXT，未检查用于慢流的 5 GiB 源是否被退出流程截断/改写。新增 `assert_sparse_source(path: Path, length: int) -> None`，检查文件长度与起始、4 GiB 之后和尾部三个 64 字节标记；每次退出后执行。
4. `docs/windows.md` 将退出中断的下载明确为浏览器媒体 HTTP 连接，避免将原生应用退出与外部 BT 引擎停止混淆。
5. 根据用户澄清，将程序构建产物作为首要验收：`assert_released` 从无序集合改为 EXE 首位的固定列表。在 `process.wait` 后立即对 EXE 零共享读写、重命名/恢复、从原构建产物原位 `shutil.copyfile` 覆盖、核对启动前保存的 SHA-256，再重命名/恢复完整程序目录。首次检查后立即以覆盖后的 EXE 重启同 profile，之前不做负向 IPC 等待。Windows 用法和人工验收同步聚焦 EXE/程序目录替换。
6. `internal/desktop/launcher.go` 原有 `errors.Is(err, context.Canceled)` 会吞掉新的 `errors.Join(context.Canceled, cleanupErr)` 中真实清理错误；多个 `_ = c.stop()` 和取消中的 `serviceDone` 分支也会丢弃清理结果。独立复核确认并交实施代理在 Go 所有权内修复：`cancellationOnly` 递归确认所有错误叶子均为正常取消，`start`、`run`、`stop` 保留真实停止错误；启动收到 nil 结果且上下文已取消时按正常取消处理。最终源码已复核。
7. `internal/app/media_process_windows.go:stopMediaJob` 单次进程快照与 `TerminateJobObject` 之间原存在新生后代遗漏窗口。主代理提出复核后，独立审查与实施代理共同确认，在捕获等待句柄前对自有 Job 设置 `JOB_OBJECT_LIMIT_ACTIVE_PROCESS=0`，同时保留 `KILL_ON_JOB_CLOSE`，关闭新增进程准入。封闭失败仍继续终止并合并错误，不会假报完整释放；随后等待捕获句柄和 Job 归零。实施代理补充并发派生 fixture，最终源码已复核。

## 首阶段 Verification

- Python AST、10 个顶层函数的中文方法说明及全部参数/返回注解检查通过；`git diff --check -- scripts/verify_desktop.py docs/windows.md` 通过（仅仓库既有 LF→CRLF 提示）。
- 独立脚本 fixture **14 项通过**：真实 Windows 零共享可打开/被占用/解锁恢复、占用不能误报释放；5 GiB 稀疏源长度与三个标记、逐个篡改拒绝、截断拒绝；文件检查先于负向 IPC、即时重启路径不执行负向 IPC、其他 IPC 错误拒绝、HTTP 仍监听拒绝、文件原位重命名后名称和内容保留。
- fixture 通过标准输入执行，没有新增保留测试源。只在项目 `.runtime/tmp/` 创建自身样本，TemporaryDirectory 正常清理；未启动应用、浏览器或 Docker，未编译实施代理的中间 Go 版本，未触及用户媒体和 C 盘。
- 本阶段没有最终应用退出验收结论；主代理记录的 52 项媒体接口通过并不能代替其中失败的退出流程。

## 程序文件验收补充 Verification

- 补充后 AST 与 **11** 个顶层方法中文说明、全部入参/返回类型检查通过；`git diff --check -- scripts/verify_desktop.py docs/windows.md` 通过。
- 更新版独立 Windows fixture **24 项通过**，包括 EXE 首项检查、真实同路径覆盖、SHA-256 一致性、完整程序目录实际重命名/恢复、包内普通附加文件保留、附加资源固定检查顺序、无 `.exit-check` 残留、负向 IPC 严格判定、首次退出跳过负向 IPC、仍监听 HTTP 拒绝/探针回收、真实 Windows EXE 共享冲突及解锁恢复，以及 5 GiB 源三标记/篡改/截断检测。
- 所有 fixture 通过 stdin 执行，未保留新增单元测试源，样本目录 `.runtime/tmp/exit-review-program-*` 已正常删除；未启动应用或编译中间 Go 版本。
- 普通包内文本 fixture 仅证明目录整体重命名保留内容，**不代表真实媒体工具或共享 DLL 的加载/卸载验证**。最终应用进程和自有工具映像释放结果由后续真实集成另行记录；未物理点击系统托盘。

## 最终生产 Go 源码复核

- `request_lifecycle.go` 的接纳锁覆盖停止标记和 `WaitGroup.Add`，关闭时先停止准入、取消 BaseContext、关闭连接，再等待 handler 和工作协程；不会用 `Serve` 返回代替文件 defer 完成。
- Windows 工具在挂起状态启动，分配自有 Job 后才恢复主线程；启动失败、取消和正常父进程结束均有所有权内回收，不按 EXE 名通杀，也不将用户浏览器归入 Job。正常结束的异常后代、继承管道和文件通过统一命令入口收尾，清理错误向应用退出传播。
- 命名管道 `OVERLAPPED` 和读写底层缓冲区在系统调用前用 `runtime.Pinner` 固定；立即/异步完成均取得 `GetOverlappedResult`，取消先 `CancelIoEx` 再阻塞收集完成后才 Unpin/关闭事件；连接的 `ERROR_PIPE_CONNECTED` 分支不错误等待未发起的异步操作。
- `cancellationOnly`、启动取消、`stop` 与 `serviceDone` 的错误传播按上述修正复核通过。未发现进一步需修改的生产问题。
- 独立执行最终八个 Go 文件 `gofmt -l` 无输出，完整 `git diff --check` 通过（仅仓库 LF→CRLF 提示）。已核对最终行为测试日志和临时测试源清除，详细结果如下。

## Findings (not fixed)

当前未发现未修复的生产缺陷。物理鼠标点击系统托盘、真实第三方共享 DLL 加载/卸载和 Docker 部署未执行，属于明确验收范围，不能从内部托盘菜单派发、工具 fixture 或普通附加文本推断。

## Verification

- Lint：**通过**。独立 gofmt、diff 检查通过；实施代理 `go vet ./...` 通过，日志 `.runtime/windows-exit/go-vet.log`。
- TypeCheck/Build：**通过**。Windows CLI/桌面构建、Linux amd64/arm64 CLI 交叉构建通过，分别记录在 `.runtime/windows-exit/go-build.log`、`go-build-linux-amd64.log`、`go-build-linux-arm64.log`；不将 Linux 交叉编译描述为实机执行。本任务无前端逻辑改动，没有重复前端构建。
- Tests：**通过**。独立 24 项 Windows 文件 fixture 通过；实施代理最终 `go test ./... -p 1 -count=1 -timeout=180s -v` 通过，`internal/app` **13.670 秒**、`internal/desktop` **2.934 秒**，已读取 `.runtime/windows-exit/go-test.log`。覆盖接纳/关闭屏障竞争、并发派生后代、父进程仍存活/已退出、独立进程保留、准确 PID/工具映像释放、启动取消、聚合清理错误、重复与独立短命进程 IPC、未完成连接取消及真实托盘消息泵/产品菜单派发。
- 按项目要求，新增的 `internal/app/request_lifecycle_temp_test.go`、`internal/app/windows_exit_temp_test.go`、`internal/desktop/windows_exit_temp_test.go` 已删除，独立目录枚举确认无保留新增测试源。删后 `go test ./...` 和 `go vet ./...` 通过，记录在 `go-test-after-cleanup.log`、`go-vet-after-cleanup.log`；该空测试套件只作为最终编译证据，不替代此前真实行为测试。
- Race detector：**未完成**。已核对 `go-race.log` 和 `go-race-cgo.log`：默认不启用 cgo，显式启用后缺少 `gcc`。未将此环境限制声称为 race 检查通过，也未自动安装编译器。
- Job 最后收紧后，主代理已重建 GUI 子系统 EXE 并重新执行完整 `verify_desktop.py`：慢流退出 **0.775 秒**、空闲 **0.336 秒**，包含 EXE 首项独占、原位覆盖、SHA-256、完整程序目录重命名/恢复及覆盖后同 profile 重启。最终日志 `.runtime/windows-exit/final-desktop.log`，资源模式为显式 `web/dist`，不将其描述为完整离线包或内嵌资源验证。
- 最终 GUI EXE 的五媒体集成 **52 项**通过，命名管道 `quit` 有序退出码为 0、耗时 **0.274 秒**；主 EXE、DB、锁、日志与全部源文件 **25 项**独占检查通过。已核对 `.runtime/windows-exit/final-media-result.json` 和 `final-media.log`；实际 FFmpeg 的应用记录由主代理保存，不能将稀疏协议文件称为可解码性能样本。
- 首次全量临时 Go 测试曾出现 fixture 就绪预算/读取空 PID 失败，已反馈实施代理修复同目录 `.pending` 写完再重命名发布 PID、限制 fixture Go 调度并发与放宽启动就绪预算；退出耗时断言未放宽。首次失败保留在 `go-test-first-fixture-failure.log`，最终质量门禁采用修正后全套重跑成功结果。
