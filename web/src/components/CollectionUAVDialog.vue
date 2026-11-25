<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, createRequestId, errorMessage, isAbort } from '../api'
import { collectionTime } from '../collection'
import type { CollectionCapabilities, CollectionCapability, CollectionPreview, CollectionSource, CollectionTask, CollectionUAVItem, CollectionUAVPage, CollectionUAVSite, Library } from '../types'
import CollectionTargetFields from './CollectionTargetFields.vue'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

/** 视频入口只接收已保存的来源与现有媒体库。 */
interface Props {
  /** 来源快照。 */ sources: CollectionSource[]
  /** 现有媒体库。 */ libraries: Library[]
  /** 实际执行端能力。 */ capabilities: CollectionCapabilities | null
  /** 优先选择的来源。 */ initialSourceId?: string
}
/** 首次提交后固定意图，网络结果不明时仍复用原始请求。 */
interface TaskInput {
  /** 持久幂等编号。 */ requestId: string
  /** 已确认的预览。 */ previewId: string
  /** 固定视频分类。 */ contentType: 'video'
  /** 是否创建独立媒体库。 */ createLibrary: boolean
  /** 现有目标库。 */ libraryId: number
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; 'open-task': [task: CollectionTask] }>()
const sources = computed<CollectionSource[]>((): CollectionSource[] => props.sources.filter((source: CollectionSource): boolean => source.kind === 'uav' && source.enabled))
const sourceId = ref<string>(sources.value.find((source: CollectionSource): boolean => source.id === props.initialSourceId)?.id || sources.value[0]?.id || '')
const site = ref<CollectionUAVSite>('jable')
const query = ref<string>('')
const result = ref<CollectionUAVPage | null>(null)
const searchError = ref<string>('')
const searching = ref<boolean>(false)
const selected = ref<CollectionUAVItem | null>(null)
const preview = ref<CollectionPreview | null>(null)
const busy = ref<boolean>(false)
const error = ref<string>('')
const createLibrary = ref<boolean>(true)
const libraryId = ref<number>(0)
const input = ref<TaskInput | null>(null)
const now = ref<number>(Date.now())
const controller: AbortController = new AbortController()
let searchController: AbortController | null = null
/** 每秒检查预览有效期，退出时释放计时器。 */
const expiryTimer: ReturnType<typeof setInterval> = setInterval((): void => { now.value = Date.now() }, 1000)
const expired = computed<boolean>((): boolean => !!preview.value && (!Number.isFinite(Date.parse(preview.value.expiresAt)) || Date.parse(preview.value.expiresAt) <= now.value))
const unavailableReason = computed<string>((): string => {
  if (!sources.value.some((source: CollectionSource): boolean => source.id === sourceId.value)) return '还没有启用的 UAV 来源，请先添加或启用视频插件来源。'
  if (!props.capabilities) return '平台能力尚未读取成功，请关闭弹窗并刷新状态。'
  if (!props.capabilities.workerReady) return props.capabilities.workerReason || '获取执行端尚未连接。'
  const capability = props.capabilities.capabilities.find((entry: CollectionCapability): boolean => entry.kind === 'uav')
  return capability?.available ? '' : capability?.reason || '执行端尚未启用或安装 UAV 视频插件，请按部署文档配置后重启执行端。'
})

/** 取消旧搜索并隔离迟到响应，错误不作为空结果展示。 */
function clearSearch(): void {
  searchController?.abort()
  searchController = null
  searching.value = false
  result.value = null
  searchError.value = ''
}
/** 输入变化使旧选择和预览失效，已提交意图保持锁定。 */
function clearSelection(): void {
  if (input.value) return
  selected.value = null
  preview.value = null
  error.value = ''
}
/** 查询固定站点的一页结果，不推测上游未知的总数量。 */
async function search(page: number = 1): Promise<void> {
  if (busy.value || input.value || unavailableReason.value) return
  const value: string = query.value.trim()
  if (!value || Array.from(value).length > 120 || /[\u0000-\u001f\u007f]/.test(value)) { searchError.value = '请输入 1–120 个字符的视频关键词。'; return }
  if (!Number.isInteger(page) || page < 1 || page > 1000) { searchError.value = '搜索页码超出允许范围。'; return }
  clearSearch()
  const request: AbortController = new AbortController()
  searchController = request
  searching.value = true
  try {
    const response = await api<CollectionUAVPage>(`/collection/sources/${encodeURIComponent(sourceId.value)}/uav/search`, 'POST', { site: site.value, query: value, page }, request.signal)
    if (request.signal.aborted || searchController !== request) return
    result.value = response
  } catch (failure: unknown) {
    if (!request.signal.aborted && searchController === request && !isAbort(failure)) searchError.value = errorMessage(failure)
  } finally {
    if (searchController === request) { searchController = null; searching.value = false }
  }
}
/** 选择单个结果后先读取服务端预览，不直接触发下载。 */
async function selectVideo(item: CollectionUAVItem): Promise<void> {
  if (busy.value || input.value || unavailableReason.value) return
  selected.value = item
  await loadPreview()
}
/** 预览只提交固定站点和编号，不暴露或接收上游媒体地址。 */
async function loadPreview(): Promise<void> {
  if (busy.value || input.value || !selected.value || unavailableReason.value) return
  busy.value = true
  error.value = ''
  preview.value = null
  try {
    const response = await api<{ preview: CollectionPreview }>('/collection/previews', 'POST', { sourceId: sourceId.value, uavSite: site.value, videoId: selected.value.videoId }, controller.signal)
    if (!controller.signal.aborted) preview.value = response.preview
  } catch (failure: unknown) { if (!isAbort(failure)) error.value = errorMessage(failure) }
  finally { busy.value = false }
}
/** 确认后沿用持久下载与视频扫描流程，未知结果重试保留原请求。 */
async function submit(): Promise<void> {
  if (busy.value || !preview.value || (!input.value && (expired.value || unavailableReason.value))) return
  if (!input.value && !createLibrary.value && !libraryId.value) { error.value = '请选择视频库或创建独立媒体库。'; return }
  if (!input.value) input.value = { requestId: createRequestId(), previewId: preview.value.id, contentType: 'video', createLibrary: createLibrary.value, libraryId: createLibrary.value ? 0 : libraryId.value }
  busy.value = true
  error.value = ''
  clearSearch()
  try {
    const response = await api<{ task: CollectionTask }>('/collection/tasks', 'POST', input.value, controller.signal)
    if (controller.signal.aborted) return
    console.info('视频插件收集任务已受理')
    emit('open-task', response.task)
  } catch (failure: unknown) { if (!isAbort(failure)) error.value = errorMessage(failure) }
  finally { busy.value = false }
}
watch(query, (): void => { clearSearch(); clearSelection() })
watch([sourceId, site], (): void => { clearSearch(); clearSelection() })
onBeforeUnmount((): void => { clearSearch(); controller.abort(); clearInterval(expiryTimer) })
</script>

<template>
  <DialogShell title="UAV 视频搜索与下载" @close="emit('close')">
    <div class="collection-form">
      <p class="muted">选择站点并搜索视频，预览后确认下载到视频库。每次只下载所选视频。</p>
      <div class="collection-field-grid">
        <label>视频插件来源<select v-model="sourceId" :disabled="busy || !!input"><option value="" disabled>选择已启用的视频来源</option><option v-for="source in sources" :key="source.id" :value="source.id">{{ source.name }}</option></select></label>
        <label>视频站点<select v-model="site" :disabled="busy || !!input"><option value="jable">Jable</option><option value="missav">MissAV</option><option value="hanime1">Hanime1</option><option value="huangguo">Huangguo（黄果）</option><option value="supjav">SupJav</option></select></label>
      </div>
      <p v-if="unavailableReason" class="error-box" role="alert">{{ unavailableReason }}</p>
      <form class="uav-search" @submit.prevent="search()">
        <!-- HTML按UTF-16计长，允许120个双码元字符；提交仍按Unicode字符校验120上限。 -->
        <label for="uav-query">视频关键词<input id="uav-query" v-model="query" autofocus maxlength="240" :disabled="busy || !!input" placeholder="输入视频关键词" autocomplete="off" /></label>
        <button class="secondary-button" type="submit" :disabled="busy || !!input || !query.trim() || !!unavailableReason"><Icon name="search" />{{ searching ? '重新搜索' : '搜索视频' }}</button>
      </form>
      <p v-if="searching" role="status" class="field-hint">正在搜索视频…</p>
      <p v-if="searchError" class="error-box" role="alert">{{ searchError }}</p>
      <template v-if="result">
        <div class="collection-section-heading"><h3>搜索结果</h3><span class="field-hint">第 {{ result.page }} 页 · 当前页 {{ result.items.length }} 项</span></div>
        <p v-if="!result.items.length" class="section-note" role="status">没有找到匹配的视频，请更换关键词或站点。</p>
        <div v-else class="uav-results">
          <article v-for="item in result.items" :key="item.videoId" class="collection-preview-row">
            <strong>{{ item.title }}</strong><p class="field-hint">编号 {{ item.videoId }} · {{ item.duration || '时长未知' }}</p>
            <button class="text-button collection-self-start" :disabled="busy || !!input" @click="selectVideo(item)">{{ busy && selected?.videoId === item.videoId ? '正在预览…' : '选择并预览' }}<Icon name="arrow" :size="16" /></button>
          </article>
        </div>
        <div class="collection-pagination"><button class="secondary-button" :disabled="busy || !!input || searching || result.page <= 1" @click="search(result.page - 1)">上一页</button><span>第 {{ result.page }} 页</span><button class="secondary-button" :disabled="busy || !!input || searching || !result.hasNext || result.page >= 1000" @click="search(result.page + 1)">下一页</button></div>
      </template>
      <p v-if="busy && !input" class="field-hint" role="status">正在读取视频预览…</p>
      <button v-if="selected && !preview && !busy" class="secondary-button collection-self-start" :disabled="!!input || !!unavailableReason" @click="loadPreview">重试视频预览</button>
      <template v-if="preview">
        <article class="collection-preview-row" aria-label="视频下载预览"><strong>{{ preview.title }}</strong><p class="field-hint">视频编号 {{ selected?.videoId }} · {{ expired && !input ? '预览已过期，请重新预览' : `有效至 ${collectionTime(preview.expiresAt)}` }}</p><p class="field-hint">文件大小与数量在获取后校验，完成发布后才会扫描入库。</p><p v-for="(warning, index) in preview.warnings" :key="index" class="collection-warning">{{ warning }}</p><button v-if="expired && !input" class="text-button collection-self-start" :disabled="busy || !!unavailableReason" @click="loadPreview">重新预览视频</button></article>
        <CollectionTargetFields content-type="video" v-model:create-library="createLibrary" v-model:library-id="libraryId" :libraries="libraries" :disabled="busy || !!input" fixed-type fixed-type-hint="视频插件固定导入视频库，入库后使用现有播放器。" />
      </template>
      <p v-if="error" class="error-box" role="alert">{{ error }}<span v-if="input"> 重试会核对原请求；请保留此窗口，避免重复提交。</span></p>
      <div class="dialog-actions"><button class="secondary-button" type="button" @click="emit('close')">关闭</button><button v-if="preview" class="primary-button" type="button" :disabled="busy || (!input && (expired || !!unavailableReason || (!createLibrary && !libraryId)))" @click="submit"><Icon name="download" />{{ busy && input ? '正在提交…' : input ? '重试原请求' : '确认下载并入库' }}</button></div>
    </div>
  </DialogShell>
</template>

<style scoped>
/* 沿用收集表单间距与主题，让长标题在小屏内自然换行。 */
.uav-search { display: flex; align-items: flex-start; gap: 12px; min-width: 0; }
.uav-search > label { flex: 1; min-width: 0; }
.uav-search > button { margin-top: 24px; flex-shrink: 0; }
.uav-results { display: flex; flex-direction: column; gap: 12px; max-height: 380px; overflow-y: auto; }
@media (max-width: 600px) { .uav-search { flex-direction: column; } .uav-search > label { width: 100%; } .uav-search > button { margin-top: 0; } }
</style>
