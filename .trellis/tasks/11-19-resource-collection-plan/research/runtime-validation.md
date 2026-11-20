# 多来源资源收集运行验收

日期：2026-11-20，UTC+08:00（Asia/Shanghai）。所有本地命令由 `python -B scripts/project_env.py -- ...` 启动，缓存、临时数据和隔离样本位于仓库 `.runtime/`。未操作用户账号、真实媒体或部署中的 NAS。

## 已执行的验证

| 范围 | 实际结果与证据 |
| --- | --- |
| 主服务、真实 worker、本地只读登记/归档复制、远程复制 | 最终 `verify_collection.py` 32项通过，使用真实 rclone v1.76.0 和本地 alias remote，含BT删库关联撤销与文件保留；见 `.runtime/verify/collection-final-reviewed/result.json`。 |
| RSS 与 BT 桥接 | 上述集成覆盖真实 HTTP RSS 的基线、ETag、增量去重、候选确认，再经受控 qB 协议替身、人工选文件、真实发布/扫描和实际媒体读取；未传输公网 BT 载荷。 |
| 网页视频与提取音频 | 真实主服务、worker、yt-dlp 2025.12.08、FFmpeg；隔离1秒 MP4 fixture，分别产出9263字节 MP4和2235字节 MP3，关联实际 itemIds；鉴权206精确字节及未鉴权401通过。见 `.runtime/verify/web-app-e2e-44ty2igp3r/result.json` 和 `worker-tool-versions.md`。 |
| 来源、任务持久化与恢复 | 原请求幂等、新编号重复指纹409、秘密分离、schema v3完整性、主服务与worker重启、worker断线后已发布媒体读取，均包含在32项集成中；更多不确定派发、独立入库重试见 `backend-verification.md`。 |
| 浏览器 | 真实 Edge/Playwright 36项，四种宽度、两主题、焦点/Escape、来源/订阅/候选/任务、实际入库播放、请求响应丢失与批量部分失败；见 `browser-acceptance.md`。 |
| 原下载流程 | `verify_downloads.py` 36项通过；真实主程序与受控qB，包含五类型下载入库、重启、选文件、独立入库重试、外部同hash拒绝。证据 `.runtime/verify/downloads-collection-final/result.json`。 |
| 原搜索流程 | `verify_torrent_search.py` 43项通过；真实主程序与受控Torznab/qB。证据 `.runtime/verify/search-collection-final/result.json`。 |
| 原托管接入 | `verify_engine_bootstrap.py` 35项通过；使用受控助手/协议服务，未操作Docker。证据 `.runtime/verify/engines-collection-final/result.json`。 |
| 五类媒体、音乐、备份、Range | 真实FFmpeg/FFprobe样本与 `verify.py` 52项通过，schema v3备份包含下载及收集表；5GiB稀疏样本仅证明大偏移协议。证据 `.runtime/verify/media-final-20261120-095335/passed.txt`。 |
| 发行边界 | `verify_release.py --self-test` 76项通过，含v1/v2固定镜像、overlay、归档路径/架构/摘要边界；不代表构建了正式容器镜像。 |
| 桌面部署模板 | 真实Compose CLI规范化v1/v2模板；8项模板变更和6项清单用例通过。另有实际container-inspect环境、用户及禁止提权矩阵临时用例通过；见 `quality-review.md`。 |
| 跨平台编译 | Linux amd64/arm64及Windows amd64主程序、worker、helper，以及Windows desktop，共10个目标通过；产物位于 `.runtime/verify/cross-final/`。这是交叉编译证据。 |

前端严格类型检查与生产构建、Go vet及包编译均通过，最终收尾记录由下文追加。临时行为测试执行后按AGENTS要求删除；保留 `scripts/verify_collection.py` 和 `scripts/verify_collection_browser.py` 供复验。测试删除后的空套件不计为行为覆盖。

## 最终修订复验

最后BT持续同步与未绑定目标删除修订完成后，主程序于2026-11-20 10:00:30重新构建，SHA-256 `ed52b05ee100e843585c6fcf2daee6230d225bb1faf9125de6c4dcf38106f227`。该程序通过32项收集集成与36项浏览器复验；Go vet、清理后全包编译和10目标交叉构建再次通过。浏览器最终目录为 `.runtime/verify/collection-browser-reviewed/`；已检查1440px浅色和375px深色截图，弹窗内容及按钮留白、换行和边界正常。

网页真实工具E2E在最后BT专属修订前的主程序上执行，其准确二进制摘要保留在worker报告中；最终BT修订没有改变网页/worker实现。原下载/搜索/托管/五媒体回归也在最后BT镜像修订前执行，最后修订本身由19项后端行为测试和最终32项集成中的删库/原BT桥接覆盖。不得将这些不同轮次改写成同一二进制上的全量实测。

收尾检查：gofmt无待格式化文件；7份变更Python源码AST解析及6份任务JSON有效；Trellis implement14/check9条上下文校验通过；`git diff --check`通过。新增临时Go测试和误留仓库根EXE已清除，隔离app/worker进程枚举为0。`.runtime/verify`证据目录保留，含随机测试凭据的文件不作为发行材料。没有提交、归档或部署。

## 可复用命令

在仓库根运行，每次 `--root` 必须使用全新且含 `verify` 路径段的隔离目录：

```powershell
python -B scripts/project_env.py -- .tools/go/bin/go.exe vet ./...
python -B scripts/project_env.py -- .tools/go/bin/go.exe test ./...
python -B scripts/project_env.py -- npm --prefix web run build
python -B scripts/project_env.py -- python -B scripts/verify_collection.py --binary .runtime/verify/nas.exe --worker-binary .runtime/verify/nas-collection-worker.exe --rclone .runtime/verify/tools/rclone/rclone.exe --root .runtime/verify/collection-new
python -B scripts/project_env.py -- python -B scripts/verify_release.py --self-test
```

浏览器先用 `verify_collection.py --serve` 创建隔离实例，再按 `browser-acceptance.md` 运行；停止文件结束该实例。验证数据中有随机测试凭据，不能作为用户配置或发布材料。

## 未覆盖的部署条件

- 当前Docker daemon不可用，没有构建/运行worker镜像，也没有执行三容器安装、NAS UID/GID/跨挂载权限、实际容器完整停止或离线整包验收。
- Linux/ARM为交叉编译，未在对应内核运行进程组、文件系统和权限测试；Windows本地服务及进程树行为有实际执行证据。
- 实际公网网页样例的DNS返回保留地址 `198.18.1.164`，连接策略正确拒绝。测试仅替换代理连接到隔离HTTP fixture，生产策略未放宽；不承诺所有站点或HLS-only/登录/DRM/JS挑战来源。
- rclone alias不是云盘账号验收，受控qB不是公网BT做种/载荷验收；容器固定yt-dlp 2026.10.26仍需目标环境验证。
- 磁盘耗尽、真实跨卷/网络卷故障及目标NAS性能仍需部署样本验收；代码拒绝不安全发布，不能据此宣称所有设备故障已实测。

部署说明见 `docs/collection.md`。本轮代码交付与外部来源/目标平台验收分别记录，任务保留待部署核验项。
