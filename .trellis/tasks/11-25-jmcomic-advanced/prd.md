# JMComic 高级搜索与下载能力

## Goal

在已完成的可选 JMComic 插件上补齐高级搜索、章节选择、批量提交、逐页进度及失败页恢复，保留现有来源、预览、持久任务、发布与漫画阅读流程。

## Requirements

- 搜索增加上游 API 确实支持的排序与时间筛选，所有值采用固定白名单，保留旧 query/page 请求兼容。上游 API 忽略分类参数，本轮不展示无效分类筛选。
- 预览显示章节和页数，允许全本或明确选章；选择必须绑定预览、任务身份、worker 校验和最终文件完整性。
- 支持有界多编号批量下载，独立预览确认与稳定请求编号，展示逐项成功或失败；不把部分受理当作整体失败重发。
- 获取中展示已完成页数、总页数及已获取字节，阶段完成后仍由现有发布／入库状态负责。
- 获取失败后的显式重试可安全复用已验证页面；元信息改变或缓存不可信必须拒绝／重新获取，取消和未完成结果不得发布。
- 原有 local、web、UAV 等来源及旧 JMComic 请求兼容，不修改历史任务或用户实际数据。
- 中文方法／字段／配置注释、关键脱敏中文日志、显式类型；所有验证通过 project_env，临时产物放 .runtime。

## Assumptions

- 用户“进一步接入”承接上一轮建议，默认实施上述四类核心使用能力，无需再确认合理技术细节。
- 账号／Cookie／收藏夹、PDF／ZIP、任意 Python／YAML 插件另行规划，不在本轮开放。
- 不访问真实漫画内容或用户账号；用真实依赖与离线响应验证，明确目标环境待验边界。
- 新建任务独立于已处于 review 的 uav-downloader-plugin；不改其任务状态，不自动 commit／归档。

## Acceptance Criteria

- [x] 高级搜索筛选可通过 UI/API 生效，非法值拒绝，旧搜索行为兼容。
- [x] 章节选择端到端绑定，只发布完整选中章节，伪造和重复选择拒绝。
- [x] 批量提交限制数量并逐项显示结果，未知响应重试保留相同身份。
- [x] 真实 worker 下载期间可观察单调有界页数／字节进度，不泄露地址和凭据。
- [x] 失败后重试复用真实已验证页面，损坏／链接／元信息漂移不能绕过完整性检查。
- [x] Go vet/test/build、Vue strict typecheck/build、新增离线／协议／浏览器验证与原回归通过。
- [x] 使用文档、API 与 Trellis spec 同步，本轮生成的临时单元测试清除。

## Research References

- research/upstream-advanced.md（固定 2.7.12 真实依赖源码研究）
- research/architecture-advanced.md（跨层身份、进度与恢复路径研究）

## Decision

采用固定受控适配器的增量扩展。批量下载沿用每本独立任务，不创造第二套协调器。对外仅暴露有界枚举、编号、章节选择和进度，不开放执行配置。

## Frozen Cross-Layer Contract

- 搜索新增可选 `orderBy:mr|mv|mp|tf`、`time:a|t|w|m`，默认仍为 `mr/a`。
- 预览请求新增 `chapterIds?:string[]`；空或省略代表全本，重复／未知编号拒绝，显式选择按上游章节顺序规范化。任务直接继承服务端不可变预览，不接受另一次选章。
- 预览新增 `jmcomic?:{albumId,pageCount,chapters:[{id,title,pageCount}],selectedChapterIds,overBudget?:boolean}`，`pageCount` 是所选总页数。全本超过本次预算可只读展示章节目录，超预算预览不可创建或获取任务；重新选择较少章节后再预览。
- 任务详情可附加 `jmcomicProgress?:{completedPages,totalPages,bytes,reusedPages}`；bytes 为已验证页面字节，包含复用，不是网络流量；不改持久 task.bytes 的语义。
- 默认全本 JSON 与旧身份摘要保持兼容，所有新意图字段 `omitempty`；新详情不直接扩展旧身份摘要结构。
- 新记录另用冻结的 `Preview.JMComic` 比较每章页数与选择，覆盖总页数未变的分章漂移；旧无详情记录沿用原摘要兼容。
- 进度／checkpoint 在 `private-state/jmcomic/<opid>/` 子目录；不得进入私有状态根的操作JSON扫描或 staging 发布清单。
- 批量最多10本，只支持全本，逐本原API编排、独立UUID、逐项结果；未知提交重试保留完整原body，即使预览已过期也先核对原任务。

## Implementation Plan

1. 上游与跨层研究，冻结增量协议（完成）。
2. 后端固定桥接／选章／进度／可信恢复与前端筛选／批量／章节交互并行实施（完成）。
3. 独立集成及真实依赖离线验证，浏览器多屏双主题，修复后运行原回归（完成）。
4. 独立质量检查、spec／使用文档同步，临时测试清理后进入 review（完成，详见 research/final-validation.md）。

## Definition of Done

实现、独立检查、真实依赖离线与页面验证、文档更新完成；明确未实际验证的站点／容器／NAS边界，等待用户审阅提交。
