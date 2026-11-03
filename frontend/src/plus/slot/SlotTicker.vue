<template>
  <div class="overflow-hidden border-b-4 nb-line bg-black py-1.5 text-xs text-[#ffe600] md:text-sm" data-testid="slot-ticker">
    <div class="nb-ticker nb-head gap-10">
      <span v-for="(m, i) in looped" :key="i" class="shrink-0">{{ m }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { FeedItem } from './api'

const props = defineProps<{ items: FeedItem[]; fallback: string[] }>()

const ICONS = ['🔥', '⚡', '💥', '🎉']

function ago(iso: string): string {
  const m = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 60_000))
  if (m < 1) return '刚刚'
  if (m < 60) return `${m} 分钟前`
  const h = Math.floor(m / 60)
  return h < 24 ? `${h} 小时前` : `${Math.floor(h / 24)} 天前`
}

const messages = computed(() => {
  const list = props.items.map(
    (f, i) => `${ICONS[i % ICONS.length]} 玩家 [${f.display_name}] ${ago(f.created_at)}摇中 ${f.emoji} ${f.label}`
  )
  return [...list, ...props.fallback]
})

// 复制一份首尾相接，配合 translateX(-50%) 实现无缝滚动
const looped = computed(() => [...messages.value, ...messages.value])
</script>
