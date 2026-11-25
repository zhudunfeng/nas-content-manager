# JMComic 高级能力最终主线交付记录

日期：2026-11-25。状态：本地实现与验证完成，等待用户审阅提交。承接原可选插件，已补齐高级搜索、选章、批量、逐页进度及显式失败后的可信页面复用；没有声称封装全部上游功能。

## 交付范围

- 搜索支持固定 `mr/mv/mp/tf` 排序与 `a/t/w/m` 时间范围，旧请求保持默认 `mr/a`。固定 API 不支持的关键词分类筛选没有暴露。
- 显示真实章节页数，全本／选章绑定不可变预览。超预算全本只展示目录，减选后重新预览；每个选中章节必须完整。
- 最多十本的全本批量，逐本预览、UUID、提交体与结果。未知响应保留原 body 重试，即使预览过期也先核对既有任务，已受理条目不重复发送。
- 获取中展示已校验页数／字节与复用页数，不改变最终产物 bytes、持久 revision 或发布权限。
- 同任务显式重试可复用已验证完整图片；绑定、目录、来源语义、图片、大小、SHA256 与链接检查不通过则拒绝或重新获取。恢复单位为完整页，不是 HTTP 字节断点续传；不自动恢复不确定状态。

主应用、网页与 worker 必须同步更新；真实 Python 桥接嵌入 worker。依赖仍锁定 `jmcomic==2.7.12`，数据库仍为 schema 6。

## 最终统一产物

位于 `.runtime/bin/jmcomic-advanced-final/`，在独立质量补丁及最终网页构建之后新鲜生成。

| Windows 文件 | 字节 | SHA256 |
| --- | ---: | --- |
| `nas.exe` | 21645824 | `d2783d0f5345a3ad8d98df611d72ae41a4cfd66c25e1b86c41c3acaf9af48d2a` |
| `nas-collection-worker.exe` | 12535296 | `ed50895f808a6d978a5025862c8461c15e4aefd6f40bf6aaf255110ad0865715` |

Linux amd64／arm64 主服务和 worker 共四个无 CGO 交叉构建成功；只作为构建证据，未执行 Linux 二进制。全部六个产物的大小与摘要位于工程门禁结果，未生成正式发行包。

## 行为证据分层记账

| 项目 | 最终通过数量 | 证据根（仓库内） |
| --- | ---: | --- |
| 高级应用／worker 协议 | 144 | `.runtime/verify/jmcomic-advanced-20261125T073227515799Z-1ec0d7/` |
| 高级浏览器 | 50 | `.runtime/verify/jmadvanced-final-browser-20261125-1/advanced-browser-153252/` |
| 原 JMComic 协议 | 66 | `.runtime/verify/jmadvanced-final-jmcomic-20261125-1/` |
| 原漫画浏览器 | 38 | `.runtime/verify/jmadvanced-final-oldbrowser-20261125-1/browser-153252/` |
| UAV 五站协议回归 | 102 | `.runtime/verify/jmadvanced-final-uav-20261125-1/` |
| 核心媒体／资源收集／发布边界 | 44／27／83 | `.runtime/verify/jmadvanced-final-baseline-20261125-1/` |
| 固定真实依赖离线桥接 | 22 检查点 | `.runtime/verify/jmcomic-real-bridge-b2944294a242457a83df76bbfb410107/` |

前六行使用同一最终 Windows 应用／worker 和最终网页，协议中的上游是受控 fixture，不能称为真实站点下载。最后一行执行真实 `jmcomic==2.7.12` 客户端、下载器、图片解码与钩子，但替换传输并禁止真实网络，`realNetworkCalls=0`；实际证明三页复用仅一页新图片传输。不同层次的数量不合并为同一测试套件。

高级协议还验证选章身份、实际鉴权图片读取、超预算拒绝、单调进度、业务 revision 不逐页改变、过期／重启幂等，以及全本从 2+2 页漂移至 1+3 页时，尽管总数及旧身份相同仍拒绝执行。原回归覆盖基础本地登记、原漫画下载阅读、UAV 五站、核心媒体与发布边界。

## 独立质量与工程门禁

- 独立检查见 `quality-check.md`。修复全本分章漂移绕过历史摘要的问题，在获取前后额外比较冻结目录，保留历史无详情记录兼容；增加本入口中文网络错误提示。
- 临时行为测试涵盖旧 JSON／摘要、章节比较、进度边界、重试和 sidecar；执行成功后全部清除。最终 `go test ./...` 为清源包编译，不冒充保留的单元测试。
- 最终 `go vet ./...`、`go test ./...`、变更 Go 文件 gofmt、五份 Python AST、`git diff --check` 与四个 Linux 交编通过。统一工程证据 `.runtime/verify/jmadvanced-final-checks-20261125-2/result.json` 为 passed，记录源码与六个产物摘要，`temporaryGoTests=[]`、`sourceDrift=[]`。
- 前端严格类型检查与 Vite production build 由独立检查者在最后补丁之后运行，188 modules；无独立 frontend lint 脚本，未虚构执行。多尺寸双主题及视觉回读详见 `browser-validation.md`。
- 上游研究、架构研究、PRD、API、使用文档及后端／前端 spec 已同步。Trellis 上下文、最终源码／产物／进程回读见 `.runtime/verify/jmadvanced-mainline-final-audit-20261125-1/result.json`。

## 保留的失败与修复记录

- 早期旧协议编译时点仍使用 PNG fixture，固定 `.jpg` 门禁拒绝；将样例修为真实 JPEG 后，在新根原 66 项及最终全部回归通过，不放宽生产格式检查。
- 第一轮高级协议在并发进度读取时暴露 Windows 原子替换共享冲突；生产 Python 对 PermissionError 做有界重试、保留旧完整快照。离线故障注入验证瞬态成功及永久冲突有界失败，最终高级协议通过。详细根因见后端与协议报告。
- 最终工程第一轮辅助脚本未解析项目 Go 工具路径，在实际 Go 检查前退出；失败证据保留在 `jmadvanced-final-checks-20261125-1/`。第二轮改用 `.tools/go/bin/go.exe` 后通过，不将工具定位失败计作产品失败或通过。

## 交付边界

未访问真实站点、用户账户、用户媒体或用户服务；未验收公网可达性、Docker 运行、Linux 实机及物理 NAS。未接入账号／Cookie／收藏夹、PDF／ZIP 或任意 Python／YAML 插件。能力子集及限制详见 `docs/jmcomic.md`。

本轮自有应用、worker 和浏览器已关闭；无临时 `_test.go` 遗留。没有自动提交、部署、归档或记录已提交会话。本任务进入 review，原 UAV 任务维持 review，等待用户审阅提交。
