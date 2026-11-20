<template>
  <div class="w-full">
    <!-- 铺满整行：每格按宽度均分（flex-1），间距固定；高度压低到 20px，避免宽屏下格子显得粗笨。
         悬停提示整条共用一个浮层（事件委托，不给每格挂监听） -->
    <div class="relative" @pointerleave="hover = -1">
      <div
        class="flex h-5 items-stretch gap-[2px] md:gap-[3px]"
        data-testid="cs-history"
        @pointerover="onOver"
      >
        <div
          v-for="(b, i) in buckets"
          :key="i"
          :data-i="i"
          class="min-w-[2px] flex-1 rounded-[2px] transition-opacity"
          :class="[STATUS_META[b.status].bar, hover >= 0 && hover !== i ? 'opacity-50' : '']"
        />
      </div>
      <div
        v-if="cur"
        class="pointer-events-none absolute bottom-full z-20 mb-2 whitespace-nowrap rounded-lg bg-gray-900 px-2.5 py-1.5 text-xs text-white shadow-lg dark:bg-dark-600"
        :style="tipStyle"
        data-testid="cs-history-tip"
      >
        <div class="text-gray-300">{{ cur.when }}</div>
        <div class="mt-0.5 flex items-center gap-1.5">
          <span class="h-2 w-2 rounded-full" :class="STATUS_META[cur.b.status].dot" />
          <span>{{ STATUS_META[cur.b.status].label }}</span>
          <span v-if="cur.b.score != null" class="text-gray-300">· 健康分 <b class="tabular-nums text-white">{{ cur.b.score }}</b></span>
        </div>
        <div v-if="cur.b.score == null" class="mt-0.5 text-[11px] text-gray-400">该时段请求过少，不评分</div>
      </div>
    </div>
    <div class="mt-1 flex justify-between text-[11px] text-gray-400 dark:text-dark-400">
      <span>{{ span }}</span>
      <span>现在</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Granularity, HistoryBucket } from '../api'
import { STATUS_META, formatBucketRange } from '../status'

const props = defineProps<{ buckets: HistoryBucket[]; granularity: Granularity; span: string }>()

const hover = ref(-1)

function onOver(e: PointerEvent) {
  const i = Number((e.target as HTMLElement).dataset?.i)
  if (Number.isInteger(i)) hover.value = i
}

const cur = computed(() => {
  const b = props.buckets[hover.value]
  if (!b) return null
  const last = hover.value === props.buckets.length - 1
  return { b, when: formatBucketRange(b.t, props.granularity) + (last ? '（进行中）' : '') }
})

// 浮层跟随所在格；靠两端时改为左/右对齐，避免超出卡片
const tipStyle = computed(() => {
  const n = props.buckets.length
  const i = hover.value
  const pct = ((i + 0.5) / n) * 100
  if (i < n * 0.15) return { left: `${pct}%`, transform: 'translateX(-12px)' }
  if (i > n * 0.85) return { left: `${pct}%`, transform: 'translateX(calc(-100% + 12px))' }
  return { left: `${pct}%`, transform: 'translateX(-50%)' }
})
</script>
