# 初始化实现与最终验收（2026-11-18）

## 实现交付

- 可选`nas-engine-helper`，私有Unix socket及独立SQLite状态；固定qB/Jackett模板、真实pull进度、持久请求绑定和事件、失败重试/恢复、安装级flock、归属与UID检查、永久凭据和原子连接发布。
- 主应用三模式兼容、生产适配器热激活、来源发现/显式选择、generation缓存隔离、固定安全API。主库保持v2；媒体Range与发布门禁保留。
- 下载中心引擎双卡及管理模态，首次告知、任务恢复、真实层进度、来源选择、单模态/焦点/迟到请求隔离。
- `docs/engines.md`、README、API、verification及backend七节契约已同步。源码未提交，任务未归档。

## 检查结果与证据边界

| 检查 | 结果/范围 |
| --- | --- |
| gofmt/vet/build | 通过；主应用/助手，Linux amd64/arm64构建 |
| helper Linux行为 | 11组顶层测试通过，拉取安全另含3个子场景；严格清单/pwritev2/流/归属/幂等/恢复/权限/凭据等 |
| 安装级flock | 独立Linux回归通过 |
| 应用临时行为回归 | 6组运行时+2组独立404/Key脱敏通过；临时测试源码已删除 |
| managed应用集成 | `verify_engine_bootstrap.py`最终35项通过，真实应用/SQLite/生产adapter+受控引擎 |
| 原下载/搜种 | 36/43项通过，受控qB/Torznab，不宣称真实BT传输 |
| 五媒体/大文件 | 52项通过，真实音视频/FFmpeg+5GiB稀疏协议样本，非性能基准 |
| 前端 | strict typecheck/Vite/45项浏览器断言通过；真实应用认证+明确引擎fixture |
| Compose | 托管覆盖文件合并校验通过 |
| 独立review | 已修复锁竞态、任务404、Key脱敏、事件隔离、清单/tmpfs；无已知未修复发现 |

详细审查见`bootstrap-code-review.md`；UI可复用脚本与结果见同目录`verify-engine-ui.js`、`ui-verification-result.json`。最终`go test`无临时文件仅作为编译检查，不替代上述行为证据。

## 远程Docker实际结果

本次经用户追加授权使用同一Linux测试宿主。Docker20.10.3/API1.41/x86_64、3.10内核；产品本身仍不支持远端daemon部署模式。

1. 真正运行了task-only scratch应用/助手，非root应用UID1000经Unix socket及只读目录操作助手。GET无创建副作用、missing、告知409、幂等目标冲突、失败重试及重启恢复通过。生产adapter经内部容器DNS认证真实qB5.2.4，未添加载荷任务。
2. 官方qB固定镜像PBKDF2首启和重启后密码验证通过。宿主官方直接拉取受网络限制，已有阿里云加速探测403；经本机原registry验证固定index/平台/config/各层/diffID并导入后运行。未改daemon配置、镜像版本或摘要校验。
3. Jackett固定镜像实际拉取成功，默认seccomp以EPERM拦截pwritev2，.NET无法写配置；产品新增拉取前明确预检失败。临时unconfined诊断确认ENOSYS fallback、空站点发现和设置管理密码后的API Key发现。该诊断不是默认部署成功。
4. 完整生产Dockerfile/FFmpeg发行构建、助手完整两引擎成功闭环、真实已配置站点/关键词搜索/BT下载及arm64实机、百GiB性能未宣称通过。上述环境限制保留在review交付，不能把fixture补成假实测。

## 精确清理已完成

任务标签`nas-bootstrap-d8424b955227`：容器0、网络0、已跟踪测试镜像与非基线父层0；专用宿主临时根已删除。镜像无force删除，未执行全局prune。原66个容器、7563个镜像及全部基线网络保留；3354卷清单与基线完全一致。只在忽略目录记录远程基线、对象ID和脱敏证据；本地明文随机测试凭据已删除。

最终清理证据：`.runtime/engine-bootstrap/cleanup-result.json`。源码中不保存服务器凭据或原始私密引擎响应。

## 交付状态

实施/独立质量检查/spec更新/清理完成，任务置review，待用户审阅后提交。完整默认引擎成功闭环仍须可用镜像仓库与兼容Docker安全策略环境补验，不自动升级用户宿主或降低产品安全设置。
