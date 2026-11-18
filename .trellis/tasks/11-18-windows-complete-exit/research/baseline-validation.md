# Windows 完整退出：基线运行证据

日期：2026-11-18。所有构建和样本位于当前项目 `.runtime/`，没有运行 Docker 或访问真实用户配置/媒体。

## HTTP 慢读复现

- 未修改的 `./cmd/nas-desktop` 构建为 `.runtime/windows-exit/baseline.exe`。
- 使用扩展后的 `scripts/verify_desktop.py`，将 EXE 复制到隔离中文目录，从另一工作目录启动独立 profile。首次创建测试管理员、注册小说库，通过认证请求打开 5 GiB 稀疏 EPUB 协议样本，以小 TCP 接收窗口停止读取。该样本不代表可解码 EPUB/视频。
- 真实 Windows 零共享打开证明请求期间源文件被占用。通过本机命名管道 `quit` 后，旧代码记录「媒体库连接关闭超过等待预算，正在终止连接」，命中慢读退出小于 8 秒的回归断言失败。
- 日志中扫描完成时间为 16:41:31.6238198，关闭预算耗尽日志为 16:41:42.152241。没有把该退出延迟描述成每次永久死锁。
- 原始脱敏输出：`.runtime/windows-exit/baseline-desktop-repeat.log`。

## IPC 偶发失败观察

首次执行相同流程时，`quit` 客户端报「已有实例返回的控制响应无效」，随后脚本仅结束其自建 Popen 实例并清理样本。报错位置是管道读取失败或计数无效分支，不是 JSON 解码分支。第二次成功发送 quit 并复现上述慢流延迟。此项已交实施代理继续定位，尚不能将偶发行为当作独立必现根因。

首次输出：`.runtime/windows-exit/baseline-desktop.log`。两次脚本的临时 profile、媒体和复制 EXE 均随 TemporaryDirectory 回收。

中间实现版本也执行了真实独立进程验证：慢流 harness 和五媒体 harness 在最终 quit 均报「已有实例的控制响应读取失败：本机控制通道响应超时」。后者的 52 项媒体/Range/备份断言通过，但整条退出验证仍失败，不能记为完整成功。对应 `.runtime/windows-exit/fixed-desktop.log`、`media-regression.log`；实施代理随后用跨进程服务端阶段诊断继续定位。单进程内连续 status 测试通过不足以排除这一问题。

## 媒体后代进程

实施代理已用 Windows 原生临时 Go fixture 在基线复现两例：父工具仍运行与父工具先结束。取消后 4 秒，孙进程继承的 stdout 管道仍阻塞命令；清理通过精确进程 HANDLE 完成。完整命令、断言与修复后对照结果由 `implementation.md` 保存，不能以研究方案代替运行证据。

## 交互验收边界

可用桌面 UI 工具无法提供任务栏/托盘入口；未使用外部 Win32 点击助手绕过。后续原生菜单 dispatch 与真实消息泵验证必须标注为内部交互链测试，不得写成鼠标物理点击验收。
