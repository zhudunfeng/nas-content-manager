package collectionworker

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	contract "nas-content-manager/internal/collectioncontract"
	"nas-content-manager/internal/uavruntime"
)

// uavVersion 与 uavCommit 锁定已审核上游源码及无头协议。
const uavVersion = "3.8.4"
const uavCommit = "d40f09457fbd38dd7dc94580637242afafaf3939"

// uavYTDLPVersion 绑定已验证内联HLS和有限片段行为的视频下载器版本，不限制原通用网页能力。
const uavYTDLPVersion = "2026.10.26"

// uavBridge 随执行端固定分发，用户不能替换脚本或选择模块。
//
//go:embed uav_bridge.py
var uavBridge string

// uavInput 是私有固定动作请求，代理仅由执行端注入。
type uavInput struct {
	Action       string `json:"action"`                 // 固定probe/search/resolve动作。
	Site         string `json:"site,omitempty"`         // 已验证站点。
	Query        string `json:"query,omitempty"`        // 有界搜索关键词。
	Page         int    `json:"page,omitempty"`         // 有界搜索页码。
	VideoID      string `json:"videoId,omitempty"`      // 规范视频编号。
	Proxy        string `json:"proxy,omitempty"`        // 本实例受控公网代理。
	SourceOffset int    `json:"sourceOffset,omitempty"` // SupJav私有固定线路起点，不接受公开请求或持久化。
}

// uavMedia 只在私有进程之间流转播放信息，不返回浏览器或日志。
type uavMedia struct {
	Site        string            `json:"site"`                  // 固定站点。
	VideoID     string            `json:"videoId"`               // 规范视频编号。
	Title       string            `json:"title"`                 // 纯文本标题。
	MediaURL    string            `json:"mediaUrl"`              // 临时HTTP(S)媒体地址。
	Headers     map[string]string `json:"headers"`               // 仅允许Referer和Origin。
	Manifest    string            `json:"manifest,omitempty"`    // 已验证有限HLS媒体清单，直链为空。
	CDN         *uavCDN           `json:"cdn,omitempty"`         // 仅黄果使用的短期CDN作用域，不写持久状态。
	SourceIndex int               `json:"sourceIndex,omitempty"` // SupJav私有线路槽位，0–5为固定HLS，6为ST直链。
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
	defer func() {
		if capability := s.uavCapability(); !capability.Available {
			s.logger.Warn("视频插件未就绪，原有资源收集保持可用", "原因", capability.Reason)
		}
	}()
	s.uavReason = "视频插件固定依赖探测未通过，请检查源码安装来源、版本、扩展库和证书文件"
	// 显式覆盖优先且失败不回退；托管环境只允许受信绝对解释器或完整私有清单。
	explicit := s.config.UAVPythonExplicit || filepath.IsAbs(s.config.UAVPython)
	if explicit {
		if (s.config.Managed || s.config.UAVRuntimeManifest != "") && !filepath.IsAbs(s.config.UAVPython) {
			s.uavReason = "视频插件的显式Python配置必须是受信绝对路径，未改用其他解释器"
			return
		}
		s.uavPython = toolPath(s.config.UAVPython)
		if s.uavPython == "" {
			s.uavReason = "视频插件显式配置的Python不存在或不可执行，未改用其他解释器"
		}
	} else if s.config.UAVRuntimeManifest != "" {
		packaged, err := uavruntime.Load(s.config.UAVRuntimeManifest)
		if err != nil {
			s.uavReason = "视频插件私有运行时清单、架构、版本或文件完整性校验失败，请重新安装匹配的完整包"
			return
		}
		s.uavPython = packaged.Python
	} else if s.config.Managed {
		s.uavReason = "视频插件私有运行时缺失，请安装含UAV的完整包或显式配置受信Python绝对路径"
		return
	} else {
		// 普通控制台保留原有固定工具名兼容，桌面托管绝不查找系统Python。
		s.uavPython = toolPath(s.config.UAVPython)
		if s.uavPython == "" {
			s.uavReason = "视频插件未找到可运行的Python，请配置受信解释器或私有运行时清单"
		}
	}
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
		return
	}
	s.uavReason = ""
	s.logger.Info("视频解析依赖本地探测通过", "版本", uavVersion)
}

// uavCapability 同时要求固定解析依赖、原生下载器和离线校验工具。
func (s *Service) uavCapability() contract.Capability {
	c := contract.Capability{Kind: "uav", Available: s.uavPython != "" && s.ytdlp != "" && s.ytdlpVersion == uavYTDLPVersion && s.ffmpeg != ""}
	if !s.config.UAVEnabled {
		c.Reason = "视频插件未启用，请配置NAS_COLLECTION_WORKER_UAV_ENABLED=true"
	} else if s.uavPython == "" {
		c.Reason = s.uavReason
		if c.Reason == "" {
			c.Reason = "视频插件需要受信Python和锁定UAV 3.8.4依赖，请检查执行端配置"
		}
	} else if s.ytdlp == "" {
		c.Reason = "视频插件未找到可运行的固定yt-dlp下载器，请检查完整包或执行端工具配置"
	} else if s.ytdlpVersion != uavYTDLPVersion {
		c.Reason = "视频插件需要固定yt-dlp " + uavYTDLPVersion + "，当前下载器版本不匹配；请恢复可信完整包或匹配的执行端工具"
	} else if s.ffmpeg == "" {
		c.Reason = "视频插件未找到可运行的FFmpeg本地校验工具，请检查完整包或执行端工具配置"
	} else {
		c.Version = uavVersion
	}
	return c
}

// initWebTools 共享服务与视频本地诊断的下载器和离线校验工具探测，不访问网络。
func (s *Service) initWebTools() {
	s.ytdlp = toolPath(s.config.YTDLP)
	s.ffmpeg = toolPath(s.config.FFmpeg)
	s.ytdlpVersion = s.version(s.ytdlp, append(s.webArguments(), "--version")...)
	if s.ytdlpVersion == "" {
		s.ytdlp = ""
	}
	if s.version(s.ffmpeg, "-version") == "" {
		s.ffmpeg = ""
	}
}

// CheckUAV 仅在私有临时目录检查固定视频依赖和工具，不监听、不恢复任务或访问站点。
func CheckUAV(ctx context.Context, config Config, logger *slog.Logger) (capability contract.Capability, result error) {
	if logger == nil {
		logger = slog.Default()
	}
	if !config.UAVEnabled {
		service := &Service{config: config}
		return service.uavCapability(), nil
	}
	root, err := os.MkdirTemp("", "qiku-uav-check-")
	if err != nil {
		return contract.Capability{}, errors.New("无法建立视频诊断专用临时目录")
	}
	defer func() {
		if os.RemoveAll(root) != nil {
			result = errors.Join(result, errors.New("视频诊断临时文件未能完整清理"))
		}
	}()
	if err = os.Mkdir(filepath.Join(root, "private-state"), 0700); err != nil {
		return contract.Capability{}, errors.New("无法建立视频诊断状态目录")
	}
	config.DataDir = root
	service := &Service{config: config, logger: logger, ctx: ctx}
	service.initWebTools()
	service.initUAV()
	return service.uavCapability(), nil
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
	body, err := s.runTool(ctx, s.uavPython, []string{"-I", "-B", "-X", "utf8", "-c", uavBridge}, filepath.Join(s.config.DataDir, "private-state"), bytes.NewReader(data), 1<<20, input.Action != "probe")
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
	return s.resolveUAVSource(ctx, site, id, 0)
}

// resolveUAVSource 仅允许执行端在SupJav固定线路中向前重试，候选信息不进入公开协议。
func (s *Service) resolveUAVSource(ctx context.Context, site string, id string, sourceOffset int) (uavMedia, error) {
	var media uavMedia
	if sourceOffset < 0 || sourceOffset > 6 || site != "supjav" && sourceOffset != 0 {
		return media, errors.New("视频播放线路无效")
	}
	if !s.uavCapability().Available {
		return media, errors.New(s.uavCapability().Reason)
	}
	if err := s.callUAV(ctx, uavInput{Action: "resolve", Site: site, VideoID: id, SourceOffset: sourceOffset}, &media); err != nil {
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
	if site == "supjav" {
		if media.SourceIndex < sourceOffset || media.SourceIndex > 6 || (media.SourceIndex == 6) != (media.Manifest == "") || strings.Contains(media.Manifest, "#EXT-X-MAP:") || strings.Contains(media.Manifest, "#EXT-X-BYTERANGE:") {
			return media, errors.New("SupJav播放线路或普通TS清单无效")
		}
	} else if media.SourceIndex != 0 {
		return media, errors.New("视频解析含跨站线路字段")
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
	for offset := 0; offset <= 6; {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		media, err := s.resolveUAVSource(ctx, input.UAVSite, input.VideoID, offset)
		if err != nil {
			return err
		}
		if uavIdentity(media) != identity {
			return errors.New("视频身份已改变，请重新预览")
		}
		err = s.fetchUAVMedia(ctx, input, target, media)
		if err == nil {
			return nil
		}
		if input.UAVSite != "supjav" || media.SourceIndex >= 6 || ctx.Err() != nil || !errors.Is(err, errUAVCandidate) {
			return err
		}
		// 先确认预算与自有暂存安全，清除上一线路残留后才能使用下一线路，禁止拼接不同媒体。
		if budgetErr := s.checkWriteBudget(target, input.MaxBytes, input.MaxFiles); budgetErr != nil {
			return budgetErr
		}
		if cleanErr := cleanUAVCandidate(target); cleanErr != nil {
			return cleanErr
		}
		if reserveErr := s.reserveSpace(input.MaxBytes); reserveErr != nil {
			return reserveErr
		}
		s.logger.Warn("SupJav播放线路未通过获取校验，正在尝试下一线路", "operation", input.ID, "sourceIndex", media.SourceIndex)
		offset = media.SourceIndex + 1
	}
	return errors.New("SupJav所有支持的播放线路均未完成获取")
}

// fetchUAVMedia 仅下载和验证一个已解析线路，每次失败均保留受控错误分类。
func (s *Service) fetchUAVMedia(ctx context.Context, input contract.OperationRequest, target string, media uavMedia) error {
	var err error
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
		if ctx.Err() != nil || errors.Is(err, errUAVStorage) || errors.Is(err, errUAVProcess) {
			return err
		}
		return fmt.Errorf("%w：视频传输失败", errUAVCandidate)
	}
	source := filepath.Join(target, "media.download")
	destination := filepath.Join(target, "media.mp4")
	stat, err := os.Lstat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w：视频下载产物缺失", errUAVCandidate)
		}
		return errors.New("视频下载产物无法读取")
	}
	if !stat.Mode().IsRegular() || stat.Size() > input.MaxBytes/2 {
		return errors.New("视频下载产物不安全或超过离线处理预算")
	}
	if stat.Size() < 1 {
		return fmt.Errorf("%w：视频下载产物为空", errUAVCandidate)
	}
	if input.UAVSite == "supjav" && media.Manifest != "" {
		stripped, normalizeErr := normalizeSupJavTS(ctx, source)
		if normalizeErr != nil {
			return normalizeErr
		}
		s.logger.Info("SupJav视频已完成本地TS格式校验", "operation", input.ID, "removedPrefixBytes", stripped)
	}
	offline := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-err_detect", "explode", "-protocol_whitelist", "file,pipe,crypto,data", "-i", source}
	remux := append(append([]string{}, offline...), "-map", "0:v:0", "-map", "0:a?", "-c", "copy", "-movflags", "+faststart", "-f", "mp4", "-n", destination)
	if _, err = s.runTool(ctx, s.ffmpeg, remux, target, nil, 4096, false); err != nil {
		if ctx.Err() != nil || errors.Is(err, errUAVStorage) || errors.Is(err, errUAVProcess) {
			return err
		}
		return fmt.Errorf("%w：视频离线封装失败，拒绝发布", errUAVCandidate)
	}
	decode := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-nostats", "-progress", "pipe:1", "-stats_period", "86400", "-xerror", "-err_detect", "explode", "-protocol_whitelist", "file,pipe,crypto,data", "-i", destination, "-map", "0:v:0", "-map", "0:a?", "-f", "null", "-"}
	decoded, decodeErr := s.runTool(ctx, s.ffmpeg, decode, target, nil, 4096, false)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(decodeErr, errUAVStorage) || errors.Is(decodeErr, errUAVProcess) {
		return decodeErr
	}
	if decodeErr != nil || !uavDecodedFrames(decoded) || !uavDecodedDuration(decoded, expectedSeconds) {
		return fmt.Errorf("%w：视频完整解码校验失败，拒绝发布", errUAVCandidate)
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
