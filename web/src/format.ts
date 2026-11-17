import type { ContentType, Item } from './types'

/** 内容类型中文名称。 */
export const typeLabels: Record<ContentType, string> = { video: '视频', image: '图片', comic: '漫画', novel: '小说', music: '音乐' }

/** 判断内容是否使用以秒为单位的音视频播放流程。 */
export function isTimeMedia(item: Item): boolean {
  return item.type === 'video' || item.type === 'music'
}

/** 格式化字节大小，支持大文件。 */
export function formatSize(bytes: number): string {
  if (bytes <= 0) return '—'
  const units: string[] = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  const power: number = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** power).toFixed(power > 1 ? 1 : 0)} ${units[power]}`
}

/** 格式化音视频时长。 */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '时长待探测'
  const rounded: number = Math.floor(seconds)
  const hours: number = Math.floor(rounded / 3600)
  return [hours > 0 ? hours : null, Math.floor(rounded / 60) % 60, rounded % 60].filter((part: number | null): part is number => part !== null).map((part: number, index: number): string => index === 0 ? String(part) : String(part).padStart(2, '0')).join(':')
}

/** 将时间转换为本地日期。 */
export function formatDate(value: string): string {
  if (!value) return '尚未扫描'
  const date: Date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

/** 计算可展示的进度百分比。 */
export function progressPercent(item: Item): number {
  if (!item.progress?.total) return 0
  // 漫画页码与 TXT 段落采用零起始索引，显示进度时需要计入当前页。
  const offset: number = item.type === 'comic' || (item.type === 'novel' && extension(item.relativePath) !== 'EPUB') ? 1 : 0
  return Math.max(0, Math.min(100, ((item.progress.position + offset) / item.progress.total) * 100))
}

/** 获取不带路径分隔符的扩展名标签。 */
export function extension(path: string): string {
  const name: string = path.split(/[\\/]/).pop() || path
  return name.includes('.') ? (name.split('.').pop() || '').toUpperCase() : '目录'
}
