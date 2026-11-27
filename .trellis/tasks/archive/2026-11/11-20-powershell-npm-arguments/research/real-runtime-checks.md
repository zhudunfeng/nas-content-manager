# 真实 npm/Vite 与前端构建验证

## 修复前复现

- 实际运行 `python -B scripts/project_env.py -- powershell -NoProfile -File scripts/dev.ps1 -Mode frontend`。
- 本机 npm 默认解析为 `D:/adunsoftware/nodejs/npm.ps1`。
- npm CLI 进程参数为 `--prefix web run dev --host 127.0.0.1`，原始 `--` 分隔符丢失。
- Vite 日志及实际进程参数为 `vite --host 127.0.0.1 127.0.0.1`；ready 日志显示 5173，但 HTTP GET `/` 返回 404。
- 仅对本次启动的 PTY 会话发送 Ctrl+C；确认端口已释放，再开始修复后检查。

## 修复后真实启动

在 Windows PowerShell 5.1 和 PowerShell 7 中分别真实运行 `scripts/dev.ps1 -Mode frontend`，均使用机器已安装的 Node/npm 与 `web/node_modules` 中 Vite 6.4.6，没有替换 npm/Vite。

两次输出均为 `vite --host 127.0.0.1`，实际 Vite 命令行也只有一次 host 参数，没有地址位置参数。两次逐项检查：

| 请求 | Windows PowerShell 5.1 | PowerShell 7 |
| --- | --- | --- |
| `GET /` | 200，包含 app 挂载点、Vite client 与 `/src/main.ts` | 200，并确认项目标题 |
| `GET /src/main.ts` | 200，包含 createApp | 200，包含 createApp |
| `GET /@vite/client` | 200，包含 createHotContext | 200，包含 createHotContext |

机器可读证据：`.runtime/verify/powershell-npm-arguments/real-http-ps51.json`、`real-http-ps7.json`。

## 实际质量命令

- `python -B scripts/project_env.py -- npm --prefix web run build`：退出 0。
- 其中实际执行 `vue-tsc --noEmit`、`vite build`；类型检查通过、169 个模块构建通过。
- 业务 Go/前端源码未改动，未运行无关的后端全量回归。

## 验证边界与清理

- 前端 GET 检查没有启动后端或操作真实数据，不代表 API 会话或媒体服务已经运行。
- 完整开发联调仍须另一个终端运行 `./scripts/dev.ps1 -Mode backend`；前端 API/healthz 继续代理 `127.0.0.1:8090`。
- 两个修复后 PTY 服务均经本会话 Ctrl+C 结束；未按进程名杀进程，未更改已有 npm、全局环境或执行策略。
- Ctrl+C 时 Python 包装层打印 KeyboardInterrupt，属于本轮主动停止测试服务，不是启动失败。

## 回归教训

上一任务对 `.cmd` 替身的参数断言不能代表本机 `npm.ps1` 行为。此类问题必须检查真实进程参数、HTTP 首页与模块状态，不能仅以 Vite ready 或端口监听为成功标准。
