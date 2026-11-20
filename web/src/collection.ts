import type { CollectionKind, CollectionTask } from './types'

/** 固定来源类型的中文名称。 */
export const collectionKindLabels: Record<CollectionKind, string> = { rss: 'RSS 订阅', web: '网页音视频', local: '本地目录', remote: '远程文件', external: '外部自动化' }
/** 固定来源类型的本地图标。 */
export const collectionKindIcons: Record<CollectionKind, string> = { rss: 'rss', web: 'link', local: 'folder', remote: 'storage', external: 'external' }
/** 每个网页批次的显式条目上限。 */
export const collectionBatchLimit: number = 10
/** 统一任务和候选列表的每页条数。 */
export const collectionPageSize: number = 20
/** 将真实获取阶段转换为中文，不从传输完成推断入库结果。 */
export function collectionStateLabel(state: string): string {
  const labels: Record<string, string> = { pending_review: '待确认', queued: '排队中', dispatching: '正在提交', retrying: '恢复对账中', canceling: '正在取消', acquiring: '获取中', verifying: '校验中', publishing: '发布中', importing: '扫描入库中', completed: '获取完成', failed: '获取失败', canceled: '已取消', cancelled: '已取消', reconciliation_required: '需要对账', awaiting_selection: '待选文件', pending_selection: '待选文件' }
  return labels[state] || state || '等待状态'
}
/** 独立解释扫描与入库状态。 */
export function collectionImportLabel(state: string): string {
  const labels: Record<string, string> = { pending: '待入库', none: '尚未入库', not_started: '尚未入库', queued: '等待扫描', importing: '扫描入库中', scanning: '扫描中', running: '扫描中', completed: '已入库', imported: '已入库', ready: '已入库', failed: '入库失败', blocked: '入库受阻', canceled: '已取消入库', cancelled: '已取消入库' }
  return labels[state] || state || '尚未入库'
}
/** 将候选的决定转换为中文。 */
export function collectionDecisionLabel(decision: string): string {
  const labels: Record<string, string> = { review: '待确认', pending_review: '待确认', matched: '规则命中', baseline: '历史基线', ignored: '已忽略', approved: '已创建任务', acquired: '已创建任务', acquiring: '已创建任务', queued: '已排队', rejected: '未命中', duplicate: '重复资源' }
  return labels[decision] || decision || '待确认'
}
/** 显示中文日期和时间，未知时间保持未知。 */
export function collectionTime(value: string): string {
  const time: Date = new Date(value)
  return value && Number.isFinite(time.getTime()) ? time.toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '尚无记录'
}
/** 明确校验浏览器可打开的公开应用入口。 */
export function collectionExternalUrl(value: string): string {
  try {
    const url: URL = new URL(value)
    return ['https:', 'http:'].includes(url.protocol) && !url.username && !url.password && !url.search && !url.hash ? url.href : ''
  } catch { return '' }
}
/** 只在非 BT 的活动任务中提供取消入口，最终能力由服务端核实。 */
export function collectionCanCancel(task: CollectionTask): boolean {
  return !task.downloadId && ['queued', 'dispatching', 'acquiring', 'reconciliation_required'].includes(task.state)
}
/** 实际适配器已经失败时允许使用原有计划重试。 */
export function collectionCanRetry(task: CollectionTask): boolean {
  return !task.downloadId && ['failed', 'reconciliation_required'].includes(task.state)
}
/** 只对获取已完成且扫描失败的任务开放独立入库重试。 */
export function collectionCanRetryImport(task: CollectionTask): boolean {
  return !task.downloadId && task.state === 'importing' && task.importState === 'failed'
}
