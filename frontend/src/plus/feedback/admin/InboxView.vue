<!-- [plus] 管理端「反馈处理」：沿用项目标准列表页（筛选 + 表格 + 分页） -->
<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="w-full sm:w-56">
            <SearchInput v-model="search.content" placeholder="内容" data-testid="search-content" @search="apply" />
          </div>
          <div class="w-full sm:w-56">
            <SearchInput v-model="search.user" placeholder="用户邮箱、用户名或 ID" data-testid="search-user" @search="apply" />
          </div>
          <div class="w-full sm:w-32">
            <SearchInput v-model="search.id" placeholder="反馈 ID" data-testid="search-id" @search="apply" />
          </div>
          <Select v-model="q.status" :options="statusOptions" class="w-36" />
          <Select v-model="q.kind" :options="kindOptions" class="w-32" />
          <Select v-model="q.scenario" :options="scenarioOptions" class="w-40" searchable />
          <Select v-model="q.severity" :options="severityOptions" class="w-32" />
          <Select v-if="tags.length || q.tag" v-model="q.tag" :options="tagOptions" class="w-32" searchable />
          <Select v-model="q.days" :options="dayOptions" class="w-32" data-testid="filter-days" />

          <div class="flex flex-1 justify-end">
            <button type="button" class="btn btn-secondary" :disabled="loading" title="刷新" @click="load">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>

        <div
          v-if="selected.length"
          class="mt-3 flex flex-wrap items-center gap-2 rounded-xl border border-primary-200 bg-primary-50 p-3 dark:border-primary-800 dark:bg-primary-900/20"
        >
          <span class="mr-2 text-sm font-medium text-primary-800 dark:text-primary-200">已选 {{ selected.length }} 条</span>
          <Select
            :model-value="null"
            :options="batchSeverityOptions"
            class="w-28"
            placeholder="定级"
            :disabled="batching"
            data-testid="batch-severity"
            @update:model-value="(v) => v && runBatch({ severity: v as Severity })"
          />
          <Select
            :model-value="null"
            :options="tagChoices"
            class="w-32"
            placeholder="加标签"
            searchable
            creatable
            creatable-prefix="新标签："
            search-placeholder="搜索或输入新标签"
            :disabled="batching"
            data-testid="batch-tag"
            @update:model-value="batchTag"
          />
          <button
            type="button"
            class="btn btn-danger btn-sm"
            :disabled="batching || !declinable.length"
            :title="declinable.length ? '' : '所选反馈都已结案'"
            data-testid="batch-decline"
            @click="openDecline"
          >
            不处理 ({{ declinable.length }})
          </button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="batching" @click="selected = []">取消选择</button>
          <span v-if="batching" class="text-sm text-primary-700 dark:text-primary-300">处理中…</span>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="items"
          :loading="loading"
          row-key="id"
          :sticky-first-column="false"
          selectable
          :selected-keys="selected"
          clickable-rows
          @update:selected-keys="(k) => (selected = k as number[])"
          @row-click="(row) => router.push(`/admin/feedback/${row.id}`)"
        >
          <template #cell-summary="{ row }">
            <div class="flex min-w-[280px] max-w-xl items-start gap-2.5">
              <span class="mt-0.5 shrink-0 text-base leading-none">{{ scenarioIcon(row.scenario) }}</span>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span class="truncate text-sm text-gray-900 dark:text-white">{{ row.summary }}</span>
                  <span v-if="row.has_attachments" class="shrink-0 text-gray-400" title="有图片"><Icon name="document" size="sm" /></span>
                  <span v-if="(row.impact ?? 1) > 1" class="badge badge-primary shrink-0" title="影响人数">×{{ row.impact }}</span>
                </div>
                <div class="mt-1 flex items-center gap-1.5 text-xs text-gray-400 dark:text-dark-400">
                  <span class="tabular-nums">#{{ row.id }}</span>
                  <span>·</span>
                  <button type="button" class="truncate hover:text-primary-600" title="只看此用户" @click.stop="filterUser(row.email)">{{ row.email }}</button>
                  <span v-for="t in (row.tags ?? []).slice(0, 3)" :key="t" class="badge badge-gray !px-1.5 !py-0">{{ t }}</span>
                </div>
              </div>
            </div>
          </template>

          <template #cell-kind="{ row }">
            <span class="text-sm text-gray-600 dark:text-dark-300">{{ KIND_LABEL[row.kind as Kind] }} · {{ scenarioLabel(row.scenario) }}</span>
          </template>

          <template #cell-severity="{ value }">
            <span v-if="value" class="inline-flex items-center gap-1.5 text-sm text-gray-700 dark:text-dark-200">
              <span class="h-2 w-2 rounded-full" :class="SEVERITY_META[value as Exclude<Severity, ''>].dot" />
              {{ SEVERITY_META[value as Exclude<Severity, ''>].label }}
            </span>
            <span v-else class="text-sm text-gray-300 dark:text-dark-500">—</span>
          </template>

          <template #cell-status="{ row }">
            <div class="flex items-center gap-2">
              <StatusPill :status="row.status" />
              <span v-if="row.reward_points" class="text-xs font-medium text-purple-600 dark:text-purple-400">+{{ row.reward_points }}</span>
            </div>
          </template>

          <template #cell-last_activity_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400" :title="fullTime(value)">{{ ago(value) }}</span>
          </template>

          <template #empty>
            <EmptyState v-if="loadError" title="加载失败" :description="loadError" action-text="重试" :action-icon="false" @action="load" />
            <EmptyState v-else-if="hasFilters" title="没有符合条件的反馈" action-text="清除筛选" :action-icon="false" @action="resetFilters" />
            <EmptyState v-else :title="q.status === 'pending' ? '都处理完了' : '暂无反馈'" />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="total > 0"
          :page="q.page"
          :total="total"
          :page-size="pageSize"
          @update:page="(p) => (q.page = p)"
          @update:page-size="onPageSize"
        />
      </template>
    </TablePageLayout>

    <PromptSheet
      ref="sheetRef"
      :open="declining"
      :title="`不处理 ${declineIds.length} 条反馈`"
      placeholder="会展示给用户"
      confirm-text="不处理"
      danger
      @close="declining = false"
      @confirm="decline"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter, type LocationQuery } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminApi, type Kind, type ListItem, type Severity, type Status } from '../api'
import { KIND_LABEL, SEVERITIES, SEVERITY_META, ago, fullTime } from '../format'
import { pendingBadge } from '../state'
import { allScenarios, loadAdminScenarios, scenarioIcon, scenarioLabel } from './shared'
import StatusPill from '../components/StatusPill.vue'
import PromptSheet from '../components/PromptSheet.vue'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const here = route.name

const columns: Column[] = [
  { key: 'summary', label: '内容' },
  { key: 'kind', label: '类型 · 场景' },
  { key: 'severity', label: '等级' },
  { key: 'status', label: '状态' },
  { key: 'last_activity_at', label: '最近活动' }
]

// 筛选条件、页码和 URL 同步：看板下钻、详情页返回、刷新页面都能还原。
// URL 里 status 缺省 = 待处理，status=all（或旧链接的 status=）= 全部状态
const one = (v: LocationQuery[string] | undefined) => String((Array.isArray(v) ? v[0] : v) ?? '')
function readQuery(rq: LocationQuery) {
  const status = one(rq.status)
  return {
    q: {
      status: rq.status === undefined ? 'pending' : status === 'all' ? '' : status,
      kind: one(rq.kind) as Kind | '',
      scenario: one(rq.scenario),
      severity: one(rq.severity),
      tag: one(rq.tag),
      days: one(rq.days),
      page: Math.max(1, Math.floor(Number(one(rq.page))) || 1)
    },
    // 三个搜索框各自模糊匹配；旧链接里的 user_id 按用户 ID 搜
    search: { content: one(rq.content), user: one(rq.user ?? rq.user_id), id: one(rq.id) }
  }
}
const initial = readQuery(route.query)
const q = reactive(initial.q)
const search = reactive(initial.search)
const pageSize = ref(getPersistedPageSize())
const items = ref<ListItem[]>([])
const total = ref(0)
const counts = ref<Partial<Record<Status, number>>>({})
const tags = ref<string[]>([])
const loading = ref(false)
const loadError = ref('')
const selected = ref<number[]>([])

const withCount = (label: string, n?: number) => (n ? `${label} (${n})` : label)
const statusOptions = computed(() => [
  { value: 'pending', label: withCount('待处理', counts.value.pending) },
  { value: 'awaiting', label: withCount('已回复', counts.value.awaiting) },
  { value: 'closed', label: '已结案' },
  { value: '', label: '全部状态' }
])
const kindOptions = [
  { value: '', label: '全部类型' },
  { value: 'issue', label: '问题' },
  { value: 'suggestion', label: '建议' }
]
const scenarioOptions = computed(() => [
  { value: '', label: '全部场景' },
  ...allScenarios.value.filter((s) => !q.kind || s.kind === q.kind).map((s) => ({ value: s.key, label: `${s.icon} ${s.label}` }))
])
const severityOptions = [
  { value: '', label: '全部等级' },
  { value: 'none', label: '未定级' },
  ...SEVERITIES.map((s) => ({ value: s, label: SEVERITY_META[s].label }))
]
const tagOptions = computed(() => [{ value: '', label: '全部标签' }, ...tags.value.map((t) => ({ value: t, label: t }))])
// 与看板口径一致：按提交时间，近 N 天
const dayOptions = computed(() => {
  const opts = [
    { value: '', label: '全部时间' },
    { value: '7', label: '近 7 天' },
    { value: '30', label: '近 30 天' },
    { value: '90', label: '近 90 天' }
  ]
  if (q.days && !opts.some((o) => o.value === q.days)) opts.push({ value: q.days, label: `近 ${q.days} 天` })
  return opts
})

const hasFilters = computed(
  () => !!(q.kind || q.scenario || q.severity || q.tag || q.days || search.content.trim() || search.user.trim() || search.id.trim())
)

// 新请求发出时取消上一次，避免慢的旧响应覆盖新筛选的结果
let ctrl: AbortController | null = null
async function load() {
  ctrl?.abort()
  const c = (ctrl = new AbortController())
  loading.value = true
  loadError.value = ''
  try {
    const res = await adminApi.list(
      { ...q, content: search.content.trim(), user: search.user.trim(), id: search.id.trim(), page_size: pageSize.value },
      { signal: c.signal }
    )
    if (c.signal.aborted) return
    // 页码超出范围（旧链接、批量处理后最后一页清空）：退到最后一页
    if (!res.items.length && res.total > 0 && q.page > 1 && res.pages < q.page) {
      q.page = Math.max(1, res.pages)
      return
    }
    items.value = res.items
    total.value = res.total
    counts.value = res.counts
    selected.value = []
  } catch (e) {
    if (c.signal.aborted) return
    loadError.value = extractApiErrorMessage(e, '加载失败')
    // 表格里还有旧数据时空状态不会出现，用提示告知
    if (items.value.length) appStore.showError(loadError.value)
  } finally {
    if (ctrl === c) {
      loading.value = false
      ctrl = null
    }
  }
}

function currentQuery(): Record<string, string> {
  const out: Record<string, string> = {}
  if (q.status !== 'pending') out.status = q.status || 'all'
  const rest = {
    kind: q.kind,
    scenario: q.scenario,
    severity: q.severity,
    tag: q.tag,
    days: q.days,
    content: search.content.trim(),
    user: search.user.trim(),
    id: search.id.trim()
  }
  for (const [k, v] of Object.entries(rest)) if (v) out[k] = v
  if (q.page > 1) out.page = String(q.page)
  return out
}

// 记下自己写进 URL 的地址，路由监听据此区分「自己同步的」和「外部跳转」
let lastSynced = ''
function syncQuery() {
  if (route.name !== here) return
  const to = router.resolve({ path: route.path, query: currentQuery() }).fullPath
  lastSynced = to
  if (to !== route.fullPath) void router.replace(to)
}

function apply() {
  if (q.page !== 1) q.page = 1
  else {
    syncQuery()
    void load()
  }
}

function filterUser(email?: string) {
  if (!email) return
  search.user = email
  apply()
}

function onPageSize(size: number) {
  pageSize.value = size
  apply()
}

// 只清筛选和搜索，保留状态
function resetFilters() {
  const changed = !!(q.kind || q.scenario || q.severity || q.tag || q.days)
  search.content = ''
  search.user = ''
  search.id = ''
  Object.assign(q, { kind: '', scenario: '', severity: '', tag: '', days: '' })
  if (!changed) apply()
}

// 从 URL 还原时不触发下面的筛选监听（否则会把页码重置为 1 并重复加载）
let hydrating = false
watch(
  () => q.kind,
  () => {
    if (hydrating || !allScenarios.value.length) return
    if (q.scenario && !scenarioOptions.value.some((o) => o.value === q.scenario)) q.scenario = ''
  }
)
watch(
  () => [q.status, q.kind, q.scenario, q.severity, q.tag, q.days],
  () => {
    if (!hydrating) apply()
  }
)
watch(
  () => q.page,
  () => {
    if (hydrating) return
    syncQuery()
    void load()
  }
)

// 同一路由内的外部跳转（侧边栏入口、看板链接）：按新 URL 重新还原筛选
watch(
  () => route.fullPath,
  (full) => {
    if (route.name !== here || full === lastSynced) return
    const next = readQuery(route.query)
    hydrating = true
    Object.assign(q, next.q)
    Object.assign(search, next.search)
    void nextTick(() => (hydrating = false))
    lastSynced = full
    void load()
  }
)

// ─── 批量操作 ──────────────────────────────────────────────────────────

const batchSeverityOptions = SEVERITIES.map((s) => ({ value: s, label: SEVERITY_META[s].label }))
const tagChoices = computed(() => tags.value.map((t) => ({ value: t, label: t })))
const batching = ref(false)
const sheetRef = ref<InstanceType<typeof PromptSheet> | null>(null)
// 打开「不处理」时记下要结案的 ID，确认时只提交这些
const declineIds = ref<number[]>([])
const declining = ref(false)

// 已结案的不能再「不处理」，只统计、提交未结案的
const isOpen = (s: Status) => s === 'pending' || s === 'awaiting'
const declinable = computed(() => items.value.filter((r) => selected.value.includes(r.id) && isOpen(r.status)).map((r) => r.id))

type BatchInput = { severity?: Severity; add_tag?: string; decline_reason?: string }

async function runBatch(input: BatchInput, ids: number[] = selected.value, onError: (m: string) => void = (m) => appStore.showError(m)): Promise<boolean> {
  if (batching.value || !ids.length) return false
  batching.value = true
  try {
    const res = await adminApi.batch({ ids, ...input })
    const reasons = new Map<string, number>()
    for (const r of Object.values(res.failed ?? {})) reasons.set(r, (reasons.get(r) ?? 0) + 1)
    if (reasons.size) {
      const detail = [...reasons].map(([r, n]) => `${n} 条${r}`).join('，')
      appStore.showWarning(`已处理 ${res.done} 条，${detail}`)
    } else {
      appStore.showSuccess(`已处理 ${res.done} 条`)
    }
    selected.value = []
    pendingBadge(true)
    void load()
    return true
  } catch (e) {
    onError(extractApiErrorMessage(e, '操作失败'))
    return false
  } finally {
    batching.value = false
  }
}

async function batchTag(v: string | number | boolean | null) {
  const tag = String(v ?? '').trim()
  if (tag && (await runBatch({ add_tag: tag }))) loadTags()
}

function openDecline() {
  if (!declinable.value.length) return
  declineIds.value = [...declinable.value]
  declining.value = true
}

// 失败时对话框不关，已填的原因保留，错误显示在输入框下
async function decline(reason: string) {
  const ok = await runBatch({ decline_reason: reason }, declineIds.value, (m) => sheetRef.value?.fail(m))
  sheetRef.value?.done()
  if (ok) declining.value = false
}

function loadTags() {
  adminApi
    .tags()
    .then((t) => (tags.value = t))
    .catch(() => {})
}

onMounted(() => {
  void loadAdminScenarios()
  loadTags()
  pendingBadge(true)
  void load()
})
onBeforeUnmount(() => ctrl?.abort())
</script>
