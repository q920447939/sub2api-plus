<!--
  [plus] 中奖动态：头像 + 相对时间 + 大奖高亮，竖向无缝滚动（悬停暂停，减少动态效果时静止）。
  轮询拉到的新记录会以“新”标记出现在顶部。
-->
<template>
  <div class="card p-5">
    <div class="flex items-center justify-between">
      <h2 class="flex items-center gap-2 text-base font-semibold text-gray-900 dark:text-white">
        <Icon name="sparkles" size="sm" class="text-orange-500" /> 中奖动态
      </h2>
      <span class="flex items-center gap-1.5 text-xs text-gray-400">
        <span class="relative flex h-2 w-2" aria-hidden="true">
          <span
            class="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75 motion-reduce:animate-none"
          ></span>
          <span class="relative inline-flex h-2 w-2 rounded-full bg-green-500"></span>
        </span>
        实时
      </span>
    </div>

    <div
      v-if="items.length"
      class="feed-mask relative mt-3 h-72 overflow-hidden"
      @mouseenter="paused = true"
      @mouseleave="paused = false"
      @focusin="paused = true"
      @focusout="paused = false"
    >
      <ul
        :class="{ 'feed-track': scrolling, 'feed-paused': paused }"
        :style="scrolling ? { animationDuration: items.length * 3 + 's' } : undefined"
        aria-live="off"
      >
        <li
          v-for="(item, i) in loop"
          :key="i"
          class="flex items-center gap-3 rounded-lg px-2 py-2"
          :class="isBig(item) ? 'bg-gradient-to-r from-amber-50 to-transparent dark:from-amber-900/20' : ''"
          :aria-hidden="i >= items.length ? 'true' : undefined"
        >
          <span
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-semibold"
            :class="avatarClass(item.display_name)"
            aria-hidden="true"
          >
            {{ avatarText(item.display_name) }}
          </span>
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm text-gray-700 dark:text-gray-300">
              {{ item.display_name }}
              <span
                v-if="fresh.has(key(item))"
                class="ml-1 rounded bg-rose-500 px-1 text-[10px] font-semibold text-white"
                >新</span
              >
            </p>
            <p class="text-xs text-gray-400">
              {{ relativeTime(item.created_at, now) }}
            </p>
          </div>
          <div class="shrink-0 text-right">
            <p
              class="text-sm font-bold tabular-nums"
              :class="isBig(item) ? 'text-rose-600 dark:text-rose-400' : 'text-orange-600 dark:text-orange-400'"
            >
              +${{ item.amount.toFixed(2) }}
            </p>
            <p v-if="isBig(item)" class="flex items-center justify-end gap-0.5 text-[10px] font-semibold text-rose-500">
              <Icon name="fire" size="xs" /> 大奖
            </p>
          </div>
        </li>
      </ul>
    </div>
    <div v-else class="mt-4 rounded-xl border border-dashed border-gray-200 py-8 text-center dark:border-dark-600">
      <p class="text-sm text-gray-400">暂无动态，刮一张试试手气</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import type { FeedItem } from '@/api/lottery'
import { avatarClass, avatarText, bigWinThreshold, relativeTime } from './feedUtils'

const props = defineProps<{ items: FeedItem[]; promoMax: number }>()

const paused = ref(false)
const reduceMotion = typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

// 条目少时不滚动；滚动时复制一份实现无缝循环
const scrolling = computed(() => !reduceMotion && props.items.length > 6)
const loop = computed(() => (scrolling.value ? [...props.items, ...props.items] : props.items))

const big = computed(() => bigWinThreshold(props.promoMax))
const isBig = (f: FeedItem) => f.amount >= big.value

// 相对时间每 30 秒刷新一次
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => (timer = setInterval(() => (now.value = Date.now()), 30_000)))
onBeforeUnmount(() => clearInterval(timer))

// 首屏之后新出现的记录打“新”标记
const key = (f: FeedItem) => `${f.display_name}|${f.created_at}|${f.amount}`
const fresh = ref(new Set<string>())
let seen: Set<string> | null = null
watch(
  () => props.items,
  (list) => {
    const keys = list.map(key)
    if (seen) fresh.value = new Set(keys.filter((k) => !seen!.has(k)))
    seen = new Set(keys)
  },
  { immediate: true }
)
</script>

<style scoped>
.feed-mask {
  mask-image: linear-gradient(to bottom, transparent, #000 8%, #000 92%, transparent);
}
.feed-track {
  animation: feed-scroll linear infinite;
}
.feed-paused {
  animation-play-state: paused;
}
@keyframes feed-scroll {
  from {
    transform: translateY(0);
  }
  to {
    transform: translateY(-50%);
  }
}
</style>
