# Research: Windows 私有 Python 与 JMComic 分发方式

- Query: 在现有 Go worker 调用固定 Python 桥接的架构下，Windows 原生如何分发 JMComic，兼顾普通用户、DLL／wheel、编码、路径与更新？
- Scope: mixed；读取当前源码、真实已安装锁定依赖元信息与官方文档，仅做方案研究。
- Date: 2026-11-26（Asia/Shanghai）。
- Task: 明确受派写入本规划任务。`.trellis/.current-task` 仍指向上轮 `11-25-jmcomic-advanced`，本研究没有改变或写入旧任务。

## Files found

| 路径 | 当前实际职责与定位 |
| --- | --- |
| `internal/collectionworker/config.go:61-73,193-214` | 可选启用参数；Windows 默认 `python`；固定工具解析为绝对 EXE |
| `internal/collectionworker/jmcomic.go:25-29,62-80,97-119` | 固定 `2.7.12`、Go embed 桥接、一次性 probe、无 shell 的 `-I -B -c` |
| `internal/collectionworker/tools.go:91-101,105-129` | 操作目录、临时目录、受控环境与代理；有界 stdout／stderr |
| `internal/collectionworker/process_windows.go:18-21,36-105` | 隐藏窗口、挂起后归入 Windows Job、取消／结束回收自有进程树 |
| `internal/collectionworker/jmcomic_bridge.py:120-135,421-450` | 导入真实依赖、读取 `importlib.metadata.version`、字节输入与唯一 JSON 输出 |
| `deploy/jmcomic-requirements.txt:1-93` | 八项基础依赖的精确版本及平台 wheel 哈希白名单 |
| `scripts/release.py:299-320` | 目前构建主服务／桌面 EXE，打入媒体工具与文档；此处未打包原生 worker 或漫画 Python |
| `docs/jmcomic.md` | 原生管理员手工建立 venv、安装锁定依赖并单独配置 worker |
| `.trellis/spec/backend/cross-platform-contract.md` | 包内工具、共享启动关闭、平台隔离与真实验收边界 |
| `.trellis/spec/backend/jmcomic-contract.md` | 固定协议、可选能力、站点／代理／输出不由客户端控制 |

定位时首先使用已有 CodeGraph；`jmcomicCommand` 未命中，随后读取实际文件校正定位。没有索引重建或产品改动。

## Findings

### 1. 已存在的 Windows 证据与缺口

本次通过项目环境入口，只读运行 `.runtime/tools/jmcomic-venv/Scripts/python.exe -I -B` 查询元信息，观察到 **CPython 3.14.4、AMD64**，八项版本均与锁文件一致：

| 包 | 已安装精确版本 |
| --- | --- |
| jmcomic | 2.7.12 |
| curl-cffi | 0.17.0 |
| pillow | 13.0.0 |
| pycryptodome | 4.6.0 |
| pyyaml | 6.0.4 |
| cffi | 2.1.1 |
| certifi | 2026.11.20 |
| pycparser | 3.12 |

这只是当前开发机完整 Python／venv 的已安装事实，不是这些版本“最新”、embeddable 已验收或干净 Windows 已通过的声明。

从已安装分发文件清单实际观察到 `curl_cffi.libs/libcurl-impersonate-*.dll`、`curl_cffi/_wrapper.pyd`、`PIL/_imaging.cp314-win_amd64.pyd`、`yaml/_yaml.cp314-win_amd64.pyd`、`_cffi_backend.cp314-win_amd64.pyd` 与大量 `Crypto/**/*.pyd`。因此运行包必须处理 native DLL／扩展、数据文件与 ABI，不能仅复制 `.py`。当前开发 venv 中没有 `._pth` 文件，不能将此环境运行成功外推为私有 embeddable 的模块搜索配置正确。

### 2. 官方文档事实

- **[R1] 私有 Python 可行**：官方 Windows embeddable 包适用于随应用分发私有解释器；不含 pip，普通 Python 安装式的 pip 依赖管理不受该分发支持。第三方包应由应用侧随版本交付。
- **[R2] venv 不是通用便携包**：官方将其视为可重建环境，移动时通常应在目标位置重建。
- **[R3] 搜索路径必须明确**：`._pth` 可限定 `sys.path`；其存在时忽略注册表／环境路径，默认不加载 site，路径可相对该配置文件。
- **[R4] 编码不应只靠环境变量**：`-I` 忽略 `PYTHON*` 环境设置；命令行 `-X utf8` 可显式开启 UTF-8。
- **[R5] 冻结可选但有运行区别**：PyInstaller one-file 会先解压支持文件到临时目录，比 one-folder 多启动开销；官方建议先调通 one-folder。
- **[R6] 冻结要声明资源**：spec 的 `binaries`、`datas`、`hiddenimports` 分别控制二进制依赖、数据及动态导入收集。

以上为一手文档事实；下表与建议是结合当前项目的工程推断，尚未做发行样机验收。

### 3. 三种方案对比

| 方案 | 适合用途 | 对当前实现的影响 | 主要代价 |
| --- | --- | --- | --- |
| 管理员现有 Python + 目标机新建 venv／PowerShell 脚本 | 开发、服务器运维、快速补齐手工流程 | 保留现有解释器参数和 `-I -B -c`，脚本核验唯一受支持版本及 wheel 哈希，显式绝对路径 | 终端仍需 Python／安装条件；不能把开发 venv 原样移机作为便携发行 |
| **随包私有 CPython + 固定 site-packages（推荐桌面发行）** | Windows 11 x64 普通用户，无 Docker／全局 Python | 保留固定桥接与 JSON 协议；绝对路径调用包内解释器；构建机准备依赖闭包及 `_pth` | 包体较大，需要维护运行时清单、native 文件、许可证和成品 smoke test |
| 冻结漫画桥接 EXE（先 one-folder） | 后续有明确“只分发桥接程序”诉求 | Go 需区分解释器与桥接可执行入口；不能向冻结 EXE 原样传 `-I -B -c`；新增 spec／hooks／资源声明 | DLL／动态导入／dist-info／证书收集与更新多一条链；one-file 每次调用还增加临时解压和关闭检查 |

**收敛建议**：管理员脚本作为过渡和高级模式保留；面向桌面普通用户采用随包私有 CPython。冻结 EXE 作为后续比较实验，不作为本轮 Windows 兼容的必要前提。无需在 Go 进程内嵌 CPython，也不需要重写现有下载与发布逻辑。

### 4. 推荐运行时交付约束

1. **只在构建端安装**。选定唯一 CPython 补丁版本、Windows x64 架构和 ABI。可先采用与已有 Windows 离线证据一致的 3.14 系列作候选，再通过官方 embeddable 与 wheel 组合构建验证后冻结补丁号；不能从“锁文件接纳 3.11–3.14”推断每个组合已经跑过。终端不调用 pip、不依赖网络补包、不写全局 PATH。
2. **构建固定闭包**。在受控构建解释器中验证锁文件、用目标 ABI 的 wheel 组装私有 `Lib/site-packages`；完整保留 `.dist-info`、证书和 native 文件。桥接的 `importlib.metadata.version("jmcomic")` 依赖元数据，不能为了减包删掉；certifi 的 CA 数据也应进入清单。通过独立探测、实际导入和离线解码检验 DLL／VC 运行库前提，不用开发机已安装运行库掩盖缺失。
3. **限定模块搜索路径**。随运行时生成仅指向包内标准库、扩展及 `Lib/site-packages` 的 `_pth`；优先不启用 site。若某个经过锁定的分发确实需要 `.pth`，须单独研究和白名单，不能为“能导入”而加入用户／工作目录。不得以 `PYTHONPATH` 解决私有运行时导入失败。
4. **显式编码**。现有 `toolEnvironment` 写入 `PYTHONIOENCODING=utf-8`，但 `-I` 会忽略它。协议正常输出已使用 `ensure_ascii=True`，输入以 bytes 交给 JSON，不应声称当前必然乱码；风险在中文错误和后续文本路径。建议统一 `-I -B -X utf8`，或在可信固定桥接内明确标准流编码，并补 CP936／西文代码页的错误分支验证。
5. **路径与运行时身份**。由程序目录解析包内相对位置再转成绝对路径，不能依赖 CWD。只接受当前平台、固定版本、清单内相对路径和 SHA256；明确管理员解释器覆盖优先级，显式配置无效时给出原因，不悄悄切换到任意 PATH Python。保留 worker 进程所属的目录和现有 Job 回收方式。
6. **只读安装目录与可写状态分离**。Python 与依赖留在安装包；缓存、临时目录、检查点、下载及日志仍在当前 profile／worker 数据根。继续 `-B`，不让升级依赖覆盖 staging 或库目录。成品移动到中文／空格目录、从不同 CWD 启动必须复测。
7. **成套更新与回滚**。记录解释器／ABI／依赖锁／worker 协议和各文件摘要。停止本实例 worker 及 Python 后整体替换程序运行时，不覆盖 profile／数据库／页面检查点；旧数据记录是否兼容仍依既有协议核验，不能只看 jmcomic 版本号。缺少或不兼容运行时只禁用漫画可选能力。

### 5. 冻结 EXE 额外要求

若将来选择冻结，先 one-folder 以方便核对 native 文件。必须验证 `jmcomic` 动态导入、curl DLL、Pillow／Crypto 扩展、certifi 数据和 `importlib.metadata` 所需的发行元数据；将固定 JSON 入口编译为专用 launcher，不增加任意脚本参数。one-file 还需要验证取消后临时解压目录和 DLL 占用完全释放，不能把关闭主进程当作结束。PyInstaller 文档 v6.16.0 在此仅为打包行为的一手参考，**不表示已选择该版本，更不表示它已支持或验收 CPython 3.14.4**。

### 6. 建议验收

- 干净 Windows 11 x64 普通用户、无全局 Python／Go／Node／Docker：解压后私有运行时探测成功，缺包时中文可恢复，不影响基础媒体能力。
- 完整包成品执行真实固定依赖的离线搜索／选章／解码／页面复用 harness；真实元数据来源与图片传输可替换，但不是冻结空壳返回假成功。
- 清空或污染 PATH、设置错误 `PYTHONPATH`／`PYTHONHOME`、CP936 与重定向标准流、中文空格路径、只读程序目录、任意 CWD。
- 下载中取消、托盘退出、子进程继承句柄、立即覆盖运行时目录后重启；只回收当前实例，不关闭其他 Python。
- 对最终归档解压内容重新核 SHA256、native 架构和许可证，验证损坏／缺少 DLL／元数据时诚实报告；不以源码目录运行通过代替成品包。

## External references

查阅日期均为 **2026-11-26**。Python 使用明确 3.14 文档，不宣称最新；PyInstaller 使用官方仓库固定 v6.16.0 的文档源码。官方站点 `pyinstaller.org/en/stable/` 本次直读返回 HTTP 403，因此改读同项目官方 GitHub，而非采用不明转载。

- [R1 — Python 3.14: Using Python on Windows / The embeddable package](https://docs.python.org/3.14/using/windows.html#the-embeddable-package)
- [R2 — Python 3.14: venv](https://docs.python.org/3.14/library/venv.html)
- [R3 — Python 3.14: sys.path initialization / _pth files](https://docs.python.org/3.14/library/sys_path_init.html#pth-files)
- [R4 — Python 3.14: Command line and environment](https://docs.python.org/3.14/using/cmdline.html)
- [R5 — Official PyInstaller v6.16.0 operating-mode source](https://github.com/pyinstaller/pyinstaller/blob/v6.16.0/doc/operating-mode.rst)
- [R6 — Official PyInstaller v6.16.0 spec-files source](https://github.com/pyinstaller/pyinstaller/blob/v6.16.0/doc/spec-files.rst)

## Related specs

- `.trellis/spec/backend/cross-platform-contract.md`
- `.trellis/spec/backend/jmcomic-contract.md`
- `.trellis/spec/backend/collection-contract.md`

## Caveats / Not Found

- 本轮未下载或构建 embeddable、未执行 pip 安装、未运行 PyInstaller，也没有新安装包／DLL 依赖扫描／干净虚拟机证据；推荐只是可实施方案。
- 未验证真实站点、账号、Docker、Windows ARM64 或其他 Windows 版本，不将当前 AMD64 开发 venv 推广为目标支持矩阵。
- 运行时随包交付不能解决桌面是否代管 worker 的生命周期、共享目录、鉴权与配置接线；这些仍由主线方案明确。将 Python 安装在应用机器上不能自动赋能另一台远程 worker。
- 当前 Go 以命令行 `-c` 传固定桥接；源码实测为 21,915 字符／24,407 UTF-8 字节。未发现当前已超 Windows 命令行限制的证据，但后续脚本增长、长路径应纳入总长度边界，不能无限往启动参数追加配置。若改为短可信引导／私有脚本，必须保留固定来源、摘要、目录所有权和原 JSON 输入约束。
- 当前所有数据查询均只读，唯一写入是本研究报告，未改配置、业务源码或旧任务。
