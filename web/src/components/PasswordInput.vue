<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Icon from './Icon.vue'

// 原生校验、自动填充、标签关联和事件统一落到实际输入框。
defineOptions({ inheritAttrs: false })

/** 掩码输入框的显示控制参数；其他原生属性与事件转发至输入框。 */
interface Props {
  /** 输入框及显示按钮是否禁用。 */ disabled?: boolean
  /** 显示按钮提示中使用的字段名称。 */ secretLabel?: string
}
const props = withDefaults(defineProps<Props>(), {
  // 默认允许编辑，禁用状态由所属表单控制。
  disabled: false,
  // 密码为通用默认名称，确认密码和外部凭据可分别指定。
  secretLabel: '密码',
})
// 输入值必须由所属表单提供，组件只持有临时的显示状态。
const model = defineModel<string>({ required: true })
const visible = ref<boolean>(false)
const toggleLabel = computed<string>((): string => `${visible.value ? '隐藏' : '显示'}${props.secretLabel}`)

/** 仅切换当前字段的显示方式，不改变内容或触发表单提交。 */
function toggleVisibility(): void {
  if (!props.disabled) visible.value = !visible.value
}

// 清空输入或父级重置凭据时同步恢复掩码，避免下次填写沿用明文状态。
watch(model, (value: string): void => {
  if (value === '') visible.value = false
}, { flush: 'sync' })
</script>

<template>
  <span class="password-input">
    <input v-model="model" v-bind="$attrs" :type="visible ? 'text' : 'password'" :disabled="disabled" />
    <button class="icon-button password-toggle" type="button" :disabled="disabled" :aria-label="toggleLabel" :title="toggleLabel" :aria-pressed="visible" @click="toggleVisibility"><Icon :name="visible ? 'eye-off' : 'eye'" /></button>
  </span>
</template>

<style scoped>
.password-input { display: block; position: relative; width: 100%; min-width: 0; }
.password-input > input { display: block; width: 100%; padding-inline-end: 56px; }
.password-toggle { position: absolute; inset-inline-end: 2px; top: 50%; transform: translateY(-50%); width: 44px; height: 44px; padding: 10px; border-radius: 6px; }
</style>
