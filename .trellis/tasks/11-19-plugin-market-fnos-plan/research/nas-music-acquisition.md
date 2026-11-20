# Research: NAS 音乐获取、整理与播放边界

- Query: 在 GitHub 选择 NAS 音乐获取工具，区分下载、导入整理和媒体服务。
- Scope: mixed；以官方仓库和文档为外部证据，结合本项目既有音乐功能。
- Date: 2026-11-19

## Findings

建议音乐首先采用“自有 CD 转录／已购买下载／已有文件同步 → NAS 目录 → 可选 beets 整理 → 本项目或 Navidrome 播放”。歌曲文件、专辑元数据和在线播放服务是三种不同输入；不能把元数据刮削当作下载音乐。

| 项目与官方仓库 | 职责及选择建议 |
| --- | --- |
| [beets](https://github.com/beetbox/beets) | 导入已有文件、匹配 MusicBrainz 等元数据、纠正标签与目录，适合维护自有音乐库；基础能力不是歌曲来源。官方提供 Python CLI 安装，不在此承诺官方 NAS 应用包或官方 Docker 镜像。导入可以改标签和移动文件，接本项目时应在专用整理目录执行，再交给媒体库扫描。 |
| [Lidarr](https://github.com/Lidarr/Lidarr) | 按艺人、专辑监控 RSS 并安排 BT／Usenet 下载和整理。至少配置一个自己可访问的索引器与下载器，不是免费曲库。需要自动补专辑时再考虑。 |
| [Navidrome](https://github.com/navidrome/navidrome) | 扫描并播放自己的音乐收藏，属于个人音乐服务，不提供歌曲资源；有官方 Docker／Compose 部署，多种 Linux 架构，音乐目录可只读挂载。适合追求专门音乐客户端生态的用户。 |
| [Jellyfin](https://github.com/jellyfin/jellyfin) | 管理并流式播放已有媒体，适合统一影音服务；不能代替内容获取。有官方多架构 Linux 容器镜像；Windows 应使用原生安装，官方不支持在 Windows 宿主的容器中运行。 |

**当前可用性提示**：Lidarr README 明确提示元数据服务器正在恢复和重建缓存，影响添加艺人与音乐库导入；关联 issue #5498 页面嵌入的当前状态为 `OPEN`。不能表述为“已完全恢复”，也不能据此认定项目停止维护；建议默认先走自有文件导入，自动订阅作为部署前验证项。

**维护核验**：本次成功读取四个官方 README 和部署文档；GitHub API 返回 403 rate limit exceeded，因此改以官方发布入口的 HTTP 重定向核实版本：`beetbox/beets/releases/latest` 指向 `v2.14.1`，`navidrome/navidrome/releases/latest` 指向 `v0.64.2`，`jellyfin/jellyfin/releases/latest` 指向 `v12.4`。这些是查询时可见版本，不表示已验证 NAS 部署；不以页面可访问就断言维护活跃度。

## Files Found / Code Patterns

- `README.md:73`：已有音乐目录扫描和常见音频格式支持；`README.md:80`：原文件流式播放与内嵌封面，无须为获取文件而部署额外播放器。
- `internal/app/music.go:17`：`audioMIME` 集中定义 MP3、M4A、AAC、FLAC、OGG、OPUS、WAV、AIF／AIFF 类型；`internal/app/music.go:66`：`enrichMusic` 读取现有文件并有界探测，不是音乐来源接口。
- `.trellis/spec/backend/runtime-contract.md`：源媒体不得被本项目改写，媒体增强异步执行。
- `.trellis/spec/backend/download-contract.md`：qB 执行下载、选定文件完成后才发布，外部整理服务不能直接接管已有受管下载任务。

## External References

本研究的 web.run 调用未返回可复用的来源标识，因此不伪造引用 ID；已通过项目环境中的 Python urllib 直接读取下列官方来源，最终答复可链接对应官方页面。

上述表格提供官方 GitHub 链接；补充官方文档：

- [beets 入门及导入策略](https://beets.readthedocs.io/en/stable/guides/main.html)；[Lidarr 快速开始](https://wiki.servarr.com/lidarr/quick-start-guide)。
- [Navidrome Docker 安装](https://www.navidrome.org/docs/installation/docker/)；[Jellyfin 容器安装](https://jellyfin.org/docs/general/installation/container/)。
- [Lidarr 元数据服务问题 #5498](https://github.com/Lidarr/Lidarr/issues/5498)。
- [beets 发布](https://github.com/beetbox/beets/releases)、[Navidrome 发布](https://github.com/navidrome/navidrome/releases)、[Jellyfin 发布](https://github.com/jellyfin/jellyfin/releases)。

## Caveats / Not Found

- 未部署、未连接用户索引器／帐号，无法保证具体专辑的覆盖、质量或可下载性。
- Docker 部署文档不等于飞牛原生 FPK 兼容证据；ARM 镜像架构、目录权限、外部元数据连通性仍应按宿主核实。
- 本轮仅为选型，未安装工具、未改变媒体标签、未修改业务代码。
