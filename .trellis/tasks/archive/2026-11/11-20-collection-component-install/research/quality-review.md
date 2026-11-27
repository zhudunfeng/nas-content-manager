# 组件部署独立质量审查

日期：2026-11-20。审查依据：本任务 PRD、info.md、check.jsonl 引用规范，以及 `collection-component-contract.md`。

## Findings (fixed)

- File: `web/src/components/CollectionSourceDialog.vue`
  - Issue: 同一来源弹窗先连接已部署 MoviePilot，再切换 Sonarr 时，旧的自动浏览器入口和 `library/moviepilot` 完成目录会保留；第二次连接仅补空值，可能保存错误入口和目录。
  - Fix: 跟踪系统自动建议的入口、目录根及子目录；切服务或来源类型时，仅清理仍与建议完全匹配的字段。用户另填入口、另选目录或修改子目录均保留。保持 `/collection-media/library` 严格根匹配，未为 Windows 测试扩大匹配范围。

- File: `internal/enginehelper/service.go`, `internal/enginehelper/collection_api.go`
  - Issue: 活动任务合并原来只比较 action/component。资源组件的启动意图包含 revision，而路由先查签名、随后调用 submit 存在并发窗口，不同版本可能错误绑定同一活动任务；原活动任务在此间结束时，别名分支还可能新建绕过状态复检的任务。
  - Fix: submit 在同一个 opMu 短锁内校验资源组件的完整持久签名；活动意图使用同锁下的专门别名操作，只绑定既有任务，无活动任务则回到原状态校验。原 qB/Jackett 合并协议保留。

## Findings (not fixed)

无待修复的具体代码问题。真实四产品初始化、NAS ACL、Docker Desktop 运行仍属于部署验收边界，不能以本次隔离验证声称已完成真实安装。

## 目录归属复核

- bind 模式的既有 qB downloads 仅通过固定 HostRoot、UID/GID、安全路径和访问探针管理，历史上没有 owner marker；不能对非空正确身份的既有下载根突然强制新 marker，否则会拒绝合法升级数据。新增组件复用同一固定目录并通过主动告知授权。
- 首次启动时 app 的 bind 挂载可能预建空的 root-owned collection-media。显式部署中 `ownedDirectory(..., true)` 先执行 ensureDirectory，允许对空目录设置目标 UID/GID，再写入安装 marker；非空未知目录依然拒绝。后续创建 library 子目录及目标身份访问探针保持原路径约束。
- 私密配置卷由持久 nonce 和完整标签证明归属，GET 不认领、不创建；未知同名卷拒绝，创建失响应经 inspect 对账；实际镜像、环境、入口、网络、端口、挂载和安全配置在状态与执行路径核对。

## Verification

- Lint: pass。修改后运行 `go vet ./internal/enginehelper ./internal/enginecontract ./internal/app ./internal/desktop`。
- TypeCheck: pass。`npm --prefix web run typecheck`；生产 `npm --prefix web run build` 再次包含 vue-tsc，Vite 构建通过。
- Tests: pass。临时真实 Vue SFC setup 隔离验证 23 个断言：MoviePilot 自动建议清理后重新填入 Sonarr；手填入口/目录保留；仅修改子目录保留；连接前已有设置保留；来源类型切换清理自动建议；私密输入切换清空。该用例编译实际 SFC 并使用 Vue 响应式状态，未使用 DOM，不能称浏览器/NAS 验收。
- Tests: pass。临时 Go `TestReviewCollectionIntentSignature` 使用真实持久 SQLite，覆盖完整意图成功别名、不同 revision 冲突且不留绑定、原子别名、异组件忙碌、原任务完成后别名分支不新建、已绑定完成任务恢复、任务数量不膨胀、旧 qB/start 合并兼容。
- 所有本地构建/测试命令均使用 `python -B scripts/project_env.py -- ...`。临时 Go 测试源与临时 SFC 验证脚本运行后清除，保留本报告中的可核对结果。
- 其他真实 app→Unix 协议 fixture、浏览器及全回归证据由主任务统一记录。本报告不将 fixture 结果称为真实 Docker 产品部署。

## 交接

- 审查修改范围仅为上述三个生产文件；web 已重建，helper 相关二进制需要主任务最终统一构建。
- 主任务补充前端/部署契约：自动建议只在未修改时按组件清理；活动组件任务只能合并完整持久签名。
- 浏览器补验范围：MoviePilot 自动入口→Sonarr 自动入口，以及用户手填完成目录与入口保留；严格 Linux 自动完成根另由上述真实 SFC setup 隔离断言覆盖。
