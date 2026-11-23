# Research: S01–S04 可靠性能力实施设计

- Query: 将运行基线、单管理员账号生命周期、定时备份和持久事件中心拆解为逐步可实施、可验收、可恢复的小交付。
- Scope: internal；基于既有代码与项目契约的实施规划，不进行外部产品或最佳实践调研。
- Date: 2026-11-23
- Baseline: 父任务已核对 `main / 6f380be`；本研究不执行 Git 操作。以下所有新增接口、表、CLI、文件名和限额均为**拟议设计**，实施时按实际头版本复核，不能当作当前能力或已通过结果。

## Findings

### 1. 已核实的入口文件与复用模式

| 文件 | 作用与代码证据 |
| --- | --- |
| `internal/app/auth.go` | `validNewPassword` 在 25–46 行；单管理员建立在 59–100 行；登录兼容旧密码在 102–132 行；会话仅持有随机令牌摘要在 159–194 行；当前退出撤销在 197–206 行。 |
| `internal/app/store.go` | 15–45 行为本地 SQLite、单连接、WAL 与版本检查；150–151 行为唯一管理员和当前精简会话表。新能力必须增量迁移，不能重建或清空旧库。 |
| `internal/app/instance.go` | 9–51 行规范化目录并获取 OS 独占租约；所有离线写工具必须复用同一目录身份及锁。 |
| `internal/app/server.go` | 72–157 行统一拥有启动、请求、后台线程与退出；239–264 行为 API 路由；267–307 行为同源写入、鉴权外围安全和请求体限额。 |
| `cmd/nas/main.go` | 14–27 行目前直接加载配置并运行服务；尚无离线账号或备份子命令分派。 |
| `internal/app/media.go` | 75–121 行现有 `GET /api/backup` 通过备份互斥锁、磁盘余量检查、`VACUUM INTO` 生成运行中数据库一致性快照；下载后移除临时文件。 |
| `internal/app/jobs.go` | 17–54 行为合并唤醒、固定后台执行器和周期扫描；57–73 行为短事务领取任务。只借鉴有界执行与生命周期，不把通知或备份硬塞进扫描/enrich 的既有状态机。 |
| `internal/app/collection_store.go` | 63–105 行为私密目录、随机引用、0600 临时文件、同步及同目录替换；108 行起为有界读取并拒绝符号链接。新秘密文件复用约束，另做 Windows ACL 验证。 |
| `internal/app/collection_rss.go` | 35–79 行为每次拨号校验 DNS 结果与实际连接 IP 的模式。RSS 在 81–85 行允许有限同源重定向，**Webhook 应明确拒绝所有重定向**，不可直接套用该行为。 |
| `internal/app/collection_external_store.go` | 23–50 行为公开安全 DTO、私密引用和持久化不确定外部动作；通知可借鉴幂等意图和恢复边界，不能把 HTTP 超时当作未投递证明。 |
| `web/src/App.vue` | 84–88 行现有 `notify()` 为临时 toast；430 行设置页已有数据库下载、恢复提示、主题及退出入口。应新增局部设置组件，保留现有导航与媒体能力。 |
| `web/src/api.ts` | 13–27 行统一同源 JSON 与 401 会话过期处理；41–48 行提供 UUID 请求键。 |
| `scripts/verify*.py` | 已有媒体、下载、搜种、托管接入、收集、外部 API、组件、桌面和发布验证工具；应编排既有脚本，保留各自真实/夹具证据标签。 |
| `docs/verification.md` | 405–417 行已有历史验证证据和真实 NAS/真实组件未测边界；不能把历史记录写成本轮重跑结果。 |
| `docs/collection.md`、`docs/windows.md`、`docs/components.md` | 分别在 121–123、104–110、50–56 行已有恢复材料与停机顺序。缺口是调度、核验、清单自动化与演练证据，不是从零补恢复说明。 |

检索已优先用 CodeGraph CLI 定位 `acquireInstance`、`RunWithOptions` 和 `backup`，再按位置读取源码。CodeGraph 返回的行号已与文本读取对应；未重新建图或改动索引。

### 2. 总体拆分、依赖与实施公共门禁

| 任务 | 相对规模 | 硬依赖 | 可并行部分 | 完成定义 |
| --- | --- | --- | --- | --- |
| S01 运行基线与目标验收轨道 | M | 无 | 本机基线与真实设备资料收集可分轨 | 可重放命令、独立结果、已知未测项和隔离目录规则可用。 |
| S02 单管理员账号生命周期 | M | S01 本机基线可用 | 与 S03 主体互不依赖 | 改密、列出/撤销会话、停服离线重置及兼容旧账号验收通过。 |
| S03 定时备份与离线恢复核验 | L | S01 本机基线可用 | 与 S02 主体互不依赖 | 持久调度、保留轮转、核验报告和隔离恢复演练通过。 |
| S04 持久事件与可选 Webhook | L | S01 本机基线可用 | 事件存储可先于 S02/S03 开发 | 未读事件重启保留；投递限额/重复/恢复/网络边界可验证。S03 失败事件接入后再完成全链路验收。 |

实施顺序建议：S01 首个基线 PR → S02 和 S03 → S04 全链路；S04 存储可提前，S03 不以 S04 为前置，避免依赖环。真实 NAS 验收作为发布门禁持续补证，不阻塞无关的本地开发；未通过的环境不能标记支持已验收。

公共要求：Go 方法/实体字段/配置均有中文解释，显式输入输出类型，关键业务动作输出脱敏中文日志；前端严格 TypeScript、复用 `PasswordInput` 和 API 助手。所有本地构建/测试/临时工具通过 `python -B scripts/project_env.py -- ...`；不写用户级环境或 `go env -w`。新增单元测试临时运行后仅清除本次生成项，保留可复用集成脚本和脱敏结果。每个业务 PR 单独跑受影响回归；最终里程碑再跑完整组合。

所有新迁移按实际串行合并顺序分配版本，**本计划不预占未来 schema 编号**。升级前保存一致性数据库与配套私密材料；旧程序不得打开新 schema，不允许手改 `user_version` 或猜测降级。

### 3. S01：回归基线与目标环境验收

#### 3.1 范围与接口

本项不增加产品 API/业务表。拟新增 `scripts/verify_baseline.py` 作为现有脚本的编排入口，及版本化结果说明文档；它不能默认触碰用户目录或启动真实组件。

拟议 CLI：`python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --artifacts <已构建产物目录> --root .runtime/verify/<唯一运行编号>`。`--suite local|target` 明确区分受控本机与目标环境；`target` 还必须给显式目标描述文件，并逐项授权会改变运行状态的检查。暂不把上述拟议入口当作现成命令。

单次 `run-manifest.json` 至少保存：运行编号、源码版本和工作区是否含修改（由实施者的主流程采集）、开始/结束时间、平台/CPU/文件系统、产物及固定镜像摘要、命令与退出码、工具版本、用例结果 `passed|failed|blocked|not_run`、证据相对路径、未满足前置条件。敏感命令参数、主机私密地址、账号和令牌不得进入公开报告；缺失凭据以 `blocked` 标记，不能跳过后合成通过。

#### 3.2 小 PR 顺序

1. **S01-A：基线清单与执行协议。** 整理现有脚本的实际参数和各自前置条件；统一证据分类（协议夹具、本机实际应用、本机实际工具、真实 NAS/组件）。给每类功能固定最小冒烟与按变更触发的扩展组合，保留手动 `workflow_dispatch` 策略。
2. **S01-B：有界编排与结果汇总。** 只调现有脚本；一个 suite 使用独立根目录/端口，超时只回收自己创建的进程；单项失败保留日志并可继续独立项。校验 `--root` 必须在项目 `.runtime/verify/` 下且无链接越界，不能覆盖已有运行。产物缺失直接报前置不足。
3. **S01-C：目标环境矩阵和首次实测记录。** 记录选定 NAS 品牌/系统版本、Linux amd64/arm64 实际设备、磁盘/文件系统、UID/GID/ACL、网络挂载和 Docker 版本；另列 Windows 原生、Docker Desktop。只对实际拿到的组合写结果，其余保留 `blocked/not_run` 和需要提供的环境。

#### 3.3 可立即复用的命令与验收

已有且参数已读取的命令形态包括：

```text
python -B scripts/project_env.py -- go vet ./...
python -B scripts/project_env.py -- go test ./...
python -B scripts/project_env.py -- go build -o .runtime/bin/nas.exe ./cmd/nas
python -B scripts/project_env.py -- npm --prefix web run typecheck
python -B scripts/project_env.py -- npm --prefix web run build
python -B scripts/project_env.py -- python -B scripts/verify_downloads.py --binary .runtime/bin/nas.exe --root .runtime/verify/<唯一运行编号>/downloads
python -B scripts/project_env.py -- python -B scripts/verify_torrent_search.py --binary .runtime/bin/nas.exe --root .runtime/verify/<唯一运行编号>/search
python -B scripts/project_env.py -- python -B scripts/verify_engine_bootstrap.py --binary .runtime/bin/nas.exe --root .runtime/verify/<唯一运行编号>/engines
python -B scripts/project_env.py -- python -B scripts/verify_external.py --binary .runtime/bin/nas.exe --root .runtime/verify/<唯一运行编号>/external --workflow
python -B scripts/project_env.py -- python -B scripts/verify_components.py --binary .runtime/bin/nas.exe --root .runtime/verify/<唯一运行编号>/components
python -B scripts/project_env.py -- python -B scripts/verify_release.py --self-test
```

以上为后续实施命令模板，`<唯一运行编号>` 须替换，未在本研究执行；Linux 使用对应本机构建文件。`verify_collection.py` 还要求 `--worker-binary`，真实 rclone/yt-dlp/FFmpeg 是独立显式选项。`verify.py` 要求已有被测服务，使用 `--base-url`、`--root`，可选 `--large-file`；不应误以为它带 `--binary` 自动管理实例。桌面验证使用 `verify_desktop.py --binary <桌面产物>`；不对普通主服务二进制运行。

最低验收：同版本二次运行结果可比较且目录不冲突；有意缺产物、占端口、失败子进程、超时、链接越界均产出真实失败/阻塞证据并完成自有进程回收；五媒体与账号/标签/进度保留、备份一致性、下载发布门禁、外部操作不确定状态至少有原脚本对应结果。5 GiB 稀疏文件只记作 Range 偏移证据，真实视频编码、吞吐、内存峰值另测。目标环境包含只读媒体写入拒绝、UID/ACL、重启/网络中断、停服恢复及受控掉电演练；破坏性演练只在已备份的隔离副本进行。

失败/回退：本项不迁移数据、不改变服务运行策略；编排脚本失败不能终止用户已有进程。回退移除编排入口即可，保留已有验证脚本与证据。真实设备资料缺失只阻塞该矩阵的通过声明。

### 4. S02：单管理员改密、会话管理和离线重置

#### 4.1 拟议契约

继续 `users.id=1`，不顺带引入注册、多用户、共享权限或匿名找回密码。

| 拟议接口/数据 | 约束 |
| --- | --- |
| `POST /api/account/password`，`{currentPassword,newPassword}` | 鉴权与同源；原密码按现有 bcrypt 校验，新密码复用现有 10–72 ASCII 四类规则，不 trim。成功返回 `200 {requiresLogin:true}`，清 Cookie，撤销全部会话。 |
| `GET /api/account/sessions?cursor=...&limit=...` | 默认 20、最多 100；安全会话编号、`current`、创建时间（历史未知用 null）、过期时间和有界客户端说明。不返回 token/hash、精确 IP 或未经脱敏 UA。 |
| `DELETE /api/account/sessions/{sessionId}` | 只允许唯一管理员自己的会话；幂等撤销，不把公开编号作为身份凭据。撤销当前会话后清 Cookie。 |
| `POST /api/account/sessions/revoke-others`，`{currentPassword}` | 再认证并限流；短事务内保留当前会话、删除其余；响应被撤销数量。 |
| `users.auth_revision`、`sessions.auth_revision` | 整数默认同一初始值，兼容现存会话；改密递增账号修订并删除会话。登录验证密码后，创建会话必须对原账号修订/密码快照做条件检查，避免改密期间旧密码验证线程补建新会话。 |
| 会话附加字段 | 非 bearer 的随机公开 `session_id` 唯一；`created_at` 对历史记录可空；客户端说明限长且不执行 HTML。不保存原令牌。迁移给旧会话分配编号但不杜撰创建时间。 |
| `nas account reset-password --data-dir <目录>` | 拟议停服离线命令；交互隐藏输入两次并复用新密码规则。管道模式须显式 `--password-stdin`；禁止密码出现在 argv/环境变量/日志。无需网络或媒体根。 |

拟议内部边界为 `ChangeAdminPassword(ctx context.Context, input ChangePasswordInput) error`、`ResetAdminPasswordOffline(ctx context.Context, options ResetPasswordOptions, input io.Reader) error`，实际命名由实施任务确定；所有结构字段有中文注释。命令只开账号所需存储，不调用会启动监听/扫描的 `RunWithOptions`；也不直接使用附带任务恢复和缓存清理的 `openDatabase`。离线工具先获取现有 canonical data lease，确认目录中已有数据库、schema 可支持、管理员存在，再执行短事务；不自动创建空账号、不猜测旧版迁移。遇不支持版本给明确匹配版本/升级指引。

#### 4.2 小 PR 顺序和安全边界

1. **S02-A：账号存储契约及并发防线。** 增量增加修订与会话元数据；迁移保留旧用户/hash/会话；登录的密码比较可在事务外做，但最终建会话需比较原修订。改密在事务内再次校验预期修订、当前有效会话与原密码快照，CAS 失败返回冲突且不部分改写。
2. **S02-B：鉴权 API 与局部 UI。** 设置页“账号安全”中改密、查看会话、退出其他设备；复用 PasswordInput，密码不会回填或长期保留；提交后清字段和敏感状态，401 使用既有 session-expired。改密/再认证限流独立有界，不以登录成功绕过。
3. **S02-C：停服维护 CLI 与操作手册。** 在现有启动入口之前解析明确子命令；零共享实例锁冲突直接拒绝，绝不杀占用者。事务更新 hash、修订与撤销会话，提交后记录只含管理员编号和结果的中文审计日志；中断前后保持旧密码或新密码一种完整状态。补 Windows 原生与 Linux 操作示例。

撤销保证“提交后的新请求不再认证旧会话”；已获准进入的普通请求/媒体传输可能完成，不宣称强行回收所有既有请求。敏感账号变更必须在最终事务重新检查会话和修订，不能仅靠路由开始时检查。不要为此新增全局短流超时或破坏媒体读取。

#### 4.3 验收与回退

- 合法新密码改密后旧密码和所有旧 Cookie 均失败，新密码可登录；旧中文/弱密码仍可登录并作为 currentPassword 完成升级；包含中文、空格、超过 72 字节或缺一类别的新密码被拒绝，库不变。
- 登录比较旧密码后并发改密，旧线程不能补建可用会话；并发两次改密只有一个提交；撤销当前/其他会话、已过期会话与未知编号不泄露 token。
- 匿名、跨站、超限、重复/未知字段、超大正文和密码猜测有可预测的 401/403/400/429；日志、SQLite 公开元数据、API 响应和浏览器存储无原密码/令牌。
- 离线命令遇活实例、目录别名、缺库、较新 schema、未知用户、无读写权限、确认不一致或 Ctrl+C 均保留原密码；成功后全部旧会话失效，日志和命令历史无密码。Windows/Linux 锁分别实测。
- 回退功能代码前恢复匹配 schema 的数据库；已提交的密码修改不自动回滚成旧密码。不得以恢复旧快照静默复活旧会话；恢复整库后须明确重新撤销会话并完成管理员登录验证。

### 5. S03：定时备份、核验与离线恢复演练

#### 5.1 先明确两类备份

1. **在线数据库快照**：复用现有 `VACUUM INTO`，只含主数据库；不包含 worker 状态、私密来源/令牌、helper 安装身份、Docker 卷、下载载荷或媒体。
2. **完整恢复材料清单**：将既有三份恢复文档转成可核验 inventory，仍要求各服务停止或按其专用一致性方案备份。Web 应用不得宣称已在线一致性抓取所有组件卷。本项不实现自动跨服务打包、自动停栈和跨 OS 路径转换。

#### 5.2 拟议配置、API 与存储

- 首轮最多一条“每日 UTC 固定时间”的数据库备份策略，默认关闭；UI 显示本地换算。避免首轮引入任意 cron/夏令时解释。策略包含 `enabled`、`hourUTC`、`minuteUTC`、`keepCount`（建议默认 7、允许 1–30）、`maxBytes`（正整数 int64、且不得小于一次快照需求）；所有配置给中文帮助。保留数量/体积均有界，不能配置删除媒体。
- 宿主通过 `NAS_BACKUP_DIR` 提供专用备份根，默认数据目录下独立 `backups/`，不从浏览器接受任意输出路径。核验根不在任何媒体/下载/worker staging 根中，解析链接及目录别名，必要时拒绝启用；默认同盘副本不是异地灾备，UI 明示。远端 NAS 目录若不支持预期原子/持久化语义，不在首轮承诺支持。
- 拟议表：`backup_policy(id=1,revision,enabled,hour_utc,minute_utc,keep_count,max_bytes,next_run_at,updated_at)`；`backup_runs(id,request_id UNIQUE,trigger,schedule_key UNIQUE,state,created_at,started_at,finished_at,file_id,size,sha256,message)`。状态 `queued|running|completed|failed|interrupted`；历史有界（建议最近 200 条），保留的文件引用不随历史清理失去所有权。
- `GET/PUT /api/backup-policy`，更新带 `revision`；`POST /api/backups {requestId}` 返回 202 持久运行记录；`GET /api/backups?cursor&limit`；`GET /api/backups/{id}`；`GET /api/backups/{id}/file` 仅下载 completed 且仍保留/核验一致的自有文件。保留现有 `GET /api/backup` 兼容；它和新执行器共享同一快照互斥，冲突返回现有 409，不偷偷建立第二个并行快照。
- 单个文件附伴随 manifest：格式版本、运行 ID、创建时间、应用版本、**实际读出的**数据库 schema、字节数、SHA-256、`scope=database-only`、配套材料角色列表。SHA-256 只证明字节一致性，不证明来源可信。manifest 不放任何凭据、私有上游地址或秘密内容。

#### 5.3 调度与文件原子边界

由 `RunWithOptions` 管理固定一个备份执行器和有界定时器，接入 `a.ctx` / `a.wg`；短事务按 `(策略修订,计划时间)` 唯一键领取。重启错过多次只补最近一个计划点，过往记跳过摘要，不制造补跑风暴；时钟回拨不重做既有 schedule_key。手动请求重复 UUID 返回原 run，不重复生成文件。

生成过程：确认预算与目标 → 创建本应用所有的临时路径 → 执行一致性快照 → 只读打开快照并执行 `PRAGMA quick_check` / 外键检查、读版本 → 流式计算哈希 → 同目录提交文件及 manifest → 短事务登记 completed → 成功后才轮转。快照和校验不持有长业务事务；执行受应用取消控制，不能整库读入内存。

文件与 SQLite 不是同一事务：启动仅在专属目录内核对 run ID、完整 manifest、大小和哈希，处理“文件已提交/记录未提交”及“running 被打断”。可验证的自有完整产物允许补记完成；其他标 interrupted 并清理已识别临时文件。不遍历删除任意未知文件，不把孤立文件自动当可恢复备份。

轮转只操作明确登记且路径/文件身份复核通过的自有 completed 文件；至少保留最近一份已验证成功快照。先检查本次备份所需空间，不先删最后一份旧备份换空间；`maxBytes` 容不下一次或磁盘不足返回失败并保留旧备份。下载期间用文件租约避免 Windows 删除冲突，跳过活跃下载文件，下轮再清理。未达成保留策略应显示实际占用和积压，不谎报成功。

#### 5.4 拟议离线 CLI 与小 PR

拟议 `nas backup verify --snapshot <文件> --manifest <文件> --report <新文件>`：只读打开备份，拒绝链接/大小失配/不支持 manifest、哈希错误、SQLite 损坏/外键错误、较新 schema；不运行迁移、任务恢复或启动网络。输出机器 JSON，退出码明确区分通过、材料不全、不可用；不上传报告。

拟议 `nas backup inspect-recovery --data-dir <停服目录> --inventory <清单> --report <新文件>`：复用数据租约、只核验当前可读材料、角色/实例身份和权限，不停容器、不恢复文件、不触发外部写。无法读取 Docker 命名卷等材料记 `unverified`，不能记通过。清单的 `role`、备份文件引用、预期实例/卷身份、摘要/大小、敏感级别和恢复顺序由已停服备份流程填写；秘密内容不得复制到普通报告。

1. **S03-A：快照服务抽取和校验。** 保持现有下载接口行为；新增私有内部快照结果与校验函数，完成失败清理和共同互斥，不先加定时器。
2. **S03-B：持久任务、策略与轮转。** 增量迁移、单执行器、missed-run 合并、重启对账、受控根目录和有界保留；先通过 API 验收。
3. **S03-C：设置 UI、离线核验与 inventory。** 只显示真实状态/容量/最近成功/失败；从既有恢复手册整理材料角色，不实现一键恢复。增加隔离恢复演练脚本/说明。
4. **S03-D：目标环境演练与交接。** 在隔离新目录恢复数据库及匹配私密材料，验证账号、媒体索引、用户状态和任务恢复；核对 helper/worker 身份与未知外部动作不重发。S03交付已有日志与backup_runs，不等待S04；后续备份失败事件接入与全链路联调由S04负责。

规划收敛补充：恢复副本对外启动前必须通过独立、显式的离线恢复准备步骤，在数据目录独占锁内撤销快照内全部会话；受支持schema含auth_revision时同步推进，不改密码、不启动服务/任务、不隐式迁移。不把该写步骤混入只读verify/inspect；不支持快照版本时阻止启用并给匹配版本指引。相应实现与旧Cookie失效验收归S03，复用S02离线存储边界但不强制依赖S02 UI。

资产验收归属：S03交付通用持久资产登记/清单、受控文件采集与恢复，用合成原件及引用完成自身验收；S06.3负责将真实上传海报接入并完成端到端恢复后开放入口。S03不等待S06，避免资产备份和上传功能互为完成前置。

#### 5.5 验收、失败与回退

- 在线写入收藏/进度的同时备份，产物可打开、检查通过，代表某个一致性时间点；不要求包含快照开始后的所有后续写入。手动/定时争用只生成一个；相同 UUID 不重复。
- 备份中取消/退出、磁盘低空间、目录卸载、只读权限、符号链接替换、快照损坏、写 manifest 后宕机、登记前宕机、轮转被占用均有明确结果；原库/媒体/上一份好备份不损坏。
- 重启跨过十个计划点只补一次；改策略、时钟回拨和重复 tick 无重复 schedule_key；后台退出后才释放 DB/lease。
- 核验拒绝错误 SHA、错误 schema、非 SQLite、缺材料和错实例；用新空目录及专属端口恢复，实际读取既有内容并核对收藏/标签/进度；外部不确定写仍待核对。
- 禁用策略后停止接纳计划，已接纳任务有明确取消/完成结果；不删备份。功能回退不降级 schema，不自动删除 inventory/私密文件；程序回滚使用升级前完整匹配材料。生产恢复是另一次显式维护操作，不由核验命令顺手执行。

### 6. S04：持久事件中心和可选 Webhook

#### 6.1 范围、事件和拟议数据

首轮为唯一管理员的持久未读中心及一个可选目标；不引入站内聊天、邮件/短信矩阵或通用脚本插件。保留已有业务任务/阶段历史，统一事件只是可追踪摘要，不能替换事实状态。

首批固定事件：获取/导入失败、需要人工核对、来源/组件持续离线、磁盘低水位、备份失败。持续故障有阈值和冷却（建议连续 3 次且至少 5 分钟），恢复发一条解除事件；复用各模块已有探测事实，不因为事件中心另开重复的任意网络探测。低磁盘的完整容量策略由后续 S10 提供，首轮只消费现有 freeDisk 检测并清楚标出监测范围。

拟议表：

- `system_events(id,type,severity,resource_type,resource_id,occurrence_key UNIQUE,summary,created_at,read_at,resolved_at,repeat_count,last_seen_at)`；`summary` 为有界安全中文，resource 为本地 ID，不放绝对路径、原始 URL 或异常原文。故障一次 episode 对应稳定 occurrence_key，冷却内重复更新次数，恢复后再次发生使用新 episode。
- `notification_channels(id=1,revision,enabled,private_ref,configured,updated_at)`；私密 URL/授权头/签名密钥在独立 0600/受限 ACL 文件，只保存私密引用和安全配置状态到 DB。
- `notification_deliveries(id,event_id,channel_revision,state,attempts,next_attempt_at,last_error,created_at,updated_at,UNIQUE(event_id,channel_revision))`，状态 `queued|sending|retry_wait|delivered|failed|disabled`。投递使用接受时的不可变私密目标快照，编辑目标后旧队列不会被静默改发到新目标；废弃私密快照只有无活跃引用且保留期到后才能清理。

同库业务终态和事件可在短事务原子写入；已有外部操作事实回执必须先遵循原状态机，不能因通知失败盲重发外部业务。备份等跨文件系统操作用可重放本地对账及 occurrence_key 修复“状态已完成/事件未登记”。Webhook 网络调用永不放在事务或业务锁内。

事件建议保留最多 90 天/10000 条，超过数量优先清理已读旧事件，再清理最老事件并维护可见的截断计数（包括可能过期的未读）；不能以“永远保留未读”制造无界增长。投递活跃引用独立保留安全 payload 或延迟清理，不能外键级联误取消已排队通知。读标记不是不可丢失审计，关键操作审计仍由原业务状态与日志承担。

#### 6.2 拟议 API、投递协议和网络限制

| 拟议接口 | 行为 |
| --- | --- |
| `GET /api/events?cursor&limit&unread&severity` | 稳定 ID 游标、默认 50/最多 100、返回未读计数和保留截断提示。 |
| `POST /api/events/read`，`{ids:[...]}` | 最多 100 个 ID；幂等标已读。不提供 GET 写操作。 |
| `GET/PUT /api/notification-channel` | GET 仅 configured/enabled/revision/安全标签；PUT 带 revision，私密连接写入只进不出，清空凭据必须独立显式动作。 |
| `POST /api/notification-channel/test`，`{requestId}` | 生成固定安全测试事件并异步投递，限流、幂等；不能接受任意 URL/body/header 作为临时 HTTP 客户端。 |
| `GET /api/notification-deliveries?cursor&limit` | 安全事件 ID、状态、尝试次数、下次时间、脱敏原因；不返回上游响应原文。 |
| `POST /api/notification-deliveries/{id}/retry`，`{requestId}` | 只重试最终失败项；提示接收方可能已处理；未终止投递返回冲突。新投递代次保留原事件幂等标识。 |

载荷是固定 JSON（建议不超过 16 KiB），含格式版本、稳定 eventId、类型、级别、发生时间、中文摘要、资源类型和 ID；默认不携带可泄漏内网部署的绝对链接。发送 `X-Qiku-Event-Id` 供接收方去重；可选 HMAC-SHA256 签名使用独立随机秘密并签时间戳+原始请求字节。首轮可只支持原生通用 JSON 接收端，不承诺直接兼容各聊天产品独有格式。

默认仅 HTTPS 固定来源，拒绝 URL userinfo/fragment、控制字符、任意查询凭据、无效端口、转义分隔与路径穿越；需要 URL token 的服务应放私密配置且严格脱敏，作为实施前显式兼容决定，不能默许泄露。私网/loopback 需单独授权；链路本地、云元数据、保留地址始终拒绝。每次拨号复查全部 DNS 结果并直连已验证 IP，保留正常 TLS 主机校验；拒绝所有 30x、无环境代理、无 cookie jar、无任意自定义 HTTP 方法。复用网络校验模式并单独测试 Webhook 更严策略。

建议全局并发 1，单次应用关联 deadline 10 秒，请求/响应头和响应正文限额分别明确，正文最多读取 64 KiB 且不落日志；2xx 为接收成功，408/429/5xx/传输失败退避重试，其他 4xx 最终失败。最多 6 次、最长 24 小时、退避带抖动，Retry-After 有上限，队列有总数与单事件唯一约束；到界记录失败并留站内事件，不能递归为“通知失败”再次发送自身而形成风暴。

语义为**有界至少一次尝试，可能重复，不能保证最终送达或 exactly-once**。超时/断连无法证明对方未处理；sending 进程中断恢复到 retry_wait，继续使用同一 eventId。测试、手动重试同样提示重复可能；不把该规则套到外部下载/订阅等不能盲重发的业务写操作。

#### 6.3 小 PR 顺序

1. **S04-A：事件存储与生产者接入。** 增量迁移、typed event helper、事务内事件与 episode 去重；先接入收集失败/人工确认及备份失败。存储失败保留既有业务恢复语义与中文日志，不凭通知推断任务成功。
2. **S04-B：持久未读 API/UI。** 顶部小入口或“更多/管理”内事件中心，移动端不新增第六主导航；加载失败与空态分开，已读请求有批次限额，重启未读保留、资源跳转使用本地 ID。
3. **S04-C：一个安全 Webhook 通道及有界 outbox。** 私密文件、修订快照、固定 HTTP 客户端、发送前持久领取、重试/去重/退出；先用受控接收 fixture 验证，不连接用户真实账号。
4. **S04-D：故障阈值、真实接收端验收与运维。** 接入持续离线和有限磁盘事实，测试告警合并/恢复事件；补凭据轮换、关闭通道、旧队列处理与数据恢复后的重投递政策。

关闭通道后不发送任何旧任务；保留队列状态为 disabled。再次启用默认只接收新事件，旧项要逐项显式重试。恢复数据库备份会回到旧投递状态，首次恢复应保持通道关闭，管理员核对后再启用，避免历史事件成批重发；这一恢复开关需有可操作 CLI/配置和恢复说明，不依赖“管理员应该知道”。

#### 6.4 验收与回退

- 关闭浏览器期间发生失败，重启后仍可见未读；同一故障连续探测只更新一个 episode，恢复后再故障有新事件；分页不丢项/不重复，截断计数可见。
- 业务状态与事件原子边界有故障注入；重复生产不重复建事件/投递；源模块原任务与未确认外部写保持原语义。
- fixture 分别返回 2xx、400、408、429、500、慢响应、超大正文、重定向和“处理后丢响应”；验证重试次数、超时、稳定 eventId、重复提示、重启恢复及关闭通道后无网络访问。
- DNS 重绑定、公网 HTTP 携密、未授权私网/loopback、metadata/link-local、IPv4/IPv6 保留地址、任意跳转均拒绝；代理变量和上游响应不能引起凭据外送或日志泄漏。
- 修改目标 revision 后原队列不投向新目标；秘密不在 DTO、普通 DB/快照、日志、localStorage；秘密文件权限在 Linux/Windows 单独验证。
- 退出时取消并等待投递及所有请求，之后才能关 DB/释放数据租约。通道禁用即可停止外送，事件仍可浏览；回退不删未读和业务记录，数据库回退仍需配套快照。真实 Webhook 接收端未经实际验证不得宣称兼容。

### 7. Related specs

- `.trellis/workflow.md`：Phase 1.2 研究落盘，未来实现/检查按 Phase 2 进行。
- `.trellis/spec/backend/index.md`：中文注释、入口检查和 Go/集成质量门禁。
- `.trellis/spec/backend/runtime-contract.md`：唯一管理员、新密码/旧登录兼容、SQLite、本地磁盘、源只读、任务/请求生命周期。
- `.trellis/spec/backend/cross-platform-contract.md`：目录租约、Windows/Linux 生命周期、完整模式数据身份、发布验收边界。
- `.trellis/spec/backend/external-collection-contract.md`：私密引用、DNS/connect 策略、固定业务 API、不确定写操作不得重发。
- `.trellis/spec/frontend/app-contract.md`：PasswordInput、401、最多五个移动端主入口、真实空/失败状态、可访问性与屏幕宽度/主题验收。
- `docs/api.md`、`docs/verification.md`、`docs/collection.md`、`docs/windows.md`、`docs/components.md`：未来实施应同步的 API、真实执行记录与操作手册。规划阶段保持现有契约真实，不把拟议设计写入 implemented spec。

### 8. External references (docs, versions)

本项未使用外部来源、未查询或推荐新依赖版本；Go/SQLite/Vue 的实际版本由 S01 从当前仓库与构建产物记录，不以记忆填写。优先复用现有库和纯 Go/标准库能力，若隐藏终端密码输入等需要新增依赖，实施前单独审核依赖、平台支持与许可证并记录锁定版本。

## Caveats / Not Found

- 本研究只读代码并新增此研究文档；未构建、未测试、未启动服务/Docker/浏览器，未连接真实 NAS、Webhook 或用户账号，不新增业务代码/测试文件。
- 当前不存在上述账号维护、定时备份、事件中心 API/数据表/CLI；所有命名与建议限额需在实施任务冻结后落实和验证。
- 已有完整恢复材料说明，不能再写“没有备份/没有恢复文档/没有持久事件记录”；准确表述是目前主库手动一致性备份、业务任务/阶段事件已经存在，缺统一自动调度/核验和跨模块未读通知中心。
- 首轮继续单管理员；家庭多用户、统一权限、SSO、邮件短信、多 Webhook、任意 cron、在线整栈一致性备份、一键生产恢复和跨平台路径迁移均不夹带实施。
- S02 与 S03 共享离线维护边界但不强制先后；合并时避免重复 CLI/租约封装。S04 与 S03 只在生产者集成阶段形成单向依赖，不能互相阻塞主体交付。
- 目标环境资料、真实硬件与镜像/接收端缺失时，相关结果保持 blocked/not_run；本机协议夹具或交叉编译不构成真实产品部署、掉电恢复与性能通过证据。
