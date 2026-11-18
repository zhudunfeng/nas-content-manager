# 双平台与 Windows 一键启动最终质量审查

- 日期：2026-11-18。
- 范围：P0–P3 共享运行时、Windows 启动器、Desktop 命名卷/停止协议、三目标发布与验证脚本；P4 安装器/签名/自动更新/迁移不在本次范围。
- 审查依据：本任务 `prd.md`、`info.md`、`check.jsonl`、Trellis check/finish-work、backend runtime/download/bootstrap/cross-platform 可执行契约。
- 操作约束：未启动/安装/重新配置 Docker 或 WSL，未访问旧远程 Docker 测试资源，未拉取/加载镜像或创建部署容器/镜像/网络/卷；Docker 部署由用户手动验证。未提交 Git 或归档任务。

## Findings (fixed)

### 1. 工具名称大小写可绕过架构检查

- File：`scripts/release.py`、`scripts/tools-manifest.schema.json`。
- Issue：必需工具去重/存在性采用大小写折叠，但架构检查和 Linux 可执行权限仅匹配精确小写名称；`tools/FFMPEG` 之类别名可能满足清单却跳过真实检查，并产生无法定位的 Linux 包。
- Fix：抽取构建/验包共用 `validate_tools_manifest`，要求 runtime 组中两个主工具使用精确小写规范路径；复制前统一校验。补充持久自测与 schema/发布规范。

### 2. 用户材料可以占用并被覆盖的生成清单路径

- File：`scripts/release.py`。
- Issue：输入许可证/源码材料可使用 `licenses/ffmpeg/tools-manifest.json`，随后被生成的清单覆盖，造成元数据与实际材料不一致。
- Fix：大小写不敏感保留该生成路径，拒绝输入材料占用；自测覆盖拒绝行为。

### 3. 跨平台归档路径缺少 Windows 别名和控制字符约束

- File：`scripts/release.py`。
- Issue：原路径校验未拒绝 Windows 设备名、非法字符、尾点/尾空格、控制字符和 DEL，可能产生不同解包结果或损坏校验和记录。
- Fix：统一 `relative_path` 在三个目标上拒绝上述路径及原有绝对路径/遍历/反斜杠/盘符；保留中文/空格普通路径。补充边界自测。

### 4. 验包仅验证部分元数据，未完整证明材料与二进制结构

- File：`scripts/release.py`、`scripts/verify_release.py`。
- Issue：弱 JSON 解析可能接受重复字段或非对象值；正式包可缺少实际来源/许可组；工具字节或架构在内外哈希重算后可能未被拒绝；镜像清单规则与构建方不一致。
- Fix：共享严格工具/镜像校验器；有界对象 JSON 解析拒绝重复字段、未知字段、不正确类型/版本；检查 runtime/license/source 三组、每条文件哈希、未列材料、版本/来源一致性、镜像不可变身份及配套归档哈希。提取 `validate_binary_stream`，直接检查 ZIP/TAR 成员的 PE/ELF 架构，不解压或执行工具。JSON 实际读取限制为 1 MiB + 1 字节，避免仅检查读取前大小。
- Regression：可复用 `verify_release.py --self-test` 扩展至 **52 项**，包括完整 Windows/Linux 合成结构、重算哈希后的错误架构、丢失许可/源码、清单碰撞、确定性归档和未列成员。合成 PE/ELF/许可文本仅用于验证校验器，不证明真实工具执行或再分发许可合规。

### 5. 已保存桌面配置可能退回默认/CWD 数据库

- File：`internal/desktop/config.go`，由审查提出并由 desktop 实现负责人修复。
- Issue：将已保存 JSON 叠加在默认值上，可能使缺少 `dataDir` 等字段的损坏配置选中新空库；空数据路径也可能被 `filepath.Abs` 解释为当前目录。
- Fix：仅文件不存在时使用默认值；保存的 schema v1 必须含全部九个精确规范键，均非 null，解码到新值；要求非空绝对数据路径、非空绝对媒体根及有效模式配置。逐字段缺失/null、空/相对路径、原损坏字节保持的回归已执行，临时源已清理。

### 6. Docker 工具与镜像平台核验不完整

- File：`internal/desktop/docker.go`，由审查提出并由 desktop 实现负责人修复。
- Issue：仅检查镜像内容 ID 不足以证明其 Linux/amd64 平台；若 Go 相对可执行保护被环境设置改变，`LookPath` 返回相对路径后切换工作目录会改变实际加载位置。
- Fix：显式要求 Docker CLI 绝对路径；检查真实 daemon 与 app/helper 镜像的 Linux amd64 OS/架构/content ID；拒绝同一 app/helper image ID 和隐式跨架构模拟。补充已执行的临时回归。

### 7. Desktop 最终部署模型与停止边界完成严格核验

- File：`internal/desktop/docker_compose.go`、`internal/desktop/docker.go`、`internal/desktop/launcher.go`，由 desktop 实现负责人完成并交接。
- Issue：仅检查模板或局部字段不能保证 Compose 最终插值配置；helper 停止轮询为 90 秒，外层过短会提前误报；运行中启动器的 CLI stop 不应变成打开页面。
- Fix：对真实离线 Compose 规范化 JSON 使用严格模型，限定两服务/七角色卷/一桥接网、固定镜像/环境/权限/标签/日志/挂载及唯一回环端口，最终端口回退后重新验证。33 项变异被拒绝。helper stop 外层 105 秒，成功后才精确停止 app/helper；同 profile 启动器存在时 CLI stop 明确要求托盘完整停止或先退出启动器。

### 8. 规范同步

- File：`docs/release.md`、`scripts/tools-manifest.schema.json`、`.trellis/spec/backend/cross-platform-contract.md`。
- Fix：记录精确小写工具、保留生成路径、通用安全路径、严格材料/架构验包；同步九字段非 null 保存契约、最终 Compose JSON 预检、Linux/amd64 Desktop 基线、105s/90s 停止预算及运行中 CLI stop 行为。保留原 Linux bind/CLI 和原生/Docker 数据分离契约。

## Findings (not fixed)

未发现必须修复而尚未修复的已确认范围内代码问题。下面的环境与人工验收边界单独记录，不将其表述为已通过。

## Verification

### 最终自动质量门禁

| 检查 | 结果与证据范围 |
| --- | --- |
| `.tools/go/bin/gofmt.exe -l internal/app internal/desktop internal/enginehelper cmd/nas-desktop cmd/nas-engine-helper` | 通过，无输出 |
| Windows amd64：`go vet ./...` | 通过 |
| Windows amd64：`go test ./...` | 通过；临时测试已删除，最终输出为 no test files，仅是全部包编译证据 |
| Windows amd64：`go build ./...` 及独立 `cmd/nas` 构建 | 通过；CGO_ENABLED=0 |
| Linux amd64：`go vet ./...`、`go build ./...` | 通过；交叉检查/编译，CGO_ENABLED=0 |
| Linux arm64：`go vet ./...`、`go build ./...` | 通过；交叉检查/编译，CGO_ENABLED=0 |
| `npm --prefix web run typecheck` | 通过，vue-tsc 严格检查 |
| `npm --prefix web run build` | 通过，Vite 148 modules；父任务独占最终发布暂存后未再次改写 dist |
| `python -m py_compile scripts/release.py scripts/verify_release.py scripts/package_desktop_images.py scripts/verify_desktop.py` | 通过 |
| `python scripts/verify_release.py --self-test` | 最终复跑通过，52 项；较早的 15/47/48 项记录不作为最终计数 |
| 三目标既有开发归档通过强化验包器 | 通过；最终重新构建的归档及移动路径执行另见父任务最终交付记录 |
| `git diff --check` | 通过；仓库 CRLF 转换提示不属于空白错误 |
| 新生成 `_test.go` 清理 | 已确认没有保留的 Go 临时测试源；可复用验证脚本保留 |

Go 工具版本为仓库 `.tools/go` 的 go1.27.4 windows/amd64。前端生产构建与发布暂存串行，未并发改写父任务消费的资源。

### 行为验证的独立证据

- 共享运行时：[runtime-implementation.md](runtime-implementation.md)。实际 Windows 数据锁/监听器/实例 token/关闭/资源检查；WSL1 的 `/tmp` flock、符号链接、静态资源与配置检查。WSL1 的 SO_REUSEADDR 行为和 Windows symlink 权限限制均单独记录，未当作标准 Linux 端口冲突或 Windows symlink 证明。
- helper：[helper-implementation.md](helper-implementation.md)。真实 Linux `/tmp`、SQLite/Unix socket 与受控 Docker 接口的 9 项行为检查通过；1 项正向 fsuid 因 WSL1 EACCES 能力限制跳过。未放宽生产权限探针；受控 Docker 不代替真实引擎部署。
- 桌面：[desktop-implementation.md](desktop-implementation.md)。8 个顶层临时行为测试通过，包含九字段配置损坏、真实命名管道/互斥量、共享生命周期、平台检查及离线 Compose 33 项变异；临时源已删除。可复用 `scripts/verify_desktop.py` 在实际 Windows 普通构建执行；最终嵌入包由父任务重新构建后复跑并单独记录。
- 父任务原有业务回归：五类媒体/鉴权与 5 GiB 偏移 Range 52 项、下载 36 项、搜种 43 项、受控初始化集成 35 项通过；最终汇总见父任务 `implementation-result.md`。稀疏文件证明协议边界，不能替代真实超大视频起播/拖动/吞吐/RSS 测试。

### 保留的验收边界

- 干净 Windows 11 普通用户、完整工具材料、文件夹选择器/托盘菜单/Explorer 恢复/休眠唤醒等 GUI 人工验收仍须执行；开发宿主的可执行验证不能替代。
- 无已审计的固定版本 FFmpeg/ffprobe、依赖、许可证和对应源码材料，所以本轮只能标记为 `developmentWithoutTools=true` 的开发包；不能宣称完整免全局工具离线发行。正式包入口及严格门禁已实现。
- 没有真实发布 app/helper 镜像清单或 Docker Desktop 全栈部署证据；拉取/导入、命名卷、qB/Jackett 真实安装认证、来源配置、重启恢复和精确停止由用户手动验收。不得使用旧 Linux 服务器结果替代。
- Linux arm64 为交叉编译证据，没有 ARM64 实机运行；WSL1 不等于 Docker Desktop/WSL2 或常规 Linux 内核验收。
- 本轮未产生 Docker 测试资源，因此没有待清理的本轮测试容器/镜像/卷；未运行全局 prune。
