<!-- [plus] 上游调度台 · 待处理：维度变成可疑 / 异常、检测请求连续失败、有流量的模型没被测到时自动出现；恢复正常后自动消失 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-events">
      <Tabs v-model="tab" :tabs="TABS.map((t) => ({ ...t, count: counts[t.k] ?? 0 }))" />
      <div v-if="!events" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
      <div v-else-if="!events.length" class="py-16 text-center text-sm text-gray-400">没有{{ TABS.find((t) => t.k === tab)?.n }}的事项</div>
      <div v-for="row in events ?? []" :key="row.event.id" class="rounded-xl border border-l-4 bg-white p-4 dark:bg-dark-800" :class="row.event.level === 'bad' ? 'border-l-red-500' : 'border-l-amber-400'" data-testid="dispatch-event">
        <div class="flex flex-wrap items-center gap-2 text-xs">
          <LevelPill v-if="row.event.kind === 'dim'" :level="row.event.level" />
          <span v-else class="rounded-md border border-amber-200 bg-amber-50 px-2 py-0.5 text-amber-700">{{ row.event.kind === 'fault' ? '检测请求连续失败' : '有流量的模型未被检测' }}</span>
          <span v-if="row.dimension" class="rounded-full px-2" :class="ROLE_CLASS[row.dimension.role as Role]">{{ ROLE_TEXT[row.dimension.role as Role] }}</span>
          <span class="text-gray-500">{{ [row.group, row.account, row.event.model].filter(Boolean).join(' · ') }}</span>
          <span class="flex-1" />
          <span v-if="row.share != null" class="text-gray-400">流量 {{ row.share }}%</span>
          <span class="text-gray-400">{{ fullTime(row.event.first_at) }} 起</span>
        </div>
        <div class="mt-2 font-semibold">{{ row.event.title }}</div>
        <ul class="mt-1 list-disc space-y-0.5 pl-5 text-sm text-gray-600 dark:text-dark-200">
          <li v-for="(ev, i) in row.event.evidence ?? []" :key="i">{{ ev }}</li>
        </ul>
        <div v-if="row.impact" class="mt-2 rounded bg-gray-50 px-3 py-1.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-200">
          {{ row.impact }}
          <template v-if="row.suggest && row.suggest.action !== 'normal' && !row.suggest.settled"> · 调度建议：{{ row.suggest.before }}% → {{ row.suggest.after }}%</template>
          <template v-if="row.adjustment"> · 当前调整：{{ adjustText(row.adjustment.action, row.adjustment.factor) }}</template>
        </div>
        <div class="mt-3 flex flex-wrap gap-2">
          <template v-if="row.event.kind === 'dim'">
            <template v-if="row.event.state === 'todo'">
              <button type="button" class="btn btn-primary btn-sm" data-testid="dispatch-event-confirm" @click="setState(row, 'confirmed')">确认有问题</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="setState(row, 'false')">误报</button>
            </template>
            <button v-else type="button" class="btn btn-secondary btn-sm" @click="setState(row, 'todo')">撤销{{ row.event.state === 'confirmed' ? '确认' : row.event.state === 'false' ? '误报' : '忽略' }}</button>
            <button type="button" class="btn btn-secondary btn-sm" @click="openAdjust(row)">调整</button>
            <router-link class="btn btn-secondary btn-sm" :to="{ path: `${BASE}/account/${row.event.account_id}`, query: { group: row.event.group_id, dim: row.event.dimension_id, model: row.event.model } }">诊断详情</router-link>
          </template>
          <template v-else-if="row.event.kind === 'fault'">
            <button v-if="row.event.state === 'todo'" type="button" class="btn btn-primary btn-sm" @click="setState(row, 'ignored')">知道了</button>
            <button v-else type="button" class="btn btn-secondary btn-sm" @click="setState(row, 'todo')">移回待处理</button>
            <router-link class="btn btn-secondary btn-sm" :to="{ path: `${BASE}/runs`, query: { item: row.event.item_id, account: row.event.account_id } }">看检测记录</router-link>
          </template>
          <template v-else>
            <router-link class="btn btn-primary btn-sm" :to="{ path: `${BASE}/config`, query: { tab: 'plans', item: row.event.item_id } }">补计划</router-link>
            <button v-if="row.event.state === 'todo'" type="button" class="btn btn-secondary btn-sm" @click="setState(row, 'ignored')">忽略</button>
            <button v-else type="button" class="btn btn-secondary btn-sm" @click="setState(row, 'todo')">移回待处理</button>
          </template>
        </div>
      </div>
    </div>
    <AdjustDialog :target="adjustTarget" @close="adjustTarget = null" @done="adjustTarget = null; load()" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Role } from '../api'
import { DISPATCH_BASE as BASE } from '../routes'
import AdjustDialog, { type AdjustTarget } from '../shared/AdjustDialog.vue'
import LevelPill from '../shared/LevelPill.vue'
import Tabs from '../shared/Tabs.vue'
import { adjustText, fullTime, ROLE_CLASS, ROLE_TEXT } from '../shared/format'
import { loadMeta, meta } from '../shared/meta'

const TABS = [{ k: 'todo', n: '待处理' }, { k: 'confirmed', n: '已确认' }, { k: 'false', n: '误报' }, { k: 'ignored', n: '已忽略' }]
const app = useAppStore()
const tab = ref('todo')
const events = ref<any[] | null>(null)
const counts = ref<Record<string, number>>({})
const error = ref('')
const adjustTarget = ref<AdjustTarget | null>(null)

async function load() {
  try {
    const r = await dispatchApi.events(tab.value)
    events.value = r.events ?? []
    counts.value = r.counts
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}
watch(tab, load, { immediate: true })
void loadMeta()

async function setState(row: any, state: string) {
  try {
    await dispatchApi.setEventState(row.event.id, state)
    app.showSuccess('已更新，调度建议会跟着变')
    await load()
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '操作失败'))
  }
}

function openAdjust(row: any) {
  adjustTarget.value = {
    groupId: row.event.group_id, groupName: row.group, accountId: row.event.account_id, accountName: row.account, worst: row.event.level,
    oneLine: row.event.title, current: row.adjustment ?? null, suggest: row.suggest ?? null,
    names: Object.fromEntries((meta.value?.accounts ?? []).map((a) => [String(a.id), a.name]))
  }
}
</script>
