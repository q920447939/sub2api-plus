<template>
  <div>
    <div class="flex h-7 items-stretch gap-[3px]" data-testid="cs-history">
      <div
        v-for="(b, i) in buckets"
        :key="i"
        class="min-w-[3px] flex-1 rounded-[3px] transition-opacity hover:opacity-70"
        :class="STATUS_META[b.status].bar"
        :title="tip(b)"
      />
    </div>
    <div class="mt-1 flex justify-between text-[11px] text-gray-400 dark:text-dark-400">
      <span>{{ span }}</span>
      <span>现在</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Granularity, HistoryBucket } from '../api'
import { STATUS_META, formatBucket } from '../status'

const props = defineProps<{ buckets: HistoryBucket[]; granularity: Granularity; span: string }>()

function tip(b: HistoryBucket): string {
  const when = formatBucket(b.t, props.granularity)
  const label = STATUS_META[b.status].label
  return b.score == null ? `${when} · ${label}` : `${when} · ${label} · 健康分 ${b.score}`
}
</script>
