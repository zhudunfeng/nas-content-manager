<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errorMessage, isAbort } from '../api'
import { typeLabels } from '../format'
import type { BrowseResult, ContentType, Library } from '../types'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'

const emit = defineEmits<{ close: []; created: [library: Library] }>()
const name = ref<string>('')
const type = ref<ContentType>('video')
const path = ref<string>('')
const browse = ref<BrowseResult>({ path: '', parent: '', roots: [], entries: [] })
const loading = ref<boolean>(false)
const saving = ref<boolean>(false)
const error = ref<string>('')
let controller: AbortController | null = null

/** 浏览服务端已经挂载并授权的媒体目录。 */
async function openPath(target: string): Promise<void> {
  controller?.abort()
  const request: AbortController = new AbortController()
  controller = request
  loading.value = true
  error.value = ''
  try {
    const result: BrowseResult = await api<BrowseResult>(`/browse?path=${encodeURIComponent(target)}`, 'GET', undefined, request.signal)
    // 快速切换目录时只接受最后一次请求，避免旧结果覆盖当前选择。
    if (request.signal.aborted || controller !== request) return
    browse.value = result
    path.value = result.path
  } catch (failure: unknown) {
    if (!isAbort(failure) && controller === request) error.value = errorMessage(failure)
  } finally { if (controller === request) loading.value = false }
}

/** 保存媒体库并让持久任务在后台扫描，不阻塞内容浏览。 */
async function save(): Promise<void> {
  if (saving.value || loading.value || !path.value || !name.value.trim()) return
  saving.value = true
  error.value = ''
  try {
    const result = await api<{ library: Library }>('/libraries', 'POST', { name: name.value.trim(), type: type.value, path: path.value })
    console.info('媒体库已添加，后台扫描已提交')
    emit('created', result.library)
  } catch (failure: unknown) { error.value = errorMessage(failure) }
  finally { saving.value = false }
}

onMounted((): void => { void openPath('') })
onBeforeUnmount((): void => { controller?.abort() })
</script>

<template>
  <DialogShell title="添加媒体库" @close="emit('close')">
    <form class="library-form" @submit.prevent="save">
      <p class="muted">选择 NAS 已挂载的目录。栖库只建立索引，不搬移或修改原文件。</p>
      <label>媒体库名称<input v-model="name" autofocus required maxlength="80" placeholder="例如：我的电影收藏" /></label>
      <fieldset><legend>内容类型</legend><div class="type-picker"><button v-for="(label, key) in typeLabels" :key="key" type="button" :class="{ active: type === key }" :aria-pressed="type === key" @click="type = key"><Icon :name="key" />{{ label }}</button></div></fieldset>
      <div class="field-heading"><span>媒体目录</span><button class="text-button" type="button" @click="openPath('')">返回挂载根目录</button></div>
      <div class="folder-browser" :aria-busy="loading">
        <div class="folder-location"><Icon name="folder" /><span>{{ browse.path || '选择挂载目录' }}</span><button v-if="browse.path" class="icon-button" type="button" aria-label="返回上级目录" @click="openPath(browse.parent)"><Icon name="back" /></button></div>
        <div v-if="loading" class="folder-empty">正在读取目录…</div>
        <template v-else>
          <button v-for="root in browse.path ? [] : browse.roots" :key="root" class="folder-row" type="button" @click="openPath(root)"><Icon name="folder" /><span>{{ root }}</span><Icon name="chevron" /></button>
          <button v-for="entry in browse.entries.filter((entry): boolean => entry.isDir)" :key="entry.path" class="folder-row" type="button" @click="openPath(entry.path)"><Icon name="folder" /><span>{{ entry.name }}</span><Icon name="chevron" /></button>
          <p v-if="browse.path && !browse.entries.some((entry): boolean => entry.isDir)" class="folder-empty">此目录没有子文件夹，可以直接添加当前目录。</p>
          <p v-if="!browse.path && !browse.roots.length" class="folder-empty">没有可访问的挂载。请检查 NAS_MEDIA_ROOTS 和容器目录映射。</p>
        </template>
      </div>
      <p class="field-hint">当前选择：{{ path || '尚未选择目录' }}。网页只能访问部署时授权的挂载。</p>
      <p v-if="type === 'comic'" class="field-hint">支持 CBZ 和图片文件夹；每个包含图片的目录会作为一册漫画，页码按文件名自然排序。</p>
      <p v-if="error" class="error-box" role="alert">{{ error }}</p>
      <div class="dialog-actions"><button class="secondary-button" type="button" @click="emit('close')">取消</button><button class="primary-button" :disabled="saving || loading || !path || !name.trim()" type="submit"><Icon name="plus" />{{ saving ? '正在添加…' : '添加并扫描' }}</button></div>
    </form>
  </DialogShell>
</template>
