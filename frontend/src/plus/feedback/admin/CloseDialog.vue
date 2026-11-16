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

      <TextArea v-if="verdict === 'declined'" v-model="reason" :rows="2" label="原因" required placeholder="会展示给用户" />

      <div v-if="rewardable" class="rounded-xl border border-gray-200 px-4 py-3 dark:border-dark-600">
        <div class="flex items-center gap-3">
          <span class="flex-1 text-sm font-medium text-gray-900 dark:text-white">发放积分</span>
          <Toggle v-model="giveReward" />
        </div>
        <div v-if="giveReward" class="mt-3 flex items-center justify-center gap-3">
          <button type="button" class="btn btn-secondary h-9 w-9 !p-0" :disabled="points <= step" aria-label="减少" @click="points -= step">−</button>
          <input v-model.number="points" type="number" min="1" class="input w-28 text-center text-lg font-semibold tabular-nums" aria-label="积分" />
          <button type="button" class="btn btn-secondary h-9 w-9 !p-0" aria-label="增加" @click="points += step">+</button>
        </div>
      </div>

      <TextArea v-model="message" :rows="2" label="给用户的话" placeholder="选填" />
    </div>
    <template #footer>
      <div class="flex w-full justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="!valid || busy" data-testid="close-confirm" @click="confirm">
          {{ busy ? '处理中…' : giveReward && rewardable ? `结案并发放 ${points} 积分` : '结案' }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TextArea from '@/components/common/TextArea.vue'
import Toggle from '@/components/common/Toggle.vue'
import type { CloseRequest, Kind, Status } from '../api'
import { STATUS_META, VERDICTS } from '../format'

const props = defineProps<{ open: boolean; kind: Kind; rewardEnabled: boolean; rewardDefault: number; step: number }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'confirm', req: CloseRequest): void }>()

const verdicts = computed(() => VERDICTS[props.kind])
const verdict = ref<Status>('resolved')
const reason = ref('')
const giveReward = ref(true)
const points = ref(500)
const message = ref('')
const busy = ref(false)

const rewardable = computed(() => props.rewardEnabled && props.kind === 'suggestion' && verdict.value === 'adopted')
const valid = computed(() => {
  if (verdict.value === 'declined' && !reason.value.trim()) return false
  if (rewardable.value && giveReward.value && !(points.value > 0)) return false
  return true
})

watch(
  () => props.open,
  (o) => {
    if (!o) return
    verdict.value = verdicts.value[0]
    reason.value = ''
    giveReward.value = true
    points.value = props.rewardDefault
    message.value = ''
    busy.value = false
  }
)

function confirm() {
  if (!valid.value) return
  busy.value = true
  emit('confirm', {
    verdict: verdict.value,
    reason: verdict.value === 'declined' ? reason.value.trim() : undefined,
    reward_points: rewardable.value && giveReward.value ? points.value : 0,
    message: message.value.trim() || undefined
  })
}

defineExpose({ done: () => (busy.value = false) })
</script>
