<!-- [plus] 免登录退订页：必须点按钮才退订，避免邮件安全网关预取链接导致误退订 -->
<template>
  <AuthLayout>
    <div class="space-y-6 text-center">
      <h2 class="text-2xl font-bold text-gray-900 dark:text-white">退订邮件</h2>
      <template v-if="!token">
        <p class="text-sm text-gray-500">链接无效。</p>
      </template>
      <template v-else-if="serviceOnly">
        <p class="text-sm text-gray-600 dark:text-dark-300">这是与账号相关的服务邮件，不需要也无法退订。</p>
      </template>
      <template v-else-if="done">
        <p class="text-sm text-gray-600 dark:text-dark-300">已退订「{{ done }}」类邮件，之后不会再收到。</p>
        <p class="text-xs text-gray-400">验证码、密码重置等与账号安全相关的邮件不受影响。</p>
      </template>
      <template v-else>
        <p class="text-sm text-gray-600 dark:text-dark-300">确认不再接收这类邮件吗？</p>
        <button type="button" class="btn btn-primary w-full" :disabled="busy" @click="submit">{{ busy ? '处理中…' : '确认退订' }}</button>
        <p v-if="error" class="text-sm text-red-600">{{ error }}</p>
      </template>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { publicApi } from './api'

const token = String(useRoute().query.t ?? '')
const busy = ref(false)
const done = ref('')
const serviceOnly = ref(false)
const error = ref('')

async function submit() {
  busy.value = true
  error.value = ''
  try {
    const r = await publicApi.unsubscribe(token)
    serviceOnly.value = !r.unsubscribed
    done.value = r.category
  } catch (e) {
    error.value = extractApiErrorMessage(e, '退订失败，链接可能已失效')
  } finally {
    busy.value = false
  }
}
</script>
