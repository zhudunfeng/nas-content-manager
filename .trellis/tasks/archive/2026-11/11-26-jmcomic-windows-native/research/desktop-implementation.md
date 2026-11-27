# Windows 桌面托管实现与验证

日期：2026-11-26。范围：`internal/desktop/` 与专用桌面托管回归脚本。运行时清单／装配、worker 控制端、主应用诊断与总体验证由同任务其他实现分工负责。

## 1. 改动文件

修改：

- `internal/desktop/config.go`：桌面 schema 2、`collectionMode`、CLI／环境优先级、v1 显式迁移与保存前备份。
- `internal/desktop/launcher.go`：独立漫画组件状态、固定 IPC 动作、主服务连接快照及启停屏障；主服务排空超时仍等待自有可选进程清理，保留超时错误并拒绝该次重启。
- `internal/desktop/ipc_windows.go`：当前用户私有管道固定动作白名单扩展。
- `internal/desktop/shell_windows.go`：漫画组件状态、启用、停用与重新检测菜单。

新增：

- `internal/desktop/collection_config.go`：固定模式／控制动作、完整包新 profile 默认判断、有界原字节 v1 备份，不覆盖不同的既有备份。
- `internal/desktop/collection_controller.go`：三种模式连接快照、中文降级原因、串行变更与自有 worker 清理。
- `internal/desktop/collection_managed.go`：专属目录所有权、受控子进程环境、私有继承管道、认证能力和共享根挑战、停止预算。
- `internal/desktop/collection_process_windows.go`：挂起创建、恢复前归入本实例 kill-on-close Job、嵌套后代所有权、真实句柄／Job 排空；区分 worker 自身非零退出与资源清理失败。
- `internal/desktop/collection_process_other.go`：非 Windows 编译兼容，不在 Linux 启用桌面托管。
- `scripts/verify_desktop_collection.py`：持久可复用 Windows 集成脚本，支持基础 worker、显式受信 Python 或完整私有运行时清单。

没有修改 `cmd/nas-desktop/`，没有修改用户全局 PATH／Python／Go 环境，没有提交 Git。

## 2. 实际接口与边界

- 模式：`disabled|managed|external`；CLI `--collection-mode` 优先于 `NAS_DESKTOP_COLLECTION_MODE`，再优先于保存配置。Docker 不接受 `managed`。
- v1 的实例身份、数据目录、媒体根保持原值；存在显式外接 tuple 时迁移 `external`，否则迁移 `disabled`，不会因完整包升级自动接管。只在保存前创建原字节 `desktop.v1.json`；`--print-config` 不写入。
- 新原生 profile 在相邻 worker 和 manifest 均存在时默认 `managed`；显式外接连接优先。基础包不假报依赖就绪。
- 固定 IPC：`collection-enable`、`collection-disable`、`collection-check`，经现有生命周期队列串行处理；托盘名称为「启用本机漫画下载」「停用本机漫画下载」「重新检测漫画组件」。
- 状态：`collectionMode`、`collectionState`、`collectionMessage`；`collectionState` 为 `disabled|external|starting|ready|unavailable|failed`。
- 专属状态：`<profile>/collection-managed/{owner.json,data,secrets,tmp}`；日志：`<profile>/logs/collection-worker.log`。未知目录／归属不匹配／链接别名不接管，专属根不与程序、业务数据或授权媒体重叠。
- 托管只向子进程传受控环境；Python 覆盖必须为绝对路径，不继承 `PYTHONHOME`／`PYTHONPATH`，不从系统 Python 回退。`disabled` 清除整组 app tuple，`managed` 仅在核验后整组注入，`external` 完整保留原语义且不拥有外接进程。
- ready 通过 `internal/collectioncontrol` 的严格版本协议解析，核对关联值、实例、真实回环端口、规范数据根，再进行 token 认证 capabilities 和真实落盘探针；不从日志判断就绪。
- 缺 worker／启动失败：主应用获得脱敏通用原因「本机漫画获取服务启动失败，请在托盘查看日志或重新检测」，具体失败原因留在桌面日志。worker 可运行但本地依赖不可用时保留其具体能力原因。没有将本地依赖就绪扩大为站点可达。
- worker 异常退出仅降级获取功能；当确切进程和 Job 已完整清理时，允许用户主动重检。真正 Job／句柄清理失败继续阻止重启；不自动重发不确定下载。

## 3. 已执行检查

所有命令通过 `python -B scripts/project_env.py -- ...`，可控产物、缓存和临时目录均在仓库 `.runtime/`。

```powershell
python -B scripts/project_env.py -- .tools/go/bin/gofmt.exe -w <本轮桌面Go文件>
python -B scripts/project_env.py -- go vet ./internal/desktop
python -B scripts/project_env.py -- go test ./internal/desktop ./cmd/nas-desktop
python -B scripts/project_env.py -- go build -o .runtime/verify/desktop-jm-managed.exe ./cmd/nas-desktop
python -B scripts/project_env.py -- go build -o .runtime/verify/worker-jm-managed.exe ./cmd/nas-collection-worker
python -B scripts/project_env.py -- python -B scripts/verify_desktop_collection.py --binary .runtime/verify/desktop-jm-managed.exe --worker .runtime/verify/worker-jm-managed.exe --web-dir web/dist
python -B scripts/project_env.py -- python -B scripts/verify_desktop_collection.py --binary .runtime/verify/desktop-jm-managed.exe --worker .runtime/verify/worker-jm-managed.exe --web-dir web/dist --runtime-manifest .runtime/tools/jmcomic-private/jmcomic-runtime.json
```

- Go vet／编译／构建通过；保留的 Go 包没有单元测试文件，因此 `go test` 记录是编译证据，不冒充行为测试。
- `cmd/nas-desktop` 在子进程局部 `GOOS=linux, CGO_ENABLED=0`、`GOARCH=amd64`／`arm64` 下分别交叉构建通过，输出为 `.runtime/verify/desktop-jm-linux-amd64` 与 `desktop-jm-linux-arm64`。未执行 Linux 产物。
- 基础隔离回归在补入最终配置附加项前通过 9 组，包含崩溃后重检修复；最终完整私有运行时回归通过 **14 组**，包含以下全部新增断言。
- Python 脚本 `ast.parse` 与 scoped `git diff --check` 通过；不会生成源码旁 `__pycache__`。
- 未生成保留的临时单元测试类；嵌套测试的一次性 Go fixture 源码在编译后立即删除，其 EXE 与全部样本目录随隔离回归清理。

14 组行为结果：

1. 缺 worker 只降级；真实登录后 `/api/collection/capabilities` 的 `workerReason` 返回上述脱敏启动原因。
2. `disabled` 清除污染的完整外接 tuple。
3. schema 2 的十个字段逐一缺失／null 全部拒绝，原字节不变。
4. v1 显式迁移、身份与数据目录不变、原字节备份、只读 print-config。
5. v1 外接配置不被接管；CLI 高于环境，高于保存配置。
6. 新完整包 profile 默认 managed；print-config 无落盘。
7. 真实 worker、认证目录挑战、正常退出及自有句柄释放；此轮启动同时污染外接 tuple、PYTHONPATH 和 PYTHONHOME。
8. 清单选择真实私有 Python 执行固定桥接；退出后私有映像独占打开、改名、原位覆盖并核对摘要、程序目录改名。
9. 真实 IPC 启用／重检／停用串行重启；重检须观察到新的真实 worker PID，不能把旧 ready 响应当完成。
10. 通过确切 worker 内核句柄制造异常退出；组件失败期间媒体 `/healthz` 仍 200，主动重检恢复。
11. 父桌面强杀时 Job 回收，独立 Python 哨兵仍存活，同 profile 可重启。
12. 真实私有 Python 已映射 Pillow／curl／AES／CPython 扩展；启动取消和父强杀均回收嵌套 Job，独立哨兵保留。
13. 真实嵌套子树停止、运行时映像替换后，同 profile 恢复固定桥接。
14. 未标记专用目录拒绝接管，不创建 owner 标记、不改用户原文。

## 4. 真实 DLL 与嵌套 Job 的证据范围

使用 `.runtime/tools/jmcomic-private/jmcomic-runtime.json` 的 CPython **3.14.4** 和 JMComic **2.7.12**，整份清单成员先复制到带中文／空格的隔离程序目录；从其他 CWD 启动。没有修改原运行时构建材料。

固定桥接先完成真实 probe；另外以一次性可信 Go 工具替身维持真实私有 Python 活动。真实 worker 将该工具归入既有 tool Job，Python 是该工具的后代，外层仍归桌面 Job。Python 实际执行 Pillow 图像创建、curl 句柄初始化和 AES 加密后写入仅含 PID 的完成标记；不执行漫画搜索或下载。

验证器用 PSAPI 读取 Python 已映射模块，而非只枚举旁置文件，共观察到 **26 个私有真实映像**，包括：

- `python.exe`、`python3.dll`、`python314.dll`、`vcruntime140.dll`；
- Pillow `_imaging.cp314-win_amd64.pyd`；
- curl `_wrapper.pyd`、`libcurl-impersonate-ae864b926b0a86bfa3b18a664407a1ab.dll`；
- `_cffi_backend.cp314-win_amd64.pyd`、`libffi-8.dll`；
- `_raw_aes.pyd`、`_raw_aesni.pyd`、`_raw_ecb.pyd`；
- `libcrypto-3.dll`、`libssl-3.dll`、`_ssl.pyd` 及其余标准扩展。

活动期间逐个零共享读写打开必须失败，建立占用正例。正常启动取消或父强杀后，等待事先保存的确切后代进程句柄全部触发，再对同一映像独占打开／改名／原位覆盖、核对源摘要；整个程序目录随后改名还原并立刻同 profile 重启。没有用固定睡眠、进程名批量终止或单纯 PID 消失冒充回收屏障。

异常强杀中，父 `wait()` 本身不保证内核已调度完所有 Job 后代终止；验证器先检查父 EXE，再等待保存的后代句柄，之后检查 worker／Python／DLL。这与正常退出由桌面主动等待完整屏障是不同的路径。

## 5. 尚未验收

- 本机是现有开发 Windows 环境；不是干净 Windows 11 普通用户虚拟机，不能据此排除开发机预装依赖、其他用户权限／杀毒策略差异。
- 未访问真实漫画站点；嵌套 fixture 是实际依赖和进程生命周期证据，不是公网搜索／下载或真实下载中断验收。
- 没有物理点击 Explorer 托盘菜单；本轮固定 IPC 路由与真实菜单代码接线已实现。原桌面托盘／媒体流回归及最终发行包由主线统一复跑。
- 没有证明 Windows ARM64 或其他 Windows 版本运行通过；Linux 交叉编译仅证明编译兼容，不代表 Linux 桌面托管运行。
