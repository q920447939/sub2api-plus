<template>
  <AuthLayout>
    <div class="space-y-6 text-center" data-testid="verify-email-page">
      <template v-if="state === 'loading'">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">正在验证邮箱…</h2>
      </template>

      <template v-else-if="state === 'success'">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">邮箱验证成功</h2>
        <p v-if="result?.reward_granted" class="text-sm text-gray-600 dark:text-dark-300">
          已发放 <span class="font-semibold text-primary-600">{{ result.reward_points }}</span> 积分，登录后可在积分乐园查看。
        </p>
        <p v-else-if="result?.status === 'exempt'" class="text-sm text-gray-600 dark:text-dark-300">管理员账号无需验证邮箱。</p>
        <p v-else class="text-sm text-gray-600 dark:text-dark-300">你的邮箱现在可以接收余额和订阅提醒。</p>
      </template>

      <template v-else>
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">链接无效或已过期</h2>
        <p class="text-sm text-gray-600 dark:text-dark-300">{{ errorMsg }}</p>
        <p class="text-sm text-gray-500 dark:text-dark-400">登录后在「个人资料」里用验证码也可以完成验证。</p>
      </template>

      <router-link
        v-if="state !== 'loading'"
        to="/login"
        class="inline-flex items-center gap-2 font-medium text-primary-600 hover:text-primary-500"
      >前往登录</router-link>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { consumeInviteToken, type VerifyResult } from './api'

const route = useRoute()
const state = ref<'loading' | 'success' | 'error'>('loading')
const result = ref<VerifyResult | null>(null)
const errorMsg = ref('请重新联系管理员，或使用验证码方式验证。')

onMounted(async () => {
  const token = String(route.query.token ?? '')
  if (!token) {
    state.value = 'error'
    return
  }
  try {
    result.value = await consumeInviteToken(token)
    state.value = 'success'
  } catch (e: unknown) {
    errorMsg.value = extractApiErrorMessage(e, errorMsg.value)
    state.value = 'error'
  }
})
</script>
