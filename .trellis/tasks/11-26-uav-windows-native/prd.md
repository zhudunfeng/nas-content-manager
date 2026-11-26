# UAV／SupJav Windows 原生运行与分发

## Goal

针对现有 UAV 五站插件（Jable、MissAV、Hanime1、黄果、SupJav），实现已评审的 Windows 原生运行时、桌面托管与发布交付方案。用户已于2026-11-26明确要求“按计划进行实现”。

## Requirements

- 面向 Windows 原生桌面双击使用，同时保留原生手工 worker、外接 worker 和 Linux/Docker 路径。
- 先审阅既有 JMComic 私有 Python 运行时、桌面托管 worker 与发布装配实现，优先复用已存在的通用能力。
- 保留固定上游、五站业务、鉴权代理、SupJav 候选线路与 TS 校验、原任务和播放器。
- 运行时不依赖全局 Python、pip、PATH 修改或管理员安装；构建时完成可追溯装配。
- 方案覆盖缺失/损坏依赖、中文和空格路径、只读程序目录、取消退出、升级与回退。

## Assumptions

- 讨论对象为当前对话的 UAV／SupJav 五站插件，不重新设计已完成的 JMComic Windows 功能。
- 首批建议 Windows 11 x64，其他 Windows/ARM64 作为独立验收目标。
- 用户已授权按 info.md 推荐路线实施；允许仓库内隔离依赖装配、构建和验证，不自动发布、提交或安装系统依赖。

## Acceptance Criteria

- [x] 区分当前已有 Windows 代码支持、开发机验证和终端可分发能力。
- [x] 持久化仓库调研与官方 Windows Python 分发依据。
- [x] 提供推荐方案、文件级改动、分阶段实施及可执行验收矩阵。
- [x] 明确版本/依赖隔离、生命周期、部署兼容和未验证边界。
- [x] UAV独立embedded运行时装配、静态加载与真实离线probe，保留源码来源及文件清单校验。
- [x] worker支持清单、显式解释器优先、固定UTF-8、本地--check-uav及独立失败原因。
- [x] 桌面单worker同时支持UAV/JMComic，独立能力、受控环境、原配置和生命周期兼容。
- [x] 固定yt-dlp.exe材料装配、release schema 3和旧schema兼容、归档校验与文档齐备。
- [x] 项目质量门禁、真实依赖离线回归和Windows本机验证通过；干净机、公网站点、Docker/ARM64证据分别记录。

实施验收见 `research/implementation-validation.md`、`research/worker-desktop-validation.md` 与 `research/quality-review.md`。本地实现进入 review；完整下载器对应源码/组件材料和人工分发审查、干净 Windows、真实站点、Linux 实际执行、Docker/ARM64/NAS 均保留待验边界，不计为已交付正式完整发行包。

## Out of Scope

- 自动发布正式发行包、提交代码、修改系统Python或全局环境。
- 真实站点批量下载、账号或 Cookie 输入、新站点接入。
- 自动修改系统环境、接管已有外接 worker、数据库迁移。

## Research References

- `research/windows-runtime-options.md`：官方嵌入式 Python、依赖、下载器和离线装配调查。
- `research/windows-codebase.md`：本仓库可复用能力与 UAV 缺口。
- `info.md`：具体修改方案、配置含义、交付阶段及验收矩阵。

## Recommendation (ADR-lite)

- 背景：已有 Windows 手工部署和 JMComic 私有运行时/托管进程能力，但桌面及发布链路尚未纳入 UAV 和下载器。
- 推荐：复用单个托管 worker，新增 UAV 独立 embedded runtime，固定分发下载器，共用现有 FFmpeg/ffprobe；保留手工 venv 与外接/Docker 路径。
- 代价：包体和构建验证增加，需分别记录原生扩展、源码 provenance、下载器许可材料和干净机验收；不进行大规模运行时框架重构。
- 状态：已按用户确认完成本机实现与验证，进入 review；正式分发材料与目标环境保留待验。原方案文档中的“仅方案”描述保留为前一轮交付边界，以本PRD最新授权与验收记录为准。
