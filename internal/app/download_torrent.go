package app

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// torrentIdentity 保存协议算法明确的内容身份及可选本地清单。
type torrentIdentity struct {
	V1    string         // BT v1 SHA-1 十六进制身份。
	V2    string         // BT v2 SHA-256 十六进制身份。
	Name  string         // 元数据标题，只用于显示。
	Files []DownloadFile // 本地种子解析得到的安全路径与大小。
}

// bencodeNode 保存有界解码结构和原始字节边界，不重编码 info 计算身份。
type bencodeNode struct {
	Start  int                     // 原始输入开始偏移。
	End    int                     // 原始输入结束偏移。
	Text   []byte                  // 字符串字节。
	Number int64                   // 整数值。
	Kind   byte                    // i/l/d/s 节点类型。
	List   []*bencodeNode          // 列表元素。
	Dict   map[string]*bencodeNode // 不允许重复键的字典。
}

// bencodeParser 持有最大 8 MiB 输入及有限节点计数。
type bencodeParser struct {
	Data   []byte // 当前完整种子字节。
	Offset int    // 当前读取位置。
	Nodes  int    // 已分配节点数。
}

// parseTorrent 校验种子、路径、文件数与安全整数，并对原始 info 字节计算身份。
func parseTorrent(data []byte) (torrentIdentity, error) {
	var result torrentIdentity
	if len(data) == 0 || len(data) > maxTorrentBytes {
		return result, fmt.Errorf("种子文件为空或超过 8 MiB")
	}
	p := bencodeParser{Data: data}
	root, err := p.read(0)
	if err != nil || p.Offset != len(data) || root.Kind != 'd' {
		return result, fmt.Errorf("种子 bencode 结构无效或超过解析预算")
	}
	info := root.Dict["info"]
	if info == nil || info.Kind != 'd' {
		return result, fmt.Errorf("种子缺少有效 info 字典")
	}
	name := info.Dict["name.utf-8"]
	if name == nil {
		name = info.Dict["name"]
	}
	if name == nil || name.Kind != 's' || validateDownloadRelative(string(name.Text)) != nil || strings.ContainsAny(string(name.Text), "/\\") {
		return result, fmt.Errorf("种子名称或路径无效")
	}
	result.Name = string(name.Text)
	if version := info.Dict["meta version"]; version != nil {
		if version.Kind != 'i' || version.Number != 2 {
			return result, fmt.Errorf("不支持此种子元数据版本")
		}
		hash := sha256.Sum256(data[info.Start:info.End])
		result.V2 = hex.EncodeToString(hash[:])
		tree := info.Dict["file tree"]
		if tree == nil || tree.Kind != 'd' {
			return result, fmt.Errorf("v2 种子缺少文件树")
		}
		if err = collectV2Files(tree, "", &result.Files); err != nil {
			return result, err
		}
		// BEP52 单文件树已包含文件名；多文件树才需要在外层补上种子根名称。
		if len(result.Files) != 1 || result.Files[0].Path != result.Name {
			for i := range result.Files {
				result.Files[i].Path = result.Name + "/" + result.Files[i].Path
			}
		}
	}
	if pieces := info.Dict["pieces"]; pieces != nil {
		if pieces.Kind != 's' || len(pieces.Text)%20 != 0 {
			return result, fmt.Errorf("v1 种子分片身份无效")
		}
		hash := sha1.Sum(data[info.Start:info.End])
		result.V1 = hex.EncodeToString(hash[:])
		files, fileErr := collectV1Files(info, result.Name)
		if fileErr != nil {
			return result, fileErr
		}
		if result.V2 == "" {
			result.Files = files
		}
		// 混合种子的两套清单必须一致，不能把一套路径用于另一套内容发布。
		if result.V2 != "" {
			if len(files) != len(result.Files) {
				return result, fmt.Errorf("混合种子的 v1/v2 文件清单不一致")
			}
			byPath := map[string]int64{}
			for _, file := range files {
				byPath[file.Path] = file.SizeBytes
			}
			for _, file := range result.Files {
				if size, ok := byPath[file.Path]; !ok || size != file.SizeBytes {
					return result, fmt.Errorf("混合种子的 v1/v2 文件清单不一致")
				}
			}
		}
	}
	if result.V1 == "" && result.V2 == "" {
		return result, fmt.Errorf("种子没有可识别的 BT v1/v2 身份")
	}
	if err = validateDownloadFiles(result.Files); err != nil {
		return result, err
	}
	return result, nil
}

// read 递归解析有限 bencode，限制深度与节点总数并拒绝重复字典键。
func (p *bencodeParser) read(depth int) (*bencodeNode, error) {
	if depth > 64 || p.Nodes >= 160000 || p.Offset >= len(p.Data) {
		return nil, fmt.Errorf("种子结构超限")
	}
	p.Nodes++
	node := &bencodeNode{Start: p.Offset}
	token := p.Data[p.Offset]
	p.Offset++
	switch token {
	case 'i':
		node.Kind = 'i'
		end := bytes.IndexByte(p.Data[p.Offset:], 'e')
		if end < 0 || end > 20 {
			return nil, fmt.Errorf("整数无效")
		}
		text := string(p.Data[p.Offset : p.Offset+end])
		if text == "" || text == "-0" || (len(text) > 1 && text[0] == '0') || strings.HasPrefix(text, "-0") {
			return nil, fmt.Errorf("整数格式无效")
		}
		for i, digit := range text {
			if digit == '-' && i == 0 {
				continue
			}
			if digit < '0' || digit > '9' {
				return nil, fmt.Errorf("整数格式无效")
			}
		}
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, err
		}
		node.Number = value
		p.Offset += end + 1
	case 'l', 'd':
		node.Kind = token
		if token == 'd' {
			node.Dict = map[string]*bencodeNode{}
		}
		for p.Offset < len(p.Data) && p.Data[p.Offset] != 'e' {
			if token == 'l' {
				child, err := p.read(depth + 1)
				if err != nil {
					return nil, err
				}
				node.List = append(node.List, child)
			} else {
				key, err := p.read(depth + 1)
				if err != nil || key.Kind != 's' || len(key.Text) > 4096 {
					return nil, fmt.Errorf("字典键无效")
				}
				name := string(key.Text)
				if _, exists := node.Dict[name]; exists {
					return nil, fmt.Errorf("字典键重复")
				}
				child, err := p.read(depth + 1)
				if err != nil {
					return nil, err
				}
				node.Dict[name] = child
			}
		}
		if p.Offset >= len(p.Data) {
			return nil, fmt.Errorf("种子未结束")
		}
		p.Offset++
	default:
		if token < '0' || token > '9' {
			return nil, fmt.Errorf("种子标记无效")
		}
		p.Offset--
		colon := bytes.IndexByte(p.Data[p.Offset:], ':')
		if colon < 1 || colon > 10 {
			return nil, fmt.Errorf("字串长度无效")
		}
		digits := p.Data[p.Offset : p.Offset+colon]
		if len(digits) > 1 && digits[0] == '0' {
			return nil, fmt.Errorf("字串长度无效")
		}
		size, err := strconv.ParseInt(string(digits), 10, 32)
		if err != nil || size < 0 || size > int64(len(p.Data)-p.Offset-colon-1) {
			return nil, fmt.Errorf("字串超限")
		}
		p.Offset += colon + 1
		node.Kind = 's'
		node.Text = p.Data[p.Offset : p.Offset+int(size)]
		p.Offset += int(size)
	}
	node.End = p.Offset
	return node, nil
}

// collectV1Files 从单文件或多文件 info 清单提取安全文件，不接受软链接属性。
func collectV1Files(info *bencodeNode, name string) ([]DownloadFile, error) {
	files := []DownloadFile{}
	if list := info.Dict["files"]; list != nil {
		if list.Kind != 'l' || len(list.List) > maxDownloadFiles {
			return nil, fmt.Errorf("种子文件数量超限")
		}
		for _, entry := range list.List {
			if entry.Kind != 'd' {
				return nil, fmt.Errorf("种子文件条目无效")
			}
			if attr := entry.Dict["attr"]; attr != nil && attr.Kind == 's' && bytes.ContainsRune(attr.Text, 'p') {
				continue
			}
			parts := entry.Dict["path.utf-8"]
			if parts == nil {
				parts = entry.Dict["path"]
			}
			if parts == nil || parts.Kind != 'l' || len(parts.List) == 0 {
				return nil, fmt.Errorf("种子文件路径为空")
			}
			names := []string{name}
			for _, part := range parts.List {
				if part.Kind != 's' || strings.ContainsAny(string(part.Text), "/\\") || validateDownloadRelative(string(part.Text)) != nil {
					return nil, fmt.Errorf("种子含不安全路径")
				}
				names = append(names, string(part.Text))
			}
			file, err := torrentFileEntry(entry, strings.Join(names, "/"), len(files))
			if err != nil {
				return nil, err
			}
			files = append(files, file)
		}
	} else {
		file, err := torrentFileEntry(info, name, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

// collectV2Files 遍历 BEP52 文件树；文件名段永远不能包含目录分隔符。
func collectV2Files(node *bencodeNode, prefix string, files *[]DownloadFile) error {
	if len(*files) > maxDownloadFiles || node.Kind != 'd' {
		return fmt.Errorf("v2 文件树无效或超限")
	}
	for name, child := range node.Dict {
		if name == "" {
			file, err := torrentFileEntry(child, prefix, len(*files))
			if err != nil {
				return err
			}
			*files = append(*files, file)
			continue
		}
		if validateDownloadRelative(name) != nil || strings.ContainsAny(name, "/\\") {
			return fmt.Errorf("v2 种子含不安全路径")
		}
		childPath := name
		if prefix != "" {
			childPath = prefix + "/" + name
		}
		if err := collectV2Files(child, childPath, files); err != nil {
			return err
		}
	}
	return nil
}

// torrentFileEntry 校验文件属性与 64 位尺寸，不允许特殊或软链接文件。
func torrentFileEntry(node *bencodeNode, name string, index int) (DownloadFile, error) {
	if node == nil || node.Kind != 'd' {
		return DownloadFile{}, fmt.Errorf("种子文件属性无效")
	}
	length := node.Dict["length"]
	if length == nil || length.Kind != 'i' || length.Number < 0 || length.Number > maxDownloadInteger {
		return DownloadFile{}, fmt.Errorf("种子文件尺寸超出安全整数范围")
	}
	if attr := node.Dict["attr"]; attr != nil && (attr.Kind != 's' || bytes.ContainsAny(attr.Text, "l")) {
		return DownloadFile{}, fmt.Errorf("种子不允许软链接或特殊文件")
	}
	if node.Dict["symlink path"] != nil {
		return DownloadFile{}, fmt.Errorf("种子不允许软链接")
	}
	return DownloadFile{Index: index, Path: name, SizeBytes: length.Number}, nil
}

// validateDownloadRelative 拒绝绝对路径、跨平台设备名、穿越和控制字符。
func validateDownloadRelative(value string) error {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsAny(value, "\\:\x00<>\"|?*") || strings.HasPrefix(value, "/") {
		return fmt.Errorf("下载文件路径无效")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("下载文件路径含控制字符")
		}
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return fmt.Errorf("下载文件路径不安全")
		}
		device := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if device == "CON" || device == "PRN" || device == "AUX" || device == "NUL" || (len(device) == 4 && (strings.HasPrefix(device, "COM") || strings.HasPrefix(device, "LPT")) && device[3] >= '0' && device[3] <= '9') {
			return fmt.Errorf("下载文件路径包含设备名")
		}
	}
	return nil
}

// validateDownloadFiles 统一检查解析器与引擎清单，避免重复、大小写碰撞和数值溢出。
func validateDownloadFiles(files []DownloadFile) error {
	if len(files) == 0 || len(files) > maxDownloadFiles {
		return fmt.Errorf("文件清单为空或超过 20000 项")
	}
	seen := map[string]bool{}
	ids := map[int]bool{}
	var total int64
	for _, file := range files {
		key := strings.ToLower(file.Path)
		if validateDownloadRelative(file.Path) != nil || file.SizeBytes < 0 || file.SizeBytes > maxDownloadInteger-total || file.Index < 0 || ids[file.Index] || seen[key] {
			return fmt.Errorf("文件清单含重复路径、无效标识或超大尺寸")
		}
		seen[key] = true
		ids[file.Index] = true
		total += file.SizeBytes
	}
	for key := range seen {
		for slash := strings.LastIndex(key, "/"); slash > 0; slash = strings.LastIndex(key[:slash], "/") {
			if seen[key[:slash]] {
				return fmt.Errorf("文件与目录路径冲突")
			}
		}
	}
	return nil
}

// parseMagnet 只接受带明确 v1/v2 内容身份的 magnet，不记录私有 Tracker 参数。
func parseMagnet(value string) (torrentIdentity, error) {
	var result torrentIdentity
	if len(value) == 0 || len(value) > 32768 || strings.ContainsAny(value, "\r\n\x00") {
		return result, fmt.Errorf("磁力链接为空或过长")
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "magnet" || u.Host != "" || u.Path != "" || u.Fragment != "" {
		return result, fmt.Errorf("请输入有效 magnet 链接")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return result, fmt.Errorf("磁力参数无效")
	}
	for _, xt := range query["xt"] {
		lower := strings.ToLower(xt)
		if strings.HasPrefix(lower, "urn:btih:") {
			hash := xt[9:]
			var data []byte
			if len(hash) == 40 {
				data, err = hex.DecodeString(hash)
			} else if len(hash) == 32 {
				data, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(hash))
			} else {
				return result, fmt.Errorf("磁力 v1 身份长度无效")
			}
			if err != nil || len(data) != 20 {
				return result, fmt.Errorf("磁力 v1 身份无效")
			}
			hash = hex.EncodeToString(data)
			if result.V1 != "" && result.V1 != hash {
				return result, fmt.Errorf("磁力含冲突身份")
			}
			result.V1 = hash
		} else if strings.HasPrefix(lower, "urn:btmh:") {
			hash := lower[9:]
			if len(hash) != 68 || !strings.HasPrefix(hash, "1220") {
				return result, fmt.Errorf("仅支持 SHA-256 v2 磁力身份")
			}
			if _, err = hex.DecodeString(hash[4:]); err != nil {
				return result, fmt.Errorf("磁力 v2 身份无效")
			}
			if result.V2 != "" && result.V2 != hash[4:] {
				return result, fmt.Errorf("磁力含冲突身份")
			}
			result.V2 = hash[4:]
		}
	}
	if result.V1 == "" && result.V2 == "" {
		return result, fmt.Errorf("磁力缺少 BT v1/v2 内容身份")
	}
	result.Name = "正在获取元数据"
	return result, nil
}
