<!-- [plus] 状态条：每格一小时（或一天），颜色和渠道状态一致；调整发生的那一格下面标一个小三角 -->
<template>
  <div class="relative flex items-stretch" :class="tall ? 'h-9 gap-[3px]' : 'h-[18px] gap-[2px]'">
    <div
      v-for="s in bars"
      :key="s.t"
      class="relative rounded-[2px] hover:opacity-60"
      :class="[STATUS_META[s.status].bar, fixed ? 'w-[5px] flex-none' : 'min-w-[2px] flex-1', tall ? 'rounded-[3px]' : '']"
      :title="tip(s)"
    >
      <i v-if="marked.has(s.t)" class="absolute -bottom-[7px] left-1/2 -ml-[3.5px] border-x-[3.5px] border-b-[4px] border-x-transparent border-b-gray-800 dark:border-b-gray-200" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Gran, Slot } from '../api'
import { STATUS_META, slotTime } from './format'

const props = withDefaults(defineProps<{ bars: Slot[]; gran?: Gran; marks?: string[]; fixed?: boolean; tall?: boolean }>(), {
  gran: 'hour',
  marks: () => [],
  fixed: false,
  tall: false
})

// 调整时间落在哪一格
const marked = computed(() => {
  const out = new Set<number>()
  const starts = props.bars.map((b) => b.t)
  for (const m of props.marks) {
    const t = new Date(m).getTime() / 1000
    let hit = -1
    for (const s of starts) if (s <= t) hit = s
    if (hit >= 0) out.add(hit)
  }
  return out
})

function tip(s: Slot): string {
  let t = `${slotTime(s.t, props.gran)} · ${STATUS_META[s.status].label}`
  if (s.score != null) t += ` · 健康分 ${s.score}`
  if (s.n) t += `\n${s.n} 次请求${s.err_pct != null ? ` · 失败 ${s.err_pct}%` : ''}${s.ttft != null ? ` · 首字 ${s.ttft} 秒` : ''}`
  if (marked.value.has(s.t)) t += '\n这段时间调整过权重'
  return t
}
</script>
