<!-- [plus] 上游调度台 · 账号详情：一排大数字 + 大状态条；模型列表（同一套列，点开看指标、检测、趋势和同类对比）；所在分组；调整记录 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-account">
      <router-link :to="`${DISPATCH_BASE}/board`" class="text-sm text-primary-600">‹ 上游账号</router-link>
      <div v-if="!d" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
      <template v-else>
        <div class="flex flex-wrap items-start gap-3">
          <div>
            <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ a.name }}</h1>
            <div class="mb-2.5 mt-1 text-sm text-gray-500">{{ platformName(a.platform) }} · {{ groupsText }} · 最近请求 {{ lastText(a.last_at, a.last_hour) }}</div>
            <Tags :rules="a.rules" :probes="a.probes" :groups="a.group_ids.length" :group-name="gname" :is-new="a.is_new" :created-at="a.created_at" :down-models="a.down_models" @probe="probe = { accountId: a.id, itemId: $event }" />
          </div>
          <div class="ml-auto flex items-center gap-3">
            <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
              <button v-for="g in GRANS" :key="g.v" type="button" class="rounded-md px-3 py-1 text-sm" :class="gran === g.v ? 'bg-white font-medium shadow-sm dark:bg-dark-900' : 'text-gray-500'" @click="gran = g.v">{{ g.n }}</button>
            </div>
            <button type="button" class="btn" :class="a.rules.length ? 'btn-secondary' : 'btn-primary'" @click="weight = { accountId: a.id }">权重 · {{ flowText(a.rules, a.group_ids.length, null).text }}</button>
          </div>
        </div>

        <ApplyOff v-if="!applyOn && a.rules.length" :rules="a.rules.length" @changed="load" />

        <div class="card grid grid-flow-col auto-cols-fr divide-x divide-gray-100 dark:divide-dark-700">
          <Kpi label="健康分" :value="String(c.score ?? '--')" :sub="STATUS_META[c.status].label" :tone="LV.score(c.score)" />
          <Kpi label="24 小时请求" :value="num(c.n24)" />
          <Kpi label="错误率" :value="pct1(c.err_pct)" :sub="times(c.failed)" :tone="LV.err(c.err_pct)" />
          <Kpi label="首字耗时" :value="c.ttft == null ? '—' : `${c.ttft.toFixed(1)} 秒`" :tone="LV.ttft(c.ttft)" />
          <Kpi label="输出速度" :value="c.tps == null ? '—' : String(Math.round(c.tps))" :sub="c.tps == null ? '' : 'tok/s'" />
          <Kpi label="缓存命中" :value="c.cache == null ? '—' : `${c.cache.toFixed(1)}%`" :sub="c.cache_read ? tok(c.cache_read) : ''" :tone="LV.cache(c.cache)" />
          <Kpi label="7 天成功" :value="num(a.week_ok)" :sub="`失败 ${num(a.week_fail)}`" />
        </div>
        <div class="card px-5 py-4">
          <StatusBar :bars="a.bars" :gran="gran" :marks="a.marks" tall />
          <div class="mt-2.5 flex justify-between text-xs text-gray-400"><span>{{ gran === 'day' ? '30 天前' : '48 小时前' }}</span><span>现在</span></div>
        </div>

        <div class="px-1 pt-2 text-sm text-gray-500">模型</div>
        <div class="card overflow-x-auto p-0">
          <table class="table min-w-[1250px]">
            <thead><tr><th class="sticky left-0 z-10">模型</th><th>{{ gran === 'day' ? '近 30 天' : '近 48 小时' }}</th><th>状态</th><ColHeads /><th class="sticky right-0 z-10">权重</th></tr></thead>
            <tbody>
              <template v-for="m in d.models" :key="m.model">
                <tr class="cursor-pointer" :class="openModel === m.model ? 'bg-gray-50 dark:bg-dark-800' : ''" data-testid="dispatch-model-row" @click="openModel = openModel === m.model ? '' : m.model">
                  <td class="sticky left-0 z-[1] whitespace-nowrap bg-white dark:bg-dark-900"><span class="mr-1.5 inline-block text-gray-400 transition-transform" :class="openModel === m.model ? 'rotate-90' : ''">›</span><span class="font-mono font-semibold">{{ m.model }}</span></td>
                  <td class="min-w-[300px]"><StatusBar :bars="m.bars" :gran="gran" :marks="marksOf(m.model)" /></td>
                  <td @click.stop><Tags :rules="a.rules" :probes="m.probes" :groups="a.group_ids.length" :group-name="gname" :model="m.model" @probe="probe = { accountId: a.id, itemId: $event, model: m.model }" /></td>
                  <ColCells :c="m.cols" />
                  <td class="sticky right-0 z-[1] bg-white dark:bg-dark-900" @click.stop><FlowButton :rules="a.rules" :groups="a.group_ids.length" :model="m.model" @click="weight = { accountId: a.id, model: m.model }" /></td>
                </tr>
                <tr v-if="openModel === m.model">
                  <td colspan="9" class="bg-gray-50/60 dark:bg-dark-800/40">
                    <ModelPanel :account-id="a.id" :model="m.model" :account-name="a.name" :marks="marksOf(m.model)" @probe="probe = { accountId: a.id, itemId: $event, model: m.model }" @open-account="openAccount" />
                  </td>
                </tr>
              </template>
              <tr v-if="!d.models.length"><td colspan="9" class="py-10 text-center text-sm text-gray-400">近 24 小时没有请求，也没有检测过</td></tr>
            </tbody>
          </table>
        </div>
        <div v-if="d.quiet" class="px-1 text-xs text-gray-400">另有 {{ d.quiet }} 个模型请求很少</div>

        <div class="px-1 pt-2 text-sm text-gray-500">所在分组</div>
        <div class="card overflow-x-auto p-0">
          <table class="table">
            <thead><tr><th>分组</th><th>近 24 小时</th><th>健康分</th><th>承接流量</th><th>同组账号</th></tr></thead>
            <tbody>
              <tr v-for="g in d.groups" :key="g.group_id">
                <td class="whitespace-nowrap font-semibold">{{ g.name }}</td>
                <td class="min-w-[200px]"><StatusBar :bars="g.bars" fixed /></td>
                <td class="whitespace-nowrap"><div class="text-base font-semibold tabular-nums"><span :class="LV.score(g.score)">{{ g.score ?? '--' }}</span><small class="text-xs font-normal text-gray-400">/100</small></div><div class="text-xs text-gray-500">24h {{ times(g.n24) }}</div></td>
                <td class="whitespace-nowrap"><span class="mr-2 inline-block h-1.5 w-28 overflow-hidden rounded bg-gray-200 align-middle dark:bg-dark-600"><i class="block h-full rounded bg-gray-500" :style="{ width: `${g.pct}%` }" /></span><b class="tabular-nums">{{ pct(g.pct) }}</b></td>
                <td class="text-sm text-gray-500">{{ g.others.filter((o) => o.pct > 0).map((o) => `${aname(o.account_id)} ${pct(o.pct)}`).join('  ') || '只有它一个账号' }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <template v-if="d.records.length">
          <div class="px-1 pt-2 text-sm text-gray-500">调整记录</div>
          <div class="card overflow-hidden p-0">
            <RecordCard v-for="r in d.records" :key="r.id" :rec="r" :names="d.names" @undo="undo" @move-away="moveAway" />
          </div>
        </template>
      </template>
    </div>
    <WeightDialog :target="weight" @close="weight = null" @saved="load" />
    <ProbeDialog :target="probe" @close="probe = null" @changed="load" @open-run="runId = $event" />
    <RunDialog :run-id="runId" @close="runId = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type AccountDetail, type Gran } from '../api'
import { DISPATCH_BASE } from '../routes'
import ApplyOff from '../shared/ApplyOff.vue'
import ColCells from '../shared/ColCells.vue'
import ColHeads from '../shared/ColHeads.vue'
import FlowButton from '../shared/FlowButton.vue'
import Kpi from '../shared/Kpi.vue'
import ModelPanel from '../shared/ModelPanel.vue'
import ProbeDialog from '../shared/ProbeDialog.vue'
import RecordCard from '../shared/RecordCard.vue'
import RunDialog from '../shared/RunDialog.vue'
import StatusBar from '../shared/StatusBar.vue'
import Tags from '../shared/Tags.vue'
import WeightDialog, { type WeightTarget } from '../shared/WeightDialog.vue'
import { LV, STATUS_META, flowText, lastText, num, pct, pct1, platformName, times, tok } from '../shared/format'

const GRANS = [{ v: 'hour' as const, n: '按小时' }, { v: 'day' as const, n: '按天' }]
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const d = ref<AccountDetail | null>(null)
const error = ref('')
const gran = ref<Gran>('hour')
const openModel = ref('')
const applyOn = ref(true)
const weight = ref<WeightTarget | null>(null)
const probe = ref<{ accountId: number; itemId: number; model?: string } | null>(null)
const runId = ref<number | null>(null)

const id = computed(() => Number(route.params.id))
const a = computed(() => d.value!.account)
const c = computed(() => d.value!.account.cols)
const gname = (gid: number) => d.value?.names.groups[String(gid)] ?? `#${gid}`
const aname = (aid: number) => d.value?.names.accounts[String(aid)] ?? `#${aid}`
const groupsText = computed(() => {
  const ids = a.value.group_ids
  return ids.length > 1 ? `${gname(ids[0])} 等 ${ids.length} 个分组` : ids.length ? gname(ids[0]) : '没有分组'
})
const marksOf = (model: string) => (d.value?.records ?? []).filter((r) => !r.model || r.model === model).map((r) => r.created_at)

async function load() {
  try {
    const [detail, cfg] = await Promise.all([dispatchApi.account(id.value, gran.value), dispatchApi.config()])
    d.value = detail
    applyOn.value = cfg.config.apply
    error.value = ''
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  }
}
watch([id, gran], () => {
  if (route.params.id) void load()
}, { immediate: true })

function openAccount(aid: number) {
  openModel.value = ''
  void router.push(`${DISPATCH_BASE}/account/${aid}`)
}

async function undo(rid: number) {
  try {
    await dispatchApi.undo(rid)
    appStore.showSuccess('已恢复原样')
    await load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  }
}

function moveAway(p: { accountId: number; models: string[] }) {
  weight.value = { accountId: p.accountId, model: p.models[0], also: p.models.slice(1), note: '这些模型在它身上全部失败', pauseAll: true }
}
</script>
