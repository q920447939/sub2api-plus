<template>
  <button
    v-if="status?.enabled"
    type="button"
    class="relative flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
    :aria-label="status.today_checked ? '今日已签到，查看签到' : '今日未签到，去签到'"
    :title="status.today_checked ? '今日已签到' : `签到领 +${status.today_points} 积分`"
    data-testid="checkin-header-button"
    @click="onClick"
  >
    <Icon name="calendar" size="sm" />
    <span class="hidden sm:inline">{{ status.today_checked ? '已签到' : '签到' }}</span>
    <span
      v-if="!status.today_checked"
      class="absolute right-1 top-1 size-2 rounded-full bg-red-500 ring-2 ring-white dark:ring-dark-900"
      data-testid="checkin-header-dot"
    />
  </button>
</template>

<script setup lang="ts">
// [plus] 顶栏签到入口：未签到时带红点，点击打开签到弹窗；已签到则跳签到页
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { checkinStatus, popupOpen, refreshCheckinStatus } from './popupState'

const router = useRouter()
const status = checkinStatus

async function onClick() {
  // 跨天后本地状态可能过期，点击时先刷新
  const st = (await refreshCheckinStatus()) ?? status.value
  if (!st) return
  if (st.today_checked) void router.push('/points-park')
  else popupOpen.value = true
}

onMounted(() => {
  if (!status.value) void refreshCheckinStatus()
})
</script>
