# Windows desktop 图标实现与验证

日期：2026-11-19。状态：实现完成，待用户审阅；未提交 Git，未归档任务。

## 实现

- `assets/windows/qiku.ico`：沿用现有 SVG 品牌的深色圆角底与薄荷色双书架，透明 32 位图标，包含 16、20、24、32、40、48、64、96、128、256 像素十帧。初次导出使用本机已有 Pillow 12.3.0；它不是项目新增构建或运行依赖。
- `cmd/nas-desktop/resource_windows_amd64.syso`：固定版本 rsrc v0.10.2 生成的仅图标 COFF 资源，组 ID 1。已预置源码树，普通 Go 构建与原有 release.py 自动链接，Linux 忽略。
- `scripts/generate_windows_icon.py`：复用项目环境，提供再生与 `--check` 字节一致性检查；固定维护工具不加入主 go.mod，无系统安装要求。入口增加 `go generate` 指令。
- `internal/desktop/shell_windows.go`：分别加载拥有独立句柄的大小品牌图标，托盘使用小图标，窗口类设置 Icon/SmallIcon；资源失败有中文错误。托盘删除、窗口销毁、类注销之后才销毁图标，异常路径也销毁存活窗口。
- `scripts/verify_desktop.py --with-tray`：扩展现有可复用集成脚本，逐帧比较 PE 资源与源 ICO，随后释放资源映射；使用真实原生托盘代替 headless，检查首次启动与原位替换重启后的独立大小图标句柄。保留原来全部退出/文件占用断言。没有新增临时单测类或测试源。

## 实际验证

全部开发命令通过 `python -B scripts/project_env.py -- ...`，缓存、下载与样本在仓库内。

1. 资源生成后运行 `scripts/generate_windows_icon.py --check` 通过；再运行 `go generate ./cmd/nas-desktop` 报告字节一致、无需更新。
2. `go vet ./...`、`go test ./...` 及修改文件 gofmt 通过。项目没有保留 Go 单测，因此该 test 命令仅证明包编译；行为覆盖来自以下真实集成。
3. 普通 Windows GUI EXE：`go build -ldflags=-H=windowsgui -o .runtime/windows-icon/栖库图标验证.exe ./cmd/nas-desktop`。
4. `scripts/verify_desktop.py --binary .runtime/windows-icon/栖库图标验证.exe --web-dir web/dist --with-tray` 通过；十帧资源一致，原生托盘正常，5 GiB 稀疏慢流退出 0.094 秒，空闲退出 0.026 秒。
5. 原有 `release.py --target windows-amd64 --skip-web-build --development-without-tools --output .runtime/windows-icon/releases` 生成带内嵌网页开发包；归档结构及内外摘要验证通过（8 份文件）。
6. 提取该开发包的 `栖库.exe`，运行 `scripts/verify_desktop.py --binary .runtime/windows-icon/packaged/栖库.exe --with-tray`，不传外置网页路径，通过十帧资源、原生托盘和完整桌面回归。慢流退出 0.092 秒，空闲退出 0.030 秒。两次原生集成都先立即独占打开 EXE，再实际覆盖、校验摘要、重命名整个程序目录，并以同一配置重启，未用负向 IPC 等待掩盖文件占用。
7. Linux amd64/arm64 CLI 交叉编译通过；`scripts/verify_release.py --self-test` 68 项通过。
8. Trellis 新任务默认 check 上下文引用了当前项目不存在的 Claude check 命令，已替换成本项目实际 `.agents/skills/trellis-check/SKILL.md` 与 finish-work skill，随后 task validate 通过。CodeGraph sync 通过：4 changed files / 96 nodes。Python AST 及 Git 空白检查通过。
9. 最后只读枚举：本轮 `.runtime/windows-icon/` EXE 进程 0、`栖库 启动验证 *` 临时目录 0、`qiku-icon-*` 临时目录 0，发布构建锁及网页嵌入暂存不存在。仅保留本轮构建产物、日志与预览。

日志：`.runtime/windows-icon/native-verification.log`、`packaged-verification.log`、`release-build.log`。图标预览：`.runtime/windows-icon/preview.png`。

## 产物摘要（SHA-256）

| 产物 | SHA-256 |
| --- | --- |
| `assets/windows/qiku.ico` | `ff2dcd67d9052a94caa6578e6c122d029b7fbd276a651804eaf0bd37e204652f` |
| `cmd/nas-desktop/resource_windows_amd64.syso` | `e35f747c3bf0f9136e2c791a7d5471fafc3b383a1c820eb04fac112047f89d5a` |
| 普通 GUI EXE | `4ad2ba39e3de4e41c8fa64486ecd0494a5cdf2bcae1e5cb39e66f2a61dbeb705` |
| 开发发布包桌面 EXE | `0b7cbae5d71d33609dfe8f71c19086e5de5faeb2baeb723c2832cadebde64f52` |

## 边界

- 本次是 Windows amd64 原生 API 与真实桌面进程验证，未物理点击托盘、未检查 Explorer 截图，也未重启 Explorer 或清理全局图标缓存。
- 发布验证复用了已有 web/dist，开发包不带 FFmpeg，不能称为完整离线发行验收；没有运行 Docker、真实 BT 或 Linux 实机。既有前端与业务逻辑未改动。
- 普通 EXE 与内嵌开发包均已验证；没有在 GitHub 云端执行 Actions，仅使用其原有打包脚本本地验证资源自动进入成品。
- 没有访问旧远程服务器或清理此前被策略拒绝的 `.runtime/windows-exit/verify`，本任务不改变此前清理报告。
