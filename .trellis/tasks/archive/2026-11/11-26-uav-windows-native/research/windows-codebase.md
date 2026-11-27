# UAV Windows 仓库现状调查

调查日期：2026-11-26。对象为已有 Jable、MissAV、Hanime1、黄果、SupJav 五站插件；本轮只交付方案。

## 检索与证据边界

- 已优先通过本机 CodeGraph CLI 查询 `startManagedCollection`、`JMComicRuntimeManifest`，当前索引均返回空数组；后续使用限定目录的 `rg` 和准确文件读取，不全量重建索引。
- 本轮起始工作区干净；既有 UAV、SupJav、JMComic Windows 实现均属于现有基线。
- 下面行号是本次调查快照，实施时需重新解析，不作为永久接口。
- 开发机能够创建 venv、运行真实依赖的离线探测，不代表 Windows 干净机或真实公网站点验收通过。

## 已存在的能力

| 位置 | 现状 | 方案影响 |
| --- | --- | --- |
| `docs/uav.md:24–59` | Windows 默认解释器为 `python`，已有独立 venv 手工安装和配置步骤 | 无需重写站点解析；保留管理员手工部署路径 |
| `internal/collectionworker/uav.go:65–120` | 固定上游 probe；通过 stdin/stdout JSON 调用嵌入桥接；参数为 `-I -B -c` | 新增包内运行时解析和 UTF-8 参数即可复用协议 |
| `internal/collectionworker/uav.go:89–98` | UAV 同时要求 Python、yt-dlp、FFmpeg；当前失败原因较笼统 | 只带 Python 不能宣布视频下载已就绪 |
| `internal/collectionworker/service.go:158–170,293–307` | 下载器已有本地版本探测；HTTP capabilities 已分别包含 jmcomic/uav | 复用能力协议，主要修正桌面对它的消费 |
| `internal/collectionworker/tools.go` | 固定 argv、无 shell、受控环境、下载限额和私有工作目录 | Windows 修复不能放开任意命令或环境继承 |
| `internal/collectionworker/jmcomic.go:65–133` | 已有显式解释器优先、清单校验、托管无 PATH 回退、独立失败原因 | UAV 按此模式补齐，不能直接使用 JMComic 清单 |
| `internal/desktop/collection_managed.go` | 自有 worker、随机回环端口、私有继承管道、鉴权及根目录验证 | 复用单个托管 worker，不另开视频服务 |
| `internal/desktop/collection_config.go` | collectionMode 已有 disabled/managed/external；老配置迁移保守 | 无需为本方案增加数据库或桌面配置版本 |
| `internal/jmcomicruntime/`、`scripts/jmcomic_runtime.py` | 已有 Windows embedded Python 装配、逐文件摘要、架构、证书、路径边界校验 | 仿照其契约新增 UAV 运行时；只抽取确有必要的共用校验 |

## 阻止普通用户直接使用的缺口

1. **UAV 无私有运行时入口。** `config.go:34–35` 只有 enabled/python，没有 runtime manifest 和解释器显式配置标记；`uav.go:69` 直接查工具路径。
2. **桌面只接漫画。** `collection_managed.go:105–125` 仅注入 JMComic 配置；`:274–276` 仅保存 jmcomic capability；`collection_controller.go:27–50` 和 `shell_windows.go:334–338` 文案、状态也仅面向漫画。
3. **UAV-only 包无法自动托管。** `collection_config.go:28–44` 只检查相邻 worker 和 `jmcomic-runtime.json`。应改为识别至少一个受支持插件运行时，真实有效性仍由 worker 校验。
4. **下载器没有随包。** `scripts/release.py:95–123` 的工具清单固定 FFmpeg/ffprobe；`deploy/collection-requirements.txt:2` 虽锁定 yt-dlp 2026.10.26，但 Windows release 没有装配它。现有 `s.ytdlp` 是可执行路径，不能直接替换为一段 `python -m ...` 命令字符串。
5. **缺少本地视频诊断。** `cmd/nas-collection-worker/main.go:25–34` 有 `--check-jmcomic`，没有 `--check-uav`。
6. **发布流程未装配 UAV。** `release.py:265–340` 仅有可选 JMComic runtime 和文档复制；PowerShell 入口也仅透传该参数。
7. **发布清单严格限制字段。** `verify_release.py:150–152` 只接受 release schema 1/2，各自要求精确字段集合；新增 UAV/下载器应演进 release schema 3，同时保留旧版本读取。
8. **UTF-8 不能只靠环境变量。** UAV 目前运行于 `-I`；方案应按 JMComic 现有调用增加固定 `-X utf8`，对中文目录、中文标题和 stdin/stdout 实测。
9. **UAV 有源码安装来源校验。** `internal/collectionworker/uav_bridge.py:388–392` 验证 `direct_url.json` 内源码归档摘要；JMComic 的纯 wheel 解压流程不能直接照搬。装配须保留经验证的固定源码 provenance，同时记录派生 wheel 身份、移除构建机路径并覆盖 RECORD/清单检查，不能弱化现有 probe。

## 版本与兼容约束

- 上游继续固定 UAV 3.8.4、commit `d40f09457fbd38dd7dc94580637242afafaf3939`，源码归档 SHA-256 为 `88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41`，见现有文档与契约。
- 既有 JMComic runtime 固定 CPython 3.14.4；它只是 UAV 优先验证候选，不能继承“已验收”的结论。新增 UAV runtime 应独立装配并记录真实 probe。
- UAV 与 JMComic 锁定依赖存在版本差异；即使 Python 同版本，也分目录、分清单、分 site-packages。
- Linux/Docker 已有独立 Python 环境与工具路径，不改变其默认行为或强制依赖 Windows 清单。
- SupJav 候选线路、原生解密后 TS 清理、FFmpeg 本地校验、公网代理、取消与发布门禁保持现有契约。
- Windows Job/句柄回收复用现有机制，只管理本实例进程；升级须等待 worker 和工具释放文件后再替换，不按进程名结束 Python。

## 设计落点

推荐“现有桌面托管 worker + UAV 独立 embedded runtime + 固定下载器 + 共用 FFmpeg/ffprobe”。具体修改、配置、阶段和验收见任务根目录 `info.md`；官方分发调查见 `windows-runtime-options.md`。
