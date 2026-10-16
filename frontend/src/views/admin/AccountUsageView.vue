<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header + filters -->
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-4">
          <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">账号用量对账</h2>
          <div class="ml-auto flex flex-wrap items-center gap-3">
            <DateRangePicker
              v-model:start-date="startDate"
              v-model:end-date="endDate"
              @change="loadAll"
            />
            <div class="w-28">
              <Select v-model="granularity" :options="granularityOptions" @change="loadTrend" />
            </div>
          </div>
        </div>
      </div>

      <!-- Summary stat tiles -->
      <div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <div v-for="tile in statTiles" :key="tile.label" class="card p-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ tile.label }}</p>
          <p class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-gray-100">{{ tile.value }}</p>
          <p v-if="tile.note" class="mt-0.5 text-xs text-amber-600 dark:text-amber-400">{{ tile.note }}</p>
        </div>
      </div>

      <!-- Trend chart -->
      <div class="card p-4">
        <h3 class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">账号用量趋势（按成本）</h3>
        <div v-if="trendLoading" class="flex h-48 items-center justify-center text-sm text-gray-400">加载中…</div>
        <div v-else-if="trendError" class="flex h-48 items-center justify-center text-sm text-red-500">{{ trendError }}</div>
        <div v-else-if="trendLabels.length === 0" class="flex h-48 items-center justify-center text-sm text-gray-400">暂无数据</div>
        <Line v-else :data="trendChartData" :options="trendChartOptions" class="max-h-64" />
      </div>

      <!-- Summary table -->
      <div class="card overflow-hidden">
        <div class="border-b border-gray-200 px-4 py-3 dark:border-dark-700">
          <h3 class="text-sm font-medium text-gray-700 dark:text-gray-300">账号消耗汇总</h3>
        </div>
        <div v-if="summaryLoading" class="flex h-32 items-center justify-center text-sm text-gray-400">加载中…</div>
        <div v-else-if="summaryError" class="flex h-32 items-center justify-center text-sm text-red-500">{{ summaryError }}</div>
        <div v-else-if="summaryItems.length === 0" class="flex h-32 items-center justify-center text-sm text-gray-400">暂无数据</div>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 bg-gray-50 text-left text-xs font-medium text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
                <th class="px-4 py-2">账号</th>
                <th class="px-4 py-2">平台</th>
                <th class="px-4 py-2">倍率来源</th>
                <th class="px-4 py-2 text-right">请求数</th>
                <th class="px-4 py-2 text-right">总 Token</th>
                <th class="px-4 py-2 text-right">标准（原价）</th>
                <th class="px-4 py-2 text-right">成本（上游扣费）</th>
                <th class="px-4 py-2 text-right">实际（用户扣费）</th>
                <th class="px-4 py-2 text-center">模型明细</th>
              </tr>
            </thead>
            <tbody>
              <template v-for="item in summaryItems" :key="item.account_id">
                <tr class="border-b border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800">
                  <td class="px-4 py-2 font-medium text-gray-900 dark:text-gray-100">{{ item.account_name || `#${item.account_id}` }}</td>
                  <td class="px-4 py-2 text-gray-500 dark:text-gray-400">{{ item.platform }}</td>
                  <td class="px-4 py-2">
                    <RateSourceBadge :item="item" />
                  </td>
                  <td class="px-4 py-2 text-right tabular-nums">{{ fmtNum(item.requests) }}</td>
                  <td class="px-4 py-2 text-right tabular-nums">{{ fmtNum(item.total_tokens) }}</td>
                  <td class="px-4 py-2 text-right tabular-nums text-gray-400 dark:text-gray-500">{{ fmtCost(item.standard_cost) }}</td>
                  <td class="px-4 py-2 text-right tabular-nums font-medium text-gray-900 dark:text-gray-100">{{ fmtCost(item.upstream_cost) }}</td>
                  <td class="px-4 py-2 text-right tabular-nums">{{ fmtCost(item.actual_cost) }}</td>
                  <td class="px-4 py-2 text-center">
                    <button
                      type="button"
                      class="text-xs text-primary-600 hover:underline dark:text-primary-400"
                      @click="toggleModelDetail(item.account_id)"
                    >{{ expandedAccountId === item.account_id ? '收起' : '展开' }}</button>
                  </td>
                </tr>
                <!-- rate mismatch warning row -->
                <tr v-if="item.rate_mismatch && expandedAccountId !== item.account_id" :key="`mismatch-${item.account_id}`">
                  <td colspan="9" class="border-b border-amber-100 bg-amber-50 px-4 py-1.5 text-xs text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-400">
                    账号倍率 {{ fmtRate(item.account_rate) }} ≠ 上游声明 {{ fmtRate(item.upstream_rate) }}，账号额度统计会有偏差 —
                    <a href="/admin/accounts" class="underline">去账号管理开启同步</a>
                  </td>
                </tr>
                <!-- model detail expand -->
                <tr v-if="expandedAccountId === item.account_id" :key="`detail-${item.account_id}`">
                  <td colspan="9" class="bg-gray-50 px-6 py-3 dark:bg-dark-800">
                    <div v-if="modelDetailLoading" class="text-xs text-gray-400">加载中…</div>
                    <div v-else-if="modelDetailError" class="text-xs text-red-500">{{ modelDetailError }}</div>
                    <div v-else-if="modelDetailRows.length === 0" class="text-xs text-gray-400">暂无模型数据</div>
                    <table v-else class="w-full text-xs">
                      <thead>
                        <tr class="text-left text-gray-400">
                          <th class="py-1 pr-4">模型</th>
                          <th class="py-1 pr-4 text-right">请求</th>
                          <th class="py-1 pr-4 text-right">Input</th>
                          <th class="py-1 pr-4 text-right">Output</th>
                          <th class="py-1 pr-4 text-right">总 Token</th>
                          <th class="py-1 pr-4 text-right">标准</th>
                          <th class="py-1 pr-4 text-right">成本</th>
                          <th class="py-1 text-right">实际</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="row in modelDetailRows" :key="row.model" class="border-t border-gray-100 dark:border-dark-700">
                          <td class="py-1 pr-4 font-mono text-gray-700 dark:text-gray-300">{{ row.model }}</td>
                          <td class="py-1 pr-4 text-right tabular-nums">{{ fmtNum(row.requests) }}</td>
                          <td class="py-1 pr-4 text-right tabular-nums">{{ fmtNum(row.input_tokens) }}</td>
                          <td class="py-1 pr-4 text-right tabular-nums">{{ fmtNum(row.output_tokens) }}</td>
                          <td class="py-1 pr-4 text-right tabular-nums">{{ fmtNum(row.total_tokens) }}</td>
                          <td class="py-1 pr-4 text-right tabular-nums text-gray-400">{{ fmtCost(row.standard_cost) }}</td>
                          <td class="py-1 pr-4 text-right tabular-nums font-medium">{{ fmtCost(row.upstream_cost) }}</td>
                          <td class="py-1 text-right tabular-nums">{{ fmtCost(row.actual_cost) }}</td>
                        </tr>
                      </tbody>
                    </table>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Line } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
} from 'chart.js'
import AppLayout from '@/components/layout/AppLayout.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import {
  getAccountSummary,
  getAccountTrend,
  getAccountModelStats,
  type AccountSummaryItem,
  type AccountUsageTrendPoint,
  type AccountModelStat,
} from '@/api/admin/accountUsage'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)

// ---------- date helpers ----------
const fmtDate = (d: Date) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
const today = new Date()
const startDate = ref(fmtDate(new Date(today.getFullYear(), today.getMonth(), 1)))
const endDate = ref(fmtDate(today))
const granularity = ref('day')
const granularityOptions = [
  { label: '按天', value: 'day' },
  { label: '按小时', value: 'hour' },
]

// ---------- summary ----------
const summaryLoading = ref(false)
const summaryError = ref('')
const summaryItems = ref<AccountSummaryItem[]>([])
const totalStandardCost = ref(0)
const totalUpstreamCost = ref(0)
const totalActualCost = ref(0)
const totalEstimatedCost = ref(0)
const totalRequests = ref(0)
const totalTokens = ref(0)

const statTiles = computed(() => [
  { label: '总请求数', value: fmtNum(totalRequests.value) },
  { label: '总 Token', value: fmtNum(totalTokens.value) },
  {
    label: '成本（上游扣费）',
    value: fmtCost(totalUpstreamCost.value),
    note: totalEstimatedCost.value > 0 ? `其中 ${fmtCost(totalEstimatedCost.value)} 按账号倍率估算` : undefined,
  },
  { label: '实际（用户扣费）', value: fmtCost(totalActualCost.value) },
])

async function loadSummary() {
  summaryLoading.value = true
  summaryError.value = ''
  try {
    const res = await getAccountSummary({
      start_date: startDate.value,
      end_date: endDate.value,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    })
    summaryItems.value = res.items ?? []
    totalStandardCost.value = res.total_standard_cost
    totalUpstreamCost.value = res.total_upstream_cost
    totalActualCost.value = res.total_actual_cost
    totalEstimatedCost.value = res.total_estimated_cost
    totalRequests.value = res.total_requests
    totalTokens.value = res.total_tokens
  } catch (e: unknown) {
    summaryError.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    summaryLoading.value = false
  }
}

// ---------- trend ----------
const trendLoading = ref(false)
const trendError = ref('')
const trendPoints = ref<AccountUsageTrendPoint[]>([])

const PALETTE = [
  '#3b82f6', '#f59e0b', '#10b981', '#ef4444', '#8b5cf6',
  '#06b6d4', '#f97316', '#84cc16', '#ec4899', '#6b7280',
]

const trendLabels = computed(() => Array.from(new Set(trendPoints.value.map(p => p.date))).sort())

const trendChartData = computed(() => {
  const accounts = new Map<number, { name: string; byDate: Map<string, number> }>()
  for (const p of trendPoints.value) {
    if (!accounts.has(p.account_id))
      accounts.set(p.account_id, { name: p.account_name || `#${p.account_id}`, byDate: new Map() })
    accounts.get(p.account_id)!.byDate.set(p.date, p.upstream_cost)
  }
  const datasets = Array.from(accounts.entries()).map(([, acc], i) => ({
    label: acc.name,
    data: trendLabels.value.map(d => acc.byDate.get(d) ?? 0),
    borderColor: PALETTE[i % PALETTE.length],
    backgroundColor: PALETTE[i % PALETTE.length] + '22',
    tension: 0.3,
    pointRadius: 3,
    borderWidth: 2,
  }))
  return { labels: trendLabels.value, datasets }
})

const trendChartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { position: 'bottom' as const, labels: { boxWidth: 12, font: { size: 11 } } },
    tooltip: { mode: 'index' as const, intersect: false },
  },
  scales: {
    y: { beginAtZero: true, ticks: { font: { size: 11 } } },
    x: { ticks: { font: { size: 11 } } },
  },
}

async function loadTrend() {
  trendLoading.value = true
  trendError.value = ''
  try {
    const res = await getAccountTrend({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      limit: 10,
    })
    trendPoints.value = res ?? []
  } catch (e: unknown) {
    trendError.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    trendLoading.value = false
  }
}

// ---------- model detail ----------
const expandedAccountId = ref<number | null>(null)
const modelDetailLoading = ref(false)
const modelDetailError = ref('')
const modelDetailRows = ref<AccountModelStat[]>([])

async function toggleModelDetail(accountId: number) {
  if (expandedAccountId.value === accountId) {
    expandedAccountId.value = null
    return
  }
  expandedAccountId.value = accountId
  modelDetailLoading.value = true
  modelDetailError.value = ''
  modelDetailRows.value = []
  try {
    const res = await getAccountModelStats({
      account_id: accountId,
      start_date: startDate.value,
      end_date: endDate.value,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    })
    modelDetailRows.value = res ?? []
  } catch (e: unknown) {
    modelDetailError.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    modelDetailLoading.value = false
  }
}

function loadAll() {
  expandedAccountId.value = null
  loadSummary()
  loadTrend()
}

// ---------- formatters ----------
function fmtNum(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(n)
}
function fmtCost(n: number): string { return '$' + n.toFixed(4) }
function fmtRate(r: number | null | undefined): string {
  return r != null ? r.toFixed(4) + 'x' : '—'
}

onMounted(loadAll)
</script>

<!-- RateSourceBadge: inline component for rate source indicator -->
<script lang="ts">
import { defineComponent, h } from 'vue'
import type { AccountSummaryItem as ASI, AccountRateSource as ARS } from '@/api/admin/accountUsage'

export const RateSourceBadge = defineComponent({
  name: 'RateSourceBadge',
  props: { item: { type: Object as () => ASI, required: true } },
  setup(props) {
    return () => {
      const item = props.item
      const src: ARS = item.rate_source
      const rate = item.upstream_rate != null ? item.upstream_rate.toFixed(4) + 'x' : null

      const badges: Record<ARS, { cls: string; text: string }> = {
        upstream: { cls: 'text-emerald-700 dark:text-emerald-400', text: `上游 ${rate}` },
        upstream_stale: { cls: 'text-amber-600 dark:text-amber-400', text: `上游 ${rate}（已过期）` },
        estimated_probe_off: { cls: 'text-amber-600 dark:text-amber-400', text: '估算（探测未开启）' },
        estimated_probe_pending: { cls: 'text-amber-600 dark:text-amber-400', text: '估算（探测中）' },
        estimated_unsupported: { cls: 'text-gray-400', text: '估算（不支持）' },
      }
      const b = badges[src] ?? { cls: 'text-gray-400', text: src }
      return h('span', { class: `text-xs ${b.cls}` }, b.text)
    }
  },
})
</script>
