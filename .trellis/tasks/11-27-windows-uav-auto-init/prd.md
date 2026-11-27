# Windows 启用 UAV 自动初始化依赖

## Goal

Windows 桌面托管模式启用 UAV 时，自动检查和初始化匹配的私有 Python/UAV 运行时、yt-dlp 和 FFmpeg，避免缺少完整包文件时只显示通用校验失败并要求手动安装。

## What I already know

- 用户已要求直接优化为“当开启 UAV 时，自动初始化所需文件”。
- 现有开发包可启用 UAV，但未携带 uav-runtime.json、runtimes/uav、licenses/uav 和固定工具文件。
- 现有严格加载器会验证版本、架构、来源、哈希和隔离边界，校验不可降低。
- 之前的本机补包只修复一个开发目录，不可作为生产自动初始化方案。
- 仓库有组件初始化、桌面托管 worker 和构建侧私有运行时装配代码，需要复用边界后确定设计。

## Requirements

- 用户开启 UAV 后自动准备所需依赖，准备成功才报告可用。
- 优先复用已经有效的依赖；重启和重复开启应幂等。
- 依赖隔离在应用拥有的目录，禁止修改系统 PATH 或要求用户先装 Python/pip。
- 保留显式 Python 配置优先级、完整性校验和插件故障隔离。
- 初始化需有中文状态和失败原因，支持失败后再次尝试和退出取消。
- 使用固定可信来源、锁定版本和哈希，临时下载不得污染可用版本。

## Acceptance Criteria

- [x] Windows 开发包缺少全部 UAV 依赖，启用后可自动初始化并通过真实离线能力探针。
- [x] 已有有效运行时不重复下载；关闭插件或外部模式不触发初始化。
- [x] 下载失败、损坏、取消、重复开启均不会发布半成品或影响主服务。
- [x] 自定义解释器优先级、托管子进程隔离和既有完整包行为保持兼容。
- [x] 相关构建、针对性集成验证和独立代码审查通过；文档/spec 同步。

## Definition of Done

- Trellis 研究、方案、实现和验证证据保存到本任务。
- 中文方法/字段/配置注释、关键日志、明确类型符合项目约定。
- 构建测试通过 scripts/project_env.py 执行，不修改全局环境。
- 验证后移除本次生成的临时单元测试文件，保留有价值的集成验证脚本。

## Assumptions

- 本次范围为 Windows amd64 桌面托管 UAV；基础媒体功能继续可用。
- 首次补齐允许联网下载固定依赖，初始化失败时明确提示并允许重试。

## Decision (ADR-lite)

- 使用 Windows 桌面 owned 后台初始化，完成后由 controller 串行停止并重新接入 worker/主服务；首次激活会短暂受控重启服务。下载期间基础媒体仍可用。
- 自动初始化触发条件是 native + managed + UAV 启用且未配置显式自定义 Python。保留 false/disabled/external 优先级；普通“启用获取”不擅自把未配置 UAV 变成已启用。
- 优先有效随包文件，其次 profile 的已验证依赖；都不可用才下载。固定 Python/官方 wheels、yt-dlp、固定 FFmpeg ZIP 均锁定 HTTPS 来源和 SHA256。
- 小型 UAV 代码和生成的隔离/来源元数据作为已装配 payload 随程序嵌入；构建侧从严格验证的运行时生成 recipe/runtime inventory，不把 payload 冒充原始构建 wheel。
- 终端没有 pip/编译步骤，不依赖开发机目录；Go 在 owned profile 隔离暂存目录还原固定文件，严格静态校验和真实离线 worker probe 后才发布不可变 generation。
- 不可变目录使用短前缀 gen-<32hex>，最终原生文件最多248个UTF-16单元；先改名并复验，再在最终位置执行真实probe，成功才发布current，禁止用较短stage的探测结果代替。
- 下载失败仅禁用 UAV，中文状态提供重新检测/下次启动重试；不无限自动重试。退出/停用等待取消和清理，禁止旧初始化完成事件重新启用。
- 显式自定义 Python 继续由管理员管理依赖；无效配置不得自动回退。

## Research References

- [desktop-integration.md](research/desktop-integration.md) — 异步初始化与串行激活、配置和生命周期边界。
- [runtime-acquisition.md](research/runtime-acquisition.md) — 固定官方物料与无 pip 还原方案。
- [native-path-analysis.md](research/native-path-analysis.md) — 最终部署位置的Windows原生DLL加载根因与防回归。
- [final-validation.md](research/final-validation.md) — 最终包主线命令、实测结果、摘要和边界。
- [quality-review.md](research/quality-review.md) — 独立审查与故障注入证据。

## Out of Scope

- JMComic 自动安装、Linux/Docker/external 模式安装、真实视频站点下载验收。
- 自动升级 UAV、修改系统 Python/全局工具、绕过文件完整性验证。

## Technical Notes

- internal/collectionworker/uav.go
- internal/desktop/collection_managed.go
- internal/uavruntime、internal/privateruntime
- scripts/uav_runtime.py、scripts/ytdlp_runtime.py
- .trellis/spec/backend/uav-windows-contract.md

## Open Questions

- 无需用户补充的信息；实现按上述专业默认值推进。
