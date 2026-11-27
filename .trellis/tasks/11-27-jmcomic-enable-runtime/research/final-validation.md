# JMComic 启用策略对齐：主线验证

日期：2026-11-27（Asia/Shanghai）。本记录区分实际 Windows 联网/离线运行、缓存材料故障注入、编译与目标环境边界。用户指定参考线程为 `01a21b3d-49cd-7132-8dc5-4bb3e096698d`；本次在其 UAV 实现上增量交付，保留工作区原有大量未提交改动。

## 1. 实际交付与启用合同

- Windows amd64、native + managed、漫画明确启用且无显式自定义 Python 时，先选择有效随包依赖，再复用 profile 已验证代次，缺少时自动准备九份固定官方材料。仅漫画不安装 UAV、yt-dlp 或 FFmpeg。
- `internal/runtimebootstrap/` 共享原 UAV 安全安装内核；两个包装器各自绑定配方、严格加载器与库存。JM 使用 `collection-managed/jm-bootstrap`；UAV 的资源字节、`uav-bootstrap` 所有权和旧 current/generation 身份兼容。
- 下载、来源/版本/PE/完整库存、ZIP/CRC/路径预算、最终目录真实探针、原子 current 发布与精确清理边界保留。最终路径最大 248 个 UTF-16 单元；没有运行终端 pip 或修改全局 Python/PATH。
- 桌面按 JMComic → UAV 顺序准备。普通失败继续另一项，取消或清理失败停止整批；本轮结束后至多统一载入一次。显式 false、disabled、external、显式自定义 Python 均不自动安装。
- 原单对象 `collectionInitialization` 兼容保留，另有 `collectionInitializations` 按 kind 保存状态；真正就绪以 worker capability 和认证 API 为准。重检允许重试，失败不无限循环，停用/退出等待取消与自有 Job 清理。
- 独立审查修复批次完成与媒体退出同时就绪的激活窗口：pending `serviceDone` 保留给生命周期分支，不再允许自动激活；当前插件门禁、排队用户操作和 worker 退出同样阻止过期结果。

## 2. 最终包与证据对应

交付归档：`.runtime/verify/jmcomic-enable-runtime/delivery/qiku-0.1.0-windows-amd64-development.zip`。

| 产物 | 字节数 | SHA256 |
| --- | ---: | --- |
| ZIP | 18055271 | `66f6ee69a89fbba1b736e613d288e497d9704ce61fac1b86a30ac85d71f3af56` |
| 栖库.exe | 17332224 | `0b6f8a3b38ef9073768c21305b5d0e39119499d632eb34915a0cac6c1beb1f1d` |
| nas.exe | 15581696 | `9df41d9421312c46f04220db0f2fe96b7611060b4ee0d61a6b771b71d5dea90d` |
| nas-collection-worker.exe | 8721408 | `754e10224204b1ba97686ce42796ef93c2898447101b577a021e997eec498a89` |

最终行为验收使用 `final-release/extracted/qiku-0.1.0-windows-amd64-development/` 中的 EXE。之后只修订随包离线说明和 `docs/release.md` 并重新打包到 `delivery/`；已逐字节核对最终三个 EXE 与受测候选完全一致。交付包的十二份文档、`.env.example` 与当前源码一致，说明明确已验证依赖可离线复用、首次完全离线需要完整包。两份归档均通过真实 24 文件校验。

构建命令（以下所有本地命令均通过项目环境入口，缓存/临时目录限定在仓库 `.runtime/`）：

```text
python -B scripts/project_env.py -- python -B scripts/release.py --target windows-amd64 --development-without-tools --skip-web-build --output .runtime/verify/jmcomic-enable-runtime/delivery
python -B scripts/project_env.py -- python -B scripts/verify_release.py .runtime/verify/jmcomic-enable-runtime/delivery/qiku-0.1.0-windows-amd64-development.zip
```

发行身份仍为 schema 3 开发验证包，不是预装完整离线媒体发行；程序含既有 `web/dist` 的内嵌网页。本轮没有修改前端业务代码，没有将未执行的新 Vite 构建声称通过。

## 3. 最终 EXE 的真实联网与离线验收

命令的 `<包>` 指上述 `final-release/extracted/qiku-0.1.0-windows-amd64-development`：

```text
python -B scripts/project_env.py -- python -B scripts/verify_jmcomic_bootstrap.py --binary <包>/栖库.exe --worker <包>/nas-collection-worker.exe --report .runtime/verify/jmcomic-enable-runtime/final-bootstrap-report.json --with-uav
python -B scripts/project_env.py -- python -B scripts/verify_uav_bootstrap.py --binary <包>/栖库.exe --worker <包>/nas-collection-worker.exe --report .runtime/verify/jmcomic-enable-runtime/final-uav-regression-report.json
python -B scripts/project_env.py -- python -B scripts/verify_desktop_env.py --binary <包>/栖库.exe --worker <包>/nas-collection-worker.exe --report .runtime/verify/jmcomic-enable-runtime/final-env-report.json
python -B scripts/project_env.py -- python -B scripts/verify_desktop.py --binary <包>/栖库.exe --with-tray
```

- JM 自动准备 **7 组通过**，整轮 207.713 秒；单漫画真实准备 28.167 秒，空 profile 双插件真实准备 108.820 秒。实际只复制桌面和 worker 两个 EXE 至施加写入拒绝 ACL 的程序目录；中文/空格 profile、异 CWD、System32-only PATH、污染 PYTHONHOME/PYTHONPATH 下，实际九份漫画材料联网还原、worker JMComic 2.7.12/认证 API 及最终目录真实图像/AES/CA 探针均通过。
- 漫画 false/disabled/external/错误显式 Python 零安装；错误代理下载失败保持基础媒体 ready，无 current/stage/gen 半成品，无无限重试，主动重检再次尝试。实际观察到下载字节后停用和退出，清理等待、无迟到激活及映像释放通过。
- 单漫画离线重启的全部文件 SHA、mtime、current 原字节不变，目录不含视频工具。双插件分别严格校验成功，日志媒体 ready 计数证明初次仅一次自动载入；离线重启直接复用，未额外自动重启。
- 原 UAV 自动准备 **7 组回归通过**，整轮 144.243 秒，成功联网准备 92.354 秒；20 份真实材料、最终目录探针、错误代理/门禁/取消/清理/离线字节不变均通过。真实 H.264 编码 1880 字节、ffprobe 时长 1.0 秒及解码通过；短样本不代表完整视频性能。
- `.env` 回归 **11 项通过**，包含优先级、格式/不可读/上限、真实 worker 开关与配额、启动快照重检、中文/空格参数和日志秘密保护。此组无私有 Python，不能代替前两组真实依赖验收。
- 主桌面生命周期和真实托盘通过；10 尺寸图标一致，内嵌网页、中文/空格/分号、异 CWD、端口避让、真实 IPC、重复实例/数据锁、5 GiB 稀疏慢流中断、EXE 原位覆盖/摘要/目录重命名、账号配置保留和源文件不变通过。两次退出 0.412 秒、0.096 秒，退出后 EXE 可立即独占访问。
- 本轮完整双插件私有运行时生命周期矩阵 **25 项通过**，报告 `.runtime/verify/jmcomic-enable-runtime/final-collection-report.json`。实际观察漫画 26 个、视频 25 个原生映像；真实私有 Python、只读程序 ACL、显式解释器优先级、双向部分故障隔离、真实 IPC 启停/重检、worker 异常退出恢复、父强杀、启动取消、嵌套 Job 回收、独立哨兵保留、运行时映像替换和同 profile 立即重启均通过。使用同一最终候选 EXE/worker、`.runtime/tools/{jmcomic-private/jmcomic-runtime,uav-private/uav-runtime}.json`，固定 `.runtime/tools/uav-windows/yt-dlp.exe` 与 `.runtime/verify/windows-uav-local-repair/stage/tools/ffmpeg.exe`，未请求站点。

首轮较早候选也通过两套自动准备脚本，报告为 `bootstrap-report.json` / `uav-regression-report.json`；它们不替代上面的最终候选结果。

## 4. 资源、行为故障注入与独立审查

- `jmcomic_bootstrap.py --runtime-manifest .runtime/tools/jmcomic-private/jmcomic-runtime.json --check`：真实 Python 3.14.4 漫画 probe 通过，9 份材料和 51675 字节 payload 逐字节确定性一致。
- `uav_bootstrap.py --runtime-manifest .runtime/tools/uav-private/uav-runtime.json --check`：真实视频 probe 通过，原 20 份材料和 506897 字节 payload 不变。
- 共享引擎临时行为测试 52.511 秒、追加 CRC/保留清单名边界 2.908 秒通过。真实缓存还原 JM 620 文件和 UAV 791 文件，各自在最终目录调用实际 worker 探针并零请求复用；首次 probe 失败、替换失败/取消保留旧 current、精确清理、ErrCleanup、来源/下载/ZIP/路径拒绝等通过。此处缓存 transport 不是首次互联网获取证据。见 `research/runtime-implementation.md` 与 `.runtime/runtimebootstrap-{temporary-tests,boundary-final-tests}.log`。
- 桌面 11 组针对性临时行为测试 0.420 秒通过，覆盖门禁、双向状态/环境隔离、失败继续、清理/取消屏障、迟到进度、故障原因保留、一次激活、用户命令/媒体退出/当前资格优先与 stop 等待。见 `research/desktop-implementation.md`。
- 独立 trellis-check 已复核共享引擎、桌面与脚本，无未修复具体问题；四包 vet/编译/格式、四脚本 AST、`vue-tsc --noEmit` 通过。行为测试引用实现方证据，未冒称审查者重跑。见 `research/quality-review.md`。
- 本轮生成的临时 Go 单元测试源码已删除，受影响目录无 `_test.go` 残留。尝试 `-race` 因当前 CGO 禁用而未运行成功，未计为通过。

## 5. 最终静态与跨平台检查

- `go vet ./...`、`go test ./...` 返回 0。当前空测试套件仅计全仓编译证据，行为覆盖来自上面已执行并记录的临时测试和长期集成脚本。
- `gofmt -l` 覆盖共享内核、两套包装器和桌面目录，无输出。
- 五个本轮涉及 Python 脚本 AST 通过；其中四个 bootstrap/验收脚本另核对函数类型注解及中文 docstring。相关 `git diff --check` 返回 0，只有仓库 LF/CRLF 转换提示。
- 发布边界自检 **131 项通过**，含 Windows PowerShell 5.1/7 解析和中文/空格参数透传；交付归档真实 **24 文件**校验通过。
- Windows amd64 桌面/core/worker 实际构建通过。Linux amd64/arm64 core 与 worker 共 **4 次** `CGO_ENABLED=0` 交叉构建通过，输出 `.runtime/verify/jmcomic-enable-runtime/cross-build/`；没有在 Linux 执行该功能。
- Trellis 当前任务 implement/check context 校验通过。

## 6. 边界与后续

- 本次实现和上述本机验收已完成，没有待修复具体代码问题；任务进入 review，等待用户审阅/提交。
- 没有请求漫画或视频站点，没有证明站点可达、账号权限或实际内容下载；没有执行 Docker、Linux/ARM 运行时或干净普通用户 Windows 验收。
- 固定来源与摘要的机器校验不代表已完成正式发行的许可/对应源码人工审查。开发包也不宣称完整离线媒体发行。
- 切换到新程序才有新启用策略；保留已有 `.env` 和整个 `qiku-data`。首次准备成功统一载入会短暂重启本机服务，可能中断播放/下载连接，不自动重发业务任务。
- 未修改用户既有安装目录、系统 Python/PATH、用户级环境或数据库 schema。任务不提交、不归档；目标验收由上述边界单独安排。

长期合同来源：`.trellis/spec/backend/{jmcomic-windows-contract,uav-windows-contract,cross-platform-contract}.md`；用户入口：`docs/{jmcomic-windows,uav-windows,windows,release}.md`。
