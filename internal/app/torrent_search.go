package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxTorrentSearchResults = 500              // 单次聚合最多五百条可展示结果。
	torrentSearchTTL        = 10 * time.Minute // 私密下载来源在内存中最多保留十分钟。
	maxTorrentSearchBatches = 8                // 最多保留八个搜索快照，避免无限增长。
)

// TorrentSearchResult 是不包含链接、密钥、GUID 或原始响应的安全展示模型。
type TorrentSearchResult struct {
	ID           string `json:"id"`           // 有到期时间的随机结果令牌，不是站点资源编号。
	Title        string `json:"title"`        // 有限长度的纯文本名称。
	SourceID     string `json:"sourceId"`     // 配置来源编号。
	SourceName   string `json:"sourceName"`   // 配置来源显示名。
	SizeBytes    *int64 `json:"sizeBytes"`    // 资源体积字节，未知时为空。
	Seeders      *int64 `json:"seeders"`      // 做种人数，未知时为空。
	Leechers     *int64 `json:"leechers"`     // 下载人数，未知时为空。
	PublishedAt  string `json:"publishedAt"`  // 可识别的 UTC 发布时间，未知时为空。
	Downloadable bool   `json:"downloadable"` // 是否有有效磁力或同源种子代理。
}

// TorrentSearchSourceStatus 表示每个来源独立的本次搜索结果，不暴露底层地址或错误。
type TorrentSearchSourceStatus struct {
	ID    string `json:"id"`    // 配置来源编号。
	Name  string `json:"name"`  // 来源显示名称。
	State string `json:"state"` // ready 或 error。
	Error string `json:"error"` // 已脱敏的中文错误，成功时为空。
}

// torrentSearchResponse 是一次有界搜索快照，排序和分页由前端在快照内完成。
type torrentSearchResponse struct {
	Results   []TorrentSearchResult       `json:"results"`   // 安全结果集合，空集合输出 []。
	Sources   []TorrentSearchSourceStatus `json:"sources"`   // 各来源状态，部分故障不抹去成功结果。
	ExpiresAt string                      `json:"expiresAt"` // 结果令牌统一到期 UTC 时间。
	Truncated bool                        `json:"truncated"` // 来源或聚合达到预算时为 true。
}

// torrentSearchPrivateResult 仅在服务端保存下载材料，绝不直接序列化。
type torrentSearchPrivateResult struct {
	View     TorrentSearchResult // 对外展示元数据。
	Source   TorrentSearchSource // 固定来源配置及凭据，仅限内存。
	Location string              // 验证过的磁力或同源种子代理地址。
	V1       string              // 已知 v1 身份，仅用于去重和下载后核对。
	V2       string              // 已知 v2 身份，仅用于去重和下载后核对。
	Key      string              // 无身份时用于去重的服务端摘要。
}

// torrentSearchBatch 保存一个有固定截止时间的私密来源快照。
type torrentSearchBatch struct {
	Expires time.Time                    // 不因读取或重试延长的失效时间。
	Results []torrentSearchPrivateResult // 最多五百条私密下载材料。
}

// torrentSearchManager 管理有界网络资源和短期来源缓存，不持有媒体扫描锁。
type torrentSearchManager struct {
	generation    uint64               // 来源快照版本，隔离刷新前的在途请求与结果。
	config        TorrentSearchConfig  // 已验证配置，只有启用时才请求网络。
	http          *http.Client         // 无代理、无凭据、禁止自动重定向的独立客户端。
	searchSlots   chan struct{}        // 同时最多两次聚合搜索。
	downloadSlots chan struct{}        // 同时最多两次私密种子解析。
	mu            sync.Mutex           // 仅保护短期缓存，不跨网络持锁。
	batches       []torrentSearchBatch // 最多八个未过期快照。
}

// newTorrentSearchManager 创建独立有界客户端，不复用 qB 或浏览器 Cookie。
func newTorrentSearchManager(config TorrentSearchConfig) *torrentSearchManager {
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 8 * time.Second, MaxResponseHeaderBytes: 32 << 10, MaxIdleConns: 8, MaxIdleConnsPerHost: 3, IdleConnTimeout: 60 * time.Second}
	return &torrentSearchManager{config: config, http: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, searchSlots: make(chan struct{}, 2), downloadSlots: make(chan struct{}, 2), batches: []torrentSearchBatch{}}
}

// snapshotConfig 在短锁内取得不可变来源快照，网络调用使用副本。
func (m *torrentSearchManager) snapshotConfig() (TorrentSearchConfig, uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	config := m.config
	config.Sources = append([]TorrentSearchSource{}, config.Sources...)
	return config, m.generation
}

// replaceConfig 在调用方持有下载命令锁时切换来源，已持久绑定的添加仍可按原请求重试。
func (m *torrentSearchManager) replaceConfig(config TorrentSearchConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	m.generation++
	m.batches = []torrentSearchBatch{}
}

// torrentSearchRoutes 注册管理员同源接口，搜种开关不影响原下载和媒体接口。
func (a *App) torrentSearchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/torrent-search/status", a.auth(a.torrentSearchStatus))
	mux.HandleFunc("POST /api/torrent-search", a.auth(a.searchTorrents))
}

// torrentSearchStatus 只返回已配置来源的显示信息，不探测外网或公开凭据。
func (a *App) torrentSearchStatus(w http.ResponseWriter, r *http.Request) {
	type sourceView struct {
		ID   string `json:"id"`   // 可供筛选的来源编号。
		Name string `json:"name"` // 来源显示名。
	}
	sources := []sourceView{}
	config, _ := a.torrentSearch.snapshotConfig()
	for _, source := range config.Sources {
		sources = append(sources, sourceView{ID: source.ID, Name: source.Name})
	}
	respond(w, 200, map[string]any{"enabled": config.Enabled, "sources": sources, "maxResults": maxTorrentSearchResults, "resultTtlSeconds": int(torrentSearchTTL.Seconds())})
}

// searchTorrents 聚合有限来源，失败独立呈现，不把全部离线伪装成空结果。
func (a *App) searchTorrents(w http.ResponseWriter, r *http.Request) {
	m := a.torrentSearch
	config, generation := m.snapshotConfig()
	if !config.Enabled {
		fail(w, 503, "搜种尚未启用，请按部署文档配置 Torznab 来源")
		return
	}
	var input struct {
		Query    string `json:"query"`    // 用户关键词，去除首尾空白后限 2–120 字符。
		SourceID string `json:"sourceId"` // 来源编号，空值搜索所有来源。
		Category string `json:"category"` // all、video、music 或 book。
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Query = strings.TrimSpace(input.Query)
	categories := map[string]string{"": "", "all": "", "video": "2000,5000", "music": "3000", "book": "7000"}
	category, categoryOK := categories[input.Category]
	if !validSearchText(input.Query, 2, 120) || !categoryOK {
		fail(w, 400, "请输入 2–120 个字符的关键词并选择有效类别")
		return
	}
	selected := []TorrentSearchSource{}
	for _, source := range config.Sources {
		if input.SourceID == "" || input.SourceID == source.ID {
			selected = append(selected, source)
		}
	}
	if len(selected) == 0 {
		fail(w, 400, "搜种来源不存在，请刷新来源列表")
		return
	}
	select {
	case m.searchSlots <- struct{}{}:
		defer func() { <-m.searchSlots }()
	default:
		fail(w, 429, "同时搜索过多，请等待当前搜索结束后重试")
		return
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	type sourceResult struct {
		Values    []torrentSearchPrivateResult // 单个来源的有限解析结果。
		Err       error                        // 不含原始 URL 或响应的中文错误。
		Truncated bool                         // 来源达到一百条预算。
	}
	results := make([]sourceResult, len(selected))
	semaphore := make(chan struct{}, 3)
	var workers sync.WaitGroup
	for index, source := range selected {
		workers.Add(1)
		go func(index int, source TorrentSearchSource) {
			defer workers.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results[index].Err = fmt.Errorf("搜索超时或已取消，请稍后重试")
				return
			}
			results[index].Values, results[index].Truncated, results[index].Err = m.querySource(ctx, source, input.Query, category)
		}(index, source)
	}
	workers.Wait()
	if r.Context().Err() != nil {
		return
	}
	response := torrentSearchResponse{Results: []TorrentSearchResult{}, Sources: []TorrentSearchSourceStatus{}}
	candidates := []torrentSearchPrivateResult{}
	succeeded := 0
	for index, result := range results {
		source := selected[index]
		status := TorrentSearchSourceStatus{ID: source.ID, Name: source.Name, State: "ready"}
		if result.Err != nil {
			status.State = "error"
			status.Error = result.Err.Error()
			a.logger.Warn("搜种来源请求失败", "sourceId", source.ID, "durationMs", time.Since(started).Milliseconds())
		} else {
			succeeded++
		}
		response.Sources = append(response.Sources, status)
		response.Truncated = response.Truncated || result.Truncated
		candidates = append(candidates, result.Values...)
	}
	if succeeded == 0 {
		fail(w, 502, "所有搜种来源均暂不可用，请检查索引服务、密钥和连接后重试")
		return
	}
	private, limited := dedupeTorrentSearchResults(candidates)
	response.Truncated = response.Truncated || limited
	expires := time.Now().Add(torrentSearchTTL)
	for index := range private {
		id, err := newTorrentSearchResultID(expires)
		if err != nil {
			fail(w, 500, "生成搜索结果编号失败，请重新搜索")
			return
		}
		private[index].View.ID = id
		response.Results = append(response.Results, private[index].View)
	}
	response.ExpiresAt = expires.UTC().Format(time.RFC3339)
	m.mu.Lock()
	if m.generation != generation {
		m.mu.Unlock()
		fail(w, 409, "搜种来源已更新，请重新搜索")
		return
	}
	m.purge(time.Now())
	if len(m.batches) >= maxTorrentSearchBatches {
		m.batches = m.batches[1:]
	}
	m.batches = append(m.batches, torrentSearchBatch{Expires: expires, Results: private})
	m.mu.Unlock()
	a.logger.Info("搜种已完成", "sourceCount", len(selected), "successCount", succeeded, "resultCount", len(private), "durationMs", time.Since(started).Milliseconds())
	respond(w, 200, response)
}

// dedupeTorrentSearchResults 合并跨来源身份别名，完成桥接后再限制五百条输出。
func dedupeTorrentSearchResults(values []torrentSearchPrivateResult) ([]torrentSearchPrivateResult, bool) {
	parents := make([]int, len(values))
	preferred := make([]torrentSearchPrivateResult, len(values))
	v1Hashes := make([]string, len(values))
	v2Hashes := make([]string, len(values))
	seen := make(map[string]int)
	for index, value := range values {
		parents[index] = index
		preferred[index] = value
		v1Hashes[index], v2Hashes[index] = value.V1, value.V2
		root := index
		for _, key := range value.dedupeKeys() {
			if previous, exists := seen[key]; exists {
				previous = torrentSearchRoot(parents, previous)
				root = torrentSearchRoot(parents, root)
				if previous != root {
					// 上游声明尚未通过种子内容校验，同算法身份冲突时不得隐藏独立资源。
					if v1Hashes[previous] != "" && v1Hashes[root] != "" && v1Hashes[previous] != v1Hashes[root] || v2Hashes[previous] != "" && v2Hashes[root] != "" && v2Hashes[previous] != v2Hashes[root] {
						continue
					}
					first, later := root, previous
					if first > later {
						first, later = later, first
					}
					// hybrid 同时连上已有 v1/v2 分组时合并全部别名，并优先保留可安全下载的来源。
					if !preferred[first].View.Downloadable && preferred[later].View.Downloadable {
						preferred[first] = preferred[later]
					}
					parents[later] = first
					if v1Hashes[first] == "" {
						v1Hashes[first] = v1Hashes[later]
					}
					if v2Hashes[first] == "" {
						v2Hashes[first] = v2Hashes[later]
					}
					root = first
				}
			} else {
				seen[key] = root
			}
		}
	}
	results := []torrentSearchPrivateResult{}
	limited := false
	for index := range values {
		if torrentSearchRoot(parents, index) != index {
			continue
		}
		if len(results) >= maxTorrentSearchResults {
			limited = true
			continue
		}
		results = append(results, preferred[index])
	}
	return results, limited
}

// torrentSearchRoot 压缩有界去重分组的路径，让已保留的所有身份别名指向同一结果。
func torrentSearchRoot(parents []int, index int) int {
	for parents[index] != index {
		parents[index] = parents[parents[index]]
		index = parents[index]
	}
	return index
}

// dedupeKeys 区分 v1/v2 算法，无身份时仅使用服务端来源摘要，不按同名资源合并。
func (result torrentSearchPrivateResult) dedupeKeys() []string {
	keys := []string{}
	if result.V1 != "" {
		keys = append(keys, "v1:"+result.V1)
	}
	if result.V2 != "" {
		keys = append(keys, "v2:"+result.V2)
	}
	if len(keys) == 0 && result.Key != "" {
		keys = append(keys, result.Source.ID+":"+result.Key)
	}
	return keys
}

// newTorrentSearchResultID 生成含有限到期信息的 128 位随机令牌，不编码来源或凭据。
func newTorrentSearchResultID(expires time.Time) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return strconv.FormatInt(expires.Unix(), 10) + "-" + hex.EncodeToString(random[:]), nil
}

// parseTorrentSearchResultID 校验令牌形状，只允许标准时间戳与随机十六进制。
func parseTorrentSearchResultID(id string) (time.Time, bool) {
	parts := strings.Split(id, "-")
	if len(parts) != 2 || len(parts[0]) != 10 || len(parts[1]) != 32 {
		return time.Time{}, false
	}
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	_, hexErr := hex.DecodeString(parts[1])
	return time.Unix(seconds, 0), err == nil && hexErr == nil
}

// purge 在调用方持有短期缓存锁时释放过期私密来源，不保留无限墓碑。
func (m *torrentSearchManager) purge(current time.Time) {
	active := make([]torrentSearchBatch, 0, maxTorrentSearchBatches)
	for _, batch := range m.batches {
		if current.Before(batch.Expires) {
			active = append(active, batch)
		}
	}
	m.batches = active
}

// findResult 只在短锁内复制私密材料，过期令牌与未知令牌分别返回明确状态。
func (m *torrentSearchManager) findResult(id string) (torrentSearchPrivateResult, int, string) {
	expires, valid := parseTorrentSearchResultID(id)
	if !valid {
		return torrentSearchPrivateResult{}, 400, "搜索结果编号无效，请重新搜索"
	}
	current := time.Now()
	if !current.Before(expires) {
		return torrentSearchPrivateResult{}, 410, "搜索结果已过期，请重新搜索后添加"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purge(current)
	for _, batch := range m.batches {
		for _, result := range batch.Results {
			if result.View.ID == id {
				return result, 0, ""
			}
		}
	}
	return torrentSearchPrivateResult{}, 404, "搜索结果已失效或服务已重启，请重新搜索"
}

// torrentSearchDigest 把去重材料转换为固定长度摘要，不把私密下载地址输出到结果。
func torrentSearchDigest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
