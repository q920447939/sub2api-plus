<template>
  <div
    class="grid grid-cols-1 items-center gap-3 px-5 py-4 md:grid-cols-[minmax(0,1.3fr)_7rem_minmax(0,3fr)] md:gap-6"
    data-testid="cs-group-row"
  >
    <div class="min-w-0">
      <div class="truncate font-semibold text-gray-900 dark:text-white">{{ group.name }}</div>
      <div v-if="group.subtitle" class="truncate text-xs text-gray-400 dark:text-dark-400">{{ group.subtitle }}</div>
    </div>
    <div class="flex items-center gap-2 md:block">
      <span class="inline-block rounded-full px-2.5 py-0.5 text-xs font-medium" :class="STATUS_META[group.status].badge">
        {{ STATUS_META[group.status].label }}
      </span>
      <div class="text-xs text-gray-500 dark:text-dark-300 md:mt-1">
        健康分
        <span class="ml-0.5 font-semibold tabular-nums text-gray-800 dark:text-gray-100">{{ group.score ?? '--' }}</span>
      </div>
    </div>
    <HistoryBar :buckets="group.history" :granularity="granularity" :span="span" />
  </div>
</template>

<script setup lang="ts">
import type { Granularity, GroupView } from '../api'
import { STATUS_META } from '../status'
import HistoryBar from './HistoryBar.vue'

defineProps<{ group: GroupView; granularity: Granularity; span: string }>()
</script>
