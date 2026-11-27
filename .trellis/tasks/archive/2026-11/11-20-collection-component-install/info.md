# 组件 Docker 部署接口与实施契约

## 固定组件与存储

`moviepilot / autobangumi / sonarr / radarr`。保留 `enginecontract.TemplateVersion`（原 qB/Jackett）；新增 `CollectionTemplateVersion = "collection-2026-11-20.1"` 和 `CollectionNoticeVersion = "collection-storage-v1"`，新组件专用模板使用新版本。

启用参数 `ENGINE_COLLECTION_ENABLED`（仅 true/false，默认false）。`Config.CollectionEnabled bool`。原 `volumeRoles()` 可保留；新增按 Config 返回启用角色的函数，enabled 时加 `collection-media`。bind模式从 HostRoot/collection-media 派生，volume模式固定 prefix-collection-media，helper挂 `/managed/collection-media`；主应用只读挂 `/collection-media` 并将 `/collection-media/library` 加入 NAS_MEDIA_ROOTS。用户授权后准备 library/moviepilot、autobangumi、sonarr、radarr 独立目录，UID/GID与下载身份一致。

四组件同时挂既有 downloads 到 `/downloads` 及 collection-media 到 `/data`；媒体从 `/data/library/<组件>` 整理。配置/缓存卷固定按 installation + component + role 派生，local无Options，labels含installation/component/role/template及私密state授权nonce。GET不写授权/建卷。绑定记录必须持久且失败停止后续副作用；未知同名卷拒绝，create失响应通过inspect精确对账。配置卷用容器初始化创建自身权限，不读取内置账号或API key。

公开端口只绑定 `127.0.0.1`：19130→MP3000、19132→AB7892、19133→Sonarr8989、19134→Radarr7878。MP内部地址用3000（官方nginx代理API），只发布此端口。固定环境设置与必要root init沿用官方推荐，禁用自更新，不挂Docker socket、不privileged/no-new-privileges，固定网络、日志限额。严格校验受管实际镜像ID、挂载、ports、network、privilege等，无任意浏览器参数。

## 共用 DTO（新增 internal/enginecontract/collection.go）

```text
CollectionStatus {
 helper:string, message:string, templateVersion:string,
 noticeVersion:string, noticeText:string,
 activeTaskId:string, components:CollectionComponent[]
}
CollectionComponent {
 id:string, name:string, version:string,
 presence:string, runtime:string, readiness:string, message:string,
 revision:int64, canDeploy:bool, canStart:bool,
 setupPort:int, serviceUrl:string, latestTaskId:string
}
CollectionDeployRequest {
 requestId:string, component:string, templateVersion:string,
 noticeVersion:string, acceptNotice:bool
}
```

readiness: not_configured / starting / configuration_required / error；helper不声称API鉴权已连接。serviceUrl仅当前归属容器实际运行且匿名HTTP可访问时返回；app连接结果沿用原source.apiState，不能因容器ready把source改ready。配置入口和内部服务地址无凭据。所有字段中文注释，结构保持严格DTO（unknown fields拒绝）。

## 路由（helper与app同名固定白名单）

```text
GET  /api/collection/components
POST /api/collection/components/deploy
POST /api/collection/components/{component}/start
GET  /api/collection/components/tasks/{id}?afterSequence=N
POST /api/collection/components/tasks/{id}/retry
```

GET状态有界探测 Docker 元数据与匿名HTTP，绝不写入；预检失败 helper unavailable；禁用时not_enabled，四组件仍有不可执行说明。lastTaskId从已有task JSON读取该组件最新非stop任务，activeTaskId仅collection task，避免原qB任务混用。

deploy使用 `CollectionDeployRequest`，模板/告知/主动确认三者都核验；一次一个组件，action=`deploy`（与旧initialize区分）；固定签名包含template+notice+component。start/retry用已有 `ActionRequest` 和revision；start action=`start`，新组件与旧引擎路由互斥。GET/retry需先加载并校验task仅属于一个新组件及原action deploy/start，不能用新接口读/重试旧引擎或stop任务。

响应沿用 `TaskResponse/Task/Step/Event/Layer`，保持持久UUID binding、单队列、层进度、受限历史、进程重启恢复。新组件任务执行分支不走qB/Jackett的私密连接发布步骤。stop-all启用时包含6组件，stopComponent验证new template分支，不拉取创建删除；StopAll预算240s，desktop调用255s，失败不宣称停止完成。

## 前端契约

新类型/函数放 `web/src/collection-components.ts`（避免与旧types大文件冲突），EngineTaskResponse等共用现有types。`CollectionComponentDeploy.vue`只嵌入来源表单，不包含新DialogShell或嵌套form。props service:string；emit connect(component:CollectionComponent) 预填当前服务baseUrl、externalUrl（仅浏览器在宿主本机时生成localhost入口）、allowPrivate=true，进入原API表单。部署来源切换时中止请求并拒绝旧响应，持久requestId仅含非敏感component/template，不持久凭据；失败不清空原UUID，读404允许清理。确认默认未勾选，接受后固定任务意图，任务结束刷新状态。控件type=button防止误保存外层form。

流程：选服务 → 连接已有 / 在此部署 → 明确部署确认 → 真实分层进度 → 首次配置说明/入口 → 连接此组件并输入凭据 → 原保存并检测。远程浏览器不打开错误的localhost，显示宿主端口+隧道说明，复制内部服务地址不等于API已连接。

## 桌面发布兼容

新增镜像清单schemaVersion3，app/helper仍必需，collectionImage可选且存在时必须有效。仅v3追加 `compose.desktop.components.yaml` 和collection-media角色；v1/v2的原挂载、环境和固定角色不变。新镜像打包器输出v3，发布验证要求相应overlay已打包；获取worker overlay先加，组件overlay最后加，媒体根由启动器受信任清单派生，不能从外部环境扩大授权。

## 实施所有权

- helper implement agent：`internal/enginecontract/collection.go`、`internal/enginehelper/**`，固定模板/状态/任务/卷/停止与隔离行为测试（运行后清除临时单测）。不改desktop/compose/web/app。
- app/UI implement agent：`internal/app/collection_components.go` 与原routes注册小改、`web/src/collection-components.ts`、`web/src/components/CollectionComponentDeploy.vue`、`CollectionSourceDialog.vue`及必要样式。按以上精确DTO，不改helper/desktop。
- root：Compose、desktop严格验证与停止预算、PRD/spec/docs、最终隔离协议/浏览器与回归验收。共享文件变更提前沟通。
