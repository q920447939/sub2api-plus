<template>
  <Teleport to="body">
    <Transition name="nb-fade">
      <div
        v-if="result"
        class="nb-root fixed inset-0 z-[100] m-0 flex min-h-0 items-center justify-center bg-black/80 p-4 backdrop-blur-sm"
        style="background-image: none"
        role="dialog"
        aria-modal="true"
        data-testid="slot-prize-modal"
        @click.self="$emit('close')"
      >
        <div class="pointer-events-none absolute inset-0 overflow-hidden">
          <span
            v-for="c in confetti"
            :key="c.id"
            class="nb-confetti"
            :style="{ left: c.left + '%', top: c.top + '%', backgroundColor: c.color, animationDelay: c.delay + 's' }"
          />
        </div>
        <Transition name="nb-pop" appear>
          <div class="nb-box-xl relative flex w-full max-w-md flex-col items-center p-6 text-center">
            <div
              class="flex size-20 -mt-16 rotate-3 items-center justify-center border-4 nb-line text-4xl"
              :class="win ? 'bg-[#ffe600]' : 'nb-surface-2'"
              style="box-shadow: 5px 5px 0 var(--nb-shadow)"
            >{{ win ? '🎉' : '🍀' }}</div>
            <div class="mt-3">
              <span class="nb-head bg-black px-2 py-0.5 text-xs uppercase tracking-widest text-[#ffe600]">
                {{ win ? (big ? 'JACKPOT WINNER!' : 'YOU WIN!') : 'SO CLOSE!' }}
              </span>
              <h3 class="nb-head mt-2 text-2xl md:text-3xl">{{ title }}</h3>
              <p class="mt-1 text-xs font-bold nb-muted">{{ subtitle }}</p>
            </div>

            <div class="nb-box nb-cream my-5 flex w-full items-center gap-4 p-4 text-left">
              <div class="nb-shadow-sm shrink-0 border-[3px] nb-line nb-surface p-2 text-4xl">{{ result.emoji || '🎁' }}</div>
              <div class="min-w-0 flex-1">
                <p class="nb-head truncate text-lg" data-testid="slot-prize-label">{{ result.label }}</p>
                <p class="mt-0.5 text-xs font-bold nb-muted">{{ detail }}</p>
                <div
                  v-if="result.redeem_code"
                  class="nb-shadow-sm mt-2 inline-flex items-center gap-2 border-2 nb-line nb-surface px-2 py-0.5 font-mono text-[11px] font-bold"
                >
                  凭证 <span class="text-[#ff5e00]">{{ result.redeem_code }}</span>
                </div>
              </div>
            </div>

            <p v-if="result.boosted" class="nb-tag mb-4 bg-[#ffe600]">⚡ 连签加成命中</p>

            <div class="flex w-full flex-col gap-3">
              <button type="button" class="nb-btn w-full bg-[#ffe600] py-3 text-sm uppercase hover:bg-yellow-300" data-testid="slot-prize-close" @click="$emit('close')">
                {{ win ? '开心收下 🥳' : '知道了' }}
              </button>
              <button
                type="button"
                class="nb-btn w-full nb-surface py-2.5 text-xs uppercase nb-hover"
                :disabled="!canAgain"
                data-testid="slot-prize-again"
                @click="$emit('again')"
              >
                {{ againLabel }}
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { SpinResult } from './api'

const props = defineProps<{ result: SpinResult | null; cost: number; canAgain: boolean; cooldown: number; againHint: string }>()

// 冷却只是防连刷的节奏控制，按钮上倒数即可，不当作错误提示
const againLabel = computed(() => {
  if (props.canAgain) return `再来一次（消耗 ${props.cost} 积分）`
  if (props.cooldown > 0) return `再来一次（${props.cooldown} 秒后可摇）`
  return props.againHint
})
defineEmits<{ close: []; again: [] }>()

const win = computed(() => !!props.result && props.result.prize_type !== 'none')
// 余额奖或积分奖 ≥ 5 倍单次消耗视为大奖
const big = computed(() => {
  const r = props.result
  if (!r) return false
  return r.prize_type === 'balance' || (r.prize_points ?? 0) >= props.cost * 5
})
const title = computed(() => (win.value ? (big.value ? '好运爆棚！欧气拉满' : '恭喜中奖！') : '差一点点！'))
const subtitle = computed(() => (win.value ? '奖励已即时到账' : '这次没中，下一把就是你'))
const detail = computed(() => {
  const r = props.result
  if (!r) return ''
  if (r.prize_type === 'points') return `+${r.prize_points} 积分 · 已存入积分账户`
  if (r.prize_type === 'balance') return `+$${r.prize_balance} 余额 · 已到账，可直接用于 API 调用`
  return r.desc || '谢谢参与'
})

const COLORS = ['#FFE600', '#FF5E00', '#FF2E93', '#000000', '#FFFFFF']
const confetti = ref<{ id: number; left: number; top: number; color: string; delay: number }[]>([])
watch(
  () => props.result,
  (r) => {
    confetti.value =
      r && r.prize_type !== 'none'
        ? Array.from({ length: big.value ? 48 : 24 }, (_, i) => ({
            id: Date.now() + i,
            left: Math.random() * 95,
            top: Math.random() * 20,
            color: COLORS[i % COLORS.length],
            delay: Math.random() * 0.4
          }))
        : []
  }
)
</script>
