package collectionworker

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	Action        string   `json:"action"`                  // 固定probe/search/album/download动作。
	Query         string   `json:"query,omitempty"`         // 已验证关键词。
	Page          int      `json:"page,omitempty"`          // 已验证页码。
	AlbumID       string   `json:"albumId,omitempty"`       // 已验证漫画编号。
	OutputDir     string   `json:"outputDir,omitempty"`     // 本操作独占暂存绝对目录。
	MaxBytes      int64    `json:"maxBytes,omitempty"`      // 实际字节预算。
	MaxFiles      int      `json:"maxFiles,omitempty"`      // 实际图片数预算。
	Proxy         string   `json:"proxy,omitempty"`         // 执行端自行生成的受控公网代理。
	OrderBy       string   `json:"orderBy,omitempty"`       // 固定搜索排序。
	Time          string   `json:"time,omitempty"`          // 固定搜索时间范围。
	ChapterIDs    []string `json:"chapterIds,omitempty"`    // 显式选章，空表示全本。
	CheckpointDir string   `json:"checkpointDir,omitempty"` // 本操作私密检查点目录，永不发布。
	Binding       string   `json:"binding,omitempty"`       // 请求与预览联合绑定摘要。
}

// jmcomicAlbum 保存受审查的漫画身份与完整性元信息。
type jmcomicAlbum struct {
	AlbumID            string                    `json:"albumId"`            // 上游规范漫画编号。
	Title              string                    `json:"title"`              // 非活动纯文本标题。
	PageCount          int                       `json:"pageCount"`          // 全部章节声明的图片数。
	Chapters           []string                  `json:"chapters"`           // 固定顺序章节编号，用于预览版本绑定。
	Files              int                       `json:"files"`              // 下载完成时实际图片数量。
	ChapterDetails     []contract.JMComicChapter `json:"chapterDetails"`     // 全本有序章节与页数。
	SelectedChapterIDs []string                  `json:"selectedChapterIds"` // 上游顺序规范化后的实际选章。
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
	if err := s.callJMComic(ctx, jmcomicInput{Action: "search", Query: input.Query, Page: input.Page, OrderBy: input.OrderBy, Time: input.Time}, "", &output); err != nil {
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
	if err := s.callJMComic(ctx, jmcomicInput{Action: "album", AlbumID: input.AlbumID, ChapterIDs: input.ChapterIDs}, "", &album); err != nil {
		return result, err
	}
	if album.AlbumID != input.AlbumID || album.Files != 0 {
		return result, errors.New("漫画编号、页数或章节不符，请使用完整漫画编号且检查预算")
	}
	metadata, err := jmcomicMetadata(album, input.ChapterIDs, input.MaxFiles)
	if err != nil {
		return result, err
	}
	result.Title = safeTitle(album.Title)
	result.JMComic = metadata
	result.Identity = jmcomicIdentity(album, input.ChapterIDs)
	return result, nil
}

// jmcomicIdentity 保持旧全本身份字节顺序，仅显式选章时增加选择绑定。
func jmcomicIdentity(album jmcomicAlbum, selection []string) string {
	legacy := struct {
		AlbumID   string   `json:"albumId"`   // 漫画编号。
		Title     string   `json:"title"`     // 旧协议固定清空展示标题。
		PageCount int      `json:"pageCount"` // 全本页数。
		Chapters  []string `json:"chapters"`  // 有序章节编号。
		Files     int      `json:"files"`     // 旧预览固定零。
	}{AlbumID: album.AlbumID, PageCount: album.PageCount, Chapters: album.Chapters}
	if len(selection) == 0 {
		return digestJSON(legacy)
	}
	chapters := []contract.JMComicChapter{}
	for _, chapter := range album.ChapterDetails {
		chapter.Title = ""
		chapters = append(chapters, chapter)
	}
	return digestJSON(struct {
		Legacy    any                       `json:"album"`              // 保持全本身份。
		Chapters  []contract.JMComicChapter `json:"chapters"`           // 绑定各章完整页数。
		Selection []string                  `json:"selectedChapterIds"` // 绑定上游规范顺序选择。
	}{legacy, chapters, album.SelectedChapterIDs})
}

// jmcomicMetadata 独立核验全本目录、唯一选择与页数，超操作预算仍允许只读目录。
func jmcomicMetadata(album jmcomicAlbum, selection []string, maxFiles int) (*contract.JMComicMetadata, error) {
	if album.PageCount < 1 || album.PageCount > 10000 || len(album.Chapters) < 1 || len(album.Chapters) > 10000 || len(album.ChapterDetails) != len(album.Chapters) {
		return nil, errors.New("漫画目录或页数超过绝对限制")
	}
	if err := contract.ValidateJMComicChapters(selection); err != nil {
		return nil, err
	}
	wanted := make(map[string]bool, len(selection))
	for _, id := range selection {
		wanted[id] = true
	}
	metadata := &contract.JMComicMetadata{AlbumID: album.AlbumID, Chapters: append([]contract.JMComicChapter{}, album.ChapterDetails...), SelectedChapterIDs: []string{}}
	seen := map[string]bool{}
	total := 0
	for i, id := range album.Chapters {
		chapter := &metadata.Chapters[i]
		if !validJMComicAlbumID(id) || seen[id] || chapter.ID != id || chapter.PageCount < 1 || chapter.PageCount > 10000-total {
			return nil, errors.New("漫画章节编号或实际页数无效")
		}
		seen[id] = true
		total += chapter.PageCount
		chapter.Title = safeTitle(chapter.Title)
		if len(selection) == 0 || wanted[id] {
			metadata.SelectedChapterIDs = append(metadata.SelectedChapterIDs, id)
			metadata.PageCount += chapter.PageCount
		}
	}
	if total != album.PageCount || len(selection) > 0 && len(metadata.SelectedChapterIDs) != len(selection) || !equalStrings(metadata.SelectedChapterIDs, album.SelectedChapterIDs) {
		return nil, errors.New("漫画选择含未知章节或目录页数不一致")
	}
	metadata.OverBudget = metadata.PageCount > maxFiles
	return metadata, nil
}

// equalStrings 比较已规范化章节序列，避免隐式忽略顺序或重复值。
func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// sameJMComicMetadata 对比冻结目录的章节页数和选择，不让旧全本摘要兼容性掩盖目录漂移。
func sameJMComicMetadata(left *contract.JMComicMetadata, right *contract.JMComicMetadata) bool {
	if left == nil || right == nil || left.AlbumID != right.AlbumID || left.PageCount != right.PageCount || left.OverBudget != right.OverBudget || len(left.Chapters) != len(right.Chapters) || !equalStrings(left.SelectedChapterIDs, right.SelectedChapterIDs) {
		return false
	}
	for i, chapter := range left.Chapters {
		if chapter.ID != right.Chapters[i].ID || chapter.PageCount != right.Chapters[i].PageCount {
			return false
		}
	}
	return true
}

// fetchJMComic 只在当前操作暂存中获取全部图片，完整性失败不允许发布。
func (s *Service) fetchJMComic(ctx context.Context, input contract.OperationRequest, target string, expected contract.Preview) error {
	if expected.JMComic == nil || expected.JMComic.OverBudget || expected.JMComic.PageCount > input.MaxFiles {
		return errors.New("漫画选择超过执行页数预算")
	}
	checkpoint, err := s.jmcomicStateDirectory(input.ID)
	if err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(checkpoint, "progress.json")); err != nil && !os.IsNotExist(err) {
		return errors.New("无法重置漫画进度")
	}
	var monitor sync.WaitGroup
	done := make(chan struct{})
	monitor.Add(1)
	go func() {
		defer monitor.Done()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.updateJMComicProgress(input.ID, checkpoint, expected.JMComic.PageCount, input.MaxBytes)
			}
		}
	}()
	defer func() {
		close(done)
		monitor.Wait()
		s.updateJMComicProgress(input.ID, checkpoint, expected.JMComic.PageCount, input.MaxBytes)
	}()
	var album jmcomicAlbum
	if err := s.callJMComic(ctx, jmcomicInput{Action: "download", AlbumID: input.AlbumID, ChapterIDs: input.ChapterIDs, OutputDir: target, MaxBytes: input.MaxBytes, MaxFiles: input.MaxFiles, CheckpointDir: checkpoint, Binding: digestJSON([]string{requestHash(input), expected.Identity})}, target, &album); err != nil {
		return err
	}
	metadata, err := jmcomicMetadata(album, input.ChapterIDs, input.MaxFiles)
	if err != nil {
		return err
	}
	if !sameJMComicMetadata(expected.JMComic, metadata) {
		return errors.New("漫画下载期间章节目录页数发生变化，请重新预览")
	}
	if album.Files < 1 || album.Files > input.MaxFiles || album.Files != metadata.PageCount || metadata.OverBudget {
		return errors.New("漫画下载不完整或超过图片数量限制")
	}
	if album.AlbumID != input.AlbumID || jmcomicIdentity(album, input.ChapterIDs) != expected.Identity {
		return errors.New("漫画下载期间章节或图片数发生变化，拒绝发布")
	}
	files, err := inspectFiles(ctx, target, input.MaxBytes, input.MaxFiles)
	if err != nil {
		return err
	}
	if len(files) != album.Files {
		return errors.New("漫画产物清单与下载结果不一致")
	}
	expectedPages := map[string]bool{}
	selected := map[string]bool{}
	for _, id := range metadata.SelectedChapterIDs {
		selected[id] = true
	}
	for i, chapter := range metadata.Chapters {
		if selected[chapter.ID] {
			for page := 1; page <= chapter.PageCount; page++ {
				expectedPages[fmt.Sprintf("%05d/%05d", i+1, page)] = true
			}
		}
	}
	for _, file := range files {
		switch strings.ToLower(filepath.Ext(file.Path)) {
		case ".jpg", ".gif":
		default:
			return errors.New("漫画下载包含非图片产物")
		}
		key := strings.TrimSuffix(file.Path, filepath.Ext(file.Path))
		if !expectedPages[key] {
			return errors.New("漫画产物包含重复、未选章节或非完整页序")
		}
		delete(expectedPages, key)
		if err := verifyJMComicImage(filepath.Join(target, filepath.FromSlash(file.Path))); err != nil {
			return err
		}
	}
	if len(expectedPages) != 0 {
		return errors.New("漫画选章缺少完整页面")
	}
	s.updateJMComicProgress(input.ID, checkpoint, expected.JMComic.PageCount, input.MaxBytes)
	s.mu.Lock()
	progress := s.ops[input.ID].Operation.JMComicProgress
	s.mu.Unlock()
	reusedPages := 0
	if progress != nil {
		reusedPages = progress.ReusedPages
	}
	s.logger.Info("漫画所选章节已完整取得，等待独立发布校验", "operation", input.ID, "files", len(files), "chapters", len(metadata.SelectedChapterIDs), "reusedPages", reusedPages)
	return nil
}

// jmcomicStateDirectory 建立仅属于本操作的私密检查点目录，拒绝链接和别名。
func (s *Service) jmcomicStateDirectory(id string) (string, error) {
	if !safeOperationID(id) {
		return "", errors.New("漫画检查点操作编号无效")
	}
	root := filepath.Join(s.config.DataDir, "private-state")
	for _, path := range []string{root, filepath.Join(root, "jmcomic"), filepath.Join(root, "jmcomic", id)} {
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return "", errors.New("漫画私密目录无法建立")
		}
		info, err := os.Lstat(path)
		canonical, pathErr := filepath.EvalSymlinks(path)
		if err != nil || pathErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !sameNativePath(path, canonical) {
			return "", errors.New("漫画私密目录含链接或别名")
		}
	}
	return filepath.Join(root, "jmcomic", id), nil
}

// updateJMComicProgress 仅接受固定私密路径中的有界单调快照，不改变最终Bytes语义。
func (s *Service) updateJMComicProgress(id string, directory string, total int, maxBytes int64) {
	name := filepath.Join(directory, "progress.json")
	info, err := os.Lstat(name)
	canonical, pathErr := filepath.EvalSymlinks(name)
	if err != nil || pathErr != nil || !info.Mode().IsRegular() || info.Size() > 4096 || !sameNativePath(name, canonical) {
		return
	}
	body, err := os.ReadFile(name)
	if err != nil {
		return
	}
	var progress contract.JMComicProgress
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&progress) != nil || progress.TotalPages != total || progress.CompletedPages < 0 || progress.CompletedPages > total || progress.Bytes < 0 || progress.Bytes > maxBytes || progress.ReusedPages < 0 || progress.ReusedPages > progress.CompletedPages {
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, ok := s.ops[id]
	if !ok || saved.Operation.Kind != "jmcomic" {
		return
	}
	previous := saved.Operation.JMComicProgress
	if previous != nil && (progress.CompletedPages < previous.CompletedPages || progress.Bytes < previous.Bytes || progress.ReusedPages < previous.ReusedPages) {
		return
	}
	saved.Operation.JMComicProgress = &progress
	s.ops[id] = saved
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
