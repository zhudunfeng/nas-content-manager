# Windows 程序同级用户配置最终验证

## 结果与实现

最终检查通过，完成时间（UTC）：`2026-11-26T06:16:26.591045+00:00`。Windows 默认 profile 为真实 `栖库.exe` 旁的 `qiku-data/profiles/native`，Docker 使用独立的 `docker` profile；只有原生业务数据库默认跟随 profile 的 `data`，Docker 实际业务数据仍在 Linux 命名卷。目录不随 CWD 或 LOCALAPPDATA 改变，无权限时明确提示 `--profile`，不静默回退 C 盘用户目录。

显式 `--profile`、`--data-dir`、适用 NAS 环境及已保存配置优先级保持。已有绝对业务/媒体路径和实例身份不改写；不自动迁移、覆盖或删除旧 C 盘 profile。升级需保留完整 `qiku-data`。

## 实际检查

- 临时 Go 行为测试 3 个顶层组、5 个目录子场景通过，覆盖默认位置、CWD/LOCALAPPDATA、目录优先级、状态专用子树、归属和业务/媒体重叠拒绝。源码复制到运行证据目录的 `.go.txt` 后已删除；清源后的 `go test ./...` 为编译证据，不称保留了长期测试。
- 修改 Go 文件 gofmt、全包 `go vet ./...`、清源全包编译、3 个 Python 入口 AST、Vue 严格类型检查与生产构建通过。
- 最终内嵌 Windows GUI 产物真实托盘回归通过：20 类桌面检查及 10 种图标尺寸，含不传 profile 的首次启动、任意 CWD、环境/CLI 覆盖、模式隔离、重复启动、5 GiB 稀疏慢流退出、EXE/数据库/日志释放、原位覆盖和账号/实例重启保留；慢流/空闲退出为 0.090/0.048 秒。这是本次实测值，不是一般性能承诺；稀疏样本不证明视频解码或吞吐。
- 实际 worker 的 11 类托管回归通过，新增不传 profile 的程序内默认布局，实际核对配置、数据库路径、owner 身份、data/secrets/tmp、worker 后代启动及退出释放。未配置可选运行时，插件显示 unavailable，与 worker 通道就绪分开记账。
- 最终内嵌 EXE 的真实 Windows DACL 拒写已观测：默认目录中文失败、无 LocalAppData 回退，显式可写 profile 真正就绪并保存数据库/日志，正常退出并恢复 ACL。
- 发布边界自检 131 项及 Windows 开发包的 23 文件成员/摘要校验通过。
- Linux amd64/arm64 的 CLI、worker、非 Windows 桌面占位入口共 6 个产物交叉编译通过；未在 Linux 运行，不代表新增 Linux GUI 支持。
- 独立 Trellis 审查及任务 context validate 通过；未自动提交或归档。

## 产物与证据

- 桌面 EXE：`.runtime\releases\windows-portable-profile\extracted\qiku-0.1.0-windows-amd64-development\栖库.exe`，SHA-256 `b34ae1803ba0b505717e37fe9fa178b5cd36945cc49711c28ded9dd3f3b258b1`。
- 开发验证归档：`.runtime\releases\windows-portable-profile\qiku-0.1.0-windows-amd64-development.zip`，SHA-256 `8247d2f0b2131022439b9fa475a5554ce7ffbcc5a8bc2a81464dba56946d665e`。该包内嵌网页和 worker，未携带 FFmpeg/可选私有 Python 运行时，不能称完整离线发行版。
- 原始报告：`.runtime/verify/windows-portable-profile/final-result.json`；实际托盘 `final-embedded-desktop-tray.stdout.log`，托管 `final-collection-result.json`，只读 `final-readonly/result.json`，其余构建/验证双流以 `final-` 命名。
- 初轮断言误把整个 LOCALAPPDATA 根存在当作配置回退。诊断 `localappdata-members.json` 只有 `Google/Chrome/User Data/Crashpad`，系重复启动 open IPC 打开默认浏览器所致；没有 `Qiku` 配置。已修正为检查栖库专有 `LOCALAPPDATA/Qiku` 不存在，同时保留 EXE profile 与实际 dataDir 的断言，重跑通过。失败证据保留在 `initial-failure-final-result.json` 和同前缀双流，不计入通过数量。

## 证据边界

本机真实 Windows 与隔离样本通过。干净普通用户 Windows VM、真实公网站点、完整插件/FFmpeg 包、Docker 实际部署及真实 Linux 执行未实测；本轮不变更数据库 schema、旧库内容或 Linux CLI 默认值。
