<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, createRequestId, errorMessage, isAbort } from '../api'
import { collectionTime } from '../collection'
import type { CollectionCapabilities, CollectionJMComicItem, CollectionJMComicPage, CollectionPreview, CollectionSource, CollectionTask, Library } from '../types'
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
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; 'open-task': [task: CollectionTask] }>()
const sources = computed<CollectionSource[]>((): CollectionSource[] => props.sources.filter((source: CollectionSource): boolean => source.kind === 'jmcomic' && source.enabled))
const sourceId = ref<string>(sources.value.find((source: CollectionSource): boolean => source.id === props.initialSourceId)?.id || sources.value[0]?.id || '')
const query = ref<string>('')
const result = ref<CollectionJMComicPage | null>(null)
const searchError = ref<string>('')
const searching = ref<boolean>(false)
const albumId = ref<string>('')
const preview = ref<CollectionPreview | null>(null)
const busy = ref<boolean>(false)
const error = ref<string>('')
const createLibrary = ref<boolean>(true)
const libraryId = ref<number>(0)
const input = ref<TaskInput | null>(null)
const now = ref<number>(Date.now())
const controller: AbortController = new AbortController()
let searchController: AbortController | null = null
/** 每秒只更新预览失效判断，关闭弹窗时释放计时器。 */
const expiryTimer: ReturnType<typeof setInterval> = setInterval((): void => { now.value = Date.now() }, 1000)
const expired = computed<boolean>((): boolean => !!preview.value && (!Number.isFinite(Date.parse(preview.value.expiresAt)) || Date.parse(preview.value.expiresAt) <= now.value))
const unavailableReason = computed<string>((): string => {
  if (!sourceId.value) return '还没有启用的 JMComic 来源，请先添加或启用漫画来源。'
  if (!props.capabilities) return '平台能力尚未读取成功，请关闭弹窗并刷新状态。'
  if (!props.capabilities.workerReady) return props.capabilities.workerReason || '获取执行端尚未连接。'
  const capability = props.capabilities.capabilities.find((entry): boolean => entry.kind === 'jmcomic')
  return capability?.available ? '' : capability?.reason || '执行端尚未启用或安装 JMComic 插件，请按部署文档配置后重启执行端。'
})

/** 丢弃旧查询的迟到响应，不将失败显示为空结果。 */
function clearSearch(): void {
  searchController?.abort()
  searchController = null
  searching.value = false
  result.value = null
  searchError.value = ''
}
/** 改变漫画编号时旧预览不能再被确认。 */
function invalidatePreview(): void { if (!input.value) { preview.value = null; error.value = '' } }
/** 查询一页结果；重复搜索会中止前一请求。 */
async function search(page: number = 1): Promise<void> {
  if (busy.value || input.value || unavailableReason.value) return
  const value: string = query.value.trim()
  if (!value || Array.from(value).length > 120 || /[\u0000-\u001f\u007f]/.test(value)) { searchError.value = '请输入 1–120 个字符的关键词或漫画编号。'; return }
  clearSearch()
  const request: AbortController = new AbortController()
  searchController = request
  searching.value = true
  try {
    const response = await api<CollectionJMComicPage>(`/collection/sources/${encodeURIComponent(sourceId.value)}/jmcomic/search`, 'POST', { query: value, page }, request.signal)
    if (request.signal.aborted || searchController !== request) return
    result.value = response
  } catch (failure: unknown) {
    if (!request.signal.aborted && searchController === request && !isAbort(failure)) searchError.value = errorMessage(failure)
  } finally {
    if (searchController === request) { searchController = null; searching.value = false }
  }
}
/** 点击结果仍先读取服务端预览，再由用户确认下载。 */
async function selectAlbum(item: CollectionJMComicItem): Promise<void> {
  if (busy.value || input.value) return
  albumId.value = item.albumId
  await loadPreview()
}
/** 编号通过固定漫画适配器解析，不接受链接或任意命令参数。 */
async function loadPreview(): Promise<void> {
  if (busy.value || input.value || unavailableReason.value) return
  if (!/^[1-9][0-9]{0,11}$/.test(albumId.value)) { error.value = '请输入有效的正整数漫画编号。'; return }
  busy.value = true
  error.value = ''
  preview.value = null
  try {
    const response = await api<{ preview: CollectionPreview }>('/collection/previews', 'POST', { sourceId: sourceId.value, albumId: albumId.value }, controller.signal)
    if (!controller.signal.aborted) preview.value = response.preview
  } catch (failure: unknown) { if (!isAbort(failure)) error.value = errorMessage(failure) }
  finally { busy.value = false }
}
/** 下载与入库沿用原任务协议，未知结果始终复用同一请求。 */
async function submit(): Promise<void> {
  if (busy.value || !preview.value || (!input.value && (expired.value || unavailableReason.value))) return
  if (!input.value && !createLibrary.value && !libraryId.value) { error.value = '请选择漫画库或创建独立媒体库。'; return }
  if (!input.value) input.value = { requestId: createRequestId(), previewId: preview.value.id, contentType: 'comic', createLibrary: createLibrary.value, libraryId: createLibrary.value ? 0 : libraryId.value }
  busy.value = true
  error.value = ''
  clearSearch()
  try {
    const response = await api<{ task: CollectionTask }>('/collection/tasks', 'POST', input.value, controller.signal)
    if (controller.signal.aborted) return
    console.info('漫画收集任务已受理')
    emit('open-task', response.task)
  } catch (failure: unknown) { if (!isAbort(failure)) error.value = errorMessage(failure) }
  finally { busy.value = false }
}
watch(query, clearSearch)
watch(albumId, invalidatePreview)
watch(sourceId, (): void => { clearSearch(); albumId.value = ''; preview.value = null; error.value = '' })
onBeforeUnmount((): void => { clearSearch(); controller.abort(); clearInterval(expiryTimer) })
</script>

<template>
  <DialogShell title="JMComic 漫画搜索与下载" @close="emit('close')">
    <div class="collection-form">
      <p class="muted">搜索后预览漫画，确认下载并导入漫画库。也可以直接填写漫画编号。</p>
      <label>漫画来源<select v-model="sourceId" :disabled="busy || !!input"><option value="" disabled>选择已启用的漫画来源</option><option v-for="source in sources" :key="source.id" :value="source.id">{{ source.name }}</option></select></label>
      <p v-if="unavailableReason" class="error-box" role="alert">{{ unavailableReason }}</p>
      <form class="jmcomic-search" @submit.prevent="search()">
        <label for="jmcomic-query">关键词或漫画编号<input id="jmcomic-query" v-model="query" autofocus maxlength="120" :disabled="busy || !!input" placeholder="输入关键词或编号" autocomplete="off" /></label>
        <button class="secondary-button" type="submit" :disabled="busy || !!input || !query.trim() || !!unavailableReason"><Icon name="search" />{{ searching ? '重新搜索' : '搜索漫画' }}</button>
      </form>
      <p v-if="searching" role="status" class="field-hint">正在搜索漫画…</p>
      <p v-if="searchError" class="error-box" role="alert">{{ searchError }}</p>
      <template v-if="result">
        <div class="collection-section-heading"><h3>搜索结果</h3><span class="field-hint">第 {{ result.page }} 页 · 共 {{ result.total }} 项</span></div>
        <p v-if="!result.items.length" class="section-note" role="status">没有找到匹配的漫画，请更换关键词。</p>
        <div v-else class="jmcomic-results">
          <article v-for="item in result.items" :key="item.albumId" class="collection-preview-row">
            <strong>{{ item.title }}</strong><p class="field-hint">编号 {{ item.albumId }}<span v-if="item.tags.length"> · {{ item.tags.join('、') }}</span></p>
            <button class="text-button collection-self-start" :disabled="busy || !!input" @click="selectAlbum(item)">选择并预览<Icon name="arrow" :size="16" /></button>
          </article>
        </div>
        <div class="collection-pagination"><button class="secondary-button" :disabled="busy || !!input || searching || result.page <= 1" @click="search(result.page - 1)">上一页</button><span>第 {{ result.page }} 页</span><button class="secondary-button" :disabled="busy || !!input || searching || !result.hasNext || result.page >= 1000" @click="search(result.page + 1)">下一页</button></div>
      </template>
      <form class="jmcomic-search" @submit.prevent="loadPreview">
        <label for="jmcomic-album">漫画编号<input id="jmcomic-album" v-model="albumId" inputmode="numeric" pattern="[1-9][0-9]{0,11}" maxlength="12" required :disabled="busy || !!input" placeholder="仅填写编号" aria-describedby="jmcomic-album-hint" /><span id="jmcomic-album-hint" class="field-hint">下载前先确认标题与目标漫画库。</span></label>
        <button class="secondary-button" type="submit" :disabled="busy || !!input || !albumId || !!unavailableReason">{{ busy && !input ? '正在预览…' : '预览漫画' }}</button>
      </form>
      <template v-if="preview">
        <article class="collection-preview-row" aria-label="漫画下载预览"><strong>{{ preview.title }}</strong><p class="field-hint">漫画编号 {{ albumId }} · {{ expired && !input ? '预览已过期，请重新预览' : `有效至 ${collectionTime(preview.expiresAt)}` }}</p><p class="field-hint">实际文件数量和大小在获取后校验，完成发布后才会扫描入库。</p><p v-for="(warning, index) in preview.warnings" :key="index" class="collection-warning">{{ warning }}</p></article>
        <CollectionTargetFields content-type="comic" v-model:create-library="createLibrary" v-model:library-id="libraryId" :libraries="libraries" :disabled="busy || !!input" fixed-type fixed-type-hint="漫画插件固定导入漫画库，可在入库后使用现有漫画阅读器。" />
      </template>
      <p v-if="error" class="error-box" role="alert">{{ error }}<span v-if="input"> 重试会核对原请求；请保留此窗口，避免重复提交。</span></p>
      <div class="dialog-actions"><button class="secondary-button" type="button" @click="emit('close')">关闭</button><button v-if="preview" class="primary-button" type="button" :disabled="busy || (!input && (expired || !!unavailableReason || (!createLibrary && !libraryId)))" @click="submit"><Icon name="download" />{{ busy && input ? '正在提交…' : input ? '重试原请求' : '确认下载并入库' }}</button></div>
    </div>
  </DialogShell>
</template>

<style scoped>
/* 搜索与编号共用布局，沿用现有表单间距和语义主题。 */
.jmcomic-search { display: flex; align-items: flex-start; gap: 12px; min-width: 0; }
.jmcomic-search > label { flex: 1; min-width: 0; }
.jmcomic-search > button { margin-top: 24px; flex-shrink: 0; }
.jmcomic-results { display: flex; flex-direction: column; gap: 12px; max-height: 380px; overflow-y: auto; }
@media (max-width: 600px) { .jmcomic-search { flex-direction: column; } .jmcomic-search > label { width: 100%; } .jmcomic-search > button { margin-top: 0; } }
</style>
