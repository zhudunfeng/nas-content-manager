# UAV Windows 独立复核记录

日期：2026-11-26。复核阶段：Trellis Phase 2.2。复核覆盖运行时清单/来源、worker能力和诊断、桌面多插件环境及发布schema兼容；本记录只声明实际完成的检查。共享工作区中的实现未提交、未发布、未归档。

## Findings（已修复）

### UAV错误版本下载器被标记可用

- 文件：`internal/collectionworker/uav.go`。
- 问题：通用网页工具初始化只要求下载器返回非空版本，UAV能力也复用这一状态，导致不等于已验证固定版本的下载器仍通过UAV诊断。
- 修复：新增有中文说明的`uavYTDLPVersion = "2026.10.26"`；仅在UAV能力中要求版本精确一致，并提供中文版本不匹配原因。保留原通用网页能力的工具兼容规则。
- 验证：临时Go行为回归覆盖固定版本成功、空版本及三个近似/错误版本拒绝、失败时保留共享下载器/FFmpeg与原网页可用能力；`go test ./internal/collectionworker -run TestUAVFixedDownloaderVersion -count=1`通过。临时`uav_windows_quality_test.go`已经删除。桌面持久harness由worker实现者补入错误版本EXE场景并负责最终运行。

### 新下载器校验误拒绝旧schema合法工具包

- 文件：`scripts/verify_release.py`。
- 问题：原schema1/2的tools清单允许在必需的FFmpeg/ffprobe之外声明额外`tools/`文件。新增`verify_ytdlp`无条件把旧tools清单已声明的`tools/yt-dlp.exe`当作缺少schema3独立摘要而拒绝，改变旧完整包契约。
- 修复：旧schema1/2完整包先继续通过原tools清单的逐项摘要和成员闭包检查，再保留其中已声明额外下载器的语义；新增`licenses/yt-dlp/`保留区域仍不能无声明夹带。schema3继续要求独立下载器清单/摘要，不沿用旧tools声明绕过。
- 验证：持久发布self-test新增schema1与schema2各一个旧完整工具包正例、各一个下载器未被tools声明负例，以及schema3只有旧tools声明仍拒绝的负例。全量发布self-test由126增至131项，通过；仍含PowerShell5.1/7真实解析与中文空格argv透传。

## 真实UAV归档生产校验

实际使用`.runtime/tools/uav-private/`中的784份真实运行时/许可材料及一份清单，封装成785成员的仅运行时ZIP，直接调用生产`verify_release.verify_uav`。没有用PE占位夹具冒充真实UAV材料，也没有装配或声称完成yt-dlp正式完整发行材料。

命令：

```powershell
python -B scripts/project_env.py -- python -B .runtime/verify/uav-archive-quality-20261126/verify_archive.py
```

结果：4项通过。

| 场景 | 实际结果 |
| --- | --- |
| 真实完整运行时归档 | 785成员通过全部实际流校验及RECORD语义检查 |
| 修改RECORD并重算该成员、清单及发布摘要 | 拒绝：视频安装记录摘要或长度不一致 |
| 修改direct_url来源，同时重写RECORD和各层摘要 | 拒绝：视频固定源码安装来源不符 |
| 增加未声明运行时文件 | 拒绝：声明文件集合不一致 |

机器证据及复现驱动位于`.runtime/verify/uav-archive-quality-20261126/`，`result.json`记录各次归档的实际成员数、大小、SHA-256及拒绝原因。

- 输入运行时清单SHA-256：`5e4cc8298e7bbfc71c08a526c72b5494395ccf5ab2c5277b92bd04b2fa99d077`。
- 保留正例：`real-uav-runtime.zip`，40,668,930字节，SHA-256：`e0298d5a6c8b2731a32f2ed783eabeb92a7bc71ac62deca230f89bcc826f8ed7`。
- ZIP为运行时专项验证输入，缺少应用入口/下载器/媒体工具等正式发行材料，因此不是`verify_release.verify`完整发行包通过的证据。重编码后的清单摘要与原清单不同，各层均绑定归档内的实际字节。

## Verification

所有本地命令使用`python -B scripts/project_env.py -- ...`，未改全局环境。

| 检查 | 结果 |
| --- | --- |
| `go vet ./internal/collectionworker ./internal/desktop` | 退出0 |
| `go test ./internal/collectionworker ./internal/desktop` | 退出0；临时测试删除后为空测试集，计Go类型/编译检查 |
| `.tools/go/bin/gofmt.exe -l internal/collectionworker/uav.go` | 退出0，无未格式化文件 |
| `scripts/verify_release.py --self-test` | 131项通过 |
| `verify_release.py` AST与14个方法中文docstring/入参出参注解检查 | 通过；不是独立Python静态类型检查器 |
| 实际UAV归档生产校验 | 4项通过 |
| `git diff --check` | 退出0；只有仓库既有LF/CRLF转换提示 |
| 临时Go测试源码清理 | 已确认不存在 |

主流程已完成的全仓vet/test、前端TypeScript/Vite、三入口构建、Linux双架构交叉编译、运行时70/JM49及原基线见`implementation-validation.md`；此处不重复计数。UAV版本门槛改动后最终worker重建、102项UAV协议、受影响收集基线和桌面真实运行回归由主流程/桌面实现者继续记录。

## Findings（未修复）与边界

未发现需要产品决策才能修复的剩余代码缺陷；完整正式下载器材料及目标环境证据仍属交付门槛：

- 当前机器为Windows开发机。干净Windows11普通用户、真实公网站点、Linux实际执行、Docker、ARM64及实体NAS尚无本次独立验收证据。
- 完整yt-dlp官方EXE对应源码与组件材料、人工分发审查仍由维护者准备；没有自动填写通过声明，也不放宽装配/归档门禁。
- 运行时摘要证明材料一致性和已约束来源身份，不等同于独立数字签名或许可法律结论。

生产Go源已通知主流程稳定，可构建最终候选。文档/spec由主流程持有并同步旧schema工具兼容规则；本复核不接管提交、发布或任务归档。
