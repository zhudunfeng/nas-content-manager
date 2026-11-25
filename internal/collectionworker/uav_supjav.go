package collectionworker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// errUAVCandidate 只标记可换线路的网络、格式或内容错误，不包含本地存储失败。
var errUAVCandidate = errors.New("视频播放线路未通过校验")

// errUAVStorage 标记工具报告的本地写入错误，不能以换线路掩盖存储问题。
var errUAVStorage = errors.New("视频本地存储写入失败，已停止获取")

// errUAVProcess 标记自有工具未完整退出，不允许启动下一线路进程。
var errUAVProcess = errors.New("执行进程未能完整退出")

// supjavPNGSignature 只允许已知PNG包装，不把任意损坏当作可修复前缀。
var supjavPNGSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// validSupJavTSPacket 检查188字节TS包同步、传输错误标记及有效载荷控制字段。
func validSupJavTSPacket(packet []byte) bool {
	if len(packet) < 188 || packet[0] != 0x47 || packet[1]&0x80 != 0 || packet[3]&0x30 == 0 {
		return false
	}
	if packet[3]&0x20 == 0 {
		return true
	}
	// 只有适配字段时必须占满剩余183字节；同时声明载荷时至少保留一个载荷字节。
	if packet[3]&0x10 == 0 {
		return packet[4] == 183
	}
	return packet[4] <= 182
}

// normalizeSupJavTS 只在原生下载器解密之后原地移除已知PNG包装，任何截包或未知损坏均失败。
func normalizeSupJavTS(ctx context.Context, name string) (removed int64, resultErr error) {
	file, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return 0, errUAVStorage
	}
	// 关闭失败同样属于本地存储故障，不能在下一线路中掩盖。
	defer func() {
		if file.Close() != nil {
			resultErr = errUAVStorage
		}
	}()
	reader := bufio.NewReaderSize(file, 64<<10)
	output := make([]byte, 0, 64<<10)
	var written int64
	// 独立WriteAt不会改变读取偏移；输出位置始终落后于已读取数据，避免覆盖尚未消费的源字节。
	flush := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if len(output) > 0 {
			count, writeErr := file.WriteAt(output, written)
			if writeErr != nil || count != len(output) {
				return errUAVStorage
			}
			written += int64(count)
			output = output[:0]
		}
		return nil
	}
	for {
		if ctx.Err() != nil {
			return removed, ctx.Err()
		}
		packet, readErr := reader.Peek(188)
		if readErr != nil {
			if readErr != io.EOF {
				return removed, errUAVStorage
			}
			if len(packet) != 0 {
				return removed, fmt.Errorf("%w：SupJav媒体含不完整TS尾包", errUAVCandidate)
			}
			break
		}
		if validSupJavTSPacket(packet) {
			output = append(output, packet...)
			_, _ = reader.Discard(188)
			if len(output) >= 64<<10 {
				if err = flush(); err != nil {
					return removed, err
				}
			}
			continue
		}
		if !bytes.HasPrefix(packet, supjavPNGSignature) {
			return removed, fmt.Errorf("%w：SupJav媒体含未知包装或损坏TS包", errUAVCandidate)
		}
		prefix, prefixErr := reader.Peek(8000 + 5*188)
		if prefixErr != nil && prefixErr != io.EOF {
			return removed, errUAVStorage
		}
		start := -1
		for offset := len(supjavPNGSignature); offset <= 8000 && offset+5*188 <= len(prefix); offset++ {
			valid := true
			for index := 0; index < 5; index++ {
				if !validSupJavTSPacket(prefix[offset+index*188:]) {
					valid = false
					break
				}
			}
			if valid {
				start = offset
				break
			}
		}
		if start < 0 {
			return removed, fmt.Errorf("%w：SupJav已知包装超限或缺少连续TS包", errUAVCandidate)
		}
		_, _ = reader.Discard(start)
		removed += int64(start)
	}
	if err = flush(); err != nil {
		return removed, err
	}
	if written == 0 {
		return removed, fmt.Errorf("%w：SupJav媒体不含TS包", errUAVCandidate)
	}
	if err = file.Truncate(written); err != nil {
		return removed, errUAVStorage
	}
	if err = file.Sync(); err != nil {
		return removed, errUAVStorage
	}
	return removed, nil
}

// cleanUAVCandidate 仅清理固定下载器留下的本次普通文件，意外目录或链接必须保留并失败。
func cleanUAVCandidate(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return errUAVStorage
	}
	for _, entry := range entries {
		name := entry.Name()
		known := name == "media.download" || name == "media.download.part" || name == "media.download.ytdl" || name == "media.mp4"
		if !known && strings.HasPrefix(name, "media.download.part-Frag") {
			tail := strings.TrimPrefix(name, "media.download.part-Frag")
			tail = strings.TrimSuffix(tail, ".part")
			known = len(tail) > 0 && len(tail) <= 10
			for _, char := range tail {
				known = known && char >= '0' && char <= '9'
			}
		}
		info, infoErr := entry.Info()
		if !known || infoErr != nil || !info.Mode().IsRegular() {
			return errors.New("视频线路失败后暂存含非预期产物，已停止获取")
		}
	}
	for _, entry := range entries {
		if os.Remove(filepath.Join(directory, entry.Name())) != nil {
			return errUAVStorage
		}
	}
	return nil
}

// uavToolStorageFailure 只识别固定工具的本地I/O故障提示，不持久化或回显原始stderr。
func uavToolStorageFailure(data []byte) bool {
	text := strings.ToLower(string(data))
	for _, marker := range []string{"no space left on device", "disk full", "not enough space", "permission denied", "access is denied", "input/output error", "read-only file system", "winerror 112", "winerror 5", "errno 28", "errno 13", "error writing", "unable to write", "unable to open for writing"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
