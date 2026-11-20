package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nas-content-manager/internal/collectioncontract"
)

// collectionLocalDirectory 将来源根编号映射到已授权本地只读目录。
func (a *App) collectionLocalDirectory(s CollectionSource, child string) (string, error) {
	if !strings.HasPrefix(s.RootID, "media-") {
		return "", collectionProblem(400, "该来源未选择主服务授权的本地目录")
	}
	index, err := strconv.Atoi(strings.TrimPrefix(s.RootID, "media-"))
	if err != nil || index < 0 || index >= len(a.config.MediaRoots) {
		return "", collectionProblem(400, "本地来源根已失效，请重新选择")
	}
	sub, err := collectionRelative(child)
	if err != nil {
		return "", err
	}
	combined := filepath.Join(a.config.MediaRoots[index], filepath.FromSlash(s.Path), filepath.FromSlash(sub))
	canonical, err := a.readableDirectory(combined)
	if err != nil {
		return "", collectionProblem(400, "来源目录不可读或未通过媒体边界检查")
	}
	if !within(a.config.MediaRoots[index], canonical) {
		return "", collectionProblem(400, "来源目录越过所选根")
	}
	return canonical, nil
}

// collectionLocalPreview 有界扫描常规媒体文件，不修改原文件和标签。
func (a *App) collectionLocalPreview(ctx context.Context, root string) ([]CollectionFile, int64, []string, error) {
	files := []CollectionFile{}
	warnings := []string{}
	gate, err := a.loadDownloadGate(ctx)
	if err != nil {
		return nil, 0, nil, err
	}
	var total int64
	visited := 0
	unsupported := 0
	err = filepath.WalkDir(root, func(p string, entry fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return collectionProblem(400, "预览目录中存在无法读取的文件")
		}
		visited++
		if visited > 20000 {
			return collectionProblem(413, "目录预览超过 20000 个条目，请选择更小的子目录")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return collectionProblem(400, "目录包含链接，请选择不含链接的媒体目录")
		}
		if entry.IsDir() {
			allowed, e := gate.allow(p, true)
			if e != nil {
				return e
			}
			if !allowed {
				return filepath.SkipDir
			}
			return nil
		}
		info, e := entry.Info()
		if e != nil || !info.Mode().IsRegular() {
			return collectionProblem(400, "目录包含无法读取的非常规文件")
		}
		allowed, e := gate.allow(p, false)
		if e != nil {
			return e
		}
		if !allowed {
			return nil
		}
		supported := false
		for _, kind := range []string{"video", "music", "image", "comic", "novel"} {
			if matchesType(kind, p) {
				supported = true
				break
			}
		}
		if !supported {
			unsupported++
			return nil
		}
		relative, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		relative = filepath.ToSlash(relative)
		if _, e = collectionRelative(relative); e != nil {
			return e
		}
		if len(files) >= a.config.Collection.MaxFiles || info.Size() > a.config.Collection.MaxBytes-total {
			return collectionProblem(413, "目录超过收集文件数量或总字节预算")
		}
		total += info.Size()
		files = append(files, CollectionFile{Path: relative, Size: info.Size(), ModTime: info.ModTime().UnixNano()})
		return nil
	})
	if unsupported > 0 {
		warnings = append(warnings, fmt.Sprintf("已跳过 %d 个不支持的文件；原文件保持不变", unsupported))
	}
	if len(files) == 0 && err == nil {
		err = collectionProblem(422, "该目录没有可登记的受支持媒体文件")
	}
	return files, total, warnings, err
}

// collectionCreatePreview 建立有时效且绑定来源版本的服务器预览。
func (a *App) collectionCreatePreview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SourceID string `json:"sourceId"` /* 来源编号。 */
		URL      string `json:"url"`      /* 单条网页链接。 */
		Path     string `json:"path"`     /* 来源内相对路径。 */
		Profile  string `json:"profile"`  /* video/audio 输出。 */
		Mode     string `json:"mode"`     /* register/copy。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	s, err := a.collectionSourceByID(r.Context(), input.SourceID)
	if err != nil {
		fail(w, 404, "收集来源不存在")
		return
	}
	if !s.Enabled {
		fail(w, 409, "来源已停用")
		return
	}
	if _, err = collectionRelative(input.Path); err != nil {
		collectionFail(w, err)
		return
	}
	if input.Profile == "" {
		input.Profile = s.OutputProfile
	}
	if input.Profile == "" {
		input.Profile = "video"
	}
	if input.Profile != "video" && input.Profile != "audio" {
		fail(w, 400, "输出规格必须为视频或音频")
		return
	}
	if input.Mode == "" {
		input.Mode = "register"
	}
	if input.Mode != "register" && input.Mode != "copy" {
		fail(w, 400, "导入模式无效")
		return
	}
	id, err := newDownloadUUID()
	if err != nil {
		collectionFail(w, err)
		return
	}
	p := CollectionPreview{ID: id, SourceID: s.ID, Title: s.Name, Profile: input.Profile, Files: []CollectionFile{}, Formats: []CollectionFormat{}, Warnings: []string{}, ExpiresAt: collectionFuture(15)}
	private := collectionPrivate{SourceRevision: s.Revision}
	if (s.Kind == "local" || s.Kind == "external") && input.Mode == "register" {
		private.LocalPath, err = a.collectionLocalDirectory(s, input.Path)
		if err == nil {
			p.Files, p.TotalBytes, p.Warnings, err = a.collectionLocalPreview(r.Context(), private.LocalPath)
		}
		p.Kind = "register"
		p.Title = filepath.Base(private.LocalPath)
		private.Identity = collectionDigest("register\x00" + private.LocalPath + "\x00" + collectionJSON(p.Files))
		p.Warnings = append(p.Warnings, "只读登记现有媒体，不复制、不改名、不改写原标签；提交前请确认目录内容已完整")
	} else if s.Kind == "web" || s.Kind == "local" || s.Kind == "remote" {
		caps, e := a.collectionWorkerReady(r.Context())
		if e != nil {
			collectionFail(w, e)
			return
		}
		available := false
		for _, c := range caps.Capabilities {
			if c.Kind == s.Kind && c.Available {
				available = true
			}
		}
		if !available {
			fail(w, 503, "执行端未具备该来源的工具或安全执行能力")
			return
		}
		// 预览与后续执行保存相同的双方最小预算，避免执行端规范化后改变意图摘要。
		request := collectioncontract.PreviewRequest{Kind: s.Kind, Profile: input.Profile, SourceID: s.RootID, Path: filepath.ToSlash(filepath.Join(filepath.FromSlash(s.Path), filepath.FromSlash(input.Path))), MaxBytes: min(a.config.Collection.MaxBytes, caps.MaxBytes), MaxFiles: min(a.config.Collection.MaxFiles, caps.MaxFiles)}
		if s.Kind != "web" {
			request.Profile = ""
			p.Profile = ""
		}
		if request.Path == "." {
			request.Path = ""
		}
		if s.Kind == "web" {
			if _, e = collectionParseURL(input.URL); e != nil {
				collectionFail(w, e)
				return
			}
			request.URL = input.URL
			request.Path = ""
			request.SourceID = ""
		} else {
			found := false
			for _, root := range caps.Sources {
				if root.ID == s.RootID && root.Kind == s.Kind {
					found = true
				}
			}
			if !found {
				fail(w, 400, "执行端未配置该输入根")
				return
			}
		}
		var preview collectioncontract.Preview
		_, err = a.collectionWorkerCall(r.Context(), "POST", "/v1/previews", request, &preview)
		if err == nil {
			if preview.Identity == "" || preview.ID == "" || collectionExpired(preview.ExpiresAt) || len(preview.Files) > a.config.Collection.MaxFiles || len(preview.Title) > 512 {
				err = collectionProblem(502, "执行端预览缺少有效身份或超过限制")
			} else {
				p.Kind = s.Kind
				p.Title = preview.Title
				p.Files = collectionWorkerFiles(preview.Files)
				p.TotalBytes = preview.TotalBytes
				p.ExpiresAt = preview.ExpiresAt
				for _, f := range preview.Formats {
					p.Formats = append(p.Formats, CollectionFormat{ID: f.ID, Label: f.Label, Extension: f.Extension})
				}
				private.Identity = preview.Identity
				private.WorkerInstance = caps.InstanceID
				private.Request = collectioncontract.OperationRequest{Kind: request.Kind, URL: request.URL, Profile: request.Profile, SourceID: request.SourceID, Path: request.Path, MaxBytes: request.MaxBytes, MaxFiles: request.MaxFiles, PreviewID: preview.ID}
			}
		}
	} else {
		fail(w, 400, "RSS 候选请通过候选确认；外部应用成品请选择只读登记")
		return
	}
	if err != nil {
		collectionFail(w, err)
		return
	}
	if err = a.writeCollectionPrivate(id, private); err != nil {
		collectionFail(w, err)
		return
	}
	_, err = a.db.ExecContext(r.Context(), "INSERT INTO collection_previews(id,source_id,source_revision,body,private_ref,expires_at) VALUES(?,?,?,?,?,?)", id, s.ID, s.Revision, collectionJSON(p), id, p.ExpiresAt)
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 200, map[string]any{"preview": p})
}

// collectionCreateTask 将有效预览转换为持久化意图，重试沿用原幂等编号。
func (a *App) collectionCreateTask(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RequestID     string `json:"requestId"`     /* 幂等请求。 */
		PreviewID     string `json:"previewId"`     /* 服务器预览。 */
		ContentType   string `json:"contentType"`   /* 媒体类型。 */
		CreateLibrary bool   `json:"createLibrary"` /* 建库授权。 */
		LibraryID     int64  `json:"libraryId"`     /* 目标库。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validDownloadRequestID(input.RequestID) || !validType(input.ContentType) || input.LibraryID < 0 {
		fail(w, 400, "请求编号、媒体类型或目标库无效")
		return
	}
	a.collections.mu.Lock()
	defer a.collections.mu.Unlock()
	var body, ref, expires, sourceID string
	var revision int64
	err := a.db.QueryRowContext(r.Context(), "SELECT body,private_ref,expires_at,source_id,source_revision FROM collection_previews WHERE id=?", input.PreviewID).Scan(&body, &ref, &expires, &sourceID, &revision)
	var p CollectionPreview
	if err != nil || json.Unmarshal([]byte(body), &p) != nil {
		fail(w, 404, "预览不存在，请重新预览")
		return
	}
	private, err := a.readCollectionPrivate(ref)
	if err != nil {
		collectionFail(w, err)
		return
	}
	fingerprint := collectionDigest(p.Kind + "\x00" + private.Identity + "\x00" + p.Profile + "\x00" + input.ContentType)
	if old, e := scanCollectionTask(a.db.QueryRowContext(r.Context(), collectionSelect+" WHERE request_id=?", input.RequestID)); e == nil {
		var oldPlan collectionPlan
		_ = json.Unmarshal([]byte(old.Plan), &oldPlan)
		if old.Fingerprint != fingerprint || oldPlan.TargetLibraryID != input.LibraryID || old.CreateLibrary != input.CreateLibrary {
			fail(w, 409, "该请求编号已用于其他计划")
			return
		}
		respond(w, 202, map[string]any{"task": old.CollectionTask})
		return
	} else if e != sql.ErrNoRows {
		collectionFail(w, e)
		return
	}
	s, err := a.collectionSourceByID(r.Context(), sourceID)
	if err != nil || !s.Enabled || s.Revision != revision || collectionExpired(expires) {
		fail(w, 409, "来源已变化、停用或预览已过期，请重新预览")
		return
	}
	if input.LibraryID == 0 && !input.CreateLibrary {
		fail(w, 400, "请选择目标媒体库或允许首次建库")
		return
	}
	files := []CollectionFile{}
	for _, file := range p.Files {
		if matchesType(input.ContentType, file.Path) {
			files = append(files, file)
		}
	}
	if p.Kind != "web" && len(files) == 0 {
		fail(w, 422, "预览中没有所选内容类型的文件")
		return
	}
	if p.Kind == "register" {
		if _, err = a.collectionValidateFiles(private.LocalPath, "", files, input.ContentType); err != nil {
			fail(w, 409, "预览文件版本已变化或不可读，请重新预览")
			return
		}
	}
	if p.Kind == "web" && ((p.Profile == "audio" && input.ContentType != "music") || (p.Profile == "video" && input.ContentType != "video")) {
		fail(w, 400, "网页输出与目标类型不一致：音频应选择音乐，视频应选择视频")
		return
	}
	id, err := newDownloadUUID()
	if err != nil {
		collectionFail(w, err)
		return
	}
	t := collectionTaskRecord{CollectionTask: CollectionTask{ID: id, RequestID: input.RequestID, SourceID: s.ID, Kind: p.Kind, Title: p.Title, ContentType: input.ContentType, LibraryID: input.LibraryID}, Fingerprint: fingerprint, Plan: collectionJSON(collectionPlan{PrivateID: ref, TargetLibraryID: input.LibraryID}), CreateLibrary: input.CreateLibrary, Files: files}
	if p.Kind == "register" {
		t.FinalPath = private.LocalPath
	} else {
		t.LocalRoot = a.config.Collection.ReadRoot
		t.FinalPath = filepath.Join(t.LocalRoot, "complete", id)
	}
	t, err = a.insertCollectionTask(r.Context(), t)
	if err != nil {
		collectionFail(w, err)
		return
	}
	respond(w, 202, map[string]any{"task": t.CollectionTask})
}
