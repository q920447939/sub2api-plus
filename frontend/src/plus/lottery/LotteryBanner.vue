<!--
  [plus] 兑换页刮刮乐横幅。活动关闭或接口失败时不渲染，对兑换流程零影响。
-->
<template>
  <div
    v-if="overview?.enabled"
    class="card flex flex-wrap items-center gap-4 border-l-4 border-orange-500 bg-gradient-to-r from-orange-50 to-transparent p-4 dark:from-orange-900/10 dark:to-transparent"
  >
    <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-orange-100 dark:bg-orange-900/30">
      <Icon name="gift" size="md" class="text-orange-600 dark:text-orange-400" />
    </div>
    <div class="min-w-0 flex-1">
      <p class="font-semibold text-gray-900 dark:text-white">
        兑换刮刮乐 · 每次必中，最高 ${{ overview.promo_max_amount.toFixed(2) }}
      </p>
      <p class="text-sm text-gray-600 dark:text-gray-400">
        <template v-if="tierText">{{ tierText }}</template>
        <template v-if="overview.remaining > 0">
          <span v-if="tierText"> · </span>您还有
          <span class="font-semibold text-orange-600 dark:text-orange-400">{{ overview.remaining }}</span>
          张待刮
        </template>
      </p>
    </div>
    <button class="btn btn-primary shrink-0" @click="router.push('/lottery')">
      {{ overview.remaining > 0 ? '去刮奖' : '查看活动' }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { getOverview, type LotteryOverview } from '@/api/lottery'

const router = useRouter()
const overview = ref<LotteryOverview | null>(null)

// 例：单张卡密满 $10 得 1 张，满 $50 得 3 张，满 $100 得 6 张
const tierText = computed(() => {
  const tiers = [...(overview.value?.chance_tiers ?? [])].sort((a, b) => a.threshold - b.threshold)
  if (!tiers.length) return ''
  return '单张卡密满 ' + tiers.map((t) => `$${t.threshold} 得 ${t.chances} 张`).join('，满 ')
})

onMounted(async () => {
  try {
    overview.value = await getOverview()
  } catch {
    // 静默：活动接口异常不影响兑换
  }
})
</script>
