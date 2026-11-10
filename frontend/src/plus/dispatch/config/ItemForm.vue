<!-- [plus] 检测项表单：参数和判定规则的字段由检测方式决定（新建和修改共用，放在弹窗里） -->
<template>
  <div class="space-y-5">
    <div v-if="!form.id">
      <label class="input-label">检测方式</label>
      <div class="grid gap-2 sm:grid-cols-3">
        <button v-for="t in meta?.types ?? []" :key="t.key" type="button" class="rounded-xl border px-3 py-2.5 text-left text-sm transition-colors" :class="form.type === t.key ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 text-gray-700 hover:border-gray-300 dark:border-dark-600 dark:text-dark-100'" @click="pickType(t.key)">
          {{ t.name }}
        </button>
      </div>
    </div>
    <template v-if="form.type">
      <div class="grid gap-4 sm:grid-cols-2">
        <div><label class="input-label">名称</label><input v-model="form.name" class="input" placeholder="例如：逻辑推理题 2" /></div>
        <div><label class="input-label">维度</label><PSelect v-model="form.dimension_id" block :options="(meta?.dimensions ?? []).map((d) => ({ value: d.id, label: d.name }))" /></div>
      </div>

      <div v-if="form.type === 'traffic'">
        <label class="input-label">指标</label>
        <PSelect v-model="form.params.metric" block :disabled="!!form.id" :options="(meta?.metrics ?? []).map((m) => ({ value: m.key, label: m.name }))" @change="resetRule" />
        <p class="input-hint">{{ metricOf(form.params.metric)?.desc }}</p>
      </div>
      <div v-if="!spec?.active">
        <label class="input-label">平台</label>
        <div class="flex flex-wrap gap-1.5">
          <button v-for="p in meta?.platforms ?? []" :key="p" type="button" class="rounded-lg border px-3 py-1 text-sm" :class="form.platforms.includes(p) ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20' : 'border-gray-200 text-gray-500 dark:border-dark-600'" @click="toggle(form.platforms, p)">{{ platformName(p) }}</button>
        </div>
      </div>
      <template v-if="form.type === 'graded'">
        <div><label class="input-label">题面</label><textarea v-model="form.params.prompt" rows="6" class="input font-mono text-xs" /></div>
        <div class="grid gap-4 sm:grid-cols-3">
          <div><label class="input-label">标准答案</label><input v-model="answersText" class="input" placeholder="多个用 | 隔开" /></div>
          <div><label class="input-label">判分方式</label><PSelect v-model="form.params.match" block :options="matcherOptions" /></div>
          <div><label class="input-label">最大输出 token</label><input v-model.number="form.params.max_tokens" type="number" class="input" /></div>
        </div>
        <div class="flex items-center gap-2">
          <input v-model="tryReply" class="input flex-1" placeholder="粘贴一段回答试判" />
          <button type="button" class="btn btn-secondary" @click="tryGrade">试判</button>
          <span v-if="tryResult" class="whitespace-nowrap text-sm" :class="tryResult.pass ? 'text-green-600' : 'text-red-600'">{{ tryResult.pass ? '判对' : '判错' }} · {{ tryResult.got || '未提取到' }}</span>
        </div>
      </template>
      <template v-if="form.type === 'archive'">
        <div><label class="input-label">题面</label><textarea v-model="form.params.prompt" rows="4" class="input font-mono text-xs" /></div>
        <details>
          <summary class="cursor-pointer text-sm text-gray-500">按平台单独设置题面</summary>
          <div v-for="p in meta?.platforms ?? []" :key="p" class="mt-3"><label class="input-label">{{ platformName(p) }}</label><textarea v-model="form.params.prompts[p]" rows="3" class="input font-mono text-xs" /></div>
        </details>
        <div class="w-48"><label class="input-label">最大输出 token</label><input v-model.number="form.params.max_tokens" type="number" class="input" /></div>
      </template>

      <div v-if="spec?.judges && !form.display_only">
        <label class="input-label">判定规则</label>
        <div class="space-y-3 rounded-xl border border-gray-200 p-4 text-sm text-gray-600 dark:border-dark-600 dark:text-dark-200">
          <template v-if="form.type === 'traffic'">
            <div class="flex flex-wrap items-center gap-1.5">参照
              <button v-for="(n, k) in meta?.references ?? {}" :key="k" type="button" class="rounded-lg border px-2.5 py-0.5 text-xs" :class="form.rule.bases.includes(k) ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20' : 'border-gray-200 text-gray-500 dark:border-dark-600'" @click="toggle(form.rule.bases, k)">
                <template v-if="form.rule.bases.includes(k)">{{ form.rule.bases.indexOf(k) + 1 }}. </template>{{ n }}
              </button>
              <input v-if="form.rule.bases.includes('fixed')" v-model.number="form.rule.fixed" type="number" step="any" class="input w-24 py-0.5" placeholder="固定值" />
            </div>
            <div class="flex flex-wrap items-center gap-2">
              比较 <PSelect v-model="form.rule.mode" size="sm" width-class="w-28" :options="MODE_OPTIONS" />
              可疑 <N v-model="form.rule.warn" step="any" /> 异常 <N v-model="form.rule.bad" step="any" />
              <template v-if="metricOf(form.params.metric)?.better === 'low'">· 低于 <N v-model="form.rule.floor" step="any" /> 算正常</template>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              持续 <N v-model="form.rule.duration_minutes" /> 分钟 · 每小时至少 <N v-model="form.rule.min_samples" /> 次请求
              <template v-if="metricOf(form.params.metric)?.counted">· 至少出错 <N v-model="form.rule.min_events" /> 次</template>
              · 参照至少 <N v-model="form.rule.min_peers" /> 家
            </div>
          </template>
          <div v-if="form.type === 'fingerprint'" class="flex flex-wrap items-center gap-2">
            连续 <N v-model="form.rule.bad_consecutive" /> 次识别为其他模型记异常 · 置信度低于 <N v-model="form.rule.min_confidence" /> % 不计入
          </div>
          <template v-if="form.type === 'graded'">
            <div class="flex flex-wrap items-center gap-2">看最近 <N v-model="form.rule.window" /> 次 · <PSelect v-model="form.rule.how" size="sm" width-class="w-36" :options="HOW_OPTIONS" /></div>
            <div v-if="form.rule.how === 'peer'" class="flex flex-wrap items-center gap-2">比同组平均少 <N v-model="form.rule.peer_warn" /> 次可疑，少 <N v-model="form.rule.peer_bad" /> 次异常</div>
            <div v-else class="flex flex-wrap items-center gap-2">答对少于 <N v-model="form.rule.warn_below" /> 次可疑，少于 <N v-model="form.rule.bad_below" /> 次异常</div>
          </template>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-6">
        <label class="flex items-center gap-2 text-sm"><Toggle v-model="form.enabled" /> 启用</label>
        <label v-if="spec?.judges" class="flex items-center gap-2 text-sm"><Toggle v-model="form.display_only" /> 仅展示，不参与判定</label>
      </div>
      <div class="flex justify-end gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
        <button type="button" class="btn btn-secondary" @click="emit('cancel')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="saving || !form.name" data-testid="dispatch-item-save" @click="save">{{ form.id ? '保存' : '创建' }}</button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, reactive, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Item } from '../api'
import Toggle from '@/components/common/Toggle.vue'
import { platformName } from '../shared/format'
import PSelect from '../shared/PSelect.vue'
import { meta, metricOf, specOf } from '../shared/meta'

const N = defineComponent({
  props: { modelValue: { type: Number, default: undefined }, step: { type: String, default: '1' } },
  emits: ['update:modelValue'],
  setup(p, { emit }) {
    return () => h('input', { type: 'number', step: p.step, class: 'input w-16 px-2 py-0.5 text-center text-sm', value: p.modelValue,
      onChange: (e: Event) => { const v = (e.target as HTMLInputElement).value; emit('update:modelValue', v === '' ? undefined : Number(v)) } })
  }
})

const MODE_OPTIONS = [{ value: 'ratio', label: '倍数' }, { value: 'pct', label: '百分比' }, { value: 'diff', label: '百分点差' }]
const HOW_OPTIONS = [{ value: 'fixed', label: '按答对次数' }, { value: 'peer', label: '和同组比' }]
const matcherOptions = computed(() => (meta.value?.matchers ?? []).map((m) => ({ value: m.key, label: m.name })))
const props = defineProps<{ item?: Item | null; dimensionId?: number }>()
const emit = defineEmits<{ cancel: []; saved: [] }>()
const app = useAppStore()
const saving = ref(false)
const tryReply = ref('')
const tryResult = ref<{ got: string; pass: boolean } | null>(null)

const clone = (v: any) => JSON.parse(JSON.stringify(v ?? {}))
const form = reactive<any>(props.item
  ? { ...clone(props.item), params: clone(props.item.params), rule: clone(props.item.rule), platforms: [...(props.item.platforms ?? [])] }
  : { id: 0, name: '', type: '', dimension_id: props.dimensionId ?? meta.value?.dimensions[0]?.id, display_only: false, params: {}, rule: {}, platforms: [], enabled: true })
if (form.type === 'archive') form.params.prompts ??= {}
const spec = computed(() => specOf(form.type))

const answersText = computed({
  get: () => (form.params.answers ?? []).join(' | '),
  set: (v: string) => (form.params.answers = v.split('|').map((x) => x.trim()).filter(Boolean))
})

function toggle(arr: string[], v: string) {
  const i = arr.indexOf(v)
  if (i >= 0) arr.splice(i, 1)
  else arr.push(v)
}

function pickType(t: string) {
  form.type = t
  form.platforms = specOf(t)?.active ? [] : [...(meta.value?.platforms ?? [])]
  form.params = t === 'traffic' ? { metric: meta.value?.metrics[0]?.key } : t === 'graded' ? { prompt: '', answers: [], match: 'num', max_tokens: 8000 }
    : t === 'archive' ? { prompt: '', prompts: {}, max_tokens: 32000 } : t === 'field' ? { field: 'rate_multiplier' } : {}
  form.display_only = t === 'archive' || t === 'field'
  resetRule()
}

function resetRule() {
  if (form.type === 'traffic') {
    const m = metricOf(form.params.metric)
    form.rule = { bases: ['peer', 'site', 'self'], min_peers: 2, min_samples: 50, ...clone(m?.rule) }
  } else if (form.type === 'fingerprint') form.rule = { bad_consecutive: 3, min_confidence: 60 }
  else if (form.type === 'graded') form.rule = { window: 10, how: 'fixed', warn_below: 6, bad_below: 3, peer_warn: 3, peer_bad: 5 }
  else form.rule = {}
}
if (!props.item) resetRule()

async function tryGrade() {
  try {
    tryResult.value = await dispatchApi.tryGrade(form.params, tryReply.value)
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '试判失败'))
  }
}

async function save() {
  saving.value = true
  try {
    await dispatchApi.saveItem(form)
    app.showSuccess(form.id ? '已保存' : `已创建「${form.name}」`)
    emit('saved')
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}
</script>
