<!-- [plus] 下拉选项编辑：回车添加，点 × 删除 -->
<template>
  <div>
    <div
      class="input flex min-h-[42px] flex-wrap items-center gap-1.5 !py-1.5"
      :class="error ? 'input-error ring-2 ring-red-500/20' : ''"
      data-testid="options-input"
      @click="inputRef?.focus()"
    >
      <span v-for="(o, i) in list" :key="o" class="badge badge-gray gap-1">
        {{ o }}
        <button type="button" class="text-gray-400 hover:text-red-500" :aria-label="`删除 ${o}`" @click.stop="remove(i)">×</button>
      </span>
      <input
        ref="inputRef"
        v-model="text"
        :placeholder="list.length ? '' : '输入选项后按回车'"
        class="min-w-[120px] flex-1 border-0 bg-transparent p-0 text-sm focus:outline-none focus:ring-0 dark:text-white"
        @input="dup = false"
        @keydown.enter.prevent="add"
        @keydown.backspace="!text && list.length && remove(list.length - 1)"
        @blur="add"
      />
    </div>
    <p v-if="dup" class="input-error-text mt-1.5">已存在</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

const props = defineProps<{ modelValue?: string[]; error?: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string[]): void }>()

const list = computed(() => props.modelValue ?? [])
const text = ref('')
const dup = ref(false)
const inputRef = ref<HTMLInputElement>()

function add() {
  // 支持一次粘贴多个，用逗号或顿号分隔
  const parts = text.value.split(/[,，、]/).map((x) => x.trim()).filter(Boolean)
  if (!parts.length) {
    text.value = ''
    return
  }
  const next = [...list.value]
  for (const p of parts) if (!next.includes(p)) next.push(p)
  // 全部重复时保留输入并提示，不悄悄清空
  if (next.length === list.value.length) {
    dup.value = true
    return
  }
  text.value = ''
  emit('update:modelValue', next)
}

function remove(i: number) {
  dup.value = false
  emit('update:modelValue', list.value.filter((_, j) => j !== i))
}
</script>
