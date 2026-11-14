<!-- [plus] 权重设置没有生效时的提示条（设置都留着，线上按原来的规则分流量） -->
<template>
  <div class="flex items-center gap-3 rounded-xl bg-amber-50 px-4 py-2.5 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
    <span>权重设置没有生效{{ rules ? `，${rules} 条设置都留着` : '' }}，线上按原来的规则分流量</span>
    <button type="button" class="btn btn-secondary btn-sm ml-auto" :disabled="busy" @click="turnOn">打开</button>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi } from '../api'

defineProps<{ rules: number }>()
const emit = defineEmits<{ changed: [] }>()
const appStore = useAppStore()
const busy = ref(false)

async function turnOn() {
  busy.value = true
  try {
    const v = await dispatchApi.config()
    await dispatchApi.saveConfig({ ...v.config, apply: true }, v.version)
    appStore.showSuccess('权重设置已生效')
    emit('changed')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  } finally {
    busy.value = false
  }
}
</script>
