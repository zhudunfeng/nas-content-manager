# BT 种子搜索与下载兼容 Windows 原生运行

## Goal

让 BT 种子搜索与下载在 Windows 原生桌面上可用，持久连接用户已经安装的 qBittorrent 与 Jackett/Prowlarr，覆盖搜索结果转下载、文件选择、下载目录、完成发布、入库与重启恢复，并保留 Linux/NAS 行为。

## Requirements

- 用户要求：BT 种子的搜索与下载需要兼容 Windows。
- 沿用现有 Torznab 搜索、qBittorrent 下载、文件选择和五媒体发布入库结构，不新增独立下载链路。
- Windows 使用原生能力，不以 WSL 或 Docker 作为默认前提；已配置外部引擎的连接方式继续可用。
- 桌面原生 profile 可选 `bt.json` schema 1，搜索与下载各自为 external/disabled；非空环境参数和旧布尔开关保留优先级。私密文件相对路径始终以 profile 为准。
- 兼容 Windows 盘符、反斜杠、大小写、中文空格目录和 PowerShell UTF-8 BOM；POSIX 路径继续区分大小写。
- 仅授权下载根的 complete 子树，不暴露 staging，不接管第三方进程。配置不改变原 desktop.json 或数据库版本。
- 新增或修改方法、字段、配置保留中文注释、明确类型和关键中文日志，禁止输出密钥。
- 本地验证和临时工具统一通过 project_env.py，临时单元测试执行后删除。

## Acceptance Criteria

- [x] 确认 Windows 当前搜索、下载链路的实际缺口并记录。
- [x] Windows 原生搜索及下载所需配置、路径与生命周期可用，失败有明确中文提示。
- [x] 搜索结果继续进入现有受管下载和入库流程。
- [x] 下载 Windows/POSIX、搜索、桌面、core 媒体回归，Go 静态检查、前端构建和跨平台编译通过。
- [x] 新增引擎说明在 375/768/1024/1440、深浅主题下通过真实浏览器布局与键盘检查。
- [x] Windows 开发归档构建并验证 BT 样例与说明实际随包；最终结果与边界归档。

## Technical Approach

1. 引擎路径按自身格式规范化与比较，统一所有权、移动确认和发布检查；本机路径复用既有 downloadPathKey。
2. 桌面原生启动有界严格读取 profile/bt.json，不改全局环境；Docker 分支不读取该文件。追加 complete 到本次有效媒体授权。
3. 密码和来源文件仅接受开头一个 UTF-8 BOM；保持原字节预算、严格来源验证与凭据隔离。
4. 新增可重复 Windows 桌面集成脚本，扩展原下载脚本路径模式及六个非法文件名负例，临时 Go 测试执行后清除。
5. 补使用说明、示例、引擎界面指引、Windows 发行内容及可执行规格，证据记录在 research/ 和 docs/verification.md。

## Out of Scope

- 不重新实现 BT 协议、不更换 Torznab 协议、不重做媒体入库。
- 不接管用户自行运行的外部搜索或下载引擎进程。
- 不自动安装 Windows 版 qBittorrent/Jackett/Prowlarr，不把 external 声称为原生托管。
- 不声称受控协议样本完成真实 Peer、公网站点、干净 Windows、Linux 容器或实体 NAS 验收。

## Progress

- 2026-11-26：已读取 Trellis 流程，当前工作树只有此前 runtime-slim 任务的未跟踪记录；独立创建本任务。
- 2026-11-26：研究、后端与独立质量子任务完成；修复 Windows 引擎路径、BOM、非法文件名与配置 null/UTF-8 边界；原生 BT 闭环和历史读取均通过。
- 2026-11-26：Windows 下载36项、最终POSIX下载42项、搜种43项、桌面BT12项、原桌面17类、发布边界131项通过；core与UI证据已生成，归档检查进行中。
- 2026-11-26：最终 core52项、引擎接入35项、浏览器33项与23文件Windows开发归档通过；包内GUI EXE再跑BT12项通过。规格与结果已同步，任务进入review，未提交/归档。
