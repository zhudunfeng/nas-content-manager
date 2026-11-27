# 桌面插件自动准备实现与行为验证

日期：2026-11-27（Asia/Shanghai）。此记录仅覆盖桌面控制层实现与隔离行为测试；真实联网材料、私有 Python 加载、发布包和站点验收由主任务单独记录。

## 实际交付

- `internal/desktop/collection_initialization.go` 代替原有单 UAV 初始化器，复用一组批次取消、状态所有权、完成与清理屏障。
- `collectionPlugins` 固定顺序为 JMComic → UAV；普通失败继续另一插件，取消或 `ErrCleanup` 阻止后续准备。本轮全部结果返回后由 controller 至多执行一次受控 core/worker 载入。
- `internal/desktop/collection_managed.go` 分别选择有效随包清单、当前已准备路径和 `jmcomicbootstrap.Resolve` / `uavbootstrap.Resolve`；漫画清单和视频清单/工具覆盖分别处理，不共享解释器目录。
- 新 profile 的明确 `JMCOMIC_ENABLED=true` 与 UAV 一样可推导 managed；旧 profile 的 mode 优先级、外接与明确关闭语义不变。
- IPC 原 `collectionInitialization` 保留，增加 `collectionInitializations` 数组保存每个 kind 的独立状态；当前进度优先，批次结束后失败项优先展示。快照复制数组和原单对象。
- 托盘及能力原因按 kind 查找，准备 ready 不会修改 worker 的 `Capability.Available`。另一插件激活不会抹除失败原因或触发自动重试。
- 下载进度只在状态短锁内写入，旧批次无权发布；完整性和最终路径探针由各自 bootstrap 提供。实际离线探针继续使用原 Windows Job 进程所有权和退出屏障。
- 激活前检查当前 native/managed/媒体 ready、当前插件仍允许自动准备、worker 未退出、没有排队用户命令、没有待处理主服务退出结果；重检、停用、退出及服务意外退出都会先撤销状态所有权并等待批次清理。

## 修改范围

- `internal/desktop/collection_initialization.go`（新增，原 `uav_initialization.go` 泛化）
- `internal/desktop/collection_config.go`
- `internal/desktop/collection_controller.go`
- `internal/desktop/collection_managed.go`
- `internal/desktop/launcher.go`
- `internal/desktop/shell_windows.go`

保留了这些文件原有的未提交 UAV 和其他项目改动；没有修改 scripts、spec、外部安装或全局环境。

## 验证证据

所有本地 Go 命令均通过 `python -B scripts/project_env.py -- ...`。格式化使用仓库 `.tools/go/bin/gofmt.exe`。

临时文件 `internal/desktop/collection_initialization_task_tmp_test.go` 提供 11 组行为测试，最后一次执行：

```text
python -B scripts/project_env.py -- go test ./internal/desktop -run TestInitializationTask -count=1 -v
PASS
ok nas-content-manager/internal/desktop 0.420s
```

通过用例：

1. `TestInitializationTaskGates`：两个固定插件的 Windows x64、预期种类与显式解释器门禁。
2. `TestInitializationTaskEnvironment`：环境副本不变、双插件路径隔离、明确 Python 优先、严格 true/false。
3. `TestInitializationTaskStatusIsolation`：插件状态、真实 capability、深拷贝及停用状态。
4. `TestInitializationTaskOrdinaryFailureContinues`：JM 失败后 UAV 顺序继续、结果独立保留。
5. `TestInitializationTaskCleanupBlocksRemaining`：资源清理错误阻止后续准备和激活，重检不抹除清理失败。
6. `TestInitializationTaskCancellationSkipsRemaining`：取消后不执行下一插件。
7. `TestInitializationTaskStaleProgress`：旧批次无法改写新状态。
8. `TestInitializationTaskPreservesFailureAfterActivation`：另一插件激活不覆盖下载失败，也不隐式重试；真实能力恢复后才置 ready。
9. `TestInitializationTaskActivationIsSingleAndCommandGuarded`：整批至多一次激活、排队用户指令优先。
10. `TestInitializationTaskActivationRequiresCurrentOwnership`：当前功能门禁失效或主服务已意外退出时不激活。
11. `TestInitializationTaskStopWaitsForCleanup`：stop 等待 owned 清理、忽略迟到成功、后续插件取消。

`go vet ./internal/desktop` 通过；受影响已跟踪文件 `git diff --check` 通过（仅 LF/CRLF 提示）。按 AGENTS 约定已删除上述临时测试源码；删除后的 `go test` 仅计编译证据，不代替本次行为结果。

尝试 `go test -race` 返回 `-race requires cgo`；当前项目运行环境禁用 CGO，因此没有声称 race 通过。并发所有权行为由上述屏障与状态测试验证，但不构成 race detector 证据。

## 边界与后续

- 主任务需使用最终构建程序验证 JM 首次联网准备、离线复用、最终路径真实探针、双插件共同启用与独立故障，以及退出后的 DLL 文件释放。
- 首次依赖完成统一载入仍有短暂受控 core/worker 重启，活动媒体/下载连接可能中断；没有自动重发原业务任务或重复打开浏览器。
- 本实现不访问漫画站点，不证明站点连通、实际账号权限或真实内容下载，也不等于干净普通用户 Windows 机器验收。
