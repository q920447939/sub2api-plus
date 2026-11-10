<!-- [plus] 上游调度台 · 检测记录：先选检测项，筛选条件、结果分类和展示方式跟着它的类型变 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-runs">
      <div class="flex flex-wrap items-center gap-3">
        <PSelect v-model="q.item" width-class="w-32" :options="activeItems.map((it) => ({ value: it.id, label: it.name }))" placeholder="检测项" @change="pickItem(q.item)" />
        <template v-if="item">
          <PSelect v-model="q.plan" width-class="w-36" :options="opt('全部计划', plans.map((p) => ({ value: String(p.id), label: `${platformName(p.platform)} · ${everyText(p.interval_minutes)}` })))" />
          <PSelect v-model="q.group" width-class="w-36" :options="opt('全部分组', (meta?.groups ?? []).map((g) => ({ value: String(g.id), label: g.name })))" />
          <PSelect v-model="q.account" width-class="w-36" :options="opt('全部账号', accounts.map((a) => ({ value: String(a.id), label: a.name })))" />
          <PSelect v-model="q.model" width-class="w-36" :options="opt('全部模型', planModels.map((m) => ({ value: m, label: m })))" />
          <PSelect v-model="q.outcome" width-class="w-36" :options="opt('全部结果', (spec?.outcomes ?? []).map((o) => ({ value: o.key, label: `${o.label}${board?.counts?.[o.key] != null ? ' · ' + board.counts[o.key] : ''}` })))" />
        </template>
        <div class="ml-auto inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
          <button v-for="v in VIEWS" :key="v.k" type="button" class="rounded-md px-3 py-1 text-sm" :class="view === v.k ? 'bg-white font-medium text-gray-900 shadow-sm dark:bg-dark-900 dark:text-white' : 'text-gray-500'" @click="view = v.k">{{ v.n }}</button>
        </div>
      </div>
      <div v-if="meta && !activeItems.length" class="py-16 text-center text-sm text-gray-400">还没有主动检测项</div>

      <template v-if="item">
        <div v-if="view === 'board'" class="card overflow-x-auto p-0">
          <div v-if="!board" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
          <div v-else-if="!board.rows?.length" class="py-16 text-center text-sm text-gray-400">没有记录</div>
          <table v-else class="w-full text-sm">
            <thead><tr class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800"><th class="px-3 py-2 font-normal">账号</th><th class="px-3 py-2 font-normal">模型</th><th class="px-3 py-2 font-normal">判定结论</th><th class="px-3 py-2 font-normal">最近结果</th></tr></thead>
            <tbody>
              <tr v-for="r in board.rows" :key="r.account_id + r.model" class="border-t border-gray-100 align-top dark:border-dark-700" data-testid="dispatch-runs-row">
                <td class="whitespace-nowrap px-3 py-2"><b>{{ r.name }}</b><div class="text-xs text-gray-400">{{ (r.groups ?? []).join('、') }}</div></td>
                <td class="px-3 py-2 font-mono text-xs">{{ r.model }}</td>
                <td class="px-3 py-2"><LevelPill v-if="r.verdict" :level="r.verdict.level" :text="r.verdict.level === 'info' ? r.verdict.short : undefined" /><div v-if="r.verdict && r.verdict.level !== 'info'" class="text-xs text-gray-500">{{ r.verdict.short }}</div></td>
                <td class="px-3 py-2">
                  <div v-if="spec?.gallery" class="flex gap-2 overflow-x-auto pb-1">
                    <RunThumb v-for="x in r.runs" :key="x.id" :run="x" @open="runId = x.id" />
                  </div>
                  <div v-else class="flex flex-wrap gap-1">
                    <button v-for="x in r.runs" :key="x.id" type="button" class="rounded border px-1.5 py-0.5 text-[11px]" :class="TONE_CLASS[outcomeOf(spec, x.outcome).tone]" :title="x.summary" @click="runId = x.id">
                      {{ chip(x) }} <span class="text-gray-400">{{ shortTime(x.t) }}</span>
                    </button>
                  </div>
                  <button v-if="r.total > r.runs.length" type="button" class="mt-1 text-xs text-primary-700" @click="perAccount += 10">更多（共 {{ r.total }} 次）</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-else class="card overflow-x-auto p-0">
          <table class="w-full text-sm">
            <thead><tr class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800"><th class="px-3 py-2 font-normal">时间</th><th class="px-3 py-2 font-normal">账号</th><th class="px-3 py-2 font-normal">模型</th><th class="px-3 py-2 font-normal">结果</th><th class="px-3 py-2 font-normal">说明</th><th class="px-3 py-2 font-normal">耗时</th><th class="px-3 py-2 font-normal">触发</th></tr></thead>
            <tbody>
              <tr v-for="r in list" :key="r.id" class="cursor-pointer border-t border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700" @click="runId = r.id">
                <td class="whitespace-nowrap px-3 py-2 text-xs">{{ fullTime(r.t) }}<div class="text-gray-400">{{ agoText(r.t) }}</div></td>
                <td class="px-3 py-2">{{ r.account }}</td>
                <td class="px-3 py-2 font-mono text-xs">{{ r.model }}</td>
                <td class="px-3 py-2"><span class="rounded border px-1.5 text-xs" :class="TONE_CLASS[outcomeOf(spec, r.outcome).tone]">{{ outcomeOf(spec, r.outcome).label }}</span></td>
                <td class="max-w-[380px] px-3 py-2 text-xs">{{ r.error || r.summary }}</td>
                <td class="px-3 py-2 text-xs">{{ r.duration_ms != null ? (r.duration_ms / 1000).toFixed(1) + ' 秒' : '—' }}</td>
                <td class="px-3 py-2 text-xs">{{ TRIGGER[r.trigger] ?? r.trigger }}</td>
              </tr>
              <tr v-if="!list.length"><td colspan="7" class="py-16 text-center text-sm text-gray-400">没有记录</td></tr>
            </tbody>
          </table>
          <div v-if="list.length && more" class="p-3 text-center"><button type="button" class="btn btn-secondary btn-sm" @click="loadList(true)">加载更多</button></div>
        </div>
      </template>
    </div>
    <RunDetailDialog :run-id="runId" @close="runId = null" @rerun="runId = null; reload()" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi } from '../api'
import LevelPill from '../shared/LevelPill.vue'
import PSelect from '../shared/PSelect.vue'
import RunDetailDialog from '../shared/RunDetailDialog.vue'
import RunThumb from '../shared/RunThumb.vue'
import { agoText, everyText, fullTime, outcomeOf, platformName, shortTime, TONE_CLASS } from '../shared/format'
import { loadMeta, meta, specOf } from '../shared/meta'

const opt = (all: string, o: { value: string; label: string }[]) => [{ value: '', label: all }, ...o]
const VIEWS = [{ k: 'board' as const, n: '按账号' }, { k: 'list' as const, n: '按时间' }]
const TRIGGER: Record<string, string> = { cron: '定时', manual: '手动', checkup: '立即体检' }

const route = useRoute()
const router = useRouter()
const q = reactive({
  item: Number(route.query.item) || 0,
  plan: String(route.query.plan ?? ''),
  group: String(route.query.group ?? ''),
  account: String(route.query.account ?? ''),
  model: String(route.query.model ?? ''),
  outcome: String(route.query.outcome ?? '')
})
const view = ref<'board' | 'list'>(route.query.view === 'list' ? 'list' : 'board')
const perAccount = ref(10)
const board = ref<any>(null)
const list = ref<any[]>([])
const more = ref(false)
const error = ref('')
const runId = ref<number | null>(null)

const activeItems = computed(() => (meta.value?.items ?? []).filter((i) => specOf(i.type)?.active))
const item = computed(() => activeItems.value.find((i) => i.id === q.item))
const spec = computed(() => (item.value ? specOf(item.value.type) : undefined))
const plans = computed(() => (meta.value?.plans ?? []).filter((p) => p.item_id === q.item))
const planModels = computed(() => [...new Set(plans.value.flatMap((p) => p.models))])
const accounts = computed(() => {
  const plats = new Set(plans.value.map((p) => p.platform))
  return (meta.value?.accounts ?? []).filter((a) => plats.has(a.platform) && (!q.group || a.group_ids.includes(Number(q.group))))
})

void loadMeta().then(() => {
  if (!q.item) q.item = activeItems.value[0]?.id ?? 0
  // 从账号页跳过来只带了 account，没有 item
  if (q.item && !item.value) q.item = activeItems.value[0]?.id ?? 0
})

function pickItem(id: number) {
  Object.assign(q, { item: id, plan: '', model: '', outcome: '' })
}

const params = () => ({ item: q.item, plan: q.plan || undefined, group: q.group || undefined, account: q.account || undefined, model: q.model || undefined, outcome: q.outcome || undefined })

async function loadBoard() {
  error.value = ''
  try {
    board.value = await dispatchApi.runsBoard({ ...params(), n: perAccount.value })
  } catch (e) {
    board.value = null
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}
async function loadList(append = false) {
  const before = append && list.value.length ? list.value[list.value.length - 1].id : undefined
  const rows = (await dispatchApi.runs({ ...params(), limit: 50, before_id: before })) ?? []
  list.value = append ? [...list.value, ...rows] : rows
  more.value = rows.length === 50
}
function reload() {
  if (!q.item) return
  void loadBoard()
  if (view.value === 'list') void loadList()
}
watch(() => ({ ...q, v: view.value, n: perAccount.value }), () => {
  void router.replace({ query: Object.fromEntries(Object.entries(q).filter(([, v]) => v !== '' && v !== 0)) as any })
  reload()
}, { deep: true, immediate: true })

function chip(x: any) {
  if (item.value?.type === 'graded') return x.outcome === 'error' ? '! 出错' : `${x.detail?.answer || '没答出'} ${x.outcome === 'pass' ? '✓' : '✗'}`
  if (item.value?.type === 'fingerprint') return x.outcome === 'pass' ? '✓ 一致' : x.outcome === 'fail' ? `✗ ${x.detail?.prediction}` : outcomeOf(spec.value, x.outcome).label
  return outcomeOf(spec.value, x.outcome).label
}
</script>
