# Vite 代理来源修复独立质量复核

## 结论

未发现需要修复的问题或阻塞。`/api` 与 `/healthz` 都显式设置原目标8090和 `changeOrigin: false`，各配置项有中文解释；其他监听、路径和构建设置保持。前两轮 PowerShell/BOM/npm 变更未撤销，后端安全中间件没有改动。

## 代码与契约复核

- 实读 PRD、check 上下文、Trellis check 技能、前端和后端契约、实际 Vite 配置与 `internal/app/server.go:security`。
- 后端对写请求要求 `Origin.Host == r.Host`，并独立拒绝 `Sec-Fetch-Site=cross-site`。保留浏览器 Host 可消除5173与8090造成的误判；没有改写 Origin、删除安全头、开放 CORS 或引入安全白名单。
- 规范完整记录两处代理、来源边界、正反案例与测试点；`git diff --check` 和 Trellis task context validate 均通过。

## 独立真实安全抽样

通过当前用户已有5173 Vite → 8090 Go 向明确不存在的 `/api/__review_origin_absent_route_20261120__` 发出四个安全样本，不使用 Cookie，不调用业务写入路由：

| 样本 | 结果 |
| --- | --- |
| 同源 PATCH，Origin=5173，Fetch-Site=same-origin | 404，接口不存在 |
| 外部 Origin，但 Fetch-Site 声明 same-origin | 403，不允许跨站修改请求 |
| Origin=null，Fetch-Site=same-origin | 403，不允许跨站修改请求 |
| 无 Origin，但 Fetch-Site=cross-site | 403，不允许跨站修改请求 |

全部通过，机器结果为 `.runtime/verify/vite-proxy-origin/independent-security-sample.json`。未启动、停止或替换用户前后端服务；使用内联 Python 检查且继承项目环境，没有生成临时测试源码。

## 其他验证证据

- 主代理 `.runtime/verify/vite-proxy-origin/real-http-result.json` 记录十二项真实 Vite→Go 验证，包括四种同源写方法、外部/不同端口/Fetch-Site跨站拒绝、读取、healthz、首页与主模块。
- 实现者 `.runtime/verify/vite-proxy-echo-result.json` 记录真实 Vite 独立端口与回显上游的十二项传输验证。复核两个原目标8090、changeOrigin=false及两组GET/POST/PUT/PATCH/DELETE/缺失来源场景，Host/Origin/Fetch-Site/Cookie与二进制body保持；该回显200只证明转发，不代表跨站业务被允许。
- 主代理实际 `python -B scripts/project_env.py -- npm --prefix web run build` 已退出0，包含 vue-tsc 与生产构建；无代码再修改，不重复全量构建。
- 实现者临时 `vite-proxy-echo.mjs` 已清除，仅保留JSON结果。

## Quality Gate

- Findings fixed：无新增发现。
- Findings not fixed / blockers：无。
- Lint / 格式：通过（diff-check）。
- TypeCheck / Build：通过（主代理实际命令与报告证据）。
- Tests：四项独立真实安全抽样通过；十二项真实后端与十二项隔离传输结果复核通过。
