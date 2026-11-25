<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, errorMessage, isAbort } from '../api'
import { collectionDecisionLabel, collectionExternalUrl, collectionImportLabel, collectionKindIcons, collectionKindLabels, collectionPageSize, collectionStateLabel, collectionTime } from '../collection'
import { formatSize, typeLabels } from '../format'
import type { CollectionCandidate, CollectionCandidatePage, CollectionCapabilities, CollectionKind, CollectionSource, CollectionSourceInput, CollectionSubscription, CollectionSubscriptionInput, CollectionTask, CollectionTaskPage, Item, Library } from '../types'
import CollectionAcquireDialog from './CollectionAcquireDialog.vue'
import CollectionPreviewDialog from './CollectionPreviewDialog.vue'
import CollectionJMComicDialog from './CollectionJMComicDialog.vue'
import CollectionUAVDialog from './CollectionUAVDialog.vue'
import CollectionSourceDialog from './CollectionSourceDialog.vue'
import CollectionSubscriptionDialog from './CollectionSubscriptionDialog.vue'
import CollectionTaskDialog from './CollectionTaskDialog.vue'
import CollectionExternalPanel from './CollectionExternalPanel.vue'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'
import './collection.css'

/** 收集中心保留五个业务分区。 */
type CollectionTab = 'overview' | 'sources' | 'subscriptions' | 'candidates' | 'tasks'
/** 元信息和分页查询的错误互不覆盖。 */
interface QueryErrors {
  /** 平台能力查询错误。 */ capabilities: string
  /** 来源查询错误。 */ sources: string
  /** 订阅查询错误。 */ subscriptions: string
  /** 候选查询错误。 */ candidates: string
  /** 任务查询错误。 */ tasks: string
  /** 目标媒体库查询错误。 */ libraries: string
}
/** 规则试运行仅显示结果，不创建任务。 */
interface RulePreview {
  /** 试运行规则名称。 */ name: string
  /** 已匹配的数量。 */ matched: number
  /** 有界候选快照。 */ candidates: CollectionCandidate[]
}
const emit = defineEmits<{ 'open-item': [item: Item]; 'open-downloads': [id?: string] }>()
/** 仅在前台按完成后调度，正常轮询间隔六秒。 */
const pollingMilliseconds: number = 6000
/** 查询失败时减速，保留手动重试。 */
const offlinePollingMilliseconds: number = 15000
const tabs: { /** 分区编号。 */ id: CollectionTab; /** 中文标题。 */ label: string }[] = [{ id: 'overview', label: '概览' }, { id: 'sources', label: '来源' }, { id: 'subscriptions', label: '订阅' }, { id: 'candidates', label: '候选' }, { id: 'tasks', label: '任务' }]
const tab = ref<CollectionTab>('overview')
const capabilities = ref<CollectionCapabilities | null>(null)
const sources = ref<CollectionSource[]>([])
const subscriptions = ref<CollectionSubscription[]>([])
const candidates = ref<CollectionCandidate[]>([])
const tasks = ref<CollectionTask[]>([])
const libraries = ref<Library[]>([])
const candidateTotal = ref<number>(0)
const taskTotal = ref<number>(0)
const candidatePage = ref<number>(1)
const taskPage = ref<number>(1)
const candidateSource = ref<string>('')
const taskState = ref<string>('')
const loading = ref<boolean>(true)
const errors = ref<QueryErrors>({ capabilities: '', sources: '', subscriptions: '', candidates: '', tasks: '', libraries: '' })
const actionError = ref<string>('')
const feedback = ref<string>('')
const busyId = ref<string>('')
const sourceDialog = ref<boolean>(false)
const editingSource = ref<CollectionSource | null>(null)
const initialKind = ref<CollectionKind>('rss')
const subscriptionDialog = ref<boolean>(false)
const editingSubscription = ref<CollectionSubscription | null>(null)
const previewDialog = ref<boolean>(false)
const jmcomicDialog = ref<boolean>(false)
const jmcomicSourceId = ref<string>('')
const uavDialog = ref<boolean>(false)
const uavSourceId = ref<string>('')
const previewSourceId = ref<string>('')
const previewKind = ref<CollectionKind>('web')
const acquiring = ref<CollectionCandidate | null>(null)
const selectedTask = ref<CollectionTask | null>(null)
const externalSource = ref<CollectionSource | null>(null)
const backfillSource = ref<CollectionSource | null>(null)
const backfillConsent = ref<boolean>(false)
const rulePreview = ref<RulePreview | null>(null)
const lastSyncedAt = ref<string>('')
const actionControllers: Set<AbortController> = new Set<AbortController>()
let controller: AbortController | null = null
let pollTimer: ReturnType<typeof setTimeout> | undefined
let disposed: boolean = false
const currentError = computed<string>((): string => tab.value === 'overview' ? '' : errors.value[tab.value])
const reviewCount = computed<number>((): number => candidates.value.filter((candidate: CollectionCandidate): boolean => ['review', 'pending_review', 'matched'].includes(candidate.decision) && !candidate.taskId).length)
const enabledSubscriptions = computed<number>((): number => subscriptions.value.filter((subscription: CollectionSubscription): boolean => subscription.enabled).length)
const enabledSources = computed<number>((): number => sources.value.filter((source: CollectionSource): boolean => source.enabled).length)
const modalActive = computed<boolean>((): boolean => sourceDialog.value || subscriptionDialog.value || previewDialog.value || jmcomicDialog.value || uavDialog.value || !!acquiring.value || !!selectedTask.value || !!externalSource.value || !!backfillSource.value || !!rulePreview.value)

/** 显示稳定来源名称，删除或不可见的来源保留中性说明。 */
function sourceName(id: string): string { return sources.value.find((source: CollectionSource): boolean => source.id === id)?.name || '来源记录' }
/** 将适配器能力转为中文说明。 */
function capabilityName(kind: string): string {
  const labels: Record<string, string> = { rss: 'RSS 收取', bt: 'BT 下载桥接', web: '网页音视频', local: '本地归档复制', remote: '远程复制', register: '本地只读登记', local_register: '本地只读登记', fetch_web_media: '网页音视频', copy_remote_files: '远程复制', copy_local_files: '本地归档复制', external: '外部成品导入' }
  return kind === 'jmcomic' ? 'JMComic 漫画搜索下载' : kind === 'uav' ? 'UAV 视频搜索下载' : labels[kind] || kind
}
/** 只在前台且本次请求结束后安排下一轮。 */
function schedule(): void {
  clearTimeout(pollTimer)
  if (!disposed && !document.hidden) pollTimer = setTimeout((): void => { void refresh(true) }, Object.values(errors.value).some((error: string): boolean => !!error) ? offlinePollingMilliseconds : pollingMilliseconds)
}
/** 各元数据查询和分页结果分别提交，任何成功都不能抹掉其他失败。 */
async function refresh(silent: boolean = false): Promise<void> {
  if (disposed || document.hidden || controller) return
  clearTimeout(pollTimer)
  const request: AbortController = new AbortController()
  controller = request
  if (!silent) loading.value = true
  const candidateParams: URLSearchParams = new URLSearchParams({ sourceId: candidateSource.value, page: String(candidatePage.value), pageSize: String(collectionPageSize) })
  const taskParams: URLSearchParams = new URLSearchParams({ state: taskState.value, page: String(taskPage.value), pageSize: String(collectionPageSize) })
  try {
    const [capabilityResult, sourceResult, subscriptionResult, candidateResult, taskResult, libraryResult] = await Promise.allSettled([
      api<CollectionCapabilities>('/collection/capabilities', 'GET', undefined, request.signal),
      api<{ sources: CollectionSource[] }>('/collection/sources', 'GET', undefined, request.signal),
      api<{ subscriptions: CollectionSubscription[] }>('/collection/subscriptions', 'GET', undefined, request.signal),
      api<CollectionCandidatePage>(`/collection/candidates?${candidateParams}`, 'GET', undefined, request.signal),
      api<CollectionTaskPage>(`/collection/tasks?${taskParams}`, 'GET', undefined, request.signal),
      api<{ libraries: Library[] }>('/libraries', 'GET', undefined, request.signal),
    ])
    if (request.signal.aborted || controller !== request || disposed) return
    if (capabilityResult.status === 'fulfilled') { capabilities.value = capabilityResult.value; errors.value.capabilities = '' }
    else if (!isAbort(capabilityResult.reason)) errors.value.capabilities = errorMessage(capabilityResult.reason)
    if (sourceResult.status === 'fulfilled') { sources.value = sourceResult.value.sources; errors.value.sources = '' }
    else if (!isAbort(sourceResult.reason)) errors.value.sources = errorMessage(sourceResult.reason)
    if (subscriptionResult.status === 'fulfilled') { subscriptions.value = subscriptionResult.value.subscriptions; errors.value.subscriptions = '' }
    else if (!isAbort(subscriptionResult.reason)) errors.value.subscriptions = errorMessage(subscriptionResult.reason)
    if (candidateResult.status === 'fulfilled') {
      candidates.value = candidateResult.value.candidates; candidateTotal.value = candidateResult.value.total; errors.value.candidates = ''
      if (candidatePage.value > 1 && candidateTotal.value <= (candidatePage.value - 1) * collectionPageSize) candidatePage.value = Math.max(1, Math.ceil(candidateTotal.value / collectionPageSize))
    } else if (!isAbort(candidateResult.reason)) errors.value.candidates = errorMessage(candidateResult.reason)
    if (taskResult.status === 'fulfilled') {
      // 详情操作可能比列表快照更新，保留已经收到的更高修订版本。
      const currentTasks: Map<string, CollectionTask> = new Map(tasks.value.map((task: CollectionTask): [string, CollectionTask] => [task.id, task]))
      tasks.value = taskResult.value.tasks.map((task: CollectionTask): CollectionTask => { const current: CollectionTask | undefined = currentTasks.get(task.id); return current && current.revision > task.revision ? current : task })
      taskTotal.value = taskResult.value.total; errors.value.tasks = ''; lastSyncedAt.value = new Date().toISOString()
      if (taskPage.value > 1 && taskTotal.value <= (taskPage.value - 1) * collectionPageSize) taskPage.value = Math.max(1, Math.ceil(taskTotal.value / collectionPageSize))
    } else if (!isAbort(taskResult.reason)) errors.value.tasks = errorMessage(taskResult.reason)
    if (libraryResult.status === 'fulfilled') { libraries.value = libraryResult.value.libraries; errors.value.libraries = '' }
    else if (!isAbort(libraryResult.reason)) errors.value.libraries = errorMessage(libraryResult.reason)
  } finally { if (controller === request) { controller = null; loading.value = false; schedule() } }
}
/** 筛选或操作变动时取消过时请求，并只接受新快照。 */
function restart(silent: boolean = false): void { controller?.abort(); controller = null; clearTimeout(pollTimer); void refresh(silent) }
/** 页面隐藏时停止查询，恢复后继续读取持久状态。 */
function visibilityChanged(): void { if (document.hidden) { clearTimeout(pollTimer); controller?.abort(); controller = null } else restart(true) }
/** 新建来源使用指定类型，编辑时保留服务器修订号。 */
function openSource(source: CollectionSource | null = null, kind: CollectionKind = 'rss'): void { if (busyId.value || modalActive.value) return; editingSource.value = source; initialKind.value = kind; sourceDialog.value = true }
/** 来源保存后读取实际健康信息，不将保存等同连接成功。 */
async function sourceSaved(source: CollectionSource): Promise<void> {
  sourceDialog.value = false
  editingSource.value = null
  sources.value = [...sources.value.filter((current: CollectionSource): boolean => current.id !== source.id), source]
  feedback.value = source.apiConfigured ? '连接配置已保存，接下来检测实际服务。' : '来源已保存，可单独测试连接；RSS 首次收取会建立历史基线。'
  restart(true)
  if (source.kind === 'external' && source.apiConfigured) { await nextTick(); if (!disposed) openExternal(source) }
  if (source.kind === 'jmcomic' && source.enabled) { await nextTick(); if (!disposed) openJMComic(source) }
  if (source.kind === 'uav' && source.enabled) { await nextTick(); if (!disposed) openUAV(source) }
}
/** 外部管理只打开一个原生对话框，打开时不会创建获取动作。 */
function openExternal(source: CollectionSource): void { if (!busyId.value && !modalActive.value) externalSource.value = source }
/** 先卸载管理面板，再进入现有只读预览链路。 */
async function importExternal(): Promise<void> {
  const source: CollectionSource | null = externalSource.value
  externalSource.value = null
  await nextTick()
  if (!disposed && source) openPreview(source)
}
/** 先关闭管理面板再编辑，避免嵌套原生对话框。 */
async function editExternal(): Promise<void> {
  const source: CollectionSource | null = externalSource.value
  externalSource.value = null
  await nextTick()
  if (!disposed && source) openSource(source)
}
/** 打开订阅表单。 */
function openSubscription(subscription: CollectionSubscription | null = null): void { if (busyId.value || modalActive.value) return; editingSubscription.value = subscription; subscriptionDialog.value = true }
/** 保存规则后回到列表，建议明确试运行。 */
function subscriptionSaved(): void { subscriptionDialog.value = false; editingSubscription.value = null; feedback.value = '规则已保存，可以试运行查看命中原因。'; restart(true) }
/** 打开固定来源或指定类型的逐项预览。 */
function openPreview(source: CollectionSource | null = null, kind: CollectionKind = 'web'): void { if (source?.kind === 'jmcomic' || kind === 'jmcomic') { openJMComic(source); return }; if (source?.kind === 'uav' || kind === 'uav') { openUAV(source); return }; if (busyId.value || modalActive.value) return; previewSourceId.value = source?.id || ''; previewKind.value = source?.kind || kind; previewDialog.value = true }
/** 已有漫画来源直接打开搜索；首次使用先完成来源配置。 */
function openJMComic(source: CollectionSource | null = null): void {
  if (busyId.value || modalActive.value || errors.value.sources) return
  const selected: CollectionSource | undefined = source || sources.value.find((entry: CollectionSource): boolean => entry.kind === 'jmcomic' && entry.enabled)
  if (!selected) { openSource(null, 'jmcomic'); return }
  jmcomicSourceId.value = selected.id
  jmcomicDialog.value = true
}
/** 已有视频插件来源直接打开搜索，首次使用先完成来源配置。 */
function openUAV(source: CollectionSource | null = null): void {
  if (busyId.value || modalActive.value || errors.value.sources) return
  const selected: CollectionSource | undefined = source || sources.value.find((entry: CollectionSource): boolean => entry.kind === 'uav' && entry.enabled)
  if (!selected) { openSource(null, 'uav'); return }
  uavSourceId.value = selected.id
  uavDialog.value = true
}
/** 异步试运行期间禁止再开其他弹窗，避免响应到达时嵌套原生对话框。 */
function openTask(task: CollectionTask): void { if (!busyId.value && !modalActive.value) selectedTask.value = task }
/** 人工获取保留当前候选版本。 */
function openAcquire(candidate: CollectionCandidate): void { if (!busyId.value && !modalActive.value) acquiring.value = candidate }
/** 历史回填始终重新取得明确的本次选择。 */
function openBackfill(source: CollectionSource): void { if (busyId.value || modalActive.value) return; actionError.value = ''; backfillSource.value = source; backfillConsent.value = false }
/** 所有变更统一加锁并保留独立反馈，日志不包含来源地址。 */
async function mutate<T>(id: string, path: string, body: unknown, method: string = 'POST'): Promise<T | null> {
  if (busyId.value) return null
  busyId.value = id
  actionError.value = ''
  feedback.value = ''
  const request: AbortController = new AbortController()
  actionControllers.add(request)
  try {
    const result: T = await api<T>(path, method, body, request.signal)
    if (request.signal.aborted || disposed) return null
    if (method !== 'GET') console.info('资源收集操作已确认')
    restart(true)
    return result
  } catch (failure: unknown) { if (!isAbort(failure)) actionError.value = errorMessage(failure); return null }
  finally { actionControllers.delete(request); busyId.value = '' }
}
/** 测试仅验证连接，不收取或获取资源。 */
async function testSource(source: CollectionSource): Promise<void> {
  const result = await mutate<{ ok: boolean; message: string }>(source.id, `/collection/sources/${encodeURIComponent(source.id)}/test`, {})
  if (result) { if (result.ok) feedback.value = result.message; else actionError.value = result.message }
}
/** RSS 普通收取与明确历史回填使用不同意图。 */
async function refreshSource(source: CollectionSource, backfill: boolean = false): Promise<void> {
  const result = await mutate<{ added: number; message: string }>(source.id, `/collection/sources/${encodeURIComponent(source.id)}/refresh`, { backfill })
  if (result) { feedback.value = result.message || `收取完成，新增 ${result.added} 条。`; backfillSource.value = null }
}
/** 来源停用只停止新意图，完整公开配置保留不含 URL。 */
async function toggleSource(source: CollectionSource): Promise<void> {
  const body: CollectionSourceInput = { kind: source.kind, name: source.name, enabled: !source.enabled, rootId: source.rootId, path: source.path, allowPrivate: source.allowPrivate, refreshMinutes: source.refreshMinutes, outputProfile: source.outputProfile, externalUrl: source.externalUrl, externalService: source.externalService || '', revision: source.revision }
  if (await mutate(source.id, `/collection/sources/${encodeURIComponent(source.id)}`, body, 'PATCH')) feedback.value = source.enabled ? '来源已停用，历史任务与媒体保留。' : '来源已启用。'
}
/** 暂停和恢复订阅保留原有规则及目标。 */
async function toggleSubscription(subscription: CollectionSubscription): Promise<void> {
  const body: CollectionSubscriptionInput = { sourceId: subscription.sourceId, name: subscription.name, enabled: !subscription.enabled, contentType: subscription.contentType, mode: subscription.mode, rule: subscription.rule, createLibrary: subscription.createLibrary, libraryId: subscription.libraryId, selectionPattern: subscription.selectionPattern, revision: subscription.revision }
  if (await mutate(subscription.id, `/collection/subscriptions/${encodeURIComponent(subscription.id)}`, body, 'PATCH')) feedback.value = subscription.enabled ? '订阅已暂停。' : '订阅已恢复。'
}
/** 规则试运行仅读取命中快照。 */
async function previewSubscription(subscription: CollectionSubscription): Promise<void> {
  const result = await mutate<{ candidates: CollectionCandidate[]; matched: number }>(subscription.id, `/collection/subscriptions/${encodeURIComponent(subscription.id)}/preview`, {})
  if (result) rulePreview.value = { name: subscription.name, matched: result.matched, candidates: result.candidates }
}
/** 手动补查仍遵守订阅授权方式和有界数量。 */
async function runSubscription(subscription: CollectionSubscription): Promise<void> {
  const result = await mutate<{ added: number; message: string }>(subscription.id, `/collection/subscriptions/${encodeURIComponent(subscription.id)}/run`, {})
  if (result) feedback.value = result.message || `补查完成，新增 ${result.added} 条。`
}
/** 人工忽略携带候选版本，避免覆盖已获取的决定。 */
async function ignoreCandidate(candidate: CollectionCandidate): Promise<void> {
  if (await mutate(candidate.id, `/collection/candidates/${encodeURIComponent(candidate.id)}/ignore`, { revision: candidate.revision })) feedback.value = '候选已忽略。'
}
/** 已有任务不重复获取；历史基线必须先明确回填。 */
function canAcquire(candidate: CollectionCandidate): boolean { return !candidate.taskId && ['review', 'pending_review', 'matched'].includes(candidate.decision) }
/** 候选任务引用通过后端读取后再打开，独立显示读取失败。 */
async function openTaskId(id: string): Promise<void> {
  if (busyId.value) return
  const result = await mutate<{ task: CollectionTask }>(id, `/collection/tasks/${encodeURIComponent(id)}`, undefined, 'GET')
  if (result) selectedTask.value = result.task
}
/** 创建成功后先卸载预览或确认弹窗，再显示任务详情。 */
async function taskCreated(task: CollectionTask): Promise<void> {
  acquiring.value = null
  previewDialog.value = false
  jmcomicDialog.value = false
  uavDialog.value = false
  await nextTick()
  if (disposed) return
  selectedTask.value = task
  feedback.value = '收集任务已受理，获取与入库将分别更新。'
  restart(true)
}
/** 详情刷新后将较新版本同步到当前列表。 */
function taskUpdated(task: CollectionTask): void { tasks.value = tasks.value.map((current: CollectionTask): CollectionTask => current.id === task.id && task.revision >= current.revision ? task : current) }
/** 媒体详情沿用应用生命周期，确保不嵌套原生对话框。 */
async function openItem(item: Item): Promise<void> { selectedTask.value = null; await nextTick(); if (!disposed) emit('open-item', item) }
/** 下载详情交给现有下载中心，并携带真实下载引用。 */
async function openDownload(id: string): Promise<void> { selectedTask.value = null; await nextTick(); if (!disposed) emit('open-downloads', id) }
watch([candidatePage, taskPage], (): void => { restart() })
watch(candidateSource, (): void => { candidatePage.value = 1; restart() })
watch(taskState, (): void => { taskPage.value = 1; restart() })
onMounted((): void => { document.addEventListener('visibilitychange', visibilityChanged); void refresh() })
onBeforeUnmount((): void => { disposed = true; clearTimeout(pollTimer); controller?.abort(); for (const request of actionControllers) request.abort(); document.removeEventListener('visibilitychange', visibilityChanged) })
</script>

<template>
  <section class="collection-center" aria-label="资源收集中心">
    <div class="collection-toolbar"><div><h2>从来源到媒体库</h2><p class="field-hint">统一管理发现、获取、校验和扫描。</p></div><div class="collection-actions"><button class="secondary-button" @click="emit('open-downloads')"><Icon name="download" />BT 搜索与下载</button><button class="secondary-button" :disabled="loading || !!errors.sources || !!errors.libraries" @click="openJMComic()"><Icon name="comic" />漫画搜索下载</button><button class="secondary-button" :disabled="loading || !!errors.sources || !!errors.libraries" @click="openUAV()"><Icon name="video" />视频搜索下载</button><button class="primary-button" :disabled="loading || !!errors.sources" @click="openPreview()"><Icon name="plus" />网页与文件</button><button class="icon-button" :disabled="loading" aria-label="刷新收集状态" @click="restart()"><Icon name="refresh" /></button></div></div>
    <nav class="collection-tabs" aria-label="收集分区"><button v-for="entry in tabs" :key="entry.id" :class="{ active: tab === entry.id }" :aria-current="tab === entry.id ? 'page' : undefined" @click="tab = entry.id">{{ entry.label }}</button></nav>
    <p v-if="feedback" class="collection-feedback" role="status">{{ feedback }}</p>
    <p v-if="actionError" class="error-box" role="alert">{{ actionError }} <button class="text-button" @click="restart(true)">重新读取状态</button></p>
    <p v-if="errors.libraries" class="error-box" role="alert">目标媒体库暂不可用：{{ errors.libraries }} <button class="text-button" @click="restart(true)">重试</button></p>
    <p v-if="currentError" class="error-box" role="alert">{{ currentError }}{{ lastSyncedAt ? ' 当前保留上次读取的记录。' : '' }} <button class="text-button" @click="restart()">重新读取</button></p>
    <p v-if="loading" class="collection-loading" role="status"><span class="loader"></span>正在读取收集状态…</p>
    <template v-if="tab === 'overview'">
      <div class="collection-stats"><button @click="tab = 'sources'"><span>已启用来源</span><strong>{{ errors.sources ? '—' : enabledSources }}</strong><small>{{ errors.sources || `已配置 ${sources.length} 个来源` }}</small></button><button @click="tab = 'subscriptions'"><span>定时订阅</span><strong>{{ errors.subscriptions ? '—' : enabledSubscriptions }}</strong><small>{{ errors.subscriptions || '启用规则独立调度' }}</small></button><button @click="tab = 'candidates'"><span>当前页待确认</span><strong>{{ errors.candidates ? '—' : reviewCount }}</strong><small>{{ errors.candidates || `当前筛选共 ${candidateTotal} 条候选` }}</small></button><button @click="tab = 'tasks'"><span>收集任务</span><strong>{{ errors.tasks ? '—' : taskTotal }}</strong><small>{{ errors.tasks || '按实际任务保存完整历史' }}</small></button></div>
      <div class="collection-section-heading"><h3>收集入口</h3><button class="text-button" @click="tab = 'sources'">管理来源<Icon name="arrow" :size="16" /></button></div>
      <div class="collection-entry-grid"><button :disabled="!!errors.sources || !!errors.libraries" @click="openUAV()"><Icon name="video" /><strong>UAV 视频插件</strong><span>选择固定站点搜索视频，预览后下载并导入视频库。</span></button><button :disabled="!!errors.sources || !!errors.libraries" @click="openJMComic()"><Icon name="comic" /><strong>JMComic 漫画</strong><span>搜索或输入漫画编号，确认下载后导入漫画库。</span></button><button @click="tab = 'subscriptions'"><Icon name="rss" /><strong>影视与动漫 RSS</strong><span>规则试运行、候选确认，再复用 BT 选文件。</span></button><button @click="openPreview(null, 'web')"><Icon name="link" /><strong>网页视频与音频</strong><span>逐条预览页面，保存视频或提取音频。</span></button><button @click="openPreview(null, 'local')"><Icon name="music" /><strong>本地文件与自有音乐</strong><span>默认只读登记，需要副本时另选归档复制。</span></button><button @click="openPreview(null, 'remote')"><Icon name="storage" /><strong>远程与外部成品</strong><span>使用预配置连接复制，保留来源文件。</span></button></div>
      <div class="collection-section-heading"><h3>平台与执行能力</h3><button class="text-button" @click="restart(true)">重新检测</button></div>
      <p v-if="errors.capabilities" class="error-box" role="alert">能力查询失败：{{ errors.capabilities }}</p>
      <template v-else-if="capabilities"><p class="section-note">{{ capabilities.workerReady ? '获取执行端已连接。' : '获取执行端暂不可用。' }} {{ capabilities.workerReason }} 本地只读登记与 BT 使用各自能力。</p><div class="collection-capabilities"><article v-for="capability in capabilities.capabilities" :key="capability.kind"><div><strong>{{ capabilityName(capability.kind) }}</strong><span class="status-pill" :class="{ 'collection-unavailable': !capability.available }">{{ capability.available ? '可用' : '不可用' }}</span></div><p class="field-hint">{{ capability.reason || (capability.available ? '已通过能力检测' : '需要配置相应执行工具') }}{{ capability.version ? ` · ${capability.version}` : '' }}</p></article></div></template>
      <div class="collection-section-heading"><h3>最近任务</h3><button class="text-button" @click="tab = 'tasks'">查看任务<Icon name="arrow" :size="16" /></button></div>
      <p v-if="errors.tasks" class="error-box" role="alert">任务暂不可用：{{ errors.tasks }}</p>
      <div v-else-if="tasks.length" class="collection-records"><button v-for="task in tasks.slice(0, 5)" :key="task.id" class="collection-task-row" @click="openTask(task)"><Icon :name="task.contentType" /><div><strong>{{ task.title }}</strong><p class="field-hint">{{ collectionStateLabel(task.state) }} · {{ collectionImportLabel(task.importState) }}</p></div><Icon name="chevron" /></button></div>
      <div v-else-if="!loading" class="collection-empty"><Icon name="collection" :size="32" /><h3>开始你的第一次收集</h3><p>先配置已有来源，然后预览一条网页或一个目录。</p><button class="secondary-button" @click="openSource()">添加来源</button></div>
    </template>
    <template v-else-if="tab === 'sources'">
      <div class="collection-section-heading"><div><h3>来源配置</h3><p class="field-hint">连接可用、工具就绪与资源可获取分别判断。秘密和私密地址不会回显。</p></div><button class="primary-button" :disabled="!!errors.capabilities" @click="openSource()"><Icon name="plus" />添加来源</button></div>
      <div v-if="sources.length" class="collection-source-grid"><article v-for="source in sources" :key="source.id" class="collection-card"><div class="collection-section-heading"><div class="collection-card-title"><Icon :name="collectionKindIcons[source.kind]" /><h3>{{ source.name }}</h3></div><span class="status-pill">{{ source.enabled ? '已启用' : '已停用' }}</span></div><p class="field-hint">{{ collectionKindLabels[source.kind] }} · 最近成功：{{ collectionTime(source.lastSuccessAt) }}</p><p v-if="source.path" class="collection-path">相对目录：{{ source.path }}</p><p v-if="source.lastError" class="collection-warning">{{ source.lastError }}</p><p v-else class="field-hint">{{ source.lastSuccessAt ? '最近操作已成功，具体资源需单独预览。' : '尚无成功记录，可先测试连接。' }}</p><div class="collection-actions"><button v-if="source.kind === 'external' && source.apiConfigured" class="primary-button" :disabled="!!busyId" @click="openExternal(source)">管理</button><button v-else class="text-button" :disabled="!!busyId" @click="testSource(source)">{{ busyId === source.id ? '处理中…' : '测试连接' }}</button><button v-if="source.kind === 'rss'" class="text-button" :disabled="!!busyId || !source.enabled" @click="refreshSource(source)">收取新条目</button><button v-if="source.kind === 'rss'" class="text-button" :disabled="!!busyId || !source.enabled" @click="openBackfill(source)">历史回填</button><button v-else-if="source.kind !== 'external' || !source.apiConfigured" class="text-button" :disabled="!source.enabled || !!errors.libraries || (source.kind === 'external' && !source.rootId)" @click="openPreview(source)">{{ source.kind === 'jmcomic' || source.kind === 'uav' ? '搜索与下载' : '预览导入' }}</button><a v-if="source.kind === 'external' && collectionExternalUrl(source.externalUrl)" class="text-button" :href="collectionExternalUrl(source.externalUrl)" target="_blank" rel="noopener noreferrer">进入外部应用<Icon name="external" :size="14" /></a><button class="text-button" @click="openSource(source)">编辑</button><button class="text-button" :disabled="!!busyId" @click="toggleSource(source)">{{ source.enabled ? '停用' : '启用' }}</button></div></article></div>
      <div v-else-if="!loading && !errors.sources" class="collection-empty"><Icon name="folder" :size="32" /><h3>还没有收集来源</h3><p>可以添加 RSS、网页、本地目录、远程连接或外部成品目录。</p><button class="secondary-button" @click="openSource(null, 'local')">添加本地目录</button></div>
    </template>
    <template v-else-if="tab === 'subscriptions'">
      <div class="collection-section-heading"><div><h3>RSS 订阅规则</h3><p class="field-hint">默认待确认；自动规则需要明确授权和唯一文件选择。</p></div><button class="primary-button" :disabled="!!errors.sources || !!errors.libraries" @click="openSubscription()"><Icon name="plus" />添加订阅</button></div>
      <div v-if="subscriptions.length" class="collection-records"><article v-for="subscription in subscriptions" :key="subscription.id" class="collection-card"><div class="collection-section-heading"><h3>{{ subscription.name }}</h3><span class="status-pill">{{ subscription.enabled ? '已启用' : '已暂停' }}</span></div><p class="field-hint">{{ sourceName(subscription.sourceId) }} · {{ typeLabels[subscription.contentType] }} · {{ subscription.mode === 'automatic' ? '已授权自动获取' : '人工确认' }}</p><p class="collection-path">包含：{{ subscription.rule.include || '不设关键词' }}<template v-if="subscription.rule.exclude"> · 排除：{{ subscription.rule.exclude }}</template></p><p class="field-hint">下次执行：{{ collectionTime(subscription.nextRunAt) }}</p><div class="collection-actions"><button class="text-button" :disabled="!!busyId" @click="previewSubscription(subscription)">试运行</button><button class="text-button" :disabled="!!busyId || !subscription.enabled" @click="runSubscription(subscription)">手工补查</button><button class="text-button" :disabled="!!errors.sources || !!errors.libraries" @click="openSubscription(subscription)">编辑规则</button><button class="text-button" :disabled="!!busyId" @click="toggleSubscription(subscription)">{{ subscription.enabled ? '暂停' : '恢复' }}</button></div></article></div>
      <div v-else-if="!loading && !errors.subscriptions" class="collection-empty"><Icon name="rss" :size="32" /><h3>还没有订阅规则</h3><p>添加 RSS 来源后，先建立停用的规则并试运行。</p><button class="secondary-button" @click="openSubscription()">添加订阅</button></div>
    </template>
    <template v-else-if="tab === 'candidates'">
      <div class="collection-section-heading"><div><h3>发现的候选</h3><p class="field-hint">历史基线需明确回填；同一条目不会重复创建获取任务。</p></div><label class="collection-inline-label">来源<select v-model="candidateSource"><option value="">全部来源</option><option v-for="source in sources.filter((entry): boolean => entry.kind === 'rss')" :key="source.id" :value="source.id">{{ source.name }}</option></select></label></div>
      <div v-if="candidates.length" class="collection-records"><article v-for="candidate in candidates" :key="candidate.id" class="collection-card"><div class="collection-section-heading"><h3>{{ candidate.title }}</h3><span class="status-pill">{{ collectionDecisionLabel(candidate.decision) }}</span></div><p class="field-hint">{{ sourceName(candidate.sourceId) }} · {{ formatSize(candidate.sizeBytes) }} · {{ collectionTime(candidate.publishedAt || candidate.createdAt) }}</p><p>{{ candidate.reason || '等待查看与决定。' }}</p><div class="collection-actions"><button v-if="candidate.taskId" class="text-button" :disabled="!!busyId" @click="openTaskId(candidate.taskId)">查看关联任务</button><template v-else-if="canAcquire(candidate)"><button class="text-button" :disabled="!!busyId || !!errors.libraries" @click="openAcquire(candidate)">确认获取</button><button class="text-button" :disabled="!!busyId" @click="ignoreCandidate(candidate)">忽略</button></template><span v-else-if="candidate.decision === 'baseline'" class="field-hint">如需获取历史条目，请在来源中明确回填。</span></div></article></div>
      <div v-else-if="!loading && !errors.candidates" class="collection-empty"><Icon name="check" :size="32" /><h3>当前没有候选</h3><p>启用并收取 RSS 来源后，新条目会保存在这里。</p><button class="secondary-button" @click="tab = 'sources'">查看来源</button></div>
      <div v-if="candidateTotal > collectionPageSize" class="collection-pagination"><button class="secondary-button" :disabled="candidatePage <= 1 || loading" @click="candidatePage -= 1">上一页</button><span>第 {{ candidatePage }} / {{ Math.ceil(candidateTotal / collectionPageSize) }} 页 · {{ candidateTotal }} 条</span><button class="secondary-button" :disabled="candidatePage * collectionPageSize >= candidateTotal || loading" @click="candidatePage += 1">下一页</button></div>
    </template>
    <template v-else>
      <div class="collection-section-heading"><div><h3>获取与入库任务</h3><p class="field-hint">最近同步：{{ collectionTime(lastSyncedAt) }}。打开详情查看实际阶段和恢复操作。</p></div><label class="collection-inline-label">获取阶段<select v-model="taskState"><option value="">全部阶段</option><option value="queued">排队中</option><option value="acquiring">获取中</option><option value="completed">获取完成</option><option value="failed">获取失败</option><option value="reconciliation_required">需要对账</option><option value="canceled">已取消</option></select></label></div>
      <div v-if="tasks.length" class="collection-records"><button v-for="task in tasks" :key="task.id" class="collection-task-row" @click="openTask(task)"><Icon :name="task.contentType" /><div><strong>{{ task.title }}</strong><p class="field-hint">{{ sourceName(task.sourceId) }} · {{ formatSize(task.bytes) }} · {{ collectionTime(task.updatedAt) }}</p><p class="field-hint">{{ task.message }}</p></div><div class="collection-task-status"><span class="status-pill">{{ collectionStateLabel(task.state) }}</span><span class="field-hint">{{ collectionImportLabel(task.importState) }}</span></div><Icon name="chevron" /></button></div>
      <div v-else-if="!loading && !errors.tasks" class="collection-empty"><Icon name="collection" :size="32" /><h3>{{ taskState ? '此阶段没有任务' : '还没有收集任务' }}</h3><p>确认候选或预览资源后，任务会持久保存在这里。</p><button class="secondary-button" @click="taskState ? taskState = '' : openPreview()">{{ taskState ? '查看全部任务' : '预览资源' }}</button></div>
      <div v-if="taskTotal > collectionPageSize" class="collection-pagination"><button class="secondary-button" :disabled="taskPage <= 1 || loading" @click="taskPage -= 1">上一页</button><span>第 {{ taskPage }} / {{ Math.ceil(taskTotal / collectionPageSize) }} 页 · {{ taskTotal }} 条</span><button class="secondary-button" :disabled="taskPage * collectionPageSize >= taskTotal || loading" @click="taskPage += 1">下一页</button></div>
    </template>
  </section>
  <CollectionSourceDialog v-if="sourceDialog" :source="editingSource" :initial-kind="initialKind" :capabilities="errors.capabilities ? null : capabilities" @close="sourceDialog = false" @saved="sourceSaved" />
  <CollectionSubscriptionDialog v-if="subscriptionDialog" :subscription="editingSubscription" :sources="sources" :libraries="libraries" @close="subscriptionDialog = false" @saved="subscriptionSaved" />
  <CollectionPreviewDialog v-if="previewDialog" :sources="sources" :libraries="libraries" :capabilities="errors.capabilities ? null : capabilities" :initial-source-id="previewSourceId" :initial-kind="previewKind" @close="previewDialog = false" @created="restart(true)" @open-task="taskCreated" />
  <CollectionJMComicDialog v-if="jmcomicDialog" :sources="sources" :libraries="libraries" :capabilities="errors.capabilities ? null : capabilities" :initial-source-id="jmcomicSourceId" @close="jmcomicDialog = false" @created="restart(true)" @open-task="taskCreated" />
  <CollectionUAVDialog v-if="uavDialog" :sources="sources" :libraries="libraries" :capabilities="errors.capabilities ? null : capabilities" :initial-source-id="uavSourceId" @close="uavDialog = false" @open-task="taskCreated" />
  <CollectionAcquireDialog v-if="acquiring" :candidate="acquiring" :libraries="libraries" @close="acquiring = null" @created="taskCreated" />
  <CollectionTaskDialog v-if="selectedTask" :key="selectedTask.id" :initial-task="selectedTask" @close="selectedTask = null" @updated="taskUpdated" @open-item="openItem" @open-download="openDownload" />
  <CollectionExternalPanel v-if="externalSource" :key="externalSource.id" :source="externalSource" @close="externalSource = null" @edit="editExternal" @import="importExternal" />
  <DialogShell v-if="backfillSource" title="确认历史回填" @close="backfillSource = null"><div class="collection-form"><p>回填“{{ backfillSource.name }}”的有界历史条目。历史资源将成为可审查候选，确认前不会按普通首次基线忽略。</p><label class="collection-check collection-consent"><input v-model="backfillConsent" type="checkbox" /><span>我需要审查该来源当前可回填的历史条目</span></label><p v-if="actionError" class="error-box" role="alert">{{ actionError }}</p><div class="dialog-actions"><button class="secondary-button" @click="backfillSource = null">取消</button><button class="primary-button" :disabled="!backfillConsent || !!busyId" @click="refreshSource(backfillSource, true)">{{ busyId ? '回填中…' : '确认历史回填' }}</button></div></div></DialogShell>
  <DialogShell v-if="rulePreview" title="订阅规则试运行" @close="rulePreview = null"><div class="collection-form"><p>“{{ rulePreview.name }}”匹配 {{ rulePreview.matched }} 条。此次试运行没有创建获取任务。</p><div v-for="candidate in rulePreview.candidates" :key="candidate.id" class="collection-preview-row"><strong>{{ candidate.title }}</strong><p class="field-hint">{{ collectionDecisionLabel(candidate.decision) }} · {{ candidate.reason }}</p></div><p v-if="!rulePreview.candidates.length" class="field-hint">当前保存的候选中没有匹配。可先收取来源或调整规则。</p><div class="dialog-actions"><button class="secondary-button" @click="rulePreview = null">关闭</button></div></div></DialogShell>
</template>
