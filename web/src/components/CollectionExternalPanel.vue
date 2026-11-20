<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ApiError, api, createRequestId, errorMessage, isAbort } from '../api'
import { collectionExternalUrl, collectionTime } from '../collection'
import type { CollectionExternalAction, CollectionExternalInfo, CollectionExternalItem, CollectionExternalMutation, CollectionExternalOptions, CollectionExternalPage, CollectionSource } from '../types'
import DialogShell from './DialogShell.vue'
import CollectionMoviePilotWorkflow from './CollectionMoviePilotWorkflow.vue'
import Icon from './Icon.vue'

/** 外部管理只有一个入口，订阅和下载事实仍归外部服务所有。 */
interface Props {
  /** 已保存且不含秘密的来源。 */ source: CollectionSource
}
/** 只有显式确认后才会发送的不可变操作。 */
interface PendingAction {
  /** 绑定不可变意图的请求编号。 */ requestId: string
  /** 来源配置修订号。 */ revision: number
  /** 固定业务动作。 */ mutation: CollectionExternalMutation
  /** 不含地址或凭据的确认说明。 */ summary: string
  /** 是否已尝试发送，发送后不能以新编号重试。 */ submitted: boolean
}
/** 外部业务分区保持简短、固定。 */
type ExternalSection = 'resources' | 'queue' | 'history' | 'workflow'
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; edit: []; import: [] }>()
/** 外部产品的固定中文展示名称。 */
const serviceNames: Record<string, string> = { moviepilot: 'MoviePilot', autobangumi: 'AutoBangumi', sonarr: 'Sonarr', radarr: 'Radarr' }
/** 显示业务分区，不将外部任务混进本地扫描任务。 */
const sections: { /** 分区编号。 */ id: ExternalSection; /** 中文名称。 */ label: string }[] = [{ id: 'resources', label: '订阅' }, { id: 'queue', label: '任务' }, { id: 'history', label: '历史' }]
const section = ref<ExternalSection>(props.source.externalService === 'moviepilot' ? 'workflow' : 'resources')
const page = ref<number>(1)
const info = ref<CollectionExternalInfo | null>(null)
const view = ref<CollectionExternalPage | null>(null)
const actions = ref<CollectionExternalAction[]>([])
const online = ref<boolean>(false)
const reading = ref<boolean>(false)
const connecting = ref<boolean>(false)
const actionBusy = ref<boolean>(false)
const connectionError = ref<string>('')
const viewError = ref<string>('')
const actionsError = ref<string>('')
const actionError = ref<string>('')
const feedback = ref<string>('')
const checkedAt = ref<string>('')
const adding = ref<boolean>(false)
const options = ref<CollectionExternalOptions>({ roots: [], profiles: [] })
const optionsBusy = ref<boolean>(false)
const optionsError = ref<string>('')
const query = ref<string>('')
const lookupItems = ref<CollectionExternalItem[]>([])
const lookupBusy = ref<boolean>(false)
const lookupError = ref<string>('')
const lookupDone = ref<boolean>(false)
const selectedItem = ref<CollectionExternalItem | null>(null)
const rootId = ref<string>('')
const profileId = ref<string>('')
const title = ref<string>('')
const externalId = ref<string>('')
const mediaType = ref<'movie' | 'series'>('movie')
const season = ref<number>(1)
const rssUrl = ref<string>('')
const pending = ref<PendingAction | null>(null)
const acknowledgeAction = ref<CollectionExternalAction | null>(null)
const checkedExternally = ref<boolean>(false)
const confirmation = ref<HTMLElement | null>(null)
const workflowVersion = ref<number>(0)
const actionLogOpen = ref<boolean>(false)
let readController: AbortController | null = null
let lookupController: AbortController | null = null
let optionsController: AbortController | null = null
let actionController: AbortController | null = null
let disposed: boolean = false
const serviceName = computed<string>((): string => serviceNames[props.source.externalService] || '外部服务')
const basePath = computed<string>((): string => `/collection/sources/${encodeURIComponent(props.source.id)}/external`)
const isServarr = computed<boolean>((): boolean => ['sonarr', 'radarr'].includes(props.source.externalService))
const unresolved = computed<boolean>((): boolean => actions.value.some((action: CollectionExternalAction): boolean => action.state === 'uncertain' || action.state === 'dispatching'))
const canWrite = computed<boolean>((): boolean => online.value && !!info.value?.supported && props.source.enabled && !reading.value && !actionBusy.value && !pending.value && !acknowledgeAction.value && !unresolved.value && !actionsError.value)
const visibleSections = computed<typeof sections>((): typeof sections => {
  const entries: typeof sections = sections.filter((entry: typeof sections[number]): boolean => supports(entry.id))
  if (props.source.externalService === 'moviepilot' && ['integrated_search', 'downloads', 'organize', 'scrape', 'refresh_library'].some((capability: string): boolean => supports(capability))) entries.unshift({ id: 'workflow', label: '工作台' })
  return entries
})

/** 查询实际检测结果，未知能力不显示可点击入口。 */
function supports(capability: string): boolean { return !!info.value?.capabilities.includes(capability) }
/** 将受控状态映射为中文，未知状态保留中性说明。 */
function statusLabel(value: string): string {
  const labels: Record<string, string> = { R: '运行中', S: '已暂停', P: '处理中', downloading: '下载中', completed: '外部已完成', queued: '等待中', paused: '已暂停', failed: '失败', warning: '需要注意', unknown: '未知', delay: '延迟中', downloadClientUnavailable: '下载器不可用', fallback: '等待备用来源', continuing: '连载中', ended: '已完结', announced: '已公布', inCinemas: '院线上映', released: '已发行', running: '处理中', enabled: '已启用', disabled: '已停用', downloadingMetadata: '获取信息中', stalledDL: '等待下载', uploading: '做种中', seeding: '做种中', pausedDL: '下载已暂停', pausedUP: '做种已暂停', stalledUP: '等待上传', checkingUP: '校验中', checkingDL: '校验中', metaDL: '获取信息中' }
  return labels[value] || (/\p{Script=Han}/u.test(value) ? value : value ? '外部状态已更新' : '未提供状态')
}
/** 监控与订阅启用使用明确目标状态，不暗示已下载或已入库。 */
function enabledLabel(item: CollectionExternalItem): string { return item.enabled === null ? '未提供启用状态' : item.enabled ? (isServarr.value ? '监控中' : '已启用') : (isServarr.value ? '未监控' : '已停用') }
/** 操作日志只显示服务器确认的独立提交状态。 */
function actionStateLabel(state: string): string { return ({ dispatching: '正在提交', succeeded: '已确认', failed: '未成功', uncertain: '结果待确认', unconfirmed: '未确认，已人工解除阻塞' } as Record<string, string>)[state] || '等待状态' }
/** 固定动作的中文名称。 */
function actionKindLabel(kind: string): string { return ({ add: props.source.externalService === 'autobangumi' ? '登记 RSS' : '新增订阅', monitor: '设置启用状态', search: '搜索此订阅', download: '下载所选资源', download_start: '继续下载', download_stop: '暂停下载', organize: '复制整理', scrape: '刮削元数据', refresh_library: '刷新 MoviePilot 媒体库索引' } as Record<string, string>)[kind] || '外部操作' }
/** 确认区可能位于长内容上方，主动聚焦让键盘和手机用户都能继续。 */
async function focusConfirmation(): Promise<void> { await nextTick(); confirmation.value?.scrollIntoView({ block: 'nearest', behavior: 'auto' }); confirmation.value?.focus({ preventScroll: true }) }
/** MoviePilot 业务步骤共享持久操作确认，不创建第二层对话框。 */
function prepareWorkflow(mutation: CollectionExternalMutation, summary: string): void {
  if (!canWrite.value || props.source.externalService !== 'moviepilot') return
  const capability: string = ({ download: 'integrated_search', download_start: 'downloads', download_stop: 'downloads', organize: 'organize', scrape: 'scrape', refresh_library: 'refresh_library', add: 'add' } as Record<string, string>)[mutation.kind] || ''
  if (!capability || !supports(capability)) return
  pending.value = { requestId: createRequestId(), revision: props.source.revision, mutation, summary, submitted: false }
  actionError.value = ''; feedback.value = ''
  void focusConfirmation()
}
/** 只展示有效百分比，未知进度不填零。 */
function progressLabel(value: number | null): string { return value !== null && Number.isFinite(value) && value >= 0 && value <= 100 ? `${Math.round(value * 10) / 10}%` : '未提供进度' }
/** 一次读取周期内连接、列表和操作日志的错误独立保留。 */
async function readView(probe: boolean = false): Promise<void> {
  if (disposed || document.hidden || actionBusy.value) return
  readController?.abort()
  const request: AbortController = new AbortController()
  readController = request
  reading.value = true
  connecting.value = probe
  if (probe) { online.value = false; connectionError.value = '' }
  const actionRequest: Promise<PromiseSettledResult<{ actions: CollectionExternalAction[] }>> = Promise.allSettled([api<{ actions: CollectionExternalAction[] }>(`${basePath.value}/actions`, 'GET', undefined, request.signal)]).then((results: PromiseSettledResult<{ actions: CollectionExternalAction[] }>[]): PromiseSettledResult<{ actions: CollectionExternalAction[] }> => results[0]!)
  try {
    if (probe) {
      try {
        const result = await api<{ info: CollectionExternalInfo }>(`${basePath.value}/test`, 'POST', { revision: props.source.revision }, request.signal)
        if (request.signal.aborted || readController !== request || disposed) return
        info.value = result.info
        online.value = true
        checkedAt.value = new Date().toISOString()
        // 未适配的工作流降回实际可读分区，不留下没有当前页的空面板。
        if (!visibleSections.value.some((entry: typeof sections[number]): boolean => entry.id === section.value) && visibleSections.value[0]) section.value = visibleSections.value[0].id
      } catch (failure: unknown) { if (!isAbort(failure) && readController === request) connectionError.value = errorMessage(failure) }
    }
    if (online.value && section.value !== 'workflow' && supports(section.value)) {
      try {
        const result = await api<CollectionExternalPage>(`${basePath.value}/items?${new URLSearchParams({ section: section.value, page: String(page.value) })}`, 'GET', undefined, request.signal)
        if (request.signal.aborted || readController !== request || disposed) return
        view.value = result
        info.value = result.info
        viewError.value = ''
        checkedAt.value = new Date().toISOString()
      } catch (failure: unknown) { if (!isAbort(failure) && readController === request) { viewError.value = errorMessage(failure); online.value = false } }
    }
    const actionResult: PromiseSettledResult<{ actions: CollectionExternalAction[] }> = await actionRequest
    if (request.signal.aborted || readController !== request || disposed) return
    if (actionResult.status === 'fulfilled') { actions.value = actionResult.value.actions; actionsError.value = '' }
    else if (!isAbort(actionResult.reason)) actionsError.value = errorMessage(actionResult.reason)
  } finally {
    if (readController === request) { readController = null; reading.value = false; connecting.value = false }
  }
}
/** 切换分区后清空上一分区，避免将旧任务当成当前订阅。 */
function changeSection(next: ExternalSection): void {
  if (section.value === next || actionBusy.value || pending.value || acknowledgeAction.value) return
  view.value = null
  viewError.value = ''
  section.value = next
  page.value = 1
}
/** 在现有外部配置中选择，不创建上游目录或画质。 */
async function loadOptions(): Promise<void> {
  if (optionsBusy.value) return
  optionsController?.abort()
  const request: AbortController = new AbortController()
  optionsController = request
  optionsBusy.value = true
  optionsError.value = ''
  try {
    const result = await api<CollectionExternalOptions>(`${basePath.value}/options`, 'GET', undefined, request.signal)
    if (disposed || request.signal.aborted || optionsController !== request) return
    options.value = result
    if (result.roots.length === 1) rootId.value = result.roots[0]!.id
    if (result.profiles.length === 1) profileId.value = result.profiles[0]!.id
  } catch (failure: unknown) { if (!isAbort(failure)) optionsError.value = errorMessage(failure) }
  finally { if (optionsController === request) { optionsBusy.value = false; optionsController = null } }
}
/** 打开明确新增表单，不在打开时创建订阅或搜索资源。 */
function startAdd(): void {
  if (!canWrite.value || !supports('add')) return
  adding.value = true
  actionError.value = ''
  feedback.value = ''
  if (isServarr.value) void loadOptions()
}
/** 资料关键词变化使旧结果失效，不自动执行网络搜索。 */
function queryChanged(): void { lookupController?.abort(); lookupItems.value = []; selectedItem.value = null; lookupDone.value = false; lookupError.value = ''; lookupBusy.value = false }
/** 手工查找元数据，仅选择明确身份后才能建立监控。 */
async function lookup(): Promise<void> {
  if (lookupBusy.value || !query.value.trim() || !canWrite.value || !supports('lookup')) return
  lookupController?.abort()
  const request: AbortController = new AbortController()
  lookupController = request
  lookupBusy.value = true
  lookupError.value = ''
  selectedItem.value = null
  try {
    const result = await api<{ items: CollectionExternalItem[] }>(`${basePath.value}/lookup?${new URLSearchParams({ q: query.value.trim() })}`, 'GET', undefined, request.signal)
    if (disposed || request.signal.aborted || lookupController !== request) return
    lookupItems.value = result.items
    lookupDone.value = true
  } catch (failure: unknown) { if (!isAbort(failure)) lookupError.value = errorMessage(failure) }
  finally { if (lookupController === request) { lookupBusy.value = false; lookupController = null } }
}
/** 从当前表单建立固定新增意图，敏感 RSS 地址不会进入确认文案。 */
function prepareAdd(): void {
  if (!canWrite.value || !supports('add')) return
  let mutation: CollectionExternalMutation
  let label: string
  if (isServarr.value) {
    if (!selectedItem.value || !rootId.value || !profileId.value) { actionError.value = '请先选择正确的资料、外部保存目录和画质配置。'; return }
    mutation = { kind: 'add', externalId: selectedItem.value.externalId, title: selectedItem.value.title, mediaType: props.source.externalService === 'sonarr' ? 'series' : 'movie', rootId: rootId.value, profileId: profileId.value, enabled: true }
    label = selectedItem.value.title
  } else if (props.source.externalService === 'moviepilot') {
    if (!/^[1-9]\d*$/.test(externalId.value.trim()) || !title.value.trim() || (mediaType.value === 'series' && (!Number.isInteger(season.value) || season.value < 1 || season.value > 999))) { actionError.value = '请填写名称、有效 TMDB 编号；电视剧还需填写季号。'; return }
    mutation = { kind: 'add', externalId: externalId.value.trim(), title: title.value.trim(), mediaType: mediaType.value, season: mediaType.value === 'series' ? season.value : 0, enabled: true }
    label = title.value.trim()
  } else {
    if (!rssUrl.value.trim() || !title.value.trim()) { actionError.value = '请填写 RSS 名称和地址。'; return }
    mutation = { kind: 'add', title: title.value.trim(), mediaType: 'rss', rssUrl: rssUrl.value.trim(), enabled: true }
    label = title.value.trim()
  }
  pending.value = { requestId: createRequestId(), revision: props.source.revision, mutation, summary: props.source.externalService === 'autobangumi' ? `将“${label}”登记为 AutoBangumi 的 RSS 来源。番剧规则仍在 AutoBangumi 中管理。` : `在 ${serviceName.value} 新增“${label}”并启用监控。后续自动获取由该服务负责。`, submitted: false }
  actionError.value = ''
  void focusConfirmation()
}
/** 单目标操作采用明确目标状态，并在同一面板内再次说明影响。 */
function prepareItem(item: CollectionExternalItem, kind: 'monitor' | 'search'): void {
  if (!canWrite.value || !supports(kind) || (kind === 'monitor' && item.enabled === null)) return
  const mutation: CollectionExternalMutation = kind === 'monitor' ? { kind, targetId: item.id, enabled: !item.enabled } : { kind, targetId: item.id }
  const summary: string = kind === 'search' ? `让 ${serviceName.value} 立即搜索“${item.title}”。可能产生新的下载，包括已暂停的订阅；仅作用于这一条。` : `${item.enabled ? '停用' : '启用'}“${item.title}”的${isServarr.value ? '监控' : '自动处理'}。这不会暂停已发出的下载，也不会删除文件。`
  pending.value = { requestId: createRequestId(), revision: props.source.revision, mutation, summary, submitted: false }
  actionError.value = ''
  feedback.value = ''
  void focusConfirmation()
}
/** 合并较新的操作回执，避免短暂的旧列表覆盖已收到结果。 */
function rememberAction(action: CollectionExternalAction): void { actions.value = [action, ...actions.value.filter((current: CollectionExternalAction): boolean => current.id !== action.id)].slice(0, 30) }
/** 人工核查后保留展开状态，用户可以直接看见仍未确认的事实。 */
function actionLogToggled(event: Event): void { if (event.currentTarget instanceof HTMLDetailsElement) actionLogOpen.value = event.currentTarget.open }
/** 首次提交或以原编号读取结果，不创建新的不确定意图。 */
async function submitPending(): Promise<void> {
  if (!pending.value || actionBusy.value) return
  const intent: PendingAction = pending.value
  intent.submitted = true
  readController?.abort()
  actionBusy.value = true
  actionError.value = ''
  const request: AbortController = new AbortController()
  actionController = request
  try {
    const result = await api<{ action: CollectionExternalAction }>(`${basePath.value}/actions`, 'POST', { requestId: intent.requestId, revision: intent.revision, ...intent.mutation }, request.signal)
    if (disposed || request.signal.aborted) return
    rememberAction(result.action)
    workflowVersion.value += 1
    pending.value = null
    if (result.action.state === 'succeeded') { adding.value = false; rssUrl.value = ''; feedback.value = result.action.message; console.info('外部服务操作结果已确认') }
    else feedback.value = result.action.message
  } catch (failure: unknown) {
    if (!isAbort(failure)) {
      actionError.value = errorMessage(failure)
      // 明确参数或权限拒绝未创建新操作，允许调整后重新确认；网络失败保留原编号。
      if (failure instanceof ApiError && [400, 401, 403, 404, 409, 413, 422].includes(failure.status)) pending.value = null
    }
  } finally { if (actionController === request) { actionController = null; actionBusy.value = false; if (!pending.value && !disposed) void readView() } }
}
/** 结果不确定时仅核对原意图，不重发外部写动作。 */
async function reconcile(action: CollectionExternalAction): Promise<void> {
  if (actionBusy.value || pending.value) return
  readController?.abort()
  const request: AbortController = new AbortController()
  actionController = request
  actionBusy.value = true
  actionError.value = ''
  try {
    const result = await api<{ action: CollectionExternalAction }>(`${basePath.value}/actions/${encodeURIComponent(action.id)}/reconcile`, 'POST', {}, request.signal)
    if (disposed || request.signal.aborted) return
    rememberAction(result.action)
    feedback.value = result.action.message
  } catch (failure: unknown) { if (!isAbort(failure)) actionError.value = errorMessage(failure) }
  finally { if (actionController === request) { actionController = null; actionBusy.value = false } }
}
/** 只有用户在外部核查后，才允许为无法证明结果的记录解除阻塞。 */
function prepareAcknowledge(action: CollectionExternalAction): void {
  if (action.state !== 'uncertain' || actionBusy.value || pending.value) return
  acknowledgeAction.value = action
  checkedExternally.value = false
  actionError.value = ''
}
/** 人工解除阻塞保留未确认事实，不重发动作也不标记成功。 */
async function acknowledge(): Promise<void> {
  if (!acknowledgeAction.value || !checkedExternally.value || actionBusy.value) return
  readController?.abort()
  const action: CollectionExternalAction = acknowledgeAction.value
  const request: AbortController = new AbortController()
  actionController = request
  actionBusy.value = true
  actionError.value = ''
  try {
    const result = await api<{ action: CollectionExternalAction }>(`${basePath.value}/actions/${encodeURIComponent(action.id)}/acknowledge`, 'POST', { checkedExternally: true }, request.signal)
    if (disposed || request.signal.aborted) return
    rememberAction(result.action)
    feedback.value = result.action.message
    acknowledgeAction.value = null
    checkedExternally.value = false
    console.info('外部操作已人工解除阻塞，保留未确认结果')
  } catch (failure: unknown) { if (!isAbort(failure)) actionError.value = errorMessage(failure) }
  finally { if (actionController === request) { actionController = null; actionBusy.value = false } }
}
/** 取消尚未提交的确认，不丢弃已经发送但未知的意图。 */
function cancelPending(): void { if (!pending.value?.submitted && !actionBusy.value) { pending.value = null; actionError.value = '' } }
/** 离开新增表单时清除仅写入的 RSS 输入。 */
function cancelAdd(): void { if (pending.value || actionBusy.value) return; adding.value = false; rssUrl.value = ''; queryChanged() }
/** 隐藏页面时取消读取，恢复后由用户显式刷新。 */
function visibilityChanged(): void { if (document.hidden) { readController?.abort(); lookupController?.abort(); optionsController?.abort() } }
watch([section, page], (): void => { void readView() })
watch(unresolved, (value: boolean): void => { if (value) actionLogOpen.value = true })
onMounted((): void => { document.addEventListener('visibilitychange', visibilityChanged); void readView(true) })
onBeforeUnmount((): void => { disposed = true; readController?.abort(); lookupController?.abort(); optionsController?.abort(); actionController?.abort(); rssUrl.value = ''; document.removeEventListener('visibilitychange', visibilityChanged) })
</script>

<template>
  <DialogShell :title="`${source.name} · 管理`" @close="emit('close')">
    <div class="collection-form collection-external-panel">
      <div class="collection-section-heading"><div><strong>由 {{ serviceName }} 负责获取</strong><p class="field-hint">连接检测不会创建订阅、搜索或下载。完成文件需另行预览并扫描入库。</p></div><button class="secondary-button" :disabled="reading || actionBusy" @click="readView(true)"><Icon name="refresh" />{{ connecting ? '检测中…' : '检测并刷新' }}</button></div>
      <p v-if="connectionError" class="error-box" role="alert">{{ connectionError }} <button class="text-button" :disabled="actionBusy" @click="emit('edit')">修改连接</button></p>
      <div v-else-if="info" class="collection-callout"><div class="collection-section-heading"><strong>{{ online ? (info.supported ? '已连接' : '已连接，版本未适配') : '上次连接信息' }}</strong><span class="field-hint">{{ serviceName }} {{ info.version }} · {{ collectionTime(checkedAt) }}</span></div><p class="field-hint">{{ info.message }}</p></div>
      <p v-if="info && !online && !connectionError" class="collection-warning">连接当前不可用，已保留上次信息。请先检测并刷新，再读取最新记录或执行操作。</p>
      <p v-if="!source.enabled" class="collection-warning">此来源已停用。可以查看记录，启用后才能从本系统创建新操作。</p>
      <p v-if="actionsError" class="error-box" role="alert">最近操作读取失败：{{ actionsError }}。请刷新确认是否有待核对操作。</p>
      <p v-if="unresolved" class="collection-warning" role="status">有操作结果待确认。请先在下方“最近操作”核对结果，避免重复获取。</p>
      <p v-if="feedback" class="collection-feedback" role="status">{{ feedback }}</p>
      <p v-if="actionError" class="error-box" role="alert">{{ actionError }}</p>

      <div v-if="pending" ref="confirmation" class="collection-callout collection-external-confirm" role="region" aria-label="确认外部操作" tabindex="-1">
        <strong>{{ pending.submitted ? '保留了此次操作编号' : '确认此次操作' }}</strong><p>{{ pending.summary }}</p>
        <p v-if="pending.submitted" class="field-hint">结果尚未确认。读取结果会复用原编号；不要重新创建同一操作。关闭后也可在最近操作中核对服务器已接收的记录。</p>
        <div class="collection-actions"><button v-if="!pending.submitted" class="secondary-button" :disabled="actionBusy" @click="cancelPending">取消</button><button class="primary-button" :disabled="actionBusy" @click="submitPending">{{ actionBusy ? '正在确认…' : pending.submitted ? '读取此次操作结果' : '确认执行' }}</button></div>
      </div>
      <div v-if="acknowledgeAction" class="collection-callout collection-external-confirm" role="region" aria-label="人工核查外部操作">
        <strong>先在外部应用核对此次操作</strong><p>{{ actionKindLabel(acknowledgeAction.kind) }}：{{ acknowledgeAction.message }}</p><p class="field-hint">若服务无法提供可追踪回执，系统无法证明是否执行。解除阻塞只允许继续新的操作；这条记录仍为未确认，也不会重新提交。</p>
        <a v-if="collectionExternalUrl(source.externalUrl)" class="text-button collection-self-start" :href="collectionExternalUrl(source.externalUrl)" target="_blank" rel="noopener noreferrer">打开外部应用核查<Icon name="external" :size="14" /></a>
        <label class="collection-check"><input v-model="checkedExternally" type="checkbox" :disabled="actionBusy" /><span>我已在外部应用核对此次操作，并了解系统仍未确认执行结果</span></label><div class="collection-actions"><button class="secondary-button" :disabled="actionBusy" @click="acknowledgeAction = null">取消</button><button class="primary-button" :disabled="!checkedExternally || actionBusy" @click="acknowledge">{{ actionBusy ? '正在保存…' : '已在外部核对，解除阻塞' }}</button></div>
      </div>

      <template v-if="info && visibleSections.length">
        <nav class="collection-tabs" aria-label="外部服务分区"><button v-for="entry in visibleSections" :key="entry.id" :class="{ active: section === entry.id }" :aria-current="section === entry.id ? 'page' : undefined" :disabled="actionBusy || !!pending || !!acknowledgeAction" @click="changeSection(entry.id)">{{ entry.label }}</button></nav>
        <CollectionMoviePilotWorkflow v-if="section === 'workflow'" :source="source" :capabilities="info.capabilities" :online="online" :disabled="!canWrite" :action-version="workflowVersion" @prepare="prepareWorkflow" />
        <template v-else>
        <div v-if="section === 'resources'" class="collection-section-heading"><h3>{{ source.externalService === 'autobangumi' ? '番剧规则与 RSS' : '外部订阅' }}</h3><button v-if="supports('add') && !adding" class="primary-button" :disabled="!canWrite || !!actionsError" @click="startAdd"><Icon name="plus" />{{ source.externalService === 'autobangumi' ? '登记 RSS' : '新增订阅' }}</button></div>
        <form v-if="adding && section === 'resources'" class="collection-callout collection-external-add" @submit.prevent="prepareAdd">
          <div class="collection-section-heading"><h3>{{ source.externalService === 'autobangumi' ? '登记 RSS 来源' : '新增外部订阅' }}</h3><button class="text-button" type="button" :disabled="!!pending || actionBusy" @click="cancelAdd">收起</button></div>
          <fieldset class="collection-external-fields" :disabled="!!pending || actionBusy">
            <template v-if="isServarr">
              <label>查找影视资料<div class="collection-external-search"><input v-model="query" maxlength="160" placeholder="输入标题或元数据编号" @input="queryChanged" /><button class="secondary-button" type="button" :disabled="lookupBusy || !query.trim() || !canWrite" @click="lookup">{{ lookupBusy ? '查找中…' : '查找资料' }}</button></div><span class="field-hint">这里只查找元数据，不搜索下载源，也不会开始下载。</span></label>
              <p v-if="lookupError" class="error-box" role="alert">{{ lookupError }}</p>
              <div v-if="lookupItems.length" class="collection-external-choices"><button v-for="item in lookupItems" :key="item.externalId" type="button" class="collection-external-choice" :class="{ active: selectedItem?.externalId === item.externalId }" :aria-pressed="selectedItem?.externalId === item.externalId" @click="selectedItem = item"><strong>{{ item.title }}</strong><span class="field-hint">{{ item.year || '年份未知' }} · {{ source.externalService === 'sonarr' ? 'TVDB' : 'TMDB' }} {{ item.externalId }}{{ selectedItem?.externalId === item.externalId ? ' · 已选择' : '' }}</span></button></div>
              <p v-else-if="lookupDone && !lookupBusy && !lookupError" class="field-hint">没有找到资料。请调整标题或使用准确的元数据编号。</p>
              <p v-if="optionsBusy" class="field-hint" role="status">正在读取外部实例的现有配置…</p>
              <p v-if="optionsError" class="error-box" role="alert">{{ optionsError }} <button class="text-button" type="button" @click="loadOptions">重新读取</button></p>
              <div class="collection-field-grid"><label>外部保存目录<select v-model="rootId" required :disabled="optionsBusy"><option value="" disabled>选择已有目录</option><option v-for="option in options.roots" :key="option.id" :value="option.id">{{ option.name }}</option></select><span class="field-hint">{{ serviceName }} 的已有配置，与本系统的本地挂载分开。</span></label><label>画质配置<select v-model="profileId" required :disabled="optionsBusy"><option value="" disabled>选择已有配置</option><option v-for="option in options.profiles" :key="option.id" :value="option.id">{{ option.name }}</option></select><span class="field-hint">{{ serviceName }} 将按该配置持续监控。</span></label></div>
              <p v-if="!optionsBusy && !optionsError && (!options.roots.length || !options.profiles.length)" class="collection-warning">请先在 {{ serviceName }} 配置保存目录和画质，再重新读取。</p>
            </template>
            <template v-else>
              <label>{{ source.externalService === 'autobangumi' ? 'RSS 名称' : '资源名称' }}<input v-model="title" required maxlength="80" :placeholder="source.externalService === 'autobangumi' ? '例如：我的动漫 RSS' : '填写要订阅的准确标题'" /></label>
              <label v-if="source.externalService === 'autobangumi'">RSS 地址<input v-model="rssUrl" type="url" required autocomplete="off" spellcheck="false" placeholder="https://…" /><span class="field-hint">只写入，不回显。这里只登记 RSS，番剧规则在 AutoBangumi 中管理。</span></label>
              <template v-else><div class="collection-field-grid"><label>TMDB 编号<input v-model="externalId" required inputmode="numeric" pattern="[1-9][0-9]*" maxlength="18" placeholder="填写准确的 TMDB 编号" /><span class="field-hint">按完整媒体身份提交，不会仅凭标题匹配。</span></label><label>媒体类型<select v-model="mediaType"><option value="movie">电影</option><option value="series">电视剧</option></select></label></div><label v-if="mediaType === 'series'">季号<input v-model.number="season" type="number" min="1" max="999" step="1" required /></label></template>
            </template>
            <p class="field-hint">新增后由 {{ serviceName }} 的自动规则负责获取。立即搜索需要另行确认。</p>
            <button class="primary-button collection-self-start" type="submit" :disabled="!canWrite || !!actionsError || optionsBusy || (isServarr && (!selectedItem || !rootId || !profileId))">继续确认</button>
          </fieldset>
        </form>
        <p v-if="reading" class="collection-loading" role="status"><span class="loader"></span>{{ connecting ? '正在检测连接…' : '正在读取外部状态…' }}</p>
        <p v-if="viewError" class="error-box" role="alert">{{ viewError }}{{ view?.items.length ? ' 当前显示上次读取的记录。' : '' }} <button class="text-button" :disabled="reading || actionBusy" @click="readView(true)">重新连接</button></p>
        <p v-if="view?.message" class="section-note">{{ view.message }}</p>
        <div v-if="view?.items.length" class="collection-records"><article v-for="item in view.items" :key="item.id" class="collection-preview-row"><div class="collection-section-heading"><strong>{{ item.title || '外部条目' }}</strong><span class="status-pill">{{ section === 'resources' ? enabledLabel(item) : statusLabel(item.status) }}</span></div><p class="field-hint"><template v-if="item.year">{{ item.year }} · </template><template v-if="item.season">第 {{ item.season }} 季 · </template>{{ serviceName }} {{ section === 'resources' ? (item.mediaType === 'rss' ? 'RSS 来源' : '管理的订阅') : section === 'queue' ? '任务' : '历史事件' }}<template v-if="item.date"> · {{ collectionTime(item.date) }}</template></p><p v-if="section === 'queue'" class="field-hint">{{ progressLabel(item.progress) }} · {{ statusLabel(item.status) }}；外部完成不等于本系统已入库。</p><div v-if="section === 'resources'" class="collection-actions"><button v-if="supports('monitor') && item.enabled !== null" class="text-button" :disabled="!canWrite || !!actionsError" @click="prepareItem(item, 'monitor')">{{ item.enabled ? (isServarr ? '停止监控' : '停用') : (isServarr ? '开启监控' : '启用') }}</button><button v-if="supports('search')" class="text-button" :disabled="!canWrite || !!actionsError" @click="prepareItem(item, 'search')">搜索此订阅</button></div></article></div>
        <div v-else-if="view && !reading && !viewError" class="collection-empty"><Icon :name="section === 'resources' ? 'rss' : 'download'" :size="28" /><h3>{{ section === 'resources' ? '还没有外部订阅' : section === 'queue' ? '当前没有外部任务' : '当前没有历史记录' }}</h3><p>{{ section === 'resources' ? '可以新增订阅，或先在外部应用中配置。' : '这是成功读取的空列表；后续状态仍由外部服务维护。' }}</p></div>
        <div v-if="view && view.total > view.pageSize" class="collection-pagination"><button class="secondary-button" :disabled="reading || page <= 1 || actionBusy" @click="page -= 1">上一页</button><span>第 {{ page }} / {{ Math.max(1, Math.ceil(view.total / view.pageSize)) }} 页 · {{ view.total }} 条</span><button class="secondary-button" :disabled="reading || page * view.pageSize >= view.total || actionBusy" @click="page += 1">下一页</button></div>
        </template>
      </template>
      <p v-else-if="connecting" class="collection-loading" role="status"><span class="loader"></span>正在检测服务版本与连接…</p>

      <details v-if="actions.length || actionsError" class="collection-advanced" :open="actionLogOpen" @toggle="actionLogToggled"><summary>最近操作{{ unresolved ? ' · 有结果待确认' : '' }}</summary><div class="collection-advanced-fields"><article v-for="action in actions" :key="action.id" class="collection-preview-row"><div class="collection-section-heading"><strong>{{ actionKindLabel(action.kind) }}</strong><span class="status-pill">{{ actionStateLabel(action.state) }}</span></div><p>{{ action.message }}</p><p class="field-hint">{{ collectionTime(action.updatedAt) }}</p><div class="collection-actions"><button v-if="action.state === 'uncertain' || action.state === 'dispatching'" class="secondary-button" :disabled="actionBusy || !!pending || !!acknowledgeAction" @click="reconcile(action)">核对此次结果</button><button v-if="action.state === 'uncertain'" class="text-button" :disabled="actionBusy || !!pending || !!acknowledgeAction" @click="prepareAcknowledge(action)">已在外部核查？</button></div></article></div></details>
      <div class="collection-callout"><strong>完成文件如何进入媒体库？</strong><p class="field-hint">{{ source.rootId ? '已配置完成目录。确认文件由外部服务整理完成后，预览并扫描登记。' : 'API 已可独立连接。需要导入时，再选择已挂载的专用完成目录。' }}</p><div class="collection-actions"><button class="secondary-button" :disabled="actionBusy || !!pending" @click="source.rootId ? emit('import') : emit('edit')">{{ source.rootId ? '预览成品导入' : '设置完成目录' }}</button><a v-if="collectionExternalUrl(source.externalUrl)" class="text-button" :href="collectionExternalUrl(source.externalUrl)" target="_blank" rel="noopener noreferrer">进入外部应用<Icon name="external" :size="14" /></a><button class="text-button" :disabled="actionBusy || !!pending" @click="emit('edit')">连接设置</button></div></div>
    </div>
  </DialogShell>
</template>
