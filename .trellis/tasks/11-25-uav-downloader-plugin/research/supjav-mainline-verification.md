# SupJav 主线增量验证 — 2026-11-25

本次基于已提交的四站集成 `e3946d9` 追加 SupJav。原四站历史报告保留，本报告只将实际重跑结果记为本次证据。

## 最终行为

- 前后端共享第五站 `supjav`，使用无前导零的 1–12 位正整数编号。搜索、分页、元数据预览、稳定 UUID 提交、持久任务、取消/重试、发布、扫描和原视频播放器沿用既有链路。
- 固定解析 EVS/FST/VAS/LUC/VOE/TV/ST 七种播放线路。私有线路序号只在 worker 与桥接间使用，未新增公开选择器、数据库字段或配置项。
- 已解析线路的传输或内容校验失败可向后换源；每次先撤销中继作用域，检查预算，只清理已知自有普通文件，再核验磁盘余量。取消、存储或预算错误、异常暂存文件、进程未完整退出和身份改变均终止。
- SupJav HLS 只支持普通 TS，拒绝 fMP4/init-map 和 BYTERANGE。真实原生下载器逐片解密后，Go 以固定缓冲原地清理已知 PNG 前缀，并验证完整 188 字节包及适配字段长度，再执行原离线 FFmpeg 解码、时长和独立发布门禁。
- UAV 3.8.4、固定提交和源码归档摘要不变；不升级依赖，不改变 schema 6，不修改用户实例配置。

## 证据分层

- 固定包解析证据使用真正的上游解析器与受控传输；不访问真实视频站。
- 独立媒体证据使用真正的生产 `fetchUAVMedia`、认证中继、固定 yt-dlp 和 FFmpeg，并以本地受控 TLS 内容覆盖普通、加密及损坏媒体。
- 应用协议与浏览器证据使用真正的主服务、worker、FFmpeg、持久任务、发布和播放器；桥接/下载器采用明确的离线协议样例，不能视作公网传输或真实解析器证据。

## 本轮检查

所有本地命令通过 `python -B scripts/project_env.py -- ...` 运行，构建、缓存、浏览器环境和证据限定在仓库 `.runtime/`。最终应用/worker 产物来自 `.runtime/bin/supjav-20261125-1/`，下列主线脚本均使用同批 Windows 二进制。

| 检查 | 实际结果 | 证据（相对 `.runtime/verify/`） |
| --- | --- | --- |
| Vue 严格类型检查、Vite 生产构建 | 通过 | `supjav-frontend-20261125-1/` |
| 清源后的全仓 Go vet、Go test 编译 | 通过；清理后无单元测试文件，行为证据另列 | `supjav-build-20261125-1/` |
| Windows、Linux amd64/arm64 主服务与 worker | 6 个产物构建通过，摘要已保存 | `supjav-build-20261125-1/result.json` |
| 固定 UAV 包离线桥接 | search/resolve/empty 各 5；bad 30、重定向 2；SupJav provider 7、fallback 10、分页 3、私有输入/ID 16 | `supjav-implementation-20261125-1/bridge.log` |
| 独立质量与真实下载器 | 18 顶层测试、68 子场景通过；其中 SupJav 22 场景（8 接受/14 拒绝）及原 relay 22 场景（5 接受/17 拒绝） | `supjav-quality-20261125-1/` |
| 五站应用协议集成 | 102 项通过 | `supjav-protocol-20261125-1/run/result.json` |
| 浏览器及原播放器 | 47 项通过 | `supjav-browser-20261125-1/run/browser-131135/result.json` |
| 核心媒体、原资源收集、发布自检 | 分别 44、27、83 项通过；没有执行大文件夹具或生成发行归档 | `supjav-baseline-20261125-1/run/run-manifest.json` |
| JMComic 回归 | 66 项通过 | `supjav-jmcomic-20261125-1/run/result.json` |

应用协议检查覆盖五站搜索与空页/错误、SupJav 规范 ID、元数据预览、稳定 UUID、首线路传输失败后切换 ST、全部候选失败时无发布、鉴权 Range、实际入库关联与重启后的幂等性；同时保留原本地收集流程。协议替身不承担真实媒体传输证明。

浏览器覆盖 375/768/1024/1440 四种宽度、明暗主题、五站选择、SupJav 分页、黄果与 SupJav 实际任务入库及原播放器；独立标记受控 UI 响应测试。已查看最终 `uav-375-dark.png`、`uav-supjav.png`、`uav-supjav-player.png`，无横向溢出，样例播放器有真实画面和时长。浏览器正常关闭，自有 serve 会话退出 0。

全部新增临时 Go 单元测试执行后已移除，准确源码文本和失败/成功日志保留在相应证据根。最终工程审计为 `.runtime/verify/supjav-final-audit-20261125-1/result.json`，核对格式、Python AST、Trellis 上下文、临时测试清理、复核源码摘要、同批产物和自有最终二进制进程回收。

## 复核修复

- SupJav 分页只接受真实 `/page/N/?s=query` 链接；通用 `?page=N` 和带 fragment 的伪下一页被拒绝。
- TS adaptation-only 包的字段长度必须为 183；同时声明 payload 时最多为 182。三个曾被接受的结构反例修复后，七个字段长度边界复测通过。
- `runTool` 的进程回收失败沿下载、封装和解码三条分支保持终止错误，避免将其错误分类成可换源失败。未故障注入真实进程回收失败，不能宣称该系统故障已动态覆盖。
- 来源表单帮助和 API 编号说明同步第五站；前端最终类型检查与生产构建记录为 `.runtime/verify/supjav-frontend-20261125-1/`。

## 真实边界

固定 yt-dlp 的 AES 填充移除不逐字节验证 PKCS#7；改变非末尾填充字节而得到相同 TS 明文的情况，后续 TS/解码校验无法识别。媒体结构/时长检查不能证明内容真实性或原片完整性。

实际固定依赖环境为 Python 3.14.4、UAV 3.8.4、yt-dlp 2026.10.26；不能据此声称 Linux/Python 3.12 运行验收。未执行真实 SupJav/其它视频站、播放器嵌入来源或公网 CDN 搜索下载；未执行 Docker、Linux 运行或物理 NAS 验收。Linux 产物仅经过交叉编译。

未自动提交、部署或归档任务。
