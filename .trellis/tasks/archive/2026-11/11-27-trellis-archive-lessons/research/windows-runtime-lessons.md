# Research: Windows 与运行环境归档要点

- Query: 从本轮归档任务中提取 Windows、构建/分发、配置、BT 与原生插件可复用经验。
- Scope: internal；仅阅读既有任务、当前规范和定向源码，未联网、未启动产品或复跑历史测试。
- Date: 2026-11-27
- 检索：在项目环境中运行 CodeGraph status/query；索引能定位 `configure_environment`，部分新 Go 符号未收录，改用定向 `rg` 与源码核对，未同步或重建索引。

## Findings

### 1. 所有开发工具先继承仓库环境

- 触发：构建、测试、临时脚本、浏览器/MCP 服务启动。
- 结论：使用 `python -B scripts/project_env.py -- <命令>`；PowerShell 可先 `. ./scripts/project-env.ps1`。只修改当前进程环境，将受控缓存/临时目录放在 `.runtime/`；拒绝链接越界和不可写目录，不回退系统目录。
- 踩坑：禁止用户环境变量和 `go env -w`；Python `tempfile` 已缓存时也要更新；不能将“受控路径隔离”扩大成所有第三方遥测零写入保证。Windows npm shim 参数需要原生 Node/npm CLI 解析。
- 来源：`scripts/project_env.py:129`；`.trellis/spec/backend/cross-platform-contract.md`；`11-18-project-local-runtime/research/validation.md`（以下任务路径均相对于 `.trellis/tasks/archive/2026-11/`）。证据：当前规范＋源代码核对＋历史验证。

### 2. 缓存清理必须保护可复用材料和唯一检查点

- 触发：`.runtime` 膨胀、清理会话缓存或历史测试产物。
- 结论：先出精确文件计划，执行时复核所有权、链接、活动进程与文件变化；保留运行时/许可证完整闭包、依赖、最新已验证二进制、压缩历史证据和暂停工作唯一草稿。
- 踩坑：不能按时间整目录删除或遍历重解析点；历史 `S03.2.3` 草稿不能当垃圾。已有 Go 模块文件不代表完整模块图可离线解析；保留文件摘要核对也不代表本次重新执行应用回归。
- 来源：`11-18-session-temp-cleanup/research/implementation-result.md`；`11-26-runtime-slim/info.md`。证据：当前规范＋历史验证；旧 C 盘清理任务只有真实预览和隔离 fixture，不能称真实 C 盘删除已验收。

### 3. 退出成功以资源真正释放为准

- 触发：托盘退出、IPC quit、启动取消、原位升级、媒体工具子孙进程。
- 结论：统一生命周期先关闭请求接纳、取消并排空 handler/worker/自有进程树，再关数据库、释放数据租约与 IPC/日志。Windows 工具挂起创建、归入自有 Job 后恢复；等待进程句柄和映像释放，才能报告安全退出与同 profile 重启。
- 踩坑：`Serve` 返回、`Server.Close`、托盘消失、Job 统计归零都不是文件释放证明；`WaitDelay` 不能替代进程树回收；禁止按 exe 名通杀。异步管道缓冲区和 OVERLAPPED 必须 pin 到真正完成。
- 来源：`internal/app/server.go:73`、`internal/app/media_process_windows.go:101`、`internal/desktop/ipc_windows.go:112`；`11-18-windows-complete-exit/research/exit-root-cause.md`；跨平台契约。证据：当前规范＋源代码核对；历史真实 Windows 验证另见该任务 `integration-validation.md`，不是本次重跑。

### 4. 可携带配置以真实 EXE 路径和稳定实例身份为锚

- 触发：双击/异目录启动、复制升级、native/docker 切换、原有 LocalAppData 配置复用。
- 结论：默认 `<真实 EXE>/qiku-data/profiles/<mode>`；原位升级保留整个 `qiku-data`。严格读取保存的完整 JSON，只有文件缺失才使用默认值；保存的绝对媒体/数据路径和 instanceId 保持权威。
- 踩坑：不能按 CWD 定位，不静默回退 C 盘，不自动迁移旧 profile，不因包移动改写原有媒体目录；默认目录只读时明确提示 `--profile`。数据租约在数据库恢复/迁移前取得，换端口不等于可共享写库。
- 来源：`internal/desktop/config.go:108`、`internal/desktop/environment.go:23`；跨平台契约；`11-26-windows-portable-profile/research/final-validation.md`。证据：当前规范＋源代码核对＋历史验证。

### 5. .env 是只读快照配置，不是可执行脚本或依赖安装器

- 触发：Windows 桌面启动、插件开关或配置优先级问题。
- 结论：显式 CLI > 进程环境存在性 > 真实 EXE 同级 `.env` > 保存 JSON > 默认；显式空字符串也屏蔽文件值。只在启动时生成统一快照；文件仅接受 NAS 键、普通 UTF-8/BOM、64 KiB 内单行语法，不展开变量/命令。完整重启才重新读取。
- 踩坑：不能用非空判断代替存在性、不能全局 `os.Setenv`，不从 profile/CWD 读文件，也不让普通 CLI/worker/Docker 隐式消费桌面文件。损坏文件不应阻止 status/quit。开关 true 不会补装运行材料。
- 来源：`internal/desktop/environment.go:40`、`:91`、`:120`；`11-26-windows-dotenv/research/final-validation.md`；跨平台契约 dotenv 章节。证据：当前规范＋源代码核对＋历史 Windows 集成；本机缺依赖时报 unavailable 属正确行为。

### 6. 原生 BT 连接和路径比较不能按宿主 OS 想当然

- 触发：Windows qB/Torznab 配置、下载选文件/发布、跨平台引擎路径。
- 结论：`<profile>/bt.json` 仅 native 读取，私有相对路径相对 profile；连接已有外部 qB/Jackett/Prowlarr，不安装、接管或停止其进程。仅将下载根的 `complete` 纳入媒体授权，继续走选中文件和发布门禁。
- 踩坑：路径比较按引擎路径自身格式：Windows 盘符绝对路径忽略大小写/斜杠差异，POSIX 保留大小写；保留 `E:/`，拒绝 UNC/设备/ADS/上级跳转。只去掉一个 BOM，不 trim 密码。外部引擎停止/移除不自动删 payload。
- 来源：`internal/desktop/bt_config.go:38`、`:84`；`internal/app/download_config.go:162`；`.trellis/spec/backend/bt-windows-contract.md`；`11-26-bt-windows-native/research/final-validation.md`。证据：当前规范＋源代码核对＋历史受控协议；真实 BT peers/分片/公网索引器未由 fixture 证明。

### 7. 托管插件应独立降级，生命周期和连接所有权要清晰

- 触发：Windows 托管 worker、多插件组合、老 profile 迁移、外部模式。
- 结论：共用一个受拥有的 worker，漫画/视频能力分别探测与展示；一项失败不拖垮另一项或基本媒体播放。私有继承管道握手后再验证 token/能力/读根 challenge；启动时预绑定随机 loopback 端口。schema-1 迁移保留原字节与身份，原 disabled/external 不因新包存在运行时而自动启用。
- 踩坑：子进程配置单独组装并清除旧连接元组，不修改全局环境；外部模式不停止外部 worker。托管模式显式 Python 无效必须失败，不回退 PATH；仅存在 manifest 不代表插件 ready。诊断 0/2/1 分别是本地就绪/禁用不可用/诊断失败。
- 来源：`internal/desktop/collection_managed.go:116`、`:162`；`.trellis/spec/backend/jmcomic-windows-contract.md`、`uav-windows-contract.md`。证据：当前规范＋源代码核对＋历史离线和真实进程集成，非公网站点验证。

### 8. 私有 Python 分发必须绑定材料闭包和真实探测

- 触发：JMComic/UAV 运行时装配、缺依赖修复、发行验证。
- 结论：按独立 manifest 固定版本/来源/源码摘要/运行时/许可/架构与每个成员，保留隔离 `._pth`，执行 `-I -B -X utf8` 真实 import/本地工具 probe；不可复制开发 venv 或运行时 pip。UAV 还要验证 `direct_url.json`、wheel RECORD 与原始源码身份。
- 踩坑：大小写不敏感路径相等不代表 manifest 字节规范；wheel dist-info 大小写应装配时规范化。JMComic 与 UAV 依赖/清单独立，不能靠修改模块全局量复用装配器。摘要一致性不能替代签名、干净机兼容或人工分发审核。
- 来源：`scripts/jmcomic_runtime.py:58`、`:127`；`scripts/uav_runtime.py:67`、`:77`、`:281`；两份 Windows 插件规范及各任务最终验证。证据：当前规范＋源代码核对＋历史真实离线 probe。

### 9. 数据库、桌面配置、发行清单版本域相互独立

- 触发：新插件/新材料增加版本字段、验证旧发行包。
- 结论：数据库 schema 6、desktop JSON schema 2、release schema 1/2/3 是独立演进域；release schema 3 为 UAV/yt-dlp 增加独立摘要，旧 schema 按原严格字段和工具 inventory 验证，不能追溯性拒绝旧合法包。
- 踩坑：完整 UAV 需要私有运行时、固定下载器、媒体工具与许可证/源码材料，`development-without-tools` 或本机补材料不能改称正式完整离线发行；机器校验不等于人工分发许可结论。必须绑定同一稳定候选的源码、二进制与验证结果。
- 来源：`.trellis/spec/backend/uav-windows-contract.md`；`11-26-uav-windows-native/research/quality-review.md`；`11-26-jmcomic-windows-native/research/final-validation.md`；`11-27-windows-uav-local-repair/repair-summary.json`。证据：当前规范＋历史验证；完整正式发布、干净机与目标 NAS 验收仍分开记录。

### 10. PowerShell 与 Vite 需要真实调用链验证

- 触发：中文脚本解析、npm 参数吞失、Vite ready 但首页 404、开发写请求 403。
- 结论：PowerShell 中文脚本使用 UTF-8 BOM，分别验证 5.1/7；npm 从项目入口保留原始 argv 和真实退出码。看真实 Node/Vite 命令行并 GET 首页、主模块、Vite client。开发代理 `/api` 与 `/healthz` 保持 `changeOrigin:false`，保留浏览器 Host/Origin/Sec-Fetch-Site。
- 踩坑：`.cmd` 替身通过不能证明 npm.ps1 正确；ready 日志/端口监听不是 HTTP 正确证据。不能为开发代理 Host 改写问题放宽后端同源校验或删除安全头；不存在路由返回 404 仅证明通过中间件，不是业务写入成功。
- 来源：`scripts/dev.ps1:15`、`web/vite.config.ts:19`；`11-20-powershell-dev-encoding/research/implementation-checks.md`；`11-20-powershell-npm-arguments/research/real-runtime-checks.md`；`11-20-vite-proxy-origin/prd.md`。证据：当前规范＋源代码核对＋历史真实双 PowerShell/Vite/Go 联通验证。

### 11. 品牌资源和 CI 触发均应保持可重复且范围准确

- 触发：桌面图标、普通 go build、发布包或 CI 修改。
- 结论：十帧 ICO 与预置 Windows amd64 `.syso` 一同维护，用固定生成器 `--check` 保证字节一致；大小图标独立 owned handle，释放顺序跟随托盘/窗口生命周期。跨平台工作流当前只允许 `workflow_dispatch`，不自动发布。
- 踩坑：同资源 ID 不可用 `LR_SHARED` 加载两种尺寸；按包路径构建，不能单独构建 Windows main 文件。原生托盘 API 验证不是物理点击/Explorer 截图验收；Linux 交叉编译不是 ARM/Linux 实机运行。
- 来源：`internal/desktop/shell_windows.go:191`；`.github/workflows/cross-platform.yml:3`；`11-19-windows-desktop-icon/research/implementation-validation.md`；`11-20-cross-platform-manual/prd.md`。证据：当前规范＋源代码核对＋历史本机 API/开发包验证，未重跑云端 Actions。

### 12. 本机网络/材料问题先分层定位，不能削弱安全门禁

- 触发：本地能力 unavailable、搜索 502、Fake-IP、特定关键词展开超限。
- 结论：先用原 `--check-uav`、manifest verify/probe 确认本地运行材料，再区分 DNS、代理、站点规则与下载能力。Fake-IP 冲突仅对受影响域名追加持久过滤并校验/热重载，保留 TUN/用户规则/凭据与公网 IP 判定；本机补材料先做所有权/摘要/可回滚核对。
- 踩坑：`198.18/15` 不能为兼容 Fake-IP 而加入公网许可；搜索成功不代表下载通过。历史 Jable/黄果搜索恢复，而黄果特定关键词仍触发 100 集有限展开保护，不能标成成功或降低限额。本机 Clash 绝对路径不应升级为通用项目契约。
- 来源：`11-27-windows-uav-fake-ip-repair/prd.md`、`research/quality-review.md`；`11-27-windows-uav-local-repair/prd.md`、`repair-summary.json`；`.trellis/spec/backend/uav-contract.md`。证据：历史真实本机搜索/本地修复；未进行媒体下载或正式分发验收，本次没有确认当前公网状态。

## 任务覆盖清单

| 归档任务 | 提炼条目 |
| --- | --- |
| 11-18-session-temp-cleanup | 2 |
| 11-18-project-local-runtime | 1、2 |
| 11-18-windows-one-click | 3、4、9、11（共享跨平台契约及其历史实现研究） |
| 11-18-windows-complete-exit | 3 |
| 11-19-windows-desktop-icon | 11 |
| 11-20-cross-platform-manual | 11 |
| 11-20-powershell-dev-encoding | 10 |
| 11-20-powershell-npm-arguments | 10 |
| 11-20-vite-proxy-origin | 10 |
| 11-26-bt-windows-native | 6 |
| 11-26-jmcomic-windows-native | 7、8、9 |
| 11-26-runtime-slim | 2 |
| 11-26-uav-windows-native | 7、8、9 |
| 11-26-windows-dotenv | 5 |
| 11-26-windows-portable-profile | 4 |
| 11-27-windows-uav-local-repair | 8、9、12 |
| 11-27-windows-uav-fake-ip-repair | 12 |

## External references / Related specs

- 本次不访问外部资料。CPython 3.14.4、JMComic 2.7.12、UAV 3.8.4、yt-dlp 2026.10.26 是既有清单/验证记录绑定的历史版本，不声称为最新版本。
- 当前权威规范：`.trellis/spec/backend/cross-platform-contract.md`、`bt-windows-contract.md`、`jmcomic-windows-contract.md`、`uav-windows-contract.md`、`uav-contract.md`，以及 `.trellis/spec/frontend/app-contract.md`。

## Caveats / Not Found

- `task.json.status=completed` 是批量 archive 自动写入结果，不能当验收证据；本报告依赖规范、定向源码与具体历史验证记录。
- 历史 `.runtime` 证据部分已压缩或原夹具已清理，引用任务中的研究文本保留；本轮未声称重新执行任何历史成功命令。
- 空 Go 测试集仅计编译；跨编译、协议 fixture、开发机真实进程、本地依赖 probe、公网站点搜索、媒体下载、干净机、Docker、目标 NAS 各自独立记账。
