<!-- [plus] 单行/多行输入对话框（批量加标签、批量不处理），替代 window.prompt -->
<template>
  <BaseDialog :show="open" :title="title" width="narrow" @close="emit('close')">
    <TextArea v-if="multiline" v-model="value" :rows="3" :placeholder="placeholder" />
    <Input v-else v-model="value" :placeholder="placeholder" @enter="confirm" />
    <template #footer>
      <div class="flex w-full justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="!value.trim()" @click="confirm">{{ confirmText }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Input from '@/components/common/Input.vue'
import TextArea from '@/components/common/TextArea.vue'

const props = withDefaults(defineProps<{ open: boolean; title: string; placeholder?: string; multiline?: boolean; confirmText?: string }>(), {
  placeholder: '',
  multiline: false,
  confirmText: '确定'
})
const emit = defineEmits<{ (e: 'close'): void; (e: 'confirm', v: string): void }>()

const value = ref('')
watch(
  () => props.open,
  (o) => o && (value.value = '')
)

function confirm() {
  const v = value.value.trim()
  if (!v) return
  emit('confirm', v)
  emit('close')
}
</script>
