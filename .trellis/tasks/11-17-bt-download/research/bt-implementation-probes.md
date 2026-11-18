# BT 实施探针与部署证据

日期：2026-11-17。本文仅记录实际执行的检查；Docker命令没有执行，NAS实机大视频性能没有测量。

## 1. 已取得的证据

| 证据 | 观察 | 对实现的约束 |
| --- | --- | --- |
| 官方镜像注册表清单 [`qb-5.2.4-image.json`](qb-5.2.4-image.json) | `5.2.4-1` 多架构清单含 Linux amd64、arm64及其他所列架构 | Compose固定实际查到的版本和摘要，不宣称容器已启动或这是最新版 |
| 真实Windows双qB [`qb-5.2.4-windows-probe.json`](qb-5.2.4-windows-probe.json) | v5.2.4 / WebAPI2.15.1，停止添加、元数据停止、选集真实传输、移动和保留文件移除通过 | 磁力元数据流程仅开放已验证版本；完成判据不能只有HTTP成功 |
| 真实应用＋双qB [`qb-5.2.4-app-probe.json`](qb-5.2.4-app-probe.json) | 磁力取得元数据后停止，选一篇64KiB小说真实传输并移入complete，发布、扫描、原字节读取通过，仍在做种 | 验证完整应用闭环；未选文件即使有分片数据也不能被入库 |
| 真实Go／Python认证最小复现 | qB日志确认登录成功，HTTP204、空正文，Cookie名为`QBT_SID_<WebUI端口>`且值长32；错误密码返回401 | 不能只接受200、强制正文`Ok.`或硬编码旧`SID`；仍须验证受限会话Cookie，拒绝失败正文和不成功HTTP码 |
| 假引擎与真实应用 [`downloads-integration-probe.json`](downloads-integration-probe.json) | 最终HTTP204／新Cookie流程的36项断言全部通过，含真实SQLite、鉴权、协调器、发布门禁、五类扫描与读取接口 | 提供可重用编排验收；假引擎不能证明真实BT或Docker行为 |

镜像固定多架构摘要：

```text
qbittorrentofficial/qbittorrent-nox:5.2.4-1@sha256:92bfd78d731e254ba64f62b77b0696add6447a33642ee5feda106a26b285aa7f
```

## 2. 真实qB样本、来源与复现

官方Windows5.2.4安装包来自项目SourceForge发布目录，只用已有7-Zip解包到 `.runtime/bt-probe/qb/`，没有系统安装。下载包共41,407,004字节，SHA-256为：

```text
EF6EFE581D80EFEB0A77DF281D93F93021FBDCC48E4D8F7B341DC864F53C30DD
```

该摘要与Microsoft winget发布清单一致。Authenticode检查结果为 **NotSigned**，因此不声称可执行文件经过数字签名验证。

协议探针启动两个全新profile，WebUI与BT接口均绑定127.0.0.1，关闭DHT、PEX、LSD、UPnP。随机生成2MiB双文件v1种子，不使用外部Tracker或真实媒体，仅显式连接本机种源。qB GUI启动仍尝试了版本／地理信息等公共请求；这里的隔离结论限于**样本BT载荷和Peer**，不能宣称进程没有任何外部网络活动。

```powershell
# 此程序须提前从官方发布包取得；目录必须不存在且包含verify段。
python .trellis/tasks/11-17-bt-download/research/probe_qb_windows.py `
  --binary .runtime/bt-probe/qb/qbittorrent.exe `
  --root .runtime/verify/qb-real-new-run
```

最终可复现运行目录为 `.runtime/verify/qb-real-repro-20261117b`，返回exit0。结果包括：

- `.torrent` 使用 `stopped=true` 添加后为`stoppedDL`，downloaded为0。
- 磁力使用`stopped=false, stopCondition=MetadataReceived`，取得完整清单后为`stoppedDL`；另等3秒，downloaded和completed仍为0，两个文件progress均为0。
- v1属性返回完整40位`infohash_v1`与空`infohash_v2`，不能把空值误当无效v1，也不能依赖全零占位符。
- 真实认证负向探针确认`LocalHostAuth=true`、无子网白名单绕过，正确密码为204与`QBT_SID_<端口>`，错误密码为401。最初只根据Cookie数量称其为SID属于不完整观察，已用Cookie名称和值长度纠正，不记录会话值。
- 将文件1的priority设0、文件0设1后，普通`start`即可继续；不需要额外清除stopCondition。
- 所选1MiB文件progress为1并与随机源逐字节一致。未选文件priority为0但API progress曾为0.09375，最终未选实体文件不存在。这说明未选文件仍可能涉及分片／附属数据，发布必须按冻结选集，而不能仅按目录存在性。
- `setLocation`后等待实际save_path及状态稳定，最终位置与文件字节正确；`deleteFiles=false`移除后文件仍在。

此前两次隔离协议运行通过；重构后第一次运行在selected progress0.96875、amount_left32768时停滞，实际文件也不完整，探针**正确失败**。该次记录保留在 `.runtime/verify/qb-real-repro-20261117/result.json`。随后将停止添加探针与磁力传输使用不同staging目录，并在关闭所有发现机制的前提下定期重连唯一回环Peer，严格保持progress1、amount_left0与字节核验，复跑通过。没有通过放松完成判据掩盖停滞。

### 真实应用端到端

以下脚本启动真实栖库与两个隔离的qB进程，沿应用API完成“磁力→元数据→选集→真实传输→移动→发布→扫描→读取”，没有绕过协调器直接登记入库：

```powershell
python .trellis/tasks/11-17-bt-download/research/probe_app_qb_windows.py `
  --qb-binary .runtime/bt-probe/qb/qbittorrent.exe `
  --nas-binary .runtime/bt-probe/nas-cookie-final.exe `
  --root .runtime/verify/qb-app-real-new-run
```

最终运行目录为 `.runtime/verify/qb-app-real-20261117d`，返回exit0，结果持久化到 [`qb-5.2.4-app-probe.json`](qb-5.2.4-app-probe.json)。真实WebAPI为2.15.1，登录返回204与`QBT_SID_3710`，只记录Cookie名称，不记录值。应用状态为ready，允许文件选择；元数据阶段为stoppedDL、downloadedBytes0、readyfalse。

本地样本包含两篇各65,536字节的有效UTF-8小说，只选择`selected.txt`。所选文件progress1，qB移动到`complete/novel/<任务UUID>`后，应用readytrue、importState imported、state seeding；唯一入库条目的原文件接口与所选源字节完全一致。未选`ignored.txt`的priority0、progress0.5，说明真实引擎仍可能保留相关分片数据，但它没有入库。脚本在finally关闭应用与两个qB进程，未修改真实媒体目录。

## 3. 假引擎验证与浏览器联调

```powershell
& ./.tools/go/bin/go.exe build -o .runtime/bt-probe/nas-downloads.exe ./cmd/nas
python scripts/verify_downloads.py --binary .runtime/bt-probe/nas-downloads.exe
```

脚本独立生成账号、qB密码、数据库和下载样本，动态监听回环端口；结束或失败都会关闭自己的应用与假引擎，保留日志与样本。它不下载外部资源，不运行Docker。

2026-11-17最终执行使用 `.runtime/bt-probe/nas-cookie-final.exe`，目录 `.runtime/verify/downloads-final-20261117c`，返回exit0，**36项断言全部通过**。假引擎登录使用与真实版本一致的HTTP204及`QBT_SID_<端口>`名称，并覆盖会话过期重登。结果另存 [`downloads-integration-probe.json`](downloads-integration-probe.json)；旧式SID兼容流程也曾独立通过36项。提交结果不确定用例等待了生产环境60秒故障退避，未为验收缩短实现退避。

验证包括应用认证／同源、64KiB JSON、8MiB种子、路径穿越、分页与身份去重、过时版本、选集锁定、KiB限速、暂停幂等与过时修订、异步移动门禁、五类扫描入库、目录漫画未选页过滤、收藏进度重启、SID刷新、离线媒体读取、扫描失败重试、v2／hybrid完整身份、外部冲突、提交结果不确定恢复与保留文件移除。纯v2／hybrid在这里验证的是解析与身份编排，**尚未进行真实v2／hybrid BT传输**。

图片与漫画使用真实生成PNG，音乐使用真实WAV，小说使用UTF-8文本。视频小样本只验证文件索引和发布契约，内容不是可解码影片，不证明视频解码或起播性能。

`--serve`可留一组真实应用＋假引擎供独立浏览器联调：

```powershell
python scripts/verify_downloads.py --binary .runtime/bt-probe/nas-downloads.exe `
  --root .runtime/verify/downloads-browser-new-run --serve
```

启动后只输出地址、用户名与凭据文件路径，不输出密码。`app-credentials.json`只属于隔离测试账号；`sample.torrent`和`sample-magnet.txt`为有限本机样本。创建根目录下`engine-offline`文件模拟离线，删除后恢复；创建`stop`文件关闭本次服务。

## 4. 官方来源与部署边界

- 官方镜像README：<https://github.com/qbittorrent/docker-qbittorrent-nox>，核对`PUID/PGID`需要可写容器根、30分钟退出等待、持久卷、临时密码与合法告知变量。
- 固定镜像注册表清单：<https://registry-1.docker.io/v2/qbittorrentofficial/qbittorrent-nox/manifests/5.2.4-1>，摘要和各架构子摘要另存JSON；不曾运行`docker pull`。
- 官方Windows发布目录：<https://sourceforge.net/projects/qbittorrent/files/qbittorrent-win32/qbittorrent-5.2.4/>。
- 对照摘要的winget清单：<https://github.com/microsoft/winget-pkgs/blob/master/manifests/q/qBittorrent/qBittorrent/5.2.4/qBittorrent.qBittorrent.installer.yaml>。
- 公开API及固定版本差异来源另见[引擎研究](bt-engine-options.md)。该文件是实施前研究，其“未启动引擎”说明只描述当时研究阶段，不覆盖本文后续实际探针。

YAML语法使用Python PyYAML解析；这只证明语法可解析，不等于Compose合并、官方镜像首次启动、NAS卷权限或防火墙端口通过。部署步骤、密码文件1000:1000读取权限、原媒体只读、恢复和用户手动验收见 [`docs/downloads.md`](../../../../docs/downloads.md)。

## 5. 仍需实机验收

Docker build／up／restart、官方Linux镜像临时密码初始化、amd64／arm64目标运行、NAS ACL／卷权限、真实磁盘跨挂载移动、空间不足／断电恢复、真实v2／hybrid传输，以及50–100GiB可解码影片在下载并发下的起播／拖动／吞吐和资源峰值，均不能由本机小样本或假引擎结果替代。
