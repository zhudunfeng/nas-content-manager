<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, createRequestId, errorMessage, isAbort } from '../api'
import { collectionBatchLimit, collectionKindLabels, collectionTime } from '../collection'
import { extension, formatSize } from '../format'
import type { CollectionCapabilities, CollectionKind, CollectionPreview, CollectionSource, CollectionTask, ContentType, Library } from '../types'
import CollectionTargetFields from './CollectionTargetFields.vue'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

/** 单条任务首次提交后保留不可变的请求意图。 */
interface TaskInput {
  /** 稳定幂等请求键。 */ requestId: string
  /** 已确认预览编号。 */ previewId: string
  /** 内容分类。 */ contentType: ContentType
  /** 是否独立建库。 */ createLibrary: boolean
  /** 目标媒体库编号。 */ libraryId: number
}
/** 每条输入分别保存预览、提交与失败状态。 */
interface PreviewRow {
  /** 批次内稳定序号。 */ index: number
  /** 仅存在表单内的用户输入地址。 */ url: string
  /** 服务端预览结果。 */ preview: CollectionPreview | null
  /** 当前预览和提交阶段。 */ state: 'waiting' | 'previewing' | 'ready' | 'preview_failed' | 'submitting' | 'submitted' | 'submit_failed'
  /** 已脱敏的失败说明。 */ error: string
  /** 结果不确定时复用的首次提交体。 */ input: TaskInput | null
  /** 已受理任务。 */ task: CollectionTask | null
}
/** 网页和文件入口共用有界预览。 */
interface Props {
  /** 已保存来源。 */ sources: CollectionSource[]
  /** 当前媒体库。 */ libraries: Library[]
  /** 实际平台与执行端能力。 */ capabilities: CollectionCapabilities | null
  /** 预选的来源编号。 */ initialSourceId?: string
  /** 预选的来源类型。 */ initialKind?: CollectionKind
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; created: []; 'open-task': [task: CollectionTask] }>()
const availableSources = computed<CollectionSource[]>((): CollectionSource[] => props.sources.filter((source: CollectionSource): boolean => source.enabled && source.kind !== 'rss'))
const firstSource: CollectionSource | undefined = availableSources.value.find((source: CollectionSource): boolean => source.id === props.initialSourceId) || availableSources.value.find((source: CollectionSource): boolean => source.kind === props.initialKind) || availableSources.value[0]
const sourceId = ref<string>(firstSource?.id || '')
const selectedSource = computed<CollectionSource | undefined>((): CollectionSource | undefined => availableSources.value.find((source: CollectionSource): boolean => source.id === sourceId.value))
const urls = ref<string>('')
const path = ref<string>('')
const profile = ref<'video' | 'audio'>(firstSource?.outputProfile === 'audio' ? 'audio' : 'video')
const contentType = ref<ContentType>(profile.value === 'audio' ? 'music' : 'video')
const createLibrary = ref<boolean>(true)
const libraryId = ref<number>(0)
const finalFilesConfirmed = ref<boolean>(false)
const rows = ref<PreviewRow[]>([])
const busy = ref<boolean>(false)
const error = ref<string>('')
const controller: AbortController = new AbortController()
const locked = computed<boolean>((): boolean => rows.value.some((row: PreviewRow): boolean => row.input !== null))
const hasResults = computed<boolean>((): boolean => rows.value.length > 0)
const readyCount = computed<number>((): number => rows.value.filter((row: PreviewRow): boolean => (row.state === 'ready' || row.state === 'submit_failed') && !expired(row)).length)
const submittedCount = computed<number>((): number => rows.value.filter((row: PreviewRow): boolean => row.state === 'submitted').length)
const register = computed<boolean>((): boolean => selectedSource.value?.kind === 'external' || (selectedSource.value?.kind === 'local' && selectedSource.value.rootId.startsWith('media-')))
const unavailableReason = computed<string>((): string => {
  if (!selectedSource.value) return ''
  if (!props.capabilities) return '平台能力尚未读取成功，请关闭并重新读取状态。'
  if (register.value) return props.capabilities.roots.some((root): boolean => root.id === selectedSource.value?.rootId) ? '' : '来源的授权根目录已变化，请在来源中重新选择。'
  if (!props.capabilities.workerReady) return props.capabilities.workerReason || '获取执行端尚未连接。'
  const capability = props.capabilities.capabilities.find((entry): boolean => entry.kind === selectedSource.value?.kind)
  return capability?.available ? '' : capability?.reason || '执行端尚未具备此来源的实际获取能力。'
})
watch(sourceId, (): void => { rows.value = []; path.value = ''; finalFilesConfirmed.value = false; profile.value = selectedSource.value?.outputProfile === 'audio' ? 'audio' : 'video'; contentType.value = profile.value === 'audio' ? 'music' : 'video' })
watch(profile, (): void => { rows.value = []; contentType.value = profile.value === 'audio' ? 'music' : 'video' })

/** 每次修改预览输入都使旧预览失效，已提交计划保持锁定。 */
function invalidate(): void { if (!locked.value) rows.value = [] }
/** 标注预览文件类型，不猜测文件是否已经入库。 */
function fileLabel(filePath: string): string {
  const ext: string = extension(filePath)
  if (['MP3', 'M4A', 'AAC', 'FLAC', 'OGG', 'OPUS', 'WAV', 'AIF', 'AIFF'].includes(ext)) return '音乐'
  if (['MP4', 'MKV', 'WEBM', 'MOV', 'AVI', 'M4V', 'TS'].includes(ext)) return '视频'
  return ext
}
/** 检查服务端预览有效期；未知提交仍可复用旧键对账。 */
function expired(row: PreviewRow): boolean { return !row.input && !!row.preview && new Date(row.preview.expiresAt).getTime() <= Date.now() }
/** 使用固定适配器逐条预览，单项失败不丢失其他条目。 */
async function previewRow(row: PreviewRow): Promise<void> {
  if (!selectedSource.value || controller.signal.aborted) return
  row.state = 'previewing'
  row.error = ''
  try {
    const result = await api<{ preview: CollectionPreview }>('/collection/previews', 'POST', { sourceId: sourceId.value, url: row.url, path: path.value.trim(), profile: profile.value, mode: register.value ? 'register' : 'copy' }, controller.signal)
    if (controller.signal.aborted) return
    row.preview = result.preview
    row.state = 'ready'
  } catch (failure: unknown) {
    if (!isAbort(failure)) { row.error = errorMessage(failure); row.state = 'preview_failed' }
  }
}
/** 显式有界的批次只包含用户粘贴的单条链接，不展开频道。 */
async function previewAll(): Promise<void> {
  if (busy.value || locked.value || !selectedSource.value || unavailableReason.value) return
  error.value = ''
  const inputs: string[] = selectedSource.value.kind === 'web' ? urls.value.split(/\r?\n/).map((value: string): string => value.trim()).filter((value: string): boolean => !!value) : ['']
  if (!inputs.length || inputs.length > collectionBatchLimit) { error.value = `每批请输入 1–${collectionBatchLimit} 条链接，一行一条。`; return }
  if (selectedSource.value.kind === 'web' && inputs.some((value: string): boolean => !/^https?:\/\//i.test(value))) { error.value = '网页入口仅接受 HTTP(S) 页面链接。'; return }
  if (selectedSource.value.kind === 'external' && !finalFilesConfirmed.value) { error.value = '请先确认选定目录中的文件已经由外部应用完成。'; return }
  if (path.value.split(/[\\/]/).some((segment: string): boolean => segment === '..') || /^[\\/]|^[a-zA-Z]:/.test(path.value)) { error.value = '请输入根目录以内的相对路径。'; return }
  rows.value = inputs.map((value: string, index: number): PreviewRow => ({ index: index + 1, url: value, preview: null, state: 'waiting', error: '', input: null, task: null }))
  busy.value = true
  try { for (const row of rows.value) { if (controller.signal.aborted) break; await previewRow(row) } }
  finally { busy.value = false }
}
/** 只重试失败或已过期的预览，不改变已有成功的任务。 */
async function retryPreview(row: PreviewRow): Promise<void> {
  if (busy.value || row.input) return
  busy.value = true
  try { await previewRow(row) } finally { busy.value = false }
}
/** 将首次目标与预览绑定，网络不确定时完整复用原请求。 */
async function submitRow(row: PreviewRow): Promise<void> {
  if (!row.preview || controller.signal.aborted || row.state === 'submitted' || expired(row)) return
  row.input ||= { requestId: createRequestId(), previewId: row.preview.id, contentType: contentType.value, createLibrary: createLibrary.value, libraryId: createLibrary.value ? 0 : libraryId.value }
  row.state = 'submitting'
  row.error = ''
  try {
    const result = await api<{ task: CollectionTask }>('/collection/tasks', 'POST', row.input, controller.signal)
    if (controller.signal.aborted) return
    row.task = result.task
    row.state = 'submitted'
    console.info('资源收集任务已受理')
    emit('created')
  } catch (failure: unknown) { if (!isAbort(failure)) { row.error = errorMessage(failure); row.state = 'submit_failed' } }
}
/** 只提交尚未成功的条目，批次内按顺序限流。 */
async function submitAll(): Promise<void> {
  if (busy.value || (!createLibrary.value && !libraryId.value)) return
  busy.value = true
  try {
    for (const row of rows.value) {
      if (controller.signal.aborted) break
      if (row.state === 'ready' || row.state === 'submit_failed') await submitRow(row)
    }
  } finally { busy.value = false }
}
onBeforeUnmount((): void => { controller.abort(); urls.value = ''; for (const row of rows.value) row.url = '' })
</script>

<template>
  <DialogShell title="网页与文件收集" @close="emit('close')">
    <div class="collection-form">
      <p class="muted">先预览资源与目标，再逐项创建任务。网页保存、归档复制和只读登记分别显示实际结果。</p>
      <form class="collection-form" @submit.prevent="previewAll">
        <label>收集来源<select v-model="sourceId" required :disabled="busy || locked"><option value="" disabled>选择启用的来源</option><option v-for="source in availableSources" :key="source.id" :value="source.id">{{ source.name }} · {{ collectionKindLabels[source.kind] }}</option></select><span class="field-hint">来源和连接在“来源”中配置；RSS 请使用订阅与候选流程。</span></label>
        <p v-if="!availableSources.length" class="section-note">还没有启用的网页或文件来源。关闭此窗口后，先添加对应来源。</p>
        <p v-if="unavailableReason" class="error-box" role="alert">{{ unavailableReason }} 请检查来源与服务端部署配置后重试。</p>
        <template v-if="selectedSource?.kind === 'web'">
          <label>页面链接<textarea v-model="urls" required rows="4" maxlength="16384" :disabled="busy || locked" spellcheck="false" placeholder="https://…&#10;每行一个链接" @input="invalidate"></textarea><span class="field-hint">每批最多 {{ collectionBatchLimit }} 条，单链接单条目；不递归播放列表或频道，不处理需登录或 DRM 内容。</span></label>
          <label>输出类型<select v-model="profile" :disabled="busy || locked"><option value="video">保存视频</option><option value="audio">提取音频</option></select><span class="field-hint">使用执行端固定推荐格式；格式列表仅用于预览，不接受任意工具参数。</span></label>
        </template>
        <template v-else-if="selectedSource">
          <label>本次相对路径<input v-model="path" maxlength="512" :disabled="busy || locked" placeholder="留空使用来源配置目录" @input="invalidate" /><span class="field-hint">在来源已配置根目录以内选择文件或目录。{{ register ? '只读登记原文件，不创建媒体副本。' : '复制到受管目录，保留来源文件。' }}</span></label>
          <label v-if="selectedSource.kind === 'external'" class="collection-check collection-consent"><input v-model="finalFilesConfirmed" type="checkbox" :disabled="busy || locked" @change="invalidate" /><span>我确认选定的是外部应用已完成的成品<span class="field-hint">只读登记独立最终目录；下载暂存或正在写入的文件不应导入。</span></span></label>
        </template>
        <button class="secondary-button collection-self-start" type="submit" :disabled="busy || locked || !selectedSource || !!unavailableReason"><Icon name="search" />{{ busy ? '正在处理…' : hasResults ? '重新预览' : '开始预览' }}</button>
      </form>
      <p v-if="error" class="error-box" role="alert">{{ error }}</p>
      <template v-if="hasResults">
        <div class="collection-section-heading"><h3>逐项预览</h3><span class="field-hint">{{ rows.length }} 项 · 已受理 {{ submittedCount }} 项</span></div>
        <article v-for="row in rows" :key="row.index" class="collection-preview-row">
          <div class="collection-section-heading"><strong>{{ row.preview?.title || `条目 ${row.index}` }}</strong><span class="status-pill">{{ row.state === 'submitted' ? '已受理' : row.state === 'previewing' ? '预览中' : row.state === 'submitting' ? '提交中' : row.state === 'waiting' ? '等待预览' : row.state === 'preview_failed' || row.state === 'submit_failed' ? '失败' : expired(row) ? '预览已过期' : '待确认' }}</span></div>
          <template v-if="row.preview">
            <p class="field-hint">{{ row.preview.files.length }} 个文件 · {{ formatSize(row.preview.totalBytes) }} · 有效至 {{ collectionTime(row.preview.expiresAt) }}</p>
            <p v-if="row.preview.formats.length" class="field-hint">格式：{{ row.preview.formats.map((format): string => format.label || format.extension).join('、') }}</p>
            <ul v-if="row.preview.files.length" class="collection-file-list"><li v-for="file in row.preview.files.slice(0, 30)" :key="file.path"><span>{{ file.path }}</span><small>{{ fileLabel(file.path) }} · {{ formatSize(file.size) }}</small></li></ul>
            <p v-if="row.preview.files.length > 30" class="field-hint">此处展示前 30 项，完整的有界清单由服务端验证。</p>
            <p v-for="(warning, index) in row.preview.warnings" :key="index" class="field-hint collection-warning">{{ warning }}</p>
          </template>
          <p v-if="row.error" class="error-box" role="alert">{{ row.error }}<span v-if="row.input"> 重试将保留原请求和目标，已成功的条目不会再次提交。</span></p>
          <button v-if="row.state === 'preview_failed' || expired(row)" class="text-button" :disabled="busy || !!row.input" @click="retryPreview(row)">重新预览此项</button>
          <button v-if="row.task" class="text-button" :disabled="busy" @click="emit('open-task', row.task)">查看收集任务<Icon name="arrow" :size="16" /></button>
        </article>
        <CollectionTargetFields v-model:content-type="contentType" v-model:create-library="createLibrary" v-model:library-id="libraryId" :libraries="libraries" :disabled="busy || locked" :fixed-type="selectedSource?.kind === 'web'" />
        <p class="section-note">{{ register ? '确认后登记原目录并扫描；原文件和音乐标签保持不变。' : '确认后先在隔离目录获取，校验发布成功后再扫描入库。' }} 已存在、不支持或冲突文件以服务端预览说明为准。</p>
        <div class="dialog-actions"><button class="secondary-button" type="button" @click="emit('close')">关闭</button><button class="primary-button" type="button" :disabled="busy || !readyCount || (!createLibrary && !libraryId)" @click="submitAll"><Icon name="check" />{{ busy ? '正在处理…' : locked ? '重试未成功条目' : `确认收集 ${readyCount} 项` }}</button></div>
      </template>
    </div>
  </DialogShell>
</template>
