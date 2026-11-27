# Research: NAS 路线、真实验收边界与归档前状态

- Query: 归档后哪些项目能力、实施顺序和未完成事项值得继续使用，如何避免把归档状态误认为功能交付？
- Scope: internal；只读既有 PRD、研究、规范及少量当前源码，不联网、不运行产品、不执行 Git。
- Date: 2026-11-27（Asia/Shanghai）。

## Files Found

- `.trellis/tasks/archive/2026-11/11-23-nas-evolution-plan/roadmap.md`：十二步路线、切片依赖和 M1/M2/M3 出口。
- `.trellis/tasks/archive/2026-11/11-23-nas-evolution-plan/info.md`：尚待逐片实施的跨层设计；不是当前 API 清单。
- `.trellis/tasks/archive/2026-11/11-23-nas-feature-gap-assessment/assessment.md`：2026-11-23 的历史能力评估，后续 S02/S03 已更新部分结论。
- `.trellis/tasks/archive/2026-11/11-23-nas-validation-baseline/implementation.md`：本机基线结果和真实设备待验表。
- `.trellis/tasks/archive/2026-11/11-23-nas-account-lifecycle/prd.md`：S02.1/2/3 本机交付与目标 Linux/NAS 待验。
- `.trellis/tasks/archive/2026-11/11-23-nas-backup-recovery/prd.md`：S03 已交付切片与后续缺口。
- `.trellis/tasks/archive/2026-11/11-23-nas-backup-recovery/research/s03-2-3-draft-checkpoint.md`：三份未接线执行器草稿已隔离，不参与当前产品。
- `.trellis/tasks/archive/2026-11/11-23-nas-backup-recovery/research/quality-review-s03-2-2.md`：受控根/发布器本机独立复核及明确的未验边界。
- `.trellis/tasks/archive/2026-11/11-19-plugin-market-fnos-plan/prd.md`：通用插件/飞牛的规划边界与待提供输入。
- `research/pre-archive-task-status.json`（与本文同目录）：主会话从提交 `68bbd0e3eced0e5e062ad7a45941d99dd39ae1dc` 提取的 50 个任务原状态及父子关系。

## Findings

### 1. 归档是生命周期操作，验收必须读实际证据

- 原归档批次为 50 个任务：`review=23`、`completed=11`、`planning=10`、`in_progress=6`；旧的 `11-20-password-visibility` 不在这 50 个内。
- 原生归档已把这批 task.json 统一改成 `completed`，因此现在的状态不能证明实现、验收或发布。恢复任务背景应使用原状态快照、PRD、独立复核和当前规范。
- 原状态也只是线索：例如 `collection-file-import`、`collection-release` 的历史 `in_progress` 含目标环境验收，不代表已有核心功能未实现。
- 证据等级：**历史元数据 + 历史评估**；来源为同目录 `pre-archive-task-status.json` 及功能评估 `assessment.md` 第 4 节。

### 2. 既有基础应复用，缺口评估必须随实现更新

- 五类媒体、原文件鉴权流、进度、BT/搜种、RSS/自动规则、网页/文件导入、四类外部组件 API 与部署均已有基础。下一阶段优先内容管理、恢复和长期运维，避免重复建设获取链路。
- 2026-11-23 评估当时将账号生命周期列为缺口；当前 S02 已本机实现。因此历史评估只能用作“为何排此顺序”的依据，不能整份当作 2026-11-27 的现状。
- 证据等级：**历史评估 + 当前规范交叉核对**；来源为 `11-23-nas-feature-gap-assessment/assessment.md` 第 2–4 节和 `.trellis/spec/backend/account-contract.md`。

### 3. 路线按恢复能力→内容整理→自动化推进，但不要制造串行互等

- 默认顺序：S01 基线 → M1（S02 账号 / S03 备份 / S04 事件）→ M2（S05 作品 / S06 元数据 / S07 字幕 / S08 管理）→ M3（S09 自动导入 / S10 容量 / S11 规则 / S12 组件维护）。
- 切片依赖比任务编号更准确：字幕、已有字段筛选、只读容量和规则模板可依赖 S01 提前推进；不能因完整 S05/S06 未完成就冻结独立能力。
- S04 基础事件不反向依赖 S02/S03；S04.4 负责后接备份失败生产者。S03 提供业务结果，不为了等待通知而永久保持未完成。
- 证据等级：**规划决策**；来源为 `11-23-nas-evolution-plan/roadmap.md` 第 1–3 节及各子 PRD。

### 4. S01 已交付的是隔离基线，不是目标 NAS 已验收

- 已有 `verify_baseline.py` 可组合真实隔离应用和领域脚本，使用 `passed/failed/blocked/not_run` 四态；缺产物、清理失败、缺断言不能伪装通过。
- 当前源码 `scripts/verify_baseline.py:374` 的 `run_command` 保留执行/清理证据，`:421` 初始化 `not_run`，`:566` 的 `require_command` 优先传播清理失败，`:662` 的目标档案验证不将设备门禁导入为已通过。
- 历史本机两次 core 各 52 项、下载 36 / external 83 / release self-test 83；这些是对应运行的独立证据，不能反复相加或作为本次重新运行成绩。
- 真实 Linux/NAS UID/GID/ACL、挂载、Docker/组件、大库资源峰值及真实大媒体解码仍待验。5GiB 稀疏 Range 只证明协议偏移；交叉编译只证明构建。
- 证据等级：**当前源码/规范 + 历史本机验证 + 目标待验**；来源为 `.trellis/spec/backend/baseline-contract.md` 与 S01 `implementation.md`、`research/quality-review.md`。

### 5. S02 本机完成，复用凭据修订和短事务防线

- 改密、会话查询/撤销、再认证限流、停机独占离线重置已本机交付；仍保持单管理员，不等同于家庭多用户/SSO。
- `internal/app/account_store.go:94` 按旧 hash + auth_revision 条件更新；`:317` 的登录会话插入也匹配快照，防止改密后旧验证线程补建有效 Cookie。
- `internal/app/account_maintenance.go:62` 的离线入口将“已提交但清理失败”“事务结果未确认”和“未修改”分别报告，不能遇到错误就自动重试或说零修改。
- 当前 schema 6 后，账号兼容门禁历史最终为 account162（83 在线 + 79 离线），S02 原独立最终 account157 是更早阶段，不能混为一个新总数。
- 真实 Linux/NAS 锁、终端和权限仍待验；Windows Console/ACL 的历史通过不能外推到 Linux。
- 证据等级：**当前源码/规范 + 历史本机验证 + 目标待验**；来源为 `.trellis/spec/backend/account-contract.md`、S02 PRD/`research/quality-review-s02-3.md`、S03.2.2 复核。

### 6. S03 已有三个基础切片，整体调度恢复闭环仍未完成

| 切片 | 已落地内容 | 仍不能宣称 |
| --- | --- | --- |
| S03.1 | 共用互斥快照租约、`VACUUM INTO`、只读校验/摘要、旧下载接口 | 全栈备份、调度、保留轮转、离线恢复工具 |
| S03.2.1 | schema 6 五张 STRICT 表、关闭默认、当前头快照与离线账号兼容 | 已开放策略 API、根初始化、执行器、容量/轮转服务 |
| S03.2.2 | 专属根身份、私有权限、严格 marker/owner/manifest、平台无覆盖发布 | DB 完成登记、last-good、调度、下载租约、删除授权 |
| S03.2.3 及以后 | 冻结设计和隔离草稿 | 可用手动执行器、计划调度、自动轮转、恢复 UI/CLI 已交付 |

- `internal/app/backup.go:81` 仍为共用快照入口；`internal/app/server.go:255` 公开的是旧 `GET /api/backup`。
- `internal/app/backup_publish.go:322` 的发布器明确只返回磁盘事实，不完成 DB 登记/last-good/轮转授权。`internal/app/backup_store.go:32` 的迁移只登记关闭的默认策略。
- S03.2.3 三个草稿 `backup_api.go` / `backup_state.go` / `backup_mutation.go` 已按摘要保存为 `.go.txt` 后移除，不能据此声称五个新路由或唯一执行器已存在。
- 下一实施入口仍是归档任务中的 `executor-manual-contract.md`；恢复实施前须重新核对当前代码、迁移头和冻结接缝，不直接把旧草稿复制回编译目录。
- 证据等级：**当前源码/规范 + 历史本机验收 + 尚未实施**；来源为三份 backup spec、S03 PRD 与 `research/s03-2-3-draft-checkpoint.md`。

### 7. 从备份切片保留的工程经验可用于后续清理和跨资源工作

- 快照是敏感的 `database-only` 材料；媒体、source secrets、helper/worker 身份、组件卷、下载原件、用户资产均有独立恢复责任。备份 DB 不等于完整灾难恢复。
- 发布确定性与 error 分开：最终移动尝试后的未知结果保留同一 ID/材料待对账；不能删除未知结果、换 fileId 或再生成源去“重试”。
- 只按精确归属清理已证明自有成员，owner 最后删；未知文件、身份变化和 ACL 不合格时保留，不用 `RemoveAll` 或递归 ACL 修复。
- 当前 publisher 的空间核验在源已存在之后，不等于执行器源前峰值准入、更不等于空间预留。源前峰值/唯一 manager/完成授权/重启对账均是后片责任。
- 证据等级：**当前规范与源码 + 历史 Windows 行为验证**；来源为 `.trellis/spec/backend/backup-contract.md`、`backup-publish-contract.md` 及 S03.2.2 独立复核。真实崩溃/断电没有被实测证明。

### 8. S04–S12 是有用的实施卡，不能因归档而抹掉待办

| 任务 | 后续最小切片与关键门禁 | 依据（目录均在本月 archive） |
| --- | --- | --- |
| S04 事件 | 先事件存储/未读；Webhook 默认关、不可变目标修订、有界至少一次，恢复不批量重投 | `11-23-nas-events-notifications/prd.md`；只读设计研究不是实现 |
| S05 作品 | 保持原 item_id/文件视图，作品/季集/多版本显式关联；模糊项不静默合并 | `11-23-nas-work-catalog/prd.md` |
| S06 元数据 | 候选预览→来源/人工值/锁定→资产闭环；上传原件先有 S03 恢复证明 | `11-23-nas-local-metadata/prd.md` |
| S07 字幕 | 有界异步诊断、授权 SRT/VTT、播放器资源回收；同目录不代表 sidecar 已发布 | `11-23-nas-subtitles-diagnostics/prd.md` |
| S08 管理 | 现有字段筛选可先行；200 ID 小批量全有或全无；失联重关联保留身份且拒绝已索引目标 | `11-23-nas-library-management/prd.md` |
| S09 自动入库 | 显式可信完成根→已有库，先试运行/默认停用，首轮只 register；必须核验真实 itemIds | `11-23-nas-external-auto-import/prd.md` |
| S10 容量 | 先只读口径，再保数据归档/精确清理；热历史减少不等于媒体字节释放 | `11-23-nas-storage-maintenance/prd.md` |
| S11 规则 | 模板/解释共用真实 evaluator；缺集依赖可信作品/预期集目，未知不显示假 100% | `11-23-nas-rule-guidance/prd.md` |
| S12 组件 | 先实际状态/单组件停启，再只读计划和单条真实升级路径；维护需匹配恢复点 | `11-23-nas-component-maintenance/prd.md` |

- 证据等级：**规划/设计，尚未取得业务交付证据**。后续实施时逐片将已确认的实际契约迁入 spec，不把这些拟议字段/阈值提前当当前接口。

### 9. 通用插件与飞牛兼容需要独立立项，不能借固定组件成功提前放行

- 该任务交付的是规划：P0 兼容验证 → P1 市场/中心 → P2 受控执行 → P3 飞牛桥接 → P4 生命周期/Windows 集成 → P5 业务扩展。
- 当前宿主/版本、实际市场协议、首批应用和真实 fnOS 动作未确认；普通 Docker 成功不等于 FPK 可运行，Windows/Linux 不承诺任意原生 FPK。
- 旧 PRD 的“schema v2 / 两引擎”是 11-19 的历史背景，当前已是 schema 6 和更丰富固定能力，后续不得照搬。
- 证据等级：**规划 + 历史背景**；来源为 `11-19-plugin-market-fnos-plan/prd.md:9–30,78–110`。本次没有重新核验外部资料的时效。

## Related Specs / External References

- 当前实施权威：`.trellis/spec/backend/baseline-contract.md`、`account-contract.md`、`backup-contract.md`、`backup-storage-contract.md`、`backup-publish-contract.md`。
- 外部引用：本研究未联网、不新增外部版本或兼容声明；既有研究的外部资料仅保留其原日期，后续正式实施再按当时官方资料核验。
- CodeGraph：使用现有只读索引定位 `publishBackupSnapshot` / `RunAccountMaintenance` 并回读当前源码；未同步或改写共享索引。

## Caveats / Not Found

- 本次只是归档证据整理，没有重新运行历史业务回归或目标设备测试；历史通过数不能标成 2026-11-27 新测试结果。
- 归档后旧文档中 `.trellis/tasks/<原目录>` 和依赖层数的相对链接可能失效；引用时使用新的 archive 路径。本文不改写其他任务的历史资料。
- 尚无本轮可用的真实 Linux/NAS、介质故障、物理断电、大库规模、全栈恢复证据，不能从 Windows 测试、注入故障或交叉编译推断已通过。

## 原归档批次 50 任务状态清单

来源：同目录 `pre-archive-task-status.json`，源提交 `68bbd0e3eced0e5e062ad7a45941d99dd39ae1dc`。状态只解释归档前背景，实际完成情况按上文证据判断。

| 原任务目录 | 归档前状态 |
| --- | --- |
| 00-bootstrap-guidelines | in_progress |
| 11-17-bt-download | review |
| 11-17-music-support | review |
| 11-17-nas-content-plan | review |
| 11-18-bt-engine-bootstrap | review |
| 11-18-password-policy | review |
| 11-18-project-local-runtime | review |
| 11-18-session-temp-cleanup | review |
| 11-18-torrent-search | review |
| 11-18-windows-complete-exit | review |
| 11-18-windows-one-click | review |
| 11-19-collection-core | completed |
| 11-19-collection-file-import | in_progress |
| 11-19-collection-release | in_progress |
| 11-19-collection-rss-bt | completed |
| 11-19-collection-web | completed |
| 11-19-plugin-market-fnos-plan | planning |
| 11-19-resource-collection-plan | in_progress |
| 11-19-windows-desktop-icon | review |
| 11-20-collection-component-install | review |
| 11-20-collection-external-api | review |
| 11-20-cross-platform-manual | completed |
| 11-20-powershell-dev-encoding | completed |
| 11-20-powershell-npm-arguments | completed |
| 11-20-vite-proxy-origin | completed |
| 11-23-nas-account-lifecycle | completed |
| 11-23-nas-backup-recovery | in_progress |
| 11-23-nas-component-maintenance | planning |
| 11-23-nas-events-notifications | planning |
| 11-23-nas-evolution-plan | in_progress |
| 11-23-nas-external-auto-import | planning |
| 11-23-nas-feature-gap-assessment | completed |
| 11-23-nas-library-management | planning |
| 11-23-nas-local-metadata | planning |
| 11-23-nas-rule-guidance | planning |
| 11-23-nas-storage-maintenance | planning |
| 11-23-nas-subtitles-diagnostics | planning |
| 11-23-nas-validation-baseline | completed |
| 11-23-nas-work-catalog | planning |
| 11-25-jmcomic-advanced | review |
| 11-25-jmcomic-plugin | review |
| 11-25-uav-downloader-plugin | review |
| 11-26-bt-windows-native | review |
| 11-26-jmcomic-windows-native | review |
| 11-26-runtime-slim | completed |
| 11-26-uav-windows-native | review |
| 11-26-windows-dotenv | review |
| 11-26-windows-portable-profile | review |
| 11-27-windows-uav-fake-ip-repair | review |
| 11-27-windows-uav-local-repair | review |
