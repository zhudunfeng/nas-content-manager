# Research: JMComic 与 UAV 自动初始化最小复用方案

- Query: JMComic 如何在 Windows x64 托管启用时沿用 UAV 自动准备、可信还原、最终路径探针和生命周期策略？
- Scope: internal；只研究仓库当前实现和锁定契约，不更新依赖、不查询外部最新版。
- Date: 2026-11-27（Asia/Shanghai）

## Findings

### 1. 已有文件与权威边界

| 文件 | 用途 |
| --- | --- |
| `internal/uavbootstrap/bootstrap.go` | Windows 门禁、profile 内 owner、immutable generation、最终路径预算、离线 probe、原子 current 与精确清理。 |
| `internal/uavbootstrap/plan.go` | 三份内嵌资源、recipe 严格解析、最终文件/来源/目录闭包、ZIP 全库存安全校验。 |
| `internal/uavbootstrap/download.go` | 固定 HTTPS 来源、窄重定向、标准代理、有限超时和三次瞬态重试。 |
| `internal/uavbootstrap/materialize.go` | 独占写入、字节数+SHA 校验、逐文件 CRC、固定映射还原、取消与清理。 |
| `internal/desktop/uav_initialization.go` | worker 能力驱动的后台准备、进度所有权、Job 内 probe、串行 controller 激活与取消屏障。 |
| `internal/desktop/launcher.go` | 控制主循环以及启动、重检、停止、意外退出时的初始化联动。 |
| `internal/desktop/collection_config.go` | 新 profile 托管模式推导和固定插件描述。 |
| `internal/desktop/collection_managed.go` | child-only 环境快照、expected 能力、随包优先与 profile generation 选择。 |
| `internal/jmcomicruntime/runtime.go` | 独立漫画清单/依赖锁/架构/目录/文件/隔离配置验证；必须继续调用，不改弱。 |
| `scripts/jmcomic_runtime.py` | 固定 Python + 八个 wheel 的构建侧装配和真实离线图像/AES/CA 探测。 |
| `scripts/uav_bootstrap.py` | 从已验收运行时与原始固定材料生成可重现 recipe/runtime/payload。 |
| `scripts/runtime_common.py` | 已有 Python 普通文件、下载、归档提取底层能力。 |
| `scripts/verify_uav_bootstrap.py` | 最终 EXE/worker 的在线初装、失败、门禁、取消、离线复用验收模式。 |

相关 specs：`.trellis/spec/backend/uav-windows-contract.md` 第 5.1 节、`jmcomic-windows-contract.md` 第 3 节、`cross-platform-contract.md`、`jmcomic-contract.md`。现有 spec 中“JMComic behavior is unchanged / does not add automatic JMComic installation”属于上一任务范围描述，本任务落地后要同步修订。

### 2. 用户所称启用策略的精确现状

- 启用依据是 child environment 的 `expected` 能力，`NAS_COLLECTION_WORKER_*_ENABLED=false` 覆盖随包存在；任意不为 true/false 的输入报配置错误。参见 `internal/desktop/collection_managed.go:116-159`。
- UAV 自动准备只在 windows/amd64、expected 包含 uav、无显式 Python 时允许，见 `internal/desktop/uav_initialization.go:38-47`。默认模式推导目前只识别 UAV 的显式 true，见 `collection_config.go:52-74`，需要让 JMComic true 同等生效，同时保留旧 disabled/external 与外接 tuple 优先级。
- “有效随包优先 → profile Resolve → unavailable 才后台安装”分为两层：随包静态选择在 `collection_managed.go:184-195`；实际能力 unavailable 以及媒体 ready 才触发在 `uav_initialization.go:112-162` 和 `launcher.go:503-507`。
- 自定义 Python 配置继续由 worker 校验，无效时不静默 fallback：`internal/collectionworker/jmcomic.go:64-116`；`CheckJMComic` 为固定离线入口，见 `jmcomic.go:135-159`。
- 每轮自动尝试一次，重检/下次启动重试；后台只回报进度和结果，不直接变更 core/worker 生命周期。`uav_initialization.go:165-245`、`launcher.go:340-406`。

### 3. 安全引擎复用建议

**推荐：抽取 `internal/runtimebootstrap` 低层引擎，保留插件薄封装和独立 assets。** 不复制约千行安全实现；不要让漫画依赖 `uavbootstrap` 这一业务包，也不要用替换模块全局变量的方式共用 Python 构建器。

建议引擎配置由插件编译期构造，字段带中文注释：

```go
// Config 仅由两个固定插件封装构造，不接收网页或环境中的任意安装配方。
type Config struct {
    Namespace string // 固定 uav-bootstrap 或 jmcomic-bootstrap。
    ManifestName string // 固定插件清单名。
    Label string // 固定中文视频/漫画，用于阶段及脱敏错误。
    Recipe []byte // 编译时精确配方字节。
    Runtime []byte // 编译时精确运行时清单字节。
    Payload []byte // 编译时小型已安装文件载荷。
    RequiredFiles []string // UAV 的三工具；漫画不要求视频工具。
    ValidateManifest func(string) error // 分别调用 unchanged uavruntime.Load/jmcomicruntime.Load。
}
```

具体类型可调整；关键是每个引擎实例有自己的 `sync.Once`/plan/错误缓存，不能继续使用包级唯一 `planState` 让两种插件共享一个配方。可用 `New(Config)` + `Engine.Resolve/Initialize`；`Paths` 可统一只暴露代次根/清单，UAV wrapper 从固定相对路径补出工具字段，或保留通用文件表但不得接纳动态路径。

必须原样保持：

- UAV `owner.json` 精确字节、`uav-bootstrap/current.json` schema、recipe identity、generation 名字不变，兼容之前已发布 profile 依赖。见 `bootstrap.go:40-56,189-273`。
- 元数据严格字段/大小写/重复键/null 和全库存闭包，见 `plan.go:103-187,197-218`；`validateGeneration` 还要调用各自原始 manifest loader，见 `bootstrap.go:276-328`。
- 路径 UTF-16 最大 248 单元，随机 32hex 名字，先 stage 完整验证，再 rename final，再 final 验证和真实 probe，最后 current 原子替换，见 `bootstrap.go:83-178,332-343`。JMComic 含 curl-cffi/Pillow/Crypto 原生库，不能省掉此限制。
- 下载的精确来源/SHA/size 和有限重试、ZIP 目录全索引与别名拒绝、只提取固定 member、写入 CRC/SHA/长度、owner 和自有本次清理，见 `download.go:20-160`、`plan.go:228-272`、`materialize.go:34-263`。
- `ErrCleanup` 统一 sentinel 供两个插件通过 `errors.Is` 检查；可用插件 wrapper alias，不能普通网络失败化或重启掩盖。

**可行备选：** 只把下载/ZIP/写入等底层 helpers 导出到共享包，JMComic 保留自己的 owner/current orchestration。这改动 UAV 面积更小，但会复制最容易发生最终路径/发布/清理问题的流程，不建议作为长期方案。**不推荐：** 完整复制 uavbootstrap 并字符串替换；会让两插件的完整性和生命周期修复漂移。

### 4. 构建资源生成

- JMComic 的固定来源已经全部在 `jmcomic-runtime.json.sources`：Python + `jmcomic,curl-cffi,pillow,pycryptodome,pyyaml,cffi,certifi,pycparser`；依赖锁权威是 `internal/jmcomicruntime/contract.json`，现有版本为 Python 3.14.4、JMComic 2.7.12。无须新增 dependency、pip、FFmpeg 或 yt-dlp。
- `scripts/jmcomic_bootstrap.py` 应先 `jmcomic_runtime.verify(manifest, probe=True)`，下载或复用所有原始固定材料，对照已验证 manifest 库存建立 ZIP member 映射。无法原字节映射的 `._pth` 及归集 license 进入确定性 payload；映射必须精确覆盖每个最终文件一次。
- `scripts/uav_bootstrap.py:38-126` 的 fetch、ZIP 索引、payload 等可提取 `scripts/bootstrap_common.py` 共用；wheel relocation 的 contract 必须显式参数化而非更改 uav_runtime globals（当前 `relocated` 在 `:69-85` 绑定 UAV contract）。JM 构建器的提取规则由 `runtime_common.py:87-131` 决定，应逐字节比对而非假定与 UAV 完全相同。
- `.runtime/tools/jmcomic-private/jmcomic-runtime.json` 已存在（163124 字节）；本轮读取确认运行和许可目录存在，尚未执行真实 verify，因此只是候选输入，不能记为通过证据。构建缓存原目录为 `.runtime/tools/jmcomic-inputs`（`jmcomic_runtime.py:267`），实际当前未列出该目录，生成可能需要联网；允许复用 UAV 相同 SHA 原始归档，但不能复制混合 installed venv。
- `.gitattributes` 目前仅覆盖 UAV assets；新增漫画 assets JSON 也必须固定 LF、ZIP binary，保证编译期 identity 不被 CRLF 改写。
- 生成后 `--check` 重建逐字节比对，且 release metadata 继续真实：开发基础包自动在线准备功能不代表它已携带离线完整漫画 runtime。

### 5. 双插件共享 controller 的关键竞态

当前 controller 只有一套 `uavInitialization/uavInitializationDone/uavStatusTask/uavAttempted/uavPaths/uavActivating/uavCleanupErr`（`launcher.go:61-67`），`collectionInitializationReply` 和托盘也硬编码 uav（`uav_initialization.go:87-109`、`collection_controller.go:61-70`）。直接加第二套 goroutine、每种一成功就重启会令 `stop()` 取消另一个下载，并有状态覆盖/迟到完成/重复激活风险。

**最小稳健方式：一个固定插件队列，串行准备缺失插件，两个独立路径/attempted/status；本轮队列结束后统一一次激活。**

1. 媒体 ready 后，基于本次 worker 的真实 expected/capabilities 构造有序候选（固定 JMComic/UAV 顺序皆可）。已可用或显式 Python 的跳过。
2. 一次最多一个后台 task，因此保留一个 done 通道、进度所有权和 Job probe 模板；task 需绑定 kind。每插件本轮至多尝试一次。
3. 一个普通失败保留失败事实并继续处理另一插件；一个成功保存该插件 paths，然后处理下一个；清理失败阻止继续及所有重启。
4. 队列耗尽且至少一个成功，controller 串行 stop/start 一次；不清除 attempted/paths，不 reopen browser；新 worker actual capability 为真才显示就绪。
5. 重检/disable/quit/core 或 worker 意外退出先撤销 task status owner、cancel、等待 done/清理，再操作 worker；排队用户命令优先，迟到成功不能反转 disabled/external。现有 `completeUAVInitialization` 的 `len(c.commands)>0`、服务/worker ready 和 done 检查都应保留。
6. `collectionInitialization` 当前单对象可继续表达“当前正在准备的插件”（kind 可 jmcomic/uav），但必须给另一个已失败插件保留真实 capability reason，或添加向后兼容的每-kind状态集合；不能把当前视频阶段覆盖漫画 reason。`collectionPluginState` 必须比较 initialization.Kind。
7. 子进程环境只覆盖当前插件 `*_RUNTIME_MANIFEST`。JMComic 无须改 PATH；UAV 工具路径不可被漫画覆盖。两个 paths 最终共同应用到一次 worker 环境，保留显式覆盖。

### 6. 最小验证矩阵

**安全引擎/临时行为测试（完成后删除新增测试源码）：** 两插件 recipe 闭包、字节 identity、非法 URL/重定向、短/长/损坏/CRC/ZIP别名、unknown owner、248路径上限、取消、final probe 失败与旧 current 保留、ErrCleanup 阻止重启、Resolve 不联网；尤其验证共享引擎没有配方缓存交叉污染，UAV 原有 assets/current 仍可用。

**可复用最终包集成：** 参考 `scripts/verify_uav_bootstrap.py:79-237`，新建漫画或参数化 helper，但保留 UAV 媒体编码回归。真实 packaged EXE/worker、只读程序目录、中文/空格 profile、foreign CWD、PATH 只含 System32、污染 Python 环境。false/disabled/external/错误显式 Python 零下载；真实固定来源初装后 IPC 与认证 API 的 JMComic capability available；离线用私有 Python 验证 Pillow PNG encode/decode、AES、CA、版本；错误代理重启时 current、所有 SHA 和 mtime 完全不变；下载中 disable/quit 不留 stage/gen，不影响独立 sentinel。

**双插件：** 均缺失同时 true；漫画成功/视频失败；漫画失败/视频成功；一种随包有效另一自动准备；两种均已有 profile；重检不重复下载且不无限重试；队列第二插件过程中退出；成功激活后 actual capabilities 分别可信；全部 owned EXE/DLL/PYD 退出后可独占打开。保留 `scripts/verify_desktop_collection.py` 已有 25项生命周期矩阵。

**静态/构建：** gofmt、go vet、go test（去临时单测后只算编译）、Windows core/worker/desktop、Linux amd64/arm64 交叉构建；Python AST；生成资源 --check；release 131边界自测/旧新 schema，文档/env/spec 同步。没有前端源码改动则无需冒充新前端 build。

## External references

- 无外部在线研究。本任务以现有 manifest 中固定 `www.python.org` / `files.pythonhosted.org` 来源与 SHA 为准，不更新版本。
- 来源线程 `01a21b3d-49cd-7132-8dc5-4bb3e096698d` 的行为已由主代理读取；本研究不单独访问聊天记录。

## Caveats / Not Found

- 未发现可用 codegraph MCP；已使用 project-env 下 rg。一次 PowerShell rg 的 `internal/uavbootstrap/*.go` 通配符未展开，随后改目录扫描取得完整定位；不是源码缺失。
- 本研究没有运行构建/下载/探针，因此上述候选 runtime 与方案不是验收证据。
- 干净 Windows 普通用户、真实漫画站点、Linux/Docker/ARM 执行、许可/对应源码人工审查仍是独立边界；本机离线探针/交叉编译不能替代。
- 只写本任务 research 文件，不修改代码、spec、其他 task 或 Git。
