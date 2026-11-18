<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ApiError, api, createRequestId, errorMessage, isAbort } from '../api'
import { downloadBytes, downloadEta, downloadPercent, downloadStateLabel, importStateLabel, syncTime, transferSpeed } from '../downloads'
import { typeLabels } from '../format'
import type { Download, DownloadAction, DownloadPage, DownloadStatus, EngineComponentId, Item, TorrentSearchResult } from '../types'
import AddDownloadDialog from './AddDownloadDialog.vue'
import DownloadDetailDialog from './DownloadDetailDialog.vue'
import DialogShell from './DialogShell.vue'
import EngineSetupDialog from './EngineSetupDialog.vue'
import EngineStatusCards from './EngineStatusCards.vue'
import Icon from './Icon.vue'
import TorrentSearchDialog from './TorrentSearchDialog.vue'

/** 结果不确定的操作保留原始请求键和修订号。 */
interface PendingAction {
  /** 服务端幂等键。 */ requestId: string
  /** 该操作首次提交时看到的修订号。 */ revision: number
}
const emit = defineEmits<{ 'open-item': [item: Item] }>()
/** 下载列表每页展示三十项。 */
const pageSize: number = 30
/** 正常状态轮询后端快照的间隔，不直接轮询下载器。 */
const pollingMilliseconds: number = 3000
/** 故障或关闭模块时降低轮询频率，保持基础浏览流畅。 */
const offlinePollingMilliseconds: number = 15000
/** 搜索输入防抖间隔。 */
const searchDelayMilliseconds: number = 300
/** 状态筛选值与服务端下载列表契约一致。 */
const filters: { /** 筛选标识。 */ value: string; /** 中文名称。 */ label: string }[] = [{ value: '', label: '全部' }, { value: 'active', label: '进行中' }, { value: 'paused', label: '已暂停' }, { value: 'completed', label: '已完成' }, { value: 'error', label: '异常' }]
const connection = ref<DownloadStatus | null>(null)
const downloads = ref<Download[]>([])
const total = ref<number>(0)
const page = ref<number>(1)
const filter = ref<string>('')
const search = ref<string>('')
const query = ref<string>('')
const loading = ref<boolean>(true)
const statusError = ref<string>('')
const listError = ref<string>('')
const actionError = ref<string>('')
const feedback = ref<string>('')
const testing = ref<boolean>(false)
const adding = ref<boolean>(false)
const searchingTorrents = ref<boolean>(false)
const managingEngines = ref<boolean>(false)
const engineComponent = ref<EngineComponentId | undefined>(undefined)
const engineRefreshKey = ref<number>(0)
const selectedSearchResult = ref<TorrentSearchResult | null>(null)
const selectedDownload = ref<Download | null>(null)
const removing = ref<Download | null>(null)
const busyIds = ref<Set<string>>(new Set<string>())
const pendingActions: Map<string, PendingAction> = new Map<string, PendingAction>()
const actionControllers: Set<AbortController> = new Set<AbortController>()
let controller: AbortController | null = null
let testController: AbortController | null = null
let pollTimer: ReturnType<typeof setTimeout> | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined
let disposed: boolean = false
const live = computed<boolean>((): boolean => connection.value?.enabled === true && connection.value.state === 'ready' && !statusError.value)
const canAdd = computed<boolean>((): boolean => live.value && connection.value?.canSelectFiles === true)
const connectionTitle = computed<string>((): string => {
  if (statusError.value) return '下载器状态暂不可用'
  const labels: Record<string, string> = { disabled: 'BT 下载尚未启用', connecting: '正在连接下载器', ready: '下载器已连接', offline: '下载器暂不可用', auth_failed: '下载器认证失败', incompatible: '下载器版本或能力不兼容' }
  return labels[connection.value?.state || ''] || '正在读取下载器状态'
})
const connectionDescription = computed<string>((): string => {
  if (statusError.value) return statusError.value
  if (connection.value?.error) return connection.value.error
  const labels: Record<string, string> = { disabled: '可在引擎管理中查看托管部署步骤，或通过服务端配置接入已有 qBittorrent。', connecting: '基础媒体库可以继续使用，连接完成后自动更新。', ready: '传输由 qBittorrent 完成；栖库负责选择文件、安全发布与内容入库。', offline: '展示上次同步状态，速度与剩余时间暂记为未知。恢复连接后继续对账。', auth_failed: '请检查服务端配置的下载器账户与密码文件；栖库登录会话不受影响。', incompatible: '请检查部署文档中的版本与文件选择能力要求，修复后重新测试连接。' }
  return labels[connection.value?.state || ''] || '正在读取模块状态，请稍候。'
})

/** 只在当前下载页面可见时安排下一次读取，防止请求重叠。 */
function scheduleRefresh(): void {
  clearTimeout(pollTimer)
  if (disposed || document.hidden) return
  pollTimer = setTimeout((): void => { void refresh(true) }, live.value ? pollingMilliseconds : offlinePollingMilliseconds)
}

/** 读取连接、列表和打开的详情；各类失败保留独立反馈。 */
async function refresh(silent: boolean = false): Promise<void> {
  if (disposed || document.hidden || controller) return
  clearTimeout(pollTimer)
  const request: AbortController = new AbortController()
  controller = request
  if (!silent) loading.value = true
  try {
    let status: DownloadStatus
    try { status = await api<DownloadStatus>('/downloads/status', 'GET', undefined, request.signal) }
    catch (failure: unknown) {
      if (!isAbort(failure) && controller === request) statusError.value = errorMessage(failure)
      return
    }
    if (request.signal.aborted || controller !== request) return
    connection.value = status
    statusError.value = ''
    // 关闭传输协调器后仍读取本地历史记录，已发布内容与只读详情继续可用。
    const params: URLSearchParams = new URLSearchParams({ page: String(page.value), pageSize: String(pageSize), state: filter.value, q: query.value })
    const detailId: string | undefined = selectedDownload.value?.id
    const [listResult, detailResult] = await Promise.allSettled([
      api<DownloadPage>(`/downloads?${params}`, 'GET', undefined, request.signal),
      detailId ? api<{ download: Download }>(`/downloads/${encodeURIComponent(detailId)}`, 'GET', undefined, request.signal) : Promise.resolve(null),
    ])
    if (request.signal.aborted || controller !== request) return
    if (listResult.status === 'fulfilled') {
      downloads.value = listResult.value.downloads
      total.value = listResult.value.total
      listError.value = ''
      if (page.value > 1 && !downloads.value.length && total.value <= (page.value - 1) * pageSize) page.value = Math.max(1, Math.ceil(total.value / pageSize))
    } else if (!isAbort(listResult.reason)) listError.value = errorMessage(listResult.reason)
    if (detailResult.status === 'fulfilled' && detailResult.value && selectedDownload.value && selectedDownload.value.id === detailId) {
      if (detailResult.value.download.revision >= selectedDownload.value.revision) selectedDownload.value = detailResult.value.download
    } else if (detailResult.status === 'rejected' && !isAbort(detailResult.reason)) {
      // 详情失败不能被成功列表请求抹去，也不把任务伪装成已删除。
      listError.value = `任务详情暂未同步：${errorMessage(detailResult.reason)}`
    }
  } finally {
    if (controller === request) { controller = null; loading.value = false; scheduleRefresh() }
  }
}

/** 筛选或操作改变时取消过时快照，再读取当前服务端状态。 */
function restartRefresh(silent: boolean = false): void {
  controller?.abort()
  controller = null
  clearTimeout(pollTimer)
  void refresh(silent)
}

/** 页面隐藏时停止轮询并取消查询，恢复可见后立即同步。 */
function visibilityChanged(): void {
  if (document.hidden) { clearTimeout(pollTimer); controller?.abort(); controller = null }
  else restartRefresh(true)
}

/** 测试后端固定下载器配置，不接受浏览器提供任意目标地址。 */
async function testConnection(): Promise<void> {
  if (testing.value) return
  testing.value = true
  statusError.value = ''
  const request: AbortController = new AbortController()
  testController = request
  try {
    const result: DownloadStatus = await api<DownloadStatus>('/downloads/connection-test', 'POST', undefined, request.signal)
    if (!request.signal.aborted) { connection.value = result; console.info('下载器连接测试已完成'); restartRefresh(true) }
  } catch (failure: unknown) { if (!isAbort(failure)) statusError.value = errorMessage(failure) }
  finally { if (!disposed) testing.value = false }
}

/** 以新集合更新操作锁，让同一任务不能被重复点击。 */
function setBusy(id: string, busy: boolean): void {
  const next: Set<string> = new Set<string>(busyIds.value)
  if (busy) next.add(id)
  else next.delete(id)
  busyIds.value = next
}

/** 将服务端确认的详情应用到列表，并取消之前发出的旧快照。 */
function updated(download: Download): void {
  controller?.abort()
  controller = null
  downloads.value = downloads.value.map((entry: Download): Download => entry.id === download.id ? download : entry)
  if (selectedDownload.value?.id === download.id) selectedDownload.value = download
  restartRefresh(true)
}

/** 提交受管任务操作，未知网络结果保留首次请求键以供重试。 */
async function performAction(download: Download, action: DownloadAction): Promise<boolean> {
  if (!live.value || busyIds.value.has(download.id)) return false
  const key: string = `${download.id}:${action}`
  const pending: PendingAction = pendingActions.get(key) || { requestId: createRequestId(), revision: download.revision }
  pendingActions.set(key, pending)
  setBusy(download.id, true)
  actionError.value = ''
  feedback.value = ''
  const request: AbortController = new AbortController()
  actionControllers.add(request)
  try {
    const result = await api<{ download: Download }>(`/downloads/${encodeURIComponent(download.id)}/actions`, 'POST', { ...pending, action }, request.signal)
    if (request.signal.aborted || disposed) return false
    pendingActions.delete(key)
    updated(result.download)
    feedback.value = action === 'remove' ? '已移除下载管理，下载文件与已入库内容保留。' : '操作已提交，正在同步下载器状态。'
    console.info(action === 'remove' ? '下载任务已移除管理，原文件保留' : '下载任务操作已确认')
    return true
  } catch (failure: unknown) {
    if (!isAbort(failure) && !disposed) {
      actionError.value = errorMessage(failure)
      if (failure instanceof ApiError && failure.status >= 400 && failure.status < 500) pendingActions.delete(key)
      restartRefresh(true)
    }
    return false
  } finally { actionControllers.delete(request); if (!disposed) setBusy(download.id, false) }
}

/** 移除只归档下载管理，确认文案明确保留文件。 */
async function removeDownload(): Promise<void> {
  if (!removing.value) return
  if (await performAction(removing.value, 'remove')) removing.value = null
}

/** 先卸载添加弹窗并恢复焦点，再显示新任务的文件清单。 */
async function created(download: Download): Promise<void> {
  adding.value = false
  selectedSearchResult.value = null
  await nextTick()
  if (disposed) return
  selectedDownload.value = download
  feedback.value = '下载已受理。获取元数据后，请在详情中确认文件选集。'
  restartRefresh(true)
}

/** 常规添加始终使用原来的磁力与种子上传表单。 */
function openAdd(): void {
  selectedSearchResult.value = null
  adding.value = true
}

/** 关闭添加后丢弃当前选中引用，不把旧搜索结果带入下一次操作。 */
function closeAdd(): void {
  adding.value = false
  selectedSearchResult.value = null
}

/** 先卸载搜索弹窗恢复焦点，再开启确认表单，避免嵌套原生对话框。 */
async function selectSearchResult(result: TorrentSearchResult): Promise<void> {
  if (!canAdd.value || !result.downloadable) return
  searchingTorrents.value = false
  await nextTick()
  if (disposed || !canAdd.value) return
  selectedSearchResult.value = result
  adding.value = true
}

/** 从卡片打开引擎管理，不自动执行初始化。 */
function manageEngines(component?: EngineComponentId): void {
  engineComponent.value = component
  managingEngines.value = true
}

/** 搜种未配置入口先关闭原弹窗，再交接焦点给引擎管理。 */
async function manageSearchEngine(): Promise<void> {
  searchingTorrents.value = false
  await nextTick()
  if (!disposed) manageEngines('jackett')
}

/** 关闭管理面板后同步卡片与现有下载能力。 */
function closeEngineManagement(): void {
  managingEngines.value = false
  engineRefreshKey.value += 1
  restartRefresh(true)
}

/** 打开详情后由同一轮询读取服务端最新修订。 */
function openDetails(download: Download): void {
  selectedDownload.value = download
  restartRefresh(true)
}

/** 先关闭下载弹窗，避免与现有媒体详情抢占原生模态焦点。 */
async function openItem(item: Item): Promise<void> {
  selectedDownload.value = null
  await nextTick()
  if (!disposed) emit('open-item', item)
}

/** 仅在已确认选集且处于可恢复状态时显示继续按钮。 */
function canStart(download: Download): boolean {
  return download.selectionLocked && ['paused', 'error'].includes(download.state)
}

/** 传输或做种中的任务可请求暂停，不改变已发布内容的索引。 */
function canPause(download: Download): boolean {
  return ['downloading', 'queued', 'seeding', 'metadata'].includes(download.state)
}

watch(search, (value: string): void => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout((): void => { query.value = value.trim(); page.value = 1 }, searchDelayMilliseconds)
})
watch([filter, query], (): void => { if (page.value !== 1) page.value = 1; else restartRefresh() })
watch(page, (): void => { restartRefresh() })
onMounted((): void => { document.addEventListener('visibilitychange', visibilityChanged); void refresh() })
onBeforeUnmount((): void => {
  disposed = true
  clearTimeout(pollTimer)
  clearTimeout(searchTimer)
  controller?.abort()
  testController?.abort()
  for (const request of actionControllers) request.abort()
  document.removeEventListener('visibilitychange', visibilityChanged)
})
</script>

<template>
  <section class="download-center" aria-label="BT 下载中心">
    <div class="download-connection" :class="{ 'download-connection-muted': !live }">
      <span class="download-connection-icon"><Icon :name="live ? 'download' : connection?.state === 'disabled' ? 'storage' : 'warning'" :size="25" /></span>
      <div class="download-connection-copy"><h2>{{ connectionTitle }}<span v-if="live && connection?.version" class="status-pill">{{ connection.version }}</span></h2><p>{{ connectionDescription }}</p><small v-if="connection?.enabled">上次同步：{{ syncTime(connection.lastSyncedAt) }}<span v-if="!live"> · 展示历史状态</span></small></div>
      <button class="secondary-button" :disabled="testing || !connection?.enabled" @click="testConnection"><Icon name="refresh" :size="16" />{{ testing ? '测试中…' : '测试连接' }}</button>
    </div>
    <p v-if="live && !connection?.canSelectFiles" class="error-box" role="alert">当前下载器尚未通过安全文件选择能力检查，暂不能添加任务。请核对版本和部署说明后测试连接。</p>
    <EngineStatusCards :paused="managingEngines" :refresh-key="engineRefreshKey" @manage="manageEngines" />
    <div class="download-toolbar"><div class="download-filters" role="group" aria-label="下载状态筛选"><button v-for="entry in filters" :key="entry.value" :class="{ active: filter === entry.value }" :aria-pressed="filter === entry.value" @click="filter = entry.value">{{ entry.label }}</button></div><div class="download-toolbar-actions"><button class="secondary-button" @click="searchingTorrents = true"><Icon name="search" />搜种</button><button class="primary-button" :disabled="!canAdd" @click="openAdd"><Icon name="plus" />添加下载</button></div></div>
    <div class="download-search-row"><label class="download-search"><Icon name="search" :size="18" /><input v-model="search" aria-label="搜索下载任务" placeholder="搜索下载任务名称…" maxlength="200" /><button v-if="search" class="icon-button" aria-label="清除下载搜索" @click="search = ''"><Icon name="close" :size="16" /></button></label><span>{{ total.toLocaleString() }} 个任务</span><button class="icon-button" aria-label="刷新下载列表" :disabled="loading" @click="restartRefresh()"><Icon name="refresh" :size="18" /></button></div>
    <p v-if="listError" class="error-box" role="alert">{{ listError }}<button class="text-button" @click="restartRefresh()">重新读取</button></p>
    <p v-if="actionError && !removing" class="error-box" role="alert">{{ actionError }}</p>
    <p v-if="feedback" class="download-feedback" role="status"><Icon name="check" :size="17" />{{ feedback }}</p>
    <div v-if="loading && !downloads.length" class="download-inline-state" role="status" aria-busy="true"><span class="loader" />正在读取下载任务…</div>
    <div v-else-if="downloads.length" class="download-list" :aria-busy="loading">
      <article v-for="download in downloads" :key="download.id" class="download-task">
        <div class="download-task-top"><span class="library-icon" :class="`shortcut-${download.contentType}`"><Icon :name="download.contentType" :size="24" /></span><div class="download-task-copy"><button class="download-title" @click="openDetails(download)">{{ download.name || '正在获取任务名称…' }}</button><div class="download-badges"><span class="download-type-label">{{ typeLabels[download.contentType] }}</span><span class="status-pill" :class="{ warning: ['error', 'missing'].includes(download.state) }">{{ downloadStateLabel(download.state) }}</span><span class="download-import-label" :class="{ 'danger-text': ['failed', 'target_missing', 'no_supported_files'].includes(download.importState) }">{{ importStateLabel(download.importState) }}</span></div></div><strong class="download-percentage">{{ downloadPercent(download).toFixed(1) }}<small>%</small></strong></div>
        <progress class="download-progress" :value="downloadPercent(download)" max="100" :aria-label="`${download.name} 下载进度`" />
        <div class="download-task-bottom"><div class="download-task-stats"><span>{{ downloadBytes(download.downloadedBytes) }} / {{ downloadBytes(download.sizeBytes) }}</span><span>↓ {{ transferSpeed(download.downloadSpeed, live) }}<span class="download-upload-speed"> · ↑ {{ transferSpeed(download.uploadSpeed, live) }}</span></span><span>剩余 {{ downloadEta(download, live) }}</span><span v-if="!live">上次同步 {{ syncTime(download.lastSyncedAt) }}</span></div><div class="download-task-actions"><button v-if="canStart(download)" class="secondary-button" :disabled="!live || busyIds.has(download.id)" @click="performAction(download, 'start')"><Icon name="play" :size="16" />继续</button><button v-if="canPause(download)" class="secondary-button" :disabled="!live || busyIds.has(download.id)" @click="performAction(download, 'pause')"><Icon name="pause" :size="16" />暂停</button><button class="secondary-button" :disabled="busyIds.has(download.id)" @click="openDetails(download)">{{ download.state === 'awaiting_selection' ? '选择文件' : '详情' }}</button><button class="icon-button" :disabled="!live || busyIds.has(download.id)" :aria-label="`重新检查任务：${download.name}`" @click="performAction(download, 'reconcile')"><Icon name="refresh" :size="17" /></button><button class="icon-button danger-text" :disabled="!live || busyIds.has(download.id)" :aria-label="`移除下载管理并保留文件：${download.name}`" @click="removing = download; actionError = ''"><Icon name="trash" :size="17" /></button></div></div>
        <p v-if="download.lastError" class="download-task-error"><Icon name="warning" :size="15" />{{ download.lastError }}</p>
      </article>
    </div>
    <div v-else-if="live && !listError && !statusError" class="empty-state download-empty"><div class="empty-art" aria-hidden="true"><span /><div><Icon :name="query ? 'search' : 'download'" :size="46" /></div><span /></div><span class="eyebrow">{{ query || filter ? '寻找你的下载' : '收藏，从这里继续' }}</span><h2>{{ query || filter ? '没有匹配的下载任务' : '把喜欢的内容，带回自己的空间' }}</h2><p>{{ query || filter ? '试试其他名称，或切换到全部任务。' : '添加磁力链接或种子文件，选择所需内容。完成后自动入库，随时观看、收听与阅读。' }}</p><button v-if="query || filter" class="secondary-button" @click="search = ''; query = ''; filter = ''">清除筛选</button><button v-else class="primary-button" :disabled="!canAdd" @click="openAdd"><Icon name="plus" />添加第一个下载</button></div>
    <div v-if="total > pageSize" class="pagination"><button class="secondary-button" :disabled="page <= 1 || loading" @click="page -= 1"><Icon name="back" />上一页</button><span>第 {{ page }} / {{ Math.ceil(total / pageSize) }} 页</span><button class="secondary-button" :disabled="page * pageSize >= total || loading" @click="page += 1">下一页<Icon name="chevron" /></button></div>
  </section>
  <TorrentSearchDialog v-if="searchingTorrents" :can-download="canAdd" @close="searchingTorrents = false" @select="selectSearchResult" @manage-engines="manageSearchEngine" />
  <EngineSetupDialog v-if="managingEngines" :initial-component="engineComponent" @close="closeEngineManagement" @changed="restartRefresh(true)" />
  <AddDownloadDialog v-if="adding" :search-result="selectedSearchResult" @close="closeAdd" @created="created" />
  <DownloadDetailDialog v-if="selectedDownload" :key="selectedDownload.id" :download="selectedDownload" :live="live" @close="selectedDownload = null" @updated="updated" @open-item="openItem" />
  <DialogShell v-if="removing" title="移除下载管理" @close="!busyIds.has(removing.id) && (removing = null)"><div class="confirm-body"><p>移除“{{ removing.name }}”的下载管理？</p><p class="muted">此操作会从下载器移除任务，保留所有下载文件和已发布的入库内容，不删除媒体数据。</p><p v-if="actionError" class="error-box" role="alert">{{ actionError }}</p><div class="dialog-actions"><button class="secondary-button" :disabled="busyIds.has(removing.id)" @click="removing = null">保留任务</button><button class="danger-button" :disabled="!live || busyIds.has(removing.id)" @click="removeDownload">{{ busyIds.has(removing.id) ? '正在确认…' : '移除管理，保留文件' }}</button></div></div></DialogShell>
</template>
