# Windows 桌面默认用户数据目录迁至程序同级

## Goal

让 Windows 原生桌面版默认使用 `栖库.exe` 所在目录下的专用用户数据根，避免首次运行把配置、日志、SQLite 和本机组件状态写入 C 盘用户目录；同时保留显式目录配置能力。

## Requirements

* Windows 原生桌面启动且未指定 `--profile` 时，默认 profile 为程序目录下 `qiku-data/profiles/<mode>`。
* 默认 native 业务数据为默认 profile 下的 `data`；默认 docker profile 同样位于程序目录下但保持模式隔离。
* 程序目录解析必须基于 `os.Executable`，与当前工作目录无关，并拒绝无法定位或无法规范化的程序路径。
* `--profile` 优先于默认 profile；`--data-dir`、`NAS_DATA_DIR` 和已保存 `desktop.json` 的数据目录语义保持不变，已有配置不自动迁移或覆盖。
* 配置、日志、锁、IPC、托管组件状态等 profile 级用户数据默认全部位于程序同级专用根；程序目录本身仍可只读时，显式 `--profile` 继续支持可写用户目录。
* 新增或修改的方法、字段、配置参数保留中文注释；关键路径选择和失败分支输出中文日志或中文错误。
* 更新 Windows 使用说明、帮助文本和相关测试/验证脚本，明确默认目录及自定义方式。

## Acceptance Criteria

* [x] 从任意工作目录启动 Windows 桌面 EXE，默认 profile/data 不包含 `%LOCALAPPDATA%` 或当前工作目录路径。
* [x] 默认 profile 路径为 `<exe目录>/qiku-data/profiles/native`，docker 使用独立 mode 目录。
* [x] `--profile`、`--data-dir`、`NAS_DATA_DIR` 和已存在 `desktop.json` 的优先级与现有行为一致。
* [x] 程序目录不可写时，默认启动失败给出明确中文提示；显式可写 profile 可正常启动。
* [x] Go 格式、vet、编译及现有 Windows 桌面回归通过；临时单元测试执行后删除。

## Out of Scope

* 不自动迁移旧 `%LOCALAPPDATA%/Qiku` 数据。
* 不改变 Linux CLI、Docker 容器内部 `/data` 目录或数据库 schema。
* 不新增安装器、注册表或系统级环境变量。

## Technical Approach

在 `internal/desktop/config.go` 及平台文件增加程序目录解析 helper：显式 profile 直接规范化；Windows 默认 profile 取 `os.Executable` 的规范父目录并拼接 `qiku-data/profiles/<mode>`。默认 `DataDir` 继续由 profile 派生。托管获取状态在程序内仅允许专用 `qiku-data` 子树，保留数据库、媒体和运行时隔离。更新 cross-platform 规格和 Windows 文档，补充路径优先级与只读程序目录边界。

## Validation

全部验收项已验证，详见 [最终验证](./research/final-validation.md) 和 [独立复核](./research/quality-review.md)。任务进入 review，等待用户复核；不自动提交或归档。
