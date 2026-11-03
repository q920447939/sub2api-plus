<!--
  [plus] 兑换成功后的"额外获得积分"提示。宿主页每次兑换余额卡密成功后把 trigger 自增即可，
  组件查询本人最近一次兑换送的积分，送了才显示；功能关闭或接口失败时不渲染。
-->
<template>
  <div
    v-if="grant && grant.points > 0"
    class="prompt-in rounded-xl bg-gradient-to-r from-orange-500 to-rose-500 p-5 text-white shadow-lg"
    role="status"
    data-testid="redeem-bonus-prompt"
  >
    <div class="flex flex-wrap items-center gap-4">
      <div class="relative flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-white/20">
        <span
          class="absolute inset-0 animate-ping rounded-full bg-white/30 motion-reduce:animate-none"
          aria-hidden="true"
        ></span>
        <Icon name="gift" size="lg" class="relative text-white" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="text-lg font-bold">兑换成功，额外获得 {{ grant.points }} 积分</p>
        <p class="text-sm text-white/90">积分可以在积分乐园的幸运九宫格里赢积分和余额</p>
      </div>
      <button
        class="shrink-0 rounded-lg bg-white px-5 py-2.5 text-sm font-bold text-orange-600 shadow-sm transition hover:bg-orange-50 motion-safe:hover:scale-105"
        @click="router.push({ path: '/points-park', query: { tab: 'grid' } })"
      >
        去积分乐园看看
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { getLatestRedeemBonus, type RedeemGrant } from './api'

const props = defineProps<{
  /** 兑换成功计数；变化（且 > 0）时查询一次 */
  trigger: number
}>()

const router = useRouter()
const grant = ref<RedeemGrant | null>(null)

watch(
  () => props.trigger,
  async (n) => {
    grant.value = null
    if (!n) return
    try {
      grant.value = await getLatestRedeemBonus()
    } catch {
      // 静默：不影响兑换
    }
  }
)
</script>

<style scoped>
.prompt-in {
  animation: prompt-in 0.45s cubic-bezier(0.34, 1.56, 0.64, 1);
}
@keyframes prompt-in {
  from {
    opacity: 0;
    transform: translateY(-8px) scale(0.97);
  }
  to {
    opacity: 1;
    transform: none;
  }
}
</style>
