<!-- [plus] 上游调度台 · 单个账号：先给每个维度的结论，展开看每个检测项的依据。依据按检测方式展示，页面里没有写死的题目名。 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-account">
      <div v-if="!data" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
      <template v-else>
        <section class="card p-5">
          <div class="flex flex-wrap items-start gap-3">
            <div class="min-w-0">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ data.account.name }}</h2>
              <div class="mt-1 text-sm text-gray-500">{{ headline }}</div>
            </div>
            <div class="ml-auto flex flex-wrap items-center gap-2">
              <PSelect v-if="data.groups?.length > 1" :model-value="groupId" width-class="w-40" :options="data.groups.map((g: any) => ({ value: g.id, label: g.name }))" @change="(v) => switchGroup(String(v))" />
              <router-link class="btn btn-secondary" :to="{ path: `${BASE}/runs`, query: { account: data.account.id } }">检测记录</router-link>
              <button v-if="data.account.direct" type="button" class="btn btn-secondary" data-testid="dispatch-account-checkup" @click="checkupTarget = { id: data.account.id, name: data.account.name, platform: data.account.platform, direct: data.account.direct }">立即体检</button>
              <DiagnoseButton @done="load" />
              <button type="button" class="btn btn-primary" data-testid="dispatch-account-adjust" @click="openAdjust">调整</button>
            </div>
          </div>
          <div class="mt-4 flex flex-wrap items-center gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
            <LevelPill :level="data.worst" />
            <span class="text-sm" :class="data.worst === 'bad' ? 'text-red-600' : data.worst === 'warn' ? 'text-amber-700' : 'text-gray-700 dark:text-dark-100'">{{ data.one_line }}</span>
            <span v-if="data.adjustment" class="ml-auto rounded-md bg-blue-50 px-2 py-1 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300" :title="data.adjustment.reason">{{ adjustText(data.adjustment.action, data.adjustment.factor) }} · {{ agoText(data.adjustment.created_at) }}</span>
            <span v-if="data.suggest && data.suggest.action !== 'normal' && !data.suggest.settled" class="rounded-md bg-amber-50 px-2 py-1 text-xs text-amber-800" :class="data.adjustment ? '' : 'ml-auto'">建议{{ data.suggest.action === 'pause' ? '暂停' : ' ×' + data.suggest.factor }}</span>
          </div>
        </section>

        <div v-if="data.models?.length > 1" class="flex flex-wrap items-center gap-2 text-sm">
          <button v-for="m in data.models" :key="m.model" type="button" class="rounded-lg border px-3 py-1 text-xs" :class="m.model === data.model ? 'border-primary-500 bg-primary-50 text-primary-700' : 'border-gray-200'" @click="switchModel(m.model)">
            {{ m.model }}<i v-if="m.worst === 'bad' || m.worst === 'warn'" class="ml-1 inline-block h-2 w-2 rounded-full" :style="{ background: LEVEL_DOT[m.worst as Level] }" />
          </button>
        </div>
        <div v-else-if="!data.models?.length" class="py-8 text-center text-sm text-gray-400">暂无结论</div>

        <!-- 维度 -->
        <div v-for="blk in data.dims" :key="blk.dimension.id" class="overflow-hidden rounded-xl border bg-white dark:bg-dark-800" :class="blk.result.level === 'bad' ? 'border-red-300' : blk.result.level === 'warn' ? 'border-amber-300' : 'border-gray-200 dark:border-dark-700'" data-testid="dispatch-dim-block">
          <div class="flex cursor-pointer flex-wrap items-center gap-2.5 px-4 py-3" :class="blk.result.level === 'bad' ? 'bg-red-50 dark:bg-red-900/10' : blk.result.level === 'warn' ? 'bg-amber-50 dark:bg-amber-900/10' : ''" @click="opened[blk.dimension.id] = !isOpen(blk)">
            <b class="min-w-[56px]">{{ blk.dimension.name }}</b>
            <span class="rounded-full px-2 text-xs" :class="ROLE_CLASS[blk.dimension.role as Role]">{{ ROLE_TEXT[blk.dimension.role as Role] }}</span>
            <LevelPill :level="blk.result.level" :text="blk.result.level === 'info' ? blk.result.short : blk.result.muted ? '正常（已标误报）' : undefined" />
            <span class="min-w-[220px] flex-1 text-sm text-gray-600 dark:text-dark-200">{{ headOf(blk) }}</span>
            <Icon name="chevronDown" size="sm" class="text-gray-400 transition-transform" :class="isOpen(blk) ? 'rotate-180' : ''" />
          </div>
          <div v-if="isOpen(blk)" class="border-t border-gray-100 px-4 pb-3 dark:border-dark-700">
            <div v-if="!blk.items?.length" class="py-3 text-sm text-gray-400">没有检测项</div>
            <div v-for="e in blk.items ?? []" :key="e.item.id" class="border-b border-dashed border-gray-200 py-3 last:border-b-0 dark:border-dark-700">
              <div class="mb-2 flex flex-wrap items-center gap-2">
                <b>{{ e.item.name }}</b>
                                <span v-if="!e.judged" class="rounded-full bg-gray-100 px-2 text-xs text-gray-500">仅展示</span>
                <LevelPill :level="e.verdict.level" :text="e.verdict.level === 'info' ? e.verdict.short : undefined" />
                <span class="text-xs text-gray-400">{{ planLine(e) }}</span>
              </div>
              <div v-if="e.verdict.level === 'none'" class="text-sm text-gray-500">
                {{ e.verdict.long }}
                <router-link v-if="e.spec.active" class="ml-1 text-primary-600 hover:underline" :to="{ path: `${BASE}/config`, query: { tab: 'plans', item: e.item.id } }">计划</router-link>
              </div>
              <template v-else>
                <div class="text-sm" :class="e.verdict.level === 'bad' ? 'text-red-600' : e.verdict.level === 'warn' ? 'text-amber-700' : 'text-gray-700 dark:text-dark-100'">{{ e.verdict.long }}</div>

                <!-- 流量统计：参照 + 24 小时小图 -->
                <div v-if="e.item.type === 'traffic'" class="mt-2 grid gap-4 lg:grid-cols-[1fr_420px]">
                  <div class="space-y-1 text-xs text-gray-500">
                    <div v-if="e.verdict.evidence?.reference_label">参照：{{ e.verdict.evidence.reference_label }}<template v-if="e.verdict.evidence.peers?.length">（{{ peerNames(e.verdict.evidence.peers) }}）</template></div>
                    <div :title="ruleText(e)">规则：{{ ruleText(e) }}</div>
                    <router-link class="mt-1 inline-block text-primary-600 hover:underline" :to="{ path: `${BASE}/compare`, query: { group: groupId, model: data.model, item: e.item.id, highlight: data.account.id } }">同类对比</router-link>
                  </div>
                  <LineChart v-if="e.series" :lines="seriesLines(e)" :labels="e.series.map((p: any) => hhmm(p.t))" :fmt="(v: number) => fmtMetric(e.metric, v)" :height="140" :legend="false" />
                </div>

                <!-- 主动检测：最近 10 次 -->
                <div v-if="e.spec.active" class="mt-2">
                  <div class="mb-1 text-xs text-gray-500">最近 {{ e.runs?.length ?? 0 }} 次</div>
                  <div v-if="e.spec.gallery" class="flex gap-2 overflow-x-auto pb-1">
                    <RunThumb v-for="r in e.runs" :key="r.id" :run="r" @open="runId = r.id" />
                  </div>
                  <div v-else class="flex flex-wrap gap-1">
                    <button v-for="r in e.runs" :key="r.id" type="button" class="rounded border px-1.5 py-0.5 text-[11px]" :class="TONE_CLASS[outcomeOf(e.spec, r.outcome).tone]" :title="r.summary" @click="runId = r.id">
                      {{ runChip(e, r) }} <span class="text-gray-400">{{ hhmm(r.t) }}</span>
                    </button>
                  </div>
                  <div v-if="e.verdict.evidence?.distribution?.length" class="mt-2 max-w-md space-y-1">
                    <div class="text-xs text-gray-500">识别结果分布</div>
                    <div v-for="d in e.verdict.evidence.distribution" :key="d.model" class="flex items-center gap-2 text-xs">
                      <span class="w-44 truncate font-mono">{{ d.model }}</span>
                      <span class="h-2 flex-1 overflow-hidden rounded bg-gray-100"><i class="block h-full" :class="d.model === e.verdict.evidence.distribution[0].model && e.verdict.level !== 'ok' ? 'bg-red-500' : 'bg-primary-500'" :style="{ width: (d.n / distTotal(e)) * 100 + '%' }" /></span>
                      <span class="w-10 text-right">{{ d.n }} 次</span>
                    </div>
                  </div>
                  <div v-if="e.verdict.evidence?.peers?.length && e.item.type === 'graded'" class="mt-2 text-xs text-gray-500">
                    同组对比：
                    <span v-for="p in e.verdict.evidence.peers" :key="p.account_id" class="mr-3">{{ accountName(p.account_id) }} {{ p.pass }}/{{ p.n }}</span>
                  </div>
                  <div class="mt-2 flex gap-4 text-xs">
                    <router-link v-if="e.spec.comparable" class="text-primary-600 hover:underline" :to="{ path: `${BASE}/compare`, query: { group: groupId, model: data.model, item: e.item.id, highlight: data.account.id } }">同类对比</router-link>
                    <router-link class="text-primary-600 hover:underline" :to="{ path: `${BASE}/runs`, query: { item: e.item.id, account: data.account.id } }">检测记录</router-link>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>

        <div class="grid gap-4 lg:grid-cols-2">
          <div class="card p-4">
            <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">状态变化 <span class="font-normal text-gray-400">48 小时</span></div>
            <div v-if="!data.changes?.length" class="text-sm text-gray-400">无</div>
            <div v-else class="space-y-1.5 border-l-2 border-gray-200 pl-3">
              <div v-for="c in data.changes" :key="c.id" class="relative text-sm">
                <i class="absolute -left-[19px] top-1.5 h-2.5 w-2.5 rounded-full" :style="{ background: LEVEL_DOT[c.to as Level] }" />
                <span class="text-xs text-gray-400">{{ fullTime(c.created_at) }}</span>
                {{ LEVEL_TEXT[c.from as Level] }} → <b>{{ LEVEL_TEXT[c.to as Level] }}</b> · {{ c.model }} · {{ c.text }}
              </div>
            </div>
          </div>
          <div class="card p-4">
            <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">调整历史</div>
            <div v-if="!data.adjust_logs?.length" class="text-sm text-gray-400">无</div>
            <div v-for="l in data.adjust_logs ?? []" :key="l.id" class="flex flex-wrap items-center gap-2 border-b border-dashed border-gray-100 py-1.5 text-sm last:border-0">
              <span class="text-xs text-gray-400">{{ fullTime(l.created_at) }}</span>
              <span>{{ adjustText(l.from_action, l.from_factor) }} → <b>{{ adjustText(l.to_action, l.to_factor) }}</b></span>
              <span class="text-xs text-gray-500">{{ l.operator }} · {{ l.reason }}</span>
              <span class="flex-1" />
              <router-link class="text-xs text-primary-600 hover:underline" :to="{ path: `${BASE}/adjust`, query: { tab: 'history', log: l.id } }">效果</router-link>
            </div>
          </div>
        </div>
      </template>
    </div>
    <RunDetailDialog :run-id="runId" @close="runId = null" @rerun="runId = null" />
    <CheckupDialog :target="checkupTarget" @close="checkupTarget = null" />
    <AdjustDialog :target="adjustTarget" @close="adjustTarget = null" @done="adjustTarget = null; load()" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Level, type Role } from '../api'
import { DISPATCH_BASE as BASE } from '../routes'
import AdjustDialog, { type AdjustTarget } from '../shared/AdjustDialog.vue'
import CheckupDialog, { type CheckupTarget } from '../shared/CheckupDialog.vue'
import PSelect from '../shared/PSelect.vue'
import LevelPill from '../shared/LevelPill.vue'
import LineChart from '../shared/LineChart.vue'
import Icon from '@/components/icons/Icon.vue'
import DiagnoseButton from '../shared/DiagnoseButton.vue'
import RunDetailDialog from '../shared/RunDetailDialog.vue'
import RunThumb from '../shared/RunThumb.vue'
import { adjustText, agoText, everyText, fmtMetric, fullTime, LEVEL_DOT, LEVEL_TEXT, outcomeOf, platformName, ROLE_CLASS, ROLE_TEXT, TONE_CLASS } from '../shared/format'
import { ruleSentence } from '../shared/rules'
import { meta, loadMeta } from '../shared/meta'

const route = useRoute()
const router = useRouter()
const data = ref<any>(null)
const error = ref('')
const runId = ref<number | null>(null)
const opened = reactive<Record<number, boolean>>({})
const adjustTarget = ref<AdjustTarget | null>(null)
const checkupTarget = ref<CheckupTarget | null>(null)

const accountId = computed(() => Number(route.params.id))
const groupId = computed(() => Number(route.query.group) || data.value?.group?.id || 0)

const headline = computed(() => {
  const d = data.value
  if (!d) return ''
  const parts = [d.group.name, platformName(d.account.platform), d.account.base_url, `流量 ${d.share}%`, `倍率 ${d.account.rate_multiplier}`]
  if (d.aliases?.length > 1) parts.push(`与 ${d.aliases.length - 1} 个账号同凭证`)
  return parts.join(' · ')
})

async function load() {
  error.value = ''
  try {
    void loadMeta()
    data.value = await dispatchApi.account(accountId.value, Number(route.query.group) || 0, String(route.query.model ?? ''))
    const dim = Number(route.query.dim)
    if (dim) opened[dim] = true
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}
watch(() => [route.params.id, route.query.group, route.query.model], load, { immediate: true })

function switchGroup(g: string) {
  void router.replace({ query: { ...route.query, group: g, model: undefined } })
}
function switchModel(m: string) {
  void router.replace({ query: { ...route.query, model: m } })
}

const isOpen = (blk: any) => opened[blk.dimension.id] ?? (blk.result.level === 'bad' || blk.result.level === 'warn')
function headOf(blk: any) {
  const r = blk.result
  if (!r.item_id) return blk.items?.length ? '这个维度的检测项都没有覆盖这个账号' : '这个维度还没有检测项'
  return `${data.value.item_names?.[r.item_id] ?? ''}：${r.long}`
}
function planLine(e: any) {
  if (!e.spec.active) return e.item.type === 'traffic' ? '从真实流量统计，每 5 分钟汇总' : '读取账号配置'
  if (!e.plan) return ''
  const last = e.runs?.[0]
  return `${platformName(e.plan.platform)} 计划 · ${everyText(e.plan.interval_minutes)}${last ? ` · 上次 ${hhmm(last.t)}` : ''}${e.next_run ? ` · 下次约 ${hhmm(e.next_run)}` : ''}`
}
const hhmm = (t: string) => {
  const d = new Date(t)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
function seriesLines(e: any) {
  const pts = e.series as any[]
  const color = e.verdict.level === 'bad' ? '#dc2626' : e.verdict.level === 'warn' ? '#d97706' : '#0d9488'
  return [
    { id: 'v', name: '它', color, width: 2.2, values: pts.map((p) => p.v ?? null) },
    { id: 'ref', name: '参照', color: '#64748b', dashed: true, values: pts.map((p) => p.ref ?? null) },
    { id: 'warn', name: '超过就算可疑', color: '#f59e0b', dashed: true, width: 1, values: pts.map((p) => p.warn ?? null) }
  ]
}
function ruleText(e: any) {
  return ruleSentence(e.item, meta.value)
}
function runChip(e: any, r: any) {
  const oc = outcomeOf(e.spec, r.outcome)
  if (e.item.type === 'graded') return r.outcome === 'error' ? '! 出错' : `${r.detail?.answer || '没答出'} ${r.outcome === 'pass' ? '✓' : '✗'}`
  if (e.item.type === 'fingerprint') return r.outcome === 'pass' ? '✓ 一致' : r.outcome === 'fail' ? `✗ ${r.detail?.prediction}` : oc.label
  return oc.label
}
const distTotal = (e: any) => e.verdict.evidence.distribution.reduce((a: number, d: any) => a + d.n, 0) || 1
function accountName(id: number) {
  return meta.value?.accounts.find((a) => a.id === id)?.name ?? `#${id}`
}
const peerNames = (ids: number[]) => ids.map(accountName).join('、')

function openAdjust() {
  const d = data.value
  adjustTarget.value = {
    groupId: d.group.id, groupName: d.group.name, accountId: d.account.id, accountName: d.account.name, worst: d.worst, oneLine: d.one_line,
    current: d.adjustment ?? null, suggest: d.suggest ?? null,
    names: Object.fromEntries((meta.value?.accounts ?? []).map((a) => [String(a.id), a.name]))
  }
}
</script>
