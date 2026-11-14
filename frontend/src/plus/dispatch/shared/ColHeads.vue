<!-- [plus] 数字列的表头：可排序，带口径说明 -->
<template>
  <th v-for="col in COLS" :key="col.key" class="whitespace-nowrap" :class="sortable ? 'cursor-pointer select-none hover:text-gray-900 dark:hover:text-white' : ''" @click="sortable && emit('sort', col.key)">
    {{ col.label }}<span class="ml-1 cursor-help text-gray-400" :title="col.tip">ⓘ</span>
    <span v-if="sort?.key === col.key" class="ml-1 text-primary-600">{{ sort.dir > 0 ? '▲' : '▼' }}</span>
  </th>
</template>

<script setup lang="ts">
import { COLS } from './format'

defineProps<{ sortable?: boolean; sort?: { key: string; dir: number } | null }>()
const emit = defineEmits<{ sort: [key: string] }>()
</script>
