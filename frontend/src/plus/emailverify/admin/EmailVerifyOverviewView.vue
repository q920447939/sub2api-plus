<!-- [plus] 管理端「邮箱验证」：概览 / 验证记录 / 设置 三个标签页。
     本模块只管「验证」这个领域；发邀请邮件交给触达中心（概览页的按钮跳过去新建活动）。 -->
<template>
  <AppLayout>
    <div class="space-y-6 pb-16">
      <div class="border-b border-gray-200 dark:border-dark-700" role="tablist">
        <nav class="-mb-px flex flex-wrap gap-6">
          <button
            v-for="t in tabs"
            :key="t.key"
            type="button"
            role="tab"
            :aria-selected="tab === t.key"
            class="border-b-2 px-1 pb-3 text-sm font-medium transition-colors"
            :class="tab === t.key
              ? 'border-primary-500 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
            @click="setTab(t.key)"
          >{{ t.label }}</button>
        </nav>
      </div>

      <!-- 概览 -->
      <template v-if="tab === 'overview'">
        <div v-if="!stats" class="card p-16 text-center text-sm text-gray-400">{{ error || '正在加载…' }}</div>
        <template v-else>
          <div class="grid grid-cols-2 gap-4 lg:grid-cols-5">
            <StatCard label="已验证" :value="stats.verified" tone="text-green-600" :sub="`覆盖率 ${coverage}`" />
            <StatCard label="待验证" :value="stats.pending" tone="text-amber-600" />
            <StatCard label="待验证 · 充值过" :value="stats.pending_paid" tone="text-blue-600" sub="首要邀请对象" />
            <StatCard label="待验证 · 没充值" :value="stats.pending_free" />
            <StatCard label="无效邮箱" :value="stats.invalid" tone="text-red-600" sub="已硬退信，不再发送" />
          </div>

          <div class="grid items-start gap-6 xl:grid-cols-2">
            <section class="card">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-base font-medium text-gray-900 dark:text-white">验证途径</h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">已累计发放验证奖励 {{ stats.reward_total }} 积分</p>
              </div>
              <div class="divide-y divide-gray-100 dark:divide-dark-700">
                <div v-for="(label, key) in SOURCE_LABEL" :key="key" class="flex items-center justify-between px-6 py-3 text-sm">
                  <span class="text-gray-700 dark:text-dark-200">{{ label }}</span>
                  <span class="tabular-nums text-gray-900 dark:text-white">{{ stats.by_source[key] ?? 0 }}</span>
                </div>
              </div>
            </section>

            <section class="card">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-base font-medium text-gray-900 dark:text-white">邀请待验证用户</h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                  邀请邮件由「触达中心」发送：点击后进入新建活动，已预填目的和人群；生成批次、核对名单后点「开始执行」才会发信。
                  没验证的用户登录后也会在站内看到提醒。
                </p>
              </div>
              <div class="flex flex-wrap gap-3 px-6 py-5">
                <button type="button" class="btn btn-primary" @click="invite('pending_paid')">邀请「待验证老客」（{{ stats.pending_paid }} 人）</button>
                <button type="button" class="btn btn-secondary" @click="invite('pending_all')">邀请全部待验证用户（{{ stats.pending }} 人）</button>
              </div>
            </section>
          </div>
        </template>
      </template>

      <!-- 验证记录 -->
      <section v-else-if="tab === 'records'" class="card">
        <div class="flex flex-wrap items-center gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <input v-model="q" class="input w-64" placeholder="搜索邮箱 / 用户名" @keyup.enter="reload" />
          <Select v-model="source" :options="sourceOptions" class="w-40" />
          <button type="button" class="btn btn-secondary" @click="reload">查询</button>
        </div>
        <DataTable :columns="columns" :data="records" :loading="recLoading" row-key="id">
          <template #cell-user="{ row }">
            <span class="text-sm text-gray-900 dark:text-white">{{ row.username || `#${row.user_id}` }}</span>
            <p class="text-xs text-gray-400">ID {{ row.user_id }}</p>
          </template>
          <template #cell-email="{ row }">
            <span class="text-sm text-gray-700 dark:text-dark-200">{{ row.email }}</span>
            <p v-if="!row.current" class="text-xs text-amber-600">已更换为其他邮箱</p>
          </template>
          <template #cell-source="{ row }">
            <span class="badge badge-gray">{{ SOURCE_LABEL[row.source as VerifySource] ?? row.source }}</span>
          </template>
          <template #cell-reward_points="{ row }">
            <span class="text-sm tabular-nums" :class="row.reward_points ? 'text-primary-600' : 'text-gray-400'">{{ row.reward_points || '—' }}</span>
          </template>
          <template #cell-verified_at="{ value }"><span class="text-sm text-gray-500">{{ formatDateTimeToMinute(value) }}</span></template>
          <template #empty><EmptyState title="还没有验证记录" description="用户注册、绑定邮箱、站内验证或点邀请链接验证后会出现在这里" /></template>
        </DataTable>
        <div class="px-4 py-3">
          <Pagination v-if="recTotal > 0" :page="page" :total="recTotal" :page-size="pageSize" @update:page="(p: number) => (page = p)" @update:pageSize="(s: number) => { pageSize = s; page = 1 }" />
        </div>
      </section>

      <!-- 设置 -->
      <template v-else>
        <div v-if="!cfg" class="card p-16 text-center text-sm text-gray-400">{{ error || '正在加载…' }}</div>
        <section v-else class="card max-w-3xl">
          <div class="divide-y divide-gray-100 dark:divide-dark-700">
            <label v-for="f in fields" :key="f.key" class="flex items-center justify-between gap-4 px-6 py-4">
              <span>
                <span class="block text-sm text-gray-900 dark:text-white">{{ f.label }}</span>
                <span class="block text-xs text-gray-500">{{ f.hint }}</span>
              </span>
              <span class="flex shrink-0 items-center gap-2">
                <input v-model.number="cfg[f.key]" type="number" :min="f.min" class="input w-28 text-right" />
                <span class="w-8 text-xs text-gray-500">{{ f.unit }}</span>
              </span>
            </label>
          </div>
          <div class="flex items-center gap-3 px-6 py-4">
            <button type="button" class="btn btn-primary" :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存' }}</button>
            <span class="text-xs text-gray-500">保存后立即生效。</span>
          </div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTimeToMinute } from '@/utils/format'
import {
  adminVerifyApi, SOURCE_LABEL,
  type AdminOverviewStats, type VerifyConfig, type VerifyRecord, type VerifySource
} from '../api'

const StatCard = defineComponent({
  props: { label: { type: String, required: true }, value: { type: Number, required: true }, tone: { type: String, default: 'text-gray-900 dark:text-white' }, sub: { type: String, default: '' } },
  setup: (p) => () =>
    h('div', { class: 'card p-5' }, [
      h('p', { class: 'text-xs font-medium text-gray-500 dark:text-dark-400' }, p.label),
      h('p', { class: ['mt-2 text-3xl font-bold tabular-nums', p.tone] }, String(p.value)),
      p.sub ? h('p', { class: 'mt-1 text-xs text-gray-400' }, p.sub) : null
    ])
})

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const error = ref('')

// ─── 标签页（?tab= 保持刷新后位置）────────────────────────────────────────
const tabs = [
  { key: 'overview', label: '概览' },
  { key: 'records', label: '验证记录' },
  { key: 'settings', label: '设置' }
] as const
type TabKey = (typeof tabs)[number]['key']
const tab = computed<TabKey>(() => {
  const q = route.query.tab
  return tabs.some((t) => t.key === q) ? (q as TabKey) : 'overview'
})
function setTab(key: TabKey) {
  if (key !== tab.value) void router.replace({ query: { ...route.query, tab: key } })
}

// ─── 概览 ────────────────────────────────────────────────────────────────
const stats = ref<AdminOverviewStats | null>(null)
const coverage = computed(() => {
  const s = stats.value
  if (!s) return '—'
  const total = s.verified + s.pending + s.invalid
  return total > 0 ? `${((s.verified / total) * 100).toFixed(1)}%` : '—'
})
function invite(preset: string) {
  router.push({ path: '/admin/outreach/campaigns/new', query: { preset } })
}

// ─── 验证记录 ────────────────────────────────────────────────────────────
const records = ref<VerifyRecord[]>([])
const recTotal = ref(0)
const recLoading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const q = ref('')
const source = ref('')
const sourceOptions = [{ value: '', label: '全部途径' }, ...Object.entries(SOURCE_LABEL).map(([value, label]) => ({ value, label }))]
const columns: Column[] = [
  { key: 'user', label: '用户' },
  { key: 'email', label: '验证的邮箱' },
  { key: 'source', label: '途径' },
  { key: 'reward_points', label: '奖励积分' },
  { key: 'verified_at', label: '验证时间' }
]
async function loadRecords() {
  recLoading.value = true
  try {
    const r = await adminVerifyApi.records({ page: page.value, page_size: pageSize.value, q: q.value.trim(), source: source.value })
    records.value = r.items
    recTotal.value = r.total
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载验证记录失败'))
  } finally {
    recLoading.value = false
  }
}
function reload() {
  if (page.value === 1) loadRecords()
  else page.value = 1
}
watch([page, pageSize], loadRecords)
watch(source, reload)

// ─── 设置 ────────────────────────────────────────────────────────────────
const cfg = ref<VerifyConfig | null>(null)
const version = ref(0)
const saving = ref(false)
const fields: { key: keyof VerifyConfig; label: string; hint: string; unit: string; min: number }[] = [
  { key: 'reward_points', label: '验证奖励', hint: '充值过的老用户首次验证成功发放，每人一次；0 = 关闭', unit: '积分', min: 0 },
  { key: 'invite_ttl_days', label: '邀请链接有效期', hint: '邀请邮件里一次性验证链接的有效期；过期后用户仍可在站内用验证码验证', unit: '天', min: 1 }
]
async function save() {
  if (!cfg.value) return
  saving.value = true
  try {
    version.value = (await adminVerifyApi.saveSettings(cfg.value, version.value)).version
    appStore.showSuccess('已保存')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

// 每个标签页首次进入时加载
async function loadTab(t: TabKey) {
  error.value = ''
  try {
    if (t === 'overview') stats.value = await adminVerifyApi.overview()
    else if (t === 'records') await loadRecords()
    else {
      const r = await adminVerifyApi.getSettings()
      cfg.value = r.config
      version.value = r.version
    }
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}
watch(tab, loadTab, { immediate: true })
</script>
