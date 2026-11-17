package app

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// audioMIME 集中定义支持的音频扩展名与稳定类型，避免平台 MIME 注册表差异。
func audioMIME(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".flac":
		return "audio/flac"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".aif", ".aiff":
		return "audio/aiff"
	}
	return ""
}

// musicPlaceholder 生成无外部资源的方形唱片占位图，标题始终按文本转义。
func musicPlaceholder(title string) string {
	runes := []rune(title)
	if len(runes) > 16 {
		runes = append(runes[:15], '…')
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 480 480"><rect width="480" height="480" fill="#203e3b"/><circle cx="240" cy="205" r="140" fill="#142e2c"/><g fill="none" stroke="#5eead4" opacity=".18"><circle cx="240" cy="205" r="123"/><circle cx="240" cy="205" r="106"/><circle cx="240" cy="205" r="89"/></g><circle cx="240" cy="205" r="49" fill="#5eead4"/><circle cx="240" cy="205" r="9" fill="#203e3b"/><text x="32" y="394" fill="#5eead4" font-family="sans-serif" font-size="13" letter-spacing="4">MUSIC / QIKU</text><text x="32" y="438" fill="#f3f4f6" font-family="sans-serif" font-size="24" font-weight="600">%s</text></svg>`, html.EscapeString(string(runes)))
}

// audioProbeResult 只接收时长、音频存在性与内嵌封面所需字段。
type audioProbeResult struct {
	Streams []struct {
		Index       int    `json:"index"`      // 容器中的流编号。
		CodecType   string `json:"codec_type"` // audio 或 video 流类型。
		Width       int    `json:"width"`      // 内嵌封面宽度。
		Height      int    `json:"height"`     // 内嵌封面高度。
		Disposition struct {
			AttachedPic int `json:"attached_pic"` // 是否为内嵌专辑封面。
		} `json:"disposition"` // 流用途标记。
	} `json:"streams"` // 有界探测输出的流列表。
	Format struct {
		Duration string `json:"duration"` // 容器总时长秒数字符串。
	} `json:"format"` // 容器基础信息。
}

// enrichMusic 有界探测音频时长与可选封面，返回是否实际生成了封面。
func (a *App) enrichMusic(ctx context.Context, item Item) (bool, error) {
	file, err := a.openSource(item)
	if err != nil {
		return false, err
	}
	info, err := file.Stat()
	file.Close()
	if err != nil || !info.Mode().IsRegular() {
		return false, fmt.Errorf("音乐文件不可访问")
	}
	source, err := a.allowedPath(filepath.Join(item.LibraryPath, filepath.FromSlash(item.RelativePath)))
	if err != nil {
		return false, err
	}
	// 仅允许本地音频容器，拒绝播放列表联网；不读取整首音频到应用内存。
	formats := "mp3,mov,aac,flac,ogg,wav,aiff"
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output := &limitedBuffer{Limit: 64 << 10}
	command := exec.CommandContext(probeCtx, "ffprobe", "-v", "error", "-max_alloc", "67108864", "-protocol_whitelist", "file,pipe", "-format_whitelist", formats, "-probesize", "8388608", "-analyzeduration", "5000000", "-show_entries", "stream=index,codec_type,width,height:stream_disposition=attached_pic:format=duration", "-of", "json", source)
	command.Stdout = output
	command.Stderr = io.Discard
	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, fmt.Errorf("音乐探测未完成：格式不支持、工具缺失或超过 20 秒预算，可手动重试")
	}
	var probe audioProbeResult
	if json.Unmarshal(output.Buffer.Bytes(), &probe) != nil {
		return false, fmt.Errorf("音乐元数据解析失败")
	}
	hasAudio, coverIndex := false, -1
	width, height := 0, 0
	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" {
			hasAudio = true
		}
		if coverIndex < 0 && stream.Index >= 0 && stream.CodecType == "video" && stream.Disposition.AttachedPic == 1 {
			coverIndex, width, height = stream.Index, stream.Width, stream.Height
		}
	}
	if !hasAudio {
		return false, fmt.Errorf("文件没有可识别的音频流，请检查音乐格式")
	}
	duration, _ := strconv.ParseFloat(probe.Format.Duration, 64)
	if duration < 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		duration = 0
	}
	if _, err = a.db.ExecContext(ctx, "UPDATE items SET duration=? WHERE id=? AND mod_time=? AND size=?", duration, item.ID, item.ModTime, item.Size); err != nil {
		return false, err
	}
	if coverIndex < 0 {
		a.logger.Info("音乐时长已登记，使用默认唱片封面", "itemId", item.ID, "duration", duration)
		return false, nil
	}
	if width < 1 || height < 1 || width > 16384 || height > 16384 || int64(width)*int64(height) > 24_000_000 {
		return false, fmt.Errorf("音乐时长已登记，内嵌封面超过 2400 万像素处理范围")
	}
	a.cacheMu.Lock()
	err = a.ensureCacheBudget(2 << 20)
	a.cacheMu.Unlock()
	if err != nil {
		return false, err
	}
	// 子进程输出同样受 2 MiB 限制，再复用缩略图校验与原子缓存发布。
	coverCtx, stop := context.WithTimeout(ctx, 25*time.Second)
	defer stop()
	cover := &limitedBuffer{Limit: 2 << 20}
	command = exec.CommandContext(coverCtx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-max_alloc", "67108864", "-threads", "1", "-protocol_whitelist", "file,pipe", "-format_whitelist", formats, "-probesize", "8388608", "-analyzeduration", "5000000", "-i", source, "-map", "0:"+strconv.Itoa(coverIndex), "-frames:v", "1", "-an", "-sn", "-vf", "scale=480:480:force_original_aspect_ratio=decrease", "-threads", "1", "-q:v", "4", "-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1")
	command.Stdout = cover
	command.Stderr = io.Discard
	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, fmt.Errorf("音乐时长已登记，内嵌封面生成未完成：可手动重试")
	}
	data, _, _, err := thumbnail(cover.Buffer.Bytes())
	if err != nil {
		return false, fmt.Errorf("音乐内嵌封面无效：%w", err)
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err = a.publishCover(item, data); err != nil {
		return false, err
	}
	a.logger.Info("音乐时长与专辑封面已生成", "itemId", item.ID, "duration", duration)
	return true, nil
}
