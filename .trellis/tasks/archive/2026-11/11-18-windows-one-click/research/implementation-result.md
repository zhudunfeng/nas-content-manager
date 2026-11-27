# 双平台与Windows一键启动实现结果

日期：2026-11-18。范围：批准计划P0–P3；P4安装器、签名、自动更新和跨平台数据迁移未纳入。代码保持未提交，任务保留review供人工部署验收，不自动归档。

## 交付内容

- 共享内核：`RunWithOptions`、数据库前数据目录锁、移交真实listener、本实例令牌就绪、有序关闭；Linux/Windows平台锁和媒体子进程隔离。
- 资源/工具：`webembed`内嵌网页及动态块、外部目录显式覆盖、安全FFmpeg/ffprobe统一解析，不依赖当前工作目录查找工具。
- 桌面：`cmd/nas-desktop`与`internal/desktop`实现Windows原生托盘、目录授权、当前用户私有IPC、严格配置持久化、端口避让、独立profile及重启/退出。
- 可选完整模式：本机Linux/amd64 Desktop context、固定镜像/归档、最终Compose JSON预检、实际资源归属校验、单只读媒体根、Linux命名卷及先引擎后Compose的精确停止。
- 助手：六角色命名卷、持久角色/安装身份绑定、SQLite打开前租约、固定private stop-all、幂等对账和停止后禁止新启动；原Linux bind默认行为保留。
- 发布/规范：统一三目标脚本、CI开发构建矩阵、严格归档校验、可复用桌面验证器、Windows/发布/引擎操作说明和七节可执行code-spec。

## 最终归档与实际执行

以下均为**缺少FFmpeg的development验证包**，不是已审核完整离线发行版。输出位于忽略目录`.runtime/windows-one-click/verified-releases-20261118/`，每份都有外部`.sha256`和内部逐文件校验：

| 目标 | 归档 | 实际执行 |
| --- | --- | --- |
| Windows amd64 | `qiku-0.1.0-windows-amd64-development.zip` | Windows GUI EXE带headless参数执行完整隔离harness，通过 |
| Linux amd64 | `qiku-0.1.0-linux-amd64-development.tar.gz` | 已安装WSL1中解压至Linux临时目录，从任意cwd启动，通过 |
| Linux arm64 | `qiku-0.1.0-linux-arm64-development.tar.gz` | 交叉编译/归档校验通过，未运行ARM64程序 |

三个最终归档均由`verify_release.py`逐包验证通过，每包8份文件。开发包命名、元数据、中文说明和缺少工具材料状态一致。

最终Windows包解压到含中文/空格目录，再执行：

```powershell
python scripts/verify_desktop.py --binary '.runtime/windows-one-click/最终 验证包 20261118/qiku-0.1.0-windows-amd64-development/栖库.exe'
```

结果为`passed`、`resourceMode=embedded`；覆盖12组检查：中文/空格/分号路径、任意cwd、实际端口占用避让、健康/HTML、真实命名管道、重复启动、同data跨profile拒绝、有序退出、本机重启保留实例/账号、重复JSON键保留原文件、全部保存字段missing/null/空相对dataDir拒绝，以及原媒体字节不变。没有通过禁用网卡模拟物理断网。

Linux最终归档运行输出：

```json
{"passed":true,"target":"linux-amd64","runtime":"WSL1","archiveExecution":true,"unrelatedCwd":true,"embeddedAssets":2,"gracefulSignalExit":true,"dockerExecuted":false}
```

实际检查内嵌HTML及其CSS/JS、SPA、缺失脚本404、SQLite位置和SIGTERM退出0。ARM64不作实际运行声明。

## 行为和质量证据

- 共享运行时：Windows与WSL1实际测试，详情见`runtime-implementation.md`。WSL1端口复用异常子例明确排除；未降低产品监听语义。
- Desktop：8个临时Go顶级测试、33个Compose配置变异负例及真实二进制集成通过，见`desktop-implementation.md`。
- Helper：9个临时行为测试通过，1项WSL1 fsuid正向能力明确跳过，见`helper-implementation.md`；实际写权限探针继续严格执行。
- 业务回归：`verify.py --large-file`52、`verify_downloads.py`36、`verify_torrent_search.py`43、`verify_engine_bootstrap.py`35，共166项断言通过。真实本地FFmpeg用于五媒体样本、音频封面和元数据；引擎使用受控qB/Torznab，未执行真实关键词搜站或BT载荷。
- 质量审阅：最终Go vet/test/build、前端strict typecheck/生产build、Python编译、52项发布边界回归和diff检查结果详见`quality-review.md`。临时Go测试删除后的`go test`仅算编译检查，不冒充保留行为套件。
- 原`compose.yaml`和新`compose.desktop.yaml`均离线config解析通过；Desktop临时行为测试还验证实际规范化JSON及33个错误变异。没有Docker运行验证。

## 清理和人工验收边界

- 本轮生成的临时`*_test.go`均已执行后移除。最终检查`internal/`、`cmd/`无残留临时测试源。
- 归档构建后`internal/webassets/dist`与`.runtime/release-build.lock`均不存在；构建暂存和独占锁已释放。
- 主任务及桌面验证器创建的本地应用进程均已退出。首次原生UI隔离实例通过固定quit正常停止，日志确认安全关闭。
- 未启动Docker、未创建容器/镜像/网络/卷、未访问旧远程daemon；不存在本轮Docker测试资源清理事项。未安装/修改Docker、WSL或系统网络配置。
- 完整发行需要实际固定版本工具及逐文件摘要、匹配运行库、许可证和对应源码/构建材料。当前已有移动版本FFmpeg只用于本地测试，不作为可再分发的固定版输入。
- 干净Windows11普通用户完整离线五媒体、目录选择器/全部托盘交互、真实Docker Desktop/WSL2权限与完整引擎初始化/停止、ARM64实机、物理断网、长时大视频性能仍待对应环境人工验收。WSL1不是Docker Desktop/WSL2的替代证据。
- 新版本Docker镜像替换/回滚需要另行明确的版本化停止重建流程；现实现拒绝接管与当前清单不符的旧镜像容器，不静默升级。

用户入口：`docs/windows.md`、`docs/release.md`、`docs/engines.md`与`docs/verification.md`。
