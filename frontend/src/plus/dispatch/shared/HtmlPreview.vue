<!-- [plus] 存档题作品预览：sandbox iframe（不给 allow-same-origin，读不到本站 cookie / localStorage） -->
<template>
  <div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700" :style="{ aspectRatio: '10 / 7' }">
    <iframe
      class="block h-full w-full"
      :style="thumb ? { width: '400%', height: '400%', transform: 'scale(.25)', transformOrigin: '0 0', pointerEvents: 'none' } : {}"
      sandbox="allow-scripts"
      referrerpolicy="no-referrer"
      :srcdoc="doc"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ html: string; thumb?: boolean }>()
// 禁止作品访问网络：CSP 只允许内联脚本和样式、data: 图片
const CSP = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:">`
const doc = computed(() => (/<head[^>]*>/i.test(props.html) ? props.html.replace(/<head[^>]*>/i, (m) => m + CSP) : CSP + props.html))
</script>
