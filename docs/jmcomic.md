# JMComic 漫画搜索与下载

JMComic 作为可选的固定资源收集插件运行。主应用负责来源、预览、任务与漫画库，独立获取执行端使用锁定的 `JMComic-Crawler-Python` 下载普通图片。无需新增数据库或 qBittorrent，数据库继续使用 schema 6。

## 在界面使用

1. 打开“资源收集”，点击“漫画搜索下载”或概览中的“JMComic 漫画”。首次使用会进入来源配置，名称默认“JMComic 漫画”。
2. 保存启用的来源后，检查执行端实际能力。工具尚未安装、插件未启用或执行端离线时会显示原因。
3. 输入关键词搜索，可选择最新、浏览量、图片数或喜欢数排序，以及全部、今日、本周或本月时间范围；使用上一页、下一页查看当前页结果。变更筛选会清除旧结果，需要重新搜索。选择结果会先读取漫画预览；也可直接填写正整数漫画编号。
4. 核对标题和章节目录，选择全本或具体章节。更改选章后需要重新预览，确认选中页数及目标漫画库，再点击“确认下载并入库”。全本超过文件数预算时仍可查看目录并减少章节，超预算预览不能提交。搜索和预览不会下载漫画图片。
5. 批量模式最多输入 10 个不重复的漫画编号，每本分别预览并创建独立任务。当前批量仅支持全本；需要选章时使用单本模式。逐项查看预览或提交错误，已经受理的项目不会重复提交。
6. 在原任务详情查看获取、校验发布和入库状态。获取阶段可显示已完成页数、总页数、已校验图片字节和复用页数；字节包含复用页面，不代表网络流量，也不估算速度或剩余时间。成功后使用实际媒体条目进入现有漫画阅读器；多章漫画以有序章节目录入库。

网络断开导致提交结果未知时，在原窗口使用“重试原请求”，系统保留同一请求编号与目标。已受理的同一请求会返回原任务。关闭窗口后应先在任务列表核对已有任务，避免重新提交同一本漫画。获取失败与入库失败分别重试；入库失败无需重新下载。取消通过获取执行端停止其自有进程。

获取失败后显式重试时，执行端可复用本任务已经完成且通过校验的页面。复用前会核对原请求、当前漫画与章节信息、页面身份、图片格式、大小和 SHA256；不可信或变化的缓存不能据此发布。恢复单位是完整图片页，不提供 HTTP 字节断点续传。进度达到 100% 后仍须完成原有完整性校验、发布与入库。

## Docker / NAS

在仓库根使用已有资源收集覆盖文件：

```sh
docker compose -f compose.yaml -f compose.collection.yaml up -d --build
```

新版 `Dockerfile.collection-worker` 安装单独的 Python 虚拟环境与完整哈希锁定依赖，镜像默认启用漫画适配器。主应用的共享目录和认证仍沿用[资源收集部署](collection.md)。已有部署需要重新构建 worker，旧镜像不会因为前端出现新入口而自动具备该能力。首次构建需要访问依赖镜像源。

高级搜索、选章、进度和页面复用需要一起更新主应用网页与 worker；真实 Python 桥接嵌入在 worker 中。锁定的 Python 依赖版本仍是 `2.7.12`，无需为本次增量升级上游版本。仅看到旧 worker 的插件可用状态，不能据此判断它已经支持新增协议。

插件属于可选执行端功能；仅启动主应用时，已有媒体管理和本地只读登记仍可使用。Windows Docker 完整模式需要打包包含新版 worker 的固定镜像清单，原生开发包不会自动下载或安装 Python 依赖。

## 原生执行端

Windows 桌面完整漫画包可使用随包私有 Python 和托管 worker，无需手工安装全局 Python；启用、停用、检查与升级步骤见 [Windows 原生漫画](jmcomic-windows.md)。以下手工方式继续用于开发者、管理员和外接执行端。

先按照[原生执行端说明](collection.md#原生windows对接)配置专用数据根、令牌与主应用只读映射，再为 worker 配置以下参数：

| 配置参数 | 默认值与中文解释 |
| --- | --- |
| `NAS_COLLECTION_WORKER_JMCOMIC_ENABLED` | `false`；原生执行端显式启用漫画适配器，严格使用 `true` 或 `false`；专用 Docker 镜像默认启用 |
| `NAS_COLLECTION_WORKER_JMCOMIC_PYTHON` | Linux 为 `python3`，Windows 为 `python`；管理员指定的可信 Python 解释器名称或绝对路径，应指向已安装锁定依赖的虚拟环境 |
| `NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST` | 空；管理员或桌面提供的私有运行时清单绝对路径，核验完整性后使用包内解释器；托管模式不隐式回退到系统 Python |

解释器必须能导入锁定版本 `jmcomic==2.7.12` 及其基础依赖。依赖安装在执行端部署阶段完成，不会在用户搜索或下载时自动安装。建议使用 Python 3.11 或更新的受支持版本；锁定依赖的实际可安装性仍取决于平台 wheel。

以 Windows 仓库开发环境为例，在项目内建立虚拟环境：

```powershell
# 在仓库内准备独立解释器，避免修改全局 Python 环境。
python -B scripts/project_env.py -- python -m venv .runtime/jmcomic-venv
# 按完整锁文件校验所有依赖，不在请求时安装。
python -B scripts/project_env.py -- .runtime/jmcomic-venv/Scripts/python.exe -m pip install --only-binary=:all: --no-deps --require-hashes -r deploy/jmcomic-requirements.txt
# 确认依赖闭包完整。
python -B scripts/project_env.py -- .runtime/jmcomic-venv/Scripts/python.exe -m pip check
# 仅当前终端启用漫画插件，保留原有 worker 的目录和认证配置。
$env:NAS_COLLECTION_WORKER_JMCOMIC_ENABLED = 'true'
# 绑定上述可信解释器；Linux 使用虚拟环境的 bin/python。
$env:NAS_COLLECTION_WORKER_JMCOMIC_PYTHON = (Resolve-Path .runtime/jmcomic-venv/Scripts/python.exe).Path
```

以上只配置漫画依赖，仍需独立启动 `nas-collection-worker` 并配置主应用的 `NAS_COLLECTION_WORKER_URL`、`NAS_COLLECTION_WORKER_TOKEN_FILE`、`NAS_COLLECTION_READ_ROOT` 与必要的私网 HTTP 许可。两端必须读到同一获取数据实体；只把 Python 装在主应用所在机器上，不能让另一台 worker 获得插件能力。

## 执行边界

- 网页只提交关键词、页码、固定排序／时间枚举、漫画编号及章节编号。站点、代理、输出目录、Python 代码、YAML、Cookie 和账号均不可由请求指定。
- 搜索、元数据和图片请求使用执行端现有的受控公网代理；内网、回环、保留地址及不安全目标继续拒绝。
- 插件探测只说明依赖和适配器可运行；实际站点的连通性、区域限制或接口变化会在搜索与下载时单独报错，不将失败显示为空结果。
- 下载只写任务所属暂存目录，受文件数、总字节数、保留空间与超时预算约束。上游报告的部分成功不能发布为完整漫画。
- 完成后仍需通过执行端实际文件清单和主应用发布校验，再扫描漫画库。暂存文件与未发布产物不可从阅读接口访问。
- 停用来源阻止新搜索与下载；历史任务和已发布媒体继续保留。停用 worker 插件不删除任何文件。

## 验证与上游

`scripts/verify_jmcomic.py` 使用真实应用、真实 worker 和受控协议 fixture，生成普通图片验证任务、发布、入库与阅读；`scripts/verify_jmcomic_advanced.py` 扩展筛选、选章及进度协议检查。`scripts/verify_jmcomic_bridge.py` 使用实际安装的固定依赖、真实下载器及图片解码，以离线传输样例独立验证桥接和页面复用；浏览器脚本检查隔离应用的界面与阅读流程。此类证据不代表已验证实际站点、真实账号、Docker 容器或目标 NAS 的运行结果。具体已执行项目见[验证记录](verification.md)。

当前适配器未开放账号登录、Cookie、收藏夹、PDF／ZIP 导出或任意上游插件配置，也不展示固定 API 实现不会生效的分类筛选。使用的是锁定版本的明确能力子集，不应视为已封装全部上游功能。

上游项目：[hect0x7/JMComic-Crawler-Python](https://github.com/hect0x7/JMComic-Crawler-Python)。本适配器使用其 MIT 许可基础库，分发时保留[第三方许可声明](third-party-notices.md)。
