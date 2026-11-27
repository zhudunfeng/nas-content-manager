# S02.3 离线重置设计准备

日期：2026-11-23。当前切片仅做只读研究和本独立准备记录；S02.2 正在复核，本文不表示离线 CLI、终端验证或依赖接入已经完成。正式约束以 `../info.md` 为准，后续须经切片交接后实施。

## 1. 复用点与最小改动建议

已先通过 CodeGraph CLI 定位 `acquireInstance`、`openDatabase`，再读取具体实现及附近代码。

| 位置 | 已核对事实 | S02.3 建议 |
| --- | --- | --- |
| `cmd/nas/main.go` | 当前先 LoadConfig，再创建信号上下文 | 将执行放入返回退出码的函数；先创建 signal.NotifyContext，再分派维护命令，最后才走原服务配置与启动。main 仅在该函数全部 defer 完成后退出。 |
| `internal/app/instance.go` | canonicalDataDirectory 支持已有目录别名；acquireInstance 同时会 MkdirAll | 离线先确认已有根及普通 library.db，规范化后直接复用 acquireInstanceFile 的同一路径/平台锁原语，避免离线误建目录；获取后再次检查目录/数据库身份。在线 acquireInstance 保持原行为。 |
| `internal/app/instance_{windows,linux}.go` | Windows 无共享 CreateFile；Linux flock；持有的锁文件不删除 | 不更换锁实现、不强制破锁、不用端口/PID判断。数据库关闭完成后才释放同一规范根的锁。 |
| `internal/app/store.go:openDatabase` | 创建缓存、清 .pending-*、迁移 schema、恢复 jobs 和外部动作 | 完全绕开。新增 existing-only 账号维护存储函数，不调用 App/Run 或正常启动存储。 |
| `internal/app/account_store.go` | accountSnapshot、CAS、修订溢出防线已有；在线写依赖真实 Cookie actor | 复用 snapshot 类型与新密码规则；新增独立离线事务，不能制造 actor。初次不重构已经复核的在线敏感事务。 |

建议首批业务文件仅改 `cmd/nas/main.go`，新增 `internal/app/account_maintenance.go`（参数分派、流程）、`internal/app/account_offline_store.go`（受限打开和事务）、`internal/app/account_terminal.go`（输入与恢复），并固定 `go.mod/go.sum` 中的官方终端依赖。文件拆分可由实际长度微调，不触及 web 或其他域。

稳定验证可新增 `scripts/verify_account_offline.py`；正式交付再补停服手册、对应 spec/任务记录。准备阶段不修改这些实现、验证器或正式文档。

## 2. 命令、返回与资源顺序

固定命令：

```text
nas account reset-password --data-dir <已有目录> [--password-stdin]
```

- 无参数才进入原服务；一旦有参数便只走明确的命令解析，未知命令不能回退为启动服务。
- 自行限制固定 token 和唯一旗标；重复、缺值、未知参数、额外位置参数均返回固定中文用法，不把未知值或原始解析错误拼入输出。不接受密码 argv 或环境入口。
- 可增加精确 `--help` 的只读帮助，但不能因帮助打开目录、锁或数据库。是否支持 `--data-dir=<值>` 应在实施时一次固定并覆盖测试，不能含糊依赖 flag 默认宽松行为。
- 建议退出码：0 已提交成功，2 用法错误，1 前置条件/存储失败，130 取消。取消发生在提交后则不能再声称未修改；成功提交后的 ctx.Err 不能覆盖提交事实。
- 资源生命周期：解析 → 已有路径预检 → 规范根独占 → 再检查身份 → existing-only DB 与账号预检 → 输入/确认 → 恢复终端 → 校验与 bcrypt → 独立短事务 → 关闭 DB → 释放 lease。输入和 bcrypt 均不持写事务；输入期间可以持应用租约。
- defer 必须保留清理错误；如果提交完成但关闭出错，应明确“密码已更新，清理失败需核对”，不声称已回滚，也不自动再重置。

## 3. 受限 SQLite 打开与结构检查

### 3.1 路径与 DSN

- 原目录和规范根都须已经存在且为目录；`library.db` 用 Lstat 检查为普通文件，不接受符号链接、目录或特殊文件。锁前后复查，必要时以 os.SameFile 比对打开前后身份；路径错误只输出固定中文原因。
- 不调用 acquireInstance 的 MkdirAll。直接在 canonicalDataDirectory 得到的已有根调用同一个 acquireInstanceFile，因此在线和离线竞争的是同一个 `.nas-instance.lock`。
- modernc 当前驱动文档已核对：普通文件名后的 `?mode=rw` 会被驱动去掉，SQLite 参数只有以 `file:` 开头的 URI 才生效。因此必须构造转义后的 file URI，固定 `mode=rw`，再 sql.Open/Ping；不能字符串拼接用户查询参数。
- Windows 盘符路径构造成 `file:///E:/...`，Linux 构造成 `file:///...`；使用 net/url 转义空格、`?`、`#`、`%`、非 ASCII 等，验证实际打开的唯一文件。拒绝或单独覆盖 UNC；不要把它误编码成 URI authority 或网络存储承诺。
- SetMaxOpenConns(1)；只设置连接级 foreign_keys/busy_timeout 等必要设置，不执行 journal_mode、迁移、缓存清理、扫描或任务恢复。不添加 immutable/nolock 来绕过锁。
- directory lease 只协调遵守本项目锁的实例；不能宣称它能抵御有权限的第三方随时替换目录/库文件。不要在本切片额外扩张为通用文件系统隔离框架。

### 3.2 支持范围

首先读取 user_version，只接受精确 5。旧版给正常应用迁移后停机的指引，未来版给匹配版本指引；不修改 user_version。

账号结构检查至少包括：users/sessions 是实际表而不是视图，所需字段/类型/主键和外键结构符合 schema5，查询的管理员至多两行且精确为一个 ID=1、合法字段类型及非负 int64 revision；缺账号、多个/其他 ID、空或损坏凭据、缺字段/表、非法版本均拒绝。用户名不因展示格式被重新命名。

还应检查与 users/sessions 写入相关的触发器：仅允许当前已知的 session_id 必填触发器，拒绝额外 UPDATE/DELETE 触发器，避免一个伪装成 schema5 的库在改密时修改无关业务表。不要对整个业务 schema 的 SQL 文本做脆弱的空白/顺序相等比较；以受影响账号对象及实际约束/查询验证为边界，并留恶意触发器夹具。

结构预检不等同全库健康检查；不要为一次账号重置扫描整个大型媒体索引。集成验证再做完整 integrity/foreign_key 检查。

### 3.3 离线写事务

保留预检得到的真实 ID/hash/revision 快照，bcrypt 在事务外生成。事务顺序：

1. BeginTx，读取并核对现有快照；只有完全相同的 ID/hash/revision 才能继续。
2. 拒绝 math.MaxInt64，不能溢出或重置为零。
3. 条件 UPDATE 同时匹配原 ID/hash/revision，推进 revision 并换 hash；RowsAffected 必须为 1。
4. DELETE 该管理员全部 sessions，不碰其他域。
5. 紧邻 Commit 前检查上下文；任一失败均 Rollback。Commit 返回成功后只报告已提交，不因随后的取消恢复旧密码。

不调用 changeAccountPassword，不伪造 Cookie。不自动重试结果未知的提交。原始存储错误可能包含路径或数据，只允许受控中文错误类别进入输出。

## 4. 官方终端库和取消设计

### 4.1 已核对源码

未改 go.mod/go.sum；通过官方 Go 模块代理在内存读取 `golang.org/x/term v0.48.0` 的 zip，检查了 term.go、term_windows.go、term_unix.go、terminal.go。官方版本/来源核验见 `offline-cli-notes.md`。

- 顶层 term.ReadPassword 自行改变模式并 defer 恢复，没有 context 参数。Windows 内部还会复制句柄；不能以关闭原 stdin 证明内部读取已经取消。
- 直接“读协程调用 term.ReadPassword，主协程取消便 Restore”有初始化竞态：主协程先恢复，读协程后切无回显，进程退出时遗留模式。禁止这种实现。
- term.MakeRaw 先同步切换并返回 State；term.NewTerminal 的 Terminal.ReadPassword 不再切操作系统模式，Ctrl+C 字节由行编辑器返回 EOF。
- Terminal 行编辑器内部上限是 4096 rune，满时会忽略后续输入，不能把这个静默上限当作本命令的有界输入策略；必须有更小、可观测、粘性失败的独立上限。

### 4.2 推荐执行模型

先确认 stdin 是实际终端，主协程同步 MakeRaw 并安装唯一 Restore defer；再启动只负责官方 Terminal.ReadPassword 的输入协程。两个密码均在同一次终端所有权内读取，不重复进入 raw。读取结束、错误、取消或确认失败均恢复原 State；恢复失败阻止任何账号写入。

正常 Ctrl+C 键在 raw 模式下作为 0x03 由库返回；Ctrl+D/EOF 也按取消处理。系统 CTRL_C_EVENT、SIGTERM 和外部 context 取消则让主协程选择取消，先恢复 State、完成 DB/lease 清理，再让维护进程直接退出。取消后的读协程只可能读取/写固定提示，绝不能调用 MakeRaw、Restore 或其他模式 setter，也不能被等待而阻止退出。结果通道有缓冲，避免收尾发送阻塞。

该模型只用于 main 中独立的维护进程，不能作为可取消读取后继续长期运行的通用库接口。管道取消同样可以等待结果或取消并直接退出，不以 Close/EOF 冒充真实 Ctrl+C。

输入句柄与输出句柄必须分离：终端识别和模式只使用 stdin 的真实 fd，固定中文提示写 stderr；stdout 只留固定结果。给 Terminal 配置窄的 io.ReadWriter 适配器，不错误地对 stderr 获取/恢复输入模式。记录输出写失败并让后续读取/流程失败；官方行编辑器对部分 Write 返回值处理较弱，不能在提示失败后仍写库。

### 4.3 交互输入上限和字符处理

建议每次输入设置最多 1024 个原始输入字节的预算（包含退格/按键序列），显著小于 4096 rune，最终密码仍严格为现有 10–72 ASCII 四类规则。计数达到上限后设置不可清除的 overflow 状态；读取到行结束或取消后明确失败，绝不能把前 72/1024 字节作为成功密码。超限后即使退格到短文本也失败并要求重新执行。

推荐适配器每次最多交给行编辑器一个字节，避免跨两个提示预读造成预算归属不清。超过预算后只消耗到该行结束并保留失败，不继续堆积内存；仍识别 Ctrl+C/取消。不要让库自己的 4096 rune 上限先发生。

行编辑器会忽略某些控制输入或处理编辑键，所以需独立记录原始输入无效状态：只把退格/DEL 作为明确支持的编辑操作，CR/LF 作为行结束，Ctrl+C/Ctrl+D 作为取消；其他控制、ESC 序列、非 ASCII（包含合法或非法 UTF-8）均使本次输入失败。为了防止坏 UTF-8 在库解析器里停留，可只向编辑器转交安全替代字节并保留粘性失败标记；任何经过这种替代的输入都必须失败，绝不能接受替代结果。新密码允许的全部 ASCII 标点原样保留，空格原样保留后由 validNewPassword 拒绝。

CRLF 只作为一个行结束：若一行以 CR 结束，适配器跨下一次 Read 保留“可忽略紧随的一个 LF”状态；不得删除其他 LF、空格或密码前后字符。两次输入比较的是未 trim 的最终字节，非法/超限标记也必须分别检查。必须用粘贴 CRLF、逐字键入和两个提示分开输入验证，而不是只测字符串函数。

交互超过预算时“消耗至行尾”保证内存有界，并不承诺恶意终端永不发送行尾时能自然结束；Ctrl+C/context 仍有退出路径。固定提示应说明输入错误需重新执行，不循环重试或偷偷保留密码。

### 4.4 显式 stdin 模式

`--password-stdin` 只在重定向输入时启用；若仍是终端则拒绝，避免用户在有回显的终端误输入。无此旗标的管道一律失败，不能自动降级。

有界读取最多 75 字节：合法极值为 72 字节密码加最终 CRLF 共 74 字节，第 75 字节是超限哨兵。只剥离一个最终 LF，以及仅在它之前的一个 CR；无结尾换行也接受。内部 CR/LF、多行、第二个空行、单独最终 CR、空白、控制和非 ASCII 均拒绝。不得 strings.TrimSpace、Scanner 默认分行后只取第一行，或只读 72 字节忽略尾部。

需要 EOF 才能证明这是完整的一个输入；生产者必须关闭管道。读取等待期间响应 context 取消，且输入读取/生成 bcrypt 之前之后都检查取消。输入数组使用后尽力清零，不声称 Go 字符串或库内部拷贝能被可靠全部抹除。

## 5. 可执行验证设计

所有构建、测试、临时工具通过 `python -B scripts/project_env.py -- <命令>` 启动。以下为待实现设计，不是已通过记录。

### 5.1 临时确定性 Go 测试

- 固定命令/旗标、缺值/重复/未知/含秘密的坏 argv；捕获输出证明坏值不回显，不调用配置/网络。
- stdin 9/10/72/73、无换行/LF/CRLF、内部/重复换行、单独 CR、空白/控制/坏 UTF-8、32 标点全量；原始字节无 trim/截断。
- 终端适配器预算 1023/1024/1025、超限后退格、非法字符被库忽略前的粘性标记、CRLF 跨 Read、输出失败；此组是逻辑注入，不能标成真实控制台。
- 真 SQLite 的独立离线事务：hash-only/revision-only/同时变更导致 CAS 失败；ID/账号异常、修订上限；UPDATE 后/DELETE 后故障与取消立即回滚。可在临时测试内用窄 driver 包装在实际 ExecContext 后触发取消，避免生产代码新增测试开关。
- 运行后清除本次临时 *_test.go，保留安全结果日志；再次 clean-source 编译，不能把无测试的 go test 冒称上述断言执行。

### 5.2 稳定真实进程/HTTP 验证

隔离副本启动自有服务，建立旧会话，停服后种入可检查但不会自动执行的 running job、dispatching action、缓存 pending 文件和无关域数据；对每次尝试做账号及其他表逻辑快照和文件清单。

- 活实例拒绝、端口不同仍拒绝，canonical 原路径/相对路径/junction 或 symlink 别名均竞争同一锁，拒绝后在线实例仍存活。
- 缺根/缺库/库为目录或链接、损坏 SQLite、真实旧 schema、未来 schema、伪版本缺字段、额外危险触发器、零/多个/其他管理员、溢出、无读写权限，均无账号写入/新库/任务恢复/缓存清理。
- 用无效 NAS 配置、缺媒体根/FFmpeg/网页的环境执行有效离线命令，证明分派早于 LoadConfig 与服务启动；不要修改真实用户环境。
- 成功重置后只 hash/revision/sessions 发生预期变化；维护进程退出后可重新获取租约；重启，旧 Cookie 和旧密码失效，新密码登录成功。单独区分重启本身正常任务恢复，不把重启后的状态归咎于离线命令。
- 快照以逻辑行/字段保留为准，并检查媒体和缓存文件。SQLite 正常 WAL/shm 建立、关闭检查点可能改变物理布局，不能把数据库全文件字节哈希当作所有拒绝路径的逻辑无写证明。
- 在持久化脱敏前，对原始内存 stdout/stderr/终端屏幕采样扫描测试密码、bcrypt、Cookie、token 摘要；有界收集，溢出直接失败。最终证据只输出断言名和安全结果。
- Windows 管理员 ACL 测试必须记录实际拒绝是否成立；只读属性/注入失败不是实际 ACL 拒绝证据。权限环境无法建立时将实机项保留待验。

### 5.3 Windows 真实 Console Ctrl+C

由稳定 Python verifier 创建专属 helper 子进程，使用 CREATE_NEW_CONSOLE 和隐藏窗口；helper 与其启动的 nas 只共享自有独立控制台，不 AttachConsole 到用户终端。输入显式打开 CONIN$，输出可分别测试重定向 stderr 和真实 CONOUT$；控制台与进程句柄按所有权回收。

helper 用 GetConsoleMode 记录初始精确 mode，在观察到第一/第二固定提示后核对 raw/无回显状态；使用 WriteConsoleInputW 写真实 KEY_EVENT，输入部分测试密码后发送 Ctrl+C 0x03，等进程有界退出，再检查完整 mode 与初始值一致、账号快照不变、租约可用。可补非秘密哨兵检查后续普通输入回显已恢复；屏幕原始捕获也扫描秘密。

另起独立场景：helper 安装自己的具体控制处理函数保护自身（不是设置可继承的全局忽略 Ctrl+C 属性），nas 不使用 CREATE_NEW_PROCESS_GROUP，让其正常接收处理事件；在同一自有控制台调用 GenerateConsoleCtrlEvent(CTRL_C_EVENT,0)。单独断言 signal.NotifyContext 取消路径、模式恢复、无写与有界退出。不要用只发 CTRL_BREAK_EVENT 替代 CTRL_C_EVENT。

分别记录：真实键盘字节 Ctrl+C、系统 CTRL_C_EVENT、管道取消、EOF/确认不符；四类互不充当对方证据。每类覆盖第一与第二提示。超时只回收本 verifier 精确持有的子进程树；不按进程名杀进程、不向用户进程组广播事件。新的 helper 在启动前继承项目环境。

### 5.4 门禁和目标环境

正式实施后运行 gofmt、go vet、临时测试、clean-source go test、Windows 构建及 CLI/HTTP/核心媒体回归；未改前端时不为展示活动重复改 web。Linux amd64/arm64 交叉编译只能记编译，不能冒称 flock、终端或 NAS 文件系统实测。Linux 真实锁/终端/权限与目标 NAS 项保留独立门禁。

## 6. 后续实施顺序

1. 接到正式切片交接后，实现入口分派和 existing-only 存储，先跑非交互拒绝/事务验证。
2. 固定官方 x/term，完成上述有界输入、句柄分离和恢复；再跑独立真实 Console 两种 Ctrl+C。
3. 完成成功重置/旧会话失效/无副作用的真实进程回归，扫描原始秘密并保留安全证据。
4. 删除仅本次生成的临时测试，做全套门禁，补操作手册、实际契约和交接记录，再独立复核。

本记录不发起实现、提交或部署；现有 S02.2 业务、前端、依赖、验证器和报告均未改动。
