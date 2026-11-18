# BT 引擎初始化实现审查

日期：2026-11-18。阶段：Trellis Phase 2.2。本文仅记录独立代码审查、修复和已执行检查，不替代真实镜像、NAS 性能或浏览器验收记录。

## 已修复发现

### 1. 安装级执行器缺少跨进程互斥

- 文件：`internal/enginehelper/service.go`、`lease_linux.go`、`lease_other.go`。
- 原因：`Run` 调用 `New` 后才检查 Unix socket；`New` 已启动 worker 并恢复未完成任务。两个助手进程共享同一安装目录时，后启动进程可能在被 socket 检查拒绝前执行同一任务。进程内 `opMu` 不能阻止此路径。
- 修复：在打开状态库、读取安装身份和恢复任务前，取得私有 `state/helper.lock` 的非阻塞 Linux `flock`。进程锁覆盖 worker 和状态库的整个生命周期；只有执行器停止、数据库关闭后才释放。锁文件保持原 inode，不在关闭时删除。拒绝符号链接、非普通文件、不安全权限和错误属主；非 Linux 平台明确拒绝该助手能力。
- 回归：隔离 Linux 环境的 `TestReviewHelperLease` 通过，验证第二个独立打开的锁描述符无法取得同一安装锁、原锁关闭后允许恢复、错误文件权限被拒绝。根代理执行结果保存于忽略目录 `.runtime/engine-bootstrap/TestReviewHelperLease.txt`。

### 2. 历史任务清理后面板无法恢复

- 文件：`internal/app/engine_api.go`。
- 原因：助手对已清理任务返回 404，`helperCall` 却将其改写为 502；前端原有 404 分支无法清除 `observedTaskId`，持续读取错误会禁用初始化操作。
- 修复：保留 404 状态，并使用固定中文错误，不透传助手原始响应。前端可以清除失效任务，再读取当前状态。
- 回归：临时 `TestReviewHelperMissingTask` 使用本地 HTTP 服务验证生产 `helperCall` 及 `engineTask` 处理器均返回 404，同时不输出原始助手错误内容，已通过。

### 3. 来源发现名称可能将已知密钥带入浏览器

- 文件：`internal/app/engine_sources.go`。
- 原因：发现 XML 的 `title` 直接作为候选名称返回；已知 API Key 出现在标题时没有沿用搜索结果的脱敏边界。上游 ID 包含相同密钥时也可能通过候选 ID 输出。
- 修复：将当前服务端 Key 传入发现解析器；完整替换标题中的已知 Key 后再校验显示名称。包含该 Key 的对外 ID 使用稳定摘要，实际 Indexer 仍仅在服务器内用于构造固定上游路径。
- 回归：临时 `TestReviewIndexerRedaction` 验证浏览器 DTO 的名称与 ID 不含 Key、私有索引器路径保持准确、重复发现产生相同安全 ID，已通过。

### 4. 新任务可能混合上一次任务的日志

- 文件：`web/src/components/EngineSetupDialog.vue`、`web/src/engines.ts`。
- 原因：原实现只在轮询路径发现任务 ID 改变时清理日志；初始化或启动 POST 直接返回新任务时，`applyTask` 会按 sequence 合并旧任务事件。初始化中的 missing/stopped 组件标签也可能被存在性分支覆盖。
- 修复：前端实现代理已将任务 ID 切换时的日志、游标和缺口状态清理集中到 `applyTask`，覆盖所有调用路径；`initializing` 标签优先于 missing/stopped。审查已读回确认。
- 验证：严格 TypeScript 检查通过；真实浏览器状态、焦点、响应式与主题证据由前端代理和根代理独立记录。

### 5. 清单严格性与临时挂载模板校验

- 文件：`internal/enginehelper/files.go`、`templates.go`、`docker.go`。
- 原因：助手原 `loadManifest` 允许未知字段并缺少组件版本不超过总版本的检查。另需确认 Docker inspect 中 tmpfs 的实际表示，避免对挂载集合做错误推断。
- 修复：助手实现代理已改为全新候选结构、严格字段和 EOF 校验，要求已存在清单总版本为正、组件版本在总版本内，并拒绝发布时 int64 上溢。真实 Docker 证据确认目标环境在 `Mounts` 中只列 bind；`HostConfig.Tmpfs` 另外核对 qB 唯一 `/tmp` 的 rw/noexec/nosuid/64 MiB 等价配置，Jackett 不允许额外 tmpfs。审查已读回确认。
- 验证：助手代理的 tmpfs 契约回归已通过；严格清单 Linux 回归二进制交根代理执行，最终结果以根代理记录为准。

## 其余代码路径复核

- 默认和 external 模式不触发 Docker 部署请求；external 连接错误没有接管路径。部署路由使用既有管理员认证和同源修改保护。
- GET/status 仅使用 Docker 元数据读取；只有明确 404 与目录归属校验成功才显示 missing。显式 start 不创建替代实例，也不拉取缺失镜像。
- create/start 先持久化阶段，响应丢失后按确定名称重新 inspect；归属标签、固定镜像、bind、网络及端口核验失败不会接管或删除资源。
- 永久 qB 凭据先保存，再生成固定版本配置；恢复复用已保存密码，不读取临时密码日志。连接文件先落盘，再原子切换 current 清单。
- 应用托管激活使用生命周期锁，一次发布下载客户端和一个 worker；有历史下载时固定安装、容器与下载根身份。清单读取失败保留原已验证连接。
- 来源同步不执行关键词搜索；XML 预算、DTD 拒绝、八来源上限和显式选择均存在。搜索快照按 generation 切换并清空未绑定结果，持久下载绑定优先于缓存查询。
- 初始化与下载载荷分离；下载根和 complete 发布门禁保持原路径，助手没有媒体卷或应用数据库挂载。

## 验证记录

- `gofmt`：本审查修改的 Go 文件已格式化。
- Lint：`.tools/go/bin/go.exe vet ./...` 通过。
- TypeCheck：`npm --prefix web run typecheck` 通过。
- 前端生产构建：`npm --prefix web run build` 通过，Vite 生产资源生成成功。
- Go build：主应用与助手构建通过。
- Go tests：临时行为回归通过后已删除本审查生成的两个临时测试文件。最终无测试的应用包输出只代表编译检查；不将其算作行为覆盖。助手代理的临时测试由其所有者及根代理按流程执行和清理。
- Linux：安装独占锁实际回归通过，未弱化 Docker seccomp。该检查只证明进程锁，不等同于完整固定镜像初始化或真实 BT 下载验收。
- 完整应用集成、原 BT/搜种/五类媒体及大文件回归、全部远程测试资源清理由根代理汇总最终证据。

## 未修复发现

本次独立审查未留下已确认但未修复的代码缺陷。目标主机旧 Docker seccomp 对 Jackett 写入行为的兼容限制属于真实环境验收结果；产品必须明确阻断和提示，不自动关闭安全配置。最终可部署范围、验收缺口与清理结果由根代理如实披露。
