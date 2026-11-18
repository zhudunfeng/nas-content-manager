import type { EngineComponent, EngineComponentId, EngineLayer } from './types'

/** 固定组件名称，不接受网页自定义组件或镜像。 */
export const engineNames: Record<EngineComponentId, string> = { qbittorrent: '下载引擎', jackett: '搜索引擎' }

/** 明确区分关闭、外接和本机托管模式。 */
export function engineModeLabel(mode: string): string {
  return ({ disabled: '未启用', external: '外部接入', managed: '本机托管' } as Record<string, string>)[mode] || '模式未知'
}

/** 使用已确认的归属和连接信息生成状态，超时不能显示成未安装。 */
export function engineStateLabel(component: EngineComponent): string {
  if (component.mode === 'disabled') return '尚未启用'
  if (component.readiness === 'ready') return '连接就绪'
  if (component.presence === 'conflict') return '配置冲突'
  if (component.readiness === 'initializing') return '正在初始化'
  if (component.mode === 'managed' && component.presence === 'missing') return '尚未安装'
  if (component.mode === 'managed' && component.runtime === 'stopped') return '已停止'
  const labels: Record<string, string> = { not_configured: '等待初始化', initializing: '正在初始化', configuration_required: '待配置来源', connecting: '正在接入', auth_failed: '认证失败', incompatible: '版本不兼容', offline: '连接离线', error: '需要处理' }
  return labels[component.readiness] || (component.runtime === 'offline' ? '连接离线' : '状态待确认')
}

/** 以中文显示持久阶段，不根据步骤序号编造完成百分比。 */
export function enginePhaseLabel(phase: string): string {
  const labels: Record<string, string> = { queued: '等待执行', preflight: '环境检查', pulling: '拉取固定镜像', preparing: '准备持久目录', creating: '创建受管容器', starting: '启动引擎', credentials: '衔接私密凭据', healthcheck: '检查接口与版本', publishing: '发布连接配置', completed: '部署完成' }
  return labels[phase] || '等待进度更新'
}

/** 展示任务生命周期，并与应用实际连接状态保持分离。 */
export function engineTaskLabel(state: string): string {
  return ({ queued: '等待执行', running: '初始化进行中', completed: '部署已完成', failed: '初始化遇到问题', interrupted: '初始化已中断' } as Record<string, string>)[state] || '等待任务状态'
}

/** 只在 Docker 报告可信分母时显示镜像层百分比。 */
export function engineLayerPercent(layer: EngineLayer): number | undefined {
  if (!Number.isSafeInteger(layer.current) || !Number.isSafeInteger(layer.total) || layer.current < 0 || layer.total <= 0) return undefined
  return Math.max(0, Math.min(100, layer.current / layer.total * 100))
}
