# 组件部署与原功能回归

日期：2026-11-20。所有构建、测试与临时工具命令通过 `python -B scripts/project_env.py --` 执行。缓存、协议 fixture、短媒体、数据库和证据只放在仓库 `.runtime/`，没有在开发机安装或运行四个真实资源组件。

## 真实应用协议检查

本轮应用构建位于 `.runtime/verify/component-build/`。以下脚本均通过真实 Go 应用 HTTP API、真实 SQLite 和生产适配器执行；上游使用本地受控协议服务：

| 命令（省略 project_env 前缀） | 结果 | 证据 |
| --- | --- | --- |
| `python -B scripts/verify_external.py --binary .runtime/verify/component-build/nas.exe --root .runtime/verify/component-external-1120 --workflow` | 83项通过，含 MoviePilot 六阶段及暂停/继续、单独刮削确认与不确定回执 | `.runtime/verify/component-external-1120/result.json` |
| `python -B scripts/verify_collection.py --binary .runtime/verify/component-build/nas.exe --worker-binary .runtime/verify/component-build/worker.exe --root .runtime/verify/component-collection-1120` | 27项通过，真实 worker、登记/复制/入库、RSS至原BT流程 | `.runtime/verify/component-collection-1120/result.json` |
| `python -B scripts/verify_downloads.py --binary .runtime/verify/component-build/nas.exe --root .runtime/verify/component-downloads-1120` | 36项通过 | `.runtime/verify/component-downloads-1120/result.json` |
| `python -B scripts/verify_torrent_search.py --binary .runtime/verify/component-build/nas.exe --root .runtime/verify/component-search-1120` | 43项通过 | `.runtime/verify/component-search-1120/result.json` |
| `python -B scripts/verify_engine_bootstrap.py --binary .runtime/verify/component-build/nas.exe --root .runtime/verify/component-engines-1120` | 35项通过 | `.runtime/verify/component-engines-1120/result.json` |
| `python -X utf8 -B .runtime/component_media_check.py` | 52项通过，原五媒体/音乐/备份/鉴权Range | `.runtime/verify/component-media-1120/report.json` |

新增 `scripts/verify_components.py` 使用真实 app → 生产私有 Unix socket 传输 → 隔离 helper 协议 fixture。Windows Python 缺少 AF_UNIX，因此脚本生成仓库内 Go Unix→本地HTTP桥，未更改产品传输，也没有浏览器拦截 API。初始52项通过后补充11秒慢响应、禁用能力和真实Unix监听离线/恢复，最终55项通过，最终干净实例证据 `.runtime/verify/cmp-final/result.json`。11秒检测发现旧私有传输 header10s/client15s不足以覆盖helper22s总预算；产品已改header25s/client30s、浏览器单轮60s，补测实际等待至少10.5秒后正常返回。

复用命令：

```powershell
# --root必须是不存在且包含verify路径段的隔离目录；不得指向用户实际数据。
python -B scripts/project_env.py -- python -B scripts/verify_components.py --binary .runtime/verify/component-build/nas-final.exe --root .runtime/verify/components-new-run
```

上述旧功能回归构建早于本轮仅限新组件的最终回执编号检查；该检查由app临时行为测试、最新组件55项和浏览器验证覆盖。私有传输超时修复不改变原接口载荷；该边界已由最新慢响应断言覆盖。

## 浏览器与最终专项复核

- 真实应用页面未拦截API，主流程30项通过，证据 `.runtime/verify/cmp-1120c/browser-result.json`。包括实际部署POST、确认、镜像层进度、刷新恢复同一任务、服务切换、首次配置入口、空凭据掩码、失败revision重试、丢回执后原UUID恢复及helper离线连接既有服务。
- 375/768/1024/1440宽度×明暗主题共8组：一个dialog、一个form、44px交互控件、无横向溢出；Space/Esc与焦点恢复通过；远程NAS主机名仅显示宿主端口和SSH指引；无pageerror。手机及桌面viewport截图已目视检查。
- 最新生产构建追加真实浏览器跨组件预填5项通过，证据 `cmp-1120c/browser-prefill-result.json`：MoviePilot→Sonarr清理旧建议并使用自身入口，用户手填入口与主动选择完成目录保留。该Windows真实应用不返回Linux根，不能用此声称验证 `/collection-media/library` 自动匹配。
- 独立审查使用实际Vue SFC setup与响应式状态验证23项，覆盖严格Linux根及子目录自动填充、未修改建议清理、用户修改保留、来源种类切换及秘密清空；属于无DOM组件逻辑证据。真实SQLite专项验证完整签名锁内别名、不同revision无绑定、结束窗口不新建、完成任务恢复及旧qB兼容。详情 `quality-review.md`；临时测试源全部删除。
- 浏览器和协议fixture自有进程、Unix socket与临时测试源已回收，浏览器已关闭；补充清理记录 `cmp-1120c/cleanup.json`。保留可复用 `scripts/verify_components.py` 和报告/截图。

## Docker模板和桌面兼容

- 真实 Docker Compose CLI 离线规范化：桌面v1、v2+worker、v3、v3+worker、Linux引擎覆盖、Linux引擎+worker叠加，六种组合均成功。使用真实空环境文件和独立变量快照，无daemon变更。
- 48项临时 Go 断言消费真实规范化JSON，核验四种桌面清单及实际mount模型，拒绝媒体根扩大、只读变写入、卷角色替换、环境/权限覆盖和缺失角色。临时测试源已删除；证据 `.runtime/verify/components-compose/report.json`。
- `python -B scripts/verify_release.py --self-test`，83项通过；涵盖v3双/三镜像、无效字段及缺失组件overlay的发布包拒绝。
- helper六项行为测试覆盖四产品×bind/volume模板、nonce/外来卷、GET无写、四匿名协议、幂等与请求隔离、丢失create响应、SQLite重开与六组件停止门禁。详见 `helper-verification.md`。
- app五组代理测试覆盖Scope/Unavailable/Deploy/TaskAndRetry/Start，包含错误任务编号回执拒绝。临时单测执行后已删除。

## 构建和边界

最终审查修复后 `go vet ./...`、清理临时单测后的 `go test ./...` 均退出0；后者仅为包编译证据。前端严格类型检查与生产构建通过。再次完成Windows amd64应用/桌面/助手构建，以及 `CGO_ENABLED=0` 的 Linux amd64和arm64应用/助手交叉构建，脚本 `.runtime/component_build_check.py`。CodeGraph同步成功，纳入32个变更文件；最终代码审查后的补充检查见 `quality-review.md`。

不将这些结果称为真实Docker部署成功。没有拉取/启动四个镜像、创建真实站点/下载器配置、连接用户账号或修改真实媒体；Linux结果是交叉编译，Windows协议fixture不是Linux权限证明。镜像初始化、NAS UID/ACL、真实回环访问、首次管理员/API Key、断电存储恢复和媒体服务器行为仍需目标NAS/Docker Desktop验收。5GiB文件为稀疏HTTP偏移样本，不代表视频解码或吞吐性能。
