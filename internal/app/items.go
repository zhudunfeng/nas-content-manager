package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// itemSelect 将公共内容、用户收藏与进度一次查询，避免 N+1 读取。
const itemSelect = `SELECT i.id,i.library_id,i.title,i.type,i.relative_path,i.size,i.mime,i.added_at,i.updated_at,i.missing,i.duration,i.width,i.height,i.mod_time,i.is_dir,l.path,i.enrich_state,coalesce(u.favorite,0),coalesce(u.tags,'[]'),p.position,p.total,p.locator,p.updated_at FROM items i JOIN libraries l ON l.id=i.library_id LEFT JOIN user_items u ON u.item_id=i.id AND u.user_id=? LEFT JOIN progress p ON p.item_id=i.id AND p.user_id=?`

// rowScanner 是数据库单行与多行结果共同支持的读取接口。
type rowScanner interface {
	// Scan 将当前记录填入显式类型目标。
	Scan(dest ...any) error
}

// scanItem 将数据库记录转换为稳定 JSON 模型，空标签始终输出数组。
func scanItem(row rowScanner) (Item, error) {
	var item Item
	var tags string
	var position, total sql.NullFloat64
	var locator, updated sql.NullString
	err := row.Scan(&item.ID, &item.LibraryID, &item.Title, &item.Type, &item.RelativePath, &item.Size, &item.Mime, &item.AddedAt, &item.UpdatedAt, &item.Missing, &item.Duration, &item.Width, &item.Height, &item.ModTime, &item.IsDir, &item.LibraryPath, &item.EnrichState, &item.Favorite, &tags, &position, &total, &locator, &updated)
	if err != nil {
		return item, err
	}
	item.Tags = []string{}
	if json.Unmarshal([]byte(tags), &item.Tags) != nil || item.Tags == nil {
		item.Tags = []string{}
	}
	item.CoverURL = fmt.Sprintf("/api/items/%d/cover?v=%d-%d-%s", item.ID, item.ModTime, item.Size, item.EnrichState)
	if position.Valid {
		item.Progress = &Progress{Position: position.Float64, Total: total.Float64, Locator: locator.String, UpdatedAt: updated.String}
	}
	return item, nil
}

// itemByID 读取内容与请求用户的个人状态。
func (a *App) itemByID(ctx context.Context, id int64, userID int64) (Item, error) {
	return scanItem(a.db.QueryRowContext(ctx, itemSelect+" WHERE i.id=?", userID, userID, id))
}

// listItems 应用参数化搜索、分页与白名单排序。
func (a *App) listItems(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		fail(w, 400, "页码过大")
		return
	}
	pageSize, _ := strconv.Atoi(query.Get("pageSize"))
	if pageSize < 1 {
		pageSize = 48
	}
	if pageSize > 96 {
		pageSize = 96
	}
	userID := currentUser(r).ID
	args := []any{userID, userID}
	conditions := []string{"1=1"}
	if kind := query.Get("type"); kind != "" {
		if !validType(kind) {
			fail(w, 400, "内容类型无效")
			return
		}
		conditions = append(conditions, "i.type=?")
		args = append(args, kind)
	}
	if library := query.Get("libraryId"); library != "" {
		id, err := strconv.ParseInt(library, 10, 64)
		if err != nil || id < 1 {
			fail(w, 400, "媒体库编号无效")
			return
		}
		conditions = append(conditions, "i.library_id=?")
		args = append(args, id)
	}
	if query.Get("favorite") == "true" {
		conditions = append(conditions, "u.favorite=1")
	}
	if search := strings.TrimSpace(query.Get("q")); search != "" {
		if len(search) > 300 {
			fail(w, 400, "搜索内容过长")
			return
		}
		conditions = append(conditions, "(i.title LIKE ? OR i.relative_path LIKE ? OR u.tags LIKE ?)")
		search = "%" + search + "%"
		args = append(args, search, search, search)
	}
	order := "i.added_at DESC,i.id DESC"
	switch query.Get("sort") {
	case "title":
		order = "i.title COLLATE NOCASE,i.id"
	case "progress":
		order = "p.updated_at DESC,i.id DESC"
		conditions = append(conditions, "p.item_id IS NOT NULL")
	}
	where := " WHERE " + strings.Join(conditions, " AND ")
	var total int64
	countSQL := `SELECT count(*) FROM items i LEFT JOIN user_items u ON u.item_id=i.id AND u.user_id=? LEFT JOIN progress p ON p.item_id=i.id AND p.user_id=?` + where
	if err := a.db.QueryRowContext(r.Context(), countSQL, args...).Scan(&total); err != nil {
		a.logger.Error("内容计数失败", "error", err)
		fail(w, 500, "内容读取失败")
		return
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := a.db.QueryContext(r.Context(), itemSelect+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
	if err != nil {
		fail(w, 500, "无法读取内容")
		return
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			fail(w, 500, "内容记录无效")
			return
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		fail(w, 500, "内容读取中断")
		return
	}
	respond(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

// requestItem 查找路由中的内容并统一处理不存在状态。
func (a *App) requestItem(w http.ResponseWriter, r *http.Request) (Item, bool) {
	id, ok := requestID(w, r)
	if !ok {
		return Item{}, false
	}
	item, err := a.itemByID(r.Context(), id, currentUser(r).ID)
	if err == sql.ErrNoRows {
		fail(w, 404, "内容不存在或已移除")
		return item, false
	}
	if err != nil {
		fail(w, 500, "无法读取内容")
		return item, false
	}
	return item, true
}

// getItem 返回用于详情面板的完整内容信息。
func (a *App) getItem(w http.ResponseWriter, r *http.Request) {
	item, ok := a.requestItem(w, r)
	if ok {
		respond(w, 200, map[string]any{"item": item})
	}
}

// favorite 幂等更新用户收藏状态，不覆盖标签。
func (a *App) favorite(w http.ResponseWriter, r *http.Request) {
	item, ok := a.requestItem(w, r)
	if !ok {
		return
	}
	var input struct {
		Favorite bool `json:"favorite"` /* 收藏开关。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), "INSERT INTO user_items(user_id,item_id,favorite) VALUES(?,?,?) ON CONFLICT(user_id,item_id) DO UPDATE SET favorite=excluded.favorite", currentUser(r).ID, item.ID, input.Favorite); err != nil {
		fail(w, 500, "保存收藏失败")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

// tags 去重并限制标签数量与长度，标签仅存为文本数据。
func (a *App) tags(w http.ResponseWriter, r *http.Request) {
	item, ok := a.requestItem(w, r)
	if !ok {
		return
	}
	var input struct {
		Tags []string `json:"tags"` /* 用户标签列表。 */
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.Tags) > 20 {
		fail(w, 400, "每项最多设置 20 个标签")
		return
	}
	clean := []string{}
	seen := map[string]bool{}
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if utf8.RuneCountInString(tag) > 40 {
			fail(w, 400, "单个标签最多 40 字")
			return
		}
		if tag != "" && !seen[tag] {
			clean = append(clean, tag)
			seen[tag] = true
		}
	}
	data, _ := json.Marshal(clean)
	if _, err := a.db.ExecContext(r.Context(), "INSERT INTO user_items(user_id,item_id,tags) VALUES(?,?,?) ON CONFLICT(user_id,item_id) DO UPDATE SET tags=excluded.tags", currentUser(r).ID, item.ID, string(data)); err != nil {
		fail(w, 500, "保存标签失败")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

// progress 保存有界播放或阅读位置，支持刷新和跨设备恢复。
func (a *App) progress(w http.ResponseWriter, r *http.Request) {
	item, ok := a.requestItem(w, r)
	if !ok {
		return
	}
	var input struct {
		Position float64 `json:"position"` // 当前秒数、页码或段落。
		Total    float64 `json:"total"`    // 总长度。
		Locator  string  `json:"locator"`  // 阅读定位信息。
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if math.IsNaN(input.Position) || math.IsInf(input.Position, 0) || input.Position < 0 || input.Total < 0 || input.Position > 1e12 || input.Total > 1e12 || len(input.Locator) > 4096 {
		fail(w, 400, "进度信息超出允许范围")
		return
	}
	if _, err := a.db.ExecContext(r.Context(), "INSERT INTO progress(user_id,item_id,position,total,locator,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(user_id,item_id) DO UPDATE SET position=excluded.position,total=excluded.total,locator=excluded.locator,updated_at=excluded.updated_at", currentUser(r).ID, item.ID, input.Position, input.Total, input.Locator, now()); err != nil {
		fail(w, 500, "保存阅读进度失败")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

// stats 返回内容总量与原文件大小，避免误报 NAS 可用磁盘空间。
func (a *App) stats(w http.ResponseWriter, r *http.Request) {
	result := map[string]int64{"total": 0, "video": 0, "image": 0, "comic": 0, "novel": 0, "music": 0, "totalSize": 0}
	rows, err := a.db.QueryContext(r.Context(), "SELECT type,count(*),coalesce(sum(size),0) FROM items GROUP BY type")
	if err != nil {
		fail(w, 500, "无法读取内容统计")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var count, size int64
		if rows.Scan(&kind, &count, &size) != nil {
			fail(w, 500, "统计记录无效")
			return
		}
		result[kind] = count
		result["total"] += count
		result["totalSize"] += size
	}
	respond(w, 200, result)
}
