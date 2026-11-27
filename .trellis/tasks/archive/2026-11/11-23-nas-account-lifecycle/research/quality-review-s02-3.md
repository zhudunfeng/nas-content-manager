# S02.3 独立质量复核

日期：2026-11-23。范围为停机离线重置、稳定验证器及其对既有在线账号和核心媒体的影响。本轮按当前 PRD、冻结接口、`check.jsonl` 所列规范与 `trellis-check` 复核，并直接修复确定问题。正式 spec、任务状态与统一 baseline account 的最后编排由主线维护。

最终独立结果：**账号 157 项通过 = 在线 83 + 离线 74；核心媒体 52 项通过**。两组真实执行同一 Windows 产物，SHA256 为 `8bc6a1a144d86bd0a65e38bafc9f1388b21bf2545c9a29168d92ebdf0dbf135a`。Linux 两架构只有交叉构建证据，不能标成实机通过。本轮没有提交、部署、操作真实账号/媒体或启动 Docker。

## Findings (fixed)

### 1. 损坏 bcrypt 摘要能通过离线预检

- File: `internal/app/account_offline_store.go`、`scripts/verify_account_offline.py`。
- Issue: 原预检只调用 `bcrypt.Cost`；实际独立夹具确认，额外尾缀、非法 salt/digest 字符、未知 minor 版本仍会通过该宽松解析，带 `+4` 的非两位数字成本也被接受。不符合“唯一有效管理员、损坏凭据拒绝”的冻结边界。
- Fix: 新增 `validOfflinePasswordHash`，先核对精确格式，再调用 `bcrypt.Cost` 校验成本范围。只支持 `$2a$`/`$2b$`/`$2y$` 的 60 字节格式，以及兼容历史 `$2$` 的 59 字节格式；成本必须为两个 ASCII 数字、值仍在 bcrypt 接受的 4–31 之间；salt 与 digest 合计 53 个 bcrypt-base64 字符，拒绝尾缀和非法字符。这里校验存储格式与成本，不声称验证未知的旧密码。
- Regression: 临时 Go 对四类损坏摘要的真实 SQLite 预检、a/b/y/历史2正例及四版本带符号成本全部通过。稳定离线增加 `hash-suffix`、`hash-salt-alphabet`、`hash-digest-alphabet`、`hash-version`、`hash-signed-cost` 五个拒绝夹具，逻辑库不变且租约释放。离线计数从 69 增至 74。

### 2. 账号已知超限正文偶发丢失 413 响应

- File: `internal/app/account.go`、`scripts/verify_account.py`、`docs/api.md`。
- Issue: 独立完整账号回归在原第 58 项实际遇到 Windows `ConnectionResetError`/`WinError 10054`，不能记成 413。原候选 SHA `dfc684470c610aeee2407fb17c880e5fd94b9a55e2d05a35e6fd9e5df08a3e81` 的隔离定向 30 次请求为 29 次 413、1 次真实重置，所有请求后 health 均为 200；应用没有崩溃。
- Root cause: 依据本机实际 Go 1.27.4 的 `.tools/go/src/net/http/server.go`，客户端请求 `Connection: close` 已在写响应头时使 `closeAfterReply=true`，因此标准库写头前默认 drain 路径跳过；不是只归因于共享 defer 的执行时机。账号已知长度提前 413 未实际读取 `MaxBytesReader`，共享小体收尾又按约定跳过大体。`transfer.go` 的 `body.Close` 在 closing 且无 trailer 时直接跳过读取，不置 early-close；因此没有进入限额命中的原生 `CloseWrite` 与 RST 规避等待路径，最后直接关闭仍有未读数据的连接。
- Fix: 经主线确认后，仅在账号 JSON 已知 `ContentLength>64 KiB` 的 413 分支、写响应前调用 `discardOversizedAccountBody`。明确 `Connection: close`；非 Expect 且读取截止时间受支持时，局部最多 250 毫秒，通过真实 `MaxBytesReader` 最多消费 64 KiB 加一个溢出字节，不解析或保留输入。预期限额错误后恢复截止时间；超时、提前 EOF 或其他故障保留已到期截止时间，防止 native body close 再次等待。Expect 分支零读取，不发送 100 Continue；不支持截止时间时零读取；未知长度保留原解码限额。共享 `security` 的唯一小体 defer、大体/未知/Expect skip、正文 Close 语义和媒体超时均未更改。
- Boundary: Go 原生限额关闭可能在 handler 返回后另加固定 500 毫秒半关闭收尾；250 毫秒是此局部读取预算，不是整个连接生命周期上限，也不是任意网络下绝对送达保证。不使用客户端重试、合成 reader 触发限额、零预算技巧或全局 sleep。
- Regression: 临时 Go 八个分支通过：底层精确最多读取 65,537 字节、Expect 零读取、不支持 deadline 零读取、意外/普通提前 EOF 及故障保持 expired、精确 64 KiB 不进入新路径、未知长度继续原限额。稳定在线增加八项真实断言，其中 100 次独立关闭连接请求逐次精确 413 且逐次 health200；传输异常直接失败，不补发。在线计数从 75 增至 83。

同次最终真实请求证据：`.runtime/verify/s02-3-account-review-http-final/oversized/oversized-result.json`。

| 场景 | 实际结果 | 响应耗时 |
| --- | --- | ---: |
| 已完整发送、独立关闭连接请求 | 100/100 为 413，0 传输错误 | 各次健康检查通过 |
| 两片正文间隔 50 毫秒 | 413 | 50.745 ms |
| 声明 1 TiB、只发送 1 字节后停滞 | 413 | 250.746 ms |
| Expect 等待正文 | 首个状态为 413，未收到 100 | 0.448 ms |
| 未知长度 chunked 超限正文 | 413 | 0.416 ms |
| 超限声明但提前半关闭写端/EOF | 413 | 0.470 ms |
| 精确 64 KiB 完整 JSON | 正常解码后按原密码 72 字节上限返回 400 | 0.480 ms |

全部拒绝后账号 hash/revision、会话数和媒体库数不变，当前 Cookie 仍可鉴权。原失败根 `.runtime/verify/s02-3-account-review-final` 及 30 次对照根 `.runtime/verify/s02-3-http-reproduction` 保留，没有覆盖或改写通过状态。

### 3. 文件链接夹具没有先尝试真实 symlink

- File: `scripts/verify_account_offline.py`。
- Issue: 原 Windows 路径直接创建 Junction 并记录文件 symlink 特权不可用，证据结论没有绑定本次真实尝试。
- Fix: 先实际创建文件 symlink；只有 Windows 明确返回 `WinError 1314` 才使用目录 Junction 替代并记录限制，其他错误直接使验证失败。最终本机确实遇到特权限制，因此 `databaseLinkFixture` 如实记录 `directory-junction; file-symlink privilege unavailable`，不能声称文件 symlink 已通过实测。目录别名 Junction 的同锁竞争为真实执行。

### 4. ACL 恢复比较未同时要求描述符长度一致

- File: `scripts/verify_account_offline.py`。
- Issue: 原恢复核对只按恢复后长度比较原始字节前缀；缺少原长度相等条件会弱化恢复证据。
- Fix: 恢复后要求报告长度与原安全描述符长度一致，且完整 raw 字节逐字节一致。最终真实普通写句柄拒绝、维护 exit1、逻辑账号不变、租约释放和原 ACL 完整恢复均通过。

`docs/accounts.md` 同步补充摘要格式拒绝、在线大正文证据位置及真实传输失败语义。

## Findings (not fixed)

- **真实 Linux/NAS 尚待目标环境验收**：本机为 Windows/AMD64，Linux amd64/arm64 只有交叉构建，未执行 flock、真实 Linux 终端、POSIX 权限、挂载和目标 NAS 性能。需要目标设备与实际文件系统，不能由本机自修成“通过”。
- **Windows 文件 symlink 特权限制**：本次真实尝试明确遇到 1314；已执行 Junction 路径拒绝与目录别名锁验证，但文件 symlink 自身仍未实测。不提升用户权限或改变全局设置来获得该证据。
- **主线最后收尾尚独立进行**：正式 `.trellis/spec/`、任务元数据与 baseline account 编排由主线持有。此报告交接精确摘要格式及账号有界 413 契约，不自行改这些文件，也不把整个 S02 或路线其余切片标为完成。

未发现仍待修复的本次 Windows 实现阻塞问题。上列目标环境与所有权边界继续保持明确待验状态。

## Verification

### 门禁与产物

- Lint: **pass**。修改 Go 文件 gofmt 无输出，`go vet ./...` exit0。
- TypeCheck: **pass**。`vue-tsc --noEmit -p web/tsconfig.json` exit0；本轮未修改 web。
- Tests: **pass**。下述独立临时 Go 行为测试实际执行后清源；清源 `go test ./... -count=1` exit0，各包显示 `no test files`，仅作为全包编译证据，不重复算行为覆盖。
- Python: 两个账号稳定验证器 AST 解析通过；真实完整回归也执行了更新后的代码。
- Toolchain: 实际为 **`go version go1.27.4 windows/amd64`**，与 `go.mod` 中语言版本声明 **`go 1.26.0`** 分开记录。使用既有项目工具链，没有升级 Go 或用户级环境设置。
- Build: Windows 主程序与 Linux amd64/arm64 交叉构建 exit0；最终结果见 `.runtime/verify/s02-3-review-checks-http/result.json`。构建阶段记录 `executed=false`，实际 Windows 执行由后述同 SHA 集成证据证明。

| 最终产物 | 字节 | SHA256 | 实际执行 |
| --- | ---: | --- | --- |
| `.runtime/bin/s02-3-review-http-final/nas.exe` | 21,447,680 | `8bc6a1a144d86bd0a65e38bafc9f1388b21bf2545c9a29168d92ebdf0dbf135a` | Windows 真实回归 |
| `.runtime/bin/s02-3-review-http-final/nas-linux-amd64` | 21,252,720 | `509c94eabe0b4d166f5f0cf196f986d6ba6cd323ff4aad6d47517b9beead3a10` | 否，仅编译 |
| `.runtime/bin/s02-3-review-http-final/nas-linux-arm64` | 19,974,539 | `522c8cc2f71dab710d2c8fac70596f4b40e079b1c521b5b89aeb6166009bc5c2` | 否，仅编译 |

全部构建、测试和临时工具均通过 `python -B scripts/project_env.py -- ...` 启动；嵌套进程继承仓库 `.runtime/` 缓存环境，并显式使用 `.tools/go/bin/go.exe`。不使用用户级环境变量或 `go env -w`。

### 独立临时 Go 行为复核

独立生成的 `internal/app/account_s023_review_temporary_test.go` 已删除；本轮共三批、九个顶层测试组，行为输出按各批原样保留，不混称同一二进制运行：

- `.runtime/verify/s02-3-review-checks-final/temporary-go-tests.log`：七组通过。覆盖合法摘要兼容；hash-only/revision-only/同时变更、schema/user_version 与溢出 CAS；真实 UPDATE/DELETE 后故障及 DELETE 后取消整笔回滚；真实提交后驱动报错及回滚报错分类为 unknown；真实提交后的取消保持成功；两提示 CRLF/确认不符、坏 UTF-8 或超限后退格仍拒绝、两提示 Ctrl+C、提示/唯一恢复及恢复失败分类；1023/1024/1025 原始预算；真实 io.Pipe 短输入必须等 EOF、最多 75 字节、等待可取消；严格分派、零初始化帮助与未知值不回显；四类坏摘要真实预检拒绝。
- `.runtime/verify/s02-3-review-checks-last/temporary-go-tests.log`：带符号成本补充一组通过，合法四版本保留。
- `.runtime/verify/s02-3-review-checks-http/temporary-go-tests.log`：账号大正文读取边界一组、八个分支通过。

事务故障通过临时 driver 包装真实 SQLite 操作后注入，没有增加生产测试开关。终端确定性适配测试不替代下面真实 Windows Console。最终已复查仓库无本轮临时 Go 测试源；再次 clean-source 编译通过。

### 最终同产物实际回归

1. **账号完整组合：157 项通过**。
   - 根：`.runtime/verify/s02-3-account-review-http-final`。
   - 顶层读回 `passed=true`、`onlineAssertions=83`、`offlineAssertions=74`、`assertions=157`、`offlineCLI=true`，`offlineVerification.requested/passed=true`；真正 schema4 弱/中文历史账号迁移均完成。
   - 子结果 `offline/result.json` 同样 SHA/大小、74项、`realCLI/rawOutputSecretScan/canonicalAliasLock/noRecoverySideEffects/restartCredentials=true`。
   - 真实活实例与 canonical Junction 同锁冲突、严格参数/精确 help、existing-only URI、旧/未来/损坏/异常结构与账号、外来 trigger/cascade、坏摘要、20组非法输入、无显式旗标的管道均按约定拒绝；成功路径只改 hash/revision/sessions。其它表、running 作业、dispatching 动作、pending 缓存及媒体哨兵保留，integrity/FK 检查通过，真实重启旧密码和旧 Cookie 失效，新密码可登录。
2. **Windows Console 真实 11 场景通过**。
   - 真实 KEY_EVENT Ctrl+C、系统 `CTRL_C_EVENT`、Ctrl+D 分别覆盖两个提示；另有确认不符、部分管道未 EOF 时系统取消、真实终端加 stdin 旗标拒绝、32标点逐键加退格且两提示分开确认、CRLF 两行成功。
   - 交互模式准确 `503→1008→503`；管道与终端拒绝场景如实保持 503。取消 exit130、确认失败 exit1、合法输入 exit0；全部有界退出、模式恢复、租约可再取得。原始双流和可见 viewport 扫描无已知秘密，屏幕范围仅 `visible-viewport`，不扩称整个滚动缓冲区。
3. **Windows ACL 真实拒绝与恢复通过**。
   - `actualPermissionDenial=true`；`permissionFixture.established/realWriteDenied/unchanged/aclRestored=true`、`maintenanceExitCode=1`。只修改自有 SQLite 副本 DACL，普通写打开实际被拒绝，finally 完整恢复原描述符；没有修改用户数据目录 ACL。
4. **核心媒体：52 项通过**。
   - 根：`.runtime/verify/s02-3-core-review-http-final`，`run-manifest.json status=passed`，同最终 Windows SHA/大小，子验证 exit0，自有进程树回收。
   - 五媒体、鉴权、扫描、个人状态、真实 FFmpeg、SQLite 快照及大文件 HEAD/Range/416/304 等通过。5 GiB 稀疏夹具仅证明 HTTP 偏移，不证明真实大视频解码、吞吐、内存或 NAS 规模性能。

主要可复现命令（重跑须换尚不存在的运行根）：

```powershell
python -B scripts/project_env.py -- .tools/go/bin/go.exe vet ./...
python -B scripts/project_env.py -- .tools/go/bin/go.exe test ./... -count=1
python -B scripts/project_env.py -- python -B scripts/verify_account.py --binary .runtime/bin/s02-3-review-http-final/nas.exe --offline --root .runtime/verify/s02-3-account-review-http-final
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --cases core --artifacts .runtime/bin/s02-3-review-http-final --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe --ffprobe D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffprobe.exe --large-file --root .runtime/verify/s02-3-core-review-http-final
```

保留早期失败、中间产物、定向对照和最终结果，不把中间旧 SHA 的通过结果拼接为最终同产物证据。主线最后的 baseline account 编排结果应另记，不在本独立报告中提前宣称通过。
