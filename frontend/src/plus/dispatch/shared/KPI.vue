<!-- [plus] 顶部数字卡（可点击筛选 / 跳转） -->
<template>
  <button type="button" class="card px-4 py-3 text-left transition hover:shadow-md" @click="emit('click')">
    <div class="text-2xl font-semibold tabular-nums" :class="TONE[tone ?? '']">{{ n }}</div>
    <div class="mt-0.5 text-xs text-gray-500">{{ label }}</div>
  </button>
</template>

<script setup lang="ts">
defineProps<{ n: number; label: string; tone?: string }>()
const emit = defineEmits<{ click: [] }>()
const TONE: Record<string, string> = { red: 'text-red-600', amber: 'text-amber-600', green: 'text-green-600', blue: 'text-blue-600', '': 'text-gray-900 dark:text-white' }
</script>
