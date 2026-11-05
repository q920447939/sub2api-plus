<!--
  [plus] 模型生成的 HTML 预览（不可信代码）。
  - sandbox="allow-scripts"，不加 allow-same-origin：文档跑在不透明源里，读不到本站的 Cookie / localStorage
  - 预览接口的响应头里还有 sandbox + 禁止外连的 CSP（后端 handler_user.go）
  - 进入视口才加载（缩略图很多，避免同时跑几十个动画）
  - 缩略图模式下按 1000×700 渲染再整体缩放，看起来就是原画面的缩小版
-->
<template>
  <div ref="box" class="relative w-full overflow-hidden bg-gray-100 dark:bg-dark-800" :style="{ aspectRatio: '10 / 7' }">
    <iframe
      v-if="visible && src"
      :src="fullSrc"
      sandbox="allow-scripts"
      referrerpolicy="no-referrer"
      loading="lazy"
      :title="title"
      :tabindex="thumb ? -1 : 0"
      class="absolute left-0 top-0 border-0 bg-white"
      :class="thumb ? 'pointer-events-none origin-top-left' : 'h-full w-full'"
      :style="thumb ? { width: `${BASE_W}px`, height: `${BASE_H}px`, transform: `scale(${scale})` } : undefined"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { buildApiUrl } from '@/api/client'

const props = withDefaults(defineProps<{ src: string; thumb?: boolean; title?: string }>(), {
  thumb: false,
  title: '动画预览'
})

const BASE_W = 1000
const BASE_H = 700

const box = ref<HTMLElement | null>(null)
const visible = ref(false)
const scale = ref(0.2)
const fullSrc = computed(() => buildApiUrl(props.src))

let io: IntersectionObserver | null = null
let ro: ResizeObserver | null = null

onMounted(() => {
  const el = box.value
  if (!el) return
  if (typeof IntersectionObserver === 'undefined') {
    visible.value = true
  } else {
    io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          visible.value = true
          io?.disconnect()
          io = null
        }
      },
      { rootMargin: '200px' }
    )
    io.observe(el)
  }
  if (props.thumb && typeof ResizeObserver !== 'undefined') {
    ro = new ResizeObserver(() => {
      scale.value = el.clientWidth / BASE_W
    })
    ro.observe(el)
  }
  scale.value = (el.clientWidth || BASE_W * 0.2) / BASE_W
})

onBeforeUnmount(() => {
  io?.disconnect()
  ro?.disconnect()
})
</script>
