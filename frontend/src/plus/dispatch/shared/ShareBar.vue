<!-- [plus] 份额条 -->
<template>
  <div class="flex items-center gap-1.5 whitespace-nowrap text-xs">
    <div class="relative h-1.5 overflow-hidden rounded bg-gray-100 dark:bg-dark-700" :style="{ width: `${width}px` }">
      <i class="absolute inset-y-0 left-0" :class="tone === 'sug' ? 'bg-primary-500' : 'bg-gray-400'" :style="{ width: `${Math.min(100, value)}%` }" />
    </div>
    <span>{{ value.toFixed(value < 10 && value % 1 ? 1 : 0) }}%</span>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ value: number; tone?: 'now' | 'sug'; width?: number }>(), { tone: 'now', width: 70 })
</script>
