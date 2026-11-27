# S03.2.1 独立可写复核与最终本机证据

2026-11-23。本报告由独立 checker 在实施者明确停写后完成，依据 `storage-schema6-contract.md`、`info.md`、`check.jsonl` 及实际产品调用链。独立检查发现并修复一个快照结构探针缺口，其余本片已冻结的存储/维护兼容门禁通过。本片可按 `local_complete_target_pending` 交回主线核定；S03整体尚未完成，不表示后片根、发布、执行器、调度或轮转已交付。

本轮只有 `internal/app/backup_verify.go` 的产品修复，以及本报告；独立临时 Go 单测已执行后精确清源，运行工具/失败与成功证据只保留在 `.runtime/`。未修改正式 spec/docs、PRD/info、root草案或旧 `quality-review-s03-1.md`，未提交、部署、归档或接触用户实例。正式文档及任务状态由主线同步。

## Findings (fixed)

- File: `internal/app/backup_verify.go`，`checkSnapshotBackupStructure`。
- Issue: 原探针用 `pragma_table_info` 检查五表字段，而该 PRAGMA 会省略生成隐藏列。独立构造真实历史v5、附加五张当前备份表，并在 `backup_request_keys` 加 `hidden_probe TEXT GENERATED ALWAYS AS('x') VIRTUAL`、设置头6后，实际 `verifyDatabaseSnapshot` 错误接受，虽其结构不是冻结schema6。
- Fix: 改为读取 `pragma_table_xinfo(?)` 的 `name,type,hidden`，要求每一列 `hidden==0`、名称/类型匹配且实际列数精确一致；保留迭代、Scan与Close错误拒绝路径。增加中文关键行注释，不扩大探针为完整DDL语义证明。
- Regression: 首轮实际失败与完整Go事件保留在 `.runtime/verify/s0321-review-behavior-1/`；修复后全新 `.runtime/verify/s0321-review-behavior-final/` 的同一反例被拒绝，正常真6仍通过，校验前后字节/mtime/目录成员不变。

## Findings (not fixed)

没有尚未修复的本片确定产品缺陷。以下为冻结范围外或尚无目标实测的边界，不能写成通过：

- 当前快照checker只支持当前头6；离线账号维护显式支持集合 `{5,6}`，两个入口的支持范围不同。没有新增公开历史快照verify、恢复准备或 `snapshotExpected.Schema`。
- 探针确认核心表、STRICT、全部xinfo列/类型/hidden=0和单例；不能等价证明任意伪造库的全部CHECK、索引、默认值或跨表业务语义。
- 无备份根/marker/锁、配置/API、持久目标生成、执行器、调度、历史裁剪、下载历史/租约或轮转实现，不开放策略启用。200运行/64非deleted所有权/2048有效键及跨表归属仍由后片服务负责。
- Linux两架构仅交叉构建，真实Linux/NAS、目标ACL/锁/终端、FULL/IOERR、拔盘/断电、多卷以及生产大库性能仍待验。5GiB稀疏夹具只证明HTTP偏移。
- 本轮以注入 `io.Reader` 的EOF/short证明可返回源错误会回滚。当前固定Go运行时的默认 `crypto/rand.Reader` 底层熵灾难可能不可恢复地fatal；没有承诺该类生产失败受控返回，也未动态注入进程崩溃/掉电来证明事务恢复。无需为此扩造OS随机源、改依赖或修改全局随机。

## Verification

全部本地命令通过 `python -B scripts/project_env.py -- ...` 运行，缓存/临时目录限仓库 `.runtime/`。没有修改用户环境或使用 `go env -w`。按项目要求，先用CodeGraph CLI查询迁移/版本支持/调用链，修复清源后再实际sync；最后一次仅同步本产品修复文件，退出0。

### 独立Go行为与真实计数

最终证据 `.runtime/verify/s0321-review-behavior-final/result.json`、`go-test.jsonl` 和 `count-audit.json`：

- **8个顶层测试组通过、653个叶子子场景通过、0失败。** 原始带斜线的pass事件为658，其中5个是有下级测试的嵌套容器，另行列账，不把容器或任意pass事件当独立断言。
- 实际行为结束：`2026-11-23T05:02:47.809033+00:00`；`rawSecretScanPassed=true`。原始输出在内存扫描夹具密码及bcrypt前缀后才脱敏保存，失败也不打印SQLite诊断、账号值或私有路径。
- 执行时临时源 `internal/app/backup_store_s0321_review_temp_test.go` SHA256：`c282b5b473175bd9a3be61eba6b6d6f663a6a31dd1fcb52ede41d34e95e24711`。已按此摘要核对后精确删除；最终同片临时测试glob为0，不删除其他参与者文件。
- 真实0至5固定历史迁移链、账号/会话/业务行保留、默认关闭、无持久记录/备份根、重启policy/store稳定。
- 五个不同表冲突（含最后一张表的晚期冲突）全回滚、旧sentinel及头5保留；非法/边界时钟、短/空可返回熵、取消、一次采时和同值初始化。
- 全五表实际SQLite的STRICT/NOT NULL、数值上下界、各毫秒边界、UTF-8字节预算、ID/摘要的大小写/非hex/Unicode/NUL、UUIDv7版本/variant/位置、JSON合法性/形状/字节、授权8种NULL组合、单active和单deleting。
- manual/scheduled/非法trigger的16种来源NULL组合；6种run状态的started/finished/file/error共16种组合；6种文件状态及非法状态、deleted_at边界、发布tuple逐项缺失；故障未发布和已发布状态均有可接受反例。允许墙钟回拨，不虚增时间顺序CHECK。
- 真6、真历史5、未来7、真5只改头的明确伪6、非STRICT、错类型、缺字段、缺policy/store单例及隐藏生成列；使用中文/空格/#/%路径，实际只读前后不变。
- 真5/正常迁移6 × 成功、双向user_version漂移且schema cookie不变、cookie/hash/revision漂移和取消；成功只改账号事实，五表/授权/ownership、业务行和pending哨兵保持。

### 最终同一新Windows产物

路径：`.runtime/bin/s0321-review-final-pass/nas.exe`；**21,581,824 bytes**；SHA256 **`18a0a77fe0c10256e7eb406d0b07f64210901b8320012a78f2b02fc810a188cd`**。

四个门禁及nested offline均逐个读回，同一产物摘要和长度一致，不复用实施者或上片数字：

| 实际门禁 | 结果 | 最终证据根 |
| --- | --- | --- |
| storage专项 | **17项断言通过**；3次正常停服另计 | `.runtime/verify/s0321-review-storage-final/` |
| 共用backup回归 | **38项断言通过** | `.runtime/verify/s0321-review-backup-final/` |
| 完整account `--offline` | **162 = 在线83 + 离线79** | `.runtime/verify/s0321-review-account-final/`，含 `offline/result.json` |
| core `--large-file` | **52项断言通过** | `.runtime/verify/s0321-review-core-final/run-manifest.json` |

storage实际5次启动：3次正常运行后专属Console `CTRL_C_EVENT` 退出0，另2次为future7/冲突的自然拒绝；`normalStops=3`、`forcedApplicationReclaims=0`、`cleanupFailures=[]`、`rawLogsCheckedBeforeSanitize=true`。真实v5保留旧Cookie和全部旧行，重启不随机化storeId，默认关闭不建备份根，不清未知pending。

backup实际3次真实正常停服、0次活应用强制回收、收尾错误为空，原始日志先扫描再脱敏为true；覆盖实际慢HTTP竞争409、取消/停服清理、DB/锁释放与同根立即重启。下载两个快照均schema6、17,244,160 bytes，摘要分别为 `0a51287840b0e6418627fc6c0615aac39fc08038865183cd13b318132976582a`、`95384bd3087619987fd2c61aaf58b0921e35419f2073fb43cad55c59fb077348`。

account完整跑实际在线与离线组合。离线原始双流扫描为true，13条真实Console记录的原始和可见屏幕扫描均通过；双向版本漂移在第二提示barrier后、SQLite schema cookie不变仍拒绝；真实写ACL拒绝及完整恢复成立。线上可分享日志的脱敏检查不扩大为所有在线原始流的秘密扫描证明。

core显式使用本机真实FFmpeg/ffprobe；manifest结束 `2026-11-23T05:06:08.806+00:00`。其应用退出1且 `cleanup=owned-tree-reclaimed`，是OwnedProcess/Job回收，**不计入上述6次正常停服，也不把该强制机制写成0次**。账号/其他OwnedProcess同样不借未列正常停服数扩大声明。

### 工程门禁与清源

`.runtime/verify/s0321-review-checks-final-pass/result.json`、各同名日志：

- Lint：gofmt、`go vet ./...`、`git diff --check`均通过。
- TypeCheck：实际 `npm --prefix web run typecheck` 退出0；本片13文件中所有Python脚本AST解析及方法入参/返回标注核对通过，无缺漏。
- 清源后 `go test ./...` 全包编译退出0，均为 `[no test files]`；只算编译，不计行为通过。
- 源码/产物摘要清单与执行开始结束时间在JSON；工程门禁实际结束 `2026-11-23T05:04:39.954017+00:00`。
- `final-audit.json` 实际收尾 `finishedAt=2026-11-23T05:07:11.046614+00:00`；最终源与构建时清单漂移 `[]`，本轮临时源 `[]`，两个本轮Windows构建的确切绝对路径活应用进程均为0。
- storage/backup/account的result本身没有 `finishedAt`，未从mtime补造字段；上述审计时间只代表实际总收尾。

四个产品源码SHA256（其余脚本共13条完整列表见checks JSON）：

| 源文件 | SHA256 |
| --- | --- |
| `internal/app/backup_store.go` | `cdcef02c0eb76dcc6d9329c249789428482484aba4a31f3ac550f99dfc1756df` |
| `internal/app/store.go` | `9219e26617b46ee2518acb0943edf3d31dac0942328f385950714b2298ea9464` |
| `internal/app/account_offline_store.go` | `c04c4870141e5b117d522d0f71a218ef73309f96bbdafd7ed645337b289d73e0` |
| `internal/app/backup_verify.go` | `6461d38b1527cc2baee0cab97bf112af8a4940a2b9c9f326bc7182276e58fc0c` |

### 新鲜交叉构建，不计目标实测

| 目标 | 产物（均在 `.runtime/bin/s0321-review-final-pass/`） | bytes | SHA256 |
| --- | --- | ---: | --- |
| windows/amd64（实际集成运行） | `nas.exe` | 21,581,824 | `18a0a77fe0c10256e7eb406d0b07f64210901b8320012a78f2b02fc810a188cd` |
| linux/amd64（仅编译） | `nas-linux-amd64` | 21,377,958 | `80018064e68585b27b6cfc1421dc506b4220dfcb0a2028da3e9b7e9ec5fb7ba1` |
| linux/arm64（仅编译） | `nas-linux-arm64` | 20,063,537 | `ac22498c66801c37d509c341d8cc15b667dde1829a4b10b0499f0289ca1caeb7` |

## 失败证据单列，不覆盖、不累加

1. 独立Go首轮 `.runtime/verify/s0321-review-behavior-1/` 为failed。完整事件与源SHA `1478da05e30decc6038e64395ad0325416e2f3af728994e5c707734c308c930f` 保留；**6顶层通过、343带斜线通过事件=338叶子+5容器**，另 `extra-hidden` 叶子及所属顶层失败，绝不计为通过。实际结束 `2026-11-23T04:58:48.864174+00:00`。修复产品并扩展矩阵后在全新final根重跑；两轮不重复累加。
2. 独立工程收集首轮 `.runtime/verify/s0321-review-checks-final/` 为failed：自有runtime工具把 `git diff --check` 的退出0、仅stderr的LF/CRLF提示误算非空失败。核对原日志后只改runtime判定为检查stdout，保留首轮，不修改仓库换行或Git设置；全新checks/artifacts根重跑所有工程项及三平台构建通过。两个Windows构建摘要相同；正式四专项只使用 `final-pass` 根。
3. 实施者历史 `.runtime/verify/s0321-storage-implement/` 为failed，由 `sqlite_sequence` 无序行比较造成验证器假失败；9项已完成、2正常停服、异常收尾 **1次自有活应用强制回收**。实施者只修排序后全新根17项通过，但本独立最终数字不累加该轮，不隐藏那1次回收。
4. 实施者历史 `.runtime/verify/s0321-core-large-implement/` 为blocked：缺显式FFmpeg参数，产品未运行、0断言。其后自查成功根与本独立新根分开，绝不把blocked写成通过。

本轮自有私密库/会话/快照保留在受限本地验证根，不整体上传。备份仍为database-only，数据库含敏感账号、索引及私密引用，不能代表外部秘密、helper/worker身份、组件卷、媒体/下载载荷、用户原件或异地灾备。本独立checker现停写交回；主线可据本报告读回证据后同步正式契约并冻结S03.2.2，不能跳过后片门禁启用策略。
