package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// searchDownloadPayload 将不含地址或凭据的结果令牌持久绑定到添加幂等键。
type searchDownloadPayload struct {
	SearchResultID string `json:"searchResultId"` // 本次用户确认的随机搜索结果编号。
}

// searchDownloadDuplicate 在缓存访问前核对持久绑定，重启或过期后仍可安全重试同一添加。
func (a *App) searchDownloadDuplicate(w http.ResponseWriter, r *http.Request, input downloadAddInput) bool {
	var downloadID, payload, kind string
	err := a.db.QueryRowContext(r.Context(), "SELECT download_id,payload,kind FROM download_operations WHERE request_id=?", input.RequestID).Scan(&downloadID, &payload, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		fail(w, 500, "查询搜种添加请求失败")
		return true
	}
	var binding searchDownloadPayload
	if kind != "add" || json.Unmarshal([]byte(payload), &binding) != nil || binding.SearchResultID != input.SearchResultID {
		fail(w, 409, "该请求编号已用于另一操作或搜索结果，请使用新请求编号")
		return true
	}
	download, err := a.downloadByID(r.Context(), downloadID)
	if err != nil {
		fail(w, 500, "读取已登记的搜种下载失败，请刷新任务列表")
		return true
	}
	if download.ContentType != input.ContentType {
		fail(w, 409, "该请求已按另一内容类型登记，请使用新请求编号")
		return true
	}
	respond(w, 202, map[string]any{"download": a.downloads.view(download)})
	return true
}

// bindDuplicateSearchDownload 对内容身份已存在的任务记录已完成的别名请求，避免重试再次取种。
func (a *App) bindDuplicateSearchDownload(ctx context.Context, input downloadAddInput, download downloadRecord) error {
	if input.SearchResultID == "" {
		return nil
	}
	if download.ContentType != input.ContentType {
		return fmt.Errorf("相同内容已按其它类型登记，请查看原任务")
	}
	payload, err := json.Marshal(searchDownloadPayload{SearchResultID: input.SearchResultID})
	if err != nil {
		return fmt.Errorf("保存搜种添加请求失败")
	}
	_, err = a.db.ExecContext(ctx, "INSERT INTO download_operations(request_id,download_id,revision,kind,payload,status,created_at,updated_at) VALUES(?,?,?,'add',?,'completed',?,?)", input.RequestID, download.ID, download.Revision, string(payload), now(), now())
	if err != nil {
		return fmt.Errorf("搜种添加请求已被使用，请刷新任务列表")
	}
	return nil
}

// resolveSearchDownload 从短期令牌恢复有界来源，只允许固定索引服务的下载代理。
func (a *App) resolveSearchDownload(ctx context.Context, id string) ([]byte, string, int, string) {
	m := a.torrentSearch
	config, _ := m.snapshotConfig()
	if !config.Enabled {
		return nil, "", 503, "搜种尚未启用，请按部署文档配置来源"
	}
	result, status, message := m.findResult(id)
	if status != 0 {
		return nil, "", status, message
	}
	if !result.View.Downloadable {
		return nil, "", 422, "该结果没有可用磁力或受信任下载代理，请从索引服务下载种子后手动上传"
	}
	if strings.HasPrefix(result.Location, "magnet:") {
		return []byte(result.Location), "magnet", 0, ""
	}
	if !validTorznabProxyURL(result.Source.URL, result.Location) {
		return nil, "", 422, "该结果不是受信任的索引器下载代理，请重新搜索或手动上传种子"
	}
	select {
	case m.downloadSlots <- struct{}{}:
		defer func() { <-m.downloadSlots }()
	default:
		return nil, "", 429, "正在解析其它种子，请稍后重试当前添加"
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, result.Location, nil)
	if err != nil {
		return nil, "", 502, "索引器下载代理配置无效，请重新搜索"
	}
	request.Header.Set("Accept", "application/x-bittorrent, application/octet-stream")
	response, err := m.http.Do(request)
	if err != nil {
		return nil, "", 502, "种子代理连接失败或超时，可保留当前表单重试"
	}
	defer response.Body.Close()
	var source []byte
	kind := "torrent"
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		location := response.Header.Get("Location")
		if len(location) > 16384 {
			return nil, "", 502, "种子代理返回的下载信息超过限制"
		}
		if _, err = parseMagnet(location); err != nil {
			// 不向 HTTP 跳转目标转发查询中的 API 密钥，也不跟随同源管理端点。
			return nil, "", 502, "种子代理重定向不受支持，请配置索引服务代理下载或手动上传种子"
		}
		source, kind = []byte(location), "magnet"
	} else {
		if response.StatusCode != http.StatusOK {
			return nil, "", 502, "索引服务未能提供种子，请检查来源或稍后重试"
		}
		source, err = io.ReadAll(io.LimitReader(response.Body, maxTorrentBytes+1))
		if err != nil {
			return nil, "", 502, "种子代理读取中断，可保留当前表单重试"
		}
		if len(source) > maxTorrentBytes {
			return nil, "", 413, "索引器种子超过 8 MiB 限额，请选择其它结果"
		}
	}
	var identity torrentIdentity
	if kind == "torrent" {
		identity, err = parseTorrent(source)
	} else {
		identity, err = parseMagnet(string(source))
	}
	if err != nil {
		return nil, "", 502, "索引器返回的种子无效或不符合路径、文件清单安全限制"
	}
	if result.V1 != "" && result.V1 != identity.V1 || result.V2 != "" && result.V2 != identity.V2 {
		return nil, "", 502, "索引器返回内容与搜索身份不一致，请重新搜索"
	}
	a.logger.Info("搜种来源已解析，继续安全下载登记", "sourceId", result.Source.ID, "sourceKind", kind)
	return source, kind, 0, ""
}
