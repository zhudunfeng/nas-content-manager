package externalapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"
)

// MPChoice 仅公开工作流名称和不透明编号，原始站点上下文保存在私密文件中。
type MPChoice struct {
	ID              string          `json:"id"`              // 本系统生成的短期选择编号。
	Title           string          `json:"title"`           // 脱敏显示名称。
	Kind            string          `json:"kind"`            // 固定选择种类。
	Status          string          `json:"status"`          // 中文状态说明。
	ExternalID      string          `json:"externalId"`      // 已确认的 TMDB 编号。
	MediaType       string          `json:"mediaType"`       // movie 或 series。
	Year            int             `json:"year"`            // 发行年份，未知时为零。
	Season          int             `json:"season"`          // 季度，未知时为零。
	OrganizeAllowed bool            `json:"organizeAllowed"` // 此媒体库目录是否已配置为不覆盖。
	Data            json.RawMessage `json:"-"`               // 绝不返回浏览器的原始上下文。
}

// mpRoot 是经过固定配置接口取得的本地根目录身份。
type mpRoot struct {
	Kind      string `json:"kind"`      // download_root 或 library_root。
	Name      string `json:"name"`      // 远端配置名称，仅内部匹配。
	Path      string `json:"path"`      // 规范化的远端本地绝对路径。
	Overwrite string `json:"overwrite"` // 远端配置的覆盖策略。
}

// mpFile 仅保留本地文件操作必需的已观察字段，不携带下载地址或子树。
type mpFile struct {
	Storage    string          `json:"storage"`                  // 固定为 local。
	Path       string          `json:"path"`                     // 已验证位于配置根内的路径。
	Type       string          `json:"type"`                     // dir 或 file。
	Name       string          `json:"name"`                     // 原文件名称，仅用于上游协议。
	Size       *int64          `json:"size,omitempty"`           // 预览时的字节数。
	ModifyTime *float64        `json:"modify_time,omitempty"`    // 上游修改时间。
	ModifyNS   json.RawMessage `json:"modify_time_ns,omitempty"` // 无损保留的纳秒版本。
}

// mpDownload 通过下载器和散列共同标识单一任务，避免同散列跨下载器误控。
type mpDownload struct {
	Hash       string `json:"hash"`       // 已验证的任务散列。
	Downloader string `json:"downloader"` // 已观察到的下载器实例名称。
	Title      string `json:"title"`      // 脱敏展示名称。
	State      string `json:"state"`      // 上游状态，仅映射已知值。
}

// mpWorkflow 是仅在服务器内部序列化的不可变操作快照。
type mpWorkflow struct {
	Kind      string          `json:"kind"`                // 快照种类。
	Media     json.RawMessage `json:"media,omitempty"`     // 选中的完整媒体上下文。
	Torrent   json.RawMessage `json:"torrent,omitempty"`   // 含站点秘密的选中资源上下文。
	MediaID   string          `json:"mediaId,omitempty"`   // TMDB 身份。
	MediaType string          `json:"mediaType,omitempty"` // 固定影视类型。
	Root      *mpRoot         `json:"root,omitempty"`      // 来源文件所属已配置根。
	File      *mpFile         `json:"file,omitempty"`      // 当前选择的单一文件或目录。
	Target    *mpRoot         `json:"target,omitempty"`    // 整理目标配置根。
	Download  *mpDownload     `json:"download,omitempty"`  // 下载控制的完整身份。
}

// moviePilotWorkflowKind 判断需要私密选择的六阶段写动作。
func moviePilotWorkflowKind(kind string) bool {
	switch kind {
	case "download", "download_start", "download_stop", "organize", "scrape", "refresh_library":
		return true
	}
	return false
}

// mpSnapshot 将有界服务器上下文编码为私密快照。
func mpSnapshot(value mpWorkflow) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil || len(data) > 60<<10 {
		return nil, &Fault{Message: "该资源上下文过大，请在 MoviePilot 中处理", Rejected: true}
	}
	return data, nil
}

// mpDecode 只解码本系统私密文件中的固定快照模型。
func mpDecode(data json.RawMessage, kind string) (mpWorkflow, error) {
	var value mpWorkflow
	if len(data) == 0 || len(data) > 60<<10 || json.Unmarshal(data, &value) != nil || value.Kind != kind {
		return value, &Fault{Message: "私密工作流选择无效，请重新选择", Rejected: true}
	}
	return value, nil
}

// mpMediaIdentity 只认可完整 TMDB 身份，排除别的元数据源或含混类型。
func mpMediaIdentity(data json.RawMessage) (string, string, string, int, int) {
	var media struct {
		Source string `json:"media_source"` // 元数据来源。
		ID     string `json:"media_id"`     // 来源原生编号。
		Type   string `json:"type"`         // 上游影视枚举。
		Title  string `json:"title"`        // 媒体名称。
		Year   string `json:"year"`         // 发行年份。
		Season int    `json:"season"`       // 当前季度。
	}
	if json.Unmarshal(data, &media) != nil {
		return "", "", "", 0, 0
	}
	if media.Source != "themoviedb" {
		return "", "", "", 0, 0
	}
	if !positiveID(media.ID) {
		return "", "", "", 0, 0
	}
	kind := moviePilotType(media.Type)
	if kind == "" {
		return "", "", "", 0, 0
	}
	year, _ := strconv.Atoi(media.Year)
	if year < 1800 || year > 3000 {
		year = 0
	}
	season := media.Season
	if kind == "movie" || season < 0 || season > 999 {
		season = 0
	}
	return media.ID, kind, media.Title, year, season
}

// MPMediaSearch 搜索资料供用户选择，不自动执行站点搜索或创建下载。
func (c *Client) MPMediaSearch(ctx context.Context, query string, mediaType string) ([]MPChoice, error) {
	if err := c.requireCapability("integrated_search"); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if len(query) < 1 || len(query) > 160 || strings.ContainsAny(query, "\r\n\x00") || (mediaType != "movie" && mediaType != "series") {
		return nil, &Fault{Message: "请输入标题并选择电影或剧集", Rejected: true}
	}
	var media []json.RawMessage
	if err := c.callMP(ctx, "GET", c.prefix+"/media/search", url.Values{"title": {query}, "type": {"media"}, "media_source": {"themoviedb"}, "page": {"1"}, "count": {"101"}}, nil, &media); err != nil {
		return nil, err
	}
	if media == nil || len(media) > 100 {
		return nil, &Fault{Message: "资料结果过多或格式无效，请缩小标题范围", Rejected: true}
	}
	out := []MPChoice{}
	for _, raw := range media {
		id, kind, title, year, season := mpMediaIdentity(raw)
		if id == "" || kind != mediaType {
			continue
		}
		data, err := mpSnapshot(mpWorkflow{Kind: "media", Media: raw, MediaID: id, MediaType: kind})
		if err != nil {
			return nil, err
		}
		out = append(out, MPChoice{Title: c.adapterText(title, 180), Kind: "media", Status: "TMDB 资料", ExternalID: id, MediaType: kind, Year: year, Season: season, Data: data})
	}
	return out, nil
}

// MPReleases 基于已选资料调用固定资源搜索，敏感种子上下文只保留在服务器。
func (c *Client) MPReleases(ctx context.Context, data json.RawMessage) ([]MPChoice, error) {
	if err := c.requireCapability("integrated_search"); err != nil {
		return nil, err
	}
	selected, err := mpDecode(data, "media")
	if err != nil {
		return nil, err
	}
	id, kind, _, _, season := mpMediaIdentity(selected.Media)
	if id == "" || id != selected.MediaID || kind != selected.MediaType {
		return nil, &Fault{Message: "媒体资料身份已不可用，请重新搜索", Rejected: true}
	}
	query := url.Values{"media_source": {"themoviedb"}, "mtype": {moviePilotMediaName(kind)}, "page": {"1"}, "count": {"101"}}
	if season > 0 {
		query.Set("season", strconv.Itoa(season))
	}
	var contexts []struct {
		Media   json.RawMessage `json:"media_info"`   // 与结果匹配的媒体资料。
		Torrent json.RawMessage `json:"torrent_info"` // 含秘密站点上下文。
	}
	if err = c.callMP(ctx, "GET", c.prefix+"/search/media/"+id, query, nil, &contexts); err != nil {
		return nil, err
	}
	if contexts == nil || len(contexts) > 100 {
		return nil, &Fault{Message: "资源结果过多或格式无效，请在 MoviePilot 缩小站点范围", Rejected: true}
	}
	out := []MPChoice{}
	for _, result := range contexts {
		rid, rkind, title, year, resultSeason := mpMediaIdentity(result.Media)
		if rid != id || rkind != kind {
			continue
		}
		var torrent struct {
			Title string `json:"title"` /* 种子名称。 */
		}
		if json.Unmarshal(result.Torrent, &torrent) != nil || torrent.Title == "" {
			continue
		}
		payload, snapshotErr := mpSnapshot(mpWorkflow{Kind: "release", Media: result.Media, Torrent: result.Torrent, MediaID: id, MediaType: kind})
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		if torrent.Title != "" {
			title = torrent.Title
		}
		out = append(out, MPChoice{Title: c.adapterText(title, 220), Kind: "release", Status: "确认后由 MoviePilot 下载", ExternalID: id, MediaType: kind, Year: year, Season: resultSeason, Data: payload})
	}
	return out, nil
}

// mpHash 校验下载任务散列，防止路径或复合目标注入。
func mpHash(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

// mpDownloads 读取单页任务，拒绝超出单次控制预算的集合。
func (c *Client) mpDownloads(ctx context.Context) ([]mpDownload, error) {
	var tasks []struct {
		Hash       string `json:"hash"`       // 任务散列。
		Downloader string `json:"downloader"` // 实际下载器。
		Title      string `json:"title"`      // 媒体名称。
		Name       string `json:"name"`       // 下载名称。
		State      string `json:"state"`      // 运行状态。
	}
	if err := c.callMP(ctx, "GET", c.prefix+"/download/", url.Values{"page": {"1"}, "count": {"101"}}, nil, &tasks); err != nil {
		return nil, err
	}
	if tasks == nil || len(tasks) > 100 {
		return nil, &Fault{Message: "下载队列超过 100 项或响应无效，请在 MoviePilot 管理", Rejected: true}
	}
	out := []mpDownload{}
	seen := map[string]bool{}
	for _, task := range tasks {
		if !mpHash(task.Hash) || task.Downloader == "" || len(task.Downloader) > 200 {
			continue
		}
		identity := task.Downloader + ":" + strings.ToLower(task.Hash)
		if seen[identity] {
			return nil, &Fault{Message: "下载任务身份重复，请在 MoviePilot 核查"}
		}
		seen[identity] = true
		title := task.Title
		if title == "" {
			title = task.Name
		}
		out = append(out, mpDownload{Hash: strings.ToLower(task.Hash), Downloader: task.Downloader, Title: c.adapterText(title, 180), State: task.State})
	}
	return out, nil
}

// MPDownloads 为当前任务生成启停选择，不暴露下载器实例或私密路径。
func (c *Client) MPDownloads(ctx context.Context) ([]MPChoice, error) {
	if err := c.requireCapability("downloads"); err != nil {
		return nil, err
	}
	tasks, err := c.mpDownloads(ctx)
	if err != nil {
		return nil, err
	}
	out := []MPChoice{}
	for _, task := range tasks {
		data, snapshotErr := mpSnapshot(mpWorkflow{Kind: "download", Download: &task})
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		out = append(out, MPChoice{Title: task.Title, Kind: "download", Status: adapterStatus(task.State), Data: data})
	}
	return out, nil
}

// mpLocalPath 规范化远端本地路径，拒绝相对路径、UNC、控制字符和穿越片段。
func mpLocalPath(value string) (string, bool) {
	if value == "" || len(value) > 4096 {
		return "", false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", false
		}
	}
	value = strings.ReplaceAll(value, "\\", "/")
	drive := len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && value[2] == '/'
	if strings.HasPrefix(value, "//") || (!strings.HasPrefix(value, "/") && !drive) {
		return "", false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || part == "." {
			return "", false
		}
	}
	clean := path.Clean(value)
	if drive {
		clean = strings.ToUpper(clean[:1]) + clean[1:]
		if len(clean) == 2 {
			clean += "/"
		}
	}
	return clean, true
}

// mpWithin 以完整目录边界校验路径，Windows 盘符路径按不区分大小写处理。
func mpWithin(root string, value string) bool {
	if len(root) > 1 && root[1] == ':' {
		root = strings.ToLower(root)
		value = strings.ToLower(value)
	}
	return value == root || strings.HasPrefix(value, strings.TrimRight(root, "/")+"/")
}

// mpParentPath 保留 Windows 盘符根的斜杠，避免把绝对根误变为盘符相对路径。
func mpParentPath(value string) string {
	parent := path.Dir(value)
	if len(parent) == 2 && parent[1] == ':' {
		parent += "/"
	}
	return parent
}

// mpOrganizeRootSafe 确认目标根不受其它交叠媒体库目录策略影响。
func mpOrganizeRootSafe(root mpRoot, roots []mpRoot) bool {
	if root.Kind != "library_root" || root.Overwrite != "never" {
		return false
	}
	for _, other := range roots {
		if other.Path == root.Path && other.Name == root.Name {
			continue
		}
		if mpWithin(root.Path, other.Path) || mpWithin(other.Path, root.Path) {
			return false
		}
	}
	return true
}

// mpReadRoots 只读取固定目录配置，不推断用户未配置的磁盘路径。
func (c *Client) mpReadRoots(ctx context.Context, kind string) ([]mpRoot, error) {
	usage := "download"
	if kind == "library_root" {
		usage = "library"
	}
	var dirs []struct {
		Name           string `json:"name"`            // 远端目录别名。
		Storage        string `json:"storage"`         // 下载存储。
		DownloadPath   string `json:"download_path"`   // 下载根。
		LibraryStorage string `json:"library_storage"` // 媒体库存储。
		LibraryPath    string `json:"library_path"`    // 媒体库根。
		Overwrite      string `json:"overwrite_mode"`  // 实际覆盖策略。
	}
	if err := c.callMP(ctx, "GET", c.prefix+"/storage/directories", url.Values{"directory_type": {usage}, "storage_type": {"local"}, "page": {"1"}, "count": {"101"}}, nil, &dirs); err != nil {
		return nil, err
	}
	if dirs == nil || len(dirs) > 100 {
		return nil, &Fault{Message: "目录配置过多或响应无效，请在 MoviePilot 管理", Rejected: true}
	}
	out := []mpRoot{}
	seen := map[string]bool{}
	for _, dir := range dirs {
		storage, raw := dir.Storage, dir.DownloadPath
		if kind == "library_root" {
			storage, raw = dir.LibraryStorage, dir.LibraryPath
		}
		clean, ok := mpLocalPath(raw)
		if storage != "local" || !ok {
			continue
		}
		key := clean
		if len(clean) > 1 && clean[1] == ':' {
			key = strings.ToLower(clean)
		}
		// 相同路径的多个配置可能采用不同覆盖策略，不能任意选择一项。
		if seen[key] {
			return nil, &Fault{Message: "MoviePilot 存在重复目录配置，请先合并后再操作", Rejected: true}
		}
		seen[key] = true
		out = append(out, mpRoot{Kind: kind, Name: dir.Name, Path: clean, Overwrite: dir.Overwrite})
	}
	return out, nil
}

// MPRoots 提供本地目录别名及整理资格，路径始终留在服务器。
func (c *Client) MPRoots(ctx context.Context) ([]MPChoice, error) {
	if err := c.requireCapability("organize"); err != nil {
		return nil, err
	}
	out := []MPChoice{}
	for _, kind := range []string{"download_root", "library_root"} {
		roots, err := c.mpReadRoots(ctx, kind)
		if err != nil {
			return nil, err
		}
		for index, root := range roots {
			data, snapshotErr := mpSnapshot(mpWorkflow{Kind: kind, Root: &root})
			if snapshotErr != nil {
				return nil, snapshotErr
			}
			title := c.adapterText(root.Name, 120)
			if title == "" || title == "外部条目" {
				title = fmt.Sprintf("配置目录 %d", index+1)
			}
			status := "已配置的本地下载目录"
			allowed := mpOrganizeRootSafe(root, roots)
			if kind == "library_root" {
				status = "媒体库目录，可选择文件刮削；整理需设置不覆盖"
				if allowed {
					status = "已配置不覆盖，可整理或刮削"
				}
			}
			out = append(out, MPChoice{Title: title, Kind: kind, Status: status, OrganizeAllowed: allowed, Data: data})
		}
	}
	if len(out) > 100 {
		return nil, &Fault{Message: "可选目录超过 100 项，请在 MoviePilot 简化目录配置", Rejected: true}
	}
	return out, nil
}

// mpCheckRoot 在每个操作阶段重新确认根存在，整理额外确认实际不覆盖策略。
func (c *Client) mpCheckRoot(ctx context.Context, root *mpRoot, noOverwrite bool) error {
	if root == nil || (root.Kind != "download_root" && root.Kind != "library_root") {
		return &Fault{Message: "所选目录无效，请重新选择", Rejected: true}
	}
	roots, err := c.mpReadRoots(ctx, root.Kind)
	if err != nil {
		return err
	}
	for _, current := range roots {
		if current.Name == root.Name && current.Path == root.Path {
			if noOverwrite && !mpOrganizeRootSafe(current, roots) {
				return &Fault{Message: "目标目录须在 MoviePilot 配置为不覆盖，且不能与其它媒体库目录交叠，请设置后重新预览", Rejected: true}
			}
			return nil
		}
	}
	return &Fault{Message: "MoviePilot 目录配置已改变，请重新选择", Rejected: true}
}

// mpReadFiles 请求单个目录的有界列表，拒绝返回其它路径或嵌套树。
func (c *Client) mpReadFiles(ctx context.Context, root *mpRoot, parent string) ([]mpFile, error) {
	if root == nil || !mpWithin(root.Path, parent) {
		return nil, &Fault{Message: "目录不在已配置根内", Rejected: true}
	}
	var files []mpFile
	if err := c.callMP(ctx, "POST", c.prefix+"/storage/list", url.Values{"page": {"1"}, "count": {"101"}, "sort": {"name"}}, mpFile{Storage: "local", Path: parent, Type: "dir"}, &files); err != nil {
		return nil, err
	}
	if files == nil || len(files) > 100 {
		return nil, &Fault{Message: "目录超过 100 项或响应无效，请在 MoviePilot 将文件分组后重试", Rejected: true}
	}
	out := []mpFile{}
	seen := map[string]bool{}
	for _, file := range files {
		clean, ok := mpLocalPath(file.Path)
		fileParent := mpParentPath(clean)
		if !ok || file.Storage != "local" || !mpWithin(root.Path, clean) || !mpWithin(parent, fileParent) || !mpWithin(fileParent, parent) || (file.Type != "dir" && file.Type != "file") {
			return nil, &Fault{Message: "文件列表含越界或异常条目，已停止操作", Rejected: true}
		}
		file.Path = clean
		key := clean
		if len(clean) > 1 && clean[1] == ':' {
			key = strings.ToLower(clean)
		}
		if seen[key] {
			return nil, &Fault{Message: "文件列表身份重复，已停止操作", Rejected: true}
		}
		seen[key] = true
		out = append(out, file)
	}
	return out, nil
}

// MPFiles 浏览配置根内一层目录，不接受浏览器输入的绝对路径。
func (c *Client) MPFiles(ctx context.Context, rootData json.RawMessage, parentData json.RawMessage) ([]MPChoice, error) {
	if err := c.requireCapability("organize"); err != nil {
		return nil, err
	}
	var rootValue mpWorkflow
	if json.Unmarshal(rootData, &rootValue) != nil || rootValue.Root == nil || rootValue.Kind != rootValue.Root.Kind {
		return nil, &Fault{Message: "请选择已配置的根目录", Rejected: true}
	}
	root := rootValue.Root
	if err := c.mpCheckRoot(ctx, root, false); err != nil {
		return nil, err
	}
	parent := root.Path
	if len(parentData) > 0 {
		selected, err := mpDecode(parentData, "dir")
		if err != nil {
			return nil, err
		}
		if selected.Root == nil || *selected.Root != *root || selected.File == nil || selected.File.Type != "dir" || !mpWithin(root.Path, selected.File.Path) {
			return nil, &Fault{Message: "当前目录不属于所选根，请重新浏览", Rejected: true}
		}
		parent = selected.File.Path
	}
	files, err := c.mpReadFiles(ctx, root, parent)
	if err != nil {
		return nil, err
	}
	out := []MPChoice{}
	for _, file := range files {
		if file.Type == "file" && !mpMediaFile(file.Path) {
			continue
		}
		data, snapshotErr := mpSnapshot(mpWorkflow{Kind: file.Type, Root: root, File: &file})
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		status := "媒体文件"
		if file.Type == "dir" {
			status = "打开目录"
		}
		out = append(out, MPChoice{Title: c.adapterText(file.Name, 180), Kind: file.Type, Status: status, Data: data})
	}
	return out, nil
}

// mpMediaFile 仅开放单一视频文件，避免刮削目录树或其它系统文件。
func mpMediaFile(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".mkv", ".mp4", ".avi", ".mov", ".m4v", ".ts", ".m2ts", ".wmv", ".webm", ".mpg", ".mpeg":
		return true
	}
	return false
}

// mpFileVersionAvailable 拒绝空的纳秒字段，确保文件至少有一种实际修改版本事实。
func mpFileVersionAvailable(file mpFile) bool {
	if file.ModifyTime != nil && *file.ModifyTime >= 0 {
		return true
	}
	var nanoseconds string
	if json.Unmarshal(file.ModifyNS, &nanoseconds) != nil {
		var number json.Number
		if json.Unmarshal(file.ModifyNS, &number) != nil {
			return false
		}
		nanoseconds = number.String()
	}
	value, err := strconv.ParseInt(nanoseconds, 10, 64)
	return err == nil && value >= 0 && strconv.FormatInt(value, 10) == nanoseconds
}

// mpCheckFile 在预览和执行前读取父目录核对同一文件的版本事实。
func (c *Client) mpCheckFile(ctx context.Context, selected mpWorkflow) error {
	if selected.File == nil || selected.Root == nil || selected.File.Type != "file" || !mpMediaFile(selected.File.Path) || selected.File.Size == nil || *selected.File.Size < 0 || !mpFileVersionAvailable(*selected.File) {
		return &Fault{Message: "文件缺少稳定版本信息或类型不支持，请重新选择", Rejected: true}
	}
	if err := c.mpCheckRoot(ctx, selected.Root, false); err != nil {
		return err
	}
	files, err := c.mpReadFiles(ctx, selected.Root, mpParentPath(selected.File.Path))
	if err != nil {
		return err
	}
	before, _ := json.Marshal(selected.File)
	for _, file := range files {
		if file.Path == selected.File.Path {
			after, _ := json.Marshal(file)
			if string(before) == string(after) {
				return nil
			}
			return &Fault{Message: "所选文件已变化，请重新浏览并预览", Rejected: true}
		}
	}
	return &Fault{Message: "所选文件已不存在，请重新浏览", Rejected: true}
}

// mpCheckMedia 只读核对用户确认的 TMDB 身份和影视类型。
func (c *Client) mpCheckMedia(ctx context.Context, id string, mediaType string) (string, error) {
	if !positiveID(id) || (mediaType != "movie" && mediaType != "series") {
		return "", &Fault{Message: "请确认有效 TMDB 编号及影视类型", Rejected: true}
	}
	var raw json.RawMessage
	if err := c.callMP(ctx, "GET", c.prefix+"/media/"+id, url.Values{"media_source": {"themoviedb"}, "type_name": {moviePilotMediaName(mediaType)}}, nil, &raw); err != nil {
		return "", err
	}
	actual, kind, title, _, _ := mpMediaIdentity(raw)
	if actual != id || kind != mediaType {
		return "", &Fault{Message: "TMDB 资料身份或影视类型不匹配，请重新确认", Rejected: true}
	}
	return c.adapterText(title, 180), nil
}

// mpTransferBody 固定复制和禁止自动刮削，不允许浏览器控制传输策略。
func mpTransferBody(plan mpWorkflow, preview bool) map[string]any {
	return map[string]any{"fileitems": []mpFile{*plan.File}, "target_storage": "local", "target_path": plan.Target.Path, "transfer_type": "copy", "media_source": "themoviedb", "media_id": plan.MediaID, "type_name": moviePilotMediaName(plan.MediaType), "preview": preview, "reorganize": false, "skip_success": true, "scrape": false}
}

// mpTransferResult 只接收单文件结果，禁止向前端透出私密目标路径。
type mpTransferResult struct {
	Items []struct {
		State   string `json:"state"`             // accepted/completed 等固定状态。
		Success *bool  `json:"success"`           // 单文件成功事实。
		Skipped bool   `json:"overwrite_skipped"` // 目标冲突时跳过。
	} `json:"items"` // 上游逐文件回执。
}

// MPPreview 只读校验刮削或请求整理预览，产生下一步确认用的私密计划。
func (c *Client) MPPreview(ctx context.Context, scrape bool, fileData json.RawMessage, targetData json.RawMessage, id string, mediaType string) (MPChoice, error) {
	capability, kind := "organize", "organize_plan"
	if scrape {
		capability, kind = "scrape", "scrape_plan"
	}
	if err := c.requireCapability(capability); err != nil {
		return MPChoice{}, err
	}
	selected, err := mpDecode(fileData, "file")
	if err != nil {
		return MPChoice{}, err
	}
	if selected.Root == nil || (!scrape && selected.Root.Kind != "download_root") || (scrape && selected.Root.Kind != "library_root") {
		return MPChoice{}, &Fault{Message: "整理请选择下载根中的文件，刮削请选择媒体库中的文件", Rejected: true}
	}
	if err = c.mpCheckFile(ctx, selected); err != nil {
		return MPChoice{}, err
	}
	title, err := c.mpCheckMedia(ctx, id, mediaType)
	if err != nil {
		return MPChoice{}, err
	}
	plan := mpWorkflow{Kind: kind, Root: selected.Root, File: selected.File, MediaID: id, MediaType: mediaType}
	status := "刮削预览已确认；执行会覆盖媒体信息和图片"
	if !scrape {
		target, decodeErr := mpDecode(targetData, "library_root")
		if decodeErr != nil {
			return MPChoice{}, decodeErr
		}
		plan.Target = target.Root
		if err = c.mpCheckRoot(ctx, plan.Target, true); err != nil {
			return MPChoice{}, err
		}
		if mpWithin(plan.Root.Path, plan.Target.Path) || mpWithin(plan.Target.Path, plan.Root.Path) {
			return MPChoice{}, &Fault{Message: "下载根与整理目标不能相互包含，请在 MoviePilot 使用独立目录", Rejected: true}
		}
		var preview mpTransferResult
		if err = c.callMP(ctx, "POST", c.prefix+"/transfer/manual", url.Values{"background": {"false"}}, mpTransferBody(plan, true), &preview); err != nil {
			return MPChoice{}, err
		}
		if len(preview.Items) != 1 || preview.Items[0].Success == nil || !*preview.Items[0].Success || preview.Items[0].Skipped {
			return MPChoice{}, &Fault{Message: "整理预览未通过或目标已存在，请在 MoviePilot 检查文件与目录策略", Rejected: true}
		}
		status = "整理预览通过；复制到已配置为不覆盖的目标目录"
	}
	data, err := mpSnapshot(plan)
	if err != nil {
		return MPChoice{}, err
	}
	return MPChoice{Title: title, Kind: kind, Status: status, ExternalID: id, MediaType: mediaType, Data: data}, nil
}

// MPLibrary 只展示确实存在的固定媒体库同步任务和其有限状态。
func (c *Client) MPLibrary(ctx context.Context) ([]MPChoice, error) {
	if err := c.requireCapability("refresh_library"); err != nil {
		return nil, err
	}
	var schedules []struct {
		ID string `json:"id"` /* 固定调度任务编号。 */
	}
	if err := c.callMP(ctx, "GET", c.prefix+"/dashboard/schedule", nil, nil, &schedules); err != nil {
		return nil, err
	}
	if schedules == nil || len(schedules) > 100 {
		return nil, &Fault{Message: "MoviePilot 调度任务列表无效或过多"}
	}
	found := false
	for _, job := range schedules {
		if job.ID == "mediaserver_sync" {
			if found {
				return nil, &Fault{Message: "MoviePilot 媒体库同步任务身份重复"}
			}
			found = true
		}
	}
	if !found {
		return []MPChoice{}, nil
	}
	var progress struct {
		ID     string `json:"id"`     // 进度所属任务。
		Status string `json:"status"` // 固定调度状态。
		Enable bool   `json:"enable"` // 是否正在执行。
	}
	if err := c.callMP(ctx, "GET", c.prefix+"/dashboard/schedule/mediaserver_sync/progress", nil, nil, &progress); err != nil {
		return nil, err
	}
	if progress.ID != "mediaserver_sync" {
		return nil, &Fault{Message: "MoviePilot 返回了其它调度任务的进度"}
	}
	status := "外部状态未知"
	switch progress.Status {
	case "waiting":
		status = "等待同步"
	case "running":
		status = "正在同步"
	case "success":
		status = "上次同步完成"
	case "failed":
		status = "上次同步失败"
	}
	if progress.Enable {
		status = "正在同步"
	}
	data, err := mpSnapshot(mpWorkflow{Kind: "library_job"})
	if err != nil {
		return nil, err
	}
	return []MPChoice{{Title: "刷新 MoviePilot 媒体库索引", Kind: "library_job", Status: status, Data: data}}, nil
}

// mpWorkflowWrite 对已发送的工作流写入保留不确定态，避免部分执行被视为可重试失败。
func (c *Client) mpWorkflowWrite(ctx context.Context, method string, route string, query url.Values, input any, output any) error {
	err := c.callMP(ctx, method, route, query, input, output)
	var fault *Fault
	if errors.As(err, &fault) && fault.Rejected {
		// MoviePilot 的业务失败可能已更新文件或下载任务，不能凭失败信封证明未执行。
		return &Fault{Message: fault.Message}
	}
	return err
}

// executeMoviePilotWorkflow 重查目标后一次提交固定业务动作，不接管下载器。
func (c *Client) executeMoviePilotWorkflow(ctx context.Context, m Mutation) (Outcome, error) {
	kinds := map[string]string{"download": "release", "download_start": "download", "download_stop": "download", "organize": "organize_plan", "scrape": "scrape_plan", "refresh_library": "library_job"}
	plan, err := mpDecode(m.Payload, kinds[m.Kind])
	if err != nil {
		return Outcome{}, err
	}
	switch m.Kind {
	case "download":
		id, kind, _, _, _ := mpMediaIdentity(plan.Media)
		if id == "" || id != plan.MediaID || kind != plan.MediaType || len(plan.Torrent) == 0 {
			return Outcome{}, &Fault{Message: "下载资料身份已不可用，请重新搜索", Rejected: true}
		}
		var receipt struct {
			ID string `json:"download_id"` /* 上游已接受的下载任务编号。 */
		}
		if err = c.mpWorkflowWrite(ctx, "POST", c.prefix+"/download/", nil, map[string]any{"media_in": plan.Media, "torrent_in": plan.Torrent}, &receipt); err != nil {
			return Outcome{}, err
		}
		if !mpHash(receipt.ID) {
			return Outcome{}, &Fault{Message: "下载已提交但未取得可靠任务回执，请在 MoviePilot 核对"}
		}
		return Outcome{TargetID: strings.ToLower(receipt.ID), Message: "MoviePilot 已接受所选资源下载，后续下载和整理由该服务编排"}, nil
	case "download_start", "download_stop":
		if plan.Download == nil {
			return Outcome{}, &Fault{Message: "下载任务选择无效", Rejected: true}
		}
		task, exists, readErr := c.mpFindDownload(ctx, *plan.Download)
		if readErr != nil {
			return Outcome{}, adapterPreflightError(readErr)
		}
		if !exists {
			return Outcome{}, &Fault{Message: "下载任务已改变或消失，请刷新队列", Rejected: true}
		}
		action := "start"
		if m.Kind == "download_stop" {
			action = "stop"
		}
		var receipt struct {
			Hash       string `json:"hash"`       // 操作目标散列。
			Downloader string `json:"downloader"` // 操作目标下载器。
			Results    []struct {
				Operation string `json:"operation"` /* 操作种类。 */
				Success   bool   `json:"success"`   /* 单项结果。 */
			} `json:"results"` // 单项操作回执。
		}
		if err = c.mpWorkflowWrite(ctx, "PATCH", c.prefix+"/download/"+task.Hash, nil, map[string]string{"action": action, "downloader": task.Downloader}, &receipt); err != nil {
			return Outcome{}, err
		}
		if !strings.EqualFold(receipt.Hash, task.Hash) || receipt.Downloader != task.Downloader || len(receipt.Results) != 1 || receipt.Results[0].Operation != action || !receipt.Results[0].Success {
			return Outcome{}, &Fault{Message: "任务控制回执不完整，请先核对下载状态"}
		}
		return Outcome{TargetID: task.Hash, Message: "MoviePilot 已接受单任务启停操作，请刷新队列观察状态"}, nil
	case "organize", "scrape":
		if err = c.mpCheckFile(ctx, plan); err != nil {
			return Outcome{}, adapterPreflightError(err)
		}
		if _, err = c.mpCheckMedia(ctx, plan.MediaID, plan.MediaType); err != nil {
			return Outcome{}, adapterPreflightError(err)
		}
		if m.Kind == "scrape" {
			if plan.Root.Kind != "library_root" || !m.ConfirmOverwrite {
				return Outcome{}, &Fault{Message: "刮削仅允许媒体库单文件，并须确认覆盖图片和 NFO", Rejected: true}
			}
			if err = c.mpWorkflowWrite(ctx, "POST", c.prefix+"/media/scrape/local", url.Values{"media_source": {"themoviedb"}, "media_id": {plan.MediaID}, "type_name": {moviePilotMediaName(plan.MediaType)}}, plan.File, nil); err != nil {
				return Outcome{}, err
			}
			return Outcome{Message: "MoviePilot 已完成所选文件的刮削，媒体资料与图片可能已更新"}, nil
		}
		if plan.Root.Kind != "download_root" || plan.Target == nil || plan.Target.Kind != "library_root" || mpWithin(plan.Root.Path, plan.Target.Path) || mpWithin(plan.Target.Path, plan.Root.Path) {
			return Outcome{}, &Fault{Message: "整理计划的来源或目标目录无效", Rejected: true}
		}
		// 外部配置不受本地事务控制，必须紧邻提交重读实际不覆盖策略。
		if err = c.mpCheckRoot(ctx, plan.Target, true); err != nil {
			return Outcome{}, adapterPreflightError(err)
		}
		var result mpTransferResult
		if err = c.mpWorkflowWrite(ctx, "POST", c.prefix+"/transfer/manual", url.Values{"background": {"true"}}, mpTransferBody(plan, false), &result); err != nil {
			return Outcome{}, err
		}
		if len(result.Items) != 1 || result.Items[0].Success == nil || !*result.Items[0].Success {
			return Outcome{}, &Fault{Message: "整理未取得完整受理结果，请在 MoviePilot 核对，系统不会自动重发"}
		}
		state := result.Items[0].State
		if state == "completed" {
			return Outcome{Message: "MoviePilot 已完成所选文件的复制整理；本系统仍需独立扫描入库"}, nil
		}
		if state == "skipped" && result.Items[0].Skipped {
			return Outcome{Message: "目标已存在，MoviePilot 已按不覆盖策略跳过整理"}, nil
		}
		if state != "accepted" {
			return Outcome{}, &Fault{Message: "整理尚未确认完成，请在 MoviePilot 核对队列"}
		}
		return Outcome{Message: "MoviePilot 已受理复制整理，请在外部队列核对完成情况；本系统尚未入库"}, nil
	case "refresh_library":
		jobs, readErr := c.MPLibrary(ctx)
		if readErr != nil {
			return Outcome{}, adapterPreflightError(readErr)
		}
		if len(jobs) != 1 {
			return Outcome{}, &Fault{Message: "请先在 MoviePilot 配置媒体服务器同步任务", Rejected: true}
		}
		if jobs[0].Status == "正在同步" {
			return Outcome{}, &Fault{Message: "MoviePilot 正在同步索引，请等待完成后刷新状态", Rejected: true}
		}
		if jobs[0].Status == "外部状态未知" {
			return Outcome{}, &Fault{Message: "MoviePilot 同步任务状态未确认，请稍后刷新或在外部应用核查", Rejected: true}
		}
		if err = c.mpWorkflowWrite(ctx, "GET", c.prefix+"/system/runscheduler", url.Values{"jobid": {"mediaserver_sync"}}, nil, nil); err != nil {
			return Outcome{}, err
		}
		return Outcome{TargetID: "mediaserver_sync", Message: "MoviePilot 已接受媒体库索引同步，请刷新状态查看结果；不代表媒体服务器已扫描磁盘"}, nil
	}
	return Outcome{}, &Fault{Message: "不支持的 MoviePilot 工作流动作", Rejected: true}
}

// mpFindDownload 按实际下载器和散列唯一寻找原任务。
func (c *Client) mpFindDownload(ctx context.Context, selected mpDownload) (mpDownload, bool, error) {
	if !mpHash(selected.Hash) || selected.Downloader == "" {
		return mpDownload{}, false, &Fault{Message: "下载任务身份无效", Rejected: true}
	}
	tasks, err := c.mpDownloads(ctx)
	if err != nil {
		return mpDownload{}, false, err
	}
	for _, task := range tasks {
		if task.Hash == selected.Hash && task.Downloader == selected.Downloader {
			return task, true, nil
		}
	}
	return mpDownload{}, false, nil
}

// reconcileMoviePilotWorkflow 仅以明确状态核对启停，其余无稳定回执时保守保持未知。
func (c *Client) reconcileMoviePilotWorkflow(ctx context.Context, m Mutation) (Outcome, bool, error) {
	if m.Kind == "download_start" || m.Kind == "download_stop" {
		plan, err := mpDecode(m.Payload, "download")
		if err != nil || plan.Download == nil {
			return Outcome{}, false, &Fault{Message: "原下载任务快照不可用"}
		}
		task, exists, err := c.mpFindDownload(ctx, *plan.Download)
		if err != nil {
			return Outcome{}, false, err
		}
		if exists {
			paused := task.State == "paused" || task.State == "stopped"
			running := task.State == "downloading" || task.State == "seeding"
			if m.Kind == "download_stop" && paused || m.Kind == "download_start" && running {
				return Outcome{TargetID: task.Hash, Message: "已只读确认原下载任务达到所选运行状态"}, true, nil
			}
		}
	}
	return Outcome{Message: "原操作缺少可唯一关联的完成回执，请在 MoviePilot 核查；本系统不会重复提交"}, false, nil
}
