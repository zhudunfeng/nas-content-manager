# S02 账号回归接入基线

日期：2026-11-23。主线拥有本记录及编排改动，独立账号/browser复核另见对应报告。本记录不将S01历史成绩并入本次计数。

## 改动与证据范围

- `verify_baseline.py` 注册 `--cases account`，复用原 `verify_account.py --binary ... --root ...` 与既有超时、独占进程树、独立新根和结果校验，不复制账号验证逻辑。
- 账号证据类别为 `local-real-application` / `historical-schema-fixtures`，不虚构上游外部协议服务。具体在线API/离线CLI范围读取同次 `result.json`，不能由组合名推断。
- 帮助由 `CASES` 生成，文档矩阵和正式baseline spec同步。真实浏览器保持显式独立入口，不在普通基线隐式启动。

## 编排自检失败与诊断

1. `.runtime/verify/s02-2-account-orchestrator-selftest`：25项通过后，原“旁观进程持续运行”断言失败，整体exit1，完整清单保留。该断言只在固定0.2秒后读取一次心跳，原运行没有保存当次两个采样值/退出码，不能事后断言原失败唯一原因。
2. `.runtime/verify/s02-2-observer-diagnostic/result.json`：独立建立两个自有进程树，停止其中一个。12次读回中旁观者始终 `poll() is None` 且心跳持续变化，同时实测到两次空字符串；被停止后代心跳不再变化。证明 `write_text` 截断再写的瞬时空窗确实存在，固定单采样可以误判存活，不支持修改生产进程回收逻辑。
3. 最小修正仅在稳定自检增加 `wait_heartbeat`：有限预算内等待有效、非空、变化的数字心跳，再同时检查真实进程句柄存活。保留后代停止与整树回收断言。
4. `.runtime/verify/s02-2-account-orchestrator-selftest-2`：39项通过，exit0。再增加“空值不能算变化”“空值后延迟发布有效心跳”两项确定性回归，最终 `.runtime/verify/s02-2-account-orchestrator-selftest-3`：41项通过，exit0。历史失败不删除、不覆盖。

## 调试复盘

- 根因类别：E 隐含假设／D 验证缺口；把非原子文件改写当作始终可读的完整心跳，固定睡眠未覆盖瞬时空值与调度差异。
- 本次没有修改Windows Job或POSIX进程组，不通过放宽实际存活/停止语义换取通过；只将取样改为有界状态等待。
- 预防已落地：两项确定性稳定回归与 `.trellis/spec/backend/baseline-contract.md` 心跳规则。
- 本机Windows结果不证明Linux进程组或NAS环境。自检中的受控失败不记为产品通过。

## 账号组合实跑

实际命令：

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases account --artifacts .runtime/bin/s02-2-review --root .runtime/verify/s02-2-account-baseline --timeout 600
```

- `run-manifest.json`读回整体passed、finishedAt非空、account 75项，其余8个组合均not_run，不将未选组合算成通过。
- 产物为`.runtime/bin/s02-2-review/nas.exe`，21,334,016字节，SHA256 `2d8c2ed03b85e9b4c1ddbcca87cda119df44f2f17cab726d509abcf056d3873a`，与独立复核产物一致。
- 同次账号结果中在线API与历史库迁移已完成，offlineCLI仍false；该次结果不构成后续S02.3的验证证据。
- 主线另外目视检查最终浏览器产物的375明色与1440暗色顶部/底部裁剪拼图，以及两个当前会话确认框原图，未见标签、内边距或按钮裁切。该抽样不冒称逐像素审查20行会话的整张长页；全尺寸/主题DOM与键盘矩阵由独立浏览器报告记录。

## S02.3 完整账号组合接入准备

- 实施者固定账号验证入口：`verify_account.py` 默认只跑在线；显式 `--offline` 才继续执行同一产物的稳定离线验证。主线已令 baseline `account` 固定传该参数。
- 完整组合除原passed/正断言计数外，还要求在线API、真实schema4迁移与离线CLI标志为true，嵌套offlineVerification明确requested/passed，在线与离线计数均为正整数且和总数一致，嵌套离线计数也一致。漏跑/布尔数值/失配不能因进程exit0就通过。
- 证据类别增加`local-offline-maintenance`，不由运行平台或组合名称推断真实Console、Linux或NAS通过；browser仍为显式独立入口。
- `.runtime/verify/s02-3-account-coverage-selftest/run-manifest.json`已实际读回passed/43项/结束时间。新增两项为受控编排结果：捕获真实domain_case分派的`--offline`，以及13类缺失/错误覆盖拒绝；它们不算产品在线或离线执行。其余原41项真实进程/路径/清理/脱敏自检继续通过。
- S02.3真实CLI和完整账号组合仍待实施者交付及独立复核；本节不提前声明通过。后续在新根执行完整组合并追加最终产物与实际计数，不覆盖上面的S02.2历史记录。

## S02.3 account --offline 接入独立定向复核

2026-11-23。本小节由独立checker追加。只读核对主线编排接入，修复范围仅限授权的两个baseline脚本；账号业务实现者仍在写S02.3，本轮没有评审、修改或执行未交付的账号源码/CLI，也不把旧S02.2产物当作本轮完整账号验收。CodeGraph查询新增`require_account_coverage`未命中后，改读实际源码与相邻分派路径。

### 生产路径结论

- `domain_case` 对account确实固定追加一次`--offline`；保留`--binary`、隔离`--root`与既有超时/进程归属。离线不是由环境或默认行为猜测，独立account脚本默认在线的兼容性不被改变。
- 子进程成功仍须同时满足`passed is True`、`assertions`为严格正整数；新增账号门禁要求`accountMutationAPIs`、`trueSchema4Migration`、`offlineCLI`及嵌套`offlineVerification.requested/passed`均为真正的布尔true。
- `onlineAssertions`和`offlineAssertions`必须为严格正整数（不接受bool、float或字符串）；总数必须等于两者之和，嵌套离线计数必须是严格整数且精确匹配。缺失、false、非正值、类型别名与失配均失败，不因exit0拿到通过。
- 只有上述门禁成功后才发布账号成功证据与计数，分类精确为`local-real-application`、`historical-schema-fixtures`、`local-offline-maintenance`。这个分类不证明真实Console/Linux/NAS已执行，具体能力仍以同次子验证结果为准。
- 本轮未发现`verify_baseline.py`的确定缺陷，因此没有为展示修改而重写生产门禁；`docs/baseline.md`与baseline spec的参数和覆盖要求已匹配。

### 已修复的稳定自检缺口

File：`scripts/verify_baseline_selftest.py:verify_account_coverage_gate`。原完整正例经过真实`domain_case`，但13个负例只直接调用`require_account_coverage`。若未来分派漏调用该门禁，helper仍正确，原自检仍可能全绿。

本轮最小补强：

1. 72个缺失/错误覆盖结果全部经过真实`domain_case`与真实JSON读回，仅子命令执行被受控替换为exit0；覆盖顶层成功/标志/计数缺失、布尔假值和数值别名、0/负数/float/字符串计数、嵌套对象/字段缺失，以及总数/在线/离线/嵌套计数失配。
2. 每个拒绝还核对零通过计数、未登记成功证据、未产生成功文件，同时保留一次命令记录；所有73次分派（1正例+72反例）均精确包含一个`--offline`。
3. 完整正例输入和保存证据显式带`fixtureOnly`/`scope=controlled-orchestration-fixture`及未执行应用/接口/CLI/终端的限制。`controlled-account-accepted.json`保存分派分类与受控输入，`coverage-rejections.json`保存72/72拒绝摘要；不能拿这些文件冒充产品执行。

另用Python stdin临时patch生产`require_account_coverage`为空操作，刻意模拟“分派跳过门禁”，新回归准确失败。安全记录为`.runtime/verify/s02-3-account-gate-bypass-review/mutation-review.json`，`regressionDetected=true`、`applicationExecuted=false`。没有保留临时测试源码或把这一预期失败计为产品故障。

### 实际验证与证据

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --self-test --root .runtime/verify/s02-3-account-coverage-review-selftest-final
```

最终新根manifest已读回：`status=passed`、有结束时间、**44项**，分类仍为`orchestrator-integration-selftest`，限制明确“不代表产品功能或真实NAS通过”。其中41项为原有进程/路径/清理/脱敏自检，3项为本轮受控账号编排检查；72个反例包含在第3个聚合断言中，不能重复相加成116项。此前首轮`.runtime/verify/s02-3-account-coverage-review-selftest`同样44项通过，保留不覆盖。

- Lint/静态：局部`git diff --check`通过；两个Python脚本AST、导入和运行时类型注解解析通过。未发现独立Python lint/mypy配置，不能将AST等同全量静态类型检查。
- TypeCheck：本轮不改Go/TypeScript；未在账号源码并发施工时重跑其构建或web检查，相关门禁由完整交付复核执行。
- Tests：编排CLI自检44项通过；门禁绕过变异实验被准确检出。实际账号CLI/完整baseline仍待实现者交付后以fresh产物、新根执行，不能用本自检替代。

### S03 info 对设计的只读核对

`11-23-nas-backup-recovery/info.md`与`research/snapshot-service-design.md`未见S03.1实质范围漂移：都限定无迁移/新API/配置/UI/调度器，保留旧GET及HEAD/错误映射，唯一锁覆盖生成至下载清理，候选租约独占所有权，只读校验/流式摘要，`database-only`，持久发布与恢复准备留后续切片。真实Linux/NAS/ACL/大库性能不提前算通过。

有一处后续交接措辞建议已发主线，未自行改S03文件：info的“后续迁移必须同步复核维护器和稳定验证”比设计第7节的可交付要求略弱。建议明确“若S03.2引入新schema，须同片更新离线账号维护的显式支持集、结构探针及稳定验证，证明新增结构不改变账号写入前提，避免升级后恢复入口不可用”。S03.1无迁移，该建议不阻塞当前切片。

本轮到此停写并交回；没有改正式spec、账号业务、S03设计、其它报告或历史成绩，没有提交或部署。

## S02.3 主线最终真实编排验收

2026-11-23。独立checker完成账号修复及停写后，主线执行最终产物真实编排：

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases account --artifacts .runtime/bin/s02-3-review-http-final --root .runtime/verify/s02-3-baseline-account-review-http-final --timeout 900
```

- 进程正常exit0，manifest为passed且有finishedAt；account子命令精确一次`--offline`、exit0、无超时。其余8组not_run。
- 实际顶层账号157=在线83+离线74，嵌套离线requested/passed为true且计数74。realApplication/accountMutationAPIs/trueSchema4Migration/offlineCLI均true；离线evidence中的realCLI/rawOutputSecretScan/canonicalAliasLock/noRecoverySideEffects/restartCredentials均true。
- 清单、实际子结果、离线子结果与独立core清单均绑定21,447,680字节、SHA256 `8bc6a1a144d86bd0a65e38bafc9f1388b21bf2545c9a29168d92ebdf0dbf135a`，主线重新计算磁盘产物摘要一致。
- 同次离线Windows Console/ACL事实以其result为准；LinuxActual/targetNAS不因此变成通过。真实目标环境待验，重复账号157不与独立157相加。
- 正式account/runtime规范、API/账号说明、verification、S02/父路线任务状态已同步并定向读回；12子任务父子关系、JSONL引用、相对文档链接/空白/冲突标记检查通过。CodeGraph同步成功，无本轮临时Go测试源。

该结果完成S02本机最终门禁，按`local_complete_target_pending`交接S03.1，未提交、部署或归档。
