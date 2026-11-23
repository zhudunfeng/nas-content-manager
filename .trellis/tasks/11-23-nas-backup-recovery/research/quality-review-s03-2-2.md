# S03.2.2 独立可写质量复核

日期：2026-11-23。范围：Trellis Phase 2.2，仅复核冻结的受控根、严格元数据及平台发布。实施者已停写；本轮独占必要产品修复、独立临时验证和本报告。主线独占正式 spec/docs/task；S03.2.3 仍未冻结/激活。本轮未提交、部署、归档、操作 Docker 或用户实例，也未开放 API、enabled=true、执行器、调度或轮转。

本轮结论：修复两处确定缺陷后，本机 Windows 行为与同产物四门禁通过，清源/进程/193 项产品源码漂移均为零，可交主线验收。Linux 两架构仅交叉编译；真实 Linux/NAS、介质故障、系统崩溃与断电仍待验，不能据此将 S03 整体记为完成。

## Findings (fixed)

1. File：`internal/app/backup_storage_windows.go`，`backupPathSyntax`。
   Issue：保留设备名检测直接取第一个点前的主干，没有剔除扩展名前的 ASCII 空格。冻结规则要求拒绝的 `CON .db`、`COM¹  .db`、`LPT9 .db` 等被静态配置路径接受；嵌套祖先分段同样存在漏检。
   Fix：只对设备名主干执行 `strings.TrimRight(..., " ")` 后再做原有大写/设备名比较，保留其它路径规则。新增中文关键行说明。独立 red 轮确实得到 32 个失败叶子、4 个正常名称正例通过；修复后这 36 叶子全部通过。该结论是冻结语法规则回归，不声称在本机创建或操作过保留设备对象，也不把 `COM10 .db` 等普通主干扩大成保留名。
2. File：`internal/app/backup_storage_linux.go`，`backupOpenAnchor`。
   Issue：第一次 `file.Stat()` 失败时执行 `_ = file.Close()`，丢弃必要 anchor 句柄的关闭错误，与本片错误合并契约不符。
   Fix：返回 `errors.Join(err, file.Close())`，保持原打开/目录重开顺序与上层受控分类。该修复为静态代码路径核对，最终 Linux amd64/arm64 交编通过；本机没有 Linux Stat/Close 故障动态证据，未将 Windows seam 测试冒称 Linux 路径实测。

本轮只改以上两个产品文件。稳定验证脚本没有新增或改写；四份自有临时 Go 源执行后精确清源并保存 `.go.txt`。正式规范由主线同步，本报告不改写冻结范围或既有历史证据。

## Findings (not fixed)

没有本片范围内尚未修复的确定阻断。以下为已冻结边界或后片责任，继续明确保留：

- publisher 的容量预检发生在源快照已经存在之后，验证 `MinFreeBytes + 目标源尺寸 + 1MiB` 事后余量，以及单份 `DB+owner+manifest` 的 int64 有界预算。源生成前同卷/异卷峰值准入由 S03.2.3 衔接，不能把本轮结果宣传成已完成源前 3S 峰值准入或预留磁盘空间。
- `SYSTEM` 与当前用户同 SID 的唯一 ACE 分支使用真实 SID/ACL 对象调用校验函数；不是以 SYSTEM 身份运行产品。junction 为真实本机夹具；无文件 symlink 权限的场景不记 symlink 产品通过。
- 本片 marker 跨重启提供逻辑命名空间而非物理卷持久证明；Windows 文件 flush/写穿移动/后验复验不外推为掉电持久性保证。未知材料、初始化 tmp 残留保守保留，后续原 ID 对账由下一片负责。

## Verification

- Lint：pass，9 个本片 Go 文件 `gofmt -l` 无输出，`go vet ./...` 通过，`git diff --check` 通过。
- TypeCheck：pass，清源 `go test ./...` 包编译、Vue `typecheck` 及 `build` 通过；22 个稳定 Python 脚本 AST/方法入参与返回注解检查通过。
- Tests：pass，独立最终 24 个顶层测试、257 个通过叶子子场景、0 个嵌套容器、0 skip；另外 5 个无子测试独立顶层单列，不混入 257 叶子数。同一最终 Windows 产物 storage 17 / backup 38 / account 162=83+79 / core 52 通过。

### 权威与实际代码核对

已读取当前 task 的 prd/info/check 上下文与关联规范，遵循 `trellis-check`；本片冻结权威为 `root-publisher-contract.md`，SHA256 `b4c3f3aab47babd9851721ba2eab79376a92b7cc1e2c8248105e6345bebd93e4`。独立设计输入 `research/root-publisher-contract-review.md` SHA256 `2dc04d3b45100e0342838c07d71834d5a16d13d4948ae55076e94552b34670b6`；实施者交接 `implementation-s03-2-2.md` SHA256 `2a1271b4e5d73af432449abb0f8b8c9484696132d084f542fd685c33f8f32870`。研究和实施者自查只作为输入，不与本轮通过数累加。

先用 CodeGraph CLI 定位 root/publisher 接缝，再完整读回本片 9 文件及唯一快照/校验/锁的相关实际调用链。最终产品写入只涉及上述两处；主线在全面停写后统一 sync 索引。本轮没有激活下一片草稿。

已核：配置同 source 快照读取且 disabled 正常启动不探/建根；DataDir 精确 backups 例外不豁免其它树；保护树、精确文件点、缺失尾部和原父链 anchor 分别处理；历史 local_root 去重 256/257 有界拒绝；固定空锁不 unlink，Windows 用专用零数据访问 Stat 核对零共享锁实体；根/父/guards/marker 变化使租约 sticky blocked。

已核：已有合法 marker 根只读回权限，不重设继承 DACL；未知子树不能批量收紧。Go 可将 junction 标成 ModeIrregular，实际解析由内核跟随句柄并交叉核对原路径/父 anchor，不能只信 EvalSymlinks。父 junction 换到同 marker 替代根后拒绝，恢复旧映射仍 blocked，原物理根锁仍独占。

已核：marker/owner/manifest 的固定字段、解码后重复键、整数词法、UTF-8/字节上限、原始摘要和 n+1 有界 ReadDir；唯一 App 快照借用固定 64KiB 复制，目标独立读取实际 schema/size/hash，源不 Close/重开/转属。publisher 只读 running/pending 意图，不写 completed、last-good 或轮转授权；三类确定性与错误同时消费，最终移动一经尝试即保守保留原 ID 材料，不删除 final 或更换 ID。

### 独立行为场景与计数

证据根：`.runtime/verify/s0322-review-behavior-final/`，实际开始 UTC `2026-11-23T06:05:45.664895+00:00`，结束 `2026-11-23T06:06:14.115372+00:00`。原始事件 `events.jsonl` SHA256 `767609f5656ba1c6fd90ec53f2dd5ede9a429b36189d210ad4825ae82c02ca5b`。`result.json` 保存执行时四份临时源 SHA 与 193 项产品源 SHA。

回归基线参考实施者已保存 txt，全部使用本轮独立 `s0322Review` / `TestS0322Review` 前缀和全新根重新执行；以下 47 个新增叶子及 2 个新增无子测试顶层由 checker 独立补充。不能将复用回归说成 257 个全新独立设计的反例，也不能与旧轮通过数相加。

| 分组 | 通过叶子 |
| --- | ---: |
| Budgets / StrictFormats | 11 / 67 |
| RootModes / Isolation / StickyIdentity | 14 / 18 / 4 |
| PublisherFaults / Collisions / IntentChecks | 25 / 4 / 7 |
| Configuration / MemberBudget / FormatEdges | 5 / 4 / 15 |
| WindowsPathSyntax / WindowsDACL / WindowsAliases | 23 / 8 / 3 |
| WindowsUnknownACLPreserved | 2 |
| 新增 ReservedStemSpaces | 36 |
| 新增 DeepUnknownACL | 2 |
| 新增 CloseEveryResource | 5 |
| 新增 IndependentPublisher | 4 |
| 合计 | 257 |

24 个顶层中 19 个是容纳上述叶子的分组；5 个无子测试顶层为 `ProductPublisher`、`WindowsHandleCollision`、`WindowsRootMappingSwap`、新增 `LockZeroData`、新增 `OwnerLast`。嵌套容器为 0，全部真实事件重新计算，未将分组 pass 或普通输出行当成断言。

新增反例细节：设备主干 16 种 × 1/2 ASCII 空格，加 4 个合法主干正例，共 36；每个设备场景同时核叶端及祖先分段。未知树每路 4 分支 × 6 层、24 文件，含树根共 49 节点；从自有外部祖先继承未 protected DACL，同卷移入合法根或预先不合格根，两次 acquire 前后逐节点核内容/DACL 摘要与根 DACL 不变。逐 directory/anchor/parent/guard/lock 注入“实际已 Close 后报错”，核每资源都关闭、错误幂等、锁最后释放且可重新取锁。

新增真实零数据 Stat 核同一空普通锁实体，普通数据访问打开被零共享锁拒绝。新增目标 SQLite 头破坏拒绝；最终移动后新增未知成员、最终目标 Close 报错都保持 needs_reconcile 与同一 running/pending/fileId，重复调用不换 ID。源 Close 故障单独记录，不抹 confirmed 目标。新增 OwnerLast 通过真实非删除共享 owner 句柄阻止末项删除，证明 DB 已先清、仅 owner 原字节和候选空壳保留；未按前缀或 RemoveAll 清理。

### 失败历史保留

- `.runtime/verify/s0322-review-reserved-red/`：修复前原始事件，32 个语法叶子失败、4 个普通名称正例通过；事件 SHA256 `264680ee6ca98f4111a158576a298d3a20016ee580528a30fe7f8d4f8d9f719c`。未覆盖、删除或把它计为产品通过。
- `.runtime/verify/s0322-review-behavior-1/`：修复后的第一全量轮有 252 通过叶子、1 个夹具失败。原因是 checker 自己把未知树移入后去掉根继承 ACE，夹具设置过程传播到后代，后代 DACL 因而无法读取，尚未进入待检 acquire。已改为先设置不合格根、再同卷移入未知树；不是据此再改产品，也未把夹具失败抹成产品通过。事件 SHA256 `36747d2546b9f7aac26b5aa11f1588a669eccd66ef381166a967290bb433eec4`。
- `.runtime/verify/s0322-review-behavior-2/`：23 顶层/257 叶子通过，之后仅加 OwnerLast 再跑最终轮；事件 SHA256 `2d97369308923c3fbcfa470cc0be126f6e806f01e4f559889a4c6f47e07f6d81`。与最终轮不累加。
- 工程 runner 准备时一次 PowerShell 内联字符串转义失败，命令在生成文件前退出；改为显式 `.runtime` Python 准备脚本。不是产品、行为测试或最终工程门禁失败，没有更改用户级环境或放宽检查。
- 实施者此前编译/API、零共享 Stat、DACL 规范化、同卷未知 ACL 与 junction 解析等失败原证据继续保留在其交接报告和各旧根。本轮不覆盖它们，也不将实施者最终 210 叶子另加到本轮 257。

### 清源、工程门禁与产物

工程证据 `.runtime/verify/s0322-review-checks-final/result.json` SHA256 `389d78a555211ca74672015391906dfe0d6a40c53e76318a2ff03a3a25f8f138`，UTC `2026-11-23T06:06:32.147524+00:00` 至 `2026-11-23T06:07:38.757680+00:00`。先核最终行为源与产品 SHA，四份测试只在 SHA 精确一致后另存 txt 并 unlink；再执行清源工程与 fresh 构建。

完整 193 项产品源码 SHA 清单为 `.runtime/verify/s0322-review-checks-final/product-sources.sha256`，193 行，SHA256 `0dd6d62aa89118b7e40e6291b23f38d9ea95780eb220f01dac2471734c08d199`；同一清单亦保存在行为及工程 result 的 `productSHA256s`。最终两处产品文件分别为 Windows `f5fe55a90ada3644bc936d8b9ee96b1a90b3ce7e5af1242e4d5358d24e54a594`、Linux `e79cc26ecedb903f52cb613bc55ebe912f5e58bec5966cebaebe90d35b3512b5`。

| fresh 产物（均在 `.runtime/bin/s0322-review-final/`） | 字节 | SHA256 | 实际验证 |
| --- | ---: | --- | --- |
| nas.exe，windows/amd64 | 21,593,600 | `9839729e64f80e052f6e5acd3114e251f8fa5fbffad8276753d2a15148a43137` | 下列同产物四门禁；私有行为另由 Go test 执行 |
| nas-linux-amd64 | 21,379,424 | `c2fda79fb9f8a0715db24acaac047951fbe2400538ec67a43918d679a4a11752` | 仅 CGO_ENABLED=0 交叉构建 |
| nas-linux-arm64 | 20,067,563 | `c76435628240db221a1511cfe23dd53f8c485bc7cae2e9bcc2ab9edb95929486` | 仅 CGO_ENABLED=0 交叉构建 |

上述行为测试由 Go test 临时测试进程执行，不把 nas.exe 当作私有函数的调用进程；其 193 源清单与 fresh 构建及四门禁保持一致。所有本地命令经 `python -B scripts/project_env.py -- ...`；Go/gofmt 固定 `.tools/go/bin/`，缓存/临时工具仅 `.runtime`，没有用户级环境或 `go env -w`。

四份保存于 `.runtime/verify/s0322-review-checks-final/removed-test-sources/`：`backup_s0322_review_temp_test.go.txt` SHA `c7883a9754f6907e0390a958fe13916bac4e976fdc06ae36423635740e26eca9`；`backup_s0322_review_temp_windows_test.go.txt` SHA `f2db38da6201df1023c5b3795bb918b26f019cb99b2b427dca711830013f2415`；`backup_s0322_review_extra_test.go.txt` SHA `94a57414363f4c4a7623808aa496d9389e694151a78a93f7fbc7fce04341df5a`；`backup_s0322_review_extra_windows_test.go.txt` SHA `c62e83661337f0c9a5ad9ec41c5be8561430e5fc3e402a3b3922a79a6fe06dde`。清理清单为 `removed-test-sources.json`，其它人的源与旧证据未删。

### 同一 Windows 产物的隔离回归

| 全新证据根（`.runtime/verify/` 下） | 结果 | 结果文件 SHA256 |
| --- | --- | --- |
| s0322-review-storage-final | 17，通过 | `c9193258169804b87ab722d9b7537f687e39daaf41bda730c04dbb9edaf97ba2` |
| s0322-review-backup-final | 38，通过 | `1343313e3d000b20b1deb4c4aa7131574609e826c397c359dc3ebbd43225269a` |
| s0322-review-account-final | 162=83 在线+79 离线，通过 | `f3593e212cfbcf13d6425318018a14b4d7c67a41a5693541b1f37e60db58b33f` |
| s0322-review-core-final | 52，通过，含 5GiB 稀疏 HTTP 偏移 | `1a896c64dc31c803022e9eabd2a943fbab2445b16d7cd03fd3e25bd978226ecf` |

前三行为 `result.json`，core 为 `run-manifest.json`；四份产物 bytes/SHA 完全相同。account 显式 `--offline`，独立读回其 `offline/result.json` passed/79/同 SHA；不是仅依据在线总数。13 条真实 Windows Console 证据的原始流与可见 viewport 秘密扫描均为 true，真实 ACL 拒写/退出1/原样恢复成立；文件 symlink 无权限，离线链接夹具为 directory-junction，未声称 file-symlink 产品通过。

storage 与 backup 合计 6 次真实隔离 Console CTRL_C_EVENT 正常停服，0 次应用强制回收，cleanupFailures 均空；backup 的慢下载取消、锁/DB/监听释放和立即重启均核实。core 独占服务 `cleanup=owned-tree-reclaimed`、服务退出1，只记 Job 所有树回收，不能冒充正常停服或进程崩溃恢复验证。真实 FFmpeg/ffprobe 使用显式绝对路径；稀疏文件只证明 HTTP 偏移，非 5GiB 解码/大库性能。

### 秘密、最终收尾与待验账

行为与工程 runner 在内存中检查原始输出的 bcrypt/Authorization Bearer/nas_session 标记后才保存事件或脱敏日志；stable storage/backup 的 `rawLogsCheckedBeforeSanitize=true` 已读回。离线账号验证另以其实际口令/Console 双流与可见 viewport 证据为准。在线账号脚本公开脱敏日志自身不能证明全部未知秘密串都不存在，本轮不扩大该声明；fixture DB 与材料仍是本地私密证据，不宜整体上传。

最终审计 `.runtime/verify/s0322-review-checks-final/final-audit.json` 的 actual UTC 为 `2026-11-23T06:10:08.902760+00:00`：passed=true；原始事件重新计数、三产物 bytes/SHA、四回归产物、193 源清单和 4 份 txt/删除事实都独立读回。只以本轮确切 nas.exe 绝对路径过滤 Win32_Process，活进程为 0；`internal/app/*s0322*test.go` 为 0；193 项 sourceDrift 为 0。没有按进程名/端口结束任何进程，也没有触及用户实例。

分账结果：真实本机 Windows NTFS/ACL/junction/零共享锁/写穿发布/同产物回归已验；Linux amd64/arm64 只交编；真实 Linux ext4/XFS（含 bind、renameat2、fsync、flock、权限/终端）未验；真实 NAS/多卷/远程介质/FULL/拔盘/真实大库性能未验；进程崩溃恢复未验；系统崩溃未验；断电未验。后四者不能用故障 seam 或 core Job 回收替代。

本报告写完后全文读回并核 SHA，随后停止产品、测试、报告写入，交回主线完成正式规范与证据验收；当前不激活 S03.2.3。
