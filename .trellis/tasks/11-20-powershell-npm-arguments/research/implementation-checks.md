# npm 入口修复与受控验证

## 实现

- 仅修改 `scripts/dev.ps1`：frontend/build 两个 npm 调用改经 `python -B scripts/project_env.py -- npm ...`，复用已有原生 Node/npm CLI 解析器。
- frontend 去掉重复的 `-- --host 127.0.0.1`，仍由 `web/package.json` 的 `vite --host 127.0.0.1` 指定本机监听。
- 两个关键调用添加中文解释；原有 BOM、公共参数、Go 分支和退出码处理保留。

## 实际执行

- Windows PowerShell 5.1.19041.6456 与 PowerShell 7.6.4 分别通过全部四个 `scripts/*.ps1` 的 `Parser.ParseFile`。
- 双解释器共十四项受控开发入口场景通过：backend 成功/Go17，frontend 成功/npm19，build 成功/npm29/Go23。
- 受控样本用真实 Python 进程记录 argv，确认 `--` 作为字面参数到达 Python，后续为准确的 `npm --prefix web run dev|build`；没有再次使用会掩盖 npm.ps1 差异的 npm.cmd 参数替身。
- 断言工作目录为隔离样本仓库、TMPDIR 仍为本项目 `.runtime/tmp`；backend/build 原样传递 `E:/Media;F:/Books`，frontend 保留继承的媒体根；build 的 npm 失败后不执行 Go。
- `git diff --check` 通过；dev.ps1 的 `EF BB BF` 已独立检查。

## 证据与清理

- 执行入口：`python -B scripts/project_env.py -- python -B .runtime/verify/npm-dispatch-regression.py`。
- 结果：`.runtime/verify/npm-dispatch-result.json`，包含十四个场景的完整参数、cwd、roots、tmp 与退出码。
- 临时 Python/PowerShell 样本和隔离 fixture 目录已全部清除，仅保留 JSON 结果。
- 本受控验证不启动 npm/Vite、Go 应用或任何真实服务；真实双解释器 Vite HTTP 和实际 typecheck/production build 由主代理另行执行，不能以此记录代替。
