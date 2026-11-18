package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// timeFromNano 将源文件时间转换为 HTTP 缓存时间。
func timeFromNano(value int64) time.Time { return time.Unix(0, value) }

// cachePath 按内容编号与源文件版本隔离封面，不扫描完整视频计算哈希。
func (a *App) cachePath(item Item) string {
	return filepath.Join(a.config.DataDir, "cache", fmt.Sprintf("%d-%x-%x.jpg", item.ID, item.ModTime, item.Size))
}

// cover 只读取已存在缓存或返回占位封面，不在 HTTP 路径启动重处理。
func (a *App) cover(w http.ResponseWriter, r *http.Request) {
	item, ok := a.requestItem(w, r)
	if !ok {
		return
	}
	// 缓存也不能绕过冻结或未发布下载的可见性限制。
	if err := a.checkDownloadPath(filepath.Join(item.LibraryPath, filepath.FromSlash(item.RelativePath)), item.IsDir); err != nil {
		fail(w, 404, err.Error())
		return
	}
	if file, err := os.Open(a.cachePath(item)); err == nil {
		defer file.Close()
		if info, statErr := file.Stat(); statErr == nil {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "private, max-age=300")
			http.ServeContent(w, r, info.Name(), info.ModTime(), file)
			return
		}
	}
	palette := map[string][2]string{"video": {"#1e3a46", "#5eead4"}, "image": {"#3b3150", "#d8b4fe"}, "comic": {"#3b3a25", "#fcd34d"}, "novel": {"#47352c", "#fdba74"}}
	colors := palette[item.Type]
	runes := []rune(item.Title)
	if len(runes) > 30 {
		runes = append(runes[:29], '…')
	}
	lines := []string{}
	for len(runes) > 0 {
		length := 10
		if len(runes) < length {
			length = len(runes)
		}
		lines = append(lines, string(runes[:length]))
		runes = runes[length:]
	}
	var title strings.Builder
	for index, line := range lines {
		title.WriteString(fmt.Sprintf(`<tspan x="32" y="%d">%s</tspan>`, 320+index*37, html.EscapeString(line)))
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 360 480"><rect width="360" height="480" fill="%s"/><path d="M32 67h70M32 78h40" stroke="%s" stroke-width="3"/><circle cx="280" cy="136" r="80" fill="%s" opacity=".09"/><path d="M73 219l78-97 82 97z" fill="%s" opacity=".12"/><text x="32" y="272" font-family="sans-serif" font-size="13" letter-spacing="4" fill="%s">%s</text><text font-family="sans-serif" font-size="26" font-weight="600" fill="#f3f4f6">%s</text></svg>`, colors[0], colors[1], colors[1], colors[1], colors[1], strings.ToUpper(item.Type), title.String())
	if item.Type == "music" {
		svg = musicPlaceholder(item.Title)
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=60")
	_, _ = io.WriteString(w, svg)
}

// cacheEntry 记录可清理衍生文件，不包含原媒体路径。
type cacheEntry struct {
	Path     string    // 缓存文件绝对路径。
	Size     int64     // 文件大小。
	Modified time.Time // 生成时间，用于最早优先清理。
}

// ensureCacheBudget 在持有缓存锁时检查剩余空间并清理最早衍生物。
func (a *App) ensureCacheBudget(needed int64) error {
	free, err := freeDiskBytes(a.config.DataDir)
	if err != nil {
		return fmt.Errorf("无法检测缓存剩余空间")
	}
	if free < a.config.MinFreeBytes+uint64(needed) {
		return fmt.Errorf("剩余空间不足，已暂停生成封面")
	}
	dir := filepath.Join(a.config.DataDir, "cache")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	files := []cacheEntry{}
	var used int64
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".pending-") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		used += info.Size()
		files = append(files, cacheEntry{Path: filepath.Join(dir, entry.Name()), Size: info.Size(), Modified: info.ModTime()})
	}
	sort.Slice(files, func(i int, j int) bool { return files[i].Modified.Before(files[j].Modified) })
	for _, file := range files {
		if used+needed <= a.config.CacheMaxBytes {
			break
		}
		if os.Remove(file.Path) == nil {
			used -= file.Size
		}
	}
	if used+needed > a.config.CacheMaxBytes {
		return fmt.Errorf("封面缓存容量不足")
	}
	return nil
}

// publishCover 原子发布受限封面，避免请求读到半写文件。
func (a *App) publishCover(item Item, data []byte) error {
	if len(data) > 2<<20 {
		return fmt.Errorf("生成的封面超过 2 MiB 限制")
	}
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if _, statErr := os.Stat(a.cachePath(item)); statErr == nil {
		return nil
	}
	if err := a.ensureCacheBudget(int64(len(data))); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Join(a.config.DataDir, "cache"), ".pending-*.jpg")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, a.cachePath(item)); err != nil {
		return err
	}
	return nil
}

// thumbnail 在解码前校验像素预算，输出不超过 480 像素的 JPEG。
func thumbnail(data []byte) ([]byte, int, int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("图片格式无效或暂不支持")
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 16384 || config.Height > 16384 || int64(config.Width)*int64(config.Height) > 24_000_000 {
		return nil, 0, 0, fmt.Errorf("图片像素超过 2400 万限制，已跳过缩略图")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("图片解码失败")
	}
	ratio := math.Min(1, 480.0/float64(max(config.Width, config.Height)))
	width, height := max(1, int(float64(config.Width)*ratio)), max(1, int(float64(config.Height)*ratio))
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(target, target.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)
	var output bytes.Buffer
	if err = jpeg.Encode(&output, target, &jpeg.Options{Quality: 82}); err != nil {
		return nil, 0, 0, err
	}
	return output.Bytes(), config.Width, config.Height, nil
}

// enrich 受控执行元数据探测与缩略图，入库可见性不依赖它完成。
func (a *App) enrich(ctx context.Context, job Job) (resultErr error) {
	item, err := a.itemByID(ctx, job.ItemID, 1)
	if err != nil {
		return fmt.Errorf("内容已移除")
	}
	state := "done"
	defer func() {
		if resultErr != nil {
			state = "failed"
		}
		_, _ = a.db.ExecContext(a.ctx, "UPDATE items SET enrich_state=? WHERE id=? AND mod_time=? AND size=?", state, item.ID, item.ModTime, item.Size)
	}()
	if item.Type == "novel" {
		return nil
	}
	if item.Type == "music" {
		hasCover, musicErr := a.enrichMusic(ctx, item)
		// 无内嵌封面是正常结果，独立状态避免每次扫描都重新探测。
		if musicErr == nil && !hasCover {
			state = "no_cover"
		}
		return musicErr
	}
	a.cacheMu.Lock()
	err = a.ensureCacheBudget(2 << 20)
	a.cacheMu.Unlock()
	if err != nil {
		return err
	}
	if item.Type == "video" {
		return a.enrichVideo(ctx, item)
	}
	var data []byte
	if item.Type == "comic" {
		data, _, err = a.readComicPage(item, 0)
	} else {
		var file *os.File
		file, err = a.openSource(item)
		if err == nil {
			data, err = io.ReadAll(io.LimitReader(file, maxPageBytes+1))
			file.Close()
			if int64(len(data)) > maxPageBytes {
				err = fmt.Errorf("图片超过 32 MiB 缩略图上限")
			}
		}
	}
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	imageData, width, height, err := thumbnail(data)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err = a.publishCover(item, imageData); err != nil {
		return err
	}
	_, err = a.db.ExecContext(ctx, "UPDATE items SET width=?,height=? WHERE id=? AND mod_time=? AND size=?", width, height, item.ID, item.ModTime, item.Size)
	return err
}

// limitedBuffer 在外部程序异常输出时拒绝额外内存分配。
type limitedBuffer struct {
	Buffer bytes.Buffer // 已捕获的有限输出。
	Limit  int          // 最大可写字节数。
}

// Write 实现 io.Writer，超过预算时让子进程输出管道失败。
func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	if buffer.Buffer.Len()+len(data) > buffer.Limit {
		return 0, fmt.Errorf("外部程序输出超过预算")
	}
	return buffer.Buffer.Write(data)
}

// probeResult 仅接收生成元数据需要的 ffprobe 字段。
type probeResult struct {
	Streams []struct {
		Width  int `json:"width"`  // 视频流宽度。
		Height int `json:"height"` // 视频流高度。
	} `json:"streams"` // 媒体流列表。
	Format struct {
		Duration string `json:"duration"` /* 总时长秒数字符串。 */
	} `json:"format"` // 容器基础信息。
}

// enrichVideo 以有限探测时间和单线程 FFmpeg 生成少量衍生信息。
func (a *App) enrichVideo(ctx context.Context, item Item) error {
	file, err := a.openSource(item)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	file.Close()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("视频文件不可访问")
	}
	source := filepath.Join(item.LibraryPath, filepath.FromSlash(item.RelativePath))
	safe, err := a.allowedPath(source)
	if err != nil {
		return err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output := &limitedBuffer{Limit: 64 << 10}
	command, err := a.mediaCommand(probeCtx, "ffprobe", "-v", "error", "-max_alloc", "67108864", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm,avi,mpegts,mpeg", "-probesize", "8388608", "-analyzeduration", "5000000", "-show_entries", "stream=width,height:format=duration", "-of", "json", safe)
	if err != nil {
		return err
	}
	command.Stdout = output
	command.Stderr = io.Discard
	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("视频探测未完成：格式不支持、工具缺失或超过 20 秒预算，可手动重试")
	}
	var probe probeResult
	if json.Unmarshal(output.Buffer.Bytes(), &probe) != nil {
		return fmt.Errorf("视频元数据解析失败")
	}
	duration, _ := strconv.ParseFloat(probe.Format.Duration, 64)
	if duration < 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		duration = 0
	}
	width, height := 0, 0
	for _, stream := range probe.Streams {
		if stream.Width > 0 && stream.Height > 0 {
			width, height = stream.Width, stream.Height
			break
		}
	}
	if int64(width)*int64(height) > 40_000_000 || width > 16384 || height > 16384 {
		return fmt.Errorf("视频分辨率超过受控封面处理范围")
	}
	if _, err = a.db.ExecContext(ctx, "UPDATE items SET duration=?,width=?,height=? WHERE id=? AND mod_time=? AND size=?", duration, width, height, item.ID, item.ModTime, item.Size); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Join(a.config.DataDir, "cache"), ".pending-*.jpg")
	if err != nil {
		return err
	}
	target := temporary.Name()
	temporary.Close()
	os.Remove(target)
	defer os.Remove(target)
	thumbCtx, stop := context.WithTimeout(ctx, 25*time.Second)
	defer stop()
	seek := "0"
	if duration > 30 {
		seek = "10"
	}
	command, err = a.mediaCommand(thumbCtx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-max_alloc", "67108864", "-threads", "1", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm,avi,mpegts,mpeg", "-probesize", "8388608", "-analyzeduration", "5000000", "-ss", seek, "-i", safe, "-map", "0:v:0", "-frames:v", "1", "-an", "-sn", "-vf", "scale=480:480:force_original_aspect_ratio=decrease", "-threads", "1", "-q:v", "4", "-fs", "2097152", target)
	if err != nil {
		return err
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("视频信息已登记，封面生成未完成：可手动重试")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("视频封面不可读取")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return a.publishCover(item, data)
}
