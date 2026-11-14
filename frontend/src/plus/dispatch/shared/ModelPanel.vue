<!-- [plus] 账号详情里展开一个模型：指标和同类比、每项检测最近的结果、趋势（这个账号 vs 同模型其他账号）、跑这个模型的账号并排对比 -->
<template>
  <div v-if="!d" class="py-8 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
  <div v-else class="space-y-5 py-1" data-testid="dispatch-model-panel">
    <div class="grid gap-8 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
      <div>
        <div class="mb-1 text-xs text-gray-400">最近 {{ d.hours }} 小时</div>
        <div v-for="m in d.metrics" :key="m.key" class="flex items-baseline gap-3 border-b border-gray-100 py-2 text-sm last:border-b-0 dark:border-dark-700">
          <span class="w-24 text-gray-500">{{ m.name }}</span>
          <b class="tabular-nums" :class="m.worse ? 'text-red-600' : ''">{{ unitVal(m.unit, m.value) }}</b>
          <span v-if="m.peer != null" class="ml-auto text-xs tabular-nums text-gray-400">其他账号 {{ unitVal(m.unit, m.peer) }}</span>
        </div>
        <div class="mb-1 mt-4 text-xs text-gray-400">检测</div>
        <div v-for="p in d.probes" :key="p.item_id" class="flex cursor-pointer items-center gap-3 border-b border-gray-100 py-2 text-sm last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800" @click="emit('probe', p.item_id)">
          <span class="w-24 text-gray-500">{{ p.name }}</span>
          <b :class="STATE[p.state]?.cls">{{ STATE[p.state]?.t }}</b>
          <span class="truncate text-xs text-gray-400" :title="p.long">{{ p.long }}</span>
          <span class="ml-auto flex gap-[3px]"><i v-for="r in p.dots" :key="r.id" class="h-2.5 w-2.5 rounded-[3px]" :class="DOT_CLASS[r.outcome] ?? 'bg-gray-300'" :title="`${dt(r.at)} · ${r.summary}`" /></span>
          <span class="text-gray-400">›</span>
        </div>
      </div>
      <div>
        <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
          <button v-for="k in KEYS" :key="k.k" type="button" class="rounded-md px-3 py-1 text-xs" :class="metric === k.k ? 'bg-white font-semibold shadow-sm dark:bg-dark-900' : 'text-gray-500'" @click="metric = k.k">{{ k.n }}</button>
        </div>
        <LineChart class="mt-2" :lines="lines" :labels="labels" :fmt="fmt" :markers="markers" :height="220" />
      </div>
    </div>
    <div v-if="d.peers.length > 1">
      <div class="mb-1 text-xs text-gray-400">跑这个模型的账号</div>
      <div class="overflow-x-auto rounded-xl border border-gray-100 dark:border-dark-700">
        <table class="table">
          <thead><tr><th>账号</th><th>近 48 小时</th><th>状态</th><ColHeads /></tr></thead>
          <tbody>
            <tr v-for="p in d.peers" :key="p.account_id" :class="p.me ? 'bg-gray-50 dark:bg-dark-800' : 'cursor-pointer'" @click="!p.me && emit('openAccount', p.account_id)">
              <td class="whitespace-nowrap font-semibold">{{ p.name }}<span v-if="p.me" class="ml-2 text-xs font-normal text-gray-400">本账号</span></td>
              <td class="min-w-[260px]"><StatusBar :bars="p.bars" /></td>
              <td><Tags :rules="[]" :probes="p.probes" :groups="1" :group-name="() => ''" :model="model" @probe="emit('probe', $event)" /></td>
              <ColCells :c="p.cols" />
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type ModelDetail } from '../api'
import ColCells from './ColCells.vue'
import ColHeads from './ColHeads.vue'
import LineChart from './LineChart.vue'
import StatusBar from './StatusBar.vue'
import Tags from './Tags.vue'
import { DOT_CLASS, dt, unitVal } from './format'

const props = defineProps<{ accountId: number; model: string; accountName: string; marks: string[] }>()
const emit = defineEmits<{ probe: [itemId: number]; openAccount: [id: number] }>()

const KEYS = [
  { k: 'score', n: '健康分', unit: '' },
  { k: 'err', n: '报错率', unit: '%' },
  { k: 'r429', n: '限流率', unit: '%' },
  { k: 'ttft', n: '首字耗时', unit: 's' },
  { k: 'tps', n: '输出速度', unit: 'tok/s' }
]
const STATE: Record<string, { t: string; cls: string }> = {
  fail: { t: '未通过', cls: 'text-red-600' },
  pass: { t: '通过', cls: 'text-emerald-600' },
  ignored: { t: '已标误报', cls: 'text-gray-500' },
  fault: { t: '测不了', cls: 'text-amber-600' },
  few: { t: '数据不足', cls: 'text-gray-400' },
  none: { t: '未检测', cls: 'text-gray-400' },
  info: { t: '只存作品', cls: 'text-gray-500' }
}

const d = ref<ModelDetail | null>(null)
const error = ref('')
const metric = ref('score')

watch(
  () => [props.accountId, props.model],
  async () => {
    d.value = null
    try {
      d.value = await dispatchApi.model(props.accountId, props.model)
    } catch (e) {
      error.value = extractApiErrorMessage(e)
    }
  },
  { immediate: true }
)

const labels = computed(() => (d.value?.trend.labels ?? []).map((t) => `${String(new Date(t * 1000).getHours()).padStart(2, '0')}:00`))
const lines = computed(() => {
  const t = d.value?.trend
  if (!t) return []
  const out = [{ id: 'me', name: props.accountName, color: '#0d9488', values: t.mine[metric.value] ?? [], width: 2 }]
  if ((t.peers[metric.value] ?? []).some((v) => v != null)) out.unshift({ id: 'peers', name: '同模型的其他账号', color: '#aeaeb2', values: t.peers[metric.value], width: 2 })
  return out
})
const fmt = (v: number) => {
  const u = KEYS.find((k) => k.k === metric.value)?.unit ?? ''
  return u ? unitVal(u, v) : String(Math.round(v))
}
const markers = computed(() => {
  const ts = d.value?.trend.labels ?? []
  return props.marks
    .map((m) => {
      const t = new Date(m).getTime() / 1000
      let idx = -1
      ts.forEach((s, i) => {
        if (s <= t) idx = i
      })
      return { index: idx, label: '调整' }
    })
    .filter((m) => m.index >= 0)
})
</script>
