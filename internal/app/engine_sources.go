package app

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"nas-content-manager/internal/enginecontract"
)

var managedIndexerPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`) // 上游索引器路径只能含安全片段，禁止浏览器提供 URL。

// managedSourceChoice 是可持久保存的非敏感索引器选择，不含端点或密钥。
type managedSourceChoice struct {
	ID      string `json:"id"`      // 对外稳定来源编号。
	Name    string `json:"name"`    // 用户可见纯文本名称。
	Indexer string `json:"indexer"` // 已验证的上游固定路径片段。
}

// managedSourceSelection 只保存来源选择，运行时从私有连接注入最新密钥。
type managedSourceSelection struct {
	InstallationID string                `json:"installationId"` // 所属安装身份。
	ContainerID    string                `json:"containerId"`    // 所属 Jackett 实例。
	Sources        []managedSourceChoice `json:"sources"`        // 最多八个明确选定的来源。
}

// engineSourceView 是可供浏览器选择的安全来源展示。
type engineSourceView struct {
	ID   string `json:"id"`   // 稳定来源编号，不含私有地址。
	Name string `json:"name"` // 经过长度和控制字符检查的名称。
}

// engineSourceResponse 区分尚无站点、需要选择与已接入，绝不返回密钥。
type engineSourceResponse struct {
	Candidates        []engineSourceView `json:"candidates"`        // 最多128个安全候选。
	SelectedIDs       []string           `json:"selectedIds"`       // 当前选定来源编号。
	SelectionRequired bool               `json:"selectionRequired"` // 超过八个候选时必须由用户选择。
	State             string             `json:"state"`             // ready/configuration_required。
	Message           string             `json:"message"`           // 受控中文说明。
}

// engineSourceError 携带受控连接状态，既不透传上游响应也不触发应用会话退出。
type engineSourceError struct {
	State   string // auth_failed/incompatible/error。
	Message string // 已脱敏的中文说明。
}

// Error 返回可安全显示的来源连接说明。
func (e *engineSourceError) Error() string { return e.Message }

// sourceSelectionPath 返回主应用中仅含非敏感选择的固定文件。
func (a *App) sourceSelectionPath() string {
	return filepath.Join(a.config.DataDir, "engine-search-sources.json")
}

// managedSourcesConfig 验证安全选择并派生各单索引器端点，不使用 all 聚合搜索。
func managedSourcesConfig(connection enginecontract.Connection, choices []managedSourceChoice) (TorrentSearchConfig, error) {
	c := TorrentSearchConfig{Mode: "managed", Enabled: len(choices) > 0, Sources: []TorrentSearchSource{}}
	if len(choices) > 8 {
		return c, fmt.Errorf("最多接入八个搜索来源")
	}
	seen := map[string]bool{}
	for _, choice := range choices {
		if !validTorrentSourceID(choice.ID) || !validSearchText(choice.Name, 1, 80) || !managedIndexerPattern.MatchString(choice.Indexer) || seen[choice.ID] {
			return c, fmt.Errorf("已保存来源选择无效")
		}
		seen[choice.ID] = true
		c.Sources = append(c.Sources, TorrentSearchSource{ID: choice.ID, Name: choice.Name, URL: strings.TrimRight(connection.URL, "/") + "/api/v2.0/indexers/" + choice.Indexer + "/results/torznab/api", APIKey: connection.APIKey})
	}
	return c, nil
}

// restoreManagedSources 从非敏感选择恢复不可变快照，不在后台轮询时访问站点或自动重新发现。
func (a *App) restoreManagedSources(connection enginecontract.Connection) {
	var selection managedSourceSelection
	err := readEngineJSON(a.config.DataDir, filepath.Base(a.sourceSelectionPath()), &selection)
	if errors.Is(err, os.ErrNotExist) {
		a.setEngineDiagnostic(enginecontract.Jackett, "")
		return
	}
	if err != nil || selection.InstallationID != connection.InstallationID || selection.ContainerID != connection.ContainerID {
		a.setEngineDiagnostic(enginecontract.Jackett, "已保存来源与当前搜索实例不一致，请重新同步来源")
		return
	}
	config, err := managedSourcesConfig(connection, selection.Sources)
	if err != nil {
		a.setEngineDiagnostic(enginecontract.Jackett, err.Error())
		return
	}
	a.downloads.commandMu.Lock()
	a.torrentSearch.replaceConfig(config)
	a.downloads.commandMu.Unlock()
	a.engines.sourcesGeneration = connection.Generation
	a.setEngineDiagnostic(enginecontract.Jackett, "")
	a.logger.Info("已从本地选择恢复托管搜索来源", "sourceCount", len(config.Sources))
}

// discoverJackettSources 通过独立 API Key Torznab 发现请求列出已配置索引器，不执行关键词搜索。
func (a *App) discoverJackettSources(ctx context.Context, connection enginecontract.Connection) ([]managedSourceChoice, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	u, err := url.Parse(strings.TrimRight(connection.URL, "/") + "/api/v2.0/indexers/all/results/torznab/api")
	if err != nil {
		return nil, fmt.Errorf("搜索服务连接配置无效")
	}
	u.RawQuery = url.Values{"t": {"indexers"}, "configured": {"true"}, "apikey": {connection.APIKey}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("来源发现请求无效")
	}
	request.Header.Set("Accept", "application/xml, text/xml")
	response, err := a.torrentSearch.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("搜索服务连接失败或超时，已保留原来源")
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, &engineSourceError{State: "auth_failed", Message: "搜索服务拒绝来源发现认证，已保留原来源"}
	}
	if response.StatusCode != 200 {
		return nil, &engineSourceError{State: "incompatible", Message: "搜索服务暂不支持来源发现，请检查固定版本及服务状态"}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, fmt.Errorf("来源清单读取失败或超过 2 MiB 限额")
	}
	return parseManagedIndexers(data, connection.APIKey)
}

// parseManagedIndexers 限制 XML 形状与预算，拒绝 DTD、异常路径和重复编号。
func parseManagedIndexers(data []byte, apiKey string) ([]managedSourceChoice, error) {
	if err := validateManagedIndexerXML(data); err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, tokens, roots := 0, 0, 0
	choices := []managedSourceChoice{}
	seen := map[string]bool{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("来源发现 XML 格式无效")
		}
		tokens++
		if tokens > 20000 {
			return nil, fmt.Errorf("来源发现超过结构预算")
		}
		switch value := token.(type) {
		case xml.Directive:
			return nil, fmt.Errorf("来源发现不允许 DTD 或外部实体")
		case xml.StartElement:
			depth++
			if depth > 16 || len(value.Attr) > 16 {
				return nil, fmt.Errorf("来源发现嵌套超过限制")
			}
			if depth == 1 {
				roots++
				if roots != 1 || value.Name.Local != "indexers" {
					return nil, fmt.Errorf("来源发现不是有效索引器清单")
				}
			}
			if value.Name.Local != "indexer" || depth != 2 {
				continue
			}
			var indexer struct {
				ID         string `xml:"id,attr"`         // 上游索引器固定路径片段。
				Configured string `xml:"configured,attr"` // 只接纳明确已配置的索引器。
				Title      string `xml:"title"`           // 有界安全显示名称。
			}
			if decoder.DecodeElement(&indexer, &value) != nil {
				return nil, fmt.Errorf("来源发现条目无效")
			}
			depth--
			if indexer.Configured != "true" {
				continue
			}
			// 先完整移除已知密钥，再校验显示名称，避免发现接口把凭据带进浏览器。
			if apiKey != "" {
				indexer.Title = strings.ReplaceAll(indexer.Title, apiKey, "[已隐藏]")
			}
			if !managedIndexerPattern.MatchString(indexer.ID) || !validSearchText(indexer.Title, 1, 80) {
				return nil, fmt.Errorf("来源编号或名称不符合安全限制")
			}
			id := indexer.ID
			if len(id) > 40 || apiKey != "" && strings.Contains(id, apiKey) {
				id = "idx_" + torrentSearchDigest(id)[:32]
			}
			if seen[id] {
				return nil, fmt.Errorf("来源发现存在重复编号")
			}
			seen[id] = true
			choices = append(choices, managedSourceChoice{ID: id, Name: indexer.Title, Indexer: indexer.ID})
			if len(choices) > 128 {
				return nil, fmt.Errorf("来源发现超过128项候选限制")
			}
		case xml.EndElement:
			depth--
		}
	}
	if roots != 1 || depth != 0 {
		return nil, fmt.Errorf("来源发现 XML 不完整")
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].ID < choices[j].ID })
	return choices, nil
}

// validateManagedIndexerXML 在 DecodeElement 前完整遍历令牌，防止嵌套条目绕过预算或 DTD 检查。
func validateManagedIndexerXML(data []byte) error {
	if len(data) > 2<<20 {
		return fmt.Errorf("来源发现超过 2 MiB 限额")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, tokens, roots, indexers := 0, 0, 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("来源发现 XML 格式无效")
		}
		tokens++
		if tokens > 20000 {
			return fmt.Errorf("来源发现超过结构预算")
		}
		switch value := token.(type) {
		case xml.Directive:
			return fmt.Errorf("来源发现不允许 DTD 或外部实体")
		case xml.StartElement:
			depth++
			if depth > 16 || len(value.Attr) > 16 {
				return fmt.Errorf("来源发现嵌套超过限制")
			}
			if depth == 1 {
				roots++
				if roots != 1 || value.Name.Local != "indexers" {
					return fmt.Errorf("来源发现不是有效索引器清单")
				}
			}
			if depth == 2 && value.Name.Local == "indexer" {
				indexers++
				if indexers > 128 {
					return fmt.Errorf("来源发现超过128项候选限制")
				}
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(value)) != "" {
				return fmt.Errorf("来源发现包含额外文档内容")
			}
		}
	}
	if depth != 0 || roots != 1 {
		return fmt.Errorf("来源发现 XML 不完整")
	}
	return nil
}

// engineSyncSources 在用户显式操作时发现与选择来源，失败不修改现有连接。
func (a *App) engineSyncSources(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SourceIDs []string `json:"sourceIds"` // 用户仅能选择服务端发现的安全编号。
	} // 用户仅能选择服务端发现的安全编号。
	if !decodeJSON(w, r, &input) {
		return
	}
	if a.config.TorrentSearch.Mode != "managed" {
		fail(w, 409, "仅托管搜索模式支持自动同步来源")
		return
	}
	if len(input.SourceIDs) > 8 {
		fail(w, 400, "每次最多选择八个搜索来源")
		return
	}
	a.refreshManagedConnections(r.Context())
	e := a.engines
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.jackettConnection == nil {
		fail(w, 503, "搜索引擎连接尚未发布，请先完成初始化")
		return
	}
	connection := *e.jackettConnection
	choices, err := a.discoverJackettSources(r.Context(), connection)
	if err != nil {
		a.setEngineDiagnostic(enginecontract.Jackett, err.Error())
		var sourceErr *engineSourceError
		if errors.As(err, &sourceErr) {
			e.diagnosticStates[enginecontract.Jackett] = sourceErr.State
		}
		fail(w, 502, err.Error())
		return
	}
	response := engineSourceResponse{Candidates: []engineSourceView{}, SelectedIDs: []string{}, State: "configuration_required", Message: "尚未配置站点，请先在 Jackett 管理界面设置管理员密码和来源"}
	lookup := map[string]managedSourceChoice{}
	for _, choice := range choices {
		response.Candidates = append(response.Candidates, engineSourceView{ID: choice.ID, Name: choice.Name})
		lookup[choice.ID] = choice
	}
	if len(choices) > 8 && len(input.SourceIDs) == 0 {
		response.SelectionRequired = true
		response.Message = "发现超过八个来源，请选择最多八个后接入"
		respond(w, 200, response)
		return
	}
	selected := []managedSourceChoice{}
	if len(input.SourceIDs) > 0 {
		seen := map[string]bool{}
		for _, id := range input.SourceIDs {
			choice, ok := lookup[id]
			if !ok || seen[id] {
				fail(w, 400, "所选来源已失效或重复，请重新同步")
				return
			}
			seen[id] = true
			selected = append(selected, choice)
		}
	} else {
		selected = choices
	}
	config, err := managedSourcesConfig(connection, selected)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	selection := managedSourceSelection{InstallationID: connection.InstallationID, ContainerID: connection.ContainerID, Sources: selected}
	// 仅短暂串行持久选择与添加意图，网络发现已经结束；防止旧结果在切换后新建下载。
	a.downloads.commandMu.Lock()
	err = writeEnginePrivateJSON(a.sourceSelectionPath(), selection)
	if err == nil {
		a.torrentSearch.replaceConfig(config)
		e.sourcesGeneration = connection.Generation
	}
	a.downloads.commandMu.Unlock()
	if err != nil {
		fail(w, 500, "保存来源选择失败，已保留原运行配置")
		return
	}
	a.setEngineDiagnostic(enginecontract.Jackett, "")
	for _, choice := range selected {
		response.SelectedIDs = append(response.SelectedIDs, choice.ID)
	}
	if len(selected) > 0 {
		response.State = "ready"
		response.Message = "已接入选定来源，旧搜索结果需重新搜索"
	}
	a.logger.Info("托管搜索来源同步完成", "candidateCount", len(choices), "sourceCount", len(selected))
	respond(w, 200, response)
}
