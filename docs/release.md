# 维护者跨平台发布流程

终端用户运行成品包不需要 Go、Node、Python 或全局 FFmpeg。以下步骤仅在构建机执行，不会自动发布 GitHub Release、注册表镜像或安装 Docker。

## 构建目标与输入

`scripts/release.py` 支持 `windows-amd64`、`linux-amd64`、`linux-arm64`，构建网页并暂存到 `internal/webassets/dist`，以 `webembed` 标签内嵌。暂存有独占构建锁并在结束后清理；不要并发操作暂存目录。Linux/Windows 共用后端，包中包含独立 worker，Windows 包另含托盘 EXE。

构建机使用项目 Go 工具链、Node 锁文件和 Python 3.10+。Go 不在 PATH 时可用 `--go`，或仓库 `.tools/go/bin/go.exe`。完整离线发布还需每目标经过审核的 FFmpeg/ffprobe 工具材料清单；不同平台不能混用。

Windows 桌面图标通过已提交的 `cmd/nas-desktop/resource_windows_amd64.syso` 自动内嵌，普通发布无需下载图标生成工具。更新 `assets/windows/qiku.ico` 后，运行 `python -B scripts/project_env.py -- python -B scripts/generate_windows_icon.py` 并提交更新后的资源；`--check` 可验证逐字节一致性。资源仅用于 Windows amd64 桌面入口，Linux CLI 不链接该对象。

发布入口自动加载 `scripts/project-env.json`，将 Go/npm/Node 缓存和通用临时目录放到当前仓库 `.runtime/`；直接执行 Python 发布脚本也会生效。手动执行 Go/npm 检查时通过 `python -B scripts/project_env.py -- <命令>` 加载同样的环境，具体见[项目构建环境](project-build-environment.md)。安装开发工具的 Action、Docker daemon 自身的镜像存储和用户显式指定的外部归档目录不受此配置控制。

发布、归档校验和桌面镜像打包的 Python 命令在解析参数前统一将标准输出、错误输出设为 UTF-8，保留中文帮助与日志；不依赖构建机的本地代码页。GitHub Actions 同时设置 `PYTHONUTF8=1`（启用 Python UTF-8 模式）和 `PYTHONIOENCODING=utf-8`（明确重定向日志编码），避免 Windows runner 输出中文时出现 `charmap` 编码异常。

构建工作流使用基于 Node.js 24 运行的官方 Actions v6（checkout、setup-go、setup-node、setup-python、upload-artifact）。Action 自身的运行时与 `setup-node` 安装的应用构建工具是独立的；项目仍使用工作流中固定的 Node/Go/Python 版本。

```powershell
# 维护者先准备真实版本工具材料及摘要清单，再构建Windows完整包。
python scripts/release.py --target windows-amd64 --tools-manifest 'D:\release-inputs\windows-amd64.json'
# PowerShell等价入口。
.\scripts\release-windows.ps1 -ToolsManifest 'D:\release-inputs\windows-amd64.json'
```

```sh
# Linux两架构分别核验工具和运行依赖。
sh scripts/release-linux.sh --target linux-amd64 --tools-manifest /srv/release-inputs/linux-amd64.json
sh scripts/release-linux.sh --target linux-arm64 --tools-manifest /srv/release-inputs/linux-arm64.json
```

默认输出到 `bin/releases`，拒绝覆盖既有归档。ZIP/tar.gz 内有 `release.json`、`checksums.sha256`、中文使用说明及文档，归档旁另有 `.sha256`。文件排序、归档时间/所有者固定；可重现性仍要求相同源码、工具链和输入材料。

## 可选 Windows 漫画运行时

默认基础包不包含 JMComic 私有 Python。完整漫画包由维护者显式提供 `--jmcomic-runtime-manifest`；当前只接受 `windows-amd64`，不得将其作为 Linux 或 ARM64 运行时。装配流程及离线参数见 [Windows 原生漫画](jmcomic-windows.md#构建端装配)。

```powershell
# 装配至全新目录；依赖由哈希锁文件控制，终端不运行pip。
python -B scripts/project_env.py -- python -B scripts/jmcomic_runtime.py assemble --output .runtime/jmcomic-runtime
# 文件清单校验与实际依赖探测分开执行。
python -B scripts/project_env.py -- python -B scripts/jmcomic_runtime.py verify .runtime/jmcomic-runtime/jmcomic-runtime.json --probe
# 将运行时及许可成套纳入发行；媒体工具材料仍须独立审核。
python -B scripts/project_env.py -- python -B scripts/release.py --target windows-amd64 --tools-manifest 'D:\release-inputs\windows-tools.json' --jmcomic-runtime-manifest .runtime/jmcomic-runtime/jmcomic-runtime.json
# PowerShell包装入口的等价可选参数，脚本须保留UTF-8 BOM。
.\scripts\release-windows.ps1 -ToolsManifest 'D:\release-inputs\windows-tools.json' -JMComicRuntimeManifest '.runtime\jmcomic-runtime\jmcomic-runtime.json'
```

运行时清单严格记录目标、解释器和依赖版本、锁文件摘要、来源、许可及每文件大小／摘要，打包与归档校验同时核对普通文件、路径别名和 PE 架构。完整保留 native 扩展、DLL、CA、包数据和 `.dist-info`；不能用复制开发 venv、只放 `.py` 或启动时联网补包代替。只在全部验证通过后发布；文件摘要检查不替代许可审核、DLL 完整闭包或干净机器验收。

程序及运行时放只读安装目录，配置／任务／秘密留在 profile。更新先正常退出本实例并等待 worker／Python 释放，再成套替换文件；不覆盖用户任务和检查点，不自动升级依赖。

## 可选 Windows 视频运行时与下载器

视频包通过 `--uav-runtime-manifest` 显式装入独立 UAV embedded Python，当前只支持 `windows-amd64`。UAV 3.8.4 固定到 commit `d40f09457fbd38dd7dc94580637242afafaf3939`；解析依赖、构建依赖和派生 wheel 摘要分别记录，保留原始源码来源身份。UAV 和 JMComic 可同时装入同一个发行包，各自拥有独立解释器、依赖目录和文件清单。

```powershell
# 联网仅发生在维护者装配阶段，终端不运行pip或自动升级依赖。
python -B scripts/project_env.py -- python -B scripts/uav_runtime.py assemble --output .runtime/uav-runtime
python -B scripts/project_env.py -- python -B scripts/uav_runtime.py verify .runtime/uav-runtime/uav-runtime.json --probe
# 离线装配需另提供官方Python归档、依赖wheel及wheel-sources.json、固定UAV源码和构建wheel目录。
python -B scripts/project_env.py -- python -B scripts/uav_runtime.py assemble --offline --output .runtime/uav-runtime-offline --python-archive 'D:\release-inputs\python-3.14.4-embed-amd64.zip' --wheelhouse 'D:\release-inputs\uav-wheels' --source-archive 'D:\release-inputs\uav-source.tar.gz' --build-wheelhouse 'D:\release-inputs\uav-build-wheels'
```

完整视频包还必须同时提供固定官方 Windows `yt-dlp.exe` 与 FFmpeg/ffprobe 材料。下载器版本固定为 `2026.10.26`，不会使用开发机 PATH 中的另一个下载器。`scripts/ytdlp-materials.schema.json` 描述输入材料，`scripts/ytdlp-inventory.schema.json` 描述人工核对的完整组件库存；二者全部字段有中文说明。

下载器材料必须包括固定 EXE、官方 LICENSE 和 THIRD_PARTY_LICENSES、GPL 文本、固定 yt-dlp 源码归档，以及实际 EXE 所含 Python、PyInstaller 和其他组件的对应源码及必要构建材料。维护者必须先完成组件和分发审核，再填写审核声明；装配工具不会自动下载、生成占位声明或推定法律完整性。库存必须绑定实际 EXE 摘要，所有许可和源码材料均有组件引用。仅复制一份通用许可证或只有 yt-dlp 源码不足以通过装配。

```powershell
# 将已审核的真实材料装入全新目录；输出路径不得已存在。
python -B scripts/project_env.py -- python -B scripts/ytdlp_runtime.py assemble --materials-manifest 'D:\release-inputs\ytdlp-materials.json' --output .runtime/ytdlp-runtime
python -B scripts/project_env.py -- python -B scripts/ytdlp_runtime.py verify .runtime/ytdlp-runtime/licenses/yt-dlp/yt-dlp-manifest.json --probe
# 完整视频包；附加 --jmcomic-runtime-manifest 可生成漫画与视频组合包。
python -B scripts/project_env.py -- python -B scripts/release.py --target windows-amd64 --tools-manifest 'D:\release-inputs\windows-tools.json' --uav-runtime-manifest .runtime/uav-runtime/uav-runtime.json --ytdlp-manifest .runtime/ytdlp-runtime/licenses/yt-dlp/yt-dlp-manifest.json
# Windows PowerShell 5.1和PowerShell 7使用相同参数含义。
.\scripts\release-windows.ps1 -ToolsManifest 'D:\release-inputs\windows-tools.json' -UAVRuntimeManifest '.runtime\uav-runtime\uav-runtime.json' -YtDlpManifest '.runtime\ytdlp-runtime\licenses\yt-dlp\yt-dlp-manifest.json'
```

`--uav-runtime-manifest` 缺少下载器或媒体工具时拒绝构建，不能与 `--development-without-tools` 组合。包名用 `-jmcomic`、`-uav` 表示实际内容；单独携带下载器使用 `-ytdlp`，并不宣称视频插件完整可用。运行与本地诊断见 [Windows 原生视频](uav-windows.md)。

新生成 `release.json` 使用 schema **3**，保留原有字段，并显式包含可为空的 `jmcomicRuntime`、`uavRuntime`、`ytDlp`。UAV 摘要精确为 `manifest`、`sha256`、`pythonVersion`、`uavVersion`、`uavCommit`；下载器摘要为 `manifest`、`sha256`、`version`。归档验证继续接受旧 schema 1/2 的原字段集合，不会把新字段误塞入旧 schema；旧完整包由原 `tools` 清单声明并逐项核验的额外工具继续按原契约处理，schema 3 才要求下载器独立材料清单。发布清单版本与 `release-images.json` 镜像清单版本相互独立。

## 工具材料清单

精确 schema 见 `scripts/tools-manifest.schema.json`，每个字段都有中文解释。以下是结构说明，不是可直接发布的伪造摘要样例：

| 字段 | 含义 |
| --- | --- |
| `schemaVersion` | 固定 1 |
| `target` | 与构建目标一致 |
| `version` | 实际固定工具构建版本，不采用移动的 latest |
| `sourceUrl` | 对应该构建的 HTTPS 来源 |
| `runtimeFiles` | 每项 `source`、`destination`、真实小写 `sha256`；包含 `tools/ffmpeg[.exe]`、`tools/ffprobe[.exe]` 和必要 DLL/共享库 |
| `licenseFiles` | 此构建实际许可及第三方声明，目标位于 `licenses/ffmpeg/` |
| `sourceFiles` | 与二进制准确对应的源码及必要构建材料，同样位于 `licenses/ffmpeg/` |

`source` 可为绝对路径或相对清单的路径；成品清单会移除构建机绝对路径。构建器分块核对源摘要、复制后再次核对，并检查主要工具 PE/ELF 的目标架构。该检查不替代维护者对 GPL/LGPL、外部编解码组件、动态库依赖、对应源码和分发义务的审核；不能仅复制一份通用许可证冒充已完成许可核验。

两个必需工具必须使用上述固定小写名称，不能以 `FFMPEG` 等大小写别名代替。`licenses/ffmpeg/tools-manifest.json` 由构建器生成，不能作为输入材料的目标。全部包内路径都要能在两端安全解压，因此拒绝控制字符、Windows 设备名、尾部点或空格等歧义名称。归档核验会严格检查 JSON、三组材料与实际文件摘要、工具架构和不可变镜像清单；重新计算外部摘要不能掩盖材料缺失。

Windows shared 构建必须列出匹配的 DLL；Linux shared 构建必须携带或明确满足目标系统的共享库。发布前在相应干净机器执行 `ffmpeg -version`、`ffprobe -version` 以及真实封面/音频探测，不把架构头检查当作运行验证。构建器不自动下载未知二进制。

## Docker Desktop 镜像资源

应用/助手目前没有公共发布镜像。维护者可在已运行的本机 Linux Docker 环境先构建，然后生成固定本地 ID 的离线包：

```sh
# 构建镜像是维护者显式操作；首次可能拉取基础镜像与依赖。
docker --context desktop-linux build --platform linux/amd64 -t qiku-app:local .
docker --context desktop-linux build --platform linux/amd64 -f Dockerfile.engine-helper -t qiku-helper:local .
# 需要网页/远程复制时显式构建获取服务。
docker --context desktop-linux build --platform linux/amd64 -f Dockerfile.collection-worker -t qiku-collection:local .
# 只保存现有镜像，不启动容器；拒绝远程context。
python scripts/package_desktop_images.py --context desktop-linux --app-image qiku-app:local --helper-image qiku-helper:local --output bin/desktop-images
# 三镜像模式：选择不同输出目录，生成可部署组件的v3清单。
python scripts/package_desktop_images.py --context desktop-linux --app-image qiku-app:local --helper-image qiku-helper:local --collection-image qiku-collection:local --output bin/desktop-collection-images
# 将校验过的离线镜像归档一并装入Windows包。
python scripts/release.py --target windows-amd64 --tools-manifest /path/to/windows-tools.json --images-manifest bin/desktop-images/release-images.json
```

`release-images.json` 的 `schemaVersion=1`保留双镜像；`schemaVersion=2`必须再提供`collectionImage`。新打包器生成 **v3**，允许双镜像或附带`collectionImage`，并显式启用四类固定资源组件部署。所有镜像引用和实际ID必须不同，均为真实本地 `sha256:<64位>` 或 `repository@sha256:<64位>`。含获取镜像的包必须有`compose.desktop.collection.yaml`；v3还必须包含`compose.desktop.components.yaml`，启动器按顺序叠加并严格核验。旧v1/v2不隐式授权新卷。组件镜像由用户在平台确认后按固定摘要另行拉取，未包含在应用/助手离线归档中。

离线时 `archive` 为 `images/` 下包内相对路径，`archiveSha256` 为真实摘要，两项必须同时出现。使用本地ID且不带归档时，终端 Docker 必须已存在相同镜像。面向在线部署时，维护者先自行发布真实镜像，再填写其不可变仓库 digest；不接受 latest/普通标签。部署操作与首次配置见 [资源组件](components.md)。

获取镜像中的yt-dlp通过`deploy/collection-requirements.txt`固定版本和wheel摘要，系统FFmpeg/rclone版本记录在`/app/system-tools.txt`。正式再分发前需将对应许可证、构建版本、源码/提供源码材料与最终镜像digest一起归档；yt-dlp、rclone和FFmpeg分别遵循其实际发行许可，不能以主应用许可替代。当前源码构建配方不等于已完成镜像许可审核或真实容器运行验收。

可在用户自己的 Docker daemon 配置中设置已验证的镜像加速器。脚本不硬编码第三方镜像域名，不更改系统 daemon 配置，摘要核验始终保留。

## 检查与开发包

```sh
# 发布边界回归；不运行Docker。
python scripts/verify_release.py --self-test
# 校验归档内外摘要、全部文件及必需入口，不解压不可信路径。
python scripts/verify_release.py bin/releases/实际归档.zip
# 未准备工具材料时仅生成明确标识的开发包。
python scripts/release.py --target windows-amd64 --development-without-tools
```

开发包文件名含 `-development`，`release.json` 中 `developmentWithoutTools=true`，中文说明明确未携带 FFmpeg。它用于构建/嵌入/生命周期验证，不能宣称完整离线媒体发行。CI 的三目标矩阵只生成这种开发附件，不自动发布成品。完整发行的工具许可、签名、干净机器运行及 Docker 实机部署仍需单独完成验收。

本期未实现安装器、代码签名、自动更新或跨平台数据迁移。升级请按 [Windows 使用文档](windows.md) 保留配置/数据，先退出旧程序再换包。
