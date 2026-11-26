# Windows UAV 实施与验证记录

日期：2026-11-26（Asia/Shanghai）。用户已授权按计划实施。本记录区分源代码完成、本机实际执行、协议样例和目标设备验收，不将开发包或离线 fixture 声明为正式完整发行。

## 实现范围

- 独立 `uav-runtime.json`、`runtimes/uav/`、`licenses/uav/`，与 JMComic 分离；共享底层普通文件、路径、PE 和 wheel 辅助校验。
- UAV 3.8.4 固定 commit `d40f09457fbd38dd7dc94580637242afafaf3939`。从原始固定源码构建 wheel，保留原源码 `direct_url.json` 身份，绑定运行依赖、构建锁、派生 wheel、RECORD 和文件清单。
- worker 显式解释器优先且无效不回退，托管禁止 PATH 解释器；固定 `-I -B -X utf8`，中文分原因诊断与 `--check-uav`。
- 单 worker、多插件独立能力，旧 disabled/external 配置及固定 IPC 保持兼容；托盘分别显示漫画/视频状态。
- 固定官方 yt-dlp.exe 材料装配、组件库存与许可/对应源码闭包检查；release schema 3，继续读取旧 schema 1/2。

## 已完成本机检查

所有命令均通过 `python -B scripts/project_env.py -- ...`，未更改用户级环境或全局 Python。开发机为 Windows AMD64；以下记录不代表干净 Windows 11 普通用户机器。

| 检查 | 实际结果与证据 |
| --- | --- |
| Vue/TypeScript 与 Vite | `npm --prefix web run build` 退出 0，188 modules；未改前端源文件 |
| Go | `go vet ./...`、`go test ./...` 退出 0；空测试集只计编译 |
| Windows 三入口 | 独立复核后，主服务、worker、桌面入口已重建于 `.runtime/bin/uav-windows-final-20261126/`；构建选项、命令和摘要见下方最终候选记录 |
| Linux 双架构 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64/arm64 go build ./...` 均退出 0；未执行 Linux 二进制 |
| UAV 真实运行时 | `verify_uav_runtime.py --manifest .runtime/tools/uav-private/uav-runtime.json` 70 项，784 文件；真实 CPython 3.14.4、bridge、AES、CA、Python/Go 交叉拒绝、空 PATH、污染 Python 环境、中文空格迁移 |
| JMComic 共享辅助回归 | `verify_jmcomic_runtime.py --manifest .runtime/tools/jmcomic-private/jmcomic-runtime.json` 49 项，含真实 embedded 和归档回归 |
| 协议集成初轮 | 真实主服务/worker/FFmpeg：UAV 102 项、JMComic 66 项；上游桥接/下载器为协议 fixture；UAV 固定下载器版本修复后另记最终回归 |
| 原核心与收集基线 | `verify_baseline.py --suite local --cases core,collection,release`：core 44、collection 27、release 126；清单 `.runtime/verify/uav-windows-baseline-20261126/run-manifest.json` |
| 下载器/发布边界 | 下载器 36、最终 release 131 项；包含 PowerShell 5.1/7 真实解析与中文空格参数透传、旧 schema 与三目标、UTF-8 重定向 |
| 实际基础开发归档 | `.runtime/release-uav-schema3-validation/qiku-0.1.0-windows-amd64-development.zip`，17 文件验证通过；SHA-256 `711e17a29539d15da22624d21c4af333a18826616ef68e5fcdf602e29d58ecdd`。明确未携带 FFmpeg/UAV，不作为完整视频包 |

运行时证据：

- `.runtime/verify/uav-runtime-result-0c533a5b55f14e87b5238b4af8b50f05/result.json`
- `.runtime/verify/jmcomic-runtime-result-b0c4f4938d2b4f0bbe5e76426ad3c749/result.json`
- UAV 清单 SHA-256 `5e4cc8298e7bbfc71c08a526c72b5494395ccf5ab2c5277b92bd04b2fa99d077`。
- 派生 UAV wheel SHA-256 `65472d6f06b5c806e3e06479f0447a60668a17ad159a4015195ba9a2b680e61c`。

## 实际私有 Python 与官方 EXE 的离线 HLS

外层使用 `verify_baseline.OwnedProcess` 独占 Windows Job，运行真正的 `runtimes/uav/python.exe -I -B -X utf8`。验证入口只注入仓库 `scripts`，依赖仍来自私有 site-packages；站点 HTTP 传输替换为固定样例并禁止未替换套接字。

命令记录 `.runtime/verify/uav-windows-real-bridge-20261126/command.json`：退出 0、无超时/清理错误。结果位于 `.runtime/verify/uav-real-bridge-13d117e4065749c8b4ca2315f5b29a1f/result.json`。

- 五站实际解析器：search 5、resolve 5、empty 5、bad 30、transportRedirect 2；SupJav provider 7、fallback 10、pagination 3、input 16。
- 实际官方 EXE SHA-256 `f5389d3698ecde4ef06f4e6b1ce0294fe9ac1396228c8990b8d3416d6d177a7d`，实际 `--version` 为 2026.10.26。
- 本机 HTTP 有限 HLS normal/AES-128 退出 0，均经真实 FFmpeg 解码；missing/unsupported 退出 1。顶层清单未重取、不启用 FFmpeg 网络回退。
- 本机 FFmpeg 来自已安装工具，用于验证；其完整正式发行材料未在本次审查。

## 独立复核与最终候选

Trellis check 已完成，详见 `quality-review.md`。修复 UAV 下载器精确版本门槛及 schema 1/2 合法额外工具被误拒绝的兼容回归。最终发布 self-test **131 项**通过。真实 UAV 专项归档直接调用生产校验器，785 成员正例和重算摘要后的 RECORD/来源/额外文件三负例共 **4 项**通过；证据 `.runtime/verify/uav-archive-quality-20261126/result.json`。专项归档并非完整发行包。

生产 Go 源稳定后，主流程以 `-buildvcs=false -trimpath` 构建最终三入口，目录 `.runtime/bin/uav-windows-final-20261126/`。构建前记录 **154 份**输入及摘要，命令和三个产物身份位于 `.runtime/verify/uav-windows-final-20261126/build.json` 与 `build-inputs.json`。

- 最终 worker 的 UAV 协议 **102 项**再次通过，结果 `.runtime/verify/uav-windows-final-protocol-20261126/result.json`；fixture 也已同步正确固定版本。
- 最终主服务/worker 的 core **44**、collection **27** 项再次通过；证据 `.runtime/verify/uav-windows-final-baseline-20261126/run-manifest.json`。
- `verify_desktop.py --with-tray --web-dir web/dist` 实际通过 **17 类**桌面回归及三个托盘图标/退出检查点，十种内嵌图标尺寸逐帧匹配；退出耗时 0.072/0.027 秒。该项使用真实桌面入口、中文/空格/分号目录、命名管道、5GiB稀疏样例流中断及程序文件替换；不作为5GiB真实媒体吞吐证据。
- 清除临时 Go 测试后，全仓 `go vet ./...`、`go test ./...` 仍退出 0。Python **11 份**改动文件 AST、全部方法中文说明和类型标注检查通过；这不是独立 Python 静态类型检查器结果。
- 完整真实桌面矩阵：JMComic-only **14 项**、UAV-only **20 项**、双插件组合 **24 项**均通过；组合实际观察到漫画 **26 个**、视频 **25 个**已映射私有 Python/原生扩展映像。覆盖本地诊断 0/2/1、错误固定下载器版本、解释器优先级、缺工具、双向部分故障隔离、只读程序 ACL、中文空格目录、IPC、取消/父强杀、嵌套 Job、自有子树回收、独立哨兵保留、映像替换及同 profile 重启。详细命令和夹具复盘见 `worker-desktop-validation.md`；JSON 在 `.runtime/verify/uav-windows-worker/desktop-{jmcomic,uav,combined}.json`。
- 多插件矩阵使用同一生产源码的默认 Go 构建，最终候选使用 `-buildvcs=false -trimpath`，因此 EXE 摘要不同。**154 份**生产输入逐项一致，矩阵 EXE 按同选项重建后逐字节一致；设置、两组产物摘要和来源证据见 `.runtime/verify/uav-windows-worker/build-provenance.json`。没有将独立夹具预检冒充桌面 Job 验收，也没有为修复夹具改变生产超时或依赖锁。

最终审计 `.runtime/verify/uav-windows-final-20261126/final_audit.py` 实际退出 **0**，同目录 `final-audit.json` 回读为 **passed**：154 份生产输入无新增/删除/摘要漂移，三份最终产物身份一致；11 份 Python、122 个方法的 AST/中文说明/入参出参注解通过；新增临时单元测试源码、精确最终产物活进程、发布锁和嵌入暂存均为空，diff 与 Trellis 上下文校验通过。该 AST 检查不等同独立静态类型检查器。原桌面 17 类与托盘结果来自实际命令输出，未另存独立机器报告，不虚构对应日志文件。

首次基础桌面矩阵在父进程强杀后，曾观察到进程句柄已退出但 worker 映像即时独占打开出现短暂 Win32 sharing error 32；只读快照无存活 worker。后续完整基础、单插件及组合矩阵未复现，未通过额外等待或放宽文件释放断言掩盖。该观察及夹具问题的根因、复盘与最终通过证据保留在 `worker-desktop-validation.md`。

## 明确未验事项

干净 Windows 11 普通用户、真实公网站点搜索/下载、Linux 实际执行、Docker、ARM64 依赖执行、UNC/超长路径 profile 及物理 NAS 尚未验收。没有自动提交、部署、发布、归档任务或操作用户实例。完整 yt-dlp EXE 对应源码/组件清单仍需维护者实际材料及人工分发审查；本次不生成虚假审核声明，也不声称已交付正式完整视频发行包。
