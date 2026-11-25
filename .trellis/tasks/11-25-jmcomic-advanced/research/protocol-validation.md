# 高级 JMComic 协议集成验证

## 范围与证据边界

修改范围仅为 `scripts/verify_jmcomic.py`、新增 `scripts/verify_jmcomic_advanced.py` 与本记录。
复用原隔离主服务、worker、来源和媒体样例生命周期；每次运行创建 `.runtime/verify/` 下的全新根，不覆盖先前证据，不访问用户实例或实际漫画站点。

证据标签明确固定：`realApplication=true`、`realWorker=true`、`pythonBridgeExecuted=false`、`upstreamProtocolFixture=true`、`realSiteSearch=false`、`realSiteDownload=false`、`dockerExecuted=false`。
Go 协议样例实际生成 JPEG，经过真实获取、清单校验、发布、扫描及鉴权阅读；它不能证明真实 Python 下载器或可信页面缓存复用。真实依赖离线验证由后端代理的独立脚本负责。

## 可复用实现

- 原工具保留全部旧协议场景；`preview` 增加可选章节参数，原省略字段的调用不变；阅读断言支持明确页数，默认仍为四页。
- Go 样例支持 `orderBy/time`、有序章节目录、选章、`checkpointDir/binding`，只把安全枚举、编号和布尔状态写入调用记录，不记录查询词、路径、代理或绑定值。
- 进度只写入 worker 指定的私密任务目录，格式为 `completedPages/totalPages/bytes/reusedPages`，不在媒体目录夹带控制文件。
- 复用主 CLI 的安全新根验证和 `--serve`。新增验证通过子类复用原服务及阅读逻辑，没有另造服务协调器。
- 清理失败时结果不会保留 `passed=true`；静态与 CLI 检查不计入行为断言数。

## 固定样例

| 条件 | 行为 |
| --- | --- |
| 普通查询 `fixture` | 第一页 1001/1002，第二页 1003，与旧场景兼容 |
| 查询 `advanced`、`1010`、`1011` | 返回 1010/1011 两个高级样例 |
| 查询 `empty/error/slow` | 空结果、受控错误、三秒搜索延迟 |
| 默认漫画 | 11/12 两章，各两页；固定原始章节序号目录 |
| 1006/1007 | 三页不完整结果／额外非图片文件，必须隔离 |
| 1008 + `fail-download` | 可控失败，删除标志后显式重试 |
| 1009 | 45 秒下载，供取消验证 |
| 1010 | 11=2、12=3、13=3，共八页，每页 850ms，可观察运行中进度 |
| 1011 | 11=2、12=100，共 102 页；默认预算 100 时只读目录，选 11 可下载 |
| 1012 + `metadata-drift` | 默认两章 2+2；标志存在时变为 1+3，总数及旧全本身份保持不变，用于拒绝分章漂移 |

## 行为覆盖

- 16 组排序／时间组合实际到达协议工具；非法枚举、虚假分类、未知参数、超长查询拒绝；旧 query/page 和默认 mr/a 兼容。
- 完整目录、逐章页数、默认／空选择全本、逆序选择规范化、重复／未知／非法章节拒绝；默认全本身份与旧 JSON SHA256 精确一致。
- 应用不接受任务时另传章节；worker 独立拒绝用预览替换选择；稳定 UUID、重复资源、不同选章独立身份及预览过期后同 body 确认。
- 真实选章 11+13 五页、另选 12 三页、超预算后改选小章两页；精确清单与实际固定路径一致，无未选章节或控制文件发布，每张图片均实际鉴权读取。
- 获取中四字段进度单调有界、能观察多个中间值、`acquiring` 业务 revision 保持稳定；最终 task/operation/manifest/真实文件 bytes 一致，不重复累计实时进度。
- 应用及 worker 分别拒绝超预算预览创建操作；进度子目录不会被重启的根级操作 JSON 扫描误解析；过期及重启后不重复获取。
- 公开元数据与日志不包含测试密码、worker 令牌、checkpoint/output/proxy/binding 字段。
- 1012 默认全本目录由 2+2 漂移为 1+3 时，使用原预览的真实操作必须失败、无清单／字节和发布目录，即使旧身份摘要与总页数相同。

## 已执行检查

所有命令均通过 `python -B scripts/project_env.py -- ...`，缓存与临时目录位于仓库 `.runtime/`。

- Python AST、21 个方法的中文说明与参数／返回类型检查通过。
- 新高级 CLI `--help` 通过；已有根和 `.runtime/verify/` 外的根均退出 2 且目录存在状态不变。
- `git diff --check` 通过。
- 协议第二轮：139 项通过；证据 `.runtime/verify/jmcomic-advanced-20261125T072323564656Z-6c5a7d/result.json` 与 `advanced-evidence.json`。两次选择实际观察页面序列 0..5、0..3，分别保存 66/69 次快照；获取中 revision 均为 3。
- v2 生产二进制与最终 Permission 类重试 fixture：旧 139 项再次通过；它发生在质量补丁之前，不作为最终 1012 回归结论。
- 最终统一构建 `.runtime/bin/jmcomic-advanced-final/nas.exe` 与 `nas-collection-worker.exe`：**144 项全通过，退出 0**。包含新增 1012 默认全本分章页数漂移拒绝的五项回归。
- 最终证据根：`.runtime/verify/jmcomic-advanced-20261125T073227515799Z-1ec0d7/`；`result.json` 保存通过／边界，`advanced-evidence.json` 保存 80/67 次进度快照，页面完整覆盖 0..5／0..3；`protocol-checks.json` 保存最终二进制 SHA256 及清理复核。
- 进程正常退出并完成 `harness.close()`；复核本轮应用／worker 日志中两个专属回环监听均已关闭。没有保持运行的本代理服务，也未停止其他代理或用户的服务。
- 原 66 项与浏览器回归由主代理运行，不在此重复或合并统计。

## 首轮发现与处理

首轮在 66 项后失败，五张 JPEG 已全部生成，但进度停在四页并留下 `progress.json.tmp`。
证据保留在 `.runtime/verify/jmcomic-advanced-20261125T072137283574Z-1520ee/`，不能当作通过。
脱离并发读取的独立 fixture 下载成功，定位为 Windows 读取快照句柄短暂阻挡原子替换。

fixture 对 `os.IsPermission` 的原子重命名增加 25 次 × 20ms 有界重试，其余错误仍失败，绝不删掉旧快照造成非原子窗口。
生产 Python 的对应修复由后端代理实现并用独立依赖离线故障注入验证。
一次诊断执行复用了首轮工具二进制，因此在该失败根的私密调用日志多出一条诊断 download 记录；它不影响后续全新根的计数，也未被计为原端到端通过。

## 浏览器专用服务

使用最新构建时指定全新根；示例根必须不存在：

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_jmcomic_advanced.py --binary .runtime/bin/jmcomic-advanced-final/nas.exe --worker-binary .runtime/bin/jmcomic-advanced-final/nas-collection-worker.exe --root .runtime/verify/jmadvanced-browser-final-20261125-1 --serve
```

`browser.json` 仅提供隔离 base、sourceId、私密 credentialFile 和 stopFile 的位置。账号值不打印到共享日志。
创建该根的 `stop` 文件触发自有服务清理；不要停止任意端口或用户进程。

## 未验证边界

未执行真实站点搜索／下载、账号／Cookie、Docker、物理 NAS、Linux 实机及网络质量验收。
协议 fixture 的 `reusedPages=0` 是明确的样例事实，不能据此宣称生产缓存算法已验证。
本代理未修改应用／worker／前端业务实现，未生成临时单元测试类，未自动提交或归档。
