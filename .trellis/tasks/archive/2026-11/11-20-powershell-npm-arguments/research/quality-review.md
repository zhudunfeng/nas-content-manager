# 独立质量复核

## 结论

未发现需要修复的问题。变更仅涉及 `dev.ps1` 的两处 npm 调用与中文解释；复用现有 `project_env.py` 原生 Node/npm CLI 解析器，移除重复 host 参数，保留公共参数、Go 分支和退出码处理。上一任务的 BOM 与 EditorConfig 保持。

## 独立执行

- 实读当前 PRD、check 上下文、跨平台和前端规范、`web/package.json`、`vite.config.ts`、`web/index.html` 以及 `resolve_command`；确认 package.json 已指定 `vite --host 127.0.0.1`，代理仍为 `127.0.0.1:8090`。
- 确认 `dev.ps1` 仍以 `EF BB BF` 开头；重新用 Windows PowerShell 5.1.19041.6456 / PowerShell 7.6.4 分别解析全部四个 `scripts/*.ps1`，共八项零错误；`git diff --check` 通过。
- 审阅 `.runtime/verify/npm-dispatch-result.json` 的十四项受控场景，独立断言两个解释器的 backend/frontend/build 成功及 Go17/npm19/npm29/Go23 退出码、完整 Python argv、分号媒体根、隔离工作目录和项目 TMPDIR；build 的 npm 失败记录只有 npm 调用。
- 使用项目环境入口真实启动一次 Windows PowerShell 5.1 `dev.ps1 -Mode frontend`，实际监听进程参数为 `vite.js --host 127.0.0.1`，没有位置 root 参数。
- 独立请求 `/`、`/src/main.ts`、`/@vite/client` 均返回 200；首页确认完整 `<title>栖库 · 你的私人内容空间</title>`、app 挂载点、Vite client 和主模块路径，两个 JS 模块分别包含 createApp / createHotContext。
- HTTP 机器证据：`.runtime/verify/powershell-npm-arguments/real-http-independent-ps51.json`。
- 仅向本次创建的 PTY 会话发送 Ctrl+C；会话已结束，已确认 5173 端口不再监听；未按进程名终止进程。受控回归源码及 fixture 目录已清除，本次使用内联检查没有生成测试源码。

## 其他证据与边界

主代理已真实验证 PowerShell 7 首页/模块及 npm typecheck/production build；本次读取对应 research 与机器结果后未重复全量构建。未启动后端、访问真实数据库或媒体，HTTP 检查不代表后端 API 已就绪。Ctrl+C 的 Python KeyboardInterrupt 为主动停止服务的日志。

## Findings

- 已修复项：无新增发现。
- 未修复项：无。
- Lint / 语法：通过（diff-check 与双解释器八项 ParseFile）。
- TypeCheck / Build：主代理实际 vue-tsc / vite build 通过，本次复核证据。
- Tests：独立真实 PS5 首页与模块通过；十四项受控参数/退出码结果复核通过。
