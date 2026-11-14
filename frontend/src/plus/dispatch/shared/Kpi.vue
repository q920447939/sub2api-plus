<!-- [plus] 一排大数字里的一格 -->
<template>
  <div class="px-5 py-3.5" :class="[clickable ? 'cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-800' : '', active ? 'bg-primary-50 dark:bg-primary-900/20' : '']" @click="clickable && emit('click')">
    <div class="text-xs text-gray-500">{{ label }}</div>
    <div class="mt-0.5 whitespace-nowrap text-2xl font-semibold tabular-nums" :class="tone">{{ value }}<small v-if="sub" class="ml-1.5 text-xs font-normal text-gray-500">{{ sub }}</small></div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ label: string; value: string; sub?: string; tone?: string; clickable?: boolean; active?: boolean }>()
const emit = defineEmits<{ click: [] }>()
</script>
