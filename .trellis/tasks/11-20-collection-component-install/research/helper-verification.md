# Helper 组件部署隔离验证

执行日期：2026-11-20。全部命令经 `python -B scripts/project_env.py --` 运行，缓存、SQLite 与临时文件限定在仓库 `.runtime/`。

## 已执行

- `.tools/go/bin/gofmt.exe -w internal/enginehelper internal/enginecontract/collection.go`，成功。
- `.tools/go/bin/go.exe test -v ./internal/enginehelper -run TestCollection`，六项测试全部通过，最终输出 `ok nas-content-manager/internal/enginehelper 1.641s`。
- `.tools/go/bin/go.exe vet ./internal/enginehelper ./internal/enginecontract`，退出码 0。

原始测试与 vet 输出保存在 `.runtime/verify/collection-helper-tests.txt`、`.runtime/verify/collection-helper-vet.txt`。新增临时测试源 `internal/enginehelper/collection_temporary_test.go` 已在成功运行并保存证据后删除。

## 六项行为验证

1. `TestCollectionFixedTemplateBoundary`：四个固定组件在 bind/volume 两种模式生成模板并映射回 inspect；篡改 privilege、capability、网络、标签、挂载、外来卷、安全选项、重启策略或公网端口均拒绝。关闭组件时保留原六个存储角色，启用时追加独立媒体角色。
2. `TestCollectionVolumeAuthorizationBoundary`：读取私密卷仅执行 GET，不写授权、不创建资源；错误 nonce 或外来标签拒绝。
3. `TestCollectionAnonymousProbeBoundary`：四个组件通过本地真实 HTTP 服务验证固定匿名协议；错误版本、数据库未就绪、错误响应与重定向拒绝。
4. `TestCollectionPersistentRoutes`：未确认告知或版本错误 409；同 UUID 同意图恢复原任务；不同目标/UUID 冲突或繁忙不入队；新旧任务路由互斥；未知参数、过期 revision 与禁用状态分别按契约返回；读取最新任务支持恢复。
5. `TestCollectionLostCreateResponse`：模拟 Docker create 响应丢失但 inspect 存在准确资源，只创建一次并成功对账；同名外来卷冲突拒绝。
6. `TestCollectionPersistentRecovery`：SQLite 关闭后重开，installation 和 requestId 仍绑定原任务；启用后的 stop-all 包含六组件，停止隔离门阻止后续部署。

## 验收边界

这些是 Windows 上的内存 Docker transport、本地匿名 HTTP、真实 SQLite 隔离测试，不是实际 Docker 产品部署。未拉取四个组件的真实镜像，未连接真实站点、账号或媒体。真实 Linux NAS 与 Docker Desktop 的镜像初始化、UID/ACL、回环访问及首次配置仍需目标环境验收；新增私密授权文件的 Linux 落盘与断电语义复用了原有安全原语，本轮未在真实 Docker/Linux 文件系统中演练。
