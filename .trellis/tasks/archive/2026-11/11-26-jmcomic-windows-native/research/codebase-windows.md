# 当前 Windows 原生接线检查

日期：2026-11-26。检查基线：`fdf69d0`（`feat: 进一步集成JMComic搜索能力`），开始时工作区干净。本轮仅读取现有实现并新增规划材料，不执行用户服务、依赖安装、下载或发行。

## 已有能力，不应重复重写

- `internal/collectionworker/config.go:29-30,61-73` 已有可选 JMComic 开关及 Windows 默认 `python`。配置可指定可信解释器绝对路径。
- `internal/collectionworker/config.go:194-221` 的 `toolPath` 已规范化绝对工具路径，并在 Windows 限定 `.exe`；不用用户输入拼 shell 命令。
- `internal/collectionworker/jmcomic.go:101-120` 使用固定嵌入桥接，参数 `-I -B -c`，JSON 经 stdin 输入，stdout 有界且严格解析。
- `internal/collectionworker/jmcomic_bridge.py:421-455` 输入按 bytes 解析；成功 JSON 使用 `ensure_ascii=True`。既有中文标题／路径不能未经反例就认定有编码缺陷；需要补验证重定向错误流，并评估 `-X utf8`。
- `internal/collectionworker/tools.go:75-123` 已有环境白名单、任务私密临时目录、无 shell 工具调用；新运行时应接入此路径。
- `internal/collectionworker/process_windows.go` 已有隐藏窗口、挂起后归属 Windows Job、取消回收子树并等待句柄释放。无需为每个漫画操作另造一套 process kill。
- `internal/collectionworker/service.go:72-103,212-228` 有生命周期、请求及操作等待和根锁；进程 CLI 主入口接受取消 context。原 CLI 正常停服依赖 signal，不适合直接当作隐藏 GUI 子进程的唯一控制协议。
- `.trellis/tasks/11-25-jmcomic-advanced/research/final-validation.md` 区分实际 Windows app/worker、真实固定依赖离线检查和协议 fixture；未证明无 Python 的干净机器可开箱运行。

## 实际缺口

1. **发行**：`scripts/release.py:299-320` 构建 `nas.exe`／`栖库.exe` 并处理 FFmpeg，未构建 `nas-collection-worker.exe`，未装配 JMComic 的 Python 运行时或专用许可清单。`scripts/verify_release.py:120-145` 的元数据是严格固定 schema，增加可选插件需同步版本及校验，不能只向 ZIP 塞目录。
2. **启动器**：`internal/desktop/launcher.go:365-459` 原生分支只启动共享 app；注入的 map 只有地址、业务根与媒体根。没有托管 worker 的真实启动就绪、连接注入和有序停止。旧外接 worker 来自继承的 NAS 配置，必须保留其优先级／所有权。
3. **连接**：`internal/app/collection_config.go:22-62` 要求 URL、token、ReadRoot 成组提供，HTTP 需显式许可。托管模式应提供整组经过核验的本机值，不能将某个新 URL 与旧 token 或旧根混搭；不能全局放宽远程 HTTP。
4. **配置迁移**：`internal/desktop/config.go:20-31` 是严格 schema v1 的九字段配置。添加原生 worker 模式必须设计 v1 显式迁移或独立版本化配置，不能让旧 profile 因缺字段无法启动，也不能偷偷改用新 worker。
5. **Windows 私密权限**：worker 的 token 检查在 Windows 不依赖 POSIX 0600；创建根主要使用 Mkdir／Chmod。桌面 `internal/desktop/platform_windows.go:39-55` 已有真实当前 SID／SYSTEM 保护 DACL 逻辑，可借鉴在**新建的自有**秘密／状态根应用。不可重设未知既有用户目录 ACL。
6. **文档**：`docs/jmcomic.md` 原生仍是手工 venv／解释器准备，`docs/collection.md:77-81` 说明单独 worker 对接。`docs/windows.md` 有部分旧 schema 文案，后续实现文档应按当前 schema 6 同步，不能把桌面配置 schema 与业务数据库 schema 混为一谈。

## 方案约束

- 外接 worker、Linux／Docker 与当前主应用不执行 Python 的边界保留。优先由桌面进程托管独立 worker，再由 worker 使用既有 Job 管理 Python。
- 托管 worker 使用 `127.0.0.1:0` 真实绑定并通过当前用户私有控制通道返回就绪地址／实例身份；不要“先探测空闲端口再启动”，也不要把 JSON 业务日志当握手。
- 私有控制通道既负责 ready，也应支持停止和父进程断开处理；正常退出后再用 Job 作自有子树的兜底。托管父 Job 与现有 Python Job 的嵌套、祖先进程退出和句柄释放要实测。
- 数据与程序目录分离。只在本实例可写本地根创建 collection-data／secrets／logs；已发布文件保留，不能升级时搬进新版本程序目录。
- 字符编码、中文／空格路径、Windows 文件占用及长路径应写成可验收项。未验证的扩展长度路径或网络盘应明确拒绝／报告，不能承诺任意长路径。

## 检查方法

先通过 CodeGraph query 查找符号，部分启动器符号未在索引命中，随后用磁盘源码校正。不因索引缺失推断实现不存在。一次 PowerShell 命令误用花括号路径展开及一次猜测不存在的 native.go，已改为读取实际 launcher.go；这些是只读工具定位问题，不是产品兼容缺陷。
