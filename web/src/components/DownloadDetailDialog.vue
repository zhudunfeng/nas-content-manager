<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ApiError, api, createRequestId, errorMessage, isAbort } from '../api'
import { downloadBytes, downloadEta, downloadPercent, downloadStateLabel, importStateLabel, syncTime, transferSpeed } from '../downloads'
import { typeLabels } from '../format'
import type { Download, DownloadFile, DownloadFilePage, Item, ItemPage } from '../types'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

/** 下载详情始终接收服务端的新快照。 */
interface Props {
  /** 当前任务快照。 */ download: Download
  /** 下载器连接是否实时可用。 */ live: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; updated: [download: Download]; 'open-item': [item: Item] }>()
/** 清单读取每页一百项，避免单个响应无限增长。 */
const filePageSize: number = 100
/** 与服务端清单预算一致，限制浏览器保存的文件记录数。 */
const maxManifestFiles: number = 20000
/** 入库内容每页数量，避免大合集挤满弹窗。 */
const itemPageSize: number = 24
const current = ref<Download>(props.download)
const files = ref<DownloadFile[]>([])
const selected = ref<Set<number>>(new Set<number>())
const filePage = ref<number>(1)
const loadedCount = ref<number>(0)
const manifestVersion = ref<number>(0)
const filesComplete = ref<boolean>(false)
const manifestStale = ref<boolean>(false)
const loadingFiles = ref<boolean>(false)
const fileError = ref<string>('')
const actionError = ref<string>('')
const busy = ref<boolean>(false)
const downloadKiB = ref<number>(0)
const uploadKiB = ref<number>(0)
const limitMessage = ref<string>('')
const showingItems = ref<boolean>(false)
const items = ref<Item[]>([])
const itemTotal = ref<number>(0)
const itemPage = ref<number>(1)
const itemError = ref<string>('')
const loadingItems = ref<boolean>(false)
let fileController: AbortController | null = null
let itemController: AbortController | null = null
let actionController: AbortController | null = null
let selectionRequestId: string = ''
let selectionFingerprint: string = ''
let disposed: boolean = false
const visibleFiles = computed<DownloadFile[]>((): DownloadFile[] => files.value.slice((filePage.value - 1) * filePageSize, filePage.value * filePageSize))
const selectedBytes = computed<number>((): number => files.value.reduce((total: number, file: DownloadFile): number => selected.value.has(file.index) ? total + file.sizeBytes : total, 0))
const canSelect = computed<boolean>((): boolean => props.live && filesComplete.value && !loadingFiles.value && !manifestStale.value && !current.value.selectionLocked && !busy.value)
const canConfirm = computed<boolean>((): boolean => canSelect.value && selected.value.size > 0 && Number.isSafeInteger(selectedBytes.value))
const retryableImport = computed<boolean>((): boolean => ['failed', 'target_missing', 'no_supported_files'].includes(current.value.importState))

/** 读取全部清单后才允许提交，防止分页隐式取消未加载文件。 */
async function loadFiles(): Promise<void> {
  fileController?.abort()
  const request: AbortController = new AbortController()
  fileController = request
  loadingFiles.value = true
  filesComplete.value = false
  fileError.value = ''
  loadedCount.value = 0
  const collected: DownloadFile[] = []
  const seen: Set<number> = new Set<number>()
  let version: number | null = null
  let expectedTotal: number | null = null
  try {
    for (let page: number = 1; ; page += 1) {
      const result: DownloadFilePage = await api<DownloadFilePage>(`/downloads/${encodeURIComponent(current.value.id)}/files?page=${page}&pageSize=${filePageSize}`, 'GET', undefined, request.signal)
      if (request.signal.aborted || fileController !== request) return
      if (!Number.isSafeInteger(result.total) || result.total < 0 || result.total > maxManifestFiles || !Number.isSafeInteger(result.manifestVersion)) throw new Error('文件清单超过安全预算或版本无效，请检查下载器')
      version ??= result.manifestVersion
      expectedTotal ??= result.total
      // 跨页发生版本或数量变化时不能合并新旧选集，必须重新加载。
      if (result.manifestVersion !== version || result.total !== expectedTotal) { manifestStale.value = true; throw new Error('文件清单已变化，请重新加载后选择') }
      for (const file of result.files) {
        if (!Number.isSafeInteger(file.index) || file.index < 0 || seen.has(file.index) || !Number.isSafeInteger(file.sizeBytes) || file.sizeBytes < 0) throw new Error('文件清单包含无效编号或大小，无法安全选择')
        seen.add(file.index)
        collected.push(file)
      }
      if (collected.length > expectedTotal || (collected.length < expectedTotal && result.files.length === 0)) throw new Error('文件清单分页不完整，请重新加载')
      loadedCount.value = collected.length
      if (result.selectionLocked) current.value = { ...current.value, selectionLocked: true }
      if (collected.length >= expectedTotal) break
    }
    if (current.value.manifestVersion > 0 && version !== current.value.manifestVersion) { manifestStale.value = true; throw new Error('文件清单版本已更新，等待任务同步后重新加载') }
    files.value = collected
    selected.value = new Set<number>(collected.filter((file: DownloadFile): boolean => file.selected).map((file: DownloadFile): number => file.index))
    manifestVersion.value = version ?? 0
    manifestStale.value = false
    filesComplete.value = true
    filePage.value = 1
    selectionRequestId = ''
    selectionFingerprint = ''
  } catch (failure: unknown) { if (!isAbort(failure) && fileController === request) fileError.value = errorMessage(failure) }
  finally { if (fileController === request) loadingFiles.value = false }
}

/** 使用引擎文件编号维护选集，不依赖当前页的数组下标。 */
function toggleFile(fileIndex: number): void {
  if (!canSelect.value) return
  const next: Set<number> = new Set<number>(selected.value)
  if (next.has(fileIndex)) next.delete(fileIndex)
  else next.add(fileIndex)
  selected.value = next
}

/** 全选或清空完整清单，操作范围包含所有分页。 */
function selectFiles(all: boolean): void {
  if (!canSelect.value) return
  selected.value = new Set<number>(all ? files.value.map((file: DownloadFile): number => file.index) : [])
}

/** 顺序提交单个业务操作，关闭弹窗前保留明确的确认结果。 */
async function mutate(operation: (signal: AbortSignal) => Promise<void>): Promise<void> {
  if (busy.value || !props.live) return
  busy.value = true
  actionError.value = ''
  const request: AbortController = new AbortController()
  actionController = request
  try { await operation(request.signal) }
  catch (failure: unknown) {
    if (!isAbort(failure) && !disposed) {
      actionError.value = errorMessage(failure)
      if (failure instanceof ApiError && failure.status === 409) manifestStale.value = true
    }
  } finally { if (!disposed) busy.value = false }
}

/** 确认固定版本选集并开始下载，未知网络结果复用同一幂等键。 */
async function confirmSelection(): Promise<void> {
  if (!canConfirm.value) return
  const indices: number[] = Array.from(selected.value).sort((left: number, right: number): number => left - right)
  const fingerprint: string = `${manifestVersion.value}:${indices.join(',')}`
  if (fingerprint !== selectionFingerprint) { selectionFingerprint = fingerprint; selectionRequestId = createRequestId() }
  await mutate(async (signal: AbortSignal): Promise<void> => {
    const result = await api<{ download: Download }>(`/downloads/${encodeURIComponent(current.value.id)}/selection`, 'PUT', { requestId: selectionRequestId, manifestVersion: manifestVersion.value, indices }, signal)
    if (signal.aborted) return
    current.value = result.download
    emit('updated', result.download)
    console.info('下载选集已确认，正式下载请求已提交')
  })
}

/** 对已有受管任务设置限速，零表示仍遵从引擎全局策略。 */
async function saveLimits(): Promise<void> {
  if (![downloadKiB.value, uploadKiB.value].every((value: number): boolean => Number.isSafeInteger(value) && value >= 0 && value <= 1048576)) { actionError.value = '限速需要填写 0–1048576 的整数，单位 KiB/s'; return }
  limitMessage.value = ''
  await mutate(async (signal: AbortSignal): Promise<void> => {
    await api<{ ok: boolean }>(`/downloads/${encodeURIComponent(current.value.id)}/limits`, 'PUT', { downloadKiB: downloadKiB.value, uploadKiB: uploadKiB.value }, signal)
    if (!signal.aborted) { limitMessage.value = '任务限速已保存'; console.info('下载任务限速已更新') }
  })
}

/** 单独重试发布和入库，不重新添加下载来源。 */
async function retryImport(): Promise<void> {
  await mutate(async (signal: AbortSignal): Promise<void> => {
    const result = await api<{ download: Download }>(`/downloads/${encodeURIComponent(current.value.id)}/retry-import`, 'POST', undefined, signal)
    if (!signal.aborted) { current.value = result.download; emit('updated', result.download); console.info('下载入库重试已提交') }
  })
}

/** 分页读取已经入库的条目，交由既有详情和阅读器打开。 */
async function loadItems(page: number = 1): Promise<void> {
  itemController?.abort()
  const request: AbortController = new AbortController()
  itemController = request
  showingItems.value = true
  loadingItems.value = true
  itemError.value = ''
  try {
    const result: ItemPage = await api<ItemPage>(`/downloads/${encodeURIComponent(current.value.id)}/items?page=${page}&pageSize=${itemPageSize}`, 'GET', undefined, request.signal)
    if (request.signal.aborted || itemController !== request) return
    items.value = result.items
    itemTotal.value = result.total
    itemPage.value = result.page
  } catch (failure: unknown) { if (!isAbort(failure) && itemController === request) itemError.value = errorMessage(failure) }
  finally { if (itemController === request) loadingItems.value = false }
}

/** 对正在确认的操作保持弹窗，避免丢失可重试的请求键。 */
function requestClose(): void {
  if (!busy.value) emit('close')
}

/** 将每个文件的有界完成比例转成可访问百分比。 */
function filePercent(progress: number): number {
  return Number.isFinite(progress) ? Math.round(Math.max(0, Math.min(1, progress)) * 100) : 0
}

watch((): Download => props.download, (download: Download): void => {
  if (download.revision < current.value.revision) return
  const changed: boolean = manifestVersion.value > 0 && download.manifestVersion !== manifestVersion.value
  current.value = download
  if (changed && filesComplete.value) manifestStale.value = true
  if (!filesComplete.value && !loadingFiles.value && !fileError.value && download.manifestVersion > 0) void loadFiles()
})
onMounted((): void => { if (current.value.manifestVersion > 0) void loadFiles() })
onBeforeUnmount((): void => { disposed = true; fileController?.abort(); itemController?.abort(); actionController?.abort() })
</script>

<template>
  <DialogShell title="下载详情" @close="requestClose">
    <div class="download-detail">
      <div class="download-detail-title"><span class="library-icon" :class="`shortcut-${current.contentType}`"><Icon :name="current.contentType" :size="26" /></span><div><span class="eyebrow">{{ typeLabels[current.contentType] }} · BT 下载</span><h3>{{ current.name || '正在获取名称…' }}</h3></div></div>
      <div class="download-badges"><span class="status-pill" :class="{ warning: ['error', 'missing'].includes(current.state) }">{{ downloadStateLabel(current.state) }}</span><span class="status-pill" :class="{ warning: retryableImport }">{{ importStateLabel(current.importState) }}</span><span v-if="!live" class="download-stale-label">上次状态 · 速度未知</span></div>
      <progress class="download-progress" :value="downloadPercent(current)" max="100" :aria-label="`${current.name} 下载进度`" />
      <dl class="download-metrics"><div><dt>已下载 / 总大小</dt><dd>{{ downloadBytes(current.downloadedBytes) }} / {{ downloadBytes(current.sizeBytes) }}</dd></div><div><dt>下载 / 上传速度</dt><dd>{{ transferSpeed(current.downloadSpeed, live) }} / {{ transferSpeed(current.uploadSpeed, live) }}</dd></div><div><dt>剩余时间</dt><dd>{{ downloadEta(current, live) }}</dd></div><div><dt>连接节点 / 分享率</dt><dd>{{ live ? current.peers : '未知' }} / {{ Number.isFinite(current.ratio) ? current.ratio.toFixed(2) : '未知' }}</dd></div><div><dt>累计上传</dt><dd>{{ downloadBytes(current.uploadedBytes) }}</dd></div><div><dt>上次同步</dt><dd>{{ syncTime(current.lastSyncedAt) }}</dd></div></dl>
      <p v-if="current.lastError" class="error-box" role="alert">{{ current.lastError }}</p>
      <section class="download-detail-section" aria-labelledby="download-file-heading"><div class="section-heading"><h3 id="download-file-heading">文件清单</h3><button class="text-button" :disabled="loadingFiles || busy" @click="loadFiles"><Icon name="refresh" :size="16" />重新加载</button></div>
        <p v-if="current.selectionLocked" class="field-hint"><Icon name="lock" :size="14" />选集已经确认并锁定。为保证已发布文件安全，首期不支持下载中改变选集。</p>
        <p v-else class="field-hint">确认会开始正式下载，之后无法修改选集。选取包含全部分页的文件清单。</p>
        <p v-if="manifestStale" class="error-box" role="alert">文件清单已更新或操作已过期，请重新加载后核对选集。</p>
        <p v-if="fileError" class="error-box" role="alert">{{ fileError }}</p>
        <div v-if="loadingFiles" class="download-inline-state" role="status"><span class="loader" />正在读取全部文件清单… 已读取 {{ loadedCount }} 项</div>
        <div v-else-if="!current.manifestVersion && !filesComplete" class="download-inline-state" role="status"><Icon name="clock" />正在等待种子元数据。清单就绪后可选择文件。</div>
        <template v-if="filesComplete && files.length">
          <div class="download-file-toolbar"><span>已选 {{ selected.size }} / {{ files.length }} 项 · {{ downloadBytes(selectedBytes) }}</span><div><button class="text-button" :disabled="!canSelect" @click="selectFiles(true)">全选</button><button class="text-button" :disabled="!canSelect" @click="selectFiles(false)">清空</button></div></div>
          <div class="download-file-list"><label v-for="file in visibleFiles" :key="file.index" class="download-file-row"><input type="checkbox" :checked="selected.has(file.index)" :disabled="!canSelect" @change="toggleFile(file.index)" /><span class="download-file-copy"><span>{{ file.path }}</span><small>{{ downloadBytes(file.sizeBytes) }} · {{ filePercent(file.progress) }}%</small></span></label></div>
          <div v-if="files.length > filePageSize" class="pagination download-file-pagination"><button class="secondary-button" :disabled="filePage <= 1" @click="filePage -= 1">上一页</button><span>{{ filePage }} / {{ Math.ceil(files.length / filePageSize) }}</span><button class="secondary-button" :disabled="filePage * filePageSize >= files.length" @click="filePage += 1">下一页</button></div>
          <button v-if="!current.selectionLocked" class="primary-button download-confirm" :disabled="!canConfirm" @click="confirmSelection"><Icon name="play" />{{ busy ? '正在确认…' : `确认 ${selected.size} 项并开始下载` }}</button>
        </template>
        <p v-else-if="filesComplete && !fileError" class="field-hint">尚无可选文件，元数据可能仍在同步。请稍后重新加载。</p>
      </section>
      <details class="download-options"><summary>任务级限速</summary><form class="download-limit-form" @submit.prevent="saveLimits"><label>下载上限（KiB/s）<input v-model.number="downloadKiB" type="number" min="0" max="1048576" step="1" required :disabled="busy || !live" /></label><label>上传上限（KiB/s）<input v-model.number="uploadKiB" type="number" min="0" max="1048576" step="1" required :disabled="busy || !live" /></label><button class="secondary-button" type="submit" :disabled="busy || !live">保存限速</button></form><p class="field-hint">只设置本任务。0 表示不额外限速，仍受下载器全局策略约束；此表单不会读取或覆盖全局设置。</p><p v-if="limitMessage" class="field-hint" role="status">{{ limitMessage }}</p></details>
      <section class="download-detail-section"><div class="section-heading"><h3>发布与入库</h3><button v-if="retryableImport" class="secondary-button" :disabled="busy || !live" @click="retryImport"><Icon name="refresh" :size="16" />重试入库</button></div><p class="field-hint">下载完成后会校验所选文件并发布，再建立内容索引。下载完成不代表格式可被浏览器解码。</p><button class="secondary-button" :disabled="loadingItems || !current.ready" @click="loadItems()"><Icon name="folder" :size="16" />{{ loadingItems ? '正在读取…' : '查看入库内容' }}</button>
        <p v-if="itemError" class="error-box" role="alert">{{ itemError }}<button class="text-button" @click="loadItems(itemPage)">重试</button></p><div v-if="showingItems && items.length" class="download-imported-list"><button v-for="item in items" :key="item.id" :disabled="busy" @click="emit('open-item', item)"><Icon :name="item.type" /><span>{{ item.title }}</span><Icon name="chevron" :size="16" /></button></div><p v-else-if="showingItems && !loadingItems && !itemError" class="field-hint">尚未发现已入库内容，扫描完成后可再次查看。</p><div v-if="itemTotal > itemPageSize" class="pagination download-file-pagination"><button class="secondary-button" :disabled="loadingItems || itemPage <= 1" @click="loadItems(itemPage - 1)">上一页</button><span>{{ itemPage }} / {{ Math.ceil(itemTotal / itemPageSize) }}</span><button class="secondary-button" :disabled="loadingItems || itemPage * itemPageSize >= itemTotal" @click="loadItems(itemPage + 1)">下一页</button></div>
      </section>
      <p v-if="actionError" class="error-box" role="alert">{{ actionError }}</p><p v-if="busy" class="field-hint" role="status">正在确认操作，请稍候。</p><div class="dialog-actions"><button class="secondary-button" :disabled="busy" @click="requestClose">关闭详情</button></div>
    </div>
  </DialogShell>
</template>
