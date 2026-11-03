<template>
  <ol class="grid grid-cols-7 gap-1.5 md:gap-2" data-testid="checkin-week">
    <li
      v-for="d in 7"
      :key="d"
      class="flex flex-col items-center justify-center border-[3px] nb-line py-2 text-center transition-transform"
      :class="cellClass(d)"
      :aria-current="d === pos ? 'step' : undefined"
    >
      <span class="nb-head text-[11px] md:text-xs">第{{ d }}天</span>
      <span class="text-base md:text-lg">{{ isDone(d) ? '✓' : d === 7 ? '🎁' : '🪙' }}</span>
      <span class="nb-head text-[10px] tabular-nums md:text-xs">+{{ checkinRewardFor(status, d) }}</span>
    </li>
  </ol>
</template>

<script setup lang="ts">
// [plus] 7 天连签进度条，签到页和签到弹窗共用
import { computed } from 'vue'
import type { CheckinStatus } from './api'
import { checkinTodayPos, checkinRewardFor } from './week'

const props = defineProps<{ status: CheckinStatus }>()

const pos = computed(() => checkinTodayPos(props.status))

function isDone(d: number): boolean {
  return d < pos.value || (d === pos.value && props.status.today_checked)
}

function cellClass(d: number): string {
  if (isDone(d)) return 'bg-[#ff2e93] text-white'
  if (d === pos.value) return 'bg-[#ffe600] outline outline-4 outline-[#ff2e93] -translate-y-1'
  return d === 7 ? 'nb-surface-2' : 'nb-surface'
}
</script>
