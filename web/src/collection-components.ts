import type { CollectionExternalService, EngineEvent, EngineStep, EngineTask, EngineTaskResponse } from './types'

/** 平台允许部署的固定资源组件标识。 */
export type CollectionComponentId = Exclude<CollectionExternalService, ''>

/** 四类固定组件的公开名称。 */
export const collectionComponentNames: Record<CollectionComponentId, string> = { moviepilot: 'MoviePilot', autobangumi: 'AutoBangumi', sonarr: 'Sonarr', radarr: 'Radarr' }

/** 资源组件的容器状态与首次配置入口，不包含 API 凭据。 */
export interface CollectionComponent {
  /** 固定产品标识。 */ id: CollectionComponentId
  /** 产品显示名称。 */ name: string
  /** 本平台固定部署版本。 */ version: string
  /** 容器是否存在及是否归属本安装。 */ presence: string
  /** 容器实际运行状态。 */ runtime: string
  /** 匿名服务探针结果，不表示已通过 API 认证。 */ readiness: string
  /** 已脱敏的中文说明。 */ message: string
  /** 启动操作所需的乐观修订号。 */ revision: number
  /** 是否允许主动确认后部署。 */ canDeploy: boolean
  /** 是否允许启动原受管容器。 */ canStart: boolean
  /** 仅绑定宿主回环的首次配置端口。 */ setupPort: number
  /** 归属与匿名探针均通过后可预填的平台内部地址。 */ serviceUrl: string
  /** 此组件最近的部署或启动任务编号。 */ latestTaskId: string
}

/** 私有助手返回的固定组件部署能力。 */
export interface CollectionComponentStatus {
  /** 部署助手可用性。 */ helper: string
  /** 实际部署前提或异常说明。 */ message: string
  /** 固定镜像和挂载模板版本。 */ templateVersion: string
  /** 目录访问授权版本。 */ noticeVersion: string
  /** 用户需要主动确认的目录访问说明。 */ noticeText: string
  /** 当前运行中的资源组件任务编号。 */ activeTaskId: string
  /** 四个固定产品的独立检测结果。 */ components: CollectionComponent[]
}

/** 沿用部署执行器的步骤字段，只替换本控制面的组件类型。 */
export interface CollectionComponentStep extends Omit<EngineStep, 'component'> {
  /** 当前固定资源组件。 */ component: CollectionComponentId
}

/** 沿用持久部署任务协议，不混入旧下载引擎或完整停止任务。 */
export interface CollectionComponentTask extends Omit<EngineTask, 'action' | 'components' | 'steps'> {
  /** 原始授权的部署或启动动作。 */ action: 'deploy' | 'start'
  /** 此控制面每次仅操作一个固定组件。 */ components: CollectionComponentId[]
  /** 固定组件的持久执行步骤。 */ steps: CollectionComponentStep[]
}

/** 沿用脱敏事件字段，只替换固定组件类型。 */
export interface CollectionComponentEvent extends Omit<EngineEvent, 'component'> {
  /** 事件对应组件，任务总体消息可为空。 */ component: CollectionComponentId | ''
}

/** 与现有执行器共享的有界任务快照与日志游标。 */
export interface CollectionComponentTaskResponse extends Omit<EngineTaskResponse, 'task' | 'events'> {
  /** 最新完整任务。 */ task: CollectionComponentTask
  /** 本轮新增的安全事件。 */ events: CollectionComponentEvent[]
}

/** 判断来自选择器或本地存储的组件标识是否为固定产品。 */
export function isCollectionComponentId(value: unknown): value is CollectionComponentId {
  return typeof value === 'string' && ['moviepilot', 'autobangumi', 'sonarr', 'radarr'].includes(value)
}

/** 运行状态与应用认证保持分离，未知状态不能推断为已安装或已连接。 */
export function collectionComponentState(component: CollectionComponent): string {
  if (component.presence === 'conflict') return '归属冲突'
  if (component.presence === 'missing') return '尚未部署'
  if (component.runtime === 'stopped') return '已停止'
  if (component.readiness === 'configuration_required') return '待首次配置 / API 连接'
  if (component.readiness === 'starting' || component.runtime === 'starting') return '正在启动'
  if (component.readiness === 'error') return '需要处理'
  return '状态待确认'
}

/** 部署任务采用产品用语，不与媒体获取或 API 接入混为一谈。 */
export function collectionComponentTaskLabel(state: string): string {
  return ({ queued: '等待部署执行', running: '组件部署中', completed: '部署完成，待配置与连接', failed: '部署遇到问题', interrupted: '部署已中断' } as Record<string, string>)[state] || '等待部署状态'
}

/** 首次配置提示严格按四类产品区分，不读取或猜测管理凭据。 */
export function collectionComponentSetupHint(service: CollectionComponentId): string {
  if (service === 'moviepilot') return '在 MoviePilot 初始化页面创建管理员和 API Key，然后配置站点、下载器、整理与媒体服务器。'
  if (service === 'autobangumi') return '在 AutoBangumi 首次登录后立即修改默认管理员密码，再配置下载器并生成 scope 为 api 的 Token。'
  return `在 ${collectionComponentNames[service]} 中完成管理员认证、索引器和下载器设置，再到设置中复制 API Key。`
}

/** 只有浏览器访问宿主回环时才预填回环入口，避免 NAS 远程用户打开错误设备。 */
export function collectionComponentLocalEntry(port: number): string {
  if (!Number.isInteger(port) || port < 1024 || port > 65535) return ''
  return ['localhost', '127.0.0.1', '::1', '[::1]'].includes(window.location.hostname) ? `http://127.0.0.1:${port}/` : ''
}
