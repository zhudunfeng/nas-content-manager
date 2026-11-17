package app

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// 这些上限分别限制归档大小、目录规模和解压后的单页资源。
const (
	maxArchiveBytes int64 = 2 << 30  // CBZ 原归档最大 2 GiB。
	maxComicPages   int   = 10000    // 单本漫画最多 10000 个页面文件。
	maxPageBytes    int64 = 32 << 20 // 单张页面解压后最多 32 MiB。
	maxTextBytes    int64 = 16 << 20 // TXT 原始字节最多 16 MiB。
)

// comicPageInfo 是按自然页序排列的漫画阅读元数据。
type comicPageInfo struct {
	Index int    `json:"index"` // 零起始页码。
	Name  string `json:"name"`  // 原始页面名称。
	URL   string `json:"url"`   // 鉴权后的页面地址。
}

// chapterInfo 表示自动识别的 TXT 章节标题。
type chapterInfo struct {
	Title string `json:"title"` // 章节标题原文。
	Index int    `json:"index"` // 章节首段的零起始序号。
}

// acquireReader 限制昂贵读取的并发，繁忙时返回可重试提示。
func (a *App) acquireReader(w http.ResponseWriter) bool {
	select {
	case a.readerSlots <- struct{}{}:
		return true
	default:
		fail(w, 503, "阅读请求较多，请稍后重试")
		return false
	}
}

// naturalLess 用数字长度和数字字面值排序，正确处理 2、10 和超长数字。
func naturalLess(left string, right string) bool {
	a, b := []rune(strings.ToLower(left)), []rune(strings.ToLower(right))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] >= '0' && a[i] <= '9' && b[j] >= '0' && b[j] <= '9' {
			startI, startJ := i, j
			for i < len(a) && a[i] >= '0' && a[i] <= '9' {
				i++
			}
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				j++
			}
			digitsA, digitsB := strings.TrimLeft(string(a[startI:i]), "0"), strings.TrimLeft(string(b[startJ:j]), "0")
			if len(digitsA) != len(digitsB) {
				return len(digitsA) < len(digitsB)
			}
			if digitsA != digitsB {
				return digitsA < digitsB
			}
			if i-startI != j-startJ {
				return i-startI < j-startJ
			}
			continue
		}
		if a[i] != b[j] {
			return a[i] < b[j]
		}
		i++
		j++
	}
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return left < right
}

// preflightZIP 检查中心目录预算，避免解析归档时先分配海量条目。
func preflightZIP(file *os.File, size int64) error {
	if size < 22 || size > maxArchiveBytes {
		return fmt.Errorf("CBZ 需为有效 ZIP，且不超过 2 GiB")
	}
	tailSize := int64(65557)
	if size < tailSize {
		tailSize = size
	}
	tail := make([]byte, int(tailSize))
	if _, err := file.ReadAt(tail, size-tailSize); err != nil {
		return err
	}
	for offset := len(tail) - 22; offset >= 0; offset-- {
		if !bytes.Equal(tail[offset:offset+4], []byte{'P', 'K', 5, 6}) {
			continue
		}
		comment := int(binary.LittleEndian.Uint16(tail[offset+20 : offset+22]))
		if offset+22+comment != len(tail) {
			continue
		}
		record := tail[offset:]
		count := int(binary.LittleEndian.Uint16(record[10:12]))
		directorySize := binary.LittleEndian.Uint32(record[12:16])
		if binary.LittleEndian.Uint16(record[4:6]) != 0 || binary.LittleEndian.Uint16(record[6:8]) != 0 || count == 65535 || count > maxComicPages+2000 || directorySize > 16<<20 {
			return fmt.Errorf("漫画归档条目过多、中心目录过大或使用了不支持的分卷/ZIP64")
		}
		// 不信任 EOCD 声明的条目数：先以固定缓冲遍历中心目录，防止伪造计数导致大量分配。
		directoryOffset := int64(binary.LittleEndian.Uint32(record[16:20]))
		if directoryOffset+int64(directorySize) > size-tailSize+int64(offset) {
			return fmt.Errorf("漫画中心目录位置无效")
		}
		reader := io.NewSectionReader(file, directoryOffset, int64(directorySize))
		header := make([]byte, 46)
		actualCount := 0
		for reader.Size() > 0 {
			position, seekErr := reader.Seek(0, io.SeekCurrent)
			if seekErr != nil {
				return fmt.Errorf("漫画中心目录不可读取")
			}
			if position == reader.Size() {
				break
			}
			if _, readErr := io.ReadFull(reader, header); readErr != nil || !bytes.Equal(header[:4], []byte{'P', 'K', 1, 2}) {
				return fmt.Errorf("漫画中心目录结构无效")
			}
			length := int64(binary.LittleEndian.Uint16(header[28:30])) + int64(binary.LittleEndian.Uint16(header[30:32])) + int64(binary.LittleEndian.Uint16(header[32:34]))
			if length > reader.Size()-position-46 {
				return fmt.Errorf("漫画中心目录长度无效")
			}
			if _, seekErr = reader.Seek(length, io.SeekCurrent); seekErr != nil {
				return fmt.Errorf("漫画中心目录不可读取")
			}
			actualCount++
			if actualCount > maxComicPages+2000 {
				return fmt.Errorf("漫画归档条目过多")
			}
		}
		if actualCount != count {
			return fmt.Errorf("漫画归档条目计数无效")
		}
		return nil
	}
	return fmt.Errorf("无法找到 CBZ 目录，文件可能不完整")
}

// archivePages 读取受预算约束的 CBZ 索引，不向磁盘解压任何路径。
func archivePages(file *os.File) ([]*zip.File, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err = preflightZIP(file, info.Size()); err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(file, info.Size())
	if err != nil {
		return nil, fmt.Errorf("CBZ 解析失败，请检查文件是否完整")
	}
	pages := []*zip.File{}
	var total uint64
	for _, entry := range archive.File {
		clean := path.Clean(strings.ReplaceAll(entry.Name, "\\", "/"))
		if strings.Contains(entry.Name, "\\") || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") {
			return nil, fmt.Errorf("漫画归档含有不安全路径")
		}
		if entry.FileInfo().IsDir() || !isImage(clean) {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxPageBytes) || entry.Flags&1 != 0 || (entry.CompressedSize64 > 0 && entry.UncompressedSize64/entry.CompressedSize64 > 300) {
			return nil, fmt.Errorf("漫画页面超过大小限制、压缩比过高或已加密")
		}
		total += entry.UncompressedSize64
		if total > 2<<30 {
			return nil, fmt.Errorf("漫画解压总量超过 2 GiB 限制")
		}
		pages = append(pages, entry)
		if len(pages) > maxComicPages {
			return nil, fmt.Errorf("漫画超过 10000 页限制")
		}
	}
	sort.Slice(pages, func(i int, j int) bool { return naturalLess(pages[i].Name, pages[j].Name) })
	if len(pages) == 0 {
		return nil, fmt.Errorf("漫画中没有受支持的图片页面")
	}
	return pages, nil
}

// directoryPages 列出漫画目录中直接包含的安全图片，不跟随软链接。
func directoryPages(directory *os.File) ([]string, error) {
	pages := []string{}
	for {
		entries, err := directory.ReadDir(256)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("漫画目录不可访问")
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !isImage(entry.Name()) {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil || !info.Mode().IsRegular() {
				continue
			}
			if info.Size() > maxPageBytes {
				return nil, fmt.Errorf("漫画页面超过 32 MiB 限制")
			}
			pages = append(pages, entry.Name())
			if len(pages) > maxComicPages {
				return nil, fmt.Errorf("漫画超过 10000 页限制")
			}
		}
		if err == io.EOF {
			break
		}
	}
	sort.Slice(pages, func(i int, j int) bool { return naturalLess(pages[i], pages[j]) })
	if len(pages) == 0 {
		return nil, fmt.Errorf("目录中没有图片页面")
	}
	return pages, nil
}

// comic 返回受约束的漫画页目录，前端按需加载页图。
func (a *App) comic(w http.ResponseWriter, r *http.Request) {
	if !a.acquireReader(w) {
		return
	}
	defer func() { <-a.readerSlots }()
	item, ok := a.requestItem(w, r)
	if !ok {
		return
	}
	if item.Type != "comic" {
		fail(w, 400, "此内容不是漫画")
		return
	}
	file, err := a.openSource(item)
	if err != nil {
		fail(w, 404, err.Error())
		return
	}
	defer file.Close()
	names := []string{}
	if item.IsDir {
		names, err = directoryPages(file)
	} else {
		var pages []*zip.File
		pages, err = archivePages(file)
		for _, page := range pages {
			names = append(names, page.Name)
		}
	}
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	pages := []comicPageInfo{}
	for index, name := range names {
		pages = append(pages, comicPageInfo{Index: index, Name: name, URL: fmt.Sprintf("/api/items/%d/pages/%d", item.ID, index)})
	}
	respond(w, 200, map[string]any{"pages": pages})
}

// readComicPage 有界读取一张页面，用于页面响应或后台封面。
func (a *App) readComicPage(item Item, index int) ([]byte, string, error) {
	file, err := a.openSource(item)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	if index < 0 {
		return nil, "", fmt.Errorf("页码无效")
	}
	if item.IsDir {
		names, readErr := directoryPages(file)
		if readErr != nil {
			return nil, "", readErr
		}
		if index >= len(names) {
			return nil, "", fmt.Errorf("页码超出漫画范围")
		}
		pageItem := item
		pageItem.RelativePath = filepath.ToSlash(filepath.Join(filepath.FromSlash(item.RelativePath), names[index]))
		page, openErr := a.openSource(pageItem)
		if openErr != nil {
			return nil, "", openErr
		}
		defer page.Close()
		data, readErr := io.ReadAll(io.LimitReader(page, maxPageBytes+1))
		if readErr != nil || int64(len(data)) > maxPageBytes {
			return nil, "", fmt.Errorf("页面读取失败或超过大小限制")
		}
		return data, fileMIME(names[index]), nil
	}
	pages, err := archivePages(file)
	if err != nil {
		return nil, "", err
	}
	if index >= len(pages) {
		return nil, "", fmt.Errorf("页码超出漫画范围")
	}
	page, err := pages[index].Open()
	if err != nil {
		return nil, "", fmt.Errorf("页面无法解压")
	}
	defer page.Close()
	data, err := io.ReadAll(io.LimitReader(page, maxPageBytes+1))
	if err != nil || int64(len(data)) > maxPageBytes {
		return nil, "", fmt.Errorf("页面解压失败或超过大小限制")
	}
	return data, fileMIME(pages[index].Name), nil
}

// comicPage 返回单张受限页面，禁止无界解压与任意磁盘写入。
func (a *App) comicPage(w http.ResponseWriter, r *http.Request) {
	if !a.acquireReader(w) {
		return
	}
	defer func() { <-a.readerSlots }()
	item, ok := a.requestItem(w, r)
	if !ok {
		return
	}
	if item.Type != "comic" {
		fail(w, 400, "此内容不是漫画")
		return
	}
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		fail(w, 400, "页码无效")
		return
	}
	data, contentType, err := a.readComicPage(item, index)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeContent(w, r, fmt.Sprintf("page-%d", index), timeFromNano(item.ModTime), bytes.NewReader(data))
}

// text 安全读取小型 TXT，自动检测 UTF-8/GB18030 并生成目录。
func (a *App) text(w http.ResponseWriter, r *http.Request) {
	if !a.acquireReader(w) {
		return
	}
	defer func() { <-a.readerSlots }()
	item, ok := a.requestItem(w, r)
	if !ok {
		return
	}
	if item.Type != "novel" || strings.ToLower(filepath.Ext(item.RelativePath)) != ".txt" {
		fail(w, 400, "此内容不是 TXT 小说")
		return
	}
	file, err := a.openSource(item)
	if err != nil {
		fail(w, 404, err.Error())
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > maxTextBytes {
		fail(w, 413, "TXT 超过首版 16 MiB 阅读上限，请拆分后导入")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTextBytes+1))
	if err != nil || int64(len(data)) > maxTextBytes {
		fail(w, 413, "TXT 读取失败或超过大小限制")
		return
	}
	encoding := strings.ToLower(r.URL.Query().Get("encoding"))
	if encoding != "" && encoding != "utf-8" && encoding != "utf8" && encoding != "gb18030" && encoding != "gbk" {
		fail(w, 400, "仅支持 UTF-8 或 GB18030 编码")
		return
	}
	if encoding == "gb18030" || encoding == "gbk" || (encoding == "" && !utf8.Valid(data)) {
		data, err = io.ReadAll(io.LimitReader(transform.NewReader(bytes.NewReader(data), simplifiedchinese.GB18030.NewDecoder()), maxTextBytes*2+1))
		if err != nil || int64(len(data)) > maxTextBytes*2 {
			fail(w, 422, "GB18030 解码失败")
			return
		}
	} else if !utf8.Valid(data) {
		fail(w, 422, "文本不是有效 UTF-8，请切换为 GB18030")
		return
	}
	paragraphs := []string{}
	chapters := []chapterInfo{}
	content := strings.TrimPrefix(string(data), "\ufeff")
	expression := regexp.MustCompile(`^(第.{1,18}[章节卷回部篇]|序章|序言|楔子|后记|尾声|Chapter\s+\d+)`)
	// 逐行迭代避免大量空行先生成数百万个字符串切片，段落预算在解析中生效。
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		if len(paragraphs) >= 100000 {
			fail(w, 413, "文本段落超过 10 万段，请拆分后导入")
			return
		}
		if utf8.RuneCountInString(line) < 100 && expression.MatchString(line) && len(chapters) < 3000 {
			chapters = append(chapters, chapterInfo{Title: line, Index: len(paragraphs)})
		}
		paragraphs = append(paragraphs, line)
	}
	if len(chapters) == 0 && len(paragraphs) > 0 {
		chapters = append(chapters, chapterInfo{Title: "正文", Index: 0})
	}
	respond(w, 200, map[string]any{"title": item.Title, "paragraphs": paragraphs, "chapters": chapters, "truncated": false})
}
