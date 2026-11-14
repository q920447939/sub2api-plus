<!-- [plus] 上游调度台 · 检测记录：按检测项分开；按账号看（每个 账号 × 模型 一行：最近 12 次、结论、最近一次）或按时间看。
     存档题（检测方式声明 gallery）不判对错：一行直接显示最近几份作品的缩略图，没有「结论」列和结论筛选 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-runs">
      <div class="flex flex-wrap items-center gap-3">
        <div class="flex flex-wrap gap-2">
          <button v-for="t in tabs" :key="t.id" type="button" class="rounded-full border px-4 py-1.5 text-sm" :class="q.item_id === t.id ? 'border-gray-900 font-semibold text-gray-900 dark:border-white dark:text-white' : 'border-gray-200 bg-white text-gray-500 dark:border-dark-600 dark:bg-dark-800'" @click="q.item_id = t.id">{{ t.name }}</button>
        </div>
        <div class="ml-auto flex flex-wrap items-center gap-3">
          <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
            <button v-for="v in VIEWS" :key="v.k" type="button" class="rounded-md px-3 py-1 text-sm" :class="view === v.k ? 'bg-white font-medium shadow-sm dark:bg-dark-900' : 'text-gray-500'" @click="view = v.k; q.result = ''">{{ v.n }}</button>
          </div>
          <Select v-model="q.platform" :options="platformOptions" class="w-36" />
          <Select v-if="!gallery" v-model="q.result" :options="resultOptions" class="w-36" />
          <div class="relative w-48">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input v-model="kw" class="input pl-10" placeholder="搜索账号" />
          </div>
        </div>
      </div>

      <div v-if="board?.no_plan" class="card py-12 text-center text-sm text-gray-400">
        还没有给「{{ tabs.find((t) => t.id === q.item_id)?.name }}」建检测计划
        <router-link :to="{ path: `${DISPATCH_BASE}/config`, query: { item: q.item_id } }" class="ml-3 text-primary-600">去设置</router-link>
      </div>
      <template v-else>
        <div v-if="board" class="card grid grid-flow-col auto-cols-fr divide-x divide-gray-100 dark:divide-dark-700">
          <Kpi label="24 小时检测" :value="num(board.kpi.total)" />
          <Kpi v-for="o in kpiOutcomes" :key="o.key" :label="o.label" :value="num(board.kpi.outcomes[o.key] ?? 0)" :tone="o.tone === 'ok' ? 'text-emerald-600' : o.tone === 'bad' && board.kpi.outcomes[o.key] ? 'text-red-600' : o.tone === 'err' && board.kpi.outcomes[o.key] ? 'text-amber-600' : ''" />
          <Kpi label="花费" :value="tok(board.kpi.tokens)" sub="token" />
          <Kpi label="已测到" :value="String(board.kpi.tested)" :sub="`/ ${board.kpi.targets} 个账号 × 模型`" />
        </div>

        <div class="card overflow-x-auto p-0">
          <div v-if="view === 'board' && !board" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
          <table v-else-if="view === 'board'" class="table min-w-[1100px]">
            <thead><tr><th>账号</th><th>模型</th><th>检测项</th><th>{{ gallery ? '最近作品' : '最近 12 次' }}</th><th v-if="!gallery">结论</th><th>最近一次</th><th>频率</th><th></th></tr></thead>
            <tbody>
              <tr v-for="r in board!.rows" :key="`${r.account_id}-${r.item_id}-${r.model}`" class="cursor-pointer" data-testid="dispatch-runs-row" @click="probe = { accountId: r.account_id, itemId: r.item_id, model: r.model }">
                <td class="whitespace-nowrap"><div class="font-semibold">{{ r.account }}</div><div class="text-xs text-gray-500">{{ groupsText(r.group_ids) }}</div></td>
                <td class="font-mono text-sm">{{ r.model }}</td>
                <td class="whitespace-nowrap">{{ r.item }}</td>
                <td>
                  <div v-if="r.works?.length" class="flex gap-2" @click.stop>
                    <button v-for="w in r.works" :key="w.id" type="button" class="w-28 shrink-0 overflow-hidden rounded-md border border-gray-200 text-left hover:border-primary-400 dark:border-dark-700" :title="`${dt(w.at)} · ${w.summary}`" @click="runId = w.id">
                      <PreviewFrame :src="w.preview_url" thumb />
                      <div class="truncate px-1 py-0.5 text-[11px] tabular-nums text-gray-500">{{ dt(w.at) }}</div>
                    </button>
                  </div>
                  <span v-else-if="r.works" class="text-sm text-gray-400">{{ r.dots.length ? '还没有成功的作品' : '还没测到' }}</span>
                  <span v-else-if="!r.dots.length" class="text-sm text-gray-400">还没测到</span>
                  <span v-else class="flex gap-[3px]"><i v-for="x in r.dots" :key="x.id" class="h-3.5 w-3.5 rounded-[4px]" :class="DOT_CLASS[x.outcome] ?? 'bg-gray-300'" :title="`${dt(x.at)} · ${x.summary}`" /></span>
                </td>
                <td v-if="!gallery" class="whitespace-nowrap"><Tag v-if="!r.works" v-bind="verdictTag(r)" /><span v-else class="text-sm text-gray-400">不判对错</span></td>
                <td class="max-w-[360px]">
                  <template v-if="r.last"><div class="text-xs tabular-nums">{{ dt(r.last.at) }}</div><div class="truncate text-xs text-gray-500" :title="r.last.summary">{{ r.last.summary }}</div></template>
                  <span v-else class="text-gray-400">—</span>
                </td>
                <td class="whitespace-nowrap text-sm text-gray-500">{{ everyText(r.every) }}</td>
                <td class="whitespace-nowrap text-right" @click.stop>
                  <button type="button" class="btn btn-secondary btn-sm" @click="retest(r)">再测</button>
                </td>
              </tr>
              <tr v-if="!board!.rows.length"><td :colspan="gallery ? 7 : 8" class="py-14 text-center text-sm text-gray-400">没有符合条件的记录</td></tr>
            </tbody>
          </table>
          <template v-else>
            <table class="table min-w-[1100px]">
              <thead><tr><th>时间</th><th>账号</th><th>模型</th><th>检测项</th><th>结果</th><th>内容</th><th>耗时</th><th>token</th><th>触发</th></tr></thead>
              <tbody>
                <tr v-for="r in list" :key="r.id" class="cursor-pointer" @click="runId = r.id">
                  <td class="whitespace-nowrap text-sm tabular-nums">{{ dt(r.queued_at) }}</td>
                  <td class="whitespace-nowrap font-semibold">{{ board?.names.accounts[String(r.account_id)] ?? `#${r.account_id}` }}</td>
                  <td class="font-mono text-sm">{{ r.model }}</td>
                  <td class="whitespace-nowrap">{{ itemName(r.item_id) }}</td>
                  <td class="whitespace-nowrap"><span class="rounded-full px-2.5 text-xs font-semibold leading-[18px]" :class="TONE_CLASS[outcomeOf(specOf(r.item_id), r.outcome).tone]">{{ outcomeOf(specOf(r.item_id), r.outcome).label }}</span></td>
                  <td class="max-w-[380px] truncate text-sm" :title="r.error || r.summary">{{ r.error || r.summary }}</td>
                  <td class="whitespace-nowrap text-sm text-gray-500">{{ r.duration_ms != null ? `${(r.duration_ms / 1000).toFixed(1)} 秒` : '—' }}</td>
                  <td class="whitespace-nowrap text-sm text-gray-500">{{ r.input_tokens != null ? num(r.input_tokens + (r.output_tokens ?? 0)) : '—' }}</td>
                  <td class="whitespace-nowrap text-sm text-gray-500">{{ TRIGGER[r.trigger] ?? r.trigger }}</td>
                </tr>
                <tr v-if="!list.length"><td colspan="9" class="py-14 text-center text-sm text-gray-400">没有符合条件的记录</td></tr>
              </tbody>
            </table>
            <div v-if="more" class="p-3 text-center"><button type="button" class="btn btn-secondary btn-sm" @click="loadList(true)">加载更多</button></div>
          </template>
        </div>
      </template>
    </div>
    <ProbeDialog :target="probe" @close="probe = null" @changed="load" @open-run="runId = $event" />
    <RunDialog :run-id="runId" @close="runId = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type ItemView, type Meta, type Run, type RunsBoard, type RunsRow } from '../api'
import { DISPATCH_BASE } from '../routes'
import Kpi from '../shared/Kpi.vue'
import ProbeDialog from '../shared/ProbeDialog.vue'
import RunDialog from '../shared/RunDialog.vue'
import Tag from '../shared/Tag.vue'
import PreviewFrame from '@/plus/iq/shared/PreviewFrame.vue'
import { DOT_CLASS, TONE_CLASS, TRIGGER, dt, everyText, num, outcomeOf, platformName, tok } from '../shared/format'

const VIEWS = [{ k: 'board' as const, n: '按账号' }, { k: 'list' as const, n: '按时间' }]
const route = useRoute()
const appStore = useAppStore()
const items = ref<ItemView[]>([])
const meta = ref<Meta | null>(null)
const board = ref<RunsBoard | null>(null)
const list = ref<Run[]>([])
const more = ref(false)
const error = ref('')
const view = ref<'board' | 'list'>('board')
const kw = ref('')
const q = reactive({ item_id: Number(route.query.item) || 0, platform: '', result: '', q: '' })
const probe = ref<{ accountId: number; itemId: number; model?: string } | null>(null)
const runId = ref<number | null>(null)

const tabs = computed(() => [{ id: 0, name: '全部' }, ...items.value.filter((i) => i.enabled).map((i) => ({ id: i.id, name: i.name }))])
const specOf = (id: number) => items.value.find((i) => i.id === id)?.spec
// 选中的检测项结果是作品（存档题）：显示缩略图，不显示结论
const gallery = computed(() => view.value === 'board' && !!specOf(q.item_id)?.gallery)
const itemName = (id: number) => items.value.find((i) => i.id === id)?.name ?? `#${id}`
const groupsText = (ids: number[]) => {
  const n = (id: number) => board.value?.names.groups[String(id)] ?? `#${id}`
  return ids.length > 1 ? `${n(ids[0])} 等 ${ids.length} 个分组` : ids.length ? n(ids[0]) : ''
}
const platformOptions = computed(() => [{ value: '', label: '全部平台' }, ...(meta.value?.platforms ?? []).map((p) => ({ value: p, label: platformName(p) }))])
const outcomes = computed(() => {
  const spec = specOf(q.item_id)
  if (spec) return spec.outcomes
  const seen = new Map<string, { key: string; label: string; tone: string }>()
  for (const it of items.value) for (const o of it.spec.outcomes) if (!seen.has(o.key)) seen.set(o.key, o)
  return [...seen.values()]
})
const kpiOutcomes = computed(() => outcomes.value.filter((o) => o.key !== 'ignored' && o.key !== 'skipped'))
const resultOptions = computed(() =>
  view.value === 'board'
    ? [{ value: '', label: '全部结论' }, { value: 'fail', label: '未通过' }, { value: 'fault', label: '测不了' }, { value: 'untested', label: '还没测到' }]
    : [{ value: '', label: '全部结果' }, ...outcomes.value.map((o) => ({ value: o.key, label: o.label }))]
)

function verdictTag(r: RunsRow): { text: string; sub?: string; tone: 'green' | 'oamber' | 'gray' | 'red' } {
  const v = r.verdict
  if (!v) return { text: '未检测', tone: 'gray' }
  if (v.level === 'bad') return { text: '未通过', sub: v.short, tone: 'oamber' }
  if (v.level === 'ok') return { text: '通过', tone: 'green' }
  if (v.level === 'fault') return { text: '测不了', sub: v.short, tone: 'red' }
  if (v.level === 'info') return { text: v.short, tone: 'gray' }
  return { text: '数据不足', sub: v.short, tone: 'gray' }
}

async function load() {
  try {
    board.value = await dispatchApi.runsBoard(view.value === 'board' ? q : { ...q, result: '' })
    error.value = ''
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  }
  if (view.value === 'list') await loadList(false)
}

async function loadList(append: boolean) {
  try {
    const before = append && list.value.length ? list.value[list.value.length - 1].id : undefined
    const rs = await dispatchApi.runsList(q, before)
    list.value = append ? [...list.value, ...rs] : rs
    more.value = rs.length >= 100
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  }
}

void Promise.all([dispatchApi.items(), dispatchApi.meta()]).then(([i, m]) => {
  items.value = i
  meta.value = m
})
let timer: ReturnType<typeof setTimeout> | undefined
watch(kw, (v) => {
  clearTimeout(timer)
  timer = setTimeout(() => (q.q = v), 300)
})
// 存档题没有结论筛选：切过去时清掉之前选的结论，不然会被一个看不见的筛选条件挡住
watch(gallery, (g) => {
  if (g) q.result = ''
})
watch([() => ({ ...q }), view], load, { immediate: true, deep: true })

async function retest(r: RunsRow) {
  try {
    const x = await dispatchApi.retest(r.account_id, r.item_id, [r.model])
    appStore.showSuccess(x.queued ? `已开始检测 ${r.account} · ${r.model}` : '已经在排队了')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  }
}
</script>
