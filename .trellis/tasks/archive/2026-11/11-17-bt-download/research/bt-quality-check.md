# BT 下载独立质量检查

检查日期：2026-11-17。检查角色：独立 Trellis reviewer。范围包含当前 tracked diff 和新增下载后端、前端、部署配置、脚本与跨层接线；本记录不替代主代理的真实浏览器、引擎及集成验收汇总。

## 检查依据

- 本任务 `prd.md`、`info.md`、`implementation-contract.md` 与 `check.jsonl` 引用规范。
- `.trellis/spec/backend/runtime-contract.md`、`download-contract.md` 和 frontend `app-contract.md`。
- `AGENTS.md` 的中文注释、类型、脱敏日志与临时单测清理要求。
- 使用 CodeGraph 定位代码后核对实际 API、协调器、发布门禁、扫描／读取和前端请求路径，不仅比较 DTO 声明。

## Findings (fixed)

### 1. 关闭下载模块后无法查看历史任务

- File：`web/src/components/DownloadCenter.vue`
- Issue：`refresh()` 在 `enabled=false` 时提前返回，后端本地历史查询虽可用，界面却完全不读取列表和详情。
- Fix：移除该提前返回，继续读取本地快照；原 `live` 条件仍限制写操作。已核对列表、详情和轮询的取消／过时响应逻辑。

### 2. 恢复下载页 hash 时提前请求私有接口

- File：`web/src/App.vue`
- Issue：未登录直接进入 `/#downloads` 时，`restoreHash()` 触发 watcher，在 `/status` 完成前请求 libraries／stats／jobs，产生无意义 401；管理页同类路径也受影响。
- Fix：为 metadata、settings 与导航 watcher 增加应用会话 guard，登录后仍由 `initializeLibrary()` 加载。修复后完成 strict typecheck 与生产构建；主代理整页刷新复验未登录 `#downloads` 只请求 `/api/status`，没有 console 错误／警告，随后登录恢复 hash 成功。

### 3. 迟到元数据响应遗留孤立清单

- File：`internal/app/download_sync.go`，`saveManifest`
- Issue：先插入 version=1 文件，再进行条件任务 UPDATE；用户在网络返回前提高 revision 时，UPDATE=0 仍提交文件，后续同步再次插入撞键。
- Fix：向后端责任代理提供复现并核对其修复：事务先条件 UPDATE 占有版本、检查 RowsAffected=1，之后使用 prepared INSERT；条件未命中即回滚。
- Regression：`TestDownloadStaleManifestRevision` 已由本 reviewer 独立执行通过，覆盖过时响应不遗留文件、新修订仍能保存。

### 4. 迟到绑定响应清理仍需恢复的私密来源

- File：`internal/app/download_sync.go`，`bindTorrent`
- Issue：条件绑定 UPDATE=0 时仍删除 source，可能破坏新修订恢复所需的来源。
- Fix：向后端责任代理反馈并读回确认，仅 UPDATE 命中后清理受控来源；迟到结果保留 source，后续对账处理当前意图。
- Regression：与清单修订临测一起独立执行通过。

### 5. ready 任务在元数据或空文件响应时未冻结

- File：`internal/app/download_sync.go`，`syncRecord`
- Issue：metadata、待处理命令和空 files 的提前返回可能绕过后续 ready 撤销，导致重下／校验阶段残留发布状态。
- Fix：向后端责任代理反馈并核对修复：unsafe engine state 的 ready 冻结提前至各返回分支之前；已发布任务收到空清单也冻结。另读回确认外部优先级变化会与锁定选集比较并撤销发布。
- Regression：`TestDownloadReadyFreeze` 的 metaDL、checkingUP、stoppedUP+空 files 三个分支均由本 reviewer 独立执行通过。

### 6. 集成验证的暂停幂等计数需跟随恢复语义

- File：`scripts/verify_downloads.py`
- Issue：新增 selection/start 前先 stop 并重申选集之后，断言“全部 stop 调用数等于 1”会误报暂停幂等失败。
- Fix：向脚本责任代理反馈后，已读回确认改为“暂停前基线 + 1”，并等待 fake 引擎实际执行后再断言，避免把早于执行的 DTO paused 当作执行完成。最终集成结果以其验证记录为准。

### 7. 实际引擎登录状态和会话名称不兼容

- File：`internal/app/download_qbittorrent.go`
- Issue：真实 qB 5.2.4 返回 HTTP 204 空正文和 `QBT_SID_<端口>` 会话，适配器仅接受 200／`SID`，且后续请求固定回送 `SID`，导致真实登录后仍被判定未认证。纯假引擎原样本未暴露这一差异。
- Fix：直接修复 login／request／call：接受 200 或 204 和 `Ok.`／空正文，仍强制有效非空会话；仅允许 `SID` 或 `QBT_SID_` 加 1–65535 规范十进制端口。实际名称和值一起在认证锁内保存和读取，随后按实际名称回送；端口不强制等于反代外部端口。拒绝任意 Cookie、无会话、失败正文／HTTP，认证失败清除旧会话并退避。
- Regression：独立新增 `TestDownloadLoginCookieQuality`，14 个子项全部通过，覆盖 200／204、新旧名称、反代端口不同、后续实际名称回送、无会话／空值／非法名称／非法端口与错误认证；临测执行后已删除。旧阶段另有 7 子项状态码临测通过，不重复计入最终 14 项。
- Interoperability：部署验证代理随后用真实应用和真实 qB 5.2.4 跑通完整闭环，退出 0。已读回 [`qb-5.2.4-app-probe.json`](qb-5.2.4-app-probe.json)：204 + `QBT_SID_3710` 认证、元数据 stoppedDL／downloadedBytes=0、确认 65,536 字节小说后真实传输、移动、ready + imported、只入库一个所选项目并读回相同字节。未选文件即使有 0.5 的引擎进度也未入库。

## 其他重点复核

- 首次绑定补完整 properties 身份、metadata 未完成不绑定／清理来源、缺完整 v2 身份保持 pending 的最新修复已读回。新增 `TestDownloadCompleteIdentity` 由后端责任代理执行通过；本 reviewer 没有将其计入先前独立 13 组临测。
- 审核 64 KiB 普通 JSON、256 KiB 选集、8 MiB 种子和 multipart envelope 的路由预算，实际文件 ID／锁定清单、幂等键／revision、外部任务归属、保留文件归档、selected size／mtime 门禁与五类导入路径。
- 审核 staged／unknown／unselected／archived／disabled 数据门禁、漫画枚举／封面与 missing 保留接线；数据库 rows 在文件工作或嵌套查询之前释放。
- 审核 Compose 只读应用下载挂载、独立凭据、固定镜像、初始化临时回环端口和文档的非 Docker 证据边界。未执行任何 Docker 命令。
- 已将修订占有、source 清理、提前冻结、完整身份及历史只读行为的规范补充建议交给主代理，避免并行改写其规范文件。

## Verification

### 本 reviewer 独立执行

| 检查 | 结果 | 说明 |
| --- | --- | --- |
| `gofmt -l internal/app/*.go` | pass | 当前所有后端 Go 文件无格式差异 |
| `go vet ./...` | pass | 实际会话名称补丁后的最后一轮独立进程退出 0 |
| `go test ./internal/app -count=1 -v` | pass | 清理前 13 个顶层临测组通过，涵盖解析、SID、提交不确定、恢复选集、发布、脱敏、迟到修订、提前冻结、上传、JSON、门禁、迁移与挂载 |
| `go test ./... -count=1 -v` | pass | 先独立重跑 root 的 JSON／gate／migration／mount 四组；实际会话名称补丁后再次重跑，14 个认证子项全部通过 |
| 清理后 `go test ./... -count=1` | pass | 最终临测均已删除，仅证明最终源码可编译，不作为行为覆盖证据 |
| `go build -o .runtime/bt-quality/nas.exe ./cmd/nas` | pass | 实际会话名称补丁后 Windows 本机构建成功 |
| `npm --prefix web run typecheck` | pass | 两项前端修复后的最终严格类型检查通过 |
| `npm --prefix web run build` | pass | 两项前端修复后的生产构建退出 0；141 modules，Vite 4.62s |
| `git diff --check` | pass | tracked 修改无空白错误 |
| 新增文件空白／Python AST | pass | 检查新增源码／文档的行尾空白与 Python 语法，无错误 |

未配置独立 ESLint 命令；前端质量门槛采用仓库定义的 strict vue-tsc 和 Vite 构建，不能虚报执行了不存在的 lint 命令。

临时 Go 测试按所属代理分别清理：后端代理已删除其 `download_backend_temp_test.go`；root 已删除其 `download_gate_regression_test.go`；reviewer 已删除自己的 `download_login_quality_temp_test.go`。未擅自删除他人临测，保留复用集成脚本。

### 其他责任代理的证据边界

- 原五类媒体 `scripts/verify.py --large-file` 的 52 项断言、四尺寸双主题和 20,000 文件前端假 API 场景，由对应代理执行并另行落证据。root 在实际会话名称补丁后重建 Windows 和 Linux amd64／arm64，均退出 0；随后最新 Windows 二进制的原 52 项媒体回归再次全通过。
- 真实 qB + 真实应用的最终闭环已由部署验证代理执行通过，JSON 已读回确认；旧 SID／204 假引擎的 36 项断言也由其执行通过。最终 `nas-cookie-final.exe` 与 HTTP 204／`QBT_SID_<端口>` 假引擎的完整集成再次 36 项全通过、退出 0，涵盖五类扫描、入库失败重试和生产 60 秒退避下的提交不确定恢复。该轮由部署验证代理执行并反馈，不标为 reviewer 亲自执行；详细记录见 `bt-implementation-probes.md` 和 `docs/verification.md`。
- Vite 重建会移除旧 chunk；已经打开的测试页需要完整刷新。旧已加载页面引用旧 chunk 的 MIME 报错不作为可复现生产源码缺陷。

## Findings (not fixed)

当前没有确认的待修生产代码缺陷。Docker 构建／部署、Linux 容器实际运行与深层 bind mount、目标 NAS 卷权限／空间不足／掉电／挂载失联恢复、真实 pure v2／hybrid BT 传输、真实超大影片的同时播放与资源峰值，以及物理手机和其他浏览器仍保留为未执行的兼容／设备验收；本机通过的模拟、稀疏协议样本和 v1 真实闭环不能替代这些结果。未运行任何 Docker 命令，不自动提交或归档任务。
