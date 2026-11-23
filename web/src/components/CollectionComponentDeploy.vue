<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ApiError, api, createRequestId, errorMessage, isAbort } from '../api'
import { collectionComponentLocalEntry, collectionComponentNames, collectionComponentSetupHint, collectionComponentState, collectionComponentTaskLabel, isCollectionComponentId } from '../collection-components'
import type { CollectionComponent, CollectionComponentEvent, CollectionComponentId, CollectionComponentStatus, CollectionComponentTask, CollectionComponentTaskResponse } from '../collection-components'
import { downloadBytes, syncTime } from '../downloads'
import { engineLayerPercent, enginePhaseLabel } from '../engines'
import type { CollectionExternalService } from '../types'
import Icon from './Icon.vue'

/** 部署面板嵌入来源弹窗，不建立第二个表单或原生对话框。 */
interface Props {
  /** 用户当前选择的固定产品。 */ service: CollectionExternalService
  /** 外层来源表单正在保存时暂停操作。 */ disabled?: boolean
}

/** 仅暂存已授权意图的随机编号与固定参数，不含地址和凭据。 */
interface PendingCommand {
  /** 固定部署、启动或失败重试动作。 */ action: 'deploy' | 'start' | 'retry'
  /** 接受本次意图的固定组件。 */ component: CollectionComponentId
  /** 服务端幂等请求键，未知结果必须复用。 */ requestId: string
  /** 首次操作时看到的修订号。 */ revision: number
  /** 失败重试的原始任务编号。 */ taskId: string
  /** 首次确认时看到的固定部署模板。 */ templateVersion: string
  /** 首次确认时看到的目录授权版本。 */ noticeVersion: string
}

const props = defineProps<Props>()
const emit = defineEmits<{ connect: [component: CollectionComponent] }>()
/** 有活动任务时每两秒读一次，上一轮结束后才计时。 */
const activePollMilliseconds: number = 2000
/** 无活动任务时降低私有控制面读取频率。 */
const idlePollMilliseconds: number = 7000
/** 覆盖助手状态与任务的串行预算；超时仅停止等待，已受理部署继续。 */
const requestTimeoutMilliseconds: number = 60000
/** 本地恢复记录只允许固定长度的任务标识。 */
const taskIdPattern: RegExp = /^[a-zA-Z0-9_-]{8,80}$/
const status = ref<CollectionComponentStatus | null>(null)
const task = ref<CollectionComponentTask | null>(null)
const events = ref<CollectionComponentEvent[]>([])
const loading = ref<boolean>(true)
const busy = ref<boolean>(false)
const readError = ref<string>('')
const operationError = ref<string>('')
const acceptNotice = ref<boolean>(false)
const eventGap = ref<boolean>(false)
const pending = ref<PendingCommand | null>(null)
let observedTaskId: string = ''
let nextSequence: number = 0
let controller: AbortController | null = null
let timer: ReturnType<typeof setTimeout> | undefined
let disposed: boolean = false
const component = computed<CollectionComponent | undefined>((): CollectionComponent | undefined => status.value?.components.find((entry: CollectionComponent): boolean => entry.id === props.service))
const taskActive = computed<boolean>((): boolean => !!task.value && ['queued', 'running'].includes(task.value.state))
const otherTaskActive = computed<boolean>((): boolean => !!status.value?.activeTaskId && status.value.activeTaskId !== component.value?.latestTaskId)
const blocked = computed<boolean>((): boolean => busy.value || !!props.disabled || !!readError.value || status.value?.helper !== 'ready')
const canDeploy = computed<boolean>((): boolean => !blocked.value && !pending.value && !taskActive.value && !otherTaskActive.value && !!component.value?.canDeploy && acceptNotice.value)
const canStart = computed<boolean>((): boolean => !blocked.value && !pending.value && !taskActive.value && !otherTaskActive.value && !!component.value?.canStart)
const canRetry = computed<boolean>((): boolean => !blocked.value && !pending.value && !otherTaskActive.value && !!task.value && ['failed', 'interrupted'].includes(task.value.state))
const canConnect = computed<boolean>((): boolean => !blocked.value && !pending.value && !taskActive.value && component.value?.presence === 'present' && component.value.runtime === 'running' && component.value.readiness === 'configuration_required' && !!component.value.serviceUrl)
const localEntry = computed<string>((): string => collectionComponentLocalEntry(component.value?.setupPort || 0))
const setupHint = computed<string>((): string => isCollectionComponentId(props.service) ? collectionComponentSetupHint(props.service) : '')
const productName = computed<string>((): string => isCollectionComponentId(props.service) ? collectionComponentNames[props.service] : '资源组件')
const helperLabel = computed<string>((): string => ({ ready: '部署助手可用', unavailable: '部署助手暂不可用', unsupported: '请更新 Docker 完整部署', not_enabled: '尚未启用组件部署' } as Record<string, string>)[status.value?.helper || ''] || '正在检测部署环境')

/** 按组件隔离恢复信息，切换产品不会复用另一产品的确认或修订。 */
function storageKey(kind: 'pending' | 'task'): string { return `nas-collection-component-${props.service}-${kind}` }

/** 浏览器禁用会话存储时仅降级恢复能力，不改变持久任务结果。 */
function readStorage(kind: 'pending' | 'task'): string {
  try { return sessionStorage.getItem(storageKey(kind)) || '' }
  catch { console.warn('浏览器未允许保存组件部署编号，服务端已受理任务仍会继续'); return '' }
}

/** 仅持久化非敏感随机标识和固定部署参数。 */
function writeStorage(kind: 'pending' | 'task', value: string): void {
  try { if (value) sessionStorage.setItem(storageKey(kind), value); else sessionStorage.removeItem(storageKey(kind)) }
  catch { console.warn('组件部署恢复编号未能保存在浏览器，请在本次页面确认结果') }
}

/** 严格校验本地暂存意图，损坏参数不能成为新的部署授权。 */
function readPending(): PendingCommand | null {
  const value: string = readStorage('pending')
  if (!value || value.length > 1024) return null
  try {
    const parsed: unknown = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object') return null
    const entry = parsed as Partial<PendingCommand>
    if (!isCollectionComponentId(entry.component) || entry.component !== props.service || !['deploy', 'start', 'retry'].includes(entry.action || '') || typeof entry.requestId !== 'string' || !/^[a-f0-9-]{36}$/.test(entry.requestId) || typeof entry.revision !== 'number' || !Number.isSafeInteger(entry.revision) || entry.revision < 0 || typeof entry.taskId !== 'string' || (entry.taskId !== '' && !taskIdPattern.test(entry.taskId)) || typeof entry.templateVersion !== 'string' || entry.templateVersion.length > 80 || typeof entry.noticeVersion !== 'string' || entry.noticeVersion.length > 80) return null
    if (entry.action === 'deploy' && (!entry.templateVersion || !entry.noticeVersion)) return null
    if (entry.action !== 'deploy' && entry.revision < 1) return null
    if (entry.action === 'retry' && !entry.taskId) return null
    return entry as PendingCommand
  } catch { console.warn('已忽略损坏的组件部署恢复信息'); return null }
}

/** 已确定受理或拒绝后才清除原始请求，网络错误保留它。 */
function clearPending(): void { pending.value = null; writeStorage('pending', '') }

/** 取消上一轮读取并清理计时器，保持至多一个在途控制请求。 */
function stopReading(): void { clearTimeout(timer); controller?.abort(); controller = null }

/** 校验回执作用域后合并快照；切换任务原子清空旧日志和游标。 */
function applyTask(response: CollectionComponentTaskResponse): void {
  if (response.task.components.length !== 1 || response.task.components[0] !== props.service || !['deploy', 'start'].includes(response.task.action)) throw new Error('组件任务回执不一致，请重新检测状态')
  if (task.value?.id === response.task.id && response.task.revision < task.value.revision) return
  if (task.value?.id !== response.task.id) { events.value = []; nextSequence = 0; eventGap.value = false }
  task.value = response.task
  observedTaskId = response.task.id
  writeStorage('task', observedTaskId)
  if (response.reloadRequired) { events.value = []; eventGap.value = true }
  const merged: Map<number, CollectionComponentEvent> = new Map<number, CollectionComponentEvent>(events.value.map((entry: CollectionComponentEvent): [number, CollectionComponentEvent] => [entry.sequence, entry]))
  for (const entry of response.events) merged.set(entry.sequence, entry)
  events.value = [...merged.values()].sort((left: CollectionComponentEvent, right: CollectionComponentEvent): number => left.sequence - right.sequence).slice(-100)
  nextSequence = Math.max(nextSequence, response.nextSequence)
}

/** 每次等待本轮完成再安排下一次，不在隐藏页面积压轮询。 */
function schedule(): void {
  clearTimeout(timer)
  if (!disposed && !document.hidden && !busy.value) timer = setTimeout((): void => { void refresh() }, taskActive.value || otherTaskActive.value ? activePollMilliseconds : idlePollMilliseconds)
}

/** 仅读取 Docker 与匿名探针状态，不触发安装或创建配置卷。 */
async function refresh(): Promise<void> {
  if (disposed || document.hidden || busy.value || controller || !isCollectionComponentId(props.service)) return
  clearTimeout(timer)
  const service: CollectionExternalService = props.service
  const request: AbortController = new AbortController()
  controller = request
  const timeout: ReturnType<typeof setTimeout> = setTimeout((): void => { request.abort() }, requestTimeoutMilliseconds)
  let readingTask: boolean = false
  try {
    const result: CollectionComponentStatus = await api<CollectionComponentStatus>('/collection/components', 'GET', undefined, request.signal)
    if (disposed || request.signal.aborted || controller !== request || props.service !== service) return
    status.value = result
    const id: string = component.value?.latestTaskId || observedTaskId
    if (id && result.helper === 'ready') {
      readingTask = true
      const sequence: number = task.value?.id === id ? nextSequence : 0
      const response: CollectionComponentTaskResponse = await api<CollectionComponentTaskResponse>(`/collection/components/tasks/${encodeURIComponent(id)}?afterSequence=${sequence}`, 'GET', undefined, request.signal)
      if (disposed || request.signal.aborted || controller !== request || props.service !== service) return
      applyTask(response)
    }
    readError.value = ''
  } catch (failure: unknown) {
    if (!disposed && controller === request && props.service === service) {
      if (readingTask && failure instanceof ApiError && failure.status === 404) {
        observedTaskId = ''; writeStorage('task', ''); task.value = null; events.value = []; nextSequence = 0; eventGap.value = false
        readError.value = '较早的部署记录已不存在，当前容器状态已保留；请重新检测。'
      } else readError.value = isAbort(failure) ? '状态读取超时，已受理任务仍会继续；请重新检测。' : errorMessage(failure)
    }
  } finally {
    clearTimeout(timeout)
    if (controller === request) { controller = null; loading.value = false; schedule() }
  }
}

/** 先保存准确意图，再提交；未收到结果时禁止以新 UUID 替代原请求。 */
async function submit(action: PendingCommand['action']): Promise<void> {
  if (!component.value || !status.value || pending.value || !isCollectionComponentId(props.service)) return
  if (action === 'deploy' ? !canDeploy.value : action === 'start' ? !canStart.value : !canRetry.value) return
  pending.value = { action, component: props.service, requestId: createRequestId(), revision: action === 'retry' ? task.value?.revision || 0 : component.value.revision, taskId: action === 'retry' ? task.value?.id || '' : '', templateVersion: status.value.templateVersion, noticeVersion: status.value.noticeVersion }
  writeStorage('pending', JSON.stringify(pending.value))
  await resumePending()
}

/** 恢复同一已授权请求，服务端按原 UUID 返回持久任务而不会重复部署。 */
async function resumePending(): Promise<void> {
  if (busy.value || props.disabled || disposed || !pending.value || status.value?.helper !== 'ready') return
  const intent: PendingCommand = { ...pending.value }
  stopReading()
  busy.value = true
  operationError.value = ''
  const request: AbortController = new AbortController()
  controller = request
  const timeout: ReturnType<typeof setTimeout> = setTimeout((): void => { request.abort() }, requestTimeoutMilliseconds)
  const path: string = intent.action === 'deploy' ? '/collection/components/deploy' : intent.action === 'start' ? `/collection/components/${intent.component}/start` : `/collection/components/tasks/${encodeURIComponent(intent.taskId)}/retry`
  const body: unknown = intent.action === 'deploy' ? { requestId: intent.requestId, component: intent.component, templateVersion: intent.templateVersion, noticeVersion: intent.noticeVersion, acceptNotice: true } : { requestId: intent.requestId, revision: intent.revision }
  try {
    const response: CollectionComponentTaskResponse = await api<CollectionComponentTaskResponse>(path, 'POST', body, request.signal)
    if (disposed || request.signal.aborted || controller !== request || props.service !== intent.component) return
    applyTask(response)
    clearPending()
    acceptNotice.value = false
    console.info('资源组件操作已受理，首次配置和API连接需继续完成')
  } catch (failure: unknown) {
    if (!disposed && controller === request && props.service === intent.component) {
      operationError.value = isAbort(failure) ? '等待回执超时，部署可能已受理。请使用下方按钮确认同一请求结果。' : errorMessage(failure)
      if (failure instanceof ApiError && [400, 401, 403, 404, 409, 413].includes(failure.status)) {
        clearPending(); acceptNotice.value = false
        if (failure.status === 409) { operationError.value += ' 已保留原容器和数据；重新检测后再确认当前操作。'; readError.value = '当前修订已变化，正在重新读取部署状态。' }
        if (failure.status === 404 && intent.action === 'retry') { observedTaskId = ''; writeStorage('task', ''); task.value = null; events.value = []; nextSequence = 0 }
      }
    }
  } finally {
    clearTimeout(timeout)
    if (controller === request) { controller = null; busy.value = false; if (!disposed) void refresh() }
  }
}

/** 只有真实匿名探针通过后才预填地址，API 认证仍交给来源保存检测。 */
function connect(): void { if (canConnect.value && component.value) emit('connect', component.value) }

/** 隐藏时取消只读轮询，已提交写请求和服务端任务不被自动重发。 */
function visibilityChanged(): void {
  if (document.hidden) { clearTimeout(timer); if (!busy.value) stopReading() }
  else if (!busy.value) void refresh()
}

/** 切换服务时立即隔离旧请求、确认、日志和任务，原部署在服务端继续。 */
watch((): CollectionExternalService => props.service, (): void => {
  stopReading(); busy.value = false; status.value = null; task.value = null; events.value = []; nextSequence = 0; eventGap.value = false
  acceptNotice.value = false; readError.value = ''; operationError.value = ''; loading.value = true
  pending.value = readPending()
  const saved: string = readStorage('task')
  observedTaskId = taskIdPattern.test(saved) ? saved : ''
  void refresh()
}, { immediate: true })
onMounted((): void => { document.addEventListener('visibilitychange', visibilityChanged) })
onBeforeUnmount((): void => { disposed = true; stopReading(); document.removeEventListener('visibilitychange', visibilityChanged) })
</script>

<template>
  <section class="component-deploy" :aria-label="`${productName} 组件部署`" :aria-busy="busy || loading">
    <div class="component-deploy-heading">
      <div><strong>{{ productName }}<small v-if="component?.version">{{ component.version }}</small></strong><p class="field-hint">{{ helperLabel }}</p></div>
      <button class="secondary-button" type="button" :disabled="busy || disabled" @click="refresh"><Icon name="refresh" :size="16" />重新检测</button>
    </div>
    <p v-if="loading && !status" class="collection-loading" role="status"><span class="loader" />正在检测 Docker 部署环境…</p>
    <p v-if="readError" class="error-box" role="alert">{{ readError }}</p>
    <p v-if="operationError" class="error-box" role="alert">{{ operationError }}</p>
    <template v-if="status">
      <p v-if="status.helper !== 'ready'" class="section-note">{{ status.message }}<span class="field-hint">Windows 使用 Docker 完整模式；Linux NAS 启用托管部署的资源组件能力。也可以切回“连接已有组件”。</span></p>
      <template v-if="component">
        <div class="component-deploy-state"><span class="status-pill" :class="{ warning: component.presence === 'conflict' || component.readiness === 'error' }">{{ collectionComponentState(component) }}</span><p>{{ component.message }}</p></div>
        <p v-if="otherTaskActive" class="section-note" role="status">其他资源组件正在部署，请等待它完成后操作当前组件。</p>
        <div v-if="pending" class="component-deploy-recovery">
          <p>上次已确认的请求尚未收到确定回执。确认结果会复用原操作编号。</p>
          <button class="secondary-button" type="button" :disabled="busy || disabled || status.helper !== 'ready'" @click="resumePending">{{ busy ? '正在确认…' : '确认上次请求结果' }}</button>
        </div>
        <div v-else-if="component.canDeploy && !taskActive" class="component-deploy-confirm">
          <p>{{ status.noticeText }}</p>
          <label class="collection-check"><input v-model="acceptNotice" type="checkbox" :disabled="blocked || otherTaskActive" /><span>我确认部署此组件，并授权上述目录访问<span class="field-hint">只部署固定版本；首次账号和业务设置由你在组件内完成。</span></span></label>
          <button class="primary-button" type="button" :disabled="!canDeploy" @click="submit('deploy')"><Icon name="download" :size="17" />部署 {{ productName }}</button>
        </div>
        <button v-if="component.canStart && !pending" class="secondary-button" type="button" :disabled="!canStart" @click="submit('start')"><Icon name="play" :size="16" />启动原组件</button>
      </template>
      <section v-if="task" class="component-deploy-task" aria-label="组件持久部署进度">
        <div class="component-deploy-heading"><strong>{{ collectionComponentTaskLabel(task.state) }}</strong><button v-if="['failed', 'interrupted'].includes(task.state) && !pending" class="secondary-button" type="button" :disabled="!canRetry" @click="submit('retry')">重试失败步骤</button></div>
        <p role="status">{{ task.message }}</p>
        <ol class="component-deploy-steps"><li v-for="step in task.steps" :key="step.component"><strong>{{ enginePhaseLabel(step.phase) }}</strong><p>{{ step.message }}</p><ul v-if="step.layers?.length" class="component-deploy-layers"><li v-for="layer in step.layers" :key="layer.id"><div><code>{{ layer.id }}</code><span>{{ layer.status }}</span></div><small v-if="engineLayerPercent(layer) !== undefined">{{ downloadBytes(layer.current) }} / {{ downloadBytes(layer.total) }}</small><progress v-if="engineLayerPercent(layer) !== undefined" :value="engineLayerPercent(layer)" max="100" :aria-label="`镜像层 ${layer.id} 处理进度`" /></li></ul></li></ol>
        <details v-if="events.length || eventGap" class="component-deploy-records"><summary>部署记录 · {{ events.length }} 条</summary><p v-if="eventGap" class="field-hint">早期记录已过保留期，已恢复最新任务快照。</p><ol><li v-for="entry in events" :key="entry.sequence"><time :datetime="entry.createdAt">{{ syncTime(entry.createdAt) }}</time><span>{{ entry.message }}</span></li></ol></details>
        <p class="field-hint">最近更新：{{ syncTime(task.updatedAt) }}。关闭面板后，已受理任务仍会继续。</p>
      </section>
      <section v-if="component?.presence === 'present' && component.runtime === 'running'" class="component-deploy-setup" aria-label="组件首次配置">
        <strong>完成首次配置，再连接到平台</strong>
        <p>{{ setupHint }}</p>
        <a v-if="localEntry" class="secondary-button" :href="localEntry" target="_blank" rel="noopener noreferrer"><Icon name="external" :size="16" />打开 {{ productName }} 首次配置</a>
        <template v-else><p class="field-hint">配置入口只监听 NAS 宿主回环端口 {{ component.setupPort }}。远程访问时，请在自己的电脑建立 SSH 转发后打开下列地址。</p><code class="component-deploy-command">ssh -N -L {{ component.setupPort }}:127.0.0.1:{{ component.setupPort }} &lt;用户&gt;@&lt;NAS地址&gt;</code><code class="component-deploy-command">http://127.0.0.1:{{ component.setupPort }}/</code></template>
        <details class="component-deploy-paths"><summary>下载与整理目录说明</summary><p>现有受管下载目录在组件内为 <code>/downloads</code>。将已整理成品复制到 <code>/data/library/{{ service }}</code>，本系统从对应的只读完成目录预览并导入。</p><p>跨挂载整理请选择复制。账号、站点和下载器不会自动配置；请勿在组件中修改平台原有下载任务。</p><p v-if="localEntry">若平台通过 SSH 转发访问，还需单独转发本组件的 {{ component.setupPort }} 端口。</p></details>
        <button class="primary-button" type="button" :disabled="!canConnect" @click="connect"><Icon name="check" :size="17" />已完成首次配置，填写 API 凭据</button>
        <p class="field-hint">部署完成和容器运行不代表已通过 API 认证。填写凭据并保存检测后，平台才开放对应功能。</p>
      </section>
    </template>
  </section>
</template>

<style scoped>
.component-deploy { display: flex; flex-direction: column; gap: 16px; min-width: 0; padding: 18px; border: 1px solid var(--border); border-radius: 10px; background: var(--surface-subtle); }
.component-deploy p { line-height: 1.8; overflow-wrap: anywhere; }
.component-deploy-heading { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; min-width: 0; }
.component-deploy-heading > div { flex: 1; min-width: 150px; }
.component-deploy-heading strong { font-size: 14px; overflow-wrap: anywhere; }
.component-deploy-heading small { color: var(--muted); font-size: 11px; font-weight: 400; margin-left: 9px; }
.component-deploy-state { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; font-size: 12px; }
.component-deploy-state .status-pill { white-space: normal; max-width: 100%; }
.component-deploy-confirm, .component-deploy-recovery, .component-deploy-setup, .component-deploy-task { display: flex; flex-direction: column; gap: 13px; min-width: 0; font-size: 12px; }
.component-deploy-recovery, .component-deploy-task, .component-deploy-setup { border-top: 1px solid var(--border); padding-top: 16px; }
.component-deploy :is(.primary-button, .secondary-button) { align-self: flex-start; min-height: 44px; max-width: 100%; white-space: normal; text-align: left; }
.component-deploy-steps { display: flex; flex-direction: column; gap: 12px; margin: 0; padding-left: 20px; min-width: 0; }
.component-deploy-steps strong { color: var(--accent); font-size: 12px; }
.component-deploy-layers { list-style: none; display: flex; flex-direction: column; gap: 12px; padding: 10px 0 0; max-height: 240px; overflow-y: auto; min-width: 0; }
.component-deploy-layers li { min-width: 0; }
.component-deploy-layers div { display: flex; justify-content: space-between; gap: 8px; flex-wrap: wrap; }
.component-deploy-layers small { color: var(--muted); display: block; margin-top: 5px; }
.component-deploy-layers progress { display: block; width: 100%; height: 7px; margin-top: 7px; accent-color: var(--accent); }
.component-deploy-records summary, .component-deploy-paths summary { cursor: pointer; min-height: 44px; display: flex; align-items: center; color: var(--accent); font-weight: 500; }
.component-deploy-records ol { margin: 8px 0 0; padding-left: 20px; max-height: 230px; overflow-y: auto; }
.component-deploy-records li { padding-bottom: 12px; overflow-wrap: anywhere; }
.component-deploy-records time { color: var(--muted); display: block; margin-bottom: 4px; font-size: 11px; }
.component-deploy-command { display: block; padding: 10px 12px; background: var(--surface); border: 1px solid var(--border); border-radius: 7px; white-space: normal; overflow-wrap: anywhere; }
.component-deploy-paths p + p { margin-top: 8px; }
@media (max-width: 600px) { .component-deploy { padding: 14px; } .component-deploy-heading > div { min-width: 110px; } .component-deploy-setup .primary-button { width: 100%; justify-content: center; } }
</style>
