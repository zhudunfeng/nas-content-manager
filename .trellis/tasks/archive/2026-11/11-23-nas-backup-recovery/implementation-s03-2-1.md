# S03.2.1 实施者自查与交接

2026-11-23。本片按主线已冻结的 `storage-schema6-contract.md` 完成实施和本机自查，停写交独立 checker；本报告不是独立验收结论。S03 整体仍实施中，主线负责正式 spec/docs、PRD/info/task 状态同步。本代理没有修改这些主线文件，没有覆盖旧 `implementation.md`，没有提交、部署、归档或接触用户实例。

## 本片变更

- `internal/app/backup_store.go`：新增冻结 raw `schemaV6`，包含五张 STRICT 表、中文字段注释、数值/毫秒/ID/UUIDv7/JSON 字节边界、授权全有全无、单 active 和单 deleting 部分唯一索引。没有跨表 FK、级联、触发器或无调用者 CRUD。迁移只使用调用方短事务，一次采实际 UTC 毫秒，一次读取16字节密码学随机并初始化两个单例；默认策略关闭、03:00 UTC、7份/10GiB，根未绑定、授权全 NULL。`migrateBackupSchemaWithSources` 是直接传参的窄时间/熵接缝，无全局时钟或生产开关；生产使用 `time.Now` 与 `io.ReadFull(crypto/rand.Reader)`，真正可返回的熵错误进入回滚。
- `internal/app/store.go`：当前头升6；在固定历史 v5 迁移后、原任务恢复前，追加 `<6` 的短事务，DDL/单例/头6同笔提交。成功输出固定安全中文日志；未知冲突不忽略。没有改写 schemaV1–V5 或其 PRAGMA 固定版本。
- `internal/app/account_offline_store.go`：显式支持集合 `{5,6}`；快照另存 `userVersion int64`，提交要求仍受支持且等于预检值，保留 SQLite schema cookie、ID/hash/revision、RowsAffected、取消与提交未知规则。不调用正常初始化或读取/对账备份域。
- `internal/app/backup_verify.go`：shared checker 仍仅当前头6；新增五表 LIMIT 0、实际表计数、STRICT、全部字段类型及两个 id=1 单例的只读探针。没有增加公开历史 verify、manifest schema expected 或恢复准备功能。
- `scripts/verify_schema.py`：历史构造扩为真实 v1–v5。v5 使用冻结 schemaV5，先复制合法账号/会话原列，再安装 schemaV5SessionConstraints；没有 source 时可构造空账号v5。旧1–4保留旧三列账号断言；不使用当前库改头伪装。
- `scripts/verify_account_offline.py`：future反例改7；新增真5成功/异常触发器拒绝、真实 Console 第二提示后的5→6/6→5 user_version barrier。已有真正6种入非空五表、授权、已保留和未发布 ownership，加磁盘 pending sentinel；逻辑快照证明成功只改账号三类事实。原锁、反向FK/触发器、输入、Console、取消、ACL与秘密检查继续执行。
- `scripts/verify_backup_store.py`：新增稳定实际应用专项，明确产物/全新根、真实v5、旧Cookie与原行、STRICT/默认关闭/无备份目录、重复启动身份稳定、未来7与冲突回滚。复用 BackupHarness 的专属 Console、真实停服、原始内存秘密扫描和确切进程所有权；保留失败证据。
- `scripts/verify.py`、`verify_backup.py`、`verify_account.py`、`verify_collection.py`、`verify_external.py`、`verify_engine_bootstrap.py`：仅同步本片涉及的“当前头”断言及对应标签；真实历史v4/v5构造和历史迁移常量不机械替换。最后三个脚本的最小同步已事先获主线纳入 ownership。
- 本报告 `implementation-s03-2-1.md`；本轮新增临时 Go 测试源已实际执行后精确删除。

本片没有增加备份配置/API、专属根/marker/锁、持久目标生成、执行器、调度、下载历史/租约或轮转；不创建 backups 目录。200/64/2048 容量与跨表归属/CAS/状态转换仍归后片服务，不能把字段/索引当作这些运行时功能已经交付。原同步 GET 导出继续使用 S03.1 共用服务与锁。

## 同一新产物与实际回归

所有命令均经 `python -B scripts/project_env.py -- ...`，缓存/临时目录受限于仓库 `.runtime/`，无用户环境或 `go env -w` 修改。

Windows 实测产物 `.runtime/bin/s03-2-1-implement/nas.exe`：

- **21,581,312 bytes**。
- **SHA256 `3e04aafb13da73b4928b0991e77941f6c8af04edc01f3022a87f7754e213a962`**。
- 下列 storage/backup/account/offline/core 结果均逐个读回、核对同一摘要；不存在以旧 S03.1 产物代替。

| 本轮门禁 | 结果 | 证据 |
| --- | --- | --- |
| 新 storage 稳定专项 | **17项通过**；3次正常停服另记，不混入17项 | `.runtime/verify/s0321-storage-implement-final/result.json`、`assertions.json` |
| 共用备份回归 | **38项通过** | `.runtime/verify/s0321-backup-implement/result.json` |
| 完整 `verify_account.py --offline` | **162 = 在线83 + 离线79**，不是只跑离线子脚本 | `.runtime/verify/s0321-account-implement/result.json`、`offline/result.json` |
| core `--large-file` | **52项通过**；5GiB稀疏HTTP偏移不计真实解码/大库性能 | `.runtime/verify/s0321-core-large-implement-final/run-manifest.json` |
| 临时 Go 行为 | **6个顶层组/202个子场景**通过；不把pass事件当独立断言数 | `.runtime/verify/s0321-implement-unit/result.json`、`go-behavior.jsonl` |
| gofmt / go vet / 空白 | 全部通过 | `.runtime/verify/s0321-final-checks/result.json`及日志 |
| 清源后 `go test ./...` | 全包编译退出0；均 `[no test files]`，**不算行为验证** | `s0321-final-checks/clean-source-compile.log` |
| Python AST/入参返回类型 | 新增/扩展脚本通过 | `s0321-final-checks/result.json` |
| Vue TypeScript | 实际 `npm --prefix web run typecheck` 退出0 | 本轮工具执行记录，结果同时记入 `s0321-final-checks/result.json` |

临时行为源为 `internal/app/backup_store_s0321_temp_test.go`，执行时 SHA256 `c9f1820512a916f6501f935dd80d7f0061bc104634f80279e07e820d99cb239b`。最终已精确删除，`temporaryTestSourceRemaining=0`。源码和构建产物摘要清单都在最终 checks JSON；没有删除稳定脚本或其他参与者文件。

### 实际覆盖的关键接缝

- 真v5→6保留账号hash、revision、会话；重启不重随机storeId；冲突表在部分DDL已执行后完整回滚到头5且原sentinel保留；未来7拒绝。
- 负时间、超过毫秒上限、熵EOF、熵不足实际经窄源执行并回滚；成功源只采一次时间、读取16字节，共用effective/updated。不是基于 `rand.Read` 不可返回的错误分支宣称通过。
- DDL覆盖STRICT类型拒绝、每类毫秒上下界、ID/摘要长度/大小写/Unicode/NUL、UUIDv7版本/variant/连字符、中文UTF-8字节预算、JSON畸形/形状、正seq、固定request过期、部分授权六种NULL组合、单active/单deleting。
- 状态覆盖queued禁file/started/finished、completed必须started、已发布tuple各字段缺失拒绝、未发布unverified与直接deleted合法、已发布missing和墙钟回拨合法。计划键和错误码字符集在有效来源/状态中单独检验，避免其它CHECK遮掩。
- checker当前真6通过；真正历史5、缺备份表、非STRICT、错误字段类型、缺策略/存储单例均实际拒绝。原独立只读校验/HTTP回归继续验证长度、页、完整性、外键、摘要与目录稳定。
- 离线5/6均覆盖成功、schema cookie/hash/revision漂移、取消；双向user_version漂移保持schema cookie不变仍拒绝。稳定真实CLI在专属Console第二提示形成预检barrier，模式恢复、秘密无回显、退出1、锁释放与账号零写均有记录。
- 真6非空备份域中的policy/store/run/file/key、轮转授权及pending哨兵，在成功reset前后全逻辑行/结构/文件摘要比较原样。真正5没有备份新表，成功不隐式迁移。原会话清空，新旧密码/Cookie的实际重启行为符合已有账号契约。

### 正常停服、秘密与进程收尾

最终 storage：5次实际启动，其中3次运行成功后以专属Console真实 `CTRL_C_EVENT` 正常返回0，2次是未来/冲突的自然拒绝退出；`normalStops=3`、`forcedApplicationReclaims=0`、`cleanupFailures=[]`、`rawLogsCheckedBeforeSanitize=true`。

最终 backup：3次真实正常停服、0次活应用强制回收，收尾错误空、原始日志先扫描再脱敏为true。它仍覆盖慢下载竞争409、取消/停服收尾及同目录立即重启。账号 Console/CLI的原始双流和可见屏幕同样先扫描，真实ACL拒绝/恢复成立；未将Job兜底当正常停服。

最终按确切本轮 Windows 产物绝对路径盘点，活应用进程数为0。私有 fixture/数据库/快照留在受限本地验证根，只分享安全JSON/日志，不整体上传含账号材料的目录。

## 交叉编译与未验边界

仅编译、没有Linux运行证据：

| 目标 | 产物 | bytes | SHA256 |
| --- | --- | ---: | --- |
| linux/amd64 | `.runtime/bin/s03-2-1-linux/nas-amd64` | 21,375,974 | `dc150b7ab9ad4d72a0fa7cc48d61e9e3028b576b6d8cc46807306c82558916a4` |
| linux/arm64 | `.runtime/bin/s03-2-1-linux/nas-arm64` | 20,062,945 | `fc204a912b7fac97bb8fe6f395814e60c08cbe23f4ef701085d2691995ec843d` |

真实Linux/NAS、FULL/IOERR、拔盘/断电、多卷身份/权限和大库性能继续待验。备份仍为database-only；含敏感账号/索引/私密引用的DB不是外部材料/媒体/卷原件备份，也不是异地灾备。历史5离线账号维护受支持，不表示当前6 snapshot checker支持历史5，更没有开放历史快照恢复准备入口。

后片S03.2.2再冻结受控根/平台发布，S03.2.3再做执行器与手动去重/取消，S03.2.4再做UTC调度/下载和授权轮转。完成各自门禁前不能开放enabled=true。

## 失败记录与修正

- 首次 storage 专项 `.runtime/verify/s0321-storage-implement/` 为 **failed**：比较原表行时，对 `sqlite_sequence` 的无序SELECT与排序后的逻辑快照直接比较，造成验证器假失败。定向逐表比对确认真实数据相同后，只修验证器排序；新根完整重放17项通过，产品Go及产物未因此改变。旧失败证据未覆盖：9项已完成、正常停服2、异常收尾时1次自有活应用强制回收。此1次不计入最终成功轮的0次，也不隐瞒为正常退出。
- 首次 core `.runtime/verify/s0321-core-large-implement/` 为 **blocked**：未传基线要求的显式FFmpeg路径，产品未运行、无通过断言。确认本机真实FFmpeg/ffprobe绝对路径后，在全新final根重跑52项通过；旧blocked清单保留。
- 临时Go早先一次工具输出被截断；最终实际 `-json` 完整日志与源摘要另存 `s0321-implement-unit`，以该轮6组/202子场景为准，不累加重叠运行。

实施者现已停写，待独立checker可写复核。正式结论、spec/docs和任务状态由主线在独立验收后更新。
