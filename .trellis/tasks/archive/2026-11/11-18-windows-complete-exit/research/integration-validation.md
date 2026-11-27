# Windows 完整退出：最终应用集成记录

日期：2026-11-18。所有实例、样本和可控缓存均在本仓库 `.runtime/` 中；没有运行 Docker、连接旧远程服务器或使用真实用户媒体/配置。

## 验收产物与范围

最终生产代码包含命名管道 Pinner 修复、Job 收尾前关闭新进程准入以及启动器清理错误传播。使用以下命令构建真实 Windows GUI 子系统桌面 EXE：

```powershell
python -B scripts/project_env.py -- go build -ldflags '-H=windowsgui' -o .runtime/windows-exit/validated.exe ./cmd/nas-desktop
```

产物 SHA-256 为 `74ba37de2a61e02c546a0c1f9889d4a115bd73c9eac871950ed31a6fdea0b9b6`。测试通过 `--headless --no-browser` 驱动相同启动器，显式使用已构建的 `web/dist`，不是完整离线发布包或内嵌前端验收。没有前端业务改动。

## 程序本体更新优先

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_desktop.py --binary .runtime/windows-exit/validated.exe --web-dir web/dist
```

最终完整运行通过，脱敏结果为 `.runtime/windows-exit/final-desktop.log`：

- 在中文程序目录运行构建产物副本，使用另一工作目录、独立 profile 和真实当前用户命名管道。
- 建立认证的 5 GiB 稀疏文件慢读连接，停止客户端继续读取；Windows 零共享打开确认退出前确实占用源文件。
- `quit` 后准确等待主进程，慢流退出 **0.775 秒**，空闲退出 **0.336 秒**；本轮同时存在其他受控验证/构建负载，这不是通用性能保证。
- `process.wait` 后首先对主 EXE 零共享读写、原位重命名/恢复、从原始构建产物实际覆盖相同路径并校验启动前摘要，再重命名/恢复整个隔离程序目录。此前没有负向 IPC 的五秒等待或人为休眠。
- 随后检查数据库、锁、日志、媒体可独占打开/重命名、HTTP 不再监听；首次退出后立即运行覆盖后的 EXE，同 profile 的账号、配置、实例身份和真实端口保持；最终退出再严格验证 IPC 明确报告实例不存在。
- 保留中文/空格/分号路径、端口实际避让、重复启动、跨 profile 共享数据根拒绝、损坏/缺失/null/空相对配置拒绝及源文件三个边界标记等既有验证。

隔离程序目录含本轮主 EXE；没有把未实际加载的包内普通文件当作共享 DLL 的卸载证据。自有工具映像的独占/覆盖与父孙进程回收由临时原生行为测试单独验证，详见实施报告。

## 五媒体与有序退出

最终 EXE 使用全新隔离 profile，指定本机已有 FFmpeg/FFprobe，运行 `scripts/verify.py --large-file`：**52 项断言通过**，包含视频、图片、漫画、小说、音乐、真实短视频/音频封面、鉴权、状态、SQLite 快照与 5 GiB 大偏移 Range。之后通过独立 EXE 命名管道客户端发送 `quit`，主进程退出码为 0，耗时 **0.274 秒**；主 EXE、数据库、锁、日志及全部源文件共 **25 项独占检查通过**。

结果为 `.runtime/windows-exit/final-media-result.json`、`final-media.log`。稀疏文件仅证明 HTTP 流生命周期和偏移语义，不能作为真实超大影片解码、吞吐或 HDR 性能证据。

## 共享业务与质量门禁

复用同一稳定版本的 `.runtime/windows-exit/checked-nas.exe`，三个既有验证脚本全部通过：下载编排 **36 项**、搜种 **43 项**、托管接入 **35 项**。使用真实本机 Go 应用和受控回环 qB/Torznab/助手协议替身，没有运行 Docker、查询真实站点或传输 BT 载荷；不能用这些结果代替引擎部署验收。对应 `final-downloads-result.json`、`final-search-result.json`、`final-engines-result.json` 和同名前缀日志保留在 `.runtime/windows-exit/`。

`gofmt`、`go vet ./...`、完整临时 Go 行为测试、Windows 构建、Linux amd64/arm64 交叉构建通过。三个新增临时 Go 测试源已删除，删后包编译与 vet 再次通过；详细行为矩阵和首次 fixture 修正见 `implementation.md`，独立复核见 `quality-review.md`。Race detector 因本机缺少 GCC 未运行，不记为通过。Trellis 上下文校验和临时测试删除后的 CodeGraph 同步通过。

## 隔离样本清理结果

最终只读枚举确认本轮 `.runtime/windows-exit/` 下的隔离应用进程为 **0**，桌面脚本的 `栖库 启动验证 *` 临时目录已正常回收；新增临时 Go 测试源也已删除。五媒体应用脱敏日志已另存为 `.runtime/windows-exit/final-media-application.log`。

精确清理 `E:\vscodeProjects\nas-content-manager\.runtime\windows-exit\verify` 的递归删除命令被工具自动审批策略拒绝，工具没有提供更具体原因，命令未启动。没有改用另一命令或 API 重试删除。该已忽略目录仍保留本轮自建样本、profile、数据库及随机测试凭据，**不能将其报告为已经清理或空间已回收**；日志与结果副本已保存到其父目录，后续可由用户手动处理。

## 交互与平台边界

原生托盘消息泵和产品菜单派发入口使用临时 Go 行为测试覆盖；本轮桌面 UI 工具不能提供实际系统托盘入口，未宣称物理鼠标点击验收。没有实测完整发行包的第三方共享 DLL 加载/卸载。Linux 目标构建与 Windows 主机行为测试分别记录，不把交叉编译描述为 Linux 实机执行。

修复前慢流预算耗尽、父孙工具残留，以及中间版本跨进程 IPC 超时仍保留在 `baseline-validation.md` 和对应日志中；只有最新产物的成功运行计入本次最终结果。
