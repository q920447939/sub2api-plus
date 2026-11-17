<!-- [plus] 填原因再确认的对话框（批量不处理），替代 window.prompt；请求失败时保留已填内容 -->
<template>
  <BaseDialog :show="open" :title="title" width="narrow" @close="close">
    <TextArea
      ref="fieldRef"
      v-model="value"
      :rows="3"
      :label="label"
      required
      :placeholder="placeholder"
      :hint="`${value.length}/${max}`"
      :error="tooLong ? `最多 ${max} 字` : err"
      data-testid="prompt-input"
      @keydown.ctrl.enter="confirm"
      @keydown.meta.enter="confirm"
    />
    <template #footer>
      <div class="flex w-full justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="busy" @click="close">取消</button>
        <button
          type="button"
          class="btn"
          :class="danger ? 'btn-danger' : 'btn-primary'"
          :disabled="!value.trim() || tooLong || busy"
          data-testid="prompt-confirm"
          @click="confirm"
        >
          {{ busy ? '处理中…' : confirmText }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TextArea from '@/components/common/TextArea.vue'

const props = withDefaults(
  defineProps<{ open: boolean; title: string; label?: string; placeholder?: string; confirmText?: string; danger?: boolean; max?: number }>(),
  {
    label: '原因',
    placeholder: '',
    confirmText: '确定',
    danger: false,
    max: 500
  }
)
const emit = defineEmits<{ (e: 'close'): void; (e: 'confirm', v: string): void }>()

const value = ref('')
const busy = ref(false)
const err = ref('')
const fieldRef = ref<InstanceType<typeof TextArea> | null>(null)
const tooLong = computed(() => value.value.length > props.max)

watch(
  () => props.open,
  (o) => {
    if (!o) return
    value.value = ''
    err.value = ''
    busy.value = false
    // BaseDialog 打开后会先聚焦第一个可聚焦元素（右上角关闭按钮），等它之后再把焦点给输入框
    void nextTick(() => nextTick(() => fieldRef.value?.focus()))
  }
)
watch(value, () => (err.value = ''))

function close() {
  if (!busy.value) emit('close')
}

function confirm() {
  const v = value.value.trim()
  if (!v || tooLong.value || busy.value) return
  busy.value = true
  err.value = ''
  emit('confirm', v)
}

// 父组件在请求结束后调用：成功 done()（并关闭），失败 fail(信息) 保留已填原因
defineExpose({
  done: () => (busy.value = false),
  fail: (m: string) => {
    busy.value = false
    err.value = m
  }
})
</script>
