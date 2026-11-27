# Research: NAS 通用音视频下载与已有文件导入

- Query：从 GitHub 选择适合 NAS 的音视频获取工具，比较网页下载、BT 下载和自有云文件导入。
- Scope：mixed；仅调研，不安装或修改业务代码。
- Date：2026-11-19（Asia/Shanghai）。

## Findings

| 选择 | 获取方式与适用场景 | NAS 部署及核验依据 |
| --- | --- | --- |
| **yt-dlp** | 命令行音视频下载，可处理受支持站点的单个视频、播放列表及音频提取；适合接入统一任务队列。它需要实际可访问的内容 URL，不能视为影片资源库。 | 官方提供 Linux/Windows 等发布文件；合并音视频、后处理通常需要 FFmpeg。发布页当前标记稳定版 `2026.10.26`。citeturn827049search3turn827049search4turn552994view0 |
| **MeTube** | yt-dlp 的自托管 Web 界面，浏览器粘贴链接即可下载到 NAS；适合希望少配置的用户。支持视频/音频、播放列表和频道。 | 官方 README 提供 Docker/Compose，镜像支持 amd64/arm64；下载目录绑定 NAS 媒体目录。发布页当前标记 `2026.11.14`。citeturn368487search1turn552994view1 |
| **qBittorrent** | 已有磁力/种子时优先，负责 BT 传输及任务管理；检索和订阅来源仍需配置或由其他工具提供。 | 官方有无桌面 `qBittorrent-nox` Docker 镜像，支持 x86/ARM 等架构。发布页当前标记 `5.3.0`，不等于本项目已验证该版本。citeturn564294view2turn827049search1turn552994view2 |
| **rclone** | 将自有网盘、对象存储、WebDAV/SFTP 等已有影音复制到 NAS；建议增量 `copy` 后再扫描入库。`mount` 提供远程文件视图，不能当成本地副本。 | 官方维护 Docker 镜像，也可原生运行；`copy` 不删除目标已有文件，`sync` 才使目标与源一致。发布页当前标记 `1.76.0`。citeturn564294view0turn564294view1turn827049search0turn827049search2turn552994view3 |

**推荐组合**：网页视频用 **MeTube → NAS 指定目录 → 媒体扫描**；BT 用 **qBittorrent + 既有 Prowlarr/Jackett → 下载完成后入库**；自有云文件用 **rclone copy → 本地目录 → 媒体扫描**。这是按工具职责给出的集成建议，尚未实机部署。前两项外部职责依据见上表；本项目既有能力见下节。

## Files Found / Code Patterns

- `README.md:20`：现有 BT 下载中心连接 qBittorrent，完成验证后入库。
- `README.md:21`、`README.md:114`：支持已有 Prowlarr/Jackett 的 Torznab 搜索，默认关闭，需要用户配置来源。
- `internal/app/download_qbittorrent.go:243`：当前应用校验 qBittorrent 5.x、WebAPI 2.11 及以上；仍需以集成验收确定具体版本。
- `internal/enginehelper/templates.go:25`：受管引擎固定使用 `5.2.4-1` 镜像及摘要；调研不应自动升级。

## External References

- yt-dlp 官方仓库：https://github.com/yt-dlp/yt-dlp
- MeTube 官方仓库：https://github.com/alexta69/metube
- qBittorrent 官方仓库：https://github.com/qbittorrent/qBittorrent
- qBittorrent 官方容器：https://github.com/qbittorrent/docker-qbittorrent-nox
- rclone 官方仓库：https://github.com/rclone/rclone
- rclone 安装、复制、挂载：https://rclone.org/install/ 、https://rclone.org/commands/rclone_copy/ 、https://rclone.org/commands/rclone_mount/

## Related Specs

- `.trellis/workflow.md`：研究落盘，不直接进入实现。
- `.trellis/spec/backend/download-contract.md`：下载完成验证及入库边界。
- `.trellis/spec/backend/torrent-search-contract.md`：配置来源及受控下载转交。
- `.trellis/spec/backend/engine-bootstrap-contract.md`：固定引擎与可选助手边界。

## Caveats / Not Found

- 官网明确：列在 yt-dlp 支持列表中的站点也可能因改版而失效，实际链接需测试；不能承诺某视频平台全部可下载。citeturn827049search5
- 以上用于能够访问且有权保存的公开或自有内容，不提供 DRM 或付费访问绕过方案。
- 四个项目页面未见归档提示，均核实到 2026 年发布；只据此说明近期有发布，不能保证未来维护或任意 NAS 兼容。具体 fnOS 应用中心/FPK 兼容性未测试。citeturn552994view0turn552994view1turn552994view2turn552994view3
- aria2/OpenList 可作为直链下载或远程聚合的补充，本轮不纳入优先推荐，避免与既有下载层重复。
