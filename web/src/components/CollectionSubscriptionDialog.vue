<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, errorMessage, isAbort } from '../api'
import type { CollectionSource, CollectionSubscription, CollectionSubscriptionInput, ContentType, Library } from '../types'
import CollectionTargetFields from './CollectionTargetFields.vue'
import DialogShell from './DialogShell.vue'

/** 订阅编辑使用当前来源与媒体库快照。 */
interface Props {
  /** 编辑对象，空值表示新建。 */ subscription?: CollectionSubscription | null
  /** 可选择的 RSS 来源。 */ sources: CollectionSource[]
  /** 当前媒体库。 */ libraries: Library[]
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; saved: [subscription: CollectionSubscription] }>()
const sourceId = ref<string>(props.subscription?.sourceId || '')
const name = ref<string>(props.subscription?.name || '')
const enabled = ref<boolean>(props.subscription?.enabled ?? false)
const contentType = ref<ContentType>(props.subscription?.contentType || 'video')
const createLibrary = ref<boolean>(props.subscription?.createLibrary ?? true)
const libraryId = ref<number>(props.subscription?.libraryId || 0)
const mode = ref<'review' | 'automatic'>(props.subscription?.mode || 'review')
const include = ref<string>(props.subscription?.rule.include || '')
const exclude = ref<string>(props.subscription?.rule.exclude || '')
const quality = ref<string>(props.subscription?.rule.quality || '')
const minMiB = ref<number>((props.subscription?.rule.minBytes || 0) / 1048576)
const maxMiB = ref<number>((props.subscription?.rule.maxBytes || 0) / 1048576)
const episodePattern = ref<string>(props.subscription?.rule.episodePattern || '')
const selectionPattern = ref<string>(props.subscription?.selectionPattern || '')
const autoConsent = ref<boolean>(false)
const saving = ref<boolean>(false)
const error = ref<string>('')
const controller: AbortController = new AbortController()
const rssSources = computed<CollectionSource[]>((): CollectionSource[] => props.sources.filter((source: CollectionSource): boolean => source.kind === 'rss'))
watch(mode, (): void => { autoConsent.value = false })

/** 保存明确的订阅规则，自动执行必须包含确定选文件策略。 */
async function save(): Promise<void> {
  if (saving.value) return
  error.value = ''
  if (!sourceId.value || (!createLibrary.value && !libraryId.value)) { error.value = '请选择来源和完整的入库目标。'; return }
  if (maxMiB.value > 0 && maxMiB.value < minMiB.value) { error.value = '最大大小不能小于最小大小。'; return }
  if (mode.value === 'automatic' && (!autoConsent.value || !include.value.trim() || !episodePattern.value.trim() || !selectionPattern.value.trim())) { error.value = '自动模式需要明确关键词、季集表达式、文件选择表达式及本次授权。'; return }
  const input: CollectionSubscriptionInput = { sourceId: sourceId.value, name: name.value.trim(), enabled: enabled.value, contentType: contentType.value, mode: mode.value, rule: { include: include.value.trim(), exclude: exclude.value.trim(), quality: quality.value.trim(), minBytes: Math.round(minMiB.value * 1048576), maxBytes: Math.round(maxMiB.value * 1048576), episodePattern: episodePattern.value.trim() }, createLibrary: createLibrary.value, libraryId: createLibrary.value ? 0 : libraryId.value, selectionPattern: selectionPattern.value.trim() }
  if (props.subscription) input.revision = props.subscription.revision
  saving.value = true
  try {
    const result = await api<{ subscription: CollectionSubscription }>(`/collection/subscriptions${props.subscription ? `/${encodeURIComponent(props.subscription.id)}` : ''}`, props.subscription ? 'PATCH' : 'POST', input, controller.signal)
    if (controller.signal.aborted) return
    console.info('资源订阅规则已保存')
    emit('saved', result.subscription)
  } catch (failure: unknown) { if (!isAbort(failure)) error.value = errorMessage(failure) }
  finally { saving.value = false }
}
onBeforeUnmount((): void => { controller.abort() })
</script>

<template>
  <DialogShell :title="subscription ? '编辑订阅规则' : '添加订阅规则'" @close="emit('close')">
    <form class="collection-form" @submit.prevent="save">
      <p class="muted">默认产生待确认候选。建议先保存停用的规则，试运行查看命中后再启用。</p>
      <div class="collection-field-grid">
        <label>规则名称<input v-model="name" autofocus required maxlength="80" :disabled="saving" placeholder="例如：每周纪录片" /><span class="field-hint">用于区分订阅和查看运行结果。</span></label>
        <label>RSS 来源<select v-model="sourceId" required :disabled="saving"><option value="" disabled>选择已配置来源</option><option v-for="source in rssSources" :key="source.id" :value="source.id">{{ source.name }}{{ source.enabled ? '' : '（已停用）' }}</option></select><span class="field-hint">来源需启用后才能运行；首次基线不自动回填。</span></label>
      </div>
      <p v-if="!rssSources.length" class="section-note">请先到“来源”添加 RSS 地址，再建立订阅。</p>
      <div class="collection-field-grid">
        <label>包含关键词<input v-model="include" maxlength="200" :disabled="saving" placeholder="例如：纪录片,2026" /><span class="field-hint">英文逗号分隔，全部关键词均须命中；自动模式必填。</span></label>
        <label>排除关键词<input v-model="exclude" maxlength="200" :disabled="saving" placeholder="例如：预告,花絮" /><span class="field-hint">英文逗号分隔，任一命中即排除；留空表示不额外排除。</span></label>
        <label>质量条件<input v-model="quality" maxlength="100" :disabled="saving" placeholder="例如：1080p" /><span class="field-hint">标题中的质量关键词，留空表示不限。</span></label>
        <label>明确集数表达式<input v-model="episodePattern" maxlength="256" :required="mode === 'automatic'" :disabled="saving" placeholder="例如：S(?P&lt;season&gt;[0-9]+)E(?P&lt;episode&gt;[0-9]+)" /><span class="field-hint">使用服务端正则表达式，包含 season 和 episode 命名捕获组；例如 S(?P&lt;season&gt;[0-9]+)E(?P&lt;episode&gt;[0-9]+)。自动模式必填，绝对集数和无法确定的条目会等待确认。</span></label>
        <label>最小大小（MiB）<input v-model.number="minMiB" type="number" min="0" step="0.1" required :disabled="saving" /><span class="field-hint">0 表示不设下限；1 MiB 为 1,048,576 字节。</span></label>
        <label>最大大小（MiB）<input v-model.number="maxMiB" type="number" min="0" step="0.1" required :disabled="saving" /><span class="field-hint">0 表示不设上限，任务仍受服务端总配额限制。</span></label>
      </div>
      <CollectionTargetFields v-model:content-type="contentType" v-model:create-library="createLibrary" v-model:library-id="libraryId" :libraries="libraries" :disabled="saving" />
      <label>执行方式<select v-model="mode" :disabled="saving"><option value="review">先由我确认候选（默认）</option><option value="automatic">明确授权自动获取</option></select><span class="field-hint">自动规则仅处理确定且无歧义的命中，其他条目仍待人工确认。</span></label>
      <template v-if="mode === 'automatic'">
        <label>文件选择表达式<input v-model="selectionPattern" required maxlength="256" :disabled="saving" placeholder="例如：(?i)episode-01[.]mkv$" /><span class="field-hint">服务端正则表达式，必须唯一匹配允许的媒体文件；多个匹配或集数不明会等待确认。</span></label>
        <label class="collection-check collection-consent"><input v-model="autoConsent" type="checkbox" :disabled="saving" /><span>我授权此规则按上述条件自动获取和入库<span class="field-hint">此项不默认勾选；修改并保存自动规则时需要重新确认。</span></span></label>
      </template>
      <label class="collection-check"><input v-model="enabled" type="checkbox" :disabled="saving" /><span>启用定时订阅<span class="field-hint">停用后保留规则、候选和已有任务。</span></span></label>
      <p v-if="error" class="error-box" role="alert">{{ error }}</p>
      <div class="dialog-actions"><button class="secondary-button" type="button" @click="emit('close')">取消</button><button class="primary-button" type="submit" :disabled="saving || !name.trim() || !sourceId || (mode === 'automatic' && !autoConsent)">{{ saving ? '保存中…' : '保存规则' }}</button></div>
    </form>
  </DialogShell>
</template>
