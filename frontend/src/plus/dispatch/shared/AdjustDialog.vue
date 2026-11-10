<!-- [plus] 就地调整：左边现状和系统建议，右边调整并预计份额；所有调整记进调整历史和操作日志 -->
<template>
  <BaseDialog :show="!!target" :title="target ? `调整 · ${target.accountName}（${target.groupName}）` : ''" width="extra-wide" @close="emit('close')">
    <div v-if="target" class="grid gap-5 lg:grid-cols-2" data-testid="dispatch-adjust-dialog">
      <!-- 左：现状 + 建议 -->
      <div class="space-y-3 text-sm">
        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <div class="mb-1 text-xs text-gray-500">当前状况</div>
          <div class="flex flex-wrap items-center gap-2">
            <LevelPill :level="target.worst ?? 'few'" />
            <span>当前调整：<b>{{ adjustText(target.current?.action, target.current?.factor) }}</b></span>
            <span v-if="target.current" class="text-xs text-gray-400" :title="target.current.reason">{{ agoText(target.current.created_at) }}</span>
          </div>
          <div class="mt-2 text-gray-600 dark:text-dark-200">{{ target.oneLine }}</div>
        </div>
        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <div class="mb-1 flex items-center gap-2 text-xs text-gray-500">
            系统建议
            <span class="flex-1" />
            <span v-if="target.suggest?.settled" class="text-xs text-gray-400">当前调整已经到位</span>
            <button v-else-if="target.suggest && target.suggest.action !== 'normal'" type="button" class="btn btn-secondary btn-sm" data-testid="dispatch-adopt" @click="adopt">采纳建议</button>
          </div>
          <template v-if="target.suggest && target.suggest.action !== 'normal'">
            <div class="font-medium">
              {{ target.suggest.action === 'pause' ? '建议暂停分配' : `建议份额 ×${target.suggest.factor}` }}
              <span class="text-xs font-normal text-gray-500">（{{ target.suggest.before }}% → {{ target.suggest.after }}%）</span>
            </div>
            <ul class="mt-1 list-disc space-y-1 pl-5 text-xs text-gray-600 dark:text-dark-200">
              <li v-for="(r, i) in target.suggest.reasons" :key="i">
                <b>{{ r.dimension }}</b>{{ LEVEL_TEXT[r.level] }}：{{ r.evidence }} → {{ r.action }}
              </li>
            </ul>
          </template>
          <div v-else class="text-gray-500">无需调整</div>
        </div>
      </div>

      <!-- 右：调整 -->
      <div class="space-y-3 text-sm">
        <div class="space-y-2">
          <label v-for="o in ACTIONS" :key="o.key" class="flex cursor-pointer items-start gap-2 rounded-lg border p-2.5" :class="form.action === o.key ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20' : 'border-gray-200 dark:border-dark-700'">
            <input v-model="form.action" type="radio" :value="o.key" class="sr-only" />
            <div class="flex-1">
              <div class="font-medium">{{ o.name }}</div>
              <div class="text-xs text-gray-500">{{ o.desc }}</div>
              <div v-if="o.key === 'factor' && form.action === 'factor'" class="mt-2 flex items-center gap-3">
                <input v-model.number="form.factor" type="range" min="0.05" max="0.95" step="0.05" class="flex-1" />
                <input v-model.number="form.factor" type="number" min="0.05" max="0.95" step="0.05" class="input w-20 py-1" />
              </div>
            </div>
          </label>
        </div>
        <div>
          <label class="input-label">原因{{ form.action === 'normal' ? '（可选）' : '' }}</label>
          <textarea v-model="form.reason" class="input w-full" rows="2" />
        </div>
        <div v-if="pv" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <div class="mb-2 text-xs text-gray-500">预计份额</div>
          <table class="w-full text-xs">
            <thead><tr class="text-gray-500"><th class="text-left font-normal">账号</th><th class="text-left font-normal">当前</th><th class="text-left font-normal">预计</th></tr></thead>
            <tbody>
              <tr v-for="id in ids" :key="id" :class="Number(id) === target.accountId ? 'font-semibold' : ''">
                <td class="py-0.5">{{ names[id] ?? '#' + id }}<span v-if="pv.current[id]" class="ml-1 text-gray-400">（{{ pv.current[id] }}）</span></td>
                <td><ShareBar :value="pv.before[id] ?? 0" /></td>
                <td><ShareBar :value="pv.predicted[id] ?? 0" tone="sug" /></td>
              </tr>
            </tbody>
          </table>
          <div v-if="pv.affected?.length" class="mt-2 rounded bg-blue-50 px-2 py-1 text-xs text-blue-700">只影响「{{ target.groupName }}」，不影响 {{ pv.affected.join('、') }}</div>
        </div>
        <div v-if="pv?.warning" class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800">{{ pv.warning }}</div>
        <div v-if="previewError" class="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700">{{ previewError }}</div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="saving || !!previewError" data-testid="dispatch-adjust-submit" @click="submit">
          {{ saving ? '提交中…' : '确认调整' }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type AdjustPreview, type Adjustment, type Level, type SuggestRow } from '../api'
import { adjustText, agoText, LEVEL_TEXT } from './format'
import LevelPill from './LevelPill.vue'
import ShareBar from './ShareBar.vue'

export interface AdjustTarget {
  groupId: number
  groupName: string
  accountId: number
  accountName: string
  worst?: Level
  oneLine?: string
  current?: Adjustment | null
  suggest?: SuggestRow | null
  names?: Record<string, string>
}

const props = defineProps<{ target: AdjustTarget | null }>()
const emit = defineEmits<{ close: []; done: [logId: number] }>()
const app = useAppStore()

const ACTIONS = [
  { key: 'normal', name: '正常', desc: '按原调度逻辑分配' },
  { key: 'factor', name: '降低份额', desc: '新会话按系数减少，已有会话不受影响' },
  { key: 'pause', name: '暂停分配', desc: '本分组不再分配给它' }
] as const

const form = reactive({ action: 'normal' as 'normal' | 'pause' | 'factor', factor: 0.5, reason: '', source: 'manual' as 'manual' | 'suggestion' })
const pv = ref<AdjustPreview | null>(null)
const previewError = ref('')
const saving = ref(false)
const names = computed<Record<string, string>>(() => props.target?.names ?? {})
const ids = computed(() => Object.keys(pv.value?.before ?? {}).sort((a, b) => (pv.value!.before[b] ?? 0) - (pv.value!.before[a] ?? 0)))

watch(
  () => props.target,
  (t) => {
    if (!t) return
    form.action = (t.current?.action ?? 'normal') as typeof form.action
    form.factor = t.current?.action === 'factor' ? t.current.factor : 0.5
    form.reason = ''
    form.source = 'manual'
    void refresh()
  },
  { immediate: true }
)

let timer: ReturnType<typeof setTimeout> | undefined
watch(() => [form.action, form.factor], () => {
  clearTimeout(timer)
  timer = setTimeout(refresh, 250)
})

async function refresh() {
  if (!props.target) return
  previewError.value = ''
  try {
    pv.value = await dispatchApi.previewAdjust({ group_id: props.target.groupId, account_id: props.target.accountId, action: form.action, factor: form.factor })
  } catch (e) {
    previewError.value = extractApiErrorMessage(e, '预测失败')
  }
}

function adopt() {
  const s = props.target?.suggest
  if (!s) return
  form.action = s.action
  if (s.action === 'factor') form.factor = s.factor
  form.reason = s.reasons.map((r) => `${r.dimension}${LEVEL_TEXT[r.level]}：${r.evidence}`).join('；')
  form.source = 'suggestion'
}

async function submit() {
  if (!props.target) return
  saving.value = true
  try {
    const log = await dispatchApi.applyAdjust({ group_id: props.target.groupId, account_id: props.target.accountId, action: form.action,
      factor: form.factor, reason: form.reason, source: form.source })
    app.showSuccess('已调整')
    emit('done', log.id)
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '调整失败'))
  } finally {
    saving.value = false
  }
}
</script>
