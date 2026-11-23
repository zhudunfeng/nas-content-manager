# S03.2 Windows 持久发布平台研究

日期：2026-11-23。**仅 Phase 1.2 研究，尚未实现、冻结或验收通过。** 本轮唯一交付为本文件；不改 `persistent-backup-design.md`、S03.1、正式 spec、任务指针、业务代码、依赖或测试，不启动 Docker、用户实例或浏览器。下文的 API 能力、故障推论和待验证行为分别标明，不能把推荐写成当前应用保证。

## 1. 结论与需要冻结的准确边界

Windows 不宜因不能机械执行 POSIX 的 `fsync(parentDirectory)` 而被永久排除。可以用已有固定依赖实现一个受控的本地同卷无覆盖 publisher：逐文件 `Sync/Close`，关闭目标成员句柄，`MoveFileExW(..., MOVEFILE_WRITE_THROUGH)` 发布，复核最终身份和内容，再登记完成。Microsoft 将该标志描述为等待实际磁盘移动完成，并明确保证 copy/delete 移动在返回前完成 flush；目录移动另有限定必须同驱动器。由此有官方依据声明 **“应用所要求的文件 flush 和 Windows 写穿移动请求均已成功返回”**。[M1][M2][G1]

但不能扩大成以下承诺：公开文档没有明确给出“任意同卷目录树 rename 等价于 POSIX 父目录 fsync、且整棵子树在任意掉电点只会完整旧态或完整新态”的通用保证；M1 对 copy/delete 的附加 flush 描述也不能用来伪证本方案禁用的跨卷路径。文件系统、过滤驱动、虚拟磁盘、控制器和设备缓存仍参与持久化。Microsoft 特别说明并非所有磁盘硬件都支持相应写穿能力。[M1][M3]

**建议冻结的是 OS 确认的发布协议边界，而不是已经通过断电认证。** Windows 使用 Windows 协议，Unix 使用其文件及目录同步协议；缺少 Windows 目录 `Sync` 本身不判失败，也不把该错误吞掉后假装执行了 POSIX 协议。真正必需的 Windows 调用失败、身份异常或结果未知，仍不得登记 completed 或授权删除旧份。若产品坚持 completed 必须附带“整目录树任意物理掉电必定持久”的更强承诺，本研究没有找到足够官方保证，应保留该要求未满足的结论，不能仅将 WRITE_THROUGH 改名为断电安全。本文件不代替主线做这个产品语义冻结。

## 2. 仓库与固定依赖核对

先通过 CodeGraph CLI 查询 `replaceConfigFile`，再读取实际文件；下表是本轮直接核对的事实，不是新实现。

| 位置 | 当前事实 | 与备份的关系 |
| --- | --- | --- |
| `internal/desktop/platform_windows.go:18–55,83–93` | 已有当前用户/SYSTEM 私密 DACL；配置发布调用 `windows.MoveFileEx(REPLACE_EXISTING \| WRITE_THROUGH)`。 | Windows 写穿原语已经使用，不需新增依赖或手写 DLL 绑定；配置函数允许覆盖，**不能直接复用给备份目录**。既有注释中的“原子”不构成掉电证明，本轮不修改它。 |
| `internal/desktop/config.go:277–304` | 配置先同目录临时写入，`file.Sync()`、`Close()`，再调用上述替换。 | 可参考资源收尾方式，不沿用覆盖策略或宣称提供目录树事务。 |
| `internal/app/engine_runtime.go:243–282` | 私有 JSON 文件先 Sync/Close、os.Rename；只在非 Windows 打开父目录 Sync。 | 已存在平台区别；Windows 分支没有额外写穿移动，不能拿此方法证明备份持久性。 |
| `internal/enginehelper/files.go:75–112` | helper 私有文件执行文件 Sync、rename、目录 Sync。 | 是不同模块的私有文件协议，不是可直接复用的 Windows 备份目录发布器。 |
| `internal/app/instance_windows.go:13–34` | `CreateFile` 零共享锁文件、检查重解析点/普通文件、句柄所有权。 | 备份根独占和身份检查可沿用原则；锁只能约束配合者，不是内容 flush 或持久性证明。 |
| `go.mod` | 声明 `go 1.26.0`、`golang.org/x/sys v0.50.0`。 | `go` 指令不能在文档中直接等同本次实际执行版本。 |
| 项目包装器下 `go version`、`.tools/go/VERSION` | 实际为 `go1.27.4 windows/amd64`。 | 本轮源码核对使用此工具链；后续构建证据必须记录实际版本，不能写成已在 go1.26.0 上验证。 |

本地固定模块 `.runtime/go/pkg/mod/golang.org/x/sys@v0.50.0/windows/` 已直接提供：

- `MoveFileEx`、`MOVEFILE_WRITE_THROUGH=0x8`、`MOVEFILE_REPLACE_EXISTING=0x1`、`MOVEFILE_COPY_ALLOWED=0x2`；包装器实际绑定 `MoveFileExW`。
- `FlushFileBuffers`、`CreateFile`、`FILE_FLAG_WRITE_THROUGH`、`FILE_FLAG_BACKUP_SEMANTICS`、`FILE_FLAG_OPEN_REPARSE_POINT`。
- `GetFileInformationByHandle`、`GetFileInformationByHandleEx`、`GetFinalPathNameByHandle`、`GetVolumeInformationByHandle`、`GetVolumePathName`。
- `SetFileInformationByHandle`、FileRenameInfo/Ex 及相关常量；**存在调用封装不等于另有更强持久性保证**。

上述版本的原始依赖文件和当前 Go 源码均为只读核对，未修改缓存。当前 Go 的 `os.File.Sync` 经文件描述符同步进入 Windows `FlushFileBuffers`；`os.Rename` 的 Windows 路径使用 `MOVEFILE_REPLACE_EXISTING`，没有 WRITE_THROUGH。Go 公共文档也明确提醒非 Unix 上的 Rename 不保证原子，因此新备份发布不能继续泛用 `os.Rename` 后声称已经获得写穿、无覆盖和崩溃原子性。[G1][G2]

## 3. 官方依据能证明什么

| 行为 | 一手依据支持的边界 | 不支持的外推 |
| --- | --- | --- |
| `File.Sync` / `FlushFileBuffers` | Go 定义为提交文件当前内容到稳定存储；Windows 要求有效且具有 GENERIC_WRITE 的文件句柄，并将该文件缓冲信息送至设备。[G1][M2] | `Close`、读回成功或摘要正确不能代替 Sync；不能由一个普通文件的 flush 推出所有兄弟目录/文件都已 flush。 |
| `FILE_FLAG_WRITE_THROUGH` | 与 NO_BUFFERING 独立；可以保留普通缓存而请求及时写出。Microsoft 对 NTFS 说明该请求产生的相关 metadata（含时间戳、rename）也会 flush；同时列出硬件写穿限制。[M3][M4] | 不能把“相关 metadata”解释为替其它任意未同步对象建立全卷持久化屏障。无需引入 NO_BUFFERING 的扇区对齐复杂度。 |
| `MoveFileExW` 仅 WRITE_THROUGH | 函数适用于文件/目录；目录目标须同驱动器，已存在目录不能被其替换；不设 REPLACE_EXISTING 时不能依靠覆盖既有文件发布。[M1] | 不能据普通成功移动推出 SQLite 与目录处于同一原子事务，或目录成员具有公开文档未规定的整体掉电原子性。 |
| 目录句柄 | `CreateFile` 打开目录需要 BACKUP_SEMANTICS；普通用户仍受原有 ACL 检查。[M3] | “能打开目录”不等于“具有 POSIX fsync(directory) 的文档化等价保证”。本研究未找到允许这样替换概念的一手说明。 |
| 整卷 flush | Windows 文档允许以卷句柄 flush 所有打开文件，但调用者需管理员权限。[M2] | 不适合作为本项目普通用户备份的最小要求；不新增管理员开关或整卷操作。 |
| `SetFileInformationByHandle(FileRenameInfo)` | 官方允许受适当访问权限约束的按句柄重命名。[M5] | 此页面没有给出可替代 WRITE_THROUGH 的独立 durable-rename 选项。为减少自行构造可变结构和 handle-relative 代码，本片不以它作为更强持久性的捷径。 |

M1 的措辞涵盖移动操作，但其更具体的 flush 保证聚焦 copy/delete；对受控同卷目录树仍应保留上表限制。推荐 Windows 协议的依据是“选用可用且文档化的写穿 API 并核验返回结果”，不是已从这些文档推导出完整文件系统证明。

## 4. 可实施的最小平台适配

建议只新增 backup 域的 scoped publisher 及 `_windows.go` / Unix 文件，不向 desktop 反向依赖，不改 S03.1 的租约所有权。它接收已经校验的专属根、规范生成的两个直接子名称和预期文件身份；不接收 Web 任意路径，不生成新全局配置或维护命令。所有新方法/字段/关键分支仍按项目要求有中文注释、类型与安全中文日志。

1. **限定第一批支持矩阵。** 推荐 Windows 先验收普通用户可写的本地固定盘 NTFS；识别来自实际根句柄，不能只比较盘符字符串。UNC、映射网络盘、重解析跳转、RAM disk 和未验文件系统不自动声称持久支持。ReFS/FAT/exFAT 不必永远拒绝，但应先保持未支持/待实测，而不是仅因 API 可调用就纳入同一证据。DRIVE_FIXED 也不证明底层控制器有掉电保护。
2. **控制身份和命名。** 保持根独占锁、私密 DACL、稳定根句柄和每次关键操作前后身份检查；源 `.pending-<fileId>` 与目标 `b-<fileId>` 都只能是同根直接子目录。排除重解析点及未知成员；随机名碰撞直接失败。不同盘、最终目标已存在或任何别名/成员检查异常时不覆盖、不降级 copy/delete。路径式 Win32 API 的保护边界是本应用受控私密根，不宣称能抵御同权限管理员持续竞态篡改。
3. **逐个准备普通文件。** 新目标用 CREATE_NEW/O_EXCL 语义创建；推荐 Windows 写句柄使用 FILE_FLAG_WRITE_THROUGH，不加 NO_BUFFERING。写入长度检查→文件 Sync（即 FlushFileBuffers）→Close，每个错误都记录到该次发布结果；只读重验连接/摘要句柄也必须在目录移动前关闭。owner、DB、manifest 都处理，不只同步 DB；不以文件最后一次 read/hash 代替写句柄 flush。S03.1 源文件仍只经原租约读取和 Close。
4. **先完成 pending 内的精确成员。** manifest 临时名到最终名也使用同卷无覆盖写穿移动；在搬动目录前核验固定三个成员及其预期长度/hash/角色，不让读者看到半份 manifest。根标记的首次发布也应走同样的无覆盖文件协议，而非用可覆盖的配置方法冒充；不在本研究增加标记格式。
5. **目录发布仅调用 WRITE_THROUGH。** UTF-16 路径按项目已解析的根构造，调用 `windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)`；**不置 REPLACE_EXISTING、COPY_ALLOWED 或 DELAY_UNTIL_REBOOT**，也不在失败后换 os.Rename/copy/remove。目标是否存在的预检查不是无覆盖保证，必须让系统调用本身在碰撞时失败。保留返回的受控阶段/错误类别，不把 sharing violation、权限错误或设备错误转换为成功。
6. **操作完成后复核，再短事务登记。** 重新验证根/最终目录/成员身份和内容，且全部规定的 flush、Close、写穿移动都成功，才形成“OS 已确认发布协议”的结果并进入原短事务登记。Windows 不额外强制执行 `os.Open(dir).Sync()`；这是采用不同平台协议，不能写成“目录 Sync 失败已忽略”。任何结果未知仍保存原 run/所有权供对账，不自动新建备份或先删旧份。
7. **Unix 保留自己的协议。** 文件 fsync、正确的同文件系统无覆盖移动，以及受影响父目录的 fsync 分别落实；不能因补 Windows 而把 Unix 目录同步删除。实际原语/文件系统不支持时按其平台错误处理。

此方案不需要升级 x/sys、直接 syscall.SyscallN、整卷管理员 flush、TxF 或新系统服务。函数层结果应区分“文件 flush 失败”“写穿移动失败”“移动后身份/内容不符”“结果待对账”，而非只返回一个容易误读为物理断电认证的布尔值；具体内部类型由实施切片冻结。

## 5. completed、轮转与原研究的接缝

本文件不修改上一份持久设计。推荐主线冻结时仅做以下有范围的解释，而非追加一套低保证模式：

- “必要持久化成功”是**对应平台协议**成功。Windows 为上述文件 flush + 无覆盖 WRITE_THROUGH 移动 + 后验身份/内容检查；不要求额外一个没有明确等价语义的 POSIX 目录同步调用。其结果不能在宣传、UI或测试报告中称“已通过真实断电验证”。
- 如果保留 `durabilityVerified` 名称，必须在正式契约写明其只表示规定 OS 发布协议已成功，不代表硬件认证；更清楚的字段命名可在 API 冻结前选择，但本研究不新增 DTO 或改 schema。不能一处表示 API 协议、另一处又当成真实掉电实验证据。
- 无覆盖移动失败、Sync/Close 失败、根/卷身份不明及最终目录校验失败，继续保持原“不得 completed / 不授权轮转”的要求。目录已可能移动但返回/提交结果未知时，仍只对账原 ID；既有正常完成事实不能因清理源租约失败被改成可重跑的新任务。
- 重启对账只能重新验证和补足所需平台调用；重启后文件可读证明本次恢复看到了它，**不反向证明此前经历了掉电且写入已落物理介质**。不能以一次正常重启清除未通过的目标平台断电门禁。
- 新成功后才轮转、至少保留最新实际有效 guard、活跃下载跳过、未知项不删的边界保持。但这些逻辑本身不是跨 SQLite/文件系统的掉电原子事务，仍需第6节故障验收；严格要求更强掉电承诺时不可只靠此适配宣布满足。

## 6. 真实验证方法与证据等级

本轮**没有创建动态 Windows fixture、运行功能测试、强杀进程或断电**。下面是实施时的可执行门禁建议，不能写成已通过。构建/测试/临时工具一律由 `python -B scripts/project_env.py -- ...` 运行，只创建仓库 `.runtime/` 下已确认自有的新随机 fixture；浏览器或用户运行实例不参与。

| 层级 | 方法与必须断言 | 能与不能证明 |
| --- | --- | --- |
| A：API与普通用户能力 | 在本地 NTFS fixture 创建根/受限 DACL、三个随机内容文件；实际 FlushFileBuffers、关闭、仅 WRITE_THROUGH 移动；独立核验内容/hash/身份和源名消失。预置目标目录、文件、重解析对象 sentinel，验证失败且 sentinel 原字节/目录成员不变；跨卷路径被拒绝且没有副本遗留。记录 OS build、实际 Go/x/sys、卷文件系统、调用阶段/受控 errno。 | 证明真实系统调用和无覆盖行为；不是掉电持久性证明。普通用户运行，不要求提升权限；不使用现有用户备份根。 |
| B：错误与生命周期 | 故障注入文件 Sync/Close、manifest 移动、最终目录移动、后验核验、DB登记失败；真实占用目标句柄产生 sharing violation，保持只读/权限拒绝场景；检查 no completed/no轮转、句柄最终释放，旧 guard/sentinel 保留。 | 注入证明分支，真实句柄/ACL证明目标系统拒绝路径；两者分开记账。不能把 mock WRITE_THROUGH 返回nil算作系统验证。 |
| C：应用进程崩溃 | 独立子进程仅处理自有 fixture，在“只写部分/各文件flush后/manifest后/MoveFileEx返回前后/DB提交前后/删除中”建立 barrier，按已验证 PID/启动身份终止该子进程；同一fixture重新对账。MoveFileEx调用中被终止应允许源/目标状态待核对，不能预设只有一个确定结果。断言不重复VACUUM、未完成不暴露下载、未知成员不碰、最新好副本不误删。 | 证明进程终止及对账边界；Windows内核和设备缓存仍存活，不能外推系统崩溃或掉电。 |
| D：OS崩溃/虚拟机突停 | 独立可销毁 VM 与测试 NTFS 数据盘，镜像/快照只用于隔离；记录虚拟磁盘及宿主缓存策略，在上述barrier附近多轮非正常掉电式停机后启动，检查目录/DB/hash/guard与对账。 | 可验证该虚拟环境的OS/文件系统崩溃恢复；宿主仍可能替客体保留并写出缓存，不等于物理断电。正常关机/正常重启不算此层。 |
| E：目标设备真实断电 | 用户另行安排专用测试机/可牺牲介质，记录NTFS/存储驱动、设备缓存/断电保护、供电切断方式和重复次数；确认仅测试卷/实例，无生产资料。断电重启后使用独立校验器核对数据库完整性/外键、清单/hash、目录状态、最后有效副本与错误事实。 | 为该硬件组合提供实验证据，不产生任意Windows/USB/RAID/NAS的普适保证；未提供目标设备则始终待验。 |

原子可见性测试应从只认“DB retained且后验可校验产物”的读取协议验证，不以文件管理器瞬间看到目录推断原子性。C/D/E对账必须覆盖根临时不可用、最终目录存在但DB未登记、DB有记录但目录缺失、错误storeId/未知成员，并保留安全失败证据。测试生成的临时Go单测源码完成后清除，稳定验证脚本可保留；无需新增任何生产CLI或全局测试开关。

## 7. 官方来源、核对方式与未解决项

2026-11-23实际读取 Microsoft Learn 与 Go 官方包文档/源码正文；搜索工具在本轮未返回可引用正文，后改为通过项目包装器启动的只读 HTTP 请求提取官方页面相关段落。未引用社区对 MoveFileEx 的推测作为保证，未穷举所有未文档化 NT API。只采用下面已成功取得内容的来源；不存在“因一个未找到的页面而证明某能力绝对不存在”的结论。

- [M1 — Microsoft MoveFileExW](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw)：参数、同驱动器目录限制、无覆盖/现有目录行为及 WRITE_THROUGH。
- [M2 — Microsoft FlushFileBuffers](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-flushfilebuffers)：写权限、缓冲 flush、整卷管理员条件。
- [M3 — Microsoft CreateFileW，Caching Behavior / Directories](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)：WRITE_THROUGH与NO_BUFFERING、NTFS相关metadata、硬件限制、目录句柄与权限。
- [M4 — Microsoft File Caching](https://learn.microsoft.com/en-us/windows/win32/fileio/file-caching)：文件系统metadata缓存、显式flush/写穿、突然断电风险。
- [M5 — Microsoft SetFileInformationByHandle](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-setfileinformationbyhandle)：FileRenameInfo能力及适当访问权限；不作为更强持久保证。
- [G1 — Go os.Rename / File.Sync](https://pkg.go.dev/os)：非Unix Rename原子性限制及Sync的公共契约。
- [G2 — Go Windows Rename官方源码](https://go.dev/src/internal/syscall/windows/syscall_windows.go)：MoveFileEx flags；本轮另定向核对实际 `.tools/go` 的相同代码路径、`os/file_posix.go` 和 `syscall/syscall_windows.go`。
- 固定 x/sys 来源为本地模块 `golang.org/x/sys@v0.50.0/windows/{types_windows.go,zsyscall_windows.go}`，对应项目 `go.mod/go.sum`；本轮没有更新依赖。

尚待主线冻结：是否接受OS确认的Windows发布协议作为completed边界及相应DTO文案；第一批Windows文件系统/存储支持矩阵；真实目标的D/E层验收。上述取舍不影响S03.1先实施，不回改本轮唯一授权文件之外的任何内容。

## 本轮验证记录

- 仓库源码/固定x/sys/实际Go版本与官方资料：已只读核对，文档完成后读回。
- Lint / TypeCheck / 构建 / 功能测试 / 动态fixture / 断电验证：本轮均未执行；研究文档不能代替这些门禁。
- 无业务代码、依赖、正式spec、任务指针或persistent设计修改；无Docker、用户实例、浏览器、提交或部署。
