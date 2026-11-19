<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-medium text-gray-900 dark:text-white">邮箱验证</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            验证邮箱后可及时收到余额和订阅提醒
          </p>
        </div>
        <!-- 状态徽章 -->
        <span
          class="inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-semibold"
          :class="badgeClass"
        >
          {{ badgeText }}
        </span>
      </div>
    </div>
    <div class="px-6 py-5">
      <div class="flex items-center justify-between">
        <p class="text-sm text-gray-600 dark:text-gray-400">
          <span class="font-medium">{{ status?.email }}</span>
        </p>
        <button
          v-if="status?.status !== 'verified' && status?.status !== 'exempt'"
          class="btn btn-primary btn-sm"
          @click="modalOpen = true"
        >
          {{ status?.status === 'invalid' ? '更换邮箱' : '去验证' }}
        </button>
      </div>

      <!-- 有奖励资格提示 -->
      <div
        v-if="status?.reward_eligible"
        class="mt-3 flex items-center gap-2 rounded-lg bg-blue-50 px-4 py-2.5 text-sm dark:bg-blue-900/20"
      >
        <span>🎁</span>
        <span>首次验证可领取 <strong>{{ status.reward_points }} 积分</strong></span>
      </div>
    </div>

    <EmailVerifyModal v-model="modalOpen" @verified="onVerified" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { getEmailVerifyStatus, type EmailVerifyStatusView } from '@/plus/emailverify/api'
import EmailVerifyModal from '@/plus/emailverify/EmailVerifyModal.vue'

const status = ref<EmailVerifyStatusView | null>(null)
const modalOpen = ref(false)

const badgeClass = computed(() => {
  switch (status.value?.status) {
    case 'verified': return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
    case 'invalid':  return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
    case 'exempt':   return 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'
    default:         return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  }
})

const badgeText = computed(() => {
  switch (status.value?.status) {
    case 'verified': return '✓ 已验证'
    case 'invalid':  return '⚠ 邮箱无效'
    case 'exempt':   return '免验证'
    default:         return '待验证'
  }
})

async function load() {
  try {
    status.value = await getEmailVerifyStatus()
  } catch {}
}

async function onVerified() {
  await load()
}

onMounted(load)
</script>
