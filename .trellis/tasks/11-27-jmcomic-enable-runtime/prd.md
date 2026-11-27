# JMComic 启用时自动准备私有运行时

## Goal

让 Windows x64 桌面托管 JMComic 与用户指定线程中的 UAV 启用策略保持一致：显式启用后自动准备缺失的私有依赖，验证成功后受控激活，并可离线复用。

## What I already know

- 用户要求与线程 `01a21b3d-49cd-7132-8dc5-4bb3e096698d` 已完成的 UAV 自动初始化策略一致。
- UAV 在 native + managed + 显式启用且无显式自定义 Python 时初始化；false、disabled、external 不自动安装。
- UAV 优先有效随包材料，其次 profile 已验证依赖；固定来源、版本、哈希，最终路径实际探针成功才发布，失败可重试且支持取消退出。
- 当前 JMComic 已有独立私有运行时加载器、构建装配脚本和锁定依赖，但 Windows 文档仍要求预装完整运行时。
- 工作区包含已完成 UAV 和任务归档等大量未提交改动，应在现有实现上增量修改并保留所有无关变更。

## Requirements

- 同步 UAV 的启用门禁、自动准备、可信校验、状态进度、失败重试、取消退出和离线复用策略。
- JMComic 使用独立 Python/依赖目录，保留显式解释器配置优先级及双插件故障隔离。
- 不修改系统 Python、PATH 或用户级环境；所有依赖归应用所有。
- 只有真实 worker JMComic 能力探针通过才报告可用。
- 构建测试和临时命令通过 `python -B scripts/project_env.py -- ...` 执行。

## Acceptance Criteria

- [x] 缺少漫画依赖的 Windows 开发包，显式启用 JMComic 后自动准备并真实就绪。
- [x] false/disabled/external/显式自定义解释器不触发自动安装；有效包和 profile 依赖优先复用。
- [x] 下载失败、损坏、取消和退出不发布半成品，不破坏旧可用版本或其他插件；手动重检允许重试。
- [x] 重启离线复用，双插件独立及共同启用均通过针对性验证。
- [x] 构建、静态检查、回归和文档/spec 更新齐备；移除本轮临时单元测试源码。

## Definition of Done

- 使用 trellis-brainstorm、trellis-before-dev 和项目工作流推进，保存实现与验证证据。
- 方法/字段/配置中文注释、关键业务中文日志和明确类型遵循现有约定。
- 保留有价值的集成验证脚本，记录未完成的目标环境与真实站点验证边界。
- 不自动提交、不归档任务，不改动用户现有安装目录。

## Assumptions

- 本次对齐范围沿用已实现 UAV 的 Windows amd64 桌面托管场景。
- 首次初始化允许访问固定官方依赖来源；不包含站点内容搜索/下载验收。

## Open Questions

- 无需用户补充；从指定线程及仓库提取一致性细节。

## Out of Scope

- Linux/Docker/ARM 自动安装、JMComic 业务 API/页面重设计、系统级依赖安装。

## Technical Notes

- 参考：`.trellis/tasks/11-27-windows-uav-auto-init/`、`internal/uavbootstrap/`、`internal/desktop/uav_initialization.go`。
- 已读取指定线程最终交付与策略说明；先研究复用边界，再确定改动文件。

## Decision (ADR-lite)

- 复用 UAV 已验证的安全安装引擎，抽出内部通用 runtimebootstrap，保留 uavbootstrap 原 API 与资源字节；JMComic 使用独立配方、清单、目录及 jmcomicruntime.Load 严格校验。
- JMComic 只安装自身 Python 与锁定 wheels，不引入视频下载器或 FFmpeg。
- 桌面统一管理插件准备生命周期。同一轮多个插件按固定顺序准备，各自记录结果，一项失败不阻断另一项；等待本轮准备完成后至多受控激活一次。
- 既有启用来源、显式 Python、disabled/external 优先级与 IPC 兼容性保留；新增状态必须按 kind 对应实际能力，禁止把初始化 ready 当 worker available。
- 用户指定对齐既有策略，范围与方案可从仓库及指定线程确定，直接按此范围实现，无需重复确认。

## Delivery and Evidence

- 已实现并完成本机验证，进入 review，未提交或归档。实际交付、长期规则、约束与后续见 `summary.md`。
- 最终程序漫画/双插件自动准备 7 组、UAV 回归 7 组、环境 11 项、双插件真实私有运行时生命周期 25 项、主桌面/托盘、发布边界 131 项与最终 ZIP 24 文件校验通过。
- 全部命令、最终产物摘要、行为测试与目标验收边界见 `research/final-validation.md`；独立审查见 `research/quality-review.md`。
- 干净普通用户 Windows、真实站点、Linux/Docker/ARM 运行时和正式分发人工审查仍需独立验收；编译、协议/缓存测试没有扩大为这些结论。
