# 启用 BT 下载与自动入库

下载中心通过 qBittorrent 下载视频、图片、漫画、小说和音乐，完成验证后沿用栖库原有播放器和阅读器。基础部署默认关闭下载；已有媒体目录仍为只读。

**本次未执行 Docker。下列 Docker 命令供你在目标 NAS 手动验证。** 已实测的本机协议版本为 qBittorrent **5.2.4 / WebAPI 2.15.1**；可选官方镜像固定为 `5.2.4-1` 和多架构摘要，不使用 `latest`。Windows 协议探针与镜像清单检查不能代替 Linux 容器运行验证，详见[探针记录](../.trellis/tasks/11-17-bt-download/research/bt-implementation-probes.md)。

## 1. 准备目录与权限

下载器和栖库必须看到同一份专用下载数据。不要将已有媒体根目录当下载根，不要通过另一个媒体挂载暴露 `staging`。

```text
宿主下载根                qB /downloads（读写）    栖库 /downloads（只读）
├── staging/<任务 UUID>/                          下载、校验、未发布数据
└── complete/
    ├── video/<任务 UUID>/
    ├── image/<任务 UUID>/
    ├── comic/<任务 UUID>/
    ├── novel/<任务 UUID>/
    └── music/<任务 UUID>/
```

将 [`.env.downloads.example`](../.env.downloads.example) 中的参数合并到已有 `.env`，保留原来的 `MEDIA_PATH`、`WEB_PORT` 等值。`DOWNLOAD_PATH` 和 `QBT_CONFIG_PATH` 使用宿主**绝对路径**，以下 Linux 示例假定为 `/srv/nas/downloads` 和 `/srv/nas/qb-config`。

```bash
# 这些目录必须预先创建，栖库的只读挂载不会代为创建。
sudo install -d -m 755 -o 1000 -g 1000 \
  /srv/nas/downloads /srv/nas/downloads/staging \
  /srv/nas/downloads/complete \
  /srv/nas/downloads/complete/video /srv/nas/downloads/complete/image \
  /srv/nas/downloads/complete/comic /srv/nas/downloads/complete/novel \
  /srv/nas/downloads/complete/music
# 配置恢复目录仅供下载器读写。
sudo install -d -m 700 -o 1000 -g 1000 /srv/nas/qb-config
# 先创建空的密码文件，首次初始化阶段不启动栖库。
install -d -m 700 .runtime/downloads
(umask 077; touch .runtime/downloads/qb-password)
```

示例 qB 身份为 `QBT_UID=1000`、`QBT_GID=1000`。修改身份后同步调整下载目录与配置目录的所有权；下载文件和上级目录还需允许栖库的 **1000:1000** 身份读取、遍历。示例 `UMASK=022` 提供普通读取权限，不需要 `chmod 777`。NAS ACL、rootless Docker、NFS／SMB 权限须在实机确认；SQLite 继续放本机 `nas_data` 卷。

`staging` 和 `complete` 推荐位于同一真实文件系统。qB 执行位置移动，栖库不再复制整片；跨盘／跨挂载可能发生复制、消耗额外空间并需要较长时间。路径名称相近不能证明同盘。

## 2. 连接已有 qBittorrent

已有 qB 无需新增下载容器，只叠加公共下载配置：

1. 在 qB 启用受认证的 WebUI，保留认证、CSRF、Host 校验。地址须能由栖库容器访问；容器内的 `127.0.0.1` 指容器自身。
2. 将专用宿主下载根挂给 qB，并填写 `DOWNLOAD_QB_URL`、`DOWNLOAD_QB_USERNAME`、`DOWNLOAD_ENGINE_ROOT`。例如 qB 内部根是 `/data/bt`，栖库内部根仍是 `/downloads`，则 `DOWNLOAD_ENGINE_ROOT=/data/bt`；两者须指向同一宿主目录，不能只是同名空目录。
3. 将永久 WebUI 密码写入 `QBT_PASSWORD_FILE` 指定的独立文件，方法见下一节。不要将密码写到 `.env`、命令行参数或 Git。
4. 执行以下手动部署命令，再进入栖库“下载中心”检查连接：

```bash
docker compose -f compose.yaml -f compose.downloads.yaml up -d --build
```

栖库只管理自己创建的任务，不接管 qB 中已有任务，也不改已有实例的全局队列、带宽和做种策略。发现外部同哈希任务会提示冲突；请在 qB 端确认资源归属。

## 3. 新增可选官方下载器

### 首次初始化

阅读[上游法律告知及镜像说明](https://github.com/qbittorrent/docker-qbittorrent-nox#environment-variables)后，才在 `.env` 设置 `QBT_LEGAL_NOTICE=confirm`。保留示例中的固定 `QBT_IMAGE`。

首次只启动 qB，并临时增加宿主回环 WebUI 入口：

```bash
docker compose -f compose.yaml -f compose.downloads.yaml \
  -f compose.downloads.qbittorrent.yaml -f compose.downloads.setup.yaml \
  up -d qbittorrent
docker compose -f compose.yaml -f compose.downloads.yaml \
  -f compose.downloads.qbittorrent.yaml -f compose.downloads.setup.yaml \
  logs --tail=100 qbittorrent
```

在 NAS 本机打开 `http://127.0.0.1:18080`（默认 `QBT_SETUP_PORT`），使用 `admin` 与日志中的临时密码登录。远程 NAS 可从你的电脑建立 SSH 隧道，再打开同一回环地址：

```bash
# 将 nas-admin@nas.local 换成实际 NAS 的 SSH 用户和主机名。
ssh -L 18080:127.0.0.1:18080 nas-admin@nas.local
```

在 qB 的“工具 → 选项 → WebUI → 认证”设置永久密码并保存；用户名如有修改，同步更新 `DOWNLOAD_QB_USERNAME`。上游使用首次临时密码流程，本配置没有虚构 `QBT_PASSWORD` 初始化环境变量。不要关闭本机认证或 CSRF 来绕过登录问题。

### 保存永久密码并关闭临时入口

在仓库根目录使用 Bash 交互输入相同的永久密码。示例不在命令历史中包含密码：

```bash
read -r -s -p 'qB 永久 WebUI 密码：' qb_password
printf '\n'
(umask 077; printf '%s' "$qb_password" > .runtime/downloads/qb-password)
unset qb_password
# 栖库镜像运行身份固定为1000:1000，文件不能仅允许root读取。
sudo chown 1000:1000 .runtime/downloads/qb-password
sudo chmod 600 .runtime/downloads/qb-password
```

如使用其他 `QBT_PASSWORD_FILE` 路径，请相应替换。密码文件只包含密码，可带末尾换行；不要保存 JSON、用户名或引号。Compose secret 的文件挂载需要目标应用身份具有读取权限，不能只依赖 secret 名称。

最后省略 `compose.downloads.setup.yaml`，重新创建服务并启动栖库：

```bash
docker compose -f compose.yaml -f compose.downloads.yaml \
  -f compose.downloads.qbittorrent.yaml up -d --build --force-recreate
```

确认宿主 `18080` 不再监听；日常只暴露栖库端口与 BT peer 的 TCP／UDP 端口。`6881` 是默认 peer 端口，不能用它登录 WebUI。qB 的内部 `8080` 由 Compose 网络中的栖库访问，不需要直接发布公网。

官方镜像覆盖设置 `read_only: false`，用于支持 `PUID`／`PGID`；数据写入边界由专用下载卷和配置卷控制，qB 没有原 `/media` 或栖库数据库挂载。停止等待时间为上游建议的 `30m`，请预留退出时间以保存恢复状态。

## 4. 添加内容并入库

进入下载中心，选择“磁力链接”或“种子文件”，选择视频、图片、漫画、小说或音乐。首次可明确允许创建该类型下载媒体库；后续复用同一受管目录。

1. 提交一个 `magnet:`，或上传单个不超过 **8 MiB** 的本地 `.torrent`；不支持远程种子 URL、搜索站或 RSS。
2. 等待元数据与文件清单，选择需要的文件并确认。清单最多 20,000 项，下载开始后选集锁定，不能在途中切换。
3. 任务开始下载，可设置单任务上／下载 KiB/s 限速。`0` 表示不附加该任务限速，仍受 qB 的全局限制；它不是“不受任何带宽限制”。
4. 下载完成后等待校验、移动、发布核验和扫描；“下载完成”与“已入库”分别展示。所选文件完成且最终路径、文件类型、尺寸等核验通过后才开放读取。
5. 使用“查看入库内容”打开原播放器／阅读器；入库失败可单独重试，不重新下载。

确认选集前的磁力元数据停止行为，仅对真实探针验证过的 **5.2.4** 开放；其他版本可能只支持停止添加的 `.torrent`，以连接状态中的能力提示为准。版本探测不等于该版本的所有 BT／NAS 行为均已覆盖，升级 qB 后应重新验证。

移除任务始终**保留文件**与已发布清单，首期没有“删种并删数据”。下载后继续做种不会阻止入库；栖库遵循引擎已有分享策略。混合内容的种子每次选择一种入库类型，其余文件可以保留，但只把该类型已有阅读器支持的文件登记为可阅读内容。

文件已下载不保证浏览器支持其编码。本功能没有增加转码、边下边播或新的 TXT／EPUB／CBZ 解析能力；原有格式、安全与大小限制继续生效。

## 5. 排查与恢复

| 现象 | 检查与操作 |
| --- | --- |
| 下载器不可用／认证失败 | 确认后端可达 `DOWNLOAD_QB_URL`，永久密码文件可由1000读取，用户名与 qB 一致；连接测试不会将 qB 密码发给浏览器。 |
| 一直等待元数据 | 检查合法源是否有可达 Peer、qB 连接设置和版本能力。无可用 Peer 无法保证取得元数据或速度。 |
| 提交后暂时没有结果 | 请求可能已被引擎接受，系统按身份对账；故障退避可达60秒。先等待或重新检查状态，不重复从外部手工添加同一任务。 |
| 目标路径／隔离检查失败 | 对照两进程路径映射，确认 complete 已存在、挂载可读；普通媒体根不得包含下载根或 staging 的别名。修正配置后重启。 |
| 100%但没有入库 | 等待 checking／moving 结束；确认所选文件存在、尺寸正确、路径未被外部移动或改写，再检查中文入库提示。 |
| 入库失败 | 查看对应扫描任务与中文错误，修复后重试入库。原下载文件保留。 |
| 目标媒体库被删除 | 不会静默重建旧目标；重新登记相同内容类型与complete目录，再重试入库。 |
| 空间不足暂停 | 为下载卷释放空间或降低任务并发后继续；安全余量不是跨进程磁盘配额，系统不会自动删除内容。 |
| qB任务被外部删除或改标签／目录 | 系统不自动重下或接管。先在qB确认数据和任务归属，避免覆盖文件。 |
| 引擎离线 | 显示上次同步状态与时间；健康检查和已发布本地媒体仍可使用。 |

备份时分别保留：栖库一致性 SQLite 快照、qB 的 `/config` 配置／BT恢复数据、专用下载根、独立密码文件，以及尚未确认提交的应用暂存源。栖库的数据库备份不包含这些外部实体。复制 qB 恢复目录前应正常停止 qB 或使用其存储支持的一致快照；不要在运行中随意复制一部分恢复文件。

恢复时先还原下载数据、qB配置和路径映射，再恢复相匹配的栖库数据库与密码权限，最后启动并对账。不要只还原数据库后期待重建所有种子。数据库已增量迁移到 **v2**；回滚到只认识v1的旧程序时，必须恢复升级前匹配的v1快照，不能手工改 `user_version` 或期待关闭下载开关自动降级。

## 6. 手动部署与 NAS 验收

以下项目留给你在目标环境验证，当前没有预填硬件性能结果：

- 基础部署关闭下载时，原五类内容、收藏、进度、备份和大文件 Range 可用。
- 官方镜像可在目标 amd64／arm64 NAS 拉取并启动，UID／GID、ACL、挂载、secret读取正确。
- 首次临时密码能替换成永久密码，撤销 setup 文件后宿主 WebUI 端口关闭。
- qB、栖库及NAS重启后正确恢复任务且不重复添加；保留文件移除和入库重试符合预期。
- 在目标磁盘上确认 staging／complete 是否同一文件系统，记录移动耗时；分别测试空间不足、挂载失联和恢复。
- 使用有授权且可解码的真实50–100 GiB视频，记录文件系统、NAS CPU／内存、网络、qB版本、样本、持续下载速度、恢复校验、同时播放起播／拖动、峰值内存和磁盘负载。
- 下载／做种与播放争用磁盘时，先降低独立实例并发和单任务限速；不把稀疏文件Range或本机小样本结果当作NAS性能承诺。

可重复的非Docker验证脚本为 [`scripts/verify_downloads.py`](../scripts/verify_downloads.py)，真实qB本机探针、镜像来源和未覆盖项见[实施探针记录](../.trellis/tasks/11-17-bt-download/research/bt-implementation-probes.md)。完整API见[接口文档](api.md)。
