<!-- [plus] 项目 Select 的薄包装：统一尺寸；传 groups 时按组列出选项（组标题不可选，子项缩进） -->
<template>
  <Select
    :model-value="modelValue"
    :options="opts"
    :placeholder="placeholder"
    :disabled="disabled"
    :searchable="searchable"
    :class="['psel', size === 'sm' ? 'psel-sm' : 'psel-md', block ? 'w-full' : 'inline-block align-middle', widthClass]"
    @update:model-value="(v) => emit('update:modelValue', v as any)"
    @change="(v) => emit('change', v as any)"
  >
    <template #option="{ option, selected }">
      <span v-if="option.kind === 'group'" class="psel-group">{{ option.label }}</span>
      <template v-else>
        <span class="select-option-label" :class="groups ? 'pl-2' : ''">{{ option.label }}</span>
        <Icon v-if="selected" name="check" size="sm" class="text-primary-500" :stroke-width="2" />
      </template>
    </template>
  </Select>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

type V = string | number | boolean | null

export interface PSelectGroup {
  label: string
  options: SelectOption[]
}

const props = withDefaults(
  defineProps<{
    modelValue: V | undefined
    options?: SelectOption[]
    groups?: PSelectGroup[]
    placeholder?: string
    disabled?: boolean
    size?: 'sm' | 'md'
    block?: boolean
    widthClass?: string
    searchable?: boolean | 'auto'
  }>(),
  { options: () => [], size: 'md', block: false, widthClass: 'min-w-[9rem]', searchable: 'auto' }
)
const emit = defineEmits<{
  (e: 'update:modelValue', v: V): void
  (e: 'change', v: V): void
}>()

const opts = computed<SelectOption[]>(() => {
  if (!props.groups) return props.options
  const out: SelectOption[] = [...props.options]
  props.groups.forEach((g, i) => {
    if (!g.options.length) return
    out.push({ value: `__group_${i}`, label: g.label, kind: 'group', disabled: true })
    out.push(...g.options)
  })
  return out
})
</script>

<style scoped>
.psel-sm :deep(.select-trigger) {
  @apply rounded-md px-2 py-0.5 text-xs;
}
</style>

<style>
/* 下拉层被 Teleport 到 body，只能用全局样式；用 :has 只命中本组件渲染的组标题 */
.select-dropdown-portal .select-option-group:has(.psel-group) {
  @apply cursor-default bg-transparent px-4 pb-1 pt-2.5 text-xs font-normal normal-case tracking-normal text-gray-400 hover:bg-transparent dark:bg-transparent dark:text-dark-400;
}
.select-dropdown-portal .select-option-group:has(.psel-group):not(:first-child) {
  @apply mt-1 border-t border-gray-100 dark:border-dark-700;
}
</style>
