# 资源收集后端实现与验证（2026-11-20）

## 已交付范围

负责 `internal/app/`：新增 `collection_models.go`、`collection_config.go`、`collection_store.go`、`collection_worker.go`、`collection_api.go`、`collection_preview.go`、`collection_runtime.go`、`collection_import.go`、`collection_rss.go`、`collection_rules.go`、`collection_bt.go`；修改 `config.go`、`server.go`、`store.go`、`downloads.go`、`download_gate.go`、`libraries.go`。新增固定来源控制面、schema v3、独立秘密文件、RSS 基线/回填/规则、共享 BT 登记服务、本地只读登记、worker 预览/原编号对账/发布、精确扫描覆盖和独立入库重试。实现字段见同任务 `api-contract.md`；部署与 worker/UI 证据由对应报告记录。

所有新增方法/字段/配置具有中文注释，关键派发、来源配置、阶段变化使用脱敏中文日志。后台协调器纳入 App context/waitgroup。没有操作用户真实帐号或来源。

## 临时行为验证证据

最终命令：

```text
python -B scripts/project_env.py -- go test ./internal/app -run TestCollection -v -count=1
PASS
ok nas-content-manager/internal/app 2.489s
```

17 个顶层用例全部通过，其中自动规则包含 6 个独立子场景。测试使用真实 SQLite、真实临时文件、真实媒体扫描器与 HTTP 隔离服务；网页获取和 qB 传输本身不在这些单元用例中伪装为真实工具验证。

随后审查补充的 `TestCollectionBTHistoricalMirror` 独立执行通过（`go test ./internal/app -run TestCollectionBTHistoricalMirror -v -count=1`，`ok ... 0.695s`）：实际持久清单与真实文件通过发布验证后修正同状态但已变化的ItemIDs；第257条已完成BT在轮转中观察原Download的target_missing并清旧链接；仅归档引擎而保留有效媒体仍完成；相同大小mtime变更撤销入库；真实删除媒体库立即撤销BT ItemIDs；原download记录删除进入待对账。累计18个顶层行为用例。

最后 `TestCollectionBTUnboundTargetDeletion` 独立执行通过（`ok ... 0.525s`）：尚无downloadId的BT收集任务经真实deleteLibrary后为failed/pending，普通retry返回202；已绑定下载与尚未绑定下载按实际阶段区分。累计19个顶层行为用例，临时文件同样执行后清理。

| 临时用例 | 实际断言 |
| --- | --- |
| `TestCollectionReadOnlyImport` | 本地预览→提交→真实扫描→精确 ItemIDs；扫描前不能入库；原请求重复返回原任务；新编号重复指纹409；改变目标409；绑定后原始零目标意图仍可重放；源字节不变；真实 deleteLibrary 立即清 ItemIDs 并标入库失败；单独重试不重建已删目标库。 |
| `TestCollectionPreviewVersion` | 文件在预览后改大小提交409且未创建任务；来源版本冲突409；路径穿越、反斜杠、绝对路径、不规范分段与尾点拒绝。 |
| `TestCollectionGate` | staging、private-state、未知成品、未发布成品、未选择额外文件拒绝；精确已发布版本允许；mtime 改动拒绝并冻结；移除当前 worker 配置后历史发布根仍保留正确门禁。 |
| `TestCollectionRSS` | 默认拒绝 loopback 且无网络命中；明确 AllowPrivate 后真实拉取；首次 baseline；ETag304；显式 backfill；稳定条目去重；主数据库公开 body 不含来源 token/GUID/定位符。 |
| `TestCollectionMigration` | 创建包含既有 user/library/item 的 v2 库后升级 v3；旧 item ID11、title、library ID8 保留，新 schema 版本3。 |
| `TestCollectionRevisionAndRules` | 迟到更新无法覆盖取消修订；显式 season/episode 捕获；绝对集数、重复季集、E01E02、E01-E02、S01E01-02、1x01-02、中文范围拒绝；BT 选择保存真实引擎 index7，不使用数组下标。 |
| `TestCollectionImportChangedVersion` | 旧索引 mtime 不满足覆盖；同大小源文件 mtime 改动在入库前冻结，不能被当成旧预览成功。 |
| `TestCollectionCandidateConflict` | 同候选仅原 requestId、相同内容类型与原目标可复用；变更类型/目标/新编号409。 |
| `TestCollectionNetworkPolicy` | 转换/隧道保留IPv6、文档地址、CGNAT、metadata、保留IPv4即使明确内网仍拒绝；RFC1918/ULA/loopback仅显式允许；真实公网IPv4/IPv6通过策略。 |
| `TestCollectionAutomaticRules` | unique 可持久排队；same_episode、multi_episode、overlapping_rules、review、stale_rule 均只保留人工确认且不创建任务。 |
| `TestCollectionHistoricalEpisode` | 跨来源201条BT历史（命中位于第二页）和超过100条候选窗口仍阻止已获取集数自动升级；历史720p不因新规则1080p/大小条件而失去身份。 |
| `TestCollectionImportOnlyRetry` | 已发布文件第一次扫描失败后 Ready保留；普通retry409；retry_import生成新扫描；最终精确1个ItemID；整个重试对worker零请求，原成品字节不变。 |
| `TestCollectionUncertainWorkerSubmit` | 双方最小预算1024字节/5文件在预览和持久操作一致；非网页profile为空；worker接收POST后回502，主任务待对账；显式retry只GET原ID恢复acquiring，POST次数仍1；换持久实例且原ID404不能重发。 |
| `TestCollectionRunnableQueue` | 前256条failed历史不占用协调窗口，新queued任务仍被读取。 |
| `TestCollectionRSSSourceRevision` | 编辑RSS清ETag/LastModified，不被304阻断；重新抓取同身份后未确认候选更新快照/revision并可确认；已确认任务私密输入保持原版本。 |
| `TestCollectionUnpublishedTargetDeletion` | 未发布任务目标库删除后为failed/pending，可普通retry并cancel；不会进入Ready=false却只能retry_import的死状态。 |
| `TestCollectionPublishedRotation` | 前256条ready历史后，第257条mtime改变在下一轮有界完整性检查冻结，游标回绕。 |

生成的 `internal/app/collection_temporary_test.go` 与 `internal/app/collection_bt_temporary_test.go` 均已在各自最后一次通过后删除，按用户要求不保留本轮临时单元测试；既有测试和可复用集成脚本未删除。

## 最终实现契约补充

- 相同资源新requestId返回409，只有原已接受编号能重放，不维护未持久化的请求别名。
- 自动规则使用 `S(?P<season>\d+)E(?P<episode>\d+)` 等显式命名捕获；关键词、唯一发行、唯一自动规则、未获取历史、唯一文件选择同时满足才派发。锁内再核对来源 enabled/revision 与订阅 enabled/revision，旧授权不能迟到派发。
- RSS 配置编辑后重新获取新快照；已接受任务引用不改。首次或更换来源URL保留历史baseline语义。
- worker能力必须有正数预算；预览持久保存双方预算最小值。worker持久InstanceID与Operation RequestHash均校对，避免目录映射/实例/请求意图混淆。
- 文件导入前核对真实size/纳秒mtime，索引覆盖也核对两者；目录漫画与扫描器一致按合计size/最大mtime核对。
- 删除媒体库与撤销非BT任务入库结果在同一事务提交；保留文件及原目标ID，不能自动重新创建同一目标。未发布任务可取消/对账，已发布任务可明确重试入库但目标仍缺失会准确失败。
- 协调队列排除等待人工处理的failed/reconciliation_required与入库失败；周期成品检查用行号游标分批256轮转，读门禁每次仍核对文件版本。
- 已绑定BT任务也参与相同行号轮转，即使业务任务曾completed仍观察Download权威事实；默认先撤销旧ItemIDs，只有真实发布版本和实际索引覆盖同时有效才恢复completed/imported。移除引擎但保留媒体与删除/更改产物分别表达。

## 构建和范围

已通过 `python -B scripts/project_env.py -- go vet ./...`、`python -B scripts/project_env.py -- go test ./...`；最终构建命令为 `python -B scripts/project_env.py -- go build -o .runtime/verify/nas.exe ./cmd/nas`，供主流程最终集成和浏览器验收使用。最后未绑定BT的CASE调整通过专项行为验证后再次构建成功，实际最终产物20,815,360字节，2026-11-20 02:00:30 UTC（10:00:30 Asia/Shanghai）。两个临时测试路径均经Test-Path确认False，`git diff --check -- internal/app`通过（只有CRLF转换提示）。

真实 app+worker 集成、全部旧功能脚本、前端/browser、真实yt-dlp/rclone、跨平台部署证据由主流程及worker研究记录汇总。本后端临时测试不声称验证真实NAS、Docker daemon、云盘帐号或公网BT传输。保留策略按文档固定保留任务/产物，不自动删用户文件；来源秘密和worker私密状态需要随备份一起恢复。
