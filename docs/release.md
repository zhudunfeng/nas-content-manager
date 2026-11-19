# 维护者跨平台发布流程

终端用户运行成品包不需要 Go、Node、Python 或全局 FFmpeg。以下步骤仅在构建机执行，不会自动发布 GitHub Release、注册表镜像或安装 Docker。

## 构建目标与输入

`scripts/release.py` 支持 `windows-amd64`、`linux-amd64`、`linux-arm64`，构建网页并暂存到 `internal/webassets/dist`，以 `webembed` 标签内嵌。暂存有独占构建锁并在结束后清理；不要并发操作暂存目录。Linux/Windows 共用后端，Windows 包另含托盘 EXE。

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
# 只保存现有镜像，不启动容器；拒绝远程context。
python scripts/package_desktop_images.py --context desktop-linux --app-image qiku-app:local --helper-image qiku-helper:local --output bin/desktop-images
# 将校验过的离线镜像归档一并装入Windows包。
python scripts/release.py --target windows-amd64 --tools-manifest /path/to/windows-tools.json --images-manifest bin/desktop-images/release-images.json
```

`release-images.json` 的 `schemaVersion=1`，`appImage/helperImage` 为真实本地 `sha256:<64位>` 或 `repository@sha256:<64位>`。离线时 `archive` 为 `images/` 下包内相对路径，`archiveSha256` 为真实摘要，两项必须同时出现。使用本地ID且不带归档时，终端 Docker 必须已存在相同镜像。面向在线部署时，维护者先自行发布真实镜像，再填写其不可变仓库 digest；不接受 latest/普通标签。

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
