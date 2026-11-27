package runtimebootstrap

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nas-content-manager/internal/privateruntime"
)

// errSourceRead 标识上游流读取中断，下载器可有限重试，压缩还原仍直接失败。
var errSourceRead = errors.New("组件依赖读取中断或压缩校验失败，请重新检测")

// materialize 创建仅属于本次尝试的固定库存，不执行外部进程。
func (e *Engine) materialize(ctx context.Context, stage string, plan *installationPlan, report func(Progress)) error {
	client, transport := newDownloadClient()
	defer transport.CloseIdleConnections()
	return e.materializeWithClient(ctx, stage, plan, report, client)
}

// materializeWithClient 保持生产默认客户端，同时允许离线集成验证复用真实固定材料。
func (e *Engine) materializeWithClient(ctx context.Context, stage string, plan *installationPlan, report func(Progress), client *http.Client) (resultErr error) {
	if privateruntime.RejectAliases(stage) != nil {
		return errors.New("组件依赖暂存目录包含异常路径")
	}
	downloads := filepath.Join(stage, ".downloads")
	if err := os.Mkdir(downloads, 0700); err != nil {
		return errors.New("无法创建组件依赖下载暂存，请检查空间和权限")
	}
	defer func() {
		if err := filepath.WalkDir(downloads, func(local string, _ fs.DirEntry, walkErr error) error {
			if walkErr != nil || privateruntime.RejectAliases(local) != nil {
				return errors.New("组件下载暂存路径异常")
			}
			return nil
		}); err != nil {
			resultErr = errors.Join(resultErr, ErrCleanup, errors.New("组件下载暂存路径异常，已保留以避免误删"))
		} else if err := os.RemoveAll(downloads); err != nil {
			resultErr = errors.Join(resultErr, ErrCleanup, errors.New("组件下载暂存未能完整清理"))
		}
	}()
	progress(report, "正在准备组件插件和许可文件", 0, 0)
	names := make([]string, 0, len(plan.payload))
	for name := range plan.payload {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := extractMember(ctx, stage, plan.payload[name], plan.files[name]); err != nil {
			return err
		}
	}
	for index, source := range plan.artifacts {
		if err := ctx.Err(); err != nil {
			return err
		}
		temporary := filepath.Join(downloads, fmt.Sprintf("artifact-%03d", index))
		if err := downloadArtifact(ctx, client, temporary, source, report); err != nil {
			return err
		}
		progress(report, "正在还原组件依赖："+source.Name, 0, 0)
		if err := restoreArtifact(ctx, stage, temporary, source, plan); err != nil {
			return err
		}
		if err := os.Remove(temporary); err != nil {
			return errors.Join(ErrCleanup, errors.New("已还原的组件下载材料未能清理"))
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeNewFile(filepath.Join(stage, e.config.ManifestName), e.config.Runtime); err != nil {
		return err
	}
	return writeNewFile(filepath.Join(stage, "bootstrap-receipt.json"), e.config.Recipe)
}

// restoreArtifact 在检查完整 ZIP 目录后仅提取固定映射，不保留额外文件。
func restoreArtifact(ctx context.Context, stage string, temporary string, source artifact, plan *installationPlan) (resultErr error) {
	if source.Format == "raw" {
		file, err := os.Open(temporary)
		if err != nil {
			return errors.New("已下载的组件依赖无法读取")
		}
		defer func() {
			if err := file.Close(); err != nil {
				resultErr = errors.Join(resultErr, ErrCleanup, errors.New("组件下载材料句柄未能完整关闭"))
			}
		}()
		mapping := source.Files[0]
		target, err := prepareTarget(stage, mapping.Destination)
		if err != nil {
			return err
		}
		return writeExpected(ctx, target, file, plan.files[mapping.Destination], nil)
	}
	reader, err := zip.OpenReader(temporary)
	if err != nil {
		return errors.New("已下载的组件依赖压缩材料无效")
	}
	defer func() {
		if err := reader.Close(); err != nil {
			resultErr = errors.Join(resultErr, ErrCleanup, errors.New("组件压缩材料句柄未能完整关闭"))
		}
	}()
	members, err := indexArchive(&reader.Reader)
	if err != nil {
		return err
	}
	for _, mapping := range source.Files {
		member, found := members[mapping.Source]
		record := plan.files[mapping.Destination]
		if !found || member.UncompressedSize64 != uint64(record.Size) {
			return errors.New("组件依赖压缩材料缺少锁定文件或长度不匹配")
		}
		if err := extractMember(ctx, stage, member, record); err != nil {
			return err
		}
	}
	return nil
}

// extractMember 完整读取单个固定 ZIP 文件，触发 CRC 并同时校验文件摘要。
func extractMember(ctx context.Context, stage string, member *zip.File, record fileRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := prepareTarget(stage, record.Path)
	if err != nil {
		return err
	}
	reader, err := member.Open()
	if err != nil {
		return errors.New("组件依赖压缩文件无法打开")
	}
	defer reader.Close()
	return writeExpected(ctx, target, reader, record, nil)
}

// prepareTarget 只在本次暂存下创建普通父目录，禁止路径穿越和重解析点。
func prepareTarget(stage string, name string) (string, error) {
	if !privateruntime.SafeRelative(name) || privateruntime.RejectAliases(stage) != nil {
		return "", errors.New("组件依赖目标路径不安全")
	}
	parts := strings.Split(name, "/")
	parent := stage
	// 每层先核验再进入，不让 MkdirAll 在检测前跟随既有链接创建下级目录。
	for _, part := range parts[:len(parts)-1] {
		child := filepath.Join(parent, part)
		if err := os.Mkdir(child, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", errors.New("无法创建安全的组件依赖目录，请检查空间和权限")
		}
		info, err := os.Lstat(child)
		if err != nil || !info.IsDir() || privateruntime.RejectAliases(child) != nil {
			return "", errors.New("组件依赖父目录包含链接或类型异常")
		}
		parent = child
	}
	return filepath.Join(parent, parts[len(parts)-1]), nil
}

// writeNewFile 独占写入并持久化固定小型元数据，失败时只清理本次创建的文件。
func writeNewFile(target string, data []byte) error {
	return writeExpected(context.Background(), target, bytes.NewReader(data), fileRecord{SHA256: digestBytes(data), Size: int64(len(data))}, nil)
}

// writeExpected 分块取消、精确长度及摘要校验后才保留独占创建的普通文件。
func writeExpected(ctx context.Context, target string, reader io.Reader, record fileRecord, update func(int64)) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.Size < 0 || record.Size > maxArtifactSize || !digestPattern.MatchString(record.SHA256) || privateruntime.RejectAliases(filepath.Dir(target)) != nil {
		return errors.New("组件依赖文件目标或固定身份无效")
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("无法独占创建组件依赖文件，请检查空间和权限")
	}
	closed := false
	valid := false
	defer func() {
		if !closed {
			if err := file.Close(); err != nil {
				resultErr = errors.Join(resultErr, ErrCleanup, errors.New("组件依赖文件句柄未能关闭"))
			}
		}
		if !valid {
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				resultErr = errors.Join(resultErr, ErrCleanup, errors.New("组件依赖不完整文件未能清理"))
			}
		}
	}()
	hash := sha256.New()
	bounded := io.LimitReader(reader, record.Size+1)
	buffer := make([]byte, 64<<10)
	var count int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := bounded.Read(buffer)
		if n > 0 {
			count += int64(n)
			if count > record.Size {
				return errors.New("组件依赖文件超过锁定长度，已拒绝保存")
			}
			written, writeErr := file.Write(buffer[:n])
			if writeErr != nil || written != n {
				return errors.New("组件依赖写入失败，请检查可用空间和权限")
			}
			_, _ = hash.Write(buffer[:n])
			if update != nil {
				update(count)
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return errSourceRead
			}
			break
		}
	}
	if count != record.Size || hex.EncodeToString(hash.Sum(nil)) != record.SHA256 {
		return errors.New("组件依赖文件长度或完整性校验失败，请重新检测")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return errors.New("组件依赖文件无法完整持久化，请检查空间和权限")
	}
	if err := file.Close(); err != nil {
		closed = true
		return errors.Join(ErrCleanup, errors.New("组件依赖文件句柄未能完整关闭"))
	}
	closed = true
	valid = true
	return nil
}
