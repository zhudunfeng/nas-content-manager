# UAV 视频搜索与下载

UAV 是可选的视频搜索下载来源，使用 [UAV-Downloader](https://github.com/Alos21750/UAV-Downloader) 的站点搜索和视频解析能力。首版适配 **Jable、MissAV、Hanime1、Huangguo（黄果）**，通过现有资源收集任务下载、发布并扫描到视频库。上游其它站点、完整 GUI/Web 队列、账号和 Cookie 配置不包含在本次适配范围内。

## 使用

1. 在“资源收集 → 来源”添加 UAV 视频插件来源，填写名称并启用。
2. 确认概览中的 UAV 能力可用。能力探测检查依赖，不代表真实站点当前可达。
3. 打开视频搜索下载，选择来源和站点，输入关键词搜索。搜索只显示文字元数据，不加载站外封面。
4. 选择一条视频并读取预览，确认标题和目标视频库，提交“下载并入库”。
5. 在原收集任务详情查看获取、发布和扫描状态。只有实际扫描覆盖到视频后才显示已入库，并可通过现有视频播放器播放。

每次只获取明确选择的视频。搜索、预览不会自动下载全部结果。预览有时效；更换来源、站点或视频后需重新确认。提交结果未知时保留弹窗并重试原请求，系统使用相同请求编号核对，避免重复下载。

黄果搜索结果按实际单集展开，编号例如 `huangguo-42-ep-3`，表示作品 42 的第 3 集；作品根页只代表第 1 集，不会整剧下载。为保持现有搜索预算，一页最多返回 100 条单集，展开使用 22 秒截止时间。超出条数或时间会明确失败，请缩小关键词；不会静默丢弃后续集数并当作成功。

## 部署

插件依赖仅在独立 `nas-collection-worker` 安装，NAS 主服务不运行第三方 Python。主服务与执行端的令牌、共享根和媒体根配置继续使用[资源收集部署](collection.md)，无需数据库迁移。

| 参数 | 默认值与中文解释 |
| --- | --- |
| `NAS_COLLECTION_WORKER_UAV_ENABLED` | 原生 `false`；显式启用固定 UAV 适配器。专用镜像内置依赖时启用。 |
| `NAS_COLLECTION_WORKER_UAV_PYTHON` | 原生 Linux 默认 `python3`、Windows 默认 `python`；应指定已安装固定依赖的可信独立环境，容器为 `/opt/uav/bin/python`。 |

上游固定提交为 `d40f09457fbd38dd7dc94580637242afafaf3939`，标识版本 3.8.4，要求 Python 3.12 或更高版本。不要将项目现有 JMComic 环境覆盖成 UAV 环境。执行端还需要可运行的固定 yt-dlp 和 FFmpeg；依赖只在管理员部署或镜像构建时安装，搜索请求不会执行安装或升级。

### Docker / NAS

仓库现有收集覆盖文件包含独立 worker：

```sh
docker compose -f compose.yaml -f compose.collection.yaml up -d --build
```

需要重建新版 `Dockerfile.collection-worker` 才会具备插件依赖；只更新主应用页面不会升级旧 worker。UAV 使用独立 Python 3.12 构建阶段与 `/opt/uav` 环境，已有 JMComic 环境继续独立保留。构建固定源码归档并校验 SHA-256，安装 `deploy/uav-build-requirements.txt` 和 `deploy/uav-requirements.txt` 的版本与哈希；运行时不安装包。首次构建需要访问对应镜像及依赖源。此处是部署命令，本次没有执行 Docker 或目标 NAS 运行验收。

### 原生执行端

先按[原生执行端配置](collection.md#原生windows对接)准备专用数据根、令牌、共享根与主应用映射。下面以 Windows 仓库开发环境为例；第一个 `python` 必须是 Python 3.12 或更高版本：

```powershell
# 创建项目内独立环境，保留系统及其他插件依赖。
python -B scripts/project_env.py -- python -m venv .runtime/uav-venv
# 只安装有版本与哈希锁的无头运行和构建依赖。
python -B scripts/project_env.py -- .runtime/uav-venv/Scripts/python.exe -m pip install --only-binary=:all: --no-deps --require-hashes -r deploy/uav-build-requirements.txt -r deploy/uav-requirements.txt
# 安装固定提交归档，URL中的摘要校验源码；禁止自动安装GUI/AI依赖。
python -B scripts/project_env.py -- .runtime/uav-venv/Scripts/python.exe -m pip install --no-deps --no-build-isolation "https://codeload.github.com/Alos21750/UAV-Downloader/tar.gz/d40f09457fbd38dd7dc94580637242afafaf3939#sha256=88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41"
# 离线探测固定版本与导入闭包，不访问视频站。
'{"action":"probe"}' | python -B scripts/project_env.py -- .runtime/uav-venv/Scripts/python.exe -I -B internal/collectionworker/uav_bridge.py
# 只为当前终端启动的worker启用插件。
$env:NAS_COLLECTION_WORKER_UAV_ENABLED = 'true'
# 解释器必须位于实际运行worker的机器。
$env:NAS_COLLECTION_WORKER_UAV_PYTHON = (Resolve-Path .runtime/uav-venv/Scripts/python.exe).Path
```

Linux 使用独立环境的 `bin/python`，并使用对应 Python ≥3.12 创建环境。仍需启动 worker、配置原有 yt-dlp/FFmpeg 路径，并按资源收集文档将主服务连接到该 worker；这些命令不替代原认证与路径授权。上游项目元数据同时声明桌面及 AI 功能包，本适配器有意只安装固定无头导入闭包，因此不将全量上游 `pip check` 作为通过结论。成功探测和离线解析也不代表真实站点可用。

## 下载与故障处理

- UAV 桥接只执行固定站点的搜索与详情解析。实际传输走执行端固定下载器，所有 HTTP/HLS 请求及重定向受公网策略约束；FFmpeg 仅处理本地文件。
- HLS 仅接受有限完整清单与 HTTPS 资源；资源通过操作专属转发，普通 AES-128 密钥严格为 16 字节。直播、缺片、GAP、增量/低延迟清单、未知扩展标签、分离音轨和不支持的加密不会发布为成功视频。按字节范围取得的分片必须收到匹配的范围响应，不能把服务器忽略 Range 后的整文件当作分片。黄果使用普通 MPEG-TS，暂不接入其 fMP4 资源。
- 黄果只接受有限 HLS 单集。其匿名播放响应中的短期 CDN 访问值保留在私有桥接和操作内存；执行端仅为已解析清单中的资源建立临时转发，每跳重新核对原始 HTTPS 源与资源类型，结束或取消即撤销。浏览器和下载器不会取得全局 CDN 认证头。
- 下载暂存在该任务专属目录，沿用字节、文件数、并发、磁盘余量及最长执行时间限制。检测到传输失败、取消、缺片或产物校验失败时不会授权发布。
- 下载后用 FFmpeg 完整解码收到的文件。HLS 还将解码时长与清单中的 EXTINF 总时长比较，允许最多 `max(0.5 秒, 预期时长的 2%)` 偏差，拒绝明显丢尾；这不能证明容差内或内容语义上的完整性。直链 MP4 没有可信的预期时长，只能核验传输与收到的媒体可解码，不能据此证明原片全长。
- 下载完成后独立校验产物，再按原流程发布和扫描。获取完成与入库完成是两个状态。扫描失败可单独重试入库，保留已发布文件。
- 插件关闭、缺依赖或执行端离线时显示不可用原因，历史任务和已发布视频继续保留。主服务其它资源收集入口可独立使用。
- 站点解析失败可能来自网络、站点改版、访问限制或不受支持的媒体。错误会脱敏显示，不回显原始 HTML、媒体直链、代理凭据或工具 stderr。
- 不接受任意站点地址、脚本、命令参数、输出目录或凭据。需要认证或付费权限的资源不由此插件自动处理。

## 验证范围

本地协议样例、真实依赖的离线适配检查、真实公网搜索下载、Docker 运行和目标 NAS 验收分别记录。执行结果见[功能验证记录](verification.md)，不能由依赖探测成功推断真实站点或下载已经通过。

上游许可证与分发说明见[第三方声明](third-party-notices.md)。
