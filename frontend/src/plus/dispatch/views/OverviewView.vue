<!-- [plus] 上游调度台 · 总览：每个分组 × 账号现在怎么样；列 = 已启用的维度；每行可以就地调整 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-overview">
      <div class="flex flex-wrap items-center gap-3">
        <div class="relative w-full sm:w-56">
          <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
          <input v-model="q.search" type="text" class="input pl-10" placeholder="搜索账号" />
        </div>
        <PSelect v-model="q.platform" width-class="w-36" :options="[{ value: '', label: '全部平台' }, ...platforms.map((p) => ({ value: p, label: platformName(p) }))]" />
        <PSelect v-model="q.group" width-class="w-44" :options="[{ value: 0, label: '全部分组' }, ...(data?.groups ?? []).map((g) => ({ value: g.id, label: g.name }))]" />
        <PSelect v-model="q.level" width-class="w-32" :options="LEVEL_OPTIONS" />
        <div class="ml-auto flex items-center gap-3">
          <span v-if="data" class="hidden text-xs text-gray-400 lg:inline">{{ fullTime(data.as_of) }}</span>
          <DiagnoseButton @done="load" />
        </div>
      </div>
      <ModeNotice />

      <div v-if="!data" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
      <template v-else>
        <div class="grid grid-cols-3 gap-3 md:grid-cols-6">
          <KPI :n="data.counts.bad ?? 0" label="异常" tone="red" @click="q.level = 'bad'" />
          <KPI :n="data.counts.warn ?? 0" label="可疑" tone="amber" @click="q.level = 'warn'" />
          <KPI :n="data.counts.ok ?? 0" label="正常" tone="green" @click="q.level = 'ok'" />
          <KPI :n="data.counts.few ?? 0" label="暂无结论" @click="q.level = 'few'" />
          <KPI :n="data.faults" label="检测失败" :tone="data.faults ? 'amber' : ''" @click="go('events')" />
          <KPI :n="data.adjustments" label="调整生效中" :tone="data.adjustments ? 'blue' : ''" @click="go('adjust')" />
        </div>

        <div v-if="data.coverage_gaps?.length" class="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
          <Icon name="exclamationTriangle" size="sm" />
          <span>门槛维度未覆盖：<template v-for="(g, i) in data.coverage_gaps" :key="i">{{ i ? '；' : '' }}{{ g.dimension }}（{{ g.accounts.join('、') }}）</template></span>
          <router-link class="ml-auto font-medium hover:underline" :to="{ path: `${BASE}/config`, query: { tab: 'plans' } }">补计划</router-link>
        </div>

        <div v-if="!groups.length" class="py-16 text-center text-sm text-gray-400">没有符合条件的账号</div>
        <section v-for="g in groups" :key="g.id" class="card overflow-hidden p-0" data-testid="dispatch-group-card">
          <div class="flex flex-wrap items-center gap-2.5 border-b border-gray-100 px-4 py-3 dark:border-dark-700">
            <span class="font-medium text-gray-900 dark:text-white">{{ g.name }}</span>
            <span class="rounded-md px-1.5 py-0.5 text-xs" :class="PLATFORM_CLASS[g.platform]">{{ platformName(g.platform) }}</span>
            <span class="text-xs text-gray-400">{{ g.requests_24h.toLocaleString() }} 次请求 / 24h</span>
            <span v-if="g.suggest_summary.startsWith('建议')" class="ml-auto min-w-0 max-w-[50%] truncate text-xs text-amber-700" :title="g.suggest_summary">{{ g.suggest_summary }}</span>
            <router-link :class="g.suggest_summary.startsWith('建议') ? '' : 'ml-auto'" class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" title="指标对比" :to="{ path: `${BASE}/compare`, query: { group: g.id } }"><Icon name="chartBar" size="sm" /></router-link>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800">
                  <th class="px-3 py-2 font-normal">账号</th>
                  <th class="px-3 py-2 font-normal">流量占比</th>
                  <th class="px-3 py-2 font-normal">诊断结论</th>
                  <th v-for="d in dims" :key="d.id" class="px-3 py-2 font-normal">
                    {{ d.name }}
                  </th>
                  <th class="sticky right-0 bg-gray-50 px-3 py-2 font-normal shadow-[-6px_0_6px_-6px_rgba(0,0,0,0.12)] dark:bg-dark-800">调度状态</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in g.rows ?? []" :key="r.account_id" class="border-t border-gray-100 dark:border-dark-700" data-testid="dispatch-row">
                  <td class="whitespace-nowrap px-3 py-2.5">
                    <router-link class="font-semibold text-primary-700 hover:underline" :to="accountLink(g.id, r.account_id)">{{ r.name }}</router-link>
                    <span v-if="r.is_new" class="ml-1 rounded bg-violet-50 px-1 text-[11px] text-violet-700">新接入</span>
                    <span v-if="!r.serving" class="ml-1 rounded bg-gray-100 px-1 text-[11px] text-gray-500">调度已关闭</span>
                    <div class="text-[11px] text-gray-400">倍率 {{ r.rate }}<template v-if="r.also_in?.length"> · 也在 {{ r.also_in.join('、') }}</template></div>
                  </td>
                  <td class="px-3 py-2.5"><ShareBar :value="r.share" /></td>
                  <td class="min-w-[240px] max-w-[320px] px-3 py-2.5 text-xs leading-5" :class="r.worst === 'bad' ? 'text-red-600' : r.worst === 'warn' ? 'text-amber-700' : 'text-gray-600 dark:text-dark-200'">{{ r.one_line }}</td>
                  <td v-for="d in dims" :key="d.id" class="min-w-[104px] cursor-pointer px-3 py-2.5 hover:bg-primary-50/60 dark:hover:bg-dark-700" :title="r.dims[d.id]?.long" @click="router.push(accountLink(g.id, r.account_id, d.id))">
                    <template v-if="r.dims[d.id]">
                      <LevelPill :level="r.dims[d.id].level" :text="r.dims[d.id].level === 'info' ? r.dims[d.id].short : r.dims[d.id].muted ? '正常（已标误报）' : r.dims[d.id].no_ref ? '无参照' : undefined" />
                      <div v-if="r.dims[d.id].level !== 'info'" class="mt-1 max-w-[150px] truncate text-[11px] text-gray-500">{{ cellText(r.dims[d.id]) }}</div>
                    </template>
                  </td>
                  <td class="sticky right-0 whitespace-nowrap bg-white px-3 py-2.5 shadow-[-6px_0_6px_-6px_rgba(0,0,0,0.12)] dark:bg-dark-800">
                    <div v-if="r.adjustment" class="mb-1">
                      <span class="rounded bg-blue-50 px-1.5 py-0.5 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">{{ adjustText(r.adjustment.action, r.adjustment.factor) }}</span>
                      <span v-if="data.mode !== 'manual'" class="ml-1 text-[11px] text-gray-400">未生效</span>
                    </div>
                    <div v-if="r.suggest" class="mb-1 text-[11px] text-amber-700">建议 {{ r.suggest.action === 'pause' ? '暂停' : '×' + r.suggest.factor }}</div>
                    <div class="flex gap-1">
                      <button type="button" class="btn btn-sm" :class="r.suggest ? 'btn-primary' : 'btn-secondary'" data-testid="dispatch-adjust-btn" @click="openAdjust(g, r)">调整</button>
                      <button v-if="r.direct" type="button" class="btn btn-secondary btn-sm" data-testid="dispatch-checkup-btn" @click="checkupTarget = { id: r.account_id, name: r.name, platform: g.platform, direct: r.direct }">立即体检</button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
        <div v-if="data.hidden || showAll" class="text-center text-xs text-gray-400">
          <template v-if="!showAll">另有 {{ data.hidden }} 个账号调度已关闭，未列出 · </template>
          <button type="button" class="text-primary-600 hover:underline" @click="showAll = !showAll; load()">{{ showAll ? '隐藏调度已关闭的账号' : '显示' }}</button>
        </div>
      </template>
    </div>
    <AdjustDialog :target="adjustTarget" @close="adjustTarget = null" @done="adjustTarget = null; load()" />
    <CheckupDialog :target="checkupTarget" @close="checkupTarget = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type DimResult, type Overview, type OverviewGroup, type OverviewRow } from '../api'
import { DISPATCH_BASE as BASE } from '../routes'
import AdjustDialog, { type AdjustTarget } from '../shared/AdjustDialog.vue'
import CheckupDialog, { type CheckupTarget } from '../shared/CheckupDialog.vue'
import KPI from '../shared/KPI.vue'
import LevelPill from '../shared/LevelPill.vue'
import Icon from '@/components/icons/Icon.vue'
import DiagnoseButton from '../shared/DiagnoseButton.vue'
import ModeNotice from '../shared/ModeNotice.vue'
import PSelect from '../shared/PSelect.vue'
import ShareBar from '../shared/ShareBar.vue'
import { adjustText, fullTime, PLATFORM_CLASS, platformName } from '../shared/format'

const router = useRouter()
const data = ref<Overview | null>(null)
const error = ref('')
const LEVEL_OPTIONS = [{ value: '', label: '全部状态' }, { value: 'bad', label: '异常' }, { value: 'warn', label: '可疑' }, { value: 'ok', label: '正常' }, { value: 'few', label: '暂无结论' }]
const q = reactive({ search: '', platform: '', group: 0, level: '' })
const showAll = ref(false)
const adjustTarget = ref<AdjustTarget | null>(null)
const checkupTarget = ref<CheckupTarget | null>(null)

const dims = computed(() => (data.value?.dimensions ?? []).filter((d) => d.enabled))
const platforms = computed(() => [...new Set((data.value?.groups ?? []).map((g) => g.platform))])
// 筛选在前端做：分组 / 平台 / 状态 / 账号名；筛完没有账号的分组不显示
const groups = computed(() => (data.value?.groups ?? [])
  .filter((g) => (!q.platform || g.platform === q.platform) && (!q.group || g.id === q.group))
  .map((g) => ({ ...g, rows: (g.rows ?? []).filter((r) => (!q.level || r.worst === q.level) && (!q.search || r.name.toLowerCase().includes(q.search.trim().toLowerCase()))) }))
  .filter((g) => g.rows.length))

async function load() {
  try {
    data.value = await dispatchApi.overview('', showAll.value)
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}
void load()

function go(page: string) {
  void router.push(`${BASE}/${page}`)
}

function accountLink(groupId: number, accountId: number, dim?: number) {
  return { path: `${BASE}/account/${accountId}`, query: { group: groupId, ...(dim ? { dim } : {}) } }
}

function cellText(r: DimResult) {
  if (r.level === 'none') return r.long.replace(/^未检测：/, '')
  return r.short + (r.model ? ` · ${r.model}` : '')
}

function openAdjust(g: OverviewGroup, r: OverviewRow) {
  adjustTarget.value = {
    groupId: g.id, groupName: g.name, accountId: r.account_id, accountName: r.name, worst: r.worst, oneLine: r.one_line,
    current: r.adjustment ?? null, suggest: r.suggest ?? null,
    names: Object.fromEntries((g.rows ?? []).map((x) => [String(x.account_id), x.name]))
  }
}
</script>
