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
          <Select v-if="tags.length" v-model="q.tag" :options="tagOptions" class="w-32" searchable />

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
          <Select v-model="batchSeverity" :options="batchSeverityOptions" class="w-28" placeholder="定级" />
          <button type="button" class="btn btn-secondary btn-sm" @click="prompt = 'tag'">加标签</button>
          <button type="button" class="btn btn-secondary btn-sm" @click="prompt = 'decline'">不处理</button>
          <button type="button" class="btn btn-secondary btn-sm" @click="selected = []">取消选择</button>
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
            <EmptyState :title="q.status === 'pending' ? '都处理完了' : '没有符合条件的反馈'" description="" />
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
      :open="prompt !== ''"
      :title="prompt === 'tag' ? '加标签' : '批量不处理'"
      :placeholder="prompt === 'tag' ? '标签' : '原因，会展示给用户'"
      :multiline="prompt === 'decline'"
      @close="prompt = ''"
      @confirm="onPrompt"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
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

const columns: Column[] = [
  { key: 'summary', label: '内容' },
  { key: 'kind', label: '类型 · 场景' },
  { key: 'severity', label: '等级' },
  { key: 'status', label: '状态' },
  { key: 'last_activity_at', label: '最近活动' }
]

// 筛选条件和 URL 同步：从看板点进来、刷新页面都能保留
const q = reactive({
  status: String(route.query.status ?? 'pending'),
  kind: String(route.query.kind ?? '') as Kind | '',
  scenario: String(route.query.scenario ?? ''),
  severity: String(route.query.severity ?? ''),
  tag: String(route.query.tag ?? ''),
  page: 1
})
// 三个搜索框各自模糊匹配；旧链接里的 user_id 按用户 ID 搜
const search = reactive({
  content: String(route.query.content ?? ''),
  user: String(route.query.user ?? route.query.user_id ?? ''),
  id: String(route.query.id ?? '')
})
const pageSize = ref(20)
const items = ref<ListItem[]>([])
const total = ref(0)
const counts = ref<Partial<Record<Status, number>>>({})
const tags = ref<string[]>([])
const loading = ref(false)
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

async function load() {
  loading.value = true
  try {
    const res = await adminApi.list({ ...q, content: search.content.trim(), user: search.user.trim(), id: search.id.trim(), page_size: pageSize.value })
    items.value = res.items
    total.value = res.total
    counts.value = res.counts
    selected.value = []
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载失败'))
  } finally {
    loading.value = false
  }
}

function syncQuery() {
  const entries = Object.entries({ status: q.status, kind: q.kind, scenario: q.scenario, severity: q.severity, tag: q.tag, content: search.content.trim(), user: search.user.trim(), id: search.id.trim() })
  const query = Object.fromEntries(entries.filter(([k, v]) => v !== '' && v !== undefined && !(k === 'status' && v === 'pending')))
  void router.replace({ query: query as Record<string, string> })
}

function apply() {
  if (q.page !== 1) q.page = 1
  else void load()
  syncQuery()
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

watch(
  () => q.kind,
  () => {
    if (q.scenario && !scenarioOptions.value.some((o) => o.value === q.scenario)) q.scenario = ''
  }
)
watch(() => [q.status, q.kind, q.scenario, q.severity, q.tag], apply)
watch(() => q.page, load)

// ─── 批量操作 ──────────────────────────────────────────────────────────

const batchSeverity = ref<string | null>(null)
const batchSeverityOptions = SEVERITIES.map((s) => ({ value: s, label: SEVERITY_META[s].label }))
const prompt = ref<'' | 'tag' | 'decline'>('')

async function runBatch(input: { severity?: Severity; add_tag?: string; decline_reason?: string }) {
  try {
    const res = await adminApi.batch({ ids: selected.value, ...input })
    const failed = Object.keys(res.failed ?? {}).length
    appStore.showSuccess(failed ? `完成 ${res.done} 条，${failed} 条未处理` : `已处理 ${res.done} 条`)
    pendingBadge(true)
    await load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '操作失败'))
  }
}

watch(batchSeverity, (v) => {
  if (!v) return
  batchSeverity.value = null
  void runBatch({ severity: v as Severity })
})

function onPrompt(v: string) {
  void runBatch(prompt.value === 'tag' ? { add_tag: v } : { decline_reason: v })
}

onMounted(() => {
  void loadAdminScenarios()
  adminApi.tags().then((t) => (tags.value = t)).catch(() => {})
  pendingBadge(true)
  void load()
})
</script>
