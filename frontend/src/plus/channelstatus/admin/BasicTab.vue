<template>
  <div v-if="c" class="space-y-5">
    <section class="card space-y-4 p-5">
      <h3 class="font-semibold text-gray-900 dark:text-white">功能开关</h3>
      <div class="flex items-center justify-between gap-4">
        <div>
          <div class="font-medium text-gray-800 dark:text-gray-100">启用渠道状态</div>
          <p class="text-xs text-gray-400">
            开启后用户侧边栏显示「渠道状态」，并由本功能自行驱动被动统计。上线时请在系统设置里关闭上游「渠道监控」，避免出现两个同名入口。
          </p>
        </div>
        <Toggle v-model="c.enabled" />
      </div>
      <div class="flex items-center justify-between gap-4 border-t border-gray-100 pt-4 dark:border-dark-700">
        <div>
          <div class="font-medium text-gray-800 dark:text-gray-100">自动展示新分组</div>
          <p class="text-xs text-gray-400">新建的分组是否默认出现在用户端（关闭则需在「展示编排」中手动开启）</p>
        </div>
        <Toggle v-model="c.auto_show_new_groups" />
      </div>
    </section>

    <section class="card p-5">
      <h3 class="font-semibold text-gray-900 dark:text-white">刷新与窗口</h3>
      <p class="mb-4 text-xs text-gray-400">当前状态 = 最近「判定窗口」内的真实调用计算；用户端按刷新周期轮询（服务端缓存 30 秒）</p>
      <div class="grid gap-4 md:grid-cols-3">
        <label class="block text-sm text-gray-600 dark:text-dark-300">
          用户端刷新周期
          <select v-model.number="c.refresh_seconds" class="input mt-1">
            <option v-for="v in [30, 60, 120, 300]" :key="v" :value="v">{{ v }} 秒</option>
          </select>
        </label>
        <label class="block text-sm text-gray-600 dark:text-dark-300">
          当前状态判定窗口
          <select v-model.number="c.judge_window_minutes" class="input mt-1">
            <option v-for="v in [5, 15, 30, 60]" :key="v" :value="v">最近 {{ v }} 分钟</option>
          </select>
        </label>
        <label class="block text-sm text-gray-600 dark:text-dark-300">
          默认历史粒度
          <select v-model="c.default_granularity" class="input mt-1">
            <option v-for="g in GRANULARITIES" :key="g.value" :value="g.value">{{ g.label }}（{{ g.span }}）</option>
          </select>
        </label>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Toggle from '@/components/common/Toggle.vue'
import { GRANULARITIES } from '../status'
import { settings } from './state'

const c = computed(() => settings.draft)
</script>
