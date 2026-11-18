# Linux/Windows 双平台兼容与 Windows 一键启动

## Goal

在同时兼容 Linux 与 Windows 的前提下降低栖库的部署和日常使用门槛，重点让 Windows 用户能够解压、双击启动，并保持 Linux NAS/无桌面服务器的现有部署与完整业务能力。两端共用视频、图片、漫画、小说、音乐及 BT 搜索/下载业务内核，平台适配不得导致 Linux 回退。

## Baseline Findings (before implementation)

- 用户已于 2026-11-18 要求按计划实现，本任务进入实现与本地验证阶段；本轮不沿用旧任务的远程测试资源。
- 用户补充硬要求：当前 NAS 系统需要同时兼容 Linux 与 Windows；Windows 优化不是改为 Windows 专用系统。
- 现有主程序为 Go + SQLite + Vue，当前源码部署需要前端构建，原生程序还依赖外置 web/dist。
- Docker Compose 当前需要复制和编辑环境文件，再执行源码构建。
- 主程序已有 Windows 磁盘空间和部分路径适配，可复用业务层；当前没有用户级 EXE 启动/托盘和发布打包流程。
- 服务当前在打开数据库、恢复后台任务后才监听端口，需在桌面化时解决重复实例与假就绪。
- BT helper 明确仅支持 Linux 的租约/UID 权限探针，并严格校验宿主 bind；不能原样运行 helper.exe 或直接填写 E:/...。
- 原 BT 初始化任务保留 review 状态，本计划建立独立任务。

## Proposed Baseline

- 首批面向 Windows 11 x64 普通用户，其他版本与 ARM64 分别验证后扩展。
- 同时保留 Linux amd64/arm64 构建与现有原生 CLI/Compose 部署；Linux 无桌面运行不能依赖 Windows 托盘或 GUI。
- 推荐 Go 原生托盘启动器 + 现有浏览器 UI；五类媒体管理不依赖 Docker/Go/Node/全局 FFmpeg。
- 可选 Docker Desktop 全功能配置保留 BT 托管能力；首次安装 Docker/WSL 的管理员步骤与可能重启不称为零确认一键。
- 原生与 Docker 配置有独立实例/数据位置，明确当前模式，禁止静默混用或迁移。

## Decision Status

- 用户已确认“双平台兼容”为硬要求，并于 2026-11-18 明确要求按计划实现。
- 实现范围 P0–P3；P4 的安装器、签名、自动更新与跨平台数据迁移仍为后续增强。

## Requirements

- 采用一个 Go/Vue/SQLite 业务内核与独立平台适配层；不维护 Linux/Windows 两套媒体、下载或搜索业务实现。
- 保留 Linux 现有 CLI、环境变量、路径默认值、UID/GID、bind 部署及 headless 行为；Windows 专用入口/系统调用放置在平台文件并使用 build tags。
- 发布和验证同时覆盖 Linux amd64/arm64 与 Windows amd64；架构交叉编译、实际运行、Docker 全栈验证分别记录，不能相互替代。
- 首次解压双击后授权媒体目录、就绪打开网页；后续复用配置，重复启动只激活已有实例。
- 提供打开页面、目录管理、日志、重启和退出的托盘入口；关闭浏览器继续运行。
- 内嵌前端发布资源，固定携带经过来源/许可核对的 ffmpeg/ffprobe，统一绝对路径定位；运行包不依赖开发工具。
- 使用稳定本地数据根、中文配置说明、原子保存和受控重启；端口实际绑定后保留 listener，数据库启动前获取数据目录锁。
- 原生默认回环监听，局域网访问显式开启；宿主目录授权仅由本机选择器完成，网页保留既有授权根和发布门禁。
- Docker 全功能模式将 app/helper/引擎全部保持在 Linux 容器；新增严格命名卷存储驱动，保留现有 Linux bind 驱动和固定模板身份校验。
- 完整模式检查本机 Desktop context、资源/引擎/来源的真实就绪，准确区分镜像安装与搜索来源配置。
- 默认 BT 数据位于 Docker Linux 数据盘并提示容量/位置，不能假称它是 Windows 普通下载目录。
- 生成可重现版本包/校验和/许可证，提供诊断、升级/备份/恢复步骤；首版不自动替换运行中的 EXE。
- 保留已有 CLI、Compose、五类媒体、超大视频流式 Range、下载发布门禁及数据库内容。

## Planning Acceptance Criteria (historical)

- [x] 方案依据当前仓库能力，不将尚未实现或验证的功能描述为已支持。
- [x] 推荐路线明确，且说明完整 BT 自动安装依赖和一次性人工步骤。
- [x] 实施阶段、文件范围、迁移策略与可复现验收场景完整，见 info.md。
- [x] 规划阶段保持代码和旧任务不变；用户批准后在本任务实现产品代码。
- [x] 将 Linux/Windows 双平台兼容设为设计与验收硬条件，并说明 Linux 无桌面运行和现有部署兼容。

## Implementation Acceptance Criteria

- [x] Linux amd64/arm64与Windows amd64构建通过，平台导入/调用已隔离；运行证据分别记录，ARM64仅交叉编译。
- [ ] Linux 原 CLI 与现有 Compose/bind 部署无需改配置即可继续使用；无桌面环境可启动/停止/备份与恢复。
- [ ] 干净 Windows 11 x64、普通用户，无 Go/Node/Docker/全局 FFmpeg，可以双击原生包并使用五类媒体。
- [ ] 中文/空格/多盘符/任意工作目录、端口冲突、并发双击、扫描中退出、离线重启均有正确行为。
- [x] 数据目录锁在数据库与恢复任务前生效；就绪由本实例令牌与静态资源检查确认，端口实际绑定避让；浏览器Shell交互仍列人工验收。
- [x] 默认回环访问，目录授权和下载staging/未发布选集门禁保持，原始媒体不修改；自动化未代替全部原生目录/托盘交互验收。
- [ ] 新包升级保留用户/库/收藏/进度，备份和失败恢复路径可复现。
- [ ] 满足 Desktop 前置条件时，真实发布镜像在 Windows 上完成 app/helper/两引擎拉取、认证、来源发现、重启恢复与本实例精确停止。
- [ ] Docker 特有的实例/卷/模板/凭据隔离继续成立，不触碰无关资源；测试专属资源清理得到核验。
- [x] 原有五媒体、鉴权、超大偏移Range、下载和搜种回归共166项断言通过；临时单元测试源按要求移除。

## Definition of Done

- 完成 P0–P3 产品实现、可重用发布/验收脚本、文档与规范更新。
- 运行实际可用环境中的质量检查和行为验证，准确记录缺失平台/工具/发布凭据造成的验收边界。
- 新生成临时单元测试验证后删除；不自动提交或归档任务，不改系统 Docker/WSL 设置。

## Implementation Delivery and Acceptance Boundaries

- P0–P3代码、Windows托盘/CLI、共享生命周期、内嵌资源、工具定位、三目标发布脚本、Desktop命名卷和固定停止协议已实现。最终代码与发布验证结果见`research/quality-review.md`及`research/implementation-result.md`。
- 当前环境完成的自动检查与目标设备人工验收分别记录。上述实施验收清单中的干净Windows普通用户、实际Docker Desktop全栈、Linux ARM64实机及目标NAS性能项保持待验，不用交叉编译或受控引擎代替。
- 完整离线发行包需要维护者提供审核后的固定版本ffmpeg/ffprobe、运行依赖、许可证和对应源码材料。本轮构建开发验证包并严格标记缺少工具；正式包生成入口已实现，未把本机移动版本工具重新标注为固定发行材料。
- 本轮没有启动Docker、创建部署资源或访问旧远程daemon；不存在本轮Docker测试容器/镜像/卷清理工作。WSL1只提供部分Linux运行证据，权限/端口兼容限制见验证记录。

## Technical Approach

- P0：双平台服务生命周期/锁适配、前端静态 FS/嵌入、按平台工具定位、桌面配置基础，Linux 原入口兼容。
- P1：Windows EXE/托盘、首次目录授权、重复启动、就绪与恢复体验。
- P2：Windows ZIP、Linux 归档发布及构建矩阵，版本/校验和、诊断与升级文档；P0–P2 形成可独立验收版本。
- P3：Docker Desktop 全功能启动、helper 命名卷驱动与完整引擎生命周期，另行验证。
- P4：安装器/签名、开机启动、确认式更新、跨平台数据迁移，后续增强。

具体影响文件、操作流程、状态隔离和验收矩阵见 [info.md](info.md)。

## Decision (ADR-lite)

- Context：用户要降低 Windows 安装和日常启动门槛，而现有完整 BT 托管具有明确 Linux 依赖。
- Recommendation：双平台共用 Go/Vue 内核，平台入口隔离；Linux 保留原生/容器路线，Windows 增加原生一键媒体入口与可选 Docker Desktop 完整模式；原生与容器数据配置明确隔离。
- Alternatives：只包装 Docker 的首期改动相对集中，但所有媒体用户都承担虚拟化安装门槛；Wails/Tauri/Electron 并不能单独解决 helper 的 Linux 路径/权限限制。
- Consequences：原生一键媒体版先可用；完整 BT 还需专门存储适配，不能以脚本掩盖成本。原生与 Docker 数据迁移必须显式设计。

## Out of Scope (explicit)

- 本轮不重写播放器、阅读器、媒体扫描及下载业务。
- 本轮不安装 Docker/WSL、不修改系统配置；旧远程部署证据不能替代当前 Windows 验收。
- 本期原生模式不自动安装 BT 引擎；原生外接引擎须单独 Windows 行为验收。
- 不将现有 Linux 功能裁剪为 Windows MVP 范围；本期原生不新增自动安装引擎并不删除 Linux 已有的托管部署能力。
- 首个可用包不引入自动更新、Windows Service、全自动 NAS 登录、跨平台库合并或托管下载 NTFS 绑定。

## Research References

- [research/repository-impact.md](research/repository-impact.md)：现有入口、静态资源、工具和 helper 边界。
- [research/windows-deployment-options.md](research/windows-deployment-options.md)：Windows 发布路线与官方依据。

## Technical Notes

- 优先 CodeGraph CLI 导航，近期 helper 文件索引缺口使用 rg 和定点读取。
- 构建、行为测试、发布验证和环境边界分别记录于 research/*-implementation.md 与 docs/verification.md。Docker部署验收由用户手动完成，不沿用旧远程测试结果。
