<!-- [plus] 一排数字列（健康分 / 错误率 / 首字耗时 / 缓存命中）：主数字按好坏着色，下面一行小字 -->
<template>
  <td class="whitespace-nowrap">
    <div class="text-base font-semibold tabular-nums"><span :class="LV.score(c.score)">{{ c.score ?? '--' }}</span><small class="text-xs font-normal text-gray-400">/100</small></div>
    <div class="text-xs text-gray-500 tabular-nums">24h {{ times(c.n24) }}</div>
  </td>
  <td class="whitespace-nowrap">
    <div class="text-base font-semibold tabular-nums" :class="LV.err(c.err_pct)">{{ pct1(c.err_pct) }}</div>
    <div class="text-xs text-gray-500 tabular-nums">失败 {{ times(c.failed) }}</div>
  </td>
  <td class="whitespace-nowrap">
    <div class="text-base font-semibold tabular-nums" :class="LV.ttft(c.ttft)">{{ c.ttft == null ? '—' : `${c.ttft.toFixed(1)} 秒` }}</div>
    <div class="text-xs text-gray-500 tabular-nums">{{ c.tps == null ? '\u00a0' : `输出 ${Math.round(c.tps)} tok/s` }}</div>
  </td>
  <td class="whitespace-nowrap">
    <div class="text-base font-semibold tabular-nums" :class="LV.cache(c.cache)">{{ c.cache == null ? '—' : `${c.cache.toFixed(1)}%` }}</div>
    <div class="text-xs text-gray-500 tabular-nums">{{ c.cache_read ? `命中 ${tok(c.cache_read)}` : '\u00a0' }}</div>
  </td>
</template>

<script setup lang="ts">
import type { Cols } from '../api'
import { LV, pct1, times, tok } from './format'

defineProps<{ c: Cols }>()
</script>
