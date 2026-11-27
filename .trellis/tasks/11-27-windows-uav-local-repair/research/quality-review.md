# 本机 UAV 修复独立复核

日期：2026-11-27（Asia/Shanghai）。复核范围为本机测试程序目录材料补齐，不是生产源码变更或正式发行包审查。

## Findings (fixed)

暂存、安装与真实桌面结果复核后，未发现需要修复的问题。

## Findings (not fixed)

无阻塞问题。现有 `.env` 同时启用的 JMComic 仍缺少自身私有材料；这是目标开发包中独立预存问题，不影响本任务 UAV 已就绪的结论，也没有扩展安装或修改漫画配置。

## 已核对行为

- `install_local_runtime.py` 目标固定为用户授权的 `E:\test\qiku-0.1.0-windows-amd64-development`；材料来源固定为已有 UAV 私有运行时、固定下载器和本机媒体工具。
- 复用 `uav_runtime.verify(..., True)`，不修改私有运行时契约、依赖锁、来源身份或完整性验证器。
- 仅补入 `uav-runtime.json`、`runtimes/uav/`、`licenses/uav/` 和 `tools/`；原 EXE、`.env`、profile、数据库、release.json、checksums 不在写入集。
- 全量预检、逐文件摘要、拒绝未知/不同既有文件和排他新建防止默默覆盖；目录祖先与重解析点检查保持。
- 官方下载器既有 SHA-256 与精确 `2026.10.26` 绑定；FFmpeg shared 工具复制同目录 DLL，并在空 PATH 中运行真实编码、时长探测及完整解码。
- `validate_desktop.py` 通过真实程序原生 IPC 检测 `state=ready`、`collectionState=ready` 和 `uav.available=true`；原程序/配置比对和关键表只读摘要检查失败会明确报错，不伪报成功。
- 本机补齐与正式再分发材料审查明确区分，不改原开发包身份、不生成占位审核声明。

## Verification

- 生产源码 diff：空；新增文件仅 Trellis 任务记录及忽略目录内一次性辅助材料。
- 语法/注释/注解检查：通过。两份 helper 共 14 个函数逐项 AST 检查，均有中文 docstring、入参和返回类型注解。
- Lint：没有发现此类一次性 Python helper 的独立 lint 配置；执行上述 AST 和人工代码检查，未将其冒充独立 linter 结果。
- TypeCheck：入参/返回注解检查通过；未执行或宣称独立 Python 静态类型检查器。
- 暂存证据：`stage-receipt.json` 显示 UAV 3.8.4、CPython 3.14.4、yt-dlp 2026.10.26，FFmpeg/ffprobe 版本相同、`emptyPath=true`、本地视频时长 1.0 秒。
- 最终目标验证：通过。`target-verification.json` 记录 795 份新增目标材料摘要、UAV 3.8.4/CPython 3.14.4 真实离线 probe、固定下载器和空 PATH 本地视频编码/探测/解码通过。
- 原目标 worker：`diagnostic-after.json` 真实退出码 0，输出“视频插件本地依赖和下载工具可用，UAV 3.8.4”；修复前相同入口退出 2。
- 实际桌面：`desktop-after.json` 为 `result=passed`，原程序正常启动后原生 IPC 为 `state=ready`、`collectionState=ready`、`uav.available=true`、`version=3.8.4`。未执行真实站点连通性验收。
- 数据保全独立复核：直接重新读取并计算 7 份原 EXE/配置/发行身份文件的摘要和大小，全部匹配 `protected-files-before.json`；修复前后 6 张关键表行数和内容摘要全部一致。未读取或暴露账户、会话、来源的原值。
- 未创建临时单元测试类，无需删除测试源码；不因无生产源码改动而重复全仓编译或无关回归。

## Spec 判断

现有 UAV Windows 契约已覆盖固定材料、隔离、探测、开发包与完整分发身份；本次不引入接口或行为契约变更，无需扩充规范。
