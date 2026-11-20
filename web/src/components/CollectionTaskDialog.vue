<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ApiError, api, errorMessage, isAbort } from '../api'
import { collectionCanCancel, collectionCanRetry, collectionCanRetryImport, collectionImportLabel, collectionStateLabel, collectionTime } from '../collection'
import { formatSize, typeLabels } from '../format'
import type { CollectionEvent, CollectionTask, Item } from '../types'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

/** 详情保留获取和入库的独立安全快照。 */
interface Props { /** 首次打开时的任务。 */ initialTask: CollectionTask }
/** 服务端支持的固定任务操作。 */
type TaskAction = 'cancel' | 'retry' | 'retry_import'
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; updated: [task: CollectionTask]; 'open-item': [item: Item]; 'open-download': [id: string] }>()
const task = ref<CollectionTask>(props.initialTask)
const events = ref<CollectionEvent[]>([])
const loading = ref<boolean>(true)
const queryingError = ref<string>('')
const eventsError = ref<string>('')
const actionError = ref<string>('')
const feedback = ref<string>('')
const busy = ref<boolean>(false)
const openingItem = ref<boolean>(false)
const pendingRevisions: Map<TaskAction, number> = new Map<TaskAction, number>()
const actionControllers: Set<AbortController> = new Set<AbortController>()
let controller: AbortController | null = null
let pollTimer: ReturnType<typeof setTimeout> | undefined
let disposed: boolean = false
const active = computed<boolean>((): boolean => !['completed', 'failed', 'canceled', 'cancelled'].includes(task.value.state) || ['queued', 'importing', 'scanning', 'running'].includes(task.value.importState))

/** 详情只在前台、无请求时安排下一次同步。 */
function schedule(): void {
  clearTimeout(pollTimer)
  if (!disposed && !document.hidden) pollTimer = setTimeout((): void => { void refresh() }, active.value ? 3000 : 10000)
}
/** 快照和事件失败分别显示，旧快照不能覆盖较新的操作结果。 */
async function refresh(): Promise<void> {
  if (disposed || document.hidden || controller || busy.value) return
  clearTimeout(pollTimer)
  const request: AbortController = new AbortController()
  controller = request
  try {
    const [taskResult, eventResult] = await Promise.allSettled([
      api<{ task: CollectionTask }>(`/collection/tasks/${encodeURIComponent(task.value.id)}`, 'GET', undefined, request.signal),
      api<{ events: CollectionEvent[] }>(`/collection/tasks/${encodeURIComponent(task.value.id)}/events`, 'GET', undefined, request.signal),
    ])
    if (request.signal.aborted || controller !== request || disposed) return
    if (taskResult.status === 'fulfilled') {
      if (taskResult.value.task.revision >= task.value.revision) { task.value = taskResult.value.task; emit('updated', task.value) }
      queryingError.value = ''
    } else if (!isAbort(taskResult.reason)) queryingError.value = errorMessage(taskResult.reason)
    if (eventResult.status === 'fulfilled') { events.value = eventResult.value.events; eventsError.value = '' }
    else if (!isAbort(eventResult.reason)) eventsError.value = errorMessage(eventResult.reason)
  } finally { if (controller === request) { controller = null; loading.value = false; schedule() } }
}
/** 停止过时查询，以服务端最新状态重新读取。 */
function restart(): void { controller?.abort(); controller = null; clearTimeout(pollTimer); void refresh() }
/** 隐藏页暂停查询，恢复时立即读取持久任务。 */
function visibilityChanged(): void { if (document.hidden) { controller?.abort(); controller = null; clearTimeout(pollTimer) } else restart() }
/** 按原修订号重试未知结果，获取与入库重试使用不同操作。 */
async function performAction(action: TaskAction): Promise<void> {
  if (busy.value) return
  const revision: number = pendingRevisions.get(action) ?? task.value.revision
  pendingRevisions.set(action, revision)
  busy.value = true
  actionError.value = ''
  feedback.value = ''
  controller?.abort()
  controller = null
  clearTimeout(pollTimer)
  const request: AbortController = new AbortController()
  actionControllers.add(request)
  try {
    const result = await api<{ task: CollectionTask }>(`/collection/tasks/${encodeURIComponent(task.value.id)}/actions`, 'POST', { action, revision }, request.signal)
    if (request.signal.aborted || disposed) return
    task.value = result.task
    pendingRevisions.delete(action)
    emit('updated', result.task)
    feedback.value = action === 'cancel' ? '取消请求已受理，来源文件保留。' : action === 'retry_import' ? '已请求重新扫描，复用现有产物。' : '原任务已请求恢复，将先核对执行端状态。'
    console.info('资源收集任务操作已确认')
  } catch (failure: unknown) {
    if (!isAbort(failure)) {
      actionError.value = errorMessage(failure)
      if (failure instanceof ApiError && failure.status >= 400 && failure.status < 500) pendingRevisions.delete(action)
    }
  } finally { actionControllers.delete(request); busy.value = false; if (!disposed) restart() }
}
/** 读取真实媒体对象交回应用现有详情生命周期。 */
async function openItem(id: number): Promise<void> {
  if (openingItem.value) return
  openingItem.value = true
  actionError.value = ''
  const request: AbortController = new AbortController()
  actionControllers.add(request)
  try {
    const result = await api<{ item: Item }>(`/items/${id}`, 'GET', undefined, request.signal)
    if (!request.signal.aborted && !disposed) emit('open-item', result.item)
  } catch (failure: unknown) { if (!isAbort(failure)) actionError.value = errorMessage(failure) }
  finally { actionControllers.delete(request); openingItem.value = false }
}
onMounted((): void => { document.addEventListener('visibilitychange', visibilityChanged); void refresh() })
onBeforeUnmount((): void => { disposed = true; clearTimeout(pollTimer); controller?.abort(); for (const request of actionControllers) request.abort(); document.removeEventListener('visibilitychange', visibilityChanged) })
</script>

<template>
  <DialogShell title="收集任务详情" @close="emit('close')">
    <div class="collection-form">
      <div class="collection-callout"><h3>{{ task.title }}</h3><p class="field-hint">{{ typeLabels[task.contentType] || task.contentType }} · {{ formatSize(task.bytes) }} · 创建于 {{ collectionTime(task.createdAt) }}</p></div>
      <div class="collection-stage-grid"><div><span class="field-hint">获取阶段</span><strong>{{ collectionStateLabel(task.state) }}</strong></div><div><span class="field-hint">媒体入库</span><strong>{{ collectionImportLabel(task.importState) }}</strong></div></div>
      <p>{{ task.message || '任务已保存，等待后续处理。' }}</p>
      <p class="field-hint">最近更新：{{ collectionTime(task.updatedAt) }}{{ queryingError ? '（当前显示上次成功读取的快照）' : '' }}</p>
      <p v-if="queryingError" class="error-box" role="alert">任务状态暂未同步：{{ queryingError }} <button class="text-button" @click="restart">重新读取</button></p>
      <p v-if="feedback" class="collection-feedback" role="status">{{ feedback }}</p>
      <p v-if="actionError" class="error-box" role="alert">{{ actionError }} 结果不确定时，重试保留原修订号。</p>
      <div class="collection-actions">
        <button v-if="task.downloadId" class="primary-button" @click="emit('open-download', task.downloadId)"><Icon name="download" />前往下载详情与选文件</button>
        <button v-if="collectionCanCancel(task)" class="secondary-button" :disabled="busy" @click="performAction('cancel')">{{ busy ? '提交中…' : '取消获取' }}</button>
        <button v-if="collectionCanRetry(task)" class="secondary-button" :disabled="busy" @click="performAction('retry')">重试获取</button>
        <button v-if="collectionCanRetryImport(task)" class="secondary-button" :disabled="busy" @click="performAction('retry_import')">仅重试入库</button>
        <button v-for="(id, index) in task.itemIds" :key="id" class="secondary-button" :disabled="openingItem" @click="openItem(id)"><Icon name="play" :size="16" />打开媒体 {{ index + 1 }}</button>
      </div>
      <p v-if="task.downloadId" class="section-note">BT 的传输、文件选择和入库恢复由原下载中心处理，做种数据继续保留。</p>
      <p v-else class="field-hint">取消只停止本次受管获取，不删除原文件。入库失败可单独重试扫描，不重新获取。</p>
      <div class="collection-section-heading"><h3>处理记录</h3><span v-if="loading" class="field-hint">读取中…</span></div>
      <p v-if="eventsError" class="error-box" role="alert">处理记录暂不可用：{{ eventsError }}</p>
      <ol v-if="events.length" class="collection-events"><li v-for="event in events" :key="event.id"><div><strong>{{ collectionStateLabel(event.stage) }}</strong><time>{{ collectionTime(event.createdAt) }}</time></div><p>{{ event.message }}</p></li></ol>
      <p v-else-if="!loading && !eventsError" class="field-hint">尚无处理记录。</p>
      <div class="dialog-actions"><button class="secondary-button" @click="emit('close')">关闭详情</button></div>
    </div>
  </DialogShell>
</template>
