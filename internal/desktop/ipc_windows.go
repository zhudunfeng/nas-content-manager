//go:build windows

package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// instanceGuard 在整个关闭过程中保持命名互斥句柄，不依赖端口或 PID 文件。
type instanceGuard struct {
	handle windows.Handle /* 当前用户的命名互斥句柄。 */
}

// Close 在服务与 IPC 已停止后释放单实例身份。
func (g *instanceGuard) Close() error { return windows.CloseHandle(g.handle) }

// instanceName 按当前用户和规范化配置根生成稳定且不可注入的对象名字。
func instanceName(profile string) (string, error) {
	sid, err := currentUserSID()
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(profile)
	if err != nil {
		return "", err
	}
	// 同一路径不同大小写和已存在的 junction 必须共用身份。
	absolute = canonicalConfigPath(absolute)
	digest := sha256.Sum256([]byte(sid + "\x00" + strings.ToLower(filepath.Clean(absolute))))
	return "Qiku-" + hex.EncodeToString(digest[:20]), nil
}

// acquireInstance 获取限当前用户 ACL 的本机会话互斥对象。
func acquireInstance(profile string) (*instanceGuard, bool, error) {
	name, err := instanceName(profile)
	if err != nil {
		return nil, false, err
	}
	sd, err := privateSecurity(false)
	if err != nil {
		return nil, false, err
	}
	wide, _ := windows.UTF16PtrFromString(`Local\` + name)
	handle, err := windows.CreateMutex(securityAttributes(sd), false, wide)
	runtime.KeepAlive(sd)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return &instanceGuard{handle: handle}, false, nil
	}
	if err != nil {
		return nil, false, errors.New("无法创建当前用户的单实例保护")
	}
	return &instanceGuard{handle: handle}, true, nil
}

// pipePath 返回固定本机管道名，不接受来自网页的目标或命令。
func pipePath(profile string) (*uint16, error) {
	name, err := instanceName(profile)
	if err != nil {
		return nil, err
	}
	return windows.UTF16PtrFromString(`\\.\pipe\` + name)
}

// awaitIO 以可取消、有限等待完成重叠 IO，取消后回收操作再释放缓冲区。
func awaitIO(ctx context.Context, handle windows.Handle, overlap *windows.Overlapped, initial error, timeout time.Duration) (uint32, error) {
	if initial != nil && !errors.Is(initial, windows.ERROR_IO_PENDING) {
		return 0, initial
	}
	deadline := time.Now().Add(timeout)
	for {
		var count uint32
		err := windows.GetOverlappedResult(handle, overlap, &count, false)
		if err == nil {
			return count, nil
		}
		if !errors.Is(err, windows.ERROR_IO_INCOMPLETE) {
			return 0, err
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			_ = windows.CancelIoEx(handle, overlap)
			_ = windows.GetOverlappedResult(handle, overlap, &count, true)
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			return 0, errors.New("本机控制通道响应超时")
		}
		_, _ = windows.WaitForSingleObject(overlap.HEvent, 50)
	}
}

// pipeReadWrite 在管道上传送单个不超过四 KiB 的固定协议报文。
func pipeReadWrite(ctx context.Context, handle windows.Handle, data []byte, write bool) (int, error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(event)
	overlap := windows.Overlapped{HEvent: event}
	// 内核会在函数调用返回后继续持有地址，必须固定到完成或取消回收之后。
	var pinned runtime.Pinner
	pinned.Pin(&overlap)
	if len(data) > 0 {
		pinned.Pin(&data[0])
	}
	defer pinned.Unpin()
	if write {
		err = windows.WriteFile(handle, data, nil, &overlap)
	} else {
		err = windows.ReadFile(handle, data, nil, &overlap)
	}
	// 重叠 IO 即使立即完成，也以最终结果确认实际字节数，不能依赖初始 count。
	count, err := awaitIO(ctx, handle, &overlap, err, 2*time.Second)
	return int(count), err
}

// connectControlPipe 固定连接状态至重叠连接完成，避免栈迁移使内核写入失效地址。
func connectControlPipe(ctx context.Context, handle windows.Handle) error {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(event)
	overlap := windows.Overlapped{HEvent: event}
	var pinned runtime.Pinner
	pinned.Pin(&overlap)
	defer pinned.Unpin()
	err = windows.ConnectNamedPipe(handle, &overlap)
	if errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
		return nil
	}
	_, err = awaitIO(ctx, handle, &overlap, err, 24*time.Hour)
	return err
}

// serveControl 预先占有固定管道并拒绝远端与其他用户，仅开放三个固定动作。
func serveControl(ctx context.Context, profile string, handler func(string) controlReply, onQuit func()) (func(), error) {
	name, err := pipePath(profile)
	if err != nil {
		return nil, err
	}
	sd, err := privateSecurity(false)
	if err != nil {
		return nil, err
	}
	newPipe := func() (windows.Handle, error) {
		handle, createErr := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED|windows.FILE_FLAG_FIRST_PIPE_INSTANCE, windows.PIPE_TYPE_MESSAGE|windows.PIPE_READMODE_MESSAGE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS, 1, 4096, 4096, 2000, securityAttributes(sd))
		runtime.KeepAlive(sd)
		return handle, createErr
	}
	first, err := newPipe()
	if err != nil {
		return nil, errors.New("本机控制通道已被占用或权限无效，已拒绝启动")
	}
	serveCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		handle := first
		defer windows.CloseHandle(handle)
		for serveCtx.Err() == nil {
			connectErr := connectControlPipe(serveCtx, handle)
			if connectErr == nil {
				request := make([]byte, 257)
				count, readErr := pipeReadWrite(serveCtx, handle, request, false)
				if readErr == nil && count > 0 && count <= 256 {
					action := string(request[:count])
					if action == "open" || action == "status" || action == "quit" {
						data, _ := json.Marshal(handler(action))
						written, writeErr := pipeReadWrite(serveCtx, handle, data, true)
						if writeErr == nil && written == len(data) {
							confirmation := make([]byte, 3)
							count, confirmErr := pipeReadWrite(serveCtx, handle, confirmation, false)
							if confirmErr == nil && string(confirmation[:count]) == "ok" && action == "quit" {
								onQuit()
							}
						}
					}
				}
			}
			_ = windows.DisconnectNamedPipe(handle)
			if serveCtx.Err() != nil {
				return
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-done }) }, nil
}

// sendControl 只连接本机当前用户的固定通道，并有界等待首实例初始化完成。
func sendControl(profile string, action string, timeout time.Duration) (controlReply, error) {
	var reply controlReply
	if action != "open" && action != "status" && action != "quit" {
		return reply, errors.New("不支持的本机控制动作")
	}
	name, err := pipePath(profile)
	if err != nil {
		return reply, err
	}
	deadline := time.Now().Add(timeout)
	var handle windows.Handle
	for {
		handle, err = windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) && !errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return reply, errors.New("已有实例的本机控制通道不可访问")
		}
		if time.Now().After(deadline) {
			return reply, errors.New("未找到可响应的已有实例；请等待启动完成后重试")
		}
		time.Sleep(75 * time.Millisecond)
	}
	defer windows.CloseHandle(handle)
	mode := uint32(windows.PIPE_READMODE_MESSAGE)
	if err = windows.SetNamedPipeHandleState(handle, &mode, nil, nil); err != nil {
		return reply, errors.New("本机控制通道类型无效")
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	if _, err = pipeReadWrite(ctx, handle, []byte(action), true); err != nil {
		return reply, errors.New("无法发送本机控制请求")
	}
	data := make([]byte, 4097)
	count, err := pipeReadWrite(ctx, handle, data, false)
	if err != nil {
		return reply, fmt.Errorf("已有实例的控制响应读取失败：%w", err)
	}
	if count == 0 || count > 4096 {
		return reply, errors.New("已有实例返回的控制响应无效")
	}
	if decodeStrictJSON(data[:count], &reply) != nil {
		return reply, errors.New("已有实例返回的控制协议无效")
	}
	if _, err = pipeReadWrite(ctx, handle, []byte("ok"), true); err != nil {
		return reply, errors.New("无法确认本机控制请求")
	}
	return reply, nil
}
