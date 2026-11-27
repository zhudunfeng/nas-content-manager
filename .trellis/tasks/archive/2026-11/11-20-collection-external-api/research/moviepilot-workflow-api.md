# MoviePilot v3.2.8 六阶段工作流补充核实

核实日期：2026-11-20。仅只读下载官方 `jxxghp/MoviePilot` 固定 `v3.2.8` 标签的 Python 源码；没有执行第三方代码或连接实际实例。下载文本保留在 `.runtime/research/moviepilot-v3.2.8/app/`。本文是基础研究 `moviepilot-autobangumi-api.md` 的补充，重点纠正整理、刮削、调度和进度的真实契约。

## 1. 整理请求没有 overwrite_mode

官方 `app/schemas/transfer.py:345` 的 `ManualTransferItem` 包含 `fileitem/fileitems`、`target_storage`、`target_path`、`media_source`、`media_id`、`type_name`、`season`、`transfer_type`、`scrape`、`preview`、`reorganize`、`skip_success` 等字段，**没有 `overwrite_mode`**。发送该未知字段不能作为不覆盖保障。

- `POST /api/v1/transfer/manual?background=false|true`，body 为 `ManualTransferItem`，要求管理权限。
- 预览使用相同端点，body `preview=true`。保持 `reorganize=false`、`from_history=false`、不传 `logid/logids`。
- 本系统固定 `transfer_type="copy"`、`scrape=false`、`preview=true/false`；仅接受已配置本地下载根内选中的有限文件及已配置本地媒体库根。
- `GET /api/v1/storage/directories?directory_type=library&storage_type=local` 返回 `Response`，`data` 数组每项含 `name,storage,download_path,library_path,library_storage,transfer_type,overwrite_mode,renaming,scraping,...`。
- `app/chain/transfer/plan.py:111` 把匹配目标目录的 `overwrite_mode` 冻结到规划输入；`app/modules/filemanager/module.py:607–614` 优先读取匹配目标目录的策略。
- 不匹配配置而使用显式目标路径时，`module.py:615–620` 才固定 `overwrite_mode="never"`。本项目不应为利用该分支而允许任意未配置路径。
- `app/modules/filemanager/transhandler.py:773` 的 `never` 决策在同名目标存在时拒绝覆盖，并返回 `overwrite_skipped=true`。

**实现约束：**目标根必须从固定目录接口读取，且 `overwrite_mode` 精确等于 `never` 才允许本系统整理；预览和提交前均重读并校验相同目录配置。没有该策略时返回可操作中文说明，要求用户在 MoviePilot 为目标目录设置不覆盖。不能声称单次 API 参数能够强制覆盖实例策略；外部管理员并发修改仍是独立实例边界。

官方来源：

- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/schemas/transfer.py#L345
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/transfer.py#L907
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/storage.py#L58
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/chain/transfer/plan.py#L111
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/modules/filemanager/module.py#L607
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/modules/filemanager/transhandler.py#L773

## 2. 整理预览、后台提交和队列

`POST /transfer/manual` 声明 `Response[ManualTransferSubmissionData | ManualTransferResultData]`；HTTP200 必须继续判断 `success`，不能把 `data` 当数组。

预览 `data`：

```json
{"summary":{"total":1,"success":1,"failed":0},"items":[{"source":"私密路径","source_storage":"local","source_item":{},"target":"私密路径","target_dir":"私密路径","success":true,"message":"...","failure_stage":null,"recovery_action":null,"overwrite_skipped":false}],"message":"..."}
```

执行 `data`：

```json
{"items":[{"state":"accepted","source":"私密路径","target":null,"target_dir":"私密路径","success":true,"message":"...","failure_stage":null,"recovery_action":null,"overwrite_skipped":false}],"message":"..."}
```

`state` 枚举 `accepted|completed|failed|retry_wait|skipped|manual_review`；`accepted` 只代表受理，只有 `completed` 是外部整理完成。本系统扫描入库仍是后续独立链路。部分失败也保留逐文件结果，不能忽略失败项。

该提交回执 **没有稳定 job_id**。不能猜测任务编号或给未知提交伪造成功对账。

- `GET /api/v1/transfer/queue`：旧版分组数组，无可靠任务编号；每组 `media,season,tasks`，task `fileitem,meta,state,downloader,download_hash`。
- 推荐有界读取 `GET /api/v1/transfer/queue/page?page=1&count=100`：`TransferQueuePageData {items,total,page,count}`，每组 `media,season,tasks`，task 只含 `fileitem {path,storage,type,name,size},state`。固定只读，继续按私密路径内部关联，不向浏览器透出路径。
- 没有从 `/transfer/manual` 返回的 job receipt 对应进度 JSON；通用 `/system/progress/{process_type}` 是持续 SSE，不适用现有一次性 JSON helper。

官方来源：

- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/schemas/transfer.py#L548
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/schemas/transfer.py#L576
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/schemas/transfer.py#L586
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/transfer.py#L191
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/transfer.py#L661
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/transfer.py#L671

## 3. 本地刮削同步执行且覆盖图片/NFO

`POST /api/v1/media/scrape/local`：body 为单个 `FileItem`，query 使用 `media_source=themoviedb&media_id=<id>&type_name=电影|电视剧`；不把媒体身份塞进随意 body。

`app/api/endpoints/media.py:658` route 委托 `_scrape_impl`，其同步调用 `ScrapingChain().scrape_metadata(...,overwrite=True)`，返回 `Response[None]`：`{"success":true|false,"message":"...","data":null}`，**没有 job_id**。路由说明明确会覆盖目录图片和 NFO。本系统必须确认覆盖元数据，并将范围限制为已配置本地媒体库根中的单一选中受支持媒体文件，不能开放全盘路径或目录树。预览只读校验身份与路径，不调用该写接口；执行重查后一次提交。

成功同步回包可记录外部刮削完成；超时、断开等没有回执时保持 uncertain，不盲目重试。日志和 DTO 不回显上游 message、路径或元数据原始对象。

官方来源：

- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/media.py#L645
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/media.py#L658

## 4. 调度列表、执行和进度的精确路径

**不存在需要本项目调用的 `/system/scheduler` JSON 约定。** 固定后台任务的已核实接口：

| 方法与路径 | 鉴权与响应 |
| --- | --- |
| `GET /api/v1/dashboard/schedule` | 管理员身份；`List[ScheduleInfo]`，经 ResponseAPIRouter 输出标准信封 |
| `GET /api/v1/dashboard/schedule/{job_id}/progress` | 管理员身份；`Response[ScheduleProgress]` |
| `GET /api/v1/system/runscheduler?jobid=mediaserver_sync` | 管理员身份；固定任务，`Response[None]`，只确认请求已提交，无独立执行ID |
| `GET /api/v1/dashboard/schedule2` | API_TOKEN查询参数专用变体；本实现不需要把secret放query |
| `GET /api/v1/dashboard/schedule2/{job_id}/progress` | API_TOKEN专用变体 |
| `GET /api/v1/system/runscheduler2?jobid=...` | API_TOKEN专用变体 |

`ScheduleInfo` 字段：`id,name,provider,execution_group,status,next_run,progress,progress_text,progress_enable,progress_detail`。状态原文不可直接公开。

`ScheduleProgress` 字段：`id,name,provider,execution_group,enable,value,text,status,success,started_at,finished_at,error,data`；`status` 为 `waiting|running|success|failed`，`enable` 表示正在执行。只映射白名单状态/有限数值与时间，不回显 error/text/data。

`GET /api/v1/system/progress/{process_type}` 是 `text/event-stream`，持续每0.5秒 `data: <json>\n\n`，由连接关闭/系统退出终止；不是一次性 `Response` JSON，不可喂给 `callMP`。

`app/scheduler/catalog.py:236` 登记固定 `mediaserver_sync`。该任务执行同步媒体服务器信息到 MoviePilot 的索引，**不等同触发 Emby/Plex 文件系统重新扫描**。界面应明确写“同步 MoviePilot 媒体库索引”。只选择列表中确实存在的固定任务；不能接受任意 jobid 或自动执行其他任务。执行后有界读取精确 progress 作为观察，不能以 scheduler 请求成功推断刷新已经完成；没有可关联回执时未知提交仍不确定。

官方来源：

- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/dashboard.py#L152
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/dashboard.py#L163
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/system.py#L1168
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/api/endpoints/system.py#L697
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/schemas/dashboard.py#L75
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/schemas/dashboard.py#L115
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/scheduler/catalog.py#L236

## 5. 测试边界

补充下载回执与身份：`POST /download/` 的 `data.download_id` 来自 `DownloadChain.download_single` 的原 `download_hash`，不是包含下载器名称的复合编号。内置 qB 返回 torrent_hash，Transmission 返回 hashString；schema 仅约束 Optional[str]，不能声称所有插件都保证40/64位十六进制。保守固定内置hash格式时，未知格式应保持未知，不能拼造编号。

单目标启停可使用 `PATCH /download/{hash}`，body 精确为 `{"action":"start"|"stop","downloader":"原下载器名称"}`。返回 `data={hash,downloader,results:[{operation:"start"|"stop",success,message}]}`，`operation` 直接等于入参action，不能按pause/resume解析。参考 `app/api/endpoints/download.py:458`、`app/application/download/tasks.py:204–218`、`app/schemas/download.py:57–101`。

元数据搜索结果由 `app/domain/context.py:2354` 的 `MediaInfo.to_dict()` 序列化，明确输出 `media_source` 和转字符串的 `media_id`（允许null）。因此筛选 `media_source=themoviedb` 且完整数字媒体身份是必要的，不能由辅助tmdb_id猜造缺失的主身份。`POST /download/` 接收 `media_in` 与 `torrent_in` 两个完整私密上下文，原站点cookie与enclosure只在服务端快照流转，不向浏览器或主库输出。

补充官方来源：

- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/chain/download/submission.py#L645
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/chain/download/submission.py#L870
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/modules/qbittorrent/__init__.py#L242
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/modules/transmission/__init__.py#L214
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/application/download/tasks.py#L204
- https://github.com/jxxghp/MoviePilot/blob/v3.2.8/app/domain/context.py#L2354

上述固定标签源码为协议依据。可控 HTTP fixture 必须匹配标准信封、逐文件回执和不含 job_id 的真实事实；不能为使实现通过而假造 receipt。未验证实际用户部署、插件对覆盖决策的介入、真实媒体服务器行为或传输吞吐。实例之外的并发改配置无法由本系统事务覆盖，提交前重读和保守不确定态是明确边界。
