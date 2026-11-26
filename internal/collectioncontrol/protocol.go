// Package collectioncontrol 提供仅经父子继承匿名管道使用的托管执行端控制协议。
package collectioncontrol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// Version 是父子进程必须严格匹配的私有控制协议版本。
const Version = 1

// MaxMessageBytes 限制单条控制消息，避免私有管道收到无界输入。
const MaxMessageBytes = 16 << 10

// Message 仅传输固定生命周期动作，不承载命令、令牌或下载参数。
type Message struct {
	Version    int    `json:"version"`            // 固定控制协议版本。
	Action     string `json:"action"`             // 固定 start、ready 或 stop 动作。
	Nonce      string `json:"nonce"`              // 父进程创建的一次性关联值，不记日志。
	InstanceID string `json:"instanceId"`         // 父桌面实例的稳定身份，不是执行端持久身份。
	Address    string `json:"address,omitempty"`  // ready 专用的实际回环 HTTP 地址。
	ReadRoot   string `json:"readRoot,omitempty"` // ready 专用的执行端规范绝对数据根。
}

// Decoder 在有界缓冲中读取严格单行 JSON，每个父子方向各持有一个实例。
type Decoder struct {
	reader *bufio.Reader // 专属输入流，不能与日志或业务响应共用。
}

// NewDecoder 建立固定大小控制缓冲，不预读其他通道。
func NewDecoder(reader io.Reader) *Decoder {
	return &Decoder{reader: bufio.NewReaderSize(reader, MaxMessageBytes)}
}

// Read 拒绝超限、缺换行、重复字段、未知字段及非规范消息。
func (d *Decoder) Read() (Message, error) {
	var message Message
	line, err := d.reader.ReadSlice('\n')
	if err != nil {
		if errors.Is(err, io.EOF) && len(line) == 0 {
			return message, io.EOF
		}
		return message, errors.New("托管控制消息不完整或超过大小限制")
	}
	if len(line) > MaxMessageBytes {
		return message, errors.New("托管控制消息超过大小限制")
	}
	// 先逐字段解码，encoding/json 的默认覆盖行为不能识别重复字段。
	decoder := json.NewDecoder(bytes.NewReader(line))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return message, errors.New("托管控制消息必须是JSON对象")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		key, keyErr := decoder.Token()
		name, ok := key.(string)
		if keyErr != nil || !ok || fields[name] != nil {
			return message, errors.New("托管控制消息包含重复或无效字段")
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return message, errors.New("托管控制消息字段无效")
		}
		fields[name] = value
	}
	if _, err = decoder.Token(); err != nil {
		return message, errors.New("托管控制消息无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return message, errors.New("托管控制消息包含尾随内容")
	}
	strict := json.NewDecoder(bytes.NewReader(line))
	strict.DisallowUnknownFields()
	if strict.Decode(&message) != nil || message.Validate() != nil {
		return Message{}, errors.New("托管控制消息版本、动作或关联字段无效")
	}
	required := []string{"version", "action", "nonce", "instanceId"}
	if message.Action == "ready" {
		required = append(required, "address", "readRoot")
	}
	if len(fields) != len(required) {
		return Message{}, errors.New("托管控制消息字段数量无效")
	}
	for _, name := range required {
		if fields[name] == nil {
			return Message{}, errors.New("托管控制消息缺少必需字段")
		}
	}
	return message, nil
}

// Validate 检查固定动作和最小关联约束，不接收任意网络端点。
func (m Message) Validate() error {
	if m.Version != Version || !safeAssociation(m.Nonce, 32, 128) || !safeAssociation(m.InstanceID, 16, 80) {
		return errors.New("托管控制关联无效")
	}
	switch m.Action {
	case "start", "stop":
		if m.Address != "" || m.ReadRoot != "" {
			return errors.New("托管控制动作包含额外字段")
		}
	case "ready":
		address, err := url.Parse(m.Address)
		if err != nil || address.Scheme != "http" || address.User != nil || address.Path != "" || address.ForceQuery || address.RawQuery != "" || address.Fragment != "" || address.Opaque != "" {
			return errors.New("托管执行端就绪地址无效")
		}
		host, port, err := net.SplitHostPort(address.Host)
		number, portErr := strconv.Atoi(port)
		if err != nil || portErr != nil || host != "127.0.0.1" || number < 1 || number > 65535 || strconv.Itoa(number) != port || m.Address != "http://"+net.JoinHostPort(host, port) {
			return errors.New("托管执行端必须报告实际回环端口")
		}
		if !filepath.IsAbs(m.ReadRoot) || filepath.Clean(m.ReadRoot) != m.ReadRoot || strings.ContainsAny(m.ReadRoot, "\x00\r\n") {
			return errors.New("托管执行端数据根无效")
		}
	default:
		return errors.New("托管控制动作不受支持")
	}
	return nil
}

// Write 一次输出已验证的有界消息，调用方独占该方向的管道。
func Write(writer io.Writer, message Message) error {
	if err := message.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(message)
	if err != nil || len(data)+1 > MaxMessageBytes {
		return errors.New("托管控制消息无法编码")
	}
	data = append(data, '\n')
	n, err := writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

// Match 检查本次父子会话的动作和关联，拒绝旧进程的就绪或停止消息。
func Match(message Message, action string, nonce string, instanceID string) error {
	if message.Validate() != nil || message.Action != action || message.Nonce != nonce || message.InstanceID != instanceID {
		return errors.New("托管控制消息不属于本次启动")
	}
	return nil
}

// safeAssociation 仅接受不含路径、控制字符或空白的关联标识。
func safeAssociation(value string, minimum int, maximum int) bool {
	if len(value) < minimum || len(value) > maximum {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return false
		}
	}
	return true
}
