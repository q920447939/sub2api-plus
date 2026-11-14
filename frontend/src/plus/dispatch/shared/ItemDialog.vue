<!-- [plus] 新建 / 编辑检测项（连同计划一起保存）。表单按检测方式的声明渲染：要不要题面、标准答案、判定规则里有哪些数字。
     计划 = 一个平台 × 若干模型 × 账号（默认全部，也可以手动指定）× 每 N 分钟，每个计划可单独停用。 -->
<template>
  <BaseDialog :show="show" :title="item?.id ? `编辑「${item.name}」` : '新建检测项'" width="wide" @close="emit('close')">
    <div v-if="item && meta" class="space-y-4" data-testid="dispatch-item-dialog">
      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="input-label">名称</label>
          <input v-model="item.name" class="input" placeholder="比如：糖果题" />
        </div>
        <div>
          <label class="input-label">检测方式</label>
          <Select v-if="!item.id" v-model="item.type" :options="meta.types.map((t) => ({ value: t.key, label: t.name }))" @change="onType" />
          <div v-else class="input bg-gray-50 dark:bg-dark-800">{{ spec?.name }}</div>
        </div>
      </div>
      <p v-if="spec" class="-mt-2 text-xs text-gray-500">{{ spec.description }}</p>
      <div v-if="spec?.has_prompt">
        <label class="input-label">题面</label>
        <textarea v-model="item.params.prompt" class="input min-h-[120px]" />
      </div>
      <div v-if="spec?.has_answers" class="grid gap-4 sm:grid-cols-[1fr_180px]">
        <div>
          <label class="input-label">标准答案（多个用 | 隔开，答对任意一个都算对）</label>
          <input v-model="answers" class="input" />
        </div>
        <div>
          <label class="input-label">怎么比对</label>
          <Select v-model="item.params.match" :options="MATCH" />
        </div>
      </div>
      <div v-if="spec?.rule_fields.length">
        <label class="input-label">怎么算未通过</label>
        <div class="flex flex-wrap items-center gap-1.5 text-sm">
          <template v-for="(part, i) in ruleParts" :key="i">
            <input v-if="part.field" v-model.number="item.rule[part.field.key]" type="number" :min="part.field.min" :max="part.field.max" class="input w-16 px-2 py-1 text-center" />
            <span v-else>{{ part.text }}</span>
          </template>
        </div>
      </div>

      <div class="flex items-center border-t border-gray-100 pt-4 dark:border-dark-700">
        <span class="text-sm font-medium">计划</span>
        <span class="ml-2 text-xs text-gray-400">在哪个平台、测哪些模型和账号、多久一次</span>
        <button type="button" class="btn btn-secondary btn-sm ml-auto" @click="addPlan">加一个计划</button>
      </div>
      <div v-if="!plans.length" class="py-2 text-sm text-gray-400">还没有计划，不会定时检测（可以在检测详情里手动测）</div>
      <div v-for="(p, i) in plans" :key="i" class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600" data-testid="dispatch-plan-row">
        <div class="flex flex-wrap items-center gap-3">
          <Select v-model="p.platform" :options="meta.platforms.map((x) => ({ value: x, label: platformName(x) }))" class="w-36" @change="p.models = []; p.account_ids = []" />
          <span class="text-sm text-gray-500">每</span>
          <input v-model.number="p.interval_minutes" type="number" min="1" class="input w-20 px-2 py-1 text-center" />
          <span class="text-sm text-gray-500">分钟测一次</span>
          <span class="ml-auto flex items-center gap-2 text-sm text-gray-500">启用 <Toggle v-model="p.enabled" /></span>
          <button type="button" class="btn btn-ghost btn-sm text-red-600" @click="plans.splice(i, 1)">删除</button>
        </div>
        <div>
          <div class="mb-1.5 text-xs text-gray-500">模型（后面是占这个平台近 24 小时请求的比例）</div>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="m in modelOptions(p)" :key="m.model" type="button" class="rounded-lg px-2.5 py-1 font-mono text-xs"
              :class="p.models.includes(m.model) ? 'bg-primary-50 font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'"
              @click="toggle(p.models, m.model)"
            >{{ m.model }}<span v-if="m.share" class="ml-1 font-normal opacity-60">{{ m.share < 1 ? '<1' : Math.round(m.share) }}%</span></button>
            <input v-model="extra[i]" class="input w-40 px-2 py-1 font-mono text-xs" placeholder="其他模型，回车添加" @keydown.enter.prevent="addModel(p, i)" @blur="addModel(p, i)" />
          </div>
        </div>
        <div>
          <div class="mb-1.5 flex items-center gap-3 text-xs text-gray-500">
            账号
            <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
              <button type="button" class="rounded-md px-2.5 py-0.5" :class="!p.account_ids.length && !picking[i] ? 'bg-white font-semibold shadow-sm dark:bg-dark-900' : ''" @click="p.account_ids = []; picking[i] = false">全部账号</button>
              <button type="button" class="rounded-md px-2.5 py-0.5" :class="p.account_ids.length || picking[i] ? 'bg-white font-semibold shadow-sm dark:bg-dark-900' : ''" @click="picking[i] = true">指定账号</button>
            </div>
            <span v-if="!p.account_ids.length && !picking[i]">调度开着、自己提供这个模型的账号都测</span>
          </div>
          <div v-if="p.account_ids.length || picking[i]" class="flex flex-wrap gap-1.5">
            <button
              v-for="a in meta.accounts[p.platform] ?? []" :key="a.id" type="button" class="rounded-lg px-2.5 py-1 text-xs"
              :class="p.account_ids.includes(a.id) ? 'bg-primary-50 font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'"
              :title="!a.serving ? '调度已关闭' : !a.direct ? a.why : ''" @click="toggle(p.account_ids, a.id)"
            >{{ a.name }}<span v-if="!a.serving" class="ml-1 opacity-60">（调度已关闭）</span></button>
          </div>
        </div>
        <div class="text-xs text-gray-500">
          会测 <b class="text-gray-900 dark:text-white">{{ cover[i]?.filter((r) => r.ok && !r.same_as).length ?? '…' }}</b> 个账号 × 模型
          <span v-if="skipped(i).length" class="ml-2 text-amber-600" :title="skipped(i).join('\n')">另有 {{ skipped(i).length }} 个测不了</span>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="saving" data-testid="dispatch-item-save" @click="save">保存</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type CoverRow, type Item, type ItemView, type Meta, type Plan } from '../api'
import { platformName } from './format'

const props = defineProps<{ show: boolean; source: ItemView | null; meta: Meta | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const appStore = useAppStore()
const MATCH = [{ value: 'num', label: '数值相等' }, { value: 'exact', label: '完全一致' }, { value: 'contains', label: '包含' }, { value: 'regex', label: '正则匹配' }]

const item = ref<Item | null>(null)
const plans = ref<Plan[]>([])
const answers = ref('')
const extra = reactive<Record<number, string>>({})
const picking = reactive<Record<number, boolean>>({})
const cover = ref<Record<number, CoverRow[]>>({})
const saving = ref(false)

const spec = computed(() => props.meta?.types.find((t) => t.key === item.value?.type))

watch(
  () => props.show,
  (on) => {
    if (!on) return
    const s = props.source
    const type = s?.type ?? props.meta?.types.find((t) => t.key === 'graded')?.key ?? props.meta?.types[0]?.key ?? ''
    item.value = s
      ? { id: s.id, name: s.name, type: s.type, params: JSON.parse(JSON.stringify(s.params ?? {})), rule: JSON.parse(JSON.stringify(s.rule ?? {})), enabled: s.enabled }
      : { id: 0, name: '', type, params: { match: 'num' }, rule: {}, enabled: true }
    plans.value = (s?.plans ?? []).map((p) => ({ ...p, models: [...p.models], account_ids: [...(p.account_ids ?? [])] }))
    answers.value = ((item.value.params.answers as string[]) ?? []).join(' | ')
    Object.keys(picking).forEach((k) => delete picking[Number(k)])
    onType()
  }
)

function onType() {
  const sp = spec.value
  if (!sp || !item.value) return
  for (const f of sp.rule_fields) if (item.value.rule[f.key] == null) item.value.rule[f.key] = DEFAULTS[f.key] ?? f.min
  if (sp.has_answers && !item.value.params.match) item.value.params.match = 'num'
}
const DEFAULTS: Record<string, number> = { window: 10, bad_min: 3, bad_below: 3, min_confidence: 60 }

// 判定规则的句子：{key} 位置放数字框
const ruleParts = computed(() => {
  const sp = spec.value
  if (!sp) return []
  return sp.rule_text.split(/(\{\w+\})/).filter(Boolean).map((t) => {
    const m = /^\{(\w+)\}$/.exec(t)
    const field = m ? sp.rule_fields.find((f) => f.key === m[1]) : undefined
    return field ? { field } : { text: t }
  })
})

function modelOptions(p: Plan) {
  const list = props.meta?.models[p.platform] ?? []
  const extras = p.models.filter((m) => !list.some((x) => x.model === m)).map((m) => ({ model: m, n: 0, share: 0 }))
  return [...list.slice(0, 16), ...extras]
}

function toggle<T>(arr: T[], v: T) {
  const i = arr.indexOf(v)
  if (i >= 0) arr.splice(i, 1)
  else arr.push(v)
}

function addModel(p: Plan, i: number) {
  const m = (extra[i] ?? '').trim()
  if (m && !p.models.includes(m)) p.models.push(m)
  extra[i] = ''
}

function addPlan() {
  plans.value.push({
    id: 0, item_id: item.value?.id ?? 0, platform: props.meta?.platforms[0] ?? '', models: [], account_ids: [],
    interval_minutes: spec.value?.default_every ?? 60, timeout_seconds: 180, enabled: true
  })
}

const skipped = (i: number) => (cover.value[i] ?? []).filter((r) => !r.ok).map((r) => `${props.meta?.accounts[plans.value[i].platform]?.find((a) => a.id === r.account_id)?.name ?? r.account_id} · ${r.model}：${r.why}`)

// 每个计划会测几个（改了就重新算）
let timer: ReturnType<typeof setTimeout> | undefined
watch(
  () => JSON.stringify(plans.value),
  () => {
    clearTimeout(timer)
    timer = setTimeout(async () => {
      const out: Record<number, CoverRow[]> = {}
      await Promise.all(
        plans.value.map(async (p, i) => {
          if (!p.models.length) return (out[i] = [])
          try {
            out[i] = await dispatchApi.previewPlan(item.value?.type ?? '', p)
          } catch {
            out[i] = []
          }
        })
      )
      cover.value = out
    }, 250)
  }
)

async function save() {
  const it = item.value
  if (!it) return
  if (spec.value?.has_answers) it.params.answers = answers.value.split('|').map((x) => x.trim()).filter(Boolean)
  saving.value = true
  try {
    await dispatchApi.saveItem(it, plans.value)
    appStore.showSuccess('已保存')
    emit('saved')
    emit('close')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  } finally {
    saving.value = false
  }
}
</script>
