package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/term"
)

// offlineTerminalInputBudget 限制每次交互的原始字节，低于官方行编辑器的静默截断上限。
const offlineTerminalInputBudget = 1024

// offlinePasswordResult 仅在进程内传递敏感读取结果，禁止序列化或输出。
type offlinePasswordResult struct {
	password []byte // 原值密码，由调用者在使用结束后尽力清零。
	err      error  // 固定安全错误或上下文取消。
}

// terminalPasswordIO 分离真实输入句柄和提示输出，保留跨两次读取的 CRLF 状态。
type terminalPasswordIO struct {
	ctx       context.Context // 维护进程取消边界。
	input     io.Reader       // 已由主协程进入 raw 模式的真实终端。
	output    io.Writer       // 仅用于固定提示和行编辑器控制输出。
	seen      int             // 本次行输入的有界原始字节计数。
	invalid   bool            // 非法字节粘性标记，不能被退格撤销。
	overflow  bool            // 原始预算超限后整次失败，不接受截断结果。
	pendingLF bool            // 前一次 CR 后只忽略紧邻的一个 LF。
	writeErr  error           // 提示失败后禁止继续读取和写库。
}

// Read 每次最多交付一个安全字节，非法或超限输入最终明确失败而非静默接受。
func (stream *terminalPasswordIO) Read(target []byte) (int, error) {
	if len(target) == 0 {
		return 0, nil
	}
	for {
		if err := stream.ctx.Err(); err != nil {
			return 0, err
		}
		if stream.writeErr != nil {
			return 0, errors.New("终端提示不可输出")
		}
		var source [1]byte
		count, err := stream.input.Read(source[:])
		if count == 0 {
			if err != nil {
				return 0, err
			}
			continue
		}
		value := source[0]
		if stream.pendingLF {
			stream.pendingLF = false
			if value == '\n' {
				continue
			}
		}
		if value == 3 || value == 4 {
			return 0, context.Canceled
		}
		if value == '\r' || value == '\n' {
			stream.pendingLF = value == '\r'
			target[0] = '\r'
			return 1, nil
		}
		if stream.seen < offlineTerminalInputBudget {
			stream.seen++
		} else {
			// 超限后继续消耗到行尾，不增长内存，也不能退格回合法输入。
			stream.overflow = true
			continue
		}
		if value == '\b' {
			value = 127
		} else if (value < 32 || value > 126) && value != 127 {
			// 不让库忽略控制字符或卡在坏 UTF-8 后，把改变的输入当成有效密码。
			stream.invalid = true
			value = '?'
		}
		target[0] = value
		return 1, nil
	}
}

// Write 记录行编辑器未必传播的提示错误，阻止提示失败后的账号写入。
func (stream *terminalPasswordIO) Write(data []byte) (int, error) {
	count, err := stream.output.Write(data)
	if err == nil && count != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		stream.writeErr = errors.New("终端提示不可输出")
	}
	return count, err
}

// readOfflinePassword 只允许真实终端双确认或显式重定向输入，绝不自动降级到有回显输入。
func readOfflinePassword(ctx context.Context, input *os.File, diagnostic io.Writer, stdinMode bool) ([]byte, error) {
	if input == nil {
		return nil, errors.New("密码输入不可用")
	}
	isTerminal := term.IsTerminal(int(input.Fd()))
	if stdinMode {
		if isTerminal {
			return nil, errors.New("password-stdin 必须使用重定向输入，不能在终端明文输入")
		}
		return readOfflinePasswordStdin(ctx, input)
	}
	if !isTerminal {
		return nil, errors.New("需要真实终端；管道输入必须显式使用 --password-stdin")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 同步完成模式切换后才允许输入协程运行；协程永不修改操作系统终端模式。
	state, err := term.MakeRaw(int(input.Fd()))
	if err != nil {
		return nil, errors.New("终端无回显模式不可启用")
	}
	return readOfflineTerminalPair(ctx, input, diagnostic, func() error {
		return term.Restore(int(input.Fd()), state)
	})
}

// readOfflineTerminalPair 统一恢复终端；取消后不等待阻塞读取，维护 main 完成清理后直接退出。
func readOfflineTerminalPair(ctx context.Context, input io.Reader, output io.Writer, restore func() error) (password []byte, resultErr error) {
	defer func() {
		if err := restore(); err != nil {
			clear(password)
			password = nil
			resultErr = errors.Join(resultErr, errors.New("终端模式恢复失败，未执行重置"))
		}
	}()
	result := make(chan offlinePasswordResult, 1)
	go func() {
		stream := &terminalPasswordIO{ctx: ctx, input: input, output: output}
		terminal := term.NewTerminal(stream, "")
		first, err := terminal.ReadPassword("新密码（输入不回显）：")
		if err == nil && (stream.invalid || stream.overflow || stream.writeErr != nil || !validNewPassword(first)) {
			err = errors.New("新密码输入无效或超过交互预算，请重新执行；" + newPasswordHint)
		}
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			result <- offlinePasswordResult{err: safeTerminalInputError(err)}
			return
		}
		stream.seen, stream.invalid, stream.overflow = 0, false, false
		confirmation, err := terminal.ReadPassword("再次输入新密码（输入不回显）：")
		if err == nil && (stream.invalid || stream.overflow || stream.writeErr != nil) {
			err = errors.New("确认输入无效或超过交互预算，请重新执行")
		}
		if err == nil && first != confirmation {
			err = errors.New("两次密码不一致，未执行重置")
		}
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			result <- offlinePasswordResult{err: safeTerminalInputError(err)}
			return
		}
		result <- offlinePasswordResult{password: []byte(first)}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case reply := <-result:
		return reply.password, reply.err
	}
}

// safeTerminalInputError 不回显终端或库原始错误，保留明确取消语义。
func safeTerminalInputError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
		return context.Canceled
	}
	return errors.New("密码输入失败、无效或确认不一致，未执行重置")
}

// readOfflinePasswordStdin 有界读完整单个输入，只去掉一个最终 LF 或 CRLF，不 trim 或截断。
func readOfflinePasswordStdin(ctx context.Context, input io.Reader) ([]byte, error) {
	result := make(chan offlinePasswordResult, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(input, 75))
		if err != nil {
			clear(data)
			result <- offlinePasswordResult{err: errors.New("密码标准输入读取失败")}
			return
		}
		if len(data) >= 75 {
			clear(data)
			result <- offlinePasswordResult{err: errors.New("密码标准输入超出单个密码上限")}
			return
		}
		if bytes.HasSuffix(data, []byte{'\n'}) {
			data = data[:len(data)-1]
			if bytes.HasSuffix(data, []byte{'\r'}) {
				data = data[:len(data)-1]
			}
		}
		if bytes.ContainsAny(data, "\r\n") {
			clear(data)
			result <- offlinePasswordResult{err: errors.New("标准输入必须是单个密码，不能包含多行")}
			return
		}
		result <- offlinePasswordResult{password: data}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case reply := <-result:
		return reply.password, reply.err
	}
}
