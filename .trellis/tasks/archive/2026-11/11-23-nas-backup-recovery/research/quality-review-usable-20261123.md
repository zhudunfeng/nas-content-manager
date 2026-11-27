# 2026-11-23 可用开发包轻量独立检查

结论：本轮指定范围内未发现阻断。可用基线保持已经独立验收的 S03.2.2；三份 S03.2.3 未完成草稿已精确保存在非编译目录，未把执行器、手动持久 API 或后续调度功能计为交付。

本报告遵循主线授权的轻量检查边界，只新增本文与 `.runtime/verify/usable-20261123-review/` 中自有工具/证据。没有产品修复，没有修改正式 spec/docs/task、共享 CodeGraph 索引、成品包或解压目录；未提交、部署、归档、操作 Docker 或用户实例。主线独占解压及产品运行回归，本文没有重复运行。

## Findings (fixed)

- 无。本轮对草稿隔离、源码、包及静态门禁的核对均通过，不为制造修改而写入产品。

## Findings (not fixed)

- 无本轮范围内待修阻断。S03.2.3 未完成项、开发包不含 FFmpeg、真实 Linux/NAS、介质故障、崩溃/断电及真实托盘交互仍是明确交付边界，不改写为通过。

## 草稿隔离与源码

已完整读回 `research/s03-2-3-draft-checkpoint.md`，独立读取三份保存文件并重算字节及 SHA256；对应 `internal/app/` 下原 `.go` 均不存在。

| 非编译保存文件，均位于 `.runtime/verify/s0323-draft-pause/` | 字节 | SHA256 |
| --- | ---: | --- |
| `backup_api.go.txt` | 14,742 | `1803a2b5ea48ef036be051a3deaca2feaffcab172d01244c877cd0ff8c6599de` |
| `backup_state.go.txt` | 21,296 | `1945bea267af17bb18d40c781e59b1dfd851a1d1e391d671d84e5d97c8f7ab26` |
| `backup_mutation.go.txt` | 10,575 | `89b13487d89cc4f5bdd0833b12d7d87814e0a9ea24d7ecc43453ce6ae282f609` |

对上片最终 `.runtime/verify/s0322-review-checks-final/product-sources.sha256` 的 193 项逐项重算，漂移 0；清单自身 SHA256 为 `0dd6d62aa89118b7e40e6291b23f38d9ea95780eb220f01dac2471734c08d199`。另外独立枚举当前 `cmd/`、`internal/` 的 127 份 Go 源，与清单 Go 子集一致，新增/缺失 Go 源均为 0，没有本轮临时 Go 单测源。

## 开发包只读核对

包为 `bin/releases/usable-20261123/qiku-0.1.0-windows-amd64-development.zip`；13,434,468 字节，SHA256 `63fae8eda5f9b5b3e693350951ecbd25a9b84f0104b0309f93b82ae33bb72c74`。独立读取 ZIP 的 12 个文件，CRC 通过；无重名、链接或危险路径。11 条 `checksums.sha256` 精确覆盖除清单自身以外的成员，全部摘要匹配；12 个成员均与主线已解压目录逐字节相同，无多余或缺失文件。未执行解压。

| 包内产物 | 字节 | SHA256 |
| --- | ---: | --- |
| `nas.exe` | 15,505,920 | `8b5f58c721573387c9133134ada1e3fd79249db329b27deec7aa4e465579156e` |
| `栖库.exe` | 15,735,296 | `ba0c2a5cb3afdf278f710cfd4ea1e15aa78ee2053dff71adbcdf10d093258220` |

两份产物均为 Windows AMD64 PE32+；`nas.exe` 为 Console 子系统，`栖库.exe` 为 Windows GUI 子系统。只读 `go version -m` 确认分别来自 `cmd/nas`、`cmd/nas-desktop`，Go 1.27.4，`-tags=webembed`、`-trimpath=true`、`CGO_ENABLED=0`。当前 `web/dist` 的 12 份 HTML/JS/CSS/SVG 完整字节均可在两份 EXE 中找到；这证明包内嵌了当前构建资源，不代替浏览器行为测试。

`release.json` 的 `developmentWithoutTools=true`、`tools=null` 与文件集合相符；无 FFmpeg/ffprobe 二进制或 tools manifest，`开始使用.txt` 明确说明为未携带 FFmpeg 的开发验证包。可用预览包不能描述为完整离线运行时或完整 NAS 环境验收。

## Verification

- Lint：pass。固定 `.tools/go/bin/` 的 `go vet ./...` 退出 0；`gofmt -l` 检查 127 份 Go 源，退出 0 且输出为空。`git diff --check` 退出 0，仅既有 LF→CRLF 提示。
- TypeCheck：pass。`node web/node_modules/vue-tsc/bin/vue-tsc.js --noEmit -p web/tsconfig.json` 退出 0；Go vet 与清源包编译同时覆盖当前 Go 类型检查。
- Tests：本轮未新增或运行行为专项。`go test ./...` 退出 0，12 包均为 `[no test files]`，仅作为草稿隔离后的包编译证据，不计新增行为断言。
- 主线运行回归：只读核对 `.runtime/verify/usable-20261123-mainline/readback.json`，其记录同一 `nas.exe` 的 core 52 / backup 38 / account 162 / storage 17，共 269 项通过、193 源零漂移、精确活进程 0；这是主线证据，不冒认本检查者重跑。主线另通知桌面 embedded 模式 17 类检查通过、未跑真实托盘；本报告未独立审计该运行明细。
- 规范一致性：产品相对上片 193 项基线无变化，无新增跨层读写路径，无需本检查者修改正式规范；S03.2.3 继续按冻结契约待续，不将整个 S03 标为完成。

所有本地命令经 `python -B scripts/project_env.py -- ...`；辅助脚本的子命令继承同一项目内缓存/临时目录。CodeGraph 仅执行帮助和 `query Files --limit 5` 定位嵌入资源接缝，未 init/index/sync。Windows `.cmd` 首次被包装器拒绝后改用明确的 `cmd /c` 只读调用，不是产品失败。

独立证据为 `.runtime/verify/usable-20261123-review/package-source-audit.json`（actual UTC `2026-11-23T06:32:41.578902+00:00`）及 `static-checks.json`（actual UTC `2026-11-23T06:33:33.081969+00:00`）；均已全文读回。包/清单/产物摘要来自实际字节，静态结果保存了退出码及原始输出；较早已完成的 vet/前端类型/空白检查保留原工具输出引用，不伪造执行时间。

本轮保存前对原始检查输出及审计 JSON 扫描固定 bcrypt、Authorization Bearer、nas_session 标记，未命中；该有限集合不代表证明所有未知秘密不存在。辅助工具只在自有证据目录留存，未生成单元测试类或产品测试源码。

本文完整读回并核行/字节/SHA 后，检查者停止产品、测试、工具、证据和报告的全部写入，将本轮结果交回主线。
