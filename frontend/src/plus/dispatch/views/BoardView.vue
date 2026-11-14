<!-- [plus] 上游调度台 · 上游账号：按平台分开，一个账号一行运营摘要（最近请求 + 状态条、状态标签、一排数字、权重）。
     列头可排序，行首把手可拖动自定义顺序（存在浏览器里）。 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-board">
      <div class="flex flex-wrap items-center gap-3">
        <div class="flex flex-wrap gap-2">
          <button
            v-for="p in board?.platforms ?? []" :key="p.platform" type="button"
            class="inline-flex items-center gap-2 rounded-full border px-4 py-1.5 text-sm"
            :class="p.platform === platform ? 'border-gray-900 font-semibold text-gray-900 dark:border-white dark:text-white' : 'border-gray-200 bg-white text-gray-500 dark:border-dark-600 dark:bg-dark-800'"
            @click="pickPlatform(p.platform)"
          >
            <i class="h-2 w-2 rounded-full" :class="STATUS_META[p.worst].dot" />{{ platformName(p.platform) }}<span class="text-xs font-normal text-gray-400">{{ p.accounts }}</span>
          </button>
        </div>
        <div class="ml-auto flex flex-wrap items-center gap-3">
          <span v-if="order[platform] && !sort" class="cursor-pointer text-sm text-primary-600" @click="resetOrder">恢复默认排序</span>
          <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
            <button v-for="g in GRANS" :key="g.v" type="button" class="rounded-md px-3 py-1 text-sm" :class="gran === g.v ? 'bg-white font-medium shadow-sm dark:bg-dark-900' : 'text-gray-500'" @click="gran = g.v">{{ g.n }}</button>
          </div>
          <Select v-model="group" :options="groupOptions" class="w-48" />
          <div class="relative w-52">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input v-model="search" class="input pl-10" placeholder="搜索账号" />
          </div>
          <button type="button" class="btn btn-secondary" :disabled="loading" title="刷新" @click="load"><Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" /></button>
        </div>
      </div>

      <ApplyOff v-if="board && !board.apply" :rules="board.accounts.reduce((n, a) => n + a.rules.length, 0)" @changed="load" />

      <div v-if="sum" class="card grid grid-flow-col auto-cols-fr divide-x divide-gray-100 dark:divide-dark-700">
        <Kpi label="账号" :value="String(all.length)" />
        <Kpi label="需要处理" :value="String(needCount)" :tone="needCount ? 'text-red-600' : ''" clickable :active="onlyNeeds" @click="onlyNeeds = !onlyNeeds" />
        <Kpi label="24 小时请求" :value="num(sum.n24)" />
        <Kpi label="错误率" :value="pct1(sum.err_pct)" :tone="LV.err(sum.err_pct)" />
        <Kpi label="首字耗时" :value="sum.ttft == null ? '—' : `${sum.ttft.toFixed(1)} 秒`" :tone="LV.ttft(sum.ttft)" />
        <Kpi label="缓存命中" :value="sum.cache == null ? '—' : `${sum.cache.toFixed(1)}%`" :sub="sum.cache_read ? tok(sum.cache_read) : ''" :tone="LV.cache(sum.cache)" />
        <Kpi label="权重调整中" :value="String(tunedCount)" clickable :active="onlyTuned" @click="onlyTuned = !onlyTuned" />
      </div>

      <div class="card overflow-x-auto p-0">
        <div v-if="!board" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
        <table v-else class="table min-w-[1250px]">
          <thead>
            <tr>
              <th class="sticky left-0 z-10">名称</th>
              <th class="cursor-pointer whitespace-nowrap" @click="toggleSort('last')">最近请求<span class="ml-1 cursor-help text-gray-400" :title="`最近一次请求的时间\n下面每一格是一${gran === 'day' ? '天' : '个小时'}：绿色正常、黄色降级、红色异常、灰色没有请求`">ⓘ</span><span v-if="sort?.key === 'last'" class="ml-1 text-primary-600">{{ sort.dir > 0 ? '▲' : '▼' }}</span></th>
              <th>状态</th>
              <ColHeads sortable :sort="sort" @sort="toggleSort" />
              <th class="cursor-pointer whitespace-nowrap" @click="toggleSort('week')">7D 活跃量<span class="ml-1 cursor-help text-gray-400" title="近 7 天成功和失败的请求数">ⓘ</span><span v-if="sort?.key === 'week'" class="ml-1 text-primary-600">{{ sort.dir > 0 ? '▲' : '▼' }}</span></th>
              <th class="sticky right-0 z-10">权重</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="a in rows" :key="a.id" class="group cursor-pointer" :class="[dragId === a.id ? 'opacity-40' : '', over?.id === a.id ? (over.after ? 'shadow-[inset_0_-2px_0_theme(colors.primary.500)]' : 'shadow-[inset_0_2px_0_theme(colors.primary.500)]') : '']"
              data-testid="dispatch-board-row" @click="open(a.id)" @dragover.prevent="dragOver(a.id, $event)" @drop.prevent="drop(a.id, $event)"
            >
              <td class="sticky left-0 z-[1] bg-white dark:bg-dark-900">
                <div class="flex min-w-[160px] items-center gap-2">
                  <span class="w-3 cursor-grab select-none text-gray-300 opacity-0 group-hover:opacity-100" draggable="true" title="拖动调整顺序" @click.stop @dragstart="dragStart(a.id, $event)" @dragend="dragEnd">⋮⋮</span>
                  <div>
                    <div class="whitespace-nowrap text-[15px] font-semibold text-gray-900 dark:text-white">{{ a.name }}</div>
                    <div class="whitespace-nowrap text-xs text-gray-500">{{ groupsText(a.group_ids) }}</div>
                  </div>
                </div>
              </td>
              <td>
                <div class="mb-1.5 text-xs tabular-nums text-gray-800 dark:text-gray-200">{{ lastText(a.last_at, a.last_hour) }}</div>
                <StatusBar :bars="a.bars" :gran="gran" :marks="a.marks" fixed />
              </td>
              <td @click.stop>
                <Tags :rules="a.rules" :probes="a.probes" :groups="a.group_ids.length" :group-name="groupName" :is-new="a.is_new" :created-at="a.created_at" :down-models="a.down_models" @probe="probe = { accountId: a.id, itemId: $event }" />
              </td>
              <ColCells :c="a.cols" />
              <td class="whitespace-nowrap">
                <div class="text-base font-semibold tabular-nums">成功 {{ times(a.week_ok) }}</div>
                <div class="text-xs text-gray-500 tabular-nums">失败 {{ times(a.week_fail) }}</div>
              </td>
              <td class="sticky right-0 z-[1] bg-white shadow-[-8px_0_8px_-8px_rgba(0,0,0,0.08)] dark:bg-dark-900" @click.stop>
                <FlowButton :rules="a.rules" :groups="a.group_ids.length" @click="weight = { accountId: a.id }" />
              </td>
            </tr>
            <tr v-if="!rows.length"><td colspan="10" class="py-14 text-center text-sm text-gray-400">没有符合条件的账号</td></tr>
          </tbody>
        </table>
      </div>
    </div>
    <WeightDialog :target="weight" @close="weight = null" @saved="load" />
    <ProbeDialog :target="probe" @close="probe = null" @changed="load" @open-run="runId = $event" />
    <RunDialog :run-id="runId" @close="runId = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type AccountRow, type Board, type Gran } from '../api'
import { DISPATCH_BASE } from '../routes'
import ApplyOff from '../shared/ApplyOff.vue'
import ColCells from '../shared/ColCells.vue'
import ColHeads from '../shared/ColHeads.vue'
import FlowButton from '../shared/FlowButton.vue'
import Kpi from '../shared/Kpi.vue'
import ProbeDialog from '../shared/ProbeDialog.vue'
import RunDialog from '../shared/RunDialog.vue'
import StatusBar from '../shared/StatusBar.vue'
import Tags from '../shared/Tags.vue'
import WeightDialog, { type WeightTarget } from '../shared/WeightDialog.vue'
import { COLS, LV, STATUS_META, lastText, num, pct1, platformName, times, tok } from '../shared/format'

const ORDER_KEY = 'plus-dispatch-order'
const PLATFORM_KEY = 'plus-dispatch-platform'
const GRANS = [{ v: 'hour' as const, n: '按小时' }, { v: 'day' as const, n: '按天' }]

const router = useRouter()
const board = ref<Board | null>(null)
const error = ref('')
const loading = ref(false)
const gran = ref<Gran>('hour')
const platform = ref(localStorage.getItem(PLATFORM_KEY) ?? '')
const group = ref<number | string>('')
const search = ref('')
const onlyNeeds = ref(false)
const onlyTuned = ref(false)
const sort = ref<{ key: string; dir: number } | null>(null)
const order = ref<Record<string, number[]>>(JSON.parse(localStorage.getItem(ORDER_KEY) ?? '{}'))
const weight = ref<WeightTarget | null>(null)
const probe = ref<{ accountId: number; itemId: number } | null>(null)
const runId = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    board.value = await dispatchApi.board(gran.value)
    if (!board.value.platforms.some((p) => p.platform === platform.value)) platform.value = board.value.platforms[0]?.platform ?? ''
    error.value = ''
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  } finally {
    loading.value = false
  }
}
void load()
watch(gran, load)

function pickPlatform(p: string) {
  platform.value = p
  group.value = ''
  localStorage.setItem(PLATFORM_KEY, p)
}

const groupName = (id: number) => board.value?.groups.find((g) => g.id === id)?.name ?? `#${id}`
const groupsText = (ids: number[]) => (ids.length > 1 ? `${groupName(ids[0])} 等 ${ids.length} 个分组` : ids.length ? groupName(ids[0]) : '没有分组')
const groupOptions = computed(() => [
  { value: '', label: '全部分组' },
  ...(board.value?.groups ?? []).filter((g) => g.platform === platform.value).map((g) => ({ value: g.id, label: g.name }))
])

// 这个平台的全部账号（默认排序：需要处理的在前，再按请求量；有自定义顺序时按自定义）
const all = computed<AccountRow[]>(() => {
  const list = (board.value?.accounts ?? []).filter((a) => a.platform === platform.value)
  const custom = order.value[platform.value]
  if (custom) return [...list].sort((x, y) => (custom.indexOf(x.id) + 1 || 1e9) - (custom.indexOf(y.id) + 1 || 1e9))
  return [...list].sort((x, y) => Number(y.needs) - Number(x.needs) || y.cols.n24 - x.cols.n24)
})
const sum = computed(() => board.value?.platforms.find((p) => p.platform === platform.value)?.cols)
const needCount = computed(() => all.value.filter((a) => a.needs).length)
const tunedCount = computed(() => all.value.filter((a) => a.rules.length).length)

const sortVal = (a: AccountRow, k: string): number | null => {
  if (k === 'week') return a.week_ok
  if (k === 'last') return a.last_at ? new Date(a.last_at).getTime() / 1000 : a.last_hour
  return COLS.find((c) => c.key === k)?.val(a.cols) ?? null
}
const rows = computed(() => {
  const kw = search.value.trim().toLowerCase()
  let list = all.value.filter(
    (a) =>
      (!group.value || a.group_ids.includes(Number(group.value))) &&
      (!kw || a.name.toLowerCase().includes(kw)) &&
      (!onlyNeeds.value || a.needs) &&
      (!onlyTuned.value || a.rules.length)
  )
  const s = sort.value
  if (s) {
    list = [...list].sort((x, y) => {
      const a = sortVal(x, s.key)
      const b = sortVal(y, s.key)
      return a == null ? 1 : b == null ? -1 : (a - b) * s.dir
    })
  }
  return list
})

function toggleSort(k: string) {
  const s = sort.value
  sort.value = !s || s.key !== k ? { key: k, dir: -1 } : s.dir === -1 ? { key: k, dir: 1 } : null
}

function resetOrder() {
  delete order.value[platform.value]
  localStorage.setItem(ORDER_KEY, JSON.stringify(order.value))
}

// 拖动排序
const dragId = ref<number | null>(null)
const over = ref<{ id: number; after: boolean } | null>(null)
function dragStart(id: number, e: DragEvent) {
  dragId.value = id
  e.dataTransfer?.setData('text/plain', String(id))
  const tr = (e.target as HTMLElement).closest('tr')
  if (tr && e.dataTransfer) e.dataTransfer.setDragImage(tr, 20, 20)
}
function dragOver(id: number, e: DragEvent) {
  if (dragId.value == null) return
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  over.value = { id, after: e.clientY >= r.top + r.height / 2 }
}
function drop(target: number, e: DragEvent) {
  const id = dragId.value
  if (id == null || id === target) return dragEnd()
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const ids = all.value.map((a) => a.id).filter((x) => x !== id)
  ids.splice(ids.indexOf(target) + (e.clientY >= r.top + r.height / 2 ? 1 : 0), 0, id)
  order.value = { ...order.value, [platform.value]: ids }
  sort.value = null
  localStorage.setItem(ORDER_KEY, JSON.stringify(order.value))
  dragEnd()
}
function dragEnd() {
  dragId.value = null
  over.value = null
}

const open = (id: number) => router.push(`${DISPATCH_BASE}/account/${id}`)
</script>
