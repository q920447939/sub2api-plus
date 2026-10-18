<!-- [plus] 今日手气榜：前三名领奖台 + 4~10 名列表（不展示中奖时间） -->
<template>
  <div class="card p-5">
    <div class="flex items-center justify-between">
      <h2 class="flex items-center gap-2 text-base font-semibold text-gray-900 dark:text-white">
        <Icon name="trophy" size="sm" class="text-amber-500" /> 今日手气榜
      </h2>
      <span class="text-xs text-gray-400">单次最高 · 每日 0 点重置</span>
    </div>

    <template v-if="items.length">
      <!-- 领奖台：视觉顺序 2-1-3，读屏顺序仍为 1-2-3 -->
      <ol class="mt-4 grid grid-cols-3 items-end gap-2" aria-label="前三名">
        <li v-for="p in podium" :key="p.rank" class="flex min-w-0 flex-col items-center" :style="{ order: p.order }">
          <div class="relative">
            <span
              class="flex items-center justify-center rounded-full font-bold ring-2"
              :class="[
                avatarClass(p.item.display_name),
                p.rank === 1 ? 'h-12 w-12 text-lg ring-amber-400' : 'h-10 w-10 ring-gray-200 dark:ring-dark-600'
              ]"
              aria-hidden="true"
              >{{ avatarText(p.item.display_name) }}</span
            >
            <span
              class="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-bold text-white"
              :class="medal[p.rank - 1]"
            >
              {{ p.rank }}
            </span>
          </div>
          <p class="mt-1 w-full truncate text-center text-xs text-gray-600 dark:text-gray-400">
            {{ p.item.display_name }}
          </p>
          <div
            class="mt-1 flex w-full flex-col items-center justify-start rounded-t-lg pt-2"
            :class="[stand[p.rank - 1], p.rank === 1 ? 'h-16' : p.rank === 2 ? 'h-12' : 'h-9']"
          >
            <span class="text-sm font-bold tabular-nums text-orange-600 dark:text-orange-400"
              >${{ p.item.amount.toFixed(2) }}</span
            >
          </div>
        </li>
      </ol>

      <ol v-if="rest.length" class="mt-3 space-y-1.5 text-sm" start="4">
        <li
          v-for="(item, i) in rest"
          :key="item.display_name"
          class="flex items-center gap-3 rounded-lg px-2 py-1 hover:bg-gray-50 dark:hover:bg-dark-800"
        >
          <span class="w-5 shrink-0 text-center text-xs font-semibold tabular-nums text-gray-400">{{ i + 4 }}</span>
          <span
            class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold"
            :class="avatarClass(item.display_name)"
            aria-hidden="true"
          >
            {{ avatarText(item.display_name) }}
          </span>
          <span class="min-w-0 flex-1 truncate text-gray-700 dark:text-gray-300">{{ item.display_name }}</span>
          <span class="w-16 text-right font-semibold tabular-nums text-orange-600 dark:text-orange-400"
            >${{ item.amount.toFixed(2) }}</span
          >
        </li>
      </ol>
    </template>
    <div v-else class="mt-4 rounded-xl border border-dashed border-gray-200 py-8 text-center dark:border-dark-600">
      <Icon name="trophy" size="lg" class="mx-auto text-gray-300 dark:text-dark-500" />
      <p class="mt-2 text-sm text-gray-400">今天还没有人上榜，快来抢第一</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import type { LeaderboardItem } from '@/api/lottery'
import { avatarClass, avatarText } from './feedUtils'

const props = defineProps<{ items: LeaderboardItem[] }>()

const medal = ['bg-amber-400', 'bg-gray-400', 'bg-orange-400']
const stand = [
  'bg-gradient-to-b from-amber-100 to-amber-50 dark:from-amber-900/40 dark:to-amber-900/10',
  'bg-gradient-to-b from-gray-100 to-gray-50 dark:from-dark-700 dark:to-dark-800',
  'bg-gradient-to-b from-orange-100 to-orange-50 dark:from-orange-900/30 dark:to-orange-900/10'
]

// 只有 1~2 人时也保持领奖台居中
const podium = computed(() => {
  const order = [2, 1, 3]
  return props.items.slice(0, 3).map((item, i) => ({ item, rank: i + 1, order: order[i] }))
})
const rest = computed(() => props.items.slice(3))
</script>
