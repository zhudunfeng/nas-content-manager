# Windows UAV 自动初始化交付总结

日期：2026-11-27。任务从`in_progress`推进至`review`；实现及本机验收完成，未提交、未归档，`completedAt`保持空。不能用任务状态代替下列实际证据。

## 实际交付

- Windows amd64 native + managed、UAV明确启用、无显式自定义Python且实际能力不可用时，基础媒体ready后后台准备固定Python/UAV/yt-dlp/FFmpeg。随包有效依赖优先，其次profile已发布依赖。首次成功后串行受控重启载入；下载期间媒体仍可用。
- 原生Go固定HTTPS来源、大小、SHA256和安全ZIP映射；无终端pip/编译、无系统Python/全局PATH依赖。791项库存加2份精确元数据，最终位置严格校验和真实worker探针后才原子发布。
- 中文托盘/IPC阶段及当前物料字节进度、一次性失败状态、显式重检/下次启动重试；取消/退出等待Job进程及文件释放，旧任务不可迟到激活。
- 新增长期`scripts/verify_uav_bootstrap.py`及确定性资源生成器；同步用户说明、配置注释、release基础包使用提示和三份长期spec。
- 已生成带内嵌网页的开发验证包：`.runtime/verify/windows-uav-auto-init/release/qiku-0.1.0-windows-amd64-development.zip`。首次自动准备需联网；发行身份保持development。没有修改现有`E:\test`程序或用户配置数据。

来源：`internal/uavbootstrap/`、`internal/desktop/uav_initialization.go`及`collection_*`/`launcher.go`；`scripts/uav_bootstrap.py`、`verify_uav_bootstrap.py`、`release.py`；`research/final-validation.md`。

## 可复用决策及原因

- 初始化只在已启用且未交由管理员维护的托管UAV执行。false、saved disabled/external、错误显式Python均零安装，避免隐式改变已有配置优先级。JMComic保持独立，不自动安装。
- 仅在已核验的profile `collection-managed/uav-bootstrap`命名空间写入；`stage-<32hex>`→不可变`gen-<32hex>`→有界严格`current.json`原子选择。下载/探针失败不能污染已选代次。
- 真实worker能力是可用性的最终权威；初始化ready和静态校验不能替代它。后台只更新身份检查过的短锁状态，生命周期变更由controller串行处理。
- 网络瞬态最多3次可取消尝试；完整性/来源/清理失败不重试。普通失败不无限循环，清理失败身份跨层保留并阻止不安全重启。
- payload是装配后安装载荷，不能冒充原始wheel；保留原`builtWheelSha256`来源回执。局部`.gitattributes`固定JSON的LF精确字节。

长期规则来源：`.trellis/spec/backend/uav-windows-contract.md`、`jmcomic-windows-contract.md`、`cross-platform-contract.md`；方案见`info.md`及两份research研究记录。

## 踩坑和约束

- 两轮早期联网集成发现：stage中探针成功，改名为更长旧generation前缀后，curl_cffi原生DLL因Windows路径限制加载失败。规范路径小于260不充分；本机248/249/250成功、251–260失败。现短前缀保留完整128位身份、最终预算248单元，并强制改名后真实探针再发布；不能通过缩短测试profile绕开。
- 自动激活的通用提示曾遮蔽worker实际安全Reason，现状态和日志保留根因。
- 构建下载共享临时名会遗留.part、阻塞重试或清理其他尝试；改为每次独享TemporaryDirectory，固定SHA缓存身份不变。
- 实际只读ACL同时拒绝EXE读写独占测试，EXE检查必须在ACL恢复后执行；profile runtime文件仍在退出后立即验证。
- 本轮诊断目录`.runtime/tmp/ur-f900a5tx`删除被自动审核拒绝，保留诊断材料，未换方式绕过；新增临时Go测试源均已清除。

来源：`research/native-path-analysis.md`、`research/desktop-implementation.md`、`research/runtime-implementation.md`、`research/quality-review.md`。

## 验证证据

- **最终包真实联网/离线7组**全部通过：20固定物料、最终路径真实探针、认证API及能力ready、错误代理/停用/退出、H.264编码→ffprobe1.0秒→解码、全部原生映像退出释放、断网重启全部摘要/mtime/current原样复用。总102.676秒，联网初始化65.732秒。
- **最终包.env11项**、主桌面生命周期与真实托盘/10尺寸图标、**真实双插件25项**均通过。双插件实际观察漫画26个/视频25个原生映像，覆盖独立故障、嵌套Job、父强杀、取消、哨兵保留、映像替换和立即重启。
- runtime完整临时行为54.538秒、最终同深度中文路径重定位29.352秒、desktop九组行为/真实Job及实际Reason回归通过。独立完整Initialize探针失败/取消注入52.862秒，旧选择保留与候选清理通过。临时测试已删除，空go test仅为编译证据。
- 最终gofmt、vet/test、Windows release构建、Linuxamd64/arm64 core+worker四次交叉构建、5个Python AST、资源逐字节`--check`、相关diff检查通过。发布边界131项、真实ZIP24文件通过。
- ZIP SHA256：`91b834dfd44f0010e5dc00be24ba155a7d795d7ff52be421f022e429d83c7291`。

详细命令和EXE摘要：`research/final-validation.md`。机器报告：`.runtime/verify/windows-uav-auto-init/{bootstrap-report,env-final-report,collection-final-report}.json`。独立审查无剩余代码阻断项，见`research/quality-review.md`。对外记录：`docs/verification.md`。

## 未完成及下一步

- 本次用户要求的代码实现和本机验收已完成；用户审阅后决定提交及升级现有程序。升级需从托盘退出旧实例，保留.env及qiku-data，再切换新包。
- 干净Windows普通用户、真实站点下载、Linux/Docker/ARM实际执行及正式分发许可/对应源码人工审查仍需单独安排；不以本机、交叉编译和一秒样本替代。
- 当前首次激活会短暂重启本机服务，正在播放/下载连接可能中断；不承诺无中断安装。不支持范围不自动安装。
- 用户原有417份历史Trellis删除/归档及其他无关工作树修改保持原状，本任务未清理、提交或归档它们。

下一位维护者从本总结、`prd.md`和`research/final-validation.md`继续，无需重做已经通过的主体实现。
