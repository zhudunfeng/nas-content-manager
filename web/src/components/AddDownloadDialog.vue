<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { api, apiMultipart, createRequestId, errorMessage, isAbort } from '../api'
import { downloadBytes } from '../downloads'
import { typeLabels } from '../format'
import type { ContentType, Download, TorrentSearchResult } from '../types'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

/** 搜索选择只携带安全展示信息与服务端短期结果编号。 */
interface Props {
  /** 来自搜索弹窗的选中资源，省略时保留原磁力与上传入口。 */ searchResult?: TorrentSearchResult | null
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; created: [download: Download] }>()
/** 种子本体上限为八 MiB，与后端有界上传契约一致。 */
const maxTorrentBytes: number = 8 * 1024 * 1024
const source = ref<'magnet' | 'torrent'>('magnet')
const magnet = ref<string>('')
const torrent = ref<File | null>(null)
const contentType = ref<ContentType>('video')
const createLibrary = ref<boolean>(true)
const saving = ref<boolean>(false)
const error = ref<string>('')
let requestId: string = ''
let controller: AbortController | null = null

/** 切换输入内容时为下一次独立提交分配幂等键，不重用旧请求。 */
function resetRequest(): void {
  requestId = ''
  error.value = ''
}

/** 仅读取种子文件引用，浏览器不读取或上传媒体原文件。 */
function chooseTorrent(event: Event): void {
  const input: HTMLInputElement | null = event.target instanceof HTMLInputElement ? event.target : null
  const file: File | undefined = input?.files?.[0]
  torrent.value = null
  resetRequest()
  if (!file) return
  if (!file.name.toLowerCase().endsWith('.torrent')) { error.value = '请选择一个 .torrent 种子文件'; return }
  if (file.size === 0 || file.size > maxTorrentBytes) { error.value = '种子文件不能为空，且不能超过 8 MiB'; return }
  torrent.value = file
}

/** 等待添加结果时保持表单，避免未知结果下重新创建请求。 */
function requestClose(): void {
  if (!saving.value) emit('close')
}

/** 以稳定请求键添加下载，重试不会重复提交另一项新任务。 */
async function submit(): Promise<void> {
  if (saving.value) return
  error.value = ''
  if (!props.searchResult && source.value === 'magnet' && !/^magnet:\?/i.test(magnet.value.trim())) { error.value = '请输入以 magnet:? 开头的有效磁力链接'; return }
  if (!props.searchResult && source.value === 'magnet' && magnet.value.trim().length > 16000) { error.value = '磁力链接过长，请改用本地种子文件'; return }
  if (!props.searchResult && source.value === 'torrent' && !torrent.value) { error.value = '请先选择一个 .torrent 种子文件'; return }
  requestId ||= createRequestId()
  saving.value = true
  const request: AbortController = new AbortController()
  controller = request
  try {
    let result: { download: Download }
    if (props.searchResult) {
      // 浏览器只提交服务端结果编号，私密下载地址与索引密钥均不经过前端。
      result = await api<{ download: Download }>('/downloads', 'POST', { requestId, searchResultId: props.searchResult.id, contentType: contentType.value, createLibrary: createLibrary.value }, request.signal)
    } else if (source.value === 'torrent' && torrent.value) {
      const body: FormData = new FormData()
      body.append('requestId', requestId)
      body.append('contentType', contentType.value)
      body.append('createLibrary', String(createLibrary.value))
      body.append('torrent', torrent.value)
      // 不手工填写 Content-Type，multipart boundary 必须与浏览器生成的请求体一致。
      result = await apiMultipart<{ download: Download }>('/downloads', body, request.signal)
    } else {
      result = await api<{ download: Download }>('/downloads', 'POST', { requestId, magnet: magnet.value.trim(), contentType: contentType.value, createLibrary: createLibrary.value }, request.signal)
    }
    if (request.signal.aborted) return
    console.info('下载添加请求已确认，等待文件清单')
    emit('created', result.download)
  } catch (failure: unknown) {
    // 保留本次幂等键；超时可能已被服务端接受，用户重试必须查询同一请求。
    if (!isAbort(failure)) error.value = `${errorMessage(failure)}。可保留当前表单重试；相同请求不会重复添加。`
  } finally { if (controller === request) saving.value = false }
}

watch([source, magnet, contentType, createLibrary], resetRequest)
watch((): string | undefined => props.searchResult?.id, resetRequest)
onBeforeUnmount((): void => { controller?.abort() })
</script>

<template>
  <DialogShell title="添加 BT 下载" @close="requestClose">
    <form class="library-form download-add-form" :aria-busy="saving" @submit.prevent="submit">
      <p class="muted">添加后先获取文件清单，由你确认所需文件，再开始正式下载。</p>
      <div v-if="searchResult" class="download-search-selection"><span class="status-pill">来自搜种 · {{ searchResult.sourceName }}</span><h3>{{ searchResult.title }}</h3><p class="field-hint">{{ searchResult.sizeBytes === null ? '体积未知' : downloadBytes(searchResult.sizeBytes) }} · 来源由服务端确认。请核对下面的内容类型，添加后仍需选择文件。</p><p class="field-hint">若结果已过期，请关闭此表单并重新搜种。</p></div>
      <fieldset v-else :disabled="saving">
        <legend class="sr-only">下载来源</legend>
        <div class="download-source-switch"><button type="button" :class="{ active: source === 'magnet' }" :aria-pressed="source === 'magnet'" @click="source = 'magnet'"><Icon name="link" />磁力链接</button><button type="button" :class="{ active: source === 'torrent' }" :aria-pressed="source === 'torrent'" @click="source = 'torrent'"><Icon name="upload" />种子文件</button></div>
      </fieldset>
      <label v-if="!searchResult && source === 'magnet'">磁力链接<textarea v-model="magnet" :disabled="saving" rows="4" maxlength="16000" required autofocus spellcheck="false" autocomplete="off" placeholder="magnet:?xt=urn:btih:…" /><span class="field-hint">链接仅发送给栖库后端，不在界面或日志中展示私有 Tracker 参数。</span></label>
      <label v-else-if="!searchResult" class="download-upload">本地种子文件<input :disabled="saving" type="file" accept=".torrent,application/x-bittorrent" required @change="chooseTorrent" /><span class="field-hint">{{ torrent ? `${torrent.name} · ${downloadBytes(torrent.size)}` : '单个 .torrent 文件，最大 8 MiB。这里只上传种子，不上传媒体。' }}</span></label>
      <fieldset :disabled="saving"><legend>完成后的内容类型</legend><div class="type-picker"><button v-for="(label, key) in typeLabels" :key="key" type="button" :class="{ active: contentType === key }" :aria-pressed="contentType === key" @click="contentType = key"><Icon :name="key" />{{ label }}</button></div><p class="field-hint">混合种子按一种类型入库；不匹配的格式不会自动转换。</p></fieldset>
      <p class="field-hint">完成后使用“{{ typeLabels[contentType] }}”下载媒体库。{{ createLibrary ? '提交即允许首次创建该受管媒体库，可在下方选项中取消。' : '当前不允许创建新库，需要已有对应类型的受管下载库。' }}</p>
      <details class="download-options"><summary>媒体库选项</summary><label class="download-checkbox"><input v-model="createLibrary" :disabled="saving" type="checkbox" /><span>允许首次创建“{{ typeLabels[contentType] }}”下载媒体库</span></label><p class="field-hint">默认使用同类型受管下载库；勾选表示在首次发布时授权创建。取消勾选需要已有对应下载库。下载仅写入部署时配置的专用下载目录。</p></details>
      <p v-if="error" class="error-box" role="alert">{{ error }}</p>
      <p v-if="saving" class="field-hint" role="status">正在确认添加结果，请稍候。请勿重复打开表单提交。</p>
      <div class="dialog-actions"><button type="button" class="secondary-button" :disabled="saving" @click="requestClose">取消</button><button type="submit" class="primary-button" :disabled="saving || (!searchResult && source === 'torrent' && !torrent)"><span v-if="saving" class="loader" /><Icon v-else name="plus" />{{ saving ? '正在添加…' : '添加并选择文件' }}</button></div>
    </form>
  </DialogShell>
</template>
