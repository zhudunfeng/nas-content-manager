import { formatSize } from './format'
import type { Download } from './types'

/** 传输状态中文说明，未知值不被误报为成功。 */
export function downloadStateLabel(state: string): string {
  const labels: Record<string, string> = { submitting: '正在提交', metadata: '获取元数据', awaiting_selection: '等待选择文件', queued: '等待下载', downloading: '下载中', paused: '已暂停', checking: '校验中', moving: '发布文件中', seeding: '下载完成 · 做种中', completed: '下载完成', error: '下载异常', missing: '引擎任务缺失', removed: '已移除管理' }
  return labels[state] || '待同步状态'
}

/** 独立显示入库结果，不将下载完成等同于可播放。 */
export function importStateLabel(state: string): string {
  const labels: Record<string, string> = { pending: '等待下载完成', publishing: '验证并发布', queued: '等待入库', scanning: '入库扫描中', imported: '已入库', no_supported_files: '没有可入库的格式', failed: '入库失败', target_missing: '目标媒体库缺失' }
  return labels[state] || '等待入库'
}

/** 仅以安全整数展示字节数，超出 JavaScript 精度时明确提示。 */
export function downloadBytes(bytes: number): string {
  if (!Number.isSafeInteger(bytes) || bytes < 0) return '大小超出安全显示范围'
  return bytes === 0 ? '0 B' : formatSize(bytes)
}

/** 不使用任何三十二位截断计算超大文件的进度。 */
export function downloadPercent(download: Download): number {
  if (!Number.isSafeInteger(download.sizeBytes) || !Number.isSafeInteger(download.downloadedBytes) || download.sizeBytes <= 0) return 0
  return Math.max(0, Math.min(100, download.downloadedBytes / download.sizeBytes * 100))
}

/** 下载器未实时连接时不把缓存速度伪装为当前速度。 */
export function transferSpeed(bytes: number, live: boolean): string {
  return live ? `${downloadBytes(bytes)}/s` : '未知'
}

/** 仅显示可靠且有有效速度的剩余时间估计。 */
export function downloadEta(download: Download, live: boolean): string {
  const seconds: number | null = download.etaSeconds
  if (!live || download.downloadSpeed <= 0 || seconds === null || !Number.isSafeInteger(seconds) || seconds < 0) return '未知'
  if (seconds < 60) return '不足 1 分钟'
  if (seconds < 3600) return `约 ${Math.ceil(seconds / 60)} 分钟`
  if (seconds < 86400) return `约 ${(seconds / 3600).toFixed(1)} 小时`
  return `约 ${(seconds / 86400).toFixed(1)} 天`
}

/** 显示精确的本地同步时间，空时间不伪装为新鲜数据。 */
export function syncTime(value: string): string {
  const date: Date = new Date(value)
  return value && !Number.isNaN(date.getTime()) ? date.toLocaleString('zh-CN', { hour12: false }) : '尚未成功同步'
}
