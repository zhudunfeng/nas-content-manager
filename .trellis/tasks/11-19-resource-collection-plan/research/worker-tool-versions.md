# 获取服务工具版本核验

- 日期：2026-11-20，Asia/Shanghai。
- 范围：仅查询官方分发元数据，未连接用户来源。
- 官方来源：https://pypi.org/pypi/yt-dlp/json 。web.run 查询未返回可读取内容，随后通过 Python urllib 只读该官方 JSON。

本次官方元数据显示 yt-dlp 版本 `2026.10.26`。wheel 文件 `yt_dlp-2026.10.26-py3-none-any.whl`，上传时间 `2026-10-26T22:25:24.877383Z`，SHA-256 为 `a05c5b5d8562f7e8a62f5334348b406af4930d56d3a65f8021d14af0e22339ae`。源码包 SHA-256 为 `abe5c6b94c3b2f0560500add1c42e0a97641752743a5b65588ac2007a6e23eb6`。

部署固定该版本与分发摘要，不在启动时自动升级。能力探测仍检查实际工具可执行性；站点提取规则与可用性不能仅凭已安装版本作保证。音频提取和视频合并需要 ffmpeg；部分站点还需要受支持的 JavaScript 运行时，缺失时应安全失败并提示该链接无法获取。首期不提供 Cookie 上传、DRM 解锁、任意命令参数或播放列表递归。

rclone 与 ffmpeg 由受信镜像的系统软件包提供，具体构建版本应随镜像记录；复制仅开放固定 copy 和受控配置，不把工具存在视为真实 remote 已验证。

## 2026-11-20 执行端实现和验证记录

- 已实现固定 `web/local/remote` 协议、短期预览绑定、持久意图和幂等重放、显式发布、映射探针、取消及有限显式重试。运行中断进入 `recovery_required`，不能通过取消再重试绕过对账。失败重试只清理已确认归属的操作暂存目录，源文件和成品不删除。
- 网页工具关闭用户配置、插件、JS运行时、远程组件和播放列表递归；URL从标准输入传递，所有网络协议配置到随机认证的本机代理。代理在实际连接时检查全部DNS答案并连接已检查IP，拒绝私网、环回、保留和元数据网段；重定向和子请求同样经过代理。
- 审查本机 yt-dlp `downloader/hls.py` 发现 `m3u8_native` 可能回退到 ffmpeg联网。因此固定格式只接受 `http/https/http_dash_segments`；HLS-only输入不支持。ffmpeg后处理输入只允许 `file,pipe,crypto,data`，不把工具命令存在当作公网隔离证明。
- Windows 临时行为用例通过：认证、路径设备名/穿越拒绝、独占锁、本地源保护、重复请求、不同意图冲突、发布前篡改拒绝、发布状态重启恢复、运行中断恢复状态、真实代理私网/保留地址拦截、映射探针创建清理。
- 使用官方 rclone `v1.76.0` 真实CLI和隔离 `alias` remote，根目录及单文件预览→复制→取得→发布通过；不是实际云盘/凭据验收。工具归档和摘要由主任务记录。
- 本机既有 yt-dlp `2025.12.08` 与 ffmpeg执行探测通过。使用真实工具和1秒合成MP4隔离HTTP fixture，视频产物9263字节、MP3产物2235字节，均通过预览、实际获取、清单核验和发布。测试中仅将代理HTTP Dial替换为fixture；生产代理规则没有放宽。
- 公网样例 `https://download.samplelib.com/mp4/sample-5s.mp4` 的实际尝试被拒绝：当前系统DNS给出 `198.18.1.164` 保留基准测试网段，代理CONNECT按策略返回失败。不能将fixture通过表述为公网站点实测；容器固定的2026.10.26版本仍需在真实Linux/NAS环境验证。
- Windows真实父进程正常退出和取消两种用例均启动持有文件及继承输出管道的后代；执行器返回后后代文件可立即改名，归属进程树回收通过。Linux进程组实现已交叉编译，未在Linux内核上执行权限/进程回收验收。
- 临时测试执行后按AGENTS要求清除；保留主任务的可复用集成脚本。所有命令经 `python -B scripts/project_env.py -- ...`，缓存和临时文件位于仓库 `.runtime/`。

### 最终主应用网页链路补充验收

2026-11-20 09:51–09:52（UTC+08:00），临时 `TestTemporaryWebAppRealTools` 经 `python -B scripts/project_env.py -- .tools/go/bin/go.exe test ./internal/collectionworker -run '^TestTemporaryWebAppRealTools$' -count=1 -v` 执行，显式启用 `NAS_COLLECTION_WEB_APP_E2E=1`，41.36秒通过。使用最终真实 `.runtime/verify/nas.exe`（SHA-256 `b44fcf6e4bb7720413ad677517d59c31aeaab8897f72f0e78a62809e1e2defc6`）、真实 `collectionworker.Service`、本机 yt-dlp `2025.12.08` 和 FFmpeg；仅测试替换代理HTTP Dial连接隔离1秒MP4 fixture，生产来源/代理策略未修改。

- 通过主应用正式API完成创建网页来源、视频/音频预览、提交任务、相同请求幂等重放、获取、显式发布和实际媒体扫描；未直接向主数据库注入成功状态。
- 视频产物 MP4 9263字节，`itemIds=[1]`；音频产物 MP3 2235字节，`itemIds=[2]`。两任务均为执行端 `published`、主应用 `imported`，分别保存7条过程事件。
- 两个真实媒体接口均返回鉴权 `206`，`Content-Range` 分别为 `bytes 0-31/9263`、`bytes 0-31/2235`，32字节响应逐字节匹配实际发布文件；无会话读取均为 `401`。
- 隔离证据位于 `.runtime/verify/web-app-e2e-44ty2igp3r/result.json`、`application.log`、`worker.log`。本次不代表公网站点、云盘、真实BT传输或Linux/NAS部署实测。
- 临时 `web_app_temporary_test.go` 通过后已清理；本次没有生产代码变更，无须重建已有二进制。
