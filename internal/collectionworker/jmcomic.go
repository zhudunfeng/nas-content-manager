package collectionworker

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	contract "nas-content-manager/internal/collectioncontract"
)

// jmcomicVersion 是经过源码与依赖协议核验的唯一兼容版本。
const jmcomicVersion = "2.7.12"

// jmcomicBridge 是随二进制分发的固定桥接，不允许请求选择脚本。
//
//go:embed jmcomic_bridge.py
var jmcomicBridge string

// jmcomicInput 是仅由执行端生成的一次性Python调用，不对浏览器开放。
type jmcomicInput struct {
	Action    string `json:"action"`              // 固定probe/search/album/download动作。
	Query     string `json:"query,omitempty"`     // 已验证关键词。
	Page      int    `json:"page,omitempty"`      // 已验证页码。
	AlbumID   string `json:"albumId,omitempty"`   // 已验证漫画编号。
	OutputDir string `json:"outputDir,omitempty"` // 本操作独占暂存绝对目录。
	MaxBytes  int64  `json:"maxBytes,omitempty"`  // 实际字节预算。
	MaxFiles  int    `json:"maxFiles,omitempty"`  // 实际图片数预算。
	Proxy     string `json:"proxy,omitempty"`     // 执行端自行生成的受控公网代理。
}

// jmcomicAlbum 保存受审查的漫画身份与完整性元信息。
type jmcomicAlbum struct {
	AlbumID   string   `json:"albumId"`   // 上游规范漫画编号。
	Title     string   `json:"title"`     // 非活动纯文本标题。
	PageCount int      `json:"pageCount"` // 全部章节声明的图片数。
	Chapters  []string `json:"chapters"`  // 固定顺序章节编号，用于预览版本绑定。
	Files     int      `json:"files"`     // 下载完成时实际图片数量。
}

// initJMComic 只在显式启用时探测固定依赖，失败仅关闭本插件。
func (s *Service) initJMComic() {
	if !s.config.JMComicEnabled {
		return
	}
	s.jmcomicPython = toolPath(s.config.JMComicPython)
	if s.jmcomicPython == "" {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	var probe struct {
		Version         string `json:"version"`         /* 上游依赖版本。 */
		ProtocolVersion int    `json:"protocolVersion"` /* 固定桥接协议。 */
	}
	if s.callJMComic(ctx, jmcomicInput{Action: "probe"}, "", &probe) != nil || probe.Version != jmcomicVersion || probe.ProtocolVersion != 1 {
		s.jmcomicPython = ""
		s.logger.Warn("漫画插件依赖探测未通过，原有资源收集保持可用")
		return
	}
	s.logger.Info("漫画搜索下载插件已启用", "version", jmcomicVersion)
}

// jmcomicCapability 返回真实依赖状态，不把探测等同于站点可访问。
func (s *Service) jmcomicCapability() contract.Capability {
	c := contract.Capability{Kind: "jmcomic", Available: s.jmcomicPython != ""}
	if !s.config.JMComicEnabled {
		c.Reason = "漫画插件未启用，请配置NAS_COLLECTION_WORKER_JMCOMIC_ENABLED=true"
	} else if !c.Available {
		c.Reason = "漫画插件需要受信Python及锁定的jmcomic 2.7.12依赖，请检查执行端配置"
	} else {
		c.Version = jmcomicVersion
	}
	return c
}

// validJMComicAlbumID 复用两端一致的编号白名单。
func validJMComicAlbumID(value string) bool { return contract.ValidJMComicAlbumID(value) }

// callJMComic 以固定解释器和脚本运行一次有界JSON协议，不调用shell。
func (s *Service) callJMComic(ctx context.Context, input jmcomicInput, dir string, output any) error {
	if s.jmcomicPython == "" {
		return errors.New("漫画插件未启用或固定依赖不可用")
	}
	if input.Action != "probe" {
		input.Proxy = s.proxy.url
	}
	data, err := json.Marshal(input)
	if err != nil {
		return errors.New("漫画插件请求编码失败")
	}
	if dir == "" {
		dir = filepath.Join(s.config.DataDir, "private-state")
	}
	body, err := s.runTool(ctx, s.jmcomicPython, []string{"-I", "-B", "-c", jmcomicBridge}, dir, bytes.NewReader(data), 256<<10, input.Action != "probe")
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(output) != nil {
		return errors.New("漫画插件响应格式无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("漫画插件响应包含多余内容")
	}
	return nil
}

// jmcomicSearchHTTP 在共享有界并发和超时内执行只读漫画搜索。
func (s *Service) jmcomicSearchHTTP(w http.ResponseWriter, r *http.Request) {
	var input contract.JMComicSearchRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := contract.NormalizeJMComicSearch(&input); err != nil {
		failJSON(w, 400, err.Error())
		return
	}
	if s.jmcomicPython == "" {
		failJSON(w, 503, s.jmcomicCapability().Reason)
		return
	}
	select {
	case s.webSlots <- struct{}{}:
		defer func() { <-s.webSlots }()
	default:
		failJSON(w, 429, "漫画搜索正在繁忙，请稍后重试")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	stop := context.AfterFunc(s.ctx, cancel)
	defer func() { stop(); cancel() }()
	var output contract.JMComicSearchResponse
	if err := s.callJMComic(ctx, jmcomicInput{Action: "search", Query: input.Query, Page: input.Page}, "", &output); err != nil {
		s.logger.Warn("漫画搜索未完成，请检查网络与来源")
		failJSON(w, 502, safeExecutionError(err))
		return
	}
	if output.Page != input.Page || output.Query != input.Query || output.Total < 0 || output.Total > 100000000 || len(output.Items) > 100 {
		failJSON(w, 502, "漫画搜索响应超过允许范围")
		return
	}
	if output.Items == nil {
		output.Items = []contract.JMComicSearchItem{}
	}
	for i := range output.Items {
		item := &output.Items[i]
		if !validJMComicAlbumID(item.AlbumID) || len(item.Tags) > 20 {
			failJSON(w, 502, "漫画搜索返回无效编号或标签")
			return
		}
		item.Title = safeTitle(item.Title)
		if item.Tags == nil {
			item.Tags = []string{}
		}
		for j, tag := range item.Tags {
			item.Tags[j] = safeTitle(tag)
		}
	}
	s.logger.Info("漫画搜索已完成", "page", input.Page, "count", len(output.Items))
	writeJSON(w, 200, output)
}

// previewJMComic 只读取漫画元信息并绑定完整章节身份，不下载图片。
func (s *Service) previewJMComic(ctx context.Context, input contract.PreviewRequest) (contract.Preview, error) {
	result := contract.Preview{Files: []contract.File{}, Formats: []contract.Format{}}
	var album jmcomicAlbum
	if err := s.callJMComic(ctx, jmcomicInput{Action: "album", AlbumID: input.AlbumID}, "", &album); err != nil {
		return result, err
	}
	if album.AlbumID != input.AlbumID || album.Files != 0 || album.PageCount < 1 || album.PageCount > input.MaxFiles || len(album.Chapters) < 1 || len(album.Chapters) > input.MaxFiles {
		return result, errors.New("漫画编号、页数或章节不符，请使用完整漫画编号且检查预算")
	}
	seen := map[string]bool{}
	for _, id := range album.Chapters {
		if !validJMComicAlbumID(id) || seen[id] {
			return result, errors.New("漫画章节编号无效")
		}
		seen[id] = true
	}
	result.Title = safeTitle(album.Title)
	album.Title = ""
	result.Identity = digestJSON(album)
	return result, nil
}

// fetchJMComic 只在当前操作暂存中获取全部图片，完整性失败不允许发布。
func (s *Service) fetchJMComic(ctx context.Context, input contract.OperationRequest, target string, expectedIdentity string) error {
	var album jmcomicAlbum
	if err := s.callJMComic(ctx, jmcomicInput{Action: "download", AlbumID: input.AlbumID, OutputDir: target, MaxBytes: input.MaxBytes, MaxFiles: input.MaxFiles}, target, &album); err != nil {
		return err
	}
	if album.Files < 1 || album.Files > input.MaxFiles || album.Files != album.PageCount {
		return errors.New("漫画下载不完整或超过图片数量限制")
	}
	identity := album
	identity.Title = ""
	identity.Files = 0
	if album.AlbumID != input.AlbumID || digestJSON(identity) != expectedIdentity {
		return errors.New("漫画下载期间章节或图片数发生变化，拒绝发布")
	}
	files, err := inspectFiles(ctx, target, input.MaxBytes, input.MaxFiles)
	if err != nil {
		return err
	}
	if len(files) != album.Files {
		return errors.New("漫画产物清单与下载结果不一致")
	}
	for _, file := range files {
		switch strings.ToLower(filepath.Ext(file.Path)) {
		case ".jpg", ".jpeg", ".png", ".gif":
		default:
			return errors.New("漫画下载包含非图片产物")
		}
		if err := verifyJMComicImage(filepath.Join(target, filepath.FromSlash(file.Path))); err != nil {
			return err
		}
	}
	s.logger.Info("漫画全部图片已取得，等待独立发布校验", "operation", input.ID, "files", len(files))
	return nil
}

// verifyJMComicImage 独立解析真实图片头和尺寸，不能仅信任Python报告或文件后缀。
func verifyJMComicImage(name string) error {
	file, err := os.Open(name)
	if err != nil {
		return errors.New("漫画图片无法读取")
	}
	defer file.Close()
	config, format, err := image.DecodeConfig(io.LimitReader(file, 1<<20))
	if err != nil || (format != "jpeg" && format != "png" && format != "gif") || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 40000000 {
		return errors.New("漫画图片格式或尺寸无效")
	}
	return nil
}
