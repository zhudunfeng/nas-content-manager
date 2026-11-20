# 资源收集实现审查（2026-11-20）

依据当前 PRD、api-contract.md、check.jsonl 和 backend/runtime/download/cross-platform/collection、frontend/app 契约审查。审查使用实际源文件、跨层调用与隔离测试；不以工具已安装推断真实来源可用。

## 已修复的审查发现

1. **实际 Docker 容器环境漂移**：此前精确校验 Compose，但实际 `container inspect` 未核对 Env/User/no-new-privileges。相同镜像、标签与挂载可覆盖 worker 工具路径或来源配置。`internal/desktop/docker.go` 现核对镜像默认环境与固定模板覆盖后的精确环境集合，拒绝重复、额外及变更参数；用户必须与发行镜像一致，禁止提权设置不可缺失。`docker_compose.go` 共用 `serviceEnvironment`，避免模板与实际核验分歧。错误不回显环境值。
2. **前端目标切换沿用隐藏库编号**：先选已有库，再勾创建库，之前隐藏 libraryId 仍会提交。`CollectionTargetFields.vue` 在创建时清零；预览、候选和订阅提交也规范为创建库时 libraryId=0，保留首次提交意图供不确定重试。
3. **前端恢复状态与按钮**：`canceling`/`retrying` 现有中文状态。已发布文件变动冻结后的 reconciliation_required/failed 不再显示必定被后端拒绝的 retry_import，而提供恢复对账。仅真实 importing/failed 显示独立入库重试。
4. **自动订阅表达式示例**：此前 S01E01 示例缺少必需的 season/episode 命名捕获，无法形成明确集数；现给出实际 Go 正则命名捕获示例。
5. **导入覆盖只检查尺寸**：交给 backend 所有者修复。导入前复核实际文件尺寸与mtime，数据库预期覆盖也核对mtime；目录漫画按扫描器相同合计size/最大mtime。
6. **候选已有任务时目标冲突**：交给 backend 修复，重复获取必须核对内容类型、原始目标和建库授权；并发 ignore/acquire 在锁内重读候选。
7. **自动订阅历史窗口与多集歧义**：交给 backend 修复，历史已获取集数不能依靠最新100候选判断；分页读取持久任务历史，常见 E01E02、范围和季包等转人工确认。
8. **本地复制再次扫描竞态**：交给 worker 所有者修复。acquire 已绑定预览后，copyLocal 不应重新选择另一个清单；改为复制已核验列表并在结束后复核来源快照。
9. **去重命中未持久化的新请求别名**：交给 backend 修复。原请求编号才可幂等返回原任务；相同候选或 fingerprint 使用新请求编号时明确返回409，不生成可再次绑定其他资源的隐式别名。
10. **RSS 转换/保留 IPv6 目的地址**：交给 backend 修复。显式私网授权只扩展到真正的私网与回环；NAT64、6to4、协议保留与文档地址仍拒绝，公网 IPv6 限制为可路由范围。临时网络矩阵覆盖默认/显式私网两种模式。
11. **历史失败任务阻塞协调窗口**：交给 backend 修复。协调列表只读取可执行状态，避免最旧256个失败记录长期挤掉新任务。
12. **删除媒体库后仍显示已入库**：交给 backend 修复。删除索引与关联收集任务失效在同一事务中完成，清空ItemIDs/JobID并增加revision；已发布任务保留Ready进入入库失败，未发布任务进入可重试的failed/pending。原文件保留，历史建库授权不能静默重建被删除的绑定库。
13. **周期核验固定最旧256项**：交给 backend 修复。使用rowid轮转游标，每轮最多256条，后续成品也会被检查；第257条变动回归验证第二轮冻结。读取门禁始终独立拒绝版本已改变的文件。
14. **来源编辑后候选授权永久过期**：交给 backend 修复。RSS编辑清条件请求游标，下一次新响应更新未确认候选的来源版本；已确认任务共用私密引用保持不变。排队锁内再次核对来源启用/版本和自动订阅启用/版本，拒绝迟到旧授权。
15. **已完成BT任务永久停止观察下载事实**：交给 backend 修复。已绑定BT任务参与相同行号轮转，持续读取Download权威状态，默认撤销旧ItemIDs，仅实际发布版本和扫描覆盖均有效时恢复已入库；同状态的条目/字节/库关联变化也持久保存。删除库立即清除BT媒体链接，未绑定download的BT仍按failed/pending保留普通重试。归档引擎但保留有效媒体不误判为文件删除。

## 验证

- `python -B scripts/project_env.py -- .tools/go/bin/go.exe vet ./...`：通过。
- `python -B scripts/project_env.py -- .tools/go/bin/go.exe test ./...`：通过（含执行时仍存在的 backend 和 desktop 临时行为用例；worker 行为证据见 worker 研究记录，空测试包不计行为覆盖）。
- 桌面真实规范化 v1/v2 Compose、8项模板漂移及6项镜像清单用例：通过。
- 审查新增临时 actual-container 用例：正式环境通过，工具路径覆盖、额外环境、重复环境、用户覆盖、缺少禁止提权5类变更全部拒绝。该临时测试已按用户要求清理；不删除原有测试与集成脚本。
- 前端 `npm --prefix web run build`：包含严格 vue-tsc 和 Vite 生产构建，通过；最后目标切换修订后再次执行。
- `git diff --check`：通过；仅工作树 CRLF 规范化提示，无空白错误。
- backend所有者完成17个顶层临时行为用例（自动规则另含6个子场景），`go test ./internal/app -run TestCollection -v -count=1`通过，覆盖迁移、真实扫描、门禁、请求/目标冲突、网络策略、自动规则、历史身份、独立入库重试、不确定提交、来源修订、删库状态及第257条轮转；随后BT持续镜像与未绑定BT删库两个临时用例分别通过，累计19个顶层行为用例。明细见 [backend-verification.md](backend-verification.md)。临时测试已在各自通过后清理。

## 验证边界与后续核实

- 主流程执行实际浏览器、全部集成及最终重建复验；本审查不声称已经完成目标 NAS、Docker daemon、真实云盘或 BT 传输验收。
- 公网网页请求受当前 DNS fake-IP 环境影响，必须保留保留地址拒绝策略。网页 native HTTP/DASH 与本地 FFmpeg 后处理的受控工具证据见 worker 研究记录；HLS-only/登录/JS挑战未承诺。
- 最终源码复核覆盖删库Ready/DownloadID区分、RSS候选revision与锁内授权、256轮转、原请求幂等、不确定外部提交对账和BT持续权威镜像；本次审查发现均已修复，没有新增实现阻塞。主流程最终构建与集成结果以其验收记录为准。
