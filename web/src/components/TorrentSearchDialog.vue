<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, errorMessage, isAbort } from '../api'
import { downloadBytes } from '../downloads'
import type { TorrentSearchCategory, TorrentSearchResponse, TorrentSearchResult, TorrentSearchSourceStatus, TorrentSearchStatus } from '../types'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

/** 搜索与下载器连接状态独立，失联时仍可查看搜索结果。 */
interface Props {
  /** 下载器是否满足现有安全添加与选集契约。 */ canDownload: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; select: [result: TorrentSearchResult]; 'manage-engines': [] }>()
/** 单页只渲染二十条结果，避免移动端长列表阻塞。 */
const pageSize: number = 20
/** 客户端最后保护时限，服务端正常搜索预算应更短。 */
const requestTimeoutMilliseconds: number = 20000
/** 本地排序选项，不会对索引来源发起额外请求。 */
type ResultSort = 'seeders' | 'newest' | 'sizeAsc' | 'sizeDesc'
const status = ref<TorrentSearchStatus | null>(null)
const statusLoading = ref<boolean>(true)
const statusError = ref<string>('')
const keyword = ref<string>('')
const sourceId = ref<string>('')
const category = ref<TorrentSearchCategory>('all')
const sort = ref<ResultSort>('seeders')
const page = ref<number>(1)
const searching = ref<boolean>(false)
const searched = ref<boolean>(false)
const searchedKeyword = ref<string>('')
const result = ref<TorrentSearchResponse | null>(null)
const error = ref<string>('')
const notice = ref<string>('')
const expired = ref<boolean>(false)
const resultHeading = ref<HTMLElement | null>(null)
let controller: AbortController | null = null
let statusController: AbortController | null = null
let expireTimer: ReturnType<typeof setTimeout> | undefined
let searchTimeout: ReturnType<typeof setTimeout> | undefined
let disposed: boolean = false
const ready = computed<boolean>((): boolean => status.value?.enabled === true && status.value.sources.length > 0 && !statusError.value)
const failures = computed<TorrentSearchSourceStatus[]>((): TorrentSearchSourceStatus[] => result.value?.sources.filter((source: TorrentSearchSourceStatus): boolean => source.state === 'error') || [])
const successfulSources = computed<number>((): number => result.value?.sources.filter((source: TorrentSearchSourceStatus): boolean => source.state === 'ready').length || 0)
const sortedResults = computed<TorrentSearchResult[]>((): TorrentSearchResult[] => [...(result.value?.results || [])].sort(compareResults))
const pageCount = computed<number>((): number => Math.max(1, Math.ceil(sortedResults.value.length / pageSize)))
const pageResults = computed<TorrentSearchResult[]>((): TorrentSearchResult[] => sortedResults.value.slice((page.value - 1) * pageSize, page.value * pageSize))

/** 未知或无效数值排到末尾，不将未知做种数伪装成零。 */
function compareNumbers(left: number | null, right: number | null, ascending: boolean = false): number {
  const leftKnown: boolean = left !== null && Number.isFinite(left) && left >= 0
  const rightKnown: boolean = right !== null && Number.isFinite(right) && right >= 0
  if (!leftKnown || !rightKnown) return leftKnown ? -1 : rightKnown ? 1 : 0
  return ascending ? (left as number) - (right as number) : (right as number) - (left as number)
}

/** 将有效发布时间用于排序，无效日期仍作为未知值处理。 */
function publishedTimestamp(value: string): number | null {
  const timestamp: number = Date.parse(value)
  return Number.isFinite(timestamp) && timestamp >= 0 ? timestamp : null
}

/** 按当前排序方式比较安全元数据，标题相同也不合并不同资源。 */
function compareResults(left: TorrentSearchResult, right: TorrentSearchResult): number {
  let comparison: number
  if (sort.value === 'newest') comparison = compareNumbers(publishedTimestamp(left.publishedAt), publishedTimestamp(right.publishedAt))
  else if (sort.value === 'sizeAsc' || sort.value === 'sizeDesc') comparison = compareNumbers(left.sizeBytes, right.sizeBytes, sort.value === 'sizeAsc')
  else comparison = compareNumbers(left.seeders, right.seeders)
  return comparison || left.title.localeCompare(right.title, 'zh-CN') || left.id.localeCompare(right.id)
}

/** 以明确的未知说明展示做种数和下载数。 */
function countLabel(value: number | null): string {
  return value !== null && Number.isSafeInteger(value) && value >= 0 ? value.toLocaleString('zh-CN') : '未知'
}

/** 只展示有效日期，避免显示 Invalid Date 或伪造当前时间。 */
function dateLabel(value: string): string {
  const timestamp: number | null = publishedTimestamp(value)
  return timestamp === null ? '时间未知' : new Date(timestamp).toLocaleDateString('zh-CN')
}

/** 独立读取模块能力，失败不会伪装为尚未配置。 */
async function loadStatus(): Promise<void> {
  statusController?.abort()
  const request: AbortController = new AbortController()
  statusController = request
  statusLoading.value = true
  statusError.value = ''
  try {
    const response: TorrentSearchStatus = await api<TorrentSearchStatus>('/torrent-search/status', 'GET', undefined, request.signal)
    if (request.signal.aborted || disposed || statusController !== request) return
    status.value = response
  } catch (failure: unknown) {
    if (!isAbort(failure) && !disposed && statusController === request) statusError.value = errorMessage(failure)
  } finally {
    if (!disposed && statusController === request) { statusController = null; statusLoading.value = false }
  }
}

/** 中止当前查询并释放计时器，迟到响应不能覆盖下一次搜索。 */
function stopSearch(): void {
  controller?.abort()
  controller = null
  clearTimeout(searchTimeout)
  searching.value = false
}

/** 用户主动取消时提供确定反馈，不把取消展示成空结果成功。 */
function cancelSearch(): void {
  stopSearch()
  searched.value = false
  notice.value = '已取消本次搜索，可以调整关键词后重新搜索。'
}

/** 输入变化清除过时结果，所有外部查询都由用户提交触发。 */
function resetSearch(): void {
  stopSearch()
  clearTimeout(expireTimer)
  result.value = null
  searched.value = false
  expired.value = false
  error.value = ''
  notice.value = ''
  page.value = 1
}

/** 在服务端令牌到期后关闭添加入口，引导重新搜索取得新结果。 */
function scheduleExpiry(expiresAt: string): void {
  clearTimeout(expireTimer)
  const remaining: number = Date.parse(expiresAt) - Date.now()
  expired.value = !Number.isFinite(remaining) || remaining <= 0
  if (!expired.value) expireTimer = setTimeout((): void => { expired.value = true }, Math.min(remaining, 2147483647))
}

/** 显式提交一次受限聚合搜索，不记录关键词或私密来源。 */
async function search(): Promise<void> {
  if (!ready.value) return
  const query: string = keyword.value.trim()
  const length: number = [...query].length
  if (length < 2 || length > 120 || /\p{Cc}/u.test(query)) { error.value = '请输入 2–120 个字符的关键词，且不要包含控制字符。'; return }
  resetSearch()
  const request: AbortController = new AbortController()
  controller = request
  searching.value = true
  searchedKeyword.value = query
  let timedOut: boolean = false
  searchTimeout = setTimeout((): void => { timedOut = true; request.abort() }, requestTimeoutMilliseconds)
  try {
    const response: TorrentSearchResponse = await api<TorrentSearchResponse>('/torrent-search', 'POST', { query, sourceId: sourceId.value, category: category.value }, request.signal)
    if (request.signal.aborted || disposed || controller !== request) return
    result.value = response
    searched.value = true
    scheduleExpiry(response.expiresAt)
    console.info('种子搜索已完成，结果仅在当前会话中展示')
  } catch (failure: unknown) {
    if (disposed || controller !== request) return
    if (timedOut) error.value = '搜索等待超时，请缩小来源范围或稍后重试。'
    else if (!isAbort(failure)) error.value = errorMessage(failure)
  } finally {
    if (!disposed && controller === request) { clearTimeout(searchTimeout); controller = null; searching.value = false }
  }
}

/** 只传递服务端结果编号与安全展示信息，后续表单仍需用户确认。 */
function chooseResult(entry: TorrentSearchResult): void {
  if (!props.canDownload || !entry.downloadable || expired.value || searching.value) return
  emit('select', entry)
}

/** 切页后让列表标题可见，保留当前键盘操作焦点。 */
function changePage(next: number): void {
  page.value = Math.max(1, Math.min(pageCount.value, next))
  resultHeading.value?.scrollIntoView({ block: 'nearest' })
}

/** 关闭前主动中止查询，由原生弹窗恢复入口焦点。 */
function requestClose(): void {
  stopSearch()
  emit('close')
}

watch([keyword, sourceId, category], resetSearch)
watch(sort, (): void => { page.value = 1 })
onMounted((): void => { void loadStatus() })
onBeforeUnmount((): void => {
  disposed = true
  stopSearch()
  statusController?.abort()
  clearTimeout(expireTimer)
})
</script>

<template>
  <DialogShell class="torrent-search-modal" title="搜种" @close="requestClose">
    <section class="torrent-search-dialog" aria-label="种子搜索">
      <div class="torrent-search-intro"><span class="torrent-search-emblem"><Icon name="search" :size="24" /></span><div><p class="eyebrow">寻找下一份收藏</p><h3>搜到喜欢的，再带回栖库</h3><p>聚合已配置的索引来源，比较资源后再选择文件下载。</p></div></div>
      <div v-if="statusLoading" class="download-inline-state" role="status"><span class="loader" />正在读取搜索来源…</div>
      <div v-else-if="statusError" class="error-box" role="alert">{{ statusError }}<button class="text-button" @click="loadStatus">重新读取</button></div>
      <div v-else-if="!ready" class="torrent-search-setup"><Icon name="settings" :size="28" /><h3>接入索引来源，即可开始搜种</h3><p>使用本机托管模式，可在引擎管理中初始化 Jackett，再同步已配置的索引来源。</p><p>已有 Prowlarr 或 Jackett 也可通过服务端来源文件接入，详见 <code>docs/torrent-search.md</code>。端点与 API Key 始终保留在服务端。</p><div class="engine-setup-entry-actions"><button class="primary-button" @click="emit('manage-engines')"><Icon name="settings" :size="16" />管理搜索引擎</button><button class="secondary-button" @click="loadStatus"><Icon name="refresh" :size="16" />重新读取配置</button></div></div>
      <template v-else>
        <form class="torrent-search-form" :aria-busy="searching" @submit.prevent="search">
          <label class="torrent-search-keyword" for="torrent-search-keyword">搜索关键词<span class="torrent-search-input"><Icon name="search" :size="19" /><input id="torrent-search-keyword" v-model="keyword" type="search" required autofocus autocomplete="off" placeholder="输入影片、专辑或书名…" aria-describedby="torrent-search-hint" /></span></label>
          <div class="torrent-search-controls"><label for="torrent-search-source">搜索来源<select id="torrent-search-source" v-model="sourceId"><option value="">全部来源</option><option v-for="entry in status?.sources" :key="entry.id" :value="entry.id">{{ entry.name }}</option></select></label><label for="torrent-search-category">资源范围<select id="torrent-search-category" v-model="category"><option value="all">全部资源</option><option value="video">影视</option><option value="music">音乐</option><option value="book">书籍 / 漫画</option></select></label><button type="submit" class="primary-button"><span v-if="searching" class="loader" /><Icon v-else name="search" :size="18" />{{ searching ? '重新搜索' : '搜索资源' }}</button><button v-if="searching" type="button" class="secondary-button" @click="cancelSearch">取消</button></div>
          <p id="torrent-search-hint" class="field-hint">2–120 个字符；点击搜索或按回车提交。搜索范围取决于来源的支持，图片等资源可选择“全部资源”。</p>
        </form>
        <p v-if="!canDownload" class="torrent-search-notice"><Icon name="info" :size="17" />下载器当前不可添加任务，仍可搜索。恢复连接与文件选择能力后即可添加。</p>
        <p v-if="error" class="error-box" role="alert">{{ error }}</p>
        <p v-if="notice" class="torrent-search-notice" role="status"><Icon name="info" :size="17" />{{ notice }}</p>
        <div v-if="searching" class="torrent-search-state" role="status" aria-live="polite"><span class="loader" /><h3>正在查询索引来源</h3><p>正在搜索“{{ searchedKeyword }}”，通常需要几秒。你可以随时取消或调整关键词。</p></div>
        <template v-else-if="searched && result">
          <div v-if="failures.length" class="torrent-search-warning" role="status"><div><Icon name="warning" :size="18" /><strong>{{ failures.length }} 个来源暂未返回结果</strong></div><ul><li v-for="source in failures" :key="source.id">{{ source.name }}：{{ source.error || '来源暂不可用，请稍后重试' }}</li></ul><p>以下仅展示 {{ successfulSources }} 个成功来源的结果；可以选择其他来源重新搜索。</p></div>
          <p v-if="expired" class="torrent-search-notice" role="status"><Icon name="clock" :size="17" />本批搜索结果已过期，请重新搜索后再添加下载。</p>
          <div ref="resultHeading" class="torrent-search-results-heading"><div><h3>“{{ searchedKeyword }}”的搜索结果</h3><p>{{ result.results.length.toLocaleString('zh-CN') }} 条资源 · {{ successfulSources }} 个来源<span v-if="result.truncated"> · 已达到结果上限，请细化关键词</span></p></div><label v-if="result.results.length" for="torrent-search-sort">结果排序<select id="torrent-search-sort" v-model="sort"><option value="seeders">做种最多</option><option value="newest">最新发布</option><option value="sizeAsc">体积从小到大</option><option value="sizeDesc">体积从大到小</option></select></label></div>
          <div v-if="!result.results.length" class="torrent-search-state"><Icon name="search" :size="35" /><h3>{{ failures.length ? '可用来源中暂无匹配资源' : '没有找到匹配资源' }}</h3><p>试试更短的名称、其他语言关键词，或将范围切换为“全部资源”。</p></div>
          <ol v-else class="torrent-search-results" aria-label="种子搜索结果" :start="(page - 1) * pageSize + 1"><li v-for="entry in pageResults" :key="entry.id" class="torrent-search-result"><div class="torrent-search-result-copy"><span class="torrent-search-source">{{ entry.sourceName }}</span><h4>{{ entry.title }}</h4><dl><div><dt>体积</dt><dd>{{ entry.sizeBytes === null ? '未知' : downloadBytes(entry.sizeBytes) }}</dd></div><div><dt>做种</dt><dd class="torrent-search-seeders">{{ countLabel(entry.seeders) }}</dd></div><div><dt>下载</dt><dd>{{ countLabel(entry.leechers) }}</dd></div><div><dt>发布</dt><dd>{{ dateLabel(entry.publishedAt) }}</dd></div></dl><p v-if="!entry.downloadable" class="field-hint">来源未提供可安全添加的磁力或种子代理，请在索引服务中检查下载代理配置。</p></div><button class="secondary-button" :disabled="!canDownload || !entry.downloadable || expired" :aria-label="`选择资源：${entry.title}`" @click="chooseResult(entry)"><Icon name="plus" :size="17" />选择下载</button></li></ol>
          <nav v-if="pageCount > 1" class="pagination torrent-search-pagination" aria-label="搜索结果分页"><button class="secondary-button" :disabled="page <= 1" @click="changePage(page - 1)"><Icon name="back" :size="16" />上一页</button><span aria-live="polite">第 {{ page }} / {{ pageCount }} 页</span><button class="secondary-button" :disabled="page >= pageCount" @click="changePage(page + 1)">下一页<Icon name="chevron" :size="16" /></button></nav>
          <p v-if="result.results.length" class="field-hint torrent-search-footer"><Icon name="lock" :size="14" />搜索结果短期保留，添加后仍需确认内容类型与文件选集。做种数由来源提供，不保证实时可用。</p>
        </template>
        <div v-else-if="!error && !notice" class="torrent-search-state"><Icon name="search" :size="35" /><h3>从一个名字开始</h3><p>搜索影片、音乐、漫画或小说。不会因搜索自动创建下载。</p></div>
      </template>
    </section>
  </DialogShell>
</template>
