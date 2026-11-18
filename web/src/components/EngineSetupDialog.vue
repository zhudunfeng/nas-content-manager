<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ApiError, api, createRequestId, errorMessage, isAbort } from '../api'
import { downloadBytes, syncTime } from '../downloads'
import { engineLayerPercent, engineModeLabel, engineNames, enginePhaseLabel, engineStateLabel, engineTaskLabel } from '../engines'
import type { EngineComponent, EngineComponentId, EngineEvent, EngineSourceSync, EngineStatus, EngineTask, EngineTaskResponse } from '../types'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

/** 面板可以从具体组件入口打开，但不自动执行部署。 */
interface Props { /** 初次默认勾选的组件。 */ initialComponent?: EngineComponentId }
/** 未知结果的操作保留同一个幂等键与最初修订。 */
interface PendingCommand {
  /** 操作语义的安全本地标识。 */ key: string
  /** 服务端幂等键。 */ requestId: string
  /** 首次提交时的任务或组件修订。 */ revision: number
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; changed: [] }>()
/** 任务进行中每两秒读取一次，必须等上一轮完成后才计时。 */
const activePollMilliseconds: number = 2000
/** 无运行任务时降低控制面读取频率。 */
const idlePollMilliseconds: number = 5000
/** 单个浏览器请求超时只停止等待，不取消服务端初始化任务。 */
const requestTimeoutMilliseconds: number = 20000
/** 仅存储随机操作编号，不在浏览器持久化任何引擎密钥。 */
const pendingStorageKey: string = 'nas-engine-pending-command'
/** 保存上次查看的任务编号以支持页面刷新后恢复观察。 */
const taskStorageKey: string = 'nas-engine-observed-task'
const status = ref<EngineStatus | null>(null)
const task = ref<EngineTask | null>(null)
const events = ref<EngineEvent[]>([])
const loading = ref<boolean>(true)
const busy = ref<boolean>(false)
const error = ref<string>('')
const readError = ref<string>('')
const feedback = ref<string>('')
const eventGap = ref<boolean>(false)
const selected = ref<EngineComponentId[]>([])
const acceptNotice = ref<boolean>(false)
const sourceSync = ref<EngineSourceSync | null>(null)
const selectedSources = ref<string[]>([])
let initializedSelection: boolean = false
let controller: AbortController | null = null
let timer: ReturnType<typeof setTimeout> | undefined
let disposed: boolean = false
let nextSequence: number = 0
let lastCompletedRevision: string = ''
let pending: PendingCommand | null = readPending()
let observedTaskId: string = readStorage(taskStorageKey)
const taskActive = computed<boolean>((): boolean => task.value !== null && ['queued', 'running'].includes(task.value.state))
const canRetry = computed<boolean>((): boolean => !readError.value && task.value !== null && ['failed', 'interrupted'].includes(task.value.state) && status.value?.helper === 'ready')
const missing = computed<EngineComponent[]>((): EngineComponent[] => status.value?.components.filter((component: EngineComponent): boolean => component.mode === 'managed' && component.presence === 'missing' && component.canInitialize) || [])
const selectedMissing = computed<EngineComponentId[]>((): EngineComponentId[] => selected.value.filter((id: EngineComponentId): boolean => missing.value.some((component: EngineComponent): boolean => component.id === id)).sort())
const requiresNotice = computed<boolean>((): boolean => selectedMissing.value.includes('qbittorrent'))
const canInitialize = computed<boolean>((): boolean => !busy.value && !readError.value && !taskActive.value && status.value?.helper === 'ready' && selectedMissing.value.length > 0 && (!requiresNotice.value || acceptNotice.value))
const helperLabel = computed<string>((): string => ({ ready: '初始化助手已连接', unavailable: '初始化助手暂不可用', unsupported: '当前部署不支持自动初始化', not_enabled: '尚未启用初始化助手' } as Record<string, string>)[status.value?.helper || ''] || '正在检测初始化助手')

/** 访问受限浏览器存储时降级为本次页面记忆，不影响服务端任务。 */
function readStorage(key: string): string {
  try { return sessionStorage.getItem(key) || '' }
  catch { console.warn('浏览器未允许保存引擎操作编号，服务端任务仍会继续'); return '' }
}

/** 本地仅记录随机编号，写失败不会把已受理的任务标为失败。 */
function writeStorage(key: string, value: string): void {
  try { if (value) sessionStorage.setItem(key, value); else sessionStorage.removeItem(key) }
  catch { console.warn('浏览器未允许保存引擎操作编号，服务端任务仍会继续') }
}

/** 严格验证本地暂存数据，避免把损坏内容当成新的操作参数。 */
function readPending(): PendingCommand | null {
  const value: string = readStorage(pendingStorageKey)
  if (!value || value.length > 1024) return null
  try {
    const parsed: unknown = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object' || !('key' in parsed) || !('requestId' in parsed) || !('revision' in parsed)) return null
    if (typeof parsed.key !== 'string' || parsed.key.length > 180 || typeof parsed.requestId !== 'string' || !/^[a-f0-9-]{36}$/.test(parsed.requestId) || typeof parsed.revision !== 'number' || !Number.isSafeInteger(parsed.revision) || parsed.revision < 0) return null
    return { key: parsed.key, requestId: parsed.requestId, revision: parsed.revision }
  } catch { console.warn('已忽略损坏的引擎操作编号'); return null }
}

/** 重试相同操作沿用第一次请求键，不把网络超时当作尚未执行。 */
function command(key: string, revision: number = 0): PendingCommand {
  if (pending?.key !== key) {
    pending = { key, revision, requestId: createRequestId() }
    writeStorage(pendingStorageKey, JSON.stringify(pending))
  }
  return pending
}

/** 只在服务端已确定接受或拒绝后清理本地操作键。 */
function clearPending(): void {
  pending = null
  writeStorage(pendingStorageKey, '')
}

/** 暂停当前读取，操作响应到来后再恢复，始终只有一组在途请求。 */
function stopReading(): void {
  clearTimeout(timer)
  controller?.abort()
  controller = null
}

/** 对部署快照进行有界读取，并在页面可见且没有操作时继续轮询。 */
async function refresh(): Promise<void> {
  if (disposed || document.hidden || busy.value || controller) return
  const request: AbortController = new AbortController()
  controller = request
  const timeout: ReturnType<typeof setTimeout> = setTimeout((): void => { request.abort() }, requestTimeoutMilliseconds)
  try {
    const response: EngineStatus = await api<EngineStatus>('/engines', 'GET', undefined, request.signal)
    if (disposed || request.signal.aborted || controller !== request) return
    status.value = response
    if (!initializedSelection && missing.value.length > 0) {
      selected.value = missing.value.filter((component: EngineComponent): boolean => !props.initialComponent || props.initialComponent === component.id).map((component: EngineComponent): EngineComponentId => component.id)
      initializedSelection = true
    }
    const id: string = response.activeTaskId || observedTaskId
    if (id && response.helper === 'ready') {
      if (task.value?.id !== id) { nextSequence = 0; events.value = []; eventGap.value = false }
      const snapshot: EngineTaskResponse = await api<EngineTaskResponse>(`/engines/initializations/${encodeURIComponent(id)}?afterSequence=${nextSequence}`, 'GET', undefined, request.signal)
      if (disposed || request.signal.aborted || controller !== request) return
      applyTask(snapshot)
    }
    readError.value = ''
  } catch (failure: unknown) {
    if (!disposed && controller === request) {
      if (isAbort(failure)) readError.value = '状态读取超时，任务仍在服务端继续；请稍后重新读取。'
      else if (failure instanceof ApiError && failure.status === 404 && observedTaskId && !status.value?.activeTaskId) { observedTaskId = ''; writeStorage(taskStorageKey, ''); task.value = null }
      else readError.value = errorMessage(failure)
    }
  } finally {
    clearTimeout(timeout)
    if (controller === request) { controller = null; loading.value = false; schedule() }
  }
}

/** 合并有界增量日志，任务完成仅表示部署完成。 */
function applyTask(response: EngineTaskResponse): void {
  // 初始化和启动操作都可能立即返回新任务，先隔离旧任务的事件游标。
  if (task.value?.id !== response.task.id) { nextSequence = 0; events.value = []; eventGap.value = false }
  task.value = response.task
  observedTaskId = response.task.id
  writeStorage(taskStorageKey, observedTaskId)
  if (response.reloadRequired) { events.value = []; eventGap.value = true }
  const merged: Map<number, EngineEvent> = new Map<number, EngineEvent>(events.value.map((event: EngineEvent): [number, EngineEvent] => [event.sequence, event]))
  for (const event of response.events) merged.set(event.sequence, event)
  events.value = [...merged.values()].sort((left: EngineEvent, right: EngineEvent): number => left.sequence - right.sequence).slice(-100)
  nextSequence = response.nextSequence
  const completedRevision: string = `${response.task.id}:${response.task.revision}`
  if (response.task.state === 'completed' && lastCompletedRevision !== completedRevision) {
    lastCompletedRevision = completedRevision
    emit('changed')
  }
}

/** 每轮响应处理完成后才安排下一轮，不积压查询。 */
function schedule(): void {
  clearTimeout(timer)
  if (!disposed && !document.hidden && !busy.value) timer = setTimeout((): void => { void refresh() }, taskActive.value ? activePollMilliseconds : idlePollMilliseconds)
}

/** 用户主动检查连接，不带任何浏览器提供的地址或凭据。 */
async function probe(): Promise<void> {
  await mutate<EngineStatus>('/engines/probe', {}, (response: EngineStatus): void => { status.value = response; feedback.value = '已重新检测连接与部署状态。'; emit('changed') })
}

/** 提交固定组件初始化；上游告知必须由用户主动确认。 */
async function initialize(): Promise<void> {
  if (!canInitialize.value || !status.value) return
  const components: EngineComponentId[] = [...selectedMissing.value]
  const noticeVersion: string = requiresNotice.value ? status.value.noticeVersion : ''
  const action: PendingCommand = command(`initialize:${components.join(',')}:${noticeVersion}:${acceptNotice.value}`)
  await mutate<EngineTaskResponse>('/engines/initializations', { requestId: action.requestId, components, noticeVersion, acceptNotice: requiresNotice.value && acceptNotice.value }, applyTask, true)
}

/** 仅启动同安装已停止的原容器，不通过初始化替换配置。 */
async function start(component: EngineComponent): Promise<void> {
  if (readError.value || !component.canStart || taskActive.value || status.value?.helper !== 'ready') return
  const action: PendingCommand = command(`start:${component.id}`, component.revision)
  await mutate<EngineTaskResponse>(`/engines/${component.id}/start`, { requestId: action.requestId, revision: action.revision }, applyTask, true)
}

/** 仅以原任务修订重试失败组件，已成功的部署不回滚。 */
async function retry(): Promise<void> {
  if (!canRetry.value || !task.value) return
  const action: PendingCommand = command(`retry:${task.value.id}`, task.value.revision)
  await mutate<EngineTaskResponse>(`/engines/initializations/${encodeURIComponent(task.value.id)}/retry`, { requestId: action.requestId, revision: action.revision }, applyTask, true)
}

/** 显式同步已配置来源，超过八个时等待用户选择。 */
async function syncSources(applySelection: boolean = false): Promise<void> {
  if (applySelection && (selectedSources.value.length < 1 || selectedSources.value.length > 8)) return
  await mutate<EngineSourceSync>('/engines/search/sync-sources', applySelection ? { sourceIds: selectedSources.value } : {}, (response: EngineSourceSync): void => {
    sourceSync.value = response
    selectedSources.value = [...response.selectedIds]
    feedback.value = response.message
    emit('changed')
    console.info('搜索来源同步已完成，私密配置保留在服务端')
  })
}

/** 串行提交一个有界操作，断线保留请求键而不取消持久任务。 */
async function mutate<T>(path: string, body: unknown, accept: (response: T) => void, durable: boolean = false): Promise<void> {
  if (busy.value || disposed) return
  stopReading()
  busy.value = true
  error.value = ''
  feedback.value = ''
  const request: AbortController = new AbortController()
  controller = request
  const timeout: ReturnType<typeof setTimeout> = setTimeout((): void => { request.abort() }, requestTimeoutMilliseconds)
  try {
    const response: T = await api<T>(path, 'POST', body, request.signal)
    if (disposed || request.signal.aborted || controller !== request) return
    accept(response)
    if (durable) { clearPending(); console.info('引擎管理操作已受理，可关闭面板后继续查看') }
  } catch (failure: unknown) {
    if (!disposed && controller === request) {
      error.value = isAbort(failure) ? '等待确认超时，服务端任务可能已受理；请保留当前选择并重试，系统会复用操作编号。' : errorMessage(failure)
      if (durable && failure instanceof ApiError && failure.status >= 400 && failure.status < 500) clearPending()
    }
  } finally {
    clearTimeout(timeout)
    if (controller === request) { controller = null; busy.value = false; if (!disposed) void refresh() }
  }
}

/** 页面隐藏取消状态读取；已提交的服务端操作继续执行。 */
function visibilityChanged(): void {
  if (document.hidden) { clearTimeout(timer); if (!busy.value) stopReading() }
  else if (!busy.value) void refresh()
}

onMounted((): void => { document.addEventListener('visibilitychange', visibilityChanged); void refresh() })
onBeforeUnmount((): void => { disposed = true; stopReading(); document.removeEventListener('visibilitychange', visibilityChanged) })
</script>

<template>
  <DialogShell class="engine-setup-modal" title="引擎管理" @close="emit('close')">
    <section class="engine-setup-dialog" aria-label="搜索与下载引擎初始化" :aria-busy="busy">
      <div class="engine-setup-intro"><span class="torrent-search-emblem"><Icon name="storage" :size="24" /></span><div><p class="eyebrow">给收藏一个起点</p><h3>搜索、下载，按需准备</h3><p>初始化完成后自动接入。关闭面板或刷新页面，安装任务仍会继续。</p></div></div>
      <p v-if="error" class="error-box" role="alert">{{ error }}<button class="text-button" :disabled="busy" @click="refresh">重新读取</button></p>
      <p v-if="readError" class="error-box" role="alert">{{ readError }}<button class="text-button" :disabled="busy" @click="refresh">重新读取状态</button></p>
      <p v-if="feedback" class="download-feedback" role="status"><Icon name="check" :size="17" />{{ feedback }}</p>
      <div v-if="loading && !status" class="download-inline-state" role="status"><span class="loader" />正在读取引擎状态…</div>
      <template v-if="status">
        <div class="engine-helper-status"><div><strong>{{ helperLabel }}</strong><p>{{ status.message }}</p></div><button class="secondary-button" :disabled="busy" @click="probe"><Icon name="refresh" :size="16" />{{ busy ? '处理中…' : '检测连接' }}</button></div>
        <div v-if="status.helper === 'not_enabled' || status.helper === 'unsupported'" class="engine-deployment-guide"><Icon name="info" :size="18" /><div><h4>先启用托管部署</h4><p>在 NAS 上应用项目的 <code>compose.engines.yaml</code> 覆盖配置，并设置 <code>NAS_DOWNLOAD_MODE=managed</code>、<code>NAS_TORRENT_SEARCH_MODE=managed</code>。部署步骤见 <code>docs/engines.md</code>。</p><p>首次需要预接下载目录、私有连接目录与初始化助手。网页中的安装按钮会在助手就绪并确认组件不存在后开放。</p></div></div>
        <div class="engine-component-list">
          <article v-for="component in status.components" :key="component.id" class="engine-component" :class="{ 'engine-component-ready': component.readiness === 'ready' }">
            <div class="engine-component-top"><span class="engine-card-icon"><Icon :name="component.id === 'jackett' ? 'search' : 'download'" :size="22" /></span><div><h4>{{ engineNames[component.id] }}<small>{{ component.id === 'jackett' ? 'Jackett' : 'qBittorrent' }}</small></h4><span class="engine-mode">{{ engineModeLabel(component.mode) }}</span></div><span class="status-pill" :class="{ warning: component.presence === 'conflict' || ['auth_failed', 'error', 'incompatible'].includes(component.readiness) }">{{ engineStateLabel(component) }}</span></div>
            <p class="engine-component-message">{{ component.message || '等待确认引擎连接状态。' }}</p>
            <p v-if="component.mode === 'external'" class="field-hint">此组件由外部配置管理。请修正服务端连接配置后检测连接。</p>
            <p v-if="component.mode === 'disabled'" class="field-hint">在部署配置中选择 managed 后可使用自动初始化；选择 external 可接入已有引擎。</p>
            <button v-if="component.mode === 'managed' && component.canStart" class="secondary-button" :disabled="busy || !!readError || taskActive || status.helper !== 'ready'" @click="start(component)"><Icon name="play" :size="16" />启动原引擎</button>
            <template v-if="component.id === 'jackett' && component.mode === 'managed' && component.presence === 'present' && component.runtime === 'running'">
              <div class="engine-jackett-guide"><p>先在 Jackett 设置管理员密码及索引站点，再同步来源。配置入口仅开放 NAS 回环地址，可用 SSH 本地转发：</p><code>ssh -L 19117:127.0.0.1:{{ status.setupPort || 19117 }} &lt;用户&gt;@&lt;NAS地址&gt;</code><p>然后在本机浏览器打开 <code>http://127.0.0.1:19117</code>。安装引擎不会自动创建站点账号。</p></div>
              <button class="secondary-button" :disabled="busy || taskActive" @click="syncSources()"><Icon name="refresh" :size="16" />同步来源</button>
            </template>
          </article>
        </div>
        <fieldset v-if="missing.length && !taskActive" class="engine-initialize"><legend>选择要初始化的引擎</legend><label v-for="component in missing" :key="component.id" class="engine-checkbox"><input v-model="selected" type="checkbox" :value="component.id" :disabled="busy" /><span>{{ engineNames[component.id] }}<small>{{ component.id === 'jackett' ? '拉取 Jackett，稍后配置索引来源' : '拉取 qBittorrent，准备安全下载目录' }}</small></span></label><div v-if="requiresNotice" class="engine-notice"><strong>qBittorrent 首次使用告知</strong><p>{{ status.noticeText }}</p><label class="engine-checkbox"><input v-model="acceptNotice" type="checkbox" :disabled="busy" /><span>我已阅读并接受上述首次使用告知</span></label></div><button class="primary-button" :disabled="!canInitialize" @click="initialize"><span v-if="busy" class="loader" /><Icon v-else name="download" :size="17" />{{ busy ? '正在提交…' : selectedMissing.length === 2 ? '初始化缺失引擎' : '初始化所选引擎' }}</button><p class="field-hint">只部署确认缺失的固定组件。镜像拉取速度取决于 NAS 网络，不会自动开始下载媒体。</p></fieldset>
        <section v-if="sourceSync?.selectionRequired" class="engine-source-selection" aria-label="选择搜索来源"><h4>已发现 {{ sourceSync.candidates.length }} 个来源，请选择最多 8 个</h4><p class="field-hint">每次查询的并发与结果数量有上限。选择常用来源后接入。</p><div class="engine-source-options"><label v-for="source in sourceSync.candidates" :key="source.id" class="engine-checkbox"><input v-model="selectedSources" type="checkbox" :value="source.id" :disabled="busy || (selectedSources.length >= 8 && !selectedSources.includes(source.id))" /><span>{{ source.name }}</span></label></div><button class="primary-button" :disabled="busy || !selectedSources.length || selectedSources.length > 8" @click="syncSources(true)">接入 {{ selectedSources.length }} 个来源</button></section>
      </template>
      <section v-if="task" class="engine-task" aria-label="持久初始化进度"><div class="engine-task-heading"><div><h3>{{ engineTaskLabel(task.state) }}</h3><p>{{ task.message }}</p></div><button v-if="canRetry" class="secondary-button" :disabled="busy" @click="retry"><Icon name="refresh" :size="16" />重试失败部分</button></div><p v-if="task.state === 'completed'" class="engine-task-explanation">部署步骤已完成。上方组件卡片显示应用实际连接结果；搜索引擎可能仍需配置来源。</p><ol class="engine-step-list"><li v-for="step in task.steps" :key="step.component" :class="`engine-step-${step.state}`"><span class="engine-step-marker"><Icon :name="step.state === 'completed' ? 'check' : step.state === 'failed' ? 'warning' : 'clock'" :size="17" /></span><div><h4>{{ engineNames[step.component] }}<span>{{ enginePhaseLabel(step.phase) }}</span></h4><p>{{ step.message }}</p><ul v-if="step.layers?.length" class="engine-layer-list"><li v-for="layer in step.layers" :key="layer.id"><div><code>{{ layer.id }}</code><span>{{ layer.status }}</span><small v-if="engineLayerPercent(layer) !== undefined">{{ downloadBytes(layer.current) }} / {{ downloadBytes(layer.total) }}</small></div><progress v-if="engineLayerPercent(layer) !== undefined" :value="engineLayerPercent(layer)" max="100" :aria-label="`镜像层 ${layer.id} 处理进度`" /></li></ul></div></li></ol><details v-if="events.length || eventGap" class="engine-event-details"><summary>安装记录 <span>{{ events.length }} 条近期事件</span></summary><p v-if="eventGap" class="field-hint">较早记录已过保留期，已恢复最新任务快照。</p><ol class="engine-events"><li v-for="event in events" :key="event.sequence" :class="{ 'danger-text': event.level === 'error' }"><time :datetime="event.createdAt">{{ syncTime(event.createdAt) }}</time><span>{{ event.message }}</span></li></ol></details><p class="field-hint">最近更新：{{ syncTime(task.updatedAt) }}。关闭面板仅停止页面轮询。</p></section>
      <div class="engine-setup-footer"><Icon name="lock" :size="16" /><p>连接密钥保存在服务端。已有媒体与已接入的引擎可独立于初始化助手继续工作。</p></div>
    </section>
  </DialogShell>
</template>
