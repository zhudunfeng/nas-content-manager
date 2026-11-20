# 修复 Windows 前端开发服务启动后首页 404

## Goal

使 `./scripts/dev.ps1 -Mode frontend` 在 Windows PowerShell 5.1 与 PowerShell 7 使用本机真实 npm/Vite 时正确提供 `http://127.0.0.1:5173/`。

## Evidence

- 已真实启动旧命令并收到 Vite ready，但 GET `/` 返回 HTTP 404。
- 本机 `npm` 解析为 `D:/adunsoftware/nodejs/npm.ps1`；原生 npm CLI 实际收到 `--prefix web run dev --host 127.0.0.1`，`--` 分隔符丢失。
- Vite 进程实际参数为 `--host 127.0.0.1 127.0.0.1`，最后一个地址被解析为错误项目根目录。
- `web/package.json` 的 dev 命令已有 `vite --host 127.0.0.1`，无需额外传递 host。
- 项目已有 `scripts/project_env.py:resolve_command` 原生 Node/npm CLI 解析器，可以复用而不修改用户 npm 安装、执行策略或全局配置。

## Requirements

- 保持 localhost 监听、5173 默认端口、API/healthz 向 8090 的代理及 backend/frontend/build 公共参数。
- dev.ps1 中 npm 调用复用项目环境 Python 入口，避免 npm.ps1/批处理包装层改写参数；移除重复 host 参数。
- 保留已有 UTF-8 BOM、中文注释、项目缓存边界和退出码传递。
- 仅结束本次验证启动的进程，不影响用户已有服务，不访问真实媒体或数据库。
- 按真实 npm.ps1 环境补充回归证据，不能仅依赖 .cmd 替身。

## Acceptance Criteria

- [x] 两个 PowerShell 版本真实运行 `dev.ps1 -Mode frontend` 均输出正确 Vite 命令，无多余地址位置参数。
- [x] 两个版本的真实 GET `/` 均为 200，包含项目 index.html、Vite client 和 `/src/main.ts`；主模块也返回 200。
- [x] `backend/frontend/build` 分支参数及失败退出码保持，构建入口至少通过实际 npm typecheck/production build。
- [x] 所有 PowerShell 脚本在两个版本均可解析，dev.ps1 保留 BOM。
- [x] 临时测试源码与本轮启动的服务清理，结果记录到 research/，规范同步并独立检查通过。

## Out of Scope

应用界面与业务、真实后端数据、外部 API、监听到公网、全局 Node/npm 升级均不属于本次修复。

## Applicable Specs

- `.trellis/spec/backend/cross-platform-contract.md`
- `.trellis/spec/frontend/app-contract.md`

## Validation Notes

前端首页和模块加载验证不代表 Go API 已启动。完整联调需要在另一个终端运行 backend 模式。
