<!-- [plus] 重新诊断：马上汇总流量并重新判定一轮（不发起新的检测请求），结果记进操作日志 -->
<template>
  <button type="button" class="btn btn-secondary" :disabled="busy" @click="run">
    <Icon name="refresh" size="md" class="mr-1.5" :class="busy ? 'animate-spin' : ''" />
    重新诊断
  </button>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi } from '../api'

const emit = defineEmits<{ done: [] }>()
const app = useAppStore()
const busy = ref(false)

async function run() {
  busy.value = true
  try {
    const r = await dispatchApi.evaluate()
    app.showSuccess(r.report.changed ? `诊断完成，${r.report.changed} 项结论有变化` : '诊断完成，结论没有变化')
    emit('done')
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '诊断失败'))
  } finally {
    busy.value = false
  }
}
</script>
