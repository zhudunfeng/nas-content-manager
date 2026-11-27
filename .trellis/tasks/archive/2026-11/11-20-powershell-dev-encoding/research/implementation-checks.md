# PowerShell 编码修复与验证

## 修改

- `scripts/dev.ps1`、`scripts/release-windows.ps1`：仅在原始字节前增加 UTF-8 BOM（EF BB BF）；写入时逐字节断言其余内容完全一致。
- `.editorconfig`：仅增加 `[*.ps1]` 的 `charset = utf-8-bom`，附中文说明。
- `project-env.ps1` 与 `clean-session-temp.ps1` 已有 BOM，不作修改。

## 实际检查

- Windows PowerShell 5.1.19041.6456：全部四个 `scripts/*.ps1` 的 ParseFile 均通过。
- PowerShell 7.6.4：同样四个脚本全部通过。
- 两个版本各执行七个受控入口场景，共十四个：backend 成功及 Go 退出 17；frontend 成功及 npm 退出 19；build 成功、Go 退出 23、npm 退出 29 且不调用 Go。
- 每次检查完整调用参数、仓库工作目录、项目内 TMPDIR 和媒体根参数：backend/build 原样保留 `E:/Media;F:/Books`；frontend 保持原有逻辑，不覆盖继承的媒体根。
- npm 使用原生 `.cmd` 记录替身，保留调用中的 `--`；Go 使用路径受控的 PowerShell 记录替身。没有编译应用、启动服务或读取真实媒体。
- 主代理以 Python configparser 实读 `.editorconfig`，解析结果为 `{'*.ps1': {'charset': 'utf-8-bom'}}`。
- `git diff --check` 通过；两个脚本的 diff 均仅首行 BOM（numstat 各 1/1）。

## 证据与清理

- 运行命令：`python -B scripts/project_env.py -- python -B .runtime/verify/powershell-encoding-regression.py`。
- 通过结果：`.runtime/verify/powershell-encoding/result.json`；逐场景参数记录保留在相邻 jsonl 文件中。
- 临时 Python/PowerShell/批处理测试源码均在 finally 中删除；仅保留忽略目录下的结果证据。
- 未运行 Go/前端全量构建：此次没有修改应用代码或命令逻辑。
