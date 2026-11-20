<template>
  <!-- 两层：上层名称独占整行（完整显示，可换行）+ 倍率；下层历史条（固定细格，靠左）+ 状态与健康分（靠右） -->
  <div class="space-y-3 px-5 py-4" data-testid="cs-group-row">
    <div class="min-w-0">
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
        <BrandLogo :platform="platform" :name="group.name" size="18px" />
        <span class="break-words font-semibold text-gray-900 [overflow-wrap:anywhere] dark:text-white" data-testid="cs-group-name">
          {{ group.name }}
        </span>
        <span
          class="badge badge-gray shrink-0 tabular-nums"
          title="分组基础倍率（不含高峰时段倍率与专属倍率）"
          data-testid="cs-group-rate"
        >{{ rate }}</span>
      </div>
      <div v-if="group.subtitle" class="mt-0.5 break-words text-xs text-gray-400 [overflow-wrap:anywhere] dark:text-dark-400">
        {{ group.subtitle }}
      </div>
    </div>
    <!-- 窄屏：状态在上、历史条在下并占满宽度；md 以上：历史条占满左侧剩余宽度，状态与健康分靠右并与色条对齐 -->
    <div class="flex flex-col gap-2 md:flex-row md:items-start md:gap-6">
      <div class="flex shrink-0 items-center gap-3 md:order-2 md:h-5 md:w-40 md:justify-end">
        <span class="inline-block rounded-full px-2.5 py-0.5 text-xs font-medium" :class="STATUS_META[group.status].badge">
          {{ STATUS_META[group.status].label }}
        </span>
        <div class="text-xs text-gray-500 dark:text-dark-300">
          健康分
          <span class="ml-0.5 font-semibold tabular-nums text-gray-800 dark:text-gray-100">{{ group.score ?? '--' }}</span>
        </div>
      </div>
      <div class="min-w-0 flex-1 md:order-1">
        <HistoryBar :buckets="group.history" :granularity="granularity" :span="span" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Granularity, GroupView } from '../api'
import { STATUS_META } from '../status'
import { formatMultiplier } from '@/utils/formatters'
import HistoryBar from './HistoryBar.vue'
import BrandLogo from './BrandLogo.vue'

const props = defineProps<{ group: GroupView; platform: string; granularity: Granularity; span: string }>()

// 与密钥页 GroupBadge 一致的写法：0.08x、1.00x
const rate = computed(() => `${formatMultiplier(props.group.rate_multiplier ?? 1)}x`)
</script>
