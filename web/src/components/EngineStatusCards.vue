<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, errorMessage, isAbort } from '../api'
import { engineModeLabel, engineNames, engineStateLabel } from '../engines'
import type { EngineComponentId, EngineStatus } from '../types'
import Icon from './Icon.vue'

/** 卡片在管理面板打开时停止重复轮询。 */
interface Props {
  /** 是否由初始化面板接管状态读取。 */ paused: boolean
  /** 面板关闭或配置变更后重新读取的版本。 */ refreshKey: number
}
const props = defineProps<Props>()
const emit = defineEmits<{ manage: [component?: EngineComponentId] }>()
/** 卡片只需低频检测部署控制面，下载任务使用原有轮询。 */
const refreshMilliseconds: number = 15000
/** 控制面连接异常时及时释放浏览器请求，不累积无限等待。 */
const requestTimeoutMilliseconds: number = 15000
const status = ref<EngineStatus | null>(null)
const error = ref<string>('')
let controller: AbortController | null = null
let timer: ReturnType<typeof setTimeout> | undefined
let disposed: boolean = false

/** 读取无部署副作用的状态；故障保留最后快照并明确标记。 */
async function refresh(): Promise<void> {
  if (disposed || props.paused || document.hidden || controller) return
  const request: AbortController = new AbortController()
  controller = request
  const timeout: ReturnType<typeof setTimeout> = setTimeout((): void => { request.abort() }, requestTimeoutMilliseconds)
  try {
    const response: EngineStatus = await api<EngineStatus>('/engines', 'GET', undefined, request.signal)
    if (disposed || request.signal.aborted || controller !== request) return
    status.value = response
    error.value = ''
  } catch (failure: unknown) {
    if (!disposed && controller === request) error.value = isAbort(failure) ? '等待控制面超时，请稍后重试。' : errorMessage(failure)
  } finally {
    clearTimeout(timeout)
    if (controller === request) {
      controller = null
      if (!disposed && !props.paused && !document.hidden) timer = setTimeout((): void => { void refresh() }, refreshMilliseconds)
    }
  }
}

/** 取消过时请求后开始新一轮，避免关闭弹窗时旧响应覆盖新状态。 */
function restart(): void {
  clearTimeout(timer)
  controller?.abort()
  controller = null
  void refresh()
}

watch((): [boolean, number] => [props.paused, props.refreshKey], restart)
onMounted((): void => { document.addEventListener('visibilitychange', restart); void refresh() })
onBeforeUnmount((): void => { disposed = true; clearTimeout(timer); controller?.abort(); document.removeEventListener('visibilitychange', restart) })
</script>

<template>
  <section class="engine-overview" aria-label="搜索与下载引擎">
    <div class="engine-overview-heading"><div><h2>引擎状态</h2><p>搜索与下载独立运行，部署进度可随时回来查看。</p></div><button class="text-button" @click="emit('manage')">引擎管理<Icon name="chevron" :size="15" /></button></div>
    <p v-if="error" class="error-box" role="alert">引擎状态暂未刷新：{{ error }}<button class="text-button" @click="restart">重新读取</button></p>
    <div v-if="status" class="engine-status-grid" :class="{ 'engine-status-stale': error }">
      <article v-for="component in status.components" :key="component.id" class="engine-status-card">
        <span class="engine-card-icon"><Icon :name="component.id === 'jackett' ? 'search' : 'download'" :size="21" /></span>
        <div class="engine-card-copy"><h3>{{ engineNames[component.id] }}<small>{{ component.id === 'jackett' ? 'Jackett' : 'qBittorrent' }}</small></h3><p><span :class="{ 'engine-state-ready': component.readiness === 'ready' }">{{ error ? '待重新确认' : engineStateLabel(component) }}</span><span> · {{ engineModeLabel(component.mode) }}</span></p></div>
        <button class="icon-button" :aria-label="`管理${engineNames[component.id]}`" @click="emit('manage', component.id)"><Icon name="settings" :size="18" /></button>
      </article>
    </div>
    <div v-else-if="!error" class="engine-status-loading" role="status">正在读取引擎配置…</div>
  </section>
</template>
