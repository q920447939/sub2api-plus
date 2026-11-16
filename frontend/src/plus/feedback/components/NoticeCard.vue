<!-- [plus] 反馈中心公告：限制高度，图片缩小显示，超出时可展开；避免挤占下方列表 -->
<template>
  <div class="fb-notice" :class="tone">
    <div ref="box" class="relative overflow-hidden transition-[max-height] duration-200" :style="{ maxHeight: expanded ? 'none' : `${collapsedPx}px` }">
      <MarkdownView :source="source" />
      <div v-if="overflow && !expanded" class="fb-notice-fade pointer-events-none absolute inset-x-0 bottom-0 h-10" />
    </div>
    <button v-if="overflow" type="button" class="mt-1 text-xs font-medium text-primary-600 hover:underline dark:text-primary-400" @click="expanded = !expanded">
      {{ expanded ? '收起' : '展开全部' }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import MarkdownView from './MarkdownView.vue'

const props = withDefaults(defineProps<{ source: string; tone?: string; collapsedPx?: number }>(), {
  tone: 'card px-5 py-3 text-gray-600 dark:text-dark-200',
  collapsedPx: 120
})

const box = ref<HTMLElement>()
const expanded = ref(false)
const overflow = ref(false)

function measure() {
  const el = box.value
  if (el) overflow.value = el.scrollHeight > props.collapsedPx + 4
}

// 图片异步加载完才知道真实高度，用 ResizeObserver 跟踪内容尺寸
let ro: ResizeObserver | undefined
onMounted(() => {
  measure()
  const inner = box.value?.firstElementChild
  if (inner && typeof ResizeObserver !== 'undefined') {
    ro = new ResizeObserver(measure)
    ro.observe(inner)
  }
})
onBeforeUnmount(() => ro?.disconnect())
watch(
  () => props.source,
  () => {
    expanded.value = false
    void nextTick(measure)
  }
)
</script>

<style scoped>
/* 公告里的图片最多 160px 高，按比例缩放，不撑满整屏 */
.fb-notice :deep(img) {
  display: block;
  max-height: 160px;
  width: auto;
  max-width: 100%;
  object-fit: contain;
  margin: 0.25rem 0;
  border-radius: 0.5rem;
  box-shadow: none;
}
.fb-notice-fade {
  background: linear-gradient(to bottom, transparent, rgb(255 255 255));
}
:global(.dark) .fb-notice-fade {
  background: linear-gradient(to bottom, transparent, rgb(31 41 55));
}
</style>
