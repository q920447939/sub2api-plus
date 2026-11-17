<!-- [plus] 用户端「我的反馈」：沿用项目标准列表页（筛选 + 表格 + 分页） -->
<template>
  <AppLayout>
    <TablePageLayout class="fb-list-page">
      <template v-if="meta?.notice" #actions>
        <NoticeCard :source="meta.notice" />
      </template>

      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <Select v-model="state" :options="stateOptions" class="min-w-0 flex-1 sm:w-32 sm:flex-none" />
          <Select v-model="kind" :options="kindOptions" class="min-w-0 flex-1 sm:w-32 sm:flex-none" />
          <div class="flex w-full items-center justify-end gap-2 sm:w-auto sm:flex-1">
            <button type="button" class="btn btn-secondary" :disabled="loading" title="刷新" @click="load">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button type="button" class="btn btn-primary whitespace-nowrap" @click="openComposer(route.fullPath)">
              <Icon name="plus" size="md" class="mr-1" />
              新反馈
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="items" :loading="loading" row-key="id" clickable-rows @row-click="(row) => router.push(`/feedback/${row.id}`)">
          <template #cell-summary="{ row }">
            <div class="flex max-w-lg items-center gap-2">
              <span class="shrink-0 text-base leading-none">{{ scenarioOf(row.scenario)?.icon ?? '💬' }}</span>
              <span class="truncate text-sm text-gray-900 dark:text-white" :class="row.user_unread ? 'font-semibold' : ''">{{ row.summary }}</span>
              <span v-if="row.user_unread" class="h-2 w-2 shrink-0 rounded-full bg-red-500" title="有新回复" />
            </div>
          </template>

          <template #cell-kind="{ row }">
            <span class="text-sm text-gray-600 dark:text-dark-300">{{ kindScenario(row) }}</span>
          </template>

          <template #cell-status="{ row }">
            <div class="flex items-center gap-2">
              <StatusPill :status="row.status" />
              <span v-if="row.reward_points" class="text-xs font-medium text-purple-600 dark:text-purple-400">+{{ row.reward_points }} 积分</span>
            </div>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ fullTime(value) }}</span>
          </template>

          <template #cell-last_activity_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400" :title="fullTime(value)">{{ ago(value) }}</span>
          </template>

          <template #empty>
            <EmptyState v-if="loadError" title="加载失败" :description="loadError">
              <template #action>
                <button type="button" class="btn btn-secondary" data-testid="list-retry" @click="load">重试</button>
              </template>
            </EmptyState>
            <EmptyState v-else :title="state || kind ? '没有符合条件的反馈' : '还没有反馈'" description="">
              <template v-if="!state && !kind" #action>
                <button type="button" class="btn btn-primary" @click="openComposer(route.fullPath)">提交第一条反馈</button>
              </template>
            </EmptyState>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="total > 0" :page="page" :total="total" :page-size="pageSize" @update:page="onPage" @update:page-size="onPageSize" />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { userApi, type Kind, type ListItem } from '../api'
import { composer, loadMeta, meta, openComposer, scenarioOf, unreadBadge } from '../state'
import { KIND_LABEL, ago, fullTime } from '../format'
import StatusPill from '../components/StatusPill.vue'
import NoticeCard from '../components/NoticeCard.vue'

const route = useRoute()
const router = useRouter()

const columns: Column[] = [
  { key: 'summary', label: '内容' },
  { key: 'kind', label: '类型 · 场景' },
  { key: 'status', label: '状态' },
  { key: 'created_at', label: '提交时间' },
  { key: 'last_activity_at', label: '最近更新' }
]

type State = '' | 'open' | 'closed'
const stateOptions = [
  { value: '', label: '全部状态' },
  { value: 'open', label: '处理中' },
  { value: 'closed', label: '已结束' }
]
const kindOptions = [
  { value: '', label: '全部类型' },
  { value: 'issue', label: '问题' },
  { value: 'suggestion', label: '建议' }
]

const state = ref<State>('')
const kind = ref<Kind | ''>('')
const items = ref<ListItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const loadError = ref('')

/** 「问题 · 接口报错」；场景已被删除时只显示类型，不露出内部 key */
function kindScenario(row: ListItem): string {
  const label = scenarioOf(row.scenario)?.label
  return label ? `${KIND_LABEL[row.kind]} · ${label}` : KIND_LABEL[row.kind]
}

const isAbortError = (e: unknown) => {
  if (!e || typeof e !== 'object') return false
  const { name, code } = e as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

// 快速切换筛选/翻页时只认最后一次请求，旧响应不会覆盖新结果
let abortController: AbortController | null = null

async function load() {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  const { signal } = controller
  loading.value = true
  loadError.value = ''
  try {
    const res = await userApi.list({ page: page.value, page_size: pageSize.value, state: state.value, kind: kind.value }, { signal })
    if (signal.aborted) return
    items.value = res.items
    total.value = res.total
  } catch (e) {
    if (isAbortError(e) || signal.aborted) return
    items.value = []
    total.value = 0
    loadError.value = extractApiErrorMessage(e, '加载失败')
  } finally {
    if (abortController === controller) loading.value = false
  }
}

function reload() {
  page.value = 1
  void load()
}
function onPage(p: number) {
  page.value = p
  void load()
}
function onPageSize(size: number) {
  pageSize.value = size
  reload()
}

watch([state, kind], reload)
watch(() => composer.submitted, reload)

onMounted(() => {
  void loadMeta()
  void load()
  unreadBadge(true)
  // 从别处带 ?new=1 进来时直接打开提交面板
  if (route.query.new) openComposer(String(route.query.from || route.path))
})
onBeforeUnmount(() => abortController?.abort())
</script>

<style scoped>
/* 公告默认展开：给列表留出最小高度，公告很长时整页滚动，而不是把列表挤没 */
.fb-list-page:not(.mobile-mode) {
  height: auto;
  min-height: calc(100vh - 64px - 4rem);
}
.fb-list-page:not(.mobile-mode) :deep(.layout-section-scrollable) {
  min-height: 360px;
}
</style>
