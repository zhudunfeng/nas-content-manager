# 仓库现状与影响分析

调研日期：2026-11-18。使用 CodeGraph CLI（status/query/impact）定位符号，再读取对应实现和部署文件。本文为规划证据，不代表新功能已经实现。

## 已确认现状

| 位置 | 已有行为 | 初始化功能需要处理的变化 |
| --- | --- | --- |
| `Dockerfile` | 主程序以 `1000:1000` 运行，没有 Docker CLI、socket 或部署接口 | 普通应用不能凭一个 HTTP 请求凭空获得宿主部署能力；需要用户首次接入可选助手 |
| `compose.yaml` | 基础单应用；仅应用数据卷及只读媒体挂载 | 保留精简默认模式，新增独立托管覆盖文件，不替换基础 Compose |
| `compose.downloads.yaml` | 事先挂载专用下载目录，只授权 complete 为媒体根；密码通过 secret | 托管模式必须在首次部署助手时就给应用预挂载下载根及只读连接配置目录 |
| `compose.downloads.qbittorrent.yaml` | 固定官方 qB `5.2.4-1` 镜像及摘要；WebUI 不发布，peer TCP/UDP 发布；要求上游告知确认 | 托管模板复用版本、目录和网络约束，不改成 latest 或不同发行镜像 |
| `compose.search.jackett.yaml` | 固定 LinuxServer Jackett 镜像及摘要；config 持久化、自动更新关闭 | 托管首版选 Jackett；已有 Prowlarr 仍手动接入，不同时部署两种索引服务 |
| `internal/app/config.go:30` | `LoadConfig` 一次性读取两个模块配置 | 需要区分静态部署边界与可激活的模块运行状态，不能在线改环境变量冒充配置热更新 |
| `internal/app/download_config.go:26` | 下载启用时，启动前强制校验密码文件、根路径和挂载隔离 | 只有 managed 模式允许连接凭据尚未生成；external 模式保留现有严格校验 |
| `internal/app/torrent_search_config.go:30` | 开启搜索后来源文件必须有 1–8 条；禁用不读文件 | 新增“引擎已安装、来源待配置”状态，不能用空 sources 强行通过原 external 文件校验 |
| `internal/app/download_sync.go:21` | 禁用时协调器不会启动；启用后 client 生命周期固定 | 增加一次性、可恢复、幂等的托管激活，不可反复运行 `startDownloads` 产生重复 worker |
| `internal/app/downloads.go` | 多个入口读取静态 `a.config.Downloads.Enabled`；磁力只允许已实测的 qB `5.2.4` | 路由和协调器改为读取统一有效运行状态；版本白名单保留，不因自动安装而放宽 |
| `internal/app/torrent_search.go` | 独立 manager 保存 immutable 配置、HTTP client、短期令牌与有限缓存 | 托管来源刷新需要 generation 与请求隔离，不可无锁修改 manager.config.Sources |
| `internal/app/jobs.go` | worker 仅路由 scan/enrich；任务与 library/item 关联 | 引擎拉取属于部署生命周期，不塞进媒体扫描队列 |
| `internal/app/store.go` | 主业务数据库 schema v2；启动恢复媒体任务 | 推荐 helper 使用独立部署状态库，避免为部署控制面修改媒体数据库或共享SQLite连接 |
| `internal/app/server.go` | 已有单管理员会话、同源修改保护和 64KiB 普通请求上限 | 新引擎 API 复用现有鉴权与错误格式，禁止浏览器直接调用 Docker/helper |
| `web/src/components/DownloadCenter.vue` | 已有下载连接卡、测试按钮、搜种与添加入口 | 在同页加入引擎管理入口与首次引导，不增加第六个手机底部导航 |
| `web/src/components/TorrentSearchDialog.vue` | 未配置时仅给部署提示；搜索和下载在线状态独立 | 可引导初始化搜索引擎；下载引擎离线时仍允许搜索 |

## CodeGraph 影响摘要

`codegraph impact DownloadConfig -j` 返回 15 个节点，覆盖配置加载、路径映射、qB client、发布和对账。运行时激活不能只改 `config.go` 或界面按钮；必须检查所有实际读取配置的调用点。

原 `downloadManager`、`torrentSearchManager` 及 App 生命周期均已定位。没有已存在的通用部署器可复用；CodeGraph 未检索到相应 Docker 任务模块。新增助手应是独立职责，而不是扩展已有媒体子进程执行工具。

## 需要保留的现有契约

- 固定下载根及 complete/staging 门禁，五类媒体读写边界，已发布内容在引擎离线或助手失联时仍可读取。
- 原选集、任务所有权标签、持久化幂等键和发布后入库流程。
- 搜索私密来源只驻留服务端；浏览器不拿私密 URL、Key、Cookie、磁力和原始上游数据。
- 大视频仍由 qB 直接写卷，应用通过原 Range 接口读取，助手不转发载荷。
- 外部 qB/Prowlarr/Jackett 配置继续有效，连接失败不转为托管接管。
- 基础 Compose 无额外默认服务；不自动提交、归档此前任务或清理已有未提交业务代码。

## 本轮边界

只编辑本任务的规划文件与当前任务指针。不实现业务功能、不执行 Docker、不启动应用或引擎，也不重复已有搜种验收。后续自动验收使用 fake Docker HTTP、fake engine 和真实应用；真实容器启动/拉取/网络/NAS权限由用户手工验证。
