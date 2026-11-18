# 搜种最终运行时验收

日期：2026-11-18，Asia/Shanghai。最终 Go 源码包含独立复核修复，测试使用 `.runtime/verify/search-nas.exe`；所有数据在忽略的 `.runtime/verify/`，不触碰真实媒体。未执行 Docker、真实索引站点或 Git 提交。

## 构建与协议集成

```powershell
.\.tools\go\bin\go.exe build -o .runtime/verify/search-nas.exe ./cmd/nas
$env:PYTHONUTF8='1'
python scripts/verify_torrent_search.py --binary .runtime/verify/search-nas.exe
$env:NAS_TORRENT_SEARCH_ENABLED='false'
python scripts/verify_downloads.py --binary .runtime/verify/search-nas.exe
```

两脚本可并行运行，使用不同的隔离根及随机回环端口；搜种脚本显式启用自己的受控来源，不读取真实来源配置。

| 检查 | 最终结果 | 本地证据 |
| --- | --- | --- |
| 搜种真实 Go／SQLite／协调器与受控 Torznab／qB | passed=true，43 项，exit 0 | `.runtime/verify/search-20261118-081551/result.json` |
| 原 BT 编排受控 qB 回归 | passed=true，36 项，exit 0 | `.runtime/verify/downloads-20261118-081551/result.json` |
| 原五媒体与大文件协议 | passed=true，52 项，exit 0 | `.runtime/verify/search-media-20261118-081809/result.json` |

原媒体调用复用 `verify.prepare`、`verify.verify`，建立新的 `search-media-*/media` 与 SQLite 数据目录，动态回环端口，随机管理员凭据，显式关闭两个可选网络模块。FFmpeg/ffprobe 使用本机 `D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin`；样本含真实音乐及 5,368,709,243 B 稀疏视频。`verification.log` 保存 52 项输出；finally 停止进程。随机密码不打印、不写公开记录。

搜种断言包括认证／同源、输入／JSON预算、部分失败／空／未知指标、64位体积、分类、去重／截断、超时／并发、XML限制、私密URL和Cookie边界、种子预算与重定向、原选集／所选字节发布入库、请求绑定重启恢复、独立停用。新增 `key-boundary` fixture 把完整密钥放在第 500 rune 后跨越标题上限，断言密钥前缀不出现在 DTO。

可复用脚本对主动断连容忍 `ConnectionAbortedError`，最终轮无超时连接噪声。默认隔离目录附随机后缀，避免同秒自动集成与 `--serve` 撞路径；显式已有目录仍拒绝。首轮并行 `--serve` 曾因原同秒路径被安全拒绝，未启动进程，之后已用唯一目录成功联调。

## 最终静态与构建检查

- 后端实现与独立 reviewer 分别通过 `go vet ./...`、`go test ./...`、Windows 构建；空测试套件仅计编译。临时行为测试9顶层／30子用例的执行与清理见 `search-backend-validation.md`。
- 最终 Windows build 通过，`CGO_ENABLED=0` 的 Linux amd64、arm64 交叉编译均 exit 0，产物在 `.runtime/verify/search-linux-*`。
- `npm --prefix web run build` 含 strict vue-tsc，143 modules，exit 0；无新增前端依赖。
- `python -m py_compile scripts/verify.py scripts/verify_downloads.py scripts/verify_torrent_search.py` 通过。
- `torrent-search.sources.example.json` 与 `compose.search.yaml`／`compose.search.jackett.yaml` 经标准 JSON／PyYAML 解析通过；不代表 Compose 合并或容器运行。
- `git diff --check` 通过；原仓库 LF／CRLF 提示不是错误。Trellis 最终 `implement.jsonl` 6 条／`check.jsonl` 4 条引用校验通过。CodeGraph 最终同步14个变更文件（新增8、修改6），387节点，exit 0。gofmt最终只读检查无输出。

## 执行边界

未运行真实 Tracker／BT传输或实际索引站点。此前 BT 任务的真实 qB 5.2.4 探针保留为历史证据，不冒称本轮重新执行。稀疏文件仅证明 HTTP byte protocol，不证明 NAS 吞吐。Docker、真实来源类别／代理、NAS权限和大影片并发由用户手动验收。
