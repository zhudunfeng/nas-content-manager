# 资源收集：网页视频与音频获取

## Goal

P3，依赖collection-core；受控yt-dlp、单条/有界批量、格式预览、发布及恢复。2026-11-20 已授权实施。

## 依赖与范围

- 主任务：[资源收集 PRD](../11-19-resource-collection-plan/prd.md)。
- 实施细节：[完整修改计划](../11-19-resource-collection-plan/info.md)，以主任务的首期范围和契约为准。
- 依赖 collection-core 的统一协议、幂等和发布契约；不要求先完成通用插件平台。
- 2026-11-20 用户已指令按计划实现；本子任务已完成本地开发验收，目标部署以发布子任务记录为准。

## Acceptance Criteria（后续实施）

- [x] 真实yt-dlp/FFmpeg隔离视频与MP3完整入库并经鉴权Range精确读取；单链接单条目，有界显式批量不递归播放列表。
- [x] 固定argv无任意shell或工具配置注入，实际连接代理拒绝私网/保留地址；HTTP/native DASH，HLS-only不支持。
- [x] 缺工具、取消、进程中断与响应丢失有真实可恢复状态和产物清单；Windows父子进程文件释放测试通过。

2026-11-20 本地开发验收完成，浏览器验证批次部分失败及请求幂等；真实工具链路见主任务 `research/worker-tool-versions.md`。公网样例受fake-IP保留地址限制，Linux容器版本及实际来源验收保留在collection-release。

## Definition of Done

按主任务质量要求通过真实验证并记录证据；构建、测试和临时工具经 scripts/project_env.py 执行，遵守中文注释/日志和显式类型要求。实施前再配置本子任务上下文并读取相关技能，不以本文件存在认定已完成。

