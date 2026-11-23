# S03.2 Windows 发布原语独立 fixture 实测

日期：2026-11-23。范围：受限 Phase 1.2 平台能力实验。**本轮实测的是临时 Go 探针直接调用 Windows 系统原语，不是尚未实现的产品 backup publisher，不证明进程崩溃、系统崩溃或真实断电持久性。** 不修改已交回平台/持久设计、PRD、spec、任务指针、业务代码或依赖。

## 1. 实际结果

四个真实场景、19项布尔断言全部通过，探针退出码0、标准错误0字节。没有 mock 或替代系统调用。运行时间为 `2026-11-23T03:41:31.0945265Z`（北京时间11:41:31）。

| 环境项 | 实际读取结果 |
| --- | --- |
| Go / 进程架构 | go1.27.4 / windows/amd64 |
| Windows 内核版本 | 10.0.19045，来自 RtlGetVersion；未读取或输出主机名 |
| 实际 fixture 文件系统 / 驱动器类别 | NTFS / fixed，来自实际根句柄和卷路径查询；不输出盘符、卷标或序列号 |
| 当前进程令牌 | limited_uac，TokenElevationType=3，TokenElevation为未提升；未取用户名或SID |
| 独立标准用户账号证明 | **未完成**。本次是未提升的UAC受限令牌，不把它改称独立标准账号测试；JSON中standardUserProven=false |
| 实际链接依赖 | golang.org/x/sys v0.50.0，运行时读取构建信息并精确断言 |
| MoveFileExW实际标志 | 8，仅MOVEFILE_WRITE_THROUGH；未置REPLACE_EXISTING、COPY_ALLOWED、DELAY_UNTIL_REBOOT |

| 场景 | 真实系统结果 | 已通过的断言 |
| --- | --- | --- |
| 同卷目录发布 | MoveFileExW成功，错误码0 | 源名称确实消失；最终目录恰有三个普通成员；各文件长度和SHA-256与固定合成内容一致。 |
| 目标已为文件 | 调用失败，ERROR_ALREADY_EXISTS=183 | 目标文件身份不变、sentinel字节不变；源三个成员和内容保持原样。 |
| 目标已为目录 | 调用失败，ERROR_ALREADY_EXISTS=183 | 原目标目录身份不变，仍只有原sentinel成员、字节不变；源三个成员和内容保持原样。 |
| 成员句柄拒绝DELETE共享 | 以GENERIC_READ打开源snapshot成员，share只含READ/WRITE；移动真实失败，ERROR_ACCESS_DENIED=5 | 持有时源原样且目标不存在；关闭该句柄成功；对**同一个源名、目标名和flags=8**重试后成功，源名消失且最终内容/摘要一致。 |

最后一项返回的是5而非32。本fixture证明该次受控占用关闭后相同移动能够成功；不能将任意ERROR_ACCESS_DENIED都解释为“可重试的句柄占用”，也不能只识别ERROR_SHARING_VIOLATION才承认Windows存在共享相关拒绝。产品应保存受控阶段/实际错误类别，不能据本次结果引入无限重试或覆盖fallback。

## 2. 探针实际执行方式

所有命令均从仓库根经项目包装器运行。只创建新专属目录：

```text
.runtime/verify/s03-platform-publish-probe-20261123T033855Z-1401c3e1/
```

探针要求fixture标签匹配固定安全格式，将根限制为自身可执行文件目录的直接新子目录，排他Mkdir；已有根直接拒绝，不复用任何用户数据或已有备份根。实际使用标签为`fixture-primary`，它已在完成时删除。

- 临时Go源码只使用标准库及项目固定x/sys，`go build -mod=readonly -trimpath`构建，不编辑go.mod/go.sum或安装新依赖。
- 每个目标普通文件都用CreateFileW的CREATE_NEW、FILE_FLAG_WRITE_THROUGH创建，写入有界合成字节，再真实调用Go File.Sync（当前Windows实现为FlushFileBuffers），检查Close结果。没有先读回就宣称已flush。
- 全部场景直接调用固定x/sys的windows.MoveFileEx，仅传WRITE_THROUGH。碰撞场景让Win32调用真实失败，不以预检或mock代替。
- 成员占用用真实CreateFileW句柄和不包含FILE_SHARE_DELETE的共享位，等待调用返回后关闭，再按同一意图重试。没有对用户进程、真实数据库或其它代理目录操作。
- 每次排他创建即登记卷/文件索引；成功rename只更新已登记对象的位置。收尾按原创建顺序逆序核对稳定身份，逐个os.Remove，不调用RemoveAll、不扫描前缀清理；遇未知或替换对象会保留并失败。

合成材料如下。`snapshot.db`只是固定字节文件，**不是SQLite快照**，本实验没有执行SQLite完整性/外键/schema校验；owner与manifest也只是探针材料，不冒充产品格式。

| 角色 | 字节数 | SHA-256 |
| --- | --- | --- |
| owner | 27 | 1142bdae519190954add1fed63e58ec417d97df961b2bd4a52827bd498e98492 |
| database | 8704 | f0109839d1769a11c88b59f70f452fa3adf998bd0e809b60c936b9b0f5c16e54 |
| manifest | 37 | 1e5530c6510a0b1ad32b9be3c3eb2a1ea920e9e28b3e5f81c410def443e2684b |

独立Python读回按固定字节重新计算上述摘要，与Go报告相同；不是仅相信探针自报“通过”。

## 3. 资源回收与保留证据

普通文件Sync成功14次，写句柄Close成功14次；额外占用的真实成员句柄关闭断言通过。已登记20个目录/文件全部核对内核身份后精确删除，20/20回收；再次独立读取确认`fixture-primary`不存在。无孤立候选、sentinel或活动句柄留在fixture中。

专属运行目录内保留以下产物，均没有绝对路径、SID、用户名或原始异常文本的JSON/安全日志：

- `result-primary.json`：实际环境、4个场景/19项断言、数值错误码、合成摘要、14次Sync/Close和20/20回收；crashTested、powerLossTested、productPublisherTested均明确false。
- `metadata.json`：构建/退出结果、独立摘要读回、源码删除与fixture根不存在证明、源码和可执行文件摘要。
- `safe.log`：固定中文阶段、场景状态、错误码、回收和源码清除结果，不记录真实路径或秘密。
- `publish-probe.exe`：原探针二进制，用于在同一受控运行目录下以新标签复跑；未打包为产品或新CLI。

原临时源码`probe.go`已先核对摘要再删除，重新读取确认不存在，专属目录内无任何`.go`源文件。没有删除共享Go缓存或其他代理文件。

| 产物 | SHA-256 |
| --- | --- |
| 已删除的临时源码 | e491d0eed9626f8d2432417f20e76f3ea8ab932836cfff3e48e1839750441bc0 |
| 保留的探针二进制 | ab150a83bcacb115ddd016f10a831e30e42ab7c99b23a5b48b16d72bc9c6977c |

安全证据读回另行扫描Windows绝对路径、SID及运行时用户名标记，未命中。包装器自身的控制台诊断会显示仓库缓存根，但没有被写进上述保留JSON/安全日志。

## 4. 原命令与复现

历史构建命令如下。初次通过包装器按短名寻找gofmt失败，随后改用已存在的仓库工具链显式路径成功；没有安装或修改全局工具。**源码现已删除，以下构建命令仅为本次执行记录，不能假称无需源码仍可重新编译。**

```powershell
python -B scripts/project_env.py -- .tools/go/bin/gofmt.exe -w .runtime/verify/s03-platform-publish-probe-20261123T033855Z-1401c3e1/probe.go
python -B scripts/project_env.py -- go build -mod=readonly -trimpath -o .runtime/verify/s03-platform-publish-probe-20261123T033855Z-1401c3e1/publish-probe.exe .runtime/verify/s03-platform-publish-probe-20261123T033855Z-1401c3e1/probe.go
```

当前可直接复跑保留的**原二进制**；先核对其上列摘要，从仓库根运行并使用新的安全标签，程序会排他创建和精确清理该次fixture。下列命令未在本轮追加执行，避免把示例误记成第二轮测试：

```powershell
python -B scripts/project_env.py -- .runtime/verify/s03-platform-publish-probe-20261123T033855Z-1401c3e1/publish-probe.exe fixture-replay-01
```

程序输出结构化JSON；退出0表示本探针场景及回收全部满足，非0需查看固定stage/数值错误及cleanup，不自动覆盖现有根或改用其他移动方式。复跑只能证明复跑时的环境；修改OS、文件系统、令牌或依赖后必须重新记录，不能沿用本次通过标签。

## 5. 能用于冻结的结论与不能外推的部分

本次在记录的Windows/NTFS/未提升UAC令牌环境中，证实当前固定依赖能够执行“逐文件flush/close、同卷目录无覆盖WRITE_THROUGH发布”，两种碰撞和真实句柄占用都有可重复辨识的拒绝路径。这可以作为实施Windows平台publisher的具体能力证据，**不需要新增依赖、整卷flush、提权或管理员开关**。

本实验没有验证产品根隔离、受限ACL继承/修改或拒绝矩阵、真实SQLite快照、manifest正式协议、持久run状态、重启对账、下载租约与轮转，也没有执行跨卷复制、磁盘故障、服务/Docker或用户实例操作。这些都没有以mock或普通文件fixture标成产品passed；受限ACL和独立标准账号亦不在本次已通过范围。

尤其没有执行进程强杀、OS崩溃、VM突停或物理断电。MoveFileExW返回成功、文件能读回及20/20删除只证明本次正常执行与资源收尾，不能反推目录metadata在任意掉电点的顺序或整树原子性，不能把OS确认的写穿请求改称硬件掉电认证。后续正式语义仍须结合`platform-backup-publish.md`的一手文档边界和目标环境故障证据冻结。

## Verification

- 临时探针gofmt、固定依赖只读构建：通过。
- 真实系统原语：4场景/19断言通过；14次Sync/Close、20/20精确回收；退出0。
- 独立摘要/JSON/安全日志读回、源码已删除/fixture根不存在：通过。
- 产品Lint / TypeCheck / 全套测试：未运行，本片不改业务代码；不冒称产品门禁通过。
- 崩溃/断电/独立标准账号/受限ACL完整验证：未执行，不属于已通过证据。
