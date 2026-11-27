# Windows 原生 JMComic 运行与分发兼容方案

## Goal

实现已确认的 Windows 原生兼容方案，使桌面用户可完成搜索、选章、批量下载和漫画阅读，并保留 Linux／Docker、外接 worker 与原有媒体能力。2026-11-26 用户已明确授权“按计划进行实现”。

## What I already know

- 用户要求“兼容在 windows 下使用的情况，请给出修改方案”。
- 当前主应用、worker 已有 Windows 构建及隔离回归；固定真实 JMComic 依赖已有 Windows 离线验证。
- 当前桌面原生文档要求独立准备 Python／JMComic 和 worker 配置，没有承诺开箱即用的原生漫画下载运行包。
- 上轮 jmcomic-advanced 已进入 review，本轮规划不得改变其状态或重复实施。

## Assumptions

- 推荐首先支持 Windows 11 x64 普通用户、无需 Docker／全局 Python 的桌面原生运行。
- 原生控制台和管理员自配外接 worker 模式继续兼容；Windows ARM64 与其他 Windows 版本单独验证。
- 需要比较手工依赖脚本、随包私有 Python 运行时和冻结桥接 EXE 三种方式，然后给出有依据的推荐。

## Requirements

- 检查启动器、获取执行端、Python 进程、权限、路径、打包与更新的当前实际边界。
- 给出具体改动模块、分阶段实施顺序及可验证验收项。
- 缺少漫画运行时只影响可选能力，不阻止媒体管理和已有漫画阅读。
- 保留受控代理、不可变预览、发布、重试与既有安全边界。
- 不将离线协议与本机构建扩大为真实站点或干净 Windows 安装验收。

## Acceptance Criteria for this planning turn

- [x] 当前兼容能力与实际缺口有代码／文档依据。
- [x] 比较可行的分发方式并给出推荐及取舍。
- [x] 方案包含进程生命周期、路径／权限、运行时配置、升级和 UI 诊断。
- [x] 分阶段改动与干净 Windows 验收矩阵明确。
- [x] 研究与方案保存在任务目录，未修改业务代码。

## Research References

- [当前项目 Windows 原生生命周期与插件接线检查](research/codebase-windows.md)。
- [官方 Python Windows 私有运行时与冻结打包依据](research/windows-python-runtime.md)。

## Planning Result

- 推荐“桌面启动器 → 独立 worker → 随包私有 CPython＋锁定依赖”，保留手工解释器与外接 worker 高级模式。
- [完整修改方案](plan.md)包含目标范围、方案比较、具体模块、分阶段实施顺序与目标系统验收矩阵。
- 方案阶段已完成；用户随后于 2026-11-26 授权实施。下列实现验收与最终报告单独记录，不将方案阶段的只读结论当作实施结果。

## Implementation Acceptance Criteria

- [x] 可在构建端装配可验证的 Windows x64 私有 CPython 和锁定依赖，完整保存本机扩展、证书、包元数据与许可；成品不依赖全局 Python。
- [x] worker 支持受信运行时清单、显式解释器覆盖和中文诊断，固定桥接保持隔离 JSON 协议与原业务语义。
- [x] 桌面支持 disabled／managed／external 三种模式及旧配置迁移；托管 worker 只绑定回环实际端口并经私有控制通道就绪，整组注入主应用连接。
- [x] 正常停止、启动取消、父进程终止能回收自有 worker／Python；进程失败只降级可选获取能力，原媒体与已发布漫画可读。
- [x] 状态／秘密／日志与程序目录分离，新建秘密目录有实际 Windows ACL；不修改未知用户目录权限。
- [x] 发布构建纳入 worker，可选完整运行时与清单，基础包和完整包准确区分，严格验证版本／架构／摘要／路径／许可。
- [x] 有可执行的路径、环境污染、配置迁移、损坏运行时、生命周期和既有行为回归；执行后删除本轮临时单元测试源码。
- [x] 更新使用文档、规范和实际验证报告，明确干净 Windows／真实站点／其他架构尚未取得的证据，不以本机离线通过替代。

## Implementation Result

- 已完成八项实现验收，最终同批19条命令与回读审计通过；详见 [主线验收报告](research/final-validation.md)。
- 成品为含私有漫画运行时的开发包，未带齐媒体工具；干净Windows普通用户、真实站点、其他架构运行及Docker/NAS仍未验收。
- 任务进入review，未提交、部署或归档；旧高级漫画任务状态保持review。

## Out of Scope

- 安装全局工具、修改系统 PATH、未经任务需要启动 Docker 或访问真实漫画站点；干净虚拟机无法取得时诚实记录未验收。
- 同时重构所有外部工具、开放账号／Cookie／任意 Python 插件。
- 将“Windows 兼容方案”解释为已要求安装系统服务、开机自启或修改系统 PATH。

## Technical Notes

- 项目路径：`E:/vscodeProjects/nas-content-manager`。
- 通过 CodeGraph 定位符号后读取实际源码校正索引时差。
- 所有本地工具经 `python -B scripts/project_env.py -- ...`，新增材料仅位于本任务。
- 原任务 `.trellis/tasks/11-25-jmcomic-advanced/` 保持 review。
