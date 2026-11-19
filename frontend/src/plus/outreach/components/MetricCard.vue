<!-- [plus] 一个统计数字 -->
<template>
  <div class="rounded-lg p-3" :class="highlight ? 'bg-primary-50 ring-1 ring-primary-200 dark:bg-primary-900/20 dark:ring-primary-800' : 'bg-gray-50 dark:bg-dark-800'">
    <p class="text-xs text-gray-500 dark:text-dark-400">{{ label }}</p>
    <p class="mt-1 text-xl font-semibold tabular-nums" :class="warn ? 'text-red-600' : 'text-gray-900 dark:text-white'">{{ value }}</p>
    <p v-if="sub" class="text-xs text-gray-400">{{ sub }}</p>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ label: string; value: number; sub?: string; warn?: boolean; highlight?: boolean }>(), {
  sub: '',
  warn: false,
  highlight: false
})
</script>
