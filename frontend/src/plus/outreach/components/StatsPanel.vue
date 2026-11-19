<!-- [plus] 效果统计：共性（发送进度）+ 目的声明的特性指标。指标来自后端，页面不写死任何目的。 -->
<template>
  <div class="space-y-6">
    <section class="card p-6">
      <div v-if="showProgress" class="mb-4">
        <div class="mb-2 flex items-center justify-between text-sm">
          <span class="font-medium text-gray-900 dark:text-white">执行进度</span>
          <span class="tabular-nums text-gray-600 dark:text-dark-300">{{ done }} / {{ stats.total }}</span>
        </div>
        <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
          <div class="h-full bg-primary-500 transition-all" :style="{ width: stats.total ? `${(done / stats.total) * 100}%` : '0%' }" />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
        <MetricCard label="已发送" :value="stats.sent" />
        <MetricCard label="待发送" :value="stats.queued" />
        <MetricCard label="发送失败" :value="stats.failed" :warn="stats.failed > 0" :sub="stats.bounced ? `其中退信 ${stats.bounced}` : ''" />
        <MetricCard label="已跳过" :value="stats.skipped" sub="发送前状态变化 / 频控 / 已取消" />
        <MetricCard v-if="marketing" label="退订" :value="stats.unsub" :sub="pct(stats.unsub, stats.sent)" />
      </div>
    </section>

    <section v-if="stats.sent > 0 && stats.metrics?.length" class="card p-6">
      <h2 class="mb-1 text-sm font-medium text-gray-900 dark:text-white">{{ title }}</h2>
      <p class="mb-4 text-xs text-gray-500">以已发送为基数。这些是发送之后发生的行为，没有对照组，只能说明相关，不能证明是邮件带来的。</p>
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <MetricCard
          v-for="m in stats.metrics"
          :key="m.key"
          :label="m.primary ? `${m.label}（核心）` : m.label"
          :value="m.value"
          :sub="pct(m.value, stats.sent)"
          :title="m.desc"
          :highlight="m.primary"
        />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import MetricCard from './MetricCard.vue'
import { pct, type DeliveryStats, type MetricView } from '../api'

const props = withDefaults(defineProps<{ stats: DeliveryStats & { metrics?: MetricView[] }; marketing: boolean; showProgress?: boolean; title?: string }>(), {
  showProgress: false,
  title: '效果'
})
const done = computed(() => props.stats.sent + props.stats.failed + props.stats.skipped)
</script>
