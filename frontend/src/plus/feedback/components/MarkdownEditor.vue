<!-- [plus] Markdown 编辑器：常用格式工具栏 + 编辑 / 预览切换；宽屏时左右并排实时预览 -->
<template>
  <div class="overflow-hidden rounded-xl border border-gray-200 bg-white focus-within:border-primary-500 focus-within:ring-2 focus-within:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-800">
    <div class="flex items-center gap-0.5 border-b border-gray-100 px-2 py-1.5 dark:border-dark-700">
      <button
        v-for="a in actions"
        :key="a.key"
        type="button"
        class="flex h-8 min-w-[32px] items-center justify-center rounded-lg px-1.5 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 disabled:opacity-30 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white"
        :class="a.cls"
        :title="a.title"
        :aria-label="a.title"
        :disabled="tab === 'preview'"
        @click="a.run"
      >
        {{ a.text }}
      </button>
      <span class="flex-1" />
      <div class="flex rounded-lg bg-gray-100 p-0.5 text-xs dark:bg-dark-700 lg:hidden">
        <button
          v-for="t in tabs"
          :key="t.value"
          type="button"
          class="rounded-md px-2.5 py-1 transition-colors"
          :class="tab === t.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white' : 'text-gray-500 dark:text-dark-300'"
          @click="tab = t.value"
        >
          {{ t.label }}
        </button>
      </div>
    </div>
    <div class="grid lg:grid-cols-2 lg:divide-x lg:divide-gray-100 dark:lg:divide-dark-700">
      <textarea
        v-show="tab === 'edit' || wide"
        ref="area"
        :value="modelValue"
        :rows="rows"
        :maxlength="maxlength"
        :placeholder="placeholder"
        class="block w-full resize-y border-0 bg-transparent px-4 py-3 font-mono text-sm leading-relaxed text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-0 dark:text-white"
        @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
        @keydown.meta.b.prevent="wrap('**')"
        @keydown.ctrl.b.prevent="wrap('**')"
      />
      <div v-show="tab === 'preview' || wide" class="min-h-[120px] px-4 py-3" data-testid="md-preview">
        <MarkdownView v-if="modelValue.trim()" :source="modelValue" />
        <p v-else class="text-sm text-gray-400">预览</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import MarkdownView from './MarkdownView.vue'

withDefaults(defineProps<{ modelValue: string; rows?: number; maxlength?: number; placeholder?: string }>(), {
  rows: 8,
  maxlength: 4000,
  placeholder: '支持 Markdown'
})
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const area = ref<HTMLTextAreaElement>()
const tab = ref<'edit' | 'preview'>('edit')
const tabs = [
  { value: 'edit' as const, label: '编辑' },
  { value: 'preview' as const, label: '预览' }
]

// lg 以上并排显示，不需要切换
const wide = ref(false)
const mq = typeof window !== 'undefined' ? window.matchMedia('(min-width: 1024px)') : null
const syncWide = () => (wide.value = !!mq?.matches)
onMounted(() => {
  syncWide()
  mq?.addEventListener('change', syncWide)
})
onBeforeUnmount(() => mq?.removeEventListener('change', syncWide))

/** 用 before/after 包住选中的文字；没选中时插入占位文字并选中它 */
function wrap(before: string, after = before, sample = '文字') {
  const el = area.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e, value } = el
  const picked = value.slice(s, e) || sample
  const next = value.slice(0, s) + before + picked + after + value.slice(e)
  emit('update:modelValue', next)
  void nextTick(() => {
    el.focus()
    el.setSelectionRange(s + before.length, s + before.length + picked.length)
  })
}

/** 给选中的每一行加前缀（标题、列表、引用） */
function prefixLines(prefix: (i: number) => string) {
  const el = area.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e, value } = el
  const lineStart = value.lastIndexOf('\n', s - 1) + 1
  const block = value.slice(lineStart, e)
  const replaced = block
    .split('\n')
    .map((l, i) => prefix(i) + l)
    .join('\n')
  emit('update:modelValue', value.slice(0, lineStart) + replaced + value.slice(e))
  void nextTick(() => {
    el.focus()
    el.setSelectionRange(lineStart, lineStart + replaced.length)
  })
}

const actions = [
  { key: 'h', text: 'H', title: '标题', cls: 'font-bold', run: () => prefixLines(() => '### ') },
  { key: 'b', text: 'B', title: '加粗', cls: 'font-bold', run: () => wrap('**') },
  { key: 'i', text: 'I', title: '斜体', cls: 'italic', run: () => wrap('*') },
  { key: 'ul', text: '•', title: '列表', cls: 'text-lg', run: () => prefixLines(() => '- ') },
  { key: 'ol', text: '1.', title: '编号列表', cls: 'text-xs', run: () => prefixLines((i) => `${i + 1}. `) },
  { key: 'q', text: '❝', title: '引用', cls: '', run: () => prefixLines(() => '> ') },
  { key: 'a', text: '🔗', title: '链接', cls: 'text-xs', run: () => wrap('[', '](https://)', '链接文字') }
]

defineExpose({ focus: () => area.value?.focus() })
</script>
