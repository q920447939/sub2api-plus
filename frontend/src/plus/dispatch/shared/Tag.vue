<!-- [plus] 状态标签：一个胶囊，下面可以带一行小字（比如模型名） -->
<template>
  <span class="inline-flex max-w-[160px] flex-col items-center gap-0.5" :class="clickable ? 'cursor-pointer' : ''" @click="clickable && emit('click')">
    <span class="whitespace-nowrap rounded-full border px-2.5 text-xs font-semibold leading-[18px]" :class="[TAG[tone], clickable ? 'hover:brightness-95' : '']" :title="tip">{{ text }}</span>
    <span v-if="sub" class="max-w-full truncate text-[11px] text-gray-500" :title="sub">{{ sub }}</span>
  </span>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ text: string; sub?: string; tone?: keyof typeof TAG; tip?: string; clickable?: boolean }>(), { tone: 'gray', clickable: false })
const emit = defineEmits<{ click: [] }>()
</script>

<script lang="ts">
export const TAG = {
  green: 'border-transparent bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
  amber: 'border-transparent bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  red: 'border-transparent bg-red-50 text-red-600 dark:bg-red-900/30 dark:text-red-300',
  dark: 'border-transparent bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900',
  tint: 'border-transparent bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300',
  violet: 'border-transparent bg-violet-50 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300',
  gray: 'border-gray-300 bg-white font-normal text-gray-500 dark:border-dark-500 dark:bg-transparent dark:text-dark-300',
  oamber: 'border-amber-400 bg-white text-amber-700 dark:bg-transparent dark:text-amber-300'
} as const
</script>
