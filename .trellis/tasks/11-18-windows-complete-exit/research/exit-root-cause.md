# Research: Windows 桌面退出与文件所有权

- Query: 点击退出后有哪些路径可能保留进程、媒体句柄、数据库或 IPC；区分 Win32 消息泵与共享核心关闭问题。
- Scope: internal；只读源码与仓库附带标准库，未启动/控制应用或用户进程。
- Date: 2026-11-18

## Findings

### 1. 原有退出链与资源所有权

| 位置（研究基线行号） | 行为/所有权 |
| --- | --- |
| `internal/desktop/shell_windows.go:372–373` | 托盘菜单命令 9 调用启动器 `c.cancel()`。 |
| `internal/desktop/shell_windows.go:191–233` | `runTray` 在主线程运行 GetMessage/Dispatch；独立 goroutine 等 `ctx.Done` 后向捕获的窗口 PostMessage WM_CLOSE。 |
| `internal/desktop/shell_windows.go:252–260` | WM_CLOSE 调 DestroyWindow；WM_DESTROY 调 PostQuitMessage。 |
| `internal/desktop/launcher.go:56–58, 187–201` | Main 提前 LockOSThread，托盘结束后再次 cancel，并等 controller.run 返回。 |
| `internal/desktop/launcher.go:287–291, 413–433` | controller 在取消后同步 stop；先 cancel 服务，再等 serviceDone，预算 30 秒；超时返回错误。 |
| `internal/app/server.go:87,109,135–146` | app 持有数据 lease 与数据库。关闭 defer cancel workers，Shutdown 最长 10 秒，超时 Close，等 Serve、等后台 wg，随后数据库与 lease 的 defer 才执行。 |
| `internal/desktop/launcher.go:94,135,184` | Main 的 defer 按后进先出关闭日志、IPC、实例 guard（其间还有 ctx cancel）。 |
| `internal/desktop/ipc_windows.go:120–191` | 本机管道使用可取消 overlapped IO；stopIPC cancel 并等待管道 goroutine done。 |
| `internal/app/instance_windows.go:15–31` | 数据锁以零共享打开文件；内核随句柄或进程退出释放，不 unlink。 |
| `internal/desktop/log.go:94–105` | rotatingWriter.Close 用互斥锁关闭当前日志句柄。 |

CodeGraph 查询了 runTray、RunWithOptions、stop 与 mediaCommand 及调用者；同名 stop 的结果跨 Python 与 Go，进一步用定向源码核对，未直接把图谱中的同名边当作精确调用关系。

### 2. 空闲 Win32 消息链：未发现静态必现死锁，必须真实复现

核对 `windowClass`、`nativeMessage` 的 Go 对齐与 Win64 对应字段，未见明显错误。Main 已 LockOSThread；窗口创建、消息泵、WndProc 中 DestroyWindow/PostQuitMessage 都由该 GUI 线程调用。GetMessage 的 `-1` 和 `0` 结果也分别处理。异步 watcher 捕获的是创建成功的局部 window 值。

仅凭源码不能把“所有空闲托盘退出都会挂住”当作已证实事实。PostMessage 返回值被忽略、GUI 没有明确的退出中状态，以及窗口先销毁而核心尚未完成，均是可改进之处，但没有运行证据时不足以独立解释本次空闲故障。主代理正在进行真实 Windows 菜单操作验证；研究代理未重复操作 UI。

特别注意两种入口语义：`web/src/App.vue:317–327`、设置页与侧栏的“退出账户/退出登录”只调用 logout，不退出本机服务；`docs/windows.md:21–22` 也规定关闭浏览器不停止服务。不能未经确认把网页登录退出变成远程可用的进程关闭接口。

### 3. 确定的媒体子进程回收缺口，可构造确定性复现

- `internal/app/media_tools.go:77–85` 以 `exec.CommandContext` 返回 Cmd；Windows `media_process_windows.go:14–16` 只设置隐藏窗口/CREATE_NO_WINDOW。
- 没有子进程树的所有权容器、关闭回收或非零 `WaitDelay`。CommandContext 默认 Cancel 只 Kill 直接 Cmd.Process。
- `internal/app/cache.go:300–306`、`internal/app/music.go:82–88` 将 ffprobe stdout 设为 limitedBuffer，os/exec 会创建复制 goroutine 与管道。音乐封面处理也同样接收 pipe stdout。
- worker 在 `command.Run()` 返回前不会执行 `wg.Done`；app 退出中的 `a.wg.Wait()` 没有额外预算（server.go:144）。

可执行复现建议（在专用 E 盘 fixture，不使用真实工具/媒体）：

1. 将 NAS_FFPROBE_PATH 指向自建原生 fixture EXE。fixture 以明确句柄继承方式启动孙进程，继承 stdout，并让孙进程零共享持有 fixture 文件后等待可控事件；记录准确 PID。
2. 向隔离应用注册一个视频/音乐样本，确认 probe 的父、孙进程都已启动且文件被占用。
3. 通过真实托盘退出或 IPC quit 取消应用。现有代码会终止直接 ffprobe，但孙进程仍持有管道写端和文件。父命令 Run 等不到 EOF，worker 无法结束，app.wg.Wait 阻塞。
4. 桌面 stop 的 30 秒预算届满后 Main 返回 1 并由 os.Exit 结束当前进程，但是孙进程仍可能存活，文件仍占用。这条路径能解释“托盘消失但有工具残留”和部分关闭卡顿；不声称它已解释所有空闲窗口问题。

最小可靠修复方向：在统一媒体执行入口设置有界 pipe 回收，并用仅覆盖本应用拥有媒体命令的 Windows 进程所有权机制取消/等待整个媒体树。仅加 WaitDelay 能防止父进程等 pipe，却不能终止仍占用文件的孙进程；仅 taskkill /IM ffmpeg.exe 会误伤独立实例，不能采用。不要把默认浏览器、另一独立运行模式纳入媒体进程树关闭范围。具体机制应验证进程启动/取消竞争及子进程继承边界。

### 4. HTTP 关闭并不等于 Handler 资源全部释放

- `server.go:127` 没有设置 BaseContext，HTTP 请求默认不从 workCtx 派生。
- app cancel 后，后台工作会取消，但活动 HTTP 请求不会立即因为应用取消而得到相同上下文通知。大文件流没有全局 WriteTimeout（正确的播放契约），故 Shutdown 可耗尽 10 秒预算。
- 超时后 `server.Close()` 关闭连接，代码只等待 `serveDone` 与后台 a.wg。`serveDone` 只证明 Serve 返回；没有请求执行计数/完成栅栏。
- `media.go:39–75` 的原文件 defer Close、backup 的临时文件 defer Close/Remove，以及其他 handler 的 Rows/文件 defer 均属于请求执行期。若请求仍在收尾，共享核心可能提前进入 db.Close / lease.Close，尤其是同进程 restart 时不能依赖 os.Exit 兜底。

修复方向：明确请求 BaseContext/取消传播、停止接收请求后等待在途 handler 的完成顺序。需要防止 WaitGroup.Add 与 Wait 的竞争；必须先有停止接纳的屏障/受控计数，不能仅在 Handler 入口裸 Add。退出不必等待播放器把超大视频全部读完，应主动中断本实例的连接并等待已拥有资源的 handler 收尾。

### 5. 推荐验证矩阵

- 空闲原生 GUI：真实托盘“退出栖库”，准确 PID 等待；验证 HTTP 端口重绑定、管道不可再连、实例互斥可重新获得。
- 活跃大文件流：客户端故意不继续读取，退出要在预算内完成；媒体独占打开与同目录重命名成功。
- 子进程与继承 stdout：上述 fake probe 父/孙进程均应退出，文件可零共享打开；保留另一独立 fixture 进程，证明未通杀。
- SQLite/log/EXE：退出后用 Windows 零共享文件句柄检查数据锁、数据库、日志；重命名/更新 EXE、重启同一 profile 验证账户及 instanceId 保留。
- 重复退出/启动中退出/重启：取消幂等，无旧 listener/worker/媒体树遗留。
- Docker 只做静态/受控替身的回归，不启动 Docker；退出启动器与明确停止容器的既有语义保留。
- Windows 专属改动通过 Linux amd64/arm64 交叉构建或 Windows 宿主可适用的 shared tests；交叉编译不声称 Linux 实机运行。

## External References / Primary Local Sources

- 仓库附带 Go 版本：`.tools/go/VERSION` 为 go1.27.4。
- `.tools/go/src/os/exec/exec.go:489–502`：CommandContext 默认 Cancel Kill 直接 Process，WaitDelay 未设置。
- 同文件 `:289–314`：WaitDelay 为零可一直等到孤儿后代关闭继承的管道；非零时会关闭相关管道并返回错误。
- `.tools/go/src/net/http/server.go:3217–3246`：Close 关闭 listeners/connections，但不等待所有 handler goroutine 完成。
- 同文件 `:3256–3273`：Shutdown 不打断活动连接，超时返回 context 错误，Serve 在 Shutdown 开始后即可返回。
- 同文件 `:3160–3166`：BaseContext 未设置时默认 context.Background。
- 本报告未访问外部网站，不引入未经核对的新版本依赖/第三方方案。

## Related Specs

- `.trellis/spec/backend/cross-platform-contract.md`：shared shutdown 先取消/收尾 workers/media/HTTP/DB，再释放 lease；IPC 固定动作与 Docker退出边界。
- `.trellis/spec/backend/runtime-contract.md`：媒体只读、没有全局短 WriteTimeout、临时测试源验证后清除、生命周期回归。
- `.trellis/spec/guides/cross-layer-thinking-guide.md`：窗口→控制器→共享核心→请求/工作/子进程的边界必须有真实集成证据。

## Caveats / Not Found

- 研究期间未写业务代码、未启动应用或浏览器、未跑 Docker、未碰 C 盘文件/用户实例；只在当前任务 research/ 写此报告。
- 进程树/HTTP问题属于源码可证实的完整退出缺口及可构造复现路径；研究代理尚未执行 fixture。主代理的真实空闲托盘观察若不同，应以运行记录修正根因，而不是把所有故障笼统归因于此。
- 新函数/结构插入后行号会移动，本报告标注研究时基线。
