# 可复跑基线与真实 NAS 验收

`scripts/verify_baseline.py` 复用已有接口验证，输出独立的 `run-manifest.json`。默认只运行 core；领域组合按变更选择。真实 NAS 档案校验是单独轨道，不能用本机 fixture 成绩替代设备验收。

## 运行本机最小基线

先准备现有 Go 工具链与构建网页；命令均通过项目环境入口，不修改用户级环境。Windows 示例：

```powershell
python -B scripts/project_env.py -- go build -o .runtime/bin/nas.exe ./cmd/nas
python -B scripts/project_env.py -- npm --prefix web run typecheck
python -B scripts/project_env.py -- npm --prefix web run build
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --artifacts .runtime/bin --cases core --ffmpeg D:/tools/ffmpeg/bin/ffmpeg.exe --ffprobe D:/tools/ffmpeg/bin/ffprobe.exe --large-file
```

将两个媒体工具替换为本机实际绝对路径。Linux 使用对应可执行文件；编排器按平台查找 `nas`/`nas.exe`。不提供媒体工具时 core 明确 `blocked`，不会把缺少可解码视频的准备工作算作五媒体通过。`--large-file` 可省略；启用时生成的是 5GiB 稀疏 HTTP 偏移夹具，不能证明解码、拖动或吞吐。

每次默认创建 `.runtime/verify/baseline-<UTC时间>-<随机值>`；也可用 `--root .runtime/verify/<全新短编号>`。已有目录一律拒绝，不覆盖历史证据。core 复制当前构建网页，隔离数据和媒体，以随机静态文件挑战加数据库健康检查确认本次服务，之后才调用 `verify.py`。CLI不能传递预绑定监听器，因此有探测后重绑的小窗口；竞争进程无法通过独有探针，不会对其执行 setup/媒体变更。

```powershell
# 收集组合还需要真实worker产物，但来源/下载器仍使用受控夹具。
python -B scripts/project_env.py -- go build -o .runtime/bin/nas-collection-worker.exe ./cmd/nas-collection-worker
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --cases downloads,search,engines,external,collection --artifacts .runtime/bin --root .runtime/verify/b01 --timeout 600

# 账号组合固定同时验证在线账号与离线维护；Windows终端检查只操作自有隐藏控制台。
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --cases account --artifacts .runtime/bin --root .runtime/verify/a01 --timeout 600

# components需本机Go；现有Unix socket协议对整个路径有100字节上限，选择短运行根。
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --cases components --artifacts .runtime/bin --root .runtime/verify/c01 --timeout 600

# 发布边界自检；实际发布包仍需单独传归档给原校验器。
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --cases release
python -B scripts/project_env.py -- python -B scripts/verify_release.py <实际归档.zip或tar.gz>

# 编排自身的真实进程/路径/超时/脱敏集成自检。
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --self-test
```

`--timeout` 是每条子验证的墙钟预算（默认300秒，1–3600），`--startup-timeout` 是 core 数据库和专属网页探针的启动预算（默认30秒，1–120）。`--port 0` 默认由内核选空闲回环端口；显式值只允许1024–65535，占用即阻塞，绝不杀原监听器。独立用例在前项失败后继续。

本机模式清除继承的 `NAS_*` 与代理参数，仅传脚本固定夹具参数；没有 `--serve`、真实站点、真实账号、rclone/yt-dlp、真实Docker等入口。Windows命令在挂起状态下加入独占Job后才恢复，正常结束/超时都会收回自有后代；不按名字或全局端口结束进程。POSIX使用独立session/process group。本轮Windows已有实际自检；Linux设备执行仍是目标待验。

## 现有脚本矩阵

以下列出真实参数与副作用。原脚本保留独立入口；表内可选项不会被本机编排器隐式启用。原脚本没有统一外层超时，编排时统一增加墙钟和整树清理；API内部超时仍以各脚本为准。

| 原脚本 / 编排组合 | 真实参数 | 前置、隔离副作用 | 成功证据与范围 |
| --- | --- | --- | --- |
| `verify.py` / core | `--base-url --root`；可选 `--prepare-only --large-file --ffmpeg --username --password` | **需要已运行主服务，没有--binary**。生成五媒体样本、私密凭据，setup独立管理员、库/状态、备份；可复用自身样本目录，因此外层必须新根 | stdout逐条断言与最终计数；实际SQLite快照。鉴权/来源边界/五媒体/扫描幂等/标签收藏进度/TXT/CBZ/EPUB/音乐/快照。原脚本口令参数不由编排器传入 |
| `verify_account.py` / account | `--binary --root`；编排固定加`--offline` | 真实应用与冻结历史SQLite库；创建隔离管理员/会话、升级历史库、修改密码与撤销会话，并执行独立离线维护验证。原脚本不加`--offline`时只验证在线 | `result.json`及逐条断言、实际产物摘要；在线/离线分别计数。编排拒绝缺失或不一致的离线证据；实际Console和目标平台范围读取子结果，不由组合名称推断 |
| `verify_downloads.py` / downloads | `--binary --root`；可选 `--serve` | 主服务+固定回环假qB协议，创建data/media/downloads，重启应用和受控任务；不执行Docker/真实BT | `result.json`含passed、assertions、realApplication、realBTTransfer=false、dockerExecuted=false；发布门禁与状态/重试 |
| `verify_torrent_search.py` / search | `--binary --root`；可选 `--serve` | 主服务+本地Torznab和qB协议；私密来源配置、搜索与确认任务均为夹具 | `result.json`及逐条断言；不证明真实站点搜索/BT |
| `verify_engine_bootstrap.py` / engines | `--binary --root` | 主服务+受控发现/引擎协议；操作隔离接入状态 | `result.json`及逐条断言；不证明Docker助手和真实镜像 |
| `verify_external.py` / external | `--binary --root`；可选 `--workflow --serve` | 编排显式加`--workflow`。实际应用访问本地固定Sonarr4/Radarr7/MoviePilot3.2.8/AutoBangumi4协议；包含隔离DB迁移夹具和不确定写入对账 | `result.json`及逐条断言；真实外部实例/媒体变更/安装第三方工具均false |
| `verify_components.py` / components | `--binary --root`；可选 `--serve` | 还需Go。临时编译私密Unix协议桥并删除生成Go源，运行回环helper夹具；socket全路径≤100字节 | `result.json`及断言；真实应用/私密协议，dockerExecuted=false |
| `verify_collection.py` / collection | `--binary --worker-binary --root`；可选 `--rclone --ytdlp --ffmpeg --web-url --serve` | **主服务和worker都必须构建**。真实worker/本地文件/RSS/受控BT；可选真实工具与公开媒体URL须独立显式运行原脚本，编排不传 | `result.json`区分realWorker、realRclone、realWeb和realBTTransfer；本机worker不能证明真实网页获取/BT |
| `verify_release.py` / release | 可选归档位置参数、`--self-test` | 编排仅`--self-test`；临时伪包/架构/摘要边界材料随TemporaryDirectory清理，不构建正式发布包或执行Docker | 最终“发布边界回归通过”计数；独立真实归档验证需归档与旁边.sha256，不能混淆 |
| `verify_collection_browser.py` / 不编排 | `--root --browser` | 需原收集脚本`--serve`环境、固定已安装浏览器及Playwright；会运行浏览器并截图 | 浏览器断言与视觉证据，只证明实际执行的UI；启动前继承project_env环境 |
| `verify_account_browser.py` / 不编排 | `--binary --root` | 需已安装Edge及Python Playwright；实际应用与两个隔离browser context。另有明确分开的受控响应呈现检查 | 结果绑定二进制与复制后的网页资源摘要；真实后端/浏览器断言与受控呈现分别计数，截图前清除密码；不等于Linux/NAS验收 |
| `verify_desktop.py` / 不编排 | `--binary`；可选 `--web-dir --with-tray` | **只接受Windows桌面EXE**。启动隔离profile，检查命名管道/锁/重启；可选真实托盘会操作原生窗口 | 桌面启动/资源/清理断言；不能把普通nas交给它，也不由本机组合自动触发 |

所有原脚本的fixture可能包含本地测试密码、来源秘密、媒体源、数据库和脱敏前的内部状态。**不要上传整个运行根**。可分享证据限清单引用的 `logs/*` 和 `evidence/*`，发布前仍需人工复核。二次日志脱敏会逐级拒绝穿过symlink/junction/reparse；已知秘密、凭据字段、URL与绝对路径被遮蔽，单个日志限制4MiB，超量仍持续排空管道。fixture不作为可公开证据自动列出。

## 清单与结果解释

`run-manifest.json` v1记录源码commit/dirty/变更计数、UTC起止、平台/Python、实际产物SHA256/大小、显式工具版本/摘要、有界参数、脱敏argv、真实退出码/超时、断言数及运行根相对证据路径。文件系统无法识别时明确null并说明，不能从开发机推断NAS文件系统。

| 状态 | 含义 | 进程退出 |
| --- | --- | --- |
| passed | 真正执行且exit0、预期结果和正数断言齐全 | 整体0 |
| failed | 断言/非零退出/超时/缺预期结果/清理或证据保存失败 | 整体1，优先于blocked |
| blocked | 缺产物/工具/安全进程所有权、端口占用；真实NAS条件不足 | 整体2 |
| not_run | 本次未选的领域或尚未执行的目标门禁 | 不记为通过；已选项未运行使整体blocked |

非法或重复运行根在创建前拒绝并exit2，不向该不安全位置写“失败清单”。选中项初始not_run；清单持续原子保存。进程被强行结束时，不会预填passed；尚未收尾的清单须结合finishedAt与命令日志判断，不把中断当作完整基线。

## 真实设备档案与 M1/M2/M3

从 `docs/nas-acceptance.template.json` 复制独立设备组合档案，保持未知值为null。不要填真实账号、令牌、私密URL或原始路径。填入匿名设备ID、实际OS/架构、CPU/内存/盘/文件系统、UID/GID/ACL、挂载、Docker/Compose及镜像摘要、真实客户端和合法媒体样本。原生Linux amd64/arm64、Windows原生、Docker Desktop每个实际组合分别存档。

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite target --target-file docs/nas-acceptance.template.json
```

v1只做JSON结构与待验门禁校验，输出脱敏档案副本。**档案项结构passed与真实门禁passed不是同一件事**：M1–M3只能blocked/not_run，整体exit2为预期；不连接设备、不启动组件、不导入任何验收通过声明。以后需要证据导入时另行定义校验/归属，不改写旧证据。

实测前固定构建/样本/缓存条件、重复次数、采样周期和阈值；每条记录原始值、p50/p95或峰值、失败数、负责人和证据编号。至少测真实大媒体起播/多点拖动/有效吞吐，并发扫描与真实下载，CPU/RSS峰值和大库查询；同时验证只读媒体、挂载掉线后索引保留、任务取消/重启、锁与恢复。掉电类演练只在**已备份隔离副本**上执行，不能对生产实例隐式注入故障。

| 里程碑 | 真实门禁 | 待提供输入/责任 |
| --- | --- | --- |
| M1 可持续使用和恢复 | 数据锁、重启、备份空间不足、停机隔离恢复；账号/会话/通知后续功能 | 目标设备操作者，OS/数据盘/备份材料/隔离副本；未分配负责人保持null |
| M2 整理与消费 | 代表客户端+真实大媒体的起播、拖动、吞吐、并发及CPU/RSS/大库；作品/元数据/字幕/管理 | 合法样本、浏览器清单、预设阈值、测量记录与相应功能完成 |
| M3 无人值守和维护 | 真实组件获取到入库、UID/ACL/挂载、断连/重启恢复与保数据维护 | 固定真实版本/摘要、挂载映射、维护/恢复条件及对应功能完成 |

设备缺失阻止该组合发布通过声明，不阻止后续独立功能开发。本轮执行证据记于S01任务的 `implementation.md`；历史结果仍见 `docs/verification.md`，不要把历史成绩当成本轮实跑。
