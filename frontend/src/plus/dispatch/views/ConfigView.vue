<!-- [plus] 上游调度台 · 检测配置：检测计划 / 检测项 / 维度 / 检测方式 / 调度设置 -->
<template>
  <AppLayout>
    <div class="space-y-5" data-testid="dispatch-config">
      <nav class="flex gap-6 border-b border-gray-200 dark:border-dark-700">
        <button v-for="t in TABS" :key="t.k" type="button" class="-mb-px border-b-2 pb-2.5 text-sm transition-colors" :class="tab === t.k ? 'border-primary-600 font-medium text-gray-900 dark:text-white' : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-dark-200'" @click="tab = t.k">{{ t.n }}</button>
      </nav>
      <div v-if="!meta" class="py-16 text-center text-sm text-gray-400">加载中…</div>
      <template v-else>
        <!-- 检测计划 -->
        <template v-if="tab === 'plans'">
          <div class="flex flex-wrap items-center gap-3">
            <PSelect v-model="pf.item" width-class="w-44" :options="[{ value: 0, label: '全部检测项' }, ...activeItems.map((it) => ({ value: it.id, label: it.name }))]" />
            <PSelect v-model="pf.platform" width-class="w-36" :options="[{ value: '', label: '全部平台' }, ...platformOptions]" />
            <PSelect v-model="pf.enabled" width-class="w-32" :options="[{ value: '', label: '全部状态' }, { value: 'on', label: '已启用' }, { value: 'off', label: '已停用' }]" />
            <button type="button" class="btn btn-primary ml-auto" data-testid="dispatch-plan-new" @click="openPlan(null)"><Icon name="plus" size="md" class="mr-1.5" />新建计划</button>
          </div>
          <div class="card overflow-hidden">
            <DataTable :columns="planColumns" :data="plans" row-key="id" :sticky-first-column="false" :sticky-actions-column="false">
              <template #cell-item_id="{ row }"><span class="font-medium text-gray-900 dark:text-white" data-testid="dispatch-plan-row">{{ itemName(row.item_id) }}</span></template>
              <template #cell-platform="{ row }">{{ platformName(row.platform) }}</template>
              <template #cell-models="{ row }"><span class="font-mono text-xs">{{ row.models.join('、') }}</span></template>
              <template #cell-interval_minutes="{ row }"><span class="whitespace-nowrap" :title="row.enabled && row.next_run_at ? `下次 ${fullTime(row.next_run_at)}` : ''">{{ everyText(row.interval_minutes) }}</span></template>
              <template #cell-scopes="{ row }"><span class="text-xs">{{ scopeSummary(row) }}</span></template>
              <template #cell-enabled="{ row }"><Toggle :model-value="row.enabled" @update:model-value="togglePlan(row)" /></template>
              <template #cell-actions="{ row }">
                <div class="flex items-center gap-1">
                  <ActionBtn icon="edit" label="编辑" @click="openPlan(row)" />
                  <ActionBtn icon="play" label="执行" data-testid="dispatch-plan-run" :disabled="!row.enabled" @click="runPlan(row.id)" />
                  <ActionBtn icon="document" label="记录" :to="{ path: `${BASE}/runs`, query: { item: row.item_id, plan: row.id } }" />
                  <ActionBtn icon="trash" label="删除" danger @click="askDelete('plan', row.id, '删除这个检测计划？已有的检测记录会保留。')" />
                </div>
              </template>
              <template #empty><EmptyState title="还没有检测计划" action-text="新建计划" @action="openPlan(null)" /></template>
            </DataTable>
          </div>
        </template>

        <!-- 检测项 -->
        <template v-if="tab === 'items'">
          <div class="flex flex-wrap items-center gap-3">
            <PSelect v-model="itf.dim" width-class="w-36" :options="[{ value: 0, label: '全部维度' }, ...meta.dimensions.map((d) => ({ value: d.id, label: d.name }))]" />
            <PSelect v-model="itf.type" width-class="w-40" :options="[{ value: '', label: '全部检测方式' }, ...meta.types.map((t) => ({ value: t.key, label: t.name }))]" />
            <button type="button" class="btn btn-primary ml-auto" data-testid="dispatch-item-new" @click="openItem(null)"><Icon name="plus" size="md" class="mr-1.5" />新建检测项</button>
          </div>
          <div class="card overflow-hidden">
            <DataTable :columns="itemColumns" :data="items" row-key="id" :sticky-first-column="false" :sticky-actions-column="false">
              <template #cell-name="{ row }">
                <span class="font-medium text-gray-900 dark:text-white" data-testid="dispatch-item-card">{{ row.name }}</span>
                <span v-if="row.display_only" class="ml-1.5 text-xs text-gray-400">仅展示</span>
              </template>
              <template #cell-dimension_id="{ row }">{{ dimOf(row.dimension_id)?.name }}</template>
              <template #cell-type="{ row }">{{ specOf(row.type)?.name }}</template>
              <template #cell-scope="{ row }"><span class="text-xs">{{ itemScope(row) }}</span></template>
              <template #cell-enabled="{ row }"><Toggle :model-value="row.enabled" @update:model-value="toggleItem(row)" /></template>
              <template #cell-actions="{ row }">
                <div class="flex items-center gap-1">
                  <ActionBtn icon="edit" label="编辑" @click="openItem(row)" />
                  <ActionBtn icon="trash" label="删除" danger @click="askDelete('item', row.id, `删除检测项「${row.name}」？它的计划和检测记录会一起删除。`)" />
                </div>
              </template>
            </DataTable>
          </div>
        </template>

        <DimsTab v-if="tab === 'dims'" />

        <!-- 检测方式 -->
        <div v-if="tab === 'types'" class="grid gap-4 md:grid-cols-2">
          <div v-for="t in meta.types" :key="t.key" class="card p-5">
            <div class="flex items-center gap-2">
              <span class="font-medium text-gray-900 dark:text-white">{{ t.name }}</span>
              <span class="rounded-md px-1.5 py-0.5 text-xs" :class="t.active ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300' : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300'">{{ t.active ? '主动检测' : '流量统计' }}</span>
            </div>
            <p class="mt-2 text-sm text-gray-600 dark:text-dark-200">{{ t.description }}</p>
            <div class="mt-3 text-xs text-gray-400">{{ meta.items.filter((i) => i.type === t.key).map((i) => i.name).join('、') || '暂无检测项' }}</div>
          </div>
        </div>

        <StrategyTab v-if="tab === 'strategy'" />
      </template>

      <PlanDialog :show="planDialog.show" :plan="planDialog.plan" :item-id="pf.item || undefined" @close="planDialog.show = false" @saved="afterPlan" />
      <BaseDialog :show="itemDialog.show" :title="itemDialog.item ? '编辑检测项' : '新建检测项'" width="wide" @close="itemDialog.show = false">
        <ItemForm v-if="itemDialog.show" :item="itemDialog.item" @cancel="itemDialog.show = false" @saved="afterItem" />
      </BaseDialog>
      <ConfirmDialog :show="!!pending" title="确认删除" :message="pending?.msg ?? ''" confirm-text="删除" danger @confirm="doDelete" @cancel="pending = null" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Item, type Plan } from '../api'
import DimsTab from '../config/DimsTab.vue'
import ItemForm from '../config/ItemForm.vue'
import PlanDialog from '../config/PlanDialog.vue'
import StrategyTab from '../config/StrategyTab.vue'
import { DISPATCH_BASE as BASE } from '../routes'
import ActionBtn from '../shared/ActionBtn.vue'
import { everyText, fullTime, platformName } from '../shared/format'
import { dimOf, loadMeta, meta, specOf } from '../shared/meta'
import PSelect from '../shared/PSelect.vue'

const TABS = [{ k: 'plans', n: '检测计划' }, { k: 'items', n: '检测项' }, { k: 'dims', n: '维度' }, { k: 'types', n: '检测方式' }, { k: 'strategy', n: '调度设置' }]
const planColumns = [
  { key: 'item_id', label: '检测项' }, { key: 'platform', label: '平台' }, { key: 'models', label: '模型' },
  { key: 'interval_minutes', label: '频率' }, { key: 'scopes', label: '账号范围' },
  { key: 'enabled', label: '启用' }, { key: 'actions', label: '操作' }
]
const itemColumns = [
  { key: 'name', label: '名称' }, { key: 'dimension_id', label: '维度' }, { key: 'type', label: '检测方式' },
  { key: 'scope', label: '范围' }, { key: 'enabled', label: '启用' }, { key: 'actions', label: '操作' }
]

const route = useRoute()
const router = useRouter()
const app = useAppStore()
const tab = ref(String(route.query.tab ?? 'plans'))
const pf = reactive({ item: Number(route.query.item) || 0, platform: '', enabled: '' })
const itf = reactive({ dim: 0, type: '' })
void loadMeta(true)
watch(tab, (t) => router.replace({ query: { tab: t } }))
watch(() => route.query.tab, (t) => { if (t) tab.value = String(t) })

const platformOptions = computed(() => (meta.value?.platforms ?? []).map((p) => ({ value: p, label: platformName(p) })))
const activeItems = computed(() => (meta.value?.items ?? []).filter((i) => specOf(i.type)?.active))
const itemName = (id: number) => meta.value?.items.find((i) => i.id === id)?.name ?? `#${id}`
const plans = computed(() => (meta.value?.plans ?? []).filter((p) =>
  (!pf.item || p.item_id === pf.item) && (!pf.platform || p.platform === pf.platform) && (!pf.enabled || p.enabled === (pf.enabled === 'on'))))
const items = computed(() => (meta.value?.items ?? []).filter((i) => (!itf.dim || i.dimension_id === itf.dim) && (!itf.type || i.type === itf.type)))

function scopeSummary(p: Plan) {
  const parts = p.scopes.map((s) => {
    const sp = meta.value?.scopes.find((x) => x.key === s.type)
    if (!sp) return s.type
    const f = sp.fields[0]
    if (f?.kind === 'number') return `${f.prefix ?? ''} ${s.params?.[f.key]}${f.suffix ?? ''}`.trim()
    if (f?.kind === 'accounts') return `${sp.name} ${s.account_ids?.length ?? 0} 个`
    return sp.name
  })
  if (p.exclude_ids.length) parts.push(`排除 ${p.exclude_ids.length} 个`)
  return parts.join(' · ')
}
function itemScope(it: Item) {
  if (specOf(it.type)?.active) {
    const ps = (meta.value?.plans ?? []).filter((p) => p.item_id === it.id)
    return ps.length ? ps.map((p) => `${platformName(p.platform)} ${everyText(p.interval_minutes)}`).join('、') : '未建计划'
  }
  return it.platforms.map(platformName).join('、') || '—'
}

const planDialog = reactive<{ show: boolean; plan: Plan | null }>({ show: false, plan: null })
const itemDialog = reactive<{ show: boolean; item: Item | null }>({ show: false, item: null })
function openPlan(p: Plan | null) {
  planDialog.plan = p
  planDialog.show = true
}
function openItem(it: Item | null) {
  itemDialog.item = it
  itemDialog.show = true
}
async function afterPlan() {
  planDialog.show = false
  await loadMeta(true)
}
async function afterItem() {
  itemDialog.show = false
  await loadMeta(true)
}

async function act(fn: () => Promise<unknown>, ok: string) {
  try {
    await fn()
    app.showSuccess(ok)
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '操作失败'))
  }
  await loadMeta(true)
}
const pending = ref<{ kind: 'item' | 'plan'; id: number; msg: string } | null>(null)
function askDelete(kind: 'item' | 'plan', id: number, msg: string) {
  pending.value = { kind, id, msg }
}
function doDelete() {
  const p = pending.value
  pending.value = null
  if (p) void act(() => (p.kind === 'item' ? dispatchApi.deleteItem(p.id) : dispatchApi.deletePlan(p.id)), '已删除')
}
const togglePlan = (p: Plan) => act(() => dispatchApi.savePlan({ ...p, enabled: !p.enabled }), p.enabled ? '已停用' : '已启用')
const toggleItem = (it: Item) => act(() => dispatchApi.saveItem({ ...it, enabled: !it.enabled }), it.enabled ? '已停用' : '已启用')
async function runPlan(id: number) {
  try {
    const r = await dispatchApi.runPlan(id)
    app.showSuccess(r.queued ? `已开始 ${r.queued} 次检测` : '没有可测的账号')
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '执行失败'))
  }
}
</script>
