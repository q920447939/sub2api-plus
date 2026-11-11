<!-- [plus] 上游调度台 · 指标对比：同一分组、同一模型下各账号的同一个检测项放在一起看，回答「和同类比怎么样、是一直这样还是刚变差」 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-compare">
      <div class="flex flex-wrap items-center gap-3">
        <PSelect v-model="q.group" width-class="w-48" placeholder="分组" :options="(meta?.groups ?? []).map((g) => ({ value: g.id, label: `${g.name} · ${platformName(g.platform)}` }))" />
        <PSelect v-model="q.model" width-class="w-52" :disabled="!models.length" :placeholder="models.length ? '模型' : '无流量模型'" :options="models.map((m) => ({ value: m, label: m }))" />
        <PSelect v-model="q.item" width-class="w-44" placeholder="检测项" :groups="itemGroups" />
        <div class="ml-auto inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
          <button v-for="r in RANGES" :key="r.k" type="button" class="rounded-md px-3 py-1 text-sm disabled:opacity-40" :disabled="graded && r.k !== '7d'" :class="q.range === r.k ? 'bg-white font-medium text-gray-900 shadow-sm dark:bg-dark-900 dark:text-white' : 'text-gray-500'" @click="q.range = r.k">{{ r.n }}</button>
        </div>
      </div>

      <div v-if="!data" class="py-16 text-center text-sm text-gray-400">{{ error || (q.group && q.model ? '加载中…' : '选择分组和模型') }}</div>
      <template v-else-if="data.graded">
        <div class="card p-4">
          <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">{{ data.item.name }} · 每日答对率</div>
          <LineChart :lines="gradedLines" :labels="data.labels.map(dayLabel)" :fmt="(v: number) => v.toFixed(0) + '%'" zero-based />
        </div>
        <div class="card overflow-x-auto p-0">
          <table class="w-full text-sm">
            <thead><tr class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800"><th class="px-3 py-2 font-normal">账号</th><th class="px-3 py-2 font-normal">近 N 次</th><th class="px-3 py-2 font-normal">7 天答对率</th><th class="px-3 py-2 font-normal">7 天出错</th><th class="px-3 py-2 font-normal">判定结论</th><th /></tr></thead>
            <tbody>
              <tr v-for="(r, i) in data.rows" :key="r.account_id" class="border-t border-gray-100 dark:border-dark-700">
                <td class="px-3 py-2"><span :style="{ color: PALETTE[i % PALETTE.length] }">●</span> <b>{{ r.name }}</b></td>
                <td class="px-3 py-2">{{ r.verdict.short }}</td>
                <td class="px-3 py-2">{{ r.total_7d ? Math.round((r.pass_7d / r.total_7d) * 100) + '%' : '—' }}（{{ r.total_7d }} 次）</td>
                <td class="px-3 py-2">{{ r.errors_7d }}</td>
                <td class="px-3 py-2"><LevelPill :level="r.verdict.level" /></td>
                <td class="px-3 py-2"><router-link class="text-xs text-primary-600 hover:underline" :to="{ path: `${BASE}/runs`, query: { item: q.item, account: r.account_id } }">记录</router-link></td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
      <template v-else>
        <div class="card p-4">
          <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white" :title="data.metric.desc">{{ data.metric.name }}</div>
          <LineChart :lines="trafficLines" :labels="data.labels.map(data.daily ? dayLabel : hourLabel)" :fmt="(v: number) => fmtMetric(data.metric, v)" />
        </div>
        <div class="card overflow-x-auto p-0">
          <table class="w-full text-sm">
            <thead><tr class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800"><th class="px-3 py-2 font-normal">账号</th><th class="px-3 py-2 font-normal">当前值</th><th class="px-3 py-2 font-normal">参照值</th><th class="px-3 py-2 font-normal">判定结论</th><th class="px-3 py-2 font-normal">区间平均</th><th class="px-3 py-2 font-normal">区间最差值</th><th class="px-3 py-2 font-normal">近 1 小时请求</th></tr></thead>
            <tbody>
              <tr v-for="(r, i) in data.rows" :key="r.account_id" class="cursor-pointer border-t border-gray-100 dark:border-dark-700" :class="r.account_id === data.highlight ? 'bg-primary-50/70 dark:bg-primary-900/10' : ''" @click="q.highlight = r.account_id">
                <td class="px-3 py-2"><span :style="{ color: PALETTE[i % PALETTE.length] }">●</span> <b>{{ r.name }}</b></td>
                <td class="px-3 py-2">{{ fmtMetric(data.metric, r.verdict.value) }}</td>
                <td class="px-3 py-2 text-xs">{{ r.verdict.ref != null ? fmtMetric(data.metric, r.verdict.ref) : '—' }}<div class="text-gray-400">{{ r.verdict.evidence?.reference_label }}</div></td>
                <td class="px-3 py-2"><LevelPill :level="r.verdict.level" /><div v-if="r.verdict.evidence?.duration_minutes && r.verdict.level !== 'ok'" class="text-xs text-gray-400">已持续 {{ Math.round(r.verdict.evidence.duration_minutes / 60) }} 小时</div></td>
                <td class="px-3 py-2">{{ fmtMetric(data.metric, r.mean) }}</td>
                <td class="px-3 py-2 text-xs">{{ fmtMetric(data.metric, r.worst) }}<div v-if="r.worst_at" class="text-gray-400">{{ data.daily ? dayLabel(r.worst_at) : hourLabel(r.worst_at) }}</div></td>
                <td class="px-3 py-2 text-xs">{{ r.requests_now }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="data.heatmap" class="card p-4">
          <div class="mb-1 font-semibold">时段规律：{{ highlightName }}</div>
          <div class="mb-2 text-sm text-gray-600 dark:text-dark-200">{{ heatSentence }}</div>
          <table class="border-separate text-[10px]" style="border-spacing: 2px">
            <tr><th /><th v-for="h in 24" :key="h" class="font-normal text-gray-400">{{ h - 1 }}</th></tr>
            <tr v-for="d in data.heatmap.days" :key="d.day">
              <td class="pr-2 text-right text-[11px] text-gray-500">{{ dayLabel(d.day) }}</td>
              <td v-for="(c, h) in d.cells" :key="h" class="h-[18px] w-6 rounded" :style="{ background: HEAT[c] }" :title="`${dayLabel(d.day)} ${h}:00 ${HEAT_TEXT[c]}`" />
            </tr>
          </table>
          <div class="mt-2 flex gap-4 text-xs text-gray-500"><span v-for="(t, k) in HEAT_TEXT" :key="k"><i class="mr-1 inline-block h-2.5 w-4 rounded" :style="{ background: HEAT[k] }" />{{ t }}</span></div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi } from '../api'
import { DISPATCH_BASE as BASE } from '../routes'
import LevelPill from '../shared/LevelPill.vue'
import LineChart, { type ChartLine } from '../shared/LineChart.vue'
import PSelect from '../shared/PSelect.vue'
import { fmtMetric, PALETTE, platformName } from '../shared/format'
import { loadMeta, meta } from '../shared/meta'

const route = useRoute()
const router = useRouter()
const RANGES = [{ k: '24h', n: '24 小时' }, { k: '7d', n: '7 天' }, { k: '30d', n: '30 天' }]
const HEAT: Record<string, string> = { ok: '#bbf7d0', warn: '#fcd34d', bad: '#ef4444', few: '#f1f5f9', na: 'transparent' }
const HEAT_TEXT: Record<string, string> = { ok: '正常', warn: '可疑', bad: '异常', few: '请求太少' }

const q = reactive({
  group: Number(route.query.group) || 0,
  model: String(route.query.model ?? ''),
  item: Number(route.query.item) || 0,
  range: String(route.query.range ?? '24h'),
  highlight: Number(route.query.highlight) || 0
})
const data = ref<any>(null)
const error = ref('')
const groupModels = ref<string[]>([])

const comparable = computed(() => (meta.value?.items ?? []).filter((i) => i.enabled && meta.value?.types.find((t) => t.key === i.type)?.comparable))
const itemGroups = computed(() => (meta.value?.dimensions ?? []).filter((x) => x.enabled).map((d) => ({
  label: d.name,
  options: comparable.value.filter((i) => i.dimension_id === d.id).map((it) => ({ value: it.id, label: it.name }))
})))
const graded = computed(() => comparable.value.find((i) => i.id === q.item)?.type === 'graded')
const models = computed(() => (q.model && !groupModels.value.includes(q.model) ? [q.model, ...groupModels.value] : groupModels.value))

void loadMeta().then((m) => {
  if (!q.group) q.group = m.groups[0]?.id ?? 0
  if (!q.item) q.item = comparable.value.find((i) => i.type === 'traffic')?.id ?? comparable.value[0]?.id ?? 0
})

watch(
  () => q.group,
  async (g) => {
    if (!g) return
    const ov = await dispatchApi.overview()
    const grp = (ov.groups ?? []).find((x) => x.id === g)
    groupModels.value = Object.entries(grp?.models ?? {}).sort((a, b) => b[1] - a[1]).map(([m]) => m)
    if (!q.model || !groupModels.value.includes(q.model)) q.model = groupModels.value[0] ?? q.model
  },
  { immediate: true }
)

watch(graded, (g) => {
  if (g) q.range = '7d'
})

watch(
  () => ({ ...q }),
  async () => {
    if (!q.group || !q.model || !q.item) return
    void router.replace({ query: { ...q } as any })
    error.value = ''
    try {
      data.value = await dispatchApi.compare({ group: q.group, model: q.model, item: q.item, range: q.range, highlight: q.highlight })
    } catch (e) {
      data.value = null
      error.value = extractApiErrorMessage(e, '加载失败')
    }
  },
  { immediate: true, deep: true }
)

const highlightName = computed(() => data.value?.rows?.find((r: any) => r.account_id === data.value.highlight)?.name ?? '')

const trafficLines = computed<ChartLine[]>(() => {
  const d = data.value
  if (!d || d.graded) return []
  const out: ChartLine[] = (d.lines ?? []).map((l: any, i: number) => ({
    id: l.account_id, name: l.name, color: PALETTE[i % PALETTE.length], values: l.values,
    width: l.account_id === d.highlight ? 2.6 : 1.4, hidden: l.account_id !== d.highlight
  }))
  if (d.reference && !d.daily) {
    out.push({ id: '_ref', name: '参照', color: '#64748b', dashed: true, width: 1.4, values: d.reference.values })
    out.push({ id: '_warn', name: '可疑线', color: '#f59e0b', dashed: true, width: 1, values: d.reference.warn, hidden: true })
    out.push({ id: '_bad', name: '异常线', color: '#dc2626', dashed: true, width: 1, values: d.reference.bad })
  }
  return out
})

const gradedLines = computed<ChartLine[]>(() =>
  (data.value?.lines ?? []).map((l: any, i: number) => ({ id: l.account_id, name: l.name, color: PALETTE[i % PALETTE.length], values: l.values }))
)

const heatSentence = computed(() => {
  const r: [number, number][] = data.value?.heatmap?.hot_ranges ?? []
  if (!r.length) return `${highlightName.value} 近 7 天没有固定时段反复超线。`
  return `${highlightName.value} 在每天 ${r.map(([a, b]) => (a === b ? `${a} 点` : `${a}–${b + 1} 点`)).join('、')} 经常超线（7 天里至少 3 天），更像是时段性的容量问题，不一定是突发故障。`
})

const pad = (n: number) => String(n).padStart(2, '0')
function hourLabel(t: string) {
  const d = new Date(t)
  return q.range === '24h' ? `${pad(d.getHours())}:00` : `${d.getMonth() + 1}/${d.getDate()} ${pad(d.getHours())}时`
}
function dayLabel(t: string) {
  const d = new Date(t)
  return `${d.getMonth() + 1}/${d.getDate()}`
}
</script>
