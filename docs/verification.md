# 功能验证记录

最新验证日期：**2026-11-26（Asia/Shanghai）**。各章节保留相应实施日期和历史结果。本记录区分已经执行的检查与目标设备待验事项，不将编译成功、稀疏文件或短视频样本视为完整 NAS 性能验收。

## Windows 原生 UAV 五站运行与分发（2026-11-26）

新增 UAV 独立 embedded Python、`--check-uav`、桌面单 worker 多插件能力、固定下载器材料装配及 release schema 3；Jable、MissAV、Hanime1、黄果和 SupJav 沿用原搜索、发布和播放流程。使用与排错见 [Windows 原生视频](uav-windows.md)，构建材料见 [发布流程](release.md)，详细本机证据见[实施验证记录](../.trellis/tasks/11-26-uav-windows-native/research/implementation-validation.md)与[独立复核记录](../.trellis/tasks/11-26-uav-windows-native/research/quality-review.md)。

- 实际离线装配 Windows x64 CPython **3.14.4**、UAV **3.8.4** 固定源码，784 份运行时/许可文件。UAV Python/Go 交叉校验及真实 embedded probe **70 项**通过，含五站模块、AES、CA、空 PATH、污染 Python 环境与中文空格迁移；共享辅助改动后原 JMComic 真实运行时回归 **49 项**通过。
- 真正私有 Python、固定官方 **yt-dlp.exe 2026.10.26** 和本机真实 FFmpeg 完成离线解析/HLS：五站搜索与解析、SupJav 七类 provider/十项回退，有限 HLS 与 AES-128 均成功解码；缺片、不支持的加密按预期失败，无 FFmpeg 网络回退。站点传输使用受控样例，未证明公网可用。
- 最终候选主服务/worker 完成 UAV **102** 项协议、core **44**、collection **27** 项回归；本轮原 JMComic 协议 **66** 项通过。协议 fixture 与上条真实依赖验证分开记录。原生桌面 **17 类**及真实托盘十尺寸图标、退出后即时文件释放检查通过。
- 真实私有运行时的 JMComic-only **14 项**、UAV-only **20 项**、双插件组合 **24 项**桌面矩阵通过，组合实际观察漫画 26 个、视频 25 个原生映像；含只读 ACL、双向部分故障隔离、取消/父强杀、嵌套 Job、独立哨兵保留、映像替换和同 profile 重启。矩阵与最终候选的154份生产输入一致，构建选项与摘要差异有记录，详见[桌面验证](../.trellis/tasks/11-26-uav-windows-native/research/worker-desktop-validation.md)。
- 独立复核修复下载器版本门槛及旧 schema 1/2 合法工具包兼容。发布边界自测 **131 项**、下载器材料自测 **36 项**通过；真实 UAV 785 成员专项归档正例及三类重算摘要负例通过。真实基础开发包构建并通过17文件归档核验，明确缺工具和可选插件，未作为完整视频包。
- 全仓 Go vet/包编译、Windows 三入口、Vue 严格类型与 Vite 构建通过；PowerShell 5.1/7 实际解析及中文空格参数透传通过。Linux amd64/arm64 为交叉编译证据，未在 Linux 执行。临时单元测试源码已清除，保留可重复集成脚本；空 Go 测试集只计编译。

本轮尚未取得干净 Windows 11 普通用户、真实公网站点、Docker、ARM64 运行或实体 NAS 验收；完整下载器对应源码与组件材料仍需维护者实际准备并人工审核。机器材料校验不代替该审核，本次未生成虚假声明、提交、发布或部署。

## Windows 原生 JMComic 运行与分发（2026-11-26）

实现随包私有 Python、桌面托管 worker、`disabled|managed|external` 三模式、桌面配置 v1→v2 原字节备份迁移、私有控制管道、Windows Job 所有权和故障降级；原搜索、选章、批量、发布、阅读及数据库 schema 6 保持原契约。使用与故障处理见 [Windows 原生漫画说明](jmcomic-windows.md)，详细结果见[主线验收报告](../.trellis/tasks/11-26-jmcomic-windows-native/research/final-validation.md)。

- 真实装配 Windows x64 CPython **3.14.4**、JMComic **2.7.12**；运行时 620 份成员、50,123,069 字节，包含本机扩展、标准库、CA、元数据和许可。Python／实际 Go 加载器的完整性及污染环境交叉验证 **49 项**通过；禁用网络入口的缓存重建清单一致。真实私有解释器执行固定桥接离线套件 **22 个检查点**通过，网络调用为 0，不代表真实站点搜索下载。
- 最终统一候选位于 `.runtime/verify/jmwindows-final-20261126-2/`，19 条命令全部退出 0；由同一实际发行包解压的应用／worker 完成原漫画 **66**、高级漫画 **144**、UAV **102** 项协议回归，以及核心媒体 **44**、资源收集 **27**、发行边界自检 **91** 项。上游使用协议 fixture；媒体检查使用真实 FFmpeg 和生成样本，其余未选择的 baseline 组不计通过。
- 同批原桌面 **17 类**、托管 worker **20 项**、托管桌面 **14 组**通过。实际 PSAPI 观察 **26 个**已加载私有 Python／Pillow／curl／AES／SSL 映像，覆盖启动取消、父强杀、嵌套 Job、独立哨兵保留、映像独占／改名／原位覆盖、同 profile 重启及 worker 故障期间媒体健康／主动重检。真实依赖生命周期 fixture 不等于真实公网下载中断。
- 实际生成并严格校验 Windows amd64 含漫画运行时的 **636 文件开发包**；可审阅程序目录为 `.runtime/bin/jmwindows-final-20261126-2/`。全仓 Go vet／包编译、gofmt、Python AST／类型注解检查、Vue 严格类型与生产构建通过；发布 PowerShell 在 5.1 和 7.6.4 实际解析通过。Linux amd64／arm64 主服务和 worker 共四份交叉构建通过，未执行。独立复核另完成失败退出中文日志定点行为回归 30 次，临时测试源码已删除；清源后的 Go 空测试集只计编译。
- `artifacts.json` 保存最终归档与七份二进制身份；`final-audit.json` 回读为 passed，169 份发行构建输入无漂移，摘要一致，新增临时 Go 测试、行尾空白和确切最终产物活进程均为空。第一候选因与独立审查修复重叠而触发源码漂移拒绝，完整保留失败证据，未混入最终通过结果。

本轮来自现有 Windows x64 开发机，未取得干净 Windows 11 普通用户、ARM64、真实公网站点、Docker、Linux 执行或物理 NAS 验收。当前包使用 `--development-without-tools`，未纳入 FFmpeg 完整发行材料，不能称完整离线媒体发行版。任务进入 review，未自动提交、部署或归档，未操作用户实例或全局环境。

## JMComic 漫画插件验证（2026-11-25）

新增固定的可选 `jmcomic` 来源，复用原资源收集任务、发布、漫画库扫描和阅读，不改变 schema 6。本轮没有执行真实漫画站点搜索或下载，没有运行 Docker 或连接真实 NAS。

- `scripts/verify_jmcomic.py` 使用真实 Go 主服务与 worker、明确的上游协议样例，**66 项断言全部通过**。覆盖鉴权／同源、分页／空／错、参数白名单、元数据预览、固定漫画分类、稳定请求重试、两章四图发布入库及鉴权阅读、不完整／非图片产物隔离、失败重试、实际获取中的取消、重启、旧本地收集兼容、禁用／缺依赖／离线降级。协议样例没有执行真实 Python 桥接。
- 真实安装 `jmcomic==2.7.12` 及固定依赖，`pip check` 通过；`scripts/verify_jmcomic_bridge.py` 保留实际选项、API 搜索逻辑、下载器及图像转换，只替换域名发现、外部元数据、搜索响应和图片传输。**11 个离线检查点通过**，包括分页、数字搜索重定向、两章四图的 3 JPEG／1 GIF 解码、部分失败、文件数／字节预算拒绝、脱敏协议与显式代理绑定；真实网络调用为 0。真实 Go worker 使用真实虚拟环境和脱敏子进程环境完成探测，能力返回 `available=true,version=2.7.12`。
- 独立传输补验使用真实 curl-cffi、生产 `runTool` 环境及认证公网代理：随机端口 HTTP／HTTPS 哨兵请求被端口白名单拒绝，哨兵连接数为 0；标准 80／443 环回请求被 IP 策略拒绝，另有 6 项 `publicDial` 私网地址拒绝断言通过。随机端口哨兵本身不作为单独的 IP 拒绝证据。没有公网请求；证据在 `.runtime/verify/jmcomic-real-proxy-20261125T012540798742400/report.json`，临时测试和自有代理／哨兵／子进程均已清理。
- `scripts/verify_jmcomic_browser.py` **38 项断言通过**：375／768／1024／1440 px、明暗主题、内边距和无横向溢出、Escape焦点恢复、来源类型切换清理、分页／空／错、编号变更清除预览、固定漫画目标、任务跳转、实际入库图片阅读翻页。迟到搜索隔离通过；第1000页、长文本、过期和未知提交重试使用明确标注的 UI 可控响应，不能当作后端证据。浏览器无未处理页面异常。
- 独立 Trellis 复核修复来源表单隐藏字段残留、第1000页越界导航、布尔配置范围不符，并通过输入边界、历史 JSON 字节快照和身份摘要绑定三组临时行为回归。生成的临时单元测试已清除；保留可重复的集成脚本。最终空套件 `go test ./...` 只计编译。
- Go vet、包编译、Windows 主服务与 worker 构建、Vue 严格 typecheck 和 Vite 生产构建通过；无 CGO 的 Linux amd64／arm64 主服务与 worker 交叉编译通过。Linux CPython 3.11 两种架构的依赖 wheel 解析与完整哈希下载通过，未在 Linux 运行依赖或二进制。最终 Windows 二进制的原回归 **core 52／collection 27／release 83 项**全部通过；core 包含 5 GiB 稀疏文件的偏移协议检查，不能代表真实性能。结果清单在 `.runtime/verify/baseline-20261125T011700441856Z-12dba1/run-manifest.json`。初次基线因缺少显式 FFmpeg 路径被标记 blocked，补齐参数后重新执行，没有把该次阻塞当作通过。

使用与启用步骤见 [JMComic 部署说明](jmcomic.md)。依赖与桥接证据、独立审查及浏览器复现见[后端验证](../.trellis/tasks/11-25-jmcomic-plugin/research/backend-validation.md)、[质量报告](../.trellis/tasks/11-25-jmcomic-plugin/research/quality-check.md)和[页面验收](../.trellis/tasks/11-25-jmcomic-plugin/research/browser-validation.md)。真实站点可访问性、接口变化、Docker 镜像构建／启动、NAS 权限与存储网络仍待目标环境验收。隔离浏览器与样例服务已关闭，不自动提交或归档。

## 搜种扩展验证（2026-11-18）

本轮新增可选 Torznab 适配器，通过已配置的 Prowlarr／Jackett 搜索，并复用原 BT 添加、选集、发布及五类入库。不增加 SQLite 迁移，不自动安装站点插件。**按用户要求，未执行任何 Docker 命令。**

### 真实应用、受控协议与原功能回归

- 最终 Windows 源码构建运行 `scripts/verify_torrent_search.py`，**43 项断言全部通过，exit 0**。实际运行 Go 应用、SQLite 和原下载协调器，Torznab／qB 为本机受控实现，不是实际索引站点或 BT 传输。
- 覆盖鉴权／同源、关键词和配置来源、类别参数、64 位体积、不同 XML namespace 前缀、未知元数据、来源部分失败／全部失败／成功空结果、10 秒超时、并发 429、2 MiB／DTD／畸形响应、100 条来源截断和身份去重。
- 覆盖浏览器／日志脱敏、密钥横跨标题截断边界、无 Cookie 转发、跨源／同源管理端点拒绝、跨源 HTTP 重定向拒绝、磁力重定向、无效种子及 8 MiB 上限。
- 搜索得到的 HTTP 种子经原文件选集、异步移动、发布后，**仅所选的一篇小说入库且读取原字节一致**。直接磁力与代理磁力同样先停在等待选集。重启丢失缓存后，已接受请求仍返回原任务且不再次取种；关闭搜种不访问来源，也不删除原下载和媒体。
- 针对复核发现的标题截断、无身份同名、可用来源替换及 hybrid 桥接／冲突，临时后端行为回归合计 **9 个顶层测试、30 个子用例通过**，执行后已按 AGENTS.md 清除。最终无测试文件的 `go test` 只计编译检查。
- 同一最终二进制重跑原 `scripts/verify_downloads.py`，**36 项断言通过**；原 `scripts/verify.py --large-file` **52 项断言通过**，保留五类内容、音乐、收藏进度、备份，以及 5,368,709,243 B 稀疏样本的大偏移 Range／HEAD／suffix／416／304。该稀疏样本仅证明协议。

可重复运行命令、隔离结果和边界见[搜种运行时验收](../.trellis/tasks/11-18-torrent-search/research/search-runtime-validation.md)，独立代码复核见[质量报告](../.trellis/tasks/11-18-torrent-search/research/search-quality-check.md)。

### 页面、构建与部署资料

- 真实 Go 服务提供生产页面，完成登录→下载中心→搜索→来源部分失败仍保留结果→体积排序→成功空结果／失败错误→选择种子→确认小说→原两文件清单→只选第一章并锁定。最终 API 为 `state=downloading`、`selectionLocked=true`、`ready=false`，两个复选框按锁定选择禁用。
- 恶意 HTML 样本标题按文本展示，没有生成图片节点。搜索、添加、详情切换时只有一个原生模态框；Esc 关闭搜索后焦点回到“搜种”，body overflow 恢复。
- 真实页面在 **375／768／1024／1440 px、明暗两主题**下无文档／弹窗横向溢出，手机底栏仍为五项。浏览器唯一控制台错误是主动测试全部来源失败产生的预期 HTTP 502，警告和未处理页面异常为 0。
- 前端可控 API 专项另覆盖 45 条结果的 20／20／5 分页、排序不重复联网、取消／迟到响应隔离、过期、未配置、下载器离线仍可搜但禁加、失败添加保持同一请求键。该检查不代替真实应用接口验证。
- 最终通过 strict vue-tsc、Vite 生产构建、`gofmt`、`go vet`、空套件 `go test`、Windows 构建，以及无 CGO 的 Linux amd64／arm64 交叉编译。Python 脚本编译、来源 JSON 和两份新 YAML 的 PyYAML 语法解析通过；YAML 解析不等于 Compose 合并和容器启动。
- Trellis implement／check 引用与最终 diff 检查通过，CodeGraph 已同步，任务进入 review；本轮新增临时 Go 单测文件已清除。
- 部署提供已有索引服务接入及可选 LinuxServer Jackett 覆盖文件；镜像固定版本／多架构摘要仅经注册表 HTTP 元数据核验。来源配置字段、密钥权限、反向代理、故障与停用步骤见[搜种部署文档](torrent-search.md)。

页面记录与截图位置见[真实页面验收](../.trellis/tasks/11-18-torrent-search/research/search-browser-acceptance.md)和[前端专项](../.trellis/tasks/11-18-torrent-search/research/search-frontend-validation.md)。

### 本轮手动验收边界

真实 Prowlarr／Jackett 与站点账号搜索、来源类别支持／下载代理兼容、Docker 构建启动、NAS UID／ACL／secret 权限和网络，以及真实大影片下载同时播放的吞吐及恢复仍由用户验收。未将受控 fixture、交叉编译或注册表核验写成真实站点／容器成功。所有本轮隔离服务与浏览器已关闭；未自动 Git 提交或归档。

## BT 下载扩展验证

本轮完成可选 qBittorrent 集成、下载中心、安全发布门禁及五类内容自动入库。SQLite 由 v1 增量迁移为 v2；原 `/media` 保持只读，下载数据由独立引擎写入专用卷。**按用户要求，本轮未执行任何 Docker 命令。**

### 真实应用与真实下载器

在 Windows 本机解包官方 qBittorrent **5.2.4 / WebAPI 2.15.1**，使用隔离双实例与唯一回环 Peer。真实应用联调结果 `passed=true`：

- 认证实际返回 HTTP 204、空正文与 `QBT_SID_<WebUI端口>`；生产 Go 适配器已保存并回送实际受限名称，同时兼容旧 `SID`。失败状态、失败正文、空／任意名称 Cookie 仍拒绝。
- 从应用提交本机磁力，取得完整元数据后引擎为 `stoppedDL`，下载载荷 **0 B**，`ready=false`。
- 确认只选择一篇 65,536 B 小说后，实际完成 BT 传输、移动到 complete、`ready=true`、`imported`，API 原字节与种源一致，只有一个内容条目，任务仍做种。
- 未选篇的 priority 为 0，API progress 曾为 0.5，仍未入库。独立 2 MiB 协议探针也观察到未选文件分片进度；这印证了必须按锁定清单发布，不能把目录里所有实体都开放。
- 独立协议探针还验证停止添加种子、移动结束与 `deleteFiles=false` 保留文件移除。首次不收敛的探针如实保留失败记录，未放松完成判据。

真实应用结果见 [qb-5.2.4-app-probe.json](../.trellis/tasks/11-17-bt-download/research/qb-5.2.4-app-probe.json)，来源、重现步骤及协议样本见[实施探针](../.trellis/tasks/11-17-bt-download/research/bt-implementation-probes.md)。该实测限 v1 BT；纯 v2／hybrid 只完成解析与身份编排验证。独立官方 Linux 镜像已固定版本和摘要，但没有拉取或运行容器。

### 编排、门禁与旧功能

- 可复用 `scripts/verify_downloads.py` 使用真实应用、SQLite 和可控假 qB，最终 HTTP 204／新 Cookie 名称轮 **36 项断言全部通过，exit 0**。覆盖认证／同源、上传与 JSON 限额、路径、身份去重、外部冲突、文件选择与版本、任务动作／限速、异步移动、五类入库、目录漫画未选页、SID 过期、离线读取、应用重启、入库失败重试、提交响应中断后对账及保留文件移除。生产 60 秒故障退避未缩短；该脚本不证明真实 BT。
- 临时后端行为回归验证解析／身份、命令恢复、迟到清单和绑定的条件修订、ready 在元数据／校验／空清单前冻结、请求限额及脱敏。认证补充 **14 个子项**验证 HTTP 200／204、受限会话名称、合法端口与下一次实际请求回送。
- 主代理另完成 **4 组门禁／迁移回归**，独立 reviewer 再次通过：staging／未知／未选／归档／关闭功能、尺寸／mtime／版本变化、v1→v2 保留收藏标签和 12 秒进度，以及合成 Linux 深层挂载别名。真实 Linux bind mount 尚未执行。
- 用最终 Cookie 修复后的 Windows 构建重启原隔离数据目录，`scripts/verify.py --large-file` 再次 **52 项全通过**。包括五类媒体、真实音乐格式／时长／封面、收藏进度、TXT／CBZ／EPUB、备份、鉴权和 5,368,709,243 B 稀疏样本的大偏移 Range／HEAD／suffix／416／304。
- 备份独立查询为 `user_version=2`、`integrity_check=ok`，含 `downloads`、`download_files`、`download_operations`。临时迁移回归两次打开旧库后保留原音乐及个人状态；降级需还原匹配的 v1 快照。

可重复的原媒体回归命令与临测范围见[运行时回归记录](../.trellis/tasks/11-17-bt-download/research/bt-runtime-validation.md)，独立复核见[质量报告](../.trellis/tasks/11-17-bt-download/research/bt-quality-check.md)。按项目要求，新增临时 Go 单测已清除；最终 `go test ./...` 的空套件结果仅计编译，不计行为覆盖。

### 页面与构建

- 前端可控接口验收：20,000 文件／200 页完整加载后才可确认；非连续实际索引提交为 114,541 B，低于 256 KiB；检查幂等重试、版本过期、轮询无重叠、隐藏页暂停、搜索竞态、上传 boundary 与焦点恢复。
- 实际 Go 页面验收：磁力添加、只选第一章及锁定、任务限速、暂停、离线历史／未知速率、恢复连接；已入库小说跳入原 TXT 阅读器并恢复 12/20 进度。
- 修复并复验未登录直接 `/#downloads`：完整刷新只请求 `/api/status` 200，无过早私有请求和 401；登录后恢复下载页。关闭模块时仍读取历史任务，写操作禁用。
- 375／768／1024／1440 px、两种主题无横向溢出；手机底栏五项，下载中心位于“更多”。最终页面控制台错误／警告为 0。浏览器证据及截图位置见[页面验收](../.trellis/tasks/11-17-bt-download/research/bt-browser-acceptance.md)。
- 最终源码通过 `gofmt`、`go vet ./...`、`go test ./...`、Windows 构建及无 CGO 的 Linux amd64／arm64 交叉编译；前端 strict vue-tsc 与 Vite 生产构建通过。
- Python 验证／真实探针脚本语法、三份 YAML 的 PyYAML 语法解析、Trellis implement／check 引用及 `git diff --check` 通过，CodeGraph 已同步。YAML 解析不等于 Compose 合并或容器启动。

### 本轮边界与手动验收

Docker 构建／启动／重启、官方 Linux 镜像初始化、目标 NAS UID／ACL／secret 权限、Linux 实际 bind mount、真实 pure v2／hybrid BT、空间耗尽／挂载失联／引擎或 NAS 掉电恢复，以及真实 50–100 GiB 影片下载同时播放的起播／拖动／吞吐／峰值资源，仍待目标环境验证。稀疏文件只能证明协议，回环小样本只能证明对应流程。步骤见 [BT 部署文档](downloads.md)。

本轮不自动 Git 提交或归档；隔离浏览器与原媒体服务已停止，真实 qB 探针通过自身 finally 清理应用与双 qB 进程。下载集成脚本同样在结束后释放自己的进程。

## 音乐扩展验证

本轮在原四类型版本上增加音乐，复用鉴权文件流、个人状态与 SQLite schema v1，没有新增部署服务、环境参数或数据库迁移。以下为本轮实际执行结果；后面的首版章节保留原四类型的历史基线。

### 接口与旧库兼容

- `scripts/verify.py` **52 项断言全部通过**：生成真实的 12 秒音频样本，覆盖 MP3（含大写后缀）、M4A、AAC、FLAC、OGG、OPUS、WAV、AIF、AIFF 的扫描、稳定 MIME、时长和原文件读取。MP3 内嵌封面成功提取，无封面音频以 `no_cover` 正常完成，重扫不重复执行无封面探测。
- 覆盖音乐类型过滤、统计、鉴权、HEAD、Range 精确字节和 416，以及中文标签搜索、收藏、秒数进度、重复扫描 ID 与个人状态保留、源文件不改写。原四类型的集成与数据库快照完整性同时通过。
- 保留 **5,368,709,243 字节**稀疏样本的 64 位协议回归，偏移 0、4,294,967,313、5,368,709,179 均返回精确字节。这是 HTTP 协议验证，不是实际大影片播放或性能基准。
- **旧库兼容 9 项断言通过**：复制首版四类型快照，在 4 个媒体库、9 个内容、2 条个人状态、6 条进度的基础上新增音乐；逐行比较原用户、媒体库、内容、收藏／标签和进度记录均保留，`integrity_check=ok`，`user_version=1`。重启后会话、音乐收藏和 4 秒进度仍保留。
- 后端临时行为回归覆盖缺少工具、损坏音频、取消、空库统计、占位图标题转义和重扫；执行后已清除本轮临时单元测试文件。保留可重复执行的接口集成脚本。

本轮接口命令：

```powershell
python -X utf8 scripts/verify.py --root .runtime/music/verify/media --large-file --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe
```

该 FFmpeg 路径为本机工具位置。脚本仅适用于隔离验证实例，不能用真实业务实例或媒体目录作为样本目录。

### 真实页面与播放边界

- 在生产构建页面新建音乐库，浏览挂载子目录，完成扫描、音乐列表、详情、中文标签、收藏和标签搜索的真实 UI 流程；新增的 WAV 样本显示 12 秒时长。
- 原生 `audio` 使用 `preload="metadata"` 与鉴权原文件 URL，实际发出 `bytes=0-` 和 `bytes=524288-` 请求；未使用整首音频 Blob。真实播放时间前进，暂停／seek 保存到 6 秒，关闭再打开恢复到 6 秒。关闭时旧元素已暂停且 `src=null`。
- 独立慢请求验证：1500 ms 延迟下连续 seek 2→4→6→8 秒，只发送 2 秒与最终 8 秒，正常写入最大并发为 1，关闭后数据库为 8 秒。保存队列仅保留一条在途和一个最新待写位置，关闭同步释放媒体。
- `pagehide` 在旧的 3 秒请求未完成时立即发出 7 秒 keepalive，队尾再次提交最新位置，隔离样本最终为 7 秒。页面卸载同步属于尽力而为；没有服务端顺序字段，不能据此宣称请求必达或严格顺序保证。
- 元信息加载前关闭时 `readyState=0`，未提交进度，原 7 秒记录保留。真实 404 媒体请求触发播放错误后，中文格式／编码与转码边界说明、重试、下载入口可见，关闭也未写入零进度。
- 独立视频回归真实播放并保存到 2.355064 秒，重开时原生 `video` 的 `readyState=4`，恢复到相同位置，关闭立即释放 `src`。
- 音乐列表、播放器及首页在明暗两主题和 **375、768、1024、1440 px** 下均无横向溢出；375 px 音频控件宽 327 px，五类型建库选择区无溢出。手机底栏五项，更多菜单可访问漫画、小说与管理，Esc 关闭后焦点回到更多按钮。
- 原图片真实加载、CBZ 下一页、TXT 正文、EPUB 排版与翻页已冒烟回归；EPUB iframe 仍为 `sandbox="allow-same-origin"`，脚本数为 0，关闭后 iframe 被移除。

截图保存在被 Git 忽略的 `.runtime/music/player-desktop.png`、`player-dark-375.png`、`player-light-375.png`、`library-mobile.png`、`library-desktop-dark.png`。验证使用本机 Chromium；不同浏览器对音频封装／编码的支持及物理 NAS 的吞吐尚待目标环境确认。音频转码、歌单、歌词和全局后台播放器不属于本轮功能。

### 构建与收尾

最终源码通过 `gofmt`、`go vet ./...`、`go test ./...`、Windows 构建及 `CGO_ENABLED=0` 的 Linux amd64／arm64 交叉编译；前端严格类型检查和 Vite 生产构建通过。临时单元测试清除后的 `go test` 仅为包编译检查，行为覆盖来自上述集成和浏览器验证。`scripts/verify.py` 语法检查、`git diff --check` 和 Trellis 任务上下文校验通过，CodeGraph 已同步。

本轮没有运行 Docker，部署由用户手动验证；不能将历史 Compose 校验或交叉编译当成本轮容器运行验证。本机隔离测试服务和浏览器会话在验证完成后关闭，测试端口已释放。

## 首版历史基线

## 环境与样本

- Windows 本机，Node.js 22.14.0、npm 10.9.2、Go 1.27.4；FFmpeg/ffprobe 可用。
- Go 程序直接提供生产构建的 Vue 页面，主验证地址 `127.0.0.1:8090`。
- 所有媒体、数据库、随机验证凭据和截图隔离在被 Git 忽略的 `.runtime/`；不是用户真实媒体库。
- 样本包含真实 4 秒 H.264 MP4、两张 PNG、图片目录漫画、CBZ、UTF-8/GB18030 TXT、无 DRM EPUB。
- 大文件协议样本是 **5,368,709,243 字节**的稀疏文件；它没有有效视频内容，预期无法探测或播放。

## 构建与静态检查

| 检查 | 结果与范围 |
| --- | --- |
| `go fmt ./...`、`go vet ./...` | 通过 |
| Windows 原生构建 | 通过；最新源码可运行 |
| `CGO_ENABLED=0` Linux amd64、arm64 交叉编译 | 两种架构均通过；不代表目标容器已运行 |
| `npm --prefix web run typecheck` | 通过，严格 TypeScript/Vue 检查 |
| `npm --prefix web run build` | 通过，包含本地动态加载的 EPUB 阅读组件 |
| `npm --prefix web audit` | 全量依赖检查报告 0 个已知漏洞；这是当日结果，不替代持续更新 |
| `python -m py_compile scripts/verify.py` | 通过 |
| `docker compose config --quiet` | 通过，单服务配置有效 |
| Trellis 任务上下文校验 | implement/check 引用均有效，已沉淀前后端实际契约 |

按项目要求，新增临时单元回归文件在执行后清除。因此最终 `go test ./...` 输出为 `[no test files]`；该命令仅确认当前包可编译，不将空测试套件计作行为覆盖。可重复运行的接口集成脚本保留在 `scripts/verify.py`。

## 接口集成：33 项断言

对最终后端重跑以下命令，**33 项全部通过**；独立质量复核实例也完成同组断言：

```powershell
python -X utf8 scripts/verify.py --large-file --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe
```

FFmpeg 路径为本机验证路径，其他环境应改为实际位置。脚本会生成／更新隔离样本；请勿对真实业务实例运行。

覆盖：

- 健康检查、首次设置／登录、持久会话、退出、未登录 401、跨站写入 403。
- 授权根目录边界、四类扫描、真实条目入库、标签／收藏／进度写入及中文标签搜索。
- UTF-8 和 GB18030 解析、目录漫画与 CBZ 的真实 PNG 页面及 `1, 2, 10` 自然排序。
- 封面、EPUB 原文件读取、一致性备份及 SQLite `integrity_check=ok`。
- 大于 4 GiB 的 HEAD 长度且无正文，起始和偏移 **4,294,967,313**、**5,368,709,179** 的精确 Range 字节，Suffix Range、416、304 和媒体鉴权。

## 后端边界与回归

实现阶段的临时回归验证过重复扫描幂等、媒体离线／空挂载保留索引、源文件不删除、目录穿越／外部软链接拒绝、恶意 CBZ 路径、低剩余空间保护、运行任务启动恢复为 queued。

独立复核另执行四组回归各 10 次，以及取消原子性回归 20 次，全部通过，修复并验证：

1. 任务取消、立即重试、迟到取消与旧执行器收尾的竞态。领取、句柄登记、取消、重试及收尾共享生命周期锁；旧执行器停止前返回 409，取消的任务不自动重新排队。
2. TXT 多空行样本的内存放大。逐行迭代替代全量 `strings.Split`；4 MiB 空行样本的解析总分配约 13.6 MiB，该数值属于特定回归样本。
3. 封面 URL 与缓存键均纳入文件大小和修改时间，保留 mtime 的大小变化不会复用旧版本。

真实 FFmpeg 样本已完成元数据探测并生成 11,648 字节封面；协议假视频的探测失败在任务页可见，未伪装成功。

## 浏览器流程与界面

已在自动化 Chromium 浏览器使用实际生产页面完成：

- 新数据目录创建管理员 → 空库引导 → 选择已挂载根／子目录 → 添加图片库 → 后台扫描出现两项真实内容 → 退出并重新登录。
- 全局中文标签搜索、详情收藏／标签编辑、媒体库目录浏览、任务状态与失败原因、设置中 FFmpeg 状态、下载数据库备份。
- 元信息接口模拟 503 时保留明确错误；随后内容列表成功不会将错误覆盖或显示“添加第一个媒体库”的虚假空库状态。解除故障后可恢复。
- 首页明暗主题均在 **375、768、1024、1440 px** 检查，均未发现横向溢出。移动端底部导航与阅读器布局已检查，漫画工具栏改成分行排列，方向／页码控件可操作。
- 图片原图实际加载，200% 缩放、相邻图片切换成功。
- 漫画实际页面加载，自然排序，单页／双页／连续模式、阅读方向、页码定位和关闭重开进度成功。
- GB18030 TXT 正文、章节跳转、字号／行距／纸色及关闭保存已检查，示例保存到第 62 / 124 段。
- EPUB 使用二进制输入实际排版，目录／字号／纸色可用；前后翻页可移动视口。示例 CFI `epubcfi(/6/2!/4/44/1:0)` 保存后，关闭重开的水平阅读位置均为 1760 px。
- EPUB iframe 为 `sandbox="allow-same-origin"`，不开放脚本；检查到脚本数为 0，并具有禁止脚本、外部连接、嵌套 frame、表单提交的 CSP。

视频专项：真实 MP4 元信息为 4 秒、640×360。原生 video 使用 `preload="metadata"` 与直接文件 URL；浏览器实际发出 `bytes=0-`、`bytes=196608-`、`bytes=360448-` 请求。播放时间前进、暂停／seek、保存 1 秒并重开恢复成功。关闭后 DOM 无 video，旧元素 `src=null` 且暂停，没有整片 Blob 下载链路。

本机截图保留在 `.runtime/verify/home-light.png`、`home-dark-final.png`、`comic-mobile-final.png`，不纳入源代码提交。窗口宽度模拟不是物理手机、Safari 或电视客户端的兼容性认证。

## EPUB 资源边界

独立复核复现过两项库默认行为：伪造 EOCD 为 1 个条目时 JSZip 仍读取 5001 个；伪造解压大小时 JSZip 在实际输出后才报告大小不符。现已增加结构和真实流式解压预检，不能仅依赖 JSZip 的解析成功或最终错误作为资源限制。

新增的 **37 项临时回归全部通过并清理**，包括合法普通／数据描述符 ZIP、5000 边界、5001 伪造计数、截断及变量越界、ZIP64、分卷、加密、本地头／目录不一致、重叠、EOCD 注释、声明及实际大小限制、损坏 DEFLATE、开始前／进行中取消、缺失安全解压能力，以及 HTTP 401 触发会话失效事件。

实际预检按最多 1 KiB 压缩输入流式解压，只计数输出，超出声明、32 MiB 单项或 256 MiB 总量即取消。之后才交给 EPUB 组件。独立复核再次重放 5 项均通过：伪造计数、伪造实际解压大小、合法 DEFLATE、预取消、不支持原生能力；伪造大小样本在首个 16 KiB 输出块后即取消。最终构建已在浏览器复验合法 EPUB；不支持原生 raw-DEFLATE 解压的浏览器会得到明确提示。

## 重启、备份与恢复

1. 停止旧主实例，用最终复核后的二进制及相同数据目录重新启动。原浏览器会话有效，图片收藏／中文标签、视频 1 秒进度仍保留。
2. 从设置页下载备份，对文件执行 `PRAGMA integrity_check`，结果 `ok`；包含 4 个媒体库、9 个内容、2 条个人状态、6 条进度。
3. 将备份复制为全新 `.runtime/restore/data/library.db`，没有携带旧 WAL/SHM，也没有缓存；独立端口启动后可读取原收藏、标签和播放进度。
4. 对恢复实例重新扫描图片库，封面重新生成为 JPEG，收藏和标签保持不变。

这证明当前首版快照的离线恢复路径可用；未覆盖未来版本升级／降级迁移，也没有模拟物理磁盘故障。

## 用户手动部署与目标设备待验事项

- 原媒体交付阶段按当时要求仅由用户手动验证Docker；2026-11-18用户追加授权的BT引擎隔离远程验收见下节。此前原生测试实例已停止，8090/8091/8092测试端口已释放。
- Linux amd64／arm64交叉编译通过；本次另运行了Linux amd64测试容器，arm64及各厂商容器管理器仍需目标设备验收。
- 没有真实 100 GiB 影片、长时间 4K/HDR 或 NAS 网络吞吐／峰值 RSS 基准。稀疏样本只证明文件长度、偏移和 HTTP 字节语义；短 MP4 只证明基础浏览器播放流程。
- 未验证硬件转码、HLS、转封装、CBR、PDF、HEIC、RAW、DRM、多用户权限；这些不属于首版已交付功能。

目标 NAS 上建议补测：实际镜像启动及 UID/GID 读取权限、真实大影片起播与接近片尾 seek、数小时播放／网络中断恢复、并发播放下 RSS/CPU/磁盘/带宽、物理移动端浏览器。以真实硬件与样本结果决定性能阈值。

## 2026-11-18 BT引擎初始化实施验收

本轮按追加授权在独立数据、端口和资源上执行。实现包括可选私有Unix-socket助手、固定镜像模板、持久幂等任务/事件/恢复、随机永久凭据、运行时自动接入及来源选择、下载中心管理界面。默认基础部署仍不访问Docker；主库保持schema v2。

### 本地与跨平台检查

- `go vet ./...`、双命令构建、Linux amd64/arm64交叉构建通过；临时行为测试执行后删除，最终`go test ./...`无测试文件仅代表编译检查。
- 助手真实Linux行为测试11组通过（另含拉取安全3个子场景）：pwritev2兼容分支、严格清单、拉取流错误/真实层进度、创建响应丢失对账、归属矩阵、tmpfs、持久幂等/恢复、严格私有HTTP、原子凭据与目标UID访问。安装级flock另1组通过。
- 应用临时6组运行时测试及独立复核2组404/Key脱敏测试通过后删除。跨进程锁在真实Linux默认安全策略下通过。
- `scripts/verify_engine_bootstrap.py`35项断言通过：真实Go应用/SQLite/生产下载适配器、受控回环qB/Torznab；覆盖401/403/400/413/409、无助手unknown、managed空启动、重复清单不重登、0/9来源/显式选择、DTD拒绝、旧连接保留、重启和secret边界。
- 原`verify_downloads.py`36项、`verify_torrent_search.py`43项通过；两者是受控引擎集成，不代表真实BT传输。
- `verify.py --large-file`52项通过：五类真实样本/FFmpeg元数据与音频封面、鉴权/状态/快照、5GiB稀疏文件HEAD/大偏移Range/suffix/416/304；未将稀疏文件解释为解码或吞吐基准。
- 前端strict typecheck、生产构建、托管Compose合并校验和diff空白检查通过。浏览器45项断言通过，明暗主题×375/768/1024/1440共8组无横向溢出，含Tab/Esc/焦点/单模态、告知非预勾选、幂等重试/刷新恢复、任务日志隔离、来源上限和移动五项底栏。真实隔离应用验证认证和助手未启用；其他引擎状态/操作使用明确标记的fixture。证据见任务`research/ui-verification-result.json`及`verify-engine-ui.js`。

### 授权Linux Docker实测及限制

测试宿主为Docker20.10.3/API1.41、x86_64、3.10.0-1127内核。先保存基线，使用随机任务标签与专用`/tmp`根；没有修改daemon配置、停动既有容器或执行全局prune。

- 实际构建并运行了应用/助手专属scratch验收镜像；主应用UID1000，经只读连接目录及Unix socket访问root助手。GET仅检测无创建副作用，缺失状态、告知拒绝、durable requestId幂等/目标冲突、失败重试及助手重启保留任务均通过。
- 应用真实生产qB适配器在内部bridge以容器DNS连接固定5.2.4，通过实际会话Cookie和版本验证。官方镜像离线PBKDF2永久密码首启及重启后仍可认证；未读取临时密码日志，也未关闭Host/CSRF校验。未添加任何BT载荷任务。
- Jackett固定GHCR镜像实际拉取成功。默认seccomp以EPERM阻断`pwritev2`，导致.NET无法写配置；同机临时放宽seccomp的**诊断容器**对该调用返回ENOSYS，运行时回退写入成功。产品增加拉取前兼容性预检并明确失败，未自动降低安全策略。
- 仅在上述诊断容器中进一步确认：空站点发现返回0，设置管理密码并持久化后，Torznab API Key发现仍正常，匿名管理访问被重定向到认证。真实已配置站点同步/关键词搜索/BT载荷未实测；0/9/选择来源分支由受控集成覆盖。
- qB官方镜像直接拉取受仓库网络超时限制。宿主已有阿里云加速；显式探测加速端点返回403，重试固定摘要仍失败，未更改全局源。为验证真实qB首启，通过本机原registry获取同一固定摘要，校验index/platform/config/每个压缩层和diffID后导入远程。Docker load不保留canonical RepoDigests，产品未放宽digest验证，因此该机助手完整自动拉取→两引擎成功闭环**尚未验证通过**。
- scratch验收镜像不等于完整生产Dockerfile及FFmpeg发行镜像构建。arm64仅交叉编译；真实站点、长时BT传输、目标NAS吞吐和百GiB影片性能仍需兼容环境补验。

脱敏远程证据保存在忽略目录`.runtime/engine-bootstrap/`；任务中另有独立代码审查记录。不应把测试期间的诊断容器当作正式部署配置。

### 清理核验

2026-11-18已完成远程精确清理：任务标签容器0、任务网络0、跟踪的测试镜像及非基线父层0，专用`/tmp`根已删除。清理以任务标签、精确ID、基线和引用共同验证，无force镜像删除、无全局prune。测试前的66个容器、7563个镜像及全部原网络仍保留；卷清单与基线一致，均为3354个。测试用明文随机凭据也已从本机忽略目录删除。完整脱敏结果见`.runtime/engine-bootstrap/cleanup-result.json`。

## 2026-11-18 Linux/Windows兼容与桌面启动验收

本节是独立的双平台实现任务，不能将上节旧远程Docker结果算作Windows Desktop验证。本轮未启动Docker、未安装或修改Docker/WSL系统配置、未连接旧远程daemon，也未创建容器、镜像、网络或卷；Linux运行检查仅使用已安装的WSL1发行版。Docker部署由用户手动验收。

### 共享运行时与业务回归

- Windows实际行为测试覆盖数据根跨进程独占、锁失败前不触碰数据库/恢复缓存、真实listener与端口占用、启动失败清理、本实例令牌就绪检查、有序关闭、内嵌动态资源、配置优先级和安全工具定位；新增临时单测运行后删除。
- 已安装的Ubuntu-22.04为WSL1，不是WSL2。Linux `/tmp`中的flock、跨进程排斥、符号链接别名、实际HTTP/SQLite/Ready及关闭、静态资源与工具查找测试通过。WSL1异常允许SO_REUSEADDR重复监听，故该环境的端口冲突子例明确排除；Windows实际冲突用例通过，未修改产品监听安全语义。
- 当前代码重新执行五媒体/真实FFmpeg与音频封面/5GiB稀疏Range回归52项、下载36项、搜种43项、引擎接入35项，全部通过。后面三组使用受控回环qB/Torznab，未查询真实站点或传输BT载荷。稀疏文件不代表影片解码、持续吞吐或百GiB性能基准。

### Windows桌面入口

真实GUI发布EXE以headless参数执行隔离集成检查：中文/空格/分号路径、任意工作目录、实际端口避让、内嵌网页和数据库就绪、真实当前用户命名管道、重复启动、跨profile共享数据根拒绝、退出、本机复启保留管理员及实例身份、重复JSON键拒绝且不覆盖配置均通过。原媒体样本字节保持不变。重启过程不需要重新构建或下载资源，但本轮没有禁用网卡来模拟物理断网。

原生“首次启动”对话框及中文提示已由Windows可访问性接口实际读到。桌面自动化工具随后出现截图捕获超时和输入几何不可用，未据此声称目录选择器/托盘交互全部验收。应用本身已正常就绪且配置保存，通过固定quit命令有序退出；日志确认安全停止。

### 命名卷与停止协议

Linux临时测试9项通过、1项因WSL1 fsuid写权限能力限制明确跳过。实际文件/SQLite/Unix socket覆盖固定六角色卷元数据、精确挂载、安装标记与数据库绑定、只读检测不认领、非空/外来/符号链接拒绝、任务持久性、固定stop-all幂等、真实镜像不符拒停、初始化冲突、停止后禁止新启动及异常停止响应对账；Docker侧是进程内受控传输。

WSL1会报告fsuid切换成功但拒绝目标身份写入，不能作为Docker Linux权限证据。产品仍执行真实写权限探针并拒绝失败，不自动放宽权限。实际Desktop/WSL2卷归属、UID/GID、qB/Jackett初始化与停止时序须人工部署验收。重新启动Compose也不表示动态引擎自动启动，需按页面现有“启动”动作恢复。

### 发布和目标环境边界

发布脚本支持Windows amd64、Linux amd64/arm64，内嵌网页、架构检查、双层SHA-256、固定镜像清单和工具许可证/对应源码材料门禁。发布边界回归52项通过，涵盖路径穿越、重复JSON/归档成员、大小写别名、保留清单覆盖、可变镜像、错误架构及完整材料核对。

最终重新生成的三份开发归档均通过逐包内外摘要与结构校验。Windows最终内嵌GUI包再次通过隔离headless集成；Linux amd64最终归档在WSL1的Linux临时目录实际解压、跨cwd启动、读取内嵌HTML/CSS/JS、检查SPA/脚本404并接收SIGTERM退出0。Linux ARM64仅交叉编译与包校验，未在ARM设备运行。

本轮三目标成品为明确标识的development验证包，缺少经过再分发审核的固定FFmpeg材料；不是完整离线发行版。本机用于真实媒体验证的工具是现有移动版本，不将它改名伪装为固定发布版本。正式工具清单、各架构动态依赖及许可证/源码审核仍由发行维护者提供。干净Windows11普通用户完整离线五媒体、完整Docker Desktop部署、ARM64实际运行和目标NAS性能均未完成验收。

最终全量质量与归档执行记录见`.trellis/tasks/11-18-windows-one-click/research/quality-review.md`和`research/implementation-result.md`。离线`docker compose config`仅证明模板可解析，不能代替部署验证。

## 2026-11-18 管理员密码规则首轮验收（后续已收紧字符范围）

本节保留用户追加“不可以包含中文”之前的实际结果。当时允许四类字符之外附加中文或空格；当前已禁止，以下含中文创建成功仅为历史事实。当前规则与验收以随后“禁止中文密码追加验收”为准。

新管理员密码保留 10–72 个 UTF-8 字节范围，并同时要求 ASCII 大写字母、小写字母、数字和可打印标点。前端创建表单与后端 `/api/setup` 一致；登录不套用新复杂度，不改写已有密码或数据库结构。

- 前后端共用的 53 组输入矩阵通过：9/10/72/73 字节、中文多字节边界、缺失四类之一、32 个 ASCII 标点、全角/中文/空格/表情不能代替必需类别，以及合法密码前后空格原样保留。
- 临时 Go 行为测试使用独立 SQLite 与真实本机 HTTP，验证无效 setup 返回 400 且不落库、有效 setup 返回 201 和 HttpOnly 会话、bcrypt 匹配未修改输入；存量弱密码登录成功、错误密码 401、重复设置 409 且原哈希不变、日志不含密码。独立复核重跑通过；临时测试源按项目要求执行后删除。
- 1000 个自动生成的隔离验证密码均满足新规则；五个脚本所有应用 setup 自动凭据复用同一生成方法，不改变无关 BT 引擎凭据。
- 浏览器连接隔离本机实例：弱密码在发出 setup 前被拦截，符合规则的含中文密码创建成功，退出后原密码登录成功。登录页不展示新建复杂度提示，较弱候选密码可正常提交到登录接口，由后端决定匹配结果。
- 创建表单在 375/768/1024/1440 宽度和深浅主题下的密码提示均无横向溢出，`aria-describedby` 正确关联。截图位于忽略目录 `.runtime/password-policy/setup-375-dark.png` 和 `setup-1440-light.png`。登录错误 401 和退出时在途任务轮询 401/中文提示为本轮观察到的预期错误路径。
- 使用本机已有 FFmpeg 执行 `python -X utf8 scripts/verify.py --large-file`，隔离五媒体/音乐/备份/5 GiB 稀疏 Range 回归 52 项通过。首次以系统 GBK 重定向输出时验证脚本在打印勾号处停止，指定 UTF-8 后完整通过；不涉及产品密码行为失败。
- Go 格式、vet、行为测试和构建，Vue 严格类型检查和生产构建，五个 Python 脚本语法检查均通过。本轮未执行 Docker 部署；稀疏文件仍仅证明 HTTP 偏移语义。隔离应用已停止，临时样本、数据库和凭据已清理。

详细复核记录见 `.trellis/tasks/11-18-password-policy/research/quality-review.md` 与 `research/implementation-result.md`。

## 2026-11-18 禁止中文密码追加验收

新建管理员密码仅允许 U+0021–U+007E：10–72 位英文字母、数字和英文标点，同时具备大写字母、小写字母、数字与英文特殊字符。不允许中文、全角字符、空格、控制字符或表情，即使已具备四类字符也拒绝附加非法字符；不 trim、不替换、不截断。前后端及提示一致，旧账户按原哈希登录。

- 前后端共用完整 71 组矩阵均通过，覆盖长度和类别、全部 32 个英文标点、中文含扩展汉字、全角、重音、空白/控制字符/表情，以及末尾 LF/CRLF。前端采用检索非法字符，避免 JavaScript `$` 对末尾换行的宽松匹配。
- 临时 Go 测试使用隔离 SQLite 与真实回环 HTTP：非法 setup 400 且无账户/会话落库，合法 setup 201 且有 HttpOnly 会话、bcrypt 匹配原样输入并可登录；已有弱密码和中文密码登录 200，错误密码 401，重复合法 setup 409 且原哈希不变，日志无密码。完整执行后已删除新增临时测试源。
- 独立复核对实际前端函数执行 65,553 项断言通过，包含全部 BMP 码元与补充 Unicode；Go 格式/vet、严格 Vue 类型检查、CLI 构建与前端生产构建通过。
- 浏览器实际连接隔离 CLI 与生产页面：中文/全角/表情在提交前被拦截，绕过前端直接提交中文返回 400 且仍需首次设置，合法 ASCII 创建 201、退出后原密码登录 200、再次退出 200，登录页没有新建复杂度提示。375/768/1024/1440 × 深浅主题 8 组无密码提示横向溢出，提示可访问关联正确；375 深色截图已人工查看。
- 浏览器已关闭，隔离实例已停止，数据库、锁及临时实例目录已删除；核验测试进程和 11774 监听均为 0。只保留忽略目录内矩阵、脱敏日志和截图。本轮未执行 Docker，也未重复首轮五媒体/5 GiB 稀疏 Range 的 52 项回归；不能将首轮结果记为此次重跑。

本轮详细记录见 `.trellis/tasks/11-18-password-policy/research/ascii-implementation-result.md` 与 `research/ascii-quality-review.md`。

## 2026-11-18 Windows 会话缓存清理脚本验收

新增固定入口 `scripts/clean-session-temp.ps1`，默认只预览并生成 `.runtime/session-temp-cleanup/manifest.json`；`-Execute` 只消费已有清单。Go 编译与依赖缓存分别显式选择，测试浏览器需要准确 profile 名称，默认仅选择带当前仓库路径边界标记的 Node 编译缓存。

- PowerShell 7.6.4 与 Windows PowerShell 5.1.19041.6456 的解析器均为 0 错误，脚本保留 UTF-8 BOM。实现验证在两版运行时中各通过 55 项隔离断言，独立复核各通过 34 项；所有可删除样本均位于 E 盘仓库自建 fixture，未将隔离结果当作真实 C 盘删除验收。
- 覆盖清单首次写入/原子覆盖、ISO 时间兼容、空清单、路径与目录边界、兄弟项目排除、字段/目标篡改、重解析点及祖先链接、活动进程与查询失败阻断、普通浏览器保留、只读/锁定文件、变更/新增/缺失文件、空目录、部分失败、重试和幂等。9 文件隔离场景首次准确记录 5 删除、1 缺失、2 变化保留、1 锁定失败，共 74 个已确认删除的逻辑字节；解锁重试后第三次运行删除 0 字节。
- 两版 PowerShell 真实默认只读预览均识别 139 个 Node 缓存文件、5,733,716 逻辑字节。最终全选预览匹配先前盘点：Go 编译 13,752 文件、模块 10,936 文件、校验 1 文件、指定浏览器 354 文件，加 Node 合计 **25,182 文件、1,735,601,734 逻辑字节，约 1.62 GiB**。精确绝对路径、长度与创建/修改时间保存在已忽略的清单中。随后读回全部计划文件：缺失 0、元数据变化 0，相关目标当前空闲，确认真实缓存仍保留。
- 独立复核修复了 PowerShell 将 `File.Replace` 的 `$null` 备份参数绑定为空字符串的问题，以及仓库路径子串误匹配同名前缀兄弟项目的问题。临时验证源已通过 E 盘精确单文件删除清除，隔离 fixture 与临时 JSON 发布文件均无残留。
- 本轮维护脚本没有改动 Go/Vue 应用和密码逻辑，未运行 Docker、全应用构建或 PSScriptAnalyzer；解析器检查不等于 PSScriptAnalyzer。README、专门用法文档和跨平台维护契约已同步。

先前真实 C 盘递归删除命令被工具自动审批拒绝，本轮没有改用脚本或其他 API 重试真实 C 盘删除。现有缓存仍保留；预览的 1.62 GiB 不是已回收空间。用户可审阅清单、关闭相关构建/测试浏览器后手动执行。详细实现和独立复核记录位于 `.trellis/tasks/11-18-session-temp-cleanup/research/`。

## 2026-11-18 项目内构建环境验收

新增 `scripts/project-env.json` 统一配置，默认将可控构建缓存和临时文件写入仓库 `.runtime/`。Python 通用入口、PowerShell 当前终端初始化、开发/发布/验证脚本和 CI Go 检查已接入；原有 `bin/`、`web/dist/` 等项目内产物布局保持一致。

- 实现环境回归 61 项、独立复核 49 项通过。覆盖配置边界、污染环境覆盖、不可写失败、真实临时文件及缓存重置、导入无副作用、参数/退出码、中文空格项目路径、junction 拒绝和外部哨兵保留。新增临时测试源及专属样本均已删除。
- PowerShell 5.1/7 实际解析和严格无错误点源通过。修复 Windows npm/npx 批处理隐式解释元字符的问题，改为原生 Node CLI；Windows 环境 JSON 不再包含仅大小写不同的重复键，解析失败立即终止。真实 Go、npm 和 Python 查询的受控目录均指向本仓库。
- `go vet ./...`、`go test ./...`、Windows CLI 构建、严格 Vue 类型检查和 Vite 生产构建通过。当前无保留 Go 单测，`go test` 仅作为包编译证据。首次官方 Go 代理请求超时后，仅验证子终端临时使用 `https://goproxy.cn` 完成检查，未改默认代理或校验配置。
- 修复后的发布边界回归 68 项通过；真实生成 Windows amd64、Linux amd64/arm64 三份开发归档并逐一通过架构、结构和内外摘要核验。归档在 `.runtime/project-env-validation/releases/`，复用已构建网页且不携带 FFmpeg，不能作为完整离线发行版。本轮 Linux 目标为交叉构建与归档核验，没有在 Linux/ARM 实机执行，也没有运行 Docker。
- Git Bash 的 Shell 解析、外部工作目录调用和相对参数保真检查通过，未扩大为 Linux 实机证据。CodeGraph 已同步，Trellis 上下文与 Git 空白检查通过；发布锁和嵌入暂存已清理。

配置只影响初始化进程及其后续子进程，不更改用户/系统环境，不迁移或清理旧 C 盘缓存。已启动的 IDE/MCP 需在配置后重新启动；Go 工具遥测、操作系统、安装器、Docker/WSL 存储和显式外部输出不在绝对隔离承诺内。具体用法见 `docs/project-build-environment.md`，实现和复核证据见 `.trellis/tasks/11-18-project-local-runtime/research/`。

## 2026-11-18 Windows 完整退出与程序文件释放验收

本轮以构建后主 EXE、自有媒体工具映像及程序目录能否立即替换为首要验收。修复 Windows 媒体工具后代进程残留、活动 HTTP 请求未完成收尾、命名管道异步内存未固定导致跨进程控制超时，以及正常取消可能掩盖聚合清理错误的问题。没有改动前端业务界面，没有运行 Docker 或连接旧远程服务器。

- 使用最终代码构建 Windows GUI 子系统 EXE，显式加载已有 `web/dist`，以独立中文程序目录、profile 和真实命名管道运行 `scripts/verify_desktop.py`。5 GiB 稀疏慢流退出 **0.775 秒**、空闲退出 **0.336 秒**；这是本轮实测值，不是所有设备的耗时保证。
- 准确等待主进程后，首先对 EXE 零共享读写、原位重命名、实际覆盖相同路径并校验 SHA-256，再重命名/恢复整个隔离程序目录。没有先等待负向 IPC 或人为休眠。覆盖后的 EXE 立即以同一配置启动，账号、实例身份与端口保持。HTTP/IPC 消失，数据库、锁、日志和源文件释放也通过检查。
- 临时原生行为测试覆盖挂起启动并归入私有 Job、退出前关闭新进程准入、准确进程 HANDLE 等待、父工具仍运行/已结束、并发派生、继承管道、独立进程保留与工具 EXE 立即独占/改名/覆盖；另覆盖请求接纳竞争、启动取消、重复退出、强制 GC 下独立短命 IPC 客户端和原生托盘消息泵/产品菜单派发。没有按 EXE 名称通杀进程。
- 最终 GUI EXE 重跑五媒体/音乐/快照/大偏移 Range **52 项**通过，随后 `quit` 有序退出码为 0、耗时 **0.274 秒**，主 EXE 与全部源文件等 **25 项**独占检查通过。下载编排 **36 项**、搜种 **43 项**、托管接入 **35 项**亦通过；这三组使用受控回环协议替身，不代表真实站点、BT 载荷或 Docker 部署验证。
- 独立 Windows 文件 fixture **24 项**、gofmt、vet、完整临时 Go 行为测试、Windows 构建及 Linux amd64/arm64 交叉构建通过。新增三个临时 Go 测试源执行后已删除；删后空测试套件仅作为编译证据。Trellis 校验、CodeGraph 同步通过。Race detector 因本机缺少 GCC 未执行，Linux 本轮没有实机运行。

修复前故障、中间失败、fixture 本身的 PID 发布/启动预算修正和最终通过记录均保留在 `.trellis/tasks/11-18-windows-complete-exit/research/`；最终集成命令及边界见 `integration-validation.md`。本轮没有物理鼠标点击系统托盘，也未用完整发行包验证第三方共享 DLL 加载/卸载；普通附加文件和工具 fixture 不能替代这一验收。稀疏文件不代表真实超大影片解码或吞吐性能。

只读枚举确认本轮隔离应用进程为 0，桌面脚本临时目录正常回收，新增临时 Go 测试源已清除。样本目录 `E:\vscodeProjects\nas-content-manager\.runtime\windows-exit\verify` 的递归删除被工具自动审批策略拒绝，未改用其他方式重试；该忽略目录仍含本轮自建样本、数据库/profile 和随机测试凭据，不能记为清理完成。脱敏日志和 JSON 结果已另存至其父目录。

## 2026-11-19 Windows desktop 品牌图标验收

桌面 EXE 预置十尺寸 ICO 对应的 Windows amd64 资源，托盘和窗口加载独立大小品牌图标。普通构建与原有 Windows 发布脚本自动内嵌；图标源和再生说明在 `assets/windows/`。

- `generate_windows_icon.py --check` 与 `go generate ./cmd/nas-desktop` 一致性检查通过。
- 普通 GUI 构建和使用原有发布脚本生成的内嵌网页开发包，均运行 `scripts/verify_desktop.py --with-tray` 通过。新增检查将 EXE 内十帧资源与源 ICO 逐字节比较，检查真实托盘窗口的独立大小图标句柄，并在原位覆盖重启后再次检查。
- 两次集成保留全部原有文件释放断言；退出后先独占打开并实际覆盖 EXE、校验摘要，再重命名程序目录和同配置立即重启。内嵌开发包本机慢流/空闲退出分别为 0.092/0.030 秒；此为本机实测，不是通用时限承诺。
- gofmt、Go vet/包编译、Linux amd64/arm64 交叉编译和发布边界 68 项通过。Trellis 上下文、CodeGraph 同步、Python 语法及 Git 空白检查通过。没有新增临时单测源；本轮隔离应用进程与临时 profile/生成目录均无残留。

可复用验证示例（在仓库根运行，二进制路径替换为本轮成品）：

```powershell
# 发布包已内嵌网页；普通开发构建另加 --web-dir web/dist。
python -B scripts/project_env.py -- python -B scripts/verify_desktop.py --binary 'path/to/栖库.exe' --with-tray
```

本轮没有物理鼠标点击或 Explorer 截图验收，没有重启 Explorer/清理图标缓存，没有执行 GitHub 云端 Actions、Docker 或 Linux 实机测试。开发包复用已有网页且不含 FFmpeg，不能称为完整离线工具包。详细命令、产物摘要与边界见 `.trellis/tasks/11-19-windows-desktop-icon/research/implementation-validation.md`；本轮未处理上节记录的旧样本残留。

## 2026-11-20 多来源资源收集实施验收

新增来源、RSS订阅与候选、统一获取任务、受控执行端、网页视频/提取音频、本地登记/复制和remote复制；复用原BT选文件、发布与扫描。获取和入库分别持久化，schema v3保留旧数据，秘密独立保存。部署及参数说明见 `docs/collection.md`。

- 最终多来源真实app+worker集成32项通过，使用真实rclone v1.76.0本地alias remote，覆盖RSS/受控qB桥接、登记/复制、精确Range、幂等、新编号重复409、秘密分离、重启、worker离线读取及BT删库关联撤销和文件保留。
- 真实yt-dlp 2025.12.08与FFmpeg执行隔离1秒HTTP fixture：MP4 9263字节、MP3 2235字节完整获取、发布、实际扫描入库；媒体接口鉴权206逐字节匹配，无会话401。未放宽生产代理策略，不能称为公网站点验证。
- 真实Edge/Playwright36项通过，375/768/1024/1440及两主题、焦点/Escape、来源与订阅、实际本地入库播放、RSS到原下载选文件、响应丢失原请求重试、批次部分失败和独立列表错误。截图检查后补齐弹窗内边距，再构建复验；预期注入的中断/422/503与未处理脚本错误分别记录。
- 最终后端累计19项临时行为用例通过（自动规则另含6子场景），覆盖历史窗口、版本/目标冲突、不确定派发对账、独立入库重试、RSS编辑、媒体库删除和256项轮转、已完成BT状态镜像；worker和桌面临时行为矩阵也已通过。新增临时Go测试执行后按要求删除，保留两份可复用集成脚本。
- 原下载36项、搜种43项、托管接入35项、五媒体/音乐/备份/5GiB稀疏Range52项及发行边界76项通过。Go vet、清理后包编译、严格Vue类型检查和Vite生产构建通过；Linux amd64/arm64、Windows amd64三程序及Windows桌面入口交叉构建通过。稀疏文件只证明协议，不代表媒体性能。

真实Compose CLI规范化、固定v1/v2清单和实际inspect数据校验矩阵通过；当前Docker daemon不可用，没有构建/运行worker镜像、三容器完整停止、目标NAS权限/跨卷/网络卷、真实云盘账号或公网BT载荷验收。公网样例DNS返回`198.18.1.164`保留地址，正确被代理拒绝；固定容器yt-dlp 2026.10.26仍待目标环境验证。Linux是交叉编译证据，未在Linux内核上执行进程组和权限测试。

详细命令、证据路径、最终修订及未覆盖条件保存在 `.trellis/tasks/11-19-resource-collection-plan/research/runtime-validation.md`、`browser-acceptance.md`、`backend-verification.md`、`worker-tool-versions.md`、`quality-review.md`。没有连接真实用户来源、部署、提交或归档任务。

## 2026-11-20 外部自动化与 MoviePilot 六项能力验收

新增 MoviePilot、AutoBangumi、Sonarr/Radarr 的固定 API 适配器、私密连接、持久动作记录及单窗口管理。接入采用“选服务→填地址和凭据→检测连接”三步，高级完成目录按需展开。MoviePilot 3.2.8 支持集成搜索、订阅、下载/暂停/继续、单文件复制整理、元数据刮削及媒体库索引刷新；原外部入口与独立完成目录预览/扫描仍保留。

- 最终真实应用连接四个本地 HTTP 协议服务，`scripts/verify_external.py --workflow` **83 项通过**。覆盖固定鉴权/路径/载荷、schema3→4保留、UUID幂等与冲突、revision、重启、不确定提交/只读对账/人工确认、秘密不进入DTO/主库/备份/日志，以及完整六阶段流程。结果位于 `.runtime/verify/external-complete-20261120/result.json`。
- MoviePilot 专项覆盖私密资源快照、一次性选择、准确下载器/hash启停、预览无副作用、整理目标实际不覆盖策略及文件版本重读、策略/文件变化拒绝、刮削覆盖确认与部分失败不确定、固定索引同步任务、限制有副作用GET自动重放，以及越界路径、过期与未知版本拒绝。整理只确认外部受理/回执，未冒充实际媒体入库。
- 最终单次实际浏览器通过真实应用API和适配器调用本地协议服务，搜索下载、订阅、暂停/继续、整理、刮削、索引刷新均保留成功提交回执；真实revision409没有新增动作，真实上游503保留旧快照且可恢复。四尺寸×两主题无横向溢出，独立检查手机明色及桌面暗色截图，无页面脚本错误。证据为 `.runtime/external-app-ui-acceptance-evidence/report.json`，passed=true、errors=[]；不含网络拦截。另有 `.runtime/external-ui-evidence/report.json` 的界面fixture覆盖UUID恢复、人工未确认、焦点/Escape及错误分区，后者仅为呈现证据。
- 独立复核执行8项临时Go行为测试后清除，修复配置根重叠、空文件版本、路径大小写/盘符根、启停回执身份、未知调度状态及秘密文件失败清理。最终 `go vet ./...`、清理后 `go test ./...`、Windows主程序构建、严格Vue类型检查和Vite生产构建通过。清理后的Go测试仅为包编译证据。Linux amd64/arm64主程序交叉构建通过。
- 本轮已有收集app+worker回归27项、下载36项、搜种43项、托管接入35项、五媒体/音乐/备份/5GiB稀疏Range52项通过。回归使用本轮前段构建，随后修改局限于新外部工作流并由最终83项和浏览器验证覆盖；此轮收集回归未重跑真实rclone或yt-dlp。

可复用后端验收命令（预先构建主程序，根目录须不存在）：

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_external.py --binary .runtime/verify/nas-external-final.exe --root .runtime/verify/external-new-run --workflow
```

本轮没有连接真实用户实例、安装第三方套件或改动真实媒体。MoviePilot六阶段能力限定已核实的精确3.2.8版本，未知版本不会推断开放。索引刷新是从已配置媒体服务器同步到MoviePilot，不代表触发Emby/Plex扫盘；目标媒体服务器、插件和NAS权限仍需真实部署验收。Linux仅交叉构建，未运行Linux/Docker；稀疏Range不证明解码或吞吐性能。详细协议依据和质量记录位于 `.trellis/tasks/11-20-collection-external-api/research/`。

## 2026-11-20 密码显示与隐藏验收

登录、创建账户、确认密码及外部服务API凭据统一使用小眼睛按钮，默认隐藏，各字段独立切换；清空、重新打开及切换外部服务后恢复掩码。

- 真实Vue组件浏览器矩阵24场景通过：三类页面×375/768/1024/1440宽度×明暗主题，覆盖反复切换、原值与原生属性保持、独立状态、键盘和可见焦点、44px按钮、禁用、清空/重挂载、准确标签及提示关联、无溢出与无意外提交。
- 最终创建账户间距复验8组合通过，规则至确认标签间距均为22px；外部服务空值/非空值切换复验另8组合通过。独立375px登录抽样通过，最终手机浅色及桌面深色截图已目视检查。
- 最终 `python -B scripts/project_env.py -- npm --prefix web run build` 通过，包含严格Vue类型检查与Vite生产构建。临时测试源码已删除，浏览器已关闭；结果和截图位于 `.runtime/verify/password-visibility/`，详细记录位于 `.trellis/tasks/11-20-password-visibility/research/`。

上述浏览器检查在独立context拦截全部API，仅验证真实前端组件、原密码策略及提交值；没有向用户后端创建账户、登录或保存外部凭据，也不构成真实后端认证通过的证据。

## 2026-11-20 平台部署资源组件验收

新增 MoviePilot、AutoBangumi、Sonarr、Radarr 的固定Docker部署入口，嵌入原来源弹窗。Linux托管配置与Windows清单v3提供能力，旧v1/v2保留原部署边界；部署、组件首次配置、API认证接入和本地完成目录导入各自展示真实结果。使用说明见[组件部署](components.md)。

- 真实应用→生产私有Unix socket→隔离helper协议服务 **55项**通过：会话/来源/严格参数、GET无部署、四模板与授权、持久请求幂等、丢回执与应用重启、任务范围、层进度/游标、启动/失败重试、禁用/旧助手/离线和11秒慢响应。没有更换产品传输；Windows Python缺少AF_UNIX时用仓库内Go桥承接真正Unix socket。
- 浏览器 **30项**通过，未拦截API：实际来源按钮提交部署、层进度、刷新恢复、服务切换、503原UUID恢复、失败revision重试、首次配置入口与连接预填、空凭据默认掩码、离线仍可连接已有服务、键盘Space/Esc及焦点恢复。375/768/1024/1440×明暗共8组均为一个dialog/一个form，44px控件可达、无横向溢出；远程主机名只显示NAS端口和SSH指引。无pageerror，手机与桌面viewport截图已目视检查。证据 `.runtime/verify/cmp-1120c/browser-result.json`。
- 最新构建追加真实浏览器跨组件预填5项通过：MoviePilot→Sonarr清理旧自动入口，保留用户自填入口和手选完成目录，证据 `cmp-1120c/browser-prefill-result.json`。Linux `/collection-media/library` 自动目录由实际Vue SFC setup专项23项覆盖；后者是无DOM组件逻辑证据。
- helper六组临时行为测试与app五组代理测试通过；四组件×bind/volume模板、私密卷nonce/外来资源拒绝、GET无写、匿名探针、丢create响应对账、SQLite重开、六组件停止门禁及错误任务回执均有断言。最终SQLite专项补验锁内完整签名、不同revision无绑定及活动任务结束后不绕过状态校验新建任务。新增临时单测已清除，保留集成脚本 `scripts/verify_components.py`；浏览器、自有fixture进程和socket已回收。
- 真实Compose CLI离线规范化六组合通过：桌面v1、v2+worker、v3、v3+worker、Linux托管及托管+worker。48项桌面临时断言核验精确媒体根、只读/写入挂载、环境和角色身份；发布自检83项通过。离线规范化没有部署Docker服务。
- 本轮原功能回归：MoviePilot等外部API含六阶段 **83项**、收集app+worker **27项**、下载 **36项**、搜种 **43项**、托管接入 **35项**、五媒体/音乐/备份/5GiB稀疏Range **52项**全部通过。真实rclone、公网站点和BT载荷未在本轮重跑。
- 最终审查修复后的Go vet、清理单测后的全包编译、前端严格类型检查与生产构建通过；重新完成Windows amd64应用/桌面/助手，以及Linux amd64、arm64应用/助手交叉构建。CodeGraph同步成功。最终独立审查及补充质量检查见任务的 `research/quality-review.md`，最终协议55项证据为 `.runtime/verify/cmp-final/result.json`。

本轮未拉取或运行四个真实产品镜像、连接用户账号或修改真实媒体。Linux文件权限、Docker Desktop实际卷、首次初始化、目标NAS网络/UID/ACL、真实媒体服务器和掉电恢复仍需目标环境验收；交叉编译和协议fixture不能替代这些证据。详细命令及结果位于 `.trellis/tasks/11-20-collection-component-install/research/runtime-validation.md`。

## 2026-11-23 S01 可重放回归基线

新增有界编排 `scripts/verify_baseline.py`、长期编排自检、脚本矩阵及真实NAS验收档案模板。业务API、数据库和原有验证器在本步骤未改。使用方式见 [基线说明](baseline.md)，实际命令与产物见 `.trellis/tasks/11-23-nas-validation-baseline/implementation.md`。

- Windows本机使用同一构建实际运行两次core，各 **52项**通过，分别使用独立目录和端口；覆盖鉴权、五媒体、扫描/收藏标签进度、SQLite快照与5GiB稀疏Range。证据 `.runtime/verify/s01-core-1/`、`s01-core-2/`。稀疏文件只证明HTTP偏移，不能作为真实解码/吞吐/目标性能成绩。
- 实际领域组合 `.runtime/verify/s01-domain/run-manifest.json` 全部passed：下载 **36项**、固定外部协议与工作流 **83项**、发布边界 **83项**。前两项是实际应用与本地协议fixture；发布为既有self-test，不能冒充真实BT、第三方产品或正式发行归档验收。本次未选search/engines/components/collection，保留not_run，历史结果不并入本轮计数。
- 最终独立复核后编排自检 **39项**通过（`s01-review-selftest`），覆盖真实自有进程树/旁观进程、超时、端口冲突、非法/重复/链接及Windows路径别名、脱敏、结果分类。链接清理与证据保存的两次完整故障注入均实际exit1且清单failed/finishedAt正确，证据 `s01-review-finally-failure`、`s01-review-evidence-failure`；另 `s01-review-tool-cleanup` 核验真实工具exit0仍因清理错误整体failed/exit1并保留命令。这些是预期失败断言，不记产品通过。开发中的失败运行保留未覆盖。
- 本轮 `go vet ./...`、`go test ./...`、Windows主程序/worker构建、前端严格类型检查与生产构建通过。当前Go各包无保留单测，因此go test仅是包编译证据。所有命令经项目环境入口，可控缓存/临时目录位于仓库.runtime；未生成持久临时单测类。
- target模板实际只完成结构验证：`s01-target`中档案结构passed，M1 blocked、M2/M3 not_run，整体exit2。没有登录设备、导入通过声明或运行真实Docker。Linux实际权限/进程组、NAS卷与UID/ACL、真实媒体/浏览器性能、外部产品及隔离恢复/断电仍需目标证据；不阻塞S02本机实施。

公开证据限清单引用的脱敏日志/证据并需人工复核；不要上传含凭据和数据库的整个运行根。独立复核及调试复盘保存在S01的research目录。

## 2026-11-23 S02.1 账号迁移与凭据快照防线

S02仍在实施。本片增加schema5的账号修订、独立公开会话编号与有限客户端类别，兼容原密码/旧Cookie；登录使用bcrypt验证前的hash/revision快照条件建会话，避免旧验证线程在凭据更新后补建有效会话。在线改密、会话管理页面和离线重置尚未交付。实际设计和证据见 `.trellis/tasks/11-23-nas-account-lifecycle/`。

- 7个顶层临时Go行为测试通过，包含真正schema1/3/4增量迁移保留性、旧Cookie/重启、hash-only/revision-only/both确定性竞态、精确到期/同秒可变小数/纳秒时间、数据库约束、迁移失败回滚、新schema拒绝及单连接事务内会话重验。日志`.runtime/verify/s02-1-go-checks.log`，新增临时单测源已清除。
- 最终本机产物`.runtime/bin/s02-1-stable/nas.exe`（SHA256 `be86e7447505884e18ebacbc66d6f6fb3a85759e4f1a3970b5d3424e77a742f6`）完成账号 **43项**、受控外部API与真实schema3迁移 **84项**、核心五媒体/SQLite快照/5GiB稀疏Range **52项**回归。证据分别为`s02-1-account-final3`、`s02-1-external-4`、`s02-1-core-4`；外部协议fixture不代表真实产品实例，稀疏文件只证明HTTP偏移。
- 核心回归曾暴露存量Windows未读小正文连接重置。失败`core-1/core-3`和S01旧产物对照保留；最终统一security收尾覆盖成功202/logout及401/403/429，只在已知不超过64KiB且无Expect时有界消费剩余正文，一秒读取预算，保留原生Close语义和媒体无全局WriteTimeout。10个临时HTTP行为测试通过，覆盖重复真实短连接、部分/全部已读JSON、不完整/慢正文、keep-alive关闭与正常复用，以及大/分块/Expect跳过；日志`http-finish-go`、`http-keepalive-go`位于`.runtime/verify/s02-1-*`。临时源已删除。
- 本轮gofmt、Go vet、删除临时单测后的全包编译和Windows主程序构建通过；无保留单测的`go test`只记编译证据。集成输出经验证器脱敏，不能反推原始业务stdout不存在秘密；当前新增日志已源码核对。

独立复核另外6个顶层Go行为测试通过并清源，重建产物SHA与stable一致；新根`s02-1-review-account`、`s02-1-review-external`、`s02-1-review-core`分别43/84/52项通过，vet/全包编译/前端严格类型与生产构建通过。原始内存日志专项直接覆盖实际login/createSession路径的固定旧中文密码、bcrypt、bearer及SHA摘要、原始UA与清理错误标记，未使用持久脱敏器；不泛称所有stdout或未来接口均已动态验证。详见任务`research/quality-review.md`。S02.1无剩余确定问题，已进入S02.2；Windows本机结果与Linux/NAS实际锁、终端、权限和媒体性能验收分开记录。未操作真实账号、用户媒体、Docker或第三方工具安装。

## 2026-11-23 S02.2 在线账号安全与会话管理

设置页新增局部账号安全区域，支持原密码改密、实际当前会话与有界分页、指定会话撤销、再认证后退出其他会话。改密原子推进账号修订并撤销全部旧Cookie，要求重新登录；原弱/中文密码仍可作为原密码升级。S02.2本机实施与独立复核完成，S02.3停机离线重置继续实施中。

- 独立fresh产物`.runtime/bin/s02-2-review/nas.exe`，21,334,016字节，SHA256 `2d8c2ed03b85e9b4c1ddbcca87cda119df44f2f17cab726d509abcf056d3873a`。真实账号HTTP **75项**与核心五媒体/用户状态/SQLite快照/5GiB稀疏Range **52项**通过，证据为`s02-2-review-account`、`s02-2-review-core`；稀疏文件只证明HTTP偏移，不代表真实长媒体性能。
- 实施者9个顶层临时Go测试通过；独立复核另6个顶层行为测试覆盖双改密/CAS、撤销冲突优先级、事务失败/取消/精确到期回滚、修订溢出、原始行有界分页、双预算并发/容量、严格JSON/query与原始内存业务日志秘密检查。独立直接转译执行生产`password.ts` **54项**密码边界及32标点通过。本次生成的临时测试源码已删除；清源后全包`go test`仅是编译证据，不重复记作行为测试。
- 最终真实Edge/Playwright **138项**通过：**122真实浏览器、14受控呈现、2资产身份**，不将受控429/409/网络响应呈现算作实际后端执行。覆盖两个真实会话、DB独立核对当前身份/完整分页、400保留登录、各撤销与改密、秘密清理、迟到读取与未知写不重发、375/768/1024/1440×明暗、键盘/modal/Esc。结果`s02-2-review-browser-final`的cleanupFailed=false、frontendUnchanged=true；复制后前端资源清单SHA256为`3194b81659ed5e2a4bb045cea3460c243a6c362688c55387a14e2eee9759111a`。手机双主题、桌面暗色及两个确认框已目视检查。
- 复核真实复现“另一表单空密码框已显示时，同值清空不能恢复掩码”，以局部输入key重建修复，保留`s02-2-review-browser-empty-mask`失败根。另三类真实app+Edge故障注入证明含密码Playwright异常只记录类型、关闭失败整体failed且继续回收其余context/app，原始异常秘密不进入捕获输出/持久文本；预期failed运行不加进138项正常计数。浏览器原始内存秘密检查与已脱敏服务日志分别说明，未泛称扫描全部服务stdout。
- 账号验证器接入统一baseline的`account`组合；主线实际执行`s02-2-account-baseline` **75项**通过，其余8组not_run，不与上面的同类账号结果叠加计数。编排自检曾因固定单次心跳采样失败，诊断实测到活旁观者文件的暂时空值，改为有界有效心跳等待并保留回收断言；最终`s02-2-account-orchestrator-selftest-3` **41项**通过，历史失败和诊断保留。
- gofmt、Go vet、清源全包编译、Vue严格类型检查及生产构建、Python AST/导入/类型标注与局部空白检查通过。独立复核未发现剩余S02.2确定缺陷；报告为账号任务`research/quality-review-s02-2.md`、`browser-review.md`，编排记录为`baseline-account-integration.md`，保留S02.1旧报告。

本片没有离线CLI通过声明，未重跑的其它领域不并入本轮。真实Linux/NAS锁、终端、权限、容器及目标性能仍待相应环境独立验收。未操作用户实例/真实账号或媒体，未提交、部署或归档。

## 2026-11-23 S02.3 停机离线重置与账号最终本机验收

新增显式`nas account reset-password --data-dir <已有目录> [--password-stdin]`。维护分派先于正常配置/服务初始化；仅受支持schema5、原数据目录独占锁和唯一有效管理员可进入，短事务只更新hash/revision并删除旧会话。真实终端两次无回显确认，重定向必须显式旗标，错误/取消不启动业务。用法见[账号说明](accounts.md)，完整独立报告为`.trellis/tasks/11-23-nas-account-lifecycle/research/quality-review-s02-3.md`。

- 最终Windows产物`.runtime/bin/s02-3-review-http-final/nas.exe`，21,447,680字节，SHA256 `8bc6a1a144d86bd0a65e38bafc9f1388b21bf2545c9a29168d92ebdf0dbf135a`。同一产物独立账号**157项=在线83+离线74**、核心媒体**52项**通过，根分别为`s02-3-account-review-http-final`与`s02-3-core-review-http-final`。核心含五媒体、真实FFmpeg、状态/SQLite快照和5GiB稀疏Range；稀疏文件仅证明HTTP偏移，进程Job回收不是正常停服证明。
- 主线最后真实执行统一baseline的account组合，`.runtime/verify/s02-3-baseline-account-review-http-final/run-manifest.json`为passed；实际子结果157=83+74、offline requested/passed与覆盖标记均真、子进程正常exit0、相同产物SHA/大小，其余8组not_run。此轮重复账号不叠加到独立157项。编排自检44项、72个覆盖负例与变异检查见账号任务`research/baseline-account-integration.md`，不把负例重复计入44项。
- 独立复核修复bcrypt.Cost宽松预检：四个显式兼容版本、精确长度、两位ASCII成本与53字节bcrypt-base64先核对；尾缀/非法salt/digest/未知版本/带符号成本均拒绝且库不变。新增临时Go行为测试三批共九个顶层组通过并删除本轮源，清源全包`go test`只记编译。
- 保留旧候选57项后WinError10054的真实失败与30次定向29个413/1个RST对照。最终账号局部已知超64KiB分支以最多65,537实际字节、250ms局部deadline经真实MaxBytesReader收尾；共享小体defer、大体/未知/Expect skip和媒体超时不改。新产物100次独立短连接逐次413、逐次health200，无补发；分片、1TiB声明停滞、Expect首状态非100、chunked、提前EOF及精确64KiB通过。Go原生可能另加500ms半关闭等待，不能把局部预算说成整连接期限或任意网络必达保证。
- 真实Windows Console **11场景**通过，两提示分别覆盖KEY_EVENT Ctrl+C、系统CTRL_C_EVENT、Ctrl+D，另含确认不符、管道未EOF取消、终端加stdin旗标拒绝、32标点/退格和CRLF成功。模式准确503→1008→503；原始双流与可见viewport先扫描再保存安全证据，屏幕范围不扩称全滚动缓冲。真实ACL拒绝写和完整描述符长度/字节恢复通过；目录别名Junction同锁竞争通过，文件symlink实际创建返回WinError1314，仅记录替代覆盖。
- gofmt、Go vet、清源全包编译、严格Vue类型检查及Windows/Linux双架构构建通过；本片未改前端，不重复汇总历史浏览器138项。实际工具链**Go1.27.4 windows/amd64**，`go.mod`语言声明为**1.26.0**。Linux amd64产物SHA `509c94eabe0b4d166f5f0cf196f986d6ba6cd323ff4aad6d47517b9beead3a10`，arm64为`522c8cc2f71dab710d2c8fac70596f4b40e079b1c521b5b89aeb6166009bc5c2`；两者仅交叉编译，未执行。

S02按`local_complete_target_pending`记录本机完成。真实Linux/NAS锁、终端、权限、目标文件系统和部署门禁仍待验；事件接入由S04后续联调。所有本地命令经项目环境入口，未修改用户级环境、真实账号/媒体或Docker，未提交、部署或归档。

## 2026-11-23 S03.1 共用快照与独立只读检查

旧`GET /api/backup`已复用唯一快照租约：一次带context的VACUUM INTO、受限临时目录、精确成员与文件身份检查、只读SQLite结构/页长/完整性/外键核验及固定64KiB流式摘要，校验后使用同一只读句柄下载。GET/HEAD、原响应头及竞争409保持；schema仍为5，没有交付定时持久备份、恢复CLI或UI。正式契约见`.trellis/spec/backend/backup-contract.md`，完整独立报告为`.trellis/tasks/11-23-nas-backup-recovery/research/quality-review-s03-1.md`。

- 独立复核修复未知目录的无界`ReadDir(-1)`，改成最多读取两项。真实2048未知成员反例检查实测39次分配，清理拒绝后2049成员完整保留、锁释放；39不是跨平台固定承诺。最终临时Go行为测试**8顶层组/22子场景**通过，覆盖并发Close/竞争、身份替换、权限/Sync/Close/删除/空间故障、真实SQLite反例、预算与取消、64MiB真实生成中取消及Windows实际DACL/只读检查。30个pass事件不计为30条独立断言；两份本轮临时源码已精确删除，详细证据`s03-1-review-behavior-final`。
- 最终新Windows产物`.runtime/bin/s03-1-review-final/nas.exe`为**21,551,616字节**，SHA256 `49754b278cfb26c3725efbcf2f45d462fbda72617eb7442859f5a21e0b40e790`。该同一产物在全新隔离根独立完成备份**38项**、账号**157=在线83+离线74项**、核心媒体大文件**52项**。证据依次为`s03-1-review-backup-final/result.json`、`s03-1-review-account-final/result.json`、`s03-1-review-core-final/run-manifest.json`；core完成时间为原始UTC `2026-11-23T04:23:33.947+00:00`，其余8组not_run。实施者旧SHA `6915b06f8ab0f46bd4be43218fe1208f2e81e148471b2f4479ac4e4d35c815d7`的自查保留为历史，不与新SHA累加通过数。
- 备份专项有真实慢HTTP租约/第二请求409/断连清理重试、真实收藏进度事务及独立下载摘要核对；单连接VACUUM期间其它SQL可等待，不称真正并行写。自有隔离Console **3次CTRL_C_EVENT正常停服**，应用强制回收**0次**、`cleanupFailures=[]`，含慢下载中停止、DB/锁释放、原端口释放、同根立即重启和未知sentinel保留。原始应用双流秘密检查先于脱敏。core的OwnedProcess Job回收只记资源回收，不算正常停服证据；5GiB稀疏Range仅证明HTTP偏移。
- gofmt、vet、清源全包编译、Vue严格类型检查和Python AST通过。实际Go工具链为**1.27.4 windows/amd64**，语言声明1.26.0。Linux amd64交叉产物21,350,440字节、SHA `d79616cb6048eee9c08233f053e24b30a3e712c6a92ccd27017d828a04e23ea8`；arm64为20,060,219字节、SHA `899f2bffac5a5a2bbbacdee15cdfd89b0cdd81d8d88f8b8e7d3450a80622d834`。它们未执行；checks证据`s03-1-review-checks-final/result.json`。清源后的无测试全包go test只记编译。

S03.1按`local_complete_target_pending`记账，S03整体仍实施中。真实Linux/NAS权限、挂载/多卷、FULL/IOERR、拔盘/断电和大库性能待验。DB包含敏感索引、口令/会话哈希与私密引用，database-only不代表公开或整栈备份；本片不撤销快照旧会话，独立恢复准备仍属后续片。未操作用户实例、真实账号/媒体或Docker，未提交、部署或归档。

## 2026-11-23 S03.2.1 备份存储与离线维护兼容

正常启动新增 raw schemaV6 五张 STRICT 表及默认关闭的策略/未绑定store单例，同笔事务推进头版本6；重启保留storeId。没有创建持久备份根、启用策略或新增API、执行器、调度、轮转。当前快照checker只接受6；离线账号支持显式集合{5,6}，并要求事务实际user_version与预检完全相等。正式契约见`.trellis/spec/backend/backup-storage-contract.md`，独立报告为`.trellis/tasks/11-23-nas-backup-recovery/research/quality-review-s03-2-1.md`，SHA256 `0506957e6e2e1a461fc4e7362dbf40fda52c6fb9d27e80812f912b3e3211dded`。

- 独立检查用真实伪6快照复现隐藏生成列漏检，将`pragma_table_info`修为`pragma_table_xinfo`并逐列要求hidden=0、精确字段名/声明类型，保留STRICT和两单例检查。该探针不证明全部DDL约束/索引语义。最终Go行为**8顶层组、653通过叶子子场景、0失败**；658个带斜线pass事件含5个嵌套容器，不记为叶子或断言。证据`s0321-review-behavior-final`含实际事件与计数审计、原始秘密扫描通过；临时源按SHA核对后精确删除。
- fresh Windows产物`.runtime/bin/s0321-review-final-pass/nas.exe`，**21,581,824字节**，SHA256 `18a0a77fe0c10256e7eb406d0b07f64210901b8320012a78f2b02fc810a188cd`。同一产物完成storage **17项**、backup **38项**、account `--offline` **162=在线83+离线79项**、core `--large-file` **52项**；对应证据根为`s0321-review-storage-final`、`s0321-review-backup-final`、`s0321-review-account-final`、`s0321-review-core-final`。不与实施者自查或旧片相加。
- storage覆盖真实v5结构迁移、旧Cookie/业务保留、冲突全回滚、future7拒绝、默认不建根和重启身份稳定。离线维护在真实5/6中成功及双向版本漂移拒绝，即使schema cookie不变也不得写；非空备份域/授权/ownership和pending sentinel保留。13条真实Console记录的原始及可见屏幕秘密扫描、实际ACL写拒绝与完整恢复通过；不把可分享脱敏日志扩大成所有线上原始流的秘密证明。
- storage/backup分别**3次真实Console正常停服、0次活应用强制回收、cleanupFailures=[]**，原始日志先扫描后脱敏；core仍为OwnedProcess/Job回收（退出1、owned-tree-reclaimed），不算正常停服或泛称全部进程0次强制。最终两个本轮Windows产物确切路径活进程0、源码漂移[]、临时源0；实际总审计结束`2026-11-23T05:07:11.046614+00:00`。证据`.runtime/verify/s0321-review-checks-final-pass/final-audit.json`。storage/backup/account结果自身没有finishedAt，不从mtime补造。
- gofmt、Go vet、清源全包编译、Python AST/类型标注、Vue严格类型和局部空白检查通过，清源go test无测试只记编译。Go工具链1.27.4、语言声明1.26.0。新Linux amd64交编产物21,377,958字节、SHA `80018064e68585b27b6cfc1421dc506b4220dfcb0a2028da3e9b7e9ec5fb7ba1`；arm64为20,063,537字节、SHA `ac22498c66801c37d509c341d8cc15b667dde1829a4b10b0499f0289ca1caeb7`，均未运行。
- 失败历史保留且不累加：独立首轮`behavior-1`有extra-hidden叶子及其顶层失败（338通过叶子+5容器）；独立工程首轮误把git退出0的LF/CRLF stderr提示当失败，仅修runtime判定，在全新checks/artifacts根重跑。实施者storage首次因sqlite_sequence无序比较假失败，9项已完、2正常停服及**1次自有活应用强制回收**单列；core首次漏FFmpeg参数blocked、产品未运行。完整证据及自查旧SHA见独立报告，不覆盖旧根。

随机失败验证只证明可返回的注入Reader EOF/short可回滚；固定Go默认crypto/rand.Reader底层熵灾难可能runtime fatal，未动态验证其崩溃恢复，也不承诺受控返回。200运行/64所有权/2048有效键和跨表关系仍留后片服务。S03.2.1记`local_complete_target_pending`，S03整体未完；真实Linux/NAS、FULL/IOERR、拔盘/断电、多卷及生产大库性能仍待验。所有命令经项目环境入口，未提交、部署、归档或操作用户实例。

## 2026-11-23 S03.2.2 受控备份根与平台发布

新增`NAS_BACKUP_DIR`静态配置、根/锁/marker身份核验、严格owner/manifest格式及Windows/Linux无覆盖发布；保持包内能力，正常disabled/unbound启动不探测或创建备份根。未开放API、执行器、启用、调度、持久下载或轮转。正式契约为`.trellis/spec/backend/backup-publish-contract.md`；独立报告`research/quality-review-s03-2-2.md`位于S03任务下，115行/15,581字节，SHA256 `4baa31ee219316e75e2290f12f37201d97ca0f81a66d090386d81b069835bbdb`。

- 独立修复Windows扩展名前ASCII空格绕过设备名检测（32个原始red反例及4个合法名称正例保留），以及Linux首次Stat失败丢弃Close错误。后者仅静态核对/交编。最终行为证据`s0322-review-behavior-final`，实际UTC `06:05:45.664895`至`06:06:14.115372`，**24顶层、257叶子、0嵌套容器、0 skip**；5个无子测试顶层单列，不能并入叶子或与旧轮相加。47个叶子及2个无子测试为checker新增，其余以独立前缀/新根复用回归，不称全部全新设计。
- 真实Windows核验包括零共享锁/零数据Stat、junction换根及恢复后sticky阻断、未知深树49节点DACL/内容前后不变、owner实际占用句柄证明末删、目标损坏和最终移动/Close未知保同ID。没有文件symlink创建权限，不计该产品场景；SYSTEM同SID分支覆盖不代表真实SYSTEM运行。源借用同App唯一快照，不重开/Close/转属。
- 最终Windows产物`.runtime/bin/s0322-review-final/nas.exe`，**21,593,600字节**、SHA256 `9839729e64f80e052f6e5acd3114e251f8fa5fbffad8276753d2a15148a43137`；同产物四新根`s0322-review-storage-final` **17项**、`s0322-review-backup-final` **38项**、`s0322-review-account-final --offline` **162=83+79项**、`s0322-review-core-final --large-file` **52项**通过。core实际完成UTC `2026-11-23T06:08:46.306+00:00`；其它结果无finishedAt，不从mtime补造。
- storage/backup共**6次真实Console CTRL_C_EVENT正常停服、0次应用强制回收、cleanupFailures=[]**；core服务退出1、owned-tree-reclaimed仅记自有Job回收。慢下载取消后锁/DB/监听释放及同根重启通过。5GiB稀疏Range仍只证明偏移，非解码/大库性能。
- gofmt、vet、清源go test包编译、空白检查、22份Python AST/方法注解、Vue typecheck/build和三平台fresh构建通过，工程证据`s0322-review-checks-final`。Linux amd64为21,379,424字节、SHA256 `c2fda79fb9f8a0715db24acaac047951fbe2400538ec67a43918d679a4a11752`；arm64为20,067,563字节、SHA256 `c76435628240db221a1511cfe23dd53f8c485bc7cae2e9bcc2ab9edb95929486`，均仅交编。四份临时Go源按最终SHA保存txt后精确清除；193行产品源清单SHA256 `0dd6d62aa89118b7e40e6291b23f38d9ea95780eb220f01dac2471734c08d199`。
- checker最终audit实际UTC `2026-11-23T06:10:08.902760+00:00`；主线另从原始事件重算计数、重算193源及三产物、核四回归同产物/临时源txt摘要和确切二进制路径活进程，证据`.runtime/verify/s0322-mainline-closeout/evidence-readback.json`为passed。sourceDrift=[]、临时源0、确切活进程0。行为/工程及storage/backup先扫描原始输出再存脱敏证据；账号离线13条Console原始流/可见viewport另有检查，不把在线脱敏日志扩大成所有秘密的原始证明。
- 原始red、深树夹具权限设置失败、第二全量轮及实施者旧失败/自查全部保留，均不与最终轮相加。主线复盘位于任务`research/root-publisher-debug-retrospective.md`，防止重用Go junction假设或重设已有根的继承DACL。所有本地命令经项目环境入口，没有改用户环境、提交、部署、归档或操作用户实例。

本片记`local_complete_target_pending`，S03整体继续。目标事后余量不是源前峰值准入；后者、原ID无source对账及执行器属S03.2.3。真实Linux ext4/XFS/NAS、多卷/FULL/IOERR/拔盘、真实大库、进程崩溃恢复、系统崩溃及物理断电仍待验；写穿/同步协议成功不构成断电保证。

## 2026-11-23 当前修改的 Windows 可用开发包

按用户“快速在当前修改上形成一个可用版本”的新优先级，本轮交付已验收 S03.2.2 源码的成品，S03.2.3 三份未接线草稿逐字节保存 `.go.txt` 后仅精确移出自有编译路径；没有回退既存产品修改或开放未完成 API。交接报告为 S03 任务 `research/s03-2-3-draft-checkpoint.md`（SHA256 `44d640541b56f6a2114b699778f6fa7a8e7439da7ef11a2eb16b147f8716ee06`），原产品193项源摘要无漂移。

- 前端 typecheck/build、清源Go包编译通过，原12包均无测试文件，不计作新增行为测试。现有 release.py 使用本轮 web/dist、新鲜源码与 webembed 标记生成两 EXE；没有重用上轮二进制。
- ZIP为 `bin/releases/usable-20261123/qiku-0.1.0-windows-amd64-development.zip`，13,434,468字节，SHA256 `63fae8eda5f9b5b3e693350951ecbd25a9b84f0104b0309f93b82ae33bb72c74`；已解压至 `bin/usable-20261123/`。开发包未捆绑FFmpeg/ffprobe，不能作为完整离线包。操作说明见[当前可用构建](usable-preview.md)。
- 实际成品 nas.exe 为15,505,920字节，SHA256 `8b5f58c721573387c9133134ada1e3fd79249db329b27deec7aa4e465579156e`；同一个成品在四个新隔离根的 core52（`--large-file`）、backup38、account162（`--offline`）、storage17共269项通过。core使用显式本机FFmpeg/ffprobe，5GiB稀疏样本只证明偏移协议。证据为 `.runtime/verify/usable-20261123-{core,backup,account,storage}/`。
- 成品栖库.exe 为15,735,296字节，SHA256 `ba0c2a5cb3afdf278f710cfd4ea1e15aa78ee2053dff71adbcdf10d093258220`；verify_desktop.py 省略 web-dir 与 with-tray，实际 embedded/headless 模式通过17类检查：中文路径/工作目录、端口避让、内嵌HTML/健康、IPC/单实例、慢流中断、退出释放、原位覆盖及同配置立即重启等。两次退出耗时0.029/0.017秒；不代表所有设备停服上限，本轮未实际操作托盘菜单。脚本临时夹具已自行回收，主线按实际工具stdout保存在 `.runtime/verify/usable-20261123-mainline/desktop-observed-result.json`，不冒充脚本原生持久结果。
- storage/backup共6次真实Console正常停服、0次应用强制回收、cleanupFailures为空；core的自有Job回收单列。主线 readback.json 实际UTC `2026-11-23T06:31:29.992738+00:00` 为passed：四套同成品、193源零漂移、归档与包内摘要吻合、三草稿编译源0、确切交付产物活进程0。没有覆盖旧S03.2.2的历史证据。
- 本轮轻量独立检查记录在 S03 任务 `research/quality-review-usable-20261123.md`；仅检本次交付完整性，不重复计入旧S03.2.2行为数，不代替未来S03.2.3完整门禁。没有新增产品契约，因此正式backend备份spec保持原已验收内容，新增启动说明与本节记录。

当前包是Windows本机开发可用版本；真实NAS/Linux运行、Docker部署、介质故障和断电未作为本轮通过项。策略仍关闭，定时、手动持久运行、历史下载、轮转和恢复准备仍留后续。未自动提交、部署、停止用户实例或修改用户配置。

## 2026-11-25 UAV 四站视频搜索下载插件

新增可选 `uav` 来源，固定适配 Jable、MissAV、Hanime1、Huangguo（黄果）。黄果按实际单集搜索和下载；预览、稳定请求编号、取消/重试、发布与视频扫描沿用原收集链路，SQLite 保持 schema 6。配置与使用见 [uav.md](uav.md)，正式契约见 `.trellis/spec/backend/uav-contract.md`；最终报告位于 `.trellis/tasks/11-25-uav-downloader-plugin/research/mainline-verification.md`。

- 最终 Windows 主服务/worker 位于 `.runtime/bin/uav-final-20261125-2/`。同批产物通过四站协议集成 **77 项**、浏览器 **42 项**、JMComic 回归 **66 项**；统一 baseline 的核心媒体 **44 项**、资源收集 **27 项**、发布边界自检 **83 项**通过。证据分别为 `.runtime/verify/uav-final-{protocol,browser,jmcomic,baseline}-20261125-1/`，浏览器子目录为 `browser-111932`。协议集成使用真实应用/worker/FFmpeg及离线桥接/下载器替身，不代表真实站点传输。发布自检也不代表已生成发行归档。
- 独立质量复核最终 **13 个测试函数、22 个真实 yt-dlp 下载场景**通过：5 个成功、17 个预期拒绝。覆盖生产认证代理、真实 TLS、CDN 逐跳作用域、AES 密钥、缺片、截断、Range 和 chunked 正常/短/长响应；另以真实 8 秒 TS 和约 1.96 秒可解码截尾反例核验时长门禁。初期另有 9 个跨层契约/处理器测试函数通过，两个阶段分开记账。证据 `.runtime/verify/uav-independent-quality-20261125/`；全部生成的临时单元测试源码已删除，保留非编译文本与日志供复验。
- 真实固定 UAV 包离线验证四站搜索/解析/空页各 4 例、无效分支 24 例及重定向 2 例；真实原生 HLS/AES 与本地解码通过。实际环境是 Python **3.14.4**、UAV **3.8.4**、yt-dlp **2026.10.26**，不是 Python 3.12 运行验收。Linux CPython 3.12 amd64/arm64 各 20 个锁定 wheel 的选择与哈希下载通过；未执行 Linux Python、容器或 NAS。
- 最终 Go vet、清源后的全包编译、gofmt、Python AST、Vue 严格类型检查和生产构建通过。Windows 与 Linux amd64/arm64 的主服务/worker 共 6 个产物构建成功；Linux 仅交叉编译。最终工程门禁 `.runtime/verify/uav-final-checks-20261125-3/result.json` 为 passed，关联第 2 轮实际构建日志与摘要；源码及产物无漂移。早期磁盘不足、工具路径和换行检查设置导致的失败均保留，详情见最终报告，不计为通过。

HLS 解码时长下界容差为 `max(0.5 秒, EXTINF 总时长的 2%)`，只能拒绝明显丢尾；直链 MP4 没有可信预期时长，传输/解码成功不能证明原片全长。真实公网搜索下载、Docker 镜像运行和物理 NAS 验收仍未执行。最终自有浏览器/服务已回收；未自动提交、部署或归档任务。

最终主线回读 `.runtime/verify/uav-mainline-final-audit-20261125-1/result.json` 为 passed：结果计数、同批产物、独立复核源码与最终源码一致；临时测试、零长度改动文件和确切最终产物活进程均为空。任务保留为 review，等待用户审阅提交。

## 2026-11-25 SupJav 第五站增量

在已提交四站的基础上接入 SupJav，支持关键词分页搜索、规范数字编号、元数据预览、持久下载任务、视频库扫描及原播放器。自动按 EVS/FST/VAS/LUC/VOE/TV/ST 尝试线路；传输或内容错误可清理暂存后换源，取消、存储/预算、异常暂存和进程回收错误会终止。上游、依赖和 schema 6 保持原固定版本。

- 同批最终产物位于 `.runtime/bin/supjav-20261125-1/`，通过五站协议集成 **102 项**、浏览器 **47 项**、JMComic **66 项**；核心媒体 **44 项**、资源收集 **27 项**、发布边界自检 **83 项**通过。对应证据为 `.runtime/verify/supjav-{protocol,browser,jmcomic,baseline}-20261125-1/`。浏览器覆盖四种宽度和明暗主题，黄果及 SupJav 样例实际入库并由原播放器打开；主线已查看手机、SupJav 搜索和播放器截图。桥接/下载器协议替身与真实站点证据分开记账。
- 固定真实 UAV 包离线检查：五站 search/resolve/empty 各 **5**，bad **30**、重定向 **2**；SupJav 七种 provider、**10** 个 fallback、**3** 个分页反例及 **16** 个私有输入/编号边界通过，证据 `.runtime/verify/supjav-implementation-20261125-1/bridge.log`。
- 独立复核 **18 个顶层测试、68 个子场景**通过，其中 **44 个真实下载器场景**分为 SupJav **22** 和原 relay **22**。真实生产获取链路覆盖普通/PNG 包装 TS、AES 显式/默认 IV、轮换密钥、损坏尾包、短传输、缺片及原 Range/CDN 范围；修复并复测 TS 适配字段长度的三个结构反例。日志、非编译源码文本与摘要见 `.runtime/verify/supjav-quality-20261125-1/`。进程回收失败传播仅静态追踪，未做实际故障注入。
- 前端严格类型检查与生产构建、清理单元测试后的全仓 Go vet/编译及 6 个 Windows/Linux amd64/arm64 主服务/worker 构建均通过；Linux 仅交叉编译。工程门禁分别为 `.runtime/verify/supjav-{frontend,build}-20261125-1/`，最终格式、AST、Trellis 上下文、源码/产物摘要和进程清理审计见 `.runtime/verify/supjav-final-audit-20261125-1/`。新增临时单元测试源码已删除，保留可复验文本。

固定下载器不逐字节验证 AES 的 PKCS#7 填充；若改变填充后仍得到相同 TS 明文，后续结构与解码校验无法识别。仍未执行真实公网站点/CDN 搜索下载、Docker、Linux 运行或物理 NAS 验收。完整本轮报告为 `.trellis/tasks/11-25-uav-downloader-plugin/research/supjav-mainline-verification.md`，任务保留 review；未自动提交、部署或归档。

## 2026-11-25 JMComic 高级搜索与下载扩展

在原可选漫画插件上新增四种排序、四种时间范围、全本／选章预览与下载、最多十本独立批量、逐页进度和显式失败后的可信页面复用。进度字节包含复用图片，不是网络流量；恢复以完整图片页为单位，不是 HTTP 字节断点续传。固定依赖仍为 `jmcomic==2.7.12`，schema 仍为 6。使用与同步升级要求见 [jmcomic.md](jmcomic.md)，最终报告为 `.trellis/tasks/11-25-jmcomic-advanced/research/final-validation.md`。

- 独立检查后统一构建 `.runtime/bin/jmcomic-advanced-final/`：高级应用／worker 协议 **144 项**、高级浏览器 **50 项**、原漫画协议 **66 项**、原漫画浏览器 **38 项**、UAV 五站协议 **102 项**全部通过；同批产物的核心媒体 **44 项**、资源收集 **27 项**、发布边界自检 **83 项**通过。协议和浏览器使用实际应用、worker、媒体发布／扫描／阅读与离线上游协议 fixture，不代表真实站点传输。高级协议另覆盖总页数不变但分章从 2+2 变为 1+3 的拒绝场景。
- 固定真实 JMComic 依赖离线桥接 **22 个检查点**通过，执行真实客户端、下载器、图片解码及筛选／选章钩子，禁止真实网络。实际验证三页可信复用、仅一页新传输，以及损坏页重新获取、元信息／请求绑定／页面来源漂移、硬链接／额外文件和总预算拒绝。证据 `.runtime/verify/jmcomic-real-bridge-b2944294a242457a83df76bbfb410107/report.json`，与协议 fixture 分开记账。
- 浏览器覆盖 375／768／1024／1440 四种宽度、明暗主题、焦点／Escape、慢响应隔离、超预算后选章入库阅读、真实获取中的原生进度条，以及批量部分失败、未知结果过期后原 body／UUID 重试和成功项不重发。最终高级及原浏览器证据分别为 `.runtime/verify/jmadvanced-final-browser-20261125-1/advanced-browser-153252/` 与 `.runtime/verify/jmadvanced-final-oldbrowser-20261125-1/browser-153252/`；已人工回读手机目录、长标题、部分失败和进度截图。
- 最终 Go vet、清理临时测试后的包编译、gofmt、Python AST、Vue 严格类型检查及生产构建通过。Windows 与 Linux amd64／arm64 主服务和 worker 共六个产物构建成功；Linux 仅交叉编译。工程证据 `.runtime/verify/jmadvanced-final-checks-20261125-2/result.json` 为 passed，记录源码／产物摘要；临时测试和源码漂移均为空。

最终高级协议证据为 `.runtime/verify/jmcomic-advanced-20261125T073227515799Z-1ec0d7/`；原漫画、UAV 与 baseline 分别为 `.runtime/verify/jmadvanced-final-{jmcomic,uav,baseline}-20261125-1/`。早期 fixture 格式、Windows 快照替换冲突及辅助工具路径失败均保留并说明修复，未混入最终通过数量。真实公网站点、账号、Docker、Linux 运行及物理 NAS 未实测；账号／Cookie／收藏夹、PDF／ZIP 与任意上游插件仍未开放。本轮自有服务和浏览器已关闭，临时单元测试源码已删除，任务进入 review；未自动提交、部署或归档。
