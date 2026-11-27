# Research: Windows UAV 依赖获取与原生初始化

- Query: 如何在干净 Windows amd64 上无需预装 Python/pip 自动初始化固定 UAV、yt-dlp 和 FFmpeg，且保留原严格运行时校验？
- Scope: mixed（仓库实现、已验证材料、官方网络材料）
- Date: 2026-11-27

## Findings

### 已有实现与材料

| 文件 | 用途与复用边界 |
| --- | --- |
| `scripts/uav_runtime.py:38–71` | 固定 Python 3.14.4 embeddable 归档来源及 SHA256；UAV 源码来源固定 commit |
| `scripts/uav_runtime.py:172–211` | `build_uav_wheel` 依赖构建机 Python 的 venv/pip 和固定构建 wheels；只能在开发/构建侧使用，不适合终端执行 |
| `scripts/uav_runtime.py:404–483` | 装配固定 `._pth`、归档提取、大小写 metadata 规范化、`direct_url.json`/RECORD、每组件许可、manifest；装配完成需真实 probe |
| `scripts/runtime_common.py:33–155` | 已有文件大小/hash/PE、普通路径、解压预算、wheel `.data` relocation、链接/重复/穿越/额外 `.pth`/`.pyc` 拒绝模式 |
| `internal/uavruntime/contract.json` | UAV 3.8.4、commit、源码hash、runtime/build requirements hash、17个依赖版本及允许的wheel hashes |
| `.runtime/tools/uav-private/uav-runtime.json` | 先前真实装配且验证过的208,737字节运行时清单，包含固定 x64/CPython 3.14 wheels 的完整来源URL及794左右安装文件库存；只能作为构建输入，不能在终端硬编码此开发路径 |
| `scripts/ytdlp_runtime.py:24–37` | 独立 EXE 的固定官方URL/hash、许可和对应源码分发材料合同；本机下载使用与正式分发审查必须区分 |
| `.trellis/spec/backend/uav-windows-contract.md` | 保留隔离、完整性、来源、清洁机器验证、显式Python优先与可选插件独立失败 |

现有 assembler 输出的 `builtWheelSha256=65472d6f06b5c806e3e06479f0447a60668a17ad159a4015195ba9a2b680e61c` 是先前构建 wheel 的历史收据；当前原 wheel 缓存已找不到。不能把新生成的归档命名为该原 wheel 或宣称该新归档有同一hash。新 bootstrap payload 可明确作为“已装配且已通过库存验证的文件压缩载荷”，有自身独立SHA256，保留原 runtime manifest 和历史构建收据不变。

### 可行方案比较

1. **构建侧固化少量 payload + 官方 ZIP/wheel 原生下载还原（推荐）**：构建侧先严格校验已装配运行时，生成编译进应用的 runtime manifest、recipe、少量确定性文件 payload；终端 Go 根据编译期锁下载官方 Python 和 wheels，逐成员映射还原固定清单，最后执行原 `uavruntime.Load` 和 worker 真实 probe。没有终端pip/build。UAV源码/metadata/所有许可可收进小型payload，避免重复实现wheel构建和METADATA生成。
2. **编译或随包携带完整压缩依赖闭包**：打开时只解压、校验、probe；最适合离线版，但包体需要 Python/wheels/UAV/yt-dlp/FFmpeg 全部压缩材料（单 FFmpeg ZIP 约115MB）。可在未来对同一recipe增加本地缓存来源，并沿用初始化器。
3. **下载项目完整预制bundle**：运行时最简单，但当前无已核实发布host、版本命名/不可变摘要和完整分发材料；无法把不存在的URL留给用户配置当作交付。不能作为本次唯一实现。

### 推荐生成器结构

- 增加构建侧 `scripts/uav_bootstrap.py`，输入是已通过现有 `uav_runtime.verify` 的 runtime，输出Go编译资源，不依赖未来终端上的仓库路径。
- 从 runtime `sources` 中直接固化 Python 和17个依赖 wheel 的URL/hash；不做终端PyPI解析、pip解析或latest查询。
- 每个最终运行文件须有精确来源：官方归档成员、payload成员或已有完全一致库存文件。不要按大目录“解压后取全部”来扩大执行闭包。
- payload 放入 UAV component 全部源码/metadata（包括 `direct_url.json` 和RECORD）、固定 `python314._pth`、规范化后与上游wheel不同的RECORD/metadata bytes及每组件license。payload自身有独立hash；manifest不应改称payload为wheel。
- 对 `.data/purelib`/`.data/platlib` 的重定位和 `PySocks` metadata大小写规范化沿用原语义。用构建侧比对归档成员hash与最终manifest可识别真正需要overlay的少数文件，避免在Go复制Python包管理器。
- 终端先按编译期 SHA256 校验下载文件，再安全读取ZIP；最终文件长度/hash和PE架构仍使用原验证器。临时输出独立 generation，经全量验证后再原子选择；不要覆盖安装目录或半成品中继续启动worker。
- 编译资源和recipe需要有稳定版本身份。启动先验证已经选中的generation，全有效时不得联网。缓存/失败stage不是成功标志。
- 资源路径不能接受浏览器自定义URL。重定向仅允许必要固定HTTPS官方/CDN域名并有次数上限；原 `runtime_common.download` 对重定向一概拒绝，GitHub EXE本身会重定向，不能照抄这个细节。

### 已实际检查的官方材料

#### Python 与 UAV

- 固定 Python URL：`https://www.python.org/ftp/python/3.14.4/python-3.14.4-embed-amd64.zip`。
- 固定 SHA256：`cda80a9b1e75c0f1b4f9872ca1b417f0d19bce32facc811aea9180e70fad5fb9`（既有assembler锁，来源说明为官方zip.sigstore中的SHA2_256；并非本次完成Sigstore验证）。
- UAV源码：`https://codeload.github.com/Alos21750/UAV-Downloader/tar.gz/d40f09457fbd38dd7dc94580637242afafaf3939`。
- 本次实际内存下载 **1,952,930字节**，SHA256=`88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41`，与契约匹配。
- UAV `pyproject.toml` 指定 setuptools build_meta，`src/uav_downloader` 包，version 3.8.4，Apache-2.0。它还声明 GUI/AI依赖；现有项目有意只装经审查无头闭包，自动初始化不得改成pip安装上游全量依赖。

#### yt-dlp

- 固定官方EXE：`https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.26/yt-dlp.exe`。
- SHA256=`f5389d3698ecde4ef06f4e6b1ce0294fe9ac1396228c8990b8d3416d6d177a7d`。
- 当前 `.runtime/tools/uav-windows/yt-dlp.exe` 大小23,575,744字节，之前任务已经版本/完整性/实际执行验证；本次未重复全EXE网络下载。
- 正式分发仍遵从 `scripts/ytdlp_runtime.py`；自动下载安装上游文件不能伪造 `correspondingSourceReviewed=true` 或完整分发清单。

#### FFmpeg 静态 x64 ZIP（本次发现可用且固定）

- 构建方页面：`https://www.gyan.dev/ffmpeg/builds/`（页面列出该固定版本package及hash）。
- 固定ZIP：`https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-9.0.2-essentials_build.zip`。
- 官方hash：同URL追加 `.sha256`，本次实际HTTP读取=`60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba`。
- 本次HEAD：HTTP200，无重定向，`Content-Length=114768076`；支持HTTP206 Range。
- 本次用Range实际检查ZIP中央目录，49项，固定根 `ffmpeg-9.0.2-essentials_build/`：
  - `bin/ffmpeg.exe`：105,423,872字节，ZIP压缩37,386,196字节。
  - `bin/ffprobe.exe`：105,221,120字节，ZIP压缩37,283,862字节。
  - `bin/ffplay.exe`：106,903,552字节，自动初始化无需提取。
  - 无配套DLL成员，README声明static build，避免先前本机shared FFmpeg需7个DLL的环境耦合。
  - `LICENSE`：35,147字节，实际解压读取SHA256=`8ceb4b9ee5adedde47b31e975c1d90c73ad27b6b165a1dcd80c7c545eb65b903`。
  - `README.txt`：41,577字节，实际解压读取SHA256=`0342de6bb39dd421cbec2c00d89ad274b640d75ad3f2cab09ffd647ce2d8f949`。
- README标明GPLv3、完整构建配置、包含libx264和libx265，源码链接 `https://github.com/FFmpeg/FFmpeg/commit/946fcce07b`。保留LICENSE和README材料及来源receipt；完整第三方对应源码分发审查仍单独处理。
- 可固定整个ZIP SHA256后只提取二EXE+LICENSE/README，不需要把ffplay或doc树加入运行闭包。安装器仍需本机实际执行二EXE并做真实离线encode/probe/decode以证明DLL/系统依赖可用。

## Related specs

- `.trellis/spec/backend/uav-windows-contract.md`
- `.trellis/spec/backend/uav-contract.md`
- `.trellis/spec/backend/jmcomic-windows-contract.md`
- `.trellis/spec/backend/cross-platform-contract.md`

## Caveats / Not Found

- 本次未下载114MB完整FFmpeg ZIP，获得的是官方checksum、HEAD、中央目录和两份小文本成员的实际读出；正式生成器/实现验收必须全量下载hash验证，随后真实执行，不能把本研究记为这些已通过。
- GitHub REST API匿名请求遇到403 rate limit；官方直链/codeload及Gyan页面正常。生产初始化不应依赖GitHub API查询release。
- 未发现已发布的项目完整bootstrap bundle host；不使用凭空假定URL。
- 先前原UAV wheel缓存已不存在；重新生成小型payload时必须用自身hash和准确名称，不能假称重新获得原wheel。
- 原spec禁止终端联网安装/构建是前任务边界；新用户要求允许固定依赖自动初始化，需要同步修改spec，保留“不调用pip、不编译、不改全局环境、不降低加载器校验”的核心约束。
