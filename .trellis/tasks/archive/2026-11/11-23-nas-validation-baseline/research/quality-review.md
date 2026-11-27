# S01 定向质量复核

日期：2026-11-23。复核对象为两份新增 baseline 脚本、档案模板、操作说明和对应规范；未改业务 API、数据库或原验证脚本。先只读复核并将问题交实施者修复，明确移交文件后才修改收尾逻辑，避免并发覆盖。

## Findings (fixed)

1. **清理错误传播与命令证据。**
   - File: `scripts/verify_baseline.py`
   - Issue: 一度把 `startBlocked/cleanupError/error` 判定追加在主入口退出后，代码不可达；此前直接抛清理错误也可能使调用者丢失已执行命令记录。
   - Fix: 命令层总是返回退出码、日志和独立错误字段；`require_command()` 按清理失败、无法安全启动、执行错误、超时/退出码顺序处理。真实清理后注入失败时，子命令 exit0 不再使对应项通过，记录仍完整。

2. **脱敏范围与 JSON 完整性。**
   - File: `scripts/verify_baseline.py`、`scripts/verify_baseline_selftest.py`
   - Issue: 原规则遗漏其他 POSIX 根、带空格路径与 UNC 共享路径；对已序列化 JSON 作正则替换可能破坏结构。
   - Fix: 扩展路径遮蔽并递归处理结构化值后再序列化。已实际复核凭据、私密 URL、`/srv`、`/root`、`/etc`、Windows 空格路径、反斜杠 UNC 与正斜杠共享路径；嵌套 JSON 脱敏后仍可解析。

3. **运行根别名和二次日志遍历。**
   - File: `scripts/verify_baseline.py`
   - Issue: Windows 尾随点/空格、设备名、ADS 等路径别名可能绕过字面路径判断；递归日志遍历不能假定初始根安全就可以穿过后代 junction。
   - Fix: 创建前拒绝危险分量并确认规范父路径；`safe_files()` 逐级剪枝符号链接与重解析目录。受控 junction 指向的根外日志摘要保持不变。

4. **真实自检入口的前置分类。**
   - File: `scripts/verify_baseline.py`、`scripts/verify_baseline_selftest.py`
   - Issue: 自检根创建异常在普通入口捕获范围之外；反向导入还曾让 `__main__` 与模块中的 `Blocked` 类型不一致，仅测帮助函数不能发现这些问题。
   - Fix: 脚本入口共享同一模块类型，自检只将根创建失败包装为前置 `Blocked`。实际 CLI 对已有/越界自检根返回2且无 traceback；执行或收尾异常返回1。

5. **媒体工具版本探测的清理分类。**
   - File: `scripts/verify_baseline.py`
   - Issue: 工具版本分支原先把 `cleanupError` 一律当成缺依赖 `blocked`。
   - Fix: 先传播清理错误为 `failed`。复核另用真实 FFmpeg `-version` 执行后注入清理失败：工具实际 exit0，case 与总清单为 failed/exit1，命令及脱敏日志保留。

6. **自检最终链接清理与证据保存。**
   - File: `scripts/verify_baseline_selftest.py`、`scripts/verify_baseline.py`
   - Issue: `finally` 中链接移除失败会中断最终断言和清单发布，并可能被外层误归为前置阻塞；最终断言文件写入失败也需要失败清单。
   - Fix: 增加局部 `cleanup_links()`，只移除自建链接，失败后记 failed 并继续收尾；断言文件写入失败同样记 failed 后保存清单。新增长期自检断言；另分别执行完整链接清理故障和证据写入故障，均验证 exit1、finishedAt 与 failed 清单落盘。故障注入后的自有链接已单独恢复，目标日志内容未变。

## Findings (not fixed)

没有尚未处理的 S01 本机代码问题。以下为明确的验收边界，不属于本轮可伪造补齐的测试成绩：

- 没有真实 NAS/Linux 内核上的 UID/GID/ACL、挂载、进程组、Docker 与固定组件全流程证据。
- 真实大媒体与目标浏览器的解码、起播、拖动、吞吐、并发负载及资源峰值仍待测；5GiB 稀疏 Range 只证明 HTTP 偏移。
- M1–M3 的目标设备和后续功能门禁继续 blocked/not_run。模板结构通过不能导入通过声明，也不冻结 S02 的独立开发。
- search/engines/components/collection 等未选领域没有计入本轮通过数；历史成绩仍独立保存。

## Verification

所有本机执行均经 `python -B scripts/project_env.py -- ...`。复核新增的故障注入从标准输入执行，没有生成临时测试类或源码。

| 检查 | 结果与证据 |
| --- | --- |
| Lint | pass：主线已执行 `go vet ./...`；复核修订未改 Go。`git diff --check` 通过，只有 Git 行尾转换提示 |
| TypeCheck | pass：主线已执行前端严格 typecheck/build；复核修订未改 Vue/TS。两份 Python AST 语法、方法说明及入参/返回注解复核通过 |
| 初始编译 | pass：主线 Go 包编译、主服务与 worker 构建通过。当前无保留 Go 单测，不能算行为覆盖 |
| 核心实际回归 | pass：`s01-core-1`、`s01-core-2` 各52项；核对真实清单与证据。主服务/网页摘要相同，根独立，端口5828/7386 |
| 领域实际回归 | pass：`s01-domain` 完整 finishedAt/overall passed，downloads 36项、external 83项、release 83项；各验证命令真实 exit0 |
| 最终编排自检 | pass：`s01-review-selftest` 39项，公开 CLI exit0；完整结果在 `evidence/selftest-assertions.json` |
| 最终链接清理故障 | 按预期 failed/exit1：`s01-review-finally-failure`；最终清单和断言仍保存，目标未变，自有链接已恢复 |
| 最终证据写入故障 | 按预期 failed/exit1：`s01-review-evidence-failure`；finishedAt 和 failed 清单落盘，没有伪造缺失的断言文件 |
| 真实工具清理故障 | 按预期 failed/exit1：`s01-review-tool-cleanup`；真实工具 exit0/cleanupError、命令与日志保留 |
| 目标档案 | 按预期 blocked/exit2：`s01-target` 只有结构校验项 passed，M1 blocked、M2/M3 not_run |
| 进程和链接收尾 | 复核后无匹配本轮 core/review 根的 nas/python 子进程；三个 review 完整自检的自建链接已释放，独立旁观进程的存活/停止由自检真实心跳断言覆盖 |
| 规范同步 | 已核对 `baseline-contract.md` 与最终错误优先级、递归脱敏、路径/重解析边界和入口/收尾断言一致；无模板目录需要同步 |

常规复验命令：

```text
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --self-test --root .runtime/verify/s01-review-selftest
```

上述运行根均已存在，应保留；再次复跑须使用全新编号。故障注入摘要与脱敏输出分别保留在对应根的 `evidence/review-injection.json` 与日志中。故意注入的失败不能抹掉，也不能混作产品失败或成功次数。

实施者的 `s01-selftest-2`、`s01-selftest-4` 等开发失败证据仍保留；最终39项通过不覆盖历史记录。没有提交、部署、启动真实第三方组件、修改用户媒体或归档任务。

本机 S01 范围已具备进入 S02 的条件；任务状态、正式验收说明和 S02 激活由主线维护。
