//go:build windows

package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows API 延迟从系统目录加载，避免工作目录中的同名 DLL 劫持。
var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	shell32        = windows.NewLazySystemDLL("shell32.dll")
	ole32          = windows.NewLazySystemDLL("ole32.dll")
	kernel32       = windows.NewLazySystemDLL("kernel32.dll")
	messageBox     = user32.NewProc("MessageBoxW")
	postMessage    = user32.NewProc("PostMessageW")
	trayWindow     uintptr        // 当前消息窗口句柄，只在锁定的 GUI 线程设置。
	trayController *controller    // 当前进程唯一的托盘控制器。
	taskbarCreated uint32         // Explorer 重启后的图标恢复消息。
	trayData       notifyIconData // 固定图标数据，消息窗口生存期间有效。
)

// windowClass 是 Win32 Unicode 消息窗口类定义。
type windowClass struct {
	Size        uint32  // 本结构字节数。
	Style       uint32  // 窗口类样式。
	Procedure   uintptr // 系统消息回调。
	ClassExtra  int32   // 类附加空间。
	WindowExtra int32   // 窗口附加空间。
	Instance    uintptr // 当前模块句柄。
	Icon        uintptr // 主图标句柄。
	Cursor      uintptr // 鼠标指针句柄。
	Background  uintptr // 背景画刷。
	Menu        *uint16 // 不使用窗口菜单。
	Name        *uint16 // 稳定窗口类名。
	SmallIcon   uintptr // 小图标句柄。
}

// nativeMessage 是 Win32 消息循环中由系统填充的消息。
type nativeMessage struct {
	Window  uintptr // 目标窗口。
	Message uint32  // 消息种类。
	WParam  uintptr // 第一系统消息参数。
	LParam  uintptr // 第二系统消息参数。
	Time    uint32  // 系统消息时间。
	X       int32   // 鼠标横坐标。
	Y       int32   // 鼠标纵坐标。
	Private uint32  // 系统保留字段。
}

// notifyIconData 是系统通知区域 Unicode 图标数据。
type notifyIconData struct {
	Size        uint32       // 结构版本由字节数确定。
	Window      uintptr      // 事件目标窗口。
	ID          uint32       // 本进程固定图标 ID。
	Flags       uint32       // 有效图标字段。
	Callback    uint32       // 托盘回调消息。
	Icon        uintptr      // 系统应用图标。
	Tip         [128]uint16  // 中文悬停说明。
	State       uint32       // 可选显示状态。
	StateMask   uint32       // 状态字段掩码。
	Info        [256]uint16  // 不使用气泡正文。
	Version     uint32       // 不启用额外事件版本。
	InfoTitle   [64]uint16   // 不使用气泡标题。
	InfoFlags   uint32       // 不使用气泡选项。
	GUID        windows.GUID // 不使用全局图标身份。
	BalloonIcon uintptr      // 不使用自定义气泡图标。
}

// wide 将内部确定的中文文字转为 Windows Unicode 字符串。
func wide(value string) *uint16 { result, _ := windows.UTF16PtrFromString(value); return result }

// showMessage 在 GUI 模式显示可恢复的中文提示，不隐藏启动失败。
func showMessage(title string, message string, failure bool) {
	flags := uintptr(0x40)
	if failure {
		flags = 0x10
	}
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(wide(message))), uintptr(unsafe.Pointer(wide(title))), flags)
}

// confirmMessage 由本机用户明确确认模式切换、数据位置变更等操作。
func confirmMessage(owner uintptr, title string, message string) bool {
	result, _, _ := messageBox.Call(owner, uintptr(unsafe.Pointer(wide(message))), uintptr(unsafe.Pointer(wide(title))), 0x24)
	return result == 6
}

// shellOpen 仅用于已校验的本机路径或回环网页，不经过 cmd/PowerShell。
func shellOpen(target string) error {
	result, _, _ := shell32.NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(wide("open"))), uintptr(unsafe.Pointer(wide(target))), 0, 0, 1)
	if result <= 32 {
		return errors.New("系统未能打开指定的本机页面或目录")
	}
	return nil
}

// openLocalURL 严格限制浏览器入口为本实例回环地址，不接受任意 URL。
func openLocalURL(address string) error {
	if !strings.HasPrefix(address, "http://127.0.0.1:") {
		return errors.New("拒绝打开非本机的启动器地址")
	}
	return shellOpen(address)
}

// chooseFolder 使用系统 IFileOpenDialog 授权目录，取消不会变为启动错误。
func chooseFolder(owner uintptr, title string) (string, error) {
	result, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2)
	if uint32(result)&0x80000000 != 0 {
		return "", errors.New("无法初始化 Windows 目录选择器")
	}
	defer ole32.NewProc("CoUninitialize").Call()
	clsid := windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iid := windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	var dialog *comObject
	result, _, _ = ole32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&dialog)))
	if uint32(result)&0x80000000 != 0 || dialog == nil {
		return "", errors.New("Windows 目录选择器不可用")
	}
	defer comValue(dialog, 2, 0)
	var options uint32
	if comPointer(dialog, 10, unsafe.Pointer(&options))&0x80000000 != 0 {
		return "", errors.New("无法读取目录选择器选项")
	}
	// 只选择真实文件系统中存在的文件夹，不接受虚拟 Shell 对象。
	if comValue(dialog, 9, uintptr(options|0x20|0x40|0x800))&0x80000000 != 0 || comPointer(dialog, 17, unsafe.Pointer(wide(title)))&0x80000000 != 0 {
		return "", errors.New("无法设置目录选择器")
	}
	hresult := comValue(dialog, 3, owner)
	if hresult == 0x800704C7 {
		return "", nil
	}
	if hresult&0x80000000 != 0 {
		return "", errors.New("目录选择失败，请重试")
	}
	var item *comObject
	if comPointer(dialog, 20, unsafe.Pointer(&item))&0x80000000 != 0 || item == nil {
		return "", errors.New("无法获取所选目录")
	}
	defer comValue(item, 2, 0)
	var name *uint16
	if comValuePointer(item, 5, 0x80058000, unsafe.Pointer(&name))&0x80000000 != 0 || name == nil {
		return "", errors.New("所选目录不是可访问的本机文件系统位置")
	}
	defer ole32.NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(name)))
	return readableRoot(windows.UTF16PtrToString(name))
}

// comObject 是系统 COM 接口，不把 Go 指针转为可跨调用保存的整数地址。
type comObject struct {
	VTable *[29]uintptr /* IUnknown/IFileDialog/IShellItem 的固定系统方法表。 */
}

// comValue 将数值参数直接交给固定系统 COM 方法，避免延迟转换 Go 指针。
func comValue(object *comObject, index int, value uintptr) uint32 {
	result, _, _ := syscall.SyscallN(object.VTable[index], uintptr(unsafe.Pointer(object)), value)
	return uint32(result)
}

// comPointer 在系统调用表达式中转换指针，保证目录选择器输出的 GC 生命周期。
func comPointer(object *comObject, index int, value unsafe.Pointer) uint32 {
	result, _, _ := syscall.SyscallN(object.VTable[index], uintptr(unsafe.Pointer(object)), uintptr(value))
	return uint32(result)
}

// comValuePointer 安全传入 Shell 显示名称格式和本机结果指针。
func comValuePointer(object *comObject, index int, value uintptr, pointer unsafe.Pointer) uint32 {
	result, _, _ := syscall.SyscallN(object.VTable[index], uintptr(unsafe.Pointer(object)), value, uintptr(pointer))
	return uint32(result)
}

// addTrayIcon 创建或恢复通知图标，Explorer 重启不丢失退出入口。
func addTrayIcon() error {
	result, _, _ := shell32.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&trayData)))
	if result == 0 {
		return errors.New("无法创建系统托盘图标")
	}
	return nil
}

// loadApplicationIcon 按系统尺寸加载内嵌品牌图标，返回的独立句柄由托盘线程释放。
func loadApplicationIcon(instance uintptr, small bool) (uintptr, error) {
	widthMetric, heightMetric := uintptr(11), uintptr(12) // SM_CXICON/SM_CYICON：常规窗口图标尺寸。
	if small {
		widthMetric, heightMetric = 49, 50 // SM_CXSMICON/SM_CYSMICON：托盘和小窗口图标尺寸。
	}
	width, _, _ := user32.NewProc("GetSystemMetrics").Call(widthMetric)
	height, _, _ := user32.NewProc("GetSystemMetrics").Call(heightMetric)
	// 资源组 ID 1 由桌面入口的 .syso 提供；不使用 LR_SHARED，避免不同尺寸命中同一缓存句柄。
	icon, _, err := user32.NewProc("LoadImageW").Call(instance, 1, 1, width, height, 0)
	if icon == 0 {
		return 0, fmt.Errorf("无法加载内嵌应用图标（%d×%d），请重新构建完整桌面入口：%w", width, height, err)
	}
	return icon, nil
}

// runTray 在锁定的系统线程中运行原生消息循环，关闭网页不关闭服务。
func runTray(ctx context.Context, c *controller) error {
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	icon, err := loadApplicationIcon(instance, false)
	if err != nil {
		return err
	}
	defer user32.NewProc("DestroyIcon").Call(icon)
	smallIcon, err := loadApplicationIcon(instance, true)
	if err != nil {
		return err
	}
	defer user32.NewProc("DestroyIcon").Call(smallIcon)
	class := windowClass{Instance: instance, Name: wide("QikuDesktopLauncher"), Icon: icon, SmallIcon: smallIcon, Procedure: syscall.NewCallback(windowProcedure)}
	class.Size = uint32(unsafe.Sizeof(class))
	registered, _, _ := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if registered == 0 {
		return errors.New("无法注册 Windows 托盘窗口")
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(class.Name)), instance)
	window, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class.Name)), uintptr(unsafe.Pointer(wide("栖库"))), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if window == 0 {
		return errors.New("无法创建 Windows 托盘窗口")
	}
	trayWindow, trayController = window, c
	defer func() {
		// 异常退出消息循环时也先销毁窗口，再注销窗口类并释放大小图标。
		if alive, _, _ := user32.NewProc("IsWindow").Call(window); alive != 0 {
			user32.NewProc("DestroyWindow").Call(window)
		}
		trayWindow = 0
		trayController = nil
	}()
	taskbar, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(wide("TaskbarCreated"))))
	taskbarCreated = uint32(taskbar)
	trayData = notifyIconData{Window: window, ID: 1, Flags: 1 | 2 | 4, Callback: 0x8001, Icon: smallIcon}
	trayData.Size = uint32(unsafe.Sizeof(trayData))
	cfg, _ := c.snapshot()
	tip := "栖库 · 原生媒体模式"
	if cfg.Mode == "docker" {
		tip = "栖库 · Docker 完整模式"
	}
	copy(trayData.Tip[:], windows.StringToUTF16(tip))
	if err := addTrayIcon(); err != nil {
		user32.NewProc("DestroyWindow").Call(window)
		return err
	}
	defer shell32.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&trayData)))
	go func() { <-ctx.Done(); _, _, _ = postMessage.Call(window, 0x0010, 0, 0) }()
	var message nativeMessage
	for {
		result, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return errors.New("Windows 托盘消息循环失败")
		}
		if result == 0 {
			return nil
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&message)))
	}
}

// windowProcedure 仅将固定托盘事件路由到本机控制器。
func windowProcedure(window uintptr, message uint32, wParam uintptr, lParam uintptr) uintptr {
	if message == taskbarCreated && taskbarCreated != 0 {
		_ = addTrayIcon()
		return 0
	}
	switch message {
	case 0x8001:
		if lParam == 0x0203 {
			if trayController != nil {
				go trayController.openPage()
			}
			return 0
		}
		if lParam == 0x0205 || lParam == 0x007B {
			showTrayMenu(window)
			return 0
		}
	case 0x0010:
		user32.NewProc("DestroyWindow").Call(window)
		return 0
	case 0x0002:
		user32.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	result, _, _ := user32.NewProc("DefWindowProcW").Call(window, uintptr(message), wParam, lParam)
	return result
}

// appendMenu 添加原生菜单条目，文字仅来自固定命令或本机配置快照。
func appendMenu(menu uintptr, flags uintptr, id uintptr, label string) {
	user32.NewProc("AppendMenuW").Call(menu, flags, id, uintptr(unsafe.Pointer(wide(label))))
}

// showTrayMenu 显示当前模式与真实状态，并提供目录、日志、重启和退出入口。
func showTrayMenu(window uintptr) {
	c := trayController
	if c == nil {
		return
	}
	cfg, reply := c.snapshot()
	menu, _, _ := user32.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer user32.NewProc("DestroyMenu").Call(menu)
	mode := "原生媒体"
	if cfg.Mode == "docker" {
		mode = "Docker 完整模式"
	}
	states := map[string]string{"starting": "正在启动", "ready": "已就绪", "stopping": "正在停止", "stopped": "已停止", "failed": "启动失败"}
	appendMenu(menu, 2, 0, "栖库 · "+mode+" · "+states[reply.State])
	appendMenu(menu, 0, 1, "打开媒体库")
	appendMenu(menu, 0x800, 0, "")
	appendMenu(menu, 0, 2, "添加授权媒体目录…")
	rootsMenu, _, _ := user32.NewProc("CreatePopupMenu").Call()
	for index, root := range cfg.MediaRoots {
		appendMenu(rootsMenu, 0, uintptr(200+index), "移除授权："+root)
	}
	appendMenu(menu, 0x10, rootsMenu, "管理授权目录")
	appendMenu(menu, 0, 3, "打开本模式数据位置")
	appendMenu(menu, 0, 4, "打开日志目录")
	if cfg.Mode == "native" {
		appendMenu(menu, 0, 5, "选择独立数据目录…")
		appendMenu(menu, 0x800, 0, "")
		collectionStates := map[string]string{"disabled": "已停用", "external": "外接服务", "starting": "正在核验", "ready": "已就绪", "unavailable": "依赖未就绪", "failed": "不可用"}
		appendMenu(menu, 2, 0, "漫画组件 · "+collectionStates[reply.CollectionState])
		appendMenu(menu, 0, 10, "启用本机漫画下载")
		appendMenu(menu, 0, 11, "停用本机漫画下载")
		appendMenu(menu, 0, 12, "重新检测漫画组件")
	}
	appendMenu(menu, 0, 6, "重启当前模式")
	if cfg.Mode == "docker" {
		appendMenu(menu, 0, 7, "停止本实例全部容器（保留数据）")
	}
	appendMenu(menu, 0, 8, "启动另一种运行模式…")
	appendMenu(menu, 0x800, 0, "")
	exit := "退出栖库"
	if cfg.Mode == "docker" {
		exit = "退出启动器（容器继续运行）"
	}
	appendMenu(menu, 0, 9, exit)
	var point struct {
		X int32
		Y int32
	}
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
	user32.NewProc("SetForegroundWindow").Call(window)
	command, _, _ := user32.NewProc("TrackPopupMenu").Call(menu, 0x100|0x2, uintptr(point.X), uintptr(point.Y), 0, window, 0)
	postMessage.Call(window, 0, 0, 0)
	dispatchTrayCommand(window, c, command)
}

// dispatchTrayCommand 统一执行原生菜单选择，退出动作与本机 IPC 共用启动器取消链。
func dispatchTrayCommand(window uintptr, c *controller, command uintptr) {
	cfg, _ := c.snapshot()
	switch {
	case command == 1:
		go c.openPage()
	case command == 2:
		root, err := chooseFolder(window, "添加授权媒体目录")
		if err != nil {
			showMessage("无法授权目录", err.Error(), true)
		} else if root != "" {
			c.updateRoots(append(cfg.MediaRoots, root))
		}
	case command >= 200 && command < uintptr(200+len(cfg.MediaRoots)):
		index := int(command) - 200
		if confirmMessage(window, "移除目录授权", "停止授权后已有索引会保留，所选目录不再可读取。原始文件不会删除。\n\n"+cfg.MediaRoots[index]) {
			c.updateRoots(append(cfg.MediaRoots[:index], cfg.MediaRoots[index+1:]...))
		}
	case command == 3:
		if cfg.Mode == "docker" {
			showMessage("Docker 数据位置", "本模式数据库、下载内容和引擎配置位于 Docker Desktop 的 Linux 命名卷，不是普通 Windows 下载文件夹。\n安装前缀："+dockerPrefix(cfg)+"\n请使用网页备份与文档中的停机卷备份流程。", false)
		} else if err := shellOpen(cfg.DataDir); err != nil {
			showMessage("无法打开数据目录", err.Error(), true)
		}
	case command == 4:
		if err := shellOpen(filepath.Join(c.profile, "logs")); err != nil {
			showMessage("无法打开日志目录", err.Error(), true)
		}
	case command == 5:
		if !confirmMessage(window, "选择独立数据目录", "选择其他目录会打开该目录自己的媒体库或创建新库，不会迁移当前账号、收藏或进度。请先从网页备份现有数据，再停止其他使用该目录的实例。") {
			return
		}
		path, err := chooseFolder(window, "选择本地独立数据目录")
		if err == nil && path != "" {
			cfg.DataDir = path
			cfg, err = validateConfig(cfg)
			if err == nil {
				err = validateLocalDataDir(path)
			}
			if err == nil {
				c.request(desktopCommand{Kind: "config", Config: &cfg})
			}
		}
		if err != nil {
			showMessage("数据目录不可用", err.Error(), true)
		}
	case command == 6:
		c.request(desktopCommand{Kind: "restart"})
	case command == 7:
		if confirmMessage(window, "停止完整模式", "将先核验并停止本实例托管引擎，再停止 app/helper。数据卷和下载文件会保留；其他 Docker 项目不会被操作。") {
			c.request(desktopCommand{Kind: "docker-stop"})
		}
	case command == 8:
		startOtherMode(window, cfg.Mode)
	case command == 9:
		c.cancel()
	case command == 10:
		c.changeCollectionMode("managed")
	case command == 11:
		c.changeCollectionMode("disabled")
	case command == 12:
		c.request(desktopCommand{Kind: "restart"})
	}
}

// startOtherMode 显式启动独立配置模式，不静默复用 SQLite 或下载卷。
func startOtherMode(owner uintptr, current string) {
	mode := "docker"
	name := "Docker 完整模式"
	if current == "docker" {
		mode = "native"
		name = "原生媒体模式"
	}
	if !confirmMessage(owner, "启动"+name, "另一种模式使用独立配置、账号库和数据位置，不会自动迁移现有媒体库。Docker 完整模式需要已安装并运行的 Docker Desktop Linux 后端及固定发布镜像清单。是否继续？") {
		return
	}
	path, err := os.Executable()
	if err == nil {
		command := exec.Command(path, "--mode", mode)
		hideCommand(command)
		err = command.Start()
		if err == nil {
			err = command.Process.Release()
		}
	}
	if err != nil {
		showMessage("无法启动另一模式", fmt.Sprintf("启动%s失败，请检查程序包权限。", name), true)
	}
}
