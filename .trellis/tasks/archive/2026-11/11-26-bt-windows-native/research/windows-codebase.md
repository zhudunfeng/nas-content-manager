# Research: Windows 原生 BT 搜索与下载兼容缺口

- Query: BT/Torznab 搜索与 qBittorrent 下载在 Windows native 模式下有哪些实际缺口，怎样以最小变更兼容。
- Scope: internal，现有代码与项目契约；未联网查新版本或安装包。
- Date: 2026-11-26

## Findings

### 1. 已有外接协议本身可复用，不需要新 BT 实现

- `internal/desktop/launcher.go:385-412` 的 `controller.start` 将桌面监听地址、数据目录、媒体授权和 collection 覆盖值交给 `app.LoadConfigWithLookup`；未覆盖参数继续读取 `os.Getenv`。因此 `NAS_DOWNLOAD_*` 与 `NAS_TORRENT_SEARCH_*` 在原生桌面模式并未被主动禁用。
- `internal/app/download_config.go:31-99` 支持 `external` qB WebAPI、应用下载根和引擎下载根分离；Windows 本地下载目录可使用 `E:\\...`，引擎侧使用 `E:/...`。启用前必须创建下载根并保证与 DataDir 分离，普通媒体根只可授权 `complete` 子树。
- `internal/app/torrent_search_config.go:38-82` 从服务端小型来源 JSON 读取 1–8 个 Torznab 源，搜索只走 HTTP；没有 Linux 进程调用依赖。Jackett 或 Prowlarr Windows 实例都可经现有 external 契约接入。
- `scripts/verify_downloads.py:412-416` 当前固定用虚拟 POSIX 引擎根 `/downloads`；即便脚本在 Windows 执行，也不会自然覆盖原生盘符和大小写回报。

### 2. 确认存在 Windows 引擎路径比较缺陷

- `internal/app/download_config.go:121-127` 的 `cleanEnginePath` 仅替换反斜杠并 `path.Clean`，接受盘符但不规范大小写；拒绝 UNC。
- `internal/app/download_sync.go:200-225` 的 `findOwnedTorrent` 将 qB 回报 `SavePath` 直接与生成的 staging/final 做 `actual != stage && actual != final` 判断。Windows 原生路径盘符或目录段大小写不同会误判为偏离受管目录。
- `internal/app/download_sync.go:367-371` 的 move 完成条件和 `internal/app/download_import.go:15-22` 的 publish 完成条件也直接比较 engine path 字符串；相同 Windows 路径的不同大小写可能导致 move 永不确认或反复登记发布。
- `internal/app/download_import.go:97-101` 与 `download_sync.go:219` 另有本机持久根字符串比较，需要复核改动范围及历史记录行为。
- `internal/app/download_gate.go:57-64` 已有 `downloadPathKey`：本机 Windows 路径用小写、Linux 保持大小写敏感。不能把它直接用作远端引擎路径比较，因为 Linux 主应用也可能连接 Windows qB，反向也可能存在；应按引擎路径自身格式判断盘符语义，保留 POSIX 大小写敏感，不对所有路径无条件 EqualFold。
- 推荐复用一个清晰的 engine 路径比较函数，覆盖上述三处，持续拒绝盘符相对路径 `C:foo`、路径前缀近似、NUL、UNC 和不支持的设备命名空间。

### 3. 原生自动安装/托管与外接支持是两个独立范围

- `internal/app/engine_config.go:10-14` 只有私有 Unix socket 和 manifest 目录配置。
- `internal/app/engine_runtime.go:43-49` 使用 Unix socket HTTP transport；`internal/app/engine_api.go:43-46` 在未配置 socket 时引导 `compose.engines.yaml`。
- `internal/app/engine_api.go:100-149` 的初始状态及 disabled 提示都面向 NAS 托管安装；external 分支不尝试接管引擎。
- `web/src/components/EngineSetupDialog.vue:250` 目前只展示 NAS Compose 启用托管说明；`TorrentSearchDialog.vue:200` 的“本机托管”措辞容易让 Windows 原生用户误以为现有 Jackett 初始化按钮可直接工作。
- `docs/windows.md` 的运行方式表将 BT 归于 Windows Docker 完整模式，后文明确“原生模式不自动安装 Windows 版 BT 引擎”。这不表示外接 Windows 引擎不可用，而是缺少一条完整的原生接入说明和启动参数样例。
- 现有 native `collectionMode` 的生命周期用于 nas-collection-worker/Python 插件，不能未经设计直接扩展为 qB/Jackett 托管器；BT 引擎有独立长期传输、做种、端口、WebUI 凭据和首次使用告知。

## Minimum Feasible Approach

1. 修复引擎 Windows 路径等价比较，保留 POSIX 严格大小写和现有标签/哈希/目录三重所有权验证。
2. 提供可复用的 Windows 原生 external 接入路径：Windows qB 开启 WebUI，应用与引擎下载根映射为同一个明确专用本机目录，密码置于私密文件；Windows Jackett/Prowlarr 来源仍用现有 Torznab JSON。
3. 桌面入口继续透传外接配置，不自动停止、重置或接管用户已有引擎；补齐 Windows 适用提示及 UTF-8 无 BOM 配置生成示例，避免 Windows PowerShell 5.1 的 `Set-Content -Encoding UTF8` 默认 BOM 与当前严格 JSON 解码冲突。
4. 扩展现有集成 harness 的引擎路径模式，让 Windows 风格根、反斜杠及盘符/目录段大小写变化实际经过生产协调器的所有权、move、发布、入库链路。

如主任务决定“默认免配置自动安装/托管”也是目标，需要额外设计并持久化：固定原生安装材料的 provenance/校验、私有 profile/配置、qB 固定 PBKDF2 凭据、Jackett API Key 生成与发现、回环端口、原生受管进程树、单实例身份、重启恢复和正式发行清单。不能把仅有 external 验证宣称成自动托管交付。

## Suggested Validation

- 临时 Go 行为测试：Windows 引擎路径不同盘符大小写、目录大小写、正反斜杠、末尾斜杠；POSIX 大小写不相等；错误盘符、兄弟路径、UNC/设备路径拒绝。执行后仅移除新增临时测试。
- 下载集成：原生 Windows 风格的 SavePath 回报贯穿上传种子/磁力、停驻元数据、显式选集、setLocation、complete 发布及入库，不只测试辅助函数。
- 搜索集成：Torznab HTTP/磁力/磁力重定向结果继续走同一受管 add intent，验证 private URL/API Key 不进入公共响应。
- 桌面验证：profile 中文/空格、外接配置透传、重启配置稳定、退出仅关闭本应用而不杀外接 qB/Jackett。
- 原 regressions：`verify_downloads.py`、`verify_torrent_search.py`、`verify_engine_bootstrap.py`，相关 Go vet/build；UI 若有修改还需 typecheck/build。
- 所有本地工具经 `python -B scripts/project_env.py -- ...` 运行。

## Related Specs

- `.trellis/spec/backend/download-contract.md`：现有选集、所有权、发布和历史根门禁。
- `.trellis/spec/backend/torrent-search-contract.md`：私密 Torznab 来源、结果令牌与原 add 路径。
- `.trellis/spec/backend/engine-bootstrap-contract.md`：默认 disabled、external 不接管和 Linux helper 边界。
- `.trellis/spec/backend/cross-platform-contract.md`：桌面原生配置优先级、实例生命周期、缓存限制。
- `.trellis/spec/backend/runtime-contract.md`：媒体授权、只读原文件、HTTP/DB 生命周期。

## Tools / References

- 已确认 `.codegraph/codegraph.db` 与 CLI 存在，通过 `python -B scripts/project_env.py -- node D:/adunsoftware/nodejs/node_modules/@colbymchenry/codegraph/npm-shim.js --help`、`query cleanEnginePath`、`query engineTaskPath --json`、`callers cleanEnginePath` 检索定位；再用当前源码核对行号。
- codegraph 的 callers 结果未列 `download_import.go:publish` 这一当前调用点，说明索引并非覆盖全部最新源码；必须结合目标源码读取，不能把未返回结果等同不存在。
- 未使用网络或更换第三方产品/版本；所有依据均来自当前仓库。

## Follow-up: Windows 文件名、清单与 BOM

- `internal/app/download_torrent.go:307-326` 的 `validateDownloadRelative` 已拒绝反斜杠、冒号、NUL、控制字符、绝对路径、空段、`.`/`..`、尾随点/空格，以及 CON/PRN/AUX/NUL/COM1–9/LPT1–9。仍未拒绝 Windows 文件名不允许的 `< > " | ? *`，这些名称在上传种子解析和 qB 元数据清单中都能通过当前校验，直到引擎或最终文件访问阶段失败。建议统一拒绝这些非法字符，以当前已采用的跨平台清单约束为准；不要通过重命名文件绕开身份校验。
- `validateDownloadFiles:330-357` 已按小写检查重复路径和文件/目录冲突，保护 Windows 大小写碰撞。这一约束是跨平台既有行为，保留即可。
- `internal/app/download_qbittorrent.go:296-312` 的 files 适配器直接使用 qB `name`；`download_sync.go:884-904` 的 `downloadManifestsMatch` 对相同文件 index 的 Path/Size 作精确比较，`torrent` 原始文件名和 qB 回报名不一致会触发清单冻结。未看到真实 Windows qB 把文件相对路径回报为反斜杠或改大小写的证据，因此目前不建议无条件放宽 manifest 路径匹配；否则容易掩盖引擎重命名。新集成样本可以保持正确的 slash 文件名，单独模拟 save_path 原生格式变化。
- `download_import.go:115-145` 使用 `filepath.FromSlash` 转成本机路径，经 `os.OpenRoot`/逐段 `Lstat`/regular-file/尺寸/mtime 再验证，因此合法正斜杠元数据能映射到 Windows NTFS；这段不依赖 POSIX 分隔符。
- `torrent_search_config.go:60-70` 直接用 Go JSON decoder 解析原始字节，没有处理文件开头 UTF-8 BOM。Windows PowerShell 5.1 的 UTF8 写文件示例容易生成 BOM，导致整份配置报“必须为仅含 sources 数组的 JSON”。可以在服务端仅去掉字节流开头的单个 UTF-8 BOM，再保持 64 KiB 预算、DisallowUnknownFields/EOF/1–8 sources 等校验；不要对内容任意 TrimSpace/移除内部 U+FEFF。
- `download_config.go:103-117` 的 `readDownloadPassword` 只 TrimRight CR/LF，故 BOM 密码文件会把 BOM 当真实密码导致认证失败。最保守兼容方案是 Windows 配置生成路径明确写 UTF-8 无 BOM，并提醒不要用 5.1 默认 UTF8 写密码。若调整读取语义需单独明确密码字节契约及回归，避免静默改写真实凭据。

## Caveats / Not Found

- 当前 `.trellis/.current-task` 在研究开始时仍指向 `11-26-uav-windows-native`；主代理明确授权本 BT task 作为研究输出目录，已通知主代理切换活动任务。
- 未找到 Windows native qB/Jackett 自动安装与托管实现；已有 worker 托管仅覆盖 collection 插件。
- 本研究未实际启动真实 qB/Jackett、下载公网 torrent、验证真实站点或执行 Docker；不可将源码分析认定为实机验收。
