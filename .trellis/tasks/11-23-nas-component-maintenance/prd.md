# S12 固定组件维护与受控升级

## Goal

为已有四固定组件补齐真实状态/版本、单组件维护和可验证的升级恢复流程，逐产品/平台开放高风险动作。状态：planning；相对工作量：大。

## 依赖与入口

- S12.1状态/单组件停启只需S01及现有helper；维护执行依赖S03完整配套恢复与S04事件，另需每条产品/平台真实证据，不必等待S09/S11。
- app：`internal/app/collection_components.go`及前端`CollectionComponentDeploy.vue`、`CollectionExternalPanel.vue`、相关types。
- helper：`internal/enginehelper/collection_api.go`、`collection_templates.go`、`collection_volumes.go`、`tasks.go`、`stop.go`；协议`internal/enginecontract/`。
- Desktop：`internal/desktop/docker.go`、`docker_compose.go`、`compose.desktop.yaml`及镜像清单/打包验证。
- [总路线](../11-23-nas-evolution-plan/roadmap.md)、[拟议契约](../11-23-nas-evolution-plan/info.md)、[自动化研究](../11-23-nas-evolution-plan/research/automation-implementation.md)。

## 范围与拟议契约

- 当前容器digest、API实际版本、模板目标、HTTP readiness、API已连接/配置待完成分开，未知不能拿配置值冒充探测值。
- 新动作能力协商，旧helper/旧Desktop显示unsupported；固定白名单、模板/镜像/参数、归属标签/nonce、私密helper与Desktop校验继续保留。
- 拟议单组件stop、upgrade/uninstall_keep_data维护plan/execute，绑定精确安装/容器/卷/修订/过期/确认；不接收任意镜像URL/Docker参数。
- 持久维护步骤与receipt，源/目标均固定受信；先对账未知状态，再决定下一步，不把deploy retry当升级重试。
- 卷标签包含template，须设计长期安装/角色/nonce与某次模板事实的兼容映射；不能改常量使旧卷全冲突，也不能放宽验证或重生成nonce认领旧卷。
- 升级回退要求旧镜像/模板、升级前一致数据、原身份/nonce和已验证恢复路径同时成立；无逆迁移证据不提供一键回滚。

## 小 PR 顺序

1. **S12.1 真实状态/版本与单组件启停**：复用现stop原语，单组件不设置全局quiescing；读取不启动，保留stop-all协调。
2. **S12.2 只读维护计划**：固定源/目标、架构/API/端口/卷/权限差异、官方迁移风险和配套备份核验；此片只计划，不升级。
3. **S12.3 首个产品的一条实测升级路径**：持久意图→独占维护锁→停止/快照→替换→readiness/API重新Probe→完成，逐阶段恢复；只开放有证据的平台/版本。
4. **S12.4 保数据卸载/原身份恢复**：仅停止且确认自有的精确容器移除，卷/媒体/helper身份/nonce/记录保留；超时先inspect，重装不能当新卷claim。

## 当步输入与放行门禁

实施S12.2–4前核实实际宿主/架构、Docker/Compose和卷布局、当前产品版本、官方支持升级路径/发布说明、固定digest/平台清单及数据迁移/备份恢复。此时再查官方当前资料，本计划不指定尚未核实的“最新版”。每个产品/平台/动作独立放行；未验组合只能诊断/预览。

## Acceptance Criteria

- [ ] 实际/目标/未知/不可达/待配置状态不混淆；旧helper不误启新维护动作。
- [ ] 外来容器/卷/标签/nonce冲突均失败封闭；单组件停启不影响其他组件，stop-all与维护互斥/预算准确。
- [ ] 请求冲突、并发维护、失联/重启各阶段、部分成功、备份缺失/失效、模板映射均有fixture断言。
- [ ] Desktop清单/Compose白名单/镜像包与helper协议同步，不全局放宽旧校验；维护任务不能被旧retry入口误操作。
- [ ] 每个拟开放的平台/产品/动作都有真实宿主证据；Linux NAS与Windows Docker完整模式分别验收，已验证组合可先放行，未验证组合仅诊断/预览，不要求等待所有平台才交付首条路径。
- [ ] 真实初始化/UID/ACL/卷持久性、API能力变化及配套恢复成功；备份勾选不能代替可恢复证明。
- [ ] 不让旧镜像读取已迁移的新DB；失败保留维护状态与数据，只有已验证组合能恢复到匹配旧材料。
- [ ] 保数据卸载不删卷/下载/媒体/秘密；原身份重装验证，未知删除结果不重复盲删。

## 失败与回退

停在最近可核对阶段，保存旧/新精确资源身份和恢复指引，不接管外来同名资源。能否回退依产品迁移和配套数据证据决定；不满足时阻止自动回滚并按已验证备份恢复。关闭维护执行后诊断/历史读取仍可用。

## Out of Scope / Definition of Done

不做通用镜像管理、任意插件/FPK、删卷清库、重置产品密码或自动配置真实站点。按父路线统一PR门禁，复用verify_components/verify_desktop/verify_external并保留集成脚本，临时单测执行后删除。
