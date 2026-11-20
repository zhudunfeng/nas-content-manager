package collectionworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	contract "nas-content-manager/internal/collectioncontract"
)

// supportedMedia 对首期能被媒体库识别的普通文件采用固定扩展白名单。
func supportedMedia(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".mp4", ".mkv", ".webm", ".mov", ".avi", ".m4v", ".ts", ".m2ts", ".mp3", ".flac", ".m4a", ".aac", ".ogg", ".opus", ".wav", ".aif", ".aiff", ".wma", ".alac", ".ape", ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".avif", ".cbz", ".epub", ".txt":
		return true
	}
	return false
}

// safeRelative 拒绝路径穿越、Windows别名和危险设备名，统一输出斜线相对路径。
func safeRelative(value string, allowRoot bool) bool {
	if allowRoot && (value == "" || value == ".") {
		return true
	}
	if value == "" || len(value) > 2048 || strings.ContainsAny(value, "\\:\x00\r\n") || strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "." || part == ".." || part == "" || len(part) > 255 || strings.TrimRight(part, ". ") != part || strings.ContainsAny(part, "<>\"|?*") {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
			return false
		}
		for _, c := range part {
			if c < 32 || c == 127 {
				return false
			}
		}
	}
	return true
}

// localFiles 在已授权根内有界扫描，拒绝所有链接、特殊文件和大小预算超限。
func localFiles(ctx context.Context, src SourceConfig, selected string, maxBytes int64, maxFiles int) ([]contract.File, error) {
	root, err := openApprovedRoot(src.Path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if selected == "" {
		selected = "."
	}
	if err = rejectLinkComponents(root, selected); err != nil {
		return nil, err
	}
	info, err := root.Lstat(selected)
	if err != nil {
		return nil, errors.New("来源文件不可读取")
	}
	base := selected
	if !info.IsDir() {
		base = path.Dir(selected)
	}
	files := []contract.File{}
	var bytes int64
	visited := 0
	err = fs.WalkDir(root.FS(), selected, func(name string, entry fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		visited++
		if visited > 20000 {
			return errors.New("扫描条目超过限制")
		}
		if walkErr != nil {
			return errors.New("来源目录读取失败")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("来源包含链接，已拒绝复制")
		}
		if entry.IsDir() {
			return nil
		}
		info, e := entry.Info()
		if e != nil || !info.Mode().IsRegular() {
			return errors.New("来源包含非常规文件")
		}
		if !supportedMedia(name) {
			return nil
		}
		rel := name
		if base != "." {
			rel = strings.TrimPrefix(name, base+"/")
		}
		if !safeRelative(rel, false) {
			return errors.New("来源文件名不安全")
		}
		if info.Size() <= 0 || info.Size() > maxBytes-bytes || len(files) >= maxFiles {
			return errors.New("来源文件数量或大小超出预算")
		}
		bytes += info.Size()
		files = append(files, contract.File{Path: rel, Size: info.Size(), ModTime: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("没有可导入的受支持媒体文件")
	}
	return files, nil
}

// openApprovedRoot 要求授权目录的所有父路径也没有链接别名。
func openApprovedRoot(dir string) (*os.Root, error) {
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil || !sameNativePath(filepath.Clean(dir), canonical) {
		return nil, errors.New("来源根不存在或包含链接别名")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("来源根不是普通目录")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, errors.New("来源根不可读取")
	}
	return root, nil
}

// sameNativePath 比较当前系统规范路径，Windows路径大小写由平台规则处理。
func sameNativePath(a string, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b) || filepath.VolumeName(a) != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// rejectLinkComponents 逐段拒绝根内链接，即使链接目标仍在根内也不接受。
func rejectLinkComponents(root *os.Root, relative string) error {
	current := ""
	for _, part := range strings.Split(relative, "/") {
		if part == "." || part == "" {
			continue
		}
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			return errors.New("来源路径不可读取")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("路径包含链接")
		}
	}
	return nil
}

// copyLocal 将预览时记录的普通文件流式复制到新隔离目录，原始文件只读打开。
func (s *Service) copyLocal(ctx context.Context, input contract.OperationRequest, target string, expected []contract.File) ([]contract.File, error) {
	src, _ := s.source(input.Kind, input.SourceID)
	files := append([]contract.File{}, expected...)
	root, err := openApprovedRoot(src.Path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dest, err := os.OpenRoot(target)
	if err != nil {
		return nil, errors.New("暂存目录不可打开")
	}
	defer dest.Close()
	selected := input.Path
	if selected == "" {
		selected = "."
	}
	selectedInfo, err := root.Lstat(selected)
	if err != nil {
		return nil, errors.New("来源已经变化")
	}
	base := selected
	if !selectedInfo.IsDir() {
		base = path.Dir(selected)
	}
	for i, entry := range files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		sourceName := path.Join(base, entry.Path)
		if err = rejectLinkComponents(root, sourceName); err != nil {
			return nil, err
		}
		in, e := root.Open(sourceName)
		if e != nil {
			return nil, errors.New("来源文件无法打开")
		}
		before, e := in.Stat()
		if e != nil || !before.Mode().IsRegular() || before.Size() != entry.Size || before.ModTime().UnixNano() != entry.ModTime {
			in.Close()
			return nil, errors.New("来源版本已改变，请重新预览")
		}
		if err = dest.MkdirAll(path.Dir(entry.Path), 0700); err != nil {
			in.Close()
			return nil, errors.New("暂存目录无法创建")
		}
		out, e := dest.OpenFile(entry.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return nil, errors.New("暂存文件冲突或无法创建")
		}
		h := sha256.New()
		copied, copyErr := copyContext(ctx, io.MultiWriter(out, h), io.LimitReader(in, entry.Size+1))
		after, statErr := in.Stat()
		_ = in.Close()
		if copyErr == nil {
			copyErr = out.Sync()
		}
		closeErr := out.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil || statErr != nil || copied != entry.Size || after.Size() != before.Size() || after.ModTime() != before.ModTime() {
			return nil, errors.New("来源在复制期间变化或暂存写入失败")
		}
		actual, e := dest.Stat(entry.Path)
		if e != nil {
			return nil, errors.New("暂存文件无法核验")
		}
		files[i] = contract.File{Path: entry.Path, Size: actual.Size(), ModTime: actual.ModTime().UnixNano(), SHA256: hex.EncodeToString(h.Sum(nil))}
	}
	after, err := localFiles(ctx, src, input.Path, input.MaxBytes, input.MaxFiles)
	if err != nil || !filesSameSnapshot(expected, after) {
		return nil, errors.New("来源在复制期间变化，请重新预览")
	}
	return files, nil
}

// copyContext 以固定小缓冲区复制，并在块之间响应取消。
func copyContext(ctx context.Context, dest io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 128<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := source.Read(buffer)
		if n > 0 {
			written, e := dest.Write(buffer[:n])
			total += int64(written)
			if e != nil {
				return total, e
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

// inspectFiles 对执行器实际产物重新有界枚举，拒绝临时文件、链接和未经支持的格式。
func inspectFiles(ctx context.Context, dir string, maxBytes int64, maxFiles int) ([]contract.File, error) {
	files, err := localFiles(ctx, SourceConfig{Path: dir}, ".", maxBytes, maxFiles)
	if err != nil {
		return nil, err
	}
	visited := 0
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	err = fs.WalkDir(root.FS(), ".", func(name string, e fs.DirEntry, walkErr error) error {
		visited++
		if visited > 20000 {
			return errors.New("产物条目过多")
		}
		if walkErr != nil {
			return walkErr
		}
		if e.IsDir() {
			return nil
		}
		if !supportedMedia(name) {
			return errors.New("产物包含临时文件或不支持格式")
		}
		return nil
	})
	return files, err
}

// manifestEqual 校验已封存的相对路径、尺寸和修改时间，不因同名而信任产物。
func manifestEqual(expected []contract.File, actual []contract.File) bool {
	if len(expected) != len(actual) {
		return false
	}
	seen := map[string]contract.File{}
	for _, f := range expected {
		seen[f.Path] = f
	}
	if len(seen) != len(expected) {
		return false
	}
	for _, f := range actual {
		old, ok := seen[f.Path]
		if !ok || old.Size != f.Size || old.ModTime != f.ModTime {
			return false
		}
	}
	return true
}

// publishHTTP 只在控制面显式批准后核对并同卷原子发布，不覆盖既有成品。
func (s *Service) publishHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, ok := s.ops[r.PathValue("id")]
	if !ok {
		failJSON(w, 404, "获取操作不存在")
		return
	}
	if saved.Operation.State == "published" {
		writeJSON(w, 200, saved.Operation)
		return
	}
	if saved.Operation.State != "acquired" {
		failJSON(w, 409, "操作尚未取得可发布的产物")
		return
	}
	staging := filepath.Join(s.config.DataDir, "staging", saved.Operation.ID)
	complete := filepath.Join(s.config.DataDir, "complete", saved.Operation.ID)
	files, err := inspectFiles(r.Context(), staging, saved.Request.MaxBytes, saved.Request.MaxFiles)
	if err != nil || !manifestEqual(saved.Operation.Files, files) {
		failJSON(w, 409, "产物清单或文件版本已经改变")
		return
	}
	if _, err = os.Lstat(complete); !os.IsNotExist(err) {
		failJSON(w, 409, "发布目录已存在或无法核验")
		return
	}
	saved.Operation.State = "publishing"
	saved.Operation.Message = "正在核验并发布产物"
	if err = s.saveLocked(&saved); err != nil {
		failJSON(w, 500, err.Error())
		return
	}
	s.ops[saved.Operation.ID] = saved
	if err = os.Rename(staging, complete); err != nil {
		saved.Operation.State = "recovery_required"
		saved.Operation.Message = "发布移动未能确认，需要核对目录和卷映射"
	} else {
		saved.Operation.State = "published"
		saved.Operation.RelativeDir = "complete/" + saved.Operation.ID
		saved.Operation.Message = "产物已发布，等待媒体库独立核验入库"
	}
	if err = s.saveLocked(&saved); err != nil {
		failJSON(w, 500, "发布结果保存失败，请按原操作标识对账")
		return
	}
	s.ops[saved.Operation.ID] = saved
	s.logger.Info("资源产物发布阶段结束", "operation", saved.Operation.ID, "state", saved.Operation.State)
	writeJSON(w, 200, saved.Operation)
}

// recoverPublish 对账已登记发布的确定目录，结果不明确时保留人工处理状态。
func (s *Service) recoverPublish(saved *savedOperation) {
	saved.Operation.State = "recovery_required"
	saved.Operation.Message = "发布在重启时中断，目录结果需要核对"
	complete := filepath.Join(s.config.DataDir, "complete", saved.Operation.ID)
	staging := filepath.Join(s.config.DataDir, "staging", saved.Operation.ID)
	_, stageErr := os.Lstat(staging)
	_, completeErr := os.Lstat(complete)
	if os.IsNotExist(stageErr) && completeErr == nil {
		files, err := inspectFiles(s.ctx, complete, saved.Request.MaxBytes, saved.Request.MaxFiles)
		if err == nil && manifestEqual(saved.Operation.Files, files) {
			saved.Operation.State = "published"
			saved.Operation.RelativeDir = "complete/" + saved.Operation.ID
			saved.Operation.Message = "已对账确认发布完成"
		}
		return
	}
	if stageErr == nil && os.IsNotExist(completeErr) {
		files, err := inspectFiles(s.ctx, staging, saved.Request.MaxBytes, saved.Request.MaxFiles)
		if err == nil && manifestEqual(saved.Operation.Files, files) {
			saved.Operation.State = "acquired"
			saved.Operation.Message = "发布尚未移动产物，等待重新确认发布"
		}
	}
}

// probeHTTP 只在专属目录创建和删除随机挑战，不接触用户媒体。
func (s *Service) probeHTTP(w http.ResponseWriter, r *http.Request) {
	nonce := r.URL.Query().Get("nonce")
	if len(nonce) < 16 || len(nonce) > 80 || !safeID.MatchString(nonce) {
		failJSON(w, 400, "映射探针挑战格式无效")
		return
	}
	p := filepath.Join(s.config.DataDir, "probes", nonce)
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == http.MethodDelete {
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			w.WriteHeader(204)
			return
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(nonce)) {
			failJSON(w, 409, "映射探针版本不可确认")
			return
		}
		data, e := os.ReadFile(p)
		if e != nil || string(data) != nonce {
			failJSON(w, 409, "映射探针内容不匹配")
			return
		}
		if os.Remove(p) != nil {
			failJSON(w, 500, "映射探针无法清理")
			return
		}
		w.WriteHeader(204)
		return
	}
	entries, err := os.ReadDir(filepath.Join(s.config.DataDir, "probes"))
	if err != nil || len(entries) > 256 {
		failJSON(w, 409, "映射探针达到上限，需要清理旧探针")
		return
	}
	for _, entry := range entries {
		if !safeID.MatchString(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, e := entry.Info()
		if e == nil && info.Mode().IsRegular() && time.Since(info.ModTime()) > 10*time.Minute {
			_ = os.Remove(filepath.Join(s.config.DataDir, "probes", entry.Name()))
		}
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		failJSON(w, 409, "映射探针已存在，请使用新挑战")
		return
	}
	_, err = f.WriteString(nonce)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		failJSON(w, 500, "映射探针无法写入")
		return
	}
	writeJSON(w, 200, contract.Probe{RelativePath: "probes/" + nonce, Nonce: nonce})
}

// source 按固定类型和标识查找授权根，不接受调用方提供真实连接参数。
func (s *Service) source(kind string, id string) (SourceConfig, bool) {
	for _, src := range s.config.Sources {
		if src.Kind == kind && src.ID == id {
			return src, true
		}
	}
	return SourceConfig{}, false
}

// filesSameSnapshot 比较来源版本快照，用于拒绝预览后被替换的输入。
func filesSameSnapshot(a []contract.File, b []contract.File) bool { return reflect.DeepEqual(a, b) }

// reserveSpace 在执行前检查隔离目录所在卷是否具备请求预算和保留空间。
func (s *Service) reserveSpace(bytes int64) error {
	free, err := freeDiskBytes(s.config.DataDir)
	if err != nil {
		return errors.New("无法确认暂存卷剩余空间")
	}
	if uint64(bytes)+uint64(s.config.MinFreeMB)*(1<<20) > free {
		return fmt.Errorf("暂存卷空间不足或低于保留限制")
	}
	return nil
}
