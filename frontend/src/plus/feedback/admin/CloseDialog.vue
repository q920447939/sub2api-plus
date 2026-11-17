<!-- [plus] 结案：结论随类型变化；只有已采纳的建议能发积分 -->
<template>
  <BaseDialog :show="open" title="结案" width="narrow" @close="emit('close')">
    <div class="space-y-4" data-testid="close-dialog">
      <div class="grid grid-cols-2 gap-2">
        <button
          v-for="v in verdicts"
          :key="v"
          type="button"
          class="rounded-xl border py-2.5 text-sm font-medium transition-colors"
          :class="verdict === v ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'border-gray-200 text-gray-700 hover:border-gray-300 dark:border-dark-600 dark:text-dark-200'"
          @click="verdict = v"
        >
          {{ STATUS_META[v].label }}
        </button>
      </div>

      <TextArea v-if="verdict === 'declined'" ref="reasonRef" v-model="reason" :rows="2" label="原因" required placeholder="会展示给用户" :error="reasonError" />

      <div v-if="rewardable" class="rounded-xl border border-gray-200 px-4 py-3 dark:border-dark-600">
        <div class="flex items-center gap-3">
          <span class="flex-1 text-sm font-medium text-gray-900 dark:text-white">发放积分</span>
          <Toggle v-model="giveReward" />
        </div>
        <template v-if="giveReward">
          <div class="mt-3 flex items-center justify-center gap-3">
            <button type="button" class="btn btn-secondary h-9 w-9 !p-0" :disabled="pointsNum <= inc" aria-label="减少" @click="bump(-inc)">−</button>
            <input
              ref="pointsRef"
              v-model.number="points"
              type="number"
              :min="inc"
              :step="inc"
              class="input w-28 text-center text-lg font-semibold tabular-nums"
              :class="pointsError ? 'input-error' : ''"
              :aria-invalid="!!pointsError"
              aria-label="积分"
              @blur="normalizePoints"
            />
            <button type="button" class="btn btn-secondary h-9 w-9 !p-0" aria-label="增加" @click="bump(inc)">+</button>
          </div>
          <p v-if="pointsError" class="input-error-text mt-1.5 text-center" role="alert">{{ pointsError }}</p>
        </template>
      </div>

      <TextArea v-model="message" :rows="2" label="给用户的话" placeholder="选填" />
    </div>
    <template #footer>
      <div class="flex w-full justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="busy" data-testid="close-confirm" @click="confirm">
          {{ busy ? '处理中…' : giveReward && rewardable && pointsNum > 0 ? `结案并发放 ${pointsNum.toLocaleString()} 积分` : '结案' }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TextArea from '@/components/common/TextArea.vue'
import Toggle from '@/components/common/Toggle.vue'
import type { CloseRequest, Kind, Status } from '../api'
import { STATUS_META, VERDICTS } from '../format'

const props = defineProps<{
  open: boolean
  kind: Kind
  rewardEnabled: boolean
  rewardDefault: number
  step: number
  /** 回复框里还没发出的内容，打开时预填到「给用户的话」 */
  draft?: string
}>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'confirm', req: CloseRequest): void }>()

const verdicts = computed(() => VERDICTS[props.kind])
const verdict = ref<Status>('resolved')
const reason = ref('')
const reasonError = ref('')
const reasonRef = ref<InstanceType<typeof TextArea>>()
const giveReward = ref(true)
// v-model.number 在输入框清空时给的是 ''
const points = ref<number | string>(500)
const pointsError = ref('')
const pointsRef = ref<HTMLInputElement>()
const message = ref('')
const busy = ref(false)

const inc = computed(() => Math.max(1, Math.floor(props.step) || 1))
const pointsNum = computed(() => Number(points.value) || 0)
const rewardable = computed(() => props.rewardEnabled && props.kind === 'suggestion' && verdict.value === 'adopted')

watch(reason, () => (reasonError.value = ''))
watch(points, () => (pointsError.value = ''))
watch(verdict, (v) => {
  reasonError.value = ''
  if (v === 'declined') void nextTick(() => reasonRef.value?.focus())
})

watch(
  () => props.open,
  (o) => {
    if (!o) return
    verdict.value = verdicts.value[0]
    reason.value = ''
    giveReward.value = true
    points.value = props.rewardDefault
    message.value = props.draft ?? ''
    busy.value = false
    reasonError.value = ''
    pointsError.value = ''
  }
)

function bump(delta: number) {
  points.value = Math.max(1, Math.floor(pointsNum.value) + delta)
}

// 只把能识别的数取整；清空或非正数保持原样，交给提交时报错，避免悄悄变成 1 积分
function normalizePoints() {
  const n = Math.floor(pointsNum.value)
  if (n >= 1 && n !== points.value) points.value = n
}

function confirm() {
  if (busy.value) return
  if (verdict.value === 'declined' && !reason.value.trim()) {
    reasonError.value = '请填写原因'
    reasonRef.value?.focus()
    return
  }
  const reward = rewardable.value && giveReward.value
  const n = Number(points.value)
  if (reward && !(points.value !== '' && Number.isInteger(n) && n > 0)) {
    pointsError.value = points.value === '' ? '请输入积分' : '积分需为正整数'
    pointsRef.value?.focus()
    return
  }
  busy.value = true
  emit('confirm', {
    verdict: verdict.value,
    reason: verdict.value === 'declined' ? reason.value.trim() : undefined,
    reward_points: reward ? n : 0,
    message: message.value.trim() || undefined
  })
}

defineExpose({ done: () => (busy.value = false) })
</script>
