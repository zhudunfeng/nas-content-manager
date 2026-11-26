# Windows 原生 BT 搜索与下载

Windows 原生桌面可以连接已经安装的 **qBittorrent** 下载器及 **Jackett／Prowlarr** 搜索服务，不需要 Docker 或 WSL。栖库继续使用 Torznab 搜种、qB WebAPI、文件选择、完成发布和媒体入库这一条链路。

首次连接需要安装并配置这些 Windows 服务；本功能不会自动下载安装或接管其进程。关闭栖库会结束栖库自己的请求和协调器，用户启动的 qB／索引服务继续运行。Docker 完整模式的自动初始化仍见 [引擎管理](engines.md)。

## 1. 准备引擎与目录

1. 在 Windows 安装 qBittorrent，在“选项 → Web UI”启用远程控制，监听 `127.0.0.1`，例如端口 `8080`，设置自己的用户名和固定密码。保留鉴权、Host 校验和 CSRF 保护。这里填写的是 qB WebUI 密码，不是栖库管理员密码。
2. 安装 Windows 版 Jackett 或 Prowlarr，保护管理页，添加并测试自己有权使用的索引器。单独复制每个索引器的 Torznab 地址及 API Key；最多接入 8 个来源。
3. 创建专用下载根，例如 `E:\QikuDownloads`，以及其中的 `staging`、`complete` 目录。此目录必须与栖库 `data`、普通媒体根隔离，不能把整个 `E:\` 同时授权为普通媒体根。仅 `complete` 子目录作为受门禁保护的媒体根。
4. 从托盘退出栖库再修改下方配置。默认原生配置目录是 `栖库.exe` 同级的 `qiku-data\profiles\native`；使用 `--profile` 时改用对应目录。

同一 Windows 上运行 qB 时，应用和引擎的下载根应填写同一绝对目录。Windows 的中文、空格和盘符路径可用，JSON 中推荐使用 `/`，例如 `E:/我的下载/BT`，或者使用转义后的 `E:\\我的下载\\BT`。不支持 UNC／设备路径作为引擎下载根。远程或容器 qB 必须明确设置 `engineRoot`，并保证 `root` 确实映射到同一批文件，不能把不相干的两个目录当作映射。

## 2. 保存私密文件

在 PowerShell 创建私密目录并写入密码；密码由交互框输入，不放在命令行、Git 或普通日志。示例中的 profile 和下载根按实际位置修改。

```powershell
# 栖库.exe 同级的原生桌面配置目录；自定义 --profile 时同步修改。
$programDir = Split-Path -Parent (Resolve-Path '.\栖库.exe').Path
$profileDir = Join-Path $programDir 'qiku-data\profiles\native'
# 专用于 BT 载荷的绝对根，不能位于应用数据或普通媒体授权根内。
$downloadRoot = 'E:\QikuDownloads'
$privateDir = Join-Path $profileDir 'bt'
New-Item -ItemType Directory -Force -Path $privateDir | Out-Null
# 凭据目录只授权当前用户，后续创建的私密文件继承该 ACL。
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
& icacls.exe $privateDir /inheritance:r /grant:r "*$($sid):(OI)(CI)F" | Out-Null
if ($LASTEXITCODE -ne 0) { throw '无法保护 BT 私密目录，停止写入凭据' }
New-Item -ItemType Directory -Force -Path (Join-Path $downloadRoot 'staging'), (Join-Path $downloadRoot 'complete') | Out-Null
# 使用 UTF-8 无 BOM，保留密码本身，不执行 Trim。
$utf8 = [System.Text.UTF8Encoding]::new($false)
$secret = Read-Host '输入 qB WebUI 的固定密码' -AsSecureString
$credential = [System.Net.NetworkCredential]::new('', $secret)
try {
    [System.IO.File]::WriteAllText((Join-Path $privateDir 'qb-password'), $credential.Password, $utf8)
} finally {
    $credential = $null
    $secret.Dispose()
}
```

在 `$privateDir` 新建 `sources.json`，填写自己实际的端口、索引器 ID 和密钥。这里的值为示例，不附带可用站点。不要将带 `?apikey=...` 的完整复制地址直接放入 `url`。

```json
{
  "sources": [
    {
      "id": "my-indexer",
      "name": "我的来源",
      "url": "http://127.0.0.1:9117/api/v2.0/indexers/实际索引器ID/results/torznab/api",
      "apiKey": "替换为自己的API-Key"
    }
  ]
}
```

Prowlarr 的端点形如 `http://127.0.0.1:9696/1/api`，以实际索引器编号为准。字段规则与故障排查见 [搜种说明](torrent-search.md)。UTF-8 的来源文件允许文件头 BOM；UTF-16 文件不受支持。

## 3. 配置桌面双击启动

将发行包中的 `bt.windows.example.json` 复制为原生 profile 下的 `bt.json`，编辑下载根、WebUI 地址及用户名。文件名必须是 `bt.json`；相对私密文件路径以 profile 为基准，不受程序解压目录或启动工作目录影响。

```json
{
  "schemaVersion": 1,
  "downloads": {
    "mode": "external",
    "root": "E:/QikuDownloads",
    "engineRoot": "E:/QikuDownloads",
    "url": "http://127.0.0.1:8080",
    "username": "admin",
    "passwordFile": "bt/qb-password"
  },
  "search": {
    "mode": "external",
    "sourcesFile": "bt/sources.json"
  }
}
```

| 参数 | 中文说明 |
| --- | --- |
| `schemaVersion` | BT 配置格式版本，必须为 `1`，与桌面及数据库版本独立 |
| `downloads` | 下载配置；省略时保留原有环境配置行为 |
| `downloads.mode` | `external` 连接已有 qB；`disabled` 停止下载协调，保留历史门禁与目录 |
| `downloads.root` | 应用可读的绝对下载根；声明后只自动授权其 `complete` 子目录 |
| `downloads.engineRoot` | qB 看到的同一根；省略时使用最终生效的 `root` |
| `downloads.url` | qB WebUI 基地址，禁止内嵌凭据、查询或片段 |
| `downloads.username` | qB WebUI 登录名 |
| `downloads.passwordFile` | qB 密码文件，绝对路径或 profile 内本地相对路径 |
| `search` | 搜种配置；可单独启用，不要求已连接下载器 |
| `search.mode` | `external` 连接来源；`disabled` 关闭搜种 |
| `search.sourcesFile` | 现有 Torznab 来源 JSON；密钥只在服务器读取 |

双击 `栖库.exe`，在“下载中心 → 引擎管理”查看连接状态，再“搜种 → 添加下载 → 选择文件 → 确认”。文件完成后进入原受管发布和入库流程，`staging` 内容不能被媒体接口读取。

只搜索时省略 `downloads`，或将其 `mode` 改为 `disabled`；只下载时省略 `search`。关闭下载但保留历史文件访问时，保留原 `root`、`engineRoot`，只改变 `mode`。配置修改后从托盘重启应用。停止或删除外接引擎不删除原载荷，也不由栖库自动重启它们。

`NAS_DOWNLOAD_*`、`NAS_TORRENT_SEARCH_*` 中非空的明确环境参数优先于此文件；旧 `*_ENABLED` 布尔开关也保留优先级。不要同时设置互相矛盾的环境和文件配置。没有 `bt.json` 时仍完全使用现有配置。原有 `nas.exe` CLI 按环境参数运行，不读取桌面 profile 的 BT 文件；相关参数见 [下载说明](downloads.md) 与 [搜种说明](torrent-search.md)。Docker 模式不读取此文件。

## 故障与验证边界

- 启动配置错误会保留原文件并给出中文说明；`bt.json` 必须是 64 KiB 以内普通 UTF-8 JSON，拒绝重复键、未知字段、显式 `null`、非法 UTF-8、多余对象及不支持的版本。
- 连接失败先检查 Windows 引擎是否运行、回环地址／端口、WebUI 固定密码、密码文件及来源 Key。不要通过关闭鉴权或 CSRF 保护解决连接问题。
- 相同 Windows 盘符路径的反斜杠、尾随分隔符和大小写变化不会再误判为任务目录改变；POSIX 路径仍严格区分大小写。任务标签、内容身份、逐文件清单和完成发布校验继续生效。
- 下载根与普通媒体根不隔离时会拒绝启动。修正授权范围，不要把下载根加入普通媒体根以绕过门禁。
- 自动回归使用隔离 Windows 桌面进程、本机受控 Torznab／qB 协议样本和小文件；这些证据不代表已完成公共站点搜索、真实 Peer 传输、干净 Windows 或目标 NAS 验收。实际执行结果见 [验证记录](verification.md)。
