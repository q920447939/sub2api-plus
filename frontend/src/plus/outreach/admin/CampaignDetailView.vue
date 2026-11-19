<!-- [plus] 活动详情：「运营」这个活动——完整人群规则、按目的的效果、批次列表、生成批次。编辑 / 复制 / 删除收进「更多」。 -->
<template>
  <AppLayout>
    <div v-if="loadError" class="card p-16 text-center text-sm text-gray-500">{{ loadError }}</div>
    <div v-else-if="!camp" class="card p-16 text-center text-sm text-gray-400">正在加载…</div>
    <div v-else class="space-y-6 pb-16">
      <div class="flex flex-wrap items-center gap-3">
        <button type="button" class="btn btn-secondary" @click="router.push('/admin/outreach/campaigns')">← 返回</button>
        <div class="min-w-0 flex-1">
          <h1 class="truncate text-lg font-semibold text-gray-900 dark:text-white">{{ camp.name }}</h1>
          <p class="truncate text-xs text-gray-500">{{ purposeLabel(camp.purpose) }} · {{ camp.subject }}</p>
        </div>
        <MoreMenu :items="menu" :disabled="busy" @select="onMenu" />
        <button type="button" class="btn btn-primary" @click="openDialog">生成批次</button>
      </div>

      <section class="card grid gap-6 p-6 text-sm md:grid-cols-2">
        <div>
          <h2 class="mb-2 text-sm font-medium text-gray-900 dark:text-white">人群（完整规则）</h2>
          <ul class="space-y-1 text-gray-600 dark:text-dark-300">
            <li v-for="r in camp.rules" :key="r">· {{ r }}</li>
          </ul>
        </div>
        <div class="space-y-1 text-gray-600 dark:text-dark-300">
          <h2 class="mb-2 text-sm font-medium text-gray-900 dark:text-white">内容与去向</h2>
          <p>内容来源：{{ camp.template_name || '手写' }}；外壳：{{ camp.layout_name || '默认外壳' }}</p>
          <p>按钮去向：{{ actionText }}</p>
          <p>附带权益：{{ camp.benefits.length ? `${camp.benefits.map(benefitText).join('、')}（发出后 ${camp.claim_days} 天内可领）` : '无' }}</p>
        </div>
        <p class="text-xs text-gray-400 md:col-span-2">
          批次是活动的一次执行：生成时冻结名单、内容和外壳，核对无误后点「开始执行」才会发信，同一时间只执行一个批次。
          批次只看投递结果（发送、失败、跳过、退信），用来确认内容和名单没问题；效果指标按活动累计，见上方。建议先随机取几个人小批量验证，再「只发给尚未收到的人」逐步放量。
        </p>
      </section>

      <StatsPanel v-if="stats" :stats="stats" :marketing="marketing" title="累计效果（全部批次）" />

      <section class="card">
        <div class="flex items-center border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="flex-1 text-sm font-medium text-gray-900 dark:text-white">批次</h2>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="loadBatches()">刷新</button>
        </div>
        <DataTable :columns="columns" :data="batches" :loading="loading" row-key="id" clickable-rows @row-click="(r: BatchRow) => router.push(`/admin/outreach/campaigns/batches/${r.id}`)">
          <template #cell-name="{ row }">
            <span class="text-sm font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
            <p class="text-xs text-gray-400">{{ pickText(row) }} · {{ SCOPE_LABEL[row.scope as BatchScope] }}</p>
          </template>
          <template #cell-status="{ row }">
            <span class="badge" :class="BATCH_CLASS[row.status as BatchStatus]">{{ BATCH_LABEL[row.status as BatchStatus] }}</span>
            <p v-if="row.status === 'paused' && row.pause_reason" class="mt-1 max-w-[16rem] truncate text-xs text-amber-600" :title="row.pause_reason">{{ row.pause_reason }}</p>
          </template>
          <template #cell-progress="{ row }">
            <span class="text-sm tabular-nums text-gray-700 dark:text-dark-200">
              {{ row.delivery.sent }} / {{ row.total }}
              <span v-if="row.delivery.failed" class="text-red-500">（失败 {{ row.delivery.failed }}<template v-if="row.delivery.bounced">，退信 {{ row.delivery.bounced }}</template>）</span>
              <span v-if="row.delivery.skipped" class="text-gray-400">（跳过 {{ row.delivery.skipped }}）</span>
            </span>
          </template>
          <template #cell-created_at="{ value }"><span class="text-sm text-gray-500">{{ fmtTime(value) }}</span></template>
          <template #empty>
            <EmptyState title="还没有批次" description="点「生成批次」，先随机取几个人验证一下" />
          </template>
        </DataTable>
        <div class="px-4 py-3">
          <Pagination v-if="total > 0" :page="page" :total="total" :page-size="pageSize" @update:page="(p: number) => (page = p)" @update:pageSize="(s: number) => { pageSize = s; page = 1 }" />
        </div>
      </section>
    </div>

    <!-- 生成批次 -->
    <BaseDialog :show="dialog" title="生成批次" @close="dialog = false">
      <div class="space-y-4">
        <div>
          <label class="input-label">选人范围</label>
          <div class="space-y-2">
            <label v-for="s in scopes" :key="s" class="flex cursor-pointer items-start gap-2 rounded-lg border p-3" :class="req.scope === s ? 'border-primary-400 bg-primary-50 dark:bg-primary-900/20' : 'border-gray-200 dark:border-dark-700'">
              <input v-model="req.scope" type="radio" :value="s" class="mt-1" />
              <span class="text-sm">
                <span class="text-gray-900 dark:text-white">{{ SCOPE_LABEL[s] }}</span>
                <span class="ml-2 text-primary-600">{{ available ? `可选 ${s === 'fresh' ? available.available_fresh : available.available_again} 人` : '计算中…' }}</span>
                <span class="block text-xs text-gray-500">{{ s === 'fresh' ? '排除本活动已经收到过的人，适合灰度放量' : '已收到过的人也纳入，发送时仍按频控跳过近期收过邮件的人' }}</span>
              </span>
            </label>
          </div>
          <p
            v-if="req.scope === 'again' && camp?.benefits.length"
            class="mt-2 rounded-lg bg-amber-50 p-3 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
          >
            本活动附带权益（{{ camp.benefits.map(benefitText).join('、') }}）。权益按「每人每批次一次」发放：已领过的人在新批次里会拿到新的领取链接，可以<strong>再领一次</strong>。只想补发提醒、不想重复发放的话，请先复制活动并去掉权益再发。
          </p>
        </div>
        <div>
          <label class="input-label">选人方式</label>
          <Select v-model="req.mode" :options="modeOptions" />
        </div>
        <div v-if="req.mode === 'random'">
          <label class="input-label">随机人数</label>
          <input v-model.number="req.count" type="number" min="1" class="input" />
        </div>
        <div v-if="req.mode === 'users'">
          <label class="input-label">邮箱（每行一个，或用逗号分隔）</label>
          <textarea v-model="emailsText" rows="5" class="input font-mono text-sm" placeholder="a@example.com&#10;b@example.com" />
          <p class="input-hint">只有在可选范围内的邮箱会进入名单。</p>
        </div>
        <div>
          <label class="input-label">批次名称（可选）</label>
          <input v-model="req.name" class="input" maxlength="120" placeholder="留空自动命名，如「批次 #2」" />
        </div>
        <p class="text-xs text-gray-500">正在本活动其他批次里排队的人始终排除。生成后不会发送任何邮件，可以在批次详情里逐个预览。</p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="dialog = false">取消</button>
          <button type="button" class="btn btn-primary" :disabled="creating" @click="createBatch">{{ creating ? '生成中…' : '生成批次' }}</button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="confirmDelete" title="删除活动" message="将删除该活动及其全部批次。没发过邮件的批次直接删除；发过邮件的批次保留一段时间后才清理（期间频控和领取链接仍有效）。执行中或已暂停的批次需要先取消。" danger confirm-text="删除" @confirm="doDelete" @cancel="confirmDelete = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import MoreMenu from '../components/MoreMenu.vue'
import StatsPanel from '../components/StatsPanel.vue'
import {
  adminApi, fmtTime, useOutreachMeta, BATCH_CLASS, BATCH_LABEL, PICK_LABEL, SCOPE_LABEL,
  type AudiencePreview, type BatchRequest, type BatchRow, type BatchScope, type BatchStatus, type CampaignDetail, type PickMode, type Stats
} from '../api'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const { purposeOf, purposeLabel, benefitText } = useOutreachMeta()
const id = Number(route.params.id)

const camp = ref<CampaignDetail | null>(null)
const stats = ref<Stats | null>(null)
const loadError = ref('')
const busy = ref(false)
const confirmDelete = ref(false)

const marketing = computed(() => purposeOf(camp.value?.purpose ?? '')?.category === 'marketing')
const actionText = computed(() => {
  const c = camp.value
  const p = purposeOf(c?.purpose ?? '')
  if (!c || !p) return ''
  if (!p.allows_landing) return '一次性验证页'
  const landing = c.landing_url || '站点首页'
  return c.benefits.length ? `先领取权益，再到 ${landing}` : landing
})

const menu = [
  { key: 'edit', label: '编辑活动' },
  { key: 'duplicate', label: '复制为新活动' },
  { key: 'delete', label: '删除活动', danger: true }
]
function onMenu(key: string) {
  if (key === 'edit') router.push(`/admin/outreach/campaigns/${id}/edit`)
  if (key === 'duplicate') duplicate()
  if (key === 'delete') confirmDelete.value = true
}

const batches = ref<BatchRow[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)

const columns: Column[] = [
  { key: 'name', label: '批次' },
  { key: 'status', label: '状态' },
  { key: 'progress', label: '已发送 / 名单' },
  { key: 'created_at', label: '创建时间' }
]

function pickText(b: BatchRow): string {
  const base = PICK_LABEL[b.pick_mode]
  return b.pick_mode === 'all' ? base : `${base}（${b.pick_count}）`
}

async function loadCamp() {
  try {
    camp.value = await adminApi.get(id)
    stats.value = await adminApi.campaignStats(id)
  } catch (e) {
    if (!camp.value) loadError.value = extractApiErrorMessage(e, '加载失败')
  }
}

async function loadBatches(silent = false) {
  if (!silent) loading.value = true
  try {
    const r = await adminApi.listBatches(id, { page: page.value, page_size: pageSize.value })
    batches.value = r.items
    total.value = r.total
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载批次失败'))
  } finally {
    loading.value = false
  }
}

// ─── 生成批次 ────────────────────────────────────────────────────────────
const dialog = ref(false)
const creating = ref(false)
const scopes: BatchScope[] = ['fresh', 'again']
const req = reactive<Omit<BatchRequest, 'emails'>>({ name: '', mode: 'random', scope: 'fresh', count: 5 })
const emailsText = ref('')
const available = ref<AudiencePreview['notes'] | null>(null)
const modeOptions = (Object.keys(PICK_LABEL) as PickMode[]).map((k) => ({ value: k, label: PICK_LABEL[k] }))

async function openDialog() {
  Object.assign(req, { name: '', mode: 'random', scope: 'fresh', count: 5 })
  emailsText.value = ''
  available.value = null
  dialog.value = true
  if (!camp.value) return
  try {
    available.value = (await adminApi.preview(camp.value.purpose, camp.value.segment, id)).notes
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '计算可选人数失败'))
  }
}

async function createBatch() {
  creating.value = true
  try {
    const emails = emailsText.value.split(/[\s,，;；]+/).filter(Boolean)
    const r = await adminApi.createBatch(id, { ...req, emails })
    dialog.value = false
    appStore.showSuccess(
      req.mode === 'users' && r.requested > r.matched
        ? `已生成批次：填写 ${r.requested} 个邮箱，其中 ${r.matched} 个可选`
        : `已生成批次，共 ${r.matched} 人，尚未发送`
    )
    router.push(`/admin/outreach/campaigns/batches/${r.batch.id}`)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '生成批次失败'))
  } finally {
    creating.value = false
  }
}

async function duplicate() {
  busy.value = true
  try {
    const c = await adminApi.duplicate(id)
    appStore.showSuccess('已复制，正在打开副本')
    router.push(`/admin/outreach/campaigns/${c.id}/edit`)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '复制失败'))
  } finally {
    busy.value = false
  }
}

async function doDelete() {
  confirmDelete.value = false
  try {
    await adminApi.remove(id)
    router.replace('/admin/outreach/campaigns')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '删除失败'))
  }
}

watch([page, pageSize], () => loadBatches())

let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  loadCamp()
  loadBatches()
  timer = setInterval(() => {
    if (batches.value.some((b) => b.status === 'running')) {
      loadBatches(true)
      loadCamp()
    }
  }, 10_000)
})
onBeforeUnmount(() => timer && clearInterval(timer))
</script>
