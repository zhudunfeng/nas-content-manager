# UAV Go 自动初始化模块交付与验证

## 实际交付

- `internal/uavbootstrap/bootstrap.go`：固定 owned 命名空间、精确归属标记、只读 Resolve、串行 Initialize、随机暂存/不可变 `gen-<32hex>`、严格 current.json 原子选择、最终离线 probe 门槛、失败/取消只清理当前拥有的尝试。保留所有已有可用 generation，不接管未知目录。最终原生 EXE/DLL/PYD 路径预算为248个 UTF-16 单元，真实 probe 必须在改名至最终目录并重新严格校验之后、发布 current 之前执行；stage 只做静态校验。
- `internal/uavbootstrap/plan.go`：只读 embed 当前 `assets/{recipe.json,runtime.json,payload.zip}`，内嵌材料精确 SHA、运行时/额外工具及许可库存闭包、每个文件唯一来源、安全 ZIP 全目录检查、大小写/父目录冲突/加密/链接/超预算拒绝；严格 JSON 拒绝重复、未知、大小写别名、缺失、null 和额外值。
- `internal/uavbootstrap/download.go`：固定官方 HTTPS、标准 TLS、已有受信代理、GitHub 已审查 CDN 有限重定向、连接/请求预算、精确字节及哈希、真实进度、脱敏中文错误。瞬态连接/读取错误及 HTTP408/429/5xx 最多3次（1秒/3秒可取消退避），完整性/长度/来源/清理失败不重试。
- `internal/uavbootstrap/materialize.go`：无 pip/系统 Python/网络脚本，真实固定材料下载到本次 .downloads 后只还原映射文件；独占写入、分块取消、CRC/长度/SHA、持久化、完整关闭和有界清理。包自身不启动进程，真实 probe 由桌面持有的 Job 执行。
- `internal/uavbootstrap/atomic_windows.go`：MoveFileEx REPLACE_EXISTING + WRITE_THROUGH，不先删除旧选择。`atomic_other.go` 仅保留跨平台编译，公开入口仍限制 Windows amd64。
- `ErrCleanup` 清理失败身份跨层保留；已有 generation 的 probe 若报告 ErrCleanup，立即失败，不继续初始化/再次执行进程。

## 真实验证证据（2026-11-27，当前 Windows amd64 开发机）

所有命令均经 `python -B scripts/project_env.py -- ...`，Go 缓存及临时目录位于仓库 `.runtime/`。

第一轮临时行为测试完整运行：

```text
.tools/go/bin/go.exe test -count=1 -v ./internal/uavbootstrap
PASS
ok nas-content-manager/internal/uavbootstrap 54.538s
```

本地完整输出：`.runtime/uav-bootstrap-tests.log`。测试使用当前编译资源和构建侧已下载、精确 SHA 命名的 `.runtime/tools/uav-bootstrap-inputs/`，仅通过测试私有 HTTP transport 提供原始字节；生产没有本机缓存/路径/URL 覆盖开关。

覆盖及实际结果：

1. 完整还原20个真实固定官方材料，791个库存文件（另有 manifest/receipt，磁盘实际共793个普通文件），严格 `validateGeneration` / `uavruntime.Load` 通过。随后调用真实 `collectionworker.CheckUAV`，私有 Python 真实导入 UAV/curl_cffi/AES 等、固定 yt-dlp 和 FFmpeg 本地执行均通过；零视频站点访问。
2. 原子发布后 Resolve 精确读回；Initialize 重新探测并复用已发布 generation，下载请求数仍为20（零重复下载）。probe 清理失败立即返回 ErrCleanup。加入未声明 DLL 后 Resolve 拒绝。
3. ZIP traversal、反斜杠、设备名、ADS、首斜杠、空路径段、重复名、大小写别名、隐式父目录别名、文件/目录碰撞、链接、加密、单文件超预算全部拒绝；有效隐式/显式目录组合通过；真实 CRC 损坏不保留部分文件。
4. 下载正常/截断/过长/哈希错误/声明长度错误/403，临时网络错误脱敏；失败未留部分文件；GitHub CDN 允许、非 HTTPS/未知域/凭据/端口/片段/超限重定向拒绝。
5. 首次初始化在还原阶段取消，返回 context.Canceled，只留下合法 owner 标记，无 stage/generation/current；未知既有目录拒绝接管。
6. 当时的原生文件260单元长路径预算在首次下载前拒绝，提示使用较短 `--profile`，下载数0、probe数0，无 stage/generation/current。此项不足以证明真实原生加载边界，已由下述最终248单元回归补充和替代。
7. HTTP503→200 恢复恰好2次；持续503恰好3次后失败；哈希错误及403恰好1次；429退避取消在1秒内返回且只发1次请求。
8. `.tools/go/bin/go.exe vet ./internal/uavbootstrap` 通过。

第一轮54.538秒用例使用较短根目录，未覆盖完整桌面中文 profile 的 stage→final 改名路径增量；不能据此宣称最终激活已被完整验证。

最终目录加载修复后的回归：

```text
.tools/go/bin/go.exe test -count=1 -run TestTemporaryFinalRelocation -v ./internal/uavbootstrap
real final CheckUAV passed with 248 UTF16 longest DLL, 20 fixed artifacts and 791 files, Chinese-space harness path, zero-download reuse
--- PASS: TestTemporaryFinalRelocation (27.95s)
PASS
ok nas-content-manager/internal/uavbootstrap 29.352s
```

本地完整输出：`.runtime/uav-bootstrap-final-relocation-tests.log`。使用与完整桌面 harness 相同长度、含中文及空格的 `.runtime/tmp/UAV 自动初始化 <8hex>/隔离 配置/collection-managed/uav-bootstrap/`。stage 最长 DLL 为250单元，仅完成静态还原与验证；改名至128位随机身份的 `gen-<32hex>` 后最长 DLL 为248单元，重新严格校验及真实 `collectionworker.CheckUAV` 均通过。测试确认真实最终 probe 前不存在 current，随后原子发布和 Resolve 精确读回；设置确定失败的本机代理后 Initialize 仍可再次真实探测并复用，下载数保持20。最终原生路径再加一个单元时拒绝。

独立审查另以完整 Initialize 注入最终 probe 失败及取消，确认候选代次清理、旧 current 字节和 Resolve 保留；证据由独立审查记录维护。

按照项目要求，验证后已删除本次生成的 `bootstrap_temporary_test.go` 和 `relocation_temporary_test.go`。空测试套件只作为编译 smoke，不能替代以上真实行为证据。长期集成验证由主线 `scripts/verify_uav_bootstrap.py` 保留。

## 踩坑与持久边界

- 完整桌面中文 profile 下，stage 静态库存及原先 stage probe 均通过，但改名至更长的旧 `generation-<32hex>` 后 curl_cffi `_wrapper` 导入失败，底层 Windows 报文件名/扩展名太长。最长固定本机文件是 `curl_cffi.libs/libcurl-impersonate-ae864b926b0a86bfa3b18a664407a1ab.dll`（相对路径102字符）。独立逐单元边界复现证明：最终 DLL 完整路径248/249/250个 UTF-16 单元时通过，251–260时失败；证据位于 `.runtime/verify/uav-review/native-loader-paths.json`（诊断夹具后续由所有者清理，JSON 保留）。因此不能将名义 MAX_PATH260 当作该第三方原生 DLL 的可执行预算。
- 最终修复保持128位随机身份，仅缩短固定前缀为 `gen-`，并将最终原生路径预算设为248个 UTF-16 单元，给实测250单元边界留2单元余量。生产在创建 stage/下载前检查，Resolve 同样检查；stage 路径不用于执行。执行顺序为完整还原→静态校验 stage→改名 final→严格重验 final→真实 probe final→原子选择。probe 失败/取消只清理本次未选 final，既有 current/代次不变；不得仅缩短测试根掩盖真实桌面路径问题。
- Go JSON 默认字段名匹配不区分大小写，DisallowUnknownFields 本身不等于精确 schema。新元数据通过精确 JSON 往返比较额外拒绝大小写别名、缺失/null；现有运行时完整验证器没有放宽。
- 真实在线上游 Gyan 的503由主线集成发现，增加的只是有限瞬态重试，没有弱化大小/哈希/HTTPS边界。

## 未完成/下一步与来源

- 本模块真实物料行为验证是官方材料的离线传输复原；当前网络的全部源可达性、桌面 controller 生命周期与真实在线下载由主 agent 的集成脚本验证，不能将本证据标记为干净 Windows 或真实站点验收。
- 需独立 trellis-check 审查以及主线全仓 Go/桌面回归。assets 与生成脚本、桌面接线、文档/spec 属各自 owner，本文没有修改它们。
- 来源：本任务 `prd.md`、`info.md`、`research/runtime-acquisition.md`；`.trellis/spec/backend/uav-windows-contract.md`、`jmcomic-windows-contract.md`、`cross-platform-contract.md`；现有 `internal/privateruntime/files.go`、`internal/uavruntime/runtime.go`、`internal/collectionworker/uav.go`。
