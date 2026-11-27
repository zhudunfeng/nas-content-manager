# 项目内构建缓存与临时目录隔离

## Goal

让本项目可控制的开发、构建、验证缓存与临时文件默认写在当前仓库中，避免后续继续写入 C 盘共享缓存；Windows 与 Linux 共用同一份可配置规则。

## Requirements

- 「当前文件中」按当前项目目录理解。默认使用仓库 `.runtime/`，配置可调整为仓库内的专用子目录；不能配置到仓库外、仓库根或经过链接的目录。
- 提供统一的 Python 环境初始化器、通用子命令入口和 PowerShell 当前终端初始化脚本；避免持久更改用户/系统环境和 `go env -w`。
- 重定向 TEMP/TMP/TMPDIR、Go 构建/模块/工作空间/工具安装/临时目录、npm 缓存与日志、Node 编译缓存、Python 字节码及 Playwright 浏览器/资料/输出目录。创建和探测失败必须报错，禁止静默回退系统临时目录。
- 现有 dev/release/verify Python 与 PowerShell/Shell 入口自动接入。GitHub Actions 的项目 Go 检查使用统一入口；工具安装 Action 自身不属此控制范围。
- 通用入口保留参数边界、中文/空格路径和子进程退出码；避免 shell 拼接。Python 临时目录的已缓存值也要重置。
- 已经启动的 IDE/MCP 服务不受后来终端环境影响；文档给出需在服务启动前配置的方式，不声称能重定向系统、Docker daemon、既有外部进程、用户显式外部输出路径或本机成品业务数据。
- 不删除或搬迁既有 C 盘缓存，不改变上一任务的默认缓存清理白名单，不运行 Docker。首轮本地缓存可能重新下载依赖。
- 所有新增方法、参数、字段有中文说明和明确类型，关键行为使用中文日志；新增临时测试源验证后清除。

## Acceptance Criteria

- [x] 可编辑的统一配置默认位于当前项目内，外部路径/目录链接被拒绝。
- [x] 默认缓存与临时目录全部解析为仓库内绝对路径，目录可写；外部污染环境不能覆盖隔离规则。
- [x] PowerShell 5.1/7 与 Python 入口可用，现有开发/打包/回归入口接入，Linux 使用兼容入口。
- [x] 子命令参数和退出码保持，Python 默认临时文件、真实 Node/npm/Go 查询均证实使用项目目录。
- [x] 静态检查、前端类型检查/构建及适用的隔离回归通过，文档和 Trellis 规格更新，临时测试源清除。

## Implementation Plan

1. 检查工具链和已有入口，确认可重定向范围并形成共享配置。
2. 实现环境初始化器及自动接入，给开发者保留简洁的命令行入口。
3. 做隔离行为检查及独立复核，记录验证证据、限制和使用命令。

## Technical Notes

- 相关既有代码：`scripts/dev.ps1`、`scripts/release.py`、`scripts/verify*.py`、`scripts/package_desktop_images.py`、`.github/workflows/cross-platform.yml`。
- 原清理任务保持 review，本项作为其子任务推进。
- 代码检索优先使用已有 CodeGraph CLI，纯配置/脚本索引不足时用 `rg` 补充。

## Out of Scope

系统全局环境改写、既有 C 盘清理、Docker 部署验证、发布成品用户数据迁移、自动安装依赖/IDE 插件、保证任意第三方进程绝无系统写入。
