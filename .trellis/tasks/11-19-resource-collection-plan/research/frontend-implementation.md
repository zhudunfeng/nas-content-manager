# 资源收集前端实施与验证

日期：2026-11-20。范围：`web/src/`，依据本任务 `api-contract.md` 和 frontend application contract 实施。浏览器端所有请求复用同源 `api<T>`，无新增依赖。

## 已实现

- 桌面管理导航和手机“更多”增加资源收集；手机底栏保留五项，原五种媒体分类不变。
- `CollectionCenter.vue` 提供概览、来源、订阅、候选和任务五区；能力请求、各列表及媒体库错误分别显示，查询失败不伪装为空记录。
- 来源支持五种固定类型、启停、编辑版本、独立连接测试、有界 RSS 收取及明确历史回填；RSS 地址只写入，编辑不预填秘密。
- 订阅支持包含/排除关键词、质量、大小、季集表达式、目标、试运行、启停及补查。默认停用、人工确认；自动模式明确要求关键词、季集表达式、唯一文件表达式及不预勾的授权选择。
- 候选人工获取携带稳定 UUID 和候选版本，结果不确定时锁定并重试原始意图；忽略操作带版本。
- 网页每批 1–10 个链接、逐项顺序预览与提交，单项错误保留，已成功条目不会再次提交；任务输入首次提交后绑定原目标与幂等键。
- 本地只读登记根据 `media-*` 授权根识别，归档复制和远程复制选择执行端预配置来源；不存在工具或映射时显示真实诊断并禁用预览。
- 外部应用按当前后端能力使用主服务已挂载最终目录只读登记；用户明确确认成品完成后才预览。公开入口仅允许不含认证、查询、片段的 HTTP(S) URL。
- 任务详情独立显示获取状态和入库状态、最多服务端提供的有界事件、取消/恢复/仅重试入库及真实媒体入口；没有伪造整体百分比。
- 收集任务的 `downloadId` 可定位原 DownloadCenter 并打开原文件选择详情；媒体对象交回原 App 详情/阅读生命周期。
- 轮询完成后才安排下轮，前台单轮查询、隐藏暂停、卸载取消、替换查询取消并拒绝旧响应；详情和列表按 revision 避免倒退。弹窗间先卸载并 `nextTick`，异步操作期间禁止另开弹窗。

## 文件

修改 `App.vue`、`types.ts`、`components/Icon.vue`、`components/DownloadCenter.vue`。

新增 `collection.ts`、`components/collection.css`、`CollectionCenter.vue`、`CollectionSourceDialog.vue`、`CollectionSubscriptionDialog.vue`、`CollectionTargetFields.vue`、`CollectionPreviewDialog.vue`、`CollectionAcquireDialog.vue`、`CollectionTaskDialog.vue`。

所有新增命名方法有中文说明和显式参数/返回类型；DTO 字段和配置/限额有中文解释；关键确认日志为中文且不输出输入 URL、路径载荷或凭据。

## 已执行检查

- `python -B scripts/project_env.py -- npm --prefix web run typecheck`：通过。
- `python -B scripts/project_env.py -- npm --prefix web run build`：最终通过，165 modules transformed；新收集分包约 65.65 kB JS / 8.42 kB CSS（gzip 20.26 / 1.67 kB）。
- `git diff --check -- web/src`：通过；仅仓库已有 LF/CRLF 转换提示。
- 静态检索新增收集代码：未使用 `any`、类型检查抑制、`v-html`、`setInterval` 或绕过同源 API 的直接 fetch。
- 未创建临时单元测试文件，无需清理；浏览器验证由主实施流程使用隔离真实服务执行，其结果以主验证记录为准。

## 后续整体验证记录与边界

- 主流程已完成36项真实浏览器验证，覆盖375/768/1024/1440、深浅主题、键盘焦点、失败/空状态、实际本地入库播放及原请求重试；截图发现弹窗缺内边距后补齐26/18px并重建复验。具体受控错误与网络边界见同目录 `browser-acceptance.md`，worker断线由API集成覆盖。
- 网页实际格式、NAS 权限、远端工具和映射由执行端能力与真实集成样例验证，前端构建成功不代表外部来源可用。
- `outputProfile` 只提交 `video/audio`；本地登记/复制通过选择主服务根或执行端来源区分，不将 `register/copy` 写进该配置字段。
- 外部成品当前仅只读登记主服务已挂载目录，不提供深度 API 管理或外部任务接管。
