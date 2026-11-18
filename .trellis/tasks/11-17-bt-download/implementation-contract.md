# BT 下载实施协作契约

2026-11-17：用户已明确要求按计划实现，P1–P4 进入实施；Docker 不运行、不验证。业务范围见 prd.md，本文件固定跨层命名，变更须同步所有责任人。

## 前后端 DTO 与接口

- `GET /api/downloads/status` → `{ enabled: boolean, state: string, version: string, webApiVersion: string, lastSyncedAt: string, error: string, canSelectFiles: boolean }`。
- `GET /api/downloads?page=1&pageSize=30&state=&q=` → `{ downloads: Download[], total: number, page: number, pageSize: number }`。
- `GET /api/downloads/{id}` → `{ download: Download }`。
- `Download`：`id: string, name: string, contentType: ContentType, libraryId: number|null, state: string, engineState: string, importState: string, sizeBytes: number, downloadedBytes: number, downloadSpeed: number, uploadSpeed: number, uploadedBytes: number, ratio: number, peers: number, etaSeconds: number|null, manifestVersion: number, selectionLocked: boolean, revision: number, ready: boolean, archived: boolean, lastSyncedAt: string, lastError: string, createdAt: string, updatedAt: string`。速度单位 bytes/s，progress 由 downloadedBytes/sizeBytes 计算；lastError／error 必须脱敏。
- `GET /api/downloads/{id}/files?page=1&pageSize=100` → `{ files: DownloadFile[], total: number, page: number, pageSize: number, manifestVersion: number, selectionLocked: boolean }`。
- `DownloadFile`：`index: number, path: string, sizeBytes: number, selected: boolean, progress: number`，progress 为 0–1。
- `POST /api/downloads`：JSON `{ requestId: string, magnet: string, contentType: ContentType, createLibrary: boolean, libraryId?: number }`，或 multipart 单个 `torrent` 文件与同名标量字段（不含 magnet），返回 202 `{ download }`。createLibrary 表示明确允许首次创建对应下载媒体库；已有下载库可省略 libraryId 由后端按类型匹配。
- `PUT /api/downloads/{id}/selection`：`{ requestId: string, manifestVersion: number, indices: number[] }`，确认并开始正式下载，返回 202 `{ download }`；确认前任务不正常下载，之后选集锁定。
- `POST /api/downloads/{id}/actions`：`{ requestId: string, revision: number, action: 'start'|'pause'|'reconcile'|'remove' }`，返回 202 `{ download }`；remove 保留文件，实际行归档。
- `PUT /api/downloads/{id}/limits`：`{ downloadKiB: number, uploadKiB: number }`，返回 `{ ok: true }`，0 不附加任务限制，仍受引擎全局限制。
- `POST /api/downloads/{id}/retry-import` → 202 `{ download }`。
- `GET /api/downloads/{id}/items` → 现有 ItemPage；`POST /api/downloads/connection-test` → 与 status 同型。
- 所有接口沿用现有认证／同源、中文 `{error}`；qB 登录失败不能返回栖库会话 401。disabled 状态可读，其他写入明确 503。

## 后端责任与接入点

- 下载后端负责新增 `internal/app/download*.go`，**但不包括 root 负责的 `download_gate.go`**；实现 HTTP、引擎、持久状态、元数据解析、对账、发布、入库。不得改现有 server/store/config/media/jobs 等文件，需接入由 root 完成。
- `Config.Downloads DownloadConfig`，`loadDownloadConfig(config Config) (DownloadConfig,error)`；DownloadConfig 至少包括 Enabled bool、Root string、EngineRoot string、URL string、Username string、PasswordFile string、MinFreeBytes uint64。
- `App.downloads *downloadManager`；`(*App).startDownloads()` 初始化并通过 a.ctx／a.wg 管理协调器，未启用则不联网；`(*App).downloadRoutes(mux *http.ServeMux)` 注册已鉴权路由。
- 下载后端提供 `const schemaV2 string`（新表 SQL），root 负责 openDatabase 按序迁移到 user_version=2。
- 新表 `downloads` 至少供门禁使用的列：`id TEXT, local_root TEXT, final_path TEXT, staging_path TEXT, content_type TEXT, ready INTEGER, manifest_version INTEGER, archived INTEGER, target_library_id INTEGER`；路径为应用侧规范绝对路径。
- `download_files` 至少供门禁使用的列：`download_id TEXT, version INTEGER, file_index INTEGER, relative_path TEXT, size_bytes INTEGER, selected INTEGER`；relative_path 相对于 qB savepath（最终 task UUID 目录）。
- root 新增 `loadDownloadGate(ctx context.Context) (*downloadGate,error)` 应用方法；`(*downloadGate).allow(path string,isDir bool) (bool,error)`；`(*App).checkDownloadPath(path string,isDir bool) error` 单次访问校验；`(*downloadGate).blockedUnder(path string) bool` 告知扫描不可执行缺失清理。后端发布完成之前不以这个 helper 反向验证待发布文件，避免循环门禁。
- 发布实现需要源路径清单、int64、symlink／边界和文件尺寸验证；不得只用百分比。登记 ready 后 enqueueScan，核对清单实际覆盖，否则合并补扫，不把老扫描结束当作成功。
- 全部中文方法／字段／配置说明与关键日志；临时单测验证后删除，保留集成脚本。

## 并行文件所有权

- 下载后端：上述 download*.go（除 download_gate.go）与临时 Go 测试。
- 前端：web/src 下新增下载页面／弹窗、api.ts、types.ts、App.vue、Icon.vue、style.css；复用现有主题与阅读器，不改服务端。
- 部署验证：可选 compose.downloads*.yaml、.env.downloads.example、docs/downloads.md、scripts/verify_downloads.py 与 research 中的技术探针记录；不改主 README、docs/api.md、docs/verification.md 和现有 verify.py，交给 root 汇总。
- root：现有后端接线／配置／schema 迁移、download_gate.go、扫描／读取门禁、主文档／规范／Trellis 与最终质量验证。

所有代理共享工作区，不回滚他人修改，不自动提交，不运行 Docker。无法取得真实引擎时需要明确记录限制，使用可控假引擎验证不等于真实传输验证。
