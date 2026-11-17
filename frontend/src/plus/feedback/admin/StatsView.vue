<!-- [plus] 管理端「反馈管理 · 看板」：场景、严重度、趋势、建议池、用户 -->
<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="flex justify-end">
        <Select :model-value="days" :options="rangeOptions" class="w-32" :disabled="loading" data-testid="stats-range" @update:model-value="(v) => pick(Number(v))" />
      </div>

      <div v-if="!st" class="card">
        <EmptyState v-if="error" title="加载失败" :description="error" action-text="重试" :action-icon="false" @action="load" />
        <div v-else class="flex justify-center py-16"><LoadingSpinner /></div>
      </div>

      <div v-else class="space-y-4 transition-opacity" :class="loading && 'pointer-events-none opacity-60'" :aria-busy="loading">
        <!-- 概览 -->
        <section class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
          <Tile label="新增" :value="o.total" :sub="`问题 ${o.issues} · 建议 ${o.suggestions}`" />
          <Tile label="待处理" :value="o.pending" :sub="o.awaiting ? `已回复 ${o.awaiting}` : ''" :to="{ path: '/admin/feedback' }" accent />
          <Tile label="已解决" :value="o.resolved" />
          <Tile label="已采纳" :value="o.adopted" />
          <Tile label="平均处理" :value="o.avg_close_hours ? formatHours(o.avg_close_hours) : '—'" :sub="o.auto_closed ? `超时关闭 ${o.auto_closed}` : ''" />
          <Tile label="发放积分" :value="o.reward_points.toLocaleString()" />
        </section>

        <!-- 场景 -->
        <section class="grid gap-4 lg:grid-cols-2">
          <Panel v-for="k in (['issue', 'suggestion'] as const)" :key="k" :title="k === 'issue' ? '问题集中在哪' : '建议集中在哪'">
            <p v-if="!byKind[k].length" class="py-6 text-center text-sm text-gray-400">暂无</p>
            <router-link
              v-for="s in byKind[k]"
              :key="s.scenario"
              :to="drill({ kind: k, scenario: s.scenario })"
              class="group block py-2"
            >
              <div class="mb-1 flex items-center gap-2 text-sm">
                <span>{{ scenarioIcon(s.scenario) }}</span>
                <span class="flex-1 truncate text-gray-700 group-hover:text-primary-600 dark:text-dark-100">{{ scenarioLabel(s.scenario) }}</span>
                <span v-if="s.reclassified" class="text-[11px] text-amber-500" title="被管理员改过场景的条数">改场景 {{ s.reclassified }}</span>
                <span class="w-8 text-right font-semibold tabular-nums">{{ s.total }}</span>
              </div>
              <div class="flex h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" :style="{ width: `${Math.max(6, (s.total / maxScenario) * 100)}%` }">
                <span class="bg-emerald-400" :style="{ width: pct(s.accepted, s.total) }" />
                <span class="bg-amber-400" :style="{ width: pct(s.open, s.total) }" />
                <span class="bg-gray-300 dark:bg-dark-500" :style="{ width: pct(s.declined, s.total) }" />
              </div>
            </router-link>
            <Legend :items="[['bg-emerald-400', k === 'issue' ? '已解决' : '已采纳'], ['bg-amber-400', '处理中'], ['bg-gray-300', '不处理']]" />
          </Panel>
        </section>

        <!-- 严重度 × 场景 -->
        <Panel title="哪里最痛">
          <div v-if="!heatRows.length" class="py-6 text-center text-sm text-gray-400">暂无</div>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr class="text-xs text-gray-400">
                  <th class="py-2 text-left font-normal">场景</th>
                  <th v-for="c in heatCols" :key="c.key" class="w-20 py-2 font-normal">{{ c.label }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in heatRows" :key="r.scenario">
                  <td class="py-1 pr-3 text-gray-700 dark:text-dark-100">{{ scenarioIcon(r.scenario) }} {{ scenarioLabel(r.scenario) }}</td>
                  <td v-for="c in heatCols" :key="c.key" class="p-1">
                    <router-link
                      v-if="r.cells[c.key]?.count"
                      :to="drill({ scenario: r.scenario, severity: c.key || 'none' })"
                      class="flex h-9 items-center justify-center rounded-lg text-xs font-semibold tabular-nums text-white transition-transform hover:scale-105"
                      :class="c.cell"
                      :style="{ opacity: 0.35 + 0.65 * (r.cells[c.key].count / maxHeat) }"
                      :title="`处理中 ${r.cells[c.key].open}`"
                    >
                      {{ r.cells[c.key].count }}
                    </router-link>
                    <div v-else class="h-9 rounded-lg bg-gray-50 dark:bg-dark-900/50" />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </Panel>

        <!-- 趋势 -->
        <Panel title="每日趋势">
          <p v-if="trendEmpty" class="py-6 text-center text-sm text-gray-400">暂无</p>
          <template v-else>
            <div class="h-40 pt-2">
              <Chart type="bar" :data="trendData" :options="trendOptions" />
            </div>
            <Legend :items="[['bg-sky-400', '问题'], ['bg-violet-400', '建议'], ['bg-emerald-500', '结案']]" />
          </template>
        </Panel>

        <section class="grid gap-4 lg:grid-cols-2">
          <!-- 建议池 -->
          <Panel title="建议池">
            <p v-if="!st.pool.length" class="py-6 text-center text-sm text-gray-400">暂无</p>
            <router-link v-for="p in st.pool" :key="p.id" :to="`/admin/feedback/${p.id}`" class="flex items-center gap-3 py-2.5 text-sm hover:text-primary-600">
              <span class="min-w-0 flex-1">
                <span class="block truncate text-gray-800 dark:text-dark-100">{{ p.summary }}</span>
                <span class="text-xs text-gray-400">{{ scenarioLabel(p.scenario) }}</span>
              </span>
              <span class="shrink-0 text-xs text-gray-400" title="影响人数">
                影响 <span class="text-sm font-semibold tabular-nums text-primary-600 dark:text-primary-400">{{ p.impact }}</span>
              </span>
              <span v-if="p.severity" class="h-2 w-2 shrink-0 rounded-full" :class="SEVERITY_META[p.severity].dot" :title="SEVERITY_META[p.severity].label" />
              <StatusPill :status="p.status" />
            </router-link>
          </Panel>

          <!-- 用户 -->
          <Panel title="谁在反馈">
            <p v-if="!st.users.length" class="py-6 text-center text-sm text-gray-400">暂无</p>
            <router-link
              v-for="u in st.users.slice(0, 15)"
              :key="u.user_id"
              :to="drill({ user: u.email })"
              class="block py-2.5 text-sm hover:text-primary-600"
            >
              <div class="flex items-center gap-2">
                <span class="min-w-0 flex-1 truncate text-gray-800 dark:text-dark-100">{{ u.email }}</span>
                <span class="text-xs text-gray-400">问题 {{ u.issues }} · 建议 {{ u.suggestions }}</span>
                <span class="w-16 text-right text-xs tabular-nums" :class="rate(u) >= 50 ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'" title="已解决或已采纳的占比">有效 {{ rate(u) }}%</span>
                <span v-if="u.reward" class="w-12 text-right text-xs font-semibold text-violet-600 dark:text-violet-300">+{{ u.reward }}</span>
              </div>
              <!-- 这个人的反馈分布在哪些场景：色条看比例，下面直接写出前三个场景，不用悬停 -->
              <div class="mt-1.5 flex h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" data-testid="user-mix">
                <span v-for="m in mixOf(u)" :key="m.key" :style="{ width: pct(m.n, u.total), background: m.color }" />
              </div>
              <p class="mt-1 truncate text-[11px] text-gray-400 dark:text-dark-400">
                <span v-for="(m, i) in mixOf(u).slice(0, 3)" :key="m.key">
                  <template v-if="i">　</template><span class="mr-1 inline-block h-1.5 w-1.5 rounded-full align-middle" :style="{ background: m.color }" />{{ scenarioLabel(m.key) }} {{ m.n }}
                </span>
                <template v-if="mixOf(u).length > 3">　等 {{ mixOf(u).length }} 个场景</template>
              </p>
            </router-link>
          </Panel>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter, type RouteLocationRaw } from 'vue-router'
import {
  Chart as ChartJS,
  BarController,
  BarElement,
  CategoryScale,
  LineController,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
  type ChartData,
  type ChartOptions
} from 'chart.js'
import { Chart } from 'vue-chartjs'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminApi, type Kind, type Stats } from '../api'
import { SEVERITIES, SEVERITY_META } from '../format'
import { FEEDBACK_ADMIN_PATH } from '../routes'
import { allScenarios, loadAdminScenarios, scenarioIcon, scenarioLabel } from './shared'
import StatusPill from '../components/StatusPill.vue'

ChartJS.register(BarController, BarElement, LineController, LineElement, PointElement, CategoryScale, LinearScale, Tooltip)

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const here = route.name
const RANGES = [7, 30, 90]
const rangeOptions = RANGES.map((d) => ({ value: d, label: `${d} 天` }))
const fromUrl = Number(route.query.days)
/** 选择器上的范围 */
const days = ref(RANGES.includes(fromUrl) ? fromUrl : 30)
/** 当前显示的数据对应的范围（加载失败时选择器退回到它） */
const shownDays = ref(days.value)
const st = ref<Stats | null>(null)
const error = ref('')
const loading = ref(false)
const o = computed(() => st.value!.overview)

// 只采用最后一次请求的结果，快速切换范围时旧响应不会覆盖新的
let seq = 0
async function load() {
  const my = ++seq
  loading.value = true
  error.value = ''
  try {
    const r = await adminApi.stats(days.value)
    if (my !== seq) return
    st.value = r
    shownDays.value = days.value
    if (route.name === here && String(route.query.days ?? 30) !== String(days.value)) void router.replace({ query: { ...route.query, days: String(days.value) } })
  } catch (e) {
    if (my !== seq) return
    const msg = extractApiErrorMessage(e, '加载失败')
    if (st.value) {
      appStore.showError(msg)
      days.value = shownDays.value
    } else {
      error.value = msg
    }
  } finally {
    if (my === seq) loading.value = false
  }
}

function pick(v: number) {
  if (!RANGES.includes(v) || v === days.value) return
  days.value = v
  void load()
}

// 下钻到反馈处理：除待处理和建议池外的数字都只算所选范围内提交的，列表带上同样的范围
const drill = (query: Record<string, string>): RouteLocationRaw => ({
  path: FEEDBACK_ADMIN_PATH,
  query: { status: 'all', ...query, days: String(shownDays.value) }
})

const byKind = computed(() => {
  const out: Record<Kind, Stats['scenarios']> = { issue: [], suggestion: [] }
  for (const s of st.value?.scenarios ?? []) out[s.kind].push(s)
  return out
})
const maxScenario = computed(() => Math.max(1, ...(st.value?.scenarios ?? []).map((s) => s.total)))

const heatCols = [...SEVERITIES.map((s) => ({ key: s as string, label: SEVERITY_META[s].label, cell: SEVERITY_META[s].cell })), { key: '', label: '未定级', cell: 'bg-slate-400' }]
const heatRows = computed(() => {
  const rows = new Map<string, { scenario: string; total: number; cells: Record<string, { count: number; open: number }> }>()
  for (const c of st.value?.severity ?? []) {
    const r = rows.get(c.scenario) ?? { scenario: c.scenario, total: 0, cells: {} }
    r.cells[c.severity] = { count: c.count, open: c.open }
    r.total += c.count
    rows.set(c.scenario, r)
  }
  // 按严重问题数排序，再按总数
  return [...rows.values()].sort((a, b) => (b.cells.critical?.count ?? 0) - (a.cells.critical?.count ?? 0) || b.total - a.total)
})
const maxHeat = computed(() => Math.max(1, ...(st.value?.severity ?? []).map((c) => c.count)))

// 趋势：问题/建议堆叠柱 + 结案折线（颜色与下方图例一致：sky-400 / violet-400 / emerald-500）
const trend = computed(() => st.value?.trend ?? [])
const trendEmpty = computed(() => !trend.value.some((p) => p.issues || p.suggestions || p.closed))
const trendData = computed(
  () =>
    ({
      labels: trend.value.map((p) => p.date.slice(5)),
      datasets: [
        { type: 'bar', label: '问题', data: trend.value.map((p) => p.issues), backgroundColor: '#38bdf8', stack: 'new', order: 2 },
        { type: 'bar', label: '建议', data: trend.value.map((p) => p.suggestions), backgroundColor: '#a78bfa', stack: 'new', order: 2 },
        {
          type: 'line',
          label: '结案',
          data: trend.value.map((p) => p.closed),
          borderColor: '#10b981',
          backgroundColor: '#10b981',
          borderWidth: 2,
          pointRadius: 0,
          pointHoverRadius: 3,
          tension: 0.3,
          stack: 'closed',
          order: 1
        }
      ]
    }) as ChartData
)
const trendOptions = computed((): ChartOptions => {
  const dark = document.documentElement.classList.contains('dark')
  const text = dark ? '#9ca3af' : '#6b7280'
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { mode: 'index', intersect: false },
    plugins: { legend: { display: false }, tooltip: { mode: 'index', intersect: false } },
    scales: {
      x: { stacked: true, grid: { display: false }, ticks: { color: text, font: { size: 10 }, maxRotation: 0, autoSkip: true, maxTicksLimit: 8 } },
      y: { stacked: true, beginAtZero: true, grid: { color: dark ? '#374151' : '#f3f4f6' }, ticks: { color: text, font: { size: 10 }, precision: 0 } }
    }
  }
})

const pct = (n: number, total: number) => `${total ? (n / total) * 100 : 0}%`

// 场景配色：按场景在配置里的顺序取色，同一场景在各处颜色一致
const PALETTE = ['#38bdf8', '#a78bfa', '#34d399', '#fbbf24', '#f87171', '#f472b6', '#60a5fa', '#2dd4bf', '#fb923c', '#a3e635', '#c084fc', '#94a3b8']
const colorOf = (key: string) => {
  const i = allScenarios.value.findIndex((s) => s.key === key)
  return PALETTE[(i < 0 ? PALETTE.length - 1 : i) % PALETTE.length]
}
/** 某个用户的场景分布，按条数从多到少 */
const mixOf = (u: Stats['users'][number]) =>
  Object.entries(u.scenarios ?? {})
    .map(([key, n]) => ({ key, n, color: colorOf(key) }))
    .sort((a, b) => b.n - a.n)
const rate = (u: Stats['users'][number]) => (u.total ? Math.round((u.accepted / u.total) * 100) : 0)
const formatHours = (h: number) => {
  const min = h * 60
  if (min < 1) return '<1 分'
  if (h < 1) return `${Math.round(min)} 分`
  return h < 48 ? `${h.toFixed(1)} 时` : `${(h / 24).toFixed(1)} 天`
}

const Tile = defineComponent({
  props: { label: { type: String, required: true }, value: { type: [String, Number], required: true }, sub: { type: String, default: '' }, to: { type: Object as () => RouteLocationRaw, default: undefined }, accent: Boolean },
  setup(p) {
    const base = 'card block px-5 py-4'
    const body = () => [
      h('div', { class: 'text-sm text-gray-500 dark:text-dark-400' }, p.label),
      h('div', { class: ['mt-1 text-2xl font-semibold tabular-nums', p.accent && Number(p.value) > 0 ? 'text-amber-500' : 'text-gray-900 dark:text-white'] }, String(p.value)),
      h('div', { class: 'mt-0.5 h-4 truncate text-[11px] text-gray-400' }, p.sub)
    ]
    return () => (p.to ? h(RouterLink, { to: p.to, class: base + ' card-hover' }, body) : h('div', { class: base }, body()))
  }
})

const Panel = defineComponent({
  props: { title: { type: String, required: true } },
  setup(p, { slots }) {
    return () =>
      h('section', { class: 'card overflow-hidden' }, [
        h('h3', { class: 'card-header text-sm font-semibold text-gray-900 dark:text-white' }, p.title),
        h('div', { class: 'divide-y divide-gray-100 px-6 py-2 dark:divide-dark-700/60' }, slots.default?.())
      ])
  }
})

const Legend = defineComponent({
  props: { items: { type: Array as () => [string, string][], required: true } },
  setup(p) {
    return () =>
      h(
        'div',
        { class: 'flex gap-4 border-0 pt-3 text-[11px] text-gray-400' },
        p.items.map(([c, l]) => h('span', { class: 'flex items-center gap-1.5' }, [h('span', { class: ['h-2 w-2 rounded-full', c] }), l]))
      )
  }
})

onMounted(() => {
  void loadAdminScenarios()
  void load()
})
// 离开页面后，还在路上的请求结果不再处理（避免改写别的页面的 URL）
onBeforeUnmount(() => seq++)
</script>
