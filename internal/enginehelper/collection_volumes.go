package enginehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"nas-content-manager/internal/enginecontract"
)

// collectionVolumeAuthorization 在Docker副作用前记录私密卷的不可猜测授权身份。
type collectionVolumeAuthorization struct {
	Version      int    `json:"version"`      // 固定授权文件协议版本1。
	Installation string `json:"installation"` // 持久助手安装身份。
	Component    string `json:"component"`    // 固定资源产品。
	Role         string `json:"role"`         // 固定配置、缓存或数据库职责。
	Name         string `json:"name"`         // 从固定职责派生的唯一Docker卷名。
	Nonce        string `json:"nonce"`        // 只保存在助手私密state与Docker标签中的随机授权值。
}

// collectionVolumeInfo 只读取归属判断需要的卷元数据，从不读取Docker内部Mountpoint。
type collectionVolumeInfo struct {
	Name    string            // 精确逻辑卷名。
	Driver  string            // 唯一允许local驱动。
	Scope   string            // 唯一允许local作用域。
	Labels  map[string]string // 精确私密授权标签。
	Options map[string]string // 必须为空，禁止宿主/远程驱动选项。
}

// collectionVolumeAuthorizationPath 只从固定产品和职责生成助手内部私密路径。
func (s *Service) collectionVolumeAuthorizationPath(component string, role string) string {
	return filepath.Join(s.config.Root, "state", "collection-volume-"+component+"-"+role+".json")
}

// readCollectionVolumeAuthorization 严格验证已有授权文件，未知或损坏数据不会被覆盖。
func (s *Service) readCollectionVolumeAuthorization(component string, role string) (collectionVolumeAuthorization, error) {
	var authorization collectionVolumeAuthorization
	path := s.collectionVolumeAuthorizationPath(component, role)
	data, err := readPrivate(path)
	if err != nil {
		return authorization, err
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		return authorization, errConflict
	}
	uid, gid, err := fileIdentity(info)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err != nil || uid != 0 || gid != 0 || decoder.Decode(&authorization) != nil || decoder.Decode(&struct{}{}) != io.EOF || authorization.Version != 1 || authorization.Installation != s.store.installation || authorization.Component != component || authorization.Role != role || authorization.Name != collectionVolumeName(s.store.installation, component, role) || !validInstallation(authorization.Nonce) {
		return authorization, errConflict
	}
	return authorization, nil
}

// collectionVolumeLabels 返回新私密卷必须精确匹配的完整身份标签。
func collectionVolumeLabels(authorization collectionVolumeAuthorization) map[string]string {
	return map[string]string{"nas.engines.installation": authorization.Installation, "nas.engines.component": authorization.Component, "nas.engines.role": authorization.Role, "nas.engines.authorization": authorization.Nonce, "nas.engines.template": enginecontract.CollectionTemplateVersion}
}

// inspectCollectionVolume 只有真实404代表不存在，其他故障不会触发创建。
func (s *Service) inspectCollectionVolume(ctx context.Context, name string) (*collectionVolumeInfo, error) {
	var info collectionVolumeInfo
	err := s.docker.call(ctx, http.MethodGet, s.docker.api()+"/volumes/"+url.PathEscape(name), nil, &info)
	var dockerErr dockerError
	if errors.As(err, &dockerErr) && dockerErr.status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("资源组件私密卷暂不可验证，请检查Docker状态")
	}
	return &info, nil
}

// validateCollectionVolume 同时核对本地驱动和持久授权nonce，绝不只凭同名认领。
func validateCollectionVolume(info *collectionVolumeInfo, authorization collectionVolumeAuthorization) error {
	if info.Name != authorization.Name || info.Driver != "local" || info.Scope != "local" || len(info.Options) != 0 || !maps.Equal(info.Labels, collectionVolumeLabels(authorization)) {
		return errors.New("资源组件私密卷归属或驱动冲突，不会接管或覆盖已有数据")
	}
	return nil
}

// collectionPrivateVolume 核验或明确创建一个固定私密卷，丢失响应后只读对账。
func (s *Service) collectionPrivateVolume(ctx context.Context, component string, role string, claim bool, required bool) error {
	definition, err := collectionDefinitionFor(component)
	if err != nil || definition.Volumes[role] == "" {
		return errConflict
	}
	name := collectionVolumeName(s.store.installation, component, role)
	actual, err := s.inspectCollectionVolume(ctx, name)
	if err != nil {
		return err
	}
	authorization, authErr := s.readCollectionVolumeAuthorization(component, role)
	if authErr != nil && !errors.Is(authErr, os.ErrNotExist) {
		return errors.New("资源组件私密卷授权文件不安全或已损坏")
	}
	if actual != nil {
		if authErr != nil {
			return errors.New("发现未知同名组件配置卷，不会接管已有数据")
		}
		return validateCollectionVolume(actual, authorization)
	}
	if !claim {
		if required {
			return errors.New("原组件私密卷已缺失，禁止启动并静默重建配置")
		}
		return nil
	}
	if authErr != nil {
		// 只有只读检查已确认卷不存在，才持久化随机创建授权；崩溃后复用原nonce。
		nonce, randomErr := randomID()
		if randomErr != nil {
			return errors.New("安全随机源不可用，未创建组件私密卷")
		}
		authorization = collectionVolumeAuthorization{Version: 1, Installation: s.store.installation, Component: component, Role: role, Name: name, Nonce: nonce}
		data, marshalErr := json.Marshal(authorization)
		if marshalErr != nil || atomicFile(s.collectionVolumeAuthorizationPath(component, role), data, 0600, 0, 0) != nil {
			return errors.New("组件私密卷授权无法持久化，已停止部署")
		}
	}
	input := struct {
		Name       string            // 固定安装职责卷名。
		Driver     string            // 固定本地驱动。
		DriverOpts map[string]string // 禁止任意宿主路径、设备或远程驱动选项。
		Labels     map[string]string // 包含持久随机授权的精确身份。
	}{name, "local", map[string]string{}, collectionVolumeLabels(authorization)}
	createErr := s.docker.call(ctx, http.MethodPost, s.docker.api()+"/volumes/create", input, nil)
	actual, err = s.inspectCollectionVolume(ctx, name)
	if err != nil || actual == nil {
		if createErr != nil {
			return errors.New("组件私密卷创建结果暂不可确认，请保留授权后重试对账")
		}
		return errors.New("组件私密卷创建响应与实际资源不一致")
	}
	if err = validateCollectionVolume(actual, authorization); err != nil {
		return err
	}
	s.logger.Info("已核对资源组件独立私密卷，未读取应用凭据", "component", component, "role", role)
	return nil
}

// collectionPrivateVolumes 按固定顺序核验产品的全部私密配置职责。
func (s *Service) collectionPrivateVolumes(ctx context.Context, component string, claim bool, required bool) error {
	definition, err := collectionDefinitionFor(component)
	if err != nil {
		return err
	}
	for _, role := range collectionVolumeRoles(definition) {
		if err = s.collectionPrivateVolume(ctx, component, role, claim, required); err != nil {
			return err
		}
	}
	return nil
}

// prepareCollectionData 只在已授权任务中认领固定数据角色，不改动原媒体库。
func (s *Service) prepareCollectionData() error {
	if s.config.StorageMode == "volume" {
		for _, role := range []string{"downloads", "collection-media"} {
			if _, err := claimVolume(s.config, role, s.store.installation, true); err != nil {
				return err
			}
		}
	} else if err := s.ownedDirectory("collection-media", true); err != nil {
		return err
	}
	for _, directory := range []string{"downloads", "collection-media", "collection-media/library", "collection-media/library/moviepilot", "collection-media/library/autobangumi", "collection-media/library/sonarr", "collection-media/library/radarr"} {
		path := filepath.Join(s.config.Root, filepath.FromSlash(directory))
		if err := ensureDirectory(path, 0755, s.config.UID, s.config.GID); err != nil {
			return err
		}
		if err := verifyTargetAccess(path, s.config.UID, s.config.GID); err != nil {
			return err
		}
	}
	return nil
}

// environmentValues 拒绝重复或无效变量，保留等号后的原始值。
func environmentValues(entries []string) (map[string]string, error) {
	values := map[string]string{}
	for _, entry := range entries {
		key, value, found := strings.Cut(entry, "=")
		if !found || key == "" {
			return nil, errConflict
		}
		if _, exists := values[key]; exists {
			return nil, errConflict
		}
		values[key] = value
	}
	return values, nil
}

// validateCollectionRuntime 还原固定镜像环境与入口，拒绝额外环境、命令和用户注入。
func (s *Service) validateCollectionRuntime(ctx context.Context, info *containerInfo, t template) error {
	if err := validateCollectionContainer(info, s.config, s.store.installation, t); err != nil {
		return err
	}
	if s.config.StorageMode == "bind" {
		for _, role := range []string{"downloads", "collection-media"} {
			if err := checkDirectoryTree(filepath.Join(s.config.Root, role)); err != nil {
				return errors.New("原资源组件数据根缺失或路径不安全，不会自动重建")
			}
		}
		if err := s.ownedDirectory("collection-media", false); err != nil {
			return err
		}
	}
	imageID, ready, err := s.docker.imageReady(ctx, t)
	if err != nil || !ready || info.Image != imageID {
		return errors.New("资源组件实际镜像内容无法与固定摘要匹配")
	}
	var image struct {
		Config struct {
			Env        []string // 固定镜像内置环境。
			Cmd        []string // 固定镜像内置启动参数。
			Entrypoint []string // 固定镜像内置入口。
			User       string   // 固定镜像初始用户。
		} // 容器必须保留的上游启动默认值。
	}
	if err = s.docker.call(ctx, http.MethodGet, s.docker.api()+"/images/"+url.PathEscape(t.Image)+"/json", nil, &image); err != nil {
		return errors.New("无法核验资源组件固定镜像启动配置")
	}
	want, err := environmentValues(image.Config.Env)
	if err != nil {
		return errors.New("固定镜像环境配置存在重复项，不能安全部署")
	}
	overrides, _ := environmentValues(makeCollectionSpec(s.config, s.store.installation, t).Env)
	maps.Copy(want, overrides)
	actual, err := environmentValues(info.Config.Env)
	if err != nil || !maps.Equal(actual, want) || !slices.Equal(info.Config.Cmd, image.Config.Cmd) || !slices.Equal(info.Config.Entrypoint, image.Config.Entrypoint) || info.Config.User != image.Config.User {
		return errors.New("资源组件启动环境、参数或用户已偏离固定镜像模板")
	}
	return s.collectionPrivateVolumes(ctx, t.Component, false, true)
}
