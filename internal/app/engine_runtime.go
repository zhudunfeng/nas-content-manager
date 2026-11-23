package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"nas-content-manager/internal/enginecontract"
)

const managedCheckInterval = 5 * time.Second // 托管清单轮询间隔，不请求部署或索引站点。

var engineIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`) // 安装与容器身份只接受有界安全字符。

// engineRuntime 保护托管激活与诊断；网络调用不占用业务锁或数据库事务。
type engineRuntime struct {
	mu                 sync.Mutex                 // 串行清单激活和显式来源同步，不与媒体扫描共享。
	http               *http.Client               // 只通过部署固定的 Unix socket 调用助手。
	installation       string                     // 已接受的稳定安装身份，不静默接管另一安装。
	qbConnection       *enginecontract.Connection // 已验证的固定下载实例，初始化后不可在线替换。
	jackettConnection  *enginecontract.Connection // 最近验证的搜索连接，仅服务器内存保存。
	diagnosticStates   map[string]string          // 下载认证或协议失败的有限状态。
	diagnostics        map[string]string          // 有限中文控制面诊断，不包含原始上游错误。
	sourcesGeneration  int64                      // 当前选定来源对应的连接版本。
	manifestGeneration int64                      // 已接受的清单版本，拒绝运行中回退。
	nextDownloadProbe  time.Time                  // 认证或离线失败后限速，避免轮询触发上游封禁。
}

// startEngineRuntime 初始化控制面并独立轮询私有清单；默认部署没有后台 Docker 请求。
func (a *App) startEngineRuntime() {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", a.config.Engines.Socket)
	}, ResponseHeaderTimeout: 25 * time.Second, MaxResponseHeaderBytes: 32 << 10, MaxIdleConns: 2, IdleConnTimeout: 30 * time.Second}
	// 资源组件只读检测最多等待22秒，私有传输预算需覆盖完整探测及有界回执读取。
	a.engines = &engineRuntime{http: &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, diagnostics: map[string]string{}, diagnosticStates: map[string]string{}}
	if a.config.Downloads.Mode != "managed" && a.config.TorrentSearch.Mode != "managed" {
		return
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer a.engines.http.CloseIdleConnections()
		timer := time.NewTicker(managedCheckInterval)
		defer timer.Stop()
		a.refreshManagedConnections(a.ctx)
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-timer.C:
				a.refreshManagedConnections(a.ctx)
			}
		}
	}()
}

// readEngineJSON 通过目录能力有界读取固定普通文件，拒绝软链接、额外字段与开放权限。
func readEngineJSON(directory string, name string, output any) error {
	if filepath.Base(name) != name {
		return fmt.Errorf("连接文件名称无效")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<10 || (runtime.GOOS != "windows" && info.Mode().Perm()&0007 != 0) {
		return fmt.Errorf("连接文件类型、大小或权限无效")
	}
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return fmt.Errorf("连接文件超过限制")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(output) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("连接文件格式无效")
	}
	return nil
}

// readManagedConnection 只从固定组件和正整数版本派生文件名，清单不能指定任意文件。
func (a *App) readManagedConnection(manifest enginecontract.Manifest, component string, generation int64) (enginecontract.Connection, error) {
	var c enginecontract.Connection
	if generation < 1 || generation > manifest.Generation {
		return c, fmt.Errorf("连接版本无效")
	}
	if err := readEngineJSON(a.config.Engines.ConnectionDir, fmt.Sprintf("%s-%d.json", component, generation), &c); err != nil {
		return c, fmt.Errorf("托管连接文件暂不可用")
	}
	u, err := url.Parse(c.URL)
	if c.Version != 1 || c.InstallationID != manifest.InstallationID || c.Component != component || c.Generation != generation || !engineIdentityPattern.MatchString(c.ContainerID) || err != nil || !validSearchHTTPURL(u) || u.RawQuery != "" || u.ForceQuery || u.Path != "" && u.Path != "/" {
		return c, fmt.Errorf("托管连接内容无效")
	}
	if component == enginecontract.QBittorrent && (!validSearchText(c.Username, 1, 200) || !validSearchText(c.Password, 1, 4096) || c.APIKey != "") {
		return c, fmt.Errorf("托管下载凭据无效")
	}
	if component == enginecontract.Jackett && (!validSearchText(c.APIKey, 1, 4096) || c.Password != "" || c.Username != "") {
		return c, fmt.Errorf("托管搜索凭据无效")
	}
	return c, nil
}

// setEngineDiagnostic 只记录受控中文消息，同样状态不重复刷日志。
func (a *App) setEngineDiagnostic(component string, message string) {
	if a.engines.diagnostics[component] == message {
		return
	}
	a.engines.diagnostics[component] = message
	delete(a.engines.diagnosticStates, component)
	if message != "" {
		a.logger.Warn("托管引擎连接待处理", "component", component, "message", message)
	}
}

// refreshManagedConnections 验证完整清单后逐项接入，错误保留此前已经验证的运行快照。
func (a *App) refreshManagedConnections(ctx context.Context) {
	e := a.engines
	e.mu.Lock()
	defer e.mu.Unlock()
	var manifest enginecontract.Manifest
	if err := readEngineJSON(a.config.Engines.ConnectionDir, "current.json", &manifest); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			a.setEngineDiagnostic("manifest", "托管连接清单不可读取或格式无效，已保留原连接")
		}
		return
	}
	if manifest.Version != 1 || manifest.Generation < 1 || manifest.Generation < e.manifestGeneration || manifest.QBGeneration < 0 || manifest.JackettGeneration < 0 || !engineIdentityPattern.MatchString(manifest.InstallationID) || e.installation != "" && manifest.InstallationID != e.installation {
		a.setEngineDiagnostic("manifest", "托管安装身份或版本不一致，请恢复原安装状态")
		return
	}
	// 下载实例身份同时持久保存在应用私密小文件，防止应用重启后把旧任务发给新实例。
	if a.config.Downloads.Mode == "managed" && manifest.QBGeneration > 0 {
		connection, err := a.readManagedConnection(manifest, enginecontract.QBittorrent, manifest.QBGeneration)
		if err != nil {
			a.setEngineDiagnostic(enginecontract.QBittorrent, err.Error())
		} else if e.qbConnection == nil && !time.Now().Before(e.nextDownloadProbe) {
			e.nextDownloadProbe = time.Now().Add(30 * time.Second)
			if err = a.activateManagedDownload(ctx, connection); err != nil {
				a.setEngineDiagnostic(enginecontract.QBittorrent, err.Error())
				var engineErr *downloadEngineError
				if errors.As(err, &engineErr) {
					a.engines.diagnosticStates[enginecontract.QBittorrent] = engineErr.State
					if engineErr.State == "offline" {
						a.engines.diagnosticStates[enginecontract.QBittorrent] = "error"
					}
				}
			} else {
				e.qbConnection = &connection
				e.installation = manifest.InstallationID
				a.setEngineDiagnostic(enginecontract.QBittorrent, "")
			}
		} else if e.qbConnection != nil && (e.qbConnection.ContainerID != connection.ContainerID || e.qbConnection.InstallationID != connection.InstallationID || e.qbConnection.URL != connection.URL || e.qbConnection.Password != connection.Password) {
			a.setEngineDiagnostic(enginecontract.QBittorrent, "下载实例或凭据已改变，请先核对历史任务；运行连接未被替换")
		}
	}
	if a.config.TorrentSearch.Mode == "managed" && manifest.JackettGeneration > 0 {
		connection, err := a.readManagedConnection(manifest, enginecontract.Jackett, manifest.JackettGeneration)
		if err != nil {
			a.setEngineDiagnostic(enginecontract.Jackett, err.Error())
		} else if e.jackettConnection == nil || e.jackettConnection.Generation != connection.Generation {
			e.jackettConnection = &connection
			e.installation = manifest.InstallationID
			a.restoreManagedSources(connection)
		}
	}
	e.manifestGeneration = manifest.Generation
	a.setEngineDiagnostic("manifest", "")
}

// managedDownloadIdentity 是无密钥的实例绑定，不改变媒体数据库结构。
type managedDownloadIdentity struct {
	InstallationID string `json:"installationId"` // 已接入安装身份。
	ContainerID    string `json:"containerId"`    // 已接入下载实例身份。
	Root           string `json:"root"`           // 固定应用下载根。
}

// activateManagedDownload 通过生产适配器认证后一次激活，不放宽固定版本元数据保护。
func (a *App) activateManagedDownload(ctx context.Context, connection enginecontract.Connection) error {
	identityPath := filepath.Join(a.config.DataDir, "engine-download-identity.json")
	var old managedDownloadIdentity
	err := readEngineJSON(a.config.DataDir, filepath.Base(identityPath), &old)
	var count int
	if a.db.QueryRowContext(ctx, "SELECT count(*) FROM downloads").Scan(&count) != nil {
		return fmt.Errorf("历史下载状态暂不可读取")
	}
	if err == nil && (old.InstallationID != connection.InstallationID || old.ContainerID != connection.ContainerID || old.Root != a.config.Downloads.Root) {
		return fmt.Errorf("托管下载实例与历史绑定不一致，已拒绝自动替换")
	}
	if err != nil && (!errors.Is(err, os.ErrNotExist) || count > 0) {
		return fmt.Errorf("缺少完整下载实例绑定，请恢复原安装状态后重试")
	}
	config := a.config.Downloads
	config.URL = strings.TrimRight(connection.URL, "/")
	config.Username = connection.Username
	config.Enabled = true
	engine := newQBClient(config)
	engine.password = connection.Password
	status, err := engine.probe(ctx)
	if err != nil {
		engine.http.CloseIdleConnections()
		return err
	}
	if strings.TrimPrefix(status.Version, "v") != "5.2.4" {
		engine.http.CloseIdleConnections()
		return fmt.Errorf("托管下载器版本不符合已验证的 5.2.4 元数据保护契约")
	}
	identity := managedDownloadIdentity{InstallationID: connection.InstallationID, ContainerID: connection.ContainerID, Root: config.Root}
	if err = writeEnginePrivateJSON(identityPath, identity); err != nil {
		engine.http.CloseIdleConnections()
		return fmt.Errorf("保存下载实例绑定失败")
	}
	if !a.downloads.activate(engine, status) {
		engine.http.CloseIdleConnections()
		return fmt.Errorf("下载协调器已绑定其它连接")
	}
	a.logger.Info("托管下载引擎已通过生产适配器并接入", "version", status.Version)
	return nil
}

// writeEnginePrivateJSON 原子保存无凭据的绑定或来源选择，避免中断留下半个文档。
func writeEnginePrivateJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".engine-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		directory, openErr := os.Open(filepath.Dir(path))
		if openErr != nil {
			return openErr
		}
		defer directory.Close()
		return directory.Sync()
	}
	return nil
}
