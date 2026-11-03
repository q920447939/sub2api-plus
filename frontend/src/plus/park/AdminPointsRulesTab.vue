<template>
  <div v-if="cfg" class="card divide-y divide-gray-100 dark:divide-dark-700">
    <div class="flex flex-wrap items-center gap-3 p-5 text-sm text-gray-600 dark:text-gray-400">
      <span class="w-44 font-medium text-gray-900 dark:text-white">无变动多少天后清零</span>
      <input v-model.number="cfg.expire_days" type="number" min="0" class="input w-28" />
      <span class="text-xs">天，0 表示永不过期。从最后一次积分收入或支出算起</span>
    </div>
    <div class="flex flex-wrap items-center gap-3 p-5 text-sm text-gray-600 dark:text-gray-400">
      <span class="w-44 font-medium text-gray-900 dark:text-white">提前多少天提醒</span>
      <input v-model.number="cfg.warn_days" type="number" min="0" class="input w-28" />
      <span class="text-xs">天，0 表示不提醒；需小于清零天数。提醒显示在积分乐园的余额旁</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { park } from './adminState'

const cfg = computed(() => park.points)
</script>
