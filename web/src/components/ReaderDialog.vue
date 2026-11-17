<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type Book from 'epubjs/types/book'
import type Rendition from 'epubjs/types/rendition'
import type { Location } from 'epubjs/types/rendition'
import type { NavItem } from 'epubjs/types/navigation'
import { api, errorMessage, isAbort } from '../api'
import { fetchEpub, sanitizeEpubSection } from '../epub-safety'
import { extension, formatDuration, formatSize, typeLabels } from '../format'
import type { ComicPage, Item, TextBook } from '../types'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

/** 沉浸阅读所需内容与相邻图片。 */
interface Props { /** 当前内容。 */ item: Item; /** 当前列表中的图片。 */ imageItems?: Item[] }
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; updated: []; navigate: [item: Item] }>()
const busy = ref<boolean>(false)
const failure = ref<string>('')
const saveFailure = ref<string>('')
const video = ref<HTMLVideoElement | null>(null)
const videoStatus = ref<string>('正在准备视频…')
const pages = ref<ComicPage[]>([])
const pageIndex = ref<number>(Math.max(0, Math.floor(props.item.progress?.position || 0)))
const comicMode = ref<'single' | 'double' | 'continuous'>('single')
const direction = ref<'ltr' | 'rtl'>('ltr')
const textBook = ref<TextBook | null>(null)
const paragraphIndex = ref<number>(Math.max(0, Math.floor(props.item.progress?.position || 0)))
const encoding = ref<string>('')
const fontSize = ref<number>(Number(localStorage.getItem('reader-font-size')) || 20)
const lineHeight = ref<string>(localStorage.getItem('reader-line-height') || '1.9')
const paper = ref<boolean>(localStorage.getItem('reader-paper') === 'true')
const imageZoom = ref<number>(1)
const showInfo = ref<boolean>(false)
const showChapters = ref<boolean>(false)
const bookHost = ref<HTMLElement | null>(null)
const readerBody = ref<HTMLElement | null>(null)
const epubNavigation = ref<NavItem[]>([])
const epubPosition = ref<number>(props.item.progress?.position || 0)
const epubTotal = ref<number>(props.item.progress?.total || 1)
let epubBook: Book | null = null
let rendition: Rendition | null = null
let currentLocator: string = props.item.progress?.locator || ''
let controller: AbortController = new AbortController()
let observer: IntersectionObserver | null = null
let savedAt: number = 0
let closed: boolean = false
let videoRestored: boolean = false
let readingLoad: number = 0
let textBase: number = Math.floor(paragraphIndex.value / 80) * 80
const textStart = ref<number>(textBase)
const comicStart = ref<number>(Math.floor(pageIndex.value / 12) * 12)
const isEpub = computed<boolean>((): boolean => extension(props.item.relativePath) === 'EPUB')
const fileUrl = computed<string>((): string => `/api/items/${props.item.id}/file`)
const displayedPages = computed<ComicPage[]>((): ComicPage[] => comicMode.value === 'continuous' ? pages.value.slice(comicStart.value, comicStart.value + 12) : pages.value.slice(pageIndex.value, pageIndex.value + (comicMode.value === 'double' ? 2 : 1)))
const displayedParagraphs = computed<string[]>((): string[] => textBook.value?.paragraphs.slice(textStart.value, textStart.value + 80) || [])
const currentPosition = computed<number>((): number => props.item.type === 'comic' ? pageIndex.value : isEpub.value ? epubPosition.value : paragraphIndex.value)
const totalPosition = computed<number>((): number => props.item.type === 'comic' ? pages.value.length : isEpub.value ? epubTotal.value : textBook.value?.paragraphs.length || 0)
const imageIndex = computed<number>((): number => props.imageItems?.findIndex((item: Item): boolean => item.id === props.item.id) ?? -1)

/** 合并频繁事件并保存阅读状态，用户关闭时另行强制写入。 */
async function saveProgress(force: boolean = false): Promise<void> {
  if (closed || props.item.type === 'image') return
  if (!force && Date.now() - savedAt < 8000) return
  const position: number = props.item.type === 'video' ? video.value?.currentTime || 0 : currentPosition.value
  const total: number = props.item.type === 'video' ? video.value?.duration || props.item.duration : totalPosition.value
  if (!Number.isFinite(total) || total <= 0) return
  savedAt = Date.now()
  try {
    await api(`/items/${props.item.id}/progress`, 'PUT', { position, total, locator: isEpub.value ? currentLocator : '' })
    saveFailure.value = ''
    emit('updated')
  } catch (error: unknown) { saveFailure.value = `进度暂未同步：${errorMessage(error)}` }
}

/** 获取当前页面的进度快照，关闭页面时使用 keepalive 尽力提交。 */
function flushProgress(): void {
  const position: number = props.item.type === 'video' ? video.value?.currentTime || 0 : currentPosition.value
  const total: number = props.item.type === 'video' ? video.value?.duration || props.item.duration : totalPosition.value
  if (props.item.type !== 'image' && Number.isFinite(total) && total > 0) {
    void fetch(`/api/items/${props.item.id}/progress`, { method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ position, total, locator: isEpub.value ? currentLocator : '' }), keepalive: true }).catch((): void => { console.warn('退出时进度同步失败，将保留最近一次已保存进度') })
  }
}

/** 关闭阅读器，先释放视频网络请求，再恢复原列表。 */
function closeReader(): void {
  flushProgress()
  closed = true
  releaseMedia()
  emit('close')
}

/** 释放播放器、电子书和读取中的网络请求。 */
function releaseMedia(): void {
  controller.abort()
  observer?.disconnect()
  if (video.value) { video.value.pause(); video.value.removeAttribute('src'); video.value.load() }
  rendition?.destroy()
  epubBook?.destroy()
  rendition = null
  epubBook = null
}

/** 读入漫画目录或有限大小的 TXT 文本。 */
async function loadReadingData(): Promise<void> {
  const loadId: number = ++readingLoad
  controller.abort()
  controller = new AbortController()
  const signal: AbortSignal = controller.signal
  observer?.disconnect()
  rendition?.destroy()
  epubBook?.destroy()
  rendition = null
  epubBook = null
  busy.value = true
  failure.value = ''
  try {
    if (props.item.type === 'comic') {
      const result = await api<{ pages: ComicPage[] }>(`/items/${props.item.id}/comic`, 'GET', undefined, signal)
      if (signal.aborted) return
      pages.value = result.pages
      pageIndex.value = Math.min(pageIndex.value, Math.max(0, pages.value.length - 1))
      comicStart.value = Math.floor(pageIndex.value / 12) * 12
      if (!pages.value.length) throw new Error('此漫画还没有可读取的图片页面')
    } else if (props.item.type === 'novel' && !isEpub.value) {
      const result: TextBook = await api<TextBook>(`/items/${props.item.id}/text?encoding=${encodeURIComponent(encoding.value)}`, 'GET', undefined, signal)
      if (signal.aborted) return
      textBook.value = result
      paragraphIndex.value = Math.min(paragraphIndex.value, Math.max(0, textBook.value.paragraphs.length - 1))
      textStart.value = Math.floor(paragraphIndex.value / 80) * 80
    } else if (isEpub.value) { await openEpub(signal) }
    if (signal.aborted) return
    await nextTick()
    observeProgress()
    if (props.item.type === 'novel' && !isEpub.value) scrollToParagraph(paragraphIndex.value)
  } catch (error: unknown) { if (!isAbort(error) && loadId === readingLoad) failure.value = errorMessage(error) }
  finally { if (loadId === readingLoad) busy.value = false }
}

/** 为第三方电子书解析增加超时和取消边界，避免损坏文件永久停留在加载中。 */
function withReadTimeout<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  return new Promise<T>((resolve: (value: T | PromiseLike<T>) => void, reject: (reason: Error) => void): void => {
    const timer: ReturnType<typeof setTimeout> = setTimeout((): void => { cleanup(); reject(new Error('电子书解析超时，请检查文件是否完整')) }, 20000)
    /** 清理本轮超时与退出监听。 */
    function cleanup(): void { clearTimeout(timer); signal.removeEventListener('abort', abort) }
    /** 用户退出时结束等待，第三方实例会在清理阶段销毁。 */
    function abort(): void { cleanup(); reject(new DOMException('读取已取消', 'AbortError')) }
    signal.addEventListener('abort', abort, { once: true })
    if (signal.aborted) { abort(); return }
    promise.then((value: T): void => { cleanup(); resolve(value) }, (error: unknown): void => { cleanup(); reject(error instanceof Error ? error : new Error('电子书解析失败')) })
  })
}

/** 加载安全受限 EPUB，脚本始终禁用且不请求外部资源。 */
async function openEpub(signal: AbortSignal): Promise<void> {
  const data: ArrayBuffer = await fetchEpub(fileUrl.value, signal)
  const { default: ePub } = await import('epubjs')
  if (closed || signal.aborted) return
  const book: Book = ePub({ replacements: 'blobUrl', openAs: 'binary' })
  epubBook = book
  try { await withReadTimeout(book.open(data, 'binary'), signal) }
  catch (error: unknown) { book.destroy(); if (epubBook === book) epubBook = null; throw error }
  if (closed || signal.aborted) { book.destroy(); return }
  // 在章节进入 iframe 前清理；沙箱禁用脚本，CSP 禁止外部连接。
  book.spine.hooks.serialize.register(sanitizeEpubSection)
  const navigation = await withReadTimeout(book.loaded.navigation, signal)
  epubNavigation.value = flattenNavigation(navigation.toc)
  epubTotal.value = Math.max(1, book.packaging.spine.length)
  await nextTick()
  if (!bookHost.value) throw new Error('电子书容器尚未就绪')
  rendition = book.renderTo(bookHost.value, { width: '100%', height: '100%', flow: 'paginated', spread: 'none', allowScriptedContent: false })
  rendition.on('relocated', (location: Location): void => {
    currentLocator = location.start.cfi
    epubPosition.value = location.atEnd ? epubTotal.value : location.start.index + ((location.start.displayed.page - 1) / Math.max(1, location.start.displayed.total))
    void saveProgress(location.atEnd)
  })
  rendition.on('displayError', (error: unknown): void => { failure.value = `章节加载失败：${errorMessage(error)}` })
  applyEpubTheme()
  try { await rendition.display(currentLocator || undefined) }
  catch (error: unknown) {
    if (!currentLocator) throw error
    currentLocator = ''
    await rendition.display()
    saveFailure.value = '原阅读位置已失效，已打开书籍开头。'
  }
  console.info('电子书已在禁用脚本的沙箱中打开')
}

/** 将嵌套目录转换为可选择的章节列表。 */
function flattenNavigation(items: NavItem[], depth: number = 0): NavItem[] {
  return items.flatMap((item: NavItem): NavItem[] => [{ ...item, label: `${'　'.repeat(Math.min(depth, 4))}${item.label}` }, ...flattenNavigation(item.subitems || [], depth + 1)])
}

/** 为电子书内容应用可读主题，不继承书籍中的外层样式。 */
function applyEpubTheme(): void {
  const dark: boolean = document.documentElement.dataset.theme === 'dark'
  rendition?.themes.default({ body: { color: paper.value ? '#3f3528 !important' : dark ? '#e2e5e9 !important' : '#29313b !important', background: 'transparent !important', 'font-family': 'Georgia, "Noto Serif SC", "Songti SC", serif !important', 'line-height': `${lineHeight.value} !important` }, a: { color: dark && !paper.value ? '#5eead4 !important' : '#0f766e !important' }, p: { 'line-height': `${lineHeight.value} !important` } })
  rendition?.themes.fontSize(`${fontSize.value}px`)
}

/** 监听当前可见文本段落或漫画页，保存稳定位置。 */
function observeProgress(): void {
  observer?.disconnect()
  if (!readerBody.value) return
  observer = new IntersectionObserver((entries: IntersectionObserverEntry[]): void => {
    const visible: IntersectionObserverEntry | undefined = entries.filter((entry: IntersectionObserverEntry): boolean => entry.isIntersecting).sort((a: IntersectionObserverEntry, b: IntersectionObserverEntry): number => a.boundingClientRect.top - b.boundingClientRect.top)[0]
    if (!visible) return
    const index: number = Number((visible.target as HTMLElement).dataset.index)
    if (props.item.type === 'comic') pageIndex.value = index
    else paragraphIndex.value = index
    void saveProgress()
  }, { root: readerBody.value, rootMargin: '-8% 0px -65% 0px', threshold: 0 })
  readerBody.value.querySelectorAll('[data-index]').forEach((element: Element): void => observer?.observe(element))
}

/** 漫画跳转到指定页；连续模式只挂载当前十二页，限制长漫画内存。 */
async function jumpPage(target: number): Promise<void> {
  pageIndex.value = Math.max(0, Math.min(pages.value.length - 1, target))
  comicStart.value = Math.floor(pageIndex.value / 12) * 12
  await nextTick()
  if (comicMode.value === 'continuous') {
    observeProgress()
    readerBody.value?.querySelector(`[data-index="${pageIndex.value}"]`)?.scrollIntoView({ block: 'start' })
  } else readerBody.value?.scrollTo({ top: 0 })
  void saveProgress(true)
}

/** 跳转到 TXT 段落并恢复内容视口。 */
async function jumpParagraph(target: number): Promise<void> {
  paragraphIndex.value = Math.max(0, Math.min((textBook.value?.paragraphs.length || 1) - 1, target))
  textBase = Math.floor(paragraphIndex.value / 80) * 80
  textStart.value = textBase
  await nextTick()
  observeProgress()
  scrollToParagraph(paragraphIndex.value)
  showChapters.value = false
  void saveProgress(true)
}

/** 将指定文本段落滚动到阅读器视口。 */
function scrollToParagraph(index: number): void {
  readerBody.value?.querySelector(`[data-index="${index}"]`)?.scrollIntoView({ block: 'start' })
}

/** 在 EPUB 目录中定位到章节。 */
async function jumpEpub(href: string): Promise<void> {
  showChapters.value = false
  try { await rendition?.display(href); void saveProgress(true) }
  catch (error: unknown) { failure.value = errorMessage(error) }
}

/** 前后翻页，同时兼容漫画和电子书。 */
async function turnPage(delta: number): Promise<void> {
  if (isEpub.value) {
    try { if (delta > 0) await rendition?.next(); else await rendition?.prev(); void saveProgress(true) }
    catch (error: unknown) { failure.value = errorMessage(error) }
  } else if (props.item.type === 'comic') await jumpPage(pageIndex.value + delta * (comicMode.value === 'double' ? 2 : comicMode.value === 'continuous' ? 12 : 1))
  else await jumpParagraph(textStart.value + delta * 80)
}

/** 视频元信息到达后只恢复一次进度，避免拖动时重复回跳。 */
function restoreVideo(): void {
  if (!video.value || videoRestored) return
  videoRestored = true
  const previous: number = props.item.progress?.position || 0
  if (previous > 0 && previous < video.value.duration - 2) video.value.currentTime = previous
  videoStatus.value = '原画直放 · 按需读取'
}

/** 显示真实浏览器播放错误，不承诺不存在的转码能力。 */
function videoError(): void {
  failure.value = '当前浏览器无法播放此视频，或文件暂不可读。首版使用原画直放，不含转码。可下载后用兼容播放器打开，或将副本转为 H.264/AAC MP4。'
  videoStatus.value = '播放失败'
}

/** 同一图片列表内切换，保持原列表上下文。 */
function adjacentImage(delta: number): void {
  const next: Item | undefined = props.imageItems?.[imageIndex.value + delta]
  if (next) emit('navigate', next)
}

/** 提供翻页键盘操作，输入字段不拦截默认行为。 */
function onKey(event: KeyboardEvent): void {
  if (event.target instanceof HTMLInputElement || event.target instanceof HTMLSelectElement || event.target instanceof HTMLTextAreaElement || event.altKey || event.ctrlKey || event.metaKey) return
  if (props.item.type === 'video') return
  if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') {
    event.preventDefault()
    const delta: number = event.key === 'ArrowRight' ? 1 : -1
    if (props.item.type === 'image') adjacentImage(delta)
    else void turnPage(direction.value === 'rtl' ? -delta : delta)
  }
}

watch([fontSize, lineHeight, paper], (): void => {
  localStorage.setItem('reader-font-size', String(fontSize.value))
  localStorage.setItem('reader-line-height', lineHeight.value)
  localStorage.setItem('reader-paper', String(paper.value))
  applyEpubTheme()
})
watch(comicMode, (): void => { void jumpPage(pageIndex.value) })
onMounted((): void => {
  document.addEventListener('keydown', onKey)
  window.addEventListener('pagehide', flushProgress)
  if (props.item.type === 'comic' || props.item.type === 'novel') void loadReadingData()
})
onBeforeUnmount((): void => {
  if (!closed) flushProgress()
  closed = true
  releaseMedia()
  document.removeEventListener('keydown', onKey)
  window.removeEventListener('pagehide', flushProgress)
})
</script>

<template>
  <DialogShell :title="`${typeLabels[item.type]}：${item.title}`" fullscreen hide-header @close="closeReader">
    <div class="reader-shell" :class="[{ 'reader-paper': paper && item.type === 'novel' }, `reader-${item.type}`]">
      <header class="reader-header"><button class="icon-button" aria-label="关闭阅读器，返回内容库" @click="closeReader"><Icon name="back" /></button><div class="reader-title"><span>{{ typeLabels[item.type] }}</span><h2>{{ item.title }}</h2></div><span v-if="item.type === 'video'" class="reader-status">{{ videoStatus }}</span><button class="icon-button" :aria-pressed="showInfo" aria-label="查看文件信息" @click="showInfo = !showInfo"><Icon name="info" /></button><button class="icon-button" aria-label="关闭阅读器" @click="closeReader"><Icon name="close" /></button></header>
      <div v-if="showInfo" class="reader-info"><span>{{ extension(item.relativePath) }} · {{ formatSize(item.size) }}</span><span>{{ item.relativePath }}</span><span v-if="item.type === 'video'">直放使用 HTTP Range，只读取播放需要的区间；编码兼容性和跳转速度取决于原文件与浏览器。</span><a :href="fileUrl" download class="text-button"><Icon name="download" />下载原文件</a></div>
      <div v-if="failure" class="reader-error error-box" role="alert"><Icon name="warning" /><span>{{ failure }}</span><button v-if="item.type !== 'video'" class="text-button" @click="loadReadingData">重试</button></div>
      <p v-if="saveFailure" class="reader-save-error" role="status">{{ saveFailure }}</p>
      <div v-if="item.type === 'video'" class="video-stage"><video ref="video" :src="fileUrl" controls playsinline preload="metadata" :poster="item.coverUrl" @loadedmetadata="restoreVideo" @waiting="videoStatus = '缓冲中…'" @playing="videoStatus = '原画直放 · 按需读取'" @canplay="videoStatus = '原画直放 · 按需读取'" @error="videoError" @timeupdate="saveProgress()" @pause="saveProgress(true)" @seeked="saveProgress(true)" @ended="saveProgress(true)" /><p class="video-help">{{ formatSize(item.size) }}<span>·</span>{{ formatDuration(item.duration) }}<span>·</span>原画直放，退出即释放连接</p></div>
      <template v-else-if="item.type === 'image'">
        <div class="image-controls"><button class="secondary-button" :disabled="imageIndex <= 0" @click="adjacentImage(-1)"><Icon name="back" />上一张</button><label>缩放<select v-model.number="imageZoom"><option :value="1">适应窗口</option><option :value="1.5">150%</option><option :value="2">200%</option><option :value="3">300%</option></select></label><button class="secondary-button" :disabled="imageIndex < 0 || imageIndex >= (imageItems?.length || 0) - 1" @click="adjacentImage(1)">下一张<Icon name="chevron" /></button></div>
        <div class="image-stage"><img :src="fileUrl" :alt="item.title" :style="{ maxWidth: imageZoom === 1 ? '100%' : 'none', maxHeight: imageZoom === 1 ? '100%' : 'none', width: imageZoom === 1 ? 'auto' : `${imageZoom * 100}%` }" @error="failure = '图片暂时无法读取，请检查挂载和格式。'" /></div>
      </template>
      <template v-else>
        <div class="reader-toolbar">
          <template v-if="item.type === 'comic'"><label>阅读模式<select v-model="comicMode"><option value="single">单页</option><option value="double">双页</option><option value="continuous">连续滚动（每组 12 页）</option></select></label><label>方向<select v-model="direction"><option value="ltr">从左到右</option><option value="rtl">从右到左</option></select></label><label>页码<select :value="pageIndex" @change="jumpPage(Number(($event.target as HTMLSelectElement).value))"><option v-for="page in pages" :key="page.index" :value="page.index">{{ page.index + 1 }} / {{ pages.length }} · {{ page.name }}</option></select></label></template>
          <template v-else><button class="secondary-button" :aria-expanded="showChapters" @click="showChapters = !showChapters"><Icon name="list" />目录</button><label>字号<select v-model.number="fontSize"><option v-for="size in [16, 18, 20, 22, 24, 28, 32]" :key="size" :value="size">{{ size }}</option></select></label><label>行距<select v-model="lineHeight"><option value="1.6">紧凑</option><option value="1.9">舒适</option><option value="2.2">宽松</option></select></label><button class="secondary-button" :aria-pressed="paper" @click="paper = !paper">{{ paper ? '应用主题' : '纸色阅读' }}</button><label v-if="!isEpub">编码<select v-model="encoding" @change="loadReadingData"><option value="">自动</option><option value="utf-8">UTF-8</option><option value="gb18030">GB18030</option></select></label></template>
        </div>
        <div v-if="showChapters && item.type === 'novel'" class="chapter-panel"><h3>目录</h3><template v-if="isEpub"><button v-for="chapter in epubNavigation" :key="chapter.id + chapter.href" @click="jumpEpub(chapter.href)">{{ chapter.label }}</button><p v-if="!epubNavigation.length" class="muted">书籍未提供目录，可以使用翻页按钮。</p></template><template v-else><button v-for="chapter in textBook?.chapters || []" :key="chapter.index" @click="jumpParagraph(chapter.index)">{{ chapter.title }}</button><button v-if="!textBook?.chapters.length" @click="jumpParagraph(0)">全文开始</button></template></div>
        <div ref="readerBody" class="reader-body" :style="{ '--reader-font-size': `${fontSize}px`, '--reader-line-height': lineHeight }" :aria-busy="busy">
          <p v-if="busy" class="reader-loading" role="status">正在准备{{ isEpub ? '电子书' : typeLabels[item.type] }}…</p>
          <!-- 电子书宿主独立挂载，加载提示绝不影响 renderTo 的 DOM 与尺寸。 -->
          <div v-if="item.type === 'novel' && isEpub" ref="bookHost" class="epub-host" />
          <div v-if="item.type === 'comic'" class="comic-pages" :class="[`comic-${comicMode}`, { 'comic-rtl': direction === 'rtl' }]">
            <figure v-for="page in displayedPages" :key="page.index" :data-index="comicMode === 'continuous' ? page.index : undefined"><img :src="page.url" :alt="`第 ${page.index + 1} 页：${page.name}`" :loading="page.index === pageIndex ? 'eager' : 'lazy'" decoding="async" @error="failure = '部分页面无法读取，请检查漫画文件是否完整。'" /><figcaption>{{ page.index + 1 }} / {{ pages.length }}</figcaption></figure>
          </div>
          <article v-else-if="textBook && !isEpub" class="text-page"><p v-if="textBook.truncated" class="error-box">文本超过读取限额，仅展示部分内容，请拆分为较小文件。</p><p v-for="(paragraph, index) in displayedParagraphs" :key="textStart + index" :data-index="textStart + index">{{ paragraph }}</p><p v-if="!textBook.paragraphs.length" class="muted">此文件没有可读取的正文。</p></article>
        </div>
        <footer class="reader-footer"><button class="secondary-button" :disabled="busy || (!isEpub && (item.type === 'comic' ? pageIndex <= 0 : textStart <= 0))" @click="turnPage(-1)"><Icon name="back" />上一{{ item.type === 'comic' && comicMode === 'continuous' ? '组' : '页' }}</button><span>{{ item.type === 'comic' ? `${Math.min(pageIndex + 1, pages.length)} / ${pages.length} 页` : isEpub ? `第 ${Math.min(Math.floor(epubPosition) + 1, epubTotal)} / ${epubTotal} 节` : `${Math.min(paragraphIndex + 1, totalPosition)} / ${totalPosition} 段` }}</span><button class="secondary-button" :disabled="busy || (!isEpub && (item.type === 'comic' ? pageIndex + (comicMode === 'double' ? 2 : comicMode === 'continuous' ? 12 : 1) >= pages.length : textStart + 80 >= totalPosition))" @click="turnPage(1)">下一{{ item.type === 'comic' && comicMode === 'continuous' ? '组' : '页' }}<Icon name="chevron" /></button></footer>
      </template>
    </div>
  </DialogShell>
</template>
