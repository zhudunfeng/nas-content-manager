# 真实 Vite → Go 请求来源检查

## 修复前

本机既有服务监听前端5173、后端8090。使用明确不存在的 `/api/__dev_origin_check_no_route_20261120__` 路径，避免修改真实业务数据。

- 经5173代理：`Origin=http://127.0.0.1:5173`、`Sec-Fetch-Site=same-origin` 的POST得到403“ 不允许跨站修改请求”。
- 直连8090且Origin匹配8090得到404“接口不存在”。
- 直连8090但明确保留Host和Origin为5173也得到404。
- 检查本机Vite6.4.6实际代码：字符串形式proxy配置被转换成 `{target: opts, changeOrigin: true}`，解释Host/Origin不匹配来源。

## 修复后

仅修改 `web/vite.config.ts` 的两处代理选项；原有Vite监听服务自动重新加载配置，没有结束用户已有前后端进程。

以下全部经过真实5173 Vite和8090 Go，未用HTTP mock替代后端来源校验：

| 场景 | 状态 | 断言 |
| --- | --- | --- |
| 同源POST/PUT/PATCH/DELETE到不存在API路径 | 均404 | 通过来源校验后进入正常路由，未进行业务写入 |
| 外部Origin但Fetch-Site声明same-origin | 403 | 不信任Fetch-Site覆盖实际Origin |
| 同主机5174端口Origin与5173 Host不匹配 | 403 | 不放宽开发端口范围 |
| 匹配5173 Origin但Fetch-Site=cross-site | 403 | 原有跨站元数据拒绝保留 |
| 外部Origin且Fetch-Site=cross-site | 403 | 拒绝真实跨站组合 |
| GET不存在API路径 | 404 | API读取仍正常代理 |
| GET /healthz | 200 | 健康检查代理正常 |
| GET / | 200 | 正确项目标题、主模块路径 |
| GET /src/main.ts | 200 | 主模块正常提供 |

机器可读结果：`.runtime/verify/vite-proxy-origin/real-http-result.json`，共12项。

## 质量命令

2026-11-20 执行 `python -B scripts/project_env.py -- npm --prefix web run build`，实际运行 `vue-tsc --noEmit` 和 Vite production build，退出码0，共构建169个模块。

## 边界

这些验证证明代理来源校验与可达性；没有声称在真实用户数据库完成setup/login/logout或内容修改。后端生产安全中间件未改动。所有内联检查继承项目 `.runtime/` 环境，不生成留存测试源码。
