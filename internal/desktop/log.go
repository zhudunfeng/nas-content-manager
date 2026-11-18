package desktop

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// rotatingWriter 限制本机诊断日志大小，避免长期使用填满数据盘。
type rotatingWriter struct {
	mu   sync.Mutex // 写入与轮转互斥锁。
	path string     // 当前日志的固定绝对路径。
	file *os.File   // 当前打开的日志文件。
	size int64      // 当前文件累计字节数。
}

// newLogger 创建最多四份、每份约五 MiB 的中文结构化日志。
func newLogger(profile string, mirror io.Writer) (*slog.Logger, io.Closer, error) {
	dir := filepath.Join(profile, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	w := &rotatingWriter{path: filepath.Join(dir, "desktop.log")}
	if err := w.open(); err != nil {
		return nil, nil, err
	}
	var output io.Writer = w
	if mirror != nil {
		output = io.MultiWriter(w, mirror)
	}
	return slog.New(slog.NewJSONHandler(output, nil)), w, nil
}

// open 打开轮转后的当前文件，不截断原诊断记录。
func (w *rotatingWriter) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file, w.size = file, info.Size()
	return nil
}

// Write 串行写入并按固定名字轮转，不记录命令行秘密或引擎输出。
func (w *rotatingWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size+int64(len(data)) > 5<<20 {
		if err := w.file.Close(); err != nil {
			return 0, err
		}
		w.file = nil
		if err := os.Remove(w.path + ".3"); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		for i := 2; i >= 0; i-- {
			from := w.path
			if i > 0 {
				from = fmt.Sprintf("%s.%d", w.path, i)
			}
			if err := os.Rename(from, fmt.Sprintf("%s.%d", w.path, i+1)); err != nil && !os.IsNotExist(err) {
				return 0, err
			}
		}
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(data)
	w.size += int64(n)
	return n, err
}

// Close 在所有服务停止后释放日志句柄。
func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
