# Research: Windows desktop managed UAV 自动初始化接线

- Query: 如何把缺少文件时的 UAV 初始化接入启用、生命周期、私有 profile 和状态协议，同时保持媒体服务可用？
- Scope: internal
- Date: 2026-11-27

## Findings

### 检索与文件

- CodeGraph 经项目环境执行 node npm-shim.js query startManagedCollection，无结果；本题使用有界 rg 与目标文件阅读，不能声称图谱已覆盖。
- `internal/desktop/launcher.go`：控制器串行生命周期、私有 IPC DTO、启停主服务与 worker。
- `internal/desktop/collection_controller.go`：逐插件能力汇总、获取方式变更。
- `internal/desktop/collection_managed.go`：状态目录归属、固定子进程环境、worker 握手和停止。
- `internal/desktop/collection_config.go`：固定插件列表、新 profile 模式推导、外接优先。
- `internal/desktop/platform_windows.go`：当前用户/SYSTEM DACL、本地磁盘检查、Windows 原子配置替换。
- `internal/desktop/ipc_windows.go`：profile 单实例互斥量、当前用户命名管道、退出确认。
- `internal/desktop/shell_windows.go`：托盘固定启用/停用/重检入口和中文短状态。
- `internal/desktop/collection_process_windows.go`：暂停创建 → Job 归属 → 恢复、退出屏障。
- `internal/uavruntime/runtime.go` / `internal/privateruntime/files.go`：严格静态校验及可复用路径、文件、PE、别名校验。
- `internal/collectionworker/uav.go`：现有固定 probe、显式解释器优先、独立失败能力。
- `scripts/verify_desktop_collection.py`、`scripts/verify_desktop_env.py`：真实桌面/worker、私有运行时、readonly 安装目录、取消和 .env 优先级集成夹具。

### 当前行为与准确接点

1. `launcher.go:261` 的 `handleControl` 对 `collection-enable|disable|check` 只发固定 `desktopCommand{Kind:"config"}`；启用只改 `CollectionMode=managed`。`shell_windows.go:414` 一带托盘相同：启用/停用改模式，重检发 restart。
2. `launcher.go:300` 的 `run` 仅在一个 goroutine 中执行 start/stop，`commands` 容量 4；`launcher.go:323` 配置变更先完整 stop，然后 saveConfig，再 start。不可从初始化后台 goroutine 直接调用 start/stop 或改 `collection` 等裸字段。
3. `launcher.go:403` 的 `start` **先**同步 `startCollection(ctx,cfg)`（413），之后才 bindLoopback 和 app.RunWithOptions；把数百 MB 下载同步塞进 startManagedCollection 会阻塞媒体就绪，也让 run 暂时不能消费停用/重启命令。quit 通过全局 cancel 仍可打断具备正确 ctx 的下载。
4. `collection_controller.go:84` disabled/external 分支必须零安装；managed 分支当前 starting → worker → 独立能力汇总。`collectionReadiness`（26）已有任一插件可用即 ready 的语义；UAV 安装不能拖垮 JMComic。
5. `collection_managed.go:116` 环境只允许指定字段；`PATH` 固定 `programDir/tools`，UAV manifest 固定程序根，即便不存在也会传递。新初始化后必须有受信的路径选择 DTO/内部值，不能改成系统 PATH fallback，也不能透传任意 `.env` manifest/tool 路径。
6. 插件启用在 `collection_managed.go:132` 推导为 manifest 存在或显式 Python，再由严格 true/false 开关覆盖。**只有 managed 模式并不代表 UAV 被启用**。缺包时 generic collection-enable 且无 UAV=true 不会选择视频插件。建议保留视频显式 opt-in，本次场景以 NAS_COLLECTION_WORKER_UAV_ENABLED=true 为初始化授权；否则 generic 获取启用不应擅自启用视频。
7. `collection_config.go:52` 新 profile 仅在 worker+manifest 存在时默认 managed；现有 profile/saved disabled/external 不能因开关或升级静默重置。若产品要求仅设 UAV=true 即可用于全新 profile，可以只扩展该新建默认推导，并保留 explicit collection mode 优先级。
8. `collection_managed.go:40` 的 `prepareCollectionDirectory` 已约束 `<profile>/collection-managed`，拒绝链接/UNC、数据/媒体重叠、未知 owner；程序内部只允许 qiku-data。可在其验证的 owned root 下建立版本化依赖目录，与 data/secrets/tmp 分开；默认 profile 仍在程序旁，readonly 程序需显式 writable --profile。
9. `ipc_windows.go:46` 已有同 profile 单实例保护且整个进程持有。如果依赖放每 profile 的 collection-managed，主实例即安装互斥；不同 profile 不共用写目标。不要复用程序根共享安装目录。后台启动器重启/退出仍必须等待安装 goroutine 完成取消清理。
10. `uavruntime.Load`（runtime.go:100）只读并平台检查；manifest 根本身可以是 profile 内目录，只要求固定 basename 与 canonical absolute path，因此无需降低加载器校验即可使用 profile 版本化私有 bundle。manifest 内 runtimes/uav、licenses/uav 保持不变。
11. `uav.go:84` 的 initUAV 明确自定义 Python 优先且无回退；`CheckUAV`（168）可作为最终**离线**实际能力证明，但它使用 owned temp 并不能变成网络安装入口。初始化应留在桌面受信代码边界，普通 worker / --check-uav 仍只读探测。

### 可复用模式比较

**A. 同步预启动安装（不推荐）**：代码改动少，可直接在 startManagedCollection 之前准备；但下载期间主媒体与 run 控制循环未就绪，违反可选组件故障隔离目标。仅可用作极短的已有依赖静态检查，不宜承载网络/装配。

**B. 桌面异步 owned 初始化 + 串行激活（推荐）**：启动时尽快运行原有 worker（UAV 暂不可用，其他插件照常）及主媒体；owned 后台任务在 profile 专目录准备不可变 generation；中文阶段通过 IPC/status 和日志可见。完成后向 controller.run 发一个有 generation/attempt 身份的内部完成事件，run 验证仍需 UAV、未停用且属于当前生命周期后，使用既有 stop/start 路径加载选定 bundle；对新下载文件先静态+真实离线 probe 再发布选择。最小实现可短暂受控重启媒体来刷新主应用固定 worker 连接；需如实文档说明此行为，不能称完全无中断。

**C. worker 内动态安装/热更新（不推荐本次）**：绕过 launcher 先启媒体问题，但会使 worker 网络安装、副作用、状态协议、资源锁与能力快照同时复杂化，破坏 --check-uav/运行时加载器只读边界；不应直接引入 pip 或可变 shell 安装。

推荐 B 的最小状态设计：复用 `CollectionState` 既有枚举；给 UAV 独立初始化状态/阶段可用内部 DTO 或局限在 CollectionMessage，不要令所有插件 starting。需要详细 UI 时可增加可选 `collectionInitialization` DTO（状态、固定阶段、已下载字节/总字节；不含 URL/路径/秘密），但不强制新 API。托盘当前只显示短状态，若阶段仅放 message，需新增一条有界中文说明菜单才能真正可见。

### 生命周期、幂等、失败边界

- 已有明确有效随包依赖优先；否则查当前 owned profile 已发布 generation；都无效才准备。完整性失败要区分为缺失/损坏/不受信，不能覆盖未知 material 或就地修写程序文件。
- 任务 context 不能直接用仅绑定启动阶段的短 readyCtx（30s），也不能遗漏 stop：创建独立 withCancel，记录 done/attempt，run 的停用、配置切换、主退出路径都先 cancel 并等待 done，再处理文件选择/重启。
- 停用期间完成消息不得重新启用；重启期间旧消息不得覆盖新状态。使用单一 owned attempt / generation 身份，channel 有界，completion 不宜通过可能满的普通 commands 丢失。
- 初始化下载/解压不持有 c.mu。仅短状态更新持锁。所有路径返回都报告 cleanup 失败，不能 errors.Is(context.Canceled) 掩盖 joined 清理错误；复用 cancellationOnly 语义。
- 失败只标 UAV unavailable，其他插件能力保留。每次失败不得无限循环重试；托盘重检/下次进程启动明确触发新尝试。失败消息给出网络/空间/校验/权限类别，不输出凭据、原始远端URL或工具错误。
- 下载在临时/随机 generation 下；完整校验和真实 probe 后原子选择，最终目标不覆盖有效 generation。仅清理本次拥有的临时目录，重启识别未完成目录时不得按宽松路径递归删除。
- 不改变系统 PATH、不 os.Setenv、不使用 registry；worker PATH 选择安装 bundle 的 tools（若 media/ytdlp需要分根，受信绝对EXE路径传递即可）。自定义 Python 不合格时不可用 bundle 替换；可补工具与否需 PRD 明确，避免无谓下载 Python。
- 仓库没有可直接复用的原生依赖下载安装器。enginehelper 仅 Linux Docker pull/模板；HTTP client 的 context、bounded body、redirect 和错误脱敏可借鉴，archive/zip 已仅见读取媒体，不存在可直接复用安全执行文件解压器。

## 建议验证

1. 真实 Windows 新测试包只含桌面/worker，显式 managed+UAV=true、清理系统 PATH、无全局 Python，先证明媒体 ready，再证明初始化完成且 worker --check-uav exit0。
2. 第一次安装后离线重启，确认无新下载，bundle 文件 hash/mtime 不变；有效随包完整依赖直接 ready。
3. 网络错误/HTTP错误/截断/hash不匹配/Zip Slip/大小写重复/链接条目/超限/磁盘写失败不得发布 current 或替换有效版本。
4. 初始化期间 collection-disable/quit/主进程崩溃：无 orphan 下载工具；不重新启用；暂存归属可解释，重试可恢复；owned Job 与文件句柄清理遵循现有 harness。
5. 外接/disabled/false/非法布尔零下载；显式Python失效不回退，已有JMComic仍ready；其他profile与独立sentinel不受影响。
6. 只读程序目录+可写profile安装成功；未知owner、profile重叠、junction/UNC拒绝；同profile第二启动器只走IPC。
7. 更新原有 desktop env/collection harness 中的缺包预期，避免现在只验证旧通用报错；保留 .env snapshot、false/empty覆盖和immutable lookup 断言。

## Related specs

- `.trellis/spec/backend/uav-windows-contract.md`：要明确新增桌面初始化边界；只读 loader、--check-uav、固定来源和无系统污染仍保持。
- `.trellis/spec/backend/jmcomic-windows-contract.md`：共享worker、profile ownership、逐插件失败，JMComic不新增安装。
- `.trellis/spec/backend/cross-platform-contract.md`：主服务就绪/退出屏障、immutable dotenv、readonly program、受控环境、IPC。
- `.trellis/spec/backend/collection-contract.md`：worker私有身份/目录授权与错误隔离。
- `.trellis/spec/backend/engine-bootstrap-contract.md`：借鉴阶段/失败事实，但不可把Linux Docker安装器等同Windows原生安装器。
- `.trellis/spec/guides/code-reuse-thinking-guide.md`：先复用既有只读校验/文件辅助/串行生命周期，禁止复制独立解析器。

## External references

此主题只研究现有仓库接线，不引入外部技术事实；上游物料选择由单独 runtime-acquisition 研究负责。

## Caveats / Not Found

- 未发现可下载的本项目预制 runtime bundle 发布地址，不能靠新增空URL配置冒充自动初始化。
- 完成后真正无中断热切 worker 连接需要应用连接快照刷新机制的额外改造；最小接线只能明确短暂串行重启或扩大架构范围。
- 泛化“启用本机获取”与“显式启用 UAV”不是同一授权/配置条件，需要 PRD 锁定；建议本期以显式 UAV=true 为触发。
- 本研究没有执行构建、网络下载或桌面运行时验收；只读代码研究不能代替最终验证。
