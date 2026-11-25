<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, ApiError, createRequestId, isAbort } from '../api'
import { collectionBatchLimit, collectionTime } from '../collection'
import type { CollectionCapabilities, CollectionJMComicChapter, CollectionJMComicItem, CollectionJMComicOrder, CollectionJMComicPage, CollectionJMComicTime, CollectionPreview, CollectionSource, CollectionTask, Library } from '../types'
import CollectionTargetFields from './CollectionTargetFields.vue'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

/** 漫画入口只使用已保存来源与现有媒体库。 */
interface Props {
  /** 来源快照。 */ sources: CollectionSource[]
  /** 现有媒体库。 */ libraries: Library[]
  /** 实际执行端能力。 */ capabilities: CollectionCapabilities | null
  /** 优先选择的来源。 */ initialSourceId?: string
}
/** 结果不确定时保留原始提交体，避免重复下载。 */
interface TaskInput {
  /** 持久幂等编号。 */ requestId: string
  /** 已确认预览。 */ previewId: string
  /** 固定漫画分类。 */ contentType: 'comic'
  /** 是否创建独立媒体库。 */ createLibrary: boolean
  /** 现有目标库。 */ libraryId: number
}
/** 批量条目独立保存预览、提交身份和实际受理结果。 */
interface BatchRow {
  /** 规范漫画编号。 */ albumId: string
  /** 当前绑定的全本预览。 */ preview: CollectionPreview | null
  /** 当前条目的处理状态。 */ state: 'waiting' | 'previewing' | 'ready' | 'preview_failed' | 'submitting' | 'submitted' | 'submit_failed'
  /** 此条目的安全错误说明。 */ error: string
  /** 首次提交后冻结的完整请求体。 */ input: TaskInput | null
  /** 已经确认受理的持久任务。 */ task: CollectionTask | null
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; created: [task: CollectionTask]; 'open-task': [task: CollectionTask] }>()
const sources = computed<CollectionSource[]>((): CollectionSource[] => props.sources.filter((source: CollectionSource): boolean => source.kind === 'jmcomic' && source.enabled))
const sourceId = ref<string>(sources.value.find((source: CollectionSource): boolean => source.id === props.initialSourceId)?.id || sources.value[0]?.id || '')
const mode = ref<'single' | 'batch'>('single')
const query = ref<string>('')
const orderBy = ref<CollectionJMComicOrder>('mr')
const time = ref<CollectionJMComicTime>('a')
const result = ref<CollectionJMComicPage | null>(null)
const searchError = ref<string>('')
const searching = ref<boolean>(false)
const albumId = ref<string>('')
const preview = ref<CollectionPreview | null>(null)
const chapterIds = ref<string[]>([])
const batchText = ref<string>('')
const rows = ref<BatchRow[]>([])
const busy = ref<boolean>(false)
const error = ref<string>('')
const createLibrary = ref<boolean>(true)
const libraryId = ref<number>(0)
const input = ref<TaskInput | null>(null)
const now = ref<number>(Date.now())
const controller: AbortController = new AbortController()
let searchController: AbortController | null = null
const locked = computed<boolean>((): boolean => input.value !== null || rows.value.some((row: BatchRow): boolean => row.input !== null))
/** 每秒只更新预览失效判断，关闭弹窗时释放计时器。 */
const expiryTimer: ReturnType<typeof setInterval> = setInterval((): void => { now.value = Date.now() }, 1000)
const expired = computed<boolean>((): boolean => !!preview.value && previewExpired(preview.value))
const chapterSelectionChanged = computed<boolean>((): boolean => !!preview.value?.jmcomic && JSON.stringify(orderedChapterIds()) !== JSON.stringify(preview.value.jmcomic.selectedChapterIds))
const noChapters = computed<boolean>((): boolean => !!preview.value?.jmcomic && chapterIds.value.length === 0)
const targetMissing = computed<boolean>((): boolean => !createLibrary.value && !libraryId.value)
const batchSubmittable = computed<BatchRow[]>((): BatchRow[] => rows.value.filter(canSubmitRow))
const batchSubmitted = computed<number>((): number => rows.value.filter((row: BatchRow): boolean => row.state === 'submitted').length)
const unavailableReason = computed<string>((): string => {
  if (!sourceId.value) return '还没有启用的 JMComic 来源，请先添加或启用漫画来源。'
  if (!props.capabilities) return '平台能力尚未读取成功，请关闭弹窗并刷新状态。'
  if (!props.capabilities.workerReady) return props.capabilities.workerReason || '获取执行端尚未连接。'
  const capability = props.capabilities.capabilities.find((entry): boolean => entry.kind === 'jmcomic')
  return capability?.available ? '' : capability?.reason || '执行端尚未启用或安装 JMComic 插件，请按部署文档配置后重启执行端。'
})

/** 停止旧查询并隔离迟到响应，保留用户已经读取的结果。 */
function stopSearch(): void {
  searchController?.abort()
  searchController = null
  searching.value = false
}
/** 查询条件改变时清除旧结果，不将失败显示为空结果。 */
function clearSearch(): void {
  stopSearch()
  result.value = null
  searchError.value = ''
}
/** 保留服务端中文错误，为浏览器网络或解码异常提供本入口的中文恢复提示。 */
function comicErrorMessage(failure: unknown): string {
  return failure instanceof ApiError ? failure.message : '网络连接中断或响应未能读取，请检查连接后重试。'
}
/** 按本地时钟禁用已失效的新提交，不改变未知提交的重试身份。 */
function previewExpired(value: CollectionPreview): boolean { return !Number.isFinite(Date.parse(value.expiresAt)) || Date.parse(value.expiresAt) <= now.value }
/** 按服务端目录顺序规范选择，忽略用户点击先后。 */
function orderedChapterIds(): string[] {
  const selected: Set<string> = new Set<string>(chapterIds.value)
  return preview.value?.jmcomic?.chapters.filter((chapter: CollectionJMComicChapter): boolean => selected.has(chapter.id)).map((chapter: CollectionJMComicChapter): string => chapter.id) || []
}
/** 改变漫画编号时旧预览不能再被确认。 */
function invalidatePreview(): void { if (!locked.value) { preview.value = null; chapterIds.value = []; error.value = '' } }
/** 模式切换只停止旧查询，已确认结果留在各自面板中。 */
function changeMode(value: 'single' | 'batch'): void { if (!busy.value && !locked.value) { mode.value = value; stopSearch(); error.value = '' } }
/** 全选与清空仅修改待确认选择，必须重新生成预览。 */
function chooseChapters(all: boolean): void {
  if (busy.value || locked.value) return
  chapterIds.value = all ? preview.value?.jmcomic?.chapters.map((chapter: CollectionJMComicChapter): string => chapter.id) || [] : []
}
/** 查询一页结果；重复搜索会中止前一请求。 */
async function search(page: number = 1): Promise<void> {
  if (busy.value || locked.value || unavailableReason.value) return
  const value: string = query.value.trim()
  if (!value || Array.from(value).length > 120 || /[\u0000-\u001f\u007f-\u009f]/.test(value)) { searchError.value = '请输入 1–120 个字符的关键词或漫画编号。'; return }
  clearSearch()
  const request: AbortController = new AbortController()
  searchController = request
  searching.value = true
  try {
    const response = await api<CollectionJMComicPage>(`/collection/sources/${encodeURIComponent(sourceId.value)}/jmcomic/search`, 'POST', { query: value, page, orderBy: orderBy.value, time: time.value }, request.signal)
    if (request.signal.aborted || searchController !== request) return
    result.value = response
  } catch (failure: unknown) {
    if (!request.signal.aborted && searchController === request && !isAbort(failure)) searchError.value = comicErrorMessage(failure)
  } finally {
    if (searchController === request) { searchController = null; searching.value = false }
  }
}
/** 点击结果仍先读取服务端预览，再由用户确认下载。 */
async function selectAlbum(item: CollectionJMComicItem): Promise<void> {
  if (busy.value || locked.value) return
  albumId.value = item.albumId
  invalidatePreview()
  await loadPreview()
}
/** 编号通过固定漫画适配器解析，不接受链接或任意命令参数。 */
async function loadPreview(): Promise<void> {
  if (busy.value || locked.value || unavailableReason.value) return
  if (!/^[1-9][0-9]{0,11}$/.test(albumId.value)) { error.value = '请输入有效的正整数漫画编号。'; return }
  if (noChapters.value) { error.value = '请至少选择一个章节后更新预览。'; return }
  const selected: string[] = orderedChapterIds()
  const chapterSelection: string[] | undefined = preview.value?.jmcomic && selected.length < preview.value.jmcomic.chapters.length ? selected : undefined
  busy.value = true
  error.value = ''
  stopSearch()
  try {
    const response = await api<{ preview: CollectionPreview }>('/collection/previews', 'POST', { sourceId: sourceId.value, albumId: albumId.value, ...(chapterSelection ? { chapterIds: chapterSelection } : {}) }, controller.signal)
    if (!controller.signal.aborted) { preview.value = response.preview; chapterIds.value = [...(response.preview.jmcomic?.selectedChapterIds || [])] }
  } catch (failure: unknown) { if (!isAbort(failure)) error.value = comicErrorMessage(failure) }
  finally { busy.value = false }
}
/** 下载与入库沿用原任务协议，未知结果始终复用同一请求。 */
async function submit(): Promise<void> {
  if (busy.value || !preview.value || (!input.value && (expired.value || unavailableReason.value || chapterSelectionChanged.value || noChapters.value || preview.value.jmcomic?.overBudget))) return
  if (!input.value && !createLibrary.value && !libraryId.value) { error.value = '请选择漫画库或创建独立媒体库。'; return }
  if (!input.value) input.value = { requestId: createRequestId(), previewId: preview.value.id, contentType: 'comic', createLibrary: createLibrary.value, libraryId: createLibrary.value ? 0 : libraryId.value }
  busy.value = true
  error.value = ''
  clearSearch()
  try {
    const response = await api<{ task: CollectionTask }>('/collection/tasks', 'POST', input.value, controller.signal)
    if (controller.signal.aborted) return
    console.info('漫画收集任务已受理')
    emit('created', response.task)
    emit('open-task', response.task)
  } catch (failure: unknown) { if (!isAbort(failure)) error.value = comicErrorMessage(failure) }
  finally { busy.value = false }
}
/** 编号批次只接受有界的不同正整数，不静默丢弃重复输入。 */
function parseBatch(): string[] | null {
  const values: string[] = batchText.value.trim().split(/[\s,，;；]+/).filter((value: string): boolean => !!value)
  if (!values.length || values.length > collectionBatchLimit) { error.value = `每批请输入 1–${collectionBatchLimit} 个漫画编号。`; return null }
  if (values.some((value: string): boolean => !/^[1-9][0-9]{0,11}$/.test(value))) { error.value = '批量编号只能是最多 12 位的正整数，请使用空格、换行或逗号分隔。'; return null }
  if (new Set<string>(values).size !== values.length) { error.value = '批次中存在重复漫画编号，请删除重复项后再预览。'; return null }
  return values
}
/** 单本预览失败不影响其他结果，已开始提交的条目不能换预览。 */
async function readBatchPreview(row: BatchRow): Promise<void> {
  if (row.input || controller.signal.aborted) return
  row.state = 'previewing'
  row.error = ''
  try {
    const response = await api<{ preview: CollectionPreview }>('/collection/previews', 'POST', { sourceId: sourceId.value, albumId: row.albumId }, controller.signal)
    if (controller.signal.aborted) return
    row.preview = response.preview
    row.state = 'ready'
  } catch (failure: unknown) { if (!isAbort(failure)) { row.state = 'preview_failed'; row.error = comicErrorMessage(failure) } }
}
/** 顺序读取新条目或失效条目，保留已成功的有效预览。 */
async function previewBatch(): Promise<void> {
  if (busy.value || locked.value || unavailableReason.value) return
  const values: string[] | null = parseBatch()
  if (!values) return
  rows.value = values.map((value: string): BatchRow => rows.value.find((row: BatchRow): boolean => row.albumId === value) || { albumId: value, preview: null, state: 'waiting', error: '', input: null, task: null })
  busy.value = true
  error.value = ''
  stopSearch()
  try {
    for (const row of rows.value) {
      if (controller.signal.aborted) break
      if (!row.preview || previewExpired(row.preview) || row.state === 'preview_failed') await readBatchPreview(row)
    }
  } finally { busy.value = false }
}
/** 显式重读尚未提交的一本，不影响同批已受理任务。 */
async function retryBatchPreview(row: BatchRow): Promise<void> {
  if (busy.value || row.input || unavailableReason.value) return
  busy.value = true
  try { await readBatchPreview(row) } finally { busy.value = false }
}
/** 未知响应可以在原预览过期后核对原请求，已受理条目永不重发。 */
function canSubmitRow(row: BatchRow): boolean {
  return !!row.preview && ['ready', 'submit_failed'].includes(row.state) && (!!row.input || (!previewExpired(row.preview) && !row.preview.jmcomic?.overBudget && !unavailableReason.value))
}
/** 逐本持久化提交身份，部分失败时只保留未受理条目的原请求重试。 */
async function submitBatch(): Promise<void> {
  if (busy.value || !batchSubmittable.value.length) return
  if (targetMissing.value && batchSubmittable.value.some((row: BatchRow): boolean => !row.input)) { error.value = '请选择漫画库或创建独立媒体库。'; return }
  busy.value = true
  error.value = ''
  clearSearch()
  try {
    for (const row of rows.value) {
      if (controller.signal.aborted) break
      const confirmedPreview: CollectionPreview | null = row.preview
      if (!confirmedPreview || !canSubmitRow(row)) continue
      // 一经发送立即冻结完整请求，网络断开也不能生成新的幂等编号。
      if (!row.input) row.input = { requestId: createRequestId(), previewId: confirmedPreview.id, contentType: 'comic', createLibrary: createLibrary.value, libraryId: createLibrary.value ? 0 : libraryId.value }
      row.state = 'submitting'
      row.error = ''
      try {
        const response = await api<{ task: CollectionTask }>('/collection/tasks', 'POST', row.input, controller.signal)
        if (controller.signal.aborted) return
        row.task = response.task
        row.state = 'submitted'
        emit('created', response.task)
        console.info('批量漫画收集条目已受理')
      } catch (failure: unknown) { if (!isAbort(failure)) { row.state = 'submit_failed'; row.error = comicErrorMessage(failure) } }
    }
  } finally { busy.value = false }
}
/** 每一行都显示自己的预览与受理事实，不将部分受理表述为全部成功。 */
function batchStateLabel(row: BatchRow): string {
  if (row.state === 'ready' && row.preview?.jmcomic?.overBudget) return '超出页数预算，请切换单本选章'
  if (row.state === 'ready' && row.preview && previewExpired(row.preview)) return '预览已过期，请重新预览此本'
  const labels: Record<BatchRow['state'], string> = { waiting: '等待预览', previewing: '正在预览…', ready: '预览就绪，待确认', preview_failed: '预览失败', submitting: '正在提交…', submitted: '任务已受理', submit_failed: '提交未确认，可重试原请求' }
  return labels[row.state]
}
watch([query, orderBy, time], clearSearch)
watch(albumId, invalidatePreview, { flush: 'sync' })
watch(batchText, (): void => { if (!locked.value) { rows.value = []; error.value = '' } }, { flush: 'sync' })
watch(sourceId, (): void => { clearSearch(); albumId.value = ''; preview.value = null; chapterIds.value = []; rows.value = []; error.value = '' }, { flush: 'sync' })
onBeforeUnmount((): void => { clearSearch(); controller.abort(); clearInterval(expiryTimer) })
</script>

<template>
  <DialogShell title="JMComic 漫画搜索与下载" @close="emit('close')">
    <div class="collection-form">
      <p class="muted">搜索后预览漫画，确认章节和目标漫画库；也可以批量填写漫画编号。</p>
      <label for="jmcomic-source">漫画来源<select id="jmcomic-source" v-model="sourceId" :disabled="busy || locked"><option value="" disabled>选择已启用的漫画来源</option><option v-for="source in sources" :key="source.id" :value="source.id">{{ source.name }}</option></select></label>
      <p v-if="unavailableReason" class="error-box" role="alert">{{ unavailableReason }}</p>
      <div class="collection-tabs" role="group" aria-label="漫画下载方式">
        <button type="button" :class="{ active: mode === 'single' }" :aria-pressed="mode === 'single'" :disabled="busy || locked" @click="changeMode('single')">搜索与选章</button>
        <button type="button" :class="{ active: mode === 'batch' }" :aria-pressed="mode === 'batch'" :disabled="busy || locked" @click="changeMode('batch')">批量编号</button>
      </div>
      <template v-if="mode === 'single'">
        <form class="jmcomic-search" @submit.prevent="search()">
          <label for="jmcomic-query">关键词或漫画编号<input id="jmcomic-query" v-model="query" autofocus maxlength="120" :disabled="busy || locked" placeholder="输入关键词或编号" autocomplete="off" /></label>
          <button class="secondary-button" type="submit" :disabled="busy || locked || !query.trim() || !!unavailableReason"><Icon name="search" />{{ searching ? '重新搜索' : '搜索漫画' }}</button>
        </form>
        <details class="collection-advanced">
          <summary>高级搜索筛选</summary>
          <div class="collection-advanced-fields">
            <div class="collection-field-grid">
              <label for="jmcomic-order">排序方式<select id="jmcomic-order" v-model="orderBy" :disabled="busy || locked"><option value="mr">最新更新</option><option value="mv">浏览最多</option><option value="mp">图片最多</option><option value="tf">喜欢最多</option></select></label>
              <label for="jmcomic-time">时间范围<select id="jmcomic-time" v-model="time" :disabled="busy || locked"><option value="a">全部时间</option><option value="t">今天</option><option value="w">本周</option><option value="m">本月</option></select></label>
            </div>
            <p class="field-hint">筛选由漫画来源执行，修改后请重新搜索。</p>
          </div>
        </details>
        <p v-if="searching" role="status" class="field-hint">正在搜索漫画…</p>
        <p v-if="searchError" class="error-box" role="alert">{{ searchError }}</p>
        <template v-if="result">
          <div class="collection-section-heading"><h3>搜索结果</h3><span class="field-hint">第 {{ result.page }} 页 · 共 {{ result.total }} 项</span></div>
          <p v-if="!result.items.length" class="section-note" role="status">没有找到匹配的漫画，请更换关键词。</p>
          <div v-else class="jmcomic-results">
            <article v-for="item in result.items" :key="item.albumId" class="collection-preview-row">
              <strong>{{ item.title }}</strong><p class="field-hint">编号 {{ item.albumId }}<span v-if="item.tags.length"> · {{ item.tags.join('、') }}</span></p>
              <button class="text-button collection-self-start" type="button" :disabled="busy || locked" @click="selectAlbum(item)">选择并预览<Icon name="arrow" :size="16" /></button>
            </article>
          </div>
          <div class="collection-pagination"><button class="secondary-button" type="button" :disabled="busy || locked || searching || result.page <= 1" @click="search(result.page - 1)">上一页</button><span>第 {{ result.page }} 页</span><button class="secondary-button" type="button" :disabled="busy || locked || searching || !result.hasNext || result.page >= 1000" @click="search(result.page + 1)">下一页</button></div>
        </template>
        <form class="jmcomic-search" @submit.prevent="loadPreview">
          <label for="jmcomic-album">漫画编号<input id="jmcomic-album" v-model="albumId" inputmode="numeric" pattern="[1-9][0-9]{0,11}" maxlength="12" required :disabled="busy || locked" placeholder="仅填写编号" aria-describedby="jmcomic-album-hint" /><span id="jmcomic-album-hint" class="field-hint">下载前先确认标题、章节与目标漫画库。</span></label>
          <button class="secondary-button" type="submit" :disabled="busy || locked || !albumId || noChapters || !!unavailableReason">{{ busy && !input ? '正在预览…' : chapterSelectionChanged ? '更新章节预览' : '预览漫画' }}</button>
        </form>
        <template v-if="preview">
          <article class="collection-preview-row" aria-label="漫画下载预览">
            <strong>{{ preview.title }}</strong><p class="field-hint">漫画编号 {{ albumId }} · {{ expired && !input ? '预览已过期，请重新预览' : `有效至 ${collectionTime(preview.expiresAt)}` }}</p>
            <p v-if="preview.jmcomic" class="field-hint">本次绑定 {{ preview.jmcomic.selectedChapterIds.length }} 个章节 · {{ preview.jmcomic.pageCount }} 页</p>
            <p class="field-hint">实际文件数量和大小在获取后校验，完成发布后才会扫描入库。</p>
            <p v-for="(warning, index) in preview.warnings" :key="index" class="collection-warning">{{ warning }}</p>
            <p v-if="preview.jmcomic?.overBudget" class="collection-warning" role="status">所选页数超过本次任务预算，请减少章节并更新章节预览。</p>
          </article>
          <fieldset v-if="preview.jmcomic" class="jmcomic-chapters" :disabled="busy || locked">
            <legend>下载章节</legend>
            <div class="collection-actions"><button class="text-button" type="button" @click="chooseChapters(true)">全选章节</button><button class="text-button" type="button" @click="chooseChapters(false)">清空选择</button><span class="field-hint">已选 {{ chapterIds.length }} / {{ preview.jmcomic.chapters.length }} 个章节</span></div>
            <div class="jmcomic-chapter-list">
              <label v-for="chapter in preview.jmcomic.chapters" :key="chapter.id" class="collection-check"><input v-model="chapterIds" type="checkbox" :value="chapter.id" /><span>{{ chapter.title || `章节 ${chapter.id}` }}<span class="field-hint">编号 {{ chapter.id }} · {{ chapter.pageCount }} 页</span></span></label>
            </div>
            <p v-if="noChapters" class="collection-warning" role="status">请至少选择一个章节。</p>
            <p v-else-if="chapterSelectionChanged" class="collection-warning" role="status">章节选择已变更，请点击“更新章节预览”后再确认下载。</p>
            <p v-else class="field-hint">章节选择已绑定当前预览；全选表示下载全本。</p>
          </fieldset>
        </template>
      </template>
      <template v-else>
        <label for="jmcomic-batch">批量漫画编号<textarea id="jmcomic-batch" v-model="batchText" rows="4" maxlength="1500" :disabled="busy || locked" placeholder="每行一个漫画编号，也可用空格或逗号分隔" aria-describedby="jmcomic-batch-hint" /><span id="jmcomic-batch-hint" class="field-hint">每批 1–{{ collectionBatchLimit }} 本，不允许重复编号。批量只下载全本，需要选章时请使用“搜索与选章”。</span></label>
        <button class="secondary-button collection-self-start" type="button" :disabled="busy || locked || !batchText.trim() || !!unavailableReason" @click="previewBatch">{{ busy && !locked ? '正在预览批次…' : '预览批次' }}</button>
        <p v-if="rows.length" class="field-hint" role="status">本批 {{ rows.length }} 本 · 已受理 {{ batchSubmitted }} 本 · 当前可提交 {{ batchSubmittable.length }} 本</p>
        <div v-if="rows.length" class="jmcomic-batch-list">
          <article v-for="row in rows" :key="row.albumId" class="collection-preview-row" :data-album-id="row.albumId">
            <strong>{{ row.preview?.title || `漫画编号 ${row.albumId}` }}</strong><p class="field-hint">编号 {{ row.albumId }} · {{ batchStateLabel(row) }}</p>
            <p v-if="row.preview?.jmcomic" class="field-hint">全本 {{ row.preview.jmcomic.selectedChapterIds.length }} 个章节 · {{ row.preview.jmcomic.pageCount }} 页</p>
            <p v-if="row.preview" class="field-hint">预览有效至 {{ collectionTime(row.preview.expiresAt) }}</p>
            <p v-for="(warning, index) in row.preview?.warnings || []" :key="index" class="collection-warning">{{ warning }}</p>
            <p v-if="row.preview?.jmcomic?.overBudget" class="collection-warning">此本超过页数预算，不能整本提交；请切换“搜索与选章”减少章节后下载。</p>
            <p v-if="row.error" class="error-box" role="alert">{{ row.error }}<span v-if="row.input"> 重试只核对本条目的原请求，不会重发已受理条目。</span></p>
            <button v-if="row.task" class="text-button collection-self-start" type="button" :disabled="busy" @click="emit('open-task', row.task)">查看收集任务<Icon name="arrow" :size="16" /></button>
            <button v-else-if="!row.input" class="text-button collection-self-start" type="button" :disabled="busy || !!unavailableReason" @click="retryBatchPreview(row)">重新预览此本</button>
          </article>
        </div>
        <p v-if="rows.length" class="field-hint">创建独立媒体库时每本各建一个库；选择已有漫画库时，本批使用同一目标库。</p>
      </template>
      <CollectionTargetFields v-if="mode === 'single' ? !!preview : !!rows.length" content-type="comic" v-model:create-library="createLibrary" v-model:library-id="libraryId" :libraries="libraries" :disabled="busy || locked" fixed-type fixed-type-hint="漫画插件固定导入漫画库，可在入库后使用现有漫画阅读器。" />
      <p v-if="error" class="error-box" role="alert">{{ error }}<span v-if="locked"> 重试会核对原请求；请保留此窗口，避免重复提交。</span></p>
      <p v-if="locked" class="field-hint">提交意图已锁定。关闭会停止本窗口后续提交，已受理任务仍会继续；关闭前请核对结果，也可在收集任务列表查看。</p>
      <div class="dialog-actions">
        <button class="secondary-button" type="button" @click="emit('close')">关闭</button>
        <button v-if="mode === 'single' && preview" class="primary-button" type="button" :disabled="busy || (!input && (expired || !!unavailableReason || targetMissing || chapterSelectionChanged || noChapters || preview.jmcomic?.overBudget))" @click="submit"><Icon name="download" />{{ busy && input ? '正在提交…' : input ? '重试原请求' : '确认下载并入库' }}</button>
        <button v-if="mode === 'batch' && batchSubmittable.length" class="primary-button" type="button" :disabled="busy || targetMissing" @click="submitBatch"><Icon name="download" />{{ busy ? '正在逐本提交…' : locked ? `重试未受理条目 ${batchSubmittable.length} 本` : `确认下载 ${batchSubmittable.length} 本` }}</button>
      </div>
    </div>
  </DialogShell>
</template>

<style scoped>
/* 搜索与编号共用布局，沿用现有表单间距和语义主题。 */
.jmcomic-search { display: flex; align-items: flex-start; gap: 12px; min-width: 0; }
.jmcomic-search > label { flex: 1; min-width: 0; }
.jmcomic-search > button { margin-top: 24px; flex-shrink: 0; }
.jmcomic-results { display: flex; flex-direction: column; gap: 12px; max-height: 380px; overflow-y: auto; }
.jmcomic-chapters { display: flex; flex-direction: column; gap: 12px; min-width: 0; padding: 14px; border: 1px solid var(--border); border-radius: 9px; }
.jmcomic-chapters legend { font-size: 13px; padding: 0 6px; }
.jmcomic-chapters .collection-actions { align-items: center; }
.jmcomic-chapter-list { display: flex; flex-direction: column; gap: 12px; max-height: 280px; overflow-y: auto; padding: 3px; }
.jmcomic-chapter-list label { overflow-wrap: anywhere; }
.jmcomic-batch-list { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
@media (max-width: 600px) { .jmcomic-search { flex-direction: column; } .jmcomic-search > label { width: 100%; } .jmcomic-search > button { margin-top: 0; } }
</style>
