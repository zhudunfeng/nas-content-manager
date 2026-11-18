# 搜种部署与使用

栖库通过 **Torznab** 连接 Prowlarr／Jackett，在下载中心搜索并比较结果，选中后复用已有 BT 文件选择和五类入库流程。搜种为可选模块，基础媒体库仍只需一个应用容器。已有索引服务可直接接入；没有时可选择额外部署 Jackett。

本功能不附带站点名单、站点账号或搜索结果。需在索引服务中添加并测试你需要的来源。是否有结果、做种数和可用性取决于该来源与网络；站点标注不构成内容完整性保证。搜种不自动订阅或自动开始载荷下载。

## 1. 复用已有 Prowlarr／Jackett

1. 在 Prowlarr／Jackett 中确认索引器测试和手动搜索成功。
2. 把 [`.env.search.example`](../.env.search.example) 合并进现有 `.env`，不要覆盖媒体和下载配置。
3. 将 [`torrent-search.sources.example.json`](../torrent-search.sources.example.json) 复制到 `.runtime/search/sources.json`，填写你实际使用的来源。必须是 JSON，不支持注释。

```json
{
  "sources": [
    {
      "id": "books",
      "name": "我的图书来源",
      "url": "http://jackett:9117/api/v2.0/indexers/实际索引器ID/results/torznab/api",
      "apiKey": "填写你自己的API-Key"
    },
    {
      "id": "movies",
      "name": "我的影音来源",
      "url": "http://prowlarr:9696/1/api",
      "apiKey": "填写你自己的API-Key"
    }
  ]
}
```

上例展示两种服务格式，不代表必须同时部署。删除未使用的来源，以实际主机名、端口和索引器 ID 替换示例。容器内 `127.0.0.1` 指容器自身；外部服务必须能由栖库容器访问。HTTP 仅用于受信任本机／内网；跨非受信任网络需配置有效证书的 HTTPS。

| 配置字段 | 中文说明 |
| --- | --- |
| `sources` | 1–8 个独立索引器；不要把 Jackett 的 `all` 聚合端点作为推荐配置 |
| `id` | 应用内唯一稳定标识，1–40 个 ASCII 字母、数字、短横线或下划线；修改会使旧结果失效 |
| `name` | 界面显示名，最长80字符；不要写入密钥或私密地址 |
| `url` | 完整 Torznab API 端点，不能含用户名、密码、查询参数或片段；从复制URL移除 `?apikey=...`，密钥填下一字段 |
| `apiKey` | 索引聚合服务 API Key，保存在本机私密文件中；不同于栖库密码和 qB WebUI 密码 |

来源文件最大64KiB，应用启动时严格校验。修改文件后重启应用使之生效。该文件不保存到 SQLite，**不包含在数据库备份**中。宿主机文件要让栖库 `1000:1000` 身份可读，建议所有者为该身份、权限 `0600`；同时保护所有父目录，避免其他用户读取。不要将真实文件放到 Git 或公开分享。

```bash
# 在仓库根目录执行；随后用本地编辑器填写sources.json，不把Key放进命令行。
install -d -m 700 .runtime/search
cp torrent-search.sources.example.json .runtime/search/sources.json
# 填写完成后，根据NAS实际身份授权读取。
sudo chown 1000:1000 .runtime/search/sources.json
sudo chmod 600 .runtime/search/sources.json
```

同时确认容器挂载和宿主目录遍历权限；`secrets` 名称本身不能替代实际文件权限。上方 `install`／`chown` 为 Linux 示例，Windows 使用文件 ACL 进行相应授权。

手动部署（已有 qB 和索引服务）：

```bash
docker compose -f compose.yaml -f compose.downloads.yaml \
  -f compose.search.yaml up -d --build
```

如果只想先搜索，可省略 `compose.downloads.yaml`；结果能浏览，但添加下载按钮提示先启用并连接下载器。下载初始化步骤见[下载文档](downloads.md)。

## 2. 可选新增 Jackett

[`compose.search.jackett.yaml`](../compose.search.jackett.yaml) 使用 LinuxServer 维护的固定版本镜像，支持 amd64／arm64；它不是 Jackett 项目官方镜像。镜像摘要已通过注册表 HTTP 元数据核验，证据见[镜像记录](../.trellis/tasks/11-18-torrent-search/research/jackett-image.json)，**未执行 Docker 构建或启动验收**。

1. 根据 `.env.search.example` 设置 `JACKETT_CONFIG_PATH`，预先创建目录，并给 `JACKETT_UID`／`JACKETT_GID` 写权限。
2. 先仅启动索引服务，避免应用在来源文件尚未填写时启动失败：

```bash
docker compose -f compose.yaml -f compose.search.jackett.yaml up -d jackett
```

默认管理地址仅绑定 NAS 回环：`http://127.0.0.1:19117`。远程 NAS 可在你的电脑建立 SSH 隧道：

```bash
# 替换SSH用户名、NAS主机名和需要调整的端口。
ssh -L 19117:127.0.0.1:19117 nas-admin@nas.local
```

3. 在本机打开该地址，先设置管理员密码，再添加、测试需要的索引器。不要把管理页面直接暴露公网。
4. 从 Jackett 复制每个索引器的 Torznab Feed URL，去掉查询字符串，将地址和 API Key 分别填入 `sources.json`。在同一 Compose 网络里，地址使用 `http://jackett:9117/...`。
5. 设置 `TORRENT_SEARCH_ENABLED=true`，填好来源文件，再叠加启动：

```bash
# 外接qB时的组合；采用仓库内可选qB时再追加compose.downloads.qbittorrent.yaml。
docker compose -f compose.yaml -f compose.downloads.yaml \
  -f compose.search.yaml -f compose.search.jackett.yaml up -d --build
```

索引服务只挂载 `/config`，不需要媒体目录、专用下载目录或应用数据库。真实视频数据仍由 qB 接收；栖库只代理最多8MiB的种子元数据。镜像自动更新关闭，升级通过更新固定镜像并重新验证完成。

## 3. 使用

1. 网页侧栏或手机“更多”打开“下载中心”，点击“搜种”。
2. 输入2–120字符关键词，按需选择来源与“全部／影视／音乐／书籍”，点击搜索或回车提交。
3. 比较标题、来源、体积、做种数和发布时间。可以本地排序和分页；缺失或非法数值显示“未知”。影视对应Torznab影片与电视类别，书籍可能包含漫画，站点是否完整支持类别需实测。
4. 选择“添加下载”，确认五类内容中的目标类型以及首次建库授权；搜索分类不会替你决定入库类型。
5. 添加后等待获取文件清单，在详情中勾选所需文件并确认。选集、暂停、限速、完成发布和阅读入口沿用原 BT 工作流。

搜索结果最多500项，每来源最多100项；截断时缩小关键词或切换来源重新搜索。部分来源失败不会丢弃已成功结果，会显示失败来源。无结果与全部来源失败分别展示；取消或关闭会终止当前请求。

本版直接传递通用搜索和标准父类别，不自动读取 caps 或协商每个站点的细分类别。来源不支持类别时，可先选“全部”搜索，并在索引服务中核对该站点的实际支持范围。

搜索结果令牌有效十分钟，最多保留最近八批；重启或被新查询逐出也会失效。结果失效后重新搜索即可。**已成功登记的添加请求**会按持久请求键恢复，即使缓存此后过期或应用重启；保留原表单重试，不必反复创建任务。

## 4. 安全与故障说明

- 浏览器只访问栖库同源 API，不接收索引器 API Key、私密 Tracker、下载URL或原始 XML。日志也不记录这些内容。
- 有效磁力直接交给原安全添加流程；HTTP种子只接受配置来源同 origin 的受控下载代理路径。外站原始链接、来源管理路径和跨源HTTP重定向被拒绝。
- 如果结果不可添加，检查索引服务是否返回自身代理地址；反向代理需让输出的 scheme／host／port／基路径与配置一致。不要关闭边界检查或改为浏览器直连私密URL。
- 代理30x返回有效磁力可支持；HTTP跳转不自动跟随。需要索引服务直接代理种子字节或返回磁力。
- 单源搜索10秒、整体12秒，全局2次搜索且每次最多3个来源并发。忙时显示稍后重试；响应过大、XML损坏或DOCTYPE返回脱敏错误。
- 下载器离线不影响搜索，但添加前需恢复下载器。搜种离线不影响原媒体库、原文件Range和已有下载。
- 无效来源文件会在启动时给出中文配置错误。确认JSON、去除URL里的apikey查询、核对文件权限后重启。

## 5. 手动验收与回滚

以下由用户在实际 NAS 上验证，本次开发没有执行 Docker 命令：

- 合并配置、拉取固定镜像、启动／重启，以及容器内到索引服务的网络。
- 宿主配置目录和secret的 UID／GID／ACL，管理UI回环访问与SSH隧道。
- 实际站点搜索、类别支持、结果体积／做种数据、私有站点代理与反向代理基路径。
- 选择一个授权的小样本，确认搜索→添加→选集→下载→入库；下载与大视频播放并发时的实机资源表现。

停用时设置 `TORRENT_SEARCH_ENABLED=false` 后重建应用，或移除 `compose.search.yaml` 覆盖。默认开关为关闭，不涉及数据库迁移；已有媒体、下载、选集与发布记录保留。不要删除 qB／下载卷或基础应用数据卷。来源文件和索引服务 `/config` 需独立备份与恢复。
