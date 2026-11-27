// Package runtimebootstrap 在桌面自有目录还原固定组件依赖，不使用系统 Python 或 pip。
package runtimebootstrap

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"

	"nas-content-manager/internal/privateruntime"
)

// Paths 保存已校验依赖的固定绝对路径，不能由浏览器选择。
type Paths struct {
	Manifest string // 插件固定名称的私有运行时清单。
	Root     string // 通过完整闭包验证的不可变代次根。
}

// Progress 保存当前材料的真实下载进度，不含 URL、路径或凭据。
type Progress struct {
	Stage           string // 有界中文阶段说明。
	DownloadedBytes int64  // 当前材料已下载的实际字节数。
	TotalBytes      int64  // 当前材料锁定的完整字节数；非下载阶段为零。
}

// currentSelection 是唯一可发布选择，不能指向暂存目录或其他配方。
type currentSelection struct {
	SchemaVersion int    `json:"schemaVersion"` // 固定元数据版本1。
	RecipeSHA256  string `json:"recipeSha256"`  // 编译期完整配方字节摘要。
	Generation    string `json:"generation"`    // 固定前缀和随机身份组成的自有代次。
}

// generationPattern 限定已发布代次，拒绝路径、别名和未完成暂存。
var generationPattern = regexp.MustCompile(`^gen-[0-9a-f]{32}$`)

// maxNativePathUTF16Units 在实测250单元加载边界内保留2单元余量，不依赖系统长路径策略。
const maxNativePathUTF16Units = 248

// ErrCleanup 标识自有暂存或元数据未完整清理，调用方不能将其当作普通下载失败。
var ErrCleanup = errors.New("组件依赖暂存或资源未能完整清理")

// Resolve 只读校验当前已发布依赖；调用方必须持有所属 profile 单实例租约。
func (e *Engine) Resolve(root string) (paths Paths, resultErr error) {
	defer func() { resultErr = e.translateError(resultErr) }()
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return Paths{}, errors.New("组件依赖自动初始化目前仅支持 Windows x64")
	}
	plan, err := e.compiledPlan()
	if err != nil {
		return Paths{}, err
	}
	base, err := e.ownedRoot(root, false)
	if err != nil {
		return Paths{}, err
	}
	selection, err := readSelection(filepath.Join(base, "current.json"))
	if err != nil || selection.RecipeSHA256 != plan.identity {
		return Paths{}, errors.New("组件自动初始化文件尚未准备或与当前程序不匹配")
	}
	final := filepath.Join(base, selection.Generation)
	if err := validateNativePaths(final, plan); err != nil {
		return Paths{}, err
	}
	return e.validateGeneration(final, plan)
}

// Initialize 下载并还原锁定依赖；调用方须串行调用，并提供真实离线进程探测。
func (e *Engine) Initialize(ctx context.Context, root string, report func(Progress), probe func(context.Context, Paths) error) (paths Paths, resultErr error) {
	defer func() { resultErr = e.translateError(resultErr) }()
	report = e.progressReporter(report)
	return e.initialize(ctx, root, report, probe, e.materialize)
}

// initialize 统一执行发布状态机，内部还原器参数用于离线验证同一条生产发布路径。
func (e *Engine) initialize(ctx context.Context, root string, report func(Progress), probe func(context.Context, Paths) error, materialize func(context.Context, string, *installationPlan, func(Progress)) error) (paths Paths, resultErr error) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return Paths{}, errors.New("组件依赖自动初始化目前仅支持 Windows x64")
	}
	if probe == nil {
		return Paths{}, errors.New("组件依赖初始化缺少离线探测，已拒绝安装")
	}
	if err := ctx.Err(); err != nil {
		return Paths{}, err
	}
	plan, err := e.compiledPlan()
	if err != nil {
		return Paths{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	progress(report, "正在检查已准备的组件依赖", 0, 0)
	if existing, resolveErr := e.Resolve(root); resolveErr == nil {
		if probeErr := probe(ctx, existing); probeErr == nil {
			return existing, nil
		} else if ctx.Err() != nil || errors.Is(probeErr, ErrCleanup) {
			return Paths{}, errors.Join(probeErr, ctx.Err())
		}
	}
	base, err := e.ownedRoot(root, true)
	if err != nil {
		return Paths{}, err
	}
	identity := make([]byte, 16)
	if _, err = rand.Read(identity); err != nil {
		return Paths{}, errors.New("无法生成组件依赖暂存身份")
	}
	id := hex.EncodeToString(identity)
	final := filepath.Join(base, "gen-"+id)
	if err := validateNativePaths(final, plan); err != nil {
		return Paths{}, err
	}
	stage := filepath.Join(base, "stage-"+id)
	if err = os.Mkdir(stage, 0700); err != nil {
		return Paths{}, errors.New("无法创建组件依赖暂存目录，请检查空间和权限")
	}
	owned := stage
	selected := false
	defer func() {
		if !selected {
			if cleanupErr := removeOwnedAttempt(base, owned, id); cleanupErr != nil {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
	}()
	if err = materialize(ctx, stage, plan, report); err != nil {
		return Paths{}, err
	}
	progress(report, "正在校验组件依赖完整性", 0, 0)
	paths, err = e.validateGeneration(stage, plan)
	if err != nil {
		return Paths{}, err
	}
	if err = ctx.Err(); err != nil {
		return Paths{}, err
	}
	if err = os.Rename(stage, final); err != nil {
		return Paths{}, errors.New("无法提交已校验的组件依赖，请检查空间和权限")
	}
	owned = final
	paths, err = e.validateGeneration(final, plan)
	if err != nil {
		return Paths{}, err
	}
	if err = ctx.Err(); err != nil {
		return Paths{}, err
	}
	progress(report, "正在离线探测最终组件依赖", 0, 0)
	// 必须在最终不可变路径执行真实探测；暂存目录更短，不能代表 Windows DLL 加载成功。
	// 最终目录尚未被 current 选中，探测失败仍只清理本次自有代次。
	if err = probe(ctx, paths); err != nil {
		return Paths{}, err
	}
	if err = ctx.Err(); err != nil {
		return Paths{}, err
	}
	selection := currentSelection{SchemaVersion: 1, RecipeSHA256: plan.identity, Generation: filepath.Base(final)}
	if err = publishSelection(base, selection); err != nil {
		// 原子替换失败时只接受确切读回的本次选择，不能猜测发布成功。
		observed, readErr := readSelection(filepath.Join(base, "current.json"))
		if readErr != nil || observed != selection {
			return Paths{}, err
		}
		selected = true
		if errors.Is(err, ErrCleanup) {
			return Paths{}, err
		}
	}
	selected = true
	progress(report, "组件依赖已完成初始化和离线校验", 0, 0)
	return paths, nil
}

// progress 只回调固定中文阶段，下载计数来自真实流读取。
func progress(report func(Progress), stage string, downloaded int64, total int64) {
	if report != nil {
		report(Progress{Stage: stage, DownloadedBytes: downloaded, TotalBytes: total})
	}
}

// ownedRoot 验证父目录和固定归属标记，拒绝认领未知既有目录。
func (e *Engine) ownedRoot(root string, create bool) (string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.HasPrefix(root, `\\`) || privateruntime.RejectAliases(root) != nil {
		return "", errors.New("组件依赖目录必须位于已核验的本机专用目录")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return "", errors.New("组件依赖所属目录不存在或类型无效")
	}
	base := filepath.Join(root, e.config.Namespace)
	created := false
	if create {
		if err = os.Mkdir(base, 0700); err == nil {
			created = true
		} else if !errors.Is(err, os.ErrExist) {
			return "", errors.New("无法创建组件依赖专用目录，请检查空间和权限")
		}
	}
	if privateruntime.RejectAliases(base) != nil {
		return "", errors.New("组件依赖目录缺失或包含链接、重解析点")
	}
	info, err = os.Lstat(base)
	if err != nil || !info.IsDir() {
		return "", errors.New("组件依赖专用目录类型无效")
	}
	marker := filepath.Join(base, "owner.json")
	if created {
		if err = writeNewFile(marker, e.ownerMarker()); err != nil {
			if cleanupErr := os.Remove(base); cleanupErr != nil {
				return "", errors.Join(err, ErrCleanup, errors.New("组件依赖目录归属记录失败，且本次空目录未能清理"))
			}
			return "", errors.Join(err, errors.New("无法完整记录组件依赖目录归属"))
		}
	} else {
		if privateruntime.RejectAliases(marker) != nil {
			return "", errors.New("既有组件依赖目录归属不明，拒绝接管")
		}
		data, readErr := privateruntime.ReadOrdinary(marker, 1024)
		if readErr != nil || !bytes.Equal(data, e.ownerMarker()) {
			return "", errors.New("既有组件依赖目录归属不明，拒绝接管")
		}
	}
	return base, nil
}

// readSelection 严格读取有界代次指针，不接受暂存、重复键或额外字段。
func readSelection(path string) (currentSelection, error) {
	if privateruntime.RejectAliases(path) != nil {
		return currentSelection{}, errors.New("组件依赖选择文件缺失或包含链接")
	}
	data, err := privateruntime.ReadOrdinary(path, 4096)
	var selection currentSelection
	if err != nil || decodeStrict(data, &selection) != nil || selection.SchemaVersion != 1 || !digestPattern.MatchString(selection.RecipeSHA256) || !generationPattern.MatchString(selection.Generation) {
		return currentSelection{}, errors.New("组件依赖选择文件格式无效")
	}
	return selection, nil
}

// publishSelection 先完整持久化新选择，再原子替换，不删除旧指针。
func publishSelection(base string, selection currentSelection) (resultErr error) {
	target := filepath.Join(base, "current.json")
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() || info.Size() > 4096 || privateruntime.RejectAliases(target) != nil {
			return errors.New("既有组件依赖选择文件类型异常，拒绝覆盖")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("无法核验组件依赖选择文件")
	}
	data, err := json.Marshal(selection)
	if err != nil {
		return errors.New("无法编码组件依赖选择")
	}
	temporary := filepath.Join(base, "current-"+strings.TrimPrefix(selection.Generation, "gen-")+".tmp")
	if err = writeNewFile(temporary, append(data, '\n')); err != nil {
		return errors.Join(err, errors.New("无法完整写入组件依赖选择"))
	}
	defer func() {
		if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, ErrCleanup, errors.New("组件依赖选择临时文件未能清理"))
		}
	}()
	if privateruntime.RejectAliases(base) != nil || replaceSelection(temporary, target) != nil {
		return errors.New("无法确认组件依赖选择已原子发布")
	}
	return nil
}

// validateGeneration 同时校验固定清单、额外工具和完整目录闭包。
func (e *Engine) validateGeneration(root string, plan *installationPlan) (Paths, error) {
	if privateruntime.RejectAliases(root) != nil {
		return Paths{}, errors.New("组件依赖代次目录包含链接或重解析点")
	}
	manifest := filepath.Join(root, e.config.ManifestName)
	data, err := privateruntime.ReadOrdinary(manifest, 4<<20)
	if err != nil || !bytes.Equal(data, e.config.Runtime) {
		return Paths{}, errors.New("组件依赖运行时清单与当前程序不一致")
	}
	receipt, err := privateruntime.ReadOrdinary(filepath.Join(root, "bootstrap-receipt.json"), 4<<20)
	if err != nil || !bytes.Equal(receipt, e.config.Recipe) {
		return Paths{}, errors.New("组件依赖来源记录与当前程序不一致")
	}
	seen := make(map[string]bool, len(plan.files)+2)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || privateruntime.RejectAliases(path) != nil {
			return errors.New("组件依赖目录包含不可读路径、链接或重解析点")
		}
		if path == root {
			return nil
		}
		local, relativeErr := filepath.Rel(root, path)
		name := filepath.ToSlash(local)
		if relativeErr != nil || !privateruntime.SafeRelative(name) {
			return errors.New("组件依赖文件越出专用目录")
		}
		if entry.IsDir() {
			if !plan.directories[name] {
				return errors.New("组件依赖包含未声明目录")
			}
			return nil
		}
		if name == e.config.ManifestName || name == "bootstrap-receipt.json" {
			seen[name] = true
			return nil
		}
		record, found := plan.files[name]
		if !found || seen[name] {
			return errors.New("组件依赖包含未声明文件或路径别名")
		}
		if err := privateruntime.VerifyFile(path, name, record.SHA256, record.Size); err != nil {
			return errors.New("组件依赖文件完整性或架构校验失败")
		}
		seen[name] = true
		return nil
	})
	if err != nil || len(seen) != len(plan.files)+2 {
		return Paths{}, errors.Join(errors.New("组件依赖文件库存不完整或包含异常内容"), err)
	}
	if err = e.config.ValidateRuntime(manifest); err != nil {
		return Paths{}, errors.New("组件私有运行时严格来源或隔离校验未通过")
	}
	return Paths{Manifest: manifest, Root: root}, nil
}

// validateNativePaths 下载前检查最终代次原生文件路径，为 Windows DLL 加载器保留内部路径余量。
func validateNativePaths(root string, plan *installationPlan) error {
	for name := range plan.files {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".exe", ".dll", ".pyd":
			// 本机 curl_cffi DLL 实测251单元已失败，不能把名义 MAX_PATH=260 当作可执行证据。
			if len(utf16.Encode([]rune(filepath.Join(root, filepath.FromSlash(name))))) > maxNativePathUTF16Units {
				return errors.New("组件依赖目录过长，Windows 无法安全加载原生扩展；请使用较短路径的 --profile 后重新检测")
			}
		}
	}
	return nil
}

// removeOwnedAttempt 只删除本次直接创建的暂存或尚未选中代次。
func removeOwnedAttempt(base string, path string, id string) error {
	if filepath.Dir(path) != base || filepath.Base(path) != "stage-"+id && filepath.Base(path) != "gen-"+id || privateruntime.RejectAliases(base) != nil {
		return errors.Join(ErrCleanup, errors.New("组件依赖暂存清理边界校验失败"))
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := filepath.WalkDir(path, func(local string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil || privateruntime.RejectAliases(local) != nil {
			return errors.New("组件依赖暂存出现异常路径，已保留以避免误删")
		}
		return nil
	}); err != nil {
		return errors.Join(ErrCleanup, err)
	}
	if err := os.RemoveAll(path); err != nil {
		return errors.Join(ErrCleanup, errors.New("组件依赖初始化失败，且本次暂存未能完整清理"))
	}
	return nil
}
