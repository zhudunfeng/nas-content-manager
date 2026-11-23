# S01 实施与执行证据（2026-11-23）

## 本项改动

- `scripts/verify_baseline.py`：严格新根、四态清单、实际产物/工具身份、独立主服务/静态挑战、原脚本组合、有界子进程与整树清理、持续原子落盘和脱敏证据。
- `scripts/verify_baseline_selftest.py`：保留的集成自检入口；真实短命子进程、超时、后代、端口、路径和脱敏断言。没有新增需要保留的单元测试类。
- `docs/baseline.md`：实际CLI、所有原验证脚本参数/前置/副作用/成功依据矩阵、领域触发、目标验收协议。
- `docs/nas-acceptance.template.json`：带中文字段说明的真实设备档案，未知null，M1/M2/M3保持blocked/not_run。
- 本任务 `info.md`：实施前冻结接口和错误矩阵；本文件记录实际结果。

不改业务Go/Vue、不改数据库、不启动真实Docker/组件，不使用用户实例。正式spec、总验收文档及任务状态由主线负责。最终脚本复核交给独立checker后如有修正，以checker最后执行证据为准。

## 实际构建与参数核对

全部本地命令通过 `python -B scripts/project_env.py -- ...`；工具和临时文件限定在仓库 `.runtime/`。

```text
go build -o .runtime/bin/nas.exe ./cmd/nas
go build -o .runtime/bin/nas-collection-worker.exe ./cmd/nas-collection-worker
```

两次构建均exit0。此处省略外层统一入口以便阅读；完整形式为 `python -B scripts/project_env.py -- go build ...`。主服务SHA256：`8c12e94aa83949c75cd61188c28dc26f29dc4518e758c2b63b166866fe454ee2`。

11个真实CLI `--help`均exit0（原十个验证脚本加新编排入口，排除内部自检模块）：`.runtime/verify/s01-help/inventory.json`及同目录帮助快照。先前已用CodeGraph定位进程清理/健康入口，并核对现有脚本argparse、结果文件和Windows Job实现；没有为了编排复制业务断言。

Python AST语法/中文方法docstring/显式入参出参检查通过，新模板JSON解析通过；未在源码旁生成pycache。主线另行完成go vet、当前无测试文件的go test、web typecheck/build，此文件不重复计为产品行为覆盖。

## 两次实际 core

执行形式：

```text
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases core --artifacts .runtime/bin --ffmpeg <本机FFmpeg绝对路径> --ffprobe <本机ffprobe绝对路径> --large-file --root .runtime/verify/s01-core-1
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases core --artifacts .runtime/bin --ffmpeg <本机FFmpeg绝对路径> --ffprobe <本机ffprobe绝对路径> --large-file --root .runtime/verify/s01-core-2
```

实际使用已安装的FFmpeg/ffprobe；完整版本与工具摘要在各清单中，公开说明不写用户安装绝对路径。

| 运行 | UTC起止 | 端口 | 断言/结果 |
| --- | --- | --- | --- |
| `s01-core-1` | 01:45:19.879–01:45:45.048 | 5828 | 52项；passed；exit0 |
| `s01-core-2` | 01:47:12.538–01:47:33.695 | 7386 | 52项；passed；exit0 |

同一主服务摘要，独立data/media/web/端口；每次验证自有进程存活、SQLite健康和随机静态探针后才调用原 `verify.py`。鉴权、五类媒体、中文检索、扫描幂等、个人状态、音乐元数据/封面/Range、TXT/CBZ/EPUB、快照一致性和大偏移断言实际完成。两个快照各237568字节，并分别通过原脚本 `PRAGMA integrity_check`。

证据分别为 `.runtime/verify/s01-core-{1,2}/run-manifest.json`、`evidence/core-assertions.json` 和清单引用的脱敏日志。`5,368,709,243 B`稀疏协议样本只证明HEAD/Range/suffix/416/304等偏移语义，不是影片解码/吞吐实测。

## 编排集成自检

移交前实施者执行：

```text
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --self-test --root .runtime/verify/s01-selftest-5
```

**38项passed，exit0**。证据：`.runtime/verify/s01-selftest-5/run-manifest.json`与`evidence/selftest-assertions.json`。包括：

- 已有/越界/上溯根；Windows尾随点/空格、设备名、ADS、通配符；已有/非法自检根实际CLI返回2且无traceback。
- 受控junction祖先拒绝；二次日志脱敏不穿过链接修改根外文件；两次根独立。
- 缺产物实际子清单blocked、真实退出2；占端口保留既有监听器。
- 真实子进程exit7、超时清理；后续独立项继续；注入清理错误时保留命令/exit0/日志并判failed。
- 自有孙进程停止，独立旁观进程心跳继续；正常父进程退出也收回遗留后代和输出管道。
- 凭据/URL/已知秘密、POSIX多类根、含空格Windows路径、UNC/网络共享路径脱敏；递归脱敏后JSON仍可解析。
- 模板校验不能制造M1/M2/M3通过，伪造passed声明拒绝。

开发过程保留失败证据而不覆盖：`s01-selftest-2`新增清理错误断言暴露了检查位置错误；`s01-selftest-4`新增CLI断言暴露了`__main__`反向导入导致异常类型重复。已分别修正为函数内判定和同模块别名，后续完整38项回归通过。早先的21/34/38项成功只代表当时覆盖，最终复核以以下39项为准，历史证据均保留。

独立checker接手后修正最终链接清理、断言证据保存及真实工具版本探测的错误传播，完成公开CLI **39项passed，exit0**：

```text
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --self-test --root .runtime/verify/s01-review-selftest
```

最终证据：`.runtime/verify/s01-review-selftest/run-manifest.json`、`evidence/selftest-assertions.json`及清单引用日志。另将以下故障注入作为失败处理验证，均实际返回exit1且最终清单为failed：

| 故障场景 | 运行根 | 最终证据 |
| --- | --- | --- |
| 最终自有链接清理失败 | `.runtime/verify/s01-review-finally-failure` | `run-manifest.json`与`evidence/review-injection.json`；finishedAt和断言仍保存，目标内容未变，自有链接已恢复 |
| 最终断言证据保存失败 | `.runtime/verify/s01-review-evidence-failure` | `run-manifest.json`与`evidence/review-injection.json`；finishedAt和failed清单保存，不伪造缺失的断言文件 |
| 真实FFmpeg版本命令清理失败 | `.runtime/verify/s01-review-tool-cleanup` | `run-manifest.json`与`evidence/review-injection.json`；真实工具exit0、cleanupError、命令和脱敏日志均保留 |

已读回上述清单和故障注入摘要，结果与[独立复核](research/quality-review.md)一致。故意注入的失败不混入产品成功或失败计数。本机S01交付结束；真实NAS门禁仍按下文待验，S02由主线继续推进。

## 扩展组合与目标轨道

实际执行：

```text
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases downloads,external,release --artifacts .runtime/bin --root .runtime/verify/s01-domain --timeout 300
```

整体 **passed，exit0**，UTC 01:50:59.456–01:54:46.845。`.runtime/verify/s01-domain/run-manifest.json`及其引用的`evidence/*`和脱敏`logs/*`保存实际结果：

| 领域 | 实际断言 | 子命令耗时/退出 | 证据范围 |
| --- | --- | --- | --- |
| downloads | 36 | 209.722s / 0，无超时 | 实际应用+受控qB协议；选择、发布门禁、入库失败/重试/恢复；不是真实BT/Docker |
| external `--workflow` | 83 | 5.564s / 0，无超时 | 实际应用+四类本地外部协议和六阶段流程、不确定写入与对账；不是真实用户组件 |
| release `--self-test` | 83 | 11.745s / 0，无超时 | 发布边界与伪包/摘要/架构断言；不是实际发布包通过 |

未运行search、engines、components、collection、浏览器或桌面组合，本轮不将其历史通过计入新基线。worker只有实际构建通过，不据此声称worker业务集成通过。最小core和上述三领域均不自动访问真实站点/账号或执行Docker。

目标模板实际执行：

```text
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite target --target-file docs/nas-acceptance.template.json --root .runtime/verify/s01-target
```

`.runtime/verify/s01-target/run-manifest.json`：仅 `target-profile` 结构校验passed；M1 blocked，M2/M3 not_run，整体blocked。没有登录设备或生成目标通过声明。该模式的Python进程设计退出码2；非零壳层显示不能被误解为实测失败或通过。

## 仍待真实环境

- Linux amd64/arm64实际NAS、文件系统/ACL/UID/GID/挂载、Compose及固定镜像、真实外部组件与真实BT/网页/工具流程尚未验收。
- 真实大媒体与代表浏览器、起播/拖动/吞吐、并发扫描/下载、CPU/RSS峰值和大库延迟仍需负责人提供设备/样本并实测。
- 重启/断连、锁、备份空间不足、秘密/组件卷完整材料、停机隔离恢复和受控掉电类演练仍为M1–M3发布门禁；只能在已备份隔离副本执行破坏性演练。
- 本项Windows真实进程自检不自动证明POSIX进程组实测，交叉编译也不能代替设备记录。

缺设备不阻止启动S02，但不能宣称真实NAS支持组合已全部通过。整个运行根含私密fixture/数据库，不能整体上传；分享限已脱敏引用证据并人工复核。
