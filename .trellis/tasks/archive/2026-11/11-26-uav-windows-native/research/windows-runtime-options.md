# Research: UAV Windows 私有运行时与下载器分发方案

- Query: 为现有 UAV 五站点搜索、解析和下载链提出 Windows 原生兼容方案；核对官方 embeddable Python、当前依赖锁、独立 yt-dlp.exe 与私有 Python `-m yt_dlp` 的分发差异。
- Scope: mixed；只读当前代码、规范、既有 JMComic Windows 研究及官方文档／小型元数据；仅输出方案，没有实现或安装。
- Date: 2026-11-26（Asia/Shanghai）。
- Task: `.trellis/tasks/11-26-uav-windows-native`。

## Files found

| 路径 | 职责与本研究关联 |
| --- | --- |
| `internal/collectionworker/config.go:31-35,73-79,197-199` | JMComic 已有显式解释器／运行时清单状态；UAV 目前只有启用开关和解释器路径 |
| `internal/collectionworker/uav.go:23-30,65-126` | 固定 UAV 3.8.4、提交与桥接；启动 probe；以 `-I -B -c` 调用解释器 |
| `internal/collectionworker/uav.go:282-315` | Go 生成受限下载意图，以固定 yt-dlp 获取有限 HLS／直链，禁止联网 FFmpeg 回退 |
| `internal/collectionworker/uav_bridge.py:388-401` | Python≥3.12、发行版本、`direct_url.json` 源码摘要校验，以及真实上游五站点和依赖导入 |
| `internal/collectionworker/service.go:158-170` | `s.ytdlp` 是预先解析的单一可执行文件路径；版本探测也是直接运行该路径 |
| `internal/collectionworker/tools.go:74-129` | 受控环境、临时目录、代理、无 shell argv、输出上限，以及按工具路径分类存储错误 |
| `deploy/uav-requirements.txt:1-19` | 17 项无头运行依赖，锁定版本与允许的官方材料摘要 |
| `deploy/uav-build-requirements.txt:1-4` | setuptools、wheel、packaging 的独立构建期锁；不在终端运行时构建 |
| `deploy/collection-requirements.txt:1-2` | 现有下载器 wheel 版本固定为 yt-dlp 2026.10.26 |
| `Dockerfile.collection-worker:12-19,30-43,55-68` | 容器使用独立 UAV Python 3.12 环境，从锁定原始源码包构建；下载器另在 collection 环境 |
| `scripts/jmcomic_runtime.py:139-170,207-240,268-308,312-350,397-436` | 已实现的 Windows 私有 Python 组装、来源、文件／架构、隔离路径、真实离线 probe 和许可归集模式 |
| `internal/jmcomicruntime/runtime.go:127-179,228-239,291-299` | Go 侧发布材料与逐文件审核；适合作为 UAV 对等实现的参照 |
| `scripts/release.py:30-35,259-269,299-334` | 当前媒体工具清单属于 FFmpeg；可携带 JMComic 运行时；尚未有 UAV 或 yt-dlp 独立发行清单 |
| `scripts/verify_uav_bridge.py:347-364` | 现有真实固定 yt-dlp 的 Python 模块调用与有限 HLS 离线验收入口 |
| `.trellis/tasks/11-26-jmcomic-windows-native/research/windows-python-runtime.md` | 前轮 Python／DLL／编码方案研究；其开发 venv 证据不能代替 UAV 成品验证 |

先以 CodeGraph CLI 查询 `uavCommand`，返回空数组，随后按实际文件使用限定范围 `rg` 及带行号读取；没有重建索引或更改源码。

## Findings

### 1. 推荐结构及最小改动结论

推荐 Windows x64 原生发行采用 **独立 `runtimes/uav/` 私有 CPython + 固定无头 UAV 闭包 + 独立固定 `tools/yt-dlp.exe` + 既有 FFmpeg／FFprobe 发行材料**。复用已经落地的 JMComic Windows 组装、清单、路径审核和离线探测模式，按 UAV 的真实身份要求增加对应实现；不重写五站点业务和下载安全链。

UAV 与 JMComic 即使共享 curl_cffi、certifi、Crypto 等包，也继续使用各自依赖环境和清单。不能把两个环境合并，也不能复制开发机 venv 作为便携包。下载器与 Python 解析器分别声明身份、版本、许可及就绪原因。

固定 `yt-dlp.exe` 是对现有代码改动最少的下载器选项，因为 `s.ytdlp`、版本检查、网页下载、UAV 传输及错误分类都把它作为单一可执行路径使用。其重要发行成本是官方 EXE 的 GPLv3+／第三方许可和对应源码材料，不能只附 yt-dlp 源码的 Unlicense。

此处提出的是工程方案，不代表已选定新的 Python 补丁版本或已通过干净 Windows 验收。可优先以现有已验证候选 CPython **3.14.4 标准 GIL x64** 做组装验证，通过后再固定；不顺带升级 UAV、下载器或依赖锁。

### 2. 官方 Python 分发约束

核对 Python 3.14 官方文档 [R1–R4]：

- Windows embeddable ZIP 是供应用携带的最小私有 Python 环境；包含解释器、DLL 和 ZIP 标准库，不含 Tcl/Tk、pip 和文档。第三方依赖应由应用构建／安装过程随版本交付。
- 官方不支持像常规安装那样使用 pip 管理 embeddable 的依赖。建议仅在构建端处理 wheel／固定源码，终端不运行 pip、不在线补包、不修改全局 PATH。
- `python314._pth` 可彻底限定模块搜索路径；存在该文件时忽略注册表和环境路径，开启隔离模式，除非显式写入 `import site` 才加载 site。保持仅 `python314.zip`、`.`、`Lib/site-packages` 的固定内容，与现有 JMComic 模式一致。
- venv 中脚本会引用解释器绝对路径，官方将其视为通常不可搬移环境，应在目标位置重建。wheel 安装生成的 Windows Console Scripts launcher 也不能被假定为可随目录移动；本方案直接调用受审核 `python.exe` 或官方独立下载器，避免依赖该假设。
- `-I` 忽略全部 `PYTHON*` 环境变量，因此仅设置 `PYTHONIOENCODING=utf-8` 不足以保证解释器编码。UAV 调用建议与已验证 JMComic probe 一致使用固定 `-I -B -X utf8`。当前桥接用字节读 JSON、`ensure_ascii=True` 输出，本轮没有证据称它已必然乱码；增加显式编码主要覆盖错误和后续路径边界。

现有 `scripts/jmcomic_runtime.py:166-170,207-240,408-411` 已实现对应隔离路径和真实探测。可复用方法，但 UAV 的上游安装来源不是同一种普通 PyPI wheel，必须单独处理下述差异。

### 3. 当前 UAV 依赖锁的 Windows 可行性证据

2026-11-26 只读查询官方 PyPI 每个已锁版本的 JSON [R5]，将每条允许 SHA256 与官方文件列表对照。**17 项运行依赖的全部已列哈希均能在相应官方版本元数据中找到**；每项至少有适合标准 CPython 3.14 Windows x64 的纯 Python／native 候选。关键 native 候选如下：

| 锁定包 | 锁内且与官方元数据一致的 Windows x64 候选 | 解释 |
| --- | --- | --- |
| `charset-normalizer==3.5.3` | `charset_normalizer-3.5.3-cp314-cp314-win_amd64.whl`，以及纯 Python wheel | 普通 CPython 3.14；不要误选 `cp314t` |
| `pycryptodome==4.6.0` | `pycryptodome-4.6.0-cp39-abi3-win_amd64.whl` | ABI3 候选；仍需实际 AES/native 装载探测 |
| `curl_cffi==0.17.0` | `curl_cffi-0.17.0-cp310-abi3-win_amd64.whl` | 包内扩展和 curl 相关 DLL 必须完整保留；不要误选 `cp314t` 或 `cp315` |
| `cffi==2.1.1` | `cffi-2.1.1-cp314-cp314-win_amd64.whl` | CPython 3.14 对应扩展；不是任意 Python 可通用 |

其余 requests、idna、urllib3、PySocks、beautifulsoup4、m3u8、cloudscraper、certifi、pyparsing、requests-toolbelt、soupsieve、typing_extensions、pycparser 均有已锁通用 wheel 候选，元数据 `Requires-Python` 不排斥 3.14。也存在部分 3.12 native 候选，但这不意味着所有 Python 3.12–3.14 组合都已验收。

这只是**哈希与文件名／Python 约束匹配证据**，没有下载 wheel、扫描或加载本机 DLL，也未形成 embedded 成品。因此报告不得写成“Windows 无须再改依赖、已经兼容”。实际构建必须明确 `win_amd64`、标准 CPython ABI 和唯一候选材料，并重新核对证书、dist-info、native DLL 和许可闭包。

### 4. UAV 特有的来源与构建约束

`uav_bridge.py:388-392` 不仅检查包名／版本，还要求 `uav-downloader` 的 `direct_url.json.archive_info.hashes.sha256` 等于固定源码包摘要。现有容器从固定 codeload tar 源码安装（`Dockerfile.collection-worker:16-19`）；普通 wheel 解压不会天然生成这一源包 provenance。

因此，UAV assembler 不能仅换一下 JMComic 包名后直接使用：

1. 固定 **UAV 3.8.4**、提交 `d40f09457fbd38dd7dc94580637242afafaf3939`、源包 SHA256 `88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41`，构建前核验原始字节。
2. 仅在构建端用 `uav-build-requirements.txt` 的固定工具组装无头 wheel／文件树；不装 GUI／AI extras，不在目标机编译。
3. 运行时保留上游 `.dist-info`、许可证与桥接已要求的来源记录。若通过构建 wheel 后解包交付，必须在核验原始源码后，按固定规则保留或生成与原始固定源码对应的来源记录，且不能带构建机绝对路径。清单同时记录源包摘要、构建工具锁摘要、派生 wheel／最终文件摘要，不能用自己刚生成的 wheel 摘要冒充原始 tar 摘要。来源规范化产生／改变的 `direct_url.json` 也必须纳入 `.dist-info/RECORD` 的生成／一致性核验和最终运行时清单逐文件审核；不得通过忽略元数据变动或放宽现有 probe 来让成品通过。
4. 对每个普通文件、祖先目录、Windows 重解析点／大小写别名、PE 架构、逐文件哈希、声明外文件做与现有运行时相当的审核；安装目录只读，状态／缓存／临时文件继续使用 worker 数据根。
5. 离线 probe 至少导入真实五站点模块、requests／m3u8／cloudscraper／curl_cffi／cffi／Crypto，检查固定版本、来源、证书和实际 AES 运算；污染 PYTHONHOME/PYTHONPATH，确认隔离；禁止 probe 访问真实站点。

完整上游 pyproject 还声明刻意未安装的 GUI／AI 依赖。既有规范明确无头 probe 不等价于完整上游 `pip check`；不能为了得到完整 `pip check` 绿灯扩大运行依赖（`.trellis/spec/backend/uav-contract.md:111`）。

### 5. 下载器两个可行选项

核对固定 **yt-dlp 2026.10.26** 官方 README、pyproject、模块入口及 Crypto 适配源码 [R6–R9]，没有选择或宣称“最新版”。

| 比较项 | 固定官方 `yt-dlp.exe` | 私有 Python `-m yt_dlp` |
| --- | --- | --- |
| 官方入口 | README 列为推荐 Windows Win10+ standalone x64，携带解释器及标记依赖 | 官方 `yt_dlp/__main__.py` 明确支持 `python -m yt_dlp`；该版本 Python≥3.11 |
| 与当前调用契合度 | 直接匹配 `s.ytdlp` 单一可执行路径，保留现有 argv、错误路径识别和两类下载调用 | 不能把 `python.exe -m yt_dlp` 塞进路径字符串；需结构化“可执行路径 + 可信固定前缀参数”，并统一版本、web、UAV 和错误分类 |
| 私有 Python 依赖 | 与 UAV 解释器解耦；EXE 自带依赖仍须完整发行审查 | 向 UAV 私有运行时增加固定 `deploy/collection-requirements.txt` wheel 或另设下载器环境；不使用生成的 Console Scripts launcher |
| 锁定与清单 | 新增独立下载器材料清单，固定 tag URL、SHA256、目标架构、版本、第三方许可与对应源码；正式包不得只在 PATH 找一个 | 运行时清单记录下载器 wheel 与所有实际需要的依赖、许可证、版本；不能假定 `yt-dlp[default]` 的全部 extras 都已由 UAV 提供 |
| 许可差异 | 官方说明 PyInstaller 合并二进制含 GPLv3+ 代码，合并作品按 GPLv3+；不能仅提供 Unlicense | 官方说明 PyPI wheel／源码只含 Unlicense 代码，但使用的其他依赖仍各有自己的许可证 |
| 业务边界 | 原有有限 HLS／直链与代理保持；禁止自动升级与自更新 | 同样保持既有固定参数；不得新增用户自定义命令、模块名、脚本、任意前缀或运行时安装 |
| 实际代价 | 增加一份独立可执行材料及许可／来源管线；Go 业务改动最少 | 可少带一份解释器，但增加所有下载器调用的命令抽象与能力耦合；需回归通用网页收集，不能只测 UAV |

官方 EXE 的来源应使用 **固定 tag** 下载地址，而不是 README 的 `latest` 链接；官方发布提供 SHA2-256SUMS 及 GPG 签名，可作为构建输入审查依据。没有验证过签名就不能宣称已验证签名。工具材料清单不能塞进当前 FFmpeg 专用 `licenses/ffmpeg/tools-manifest.json` 然后假定它已覆盖下载器。

模块方案并非不可行：当前离线脚本本来就在 `scripts/verify_uav_bridge.py:364` 用固定模块 argv；官方 `Cryptodome.py` 会从 `Cryptodome` 回退 `Crypto`，因此当前 pycryptodome 是 AES 候选，不必仅因为官方推荐 pycryptodomex 就立即引入第二套 Crypto 包。但最终 HLS、Range／缺片／AES／取消回收仍须在最终 embedded 组合实际验证。

如果发行材料的许可／对应源码组织更适合 wheel，可以选择模块方案；这是一项明确的实现选择，不能在发行阶段临时发现 `yt-dlp.exe` 不存在后改为任意 PATH Python。模块可执行解析也应独立于 UAV 启用开关，否则会意外让通用 web 能力随 UAV 关闭。

### 6. FFmpeg／FFprobe 与 JavaScript 的边界

官方 README [R6] 明确：所需 FFmpeg 是真正的本机二进制，不是同名 Python 包；Python 私有环境和 yt-dlp standalone 不能替代 FFmpeg／FFprobe 的分发。继续沿用已有固定版本、架构、摘要及许可／对应源码审核，若选用动态构建则同时包含需要的 DLL。

官方也说明 yt-dlp-ejs 与 JavaScript runtime 用于完整 YouTube 支持。本项目 UAV 已由固定上游解析出有限下载意图，并显式禁用 JS runtimes／remote components。Windows 兼容方案不应因此额外安装 Node／Deno／浏览器，亦不应放开 EJS、插件目录或网络 FFmpeg 回退。

### 7. 方案选择与验收边界

| 方案 | 适用定位 | 是否推荐本轮作为普通用户交付 |
| --- | --- | --- |
| 目标机重建 venv + 管理员显式绝对解释器／下载器路径 | 开发及高级运维回退；必须给出安装／锁验证说明 | 保留，但不能作为解压即用发行的完成标准 |
| 私有 embeddable UAV + 独立固定官方 yt-dlp.exe + 既有媒体工具 | Windows x64 桌面／托管 worker，最少改动现有业务调用 | 推荐；先完成下载器分发许可／来源管线 |
| 私有 embeddable UAV + 固定模块下载器 | 需要 wheel 分发路线且接受统一命令抽象的实现 | 可行备选，需要额外通用 web 回归 |
| 现有 Docker／外部 worker | 已部署环境或希望继续隔离运行依赖的用户 | 保留已有路径；不能等同于无 Docker 的 Windows 原生体验 |

建议支持基线先收敛 **Windows 11 x64 普通用户**；官方 yt-dlp 的 Win10+ 声明只是上游支持信息，不等于本项目同时承诺 Windows 10／ARM64。当前任务无需 SQLite schema 升级，现有 schema 6 和搜索、预览、任务／入库／播放器协议保持。

成品验收应覆盖：无全局 Python／Go／Node／Docker的干净机器；中文空格路径和不同 CWD；污染／清空 PATH 与 Python 环境；只读程序目录；证书和 native 扩展实际加载；真实固定解析器的离线五站点 fixture；真实固定下载器有限 HLS／AES／Range／缺片与 TS 规范化；取消、托盘退出、马上覆盖 EXE／DLL、同 profile 重启；缺失／损坏运行时的中文降级原因。清单及 wheel 选择静态通过不能替代这些实际证据。

## External references

查阅日期均为 2026-11-26。先调用 web 搜索／打开接口但无正文返回，随后通过项目环境入口，用 Python `urllib.request` 只读获取以下官方小型文档／元数据；没有保存外部包、安装依赖、访问视频站点或下载大文件。

- [R1 — Python 3.14 官方 Windows embeddable 文档](https://docs.python.org/3.14/using/windows.html#the-embeddable-package)
- [R2 — Python 3.14 官方 sys.path／._pth 文档](https://docs.python.org/3.14/library/sys_path_init.html#pth-files)
- [R3 — Python 3.14 官方 venv 非便携说明](https://docs.python.org/3.14/library/venv.html#how-venvs-work)
- [R4 — Python 3.14 官方隔离与 UTF-8 命令行选项](https://docs.python.org/3.14/using/cmdline.html)
- R5 — 官方 PyPI 逐个版本 JSON：`https://pypi.org/pypi/<锁文件包名>/<锁定版本>/json`。重点核对 [pycryptodome 4.6.0](https://pypi.org/pypi/pycryptodome/4.6.0/json)、[curl_cffi 0.17.0](https://pypi.org/pypi/curl_cffi/0.17.0/json)、[cffi 2.1.1](https://pypi.org/pypi/cffi/2.1.1/json)、[charset-normalizer 3.5.3](https://pypi.org/pypi/charset-normalizer/3.5.3/json)，其余 13 项按同一只读路径与本地锁逐条对照。
- [R6 — yt-dlp 2026.10.26 官方 README：Windows 分发、校验、许可与依赖](https://github.com/yt-dlp/yt-dlp/blob/2026.10.26/README.md#release-files)
- [R7 — yt-dlp 2026.10.26 官方 Python 约束、可选依赖及 wheel 许可](https://github.com/yt-dlp/yt-dlp/blob/2026.10.26/pyproject.toml)
- [R8 — yt-dlp 2026.10.26 官方 -m 入口](https://github.com/yt-dlp/yt-dlp/blob/2026.10.26/yt_dlp/__main__.py)
- [R9 — yt-dlp 2026.10.26 官方 Crypto 回退适配](https://github.com/yt-dlp/yt-dlp/blob/2026.10.26/yt_dlp/dependencies/Cryptodome.py)

## Related specs

- `.trellis/spec/backend/uav-contract.md`：固定上游身份、五站点协议、来源、有限媒体与下载器参数、独立 Python 和真实验收。
- `.trellis/spec/backend/cross-platform-contract.md`：Windows 进程／Job、路径、发行材料、原生与容器数据隔离、项目环境入口和干净机器验收边界。
- `.trellis/spec/backend/jmcomic-windows-contract.md`：现有桌面托管 worker、私有 Python、显式覆盖、运行时清单与发布接线范例。
- `.trellis/spec/backend/collection-contract.md`：既有网页收集、能力、任务、鉴权与产物协议。

## Caveats / Not Found

- 本研究没有修改业务代码、规范、配置或任务状态；唯一写入是本文件。
- 未安装／下载 wheel、embeddable、EXE 或 GUI 工具；未运行最终 UAV embedded 运行时、干净虚拟机、真实视频站点、Docker／NAS或 Windows ARM64。
- 当前依赖元数据匹配及先前开发机 Python 3.14.4 测试，均不能证明私有运行时／随包 EXE 的 native DLL 闭包完整。VC 运行库前提需通过成品和干净机器实测，不能靠开发机已有 DLL 掩盖。
- 官方 EXE 与 PyPI wheel 的许可差异是重要的材料选型边界。本研究引用官方分发声明，没有审定最终发行包应承担的每项条款或声称已收齐源码。
- Python 3.14 官方网页是所选系列文档，页面可能随补丁更新；这里只依赖分发／隔离机制，不以页面版本号选择“最新补丁”。所有建议继续固定候选及重新验收，不从可变 master／latest 取运行时代码。
- `uavCommand` CodeGraph 查询无结果，定位以当前真实源码为准。
