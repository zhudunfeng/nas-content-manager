# 外部自动化与 MoviePilot 六阶段实施复核

日期：2026-11-20。当前实现覆盖本任务PRD；代码尚未提交，不连接用户真实服务，不部署或安装第三方套件。

## 实施结果

- 连接入口三步：选择服务、填写地址/既有凭据、检测连接。服务端探测版本与能力，不要求普通用户选择API版本。高级完成目录折叠，外部入口和独立预览登记保留。
- MoviePilot精确3.2.8具备元数据/资源搜索、订阅、下载及单任务启停、单文件复制整理、单文件刮削、媒体库索引刷新。其他服务按实际能力展示，AutoBangumi不假造全局历史或通用搜索。
- 四适配器固定认证、路径与类型化载荷；无任意HTTP/命令/安装代理。读取与动作均有预算、来源授权及会话边界。
- schema4只追加连接/动作/选择引用。秘密独立保存，动作先持久化再派发，稳定UUID、来源revision和一次性选择绑定意图。重启不重发，不确定动作先只读对账或记录人工未确认。
- 搜索/预览不自动执行，整理核验真实配置不覆盖策略且复制不刮削，刮削独立确认覆盖。外部回执/完成/本系统扫描入库分别表达。

## 复核修正

独立Trellis复核完成8项临时行为测试并清理源码，修正：重叠目录配置导致有效覆盖策略歧义；null纳秒版本；Linux路径大小写与Windows盘符根；启停回执hash/下载器/动作核验；未知调度状态不能视为空闲；有副作用GET使用独立禁用长连接的transport，防止自动重放；数据库失败后清理未引用秘密文件。

界面实测修正中文“已暂停”状态识别；未知受控状态保留明确暂停/继续入口。修改TMDB或类型立即清除旧计划，目标变化具有同一watch约束。浏览器脚本曾遇到locator strict、重复订阅和CSP字符串求值问题，均属于验证工具/重复fixture问题，没有放宽正式CSP或把失败运行记为通过。

## 已执行验证

| 项目 | 证据与结果 |
| --- | --- |
| 最终四协议+完整MP流程 | `scripts/verify_external.py --workflow`，`.runtime/verify/external-complete-20261120/result.json`，83断言通过 |
| 最终单次实际应用浏览器 | `.runtime/external-app-ui-acceptance-evidence/report.json`，passed=true、errors=[]，真实动作API核验七类提交回执；精确根为`external-browser-acceptance-20261120` |
| 界面fixture | `.runtime/external-ui-evidence/report.json`，四尺寸×两主题、单窗口、焦点/Escape、UUID恢复/人工未确认；仅呈现证据 |
| 原资源收集 | `.runtime/verify/external-collection-regression-20261120/result.json`，27通过，真实app+worker；未重跑真实web/remote工具 |
| 下载/搜种/托管接入 | `.runtime/verify/external-download-regression-20261120/result.json` 36；`external-search-regression-20261120/result.json` 43；`external-bootstrap-regression-20261120/result.json` 35 |
| 五媒体/音乐/备份/大偏移 | `.runtime/verify/external-media-regression-20261120/result.json`，52通过；5GiB稀疏文件不证明播放性能 |
| Go | 最终完整vet、删除临时单测后的包编译通过；Windows主程序构建及Linux amd64/arm64交叉构建通过 |
| 前端 | 最后修改后的严格vue-tsc、Vite生产构建通过；移动375/768/1024/1440与双主题无溢出 |
| 工作流 | Trellis implement/check上下文有效，CodeGraph已同步23个变化文件，Python验证脚本AST语法通过，Git空白检查通过，新增临时`*_test.go`为零 |

原有业务回归使用本轮前段二进制；后续后端变化限新外部工作流，由最终83项覆盖，不声称所有回归均使用最终二进制。构建与临时命令通过`project_env.py`，缓存和证据保留在`.runtime/`。

收尾时三轮浏览器隔离服务均通过各自stop文件正常结束，退出码0；浏览器finally关闭，专属Vite停止。只读进程枚举确认无本轮`.runtime/verify/`应用及`verify_external.py`服务进程。保留忽略目录中的测试证据，不把调试中断报告当作最终通过证据。

## 契约同步

`docs/collection.md`、`docs/api.md`、`docs/verification.md`、README，backend external-collection/runtime/collection/index与frontend app-contract均记录真实版本、作用范围、私密快照、schema4、幂等/不确定态和易用性要求。任务`api-contract.md`与最终DTO、60KiB/15分钟/100/600限制一致。

## 真实环境限制

- 六阶段精确版本为3.2.8，未声明所有MP3兼容；内置hash格式以40/64十六进制保守支持，未知插件回执不拼造编号。
- 索引刷新执行`mediaserver_sync`，更新MoviePilot从媒体服务器观察到的索引，不等同Emby/Plex扫盘。提交回执不是本次执行完成证明。
- `ManualTransferItem`没有overwrite_mode字段。通过真实配置`never`、无歧义根与提交前重读控制；不能把外部管理员并发改配置纳入本地事务。远程符号链接解析仍由MoviePilot负责，不授予NAS本机路径权限。
- 刮削可能覆盖NFO/图片，已显式确认；未验证用户实际插件、真实外部媒体/索引、权限、跨卷、Docker部署或吞吐。
- 没有执行Git提交、发布、部署或任务归档。实现完成后保持review供用户审阅。
