<!--
  [plus] 奖池编辑器（区间 + 权重）。奖池 A（真实发奖）与奖池 B（虚拟动态）各用一份，互不联动。
  权重允许为 0（该档永不抽中），但整池权重之和必须 > 0。
-->
<template>
  <div class="space-y-3">
    <div class="overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-gray-500 dark:text-gray-400">
            <th class="pb-2 pr-2 font-medium">奖档</th>
            <th class="pb-2 pr-2 font-medium">最小 $</th>
            <th class="pb-2 pr-2 font-medium">最大 $</th>
            <th class="pb-2 pr-2 font-medium">权重</th>
            <th class="pb-2 pr-2 font-medium">概率</th>
            <th class="pb-2 font-medium"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(t, i) in tiers" :key="i" :class="{ 'opacity-50': !t.weight }">
            <td class="py-1 pr-2 text-gray-600 dark:text-gray-400">
              {{ i + 1 }}
            </td>
            <td class="py-1 pr-2">
              <input
                v-model.number="t.min"
                type="number"
                min="0"
                step="0.01"
                class="input w-24"
                :aria-label="`奖档 ${i + 1} 最小金额`"
              />
            </td>
            <td class="py-1 pr-2">
              <input
                v-model.number="t.max"
                type="number"
                min="0"
                step="0.01"
                class="input w-24"
                :aria-label="`奖档 ${i + 1} 最大金额`"
              />
            </td>
            <td class="py-1 pr-2">
              <input
                v-model.number="t.weight"
                type="number"
                min="0"
                step="1"
                class="input w-20"
                :aria-label="`奖档 ${i + 1} 权重`"
              />
            </td>
            <td class="py-1 pr-2 tabular-nums text-xs text-gray-500 dark:text-gray-400">
              {{ pct(i) }}
            </td>
            <td class="py-1">
              <button
                class="btn btn-secondary btn-sm"
                :disabled="tiers.length <= 1"
                :aria-label="`删除奖档 ${i + 1}`"
                @click="tiers.splice(i, 1)"
              >
                删除
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div class="flex flex-wrap items-center gap-4">
      <button class="btn btn-secondary btn-sm" @click="addTier">+ 添加奖档</button>
      <span class="text-xs text-gray-500 dark:text-gray-400">
        期望单次 ≈ <b class="tabular-nums">${{ expected.toFixed(3) }}</b>
        <template v-if="maxReachable > 0">
          · 可抽到的最高
          <b class="tabular-nums">${{ maxReachable.toFixed(2) }}</b></template
        >
      </span>
      <span v-if="error" class="text-xs text-red-600 dark:text-red-400" role="alert">{{ error }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { PrizeTier } from '@/api/lottery'

const tiers = defineModel<PrizeTier[]>({ required: true })

const total = computed(() => tiers.value.reduce((a, t) => a + (t.weight || 0), 0))

function pct(i: number) {
  if (!total.value) return '-'
  return (((tiers.value[i]?.weight || 0) / total.value) * 100).toFixed(1) + '%'
}

const expected = computed(() =>
  total.value ? tiers.value.reduce((a, t) => a + ((t.weight || 0) / total.value) * ((t.min + t.max) / 2), 0) : 0
)

const maxReachable = computed(() => tiers.value.reduce((m, t) => (t.weight > 0 && t.max > m ? t.max : m), 0))

const error = computed(() => {
  if (tiers.value.some((t) => t.min < 0 || t.max < t.min)) return '存在最小值大于最大值或负数的奖档'
  if (tiers.value.some((t) => t.weight < 0)) return '权重不能为负'
  if (!total.value) return '至少要有一个奖档权重 > 0'
  return ''
})

function addTier() {
  const last = tiers.value[tiers.value.length - 1]
  tiers.value.push({
    min: last?.max ?? 0.1,
    max: last ? last.max * 2 : 0.2,
    weight: 0
  })
}

defineExpose({ error })
</script>
