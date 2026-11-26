// Package jmcomicruntime 校验随发行包交付的固定漫画私有运行时，不执行用户脚本。
package jmcomicruntime

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ManifestName 是解释器清单在程序根内的固定名称。
const ManifestName = "jmcomic-runtime.json"

// manifestLimit 限制最多一万份依赖文件清单的读取预算。
const manifestLimit = 4 << 20

// dependencyContract 保存与仓库哈希锁一致的编译期依赖白名单。
//
//go:embed contract.json
var dependencyContract []byte

// Runtime 是完成静态完整性校验后的私有解释器身份。
type Runtime struct {
	Python         string // 已验证的规范绝对Python解释器，不依赖当前目录或PATH。
	Root           string // 清单所在的规范程序根。
	PythonVersion  string // 精确CPython补丁版本。
	JMComicVersion string // 固定漫画依赖版本。
}

// manifest 定义禁止额外字段的完整运行时清单。
type manifest struct {
	SchemaVersion      int          `json:"schemaVersion"`      // 固定清单格式版本1。
	Target             string       `json:"target"`             // 当前只接受windows-amd64。
	PythonVersion      string       `json:"pythonVersion"`      // 精确CPython补丁版本。
	JMComicVersion     string       `json:"jmcomicVersion"`     // 与编译期契约一致的漫画版本。
	Python             string       `json:"python"`             // 包内固定解释器相对路径。
	RuntimeRoot        string       `json:"runtimeRoot"`        // 包内固定运行时根。
	RequirementsSHA256 string       `json:"requirementsSha256"` // 构建时依赖锁字节摘要。
	Sources            []source     `json:"sources"`            // 解释器与每个wheel的固定来源身份。
	Files              []fileRecord `json:"files"`              // 全部运行与许可文件的摘要及字节数。
}

// source 记录构建输入，不包含构建机绝对路径。
type source struct {
	Name    string `json:"name"`    // python或规范依赖名称。
	Version string `json:"version"` // 精确版本。
	URL     string `json:"url"`     // 官方HTTPS构建输入地址。
	SHA256  string `json:"sha256"`  // 已锁定源归档或wheel摘要。
}

// fileRecord 绑定一份包内普通文件及其所属组件。
type fileRecord struct {
	Path      string `json:"path"`      // 安全、大小写唯一的正斜杠相对路径。
	SHA256    string `json:"sha256"`    // 完整文件SHA256。
	Size      int64  `json:"size"`      // 实际字节数。
	Role      string `json:"role"`      // runtime运行材料或license许可材料。
	Component string `json:"component"` // 来源组件名。
}

// lockedDependency 定义一个已验收依赖的精确版本及允许wheel摘要。
type lockedDependency struct {
	Version string   `json:"version"` // 固定版本。
	Hashes  []string `json:"hashes"`  // 官方wheel摘要白名单。
}

// lockContract 是编译进二进制的依赖锁身份。
type lockContract struct {
	RequirementsSHA256 string                      `json:"requirementsSha256"` // 依赖锁统一LF换行后的摘要。
	Packages           map[string]lockedDependency `json:"packages"`           // 固定基础依赖闭包。
}

// validDigest 检查摘要采用唯一小写表示。
var validDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// validVersion 只接纳项目锁定wheel覆盖的CPython正式补丁系列。
var validVersion = regexp.MustCompile(`^3\.(11|12|13|14)\.[0-9]{1,3}$`)

// reservedDevice 覆盖Win32控制台设备及被视为端口号的上标数字别名。
var reservedDevice = regexp.MustCompile(`^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|CLOCK\$|COM[1-9¹²³]|LPT[1-9¹²³])$`)

// Load 校验清单及全部运行文件后返回可信绝对解释器；不执行或安装依赖。
func Load(manifestPath string) (Runtime, error) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return Runtime{}, errors.New("随包漫画运行时仅支持Windows x64，其他平台请显式配置受信解释器")
	}
	return load(manifestPath)
}

// load 共享静态校验流程，平台检查由公开入口负责。
func load(manifestPath string) (Runtime, error) {
	if !filepath.IsAbs(manifestPath) || filepath.Clean(manifestPath) != manifestPath || filepath.Base(manifestPath) != ManifestName {
		return Runtime{}, errors.New("漫画运行时清单必须为固定名称的规范绝对路径")
	}
	root := filepath.Dir(manifestPath)
	if err := rejectAliases(manifestPath); err != nil {
		return Runtime{}, errors.New("漫画运行时清单路径包含链接或重解析点")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || !samePath(root, canonical) {
		return Runtime{}, errors.New("漫画运行时目录不存在或包含链接别名")
	}
	data, err := readOrdinary(manifestPath, manifestLimit)
	if err != nil {
		return Runtime{}, errors.New("漫画运行时清单缺失、不是普通文件或超过预算")
	}
	var value manifest
	if err := strictDecode(data, &value); err != nil {
		return Runtime{}, errors.New("漫画运行时清单字段、重复键或JSON格式无效")
	}
	var contract lockContract
	if err := json.Unmarshal(dependencyContract, &contract); err != nil {
		return Runtime{}, errors.New("漫画运行时编译期契约无效")
	}
	if value.SchemaVersion != 1 || value.Target != "windows-amd64" || !validVersion.MatchString(value.PythonVersion) || value.JMComicVersion != contract.Packages["jmcomic"].Version || value.RequirementsSHA256 != contract.RequirementsSHA256 || value.RuntimeRoot != "runtimes/jmcomic" || value.Python != value.RuntimeRoot+"/python.exe" {
		return Runtime{}, errors.New("漫画运行时格式、平台、版本或依赖锁不匹配")
	}
	sources := make(map[string]bool)
	if len(value.Sources) != len(contract.Packages)+1 {
		return Runtime{}, errors.New("漫画运行时来源闭包不完整")
	}
	for _, item := range value.Sources {
		if sources[item.Name] || !validDigest.MatchString(item.SHA256) {
			return Runtime{}, errors.New("漫画运行时来源重复或摘要无效")
		}
		if item.Name == "python" {
			expected := "https://www.python.org/ftp/python/" + value.PythonVersion + "/python-" + value.PythonVersion + "-embed-amd64.zip"
			if item.Version != value.PythonVersion || item.URL != expected {
				return Runtime{}, errors.New("漫画私有Python来源或版本不匹配")
			}
		} else {
			locked, ok := contract.Packages[item.Name]
			parsed, parseErr := url.Parse(item.URL)
			if !ok || item.Version != locked.Version || parseErr != nil || parsed.Scheme != "https" || parsed.Host != "files.pythonhosted.org" || !strings.HasPrefix(parsed.EscapedPath(), "/packages/") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil || !contains(locked.Hashes, item.SHA256) {
				return Runtime{}, errors.New("漫画依赖来源、版本或wheel摘要不在白名单")
			}
		}
		sources[item.Name] = true
	}
	if !sources["python"] || len(value.Files) == 0 || len(value.Files) > 10000 {
		return Runtime{}, errors.New("漫画运行时文件数或来源无效")
	}
	files := make(map[string]fileRecord, len(value.Files))
	folded := make(map[string]bool, len(value.Files))
	licenses := make(map[string]bool)
	var total int64
	for _, item := range value.Files {
		if !safeRelative(item.Path) || !validDigest.MatchString(item.SHA256) || item.Size < 0 || item.Size > 512<<20 || !sources[item.Component] || folded[strings.ToLower(item.Path)] {
			return Runtime{}, errors.New("漫画运行时文件路径、组件、摘要或大小无效")
		}
		if item.Role == "runtime" {
			if !strings.HasPrefix(item.Path, value.RuntimeRoot+"/") {
				return Runtime{}, errors.New("漫画运行文件越出私有解释器目录")
			}
		} else if item.Role == "license" {
			if !strings.HasPrefix(item.Path, "licenses/jmcomic/"+item.Component+"/") || item.Size == 0 {
				return Runtime{}, errors.New("漫画运行时许可位置或内容无效")
			}
			licenses[item.Component] = true
		} else {
			return Runtime{}, errors.New("漫画运行时文件角色无效")
		}
		total += item.Size
		if total > 1<<30 {
			return Runtime{}, errors.New("漫画运行时整体超过1GiB预算")
		}
		files[item.Path] = item
		folded[strings.ToLower(item.Path)] = true
	}
	if len(licenses) != len(sources) {
		return Runtime{}, errors.New("漫画运行时缺少组件许可材料")
	}
	if err := verifyRequired(value, files, contract); err != nil {
		return Runtime{}, err
	}
	// 逐个枚举受控目录，对未声明文件和目录链接也拒绝，防止旁加载污染。
	for _, directory := range []string{value.RuntimeRoot, "licenses/jmcomic"} {
		err = filepath.WalkDir(filepath.Join(root, filepath.FromSlash(directory)), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.Type()&os.ModeSymlink != 0 || rejectAliases(path) != nil {
				return errors.New("漫画运行时包含不可读路径或链接")
			}
			canonical, resolveErr := filepath.EvalSymlinks(path)
			if resolveErr != nil || !samePath(path, canonical) {
				return errors.New("漫画运行时包含重解析或链接别名")
			}
			if entry.IsDir() {
				return nil
			}
			relative, relativeErr := filepath.Rel(root, path)
			if relativeErr != nil {
				return errors.New("漫画运行时文件位置无效")
			}
			name := filepath.ToSlash(relative)
			record, ok := files[name]
			if !ok {
				return errors.New("漫画运行时存在未声明或大小写不符的文件")
			}
			if verifyErr := verifyFile(path, record); verifyErr != nil {
				return verifyErr
			}
			delete(files, name)
			return nil
		})
		if err != nil {
			return Runtime{}, err
		}
	}
	if len(files) != 0 {
		return Runtime{}, errors.New("漫画运行时缺少清单声明的文件")
	}
	return Runtime{Python: filepath.Join(root, filepath.FromSlash(value.Python)), Root: root, PythonVersion: value.PythonVersion, JMComicVersion: value.JMComicVersion}, nil
}

// verifyRequired 核对标准库、隔离路径、元数据、证书和本机扩展的必要存在。
func verifyRequired(value manifest, files map[string]fileRecord, contract lockContract) error {
	parts := strings.Split(value.PythonVersion, ".")
	abi := parts[0] + parts[1]
	prefix := value.RuntimeRoot + "/"
	for _, name := range []string{"python.exe", "python" + abi + ".dll", "python" + abi + ".zip", "python" + abi + "._pth", "Lib/site-packages/certifi/cacert.pem"} {
		if _, ok := files[prefix+name]; !ok {
			return errors.New("漫画运行时缺少解释器、标准库、隔离路径或证书")
		}
	}
	for name, locked := range contract.Packages {
		metadata := prefix + "Lib/site-packages/" + strings.ReplaceAll(name, "-", "_") + "-" + locked.Version + ".dist-info/METADATA"
		if _, ok := files[metadata]; !ok {
			return errors.New("漫画运行时缺少固定依赖的版本元数据")
		}
	}
	for _, required := range []string{"Lib/site-packages/curl_cffi/_wrapper.pyd", "Lib/site-packages/PIL/_imaging", "Lib/site-packages/_cffi_backend", "Lib/site-packages/Crypto/"} {
		found := false
		for name := range files {
			if strings.HasPrefix(name, prefix+required) && strings.HasSuffix(name, ".pyd") {
				found = true
				break
			}
		}
		if !found {
			return errors.New("漫画运行时缺少必要本机扩展")
		}
	}
	return nil
}

// verifyFile 分块校验文件及本机架构，并固定隔离路径内容。
func verifyFile(path string, record fileRecord) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != record.Size {
		return errors.New("漫画运行时文件缺失、类型或长度不匹配")
	}
	file, err := os.Open(path)
	if err != nil {
		return errors.New("漫画运行时文件无法读取")
	}
	defer file.Close()
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(file, record.Size+1))
	if err != nil || count != record.Size || hex.EncodeToString(hash.Sum(nil)) != record.SHA256 {
		return errors.New("漫画运行时文件摘要不匹配")
	}
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".exe" || extension == ".dll" || extension == ".pyd" {
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return errors.New("漫画运行时本机文件无法检查")
		}
		header := make([]byte, 64)
		if _, err = io.ReadFull(file, header); err != nil || string(header[:2]) != "MZ" {
			return errors.New("漫画运行时本机文件不是PE格式")
		}
		offset := int64(header[60]) | int64(header[61])<<8 | int64(header[62])<<16 | int64(header[63])<<24
		if offset < 64 || offset > 16<<20 {
			return errors.New("漫画运行时PE头部偏移无效")
		}
		if _, err = file.Seek(offset, io.SeekStart); err != nil {
			return errors.New("漫画运行时PE头部无法读取")
		}
		signature := make([]byte, 6)
		if _, err = io.ReadFull(file, signature); err != nil || string(signature) != "PE\x00\x00\x64\x86" {
			return errors.New("漫画运行时本机文件必须为x64架构")
		}
	}
	if strings.HasSuffix(record.Path, "._pth") {
		abi := strings.TrimSuffix(filepath.Base(path), "._pth")
		expected := abi + ".zip\n.\nLib/site-packages\n"
		data, readErr := readOrdinary(path, 1024)
		if readErr != nil || string(data) != expected {
			return errors.New("漫画运行时隔离搜索路径不符合固定契约")
		}
	}
	if strings.HasSuffix(strings.ToLower(record.Path), ".pth") || strings.HasSuffix(strings.ToLower(record.Path), ".pyc") {
		return errors.New("漫画运行时不接受额外site配置或生成缓存")
	}
	return nil
}

// readOrdinary 有界读取普通文件并发现读取期间的增长。
func readOrdinary(path string, limit int64) ([]byte, error) {
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

// safeRelative 拒绝跨平台路径穿越、设备名、尾点空格和控制字符。
func safeRelative(path string) bool {
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

// samePath 在Windows按大小写不敏感方式比较已规范化路径。
func samePath(a string, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// contains 检查一个源wheel是否属于编译期哈希白名单。
func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// strictDecode 拒绝重复字段、未知字段和多个JSON值。
func strictDecode(data []byte, target any) error {
	check := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueValue(check, 0); err != nil {
		return err
	}
	if _, err := check.Token(); err != io.EOF {
		return errors.New("多余JSON内容")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || !exactKeys(object, "schemaVersion", "target", "pythonVersion", "jmcomicVersion", "python", "runtimeRoot", "requirementsSha256", "sources", "files") {
		return errors.New("清单字段缺失、大小写别名或额外字段")
	}
	for _, group := range []struct {
		name string
		keys []string
	}{
		{"sources", []string{"name", "version", "url", "sha256"}},
		{"files", []string{"path", "sha256", "size", "role", "component"}},
	} {
		var records []map[string]json.RawMessage
		if err := json.Unmarshal(object[group.name], &records); err != nil || len(records) == 0 {
			return errors.New("清单记录不能为空")
		}
		for _, record := range records {
			if !exactKeys(record, group.keys...) {
				return errors.New("清单记录字段缺失、别名或额外字段")
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// exactKeys 拒绝JSON结构中的缺失、大小写别名、空值和额外字段。
func exactKeys(object map[string]json.RawMessage, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		value, ok := object[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return true
}

// rejectAliases 检查完整祖先链，防止目录交接或本机重解析点旁加载。
func rejectAliases(path string) error {
	for {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || isReparsePoint(info) {
			return errors.New("文件或父目录包含链接、重解析点或不可读路径")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

// uniqueValue 递归检查有界JSON的每一层对象键是否唯一。
func uniqueValue(decoder *json.Decoder, depth int) error {
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
			if err := uniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("无效JSON结构")
	}
	_, err = decoder.Token()
	return err
}
