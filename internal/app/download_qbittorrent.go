package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// downloadEngineError 只携带允许对外展示的中文错误，不保留响应体或私有 URL。
type downloadEngineError struct {
	State   string // offline/auth_failed/incompatible。
	Message string // 脱敏中文诊断。
}

// Error 返回经过约束的下载器错误。
func (e *downloadEngineError) Error() string { return e.Message }

// qbClient 对固定基地址执行有界 WebAPI 调用并在后端保留 SID。
type qbClient struct {
	password        string         // 托管模式仅在内存保留的私有连接密码，不写日志或数据库。
	config          DownloadConfig // 固定连接与独立密码文件。
	http            *http.Client   // 有超时且禁止重定向的 HTTP 客户端。
	authMu          sync.Mutex     // 合并登录刷新，避免并发错误触发登录风暴。
	SIDName         string         // 实际会话 Cookie 名称，仅允许 SID 或带合法端口的 QBT_SID_ 名称。
	SID             string         // 仅存于后端内存的会话值。
	lastAuthFailure time.Time      // 认证失败的短期退避时间。
}

// qbTorrent 是 qB 的有限传输快照，hash 作为不透明身份使用。
type qbTorrent struct {
	Hash          string  `json:"hash"`        // 引擎任务标识。
	InfohashV1    string  `json:"infohash_v1"` // 完整 v1 身份，可能由属性接口补齐。
	InfohashV2    string  `json:"infohash_v2"` // 完整 v2 身份，不截断后存储。
	Name          string  `json:"name"`        // 元数据展示名。
	State         string  `json:"state"`       // 引擎状态。
	SavePath      string  `json:"save_path"`   // 引擎实际保存目录。
	Tags          string  `json:"tags"`        // 用于独占所有权的 UUID 标签。
	Size          int64   `json:"size"`        // wanted 文件大小。
	TotalSize     int64   `json:"total_size"`  // 完整元数据总大小。
	Completed     int64   `json:"completed"`   // 当前完成字节。
	Downloaded    int64   `json:"downloaded"`  // 累计传输量，仅供诊断。
	DownloadSpeed int64   `json:"dlspeed"`     // 每秒下载字节。
	UploadSpeed   int64   `json:"upspeed"`     // 每秒上传字节。
	Uploaded      int64   `json:"uploaded"`    // 累计上传字节。
	Ratio         float64 `json:"ratio"`       // 分享率。
	Leechs        int64   `json:"num_leechs"`  // 下载连接数。
	Seeds         int64   `json:"num_seeds"`   // 做种连接数。
	ETA           int64   `json:"eta"`         // 秒数，特殊大值表示未知。
	Progress      float64 `json:"progress"`    // wanted 完成比例。
	AmountLeft    int64   `json:"amount_left"` // wanted 剩余字节。
}

// qbFile 保留引擎真实文件 ID，不能用分页数组位置替代。
type qbFile struct {
	Index    int     `json:"index"`    // 引擎文件 ID。
	Name     string  `json:"name"`     // 相对 save_path 的文件路径。
	Size     int64   `json:"size"`     // 文件字节数。
	Progress float64 `json:"progress"` // 文件完成比例。
	Priority int     `json:"priority"` // 零为不下载。
}

// newQBClient 建立只访问固定地址的有界客户端，不改变 qB 的认证或全局偏好。
func newQBClient(config DownloadConfig) *qbClient {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxIdleConns: 4, MaxIdleConnsPerHost: 4, IdleConnTimeout: 60 * time.Second}
	return &qbClient{config: config, http: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

// login 合并后端会话刷新；鉴权失败与栖库浏览器会话无关。
func (c *qbClient) login(ctx context.Context, oldSID string) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.SID != "" && c.SID != oldSID {
		return nil
	}
	if time.Since(c.lastAuthFailure) < 15*time.Second {
		return &downloadEngineError{State: "auth_failed", Message: "下载器认证失败，请检查后端用户名和密码文件"}
	}
	password := c.password
	var err error
	if password == "" {
		password, err = readDownloadPassword(c.config.PasswordFile)
	}
	if err != nil {
		c.lastAuthFailure = time.Now()
		return &downloadEngineError{State: "auth_failed", Message: "下载器密码文件不可读取或格式无效"}
	}
	form := url.Values{"username": {c.config.Username}, "password": {password}}
	request, err := c.request(ctx, http.MethodPost, "auth/login", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", "", "")
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return engineConnectionError(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(body) > 4096 {
		return &downloadEngineError{State: "offline", Message: "下载器登录响应无效"}
	}
	// 实测 qB 可用 204 空正文返回 SID；成功状态与有效会话必须同时满足，失败正文仍拒绝。
	acceptedStatus := response.StatusCode == http.StatusOK || response.StatusCode == http.StatusNoContent
	if !acceptedStatus || (strings.TrimSpace(string(body)) != "Ok." && strings.TrimSpace(string(body)) != "") {
		c.SIDName = ""
		c.SID = ""
		c.lastAuthFailure = time.Now()
		return &downloadEngineError{State: "auth_failed", Message: "下载器认证失败，请检查后端用户名和密码文件"}
	}
	for _, cookie := range response.Cookies() {
		if validQBSessionCookieName(cookie.Name) && cookie.Value != "" && len(cookie.Value) <= 4096 {
			c.SIDName = cookie.Name
			c.SID = cookie.Value
			c.lastAuthFailure = time.Time{}
			return nil
		}
	}
	c.SIDName = ""
	c.SID = ""
	c.lastAuthFailure = time.Now()
	return &downloadEngineError{State: "auth_failed", Message: "下载器未返回有效会话，请检查 WebUI 认证配置"}
}

// validQBSessionCookieName 限定已支持的会话名称，端口取引擎实际值以兼容反向代理外部端口不同。
func validQBSessionCookieName(name string) bool {
	if name == "SID" {
		return true
	}
	const prefix = "QBT_SID_"
	if !strings.HasPrefix(name, prefix) || len(name) > len(prefix)+5 {
		return false
	}
	suffix := strings.TrimPrefix(name, prefix)
	port, err := strconv.Atoi(suffix)
	return err == nil && port >= 1 && port <= 65535 && strconv.Itoa(port) == suffix
}

// request 为已配置的 API 路径添加正确 Origin/Referer，不允许浏览器透传地址。
func (c *qbClient) request(ctx context.Context, method string, endpoint string, body io.Reader, contentType string, sidName string, sid string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.config.URL+"/api/v2/"+endpoint, body)
	if err != nil {
		return nil, &downloadEngineError{State: "offline", Message: "下载器请求配置无效"}
	}
	u, _ := url.Parse(c.config.URL)
	request.Header.Set("Origin", u.Scheme+"://"+u.Host)
	request.Header.Set("Referer", c.config.URL+"/")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if sid != "" {
		if !validQBSessionCookieName(sidName) {
			return nil, &downloadEngineError{State: "auth_failed", Message: "下载器会话名称无效，请重新测试连接"}
		}
		request.AddCookie(&http.Cookie{Name: sidName, Value: sid})
	}
	return request, nil
}

// call 有界读取响应，最多刷新一次 SID，不记录私有响应或原始网络错误。
func (c *qbClient) call(ctx context.Context, method string, endpoint string, body []byte, contentType string) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		c.authMu.Lock()
		sid := c.SID
		sidName := c.SIDName
		c.authMu.Unlock()
		if sid == "" {
			if err := c.login(ctx, ""); err != nil {
				return nil, err
			}
			c.authMu.Lock()
			sid = c.SID
			sidName = c.SIDName
			c.authMu.Unlock()
		}
		request, err := c.request(ctx, method, endpoint, bytes.NewReader(body), contentType, sidName, sid)
		if err != nil {
			return nil, err
		}
		response, err := c.http.Do(request)
		if err != nil {
			return nil, engineConnectionError(err)
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
		response.Body.Close()
		if readErr != nil || len(data) > 16<<20 {
			return nil, &downloadEngineError{State: "offline", Message: "下载器响应超过预算或读取中断"}
		}
		if response.StatusCode == 401 || response.StatusCode == 403 {
			if attempt == 0 {
				if err = c.login(ctx, sid); err != nil {
					return nil, err
				}
				continue
			}
			return nil, &downloadEngineError{State: "auth_failed", Message: "下载器拒绝已认证请求，请检查凭据与 WebUI 安全配置"}
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 || strings.TrimSpace(string(data)) == "Fails." {
			return nil, &downloadEngineError{State: "offline", Message: "下载器拒绝操作，请检查任务状态、目录权限或协议版本"}
		}
		return data, nil
	}
	return nil, &downloadEngineError{State: "auth_failed", Message: "下载器会话刷新失败"}
}

// engineConnectionError 把可能含主机信息的网络错误映射为有限中文提示。
func engineConnectionError(err error) error {
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return &downloadEngineError{State: "offline", Message: "下载器请求超时，系统将先对账再继续操作"}
	}
	return &downloadEngineError{State: "offline", Message: "下载器暂不可用，正在展示上次同步状态"}
}

// probe 根据实际应用与 WebAPI 版本启用首期 qB 5.x 契约。
func (c *qbClient) probe(ctx context.Context) (DownloadStatus, error) {
	status := DownloadStatus{Enabled: true, State: "connecting"}
	version, err := c.call(ctx, http.MethodGet, "app/version", nil, "")
	if err != nil {
		return status, err
	}
	api, err := c.call(ctx, http.MethodGet, "app/webapiVersion", nil, "")
	if err != nil {
		return status, err
	}
	status.Version = strings.TrimSpace(string(version))
	status.WebAPIVersion = strings.TrimSpace(string(api))
	if len(status.Version) > 64 || len(status.WebAPIVersion) > 64 {
		return status, &downloadEngineError{State: "incompatible", Message: "下载器版本响应无效"}
	}
	parts := strings.Split(strings.TrimPrefix(status.Version, "v"), ".")
	apiParts := strings.Split(status.WebAPIVersion, ".")
	if len(parts) < 3 || parts[0] != "5" || len(apiParts) < 2 || apiParts[0] != "2" {
		return status, &downloadEngineError{State: "incompatible", Message: "首期仅支持 qBittorrent 5.x 与 WebAPI 2.11 及以上版本"}
	}
	minor, err := strconv.Atoi(apiParts[1])
	if err != nil || minor < 11 {
		return status, &downloadEngineError{State: "incompatible", Message: "下载器 WebAPI 版本不支持安全文件选择"}
	}
	status.State = "ready"
	status.LastSyncedAt = now()
	status.CanSelectFiles = true
	return status, nil
}

// torrents 读取完整但有限的身份快照，任务过多时拒绝不完整去重。
func (c *qbClient) torrents(ctx context.Context) ([]qbTorrent, error) {
	data, err := c.call(ctx, http.MethodGet, "torrents/info?limit="+strconv.Itoa(maxEngineTasks+1), nil, "")
	if err != nil {
		return nil, err
	}
	var torrents []qbTorrent
	if json.Unmarshal(data, &torrents) != nil || torrents == nil || len(torrents) > maxEngineTasks {
		return nil, &downloadEngineError{State: "incompatible", Message: "下载器任务快照无效或超过 2048 项对账预算"}
	}
	for _, torrent := range torrents {
		if torrent.Hash == "" || len(torrent.Hash) > 128 || len(torrent.Tags) > 65536 {
			return nil, &downloadEngineError{State: "incompatible", Message: "下载器返回无效任务身份"}
		}
	}
	return torrents, nil
}

// properties 补齐 v1/v2 算法身份，不把纯 v2 的引擎截短标识误当 SHA-1。
func (c *qbClient) properties(ctx context.Context, torrent *qbTorrent) error {
	data, err := c.call(ctx, http.MethodGet, "torrents/properties?hash="+url.QueryEscape(torrent.Hash), nil, "")
	if err != nil {
		return err
	}
	var value struct {
		V1 string `json:"infohash_v1"` /* v1 SHA-1。 */
		V2 string `json:"infohash_v2"` /* v2 SHA-256。 */
	}
	if json.Unmarshal(data, &value) != nil {
		return &downloadEngineError{State: "incompatible", Message: "下载器身份属性无效"}
	}
	torrent.InfohashV1 = strings.ToLower(value.V1)
	torrent.InfohashV2 = strings.ToLower(value.V2)
	return nil
}

// files 从引擎取得真实文件 ID，统一执行与上传解析相同的路径和整数边界校验。
func (c *qbClient) files(ctx context.Context, hash string) ([]DownloadFile, error) {
	data, err := c.call(ctx, http.MethodGet, "torrents/files?hash="+url.QueryEscape(hash), nil, "")
	if err != nil {
		return nil, err
	}
	var values []qbFile
	if json.Unmarshal(data, &values) != nil {
		return nil, fmt.Errorf("下载器文件清单无效")
	}
	files := make([]DownloadFile, 0, len(values))
	for _, value := range values {
		if value.Progress < 0 || value.Progress > 1 || value.Priority < 0 {
			return nil, fmt.Errorf("下载器文件进度无效")
		}
		files = append(files, DownloadFile{Index: value.Index, Path: value.Name, SizeBytes: value.Size, Selected: value.Priority > 0, Progress: value.Progress})
	}
	if len(files) == 0 {
		return files, nil
	}
	if err = validateDownloadFiles(files); err != nil {
		return nil, err
	}
	return files, nil
}

// form 执行仅由后端选定的任务级命令，不修改全局下载偏好。
func (c *qbClient) form(ctx context.Context, endpoint string, values url.Values) error {
	_, err := c.call(ctx, http.MethodPost, "torrents/"+endpoint, []byte(values.Encode()), "application/x-www-form-urlencoded")
	return err
}

// add 将本地种子停止添加；磁力采用元数据停止条件且关闭自动目录管理。
func (c *qbClient) add(ctx context.Context, d downloadRecord, source []byte) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{"savepath": engineTaskPath(c.config, "staging/"+d.ID), "autoTMM": "false", "tags": "nas-" + d.ID, "stopped": "true", "contentLayout": "Original"}
	if d.SourceKind == "magnet" {
		fields["urls"] = string(source)
		fields["stopped"] = "false"
		fields["stopCondition"] = "MetadataReceived"
	} else {
		part, err := writer.CreateFormFile("torrents", "source.torrent")
		if err != nil {
			return fmt.Errorf("准备种子请求失败")
		}
		if _, err = part.Write(source); err != nil {
			return fmt.Errorf("准备种子请求失败")
		}
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return fmt.Errorf("准备下载请求失败")
		}
	}
	if writer.Close() != nil {
		return fmt.Errorf("准备下载请求失败")
	}
	_, err := c.call(ctx, http.MethodPost, "torrents/add", body.Bytes(), writer.FormDataContentType())
	return err
}

// hasDownloadTag 按完整标签比较所有权，禁止相似前缀接管外部任务。
func hasDownloadTag(tags string, id string) bool {
	for _, tag := range strings.Split(tags, ",") {
		if strings.TrimSpace(tag) == "nas-"+id {
			return true
		}
	}
	return false
}
