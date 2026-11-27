# 独立质量复核记录

日期：2026-11-18。范围为项目内构建缓存和临时目录配置；未运行 Docker、未清理 C 盘缓存、未修改用户或系统环境。

## Findings (fixed)

- 文件：`scripts/project_env.py`、`scripts/release.py`。
  - 问题：Windows 即使以 `shell=False` 执行 `npm.cmd`，也可能隐式进入 `cmd.exe`，把参数中的 `&` 解释为命令分隔符。实际 `npm --version --probe=x&ver` 复现了额外执行 Windows `ver`。
  - 修复：统一使用 `resolve_command()`；Windows npm/npx 改为 `node.exe` 加对应 CLI JavaScript 的原生参数数组调用，其他 `.cmd/.bat` 明确拒绝。发布构建复用相同解析器。显式相对可执行路径按子进程实际仓库工作目录解析。
- 文件：`scripts/project_env.py`、`scripts/project-env.ps1`。
  - 问题：环境 JSON 同时含大小写不同的 npm 键时，Windows PowerShell 5.1 和 PowerShell 7 的 `ConvertFrom-Json` 均报错；非终止错误可能被后续成功提示掩盖。
  - 修复：Windows 返回一套 npm 小写键，依赖 Windows 环境变量本身的大小写不敏感语义；POSIX 保持双大小写覆盖。JSON 解析增加 `-ErrorAction Stop`，失败不继续导入或输出成功提示。
- 文件：`scripts/release-linux.sh`。
  - 问题：通过固定仓库 CWD 的通用运行器间接执行 release，会改变原有相对 `--tools-manifest`、`--output` 参数的含义。
  - 修复：直接运行已自动初始化环境的 `release.py`，使用 `python3 -B`，保留调用者工作目录及参数边界。
- 文件：`scripts/verify.py`。
  - 问题：只校验 `verify/` 父目录不足以防止默认 `verify/media` 本身为链接；先 `.resolve()` 会丢失该证据。
  - 修复：仅对默认媒体根在规范化前调用已有目录检查器，保留显式 root 参数和其他验证脚本原有样本目录存在性规则。

## Findings (not fixed)

无未修复的本任务范围内问题。未将 Git Bash 检查或 POSIX 分支模拟声称为 Linux 实机运行。

## Verification

- 独立临时行为回归：49 项通过。覆盖无导入环境副作用、Windows 无大小写冲突 JSON、污染变量覆盖、预缓存 tempfile 重置、真实临时文件、原生参数和退出码 37 保真、真实 npm/npx 元字符安全、批处理拒绝、发布解析器复用、两版 PowerShell 严格无错误点源和 CWD 保留、UTF-8 BOM、中文空格复制路径实际启动、17 类无效配置、重复 JSON、不可写失败与环境不部分修改、POSIX 双大小写分支、实际 Windows junction 拒绝和哨兵保留、默认媒体根检查。
- `python -B scripts/verify_release.py --self-test`：修复后重新执行，68 项通过。
- Python 脚本 AST 语法检查：全部通过。
- `git diff --check`：通过；仅有仓库既有自动换行提示，无空白错误。
- Git for Windows `sh -n scripts/release-linux.sh`、实际 `--help`：通过。
- 用复制的 shell 包装和受控 Python 标记入口，从其他目录传递相对 output/manifest：工作目录和带空格参数完整保留。
- 主代理另行报告：Go vet/test/Windows build、前端 typecheck/生产构建通过；Windows amd64、Linux amd64/arm64 三目标真实开发归档生成并逐包通过 `verify_release`。这些为主代理结果，未重复执行全量构建。
- 因本任务未改业务 API/媒体运行逻辑，未运行既有完整服务回归；Linux 环境实际执行和 Docker 部署不在本轮独立验证范围。

## 临时源及样本清理

- `.runtime/review-project-env-20261118.py` 已删除。
- 本轮专用 `.runtime/review-project-env-fixtures-20261118/` 已由测试 finally 清除，包括假项目、junction 和全部哨兵；执行后复查不存在。
- 未删除共享 `.runtime/` 构建产物或其他代理文件，未触碰 C 盘旧缓存。没有保留本轮生成的临时单元测试源。

## 规格同步

已将 npm 批处理安全解析、Windows JSON 大小写差异、PowerShell 终止错误和 Linux 相对路径语义通知主代理；主代理已同步使用文档和 `.trellis/spec/backend/cross-platform-contract.md`。
