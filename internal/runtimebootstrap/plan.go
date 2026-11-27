package runtimebootstrap

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"reflect"
	"regexp"
	"strings"

	"nas-content-manager/internal/privateruntime"
)

// digestPattern 仅接受规范小写 SHA256。
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// artifactNamePattern 限制安全阶段名称，避免输出 URL 或控制字符。
var artifactNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,160}$`)

const (
	maxArtifactSize int64 = 512 << 20 // 单个下载材料和单文件最多512MiB。
	maxExpandedSize int64 = 2 << 30   // 一个完整库存或压缩材料解包最多2GiB。
	maxArchiveFiles       = 12000     // 一个材料最多包含12000个归档条目。
)

// fileRecord 绑定一个最终文件的规范路径、内容摘要和精确长度。
type fileRecord struct {
	Path   string `json:"path"`   // 相对代次根的固定路径。
	SHA256 string `json:"sha256"` // 固定文件内容的 SHA256。
	Size   int64  `json:"size"`   // 固定文件字节数。
}

// fileMapping 仅允许从固定材料提取声明的文件。
type fileMapping struct {
	Source      string `json:"source"`      // ZIP 内规范路径；原始文件材料必须为空。
	Destination string `json:"destination"` // 最终库存中已声明的目标路径。
}

// artifact 保存构建时已锁定的可信材料身份。
type artifact struct {
	Name   string        `json:"name"`   // 仅用于脱敏中文阶段的固定名称。
	URL    string        `json:"url"`    // 官方 HTTPS 下载地址，不从用户输入获取。
	SHA256 string        `json:"sha256"` // 原始下载材料的精确摘要。
	Size   int64         `json:"size"`   // 原始下载材料的精确长度。
	Format string        `json:"format"` // 仅允许 zip 或 raw。
	Files  []fileMapping `json:"files"`  // 精确文件映射，禁止自动扫描式安装。
}

// recipe 保存编译期唯一安装配方；不是外部可修改的配置。
type recipe struct {
	SchemaVersion         int          `json:"schemaVersion"`         // 固定配方版本1。
	RuntimeManifestSHA256 string       `json:"runtimeManifestSha256"` // 随包清单精确字节摘要。
	PayloadSHA256         string       `json:"payloadSha256"`         // 小型已装配材料包摘要。
	Artifacts             []artifact   `json:"artifacts"`             // 固定官方下载材料。
	Tools                 []fileRecord `json:"tools"`                 // 运行时清单以外的全部工具及许可文件。
}

// installationPlan 是编译期配方经过闭包校验后的只读执行计划。
type installationPlan struct {
	identity    string                // 配方精确字节摘要，绑定选择指针。
	artifacts   []artifact            // 已校验的固定下载材料。
	files       map[string]fileRecord // 全部最终文件库存，不含两份固定元数据。
	directories map[string]bool       // 库存允许的完整目录闭包。
	payload     map[string]*zip.File  // 内嵌材料中需要还原的文件。
}

// compiledPlan 对每个插件分别缓存不可变配方，不能混用另一个插件的材料。
func (e *Engine) compiledPlan() (*installationPlan, error) {
	e.planOnce.Do(func() {
		if e.initErr != nil {
			e.planErr = e.initErr
			return
		}
		e.plan, e.planErr = e.parsePlan()
	})
	return e.plan, e.planErr
}

// parsePlan 拒绝配方、库存、目录或材料映射的任何不完整闭包。
func (e *Engine) parsePlan() (*installationPlan, error) {
	failure := errors.New("程序内置组件依赖材料不完整或不一致，请安装匹配的程序包")
	var compiled recipe
	if len(e.config.Recipe) > 4<<20 || len(e.config.Runtime) > 4<<20 || len(e.config.Payload) > 16<<20 || decodeStrict(e.config.Recipe, &compiled) != nil || compiled.SchemaVersion != 1 || digestBytes(e.config.Runtime) != compiled.RuntimeManifestSHA256 || digestBytes(e.config.Payload) != compiled.PayloadSHA256 || len(compiled.Artifacts) == 0 || len(compiled.Artifacts) > 100 {
		return nil, failure
	}
	// 完整运行时合同仍由各插件严格加载器检查，此处仅组合只读文件库存。
	var inventory struct {
		Files []fileRecord `json:"files"` // 已锁定的运行时及许可库存。
	}
	if json.Unmarshal(e.config.Runtime, &inventory) != nil || len(inventory.Files) == 0 || len(inventory.Files)+len(compiled.Tools) > maxArchiveFiles {
		return nil, failure
	}
	plan := &installationPlan{identity: digestBytes(e.config.Recipe), artifacts: compiled.Artifacts, files: map[string]fileRecord{}, directories: map[string]bool{}}
	aliases := map[string]string{}
	var size int64
	for _, item := range append(inventory.Files, compiled.Tools...) {
		key := strings.ToLower(item.Path)
		if !privateruntime.SafeRelative(item.Path) || !digestPattern.MatchString(item.SHA256) || item.Size < 0 || item.Size > maxArtifactSize || aliases[key] != "" || key == strings.ToLower(e.config.ManifestName) || key == "bootstrap-receipt.json" || key == ".downloads" || strings.HasPrefix(key, ".downloads/") {
			return nil, failure
		}
		size += item.Size
		if size > maxExpandedSize {
			return nil, failure
		}
		aliases[key] = item.Path
		plan.files[item.Path] = item
	}
	for name := range plan.files {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			key := strings.ToLower(parent)
			if canonical := aliases[key]; canonical != "" && canonical != parent {
				return nil, failure
			}
			if _, isFile := plan.files[parent]; isFile {
				return nil, failure
			}
			aliases[key] = parent
			plan.directories[parent] = true
		}
	}
	for _, name := range e.config.RequiredFiles {
		if _, exists := plan.files[name]; !exists {
			return nil, failure
		}
	}
	payload, err := zip.NewReader(bytes.NewReader(e.config.Payload), int64(len(e.config.Payload)))
	if err != nil {
		return nil, failure
	}
	plan.payload, err = indexArchive(payload)
	if err != nil {
		return nil, failure
	}
	sources := map[string]bool{}
	for name, member := range plan.payload {
		record, found := plan.files[name]
		if !found || member.UncompressedSize64 != uint64(record.Size) {
			return nil, failure
		}
		sources[name] = true
	}
	names := map[string]bool{}
	for _, source := range plan.artifacts {
		if !artifactNamePattern.MatchString(source.Name) || names[source.Name] || !validArtifactURL(source.URL) || !digestPattern.MatchString(source.SHA256) || source.Size <= 0 || source.Size > maxArtifactSize || len(source.Files) == 0 || len(source.Files) > maxArchiveFiles || source.Format != "zip" && source.Format != "raw" {
			return nil, failure
		}
		names[source.Name] = true
		if source.Format == "raw" && (len(source.Files) != 1 || source.Files[0].Source != "") {
			return nil, failure
		}
		members := map[string]bool{}
		for _, mapping := range source.Files {
			_, found := plan.files[mapping.Destination]
			if !found || sources[mapping.Destination] || source.Format == "zip" && (!privateruntime.SafeRelative(mapping.Source) || members[strings.ToLower(mapping.Source)]) {
				return nil, failure
			}
			members[strings.ToLower(mapping.Source)] = true
			sources[mapping.Destination] = true
		}
	}
	if len(sources) != len(plan.files) {
		return nil, failure
	}
	return plan, nil
}

// digestBytes 计算固定内嵌材料的规范 SHA256。
func digestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// decodeStrict 拒绝重复键、未知字段和多个 JSON 值。
func decodeStrict(data []byte, destination any) error {
	unique := json.NewDecoder(bytes.NewReader(data))
	if err := privateruntime.UniqueValue(unique, 0); err != nil {
		return errors.New("组件依赖元数据 JSON 字段重复或结构无效")
	}
	if _, err := unique.Token(); !errors.Is(err, io.EOF) {
		return errors.New("组件依赖元数据包含额外 JSON 内容")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return errors.New("组件依赖元数据字段无效")
	}
	// JSON 标准库按大小写不敏感匹配字段；精确往返同时拒绝别名、缺失和 null。
	encoded, err := json.Marshal(destination)
	var supplied any
	var canonical any
	if err != nil || json.Unmarshal(data, &supplied) != nil || json.Unmarshal(encoded, &canonical) != nil || !reflect.DeepEqual(supplied, canonical) {
		return errors.New("组件依赖元数据字段拼写、必填值或类型无效")
	}
	return nil
}

// archivePath 记录显式或隐式目录，防止大小写和文件目录别名。
type archivePath struct {
	name     string // 归档中的规范拼写。
	dir      bool   // 是否为目录或隐式父目录。
	explicit bool   // 是否已出现对应的显式 ZIP 条目。
}

// indexArchive 在提取任何文件前检查全部条目、别名、链接及解包预算。
func indexArchive(reader *zip.Reader) (map[string]*zip.File, error) {
	failure := errors.New("组件依赖压缩材料存在不安全路径、重复条目或超出预算")
	if len(reader.File) > maxArchiveFiles {
		return nil, failure
	}
	seen := map[string]archivePath{}
	files := map[string]*zip.File{}
	var expanded uint64
	for _, member := range reader.File {
		directory := strings.HasSuffix(member.Name, "/")
		name := member.Name
		if directory {
			name = strings.TrimSuffix(name, "/")
		}
		mode := member.Mode()
		if !privateruntime.SafeRelative(name) || member.Flags&1 != 0 || directory && !mode.IsDir() || !directory && !mode.IsRegular() || member.UncompressedSize64 > uint64(maxArtifactSize) || directory && member.UncompressedSize64 != 0 {
			return nil, failure
		}
		expanded += member.UncompressedSize64
		if expanded > uint64(maxExpandedSize) {
			return nil, failure
		}
		key := strings.ToLower(name)
		if previous, exists := seen[key]; exists && (previous.name != name || previous.explicit || previous.dir != directory) {
			return nil, failure
		}
		seen[key] = archivePath{name: name, dir: directory, explicit: true}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			key = strings.ToLower(parent)
			if previous, exists := seen[key]; exists {
				if previous.name != parent || !previous.dir {
					return nil, failure
				}
			} else {
				seen[key] = archivePath{name: parent, dir: true}
			}
		}
		if !directory {
			files[name] = member
		}
	}
	return files, nil
}
