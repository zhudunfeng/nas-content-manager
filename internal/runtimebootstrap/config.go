package runtimebootstrap

import (
	"bytes"
	"errors"
	"path"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"nas-content-manager/internal/privateruntime"
)

// Config 仅由编译期固定插件构造，不能从环境、磁盘配置或浏览器请求生成。
type Config struct {
	Label           string             // 固定中文插件名称，仅用于脱敏进度和错误。
	Namespace       string             // profile 下独立的固定安装目录，同时绑定归属标记。
	ManifestName    string             // 代次根中固定且不含目录的运行时清单名称。
	Recipe          []byte             // 构建时嵌入的固定来源、摘要、长度和映射。
	Runtime         []byte             // 构建时验证的插件完整运行时清单原始字节。
	Payload         []byte             // 构建时嵌入的小文件和许可载荷。
	RequiredFiles   []string           // 插件另外要求必须存在的工具库存，不改变配方闭包。
	ValidateRuntime func(string) error // 对应插件原有严格加载器，不得降低来源或隔离校验。
}

// Engine 保存单个固定插件独立的只读材料和解析缓存。
type Engine struct {
	config   Config            // 构造时复制材料字节，调用方之后不能改变身份。
	initErr  error             // 固定配置错误，在任何目录或网络副作用前拒绝。
	planOnce sync.Once         // 当前插件配方只解析一次，插件之间不共享状态。
	plan     *installationPlan // 成功后不可修改的完整执行计划。
	planErr  error             // 同步固定缓存的配方错误。
}

// namespacePattern 只允许一层有界固定目录，禁止路径分隔、设备名和元数据别名。
var namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)

// New 固化插件材料并验证配置边界；实际文件和来源闭包在首次使用时检查。
func New(config Config) *Engine {
	config.Recipe = bytes.Clone(config.Recipe)
	config.Runtime = bytes.Clone(config.Runtime)
	config.Payload = bytes.Clone(config.Payload)
	config.RequiredFiles = append([]string(nil), config.RequiredFiles...)
	e := &Engine{config: config}
	manifestName := strings.ToLower(config.ManifestName)
	if !namespacePattern.MatchString(config.Namespace) || !privateruntime.SafeRelative(config.Namespace) || !privateruntime.SafeRelative(config.ManifestName) || path.Base(config.ManifestName) != config.ManifestName || manifestName == "bootstrap-receipt.json" || manifestName == "current.json" || manifestName == "owner.json" || manifestName == ".downloads" || len([]rune(config.Label)) == 0 || len([]rune(config.Label)) > 16 || strings.IndexFunc(config.Label, unicode.IsControl) >= 0 || config.ValidateRuntime == nil {
		e.initErr = errors.New("程序内置组件依赖配置无效，请安装匹配的程序包")
	}
	return e
}

// ownerMarker 保留原 UAV 归属字节，同时让各插件的独立命名空间不可互相接管。
func (e *Engine) ownerMarker() []byte {
	return []byte("{\"schemaVersion\":1,\"component\":\"" + e.config.Namespace + "\"}\n")
}

// progressReporter 只将共享文案的固定组件名替换成编译期中文名称。
func (e *Engine) progressReporter(report func(Progress)) func(Progress) {
	if report == nil {
		return nil
	}
	return func(progress Progress) {
		progress.Stage = strings.ReplaceAll(progress.Stage, "组件", e.config.Label)
		report(progress)
	}
}

// labeledError 只改变可显示文案，保留取消、完整性和清理错误的原始关联。
type labeledError struct {
	message string // 不含网络地址、凭据或本地目录的错误文案。
	err     error  // errors.Is/As 仍可访问的原始错误链。
}

// Error 返回已经按固定插件名称转换的中文错误。
func (e *labeledError) Error() string { return e.message }

// Unwrap 保留真实错误链，防止将清理失败或取消误判为普通准备失败。
func (e *labeledError) Unwrap() error { return e.err }

// translateError 在公开操作边界转换固定文案，不丢弃任何底层错误身份。
func (e *Engine) translateError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ReplaceAll(err.Error(), "组件", e.config.Label)
	if message == err.Error() {
		return err
	}
	return &labeledError{message: message, err: err}
}
