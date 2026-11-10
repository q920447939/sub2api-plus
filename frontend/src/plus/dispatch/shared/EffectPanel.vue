<!-- [plus] 一次调整的效果对比：流量去哪了、分组整体有没有变好、接收方扛不扛得住、系统确实在起作用 -->
<template>
  <div class="space-y-4" data-testid="dispatch-effect">
    <div v-if="!d" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
    <template v-else>
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-2">
          <b>{{ d.account.name }}</b>
          <span class="text-sm">{{ adjustText(d.log.from_action, d.log.from_factor) }} → <b>{{ adjustText(d.log.to_action, d.log.to_factor) }}</b></span>
          <span class="text-xs text-gray-500">· {{ d.group.name }} · {{ fullTime(d.log.created_at) }} 由 {{ d.log.operator || '—' }} {{ d.log.source === 'suggestion' ? '采纳建议' : '调整' }}</span>
        </div>
        <div class="mt-1 text-xs text-gray-500">原因：{{ d.log.reason || '—' }}</div>
        <div class="mt-3 rounded-lg px-3 py-2 text-sm" :class="d.mode === 'manual' ? 'bg-primary-50 text-primary-900 dark:bg-primary-900/20 dark:text-primary-100' : 'bg-amber-50 text-amber-900'">{{ d.conclusion }}</div>
      </div>

      <div class="card p-4">
        <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">流量去向</div>
        <LineChart :lines="areaLines" :labels="labels" :fmt="(v: number) => v.toFixed(0) + '%'" stacked :markers="markers" :height="220" />
        <div class="mt-2 text-xs text-gray-500">调整前 {{ snapBefore }}% · 预计 {{ snapPredicted }}%</div>
      </div>

      <div class="card p-4">
        <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">分组指标 <span class="font-normal text-gray-400">前 {{ d.requests_before }} 次 / 后 {{ d.requests_after }} 次请求</span></div>
        <table class="w-full text-sm">
          <thead><tr class="text-left text-xs text-gray-500"><th class="py-1 font-normal">指标</th><th class="font-normal">调整前</th><th class="font-normal">调整后</th><th class="font-normal">变化</th></tr></thead>
          <tbody>
            <tr v-for="c in d.group_compare" :key="c.key" class="border-t border-gray-50 dark:border-dark-700">
              <td class="py-1.5">{{ c.name }}</td>
              <td>{{ fmt(c, c.before) }}</td>
              <td>{{ fmt(c, c.after) }}</td>
              <td :class="c.trend === 'better' ? 'text-green-600' : c.trend === 'worse' ? 'text-red-600' : 'text-gray-400'">{{ TREND[c.trend] ?? '数据不够' }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="card p-4">
        <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">接收流量的账号</div>
        <table class="w-full text-sm">
          <thead><tr class="text-left text-xs text-gray-500"><th class="py-1 font-normal">账号</th><th class="font-normal">份额</th><th class="font-normal">首字耗时</th><th class="font-normal">限流率</th><th class="font-normal">当前状态</th></tr></thead>
          <tbody>
            <tr v-for="r in d.receivers ?? []" :key="r.account_id" class="border-t border-gray-50 dark:border-dark-700">
              <td class="py-1.5"><b>{{ r.name }}</b></td>
              <td>{{ pct(r.share_before) }} → {{ pct(r.share_after) }}</td>
              <td>{{ sec(r.ttft_before) }} → {{ sec(r.ttft_after) }}</td>
              <td>{{ pct(r.r429_before) }} → {{ pct(r.r429_after) }}</td>
              <td><LevelPill :level="r.worst" /></td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="card p-4 text-sm">
        <div class="mb-2 text-sm font-medium text-gray-900 dark:text-white">调度介入</div>
        已改派 <b>{{ (d.hook_hits ?? 0).toLocaleString() }}</b> 次请求<span v-if="d.hook_fallback" class="text-amber-700">，{{ d.hook_fallback }} 次因无其他可用账号放行</span><span v-if="d.mode !== 'manual'" class="text-gray-400">（当前档位不生效）</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi } from '../api'
import LevelPill from './LevelPill.vue'
import LineChart, { type ChartLine } from './LineChart.vue'
import { adjustText, fullTime, PALETTE } from './format'

const props = defineProps<{ logId: number }>()
const d = ref<any>(null)
const error = ref('')
const TREND: Record<string, string> = { better: '↓ 好转', worse: '↑ 变差', flat: '≈ 持平' }

watch(() => props.logId, async (id) => {
  d.value = null
  error.value = ''
  try {
    d.value = await dispatchApi.effect(id)
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}, { immediate: true })

const pad = (n: number) => String(n).padStart(2, '0')
const labels = computed(() => (d.value?.series.labels ?? []).map((t: string) => { const x = new Date(t); return `${pad(x.getHours())}:${pad(x.getMinutes())}` }))
const areaLines = computed<ChartLine[]>(() => {
  const s = d.value?.series
  if (!s) return []
  const ids = Object.keys(s.lines ?? {}).sort((a, b) => (Number(a) === d.value.log.account_id ? -1 : Number(b) === d.value.log.account_id ? 1 : 0))
  return ids.map((id, i) => ({ id, name: d.value.names[id] ?? `#${id}`, color: Number(id) === d.value.log.account_id ? '#dc2626' : PALETTE[i % PALETTE.length], values: s.lines[id] }))
})
const markers = computed(() => {
  const ls: string[] = d.value?.series.labels ?? []
  const i = ls.findIndex((t) => new Date(t).getTime() >= new Date(d.value.t0).getTime())
  return i >= 0 ? [{ index: i, label: '调整' }] : []
})
const snapBefore = computed(() => d.value?.log.snapshot?.before?.[d.value.log.account_id] ?? '—')
const snapPredicted = computed(() => d.value?.log.snapshot?.predicted?.[d.value.log.account_id] ?? '—')

const sec = (v?: number | null) => (v == null ? '—' : `${v.toFixed(1)} 秒`)
const pct = (v?: number | null) => (v == null ? '—' : `${v.toFixed(1)}%`)
function fmt(c: any, v?: number | null) {
  if (v == null) return '—'
  if (c.key === 'cost') return v.toFixed(2)
  if (c.unit === 's') return sec(v)
  return pct(v)
}
</script>
