<!-- [plus] 管理端「触达活动」列表：管理活动「定义」。行内编辑 / 复制 / 删除；核心指标由各目的声明。 -->
<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex w-full items-center justify-end gap-2">
          <button type="button" class="btn btn-secondary" :disabled="loading" title="刷新" @click="load()">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button type="button" class="btn btn-primary whitespace-nowrap" @click="router.push('/admin/outreach/campaigns/new')">
            <Icon name="plus" size="md" class="mr-1" />
            新建活动
          </button>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="items" :loading="loading" row-key="id" clickable-rows @row-click="open">
          <template #cell-name="{ row }">
            <div class="max-w-xs">
              <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ row.name }}</p>
              <p class="truncate text-xs text-gray-500 dark:text-dark-400">{{ row.subject }}</p>
            </div>
          </template>
          <template #cell-purpose="{ row }">
            <span class="text-sm text-gray-600 dark:text-dark-300">{{ purposeLabel(row.purpose) }}</span>
            <p v-for="b in (row.benefits as BenefitSpec[])" :key="b.kind" class="text-xs text-purple-600 dark:text-purple-400">+ {{ benefitText(b) }}</p>
          </template>
          <template #cell-batches="{ row }">
            <span class="text-sm tabular-nums text-gray-700 dark:text-dark-200">{{ row.batch_count }}</span>
            <span v-if="row.running" class="badge badge-primary ml-2">执行中</span>
          </template>
          <template #cell-sent="{ row }">
            <span class="text-sm tabular-nums text-gray-700 dark:text-dark-200">{{ row.sent || '—' }}</span>
          </template>
          <template #cell-primary="{ row }">
            <template v-if="row.primary && row.sent">
              <span class="text-sm tabular-nums text-gray-900 dark:text-white">{{ pct(row.primary.value, row.sent) }}</span>
              <p class="text-xs text-gray-400">{{ row.primary.label }} {{ row.primary.value }}</p>
            </template>
            <span v-else class="text-sm text-gray-400">—</span>
          </template>
          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ fmtTime(value) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-3" @click.stop>
              <button type="button" class="text-sm text-primary-600 hover:underline" @click="router.push(`/admin/outreach/campaigns/${row.id}/edit`)">编辑</button>
              <button type="button" class="text-sm text-gray-500 hover:underline" :disabled="busyId === row.id" @click="doDuplicate(row)">复制</button>
              <button type="button" class="text-sm text-red-500 hover:underline" :disabled="busyId === row.id" @click="deleteTarget = row">删除</button>
            </div>
          </template>
          <template #empty>
            <EmptyState v-if="loadError" title="加载失败" :description="loadError">
              <template #action><button type="button" class="btn btn-secondary" @click="load()">重试</button></template>
            </EmptyState>
            <EmptyState v-else title="还没有活动" description="新建活动：选目的、圈人群、选模板，保存后在详情页分批发送" />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="total > 0" :page="page" :total="total" :page-size="pageSize" @update:page="(p: number) => (page = p)" @update:pageSize="(s: number) => { pageSize = s; page = 1 }" />
      </template>
    </TablePageLayout>

    <ConfirmDialog
      :show="!!deleteTarget"
      title="删除活动"
      :message="`删除「${deleteTarget?.name ?? ''}」及其全部批次。没发过邮件的批次直接删除；发过邮件的批次保留一段时间后才清理（期间频控和领取链接仍有效）。执行中或已暂停的批次需要先取消。`"
      danger
      confirm-text="删除"
      @confirm="doDelete"
      @cancel="deleteTarget = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminApi, fmtTime, pct, useOutreachMeta, type BenefitSpec, type CampaignRow } from '../api'

const router = useRouter()
const appStore = useAppStore()
const { purposeLabel, benefitText } = useOutreachMeta()

const columns: Column[] = [
  { key: 'name', label: '活动' },
  { key: 'purpose', label: '目的 / 权益' },
  { key: 'batches', label: '批次' },
  { key: 'sent', label: '累计已发送' },
  { key: 'primary', label: '核心指标' },
  { key: 'created_at', label: '创建时间' },
  { key: 'actions', label: '操作' }
]

const items = ref<CampaignRow[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const loadError = ref('')
const busyId = ref<number | null>(null)
const deleteTarget = ref<CampaignRow | null>(null)

let ctrl: AbortController | null = null
let timer: ReturnType<typeof setInterval> | null = null

async function load(silent = false) {
  ctrl?.abort()
  ctrl = new AbortController()
  if (!silent) loading.value = true
  try {
    const r = await adminApi.list({ page: page.value, page_size: pageSize.value }, ctrl.signal)
    items.value = r.items
    total.value = r.total
    loadError.value = ''
  } catch (e: unknown) {
    const n = e as { name?: string; code?: string }
    if (n?.name === 'AbortError' || n?.code === 'ERR_CANCELED') return
    loadError.value = extractApiErrorMessage(e, '加载失败')
    items.value = []
  } finally {
    loading.value = false
  }
}

const open = (row: CampaignRow) => router.push(`/admin/outreach/campaigns/${row.id}`)

async function doDuplicate(row: CampaignRow) {
  busyId.value = row.id
  try {
    const c = await adminApi.duplicate(row.id)
    appStore.showSuccess('已复制，正在打开副本')
    router.push(`/admin/outreach/campaigns/${c.id}/edit`)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '复制失败'))
  } finally {
    busyId.value = null
  }
}

async function doDelete() {
  const row = deleteTarget.value
  deleteTarget.value = null
  if (!row) return
  busyId.value = row.id
  try {
    await adminApi.remove(row.id)
    appStore.showSuccess('已删除')
    await load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '删除失败'))
  } finally {
    busyId.value = null
  }
}

watch([page, pageSize], () => load())

onMounted(() => {
  load()
  timer = setInterval(() => {
    if (items.value.some((x) => x.running)) load(true)
  }, 10_000)
})
onBeforeUnmount(() => {
  ctrl?.abort()
  if (timer) clearInterval(timer)
})
</script>
