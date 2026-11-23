# 当前可用构建（2026-11-23）

这是基于当前工作区生成的 **Windows x64 开发可用版**，版本号仍为 `0.1.0`。它包含已有媒体管理、账号安全和经校验的数据库快照下载；本轮优先交付可运行成品，S03.2.3 手动持久备份执行器尚未接入。

## 立即使用

已经解压的程序位于仓库：

```text
bin/usable-20261123/qiku-0.1.0-windows-amd64-development/栖库.exe
```

先正常退出旧的栖库托盘实例，再双击此文件。首次启动会选择媒体目录并在服务就绪后打开网页，创建管理员后即可使用。关闭浏览器不退出服务，退出请使用托盘菜单。旧版本已有账号时，升级前先在设置页下载数据库快照，并另行备份配置/数据目录；不要直接复制运行中的 WAL 数据库。

控制台或局域网方式使用同目录 `nas.exe`，配置仍按 [README](../README.md#环境参数) 的 `NAS_*` 参数；桌面入口默认仅回环访问。新包没有自动运行、接管或停止用户现有实例。

## 本包边界

- 两个 EXE 均已内嵌本轮构建的网页，无需运行 npm 或复制 `web/dist`。
- **未捆绑 FFmpeg/ffprobe**，不是完整离线发行版。需依照 [Windows 说明](windows.md#本机配置) 配置现有工具，才能获得完整的封面生成和音视频探测。检查中显式使用了本机已有 FFmpeg，不代表包内自带工具。
- 当前数据库头版本为 6；备份策略仍关闭。设置页下载继续生成一次性的数据库快照；定时、手动持久备份 API、历史下载、轮转和恢复准备 UI/CLI 尚未开放。
- 备份仅覆盖数据库，不包含原媒体、组件卷或外部秘密材料；同盘副本不等于异地备份。已有索引、账号、收藏和进度应随升级前的备份一同妥善保存。
- 真实 Linux/NAS、Docker 部署、介质故障和断电仍需对应设备验证。本轮没有部署或自动 Git 提交。

## 构建与验证

本轮重新执行前端类型检查和生产构建，并通过现有 `scripts/release.py` 生成开发包；产物是当前源码构建，不是复制上轮旧 EXE。包内包含 `release.json` 和逐文件 `checksums.sha256`，ZIP 旁也有 SHA256 文件。

同一个 `nas.exe` 已通过核心媒体 **52**、备份下载 **38**、账号在线/离线 **162**、备份存储迁移 **17** 项集成断言，共 **269** 项。桌面 EXE 另以无托盘模式验证内嵌网页、中文路径、端口避让、单实例、慢流中断、退出后文件释放和立即重启；本轮未实际操作原生托盘菜单。

| 文件 | 字节 | SHA256 |
| --- | ---: | --- |
| `qiku-0.1.0-windows-amd64-development.zip` | 13,434,468 | `63fae8eda5f9b5b3e693350951ecbd25a9b84f0104b0309f93b82ae33bb72c74` |
| `nas.exe` | 15,505,920 | `8b5f58c721573387c9133134ada1e3fd79249db329b27deec7aa4e465579156e` |
| `栖库.exe` | 15,735,296 | `ba0c2a5cb3afdf278f710cfd4ea1e15aa78ee2053dff71adbcdf10d093258220` |

本轮 ZIP 位于 `bin/releases/usable-20261123/`。运行证据保存在 `.runtime/verify/usable-20261123-*`，其中包含仅限本机保留的测试数据库和账号材料，不应整体上传。源码和回归读回见 `.runtime/verify/usable-20261123-mainline/readback.json`；已有 S03.2.2 的历史证据未被改写。

复现构建命令：

```powershell
# 所有命令的缓存与临时文件均限定在仓库 .runtime 中。
python -B scripts/project_env.py -- npm --prefix web run build
# 输出目录必须是新目录，构建器拒绝覆盖既有 ZIP。
python -B scripts/project_env.py -- python -B scripts/release.py --target windows-amd64 --development-without-tools --skip-web-build --go .tools/go/bin/go.exe --output bin/releases/usable-20261123-rebuild
```

后续继续实现 S03.2.3 时，须重新按其冻结契约完成行为验证；隔离保存的未接线草稿不属于本包已实现功能。
