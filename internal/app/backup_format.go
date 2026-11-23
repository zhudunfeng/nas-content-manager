package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	backupMarkerName    = ".nas-backup-store.json"           // 根命名空间标记的固定名称。
	backupMarkerTemp    = ".nas-backup-store.json.tmp"       // 初始化残留不得自动接管的唯一临时名。
	backupLockName      = ".nas-backup.lock"                 // 永久保留的空内核锁文件。
	backupMarkerMagic   = "nas-content-manager-backup-store" // 根标记格式身份。
	backupOwnerMagic    = "nas-content-manager-backup-owner" // 文件所有权格式身份。
	backupJSONLimit     = 1024                               // marker和owner原始UTF-8字节上限。
	backupManifestLimit = 16384                              // manifest原始UTF-8字节上限。
	backupTimestampMax  = 253402300799999                    // SQLite契约允许的UTC毫秒上界。
)

// backupMarker 只标识逻辑命名空间，不代表物理介质或跨克隆全局锁。
type backupMarker struct {
	Magic         string `json:"magic"`         // 固定根格式魔数。
	FormatVersion int64  `json:"formatVersion"` // 精确整数格式版本1。
	StoreID       string `json:"storeId"`       // 数据库内随机小写32hex命名空间。
}

// backupOwner 固定字段顺序生成登记意图前的原始所有权字节。
type backupOwner struct {
	Magic         string `json:"magic"`         // 固定所有权魔数。
	FormatVersion int64  `json:"formatVersion"` // 精确格式版本1。
	StoreID       string `json:"storeId"`       // 所属备份存储。
	RunID         string `json:"runId"`         // 唯一执行编号。
	FileID        string `json:"fileId"`        // 唯一文件编号及目录名来源。
}

// backupManifestFile 表达唯一数据库成员的实际检查结果。
type backupManifestFile struct {
	Role   string `json:"role"`   // 固定database角色。
	Name   string `json:"name"`   // 固定snapshot.db成员名。
	Size   int64  `json:"size"`   // 实测正字节数。
	SHA256 string `json:"sha256"` // 实测小写SHA256。
}

// backupManifest 表达数据库备份范围，禁止包含私密路径或凭据。
type backupManifest struct {
	FormatVersion int64                `json:"formatVersion"` // 精确格式版本1。
	StoreID       string               `json:"storeId"`       // 根的逻辑命名空间。
	RunID         string               `json:"runId"`         // 已登记执行编号。
	FileID        string               `json:"fileId"`        // 已登记文件编号。
	Scope         string               `json:"scope"`         // 固定database-only。
	CreatedAt     string               `json:"createdAt"`     // 源生成完成的UTC规范时间。
	AppVersion    string               `json:"appVersion"`    // 当前程序的有界版本文本。
	SchemaVersion int64                `json:"schemaVersion"` // 目标实际数据库头。
	Files         []backupManifestFile `json:"files"`         // 精确一个数据库成员。
	ExcludedRoles []string             `json:"excludedRoles"` // 固定顺序七项未包含材料。
}

// backupExcludedRoles 每次返回独立的固定排除角色，不允许调用者修改共享数组。
func backupExcludedRoles() []string {
	return []string{"source-secrets", "helper-identity", "worker-state", "component-volumes", "download-payloads", "media", "user-assets"}
}

// backupHex 校验精确字节数的小写ASCII十六进制值。
func backupHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// backupDigest 对已经有界的原始元数据字节计算摘要，不对重新编码内容冒称原摘要。
func backupDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// backupOwnerBytes 在登记pending前固定规范所有权字节，无额外换行。
func backupOwnerBytes(storeID string, runID string, fileID string) ([]byte, error) {
	if !backupHex(storeID, 32) || !backupHex(runID, 32) || !backupHex(fileID, 32) {
		return nil, errors.New("备份所有权编号无效")
	}
	return json.Marshal(backupOwner{backupOwnerMagic, 1, storeID, runID, fileID})
}

// backupManifestBytes 用实际源或目标事实产生规范清单，不能用编译头代替实测头。
func backupManifestBytes(storeID string, runID string, fileID string, metadata snapshotMetadata) ([]byte, error) {
	if _, err := backupOwnerBytes(storeID, runID, fileID); err != nil {
		return nil, err
	}
	created := metadata.CreatedAt.UTC().Format(time.RFC3339Nano)
	if !backupValidTime(created) || metadata.Scope != "database-only" || metadata.SchemaVersion <= 0 || metadata.SizeBytes <= 0 || !backupHex(metadata.SHA256, 64) || !backupVersionText(Version) {
		return nil, errors.New("备份清单事实无效")
	}
	return json.Marshal(backupManifest{1, storeID, runID, fileID, "database-only", created, Version, int64(metadata.SchemaVersion), []backupManifestFile{{"database", "snapshot.db", metadata.SizeBytes, metadata.SHA256}}, backupExcludedRoles()})
}

// backupVersionText 限定非空UTF-8版本文本，不接受NUL或解码替代字符。
func backupVersionText(value string) bool {
	return len(value) >= 1 && len(value) <= 64 && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\ufffd")
}

// backupValidTime 限定UTC规范文本及存储可表示的毫秒区间。
func backupValidTime(value string) bool {
	t, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && strings.HasSuffix(value, "Z") && t.UTC().Format(time.RFC3339Nano) == value && t.Year() >= 1970 && t.Year() <= 9999 && t.UnixMilli() >= 0 && t.UnixMilli() <= backupTimestampMax
}

// backupJSONValue 逐token保留已解码键集合，拒绝转义重复键及过深形状。
func backupJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 5 {
		return nil, errors.New("备份JSON嵌套超出范围")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				keyToken, e := decoder.Token()
				if e != nil {
					return nil, e
				}
				key, ok := keyToken.(string)
				if !ok || strings.ContainsAny(key, "\x00\ufffd") {
					return nil, errors.New("备份JSON键无效")
				}
				if _, exists := object[key]; exists {
					return nil, errors.New("备份JSON键重复")
				}
				member, e := backupJSONValue(decoder, depth+1)
				if e != nil {
					return nil, e
				}
				object[key] = member
			}
			end, e := decoder.Token()
			if e != nil || end != json.Delim('}') {
				return nil, errors.New("备份JSON对象未结束")
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for decoder.More() {
				member, e := backupJSONValue(decoder, depth+1)
				if e != nil {
					return nil, e
				}
				array = append(array, member)
				if len(array) > 7 {
					return nil, errors.New("备份JSON数组超出范围")
				}
			}
			end, e := decoder.Token()
			if e != nil || end != json.Delim(']') {
				return nil, errors.New("备份JSON数组未结束")
			}
			return array, nil
		}
	case string:
		if strings.ContainsAny(value, "\x00\ufffd") {
			return nil, errors.New("备份JSON文本无效")
		}
		return value, nil
	case json.Number:
		text := string(value)
		if len(text) == 0 || len(text) > 19 || len(text) > 1 && text[0] == '0' {
			return nil, errors.New("备份JSON整数无效")
		}
		for _, c := range text {
			if c < '0' || c > '9' {
				return nil, errors.New("备份JSON整数无效")
			}
		}
		integer, e := strconv.ParseInt(text, 10, 64)
		if e != nil {
			return nil, errors.New("备份JSON整数溢出")
		}
		return integer, nil
	}
	return nil, errors.New("备份JSON值类型无效")
}

// backupJSONObject 对有界UTF-8原始字节只接受恰一个完整对象。
func backupJSONObject(data []byte, limit int, keys ...string) (map[string]any, error) {
	if len(data) == 0 || len(data) > limit || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, errors.New("备份JSON原始字节无效")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := backupJSONValue(decoder, 0)
	if err != nil {
		return nil, errors.New("备份JSON格式无效")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, errors.New("备份JSON含尾随内容")
	}
	object, ok := value.(map[string]any)
	if !ok || !backupExactKeys(object, keys...) {
		return nil, errors.New("备份JSON字段无效")
	}
	return object, nil
}

// backupExactKeys 精确检查大小写及必填字段，不依赖宽松结构体匹配。
func backupExactKeys(object map[string]any, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}

// parseBackupMarker 验证精确根格式及逻辑命名空间。
func parseBackupMarker(data []byte) (backupMarker, error) {
	o, err := backupJSONObject(data, backupJSONLimit, "magic", "formatVersion", "storeId")
	if err != nil {
		return backupMarker{}, err
	}
	id, _ := o["storeId"].(string)
	if o["magic"] != backupMarkerMagic || o["formatVersion"] != int64(1) || !backupHex(id, 32) {
		return backupMarker{}, errors.New("备份根标记无效")
	}
	return backupMarker{backupMarkerMagic, 1, id}, nil
}

// parseBackupOwner 验证精确所有权格式，原始字节摘要由调用方另行保留。
func parseBackupOwner(data []byte) (backupOwner, error) {
	o, err := backupJSONObject(data, backupJSONLimit, "magic", "formatVersion", "storeId", "runId", "fileId")
	if err != nil {
		return backupOwner{}, err
	}
	s, _ := o["storeId"].(string)
	r, _ := o["runId"].(string)
	f, _ := o["fileId"].(string)
	if o["magic"] != backupOwnerMagic || o["formatVersion"] != int64(1) || !backupHex(s, 32) || !backupHex(r, 32) || !backupHex(f, 32) {
		return backupOwner{}, errors.New("备份所有权格式无效")
	}
	return backupOwner{backupOwnerMagic, 1, s, r, f}, nil
}

// parseBackupManifest 检查清单的每层精确键、整数、固定数组和成员范围。
func parseBackupManifest(data []byte) (backupManifest, error) {
	failure := errors.New("备份清单格式无效")
	o, err := backupJSONObject(data, backupManifestLimit, "formatVersion", "storeId", "runId", "fileId", "scope", "createdAt", "appVersion", "schemaVersion", "files", "excludedRoles")
	if err != nil {
		return backupManifest{}, err
	}
	s, _ := o["storeId"].(string)
	r, _ := o["runId"].(string)
	f, _ := o["fileId"].(string)
	created, _ := o["createdAt"].(string)
	version, _ := o["appVersion"].(string)
	schema, _ := o["schemaVersion"].(int64)
	if o["formatVersion"] != int64(1) || !backupHex(s, 32) || !backupHex(r, 32) || !backupHex(f, 32) || o["scope"] != "database-only" || !backupValidTime(created) || !backupVersionText(version) || schema <= 0 {
		return backupManifest{}, failure
	}
	files, ok := o["files"].([]any)
	if !ok || len(files) != 1 {
		return backupManifest{}, failure
	}
	member, ok := files[0].(map[string]any)
	if !ok || !backupExactKeys(member, "role", "name", "size", "sha256") {
		return backupManifest{}, failure
	}
	size, _ := member["size"].(int64)
	digest, _ := member["sha256"].(string)
	if member["role"] != "database" || member["name"] != "snapshot.db" || size <= 0 || !backupHex(digest, 64) {
		return backupManifest{}, failure
	}
	excluded, ok := o["excludedRoles"].([]any)
	roles := backupExcludedRoles()
	if !ok || len(excluded) != len(roles) {
		return backupManifest{}, failure
	}
	for i, role := range roles {
		if excluded[i] != role {
			return backupManifest{}, failure
		}
	}
	return backupManifest{1, s, r, f, "database-only", created, version, schema, []backupManifestFile{{"database", "snapshot.db", size, digest}}, roles}, nil
}
