<!-- [plus] 账号链路：#3005 中转E-01 → #3006 中转E-02（失败的账号划线） -->
<template>
  <span class="inline-flex flex-wrap items-center gap-1">
    <template v-if="chain.length">
      <template v-for="(h, i) in chain" :key="i">
        <span v-if="i > 0" class="text-gray-400">→</span>
        <span
          class="whitespace-nowrap rounded border px-1.5 text-[11px] leading-5"
          :class="h.ok
            ? 'border-indigo-100 bg-indigo-50 text-indigo-700 dark:border-indigo-800 dark:bg-indigo-900/30 dark:text-indigo-300'
            : 'border-red-100 bg-red-50 text-red-600 line-through dark:border-red-900 dark:bg-red-900/30'"
          :title="hopTitle(h)"
        >#{{ h.account_id }}<template v-if="!compact && h.account_name"> {{ h.account_name }}</template></span>
      </template>
    </template>
    <span v-else class="text-xs text-gray-400">{{ attributed ? '未知' : '反查中…' }}</span>
  </span>
</template>

<script setup lang="ts">
import type { HopView } from './api'

withDefaults(defineProps<{ chain: HopView[]; attributed?: boolean; compact?: boolean }>(), { attributed: true, compact: false })

function hopTitle(h: HopView): string {
  const parts = [`#${h.account_id} ${h.account_name || ''}`.trim()]
  if (!h.ok) parts.push(`失败${h.status_code ? ` HTTP ${h.status_code}` : ''}${h.message ? `：${h.message}` : ''}`)
  return parts.join('\n')
}
</script>
