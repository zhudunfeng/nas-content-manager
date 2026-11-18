<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, errorMessage, isAbort } from './api'
import { extension, formatDate, formatDuration, formatSize, isTimeMedia, progressPercent, typeLabels } from './format'
import { newPasswordHint, validNewPassword } from './password'
import type { ContentType, Item, ItemPage, Job, Library, Settings, Stats, User, ViewName } from './types'
import Icon from './components/Icon.vue'
import MediaCard from './components/MediaCard.vue'
import LibraryDialog from './components/LibraryDialog.vue'
import DialogShell from './components/DialogShell.vue'

// 播放器与 EPUB 解析依赖按需加载，保持首页轻量。
const ReaderDialog = defineAsyncComponent((): Promise<typeof import('./components/ReaderDialog.vue')> => import('./components/ReaderDialog.vue'))
// 下载中心仅在进入页面时加载，基础媒体库保持轻量。
const DownloadCenter = defineAsyncComponent((): Promise<typeof import('./components/DownloadCenter.vue')> => import('./components/DownloadCenter.vue'))
/** 首次启动状态。 */
interface Status { /** 是否需要创建管理员。 */ setupRequired: boolean; /** 当前会话。 */ user: User | null; /** 应用版本。 */ version: string }
/** 导航项定义。 */
interface Navigation { /** 页面标识。 */ id: ViewName; /** 显示名称。 */ label: string; /** 线性图标。 */ icon: string }
const primaryNav: Navigation[] = [{ id: 'home', label: '首页', icon: 'home' }, { id: 'video', label: '视频', icon: 'video' }, { id: 'music', label: '音乐', icon: 'music' }, { id: 'image', label: '图片', icon: 'image' }, { id: 'comic', label: '漫画', icon: 'comic' }, { id: 'novel', label: '小说', icon: 'novel' }]
/** 手机底栏保留四个常用入口，其余通过原生对话框访问。 */
const mobileNav: Navigation[] = primaryNav.filter((entry: Navigation): boolean => ['home', 'video', 'music', 'image'].includes(entry.id))
const secondaryNav: Navigation[] = [{ id: 'favorites', label: '我的收藏', icon: 'heart' }, { id: 'libraries', label: '媒体库', icon: 'folder' }, { id: 'downloads', label: '下载中心', icon: 'download' }, { id: 'jobs', label: '后台任务', icon: 'jobs' }]
const allNavigation: Navigation[] = [...primaryNav, ...secondaryNav, { id: 'settings', label: '设置', icon: 'settings' }]
const status = ref<Status>({ setupRequired: false, user: null, version: '' })
const starting = ref<boolean>(true)
const startupError = ref<string>('')
const authBusy = ref<boolean>(false)
const authError = ref<string>('')
const username = ref<string>('')
const password = ref<string>('')
const passwordConfirm = ref<string>('')
const view = ref<ViewName>('home')
const theme = ref<'dark' | 'light'>(localStorage.getItem('theme') === 'light' ? 'light' : localStorage.getItem('theme') === 'dark' ? 'dark' : matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
const search = ref<string>('')
const debouncedSearch = ref<string>('')
const sort = ref<string>('recent')
const libraryFilter = ref<string>('')
const page = ref<number>(1)
const items = ref<Item[]>([])
const continuing = ref<Item[]>([])
const total = ref<number>(0)
const loading = ref<boolean>(false)
const contentError = ref<string>('')
const metadataError = ref<string>('')
const libraries = ref<Library[]>([])
const jobs = ref<Job[]>([])
const settings = ref<Settings | null>(null)
const stats = ref<Stats>({ total: 0, video: 0, image: 0, comic: 0, novel: 0, music: 0, totalSize: 0 })
const showLibraryDialog = ref<boolean>(false)
const showMoreNavigation = ref<boolean>(false)
const selected = ref<Item | null>(null)
const reading = ref<Item | null>(null)
const tagsText = ref<string>('')
const tagsBusy = ref<boolean>(false)
const detailError = ref<string>('')
const deleting = ref<Library | null>(null)
const actionBusy = ref<boolean>(false)
const toast = ref<string>('')
let toastTimer: ReturnType<typeof setTimeout> | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined
let pollTimer: ReturnType<typeof setInterval> | undefined
let contentController: AbortController | null = null
let statusController: AbortController | null = null
let activeBefore: boolean = false
let savedListScroll: number = 0
const searchInput = ref<HTMLInputElement | null>(null)
const isContentView = computed<boolean>((): boolean => ['home', 'video', 'image', 'comic', 'novel', 'music', 'favorites'].includes(view.value))
const moreNavigationActive = computed<boolean>((): boolean => !mobileNav.some((entry: Navigation): boolean => entry.id === view.value))
const activeJobs = computed<Job[]>((): Job[] => jobs.value.filter((job: Job): boolean => job.status === 'running' || job.status === 'queued'))
const filteredLibraries = computed<Library[]>((): Library[] => libraries.value.filter((library: Library): boolean => !(view.value in typeLabels) || library.type === view.value))
const pageTitle = computed<string>((): string => view.value === 'home' ? (debouncedSearch.value ? '搜索你的内容' : '欢迎回到栖库') : allNavigation.find((entry: Navigation): boolean => entry.id === view.value)?.label || '内容库')
const pageDescription = computed<string>((): string => {
  const descriptions: Record<ViewName, string> = { home: '喜欢的影像、音乐与文字，都在这里。', video: '把好故事，留给属于自己的时间。', image: '那些值得留下的日常，随时重温。', comic: '翻开下一页，走进另一个世界。', novel: '让文字慢下来，让阅读继续。', music: '让熟悉的旋律，陪伴此刻的日常。', favorites: '不必再找，喜欢的都收在这里。', libraries: '连接你的内容目录，其余交给栖库。', downloads: '从下载到收藏，让喜欢的内容有序归位。', jobs: '索引与封面在后台处理，浏览不必等待。', settings: '你的内容空间，由你掌握。' }
  return descriptions[view.value]
})
const homeShelves = computed<boolean>((): boolean => view.value === 'home' && !debouncedSearch.value && !libraryFilter.value && page.value === 1)
const imageItems = computed<Item[]>((): Item[] => items.value.filter((item: Item): boolean => item.type === 'image'))

/** 弹出简短操作反馈，避免阻断连续浏览。 */
function notify(message: string): void {
  toast.value = message
  clearTimeout(toastTimer)
  toastTimer = setTimeout((): void => { toast.value = '' }, 4500)
}

/** 读取登录状态，首次设置和会话恢复共用入口。 */
async function boot(): Promise<void> {
  starting.value = true
  startupError.value = ''
  statusController?.abort()
  statusController = new AbortController()
  try {
    status.value = await api<Status>('/status', 'GET', undefined, statusController.signal)
    if (status.value.user) await initializeLibrary()
  } catch (error: unknown) { if (!isAbort(error)) startupError.value = errorMessage(error) }
  finally { starting.value = false }
}

/** 创建首个管理员或登录已有账户。 */
async function authenticate(): Promise<void> {
  authError.value = ''
  const usernameLength: number = Array.from(username.value.trim()).length
  const passwordBytes: number = new TextEncoder().encode(password.value).byteLength
  if (usernameLength < 2 || usernameLength > 40) { authError.value = '用户名需要 2–40 个字符'; return }
  if (status.value.setupRequired && password.value !== passwordConfirm.value) { authError.value = '两次输入的密码不一致'; return }
  // 新规则只用于创建账户，已有账户继续使用原密码登录。
  if (status.value.setupRequired && !validNewPassword(password.value)) { authError.value = newPasswordHint; return }
  if (!status.value.setupRequired && passwordBytes > 72) { authError.value = '密码不能超过 72 个 UTF-8 字节'; return }
  authBusy.value = true
  try {
    const result = await api<{ user: User }>(status.value.setupRequired ? '/setup' : '/login', 'POST', { username: username.value.trim(), password: password.value })
    status.value.user = result.user
    status.value.setupRequired = false
    password.value = ''
    passwordConfirm.value = ''
    console.info('管理员会话已建立')
    await initializeLibrary()
  } catch (error: unknown) { authError.value = errorMessage(error) }
  finally { authBusy.value = false }
}

/** 加载登录后的媒体库、首页与周期任务状态。 */
async function initializeLibrary(): Promise<void> {
  await refreshMetadata()
  if (view.value === 'settings') await loadSettings()
  else await loadItems()
  clearInterval(pollTimer)
  pollTimer = setInterval((): void => { void pollJobs() }, 4000)
}

/** 刷新媒体库与索引统计，不把单一失败隐藏为有效空库。 */
async function refreshMetadata(): Promise<void> {
  if (!status.value.user) return
  try {
    const [libraryResult, statsResult, jobResult] = await Promise.all([api<{ libraries: Library[] }>('/libraries'), api<Stats>('/stats'), api<{ jobs: Job[] }>('/jobs')])
    libraries.value = libraryResult.libraries
    stats.value = statsResult
    jobs.value = jobResult.jobs
    activeBefore = activeJobs.value.length > 0
    metadataError.value = ''
  } catch (error: unknown) { metadataError.value = errorMessage(error) }
}

/** 周期检查后台任务；活动期间轻量更新列表，失败可在任务页恢复。 */
async function pollJobs(): Promise<void> {
  if (!status.value.user || document.hidden) return
  try {
    const result = await api<{ jobs: Job[] }>('/jobs')
    jobs.value = result.jobs
    const activeNow: boolean = activeJobs.value.length > 0
    if (activeBefore || activeNow) {
      await refreshMetadata()
      if (isContentView.value && !reading.value && !selected.value) await loadItems(true)
    }
    activeBefore = activeNow
  } catch (error: unknown) { if (!(error instanceof Error && error.message.includes('会话'))) console.warn('后台任务状态暂未更新') }
}

/** 分页读取索引；取消过期搜索，避免旧请求覆盖新筛选。 */
async function loadItems(silent: boolean = false): Promise<void> {
  if (!status.value.user || !isContentView.value) return
  contentController?.abort()
  const controller: AbortController = new AbortController()
  contentController = controller
  if (!silent) loading.value = true
  contentError.value = ''
  const query: URLSearchParams = new URLSearchParams({ page: String(page.value), pageSize: '48', sort: sort.value })
  if (view.value in typeLabels) query.set('type', view.value)
  if (view.value === 'favorites') query.set('favorite', 'true')
  if (debouncedSearch.value) query.set('q', debouncedSearch.value)
  if (libraryFilter.value) query.set('libraryId', libraryFilter.value)
  try {
    const result = await api<ItemPage>(`/items?${query}`, 'GET', undefined, controller.signal)
    if (controller.signal.aborted) return
    items.value = result.items
    total.value = result.total
    if (homeShelves.value) {
      const progressResult = await api<ItemPage>('/items?sort=progress&page=1&pageSize=12', 'GET', undefined, controller.signal)
      continuing.value = progressResult.items.filter((item: Item): boolean => !!item.progress && item.type !== 'image' && progressPercent(item) < 99.5).slice(0, 6)
    }
  } catch (error: unknown) { if (!isAbort(error)) contentError.value = errorMessage(error) }
  finally { if (contentController === controller) loading.value = false }
}

/** 保持导航与 URL 哈希一致，支持浏览器前进后退。 */
function navigate(target: ViewName): void {
  showMoreNavigation.value = false
  if (target === view.value) return
  view.value = target
  page.value = 1
  libraryFilter.value = ''
  location.hash = target
  window.scrollTo({ top: 0 })
}

/** 根据浏览器历史恢复有效导航。 */
function restoreHash(): void {
  const target: string = location.hash.slice(1)
  if (allNavigation.some((entry: Navigation): boolean => entry.id === target)) view.value = target as ViewName
}

/** 翻到内容列表相邻页，并把焦点所在区域恢复到列表顶部。 */
function changePage(delta: number): void {
  page.value += delta
  window.scrollTo({ top: 0 })
}

/** 保存主题选择，明暗使用同一套语义颜色令牌。 */
function toggleTheme(): void {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  localStorage.setItem('theme', theme.value)
}

/** 收藏状态由服务端确认后更新所有相关卡片。 */
async function toggleFavorite(item: Item): Promise<void> {
  try {
    const next: boolean = !item.favorite
    await api(`/items/${item.id}/favorite`, 'POST', { favorite: next })
    for (const record of [...items.value, ...continuing.value]) if (record.id === item.id) record.favorite = next
    if (selected.value?.id === item.id) selected.value.favorite = next
    notify(next ? '已加入收藏' : '已取消收藏')
    if (view.value === 'favorites') void loadItems(true)
  } catch (error: unknown) { notify(errorMessage(error)) }
}

/** 显示内容详情与标签，不预先下载媒体原文件。 */
function openDetails(item: Item): void {
  selected.value = { ...item, tags: [...item.tags] }
  tagsText.value = item.tags.join('，')
  detailError.value = ''
}

/** 将用户标签按中英文逗号分隔后保存。 */
async function saveTags(): Promise<void> {
  if (!selected.value) return
  tagsBusy.value = true
  detailError.value = ''
  const tags: string[] = [...new Set(tagsText.value.split(/[,，]/).map((tag: string): string => tag.trim()).filter(Boolean))]
  try {
    await api(`/items/${selected.value.id}/tags`, 'PUT', { tags })
    selected.value.tags = tags
    for (const item of [...items.value, ...continuing.value]) if (item.id === selected.value.id) item.tags = tags
    notify('标签已保存')
  } catch (error: unknown) { detailError.value = errorMessage(error) }
  finally { tagsBusy.value = false }
}

/** 进入沉浸模式前保存列表位置，关闭后恢复。 */
async function startReading(item: Item): Promise<void> {
  if (item.missing) { notify('源文件暂不可用，请检查媒体挂载并重新扫描'); return }
  try {
    const result = await api<{ item: Item }>(`/items/${item.id}`)
    savedListScroll = window.scrollY
    selected.value = null
    await nextTick()
    reading.value = result.item
  } catch (error: unknown) { notify(errorMessage(error)) }
}

/** 关闭阅读器后更新进度，并恢复原来的浏览位置。 */
async function stopReading(): Promise<void> {
  reading.value = null
  await nextTick()
  window.scrollTo({ top: savedListScroll })
  await loadItems(true)
}

/** 媒体库添加后在现有页面立即显示后台扫描状态。 */
async function libraryCreated(library: Library): Promise<void> {
  showLibraryDialog.value = false
  libraries.value.push(library)
  notify('媒体库已添加，正在后台扫描')
  await refreshMetadata()
  await loadItems()
}

/** 对已有媒体库提交手动扫描。 */
async function scanLibrary(library: Library): Promise<void> {
  try {
    await api(`/libraries/${library.id}/scan`, 'POST')
    notify(`“${library.name}”扫描已排队`)
    await refreshMetadata()
  } catch (error: unknown) { notify(errorMessage(error)) }
}

/** 删除媒体库索引，源文件始终保留。 */
async function deleteLibrary(): Promise<void> {
  if (!deleting.value) return
  actionBusy.value = true
  try {
    await api(`/libraries/${deleting.value.id}`, 'DELETE')
    deleting.value = null
    notify('媒体库索引已移除，原文件保持不变')
    await refreshMetadata()
  } catch (error: unknown) { notify(errorMessage(error)) }
  finally { actionBusy.value = false }
}

/** 取消或重新提交后台任务。 */
async function jobAction(job: Job, action: 'cancel' | 'retry'): Promise<void> {
  try {
    await api(`/jobs/${job.id}/${action}`, 'POST')
    notify(action === 'cancel' ? '已请求取消任务' : '任务已重新排队')
    await refreshMetadata()
  } catch (error: unknown) { notify(errorMessage(error)) }
}

/** 读取服务端配置状态，防止将硬件能力猜测为已启用。 */
async function loadSettings(): Promise<void> {
  if (!status.value.user) return
  contentError.value = ''
  try { settings.value = await api<Settings>('/settings') }
  catch (error: unknown) { contentError.value = errorMessage(error) }
}

/** 退出账户并停止轮询，媒体仍保存在原目录。 */
async function logout(): Promise<void> {
  try {
    await api('/logout', 'POST')
    expireSession()
    notify('已安全退出')
  } catch (error: unknown) { notify(errorMessage(error)) }
}

/** 会话失效时释放私有状态并显示登录界面。 */
function expireSession(): void {
  status.value.user = null
  showMoreNavigation.value = false
  items.value = []
  continuing.value = []
  selected.value = null
  reading.value = null
  libraries.value = []
  metadataError.value = ''
  clearInterval(pollTimer)
  contentController?.abort()
}

/** 转换后台状态为稳定的中文标签。 */
function jobLabel(value: string): string {
  return ({ queued: '等待中', running: '处理中', completed: '已完成', failed: '失败', cancelled: '已取消', scanning: '扫描中', ready: '可用', idle: '就绪', offline: '离线' } as Record<string, string>)[value] || value
}

/** 内容类型安全地从统计记录读取数量。 */
function navCount(id: ViewName): number {
  return id in typeLabels ? stats.value[id as ContentType] : 0
}

/** 提供搜索快捷键，不干扰输入框和系统快捷键。 */
function shortcut(event: KeyboardEvent): void {
  if ((event.ctrlKey || event.metaKey) && event.key === 'k' && status.value.user && !reading.value) { event.preventDefault(); searchInput.value?.focus() }
}

watch(theme, (value: string): void => { document.documentElement.dataset.theme = value }, { immediate: true })
watch(search, (value: string): void => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout((): void => { if (!isContentView.value) navigate('home'); debouncedSearch.value = value.trim(); page.value = 1 }, 300)
})
watch([view, debouncedSearch, sort, libraryFilter, page], (): void => {
  // 恢复 URL 时会话可能仍在启动读取中，登录后统一由初始化入口加载私有数据。
  if (!status.value.user) return
  contentError.value = ''
  if (view.value === 'settings') void loadSettings()
  else if (isContentView.value) void loadItems()
  else void refreshMetadata()
})
watch(selected, (item: Item | null): void => { if (!item) detailError.value = '' })
onMounted((): void => {
  restoreHash()
  void boot()
  window.addEventListener('hashchange', restoreHash)
  window.addEventListener('session-expired', expireSession)
  document.addEventListener('keydown', shortcut)
})
onBeforeUnmount((): void => {
  clearInterval(pollTimer)
  clearTimeout(searchTimer)
  clearTimeout(toastTimer)
  contentController?.abort()
  statusController?.abort()
  window.removeEventListener('hashchange', restoreHash)
  window.removeEventListener('session-expired', expireSession)
  document.removeEventListener('keydown', shortcut)
})
</script>

<template>
  <div v-if="starting" class="boot-screen"><div class="brand-mark"><Icon name="comic" :size="32" /></div><span class="loader" /><p>正在打开你的内容空间…</p></div>
  <div v-else-if="startupError" class="boot-screen"><Icon name="warning" :size="40" /><h1>暂时无法连接栖库</h1><p>{{ startupError }}</p><button class="primary-button" @click="boot">重新连接</button></div>
  <div v-else-if="!status.user" class="auth-page">
    <button class="icon-button auth-theme" :aria-label="`切换到${theme === 'dark' ? '浅色' : '深色'}主题`" @click="toggleTheme"><Icon :name="theme === 'dark' ? 'sun' : 'moon'" /></button>
    <div class="auth-story"><div class="brand"><div class="brand-mark"><Icon name="comic" :size="28" /></div><span>栖库<small>QIKU</small></span></div><div class="auth-art" aria-hidden="true"><div class="art-frame art-video"><Icon name="play" :size="42" /><span>光影</span></div><div class="art-frame art-book"><span>在文字里<br />遇见远方。</span><Icon name="novel" :size="32" /></div><div class="art-frame art-photo"><svg viewBox="0 0 240 180"><circle cx="175" cy="48" r="20" fill="currentColor" opacity=".5"/><path d="m0 150 70-85 62 66 30-35 78 84H0Z" fill="currentColor" opacity=".25"/><path d="m0 180 100-90 90 90Z" fill="currentColor" opacity=".3"/></svg></div></div><h1>给喜欢的内容，<br />一个自己的家。</h1><p>视频、音乐、照片、漫画与小说。<br />一处收藏，随时继续。</p><div class="auth-note"><span class="status-dot" />本地存储 · 私人空间 · 原文件只读</div></div>
    <main class="auth-main"><form class="auth-form" @submit.prevent="authenticate"><span class="eyebrow">{{ status.setupRequired ? '让我们开始' : '欢迎回来' }}</span><h2>{{ status.setupRequired ? '创建你的内容空间' : '登录栖库' }}</h2><p class="muted">{{ status.setupRequired ? '先创建管理员，然后选择已挂载的媒体目录。' : '登录后，继续喜欢的影像、旋律与文字。' }}</p><ol v-if="status.setupRequired" class="setup-steps"><li class="active"><span>1</span>创建账户</li><li><span>2</span>添加目录</li><li><span>3</span>开始浏览</li></ol><label>用户名<input v-model="username" required minlength="2" maxlength="40" autocomplete="username" autofocus placeholder="输入你的用户名" /></label><label>密码<input v-model="password" required minlength="1" maxlength="72" type="password" :autocomplete="status.setupRequired ? 'new-password' : 'current-password'" :placeholder="status.setupRequired ? '大小写字母 + 数字 + 特殊字符' : '输入密码'" :aria-describedby="status.setupRequired ? 'new-password-hint' : undefined" /></label><p v-if="status.setupRequired" id="new-password-hint" class="field-hint">{{ newPasswordHint }}</p><label v-if="status.setupRequired">确认密码<input v-model="passwordConfirm" required minlength="1" maxlength="72" type="password" autocomplete="new-password" placeholder="再次输入密码" /></label><p v-if="authError" class="error-box" role="alert">{{ authError }}</p><button type="submit" class="primary-button auth-submit" :disabled="authBusy">{{ authBusy ? '正在连接…' : status.setupRequired ? '创建并进入' : '进入内容空间' }}<Icon name="arrow" /></button><p class="auth-footnote">{{ status.setupRequired ? '账户与索引保存在 NAS 本地，不需要注册云端账号。' : '账户保存在 NAS 本地，请妥善保管密码和数据库备份。' }}</p></form><p class="auth-copyright">栖库 QIKU <span>·</span> 让收藏成为日常</p></main>
  </div>
  <div v-else class="app-shell">
    <a href="#main-content" class="skip-link">跳转到主要内容</a>
    <aside class="sidebar"><button class="brand brand-button" aria-label="栖库首页" @click="navigate('home')"><div class="brand-mark"><Icon name="comic" :size="26" /></div><span>栖库<small>QIKU</small></span></button><span class="nav-caption">我的内容空间</span><nav aria-label="主要导航"><button v-for="entry in primaryNav" :key="entry.id" class="nav-item" :aria-label="entry.label" :class="{ active: view === entry.id }" :aria-current="view === entry.id ? 'page' : undefined" @click="navigate(entry.id)"><Icon :name="entry.icon" /><span>{{ entry.label }}</span><small v-if="navCount(entry.id)">{{ navCount(entry.id).toLocaleString() }}</small></button></nav><div class="nav-divider" /><nav aria-label="内容管理"><button v-for="entry in secondaryNav" :key="entry.id" class="nav-item" :aria-label="entry.label" :class="{ active: view === entry.id }" :aria-current="view === entry.id ? 'page' : undefined" @click="navigate(entry.id)"><Icon :name="entry.icon" /><span>{{ entry.label }}</span><small v-if="entry.id === 'jobs' && activeJobs.length" class="task-count">{{ activeJobs.length }}</small></button></nav><div class="sidebar-bottom"><div class="local-badge"><Icon name="storage" /><div><span>私有内容库</span><small>{{ stats.total.toLocaleString() }} 个内容 · {{ formatSize(stats.totalSize) }}</small></div><span class="status-dot" /></div><button class="nav-item" aria-label="设置" :class="{ active: view === 'settings' }" @click="navigate('settings')"><Icon name="settings" /><span>设置</span></button><div class="profile-row"><span class="avatar">{{ status.user.username.slice(0, 1).toUpperCase() }}</span><div><strong>{{ status.user.username }}</strong><small>空间管理员</small></div><button class="icon-button" aria-label="退出登录" @click="logout"><Icon name="logout" :size="18" /></button></div></div></aside>
    <div class="main-column"><header class="topbar"><button class="mobile-brand" aria-label="返回首页" @click="navigate('home')"><Icon name="comic" :size="24" />栖库</button><label class="search-field"><Icon name="search" /><input ref="searchInput" v-model="search" aria-label="搜索内容、文件名或标签" placeholder="搜索内容、文件名或标签…" /><kbd>⌘ / Ctrl K</kbd><button v-if="search" class="search-clear icon-button" aria-label="清除搜索" @click="search = ''"><Icon name="close" :size="16" /></button></label><div class="topbar-actions"><button class="icon-button" :aria-label="`切换到${theme === 'dark' ? '浅色' : '深色'}主题`" @click="toggleTheme"><Icon :name="theme === 'dark' ? 'sun' : 'moon'" /></button><button class="icon-button mobile-manage" aria-label="媒体库与设置" @click="navigate(view === 'libraries' ? 'settings' : 'libraries')"><Icon name="settings" /></button><button class="primary-button add-top" @click="showLibraryDialog = true"><Icon name="plus" /><span>添加媒体库</span></button></div></header>
      <main id="main-content" class="main-content" tabindex="-1"><div class="page-heading"><div><span class="eyebrow">{{ view === 'home' ? 'YOUR PRIVATE COLLECTION' : view === 'favorites' ? 'COLLECTED WITH LOVE' : isContentView ? 'EXPLORE YOUR LIBRARY' : 'MAKE IT YOURS' }}</span><h1>{{ pageTitle }}<span v-if="view === 'home' && !debouncedSearch" class="heading-dot">.</span></h1><p>{{ pageDescription }}</p></div><button v-if="['libraries', 'jobs', 'settings'].includes(view)" class="secondary-button" @click="view === 'settings' ? loadSettings() : refreshMetadata()"><Icon name="refresh" />刷新</button><span v-else-if="isContentView && stats.total" class="collection-count">{{ total.toLocaleString() }} 个内容</span></div>
        <div v-if="activeJobs.length && view !== 'jobs'" class="scan-banner" role="status"><span class="loader" /><span>{{ activeJobs.length }} 个后台任务正在处理<span class="desktop-only"> · 新内容会陆续出现在媒体库</span></span><button class="text-button" @click="navigate('jobs')">查看任务<Icon name="chevron" :size="16" /></button></div>
        <p v-if="metadataError" class="error-box content-error" role="alert"><Icon name="warning" />媒体库信息暂未加载：{{ metadataError }}<button class="text-button" @click="refreshMetadata">重试</button></p>
        <p v-if="contentError" class="error-box content-error" role="alert"><Icon name="warning" />{{ contentError }}<button class="text-button" @click="isContentView ? loadItems() : view === 'settings' ? loadSettings() : refreshMetadata()">重试</button></p>
        <template v-if="isContentView">
          <div v-if="homeShelves && libraries.length" class="type-shortcuts"><button v-for="(label, key) in typeLabels" :key="key" :class="`shortcut-${key}`" @click="navigate(key)"><span class="shortcut-icon"><Icon :name="key" :size="24" /></span><span>{{ label }}<small>{{ stats[key].toLocaleString() }} 个{{ key === 'video' ? '视频' : key === 'image' ? '瞬间' : key === 'comic' ? '故事' : '作品' }}</small></span><Icon name="arrow" :size="18" /></button></div>
          <section v-if="homeShelves && continuing.length" class="shelf"><div class="section-heading"><h2>继续观看、收听与阅读<span>CONTINUE</span></h2></div><div class="continue-grid"><MediaCard v-for="item in continuing" :key="item.id" :item="item" wide @open="startReading" @favorite="toggleFavorite" /></div></section>
          <div v-if="libraries.length" class="content-toolbar"><div><h2 v-if="homeShelves">最近添加<span class="section-english">RECENTLY ADDED</span></h2><span v-else class="result-count">{{ debouncedSearch ? `“${debouncedSearch}” · ` : '' }}{{ total }} 个结果</span></div><div class="filter-controls"><select v-model="libraryFilter" aria-label="筛选媒体库" @change="page = 1"><option value="">全部媒体库</option><option v-for="library in filteredLibraries" :key="library.id" :value="String(library.id)">{{ library.name }}</option></select><select v-model="sort" aria-label="内容排序"><option value="recent">最近添加</option><option value="title">标题顺序</option><option value="progress">最近播放 / 阅读</option></select></div></div>
          <div v-if="loading && !items.length" class="media-grid" aria-label="正在加载内容" aria-busy="true"><div v-for="skeleton in 8" :key="skeleton" class="skeleton-card"><div /><span /><small /></div></div>
          <div v-else-if="!items.length && !contentError && !metadataError" class="empty-state" :class="{ 'welcome-empty': !libraries.length }"><div class="empty-art" aria-hidden="true"><span /><div><Icon :name="!libraries.length ? 'folder' : view === 'favorites' ? 'heart' : debouncedSearch ? 'search' : view === 'home' ? 'book' : view" :size="48" /></div><span /></div><span class="eyebrow">{{ !libraries.length ? '一切从这里开始' : debouncedSearch ? 'SEARCH YOUR COLLECTION' : '留一点空间，给喜欢的内容' }}</span><h2>{{ !libraries.length ? '让你的收藏，在这里相遇' : debouncedSearch ? '没有找到匹配的内容' : view === 'favorites' ? '把喜欢的内容，留在手边' : activeJobs.length ? '正在整理你的内容' : '这里还没有内容' }}</h2><p>{{ !libraries.length ? '添加 NAS 上的媒体目录，栖库会自动建立索引。\n原文件保持原样，浏览与阅读从此更简单。' : debouncedSearch ? '试试其他关键词，或切换媒体库筛选。' : view === 'favorites' ? '点亮内容卡片上的爱心，喜欢的影像、音乐和文字会收集在这里。' : activeJobs.length ? '扫描在后台进行，发现的内容会陆续显示。你可以先去看看任务进度。' : '请检查此目录的格式是否支持，或添加一个新的媒体库。' }}</p><button v-if="!libraries.length || (!debouncedSearch && view !== 'favorites')" class="primary-button" @click="!libraries.length || !activeJobs.length ? showLibraryDialog = true : navigate('jobs')"><Icon :name="activeJobs.length ? 'jobs' : 'plus'" />{{ activeJobs.length ? '查看后台任务' : '添加第一个媒体库' }}</button><button v-if="debouncedSearch" class="secondary-button" @click="search = ''; libraryFilter = ''">清除筛选</button><div v-if="!libraries.length" class="supported-types"><span><Icon name="video" />视频直放</span><span><Icon name="image" />图片浏览</span><span><Icon name="comic" />漫画阅读</span><span><Icon name="novel" />TXT / EPUB</span><span><Icon name="music" />音乐播放</span></div></div>
          <div v-else-if="items.length" class="media-grid" :class="{ 'photo-grid': view === 'image', 'music-grid': view === 'music', 'book-grid': view === 'novel' || view === 'comic' }" :aria-busy="loading"><MediaCard v-for="item in items" :key="item.id" :item="item" @open="openDetails" @favorite="toggleFavorite" /></div>
          <div v-if="total > 48" class="pagination"><button class="secondary-button" :disabled="page <= 1 || loading" @click="changePage(-1)"><Icon name="back" />上一页</button><span>第 {{ page }} / {{ Math.ceil(total / 48) }} 页</span><button class="secondary-button" :disabled="page * 48 >= total || loading" @click="changePage(1)">下一页<Icon name="chevron" /></button></div>
        </template>
        <DownloadCenter v-else-if="view === 'downloads'" @open-item="openDetails" />
        <template v-else-if="view === 'libraries'">
          <div class="section-note"><Icon name="info" /><p>媒体库连接部署时挂载的目录。移除媒体库只删除索引，不会删除你的原文件。</p><button class="primary-button" @click="showLibraryDialog = true"><Icon name="plus" />添加媒体库</button></div>
          <div v-if="libraries.length" class="library-grid"><article v-for="library in libraries" :key="library.id" class="library-card"><div class="library-card-head"><span class="library-icon" :class="`shortcut-${library.type}`"><Icon :name="library.type" :size="28" /></span><span class="status-pill" :class="{ warning: library.status === 'offline' || library.status === 'failed' }">{{ jobLabel(library.status) }}</span></div><h2>{{ library.name }}</h2><p class="library-path">{{ library.path }}</p><div class="library-meta"><span>{{ typeLabels[library.type] }} · {{ library.itemCount }} 个内容</span><span>扫描 {{ formatDate(library.lastScannedAt) }}</span></div><div class="library-actions"><button class="secondary-button" @click="scanLibrary(library)"><Icon name="refresh" :size="16" />重新扫描</button><button class="icon-button danger-text" :aria-label="`移除媒体库：${library.name}`" @click="deleting = library"><Icon name="trash" :size="18" /></button></div></article></div><div v-else-if="!metadataError" class="empty-state"><Icon name="folder" :size="52" /><h2>还没有连接媒体目录</h2><p>添加已挂载目录，即可开始整理你的内容。</p><button class="primary-button" @click="showLibraryDialog = true"><Icon name="plus" />添加媒体库</button></div>
        </template>
        <template v-else-if="view === 'jobs'"><div class="section-note"><Icon name="jobs" /><p>任务与进度保存在本地数据库，重启后会继续处理。失败任务可重试，取消不会删除已发现的内容。</p></div><div v-if="jobs.length" class="job-list"><article v-for="job in jobs" :key="job.id" class="job-row"><div class="job-status-icon" :class="`job-${job.status}`"><span v-if="job.status === 'running'" class="loader" /><Icon v-else :name="job.status === 'completed' ? 'check' : job.status === 'failed' ? 'warning' : job.status === 'cancelled' ? 'close' : 'clock'" /></div><div class="job-copy"><h2>{{ libraries.find((library): boolean => library.id === job.libraryId)?.name || `媒体库 #${job.libraryId}` }}<span class="status-pill" :class="{ warning: job.status === 'failed' }">{{ jobLabel(job.status) }}</span></h2><p>{{ job.message || (job.kind === 'scan' ? '索引媒体目录' : '生成元数据和封面') }}</p><small>{{ job.kind === 'scan' ? '目录扫描' : '衍生处理' }} · 已处理 {{ job.processed }} 项 · {{ formatDate(job.updatedAt) }}</small></div><button v-if="job.status === 'running' || job.status === 'queued'" class="secondary-button" @click="jobAction(job, 'cancel')">取消</button><button v-else-if="job.status === 'failed' || job.status === 'cancelled'" class="secondary-button" @click="jobAction(job, 'retry')"><Icon name="refresh" :size="16" />重试</button></article></div><div v-else-if="!metadataError" class="empty-state"><Icon name="check" :size="48" /><h2>一切就绪</h2><p>添加媒体库后，扫描和封面任务会显示在这里。</p></div></template>
        <template v-else-if="view === 'settings'"><div class="settings-layout"><section class="settings-panel"><div class="section-heading"><h2>空间与存储</h2><span class="status-pill">{{ settings?.version || status.version }}</span></div><dl class="settings-list"><div><dt>业务数据目录</dt><dd>{{ settings?.dataDir || '正在读取…' }}</dd></div><div><dt>允许访问的媒体目录</dt><dd><span v-for="root in settings?.mediaRoots || []" :key="root" class="path-line">{{ root }}</span></dd></div><div><dt>自动扫描间隔</dt><dd>{{ settings?.scanIntervalMinutes }} 分钟</dd></div><div><dt>衍生缓存上限</dt><dd>{{ formatSize(settings?.cacheMaxBytes || 0) }}</dd></div><div><dt>FFmpeg / ffprobe</dt><dd>{{ settings?.ffmpegAvailable ? 'FFmpeg 可用' : 'FFmpeg 不可用' }} · {{ settings?.ffprobeAvailable ? 'ffprobe 可用' : 'ffprobe 不可用' }}</dd></div></dl><p class="field-hint">以上参数在部署环境变量中配置。业务数据库应保存在 NAS 本地磁盘，媒体可来自只读挂载。</p></section><section class="settings-panel backup-panel"><div class="backup-icon"><Icon name="storage" :size="30" /></div><h2>为收藏留一份备份</h2><p>下载数据库一致性快照，保留账户、媒体索引、收藏、标签与阅读进度。原始媒体需要另外备份。</p><a class="primary-button" href="/api/backup" download><Icon name="download" />下载数据备份</a><p class="field-hint">恢复步骤：停止应用 → 保留当前数据 → 替换备份数据库 → 重启并扫描。详细说明见项目 README。</p></section><section class="settings-panel"><h2>外观与账户</h2><div class="setting-row"><div><strong>应用主题</strong><p class="muted">默认跟随系统，也可以固定选择。</p></div><button class="secondary-button" @click="toggleTheme"><Icon :name="theme === 'dark' ? 'sun' : 'moon'" />切换到{{ theme === 'dark' ? '浅色' : '深色' }}</button></div><div class="setting-row"><div><strong>{{ status.user.username }}</strong><p class="muted">首版空间管理员</p></div><button class="secondary-button" @click="logout"><Icon name="logout" />退出账户</button></div></section><section class="settings-panel"><h2>首版格式与能力</h2><ul class="capability-list"><li><Icon name="video" /><span>视频原画直放，支持 Range 与大文件续播；具体编码兼容性取决于浏览器，暂不含转码。</span></li><li><Icon name="music" /><span>MP3、M4A、AAC、FLAC、OGG、OPUS、WAV 与 AIF/AIFF，支持内嵌封面与续播；编码兼容性取决于浏览器，暂不含转码。</span></li><li><Icon name="image" /><span>常见 JPEG、PNG、WebP 图片预览与后台缩略图。</span></li><li><Icon name="comic" /><span>图片目录与 CBZ，支持自然页序、双页和连续滚动。</span></li><li><Icon name="novel" /><span>UTF-8 / GB18030 TXT 与无 DRM EPUB。EPUB 限 64 MiB，书籍脚本禁用。</span></li></ul></section></div></template>
        <footer class="page-footer"><span>栖库 · 你的私人内容空间</span><span>内容在本地，故事在继续。</span><div class="mobile-settings-links"><button class="text-button" @click="navigate('favorites')">收藏</button><button class="text-button" @click="navigate('jobs')">任务</button><button class="text-button" @click="navigate('settings')">设置</button></div></footer>
      </main>
    </div>
    <nav class="bottom-nav" aria-label="手机主要导航"><button v-for="entry in mobileNav" :key="entry.id" :class="{ active: view === entry.id }" :aria-current="view === entry.id ? 'page' : undefined" @click="navigate(entry.id)"><Icon :name="entry.icon" :size="21" /><span>{{ entry.label }}</span></button><button :class="{ active: moreNavigationActive }" aria-label="更多内容与管理" aria-haspopup="dialog" :aria-expanded="showMoreNavigation" @click="showMoreNavigation = true"><Icon name="more" :size="21" /><span>更多</span></button></nav>
  </div>
  <div v-if="toast" class="toast" role="status"><Icon name="info" :size="18" />{{ toast }}</div>
  <DialogShell v-if="showMoreNavigation && status.user" title="浏览与管理" @close="showMoreNavigation = false">
    <nav class="more-navigation" aria-label="全部内容与管理导航"><div class="more-content-types"><button v-for="entry in primaryNav" :key="entry.id" :class="{ active: view === entry.id }" :aria-current="view === entry.id ? 'page' : undefined" @click="navigate(entry.id)"><Icon :name="entry.icon" :size="24" /><span>{{ entry.label }}</span></button></div><div class="more-management"><button v-for="entry in secondaryNav" :key="entry.id" @click="navigate(entry.id)"><Icon :name="entry.icon" /><span>{{ entry.label }}</span><Icon name="chevron" :size="16" /></button><button @click="navigate('settings')"><Icon name="settings" /><span>设置</span><Icon name="chevron" :size="16" /></button></div></nav>
  </DialogShell>
  <LibraryDialog v-if="showLibraryDialog && status.user" @close="showLibraryDialog = false" @created="libraryCreated" />
  <DialogShell v-if="selected" :title="selected.title" @close="selected = null"><div class="detail-layout"><div class="detail-cover" :class="{ 'music-detail-cover': selected.type === 'music' }"><img :src="selected.coverUrl" :alt="selected.title" /></div><div class="detail-copy"><span class="eyebrow">{{ typeLabels[selected.type] }} · {{ extension(selected.relativePath) }}</span><h3>{{ selected.title }}</h3><p class="muted">{{ formatSize(selected.size) }}<span v-if="isTimeMedia(selected)"> · {{ formatDuration(selected.duration) }}</span></p><p v-if="selected.progress" class="detail-progress">上次{{ selected.type === 'music' ? '听到' : '看到' }} {{ Math.round(progressPercent(selected)) }}% · {{ formatDate(selected.progress.updatedAt) }}</p><p class="detail-path">{{ selected.relativePath }}</p><p v-if="selected.missing" class="error-box">原文件暂不可用，请检查挂载并重新扫描媒体库。</p><div class="detail-actions"><button class="primary-button" :disabled="selected.missing" @click="startReading(selected)"><Icon :name="isTimeMedia(selected) ? 'play' : selected.type === 'image' ? 'zoom' : 'novel'" />{{ isTimeMedia(selected) ? (selected.progress ? '继续播放' : '开始播放') : selected.type === 'image' ? '查看大图' : selected.progress ? '继续阅读' : '开始阅读' }}</button><button class="icon-button" :class="{ selected: selected.favorite }" :aria-label="selected.favorite ? '取消收藏' : '收藏'" :aria-pressed="selected.favorite" @click="toggleFavorite(selected)"><Icon name="heart" /></button></div><form class="tags-form" @submit.prevent="saveTags"><label>标签<input v-model="tagsText" maxlength="1000" placeholder="用逗号分隔，例如：旅行，想重看" /></label><p v-if="detailError" class="error-box" role="alert">{{ detailError }}</p><button class="secondary-button" type="submit" :disabled="tagsBusy">{{ tagsBusy ? '保存中…' : '保存标签' }}</button></form></div></div></DialogShell>
  <DialogShell v-if="deleting" title="移除媒体库" @close="deleting = null"><div class="confirm-body"><p>确定移除“{{ deleting.name }}”的索引？关联的收藏、标签和进度也会移除。</p><p class="muted">源目录和媒体原文件不会被删除。</p><div class="dialog-actions"><button class="secondary-button" @click="deleting = null">保留媒体库</button><button class="danger-button" :disabled="actionBusy" @click="deleteLibrary">{{ actionBusy ? '正在移除…' : '移除索引' }}</button></div></div></DialogShell>
  <ReaderDialog v-if="reading && status.user" :key="reading.id" :item="reading" :image-items="imageItems" @close="stopReading" @navigate="startReading" />
</template>
