<!-- [plus] 批次详情：名单 + 每个人将收到 / 已收到的邮件内容 + 状态操作 + 统计 -->
<template>
  <AppLayout>
    <div v-if="loadError" class="card p-16 text-center text-sm text-gray-500">{{ loadError }}</div>
    <div v-else-if="!batch" class="card p-16 text-center text-sm text-gray-400">正在加载…</div>
    <div v-else class="space-y-6 pb-16">
      <div class="flex flex-wrap items-center gap-3">
        <button type="button" class="btn btn-secondary" @click="router.push(`/admin/outreach/campaigns/${batch.campaign_id}`)">← 返回活动</button>
        <div class="min-w-0 flex-1">
          <h1 class="truncate text-lg font-semibold text-gray-900 dark:text-white">{{ batch.name }}</h1>
          <p class="truncate text-xs text-gray-500">{{ batch.campaign_name }} · {{ purposeLabel(batch.purpose) }} · {{ SCOPE_LABEL[batch.scope] }} · {{ batch.subject }}</p>
        </div>
        <span class="badge" :class="BATCH_CLASS[batch.status]">{{ BATCH_LABEL[batch.status] }}</span>
        <button v-if="batch.status === 'pending'" type="button" class="btn btn-primary" :disabled="busy" @click="confirmStart = true">开始执行</button>
        <button v-if="batch.status === 'running'" type="button" class="btn btn-secondary" :disabled="busy" @click="act('pause')">暂停</button>
        <button v-if="batch.status === 'paused'" type="button" class="btn btn-primary" :disabled="busy" @click="act('resume')">恢复执行</button>
        <button v-if="['pending', 'running', 'paused'].includes(batch.status)" type="button" class="btn btn-danger" :disabled="busy" @click="confirmCancel = true">取消</button>
        <button v-if="!['running', 'paused'].includes(batch.status)" type="button" class="btn btn-danger" :disabled="busy" @click="confirmDelete = true">删除</button>
      </div>

      <div v-if="batch.status === 'pending'" class="rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800 dark:border-blue-800/50 dark:bg-blue-900/20 dark:text-blue-200">
        名单已生成但<strong>尚未发送</strong>。下面可以点「预览」查看每个人将收到的邮件。「开始执行」后，每封邮件发出前仍会重新校验用户状态（已退订、已验证、换邮箱、频控等），不满足的自动跳过并写明原因。
      </div>
      <div v-if="batch.status === 'paused' && batch.pause_reason" class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-200">
        暂停原因：{{ batch.pause_reason }}
      </div>

      <StatsPanel v-if="stats" :stats="stats" :marketing="marketing" show-progress title="投递结果" />

      <section class="card">
        <div class="flex flex-wrap items-center gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="flex-1 text-sm font-medium text-gray-900 dark:text-white">名单与邮件内容</h2>
          <input v-model="q" class="input w-56" placeholder="搜索邮箱" @keyup.enter="reload" />
          <Select v-model="rcptStatus" :options="rcptOptions" class="w-36" />
        </div>
        <DataTable :columns="rcptColumns" :data="rcpts" :loading="rcptLoading" row-key="id">
          <template #cell-status="{ row }">
            <span class="text-sm" :class="row.status === 'failed' ? 'text-red-600' : 'text-gray-700 dark:text-dark-200'">{{ RCPT_LABEL[row.status as string] }}</span>
            <span v-if="row.bounced" class="ml-1 text-xs text-red-500">退信</span>
            <p v-if="row.error" class="max-w-xs truncate text-xs text-gray-400" :title="row.error">{{ row.error }}</p>
          </template>
          <template #cell-sent_at="{ value }"><span class="text-sm text-gray-500">{{ fmtTime(value) }}</span></template>
          <template #cell-clicked_at="{ value }"><span class="text-sm text-gray-500">{{ fmtTime(value) }}</span></template>
          <template #cell-claimed_at="{ value }"><span class="text-sm text-gray-500">{{ fmtTime(value) }}</span></template>
          <template #cell-actions="{ row }">
            <button type="button" class="text-sm text-primary-600 hover:underline" @click="openPreview(row)">预览邮件</button>
          </template>
        </DataTable>
        <div class="px-4 py-3">
          <Pagination v-if="rcptTotal > 0" :page="rcptPage" :total="rcptTotal" :page-size="rcptSize" @update:page="(p: number) => (rcptPage = p)" @update:pageSize="(s: number) => { rcptSize = s; rcptPage = 1 }" />
        </div>
      </section>
    </div>

    <BaseDialog :show="!!preview || previewLoading" title="邮件预览" width="wide" @close="closePreview">
      <p v-if="previewLoading" class="py-10 text-center text-sm text-gray-400">正在渲染…</p>
      <template v-else-if="preview">
        <p class="text-xs text-gray-500">收件人：{{ preview.email }}</p>
        <p class="mb-3 mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ preview.subject }}</p>
        <iframe :srcdoc="preview.html" sandbox="" class="h-[520px] w-full rounded-lg border border-gray-200 bg-white dark:border-dark-700" title="邮件预览" />
        <p class="mt-2 text-xs text-gray-400">未发送的邮件按名单生成时的数据显示，实际发送时余额等变量会取最新值。</p>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="confirmStart" title="开始执行批次" :message="`将向名单里的 ${batch?.total ?? 0} 人按「发送设置」里的限速逐步发出邮件，发出前会重新校验每个人的状态。执行中可以暂停或取消。`" confirm-text="开始执行" @confirm="doStart" @cancel="confirmStart = false" />
    <ConfirmDialog :show="confirmCancel" title="取消批次" message="取消后未发送的邮件不会再发送，已发送的不受影响。此操作不可恢复。" danger confirm-text="确认取消" @confirm="doCancel" @cancel="confirmCancel = false" />
    <ConfirmDialog :show="confirmDelete" title="删除批次" :message="deleteMessage" danger confirm-text="删除" @confirm="doDelete" @cancel="confirmDelete = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import StatsPanel from '../components/StatsPanel.vue'
import { adminApi, fmtTime, useOutreachMeta, BATCH_CLASS, BATCH_LABEL, SCOPE_LABEL, type Batch, type MailPreview, type RecipientRow, type DeliveryStats } from '../api'


const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const id = Number(route.params.id)
const { purposeOf, purposeLabel } = useOutreachMeta()

const batch = ref<Batch | null>(null)
const stats = ref<DeliveryStats | null>(null)
const loadError = ref('')
const busy = ref(false)
const confirmStart = ref(false)
const confirmCancel = ref(false)
const confirmDelete = ref(false)

const marketing = computed(() => purposeOf(batch.value?.purpose ?? '')?.category === 'marketing')
const deleteMessage = computed(() =>
  (stats.value?.sent ?? 0) > 0
    ? '该批次已发出过邮件：删除后会从列表消失，但数据保留一段时间（见「发送设置 · 已删除批次保留」）后才清理，期间频控和领取链接依然有效。'
    : '该批次没有发出过邮件，将直接删除。'
)

const RCPT_LABEL: Record<string, string> = { queued: '待发送', sending: '发送中', sent: '已发送', failed: '失败', skipped: '已跳过' }
const rcptOptions = [{ value: '', label: '全部' }, ...Object.entries(RCPT_LABEL).map(([value, label]) => ({ value, label }))]
const rcptColumns: Column[] = [
  { key: 'email', label: '邮箱' },
  { key: 'status', label: '状态 / 原因' },
  { key: 'sent_at', label: '发送时间' },
  { key: 'clicked_at', label: '点击时间' },
  { key: 'claimed_at', label: '领取时间' },
  { key: 'actions', label: '' }
]
const q = ref('')
const rcptStatus = ref('')
const rcpts = ref<RecipientRow[]>([])
const rcptTotal = ref(0)
const rcptPage = ref(1)
const rcptSize = ref(20)
const rcptLoading = ref(false)

async function loadBatch() {
  try {
    batch.value = await adminApi.getBatch(id)
    stats.value = await adminApi.batchStats(id)
  } catch (e) {
    if (!batch.value) loadError.value = extractApiErrorMessage(e, '加载失败')
  }
}

async function loadRcpts() {
  rcptLoading.value = true
  try {
    const r = await adminApi.recipients(id, { status: rcptStatus.value, q: q.value, page: rcptPage.value, page_size: rcptSize.value })
    rcpts.value = r.items
    rcptTotal.value = r.total
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载名单失败'))
  } finally {
    rcptLoading.value = false
  }
}

function reload() {
  if (rcptPage.value !== 1) rcptPage.value = 1
  else loadRcpts()
}

async function run(fn: () => Promise<void>, fail: string) {
  busy.value = true
  try {
    await fn()
    await loadBatch()
    await loadRcpts()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, fail))
  } finally {
    busy.value = false
  }
}

const act = (kind: 'pause' | 'resume') =>
  run(() => (kind === 'pause' ? adminApi.pauseBatch(id) : adminApi.resumeBatch(id)), '操作失败')

function doStart() {
  confirmStart.value = false
  return run(async () => {
    await adminApi.startBatch(id)
    appStore.showSuccess('已开始执行')
  }, '开始执行失败')
}

function doCancel() {
  confirmCancel.value = false
  return run(() => adminApi.cancelBatch(id), '取消失败')
}

async function doDelete() {
  confirmDelete.value = false
  const cid = batch.value?.campaign_id
  try {
    await adminApi.removeBatch(id)
    router.replace(cid ? `/admin/outreach/campaigns/${cid}` : '/admin/outreach/campaigns')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '删除失败'))
  }
}

// ─── 单个收件人邮件预览 ──────────────────────────────────────────────────
const preview = ref<MailPreview | null>(null)
const previewLoading = ref(false)

async function openPreview(row: RecipientRow) {
  previewLoading.value = true
  preview.value = null
  try {
    preview.value = await adminApi.recipientPreview(id, row.id)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '预览失败'))
  } finally {
    previewLoading.value = false
  }
}

function closePreview() {
  preview.value = null
  previewLoading.value = false
}

watch([rcptStatus, rcptPage, rcptSize], loadRcpts)
watch(rcptStatus, () => (rcptPage.value = 1))

let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  loadBatch()
  loadRcpts()
  timer = setInterval(() => {
    if (batch.value?.status === 'running') {
      loadBatch()
      loadRcpts()
    }
  }, 10_000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>
