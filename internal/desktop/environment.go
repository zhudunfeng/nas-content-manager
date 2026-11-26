package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// desktopEnvironment 保存启动时合并的只读配置；进程变量存在即屏蔽文件中的同名项。
type desktopEnvironment struct {
	values       map[string]string // 按 Windows 不区分大小写语义归一后的配置快照。
	dotenvLoaded bool              // 是否成功读取程序同级 .env；文件缺失不是错误。
	dotenvCount  int               // 文件中不同 NAS 配置键数量；日志不记录具体值。
}

// executableDirectory 核实真实可执行文件目录，不依赖工作目录或 profile。
func executableDirectory() (string, error) {
	name, err := os.Executable()
	if err != nil || strings.TrimSpace(name) == "" {
		return "", errors.New("无法定位栖库.exe所在目录")
	}
	name, err = filepath.Abs(name)
	if err != nil {
		return "", errors.New("无法规范化栖库.exe路径")
	}
	name, err = filepath.EvalSymlinks(name)
	if err != nil {
		return "", errors.New("无法核实栖库.exe所在目录")
	}
	return filepath.Dir(name), nil
}

// loadDesktopEnvironment 仅为 Windows 桌面加载程序同级文件，其他入口不改变配置来源。
func loadDesktopEnvironment() (desktopEnvironment, error) {
	entries := os.Environ()
	if runtime.GOOS != "windows" {
		return mergeDesktopEnvironment(entries, nil), nil
	}
	directory, err := executableDirectory()
	if err != nil {
		return desktopEnvironment{}, err
	}
	return readDesktopEnvironment(directory, entries)
}

// readDesktopEnvironment 有界读取普通 UTF-8 文件，任何诊断都不回显配置值。
func readDesktopEnvironment(directory string, entries []string) (desktopEnvironment, error) {
	name := filepath.Join(directory, ".env")
	info, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return mergeDesktopEnvironment(entries, nil), nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return desktopEnvironment{}, errors.New("程序同级 .env 不可读取或不是普通文件，请检查文件权限和类型")
	}
	if info.Size() > 64<<10 {
		return desktopEnvironment{}, errors.New("程序同级 .env 超过 64 KiB，请精简配置")
	}
	file, err := os.Open(name)
	if err != nil {
		return desktopEnvironment{}, errors.New("程序同级 .env 不可读取，请检查文件权限")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) > 64<<10 {
		return desktopEnvironment{}, errors.New("程序同级 .env 读取失败或超过 64 KiB")
	}
	values, err := parseDesktopDotenv(data)
	if err != nil {
		return desktopEnvironment{}, err
	}
	environment := mergeDesktopEnvironment(entries, values)
	environment.dotenvLoaded = true
	environment.dotenvCount = len(values)
	return environment, nil
}

// mergeDesktopEnvironment 按存在性覆盖文件值，显式空值也不回退到 .env。
func mergeDesktopEnvironment(entries []string, dotenv map[string]string) desktopEnvironment {
	values := make(map[string]string, len(entries)+len(dotenv))
	for key, value := range dotenv {
		values[strings.ToUpper(key)] = value
	}
	for _, entry := range entries {
		key, value, present := strings.Cut(entry, "=")
		if present && key != "" {
			values[strings.ToUpper(key)] = value
		}
	}
	return desktopEnvironment{values: values}
}

// lookup 返回快照中的值及存在性；未提供快照的内部兼容调用仍使用进程环境。
func (environment desktopEnvironment) lookup(key string) (string, bool) {
	if environment.values == nil {
		return os.LookupEnv(key)
	}
	value, present := environment.values[strings.ToUpper(key)]
	return value, present
}

// get 为现有配置校验器提供字符串查询，空值保留各参数原有默认语义。
func (environment desktopEnvironment) get(key string) string {
	value, _ := environment.lookup(key)
	return value
}

// parseDesktopDotenv 解析不执行展开的单行配置子集，并只接受 NAS_ 配置键。
func parseDesktopDotenv(data []byte) (map[string]string, error) {
	if len(data) > 64<<10 {
		return nil, errors.New("程序同级 .env 超过 64 KiB，请精简配置")
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, errors.New("程序同级 .env 必须是有效的 UTF-8 文本，不能包含空字符")
	}
	values := make(map[string]string)
	for index, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
			line = strings.TrimSpace(line[len("export"):])
		}
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || !dotenvIdentifier(key) {
			return nil, fmt.Errorf("程序同级 .env 第 %d 行格式无效，请使用 KEY=value", index+1)
		}
		value, err := dotenvValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("程序同级 .env 第 %d 行引号或单行值格式无效", index+1)
		}
		key = strings.ToUpper(key)
		if strings.HasPrefix(key, "NAS_") {
			// 不展开变量、转义或命令；重复配置以文件中最后一项为准。
			values[key] = value
		}
	}
	return values, nil
}

// dotenvIdentifier 仅接受 ASCII 环境变量标识符，避免空格和分隔符注入。
func dotenvIdentifier(key string) bool {
	if key == "" {
		return false
	}
	for index := 0; index < len(key); index++ {
		c := key[index]
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(index > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// dotenvValue 保留 Windows 路径反斜杠和引号内原文，拒绝跨行或未闭合引号。
func dotenvValue(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n") {
		return "", errors.New("值必须在同一行")
	}
	if value == "" {
		return "", nil
	}
	if value[0] == '\'' || value[0] == '"' {
		end := strings.IndexByte(value[1:], value[0])
		if end < 0 {
			return "", errors.New("引号未闭合")
		}
		end++
		tail := strings.TrimSpace(value[end+1:])
		if tail != "" && !strings.HasPrefix(tail, "#") {
			return "", errors.New("引号后存在非注释文本")
		}
		return value[1:end], nil
	}
	for index := 1; index < len(value); index++ {
		if value[index] == '#' && (value[index-1] == ' ' || value[index-1] == '\t') {
			return strings.TrimSpace(value[:index]), nil
		}
	}
	if strings.HasPrefix(value, "#") {
		return "", nil
	}
	return value, nil
}
