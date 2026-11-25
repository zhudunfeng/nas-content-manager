# JMComic 漫画搜索下载插件

## Goal

将用户指定的 https://github.com/hect0x7/JMComic-Crawler-Python 整合为当前 NAS 项目的可选插件，提供漫画搜索、下载及现有漫画库入库入口。

## Requirements

- 优先复用现有资源收集的来源、执行端、持久任务和发布入库流程。
- 插件可选启用，未安装依赖时提供清晰的能力状态与配置指引。
- 支持关键词搜索、结果选择和漫画编号下载；前后端类型保持一致。
- 下载在受控执行端运行，沿用鉴权、路径边界、取消、错误处理和产物校验。
- 新方法、字段、配置有中文注释，关键业务记录中文且脱敏日志。
- 构建及验证通过 scripts/project_env.py 运行，临时文件留在 .runtime 中。

## Assumptions

- 用户已授权直接实施；实现细节采用现有项目约定，合理默认值写入本 PRD。
- 此任务独立于原 nas-backup-recovery，原任务及其交付物保留。

## Technical Approach

- 使用固定的 `jmcomic` 资源来源类型与内置 Python 桥接；Python 和依赖仅存在独立 worker，主服务不执行第三方代码。
- 原来源和任务表无需改动，保持 schema 6；新增可选 JSON 字段必须 omitempty，以保持已有操作意图摘要兼容。
- 搜索 `POST /api/collection/sources/{id}/jmcomic/search` 与 worker `/v1/jmcomic/search`：请求 `{query:string,page:number}`；响应 `{query,page,total,hasNext,items:[{albumId:string,title:string,tags:string[]}]}`。
- 原 `/api/collection/previews` 增加 `albumId:string`（严格正数字）用于漫画预览，返回原 preview 结构，`kind=jmcomic,profile=comic`；原任务 API 强制漫画分类。
- 复用已存在的子进程树取消、受控公网代理、暂存预算与产物发布校验。只接受漫画编号，不接受任意站点地址、命令、YAML 或账号。
- 在资源收集来源和概览提供漫画入口，复用现有单弹窗布局、目标字段和任务详情。

## Decision

- 用户明确要求整合搜索下载，按现有项目架构直接实施；固定适配器优于新建插件市场或独立下载协调器。
- 未来可在固定适配器列表扩展更多漫画来源；当前只交付指定 JMComic。
- 覆盖空结果、网络错误、缺依赖、过期预览、重复提交、取消与重启恢复；实际站点可访问性单独报告。

## Acceptance Criteria

- [x] 可配置和探测 JMComic 插件，禁用或缺失依赖时不影响原功能。
- [x] 可通过应用 UI 搜索并选择漫画发起下载，也支持明确漫画编号。
- [x] 任务状态、失败和取消正确反馈，完成结果可导入漫画库阅读。
- [x] 受控离线 fixture 覆盖搜索、下载、边界与失败，执行端真实安装依赖并验证适配协议。
- [x] Go vet/test/build、Vue typecheck/build、相关回归通过。
- [x] 更新使用文档与 Trellis 规范，清除本任务生成的临时单元测试类/文件。

## Out of Scope

- 泛用第三方代码上传执行、完整插件市场、任意 YAML/Python 插件执行。
- 自动配置用户账号、实际付费或私人内容获取。

## Research References

- research/jmcomic-upstream.md
- research/integration-architecture.md
- research/quality-check.md
- research/browser-validation.md
- research/backend-validation.md

## Verification Result

- 真实主服务及 worker + 协议样例：66 项断言通过，含完整发布入库阅读、失败／取消／重启和旧 local 任务兼容。
- 真实已安装 jmcomic 2.7.12 + 实际桥接的离线验证：11 个检查点通过，保留真实 API／下载器／图片解码，替换外部网络响应；真实 Go worker 的脱敏环境依赖探测通过。
- 真实 curl-cffi 经生产子进程环境和认证公网代理完成独立传输补验：随机端口哨兵拒绝且零连接；标准 80／443 环回地址及 6 项私网拨号断言拒绝。未请求公网，临时测试及自有服务均已清理。
- 浏览器：38 项断言通过，4 屏宽 × 明暗主题；实际 UI 搜索与下载入库阅读；单独标记可控响应专项。
- 最终 Windows 二进制原回归：core 52、collection 27、release 83 项通过；清单 `.runtime/verify/baseline-20261125T011700441856Z-12dba1/run-manifest.json`。
- Linux amd64／arm64 的主服务与 worker 无 CGO 交叉编译通过；CPython 3.11 两平台 wheel 解析／完整哈希下载通过，未宣称 Linux 运行成功。
- 真实站点、Docker 运行和目标 NAS 尚待验收；不将离线样例／交叉编译记为对应验收通过。
- Trellis 独立复核、七节可执行契约及文档已同步。待用户审阅提交，不自动 commit／归档。

## Technical Notes

- Go HTTP + SQLite，Vue 3 + TypeScript，已有独立 collectionworker。
- 工作区初始干净，数据库当前 schema 6。

## Implementation Plan

1. 研究上游与现有收集契约，确定最小可行插件接口。
2. 实现受控插件、后端路由与 UI 衔接并同步文档。
3. 运行 Trellis 检查和集成回归，修复问题并记录验证边界。
