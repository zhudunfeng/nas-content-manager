<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from './Icon.vue'

/** 原生对话框的可访问标题与尺寸。 */
interface Props {
  /** 无障碍与可视标题。 */ title: string
  /** 是否使用全屏阅读器尺寸。 */ fullscreen?: boolean
  /** 是否隐藏默认标题栏。 */ hideHeader?: boolean
}
defineProps<Props>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
let previouslyFocused: HTMLElement | null = null
let oldOverflow: string = ''

/** 点击原生对话框遮罩时关闭，内部交互不受影响。 */
function onBackdrop(event: MouseEvent): void {
  if (event.target === dialog.value) emit('close')
}

onMounted((): void => {
  previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null
  oldOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
  dialog.value?.showModal()
})
onBeforeUnmount((): void => {
  dialog.value?.close()
  document.body.style.overflow = oldOverflow
  previouslyFocused?.focus()
})
</script>

<template>
  <dialog ref="dialog" class="dialog" :class="{ 'dialog-full': fullscreen }" :aria-label="title" @cancel.prevent="emit('close')" @click="onBackdrop">
    <div class="dialog-inner">
      <header v-if="!hideHeader" class="dialog-heading"><h2>{{ title }}</h2><button class="icon-button" aria-label="关闭对话框" @click="emit('close')"><Icon name="close" /></button></header>
      <slot />
    </div>
  </dialog>
</template>
