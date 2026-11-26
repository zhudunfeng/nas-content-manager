package app

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// CollectionConfig 约束可选执行端和服务器侧任务预算。
type CollectionConfig struct {
	WorkerURL         string // NAS_COLLECTION_WORKER_URL：获取执行端地址；为空时仅支持 RSS 与本地只读登记。
	TokenFile         string // NAS_COLLECTION_WORKER_TOKEN_FILE：专用 Bearer 秘密文件，禁止回显内容。
	ReadRoot          string // NAS_COLLECTION_READ_ROOT：执行端数据根的主服务只读映射，包含专属探针与隔离目录。
	MaxBytes          int64  // NAS_COLLECTION_MAX_BYTES：单任务总字节上限，默认 20 GiB。
	MaxFiles          int    // NAS_COLLECTION_MAX_FILES：单任务文件上限，默认 1000。
	AllowHTTP         bool   // NAS_COLLECTION_WORKER_ALLOW_HTTP：明确允许受限内网 HTTP，默认 false。
	UnavailableReason string // 仅受信桌面启动快照提供的不可用中文原因，不从环境或浏览器接收。
}

// loadCollectionConfig 验证成组的执行端设置，不创建目录或读取凭据。
func loadCollectionConfig(source configSource) (CollectionConfig, error) {
	c := CollectionConfig{WorkerURL: strings.TrimRight(source.lookup("NAS_COLLECTION_WORKER_URL"), "/"), TokenFile: source.lookup("NAS_COLLECTION_WORKER_TOKEN_FILE"), ReadRoot: source.lookup("NAS_COLLECTION_READ_ROOT")}
	allow, err := strconv.ParseBool(source.value("NAS_COLLECTION_WORKER_ALLOW_HTTP", "false"))
	if err != nil {
		return c, fmt.Errorf("NAS_COLLECTION_WORKER_ALLOW_HTTP 必须为布尔值")
	}
	c.AllowHTTP = allow
	n, err := source.integer("NAS_COLLECTION_MAX_BYTES", 20*1024*1024*1024, 1, 1024*1024*1024*1024)
	if err != nil {
		return c, err
	}
	c.MaxBytes = int64(n)
	c.MaxFiles, err = source.integer("NAS_COLLECTION_MAX_FILES", 1000, 1, 10000)
	if err != nil {
		return c, err
	}
	if c.WorkerURL == "" && c.TokenFile == "" && c.ReadRoot == "" {
		return c, nil
	}
	u, err := url.Parse(c.WorkerURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") || c.TokenFile == "" || c.ReadRoot == "" {
		return c, fmt.Errorf("资源获取执行端需同时设置有效地址、凭据文件和只读数据根")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && (!c.AllowHTTP || !(u.Hostname() == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsPrivate())) || !strings.Contains(u.Hostname(), "."))) {
		return c, fmt.Errorf("执行端 HTTP 需显式允许且仅限内网，远程连接必须使用 HTTPS")
	}
	if !filepath.IsAbs(c.TokenFile) || !filepath.IsAbs(c.ReadRoot) {
		return c, fmt.Errorf("收集凭据文件和只读映射必须为绝对路径")
	}
	c.ReadRoot = filepath.Clean(c.ReadRoot)
	return c, nil
}
