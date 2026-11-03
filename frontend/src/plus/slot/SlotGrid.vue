<template>
  <div
    class="nb-box-xl relative mx-auto grid aspect-square w-full max-w-[540px] grid-cols-3 grid-rows-3 gap-3 p-3 md:gap-4 md:p-4"
    data-testid="slot-grid"
  >
    <template v-for="pos in 9" :key="pos">
      <!-- 中心：GO 按钮 -->
      <div v-if="pos - 1 === 4" class="z-20 flex items-center justify-center p-0.5">
        <button
          type="button"
          class="nb-btn group flex h-full w-full flex-col items-center justify-center bg-[#ff2e93] p-2 text-white hover:bg-[#ff1484]"
          style="border-width: 4px; box-shadow: 8px 8px 0 var(--nb-shadow)"
          :disabled="disabled"
          data-testid="slot-go"
          @click="$emit('go')"
        >
          <span
            class="nb-shadow-sm flex size-9 items-center justify-center border-2 nb-line bg-[#ffe600] text-xl text-black transition-transform group-hover:rotate-12 md:size-11 md:text-2xl"
          >⚡</span>
          <span class="nb-head mt-1 text-lg uppercase text-[#ffe600] md:text-2xl" style="text-shadow: 2px 2px 0 #000">
            {{ spinning ? '摇奖中' : 'GO! 抽奖' }}
          </span>
          <span class="nb-head mt-0.5 border border-white bg-black px-2 py-0.5 text-[10px] md:text-xs">
            {{ cost }} 积分 / 次
          </span>
        </button>
      </div>

      <!-- 外圈奖品格 -->
      <div
        v-else
        class="nb-cell relative flex select-none flex-col items-center justify-between border-[3px] nb-line p-1.5 text-center md:p-2"
        :class="[
          tierAt(pos - 1).idx % 2 === 0 ? 'nb-surface-2' : 'nb-surface',
          active === tierAt(pos - 1).idx ? 'nb-cell-active' : ''
        ]"
        style="box-shadow: 5px 5px 0 var(--nb-shadow)"
        :data-testid="`slot-cell-${tierAt(pos - 1).idx}`"
      >
        <div class="flex w-full items-start justify-between gap-1">
          <span
            v-if="tierAt(pos - 1).tier.badge"
            class="nb-tag nb-cell-badge max-w-full truncate"
            :class="badgeClass(tierAt(pos - 1).idx)"
          >{{ tierAt(pos - 1).tier.badge }}</span>
          <span v-else />
          <span
            v-if="tierAt(pos - 1).tier.boosted"
            class="nb-tag bg-[#ffe600] text-black"
            title="连签加成中，该奖档概率提升"
          >⚡UP</span>
        </div>
        <div class="text-3xl leading-none md:text-4xl">{{ tierAt(pos - 1).tier.emoji || '🎁' }}</div>
        <div class="w-full min-w-0">
          <p class="nb-head truncate text-xs md:text-sm">{{ tierAt(pos - 1).tier.label }}</p>
          <p class="truncate text-[10px] font-bold nb-muted">{{ tierAt(pos - 1).tier.desc || '\u00a0' }}</p>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { RING_TO_GRID, type PublicTier } from './api'

const props = defineProps<{
  tiers: PublicTier[]
  active: number | null
  cost: number
  spinning: boolean
  disabled: boolean
}>()

defineEmits<{ go: [] }>()

// 网格位置 → 外圈顺时针奖档下标
const GRID_TO_RING = Object.fromEntries(RING_TO_GRID.map((grid, ring) => [grid, ring])) as Record<number, number>

function tierAt(gridPos: number): { idx: number; tier: PublicTier } {
  const idx = GRID_TO_RING[gridPos]
  return { idx, tier: props.tiers[idx] ?? { label: '-', prize_type: 'none' } }
}

const BADGE_COLORS = [
  'bg-[#ff5e00] text-white',
  'bg-[#ff2e93] text-white',
  'bg-[#ffe600] text-black',
  'bg-black text-white'
]

function badgeClass(idx: number): string {
  return BADGE_COLORS[idx % BADGE_COLORS.length]
}
</script>
