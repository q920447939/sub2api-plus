<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-5 px-4 py-6 md:px-6" data-testid="cs-page">
      <!-- 顶栏：只保留标题、刷新提示与粒度切换 -->
      <header class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-2">
          <component :is="ChannelStatusNavIcon" class="h-6 w-6 text-primary-500" />
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">渠道状态</h1>
        </div>
        <div class="flex items-center gap-3">
          <span v-if="snapshot" class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-dark-300">
            <span class="h-2 w-2 animate-pulse rounded-full bg-primary-500" />
            每 {{ snapshot.refresh_seconds }} 秒自动刷新
          </span>
          <div class="flex rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
            <button
              v-for="g in GRANULARITIES"
              :key="g.value"
              type="button"
              class="rounded-md px-3 py-1 text-sm transition"
              :class="granularity === g.value
                ? 'bg-white font-semibold text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
                : 'text-gray-500 hover:text-gray-800 dark:text-dark-300 dark:hover:text-white'"
              @click="granularity = g.value"
            >{{ g.label }}</button>
          </div>
        </div>
      </header>

      <div v-if="disabled" class="card p-10 text-center text-gray-500">渠道状态暂未开放</div>
      <div v-else-if="!snapshot" class="card p-10 text-center text-gray-400">{{ error || '正在读取状态…' }}</div>
      <div v-else-if="!snapshot.platforms.length" class="card p-10 text-center text-gray-400">暂无可展示的渠道</div>

      <template v-else>
        <!-- 平台 Tab -->
        <nav class="flex flex-wrap gap-2" data-testid="cs-tabs">
          <button
            v-for="p in snapshot.platforms"
            :key="p.platform"
            type="button"
            class="flex items-center gap-2 rounded-full border px-4 py-1.5 text-sm transition"
            :class="p.platform === active?.platform
              ? 'border-gray-900 bg-white font-semibold text-gray-900 dark:border-white dark:bg-dark-800 dark:text-white'
              : 'border-gray-200 bg-white text-gray-600 hover:border-gray-400 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300'"
            @click="activeKey = p.platform"
          >
            <span class="h-2 w-2 rounded-full" :class="STATUS_META[worst(p)].dot" />
            {{ p.name }}
            <span class="text-xs text-gray-400">{{ p.groups.length }}</span>
          </button>
        </nav>

        <!-- 分组列表 -->
        <section
          v-if="active"
          class="card divide-y divide-gray-100 overflow-hidden p-0 dark:divide-dark-700"
          :class="{ 'opacity-60 transition-opacity': loading }"
        >
          <GroupRow
            v-for="g in active.groups"
            :key="g.group_id"
            :group="g"
            :granularity="snapshot.granularity"
            :span="span"
          />
        </section>

        <div class="flex flex-wrap justify-end gap-4 text-xs text-gray-500 dark:text-dark-300">
          <span v-for="s in STATUS_ORDER" :key="s" class="flex items-center gap-1.5">
            <span class="h-2.5 w-2.5 rounded-sm" :class="STATUS_META[s].bar" />{{ STATUS_META[s].label }}
          </span>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import type { PlatformView, Status } from './api'
import { GRANULARITIES, STATUS_META, STATUS_ORDER } from './status'
import { ChannelStatusNavIcon } from './navIcon'
import { useChannelStatus } from './useChannelStatus'
import GroupRow from './components/GroupRow.vue'

const { snapshot, granularity, loading, disabled, error } = useChannelStatus()

const activeKey = ref<string>('')
const active = computed<PlatformView | undefined>(() => {
  const ps = snapshot.value?.platforms ?? []
  return ps.find((p) => p.platform === activeKey.value) ?? ps[0]
})

const span = computed(() => GRANULARITIES.find((g) => g.value === snapshot.value?.granularity)?.span ?? '')

// Tab 圆点：取该平台下最差的已知状态；全部未知时显示灰色
const SEVERITY: Record<Status, number> = { unknown: 0, up: 1, warn: 2, down: 3 }
function worst(p: PlatformView): Status {
  return p.groups.reduce<Status>((acc, g) => (SEVERITY[g.status] > SEVERITY[acc] ? g.status : acc), 'unknown')
}
</script>
