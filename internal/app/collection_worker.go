package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nas-content-manager/internal/collectioncontract"
)

// collectionWorkerCall 通过固定认证端点调用执行端，错误不回显凭据和工具输出。
func (a *App) collectionWorkerCall(ctx context.Context, method string, endpoint string, input any, output any) (int, error) {
	if a.config.Collection.WorkerURL == "" {
		return 503, collectionProblem(503, "尚未配置资源获取执行端")
	}
	token, err := os.ReadFile(a.config.Collection.TokenFile)
	if err != nil || len(token) > 4096 || len(bytes.TrimSpace(token)) < 24 {
		return 503, collectionProblem(503, "资源获取执行端凭据不可用")
	}
	var body io.Reader
	if input != nil {
		data, e := json.Marshal(input)
		if e != nil {
			return 500, e
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, a.config.Collection.WorkerURL+endpoint, body)
	if err != nil {
		return 503, collectionProblem(503, "执行端配置不可用")
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := a.collections.client.Do(request)
	if err != nil {
		return 503, collectionProblem(503, "资源获取执行端暂不可达，请保留原请求编号后重试")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	if err != nil || len(data) > 2<<20 {
		return 502, collectionProblem(502, "执行端响应超过安全限制")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == 404 {
			return 404, collectionProblem(404, "执行端未找到该操作")
		}
		if response.StatusCode == 409 {
			return 409, collectionProblem(409, "执行端计划冲突或预览已过期，请重新预览")
		}
		if response.StatusCode == 422 {
			return 422, collectionProblem(422, "执行端无法处理该资源，请检查来源、工具或网页支持情况")
		}
		return response.StatusCode, collectionProblem(502, "资源获取执行端拒绝操作，请检查连接、工具与限额")
	}
	if output != nil && json.Unmarshal(data, output) != nil {
		return 502, collectionProblem(502, "执行端响应格式无效")
	}
	return response.StatusCode, nil
}

// collectionWorkerReady 检测真实能力及专属目录映射，不写入用户媒体。
func (a *App) collectionWorkerReady(ctx context.Context) (collectioncontract.Capabilities, error) {
	var caps collectioncontract.Capabilities
	if _, err := a.collectionWorkerCall(ctx, "GET", "/v1/capabilities", nil, &caps); err != nil {
		return caps, err
	}
	if caps.Version != collectioncontract.Version || caps.InstanceID == "" || len(caps.Capabilities) > 8 || len(caps.Sources) > 64 || caps.MaxBytes < 1 || caps.MaxFiles < 1 {
		return caps, collectionProblem(503, "执行端协议或能力清单不兼容")
	}
	nonce, err := newDownloadUUID()
	if err != nil {
		return caps, err
	}
	var probe collectioncontract.Probe
	if _, err = a.collectionWorkerCall(ctx, "GET", "/v1/probe?nonce="+url.QueryEscape(nonce), nil, &probe); err != nil {
		return caps, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_, _ = a.collectionWorkerCall(cleanup, "DELETE", "/v1/probe?nonce="+url.QueryEscape(nonce), nil, nil)
	}()
	if probe.Nonce != nonce || probe.RelativePath != "probes/"+nonce {
		return caps, collectionProblem(503, "执行端目录探针不匹配")
	}
	root, err := os.OpenRoot(a.config.Collection.ReadRoot)
	if err != nil {
		return caps, collectionProblem(503, "执行端只读数据根不可访问")
	}
	defer root.Close()
	file, err := root.Open(probe.RelativePath)
	if err != nil {
		return caps, collectionProblem(503, "执行端写入区与主服务只读映射不一致")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 128))
	if err != nil || string(data) != nonce {
		return caps, collectionProblem(503, "执行端目录映射挑战失败")
	}
	return caps, nil
}

// collectionValidateFiles 通过只读映射核对精确产物，拒绝链接、越界和非媒体输出。
func (a *App) collectionValidateFiles(root string, relativeDir string, files []CollectionFile, contentType string) (int64, error) {
	if len(files) == 0 || len(files) > a.config.Collection.MaxFiles {
		return 0, fmt.Errorf("产物数量不符合任务预算")
	}
	if _, err := collectionRelative(relativeDir); err != nil {
		return 0, fmt.Errorf("产物目录无效")
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return 0, fmt.Errorf("产物只读映射不可访问")
	}
	defer handle.Close()
	seen := map[string]bool{}
	var total int64
	for _, file := range files {
		safe, e := collectionRelative(file.Path)
		supported := matchesType(contentType, safe) || contentType == "comic" && isImage(safe)
		if contentType == "" {
			for _, kind := range []string{"video", "music", "image", "comic", "novel"} {
				supported = supported || matchesType(kind, safe)
			}
		}
		if e != nil || safe == "" || seen[downloadPathKey(safe)] || file.Size < 1 || file.ModTime < 1 || !supported {
			return 0, fmt.Errorf("产物清单包含重复、不安全或不匹配的媒体")
		}
		seen[downloadPathKey(safe)] = true
		current := filepath.Join(root, filepath.FromSlash(relativeDir), filepath.FromSlash(safe))
		for target := current; ; target = filepath.Dir(target) {
			info, statErr := os.Lstat(target)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
				return 0, fmt.Errorf("产物包含链接或无法访问的路径")
			}
			if target == root {
				break
			}
			if !within(root, target) {
				return 0, fmt.Errorf("产物越过只读根")
			}
		}
		opened, e := handle.Open(filepath.Join(filepath.FromSlash(relativeDir), filepath.FromSlash(safe)))
		if e != nil {
			return 0, fmt.Errorf("产物无法打开")
		}
		info, e := opened.Stat()
		_ = opened.Close()
		if e != nil || !info.Mode().IsRegular() || info.Size() != file.Size || info.ModTime().UnixNano() != file.ModTime {
			return 0, fmt.Errorf("产物版本与执行端清单不一致")
		}
		if file.Size > a.config.Collection.MaxBytes-total {
			return 0, fmt.Errorf("产物超过任务字节预算")
		}
		total += file.Size
	}
	return total, nil
}

// collectionWorkerFiles 将协议清单转换为主服务不可变发布依据。
func collectionWorkerFiles(files []collectioncontract.File) []CollectionFile {
	out := make([]CollectionFile, 0, len(files))
	for _, f := range files {
		out = append(out, CollectionFile{Path: f.Path, Size: f.Size, ModTime: f.ModTime, SHA256: f.SHA256})
	}
	return out
}
