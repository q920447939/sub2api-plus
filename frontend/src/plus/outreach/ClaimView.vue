<!-- [plus] 用户端：点邮件按钮并登录后先在这里领取附带权益，领完继续去活动的落地页 -->
<template>
  <AppLayout>
    <div class="mx-auto max-w-md pt-10">
      <div class="card p-8 text-center">
        <p v-if="loading" class="text-sm text-gray-400">正在读取…</p>
        <template v-else-if="error">
          <p class="text-base font-medium text-gray-900 dark:text-white">{{ error }}</p>
          <button type="button" class="btn btn-secondary mt-6" @click="router.push('/')">回到首页</button>
        </template>
        <template v-else-if="granted !== null">
          <p class="text-sm text-gray-500">已到账</p>
          <p v-for="g in granted" :key="g" class="mt-2 text-3xl font-bold text-primary-600">+{{ g }}</p>
          <button type="button" class="btn btn-primary mt-6" @click="goNext">继续</button>
        </template>
        <template v-else-if="info">
          <template v-if="info.claimed">
            <p class="text-base font-medium text-gray-900 dark:text-white">这份权益已经领取过了</p>
            <button type="button" class="btn btn-secondary mt-6" @click="goNext">继续</button>
          </template>
          <template v-else-if="info.expired">
            <p class="text-base font-medium text-gray-900 dark:text-white">领取已过期</p>
            <button type="button" class="btn btn-secondary mt-6" @click="router.push('/')">回到首页</button>
          </template>
          <template v-else>
            <p class="text-sm text-gray-500">欢迎回来，这是为您准备的</p>
            <p v-for="it in info.items" :key="it" class="mt-2 text-3xl font-bold text-primary-600">{{ it }}</p>
            <button type="button" class="btn btn-primary mt-6 w-full" :disabled="claiming" @click="claim">{{ claiming ? '领取中…' : '立即领取' }}</button>
          </template>
        </template>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { userApi, type ClaimInfo } from './api'

const route = useRoute()
const router = useRouter()
const token = String(route.query.t ?? '')

const loading = ref(true)
const claiming = ref(false)
const error = ref('')
const info = ref<ClaimInfo | null>(null)
const granted = ref<string[] | null>(null)

// 领完去活动的落地页；地址由服务端解析（只会是本站域名），不读 URL 参数
function goNext() {
  const next = info.value?.next
  if (!next) return router.push('/')
  try {
    const u = new URL(next, window.location.origin)
    if (u.origin === window.location.origin) return router.push(u.pathname + u.search + u.hash)
  } catch {
    /* 非法地址回首页 */
  }
  window.location.href = next
}

async function claim() {
  claiming.value = true
  try {
    granted.value = (await userApi.claim(token)).items
  } catch (e) {
    error.value = extractApiErrorMessage(e, '领取失败')
  } finally {
    claiming.value = false
  }
}

onMounted(async () => {
  if (!token) {
    error.value = '领取链接无效'
    loading.value = false
    return
  }
  try {
    info.value = await userApi.claimInfo(token)
  } catch (e) {
    // 链接属于别的账号时同样返回「无效」，不暴露原因
    error.value = extractApiErrorMessage(e, '链接无效，或不属于当前登录的账号')
  } finally {
    loading.value = false
  }
})
</script>
