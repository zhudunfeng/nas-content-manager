# Windows 私有漫画运行时与发行实现记录

日期：2026-11-26。范围只含运行时装配、Go 静态加载和发行归档，不代替桌面／worker 生命周期报告。

## 实际产物

- 清单：`.runtime/tools/jmcomic-private/jmcomic-runtime.json`。
- 解释器：`.runtime/tools/jmcomic-private/runtimes/jmcomic/python.exe`。
- 已验证组合：Windows x64 CPython **3.14.4**、`jmcomic==2.7.12` 及原 `deploy/jmcomic-requirements.txt` 固定的八项依赖闭包。
- 清单声明 620 份运行／许可材料，合计 **50,123,069 字节**；全部原样保留 CPython DLL/PYD、标准库 ZIP、wheel 包数据、certifi CA 和 `.dist-info`。
- 默认官方输入：`https://www.python.org/ftp/python/3.14.4/python-3.14.4-embed-amd64.zip`。
- 默认输入 SHA256：`cda80a9b1e75c0f1b4f9872ca1b417f0d19bce32facc811aea9180e70fad5fb9`。取自同一路径 `.zip.sigstore` 的 `messageSignature.messageDigest.digest`，并对实际下载字节核对；**没有执行 Sigstore 签名身份验证**。
- 依赖锁摘要采用统一 LF 换行后的字节，固定值 `5132b6045555e13f489d4c28f14b11145e5d2a22dd1519cc7a5273ad65eddc07`。工作区 requirements 为 CRLF、Git blob 为 LF，不能直接使用平台工作区原始字节，否则相同依赖会被误判漂移。

## 文件与接口

- `internal/jmcomicruntime/{runtime.go,path_windows.go,path_other.go,contract.json}`：公开 `Load(manifestPath string) (Runtime, error)`，仅 Windows amd64 激活。返回规范绝对 `Python`，同时提供 `Root/PythonVersion/JMComicVersion`；不启动 Python。
- `scripts/jmcomic_runtime.py`：`assemble --output <全新目录>`、`verify <绝对清单> [--probe]`。构建端可下载固定来源，支持 `--offline`、`--wheelhouse`、`--python-archive`；终端不 pip 安装、不改 PATH、不复制开发 venv。
- `scripts/release.py`：所有目标纳入 `nas-collection-worker`。可选 `--jmcomic-runtime-manifest` 只接纳 Windows amd64，复制后再次验证。新 `release.json` schema2 要求 `jmcomicRuntime` 为 `null` 或含 `manifest/sha256/pythonVersion/jmcomicVersion` 的摘要；带运行时归档加 `-jmcomic`，`-development` 独立表示缺少 FFmpeg。
- `scripts/verify_release.py`：保留 schema1 的原校验，新增 schema2 worker／主入口架构及漫画材料一致性校验，直接读取归档流，不解压或执行不可信文件。漫画基础包拒绝夹带未声明的运行时树／清单。
- `scripts/verify_jmcomic_runtime.py`：可复用集成验证；临时 Go 校验源、结构夹具和真实运行时迁移副本全部位于受控 `.runtime/verify/` 临时根，执行后清理，只保留结果 JSON。

## 静态边界

清单固定名称 `jmcomic-runtime.json`、schema1 和 `windows-amd64`；严格拒绝重复／大小写别名／额外／缺失／null 字段。校验编译期版本与允许 wheel 摘要、官方来源、来源闭包、每组件非空许可、必需解释器／标准库／证书／元数据／本机扩展、文件大小与 SHA256、每份 EXE/DLL/PYD 的 PE x64 标识及精确 `_pth`。

目录枚举仅覆盖 `runtimes/jmcomic/` 与 `licenses/jmcomic/`，拒绝未声明文件、路径穿越、ADS、尾点／空格、设备名（含 `CONIN$`、`COM¹` 等别名）、符号链接／Windows 重解析点。无关媒体工具可以存在于程序根，避免错误地把整个发行目录视为 Python 文件白名单。额外 `.pth`、源码旁 `.pyc` 不获接纳；官方标准库 ZIP 内的原始字节码正常保留。

`python314._pth` 精确内容为 `python314.zip`、`.`、`Lib/site-packages` 三行；不启用 site。固定探测使用 `-I -B -X utf8`，检查实际依赖版本、隔离模式、UTF-8、CA 内容、Pillow PNG 编解码、Crypto AES 和 YAML；禁用 socket 网络入口，且不输出未过滤依赖 stderr。

## 执行与结果

以下命令均通过 `python -B scripts/project_env.py -- ...` 执行，缓存／构建／临时文件在仓库 `.runtime/`。

1. `python -B scripts/jmcomic_runtime.py assemble --output .runtime/tools/jmcomic-private`：实际下载并核验官方 embeddable ZIP，下载八份 hash 锁定 Windows wheel，装配成功，真实离线探测通过。
2. `python -B scripts/jmcomic_runtime.py verify E:/vscodeProjects/nas-content-manager/.runtime/tools/jmcomic-private/jmcomic-runtime.json --probe`：620 文件与真实依赖通过。
3. 临时 `TestPrivateRuntime` 执行真实 `jmcomicruntime.Load` 成功，返回规范包内解释器（约 1.03 秒）；临时单元测试源码已删除。
4. `python -B scripts/verify_jmcomic_runtime.py --manifest E:/vscodeProjects/nas-content-manager/.runtime/tools/jmcomic-private/jmcomic-runtime.json`：**49 项通过**。Python 与实际构建 Go 加载器对相同夹具交叉判定；覆盖严格 JSON、版本／依赖锁／来源、摘要、缺失证书／元数据／许可／扩展、Windows 特殊设备别名、实际 junction、未声明文件、错误架构、被篡改 `_pth`。实际运行时复制到中文／空格目录后，从任意 CWD、空 PATH、污染 PYTHONHOME/PYTHONPATH 条件校验 Go 加载，执行真实隔离依赖探测。结果：`.runtime/verify/jmcomic-runtime-result-8e11771ee46e490dabb4814d1320c2e4/result.json`。
5. 同套集成验证中，结构发行夹具正常归档通过；分别重新计算外部摘要、包内摘要及运行时记录／摘要，仍拒绝错误 PE 和允许 site 的 `_pth`，避免把仅有传输校验失败当作语义校验。
6. 用已缓存官方 ZIP／wheel／来源记录 `assemble --offline` 重建，并在进程内把 `urllib.request.urlopen` 替换为必定抛错的入口：构建及真实探测通过，重建清单 SHA256 与首次产物完全一致，临时副本已清理。
7. `python -B scripts/verify_release.py --self-test`：**91 项通过**，包括既有 schema1、安全路径、中文非 UTF-8 控制台、工具／许可／源码／镜像材料、确定性 ZIP/tar，以及 schema2 Windows／Linux worker 结构与架构、缺少 worker、未声明运行时和旧格式兼容。
8. `go vet ./internal/jmcomicruntime`、`go test ./internal/jmcomicruntime`、运行时包 `GOOS=linux GOARCH=amd64|arm64 CGO_ENABLED=0 go build` 通过。保留的 Go 测试集为空，`go test` 本身仅是编译证据；真实行为来自上述临时测试与可复用交叉验证。
9. 四份 Python 文件 AST 语法检查、作用域 `git diff --check` 通过；没有新增保留的临时单元测试源码。

协同 worker 实现者另用此真实解释器执行原固定桥接离线套件，通过真实 JMComic 搜索／元数据／解码／失败／进度／页面复用测试，但替换了站点传输且网络调用数为零。记录：`.runtime/verify/jmcomic-real-bridge-5bcfd09caa494461aa124ff01bc0eb1f/report.json`。重新编译包含 LF 契约的 worker 后，真实清单诊断与托管 capability 通过；旧二进制含旧契约时的明确失败已定位，不是运行时依赖导入失败。

## 仍需独立验收

- 最终完整应用归档、桌面托管及所有生命周期回归由主线统一构建验证，本分支没有并发操作 `internal/webassets/dist`，也没有把结构夹具称为真实发行程序运行。
- 当前结果来自本机 Windows x64 开发环境，未取得干净 Windows 11 普通用户／无全局工具虚拟机验收、ARM64 或其他 Windows 版本验收。
- 未访问真实漫画站点，离线探测或 capability 不表示远端可用。
- 保留许可材料不等于完成所有第三方许可／再分发义务的人工作业；未审核 FFmpeg 许可材料，缺少工具的 development 包不能称为完整离线媒体发行。
- 文件摘要与官方源记录保证受信发行包的一致性，不是发布者数字签名、恶意代码审计或干净系统 DLL 兼容性的替代品。3.11–3.14 版本形式可验证，不表示每个解释器／wheel 组合均已执行验收；实际验收仅为 3.14.4 x64。
