# Windows UAV 自动初始化最终主线验证

日期：2026-11-27（Asia/Shanghai）。环境为本机 Windows amd64 开发环境；所有构建、测试及临时工具均通过 `python -B scripts/project_env.py -- ...`，可控缓存与临时目录位于仓库`.runtime/`。没有修改用户级环境、系统Python、全局PATH、注册表或现有`E:\test`安装。

## 1. 最终源码和可复核产物

生产Go在最终路径修复后冻结。`scripts/release.py`生成带内嵌网页、明确标识为development的Windows包；后续主线集成直接取该包中的桌面EXE和worker，没有传入`--web-dir`。

```text
python -B scripts/project_env.py -- python -B scripts/release.py --target windows-amd64 --development-without-tools --skip-web-build --output .runtime/verify/windows-uav-auto-init/release
```

- 归档：`.runtime/verify/windows-uav-auto-init/release/qiku-0.1.0-windows-amd64-development.zip`，17,936,783字节。
- ZIP SHA256：`91b834dfd44f0010e5dc00be24ba155a7d795d7ff52be421f022e429d83c7291`。
- 包内`栖库.exe` SHA256：`9185bc7416a267c97ac9a0cef542170fc2dcc1914fa9559dc65705cbfa920920`。
- 包内`nas-collection-worker.exe` SHA256：`754e10224204b1ba97686ce42796ef93c2898447101b577a021e997eec498a89`。
- 解压位置：同一`release/extracted/qiku-0.1.0-windows-amd64-development/`。
- 归档24份文件校验通过，发布边界自测131项通过；包含PowerShell5.1/7中文空格参数透传和原schema边界。`release.json`仍为schema3、`developmentWithoutTools=true`，没有伪造已携带离线视频材料。

此包首次开启托管UAV会联网准备固定依赖，不是完整离线视频发行版。没有签名、上传、替换用户现有程序或提交Git。

## 2. 最终包真实联网与离线复用

```text
python -B scripts/project_env.py -- python -B scripts/verify_uav_bootstrap.py --binary .runtime/verify/windows-uav-auto-init/release/extracted/qiku-0.1.0-windows-amd64-development/栖库.exe --worker .runtime/verify/windows-uav-auto-init/release/extracted/qiku-0.1.0-windows-amd64-development/nas-collection-worker.exe --report .runtime/verify/windows-uav-auto-init/bootstrap-report.json
```

报告：`result=passed`，总102.676秒，首次成功联网准备65.732秒。长期脚本只在本轮随机隔离目录运行，并真正对程序目录施加只读ACL；profile含中文/空格，cwd另设，PATH只含System32，PYTHONHOME/PYTHONPATH故意污染，不继承站点凭据。

7组检查全部通过：

1. false、disabled、external及错误显式自定义Python均零自动安装。
2. 错误代理使真实下载失败，基础媒体仍ready，中文失败可重试，无current和半成品；不无限自动重试，手动重检可再次尝试。
3. 下载中停用等待取消和清理，无迟到激活。
4. 下载中正常退出等待清理，无未发布gen或stage残留。
5. 同一失败profile下次联网启动，实际从20份固定官方材料还原；stage完整校验、最终gen目录严格复验和真实worker探针通过，controller串行受控激活后，IPC及认证API均报告UAV3.8.4真实available。
6. 最终私有Python独立离线verify通过；真实FFmpeg编码H.264、ffprobe报告1.0秒、1880字节视频可完整解码；退出后所有EXE/DLL/PYD可零共享独占打开。
7. 再设错误代理离线重启，直接复用已发布gen；全部文件SHA、mtime及current原字节完全不变。

字节进度来自真实下载流，报告观察到下载/还原/完整校验/最终离线探测/自动载入及实际就绪阶段；没有用初始化ready代替worker能力。

## 3. 最终包兼容性和生命周期

- `scripts/verify_desktop_env.py`：最终包EXE及worker，11项通过。报告`.runtime/verify/windows-uav-auto-init/env-final-report.json`；覆盖.env读取、优先级、格式边界、真实worker、内部重检快照、开关覆盖、配额传递和日志不泄密。
- `scripts/verify_desktop.py --with-tray`：最终包内嵌网页模式通过。真实托盘和10尺寸图标资源检查通过；两次退出0.081秒/0.027秒；中文/空格/分号、不同cwd、端口避让、真实IPC、重复启动、数据锁、5GiB稀疏慢流中断、原位覆盖/摘要/整目录重命名、立即重启配置账号保留等通过。稀疏文件仅证明协议和退出行为。
- `scripts/verify_desktop_collection.py`：最终包双插件真实私有运行时矩阵25项通过，报告`.runtime/verify/windows-uav-auto-init/collection-final-report.json`。包含真实JMComic/UAV私有Python、只读ACL、显式解释器优先级、双向部分故障隔离、真实IPC、worker异常退出恢复、父强杀与启动取消、嵌套Job回收、独立哨兵保留、运行时映像替换及同profile重启；实际观察漫画26个、视频25个原生映像。早期开发二进制25项也通过，最终结果以本次包内EXE报告为准。

## 4. 针对性行为与独立审查

- runtime最初完整临时行为测试54.538秒通过，包含20真实缓存材料、ZIP/CRC/来源/下载错误、原子选择、复用、取消和有限重试；最初的长路径预算未覆盖最终位置加载，已明确纠正。
- 最终重定位回归：与主harness同深度中文空格profile，最终DLL248 UTF-16单元，真实CheckUAV、发布、Resolve和零新增下载复用通过；用例27.95秒，包29.352秒。见`research/runtime-implementation.md`和`.runtime/uav-bootstrap-final-relocation-tests.log`。
- desktop九组临时行为/真实WindowsJob回归、激活实际Reason保留回归通过；包括独立哨兵保留、进程/句柄屏障、旧状态所有权和命令门禁。见`research/desktop-implementation.md`。
- 独立trellis-check完整Initialize故障注入通过，包52.862秒；首次final probe失败不发布，旧current存在时失败/取消保留原字节及Resolve，候选精确清理。此为真实缓存库存的故障注入，不冒充第二次联网验收。
- 生成资源`--check`逐字节通过；20个锁定来源与506897字节payload未变化。审查修复构建下载临时文件所有权和失败重试残留，未降低生产哈希/来源检查。
- 本次新增的临时Go测试源码均已删除。复核原生路径诊断目录`.runtime/tmp/ur-f900a5tx`的删除被自动审核拒绝，遵从策略保留，没有换工具绕过；不影响产品或验收。
- 独立审查无剩余代码阻断项，详见`research/quality-review.md`；根因和预防见`research/native-path-analysis.md`。

## 5. 最终静态、编译与跨平台边界

- `gofmt -l`覆盖本轮12个Go文件，无输出。
- `go vet ./...`通过。
- `go test ./...`通过；删除临时行为测试后均为空套件，只计全仓编译，不计行为覆盖。
- release构建完成Windowsamd64的core、worker、GUI桌面；内嵌网页来自已有`web/dist`，本轮未修改TypeScript，也未把未执行的新前端构建记为通过。
- Linuxamd64/arm64的core和worker共4次`CGO_ENABLED=0`交叉构建通过；没有在Linux执行，也未启用该Windows专属安装功能。
- 5个变更Python脚本AST通过，相关`git diff --check`通过；LF/CRLF转换提示不作为失败。嵌入JSON使用局部`.gitattributes`锁定LF字节。

## 6. 未完成与下一步

- 本次实现和本机验收已完成，没有待修复代码阻断项。
- 干净Windows普通用户、真实视频站点下载、Linux/Docker/ARM运行时及正式分发许可/对应源码人工审查不属于本机通过证据，仍需独立验收。
- 用户需切换到新包才能使用此功能；保留现有.env和qiku-data。初次安装需网络，成功后会短暂受控重启本机服务，既有播放/下载连接可能中断。
- 任务不提交、不归档；既有无关历史任务归档/删除和其他工作树修改保持原状。
