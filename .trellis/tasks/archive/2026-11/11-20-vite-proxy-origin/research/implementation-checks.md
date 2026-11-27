# Vite 同源开发代理实现与隔离回归

## 实现

- 仅修改 `web/vite.config.ts`：`/api`、`/healthz` 两个字符串代理改为显式对象，目标均保留 `http://127.0.0.1:8090`，明确 `changeOrigin: false`。
- 每个 `target` 与 `changeOrigin` 均添加中文说明，明确后端继续按浏览器原始 Host/Origin 检验同源性。
- 没有修改后端安全逻辑、Origin、Cookie、安全头、CORS或其他Vite配置，也未撤销之前的BOM/npm修复。

## 实际隔离验证

- 以 `python -B scripts/project_env.py -- node .runtime/verify/vite-proxy-echo.mjs` 启动真实安装的 Vite、加载项目 `vite.config.ts`。
- 先断言两个生产代理配置均为对象、原目标8090、`changeOrigin=false`；仅在测试进程内将目标替换为自己拥有的HTTP回显服务，并使用独立空闲本机端口启动Vite。
- 每个代理执行六个场景，共十二项通过：GET/POST同源、PUT外部Origin、PATCH匹配Origin但cross-site、DELETE同主机不同端口Origin、POST无Origin/Sec-Fetch-Site。
- 全部断言 Host、Origin（包括缺失）、Sec-Fetch-Site（包括缺失）、Cookie、HTTP方法、完整路径查询及二进制请求体逐字节保持。请求体包含中文和00/FE/FF字节。
- 结果保留 `.runtime/verify/vite-proxy-echo-result.json`，记录所有场景和原始代理配置。
- 样本只验证透明转发；跨站请求的403拒绝由真实Go中间件验证，不能将HTTP回显器的200解释成业务或安全验收结果。

## 清理与边界

- Vite与HTTP回显服务均在finally关闭；生成的 `.mjs` 验证源码执行后自动删除。
- 首次样本尝试 `port:0` 被Vite归为默认5173，strictPort立即拒绝；未结束或替换占用端口的用户服务。随后改用预分配独立端口完成回归。
- 没有访问真实后端、数据库、媒体或用户服务；真实Go来源校验及npm类型检查/生产构建由主代理另行执行。
- `git diff --check` 通过。
