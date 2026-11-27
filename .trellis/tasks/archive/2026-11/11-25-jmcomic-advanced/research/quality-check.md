# JMComic advanced 独立质量检查

检查日期：2026-11-25。角色：质量审查代理。范围：本轮高级搜索、目录选择、批量提交、逐页进度和显式失败恢复的实际调用路径及兼容性。

## Findings (fixed)

- File: `internal/collectionworker/jmcomic.go`、`internal/collectionworker/tools.go`
  - Issue: 默认全本必须保留历史身份摘要，但旧摘要仅包含总页数和章节编号。目录从两章各 2 页变为 1 页与 3 页时，总数及旧摘要均不变，新增目录详情可能因此未被预览到执行链路检测。
  - Fix: 增加 `sameJMComicMetadata`，独立对比冻结的全本编号、有序章节编号、每章页数、所选章节与预算状态；获取前比较持久 `Preview.JMComic`，下载后再次比较本次预览和桥接响应。展示标题不参与判断。历史无目录详情记录继续使用原摘要，默认全本旧摘要不变。
  - Verification: 临时 Go 测试验证 11 个目录比较场景、缺少目录拒绝，以及独立计算历史 JSON 的 SHA256 与新函数一致；确认 2+2 → 1+3 时旧摘要仍一致而冻结目录比较拒绝。全部通过，临时 `_test.go` 已删除。完整 worker 协议漂移案例由协议代理补入复用验证脚本，最终二进制结果交主代理统一记录。

- File: `web/src/components/CollectionJMComicDialog.vue`
  - Issue: 浏览器网络或解码异常经通用错误读取后可能显示英文 `Failed to fetch`，没有中文恢复说明。
  - Fix: 在本入口增加有类型、中文注释的 `comicErrorMessage`；保留服务端 `ApiError` 的受控说明，其余网络／解码错误显示中文连接与重试提示。搜索、单本预览／提交、批量预览／提交五处统一使用。
  - Verification: 前端严格类型检查及生产构建通过；主代理浏览器验收覆盖网络错误和批量未知响应，最终重跑结果由主代理统一归档。

## Findings (not fixed)

本次已检查范围内无待修复实现问题。实际站点可用性、用户账号、Docker 运行和实体 NAS 行为不属于本次离线验收证据，不能由协议 fixture、真实依赖离线传输或构建结果推定已通过。

## Verification

本代理实际执行并核对结果：

- Lint: pass，`python -B scripts/project_env.py -- go vet ./...`。前端未定义独立 lint 命令，未虚构 lint 执行结果。
- TypeCheck: pass，`python -B scripts/project_env.py -- npm --prefix web run build` 内先运行 `vue-tsc --noEmit`。
- Go build: pass，`python -B scripts/project_env.py -- go build ./...`。
- Go test: pass，`python -B scripts/project_env.py -- go test ./...`；清理临时测试后当前包显示 `[no test files]`，该最终命令仅代表包编译检查，不冒称保留了完整单元测试套件。
- 定向临时行为测试：pass，`go test ./internal/collectionworker -run TestTemporaryComic -count=1 -v`，含两组测试及 11 个目录比较子场景，随后清理临时源码。
- Vue/Vite production build: pass，188 modules transformed。
- `git diff --check`: pass；只有仓库现有换行策略的 LF→CRLF 提示，无 whitespace error。
- 清理核对：`internal/collectionworker/jmcomic_quality_temporary_test.go` 不存在。

审查时跟踪到的跨层约束：选章仅进入 preview/private request/worker immutable input，不进入 task 可变字段；新字段 `omitempty` 保持旧 JSON；排序和时间白名单由共享 Go 合同校验；超过预算只能读目录；批次逐条冻结 UUID 和完整提交体，未知结果在预览失效后仍可核对原请求；成功条目不重发；页进度不写持久 `task.bytes` 或 CAS revision；sidecar 在操作专属私有子目录；monitor 在退出前停止并等待；显式重试保留 JMComic 暂存，其他来源保持清理流程。

协作验收边界：真实固定依赖 `jmcomic==2.7.12` 的离线桥接、完整 app/worker 协议、多尺寸双主题浏览器及原回归由其各自负责代理执行；以对应复用脚本报告和主代理最终统一二进制重跑为准。没有访问真实漫画站点、账户或用户服务，没有执行 Docker／NAS 实测，没有提交或归档任务。
