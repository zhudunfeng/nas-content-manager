package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// downloadGate 为一次扫描或读取提供持久化下载发布清单的只读快照。
type downloadGate struct {
	roots   []string                        // 即使关闭模块也必须隔离的历史下载根。
	tasks   map[string]*downloadPublication // 任务标识对应的发布依据。
	finals  map[string]*downloadPublication // 按成品目录定位任务，避免每个文件遍历全部历史任务。
	anchors []downloadAnchor                // 识别相同物理目录的不同挂载路径。
	aliases map[string]string               // 每次扫描只解析一次目录别名，空值表示已确认的普通目录。
	mounts  []downloadMount                 // Linux 真实挂载根映射，覆盖任意深度的绑定子目录。
}

// downloadMount 保存 Linux 挂载命名空间中目录到同设备真实根的映射。
type downloadMount struct {
	device string // 内核设备编号，不依赖可伪装的挂载名称。
	root   string // 此挂载在设备文件系统内的根。
	point  string // 当前进程可见的挂载点。
}

// downloadPublication 保存受管任务的最终路径及确认选集。
type downloadPublication struct {
	root    string                      // 应用侧下载根路径。
	final   string                      // 发布目标目录。
	staging string                      // 暂存目录。
	ready   bool                        // 是否已经过发布验证。
	version int64                       // 发布清单版本。
	files   map[string]downloadGateFile // 仅已选文件的精确清单。
	dirs    map[string]bool             // 已选文件所需的目录前缀。
	app     *App                        // 按访问任务延迟读取清单，避免每个 Range 请求装入全库选集。
	ctx     context.Context             // 当前扫描或读取的取消上下文。
	id      string                      // 持久化任务编号。
	loaded  bool                        // 当前任务清单是否已经载入快照。
}

// downloadGateFile 是发布时核对过的文件版本，不包含文件内容。
type downloadGateFile struct {
	size    int64 // 文件长度，支持超大媒体。
	modTime int64 // 发布时的纳秒修改时间。
}

// downloadAnchor 将真实目录身份映射回持久化的逻辑路径。
type downloadAnchor struct {
	path string      // 规范逻辑目录。
	info os.FileInfo // 可用时记录设备和目录标识，用于绑定挂载别名识别。
}

// downloadPathKey 统一本机路径比较规则，同时保持 Linux 大小写敏感。
func downloadPathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

// loadDownloadGate 先关闭查询再处理文件系统，避免单连接数据库嵌套读取死锁。
func (a *App) loadDownloadGate(ctx context.Context) (*downloadGate, error) {
	gate := &downloadGate{tasks: make(map[string]*downloadPublication), finals: make(map[string]*downloadPublication), aliases: make(map[string]string)}
	roots := make(map[string]bool)
	if a.config.Downloads.Root != "" {
		roots[filepath.Clean(a.config.Downloads.Root)] = true
	}
	rows, err := a.db.QueryContext(ctx, "SELECT id,local_root,final_path,staging_path,ready,manifest_version FROM downloads")
	if err != nil {
		return nil, fmt.Errorf("无法读取下载发布状态")
	}
	for rows.Next() {
		var id string
		publication := &downloadPublication{app: a, ctx: ctx}
		if err = rows.Scan(&id, &publication.root, &publication.final, &publication.staging, &publication.ready, &publication.version); err != nil {
			break
		}
		if !filepath.IsAbs(publication.root) || !filepath.IsAbs(publication.final) || !filepath.IsAbs(publication.staging) || !within(publication.root, publication.final) || !within(publication.root, publication.staging) {
			err = fmt.Errorf("下载发布路径记录异常")
			break
		}
		roots[filepath.Clean(publication.root)] = true
		publication.id = id
		gate.tasks[id] = publication
		gate.finals[downloadPathKey(publication.final)] = publication
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = a.addCollectionGate(ctx, gate, roots); err != nil {
		return nil, err
	}
	anchors := make(map[string]bool)
	for root := range roots {
		gate.roots = append(gate.roots, root)
		anchors[root] = true
	}
	for _, publication := range gate.tasks {
		anchors[publication.final] = true
		anchors[publication.staging] = true
	}
	for path := range anchors {
		info, _ := os.Stat(path)
		gate.anchors = append(gate.anchors, downloadAnchor{path: path, info: info})
	}
	gate.mounts, err = loadDownloadMounts()
	if err != nil {
		return nil, err
	}
	return gate, nil
}

// loadDownloadMounts 有界解析内核挂载表，其他平台沿用规范路径及目录身份检查。
func loadDownloadMounts() ([]downloadMount, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, fmt.Errorf("无法确认下载目录的挂载隔离")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, fmt.Errorf("下载挂载信息不可读取或超出预算")
	}
	mounts := []downloadMount{}
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, "\\")
	for line := range strings.Lines(string(data)) {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			return nil, fmt.Errorf("下载挂载信息格式异常")
		}
		mounts = append(mounts, downloadMount{device: fields[2], root: unescape.Replace(fields[3]), point: unescape.Replace(fields[4])})
	}
	if len(mounts) == 0 {
		return nil, fmt.Errorf("下载挂载信息为空")
	}
	return mounts, nil
}

// downloadPhysicalPath 将当前命名空间路径转换为设备标识和设备内的绝对路径。
func downloadPhysicalPath(mounts []downloadMount, path string) (string, string) {
	var match *downloadMount
	for index := range mounts {
		mount := &mounts[index]
		if within(mount.point, path) && (match == nil || len(mount.point) > len(match.point)) {
			match = mount
		}
	}
	if match == nil {
		return "", ""
	}
	relative, err := filepath.Rel(match.point, path)
	if err != nil {
		return "", ""
	}
	return match.device, filepath.Join(match.root, relative)
}

// checkDownloadMountIsolation 拒绝原媒体根通过绑定挂载包含暂存区或下载根的别名。
func checkDownloadMountIsolation(config DownloadConfig, mediaRoots []string) error {
	mounts, err := loadDownloadMounts()
	if err != nil {
		return err
	}
	if len(mounts) == 0 {
		return nil
	}
	device, physicalRoot := downloadPhysicalPath(mounts, config.Root)
	if device == "" {
		return fmt.Errorf("下载卷不在可确认的挂载范围内")
	}
	for _, root := range mediaRoots {
		canonical, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr != nil {
			canonical = root
		}
		rootDevice, physicalMedia := downloadPhysicalPath(mounts, canonical)
		if rootDevice != device {
			continue
		}
		if within(physicalMedia, physicalRoot) || within(physicalRoot, physicalMedia) && !within(filepath.Join(physicalRoot, "complete"), physicalMedia) {
			return fmt.Errorf("媒体挂载别名包含下载根或暂存区，请仅授权独立的 complete 子树")
		}
	}
	return nil
}

// loadFiles 仅为实际访问的任务加载一次选集，内存规模不随其他下载的文件数增长。
func (p *downloadPublication) loadFiles() error {
	if p.loaded {
		return nil
	}
	p.files = make(map[string]downloadGateFile)
	p.dirs = make(map[string]bool)
	rows, err := p.app.db.QueryContext(p.ctx, "SELECT f.relative_path,f.size_bytes,f.mod_time FROM download_files f JOIN downloads d ON d.id=f.download_id AND d.manifest_version=f.version WHERE f.download_id=? AND f.version=? AND f.selected=1 AND d.ready=1", p.id, p.version)
	if err != nil {
		return fmt.Errorf("无法读取下载选集清单")
	}
	defer rows.Close()
	for rows.Next() {
		var relative string
		var file downloadGateFile
		if err = rows.Scan(&relative, &file.size, &file.modTime); err != nil {
			return err
		}
		relative = filepath.FromSlash(relative)
		if !filepath.IsLocal(relative) || relative == "." || strings.Contains(relative, "\\") && runtime.GOOS != "windows" {
			return fmt.Errorf("下载选集路径记录异常")
		}
		p.files[downloadPathKey(relative)] = file
		for parent := filepath.Dir(relative); ; parent = filepath.Dir(parent) {
			p.dirs[downloadPathKey(parent)] = true
			if parent == "." {
				break
			}
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	p.loaded = true
	return nil
}

// allowLogical 拒绝暂存及未知成品目录，只允许已发布选集与其必需祖先目录。
func (g *downloadGate) allowLogical(path string, isDir bool) (bool, bool, error) {
	managed := false
	for _, root := range g.roots {
		if !within(root, path) {
			continue
		}
		managed = true
		relative, _ := filepath.Rel(root, path)
		if isDir && (relative == "." || relative == "complete" || filepath.Dir(relative) == "complete" && validType(filepath.Base(relative))) {
			continue
		}
		var publication *downloadPublication
		for ancestor := path; within(root, ancestor); ancestor = filepath.Dir(ancestor) {
			if candidate := g.finals[downloadPathKey(ancestor)]; candidate != nil && downloadPathKey(candidate.root) == downloadPathKey(root) {
				publication = candidate
				break
			}
			if ancestor == root {
				break
			}
		}
		if publication == nil {
			return false, true, nil
		}
		allowed, err := publication.allow(path, isDir)
		if err != nil || !allowed {
			return false, true, err
		}
	}
	return true, managed, nil
}

// allow 验证逻辑路径以及真实目录别名，关闭模块和归档任务也不能绕过门禁。
func (g *downloadGate) allow(path string, isDir bool) (bool, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("下载路径无法解析")
	}
	path = filepath.Clean(path)
	allowed, managed, err := g.allowLogical(path, isDir)
	if err != nil || !allowed || managed || len(g.anchors) == 0 {
		return allowed, err
	}
	device, physical := downloadPhysicalPath(g.mounts, path)
	physicalMatch := false
	for _, root := range g.roots {
		rootDevice, physicalRoot := downloadPhysicalPath(g.mounts, root)
		if device == "" || device != rootDevice || !within(physicalRoot, physical) {
			continue
		}
		physicalMatch = true
		relative, _ := filepath.Rel(physicalRoot, physical)
		logicalPath := filepath.Join(root, relative)
		result, _, checkErr := g.allowLogical(logicalPath, isDir)
		if checkErr != nil || !result {
			return false, checkErr
		}
		actual, actualErr := os.Lstat(path)
		logical, logicalErr := os.Stat(logicalPath)
		if actualErr != nil || logicalErr != nil || actual.Mode()&os.ModeSymlink != 0 || !os.SameFile(actual, logical) {
			return false, nil
		}
	}
	if physicalMatch {
		return true, nil
	}
	directory := path
	if !isDir {
		directory = filepath.Dir(path)
	}
	logicalDirectory, alias := g.resolveAlias(directory)
	if !alias {
		return true, nil
	}
	logicalPath := logicalDirectory
	if !isDir {
		logicalPath = filepath.Join(logicalDirectory, filepath.Base(path))
	}
	result, _, err := g.allowLogical(logicalPath, isDir)
	if err != nil || !result {
		return false, err
	}
	// 仍验证访问侧文件，避免两个挂载的内容已发生分歧。
	if !isDir {
		actual, actualErr := os.Lstat(path)
		logical, logicalErr := os.Stat(logicalPath)
		if actualErr != nil || logicalErr != nil || !actual.Mode().IsRegular() || !os.SameFile(actual, logical) {
			return false, nil
		}
	}
	return true, nil
}

// resolveAlias 按真实目录身份识别绑定挂载，并缓存父目录结果供同一扫描复用。
func (g *downloadGate) resolveAlias(directory string) (string, bool) {
	for ancestor := directory; ; ancestor = filepath.Dir(ancestor) {
		if cached, exists := g.aliases[downloadPathKey(ancestor)]; exists {
			if cached == "" {
				g.aliases[downloadPathKey(directory)] = ""
				return "", false
			}
			relative, _ := filepath.Rel(ancestor, directory)
			logical := filepath.Join(cached, relative)
			g.aliases[downloadPathKey(directory)] = logical
			return logical, true
		}
		if info, err := os.Stat(ancestor); err == nil && info.IsDir() {
			for _, anchor := range g.anchors {
				if anchor.info != nil && os.SameFile(anchor.info, info) {
					relative, _ := filepath.Rel(ancestor, directory)
					logical := filepath.Join(anchor.path, relative)
					g.aliases[downloadPathKey(directory)] = logical
					return logical, true
				}
			}
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	g.aliases[downloadPathKey(directory)] = ""
	return "", false
}

// allow 检查已发布清单、父目录软链接和文件版本，不读取整份媒体计算哈希。
func (p *downloadPublication) allow(path string, isDir bool) (bool, error) {
	if !p.ready {
		return false, nil
	}
	if err := p.loadFiles(); err != nil {
		return false, err
	}
	relative, err := filepath.Rel(p.final, path)
	if err != nil {
		return false, err
	}
	if isDir {
		if !p.dirs[downloadPathKey(relative)] {
			return false, nil
		}
	} else if _, exists := p.files[downloadPathKey(relative)]; !exists {
		return false, nil
	}
	for ancestor := path; ; ancestor = filepath.Dir(ancestor) {
		info, statErr := os.Lstat(ancestor)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			return false, nil
		}
		if ancestor == path && !isDir {
			file := p.files[downloadPathKey(relative)]
			if !info.Mode().IsRegular() || info.Size() != file.size || file.modTime == 0 || info.ModTime().UnixNano() != file.modTime {
				return false, nil
			}
		}
		if downloadPathKey(ancestor) == downloadPathKey(p.root) {
			break
		}
		if filepath.Dir(ancestor) == ancestor {
			return false, nil
		}
	}
	return true, nil
}

// checkDownloadPath 为单次媒体读取应用最新门禁，数据库异常时拒绝访问。
func (a *App) checkDownloadPath(path string, isDir bool) error {
	gate, err := a.loadDownloadGate(a.ctx)
	if err != nil {
		return err
	}
	allowed, err := gate.allow(path, isDir)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("下载内容尚未发布、未被选择或文件版本已变化")
	}
	return nil
}

// blockedUnder 判断路径是否可能包含受管子树，以决定是否逐项保护缺失清理。
func (g *downloadGate) blockedUnder(path string) bool {
	for _, root := range g.roots {
		if within(path, root) || within(root, path) {
			return true
		}
	}
	// 非文本关联的路径仍可能是绑定挂载，保守进入逐项判断而不清空旧索引。
	return len(g.anchors) > 0
}
