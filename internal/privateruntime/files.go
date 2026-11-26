// Package privateruntime 提供私有插件共用的路径、文件完整性和隔离校验，不执行运行时。
package privateruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// reservedDevice 拒绝Windows设备名和上标端口别名。
var reservedDevice = regexp.MustCompile(`^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|CLOCK\$|COM[1-9¹²³]|LPT[1-9¹²³])$`)

// VerifyFile 分块校验文件及本机架构，并固定隔离路径内容。
func VerifyFile(path string, name string, expectedDigest string, size int64) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return errors.New("私有运行时文件缺失、类型或长度不匹配")
	}
	file, err := os.Open(path)
	if err != nil {
		return errors.New("私有运行时文件无法读取")
	}
	defer file.Close()
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(file, size+1))
	if err != nil || count != size || hex.EncodeToString(hash.Sum(nil)) != expectedDigest {
		return errors.New("私有运行时文件摘要不匹配")
	}
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".exe" || extension == ".dll" || extension == ".pyd" {
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return errors.New("私有运行时本机文件无法检查")
		}
		header := make([]byte, 64)
		if _, err = io.ReadFull(file, header); err != nil || string(header[:2]) != "MZ" {
			return errors.New("私有运行时本机文件不是PE格式")
		}
		offset := int64(header[60]) | int64(header[61])<<8 | int64(header[62])<<16 | int64(header[63])<<24
		if offset < 64 || offset > 16<<20 {
			return errors.New("私有运行时PE头部偏移无效")
		}
		if _, err = file.Seek(offset, io.SeekStart); err != nil {
			return errors.New("私有运行时PE头部无法读取")
		}
		signature := make([]byte, 6)
		if _, err = io.ReadFull(file, signature); err != nil || string(signature) != "PE\x00\x00\x64\x86" {
			return errors.New("私有运行时本机文件必须为x64架构")
		}
	}
	if strings.HasSuffix(name, "._pth") {
		abi := strings.TrimSuffix(filepath.Base(path), "._pth")
		expected := abi + ".zip\n.\nLib/site-packages\n"
		data, readErr := ReadOrdinary(path, 1024)
		if readErr != nil || string(data) != expected {
			return errors.New("私有运行时隔离搜索路径不符合固定契约")
		}
	}
	if strings.HasSuffix(strings.ToLower(name), ".pth") || strings.HasSuffix(strings.ToLower(name), ".pyc") {
		return errors.New("私有运行时不接受额外site配置或生成缓存")
	}
	return nil
}

// ReadOrdinary 有界读取普通文件并发现读取期间的增长。
func ReadOrdinary(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("文件类型或长度无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("文件读取超过预算")
	}
	return data, nil
}

// SafeRelative 拒绝跨平台路径穿越、设备名、尾点空格和控制字符。
func SafeRelative(path string) bool {
	if path == "" || strings.ContainsAny(path, "\\:<>\"|?*") {
		return false
	}
	for _, r := range path {
		if r < 32 || r == 127 {
			return false
		}
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		stem := strings.ToUpper(strings.TrimRight(strings.SplitN(part, ".", 2)[0], " ."))
		if reservedDevice.MatchString(stem) {
			return false
		}
	}
	return true
}

// SamePath 在Windows按大小写不敏感方式比较已规范化路径。
func SamePath(a string, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// Contains 检查一个源wheel是否属于编译期哈希白名单。
func Contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// RejectAliases 检查完整祖先链，防止目录交接或本机重解析点旁加载。
func RejectAliases(path string) error {
	for {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || IsReparsePoint(info) {
			return errors.New("文件或父目录包含链接、重解析点或不可读路径")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

// UniqueValue 递归检查有界JSON的每一层对象键是否唯一。
func UniqueValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("JSON嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, tokenErr := decoder.Token()
			key, isString := keyToken.(string)
			if tokenErr != nil || !isString || seen[key] {
				return errors.New("JSON对象字段重复或无效")
			}
			seen[key] = true
			if err := UniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := UniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("无效JSON结构")
	}
	_, err = decoder.Token()
	return err
}
