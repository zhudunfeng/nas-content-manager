<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Item } from '../types'
import { extension, formatDuration, formatSize, progressPercent, typeLabels } from '../format'
import Icon from './Icon.vue'

/** 媒体卡片的数据与显示形式。 */
interface Props { /** 内容记录。 */ item: Item; /** 是否横向展示续看卡片。 */ wide?: boolean }
const props = defineProps<Props>()
const emit = defineEmits<{ open: [item: Item]; favorite: [item: Item] }>()
const failed = ref<boolean>(false)
const percent = computed<number>((): number => progressPercent(props.item))
const subtitle = computed<string>((): string => props.item.type === 'video' ? formatDuration(props.item.duration) : `${extension(props.item.relativePath)} · ${formatSize(props.item.size)}`)
</script>

<template>
  <article class="media-card" :class="[{ wide }, `media-${item.type}`]">
    <button class="cover-button" :aria-label="`打开${typeLabels[item.type]}：${item.title}`" @click="emit('open', item)">
      <img v-if="!failed" :src="item.coverUrl || `/api/items/${item.id}/cover`" alt="" loading="lazy" decoding="async" @error="failed = true" />
      <div v-else class="cover-fallback"><Icon :name="item.type" :size="38" /><span>{{ item.title }}</span></div>
      <span class="media-kind">{{ typeLabels[item.type] }}</span>
      <span v-if="item.missing" class="media-offline">源文件暂不可用</span>
      <span class="cover-action"><Icon :name="item.type === 'video' ? 'play' : item.type === 'image' ? 'zoom' : 'novel'" :size="22" /></span>
      <span v-if="item.progress" class="card-progress" :style="{ '--progress': `${percent}%` }"><span /></span>
    </button>
    <div class="card-bottom"><div class="card-copy"><button class="card-title" :title="item.title" @click="emit('open', item)">{{ item.title }}</button><p>{{ item.progress ? `已${item.type === 'video' ? '观看' : '阅读'} ${Math.round(percent)}% · ` : '' }}{{ subtitle }}</p></div><button class="icon-button favorite-button" :class="{ selected: item.favorite }" :aria-label="`${item.favorite ? '取消收藏' : '收藏'}：${item.title}`" :aria-pressed="item.favorite" @click="emit('favorite', item)"><Icon name="heart" :size="18" /></button></div>
  </article>
</template>
