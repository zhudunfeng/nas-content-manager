# JMComic 启用策略对齐总结

日期：2026-11-27（Asia/Shanghai）。状态：本机实现与验收完成，review；未自动提交或归档。此总结以实际实现/验证为依据，不以生命周期状态代替证据。

## 实际交付

- JMComic 与指定线程 UAV 策略一致：Windows x64 原生托管、明确启用、无显式自定义 Python 时，先复用有效随包或 profile 依赖，缺失再自动准备九份固定官方材料，最终位置真实探针成功后原子发布和受控激活。
- 共用 `internal/runtimebootstrap/` 安全引擎，插件薄封装与资源严格独立。漫画位于 `jm-bootstrap`，不附加视频工具；UAV 原资源与已发布 profile 身份兼容。
- 桌面 JMComic→UAV 串行整批准备、普通失败隔离、取消/清理屏障、至多一次激活及按 kind 状态；显式 false/disabled/external/自定义 Python 零安装。
- 增加长期 `scripts/verify_jmcomic_bootstrap.py`；资源生成脚本共用构建材料处理。Windows 配置示例、漫画/视频/发行文档、验证记录及相关 specs 已更新。
- 本地交付归档：`.runtime/verify/jmcomic-enable-runtime/delivery/qiku-0.1.0-windows-amd64-development.zip`；仍为开发验证包，首次依赖准备需要网络。

来源：`prd.md`、`research/parity-design.md`、`research/{runtime-implementation,desktop-implementation,integration-harness}.md`；实现 `internal/{runtimebootstrap,jmcomicbootstrap,uavbootstrap}/`、`internal/desktop/collection_initialization.go` 与相关 config/managed/controller/launcher/tray 文件；脚本 `scripts/{bootstrap_common,jmcomic_bootstrap,uav_bootstrap,verify_jmcomic_bootstrap,release}.py`。

## 可复用决策

- 两个插件共享安全机制，保持独立的配方、plan 缓存、所有权、清单和严格加载器，避免同一完整性规则出现两套漂移实现。
- 状态进度和真正能力分开：初始化 ready 不能写成 worker available；兼容旧单对象 IPC，同时提供按 kind 的独立事实。
- 生命周期副作用只在 controller 串行循环执行；整批结束后统一激活，用户操作、当前资格变化、worker 退出或媒体退出待处理都可阻止过期激活。
- 取消/清理错误不能普通网络失败化；保留错误身份并等待自有 Job 释放后再退出或替换。有限失败后只允许主动重检或下次启动重试。

长期规则已回写 `.trellis/spec/backend/{jmcomic-windows-contract,uav-windows-contract,cross-platform-contract}.md`，索引在 `.trellis/spec/backend/index.md`。

## 踩坑与约束

- Windows 原生扩展要在最终 `gen-<32hex>` 路径真实加载；stage 成功不证明重命名后成功。保留 248 UTF-16 原生路径预算，漫画短命名空间为 `jm-bootstrap`。
- 初始化完成与媒体服务退出可能同时 ready；不得根据旧 ready 快照重新激活。审查补充 pending `serviceDone` 门禁，且不消费退出错误来判断状态。
- 随包说明必须区分“首次完全离线需完整包”和“成功准备后可离线复用”；最终归档已同步说明，三个 EXE 与受测候选逐字节一致。
- 构建/检查使用项目环境包装器，未污染系统 Python/PATH 或用户级环境。保留原工作区大量无关改动，未改用户既有安装或数据库 schema。
- 按 AGENTS 删除本轮临时单元测试源码；保留长期集成脚本与真实日志。CGO 禁用导致 race detector 未执行，不能列为通过。

来源：`research/quality-review.md`、`research/runtime-implementation.md`、`research/desktop-implementation.md`、`research/final-validation.md`。

## 验证证据

- 最终候选漫画/双插件真实自动准备 7 组，单漫画准备 28.167 秒、双插件 108.820 秒；真实 worker/认证 API、离线全文件 SHA/mtime/current 不变、一次载入、失败/停用/退出均通过。
- 最终候选 UAV 回归 7 组、环境 11 项、完整双插件真实运行时生命周期 25 项；实际漫画 26/视频 25 个原生映像及退出释放通过。主桌面/托盘、10 尺寸图标、即时替换和配置账号保留通过。
- 共享内核真实缓存故障注入、漫画 620/视频 791 文件最终位置真实探针、桌面 11 组行为通过；本轮临时测试源码已删除。
- 最终 Go vet/test、格式、Python AST/新增脚本类型注释、Vue 类型、两套资源确定性、Linux 两架构 core+worker 四次交叉构建、发布边界 131 项及最终 ZIP 24 文件核验通过。空 Go 套件只记编译，Linux 未执行。

主证据：`research/final-validation.md`，包含精确命令和交付摘要；独立审查 `research/quality-review.md`。机器报告 `.runtime/verify/jmcomic-enable-runtime/final-{bootstrap,uav-regression,env,collection}-report.json`；临时测试原始日志 `.runtime/runtimebootstrap-{temporary-tests,boundary-final-tests}.log`。

## 未完成与下一步

- 无待修复具体代码问题。用户可审阅并提交；任务不归档，未替换现有安装。
- 切换新程序时正常退出旧实例，保留 `.env` 与整个 `qiku-data`。第一次成功准备会短暂受控重启服务，可能中断现有播放/下载连接，不自动重发业务任务。
- 干净 Windows 普通用户、真实漫画/视频站点及账号、Linux/Docker/ARM 运行时、正式分发许可/对应源码人工审查仍需单独验收；不把本机隔离、缓存 transport、短媒体样本或交叉编译扩大为这些结论。

用户文档来源：`docs/{jmcomic-windows,jmcomic,uav-windows,windows,release,verification}.md` 与 `.env.windows.example`。
