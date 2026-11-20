<script setup lang="ts">
import { computed } from 'vue'
import { typeLabels } from '../format'
import type { ContentType, Library } from '../types'

/** 所有收集入口共用的媒体目标字段。 */
interface Props {
  /** 目标分类。 */ contentType: ContentType
  /** 是否创建独立媒体库。 */ createLibrary: boolean
  /** 现有目标库编号。 */ libraryId: number
  /** 当前可选择的媒体库。 */ libraries: Library[]
  /** 提交中锁定目标，避免改变幂等意图。 */ disabled?: boolean
  /** 网页输出固定对应视频或音乐分类。 */ fixedType?: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{ 'update:contentType': [value: ContentType]; 'update:createLibrary': [value: boolean]; 'update:libraryId': [value: number] }>()
const matchingLibraries = computed<Library[]>((): Library[] => props.libraries.filter((library: Library): boolean => library.type === props.contentType))

/** 切换分类时重置不兼容的现有库。 */
function changeType(event: Event): void {
  const value: string = (event.target as HTMLSelectElement).value
  if (!(value in typeLabels)) return
  emit('update:contentType', value as ContentType)
  emit('update:libraryId', 0)
}
/** 更新目标库编号，保持请求体是数值。 */
function changeLibrary(event: Event): void { emit('update:libraryId', Number((event.target as HTMLSelectElement).value)) }
/** 显式切换创建目标库的选择。 */
function changeCreate(event: Event): void {
  const checked: boolean = (event.target as HTMLInputElement).checked
  emit('update:createLibrary', checked)
  // 新建授权不能携带先前隐藏的现有库编号，避免服务端继续绑定旧目标。
  if (checked) emit('update:libraryId', 0)
}
</script>

<template>
  <fieldset class="collection-target" :disabled="disabled">
    <legend>入库目标</legend>
    <div class="collection-field-grid">
      <label>内容类型<select :value="contentType" :disabled="fixedType" @change="changeType"><option v-for="(label, key) in typeLabels" :key="key" :value="key">{{ label }}</option></select><span class="field-hint">{{ fixedType ? '网页输出已对应到视频或音乐；可在输出类型中更改。' : '决定媒体的索引和播放方式。' }}</span></label>
      <label v-if="!createLibrary">已有媒体库<select :value="libraryId" required @change="changeLibrary"><option :value="0" disabled>请选择同类媒体库</option><option v-for="library in matchingLibraries" :key="library.id" :value="library.id">{{ library.name }}</option></select><span class="field-hint">只显示当前内容类型的媒体库。</span></label>
    </div>
    <label class="collection-check"><input type="checkbox" :checked="createLibrary" @change="changeCreate" /><span>创建独立媒体库<span class="field-hint">由服务端按收集目录创建并扫描；已有库路径需符合当前导入方式。</span></span></label>
  </fieldset>
</template>
