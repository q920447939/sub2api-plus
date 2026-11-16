<!-- [plus] 管理端「反馈管理 · 看板」：场景、严重度、趋势、建议池、用户 -->
<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="flex justify-end">
        <Select v-model="days" :options="rangeOptions" class="w-32" />
      </div>

      <div v-if="!st" class="card p-16 text-center text-sm text-gray-400">{{ error || '正在加载…' }}</div>

      <template v-else>
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
              :to="{ path: '/admin/feedback', query: { status: '', kind: k, scenario: s.scenario } }"
              class="group block py-2"
            >
              <div class="mb-1 flex items-center gap-2 text-sm">
                <span>{{ scenarioIcon(s.scenario) }}</span>
                <span class="flex-1 truncate text-gray-700 group-hover:text-primary-600 dark:text-dark-100">{{ scenarioLabel(s.scenario) }}</span>
                <span v-if="s.reclassified" class="text-[11px] text-amber-500" title="被管理员改过场景的条数">改 {{ s.reclassified }}</span>
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
                      :to="{ path: '/admin/feedback', query: { status: '', scenario: r.scenario, severity: c.key || 'none' } }"
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
          <div class="flex h-40 items-end gap-[3px]">
            <div v-for="p in st.trend" :key="p.date" class="group relative flex h-full flex-1 flex-col justify-end">
              <div class="pointer-events-none absolute -top-1 left-1/2 z-10 hidden -translate-x-1/2 -translate-y-full whitespace-nowrap rounded-lg bg-gray-900 px-2 py-1 text-[11px] text-white group-hover:block">
                {{ p.date.slice(5) }} · 问题 {{ p.issues }} · 建议 {{ p.suggestions }} · 结案 {{ p.closed }}
              </div>
              <div class="w-full rounded-t-sm bg-violet-400" :style="{ height: barH(p.suggestions) }" />
              <div class="w-full bg-sky-400" :class="p.suggestions ? '' : 'rounded-t-sm'" :style="{ height: barH(p.issues) }" />
              <div class="absolute bottom-0 left-0 right-0 border-t-2 border-emerald-500" :style="{ bottom: barH(p.closed) }" :class="p.closed ? '' : 'hidden'" />
            </div>
          </div>
          <div class="mt-2 flex justify-between text-[11px] text-gray-400">
            <span>{{ st.trend[0]?.date.slice(5) }}</span>
            <span>{{ st.trend[st.trend.length - 1]?.date.slice(5) }}</span>
          </div>
          <Legend :items="[['bg-sky-400', '问题'], ['bg-violet-400', '建议'], ['bg-emerald-500', '结案']]" />
        </Panel>

        <section class="grid gap-4 lg:grid-cols-2">
          <!-- 建议池 -->
          <Panel title="建议池">
            <p v-if="!st.pool.length" class="py-6 text-center text-sm text-gray-400">暂无</p>
            <router-link v-for="p in st.pool" :key="p.id" :to="`/admin/feedback/${p.id}`" class="flex items-center gap-3 py-2.5 text-sm hover:text-primary-600">
              <span class="w-8 shrink-0 text-center text-base font-semibold tabular-nums text-primary-600 dark:text-primary-400" title="影响人数">{{ p.impact }}</span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-gray-800 dark:text-dark-100">{{ p.summary }}</span>
                <span class="text-xs text-gray-400">{{ scenarioLabel(p.scenario) }}</span>
              </span>
              <span v-if="p.severity" class="h-2 w-2 shrink-0 rounded-full" :class="SEVERITY_META[p.severity].dot" />
              <StatusPill :status="p.status" />
            </router-link>
          </Panel>

          <!-- 用户 -->
          <Panel title="谁在反馈">
            <p v-if="!st.users.length" class="py-6 text-center text-sm text-gray-400">暂无</p>
            <router-link
              v-for="u in st.users.slice(0, 15)"
              :key="u.user_id"
              :to="{ path: '/admin/feedback', query: { status: '', user: u.email } }"
              class="block py-2.5 hover:text-primary-600"
            >
              <div class="flex items-center gap-2 text-sm">
                <span class="min-w-0 flex-1 truncate text-gray-800 dark:text-dark-100">{{ u.email }}</span>
                <span class="text-xs text-gray-400">问题 {{ u.issues }} · 建议 {{ u.suggestions }}</span>
                <span class="w-14 text-right text-xs tabular-nums" :class="rate(u) >= 50 ? 'text-emerald-600' : 'text-gray-400'" title="有效率">{{ rate(u) }}%</span>
                <span v-if="u.reward" class="w-12 text-right text-xs font-semibold text-violet-600 dark:text-violet-300">+{{ u.reward }}</span>
              </div>
              <div class="mt-1.5 flex h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <span
                  v-for="(n, key) in u.scenarios"
                  :key="key"
                  :style="{ width: pct(n, u.total), background: colorOf(String(key)) }"
                  :title="`${scenarioLabel(String(key))} ${n}`"
                />
              </div>
            </router-link>
          </Panel>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import { RouterLink, type RouteLocationRaw } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminApi, type Kind, type Stats } from '../api'
import { SEVERITIES, SEVERITY_META } from '../format'
import { allScenarios, loadAdminScenarios, scenarioIcon, scenarioLabel } from './shared'
import Select from '@/components/common/Select.vue'
import StatusPill from '../components/StatusPill.vue'

const days = ref(30)
const rangeOptions = [
  { value: 7, label: '7 天' },
  { value: 30, label: '30 天' },
  { value: 90, label: '90 天' }
]
const st = ref<Stats | null>(null)
const error = ref('')
const o = computed(() => st.value!.overview)

async function load() {
  error.value = ''
  try {
    st.value = await adminApi.stats(days.value)
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}

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

const maxTrend = computed(() => Math.max(1, ...(st.value?.trend ?? []).map((p) => Math.max(p.issues + p.suggestions, p.closed))))
const barH = (n: number) => `${(n / maxTrend.value) * 100}%`
const pct = (n: number, total: number) => `${total ? (n / total) * 100 : 0}%`
const rate = (u: Stats['users'][number]) => (u.total ? Math.round((u.accepted / u.total) * 100) : 0)
const formatHours = (h: number) => {
  const min = h * 60
  if (min < 1) return '<1 分'
  if (h < 1) return `${Math.round(min)} 分`
  return h < 48 ? `${h.toFixed(1)} 时` : `${(h / 24).toFixed(1)} 天`
}

const PALETTE = ['#38bdf8', '#a78bfa', '#34d399', '#fbbf24', '#f87171', '#f472b6', '#60a5fa', '#2dd4bf', '#fb923c', '#a3e635', '#c084fc', '#94a3b8']
const colorOf = (key: string) => {
  const i = allScenarios.value.findIndex((s) => s.key === key)
  return PALETTE[(i < 0 ? 11 : i) % PALETTE.length]
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

watch(days, load)
onMounted(() => {
  void loadAdminScenarios()
  void load()
})
</script>
