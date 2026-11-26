# Windows 托管 worker 实施与验证

日期：2026-11-26。范围仅为 worker／父子控制协议／JMComic 运行时选择及对应回归；桌面、运行时装配、发行和总体验收分别由主线汇总。

## 实施文件

- `internal/collectioncontrol/protocol.go`：16KiB 有界换行 JSON；严格版本、固定动作、必需字段、无重复／未知／null 字段；start/stop 仅四字段，ready 增加真实回环地址和规范数据根；会话关联比较；只允许精确 `http://127.0.0.1:<实际非零端口>`。
- `internal/collectionworker/managed.go`：`RunManaged` 通过父子继承匿名 stdin/stdout 管道启动，stderr 记录日志；首条 start 有超时；同会话 stop 或父 EOF 有序停止；异常控制消息关闭本实例；只允许 `127.0.0.1:0`，ready 取自仍被本实例持有的实际监听器。
- `internal/collectionworker/config.go`：共享漫画配置读取；增加显式解释器标志、私有运行时清单、只由内部设置的 Managed 标志；独立诊断入口不要求令牌、来源配置或持久数据根。
- `internal/collectionworker/jmcomic.go`：显式解释器优先，显式失败禁止回退；无显式值时验证私有运行时；托管缺少清单时禁止 PATH 回退；普通控制台保留原固定工具名兼容；固定桥接加入 `-X utf8`；只执行本地 probe 的临时目录诊断和脱敏能力原因。
- `internal/collectionworker/service.go`：提取持有真实监听器的运行函数；就绪前完成初始化；返回服务资源释放错误。
- `cmd/nas-collection-worker/main.go`：严格分派普通模式、`--managed-stdio`、`--check-jmcomic` 和帮助；诊断可用返回0、未就绪返回2、实际配置／诊断失败返回1。
- `scripts/verify_collection_managed.py`：可复用的真实 worker 生命周期／认证／降级集成验证。
- `scripts/verify_jmcomic_bridge.py`：真实隔离 probe 参数与生产一致加入 `-X utf8`。

所有新增函数、字段和配置说明均保留中文注释；未新增账号、Cookie、任意 YAML／脚本或浏览器解释器参数入口；不改变数据库 schema 或下载发布状态机。

## 本地检查与临时测试

所有下列构建／测试命令经 `python -B scripts/project_env.py -- ...` 执行，缓存、临时目录和产物位于仓库 `.runtime/`。Go 不在当前 PATH，改用仓库 `.tools/go/bin/go.exe` 与 `gofmt.exe`；没有修改用户环境或 `go env -w`。

1. 初次裸 `gofmt` 因本机 PATH 无对应程序而失败，之后用 `.tools/go/bin/gofmt.exe -w` 正常格式化。
2. 临时生成 `internal/collectioncontrol/protocol_temporary_test.go`，覆盖合法双消息往返、EOF、错误动作／关联值、数组／null／空对象、缺换行、重复／大小写别名／未知／null 字段、错误类型／版本、尾随数据、超限以及 ready 非规范 URL。检查时补充拒绝空查询 `?` 和空片段 `#`，生产地址增加精确规范形式比较。
3. 尝试 `python -B scripts/project_env.py -- .tools/go/bin/go.exe test -race ./internal/collectioncontrol`，Go 在执行前报告 `-race requires cgo`。没有启用 CGO，也没有将其记录为通过。
4. `python -B scripts/project_env.py -- .tools/go/bin/go.exe test ./internal/collectioncontrol` 真实执行上述临时测试并通过，随后已删除该临时测试源文件。
5. 删除临时测试后执行以下命令均成功；此时 Go 包显示 `[no test files]`，仅能作为编译检查，不能重复计算行为断言：

```powershell
python -B scripts/project_env.py -- .tools/go/bin/go.exe test ./internal/collectioncontrol ./internal/collectionworker ./cmd/nas-collection-worker
python -B scripts/project_env.py -- .tools/go/bin/go.exe vet ./internal/collectioncontrol ./internal/collectionworker ./cmd/nas-collection-worker
python -B scripts/project_env.py -- .tools/go/bin/go.exe build -o .runtime/verify/jmwindows-worker.exe ./cmd/nas-collection-worker
python -B scripts/project_env.py -- .tools/go/bin/go.exe build -o .runtime/verify/jmwindows-nas.exe ./cmd/nas
```

Python 对两个变更验证脚本的 `ast.parse` 语法检查通过；限定本实现文件的 `git diff --check` 通过（仅既有 Git 行尾规范提示）。

## 托管集成证据

首次使用既有真实 venv 执行17项通过：

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_collection_managed.py --worker .runtime/verify/jmwindows-worker.exe --python .runtime/tools/jmcomic-venv/Scripts/python.exe
```

证据：`.runtime/verify/jmcomic-managed-worker-20261126T002333638676Z-c93303/result.json`。该轮是 venv 的真实依赖 probe，不能作为随包私有解释器证据。

运行时实现者随后提供实际装配的 CPython 3.14.4／JMComic 2.7.12 清单 `.runtime/tools/jmcomic-private/jmcomic-runtime.json`。首次新清单诊断失败，安全能力原因指向清单完整性校验，不是 Python probe。原因是运行时契约将 requirements 摘要从 Windows 工作区 CRLF 统一为规范 LF，但当时 worker 二进制仍含旧摘要。运行时实现者更新契约／清单并真实验证 Go Load 后，重新构建 worker；没有放宽校验或回退到系统 Python。

失败轮留档：`.runtime/verify/jmcomic-managed-worker-20261126T002812251876Z-f7cb2e/result.json`（在诊断阶段终止，cases 为空，不能计为通过）。

```powershell
python -B scripts/project_env.py -- .tools/go/bin/go.exe build -o .runtime/verify/jmwindows-worker-v2.exe ./cmd/nas-collection-worker
python -B scripts/project_env.py -- python -B scripts/verify_collection_managed.py --worker .runtime/verify/jmwindows-worker-v2.exe --runtime-manifest .runtime/tools/jmcomic-private/jmcomic-runtime.json
```

修复后20项全部通过：`.runtime/verify/jmcomic-managed-worker-20261126T002936194872Z-7eb146/result.json`。范围包括：

- 真实 worker 与匿名私有管道，stdout 仅 ready；真正绑定回环临时端口；中文／空格数据目录及非仓库 CWD。
- 诊断不要求 token/data，不在该 CWD 创建持久文件；disabled、缺清单、显式无效解释器、真实私有清单；显式无效解释器即使存在有效清单也不回退。
- 托管子进程 PATH 为空，PYTHONHOME/PYTHONPATH 故意污染；实际私有解释器通过清单验证与依赖 probe，认证 capabilities 返回 JMComic 可用。
- 未提供运行时仍可启动 worker，但 JMComic 不可用且不搜索系统 Python；未认证 capabilities 为401，healthz 正常。
- 正常 stop、父 EOF、同目录立即重启；启动阶段 EOF 不创建数据根；六类无效 start 在数据初始化前拒绝；错误 stop 关联值终止并报错。
- 在真实 worker 中启动固定 Go 协议工具的慢操作后停止，保留 `recovery_required`，不发布 complete，重启不自动重试。此项下载过程使用 Go 协议替身，不能称为真实 Python 下载／DLL 释放测试。

生成协议工具使用的临时 `main.go` 由脚本在构建后删除。执行日志不保存 token、nonce、原始控制消息或搜索词。

## 真实私有 Python 桥接离线验证

实际使用清单内私有解释器，未使用开发 venv：

```powershell
python -B scripts/project_env.py -- .runtime/tools/jmcomic-private/runtimes/jmcomic/python.exe -I -B -X utf8 -c "import sys, runpy; sys.path.insert(0, 'E:/vscodeProjects/nas-content-manager/scripts'); runpy.run_path('E:/vscodeProjects/nas-content-manager/scripts/verify_jmcomic_bridge.py', run_name='__main__')"
```

测试入口显式加入 scripts 路径仅为离线验证 harness 导入项目工具，生产桥接没有改动模块搜索路径。harness 另建真实隔离子进程执行固定生产桥接 probe。

结果：`.runtime/verify/jmcomic-real-bridge-5bcfd09caa494461aa124ff01bc0eb1f/report.json`。真实固定包、上游实体、客户端构建、下载器 hook、图片解码／GIF、规范选章、完整性与预算、私密进度、三页复用仅再取一页、损坏页重取、检查点／身份漂移／硬链接／sidecar 拒绝、原子替换争用均通过。仅替换域名发现和元数据／搜索／图片的传输层，`realNetworkCalls=0`；不是公网站点下载。

## 既有 JMComic 协议回归

以下通过 Python 子进程只为当前验证进程把仓库 `.tools/go/bin` 加到 PATH，以满足已有脚本构建 Go 固定协议工具；仍经项目环境入口：

```powershell
python -B scripts/project_env.py -- python -B -c "import os, pathlib, subprocess, sys; os.environ['PATH'] = str(pathlib.Path('.tools/go/bin').resolve()) + os.pathsep + os.environ.get('PATH', ''); raise SystemExit(subprocess.call([sys.executable, '-B', 'scripts/verify_jmcomic.py', '--binary', '.runtime/verify/jmwindows-nas.exe', '--worker-binary', '.runtime/verify/jmwindows-worker.exe']))"
python -B scripts/project_env.py -- python -B -c "import os, pathlib, subprocess, sys; os.environ['PATH'] = str(pathlib.Path('.tools/go/bin').resolve()) + os.pathsep + os.environ.get('PATH', ''); raise SystemExit(subprocess.call([sys.executable, '-B', 'scripts/verify_jmcomic_advanced.py', '--binary', '.runtime/verify/jmwindows-nas.exe', '--worker-binary', '.runtime/verify/jmwindows-worker.exe']))"
```

- 基础66项：`.runtime/verify/jmcomic-20261126T002812581455Z-561be6/result.json`。
- 高级144项：`.runtime/verify/jmcomic-advanced-20261126T002812579621Z-e2a346/result.json` 与 `advanced-evidence.json`。

均使用真实主应用／worker 及 Go 上游协议工具，覆盖入库／阅读／发布／选章／进度等原业务，但不执行 Python 桥接。上述轮次的 worker 早于 LF 摘要契约更新；这些用例使用显式协议工具，不经过运行时清单分支。最终完整发行后的统一回归由主线再执行，计数不能叠加为独立覆盖项。

## 剩余证据边界

本报告没有取得干净 Windows 11 普通用户机器、真实公网站点、ARM64 执行、Linux 实机或 Docker/NAS 验收。真实 Python 后代 Job／DLL 释放、桌面父进程崩溃和实际发行移动／替换由桌面与主线专项结果说明，不能用本报告的 Go 慢操作替身替代。未执行自动安装全局工具、修改系统 PATH、进程名批量终止或 Git 提交。
