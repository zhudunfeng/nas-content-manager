# S05 建立作品、季集与文件关联模型

## Goal

将电影/电视剧从文件列表提升为可浏览的作品、季集和多版本关系，同时完整保留已有文件身份、媒体读取与个人状态。状态：planning；相对工作量：大。

## 依赖与入口

- 硬前置：S01 本机基线；推荐 M1 后交付，生产迁移前要有已验证的 S03 恢复点。
- 现有入口：`internal/app/store.go`、`models.go`、`items.go`、`jobs.go`、`server.go`；`web/src/types.ts`、`App.vue`、`components/MediaCard.vue`。
- 拟新增：`internal/app/media_catalog*.go`、`web/src/components/WorkDetailDialog.vue`，最终命名以实现时职责划分为准。
- [总路线](../11-23-nas-evolution-plan/roadmap.md)、[拟议契约](../11-23-nas-evolution-plan/info.md)、[媒体研究](../11-23-nas-evolution-plan/research/media-implementation.md)。

## 范围与拟议契约

- 仅电影/剧集：作品、S00 特别篇、季/集、同作品多文件版本、多集文件、人工归组与拆组；其他类型和未匹配文件保留平铺视图。
- 拟议 `media_works/media_seasons/media_episodes/media_work_items/media_episode_items` 为附加表，保留 `items.id` 及所有旧外键。迁移只建空结构，不在启动时同步全盘识别。
- 拟议分页 `GET /api/works`/详情、人工创建/编辑、归组 preview/apply。写入绑定 revision、计划有效期、原 requestId；歧义不自动接受。
- 继续按 item_id 播放和存个人进度；作品级统计明确区分“任一版本可用”和个人完成状态，不更改已有语义。

## 小 PR 顺序

1. **S05.1 数据骨架和兼容读取**：增量迁移、Go/TS DTO、分页读接口、旧库迁移断言；文件视图保持原行为。
2. **S05.2 显式关系写入**：作品/季集创建、文件版本和多集映射、关联冲突预览/提交、取消关联；短事务与修订并发控制。
3. **S05.3 作品 UI**：视图切换、作品详情、季集导航、选版本后复用 ReaderDialog；支持未归组回退，不强迫全库一次归组。

每片完成后可停止；S05.1不开放未经验证的写入口，S05.2可先用API验证再开放UI。若新增任务种类，必须把现有默认 enrich 分派改为显式分派并同步取消/重启/重试。

## Acceptance Criteria

- [ ] 旧库迁移后逐项核对 item ID、条目数、标签、收藏、进度、下载/收集任务引用均保持；新库和重启同样可用。
- [ ] 覆盖同名不同年份、电影双版本、S00、多集文件、未知集号、无外部ID；模糊项不静默合并。
- [ ] 归组、拆组、重组不改变源文件或清空个人状态；删作品不级联删 item。
- [ ] 旧revision、计划过期、重复提交和同requestId不同参数行为明确；分页稳定，无全库一次加载。
- [ ] 扫描失败/挂载失联不破坏关系，变更不阻塞首次扫描与原文件Range。
- [ ] 375/768/1024/1440宽度、双主题、键盘/关闭焦点与Reader生命周期验证通过并留证。
- [ ] 按变更运行隔离HTTP/迁移及五类媒体回归；记录真实执行，更新已实现API/spec。

## 失败与回退

关闭作品入口即可继续文件视图；逻辑拆组不删除媒体/条目。数据库降级需恢复匹配版本快照，不能手改user_version让旧程序打开新库。修订冲突先刷新预览，不覆盖新编辑。

## Out of Scope

自动重命名/移动原片、全片hash、同名自动合并、音乐专用专辑模型、远程在线刮削、重复项状态合并。简介/年份/海报与字段来源由S06完成。

## Definition of Done

执行父路线统一PR门禁，中文注释/日志与显式类型齐备；复用并扩展集成脚本，新增临时单测执行后删除；生产迁移/真实NAS结果单独记录，不以fixture代替。
