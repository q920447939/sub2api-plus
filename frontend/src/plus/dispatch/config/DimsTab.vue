<!-- [plus] 维度：作用决定调度怎么用它的结论；可以新建、编辑、排序、停用，没有检测项的可以删 -->
<template>
  <div class="space-y-5">
    <div class="flex items-center">
      <button type="button" class="btn btn-primary ml-auto" data-testid="dispatch-dim-new" @click="open(null)"><Icon name="plus" size="md" class="mr-1.5" />新建维度</button>
    </div>
    <div class="card overflow-hidden">
      <DataTable :columns="columns" :data="dims" row-key="id" :sticky-first-column="false" :sticky-actions-column="false">
        <template #cell-sort="{ row }">
          <div class="flex items-center text-gray-400">
            <button type="button" class="rounded p-1 hover:bg-gray-100 disabled:opacity-30 dark:hover:bg-dark-700" :disabled="idx(row) === 0" @click="move(idx(row), -1)"><Icon name="arrowUp" size="sm" /></button>
            <button type="button" class="rounded p-1 hover:bg-gray-100 disabled:opacity-30 dark:hover:bg-dark-700" :disabled="idx(row) === dims.length - 1" @click="move(idx(row), 1)"><Icon name="arrowDown" size="sm" /></button>
          </div>
        </template>
        <template #cell-name="{ row }"><span class="font-medium text-gray-900 dark:text-white" data-testid="dispatch-dim-row">{{ row.name }}</span></template>
        <template #cell-role="{ row }"><span class="rounded-md px-1.5 py-0.5 text-xs" :class="ROLE_CLASS[row.role as Role]">{{ ROLE_TEXT[row.role as Role] }}</span></template>
        <template #cell-items="{ row }"><span class="text-xs">{{ itemsOf(row.id).map((i) => i.name).join('、') || '—' }}</span></template>
        <template #cell-handling="{ row }"><span class="text-xs text-gray-500">{{ handling(row) }}</span></template>
        <template #cell-enabled="{ row }"><Toggle :model-value="row.enabled" @update:model-value="(v: boolean) => save({ ...row, enabled: v })" /></template>
        <template #cell-actions="{ row }">
          <div class="flex items-center gap-1">
            <ActionBtn icon="edit" label="编辑" @click="open(row)" />
            <ActionBtn icon="trash" label="删除" danger :disabled="itemsOf(row.id).length > 0" :title="itemsOf(row.id).length ? '先移走它下面的检测项' : ''" @click="pendingDelete = row" />
          </div>
        </template>
      </DataTable>
    </div>

    <BaseDialog :show="dialog" :title="draft.id ? '编辑维度' : '新建维度'" width="normal" @close="dialog = false">
      <form id="dispatch-dim-form" class="space-y-5" data-testid="dispatch-dim-dialog" @submit.prevent="submit">
        <div>
          <label class="input-label">名称</label>
          <input v-model="draft.name" class="input" placeholder="例如：上下文" />
        </div>
        <div>
          <label class="input-label">作用</label>
          <PSelect v-model="draft.role" block :options="roleOptions" />
          <p class="input-hint">{{ ROLE_DESC[draft.role] }}</p>
        </div>
        <div v-if="draft.role === 'gate'" class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">判为异常，确认前</label>
            <div class="flex items-center gap-2">
              <PSelect v-model="draft.gate.before" block :options="GATE_BEFORE" />
              <template v-if="draft.gate.before === 'cut'"><input v-model.number="draft.gate.before_pct" type="number" class="input w-20 text-center" /><span class="text-sm text-gray-500">%</span></template>
            </div>
          </div>
          <div>
            <label class="input-label">管理员确认后</label>
            <div class="flex items-center gap-2">
              <PSelect v-model="draft.gate.after" block :options="GATE_AFTER" />
              <template v-if="draft.gate.after === 'cut'"><input v-model.number="draft.gate.after_pct" type="number" class="input w-20 text-center" /><span class="text-sm text-gray-500">%</span></template>
            </div>
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="dialog = false">取消</button>
          <button type="submit" form="dispatch-dim-form" class="btn btn-primary" data-testid="dispatch-dim-create" :disabled="!draft.name.trim()">{{ draft.id ? '保存' : '创建' }}</button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="!!pendingDelete" title="删除维度" :message="`删除维度「${pendingDelete?.name}」？`" confirm-text="删除" danger @confirm="doDelete" @cancel="pendingDelete = null" />
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Dimension, type Role } from '../api'
import ActionBtn from '../shared/ActionBtn.vue'
import { ROLE_CLASS, ROLE_DESC, ROLE_TEXT } from '../shared/format'
import { loadMeta, meta } from '../shared/meta'
import PSelect from '../shared/PSelect.vue'

const GATE_BEFORE = [{ value: 'cut', label: '降到' }, { value: 'none', label: '只提醒' }, { value: 'stop', label: '停止分配' }]
const GATE_AFTER = [{ value: 'stop', label: '停止分配' }, { value: 'cut', label: '降到' }]
const roleOptions = Object.entries(ROLE_TEXT).map(([value, label]) => ({ value, label }))
const columns = [
  { key: 'sort', label: '' }, { key: 'name', label: '名称' }, { key: 'role', label: '作用' }, { key: 'items', label: '检测项' },
  { key: 'handling', label: '异常时' }, { key: 'enabled', label: '启用' }, { key: 'actions', label: '操作' }
]

const app = useAppStore()
const dims = computed(() => meta.value?.dimensions ?? [])
const idx = (d: Dimension) => dims.value.findIndex((x) => x.id === d.id)
const itemsOf = (id: number) => (meta.value?.items ?? []).filter((i) => i.dimension_id === id)
const blank = (): Dimension => ({ id: 0, name: '', role: 'display', enabled: true, sort: 0, gate: { before: 'cut', before_pct: 3, after: 'stop', after_pct: 0 } })
const draft = reactive<Dimension>(blank())
const dialog = ref(false)
const pendingDelete = ref<Dimension | null>(null)

function handling(d: Dimension) {
  if (d.role === 'display') return '不影响流量'
  if (d.role === 'score') return `份额 ×${meta.value?.config.bad_factor}（可疑 ×${meta.value?.config.warn_factor}）`
  const t = (a: string, p: number) => (a === 'stop' ? '停止分配' : a === 'cut' ? `降到 ${p}%` : '只提醒')
  return `确认前${t(d.gate.before, d.gate.before_pct)}，确认后${t(d.gate.after, d.gate.after_pct)}`
}

async function run(fn: () => Promise<unknown>, ok: string): Promise<boolean> {
  let done = false
  try {
    await fn()
    app.showSuccess(ok)
    done = true
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '操作失败'))
  }
  await loadMeta(true)
  return done
}
const save = (d: Dimension) => run(() => dispatchApi.saveDimension(d), '已保存')
function open(d: Dimension | null) {
  Object.assign(draft, blank(), d ? JSON.parse(JSON.stringify(d)) : {})
  dialog.value = true
}
async function submit() {
  if (await run(() => dispatchApi.saveDimension({ ...draft }), draft.id ? '已保存' : `已创建维度「${draft.name}」`)) dialog.value = false
}
async function doDelete() {
  const d = pendingDelete.value
  pendingDelete.value = null
  if (d) await run(() => dispatchApi.deleteDimension(d.id), '已删除')
}
function move(i: number, d: number) {
  const ids = dims.value.map((x) => x.id)
  ;[ids[i], ids[i + d]] = [ids[i + d], ids[i]]
  void run(() => dispatchApi.reorderDimensions(ids), '已调整顺序')
}
</script>
