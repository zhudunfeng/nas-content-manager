package collectionworker

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	contract "nas-content-manager/internal/collectioncontract"
)

// uavVersion 与 uavCommit 锁定已审核上游源码及无头协议。
const uavVersion = "3.8.4"
const uavCommit = "d40f09457fbd38dd7dc94580637242afafaf3939"

// uavBridge 随执行端固定分发，用户不能替换脚本或选择模块。
//
//go:embed uav_bridge.py
var uavBridge string

// uavInput 是私有固定动作请求，代理仅由执行端注入。
type uavInput struct {
	Action  string `json:"action"`            // 固定probe/search/resolve动作。
	Site    string `json:"site,omitempty"`    // 已验证站点。
	Query   string `json:"query,omitempty"`   // 有界搜索关键词。
	Page    int    `json:"page,omitempty"`    // 有界搜索页码。
	VideoID string `json:"videoId,omitempty"` // 规范视频编号。
	Proxy   string `json:"proxy,omitempty"`   // 本实例受控公网代理。
}

// uavMedia 只在私有进程之间流转播放信息，不返回浏览器或日志。
type uavMedia struct {
	Site     string            `json:"site"`               // 固定站点。
	VideoID  string            `json:"videoId"`            // 规范视频编号。
	Title    string            `json:"title"`              // 纯文本标题。
	MediaURL string            `json:"mediaUrl"`           // 临时HTTP(S)媒体地址。
	Headers  map[string]string `json:"headers"`            // 仅允许Referer和Origin。
	Manifest string            `json:"manifest,omitempty"` // 已验证有限HLS媒体清单，直链为空。
	CDN      *uavCDN           `json:"cdn,omitempty"`      // 仅黄果使用的短期CDN作用域，不写持久状态。
}

// uavCDN 保存只允许发往原始媒体源指定资源类型的短期访问头。
type uavCDN struct {
	Origin   string `json:"origin"`   // 原始videoSrc的HTTPS源，不随重定向扩展。
	Auth     string `json:"auth"`     // x-cdn-auth短期访问值，不输出日志。
	App      string `json:"app"`      // x-cdn-app短期应用标记，不输出日志。
	Manifest bool   `json:"manifest"` // 仅允许.m3u8携带访问头。
	Other    bool   `json:"other"`    // 仅允许.ts/.m4s/.key携带访问头。
}

// initUAV 显式启用后才探测锁定包，失败不会影响其他收集能力。
func (s *Service) initUAV() {
	if !s.config.UAVEnabled {
		return
	}
	s.uavPython = toolPath(s.config.UAVPython)
	if s.uavPython == "" {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	var probe struct {
		Version         string `json:"version"`         // 上游固定版本。
		Commit          string `json:"commit"`          // 构建核验的上游提交。
		ProtocolVersion int    `json:"protocolVersion"` // 固定桥接协议版本。
	}
	if s.callUAV(ctx, uavInput{Action: "probe"}, &probe) != nil || probe.Version != uavVersion || probe.Commit != uavCommit || probe.ProtocolVersion != 1 {
		s.uavPython = ""
		s.logger.Warn("视频插件依赖探测未通过，原有资源收集保持可用")
		return
	}
	s.logger.Info("视频搜索下载插件已启用", "version", uavVersion)
}

// uavCapability 同时要求固定解析依赖、原生下载器和离线校验工具。
func (s *Service) uavCapability() contract.Capability {
	c := contract.Capability{Kind: "uav", Available: s.uavPython != "" && s.ytdlp != "" && s.ffmpeg != ""}
	if !s.config.UAVEnabled {
		c.Reason = "视频插件未启用，请配置NAS_COLLECTION_WORKER_UAV_ENABLED=true"
	} else if !c.Available {
		c.Reason = "视频插件需要Python>=3.12、锁定UAV 3.8.4依赖、yt-dlp和ffmpeg，请检查执行端配置"
	} else {
		c.Version = uavVersion
	}
	return c
}

// callUAV 通过标准输入执行固定有界JSON协议，不泄漏工具错误原文。
func (s *Service) callUAV(ctx context.Context, input uavInput, output any) error {
	if s.uavPython == "" {
		return errors.New("视频插件未启用或固定依赖不可用")
	}
	if input.Action != "probe" {
		input.Proxy = s.proxy.url
	}
	data, err := json.Marshal(input)
	if err != nil {
		return errors.New("视频插件请求编码失败")
	}
	body, err := s.runTool(ctx, s.uavPython, []string{"-I", "-B", "-c", uavBridge}, filepath.Join(s.config.DataDir, "private-state"), bytes.NewReader(data), 1<<20, input.Action != "probe")
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(output) != nil {
		return errors.New("视频插件响应格式无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("视频插件响应包含多余内容")
	}
	return nil
}

// uavSearchHTTP 在共享并发槽、生命周期和短超时内搜索，不执行下载。
func (s *Service) uavSearchHTTP(w http.ResponseWriter, r *http.Request) {
	var input contract.UAVSearchRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input, err := contract.NormalizeUAVSearch(input)
	if err != nil {
		failJSON(w, 400, err.Error())
		return
	}
	if !s.uavCapability().Available {
		failJSON(w, 503, s.uavCapability().Reason)
		return
	}
	select {
	case s.webSlots <- struct{}{}:
		defer func() { <-s.webSlots }()
	default:
		failJSON(w, 429, "视频搜索正在繁忙，请稍后重试")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	stop := context.AfterFunc(s.ctx, cancel)
	defer func() { stop(); cancel() }()
	var output contract.UAVSearchResponse
	if err = s.callUAV(ctx, uavInput{Action: "search", Site: input.Site, Query: input.Query, Page: input.Page}, &output); err != nil {
		s.logger.Warn("视频搜索未完成，请检查站点与网络", "site", input.Site)
		failJSON(w, 502, safeExecutionError(err))
		return
	}
	if err = contract.ValidateUAVSearchResponse(input, output); err != nil {
		failJSON(w, 502, err.Error())
		return
	}
	if output.Items == nil {
		output.Items = []contract.UAVSearchItem{}
	}
	for i := range output.Items {
		output.Items[i].Title = safeTitle(output.Items[i].Title)
	}
	s.logger.Info("视频搜索已完成", "site", input.Site, "page", input.Page, "count", len(output.Items))
	writeJSON(w, 200, output)
}

// resolveUAV 独立核验解析身份、临时地址和请求头白名单。
func (s *Service) resolveUAV(ctx context.Context, site string, id string) (uavMedia, error) {
	var media uavMedia
	if !s.uavCapability().Available {
		return media, errors.New(s.uavCapability().Reason)
	}
	if err := s.callUAV(ctx, uavInput{Action: "resolve", Site: site, VideoID: id}, &media); err != nil {
		return media, err
	}
	if media.Site != site || media.VideoID != id || len(media.Title) > 1200 || len(media.Manifest) > 512<<10 || !validUAVURL(media.MediaURL) || len(media.Headers) > 2 {
		return media, errors.New("视频解析返回无效媒体信息")
	}
	for key, value := range media.Headers {
		if (key != "Referer" && key != "Origin") || !validUAVURL(value) {
			return media, errors.New("视频解析返回不允许的请求头")
		}
	}
	if media.Manifest != "" && (!strings.HasPrefix(strings.TrimSpace(media.Manifest), "#EXTM3U") || !strings.Contains(media.Manifest, "#EXT-X-ENDLIST")) {
		return media, errors.New("仅支持完整的有限HLS视频")
	}
	if media.CDN != nil && (site != "huangguo" || !validUAVCDN(*media.CDN)) {
		return media, errors.New("视频CDN访问作用域无效")
	}
	if site == "huangguo" && media.Manifest == "" {
		return media, errors.New("黄果只支持有限HLS单集视频")
	}
	return media, nil
}

// validUAVURL 拒绝凭据、非HTTP协议和异常端口，公网IP由实际连接代理核验。
func validUAVURL(value string) bool {
	u, err := url.Parse(value)
	if err != nil || len(value) > 8192 || strings.ContainsAny(value, "\r\n\x00") || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	_, _, err = publicAddress(u.Host, map[bool]string{true: "443", false: "80"}[u.Scheme == "https"])
	return err == nil
}

// uavIdentity 绑定规范内容身份与标题，不绑定会自动轮换的签名播放链接。
func uavIdentity(media uavMedia) string {
	return digestJSON([]string{"uav", media.Site, media.VideoID, safeTitle(media.Title)})
}

// previewUAV 只解析身份元信息，浏览器看不到媒体链接或请求头。
func (s *Service) previewUAV(ctx context.Context, input contract.PreviewRequest) (contract.Preview, error) {
	result := contract.Preview{Files: []contract.File{}, Formats: []contract.Format{{ID: "video", Label: "保存视频（MP4）", Extension: "mp4"}}}
	media, err := s.resolveUAV(ctx, input.UAVSite, input.VideoID)
	if err != nil {
		return result, err
	}
	result.Title = safeTitle(media.Title)
	result.Identity = uavIdentity(media)
	return result, nil
}

// fetchUAV 原生获取后离线封装和全程解码，任何失败均不发布。
func (s *Service) fetchUAV(ctx context.Context, input contract.OperationRequest, target string, identity string) error {
	media, err := s.resolveUAV(ctx, input.UAVSite, input.VideoID)
	if err != nil {
		return err
	}
	if uavIdentity(media) != identity {
		return errors.New("视频身份已改变，请重新预览")
	}
	expectedSeconds, durationErr := uavHLSSeconds(media.Manifest)
	if durationErr != nil {
		return durationErr
	}
	if media.Manifest != "" {
		var cleanup func()
		media, cleanup, err = s.proxy.registerUAVMedia(ctx, media, input.MaxBytes/2)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	protocol := "https"
	if strings.HasPrefix(media.MediaURL, "http:") {
		protocol = "http"
	}
	if media.Manifest != "" {
		protocol = "m3u8_native"
	}
	info := map[string]any{"id": media.VideoID, "title": "media", "url": media.MediaURL, "protocol": protocol, "ext": "mp4", "http_headers": media.Headers, "extractor": "UAV", "extractor_key": "UAV", "is_live": false}
	if media.Manifest != "" {
		info["hls_media_playlist_data"] = media.Manifest
	}
	data, err := json.Marshal(info)
	if err != nil {
		return errors.New("视频下载意图编码失败")
	}
	args := s.webArguments()
	args = args[:len(args)-2]
	// 禁用下载器发现FFmpeg，任何原生HLS不支持特性都不能回退为联网进程。
	args = append(args, "--load-info-json", "-", "--output", "media.download", "--fixup", "never", "--ffmpeg-location", filepath.Join(target, ".no-network-ffmpeg"), "--no-overwrites", "--no-continue", "--concurrent-fragments", "1", "--max-filesize", fmt.Sprint(input.MaxBytes/2), "--no-write-info-json", "--no-write-thumbnail", "--no-write-subs")
	if _, err = s.runTool(ctx, s.ytdlp, args, target, bytes.NewReader(data), 64<<10, true); err != nil {
		return err
	}
	source := filepath.Join(target, "media.download")
	destination := filepath.Join(target, "media.mp4")
	stat, err := os.Lstat(source)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() < 1 || stat.Size() > input.MaxBytes/2 {
		return errors.New("视频下载产物缺失或超过离线处理预算")
	}
	offline := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-err_detect", "explode", "-protocol_whitelist", "file,pipe,crypto,data", "-i", source}
	remux := append(append([]string{}, offline...), "-map", "0:v:0", "-map", "0:a?", "-c", "copy", "-movflags", "+faststart", "-f", "mp4", "-n", destination)
	if _, err = s.runTool(ctx, s.ffmpeg, remux, target, nil, 4096, false); err != nil {
		return errors.New("视频离线封装失败，拒绝发布")
	}
	decode := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-nostats", "-progress", "pipe:1", "-stats_period", "86400", "-xerror", "-err_detect", "explode", "-protocol_whitelist", "file,pipe,crypto,data", "-i", destination, "-map", "0:v:0", "-map", "0:a?", "-f", "null", "-"}
	decoded, decodeErr := s.runTool(ctx, s.ffmpeg, decode, target, nil, 4096, false)
	if decodeErr != nil || !uavDecodedFrames(decoded) || !uavDecodedDuration(decoded, expectedSeconds) {
		return errors.New("视频完整解码校验失败，拒绝发布")
	}
	if err = os.Remove(source); err != nil {
		return errors.New("视频临时产物无法清理，拒绝发布")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 1 || entries[0].Name() != "media.mp4" {
		return errors.New("视频下载留下非预期产物，拒绝发布")
	}
	s.logger.Info("视频已通过离线完整性校验，等待独立发布", "operation", input.ID, "site", input.UAVSite)
	return nil
}

// uavDecodedFrames 只承认正常结束且实际解码至少一帧的视频，拒绝空轨道。
func uavDecodedFrames(data []byte) bool {
	frames := int64(0)
	ended := false
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		if key == "frame" {
			parsed, parseErr := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if parseErr == nil && parsed > frames {
				frames = parsed
			}
		}
		if key == "progress" && value == "end" {
			ended = true
		}
	}
	return ended && frames > 0
}

// uavHLSSeconds 汇总有限清单的实际片段时长，直链无清单时不推测总时长。
func uavHLSSeconds(manifest string) (float64, error) {
	if manifest == "" {
		return 0, nil
	}
	total := float64(0)
	segments := 0
	for _, line := range strings.Split(manifest, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#EXTINF:") {
			continue
		}
		value, _, comma := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
		seconds, parseErr := strconv.ParseFloat(value, 64)
		if !comma || parseErr != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
			return 0, errors.New("视频清单片段时长无效")
		}
		total += seconds
		segments++
		if segments > 20000 || math.IsInf(total, 0) {
			return 0, errors.New("视频清单时长或片段数量超限")
		}
	}
	if segments == 0 {
		return 0, errors.New("视频清单缺少片段时长")
	}
	return total, nil
}

// uavDecodedDuration 用清单时长下界阻止明显丢尾，不把可解码误认为保证原片全长。
func uavDecodedDuration(data []byte, expectedSeconds float64) bool {
	if expectedSeconds == 0 {
		return true
	}
	if expectedSeconds < 0 || math.IsNaN(expectedSeconds) || math.IsInf(expectedSeconds, 0) {
		return false
	}
	actualMicroseconds := int64(-1)
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "out_time_us" {
			parsed, parseErr := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if parseErr == nil {
				actualMicroseconds = parsed
			}
		}
	}
	tolerance := math.Max(0.5, expectedSeconds*0.02)
	return actualMicroseconds >= 0 && float64(actualMicroseconds)/1e6 >= expectedSeconds-tolerance
}
