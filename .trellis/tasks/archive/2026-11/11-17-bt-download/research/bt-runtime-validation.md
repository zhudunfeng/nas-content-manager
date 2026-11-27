# BT 接线、发布门禁与旧功能回归

日期：2026-11-17。执行环境为 Windows 本机、Go 1.27.4、生产 Vue 构建与隔离 SQLite／媒体样本；本轮未运行 Docker。

## 主代理临时行为回归

`download_gate_regression_test.go` 的四个顶层测试组由主代理执行通过，独立质量检查再次执行通过：

1. JSON 请求预算：合法 JSON 通过，正文超限与首个 JSON 后超限尾随空白返回 413，多个 JSON 对象返回 400。
2. 发布门禁：拒绝 staging、未知成品目录、未选文件和未发布任务；ready 选集及祖先目录可读取；归档／关闭下载后仍按历史清单约束；尺寸或纳秒 mtime 变化、ready 冻结、清单版本变更后拒绝读取。
3. v1→v2 增量迁移：同一数据库两次打开，保留原音乐条目、收藏、中文标签与 12 秒进度，下载表存在，`integrity_check=ok`。
4. Linux mountinfo 解析：合成同 device／root 的深层 bind 别名，能识别映射到 staging 的深层目录并拒绝启用。这是纯解析／映射回归，没有在 Linux 主机上实际执行 mount。

按 AGENTS.md 要求，此临时文件在记录证据后已清理。后端代理和独立检查代理的临测由各自清理；最终空测试包的 `go test` 只代表编译，不能替代上述已执行行为回归。可复用脚本保留在 `scripts/verify.py`、`scripts/verify_downloads.py` 与本目录的真实 qB 探针中。

## 原五类媒体回归

主代理用本轮新增下载接线的原生程序运行以下命令，19:53 的重跑以及 19:56 包含最终会话 Cookie 名称修复的 `nas-final-cookie.exe` 重跑，均为 **52 项断言全部通过，进程退出 0**：

```powershell
python -X utf8 scripts/verify.py `
  --root .runtime/verify-bt/original/verify/media `
  --base-url http://127.0.0.1:18123 --large-file `
  --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe
```

原生服务已先停止上一构建，再用新二进制与同一数据目录重启；下载配置为关闭。覆盖：

- 登录、未授权、同源保护、授权目录边界。
- 视频／图片／漫画／小说／音乐扫描、实际内容读取、UTF-8／GB18030、CBZ 与 EPUB 原文件。
- 全部已有音乐后缀／MIME、时长、内嵌及默认封面、精确 Range、收藏／标签／进度、重复扫描和原文件不改写。
- v2 一致性 SQLite 备份与完整性。
- 5,368,709,243 字节稀疏样本的 HEAD、偏移 4,294,967,313 和片尾精确字节、suffix、416、304 与鉴权。

稀疏样本只证明 64 位长度／偏移和 HTTP 语义；真实短视频回归来自原首版／音乐记录。没有测量真实 50–100 GiB 影片、目标 NAS 并发吞吐或峰值资源。

## 证据分工

- 下载适配器、命令竞态、提前冻结与认证回归：`bt-quality-check.md`。
- 下载编排集成、真实 qB peer 传输与官方镜像来源：`bt-implementation-probes.md`。
- 最终生产页面操作、会话恢复与响应式：`bt-browser-acceptance.md`。
- 构建、清理及全部待验边界的最终汇总：仓库 `docs/verification.md`。
