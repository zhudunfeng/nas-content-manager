# Research: Windows desktop 图标资源与原生句柄生命周期

- Query: 如何在不引入桌面框架或系统资源编译器依赖的前提下，为 Go Windows desktop 的 EXE、托盘和窗口类添加统一品牌图标？
- Scope: mixed；只读研究现有代码、Go/Microsoft 官方文档及拟用再生工具的固定版本源码。
- Date: 2026-11-19

## Findings

### 当前代码位置与调用关系

已优先使用仓库 CodeGraph：`query runTray` 定位 `internal/desktop/shell_windows.go:191`，`callers runTray` 确认由 `internal/desktop/launcher.go:54 Main` 调用。命令通过 `python -B scripts/project_env.py -- node <已有 npm-shim.js>` 执行；直接调用 `codegraph` 的 Windows 批处理包装会被项目环境入口拒绝，应使用原生 Node EXE 和现有 CLI JavaScript。

| 文件 | 当前职责与证据 |
| --- | --- |
| `web/public/favicon.svg:1` | 64×64 深色圆角底 `#111418`，薄荷色 `#5eead4` 双书架，可复用形状与颜色。 |
| `cmd/nas-desktop/main_windows.go:12` | Windows 薄入口，将生命周期交由 desktop.Main 管理。 |
| `internal/desktop/shell_windows.go:193` | 当前调用 `LoadIconW(0, 32512)`，实际为系统 `IDI_APPLICATION` 通用图标。 |
| `internal/desktop/shell_windows.go:194` | 将同一图标放入窗口类 Icon，SmallIcon 字段尚未设置。 |
| `internal/desktop/shell_windows.go:209` | NOTIFYICONDATA.Icon 使用上述通用图标。 |
| `internal/desktop/shell_windows.go:239` | Explorer 重启后重新执行 addTrayIcon，复用 trayData。 |
| `scripts/release.py:301` | 通过 `go build ... -H=windowsgui ... ./cmd/nas-desktop` 生成发布桌面 EXE。 |

### 三种常见方案与本仓库适配

1. **提交预生成、按平台架构命名的 COFF `.syso`，并保存 ICO 与再生入口（建议）**：Go 工具链自动将包内系统对象加入归档及链接，普通 `go build ./cmd/nas-desktop` 和既有 release.py 无需额外编译器、CGO 或网络。使用 `resource_windows_amd64.syso` 让 Linux 构建忽略该 Windows 对象。本仓库当前明确的桌面目标为 Windows amd64。
2. **每次构建调用 rsrc/go-winres 或 windres**：便于即时生成资源，但会把下载工具或系统资源编译器变成每次普通构建/CI 的前提；与本次最小目标不符。固定 `go run github.com/akavel/rsrc@v0.10.2` 只作为维护者显式再生步骤合理，不必修改主 go.mod。
3. **运行时从内存 ICO 创建 HICON**：可以在没有 PE 资源的测试进程中显示托盘，但单独使用它不能给 Explorer 提供 EXE 文件图标，仍需 PE 资源；会带来第二条加载路径及额外解析/所有权逻辑。本次不建议只用此方案。

### 多尺寸 ICO、资源 ID 与可重现性

- Microsoft 图标文档建议一个资源包含多帧，常见标准尺寸为 16、32、48、256 像素，32 位 ARGB 保留透明度。可补 20、24、40、64、96、128 像素，减少缩放损失。当前支持目标 Windows 11 可使用 PNG 压缩帧；本轮仍需对实际生成 ICO 运行加载验证，不能只凭目录存在宣称显示成功。
- 固定版本 rsrc 的 `rsrc/rsrc.go:32` 将 ID 计数器置零；在**没有传入 manifest**、只传入一个 ICO 的情况下，`addIcon:90` 先分配 group ID 1，然后帧 RT_ICON 获得 ID 2 起，`addIcon:97` 写入 RT_GROUP_ICON。运行时可通过 `MAKEINTRESOURCE(1)` 加载该 group。
- **新增 manifest 会改变分配顺序**：`Embed:51` 先给 manifest 分配 ID，所以同一工具下 group 将变为 2。应在再生和验证时检查资源 ID 契约，不能无说明地给原命令加 manifest。
- `ico/ico.go:95 DecodeHeaders` 只读取 ICO 目录；`rsrc/rsrc.go:93` 对每帧创建 SectionReader 并原样写入 RT_ICON，因此不会把 PNG 数据误当 BMP 解码。
- `coff/coff.go NewRSRC` 将 TimeDateStamp 固定为 0。固定 ICO 字节、arch 和工具版本可得到稳定输出；实现仍应实际两次再生比较 SHA-256，而不只从源码推测可重现性。
- `.syso` 是有架构的二进制输入，应记录生成命令、来源 ICO 和工具版本；不应把任意下载对象作为不可追溯输入提交。

### 重要：不要给同一资源的大小图标同时使用 LR_SHARED

Microsoft LoadImageW 文档明确：带 LR_SHARED 时按资源名查找首个缓存图像，**不考虑本次请求的尺寸**。因此同一个 group ID 先加载大图再加载小图，不能依靠 `LR_SHARED` 保证两个句柄分别匹配尺寸。

建议最小原生路径：

1. 取得当前模块句柄，对 group ID 1 调用 `LoadImageW`，type 为 IMAGE_ICON，flags 为 0，分别使用系统大图标 `SM_CXICON/SM_CYICON` 与小图标 `SM_CXSMICON/SM_CYSMICON` 尺寸。
2. 将独立的大、小 HICON 分别赋给窗口类 Icon 和 SmallIcon；托盘使用小 HICON。
3. 不带 LR_SHARED 得到的句柄归应用所有，需要 `DestroyIcon`。应先删除托盘图标、销毁窗口、注销窗口类，再销毁 HICON；部分加载/注册失败也必须释放已经成功创建的图标。不要把它们当文件句柄 CloseHandle。
4. 若坚持使用共享图标，`LoadIconW` 或带 LR_SHARED 的 LoadImageW 得到的共享 HICON **不可 DestroyIcon**；但上述缓存尺寸问题仍存在，不是本次建议。
5. `LoadIconMetric` 是高 DPI 图标选择的另一官方 API，LIM_SMALL/LIM_LARGE 分别对应小/大系统尺寸，返回句柄也需 DestroyIcon；它增加 Comctl32 调用且本次非必要。微软说明它在没有精确尺寸时优先从较大图缩小。

资源嵌入当前 EXE，不需要打开或长期持有外置 ICO 文件，不会额外锁住程序目录中的图标文件；仍应回归验证退出后 EXE 可替换，避免图标生命周期改变现有清理顺序。

### 缺资源行为与开发入口

- `go run ./cmd/nas-desktop` 和按包路径构建可纳入匹配的 `.syso`；`go run cmd/nas-desktop/main_windows.go` 这种显式源文件方式不能依赖包目录其他构建输入。
- `go test ./internal/desktop` 的测试 EXE 属于 internal/desktop 包，并不会自动纳入另一个 cmd 包的资源；若临时测试调用真实 runTray，应使用独立 desktop EXE 集成测试，或将缺资源作为预期失败测试。
- 当前任务倾向资源缺失时返回明确中文错误，避免成品静默回到通用图标；这可以实施，但须保留 headless 路径可用于无托盘测试，并在开发文档中说明按包路径启动。不要因测试 EXE 无资源而取消资源存在验证。

## External references

以下 Microsoft/Go 官方页面在 2026-11-19 通过 HTTPS 直接读取核验；Web 工具搜索/打开未返回可见内容，改用项目环境内 Python 标准库读取指定官方页面，没有安装外部工具。

- [Go command documentation](https://go.dev/cmd/go/)：filetype 将 `.syso` 定义为 system object files，`go list` 的 SysoFiles 是加入 archive 的对象。
- [Go build source](https://go.dev/src/go/build/build.go)：包内文件与操作系统/架构匹配规则的官方实现。
- [Microsoft LoadImageW](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-loadimagew)：资源 ID、尺寸参数、LR_SHARED 缓存及销毁规则。
- [Microsoft LoadIconW](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-loadiconw)：仅大系统尺寸、被 LoadImage 取代及共享句柄语义。
- [Microsoft DestroyIcon](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-destroyicon)：共享图标禁止销毁。
- [Microsoft LoadIconMetric](https://learn.microsoft.com/en-us/windows/win32/api/commctrl/nf-commctrl-loadiconmetric)：系统大/小尺寸、高 DPI 选择及返回图标所有权。
- [Microsoft About Icons](https://learn.microsoft.com/en-us/windows/win32/menurc/about-icons)：多尺寸 ICO、常见标准尺寸和图像选择。
- [Microsoft PE Format](https://learn.microsoft.com/en-us/windows/win32/debug/pe-format)：COFF、资源目录与 AMD64 RVA relocation。

拟用工具来源，属于固定版本的作者源码，非第三方教程：

- [rsrc v0.10.2 entry](https://github.com/akavel/rsrc/blob/v0.10.2/rsrc.go)：CLI 的 ico/arch/o 参数。
- [rsrc resource builder](https://github.com/akavel/rsrc/blob/v0.10.2/rsrc/rsrc.go)：数值 ID 分配及各帧直接复制。
- [rsrc ICO parser](https://github.com/akavel/rsrc/blob/v0.10.2/ico/ico.go)：仅解析 ICO 目录。
- [rsrc COFF builder](https://github.com/akavel/rsrc/blob/v0.10.2/coff/coff.go)：固定零时间戳与资源对象构建。

## Related specs

- `.trellis/spec/backend/cross-platform-contract.md`：原生启动器、Linux 隔离、普通构建与发布、退出释放 EXE、项目内缓存。
- `.trellis/spec/backend/runtime-contract.md`：中文方法/字段/关键逻辑注释、真实验证边界。
- `.trellis/spec/guides/code-reuse-thinking-guide.md`：沿用已有品牌与避免资源生成/运行时多套事实来源。

## Caveats / Not Found

- 本研究未生成或运行资源、未执行托盘 UI 验收、未更改代码、未验证 rsrc 在当前工具链上的实际构建；这些属于实现和质量检查。
- 仅 Windows amd64 属于现有桌面发行契约；ARM64 应另行生成对应对象并实际验证，不能复用 amd64 COFF。
- 系统 MessageBox 的感叹号/错误图标属于消息类别提示，不要求随应用图标一并替换；窗口/Explorer/托盘是本次范围。
- Windows Explorer 可能缓存旧 EXE 图标，测试应先用新文件名/新目录验证资源，再区分缓存现象；不应为验收全局清空系统图标缓存或重启 Explorer。
