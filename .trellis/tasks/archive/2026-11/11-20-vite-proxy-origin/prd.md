# 修复 Vite 开发代理导致同源修改请求被拒绝

## Goal

本地通过 `http://127.0.0.1:5173/` 测试时，允许浏览器同源请求经过开发代理到达 Go 后端，解决“不允许跨站修改请求”，同时保留真实跨站请求的 403 拒绝。

## Evidence

- 通过当前 Vite 服务向不存在的 `/api/__dev_origin_check_no_route_20261120__` 发送 POST：`Origin=http://127.0.0.1:5173`、`Sec-Fetch-Site=same-origin`，后端返回 403 及用户报告的错误。
- 直连当前后端8090并使用匹配的 Host/Origin，或明确保留 `Host=127.0.0.1:5173`，相同不存在路径返回404“接口不存在”，说明请求能通过安全中间件。
- `web/vite.config.ts` 使用字符串代理；本项目安装的 Vite 6.4.6 在字符串代理转换时设 `changeOrigin: true`，将 Host 改为目标8090，Origin仍保留前端5173。
- `internal/app/server.go:security` 比较 Origin.Host 与 r.Host，并拒绝 Sec-Fetch-Site=cross-site，生产规则本身无需放宽。

## Requirements

- `/api` 和 `/healthz` 代理改为显式对象，target 仍为 `http://127.0.0.1:8090`，设置 `changeOrigin: false`，每个配置项附中文说明。
- 保留浏览器原始 Host/Origin/Sec-Fetch-Site；不得重写 Origin、移除安全头、关闭CSRF规则、增加开发端口白名单或引入全局CORS放行。
- 保持本机监听、现有端口、路径、Cookie与请求体转发行为。
- 不改后端业务或真实用户数据，仅通过不存在路由与隔离样本验证；不结束用户已有前后端进程。
- 保留上一轮 PowerShell 编码/npm修复，临时测试源码完成后删除。

## Acceptance Criteria

- [x] 真实 Vite→当前 Go 后端同源 POST/PUT/PATCH/DELETE 不存在路由返回404而不是403。
- [x] 外部Origin、同主机不同端口Origin，以及匹配Origin但Sec-Fetch-Site=cross-site，均继续返回403。
- [x] API读取与 `/healthz` 仍可代理，首页/主模块可访问。
- [x] 隔离代理记录验证 Host、Origin、Sec-Fetch-Site、Cookie及请求体保持，未静默修改安全头。
- [x] 前端类型检查/生产构建及独立质量检查通过，规范记录开发代理契约。

## Out of Scope

不改生产CSRF/认证、部署反向代理、CORS规则、Go业务逻辑或用户数据库。

## Applicable Specs

- `.trellis/spec/frontend/app-contract.md`
- `.trellis/spec/backend/runtime-contract.md`
- `.trellis/spec/backend/cross-platform-contract.md`

## Validation Notes

不存在路由的404是“已通过来源校验”的断言，不是业务修改成功。验证不得向真实setup/login/logout或媒体写接口发送变更请求。

2026-11-20 验收完成：十二项真实 Vite→Go 检查、十二项隔离代理传输检查与四项独立安全抽样全部通过；前端类型检查/生产构建退出0，diff-check及任务上下文校验通过。详情见 `research/real-runtime-checks.md`、`research/implementation-checks.md` 和 `research/quality-review.md`。临时测试源码及自有验证服务已清理，用户已有服务未被结束。
