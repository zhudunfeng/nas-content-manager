# UAV 最终主线验证与交接

日期：2026-11-25（Asia/Shanghai）。基线提交：`e973f7f93796afc5be0fe3b43955dd352bbb0b51`；工作区变更未提交。任务保留在 `review`，按 `local_complete_target_pending` 交接，不归档既有 JMComic 或当前任务。

## 已交付行为

- 固定可选 `uav` 来源与能力：Jable、MissAV、Hanime1、Huangguo 四站搜索、元数据预览、单视频/单集下载，复用原持久任务、稳定 UUID、取消/重试、发布和视频库扫描。
- 黄果使用真实观察到的分集链接，规范 ID 为 `huangguo-<作品>-ep-<集数>`；不合成缺失集、不下载整剧。每页最多 100 条单集、22 秒展开期限，超限明确失败。
- 固定 UAV 3.8.4 提交 `d40f09457fbd38dd7dc94580637242afafaf3939`、源码归档 SHA-256 `88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41`，保留 Apache-2.0 声明。独立 worker 执行受限 Python 桥接，运行时不安装包。
- 所有有限 HLS 资源登记在操作作用域代理，按每跳原始 HTTPS 源与资源扩展重算黄果访问头；认证值不进入公共响应、持久元数据或下载器全局请求头。保留原 TLS 与公网访问策略。
- 精确 AES-128 16 字节密钥；闭合单 Range 的 206、Content-Range 与实际字节数必须吻合，确认上游 EOF 后才释放末字节。下载后本地重封装、完整解码并比较 HLS 时长下界，再走原独立产物/发布门禁。
- SQLite 保持 schema 6；新增选择字段使用 `omitempty`，原 web/local/remote/JMComic 请求序列化与摘要兼容。

使用说明：`docs/uav.md`；接口：`docs/api.md`；可执行契约：`.trellis/spec/backend/uav-contract.md`。本地原生 worker 默认关闭 UAV，需按文档安装固定依赖并设置启用参数；本轮未修改用户实例配置。

## 最终产物与工程门禁

实际构建目录：`.runtime/bin/uav-final-20261125-2/`。

| 产物 | 字节数 | SHA-256 |
| --- | ---: | --- |
| nas.exe | 21634048 | 849f5d799642bd285f1e6c485f4f226bf9ef49e5e629b1a03f16e4b46ef9ff2a |
| nas-collection-worker.exe | 12453376 | 5f8172a63cc082a687cf102d42a12133f27533601675cd3bac3a142097af7f03 |
| nas-linux-amd64 | 21424435 | c2b8e375442ab8c946c9c43a697e4c4990821d50ee4f34ef54e9bb470aee483d |
| nas-collection-worker-linux-amd64 | 12118413 | b78451ea184e7d4bd56e9c205f6014f7f3704cdc37a1d19ed1daacf04fdf2512 |
| nas-linux-arm64 | 20077398 | ff84f5f674a05a5d8f7c466d4fe7b5d54b1859984313c3396fe934e46ff7bb83 |
| nas-collection-worker-linux-arm64 | 11226297 | a32d4a4d291b21be0744282cbfec9030162bf793e8435e80cf12de96fced5fd3 |

实际工具链 Go 1.27.4 windows/amd64。Go vet、清理测试源码后的 `go test ./...`、gofmt、Python AST、Vue 严格类型检查和 Vite 构建通过。清源 Go test 的各包无保留测试文件，按编译检查记账，不当作新增行为覆盖。Linux 四个产物仅交叉编译，未执行。

最终工程门禁：`.runtime/verify/uav-final-checks-20261125-3/result.json` 为 passed；其关联 `.runtime/verify/uav-final-checks-20261125-2/` 的 8 条实际成功命令日志及 6 个产物。源码和产物摘要复核无漂移。报告保留每条命令与临时环境覆盖；不修改用户级变量或持久 Go 设置。

最终主线回读 `.runtime/verify/uav-mainline-final-audit-20261125-1/result.json` 为 passed（UTC `2026-11-25T03:23:41.900971+00:00`）：各套结果计数吻合、baseline 使用同批主服务/worker/前端摘要、独立复核源码与最终源码一致、6 个产物无漂移、临时测试和零长度修改文件均为空、任务为 review。Trellis 上下文校验同时通过。

## 同批 Windows 产物的集成验证

| 验证 | 实际结果 | 证据根（均在 .runtime/verify/ 下） |
| --- | --- | --- |
| 四站协议集成 | 77 项通过 | uav-final-protocol-20261125-1/result.json |
| Edge 浏览器 | 42 项通过，无页面未处理异常 | uav-final-browser-20261125-1/browser-111932/result.json |
| 既有 JMComic | 66 项通过 | uav-final-jmcomic-20261125-1/result.json |
| 统一 baseline | core 44 / collection 27 / release 83，通过 | uav-final-baseline-20261125-1/run-manifest.json |

协议集成启动真实应用、worker 与 FFmpeg；站点桥接与下载器使用明确标记的离线协议替身。覆盖黄果规范单集搜索/预览/任务/入库、真实 MP4 Range、长中文标题、鉴权/同源/输入、稳定请求与来源修订、损坏拒绝、取消/重试、主服务和 worker 重启、插件禁用/缺依赖/执行端离线及原本地来源。该脚本确实登记 relay，但替身不经 relay 传输；真实 relay 证据另记在独立质量复核。

浏览器使用同批真实应用端点，覆盖 375/768/1024/1440、明暗主题、四站选择、120 个补充平面字符输入、键盘/焦点、过时搜索隔离和明确标记的受控 UI 过期/未知提交响应。黄果单集实际入库后通过原视频播放器打开生成 MP4。主线查看最终移动深色、黄果和播放器截图。浏览器与其自有应用/worker 服务均已结束（serve 会话退出 0）；其它脚本回收其自有进程。baseline 的核心服务由 owned Job 回收（退出 1），不能称为正常停服证据。

最终 baseline 未启用 `--large-file`，不把早期 5 GiB 稀疏 HTTP 偏移样例扩称媒体解码或性能验证；其它未选的 baseline 组均为 not_run。发布组是脚本自检，不是实际发行打包。

## 独立真实依赖与网络边界复核

详细责任和证据见 `quality-check.md`、`uav-worker-verification.md`。独立证据根为 `.runtime/verify/uav-independent-quality-20261125/`。

- 初期 9 个跨层契约/处理器测试函数通过，含旧 JSON/摘要、选择字段隔离和长中文标题。最终另有 13 个函数通过，两阶段分开执行。
- 最终矩阵 22 个不同的实际锁定 yt-dlp 下载场景：5 个成功、17 个明确拒绝，均通过预期结果。生产 `webArguments()`、`toolEnvironment(true)`、真实认证代理和受控 TLS 上游串联，覆盖普通/AES、密钥长度及跨源、不完整响应、精确/忽略/错误 Range 和 chunked 正常/短/长。受控上游替换拨号并信任该夹具证书，另测默认 TLS 拒绝不可信证书；生产未放宽 TLS/公网策略。
- 真实 8 秒 TS 的正常片通过；在 188 字节包边界截断后只剩约 1.96 秒，虽然仍有 49 帧且可解码，也必须被时长门禁拒绝。HLS 下界容差为 `max(0.5 秒, 预期 EXTINF 总时长的 2%)`。
- 独立重跑真实固定包：四站 search/resolve/empty 各 4 例、bad 24 例、重定向 2 例；实际原生 HLS 和 AES 成功，缺片/不支持加密失败。实际 Python 3.14.4、yt-dlp 2026.10.26，FFmpeg N-122527-g4561fc5e48-20260122。
- 四个生成的临时 Go 单元测试文件全部删除；最终测试文本备份 `uav_relay_quality_test.go.txt` 不参与编译，保留日志可复现。清源后 Go 检查通过。
- Linux CPython 3.12 amd64/arm64 各 20 个锁定 wheel 的选择和哈希下载通过；这不证明 Linux 解释器/动态库闭包或 Docker 运行。上游 GUI/AI 依赖有意未安装，未宣称全量 upstream pip check 通过。

## 失败记录与恢复

- 早期并行验证与额外 5 GiB 夹具耗尽 E: 余量。`uav-release-protocol-20261125-1/result.json` 的 passed=false、53 项前置记录和不完整 baseline 保留，不计为成功。仅删除两份确知属于本任务的 5 GiB 样例，后续使用新根且不再启用大文件夹具。
- 磁盘不足曾中断 bridge 编辑；实现者从已构建 worker 内嵌文本恢复完整文件，并核对仅保留预期空列表修复；随后真实包桥接套件与独立复核通过，最终生产源码摘要固定。修改/新增文件无遗留零长度源。
- 最终工程首轮脚本的裸 `go` 子进程无法定位；改用 `.tools/go/bin/go.exe`，首轮 `startup-failure.json` 保留。
- 第 2 轮 8 条工程命令全部成功，但附加 `core.autocrlf=false` 让 Windows CRLF 被误报为行尾空白，原门禁仍保留 passed=false。第 3 轮使用原仓库设置执行 `git diff --check` 并重新核对同一批源码与产物后通过；未修改生产源码或重复构建来掩盖检查设置问题。
- 中途用户中断使独立矩阵会话无法取回，复核者在最终日志根重跑后才放行，没有将未取回会话计为通过。

复盘已写入 `completeness-retrospective.md` 和 UAV code-spec；本仓库没有对应生成 spec 模板目录。

## 验收边界与后续

本地实现和离线验收完成，未执行真实公网站点搜索/媒体传输、Docker 镜像构建/运行、Linux Python 运行或物理 NAS 性能/存储验收。上述范围需在实际部署环境单独验证，能力探测不证明站点可达。无账号/Cookie/付费资源流程。

HLS 时长检查只拒绝明显丢尾，不证明容差内或内容语义完整性；直链 MP4 无可信预期时长，HTTP 传输和可解码只证明收到的内容，不证明原片全长。

没有自动提交、发布、部署、归档任务或修改用户实例。用户审阅后提交；提交后可按 `/finish-work` 归档和记录会话。
