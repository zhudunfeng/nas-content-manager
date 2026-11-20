<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { api, createRequestId, errorMessage, isAbort } from '../api'
import { formatSize } from '../format'
import type { CollectionCandidate, CollectionTask, ContentType, Library } from '../types'
import CollectionTargetFields from './CollectionTargetFields.vue'
import DialogShell from './DialogShell.vue'

/** 候选人工获取所需的安全快照。 */
interface Props {
  /** 当前候选及乐观版本。 */ candidate: CollectionCandidate
  /** 可用媒体库。 */ libraries: Library[]
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; created: [task: CollectionTask] }>()
const contentType = ref<ContentType>('video')
const createLibrary = ref<boolean>(true)
const libraryId = ref<number>(0)
const requestId = ref<string>(createRequestId())
const submitted = ref<boolean>(false)
const saving = ref<boolean>(false)
const error = ref<string>('')
const controller: AbortController = new AbortController()
watch([contentType, createLibrary, libraryId], (): void => { if (!submitted.value) requestId.value = createRequestId() })

/** 原样重试同一获取意图，不以网络失败推断没有创建任务。 */
async function acquire(): Promise<void> {
  if (saving.value || (!createLibrary.value && !libraryId.value)) return
  saving.value = true
  submitted.value = true
  error.value = ''
  try {
    const result = await api<{ task: CollectionTask }>(`/collection/candidates/${encodeURIComponent(props.candidate.id)}/acquire`, 'POST', { requestId: requestId.value, revision: props.candidate.revision, contentType: contentType.value, createLibrary: createLibrary.value, libraryId: createLibrary.value ? 0 : libraryId.value }, controller.signal)
    if (controller.signal.aborted) return
    console.info('候选资源获取已受理')
    emit('created', result.task)
  } catch (failure: unknown) { if (!isAbort(failure)) error.value = errorMessage(failure) }
  finally { saving.value = false }
}
onBeforeUnmount((): void => { controller.abort() })
</script>

<template>
  <DialogShell title="确认获取候选资源" @close="emit('close')">
    <form class="collection-form" @submit.prevent="acquire">
      <div class="collection-callout"><strong>{{ candidate.title }}</strong><p class="field-hint">{{ formatSize(candidate.sizeBytes) }} · {{ candidate.reason || '请确认资源和入库目标。' }}</p></div>
      <CollectionTargetFields v-model:content-type="contentType" v-model:create-library="createLibrary" v-model:library-id="libraryId" :libraries="libraries" :disabled="saving || submitted" />
      <p class="section-note">BT 获取复用下载中心。元数据就绪后，需要确认文件选集；传输完成后还会独立校验、发布和扫描。</p>
      <p v-if="error" class="error-box" role="alert">{{ error }}<br />重试会复用本次请求和候选版本；可关闭后刷新查看任务是否已受理。</p>
      <div class="dialog-actions"><button class="secondary-button" type="button" @click="emit('close')">关闭</button><button class="primary-button" type="submit" :disabled="saving || (!createLibrary && !libraryId)">{{ saving ? '正在提交…' : submitted ? '重试原请求' : '确认获取' }}</button></div>
    </form>
  </DialogShell>
</template>
