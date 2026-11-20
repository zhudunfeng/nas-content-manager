<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, errorMessage, isAbort } from '../api'
import type { CollectionExternalMutation, CollectionMoviePilotChoice, CollectionMoviePilotChoices, CollectionSource } from '../types'
import Icon from './Icon.vue'

/** 工作流嵌入现有外部管理窗口，不创建新的对话框。 */
interface Props {
  /** 当前已保存的脱敏来源。 */ source: CollectionSource
  /** 服务端检测到的实际能力。 */ capabilities: string[]
  /** 当前连接是否可用。 */ online: boolean
  /** 父级统一控制并发、确认和不确定状态。 */ disabled: boolean
  /** 已收到的新操作回执计数，使一次性选择失效。 */ actionVersion: number
}
/** 渐进展开的四类常用任务。 */
type WorkflowTab = 'search' | 'downloads' | 'files' | 'library'
/** 不透明目录层级，仅保留标题与编号。 */
interface DirectoryEntry {
  /** 服务端目录编号。 */ id: string
  /** 可见目录名称。 */ title: string
}
const props = defineProps<Props>()
const emit = defineEmits<{ prepare: [mutation: CollectionExternalMutation, summary: string] }>()
/** 固定工作流入口及所需能力，未知能力不能推断启用。 */
const tabs: { /** 分区编号。 */ id: WorkflowTab; /** 中文名称。 */ title: string; /** 所需能力。 */ capabilities: string[] }[] = [
  { id: 'search', title: '搜索获取', capabilities: ['integrated_search'] },
  { id: 'downloads', title: '下载控制', capabilities: ['downloads'] },
  { id: 'files', title: '整理刮削', capabilities: ['organize', 'scrape'] },
  { id: 'library', title: '媒体索引', capabilities: ['refresh_library'] },
]
const tab = ref<WorkflowTab>('search')
const busy = ref<boolean>(false)
const error = ref<string>('')
const message = ref<string>('')
const query = ref<string>('')
const mediaType = ref<'movie' | 'series'>('movie')
const media = ref<CollectionMoviePilotChoice[]>([])
const selectedMedia = ref<CollectionMoviePilotChoice | null>(null)
const releases = ref<CollectionMoviePilotChoice[]>([])
const searched = ref<boolean>(false)
const releasesLoaded = ref<boolean>(false)
const subscribeSeason = ref<number>(1)
const downloads = ref<CollectionMoviePilotChoice[]>([])
const downloadsLoaded = ref<boolean>(false)
const fileMode = ref<'organize' | 'scrape'>('organize')
const roots = ref<CollectionMoviePilotChoice[]>([])
const rootsLoaded = ref<boolean>(false)
const rootId = ref<string>('')
const targetId = ref<string>('')
const directories = ref<DirectoryEntry[]>([])
const files = ref<CollectionMoviePilotChoice[]>([])
const filesLoaded = ref<boolean>(false)
const selectedFile = ref<CollectionMoviePilotChoice | null>(null)
const fileExternalId = ref<string>('')
const fileMediaType = ref<'movie' | 'series'>('movie')
const plan = ref<CollectionMoviePilotChoice | null>(null)
const overwriteConfirmed = ref<boolean>(false)
const libraryJobs = ref<CollectionMoviePilotChoice[]>([])
const libraryLoaded = ref<boolean>(false)
let requestController: AbortController | null = null
let disposed: boolean = false
const basePath = computed<string>((): string => `/collection/sources/${encodeURIComponent(props.source.id)}/external/moviepilot`)
const visibleTabs = computed<typeof tabs>((): typeof tabs => tabs.filter((entry: typeof tabs[number]): boolean => entry.capabilities.some((capability: string): boolean => supports(capability))))
const canRead = computed<boolean>((): boolean => props.online && !busy.value && !props.disabled)
const sourceRoots = computed<CollectionMoviePilotChoice[]>((): CollectionMoviePilotChoice[] => roots.value.filter((choice: CollectionMoviePilotChoice): boolean => choice.kind === (fileMode.value === 'organize' ? 'download_root' : 'library_root')))
const libraryRoots = computed<CollectionMoviePilotChoice[]>((): CollectionMoviePilotChoice[] => roots.value.filter((choice: CollectionMoviePilotChoice): boolean => choice.kind === 'library_root' && choice.organizeAllowed === true))
const canPreview = computed<boolean>((): boolean => canRead.value && !!selectedFile.value && /^[1-9]\d{0,17}$/.test(fileExternalId.value.trim()) && (fileMode.value === 'scrape' || !!targetId.value))

/** 仅使用服务端明确声明的业务能力。 */
function supports(capability: string): boolean { return props.capabilities.includes(capability) }
/** 受控状态使用中文，未知英文不冒充成功或失败。 */
function statusLabel(status: string): string {
  const labels: Record<string, string> = { downloading: '下载中', paused: '已暂停', pausedDL: '下载已暂停', pausedUP: '做种已暂停', stopped: '已停止', stoppedDL: '下载已停止', stoppedUP: '做种已停止', queued: '等待中', running: '执行中', idle: '空闲', completed: '外部已完成', seeding: '做种中', uploading: '做种中', failed: '失败', error: '需要检查', enabled: '已启用', disabled: '已停用', ready: '已就绪', sleeping: '等待计划', unknown: '状态未知' }
  return labels[status] || (/\p{Script=Han}/u.test(status) ? status : status ? '外部状态已更新' : '未提供状态')
}
/** 暂停状态只用于简化按钮展示，服务端仍会核验目标。 */
function isPaused(status: string): boolean { return ['paused', 'pausedDL', 'pausedUP', 'stopped', 'stoppedDL', 'stoppedUP', '已暂停', '下载已暂停', '做种已暂停', '已停止', '下载已停止', '做种已停止'].includes(status) }
/** 无法确定暂停状态时保留明确的继续入口，不从外部自由文本猜测。 */
function isUnknownDownloadStatus(status: string): boolean { return ['', 'unknown', '状态未知', '外部记录'].includes(status) }
/** 取消旧读取，避免切换任务后被过期响应覆盖。 */
function cancelRead(): void { requestController?.abort(); requestController = null; busy.value = false }
/** 每个工作流步骤只有一个读取请求，错误不会转为空列表。 */
async function readChoices(path: string, method: 'GET' | 'POST', body: unknown, accept: (result: CollectionMoviePilotChoices) => void): Promise<void> {
  if (!canRead.value || disposed || document.hidden) return
  cancelRead()
  const request: AbortController = new AbortController()
  requestController = request
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    const result = await api<CollectionMoviePilotChoices>(`${basePath.value}${path}`, method, body, request.signal)
    if (disposed || request.signal.aborted || requestController !== request) return
    accept(result)
    message.value = result.message
  } catch (failure: unknown) { if (!isAbort(failure) && requestController === request) error.value = errorMessage(failure) }
  finally { if (requestController === request) { requestController = null; busy.value = false } }
}
/** 选择一个任务后才读取该任务所需信息，不创建远端操作。 */
function changeTab(next: WorkflowTab): void {
  if (busy.value || props.disabled || tab.value === next) return
  cancelRead()
  tab.value = next
  error.value = ''
  message.value = ''
  if (next === 'downloads') void loadDownloads()
  else if (next === 'files') { if (!supports('organize')) fileMode.value = 'scrape'; if (!rootsLoaded.value) void loadRoots() }
  else if (next === 'library') void loadLibrary()
}
/** 修改查询即废弃旧选择，网络搜索始终由用户主动提交。 */
function clearSearch(): void { cancelRead(); media.value = []; selectedMedia.value = null; releases.value = []; searched.value = false; releasesLoaded.value = false; error.value = ''; message.value = '' }
/** 第一步查找准确影片身份，不发出下载指令。 */
async function searchMedia(): Promise<void> {
  if (!query.value.trim() || !supports('integrated_search')) return
  media.value = []; selectedMedia.value = null; releases.value = []; searched.value = false; releasesLoaded.value = false
  await readChoices('/search', 'POST', { revision: props.source.revision, query: query.value.trim(), mediaType: mediaType.value }, (result: CollectionMoviePilotChoices): void => { media.value = result.choices.filter((choice: CollectionMoviePilotChoice): boolean => choice.kind === 'media'); searched.value = true })
}
/** 切换影片时清除上一片的资源和季号。 */
function selectMedia(choice: CollectionMoviePilotChoice): void { selectedMedia.value = choice; subscribeSeason.value = choice.season > 0 ? choice.season : 1; releases.value = []; releasesLoaded.value = false; error.value = ''; message.value = '' }
/** 第二步仅搜索选中影片的下载资源，下载另行确认。 */
async function searchReleases(): Promise<void> {
  if (!selectedMedia.value || !supports('integrated_search')) return
  releases.value = []; releasesLoaded.value = false
  await readChoices('/releases', 'POST', { revision: props.source.revision, selectionId: selectedMedia.value.id }, (result: CollectionMoviePilotChoices): void => { releases.value = result.choices.filter((choice: CollectionMoviePilotChoice): boolean => choice.kind === 'release'); releasesLoaded.value = true })
}
/** 精确媒体身份建立外部订阅，复用父级统一确认与稳定编号。 */
function subscribeSelected(): void {
  const selected: CollectionMoviePilotChoice | null = selectedMedia.value
  if (!selected || props.disabled || busy.value || !supports('add') || !selected.externalId) return
  if (selected.mediaType === 'series' && (!Number.isInteger(subscribeSeason.value) || subscribeSeason.value < 1 || subscribeSeason.value > 999)) { error.value = '请填写 1–999 的整数季号。'; return }
  emit('prepare', { kind: 'add', externalId: selected.externalId, title: selected.title, mediaType: selected.mediaType, season: selected.mediaType === 'series' ? subscribeSeason.value : 0, enabled: true }, `在 MoviePilot 订阅“${selected.title}”${selected.mediaType === 'series' ? `第 ${subscribeSeason.value} 季` : ''}并启用自动获取。后续获取由 MoviePilot 负责。`)
}
/** 已选定的单个资源在统一确认后才提交下载。 */
function download(choice: CollectionMoviePilotChoice): void {
  if (props.disabled || busy.value || choice.kind !== 'release') return
  emit('prepare', { kind: 'download', selectionId: choice.id }, `由 MoviePilot 下载已选择的资源“${choice.title}”。将使用 MoviePilot 已有下载配置；外部完成后仍需单独导入本系统。`)
}
/** 读取实际下载任务，只使用受控不透明目标。 */
async function loadDownloads(): Promise<void> { await readChoices('/downloads', 'GET', undefined, (result: CollectionMoviePilotChoices): void => { downloads.value = result.choices.filter((choice: CollectionMoviePilotChoice): boolean => choice.kind === 'download'); downloadsLoaded.value = true }) }
/** 暂停或继续仅针对当前选中的外部任务。 */
function controlDownload(choice: CollectionMoviePilotChoice, kind: 'download_start' | 'download_stop'): void {
  if (props.disabled || busy.value) return
  emit('prepare', { kind, selectionId: choice.id }, `让 MoviePilot ${kind === 'download_start' ? '继续' : '暂停'}“${choice.title}”的下载。只作用于这一项，不删除文件。操作后请刷新任务状态。`)
}
/** 读取 MoviePilot 已配置目录，不接受任意路径。 */
async function loadRoots(): Promise<void> { await readChoices('/roots', 'GET', undefined, (result: CollectionMoviePilotChoices): void => { roots.value = result.choices.filter((choice: CollectionMoviePilotChoice): boolean => choice.kind === 'download_root' || choice.kind === 'library_root'); rootsLoaded.value = true; rootId.value = ''; targetId.value = ''; directories.value = []; files.value = []; filesLoaded.value = false; selectedFile.value = null; invalidatePlan() }) }
/** 改变操作范围时清除选择和预览，防止误用上一份计划。 */
function invalidatePlan(): void { plan.value = null; overwriteConfirmed.value = false }
/** 切换整理或刮削时重新选择各自授权的来源根目录。 */
function changeFileMode(): void { cancelRead(); rootId.value = ''; targetId.value = ''; directories.value = []; files.value = []; filesLoaded.value = false; selectedFile.value = null; invalidatePlan(); error.value = ''; message.value = '' }
/** 从根目录重新读取，目录编号从未转化为客户端文件路径。 */
async function rootChanged(): Promise<void> { directories.value = []; files.value = []; filesLoaded.value = false; selectedFile.value = null; invalidatePlan(); if (rootId.value) await loadFiles() }
/** 读取固定根以内的一层目录，不递归遍历。 */
async function loadFiles(): Promise<void> {
  if (!rootId.value) return
  const parent: DirectoryEntry | undefined = directories.value[directories.value.length - 1]
  await readChoices('/files', 'POST', { revision: props.source.revision, rootId: rootId.value, ...(parent ? { parentId: parent.id } : {}) }, (result: CollectionMoviePilotChoices): void => { files.value = result.choices.filter((choice: CollectionMoviePilotChoice): boolean => choice.kind === 'dir' || choice.kind === 'file'); filesLoaded.value = true })
}
/** 打开子目录前撤销旧文件与计划。 */
async function openDirectory(choice: CollectionMoviePilotChoice): Promise<void> { directories.value.push({ id: choice.id, title: choice.title }); selectedFile.value = null; files.value = []; filesLoaded.value = false; invalidatePlan(); await loadFiles() }
/** 返回上级目录仍由服务端核验当前根边界。 */
async function parentDirectory(): Promise<void> { directories.value.pop(); selectedFile.value = null; files.value = []; filesLoaded.value = false; invalidatePlan(); await loadFiles() }
/** 明确选择单个文件，禁止沿用另一文件的整理或刮削预览。 */
function selectFile(choice: CollectionMoviePilotChoice): void { selectedFile.value = choice; invalidatePlan() }
/** 先生成只读计划；此步骤不会移动文件或覆盖元数据。 */
async function previewFile(): Promise<void> {
  if (!canPreview.value || !selectedFile.value) return
  invalidatePlan()
  await readChoices(`/${fileMode.value}/preview`, 'POST', { revision: props.source.revision, fileId: selectedFile.value.id, ...(fileMode.value === 'organize' ? { targetId: targetId.value } : {}), externalId: fileExternalId.value.trim(), mediaType: fileMediaType.value }, (result: CollectionMoviePilotChoices): void => { plan.value = result.choices.find((choice: CollectionMoviePilotChoice): boolean => choice.kind === (fileMode.value === 'organize' ? 'organize_plan' : 'scrape_plan')) || null; if (!plan.value) error.value = '未取得有效计划，请检查配置后重新预览。' })
}
/** 确认具体计划，刮削覆盖需要独立主动勾选。 */
function prepareFile(): void {
  if (!plan.value || props.disabled || busy.value || (fileMode.value === 'scrape' && !overwriteConfirmed.value)) return
  if (fileMode.value === 'organize') emit('prepare', { kind: 'organize', selectionId: plan.value.id }, `由 MoviePilot 按预览复制整理“${selectedFile.value?.title || plan.value.title}”。仅使用已配置为不覆盖的目标目录，执行前再次核验策略，原文件保留；这不会扫描登记到本系统。`)
  else emit('prepare', { kind: 'scrape', selectionId: plan.value.id, confirmOverwrite: true }, `由 MoviePilot 为“${selectedFile.value?.title || plan.value.title}”刮削元数据。你已确认可能覆盖该文件对应的 NFO 和图片；不会改写视频内容。`)
}
/** 读取固定媒体服务器同步任务及状态，不触发扫盘。 */
async function loadLibrary(): Promise<void> { await readChoices('/library', 'GET', undefined, (result: CollectionMoviePilotChoices): void => { libraryJobs.value = result.choices.filter((choice: CollectionMoviePilotChoice): boolean => choice.kind === 'library_job'); libraryLoaded.value = true }) }
/** 索引同步必须明确确认，不夸大为媒体服务器文件系统扫描。 */
function refreshLibrary(choice: CollectionMoviePilotChoice): void {
  if (props.disabled || busy.value) return
  emit('prepare', { kind: 'refresh_library', selectionId: choice.id }, '刷新 MoviePilot 媒体库索引：从已配置的媒体服务器同步索引。这不会触发 Emby/Plex 文件系统扫盘，也不会扫描本系统媒体库。')
}
/** 页面隐藏后取消读取，返回时保留显式重新读取入口。 */
function visibilityChanged(): void { if (document.hidden) cancelRead() }
watch([fileExternalId, fileMediaType, targetId], invalidatePlan)
watch((): number => props.actionVersion, (): void => {
  // 一次性选择已交给服务端，后续操作必须重新读取新选择，避免误用消费过的编号。
  releases.value = []; releasesLoaded.value = false; downloads.value = []; downloadsLoaded.value = false; libraryJobs.value = []; libraryLoaded.value = false; invalidatePlan()
  message.value = '操作已记录。需要继续同类操作时，请重新读取资源、任务或计划。'
})
onMounted((): void => { document.addEventListener('visibilitychange', visibilityChanged); const initial: typeof tabs[number] | undefined = visibleTabs.value[0]; if (initial) { tab.value = initial.id; if (initial.id === 'downloads') void loadDownloads(); else if (initial.id === 'files') { if (!supports('organize')) fileMode.value = 'scrape'; void loadRoots() } else if (initial.id === 'library') void loadLibrary() } })
onBeforeUnmount((): void => { disposed = true; cancelRead(); document.removeEventListener('visibilitychange', visibilityChanged) })
</script>

<template>
  <section class="collection-mp-workflow" aria-label="MoviePilot 工作流">
    <div class="collection-mp-tools" aria-label="选择 MoviePilot 任务"><button v-for="entry in visibleTabs" :key="entry.id" class="secondary-button" :class="{ active: tab === entry.id }" :aria-pressed="tab === entry.id" :disabled="busy || disabled" @click="changeTab(entry.id)">{{ entry.title }}</button></div>
    <p v-if="!online" class="collection-warning">连接当前不可用。请先“检测并刷新”，再读取或操作。</p>
    <p v-if="busy" class="collection-loading" role="status"><span class="loader"></span>正在读取 MoviePilot…</p>
    <p v-if="error" class="error-box" role="alert">{{ error }}。请检查连接或重新读取；旧记录仅供参考。</p>
    <p v-if="message" class="section-note">{{ message }}</p>

    <template v-if="tab === 'search'">
      <form class="collection-external-fields" @submit.prevent="searchMedia"><label>想看什么？<div class="collection-external-search"><input v-model="query" maxlength="160" required placeholder="输入影片或剧集名称" :disabled="disabled" @input="clearSearch" /><button class="primary-button" :disabled="!canRead || !query.trim()">搜索</button></div></label><label class="collection-inline-label">媒体类型<select v-model="mediaType" :disabled="busy || disabled" @change="clearSearch"><option value="movie">电影</option><option value="series">电视剧</option></select></label><p class="field-hint">先选准确影片，再查下载资源。搜索不会自动下载。</p></form>
      <div v-if="media.length" class="collection-external-choices"><button v-for="choice in media" :key="choice.id" class="collection-external-choice" :class="{ active: selectedMedia?.id === choice.id }" :aria-pressed="selectedMedia?.id === choice.id" :disabled="busy || disabled" @click="selectMedia(choice)"><strong>{{ choice.title }}</strong><span class="field-hint">{{ choice.year || '年份未知' }} · {{ choice.mediaType === 'series' ? '电视剧' : '电影' }}{{ selectedMedia?.id === choice.id ? ' · 已选择' : '' }}</span></button></div>
      <p v-else-if="searched && !busy && !error" class="section-note">没有找到影片，请调整名称或媒体类型后重新搜索。</p>
      <div v-if="selectedMedia" class="collection-callout collection-external-fields"><strong>{{ selectedMedia.title }}</strong><div class="collection-actions"><button class="primary-button" :disabled="!canRead" @click="searchReleases">查找下载资源</button><button v-if="supports('add')" class="secondary-button" :disabled="!canRead || !selectedMedia.externalId" @click="subscribeSelected">订阅此片</button></div><label v-if="selectedMedia.mediaType === 'series' && supports('add')" class="collection-inline-label">订阅季号<input v-model.number="subscribeSeason" type="number" min="1" max="999" :disabled="busy || disabled" /></label><p class="field-hint">订阅交由 MoviePilot 持续获取。查找资源会访问其已配置站点，仍需逐条确认下载。</p></div>
      <div v-if="releases.length" class="collection-records"><article v-for="choice in releases" :key="choice.id" class="collection-preview-row"><strong>{{ choice.title }}</strong><p class="field-hint">{{ statusLabel(choice.status) }}</p><button class="secondary-button" :disabled="!canRead" @click="download(choice)">选择并确认下载</button></article></div>
      <p v-else-if="releasesLoaded && !busy && !error" class="section-note">当前没有可选下载资源。可稍后重试，或在 MoviePilot 检查站点配置。</p>
    </template>

    <template v-else-if="tab === 'downloads'">
      <div class="collection-section-heading"><h3>MoviePilot 下载任务</h3><button class="secondary-button" :disabled="!canRead" @click="loadDownloads"><Icon name="refresh" />刷新任务</button></div><p class="field-hint">暂停和继续由 MoviePilot 执行。完成状态不等于已经导入本系统。</p>
      <div v-if="downloads.length" class="collection-records"><article v-for="choice in downloads" :key="choice.id" class="collection-preview-row"><div class="collection-section-heading"><strong>{{ choice.title }}</strong><span class="status-pill">{{ statusLabel(choice.status) }}</span></div><div class="collection-actions"><button class="secondary-button" :disabled="!canRead" @click="controlDownload(choice, isPaused(choice.status) ? 'download_start' : 'download_stop')">{{ isPaused(choice.status) ? '继续此任务' : '暂停此任务' }}</button><button v-if="isUnknownDownloadStatus(choice.status)" class="text-button" :disabled="!canRead" @click="controlDownload(choice, 'download_start')">继续此任务</button></div></article></div>
      <p v-else-if="downloadsLoaded && !busy && !error" class="section-note">当前没有下载任务。</p>
    </template>

    <template v-else-if="tab === 'files'">
      <label>要做什么？<select v-model="fileMode" :disabled="busy || disabled" @change="changeFileMode"><option v-if="supports('organize')" value="organize">复制整理到媒体目录</option><option v-if="supports('scrape')" value="scrape">刮削已有媒体元数据</option></select></label><p class="field-hint">{{ fileMode === 'organize' ? '从 MoviePilot 已配置下载目录选择文件。仅使用已配置为不覆盖的目标目录；复制后原文件保留。' : '仅从 MoviePilot 已配置媒体目录选择文件；刮削可能覆盖 NFO 和图片，需要明确确认。' }}</p>
      <button v-if="!rootsLoaded || error" class="secondary-button collection-self-start" :disabled="!canRead" @click="loadRoots">读取已配置目录</button>
      <template v-if="rootsLoaded"><div class="collection-field-grid"><label>{{ fileMode === 'organize' ? '选择下载目录' : '选择媒体目录' }}<select v-model="rootId" :disabled="!canRead" @change="rootChanged"><option value="" disabled>请选择</option><option v-for="choice in sourceRoots" :key="choice.id" :value="choice.id">{{ choice.title }}</option></select></label><label v-if="fileMode === 'organize'">整理到媒体目录<select v-model="targetId" :disabled="!canRead"><option value="" disabled>请选择</option><option v-for="choice in libraryRoots" :key="choice.id" :value="choice.id">{{ choice.title }}</option></select><span class="field-hint">仅显示已配置为不覆盖的目标目录。</span></label></div><p v-if="!sourceRoots.length" class="collection-warning">MoviePilot 尚未提供所需来源目录，请先在其中完成目录配置。</p><p v-if="fileMode === 'organize' && !libraryRoots.length" class="collection-warning">没有可用目标。请先在 MoviePilot 将媒体目录覆盖策略设置为“不覆盖”，再读取已配置目录。</p></template>
      <div v-if="rootId" class="collection-callout collection-external-fields"><div class="collection-section-heading"><strong>{{ directories[directories.length - 1]?.title || '当前根目录' }}</strong><div class="collection-actions"><button v-if="directories.length" class="text-button" :disabled="!canRead" @click="parentDirectory">返回上级</button><button class="text-button" :disabled="!canRead" @click="loadFiles">刷新</button></div></div><div v-if="files.length" class="collection-external-choices"><button v-for="choice in files" :key="choice.id" class="collection-external-choice" :class="{ active: selectedFile?.id === choice.id }" :disabled="!canRead" :aria-pressed="choice.kind === 'file' ? selectedFile?.id === choice.id : undefined" @click="choice.kind === 'dir' ? openDirectory(choice) : selectFile(choice)"><strong>{{ choice.kind === 'dir' ? '目录 · ' : '文件 · ' }}{{ choice.title }}</strong><span class="field-hint">{{ choice.kind === 'dir' ? '打开目录' : selectedFile?.id === choice.id ? '已选择' : '选择此文件' }}</span></button></div><p v-else-if="filesLoaded && !busy && !error" class="field-hint">此目录没有可选文件。</p></div>
      <form v-if="selectedFile" class="collection-external-fields" @submit.prevent="previewFile"><strong>已选：{{ selectedFile.title }}</strong><div class="collection-field-grid"><label>对应 TMDB 编号<input v-model="fileExternalId" required inputmode="numeric" pattern="[1-9][0-9]*" maxlength="18" :disabled="busy || disabled" placeholder="准确匹配此文件的影片编号" /></label><label>媒体类型<select v-model="fileMediaType" :disabled="busy || disabled"><option value="movie">电影</option><option value="series">电视剧</option></select></label></div><button class="secondary-button collection-self-start" :disabled="!canPreview">先预览{{ fileMode === 'organize' ? '整理' : '刮削' }}计划</button></form>
      <div v-if="plan" class="collection-callout collection-external-fields"><strong>{{ plan.title }}</strong><p class="field-hint">{{ fileMode === 'organize' ? '将复制所选文件到已配置为不覆盖的目标目录；执行前再次核验策略，源文件保留。' : '将更新所选媒体文件对应的元数据；已有 NFO 和图片可能被覆盖。' }}</p><label v-if="fileMode === 'scrape'" class="collection-check"><input v-model="overwriteConfirmed" type="checkbox" :disabled="busy || disabled" /><span>我确认覆盖此文件已有的 NFO 和图片元数据</span></label><button class="primary-button collection-self-start" :disabled="!canRead || (fileMode === 'scrape' && !overwriteConfirmed)" @click="prepareFile">继续确认{{ fileMode === 'organize' ? '复制整理' : '刮削覆盖' }}</button></div>
    </template>

    <template v-else-if="tab === 'library'">
      <div class="collection-section-heading"><h3>MoviePilot 媒体库索引</h3><button class="secondary-button" :disabled="!canRead" @click="loadLibrary">读取索引任务状态</button></div><p class="field-hint">从已配置媒体服务器同步索引到 MoviePilot，不会触发 Emby/Plex 文件系统扫盘，也不会扫描本系统媒体库。</p><div v-if="libraryJobs.length" class="collection-records"><article v-for="choice in libraryJobs" :key="choice.id" class="collection-preview-row"><div class="collection-section-heading"><strong>{{ choice.title }}</strong><span class="status-pill">{{ statusLabel(choice.status) }}</span></div><button class="primary-button" :disabled="!canRead" @click="refreshLibrary(choice)">刷新 MoviePilot 媒体库索引</button></article></div><p v-else-if="libraryLoaded && !busy && !error" class="section-note">未取得可用同步任务，请检查 MoviePilot 媒体服务器配置。</p>
    </template>
  </section>
</template>
