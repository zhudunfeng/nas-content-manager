# Windows 桌面程序同级 .env 配置

## Goal

让 Windows 用户可以在真实 `栖库.exe` 同级 `.env` 配置视频、漫画及现有原生应用参数，双击启动即可使用，同时保留进程环境变量优先与既有便携数据布局。

## Requirements

- 真实 EXE 目录定位 `.env`，不读取调用者 CWD 或 `--profile` 下的同名文件；文件缺失保持兼容。
- 优先级：明确 CLI > 进程环境变量 > `.env` > 保存配置 > 默认值。进程变量按是否存在判定，显式空值屏蔽 `.env` 并继续既有空值处理规则；Windows 键名不区分大小写。
- 启动时读取不可变快照，托盘内部重启复用同一快照；修改文件需退出程序后重启。不修改全局、用户或系统环境。
- 桌面原生配置、外接获取模式检测、BT 参数与共享 app lookup、托管 worker 的既有可配置项使用同一快照。
- `.env` 仅补充 `NAS_` 配置。托管 worker 仍执行既有允许列表，不透传任意变量；监听、工作目录、token、manifest、工具路径等由启动器持有。Docker 的容器配置、Compose 空 env-file 和隔离不变，桌面自身参数可使用该快照。
- 支持 UTF-8（可带 BOM）、LF/CRLF、空行、整行注释、`KEY=value`、可选 `export `、单/双引号与未加引号的 Windows 路径。引号内内容按原文读取（不做反斜杠转义），未引号值的空白后 `#` 可作行内注释；重复键最后一个生效。无变量展开、命令替换、多行值。
- 文件有界读取（最多 64 KiB），格式/编码/权限错误中文报错且不包含配置值。控制/帮助入口保持可用，`--print-config` 不输出 `.env` 秘密值。
- 提供 Windows 专用 `.env.example`（每项中文说明），发行包携带 `.env.example`；不生成/覆盖用户 `.env`。更新使用、升级备份与配置边界说明。
- 遵守中文方法/字段/配置注释与关键日志规范；日志只记录加载状态/数量，不记录值。

## Acceptance Criteria

- [x] 无 `.env` 原行为不变；从不同 CWD/自定义 profile 启动仍只读取程序同级文件。
- [x] 文件可设置 UAV/JMComic 开关、Python 覆盖、worker 配额和应用参数；实际启动证明主应用与子 worker 一致生效。
- [x] 环境 false/空值/混合大小写覆盖文件，CLI 覆盖环境与文件，保存配置被适用文件值覆盖。
- [x] 解析边界（BOM/CRLF/中文/引号/注释/路径/重复/空值/无展开/错误/超限/不可读）有证据；错误和日志不泄露值。
- [x] 托管 worker 固定项与 Docker 原合同保持；内部重启复用快照。
- [x] Go vet/test/build 与 Windows 真实启动、Linux 编译通过；发行包实际携带示例并通过现有验证。
- [x] 本任务临时单元测试执行后精确删除；复用型集成验证保留。更新 spec、验证证据，任务进入 review。

## Technical Approach

使用 desktop 私有的只读环境快照适配现有 `app.LoadConfigWithLookup`，避免引入 dotenv 依赖或 `os.Setenv`。NAS 键标准化；子 worker 仍通过现有 child-only env 传递明确允许的参数。现有 CLI/worker 不独立自动加载文件，避免容器与 Linux 行为变化。

## Out of Scope

- 自动安装视频/Python/下载器依赖、热加载、用户自定义可执行命令。
- 用户数据迁移、desktop.json schema 修改、任意进程环境透传、容器配置扩展。
- 提交、发布与覆盖实际用户配置。

## Research References

- `research/configuration-design.md` — 现有 lookup、托管边界与发行路径调研。

## User authorization

用户已明确要求实现 exe 同级 `.env` 且环境变量优先；按此范围直接实施，无阻塞决策。
