package collectionworker

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	contract "nas-content-manager/internal/collectioncontract"
)

// limitedOutput 保留有界工具输出；超限后返回错误，不允许全量JSON撑满内存。
type limitedOutput struct {
	data  bytes.Buffer
	limit int
} // data为已捕获字节，limit为字节上限。

// Write 拒绝超限工具输出，不回显原始stderr。
func (b *limitedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.data.Len() {
		return 0, errors.New("工具响应超过大小限制")
	}
	return b.data.Write(p)
}

// regularPrivateConfig 只允许受限普通配置文件，链接和开放权限不启用远程能力。
func regularPrivateConfig(name string) bool {
	if name == "" {
		return false
	}
	info, err := os.Lstat(name)
	return err == nil && info.Mode().IsRegular() && info.Size() <= 1<<20 && (runtime.GOOS == "windows" || info.Mode().Perm()&0077 == 0)
}

// version 用短超时探测固定工具，版本字符串仅保留安全首行。
func (s *Service) version(executable string, args ...string) string {
	if executable == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	data, err := s.runTool(ctx, executable, args, "", nil, 4096, false)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	if len(text) > 120 {
		return ""
	}
	for _, c := range text {
		if c < 32 || c > 126 {
			return ""
		}
	}
	return text
}

// toolEnvironment 清除用户级工具配置和远程秘密，私密链接通过标准输入传递。
func (s *Service) toolEnvironment(web bool) []string {
	values := []string{}
	for _, name := range []string{"SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "PATH", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value := os.Getenv(name); value != "" {
			if name == "PATH" {
				parts := []string{}
				for _, dir := range filepath.SplitList(value) {
					if filepath.IsAbs(dir) {
						parts = append(parts, dir)
					}
				}
				value = strings.Join(parts, string(os.PathListSeparator))
			}
			values = append(values, name+"="+value)
		}
	}
	tmp := filepath.Join(s.config.DataDir, "private-state", "tool-temp")
	_ = os.MkdirAll(tmp, 0700)
	values = append(values, "HOME="+tmp, "USERPROFILE="+tmp, "APPDATA="+tmp, "LOCALAPPDATA="+tmp, "TMP="+tmp, "TEMP="+tmp, "TMPDIR="+tmp, "XDG_CACHE_HOME="+tmp, "XDG_CONFIG_HOME="+tmp, "PYTHONNOUSERSITE=1", "PYTHONDONTWRITEBYTECODE=1", "PYTHONIOENCODING=utf-8", "RCLONE_CONFIG="+s.config.RcloneConfigFile)
	if web {
		// FTP和WebSocket也不能回退为直连；代理会明确拒绝这些非HTTP业务协议。
		for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "FTP_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "ftp_proxy", "all_proxy", "ws_proxy", "wss_proxy"} {
			values = append(values, name+"="+s.proxy.url)
		}
		values = append(values, "NO_PROXY=", "no_proxy=")
	}
	return values
}

// runTool 仅启动预先解析的固定工具，不经过shell，不保存或回显工具错误原文。
func (s *Service) runTool(ctx context.Context, executable string, args []string, dir string, input io.Reader, limit int, web bool) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = dir
	command.Env = s.toolEnvironment(web)
	command.Stdin = input
	command.WaitDelay = 3 * time.Second
	configureWorkerProcess(command)
	stdout := &limitedOutput{limit: limit}
	stderr := &limitedOutput{limit: 64 << 10}
	command.Stdout = stdout
	command.Stderr = stderr
	runErr, cleanupErr := runWorkerProcess(command)
	if cleanupErr != nil {
		return nil, errors.New("执行进程未能完整退出")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if runErr != nil {
		return nil, errors.New("获取工具执行失败，来源可能不支持、需要认证或网络不可用")
	}
	return stdout.data.Bytes(), nil
}

// validateRequest 规范化固定业务输入并确认来源预算，不执行下载或复制。
func (s *Service) validateRequest(input *contract.PreviewRequest) error {
	if input.Kind != "jmcomic" && input.AlbumID != "" {
		return errors.New("漫画编号只能用于JMComic插件")
	}
	if input.MaxBytes == 0 {
		input.MaxBytes = s.config.MaxBytes
	}
	if input.MaxFiles == 0 {
		input.MaxFiles = s.config.MaxFiles
	}
	if input.MaxBytes < 1 || input.MaxBytes > s.config.MaxBytes || input.MaxFiles < 1 || input.MaxFiles > s.config.MaxFiles {
		return errors.New("获取预算超过执行端限制")
	}
	switch input.Kind {
	case "jmcomic":
		if !validJMComicAlbumID(input.AlbumID) || input.URL != "" || input.SourceID != "" || input.Path != "" || (input.Profile != "" && input.Profile != "comic") {
			return errors.New("漫画获取仅接受正整数漫画编号和漫画输出")
		}
		input.Profile = "comic"
	case "web":
		if input.SourceID != "" || input.Path != "" {
			return errors.New("网页获取不能携带文件来源参数")
		}
		if input.Profile == "" {
			input.Profile = "video"
		}
		if input.Profile != "video" && input.Profile != "audio" {
			return errors.New("网页输出仅支持视频或音频")
		}
		u, err := url.Parse(input.URL)
		if err != nil || len(input.URL) > 8192 || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Fragment != "" || strings.ContainsAny(input.URL, "\r\n\x00") {
			return errors.New("网页链接必须是无凭据的HTTP(S)单条目链接")
		}
		if _, _, err = publicAddress(u.Host, map[bool]string{true: "443", false: "80"}[u.Scheme == "https"]); err != nil {
			return errors.New("网页目标地址或端口不允许")
		}
	case "local", "remote":
		if input.URL != "" || input.Profile != "" {
			return errors.New("文件复制不能携带网页参数")
		}
		if _, ok := s.source(input.Kind, input.SourceID); !ok {
			return errors.New("文件来源未获执行端授权")
		}
		if !safeRelative(input.Path, true) {
			return errors.New("来源相对路径不安全")
		}
		if input.Path == "." {
			input.Path = ""
		}
	default:
		return errors.New("获取类型不受支持")
	}
	return nil
}

// previewHTTP 获取有时效的实际来源预览，任务必须绑定同一输入和预算。
func (s *Service) previewHTTP(w http.ResponseWriter, r *http.Request) {
	var input contract.PreviewRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := s.validateRequest(&input); err != nil {
		failJSON(w, 400, err.Error())
		return
	}
	select {
	case s.webSlots <- struct{}{}:
		defer func() { <-s.webSlots }()
	default:
		failJSON(w, 429, "预览正在繁忙，请稍后重试")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	stop := context.AfterFunc(s.ctx, cancel)
	defer func() { stop(); cancel() }()
	preview, err := s.preview(ctx, input)
	if err != nil {
		failJSON(w, 422, safeExecutionError(err))
		return
	}
	preview.ID = rand.Text()
	until := time.Now().UTC().Add(5 * time.Minute)
	preview.ExpiresAt = until.Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.previews {
		if time.Now().After(p.Until) {
			delete(s.previews, id)
		}
	}
	if len(s.previews) >= 256 {
		failJSON(w, 429, "有效预览数量已达上限")
		return
	}
	s.previews[preview.ID] = savedPreview{Request: input, Response: preview, Until: until}
	writeJSON(w, 200, preview)
}

// preview 根据固定来源类型探测元数据，不自动获取远程文件。
func (s *Service) preview(ctx context.Context, input contract.PreviewRequest) (contract.Preview, error) {
	result := contract.Preview{Files: []contract.File{}, Formats: []contract.Format{}}
	if input.Kind == "jmcomic" {
		return s.previewJMComic(ctx, input)
	}
	if input.Kind == "web" {
		return s.previewWeb(ctx, input)
	}
	src, _ := s.source(input.Kind, input.SourceID)
	var err error
	if input.Kind == "local" {
		result.Files, err = localFiles(ctx, src, input.Path, input.MaxBytes, input.MaxFiles)
	} else {
		if s.rclone == "" || !regularPrivateConfig(s.config.RcloneConfigFile) {
			return result, errors.New("未配置可运行的rclone及其专用配置")
		}
		result.Files, err = s.remoteFiles(ctx, src, input.Path, input.MaxBytes, input.MaxFiles)
	}
	if err != nil {
		return result, err
	}
	result.Title = src.Name
	if input.Path != "" {
		result.Title = path.Base(input.Path)
	}
	for _, f := range result.Files {
		result.TotalBytes += f.Size
	}
	result.Identity = fileIdentity(src, input.Path, result.Files)
	return result, nil
}

// webArguments 固定关闭用户配置、插件、远程组件和外部下载器，只允许原生HTTP下载。
func (s *Service) webArguments() []string {
	return []string{"--ignore-config", "--no-plugin-dirs", "--no-js-runtimes", "--no-remote-components", "--no-playlist", "--playlist-items", "1", "--no-cache-dir", "--no-warnings", "--no-progress", "--socket-timeout", "20", "--retries", "1", "--fragment-retries", "1", "--extractor-retries", "1", "--abort-on-unavailable-fragments", "--downloader", "native", "--batch-file", "-"}
}

// webFormat 仅接受不会回退为ffmpeg联网下载的原生HTTP和DASH格式。
func webFormat(profile string) string {
	filter := "[protocol~='^(https?|http_dash_segments)$']"
	if profile == "audio" {
		return "bestaudio" + filter + "/best" + filter
	}
	return "bestvideo" + filter + "+bestaudio" + filter + "/best" + filter
}

// webMetadata 只解析白名单网页元数据，原始地址不会进入公开预览。
type webMetadata struct {
	ID        string `json:"id"`              // 提取器内容身份。
	Extractor string `json:"extractor_key"`   // 固定提取器名称。
	Title     string `json:"title"`           // 来源展示标题。
	Type      string `json:"_type"`           // 只接受单一媒体条目。
	Size      int64  `json:"filesize"`        // 可用时的准确大小。
	Approx    int64  `json:"filesize_approx"` // 仅供预览的估算大小。
}

// previewWeb 使用固定工具通过公共代理提取单一条目，无登录或播放列表递归。
func (s *Service) previewWeb(ctx context.Context, input contract.PreviewRequest) (contract.Preview, error) {
	result := contract.Preview{Files: []contract.File{}, Formats: []contract.Format{}}
	if s.ytdlp == "" || s.ffmpeg == "" {
		return result, errors.New("未配置可运行的yt-dlp或ffmpeg")
	}
	args := append(s.webArguments(), "--format", webFormat(input.Profile), "--skip-download", "--dump-single-json")
	data, err := s.runTool(ctx, s.ytdlp, args, "", strings.NewReader(input.URL+"\n"), 2<<20, true)
	if err != nil {
		return result, err
	}
	var meta webMetadata
	if json.Unmarshal(data, &meta) != nil || meta.ID == "" || meta.Type != "" && meta.Type != "video" {
		return result, errors.New("网页不是受支持的单条目音视频")
	}
	result.Title = safeTitle(meta.Title)
	result.Identity = digestJSON([]string{meta.Extractor, meta.ID, input.Profile})
	result.TotalBytes = meta.Size
	if result.TotalBytes == 0 {
		result.TotalBytes = meta.Approx
	}
	if result.TotalBytes > input.MaxBytes {
		return result, errors.New("网页媒体大小超过请求预算")
	}
	result.Formats = []contract.Format{{ID: "video", Label: "保存视频", Extension: ""}, {ID: "audio", Label: "提取音频（MP3）", Extension: "mp3"}}
	return result, nil
}

// remoteEntry 是rclone有界清单的受审查字段，不公开远程对象URL。
type remoteEntry struct {
	Path     string    `json:"Path"`     // 相对指定路径的对象名。
	Name     string    `json:"Name"`     // 单文件结果名称。
	Size     int64     `json:"Size"`     // 对象字节数。
	ModTime  time.Time `json:"ModTime"`  // 远程版本时间。
	IsDir    bool      `json:"IsDir"`    // 目录标志。
	MimeType string    `json:"MimeType"` // 用于拒绝链接对象。
}

// remoteName 将经过验证的相对路径追加到已授权具名remote前缀。
func remoteName(src SourceConfig, selected string) string {
	if selected == "" {
		return src.Path
	}
	if strings.HasSuffix(src.Path, ":") {
		return src.Path + selected
	}
	return strings.TrimRight(src.Path, "/") + "/" + selected
}

// remoteFiles 从固定remote读取有界清单，不能提交原始连接字符串或额外flags。
func (s *Service) remoteFiles(ctx context.Context, src SourceConfig, selected string, maxBytes int64, maxFiles int) ([]contract.File, error) {
	args := []string{"lsjson", remoteName(src, selected), "--recursive", "--files-only", "--no-mimetype", "--max-depth", "32", "--retries", "1", "--low-level-retries", "1", "--contimeout", "15s", "--timeout", "30s", "--log-level", "ERROR"}
	data, err := s.runTool(ctx, s.rclone, args, "", nil, 2<<20, false)
	if err != nil {
		return nil, err
	}
	var entries []remoteEntry
	if json.Unmarshal(data, &entries) != nil {
		return nil, errors.New("远程文件清单无效或超过限制")
	}
	if len(entries) > 10000 {
		return nil, errors.New("远程扫描条目超过限制")
	}
	files := []contract.File{}
	var bytes int64
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		name := e.Path
		if name == "" {
			name = e.Name
		}
		if !safeRelative(name, false) {
			return nil, errors.New("远程对象路径不安全")
		}
		if !supportedMedia(name) {
			continue
		}
		if seen[name] || e.Size <= 0 || e.Size > maxBytes-bytes || len(files) >= maxFiles || strings.Contains(e.MimeType, "symlink") {
			return nil, errors.New("远程文件版本、数量或大小超出限制")
		}
		seen[name] = true
		bytes += e.Size
		files = append(files, contract.File{Path: name, Size: e.Size, ModTime: e.ModTime.UnixNano()})
	}
	if len(files) == 0 {
		return nil, errors.New("没有可导入的受支持媒体文件")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// acquire 为新意图建立独立隔离目录；重启后不会对不确定操作重新执行。
func (s *Service) acquire(ctx context.Context, saved savedOperation) (files []contract.File, resultErr error) {
	input := saved.Request
	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, 45*time.Minute)
	defer deadlineCancel()
	ctx, cancelCause := context.WithCancelCause(deadlineCtx)
	defer cancelCause(nil)
	if err := s.reserveSpace(input.MaxBytes); err != nil {
		return nil, err
	}
	target := filepath.Join(s.config.DataDir, "staging", input.ID)
	if err := os.Mkdir(target, 0700); err != nil {
		return nil, errors.New("暂存目录已存在或无法创建，需要核对旧结果")
	}
	// 正在写入的文件也计入预算，超过大小、数量或磁盘余量立即取消自有执行。
	var monitor sync.WaitGroup
	monitor.Add(1)
	monitorDone := make(chan struct{})
	defer func() {
		close(monitorDone)
		monitor.Wait()
		cause := context.Cause(ctx)
		if cause != nil && !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
			files = nil
			resultErr = cause
		}
	}()
	go func() {
		defer monitor.Done()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-monitorDone:
				return
			case <-ticker.C:
				if err := s.checkWriteBudget(target, input.MaxBytes, input.MaxFiles); err != nil {
					cancelCause(err)
					return
				}
			}
		}
	}()
	switch input.Kind {
	case "jmcomic":
		current, err := s.previewJMComic(ctx, requestPreview(input))
		if err != nil {
			return nil, err
		}
		if saved.Preview.Identity != current.Identity {
			return nil, errors.New("漫画章节或页数已改变，请重新预览")
		}
		if err = s.fetchJMComic(ctx, input, target, saved.Preview.Identity); err != nil {
			return nil, err
		}
	case "local":
		src, _ := s.source(input.Kind, input.SourceID)
		current, err := localFiles(ctx, src, input.Path, input.MaxBytes, input.MaxFiles)
		if err != nil {
			return nil, err
		}
		if !filesSameSnapshot(saved.Preview.Files, current) || saved.Preview.Identity != fileIdentity(src, input.Path, current) {
			return nil, errors.New("来源版本已改变，请重新预览")
		}
		return s.copyLocal(ctx, input, target, current)
	case "remote":
		src, _ := s.source(input.Kind, input.SourceID)
		current, err := s.remoteFiles(ctx, src, input.Path, input.MaxBytes, input.MaxFiles)
		if err != nil {
			return nil, err
		}
		if !filesSameSnapshot(saved.Preview.Files, current) || saved.Preview.Identity != fileIdentity(src, input.Path, current) {
			return nil, errors.New("来源版本已改变，请重新预览")
		}
		if err = s.copyRemote(ctx, src, input, current, target); err != nil {
			return nil, err
		}
	case "web":
		current, err := s.previewWeb(ctx, requestPreview(input))
		if err != nil {
			return nil, err
		}
		if saved.Preview.Identity != current.Identity {
			return nil, errors.New("网页条目身份已改变，请重新预览")
		}
		if err = s.fetchWeb(ctx, input, target); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("获取类型不受支持")
	}
	return inspectFiles(ctx, target, input.MaxBytes, input.MaxFiles)
}

// fetchWeb 固定输出单条视频或音频，源地址走标准输入，后处理禁止网络协议。
func (s *Service) fetchWeb(ctx context.Context, input contract.OperationRequest, target string) error {
	args := append(s.webArguments(), "--format", webFormat(input.Profile), "--output", "media.%(ext)s", "--restrict-filenames", "--no-overwrites", "--max-filesize", fmt.Sprint(input.MaxBytes), "--ffmpeg-location", s.ffmpeg, "--postprocessor-args", "ffmpeg_i:-protocol_whitelist file,pipe,crypto,data", "--no-write-info-json", "--no-write-thumbnail", "--no-write-subs")
	if input.Profile == "audio" {
		args = append(args, "--extract-audio", "--audio-format", "mp3")
	}
	_, err := s.runTool(ctx, s.ytdlp, args, target, strings.NewReader(input.URL+"\n"), 64<<10, true)
	return err
}

// copyRemote 仅复制清单中的媒体对象；rclone配置通过受限环境传递，不删除来源。
func (s *Service) copyRemote(ctx context.Context, src SourceConfig, input contract.OperationRequest, files []contract.File, target string) error {
	list, err := os.CreateTemp(filepath.Join(s.config.DataDir, "private-state"), "filelist-")
	if err != nil {
		return errors.New("无法建立远程复制清单")
	}
	name := list.Name()
	defer os.Remove(name)
	_ = list.Chmod(0600)
	for _, f := range files {
		if _, err = list.WriteString(f.Path + "\n"); err != nil {
			list.Close()
			return errors.New("无法保存远程复制清单")
		}
	}
	if list.Close() != nil {
		return errors.New("无法保存远程复制清单")
	}
	args := []string{"copy", remoteName(src, input.Path), target, "--files-from-raw", name, "--immutable", "--max-transfer", fmt.Sprint(input.MaxBytes), "--cutoff-mode", "cautious", "--transfers", "2", "--checkers", "2", "--retries", "1", "--low-level-retries", "1", "--contimeout", "15s", "--timeout", "30s", "--log-level", "ERROR", "--no-traverse"}
	_, err = s.runTool(ctx, s.rclone, args, target, nil, 64<<10, false)
	if err != nil {
		return err
	}
	after, err := s.remoteFiles(ctx, src, input.Path, input.MaxBytes, input.MaxFiles)
	if err != nil || !filesSameSnapshot(files, after) {
		return errors.New("远程来源在复制期间发生变化，拒绝发布")
	}
	actual, err := inspectFiles(ctx, target, input.MaxBytes, input.MaxFiles)
	if err != nil {
		return err
	}
	if len(actual) != len(files) {
		return errors.New("远程复制结果缺少清单文件")
	}
	expected := map[string]int64{}
	for _, f := range files {
		expected[f.Path] = f.Size
	}
	for _, f := range actual {
		if n, ok := expected[f.Path]; !ok || n != f.Size {
			return errors.New("远程复制结果与来源清单不一致")
		}
	}
	return nil
}

// checkWriteBudget 监测暂存中的全部文件和空间，包含下载器尚未完成的临时文件。
func (s *Service) checkWriteBudget(dir string, maxBytes int64, maxFiles int) error {
	var bytes int64
	count := 0
	err := filepath.WalkDir(dir, func(name string, e os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		count++
		if count > maxFiles*4+100 {
			return errors.New("暂存文件数量超过限制")
		}
		if e.IsDir() {
			return nil
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("暂存包含不安全文件")
		}
		bytes += info.Size()
		if bytes > maxBytes {
			return errors.New("暂存大小超过限制")
		}
		return nil
	})
	if err != nil {
		return err
	}
	free, err := freeDiskBytes(s.config.DataDir)
	if err != nil || free < uint64(s.config.MinFreeMB)*(1<<20) {
		return errors.New("暂存卷低于保留空间")
	}
	return nil
}

// digestJSON 用不可逆摘要保存非公开来源身份和版本。
func digestJSON(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fileIdentity 将授权根绑定与来源版本一起摘要，重启后换根不能复用旧预览。
func fileIdentity(src SourceConfig, selected string, files []contract.File) string {
	return digestJSON(struct {
		Source string          // 已授权来源标识。
		Root   string          // 不公开的来源绑定，用摘要识别部署换根。
		Path   string          // 选择的相对路径。
		Files  []contract.File // 观测的文件版本清单。
	}{src.ID, src.Path, selected, files})
}

// safeTitle 过滤控制字符并限制标题大小，避免错误文本和活动内容污染展示。
func safeTitle(value string) string {
	if strings.Contains(value, "://") {
		return "网页媒体"
	}
	value = strings.Map(func(c rune) rune {
		if c < 32 || c == 127 {
			return -1
		}
		return c
	}, value)
	runes := []rune(value)
	if len(runes) > 240 {
		value = string(runes[:240])
	}
	if value == "" {
		return "网页媒体"
	}
	return value
}

// safeExecutionError 只返回执行器自有中文错误，禁止泄露操作系统和工具原始消息。
func safeExecutionError(err error) string {
	if errors.Is(err, context.Canceled) {
		return "获取已取消或执行预算达到限制"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "获取或预览超时，可核对结果后重试"
	}
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 240 || strings.ContainsAny(text, "\r\n/\\") || strings.Contains(text, "://") {
		return "获取失败，请检查来源、预算和工具配置"
	}
	for _, c := range text {
		if c > 127 {
			return text
		}
	}
	return "获取失败，请检查来源、预算和工具配置"
}
