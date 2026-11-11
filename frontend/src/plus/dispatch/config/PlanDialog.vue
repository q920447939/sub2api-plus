<!-- [plus] 新建 / 编辑检测计划（弹窗）。「测哪些账号」的规则由后端注册表声明（meta.scopes），多选、命中任意一条就测。 -->
<template>
  <BaseDialog :show="show" :title="form.id ? '编辑检测计划' : '新建检测计划'" width="wide" @close="emit('close')">
    <form id="dispatch-plan-form" class="space-y-5" data-testid="dispatch-plan-form" @submit.prevent="save">
      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="input-label">检测项</label>
          <PSelect v-model="form.item_id" block :disabled="!!form.id" placeholder="选择检测项" :groups="itemGroups" @change="onItem" />
        </div>
        <div>
          <label class="input-label">平台</label>
          <PSelect v-model="form.platform" block :disabled="!!form.id" placeholder="选择平台" :options="platformOptions" @change="form.models = []" />
        </div>
      </div>

      <template v-if="item && form.platform">
        <div>
          <label class="input-label">模型</label>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="m in platformModels"
              :key="m.model"
              type="button"
              :disabled="!!m.disabled"
              :title="m.disabled"
              class="rounded-lg border px-2.5 py-1 text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-50"
              :class="form.models.includes(m.model) ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-dark-200'"
              @click="toggle(form.models, m.model)"
            >
              <span class="font-mono">{{ m.model }}</span>
              <span v-if="m.disabled" class="ml-1">· {{ m.disabled }}</span>
              <span v-else-if="m.share" class="ml-1 text-gray-400">{{ m.share }}%</span>
            </button>
            <input v-model="extraModel" class="input w-44 py-1 text-xs" placeholder="其他模型，回车添加" @keydown.enter.prevent="addModel" />
          </div>
        </div>

        <div>
          <label class="input-label">测哪些账号</label>
          <div class="divide-y divide-gray-100 rounded-xl border border-gray-200 dark:divide-dark-700 dark:border-dark-600">
            <div v-for="sp in meta?.scopes ?? []" :key="sp.key" class="px-4 py-3">
              <label class="flex cursor-pointer items-center gap-3">
                <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" :checked="!!scopeOf(sp.key)" :data-testid="`dispatch-scope-${sp.key}`" @change="toggleScope(sp)" />
                <span class="text-sm font-medium text-gray-800 dark:text-dark-100">{{ sp.name }}</span>
                <template v-for="f in sp.fields" :key="f.key">
                  <span v-if="f.kind === 'number' && scopeOf(sp.key)" class="ml-auto flex items-center gap-1.5 text-sm text-gray-500" @click.prevent>
                    {{ f.prefix }}
                    <input v-model.number="scopeOf(sp.key)!.params![f.key]" type="number" :min="f.min" :max="f.max" class="input w-16 px-2 py-1 text-center" />
                    {{ f.suffix }}
                  </span>
                </template>
              </label>
              <template v-for="f in sp.fields" :key="f.key">
                <div v-if="f.kind === 'accounts' && scopeOf(sp.key)" class="mt-3 pl-7">
                  <AccountPicker v-model="scopeOf(sp.key)!.account_ids!" :platform="form.platform" />
                </div>
              </template>
            </div>
          </div>
        </div>

        <div>
          <label class="input-label">排除账号</label>
          <AccountPicker v-model="form.exclude_ids" :platform="form.platform" />
        </div>

        <div class="grid gap-4 sm:grid-cols-3">
          <div>
            <label class="input-label">频率</label>
            <PSelect v-model="form.interval_minutes" block :options="intervalOptions" />
          </div>
          <div>
            <label class="input-label">单次超时（秒）</label>
            <input v-model.number="form.timeout_seconds" type="number" min="10" max="1800" class="input" />
          </div>
          <div>
            <label class="input-label">启用</label>
            <div class="flex h-[38px] items-center"><Toggle v-model="form.enabled" /></div>
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 px-4 py-3 text-sm dark:bg-dark-800">
          <button type="button" class="flex w-full items-center gap-2 text-left" :disabled="!preview" @click="showRows = !showRows">
            <span v-if="previewError" class="text-red-600">{{ previewError }}</span>
            <span v-else-if="!form.models.length" class="text-gray-400">选择模型后显示会测哪些账号</span>
            <span v-else-if="preview" class="text-gray-700 dark:text-dark-100">
              每轮 <b>{{ preview.targets }}</b> 个账号 × 模型 · 每天约 <b>{{ preview.requests_per_day }}</b> 次请求
            </span>
            <Icon v-if="preview?.rows?.length" name="chevronDown" size="sm" class="ml-auto text-gray-400 transition-transform" :class="showRows ? 'rotate-180' : ''" />
          </button>
          <table v-if="showRows && preview?.rows?.length" class="mt-3 w-full text-xs">
            <tbody>
              <tr v-for="r in preview.rows" :key="r.account_id + r.model" class="border-t border-gray-200/70 dark:border-dark-700">
                <td class="py-1.5 pr-2 text-gray-800 dark:text-dark-100">{{ accountName(r.account_id) }}</td>
                <td class="pr-2 font-mono text-gray-500">{{ r.model }}</td>
                <td class="text-right">
                  <span v-if="r.ok && r.same_as" class="text-gray-400">与 {{ accountName(r.same_as) }} 同凭证</span>
                  <span v-else-if="r.ok" class="text-green-600">{{ r.matched?.join('、') }}</span>
                  <span v-else class="text-gray-400">{{ r.why }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
        <button type="submit" form="dispatch-plan-form" class="btn btn-primary" :disabled="saving || !item || !form.platform" data-testid="dispatch-plan-save">
          {{ saving ? '保存中…' : form.id ? '保存' : '创建' }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Plan, type PlanCoverRow, type PlanScope, type ScopeSpec } from '../api'
import AccountPicker from '../shared/AccountPicker.vue'
import { everyText, platformName } from '../shared/format'
import { meta, specOf } from '../shared/meta'
import PSelect from '../shared/PSelect.vue'

const INTERVALS = [10, 15, 30, 60, 120, 360, 720, 1440]
const DEFAULTS: Record<string, [number, number]> = { fingerprint: [60, 180], graded: [360, 300], archive: [1440, 600] }
const props = defineProps<{ show: boolean; plan?: Plan | null; itemId?: number }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const app = useAppStore()

const blank = (): Plan => ({ id: 0, item_id: 0, platform: '', models: [], scopes: [], exclude_ids: [], interval_minutes: 60, timeout_seconds: 180, enabled: true })
const form = reactive<Plan>(blank())
const saving = ref(false)
const extraModel = ref('')
const preview = ref<{ rows: PlanCoverRow[]; targets: number; requests_per_day: number } | null>(null)
const previewError = ref('')
const showRows = ref(false)

watch(() => props.show, (open) => {
  if (!open) return
  Object.assign(form, blank(), props.plan ? JSON.parse(JSON.stringify(props.plan)) : {})
  if (!props.plan) {
    form.item_id = props.itemId ?? 0
    form.scopes = defaultScopes()
    if (form.item_id) onItem()
  }
  preview.value = null
  showRows.value = false
}, { immediate: true })

// 新建时默认勾上带数字参数的规则（按流量、新接入），指定账号默认不勾
function defaultScopes(): PlanScope[] {
  return (meta.value?.scopes ?? []).filter((s) => s.fields.every((f) => f.kind === 'number')).map(newScope)
}
function newScope(sp: ScopeSpec): PlanScope {
  const s: PlanScope = { type: sp.key }
  for (const f of sp.fields) {
    if (f.kind === 'number') s.params = { ...(s.params ?? {}), [f.key]: f.default ?? 0 }
    if (f.kind === 'accounts') s.account_ids = []
  }
  return s
}
const scopeOf = (key: string) => form.scopes.find((s) => s.type === key)
function toggleScope(sp: ScopeSpec) {
  const i = form.scopes.findIndex((s) => s.type === sp.key)
  if (i >= 0) form.scopes.splice(i, 1)
  else form.scopes.push(newScope(sp))
}

const activeItems = computed(() => (meta.value?.items ?? []).filter((i) => specOf(i.type)?.active))
const item = computed(() => activeItems.value.find((i) => i.id === form.item_id))
const itemGroups = computed(() => (meta.value?.dimensions ?? []).map((d) => ({
  label: d.name,
  options: activeItems.value.filter((i) => i.dimension_id === d.id).map((it) => ({ value: it.id, label: it.name }))
})))
const platformOptions = computed(() => (meta.value?.platforms ?? []).map((p) => ({ value: p, label: platformName(p) })))
const intervalOptions = computed(() => {
  const list = INTERVALS.includes(form.interval_minutes) ? INTERVALS : [...INTERVALS, form.interval_minutes].sort((a, b) => a - b)
  return list.map((m) => ({ value: m, label: everyText(m) }))
})

function onItem() {
  const d = DEFAULTS[item.value?.type ?? ''] ?? [60, 180]
  form.interval_minutes = d[0]
  form.timeout_seconds = d[1]
}

const libModels = computed(() => {
  const l = meta.value?.fingerprint_library
  if (item.value?.type !== 'fingerprint' || !l?.known) return null
  return new Set((l.models ?? []).map((m) => m.toLowerCase().replace(/[._]/g, '-')))
})
const platformModels = computed(() => {
  const list = [...(meta.value?.models[form.platform] ?? [])]
  for (const m of form.models) if (!list.find((x) => x.model === m)) list.push({ model: m, share: 0 })
  const taken = (meta.value?.plans ?? []).filter((p) => p.item_id === form.item_id && p.platform === form.platform && p.id !== form.id).flatMap((p) => p.models)
  return list.map((x) => {
    const norm = x.model.toLowerCase().replace(/[._]/g, '-').replace(/-(\d{8}|latest)$/, '').split('/').pop()!
    const disabled = taken.includes(x.model) ? '已有计划' : libModels.value && !libModels.value.has(norm) ? '不支持' : ''
    return { ...x, disabled }
  })
})
function addModel() {
  const m = extraModel.value.trim()
  if (m && !form.models.includes(m)) form.models.push(m)
  extraModel.value = ''
}
function toggle(arr: string[], v: string) {
  const i = arr.indexOf(v)
  if (i >= 0) arr.splice(i, 1)
  else arr.push(v)
}
const accountName = (id: number) => meta.value?.accounts.find((a) => a.id === id)?.name ?? `#${id}`

let timer: ReturnType<typeof setTimeout> | undefined
watch(() => JSON.stringify([props.show, form.item_id, form.platform, form.models, form.scopes, form.exclude_ids, form.interval_minutes]), () => {
  clearTimeout(timer)
  timer = setTimeout(async () => {
    if (!props.show || !form.item_id || !form.platform || !form.models.length) {
      preview.value = null
      return
    }
    previewError.value = ''
    try {
      preview.value = await dispatchApi.previewPlan({ ...form, enabled: true })
    } catch (e) {
      preview.value = null
      previewError.value = extractApiErrorMessage(e, '预览失败')
    }
  }, 300)
})

async function save() {
  saving.value = true
  try {
    await dispatchApi.savePlan(form)
    app.showSuccess(form.id ? '计划已保存' : '计划已创建')
    emit('saved')
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}
</script>
