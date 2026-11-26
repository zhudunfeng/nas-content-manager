package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// nativeBTConfig 保存桌面原生模式的外接 BT 配置，凭据内容仍放在独立私密文件。
type nativeBTConfig struct {
	SchemaVersion int                     `json:"schemaVersion"`       // 独立配置版本，当前仅支持 1，不改变 desktop.json 格式。
	Downloads     *nativeBTDownloadConfig `json:"downloads,omitempty"` // 可选下载配置；省略时保留原环境配置行为。
	Search        *nativeBTSearchConfig   `json:"search,omitempty"`    // 可选搜种配置；搜索和下载可以独立启用。
}

// nativeBTDownloadConfig 只连接用户已经安装的 qBittorrent，不取得其进程所有权。
type nativeBTDownloadConfig struct {
	Mode         string `json:"mode"`                   // external 接入已有引擎，disabled 关闭协调但保留历史目录门禁。
	Root         string `json:"root,omitempty"`         // 应用可读的绝对下载根；只授权其中 complete 子目录供媒体入库。
	EngineRoot   string `json:"engineRoot,omitempty"`   // qB 看到的同一目录；省略时使用 root，支持不同宿主的明确映射。
	URL          string `json:"url,omitempty"`          // qB WebUI 的 HTTP(S) 基地址，不能携带用户名、密码或查询。
	Username     string `json:"username,omitempty"`     // qB WebUI 登录名，与栖库账号独立。
	PasswordFile string `json:"passwordFile,omitempty"` // 密码文件位置，绝对路径或相对当前 profile 的本地路径。
}

// nativeBTSearchConfig 复用现有有界 Torznab 来源文件与结果转下载流程。
type nativeBTSearchConfig struct {
	Mode        string `json:"mode"`                  // external 接入 Jackett/Prowlarr，disabled 关闭搜种。
	SourcesFile string `json:"sourcesFile,omitempty"` // 私密来源 JSON，绝对路径或相对当前 profile 的本地路径。
}

// nativeBTFilePath 按稳定 profile 解析本地配置引用，避免双击时随工作目录变化。
func nativeBTFilePath(profile string, value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("BT 私密文件路径不能为空或包含无效字符")
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	if !filepath.IsLocal(value) {
		return "", errors.New("BT 相对文件路径必须位于当前配置目录内")
	}
	return filepath.Join(profile, value), nil
}

// decodeNativeBTJSON 保留严格字段校验，并区分省略可选字段与误写的 null。
func decodeNativeBTJSON(data []byte, saved *nativeBTConfig) error {
	if !utf8.Valid(data) {
		return errors.New("BT 配置必须使用有效 UTF-8 编码")
	}
	if err := decodeStrictJSON(data, saved); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("BT 配置字段不能为 null")
		}
		if !strings.EqualFold(key, "downloads") && !strings.EqualFold(key, "search") {
			continue
		}
		var section map[string]json.RawMessage
		if err := json.Unmarshal(value, &section); err != nil {
			return err
		}
		for _, entry := range section {
			if bytes.Equal(bytes.TrimSpace(entry), []byte("null")) {
				return errors.New("BT 配置字段不能为 null")
			}
		}
	}
	return nil
}

// loadNativeBTValues 有界读取原生 BT 配置，显式环境参数继续优先且不修改全局环境。
func loadNativeBTValues(profile string, lookup func(string) string) (map[string]string, error) {
	values := make(map[string]string)
	name := filepath.Join(profile, "bt.json")
	info, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<10 {
		return nil, errors.New("本机 bt.json 必须是至多 64 KiB 的普通文件，已保留原文件")
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, errors.New("本机 bt.json 不可读取，请检查配置目录权限")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	closeErr := file.Close()
	tooLarge := len(data) > 64<<10
	var saved nativeBTConfig
	// Windows PowerShell 的 UTF-8 输出可能带 BOM；只接受文件开头的一个 UTF-8 标记。
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if readErr != nil || closeErr != nil || tooLarge || decodeNativeBTJSON(data, &saved) != nil || saved.SchemaVersion != 1 || (saved.Downloads == nil && saved.Search == nil) {
		return nil, errors.New("本机 bt.json 格式或版本无效，已保留原文件；请按 docs/bt-windows.md 修正")
	}
	if download := saved.Downloads; download != nil {
		if download.Mode != "external" && download.Mode != "disabled" {
			return nil, errors.New("原生 BT 下载方式只支持 external 或 disabled；自动初始化请使用 Docker 完整模式")
		}
		root := download.Root
		if value := lookup("NAS_DOWNLOAD_ROOT"); value != "" {
			root = value
		}
		if root != "" {
			if !filepath.IsAbs(root) || strings.ContainsAny(root, "\x00\r\n") {
				return nil, errors.New("原生 BT 下载根必须是明确的绝对路径")
			}
			values["NAS_DOWNLOAD_ROOT"] = filepath.Clean(root)
			values["NAS_DOWNLOAD_ENGINE_ROOT"] = download.EngineRoot
			if download.EngineRoot == "" {
				values["NAS_DOWNLOAD_ENGINE_ROOT"] = values["NAS_DOWNLOAD_ROOT"]
			}
		} else if download.Mode == "external" {
			return nil, errors.New("启用原生 BT 下载必须明确设置下载根 root")
		}
		if download.PasswordFile != "" {
			values["NAS_DOWNLOAD_QB_PASSWORD_FILE"], err = nativeBTFilePath(profile, download.PasswordFile)
			if err != nil {
				return nil, err
			}
		}
		values["NAS_DOWNLOAD_MODE"] = download.Mode
		values["NAS_DOWNLOAD_QB_URL"] = download.URL
		values["NAS_DOWNLOAD_QB_USERNAME"] = download.Username
	}
	if search := saved.Search; search != nil {
		if search.Mode != "external" && search.Mode != "disabled" {
			return nil, errors.New("原生 BT 搜种方式只支持 external 或 disabled；自动初始化请使用 Docker 完整模式")
		}
		values["NAS_TORRENT_SEARCH_MODE"] = search.Mode
		if search.SourcesFile != "" {
			values["NAS_TORRENT_SEARCH_SOURCES_FILE"], err = nativeBTFilePath(profile, search.SourcesFile)
			if err != nil {
				return nil, err
			}
		}
	}
	// 旧布尔开关也是明确的环境选择，不能被文件中的新 mode 字段悄悄覆盖。
	for mode, legacy := range map[string]string{"NAS_DOWNLOAD_MODE": "NAS_DOWNLOAD_ENABLED", "NAS_TORRENT_SEARCH_MODE": "NAS_TORRENT_SEARCH_ENABLED"} {
		if lookup(mode) == "" && lookup(legacy) != "" {
			delete(values, mode)
		}
	}
	for key := range values {
		if value := lookup(key); value != "" {
			values[key] = value
		}
	}
	return values, nil
}
