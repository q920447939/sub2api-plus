<!--
  [plus] 兑换成功后的刮刮乐提示。宿主页每次兑换成功后把 trigger 自增即可，
  组件查询"本人最近一次兑换"带来的刮刮卡数，得卡时才显示；活动关闭或接口失败时不渲染。
-->
<template>
  <div
    v-if="grant && grant.granted > 0"
    class="prompt-in rounded-xl bg-gradient-to-r from-orange-500 to-rose-500 p-5 text-white shadow-lg"
    role="status"
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
        <p class="text-lg font-bold">兑换成功，额外获得 {{ grant.granted }} 张刮刮卡</p>
        <p class="text-sm text-white/90">
          当前共 <span class="font-semibold">{{ grant.remaining }}</span> 张待刮 · 每张必中 · 奖金直接进余额
        </p>
      </div>
      <button
        class="shrink-0 rounded-lg bg-white px-5 py-2.5 text-sm font-bold text-orange-600 shadow-sm transition hover:bg-orange-50 motion-safe:hover:scale-105"
        @click="router.push('/lottery')"
      >
        立即刮奖
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { getRedeemGrant, type RedeemGrant } from '@/api/lottery'

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
      grant.value = await getRedeemGrant()
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
@media (prefers-reduced-motion: reduce) {
  .prompt-in {
    animation: none;
  }
}
</style>
